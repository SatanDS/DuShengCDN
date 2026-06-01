package service

import "testing"

func TestNormalizeProxyRouteDomainsRejectsInjectedDomain(t *testing.T) {
	_, err := normalizeProxyRouteDomains([]string{"good.example.com; include /etc/passwd"})
	if err == nil {
		t.Fatal("expected injected domain to be rejected")
	}
}

func TestNormalizeProxyRouteDomainsAcceptsWildcard(t *testing.T) {
	domains, err := normalizeProxyRouteDomains([]string{"*.example.com"})
	if err != nil {
		t.Fatalf("expected wildcard domain to be accepted: %v", err)
	}
	if len(domains) != 1 || domains[0] != "*.example.com" {
		t.Fatalf("unexpected normalized domains: %#v", domains)
	}
}

func TestNormalizeGSLBPolicySkipsExplicitlyDisabledPools(t *testing.T) {
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Pools: []ProxyRouteGSLBPoolPolicy{
			{Name: "hk", Weight: 100, Enabled: true},
			{Name: "eu", Weight: 1000, Enabled: false},
		},
	}, "default", 1, "weighted", 60)
	if err != nil {
		t.Fatalf("normalize GSLB policy: %v", err)
	}
	if len(policy.Pools) != 1 || policy.Pools[0].Name != "hk" || !policy.Pools[0].Enabled {
		t.Fatalf("expected only explicitly enabled pool to remain, got %+v", policy.Pools)
	}
}

func TestNormalizeGSLBPolicyTreatsLegacyPoolsAsEnabled(t *testing.T) {
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Pools: []ProxyRouteGSLBPoolPolicy{
			{Name: "legacy", Weight: 100},
		},
	}, "default", 1, "weighted", 60)
	if err != nil {
		t.Fatalf("normalize legacy GSLB policy: %v", err)
	}
	if len(policy.Pools) != 1 || policy.Pools[0].Name != "legacy" || !policy.Pools[0].Enabled {
		t.Fatalf("expected legacy pool to stay enabled, got %+v", policy.Pools)
	}
}
