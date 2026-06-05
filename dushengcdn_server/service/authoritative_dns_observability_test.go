package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestLoadDNSObservabilitySummaryQueryDataRunsQueriesConcurrently(t *testing.T) {
	const queryCount = 2
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	markStartedAndWait := func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	queries := dnsObservabilitySummaryQueries{
		queryRecentRows: func(DNSObservabilitySummaryInput, dnsObservabilityWindow, int) ([]dnsObservabilityRollupSampleRow, error) {
			markStartedAndWait()
			return nil, nil
		},
		queryLastRollupAt: func(DNSObservabilitySummaryInput, dnsObservabilityWindow) (*time.Time, error) {
			markStartedAndWait()
			return nil, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := loadDNSObservabilitySummaryQueryData(DNSObservabilitySummaryInput{}, dnsObservabilityWindow{}, queries)
		done <- err
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all DNS observability summary queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loadDNSObservabilitySummaryQueryData returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent DNS observability summary query load")
	}
}

func TestLoadDNSObservabilitySummaryQueryDataReturnsQueryError(t *testing.T) {
	wantErr := errors.New("dns observability query failed")
	queries := successfulDNSObservabilitySummaryQueries()
	queries.queryRecentRows = func(DNSObservabilitySummaryInput, dnsObservabilityWindow, int) ([]dnsObservabilityRollupSampleRow, error) {
		return nil, wantErr
	}

	_, err := loadDNSObservabilitySummaryQueryData(DNSObservabilitySummaryInput{}, dnsObservabilityWindow{}, queries)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected query error to be returned, got %v", err)
	}
}

func TestLoadDNSObservabilitySummaryLabelDataRunsQueriesConcurrently(t *testing.T) {
	const queryCount = 3
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	markStartedAndWait := func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	queries := dnsObservabilitySummaryLabelQueries{
		dnsWorkerLabels: func() (map[string]string, error) {
			markStartedAndWait()
			return map[string]string{}, nil
		},
		dnsZoneLabels: func() (map[string]string, error) {
			markStartedAndWait()
			return map[string]string{}, nil
		},
		dnsRouteLabels: func(map[uint]int64) (map[string]string, error) {
			markStartedAndWait()
			return map[string]string{}, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := loadDNSObservabilitySummaryLabelData(map[uint]int64{1: 10}, queries)
		done <- err
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all DNS observability label queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loadDNSObservabilitySummaryLabelData returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent DNS observability label query load")
	}
}

func TestLoadDNSObservabilitySummaryLabelDataReturnsQueryError(t *testing.T) {
	wantErr := errors.New("dns route labels failed")
	queries := dnsObservabilitySummaryLabelQueries{
		dnsWorkerLabels: func() (map[string]string, error) {
			return map[string]string{}, nil
		},
		dnsZoneLabels: func() (map[string]string, error) {
			return map[string]string{}, nil
		},
		dnsRouteLabels: func(map[uint]int64) (map[string]string, error) {
			return nil, wantErr
		},
	}

	_, err := loadDNSObservabilitySummaryLabelData(map[uint]int64{1: 10}, queries)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected label query error to be returned, got %v", err)
	}
}

func TestDNSRouteLabelsLoadsRoutesByIDs(t *testing.T) {
	setupServiceTestDB(t)

	routeA := &model.ProxyRoute{
		SiteName:  "edge-a",
		Domain:    "a.example.com",
		Domains:   `["a.example.com"]`,
		OriginURL: "https://origin-a.internal",
		Upstreams: `["https://origin-a.internal"]`,
		NodePool:  "default",
		Enabled:   true,
	}
	if err := routeA.Insert(); err != nil {
		t.Fatalf("insert route a: %v", err)
	}
	routeB := &model.ProxyRoute{
		Domain:    "b.example.com",
		Domains:   `["b.example.com"]`,
		OriginURL: "https://origin-b.internal",
		Upstreams: `["https://origin-b.internal"]`,
		NodePool:  "default",
		Enabled:   true,
	}
	if err := routeB.Insert(); err != nil {
		t.Fatalf("insert route b: %v", err)
	}

	labels, err := dnsRouteLabels(map[uint]int64{
		0:         99,
		routeB.ID: 7,
		999999:    3,
		routeA.ID: 10,
	})
	if err != nil {
		t.Fatalf("dnsRouteLabels failed: %v", err)
	}
	if labels[fmt.Sprint(routeA.ID)] != "edge-a" {
		t.Fatalf("expected route a site label, got %+v", labels)
	}
	if labels[fmt.Sprint(routeB.ID)] != "b.example.com" {
		t.Fatalf("expected route b domain fallback label, got %+v", labels)
	}
	if _, ok := labels["999999"]; ok {
		t.Fatalf("expected missing route label to be omitted, got %+v", labels)
	}
	if _, ok := labels["0"]; ok {
		t.Fatalf("expected zero route label to be omitted, got %+v", labels)
	}
}

func successfulDNSObservabilitySummaryQueries() dnsObservabilitySummaryQueries {
	return dnsObservabilitySummaryQueries{
		queryRecentRows: func(DNSObservabilitySummaryInput, dnsObservabilityWindow, int) ([]dnsObservabilityRollupSampleRow, error) {
			return nil, nil
		},
		queryLastRollupAt: func(DNSObservabilitySummaryInput, dnsObservabilityWindow) (*time.Time, error) {
			return nil, nil
		},
	}
}
