package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"dushengcdn/service"
)

const licenseTokenPrefix = "dscdn_license_v1."

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError("")
	}
	command := strings.TrimSpace(args[0])
	switch command {
	case "keygen":
		return runKeygen(args[1:])
	case "sign":
		return runSign(args[1:])
	case "inspect":
		return runInspect(args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		return usageError("unknown command: " + command)
	}
}

func runKeygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	encoding := flags.String("encoding", "base64url", "key encoding: base64url or hex")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError("keygen does not accept positional arguments")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	output := map[string]string{
		"algorithm":   "Ed25519",
		"encoding":    normalizeKeyEncoding(*encoding),
		"public_key":  encodeKey(publicKey, *encoding),
		"private_key": encodeKey(privateKey, *encoding),
	}
	return writeJSON(os.Stdout, output)
}

func runSign(args []string) error {
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	privateKeyRaw := flags.String("private-key", "", "Ed25519 private key, base64url/base64/hex")
	privateKeyFile := flags.String("private-key-file", "", "file containing Ed25519 private key")
	licenseID := flags.String("license-id", "", "license id")
	customerID := flags.String("customer-id", "", "customer id")
	customerName := flags.String("customer-name", "", "customer name")
	plan := flags.String("plan", "business", "license plan")
	features := flags.String("features", "", "comma-separated features; use all for every commercial feature")
	maxNodes := flags.Int("max-nodes", 0, "maximum nodes, 0 means unlimited")
	maxSites := flags.Int("max-sites", 0, "maximum sites, 0 means unlimited")
	issuedAtRaw := flags.String("issued-at", "now", "issued time: now, RFC3339, or YYYY-MM-DD")
	expiresAtRaw := flags.String("expires-at", "", "expiry time: RFC3339 or YYYY-MM-DD; empty means perpetual")
	payloadFile := flags.String("payload-file", "", "optional JSON payload file; flags override its fields")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError("sign does not accept positional arguments")
	}
	privateKey, err := readPrivateKey(*privateKeyRaw, *privateKeyFile)
	if err != nil {
		return err
	}
	payload, err := buildPayload(*payloadFile)
	if err != nil {
		return err
	}
	if value := strings.TrimSpace(*licenseID); value != "" {
		payload.LicenseID = value
	}
	if value := strings.TrimSpace(*customerID); value != "" {
		payload.CustomerID = value
	}
	if value := strings.TrimSpace(*customerName); value != "" {
		payload.CustomerName = value
	}
	if value := strings.TrimSpace(*plan); value != "" {
		payload.Plan = value
	}
	if strings.TrimSpace(*features) != "" {
		payload.Features = parseList(*features)
	}
	provided := providedFlags(args)
	if provided["max-nodes"] {
		payload.MaxNodes = *maxNodes
	}
	if provided["max-sites"] {
		payload.MaxSites = *maxSites
	}
	if provided["issued-at"] || (strings.TrimSpace(*payloadFile) == "" && strings.TrimSpace(*issuedAtRaw) != "") {
		issuedAt, err := parseOptionalTime(*issuedAtRaw, true)
		if err != nil {
			return fmt.Errorf("invalid issued-at: %w", err)
		}
		payload.IssuedAt = issuedAt
	}
	if strings.TrimSpace(*expiresAtRaw) != "" {
		expiresAt, err := parseOptionalTime(*expiresAtRaw, false)
		if err != nil {
			return fmt.Errorf("invalid expires-at: %w", err)
		}
		payload.ExpiresAt = expiresAt
	}
	if err := validatePayload(payload); err != nil {
		return err
	}
	token, payloadJSON, err := signPayload(privateKey, payload)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, token)
	fmt.Fprintln(os.Stderr, string(payloadJSON))
	return nil
}

func providedFlags(args []string) map[string]bool {
	result := make(map[string]bool)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		if key, _, ok := strings.Cut(name, "="); ok {
			name = key
		}
		result[name] = true
	}
	return result
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	tokenRaw := flags.String("token", "", "license token")
	tokenFile := flags.String("token-file", "", "file containing license token")
	publicKeyRaw := flags.String("public-key", "", "optional Ed25519 public key for signature verification")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return usageError("inspect does not accept positional arguments")
	}
	token, err := readValue(*tokenRaw, *tokenFile, "license token")
	if err != nil {
		return err
	}
	payloadBytes, signature, err := decodeToken(token)
	if err != nil {
		return err
	}
	var payload service.CommercialLicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("invalid license payload: %w", err)
	}
	result := map[string]any{
		"payload": payload,
	}
	if strings.TrimSpace(*publicKeyRaw) != "" {
		publicKey, err := decodeKey(*publicKeyRaw, ed25519.PublicKeySize, "public key")
		if err != nil {
			return err
		}
		result["signature_verified"] = ed25519.Verify(ed25519.PublicKey(publicKey), payloadBytes, signature)
	}
	return writeJSON(os.Stdout, result)
}

