package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func withAccessLogMeteringEnabled(t *testing.T) {
	t.Helper()
	previous := common.AccessLogPersistenceEnabled
	common.AccessLogPersistenceEnabled = true
	t.Cleanup(func() {
		common.AccessLogPersistenceEnabled = previous
	})
}

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

func TestLoadAccessLogCountWithCacheCoalescesConcurrentLoads(t *testing.T) {
	resetAccessLogCountCacheForTest()
	t.Cleanup(resetAccessLogCountCacheForTest)

	const callers = 12
	key := "count-coalesce:" + t.Name()
	var calls atomic.Int32
	ready := make(chan struct{}, callers)
	start := make(chan struct{})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup

	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			totalRecords, totalIPs, err := loadAccessLogCountWithCache(key, func() (int64, int64, error) {
				if calls.Add(1) == 1 {
					entered <- struct{}{}
				}
				<-release
				return 42, 7, nil
			})
			if err != nil {
				errs <- err
				return
			}
			if totalRecords != 42 || totalIPs != 7 {
				errs <- errors.New("unexpected coalesced access log count")
			}
		}()
	}
	for range callers {
		<-ready
	}
	close(start)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("timed out waiting for access log count loader")
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("coalesced access log count failed: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one access log count load, got %d", calls.Load())
	}

	totalRecords, totalIPs, err := loadAccessLogCountWithCache(key, func() (int64, int64, error) {
		calls.Add(1)
		return 0, 0, errors.New("cached count should not reload")
	})
	if err != nil {
		t.Fatalf("cached access log count failed: %v", err)
	}
	if totalRecords != 42 || totalIPs != 7 || calls.Load() != 1 {
		t.Fatalf("expected cached access log count, got records=%d ips=%d calls=%d", totalRecords, totalIPs, calls.Load())
	}
}

func TestLoadAccessLogSingleCountWithCacheCoalescesConcurrentLoads(t *testing.T) {
	cache := &accessLogSingleCountCacheStore{values: make(map[string]accessLogSingleCountCacheEntry)}

	const callers = 12
	key := "single-count-coalesce:" + t.Name()
	var calls atomic.Int32
	ready := make(chan struct{}, callers)
	start := make(chan struct{})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	errs := make(chan error, callers)
	var wg sync.WaitGroup

	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			count, err := loadAccessLogSingleCountWithCache(cache, key, func() (int64, error) {
				if calls.Add(1) == 1 {
					entered <- struct{}{}
				}
				<-release
				return 9, nil
			})
			if err != nil {
				errs <- err
				return
			}
			if count != 9 {
				errs <- errors.New("unexpected coalesced access log single count")
			}
		}()
	}
	for range callers {
		<-ready
	}
	close(start)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("timed out waiting for access log single count loader")
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("coalesced access log single count failed: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one access log single count load, got %d", calls.Load())
	}

	count, err := loadAccessLogSingleCountWithCache(cache, key, func() (int64, error) {
		calls.Add(1)
		return 0, errors.New("cached single count should not reload")
	})
	if err != nil {
		t.Fatalf("cached access log single count failed: %v", err)
	}
	if count != 9 || calls.Load() != 1 {
		t.Fatalf("expected cached access log single count, got count=%d calls=%d", count, calls.Load())
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

func TestListAccessLogsReusesCountAcrossPages(t *testing.T) {
	setupServiceTestDB(t)
	resetAccessLogCountCacheForTest()

	now := time.Now()
	logs := make([]*model.NodeAccessLog, 0, 5)
	for index := range 5 {
		logs = append(logs, &model.NodeAccessLog{
			NodeID:     "node-count-cache",
			LoggedAt:   now.Add(-time.Duration(index) * time.Minute),
			RemoteAddr: "203.0.113." + strconv.Itoa(index+1),
			Host:       "cache.example.com",
			Path:       "/count-cache",
			StatusCode: 200,
		})
	}
	seedNodeAccessLogs(t, logs)

	firstPage, err := ListAccessLogs(AccessLogQuery{
		NodeID:   "node-count-cache",
		Page:     0,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs first page failed: %v", err)
	}
	if firstPage.TotalRecord != 5 || firstPage.TotalIP != 5 {
		t.Fatalf("unexpected first page totals: %+v", firstPage)
	}

	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-count-cache",
			LoggedAt:   now.Add(time.Minute),
			RemoteAddr: "203.0.113.99",
			Host:       "cache.example.com",
			Path:       "/count-cache/new",
			StatusCode: 200,
		},
	})

	secondPage, err := ListAccessLogs(AccessLogQuery{
		NodeID:   "node-count-cache",
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs second page failed: %v", err)
	}
	if secondPage.TotalRecord != 5 || secondPage.TotalIP != 5 {
		t.Fatalf("expected cached totals from first page, got %+v", secondPage)
	}
	if len(secondPage.Items) != 2 || !secondPage.HasMore {
		t.Fatalf("expected lookahead pagination to keep page shape and has_more, got %+v", secondPage)
	}
}

