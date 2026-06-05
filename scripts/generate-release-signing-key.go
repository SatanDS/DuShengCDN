package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate release signing key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(publicKey))
	fmt.Printf("DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(privateKey))
}
