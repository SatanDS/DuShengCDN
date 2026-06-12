package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/security"

	"github.com/miekg/dns"
	"gorm.io/gorm"
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
		PublicAddress: "8.8.8.8:53",
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
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
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
	if workerSnapshot.SchedulingStates[0].LastChangedAt == nil ||
		!workerSnapshot.SchedulingStates[0].LastChangedAt.Equal(lastChangedAt) {
		t.Fatalf("unexpected worker scheduling state time: %+v", workerSnapshot.SchedulingStates[0])
	}

	var reloadedWorker model.DNSWorker
	if err := model.DB.First(&reloadedWorker, authenticated.ID).Error; err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if reloadedWorker.LastSnapshotVersion != snapshot.SnapshotVersion || reloadedWorker.LastSnapshotAt == nil {
		t.Fatalf("expected worker snapshot metadata to be updated, got %+v", reloadedWorker)
	}
}

func TestSignedAuthoritativeDNSSnapshotConditionalUsesCachedVersion(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker: %v", err)
	}

	initial, err := GetSignedAuthoritativeDNSSnapshotConditional(authenticated, worker.Token, "")
	if err != nil {
		t.Fatalf("GetSignedAuthoritativeDNSSnapshotConditional initial: %v", err)
	}
	if initial.NotModified || initial.Snapshot == nil || initial.SnapshotVersion == "" {
		t.Fatalf("expected initial conditional request to return snapshot, got %+v", initial)
	}

	unchanged, err := GetSignedAuthoritativeDNSSnapshotConditional(authenticated, worker.Token, `"`+initial.SnapshotVersion+`"`)
	if err != nil {
		t.Fatalf("GetSignedAuthoritativeDNSSnapshotConditional unchanged: %v", err)
	}
	if !unchanged.NotModified || unchanged.Snapshot != nil || unchanged.SnapshotVersion != initial.SnapshotVersion {
		t.Fatalf("expected matching cached version to short-circuit, got %+v", unchanged)
	}

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{
		Name:  "www",
		Type:  "A",
		Value: "8.8.8.8",
	}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	changed, err := GetSignedAuthoritativeDNSSnapshotConditional(authenticated, worker.Token, initial.SnapshotVersion)
	if err != nil {
		t.Fatalf("GetSignedAuthoritativeDNSSnapshotConditional changed: %v", err)
	}
	if changed.NotModified || changed.Snapshot == nil || changed.SnapshotVersion == "" || changed.SnapshotVersion == initial.SnapshotVersion {
		t.Fatalf("expected changed revision to return a fresh snapshot, got initial=%s changed=%+v", initial.SnapshotVersion, changed)
	}
}

func TestAuthoritativeDNSSnapshotCacheRevisionIgnoresWorkerRuntimeMetadata(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	revisionBefore, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || revisionBefore == "" {
		t.Fatalf("expected initial cache revision, got %q ok=%v", revisionBefore, ok)
	}

	now := time.Now().UTC()
	if err := model.DB.Model(&model.DNSWorker{}).Where("id = ?", worker.ID).Updates(map[string]any{
		"status":                dnsWorkerStatusOnline,
		"last_seen_at":          now,
		"last_snapshot_at":      now,
		"last_snapshot_version": "snapshot-a",
	}).Error; err != nil {
		t.Fatalf("update worker runtime metadata: %v", err)
	}
	revisionAfterWorkerRuntimeUpdate, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok {
		t.Fatal("expected cache revision after worker runtime update")
	}
	if revisionAfterWorkerRuntimeUpdate != revisionBefore {
		t.Fatalf("expected worker runtime metadata not to change snapshot cache revision, got %s then %s", revisionBefore, revisionAfterWorkerRuntimeUpdate)
	}

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	revisionAfterZoneChange, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok {
		t.Fatal("expected cache revision after zone change")
	}
	if revisionAfterZoneChange == revisionBefore {
		t.Fatalf("expected DNS content change to update cache revision, got %s", revisionAfterZoneChange)
	}
}

func TestAuthoritativeDNSSnapshotCacheRevisionUsesEventDrivenStaticMutation(t *testing.T) {
	setupServiceTestDB(t)
	resetAuthoritativeDNSSnapshotCache()
	t.Cleanup(resetAuthoritativeDNSSnapshotCache)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	initial, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || initial == "" {
		t.Fatalf("expected initial cache revision, got %q ok=%v", initial, ok)
	}

	var queryCount atomic.Int32
	const callbackName = "dushengcdn:test_authoritative_dns_snapshot_revision_cache_queries"
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		queryCount.Add(1)
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	cached, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || cached != initial {
		t.Fatalf("expected cached revision %q, got %q ok=%v", initial, cached, ok)
	}
	if got := queryCount.Load(); got != 0 {
		t.Fatalf("expected cached revision not to query snapshot tables, got %d queries", got)
	}

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{
		Name:  "www",
		Type:  "A",
		Value: "8.8.8.8",
	}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	queryCount.Store(0)
	changed, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || changed == "" || changed == initial {
		t.Fatalf("expected DNS record write to bump revision cache, initial=%q changed=%q ok=%v", initial, changed, ok)
	}
	if got := queryCount.Load(); got != 0 {
		t.Fatalf("expected static mutation bump not to query snapshot tables, got %d queries", got)
	}

	queryCount.Store(0)
	cachedAgain, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || cachedAgain != changed {
		t.Fatalf("expected changed revision to be cached, got %q want %q ok=%v", cachedAgain, changed, ok)
	}
	if got := queryCount.Load(); got != 0 {
		t.Fatalf("expected cached changed revision not to query snapshot tables, got %d queries", got)
	}
}

func TestAuthoritativeDNSSnapshotCacheRevisionExpiresDynamicNodeStatus(t *testing.T) {
	setupServiceTestDB(t)
	resetAgentWSHubForAuthoritativeDNSTest()
	resetAuthoritativeDNSSnapshotCache()
	t.Cleanup(func() {
		resetAgentWSHubForAuthoritativeDNSTest()
		resetAuthoritativeDNSSnapshotCache()
	})

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = 2 * time.Second
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	lastSeenAt := time.Now().UTC().Add(-1 * time.Second)
	if err := model.DB.Create(&model.Node{
		NodeID:            "node-revision-expire",
		Name:              "node-revision-expire",
		IP:                "198.51.100.10",
		PoolName:          "default",
		Weight:            100,
		PublicIPs:         `["8.8.8.8"]`,
		SchedulingEnabled: true,
		OpenrestyStatus:   OpenrestyStatusHealthy,
		LastSeenAt:        lastSeenAt,
	}).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	initial, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || initial == "" {
		t.Fatalf("expected initial revision, got %q ok=%v", initial, ok)
	}
	cached, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || cached != initial {
		t.Fatalf("expected revision to cache before threshold expires, got %q want %q ok=%v", cached, initial, ok)
	}

	time.Sleep(time.Until(lastSeenAt.Add(common.NodeOfflineThreshold)) + 50*time.Millisecond)
	changed, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || changed == "" {
		t.Fatalf("expected revision after node status expiry, got %q ok=%v", changed, ok)
	}
	if changed == initial {
		t.Fatalf("expected node offline threshold expiry to change revision, got %s", changed)
	}
}

func TestAuthoritativeDNSSnapshotCacheRevisionTracksAgentWSConnections(t *testing.T) {
	setupServiceTestDB(t)
	resetAgentWSHubForAuthoritativeDNSTest()
	resetAuthoritativeDNSSnapshotCache()
	t.Cleanup(func() {
		resetAgentWSHubForAuthoritativeDNSTest()
		resetAuthoritativeDNSSnapshotCache()
	})

	if err := model.DB.Create(&model.Node{
		NodeID:            "node-revision-ws",
		Name:              "node-revision-ws",
		IP:                "198.51.100.11",
		PoolName:          "default",
		Weight:            100,
		PublicIPs:         `["8.8.4.4"]`,
		SchedulingEnabled: true,
		OpenrestyStatus:   OpenrestyStatusHealthy,
	}).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	initial, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || initial == "" {
		t.Fatalf("expected initial revision, got %q ok=%v", initial, ok)
	}
	client := RegisterAgentWSClient("node-revision-ws")
	connected, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || connected == "" || connected == initial {
		t.Fatalf("expected agent ws connection to change revision, initial=%q connected=%q ok=%v", initial, connected, ok)
	}
	UnregisterAgentWSClient(client)
	disconnected, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || disconnected != initial {
		t.Fatalf("expected agent ws disconnection to restore baseline revision, got %q want %q ok=%v", disconnected, initial, ok)
	}
}

func TestAuthoritativeDNSSnapshotCacheRevisionInvalidatesOnNodeMetrics(t *testing.T) {
	setupServiceTestDB(t)
	resetAuthoritativeDNSSnapshotCache()
	t.Cleanup(resetAuthoritativeDNSSnapshotCache)

	oldFreshness := common.GSLBMetricFreshnessSeconds
	common.GSLBMetricFreshnessSeconds = 60
	t.Cleanup(func() {
		common.GSLBMetricFreshnessSeconds = oldFreshness
	})

	if err := model.DB.Create(&model.Node{
		NodeID:            "node-revision-metric",
		Name:              "node-revision-metric",
		IP:                "198.51.100.12",
		PoolName:          "default",
		Weight:            100,
		PublicIPs:         `["1.1.1.1"]`,
		SchedulingEnabled: true,
		OpenrestyStatus:   OpenrestyStatusHealthy,
		LastSeenAt:        time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	initial, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || initial == "" {
		t.Fatalf("expected initial revision, got %q ok=%v", initial, ok)
	}

	if err := (&model.NodeMetricSnapshot{
		NodeID:               "node-revision-metric",
		CapturedAt:           time.Now().UTC(),
		CPUUsagePercent:      90,
		MemoryUsedBytes:      9,
		MemoryTotalBytes:     10,
		OpenrestyConnections: 42,
	}).Insert(); err != nil {
		t.Fatalf("insert node metric snapshot: %v", err)
	}
	changed, ok := authoritativeDNSSnapshotCacheRevision()
	if !ok || changed == "" || changed == initial {
		t.Fatalf("expected node metric write to invalidate revision, initial=%q changed=%q ok=%v", initial, changed, ok)
	}
}

func resetAgentWSHubForAuthoritativeDNSTest() {
	defaultAgentWSHub.mu.Lock()
	for _, client := range defaultAgentWSHub.clients {
		if client != nil {
			client.Close()
		}
	}
	defaultAgentWSHub.clients = make(map[string]*AgentWSClient)
	defaultAgentWSHub.mu.Unlock()
}

func TestAuthoritativeDNSSnapshotCacheKeepsEntriesUntilPruned(t *testing.T) {
	resetAuthoritativeDNSSnapshotCache()
	previousMaxEntries := authoritativeDNSSnapshotCache.maxEntries
	authoritativeDNSSnapshotCache.maxEntries = 8
	t.Cleanup(func() {
		authoritativeDNSSnapshotCache.maxEntries = previousMaxEntries
		resetAuthoritativeDNSSnapshotCache()
	})

	key := "authoritative-dns:test:revision-a"
	authoritativeDNSSnapshotCache.storeSnapshot(key, &AuthoritativeDNSSnapshot{SnapshotVersion: "snapshot-a"})
	authoritativeDNSSnapshotCache.mu.Lock()
	authoritativeDNSSnapshotCache.entries[key].lastAccessAt = time.Now().Add(-24 * time.Hour)
	authoritativeDNSSnapshotCache.mu.Unlock()

	cached, ok := authoritativeDNSSnapshotCache.getSnapshot(key)
	if !ok || cached == nil || cached.SnapshotVersion != "snapshot-a" {
		t.Fatalf("expected aged revision cache entry to remain available, got cached=%+v ok=%v", cached, ok)
	}
}

func TestAuthoritativeDNSSnapshotCachePrunesLeastRecentlyUsedEntries(t *testing.T) {
	resetAuthoritativeDNSSnapshotCache()
	previousMaxEntries := authoritativeDNSSnapshotCache.maxEntries
	authoritativeDNSSnapshotCache.maxEntries = 2
	t.Cleanup(func() {
		authoritativeDNSSnapshotCache.maxEntries = previousMaxEntries
		resetAuthoritativeDNSSnapshotCache()
	})

	authoritativeDNSSnapshotCache.storeSnapshot("authoritative-dns:test:old", &AuthoritativeDNSSnapshot{SnapshotVersion: "old"})
	time.Sleep(time.Millisecond)
	authoritativeDNSSnapshotCache.storeSnapshot("authoritative-dns:test:keep", &AuthoritativeDNSSnapshot{SnapshotVersion: "keep"})
	if _, ok := authoritativeDNSSnapshotCache.getSnapshot("authoritative-dns:test:old"); !ok {
		t.Fatal("expected old entry to be available before capacity pressure")
	}
	time.Sleep(time.Millisecond)
	authoritativeDNSSnapshotCache.storeSnapshot("authoritative-dns:test:new", &AuthoritativeDNSSnapshot{SnapshotVersion: "new"})

	if _, ok := authoritativeDNSSnapshotCache.getSnapshot("authoritative-dns:test:keep"); ok {
		t.Fatal("expected least recently used entry to be pruned")
	}
	if _, ok := authoritativeDNSSnapshotCache.getSnapshot("authoritative-dns:test:old"); !ok {
		t.Fatal("expected recently accessed entry to survive pruning")
	}
	if _, ok := authoritativeDNSSnapshotCache.getSnapshot("authoritative-dns:test:new"); !ok {
		t.Fatal("expected newest entry to survive pruning")
	}
}

func TestAuthoritativeDNSWorkerTokenHashRotateRevokeAndLegacyFallback(t *testing.T) {
	setupServiceTestDB(t)

	workerView, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if workerView.Token == "" || workerView.TokenPrefix == "" {
		t.Fatalf("expected created worker to return one-time token metadata: %+v", workerView)
	}
	createdToken := workerView.Token

	workerModel, err := model.GetDNSWorkerByID(workerView.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if workerModel.Token != "" || workerModel.TokenHash == "" || workerModel.TokenPrefix != dnsWorkerTokenPrefix(createdToken) {
		t.Fatalf("expected token to be stored as hash/prefix only, got %+v", workerModel)
	}
	if workerModel.TokenHash != dnsWorkerTokenHash(createdToken) {
		t.Fatal("stored token hash does not match issued token")
	}

	authenticated, err := AuthenticateDNSWorkerToken(" " + createdToken + " ")
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken hashed token: %v", err)
	}
	if authenticated.ID != workerView.ID {
		t.Fatalf("expected token to authenticate worker %d, got %d", workerView.ID, authenticated.ID)
	}

	rotated, err := RotateAuthoritativeDNSWorkerToken(workerView.ID)
	if err != nil {
		t.Fatalf("RotateAuthoritativeDNSWorkerToken: %v", err)
	}
	if rotated.Token == "" || rotated.Token == createdToken || rotated.TokenPrefix != dnsWorkerTokenPrefix(rotated.Token) {
		t.Fatalf("unexpected rotated token view: %+v", rotated)
	}
	if _, err := AuthenticateDNSWorkerToken(createdToken); err == nil {
		t.Fatal("expected original token to stop authenticating after rotation")
	}
	if _, err := AuthenticateDNSWorkerToken(rotated.Token); err != nil {
		t.Fatalf("expected rotated token to authenticate: %v", err)
	}

	revoked, err := RevokeAuthoritativeDNSWorkerToken(workerView.ID)
	if err != nil {
		t.Fatalf("RevokeAuthoritativeDNSWorkerToken: %v", err)
	}
	if revoked.Token != "" || revoked.TokenPrefix != "" || revoked.TokenRevokedAt == nil {
		t.Fatalf("unexpected revoked token view: %+v", revoked)
	}
	if _, err := AuthenticateDNSWorkerToken(rotated.Token); err == nil {
		t.Fatal("expected revoked token to be rejected")
	}

	legacyToken := "legacy-plaintext-token"
	legacyWorker := &model.DNSWorker{
		WorkerID:      "dns-legacy-token",
		Name:          "legacy",
		Token:         legacyToken,
		TokenHash:     "",
		TokenPrefix:   "",
		PublicAddress: "ns-legacy.example.net",
		Status:        dnsWorkerStatusOffline,
	}
	if err := legacyWorker.Insert(); err != nil {
		t.Fatalf("insert legacy worker: %v", err)
	}
	authenticated, err = AuthenticateDNSWorkerToken(" " + legacyToken + " ")
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken legacy token: %v", err)
	}
	if authenticated.ID != legacyWorker.ID {
		t.Fatalf("expected legacy token to authenticate worker %d, got %d", legacyWorker.ID, authenticated.ID)
	}
	reloadedLegacy, err := model.GetDNSWorkerByID(legacyWorker.ID)
	if err != nil {
		t.Fatalf("reload legacy worker: %v", err)
	}
	if reloadedLegacy.Token != "" ||
		reloadedLegacy.TokenHash != dnsWorkerTokenHash(legacyToken) ||
		reloadedLegacy.TokenPrefix != dnsWorkerTokenPrefix(legacyToken) {
		t.Fatalf("expected legacy token to be migrated to hash/prefix, got %+v", reloadedLegacy)
	}
}

func TestAuthoritativeDNSSnapshotIncludesWorkerPolicyOptions(t *testing.T) {
	setupServiceTestDB(t)

	oldQueryLimit := common.AuthoritativeDNSWorkerQueryRateLimit
	oldResponseLimit := common.AuthoritativeDNSWorkerResponseRateLimit
	oldUDPSize := common.AuthoritativeDNSWorkerUDPResponseSize
	oldECSEnabled := common.AuthoritativeDNSWorkerECSEnabled
	oldECSIPv4 := common.AuthoritativeDNSWorkerECSIPv4Prefix
	oldECSIPv6 := common.AuthoritativeDNSWorkerECSIPv6Prefix
	common.AuthoritativeDNSWorkerQueryRateLimit = 321
	common.AuthoritativeDNSWorkerResponseRateLimit = 45
	common.AuthoritativeDNSWorkerUDPResponseSize = 1400
	common.AuthoritativeDNSWorkerECSEnabled = false
	common.AuthoritativeDNSWorkerECSIPv4Prefix = 20
	common.AuthoritativeDNSWorkerECSIPv6Prefix = 60
	t.Cleanup(func() {
		common.AuthoritativeDNSWorkerQueryRateLimit = oldQueryLimit
		common.AuthoritativeDNSWorkerResponseRateLimit = oldResponseLimit
		common.AuthoritativeDNSWorkerUDPResponseSize = oldUDPSize
		common.AuthoritativeDNSWorkerECSEnabled = oldECSEnabled
		common.AuthoritativeDNSWorkerECSIPv4Prefix = oldECSIPv4
		common.AuthoritativeDNSWorkerECSIPv6Prefix = oldECSIPv6
	})

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if snapshot.WorkerPolicy.QueryRateLimit != 321 ||
		snapshot.WorkerPolicy.ResponseRateLimit != 45 ||
		snapshot.WorkerPolicy.UDPResponseSize != 1400 ||
		snapshot.WorkerPolicy.ECSEnabled ||
		snapshot.WorkerPolicy.ECSIPv4Prefix != 20 ||
		snapshot.WorkerPolicy.ECSIPv6Prefix != 60 {
		t.Fatalf("unexpected authoritative worker policy: %+v", snapshot.WorkerPolicy)
	}
	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	if workerSnapshot.WorkerPolicy != snapshot.WorkerPolicy {
		t.Fatalf("expected worker snapshot policy to match, got %+v vs %+v", workerSnapshot.WorkerPolicy, snapshot.WorkerPolicy)
	}
}

func TestAuthoritativeDNSSECEnableCreatesEncryptedKeysAndSnapshot(t *testing.T) {
	setupServiceTestDB(t)
	t.Setenv("DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY", "test-dnssec-secret")

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	view, err := EnableAuthoritativeDNSSEC(zone.ID, DNSSECEnableInput{})
	if err != nil {
		t.Fatalf("EnableAuthoritativeDNSSEC: %v", err)
	}
	if !view.Enabled || view.DenialMode != "nsec" || len(view.Keys) != 2 || len(view.DSRecords) != 1 {
		t.Fatalf("unexpected DNSSEC view: %+v", view)
	}

	keys, err := model.ListDNSSECKeysByZoneID(zone.ID)
	if err != nil {
		t.Fatalf("ListDNSSECKeysByZoneID: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected two DNSSEC keys, got %+v", keys)
	}
	for _, key := range keys {
		if key.EncryptedPrivateKey == "" || strings.Contains(key.EncryptedPrivateKey, "Private-key-format") {
			t.Fatalf("expected encrypted private key envelope, got %+v", key)
		}
		if key.PublicKey == "" || key.KeyTag == 0 {
			t.Fatalf("expected public DNSSEC key metadata, got %+v", key)
		}
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if len(snapshot.Zones) != 1 || !snapshot.Zones[0].DNSSEC.Enabled || len(snapshot.Zones[0].DNSSECKeys) != 2 {
		t.Fatalf("expected DNSSEC keys in snapshot, got %+v", snapshot.Zones)
	}
	for _, key := range snapshot.Zones[0].DNSSECKeys {
		if key.EncryptedPrivateKey == "" || strings.Contains(key.EncryptedPrivateKey, "Private-key-format") {
			t.Fatalf("snapshot leaked plaintext DNSSEC key: %+v", key)
		}
	}
}

func TestAuthoritativeDNSSnapshotFiltersZonesRoutesAndStatesForAssignedWorker(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	assignedZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "assigned.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone assigned: %v", err)
	}
	openZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "open.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone open: %v", err)
	}
	otherZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "other.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone other: %v", err)
	}
	for _, zone := range []*DNSZoneView{assignedZone, openZone, otherZone} {
		if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
			t.Fatalf("CreateAuthoritativeDNSRecord %s: %v", zone.Name, err)
		}
	}

	workerA, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-a"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker A: %v", err)
	}
	workerB, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-b"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker B: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(assignedZone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{workerA.ID}}); err != nil {
		t.Fatalf("assign worker A: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(otherZone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{workerB.ID}}); err != nil {
		t.Fatalf("assign worker B: %v", err)
	}

	now := time.Now().UTC()
	if err := (&model.Node{
		NodeID:          "node-hk",
		Name:            "hk",
		IP:              "8.8.4.4",
		PoolName:        "hk",
		PublicIPs:       `["8.8.4.4"]`,
		Weight:          100,
		AgentToken:      "token-hk-assigned",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := (&model.Node{
		NodeID:          "node-eu",
		Name:            "eu",
		IP:              "1.1.1.1",
		PoolName:        "eu",
		PublicIPs:       `["1.1.1.1"]`,
		Weight:          100,
		AgentToken:      "token-eu-other",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert EU node: %v", err)
	}

	assignedRoute := seedAuthoritativeSnapshotFilterRoute(t, assignedZone.ID, "www.assigned.example.com")
	openRoute := seedAuthoritativeSnapshotFilterRoute(t, openZone.ID, "www.open.example.com")
	otherRoute := seedAuthoritativeSnapshotFilterRoute(t, otherZone.ID, "www.other.example.com")
	otherRoute.NodePool = "eu"
	otherRoute.GSLBPolicy = mustJSON(t, defaultGSLBPolicy("eu", 1, "weighted", 30))
	if err := otherRoute.Update(); err != nil {
		t.Fatalf("update other route pool: %v", err)
	}
	for _, route := range []*model.ProxyRoute{assignedRoute, openRoute, otherRoute} {
		changedAt := now.Add(-time.Duration(route.ID) * time.Second)
		if err := model.DB.Create(&model.GSLBSchedulingState{
			ProxyRouteID:    route.ID,
			DNSRecordType:   "A",
			ScopeKey:        "global",
			SelectedTargets: `["8.8.4.4"]`,
			DesiredTargets:  `["8.8.4.4"]`,
			LastChangedAt:   &changedAt,
			LastEvaluatedAt: &changedAt,
		}).Error; err != nil {
			t.Fatalf("insert scheduling state for route %d: %v", route.ID, err)
		}
	}

	workerModel, err := model.GetDNSWorkerByID(workerA.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	snapshot, err := GetAuthoritativeDNSSnapshot(workerModel)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}

	gotZones := snapshotZoneNames(snapshot.Zones)
	if !sameStringSet(gotZones, []string{"assigned.example.com"}) {
		t.Fatalf("expected only explicitly assigned zones, got %v", gotZones)
	}
	gotRoutes := snapshotRouteDomains(snapshot.Routes)
	if !sameStringSet(gotRoutes, []string{"www.assigned.example.com"}) {
		t.Fatalf("expected routes only for explicitly assigned zones, got %v", gotRoutes)
	}
	gotStateRoutes := snapshotStateRouteIDs(snapshot.SchedulingStates)
	if !sameUintSet(gotStateRoutes, []uint{assignedRoute.ID}) {
		t.Fatalf("expected scheduling states only for explicitly assigned routes, got %v", gotStateRoutes)
	}
	gotNodes := snapshotNodeIDs(snapshot.Nodes)
	if !sameStringSet(gotNodes, []string{"node-hk"}) {
		t.Fatalf("expected nodes only for allowed route pools, got %v", gotNodes)
	}

	oldAllowUnassigned := common.AuthoritativeDNSWorkerAllowUnassignedZones
	common.AuthoritativeDNSWorkerAllowUnassignedZones = true
	t.Cleanup(func() {
		common.AuthoritativeDNSWorkerAllowUnassignedZones = oldAllowUnassigned
	})
	compatSnapshot, err := GetAuthoritativeDNSSnapshot(workerModel)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot compat: %v", err)
	}
	if !sameStringSet(snapshotZoneNames(compatSnapshot.Zones), []string{"assigned.example.com", "open.example.com"}) {
		t.Fatalf("expected compatibility switch to include unassigned zones, got %v", snapshotZoneNames(compatSnapshot.Zones))
	}
	if !sameUintSet(snapshotStateRouteIDs(compatSnapshot.SchedulingStates), []uint{assignedRoute.ID, openRoute.ID}) {
		t.Fatalf("expected compatibility switch to include unassigned route states, got %v", snapshotStateRouteIDs(compatSnapshot.SchedulingStates))
	}
}

func TestAuthoritativeDNSSnapshotAllowsLegacyUnassignedZonesWhenNoAssignmentsExist(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "legacy.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := seedAuthoritativeSnapshotFilterRoute(t, zone.ID, "www.legacy.example.com")
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-legacy"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(workerModel)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if !sameStringSet(snapshotZoneNames(snapshot.Zones), []string{"legacy.example.com"}) {
		t.Fatalf("expected legacy unassigned zone to remain visible, got %v", snapshotZoneNames(snapshot.Zones))
	}
	if !sameStringSet(snapshotRouteDomains(snapshot.Routes), []string{"www.legacy.example.com"}) {
		t.Fatalf("expected legacy unassigned route to remain visible, got %v", snapshotRouteDomains(snapshot.Routes))
	}

	windowStart := time.Now().UTC().Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zone.ID,
				ProxyRouteID:  route.ID,
				QName:         "www.legacy.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    3,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	var count int64
	if err := model.DB.Model(&model.DNSQueryRollup{}).Where("worker_id = ?", worker.WorkerID).Count(&count).Error; err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected legacy unassigned rollup to persist, got %d", count)
	}
}

