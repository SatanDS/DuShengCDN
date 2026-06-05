package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type NodeMetricSnapshot struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	NodeID               string    `json:"node_id" gorm:"index;size:64;not null"`
	CapturedAt           time.Time `json:"captured_at" gorm:"index"`
	CPUUsagePercent      float64   `json:"cpu_usage_percent"`
	MemoryUsedBytes      int64     `json:"memory_used_bytes"`
	MemoryTotalBytes     int64     `json:"memory_total_bytes"`
	StorageUsedBytes     int64     `json:"storage_used_bytes"`
	StorageTotalBytes    int64     `json:"storage_total_bytes"`
	DiskReadBytes        int64     `json:"disk_read_bytes"`
	DiskWriteBytes       int64     `json:"disk_write_bytes"`
	NetworkRxBytes       int64     `json:"network_rx_bytes"`
	NetworkTxBytes       int64     `json:"network_tx_bytes"`
	OpenrestyRxBytes     int64     `json:"openresty_rx_bytes"`
	OpenrestyTxBytes     int64     `json:"openresty_tx_bytes"`
	OpenrestyConnections int64     `json:"openresty_connections"`
	CreatedAt            time.Time `json:"created_at"`
}

type NodeMetricSnapshotTrendBucket struct {
	BucketEpoch              int64   `json:"bucket_epoch"`
	CPUUsageSum              float64 `json:"cpu_usage_sum"`
	CPUUsageCount            int64   `json:"cpu_usage_count"`
	MemoryUsageSum           float64 `json:"memory_usage_sum"`
	MemoryUsageCount         int64   `json:"memory_usage_count"`
	ReportedNodes            int     `json:"reported_nodes"`
	NetworkRxBytes           int64   `json:"network_rx_bytes"`
	NetworkTxBytes           int64   `json:"network_tx_bytes"`
	OpenrestyRxBytes         int64   `json:"openresty_rx_bytes"`
	OpenrestyTxBytes         int64   `json:"openresty_tx_bytes"`
	OpenrestyConnectionTotal int64   `json:"openresty_connection_total"`
}

type NodeMetricSnapshotCounterSample struct {
	NodeID           string    `json:"node_id"`
	CapturedAt       time.Time `json:"captured_at"`
	DiskReadBytes    int64     `json:"disk_read_bytes"`
	DiskWriteBytes   int64     `json:"disk_write_bytes"`
	NetworkRxBytes   int64     `json:"network_rx_bytes"`
	NetworkTxBytes   int64     `json:"network_tx_bytes"`
	OpenrestyRxBytes int64     `json:"openresty_rx_bytes"`
	OpenrestyTxBytes int64     `json:"openresty_tx_bytes"`
}

type NodeMetricSnapshotCounterDeltaBucket struct {
	BucketEpoch         int64  `json:"bucket_epoch"`
	NodeID              string `json:"node_id"`
	DiskReadBytes       int64  `json:"disk_read_bytes"`
	DiskWriteBytes      int64  `json:"disk_write_bytes"`
	NetworkRxBytes      int64  `json:"network_rx_bytes"`
	NetworkTxBytes      int64  `json:"network_tx_bytes"`
	OpenrestyRxBytes    int64  `json:"openresty_rx_bytes"`
	OpenrestyTxBytes    int64  `json:"openresty_tx_bytes"`
	SamplesWithPrevious int64  `json:"samples_with_previous"`
	ReportedNodeCount   int    `json:"reported_node_count"`
}

type nodeMetricSnapshotDedupKey struct {
	nodeID       string
	capturedAtNS int64
}

func (snapshot *NodeMetricSnapshot) BeforeCreate(tx *gorm.DB) error {
	return assignObservabilityID(&snapshot.ID)
}

func (snapshot *NodeMetricSnapshot) Insert() error {
	return DB.Create(snapshot).Error
}

func ListNodeMetricSnapshots(nodeID string, since time.Time, limit int) (snapshots []*NodeMetricSnapshot, err error) {
	rows, err := listNodeMetricSnapshotsSQL(nodeID, since, limit)
	if err == nil {
		return rows, nil
	}
	return listNodeMetricSnapshotsInMemory(nodeID, since, limit)
}

