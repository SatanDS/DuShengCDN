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
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"

	"github.com/miekg/dns"
)

func TestAuthoritativeDNSZoneRecordWorkerAndSnapshot(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = false
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
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
	if snapshot.GSLBProbeSchedulingEnabled {
		t.Fatal("expected probe scheduling to be disabled by default in snapshot")
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

func TestAuthoritativeDNSSnapshotIgnoresInvalidGSLBPolicyWhenDisabled(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = false
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
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
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     false,
		GSLBPolicy:      "{not-json",
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if len(snapshot.Routes) != 1 || len(snapshot.Routes[0].CurrentTargets) != 1 || snapshot.Routes[0].CurrentTargets[0] != "8.8.4.4" {
		t.Fatalf("expected non-GSLB authoritative route to ignore invalid stored GSLB policy, got %+v", snapshot.Routes)
	}
	if snapshot.Routes[0].GSLBEnabled {
		t.Fatalf("expected route to remain non-GSLB in snapshot, got %+v", snapshot.Routes[0])
	}
}

func TestAuthoritativeDNSSnapshotProbeSchedulingFiltersTargets(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = true
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{
		Name:        "example.com",
		NameServers: []string{"ns1.example.com"},
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1",
		PublicAddress: "203.0.113.10:53",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:          "node-unprobed",
			Name:            "unprobed",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          1000,
			AgentToken:      "token-unprobed",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-probed",
			Name:            "probed",
			IP:              "8.8.4.4",
			PoolName:        "hk",
			PublicIPs:       `["8.8.4.4"]`,
			Weight:          10,
			AgentToken:      "token-probed",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}
	if err := model.UpsertDNSWorkerNodeProbe(model.DB, &model.DNSWorkerNodeProbe{
		WorkerID:       workerModel.WorkerID,
		NodeID:         "node-probed",
		PublicAddress:  workerModel.PublicAddress,
		QueryName:      "example.com",
		QueryType:      "SOA",
		Healthy:        true,
		AverageRTTMs:   12,
		MaxRTTMs:       18,
		ResultsJSON:    `[]`,
		CheckedAt:      now,
		FailureSamples: 0,
	}); err != nil {
		t.Fatalf("upsert probe: %v", err)
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
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      string(rawPolicy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if !snapshot.GSLBProbeSchedulingEnabled {
		t.Fatal("expected probe scheduling flag in snapshot")
	}
	if len(snapshot.Routes) != 1 || len(snapshot.Routes[0].CurrentTargets) != 1 || snapshot.Routes[0].CurrentTargets[0] != "8.8.4.4" {
		t.Fatalf("expected snapshot target to require healthy DNS probe, got %+v", snapshot.Routes)
	}
	probed := findSnapshotNode(snapshot.Nodes, "node-probed")
	if probed == nil || !probed.DNSProbeHealthy || probed.DNSProbeHealthyCount != 1 || probed.DNSProbeAverageRTTMs != 12 || probed.DNSProbeMaxRTTMs != 18 {
		t.Fatalf("expected probed node summary in snapshot, got %+v", probed)
	}
	unprobed := findSnapshotNode(snapshot.Nodes, "node-unprobed")
	if unprobed == nil || unprobed.DNSProbeHealthy || unprobed.DNSProbeCheckedCount != 0 {
		t.Fatalf("expected unprobed node to be unhealthy for probe scheduling, got %+v", unprobed)
	}

	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	server := dnsworker.NewDNSServer(
		dnsworker.NewSnapshotStore("", dnsworker.DefaultSnapshotMaxAge),
		dnsworker.NewScheduler(),
		dnsworker.NewRollupAggregator(time.Minute),
		nil,
		"",
	)
	if err := server.Store.Set(workerSnapshot); err != nil {
		t.Fatalf("set worker snapshot: %v", err)
	}
	response := server.Resolve(testDNSQuery("www.example.com", dns.TypeA, ""), nil)
	if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("expected worker response from probed node, rcode=%s answer=%v", dns.RcodeToString[response.Rcode], response.Answer)
	}
	if got := response.Answer[0].(*dns.A).A.String(); got != "8.8.4.4" {
		t.Fatalf("expected worker scheduler to require healthy DNS probe, got %s", got)
	}
}

func TestAuthoritativeDNSSnapshotVersionIgnoresProbeRTTJitter(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &AuthoritativeDNSSnapshot{
		GSLBProbeSchedulingEnabled: true,
		GeneratedAt:                now,
		Nodes: []AuthoritativeDNSSnapshotNode{
			{
				NodeID:               "node-a",
				Name:                 "node-a",
				PoolName:             "hk",
				PublicIPs:            []string{"8.8.4.4"},
				Weight:               100,
				SchedulingEnabled:    true,
				Status:               NodeStatusOnline,
				OpenrestyStatus:      OpenrestyStatusHealthy,
				LastSeenAt:           now,
				DNSProbeHealthy:      true,
				DNSProbeCheckedCount: 1,
				DNSProbeHealthyCount: 1,
				DNSProbeAverageRTTMs: 12,
				DNSProbeMaxRTTMs:     18,
			},
		},
	}
	left, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version: %v", err)
	}
	snapshot.Nodes[0].DNSProbeAverageRTTMs = 99
	snapshot.Nodes[0].DNSProbeMaxRTTMs = 120
	right, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version after rtt jitter: %v", err)
	}
	if left != right {
		t.Fatalf("expected probe RTT jitter not to change snapshot version, got %s and %s", left, right)
	}
	snapshot.Nodes[0].DNSProbeHealthy = false
	changed, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version after health change: %v", err)
	}
	if changed == left {
		t.Fatalf("expected probe health change to affect snapshot version, got %s", changed)
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

func TestCreateAuthoritativeDNSRecordRejectsDynamicRouteConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
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
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err == nil || !strings.Contains(err.Error(), "动态") {
		t.Fatalf("expected dynamic A conflict, got %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "CNAME", Value: "alias.example.com"}); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("expected CNAME conflict, got %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "AAAA", Value: "2001:db8::1"}); err != nil {
		t.Fatalf("expected AAAA record to coexist with dynamic A route: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "mail", Type: "MX", Value: "mail.example.com", Priority: 10}); err != nil {
		t.Fatalf("expected MX record to coexist with dynamic route: %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRejectsStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "静态记录") {
		t.Fatalf("expected static record conflict, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRejectsNoAvailableTargets(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "无法返回 A 边缘 IP") {
		t.Fatalf("expected missing target error, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeAllowsDisabledDraftWithoutTargets(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	view, err := CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         false,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute: %v", err)
	}
	if view.Enabled || view.DNSProviderMode != DNSProviderModeAuthoritative {
		t.Fatalf("unexpected disabled draft view: %+v", view)
	}
}

func TestCreateProxyRouteAuthoritativeRequiresReadyDNSWorker(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "DNS Worker") {
		t.Fatalf("expected missing DNS Worker readiness error, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRequiresFreshDNSWorkerSnapshot(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	worker := createProbeHealthyDNSWorker(t, now)
	worker.LastSnapshotVersion = "stale-snapshot"
	staleSnapshotAt := now.Add(-(defaultDNSSnapshotMaxAge + time.Minute))
	worker.LastSnapshotAt = &staleSnapshotAt
	if err := worker.Update(); err != nil {
		t.Fatalf("update stale worker snapshot: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "调度快照") {
		t.Fatalf("expected stale DNS Worker snapshot error, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRejectsPartialPublicWorkerStaleSnapshot(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	createReadyDNSWorker(t, now)
	staleWorker := createReadyDNSWorkerWithName(t, "ns2", now)
	staleWorker.LastSnapshotVersion = "stale-snapshot"
	staleSnapshotAt := now.Add(-(defaultDNSSnapshotMaxAge + time.Minute))
	staleWorker.LastSnapshotAt = &staleSnapshotAt
	if err := staleWorker.Update(); err != nil {
		t.Fatalf("update stale worker snapshot: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "部分公网可达") {
		t.Fatalf("expected partial stale DNS Worker snapshot error, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRejectsDivergentPublicWorkerSnapshots(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	createReadyDNSWorker(t, now)
	peerWorker := createReadyDNSWorkerWithName(t, "ns2", now)
	peerWorker.LastSnapshotVersion = "snapshot-b"
	if err := peerWorker.Update(); err != nil {
		t.Fatalf("update divergent worker snapshot: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "调度快照版本不一致") {
		t.Fatalf("expected divergent DNS Worker snapshot error, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeAllowsReadyDNSWorker(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	createReadyDNSWorker(t, now)

	view, err := CreateProxyRoute(ProxyRouteInput{
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute: %v", err)
	}
	if !view.Enabled || view.DNSProviderMode != DNSProviderModeAuthoritative {
		t.Fatalf("unexpected authoritative route view: %+v", view)
	}
}

func TestUpdateProxyRouteAuthoritativeRejectsSourceSpecificNoTargets(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         false,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "weighted", 60)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 80, Countries: []string{"HK"}, Enabled: true},
		{Name: "eu", Weight: 20, Countries: []string{"DE"}, Enabled: true},
	}
	_, err = UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		OriginURL:       "https://origin.internal",
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      policy,
	})
	if err == nil || !strings.Contains(err.Error(), "来源国家 DE 无法返回 A 边缘 IP") {
		t.Fatalf("expected DE source target error, got %v", err)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesReportsStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
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
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready {
		t.Fatalf("expected candidate to be blocked by static record conflict: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID || candidate.PublicReachableWorkerCount != 1 || candidate.ReadyWorkerCount != 1 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "静态记录") {
		t.Fatalf("expected static record blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesReportsNoAvailableGSLBTargets(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "load_aware",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "load_aware", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
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
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready {
		t.Fatalf("expected candidate to be blocked by missing GSLB targets: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID || candidate.PublicReachableWorkerCount != 1 || candidate.ReadyWorkerCount != 1 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "无法返回 A 边缘 IP") {
		t.Fatalf("expected missing target blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesRequiresFreshWorkerSnapshot(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	createProbeHealthyDNSWorker(t, now)

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready {
		t.Fatalf("expected candidate to be blocked by missing fresh snapshot: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID || candidate.PublicReachableWorkerCount != 1 || candidate.FreshSnapshotWorkerCount != 0 || candidate.ReadyWorkerCount != 0 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "调度快照") {
		t.Fatalf("expected snapshot blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesRejectsDivergentPublicWorkerSnapshots(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	createReadyDNSWorker(t, now)
	peerWorker := createReadyDNSWorkerWithName(t, "ns2", now)
	peerWorker.LastSnapshotVersion = "snapshot-b"
	if err := peerWorker.Update(); err != nil {
		t.Fatalf("update divergent worker snapshot: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready {
		t.Fatalf("expected candidate to be blocked by divergent snapshots: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID || candidate.PublicReachableWorkerCount != 2 || candidate.ReadyWorkerCount != 2 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "调度快照版本不一致") {
		t.Fatalf("expected divergent snapshot blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesChecksSourceSpecificGSLBTargets(t *testing.T) {
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
	now := time.Now().UTC()
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
		t.Fatalf("insert hk node: %v", err)
	}
	policy := defaultGSLBPolicy("hk", 1, "weighted", 60)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 80, Countries: []string{"HK"}, Enabled: true},
		{Name: "eu", Weight: 20, Countries: []string{"DE"}, Enabled: true},
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1", PublicAddress: "ns1.example.net"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	workerModel.Status = dnsWorkerStatusOnline
	workerModel.LastSeenAt = &now
	workerModel.LastProbeAt = &now
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &now
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready {
		t.Fatalf("expected candidate to be blocked by DE source pool: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID {
		t.Fatalf("unexpected candidate zone metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "来源国家 DE 无法返回 A 边缘 IP") {
		t.Fatalf("expected DE source blocker, got %+v", candidate.Blockers)
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
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}
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
		LastSeenAt:      checkedAt,
	}).Insert(); err != nil {
		t.Fatalf("insert schedulable node: %v", err)
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

func TestSwitchProxyRouteToAuthoritativeDNSRejectsStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
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
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	if _, err := SwitchProxyRouteToAuthoritativeDNS(route.ID, AuthoritativeDNSMigrationInput{DNSZoneIDRef: &zone.ID}); err == nil || !strings.Contains(err.Error(), "静态记录") {
		t.Fatalf("expected static record conflict, got %v", err)
	}
}

func TestSwitchProxyRouteToAuthoritativeDNSRejectsNoAvailableGSLBTargets(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "load_aware",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "load_aware", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
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
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}

	if _, err := SwitchProxyRouteToAuthoritativeDNS(route.ID, AuthoritativeDNSMigrationInput{DNSZoneIDRef: &zone.ID}); err == nil || !strings.Contains(err.Error(), "无法返回 A 边缘 IP") {
		t.Fatalf("expected missing target error, got %v", err)
	}
}

func TestSwitchProxyRouteToAuthoritativeDNSExplainsProbeThresholdPrecheck(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = true
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	checkedAt := time.Now().UTC()
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
		LastSeenAt:      checkedAt,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSAutoSync:     true,
		DNSProviderMode: DNSProviderModeCloudflare,
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 60)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	createReadyDNSWorker(t, checkedAt)

	_, err = SwitchProxyRouteToAuthoritativeDNS(route.ID, AuthoritativeDNSMigrationInput{DNSZoneIDRef: &zone.ID})
	if err == nil {
		t.Fatal("expected probe threshold precheck error")
	}
	if !strings.Contains(err.Error(), "Agent 探测调度门槛") || !strings.Contains(err.Error(), "Agent 探测未达到调度门槛") {
		t.Fatalf("expected probe threshold guidance in error, got %v", err)
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
				TargetSummary: map[string]int64{
					"8.8.8.8":   40,
					" 8.8.8.8 ": 2,
					" ":         9,
					"1.1.1.1":   0,
					"9.9.9.9":   -3,
				},
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
	targetSummary := decodeDNSTargetSummary(rollup.TargetSummary)
	if len(targetSummary) != 1 || targetSummary["8.8.8.8"] != 42 {
		t.Fatalf("expected sanitized target summary, got raw=%s decoded=%+v", rollup.TargetSummary, targetSummary)
	}
}

func TestDNSWorkerHeartbeatClampsFutureSnapshotTime(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	futureSnapshotAt := time.Now().UTC().Add(time.Hour)
	view, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:              "online",
		LastSnapshotVersion: "future-snapshot",
		LastSnapshotAt:      &futureSnapshotAt,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if view.LastSnapshotAt == nil || !view.LastSnapshotAt.Before(futureSnapshotAt) {
		t.Fatalf("expected future snapshot time to be clamped in view, got %+v", view.LastSnapshotAt)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if reloaded.LastSnapshotAt == nil || !reloaded.LastSnapshotAt.Before(futureSnapshotAt) {
		t.Fatalf("expected future snapshot time to be clamped in database, got %+v", reloaded.LastSnapshotAt)
	}
	summary := buildDNSWorkerSnapshotConsistency(time.Now().UTC())
	if summary.LatestSnapshotAt == nil || !summary.LatestSnapshotAt.Before(futureSnapshotAt) {
		t.Fatalf("expected snapshot consistency to avoid future latest snapshot, got %+v", summary)
	}
}

func TestDNSWorkerHeartbeatNormalizesInconsistentRollupDurations(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	windowStart := time.Now().UTC().Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      4,
				TotalDurationMs: 10,
				MaxDurationMs:   30,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	var rollup model.DNSQueryRollup
	if err := model.DB.Where("worker_id = ?", authenticated.WorkerID).First(&rollup).Error; err != nil {
		t.Fatalf("load rollup: %v", err)
	}
	if rollup.TotalDurationMs != 30 || rollup.MaxDurationMs != 30 {
		t.Fatalf("expected total duration to be at least max duration, got %+v", rollup)
	}
	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.WorkerHealth.Workers[0].AverageLatencyMs != 7.5 || summary.WorkerHealth.Workers[0].MaxLatencyMs != 30 {
		t.Fatalf("unexpected normalized worker latency: %+v", summary.WorkerHealth.Workers[0])
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

	futureChangedAt := time.Now().UTC().Add(time.Hour)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         route.ID,
				RecordType:      "A",
				ScopeKey:        "country:HK",
				SelectedTargets: []string{"8.8.8.8"},
				DesiredTargets:  []string{"8.8.8.8"},
				LastChangedAt:   &futureChangedAt,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat future state: %v", err)
	}
	if err := model.DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", route.ID, "A", "country:HK").First(&state).Error; err != nil {
		t.Fatalf("reload future scheduling state: %v", err)
	}
	if state.LastChangedAt == nil || !state.LastChangedAt.Before(futureChangedAt) {
		t.Fatalf("expected future LastChangedAt to be clamped, got %+v", state)
	}
	clampedChangedAt := *state.LastChangedAt
	for !time.Now().UTC().After(clampedChangedAt) {
		time.Sleep(time.Millisecond)
	}
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         route.ID,
				RecordType:      "A",
				ScopeKey:        "country:HK",
				SelectedTargets: []string{"1.1.1.1"},
				DesiredTargets:  []string{"1.1.1.1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat normal state after future: %v", err)
	}
	if err := model.DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", route.ID, "A", "country:HK").First(&state).Error; err != nil {
		t.Fatalf("reload normal scheduling state: %v", err)
	}
	if state.SelectedTargets != `["1.1.1.1"]` || state.LastChangedAt == nil || !state.LastChangedAt.After(clampedChangedAt) {
		t.Fatalf("expected normal heartbeat to overwrite clamped future state, got %+v", state)
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
	if trendTotalQueries(summary.TrendPoints) != 87 ||
		trendTotalServfailQueries(summary.TrendPoints) != 2 ||
		trendTotalNXDomainQueries(summary.TrendPoints) != 5 ||
		trendTotalDynamicQueries(summary.TrendPoints) != 82 {
		t.Fatalf("unexpected trend points: %+v", summary.TrendPoints)
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

func TestAuthoritativeDNSWorkerHealthIgnoresUnknownWorkerRollups(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	windowStart := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      10,
				TotalDurationMs: 100,
				MaxDurationMs:   20,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if err := (&model.DNSQueryRollup{
		WindowStart:     windowStart,
		WindowMinutes:   1,
		WorkerID:        "stale-worker",
		QName:           "www.example.com",
		QType:           "A",
		RCode:           "SERVFAIL",
		QueryCount:      90,
		TotalDurationMs: 9000,
		MaxDurationMs:   5000,
		TargetSummary:   `{}`,
	}).Insert(); err != nil {
		t.Fatalf("insert stale worker rollup: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.TotalQueries != 100 || summary.ErrorQueries != 90 {
		t.Fatalf("expected observability totals to retain historical rollups, got %+v", summary)
	}
	assertCounter(t, summary.WorkerBreakdown, "stale-worker", "stale-worker", 90)
	if summary.WorkerHealth.TotalWorkerCount != 1 || len(summary.WorkerHealth.Workers) != 1 {
		t.Fatalf("unexpected worker health workers: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.MaxLatencyMs != 20 || summary.WorkerHealth.AverageLatencyMs != 10 || summary.WorkerHealth.ErrorRatePercent != 0 {
		t.Fatalf("worker health should ignore unknown worker rollups: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.Workers[0].WorkerID != authenticated.WorkerID ||
		summary.WorkerHealth.Workers[0].QueryCount != 10 ||
		summary.WorkerHealth.Workers[0].ErrorQueries != 0 ||
		summary.WorkerHealth.Workers[0].MaxLatencyMs != 20 {
		t.Fatalf("unexpected current worker health: %+v", summary.WorkerHealth.Workers)
	}
}

func TestAuthoritativeDNSWorkerHealthNormalizesHistoricalRollupDurations(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	windowStart := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	if err := (&model.DNSQueryRollup{
		WindowStart:     windowStart,
		WindowMinutes:   1,
		WorkerID:        worker.WorkerID,
		QName:           "www.example.com",
		QType:           "A",
		RCode:           "NOERROR",
		QueryCount:      4,
		TotalDurationMs: 10,
		MaxDurationMs:   30,
		TargetSummary:   `{}`,
	}).Insert(); err != nil {
		t.Fatalf("insert historical rollup: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if len(summary.WorkerHealth.Workers) != 1 {
		t.Fatalf("expected one worker health item, got %+v", summary.WorkerHealth.Workers)
	}
	workerHealth := summary.WorkerHealth.Workers[0]
	if workerHealth.AverageLatencyMs != 7.5 || workerHealth.MaxLatencyMs != 30 {
		t.Fatalf("expected historical rollup duration normalization, got %+v", workerHealth)
	}
	if summary.WorkerHealth.AverageLatencyMs != 7.5 || summary.WorkerHealth.MaxLatencyMs != 30 {
		t.Fatalf("expected summary duration normalization, got %+v", summary.WorkerHealth)
	}
}

func TestAuthoritativeDNSObservabilityIncludesOverlappingRollupWindow(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	overlapStart := now.Add(-65 * time.Minute)
	overlapEnd := overlapStart.Add(10 * time.Minute)
	if err := (&model.DNSQueryRollup{
		WindowStart:     overlapStart,
		WindowMinutes:   10,
		WorkerID:        worker.WorkerID,
		QName:           "www.example.com",
		QType:           "A",
		RCode:           "NOERROR",
		QueryCount:      12,
		TotalDurationMs: 120,
		MaxDurationMs:   20,
		TargetSummary:   `{"8.8.8.8":12}`,
	}).Insert(); err != nil {
		t.Fatalf("insert overlapping rollup: %v", err)
	}
	if err := (&model.DNSQueryRollup{
		WindowStart:     now.Add(-90 * time.Minute),
		WindowMinutes:   20,
		WorkerID:        worker.WorkerID,
		QName:           "old.example.com",
		QType:           "A",
		RCode:           "SERVFAIL",
		QueryCount:      99,
		TotalDurationMs: 990,
		MaxDurationMs:   99,
		TargetSummary:   `{"1.1.1.1":99}`,
	}).Insert(); err != nil {
		t.Fatalf("insert old rollup: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.TotalQueries != 12 || summary.SuccessfulQueries != 12 || summary.ErrorQueries != 0 {
		t.Fatalf("expected only overlapping rollup to be counted, got %+v", summary)
	}
	var trendTotal int64
	for _, point := range summary.TrendPoints {
		trendTotal += point.QueryCount
	}
	if trendTotal != 12 {
		t.Fatalf("expected overlapping rollup to be counted in trend points, got %+v", summary.TrendPoints)
	}
	assertCounter(t, summary.TopTargets, "8.8.8.8", "8.8.8.8", 12)
	if summary.WorkerHealth.MaxLatencyMs != 20 || summary.WorkerHealth.AverageLatencyMs != 10 || summary.WorkerHealth.ErrorRatePercent != 0 {
		t.Fatalf("worker health should use the same filtered rollups: %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 1 ||
		summary.WorkerHealth.Workers[0].QueryCount != 12 ||
		summary.WorkerHealth.Workers[0].MaxLatencyMs != 20 {
		t.Fatalf("unexpected worker health rollup scope: %+v", summary.WorkerHealth.Workers)
	}
	if summary.LastRollupAt == nil || !summary.LastRollupAt.Equal(overlapEnd) {
		t.Fatalf("expected last rollup at %s, got %+v", overlapEnd, summary.LastRollupAt)
	}
}

func TestAuthoritativeDNSObservabilityTrendCoversRollingWindowStartHour(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	rollupStart := now.Add(-55 * time.Minute)
	if err := (&model.DNSQueryRollup{
		WindowStart:     rollupStart,
		WindowMinutes:   1,
		WorkerID:        worker.WorkerID,
		QName:           "early.example.com",
		QType:           "A",
		RCode:           "NOERROR",
		QueryCount:      7,
		TotalDurationMs: 70,
		MaxDurationMs:   10,
		TargetSummary:   `{"8.8.4.4":7}`,
	}).Insert(); err != nil {
		t.Fatalf("insert rollup: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.TotalQueries != 7 {
		t.Fatalf("expected rollup to be counted, got %+v", summary)
	}
	if trendTotalQueries(summary.TrendPoints) != 7 {
		t.Fatalf("expected trend to cover rolling window start hour, got %+v", summary.TrendPoints)
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
					{Network: "TCP", Reachable: true, DurationMs: 17, RCode: "NOERROR", AnswerCount: -3},
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
	persistedResults := decodeDNSWorkerProbeResults(probes[0].ResultsJSON)
	if len(persistedResults) != 2 || persistedResults[1].AnswerCount != 0 {
		t.Fatalf("expected negative answer count to be normalized, got %+v", persistedResults)
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
	if nodeProbe.NodeName != "hk-edge-1" || nodeProbe.PoolName != "HK" || !nodeProbe.Healthy || len(nodeProbe.Results) != 2 || nodeProbe.Results[1].AnswerCount != 0 {
		t.Fatalf("unexpected node probe view: %+v", nodeProbe)
	}
}

func TestAgentDNSProbeSummaryNormalizesHistoricalRTT(t *testing.T) {
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
		Status:         dnsWorkerStatusOnline,
		LastSnapshotAt: &now,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	node := &model.Node{
		NodeID:            "node-hk",
		Name:              "hk-edge",
		PoolName:          "HK",
		AgentToken:        "agent-token",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
		SchedulingEnabled: true,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := model.UpsertDNSWorkerNodeProbe(nil, &model.DNSWorkerNodeProbe{
		WorkerID:      worker.WorkerID,
		NodeID:        node.NodeID,
		PublicAddress: "ns1.example.net",
		QueryName:     "example.com.",
		QueryType:     "SOA",
		CheckedAt:     now,
		ResultsJSON:   `[{"network":"UDP","reachable":true,"duration_ms":13,"rcode":"NOERROR","answer_count":1}]`,
		Healthy:       true,
		AverageRTTMs:  12.5,
		MaxRTTMs:      7,
	}); err != nil {
		t.Fatalf("UpsertDNSWorkerNodeProbe: %v", err)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.WorkerHealth.NodeProbeAverageRTTMs != 12.5 || summary.WorkerHealth.NodeProbeMaxRTTMs != 13 {
		t.Fatalf("expected normalized summary RTT, got %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 1 || len(summary.WorkerHealth.Workers[0].NodeProbes) != 1 {
		t.Fatalf("expected node probe in worker health item: %+v", summary.WorkerHealth.Workers)
	}
	nodeProbe := summary.WorkerHealth.Workers[0].NodeProbes[0]
	if nodeProbe.AverageRTTMs != 12.5 || nodeProbe.MaxRTTMs != 13 {
		t.Fatalf("expected normalized node probe RTT, got %+v", nodeProbe)
	}

	nodeStats := buildDNSWorkerNodeProbeStatsByNode(now)
	probeStats := nodeStats[node.NodeID]
	if probeStats == nil || averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples) != 12.5 || probeStats.maxRTTMs != 13 {
		t.Fatalf("expected normalized node-level probe stats, got %+v", probeStats)
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
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	persistHeartbeatObservability("node-hk", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      worker.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 12, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 18, RCode: "NOERROR", AnswerCount: 1},
				},
			},
		},
	}, now)

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
	if hkNode := findSimulationNode(hk.Nodes, "node-hk"); hkNode == nil ||
		hkNode.NodeProbeStatus != dnsWorkerProbeHealthy ||
		hkNode.NodeProbeHealthyCount != 1 ||
		hkNode.NodeProbeCheckedCount != 1 ||
		hkNode.NodeProbeAverageRTTMs != 15 ||
		hkNode.NodeProbeMaxRTTMs != 18 {
		t.Fatalf("expected HK node simulation to include Agent probe summary, got %+v", hkNode)
	}
	if hot := findSimulationNode(hk.Nodes, "node-hot"); hot == nil || hot.MetricCapturedAt == nil {
		t.Fatalf("expected hot node simulation to include metric captured time, got %+v", hot)
	}

	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.GSLBProbeSchedulingEnabled = true
	probeFiltered, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
		Country:      "DE",
	})
	common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	if err != nil {
		t.Fatalf("expected DE simulation to return diagnostics when probe scheduling filters candidates, got %v", err)
	}
	if probeFiltered == nil || len(probeFiltered.Targets) != 0 || !strings.Contains(probeFiltered.Message, "Agent 探测未达到调度门槛") {
		t.Fatalf("expected no-target diagnostic result when probe scheduling filters DE node, got %+v", probeFiltered)
	}
	if probeFiltered.Targets == nil {
		t.Fatal("expected no-target diagnostic result to expose an empty targets array")
	}
	assertSimulationNodeReasonContains(t, probeFiltered.Nodes, "node-eu", "尚未收到新鲜成功探测")
	if diagnostic := findSimulationNode(probeFiltered.Nodes, "node-eu"); diagnostic == nil || diagnostic.Eligible || diagnostic.Selected {
		t.Fatalf("expected DE node to be visible but ineligible after probe threshold filtering, got %+v", diagnostic)
	}
	if diagnostic := findSimulationNode(hk.Nodes, "node-eu"); diagnostic != nil && containsString(diagnostic.Reasons, "Agent 探测未达到调度门槛") {
		t.Fatalf("expected probe threshold reason to stay hidden while option is disabled, got %+v", diagnostic.Reasons)
	}

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

func TestSimulateAuthoritativeDNSGSLBProbeSchedulingPrefersLowerRTT(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = true
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

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
	now := time.Now()
	for _, node := range []*model.Node{
		{
			NodeID:          "node-slow",
			Name:            "slow",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          100,
			AgentToken:      "token-slow",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-fast",
			Name:            "fast",
			IP:              "8.8.4.4",
			PoolName:        "hk",
			PublicIPs:       `["8.8.4.4"]`,
			Weight:          100,
			AgentToken:      "token-fast",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-10 * time.Second),
		},
	} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}
	persistHeartbeatObservability("node-slow", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      worker.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 70, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 90, RCode: "NOERROR", AnswerCount: 1},
				},
			},
		},
	}, now)
	persistHeartbeatObservability("node-fast", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      worker.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 10, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 14, RCode: "NOERROR", AnswerCount: 1},
				},
			},
		},
	}, now)

	policy := defaultGSLBPolicy("hk", 1, "weighted", 30)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 100, Enabled: true},
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
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	simulation, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB: %v", err)
	}
	if len(simulation.Targets) != 1 || simulation.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected lower Agent probe RTT target, got %+v", simulation.Targets)
	}
	fast := findSimulationNode(simulation.Nodes, "node-fast")
	slow := findSimulationNode(simulation.Nodes, "node-slow")
	if fast == nil || slow == nil || !fast.Selected || slow.Selected || fast.NodeProbeAverageRTTMs != 12 || slow.NodeProbeAverageRTTMs != 80 {
		t.Fatalf("unexpected probe RTT diagnostics, fast=%+v slow=%+v", fast, slow)
	}
}

func TestSimulateAuthoritativeDNSGSLBNoAvailableTargetReturnsDiagnostics(t *testing.T) {
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
	if err := (&model.Node{
		NodeID:          "node-hot",
		Name:            "hot",
		IP:              "8.8.4.4",
		PoolName:        "hk",
		PublicIPs:       `["8.8.4.4"]`,
		Weight:          100,
		AgentToken:      "token-hot",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               "node-hot",
		CapturedAt:           now,
		CPUUsagePercent:      95,
		MemoryUsedBytes:      95,
		MemoryTotalBytes:     100,
		OpenrestyConnections: 100,
	}).Insert(); err != nil {
		t.Fatalf("insert metric: %v", err)
	}
	policy := defaultGSLBPolicy("hk", 1, "load_aware", 30)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 100, Enabled: true},
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
		DNSTargetCount:  1,
		DNSScheduleMode: "load_aware",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	simulation, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
	})
	if err != nil {
		t.Fatalf("expected no-available-target simulation to return diagnostics, got %v", err)
	}
	if simulation == nil || len(simulation.Targets) != 0 || simulation.Targets == nil || !strings.Contains(simulation.Message, "当前来源没有可用于 A 记录的边缘节点") {
		t.Fatalf("unexpected no-target simulation result: %+v", simulation)
	}
	assertSimulationPool(t, simulation.MatchedPools, "hk", true)
	assertSimulationNode(t, simulation.Nodes, "node-hot", false, false, "节点负载超过 GSLB 阈值")
}

func TestSimulateAuthoritativeDNSGSLBProbeSchedulingExplainsThresholdReasons(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = true
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

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
	now := time.Now()
	for _, node := range []*model.Node{
		{
			NodeID:          "node-healthy",
			Name:            "healthy",
			IP:              "8.8.4.4",
			PoolName:        "hk",
			PublicIPs:       `["8.8.4.4"]`,
			Weight:          100,
			AgentToken:      "token-healthy",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-unprobed",
			Name:            "unprobed",
			IP:              "1.0.0.1",
			PoolName:        "hk",
			PublicIPs:       `["1.0.0.1"]`,
			Weight:          100,
			AgentToken:      "token-unprobed",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-time.Second),
		},
		{
			NodeID:          "node-stale",
			Name:            "stale",
			IP:              "9.9.9.9",
			PoolName:        "hk",
			PublicIPs:       `["9.9.9.9"]`,
			Weight:          100,
			AgentToken:      "token-stale",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-2 * time.Second),
		},
		{
			NodeID:          "node-partial",
			Name:            "partial",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          100,
			AgentToken:      "token-partial",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-3 * time.Second),
		},
		{
			NodeID:          "node-failed",
			Name:            "failed",
			IP:              "8.8.8.8",
			PoolName:        "hk",
			PublicIPs:       `["8.8.8.8"]`,
			Weight:          100,
			AgentToken:      "token-failed",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-4 * time.Second),
		},
	} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}
	healthyResults := []AgentDNSProbeResult{
		{Network: "UDP", Reachable: true, DurationMs: 11, RCode: "NOERROR", AnswerCount: 1},
		{Network: "TCP", Reachable: true, DurationMs: 13, RCode: "NOERROR", AnswerCount: 1},
	}
	persistHeartbeatObservability("node-healthy", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: now.Unix(),
			Results:       healthyResults,
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
			Results:       healthyResults,
		}},
	}, now)
	persistHeartbeatObservability("node-partial", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: now.Unix(),
			Results: []AgentDNSProbeResult{
				{Network: "UDP", Reachable: true, DurationMs: 10, RCode: "NOERROR", AnswerCount: 1},
				{Network: "TCP", Reachable: false, Error: "tcp timeout"},
			},
		}},
	}, now)
	persistHeartbeatObservability("node-failed", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: now.Unix(),
			Results: []AgentDNSProbeResult{
				{Network: "UDP", Reachable: false, Error: "udp timeout"},
				{Network: "TCP", Reachable: false, Error: "tcp timeout"},
			},
		}},
	}, now)

	policy := defaultGSLBPolicy("hk", 1, "weighted", 30)
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 100, Enabled: true},
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
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	simulation, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB: %v", err)
	}
	assertSimulationNode(t, simulation.Nodes, "node-healthy", true, true, "可参与当前调度")
	assertSimulationNodeReasonContains(t, simulation.Nodes, "node-unprobed", "尚未收到新鲜成功探测")
	assertSimulationNodeReasonContains(t, simulation.Nodes, "node-stale", "探测结果已过期")
	assertSimulationNodeReasonContains(t, simulation.Nodes, "node-partial", "UDP/TCP 53 未同时可达")
	assertSimulationNodeReasonContains(t, simulation.Nodes, "node-failed", "UDP/TCP 53 探测均失败")
	if unprobed := findSimulationNode(simulation.Nodes, "node-unprobed"); unprobed == nil || unprobed.NodeProbeStatus != dnsWorkerProbeUnknown {
		t.Fatalf("expected unprobed node to expose unknown probe status, got %+v", unprobed)
	}
	if stale := findSimulationNode(simulation.Nodes, "node-stale"); stale == nil || stale.NodeProbeStatus != dnsWorkerProbeStale || stale.NodeProbeStaleCount != 1 {
		t.Fatalf("expected stale node to expose stale probe status, got %+v", stale)
	}
	if partial := findSimulationNode(simulation.Nodes, "node-partial"); partial == nil || partial.NodeProbeStatus != dnsWorkerProbePartial || partial.NodeProbeHealthyCount != 0 {
		t.Fatalf("expected partial node to expose partial probe status, got %+v", partial)
	}
	if failed := findSimulationNode(simulation.Nodes, "node-failed"); failed == nil || failed.NodeProbeStatus != dnsWorkerProbeFailed || failed.NodeProbeHealthyCount != 0 {
		t.Fatalf("expected failed node to expose failed probe status, got %+v", failed)
	}
}

