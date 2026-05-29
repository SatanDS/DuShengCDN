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