func listNodeMetricSnapshotsSQL(nodeID string, since time.Time, limit int) ([]*NodeMetricSnapshot, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildMetricSnapshotUnionBranches(nodeID, since, time.Time{}, "*")
	sql := "WITH metric_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT * FROM metric_rows ORDER BY captured_at desc, id desc"
	if limit > 0 {
		sql += " LIMIT ?"
		args = append(args, limit)
	}
	var rows []*NodeMetricSnapshot
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query node metric snapshots across shards failed: %w", err)
	}
	return rows, nil
}

func listNodeMetricSnapshotsInMemory(nodeID string, since time.Time, limit int) (snapshots []*NodeMetricSnapshot, err error) {
	rows, err := queryAcrossShards("node_metric_snapshots", func(tx *gorm.DB) ([]*NodeMetricSnapshot, error) {
		var shardRows []*NodeMetricSnapshot
		query := tx.Order("captured_at desc, id desc")
		if nodeID != "" {
			query = query.Where("node_id = ?", nodeID)
		}
		if !since.IsZero() {
			query = query.Where("captured_at >= ?", since)
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
		if rows[i].CapturedAt.Equal(rows[j].CapturedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CapturedAt.After(rows[j].CapturedAt)
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func ListMetricSnapshotsSince(since time.Time) (snapshots []*NodeMetricSnapshot, err error) {
	rows, err := listMetricSnapshotsSinceSQL(since)
	if err == nil {
		return rows, nil
	}
	return listMetricSnapshotsSinceInMemory(since)
}

func listMetricSnapshotsSinceSQL(since time.Time) ([]*NodeMetricSnapshot, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildMetricSnapshotUnionBranches("", since, time.Time{}, "*")
	sql := "WITH metric_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		") SELECT * FROM metric_rows ORDER BY captured_at desc, id desc"
	var rows []*NodeMetricSnapshot
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query metric snapshots across shards failed: %w", err)
	}
	return rows, nil
}

func listMetricSnapshotsSinceInMemory(since time.Time) (snapshots []*NodeMetricSnapshot, err error) {
	rows, err := queryAcrossShards("node_metric_snapshots", func(tx *gorm.DB) ([]*NodeMetricSnapshot, error) {
		var shardRows []*NodeMetricSnapshot
		query := tx.Order("captured_at desc")
		if !since.IsZero() {
			query = query.Where("captured_at >= ?", since)
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
		if rows[i].CapturedAt.Equal(rows[j].CapturedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CapturedAt.After(rows[j].CapturedAt)
	})
	return rows, nil
}

func buildMetricSnapshotUnionBranches(nodeID string, since time.Time, until time.Time, columns string) ([]string, []any) {
	trimmedNodeID := strings.TrimSpace(nodeID)
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*3)
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		branch := "SELECT " + columns + " FROM " + quoteIdentifier(table)
		whereClauses := make([]string, 0, 3)
		if trimmedNodeID != "" {
			whereClauses = append(whereClauses, "node_id = ?")
			args = append(args, trimmedNodeID)
		}
		if !since.IsZero() {
			whereClauses = append(whereClauses, "captured_at >= ?")
			args = append(args, since)
		}
		if !until.IsZero() {
			whereClauses = append(whereClauses, "captured_at <= ?")
			args = append(args, until)
		}
		if len(whereClauses) > 0 {
			branch += " WHERE " + strings.Join(whereClauses, " AND ")
		}
		branches = append(branches, branch)
	}
	return branches, args
}

func ListLatestMetricSnapshotsByNodeSince(since time.Time) (snapshots []*NodeMetricSnapshot, err error) {
	return ListLatestMetricSnapshotsByNode(since, time.Time{})
}

func ListLatestMetricSnapshotsByNode(since time.Time, until time.Time) (snapshots []*NodeMetricSnapshot, err error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches, args := buildMetricSnapshotUnionBranches("", since, until, "*")

	sql := "WITH metric_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), ranked AS (" +
		"SELECT *, ROW_NUMBER() OVER (PARTITION BY node_id ORDER BY captured_at DESC, id DESC) AS rn " +
		"FROM metric_rows WHERE TRIM(COALESCE(node_id, '')) <> ''" +
		") SELECT * FROM ranked WHERE rn = 1 ORDER BY captured_at DESC, id DESC"
	var rows []*NodeMetricSnapshot
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query latest metric snapshots by node failed: %w", err)
	}
	return rows, nil
}

func ListMetricSnapshotTrendBuckets(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotTrendBucket, error) {
	rows, err := listMetricSnapshotTrendBucketsSQL(nodeID, since, until, bucketMinutes)
	if err == nil {
		return rows, nil
	}
	return listMetricSnapshotTrendBucketsInMemory(nodeID, since, until, bucketMinutes)
}

func listMetricSnapshotTrendBucketsSQL(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotTrendBucket, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildMetricSnapshotUnionBranches(
		nodeID,
		since,
		until,
		"node_id, captured_at, cpu_usage_percent, memory_used_bytes, memory_total_bytes, network_rx_bytes, network_tx_bytes, openresty_rx_bytes, openresty_tx_bytes, openresty_connections",
	)

	bucketExpr := metricSnapshotBucketEpochExpr(bucketMinutes)
	memoryUsageExpr := metricSnapshotMemoryUsageExpr()
	sql := "WITH metric_rows AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), bucket_rows AS (" +
		"SELECT " + bucketExpr + " AS bucket_epoch, node_id, cpu_usage_percent, memory_used_bytes, memory_total_bytes, " +
		"network_rx_bytes, network_tx_bytes, openresty_rx_bytes, openresty_tx_bytes, openresty_connections FROM metric_rows" +
		") SELECT bucket_epoch, " +
		"COALESCE(SUM(CASE WHEN cpu_usage_percent > 0 THEN cpu_usage_percent ELSE 0 END), 0) AS cpu_usage_sum, " +
		"COALESCE(SUM(CASE WHEN cpu_usage_percent > 0 THEN 1 ELSE 0 END), 0) AS cpu_usage_count, " +
		"COALESCE(SUM(CASE WHEN memory_used_bytes > 0 AND memory_total_bytes > 0 THEN " + memoryUsageExpr + " ELSE 0 END), 0) AS memory_usage_sum, " +
		"COALESCE(SUM(CASE WHEN memory_used_bytes > 0 AND memory_total_bytes > 0 THEN 1 ELSE 0 END), 0) AS memory_usage_count, " +
		"COUNT(DISTINCT CASE WHEN TRIM(COALESCE(node_id, '')) <> '' THEN TRIM(node_id) ELSE NULL END) AS reported_nodes, " +
		"COALESCE(SUM(network_rx_bytes), 0) AS network_rx_bytes, " +
		"COALESCE(SUM(network_tx_bytes), 0) AS network_tx_bytes, " +
		"COALESCE(SUM(openresty_rx_bytes), 0) AS openresty_rx_bytes, " +
		"COALESCE(SUM(openresty_tx_bytes), 0) AS openresty_tx_bytes, " +
		"COALESCE(SUM(openresty_connections), 0) AS openresty_connection_total " +
		"FROM bucket_rows GROUP BY bucket_epoch ORDER BY bucket_epoch asc"
	var rows []*NodeMetricSnapshotTrendBucket
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query metric snapshot trend buckets across shards failed: %w", err)
	}
	return rows, nil
}

func listMetricSnapshotTrendBucketsInMemory(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotTrendBucket, error) {
	type shardBucketRow struct {
		BucketEpoch              int64
		NodeID                   string
		CPUUsageSum              float64
		CPUUsageCount            int64
		MemoryUsageSum           float64
		MemoryUsageCount         int64
		NetworkRxBytes           int64
		NetworkTxBytes           int64
		OpenrestyRxBytes         int64
		OpenrestyTxBytes         int64
		OpenrestyConnectionTotal int64
	}

	db := normalizeShardedDB(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	bucketExpr := metricSnapshotBucketEpochExpr(bucketMinutes)
	memoryUsageExpr := metricSnapshotMemoryUsageExpr()
	buckets := make(map[int64]*NodeMetricSnapshotTrendBucket)
	reportedNodes := make(map[int64]map[string]struct{})
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		var rows []*shardBucketRow
		query := db.Table(table).
			Select(
				bucketExpr + " AS bucket_epoch, node_id, " +
					"COALESCE(SUM(CASE WHEN cpu_usage_percent > 0 THEN cpu_usage_percent ELSE 0 END), 0) AS cpu_usage_sum, " +
					"SUM(CASE WHEN cpu_usage_percent > 0 THEN 1 ELSE 0 END) AS cpu_usage_count, " +
					"COALESCE(SUM(CASE WHEN memory_used_bytes > 0 AND memory_total_bytes > 0 THEN " + memoryUsageExpr + " ELSE 0 END), 0) AS memory_usage_sum, " +
					"SUM(CASE WHEN memory_used_bytes > 0 AND memory_total_bytes > 0 THEN 1 ELSE 0 END) AS memory_usage_count, " +
					"COALESCE(SUM(network_rx_bytes), 0) AS network_rx_bytes, " +
					"COALESCE(SUM(network_tx_bytes), 0) AS network_tx_bytes, " +
					"COALESCE(SUM(openresty_rx_bytes), 0) AS openresty_rx_bytes, " +
					"COALESCE(SUM(openresty_tx_bytes), 0) AS openresty_tx_bytes, " +
					"COALESCE(SUM(openresty_connections), 0) AS openresty_connection_total",
			).
			Group("bucket_epoch, node_id")
		if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
			query = query.Where("node_id = ?", trimmed)
		}
		if !since.IsZero() {
			query = query.Where("captured_at >= ?", since)
		}
		if !until.IsZero() {
			query = query.Where("captured_at <= ?", until)
		}
		if err := query.Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("query metric snapshot trend buckets from %s failed: %w", table, err)
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			bucket := buckets[row.BucketEpoch]
			if bucket == nil {
				bucket = &NodeMetricSnapshotTrendBucket{BucketEpoch: row.BucketEpoch}
				buckets[row.BucketEpoch] = bucket
			}
			bucket.CPUUsageSum += row.CPUUsageSum
			bucket.CPUUsageCount += row.CPUUsageCount
			bucket.MemoryUsageSum += row.MemoryUsageSum
			bucket.MemoryUsageCount += row.MemoryUsageCount
			bucket.NetworkRxBytes += row.NetworkRxBytes
			bucket.NetworkTxBytes += row.NetworkTxBytes
			bucket.OpenrestyRxBytes += row.OpenrestyRxBytes
			bucket.OpenrestyTxBytes += row.OpenrestyTxBytes
			bucket.OpenrestyConnectionTotal += row.OpenrestyConnectionTotal
			if trimmedNodeID := strings.TrimSpace(row.NodeID); trimmedNodeID != "" {
				nodes := reportedNodes[row.BucketEpoch]
				if nodes == nil {
					nodes = make(map[string]struct{})
					reportedNodes[row.BucketEpoch] = nodes
				}
				nodes[trimmedNodeID] = struct{}{}
			}
		}
	}
	rows := make([]*NodeMetricSnapshotTrendBucket, 0, len(buckets))
	for bucketEpoch, bucket := range buckets {
		bucket.ReportedNodes = len(reportedNodes[bucketEpoch])
		rows = append(rows, bucket)
	}
	sort.Slice(rows, func(i int, j int) bool {
		return rows[i].BucketEpoch < rows[j].BucketEpoch
	})
	return rows, nil
}

