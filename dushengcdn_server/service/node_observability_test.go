package service

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"
	"dushengcdn/utils/geoip"

	"gorm.io/gorm"
)

type countingAccessLogGeoProvider struct {
	calls atomic.Int32
}

func (p *countingAccessLogGeoProvider) Name() string {
	return "counting-access-log-geoip"
}

func (p *countingAccessLogGeoProvider) GetGeoInfo(net.IP) (*geoip.GeoInfo, error) {
	p.calls.Add(1)
	return &geoip.GeoInfo{
		Name:     "Test Region",
		Operator: "Test ISP",
	}, nil
}

func (p *countingAccessLogGeoProvider) UpdateDatabase() error {
	return nil
}

func (p *countingAccessLogGeoProvider) Close() error {
	return nil
}

func TestLoadNodeObservabilityQueryDataRunsQueriesConcurrently(t *testing.T) {
	withAccessLogPersistenceEnabled(t)

	const queryCount = 8
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	markStartedAndWait := func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	queries := nodeObservabilityQueries{
		getNodeSystemProfile: func(string) (*model.NodeSystemProfile, error) {
			markStartedAndWait()
			return nil, gorm.ErrRecordNotFound
		},
		listNodeMetricSnapshots: func(string, time.Time, int) ([]*model.NodeMetricSnapshot, error) {
			markStartedAndWait()
			return nil, nil
		},
		listNodeRequestReports: func(string, time.Time, int) ([]*model.NodeRequestReport, error) {
			markStartedAndWait()
			return nil, nil
		},
		listNodeAccessLogRegionCounts: func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error) {
			markStartedAndWait()
			return nil, nil
		},
		listMetricSnapshotTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotTrendBucket, error) {
			markStartedAndWait()
			return nil, nil
		},
		listMetricCounterDeltaBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotCounterDeltaBucket, error) {
			markStartedAndWait()
			return nil, nil
		},
		listRequestReportTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error) {
			markStartedAndWait()
			return nil, nil
		},
		listNodeHealthEvents: func(string, bool, int) ([]*model.NodeHealthEvent, error) {
			markStartedAndWait()
			return nil, nil
		},
	}

	now := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := loadNodeObservabilityQueryData("node-concurrent", now.Add(-time.Hour), now.Add(-24*time.Hour), now, 10, queries)
		done <- err
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all node observability queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loadNodeObservabilityQueryData returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent node observability query load")
	}
}

func TestLoadNodeObservabilityQueryDataIgnoresMissingProfile(t *testing.T) {
	queries := successfulNodeObservabilityQueries()
	queries.getNodeSystemProfile = func(string) (*model.NodeSystemProfile, error) {
		return nil, gorm.ErrRecordNotFound
	}

	now := time.Now()
	data, err := loadNodeObservabilityQueryData("node-no-profile", now.Add(-time.Hour), now.Add(-24*time.Hour), now, 10, queries)
	if err != nil {
		t.Fatalf("expected missing profile to be ignored, got %v", err)
	}
	if data.profile != nil {
		t.Fatalf("expected missing profile to remain nil, got %+v", data.profile)
	}
}

func TestLoadNodeObservabilityQueryDataReturnsQueryError(t *testing.T) {
	wantErr := errors.New("metric snapshots failed")
	queries := successfulNodeObservabilityQueries()
	queries.listNodeMetricSnapshots = func(string, time.Time, int) ([]*model.NodeMetricSnapshot, error) {
		return nil, wantErr
	}

	now := time.Now()
	_, err := loadNodeObservabilityQueryData("node-error", now.Add(-time.Hour), now.Add(-24*time.Hour), now, 10, queries)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected query error to be returned, got %v", err)
	}
}

func TestGetNodeObservabilityCacheReusesRecentResult(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:     "node-observability-cache",
		Name:       "cache-edge",
		IP:         "10.0.0.64",
		AgentToken: "token-observability-cache",
		Status:     NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}
	if err := (&model.NodeMetricSnapshot{
		NodeID:           node.NodeID,
		CapturedAt:       time.Now(),
		CPUUsagePercent:  12,
		MemoryUsedBytes:  256,
		MemoryTotalBytes: 1024,
	}).Insert(); err != nil {
		t.Fatalf("failed to insert first metric snapshot: %v", err)
	}

	first, err := GetNodeObservability(node.ID, NodeObservabilityQuery{Hours: 24, Limit: 10})
	if err != nil {
		t.Fatalf("GetNodeObservability first call failed: %v", err)
	}
	if len(first.MetricSnapshots) != 1 {
		t.Fatalf("expected first call to load one metric snapshot, got %d", len(first.MetricSnapshots))
	}

	if err := (&model.NodeMetricSnapshot{
		NodeID:           node.NodeID,
		CapturedAt:       time.Now().Add(time.Second),
		CPUUsagePercent:  88,
		MemoryUsedBytes:  512,
		MemoryTotalBytes: 1024,
	}).Insert(); err != nil {
		t.Fatalf("failed to insert second metric snapshot: %v", err)
	}

	second, err := GetNodeObservability(node.ID, NodeObservabilityQuery{Hours: 24, Limit: 10})
	if err != nil {
		t.Fatalf("GetNodeObservability cached call failed: %v", err)
	}
	if len(second.MetricSnapshots) != 1 {
		t.Fatalf("expected cached call to reuse previous metric snapshots, got %d", len(second.MetricSnapshots))
	}
	if second.MetricSnapshots[0].CPUUsagePercent != first.MetricSnapshots[0].CPUUsagePercent {
		t.Fatalf("expected cached metric snapshot to be reused, got %+v want %+v", second.MetricSnapshots[0], first.MetricSnapshots[0])
	}
}

