package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultAccessLogPageSize   = 20
	maxAccessLogPageSize       = 200
	defaultAccessLogSortBy     = "logged_at"
	defaultAccessLogSortOrder  = "desc"
	defaultAccessLogFoldMinute = 3
	defaultIPTrendHours        = 24
	defaultIPTrendBucketMinute = 30
	maxIPTrendHours            = 168
	nodeAccessLogRetentionDays = 90
)

const (
	accessLogCountCacheTTL        = 30 * time.Second
	accessLogCountCacheMaxEntries = 512
)

type AccessLogQuery struct {
	NodeID      string `json:"node_id"`
	RemoteAddr  string `json:"remote_addr"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	Cursor      string `json:"cursor"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
	SortBy      string `json:"sort_by"`
	SortOrder   string `json:"sort_order"`
	FoldMinutes int    `json:"fold_minutes"`
}

type AccessLogView struct {
	ID          uint      `json:"id"`
	NodeID      string    `json:"node_id"`
	NodeName    string    `json:"node_name"`
	LoggedAt    time.Time `json:"logged_at"`
	RemoteAddr  string    `json:"remote_addr"`
	Region      string    `json:"region"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	StatusCode  int       `json:"status_code"`
	Reason      string    `json:"reason"`
	CacheStatus string    `json:"cache_status"`
}

type AccessLogList struct {
	Items       []AccessLogView `json:"items"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
	HasMore     bool            `json:"has_more"`
	NextCursor  string          `json:"next_cursor,omitempty"`
	TotalRecord int64           `json:"total_record"`
	TotalIP     int64           `json:"total_ip"`
}

type accessLogCursor struct {
	LoggedAt time.Time `json:"logged_at"`
	ID       uint      `json:"id"`
}

type accessLogCountCacheEntry struct {
	totalRecords int64
	totalIPs     int64
	expiresAt    time.Time
}

type accessLogCountLoadResult struct {
	totalRecords int64
	totalIPs     int64
}

type accessLogSingleCountCacheEntry struct {
	total     int64
	expiresAt time.Time
}

type accessLogSingleCountCacheStore struct {
	sync.Mutex
	values map[string]accessLogSingleCountCacheEntry
	group  singleflight.Group
}

var accessLogCountCache = struct {
	sync.Mutex
	values map[string]accessLogCountCacheEntry
}{
	values: make(map[string]accessLogCountCacheEntry),
}

var accessLogCountLoadGroup singleflight.Group

var accessLogBucketCountCache = accessLogSingleCountCacheStore{
	values: make(map[string]accessLogSingleCountCacheEntry),
}

var accessLogIPSummaryCountCache = accessLogSingleCountCacheStore{
	values: make(map[string]accessLogSingleCountCacheEntry),
}

type FoldedAccessLogView struct {
	BucketStartedAt  time.Time `json:"bucket_started_at"`
	RequestCount     int64     `json:"request_count"`
	UniqueIPCount    int64     `json:"unique_ip_count"`
	UniqueHostCount  int64     `json:"unique_host_count"`
	SuccessCount     int64     `json:"success_count"`
	ClientErrorCount int64     `json:"client_error_count"`
	ServerErrorCount int64     `json:"server_error_count"`
}

type FoldedAccessLogList struct {
	Items       []FoldedAccessLogView `json:"items"`
	Page        int                   `json:"page"`
	PageSize    int                   `json:"page_size"`
	HasMore     bool                  `json:"has_more"`
	TotalBucket int64                 `json:"total_bucket"`
	TotalRecord int64                 `json:"total_record"`
	TotalIP     int64                 `json:"total_ip"`
	FoldMinutes int                   `json:"fold_minutes"`
}

