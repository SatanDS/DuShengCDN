package model

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type NodeAccessLog struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	NodeID        string    `json:"node_id" gorm:"index:,composite:node_logged_at,priority:1;size:64;not null"`
	LoggedAt      time.Time `json:"logged_at" gorm:"index;index:,composite:node_logged_at,priority:2"`
	RemoteAddr    string    `json:"remote_addr" gorm:"index;size:128"`
	Region        string    `json:"region" gorm:"size:128"`
	Operator      string    `json:"operator" gorm:"size:255"`
	Host          string    `json:"host" gorm:"index;size:255"`
	Path          string    `json:"path" gorm:"size:2048"`
	StatusCode    int       `json:"status_code" gorm:"index"`
	Reason        string    `json:"reason" gorm:"type:text"`
	CacheStatus   string    `json:"cache_status" gorm:"size:32"`
	RequestBytes  int64     `json:"request_bytes"`
	ResponseBytes int64     `json:"response_bytes"`
	UpstreamBytes int64     `json:"upstream_bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

type NodeAccessLogRegionCount struct {
	Region string `json:"region"`
	Count  int64  `json:"count"`
}

type NodeAccessLogQuery struct {
	NodeID         string
	RemoteAddr     string
	Host           string
	Path           string
	Since          time.Time
	Page           int
	PageSize       int
	Lookahead      int
	SortBy         string
	SortOrder      string
	CursorLoggedAt time.Time
	CursorID       uint
}

type NodeAccessLogBucketQuery struct {
	NodeID      string
	RemoteAddr  string
	Host        string
	Path        string
	Since       time.Time
	Page        int
	PageSize    int
	Lookahead   int
	SortBy      string
	SortOrder   string
	FoldMinutes int
}

type NodeAccessLogBucketRow struct {
	BucketEpoch      int64 `json:"bucket_epoch"`
	RequestCount     int64 `json:"request_count"`
	UniqueIPCount    int64 `json:"unique_ip_count"`
	UniqueHostCount  int64 `json:"unique_host_count"`
	SuccessCount     int64 `json:"success_count"`
	ClientErrorCount int64 `json:"client_error_count"`
	ServerErrorCount int64 `json:"server_error_count"`
}

type NodeAccessLogIPSummaryQuery struct {
	NodeID     string
	RemoteAddr string
	Host       string
	Since      time.Time
	Page       int
	PageSize   int
	Lookahead  int
	SortBy     string
	SortOrder  string
}

type NodeAccessLogIPSummaryRow struct {
	RemoteAddr     string `json:"remote_addr"`
	Region         string `json:"region"`
	Operator       string `json:"operator"`
	TotalRequests  int64  `json:"total_requests"`
	RecentRequests int64  `json:"recent_requests"`
	LastSeenEpoch  int64  `json:"last_seen_epoch"`
}

type NodeAccessLogIPTrendQuery struct {
	NodeID        string
	RemoteAddr    string
	Host          string
	Since         time.Time
	BucketMinutes int
}

type NodeAccessLogTrendPointRow struct {
	BucketEpoch  int64 `json:"bucket_epoch"`
	RequestCount int64 `json:"request_count"`
}

type NodeAccessLogDistributionQuery struct {
	NodeID string
	Host   string
	Since  time.Time
	Limit  int
}

type NodeAccessLogDistributionRow struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
}

type NodeAccessLogMeteringSummary struct {
	RequestCount          int64 `json:"request_count"`
	RequestBytes          int64 `json:"request_bytes"`
	ResponseBytes         int64 `json:"response_bytes"`
	UpstreamBytes         int64 `json:"upstream_bytes"`
	UpstreamBytesHitCount int64 `json:"upstream_bytes_hit_count"`
	CacheHitCount         int64 `json:"cache_hit_count"`
	CacheMissCount        int64 `json:"cache_miss_count"`
	CacheBypassCount      int64 `json:"cache_bypass_count"`
	CacheExpiredCount     int64 `json:"cache_expired_count"`
	CacheStaleCount       int64 `json:"cache_stale_count"`
	CacheClassifiedCount  int64 `json:"cache_classified_count"`
}

type NodeAccessLogMeteringTrafficRow struct {
	Key           string `json:"key"`
	RequestCount  int64  `json:"request_count"`
	RequestBytes  int64  `json:"request_bytes"`
	ResponseBytes int64  `json:"response_bytes"`
	UpstreamBytes int64  `json:"upstream_bytes"`
}

type nodeAccessLogBucketAccumulator struct {
	requestCount     int64
	uniqueIPs        map[string]struct{}
	uniqueHosts      map[string]struct{}
	successCount     int64
	clientErrorCount int64
	serverErrorCount int64
}

type nodeAccessLogDedupKey struct {
	nodeID     string
	loggedAtNS int64
	remoteAddr string
	host       string
	path       string
	statusCode int
}

type nodeAccessLogTimeRange struct {
	min time.Time
	max time.Time
}

type nodeAccessLogPathFilter struct {
	exact  string
	prefix string
}

func (log *NodeAccessLog) BeforeCreate(tx *gorm.DB) error {
	return assignObservabilityID(&log.ID)
}

func ListNodeAccessLogs(query NodeAccessLogQuery) (logs []*NodeAccessLog, err error) {
	return listNodeAccessLogsAcrossShards(query)
}

func CountNodeAccessLogs(query NodeAccessLogQuery) (totalRecords int64, totalIPs int64, err error) {
	if totalRecords, totalIPs, ok, err := countNodeAccessLogsFromRollups(query); ok || err != nil {
		return totalRecords, totalIPs, err
	}
	totalRecords, totalIPs, err = countNodeAccessLogsSQL(query)
	if err == nil {
		return totalRecords, totalIPs, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return 0, 0, err
	}
	logNodeAccessLogSQLFallback("count access logs", err)
	return countNodeAccessLogsInMemory(query)
}

func countNodeAccessLogsSQL(query NodeAccessLogQuery) (totalRecords int64, totalIPs int64, err error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return 0, 0, fmt.Errorf("database handle is nil")
	}
	countBranches, countArgs := buildNodeAccessLogUnionBranches(query, "COUNT(*) AS total_records")
	ipBranches, ipArgs := buildNodeAccessLogUnionBranchesWithSuffix(
		query,
		"remote_addr AS remote_addr",
		"GROUP BY remote_addr",
		"remote_addr <> ''",
	)

	var row struct {
		TotalRecords int64
		TotalIPs     int64
	}
	sql := "WITH access_log_counts AS (" +
		strings.Join(countBranches, " UNION ALL ") +
		"), access_log_ips AS (" +
		strings.Join(ipBranches, " UNION ALL ") +
		"), grouped_access_log_ips AS (" +
		"SELECT remote_addr FROM access_log_ips GROUP BY remote_addr" +
		") SELECT COALESCE((SELECT SUM(total_records) FROM access_log_counts), 0) AS total_records, " +
		"(SELECT COUNT(*) FROM grouped_access_log_ips) AS total_ips"
	args := append(countArgs, ipArgs...)
	if err := db.Raw(sql, args...).Scan(&row).Error; err != nil {
		return 0, 0, fmt.Errorf("count access logs across shards failed: %w", err)
	}
	return row.TotalRecords, row.TotalIPs, nil
}

func countNodeAccessLogsInMemory(query NodeAccessLogQuery) (totalRecords int64, totalIPs int64, err error) {
	db := normalizeShardedDB(DB)
	ips := make(map[string]struct{})
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []struct {
			RemoteAddr   string
			RequestCount int64
		}
		if err := applyNodeAccessLogFilters(db.Table(table), query).
			Select("remote_addr, COUNT(*) AS request_count").
			Group("remote_addr").
			Scan(&rows).Error; err != nil {
			return 0, 0, err
		}
		for _, row := range rows {
			totalRecords += row.RequestCount
			if trimmed := strings.TrimSpace(row.RemoteAddr); trimmed != "" {
				ips[trimmed] = struct{}{}
			}
		}
	}
	return totalRecords, int64(len(ips)), nil
}

const nodeAccessLogListColumns = "id, node_id, logged_at, remote_addr, region, operator, host, path, status_code, reason, cache_status, request_bytes, response_bytes, upstream_bytes, created_at"

func ListNodeAccessLogRegionCounts(nodeID string, since time.Time, limit int) (items []*NodeAccessLogRegionCount, err error) {
	items, err = listNodeAccessLogRegionCountsSQL(nodeID, since, limit)
	if err == nil {
		return items, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("list access log region counts", err)
	return listNodeAccessLogRegionCountsInMemory(nodeID, since, limit)
}

func listNodeAccessLogRegionCountsSQL(nodeID string, since time.Time, limit int) ([]*NodeAccessLogRegionCount, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	query := NodeAccessLogQuery{
		NodeID: nodeID,
		Since:  since,
	}
	branches, args := buildNodeAccessLogUnionBranches(query, "region", "region <> ''")
	sql := "WITH access_log_region_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT TRIM(region) AS region, COUNT(*) AS count FROM access_log_region_rows " +
		"WHERE TRIM(COALESCE(region, '')) <> '' GROUP BY TRIM(region) ORDER BY count desc, region asc"
	if limit > 0 {
		sql += " LIMIT ?"
		args = append(args, limit)
	}

	var rows []*NodeAccessLogRegionCount
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log region counts across shards failed: %w", err)
	}
	return rows, nil
}

func listNodeAccessLogRegionCountsInMemory(nodeID string, since time.Time, limit int) (items []*NodeAccessLogRegionCount, err error) {
	query := NodeAccessLogQuery{
		NodeID: nodeID,
		Since:  since,
	}
	db := normalizeShardedDB(DB)
	counts := make(map[string]int64)
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLogRegionCount
		if err := applyNodeAccessLogFilters(db.Table(table), query).
			Select("region, COUNT(*) AS count").
			Where("region <> ''").
			Group("region").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			region := strings.TrimSpace(row.Region)
			if region == "" {
				continue
			}
			counts[region] += row.Count
		}
	}
	items = make([]*NodeAccessLogRegionCount, 0, len(counts))
	for region, count := range counts {
		items = append(items, &NodeAccessLogRegionCount{
			Region: region,
			Count:  count,
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Region < items[j].Region
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func ListNodeAccessLogBuckets(query NodeAccessLogBucketQuery) (items []*NodeAccessLogBucketRow, err error) {
	if items, ok, err := listNodeAccessLogBucketsFromRollups(query); ok || err != nil {
		return items, err
	}
	items, err = listNodeAccessLogBucketsSQL(query)
	if err == nil {
		return items, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("list access log buckets", err)

	rows, err := buildNodeAccessLogBucketRows(query)
	if err != nil {
		return nil, err
	}
	start, end := paginateBoundsWithLookahead(len(rows), query.Page, query.PageSize, query.Lookahead)
	if start >= len(rows) {
		return []*NodeAccessLogBucketRow{}, nil
	}
	return rows[start:end], nil
}

func CountNodeAccessLogBuckets(query NodeAccessLogBucketQuery) (total int64, err error) {
	if total, ok, err := countNodeAccessLogBucketsFromRollups(query); ok || err != nil {
		return total, err
	}
	total, err = countNodeAccessLogBucketsSQL(query)
	if err == nil {
		return total, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return 0, err
	}
	logNodeAccessLogSQLFallback("count access log buckets", err)

	return countNodeAccessLogBucketsInMemory(query)
}

func listNodeAccessLogBucketsSQL(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	bucketExpr := accessLogBucketEpochExpr(query.FoldMinutes)
	countBranches, countArgs := buildNodeAccessLogBucketUnionBranchesWithSuffix(
		query,
		bucketExpr+" AS bucket_epoch, "+
			"COUNT(*) AS request_count, "+
			"SUM(CASE WHEN status_code < 400 THEN 1 ELSE 0 END) AS success_count, "+
			"SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END) AS client_error_count, "+
			"SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END) AS server_error_count",
		"GROUP BY "+bucketExpr,
	)
	ipBranches, ipArgs := buildNodeAccessLogBucketUnionBranchesWithSuffix(
		query,
		bucketExpr+" AS bucket_epoch, remote_addr",
		"GROUP BY "+bucketExpr+", remote_addr",
		"remote_addr <> ''",
	)
	hostBranches, hostArgs := buildNodeAccessLogBucketUnionBranchesWithSuffix(
		query,
		bucketExpr+" AS bucket_epoch, host",
		"GROUP BY "+bucketExpr+", host",
		"host <> ''",
	)
	sql := "WITH bucket_count_rows AS (" +
		strings.Join(countBranches, " UNION ALL ") +
		"), grouped_bucket_counts AS (" +
		"SELECT bucket_epoch, " +
		"SUM(request_count) AS request_count, " +
		"SUM(success_count) AS success_count, " +
		"SUM(client_error_count) AS client_error_count, " +
		"SUM(server_error_count) AS server_error_count " +
		"FROM bucket_count_rows GROUP BY bucket_epoch" +
		"), bucket_ip_rows AS (" +
		strings.Join(ipBranches, " UNION ALL ") +
		"), grouped_bucket_ip_rows AS (" +
		"SELECT bucket_epoch, remote_addr FROM bucket_ip_rows GROUP BY bucket_epoch, remote_addr" +
		"), bucket_host_rows AS (" +
		strings.Join(hostBranches, " UNION ALL ") +
		"), grouped_bucket_host_rows AS (" +
		"SELECT bucket_epoch, host FROM bucket_host_rows GROUP BY bucket_epoch, host" +
		"), bucket_ip_counts AS (" +
		"SELECT bucket_epoch, COUNT(*) AS unique_ip_count FROM grouped_bucket_ip_rows GROUP BY bucket_epoch" +
		"), bucket_host_counts AS (" +
		"SELECT bucket_epoch, COUNT(*) AS unique_host_count FROM grouped_bucket_host_rows GROUP BY bucket_epoch" +
		"), grouped_bucket_rows AS (" +
		"SELECT grouped_bucket_counts.bucket_epoch, " +
		"grouped_bucket_counts.request_count, " +
		"COALESCE(bucket_ip_counts.unique_ip_count, 0) AS unique_ip_count, " +
		"COALESCE(bucket_host_counts.unique_host_count, 0) AS unique_host_count, " +
		"grouped_bucket_counts.success_count, " +
		"grouped_bucket_counts.client_error_count, " +
		"grouped_bucket_counts.server_error_count " +
		"FROM grouped_bucket_counts " +
		"LEFT JOIN bucket_ip_counts ON bucket_ip_counts.bucket_epoch = grouped_bucket_counts.bucket_epoch " +
		"LEFT JOIN bucket_host_counts ON bucket_host_counts.bucket_epoch = grouped_bucket_counts.bucket_epoch" +
		") SELECT bucket_epoch, request_count, unique_ip_count, unique_host_count, success_count, client_error_count, server_error_count " +
		"FROM grouped_bucket_rows ORDER BY " + buildNodeAccessLogBucketSortClause(query.SortBy, query.SortOrder)
	args := append(countArgs, ipArgs...)
	args = append(args, hostArgs...)
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		limit := query.PageSize + max(query.Lookahead, 0)
		sql += " LIMIT ? OFFSET ?"
		args = append(args, limit, page*query.PageSize)
	}

	var rows []*NodeAccessLogBucketRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log buckets across shards failed: %w", err)
	}
	return rows, nil
}

func countNodeAccessLogBucketsSQL(query NodeAccessLogBucketQuery) (int64, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}

	bucketExpr := accessLogBucketEpochExpr(query.FoldMinutes)
	branches, args := buildNodeAccessLogBucketUnionBranchesWithSuffix(
		query,
		bucketExpr+" AS bucket_epoch",
		"GROUP BY "+bucketExpr,
	)
	var row struct {
		Total int64
	}
	sql := "WITH bucket_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), grouped_bucket_rows AS (" +
		"SELECT bucket_epoch FROM bucket_rows GROUP BY bucket_epoch" +
		") SELECT COUNT(*) AS total FROM grouped_bucket_rows"
	if err := db.Raw(sql, args...).Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("count access log buckets across shards failed: %w", err)
	}
	return row.Total, nil
}

func countNodeAccessLogBucketsInMemory(query NodeAccessLogBucketQuery) (int64, error) {
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}
	db := normalizeShardedDB(DB)
	bucketExpr := accessLogBucketEpochExpr(query.FoldMinutes)
	buckets := make(map[int64]struct{})
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []struct {
			BucketEpoch int64
		}
		if err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(bucketExpr + " AS bucket_epoch").
			Group("bucket_epoch").
			Scan(&rows).Error; err != nil {
			return 0, err
		}
		for _, row := range rows {
			buckets[row.BucketEpoch] = struct{}{}
		}
	}
	return int64(len(buckets)), nil
}

func buildNodeAccessLogBucketUnionBranches(query NodeAccessLogBucketQuery, columns string) ([]string, []any) {
	return buildNodeAccessLogBucketUnionBranchesWithSuffix(query, columns, "")
}

func buildNodeAccessLogBucketUnionBranchesWithSuffix(
	query NodeAccessLogBucketQuery,
	columns string,
	branchSuffix string,
	extraWhereClauses ...string,
) ([]string, []any) {
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}
	return buildNodeAccessLogUnionBranchesWithSuffix(modelQuery, columns, branchSuffix, extraWhereClauses...)
}

func buildNodeAccessLogUnionBranches(query NodeAccessLogQuery, columns string, extraWhereClauses ...string) ([]string, []any) {
	return buildNodeAccessLogUnionBranchesWithSuffix(query, columns, "", extraWhereClauses...)
}

func buildNodeAccessLogUnionBranchesWithSuffix(
	query NodeAccessLogQuery,
	columns string,
	branchSuffix string,
	extraWhereClauses ...string,
) ([]string, []any) {
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*5)
	for _, table := range observabilityShardTables("node_access_logs") {
		branch := "SELECT " + columns + " FROM " + quoteIdentifier(table)
		clauses := make([]string, 0, 1+len(extraWhereClauses))
		whereClause, whereArgs := buildNodeAccessLogRawWhereClause(query)
		if whereClause != "" {
			clauses = append(clauses, whereClause)
			args = append(args, whereArgs...)
		}
		for _, extraClause := range extraWhereClauses {
			if trimmed := strings.TrimSpace(extraClause); trimmed != "" {
				clauses = append(clauses, trimmed)
			}
		}
		if len(clauses) > 0 {
			branch += " WHERE " + strings.Join(clauses, " AND ")
		}
		if trimmedSuffix := strings.TrimSpace(branchSuffix); trimmedSuffix != "" {
			branch += " " + trimmedSuffix
		}
		branches = append(branches, branch)
	}
	return branches, args
}

func ListNodeAccessLogIPSummaries(query NodeAccessLogIPSummaryQuery, recentSince time.Time) (items []*NodeAccessLogIPSummaryRow, err error) {
	if items, ok, err := listNodeAccessLogIPSummariesFromRollups(query, recentSince); ok || err != nil {
		return items, err
	}
	items, err = listNodeAccessLogIPSummariesSQL(query, recentSince)
	if err == nil {
		if err := enrichNodeAccessLogIPSummaryRows(query, items); err != nil {
			return nil, err
		}
		return items, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("list access log ip summaries", err)

	rows, err := buildNodeAccessLogIPSummaryRows(query, recentSince)
	if err != nil {
		return nil, err
	}
	start, end := paginateBoundsWithLookahead(len(rows), query.Page, query.PageSize, query.Lookahead)
	if start >= len(rows) {
		return []*NodeAccessLogIPSummaryRow{}, nil
	}
	items = rows[start:end]
	if err := enrichNodeAccessLogIPSummaryRows(query, items); err != nil {
		return nil, err
	}
	return items, nil
}

func CountNodeAccessLogIPSummaries(query NodeAccessLogIPSummaryQuery) (total int64, err error) {
	if total, ok, err := countNodeAccessLogIPSummariesFromRollups(query); ok || err != nil {
		return total, err
	}
	total, err = countNodeAccessLogIPSummariesSQL(query)
	if err == nil {
		return total, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return 0, err
	}
	logNodeAccessLogSQLFallback("count access log ip summaries", err)

	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	_, totalIPs, err := countNodeAccessLogsInMemory(modelQuery)
	return totalIPs, err
}

func listNodeAccessLogIPSummariesSQL(query NodeAccessLogIPSummaryQuery, recentSince time.Time) ([]*NodeAccessLogIPSummaryRow, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*4+3)
	recentRequestsExpr := "0"
	if !recentSince.IsZero() {
		recentRequestsExpr = "SUM(CASE WHEN logged_at >= ? THEN 1 ELSE 0 END)"
	}
	lastSeenExpr := accessLogEpochExpr("MAX(logged_at)")
	for _, table := range observabilityShardTables("node_access_logs") {
		branch := "SELECT remote_addr, " +
			"COUNT(*) AS total_requests, " +
			recentRequestsExpr + " AS recent_requests, " +
			lastSeenExpr + " AS last_seen_epoch " +
			"FROM " + quoteIdentifier(table)
		whereClause, whereArgs := buildNodeAccessLogRawWhereClause(modelQuery)
		if whereClause != "" {
			branch += " WHERE " + whereClause + " AND remote_addr <> ''"
		} else {
			branch += " WHERE remote_addr <> ''"
		}
		branch += " GROUP BY remote_addr"
		if !recentSince.IsZero() {
			args = append(args, recentSince)
		}
		args = append(args, whereArgs...)
		branches = append(branches, branch)
	}
	sql := "WITH access_log_ip_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), grouped_ip_rows AS (" +
		"SELECT remote_addr AS remote_addr, " +
		"SUM(total_requests) AS total_requests, " +
		"SUM(recent_requests) AS recent_requests, " +
		"MAX(last_seen_epoch) AS last_seen_epoch " +
		"FROM access_log_ip_rows WHERE remote_addr <> '' GROUP BY remote_addr" +
		") SELECT remote_addr, '' AS region, '' AS operator, total_requests, recent_requests, last_seen_epoch FROM grouped_ip_rows ORDER BY " +
		buildNodeAccessLogIPSummarySortClause(query.SortBy, query.SortOrder)
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		limit := query.PageSize + max(query.Lookahead, 0)
		sql += " LIMIT ? OFFSET ?"
		args = append(args, limit, page*query.PageSize)
	}

	var rows []*NodeAccessLogIPSummaryRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log ip summaries across shards failed: %w", err)
	}
	return rows, nil
}

func countNodeAccessLogIPSummariesSQL(query NodeAccessLogIPSummaryQuery) (int64, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	branches, args := buildNodeAccessLogUnionBranchesWithSuffix(
		modelQuery,
		"remote_addr",
		"GROUP BY remote_addr",
		"remote_addr <> ''",
	)

	var row struct {
		Total int64
	}
	sql := "WITH access_log_ip_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), grouped_access_log_ips AS (" +
		"SELECT remote_addr FROM access_log_ip_rows WHERE remote_addr <> '' GROUP BY remote_addr" +
		") SELECT COUNT(*) AS total FROM grouped_access_log_ips"
	if err := db.Raw(sql, args...).Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("count access log ip summaries across shards failed: %w", err)
	}
	return row.Total, nil
}

func ListNodeAccessLogIPTrend(query NodeAccessLogIPTrendQuery) (items []*NodeAccessLogTrendPointRow, err error) {
	if items, ok, err := listNodeAccessLogIPTrendFromRollups(query); ok || err != nil {
		return items, err
	}
	items, err = listNodeAccessLogIPTrendSQL(query)
	if err == nil {
		return items, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("list access log ip trend", err)
	return listNodeAccessLogIPTrendInMemory(query)
}

func shouldFallbackNodeAccessLogSQL(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"syntax error",
		"near \"(\"",
		"window function",
		"no such function",
		"does not support",
		"unsupported",
	} {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func logNodeAccessLogSQLFallback(operation string, err error) {
	slog.Warn("access log sql query fallback", "operation", operation, "error", err)
}

func listNodeAccessLogIPTrendSQL(query NodeAccessLogIPTrendQuery) ([]*NodeAccessLogTrendPointRow, error) {
	remoteAddr := strings.TrimSpace(query.RemoteAddr)
	if remoteAddr == "" {
		return []*NodeAccessLogTrendPointRow{}, nil
	}
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Since:      query.Since,
	}
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*6)
	for _, table := range observabilityShardTables("node_access_logs") {
		branch := "SELECT logged_at FROM " + quoteIdentifier(table)
		clauses := make([]string, 0, 2)
		whereClause, whereArgs := buildNodeAccessLogRawWhereClause(modelQuery)
		if whereClause != "" {
			clauses = append(clauses, whereClause)
			args = append(args, whereArgs...)
		}
		clauses = append(clauses, "remote_addr = ?")
		args = append(args, remoteAddr)
		branch += " WHERE " + strings.Join(clauses, " AND ")
		branches = append(branches, branch)
	}

	bucketExpr := accessLogBucketEpochExpr(query.BucketMinutes)
	sql := "WITH access_log_ip_trend_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), bucket_rows AS (" +
		"SELECT " + bucketExpr + " AS bucket_epoch FROM access_log_ip_trend_rows" +
		") SELECT bucket_epoch, COUNT(*) AS request_count FROM bucket_rows GROUP BY bucket_epoch ORDER BY bucket_epoch asc"
	var rows []*NodeAccessLogTrendPointRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log ip trend across shards failed: %w", err)
	}
	return rows, nil
}

func listNodeAccessLogIPTrendInMemory(query NodeAccessLogIPTrendQuery) (items []*NodeAccessLogTrendPointRow, err error) {
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Since:      query.Since,
	}
	remoteAddr := strings.TrimSpace(query.RemoteAddr)
	if remoteAddr == "" {
		return []*NodeAccessLogTrendPointRow{}, nil
	}
	buckets := make(map[int64]int64)
	db := normalizeShardedDB(DB)
	bucketExpr := accessLogBucketEpochExpr(query.BucketMinutes)
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLogTrendPointRow
		if err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(bucketExpr+" AS bucket_epoch, COUNT(*) AS request_count").
			Where("remote_addr = ?", remoteAddr).
			Group("bucket_epoch").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			buckets[row.BucketEpoch] += row.RequestCount
		}
	}
	items = make([]*NodeAccessLogTrendPointRow, 0, len(buckets))
	for bucketEpoch, requestCount := range buckets {
		items = append(items, &NodeAccessLogTrendPointRow{
			BucketEpoch:  bucketEpoch,
			RequestCount: requestCount,
		})
	}
	sort.Slice(items, func(i int, j int) bool {
		return items[i].BucketEpoch < items[j].BucketEpoch
	})
	return items, nil
}

func ListNodeAccessLogHostDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "host"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, "host", "host <> ''")
}

func ListNodeAccessLogPathDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "path"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, "path", "path <> ''")
}

func ListNodeAccessLogURLDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "url_key"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, accessLogURLKeyExpr(), "(host <> '' OR path <> '')")
}

func ListNodeAccessLogIPDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "remote_addr"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, "remote_addr", "remote_addr <> ''")
}

func ListNodeAccessLogRegionDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "region"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, "region", "region <> ''")
}

func ListNodeAccessLogStatusDistributions(query NodeAccessLogDistributionQuery) ([]*NodeAccessLogDistributionRow, error) {
	if rows, ok, err := listNodeAccessLogDistributionRowsFromRollups(query, "status_code"); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogDistributionRows(query, accessLogStatusCodeKeyExpr(), "status_code > 0")
}

func GetNodeAccessLogMeteringSummary(since time.Time) (*NodeAccessLogMeteringSummary, error) {
	if summary, ok, err := getNodeAccessLogMeteringSummaryFromRollups(since); ok || err != nil {
		return summary, err
	}
	summary, err := getNodeAccessLogMeteringSummarySQL(since)
	if err == nil {
		return summary, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("get access log metering summary", err)
	return getNodeAccessLogMeteringSummaryInMemory(since)
}

func getNodeAccessLogMeteringSummarySQL(since time.Time) (*NodeAccessLogMeteringSummary, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	query := NodeAccessLogQuery{Since: since}
	columns := "request_bytes, response_bytes, upstream_bytes, cache_status"
	branches, args := buildNodeAccessLogUnionBranches(query, columns)
	sql := "WITH access_log_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT " +
		"COUNT(*) AS request_count, " +
		"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
		"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
		"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes, " +
		"COALESCE(SUM(CASE WHEN upstream_bytes > 0 THEN 1 ELSE 0 END), 0) AS upstream_bytes_hit_count, " +
		"COALESCE(SUM(CASE WHEN cache_status = 'HIT' THEN 1 ELSE 0 END), 0) AS cache_hit_count, " +
		"COALESCE(SUM(CASE WHEN cache_status = 'MISS' THEN 1 ELSE 0 END), 0) AS cache_miss_count, " +
		"COALESCE(SUM(CASE WHEN cache_status = 'BYPASS' THEN 1 ELSE 0 END), 0) AS cache_bypass_count, " +
		"COALESCE(SUM(CASE WHEN cache_status = 'EXPIRED' THEN 1 ELSE 0 END), 0) AS cache_expired_count, " +
		"COALESCE(SUM(CASE WHEN cache_status = 'STALE' THEN 1 ELSE 0 END), 0) AS cache_stale_count, " +
		"COALESCE(SUM(CASE WHEN cache_status IN ('HIT', 'MISS', 'BYPASS', 'EXPIRED', 'STALE') THEN 1 ELSE 0 END), 0) AS cache_classified_count " +
		"FROM access_log_rows"
	var summary NodeAccessLogMeteringSummary
	if err := db.Raw(sql, args...).Scan(&summary).Error; err != nil {
		return nil, fmt.Errorf("query access log metering summary across shards failed: %w", err)
	}
	return &summary, nil
}

func getNodeAccessLogMeteringSummaryInMemory(since time.Time) (*NodeAccessLogMeteringSummary, error) {
	query := NodeAccessLogQuery{Since: since}
	db := normalizeShardedDB(DB)
	summary := &NodeAccessLogMeteringSummary{}
	for _, table := range observabilityShardTables("node_access_logs") {
		var row NodeAccessLogMeteringSummary
		if err := applyNodeAccessLogFilters(db.Table(table), query).
			Select(
				"COUNT(*) AS request_count, " +
					"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
					"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
					"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes, " +
					"SUM(CASE WHEN upstream_bytes > 0 THEN 1 ELSE 0 END) AS upstream_bytes_hit_count, " +
					"SUM(CASE WHEN cache_status = 'HIT' THEN 1 ELSE 0 END) AS cache_hit_count, " +
					"SUM(CASE WHEN cache_status = 'MISS' THEN 1 ELSE 0 END) AS cache_miss_count, " +
					"SUM(CASE WHEN cache_status = 'BYPASS' THEN 1 ELSE 0 END) AS cache_bypass_count, " +
					"SUM(CASE WHEN cache_status = 'EXPIRED' THEN 1 ELSE 0 END) AS cache_expired_count, " +
					"SUM(CASE WHEN cache_status = 'STALE' THEN 1 ELSE 0 END) AS cache_stale_count, " +
					"SUM(CASE WHEN cache_status IN ('HIT', 'MISS', 'BYPASS', 'EXPIRED', 'STALE') THEN 1 ELSE 0 END) AS cache_classified_count",
			).
			Scan(&row).Error; err != nil {
			return nil, err
		}
		summary.RequestCount += row.RequestCount
		summary.RequestBytes += row.RequestBytes
		summary.ResponseBytes += row.ResponseBytes
		summary.UpstreamBytes += row.UpstreamBytes
		summary.UpstreamBytesHitCount += row.UpstreamBytesHitCount
		summary.CacheHitCount += row.CacheHitCount
		summary.CacheMissCount += row.CacheMissCount
		summary.CacheBypassCount += row.CacheBypassCount
		summary.CacheExpiredCount += row.CacheExpiredCount
		summary.CacheStaleCount += row.CacheStaleCount
		summary.CacheClassifiedCount += row.CacheClassifiedCount
	}
	return summary, nil
}

func ListNodeAccessLogMeteringTrafficByHost(since time.Time, limit int) ([]*NodeAccessLogMeteringTrafficRow, error) {
	if rows, ok, err := listNodeAccessLogMeteringTrafficRowsFromRollups(since, "host", limit); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogMeteringTrafficRows(NodeAccessLogQuery{Since: since}, "host", "host <> ''", limit)
}

func ListNodeAccessLogMeteringTrafficByNode(since time.Time, limit int) ([]*NodeAccessLogMeteringTrafficRow, error) {
	if rows, ok, err := listNodeAccessLogMeteringTrafficRowsFromRollups(since, "node_id", limit); ok || err != nil {
		return rows, err
	}
	return buildNodeAccessLogMeteringTrafficRows(NodeAccessLogQuery{Since: since}, "node_id", "node_id <> ''", limit)
}

func DeleteNodeAccessLogsBefore(before time.Time) (deleted int64, err error) {
	deleted, err = deleteAcrossShards(DB, "node_access_logs", &NodeAccessLog{}, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("logged_at < ?", before)
	})
	if err == nil && deleted > 0 {
		err = RebuildNodeAccessLogRollups(DB)
	}
	return deleted, err
}

func DeleteAllNodeAccessLogs(db *gorm.DB) (deleted int64, err error) {
	deleted, err = deleteAcrossShards(db, "node_access_logs", &NodeAccessLog{}, nil)
	if err == nil {
		err = DeleteAllNodeAccessLogRollups(db)
	}
	return deleted, err
}

func NodeAccessLogExists(db *gorm.DB, record *NodeAccessLog) (bool, error) {
	if record == nil {
		return false, nil
	}
	db = normalizeShardedDB(db)
	for _, table := range observabilityShardTables("node_access_logs") {
		var count int64
		if err := db.Table(table).
			Where(
				"node_id = ? AND logged_at = ? AND remote_addr = ? AND host = ? AND path = ? AND status_code = ?",
				record.NodeID,
				record.LoggedAt,
				record.RemoteAddr,
				record.Host,
				record.Path,
				record.StatusCode,
			).
			Limit(1).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func InsertNewNodeAccessLogs(db *gorm.DB, records []*NodeAccessLog) (inserted int64, err error) {
	if len(records) == 0 {
		return 0, nil
	}
	db = normalizeShardedDB(db)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	uniqueRecords := make([]*NodeAccessLog, 0, len(records))
	seenIncoming := make(map[nodeAccessLogDedupKey]struct{}, len(records))
	rangesByNode := make(map[string]nodeAccessLogTimeRange)
	for _, record := range records {
		if record == nil {
			continue
		}
		key := nodeAccessLogDedupKeyFor(record)
		if _, exists := seenIncoming[key]; exists {
			continue
		}
		seenIncoming[key] = struct{}{}
		uniqueRecords = append(uniqueRecords, record)
		rangesByNode[key.nodeID] = expandNodeAccessLogTimeRange(rangesByNode[key.nodeID], record.LoggedAt)
	}
	if len(uniqueRecords) == 0 {
		return 0, nil
	}

	existingKeys, err := existingNodeAccessLogDedupKeys(db, rangesByNode)
	if err != nil {
		return 0, err
	}
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	grouped := make(map[string][]*NodeAccessLog, observabilityShardCount)
	insertedRecords := make([]*NodeAccessLog, 0, len(uniqueRecords))
	for _, record := range uniqueRecords {
		if _, exists := existingKeys[nodeAccessLogDedupKeyFor(record)]; exists {
			continue
		}
		if err := assignObservabilityID(&record.ID); err != nil {
			return inserted, err
		}
		table := observabilityShardTableForID("node_access_logs", record.ID)
		grouped[table] = append(grouped[table], record)
		existingKeys[nodeAccessLogDedupKeyFor(record)] = struct{}{}
	}
	for table, batch := range grouped {
		if len(batch) == 0 {
			continue
		}
		if err := rawDB.Session(&gorm.Session{SkipHooks: true}).Table(table).CreateInBatches(batch, 500).Error; err != nil {
			return inserted, fmt.Errorf("insert access logs into %s failed: %w", table, err)
		}
		inserted += int64(len(batch))
		insertedRecords = append(insertedRecords, batch...)
	}
	if err := upsertNodeAccessLogRollups(rawDB, insertedRecords); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func existingNodeAccessLogDedupKeys(db *gorm.DB, rangesByNode map[string]nodeAccessLogTimeRange) (map[nodeAccessLogDedupKey]struct{}, error) {
	keys := make(map[nodeAccessLogDedupKey]struct{})
	if len(rangesByNode) == 0 {
		return keys, nil
	}
	db = normalizeShardedDB(db)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	nodeIDs := nodeIDsFromTimeRanges(rangesByNode)
	timeRange := combinedNodeTimeRange(rangesByNode)
	if len(nodeIDs) == 0 {
		return keys, nil
	}
	if sqlKeys, err := existingNodeAccessLogDedupKeysSQL(db, nodeIDs, timeRange); err == nil {
		return sqlKeys, nil
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLog
		query := db.Table(table).
			Select("node_id, logged_at, remote_addr, host, path, status_code").
			Where("node_id IN ?", nodeIDs)
		if !timeRange.min.IsZero() {
			query = query.Where("logged_at >= ?", timeRange.min)
		}
		if !timeRange.max.IsZero() {
			query = query.Where("logged_at <= ?", timeRange.max)
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query existing access log keys from %s failed: %w", table, err)
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			keys[nodeAccessLogDedupKeyFor(row)] = struct{}{}
		}
	}
	return keys, nil
}

func existingNodeAccessLogDedupKeysSQL(db *gorm.DB, nodeIDs []string, timeRange nodeAccessLogTimeRange) (map[nodeAccessLogDedupKey]struct{}, error) {
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*(len(nodeIDs)+2))
	for _, table := range observabilityShardTables("node_access_logs") {
		branch := "SELECT node_id, logged_at, remote_addr, host, path, status_code FROM " + quoteIdentifier(table) +
			" WHERE node_id IN ?"
		branchArgs := []any{nodeIDs}
		if !timeRange.min.IsZero() {
			branch += " AND logged_at >= ?"
			branchArgs = append(branchArgs, timeRange.min)
		}
		if !timeRange.max.IsZero() {
			branch += " AND logged_at <= ?"
			branchArgs = append(branchArgs, timeRange.max)
		}
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	var rows []*NodeAccessLog
	sql := strings.Join(branches, " UNION ALL ")
	if err := rawDB.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query existing access log keys across shards failed: %w", err)
	}
	keys := make(map[nodeAccessLogDedupKey]struct{}, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		keys[nodeAccessLogDedupKeyFor(row)] = struct{}{}
	}
	return keys, nil
}

func nodeAccessLogDedupKeyFor(record *NodeAccessLog) nodeAccessLogDedupKey {
	if record == nil {
		return nodeAccessLogDedupKey{}
	}
	return nodeAccessLogDedupKey{
		nodeID:     strings.TrimSpace(record.NodeID),
		loggedAtNS: record.LoggedAt.UTC().UnixNano(),
		remoteAddr: strings.TrimSpace(record.RemoteAddr),
		host:       strings.TrimSpace(record.Host),
		path:       strings.TrimSpace(record.Path),
		statusCode: record.StatusCode,
	}
}

func expandNodeAccessLogTimeRange(current nodeAccessLogTimeRange, value time.Time) nodeAccessLogTimeRange {
	value = value.UTC()
	if current.min.IsZero() || value.Before(current.min) {
		current.min = value
	}
	if current.max.IsZero() || value.After(current.max) {
		current.max = value
	}
	return current
}

func nodeIDsFromTimeRanges(rangesByNode map[string]nodeAccessLogTimeRange) []string {
	nodeIDs := make([]string, 0, len(rangesByNode))
	for nodeID := range rangesByNode {
		nodeIDs = append(nodeIDs, strings.TrimSpace(nodeID))
	}
	sort.Strings(nodeIDs)
	return nodeIDs
}

func combinedNodeTimeRange(rangesByNode map[string]nodeAccessLogTimeRange) nodeAccessLogTimeRange {
	var combined nodeAccessLogTimeRange
	for _, timeRange := range rangesByNode {
		if !timeRange.min.IsZero() {
			combined = expandNodeAccessLogTimeRange(combined, timeRange.min)
		}
		if !timeRange.max.IsZero() {
			combined = expandNodeAccessLogTimeRange(combined, timeRange.max)
		}
	}
	return combined
}

func DeleteNodeAccessLogsByNodeBefore(db *gorm.DB, nodeID string, before time.Time) (deleted int64, err error) {
	deleted, err = deleteAcrossShards(db, "node_access_logs", &NodeAccessLog{}, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("node_id = ? AND logged_at < ?", nodeID, before)
	})
	if err == nil && deleted > 0 {
		err = RebuildNodeAccessLogRollups(db)
	}
	return deleted, err
}

func buildNodeAccessLogQuery(db *gorm.DB, query NodeAccessLogQuery) *gorm.DB {
	if db == nil {
		db = DB.Model(&NodeAccessLog{})
	}
	if db.Statement == nil || db.Statement.Model == nil {
		db = db.Model(&NodeAccessLog{})
	}
	return applyNodeAccessLogFilters(db, query)
}

func applyNodeAccessLogFilters(db *gorm.DB, query NodeAccessLogQuery) *gorm.DB {
	if trimmed := strings.TrimSpace(query.NodeID); trimmed != "" {
		db = db.Where("node_id = ?", trimmed)
	}
	if trimmed := strings.TrimSpace(query.RemoteAddr); trimmed != "" {
		db = db.Where("remote_addr LIKE ?", trimmed+"%")
	}
	if patterns := nodeAccessLogHostFilterPatterns(query.Host); len(patterns) > 0 {
		db = db.Where(nodeAccessLogHostWhereClause(), patterns...)
	}
	if filter := nodeAccessLogPathFilterFromRaw(query.Path); !filter.empty() {
		clause, args := filter.whereClause()
		db = db.Where(clause, args...)
	}
	if !query.Since.IsZero() {
		db = db.Where("logged_at >= ?", query.Since)
	}
	if clause, args := nodeAccessLogCursorWhereClause(query); clause != "" {
		db = db.Where(clause, args...)
	}
	return db
}

func listNodeAccessLogsAcrossShards(query NodeAccessLogQuery) ([]*NodeAccessLog, error) {
	rows, err := listNodeAccessLogsAcrossShardsSQL(query)
	if err == nil {
		return rows, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("list access logs", err)
	return listNodeAccessLogsAcrossShardsInMemory(query)
}

func listNodeAccessLogsAcrossShardsSQL(query NodeAccessLogQuery) ([]*NodeAccessLog, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*5+2)
	sortClause := buildNodeAccessLogSortClause(query.SortBy, query.SortOrder)
	pageSize := query.PageSize
	page := query.Page
	if page < 0 {
		page = 0
	}
	offset := 0
	outerLimit := 0
	shardLimit := 0
	if pageSize > 0 {
		offset = page * pageSize
		outerLimit = pageSize + max(query.Lookahead, 0)
		shardLimit = offset + outerLimit
	}
	for index, table := range observabilityShardTables("node_access_logs") {
		branch := "SELECT " + nodeAccessLogListColumns + " FROM " + quoteIdentifier(table)
		whereClause, whereArgs := buildNodeAccessLogRawWhereClause(query)
		if whereClause != "" {
			branch += " WHERE " + whereClause
			args = append(args, whereArgs...)
		}
		if shardLimit > 0 {
			branch += " ORDER BY " + sortClause + " LIMIT ?"
			args = append(args, shardLimit)
			branch = "SELECT " + nodeAccessLogListColumns + " FROM (" + branch + ") AS access_log_shard_" + fmt.Sprint(index)
		}
		branches = append(branches, branch)
	}

	sql := "WITH access_log_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT " + nodeAccessLogListColumns + " FROM access_log_rows ORDER BY " + sortClause
	if outerLimit > 0 {
		sql += " LIMIT ? OFFSET ?"
		args = append(args, outerLimit, offset)
	}

	var rows []*NodeAccessLog
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access logs across shards failed: %w", err)
	}
	return rows, nil
}

func buildNodeAccessLogRawWhereClause(query NodeAccessLogQuery) (string, []any) {
	whereClauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if trimmed := strings.TrimSpace(query.NodeID); trimmed != "" {
		whereClauses = append(whereClauses, "node_id = ?")
		args = append(args, trimmed)
	}
	if trimmed := strings.TrimSpace(query.RemoteAddr); trimmed != "" {
		whereClauses = append(whereClauses, "remote_addr LIKE ?")
		args = append(args, trimmed+"%")
	}
	if patterns := nodeAccessLogHostFilterPatterns(query.Host); len(patterns) > 0 {
		whereClauses = append(whereClauses, nodeAccessLogHostWhereClause())
		args = append(args, patterns...)
	}
	if filter := nodeAccessLogPathFilterFromRaw(query.Path); !filter.empty() {
		clause, clauseArgs := filter.whereClause()
		whereClauses = append(whereClauses, clause)
		args = append(args, clauseArgs...)
	}
	if !query.Since.IsZero() {
		whereClauses = append(whereClauses, "logged_at >= ?")
		args = append(args, query.Since)
	}
	if clause, clauseArgs := nodeAccessLogCursorWhereClause(query); clause != "" {
		whereClauses = append(whereClauses, clause)
		args = append(args, clauseArgs...)
	}
	return strings.Join(whereClauses, " AND "), args
}

func nodeAccessLogCursorWhereClause(query NodeAccessLogQuery) (string, []any) {
	if !nodeAccessLogQueryUsesTimeCursor(query) {
		return "", nil
	}
	if normalizeSortOrder(query.SortOrder) == "asc" {
		return "(logged_at > ? OR (logged_at = ? AND id > ?))", []any{query.CursorLoggedAt, query.CursorLoggedAt, query.CursorID}
	}
	return "(logged_at < ? OR (logged_at = ? AND id < ?))", []any{query.CursorLoggedAt, query.CursorLoggedAt, query.CursorID}
}

func nodeAccessLogQueryUsesTimeCursor(query NodeAccessLogQuery) bool {
	if query.CursorLoggedAt.IsZero() || query.CursorID == 0 {
		return false
	}
	return strings.TrimSpace(query.SortBy) == "" || strings.TrimSpace(query.SortBy) == "logged_at"
}

func nodeAccessLogHostWhereClause() string {
	return "(host = ? OR host LIKE ?)"
}

func nodeAccessLogHostFilterPatterns(raw string) []any {
	host := normalizeNodeAccessLogHostFilter(raw)
	if host == "" {
		return nil
	}
	return []any{host, "%." + host}
}

func (filter nodeAccessLogPathFilter) empty() bool {
	return filter.exact == "" && filter.prefix == ""
}

func (filter nodeAccessLogPathFilter) whereClause() (string, []any) {
	switch {
	case filter.exact != "" && filter.prefix != "":
		return "(path = ? OR path LIKE ? ESCAPE '\\')", []any{filter.exact, escapeSQLLikePattern(filter.prefix) + "%"}
	case filter.exact != "":
		return "path = ?", []any{filter.exact}
	case filter.prefix != "":
		return "path LIKE ? ESCAPE '\\'", []any{escapeSQLLikePattern(filter.prefix) + "%"}
	default:
		return "", nil
	}
}

func nodeAccessLogPathFilterFromRaw(raw string) nodeAccessLogPathFilter {
	path := normalizeNodeAccessLogPathFilter(raw)
	if path == "" {
		return nodeAccessLogPathFilter{}
	}
	if path == "/" {
		return nodeAccessLogPathFilter{exact: path}
	}
	return nodeAccessLogPathFilter{
		exact:  path,
		prefix: strings.TrimRight(path, "/") + "/",
	}
}

func normalizeNodeAccessLogPathFilter(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil {
		switch {
		case parsed.Scheme != "" && parsed.Host != "":
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return strings.TrimSpace(parsed.String())
		case parsed.Path != "":
			value = parsed.Path
		}
	} else if strings.Contains(value, "://") {
		return ""
	}
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		value = value[:index]
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func escapeSQLLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func normalizeNodeAccessLogHostFilter(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	} else if strings.Contains(value, "://") {
		return ""
	}
	value = strings.TrimSpace(value)
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
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

func listNodeAccessLogsAcrossShardsInMemory(query NodeAccessLogQuery) ([]*NodeAccessLog, error) {
	pageSize := query.PageSize
	limit := 0
	offset := 0
	shardLimit := 0
	if pageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		limit = pageSize + max(query.Lookahead, 0)
		offset = page * pageSize
		shardLimit = offset + limit
	}
	items, err := queryAcrossShards("node_access_logs", func(tx *gorm.DB) ([]*NodeAccessLog, error) {
		var shardRows []*NodeAccessLog
		db := applyNodeAccessLogFilters(tx, query).Order(buildNodeAccessLogSortClause(query.SortBy, query.SortOrder))
		if shardLimit > 0 {
			db = db.Limit(shardLimit)
		}
		if err := db.Find(&shardRows).Error; err != nil {
			return nil, err
		}
		return shardRows, nil
	})
	if err != nil {
		return nil, err
	}
	sortNodeAccessLogs(items, query.SortBy, query.SortOrder)
	if pageSize <= 0 {
		return items, nil
	}
	start := offset
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	if start >= len(items) {
		return []*NodeAccessLog{}, nil
	}
	return items[start:end], nil
}

func buildNodeAccessLogBucketRows(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, error) {
	rows, err := listNodeAccessLogBucketRowsAcrossShards(query)
	if err != nil {
		return nil, err
	}
	sortNodeAccessLogBucketRows(rows, query.SortBy, query.SortOrder)
	return rows, nil
}

func listNodeAccessLogBucketRowsAcrossShards(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, error) {
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}
	accumulators := make(map[int64]*nodeAccessLogBucketAccumulator)
	db := normalizeShardedDB(DB)
	bucketExpr := accessLogBucketEpochExpr(query.FoldMinutes)
	for _, table := range observabilityShardTables("node_access_logs") {
		var countRows []*NodeAccessLogBucketRow
		if err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(
				bucketExpr + " AS bucket_epoch, COUNT(*) AS request_count, " +
					"SUM(CASE WHEN status_code < 400 THEN 1 ELSE 0 END) AS success_count, " +
					"SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END) AS client_error_count, " +
					"SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END) AS server_error_count",
			).
			Group("bucket_epoch").
			Scan(&countRows).Error; err != nil {
			return nil, err
		}
		for _, row := range countRows {
			if row == nil {
				continue
			}
			accumulator := bucketAccumulatorForEpoch(accumulators, row.BucketEpoch)
			accumulator.requestCount += row.RequestCount
			accumulator.successCount += row.SuccessCount
			accumulator.clientErrorCount += row.ClientErrorCount
			accumulator.serverErrorCount += row.ServerErrorCount
		}

		var ipRows []struct {
			BucketEpoch int64
			RemoteAddr  string
		}
		if err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(bucketExpr + " AS bucket_epoch, remote_addr").
			Where("remote_addr <> ''").
			Group("bucket_epoch, remote_addr").
			Scan(&ipRows).Error; err != nil {
			return nil, err
		}
		for _, row := range ipRows {
			if trimmed := strings.TrimSpace(row.RemoteAddr); trimmed != "" {
				bucketAccumulatorForEpoch(accumulators, row.BucketEpoch).uniqueIPs[trimmed] = struct{}{}
			}
		}

		var hostRows []struct {
			BucketEpoch int64
			Host        string
		}
		if err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(bucketExpr + " AS bucket_epoch, host").
			Where("host <> ''").
			Group("bucket_epoch, host").
			Scan(&hostRows).Error; err != nil {
			return nil, err
		}
		for _, row := range hostRows {
			if trimmed := strings.TrimSpace(row.Host); trimmed != "" {
				bucketAccumulatorForEpoch(accumulators, row.BucketEpoch).uniqueHosts[trimmed] = struct{}{}
			}
		}
	}
	rows := make([]*NodeAccessLogBucketRow, 0, len(accumulators))
	for bucketEpoch, accumulator := range accumulators {
		rows = append(rows, &NodeAccessLogBucketRow{
			BucketEpoch:      bucketEpoch,
			RequestCount:     accumulator.requestCount,
			UniqueIPCount:    int64(len(accumulator.uniqueIPs)),
			UniqueHostCount:  int64(len(accumulator.uniqueHosts)),
			SuccessCount:     accumulator.successCount,
			ClientErrorCount: accumulator.clientErrorCount,
			ServerErrorCount: accumulator.serverErrorCount,
		})
	}
	return rows, nil
}

func bucketAccumulatorForEpoch(
	accumulators map[int64]*nodeAccessLogBucketAccumulator,
	bucketEpoch int64,
) *nodeAccessLogBucketAccumulator {
	accumulator := accumulators[bucketEpoch]
	if accumulator == nil {
		accumulator = &nodeAccessLogBucketAccumulator{
			uniqueIPs:   make(map[string]struct{}),
			uniqueHosts: make(map[string]struct{}),
		}
		accumulators[bucketEpoch] = accumulator
	}
	return accumulator
}

func buildNodeAccessLogIPSummaryRows(query NodeAccessLogIPSummaryQuery, recentSince time.Time) ([]*NodeAccessLogIPSummaryRow, error) {
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	type accumulator struct {
		totalRequests  int64
		recentRequests int64
		lastSeenEpoch  int64
	}
	accumulators := make(map[string]*accumulator)
	db := normalizeShardedDB(DB)
	lastSeenExpr := accessLogEpochExpr("MAX(logged_at)")
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLogIPSummaryRow
		shardQuery := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Where("remote_addr <> ''").
			Group("remote_addr")
		selectClause, selectArgs := buildNodeAccessLogIPSummarySelectClause(recentSince, lastSeenExpr)
		shardQuery = shardQuery.Select(selectClause, selectArgs...)
		if err := shardQuery.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			remoteAddr := strings.TrimSpace(row.RemoteAddr)
			if remoteAddr == "" {
				continue
			}
			acc := accumulators[remoteAddr]
			if acc == nil {
				acc = &accumulator{}
				accumulators[remoteAddr] = acc
			}
			acc.totalRequests += row.TotalRequests
			acc.recentRequests += row.RecentRequests
			if row.LastSeenEpoch > acc.lastSeenEpoch {
				acc.lastSeenEpoch = row.LastSeenEpoch
			}
		}
	}
	rows := make([]*NodeAccessLogIPSummaryRow, 0, len(accumulators))
	for remoteAddr, acc := range accumulators {
		rows = append(rows, &NodeAccessLogIPSummaryRow{
			RemoteAddr:     remoteAddr,
			TotalRequests:  acc.totalRequests,
			RecentRequests: acc.recentRequests,
			LastSeenEpoch:  acc.lastSeenEpoch,
		})
	}
	sortNodeAccessLogIPSummaryRows(rows, query.SortBy, query.SortOrder)
	return rows, nil
}

func buildNodeAccessLogIPSummarySelectClause(recentSince time.Time, lastSeenExpr string) (string, []any) {
	selectClause := "remote_addr, COUNT(*) AS total_requests, 0 AS recent_requests, " + lastSeenExpr + " AS last_seen_epoch"
	if !recentSince.IsZero() {
		selectClause = "remote_addr, COUNT(*) AS total_requests, " +
			"SUM(CASE WHEN logged_at >= ? THEN 1 ELSE 0 END) AS recent_requests, " +
			lastSeenExpr + " AS last_seen_epoch"
		return selectClause, []any{recentSince}
	}
	return selectClause, nil
}

func enrichNodeAccessLogIPSummaryRows(query NodeAccessLogIPSummaryQuery, rows []*NodeAccessLogIPSummaryRow) error {
	latestByRemoteAddr, err := latestNodeAccessLogsByRemoteAddr(query, rows)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row == nil || row.LastSeenEpoch <= 0 {
			continue
		}
		remoteAddr := strings.TrimSpace(row.RemoteAddr)
		if remoteAddr == "" {
			continue
		}
		latest := latestByRemoteAddr[remoteAddr]
		if latest == nil {
			continue
		}
		if region := strings.TrimSpace(latest.Region); region != "" {
			row.Region = region
		}
		if operator := strings.TrimSpace(latest.Operator); operator != "" {
			row.Operator = operator
		}
	}
	return nil
}

func latestNodeAccessLogsByRemoteAddr(query NodeAccessLogIPSummaryQuery, rows []*NodeAccessLogIPSummaryRow) (map[string]*NodeAccessLog, error) {
	latestByRemoteAddr := make(map[string]*NodeAccessLog)
	lastSeenEpochByRemoteAddr := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row == nil || row.LastSeenEpoch <= 0 {
			continue
		}
		remoteAddr := strings.TrimSpace(row.RemoteAddr)
		if remoteAddr == "" {
			continue
		}
		if row.LastSeenEpoch > lastSeenEpochByRemoteAddr[remoteAddr] {
			lastSeenEpochByRemoteAddr[remoteAddr] = row.LastSeenEpoch
		}
	}
	if len(lastSeenEpochByRemoteAddr) == 0 {
		return latestByRemoteAddr, nil
	}
	remoteAddrs := make([]string, 0, len(lastSeenEpochByRemoteAddr))
	for remoteAddr := range lastSeenEpochByRemoteAddr {
		remoteAddrs = append(remoteAddrs, remoteAddr)
	}
	sort.Strings(remoteAddrs)
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	db := normalizeShardedDB(DB)
	for _, table := range observabilityShardTables("node_access_logs") {
		var items []*NodeAccessLog
		err := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Where("remote_addr IN ?", remoteAddrs).
			Order("logged_at desc, id desc").
			Find(&items).Error
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			remoteAddr := strings.TrimSpace(item.RemoteAddr)
			if remoteAddr == "" {
				continue
			}
			lastSeenEpoch := lastSeenEpochByRemoteAddr[remoteAddr]
			if lastSeenEpoch <= 0 || item.LoggedAt.Unix() > lastSeenEpoch {
				continue
			}
			latest := latestByRemoteAddr[remoteAddr]
			if latest == nil || item.LoggedAt.After(latest.LoggedAt) || (item.LoggedAt.Equal(latest.LoggedAt) && item.ID > latest.ID) {
				copy := *item
				latestByRemoteAddr[remoteAddr] = &copy
			}
		}
	}
	return latestByRemoteAddr, nil
}

func nodeAccessLogQueryFromIPSummaryQuery(query NodeAccessLogIPSummaryQuery) NodeAccessLogQuery {
	return NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Since:      query.Since,
	}
}

func buildNodeAccessLogDistributionRows(
	query NodeAccessLogDistributionQuery,
	keyExpr string,
	nonEmptyClause string,
) ([]*NodeAccessLogDistributionRow, error) {
	rows, err := buildNodeAccessLogDistributionRowsSQL(query, keyExpr, nonEmptyClause)
	if err == nil {
		return rows, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("build access log distribution rows", err)
	return buildNodeAccessLogDistributionRowsInMemory(query, keyExpr, nonEmptyClause)
}

func buildNodeAccessLogDistributionRowsSQL(
	query NodeAccessLogDistributionQuery,
	keyExpr string,
	nonEmptyClause string,
) ([]*NodeAccessLogDistributionRow, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	modelQuery := NodeAccessLogQuery{
		NodeID: query.NodeID,
		Host:   query.Host,
		Since:  query.Since,
	}
	branches, args := buildNodeAccessLogUnionBranches(modelQuery, keyExpr+" AS key", nonEmptyClause)
	sql := "WITH access_log_distribution_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), grouped_distribution_rows AS (" +
		"SELECT TRIM(key) AS key, COUNT(*) AS value FROM access_log_distribution_rows " +
		"WHERE TRIM(COALESCE(key, '')) <> '' GROUP BY TRIM(key)" +
		") SELECT key, value FROM grouped_distribution_rows ORDER BY value desc, key asc"
	if query.Limit > 0 {
		sql += " LIMIT ?"
		args = append(args, query.Limit)
	}

	var rows []*NodeAccessLogDistributionRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log distribution across shards failed: %w", err)
	}
	return rows, nil
}

func buildNodeAccessLogDistributionRowsInMemory(
	query NodeAccessLogDistributionQuery,
	keyExpr string,
	nonEmptyClause string,
) ([]*NodeAccessLogDistributionRow, error) {
	modelQuery := NodeAccessLogQuery{
		NodeID: query.NodeID,
		Host:   query.Host,
		Since:  query.Since,
	}
	db := normalizeShardedDB(DB)
	counts := make(map[string]int64)
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLogDistributionRow
		shardQuery := applyNodeAccessLogFilters(db.Table(table), modelQuery).
			Select(keyExpr + " AS key, COUNT(*) AS value").
			Group("key")
		if strings.TrimSpace(nonEmptyClause) != "" {
			shardQuery = shardQuery.Where(nonEmptyClause)
		}
		if err := shardQuery.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			key := strings.TrimSpace(row.Key)
			if key == "" {
				continue
			}
			counts[key] += row.Value
		}
	}
	rows := make([]*NodeAccessLogDistributionRow, 0, len(counts))
	for key, value := range counts {
		rows = append(rows, &NodeAccessLogDistributionRow{
			Key:   key,
			Value: value,
		})
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].Value == rows[j].Value {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Value > rows[j].Value
	})
	if query.Limit > 0 && len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}

func buildNodeAccessLogMeteringTrafficRows(
	query NodeAccessLogQuery,
	keyExpr string,
	nonEmptyClause string,
	limit int,
) ([]*NodeAccessLogMeteringTrafficRow, error) {
	rows, err := buildNodeAccessLogMeteringTrafficRowsSQL(query, keyExpr, nonEmptyClause, limit)
	if err == nil {
		return rows, nil
	}
	if !shouldFallbackNodeAccessLogSQL(err) {
		return nil, err
	}
	logNodeAccessLogSQLFallback("build access log metering traffic rows", err)
	return buildNodeAccessLogMeteringTrafficRowsInMemory(query, keyExpr, nonEmptyClause, limit)
}

func buildNodeAccessLogMeteringTrafficRowsSQL(
	query NodeAccessLogQuery,
	keyExpr string,
	nonEmptyClause string,
	limit int,
) ([]*NodeAccessLogMeteringTrafficRow, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	columns := keyExpr + " AS key, request_bytes, response_bytes, upstream_bytes"
	branches, args := buildNodeAccessLogUnionBranches(query, columns, nonEmptyClause)
	sql := "WITH access_log_metering_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), grouped_metering_rows AS (" +
		"SELECT TRIM(key) AS key, " +
		"COUNT(*) AS request_count, " +
		"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
		"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
		"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes " +
		"FROM access_log_metering_rows WHERE TRIM(COALESCE(key, '')) <> '' GROUP BY TRIM(key)" +
		") SELECT key, request_count, request_bytes, response_bytes, upstream_bytes " +
		"FROM grouped_metering_rows ORDER BY response_bytes desc, request_count desc, key asc"
	if limit > 0 {
		sql += " LIMIT ?"
		args = append(args, limit)
	}

	var rows []*NodeAccessLogMeteringTrafficRow
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log metering traffic across shards failed: %w", err)
	}
	return rows, nil
}

func buildNodeAccessLogMeteringTrafficRowsInMemory(
	query NodeAccessLogQuery,
	keyExpr string,
	nonEmptyClause string,
	limit int,
) ([]*NodeAccessLogMeteringTrafficRow, error) {
	db := normalizeShardedDB(DB)
	rowsByKey := make(map[string]*NodeAccessLogMeteringTrafficRow)
	for _, table := range observabilityShardTables("node_access_logs") {
		var rows []*NodeAccessLogMeteringTrafficRow
		shardQuery := applyNodeAccessLogFilters(db.Table(table), query).
			Select(
				keyExpr + " AS key, " +
					"COUNT(*) AS request_count, " +
					"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
					"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
					"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes",
			).
			Group("key")
		if strings.TrimSpace(nonEmptyClause) != "" {
			shardQuery = shardQuery.Where(nonEmptyClause)
		}
		if err := shardQuery.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			key := strings.TrimSpace(row.Key)
			if key == "" {
				continue
			}
			acc := rowsByKey[key]
			if acc == nil {
				acc = &NodeAccessLogMeteringTrafficRow{Key: key}
				rowsByKey[key] = acc
			}
			acc.RequestCount += row.RequestCount
			acc.RequestBytes += row.RequestBytes
			acc.ResponseBytes += row.ResponseBytes
			acc.UpstreamBytes += row.UpstreamBytes
		}
	}
	rows := make([]*NodeAccessLogMeteringTrafficRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].ResponseBytes == rows[j].ResponseBytes {
			if rows[i].RequestCount == rows[j].RequestCount {
				return rows[i].Key < rows[j].Key
			}
			return rows[i].RequestCount > rows[j].RequestCount
		}
		return rows[i].ResponseBytes > rows[j].ResponseBytes
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func sortNodeAccessLogs(items []*NodeAccessLog, sortBy string, sortOrder string) {
	desc := normalizeSortOrder(sortOrder) != "asc"
	sort.Slice(items, func(i int, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || right == nil {
			return left != nil
		}
		var compare int
		switch strings.TrimSpace(sortBy) {
		case "status_code":
			compare = compareInt(left.StatusCode, right.StatusCode)
		case "remote_addr":
			compare = strings.Compare(left.RemoteAddr, right.RemoteAddr)
		case "host":
			compare = strings.Compare(left.Host, right.Host)
		case "path":
			compare = strings.Compare(left.Path, right.Path)
		default:
			compare = compareTime(left.LoggedAt, right.LoggedAt)
		}
		if compare == 0 {
			compare = compareTime(left.LoggedAt, right.LoggedAt)
		}
		if compare == 0 {
			compare = compareUint(left.ID, right.ID)
		}
		if desc {
			return compare > 0
		}
		return compare < 0
	})
}

func sortNodeAccessLogBucketRows(items []*NodeAccessLogBucketRow, sortBy string, sortOrder string) {
	desc := normalizeSortOrder(sortOrder) != "asc"
	sort.Slice(items, func(i int, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || right == nil {
			return left != nil
		}
		var compare int
		switch strings.TrimSpace(sortBy) {
		case "request_count":
			compare = compareInt64(left.RequestCount, right.RequestCount)
		default:
			compare = compareInt64(left.BucketEpoch, right.BucketEpoch)
		}
		if compare == 0 {
			compare = compareInt64(left.BucketEpoch, right.BucketEpoch)
		}
		if desc {
			return compare > 0
		}
		return compare < 0
	})
}

func sortNodeAccessLogIPSummaryRows(items []*NodeAccessLogIPSummaryRow, sortBy string, sortOrder string) {
	desc := normalizeSortOrder(sortOrder) != "asc"
	sort.Slice(items, func(i int, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || right == nil {
			return left != nil
		}
		var compare int
		switch strings.TrimSpace(sortBy) {
		case "recent_requests":
			compare = compareInt64(left.RecentRequests, right.RecentRequests)
		case "last_seen_at":
			compare = compareInt64(left.LastSeenEpoch, right.LastSeenEpoch)
		case "remote_addr":
			compare = strings.Compare(left.RemoteAddr, right.RemoteAddr)
		default:
			compare = compareInt64(left.TotalRequests, right.TotalRequests)
		}
		if compare == 0 {
			compare = compareInt64(left.LastSeenEpoch, right.LastSeenEpoch)
		}
		if compare == 0 {
			compare = strings.Compare(left.RemoteAddr, right.RemoteAddr)
		}
		if desc {
			return compare > 0
		}
		return compare < 0
	})
}

func paginateBounds(total int, page int, pageSize int) (int, int) {
	return paginateBoundsWithLookahead(total, page, pageSize, 0)
}

func paginateBoundsWithLookahead(total int, page int, pageSize int, lookahead int) (int, int) {
	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		return 0, total
	}
	start := page * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize + max(lookahead, 0)
	if end > total {
		end = total
	}
	return start, end
}

func compareTime(left time.Time, right time.Time) int {
	switch {
	case left.After(right):
		return 1
	case left.Before(right):
		return -1
	default:
		return 0
	}
}

func compareInt(left int, right int) int {
	switch {
	case left > right:
		return 1
	case left < right:
		return -1
	default:
		return 0
	}
}

func compareInt64(left int64, right int64) int {
	switch {
	case left > right:
		return 1
	case left < right:
		return -1
	default:
		return 0
	}
}

func compareUint(left uint, right uint) int {
	switch {
	case left > right:
		return 1
	case left < right:
		return -1
	default:
		return 0
	}
}

func buildNodeAccessLogSortClause(sortBy string, sortOrder string) string {
	column := "logged_at"
	switch strings.TrimSpace(sortBy) {
	case "status_code":
		column = "status_code"
	case "remote_addr":
		column = "remote_addr"
	case "host":
		column = "host"
	case "path":
		column = "path"
	}
	order := normalizeSortOrder(sortOrder)
	if column == "logged_at" {
		return fmt.Sprintf("%s %s, id %s", column, order, order)
	}
	return fmt.Sprintf("%s %s, logged_at desc, id desc", column, order)
}

func buildNodeAccessLogIPSummarySortClause(sortBy string, sortOrder string) string {
	order := normalizeSortOrder(sortOrder)
	switch strings.TrimSpace(sortBy) {
	case "recent_requests":
		return fmt.Sprintf("recent_requests %s, last_seen_epoch desc, remote_addr asc", order)
	case "last_seen_at":
		return fmt.Sprintf("last_seen_epoch %s, total_requests desc, remote_addr asc", order)
	case "remote_addr":
		return fmt.Sprintf("remote_addr %s", order)
	default:
		return fmt.Sprintf("total_requests %s, last_seen_epoch desc, remote_addr asc", order)
	}
}

func buildNodeAccessLogBucketSortClause(sortBy string, sortOrder string) string {
	order := normalizeSortOrder(sortOrder)
	switch strings.TrimSpace(sortBy) {
	case "request_count":
		return fmt.Sprintf("request_count %s, bucket_epoch %s", order, order)
	default:
		return fmt.Sprintf("bucket_epoch %s", order)
	}
}

func accessLogBucketEpochExpr(bucketMinutes int) string {
	return accessLogBucketEpochExprForColumn("logged_at", bucketMinutes)
}

func accessLogEpochExpr(expression string) string {
	switch databaseDialectorName(DB) {
	case "postgres":
		return fmt.Sprintf("CAST(extract(epoch from %s) AS BIGINT)", expression)
	default:
		return fmt.Sprintf("CAST(strftime('%%s', %s) AS INTEGER)", expression)
	}
}

func accessLogURLKeyExpr() string {
	switch databaseDialectorName(DB) {
	case "postgres":
		return "COALESCE(NULLIF(host, ''), '') || COALESCE(NULLIF(path, ''), '')"
	default:
		return "COALESCE(NULLIF(host, ''), '') || COALESCE(NULLIF(path, ''), '')"
	}
}

func accessLogStatusCodeKeyExpr() string {
	switch databaseDialectorName(DB) {
	case "postgres":
		return "CAST(status_code AS TEXT)"
	default:
		return "CAST(status_code AS TEXT)"
	}
}

func normalizeSortOrder(sortOrder string) string {
	if strings.EqualFold(strings.TrimSpace(sortOrder), "asc") {
		return "asc"
	}
	return "desc"
}
