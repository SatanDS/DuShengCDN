package dnsworker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

const dnssecKeyEnvelopePrefix = "v1:"

func decryptDNSSECPrivateKey(envelope string) (string, error) {
	rawKey := strings.TrimSpace(os.Getenv("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY"))
	if rawKey == "" {
		return "", errors.New("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY is required for DNSSEC signing")
	}
	payloadText := strings.TrimSpace(envelope)
	if !strings.HasPrefix(payloadText, dnssecKeyEnvelopePrefix) {
		return "", errors.New("unsupported DNSSEC private key envelope")
	}
	sum := sha256.Sum256([]byte(rawKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(payloadText, dnssecKeyEnvelopePrefix))
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("DNSSEC private key envelope is truncated")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func encryptDNSSECPrivateKeyForWorkerSnapshot(plain string) (string, error) {
	rawKey := strings.TrimSpace(os.Getenv("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY"))
	if rawKey == "" {
		return "", errors.New("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY is required for DNSSEC signing")
	}
	sum := sha256.Sum256([]byte(rawKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	payload := append(nonce, gcm.Seal(nil, nonce, []byte(plain), nil)...)
	return dnssecKeyEnvelopePrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}
