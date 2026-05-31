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

func ptrTime(value time.Time) *time.Time {
	return &value
}
