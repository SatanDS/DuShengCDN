package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	privateKeyText := flag.String("private-key", "", "base64-encoded Ed25519 seed or private key")
	assetName := flag.String("asset", "", "release asset name")
	tagName := flag.String("tag", "", "release tag name")
	checksumFile := flag.String("checksum-file", "", "sha256 checksum file")
	signatureFile := flag.String("signature-file", "", "signature output file")
	flag.Parse()

	if err := run(*privateKeyText, *assetName, *tagName, *checksumFile, *signatureFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(privateKeyText string, assetName string, tagName string, checksumFile string, signatureFile string) error {
	privateKey, err := parsePrivateKey(privateKeyText)
	if err != nil {
		return err
	}
	assetName = strings.TrimSpace(assetName)
	if assetName == "" {
		return fmt.Errorf("asset name is required")
	}
	tagName = strings.TrimSpace(tagName)
	if tagName == "" {
		return fmt.Errorf("release tag is required")
	}
	checksum, err := readChecksum(checksumFile, assetName)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(privateKey, releaseSignaturePayload(tagName, assetName, checksum))
	output := base64.StdEncoding.EncodeToString(signature) + "\n"
	if err = os.WriteFile(signatureFile, []byte(output), 0o644); err != nil {
		return fmt.Errorf("write signature file: %w", err)
	}
	return nil
}

func parsePrivateKey(value string) (ed25519.PrivateKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("release signing private key is required")
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("release signing private key must be base64")
	}
	switch len(key) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(key), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(key), nil
	default:
		return nil, fmt.Errorf("release signing private key must be an Ed25519 seed or private key")
	}
}

func readChecksum(path string, assetName string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read checksum file: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 1 && isSHA256Hex(fields[0]) {
			return strings.ToLower(fields[0]), nil
		}
		if len(fields) >= 2 && isSHA256Hex(fields[0]) {
			fileName := strings.TrimPrefix(fields[1], "*")
			if fileName == assetName || filepath.Base(fileName) == assetName {
				return strings.ToLower(fields[0]), nil
			}
		}
	}
	return "", fmt.Errorf("checksum file does not contain a sha256 digest for %q", assetName)
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func releaseSignaturePayload(tagName string, assetName string, checksum string) []byte {
	return []byte(strings.Join([]string{
		"dushengcdn-release-v1",
		strings.TrimSpace(tagName),
		strings.TrimSpace(assetName),
		strings.ToLower(strings.TrimSpace(checksum)),
		"",
	}, "\n"))
}
