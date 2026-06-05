package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type NodeRequestReport struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	NodeID              string    `json:"node_id" gorm:"index;size:64;not null"`
	WindowStartedAt     time.Time `json:"window_started_at" gorm:"index"`
	WindowEndedAt       time.Time `json:"window_ended_at" gorm:"index"`
	RequestCount        int64     `json:"request_count"`
	ErrorCount          int64     `json:"error_count"`
	CacheHitCount       int64     `json:"cache_hit_count"`
	CacheMissCount      int64     `json:"cache_miss_count"`
	CacheBypassCount    int64     `json:"cache_bypass_count"`
	CacheExpiredCount   int64     `json:"cache_expired_count"`
	CacheStaleCount     int64     `json:"cache_stale_count"`
	UpstreamErrorCount  int64     `json:"upstream_error_count"`
	UpstreamResponseMS  int64     `json:"upstream_response_ms"`
	UniqueVisitorCount  int64     `json:"unique_visitor_count"`
	StatusCodesJSON     string    `json:"status_codes_json" gorm:"type:text"`
	TopDomainsJSON      string    `json:"top_domains_json" gorm:"type:text"`
	SourceCountriesJSON string    `json:"source_countries_json" gorm:"type:text"`
	CreatedAt           time.Time `json:"created_at"`
}

type NodeRequestReportTrendBucket struct {
	BucketEpoch        int64 `json:"bucket_epoch"`
	RequestCount       int64 `json:"request_count"`
	ErrorCount         int64 `json:"error_count"`
	UniqueVisitorCount int64 `json:"unique_visitor_count"`
}

type NodeRequestReportCacheSummary struct {
	CacheHitCount        int64 `json:"cache_hit_count"`
	CacheMissCount       int64 `json:"cache_miss_count"`
	CacheBypassCount     int64 `json:"cache_bypass_count"`
	CacheExpiredCount    int64 `json:"cache_expired_count"`
	CacheStaleCount      int64 `json:"cache_stale_count"`
	CacheClassifiedCount int64 `json:"cache_classified_count"`
}

type NodeRequestReportTrafficSummary struct {
	RequestCount int64 `json:"request_count"`
	ErrorCount   int64 `json:"error_count"`
}

type nodeRequestReportDedupKey struct {
	nodeID            string
	windowStartedAtNS int64
	windowEndedAtNS   int64
}

func (report *NodeRequestReport) BeforeCreate(tx *gorm.DB) error {
	return assignObservabilityID(&report.ID)
}

func (report *NodeRequestReport) Insert() error {
	return DB.Create(report).Error
}

func ListNodeRequestReports(nodeID string, since time.Time, limit int) (reports []*NodeRequestReport, err error) {
	rows, err := listNodeRequestReportsSQL(nodeID, since, limit)
	if err == nil {
		return rows, nil
	}
	return listNodeRequestReportsInMemory(nodeID, since, limit)
}

func listNodeRequestReportsSQL(nodeID string, since time.Time, limit int) ([]*NodeRequestReport, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildRequestReportUnionBranches(nodeID, since, time.Time{}, "*")
	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT * FROM request_report_rows ORDER BY window_ended_at desc, id desc"
	if limit > 0 {
		sql += " LIMIT ?"
		args = append(args, limit)
	}
	var rows []*NodeRequestReport
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query node request reports across shards failed: %w", err)
	}
	return rows, nil
}