func ListMetricSnapshotCounterSamples(nodeID string, since time.Time, until time.Time) ([]*NodeMetricSnapshotCounterSample, error) {
	db := normalizeShardedDB(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	rows := make([]*NodeMetricSnapshotCounterSample, 0)
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		var shardRows []*NodeMetricSnapshotCounterSample
		query := db.Table(table).
			Select("node_id, captured_at, disk_read_bytes, disk_write_bytes, network_rx_bytes, network_tx_bytes, openresty_rx_bytes, openresty_tx_bytes")
		if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
			query = query.Where("node_id = ?", trimmed)
		}
		if !since.IsZero() {
			query = query.Where("captured_at >= ?", since)
		}
		if !until.IsZero() {
			query = query.Where("captured_at <= ?", until)
		}
		if err := query.Find(&shardRows).Error; err != nil {
			return nil, fmt.Errorf("query metric snapshot counter samples from %s failed: %w", table, err)
		}
		rows = append(rows, shardRows...)
	}
	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].CapturedAt.Equal(rows[j].CapturedAt) {
			return rows[i].NodeID < rows[j].NodeID
		}
		return rows[i].CapturedAt.Before(rows[j].CapturedAt)
	})
	return rows, nil
}

func ListMetricSnapshotCounterDeltaBuckets(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotCounterDeltaBucket, error) {
	rows, err := listMetricSnapshotCounterDeltaBucketsSQL(nodeID, since, until, bucketMinutes)
	if err == nil {
		return rows, nil
	}
	return listMetricSnapshotCounterDeltaBucketsInMemory(nodeID, since, until, bucketMinutes)
}

