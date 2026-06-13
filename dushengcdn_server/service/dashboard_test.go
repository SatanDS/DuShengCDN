package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestLoadDashboardOverviewQueryDataRunsQueriesConcurrently(t *testing.T) {
	withAccessLogPersistenceEnabled(t)

	const queryCount = 10
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	markStartedAndWait := func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	queries := dashboardOverviewQueries{
		listNodes: func() ([]*model.Node, error) {
			markStartedAndWait()
			return nil, nil
		},
		listLatestMetricSnapshotsByNode: func(time.Time, time.Time) ([]*model.NodeMetricSnapshot, error) {
			markStartedAndWait()
			return nil, nil
		},
		listLatestRequestReportsByNode: func(time.Time, time.Time) ([]*model.NodeRequestReport, error) {
			markStartedAndWait()
			return nil, nil
		},
		listRequestReportTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error) {
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
		listAccessLogStatusDistributions: func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error) {
			markStartedAndWait()
			return nil, nil
		},
		listAccessLogHostDistributions: func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error) {
			markStartedAndWait()
			return nil, nil
		},
		listAccessLogRegionCounts: func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error) {
			markStartedAndWait()
			return nil, nil
		},
		listActiveNodeHealthEvents: func() ([]*model.NodeHealthEvent, error) {
			markStartedAndWait()
			return nil, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := loadDashboardOverviewQueryData(time.Now().Add(-time.Hour), time.Now(), queries)
		done <- err
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all dashboard queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loadDashboardOverviewQueryData returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent dashboard query load")
	}
}

func TestLoadDashboardOverviewQueryDataReturnsQueryError(t *testing.T) {
	wantErr := errors.New("dashboard query failed")
	queries := dashboardOverviewQueries{
		listNodes: func() ([]*model.Node, error) {
			return nil, wantErr
		},
		listLatestMetricSnapshotsByNode: func(time.Time, time.Time) ([]*model.NodeMetricSnapshot, error) {
			return nil, nil
		},
		listLatestRequestReportsByNode: func(time.Time, time.Time) ([]*model.NodeRequestReport, error) {
			return nil, nil
		},
		listRequestReportTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error) {
			return nil, nil
		},
		listMetricSnapshotTrendBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotTrendBucket, error) {
			return nil, nil
		},
		listMetricCounterDeltaBuckets: func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotCounterDeltaBucket, error) {
			return nil, nil
		},
		listAccessLogStatusDistributions: func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error) {
			return nil, nil
		},
		listAccessLogHostDistributions: func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error) {
			return nil, nil
		},
		listAccessLogRegionCounts: func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error) {
			return nil, nil
		},
		listActiveNodeHealthEvents: func() ([]*model.NodeHealthEvent, error) {
			return nil, nil
		},
	}

	_, err := loadDashboardOverviewQueryData(time.Now().Add(-time.Hour), time.Now(), queries)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected query error to be returned, got %v", err)
	}
}
