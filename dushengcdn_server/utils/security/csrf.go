package security

import (
	"crypto/subtle"
	"strings"
)

const CSRFTokenLength = 32

func GenerateCSRFToken() string {
	return GenerateRandomString(CSRFTokenLength)
}

func VerifyCSRFToken(expected string, provided string) bool {
	expected = strings.TrimSpace(expected)
	provided = strings.TrimSpace(provided)
	if expected == "" || provided == "" {
		return false
	}
	if len(expected) != len(provided) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
