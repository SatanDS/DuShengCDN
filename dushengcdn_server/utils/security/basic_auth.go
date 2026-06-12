package security

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const BasicAuthCredentialHashMaterial = "dushengcdn basic auth v1\n"

func BasicAuthCredentialHash(username, password string) string {
	credentials := strings.TrimSpace(username) + ":" + strings.TrimSpace(password)
	if credentials == ":" {
		return ""
	}
	sum := sha256.Sum256([]byte(BasicAuthCredentialHashMaterial + credentials))
	return hex.EncodeToString(sum[:])
}