type AccessLogIPSummaryQuery struct {
	NodeID     string `json:"node_id"`
	RemoteAddr string `json:"remote_addr"`
	Host       string `json:"host"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	SortBy     string `json:"sort_by"`
	SortOrder  string `json:"sort_order"`
}

type AccessLogIPSummaryView struct {
	RemoteAddr     string    `json:"remote_addr"`
	Region         string    `json:"region"`
	Operator       string    `json:"operator"`
	TotalRequests  int64     `json:"total_requests"`
	RecentRequests int64     `json:"recent_requests"`
	LastSeenAt     time.Time `json:"last_seen_at"`
}

type AccessLogIPSummaryList struct {
	Items     []AccessLogIPSummaryView `json:"items"`
	Page      int                      `json:"page"`
	PageSize  int                      `json:"page_size"`
	HasMore   bool                     `json:"has_more"`
	TotalIP   int64                    `json:"total_ip"`
	SortBy    string                   `json:"sort_by"`
	SortOrder string                   `json:"sort_order"`
}

type AccessLogIPTrendQuery struct {
	NodeID        string `json:"node_id"`
	RemoteAddr    string `json:"remote_addr"`
	Host          string `json:"host"`
	Hours         int    `json:"hours"`
	BucketMinutes int    `json:"bucket_minutes"`
}

type AccessLogIPTrendPoint struct {
	BucketStartedAt time.Time `json:"bucket_started_at"`
	RequestCount    int64     `json:"request_count"`
}

type AccessLogIPTrendView struct {
	RemoteAddr    string                  `json:"remote_addr"`
	Hours         int                     `json:"hours"`
	BucketMinutes int                     `json:"bucket_minutes"`
	Points        []AccessLogIPTrendPoint `json:"points"`
}

type AccessLogCleanupInput struct {
	RetentionDays int `json:"retention_days"`
}

type AccessLogCleanupResult struct {
	RetentionDays int       `json:"retention_days"`
	DeletedCount  int64     `json:"deleted_count"`
	Cutoff        time.Time `json:"cutoff"`
}

type ObservabilityMeteringOverview struct {
	GeneratedAt             time.Time                `json:"generated_at"`
	WindowStartedAt         time.Time                `json:"window_started_at"`
	WindowEndedAt           time.Time                `json:"window_ended_at"`
	RequestCount            int64                    `json:"request_count"`
	ResponseBytes           int64                    `json:"response_bytes"`
	RequestBytes            int64                    `json:"request_bytes"`
	UpstreamBytes           int64                    `json:"upstream_bytes"`
	UpstreamBytesSupported  bool                     `json:"upstream_bytes_supported"`
	CacheHitCount           int64                    `json:"cache_hit_count"`
	CacheMissCount          int64                    `json:"cache_miss_count"`
	CacheBypassCount        int64                    `json:"cache_bypass_count"`
	CacheExpiredCount       int64                    `json:"cache_expired_count"`
	CacheStaleCount         int64                    `json:"cache_stale_count"`
	CacheClassifiedCount    int64                    `json:"cache_classified_count"`
	CacheHitRatePercent     float64                  `json:"cache_hit_rate_percent"`
	BandwidthP95Bps         float64                  `json:"bandwidth_p95_bps"`
	NodeAvailabilityPercent float64                  `json:"node_availability_percent"`
	OnlineNodes             int                      `json:"online_nodes"`
	TotalNodes              int                      `json:"total_nodes"`
	SiteTraffic             []MeteringTrafficItem    `json:"site_traffic"`
	NodeTraffic             []MeteringTrafficItem    `json:"node_traffic"`
	StatusCodes             []DistributionItem       `json:"status_codes"`
	TopURLs                 []DistributionItem       `json:"top_urls"`
	TopIPs                  []DistributionItem       `json:"top_ips"`
	TopRegions              []DistributionItem       `json:"top_regions"`
	BandwidthTrend          []MeteringBandwidthPoint `json:"bandwidth_trend"`
}

type MeteringTrafficItem struct {
	Key           string `json:"key"`
	RequestCount  int64  `json:"request_count"`
	RequestBytes  int64  `json:"request_bytes"`
	ResponseBytes int64  `json:"response_bytes"`
	UpstreamBytes int64  `json:"upstream_bytes"`
}

type MeteringBandwidthPoint struct {
	BucketStartedAt time.Time `json:"bucket_started_at"`
	Bytes           int64     `json:"bytes"`
	Bps             float64   `json:"bps"`
}

type meteringOverviewAggregatedDataSource struct {
	now         time.Time
	summary     *model.NodeAccessLogMeteringSummary
	statusCodes []*model.NodeAccessLogDistributionRow
	topURLs     []*model.NodeAccessLogDistributionRow
	topIPs      []*model.NodeAccessLogDistributionRow
	topRegions  []*model.NodeAccessLogDistributionRow
	siteTraffic []*model.NodeAccessLogMeteringTrafficRow
	nodeTraffic []*model.NodeAccessLogMeteringTrafficRow
	cache       *model.NodeRequestReportCacheSummary
	bandwidth   []*model.NodeMetricSnapshotCounterDeltaBucket
	nodes       []*model.Node
}

func runConcurrentQueries(functions ...func() error) error {
	var firstErr error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	for _, fn := range functions {
		if fn == nil {
			continue
		}
		wg.Add(1)
		go func(run func() error) {
			defer wg.Done()
			if err := run(); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(fn)
	}
	wg.Wait()
	return firstErr
}

func accessLogCountCacheKey(query model.NodeAccessLogQuery) string {
	return strings.Join([]string{
		fmt.Sprintf("%p", model.DB),
		common.SQLDSN,
		common.SQLitePath,
		strconv.Itoa(nodeAccessLogRetentionDays),
		strings.TrimSpace(query.NodeID),
		normalizeAccessLogRemoteAddrFilter(query.RemoteAddr),
		normalizeAccessLogHostFilter(query.Host),
		strings.TrimSpace(query.Path),
	}, "\x00")
}

func accessLogBucketCountCacheKey(query model.NodeAccessLogBucketQuery) string {
	return strings.Join([]string{
		"bucket",
		accessLogCountCacheKey(model.NodeAccessLogQuery{
			NodeID:     query.NodeID,
			RemoteAddr: query.RemoteAddr,
			Host:       query.Host,
			Path:       query.Path,
		}),
		strconv.Itoa(query.FoldMinutes),
	}, "\x00")
}

func accessLogIPSummaryCountCacheKey(query model.NodeAccessLogIPSummaryQuery) string {
	return strings.Join([]string{
		"ip_summary",
		accessLogCountCacheKey(model.NodeAccessLogQuery{
			NodeID:     query.NodeID,
			RemoteAddr: query.RemoteAddr,
			Host:       query.Host,
		}),
	}, "\x00")
}

func getAccessLogCachedCount(key string, now time.Time) (int64, int64, bool) {
	accessLogCountCache.Lock()
	defer accessLogCountCache.Unlock()
	entry, ok := accessLogCountCache.values[key]
	if !ok {
		return 0, 0, false
	}
	if !entry.expiresAt.After(now) {
		delete(accessLogCountCache.values, key)
		return 0, 0, false
	}
	return entry.totalRecords, entry.totalIPs, true
}

func getAccessLogCachedSingleCount(cache *accessLogSingleCountCacheStore, key string, now time.Time) (int64, bool) {
	cache.Lock()
	defer cache.Unlock()
	entry, ok := cache.values[key]
	if !ok {
		return 0, false
	}
	if !entry.expiresAt.After(now) {
		delete(cache.values, key)
		return 0, false
	}
	return entry.total, true
}

func loadAccessLogCountWithCache(key string, load func() (int64, int64, error)) (int64, int64, error) {
	if recordCount, ipCount, ok := getAccessLogCachedCount(key, time.Now()); ok {
		return recordCount, ipCount, nil
	}
	value, err, _ := accessLogCountLoadGroup.Do(key, func() (any, error) {
		if recordCount, ipCount, ok := getAccessLogCachedCount(key, time.Now()); ok {
			return accessLogCountLoadResult{totalRecords: recordCount, totalIPs: ipCount}, nil
		}
		recordCount, ipCount, err := load()
		if err != nil {
			return nil, err
		}
		setAccessLogCachedCount(key, recordCount, ipCount, time.Now())
		return accessLogCountLoadResult{totalRecords: recordCount, totalIPs: ipCount}, nil
	})
	if err != nil {
		return 0, 0, err
	}
	result, ok := value.(accessLogCountLoadResult)
	if !ok {
		return 0, 0, errors.New("invalid access log count result")
	}
	return result.totalRecords, result.totalIPs, nil
}

func loadAccessLogSingleCountWithCache(cache *accessLogSingleCountCacheStore, key string, load func() (int64, error)) (int64, error) {
	if cache == nil {
		return load()
	}
	if count, ok := getAccessLogCachedSingleCount(cache, key, time.Now()); ok {
		return count, nil
	}
	value, err, _ := cache.group.Do(key, func() (any, error) {
		if count, ok := getAccessLogCachedSingleCount(cache, key, time.Now()); ok {
			return count, nil
		}
		count, err := load()
		if err != nil {
			return nil, err
		}
		setAccessLogCachedSingleCount(cache, key, count, time.Now())
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	count, ok := value.(int64)
	if !ok {
		return 0, errors.New("invalid access log count result")
	}
	return count, nil
}

func setAccessLogCachedCount(key string, totalRecords int64, totalIPs int64, now time.Time) {
	accessLogCountCache.Lock()
	defer accessLogCountCache.Unlock()
	pruneAccessLogCountCacheLocked(now)
	accessLogCountCache.values[key] = accessLogCountCacheEntry{
		totalRecords: totalRecords,
		totalIPs:     totalIPs,
		expiresAt:    now.Add(accessLogCountCacheTTL),
	}
}

func setAccessLogCachedSingleCount(cache *accessLogSingleCountCacheStore, key string, total int64, now time.Time) {
	cache.Lock()
	defer cache.Unlock()
	pruneAccessLogSingleCountCacheLocked(cache, now)
	cache.values[key] = accessLogSingleCountCacheEntry{
		total:     total,
		expiresAt: now.Add(accessLogCountCacheTTL),
	}
}

func pruneAccessLogCountCacheLocked(now time.Time) {
	if len(accessLogCountCache.values) < accessLogCountCacheMaxEntries {
		return
	}
	for cachedKey, entry := range accessLogCountCache.values {
		if !entry.expiresAt.After(now) {
			delete(accessLogCountCache.values, cachedKey)
		}
	}
	for len(accessLogCountCache.values) >= accessLogCountCacheMaxEntries {
		for cachedKey := range accessLogCountCache.values {
			delete(accessLogCountCache.values, cachedKey)
			break
		}
	}
}

func pruneAccessLogSingleCountCacheLocked(cache *accessLogSingleCountCacheStore, now time.Time) {
	if len(cache.values) < accessLogCountCacheMaxEntries {
		return
	}
	for cachedKey, entry := range cache.values {
		if !entry.expiresAt.After(now) {
			delete(cache.values, cachedKey)
		}
	}
	for len(cache.values) >= accessLogCountCacheMaxEntries {
		for cachedKey := range cache.values {
			delete(cache.values, cachedKey)
			break
		}
	}
}

func resetAccessLogCountCache() {
	accessLogCountCache.Lock()
	accessLogCountCache.values = make(map[string]accessLogCountCacheEntry)
	accessLogCountCache.Unlock()

	accessLogBucketCountCache.Lock()
	accessLogBucketCountCache.values = make(map[string]accessLogSingleCountCacheEntry)
	accessLogBucketCountCache.Unlock()

	accessLogIPSummaryCountCache.Lock()
	accessLogIPSummaryCountCache.values = make(map[string]accessLogSingleCountCacheEntry)
	accessLogIPSummaryCountCache.Unlock()
}

func resetAccessLogCountCacheForTest() {
	resetAccessLogCountCache()
}

func ListAccessLogs(input AccessLogQuery) (*AccessLogList, error) {
	normalized := normalizeAccessLogQuery(input)
	modelQuery := buildModelAccessLogQuery(normalized)
	cursorEnabled := accessLogQuerySupportsCursor(normalized)
	if cursorEnabled && normalized.Cursor != "" {
		loggedAt, id, err := decodeAccessLogCursor(normalized.Cursor)
		if err != nil {
			return nil, err
		}
		modelQuery.CursorLoggedAt = loggedAt
		modelQuery.CursorID = id
		modelQuery.Page = 0
		normalized.Page = 0
	}
	modelQuery.Lookahead = 1
	countKey := accessLogCountCacheKey(modelQuery)

	var logs []*model.NodeAccessLog
	var totalRecords int64
	var totalIPs int64
	countCached := false
	if recordCount, ipCount, ok := getAccessLogCachedCount(countKey, time.Now()); ok {
		totalRecords = recordCount
		totalIPs = ipCount
		countCached = true
	}
	countQuery := modelQuery
	countQuery.Lookahead = 0
	countQuery.CursorLoggedAt = time.Time{}
	countQuery.CursorID = 0
	queries := []func() error{
		func() error {
			rows, err := model.ListNodeAccessLogs(modelQuery)
			logs = rows
			return err
		},
	}
	if !countCached {
		queries = append(queries, func() error {
			recordCount, ipCount, err := loadAccessLogCountWithCache(countKey, func() (int64, int64, error) {
				return model.CountNodeAccessLogs(countQuery)
			})
			if err != nil {
				return err
			}
			totalRecords = recordCount
			totalIPs = ipCount
			return nil
		})
	}
	if err := runConcurrentQueries(queries...); err != nil {
		return nil, err
	}
	hasMore := len(logs) > normalized.PageSize
	if hasMore {
		logs = logs[:normalized.PageSize]
	}
	nextCursor := ""
	if cursorEnabled && hasMore && len(logs) > 0 {
		nextCursor = encodeAccessLogCursor(logs[len(logs)-1])
	}
	nodeNames, err := listNodeNameMap(logs)
	if err != nil {
		return nil, err
	}
	views := make([]AccessLogView, 0, len(logs))
	for _, item := range logs {
		if item == nil {
			continue
		}
		views = append(views, AccessLogView{
			ID:          item.ID,
			NodeID:      item.NodeID,
			NodeName:    nodeNames[item.NodeID],
			LoggedAt:    item.LoggedAt,
			RemoteAddr:  item.RemoteAddr,
			Region:      item.Region,
			Host:        item.Host,
			Path:        item.Path,
			StatusCode:  item.StatusCode,
			Reason:      item.Reason,
			CacheStatus: item.CacheStatus,
		})
	}
	return &AccessLogList{
		Items:       views,
		Page:        normalized.Page,
		PageSize:    normalized.PageSize,
		HasMore:     hasMore,
		NextCursor:  nextCursor,
		TotalRecord: totalRecords,
		TotalIP:     totalIPs,
	}, nil
}

func ListFoldedAccessLogs(input AccessLogQuery) (*FoldedAccessLogList, error) {
	normalized := normalizeAccessLogQuery(input)
	foldMinutes, err := normalizeFoldMinutes(normalized.FoldMinutes)
	if err != nil {
		return nil, err
	}
	modelQuery := buildModelAccessLogQuery(normalized)
	bucketQuery := model.NodeAccessLogBucketQuery{
		NodeID:      modelQuery.NodeID,
		RemoteAddr:  modelQuery.RemoteAddr,
		Host:        modelQuery.Host,
		Path:        modelQuery.Path,
		Since:       modelQuery.Since,
		Page:        normalized.Page,
		PageSize:    normalized.PageSize,
		Lookahead:   1,
		SortBy:      normalizeFoldSortBy(normalized.SortBy),
		SortOrder:   normalized.SortOrder,
		FoldMinutes: foldMinutes,
	}

	var items []*model.NodeAccessLogBucketRow
	var totalBuckets int64
	var totalRecords int64
	var totalIPs int64
	countKey := accessLogCountCacheKey(modelQuery)
	countCached := false
	if recordCount, ipCount, ok := getAccessLogCachedCount(countKey, time.Now()); ok {
		totalRecords = recordCount
		totalIPs = ipCount
		countCached = true
	}
	bucketCountKey := accessLogBucketCountCacheKey(bucketQuery)
	bucketCountCached := false
	if count, ok := getAccessLogCachedSingleCount(&accessLogBucketCountCache, bucketCountKey, time.Now()); ok {
		totalBuckets = count
		bucketCountCached = true
	}
	queries := []func() error{
		func() error {
			rows, err := model.ListNodeAccessLogBuckets(bucketQuery)
			items = rows
			return err
		},
	}
	if !bucketCountCached {
		queries = append(queries, func() error {
			count, err := loadAccessLogSingleCountWithCache(&accessLogBucketCountCache, bucketCountKey, func() (int64, error) {
				return model.CountNodeAccessLogBuckets(bucketQuery)
			})
			if err != nil {
				return err
			}
			totalBuckets = count
			return nil
		})
	}
	if !countCached {
		queries = append(queries, func() error {
			recordCount, ipCount, err := loadAccessLogCountWithCache(countKey, func() (int64, int64, error) {
				return model.CountNodeAccessLogs(modelQuery)
			})
			if err != nil {
				return err
			}
			totalRecords = recordCount
			totalIPs = ipCount
			return nil
		})
	}
	if err := runConcurrentQueries(queries...); err != nil {
		return nil, err
	}
	hasMore := len(items) > normalized.PageSize
	if hasMore {
		items = items[:normalized.PageSize]
	}
	views := make([]FoldedAccessLogView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, FoldedAccessLogView{
			BucketStartedAt:  time.Unix(item.BucketEpoch, 0).UTC(),
			RequestCount:     item.RequestCount,
			UniqueIPCount:    item.UniqueIPCount,
			UniqueHostCount:  item.UniqueHostCount,
			SuccessCount:     item.SuccessCount,
			ClientErrorCount: item.ClientErrorCount,
			ServerErrorCount: item.ServerErrorCount,
		})
	}
	return &FoldedAccessLogList{
		Items:       views,
		Page:        normalized.Page,
		PageSize:    normalized.PageSize,
		HasMore:     hasMore,
		TotalBucket: totalBuckets,
		TotalRecord: totalRecords,
		TotalIP:     totalIPs,
		FoldMinutes: foldMinutes,
	}, nil
}

func ListAccessLogIPSummaries(input AccessLogIPSummaryQuery) (*AccessLogIPSummaryList, error) {
	normalized := normalizeAccessLogIPSummaryQuery(input)
	since := time.Now().UTC().Add(-nodeAccessLogRetentionWindow)
	recentSince := time.Now().UTC().Add(-3 * time.Hour)
	query := model.NodeAccessLogIPSummaryQuery{
		NodeID:     strings.TrimSpace(normalized.NodeID),
		RemoteAddr: strings.TrimSpace(normalized.RemoteAddr),
		Host:       strings.TrimSpace(normalized.Host),
		Since:      since,
		Page:       normalized.Page,
		PageSize:   normalized.PageSize,
		Lookahead:  1,
		SortBy:     normalized.SortBy,
		SortOrder:  normalized.SortOrder,
	}

	var items []*model.NodeAccessLogIPSummaryRow
	var totalIP int64
	countKey := accessLogIPSummaryCountCacheKey(query)
	countCached := false
	if count, ok := getAccessLogCachedSingleCount(&accessLogIPSummaryCountCache, countKey, time.Now()); ok {
		totalIP = count
		countCached = true
	}
	queries := []func() error{
		func() error {
			rows, err := model.ListNodeAccessLogIPSummaries(query, recentSince)
			items = rows
			return err
		},
	}
	if !countCached {
		queries = append(queries, func() error {
			count, err := loadAccessLogSingleCountWithCache(&accessLogIPSummaryCountCache, countKey, func() (int64, error) {
				return model.CountNodeAccessLogIPSummaries(query)
			})
			if err != nil {
				return err
			}
			totalIP = count
			return nil
		})
	}
	if err := runConcurrentQueries(queries...); err != nil {
		return nil, err
	}
	hasMore := len(items) > normalized.PageSize
	if hasMore {
		items = items[:normalized.PageSize]
	}
	views := make([]AccessLogIPSummaryView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, AccessLogIPSummaryView{
			RemoteAddr:     item.RemoteAddr,
			Region:         item.Region,
			Operator:       item.Operator,
			TotalRequests:  item.TotalRequests,
			RecentRequests: item.RecentRequests,
			LastSeenAt:     time.Unix(item.LastSeenEpoch, 0).UTC(),
		})
	}
	return &AccessLogIPSummaryList{
		Items:     views,
		Page:      normalized.Page,
		PageSize:  normalized.PageSize,
		HasMore:   hasMore,
		TotalIP:   totalIP,
		SortBy:    normalized.SortBy,
		SortOrder: normalized.SortOrder,
	}, nil
}

func GetAccessLogIPTrend(input AccessLogIPTrendQuery) (*AccessLogIPTrendView, error) {
	normalized, err := normalizeAccessLogIPTrendQuery(input)
	if err != nil {
		return nil, err
	}
	points, err := model.ListNodeAccessLogIPTrend(model.NodeAccessLogIPTrendQuery{
		NodeID:        strings.TrimSpace(normalized.NodeID),
		RemoteAddr:    strings.TrimSpace(normalized.RemoteAddr),
		Host:          strings.TrimSpace(normalized.Host),
		Since:         time.Now().UTC().Add(-time.Duration(normalized.Hours) * time.Hour),
		BucketMinutes: normalized.BucketMinutes,
	})
	if err != nil {
		return nil, err
	}
	pointMap := make(map[int64]int64, len(points))
	for _, item := range points {
		if item == nil {
			continue
		}
		pointMap[item.BucketEpoch] = item.RequestCount
	}
	bucketDuration := time.Duration(normalized.BucketMinutes) * time.Minute
	start := time.Now().UTC().Add(-time.Duration(normalized.Hours) * time.Hour).Truncate(bucketDuration)
	end := time.Now().UTC().Truncate(bucketDuration)
	views := make([]AccessLogIPTrendPoint, 0, int(end.Sub(start)/bucketDuration)+1)
	for cursor := start; !cursor.After(end); cursor = cursor.Add(bucketDuration) {
		views = append(views, AccessLogIPTrendPoint{
			BucketStartedAt: cursor,
			RequestCount:    pointMap[cursor.Unix()],
		})
	}
	return &AccessLogIPTrendView{
		RemoteAddr:    normalized.RemoteAddr,
		Hours:         normalized.Hours,
		BucketMinutes: normalized.BucketMinutes,
		Points:        views,
	}, nil
}

func GetObservabilityMeteringOverview() (*ObservabilityMeteringOverview, error) {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)
	const limit = 8

	var summary *model.NodeAccessLogMeteringSummary
	var statusCodes []*model.NodeAccessLogDistributionRow
	var topURLs []*model.NodeAccessLogDistributionRow
	var topIPs []*model.NodeAccessLogDistributionRow
	var topRegions []*model.NodeAccessLogDistributionRow
	var siteTraffic []*model.NodeAccessLogMeteringTrafficRow
	var nodeTraffic []*model.NodeAccessLogMeteringTrafficRow
	var cacheSummary *model.NodeRequestReportCacheSummary
	var bandwidthBuckets []*model.NodeMetricSnapshotCounterDeltaBucket
	var nodes []*model.Node
	if err := runConcurrentQueries(
		func() error {
			value, err := model.GetNodeAccessLogMeteringSummary(since)
			summary = value
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogStatusDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: limit})
			statusCodes = rows
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogURLDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: limit})
			topURLs = rows
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogIPDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: limit})
			topIPs = rows
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogRegionDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: limit})
			topRegions = rows
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogMeteringTrafficByHost(since, limit)
			siteTraffic = rows
			return err
		},
		func() error {
			rows, err := model.ListNodeAccessLogMeteringTrafficByNode(since, limit)
			nodeTraffic = rows
			return err
		},
		func() error {
			value, err := model.GetRequestReportCacheSummary(since, now)
			cacheSummary = value
			return err
		},
		func() error {
			rows, err := model.ListMetricSnapshotCounterDeltaBuckets("", since, now, 60)
			bandwidthBuckets = rows
			return err
		},
		func() error {
			rows, err := model.ListNodes()
			nodes = rows
			return err
		},
	); err != nil {
		return nil, err
	}

	return buildAggregatedObservabilityMeteringOverview(meteringOverviewAggregatedDataSource{
		now:         now,
		summary:     summary,
		statusCodes: statusCodes,
		topURLs:     topURLs,
		topIPs:      topIPs,
		topRegions:  topRegions,
		siteTraffic: siteTraffic,
		nodeTraffic: nodeTraffic,
		cache:       cacheSummary,
		bandwidth:   bandwidthBuckets,
		nodes:       nodes,
	}), nil
}

func buildAggregatedObservabilityMeteringOverview(source meteringOverviewAggregatedDataSource) *ObservabilityMeteringOverview {
	now := source.now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	since := now.Add(-24 * time.Hour)
	const limit = 8

	overview := &ObservabilityMeteringOverview{
		GeneratedAt:     now,
		WindowStartedAt: since,
		WindowEndedAt:   now,
		TotalNodes:      len(source.nodes),
		StatusCodes:     distributionRowsToItems(source.statusCodes),
		TopURLs:         distributionRowsToItems(source.topURLs),
		TopIPs:          distributionRowsToItems(source.topIPs),
		TopRegions:      distributionRowsToItems(source.topRegions),
		SiteTraffic:     meteringTrafficRowsToItems(source.siteTraffic, nil),
		NodeTraffic:     meteringTrafficRowsToItems(source.nodeTraffic, buildMeteringNodeNameMap(source.nodes)),
		BandwidthTrend:  buildMeteringBandwidthTrendFromCounterBuckets(now, source.bandwidth),
	}
	if summary := source.summary; summary != nil {
		overview.RequestCount = summary.RequestCount
		overview.RequestBytes = summary.RequestBytes
		overview.ResponseBytes = summary.ResponseBytes
		overview.UpstreamBytes = summary.UpstreamBytes
		overview.UpstreamBytesSupported = summary.UpstreamBytesHitCount > 0
		overview.CacheHitCount = summary.CacheHitCount
		overview.CacheMissCount = summary.CacheMissCount
		overview.CacheBypassCount = summary.CacheBypassCount
		overview.CacheExpiredCount = summary.CacheExpiredCount
		overview.CacheStaleCount = summary.CacheStaleCount
		overview.CacheClassifiedCount = summary.CacheClassifiedCount
	}
	if cache := source.cache; cache != nil && cache.CacheClassifiedCount > 0 {
		overview.CacheHitCount = cache.CacheHitCount
		overview.CacheMissCount = cache.CacheMissCount
		overview.CacheBypassCount = cache.CacheBypassCount
		overview.CacheExpiredCount = cache.CacheExpiredCount
		overview.CacheStaleCount = cache.CacheStaleCount
		overview.CacheClassifiedCount = cache.CacheClassifiedCount
	}
	if overview.CacheClassifiedCount > 0 {
		overview.CacheHitRatePercent = float64(overview.CacheHitCount) / float64(overview.CacheClassifiedCount) * 100
	}
	overview.BandwidthP95Bps = calculateP95BandwidthBps(overview.BandwidthTrend)
	for _, node := range source.nodes {
		if node == nil {
			continue
		}
		if meteringNodeOnline(node, now) {
			overview.OnlineNodes++
		}
	}
	if overview.TotalNodes > 0 {
		overview.NodeAvailabilityPercent = float64(overview.OnlineNodes) / float64(overview.TotalNodes) * 100
	}
	return overview
}

func distributionRowsToItems(rows []*model.NodeAccessLogDistributionRow) []DistributionItem {
	items := make([]DistributionItem, 0, len(rows))
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.Key) == "" || row.Value <= 0 {
			continue
		}
		items = append(items, DistributionItem{Key: row.Key, Value: row.Value})
	}
	return items
}

func meteringTrafficRowsToItems(rows []*model.NodeAccessLogMeteringTrafficRow, nodeNames map[string]string) []MeteringTrafficItem {
	items := make([]MeteringTrafficItem, 0, len(rows))
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.Key) == "" {
			continue
		}
		key := strings.TrimSpace(row.Key)
		if displayName := strings.TrimSpace(nodeNames[key]); displayName != "" {
			key = displayName
		}
		items = append(items, MeteringTrafficItem{
			Key:           key,
			RequestCount:  row.RequestCount,
			RequestBytes:  row.RequestBytes,
			ResponseBytes: row.ResponseBytes,
			UpstreamBytes: row.UpstreamBytes,
		})
	}
	return items
}

func buildMeteringNodeNameMap(nodes []*model.Node) map[string]string {
	result := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			continue
		}
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = nodeID
		}
		result[nodeID] = name
	}
	return result
}

func CleanupAccessLogs(input AccessLogCleanupInput) (*AccessLogCleanupResult, error) {
	if input.RetentionDays <= 0 || input.RetentionDays > nodeAccessLogRetentionDays {
		return nil, errors.New("retention_days 必须在 1 到 90 之间")
	}
	cutoff := time.Now().UTC().Add(-time.Duration(input.RetentionDays) * 24 * time.Hour)
	deleted, err := model.DeleteNodeAccessLogsBefore(cutoff)
	if err != nil {
		return nil, err
	}
	resetAccessLogCountCache()
	return &AccessLogCleanupResult{
		RetentionDays: input.RetentionDays,
		DeletedCount:  deleted,
		Cutoff:        cutoff,
	}, nil
}

func buildModelAccessLogQuery(input AccessLogQuery) model.NodeAccessLogQuery {
	return model.NodeAccessLogQuery{
		NodeID:     strings.TrimSpace(input.NodeID),
		RemoteAddr: normalizeAccessLogRemoteAddrFilter(input.RemoteAddr),
		Host:       normalizeAccessLogHostFilter(input.Host),
		Path:       strings.TrimSpace(input.Path),
		Since:      time.Now().UTC().Add(-nodeAccessLogRetentionWindow),
		Page:       input.Page,
		PageSize:   input.PageSize,
		SortBy:     input.SortBy,
		SortOrder:  input.SortOrder,
	}
}

func normalizeAccessLogCacheStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "HIT", "MISS", "BYPASS", "EXPIRED", "STALE", "UPDATING", "REVALIDATED":
		return status
	default:
		return ""
	}
}

func meteringNodeOnline(node *model.Node, now time.Time) bool {
	if node == nil {
		return false
	}
	if IsAgentWSConnected(node.NodeID) {
		return true
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return now.Sub(node.LastSeenAt) <= common.NodeOfflineThreshold
}

func buildMeteringBandwidthTrendFromCounterBuckets(now time.Time, buckets []*model.NodeMetricSnapshotCounterDeltaBucket) []MeteringBandwidthPoint {
	start := now.Truncate(time.Hour).Add(-(observabilityTrendBuckets - 1) * time.Hour)
	points := make([]MeteringBandwidthPoint, observabilityTrendBuckets)
	for index := range points {
		points[index].BucketStartedAt = start.Add(time.Duration(index) * time.Hour)
	}
	for _, bucket := range buckets {
		if bucket == nil {
			continue
		}
		index, ok := trendBucketIndex(time.Unix(bucket.BucketEpoch, 0).UTC(), start)
		if !ok {
			continue
		}
		points[index].Bytes += bucket.OpenrestyRxBytes + bucket.OpenrestyTxBytes
	}
	for index := range points {
		points[index].Bps = float64(points[index].Bytes) / 3600
	}
	return points
}

func calculateP95BandwidthBps(points []MeteringBandwidthPoint) float64 {
	values := make([]float64, 0, len(points))
	for _, point := range points {
		if point.Bps > 0 {
			values = append(values, point.Bps)
		}
	}
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	index := int(math.Ceil(float64(len(values))*0.95)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func listNodeNameMap(logs []*model.NodeAccessLog) (map[string]string, error) {
	nodeIDs := make([]string, 0, len(logs))
	seen := make(map[string]struct{}, len(logs))
	for _, item := range logs {
		if item == nil || item.NodeID == "" {
			continue
		}
		if _, exists := seen[item.NodeID]; exists {
			continue
		}
		seen[item.NodeID] = struct{}{}
		nodeIDs = append(nodeIDs, item.NodeID)
	}
	nodes, err := model.ListNodesByNodeIDs(nodeIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		result[node.NodeID] = node.Name
	}
	return result, nil
}

func normalizeAccessLogQuery(input AccessLogQuery) AccessLogQuery {
	return AccessLogQuery{
		NodeID:      strings.TrimSpace(input.NodeID),
		RemoteAddr:  normalizeAccessLogRemoteAddrFilter(input.RemoteAddr),
		Host:        normalizeAccessLogHostFilter(input.Host),
		Path:        strings.TrimSpace(input.Path),
		Cursor:      strings.TrimSpace(input.Cursor),
		Page:        normalizeAccessLogPage(input.Page),
		PageSize:    normalizeAccessLogPageSize(input.PageSize),
		SortBy:      normalizeAccessLogSortBy(input.SortBy),
		SortOrder:   normalizeAccessLogSortOrder(input.SortOrder),
		FoldMinutes: input.FoldMinutes,
	}
}

func normalizeAccessLogIPSummaryQuery(input AccessLogIPSummaryQuery) AccessLogIPSummaryQuery {
	return AccessLogIPSummaryQuery{
		NodeID:     strings.TrimSpace(input.NodeID),
		RemoteAddr: normalizeAccessLogRemoteAddrFilter(input.RemoteAddr),
		Host:       normalizeAccessLogHostFilter(input.Host),
		Page:       normalizeAccessLogPage(input.Page),
		PageSize:   normalizeAccessLogPageSize(input.PageSize),
		SortBy:     normalizeIPSummarySortBy(input.SortBy),
		SortOrder:  normalizeAccessLogSortOrder(input.SortOrder),
	}
}

func normalizeAccessLogIPTrendQuery(input AccessLogIPTrendQuery) (AccessLogIPTrendQuery, error) {
	remoteAddr := normalizeAccessLogRemoteAddrFilter(input.RemoteAddr)
	if remoteAddr == "" {
		return AccessLogIPTrendQuery{}, errors.New("remote_addr 不能为空")
	}
	hours := input.Hours
	if hours <= 0 {
		hours = defaultIPTrendHours
	}
	if hours > maxIPTrendHours {
		hours = maxIPTrendHours
	}
	bucketMinutes := input.BucketMinutes
	if bucketMinutes <= 0 {
		bucketMinutes = defaultIPTrendBucketMinute
	}
	switch bucketMinutes {
	case 5, 10, 15, 30, 60:
	default:
		return AccessLogIPTrendQuery{}, errors.New("bucket_minutes 仅支持 5、10、15、30、60")
	}
	return AccessLogIPTrendQuery{
		NodeID:        strings.TrimSpace(input.NodeID),
		RemoteAddr:    remoteAddr,
		Host:          normalizeAccessLogHostFilter(input.Host),
		Hours:         hours,
		BucketMinutes: bucketMinutes,
	}, nil
}

func normalizeAccessLogRemoteAddrFilter(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if normalizedIP := normalizeAccessLogIP(trimmed); normalizedIP != "" {
		return normalizedIP
	}
	return trimmed
}

func normalizeAccessLogHostFilter(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	} else if strings.Contains(value, "://") {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if slash := strings.IndexAny(value, "/?#"); slash >= 0 {
		value = value[:slash]
	}
	if colon := strings.LastIndex(value, ":"); colon > -1 && !strings.Contains(value[:colon], ":") {
		value = value[:colon]
	}
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimPrefix(value, "*.")
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

func accessLogQuerySupportsCursor(query AccessLogQuery) bool {
	return query.SortBy == defaultAccessLogSortBy
}

func encodeAccessLogCursor(item *model.NodeAccessLog) string {
	if item == nil || item.ID == 0 || item.LoggedAt.IsZero() {
		return ""
	}
	raw, err := json.Marshal(accessLogCursor{
		LoggedAt: item.LoggedAt.UTC(),
		ID:       item.ID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeAccessLogCursor(raw string) (time.Time, uint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return time.Time{}, 0, errors.New("invalid access log cursor")
	}
	var cursor accessLogCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return time.Time{}, 0, errors.New("invalid access log cursor")
	}
	if cursor.ID == 0 || cursor.LoggedAt.IsZero() {
		return time.Time{}, 0, errors.New("invalid access log cursor")
	}
	return cursor.LoggedAt.UTC(), cursor.ID, nil
}

func normalizeAccessLogPage(page int) int {
	if page < 0 {
		return 0
	}
	return page
}

func normalizeAccessLogPageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultAccessLogPageSize
	}
	if pageSize > maxAccessLogPageSize {
		return maxAccessLogPageSize
	}
	return pageSize
}

func normalizeAccessLogSortBy(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case "status_code", "remote_addr", "host", "path":
		return strings.TrimSpace(sortBy)
	default:
		return defaultAccessLogSortBy
	}
}

func normalizeAccessLogSortOrder(sortOrder string) string {
	if strings.EqualFold(strings.TrimSpace(sortOrder), "asc") {
		return "asc"
	}
	return defaultAccessLogSortOrder
}

func normalizeFoldSortBy(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case "request_count":
		return "request_count"
	default:
		return "bucket_started_at"
	}
}

func normalizeIPSummarySortBy(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case "recent_requests", "last_seen_at", "remote_addr":
		return strings.TrimSpace(sortBy)
	default:
		return "total_requests"
	}
}

func normalizeFoldMinutes(value int) (int, error) {
	if value <= 0 {
		return defaultAccessLogFoldMinute, nil
	}
	switch value {
	case 3, 5:
		return value, nil
	default:
		return 0, errors.New("fold_minutes 仅支持 3 或 5")
	}
}
