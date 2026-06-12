package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/sharding"
)

const nodeAccessLogRollupBucketMinutes = 1

type NodeAccessLogRollup struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	BucketStartedAt       time.Time `json:"bucket_started_at" gorm:"index;uniqueIndex:idx_node_access_log_rollups_bucket_hash,priority:1"`
	DimensionHash         string    `json:"dimension_hash" gorm:"size:64;uniqueIndex:idx_node_access_log_rollups_bucket_hash,priority:2"`
	NodeID                string    `json:"node_id" gorm:"index;size:64"`
	RemoteAddr            string    `json:"remote_addr" gorm:"index;size:128"`
	Region                string    `json:"region" gorm:"index;size:128"`
	Operator              string    `json:"operator" gorm:"size:255"`
	Host                  string    `json:"host" gorm:"index;size:255"`
	Path                  string    `json:"path" gorm:"type:text"`
	URLKey                string    `json:"url_key" gorm:"type:text"`
	StatusCode            int       `json:"status_code" gorm:"index"`
	CacheStatus           string    `json:"cache_status" gorm:"index;size:32"`
	RequestCount          int64     `json:"request_count"`
	RequestBytes          int64     `json:"request_bytes"`
	ResponseBytes         int64     `json:"response_bytes"`
	UpstreamBytes         int64     `json:"upstream_bytes"`
	UpstreamBytesHitCount int64     `json:"upstream_bytes_hit_count"`
	CacheHitCount         int64     `json:"cache_hit_count"`
	CacheMissCount        int64     `json:"cache_miss_count"`
	CacheBypassCount      int64     `json:"cache_bypass_count"`
	CacheExpiredCount     int64     `json:"cache_expired_count"`
	CacheStaleCount       int64     `json:"cache_stale_count"`
	CacheClassifiedCount  int64     `json:"cache_classified_count"`
	LastSeenAt            time.Time `json:"last_seen_at" gorm:"index"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type nodeAccessLogRollupAggregateRow struct {
	BucketEpoch           int64
	NodeID                string
	RemoteAddr            string
	Region                string
	Operator              string
	Host                  string
	Path                  string
	StatusCode            int
	CacheStatus           string
	RequestCount          int64
	RequestBytes          int64
	ResponseBytes         int64
	UpstreamBytes         int64
	UpstreamBytesHitCount int64
	CacheHitCount         int64
	CacheMissCount        int64
	CacheBypassCount      int64
	CacheExpiredCount     int64
	CacheStaleCount       int64
	CacheClassifiedCount  int64
	LastSeenEpoch         int64
}

func (log *NodeAccessLog) AfterCreate(tx *gorm.DB) error {
	if log == nil {
		return nil
	}
	return upsertNodeAccessLogRollups(tx, []*NodeAccessLog{log})
}

func ensureNodeAccessLogRollupSchema(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	if err := db.AutoMigrate(&NodeAccessLogRollup{}); err != nil {
		return fmt.Errorf("auto migrate node access log rollups failed: %w", err)
	}
	return backfillNodeAccessLogRollupsIfEmpty(db)
}

func backfillNodeAccessLogRollupsIfEmpty(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	var count int64
	if err := nodeAccessLogRollupTableDB(db).Count(&count).Error; err != nil {
		return fmt.Errorf("count node access log rollups failed: %w", err)
	}
	if count > 0 {
		return nil
	}
	return RebuildNodeAccessLogRollups(db)
}

func RebuildNodeAccessLogRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	if err := nodeAccessLogRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogRollup{}).Error; err != nil {
		return fmt.Errorf("clear node access log rollups failed: %w", err)
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		if !db.Migrator().HasTable(table) {
			continue
		}
		rows, err := nodeAccessLogRollupRowsForShard(db, table)
		if err != nil {
			return err
		}
		if err := upsertNodeAccessLogRollupRows(db, rows); err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllNodeAccessLogRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	return nodeAccessLogRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogRollup{}).Error
}

func nodeAccessLogRollupRowsForShard(db *gorm.DB, table string) ([]*NodeAccessLogRollup, error) {
	bucketExpr := accessLogBucketEpochExprForColumn("logged_at", nodeAccessLogRollupBucketMinutes)
	lastSeenExpr := accessLogEpochExpr("MAX(logged_at)")
	var rows []nodeAccessLogRollupAggregateRow
	err := db.Table(table).
		Select(
			bucketExpr + " AS bucket_epoch, " +
				"node_id, remote_addr, region, operator, host, path, status_code, cache_status, " +
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
				"COALESCE(SUM(CASE WHEN cache_status IN ('HIT', 'MISS', 'BYPASS', 'EXPIRED', 'STALE') THEN 1 ELSE 0 END), 0) AS cache_classified_count, " +
				lastSeenExpr + " AS last_seen_epoch",
		).
		Group(bucketExpr + ", node_id, remote_addr, region, operator, host, path, status_code, cache_status").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query node access log rollups from %s failed: %w", table, err)
	}
	result := make([]*NodeAccessLogRollup, 0, len(rows))
	for _, row := range rows {
		result = append(result, nodeAccessLogRollupFromAggregate(row))
	}
	return result, nil
}

func upsertNodeAccessLogRollups(db *gorm.DB, records []*NodeAccessLog) error {
	rows := aggregateNodeAccessLogRollups(records)
	return upsertNodeAccessLogRollupRows(db, rows)
}

func aggregateNodeAccessLogRollups(records []*NodeAccessLog) []*NodeAccessLogRollup {
	byKey := make(map[string]*NodeAccessLogRollup, len(records))
	for _, record := range records {
		row := nodeAccessLogRollupFromRecord(record)
		if row == nil {
			continue
		}
		key := strconv.FormatInt(row.BucketStartedAt.Unix(), 10) + "\x00" + row.DimensionHash
		current := byKey[key]
		if current == nil {
			byKey[key] = row
			continue
		}
		mergeNodeAccessLogRollup(current, row)
	}
	rows := make([]*NodeAccessLogRollup, 0, len(byKey))
	for _, row := range byKey {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].BucketStartedAt.Equal(rows[j].BucketStartedAt) {
			return rows[i].DimensionHash < rows[j].DimensionHash
		}
		return rows[i].BucketStartedAt.Before(rows[j].BucketStartedAt)
	})
	return rows
}

func nodeAccessLogRollupFromRecord(record *NodeAccessLog) *NodeAccessLogRollup {
	if record == nil || record.LoggedAt.IsZero() {
		return nil
	}
	row := &NodeAccessLogRollup{
		BucketStartedAt: nodeAccessLogRollupBucketStart(record.LoggedAt),
		NodeID:          strings.TrimSpace(record.NodeID),
		RemoteAddr:      strings.TrimSpace(record.RemoteAddr),
		Region:          strings.TrimSpace(record.Region),
		Operator:        strings.TrimSpace(record.Operator),
		Host:            strings.TrimSpace(record.Host),
		Path:            strings.TrimSpace(record.Path),
		StatusCode:      record.StatusCode,
		CacheStatus:     strings.TrimSpace(record.CacheStatus),
		RequestCount:    1,
		RequestBytes:    nonNegativeRollupInt64(record.RequestBytes),
		ResponseBytes:   nonNegativeRollupInt64(record.ResponseBytes),
		UpstreamBytes:   nonNegativeRollupInt64(record.UpstreamBytes),
		LastSeenAt:      record.LoggedAt.UTC(),
	}
	row.URLKey = nodeAccessLogRollupURLKey(row.Host, row.Path)
	if row.UpstreamBytes > 0 {
		row.UpstreamBytesHitCount = 1
	}
	applyNodeAccessLogRollupCacheCounters(row, row.CacheStatus, 1)
	row.DimensionHash = nodeAccessLogRollupDimensionHash(row)
	return row
}

func nodeAccessLogRollupFromAggregate(row nodeAccessLogRollupAggregateRow) *NodeAccessLogRollup {
	rollup := &NodeAccessLogRollup{
		BucketStartedAt:       time.Unix(row.BucketEpoch, 0).UTC(),
		NodeID:                strings.TrimSpace(row.NodeID),
		RemoteAddr:            strings.TrimSpace(row.RemoteAddr),
		Region:                strings.TrimSpace(row.Region),
		Operator:              strings.TrimSpace(row.Operator),
		Host:                  strings.TrimSpace(row.Host),
		Path:                  strings.TrimSpace(row.Path),
		StatusCode:            row.StatusCode,
		CacheStatus:           strings.TrimSpace(row.CacheStatus),
		RequestCount:          row.RequestCount,
		RequestBytes:          row.RequestBytes,
		ResponseBytes:         row.ResponseBytes,
		UpstreamBytes:         row.UpstreamBytes,
		UpstreamBytesHitCount: row.UpstreamBytesHitCount,
		CacheHitCount:         row.CacheHitCount,
		CacheMissCount:        row.CacheMissCount,
		CacheBypassCount:      row.CacheBypassCount,
		CacheExpiredCount:     row.CacheExpiredCount,
		CacheStaleCount:       row.CacheStaleCount,
		CacheClassifiedCount:  row.CacheClassifiedCount,
		LastSeenAt:            time.Unix(row.LastSeenEpoch, 0).UTC(),
	}
	rollup.URLKey = nodeAccessLogRollupURLKey(rollup.Host, rollup.Path)
	rollup.DimensionHash = nodeAccessLogRollupDimensionHash(rollup)
	return rollup
}

func mergeNodeAccessLogRollup(target *NodeAccessLogRollup, source *NodeAccessLogRollup) {
	if target == nil || source == nil {
		return
	}
	target.RequestCount += source.RequestCount
	target.RequestBytes += source.RequestBytes
	target.ResponseBytes += source.ResponseBytes
	target.UpstreamBytes += source.UpstreamBytes
	target.UpstreamBytesHitCount += source.UpstreamBytesHitCount
	target.CacheHitCount += source.CacheHitCount
	target.CacheMissCount += source.CacheMissCount
	target.CacheBypassCount += source.CacheBypassCount
	target.CacheExpiredCount += source.CacheExpiredCount
	target.CacheStaleCount += source.CacheStaleCount
	target.CacheClassifiedCount += source.CacheClassifiedCount
	if source.LastSeenAt.After(target.LastSeenAt) {
		target.LastSeenAt = source.LastSeenAt
	}
}

func applyNodeAccessLogRollupCacheCounters(row *NodeAccessLogRollup, cacheStatus string, count int64) {
	if row == nil || count <= 0 {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(cacheStatus)) {
	case "HIT":
		row.CacheHitCount += count
	case "MISS":
		row.CacheMissCount += count
	case "BYPASS":
		row.CacheBypassCount += count
	case "EXPIRED":
		row.CacheExpiredCount += count
	case "STALE":
		row.CacheStaleCount += count
	default:
		return
	}
	row.CacheClassifiedCount += count
}

func upsertNodeAccessLogRollupRows(db *gorm.DB, rows []*NodeAccessLogRollup) error {
	if len(rows) == 0 {
		return nil
	}
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	updates := clause.Assignments(map[string]any{
		"request_count":            gorm.Expr("node_access_log_rollups.request_count + excluded.request_count"),
		"request_bytes":            gorm.Expr("node_access_log_rollups.request_bytes + excluded.request_bytes"),
		"response_bytes":           gorm.Expr("node_access_log_rollups.response_bytes + excluded.response_bytes"),
		"upstream_bytes":           gorm.Expr("node_access_log_rollups.upstream_bytes + excluded.upstream_bytes"),
		"upstream_bytes_hit_count": gorm.Expr("node_access_log_rollups.upstream_bytes_hit_count + excluded.upstream_bytes_hit_count"),
		"cache_hit_count":          gorm.Expr("node_access_log_rollups.cache_hit_count + excluded.cache_hit_count"),
		"cache_miss_count":         gorm.Expr("node_access_log_rollups.cache_miss_count + excluded.cache_miss_count"),
		"cache_bypass_count":       gorm.Expr("node_access_log_rollups.cache_bypass_count + excluded.cache_bypass_count"),
		"cache_expired_count":      gorm.Expr("node_access_log_rollups.cache_expired_count + excluded.cache_expired_count"),
		"cache_stale_count":        gorm.Expr("node_access_log_rollups.cache_stale_count + excluded.cache_stale_count"),
		"cache_classified_count":   gorm.Expr("node_access_log_rollups.cache_classified_count + excluded.cache_classified_count"),
		"last_seen_at":             gorm.Expr("CASE WHEN excluded.last_seen_at > node_access_log_rollups.last_seen_at THEN excluded.last_seen_at ELSE node_access_log_rollups.last_seen_at END"),
		"updated_at":               gorm.Expr("excluded.updated_at"),
	})
	return nodeAccessLogRollupTableDB(db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bucket_started_at"}, {Name: "dimension_hash"}},
		DoUpdates: updates,
	}).CreateInBatches(rows, 500).Error
}

func countNodeAccessLogsFromRollups(query NodeAccessLogQuery) (int64, int64, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return 0, 0, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return 0, 0, false, nil
	}
	base := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), query)
	var totalRecords int64
	if err := base.Select("COALESCE(SUM(request_count), 0)").Scan(&totalRecords).Error; err != nil {
		return 0, 0, false, fmt.Errorf("count access log records from rollups failed: %w", err)
	}
	var totalIPs int64
	if err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), query).
		Where("remote_addr <> ''").
		Distinct("remote_addr").
		Count(&totalIPs).Error; err != nil {
		return 0, 0, false, fmt.Errorf("count access log ips from rollups failed: %w", err)
	}
	return totalRecords, totalIPs, true, nil
}

func listNodeAccessLogBucketsFromRollups(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return nil, false, nil
	}
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.FoldMinutes)
	rollupQuery := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Select(
			bucketExpr + " AS bucket_epoch, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COUNT(DISTINCT CASE WHEN remote_addr <> '' THEN remote_addr ELSE NULL END) AS unique_ip_count, " +
				"COUNT(DISTINCT CASE WHEN host <> '' THEN host ELSE NULL END) AS unique_host_count, " +
				"COALESCE(SUM(CASE WHEN status_code < 400 THEN request_count ELSE 0 END), 0) AS success_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN request_count ELSE 0 END), 0) AS client_error_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 500 THEN request_count ELSE 0 END), 0) AS server_error_count",
		).
		Group("bucket_epoch").
		Order(buildNodeAccessLogBucketSortClause(query.SortBy, query.SortOrder))
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		rollupQuery = rollupQuery.Limit(query.PageSize + max(query.Lookahead, 0)).Offset(page * query.PageSize)
	}
	var rows []*NodeAccessLogBucketRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log buckets from rollups failed: %w", err)
	}
	return rows, true, nil
}

func countNodeAccessLogBucketsFromRollups(query NodeAccessLogBucketQuery) (int64, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return 0, false, nil
	}
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.FoldMinutes)
	grouped := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Select(bucketExpr + " AS bucket_epoch").
		Group("bucket_epoch")
	var total int64
	if err := nodeAccessLogRollupSession(DB).Table("(?) AS grouped_bucket_rows", grouped).Count(&total).Error; err != nil {
		return 0, false, fmt.Errorf("count access log buckets from rollups failed: %w", err)
	}
	return total, true, nil
}

func countNodeAccessLogIPSummariesFromRollups(query NodeAccessLogIPSummaryQuery) (int64, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return 0, false, nil
	}
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	var total int64
	if err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Where("remote_addr <> ''").
		Distinct("remote_addr").
		Count(&total).Error; err != nil {
		return 0, false, fmt.Errorf("count access log ip summaries from rollups failed: %w", err)
	}
	return total, true, nil
}

func listNodeAccessLogIPSummariesFromRollups(query NodeAccessLogIPSummaryQuery, recentSince time.Time) ([]*NodeAccessLogIPSummaryRow, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) || !nodeAccessLogRollupCanServeSince(recentSince) {
		return nil, false, nil
	}
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	recentExpr := "0"
	selectArgs := []any(nil)
	if !recentSince.IsZero() {
		recentExpr = "COALESCE(SUM(CASE WHEN bucket_started_at >= ? THEN request_count ELSE 0 END), 0)"
		selectArgs = append(selectArgs, nodeAccessLogRollupBucketStart(recentSince))
	}
	rollupQuery := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Where("remote_addr <> ''").
		Select(
			"remote_addr, '' AS region, '' AS operator, "+
				"COALESCE(SUM(request_count), 0) AS total_requests, "+
				recentExpr+" AS recent_requests, "+
				accessLogEpochExpr("MAX(last_seen_at)")+" AS last_seen_epoch",
			selectArgs...,
		).
		Group("remote_addr").
		Order(buildNodeAccessLogIPSummarySortClause(query.SortBy, query.SortOrder))
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		rollupQuery = rollupQuery.Limit(query.PageSize + max(query.Lookahead, 0)).Offset(page * query.PageSize)
	}
	var rows []*NodeAccessLogIPSummaryRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log ip summaries from rollups failed: %w", err)
	}
	if err := enrichNodeAccessLogIPSummaryRowsFromRollups(db, modelQuery, rows); err != nil {
		return nil, false, err
	}
	return rows, true, nil
}

func enrichNodeAccessLogIPSummaryRowsFromRollups(db *gorm.DB, modelQuery NodeAccessLogQuery, rows []*NodeAccessLogIPSummaryRow) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	remoteSet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row == nil || row.LastSeenEpoch <= 0 {
			continue
		}
		remoteAddr := strings.TrimSpace(row.RemoteAddr)
		if remoteAddr != "" {
			remoteSet[remoteAddr] = struct{}{}
		}
	}
	if len(remoteSet) == 0 {
		return nil
	}
	remoteAddrs := make([]string, 0, len(remoteSet))
	for remoteAddr := range remoteSet {
		remoteAddrs = append(remoteAddrs, remoteAddr)
	}
	sort.Strings(remoteAddrs)
	type latestRow struct {
		RemoteAddr string
		Region     string
		Operator   string
	}
	var latestRows []latestRow
	if err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Where("remote_addr IN ?", remoteAddrs).
		Order("last_seen_at desc, id desc").
		Select("remote_addr, region, operator").
		Scan(&latestRows).Error; err != nil {
		return fmt.Errorf("query access log ip summary metadata from rollups failed: %w", err)
	}
	latestByRemoteAddr := make(map[string]latestRow, len(remoteAddrs))
	for _, latest := range latestRows {
		remoteAddr := strings.TrimSpace(latest.RemoteAddr)
		if remoteAddr == "" {
			continue
		}
		if _, exists := latestByRemoteAddr[remoteAddr]; exists {
			continue
		}
		latestByRemoteAddr[remoteAddr] = latest
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		latest, ok := latestByRemoteAddr[strings.TrimSpace(row.RemoteAddr)]
		if !ok {
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

func listNodeAccessLogIPTrendFromRollups(query NodeAccessLogIPTrendQuery) ([]*NodeAccessLogTrendPointRow, bool, error) {
	remoteAddr := strings.TrimSpace(query.RemoteAddr)
	if remoteAddr == "" {
		return []*NodeAccessLogTrendPointRow{}, true, nil
	}
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return nil, false, nil
	}
	modelQuery := NodeAccessLogQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Since:      query.Since,
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.BucketMinutes)
	var rows []*NodeAccessLogTrendPointRow
	err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Where("remote_addr = ?", remoteAddr).
		Select(bucketExpr + " AS bucket_epoch, COALESCE(SUM(request_count), 0) AS request_count").
		Group("bucket_epoch").
		Order("bucket_epoch asc").
		Scan(&rows).Error
	if err != nil {
		return nil, false, fmt.Errorf("query access log ip trend from rollups failed: %w", err)
	}
	return rows, true, nil
}

func getNodeAccessLogMeteringSummaryFromRollups(since time.Time) (*NodeAccessLogMeteringSummary, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(since) {
		return nil, false, nil
	}
	var summary NodeAccessLogMeteringSummary
	err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), NodeAccessLogQuery{Since: since}).
		Select(
			"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
				"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
				"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes, " +
				"COALESCE(SUM(upstream_bytes_hit_count), 0) AS upstream_bytes_hit_count, " +
				"COALESCE(SUM(cache_hit_count), 0) AS cache_hit_count, " +
				"COALESCE(SUM(cache_miss_count), 0) AS cache_miss_count, " +
				"COALESCE(SUM(cache_bypass_count), 0) AS cache_bypass_count, " +
				"COALESCE(SUM(cache_expired_count), 0) AS cache_expired_count, " +
				"COALESCE(SUM(cache_stale_count), 0) AS cache_stale_count, " +
				"COALESCE(SUM(cache_classified_count), 0) AS cache_classified_count",
		).
		Scan(&summary).Error
	if err != nil {
		return nil, false, fmt.Errorf("query access log metering summary from rollups failed: %w", err)
	}
	return &summary, true, nil
}

func listNodeAccessLogDistributionRowsFromRollups(query NodeAccessLogDistributionQuery, dimension string) ([]*NodeAccessLogDistributionRow, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return nil, false, nil
	}
	selectExpr, nonEmptyClause := nodeAccessLogRollupDistributionDimension(dimension)
	if selectExpr == "" {
		return nil, false, nil
	}
	modelQuery := NodeAccessLogQuery{
		NodeID: query.NodeID,
		Host:   query.Host,
		Since:  query.Since,
	}
	rollupQuery := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Select(selectExpr + " AS key, COALESCE(SUM(request_count), 0) AS value").
		Group("key").
		Order("value desc, key asc")
	if nonEmptyClause != "" {
		rollupQuery = rollupQuery.Where(nonEmptyClause)
	}
	if query.Limit > 0 {
		rollupQuery = rollupQuery.Limit(query.Limit)
	}
	var rows []*NodeAccessLogDistributionRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log distribution from rollups failed: %w", err)
	}
	return rows, true, nil
}

func listNodeAccessLogMeteringTrafficRowsFromRollups(since time.Time, dimension string, limit int) ([]*NodeAccessLogMeteringTrafficRow, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(since) {
		return nil, false, nil
	}
	keyColumn := strings.TrimSpace(dimension)
	switch keyColumn {
	case "host", "node_id":
	default:
		return nil, false, nil
	}
	rollupQuery := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), NodeAccessLogQuery{Since: since}).
		Where(keyColumn + " <> ''").
		Select(
			keyColumn + " AS key, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(SUM(request_bytes), 0) AS request_bytes, " +
				"COALESCE(SUM(response_bytes), 0) AS response_bytes, " +
				"COALESCE(SUM(upstream_bytes), 0) AS upstream_bytes",
		).
		Group(keyColumn).
		Order("response_bytes desc, request_count desc, key asc")
	if limit > 0 {
		rollupQuery = rollupQuery.Limit(limit)
	}
	var rows []*NodeAccessLogMeteringTrafficRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log metering traffic from rollups failed: %w", err)
	}
	return rows, true, nil
}

func nodeAccessLogRollupQueryDB() (*gorm.DB, bool) {
	db := nodeAccessLogRollupSession(DB)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil, false
	}
	var count int64
	if err := nodeAccessLogRollupTableDB(db).Limit(1).Count(&count).Error; err != nil || count == 0 {
		return nil, false
	}
	return db, true
}

func nodeAccessLogRollupSession(db *gorm.DB) *gorm.DB {
	db = normalizeShardedDB(db)
	if db == nil {
		return nil
	}
	return db.Session(&gorm.Session{NewDB: true}).Set(sharding.ShardingIgnoreStoreKey, true)
}

func nodeAccessLogRollupTableDB(db *gorm.DB) *gorm.DB {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	return db.Table((NodeAccessLogRollup{}).TableName())
}

func nodeAccessLogRollupCanServeSince(since time.Time) bool {
	return since.IsZero() || since.Equal(nodeAccessLogRollupBucketStart(since))
}

func applyNodeAccessLogRollupFilters(db *gorm.DB, query NodeAccessLogQuery) *gorm.DB {
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
		db = db.Where("bucket_started_at >= ?", nodeAccessLogRollupBucketStart(query.Since))
	}
	return db
}

func nodeAccessLogRollupDistributionDimension(dimension string) (string, string) {
	switch strings.TrimSpace(dimension) {
	case "host":
		return "host", "host <> ''"
	case "path":
		return "path", "path <> ''"
	case "url_key":
		return "url_key", "url_key <> ''"
	case "remote_addr":
		return "remote_addr", "remote_addr <> ''"
	case "region":
		return "region", "region <> ''"
	case "status_code":
		return accessLogRollupStatusCodeKeyExpr(), "status_code > 0"
	default:
		return "", ""
	}
}

func accessLogRollupStatusCodeKeyExpr() string {
	switch databaseDialectorName(DB) {
	case "postgres":
		return "CAST(status_code AS TEXT)"
	default:
		return "CAST(status_code AS TEXT)"
	}
}

func accessLogBucketEpochExprForColumn(column string, bucketMinutes int) string {
	bucketSeconds := bucketMinutes * 60
	if bucketSeconds <= 0 {
		bucketSeconds = 180
	}
	switch databaseDialectorName(DB) {
	case "postgres":
		return fmt.Sprintf("CAST(floor(extract(epoch from %s) / %d) * %d AS BIGINT)", column, bucketSeconds, bucketSeconds)
	default:
		return fmt.Sprintf("CAST((strftime('%%s', %s) / %d) * %d AS INTEGER)", column, bucketSeconds, bucketSeconds)
	}
}

func nodeAccessLogRollupBucketStart(value time.Time) time.Time {
	return value.UTC().Truncate(time.Duration(nodeAccessLogRollupBucketMinutes) * time.Minute)
}

func nodeAccessLogRollupURLKey(host string, path string) string {
	return strings.TrimSpace(host) + strings.TrimSpace(path)
}

func nodeAccessLogRollupDimensionHash(row *NodeAccessLogRollup) string {
	if row == nil {
		return ""
	}
	parts := []string{
		row.NodeID,
		row.RemoteAddr,
		row.Region,
		row.Operator,
		row.Host,
		row.Path,
		strconv.Itoa(row.StatusCode),
		row.CacheStatus,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func nonNegativeRollupInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