func TestAuthoritativeDNSSnapshotPropagatesAssignmentQueryError(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}

	const callbackName = "dushengcdn:test_dns_zone_worker_assignment_query_error"
	forcedErr := errors.New("forced assignment query error")
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "dns_zone_worker_assignments" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "dns_zone_worker_assignments") ||
			strings.Contains(db.Statement.SQL.String(), "dns_zone_worker_assignments") {
			db.AddError(forcedErr)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	if _, err = GetAuthoritativeDNSSnapshot(workerModel); !errors.Is(err, forcedErr) {
		t.Fatalf("expected assignment query error to propagate, got %v", err)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if reloaded.LastSnapshotVersion != "" || reloaded.LastSnapshotAt != nil {
		t.Fatalf("failed snapshot should not record snapshot pull metadata, got %+v", reloaded)
	}
}

func TestAuthoritativeDNSZoneCreationAssignsExistingWorkers(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	assignments, err := GetAuthoritativeDNSZoneWorkerAssignments(zone.ID)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSZoneWorkerAssignments: %v", err)
	}
	if !sameUintSet(assignments.WorkerIDs, []uint{worker.ID}) {
		t.Fatalf("expected new zone to be assigned to existing workers, got %+v", assignments.WorkerIDs)
	}
}

func TestAuthoritativeDNSZoneWorkerAssignmentsExpandEmptyInputToAllWorkers(t *testing.T) {
	setupServiceTestDB(t)

	workerA, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-a"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker A: %v", err)
	}
	workerB, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-b"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker B: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{workerA.ID}}); err != nil {
		t.Fatalf("assign worker A: %v", err)
	}

	assignments, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{}})
	if err != nil {
		t.Fatalf("UpdateAuthoritativeDNSZoneWorkerAssignments: %v", err)
	}
	if !sameUintSet(assignments.WorkerIDs, []uint{workerA.ID, workerB.ID}) {
		t.Fatalf("expected empty input to assign all workers, got %+v", assignments.WorkerIDs)
	}
}

func TestDeleteAuthoritativeDNSZoneRollsBackRecordsWhenZoneDeleteFails(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}

	const callbackName = "dushengcdn:test_dns_zone_delete_error"
	forcedErr := errors.New("forced zone delete error")
	deleteCallback := model.DB.Callback().Delete()
	if err := deleteCallback.After("gorm:delete").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "dns_zones" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "dns_zones") ||
			strings.Contains(db.Statement.SQL.String(), "dns_zones") {
			db.AddError(forcedErr)
		}
	}); err != nil {
		t.Fatalf("register delete callback: %v", err)
	}
	t.Cleanup(func() {
		_ = deleteCallback.Remove(callbackName)
	})

	if err := DeleteAuthoritativeDNSZone(zone.ID); !errors.Is(err, forcedErr) {
		t.Fatalf("expected forced zone delete error, got %v", err)
	}
	if _, err := model.GetDNSZoneByID(zone.ID); err != nil {
		t.Fatalf("zone should remain after rollback: %v", err)
	}
	records, err := model.ListDNSRecordsByZoneID(zone.ID)
	if err != nil {
		t.Fatalf("ListDNSRecordsByZoneID: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record delete should roll back with zone delete failure, got %+v", records)
	}
}

func TestAuthoritativeDNSRejectsWritesAndSnapshotWhenLicenseExpires(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	activeNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := activeNow.Add(24 * time.Hour)
	restoreNow := setCommercialLicenseNowForTest(t, activeNow)
	defer restoreNow()

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-auth-expiry",
		CustomerName: "Auth DNS Ltd.",
		Plan:         "business",
		Features:     []string{CommercialFeatureAuthoritativeDNS},
		ExpiresAt:    &expiresAt,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	record, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	workerView, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	worker, err := AuthenticateDNSWorkerToken(workerView.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}

	commercialLicenseNow = func() time.Time {
		return expiresAt.Add(time.Second)
	}

	if _, err := UpdateAuthoritativeDNSZone(zone.ID, DNSZoneInput{Name: "example.com", DefaultTTL: 600}); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block zone update, got %v", err)
	}
	if _, err := UpdateAuthoritativeDNSRecord(record.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.11"}); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block record update, got %v", err)
	}
	if err := DeleteAuthoritativeDNSRecord(record.ID); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block record delete, got %v", err)
	}
	if _, err := RecordDNSWorkerHeartbeat(worker, DNSWorkerHeartbeatInput{Status: dnsWorkerStatusOnline}); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block worker heartbeat, got %v", err)
	}
	if _, err := GetAuthoritativeDNSSnapshot(worker); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block DNS snapshot, got %v", err)
	}
	if err := DeleteAuthoritativeDNSWorker(worker.ID); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block worker delete, got %v", err)
	}
	if err := DeleteAuthoritativeDNSZone(zone.ID); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expired license to block zone delete, got %v", err)
	}
}

func TestAuthoritativeDNSSnapshotReusesTargetSelectionNodeSnapshot(t *testing.T) {
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
	now := time.Now().UTC()
	if err := (&model.Node{
		NodeID:            "node-hk",
		Name:              "hk",
		IP:                "8.8.4.4",
		PoolName:          "hk",
		PublicIPs:         `["8.8.4.4"]`,
		Weight:            100,
		SchedulingEnabled: true,
		AgentToken:        "token-hk",
		AgentVersion:      "dev",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
	}).Insert(); err != nil {
		t.Fatalf("insert hk node: %v", err)
	}
	for _, domain := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		route := &model.ProxyRoute{
			SiteName:        "edge-" + strings.TrimSuffix(domain, ".example.com"),
			Domain:          domain,
			Domains:         mustJSON(t, []string{domain}),
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
			DNSTTL:          60,
		}
		if err := route.Insert(); err != nil {
			t.Fatalf("insert route %s: %v", domain, err)
		}
	}

	var listNodesCalls atomic.Int64
	snapshot, err := getAuthoritativeDNSSnapshotWithQueries(nil, gslbDNSSchedulingDataQueries{
		ListNodes: func() ([]*model.Node, error) {
			listNodesCalls.Add(1)
			return model.ListNodes()
		},
	})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if len(snapshot.Routes) != 3 || len(snapshot.Nodes) != 1 {
		t.Fatalf("unexpected snapshot size: routes=%+v nodes=%+v", snapshot.Routes, snapshot.Nodes)
	}
	for _, route := range snapshot.Routes {
		if len(route.CurrentTargets) != 1 || route.CurrentTargets[0] != "8.8.4.4" || route.TargetError != "" {
			t.Fatalf("expected route target from shared node snapshot, got %+v", route)
		}
	}
	if got := int(listNodesCalls.Load()); got != 1 {
		t.Fatalf("expected snapshot generation to load nodes once, got %d listNodes calls", got)
	}
}

func TestSelectProxyRouteDNSTargetsUsesCachedSchedulingData(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})
	now := time.Now().UTC()
	if err := (&model.Node{
		NodeID:            "node-cache",
		Name:              "node-cache",
		PoolName:          "edge",
		PublicIPs:         `["8.8.4.4"]`,
		Weight:            100,
		SchedulingEnabled: true,
		AgentToken:        "token-cache",
		AgentVersion:      "dev",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "edge-cache",
		Domain:          "cache.example.com",
		Domains:         `["cache.example.com"]`,
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "edge",
		Enabled:         true,
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          60,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	resetGSLBDNSSchedulingDataCache()
	defer resetGSLBDNSSchedulingDataCache()

	first, err := selectProxyRouteDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select first: %v", err)
	}
	if len(first.Targets) != 1 || first.Targets[0] != "8.8.4.4" {
		t.Fatalf("unexpected first selection: %+v", first)
	}
	if err := model.DB.Where("node_id = ?", "node-cache").Delete(&model.Node{}).Error; err != nil {
		t.Fatalf("delete node: %v", err)
	}
	second, err := selectProxyRouteDNSTargets(route, "A")
	if err != nil {
		t.Fatalf("select second: %v", err)
	}
	if len(second.Targets) != 1 || second.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected second selection to reuse cached scheduling data, got %+v", second)
	}
	resetGSLBDNSSchedulingDataCache()
	if _, err := selectProxyRouteDNSTargets(route, "A"); err == nil {
		t.Fatal("expected selection after cache reset to observe deleted node")
	}
}

