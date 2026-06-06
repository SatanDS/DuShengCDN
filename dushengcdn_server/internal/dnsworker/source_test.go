package dnsworker

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
)

func TestSourceResolverStatusReportsGeoIPOpenError(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.mmdb")
	resolver, err := NewSourceResolver(missingPath)
	if err == nil {
		t.Fatal("expected missing GeoIP database to return an error")
	}
	if resolver == nil {
		t.Fatal("expected resolver to be returned after GeoIP open failure")
	}
	status := resolver.Status()
	if status.Enabled {
		t.Fatalf("expected GeoIP to be disabled after open failure: %+v", status)
	}
	if status.DatabasePath != missingPath {
		t.Fatalf("expected database path %q, got %q", missingPath, status.DatabasePath)
	}
	if status.LastError == "" {
		t.Fatal("expected GeoIP open error to be recorded")
	}

	source := resolver.Resolve(&dns.Msg{}, &net.UDPAddr{IP: net.ParseIP("198.51.100.10"), Port: 53000})
	if source.IP != "198.51.100.10" {
		t.Fatalf("expected source IP to resolve without GeoIP database, got %+v", source)
	}
	if source.Country != "" || source.ScopeKey != "global" {
		t.Fatalf("expected unresolved country to fall back to global, got %+v", source)
	}
}

func TestSourceResolverUsesOperatorCIDRDatabase(t *testing.T) {
	dir := t.TempDir()
	operatorDir := filepath.Join(dir, "operator-cidr")
	if err := os.Mkdir(operatorDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(operatorDir, "unicom.txt"), []byte("198.51.100.0/24\n"), 0o644); err != nil {
		t.Fatalf("write operator CIDR: %v", err)
	}

	resolver, err := NewSourceResolver("", "", operatorDir)
	if err != nil {
		t.Fatalf("NewSourceResolver: %v", err)
	}
	status := resolver.Status()
	if !status.OperatorEnabled {
		t.Fatalf("expected operator capability to be enabled: %+v", status)
	}
	source := resolver.Resolve(&dns.Msg{}, &net.UDPAddr{IP: net.ParseIP("198.51.100.10"), Port: 53000})
	if source.Operator != "cn-unicom" || source.Country != "CN" || source.ScopeKey != "operator:cn-unicom" {
		t.Fatalf("expected China Unicom operator context, got %+v", source)
	}
}
