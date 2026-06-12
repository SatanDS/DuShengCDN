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

func TestOperatorCIDRMatcherMatchesNetipRangesAtBoundaries(t *testing.T) {
	ranges := []operatorCIDRRange{}
	for _, item := range []struct {
		value    string
		operator string
	}{
		{value: "198.51.100.42", operator: "cn-unicom"},
		{value: "203.0.113.0/24", operator: "cn-telecom"},
		{value: "2001:db8:8::/125", operator: "cn-mobile"},
	} {
		parsed, err := parseOperatorCIDR(item.value, item.operator)
		if err != nil {
			t.Fatalf("parse %s: %v", item.value, err)
		}
		ranges = append(ranges, parsed)
	}
	matcher := &OperatorCIDRMatcher{ranges: ranges}

	tests := []struct {
		ip   string
		want string
	}{
		{ip: "203.0.113.0", want: "cn-telecom"},
		{ip: "203.0.113.255", want: "cn-telecom"},
		{ip: "203.0.114.0", want: ""},
		{ip: "198.51.100.42", want: "cn-unicom"},
		{ip: "198.51.100.43", want: ""},
		{ip: "2001:db8:8::", want: "cn-mobile"},
		{ip: "2001:db8:8::7", want: "cn-mobile"},
		{ip: "2001:db8:8::8", want: ""},
	}
	for _, tt := range tests {
		if got := matcher.Lookup(parseTestIP(t, tt.ip)); got != tt.want {
			t.Fatalf("Lookup(%s) = %q, want %q", tt.ip, got, tt.want)
		}
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
