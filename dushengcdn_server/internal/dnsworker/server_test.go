package dnsworker

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestResolveStaticRecordsAndGeneratedSOA(t *testing.T) {
	server := testServer(t, baseSnapshot())

	response := server.Resolve(testQuery("www.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected one A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.8.8" {
		t.Fatalf("unexpected A answer: %s", got)
	}

	response = server.Resolve(testQuery("example.com", dns.TypeSOA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected SOA answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	soa := response.Answer[0].(*dns.SOA)
	if soa.Ns != "ns1.example.com." || soa.Serial != 123 {
		t.Fatalf("unexpected SOA: %+v", soa)
	}
}

func TestResolveNXDOMAINAndNODATA(t *testing.T) {
	server := testServer(t, baseSnapshot())

	response := server.Resolve(testQuery("missing.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %s", dns.RcodeToString[response.Rcode])
	}

	response = server.Resolve(testQuery("www.example.com", dns.TypeAAAA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 0 {
		t.Fatalf("expected NODATA style NOERROR/empty, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
}

func TestResolveDynamicRouteSkipsUnhealthyAndLoadThresholds(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           7,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "load_aware",
			TTL:          1,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "load_aware",
				TargetCount: 1,
				TTL:         1,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
				LoadThresholds: GSLBLoadThresholds{
					MaxOpenrestyConnections: 100,
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("overloaded", "hk", "1.1.1.1", 100, 999),
		testNode("healthy", "hk", "8.8.4.4", 100, 10),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected healthy node target, got %s", got)
	}
	if ttl := response.Answer[0].Header().Ttl; ttl != DefaultAuthoritativeTTL {
		t.Fatalf("expected authoritative auto TTL %d, got %d", DefaultAuthoritativeTTL, ttl)
	}
}

func TestResolveDynamicRouteMatchesSingleLevelWildcardDomain(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           30,
			Domains:      []string{"*.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("hk-node", "hk", "8.8.4.4", 100, 1),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("api.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected wildcard dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected wildcard route target, got %s", got)
	}

	response = server.Resolve(testQuery("deep.api.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeNameError {
		t.Fatalf("expected single-level wildcard not to match deep subdomain, got %s", dns.RcodeToString[response.Rcode])
	}
}

func TestResolveDynamicRouteSkipsDisabledGSLBPool(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           29,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
					{Name: "disabled", Weight: 1000, Enabled: false},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("disabled-node", "disabled", "9.9.9.9", 1000, 1),
		testNode("hk-node", "hk", "8.8.4.4", 100, 1),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected enabled pool target, got %s", got)
	}
}

func TestResolveDynamicRouteRespectsSelectedPoolNodeIDs(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           31,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, NodeIDs: []string{"node-backup"}, Enabled: true},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("node-primary", "hk", "8.8.8.8", 1000, 1),
		testNode("node-backup", "hk", "1.1.1.1", 1, 1),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "1.1.1.1" {
		t.Fatalf("expected selected node target, got %s", got)
	}
}

func TestResolveDynamicRouteTreatsLegacyGSLBPoolsAsEnabled(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           30,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "legacy", Weight: 100},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("legacy-node", "legacy", "9.9.9.9", 100, 1),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected legacy pool answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "9.9.9.9" {
		t.Fatalf("expected legacy pool target, got %s", got)
	}
}

func TestResolveDynamicRouteSkipsNonPublicSnapshotIPs(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           31,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("private-node", "hk", "10.0.0.1", 1000, 1),
		testNode("public-node", "hk", "8.8.4.4", 100, 1),
	}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected non-public snapshot IP to be skipped, got %s", got)
	}
}

func TestResolveDynamicRouteLoadAwarePrefersFreshMetrics(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           19,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "load_aware",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "load_aware",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	withMetric := testNode("with-metric", "hk", "8.8.4.4", 10, 8)
	withoutMetric := testNode("without-metric", "hk", "1.1.1.1", 1000, 0)
	withoutMetric.MetricCapturedAt = nil
	snapshot.Nodes = []SnapshotNode{withoutMetric, withMetric}
	server := testServer(t, snapshot)
	server.Scheduler = NewScheduler()

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected node with fresh metrics before missing metric fallback, got %s", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingSkipsUnhealthyProbe(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = true
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           20,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	unhealthyProbe := testNode("unhealthy-probe", "hk", "1.1.1.1", 1000, 1)
	unhealthyProbe.DNSProbeHealthy = false
	healthyProbe := testNode("healthy-probe", "hk", "8.8.4.4", 10, 1)
	healthyProbe.DNSProbeHealthy = true
	snapshot.Nodes = []SnapshotNode{unhealthyProbe, healthyProbe}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected node with healthy DNS probe, got %s", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingDisabledKeepsExistingBehavior(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = false
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           21,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	unhealthyProbe := testNode("unhealthy-probe", "hk", "1.1.1.1", 1000, 1)
	unhealthyProbe.DNSProbeHealthy = false
	healthyProbe := testNode("healthy-probe", "hk", "8.8.4.4", 10, 1)
	healthyProbe.DNSProbeHealthy = true
	snapshot.Nodes = []SnapshotNode{unhealthyProbe, healthyProbe}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "1.1.1.1" {
		t.Fatalf("expected weighted scheduling to ignore DNS probe when disabled, got %s", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingPrefersLowerRTT(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = true
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           23,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	now := time.Now().UTC()
	slowProbe := testNode("slow-probe", "hk", "1.1.1.1", 100, 1)
	slowProbe.LastSeenAt = now
	slowProbe.DNSProbeHealthy = true
	slowProbe.DNSProbeAverageRTTMs = 80
	fastProbe := testNode("fast-probe", "hk", "8.8.4.4", 100, 1)
	fastProbe.LastSeenAt = now.Add(-10 * time.Second)
	fastProbe.DNSProbeHealthy = true
	fastProbe.DNSProbeAverageRTTMs = 12
	snapshot.Nodes = []SnapshotNode{slowProbe, fastProbe}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected lower Agent DNS probe RTT target, got %s", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingScoresProbeQuality(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = true
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           24,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	now := time.Now().UTC()
	weakProbe := testNode("weak-probe", "hk", "1.1.1.1", 100, 1)
	weakProbe.LastSeenAt = now
	weakProbe.DNSProbeHealthy = true
	weakProbe.DNSProbeCheckedCount = 4
	weakProbe.DNSProbeHealthyCount = 1
	weakProbe.DNSProbeStaleCount = 2
	weakProbe.DNSProbeAverageRTTMs = 900
	strongProbe := testNode("strong-probe", "hk", "8.8.4.4", 100, 1)
	strongProbe.LastSeenAt = now.Add(-10 * time.Second)
	strongProbe.DNSProbeHealthy = true
	strongProbe.DNSProbeCheckedCount = 4
	strongProbe.DNSProbeHealthyCount = 4
	strongProbe.DNSProbeAverageRTTMs = 20
	snapshot.Nodes = []SnapshotNode{weakProbe, strongProbe}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected higher Agent probe quality target, got %s", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingLoadAwareCombinesLoadAndProbeQuality(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = true
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           25,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "load_aware",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "load_aware",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	lowLoadPoorProbe := testNode("low-load-poor-probe", "hk", "1.1.1.1", 100, 0)
	lowLoadPoorProbe.DNSProbeHealthy = true
	lowLoadPoorProbe.DNSProbeCheckedCount = 4
	lowLoadPoorProbe.DNSProbeHealthyCount = 1
	lowLoadPoorProbe.DNSProbeAverageRTTMs = 900
	higherLoadGoodProbe := testNode("higher-load-good-probe", "hk", "8.8.4.4", 100, 20)
	higherLoadGoodProbe.DNSProbeHealthy = true
	higherLoadGoodProbe.DNSProbeCheckedCount = 4
	higherLoadGoodProbe.DNSProbeHealthyCount = 4
	higherLoadGoodProbe.DNSProbeAverageRTTMs = 10
	snapshot.Nodes = []SnapshotNode{lowLoadPoorProbe, higherLoadGoodProbe}
	server := testServer(t, snapshot)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected dynamic A answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected load-aware score to include Agent probe quality, got %s", got)
	}
}

func TestSpreadCandidateWeightUsesProbeQualityScoreForWeighted(t *testing.T) {
	candidate := targetCandidate{
		NodeWeight: 100,
		Score:      25,
	}
	if got := spreadCandidateWeight(candidate, "weighted"); got != 25 {
		t.Fatalf("expected weighted source spread to use score with probe quality, got %v", got)
	}
	if got := spreadCandidateWeight(candidate, "load_aware"); got != 25 {
		t.Fatalf("expected load-aware source spread to use score with probe quality, got %v", got)
	}
	if got := spreadCandidateWeight(candidate, "healthy"); got != 100 {
		t.Fatalf("expected non-weighted source spread fallback to node weight, got %v", got)
	}
}

func TestResolveDynamicRouteProbeSchedulingExplainsFilteredCandidates(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GSLBProbeSchedulingEnabled = true
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           22,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
			},
		},
	}
	unhealthyProbe := testNode("unhealthy-probe", "hk", "1.1.1.1", 100, 1)
	unhealthyProbe.DNSProbeHealthy = false
	snapshot.Nodes = []SnapshotNode{unhealthyProbe}
	scheduler := NewScheduler()

	_, _, _, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", SourceContext{}, true)
	if err == nil || !strings.Contains(err.Error(), "Agent DNS Worker probe threshold") {
		t.Fatalf("expected probe threshold error, got %v", err)
	}
}

func TestResolveGSLBMatchesECSCountryPools(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           8,
			Domains:      []string{"cdn.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Countries: []string{"HK"}, Enabled: true},
					{Name: "eu", Weight: 100, Countries: []string{"DE"}, Enabled: true},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("hk-node", "hk", "8.8.4.4", 100, 1),
		testNode("eu-node", "eu", "9.9.9.9", 100, 1),
	}
	server := testServer(t, snapshot)
	server.Scheduler = NewScheduler()
	server.Scheduler.now = func() time.Time { return time.Unix(100, 0) }

	query := testQuery("cdn.example.com", dns.TypeA, "203.0.113.0")
	response := server.Resolve(query, &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 53000})
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected country route answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected HK target for test ECS resolver fallback, got %s", got)
	}
}

func TestResolveGSLBMatchesSourceCIDRBeforeCountry(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           18,
			Domains:      []string{"cdn.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Countries: []string{"HK"}, SourceCIDRs: []string{"198.51.100.0/24"}, Enabled: true},
					{Name: "eu", Weight: 100, Countries: []string{"DE"}, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("hk-node", "hk", "8.8.4.4", 100, 1),
		testNode("eu-node", "eu", "9.9.9.9", 100, 1),
	}
	server := testServer(t, snapshot)
	server.Scheduler = NewScheduler()
	server.Scheduler.now = func() time.Time { return time.Unix(100, 0) }

	response := server.Resolve(testQuery("cdn.example.com", dns.TypeA, "203.0.113.10"), &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 53000})
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected CIDR route answer, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "9.9.9.9" {
		t.Fatalf("expected EU target for source CIDR, got %s", got)
	}
	states := server.Scheduler.SnapshotStates(snapshot)
	if len(states) != 1 || !strings.HasPrefix(states[0].ScopeKey, "cidr:203.0.113.0/24|bucket:") {
		t.Fatalf("expected CIDR scoped debounce state, got %+v", states)
	}
}

func TestSchedulerWeightedSpreadUsesSourceBuckets(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           28,
			Domains:      []string{"cdn.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 80, Enabled: true},
					{Name: "eu", Weight: 20, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 600},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("hk-node", "hk", "8.8.4.4", 100, 1),
		testNode("eu-node", "eu", "9.9.9.9", 100, 1),
	}
	policy := normalizePolicy(snapshot.Routes[0].GSLBPolicy, snapshot.Routes[0])
	hkSource, euSource := findWeightedSpreadSources(t, policy, snapshot.Routes[0].ID, "A")
	scheduler := NewScheduler()
	scheduler.now = func() time.Time { return time.Unix(100, 0) }

	hkTargets, _, hkScope, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", hkSource, true)
	if err != nil {
		t.Fatalf("select HK bucket: %v", err)
	}
	euTargets, _, euScope, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", euSource, true)
	if err != nil {
		t.Fatalf("select EU bucket: %v", err)
	}
	if len(hkTargets) != 1 || hkTargets[0] != "8.8.4.4" {
		t.Fatalf("expected HK bucket target, got targets=%v source=%+v scope=%s", hkTargets, hkSource, hkScope)
	}
	if len(euTargets) != 1 || euTargets[0] != "9.9.9.9" {
		t.Fatalf("expected EU bucket target, got targets=%v source=%+v scope=%s", euTargets, euSource, euScope)
	}
	if hkScope == euScope || !strings.Contains(hkScope, "|bucket:") || !strings.Contains(euScope, "|bucket:") {
		t.Fatalf("expected distinct bucket scopes, hk=%s eu=%s", hkScope, euScope)
	}
	states := scheduler.SnapshotStates(snapshot)
	if len(states) != 2 {
		t.Fatalf("expected two bucket-scoped states, got %+v", states)
	}
	seenScopes := map[string]struct{}{}
	for _, state := range states {
		seenScopes[state.ScopeKey] = struct{}{}
	}
	if _, ok := seenScopes[hkScope]; !ok {
		t.Fatalf("missing HK bucket state %s in %+v", hkScope, states)
	}
	if _, ok := seenScopes[euScope]; !ok {
		t.Fatalf("missing EU bucket state %s in %+v", euScope, states)
	}
}

func TestSchedulerWeightedSpreadReturnsAvailableTargetsUpToTargetCount(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           29,
			Domains:      []string{"cdn.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  20,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 20,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "jp", Weight: 1, Enabled: true},
					{Name: "hk", Weight: 99, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 600},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("jp-node", "jp", "8.8.4.4", 100, 1),
		testNode("hk-node", "hk", "1.1.1.1", 100, 1),
	}
	scheduler := NewScheduler()
	scheduler.now = func() time.Time { return time.Unix(100, 0) }

	targets, _, scope, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", SourceContext{IP: "175.8.66.28"}, true)
	if err != nil {
		t.Fatalf("select weighted targets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected both available targets when target count exceeds candidates, got targets=%v scope=%s", targets, scope)
	}
	targetSet := map[string]struct{}{}
	for _, target := range targets {
		targetSet[target] = struct{}{}
	}
	if _, ok := targetSet["8.8.4.4"]; !ok {
		t.Fatalf("missing jp target, got %v", targets)
	}
	if _, ok := targetSet["1.1.1.1"]; !ok {
		t.Fatalf("missing hk target, got %v", targets)
	}
}

func TestSchedulerRestoresDebounceFromSnapshotState(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           10,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 60},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("previous", "hk", "8.8.4.4", 100, 1),
		testNode("desired", "hk", "1.1.1.1", 900, 1),
	}
	lastChangedAt := now.Add(-10 * time.Second)
	snapshot.SchedulingStates = []SnapshotSchedulingState{
		{
			RouteID:         10,
			RecordType:      "A",
			ScopeKey:        "global",
			SelectedTargets: []string{"8.8.4.4"},
			DesiredTargets:  []string{"8.8.4.4"},
			LastChangedAt:   &lastChangedAt,
		},
	}
	store := NewSnapshotStore("", DefaultSnapshotMaxAge)
	if err := store.Set(snapshot); err != nil {
		t.Fatalf("set snapshot: %v", err)
	}
	loaded, _, _, _ := store.Current()
	scheduler := NewScheduler()
	scheduler.now = func() time.Time { return now }
	scheduler.LoadSnapshotStates(loaded)

	selected, _, _, err := scheduler.Select(loaded, &loaded.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(selected) != 1 || selected[0] != "8.8.4.4" {
		t.Fatalf("expected restored debounce state to keep previous target, got %+v", selected)
	}
}