func listMetricSnapshotCounterDeltaBucketsSQL(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotCounterDeltaBucket, error) {
	db := sessionIgnoringSharding(DB)
	if db == nil {
		return nil, fmt.Errorf("database handle is nil")
	}

	branches, args := buildMetricSnapshotUnionBranches(
		nodeID,
		since,
		until,
		"id, node_id, captured_at, disk_read_bytes, disk_write_bytes, network_rx_bytes, network_tx_bytes, openresty_rx_bytes, openresty_tx_bytes",
	)

	bucketExpr := metricSnapshotBucketEpochExpr(bucketMinutes)
	sql := "WITH counter_samples AS (" +
		strings.Join(branches, " UNION ALL ") +
		"), ordered_samples AS (" +
		"SELECT *, " +
		"LAG(captured_at) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_captured_at, " +
		"LAG(disk_read_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_disk_read_bytes, " +
		"LAG(disk_write_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_disk_write_bytes, " +
		"LAG(network_rx_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_network_rx_bytes, " +
		"LAG(network_tx_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_network_tx_bytes, " +
		"LAG(openresty_rx_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_openresty_rx_bytes, " +
		"LAG(openresty_tx_bytes) OVER (PARTITION BY node_key ORDER BY captured_at ASC, id ASC) AS previous_openresty_tx_bytes " +
		"FROM (" +
		"SELECT *, CASE WHEN TRIM(COALESCE(node_id, '')) = '' THEN '__unknown__' ELSE TRIM(node_id) END AS node_key " +
		"FROM counter_samples" +
		") normalized_samples" +
		"), delta_samples AS (" +
		"SELECT " + bucketExpr + " AS bucket_epoch, node_id, " +
		"CASE WHEN disk_read_bytes < previous_disk_read_bytes THEN 0 ELSE disk_read_bytes - previous_disk_read_bytes END AS disk_read_bytes, " +
		"CASE WHEN disk_write_bytes < previous_disk_write_bytes THEN 0 ELSE disk_write_bytes - previous_disk_write_bytes END AS disk_write_bytes, " +
		"CASE WHEN network_rx_bytes < previous_network_rx_bytes THEN 0 ELSE network_rx_bytes - previous_network_rx_bytes END AS network_rx_bytes, " +
		"CASE WHEN network_tx_bytes < previous_network_tx_bytes THEN 0 ELSE network_tx_bytes - previous_network_tx_bytes END AS network_tx_bytes, " +
		"CASE WHEN openresty_rx_bytes < previous_openresty_rx_bytes THEN 0 ELSE openresty_rx_bytes - previous_openresty_rx_bytes END AS openresty_rx_bytes, " +
		"CASE WHEN openresty_tx_bytes < previous_openresty_tx_bytes THEN 0 ELSE openresty_tx_bytes - previous_openresty_tx_bytes END AS openresty_tx_bytes " +
		"FROM ordered_samples " +
		"WHERE previous_captured_at IS NOT NULL" +
		") " +
		"SELECT bucket_epoch, " +
		"COALESCE(SUM(disk_read_bytes), 0) AS disk_read_bytes, " +
		"COALESCE(SUM(disk_write_bytes), 0) AS disk_write_bytes, " +
		"COALESCE(SUM(network_rx_bytes), 0) AS network_rx_bytes, " +
		"COALESCE(SUM(network_tx_bytes), 0) AS network_tx_bytes, " +
		"COALESCE(SUM(openresty_rx_bytes), 0) AS openresty_rx_bytes, " +
		"COALESCE(SUM(openresty_tx_bytes), 0) AS openresty_tx_bytes, " +
		"COUNT(*) AS samples_with_previous, " +
		"COUNT(DISTINCT CASE WHEN TRIM(COALESCE(node_id, '')) <> '' THEN TRIM(node_id) ELSE NULL END) AS reported_node_count " +
		"FROM delta_samples GROUP BY bucket_epoch ORDER BY bucket_epoch ASC"

	var rows []*NodeMetricSnapshotCounterDeltaBucket
	if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query metric snapshot counter delta buckets failed: %w", err)
	}
	return rows, nil
}

