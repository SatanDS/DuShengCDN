package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func TestLoadNodeObservabilityQueryDataRunsQueriesConcurrently(t *testing.T) {
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
