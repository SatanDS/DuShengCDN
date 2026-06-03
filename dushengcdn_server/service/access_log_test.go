package service

import (
	"dushengcdn/model"
	"strings"
	"testing"
	"time"
)

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
			Host:       "alpha.example.com",
			Path:       "/first",
			StatusCode: 200,
		},
		{
			NodeID:     "node-folded",
			LoggedAt:   now.Add(-3 * time.Minute),
			RemoteAddr: "203.0.113.1",
			Region:     "Hong Kong",
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

func seedNodeAccessLogs(t *testing.T, logs []*model.NodeAccessLog) {
	t.Helper()
	for _, item := range logs {
		if err := model.DB.Create(item).Error; err != nil {
			t.Fatalf("failed to seed access log: %v", err)
		}
	}
}