func TestPersistNodeAccessLogsMemoizesGeoLookupPerBatch(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:     "node-access-log-geo-memo",
		Name:       "geo-memo-edge",
		IP:         "10.0.0.88",
		AgentToken: "token-access-log-geo-memo",
		Status:     NodeStatusOnline,
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}

	provider := &countingAccessLogGeoProvider{}
	restore := setAccessLogGeoProviderFactoryForTest(func() (geoip.GeoIPService, error) {
		return provider, nil
	})
	t.Cleanup(restore)

	reportedAt := time.Now().UTC()
	err := persistNodeAccessLogsWithTransaction(node.NodeID, []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "198.51.100.10",
			Host:         "memo.example.com",
			Path:         "/first",
			StatusCode:   200,
		},
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "198.51.100.10:443",
			Host:         "memo.example.com",
			Path:         "/same-ip",
			StatusCode:   200,
		},
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "198.51.100.11",
			Host:         "memo.example.com",
			Path:         "/second-ip",
			StatusCode:   200,
		},
	}, reportedAt)
	if err != nil {
		t.Fatalf("persistNodeAccessLogsWithTransaction failed: %v", err)
	}

	if got := provider.calls.Load(); got != 2 {
		t.Fatalf("expected one GeoIP lookup per normalized IP, got %d", got)
	}
}

func TestPersistNodeAccessLogsSkipsGeoProviderForInvalidRemoteAddresses(t *testing.T) {
	setupServiceTestDB(t)
	resetAccessLogRegionProviderForTest()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	accessLogRegionState.Lock()
	previousFactory := accessLogGeoProviderFactory
	accessLogGeoProviderFactory = func() (geoip.GeoIPService, error) {
		started <- struct{}{}
		<-release
		return &countingAccessLogGeoProvider{}, nil
	}
	accessLogRegionState.Unlock()
	t.Cleanup(func() {
		close(release)
		accessLogRegionState.Lock()
		accessLogGeoProviderFactory = previousFactory
		accessLogRegionState.Unlock()
		resetAccessLogRegionProviderForTest()
	})

	reportedAt := time.Now().UTC()
	err := persistNodeAccessLogsWithTransaction("node-access-log-invalid-remote", []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "",
			Host:         "invalid-remote.example.com",
			Path:         "/blank",
			StatusCode:   200,
		},
		{
			LoggedAtUnix: reportedAt.Add(time.Second).Unix(),
			RemoteAddr:   "not-an-ip",
			Host:         "invalid-remote.example.com",
			Path:         "/text",
			StatusCode:   200,
		},
		{
			LoggedAtUnix: reportedAt.Add(2 * time.Second).Unix(),
			RemoteAddr:   "example.com:443",
			Host:         "invalid-remote.example.com",
			Path:         "/hostname-port",
			StatusCode:   200,
		},
	}, reportedAt)
	if err != nil {
		t.Fatalf("persistNodeAccessLogsWithTransaction failed: %v", err)
	}

	accessLogRegionState.Lock()
	initializing := accessLogRegionState.initializing
	providerReady := accessLogRegionState.provider != nil
	accessLogRegionState.Unlock()
	if initializing || providerReady {
		t.Fatalf("expected invalid remote addresses not to initialize GeoIP provider, initializing=%v providerReady=%v", initializing, providerReady)
	}
	select {
	case <-started:
		t.Fatal("expected invalid remote addresses not to start GeoIP provider factory")
	default:
	}
}

func successfulNodeObservabilityQueries() nodeObservabilityQueries {
	return nodeObservabilityQueries{
		getNodeSystemProfile: func(string) (*model.NodeSystemProfile, error) {
			return &model.NodeSystemProfile{}, nil
		},
		listNodeMetricSnapshots: func(string, time.Time, int) ([]*model.NodeMetricSnapshot, error) {
			return nil, nil
		},
		listNodeRequestReports: func(string, time.Time, int) ([]*model.NodeRequestReport, error) {
			return nil, nil
		},
		listNodeAccessLogRegionCounts: func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error) {
			return nil, nil
		},
		listMetricSnapshotTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotTrendBucket, error) {
			return nil, nil
		},
		listMetricCounterDeltaBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotCounterDeltaBucket, error) {
			return nil, nil
		},
		listRequestReportTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error) {
			return nil, nil
		},
		listNodeHealthEvents: func(string, bool, int) ([]*model.NodeHealthEvent, error) {
			return nil, nil
		},
	}
}
