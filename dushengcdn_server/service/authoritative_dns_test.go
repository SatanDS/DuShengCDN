package service

import (
	"encoding/json"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
)

func TestAuthoritativeDNSZoneRecordWorkerAndSnapshot(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{
		Name:        "Example.COM.",
		PrimaryNS:   "NS1.Example.COM.",
		NameServers: []string{"ns1.example.com", "ns2.example.com"},
		DefaultTTL:  120,
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if zone.Name != "example.com" || zone.PrimaryNS != "ns1.example.com" || zone.RecordCount != 0 {
		t.Fatalf("unexpected zone view: %+v", zone)
	}

	record, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{
		Name:  "www",
		Type:  "A",
		Value: "8.8.8.8",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	if record.Name != "www.example.com" || record.TTL != 120 {
		t.Fatalf("unexpected record view: %+v", record)
	}

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1",
		PublicAddress: "203.0.113.10:53",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if worker.Token == "" {
		t.Fatal("expected created worker view to include token")
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}

	now := time.Now()
	if err := (&model.Node{
		NodeID:          "node-hk",
		Name:            "hk",
		IP:              "8.8.4.4",
		PoolName:        "hk",
		PublicIPs:       `["8.8.4.4"]`,
		Weight:          100,
		AgentToken:      "token-hk",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "weighted", 30)
	rawPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		DNSAutoSync:     true,
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          1,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(authenticated)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if snapshot.SnapshotVersion == "" {
		t.Fatal("expected snapshot version")
	}
	if len(snapshot.Zones) != 1 || len(snapshot.Zones[0].Records) != 1 {
		t.Fatalf("unexpected snapshot zones: %+v", snapshot.Zones)
	}
	if len(snapshot.Routes) != 1 {
		t.Fatalf("unexpected snapshot routes: %+v", snapshot.Routes)
	}
	if snapshot.Routes[0].CurrentTargets[0] != "8.8.4.4" {
		t.Fatalf("unexpected route targets: %+v", snapshot.Routes[0].CurrentTargets)
	}
	if snapshot.Routes[0].TTL != defaultAuthoritativeTTL {
		t.Fatalf("expected authoritative auto ttl, got %d", snapshot.Routes[0].TTL)
	}
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].PublicIPs[0] != "8.8.4.4" {
		t.Fatalf("unexpected snapshot nodes: %+v", snapshot.Nodes)
	}

	var reloadedWorker model.DNSWorker
	if err := model.DB.First(&reloadedWorker, authenticated.ID).Error; err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if reloadedWorker.LastSnapshotVersion != snapshot.SnapshotVersion || reloadedWorker.LastSnapshotAt == nil {
		t.Fatalf("expected worker snapshot metadata to be updated, got %+v", reloadedWorker)
	}
}

func TestAuthoritativeDNSProxyRouteRequiresZoneMatch(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "other.test",
		OriginURL:       "https://origin.internal",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
	})
	if err == nil {
		t.Fatal("expected authoritative route outside zone to fail")
	}
}

func TestDNSWorkerHeartbeatPersistsRollupsWithoutTokenLeak(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeatAt := time.Now().UTC().Truncate(time.Minute)
	view, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:             "v1.0.0",
		Status:              "online",
		LastSnapshotVersion: "abc123",
		LastSnapshotAt:      &heartbeatAt,
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:   heartbeatAt,
				WindowMinutes: 5,
				QName:         "www.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    42,
				TargetSummary: map[string]int64{"8.8.8.8": 42},
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if view.Token != "" {
		t.Fatal("expected heartbeat worker view to omit token")
	}
	if view.Status != dnsWorkerStatusOnline || view.Version != "v1.0.0" {
		t.Fatalf("unexpected heartbeat view: %+v", view)
	}
	var count int64
	if err := model.DB.Model(&model.DNSQueryRollup{}).Where("worker_id = ?", authenticated.WorkerID).Count(&count).Error; err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one rollup, got %d", count)
	}
}

func TestAuthoritativeDNSObservabilitySummaryAggregatesRollups(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1-hk"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	windowStart := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zone.ID,
				ProxyRouteID:  route.ID,
				QName:         "www.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    80,
				TargetSummary: map[string]int64{"8.8.8.8": 64, "1.1.1.1": 16},
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zone.ID,
				QName:         "missing.example.com",
				QType:         "A",
				RCode:         "NXDOMAIN",
				QueryCount:    5,
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zone.ID,
				ProxyRouteID:  route.ID,
				QName:         "www.example.com",
				QType:         "A",
				RCode:         "SERVFAIL",
				QueryCount:    2,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.TotalQueries != 87 || summary.SuccessfulQueries != 80 || summary.NegativeQueries != 5 || summary.ErrorQueries != 2 {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if summary.DynamicQueries != 82 || summary.StaticQueries != 5 {
		t.Fatalf("unexpected dynamic/static totals: %+v", summary)
	}
	assertCounter(t, summary.RCodeBreakdown, "NOERROR", "NOERROR", 80)
	assertCounter(t, summary.RCodeBreakdown, "NXDOMAIN", "NXDOMAIN", 5)
	assertCounter(t, summary.RCodeBreakdown, "SERVFAIL", "SERVFAIL", 2)
	assertCounter(t, summary.TopTargets, "8.8.8.8", "8.8.8.8", 64)
	assertCounter(t, summary.TopTargets, "1.1.1.1", "1.1.1.1", 16)
	assertCounter(t, summary.WorkerBreakdown, authenticated.WorkerID, "ns1-hk", 87)
	assertCounter(t, summary.ZoneBreakdown, "1", "example.com", 87)
	assertCounter(t, summary.RouteBreakdown, "1", "edge-site", 82)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func assertCounter(t *testing.T, counters []DNSObservabilityCounterView, key string, label string, count int64) {
	t.Helper()
	for _, counter := range counters {
		if counter.Key == key {
			if counter.Label != label || counter.Count != count {
				t.Fatalf("unexpected counter for %s: %+v", key, counter)
			}
			return
		}
	}
	t.Fatalf("missing counter %s in %+v", key, counters)
}
