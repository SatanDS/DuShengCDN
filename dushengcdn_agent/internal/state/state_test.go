package state

import (
	"path/filepath"
	"testing"
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
