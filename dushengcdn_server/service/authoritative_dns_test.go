package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"

	"github.com/miekg/dns"
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
	lastChangedAt := now.Add(-10 * time.Second).UTC()
	if err := model.DB.Create(&model.GSLBSchedulingState{
		ProxyRouteID:    route.ID,
		DNSRecordType:   "A",
		ScopeKey:        "global",
		SelectedTargets: `["8.8.4.4"]`,
		DesiredTargets:  `["8.8.4.4"]`,
		LastChangedAt:   &lastChangedAt,
		LastEvaluatedAt: &lastChangedAt,
	}).Error; err != nil {
		t.Fatalf("insert gslb state: %v", err)
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
	if snapshot.Routes[0].TTL != authoritativeDNSDefaultTTL() {
		t.Fatalf("expected authoritative auto ttl, got %d", snapshot.Routes[0].TTL)
	}
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].PublicIPs[0] != "8.8.4.4" {
		t.Fatalf("unexpected snapshot nodes: %+v", snapshot.Nodes)
	}
	if len(snapshot.SchedulingStates) != 1 {
		t.Fatalf("expected one scheduling state in snapshot, got %+v", snapshot.SchedulingStates)
	}
	state := snapshot.SchedulingStates[0]
	if state.RouteID != route.ID ||
		state.RecordType != "A" ||
		state.ScopeKey != "global" ||
		len(state.SelectedTargets) != 1 ||
		state.SelectedTargets[0] != "8.8.4.4" ||
		state.LastChangedAt == nil ||
		!state.LastChangedAt.Equal(lastChangedAt) {
		t.Fatalf("unexpected snapshot scheduling state: %+v", state)
	}
	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	if len(workerSnapshot.SchedulingStates) != 1 ||
		workerSnapshot.SchedulingStates[0].RouteID != route.ID ||
		workerSnapshot.SchedulingStates[0].SelectedTargets[0] != "8.8.4.4" {
		t.Fatalf("unexpected worker scheduling states: %+v", workerSnapshot.SchedulingStates)
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

func TestSwitchProxyRouteToAuthoritativeDNSRequiresReadyWorker(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:           "edge-site",
		Domain:             "www.example.com",
		Domains:            `["www.example.com"]`,
		OriginURL:          "https://origin.internal",
		Upstreams:          `["https://origin.internal"]`,
		NodePool:           "hk",
		DNSAutoSync:        true,
		DNSAccountID:       ptrUint(42),
		DNSZoneID:          "cloudflare-zone",
		DNSRecordType:      "A",
		DNSRecordContent:   "203.0.113.10",
		DNSRecordIDs:       `{"www.example.com|203.0.113.10":"record-id"}`,
		DNSTargetCount:     2,
		DNSScheduleMode:    "weighted",
		DNSTTL:             120,
		DNSProviderMode:    DNSProviderModeCloudflare,
		CloudflareProxied:  true,
		DDOSProtectionMode: DDOSProtectionModeAuto,
		GSLBEnabled:        true,
		GSLBPolicy:         mustJSON(t, defaultGSLBPolicy("hk", 2, "weighted", 120)),
		Enabled:            true,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	if _, err := SwitchProxyRouteToAuthoritativeDNS(route.ID, AuthoritativeDNSMigrationInput{DNSZoneIDRef: &zone.ID}); err == nil {
		t.Fatal("expected switch without ready DNS Worker to fail")
	}

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1", PublicAddress: "ns1.example.net"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	checkedAt := time.Now().UTC()
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	workerModel.Status = dnsWorkerStatusOnline
	workerModel.LastSeenAt = &checkedAt
	workerModel.LastProbeAt = &checkedAt
	workerModel.LastProbeQuery = "example.com. SOA"
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	view, err := SwitchProxyRouteToAuthoritativeDNS(route.ID, AuthoritativeDNSMigrationInput{DNSZoneIDRef: &zone.ID})
	if err != nil {
		t.Fatalf("SwitchProxyRouteToAuthoritativeDNS: %v", err)
	}
	if view.DNSProviderMode != DNSProviderModeAuthoritative || view.DNSZoneIDRef == nil || *view.DNSZoneIDRef != zone.ID {
		t.Fatalf("expected authoritative DNS mode after switch: %+v", view)
	}
	if view.DNSAutoSync || view.DNSAccountID != nil || view.CloudflareProxied || view.DDOSProtectionMode != DDOSProtectionModeOff {
		t.Fatalf("expected Cloudflare-only DNS settings to be disabled: %+v", view)
	}
	if !view.DNSAutoTarget || view.DNSRecordContent != "" || view.DNSZoneID != "" || view.DNSRecordType != "A" {
		t.Fatalf("unexpected DNS target settings after switch: %+v", view)
	}
	if len(view.DNSRecordIDs) != 0 {
		t.Fatalf("expected Cloudflare record IDs to be cleared: %+v", view.DNSRecordIDs)
	}
	if view.GSLBEnabled != route.GSLBEnabled || view.DNSTargetCount != 2 || view.DNSScheduleMode != "weighted" || view.DNSTTL != 120 {
		t.Fatalf("expected existing GSLB scheduling settings to be preserved: %+v", view)
	}
	if view.DNSLastSyncStatus != DNSRecordSyncStatusSuccess || !strings.Contains(view.DNSLastSyncMessage, "自建权威 DNS") || view.DNSLastSyncedAt == nil {
		t.Fatalf("expected migration status message: %+v", view)
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
		GeoIPEnabled:        true,
		GeoIPDatabasePath:   "/opt/dushengcdn-dns-worker/data/geoip/GeoLite2-Country.mmdb",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     heartbeatAt,
				WindowMinutes:   5,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      42,
				TotalDurationMs: 210,
				MaxDurationMs:   12,
				SourceScope:     "country:HK",
				TargetSummary:   map[string]int64{"8.8.8.8": 42},
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
	if !view.GeoIPEnabled || view.GeoIPDatabasePath == "" {
		t.Fatalf("expected heartbeat view to include geoip status: %+v", view)
	}
	var count int64
	if err := model.DB.Model(&model.DNSQueryRollup{}).Where("worker_id = ?", authenticated.WorkerID).Count(&count).Error; err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one rollup, got %d", count)
	}
	var rollup model.DNSQueryRollup
	if err := model.DB.Where("worker_id = ?", authenticated.WorkerID).First(&rollup).Error; err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if rollup.TotalDurationMs != 210 || rollup.MaxDurationMs != 12 {
		t.Fatalf("unexpected rollup duration: %+v", rollup)
	}
	if rollup.SourceScope != "country:HK" {
		t.Fatalf("unexpected rollup source scope: %+v", rollup)
	}
}

func TestDNSWorkerHeartbeatPersistsSchedulingStates(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
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
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	changedAt := time.Now().UTC().Add(-20 * time.Second).Truncate(time.Second)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         route.ID,
				RecordType:      "A",
				ScopeKey:        "country:hk",
				SelectedTargets: []string{"8.8.4.4"},
				DesiredTargets:  []string{"1.1.1.1"},
				LastChangedAt:   &changedAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	var state model.GSLBSchedulingState
	if err := model.DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", route.ID, "A", "country:HK").First(&state).Error; err != nil {
		t.Fatalf("load scheduling state: %v", err)
	}
	if state.SelectedTargets != `["8.8.4.4"]` || state.DesiredTargets != `["1.1.1.1"]` || state.LastChangedAt == nil || !state.LastChangedAt.Equal(changedAt) {
		t.Fatalf("unexpected scheduling state: %+v", state)
	}

	olderChangedAt := changedAt.Add(-time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         route.ID,
				RecordType:      "A",
				ScopeKey:        "country:HK",
				SelectedTargets: []string{"9.9.9.9"},
				DesiredTargets:  []string{"9.9.9.9"},
				LastChangedAt:   &olderChangedAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat old state: %v", err)
	}
	if err := model.DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", route.ID, "A", "country:HK").First(&state).Error; err != nil {
		t.Fatalf("reload scheduling state: %v", err)
	}
	if state.SelectedTargets != `["8.8.4.4"]` {
		t.Fatalf("expected older heartbeat not to overwrite state, got %+v", state)
	}
}