func listNodeRequestReportsInMemory(nodeID string, since time.Time, limit int) (reports []*NodeRequestReport, err error) {
	rows, err := queryAcrossShards("node_request_reports", func(tx *gorm.DB) ([]*NodeRequestReport, error) {
		var shardRows []*NodeRequestReport
		query := tx.Order("window_ended_at desc, id desc")
		if nodeID != "" {
			query = query.Where("node_id = ?", nodeID)
		}
		if !since.IsZero() {
			query = query.Where("window_ended_at >= ?", since)
		}
		if limit > 0 {
			query = query.Limit(limit)
		}
		if err := query.Find(&shardRows).Error; err != nil {
			return nil, err
		}
		return shardRows, nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].WindowEndedAt.Equal(rows[j].WindowEndedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].WindowEndedAt.After(rows[j].WindowEndedAt)
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func ListRequestReportsSince(since time.Time) (reports []*NodeRequestReport, err error) {
	rows, err := listRequestReportsSinceSQL(since)
	if err == nil {
		return rows, nil
	}
	return listRequestReportsSinceInMemory(since)
}

func listRequestReportsSinceSQL(since time.Time) ([]*NodeRequestReport, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildRequestReportUnionBranches("", since, time.Time{}, "*")
	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT * FROM request_report_rows ORDER BY window_ended_at desc, id desc"
	var rows []*NodeRequestReport
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query request reports across shards failed: %w", err)
	}
	return rows, nil
}

func listRequestReportsSinceInMemory(since time.Time) (reports []*NodeRequestReport, err error) {
	rows, err := queryAcrossShards("node_request_reports", func(tx *gorm.DB) ([]*NodeRequestReport, error) {
		var shardRows []*NodeRequestReport
		query := tx.Order("window_ended_at desc")
		if !since.IsZero() {
			query = query.Where("window_ended_at >= ?", since)
		}
		if err := query.Find(&shardRows).Error; err != nil {
			return nil, err
		}
		return shardRows, nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].WindowEndedAt.Equal(rows[j].WindowEndedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].WindowEndedAt.After(rows[j].WindowEndedAt)
	})
	return rows, nil
}

func buildRequestReportUnionBranches(nodeID string, since time.Time, until time.Time, columns string) ([]string, []any) {
	trimmedNodeID := strings.TrimSpace(nodeID)
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*3)
	for _, table := range observabilityShardTables("node_request_reports") {
		branch := "SELECT " + columns + " FROM " + quoteIdentifier(table)
		whereClauses := make([]string, 0, 3)
		if trimmedNodeID != "" {
			whereClauses = append(whereClauses, "node_id = ?")
			args = append(args, trimmedNodeID)
		}
		if !since.IsZero() {
			whereClauses = append(whereClauses, "window_ended_at >= ?")
			args = append(args, since)
		}
		if !until.IsZero() {
			whereClauses = append(whereClauses, "window_ended_at <= ?")
			args = append(args, until)
		}
		if len(whereClauses) > 0 {
			branch += " WHERE " + strings.Join(whereClauses, " AND ")
		}
		branches = append(branches, branch)
	}
	return branches, args
}

func ListLatestRequestReportsByNode(since time.Time, until time.Time) ([]*NodeRequestReport, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches, args := buildRequestReportUnionBranches("", since, until, "*")

	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), ranked AS (" +
		"SELECT *, ROW_NUMBER() OVER (PARTITION BY node_id ORDER BY window_ended_at DESC, id DESC) AS rn " +
		"FROM request_report_rows WHERE TRIM(COALESCE(node_id, '')) <> ''" +
		") SELECT * FROM ranked WHERE rn = 1 ORDER BY window_ended_at DESC, id DESC"
	var rows []*NodeRequestReport
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query latest request reports by node failed: %w", err)
	}
	return rows, nil
}

func ListRequestReportTrendBuckets(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeRequestReportTrendBucket, error) {
	rows, err := listRequestReportTrendBucketsSQL(nodeID, since, until, bucketMinutes)
	if err == nil {
		return rows, nil
	}
	return listRequestReportTrendBucketsInMemory(nodeID, since, until, bucketMinutes)
}

func listRequestReportTrendBucketsSQL(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeRequestReportTrendBucket, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildRequestReportUnionBranches(
		nodeID,
		since,
		until,
		"window_ended_at, request_count, error_count, unique_visitor_count",
	)

	bucketExpr := requestReportBucketEpochExpr(bucketMinutes)
	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), bucket_rows AS (" +
		"SELECT " + bucketExpr + " AS bucket_epoch, request_count, error_count, unique_visitor_count FROM request_report_rows" +
		") SELECT bucket_epoch, " +
		"COALESCE(SUM(request_count), 0) AS request_count, " +
		"COALESCE(SUM(error_count), 0) AS error_count, " +
		"COALESCE(SUM(unique_visitor_count), 0) AS unique_visitor_count " +
		"FROM bucket_rows GROUP BY bucket_epoch ORDER BY bucket_epoch asc"
	var rows []*NodeRequestReportTrendBucket
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query request report trend buckets across shards failed: %w", err)
	}
	return rows, nil
}

