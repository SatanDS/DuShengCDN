package dnsworker

import (
	"net"
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