func TestSchedulerKeepsLocalStateForRoutesPresentInNewSnapshot(t *testing.T) {
	now := time.Unix(300, 0).UTC()
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           11,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 60},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("previous", "hk", "8.8.4.4", 100, 1),
		testNode("desired", "hk", "1.1.1.1", 900, 1),
	}
	scheduler := NewScheduler()
	scheduler.now = func() time.Time { return now }
	scheduler.states[schedulerStateKey(11, "A", "global")] = debounceState{
		Targets:       []string{"8.8.4.4"},
		Desired:       []string{"8.8.4.4"},
		LastChangedAt: now.Add(-10 * time.Second),
	}

	scheduler.LoadSnapshotStates(snapshot)
	selected, _, _, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(selected) != 1 || selected[0] != "8.8.4.4" {
		t.Fatalf("expected local state to survive snapshot refresh, got %+v", selected)
	}
}

func TestSchedulerClampsFutureLoadedSnapshotState(t *testing.T) {
	now := time.Unix(350, 0).UTC()
	generatedAt := now.Add(-20 * time.Second)
	futureChangedAt := now.Add(time.Hour)
	snapshot := baseSnapshot()
	snapshot.GeneratedAt = generatedAt
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           12,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 60},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode("previous", "hk", "8.8.4.4", 100, 1),
		testNode("desired", "hk", "1.1.1.1", 900, 1),
	}
	snapshot.SchedulingStates = []SnapshotSchedulingState{
		{
			RouteID:         12,
			RecordType:      "A",
			ScopeKey:        "global",
			SelectedTargets: []string{"8.8.4.4"},
			DesiredTargets:  []string{"8.8.4.4"},
			LastChangedAt:   &futureChangedAt,
		},
	}

	scheduler := NewScheduler()
	scheduler.now = func() time.Time { return now }
	scheduler.LoadSnapshotStates(snapshot)
	selected, _, _, err := scheduler.Select(snapshot, &snapshot.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(selected) != 1 || selected[0] != "8.8.4.4" {
		t.Fatalf("expected clamped snapshot state to keep previous target, got %+v", selected)
	}
	states := scheduler.SnapshotStates(snapshot)
	if len(states) != 1 || states[0].LastChangedAt == nil || !states[0].LastChangedAt.Equal(generatedAt) {
		t.Fatalf("expected exported state time to use snapshot generated time, got %+v", states)
	}
}

