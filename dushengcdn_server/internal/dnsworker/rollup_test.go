package dnsworker

import (
	"testing"
	"time"
)

func TestRollupAggregatorRecordsAndRestoresDurations(t *testing.T) {
	aggregator := NewRollupAggregator(time.Minute)

	aggregator.Record(1, 2, "WWW.Example.COM.", "a", "NOERROR", []string{"8.8.8.8"}, 12*time.Millisecond)
	aggregator.Record(1, 2, "www.example.com", "A", "NOERROR", []string{"8.8.8.8"}, 30*time.Millisecond)
	payloads := aggregator.Drain()
	if len(payloads) != 1 {
		t.Fatalf("expected one payload, got %+v", payloads)
	}
	payload := payloads[0]
	if payload.QueryCount != 2 || payload.TotalDurationMs != 42 || payload.MaxDurationMs != 30 {
		t.Fatalf("unexpected duration payload: %+v", payload)
	}

	restored := NewRollupAggregator(time.Minute)
	restored.Restore(payloads)
	restoredPayloads := restored.Drain()
	if len(restoredPayloads) != 1 {
		t.Fatalf("expected one restored payload, got %+v", restoredPayloads)
	}
	restoredPayload := restoredPayloads[0]
	if restoredPayload.QueryCount != 2 || restoredPayload.TotalDurationMs != 42 || restoredPayload.MaxDurationMs != 30 {
		t.Fatalf("unexpected restored duration payload: %+v", restoredPayload)
	}
}
