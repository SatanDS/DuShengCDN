package service

import (
	"dushengcdn/model"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	defaultObservabilityWindow = 24 * time.Hour
	defaultObservabilityLimit  = 120
	maxObservabilityLimit      = 500
)

type NodeObservabilityQuery struct {
	Hours int `json:"hours"`
	Limit int `json:"limit"`
}

type NodeObservabilityView struct {
	NodeID          string                      `json:"node_id"`
	Profile         *model.NodeSystemProfile    `json:"profile"`
	MetricSnapshots []*model.NodeMetricSnapshot `json:"metric_snapshots"`
	TrafficReports  []*model.NodeRequestReport  `json:"traffic_reports"`
	HealthEvents    []*model.NodeHealthEvent    `json:"health_events"`
	Analytics       NodeObservabilityAnalytics  `json:"analytics"`
	Trends          NodeObservabilityTrends     `json:"trends"`
}

type NodeObservabilityAnalytics struct {
	Traffic       TrafficWindowSummary       `json:"traffic"`
	Distributions TrafficDistributions       `json:"distributions"`
	Health        ObservabilityHealthSummary `json:"health"`
}

type NodeObservabilityTrends struct {
	Traffic24h  []TrafficTrendPoint  `json:"traffic_24h"`
	Capacity24h []CapacityTrendPoint `json:"capacity_24h"`
	Network24h  []NetworkTrendPoint  `json:"network_24h"`
	DiskIO24h   []DiskIOTrendPoint   `json:"disk_io_24h"`
}

type NodeHealthEventCleanupResult struct {
	NodeID       string `json:"node_id"`
	DeletedCount int64  `json:"deleted_count"`
}

type nodeObservabilityQueryData struct {
	profile             *model.NodeSystemProfile
	snapshots           []*model.NodeMetricSnapshot
	reports             []*model.NodeRequestReport
	accessLogRegions    []*model.NodeAccessLogRegionCount
	metricTrendBuckets  []*model.NodeMetricSnapshotTrendBucket
	counterTrendBuckets []*model.NodeMetricSnapshotCounterDeltaBucket
	trafficTrendBuckets []*model.NodeRequestReportTrendBucket
	events              []*model.NodeHealthEvent
}

type nodeObservabilityQueries struct {
	getNodeSystemProfile           func(string) (*model.NodeSystemProfile, error)
	listNodeMetricSnapshots        func(string, time.Time, int) ([]*model.NodeMetricSnapshot, error)
	listNodeRequestReports         func(string, time.Time, int) ([]*model.NodeRequestReport, error)
	listNodeAccessLogRegionCounts  func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error)
	listMetricSnapshotTrendBuckets func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotTrendBucket, error)
	listMetricCounterDeltaBuckets  func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotCounterDeltaBucket, error)
	listRequestReportTrendBuckets  func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error)
	listNodeHealthEvents           func(string, bool, int) ([]*model.NodeHealthEvent, error)
}

var defaultNodeObservabilityQueries = nodeObservabilityQueries{
	getNodeSystemProfile:           model.GetNodeSystemProfile,
	listNodeMetricSnapshots:        model.ListNodeMetricSnapshots,
	listNodeRequestReports:         model.ListNodeRequestReports,
	listNodeAccessLogRegionCounts:  model.ListNodeAccessLogRegionCounts,
	listMetricSnapshotTrendBuckets: model.ListMetricSnapshotTrendBuckets,
	listMetricCounterDeltaBuckets:  model.ListMetricSnapshotCounterDeltaBuckets,
	listRequestReportTrendBuckets:  model.ListRequestReportTrendBuckets,
	listNodeHealthEvents:           model.ListNodeHealthEvents,
}

