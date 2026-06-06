package dnsworker

import (
	"reflect"
	"testing"
)

func TestNormalizePolicyPreservesOperatorAndASNSelectors(t *testing.T) {
	route := SnapshotRoute{
		NodePool:     "default",
		ScheduleMode: "weighted",
		TargetCount:  1,
		TTL:          30,
	}
	policy := normalizePolicy(GSLBPolicy{
		Pools: []GSLBPoolPolicy{
			{
				Name:      " CN ",
				Weight:    0,
				Operators: []string{" China Telecom ", "cn-telecom", "", "CMCC", "custom isp"},
				ASNs:      []uint32{0, 4134, 9808, 4134},
				Enabled:   true,
			},
		},
	}, route)

	if len(policy.Pools) != 1 {
		t.Fatalf("expected one normalized pool, got %+v", policy.Pools)
	}
	pool := policy.Pools[0]
	if pool.Name != "cn" {
		t.Fatalf("expected pool name to be normalized, got %q", pool.Name)
	}
	if pool.Weight != 100 {
		t.Fatalf("expected default weight, got %d", pool.Weight)
	}
	if !reflect.DeepEqual(pool.Operators, []string{"cn-telecom", "cn-mobile", "custom-isp"}) {
		t.Fatalf("unexpected normalized operators: %+v", pool.Operators)
	}
	if !reflect.DeepEqual(pool.ASNs, []uint32{4134, 9808}) {
		t.Fatalf("unexpected normalized ASNs: %+v", pool.ASNs)
	}
}

func TestMatchPoolsForSourcePrefersCIDRASNOperatorCountryGlobal(t *testing.T) {
	pools := []GSLBPoolPolicy{
		{Name: "global", Weight: 100, Enabled: true},
		{Name: "country", Weight: 100, Countries: []string{"CN"}, Enabled: true},
		{Name: "operator", Weight: 100, Operators: []string{"cn-telecom"}, Enabled: true},
		{Name: "asn", Weight: 100, ASNs: []uint32{4134}, Enabled: true},
		{Name: "cidr", Weight: 100, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
		{Name: "disabled", Weight: 100, SourceCIDRs: []string{"203.0.113.0/24"}, ASNs: []uint32{4134}, Operators: []string{"cn-telecom"}, Countries: []string{"CN"}, Enabled: false},
	}
	policy := GSLBPolicy{Pools: pools}

	tests := []struct {
		name      string
		source    SourceContext
		wantPools []string
		wantScope string
	}{
		{
			name:      "cidr before asn operator and country",
			source:    SourceContext{IP: "203.0.113.10", ASN: 4134, Operator: "cn-telecom", Country: "CN"},
			wantPools: []string{"cidr"},
			wantScope: "cidr:203.0.113.0/24",
		},
		{
			name:      "asn before operator and country",
			source:    SourceContext{IP: "198.51.100.10", ASN: 4134, Operator: "cn-telecom", Country: "CN"},
			wantPools: []string{"asn"},
			wantScope: "asn:4134",
		},
		{
			name:      "operator before country",
			source:    SourceContext{IP: "198.51.100.10", Operator: "China Telecom", Country: "CN"},
			wantPools: []string{"operator"},
			wantScope: "operator:cn-telecom",
		},
		{
			name:      "country before global",
			source:    SourceContext{IP: "198.51.100.10", Country: "CN"},
			wantPools: []string{"country"},
			wantScope: "country:CN",
		},
		{
			name:      "global fallback",
			source:    SourceContext{IP: "198.51.100.10"},
			wantPools: []string{"global", "country", "operator", "asn", "cidr"},
			wantScope: "global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPools := matchPoolsForSource(pools, tt.source)
			assertPoolNames(t, gotPools, tt.wantPools)
			if gotScope := sourceScopeKeyForPolicy(policy, tt.source); gotScope != tt.wantScope {
				t.Fatalf("expected scope %q, got %q", tt.wantScope, gotScope)
			}
		})
	}
}

func TestSourceScopeKeyForPolicyIgnoresUnconfiguredASNOrOperatorScopes(t *testing.T) {
	policy := GSLBPolicy{Pools: []GSLBPoolPolicy{
		{Name: "country", Countries: []string{"CN"}, Enabled: true},
		{Name: "global", Enabled: true},
	}}

	source := SourceContext{ASN: 4134, Operator: "cn-telecom", Country: "CN"}
	if got := sourceScopeKeyForPolicy(policy, source); got != "country:CN" {
		t.Fatalf("expected source scope key to fall back to country when no ASN/operator pool matches, got %q", got)
	}
}

func TestClassifySourceOperatorValueHandlesAliasesAndFallbacks(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "telecom", value: "ChinaNet Backbone", want: "cn-telecom"},
		{name: "unicom", value: "China Unicom Guangdong", want: "cn-unicom"},
		{name: "mobile", value: "CMCC mobile network", want: "cn-mobile"},
		{name: "broadcast", value: "China Broadcast Network", want: "cn-broadcast"},
		{name: "cernet", value: "CERNET Center", want: "cernet"},
		{name: "custom fallback", value: "Example ISP", want: "example-isp"},
		{name: "blank", value: "  ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifySourceOperatorValue(tt.value); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizeSourceScopeBaseSupportsOperatorAndASN(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: " operator: China Telecom ", want: "operator:cn-telecom"},
		{raw: "operator:CMCC", want: "operator:cn-mobile"},
		{raw: "asn:AS4134", want: "asn:4134"},
		{raw: "asn: 9808 ", want: "asn:9808"},
		{raw: "asn:0", want: "asn:0"},
		{raw: "operator:   ", want: "operator:"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := normalizeSourceScopeBase(tt.raw); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func assertPoolNames(t *testing.T, pools map[string]GSLBPoolPolicy, want []string) {
	t.Helper()
	got := make(map[string]struct{}, len(pools))
	for name := range pools {
		got[name] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("expected pools %v, got %v", want, got)
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("expected pools %v, got %v", want, got)
		}
	}
}
