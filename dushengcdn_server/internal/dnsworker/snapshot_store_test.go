package dnsworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotStoreSaveLoadEnvelopeWithChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snapshot := snapshotStoreTestSnapshot("version-1")

	store := NewSnapshotStore(path, time.Minute)
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot cache: %v", err)
	}
	var envelope persistedSnapshotEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode snapshot envelope: %v", err)
	}
	if envelope.Format != persistedSnapshotFormat || envelope.FormatVersion != persistedSnapshotFormatVersion {
		t.Fatalf("unexpected snapshot envelope metadata: %+v", envelope)
	}
	if envelope.Snapshot == nil || envelope.Snapshot.SnapshotVersion != "version-1" {
		t.Fatalf("unexpected snapshot envelope payload: %+v", envelope.Snapshot)
	}
	if !strings.HasPrefix(envelope.Checksum, "sha256:") {
		t.Fatalf("expected sha256 checksum, got %q", envelope.Checksum)
	}

	loaded := NewSnapshotStore(path, time.Minute)
	if err := loaded.LoadFromDisk(); err != nil {
		t.Fatalf("load snapshot cache: %v", err)
	}
	if got := loaded.Version(); got != "version-1" {
		t.Fatalf("expected loaded version version-1, got %q", got)
	}
	if got := loaded.LastError(); got != "" {
		t.Fatalf("expected no last error, got %q", got)
	}
	loadedSnapshot, _, _, _ := loaded.Current()
	if loadedSnapshot == nil ||
		len(loadedSnapshot.SchedulingStates) != 1 ||
		loadedSnapshot.SchedulingStates[0].SelectedTargets[0] != "8.8.4.4" {
		t.Fatalf("expected scheduling states to survive cache roundtrip, got %+v", loadedSnapshot)
	}
}

func TestSnapshotStoreRejectsTamperedEnvelopeWithoutReplacingCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	store := NewSnapshotStore(path, time.Minute)
	if err := store.Set(snapshotStoreTestSnapshot("current")); err != nil {
		t.Fatalf("set current snapshot: %v", err)
	}
	if err := store.Save(snapshotStoreTestSnapshot("disk")); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot cache: %v", err)
	}
	var envelope persistedSnapshotEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode snapshot envelope: %v", err)
	}
	envelope.Snapshot.SnapshotVersion = "tampered"
	raw, err = json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered snapshot envelope: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write tampered snapshot cache: %v", err)
	}

	if err := store.LoadFromDisk(); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if got := store.Version(); got != "current" {
		t.Fatalf("expected current snapshot to remain loaded, got %q", got)
	}
	if got := store.LastError(); !strings.Contains(got, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch last error, got %q", got)
	}
}

func TestSnapshotStoreLoadsLegacySnapshotFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	raw, err := json.MarshalIndent(snapshotStoreTestSnapshot("legacy"), "", "  ")
	if err != nil {
		t.Fatalf("encode legacy snapshot: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}

	store := NewSnapshotStore(path, time.Minute)
	if err := store.LoadFromDisk(); err != nil {
		t.Fatalf("load legacy snapshot: %v", err)
	}
	if got := store.Version(); got != "legacy" {
		t.Fatalf("expected legacy snapshot version, got %q", got)
	}
}

func TestSnapshotStoreLoadsLegacyGSLBPoolsWithoutEnabledAsEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snapshot := snapshotStoreTestGSLBSnapshot("legacy-gslb")
	snapshot.Routes[0].GSLBPolicy.Pools = []GSLBPoolPolicy{
		{Name: "legacy", Weight: 100},
	}
	snapshot.Nodes = []SnapshotNode{
		snapshotStoreTestNode("legacy-node", "legacy", "9.9.9.9", 1000),
	}

	raw := marshalSnapshotWithPoolEnabledOmitted(t, snapshot)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write legacy GSLB snapshot: %v", err)
	}

	store := NewSnapshotStore(path, time.Minute)
	if err := store.LoadFromDisk(); err != nil {
		t.Fatalf("load legacy GSLB snapshot: %v", err)
	}
	loaded, _, _, _ := store.Current()
	if loaded == nil || len(loaded.Routes) != 1 || len(loaded.Routes[0].GSLBPolicy.Pools) != 1 {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}
	if !loaded.Routes[0].GSLBPolicy.Pools[0].Enabled {
		t.Fatalf("expected legacy pool missing enabled to be normalized as enabled, got %+v", loaded.Routes[0].GSLBPolicy.Pools[0])
	}

	targets, _, _, err := NewScheduler().Select(loaded, &loaded.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select legacy GSLB target: %v", err)
	}
	if len(targets) != 1 || targets[0] != "9.9.9.9" {
		t.Fatalf("expected legacy GSLB pool target after load, got %+v", targets)
	}
}

