package dnsprobe

import (
	"context"
	"net"
	"strings"
	"testing"

	"dushengcdn-agent/internal/protocol"
)

func TestProbeTargetsRejectsUnsafePublicAddress(t *testing.T) {
	reports := ProbeTargets(context.Background(), []protocol.DNSProbeTarget{
		{
			WorkerID:      "dns-worker-1",
			PublicAddress: "169.254.169.254:53",
			QueryName:     "example.com",
			QueryType:     "SOA",
		},
	})
	if len(reports) != 1 || len(reports[0].Results) != 2 {
		t.Fatalf("expected invalid address report, got %+v", reports)
	}
	for _, result := range reports[0].Results {
		if result.Reachable || result.Error == "" {
			t.Fatalf("expected failed result with error, got %+v", result)
		}
	}
}

func TestResolvePublicProbeTargetsRejectsUnsafeResolvedHost(t *testing.T) {
	restoreLookupIPAddr(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "ns1.example.net" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})

	_, err := resolvePublicProbeTargets(context.Background(), "ns1.example.net:53")
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("expected unsafe resolved ip error, got %v", err)
	}
}

func TestResolvePublicProbeTargetsAllowsPublicAddressOnPort53(t *testing.T) {
	restoreLookupIPAddr(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "ns1.example.net" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})

	targets, err := resolvePublicProbeTargets(context.Background(), "ns1.example.net:53")
	if err != nil {
		t.Fatalf("resolvePublicProbeTargets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "8.8.8.8:53" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if _, err := resolvePublicProbeTargets(context.Background(), "8.8.4.4:8053"); err == nil {
		t.Fatal("expected non-53 port to be rejected")
	}
}

func restoreLookupIPAddr(t *testing.T, lookup func(context.Context, string) ([]net.IPAddr, error)) {
	t.Helper()
	original := lookupIPAddr
	lookupIPAddr = lookup
	t.Cleanup(func() {
		lookupIPAddr = original
	})
}
