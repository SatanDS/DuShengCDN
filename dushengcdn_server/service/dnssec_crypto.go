package service

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

func dnssecEncryptionKeyFromEnv() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY"))
	if raw == "" {
		return nil, errors.New("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY is required before enabling DNSSEC")
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

func encryptDNSSECPrivateKey(plain string) (string, error) {
	key, err := dnssecEncryptionKeyFromEnv()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
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