func listRequestReportTrendBucketsInMemory(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeRequestReportTrendBucket, error) {
	buckets := make(map[int64]*NodeRequestReportTrendBucket)
	db := normalizeShardedDB(DB)
	bucketExpr := requestReportBucketEpochExpr(bucketMinutes)
	for _, table := range observabilityShardTables("node_request_reports") {
		var rows []*NodeRequestReportTrendBucket
		query := db.Table(table).
			Select(
				bucketExpr + " AS bucket_epoch, " +
					"SUM(request_count) AS request_count, " +
					"SUM(error_count) AS error_count, " +
					"SUM(unique_visitor_count) AS unique_visitor_count",
			).
			Group("bucket_epoch")
		if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
			query = query.Where("node_id = ?", trimmed)
		}
		if !since.IsZero() {
			query = query.Where("window_ended_at >= ?", since)
		}
		if !until.IsZero() {
			query = query.Where("window_ended_at <= ?", until)
		}
		if err := query.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			acc := buckets[row.BucketEpoch]
			if acc == nil {
				acc = &NodeRequestReportTrendBucket{BucketEpoch: row.BucketEpoch}
				buckets[row.BucketEpoch] = acc
			}
			acc.RequestCount += row.RequestCount
			acc.ErrorCount += row.ErrorCount
			acc.UniqueVisitorCount += row.UniqueVisitorCount
		}
	}
	rows := make([]*NodeRequestReportTrendBucket, 0, len(buckets))
	for _, bucket := range buckets {
		rows = append(rows, bucket)
	}
	sort.Slice(rows, func(i int, j int) bool {
		return rows[i].BucketEpoch < rows[j].BucketEpoch
	})
	return rows, nil
}

func GetRequestReportTrafficSummary(since time.Time, until time.Time) (*NodeRequestReportTrafficSummary, error) {
	summary, err := getRequestReportTrafficSummarySQL(since, until)
	if err == nil {
		return summary, nil
	}
	return getRequestReportTrafficSummaryInMemory(since, until)
}

func getRequestReportTrafficSummarySQL(since time.Time, until time.Time) (*NodeRequestReportTrafficSummary, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildRequestReportUnionBranches("", since, until, "request_count, error_count")

	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT " +
		"COALESCE(SUM(request_count), 0) AS request_count, " +
		"COALESCE(SUM(error_count), 0) AS error_count " +
		"FROM request_report_rows"
	var summary NodeRequestReportTrafficSummary
	if err := db.Raw(sql, args...).Scan(&summary).Error; err != nil {
		return nil, fmt.Errorf("query request report traffic summary across shards failed: %w", err)
	}
	return &summary, nil
}