func TestSnapshotGSLBSchedulingStatesUsesPreloadedData(t *testing.T) {
	setupServiceTestDB(t)

	route := AuthoritativeDNSSnapshotRoute{
		ID:         42,
		RecordType: "A",
	}
	lastChangedAt := time.Now().Add(-time.Minute).UTC()
	data := &gslbDNSSchedulingData{
		SchedulingStatesLoaded: true,
		SchedulingStates: map[dnsWorkerSchedulingStateKey]*model.GSLBSchedulingState{
			{routeID: 42, recordType: "A", scopeKey: "global"}: {
				ProxyRouteID:    42,
				DNSRecordType:   "A",
				ScopeKey:        "global",
				SelectedTargets: `["8.8.4.4"]`,
				DesiredTargets:  `["8.8.4.4"]`,
				LastChangedAt:   &lastChangedAt,
			},
		},
	}
	if err := model.DB.Migrator().DropTable(&model.GSLBSchedulingState{}); err != nil {
		t.Fatalf("drop gslb scheduling states: %v", err)
	}
	states, err := snapshotGSLBSchedulingStatesWithData([]AuthoritativeDNSSnapshotRoute{route}, data)
	if err != nil {
		t.Fatalf("snapshotGSLBSchedulingStatesWithData: %v", err)
	}
	if len(states) != 1 || states[0].RouteID != 42 || states[0].SelectedTargets[0] != "8.8.4.4" {
		t.Fatalf("expected preloaded scheduling state, got %+v", states)
	}
}

func TestSnapshotAuthoritativeRoutesBatchesZoneLoading(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	zoneA, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "a.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneA: %v", err)
	}
	zoneB, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "b.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneB: %v", err)
	}
	zoneC, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "c.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneC: %v", err)
	}
	rawZoneC, err := model.GetDNSZoneByID(zoneC.ID)
	if err != nil {
		t.Fatalf("GetDNSZoneByID zoneC: %v", err)
	}
	rawZoneC.Enabled = false
	if err := rawZoneC.Update(); err != nil {
		t.Fatalf("disable zoneC: %v", err)
	}

	now := time.Now().UTC()
	nodes := []*model.Node{
		{NodeID: "node-hk", Name: "hk", IP: "8.8.4.4", PoolName: "hk", PublicIPs: `["8.8.4.4"]`, Weight: 100, SchedulingEnabled: true, AgentToken: "token-hk", AgentVersion: "dev", OpenrestyStatus: OpenrestyStatusHealthy, Status: NodeStatusOnline, LastSeenAt: now},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node: %v", err)
		}
	}
	for _, fixture := range []struct {
		domain string
		zoneID uint
	}{
		{"www.a.example.com", zoneA.ID},
		{"api.a.example.com", zoneA.ID},
		{"www.b.example.com", zoneB.ID},
		{"www.c.example.com", zoneC.ID},
	} {
		route := &model.ProxyRoute{
			SiteName:        "edge-" + fixture.domain,
			Domain:          fixture.domain,
			Domains:         mustJSON(t, []string{fixture.domain}),
			OriginURL:       "https://origin.internal",
			Upstreams:       `["https://origin.internal"]`,
			NodePool:        "hk",
			Enabled:         true,
			DNSProviderMode: DNSProviderModeAuthoritative,
			DNSZoneIDRef:    &fixture.zoneID,
			DNSRecordType:   "A",
			DNSAutoTarget:   true,
			DNSTargetCount:  1,
			DNSScheduleMode: "weighted",
			DNSTTL:          60,
		}
		if err := route.Insert(); err != nil {
			t.Fatalf("insert route %s: %v", fixture.domain, err)
		}
	}

	schedulingData, err := loadGSLBDNSSchedulingDataWithQueries(true, gslbDNSSchedulingDataQueries{
		ListNodes: func() ([]*model.Node, error) {
			return model.ListNodes()
		},
	})
	if err != nil {
		t.Fatalf("loadGSLBDNSSchedulingData: %v", err)
	}
	schedulingOptions := authoritativeDNSSchedulingOptions()
	schedulingOptions.Data = schedulingData
	var listZonesCalls atomic.Int64
	routes, err := snapshotAuthoritativeRoutesWithQueries(schedulingOptions, authoritativeDNSSnapshotRouteQueries{
		ListDNSZonesByIDs: func(zoneIDs []uint) ([]*model.DNSZone, error) {
			listZonesCalls.Add(1)
			if fmt.Sprint(zoneIDs) != fmt.Sprint([]uint{zoneA.ID, zoneB.ID, zoneC.ID}) {
				t.Fatalf("unexpected batched route zone ids: %v", zoneIDs)
			}
			return model.ListDNSZonesByIDs(zoneIDs)
		},
	})
	if err != nil {
		t.Fatalf("snapshotAuthoritativeRoutes: %v", err)
	}
	if got := int(listZonesCalls.Load()); got != 1 {
		t.Fatalf("expected snapshot routes to load zones once, got %d calls", got)
	}
	if len(routes) != 3 {
		t.Fatalf("expected routes in enabled zones only, got %+v", routes)
	}
	for _, route := range routes {
		if route.ZoneID == zoneC.ID || len(route.CurrentTargets) != 1 || route.CurrentTargets[0] != "8.8.4.4" {
			t.Fatalf("unexpected snapshot route: %+v", route)
		}
	}
}

func TestListAuthoritativeDNSZonesIncludesRecordCounts(t *testing.T) {
	setupServiceTestDB(t)

	zoneA, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "a.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneA: %v", err)
	}
	zoneB, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "b.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneB: %v", err)
	}
	emptyZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "empty.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone emptyZone: %v", err)
	}

	for _, input := range []struct {
		zoneID uint
		record DNSRecordInput
	}{
		{zoneA.ID, DNSRecordInput{Name: "www", Type: "A", Value: "192.0.2.1"}},
		{zoneA.ID, DNSRecordInput{Name: "api", Type: "A", Value: "192.0.2.2"}},
		{zoneB.ID, DNSRecordInput{Name: "www", Type: "A", Value: "192.0.2.3"}},
	} {
		if _, err := CreateAuthoritativeDNSRecord(input.zoneID, input.record); err != nil {
			t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
		}
	}

	zones, err := ListAuthoritativeDNSZones()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSZones: %v", err)
	}
	viewsByID := make(map[uint]DNSZoneView, len(zones))
	for _, zone := range zones {
		viewsByID[zone.ID] = zone
		if len(zone.Records) != 0 {
			t.Fatalf("expected list view to omit hydrated records, got %+v", zone)
		}
	}
	if viewsByID[zoneA.ID].RecordCount != 2 {
		t.Fatalf("expected zoneA record count 2, got %+v", viewsByID[zoneA.ID])
	}
	if viewsByID[zoneB.ID].RecordCount != 1 {
		t.Fatalf("expected zoneB record count 1, got %+v", viewsByID[zoneB.ID])
	}
	if viewsByID[emptyZone.ID].RecordCount != 0 {
		t.Fatalf("expected empty zone record count 0, got %+v", viewsByID[emptyZone.ID])
	}

	detail, err := GetAuthoritativeDNSZone(zoneA.ID)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSZone: %v", err)
	}
	if detail.RecordCount != 2 || len(detail.Records) != 2 {
		t.Fatalf("expected detail view to include two records and count, got %+v", detail)
	}
}

func TestSnapshotDNSZonesBatchesRecordLoading(t *testing.T) {
	setupServiceTestDB(t)

	zoneA, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "a.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneA: %v", err)
	}
	zoneB, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "b.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneB: %v", err)
	}
	zoneC, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "c.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone zoneC: %v", err)
	}
	for _, input := range []struct {
		zoneID uint
		record DNSRecordInput
	}{
		{zoneA.ID, DNSRecordInput{Name: "www", Type: "A", Value: "192.0.2.1"}},
		{zoneB.ID, DNSRecordInput{Name: "www", Type: "AAAA", Value: "2001:db8::1"}},
		{zoneC.ID, DNSRecordInput{Name: "www", Type: "A", Value: "192.0.2.3"}},
	} {
		if _, err := CreateAuthoritativeDNSRecord(input.zoneID, input.record); err != nil {
			t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
		}
	}
	disabledRecord, err := CreateAuthoritativeDNSRecord(zoneA.ID, DNSRecordInput{Name: "disabled", Type: "A", Value: "192.0.2.2"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord disabled: %v", err)
	}
	rawDisabledRecord, err := model.GetDNSRecordByID(disabledRecord.ID)
	if err != nil {
		t.Fatalf("GetDNSRecordByID disabled: %v", err)
	}
	rawDisabledRecord.Enabled = false
	if err := rawDisabledRecord.Update(); err != nil {
		t.Fatalf("disable dns record: %v", err)
	}

	var listRecordsCalls atomic.Int64
	snapshotZones, err := snapshotDNSZonesWithQueries(authoritativeDNSSnapshotZoneQueries{
		ListDNSRecordsByZoneIDs: func(zoneIDs []uint) ([]*model.DNSRecord, error) {
			listRecordsCalls.Add(1)
			if fmt.Sprint(zoneIDs) != fmt.Sprint([]uint{zoneA.ID, zoneB.ID, zoneC.ID}) {
				t.Fatalf("unexpected batched zone ids: %v", zoneIDs)
			}
			return model.ListDNSRecordsByZoneIDs(zoneIDs)
		},
	})
	if err != nil {
		t.Fatalf("snapshotDNSZones: %v", err)
	}
	if got := int(listRecordsCalls.Load()); got != 1 {
		t.Fatalf("expected snapshot zones to load records once, got %d calls", got)
	}
	if len(snapshotZones) != 3 {
		t.Fatalf("expected three zones in snapshot, got %+v", snapshotZones)
	}
	recordsByZone := make(map[string][]AuthoritativeDNSSnapshotRecord, len(snapshotZones))
	for _, zone := range snapshotZones {
		recordsByZone[zone.Name] = zone.Records
	}
	if len(recordsByZone["a.example.com"]) != 1 || recordsByZone["a.example.com"][0].Name != "www.a.example.com" {
		t.Fatalf("expected enabled records for zoneA only, got %+v", recordsByZone["a.example.com"])
	}
	if len(recordsByZone["b.example.com"]) != 1 || recordsByZone["b.example.com"][0].Type != "AAAA" {
		t.Fatalf("expected zoneB AAAA record, got %+v", recordsByZone["b.example.com"])
	}
	if len(recordsByZone["c.example.com"]) != 1 || recordsByZone["c.example.com"][0].Name != "www.c.example.com" {
		t.Fatalf("expected zoneC A record, got %+v", recordsByZone["c.example.com"])
	}
}

func TestAuthoritativeDNSSnapshotClampsFutureSchedulingStateTime(t *testing.T) {
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
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
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
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	futureChangedAt := now.Add(time.Hour)
	evaluatedAt := now.Add(-30 * time.Second)
	if err := model.DB.Create(&model.GSLBSchedulingState{
		ProxyRouteID:    route.ID,
		DNSRecordType:   "A",
		ScopeKey:        "global",
		SelectedTargets: `["8.8.4.4"]`,
		DesiredTargets:  `["8.8.4.4"]`,
		LastChangedAt:   &futureChangedAt,
		LastEvaluatedAt: &evaluatedAt,
	}).Error; err != nil {
		t.Fatalf("insert future gslb state: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(authenticated)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	if len(snapshot.SchedulingStates) != 1 {
		t.Fatalf("expected one scheduling state in snapshot, got %+v", snapshot.SchedulingStates)
	}
	state := snapshot.SchedulingStates[0]
	if state.LastChangedAt == nil || !state.LastChangedAt.Equal(evaluatedAt) {
		t.Fatalf("expected snapshot scheduling time to use non-future fallback, got %+v", state)
	}
	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	if len(workerSnapshot.SchedulingStates) != 1 ||
		workerSnapshot.SchedulingStates[0].LastChangedAt == nil ||
		!workerSnapshot.SchedulingStates[0].LastChangedAt.Equal(evaluatedAt) {
		t.Fatalf("expected worker snapshot scheduling time to be clamped, got %+v", workerSnapshot.SchedulingStates)
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
		PublicAddress: "8.8.8.8:53",
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

func TestAuthoritativeDNSSnapshotVersionIgnoresRuntimeTelemetry(t *testing.T) {
	now := time.Now().UTC()
	metricCapturedAt := now.Add(-30 * time.Second)
	lastChangedAt := now.Add(-time.Minute)
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
				OpenrestyConnections: 10,
				CPUUsagePercent:      20,
				MemoryUsagePercent:   30,
				MetricCapturedAt:     &metricCapturedAt,
				DNSProbeHealthy:      true,
				DNSProbeCheckedCount: 4,
				DNSProbeHealthyCount: 4,
				DNSProbeAverageRTTMs: 12,
				DNSProbeMaxRTTMs:     18,
			},
		},
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         1,
				RecordType:      "A",
				ScopeKey:        "global",
				SelectedTargets: []string{"8.8.4.4"},
				DesiredTargets:  []string{"8.8.4.4"},
				LastChangedAt:   &lastChangedAt,
			},
		},
	}
	left, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version: %v", err)
	}

	nextMetricCapturedAt := now.Add(15 * time.Second)
	nextLastChangedAt := now.Add(5 * time.Second)
	snapshot.GeneratedAt = now.Add(time.Minute)
	snapshot.Nodes[0].LastSeenAt = now.Add(time.Minute)
	snapshot.Nodes[0].OpenrestyConnections = 2048
	snapshot.Nodes[0].CPUUsagePercent = 87
	snapshot.Nodes[0].MemoryUsagePercent = 91
	snapshot.Nodes[0].MetricCapturedAt = &nextMetricCapturedAt
	snapshot.Nodes[0].DNSProbeCheckedCount = 20
	snapshot.Nodes[0].DNSProbeHealthyCount = 19
	snapshot.Nodes[0].DNSProbeStaleCount = 1
	snapshot.Nodes[0].DNSProbeAverageRTTMs = 95
	snapshot.Nodes[0].DNSProbeMaxRTTMs = 180
	snapshot.SchedulingStates[0].DesiredTargets = []string{"1.1.1.1"}
	snapshot.SchedulingStates[0].LastChangedAt = &nextLastChangedAt
	right, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version after telemetry changes: %v", err)
	}
	if left != right {
		t.Fatalf("expected runtime telemetry not to change snapshot version, got %s and %s", left, right)
	}

	snapshot.SchedulingStates[0].SelectedTargets = []string{"1.1.1.1"}
	changed, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version after selected target change: %v", err)
	}
	if changed != left {
		t.Fatalf("expected scheduling state changes not to affect snapshot version, got %s and %s", left, changed)
	}
}

func TestAuthoritativeDNSSnapshotVersionIgnoresGSLBRoutePreviewTargets(t *testing.T) {
	snapshot := &AuthoritativeDNSSnapshot{
		Routes: []AuthoritativeDNSSnapshotRoute{
			{
				ID:             1,
				SiteName:       "edge",
				Domains:        []string{"edge.example.com"},
				ZoneID:         1,
				NodePool:       "default",
				RecordType:     "A",
				TargetCount:    1,
				ScheduleMode:   "load_aware",
				TTL:            30,
				GSLBEnabled:    true,
				GSLBPolicy:     defaultGSLBPolicy("default", 1, "load_aware", 30),
				CurrentTargets: []string{"8.8.4.4"},
			},
		},
	}
	left, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version: %v", err)
	}
	snapshot.Routes[0].CurrentTargets = []string{"1.1.1.1"}
	right, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("snapshot version after gslb preview target change: %v", err)
	}
	if left != right {
		t.Fatalf("expected GSLB preview targets not to change snapshot version, got %s and %s", left, right)
	}

	snapshot.Routes[0].GSLBEnabled = false
	nonGSLBLeft, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("non-GSLB snapshot version: %v", err)
	}
	snapshot.Routes[0].CurrentTargets = []string{"8.8.4.4"}
	nonGSLBRight, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		t.Fatalf("non-GSLB snapshot version after target change: %v", err)
	}
	if nonGSLBLeft == nonGSLBRight {
		t.Fatalf("expected non-GSLB target changes to affect snapshot version, got %s", nonGSLBRight)
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

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err == nil || !strings.Contains(err.Error(), "自动") {
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

func TestCreateAuthoritativeDNSRecordRejectsStaticCNAMEConflictsAndDuplicates(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord A: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err == nil {
		t.Fatal("expected exact duplicate A record to be rejected")
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.11"}); err != nil {
		t.Fatalf("expected same-name A record with different value to be allowed: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "CNAME", Value: "alias.example.com"}); err == nil {
		t.Fatal("expected CNAME to be rejected when same-name A records exist")
	}

	cname, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "alias", Type: "CNAME", Value: "target.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord CNAME: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "alias", Type: "TXT", Value: "metadata"}); err == nil {
		t.Fatal("expected non-CNAME record to be rejected when same-name CNAME exists")
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "alias", Type: "CNAME", Value: cname.Value}); err == nil {
		t.Fatal("expected duplicate CNAME to be rejected")
	}
}

func TestCreateAuthoritativeDNSRecordRejectsWildcardDynamicRouteConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "wildcard-site",
		Domain:          "*.example.com",
		Domains:         `["*.example.com"]`,
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

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "api", Type: "A", Value: "203.0.113.10"}); err == nil || !strings.Contains(err.Error(), "自动") {
		t.Fatalf("expected wildcard dynamic A conflict, got %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "api", Type: "CNAME", Value: "alias.example.com"}); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("expected wildcard CNAME conflict, got %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "deep.api.example.com", Type: "A", Value: "203.0.113.11"}); err != nil {
		t.Fatalf("expected deep subdomain to stay outside single-level wildcard conflict: %v", err)
	}
}

