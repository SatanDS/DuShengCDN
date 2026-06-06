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

func TestNormalizeGSLBPolicyKeepsSelectedNodeIDs(t *testing.T) {
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Pools: []ProxyRouteGSLBPoolPolicy{
			{
				Name:   " hk ",
				Weight: 100,
				NodeIDs: []string{
					" node-a ",
					"node-a",
					"",
					"node-b",
				},
				Enabled: true,
			},
		},
	}, "default", 1, "weighted", 60)
	if err != nil {
		t.Fatalf("normalize GSLB policy: %v", err)
	}
	if len(policy.Pools) != 1 {
		t.Fatalf("expected one pool, got %+v", policy.Pools)
	}
	if got := policy.Pools[0].NodeIDs; len(got) != 2 || got[0] != "node-a" || got[1] != "node-b" {
		t.Fatalf("expected deduped selected node ids, got %+v", got)
	}
}

func TestNormalizeGSLBPolicyNormalizesAndMergesOperatorASNScopes(t *testing.T) {
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Pools: []ProxyRouteGSLBPoolPolicy{
			{
				Name:      " edge-cn ",
				Weight:    100,
				Operators: []string{"Telecom", "cn-telecom", "CMCC", ""},
				ASNs:      []uint32{0, 4134, 9808, 4134},
				Enabled:   true,
			},
			{
				Name:      "edge-cn",
				Weight:    200,
				Operators: []string{"china-mobile", "unicom"},
				ASNs:      []uint32{9808, 4837},
				Enabled:   true,
			},
		},
	}, "default", 1, "weighted", 60)
	if err != nil {
		t.Fatalf("normalize GSLB policy: %v", err)
	}
	if len(policy.Pools) != 1 {
		t.Fatalf("expected duplicate pools to merge, got %+v", policy.Pools)
	}
	pool := policy.Pools[0]
	if pool.Name != "edge-cn" {
		t.Fatalf("expected normalized pool name, got %q", pool.Name)
	}
	if pool.Weight != 200 {
		t.Fatalf("expected merged pool to keep latest weight, got %d", pool.Weight)
	}
	wantOperators := []string{"cn-telecom", "cn-mobile", "cn-unicom"}
	if !sameStringSet(pool.Operators, wantOperators) {
		t.Fatalf("expected normalized merged operators %v, got %v", wantOperators, pool.Operators)
	}
	wantASNs := []uint32{4134, 9808, 4837}
	if len(pool.ASNs) != len(wantASNs) {
		t.Fatalf("expected merged ASNs %v, got %v", wantASNs, pool.ASNs)
	}
	for index, want := range wantASNs {
		if pool.ASNs[index] != want {
			t.Fatalf("expected merged ASNs %v, got %v", wantASNs, pool.ASNs)
		}
	}
}

func TestMatchGSLBPoolsForSourcePrefersCIDRASNOperatorCountry(t *testing.T) {
	pools := []ProxyRouteGSLBPoolPolicy{
		{Name: "global", Weight: 100, Enabled: true},
		{Name: "country", Weight: 100, Countries: []string{"CN"}, Enabled: true},
		{Name: "operator", Weight: 100, Operators: []string{"cn-telecom"}, Enabled: true},
		{Name: "asn", Weight: 100, ASNs: []uint32{4134}, Enabled: true},
		{Name: "cidr", Weight: 100, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
	}

	source := GSLBSourceContext{
		IP:       "203.0.113.10",
		Country:  "CN",
		Operator: "China Telecom",
		ASN:      4134,
	}
	matched := matchGSLBPoolsForSource(pools, source)
	if len(matched) != 1 || matched["cidr"].Name != "cidr" {
		t.Fatalf("expected CIDR match to override all other scopes, got %+v", matched)
	}
	if scope := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); scope != "cidr:203.0.113.0/24" {
		t.Fatalf("expected CIDR scope, got %s", scope)
	}

	source.IP = "198.51.100.10"
	matched = matchGSLBPoolsForSource(pools, source)
	if len(matched) != 1 || matched["asn"].Name != "asn" {
		t.Fatalf("expected ASN match to override operator/country, got %+v", matched)
	}
	if scope := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); scope != "asn:4134" {
		t.Fatalf("expected ASN scope, got %s", scope)
	}

	source.ASN = 4837
	matched = matchGSLBPoolsForSource(pools, source)
	if len(matched) != 1 || matched["operator"].Name != "operator" {
		t.Fatalf("expected operator match to override country, got %+v", matched)
	}
	if scope := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); scope != "operator:cn-telecom" {
		t.Fatalf("expected operator scope, got %s", scope)
	}

	source.Operator = "cn-mobile"
	matched = matchGSLBPoolsForSource(pools, source)
	if len(matched) != 1 || matched["country"].Name != "country" {
		t.Fatalf("expected country fallback after ASN/operator miss, got %+v", matched)
	}
	if scope := gslbScopeKeyForPolicy(ProxyRouteGSLBPolicy{Pools: pools}, source); scope != "country:CN" {
		t.Fatalf("expected country fallback scope, got %s", scope)
	}
}