func getRequestReportTrafficSummaryInMemory(since time.Time, until time.Time) (*NodeRequestReportTrafficSummary, error) {
	db := normalizeShardedDB(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	summary := &NodeRequestReportTrafficSummary{}
	for _, table := range observabilityShardTables("node_request_reports") {
		var row NodeRequestReportTrafficSummary
		query := db.Table(table).Select(
			"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(SUM(error_count), 0) AS error_count",
		)
		if !since.IsZero() {
			query = query.Where("window_ended_at >= ?", since)
		}
		if !until.IsZero() {
			query = query.Where("window_ended_at <= ?", until)
		}
		if err := query.Scan(&row).Error; err != nil {
			return nil, fmt.Errorf("query request report traffic summary from %s failed: %w", table, err)
		}
		summary.RequestCount += row.RequestCount
		summary.ErrorCount += row.ErrorCount
	}
	return summary, nil
}

func GetRequestReportCacheSummary(since time.Time, until time.Time) (*NodeRequestReportCacheSummary, error) {
	summary, err := getRequestReportCacheSummarySQL(since, until)
	if err == nil {
		return summary, nil
	}
	return getRequestReportCacheSummaryInMemory(since, until)
}

func getRequestReportCacheSummarySQL(since time.Time, until time.Time) (*NodeRequestReportCacheSummary, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildRequestReportUnionBranches(
		"",
		since,
		until,
		"cache_hit_count, cache_miss_count, cache_bypass_count, cache_expired_count, cache_stale_count",
	)

	sql := "WITH request_report_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT " +
		"COALESCE(SUM(cache_hit_count), 0) AS cache_hit_count, " +
		"COALESCE(SUM(cache_miss_count), 0) AS cache_miss_count, " +
		"COALESCE(SUM(cache_bypass_count), 0) AS cache_bypass_count, " +
		"COALESCE(SUM(cache_expired_count), 0) AS cache_expired_count, " +
		"COALESCE(SUM(cache_stale_count), 0) AS cache_stale_count, " +
		"COALESCE(SUM(cache_hit_count + cache_miss_count + cache_bypass_count + cache_expired_count + cache_stale_count), 0) AS cache_classified_count " +
		"FROM request_report_rows"
	var summary NodeRequestReportCacheSummary
	if err := db.Raw(sql, args...).Scan(&summary).Error; err != nil {
		return nil, fmt.Errorf("query request report cache summary across shards failed: %w", err)
	}
	return &summary, nil
}

func getRequestReportCacheSummaryInMemory(since time.Time, until time.Time) (*NodeRequestReportCacheSummary, error) {
	db := normalizeShardedDB(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	summary := &NodeRequestReportCacheSummary{}
	for _, table := range observabilityShardTables("node_request_reports") {
		var row NodeRequestReportCacheSummary
		query := db.Table(table).Select(
			"COALESCE(SUM(cache_hit_count), 0) AS cache_hit_count, " +
				"COALESCE(SUM(cache_miss_count), 0) AS cache_miss_count, " +
				"COALESCE(SUM(cache_bypass_count), 0) AS cache_bypass_count, " +
				"COALESCE(SUM(cache_expired_count), 0) AS cache_expired_count, " +
				"COALESCE(SUM(cache_stale_count), 0) AS cache_stale_count, " +
				"COALESCE(SUM(cache_hit_count + cache_miss_count + cache_bypass_count + cache_expired_count + cache_stale_count), 0) AS cache_classified_count",
		)
		if !since.IsZero() {
			query = query.Where("window_ended_at >= ?", since)
		}
		if !until.IsZero() {
			query = query.Where("window_ended_at <= ?", until)
		}
		if err := query.Scan(&row).Error; err != nil {
			return nil, fmt.Errorf("query request report cache summary from %s failed: %w", table, err)
		}
		summary.CacheHitCount += row.CacheHitCount
		summary.CacheMissCount += row.CacheMissCount
		summary.CacheBypassCount += row.CacheBypassCount
		summary.CacheExpiredCount += row.CacheExpiredCount
		summary.CacheStaleCount += row.CacheStaleCount
		summary.CacheClassifiedCount += row.CacheClassifiedCount
	}
	return summary, nil
}

func NodeRequestReportExists(db *gorm.DB, nodeID string, windowStartedAt time.Time, windowEndedAt time.Time) (bool, error) {
	db = normalizeShardedDB(db)
	for _, table := range observabilityShardTables("node_request_reports") {
		var count int64
		if err := db.Table(table).
			Where("node_id = ? AND window_started_at = ? AND window_ended_at = ?", nodeID, windowStartedAt, windowEndedAt).
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

func InsertNewNodeRequestReports(db *gorm.DB, reports []*NodeRequestReport) (inserted int64, err error) {
	if len(reports) == 0 {
		return 0, nil
	}
	db = normalizeShardedDB(db)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	uniqueReports := make([]*NodeRequestReport, 0, len(reports))
	seenIncoming := make(map[nodeRequestReportDedupKey]struct{}, len(reports))
	rangesByNode := make(map[string]nodeAccessLogTimeRange)
	for _, report := range reports {
		if report == nil {
			continue
		}
		key := nodeRequestReportDedupKeyFor(report)
		if key.nodeID == "" || key.windowEndedAtNS == 0 {
			continue
		}
		if _, exists := seenIncoming[key]; exists {
			continue
		}
		seenIncoming[key] = struct{}{}
		uniqueReports = append(uniqueReports, report)
		rangesByNode[key.nodeID] = expandNodeAccessLogTimeRange(rangesByNode[key.nodeID], report.WindowEndedAt)
	}
	if len(uniqueReports) == 0 {
		return 0, nil
	}

	existingKeys, err := existingNodeRequestReportDedupKeys(db, rangesByNode)
	if err != nil {
		return 0, err
	}
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	grouped := make(map[string][]*NodeRequestReport, observabilityShardCount)
	for _, report := range uniqueReports {
		key := nodeRequestReportDedupKeyFor(report)
		if _, exists := existingKeys[key]; exists {
			continue
		}
		if err := assignObservabilityID(&report.ID); err != nil {
			return inserted, err
		}
		table := observabilityShardTableForID("node_request_reports", report.ID)
		grouped[table] = append(grouped[table], report)
		existingKeys[key] = struct{}{}
	}
	for table, batch := range grouped {
		if len(batch) == 0 {
			continue
		}
		if err := rawDB.Table(table).CreateInBatches(batch, 500).Error; err != nil {
			return inserted, fmt.Errorf("insert request reports into %s failed: %w", table, err)
		}
		inserted += int64(len(batch))
	}
	return inserted, nil
}

func existingNodeRequestReportDedupKeys(db *gorm.DB, rangesByNode map[string]nodeAccessLogTimeRange) (map[nodeRequestReportDedupKey]struct{}, error) {
	keys := make(map[nodeRequestReportDedupKey]struct{})
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
	if sqlKeys, err := existingNodeRequestReportDedupKeysSQL(db, nodeIDs, timeRange); err == nil {
		return sqlKeys, nil
	}
	for _, table := range observabilityShardTables("node_request_reports") {
		var rows []*NodeRequestReport
		query := db.Table(table).
			Select("node_id, window_started_at, window_ended_at").
			Where("node_id IN ?", nodeIDs)
		if !timeRange.min.IsZero() {
			query = query.Where("window_ended_at >= ?", timeRange.min)
		}
		if !timeRange.max.IsZero() {
			query = query.Where("window_ended_at <= ?", timeRange.max)
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query existing request report keys from %s failed: %w", table, err)
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			keys[nodeRequestReportDedupKeyFor(row)] = struct{}{}
		}
	}
	return keys, nil
}

func existingNodeRequestReportDedupKeysSQL(db *gorm.DB, nodeIDs []string, timeRange nodeAccessLogTimeRange) (map[nodeRequestReportDedupKey]struct{}, error) {
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*(len(nodeIDs)+2))
	for _, table := range observabilityShardTables("node_request_reports") {
		branch := "SELECT node_id, window_started_at, window_ended_at FROM " + quoteIdentifier(table) +
			" WHERE node_id IN ?"
		branchArgs := []any{nodeIDs}
		if !timeRange.min.IsZero() {
			branch += " AND window_ended_at >= ?"
			branchArgs = append(branchArgs, timeRange.min)
		}
		if !timeRange.max.IsZero() {
			branch += " AND window_ended_at <= ?"
			branchArgs = append(branchArgs, timeRange.max)
		}
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	var rows []*NodeRequestReport
	sql := strings.Join(branches, " UNION ALL ")
	if err := rawDB.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query existing request report keys across shards failed: %w", err)
	}
	keys := make(map[nodeRequestReportDedupKey]struct{}, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		keys[nodeRequestReportDedupKeyFor(row)] = struct{}{}
	}
	return keys, nil
}

func nodeRequestReportDedupKeyFor(report *NodeRequestReport) nodeRequestReportDedupKey {
	if report == nil {
		return nodeRequestReportDedupKey{}
	}
	return nodeRequestReportDedupKey{
		nodeID:            strings.TrimSpace(report.NodeID),
		windowStartedAtNS: report.WindowStartedAt.UTC().UnixNano(),
		windowEndedAtNS:   report.WindowEndedAt.UTC().UnixNano(),
	}
}

func DeleteNodeRequestReportsBefore(db *gorm.DB, before time.Time) (int64, error) {
	return deleteAcrossShards(db, "node_request_reports", &NodeRequestReport{}, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("window_ended_at < ?", before)
	})
}

func DeleteAllNodeRequestReports(db *gorm.DB) (int64, error) {
	return deleteAcrossShards(db, "node_request_reports", &NodeRequestReport{}, nil)
}

func requestReportBucketEpochExpr(bucketMinutes int) string {
	bucketSeconds := bucketMinutes * 60
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	switch databaseDialectorName(DB) {
	case "postgres":
		return fmt.Sprintf("CAST(floor(extract(epoch from window_ended_at) / %d) * %d AS BIGINT)", bucketSeconds, bucketSeconds)
	default:
		return fmt.Sprintf("CAST((strftime('%%s', window_ended_at) / %d) * %d AS INTEGER)", bucketSeconds, bucketSeconds)
	}
}