func TestAuthoritativeDNSAdvancedStaticRecordsNormalizeAndRejectInvalidRDATA(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}

	tests := []struct {
		name      string
		input     DNSRecordInput
		wantName  string
		wantType  string
		wantValue string
		wantPrio  int
	}{
		{
			name:      "CAA",
			input:     DNSRecordInput{Name: "@", Type: "caa", Value: `0 issue "letsencrypt.org"`},
			wantName:  "example.com",
			wantType:  "CAA",
			wantValue: `0 issue "letsencrypt.org"`,
		},
		{
			name:      "SRV",
			input:     DNSRecordInput{Name: "_sip._tcp.example.com", Type: "srv", Value: "5 5060 sip.example.com", Priority: 10},
			wantName:  "_sip._tcp.example.com",
			wantType:  "SRV",
			wantValue: "5 5060 sip.example.com",
			wantPrio:  10,
		},
		{
			name:      "HTTPS",
			input:     DNSRecordInput{Name: "svc", Type: "https", Value: `. alpn=h2 ipv4hint=192.0.2.1`, Priority: 1},
			wantName:  "svc.example.com",
			wantType:  "HTTPS",
			wantValue: `. alpn=h2 ipv4hint=192.0.2.1`,
			wantPrio:  1,
		},
		{
			name:      "SVCB",
			input:     DNSRecordInput{Name: "_dns", Type: "svcb", Value: `resolver.example.com alpn=dot port=853`, Priority: 2},
			wantName:  "_dns.example.com",
			wantType:  "SVCB",
			wantValue: `resolver.example.com alpn=dot port=853`,
			wantPrio:  2,
		},
		{
			name:      "TLSA",
			input:     DNSRecordInput{Name: "_443._tcp.example.com", Type: "tlsa", Value: "3 1 1 c22be239f483c08957bc106219cc2d3ac1a308dfbbdd0a365f17b9351234cf00"},
			wantName:  "_443._tcp.example.com",
			wantType:  "TLSA",
			wantValue: "3 1 1 c22be239f483c08957bc106219cc2d3ac1a308dfbbdd0a365f17b9351234cf00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := CreateAuthoritativeDNSRecord(zone.ID, tt.input)
			if err != nil {
				t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
			}
			if record.Name != tt.wantName ||
				record.Type != tt.wantType ||
				record.Value != tt.wantValue ||
				record.Priority != tt.wantPrio {
				t.Fatalf("unexpected normalized record: %+v", record)
			}
		})
	}

	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "bad", Type: "CAA", Value: `0 issue`}); err == nil {
		t.Fatal("expected invalid CAA RDATA to be rejected")
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "bad-svc", Type: "HTTPS", Value: `. ipv4hint=not-an-ip`, Priority: 1}); err == nil {
		t.Fatal("expected invalid HTTPS RDATA to be rejected")
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
	for _, name := range []string{"api", "static", "img", "download"} {
		if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: name, Type: "A", Value: "203.0.113.11"}); err != nil {
			t.Fatalf("CreateAuthoritativeDNSRecord unrelated %s: %v", name, err)
		}
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
	if err == nil || !strings.Contains(err.Error(), "静态记录") || !strings.Contains(err.Error(), "左侧「本地自建解析」") || !strings.Contains(err.Error(), "托管域名「example.com」") {
		t.Fatalf("expected static record conflict, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeRejectsWildcardStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "api", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}

	_, err = CreateProxyRoute(ProxyRouteInput{
		Domain:          "*.example.com",
		OriginURL:       "https://origin.internal",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		DNSRecordType:   "A",
		DNSTTL:          30,
	})
	if err == nil || !strings.Contains(err.Error(), "静态记录") || !strings.Contains(err.Error(), "托管域名「example.com」") {
		t.Fatalf("expected wildcard static record conflict, got %v", err)
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

func TestCreateProxyRouteAuthoritativeAllowsMissingDNSWorkerReadiness(t *testing.T) {
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
		t.Fatalf("CreateProxyRoute should allow saving before DNS response worker readiness: %v", err)
	}
	if view.DNSProviderMode != DNSProviderModeAuthoritative || !view.Enabled {
		t.Fatalf("unexpected authoritative route view: %+v", view)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "DNS 响应端") {
		t.Fatalf("expected DNS response worker readiness check to remain unhealthy, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeAllowsStaleDNSWorkerSnapshot(t *testing.T) {
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
		t.Fatalf("CreateProxyRoute should allow saving with stale DNS response worker snapshot: %v", err)
	}
	if view.DNSProviderMode != DNSProviderModeAuthoritative || !view.Enabled {
		t.Fatalf("unexpected authoritative route view: %+v", view)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "解析配置") {
		t.Fatalf("expected stale DNS response worker readiness check to remain unhealthy, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeAllowsPartialPublicWorkerStaleSnapshot(t *testing.T) {
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
		t.Fatalf("CreateProxyRoute should allow saving with partial stale DNS Worker snapshot: %v", err)
	}
	if view.DNSProviderMode != DNSProviderModeAuthoritative || !view.Enabled {
		t.Fatalf("unexpected authoritative route view: %+v", view)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "部分公网可达") {
		t.Fatalf("expected partial stale DNS Worker readiness check to remain unhealthy, got %v", err)
	}
}

func TestCreateProxyRouteAuthoritativeAllowsDivergentPublicWorkerSnapshots(t *testing.T) {
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
		t.Fatalf("CreateProxyRoute should allow saving with divergent DNS response worker snapshots: %v", err)
	}
	if view.DNSProviderMode != DNSProviderModeAuthoritative || !view.Enabled {
		t.Fatalf("unexpected authoritative route view: %+v", view)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "解析配置版本不一致") {
		t.Fatalf("expected divergent DNS response worker readiness check to remain unhealthy, got %v", err)
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

func TestUpdateProxyRouteAuthoritativeSkipsWorkerReadinessForNonDNSChange(t *testing.T) {
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
	worker := createReadyDNSWorker(t, now)

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

	staleProbeAt := now.Add(-(defaultDNSWorkerProbeMaxAge + time.Minute))
	worker.LastProbeAt = &staleProbeAt
	if err := worker.Update(); err != nil {
		t.Fatalf("stale DNS worker probe: %v", err)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "公网 UDP/TCP 53") {
		t.Fatalf("expected stale DNS worker readiness to fail, got %v", err)
	}

	powConfig := defaultPoWConfig()
	powConfig.Whitelist.PathRegexes = []string{`^/api/agent/`, `^/api/status$`}
	updated, err := UpdateProxyRoute(view.ID, ProxyRouteInput{
		SiteName:        "www.example.com",
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
		PoWEnabled:      true,
		PoWConfig:       mustJSON(t, powConfig),
	})
	if err != nil {
		t.Fatalf("UpdateProxyRoute non-DNS change should not require worker readiness: %v", err)
	}
	if updated.PoWConfig == nil || len(updated.PoWConfig.Whitelist.PathRegexes) != 2 {
		t.Fatalf("expected PoW whitelist to be saved, got %+v", updated.PoWConfig)
	}
}

func TestUpdateProxyRouteAuthoritativeAllowsDNSChangeBeforeWorkerReadiness(t *testing.T) {
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
	worker := createReadyDNSWorker(t, now)

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

	staleProbeAt := now.Add(-(defaultDNSWorkerProbeMaxAge + time.Minute))
	worker.LastProbeAt = &staleProbeAt
	if err := worker.Update(); err != nil {
		t.Fatalf("stale DNS worker probe: %v", err)
	}

	updated, err := UpdateProxyRoute(view.ID, ProxyRouteInput{
		SiteName:        "www.example.com",
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
	})
	if err != nil {
		t.Fatalf("UpdateProxyRoute should allow saving DNS changes before worker readiness: %v", err)
	}
	if updated.DNSTTL != 60 || updated.DNSProviderMode != DNSProviderModeAuthoritative {
		t.Fatalf("unexpected updated authoritative route: %+v", updated)
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err == nil || !strings.Contains(err.Error(), "公网 UDP/TCP 53") {
		t.Fatalf("expected worker readiness check to remain unhealthy after DNS change, got %v", err)
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
	if !strings.Contains(err.Error(), "诊断：匹配节点池 eu（匹配来源国家 DE） 没有节点") {
		t.Fatalf("expected DE source diagnostic to explain empty matched pool, got %v", err)
	}
}

func TestPrecheckAuthoritativeRouteDNSTargetsIncludesNodeDiagnostics(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	now := time.Now().UTC()
	if err := (&model.Node{
		NodeID:          "node-hot",
		Name:            "hot",
		IP:              "2001:4860:4860::8888",
		PoolName:        "hk",
		PublicIPs:       `["2001:4860:4860::8888"]`,
		Weight:          100,
		AgentToken:      "token-hot",
		AgentVersion:    "dev",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
		LastSeenAt:      now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "load_aware", 30)
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
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		DNSTargetCount:  1,
		DNSScheduleMode: "load_aware",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}

	_, err := precheckAuthoritativeRouteDNSTargets(route, "A")
	if err == nil {
		t.Fatal("expected no-target precheck error")
	}
	for _, fragment := range []string{
		"当前节点池/GSLB 无法返回 A 边缘 IP",
		"诊断：匹配节点池 hk",
		"节点 node-hot/hot：缺少 IPv4 公网 IP",
	} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("expected precheck error to contain %q, got %v", fragment, err)
		}
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

func TestListAuthoritativeDNSMigrationCandidatesReportsWildcardStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "api", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "wildcard-site",
		Domain:          "*.example.com",
		Domains:         `["*.example.com"]`,
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
		t.Fatalf("expected wildcard candidate to be blocked by static record conflict: %+v", candidate)
	}
	if candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID || candidate.PublicReachableWorkerCount != 1 || candidate.ReadyWorkerCount != 1 {
		t.Fatalf("unexpected candidate metadata: %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "静态记录") {
		t.Fatalf("expected wildcard static record blocker, got %+v", candidate.Blockers)
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

func TestListAuthoritativeDNSMigrationCandidatesReusesTargetPrecheckNodeSnapshot(t *testing.T) {
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
	now := time.Now().UTC()
	if err := (&model.Node{
		NodeID:            "node-hk",
		Name:              "hk",
		IP:                "8.8.4.4",
		PoolName:          "hk",
		PublicIPs:         `["8.8.4.4"]`,
		Weight:            100,
		SchedulingEnabled: true,
		AgentToken:        "token-hk",
		AgentVersion:      "dev",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
	}).Insert(); err != nil {
		t.Fatalf("insert hk node: %v", err)
	}
	for _, domain := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		route := &model.ProxyRoute{
			SiteName:        "edge-" + strings.TrimSuffix(domain, ".example.com"),
			Domain:          domain,
			Domains:         mustJSON(t, []string{domain}),
			OriginURL:       "https://origin.internal",
			Upstreams:       `["https://origin.internal"]`,
			NodePool:        "hk",
			Enabled:         true,
			DNSAutoSync:     true,
			DNSProviderMode: DNSProviderModeCloudflare,
			DNSRecordType:   "A",
			DNSAutoTarget:   true,
			DNSTargetCount:  1,
			DNSScheduleMode: "weighted",
			DNSTTL:          60,
		}
		if err := route.Insert(); err != nil {
			t.Fatalf("insert route %s: %v", domain, err)
		}
	}
	createReadyDNSWorker(t, now)

	var listNodesCalls atomic.Int64
	candidates, err := listAuthoritativeDNSMigrationCandidatesWithQueries(gslbDNSSchedulingDataQueries{
		ListNodes: func() ([]*model.Node, error) {
			listNodesCalls.Add(1)
			return model.ListNodes()
		},
	})
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("expected three migration candidates, got %+v", candidates)
	}
	for _, candidate := range candidates {
		if !candidate.Ready || candidate.MatchingZoneID == nil || *candidate.MatchingZoneID != zone.ID {
			t.Fatalf("expected ready candidate backed by shared node snapshot, got %+v", candidate)
		}
	}
	if got := int(listNodesCalls.Load()); got != 1 {
		t.Fatalf("expected migration candidates to load nodes once, got %d listNodes calls", got)
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
	if !containsStringWith(candidate.Blockers, "解析配置") {
		t.Fatalf("expected snapshot blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesClampsHistoricalFutureWorkerSnapshot(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
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

	worker := createReadyDNSWorker(t, now.Add(-(defaultDNSSnapshotMaxAge + time.Minute)))
	futureSnapshotAt := now.Add(time.Hour)
	staleUpdatedAt := now.Add(-(defaultDNSSnapshotMaxAge + time.Minute))
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", worker.ID).
		Updates(map[string]any{
			"last_snapshot_at": futureSnapshotAt,
			"updated_at":       staleUpdatedAt,
		}).Error; err != nil {
		t.Fatalf("update future worker snapshot: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready || candidate.FreshSnapshotWorkerCount != 0 || candidate.ReadyWorkerCount != 0 {
		t.Fatalf("expected future historical snapshot time to remain blocked, got %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "解析配置") {
		t.Fatalf("expected snapshot blocker, got %+v", candidate.Blockers)
	}
}

func TestListAuthoritativeDNSMigrationCandidatesClampsHistoricalFutureWorkerProbe(t *testing.T) {
	setupServiceTestDB(t)

	oldThreshold := common.NodeOfflineThreshold
	common.NodeOfflineThreshold = time.Minute
	t.Cleanup(func() {
		common.NodeOfflineThreshold = oldThreshold
	})

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
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

	worker := createReadyDNSWorker(t, now)
	futureProbeAt := now.Add(time.Hour)
	staleUpdatedAt := now.Add(-(defaultDNSWorkerProbeMaxAge + time.Minute))
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", worker.ID).
		Updates(map[string]any{
			"last_probe_at": futureProbeAt,
			"updated_at":    staleUpdatedAt,
		}).Error; err != nil {
		t.Fatalf("update future worker probe: %v", err)
	}

	candidates, err := ListAuthoritativeDNSMigrationCandidates()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSMigrationCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one migration candidate, got %+v", candidates)
	}
	candidate := candidates[0]
	if candidate.Ready || candidate.PublicReachableWorkerCount != 0 || candidate.ReadyWorkerCount != 0 {
		t.Fatalf("expected future historical probe time to remain blocked, got %+v", candidate)
	}
	if !containsStringWith(candidate.Blockers, "公网 UDP/TCP 53 探测") {
		t.Fatalf("expected public probe blocker, got %+v", candidate.Blockers)
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
	if !containsStringWith(candidate.Blockers, "解析配置版本不一致") {
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
	if view.DNSLastSyncStatus != DNSRecordSyncStatusSuccess || !strings.Contains(view.DNSLastSyncMessage, "本地自建解析") || view.DNSLastSyncedAt == nil {
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

func TestSwitchProxyRouteToAuthoritativeDNSRejectsWildcardStaticRecordConflict(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "api", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:        "wildcard-site",
		Domain:          "*.example.com",
		Domains:         `["*.example.com"]`,
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
		t.Fatalf("expected wildcard static record conflict, got %v", err)
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
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker: %v", err)
	}
	route := seedAuthoritativeSnapshotFilterRoute(t, zone.ID, "www.example.com")
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeatAt := time.Now().UTC().Truncate(time.Minute)
	view, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:                  "v1.0.0",
		Status:                   "online",
		LastSnapshotVersion:      "abc123",
		LastSnapshotAt:           &heartbeatAt,
		GeoIPEnabled:             true,
		GeoIPDatabasePath:        "/opt/dushengcdn-dns-worker/data/geoip/GeoLite2-Country.mmdb",
		ASNDatabasePath:          "/opt/dushengcdn-dns-worker/data/geoip/GeoLite2-ASN.mmdb",
		GeoIPDatabaseType:        "GeoLite2-Country",
		ASNDatabaseType:          "GeoLite2-ASN",
		GeoIPCountryEnabled:      true,
		GeoIPASNEnabled:          true,
		GeoIPOperatorEnabled:     true,
		OperatorCIDRDatabasePath: "/opt/dushengcdn-dns-worker/data/operator-cidr",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     heartbeatAt,
				WindowMinutes:   5,
				ZoneID:          zone.ID,
				ProxyRouteID:    route.ID,
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
	workerView := view.Worker
	if workerView.Token != "" {
		t.Fatal("expected heartbeat worker view to omit token")
	}
	if workerView.Status != dnsWorkerStatusOnline || workerView.Version != "v1.0.0" {
		t.Fatalf("unexpected heartbeat view: %+v", workerView)
	}
	if !workerView.GeoIPEnabled || workerView.GeoIPDatabasePath == "" {
		t.Fatalf("expected heartbeat view to include geoip status: %+v", workerView)
	}
	if !workerView.GeoIPCountryEnabled || !workerView.GeoIPASNEnabled || !workerView.GeoIPOperatorEnabled {
		t.Fatalf("expected heartbeat view to include source capabilities: %+v", workerView)
	}
	if workerView.ASNDatabasePath == "" || workerView.OperatorCIDRDatabasePath == "" {
		t.Fatalf("expected heartbeat view to include source database paths: %+v", workerView)
	}
	if workerView.LastHeartbeatAt == nil {
		t.Fatalf("expected heartbeat timestamp in view: %+v", workerView)
	}
	if workerView.LastRollupAt == nil || workerView.LastRollupCount != 42 {
		t.Fatalf("expected rollup metadata in view: %+v", workerView)
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

func TestDNSWorkerSchedulingTargetsReusePreloadedNodes(t *testing.T) {
	setupServiceTestDB(t)

	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	routeA := seedAuthoritativeSnapshotFilterRoute(t, zone.ID, "a.example.com")
	routeB := seedAuthoritativeSnapshotFilterRoute(t, zone.ID, "b.example.com")
	now := time.Now().UTC()
	nodes := []*model.Node{
		{
			NodeID:            "node-a",
			Name:              "node-a",
			PoolName:          "hk",
			PublicIPs:         `["8.8.4.4"]`,
			SchedulingEnabled: true,
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now,
		},
	}
	if err := model.DB.Migrator().DropTable(&model.Node{}); err != nil {
		t.Fatalf("drop nodes: %v", err)
	}

	targetsA := allowedDNSSchedulingTargetsForRouteWithNodes(routeA, "A", nodes)
	targetsB := allowedDNSSchedulingTargetsForRouteWithNodes(routeB, "A", nodes)
	if _, ok := targetsA["8.8.4.4"]; !ok {
		t.Fatalf("expected route A to allow preloaded node target, got %+v", targetsA)
	}
	if _, ok := targetsB["8.8.4.4"]; !ok {
		t.Fatalf("expected route B to allow preloaded node target, got %+v", targetsB)
	}
	empty := allowedDNSSchedulingTargetsForRoute(nil, routeA, "A")
	if _, ok := empty["8.8.4.4"]; ok {
		t.Fatalf("expected fallback wrapper to avoid per-route node table scan after nodes table was dropped, got %+v", empty)
	}
}

func TestDNSWorkerHeartbeatDoesNotOverwriteManagerUpdateStateFromStaleModel(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	staleWorker, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID stale: %v", err)
	}
	dispatchedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if err := model.DB.Model(&model.DNSWorker{}).Where("id = ?", worker.ID).Updates(map[string]any{
		"update_requested":        true,
		"update_channel":          "preview",
		"update_tag":              "v9.9.9",
		"update_dispatch_mode":    "agent_ws",
		"update_dispatch_message": "manager dispatched",
		"update_dispatched_at":    dispatchedAt,
	}).Error; err != nil {
		t.Fatalf("simulate manager update: %v", err)
	}

	if _, err := RecordDNSWorkerHeartbeat(staleWorker, DNSWorkerHeartbeatInput{
		Status:  dnsWorkerStatusOnline,
		Version: "v1.0.0",
	}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if !reloaded.UpdateRequested ||
		reloaded.UpdateChannel != "preview" ||
		reloaded.UpdateTag != "v9.9.9" ||
		reloaded.UpdateDispatchMode != "agent_ws" ||
		reloaded.UpdateDispatchMessage != "manager dispatched" ||
		reloaded.UpdateDispatchedAt == nil ||
		!reloaded.UpdateDispatchedAt.Equal(dispatchedAt) {
		t.Fatalf("heartbeat should not overwrite manager update state, got %+v", reloaded)
	}
}

func TestRecordDNSWorkerSnapshotPullDoesNotOverwriteManagerUpdateStateFromStaleModel(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker: %v", err)
	}
	staleWorker, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID stale: %v", err)
	}
	dispatchedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if err := model.DB.Model(&model.DNSWorker{}).Where("id = ?", worker.ID).Updates(map[string]any{
		"update_requested":        true,
		"update_channel":          "preview",
		"update_tag":              "v9.9.9",
		"update_dispatch_mode":    "worker_heartbeat",
		"update_dispatch_message": "manager requested",
		"update_dispatched_at":    dispatchedAt,
	}).Error; err != nil {
		t.Fatalf("simulate manager update: %v", err)
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(staleWorker)
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSSnapshot: %v", err)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("reload worker: %v", err)
	}
	if reloaded.LastSnapshotVersion != snapshot.SnapshotVersion || reloaded.LastSnapshotAt == nil {
		t.Fatalf("expected snapshot pull metadata to update, got %+v", reloaded)
	}
	if !reloaded.UpdateRequested ||
		reloaded.UpdateChannel != "preview" ||
		reloaded.UpdateTag != "v9.9.9" ||
		reloaded.UpdateDispatchMode != "worker_heartbeat" ||
		reloaded.UpdateDispatchMessage != "manager requested" ||
		reloaded.UpdateDispatchedAt == nil ||
		!reloaded.UpdateDispatchedAt.Equal(dispatchedAt) {
		t.Fatalf("snapshot pull should not overwrite manager update state, got %+v", reloaded)
	}
}

func TestDNSWorkerHeartbeatRedactsReportedErrors(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}

	view, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:                "online",
		LastError:             `sync failed Authorization=Bearer node-token password="node-password"`,
		GeoIPLastError:        `download https://geo.example/db.mmdb?token=geo-token failed`,
		ASNLastError:          `local expected_hash = "abcdef123456"`,
		OperatorCIDRLastError: `proxy_set_header Authorization "Basic dXNlcjpwYXNz";`,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	combined := strings.Join([]string{
		view.Worker.LastError,
		view.Worker.GeoIPLastError,
		view.Worker.ASNLastError,
		view.Worker.OperatorCIDRLastError,
	}, "\n")
	for _, leaked := range []string{
		"node-token",
		"node-password",
		"geo-token",
		"abcdef123456",
		"dXNlcjpwYXNz",
	} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("expected heartbeat error fields to redact %q, got %q", leaked, combined)
		}
	}
	if !strings.Contains(combined, security.RedactedValue) {
		t.Fatalf("expected heartbeat errors to contain redaction marker, got %q", combined)
	}

	var persisted model.DNSWorker
	if err := model.DB.First(&persisted, worker.ID).Error; err != nil {
		t.Fatalf("load persisted worker: %v", err)
	}
	persistedCombined := strings.Join([]string{
		persisted.LastError,
		persisted.GeoIPLastError,
		persisted.ASNLastError,
		persisted.OperatorCIDRLastError,
	}, "\n")
	for _, leaked := range []string{"node-token", "node-password", "geo-token", "abcdef123456", "dXNlcjpwYXNz"} {
		if strings.Contains(persistedCombined, leaked) {
			t.Fatalf("expected persisted heartbeat error fields to redact %q, got %q", leaked, persistedCombined)
		}
	}
}

func TestDNSWorkerHeartbeatRejectsUnauthorizedSchedulingAndRollups(t *testing.T) {
	setupServiceTestDB(t)

	zoneA, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "a.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone A: %v", err)
	}
	zoneB, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "b.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone B: %v", err)
	}
	workerA, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-a"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker A: %v", err)
	}
	workerB, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-b"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker B: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zoneA.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{workerA.ID}}); err != nil {
		t.Fatalf("assign zone A: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zoneB.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{workerB.ID}}); err != nil {
		t.Fatalf("assign zone B: %v", err)
	}

	routeA := seedAuthoritativeSnapshotFilterRoute(t, zoneA.ID, "www.a.example.com")
	routeA.DNSRecordContent = "8.8.4.4"
	if err := routeA.Update(); err != nil {
		t.Fatalf("update route A targets: %v", err)
	}
	routeB := seedAuthoritativeSnapshotFilterRoute(t, zoneB.ID, "www.b.example.com")
	routeB.DNSRecordContent = "1.1.1.1"
	if err := routeB.Update(); err != nil {
		t.Fatalf("update route B targets: %v", err)
	}

	authenticatedA, err := AuthenticateDNSWorkerToken(workerA.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken A: %v", err)
	}
	windowStart := time.Now().UTC().Truncate(time.Minute)
	changedAt := windowStart.Add(-time.Minute)
	view, err := RecordDNSWorkerHeartbeat(authenticatedA, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         routeA.ID,
				RecordType:      "A",
				ScopeKey:        "global",
				SelectedTargets: []string{"8.8.4.4"},
				DesiredTargets:  []string{"8.8.4.4"},
				LastChangedAt:   &changedAt,
			},
			{
				RouteID:         routeA.ID,
				RecordType:      "A",
				ScopeKey:        "country:TW",
				SelectedTargets: []string{"1.1.1.1"},
				DesiredTargets:  []string{"1.1.1.1"},
				LastChangedAt:   &changedAt,
			},
			{
				RouteID:         routeB.ID,
				RecordType:      "A",
				ScopeKey:        "country:JP",
				SelectedTargets: []string{"1.1.1.1"},
				DesiredTargets:  []string{"1.1.1.1"},
				LastChangedAt:   &changedAt,
			},
		},
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zoneA.ID,
				ProxyRouteID:  routeA.ID,
				QName:         "www.a.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    5,
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zoneB.ID,
				ProxyRouteID:  routeB.ID,
				QName:         "www.b.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    7,
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        zoneA.ID,
				ProxyRouteID:  routeA.ID,
				QName:         "www.a.example.com",
				QType:         "AAAA",
				RCode:         "NOERROR",
				QueryCount:    11,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if view.Worker.LastRollupCount != 5 {
		t.Fatalf("expected heartbeat rollup metadata to count only authorized inputs, got %+v", view.Worker)
	}

	var states []model.GSLBSchedulingState
	if err := model.DB.Order("proxy_route_id asc, scope_key asc").Find(&states).Error; err != nil {
		t.Fatalf("list scheduling states: %v", err)
	}
	if len(states) != 1 || states[0].ProxyRouteID != routeA.ID || states[0].ScopeKey != "global" || states[0].SelectedTargets != `["8.8.4.4"]` {
		t.Fatalf("expected only authorized scheduling state, got %+v", states)
	}

	var rollups []model.DNSQueryRollup
	if err := model.DB.Order("proxy_route_id asc").Find(&rollups).Error; err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	if len(rollups) != 1 || rollups[0].ProxyRouteID != routeA.ID || rollups[0].ZoneID != zoneA.ID || rollups[0].QueryCount != 5 {
		t.Fatalf("expected only authorized route rollup, got %+v", rollups)
	}
}

func TestDNSWorkerHeartbeatRejectsUnassignedZoneAndGlobalRollupsByDefault(t *testing.T) {
	setupServiceTestDB(t)

	assignedZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "assigned.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone assigned: %v", err)
	}
	openZone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "open.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone open: %v", err)
	}
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-assigned"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(assignedZone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign zone: %v", err)
	}
	assignedRoute := seedAuthoritativeSnapshotFilterRoute(t, assignedZone.ID, "www.assigned.example.com")
	assignedRoute.DNSRecordContent = "8.8.4.4"
	if err := assignedRoute.Update(); err != nil {
		t.Fatalf("update assigned route: %v", err)
	}
	openRoute := seedAuthoritativeSnapshotFilterRoute(t, openZone.ID, "www.open.example.com")
	openRoute.DNSRecordContent = "1.1.1.1"
	if err := openRoute.Update(); err != nil {
		t.Fatalf("update open route: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}

	windowStart := time.Now().UTC().Truncate(time.Minute)
	changedAt := windowStart.Add(-time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		SchedulingStates: []AuthoritativeDNSSnapshotSchedulingState{
			{
				RouteID:         assignedRoute.ID,
				RecordType:      "A",
				ScopeKey:        "global",
				SelectedTargets: []string{"8.8.4.4"},
				DesiredTargets:  []string{"8.8.4.4"},
				LastChangedAt:   &changedAt,
			},
			{
				RouteID:         openRoute.ID,
				RecordType:      "A",
				ScopeKey:        "global",
				SelectedTargets: []string{"1.1.1.1"},
				DesiredTargets:  []string{"1.1.1.1"},
				LastChangedAt:   &changedAt,
			},
		},
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        assignedZone.ID,
				ProxyRouteID:  assignedRoute.ID,
				QName:         "www.assigned.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    5,
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				ZoneID:        openZone.ID,
				ProxyRouteID:  openRoute.ID,
				QName:         "www.open.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    7,
			},
			{
				WindowStart:   windowStart,
				WindowMinutes: 1,
				QName:         "global.example.com",
				QType:         "A",
				RCode:         "NOERROR",
				QueryCount:    11,
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	var states []model.GSLBSchedulingState
	if err := model.DB.Order("proxy_route_id asc").Find(&states).Error; err != nil {
		t.Fatalf("list scheduling states: %v", err)
	}
	if len(states) != 1 || states[0].ProxyRouteID != assignedRoute.ID {
		t.Fatalf("expected only assigned route scheduling state, got %+v", states)
	}
	var rollups []model.DNSQueryRollup
	if err := model.DB.Order("query_count asc").Find(&rollups).Error; err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	if len(rollups) != 1 || rollups[0].ProxyRouteID != assignedRoute.ID || rollups[0].QueryCount != 5 {
		t.Fatalf("expected only assigned route rollup, got %+v", rollups)
	}
}

func TestUpdateDNSWorkerRemarkIsReturnedInHealthSummary(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:   "ns1",
		Remark: "initial remark",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if worker.Remark != "initial remark" {
		t.Fatalf("expected initial remark in create view, got %+v", worker)
	}
	updated, err := UpdateAuthoritativeDNSWorker(worker.ID, DNSWorkerMutationInput{
		Remark: "  hk dns response  ",
	})
	if err != nil {
		t.Fatalf("UpdateAuthoritativeDNSWorker: %v", err)
	}
	if updated.Remark != "hk dns response" {
		t.Fatalf("expected trimmed remark in update view, got %+v", updated)
	}
	if _, err := UpdateAuthoritativeDNSWorker(worker.ID, DNSWorkerMutationInput{
		Remark: strings.Repeat("x", 256),
	}); err == nil {
		t.Fatal("expected overlong remark to fail")
	}
	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 24})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if len(summary.WorkerHealth.Workers) != 1 {
		t.Fatalf("expected one worker in health summary, got %+v", summary.WorkerHealth.Workers)
	}
	if summary.WorkerHealth.Workers[0].Remark != "hk dns response" {
		t.Fatalf("expected remark in health summary, got %+v", summary.WorkerHealth.Workers[0])
	}
}