func TestSnapshotStoreRoundTripKeepsExplicitDisabledGSLBPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snapshot := snapshotStoreTestGSLBSnapshot("disabled-pool")

	store := NewSnapshotStore(path, time.Minute)
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("save GSLB snapshot: %v", err)
	}

	loadedStore := NewSnapshotStore(path, time.Minute)
	if err := loadedStore.LoadFromDisk(); err != nil {
		t.Fatalf("load GSLB snapshot: %v", err)
	}
	loaded, _, _, _ := loadedStore.Current()
	if loaded == nil || len(loaded.Routes) != 1 || len(loaded.Routes[0].GSLBPolicy.Pools) != 2 {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}
	disabled := loaded.Routes[0].GSLBPolicy.Pools[1]
	if disabled.Name != "disabled" || disabled.Enabled {
		t.Fatalf("expected explicitly disabled pool to survive cache roundtrip, got %+v", loaded.Routes[0].GSLBPolicy.Pools)
	}

	targets, _, _, err := NewScheduler().Select(loaded, &loaded.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select GSLB target: %v", err)
	}
	if len(targets) != 1 || targets[0] != "8.8.4.4" {
		t.Fatalf("expected enabled pool target after cache roundtrip, got %+v", targets)
	}
}

func snapshotStoreTestSnapshot(version string) *Snapshot {
	return &Snapshot{
		SnapshotVersion: version,
		GeneratedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
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
				},
			},
		},
		Routes: []SnapshotRoute{
			{
				ID:             1,
				Domains:        []string{"cdn.example.com"},
				ZoneID:         1,
				NodePool:       "default",
				RecordType:     "A",
				TargetCount:    1,
				TTL:            30,
				CurrentTargets: []string{"8.8.4.4"},
			},
		},
		Nodes: []SnapshotNode{
			{
				NodeID:            "node-1",
				Name:              "node-1",
				PoolName:          "default",
				PublicIPs:         []string{"8.8.4.4"},
				Weight:            100,
				SchedulingEnabled: true,
				Status:            "online",
				OpenrestyStatus:   "healthy",
				LastSeenAt:        time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
			},
		},
		SchedulingStates: []SnapshotSchedulingState{
			{
				RouteID:         1,
				RecordType:      "A",
				ScopeKey:        "global",
				SelectedTargets: []string{"8.8.4.4"},
				DesiredTargets:  []string{"8.8.4.4"},
				LastChangedAt:   ptrTime(time.Date(2026, 1, 2, 3, 4, 1, 0, time.UTC)),
			},
		},
	}
}

func snapshotStoreTestGSLBSnapshot(version string) *Snapshot {
	snapshot := snapshotStoreTestSnapshot(version)
	snapshot.Routes[0].NodePool = "hk"
	snapshot.Routes[0].GSLBEnabled = true
	snapshot.Routes[0].ScheduleMode = "weighted"
	snapshot.Routes[0].CurrentTargets = nil
	snapshot.Routes[0].GSLBPolicy = GSLBPolicy{
		Strategy:    "weighted",
		TargetCount: 1,
		TTL:         30,
		Pools: []GSLBPoolPolicy{
			{Name: "hk", Weight: 100, Enabled: true},
			{Name: "disabled", Weight: 1000, Enabled: false},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		snapshotStoreTestNode("hk-node", "hk", "8.8.4.4", 100),
		snapshotStoreTestNode("disabled-node", "disabled", "9.9.9.9", 1000),
	}
	snapshot.SchedulingStates = nil
	return snapshot
}

func snapshotStoreTestNode(id string, pool string, ip string, weight int) SnapshotNode {
	return SnapshotNode{
		NodeID:            id,
		Name:              id,
		PoolName:          pool,
		PublicIPs:         []string{ip},
		Weight:            weight,
		SchedulingEnabled: true,
		Status:            "online",
		OpenrestyStatus:   "healthy",
		LastSeenAt:        time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
	}
}

func marshalSnapshotWithPoolEnabledOmitted(t *testing.T, snapshot *Snapshot) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode snapshot document: %v", err)
	}
	routes, ok := document["routes"].([]any)
	if !ok {
		t.Fatalf("expected routes in snapshot document: %+v", document)
	}
	for _, routeItem := range routes {
		route, ok := routeItem.(map[string]any)
		if !ok {
			t.Fatalf("expected route object, got %+v", routeItem)
		}
		policy, ok := route["gslb_policy"].(map[string]any)
		if !ok {
			continue
		}
		pools, ok := policy["pools"].([]any)
		if !ok {
			continue
		}
		for _, poolItem := range pools {
			pool, ok := poolItem.(map[string]any)
			if !ok {
				t.Fatalf("expected pool object, got %+v", poolItem)
			}
			delete(pool, "enabled")
		}
	}
	raw, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode legacy snapshot document: %v", err)
	}
	return raw
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