func TestListAccessLogsUsesCursorForStableKeysetPagination(t *testing.T) {
	setupServiceTestDB(t)
	resetAccessLogCountCacheForTest()

	now := time.Now().UTC().Truncate(time.Second)
	logs := make([]*model.NodeAccessLog, 0, 5)
	for index := range 5 {
		logs = append(logs, &model.NodeAccessLog{
			NodeID:     "node-cursor",
			LoggedAt:   now.Add(-time.Duration(index) * time.Minute),
			RemoteAddr: "203.0.113." + strconv.Itoa(index+1),
			Host:       "cursor.example.com",
			Path:       "/cursor/" + strconv.Itoa(index),
			StatusCode: 200,
		})
	}
	seedNodeAccessLogs(t, logs)

	firstPage, err := ListAccessLogs(AccessLogQuery{
		NodeID:   "node-cursor",
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs first page failed: %v", err)
	}
	if len(firstPage.Items) != 2 || firstPage.Items[0].Path != "/cursor/0" || firstPage.Items[1].Path != "/cursor/1" {
		t.Fatalf("unexpected first cursor page: %+v", firstPage.Items)
	}
	if firstPage.NextCursor == "" || !firstPage.HasMore {
		t.Fatalf("expected first cursor page to expose next cursor, got %+v", firstPage)
	}

	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-cursor",
			LoggedAt:   now.Add(time.Minute),
			RemoteAddr: "203.0.113.99",
			Host:       "cursor.example.com",
			Path:       "/cursor/newer",
			StatusCode: 200,
		},
	})
	resetAccessLogCountCacheForTest()

	secondPage, err := ListAccessLogs(AccessLogQuery{
		NodeID:   "node-cursor",
		PageSize: 2,
		Cursor:   firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs second cursor page failed: %v", err)
	}
	if secondPage.TotalRecord != 6 {
		t.Fatalf("expected cursor count to include all matching records, got %+v", secondPage)
	}
	if len(secondPage.Items) != 2 || secondPage.Items[0].Path != "/cursor/2" || secondPage.Items[1].Path != "/cursor/3" {
		t.Fatalf("expected cursor page not to drift after newer insert, got %+v", secondPage.Items)
	}
	if secondPage.NextCursor == "" || !secondPage.HasMore {
		t.Fatalf("expected second cursor page to expose next cursor, got %+v", secondPage)
	}
}

func TestListAccessLogsRejectsInvalidCursor(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := ListAccessLogs(AccessLogQuery{Cursor: "not-a-valid-cursor"}); err == nil || !strings.Contains(err.Error(), "invalid access log cursor") {
		t.Fatalf("expected invalid cursor error, got %v", err)
	}
}

