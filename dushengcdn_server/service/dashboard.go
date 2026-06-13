package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"sort"
	"time"
)

var dashboardOverviewCache = shortTTLResultCache[*DashboardOverviewView]{
	values:     make(map[string]shortTTLResultCacheEntry[*DashboardOverviewView]),
	ttl:        10 * time.Second,
	maxEntries: 8,
}

type DashboardOverviewView struct {
	GeneratedAt   time.Time             `json:"generated_at"`
	Summary       DashboardSummary      `json:"summary"`
	Traffic       DashboardTraffic      `json:"traffic"`
	Capacity      DashboardCapacity     `json:"capacity"`
	Distributions TrafficDistributions  `json:"distributions"`
	Trends        DashboardTrends       `json:"trends"`
	Nodes         []DashboardNodeHealth `json:"nodes"`
}

type DashboardSummary struct {
	TotalNodes     int `json:"total_nodes"`
	OnlineNodes    int `json:"online_nodes"`
	OfflineNodes   int `json:"offline_nodes"`
	PendingNodes   int `json:"pending_nodes"`
	UnhealthyNodes int `json:"unhealthy_nodes"`
}

type DashboardTraffic struct {
	RequestCount         int64   `json:"request_count"`
	UniqueVisitors       int64   `json:"unique_visitors"`
	ErrorCount           int64   `json:"error_count"`
	EstimatedQPS         float64 `json:"estimated_qps"`
	ReportedNodes        int     `json:"reported_nodes"`
	CacheHitCount        int64   `json:"cache_hit_count"`
	CacheMissCount       int64   `json:"cache_miss_count"`
	CacheBypassCount     int64   `json:"cache_bypass_count"`
	CacheExpiredCount    int64   `json:"cache_expired_count"`
	CacheStaleCount      int64   `json:"cache_stale_count"`
	CacheClassifiedCount int64   `json:"cache_classified_count"`
	CacheHitRatePercent  float64 `json:"cache_hit_rate_percent"`
}

type DashboardCapacity struct {
	AverageCPUUsagePercent    float64 `json:"average_cpu_usage_percent"`
	AverageMemoryUsagePercent float64 `json:"average_memory_usage_percent"`
	HighCPUNodes              int     `json:"high_cpu_nodes"`
	HighMemoryNodes           int     `json:"high_memory_nodes"`
	HighStorageNodes          int     `json:"high_storage_nodes"`
}

type DashboardTrends struct {
	Traffic24h  []TrafficTrendPoint  `json:"traffic_24h"`
	Capacity24h []CapacityTrendPoint `json:"capacity_24h"`
	Network24h  []NetworkTrendPoint  `json:"network_24h"`
	DiskIO24h   []DiskIOTrendPoint   `json:"disk_io_24h"`
}

type DashboardNodeHealth struct {
	ID                  uint     `json:"id"`
	NodeID              string   `json:"node_id"`
	Name                string   `json:"name"`
	GeoName             string   `json:"geo_name"`
	GeoLatitude         *float64 `json:"geo_latitude"`
	GeoLongitude        *float64 `json:"geo_longitude"`
	Status              string   `json:"status"`
	OpenrestyStatus     string   `json:"openresty_status"`
	CurrentVersion      string   `json:"current_version"`
	LastSeenAt          any      `json:"last_seen_at"`
	ActiveEventCount    int      `json:"active_event_count"`
	CPUUsagePercent     float64  `json:"cpu_usage_percent"`
	MemoryUsagePercent  float64  `json:"memory_usage_percent"`
	StorageUsagePercent float64  `json:"storage_usage_percent"`
	RequestCount        int64    `json:"request_count"`
	ErrorCount          int64    `json:"error_count"`
	UniqueVisitorCount  int64    `json:"unique_visitor_count"`
}

type dashboardOverviewQueryData struct {
	nodes               []*model.Node
	latestSnapshotRows  []*model.NodeMetricSnapshot
	latestTrafficRows   []*model.NodeRequestReport
	trafficTrendBuckets []*model.NodeRequestReportTrendBucket
	metricTrendBuckets  []*model.NodeMetricSnapshotTrendBucket
	counterTrendBuckets []*model.NodeMetricSnapshotCounterDeltaBucket
	accessLogStatus     []*model.NodeAccessLogDistributionRow
	accessLogTopDomains []*model.NodeAccessLogDistributionRow
	accessLogRegions    []*model.NodeAccessLogRegionCount
	activeEvents        []*model.NodeHealthEvent
}