func TestListAuthoritativeDNSGSLBSchedulingStates(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com","api.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	changedAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	evaluatedAt := time.Now().UTC().Truncate(time.Second)
	if err := model.DB.Create(&model.GSLBSchedulingState{
		ProxyRouteID:    route.ID,
		DNSRecordType:   "A",
		ScopeKey:        "country:hk",
		SelectedTargets: `["8.8.4.4"]`,
		DesiredTargets:  `["1.1.1.1"]`,
		LastReason:      "cooldown keeps previous target",
		LastChangedAt:   &changedAt,
		LastEvaluatedAt: &evaluatedAt,
	}).Error; err != nil {
		t.Fatalf("insert scheduling state: %v", err)
	}

	view, err := ListAuthoritativeDNSGSLBSchedulingStates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSGSLBSchedulingStates: %v", err)
	}
	if view.Total != 1 || len(view.States) != 1 {
		t.Fatalf("expected one scheduling state, got %+v", view)
	}
	state := view.States[0]
	if state.ProxyRouteID != route.ID ||
		state.SiteName != "edge-site" ||
		state.PrimaryDomain != "www.example.com" ||
		state.ScopeKey != "country:HK" ||
		state.Status != "debouncing" ||
		state.LastReason != "cooldown keeps previous target" ||
		len(state.Domains) != 2 ||
		len(state.SelectedTargets) != 1 ||
		state.SelectedTargets[0] != "8.8.4.4" ||
		len(state.DesiredTargets) != 1 ||
		state.DesiredTargets[0] != "1.1.1.1" {
		t.Fatalf("unexpected scheduling state view: %+v", state)
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
	peerWorker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns2-eu"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker peer: %v", err)
	}

	windowStart := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	snapshotAt := time.Now().UTC().Add(-time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:              "online",
		LastSnapshotVersion: "snapshot-a",
		LastSnapshotAt:      &snapshotAt,
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
				ProxyRouteID:    route.ID,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      80,
				TotalDurationMs: 1600,
				MaxDurationMs:   50,
				SourceScope:     "country:HK",
				TargetSummary:   map[string]int64{"8.8.8.8": 64, "1.1.1.1": 16},
			},
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
				QName:           "missing.example.com",
				QType:           "A",
				RCode:           "NXDOMAIN",
				QueryCount:      5,
				TotalDurationMs: 50,
				MaxDurationMs:   15,
				SourceScope:     "country:DE",
			},
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
				ProxyRouteID:    route.ID,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "SERVFAIL",
				QueryCount:      2,
				TotalDurationMs: 70,
				MaxDurationMs:   40,
				SourceScope:     "",
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	peerAuthenticated, err := AuthenticateDNSWorkerToken(peerWorker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken peer: %v", err)
	}
	_, err = RecordDNSWorkerHeartbeat(peerAuthenticated, DNSWorkerHeartbeatInput{
		Status:              "online",
		LastSnapshotVersion: "snapshot-b",
		LastSnapshotAt:      &snapshotAt,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat peer: %v", err)
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
	assertCounter(t, summary.SourceScopeBreakdown, "country:HK", "country:HK", 80)
	assertCounter(t, summary.SourceScopeBreakdown, "country:DE", "country:DE", 5)
	assertCounter(t, summary.SourceScopeBreakdown, "global", "global", 2)
	if len(summary.TrendPoints) != 1 {
		t.Fatalf("expected one trend point for one-hour window, got %+v", summary.TrendPoints)
	}
	trend := summary.TrendPoints[0]
	if trend.QueryCount != 87 || trend.ServfailQueries != 2 || trend.NXDomainQueries != 5 || trend.DynamicQueries != 82 {
		t.Fatalf("unexpected trend point: %+v", trend)
	}
	if summary.SnapshotConsistency.Status != dnsSnapshotDivergent {
		t.Fatalf("expected divergent snapshot status, got %+v", summary.SnapshotConsistency)
	}
	if summary.SnapshotConsistency.OnlineWorkerCount != 2 || summary.SnapshotConsistency.DivergentWorkerCount != 1 {
		t.Fatalf("unexpected snapshot counts: %+v", summary.SnapshotConsistency)
	}
	if len(summary.SnapshotConsistency.VersionBreakdown) != 2 {
		t.Fatalf("expected two snapshot versions, got %+v", summary.SnapshotConsistency.VersionBreakdown)
	}
	if summary.WorkerHealth.TotalWorkerCount != 2 || summary.WorkerHealth.OnlineWorkerCount != 2 {
		t.Fatalf("unexpected worker health counts: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.AvailabilityPercent != 100 {
		t.Fatalf("unexpected worker availability: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.MaxLatencyMs != 50 {
		t.Fatalf("unexpected worker max latency: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.AverageLatencyMs < 19.7 || summary.WorkerHealth.AverageLatencyMs > 19.8 {
		t.Fatalf("unexpected worker average latency: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.ErrorRatePercent < 2.29 || summary.WorkerHealth.ErrorRatePercent > 2.3 {
		t.Fatalf("unexpected worker error rate: %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 2 {
		t.Fatalf("expected two worker health items, got %+v", summary.WorkerHealth.Workers)
	}
	if summary.WorkerHealth.Workers[0].WorkerID != authenticated.WorkerID ||
		summary.WorkerHealth.Workers[0].QueryCount != 87 ||
		summary.WorkerHealth.Workers[0].ErrorQueries != 2 ||
		summary.WorkerHealth.Workers[0].MaxLatencyMs != 50 {
		t.Fatalf("unexpected primary worker health: %+v", summary.WorkerHealth.Workers)
	}
}

func TestAuthoritativeDNSZoneDelegationCheckMatchedWithGlueHint(t *testing.T) {
	setupServiceTestDB(t)
	restoreDNSLookupNS(t, func(name string) ([]*net.NS, error) {
		if name != "example.com" {
			t.Fatalf("unexpected lookup name: %s", name)
		}
		return []*net.NS{
			{Host: "ns2.example.com."},
			{Host: "ns1.example.com."},
		}, nil
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{
		Name:        "example.com",
		NameServers: []string{"ns1.example.com", "ns2.example.com"},
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	check, err := CheckAuthoritativeDNSZoneDelegation(zone.ID)
	if err != nil {
		t.Fatalf("CheckAuthoritativeDNSZoneDelegation: %v", err)
	}
	if check.Status != dnsDelegationMatched {
		t.Fatalf("expected matched status, got %+v", check)
	}
	if len(check.MissingNameServers) != 0 || len(check.ExtraNameServers) != 0 {
		t.Fatalf("expected no missing/extra NS, got %+v", check)
	}
	if !check.GlueRequired || len(check.GlueNameServers) != 2 {
		t.Fatalf("expected glue hint for in-zone NS, got %+v", check)
	}
}

func TestAuthoritativeDNSZoneDelegationCheckPartial(t *testing.T) {
	setupServiceTestDB(t)
	restoreDNSLookupNS(t, func(name string) ([]*net.NS, error) {
		return []*net.NS{
			{Host: "ns1.example.net."},
			{Host: "ns3.example.net."},
		}, nil
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{
		Name:        "example.com",
		NameServers: []string{"ns1.example.net", "ns2.example.net"},
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	check, err := CheckAuthoritativeDNSZoneDelegation(zone.ID)
	if err != nil {
		t.Fatalf("CheckAuthoritativeDNSZoneDelegation: %v", err)
	}
	if check.Status != dnsDelegationPartial {
		t.Fatalf("expected partial status, got %+v", check)
	}
	if len(check.MatchedNameServers) != 1 || check.MatchedNameServers[0] != "ns1.example.net" {
		t.Fatalf("unexpected matched NS: %+v", check.MatchedNameServers)
	}
	if len(check.MissingNameServers) != 1 || check.MissingNameServers[0] != "ns2.example.net" {
		t.Fatalf("unexpected missing NS: %+v", check.MissingNameServers)
	}
	if len(check.ExtraNameServers) != 1 || check.ExtraNameServers[0] != "ns3.example.net" {
		t.Fatalf("unexpected extra NS: %+v", check.ExtraNameServers)
	}
	if check.GlueRequired {
		t.Fatalf("did not expect glue hint for out-of-zone NS: %+v", check)
	}
}

func TestAuthoritativeDNSZoneDelegationCheckLookupFailureAndNotConfigured(t *testing.T) {
	setupServiceTestDB(t)
	lookupCalls := 0
	restoreDNSLookupNS(t, func(name string) ([]*net.NS, error) {
		lookupCalls++
		return nil, errors.New("lookup failed")
	})

	emptyZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "empty.example"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone empty: %v", err)
	}
	notConfigured, err := CheckAuthoritativeDNSZoneDelegation(emptyZone.ID)
	if err != nil {
		t.Fatalf("CheckAuthoritativeDNSZoneDelegation empty: %v", err)
	}
	if notConfigured.Status != dnsDelegationNotConfig {
		t.Fatalf("expected not_configured, got %+v", notConfigured)
	}
	if lookupCalls != 0 {
		t.Fatalf("expected no lookup for zone without expected NS")
	}

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{
		Name:        "example.com",
		NameServers: []string{"ns1.example.net"},
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	failed, err := CheckAuthoritativeDNSZoneDelegation(zone.ID)
	if err != nil {
		t.Fatalf("CheckAuthoritativeDNSZoneDelegation failed: %v", err)
	}
	if failed.Status != dnsDelegationFailed || failed.Error == "" {
		t.Fatalf("expected failed status with error, got %+v", failed)
	}
	if lookupCalls != 1 {
		t.Fatalf("expected one lookup, got %d", lookupCalls)
	}
}

func TestProbeAuthoritativeDNSWorkerChecksUDPAndTCP(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	var calls []string
	restoreDNSWorkerProbeExchange(t, func(ctx context.Context, target string, network string, qname string, qtype uint16, timeout time.Duration) DNSWorkerProbeResultView {
		calls = append(calls, network+"|"+target+"|"+qname)
		if target != "ns1.example.net:53" {
			t.Fatalf("unexpected probe target: %s", target)
		}
		if qname != "example.com." || qtype != dns.TypeSOA {
			t.Fatalf("unexpected probe query: %s %d", qname, qtype)
		}
		return DNSWorkerProbeResultView{
			Network:     strings.ToUpper(network),
			Reachable:   true,
			DurationMs:  12,
			RCode:       "NOERROR",
			AnswerCount: 1,
		}
	})

	probe, err := ProbeAuthoritativeDNSWorker(worker.ID, DNSWorkerProbeInput{ZoneID: zone.ID})
	if err != nil {
		t.Fatalf("ProbeAuthoritativeDNSWorker: %v", err)
	}
	if probe.WorkerID != worker.WorkerID || probe.QueryName != "example.com." || probe.QueryType != "SOA" {
		t.Fatalf("unexpected probe view: %+v", probe)
	}
	if len(probe.Results) != 2 || len(calls) != 2 {
		t.Fatalf("expected UDP and TCP probe results, got calls=%+v results=%+v", calls, probe.Results)
	}
	if probe.Results[0].Network != "UDP" || probe.Results[1].Network != "TCP" {
		t.Fatalf("unexpected probe networks: %+v", probe.Results)
	}
	workers, err := ListAuthoritativeDNSWorkers()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSWorkers: %v", err)
	}
	if len(workers) != 1 || workers[0].LastProbeAt == nil || workers[0].LastProbeQuery != "example.com. SOA" {
		t.Fatalf("unexpected persisted probe worker view: %+v", workers)
	}
	if len(workers[0].LastProbeResults) != 2 || !workers[0].LastProbeResults[0].Reachable {
		t.Fatalf("unexpected persisted probe results: %+v", workers[0].LastProbeResults)
	}
	if !workers[0].ProbeHealthy || workers[0].ProbeStatus != dnsWorkerProbeHealthy || workers[0].ProbeMessage == "" {
		t.Fatalf("unexpected persisted probe health: %+v", workers[0])
	}
	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if len(summary.WorkerHealth.Workers) != 1 ||
		summary.WorkerHealth.Workers[0].LastProbeAt == nil ||
		len(summary.WorkerHealth.Workers[0].LastProbeResults) != 2 ||
		!summary.WorkerHealth.Workers[0].ProbeHealthy ||
		summary.WorkerHealth.ProbeHealthyCount != 1 ||
		summary.WorkerHealth.ProbeCheckedCount != 1 {
		t.Fatalf("unexpected worker health probe state: %+v", summary.WorkerHealth.Workers)
	}
}

func TestAgentDNSProbeResultsPersistToWorkerHealth(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1-hk",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	_, err = RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{
		Version:             "v1.0.0",
		Status:              dnsWorkerStatusOnline,
		LastSnapshotVersion: "snapshot-a",
		LastSnapshotAt:      &now,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	node := &model.Node{
		NodeID:            "node-hk-1",
		Name:              "hk-edge-1",
		IP:                "203.0.113.10",
		PoolName:          "HK",
		AgentToken:        "agent-token",
		AgentVersion:      "1.0.0",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
		SchedulingEnabled: true,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	persistHeartbeatObservability(node.NodeID, AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      worker.WorkerID,
				Name:          "ns1-hk",
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 11, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 17, RCode: "NOERROR", AnswerCount: 1},
				},
			},
		},
	}, now)

	probes, err := model.ListDNSWorkerNodeProbes()
	if err != nil {
		t.Fatalf("ListDNSWorkerNodeProbes: %v", err)
	}
	if len(probes) != 1 || probes[0].WorkerID != worker.WorkerID || probes[0].NodeID != node.NodeID || !probes[0].Healthy {
		t.Fatalf("unexpected persisted node probe: %+v", probes)
	}
	if probes[0].AverageRTTMs != 14 || probes[0].MaxRTTMs != 17 || probes[0].FailureSamples != 0 {
		t.Fatalf("unexpected probe stats: %+v", probes[0])
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.WorkerHealth.NodeProbeCheckedCount != 1 ||
		summary.WorkerHealth.NodeProbeHealthyCount != 1 ||
		summary.WorkerHealth.NodeProbeHealthyPercent != 100 ||
		summary.WorkerHealth.NodeProbeAverageRTTMs != 14 ||
		summary.WorkerHealth.NodeProbeMaxRTTMs != 17 {
		t.Fatalf("unexpected node probe summary: %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 1 || len(summary.WorkerHealth.Workers[0].NodeProbes) != 1 {
		t.Fatalf("expected node probe in worker health item: %+v", summary.WorkerHealth.Workers)
	}
	nodeProbe := summary.WorkerHealth.Workers[0].NodeProbes[0]
	if nodeProbe.NodeName != "hk-edge-1" || nodeProbe.PoolName != "HK" || !nodeProbe.Healthy || len(nodeProbe.Results) != 2 {
		t.Fatalf("unexpected node probe view: %+v", nodeProbe)
	}
}

func TestAgentDNSProbeResultsStaleExcludedFromWorkerHealth(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1-hk",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	_, err = RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{
		Version:             "v1.0.0",
		Status:              dnsWorkerStatusOnline,
		LastSnapshotVersion: "snapshot-a",
		LastSnapshotAt:      &now,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	for _, node := range []*model.Node{
		{
			NodeID:            "node-fresh",
			Name:              "fresh-edge",
			PoolName:          "HK",
			AgentToken:        "agent-token-fresh",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now,
			SchedulingEnabled: true,
		},
		{
			NodeID:            "node-stale",
			Name:              "stale-edge",
			PoolName:          "EU",
			AgentToken:        "agent-token-stale",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now,
			SchedulingEnabled: true,
		},
	} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}

	results := []AgentDNSProbeResult{
		{Network: "UDP", Reachable: true, DurationMs: 10, RCode: "NOERROR", AnswerCount: 1},
		{Network: "TCP", Reachable: true, DurationMs: 20, RCode: "NOERROR", AnswerCount: 1},
	}
	persistHeartbeatObservability("node-fresh", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: now.Unix(),
			Results:       results,
		}},
	}, now)
	staleCheckedAt := now.Add(-defaultDNSWorkerNodeProbeMaxAge - time.Minute)
	persistHeartbeatObservability("node-stale", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: staleCheckedAt.Unix(),
			Results:       results,
		}},
	}, now)

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.WorkerHealth.NodeProbeCheckedCount != 2 ||
		summary.WorkerHealth.NodeProbeHealthyCount != 1 ||
		summary.WorkerHealth.NodeProbeStaleCount != 1 ||
		summary.WorkerHealth.NodeProbeHealthyPercent != 50 ||
		summary.WorkerHealth.NodeProbeAverageRTTMs != 15 ||
		summary.WorkerHealth.NodeProbeMaxRTTMs != 20 {
		t.Fatalf("unexpected stale-aware node probe summary: %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 1 || len(summary.WorkerHealth.Workers[0].NodeProbes) != 2 {
		t.Fatalf("expected two node probes in worker health item: %+v", summary.WorkerHealth.Workers)
	}
	workerHealth := summary.WorkerHealth.Workers[0]
	if workerHealth.NodeProbeHealthyCount != 1 || workerHealth.NodeProbeStaleCount != 1 || workerHealth.NodeProbeAverageRTTMs != 15 {
		t.Fatalf("unexpected worker node probe stats: %+v", workerHealth)
	}
	var staleProbe DNSWorkerNodeProbeView
	for _, probe := range workerHealth.NodeProbes {
		if probe.NodeID == "node-stale" {
			staleProbe = probe
			break
		}
	}
	if staleProbe.NodeID == "" || staleProbe.Healthy || staleProbe.ProbeStatus != dnsWorkerProbeStale || staleProbe.ProbeAgeSeconds <= 0 {
		t.Fatalf("unexpected stale probe view: %+v", staleProbe)
	}
}

func TestSimulateAuthoritativeDNSGSLBMatchesSourceCountryAndLoad(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	now := time.Now()
	for _, node := range []*model.Node{
		{
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
		},
		{
			NodeID:          "node-eu",
			Name:            "eu",
			IP:              "1.1.1.1",
			PoolName:        "eu",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          100,
			AgentToken:      "token-eu",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-time.Second),
		},
		{
			NodeID:          "node-hot",
			Name:            "hot",
			IP:              "9.9.9.9",
			PoolName:        "hk",
			PublicIPs:       `["9.9.9.9"]`,
			Weight:          100,
			AgentToken:      "token-hot",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-2 * time.Second),
		},
	} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               "node-hot",
		CapturedAt:           now,
		CPUUsagePercent:      20,
		MemoryUsedBytes:      20,
		MemoryTotalBytes:     100,
		OpenrestyConnections: 99,
	}).Insert(); err != nil {
		t.Fatalf("insert hot node metric: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "weighted", 30)
	policy.TargetCount = 2
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 80, Countries: []string{"HK"}, SourceCIDRs: []string{"198.51.100.0/24"}, Enabled: true},
		{Name: "eu", Weight: 20, Countries: []string{"DE"}, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
	}
	policy.LoadThresholds.MaxOpenrestyConnections = 50
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
		DNSTargetCount:  2,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	hk, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
		Country:      "hk",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB HK: %v", err)
	}
	if hk.SourceScope != "country:HK" || hk.TTL != 30 || hk.Strategy != "weighted" {
		t.Fatalf("unexpected HK simulation metadata: %+v", hk)
	}
	if len(hk.Targets) != 1 || hk.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected HK pool target without overloaded node, got %+v", hk.Targets)
	}
	assertSimulationPool(t, hk.MatchedPools, "hk", true)
	assertSimulationPool(t, hk.MatchedPools, "eu", false)
	assertSimulationNode(t, hk.Nodes, "node-hk", true, true, "可参与当前调度")
	assertSimulationNode(t, hk.Nodes, "node-hot", false, false, "节点负载超过 GSLB 阈值")
	assertSimulationNode(t, hk.Nodes, "node-eu", false, false, "节点池未匹配当前来源")

	de, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
		Country:      "DE",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB DE: %v", err)
	}
	if de.SourceScope != "country:DE" || len(de.Targets) != 1 || de.Targets[0] != "1.1.1.1" {
		t.Fatalf("expected DE pool target, got %+v", de)
	}
	assertSimulationPool(t, de.MatchedPools, "eu", true)
	assertSimulationNode(t, de.Nodes, "node-eu", true, true, "可参与当前调度")

	cidr, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
		Country:      "HK",
		SourceIP:     "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB CIDR: %v", err)
	}
	if !strings.HasPrefix(cidr.SourceScope, "cidr:203.0.113.0/24|bucket:") || len(cidr.Targets) != 1 || cidr.Targets[0] != "1.1.1.1" {
		t.Fatalf("expected CIDR pool target to override country, got %+v", cidr)
	}
	assertSimulationPool(t, cidr.MatchedPools, "eu", true)
	assertSimulationPool(t, cidr.MatchedPools, "hk", false)
	assertSimulationPoolReason(t, cidr.MatchedPools, "eu", "匹配来源网段 203.0.113.0/24")
}