func TestCreateDNSWorkerRejectsUnsafePublicAddress(t *testing.T) {
	setupServiceTestDB(t)

	tests := []string{
		"127.0.0.1:53",
		"localhost:53",
		"169.254.169.254:53",
		"100.100.100.200:53",
		"10.0.0.1:53",
		":53",
		"8.8.8.8:8053",
	}
	for _, publicAddress := range tests {
		t.Run(publicAddress, func(t *testing.T) {
			if _, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-" + strings.NewReplacer(".", "-", ":", "-", "[", "", "]", "").Replace(publicAddress), PublicAddress: publicAddress}); err == nil {
				t.Fatalf("expected unsafe public address %q to fail", publicAddress)
			}
		})
	}
}

func TestNormalizeDNSWorkerProbeAddressRejectsUnsafeResolvedHost(t *testing.T) {
	restoreDNSWorkerAddressLookupIP(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "ns1.example.net" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})

	if _, err := normalizeDNSWorkerProbeAddress(context.Background(), "ns1.example.net"); err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("expected unsafe resolved ip error, got %v", err)
	}
}

func TestDNSWorkerManualUpdateRequestIsDeliveredOnHeartbeat(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	requested, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{
		Channel: string(ReleaseChannelPreview),
	})
	if err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	if !requested.UpdateRequested || requested.UpdateChannel != string(ReleaseChannelPreview) {
		t.Fatalf("expected pending preview update request, got %+v", requested)
	}

	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if !heartbeat.Settings.UpdateNow || heartbeat.Settings.UpdateRepo == "" || heartbeat.Settings.UpdateChannel != string(ReleaseChannelPreview) {
		t.Fatalf("expected heartbeat to deliver update settings, got %+v", heartbeat.Settings)
	}
	if !heartbeat.Worker.UpdateRequested || heartbeat.Worker.UpdateChannel != string(ReleaseChannelPreview) || heartbeat.Worker.UpdateDispatchMode != "worker_heartbeat_sent" {
		t.Fatalf("expected heartbeat view to keep pending update until success is reported, got %+v", heartbeat.Worker)
	}
	if !heartbeat.Worker.UpdateSupported || heartbeat.Worker.LastUpdateSupportedAt == nil {
		t.Fatalf("expected heartbeat view to record update support, got %+v", heartbeat.Worker)
	}
	secondHeartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat second: %v", err)
	}
	if secondHeartbeat.Settings.UpdateNow {
		t.Fatalf("expected fallback heartbeat update to be delivered only once, got %+v", secondHeartbeat.Settings)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if !reloaded.UpdateRequested || reloaded.UpdateChannel != string(ReleaseChannelPreview) || reloaded.UpdateTag != "" || reloaded.UpdateDispatchMode != "worker_heartbeat_sent" {
		t.Fatalf("expected pending update to remain in database until success is reported, got %+v", reloaded)
	}
	if !reloaded.UpdateSupported || reloaded.LastUpdateSupportedAt == nil {
		t.Fatalf("expected update support to be recorded in database, got %+v", reloaded)
	}
}

func TestDNSWorkerManualUpdateRequestDispatchesViaMatchingAgentWS(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:          "node-dns-worker-host",
		Name:            "edge-dns-host",
		IP:              "8.8.4.4",
		PublicIPs:       `["8.8.4.4"]`,
		AgentToken:      "agent-token",
		AgentVersion:    "v1.0.0",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}
	wsClient := RegisterAgentWSClient(node.NodeID)
	defer UnregisterAgentWSClient(wsClient)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-agent", PublicAddress: "8.8.4.4:53"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	requested, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{})
	if err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	if requested.UpdateDispatchMode != "agent_ws" {
		t.Fatalf("expected agent ws dispatch mode, got %+v", requested)
	}
	if requested.UpdateDispatchedNodeID != node.NodeID {
		t.Fatalf("expected dispatch node id %s, got %+v", node.NodeID, requested)
	}

	select {
	case message := <-wsClient.Messages():
		if message.Type != AgentWSMessageTypeDNSWorkerUpdate {
			t.Fatalf("expected dns worker update ws message, got %s", message.Type)
		}
		payload, ok := message.Payload.(*AgentDNSWorkerUpdateRequest)
		if !ok {
			t.Fatalf("expected AgentDNSWorkerUpdateRequest payload, got %T", message.Payload)
		}
		if payload.WorkerID != worker.WorkerID || payload.Channel != "stable" || payload.Repo == "" {
			t.Fatalf("unexpected dns worker update payload: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected dns worker update websocket message")
	}
}

func TestDNSWorkerManualUpdateRequestFallsBackWhenNoMatchingAgent(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-no-agent", PublicAddress: "9.9.9.9:53"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	requested, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{})
	if err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	if requested.UpdateDispatchMode != "worker_heartbeat" {
		t.Fatalf("expected worker heartbeat fallback, got %+v", requested)
	}
}