type dashboardOverviewQueries struct {
	listNodes                        func() ([]*model.Node, error)
	listLatestMetricSnapshotsByNode  func(time.Time, time.Time) ([]*model.NodeMetricSnapshot, error)
	listLatestRequestReportsByNode   func(time.Time, time.Time) ([]*model.NodeRequestReport, error)
	listRequestReportTrendBuckets    func(string, time.Time, time.Time, int) ([]*model.NodeRequestReportTrendBucket, error)
	listMetricSnapshotTrendBuckets   func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotTrendBucket, error)
	listMetricCounterDeltaBuckets    func(string, time.Time, time.Time, int) ([]*model.NodeMetricSnapshotCounterDeltaBucket, error)
	listAccessLogStatusDistributions func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error)
	listAccessLogHostDistributions   func(model.NodeAccessLogDistributionQuery) ([]*model.NodeAccessLogDistributionRow, error)
	listAccessLogRegionCounts        func(string, time.Time, int) ([]*model.NodeAccessLogRegionCount, error)
	listActiveNodeHealthEvents       func() ([]*model.NodeHealthEvent, error)
}

var defaultDashboardOverviewQueries = dashboardOverviewQueries{
	listNodes:                        model.ListNodes,
	listLatestMetricSnapshotsByNode:  model.ListLatestMetricSnapshotsByNode,
	listLatestRequestReportsByNode:   model.ListLatestRequestReportsByNode,
	listRequestReportTrendBuckets:    model.ListRequestReportTrendBuckets,
	listMetricSnapshotTrendBuckets:   model.ListMetricSnapshotTrendBuckets,
	listMetricCounterDeltaBuckets:    model.ListMetricSnapshotCounterDeltaBuckets,
	listAccessLogStatusDistributions: model.ListNodeAccessLogStatusDistributions,
	listAccessLogHostDistributions:   model.ListNodeAccessLogHostDistributions,
	listAccessLogRegionCounts:        model.ListNodeAccessLogRegionCounts,
	listActiveNodeHealthEvents:       model.ListActiveNodeHealthEvents,
}

func GetDashboardOverview() (*DashboardOverviewView, error) {
	return dashboardOverviewCache.load(serviceRuntimeCacheKey("dashboard-overview"), buildDashboardOverview)
}

func resetDashboardOverviewCache() {
	dashboardOverviewCache.reset()
}

