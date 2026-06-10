package dnsworker

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	SnapshotSignatureVersion = "dushengcdn-dns-snapshot-hmac-sha256-v1"
	SnapshotSignatureHeader  = "X-DNS-Snapshot-Signature"
)

type SignedSnapshot struct {
	SignatureVersion string   `json:"signature_version"`
	Signature        string   `json:"signature"`
	Snapshot         Snapshot `json:"snapshot"`
}

func SignSnapshot(snapshot *Snapshot, token string) (*SignedSnapshot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("snapshot signing token cannot be empty")
	}
	signature, err := computeSnapshotSignature(snapshot, token)
	if err != nil {
		return nil, err
	}
	normalized := normalizeSnapshot(snapshot)
	return &SignedSnapshot{
		SignatureVersion: SnapshotSignatureVersion,
		Signature:        signature,
		Snapshot:         *normalized,
	}, nil
}

func VerifySignedSnapshot(envelope *SignedSnapshot, token string) error {
	if envelope == nil {
		return errors.New("signed snapshot is missing")
	}
	if strings.TrimSpace(envelope.SignatureVersion) != SnapshotSignatureVersion {
		return fmt.Errorf("unsupported snapshot signature version %q", envelope.SignatureVersion)
	}
	expected, err := computeSnapshotSignature(&envelope.Snapshot, token)
	if err != nil {
		return err
	}
	actual, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Signature))
	if err != nil {
		return errors.New("snapshot signature is invalid")
	}
	want, err := base64.StdEncoding.DecodeString(expected)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(actual, want) != 1 {
		return errors.New("snapshot signature verification failed")
	}
	return nil
}

func computeSnapshotSignature(snapshot *Snapshot, token string) (string, error) {
	if snapshot == nil || strings.TrimSpace(snapshot.SnapshotVersion) == "" {
		return "", errors.New("snapshot is invalid")
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("snapshot signing token cannot be empty")
	}
	payload, err := canonicalSnapshotSignaturePayload(snapshot)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func canonicalSnapshotSignaturePayload(snapshot *Snapshot) ([]byte, error) {
	normalized := normalizeSnapshot(snapshot)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	var payload bytes.Buffer
	payload.WriteString(SnapshotSignatureVersion)
	payload.WriteByte('\n')
	payload.WriteString(strings.TrimSpace(normalized.SnapshotVersion))
	payload.WriteByte('\n')
	payload.Write(raw)
	return payload.Bytes(), nil
}
