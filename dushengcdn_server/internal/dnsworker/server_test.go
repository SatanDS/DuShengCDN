package dnsworker

import (
	"net"
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

func testServer(t *testing.T, snapshot *Snapshot) *DNSServer {
	t.Helper()
	return testServerWithMaxAge(t, snapshot, DefaultSnapshotMaxAge)
}

func testServerWithMaxAge(t *testing.T, snapshot *Snapshot, maxAge time.Duration) *DNSServer {
	t.Helper()
	store := NewSnapshotStore("", maxAge)
	if err := store.Set(snapshot); err != nil {
		t.Fatalf("set snapshot: %v", err)
	}
	return NewDNSServer(store, NewScheduler(), NewRollupAggregator(time.Minute), &testSourceResolver{}, ":0")
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
