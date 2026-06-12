package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dushengcdn-agent/internal/protocol"
)

func TestObservabilityBufferStoreUpsertReplayAndAck(t *testing.T) {
	store := NewObservabilityBufferStore(filepath.Join(t.TempDir(), "observability-buffer.json"))

	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403205},
		TrafficReport:       &protocol.NodeTrafficReport{WindowStartedAtUnix: 1710403200, WindowEndedAtUnix: 1710403260, RequestCount: 5},
		QueuedAtUnix:        1710403205,
	}, 1710403000); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403255},
		TrafficReport:       &protocol.NodeTrafficReport{WindowStartedAtUnix: 1710403200, WindowEndedAtUnix: 1710403260, RequestCount: 12},
		QueuedAtUnix:        1710403255,
	}, 1710403000); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403260,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403265},
		TrafficReport:       &protocol.NodeTrafficReport{WindowStartedAtUnix: 1710403260, WindowEndedAtUnix: 1710403320, RequestCount: 2},
		QueuedAtUnix:        1710403265,
	}, 1710403000); err != nil {
		t.Fatalf("third upsert failed: %v", err)
	}

	records, err := store.Replayable(1710403260, 1710403000)
	if err != nil {
		t.Fatalf("Replayable failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one replayable record before current window, got %d", len(records))
	}
	if records[0].TrafficReport == nil || records[0].TrafficReport.RequestCount != 12 {
		t.Fatalf("expected replayable record to keep latest upsert, got %+v", records[0])
	}

	if err = store.Ack([]int64{1710403200}, 1710403000); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
	records, err = store.Replayable(0, 1710403000)
	if err != nil {
		t.Fatalf("Replayable after ack failed: %v", err)
	}
	if len(records) != 1 || records[0].WindowStartedAtUnix != 1710403260 {
		t.Fatalf("unexpected records after ack: %+v", records)
	}
}

func TestObservabilityBufferStoreMergesAccessLogsWithinWindow(t *testing.T) {
	store := NewObservabilityBufferStore(filepath.Join(t.TempDir(), "observability-buffer.json"))

	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		AccessLogs: []protocol.NodeAccessLog{
			{LoggedAtUnix: 1710403201, RemoteAddr: "10.0.0.1", Host: "app.example.com", Path: "/a", StatusCode: 200},
		},
	}, 1710403000); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		AccessLogs: []protocol.NodeAccessLog{
			{LoggedAtUnix: 1710403201, RemoteAddr: "10.0.0.1", Host: "app.example.com", Path: "/a", StatusCode: 200},
			{LoggedAtUnix: 1710403205, RemoteAddr: "10.0.0.2", Host: "app.example.com", Path: "/b", StatusCode: 502},
		},
	}, 1710403000); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	records, err := store.Replayable(0, 1710403000)
	if err != nil {
		t.Fatalf("Replayable failed: %v", err)
	}
	if len(records) != 1 || len(records[0].AccessLogs) != 2 {
		t.Fatalf("expected merged access logs, got %+v", records)
	}
}

func TestObservabilityBufferStoreUpsertAndReplayableUsesCompactJSONAndClones(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability-buffer.json")
	store := NewObservabilityBufferStore(path)

	records, err := store.UpsertAndReplayable(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403205, CPUUsagePercent: 25},
		TrafficReport: &protocol.NodeTrafficReport{
			WindowStartedAtUnix: 1710403200,
			WindowEndedAtUnix:   1710403260,
			RequestCount:        5,
			StatusCodes:         map[string]int64{"200": 5},
		},
		QueuedAtUnix: 1710403205,
	}, 1710403260, 1710403000)
	if err != nil {
		t.Fatalf("UpsertAndReplayable failed: %v", err)
	}
	if len(records) != 1 || records[0].TrafficReport == nil || records[0].TrafficReport.RequestCount != 5 {
		t.Fatalf("expected previous window to be replayable, got %+v", records)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(data), "\n") || strings.Contains(string(data), "  ") {
		t.Fatalf("expected compact JSON, got %q", string(data))
	}

	records[0].TrafficReport.RequestCount = 99
	records[0].TrafficReport.StatusCodes["200"] = 99
	reloaded, err := store.Replayable(1710403260, 1710403000)
	if err != nil {
		t.Fatalf("Replayable failed: %v", err)
	}
	if len(reloaded) != 1 || reloaded[0].TrafficReport == nil || reloaded[0].TrafficReport.RequestCount != 5 || reloaded[0].TrafficReport.StatusCodes["200"] != 5 {
		t.Fatalf("expected replayable records to be cloned from cache, got %+v", reloaded)
	}
}

func TestObservabilityBufferStoreReplayableSkipsUnchangedRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability-buffer.json")
	store := NewObservabilityBufferStore(path)
	if err := store.Upsert(ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403205},
		QueuedAtUnix:        1710403205,
	}, 1710403000); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, err = store.Replayable(1710403260, 1710403000); err != nil {
		t.Fatalf("Replayable failed: %v", err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("second Stat failed: %v", err)
	}
	if !info.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("expected replay without pruning changes to skip rewriting buffer file")
	}
}

func TestObservabilityBufferStoreUpsertSkipsRewriteWhenConcurrentStoreAlreadyPersistedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability-buffer.json")
	firstStore := NewObservabilityBufferStore(path)
	firstRecord := ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403200,
		Snapshot:            &protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403205},
		QueuedAtUnix:        1710403205,
	}
	if err := firstStore.Upsert(firstRecord, 1710403000); err != nil {
		t.Fatalf("first Upsert failed: %v", err)
	}
	staleStore := NewObservabilityBufferStore(path)
	if _, err := staleStore.Replayable(0, 1710403000); err != nil {
		t.Fatalf("stale Replayable failed: %v", err)
	}

	secondRecord := ObservabilityBufferRecord{
		WindowStartedAtUnix: 1710403260,
		TrafficReport:       &protocol.NodeTrafficReport{WindowStartedAtUnix: 1710403260, WindowEndedAtUnix: 1710403320, RequestCount: 3},
		QueuedAtUnix:        1710403265,
	}
	if err := firstStore.Upsert(secondRecord, 1710403000); err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if err = staleStore.Upsert(secondRecord, 1710403000); err != nil {
		t.Fatalf("stale Upsert failed: %v", err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("second Stat failed: %v", err)
	}
	if !info.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("expected already-persisted buffer records to skip rewriting buffer file")
	}
}

func TestObservabilityWindowStartedAt(t *testing.T) {
	if value := ObservabilityWindowStartedAt(nil, &protocol.NodeTrafficReport{WindowStartedAtUnix: 1710403200}); value != 1710403200 {
		t.Fatalf("unexpected traffic window start: %d", value)
	}
	if value := ObservabilityWindowStartedAt(&protocol.NodeMetricSnapshot{CapturedAtUnix: 1710403259}, nil); value != 1710403200 {
		t.Fatalf("unexpected snapshot-derived window start: %d", value)
	}
}