func TestDNSWorkerProbeStateClassifiesFailedPartialAndStale(t *testing.T) {
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)

	failedAt := now.Add(-time.Hour)
	failed := evaluateDNSWorkerProbeState(now, &failedAt, []DNSWorkerProbeResultView{
		{Network: "UDP", Reachable: false},
		{Network: "TCP", Reachable: false},
	})
	if failed.status != dnsWorkerProbeFailed || failed.healthy {
		t.Fatalf("unexpected failed probe state: %+v", failed)
	}

	partial := evaluateDNSWorkerProbeState(now, &failedAt, []DNSWorkerProbeResultView{
		{Network: "UDP", Reachable: true},
		{Network: "TCP", Reachable: false},
	})
	if partial.status != dnsWorkerProbePartial || partial.healthy {
		t.Fatalf("unexpected partial probe state: %+v", partial)
	}

	staleAt := now.Add(-(defaultDNSWorkerProbeMaxAge + time.Second))
	stale := evaluateDNSWorkerProbeState(now, &staleAt, []DNSWorkerProbeResultView{
		{Network: "UDP", Reachable: true},
		{Network: "TCP", Reachable: true},
	})
	if stale.status != dnsWorkerProbeStale || stale.healthy {
		t.Fatalf("unexpected stale probe state: %+v", stale)
	}
}