func TestSchedulerExportsSnapshotStates(t *testing.T) {
	now := time.Unix(400, 0).UTC()
	snapshot := baseSnapshot()
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           12,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     "hk",
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: "hk", Weight: 100, Enabled: true},
				},
				Debounce: GSLBDebounce{CooldownSeconds: 60},
			},
		},
	}
	scheduler := NewScheduler()
	scheduler.states[schedulerStateKey(12, "A", "country:hk")] = debounceState{
		Targets:       []string{"8.8.4.4"},
		Desired:       []string{"1.1.1.1"},
		LastChangedAt: now,
	}
	scheduler.states[schedulerStateKey(404, "A", "global")] = debounceState{
		Targets:       []string{"9.9.9.9"},
		Desired:       []string{"9.9.9.9"},
		LastChangedAt: now,
	}

	states := scheduler.SnapshotStates(snapshot)
	if len(states) != 1 {
		t.Fatalf("expected one exported scheduling state, got %+v", states)
	}
	state := states[0]
	if state.RouteID != 12 ||
		state.RecordType != "A" ||
		state.ScopeKey != "country:HK" ||
		len(state.SelectedTargets) != 1 ||
		state.SelectedTargets[0] != "8.8.4.4" ||
		len(state.DesiredTargets) != 1 ||
		state.DesiredTargets[0] != "1.1.1.1" ||
		state.LastChangedAt == nil ||
		!state.LastChangedAt.Equal(now) {
		t.Fatalf("unexpected exported scheduling state: %+v", state)
	}
}