func listMetricSnapshotCounterDeltaBucketsInMemory(nodeID string, since time.Time, until time.Time, bucketMinutes int) ([]*NodeMetricSnapshotCounterDeltaBucket, error) {
	type counterState struct {
		diskReadBytes    int64
		diskWriteBytes   int64
		networkRxBytes   int64
		networkTxBytes   int64
		openrestyRxBytes int64
		openrestyTxBytes int64
		seen             bool
	}

	samples, err := ListMetricSnapshotCounterSamples(nodeID, since, until)
	if err != nil {
		return nil, err
	}
	buckets := make(map[int64]*NodeMetricSnapshotCounterDeltaBucket)
	reportedNodes := make(map[int64]map[string]struct{})
	previousByNode := make(map[string]counterState)
	for _, sample := range samples {
		if sample == nil {
			continue
		}
		nodeKey := strings.TrimSpace(sample.NodeID)
		if nodeKey == "" {
			nodeKey = "__unknown__"
		}
		previous := previousByNode[nodeKey]
		previousByNode[nodeKey] = counterState{
			diskReadBytes:    sample.DiskReadBytes,
			diskWriteBytes:   sample.DiskWriteBytes,
			networkRxBytes:   sample.NetworkRxBytes,
			networkTxBytes:   sample.NetworkTxBytes,
			openrestyRxBytes: sample.OpenrestyRxBytes,
			openrestyTxBytes: sample.OpenrestyTxBytes,
			seen:             true,
		}
		if !previous.seen {
			continue
		}
		bucketEpoch := metricSnapshotBucketEpoch(sample.CapturedAt, bucketMinutes)
		bucket := buckets[bucketEpoch]
		if bucket == nil {
			bucket = &NodeMetricSnapshotCounterDeltaBucket{BucketEpoch: bucketEpoch}
			buckets[bucketEpoch] = bucket
		}
		bucket.DiskReadBytes += nonNegativeMetricCounterDelta(sample.DiskReadBytes, previous.diskReadBytes)
		bucket.DiskWriteBytes += nonNegativeMetricCounterDelta(sample.DiskWriteBytes, previous.diskWriteBytes)
		bucket.NetworkRxBytes += nonNegativeMetricCounterDelta(sample.NetworkRxBytes, previous.networkRxBytes)
		bucket.NetworkTxBytes += nonNegativeMetricCounterDelta(sample.NetworkTxBytes, previous.networkTxBytes)
		bucket.OpenrestyRxBytes += nonNegativeMetricCounterDelta(sample.OpenrestyRxBytes, previous.openrestyRxBytes)
		bucket.OpenrestyTxBytes += nonNegativeMetricCounterDelta(sample.OpenrestyTxBytes, previous.openrestyTxBytes)
		bucket.SamplesWithPrevious++
		if sampleNodeID := strings.TrimSpace(sample.NodeID); sampleNodeID != "" {
			nodes := reportedNodes[bucketEpoch]
			if nodes == nil {
				nodes = make(map[string]struct{})
				reportedNodes[bucketEpoch] = nodes
			}
			nodes[sampleNodeID] = struct{}{}
		}
	}
	rows := make([]*NodeMetricSnapshotCounterDeltaBucket, 0, len(buckets))
	for bucketEpoch, bucket := range buckets {
		bucket.ReportedNodeCount = len(reportedNodes[bucketEpoch])
		rows = append(rows, bucket)
	}
	sort.Slice(rows, func(i int, j int) bool {
		return rows[i].BucketEpoch < rows[j].BucketEpoch
	})
	return rows, nil
}

