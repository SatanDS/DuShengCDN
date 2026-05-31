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