func buildPayload(payloadFile string) (service.CommercialLicensePayload, error) {
	payload := service.CommercialLicensePayload{}
	if strings.TrimSpace(payloadFile) == "" {
		return payload, nil
	}
	raw, err := os.ReadFile(payloadFile)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func signPayload(privateKey ed25519.PrivateKey, payload service.CommercialLicensePayload) (string, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	signature := ed25519.Sign(privateKey, raw)
	token := licenseTokenPrefix +
		base64.RawURLEncoding.EncodeToString(raw) +
		"." +
		base64.RawURLEncoding.EncodeToString(signature)
	return token, raw, nil
}

func decodeToken(token string) ([]byte, []byte, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, licenseTokenPrefix) {
		return nil, nil, errors.New("license token must start with dscdn_license_v1.")
	}
	compact := strings.TrimPrefix(token, licenseTokenPrefix)
	parts := strings.Split(compact, ".")
	if len(parts) != 2 {
		return nil, nil, errors.New("license token format is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("decode payload: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, nil, errors.New("license signature length is invalid")
	}
	return payload, signature, nil
}

func validatePayload(payload service.CommercialLicensePayload) error {
	if strings.TrimSpace(payload.LicenseID) == "" {
		return errors.New("license-id is required")
	}
	if strings.TrimSpace(payload.CustomerID) == "" && strings.TrimSpace(payload.CustomerName) == "" {
		return errors.New("customer-id or customer-name is required")
	}
	if strings.TrimSpace(payload.Plan) == "" {
		return errors.New("plan is required")
	}
	if payload.IssuedAt != nil && payload.ExpiresAt != nil && !payload.ExpiresAt.After(*payload.IssuedAt) {
		return errors.New("expires-at must be after issued-at")
	}
	return nil
}

func readPrivateKey(raw string, path string) (ed25519.PrivateKey, error) {
	value, err := readValue(raw, path, "private key")
	if err != nil {
		return nil, err
	}
	decoded, err := decodeKey(value, ed25519.PrivateKeySize, "private key")
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(decoded), nil
}

func readValue(raw string, path string, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	path = strings.TrimSpace(path)
	if raw != "" && path != "" {
		return "", fmt.Errorf("%s: use either inline value or file, not both", label)
	}
	if raw != "" {
		return raw, nil
	}
	if path == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func decodeKey(raw string, expectedSize int, label string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "base64url:")
	value = strings.TrimPrefix(value, "base64:")
	value = strings.TrimPrefix(value, "hex:")
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		decoded, err = hex.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("%s encoding is invalid", label)
	}
	if len(decoded) != expectedSize {
		return nil, fmt.Errorf("%s length is invalid: got %d want %d", label, len(decoded), expectedSize)
	}
	return decoded, nil
}

func encodeKey(key []byte, encoding string) string {
	switch normalizeKeyEncoding(encoding) {
	case "hex":
		return hex.EncodeToString(key)
	default:
		return base64.RawURLEncoding.EncodeToString(key)
	}
}

func normalizeKeyEncoding(encoding string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "hex":
		return "hex"
	default:
		return "base64url"
	}
}

func parseList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			values = append(values, field)
		}
	}
	return values
}

func parseOptionalTime(raw string, defaultNow bool) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if defaultNow && strings.EqualFold(value, "now") {
		now := time.Now().UTC().Truncate(time.Second)
		return &now, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	return nil, errors.New("expected now, RFC3339, or YYYY-MM-DD")
}

func writeJSON(writer *os.File, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usageError(message string) error {
	if message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	printUsage(os.Stderr)
	if message == "" {
		return errors.New("missing command")
	}
	return errors.New(message)
}

func printUsage(writer *os.File) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  go run ./cmd/license keygen [-encoding base64url|hex]")
	fmt.Fprintln(writer, "  go run ./cmd/license sign -private-key <key> -license-id <id> -customer-name <name> [options]")
	fmt.Fprintln(writer, "  go run ./cmd/license inspect -token <token> [-public-key <key>]")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Features: all, acme-automation, authoritative-dns, cloudflare-dns, gslb, ddos-protection, waf, cc-protection, country-region-access-control, operator-access-control, source-cidr-access-control, asn-access-control")
}
