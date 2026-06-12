package service

import (
	"dushengcdn/model"
	"time"
)

const observabilityTrendBuckets = 24

type TrafficTrendPoint struct {
	BucketStartedAt    time.Time `json:"bucket_started_at"`
	RequestCount       int64     `json:"request_count"`
	ErrorCount         int64     `json:"error_count"`
	UniqueVisitorCount int64     `json:"unique_visitor_count"`
}

type CapacityTrendPoint struct {
	BucketStartedAt           time.Time `json:"bucket_started_at"`
	AverageCPUUsagePercent    float64   `json:"average_cpu_usage_percent"`
	AverageMemoryUsagePercent float64   `json:"average_memory_usage_percent"`
	ReportedNodes             int       `json:"reported_nodes"`
}

type NetworkTrendPoint struct {
	BucketStartedAt  time.Time `json:"bucket_started_at"`
	NetworkRxBytes   int64     `json:"network_rx_bytes"`
	NetworkTxBytes   int64     `json:"network_tx_bytes"`
	OpenrestyRxBytes int64     `json:"openresty_rx_bytes"`
	OpenrestyTxBytes int64     `json:"openresty_tx_bytes"`
	ReportedNodes    int       `json:"reported_nodes"`
}

type DiskIOTrendPoint struct {
	BucketStartedAt time.Time `json:"bucket_started_at"`
	DiskReadBytes   int64     `json:"disk_read_bytes"`
	DiskWriteBytes  int64     `json:"disk_write_bytes"`
	ReportedNodes   int       `json:"reported_nodes"`
}

func buildTrafficTrendPointsFromBuckets(now time.Time, buckets []*model.NodeRequestReportTrendBucket) []TrafficTrendPoint {
	start := trendWindowStart(now)
	points := make([]TrafficTrendPoint, observabilityTrendBuckets)
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
		points[index].RequestCount += bucket.RequestCount
		points[index].ErrorCount += bucket.ErrorCount
		points[index].UniqueVisitorCount += bucket.UniqueVisitorCount
	}

	return points
}

func buildCapacityTrendPointsFromBuckets(now time.Time, buckets []*model.NodeMetricSnapshotTrendBucket) []CapacityTrendPoint {
	start := trendWindowStart(now)
	points := make([]CapacityTrendPoint, observabilityTrendBuckets)
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
		if bucket.CPUUsageCount > 0 {
			points[index].AverageCPUUsagePercent = bucket.CPUUsageSum / float64(bucket.CPUUsageCount)
		}
		if bucket.MemoryUsageCount > 0 {
			points[index].AverageMemoryUsagePercent = bucket.MemoryUsageSum / float64(bucket.MemoryUsageCount)
		}
		points[index].ReportedNodes = bucket.ReportedNodes
	}

	return points
}

func buildNetworkTrendPointsFromCounterBuckets(now time.Time, buckets []*model.NodeMetricSnapshotCounterDeltaBucket) []NetworkTrendPoint {
	start := trendWindowStart(now)
	points := make([]NetworkTrendPoint, observabilityTrendBuckets)
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
		points[index].NetworkRxBytes += bucket.NetworkRxBytes
		points[index].NetworkTxBytes += bucket.NetworkTxBytes
		points[index].OpenrestyRxBytes += bucket.OpenrestyRxBytes
		points[index].OpenrestyTxBytes += bucket.OpenrestyTxBytes
		points[index].ReportedNodes = maxInt(points[index].ReportedNodes, bucket.ReportedNodeCount)
	}
	return points
}

func buildDiskIOTrendPointsFromCounterBuckets(now time.Time, buckets []*model.NodeMetricSnapshotCounterDeltaBucket) []DiskIOTrendPoint {
	start := trendWindowStart(now)
	points := make([]DiskIOTrendPoint, observabilityTrendBuckets)
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
		points[index].DiskReadBytes += bucket.DiskReadBytes
		points[index].DiskWriteBytes += bucket.DiskWriteBytes
		points[index].ReportedNodes = maxInt(points[index].ReportedNodes, bucket.ReportedNodeCount)
	}
	return points
}

func trendWindowStart(now time.Time) time.Time {
	return now.Truncate(time.Hour).Add(-(observabilityTrendBuckets - 1) * time.Hour)
}

func trendBucketIndex(timestamp time.Time, start time.Time) (int, bool) {
	if timestamp.Before(start) {
		return 0, false
	}
	delta := timestamp.Sub(start)
	index := int(delta / time.Hour)
	if index < 0 || index >= observabilityTrendBuckets {
		return 0, false
	}
	return index, true
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
