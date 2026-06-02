package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
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

type AccessLogQuery struct {
	NodeID      string `json:"node_id"`
	RemoteAddr  string `json:"remote_addr"`
	Host        string `json:"host"`
	Path        string `json:"path"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
	SortBy      string `json:"sort_by"`
	SortOrder   string `json:"sort_order"`
	FoldMinutes int    `json:"fold_minutes"`
}

type AccessLogView struct {
	ID         uint      `json:"id"`
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	LoggedAt   time.Time `json:"logged_at"`
	RemoteAddr string    `json:"remote_addr"`
	Region     string    `json:"region"`
	Host       string    `json:"host"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	Reason     string    `json:"reason"`
}

type AccessLogList struct {
	Items       []AccessLogView `json:"items"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
	HasMore     bool            `json:"has_more"`
	TotalRecord int64           `json:"total_record"`
	TotalIP     int64           `json:"total_ip"`
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

type meteringOverviewDataSource struct {
	now       time.Time
	logs      []*model.NodeAccessLog
	reports   []*model.NodeRequestReport
	snapshots []*model.NodeMetricSnapshot
	nodes     []*model.Node
}

func ListAccessLogs(input AccessLogQuery) (*AccessLogList, error) {
	normalized := normalizeAccessLogQuery(input)
	modelQuery := buildModelAccessLogQuery(normalized)
	logs, err := model.ListNodeAccessLogs(modelQuery)
	if err != nil {
		return nil, err
	}
	totalRecords, totalIPs, err := model.CountNodeAccessLogs(modelQuery)
	if err != nil {
		return nil, err
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
			ID:         item.ID,
			NodeID:     item.NodeID,
			NodeName:   nodeNames[item.NodeID],
			LoggedAt:   item.LoggedAt,
			RemoteAddr: item.RemoteAddr,
			Region:     item.Region,
			Host:       item.Host,
			Path:       item.Path,
			StatusCode: item.StatusCode,
			Reason:     item.Reason,
		})
	}
	return &AccessLogList{
		Items:       views,
		Page:        normalized.Page,
		PageSize:    normalized.PageSize,
		HasMore:     int64((normalized.Page+1)*normalized.PageSize) < totalRecords,
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
		SortBy:      normalizeFoldSortBy(normalized.SortBy),
		SortOrder:   normalized.SortOrder,
		FoldMinutes: foldMinutes,
	}
	items, err := model.ListNodeAccessLogBuckets(bucketQuery)
	if err != nil {
		return nil, err
	}
	totalBuckets, err := model.CountNodeAccessLogBuckets(bucketQuery)
	if err != nil {
		return nil, err
	}
	totalRecords, totalIPs, err := model.CountNodeAccessLogs(modelQuery)
	if err != nil {
		return nil, err
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
		HasMore:     int64((normalized.Page+1)*normalized.PageSize) < totalBuckets,
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
		SortBy:     normalized.SortBy,
		SortOrder:  normalized.SortOrder,
	}
	items, err := model.ListNodeAccessLogIPSummaries(query, recentSince)
	if err != nil {
		return nil, err
	}
	totalIP, err := model.CountNodeAccessLogIPSummaries(query)
	if err != nil {
		return nil, err
	}
	views := make([]AccessLogIPSummaryView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, AccessLogIPSummaryView{
			RemoteAddr:     item.RemoteAddr,
			TotalRequests:  item.TotalRequests,
			RecentRequests: item.RecentRequests,
			LastSeenAt:     time.Unix(item.LastSeenEpoch, 0).UTC(),
		})
	}
	return &AccessLogIPSummaryList{
		Items:     views,
		Page:      normalized.Page,
		PageSize:  normalized.PageSize,
		HasMore:   int64((normalized.Page+1)*normalized.PageSize) < totalIP,
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

	logs, err := model.ListNodeAccessLogs(model.NodeAccessLogQuery{
		Since:     since,
		SortBy:    "logged_at",
		SortOrder: "desc",
	})
	if err != nil {
		return nil, err
	}
	reports, err := model.ListRequestReportsSince(since)
	if err != nil {
		return nil, err
	}
	snapshots, err := model.ListMetricSnapshotsSince(since)
	if err != nil {
		return nil, err
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}

	return buildObservabilityMeteringOverview(meteringOverviewDataSource{
		now:       now,
		logs:      logs,
		reports:   reports,
		snapshots: snapshots,
		nodes:     nodes,
	}), nil
}

func buildObservabilityMeteringOverview(source meteringOverviewDataSource) *ObservabilityMeteringOverview {
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
		StatusCodes:     []DistributionItem{},
		TopURLs:         []DistributionItem{},
		TopIPs:          []DistributionItem{},
		TopRegions:      []DistributionItem{},
		SiteTraffic:     []MeteringTrafficItem{},
		NodeTraffic:     []MeteringTrafficItem{},
		BandwidthTrend:  buildMeteringBandwidthTrend(now, source.snapshots),
	}

	siteAccumulators := make(map[string]*meteringTrafficAccumulator)
	nodeAccumulators := make(map[string]*meteringTrafficAccumulator)
	topURLCounts := make(distributionAccumulator)
	topIPCounts := make(distributionAccumulator)
	topRegionCounts := make(distributionAccumulator)
	statusCounts := make(distributionAccumulator)

	for _, item := range source.logs {
		if item == nil {
			continue
		}
		overview.RequestCount++
		overview.RequestBytes += nonNegativeInt64(item.RequestBytes)
		overview.ResponseBytes += nonNegativeInt64(item.ResponseBytes)
		overview.UpstreamBytes += nonNegativeInt64(item.UpstreamBytes)
		if item.UpstreamBytes > 0 {
			overview.UpstreamBytesSupported = true
		}
		siteKey := strings.TrimSpace(item.Host)
		if siteKey == "" {
			siteKey = "未识别站点"
		}
		accumulateMeteringTraffic(siteAccumulators, siteKey, item)
		nodeKey := strings.TrimSpace(item.NodeID)
		if nodeKey == "" {
			nodeKey = "未识别节点"
		}
		accumulateMeteringTraffic(nodeAccumulators, nodeKey, item)
		if key := buildAccessLogURLKey(item); key != "" {
			topURLCounts[key]++
		}
		if remoteAddr := strings.TrimSpace(item.RemoteAddr); remoteAddr != "" {
			topIPCounts[remoteAddr]++
		}
		if region := strings.TrimSpace(item.Region); region != "" {
			topRegionCounts[region]++
		}
		if item.StatusCode > 0 {
			statusCounts[formatStatusCode(item.StatusCode)]++
		}
	}

	for _, report := range source.reports {
		if report == nil {
			continue
		}
		overview.CacheHitCount += report.CacheHitCount
		overview.CacheClassifiedCount += report.CacheHitCount + report.CacheMissCount + report.CacheBypassCount + report.CacheExpiredCount + report.CacheStaleCount
		if len(statusCounts) == 0 {
			mergeJSONCounts(statusCounts, report.StatusCodesJSON)
		}
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

	overview.SiteTraffic = meteringTrafficItems(siteAccumulators, limit)
	overview.NodeTraffic = meteringTrafficItems(nodeAccumulators, limit)
	overview.StatusCodes = toDistributionItems(statusCounts, limit)
	overview.TopURLs = toDistributionItems(topURLCounts, limit)
	overview.TopIPs = toDistributionItems(topIPCounts, limit)
	overview.TopRegions = toDistributionItems(topRegionCounts, limit)
	return overview
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
	return &AccessLogCleanupResult{
		RetentionDays: input.RetentionDays,
		DeletedCount:  deleted,
		Cutoff:        cutoff,
	}, nil
}

func buildModelAccessLogQuery(input AccessLogQuery) model.NodeAccessLogQuery {
	return model.NodeAccessLogQuery{
		NodeID:     strings.TrimSpace(input.NodeID),
		RemoteAddr: strings.TrimSpace(input.RemoteAddr),
		Host:       strings.TrimSpace(input.Host),
		Path:       strings.TrimSpace(input.Path),
		Since:      time.Now().UTC().Add(-nodeAccessLogRetentionWindow),
		Page:       input.Page,
		PageSize:   input.PageSize,
		SortBy:     input.SortBy,
		SortOrder:  input.SortOrder,
	}
}

type meteringTrafficAccumulator struct {
	requestCount  int64
	requestBytes  int64
	responseBytes int64
	upstreamBytes int64
}

func accumulateMeteringTraffic(target map[string]*meteringTrafficAccumulator, key string, item *model.NodeAccessLog) {
	if item == nil {
		return
	}
	accumulator := target[key]
	if accumulator == nil {
		accumulator = &meteringTrafficAccumulator{}
		target[key] = accumulator
	}
	accumulator.requestCount++
	accumulator.requestBytes += nonNegativeInt64(item.RequestBytes)
	accumulator.responseBytes += nonNegativeInt64(item.ResponseBytes)
	accumulator.upstreamBytes += nonNegativeInt64(item.UpstreamBytes)
}

func meteringTrafficItems(values map[string]*meteringTrafficAccumulator, limit int) []MeteringTrafficItem {
	items := make([]MeteringTrafficItem, 0, len(values))
	for key, accumulator := range values {
		if accumulator == nil || strings.TrimSpace(key) == "" {
			continue
		}
		items = append(items, MeteringTrafficItem{
			Key:           key,
			RequestCount:  accumulator.requestCount,
			RequestBytes:  accumulator.requestBytes,
			ResponseBytes: accumulator.responseBytes,
			UpstreamBytes: accumulator.upstreamBytes,
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].ResponseBytes == items[j].ResponseBytes {
			if items[i].RequestCount == items[j].RequestCount {
				return items[i].Key < items[j].Key
			}
			return items[i].RequestCount > items[j].RequestCount
		}
		return items[i].ResponseBytes > items[j].ResponseBytes
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func buildAccessLogURLKey(item *model.NodeAccessLog) string {
	if item == nil {
		return ""
	}
	host := strings.TrimSpace(item.Host)
	path := strings.TrimSpace(item.Path)
	if host == "" {
		return path
	}
	if path == "" {
		return host
	}
	return host + path
}

func formatStatusCode(statusCode int) string {
	return strconv.Itoa(statusCode)
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

func buildMeteringBandwidthTrend(now time.Time, snapshots []*model.NodeMetricSnapshot) []MeteringBandwidthPoint {
	start := now.Truncate(time.Hour).Add(-(observabilityTrendBuckets - 1) * time.Hour)
	points := make([]MeteringBandwidthPoint, observabilityTrendBuckets)
	for index := range points {
		points[index].BucketStartedAt = start.Add(time.Duration(index) * time.Hour)
	}
	if len(snapshots) == 0 {
		return points
	}

	sort.Slice(snapshots, func(i int, j int) bool {
		if snapshots[i] == nil || snapshots[j] == nil {
			return snapshots[i] != nil
		}
		if snapshots[i].CapturedAt.Equal(snapshots[j].CapturedAt) {
			return snapshots[i].NodeID < snapshots[j].NodeID
		}
		return snapshots[i].CapturedAt.Before(snapshots[j].CapturedAt)
	})

	type bandwidthCounterState struct {
		rx   int64
		tx   int64
		seen bool
	}
	previousByNode := make(map[string]bandwidthCounterState)
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		nodeKey := strings.TrimSpace(snapshot.NodeID)
		if nodeKey == "" {
			nodeKey = "__unknown__"
		}
		previous := previousByNode[nodeKey]
		previousByNode[nodeKey] = bandwidthCounterState{
			rx:   snapshot.OpenrestyRxBytes,
			tx:   snapshot.OpenrestyTxBytes,
			seen: true,
		}
		if !previous.seen {
			continue
		}
		index, ok := trendBucketIndex(snapshot.CapturedAt, start)
		if !ok {
			continue
		}
		rxDelta := snapshot.OpenrestyRxBytes - previous.rx
		txDelta := snapshot.OpenrestyTxBytes - previous.tx
		if rxDelta < 0 {
			rxDelta = 0
		}
		if txDelta < 0 {
			txDelta = 0
		}
		points[index].Bytes += rxDelta + txDelta
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
		RemoteAddr:  strings.TrimSpace(input.RemoteAddr),
		Host:        strings.TrimSpace(input.Host),
		Path:        strings.TrimSpace(input.Path),
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
		RemoteAddr: strings.TrimSpace(input.RemoteAddr),
		Host:       strings.TrimSpace(input.Host),
		Page:       normalizeAccessLogPage(input.Page),
		PageSize:   normalizeAccessLogPageSize(input.PageSize),
		SortBy:     normalizeIPSummarySortBy(input.SortBy),
		SortOrder:  normalizeAccessLogSortOrder(input.SortOrder),
	}
}

func normalizeAccessLogIPTrendQuery(input AccessLogIPTrendQuery) (AccessLogIPTrendQuery, error) {
	remoteAddr := strings.TrimSpace(input.RemoteAddr)
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
		Host:          strings.TrimSpace(input.Host),
		Hours:         hours,
		BucketMinutes: bucketMinutes,
	}, nil
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