func NodeMetricSnapshotExists(db *gorm.DB, nodeID string, capturedAt time.Time) (bool, error) {
	db = normalizeShardedDB(db)
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		var count int64
		if err := db.Table(table).
			Where("node_id = ? AND captured_at = ?", nodeID, capturedAt).
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

func InsertNewNodeMetricSnapshots(db *gorm.DB, snapshots []*NodeMetricSnapshot) (inserted int64, err error) {
	if len(snapshots) == 0 {
		return 0, nil
	}
	db = normalizeShardedDB(db)
	if db == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	uniqueSnapshots := make([]*NodeMetricSnapshot, 0, len(snapshots))
	seenIncoming := make(map[nodeMetricSnapshotDedupKey]struct{}, len(snapshots))
	rangesByNode := make(map[string]nodeAccessLogTimeRange)
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		key := nodeMetricSnapshotDedupKeyFor(snapshot)
		if key.nodeID == "" || key.capturedAtNS == 0 {
			continue
		}
		if _, exists := seenIncoming[key]; exists {
			continue
		}
		seenIncoming[key] = struct{}{}
		uniqueSnapshots = append(uniqueSnapshots, snapshot)
		rangesByNode[key.nodeID] = expandNodeAccessLogTimeRange(rangesByNode[key.nodeID], snapshot.CapturedAt)
	}
	if len(uniqueSnapshots) == 0 {
		return 0, nil
	}

	existingKeys, err := existingNodeMetricSnapshotDedupKeys(db, rangesByNode)
	if err != nil {
		return 0, err
	}
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return 0, fmt.Errorf("database handle is nil")
	}
	grouped := make(map[string][]*NodeMetricSnapshot, observabilityShardCount)
	for _, snapshot := range uniqueSnapshots {
		key := nodeMetricSnapshotDedupKeyFor(snapshot)
		if _, exists := existingKeys[key]; exists {
			continue
		}
		if err := assignObservabilityID(&snapshot.ID); err != nil {
			return inserted, err
		}
		table := observabilityShardTableForID("node_metric_snapshots", snapshot.ID)
		grouped[table] = append(grouped[table], snapshot)
		existingKeys[key] = struct{}{}
	}
	for table, batch := range grouped {
		if len(batch) == 0 {
			continue
		}
		if err := rawDB.Table(table).CreateInBatches(batch, 500).Error; err != nil {
			return inserted, fmt.Errorf("insert metric snapshots into %s failed: %w", table, err)
		}
		inserted += int64(len(batch))
	}
	return inserted, nil
}