func buildDashboardOverview() (*DashboardOverviewView, error) {
	now := time.Now()
	since := now.Add(-24 * time.Hour)

	data, err := loadDashboardOverviewQueryData(since, now, defaultDashboardOverviewQueries)
	if err != nil {
		return nil, err
	}

	view := &DashboardOverviewView{
		GeneratedAt:   now,
		Nodes:         make([]DashboardNodeHealth, 0, len(data.nodes)),
		Distributions: buildDashboardTrafficDistributions(data.accessLogStatus, data.accessLogTopDomains, data.accessLogRegions, 8),
		Trends: DashboardTrends{
			Traffic24h:  buildTrafficTrendPointsFromBuckets(now, data.trafficTrendBuckets),
			Capacity24h: buildCapacityTrendPointsFromBuckets(now, data.metricTrendBuckets),
			Network24h:  buildNetworkTrendPointsFromCounterBuckets(now, data.counterTrendBuckets),
			DiskIO24h:   buildDiskIOTrendPointsFromCounterBuckets(now, data.counterTrendBuckets),
		},
	}

	var cpuNodeCount int
	var memoryNodeCount int
	latestSnapshots := latestMetricSnapshotsByNode(data.latestSnapshotRows)
	latestTrafficReports := latestTrafficReportsByNode(data.latestTrafficRows)
	activeEventsByNode := activeHealthEventsByNode(data.activeEvents)

	for _, node := range data.nodes {
		computedStatus := computeNodeStatus(node)
		switch computedStatus {
		case NodeStatusOnline:
			view.Summary.OnlineNodes++
		case NodeStatusOffline:
			view.Summary.OfflineNodes++
		case NodeStatusPending:
			view.Summary.PendingNodes++
		}
		if node.OpenrestyStatus == OpenrestyStatusUnhealthy {
			view.Summary.UnhealthyNodes++
		}

		latestSnapshot := latestSnapshots[node.NodeID]
		latestTraffic := latestTrafficReports[node.NodeID]
		nodeActiveEvents := activeEventsByNode[node.NodeID]

		nodeHealth := DashboardNodeHealth{
			ID:               node.ID,
			NodeID:           node.NodeID,
			Name:             node.Name,
			GeoName:          node.GeoName,
			GeoLatitude:      node.GeoLatitude,
			GeoLongitude:     node.GeoLongitude,
			Status:           computedStatus,
			OpenrestyStatus:  node.OpenrestyStatus,
			CurrentVersion:   node.CurrentVersion,
			LastSeenAt:       nodeViewLastSeenAt(node),
			ActiveEventCount: len(nodeActiveEvents),
		}

		if latestSnapshot != nil {
			nodeHealth.CPUUsagePercent = latestSnapshot.CPUUsagePercent
			nodeHealth.MemoryUsagePercent = percentage(latestSnapshot.MemoryUsedBytes, latestSnapshot.MemoryTotalBytes)
			nodeHealth.StorageUsagePercent = percentage(latestSnapshot.StorageUsedBytes, latestSnapshot.StorageTotalBytes)
			if latestSnapshot.CPUUsagePercent > 0 {
				view.Capacity.AverageCPUUsagePercent += latestSnapshot.CPUUsagePercent
				cpuNodeCount++
			}
			if nodeHealth.MemoryUsagePercent > 0 {
				view.Capacity.AverageMemoryUsagePercent += nodeHealth.MemoryUsagePercent
				memoryNodeCount++
			}
			if latestSnapshot.CPUUsagePercent >= 80 {
				view.Capacity.HighCPUNodes++
			}
			if nodeHealth.MemoryUsagePercent >= 85 {
				view.Capacity.HighMemoryNodes++
			}
			if nodeHealth.StorageUsagePercent >= 85 {
				view.Capacity.HighStorageNodes++
			}
		}

		if latestTraffic != nil {
			nodeHealth.RequestCount = latestTraffic.RequestCount
			nodeHealth.ErrorCount = latestTraffic.ErrorCount
			nodeHealth.UniqueVisitorCount = latestTraffic.UniqueVisitorCount
			view.Traffic.RequestCount += latestTraffic.RequestCount
			view.Traffic.UniqueVisitors += latestTraffic.UniqueVisitorCount
			view.Traffic.ErrorCount += latestTraffic.ErrorCount
			view.Traffic.CacheHitCount += latestTraffic.CacheHitCount
			view.Traffic.CacheMissCount += latestTraffic.CacheMissCount
			view.Traffic.CacheBypassCount += latestTraffic.CacheBypassCount
			view.Traffic.CacheExpiredCount += latestTraffic.CacheExpiredCount
			view.Traffic.CacheStaleCount += latestTraffic.CacheStaleCount
			view.Traffic.CacheClassifiedCount += latestTraffic.CacheHitCount + latestTraffic.CacheMissCount + latestTraffic.CacheBypassCount + latestTraffic.CacheExpiredCount + latestTraffic.CacheStaleCount
			if duration := latestTraffic.WindowEndedAt.Sub(latestTraffic.WindowStartedAt).Seconds(); duration > 0 {
				view.Traffic.EstimatedQPS += float64(latestTraffic.RequestCount) / duration
			}
			view.Traffic.ReportedNodes++
		}

		view.Nodes = append(view.Nodes, nodeHealth)
	}

	view.Summary.TotalNodes = len(data.nodes)

	if cpuNodeCount > 0 {
		view.Capacity.AverageCPUUsagePercent /= float64(cpuNodeCount)
	}
	if memoryNodeCount > 0 {
		view.Capacity.AverageMemoryUsagePercent /= float64(memoryNodeCount)
	}
	if view.Traffic.CacheClassifiedCount > 0 {
		view.Traffic.CacheHitRatePercent = float64(view.Traffic.CacheHitCount) / float64(view.Traffic.CacheClassifiedCount) * 100
	}

	sort.Slice(view.Nodes, func(i int, j int) bool {
		if view.Nodes[i].ActiveEventCount == view.Nodes[j].ActiveEventCount {
			return view.Nodes[i].CPUUsagePercent > view.Nodes[j].CPUUsagePercent
		}
		return view.Nodes[i].ActiveEventCount > view.Nodes[j].ActiveEventCount
	})

	return view, nil
}

