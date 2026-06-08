package dnsworker

import (
	"fmt"
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

func TestRollupAggregatorRecordsIndependentSourceDimensions(t *testing.T) {
	aggregator := NewRollupAggregator(time.Minute)

	aggregator.RecordWithSource(
		1,
		2,
		SourceContext{ScopeKey: "asn:45102", Country: "cn", ASN: 45102, Operator: "China Telecom"},
		"www.example.com",
		"A",
		"NOERROR",
		[]string{"203.0.113.10"},
		time.Millisecond,
	)

	payloads := aggregator.Drain()
	payload := findRollupPayload(t, payloads, "asn:45102")
	if payload.SourceCountry != "CN" {
		t.Fatalf("expected source country CN, got %+v", payload)
	}
	if payload.SourceASN != 45102 {
		t.Fatalf("expected source ASN 45102, got %+v", payload)
	}
	if payload.SourceOperator != "cn-telecom" {
		t.Fatalf("expected normalized source operator, got %+v", payload)
	}
}

func TestRollupAggregatorCollapsesExcessBuckets(t *testing.T) {
	aggregator := NewRollupAggregator(time.Minute)

	for index := 0; index < DefaultRollupMaxBuckets+250; index++ {
		aggregator.Record(1, 2, "country:HK", fmt.Sprintf("q-%d.example.com", index), "A", "NOERROR", []string{"8.8.8.8"}, time.Millisecond)
	}

	payloads := aggregator.Drain()
	if len(payloads) > DefaultRollupMaxBuckets+1 {
		t.Fatalf("expected bucket cap plus overflow, got %d payloads", len(payloads))
	}
	var overflow QueryRollupPayload
	for _, payload := range payloads {
		if payload.QName == rollupOverflowQName {
			overflow = payload
			break
		}
	}
	if overflow.QueryCount != 250 || overflow.QType != "ANY" || overflow.SourceScope != "global" {
		t.Fatalf("expected excess queries to collapse into overflow bucket, got %+v", overflow)
	}
}

func TestRollupAggregatorLimitsTargetSummary(t *testing.T) {
	aggregator := NewRollupAggregator(time.Minute)
	targets := make([]string, 0, DefaultRollupMaxTargetSummary+25)
	for index := 0; index < DefaultRollupMaxTargetSummary+25; index++ {
		targets = append(targets, fmt.Sprintf("192.0.2.%d", index))
	}

	aggregator.Record(1, 2, "global", "www.example.com", "A", "NOERROR", targets, time.Millisecond)
	payloads := aggregator.Drain()
	if len(payloads) != 1 {
		t.Fatalf("expected one payload, got %+v", payloads)
	}
	if len(payloads[0].TargetSummary) != DefaultRollupMaxTargetSummary {
		t.Fatalf("expected target summary cap %d, got %d", DefaultRollupMaxTargetSummary, len(payloads[0].TargetSummary))
	}
}
