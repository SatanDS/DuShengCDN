package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureNodeIDPersists(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID1, err := store.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	nodeID2, err := store.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID second call failed: %v", err)
	}
	if nodeID1 == "" || nodeID1 != nodeID2 {
		t.Fatal("expected node id to persist across calls")
	}
}

func TestStoreSaveUsesCompactJSONAndSkipsUnchangedRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path)
	snapshot := &Snapshot{
		NodeID:          "node-compact",
		CurrentVersion:  "20260612-001",
		CurrentChecksum: "checksum",
	}
	if err := store.Save(snapshot); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(data), "\n") || strings.Contains(string(data), "  ") {
		t.Fatalf("expected compact JSON, got %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if err = store.Save(cloneSnapshot(snapshot)); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("second Stat failed: %v", err)
	}
	if !info.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("expected unchanged snapshot save to skip rewriting state file")
	}
}

func TestStoreSaveSkipsRewriteWhenConcurrentStoreAlreadyPersistedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	firstStore := NewStore(path)
	initial := &Snapshot{NodeID: "node-concurrent", CurrentVersion: "20260612-001"}
	if err := firstStore.Save(initial); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}
	staleStore := NewStore(path)
	if _, err := staleStore.Load(); err != nil {
		t.Fatalf("stale Load failed: %v", err)
	}

	updated := cloneSnapshot(initial)
	updated.CurrentVersion = "20260612-002"
	if err := firstStore.Save(updated); err != nil {
		t.Fatalf("updated Save failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if err = staleStore.Save(updated); err != nil {
		t.Fatalf("stale Save failed: %v", err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("second Stat failed: %v", err)
	}
	if !info.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("expected already-persisted snapshot to skip rewriting state file")
	}
}
