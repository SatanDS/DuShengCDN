package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const hashedSecretTokenPrefix = "sha256:"

func GenerateSecretToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func HashSecretToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hashedSecretTokenPrefix + hex.EncodeToString(sum[:])
}

func SecretTokenPrefix(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func IsHashedSecretToken(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), hashedSecretTokenPrefix)
}

func VerifySecretTokenHash(token string, expectedHash string) bool {
	hash := HashSecretToken(token)
	expectedHash = strings.TrimSpace(expectedHash)
	if hash == "" || expectedHash == "" || len(hash) != len(expectedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(expectedHash)) == 1
}

func PasswordSessionFingerprint(passwordHash string, sessionSecret string) string {
	passwordHash = strings.TrimSpace(passwordHash)
	sessionSecret = strings.TrimSpace(sessionSecret)
	if sessionSecret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	_, _ = mac.Write([]byte(passwordHash))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func VerifyPasswordSessionFingerprint(passwordHash string, sessionSecret string, expected string) bool {
	fingerprint := PasswordSessionFingerprint(passwordHash, sessionSecret)
	expected = strings.TrimSpace(expected)
	if fingerprint == "" || expected == "" || len(fingerprint) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(fingerprint), []byte(expected)) == 1
}
