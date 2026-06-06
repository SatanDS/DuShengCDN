package dnsworker

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorCIDRMatcherLoadsGaoyifanStyleDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"chinanet.txt": "203.0.113.0/24\n",
		"cmcc6.txt":    "2001:db8:8::/48\n",
		"china.txt":    "198.51.100.0/24\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	matcher, err := LoadOperatorCIDRMatcher(dir)
	if err != nil {
		t.Fatalf("LoadOperatorCIDRMatcher: %v", err)
	}
	if matcher.Count() != 2 {
		t.Fatalf("expected only operator-specific files to load, got %d ranges", matcher.Count())
	}
	if got := matcher.Lookup(parseTestIP(t, "203.0.113.10")); got != "cn-telecom" {
		t.Fatalf("expected telecom match, got %q", got)
	}
	if got := matcher.Lookup(parseTestIP(t, "2001:db8:8::1")); got != "cn-mobile" {
		t.Fatalf("expected mobile match, got %q", got)
	}
	if got := matcher.Lookup(parseTestIP(t, "198.51.100.10")); got != "" {
		t.Fatalf("expected aggregate china.txt to be ignored, got %q", got)
	}
}

func parseTestIP(t *testing.T, value string) net.IP {
	t.Helper()
	ip := net.ParseIP(value)
	if ip == nil {
		t.Fatalf("invalid test IP %s", value)
	}
	return ip
}