func TestGSLBSimulationDiagnosticsUsesSnapshotProbeSchedulingFlag(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	oldProbeScheduling := common.GSLBProbeSchedulingEnabled
	common.NodeOfflineThreshold = time.Minute
	common.GSLBProbeSchedulingEnabled = true
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
		common.GSLBProbeSchedulingEnabled = oldProbeScheduling
	})

	now := time.Now()
	if err := (&model.Node{
		NodeID:          "node-unprobed",
		Name:            "unprobed",
		IP:              "1.1.1.1",
		PoolName:        "hk",
		PublicIPs:       `["1.1.1.1"]`,
		Weight:          100,
		AgentToken:      "token-unprobed",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	policy := dnsworker.GSLBPolicy{
		Strategy:    "weighted",
		TargetCount: 1,
		TTL:         30,
		Pools: []dnsworker.GSLBPoolPolicy{
			{Name: "hk", Weight: 100, Enabled: true},
		},
	}
	diagnostics := buildDNSGSLBSimulationDiagnostics("A", policy, GSLBSourceContext{Country: "HK"}, []string{"1.1.1.1"}, false)
	node := findSimulationNode(diagnostics.nodes, "node-unprobed")
	if node == nil {
		t.Fatalf("expected node diagnostic to be present")
	}
	if !node.Eligible || !node.Selected {
		t.Fatalf("expected snapshot-disabled probe scheduling to keep node eligible and selected, got %+v", node)
	}
	if containsString(node.Reasons, "尚未收到新鲜成功探测") {
		t.Fatalf("expected probe threshold reason to follow snapshot flag instead of global option, got %+v", node.Reasons)
	}
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

func createReadyDNSWorker(t *testing.T, checkedAt time.Time) *model.DNSWorker {
	t.Helper()
	return createReadyDNSWorkerWithName(t, "ns1", checkedAt)
}

func createReadyDNSWorkerWithName(t *testing.T, name string, checkedAt time.Time) *model.DNSWorker {
	t.Helper()
	workerModel := createProbeHealthyDNSWorkerWithName(t, name, checkedAt)
	checkedAt = checkedAt.UTC()
	workerModel.LastSnapshotVersion = "snapshot-a"
	workerModel.LastSnapshotAt = &checkedAt
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker snapshot readiness: %v", err)
	}
	return workerModel
}

func createProbeHealthyDNSWorker(t *testing.T, checkedAt time.Time) *model.DNSWorker {
	t.Helper()
	return createProbeHealthyDNSWorkerWithName(t, "ns1", checkedAt)
}

func createProbeHealthyDNSWorkerWithName(t *testing.T, name string, checkedAt time.Time) *model.DNSWorker {
	t.Helper()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "ns1"
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: name, PublicAddress: name + ".example.net"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	checkedAt = checkedAt.UTC()
	workerModel.Status = dnsWorkerStatusOnline
	workerModel.LastSeenAt = &checkedAt
	workerModel.LastProbeAt = &checkedAt
	workerModel.LastProbeQuery = "example.com. SOA"
	workerModel.LastProbeResult = `[{"network":"UDP","reachable":true,"duration_ms":11,"rcode":"NOERROR","answer_count":1},{"network":"TCP","reachable":true,"duration_ms":14,"rcode":"NOERROR","answer_count":1}]`
	if err := workerModel.Update(); err != nil {
		t.Fatalf("update worker readiness: %v", err)
	}
	return workerModel
}

func containsStringWith(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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

func trendTotalQueries(points []DNSObservabilityTrendPointView) int64 {
	var total int64
	for _, point := range points {
		total += point.QueryCount
	}
	return total
}

func trendTotalServfailQueries(points []DNSObservabilityTrendPointView) int64 {
	var total int64
	for _, point := range points {
		total += point.ServfailQueries
	}
	return total
}

func trendTotalNXDomainQueries(points []DNSObservabilityTrendPointView) int64 {
	var total int64
	for _, point := range points {
		total += point.NXDomainQueries
	}
	return total
}

func trendTotalDynamicQueries(points []DNSObservabilityTrendPointView) int64 {
	var total int64
	for _, point := range points {
		total += point.DynamicQueries
	}
	return total
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
	node := findSimulationNode(nodes, nodeID)
	if node == nil {
		t.Fatalf("missing simulation node %s in %+v", nodeID, nodes)
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

func assertSimulationNodeReasonContains(t *testing.T, nodes []DNSGSLBSimulationNodeView, nodeID string, reasonFragment string) {
	t.Helper()
	node := findSimulationNode(nodes, nodeID)
	if node == nil {
		t.Fatalf("missing simulation node %s in %+v", nodeID, nodes)
	}
	if !containsStringWith(node.Reasons, reasonFragment) {
		t.Fatalf("missing reason containing %q for node %s: %+v", reasonFragment, nodeID, node.Reasons)
	}
}

func findSimulationNode(nodes []DNSGSLBSimulationNodeView, nodeID string) *DNSGSLBSimulationNodeView {
	for _, node := range nodes {
		if node.NodeID != nodeID {
			continue
		}
		found := node
		return &found
	}
	return nil
}

func findSnapshotNode(nodes []AuthoritativeDNSSnapshotNode, nodeID string) *AuthoritativeDNSSnapshotNode {
	for _, node := range nodes {
		if node.NodeID != nodeID {
			continue
		}
		found := node
		return &found
	}
	return nil
}

func testDNSQuery(name string, qtype uint16, remoteAddr string) *dns.Msg {
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(name), qtype)
	return message
}
