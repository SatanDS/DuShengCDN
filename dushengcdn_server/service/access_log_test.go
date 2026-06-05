package service

import (
	"dushengcdn/model"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunConcurrentQueriesRunsAllQueriesConcurrently(t *testing.T) {
	const queryCount = 5
	started := make(chan struct{}, queryCount)
	release := make(chan struct{})
	var calls atomic.Int32

	functions := make([]func() error, 0, queryCount)
	for range queryCount {
		functions = append(functions, func() error {
			calls.Add(1)
			started <- struct{}{}
			<-release
			return nil
		})
	}

	done := make(chan error, 1)
	go func() {
		done <- runConcurrentQueries(functions...)
	}()

	for index := 0; index < queryCount; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("expected all queries to start concurrently, got %d/%d", calls.Load(), queryCount)
		}
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runConcurrentQueries returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent queries")
	}
}

func TestRunConcurrentQueriesReturnsFirstError(t *testing.T) {
	wantErr := errors.New("query failed")
	err := runConcurrentQueries(
		func() error { return nil },
		func() error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected query error to be returned, got %v", err)
	}
}

func TestListAccessLogsIncludesSummaryTotals(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now()
	if err := model.DB.Create(&model.Node{
		NodeID: "node-a",
		Name:   "edge-a",
	}).Error; err != nil {
		t.Fatalf("failed to seed node-a: %v", err)
	}
	if err := model.DB.Create(&model.Node{
		NodeID: "node-b",
		Name:   "edge-b",
	}).Error; err != nil {
		t.Fatalf("failed to seed node-b: %v", err)
	}

	logs := []*model.NodeAccessLog{
		{
			NodeID:     "node-a",
			LoggedAt:   now.Add(-5 * time.Minute),
			RemoteAddr: "1.1.1.1",
			Region:     "United States",
			Host:       "a.example.com",
			Path:       "/alpha",
			StatusCode: 200,
		},
		{
			NodeID:     "node-a",
			LoggedAt:   now.Add(-4 * time.Minute),
			RemoteAddr: "2.2.2.2",
			Region:     "China",
			Host:       "a.example.com",
			Path:       "/beta",
			StatusCode: 404,
			Reason:     "恶意请求防护拦截: sensitive_paths",
		},
		{
			NodeID:     "node-b",
			LoggedAt:   now.Add(-3 * time.Minute),
			RemoteAddr: "1.1.1.1",
			Region:     "United States",
			Host:       "b.example.com",
			Path:       "/gamma",
			StatusCode: 502,
		},
		{
			NodeID:     "node-b",
			LoggedAt:   now.Add(-2 * time.Minute),
			RemoteAddr: "",
			Host:       "b.example.com",
			Path:       "/delta",
			StatusCode: 200,
		},
	}
	seedNodeAccessLogs(t, logs)

	result, err := ListAccessLogs(AccessLogQuery{Page: 0, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if result.TotalRecord != 4 {
		t.Fatalf("expected total_record=4, got %d", result.TotalRecord)
	}
	if result.TotalIP != 2 {
		t.Fatalf("expected total_ip=2, got %d", result.TotalIP)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected current page items=2, got %d", len(result.Items))
	}
	if result.Items[1].Region == "" {
		t.Fatalf("expected region to be returned, got %+v", result.Items[1])
	}
	if !result.HasMore {
		t.Fatal("expected has_more to be true")
	}

	filtered, err := ListAccessLogs(AccessLogQuery{NodeID: "node-a", Page: 0, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAccessLogs filtered failed: %v", err)
	}
	if filtered.TotalRecord != 2 {
		t.Fatalf("expected filtered total_record=2, got %d", filtered.TotalRecord)
	}
	if filtered.TotalIP != 2 {
		t.Fatalf("expected filtered total_ip=2, got %d", filtered.TotalIP)
	}
	if len(filtered.Items) != 2 {
		t.Fatalf("expected filtered items=2, got %d", len(filtered.Items))
	}
	if filtered.Items[0].Reason != "恶意请求防护拦截: sensitive_paths" {
		t.Fatalf("expected access log reason to be returned, got %+v", filtered.Items[0])
	}
}

func TestListAccessLogsUsesDefaultPageSize(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now()
	if err := model.DB.Create(&model.Node{
		NodeID: "node-default-page-size",
		Name:   "edge-default-page-size",
	}).Error; err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}

	logs := make([]*model.NodeAccessLog, 0, 25)
	for index := range 25 {
		logs = append(logs, &model.NodeAccessLog{
			NodeID:     "node-default-page-size",
			LoggedAt:   now.Add(-time.Duration(index) * time.Minute),
			RemoteAddr: "1.1.1.1",
			Host:       "example.com",
			Path:       "/default-page-size",
			StatusCode: 200,
		})
	}
	seedNodeAccessLogs(t, logs)

	result, err := ListAccessLogs(AccessLogQuery{})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if result.PageSize != 20 {
		t.Fatalf("expected default page_size=20, got %d", result.PageSize)
	}
	if len(result.Items) != 20 {
		t.Fatalf("expected current page items=20, got %d", len(result.Items))
	}
	if !result.HasMore {
		t.Fatal("expected has_more to be true")
	}
}

func TestListFoldedAccessLogsAndIPSummaries(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Date(2026, 3, 19, 8, 12, 30, 0, time.UTC)
	if err := model.DB.Create(&model.Node{
		NodeID: "node-folded",
		Name:   "edge-folded",
	}).Error; err != nil {
		t.Fatalf("failed to seed node: %v", err)
	}
	logs := []*model.NodeAccessLog{
		{
			NodeID:     "node-folded",
			LoggedAt:   now.Add(-4 * time.Minute),
			RemoteAddr: "203.0.113.1",
			Region:     "Hong Kong",
			Operator:   "ExampleNet HK",
			Host:       "alpha.example.com",
			Path:       "/first",
			StatusCode: 200,
		},
		{
			NodeID:     "node-folded",
			LoggedAt:   now.Add(-3 * time.Minute),
			RemoteAddr: "203.0.113.1",
			Region:     "Hong Kong",
			Operator:   "ExampleNet HK",
			Host:       "alpha.example.com",
			Path:       "/second",
			StatusCode: 502,
		},
		{
			NodeID:     "node-folded",
			LoggedAt:   now.Add(-2 * time.Minute),
			RemoteAddr: "203.0.113.2",
			Region:     "Singapore",
			Host:       "beta.example.com",
			Path:       "/third",
			StatusCode: 404,
		},
	}
	seedNodeAccessLogs(t, logs)

	folded, err := ListFoldedAccessLogs(AccessLogQuery{
		NodeID:      "node-folded",
		Page:        0,
		PageSize:    10,
		FoldMinutes: 5,
	})
	if err != nil {
		t.Fatalf("ListFoldedAccessLogs failed: %v", err)
	}
	if len(folded.Items) != 2 {
		t.Fatalf("expected two folded buckets, got %+v", folded.Items)
	}
	if folded.TotalRecord != 3 || folded.TotalBucket != 2 {
		t.Fatalf("unexpected folded totals: %+v", folded)
	}
	if folded.Items[0].RequestCount+folded.Items[1].RequestCount != 3 {
		t.Fatalf("unexpected folded request count sum: %+v", folded.Items)
	}

	ipSummaries, err := ListAccessLogIPSummaries(AccessLogIPSummaryQuery{
		NodeID:    "node-folded",
		Page:      0,
		PageSize:  10,
		SortBy:    "total_requests",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("ListAccessLogIPSummaries failed: %v", err)
	}
	if len(ipSummaries.Items) != 2 {
		t.Fatalf("expected two ip summary rows, got %+v", ipSummaries.Items)
	}
	if ipSummaries.Items[0].RemoteAddr != "203.0.113.1" || ipSummaries.Items[0].TotalRequests != 2 {
		t.Fatalf("unexpected top ip summary row: %+v", ipSummaries.Items[0])
	}
	if ipSummaries.Items[0].Region != "Hong Kong" {
		t.Fatalf("expected top ip summary to include latest region, got %+v", ipSummaries.Items[0])
	}
	if ipSummaries.Items[0].Operator != "ExampleNet HK" {
		t.Fatalf("expected top ip summary to include latest operator, got %+v", ipSummaries.Items[0])
	}
}

func TestCleanupAccessLogsDeletesExpiredData(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-cleanup",
			LoggedAt:   now.Add(-10 * 24 * time.Hour),
			RemoteAddr: "203.0.113.9",
			Host:       "cleanup.example.com",
			Path:       "/old",
			StatusCode: 200,
		},
		{
			NodeID:     "node-cleanup",
			LoggedAt:   now.Add(-2 * 24 * time.Hour),
			RemoteAddr: "203.0.113.10",
			Host:       "cleanup.example.com",
			Path:       "/recent",
			StatusCode: 200,
		},
	})

	result, err := CleanupAccessLogs(AccessLogCleanupInput{RetentionDays: 7})
	if err != nil {
		t.Fatalf("CleanupAccessLogs failed: %v", err)
	}
	if result.DeletedCount != 1 {
		t.Fatalf("expected 1 deleted record, got %+v", result)
	}

	remaining, err := ListAccessLogs(AccessLogQuery{Page: 0, PageSize: 10, NodeID: "node-cleanup"})
	if err != nil {
		t.Fatalf("ListAccessLogs failed after cleanup: %v", err)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].Path != "/recent" {
		t.Fatalf("unexpected remaining logs after cleanup: %+v", remaining.Items)
	}
}