func TestListAccessLogsFiltersByHostDomain(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now()
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-domain-filter",
			LoggedAt:   now.Add(-3 * time.Minute),
			RemoteAddr: "47.86.192.73",
			Host:       "www.satandu.com",
			Path:       "/api/dns-worker-heartbeat",
			StatusCode: 200,
		},
		{
			NodeID:     "node-domain-filter",
			LoggedAt:   now.Add(-2 * time.Minute),
			RemoteAddr: "129.226.213.145",
			Host:       "8.211.168.34",
			Path:       "/",
			StatusCode: 404,
		},
		{
			NodeID:     "node-domain-filter",
			LoggedAt:   now.Add(-1 * time.Minute),
			RemoteAddr: "47.86.192.73",
			Host:       "api.satandu.com",
			Path:       "/api/dns-snapshot",
			StatusCode: 200,
		},
		{
			NodeID:     "node-domain-filter",
			LoggedAt:   now,
			RemoteAddr: "47.86.192.74",
			Host:       "not-satandu.com",
			Path:       "/noise",
			StatusCode: 200,
		},
	})

	result, err := ListAccessLogs(AccessLogQuery{
		Host:     "satandu.com",
		Page:     0,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if result.TotalRecord != 2 || len(result.Items) != 2 {
		t.Fatalf("expected only satandu.com hosts, got %+v", result)
	}
	for _, item := range result.Items {
		if !strings.Contains(item.Host, "satandu.com") {
			t.Fatalf("expected domain filter to exclude unrelated host, got %+v", result.Items)
		}
	}
}

func TestListAccessLogsFiltersPersistedNormalizedHost(t *testing.T) {
	setupServiceTestDB(t)

	reportedAt := time.Now().UTC()
	if err := persistNodeAccessLogs(model.DB, "node-host-normalized", []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "203.0.113.40",
			Host:         " WWW.SATANDU.COM ",
			Path:         "/normalized-host",
			StatusCode:   200,
		},
		{
			LoggedAtUnix: reportedAt.Add(time.Second).Unix(),
			RemoteAddr:   "203.0.113.41",
			Host:         "not-satandu.com",
			Path:         "/noise",
			StatusCode:   200,
		},
	}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs failed: %v", err)
	}

	result, err := ListAccessLogs(AccessLogQuery{
		Host:     "satandu.com",
		Page:     0,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if result.TotalRecord != 1 || len(result.Items) != 1 {
		t.Fatalf("expected one normalized host match, got %+v", result)
	}
	if result.Items[0].Host != "www.satandu.com" {
		t.Fatalf("expected persisted host to be normalized, got %+v", result.Items[0])
	}
}

func TestListAccessLogsNormalizesHostAndRemoteFilters(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now().UTC()
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-normalized-filter",
			LoggedAt:   now,
			RemoteAddr: "203.0.113.70",
			Host:       "www.satandu.com",
			Path:       "/normalized-filter",
			StatusCode: 200,
		},
		{
			NodeID:     "node-normalized-filter",
			LoggedAt:   now.Add(time.Second),
			RemoteAddr: "203.0.113.71",
			Host:       "www.satandu.com",
			Path:       "/noise",
			StatusCode: 200,
		},
	})

	result, err := ListAccessLogs(AccessLogQuery{
		NodeID:     "node-normalized-filter",
		RemoteAddr: "203.0.113.70:443",
		Host:       "https://WWW.SATANDU.COM:443/some/path?token=secret",
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("ListAccessLogs failed: %v", err)
	}
	if result.TotalRecord != 1 || len(result.Items) != 1 {
		t.Fatalf("expected normalized host and remote filters to match one row, got %+v", result)
	}
	if result.Items[0].Path != "/normalized-filter" {
		t.Fatalf("unexpected normalized filter result: %+v", result.Items)
	}
}

