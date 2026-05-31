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
				WindowStart:     heartbeatAt,
				WindowMinutes:   5,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      42,
				TotalDurationMs: 210,
				MaxDurationMs:   12,
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