func TestResolveStaleSnapshotServfailForDynamicButKeepsStatic(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GeneratedAt = time.Now().Add(-10 * time.Minute)
	snapshot.Routes = []SnapshotRoute{
		{
			ID:             9,
			Domains:        []string{"edge.example.com"},
			ZoneID:         1,
			NodePool:       "hk",
			RecordType:     "A",
			TargetCount:    1,
			TTL:            30,
			CurrentTargets: []string{"8.8.4.4"},
		},
	}
	server := testServerWithMaxAge(t, snapshot, time.Minute)

	response := server.Resolve(testQuery("edge.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected stale dynamic SERVFAIL, got %s", dns.RcodeToString[response.Rcode])
	}

	response = server.Resolve(testQuery("www.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected static answer from stale snapshot, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
}

func TestRejectsZoneTransferAndRollupDrain(t *testing.T) {
	server := testServer(t, baseSnapshot())

	response := server.Resolve(testQuery("example.com", dns.TypeAXFR, ""), nil)
	if response.Rcode != dns.RcodeRefused {
		t.Fatalf("expected refused AXFR, got %s", dns.RcodeToString[response.Rcode])
	}
	rollups := server.Rollups.Drain()
	if len(rollups) != 1 || rollups[0].RCode != "REFUSED" || rollups[0].QueryCount != 1 {
		t.Fatalf("unexpected rollups: %+v", rollups)
	}
	if rollups[0].TotalDurationMs < 0 || rollups[0].MaxDurationMs < 0 {
		t.Fatalf("unexpected negative duration rollup: %+v", rollups[0])
	}
}

func TestRateLimitsQueriesPerSourceIP(t *testing.T) {
	server := testServerWithLimits(t, baseSnapshot(), DefaultSnapshotMaxAge, 2, DefaultUDPResponseSize)
	remote := &net.UDPAddr{IP: net.ParseIP("192.0.2.55"), Port: 53000}

	for i := 0; i < 2; i++ {
		response := server.Resolve(testQuery("www.example.com", dns.TypeA, ""), remote)
		if response.Rcode != dns.RcodeSuccess {
			t.Fatalf("expected allowed query %d, got %s", i+1, dns.RcodeToString[response.Rcode])
		}
	}
	response := server.Resolve(testQuery("www.example.com", dns.TypeA, ""), remote)
	if response.Rcode != dns.RcodeRefused {
		t.Fatalf("expected rate limited REFUSED, got %s", dns.RcodeToString[response.Rcode])
	}

	rollups := server.Rollups.Drain()
	var refused int64
	for _, rollup := range rollups {
		if rollup.RCode == "REFUSED" {
			refused += rollup.QueryCount
		}
	}
	if refused != 1 {
		t.Fatalf("expected one refused rollup, got %+v", rollups)
	}
}

func TestUDPResponseTruncationUsesEDNSAndServerLimit(t *testing.T) {
	server := testServerWithLimits(t, largeTXTRecordSnapshot(), DefaultSnapshotMaxAge, 0, 700)
	query := testQuery("large.example.com", dns.TypeTXT, "")
	query.SetEdns0(4096, false)

	response := server.Resolve(query, &net.UDPAddr{IP: net.ParseIP("192.0.2.56"), Port: 53000})
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) < 2 {
		t.Fatalf("expected large TXT response before truncation, rcode=%s answers=%d", dns.RcodeToString[response.Rcode], len(response.Answer))
	}

	truncated := server.truncateUDPResponse(query, response)
	if !truncated.Truncated {
		t.Fatal("expected UDP response to be truncated")
	}
	if truncated.Len() > 700 {
		t.Fatalf("expected truncated response to fit server limit, got %d", truncated.Len())
	}
	if len(truncated.Answer) >= len(response.Answer) {
		t.Fatalf("expected answer set to shrink, before=%d after=%d", len(response.Answer), len(truncated.Answer))
	}
}

func testServer(t *testing.T, snapshot *Snapshot) *DNSServer {
	t.Helper()
	return testServerWithMaxAge(t, snapshot, DefaultSnapshotMaxAge)
}

func testServerWithMaxAge(t *testing.T, snapshot *Snapshot, maxAge time.Duration) *DNSServer {
	t.Helper()
	return testServerWithLimits(t, snapshot, maxAge, DefaultQueryRateLimit, DefaultUDPResponseSize)
}

func testServerWithLimits(t *testing.T, snapshot *Snapshot, maxAge time.Duration, queryRateLimit int, udpSize int) *DNSServer {
	t.Helper()
	store := NewSnapshotStore("", maxAge)
	if err := store.Set(snapshot); err != nil {
		t.Fatalf("set snapshot: %v", err)
	}
	return NewDNSServerWithLimits(store, NewScheduler(), NewRollupAggregator(time.Minute), &testSourceResolver{}, ":0", queryRateLimit, udpSize)
}

type testSourceResolver struct {
	SourceResolver
}

func (r *testSourceResolver) Resolve(request *dns.Msg, remoteAddr net.Addr) SourceContext {
	source := r.SourceResolver.Resolve(request, remoteAddr)
	if source.IP == "203.0.113.0" {
		source.Country = "HK"
	}
	source.ScopeKey = sourceScopeKey(source)
	return source
}

func baseSnapshot() *Snapshot {
	now := time.Now().UTC()
	return &Snapshot{
		SnapshotVersion: "test",
		GeneratedAt:     now,
		Zones: []SnapshotZone{
			{
				ID:          1,
				Name:        "example.com",
				SOAEmail:    "hostmaster@example.com",
				PrimaryNS:   "ns1.example.com",
				NameServers: []string{"ns1.example.com", "ns2.example.com"},
				DefaultTTL:  120,
				Serial:      123,
				Records: []SnapshotRecord{
					{ID: 1, Name: "www.example.com", Type: "A", Value: "8.8.8.8", TTL: 120},
					{ID: 2, Name: "mail.example.com", Type: "MX", Value: "mx.example.com", TTL: 120, Priority: 10},
				},
			},
		},
	}
}

func largeTXTRecordSnapshot() *Snapshot {
	snapshot := baseSnapshot()
	records := make([]SnapshotRecord, 0, 20)
	value := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < 20; i++ {
		records = append(records, SnapshotRecord{
			ID:    uint(i + 10),
			Name:  "large.example.com",
			Type:  "TXT",
			Value: value,
			TTL:   120,
		})
	}
	snapshot.Zones[0].Records = append(snapshot.Zones[0].Records, records...)
	return snapshot
}

func testNode(id string, pool string, ip string, weight int, connections int64) SnapshotNode {
	now := time.Now().UTC()
	return SnapshotNode{
		NodeID:               id,
		Name:                 id,
		PoolName:             pool,
		PublicIPs:            []string{ip},
		Weight:               weight,
		SchedulingEnabled:    true,
		Status:               "online",
		OpenrestyStatus:      "healthy",
		LastSeenAt:           now,
		OpenrestyConnections: connections,
		MetricCapturedAt:     &now,
	}
}

func findWeightedSpreadSources(t *testing.T, policy GSLBPolicy, routeID uint, recordType string) (SourceContext, SourceContext) {
	t.Helper()
	hkSource := SourceContext{}
	euSource := SourceContext{}
	for i := 1; i < 255; i++ {
		source := SourceContext{IP: net.IPv4(198, 51, 100, byte(i)).String()}
		scopeKey := sourceScopeKeyForPolicy(policy, source)
		spread := sourceSpreadForPolicy(policy, routeID, recordType, source, scopeKey)
		if spread == nil {
			t.Fatal("expected source spread")
		}
		pool := selectPoolByBucket([]weightedAvailablePool{
			{Name: "hk", Weight: 80},
			{Name: "eu", Weight: 20},
		}, spread.Bucket)
		switch pool {
		case "hk":
			if hkSource.IP == "" {
				hkSource = source
			}
		case "eu":
			if euSource.IP == "" {
				euSource = source
			}
		}
		if hkSource.IP != "" && euSource.IP != "" {
			return hkSource, euSource
		}
	}
	t.Fatalf("could not find both spread sources, hk=%+v eu=%+v", hkSource, euSource)
	return SourceContext{}, SourceContext{}
}

func testQuery(name string, qtype uint16, ecs string) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	if ecs != "" {
		msg.SetEdns0(1232, false)
		opt := msg.IsEdns0()
		opt.Option = append(opt.Option, &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        1,
			SourceNetmask: 24,
			SourceScope:   0,
			Address:       net.ParseIP(ecs),
		})
	}
	return msg
}