func TestListFoldedAccessLogsReusesCountAcrossPages(t *testing.T) {
	setupServiceTestDB(t)
	resetAccessLogCountCacheForTest()

	base := time.Now().UTC().Truncate(5 * time.Minute)
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-folded-count-cache",
			LoggedAt:   base,
			RemoteAddr: "203.0.113.50",
			Host:       "folded-count.example.com",
			Path:       "/first",
			StatusCode: 200,
		},
		{
			NodeID:     "node-folded-count-cache",
			LoggedAt:   base.Add(5 * time.Minute),
			RemoteAddr: "203.0.113.51",
			Host:       "folded-count.example.com",
			Path:       "/second",
			StatusCode: 200,
		},
	})

	firstPage, err := ListFoldedAccessLogs(AccessLogQuery{
		NodeID:      "node-folded-count-cache",
		Page:        0,
		PageSize:    1,
		SortOrder:   "asc",
		FoldMinutes: 5,
	})
	if err != nil {
		t.Fatalf("ListFoldedAccessLogs first page failed: %v", err)
	}
	if firstPage.TotalBucket != 2 || firstPage.TotalRecord != 2 || firstPage.TotalIP != 2 {
		t.Fatalf("unexpected first page totals: %+v", firstPage)
	}
	if len(firstPage.Items) != 1 || !firstPage.HasMore {
		t.Fatalf("expected first folded page to use lookahead, got %+v", firstPage)
	}

	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-folded-count-cache",
			LoggedAt:   base.Add(10 * time.Minute),
			RemoteAddr: "203.0.113.52",
			Host:       "folded-count.example.com",
			Path:       "/third",
			StatusCode: 200,
		},
	})

	secondPage, err := ListFoldedAccessLogs(AccessLogQuery{
		NodeID:      "node-folded-count-cache",
		Page:        1,
		PageSize:    1,
		SortOrder:   "asc",
		FoldMinutes: 5,
	})
	if err != nil {
		t.Fatalf("ListFoldedAccessLogs second page failed: %v", err)
	}
	if secondPage.TotalBucket != 2 || secondPage.TotalRecord != 2 || secondPage.TotalIP != 2 {
		t.Fatalf("expected cached folded totals, got %+v", secondPage)
	}
	if len(secondPage.Items) != 1 || !secondPage.Items[0].BucketStartedAt.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("expected second page to preserve original offset, got %+v", secondPage.Items)
	}
	if !secondPage.HasMore {
		t.Fatalf("expected folded lookahead to see newly inserted third page, got %+v", secondPage)
	}
}

