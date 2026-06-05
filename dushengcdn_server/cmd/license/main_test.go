package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"dushengcdn/service"
)

func TestSignPayloadProducesVerifiableToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	payload := service.CommercialLicensePayload{
		LicenseID:    "lic-test",
		CustomerName: "Example Ltd.",
		Plan:         "enterprise",
		Features:     []string{"all"},
		MaxNodes:     3,
		MaxSites:     10,
	}

	token, payloadJSON, err := signPayload(privateKey, payload)
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}
	if !strings.HasPrefix(token, licenseTokenPrefix) {
		t.Fatalf("unexpected token prefix: %s", token)
	}
	decodedPayload, signature, err := decodeToken(token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if !bytes.Equal(decodedPayload, payloadJSON) {
		t.Fatal("decoded payload does not match signed payload")
	}
	if !ed25519.Verify(publicKey, decodedPayload, signature) {
		t.Fatal("expected signature to verify")
	}
}

func TestRunKeygenOutputsUsableKeys(t *testing.T) {
	stdout, restore := captureStdout(t)
	if err := run([]string{"keygen"}); err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	restore()

	var output map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode keygen output: %v", err)
	}
	if _, err := decodeKey(output["public_key"], ed25519.PublicKeySize, "public key"); err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	if _, err := decodeKey(output["private_key"], ed25519.PrivateKeySize, "private key"); err != nil {
		t.Fatalf("decode private key: %v", err)
	}
}

func TestRunSignAndInspect(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateKeyValue := base64.RawURLEncoding.EncodeToString(privateKey)
	publicKeyValue := base64.RawURLEncoding.EncodeToString(publicKey)

	stdout, restore := captureStdout(t)
	if err := run([]string{
		"sign",
		"-private-key", privateKeyValue,
		"-license-id", "lic-cli",
		"-customer-name", "CLI Customer",
		"-plan", "enterprise",
		"-features", "all,waf",
		"-max-nodes", "5",
		"-max-sites", "20",
		"-expires-at", "2030-01-01",
	}); err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	restore()

	token := strings.TrimSpace(stdout.String())
	if token == "" {
		t.Fatal("expected token output")
	}

	inspectStdout, inspectRestore := captureStdout(t)
	if err := run([]string{"inspect", "-token", token, "-public-key", publicKeyValue}); err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	inspectRestore()

	var result struct {
		Payload           service.CommercialLicensePayload `json:"payload"`
		SignatureVerified bool                             `json:"signature_verified"`
	}
	if err := json.Unmarshal(inspectStdout.Bytes(), &result); err != nil {
		t.Fatalf("decode inspect output: %v", err)
	}
	if !result.SignatureVerified {
		t.Fatal("expected signature to verify")
	}
	if result.Payload.LicenseID != "lic-cli" || result.Payload.CustomerName != "CLI Customer" {
		t.Fatalf("unexpected inspected payload: %+v", result.Payload)
	}
}

func TestRunSignPreservesPayloadFileLimitsUnlessFlagsOverride(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateKeyValue := base64.RawURLEncoding.EncodeToString(privateKey)
	issuedAt, err := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("parse issued_at: %v", err)
	}
	payloadPath := writePayloadFile(t, service.CommercialLicensePayload{
		LicenseID:    "lic-payload",
		CustomerName: "Payload Customer",
		Plan:         "business",
		MaxNodes:     7,
		MaxSites:     70,
		IssuedAt:     &issuedAt,
	})

	stdout, restore := captureStdout(t)
	if err := run([]string{
		"sign",
		"-private-key", privateKeyValue,
		"-payload-file", payloadPath,
		"-max-sites", "0",
	}); err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	restore()

	payload, _, err := decodeIssuedPayload(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("decode issued payload: %v", err)
	}
	if payload.MaxNodes != 7 || payload.MaxSites != 0 {
		t.Fatalf("unexpected payload limits: %+v", payload)
	}
	if payload.IssuedAt == nil || !payload.IssuedAt.Equal(issuedAt) {
		t.Fatalf("expected payload-file issued_at to be preserved, got %+v", payload.IssuedAt)
	}
}

func writePayloadFile(t *testing.T, payload service.CommercialLicensePayload) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	path := t.TempDir() + "/payload.json"
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write payload file: %v", err)
	}
	return path
}

func decodeIssuedPayload(token string) (service.CommercialLicensePayload, []byte, error) {
	var payload service.CommercialLicensePayload
	raw, signature, err := decodeToken(token)
	if err != nil {
		return payload, nil, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, nil, err
	}
	return payload, signature, nil
}

func captureStdout(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	buffer := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(buffer, reader)
		close(done)
	}()
	return buffer, func() {
		_ = writer.Close()
		<-done
		os.Stdout = original
		_ = reader.Close()
	}
}
