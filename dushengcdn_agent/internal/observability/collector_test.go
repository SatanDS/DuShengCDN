package observability

import (
	"testing"

	"dushengcdn-agent/internal/protocol"
)

func TestFingerprintProfileIgnoresVolatileFields(t *testing.T) {
	first := &protocol.NodeSystemProfile{
		Hostname:         "edge-01",
		OSName:           "linux",
		Architecture:     "amd64",
		CPUModel:         "cpu",
		CPUCores:         4,
		TotalMemoryBytes: 8 * 1024 * 1024,
		TotalDiskBytes:   128 * 1024 * 1024,
		UptimeSeconds:    10,
		ReportedAtUnix:   100,
	}
	second := *first
	second.UptimeSeconds = 20
	second.ReportedAtUnix = 200

	if fingerprintProfile(first) != fingerprintProfile(&second) {
		t.Fatal("expected volatile profile fields to be ignored by fingerprint")
	}
}