func TestListAccessLogIPSummariesReusesCountAcrossPages(t *testing.T) {
	setupServiceTestDB(t)
	resetAccessLogCountCacheForTest()

	base := time.Now().UTC()
	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-ip-count-cache",
			LoggedAt:   base,
			RemoteAddr: "203.0.113.61",
			Host:       "ip-count.example.com",
			Path:       "/first",
			StatusCode: 200,
		},
		{
			NodeID:     "node-ip-count-cache",
			LoggedAt:   base.Add(time.Second),
			RemoteAddr: "203.0.113.62",
			Host:       "ip-count.example.com",
			Path:       "/second",
			StatusCode: 200,
		},
		{
			NodeID:     "node-ip-count-cache",
			LoggedAt:   base.Add(2 * time.Second),
			RemoteAddr: "203.0.113.63",
			Host:       "ip-count.example.com",
			Path:       "/third",
			StatusCode: 200,
		},
	})

	firstPage, err := ListAccessLogIPSummaries(AccessLogIPSummaryQuery{
		NodeID:    "node-ip-count-cache",
		Page:      0,
		PageSize:  1,
		SortBy:    "remote_addr",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListAccessLogIPSummaries first page failed: %v", err)
	}
	if firstPage.TotalIP != 3 || len(firstPage.Items) != 1 || !firstPage.HasMore {
		t.Fatalf("unexpected first ip summary page: %+v", firstPage)
	}

	seedNodeAccessLogs(t, []*model.NodeAccessLog{
		{
			NodeID:     "node-ip-count-cache",
			LoggedAt:   base.Add(3 * time.Second),
			RemoteAddr: "203.0.113.64",
			Host:       "ip-count.example.com",
			Path:       "/fourth",
			StatusCode: 200,
		},
	})

	secondPage, err := ListAccessLogIPSummaries(AccessLogIPSummaryQuery{
		NodeID:    "node-ip-count-cache",
		Page:      1,
		PageSize:  1,
		SortBy:    "remote_addr",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListAccessLogIPSummaries second page failed: %v", err)
	}
	if secondPage.TotalIP != 3 {
		t.Fatalf("expected cached ip summary total, got %+v", secondPage)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].RemoteAddr != "203.0.113.62" {
		t.Fatalf("expected second page to preserve original offset, got %+v", secondPage.Items)
	}
	if !secondPage.HasMore {
		t.Fatalf("expected ip summary lookahead to see another page, got %+v", secondPage)
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
	view := buildAggregatedObservabilityMeteringOverview(meteringOverviewAggregatedDataSource{
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
		t.Fatalf("unexpected aggregated site traffic: %+v", view.SiteTraffic)
	}
	if len(view.NodeTraffic) == 0 || view.NodeTraffic[0].Key != "AKKO DE" {
		t.Fatalf("expected aggregated node traffic to use node display names, got %+v", view.NodeTraffic)
	}
	if len(view.TopURLs) == 0 || view.TopURLs[0].Key == "" {
		t.Fatalf("expected top URLs, got %+v", view.TopURLs)
	}
	if view.BandwidthP95Bps <= 0 {
		t.Fatalf("expected positive p95 bandwidth, got %f", view.BandwidthP95Bps)
	}
}

func TestBuildAggregatedObservabilityMeteringOverviewFallsBackToAccessLogCacheSummary(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	view := buildAggregatedObservabilityMeteringOverview(meteringOverviewAggregatedDataSource{
		now: now,
		summary: &model.NodeAccessLogMeteringSummary{
			RequestCount:         3,
			CacheHitCount:        1,
			CacheMissCount:       1,
			CacheBypassCount:     1,
			CacheClassifiedCount: 3,
		},
		cache: &model.NodeRequestReportCacheSummary{},
	})
	if view.CacheHitCount != 1 || view.CacheMissCount != 1 || view.CacheBypassCount != 1 || view.CacheClassifiedCount != 3 {
		t.Fatalf("expected access-log cache summary fallback, got %+v", view)
	}
	if view.CacheHitRatePercent < 33.3 || view.CacheHitRatePercent > 33.4 {
		t.Fatalf("expected cache hit rate from access logs, got %f", view.CacheHitRatePercent)
	}
}

func TestGetObservabilityMeteringOverviewUsesAggregatedAccessLogs(t *testing.T) {
	setupServiceTestDB(t)
	withAccessLogMeteringEnabled(t)

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

func TestObservabilityMeteringOverviewCacheReusesRecentResult(t *testing.T) {
	setupServiceTestDB(t)

	first, err := GetObservabilityMeteringOverview()
	if err != nil {
		t.Fatalf("GetObservabilityMeteringOverview failed: %v", err)
	}
	if err := model.DB.Create(&model.Node{
		NodeID: "node-metering-cache",
		Name:   "Metering Cache",
		Status: NodeStatusOnline,
	}).Error; err != nil {
		t.Fatalf("seed node after cache: %v", err)
	}
	second, err := GetObservabilityMeteringOverview()
	if err != nil {
		t.Fatalf("GetObservabilityMeteringOverview cached call failed: %v", err)
	}
	if first != second {
		t.Fatal("expected recent metering overview calls to reuse cached result")
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

func TestPersistNodeAccessLogsDropsSensitiveQuery(t *testing.T) {
	setupServiceTestDB(t)

	reportedAt := time.Now().UTC()
	if err := persistNodeAccessLogs(model.DB, "node-query-redact", []AgentNodeAccessLog{
		{
			LoggedAtUnix: reportedAt.Unix(),
			RemoteAddr:   "203.0.113.11",
			Host:         "query.example.com",
			Path:         "/oauth/callback?code=oauth-code&state=csrf-state&safe=1#fragment",
			StatusCode:   302,
		},
		{
			LoggedAtUnix: reportedAt.Add(time.Second).Unix(),
			RemoteAddr:   "203.0.113.12",
			Host:         "query.example.com",
			Path:         "https://query.example.com/reset?token=reset-token",
			StatusCode:   200,
		},
	}, reportedAt); err != nil {
		t.Fatalf("persistNodeAccessLogs failed: %v", err)
	}

	logs, err := model.ListNodeAccessLogs(model.NodeAccessLogQuery{
		NodeID:   "node-query-redact",
		Page:     0,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListNodeAccessLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected two stored logs, got %+v", logs)
	}
	for _, entry := range logs {
		if strings.Contains(entry.Path, "oauth-code") || strings.Contains(entry.Path, "csrf-state") || strings.Contains(entry.Path, "reset-token") || strings.ContainsAny(entry.Path, "?#") {
			t.Fatalf("expected sensitive query to be removed, got %q", entry.Path)
		}
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