func TestBuildObservabilityMeteringOverviewAggregatesBillingSignals(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	view := buildObservabilityMeteringOverview(meteringOverviewDataSource{
		now: now,
		nodes: []*model.Node{
			{NodeID: "node-a", Name: "AKKO HK", Status: NodeStatusOnline, LastSeenAt: now},
			{NodeID: "node-b", Name: "AKKO DE", Status: NodeStatusOffline, LastSeenAt: now.Add(-3 * time.Hour)},
		},
		logs: []*model.NodeAccessLog{
			{
				NodeID:        "node-a",
				LoggedAt:      now.Add(-10 * time.Minute),
				RemoteAddr:    "203.0.113.1",
				Region:        "China",
				Host:          "app.example.com",
				Path:          "/index",
				StatusCode:    200,
				RequestBytes:  100,
				ResponseBytes: 1000,
				UpstreamBytes: 300,
			},
			{
				NodeID:        "node-b",
				LoggedAt:      now.Add(-8 * time.Minute),
				RemoteAddr:    "203.0.113.2",
				Region:        "United States",
				Host:          "app.example.com",
				Path:          "/api",
				StatusCode:    502,
				RequestBytes:  200,
				ResponseBytes: 2000,
				UpstreamBytes: 900,
			},
			{
				NodeID:        "node-a",
				LoggedAt:      now.Add(-6 * time.Minute),
				RemoteAddr:    "203.0.113.1",
				Region:        "China",
				Host:          "static.example.com",
				Path:          "/asset.js",
				StatusCode:    200,
				ResponseBytes: 500,
			},
		},
		reports: []*model.NodeRequestReport{
			{
				CacheHitCount:    7,
				CacheMissCount:   2,
				CacheBypassCount: 1,
				StatusCodesJSON:  `{"200":8,"502":2}`,
			},
		},
		snapshots: []*model.NodeMetricSnapshot{
			{NodeID: "node-a", CapturedAt: now.Add(-2 * time.Hour), OpenrestyRxBytes: 1000, OpenrestyTxBytes: 2000},
			{NodeID: "node-a", CapturedAt: now.Add(-1 * time.Hour), OpenrestyRxBytes: 1500, OpenrestyTxBytes: 3200},
			{NodeID: "node-a", CapturedAt: now, OpenrestyRxBytes: 2200, OpenrestyTxBytes: 5200},
		},
	})

	if view.RequestCount != 3 {
		t.Fatalf("expected request count 3, got %d", view.RequestCount)
	}
	if view.ResponseBytes != 3500 || view.UpstreamBytes != 1200 || !view.UpstreamBytesSupported {
		t.Fatalf("unexpected traffic bytes: %+v", view)
	}
	if view.CacheHitRatePercent != 70 {
		t.Fatalf("expected cache hit rate 70, got %f", view.CacheHitRatePercent)
	}
	if view.CacheHitCount != 7 || view.CacheMissCount != 2 || view.CacheBypassCount != 1 || view.CacheClassifiedCount != 10 {
		t.Fatalf("expected cache status breakdown, got %+v", view)
	}
	if view.NodeAvailabilityPercent != 50 {
		t.Fatalf("expected availability 50, got %f", view.NodeAvailabilityPercent)
	}
	if len(view.SiteTraffic) == 0 || view.SiteTraffic[0].Key != "app.example.com" || view.SiteTraffic[0].ResponseBytes != 3000 {
		t.Fatalf("unexpected site traffic: %+v", view.SiteTraffic)
	}
	if len(view.NodeTraffic) == 0 || view.NodeTraffic[0].Key != "AKKO DE" {
		t.Fatalf("expected node traffic to use node display names, got %+v", view.NodeTraffic)
	}
	if len(view.TopURLs) == 0 || view.TopURLs[0].Key == "" {
		t.Fatalf("expected top URLs, got %+v", view.TopURLs)
	}
	if view.BandwidthP95Bps <= 0 {
		t.Fatalf("expected positive p95 bandwidth, got %f", view.BandwidthP95Bps)
	}

	aggregatedView := buildAggregatedObservabilityMeteringOverview(meteringOverviewAggregatedDataSource{
		now: now,
		nodes: []*model.Node{
			{NodeID: "node-a", Name: "AKKO HK", Status: NodeStatusOnline, LastSeenAt: now},
			{NodeID: "node-b", Name: "AKKO DE", Status: NodeStatusOffline, LastSeenAt: now.Add(-3 * time.Hour)},
		},
		summary: &model.NodeAccessLogMeteringSummary{
			RequestCount:          3,
			RequestBytes:          300,
			ResponseBytes:         3500,
			UpstreamBytes:         1200,
			UpstreamBytesHitCount: 2,
		},
		statusCodes: []*model.NodeAccessLogDistributionRow{
			{Key: "200", Value: 2},
			{Key: "502", Value: 1},
		},
		topURLs: []*model.NodeAccessLogDistributionRow{
			{Key: "app.example.com/index", Value: 1},
		},
		topIPs: []*model.NodeAccessLogDistributionRow{
			{Key: "203.0.113.1", Value: 2},
		},
		topRegions: []*model.NodeAccessLogDistributionRow{
			{Key: "China", Value: 2},
		},
		siteTraffic: []*model.NodeAccessLogMeteringTrafficRow{
			{Key: "app.example.com", RequestCount: 2, RequestBytes: 300, ResponseBytes: 3000, UpstreamBytes: 1200},
			{Key: "static.example.com", RequestCount: 1, ResponseBytes: 500},
		},
		nodeTraffic: []*model.NodeAccessLogMeteringTrafficRow{
			{Key: "node-b", RequestCount: 1, RequestBytes: 200, ResponseBytes: 2000, UpstreamBytes: 900},
			{Key: "node-a", RequestCount: 2, RequestBytes: 100, ResponseBytes: 1500, UpstreamBytes: 300},
		},
		cache: &model.NodeRequestReportCacheSummary{
			CacheHitCount:        7,
			CacheMissCount:       2,
			CacheBypassCount:     1,
			CacheClassifiedCount: 10,
		},
		bandwidth: []*model.NodeMetricSnapshotCounterDeltaBucket{
			{
				BucketEpoch:      now.Add(-1 * time.Hour).Truncate(time.Hour).Unix(),
				OpenrestyRxBytes: 500,
				OpenrestyTxBytes: 1200,
			},
			{
				BucketEpoch:      now.Truncate(time.Hour).Unix(),
				OpenrestyRxBytes: 700,
				OpenrestyTxBytes: 2000,
			},
		},
	})
	if aggregatedView.RequestCount != view.RequestCount ||
		aggregatedView.ResponseBytes != view.ResponseBytes ||
		aggregatedView.UpstreamBytes != view.UpstreamBytes ||
		aggregatedView.CacheHitRatePercent != view.CacheHitRatePercent ||
		aggregatedView.NodeAvailabilityPercent != view.NodeAvailabilityPercent {
		t.Fatalf("aggregated overview diverged: aggregated=%+v memory=%+v", aggregatedView, view)
	}
	if len(aggregatedView.SiteTraffic) == 0 || aggregatedView.SiteTraffic[0].Key != "app.example.com" || aggregatedView.SiteTraffic[0].ResponseBytes != 3000 {
		t.Fatalf("unexpected aggregated site traffic: %+v", aggregatedView.SiteTraffic)
	}
	if len(aggregatedView.NodeTraffic) == 0 || aggregatedView.NodeTraffic[0].Key != "AKKO DE" {
		t.Fatalf("expected aggregated node traffic to use node display names, got %+v", aggregatedView.NodeTraffic)
	}
}

