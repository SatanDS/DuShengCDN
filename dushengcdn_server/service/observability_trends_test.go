package service

import (
	"dushengcdn/model"
	"testing"
	"time"
)

func TestBuildDiskIOTrendPointsFromCounterBucketsMapsBuckets(t *testing.T) {
	now := time.Date(2026, 3, 14, 18, 30, 0, 0, time.UTC)
	start := trendWindowStart(now)

	points := buildDiskIOTrendPointsFromCounterBuckets(now, []*model.NodeMetricSnapshotCounterDeltaBucket{
		nil,
		{
			BucketEpoch:       start.Add(23 * time.Hour).Unix(),
			DiskReadBytes:     150,
			DiskWriteBytes:    60,
			ReportedNodeCount: 2,
		},
		{
			BucketEpoch:       start.Add(-time.Hour).Unix(),
			DiskReadBytes:     999,
			DiskWriteBytes:    999,
			ReportedNodeCount: 9,
		},
	})

	if len(points) != observabilityTrendBuckets {
		t.Fatalf("expected %d trend buckets, got %d", observabilityTrendBuckets, len(points))
	}
	last := points[len(points)-1]
	if last.DiskReadBytes != 150 || last.DiskWriteBytes != 60 || last.ReportedNodes != 2 {
		t.Fatalf("expected disk io trend to map counter bucket into the last point, got %+v", last)
	}
	if points[0].DiskReadBytes != 0 || points[0].DiskWriteBytes != 0 {
		t.Fatalf("expected out-of-window bucket to be ignored, got %+v", points[0])
	}
}
