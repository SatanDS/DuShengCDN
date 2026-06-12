package service

import "testing"

func TestGSLBPoolSourceHelpersPreservePriorityAcrossPools(t *testing.T) {
	pools := []ProxyRouteGSLBPoolPolicy{
		{Name: "asn", Weight: 100, ASNs: []uint32{4134}, Enabled: true},
		{Name: "operator", Weight: 100, Operators: []string{"cn-telecom"}, Enabled: true},
		{Name: "cidr", Weight: 100, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
		{Name: "country", Weight: 100, Countries: []string{"CN"}, Enabled: true},
	}
	source := GSLBSourceContext{
		IP:       "203.0.113.10",
		ASN:      4134,
		Operator: "China Telecom",
		Country:  "CN",
	}

	pool, match, ok := firstMatchingPoolSource(pools, source, gslbScopePoolSourceMatchPriority...)
	if !ok || pool.Name != "cidr" || match.scopeKey() != "cidr:203.0.113.0/24" {
		t.Fatalf("expected CIDR helper match across later pool, got pool=%q match=%+v ok=%t", pool.Name, match, ok)
	}
	if got := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); got != "cidr:203.0.113.0/24" {
		t.Fatalf("expected CIDR scope, got %q", got)
	}
	assertGSLBMatchedPoolNames(t, matchGSLBPoolsForSource(pools, source), "cidr")

	source.IP = "198.51.100.10"
	pool, match, ok = firstMatchingPoolSource(pools, source, gslbScopePoolSourceMatchPriority...)
	if !ok || pool.Name != "asn" || match.scopeKey() != "asn:4134" {
		t.Fatalf("expected ASN helper match after CIDR miss, got pool=%q match=%+v ok=%t", pool.Name, match, ok)
	}
	if got := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); got != "asn:4134" {
		t.Fatalf("expected ASN scope, got %q", got)
	}
	assertGSLBMatchedPoolNames(t, matchGSLBPoolsForSource(pools, source), "asn")
}

func TestGSLBScopeKeyKeepsSourceCountryWhenNoCountryPoolMatches(t *testing.T) {
	pools := []ProxyRouteGSLBPoolPolicy{
		{Name: "global", Weight: 100, Enabled: true},
		{Name: "jp", Weight: 100, Countries: []string{"JP"}, Enabled: true},
	}
	source := GSLBSourceContext{Country: "cn"}

	if _, match, ok := firstMatchingPoolSource(pools, source, gslbScopePoolSourceMatchPriority...); ok {
		t.Fatalf("expected no pool-derived scope match, got %+v", match)
	}
	if got := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); got != "country:CN" {
		t.Fatalf("expected country scope to remain source-derived, got %q", got)
	}
}

func assertGSLBMatchedPoolNames(t *testing.T, matched map[string]ProxyRouteGSLBPoolPolicy, wantNames ...string) {
	t.Helper()
	if len(matched) != len(wantNames) {
		t.Fatalf("expected matched pools %v, got %+v", wantNames, matched)
	}
	for _, name := range wantNames {
		if matched[name].Name != name {
			t.Fatalf("expected matched pools %v, got %+v", wantNames, matched)
		}
	}
}