func TestDNSWorkerManualUpdateRequestMatchesAgentByHeartbeatRemoteIP(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:          "node-dns-worker-remote-ip",
		Name:            "edge-remote-ip",
		IP:              "1.1.1.1",
		PublicIPs:       `["1.1.1.1"]`,
		AgentToken:      "agent-token",
		AgentVersion:    "v1.0.0",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}
	wsClient := RegisterAgentWSClient(node.NodeID)
	defer UnregisterAgentWSClient(wsClient)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-remote-ip"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	if _, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:   dnsWorkerStatusOnline,
		RemoteIP: "1.1.1.1",
	}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	requested, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{})
	if err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	if requested.UpdateDispatchMode != "agent_ws" || requested.UpdateDispatchedNodeID != node.NodeID {
		t.Fatalf("expected agent ws dispatch via heartbeat remote ip, got %+v", requested)
	}
	select {
	case message := <-wsClient.Messages():
		if message.Type != AgentWSMessageTypeDNSWorkerUpdate {
			t.Fatalf("expected dns worker update ws message, got %s", message.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("expected dns worker update websocket message")
	}
}

func TestDNSWorkerManualUpdateRequestWaitsForSupportedHeartbeat(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{
		Channel: string(ReleaseChannelPreview),
	}); err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version: "v1.0.0",
		Status:  dnsWorkerStatusOnline,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if heartbeat.Settings.UpdateNow {
		t.Fatalf("expected unsupported heartbeat to leave update pending, got %+v", heartbeat.Settings)
	}
	if heartbeat.Worker.UpdateSupported || heartbeat.Worker.LastUpdateSupportedAt != nil {
		t.Fatalf("expected heartbeat view to show unsupported update, got %+v", heartbeat.Worker)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if !reloaded.UpdateRequested || reloaded.UpdateChannel != string(ReleaseChannelPreview) {
		t.Fatalf("expected pending update to remain in database, got %+v", reloaded)
	}
	if reloaded.UpdateSupported || reloaded.LastUpdateSupportedAt != nil {
		t.Fatalf("expected update support to remain false in database, got %+v", reloaded)
	}
}

func TestDNSWorkerHeartbeatUpdateResultClearsPendingUpdate(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{}); err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
		UpdateResult: &DNSWorkerUpdateResultInput{
			Success: true,
			Message: "installer completed",
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if heartbeat.Settings.UpdateNow {
		t.Fatalf("expected completed update result to avoid another update delivery, got %+v", heartbeat.Settings)
	}
	if heartbeat.Worker.UpdateRequested || heartbeat.Worker.UpdateTag != "" || heartbeat.Worker.UpdateDispatchMode != "worker_result" {
		t.Fatalf("expected update request to be cleared in heartbeat view, got %+v", heartbeat.Worker)
	}
	if !strings.Contains(heartbeat.Worker.UpdateDispatchMessage, "completed") {
		t.Fatalf("expected completion message, got %q", heartbeat.Worker.UpdateDispatchMessage)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if reloaded.UpdateRequested || reloaded.UpdateTag != "" {
		t.Fatalf("expected update request to be cleared in database, got %+v", reloaded)
	}
}

func TestDNSWorkerHeartbeatUpdateFailureClearsWaitingButKeepsMessage(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{}); err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
		UpdateResult: &DNSWorkerUpdateResultInput{
			Success: false,
			Message: "github returned 504",
		},
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if heartbeat.Worker.UpdateRequested {
		t.Fatalf("expected failed update attempt to clear waiting state, got %+v", heartbeat.Worker)
	}
	if !strings.Contains(heartbeat.Worker.UpdateDispatchMessage, "failed") || !strings.Contains(heartbeat.Worker.UpdateDispatchMessage, "504") {
		t.Fatalf("expected failure message to be kept, got %q", heartbeat.Worker.UpdateDispatchMessage)
	}
}

func TestDNSWorkerHeartbeatUpdateAckClearsRequestedTag(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	dispatchAt := time.Now()
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", worker.ID).
		Updates(map[string]any{
			"update_requested":        true,
			"update_channel":          string(ReleaseChannelStable),
			"update_tag":              "v1.0.1",
			"update_dispatch_mode":    "worker_heartbeat_sent",
			"update_dispatch_message": "waiting for worker heartbeat",
			"update_dispatched_at":    dispatchAt,
		}).Error; err != nil {
		t.Fatalf("seed pending heartbeat update: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.1",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if heartbeat.Worker.UpdateRequested || heartbeat.Worker.UpdateTag != "" || heartbeat.Worker.UpdateDispatchMode != "worker_heartbeat_ack" {
		t.Fatalf("expected requested tag heartbeat to clear pending update, got %+v", heartbeat.Worker)
	}
	if heartbeat.Settings.UpdateNow {
		t.Fatalf("expected cleared heartbeat update not to be delivered again, got %+v", heartbeat.Settings)
	}
}

func TestDNSWorkerHeartbeatUpdateAckClearsManualStableAfterDelay(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	oldDispatchAt := time.Now().Add(-dnsWorkerAgentUpdateAckDelay - time.Second)
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", worker.ID).
		Updates(map[string]any{
			"update_requested":        true,
			"update_channel":          string(ReleaseChannelStable),
			"update_tag":              "",
			"update_dispatch_mode":    "worker_heartbeat_sent",
			"update_dispatch_message": "waiting for worker heartbeat",
			"update_dispatched_at":    oldDispatchAt,
		}).Error; err != nil {
		t.Fatalf("seed pending heartbeat update: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if heartbeat.Worker.UpdateRequested || heartbeat.Worker.UpdateDispatchMode != "worker_heartbeat_ack" {
		t.Fatalf("expected delayed heartbeat ack to clear manual stable update, got %+v", heartbeat.Worker)
	}
	if heartbeat.Settings.UpdateNow {
		t.Fatalf("expected cleared heartbeat update not to be delivered again, got %+v", heartbeat.Settings)
	}
}

func TestAgentDNSWorkerUpdateResultClearsPendingUpdate(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:          "node-dns-worker-host",
		Name:            "edge-dns-host",
		IP:              "8.8.4.4",
		PublicIPs:       `["8.8.4.4"]`,
		AgentToken:      "agent-token",
		AgentVersion:    "v1.0.0",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}
	wsClient := RegisterAgentWSClient(node.NodeID)
	defer UnregisterAgentWSClient(wsClient)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-agent", PublicAddress: "8.8.4.4:53"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{}); err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	select {
	case <-wsClient.Messages():
	case <-time.After(time.Second):
		t.Fatal("expected dns worker update websocket message")
	}

	freshNode, err := model.GetNodeByNodeID(node.NodeID)
	if err != nil {
		t.Fatalf("GetNodeByNodeID: %v", err)
	}
	if _, err := HeartbeatNode(freshNode, AgentNodePayload{
		IP:             "8.8.4.4",
		AgentVersion:   "v1.0.1",
		CurrentVersion: "v1.0.1",
		DNSWorkerUpdateResults: []AgentDNSWorkerUpdateResult{{
			WorkerID: worker.WorkerID,
			Success:  true,
			Message:  "installer completed",
		}},
	}); err != nil {
		t.Fatalf("HeartbeatNode: %v", err)
	}
	reloaded, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if reloaded.UpdateRequested || reloaded.UpdateTag != "" || reloaded.UpdateDispatchMode != "agent_result" {
		t.Fatalf("expected agent update result to clear pending update, got %+v", reloaded)
	}
	if !strings.Contains(reloaded.UpdateDispatchMessage, "completed") {
		t.Fatalf("expected completion message, got %q", reloaded.UpdateDispatchMessage)
	}
}

func TestAgentDNSWorkerUpdateAckWaitsBeforeCompatibilityClear(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:          "node-dns-worker-host",
		Name:            "edge-dns-host",
		IP:              "8.8.4.4",
		PublicIPs:       `["8.8.4.4"]`,
		AgentToken:      "agent-token",
		AgentVersion:    "v1.0.0",
		OpenrestyStatus: OpenrestyStatusHealthy,
		Status:          NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}
	wsClient := RegisterAgentWSClient(node.NodeID)
	defer UnregisterAgentWSClient(wsClient)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns-agent", PublicAddress: "8.8.4.4:53"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := RequestAuthoritativeDNSWorkerUpdate(worker.ID, DNSWorkerUpdateInput{}); err != nil {
		t.Fatalf("RequestAuthoritativeDNSWorkerUpdate: %v", err)
	}
	select {
	case <-wsClient.Messages():
	case <-time.After(time.Second):
		t.Fatal("expected dns worker update websocket message")
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if !heartbeat.Worker.UpdateRequested {
		t.Fatalf("expected immediate heartbeat after agent dispatch to stay pending, got %+v", heartbeat.Worker)
	}
	oldDispatchAt := time.Now().Add(-dnsWorkerAgentUpdateAckDelay - time.Second)
	if err := model.DB.Model(&model.DNSWorker{}).Where("id = ?", worker.ID).Update("update_dispatched_at", oldDispatchAt).Error; err != nil {
		t.Fatalf("backdate update_dispatched_at: %v", err)
	}
	authenticated, err = AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:         "v1.0.0",
		Status:          dnsWorkerStatusOnline,
		UpdateSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat second: %v", err)
	}
	if heartbeat.Worker.UpdateRequested || heartbeat.Worker.UpdateDispatchMode != "agent_heartbeat_ack" {
		t.Fatalf("expected delayed heartbeat ack to clear pending update, got %+v", heartbeat.Worker)
	}
}

func TestDeleteDNSWorkerDeliversUninstallOnHeartbeatAndRemovesRecord(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	supportHeartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:            "v1.0.0",
		Status:             dnsWorkerStatusOnline,
		UninstallSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat support: %v", err)
	}
	if !supportHeartbeat.Worker.UninstallSupported || supportHeartbeat.Worker.LastUninstallSupportedAt == nil {
		t.Fatalf("expected heartbeat view to record uninstall support, got %+v", supportHeartbeat.Worker)
	}
	if err := DeleteAuthoritativeDNSWorker(worker.ID); err != nil {
		t.Fatalf("DeleteAuthoritativeDNSWorker: %v", err)
	}
	workers, err := ListAuthoritativeDNSWorkers()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSWorkers: %v", err)
	}
	if len(workers) != 0 {
		t.Fatalf("expected uninstall-requested worker to be hidden, got %+v", workers)
	}
	marked, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	if !marked.UninstallRequested || marked.UninstallRequestedAt == nil {
		t.Fatalf("expected uninstall request to be persisted, got %+v", marked)
	}
	authenticated, err = AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	heartbeat, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Version:            "v1.0.0",
		Status:             dnsWorkerStatusOnline,
		UpdateSupported:    true,
		UninstallSupported: true,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if !heartbeat.Settings.UninstallNow {
		t.Fatalf("expected heartbeat to deliver uninstall command, got %+v", heartbeat.Settings)
	}
	if _, err := model.GetDNSWorkerByID(worker.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected DNS worker record to be removed after uninstall heartbeat, got err=%v", err)
	}
	if _, err := AuthenticateDNSWorkerToken(worker.Token); err == nil {
		t.Fatal("expected token to be invalid after uninstall heartbeat")
	}
}

func TestDeleteDNSWorkerRequiresUninstallSupport(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	err = DeleteAuthoritativeDNSWorker(worker.ID)
	if err == nil || !strings.Contains(err.Error(), "不支持远程卸载") {
		t.Fatalf("expected unsupported uninstall error, got %v", err)
	}
	workers, err := ListAuthoritativeDNSWorkers()
	if err != nil {
		t.Fatalf("ListAuthoritativeDNSWorkers: %v", err)
	}
	if len(workers) != 1 || workers[0].ID != worker.ID {
		t.Fatalf("expected unsupported worker to remain visible, got %+v", workers)
	}
}

func TestPersistDNSQueryRollupsBatchesInserts(t *testing.T) {
	setupServiceTestDB(t)

	const callbackName = "dushengcdn:test_dns_query_rollup_create_counter"
	var rollupCreates int64
	createCallback := model.DB.Callback().Create()
	if err := createCallback.After("gorm:create").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "dns_query_rollups" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "dns_query_rollups") ||
			strings.Contains(db.Statement.SQL.String(), "dns_query_rollups") {
			atomic.AddInt64(&rollupCreates, 1)
		}
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}
	t.Cleanup(func() {
		_ = createCallback.Remove(callbackName)
	})

	windowStart := time.Now().UTC().Add(-time.Minute).Truncate(time.Minute)
	if err := persistDNSQueryRollups("ns1", []DNSQueryRollupInput{
		{
			WindowStart:   windowStart,
			WindowMinutes: 5,
			QName:         "www.example.com",
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    10,
			TargetSummary: map[string]int64{
				"8.8.8.8":   6,
				" 8.8.8.8 ": 4,
			},
		},
		{
			WindowStart:   windowStart,
			WindowMinutes: 5,
			QName:         "api.example.com",
			QType:         "AAAA",
			RCode:         "NOERROR",
			QueryCount:    7,
			SourceScope:   "country:tw",
		},
		{
			WindowStart:   windowStart,
			WindowMinutes: 5,
			QName:         "ignored.example.com",
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    0,
		},
	}); err != nil {
		t.Fatalf("persistDNSQueryRollups: %v", err)
	}

	if got := atomic.LoadInt64(&rollupCreates); got != 1 {
		t.Fatalf("expected one batched rollup insert, got %d", got)
	}
	var rollups []model.DNSQueryRollup
	if err := model.DB.Order("q_name asc").Find(&rollups).Error; err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	if len(rollups) != 2 {
		t.Fatalf("expected two persisted rollups, got %+v", rollups)
	}
	if rollups[0].QName != "api.example.com" || rollups[0].SourceScope != "country:TW" {
		t.Fatalf("unexpected normalized scoped rollup: %+v", rollups[0])
	}
	targetSummary := decodeDNSTargetSummary(rollups[1].TargetSummary)
	if rollups[1].QName != "www.example.com" || targetSummary["8.8.8.8"] != 10 {
		t.Fatalf("unexpected aggregated target summary: raw=%s decoded=%+v", rollups[1].TargetSummary, targetSummary)
	}
}

func TestPersistDNSQueryRollupsRejectsExcessHeartbeatRollups(t *testing.T) {
	setupServiceTestDB(t)

	inputs := make([]DNSQueryRollupInput, defaultDNSMaxHeartbeatRollups+1)
	for index := range inputs {
		inputs[index] = DNSQueryRollupInput{
			QName:      "www.example.com",
			QType:      "A",
			RCode:      "NOERROR",
			QueryCount: 1,
		}
	}

	err := persistDNSQueryRollups("ns1", inputs)
	if err == nil || !strings.Contains(err.Error(), "rollups exceed limit") {
		t.Fatalf("expected rollup limit error, got %v", err)
	}
	var count int64
	if err := model.DB.Model(&model.DNSQueryRollup{}).Count(&count).Error; err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected rejected heartbeat to persist no rollups, got %d", count)
	}
}

func TestDNSWorkerHeartbeatLimitsFilteredRollupBatch(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	inputs := make([]DNSQueryRollupInput, defaultDNSMaxHeartbeatRollups+25)
	windowStart := time.Now().UTC().Add(-time.Minute).Truncate(time.Minute)
	for index := range inputs {
		inputs[index] = DNSQueryRollupInput{
			WindowStart:   windowStart.Add(-time.Duration(index) * time.Minute),
			WindowMinutes: 1,
			ZoneID:        zone.ID,
			QName:         fmt.Sprintf("www-%04d.example.com", index),
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    1,
		}
	}

	view, err := RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:  dnsWorkerStatusOnline,
		Rollups: inputs,
	})
	if err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}
	if view.Worker.LastRollupCount != defaultDNSMaxHeartbeatRollups {
		t.Fatalf("expected rollup metadata to reflect limited batch, got %+v", view.Worker)
	}
	var count int64
	if err := model.DB.Model(&model.DNSQueryRollup{}).Where("worker_id = ?", authenticated.WorkerID).Count(&count).Error; err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count != defaultDNSMaxHeartbeatRollups {
		t.Fatalf("expected heartbeat to persist limited rollup batch, got %d", count)
	}
	var newest model.DNSQueryRollup
	if err := model.DB.Where("worker_id = ?", authenticated.WorkerID).Order("q_name desc").First(&newest).Error; err != nil {
		t.Fatalf("load newest rollup: %v", err)
	}
	if newest.QName != "www-4999.example.com" {
		t.Fatalf("expected rollups beyond limit to be truncated, got last qname %q", newest.QName)
	}
}

func TestPersistDNSQueryRollupsRejectsOutOfRangeWindows(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC().Truncate(time.Minute)
	tooOld := now.Add(-dnsRollupAcceptedHistoryWindow() - time.Minute)
	futureWithinTolerance := now.Add(30 * time.Second)
	tooFuture := now.Add(defaultDNSRollupFutureTolerance + time.Minute)
	if err := persistDNSQueryRollups("ns1", []DNSQueryRollupInput{
		{
			WindowStart:   tooOld,
			WindowMinutes: 1,
			QName:         "old.example.com",
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    1,
		},
		{
			WindowStart:   futureWithinTolerance,
			WindowMinutes: 1,
			QName:         "clamped.example.com",
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    2,
		},
		{
			WindowStart:   tooFuture,
			WindowMinutes: 1,
			QName:         "future.example.com",
			QType:         "A",
			RCode:         "NOERROR",
			QueryCount:    3,
		},
	}); err != nil {
		t.Fatalf("persistDNSQueryRollups: %v", err)
	}
	var rollups []model.DNSQueryRollup
	if err := model.DB.Find(&rollups).Error; err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	if len(rollups) != 1 || rollups[0].QName != "clamped.example.com" {
		t.Fatalf("expected only in-range rollup, got %+v", rollups)
	}
	if !rollups[0].WindowStart.Equal(now) {
		t.Fatalf("expected near-future rollup to clamp to now, got %s want %s", rollups[0].WindowStart, now)
	}
}