func loadDashboardOverviewQueryData(since time.Time, now time.Time, queries dashboardOverviewQueries) (*dashboardOverviewQueryData, error) {
	data := &dashboardOverviewQueryData{}
	if err := runConcurrentQueries(
		func() error {
			rows, err := queries.listNodes()
			data.nodes = rows
			return err
		},
		func() error {
			rows, err := queries.listLatestMetricSnapshotsByNode(since, now)
			data.latestSnapshotRows = rows
			return err
		},
		func() error {
			rows, err := queries.listLatestRequestReportsByNode(since, now)
			data.latestTrafficRows = rows
			return err
		},
		func() error {
			rows, err := queries.listRequestReportTrendBuckets("", since, now, 60)
			data.trafficTrendBuckets = rows
			return err
		},
		func() error {
			rows, err := queries.listMetricSnapshotTrendBuckets("", since, now, 60)
			data.metricTrendBuckets = rows
			return err
		},
		func() error {
			rows, err := queries.listMetricCounterDeltaBuckets("", since, now, 60)
			data.counterTrendBuckets = rows
			return err
		},
		func() error {
			if !common.AccessLogPersistenceEnabled {
				return nil
			}
			rows, err := queries.listAccessLogStatusDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: 8})
			data.accessLogStatus = rows
			return err
		},
		func() error {
			if !common.AccessLogPersistenceEnabled {
				return nil
			}
			rows, err := queries.listAccessLogHostDistributions(model.NodeAccessLogDistributionQuery{Since: since, Limit: 8})
			data.accessLogTopDomains = rows
			return err
		},
		func() error {
			if !common.AccessLogPersistenceEnabled {
				return nil
			}
			rows, err := queries.listAccessLogRegionCounts("", since, 8)
			data.accessLogRegions = rows
			return err
		},
		func() error {
			rows, err := queries.listActiveNodeHealthEvents()
			data.activeEvents = rows
			return err
		},
	); err != nil {
		return nil, err
	}
	return data, nil
}

func percentage(used int64, total int64) float64 {
	if used <= 0 || total <= 0 {
		return 0
	}
	return (float64(used) / float64(total)) * 100
}

func latestMetricSnapshotsByNode(snapshots []*model.NodeMetricSnapshot) map[string]*model.NodeMetricSnapshot {
	result := make(map[string]*model.NodeMetricSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.NodeID == "" {
			continue
		}
		if existing, ok := result[snapshot.NodeID]; ok && !snapshot.CapturedAt.After(existing.CapturedAt) {
			continue
		}
		result[snapshot.NodeID] = snapshot
	}
	return result
}

func latestTrafficReportsByNode(reports []*model.NodeRequestReport) map[string]*model.NodeRequestReport {
	result := make(map[string]*model.NodeRequestReport, len(reports))
	for _, report := range reports {
		if report == nil || report.NodeID == "" {
			continue
		}
		if existing, ok := result[report.NodeID]; ok && !report.WindowEndedAt.After(existing.WindowEndedAt) {
			continue
		}
		result[report.NodeID] = report
	}
	return result
}

func activeHealthEventsByNode(events []*model.NodeHealthEvent) map[string][]*model.NodeHealthEvent {
	result := make(map[string][]*model.NodeHealthEvent)
	for _, event := range events {
		if event == nil || event.NodeID == "" {
			continue
		}
		result[event.NodeID] = append(result[event.NodeID], event)
	}
	return result
}