func TestGetObservabilityMeteringOverviewUsesAggregatedAccessLogs(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC().Truncate(time.Hour)
	if err := model.DB.Create(&model.Node{
		NodeID:     "node-metering-a",
		Name:       "Metering A",
		Status:     NodeStatusOnline,
		LastSeenAt: now,
	}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:        "node-metering-a",
			LoggedAt:      now.Add(-10 * time.Minute),
			RemoteAddr:    "203.0.113.80",
			Region:        "HK",
			Host:          "metering.example.com",
			Path:          "/index",
			StatusCode:    200,
			RequestBytes:  100,
			ResponseBytes: 1000,
			UpstreamBytes: 200,
		},
		{
			NodeID:        "node-metering-a",
			LoggedAt:      now.Add(-5 * time.Minute),
			RemoteAddr:    "203.0.113.81",
			Region:        "HK",
			Host:          "metering.example.com",
			Path:          "/api",
			StatusCode:    502,
			RequestBytes:  200,
			ResponseBytes: 2500,
			UpstreamBytes: 700,
		},
	})
	if err := model.DB.Create(&model.NodeRequestReport{
		NodeID:              "node-metering-a",
		WindowStartedAt:     now.Add(-time.Hour),
		WindowEndedAt:       now,
		CacheHitCount:       3,
		CacheMissCount:      1,
		StatusCodesJSON:     `{"200":1,"502":1}`,
		TopDomainsJSON:      `{"metering.example.com":2}`,
		SourceCountriesJSON: `{"HK":2}`,
	}).Error; err != nil {
		t.Fatalf("seed request report: %v", err)
	}
	for _, snapshot := range []*model.NodeMetricSnapshot{
		{
			NodeID:           "node-metering-a",
			CapturedAt:       now.Add(-time.Hour),
			OpenrestyRxBytes: 100,
			OpenrestyTxBytes: 200,
		},
		{
			NodeID:           "node-metering-a",
			CapturedAt:       now,
			OpenrestyRxBytes: 500,
			OpenrestyTxBytes: 900,
		},
	} {
		if err := snapshot.Insert(); err != nil {
			t.Fatalf("seed metric snapshot: %v", err)
		}
	}

	view, err := GetObservabilityMeteringOverview()
	if err != nil {
		t.Fatalf("GetObservabilityMeteringOverview failed: %v", err)
	}
	if view.RequestCount != 2 || view.ResponseBytes != 3500 || view.UpstreamBytes != 900 {
		t.Fatalf("unexpected aggregated metering totals: %+v", view)
	}
	if len(view.SiteTraffic) == 0 || view.SiteTraffic[0].Key != "metering.example.com" || view.SiteTraffic[0].ResponseBytes != 3500 {
		t.Fatalf("unexpected aggregated site traffic: %+v", view.SiteTraffic)
	}
	if len(view.NodeTraffic) == 0 || view.NodeTraffic[0].Key != "Metering A" {
		t.Fatalf("unexpected aggregated node traffic: %+v", view.NodeTraffic)
	}
	if len(view.StatusCodes) == 0 || view.StatusCodes[0].Key == "" {
		t.Fatalf("expected aggregated status codes, got %+v", view.StatusCodes)
	}
	if view.CacheHitRatePercent != 75 {
		t.Fatalf("expected cache hit rate from request reports, got %f", view.CacheHitRatePercent)
	}
	if view.BandwidthP95Bps <= 0 {
		t.Fatalf("expected aggregated bandwidth p95 from counter buckets, got %f", view.BandwidthP95Bps)
	}
}

