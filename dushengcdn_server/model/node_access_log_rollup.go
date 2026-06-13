package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/sharding"
)

const (
	nodeAccessLogRollupBucketMinutes       = 1
	nodeAccessLogDerivedRollupRefreshDelay = 2 * time.Second
	nodeAccessLogDerivedRollupRetryDelay   = 10 * time.Second
)

const (
	nodeAccessLogBucketIdentityKindIP   = "ip"
	nodeAccessLogBucketIdentityKindHost = "host"
)

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

type NodeAccessLogBucketRollup struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	BucketStartedAt  time.Time `json:"bucket_started_at" gorm:"uniqueIndex;index"`
	RequestCount     int64     `json:"request_count"`
	UniqueIPCount    int64     `json:"unique_ip_count"`
	UniqueHostCount  int64     `json:"unique_host_count"`
	SuccessCount     int64     `json:"success_count"`
	ClientErrorCount int64     `json:"client_error_count"`
	ServerErrorCount int64     `json:"server_error_count"`
	LastSeenAt       time.Time `json:"last_seen_at" gorm:"index"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type NodeAccessLogBucketIdentityRollup struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	BucketStartedAt time.Time `json:"bucket_started_at" gorm:"index;uniqueIndex:idx_node_access_log_bucket_identity,priority:1"`
	IdentityKind    string    `json:"identity_kind" gorm:"size:16;index;uniqueIndex:idx_node_access_log_bucket_identity,priority:2"`
	IdentityValue   string    `json:"identity_value" gorm:"size:255;index;uniqueIndex:idx_node_access_log_bucket_identity,priority:3"`
	CreatedAt       time.Time `json:"created_at"`
}

type NodeAccessLogBucketFilterIdentityRollup struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	BucketStartedAt time.Time `json:"bucket_started_at" gorm:"index;uniqueIndex:idx_node_access_log_bucket_filter_identity,priority:1"`
	DimensionHash   string    `json:"dimension_hash" gorm:"size:64;uniqueIndex:idx_node_access_log_bucket_filter_identity,priority:2"`
	NodeID          string    `json:"node_id" gorm:"index;size:64"`
	RemoteAddr      string    `json:"remote_addr" gorm:"index;size:128"`
	Host            string    `json:"host" gorm:"index;size:255"`
	Path            string    `json:"path" gorm:"type:text"`
	IdentityKind    string    `json:"identity_kind" gorm:"size:16;index"`
	IdentityValue   string    `json:"identity_value" gorm:"size:255;index"`
	CreatedAt       time.Time `json:"created_at"`
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

type nodeAccessLogBucketRollupAggregateRow struct {
	BucketStartedAt  time.Time
	RequestCount     int64
	UniqueIPCount    int64
	UniqueHostCount  int64
	SuccessCount     int64
	ClientErrorCount int64
	ServerErrorCount int64
	LastSeenEpoch    int64
}

var nodeAccessLogDerivedRollupRefreshState = struct {
	sync.Mutex
	pending map[int64]time.Time
	timer   *time.Timer
	running bool
}{}

func (log *NodeAccessLog) AfterCreate(tx *gorm.DB) error {
	if log == nil {
		return nil
	}
	if err := upsertNodeAccessLogRollups(tx, []*NodeAccessLog{log}); err != nil {
		slog.Warn("access log rollup update failed after raw log create", "node_id", log.NodeID, "error", err)
	}
	return nil
}

func ensureNodeAccessLogRollupSchema(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	if err := db.AutoMigrate(
		&NodeAccessLogRollup{},
		&NodeAccessLogBucketRollup{},
		&NodeAccessLogBucketIdentityRollup{},
		&NodeAccessLogBucketFilterIdentityRollup{},
	); err != nil {
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
		return backfillNodeAccessLogDerivedRollupsIfEmpty(db)
	}
	return RebuildNodeAccessLogRollups(db)
}

func backfillNodeAccessLogDerivedRollupsIfEmpty(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !nodeAccessLogBucketRollupTablesExist(db) {
		return nil
	}
	for _, item := range []struct {
		name  string
		model any
	}{
		{name: "node access log bucket rollups", model: &NodeAccessLogBucketRollup{}},
		{name: "node access log bucket identity rollups", model: &NodeAccessLogBucketIdentityRollup{}},
		{name: "node access log bucket filter identity rollups", model: &NodeAccessLogBucketFilterIdentityRollup{}},
	} {
		if !db.Migrator().HasTable(item.model) {
			continue
		}
		var count int64
		if err := nodeAccessLogRollupSession(db).Model(item.model).Count(&count).Error; err != nil {
			return fmt.Errorf("count %s failed: %w", item.name, err)
		}
		if count == 0 {
			return RebuildNodeAccessLogBucketRollups(db)
		}
	}
	return nil
}

func RebuildNodeAccessLogRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	if err := nodeAccessLogRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogRollup{}).Error; err != nil {
		return fmt.Errorf("clear node access log rollups failed: %w", err)
	}
	if err := clearNodeAccessLogBucketRollups(db); err != nil {
		return err
	}
	for _, table := range observabilityShardTables("node_access_logs") {
		if !db.Migrator().HasTable(table) {
			continue
		}
		rows, err := nodeAccessLogRollupRowsForShard(db, table)
		if err != nil {
			return err
		}
		if err := upsertNodeAccessLogRollupRowsWithRefresh(db, rows, false); err != nil {
			return err
		}
	}
	return RebuildNodeAccessLogBucketRollups(db)
}

func DeleteAllNodeAccessLogRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	if err := nodeAccessLogRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogRollup{}).Error; err != nil {
		return err
	}
	return clearNodeAccessLogBucketRollups(db)
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
	if err := upsertNodeAccessLogRollupRowsWithRefresh(db, rows, false); err != nil {
		return fmt.Errorf("upsert access log detailed rollups failed: %w", err)
	}
	enqueueNodeAccessLogDerivedRollupRefresh(db, rows)
	return nil
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
	return upsertNodeAccessLogRollupRowsWithRefresh(db, rows, true)
}

func upsertNodeAccessLogRollupRowsWithRefresh(db *gorm.DB, rows []*NodeAccessLogRollup, refreshBuckets bool) error {
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
	if err := nodeAccessLogRollupTableDB(db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bucket_started_at"}, {Name: "dimension_hash"}},
		DoUpdates: updates,
	}).CreateInBatches(rows, 500).Error; err != nil {
		return err
	}
	if refreshBuckets {
		return refreshNodeAccessLogBucketRollupsForRows(db, rows)
	}
	return nil
}

func enqueueNodeAccessLogDerivedRollupRefresh(_ *gorm.DB, rows []*NodeAccessLogRollup) {
	buckets := nodeAccessLogDerivedRollupBuckets(rows)
	if len(buckets) == 0 {
		return
	}
	if DB == nil {
		return
	}
	nodeAccessLogDerivedRollupRefreshState.Lock()
	defer nodeAccessLogDerivedRollupRefreshState.Unlock()
	if nodeAccessLogDerivedRollupRefreshState.pending == nil {
		nodeAccessLogDerivedRollupRefreshState.pending = make(map[int64]time.Time)
	}
	for _, bucket := range buckets {
		nodeAccessLogDerivedRollupRefreshState.pending[bucket.Unix()] = bucket
	}
	if nodeAccessLogDerivedRollupRefreshState.timer == nil && !nodeAccessLogDerivedRollupRefreshState.running {
		nodeAccessLogDerivedRollupRefreshState.timer = time.AfterFunc(nodeAccessLogDerivedRollupRefreshDelay, runNodeAccessLogDerivedRollupRefresh)
	}
}

func nodeAccessLogDerivedRollupBuckets(rows []*NodeAccessLogRollup) []time.Time {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		if row == nil || row.BucketStartedAt.IsZero() {
			continue
		}
		bucket := nodeAccessLogRollupBucketStart(row.BucketStartedAt)
		seen[bucket.Unix()] = bucket
	}
	if len(seen) == 0 {
		return nil
	}
	buckets := make([]time.Time, 0, len(seen))
	for _, bucket := range seen {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Before(buckets[j])
	})
	return buckets
}

func runNodeAccessLogDerivedRollupRefresh() {
	buckets := takeNodeAccessLogDerivedRollupRefreshBatch()
	if len(buckets) == 0 {
		return
	}
	err := refreshNodeAccessLogBucketRollupsForBuckets(DB, buckets)
	finishNodeAccessLogDerivedRollupRefresh(buckets, err)
	if err != nil {
		slog.Warn("refresh node access log derived rollups failed", "bucket_count", len(buckets), "error", err)
	}
}

func takeNodeAccessLogDerivedRollupRefreshBatch() []time.Time {
	nodeAccessLogDerivedRollupRefreshState.Lock()
	defer nodeAccessLogDerivedRollupRefreshState.Unlock()
	if nodeAccessLogDerivedRollupRefreshState.running || len(nodeAccessLogDerivedRollupRefreshState.pending) == 0 {
		return nil
	}
	if nodeAccessLogDerivedRollupRefreshState.timer != nil {
		nodeAccessLogDerivedRollupRefreshState.timer.Stop()
		nodeAccessLogDerivedRollupRefreshState.timer = nil
	}
	buckets := make([]time.Time, 0, len(nodeAccessLogDerivedRollupRefreshState.pending))
	for _, bucket := range nodeAccessLogDerivedRollupRefreshState.pending {
		buckets = append(buckets, bucket)
	}
	nodeAccessLogDerivedRollupRefreshState.pending = make(map[int64]time.Time)
	nodeAccessLogDerivedRollupRefreshState.running = true
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Before(buckets[j])
	})
	return buckets
}

func finishNodeAccessLogDerivedRollupRefresh(buckets []time.Time, refreshErr error) {
	nodeAccessLogDerivedRollupRefreshState.Lock()
	defer nodeAccessLogDerivedRollupRefreshState.Unlock()
	nodeAccessLogDerivedRollupRefreshState.running = false
	delay := nodeAccessLogDerivedRollupRefreshDelay
	if refreshErr != nil {
		if nodeAccessLogDerivedRollupRefreshState.pending == nil {
			nodeAccessLogDerivedRollupRefreshState.pending = make(map[int64]time.Time)
		}
		for _, bucket := range buckets {
			if bucket.IsZero() {
				continue
			}
			nodeAccessLogDerivedRollupRefreshState.pending[bucket.Unix()] = bucket
		}
		delay = nodeAccessLogDerivedRollupRetryDelay
	}
	if len(nodeAccessLogDerivedRollupRefreshState.pending) > 0 {
		nodeAccessLogDerivedRollupRefreshState.timer = time.AfterFunc(delay, runNodeAccessLogDerivedRollupRefresh)
		return
	}
	nodeAccessLogDerivedRollupRefreshState.timer = nil
}

func flushNodeAccessLogDerivedRollups() error {
	for {
		for {
			nodeAccessLogDerivedRollupRefreshState.Lock()
			running := nodeAccessLogDerivedRollupRefreshState.running
			nodeAccessLogDerivedRollupRefreshState.Unlock()
			if !running {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		buckets := takeNodeAccessLogDerivedRollupRefreshBatch()
		if len(buckets) == 0 {
			return nil
		}
		err := refreshNodeAccessLogBucketRollupsForBuckets(DB, buckets)
		finishNodeAccessLogDerivedRollupRefresh(buckets, err)
		if err != nil {
			return err
		}
	}
}

func resetNodeAccessLogDerivedRollupRefresh() {
	nodeAccessLogDerivedRollupRefreshState.Lock()
	defer nodeAccessLogDerivedRollupRefreshState.Unlock()
	if nodeAccessLogDerivedRollupRefreshState.timer != nil {
		nodeAccessLogDerivedRollupRefreshState.timer.Stop()
	}
	nodeAccessLogDerivedRollupRefreshState.pending = nil
	nodeAccessLogDerivedRollupRefreshState.timer = nil
	nodeAccessLogDerivedRollupRefreshState.running = false
}

func clearNodeAccessLogBucketRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	if db.Migrator().HasTable(&NodeAccessLogBucketIdentityRollup{}) {
		if err := nodeAccessLogBucketIdentityRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogBucketIdentityRollup{}).Error; err != nil {
			return fmt.Errorf("clear node access log bucket identity rollups failed: %w", err)
		}
	}
	if db.Migrator().HasTable(&NodeAccessLogBucketFilterIdentityRollup{}) {
		if err := nodeAccessLogBucketFilterIdentityRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogBucketFilterIdentityRollup{}).Error; err != nil {
			return fmt.Errorf("clear node access log bucket filter identity rollups failed: %w", err)
		}
	}
	if db.Migrator().HasTable(&NodeAccessLogBucketRollup{}) {
		if err := nodeAccessLogBucketRollupTableDB(db).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&NodeAccessLogBucketRollup{}).Error; err != nil {
			return fmt.Errorf("clear node access log bucket rollups failed: %w", err)
		}
	}
	return nil
}

func RebuildNodeAccessLogBucketRollups(db *gorm.DB) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || !nodeAccessLogBucketRollupTablesExist(db) || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	if err := clearNodeAccessLogBucketRollups(db); err != nil {
		return err
	}
	var buckets []time.Time
	if err := nodeAccessLogRollupTableDB(db).
		Distinct("bucket_started_at").
		Order("bucket_started_at asc").
		Pluck("bucket_started_at", &buckets).Error; err != nil {
		return fmt.Errorf("query node access log rollup buckets failed: %w", err)
	}
	const batchSize = 500
	for start := 0; start < len(buckets); start += batchSize {
		end := start + batchSize
		if end > len(buckets) {
			end = len(buckets)
		}
		if err := refreshNodeAccessLogBucketRollupsForBuckets(db, buckets[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func refreshNodeAccessLogBucketRollupsForRows(db *gorm.DB, rows []*NodeAccessLogRollup) error {
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[int64]time.Time, len(rows))
	for _, row := range rows {
		if row == nil || row.BucketStartedAt.IsZero() {
			continue
		}
		bucket := nodeAccessLogRollupBucketStart(row.BucketStartedAt)
		seen[bucket.Unix()] = bucket
	}
	if len(seen) == 0 {
		return nil
	}
	buckets := make([]time.Time, 0, len(seen))
	for _, bucket := range seen {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Before(buckets[j])
	})
	return refreshNodeAccessLogBucketRollupsForBuckets(db, buckets)
}

func refreshNodeAccessLogBucketRollupsForBuckets(db *gorm.DB, buckets []time.Time) error {
	db = nodeAccessLogRollupSession(db)
	if db == nil || len(buckets) == 0 || !nodeAccessLogBucketRollupTablesExist(db) || !db.Migrator().HasTable(&NodeAccessLogRollup{}) {
		return nil
	}
	if err := nodeAccessLogBucketIdentityRollupTableDB(db).
		Where("bucket_started_at IN ?", buckets).
		Delete(&NodeAccessLogBucketIdentityRollup{}).Error; err != nil {
		return fmt.Errorf("clear changed node access log bucket identity rollups failed: %w", err)
	}
	if nodeAccessLogBucketFilterIdentityRollupTableExists(db) {
		if err := nodeAccessLogBucketFilterIdentityRollupTableDB(db).
			Where("bucket_started_at IN ?", buckets).
			Delete(&NodeAccessLogBucketFilterIdentityRollup{}).Error; err != nil {
			return fmt.Errorf("clear changed node access log bucket filter identity rollups failed: %w", err)
		}
	}
	if err := nodeAccessLogBucketRollupTableDB(db).
		Where("bucket_started_at IN ?", buckets).
		Delete(&NodeAccessLogBucketRollup{}).Error; err != nil {
		return fmt.Errorf("clear changed node access log bucket rollups failed: %w", err)
	}
	bucketRows, err := nodeAccessLogBucketRollupRowsForBuckets(db, buckets)
	if err != nil {
		return err
	}
	if len(bucketRows) > 0 {
		if err := nodeAccessLogBucketRollupTableDB(db).CreateInBatches(bucketRows, 500).Error; err != nil {
			return fmt.Errorf("insert node access log bucket rollups failed: %w", err)
		}
	}
	identityRows, err := nodeAccessLogBucketIdentityRollupRowsForBuckets(db, buckets)
	if err != nil {
		return err
	}
	if len(identityRows) > 0 {
		if err := nodeAccessLogBucketIdentityRollupTableDB(db).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(identityRows, 500).Error; err != nil {
			return fmt.Errorf("insert node access log bucket identity rollups failed: %w", err)
		}
	}
	if nodeAccessLogBucketFilterIdentityRollupTableExists(db) {
		filterIdentityRows, err := nodeAccessLogBucketFilterIdentityRollupRowsForBuckets(db, buckets)
		if err != nil {
			return err
		}
		if len(filterIdentityRows) > 0 {
			if err := nodeAccessLogBucketFilterIdentityRollupTableDB(db).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(filterIdentityRows, 500).Error; err != nil {
				return fmt.Errorf("insert node access log bucket filter identity rollups failed: %w", err)
			}
		}
	}
	return nil
}

func nodeAccessLogBucketRollupRowsForBuckets(db *gorm.DB, buckets []time.Time) ([]*NodeAccessLogBucketRollup, error) {
	lastSeenExpr := accessLogEpochExpr("MAX(last_seen_at)")
	var aggregateRows []nodeAccessLogBucketRollupAggregateRow
	err := nodeAccessLogRollupTableDB(db).
		Where("bucket_started_at IN ?", buckets).
		Select(
			"bucket_started_at, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COUNT(DISTINCT CASE WHEN remote_addr <> '' THEN remote_addr ELSE NULL END) AS unique_ip_count, " +
				"COUNT(DISTINCT CASE WHEN host <> '' THEN host ELSE NULL END) AS unique_host_count, " +
				"COALESCE(SUM(CASE WHEN status_code < 400 THEN request_count ELSE 0 END), 0) AS success_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN request_count ELSE 0 END), 0) AS client_error_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 500 THEN request_count ELSE 0 END), 0) AS server_error_count, " +
				lastSeenExpr + " AS last_seen_epoch",
		).
		Group("bucket_started_at").
		Scan(&aggregateRows).Error
	if err != nil {
		return nil, fmt.Errorf("aggregate node access log bucket rollups failed: %w", err)
	}
	rows := make([]*NodeAccessLogBucketRollup, 0, len(aggregateRows))
	for _, row := range aggregateRows {
		rows = append(rows, &NodeAccessLogBucketRollup{
			BucketStartedAt:  row.BucketStartedAt.UTC(),
			RequestCount:     row.RequestCount,
			UniqueIPCount:    row.UniqueIPCount,
			UniqueHostCount:  row.UniqueHostCount,
			SuccessCount:     row.SuccessCount,
			ClientErrorCount: row.ClientErrorCount,
			ServerErrorCount: row.ServerErrorCount,
			LastSeenAt:       time.Unix(row.LastSeenEpoch, 0).UTC(),
		})
	}
	return rows, nil
}

func nodeAccessLogBucketIdentityRollupRowsForBuckets(db *gorm.DB, buckets []time.Time) ([]*NodeAccessLogBucketIdentityRollup, error) {
	rows := []*NodeAccessLogBucketIdentityRollup{}
	for _, dimension := range []struct {
		kind   string
		column string
	}{
		{kind: nodeAccessLogBucketIdentityKindIP, column: "remote_addr"},
		{kind: nodeAccessLogBucketIdentityKindHost, column: "host"},
	} {
		var dimensionRows []*NodeAccessLogBucketIdentityRollup
		err := nodeAccessLogRollupTableDB(db).
			Where("bucket_started_at IN ?", buckets).
			Where(dimension.column+" <> ''").
			Select("bucket_started_at, ? AS identity_kind, "+dimension.column+" AS identity_value", dimension.kind).
			Group("bucket_started_at, " + dimension.column).
			Scan(&dimensionRows).Error
		if err != nil {
			return nil, fmt.Errorf("aggregate node access log bucket %s identities failed: %w", dimension.kind, err)
		}
		rows = append(rows, dimensionRows...)
	}
	return rows, nil
}

func nodeAccessLogBucketFilterIdentityRollupRowsForBuckets(db *gorm.DB, buckets []time.Time) ([]*NodeAccessLogBucketFilterIdentityRollup, error) {
	rows := []*NodeAccessLogBucketFilterIdentityRollup{}
	for _, dimension := range []struct {
		kind   string
		column string
	}{
		{kind: nodeAccessLogBucketIdentityKindIP, column: "remote_addr"},
		{kind: nodeAccessLogBucketIdentityKindHost, column: "host"},
	} {
		var dimensionRows []*NodeAccessLogBucketFilterIdentityRollup
		err := nodeAccessLogRollupTableDB(db).
			Where("bucket_started_at IN ?", buckets).
			Where(dimension.column+" <> ''").
			Select(
				"bucket_started_at, node_id, remote_addr, host, path, "+
					"? AS identity_kind, "+dimension.column+" AS identity_value",
				dimension.kind,
			).
			Group("bucket_started_at, node_id, remote_addr, host, path, " + dimension.column).
			Scan(&dimensionRows).Error
		if err != nil {
			return nil, fmt.Errorf("aggregate node access log bucket filter %s identities failed: %w", dimension.kind, err)
		}
		for _, row := range dimensionRows {
			if row == nil {
				continue
			}
			row.BucketStartedAt = nodeAccessLogRollupBucketStart(row.BucketStartedAt)
			row.NodeID = strings.TrimSpace(row.NodeID)
			row.RemoteAddr = strings.TrimSpace(row.RemoteAddr)
			row.Host = strings.TrimSpace(row.Host)
			row.Path = strings.TrimSpace(row.Path)
			row.IdentityKind = strings.TrimSpace(row.IdentityKind)
			row.IdentityValue = strings.TrimSpace(row.IdentityValue)
			row.DimensionHash = nodeAccessLogBucketFilterIdentityRollupDimensionHash(row)
		}
		rows = append(rows, dimensionRows...)
	}
	return rows, nil
}

func countNodeAccessLogsFromRollups(query NodeAccessLogQuery) (int64, int64, bool, error) {
	if totalRecords, totalIPs, ok, err := countNodeAccessLogsFromBucketRollups(query); ok || err != nil {
		return totalRecords, totalIPs, ok, err
	}
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return 0, 0, false, nil
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return countNodeAccessLogsFromPartialRawAndRollups(db, query)
	}
	base := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), query)
	var totalRecords int64
	if err := base.Select("COALESCE(SUM(request_count), 0)").Scan(&totalRecords).Error; err != nil {
		return 0, 0, false, fmt.Errorf("count access log records from rollups failed: %w", err)
	}
	if totalIPs, ok, err := countNodeAccessLogIdentitiesFromFilterRollups(query, nodeAccessLogBucketIdentityKindIP); ok || err != nil {
		return totalRecords, totalIPs, ok, err
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

func countNodeAccessLogsFromPartialRawAndRollups(db *gorm.DB, query NodeAccessLogQuery) (int64, int64, bool, error) {
	remainderSince, ok := nodeAccessLogRollupRemainderSince(query.Since)
	if !ok || remainderSince.IsZero() || !remainderSince.After(query.Since) {
		return 0, 0, false, nil
	}
	remainderQuery := query
	remainderQuery.Since = remainderSince
	remainderQuery.Before = time.Time{}
	if !nodeAccessLogBucketFilterIdentityRollupsCanServe(db, remainderQuery) {
		return 0, 0, false, nil
	}
	partialQuery := query
	partialQuery.Before = remainderSince
	partialRecords, err := countNodeAccessLogRawRecords(partialQuery)
	if err != nil {
		return 0, 0, false, err
	}
	remainderRecords, err := countNodeAccessLogRollupRecords(db, remainderQuery)
	if err != nil {
		return 0, 0, false, err
	}
	totalIPs, ok, err := countNodeAccessLogIdentitiesFromPartialRawAndFilterRollups(partialQuery, remainderQuery, nodeAccessLogBucketIdentityKindIP)
	if !ok || err != nil {
		return 0, 0, ok, err
	}
	return partialRecords + remainderRecords, totalIPs, true, nil
}

func countNodeAccessLogRollupRecords(db *gorm.DB, query NodeAccessLogQuery) (int64, error) {
	var totalRecords int64
	if err := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), query).
		Select("COALESCE(SUM(request_count), 0)").
		Scan(&totalRecords).Error; err != nil {
		return 0, fmt.Errorf("count access log records from rollups failed: %w", err)
	}
	return totalRecords, nil
}

func countNodeAccessLogRawRecords(query NodeAccessLogQuery) (int64, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	branches, args := buildNodeAccessLogUnionBranches(query, "COUNT(*) AS total_records")
	var row struct {
		TotalRecords int64
	}
	sql := "WITH access_log_counts AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT COALESCE(SUM(total_records), 0) AS total_records FROM access_log_counts"
	if err := db.Raw(sql, args...).Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("count access log records across shards failed: %w", err)
	}
	return row.TotalRecords, nil
}

func listNodeAccessLogBucketsFromRollups(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, bool, error) {
	if rows, ok, err := listNodeAccessLogBucketsFromBucketRollups(query); ok || err != nil {
		return rows, ok, err
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
		Path:       query.Path,
		Since:      query.Since,
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.FoldMinutes)
	if nodeAccessLogBucketFilterIdentityRollupsCanServe(db, modelQuery) {
		return listNodeAccessLogBucketsFromRollupsWithFilterIdentities(db, query, modelQuery, bucketExpr)
	}
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

func listNodeAccessLogBucketsFromRollupsWithFilterIdentities(db *gorm.DB, query NodeAccessLogBucketQuery, modelQuery NodeAccessLogQuery, bucketExpr string) ([]*NodeAccessLogBucketRow, bool, error) {
	bucketCounts := applyNodeAccessLogRollupFilters(nodeAccessLogRollupTableDB(db), modelQuery).
		Select(
			bucketExpr + " AS bucket_epoch, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(SUM(CASE WHEN status_code < 400 THEN request_count ELSE 0 END), 0) AS success_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN request_count ELSE 0 END), 0) AS client_error_count, " +
				"COALESCE(SUM(CASE WHEN status_code >= 500 THEN request_count ELSE 0 END), 0) AS server_error_count",
		).
		Group("bucket_epoch")
	ipCounts := nodeAccessLogFilterIdentityCountsByFold(db, modelQuery, query.FoldMinutes, nodeAccessLogBucketIdentityKindIP, "unique_ip_count")
	hostCounts := nodeAccessLogFilterIdentityCountsByFold(db, modelQuery, query.FoldMinutes, nodeAccessLogBucketIdentityKindHost, "unique_host_count")
	rollupQuery := nodeAccessLogRollupSession(db).
		Table("(?) AS bucket_counts", bucketCounts).
		Joins("LEFT JOIN (?) AS ip_counts ON ip_counts.bucket_epoch = bucket_counts.bucket_epoch", ipCounts).
		Joins("LEFT JOIN (?) AS host_counts ON host_counts.bucket_epoch = bucket_counts.bucket_epoch", hostCounts).
		Select(
			"bucket_counts.bucket_epoch, " +
				"bucket_counts.request_count, " +
				"COALESCE(ip_counts.unique_ip_count, 0) AS unique_ip_count, " +
				"COALESCE(host_counts.unique_host_count, 0) AS unique_host_count, " +
				"bucket_counts.success_count, " +
				"bucket_counts.client_error_count, " +
				"bucket_counts.server_error_count",
		).
		Order(buildNodeAccessLogBucketRollupSortClause(query.SortBy, query.SortOrder))
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		rollupQuery = rollupQuery.Limit(query.PageSize + max(query.Lookahead, 0)).Offset(page * query.PageSize)
	}
	var rows []*NodeAccessLogBucketRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log buckets from filter identity rollups failed: %w", err)
	}
	return rows, true, nil
}

func countNodeAccessLogBucketsFromRollups(query NodeAccessLogBucketQuery) (int64, bool, error) {
	if total, ok, err := countNodeAccessLogBucketsFromBucketRollups(query); ok || err != nil {
		return total, ok, err
	}
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

func countNodeAccessLogsFromBucketRollups(query NodeAccessLogQuery) (int64, int64, bool, error) {
	if !nodeAccessLogBucketRollupsCanServe(NodeAccessLogBucketQuery{
		NodeID:     query.NodeID,
		RemoteAddr: query.RemoteAddr,
		Host:       query.Host,
		Path:       query.Path,
		Since:      query.Since,
	}) {
		return 0, 0, false, nil
	}
	db, ok := nodeAccessLogBucketRollupQueryDB()
	if !ok {
		return 0, 0, false, nil
	}
	bucketBase := applyNodeAccessLogBucketRollupSince(nodeAccessLogBucketRollupTableDB(db), query.Since)
	var totalRecords int64
	if err := bucketBase.Select("COALESCE(SUM(request_count), 0)").Scan(&totalRecords).Error; err != nil {
		return 0, 0, false, fmt.Errorf("count access log records from bucket rollups failed: %w", err)
	}
	ipMembers := applyNodeAccessLogBucketIdentityRollupSince(
		nodeAccessLogBucketIdentityRollupTableDB(db).Where("identity_kind = ?", nodeAccessLogBucketIdentityKindIP),
		query.Since,
	).Select("identity_value").Group("identity_value")
	var totalIPs int64
	if err := nodeAccessLogRollupSession(db).Table("(?) AS unique_ip_members", ipMembers).Count(&totalIPs).Error; err != nil {
		return 0, 0, false, fmt.Errorf("count access log ips from bucket rollups failed: %w", err)
	}
	return totalRecords, totalIPs, true, nil
}

func listNodeAccessLogBucketsFromBucketRollups(query NodeAccessLogBucketQuery) ([]*NodeAccessLogBucketRow, bool, error) {
	if !nodeAccessLogBucketRollupsCanServe(query) {
		return nil, false, nil
	}
	db, ok := nodeAccessLogBucketRollupQueryDB()
	if !ok {
		return nil, false, nil
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.FoldMinutes)
	bucketCounts := applyNodeAccessLogBucketRollupSince(nodeAccessLogBucketRollupTableDB(db), query.Since).
		Select(
			bucketExpr + " AS bucket_epoch, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(SUM(success_count), 0) AS success_count, " +
				"COALESCE(SUM(client_error_count), 0) AS client_error_count, " +
				"COALESCE(SUM(server_error_count), 0) AS server_error_count",
		).
		Group("bucket_epoch")
	ipCounts := nodeAccessLogBucketIdentityCountsByFold(db, query.Since, query.FoldMinutes, nodeAccessLogBucketIdentityKindIP, "unique_ip_count")
	hostCounts := nodeAccessLogBucketIdentityCountsByFold(db, query.Since, query.FoldMinutes, nodeAccessLogBucketIdentityKindHost, "unique_host_count")
	rollupQuery := nodeAccessLogRollupSession(db).
		Table("(?) AS bucket_counts", bucketCounts).
		Joins("LEFT JOIN (?) AS ip_counts ON ip_counts.bucket_epoch = bucket_counts.bucket_epoch", ipCounts).
		Joins("LEFT JOIN (?) AS host_counts ON host_counts.bucket_epoch = bucket_counts.bucket_epoch", hostCounts).
		Select(
			"bucket_counts.bucket_epoch, " +
				"bucket_counts.request_count, " +
				"COALESCE(ip_counts.unique_ip_count, 0) AS unique_ip_count, " +
				"COALESCE(host_counts.unique_host_count, 0) AS unique_host_count, " +
				"bucket_counts.success_count, " +
				"bucket_counts.client_error_count, " +
				"bucket_counts.server_error_count",
		).
		Order(buildNodeAccessLogBucketRollupSortClause(query.SortBy, query.SortOrder))
	if query.PageSize > 0 {
		page := query.Page
		if page < 0 {
			page = 0
		}
		rollupQuery = rollupQuery.Limit(query.PageSize + max(query.Lookahead, 0)).Offset(page * query.PageSize)
	}
	var rows []*NodeAccessLogBucketRow
	if err := rollupQuery.Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("query access log buckets from bucket rollups failed: %w", err)
	}
	return rows, true, nil
}

func countNodeAccessLogBucketsFromBucketRollups(query NodeAccessLogBucketQuery) (int64, bool, error) {
	if !nodeAccessLogBucketRollupsCanServe(query) {
		return 0, false, nil
	}
	db, ok := nodeAccessLogBucketRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", query.FoldMinutes)
	grouped := applyNodeAccessLogBucketRollupSince(nodeAccessLogBucketRollupTableDB(db), query.Since).
		Select(bucketExpr + " AS bucket_epoch").
		Group("bucket_epoch")
	var total int64
	if err := nodeAccessLogRollupSession(db).Table("(?) AS grouped_bucket_rows", grouped).Count(&total).Error; err != nil {
		return 0, false, fmt.Errorf("count access log buckets from bucket rollups failed: %w", err)
	}
	return total, true, nil
}

func nodeAccessLogBucketIdentityCountsByFold(db *gorm.DB, since time.Time, foldMinutes int, identityKind string, alias string) *gorm.DB {
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", foldMinutes)
	members := applyNodeAccessLogBucketIdentityRollupSince(
		nodeAccessLogBucketIdentityRollupTableDB(db).Where("identity_kind = ?", identityKind),
		since,
	).Select(bucketExpr + " AS bucket_epoch, identity_value").Group("bucket_epoch, identity_value")
	return nodeAccessLogRollupSession(db).
		Table("(?) AS bucket_identity_members", members).
		Select("bucket_epoch, COUNT(*) AS " + alias).
		Group("bucket_epoch")
}

func nodeAccessLogFilterIdentityCountsByFold(db *gorm.DB, query NodeAccessLogQuery, foldMinutes int, identityKind string, alias string) *gorm.DB {
	bucketExpr := accessLogBucketEpochExprForColumn("bucket_started_at", foldMinutes)
	members := applyNodeAccessLogFilterIdentityRollupFilters(
		nodeAccessLogBucketFilterIdentityRollupTableDB(db),
		query,
		identityKind,
	).Select(bucketExpr + " AS bucket_epoch, identity_value").Group("bucket_epoch, identity_value")
	return nodeAccessLogRollupSession(db).
		Table("(?) AS bucket_filter_identity_members", members).
		Select("bucket_epoch, COUNT(*) AS " + alias).
		Group("bucket_epoch")
}

func buildNodeAccessLogBucketRollupSortClause(sortBy string, sortOrder string) string {
	order := normalizeSortOrder(sortOrder)
	switch strings.TrimSpace(sortBy) {
	case "request_count":
		return fmt.Sprintf("bucket_counts.request_count %s, bucket_counts.bucket_epoch %s", order, order)
	default:
		return fmt.Sprintf("bucket_counts.bucket_epoch %s", order)
	}
}

func countNodeAccessLogIPSummariesFromRollups(query NodeAccessLogIPSummaryQuery) (int64, bool, error) {
	db, ok := nodeAccessLogRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	modelQuery := nodeAccessLogQueryFromIPSummaryQuery(query)
	if total, ok, err := countNodeAccessLogIdentitiesFromFilterRollups(modelQuery, nodeAccessLogBucketIdentityKindIP); ok || err != nil {
		return total, ok, err
	}
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return 0, false, nil
	}
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

func nodeAccessLogBucketRollupQueryDB() (*gorm.DB, bool) {
	if !nodeAccessLogDerivedRollupRefreshIdle() {
		return nil, false
	}
	db := nodeAccessLogRollupSession(DB)
	if db == nil || !nodeAccessLogBucketRollupTablesExist(db) {
		return nil, false
	}
	var count int64
	if err := nodeAccessLogBucketRollupTableDB(db).Limit(1).Count(&count).Error; err != nil || count == 0 {
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

func nodeAccessLogBucketRollupTableDB(db *gorm.DB) *gorm.DB {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	return db.Table((NodeAccessLogBucketRollup{}).TableName())
}

func nodeAccessLogBucketIdentityRollupTableDB(db *gorm.DB) *gorm.DB {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	return db.Table((NodeAccessLogBucketIdentityRollup{}).TableName())
}

func nodeAccessLogBucketFilterIdentityRollupTableDB(db *gorm.DB) *gorm.DB {
	db = nodeAccessLogRollupSession(db)
	if db == nil {
		return nil
	}
	return db.Table((NodeAccessLogBucketFilterIdentityRollup{}).TableName())
}

func nodeAccessLogBucketRollupTablesExist(db *gorm.DB) bool {
	db = nodeAccessLogRollupSession(db)
	return db != nil &&
		db.Migrator().HasTable(&NodeAccessLogBucketRollup{}) &&
		db.Migrator().HasTable(&NodeAccessLogBucketIdentityRollup{})
}

func nodeAccessLogBucketFilterIdentityRollupTableExists(db *gorm.DB) bool {
	db = nodeAccessLogRollupSession(db)
	return db != nil && db.Migrator().HasTable(&NodeAccessLogBucketFilterIdentityRollup{})
}

func nodeAccessLogBucketFilterIdentityRollupQueryDB() (*gorm.DB, bool) {
	if !nodeAccessLogDerivedRollupRefreshIdle() {
		return nil, false
	}
	db := nodeAccessLogRollupSession(DB)
	if db == nil || !nodeAccessLogBucketFilterIdentityRollupTableExists(db) {
		return nil, false
	}
	var count int64
	if err := nodeAccessLogBucketFilterIdentityRollupTableDB(db).Limit(1).Count(&count).Error; err != nil || count == 0 {
		return nil, false
	}
	return db, true
}

func nodeAccessLogRollupCanServeSince(since time.Time) bool {
	return since.IsZero() || since.Equal(nodeAccessLogRollupBucketStart(since))
}

func nodeAccessLogRollupRemainderSince(since time.Time) (time.Time, bool) {
	if since.IsZero() {
		return time.Time{}, false
	}
	if nodeAccessLogRollupCanServeSince(since) {
		return since, true
	}
	remainderSince := nodeAccessLogRollupBucketStart(since).Add(time.Duration(nodeAccessLogRollupBucketMinutes) * time.Minute)
	if !remainderSince.After(since) {
		return time.Time{}, false
	}
	return remainderSince, true
}

func nodeAccessLogBucketRollupsCanServe(query NodeAccessLogBucketQuery) bool {
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return false
	}
	return strings.TrimSpace(query.NodeID) == "" &&
		strings.TrimSpace(query.RemoteAddr) == "" &&
		strings.TrimSpace(query.Host) == "" &&
		strings.TrimSpace(query.Path) == ""
}

func applyNodeAccessLogBucketRollupSince(db *gorm.DB, since time.Time) *gorm.DB {
	if db == nil || since.IsZero() {
		return db
	}
	return db.Where("bucket_started_at >= ?", nodeAccessLogRollupBucketStart(since))
}

func applyNodeAccessLogBucketIdentityRollupSince(db *gorm.DB, since time.Time) *gorm.DB {
	if db == nil || since.IsZero() {
		return db
	}
	return db.Where("bucket_started_at >= ?", nodeAccessLogRollupBucketStart(since))
}

func nodeAccessLogBucketFilterIdentityRollupsCanServe(db *gorm.DB, query NodeAccessLogQuery) bool {
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		return false
	}
	if !nodeAccessLogDerivedRollupRefreshIdle() {
		return false
	}
	db = nodeAccessLogRollupSession(db)
	if db == nil || !nodeAccessLogBucketFilterIdentityRollupTableExists(db) {
		return false
	}
	var count int64
	err := nodeAccessLogBucketFilterIdentityRollupTableDB(db).Limit(1).Count(&count).Error
	return err == nil && count > 0
}

func nodeAccessLogDerivedRollupRefreshIdle() bool {
	nodeAccessLogDerivedRollupRefreshState.Lock()
	defer nodeAccessLogDerivedRollupRefreshState.Unlock()
	return !nodeAccessLogDerivedRollupRefreshState.running && len(nodeAccessLogDerivedRollupRefreshState.pending) == 0
}

func countNodeAccessLogIdentitiesFromFilterRollups(query NodeAccessLogQuery, identityKind string) (int64, bool, error) {
	if !nodeAccessLogRollupCanServeSince(query.Since) {
		remainderSince, ok := nodeAccessLogRollupRemainderSince(query.Since)
		if !ok || remainderSince.IsZero() || !remainderSince.After(query.Since) {
			return 0, false, nil
		}
		partialQuery := query
		partialQuery.Before = remainderSince
		remainderQuery := query
		remainderQuery.Since = remainderSince
		remainderQuery.Before = time.Time{}
		return countNodeAccessLogIdentitiesFromPartialRawAndFilterRollups(partialQuery, remainderQuery, identityKind)
	}
	db, ok := nodeAccessLogBucketFilterIdentityRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	members := applyNodeAccessLogFilterIdentityRollupFilters(
		nodeAccessLogBucketFilterIdentityRollupTableDB(db),
		query,
		identityKind,
	).Select("identity_value").Group("identity_value")
	var total int64
	if err := nodeAccessLogRollupSession(db).Table("(?) AS unique_filter_identity_members", members).Count(&total).Error; err != nil {
		return 0, false, fmt.Errorf("count access log %s identities from filter rollups failed: %w", identityKind, err)
	}
	return total, true, nil
}

func countNodeAccessLogIdentitiesFromPartialRawAndFilterRollups(partialQuery NodeAccessLogQuery, remainderQuery NodeAccessLogQuery, identityKind string) (int64, bool, error) {
	remainderTotal, ok, err := countNodeAccessLogIdentitiesFromFilterRollups(remainderQuery, identityKind)
	if !ok || err != nil {
		return 0, ok, err
	}
	partialIdentities, err := listNodeAccessLogRawIdentityValues(partialQuery, identityKind)
	if err != nil {
		return 0, false, err
	}
	if len(partialIdentities) == 0 {
		return remainderTotal, true, nil
	}
	db, ok := nodeAccessLogBucketFilterIdentityRollupQueryDB()
	if !ok {
		return 0, false, nil
	}
	members := applyNodeAccessLogFilterIdentityRollupFilters(
		nodeAccessLogBucketFilterIdentityRollupTableDB(db),
		remainderQuery,
		identityKind,
	).Where("identity_value IN ?", partialIdentities).
		Select("identity_value").
		Group("identity_value")
	var overlap int64
	if err := nodeAccessLogRollupSession(db).Table("(?) AS overlapping_filter_identity_members", members).Count(&overlap).Error; err != nil {
		return 0, false, fmt.Errorf("count overlapping access log %s identities from filter rollups failed: %w", identityKind, err)
	}
	total := remainderTotal + int64(len(partialIdentities)) - overlap
	if total < 0 {
		total = 0
	}
	return total, true, nil
}

func listNodeAccessLogRawIdentityValues(query NodeAccessLogQuery, identityKind string) ([]string, error) {
	column := ""
	switch strings.TrimSpace(identityKind) {
	case nodeAccessLogBucketIdentityKindIP:
		column = "remote_addr"
	case nodeAccessLogBucketIdentityKindHost:
		column = "host"
	default:
		return nil, fmt.Errorf("unsupported access log identity kind %q", identityKind)
	}
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches, args := buildNodeAccessLogUnionBranchesWithSuffix(
		query,
		column+" AS identity_value",
		"GROUP BY "+column,
		column+" <> ''",
	)
	sql := "WITH access_log_identity_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT identity_value FROM access_log_identity_rows WHERE identity_value <> '' GROUP BY identity_value"
	var rows []struct {
		IdentityValue string
	}
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query access log %s identities across shards failed: %w", identityKind, err)
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		if value := strings.TrimSpace(row.IdentityValue); value != "" {
			values = append(values, value)
		}
	}
	return values, nil
}

func applyNodeAccessLogFilterIdentityRollupFilters(db *gorm.DB, query NodeAccessLogQuery, identityKind string) *gorm.DB {
	if db == nil {
		return db
	}
	db = db.Where("identity_kind = ?", strings.TrimSpace(identityKind))
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
	return db.Where("identity_value <> ''")
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

func nodeAccessLogBucketFilterIdentityRollupDimensionHash(row *NodeAccessLogBucketFilterIdentityRollup) string {
	if row == nil {
		return ""
	}
	parts := []string{
		strconv.FormatInt(nodeAccessLogRollupBucketStart(row.BucketStartedAt).Unix(), 10),
		row.NodeID,
		row.RemoteAddr,
		row.Host,
		row.Path,
		row.IdentityKind,
		row.IdentityValue,
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
