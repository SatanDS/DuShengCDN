package dnsworker

import (
	"testing"
	"time"
)

func TestRollupAggregatorRecordsAndRestoresDurations(t *testing.T) {
	aggregator := NewRollupAggregator(time.Minute)

	aggregator.Record(1, 2, "country:HK", "WWW.Example.COM.", "a", "NOERROR", []string{"8.8.8.8"}, 12*time.Millisecond)
	aggregator.Record(1, 2, "country:HK", "www.example.com", "A", "NOERROR", []string{"8.8.8.8"}, 30*time.Millisecond)
	aggregator.Record(1, 2, "country:DE", "www.example.com", "A", "NOERROR", []string{"1.1.1.1"}, 8*time.Millisecond)
	payloads := aggregator.Drain()
	if len(payloads) != 2 {
		t.Fatalf("expected two source-scoped payloads, got %+v", payloads)
	}
	payload := findRollupPayload(t, payloads, "country:HK")
	if payload.QueryCount != 2 || payload.TotalDurationMs != 42 || payload.MaxDurationMs != 30 {
		t.Fatalf("unexpected duration payload: %+v", payload)
	}
	if payload.SourceScope != "country:HK" {
		t.Fatalf("unexpected source scope: %+v", payload)
	}
	dePayload := findRollupPayload(t, payloads, "country:DE")
	if dePayload.QueryCount != 1 || dePayload.TargetSummary["1.1.1.1"] != 1 {
		t.Fatalf("unexpected DE payload: %+v", dePayload)
	}

	restored := NewRollupAggregator(time.Minute)
	restored.Restore(payloads)
	restoredPayloads := restored.Drain()
	if len(restoredPayloads) != 2 {
		t.Fatalf("expected one restored payload, got %+v", restoredPayloads)
	}
	restoredPayload := findRollupPayload(t, restoredPayloads, "country:HK")
	if restoredPayload.QueryCount != 2 || restoredPayload.TotalDurationMs != 42 || restoredPayload.MaxDurationMs != 30 {
		t.Fatalf("unexpected restored duration payload: %+v", restoredPayload)
	}
}

func findRollupPayload(t *testing.T, payloads []QueryRollupPayload, sourceScope string) QueryRollupPayload {
	t.Helper()
	for _, payload := range payloads {
		if payload.SourceScope == sourceScope {
			return payload
		}
	}
	t.Fatalf("missing source scope %s in %+v", sourceScope, payloads)
	return QueryRollupPayload{}
}