func existingNodeMetricSnapshotDedupKeys(db *gorm.DB, rangesByNode map[string]nodeAccessLogTimeRange) (map[nodeMetricSnapshotDedupKey]struct{}, error) {
	keys := make(map[nodeMetricSnapshotDedupKey]struct{})
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
	if sqlKeys, err := existingNodeMetricSnapshotDedupKeysSQL(db, nodeIDs, timeRange); err == nil {
		return sqlKeys, nil
	}
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		var rows []*NodeMetricSnapshot
		query := db.Table(table).
			Select("node_id, captured_at").
			Where("node_id IN ?", nodeIDs)
		if !timeRange.min.IsZero() {
			query = query.Where("captured_at >= ?", timeRange.min)
		}
		if !timeRange.max.IsZero() {
			query = query.Where("captured_at <= ?", timeRange.max)
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("query existing metric snapshot keys from %s failed: %w", table, err)
		}
		for _, row := range rows {
			if row == nil {
				continue
			}
			keys[nodeMetricSnapshotDedupKeyFor(row)] = struct{}{}
		}
	}
	return keys, nil
}

func existingNodeMetricSnapshotDedupKeysSQL(db *gorm.DB, nodeIDs []string, timeRange nodeAccessLogTimeRange) (map[nodeMetricSnapshotDedupKey]struct{}, error) {
	rawDB := sessionIgnoringSharding(db)
	if rawDB == nil {
		return nil, fmt.Errorf("database handle is nil")
	}
	branches := make([]string, 0, observabilityShardCount)
	args := make([]any, 0, observabilityShardCount*(len(nodeIDs)+2))
	for _, table := range observabilityShardTables("node_metric_snapshots") {
		branch := "SELECT node_id, captured_at FROM " + quoteIdentifier(table) +
			" WHERE node_id IN ?"
		branchArgs := []any{nodeIDs}
		if !timeRange.min.IsZero() {
			branch += " AND captured_at >= ?"
			branchArgs = append(branchArgs, timeRange.min)
		}
		if !timeRange.max.IsZero() {
			branch += " AND captured_at <= ?"
			branchArgs = append(branchArgs, timeRange.max)
		}
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	var rows []*NodeMetricSnapshot
	sql := strings.Join(branches, " UNION ALL ")
	if err := rawDB.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("query existing metric snapshot keys across shards failed: %w", err)
	}
	keys := make(map[nodeMetricSnapshotDedupKey]struct{}, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		keys[nodeMetricSnapshotDedupKeyFor(row)] = struct{}{}
	}
	return keys, nil
}