func TestNormalizeDNSTargetSummaryKeepsTopTargets(t *testing.T) {
	values := make(map[string]int64, defaultDNSMaxRollupTargetSummary+5)
	for index := 0; index < defaultDNSMaxRollupTargetSummary+5; index++ {
		values[fmt.Sprintf("192.0.2.%d", index)] = int64(index + 1)
	}
	values[" 192.0.2.99 "] = 1000
	values[""] = 5000
	values["ignored"] = 0

	result := normalizeDNSTargetSummary(values)
	if len(result) != defaultDNSMaxRollupTargetSummary {
		t.Fatalf("expected target summary cap %d, got %d", defaultDNSMaxRollupTargetSummary, len(result))
	}
	if result["192.0.2.99"] != 1000 {
		t.Fatalf("expected normalized top target to be kept, got %+v", result)
	}
	if _, ok := result["192.0.2.0"]; ok {
		t.Fatalf("expected lowest target to be trimmed, got %+v", result)
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
	if view.Worker.LastSnapshotAt == nil || !view.Worker.LastSnapshotAt.Before(futureSnapshotAt) {
		t.Fatalf("expected future snapshot time to be clamped in view, got %+v", view.Worker.LastSnapshotAt)
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

func TestDNSWorkerViewsClampHistoricalFutureSnapshotTime(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	workerModel, err := model.GetDNSWorkerByID(worker.ID)
	if err != nil {
		t.Fatalf("GetDNSWorkerByID: %v", err)
	}
	futureSnapshotAt := time.Now().UTC().Add(time.Hour)
	fallbackSnapshotAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", workerModel.ID).
		Updates(map[string]any{
			"status":                dnsWorkerStatusOnline,
			"last_snapshot_version": "future-snapshot",
			"last_snapshot_at":      futureSnapshotAt,
			"updated_at":            fallbackSnapshotAt,
		}).Error; err != nil {
		t.Fatalf("update worker: %v", err)
	}

	snapshotSummary := buildDNSWorkerSnapshotConsistency(time.Now().UTC())
	if snapshotSummary.LatestSnapshotAt == nil || !snapshotSummary.LatestSnapshotAt.Equal(fallbackSnapshotAt) {
		t.Fatalf("expected snapshot consistency to fall back from future snapshot time, got %+v", snapshotSummary)
	}
	if len(snapshotSummary.Workers) != 1 ||
		snapshotSummary.Workers[0].LastSnapshotAt == nil ||
		!snapshotSummary.Workers[0].LastSnapshotAt.Equal(fallbackSnapshotAt) {
		t.Fatalf("expected worker snapshot view to fall back from future snapshot time, got %+v", snapshotSummary.Workers)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if len(summary.WorkerHealth.Workers) != 1 {
		t.Fatalf("expected one worker health item, got %+v", summary.WorkerHealth.Workers)
	}
	workerHealth := summary.WorkerHealth.Workers[0]
	if workerHealth.LastSnapshotAt == nil || !workerHealth.LastSnapshotAt.Equal(fallbackSnapshotAt) {
		t.Fatalf("expected worker health to fall back from future snapshot time, got %+v", workerHealth)
	}
}

func TestDNSWorkerHeartbeatNormalizesInconsistentRollupDurations(t *testing.T) {
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
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
	}
	windowStart := time.Now().UTC().Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
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
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:         "edge-site",
		Domain:           "www.example.com",
		Domains:          `["www.example.com"]`,
		OriginURL:        "https://origin.internal",
		Upstreams:        `["https://origin.internal"]`,
		NodePool:         "hk",
		Enabled:          true,
		DNSProviderMode:  DNSProviderModeAuthoritative,
		DNSZoneIDRef:     &zone.ID,
		DNSRecordType:    "A",
		DNSRecordContent: "8.8.4.4,1.1.1.1,9.9.9.9,8.8.8.8",
		DNSAutoTarget:    true,
		GSLBEnabled:      true,
		GSLBPolicy:       mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
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

func TestDNSWorkerHeartbeatPersistsLargeSchedulingStateBatch(t *testing.T) {
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
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:         "large-edge-site",
		Domain:           "www.example.com",
		Domains:          `["www.example.com"]`,
		OriginURL:        "https://origin.internal",
		Upstreams:        `["https://origin.internal"]`,
		NodePool:         "hk",
		Enabled:          true,
		DNSProviderMode:  DNSProviderModeAuthoritative,
		DNSZoneIDRef:     &zone.ID,
		DNSRecordType:    "A",
		DNSRecordContent: "8.8.8.8",
		DNSAutoTarget:    true,
		GSLBEnabled:      true,
		GSLBPolicy:       mustJSON(t, defaultGSLBPolicy("hk", 1, "weighted", 30)),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	changedAt := time.Now().UTC().Truncate(time.Second)
	states := make([]AuthoritativeDNSSnapshotSchedulingState, 0, 101)
	for index := 0; index < 101; index++ {
		states = append(states, AuthoritativeDNSSnapshotSchedulingState{
			RouteID:         route.ID,
			RecordType:      "A",
			ScopeKey:        fmt.Sprintf("country:HK|bucket:%02d", index),
			SelectedTargets: []string{"8.8.8.8"},
			DesiredTargets:  []string{"8.8.8.8"},
			LastChangedAt:   &changedAt,
		})
	}

	if _, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status:           "online",
		SchedulingStates: states,
	}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat with large scheduling state batch: %v", err)
	}

	var count int64
	if err := model.DB.Model(&model.GSLBSchedulingState{}).Where("proxy_route_id = ?", route.ID).Count(&count).Error; err != nil {
		t.Fatalf("count scheduling states: %v", err)
	}
	if count != int64(len(states)) {
		t.Fatalf("expected %d scheduling states, got %d", len(states), count)
	}
}

func TestPersistDNSWorkerSchedulingStatesBatchesExistingStateLookup(t *testing.T) {
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
		GSLBEnabled:     true,
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert route: %v", err)
	}

	existingChangedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if err := model.DB.Create(&model.GSLBSchedulingState{
		ProxyRouteID:    route.ID,
		DNSRecordType:   "A",
		ScopeKey:        "country:HK",
		SelectedTargets: `["8.8.4.4"]`,
		DesiredTargets:  `["8.8.4.4"]`,
		LastReason:      "existing",
		LastChangedAt:   &existingChangedAt,
		LastEvaluatedAt: &existingChangedAt,
	}).Error; err != nil {
		t.Fatalf("insert existing scheduling state: %v", err)
	}

	const callbackName = "dushengcdn:test_gslb_state_lookup_counter"
	var stateQueries int64
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "gslb_scheduling_states" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "gslb_scheduling_states") ||
			strings.Contains(db.Statement.SQL.String(), "gslb_scheduling_states") {
			atomic.AddInt64(&stateQueries, 1)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	olderChangedAt := existingChangedAt.Add(-time.Minute)
	newerChangedAt := existingChangedAt.Add(time.Minute)
	if err := persistDNSWorkerSchedulingStates([]AuthoritativeDNSSnapshotSchedulingState{
		{
			RouteID:         route.ID,
			RecordType:      "A",
			ScopeKey:        "country:hk",
			SelectedTargets: []string{"9.9.9.9"},
			DesiredTargets:  []string{"9.9.9.9"},
			LastChangedAt:   &olderChangedAt,
		},
		{
			RouteID:         route.ID,
			RecordType:      "A",
			ScopeKey:        "country:HK",
			SelectedTargets: []string{"1.1.1.1"},
			DesiredTargets:  []string{"1.1.1.1"},
			LastChangedAt:   &newerChangedAt,
		},
		{
			RouteID:         route.ID,
			RecordType:      "A",
			ScopeKey:        "country:TW",
			SelectedTargets: []string{"2.2.2.2"},
			DesiredTargets:  []string{"2.2.2.2"},
			LastChangedAt:   &newerChangedAt,
		},
	}); err != nil {
		t.Fatalf("persistDNSWorkerSchedulingStates: %v", err)
	}

	if got := atomic.LoadInt64(&stateQueries); got != 1 {
		t.Fatalf("expected one batched scheduling state lookup, got %d", got)
	}
	var states []model.GSLBSchedulingState
	if err := model.DB.Order("scope_key asc").Find(&states).Error; err != nil {
		t.Fatalf("list scheduling states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected two scheduling states, got %+v", states)
	}
	if states[0].ScopeKey != "country:HK" || states[0].SelectedTargets != `["1.1.1.1"]` || states[0].LastChangedAt == nil || !states[0].LastChangedAt.Equal(newerChangedAt) {
		t.Fatalf("expected newer duplicate input to win, got %+v", states[0])
	}
	if states[1].ScopeKey != "country:TW" || states[1].SelectedTargets != `["2.2.2.2"]` {
		t.Fatalf("expected second scope to be inserted, got %+v", states[1])
	}
}

func TestBuildDNSWorkerHealthSummaryReusesWorkerListForNodeProbeStats(t *testing.T) {
	setupServiceTestDB(t)

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	authenticated, err := AuthenticateDNSWorkerToken(worker.Token)
	if err != nil {
		t.Fatalf("AuthenticateDNSWorkerToken: %v", err)
	}
	now := time.Now().UTC()
	if err := model.DB.Model(&model.DNSWorker{}).
		Where("id = ?", authenticated.ID).
		Updates(map[string]any{
			"status":       dnsWorkerStatusOnline,
			"last_seen_at": now,
		}).Error; err != nil {
		t.Fatalf("mark worker online: %v", err)
	}
	if err := (&model.Node{
		NodeID:            "node-probe",
		Name:              "node-probe",
		IP:                "8.8.8.8",
		PoolName:          "default",
		PublicIPs:         `["8.8.8.8"]`,
		SchedulingEnabled: true,
		AgentToken:        "token-probe",
		AgentVersion:      "dev",
		OpenrestyStatus:   OpenrestyStatusHealthy,
		Status:            NodeStatusOnline,
		LastSeenAt:        now,
	}).Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	const callbackName = "dushengcdn:test_dns_worker_health_query_counter"
	var workerQueries int64
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		if db.Statement.Table == "dns_workers" ||
			(db.Statement.Schema != nil && db.Statement.Schema.Table == "dns_workers") ||
			strings.Contains(db.Statement.SQL.String(), "dns_workers") {
			atomic.AddInt64(&workerQueries, 1)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	view := buildDNSWorkerHealthSummary(now, nil)
	if len(view.Workers) != 1 || view.Workers[0].WorkerID != authenticated.WorkerID {
		t.Fatalf("unexpected worker health view: %+v", view)
	}
	if got := atomic.LoadInt64(&workerQueries); got != 1 {
		t.Fatalf("expected worker health summary to query dns_workers once, got %d", got)
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
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
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
				SourceCountry:   "HK",
				TargetSummary:   map[string]int64{"8.8.8.8": 64, "1.1.1.1": 16},
			},
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
				ProxyRouteID:    route.ID,
				QName:           "www.example.com",
				QType:           "A",
				RCode:           "NOERROR",
				QueryCount:      11,
				TotalDurationMs: 110,
				MaxDurationMs:   20,
				SourceScope:     "asn:45102",
				SourceCountry:   "CN",
				SourceASN:       45102,
				SourceOperator:  "cn-telecom",
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
				SourceCountry:   "DE",
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
	if summary.TotalQueries != 98 || summary.SuccessfulQueries != 91 || summary.NegativeQueries != 5 || summary.ErrorQueries != 2 {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if summary.DynamicQueries != 93 || summary.StaticQueries != 5 {
		t.Fatalf("unexpected dynamic/static totals: %+v", summary)
	}
	assertCounter(t, summary.RCodeBreakdown, "NOERROR", "NOERROR", 91)
	assertCounter(t, summary.RCodeBreakdown, "NXDOMAIN", "NXDOMAIN", 5)
	assertCounter(t, summary.RCodeBreakdown, "SERVFAIL", "SERVFAIL", 2)
	assertCounter(t, summary.TopTargets, "8.8.8.8", "8.8.8.8", 64)
	assertCounter(t, summary.TopTargets, "1.1.1.1", "1.1.1.1", 16)
	assertCounter(t, summary.WorkerBreakdown, authenticated.WorkerID, "ns1-hk", 98)
	assertCounter(t, summary.ZoneBreakdown, "1", "example.com", 98)
	assertCounter(t, summary.RouteBreakdown, "1", "edge-site", 93)
	assertCounter(t, summary.SourceScopeBreakdown, "country:HK", "country:HK", 80)
	assertCounter(t, summary.SourceScopeBreakdown, "asn:45102", "asn:45102", 11)
	assertCounter(t, summary.SourceScopeBreakdown, "country:DE", "country:DE", 5)
	assertCounter(t, summary.SourceScopeBreakdown, "global", "global", 2)
	assertCounter(t, summary.SourceCountryBreakdown, "HK", "HK", 80)
	assertCounter(t, summary.SourceCountryBreakdown, "CN", "CN", 11)
	assertCounter(t, summary.SourceCountryBreakdown, "DE", "DE", 5)
	assertCounter(t, summary.SourceASNBreakdown, "45102", "45102", 11)
	assertCounter(t, summary.SourceOperatorBreakdown, "cn-telecom", "cn-telecom", 11)
	if trendTotalQueries(summary.TrendPoints) != 98 ||
		trendTotalServfailQueries(summary.TrendPoints) != 2 ||
		trendTotalNXDomainQueries(summary.TrendPoints) != 5 ||
		trendTotalDynamicQueries(summary.TrendPoints) != 93 {
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
	if summary.WorkerHealth.AverageLatencyMs < 18.6 || summary.WorkerHealth.AverageLatencyMs > 18.7 {
		t.Fatalf("unexpected worker average latency: %+v", summary.WorkerHealth)
	}
	if summary.WorkerHealth.ErrorRatePercent < 2.04 || summary.WorkerHealth.ErrorRatePercent > 2.05 {
		t.Fatalf("unexpected worker error rate: %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 2 {
		t.Fatalf("expected two worker health items, got %+v", summary.WorkerHealth.Workers)
	}
	if summary.WorkerHealth.Workers[0].WorkerID != authenticated.WorkerID ||
		summary.WorkerHealth.Workers[0].QueryCount != 98 ||
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
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := UpdateAuthoritativeDNSZoneWorkerAssignments(zone.ID, DNSZoneWorkerAssignmentInput{WorkerIDs: []uint{worker.ID}}); err != nil {
		t.Fatalf("assign worker to zone: %v", err)
	}
	windowStart := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	_, err = RecordDNSWorkerHeartbeat(authenticated, DNSWorkerHeartbeatInput{
		Status: "online",
		Rollups: []DNSQueryRollupInput{
			{
				WindowStart:     windowStart,
				WindowMinutes:   1,
				ZoneID:          zone.ID,
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

func TestAuthoritativeDNSObservabilityLimitsHeavyCounterScans(t *testing.T) {
	setupServiceTestDB(t)

	oldLimit := dnsObservabilityHeavyCounterScanLimit
	dnsObservabilityHeavyCounterScanLimit = 2
	t.Cleanup(func() {
		dnsObservabilityHeavyCounterScanLimit = oldLimit
	})

	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	rollups := []model.DNSQueryRollup{
		{
			WindowStart:     now.Add(-3 * time.Minute),
			WindowMinutes:   1,
			WorkerID:        worker.WorkerID,
			QName:           "old.example.com",
			QType:           "A",
			RCode:           "NOERROR",
			QueryCount:      100,
			TotalDurationMs: 100,
			MaxDurationMs:   10,
			TargetSummary:   `{"192.0.2.10":100}`,
		},
		{
			WindowStart:     now.Add(-2 * time.Minute),
			WindowMinutes:   1,
			WorkerID:        worker.WorkerID,
			QName:           "newer.example.com",
			QType:           "A",
			RCode:           "NOERROR",
			QueryCount:      2,
			TotalDurationMs: 20,
			MaxDurationMs:   10,
			TargetSummary:   `{"192.0.2.20":2}`,
		},
		{
			WindowStart:     now.Add(-1 * time.Minute),
			WindowMinutes:   1,
			WorkerID:        worker.WorkerID,
			QName:           "newest.example.com",
			QType:           "A",
			RCode:           "NOERROR",
			QueryCount:      1,
			TotalDurationMs: 10,
			MaxDurationMs:   10,
			TargetSummary:   `{"192.0.2.30":1}`,
		},
	}
	for index := range rollups {
		if err := rollups[index].Insert(); err != nil {
			t.Fatalf("insert rollup %d: %v", index, err)
		}
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.TotalQueries != 103 || summary.SuccessfulQueries != 103 {
		t.Fatalf("expected totals to use full database aggregation, got %+v", summary)
	}
	assertCounter(t, summary.TopQNames, "old.example.com", "old.example.com", 100)
	assertCounter(t, summary.TopQNames, "newest.example.com", "newest.example.com", 1)
	assertCounter(t, summary.TopQNames, "newer.example.com", "newer.example.com", 2)
	assertCounter(t, summary.TopTargets, "192.0.2.30", "192.0.2.30", 1)
	assertCounter(t, summary.TopTargets, "192.0.2.20", "192.0.2.20", 2)
	assertNoCounter(t, summary.TopTargets, "192.0.2.10")
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

	restoreDNSWorkerAddressLookupIP(t, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "ns1.example.net" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
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

func TestAgentDNSProbeWorkerHealthIncludesUnreportedOnlineNodes(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
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
	if _, err = RecordDNSWorkerHeartbeat(workerModel, DNSWorkerHeartbeatInput{
		Version:             "v1.0.0",
		Status:              dnsWorkerStatusOnline,
		LastSnapshotVersion: "snapshot-a",
		LastSnapshotAt:      &now,
	}); err != nil {
		t.Fatalf("RecordDNSWorkerHeartbeat: %v", err)
	}

	nodes := []*model.Node{
		{
			NodeID:            "node-jp",
			Name:              "CLAW-JP",
			PoolName:          "日本",
			AgentToken:        "agent-token-jp",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now,
			SchedulingEnabled: true,
		},
		{
			NodeID:            "node-eu",
			Name:              "AKKO GB",
			PoolName:          "欧洲",
			AgentToken:        "agent-token-eu",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now.Add(-time.Second),
			SchedulingEnabled: true,
		},
		{
			NodeID:            "node-hk",
			Name:              "Aliyun HK",
			PoolName:          "香港",
			AgentToken:        "agent-token-hk",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now.Add(-2 * time.Second),
			SchedulingEnabled: true,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}

	results := []AgentDNSProbeResult{
		{Network: "UDP", Reachable: true, DurationMs: 10, RCode: "NOERROR", AnswerCount: 1},
		{Network: "TCP", Reachable: true, DurationMs: 20, RCode: "NOERROR", AnswerCount: 1},
	}
	for _, nodeID := range []string{"node-jp", "node-hk"} {
		persistHeartbeatObservability(nodeID, AgentNodePayload{
			DNSProbeResults: []AgentDNSProbeReport{{
				WorkerID:      worker.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results:       results,
			}},
		}, now)
	}

	summary, err := GetAuthoritativeDNSObservabilitySummary(DNSObservabilitySummaryInput{Hours: 1})
	if err != nil {
		t.Fatalf("GetAuthoritativeDNSObservabilitySummary: %v", err)
	}
	if summary.WorkerHealth.NodeProbeCheckedCount != 3 ||
		summary.WorkerHealth.NodeProbeHealthyCount != 2 ||
		summary.WorkerHealth.NodeProbeHealthyPercent < 66.6 ||
		summary.WorkerHealth.NodeProbeHealthyPercent > 66.7 {
		t.Fatalf("expected 2/3 node probe summary, got %+v", summary.WorkerHealth)
	}
	if len(summary.WorkerHealth.Workers) != 1 || len(summary.WorkerHealth.Workers[0].NodeProbes) != 3 {
		t.Fatalf("expected three node probe cards, got %+v", summary.WorkerHealth.Workers)
	}
	var unreported DNSWorkerNodeProbeView
	for _, probe := range summary.WorkerHealth.Workers[0].NodeProbes {
		if probe.NodeID == "node-eu" {
			unreported = probe
			break
		}
	}
	if unreported.NodeID == "" ||
		unreported.NodeName != "AKKO GB" ||
		unreported.PoolName != "欧洲" ||
		unreported.ProbeStatus != dnsWorkerProbeUnknown ||
		unreported.Healthy ||
		!unreported.CheckedAt.IsZero() {
		t.Fatalf("expected unreported online node to be visible as unknown, got %+v", unreported)
	}
}

func TestAgentDNSProbeFutureCheckedAtIsClampedOnPersist(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	worker, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1-hk",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	node := &model.Node{
		NodeID:            "node-hk-1",
		Name:              "hk-edge-1",
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

	futureCheckedAt := now.Add(time.Hour)
	persistHeartbeatObservability(node.NodeID, AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{{
			WorkerID:      worker.WorkerID,
			PublicAddress: "ns1.example.net",
			QueryName:     "example.com.",
			QueryType:     "SOA",
			CheckedAtUnix: futureCheckedAt.Unix(),
			Results: []AgentDNSProbeResult{
				{Network: "UDP", Reachable: true, DurationMs: 11, RCode: "NOERROR", AnswerCount: 1},
				{Network: "TCP", Reachable: true, DurationMs: 17, RCode: "NOERROR", AnswerCount: 1},
			},
		}},
	}, now)

	probes, err := model.ListDNSWorkerNodeProbes()
	if err != nil {
		t.Fatalf("ListDNSWorkerNodeProbes: %v", err)
	}
	if len(probes) != 1 {
		t.Fatalf("expected one node probe, got %+v", probes)
	}
	if probes[0].CheckedAt.After(now) {
		t.Fatalf("expected future checked_at to be clamped to report time, got %v > %v", probes[0].CheckedAt, now)
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

func TestAuthoritativeDNSGSLBSourceMatchPriorityIncludesASNAndOperator(t *testing.T) {
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Pools: []ProxyRouteGSLBPoolPolicy{
			{Name: "global", Weight: 100, Enabled: true},
			{Name: "country-cn", Weight: 100, Countries: []string{"CN"}, Enabled: true},
			{Name: "operator-telecom", Weight: 100, Operators: []string{"Telecom"}, Enabled: true},
			{Name: "asn-4134", Weight: 100, ASNs: []uint32{4134}, Enabled: true},
			{Name: "cidr-edge", Weight: 100, SourceCIDRs: []string{"203.0.113.0/24"}, Enabled: true},
		},
	}, "global", 1, "weighted", 30)
	if err != nil {
		t.Fatalf("normalize GSLB policy: %v", err)
	}

	matchedPoolNames := func(matched map[string]ProxyRouteGSLBPoolPolicy) []string {
		names := make([]string, 0, len(matched))
		for name := range matched {
			names = append(names, name)
		}
		return names
	}

	tests := []struct {
		name      string
		source    GSLBSourceContext
		wantPools []string
		wantScope string
	}{
		{
			name:      "CIDR overrides ASN operator and country",
			source:    GSLBSourceContext{IP: "203.0.113.10", ASN: 4134, Operator: "cn-telecom", Country: "CN"},
			wantPools: []string{"cidr-edge"},
			wantScope: "cidr:203.0.113.0/24",
		},
		{
			name:      "ASN overrides operator and country",
			source:    GSLBSourceContext{IP: "198.51.100.10", ASN: 4134, Operator: "cn-telecom", Country: "CN"},
			wantPools: []string{"asn-4134"},
			wantScope: "asn:4134",
		},
		{
			name:      "operator overrides country",
			source:    GSLBSourceContext{IP: "198.51.100.10", ASN: 9808, Operator: "China Telecom", Country: "CN"},
			wantPools: []string{"operator-telecom"},
			wantScope: "operator:cn-telecom",
		},
		{
			name:      "country overrides global fallback",
			source:    GSLBSourceContext{IP: "198.51.100.10", Country: "cn"},
			wantPools: []string{"country-cn"},
			wantScope: "country:CN",
		},
		{
			name:      "global fallback keeps every enabled pool",
			source:    GSLBSourceContext{},
			wantPools: []string{"global", "country-cn", "operator-telecom", "asn-4134", "cidr-edge"},
			wantScope: defaultGSLBScopeKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := matchGSLBPoolsForSource(policy.Pools, tt.source)
			if got := matchedPoolNames(matched); !sameStringSet(got, tt.wantPools) {
				t.Fatalf("expected matched pools %v, got %v", tt.wantPools, got)
			}
			if got := gslbScopeKeyForPolicy(policy, tt.source); got != tt.wantScope {
				t.Fatalf("expected source scope %q, got %q", tt.wantScope, got)
			}
		})
	}
}

func TestAuthoritativeDNSGSLBSourcePoolFallbackModeStrictVsGlobal(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	nodes := []*model.Node{
		{
			NodeID:            "node-global",
			Name:              "global",
			IP:                "8.8.4.4",
			PoolName:          "global",
			PublicIPs:         `["8.8.4.4"]`,
			Weight:            100,
			SchedulingEnabled: true,
			AgentVersion:      "dev",
			OpenrestyStatus:   OpenrestyStatusHealthy,
			Status:            NodeStatusOnline,
			LastSeenAt:        now,
		},
	}
	policy, err := normalizeGSLBPolicy(ProxyRouteGSLBPolicy{
		Strategy:    "weighted",
		TargetCount: 1,
		TTL:         30,
		Pools: []ProxyRouteGSLBPoolPolicy{
			{Name: "global", Weight: 100, Enabled: true},
			{Name: "de", Weight: 100, Countries: []string{"DE"}, Enabled: true},
		},
	}, "global", 1, "weighted", 30)
	if err != nil {
		t.Fatalf("normalize GSLB policy: %v", err)
	}
	route := &model.ProxyRoute{
		ID:              77,
		SiteName:        "edge-site",
		Domain:          "www.example.com",
		Domains:         `["www.example.com"]`,
		NodePool:        "global",
		DNSRecordType:   "A",
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
	}
	options := gslbDNSSchedulingOptions{
		Data: &gslbDNSSchedulingData{
			Nodes:         nodes,
			MetricsByNode: map[string]*model.NodeMetricSnapshot{},
		},
	}
	source := GSLBSourceContext{Country: "DE"}

	policy.SourcePoolFallbackMode = gslbSourcePoolFallbackStrict
	route.GSLBPolicy = mustJSON(t, policy)
	strictSelection, err := selectGSLBDNSTargetsWithOptions(route, "A", source, options)
	if err == nil {
		t.Fatalf("expected strict source pool mode to fail without DE targets, got %+v", strictSelection)
	}
	if strictSelection.ScopeKey != "country:DE" {
		t.Fatalf("expected strict mode to keep source scope, got %s", strictSelection.ScopeKey)
	}

	policy.SourcePoolFallbackMode = gslbSourcePoolFallbackGlobal
	route.GSLBPolicy = mustJSON(t, policy)
	fallbackSelection, err := selectGSLBDNSTargetsWithOptions(route, "A", source, options)
	if err != nil {
		t.Fatalf("expected fallback_to_global to use global pool: %v", err)
	}
	if len(fallbackSelection.Targets) != 1 || fallbackSelection.Targets[0] != "8.8.4.4" {
		t.Fatalf("unexpected fallback targets: %+v", fallbackSelection)
	}
	if fallbackSelection.ScopeKey != "country:DE" {
		t.Fatalf("expected fallback selection to retain source scope for debounce, got %s", fallbackSelection.ScopeKey)
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

	global, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "www.example.com",
		RecordType:   "A",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB global: %v", err)
	}
	if global.SourceScope != defaultGSLBScopeKey || !strings.Contains(global.Message, "未指定来源条件时使用全局作用域") {
		t.Fatalf("expected global simulation message to explain fallback scope, got %+v", global)
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

func TestSimulateAuthoritativeDNSGSLBProbeSchedulingScoresProbeQuality(t *testing.T) {
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
	workerA, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns1",
		PublicAddress: "ns1.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker ns1: %v", err)
	}
	workerB, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{
		Name:          "ns2",
		PublicAddress: "ns2.example.net",
	})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker ns2: %v", err)
	}
	now := time.Now()
	for _, node := range []*model.Node{
		{
			NodeID:          "node-weak",
			Name:            "weak",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          100,
			AgentToken:      "token-weak",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-strong",
			Name:            "strong",
			IP:              "8.8.4.4",
			PoolName:        "hk",
			PublicIPs:       `["8.8.4.4"]`,
			Weight:          100,
			AgentToken:      "token-strong",
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
	persistHeartbeatObservability("node-weak", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      workerA.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 850, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 950, RCode: "NOERROR", AnswerCount: 1},
				},
			},
			{
				WorkerID:      workerB.WorkerID,
				PublicAddress: "ns2.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: false, Error: "timeout"},
					{Network: "TCP", Reachable: false, Error: "timeout"},
				},
			},
		},
	}, now)
	persistHeartbeatObservability("node-strong", AgentNodePayload{
		DNSProbeResults: []AgentDNSProbeReport{
			{
				WorkerID:      workerA.WorkerID,
				PublicAddress: "ns1.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 15, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 25, RCode: "NOERROR", AnswerCount: 1},
				},
			},
			{
				WorkerID:      workerB.WorkerID,
				PublicAddress: "ns2.example.net",
				QueryName:     "example.com.",
				QueryType:     "SOA",
				CheckedAtUnix: now.Unix(),
				Results: []AgentDNSProbeResult{
					{Network: "UDP", Reachable: true, DurationMs: 20, RCode: "NOERROR", AnswerCount: 1},
					{Network: "TCP", Reachable: true, DurationMs: 30, RCode: "NOERROR", AnswerCount: 1},
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
		t.Fatalf("expected higher Agent probe quality target, got %+v", simulation.Targets)
	}
	strong := findSimulationNode(simulation.Nodes, "node-strong")
	weak := findSimulationNode(simulation.Nodes, "node-weak")
	if strong == nil || weak == nil || !strong.Selected || weak.Selected || !(strong.Score > weak.Score) {
		t.Fatalf("expected probe quality score to prefer strong node, strong=%+v weak=%+v", strong, weak)
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

func TestSimulateAuthoritativeDNSGSLBLoadAwareMarksMissingMetricsAsFallback(t *testing.T) {
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
			NodeID:          "node-metric",
			Name:            "metric",
			IP:              "8.8.4.4",
			PoolName:        "hk",
			PublicIPs:       `["8.8.4.4"]`,
			Weight:          100,
			AgentToken:      "token-metric",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now,
		},
		{
			NodeID:          "node-no-metric",
			Name:            "no-metric",
			IP:              "1.1.1.1",
			PoolName:        "hk",
			PublicIPs:       `["1.1.1.1"]`,
			Weight:          100,
			AgentToken:      "token-no-metric",
			AgentVersion:    "dev",
			OpenrestyStatus: OpenrestyStatusHealthy,
			Status:          NodeStatusOnline,
			LastSeenAt:      now.Add(-time.Second),
		},
	} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:               "node-metric",
		CapturedAt:           now,
		CPUUsagePercent:      30,
		MemoryUsedBytes:      30,
		MemoryTotalBytes:     100,
		OpenrestyConnections: 20,
	}).Insert(); err != nil {
		t.Fatalf("insert metric: %v", err)
	}

	policy := defaultGSLBPolicy("hk", 1, "load_aware", 30)
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
		SourceIP:     "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB: %v", err)
	}
	if len(simulation.Targets) != 1 || simulation.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected load-aware simulation to select fresh metric target, got %+v", simulation.Targets)
	}
	assertSimulationNode(t, simulation.Nodes, "node-metric", true, true, "可参与当前调度")
	assertSimulationNode(t, simulation.Nodes, "node-no-metric", true, false, "暂无新鲜负载指标，仅作为兜底候选")
	if node := findSimulationNode(simulation.Nodes, "node-no-metric"); node == nil || node.HasMetric || node.MetricCapturedAt != nil {
		t.Fatalf("expected missing metric node to stay visible without metric timestamp, got %+v", node)
	}
}

func TestSimulateAuthoritativeDNSGSLBMatchesWildcardRouteDomain(t *testing.T) {
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
	policy.Pools = []ProxyRouteGSLBPoolPolicy{
		{Name: "hk", Weight: 100, Enabled: true},
	}
	route := &model.ProxyRoute{
		SiteName:        "wildcard-site",
		Domain:          "*.example.com",
		Domains:         `["*.example.com"]`,
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
		QName:        "api.example.com",
		RecordType:   "A",
	})
	if err != nil {
		t.Fatalf("SimulateAuthoritativeDNSGSLB wildcard: %v", err)
	}
	if len(simulation.Targets) != 1 || simulation.Targets[0] != "8.8.4.4" {
		t.Fatalf("expected wildcard simulation target, got %+v", simulation.Targets)
	}

	if _, err := SimulateAuthoritativeDNSGSLB(DNSGSLBSimulationInput{
		ProxyRouteID: route.ID,
		QName:        "deep.api.example.com",
		RecordType:   "A",
	}); err == nil || !strings.Contains(err.Error(), "qname does not belong") {
		t.Fatalf("expected deep subdomain to stay outside single-level wildcard, got %v", err)
	}
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

	futureAt := now.Add(time.Hour)
	future := evaluateDNSWorkerProbeState(now, &futureAt, []DNSWorkerProbeResultView{
		{Network: "UDP", Reachable: true},
		{Network: "TCP", Reachable: true},
	})
	if !future.healthy || future.status != dnsWorkerProbeHealthy || future.ageSeconds != 0 {
		t.Fatalf("expected future server probe time to be clamped to current time, got %+v", future)
	}
}

func TestDNSWorkerNodeProbeStateClampsHistoricalFutureCheckedAt(t *testing.T) {
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	futureAt := now.Add(time.Hour)
	updatedAt := now.Add(-2 * time.Minute)
	probe := &model.DNSWorkerNodeProbe{
		CheckedAt:   futureAt,
		UpdatedAt:   updatedAt,
		CreatedAt:   updatedAt.Add(-time.Minute),
		ResultsJSON: `[{"network":"UDP","reachable":true},{"network":"TCP","reachable":true}]`,
		Healthy:     true,
	}

	state := evaluateDNSWorkerNodeProbeState(now, probe)
	if !state.healthy || state.status != dnsWorkerProbeHealthy || state.ageSeconds != int64(now.Sub(updatedAt).Seconds()) {
		t.Fatalf("expected future node probe checked_at to fall back to updated_at, got %+v", state)
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

func restoreDNSWorkerAddressLookupIP(t *testing.T, lookup func(context.Context, string) ([]net.IPAddr, error)) {
	t.Helper()
	original := dnsWorkerAddressLookupIP
	dnsWorkerAddressLookupIP = lookup
	t.Cleanup(func() {
		dnsWorkerAddressLookupIP = original
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

func seedAuthoritativeSnapshotFilterRoute(t *testing.T, zoneID uint, domain string) *model.ProxyRoute {
	t.Helper()
	policy := defaultGSLBPolicy("hk", 1, "weighted", 30)
	route := &model.ProxyRoute{
		SiteName:        domain,
		Domain:          domain,
		Domains:         mustJSON(t, []string{domain}),
		OriginURL:       "https://origin.internal",
		Upstreams:       `["https://origin.internal"]`,
		NodePool:        "hk",
		Enabled:         true,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zoneID,
		DNSRecordType:   "A",
		DNSAutoTarget:   true,
		DNSTargetCount:  1,
		DNSScheduleMode: "weighted",
		DNSTTL:          30,
		GSLBEnabled:     true,
		GSLBPolicy:      mustJSON(t, policy),
	}
	if err := route.Insert(); err != nil {
		t.Fatalf("insert authoritative snapshot route %s: %v", domain, err)
	}
	return route
}

func snapshotZoneNames(zones []AuthoritativeDNSSnapshotZone) []string {
	names := make([]string, 0, len(zones))
	for _, zone := range zones {
		names = append(names, zone.Name)
	}
	return names
}

func snapshotRouteDomains(routes []AuthoritativeDNSSnapshotRoute) []string {
	domains := make([]string, 0, len(routes))
	for _, route := range routes {
		if len(route.Domains) > 0 {
			domains = append(domains, route.Domains[0])
		}
	}
	return domains
}

func snapshotStateRouteIDs(states []AuthoritativeDNSSnapshotSchedulingState) []uint {
	routeIDs := make([]uint, 0, len(states))
	for _, state := range states {
		routeIDs = append(routeIDs, state.RouteID)
	}
	return routeIDs
}

func snapshotNodeIDs(nodes []AuthoritativeDNSSnapshotNode) []string {
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.NodeID)
	}
	return nodeIDs
}

func sameUintSet(left []uint, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[uint]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		if counts[value] == 0 {
			return false
		}
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
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

func assertNoCounter(t *testing.T, counters []DNSObservabilityCounterView, key string) {
	t.Helper()
	for _, counter := range counters {
		if counter.Key == key {
			t.Fatalf("unexpected counter %s in %+v", key, counters)
		}
	}
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