func TestPersistNodeAccessLogsTruncatesLongPath(t *testing.T) {
	setupServiceTestDB(t)

	longPath := "/" + strings.Repeat("a", 140)
	longReason := "恶意请求防护拦截: " + strings.Repeat("sensitive_paths", 80)
	reportedAt := time.Now().UTC()
	if err := persistNodeAccessLogs(model.DB, "node-truncate", []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "203.0.113.10",
			Host:         "truncate.example.com",
			Path:         longPath,
			StatusCode:   200,
			Reason:       longReason,
		},
	}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs failed: %v", err)
	}

	logs, err := model.ListNodeAccessLogs(model.NodeAccessLogQuery{
		NodeID:   "node-truncate",
		Page:     0,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one stored log, got %+v", logs)
	}
	if got := len([]rune(logs[0].Path)); got != nodeAccessLogPathMaxLength {
		t.Fatalf("expected truncated path length %d, got %d (%q)", nodeAccessLogPathMaxLength, got, logs[0].Path)
	}
	if got := len([]rune(logs[0].Reason)); got != 512 {
		t.Fatalf("expected truncated reason length 512, got %d", got)
	}
}

func TestPersistNodeAccessLogsStoresCacheStatus(t *testing.T) {
	setupServiceTestDB(t)

	reportedAt := time.Now().UTC()
	if err := persistNodeAccessLogs(model.DB, "node-cache-status", []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "203.0.113.20",
			Host:         "cache.example.com",
			Path:         "/emby/Items/12039/Images/Primary",
			StatusCode:   200,
			CacheStatus:  "hit",
		},
	}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs failed: %v", err)
	}

	view, err := ListAccessLogs(AccessLogQuery{
		NodeID:   "node-cache-status",
		Page:     0,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if len(view.Items) != 1 || view.Items[0].CacheStatus != "HIT" {
		t.Fatalf("expected cache status in access log view, got %+v", view.Items)
	}
}

func TestPersistNodeAccessLogsDeduplicatesBatch(t *testing.T) {
	setupServiceTestDB(t)

	reportedAt := time.Now().UTC()
	log := AgentNodeAccessLog{
		LoggedAtUnix: reportedAt.Unix(),
		RemoteAddr:   "203.0.113.30",
		Host:         "dedupe.example.com",
		Path:         "/same",
		StatusCode:   200,
	}
	if err := persistNodeAccessLogs(model.DB, "node-dedupe", []AgentNodeAccessLog{log, log}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs failed: %v", err)
	}
	if err := persistNodeAccessLogs(model.DB, "node-dedupe", []AgentNodeAccessLog{log}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs second call failed: %v", err)
	}

	totalRecords, totalIPs, err := model.CountNodeAccessLogs(model.NodeAccessLogQuery{NodeID: "node-dedupe"})
	if err != nil {
		t.Fatalf("CountNodeAccessLogs failed: %v", err)
	}
	if totalRecords != 1 || totalIPs != 1 {
		t.Fatalf("expected one deduped access log and IP, got records=%d ips=%d", totalRecords, totalIPs)
	}
}

func seedNodeAccessLogs(t *testing.T, logs []*model.NodeAccessLog) {
	t.Helper()
	for _, item := range logs {
		if err := model.DB.Create(item).Error; err != nil {
			t.Fatalf("failed to seed access log: %v", err)
		}
	}
}