func nodeMetricSnapshotDedupKeyFor(snapshot *NodeMetricSnapshot) nodeMetricSnapshotDedupKey {
	if snapshot == nil {
		return nodeMetricSnapshotDedupKey{}
	}
	return nodeMetricSnapshotDedupKey{
		nodeID:       strings.TrimSpace(snapshot.NodeID),
		capturedAtNS: snapshot.CapturedAt.UTC().UnixNano(),
	}
}

func DeleteNodeMetricSnapshotsBefore(db *gorm.DB, before time.Time) (int64, error) {
	return deleteAcrossShards(db, "node_metric_snapshots", &NodeMetricSnapshot{}, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("captured_at < ?", before)
	})
}

func DeleteAllNodeMetricSnapshots(db *gorm.DB) (int64, error) {
	return deleteAcrossShards(db, "node_metric_snapshots", &NodeMetricSnapshot{}, nil)
}

func metricSnapshotBucketEpochExpr(bucketMinutes int) string {
	bucketSeconds := bucketMinutes * 60
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	switch databaseDialectorName(DB) {
	case "postgres":
		return fmt.Sprintf("CAST(floor(extract(epoch from captured_at) / %d) * %d AS BIGINT)", bucketSeconds, bucketSeconds)
	default:
		return fmt.Sprintf("CAST((strftime('%%s', captured_at) / %d) * %d AS INTEGER)", bucketSeconds, bucketSeconds)
	}
}

func metricSnapshotMemoryUsageExpr() string {
	switch databaseDialectorName(DB) {
	case "postgres":
		return "(CAST(memory_used_bytes AS DOUBLE PRECISION) / CAST(memory_total_bytes AS DOUBLE PRECISION)) * 100"
	default:
		return "(CAST(memory_used_bytes AS REAL) / CAST(memory_total_bytes AS REAL)) * 100"
	}
}

func metricSnapshotBucketEpoch(timestamp time.Time, bucketMinutes int) int64 {
	bucketSeconds := bucketMinutes * 60
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	unix := timestamp.UTC().Unix()
	return (unix / int64(bucketSeconds)) * int64(bucketSeconds)
}

func nonNegativeMetricCounterDelta(current int64, previous int64) int64 {
	if current < previous {
		return 0
	}
	return current - previous
}