func restoreDNSLookupNS(t *testing.T, lookup func(string) ([]*net.NS, error)) {
	t.Helper()
	original := dnsLookupNS
	dnsLookupNS = lookup
	t.Cleanup(func() {
		dnsLookupNS = original
	})
}

func restoreDNSWorkerProbeExchange(t *testing.T, exchange func(context.Context, string, string, string, uint16, time.Duration) DNSWorkerProbeResultView) {
	t.Helper()
	original := dnsWorkerProbeExchange
	dnsWorkerProbeExchange = exchange
	t.Cleanup(func() {
		dnsWorkerProbeExchange = original
	})
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func ptrUint(value uint) *uint {
	return &value
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

func assertSimulationPool(t *testing.T, pools []DNSGSLBSimulationPoolView, name string, matched bool) {
	t.Helper()
	for _, pool := range pools {
		if pool.Name == name {
			if pool.Matched != matched {
				t.Fatalf("unexpected simulation pool %s: %+v", name, pool)
			}
			return
		}
	}
	t.Fatalf("missing simulation pool %s in %+v", name, pools)
}

func assertSimulationPoolReason(t *testing.T, pools []DNSGSLBSimulationPoolView, name string, reason string) {
	t.Helper()
	for _, pool := range pools {
		if pool.Name != name {
			continue
		}
		if pool.Reason != reason {
			t.Fatalf("unexpected simulation pool reason %s: %+v", name, pool)
		}
		return
	}
	t.Fatalf("missing simulation pool %s in %+v", name, pools)
}

func assertSimulationNode(t *testing.T, nodes []DNSGSLBSimulationNodeView, nodeID string, eligible bool, selected bool, reason string) {
	t.Helper()
	for _, node := range nodes {
		if node.NodeID != nodeID {
			continue
		}
		if node.Eligible != eligible || node.Selected != selected {
			t.Fatalf("unexpected simulation node %s: %+v", nodeID, node)
		}
		for _, item := range node.Reasons {
			if item == reason {
				return
			}
		}
		t.Fatalf("missing reason %q for node %s: %+v", reason, nodeID, node.Reasons)
	}
	t.Fatalf("missing simulation node %s in %+v", nodeID, nodes)
}