func GetNodeObservability(id uint, query NodeObservabilityQuery) (*NodeObservabilityView, error) {
	now := time.Now()
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}

	limit := normalizeObservabilityLimit(query.Limit)
	since := now.Add(-normalizeObservabilityWindow(query.Hours))
	trendSince := now.Add(-24 * time.Hour)

	data, err := loadNodeObservabilityQueryData(node.NodeID, since, trendSince, now, limit, defaultNodeObservabilityQueries)
	if err != nil {
		return nil, err
	}

	return &NodeObservabilityView{
		NodeID:          node.NodeID,
		Profile:         data.profile,
		MetricSnapshots: data.snapshots,
		TrafficReports:  data.reports,
		HealthEvents:    data.events,
		Analytics: NodeObservabilityAnalytics{
			Traffic:       buildTrafficWindowSummary(latestTrafficReport(data.reports)),
			Distributions: buildTrafficDistributions(data.reports, data.accessLogRegions, 8),
			Health:        buildObservabilityHealthSummary(latestMetricSnapshot(data.snapshots), latestTrafficReport(data.reports), data.events),
		},
		Trends: NodeObservabilityTrends{
			Traffic24h:  buildTrafficTrendPointsFromBuckets(now, data.trafficTrendBuckets),
			Capacity24h: buildCapacityTrendPointsFromBuckets(now, data.metricTrendBuckets),
			Network24h:  buildNetworkTrendPointsFromCounterBuckets(now, data.counterTrendBuckets),
			DiskIO24h:   buildDiskIOTrendPointsFromCounterBuckets(now, data.counterTrendBuckets),
		},
	}, nil
}

func loadNodeObservabilityQueryData(nodeID string, since time.Time, trendSince time.Time, now time.Time, limit int, queries nodeObservabilityQueries) (*nodeObservabilityQueryData, error) {
	data := &nodeObservabilityQueryData{}
	if err := runConcurrentQueries(
		func() error {
			profile, err := queries.getNodeSystemProfile(nodeID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			data.profile = profile
			return err
		},
		func() error {
			rows, err := queries.listNodeMetricSnapshots(nodeID, since, limit)
			data.snapshots = rows
			return err
		},
		func() error {
			rows, err := queries.listNodeRequestReports(nodeID, since, limit)
			data.reports = rows
			return err
		},
		func() error {
			rows, err := queries.listNodeAccessLogRegionCounts(nodeID, since, 8)
			data.accessLogRegions = rows
			return err
		},
		func() error {
			rows, err := queries.listMetricSnapshotTrendBuckets(nodeID, trendSince, now, 60)
			data.metricTrendBuckets = rows
			return err
		},
		func() error {
			rows, err := queries.listMetricCounterDeltaBuckets(nodeID, trendSince, now, 60)
			data.counterTrendBuckets = rows
			return err
		},
		func() error {
			rows, err := queries.listRequestReportTrendBuckets(nodeID, trendSince, now, 60)
			data.trafficTrendBuckets = rows
			return err
		},
		func() error {
			rows, err := queries.listNodeHealthEvents(nodeID, false, limit)
			data.events = rows
			return err
		},
	); err != nil {
		return nil, err
	}
	return data, nil
}

func CleanupNodeHealthEvents(id uint) (*NodeHealthEventCleanupResult, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	deletedCount, err := model.DeleteNodeHealthEvents(node.NodeID)
	if err != nil {
		return nil, err
	}
	return &NodeHealthEventCleanupResult{
		NodeID:       node.NodeID,
		DeletedCount: deletedCount,
	}, nil
}

func latestMetricSnapshot(snapshots []*model.NodeMetricSnapshot) *model.NodeMetricSnapshot {
	for _, snapshot := range snapshots {
		if snapshot != nil {
			return snapshot
		}
	}
	return nil
}

func latestTrafficReport(reports []*model.NodeRequestReport) *model.NodeRequestReport {
	for _, report := range reports {
		if report != nil {
			return report
		}
	}
	return nil
}

func normalizeObservabilityLimit(limit int) int {
	if limit <= 0 {
		return defaultObservabilityLimit
	}
	if limit > maxObservabilityLimit {
		return maxObservabilityLimit
	}
	return limit
}

func normalizeObservabilityWindow(hours int) time.Duration {
	if hours <= 0 {
		return defaultObservabilityWindow
	}
	return time.Duration(hours) * time.Hour
}
