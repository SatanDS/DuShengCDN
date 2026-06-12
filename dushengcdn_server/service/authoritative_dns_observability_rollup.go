package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"time"
)

var dnsObservabilityHeavyCounterScanLimit = 5000

type DNSQueryRollupInput struct {
	WindowStart     time.Time        `json:"window_start"`
	WindowMinutes   int              `json:"window_minutes"`
	ZoneID          uint             `json:"zone_id"`
	ProxyRouteID    uint             `json:"proxy_route_id"`
	SourceScope     string           `json:"source_scope"`
	SourceCountry   string           `json:"source_country"`
	SourceASN       uint32           `json:"source_asn"`
	SourceOperator  string           `json:"source_operator"`
	QName           string           `json:"qname"`
	QType           string           `json:"qtype"`
	RCode           string           `json:"rcode"`
	QueryCount      int64            `json:"query_count"`
	TotalDurationMs int64            `json:"total_duration_ms"`
	MaxDurationMs   int64            `json:"max_duration_ms"`
	TargetSummary   map[string]int64 `json:"target_summary"`
}

type DNSObservabilitySummaryInput struct {
	Hours    int
	ZoneID   uint
	WorkerID string
}

type DNSObservabilityCounterView struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type DNSObservabilitySummaryView struct {
	WindowHours             int                              `json:"window_hours"`
	WindowStart             time.Time                        `json:"window_start"`
	WindowEnd               time.Time                        `json:"window_end"`
	LastRollupAt            *time.Time                       `json:"last_rollup_at"`
	TotalQueries            int64                            `json:"total_queries"`
	SuccessfulQueries       int64                            `json:"successful_queries"`
	NegativeQueries         int64                            `json:"negative_queries"`
	ErrorQueries            int64                            `json:"error_queries"`
	DynamicQueries          int64                            `json:"dynamic_queries"`
	StaticQueries           int64                            `json:"static_queries"`
	RCodeBreakdown          []DNSObservabilityCounterView    `json:"rcode_breakdown"`
	QTypeBreakdown          []DNSObservabilityCounterView    `json:"qtype_breakdown"`
	TopQNames               []DNSObservabilityCounterView    `json:"top_qnames"`
	TopTargets              []DNSObservabilityCounterView    `json:"top_targets"`
	WorkerBreakdown         []DNSObservabilityCounterView    `json:"worker_breakdown"`
	ZoneBreakdown           []DNSObservabilityCounterView    `json:"zone_breakdown"`
	RouteBreakdown          []DNSObservabilityCounterView    `json:"route_breakdown"`
	SourceScopeBreakdown    []DNSObservabilityCounterView    `json:"source_scope_breakdown"`
	SourceCountryBreakdown  []DNSObservabilityCounterView    `json:"source_country_breakdown"`
	SourceASNBreakdown      []DNSObservabilityCounterView    `json:"source_asn_breakdown"`
	SourceOperatorBreakdown []DNSObservabilityCounterView    `json:"source_operator_breakdown"`
	TrendPoints             []DNSObservabilityTrendPointView `json:"trend_points"`
	SnapshotConsistency     DNSWorkerSnapshotConsistencyView `json:"snapshot_consistency"`
	WorkerHealth            DNSWorkerHealthSummaryView       `json:"worker_health"`
}

type DNSObservabilityTrendPointView struct {
	BucketStartedAt   time.Time `json:"bucket_started_at"`
	QueryCount        int64     `json:"query_count"`
	SuccessfulQueries int64     `json:"successful_queries"`
	NegativeQueries   int64     `json:"negative_queries"`
	ErrorQueries      int64     `json:"error_queries"`
	DynamicQueries    int64     `json:"dynamic_queries"`
	StaticQueries     int64     `json:"static_queries"`
	NoErrorQueries    int64     `json:"noerror_queries"`
	NXDomainQueries   int64     `json:"nxdomain_queries"`
	ServfailQueries   int64     `json:"servfail_queries"`
}

type dnsObservabilityWindow struct {
	Hours       int
	WindowStart time.Time
	WindowEnd   time.Time
	QueryStart  time.Time
}

type dnsObservabilityStringCountRow struct {
	Key   string
	Count int64
}

type dnsObservabilityUintCountRow struct {
	Key   uint
	Count int64
}

type dnsObservabilityTrendRow struct {
	Bucket  string
	RCode   string
	Dynamic int
	Count   int64
}

type dnsObservabilityTargetSummaryRow struct {
	TargetSummary string
}

type dnsObservabilityLastRollupRow struct {
	WindowStart   time.Time
	WindowMinutes int
}

type dnsWorkerHealthRollupRow struct {
	WorkerID        string
	QueryCount      int64
	ErrorQueries    int64
	TotalDurationMs int64
	MaxDurationMs   int64
}

type dnsObservabilitySummaryQueryData struct {
	rcodeCounts          map[string]int64
	qtypeCounts          map[string]int64
	qnameCounts          map[string]int64
	workerCounts         map[string]int64
	sourceScopeCounts    map[string]int64
	sourceCountryCounts  map[string]int64
	sourceASNCounts      map[uint]int64
	sourceOperatorCounts map[string]int64
	zoneCounts           map[uint]int64
	routeCounts          map[uint]int64
	targetCounts         map[string]int64
	dynamicQueries       int64
	trendPoints          []DNSObservabilityTrendPointView
	workerHealthRollups  []dnsWorkerHealthRollupRow
	lastRollupAt         *time.Time
}

type dnsObservabilitySummaryQueries struct {
	queryRollupData   func(DNSObservabilitySummaryInput, dnsObservabilityWindow) (*dnsObservabilitySummaryQueryData, error)
	queryLastRollupAt func(DNSObservabilitySummaryInput, dnsObservabilityWindow) (*time.Time, error)
}

type dnsObservabilitySummaryLabelData struct {
	workerLabels map[string]string
	zoneLabels   map[string]string
	routeLabels  map[string]string
}

type dnsObservabilitySummaryLabelQueries struct {
	dnsWorkerLabels func() (map[string]string, error)
	dnsZoneLabels   func() (map[string]string, error)
	dnsRouteLabels  func(map[uint]int64) (map[string]string, error)
}

var defaultDNSObservabilitySummaryQueries = dnsObservabilitySummaryQueries{
	queryRollupData:   queryDNSObservabilityAggregatedData,
	queryLastRollupAt: queryDNSObservabilityLastRollupAt,
}

var defaultDNSObservabilitySummaryLabelQueries = dnsObservabilitySummaryLabelQueries{
	dnsWorkerLabels: dnsWorkerLabels,
	dnsZoneLabels:   dnsZoneLabels,
	dnsRouteLabels:  dnsRouteLabels,
}

type DNSWorkerSnapshotConsistencyView struct {
	Status                string                         `json:"status"`
	CheckedAt             time.Time                      `json:"checked_at"`
	SnapshotMaxAgeSeconds int64                          `json:"snapshot_max_age_seconds"`
	TotalWorkerCount      int                            `json:"total_worker_count"`
	OnlineWorkerCount     int                            `json:"online_worker_count"`
	StaleWorkerCount      int                            `json:"stale_worker_count"`
	DivergentWorkerCount  int                            `json:"divergent_worker_count"`
	LatestSnapshotVersion string                         `json:"latest_snapshot_version"`
	LatestSnapshotAt      *time.Time                     `json:"latest_snapshot_at"`
	VersionBreakdown      []DNSWorkerSnapshotVersionView `json:"version_breakdown"`
	Workers               []DNSWorkerSnapshotWorkerView  `json:"workers"`
}

type DNSWorkerSnapshotVersionView struct {
	Version          string     `json:"version"`
	WorkerCount      int        `json:"worker_count"`
	LatestSnapshotAt *time.Time `json:"latest_snapshot_at"`
	Workers          []string   `json:"workers"`
}

type DNSWorkerSnapshotWorkerView struct {
	WorkerID              string     `json:"worker_id"`
	Name                  string     `json:"name"`
	Status                string     `json:"status"`
	SnapshotVersion       string     `json:"snapshot_version"`
	LastSnapshotAt        *time.Time `json:"last_snapshot_at"`
	LastSeenAt            *time.Time `json:"last_seen_at"`
	LastHeartbeatAt       *time.Time `json:"last_heartbeat_at"`
	LastRollupAt          *time.Time `json:"last_rollup_at"`
	LastRollupCount       int64      `json:"last_rollup_count"`
	Stale                 bool       `json:"stale"`
	GeoIPEnabled          bool       `json:"geoip_enabled"`
	GeoIPLastError        string     `json:"geoip_last_error"`
	ASNLastError          string     `json:"asn_last_error"`
	GeoIPCountryEnabled   bool       `json:"geoip_country_enabled"`
	GeoIPASNEnabled       bool       `json:"geoip_asn_enabled"`
	GeoIPOperatorEnabled  bool       `json:"geoip_operator_enabled"`
	OperatorCIDRLastError string     `json:"operator_cidr_last_error"`
}

type DNSWorkerHealthSummaryView struct {
	CheckedAt               time.Time                 `json:"checked_at"`
	TotalWorkerCount        int                       `json:"total_worker_count"`
	OnlineWorkerCount       int                       `json:"online_worker_count"`
	ProbeHealthyCount       int                       `json:"probe_healthy_count"`
	ProbeCheckedCount       int                       `json:"probe_checked_count"`
	ProbeHealthyPercent     float64                   `json:"probe_healthy_percent"`
	NodeProbeHealthyCount   int                       `json:"node_probe_healthy_count"`
	NodeProbeCheckedCount   int                       `json:"node_probe_checked_count"`
	NodeProbeStaleCount     int                       `json:"node_probe_stale_count"`
	NodeProbeHealthyPercent float64                   `json:"node_probe_healthy_percent"`
	NodeProbeAverageRTTMs   float64                   `json:"node_probe_average_rtt_ms"`
	NodeProbeMaxRTTMs       int64                     `json:"node_probe_max_rtt_ms"`
	AvailabilityPercent     float64                   `json:"availability_percent"`
	AverageLatencyMs        float64                   `json:"average_latency_ms"`
	MaxLatencyMs            int64                     `json:"max_latency_ms"`
	ErrorRatePercent        float64                   `json:"error_rate_percent"`
	Workers                 []DNSWorkerHealthItemView `json:"workers"`
}

type DNSWorkerHealthItemView struct {
	ID                       uint                       `json:"id"`
	WorkerID                 string                     `json:"worker_id"`
	Name                     string                     `json:"name"`
	Remark                   string                     `json:"remark"`
	Status                   string                     `json:"status"`
	PublicAddress            string                     `json:"public_address"`
	QueryCount               int64                      `json:"query_count"`
	ErrorQueries             int64                      `json:"error_queries"`
	ErrorRatePercent         float64                    `json:"error_rate_percent"`
	AverageLatencyMs         float64                    `json:"average_latency_ms"`
	MaxLatencyMs             int64                      `json:"max_latency_ms"`
	LastSeenAt               *time.Time                 `json:"last_seen_at"`
	LastHeartbeatAt          *time.Time                 `json:"last_heartbeat_at"`
	LastRemoteIP             string                     `json:"last_remote_ip"`
	LastRollupAt             *time.Time                 `json:"last_rollup_at"`
	LastRollupCount          int64                      `json:"last_rollup_count"`
	LastSnapshotAt           *time.Time                 `json:"last_snapshot_at"`
	SnapshotAgeSeconds       int64                      `json:"snapshot_age_seconds"`
	SnapshotStale            bool                       `json:"snapshot_stale"`
	GeoIPEnabled             bool                       `json:"geoip_enabled"`
	GeoIPDatabasePath        string                     `json:"geoip_database_path"`
	GeoIPLastError           string                     `json:"geoip_last_error"`
	ASNDatabasePath          string                     `json:"asn_database_path"`
	ASNLastError             string                     `json:"asn_last_error"`
	GeoIPDatabaseType        string                     `json:"geoip_database_type"`
	ASNDatabaseType          string                     `json:"asn_database_type"`
	GeoIPCountryEnabled      bool                       `json:"geoip_country_enabled"`
	GeoIPASNEnabled          bool                       `json:"geoip_asn_enabled"`
	GeoIPOperatorEnabled     bool                       `json:"geoip_operator_enabled"`
	OperatorCIDRDatabasePath string                     `json:"operator_cidr_database_path"`
	OperatorCIDRLastError    string                     `json:"operator_cidr_last_error"`
	UpdateRequested          bool                       `json:"update_requested"`
	UpdateChannel            string                     `json:"update_channel"`
	UpdateTag                string                     `json:"update_tag"`
	UpdateSupported          bool                       `json:"update_supported"`
	LastUpdateSupportedAt    *time.Time                 `json:"last_update_supported_at"`
	UpdateDispatchMode       string                     `json:"update_dispatch_mode"`
	UpdateDispatchMessage    string                     `json:"update_dispatch_message"`
	UpdateDispatchedAt       *time.Time                 `json:"update_dispatched_at"`
	UpdateDispatchedNodeID   string                     `json:"update_dispatched_node_id"`
	UninstallSupported       bool                       `json:"uninstall_supported"`
	LastUninstallSupportedAt *time.Time                 `json:"last_uninstall_supported_at"`
	UninstallRequested       bool                       `json:"uninstall_requested"`
	UninstallRequestedAt     *time.Time                 `json:"uninstall_requested_at"`
	LastError                string                     `json:"last_error"`
	LastProbeAt              *time.Time                 `json:"last_probe_at"`
	LastProbeResults         []DNSWorkerProbeResultView `json:"last_probe_results"`
	ProbeStatus              string                     `json:"probe_status"`
	ProbeHealthy             bool                       `json:"probe_healthy"`
	ProbeAgeSeconds          int64                      `json:"probe_age_seconds"`
	ProbeMessage             string                     `json:"probe_message"`
	NodeProbeTotalCount      int                        `json:"node_probe_total_count"`
	NodeProbeHealthyCount    int                        `json:"node_probe_healthy_count"`
	NodeProbeStaleCount      int                        `json:"node_probe_stale_count"`
	NodeProbeHealthyPercent  float64                    `json:"node_probe_healthy_percent"`
	NodeProbeAverageRTTMs    float64                    `json:"node_probe_average_rtt_ms"`
	NodeProbeMaxRTTMs        int64                      `json:"node_probe_max_rtt_ms"`
	NodeProbes               []DNSWorkerNodeProbeView   `json:"node_probes"`
}

func GetAuthoritativeDNSObservabilitySummary(input DNSObservabilitySummaryInput) (*DNSObservabilitySummaryView, error) {
	window := normalizeDNSObservabilityWindow(input.Hours)

	summary := &DNSObservabilitySummaryView{
		WindowHours: window.Hours,
		WindowStart: window.WindowStart,
		WindowEnd:   window.WindowEnd,
	}

	data, err := loadDNSObservabilitySummaryQueryData(input, window, defaultDNSObservabilitySummaryQueries)
	if err != nil {
		return nil, err
	}
	summary.LastRollupAt = data.lastRollupAt
	summary.TrendPoints = data.trendPoints

	for rcode, count := range data.rcodeCounts {
		summary.TotalQueries += count
		switch normalizeDNSRCode(rcode) {
		case "NOERROR":
			summary.SuccessfulQueries += count
		case "SERVFAIL", "REFUSED":
			summary.ErrorQueries += count
		default:
			summary.NegativeQueries += count
		}
	}
	summary.DynamicQueries = data.dynamicQueries
	if summary.TotalQueries > summary.DynamicQueries {
		summary.StaticQueries = summary.TotalQueries - summary.DynamicQueries
	}

	labels, err := loadDNSObservabilitySummaryLabelData(data.routeCounts, defaultDNSObservabilitySummaryLabelQueries)
	if err != nil {
		return nil, err
	}

	summary.RCodeBreakdown = buildDNSObservabilityCounters(data.rcodeCounts, nil, 10)
	summary.QTypeBreakdown = buildDNSObservabilityCounters(data.qtypeCounts, nil, 10)
	summary.TopQNames = buildDNSObservabilityCounters(data.qnameCounts, nil, 8)
	summary.TopTargets = buildDNSObservabilityCounters(data.targetCounts, nil, 8)
	summary.WorkerBreakdown = buildDNSObservabilityCounters(data.workerCounts, labels.workerLabels, 8)
	summary.ZoneBreakdown = buildDNSObservabilityCounters(uintCountsToStringCounts(data.zoneCounts), labels.zoneLabels, 8)
	summary.RouteBreakdown = buildDNSObservabilityCounters(uintCountsToStringCounts(data.routeCounts), labels.routeLabels, 8)
	summary.SourceScopeBreakdown = buildDNSObservabilityCounters(data.sourceScopeCounts, nil, 8)
	summary.SourceCountryBreakdown = buildDNSObservabilityCounters(data.sourceCountryCounts, nil, 8)
	summary.SourceASNBreakdown = buildDNSObservabilityCounters(uintCountsToStringCounts(data.sourceASNCounts), nil, 8)
	summary.SourceOperatorBreakdown = buildDNSObservabilityCounters(data.sourceOperatorCounts, nil, 8)
	checkedAt := time.Now().UTC()
	summary.SnapshotConsistency = buildDNSWorkerSnapshotConsistency(checkedAt)
	summary.WorkerHealth = buildDNSWorkerHealthSummary(checkedAt, data.workerHealthRollups)
	return summary, nil
}

func loadDNSObservabilitySummaryQueryData(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, queries dnsObservabilitySummaryQueries) (*dnsObservabilitySummaryQueryData, error) {
	var data *dnsObservabilitySummaryQueryData
	var lastRollupAt *time.Time
	if err := runConcurrentQueries(
		func() error {
			value, err := queries.queryRollupData(input, window)
			data = value
			return err
		},
		func() error {
			value, err := queries.queryLastRollupAt(input, window)
			lastRollupAt = value
			return err
		},
	); err != nil {
		return nil, err
	}
	if data == nil {
		data = newDNSObservabilitySummaryQueryData(window)
	}
	data.lastRollupAt = lastRollupAt
	return data, nil
}

func loadDNSObservabilitySummaryLabelData(routeCounts map[uint]int64, queries dnsObservabilitySummaryLabelQueries) (*dnsObservabilitySummaryLabelData, error) {
	data := &dnsObservabilitySummaryLabelData{}
	if err := runConcurrentQueries(
		func() error {
			labels, err := queries.dnsWorkerLabels()
			data.workerLabels = labels
			return err
		},
		func() error {
			labels, err := queries.dnsZoneLabels()
			data.zoneLabels = labels
			return err
		},
		func() error {
			labels, err := queries.dnsRouteLabels(routeCounts)
			data.routeLabels = labels
			return err
		},
	); err != nil {
		return nil, err
	}
	return data, nil
}

func persistDNSQueryRollups(workerID string, inputs []DNSQueryRollupInput) error {
	return persistDNSQueryRollupsWithDB(model.DB, &model.DNSWorker{WorkerID: workerID}, inputs)
}

type dnsQueryRollupInputSummary struct {
	count        int64
	lastRollupAt time.Time
}

func summarizeDNSQueryRollupInputs(inputs []DNSQueryRollupInput) dnsQueryRollupInputSummary {
	summary := dnsQueryRollupInputSummary{}
	for _, input := range inputs {
		if input.QueryCount <= 0 {
			continue
		}
		windowStart := input.WindowStart
		rollupEnd := windowStart.UTC().Add(time.Duration(normalizeDNSRollupWindow(input.WindowMinutes)) * time.Minute)
		summary.count += input.QueryCount
		if summary.lastRollupAt.IsZero() || rollupEnd.After(summary.lastRollupAt) {
			summary.lastRollupAt = rollupEnd
		}
	}
	return summary
}

func filterDNSQueryRollupInputsForACL(inputs []DNSQueryRollupInput, acl *dnsWorkerHeartbeatACL) []DNSQueryRollupInput {
	filtered := make([]DNSQueryRollupInput, 0, len(inputs))
	now := time.Now().UTC()
	for _, input := range inputs {
		if input.QueryCount <= 0 {
			continue
		}
		if acl != nil && acl.enforce && !acl.allowsRollup(input) {
			continue
		}
		windowStart, ok := normalizeDNSRollupWindowStart(input.WindowStart, now)
		if !ok {
			continue
		}
		input.WindowStart = windowStart
		filtered = append(filtered, input)
	}
	return filtered
}

func normalizeDNSRollupWindowStart(value time.Time, now time.Time) (time.Time, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if value.IsZero() {
		return now.Truncate(time.Minute), true
	}
	windowStart := value.UTC().Truncate(time.Minute)
	if windowStart.After(now.Add(defaultDNSRollupFutureTolerance)) {
		return time.Time{}, false
	}
	if windowStart.After(now) {
		return now.Truncate(time.Minute), true
	}
	if windowStart.Before(now.Add(-dnsRollupAcceptedHistoryWindow())) {
		return time.Time{}, false
	}
	return windowStart, true
}

func dnsRollupAcceptedHistoryWindow() time.Duration {
	retentionDays := common.DatabaseAutoCleanupRetentionDays
	if retentionDays < 1 {
		retentionDays = 30
	}
	return time.Duration(retentionDays) * 24 * time.Hour
}

func persistDNSQueryRollupsWithDB(db *gorm.DB, worker *model.DNSWorker, inputs []DNSQueryRollupInput) error {
	if db == nil {
		db = model.DB
	}
	if len(inputs) > defaultDNSMaxHeartbeatRollups {
		return fmt.Errorf("DNS Worker heartbeat rollups exceed limit %d", defaultDNSMaxHeartbeatRollups)
	}
	acl, err := buildDNSWorkerHeartbeatACL(db, worker)
	if err != nil {
		return err
	}
	return persistDNSQueryRollupsWithACL(db, worker, inputs, acl)
}

func persistDNSQueryRollupsWithACL(db *gorm.DB, worker *model.DNSWorker, inputs []DNSQueryRollupInput, acl *dnsWorkerHeartbeatACL) error {
	if db == nil {
		db = model.DB
	}
	if acl == nil {
		var err error
		acl, err = buildDNSWorkerHeartbeatACL(db, worker)
		if err != nil {
			return err
		}
	}
	inputs = limitDNSQueryRollupInputs(filterDNSQueryRollupInputsForACL(inputs, acl), defaultDNSMaxHeartbeatRollups)
	workerID := ""
	if worker != nil {
		workerID = strings.TrimSpace(worker.WorkerID)
	}
	rollups := make([]*model.DNSQueryRollup, 0, len(inputs))
	for _, input := range inputs {
		if input.QueryCount <= 0 {
			continue
		}
		targetSummary := normalizeDNSTargetSummary(input.TargetSummary)
		targetSummaryJSON, err := json.Marshal(targetSummary)
		if err != nil {
			return err
		}
		totalDurationMs, maxDurationMs := normalizeDNSRollupDurations(input.TotalDurationMs, input.MaxDurationMs)
		rollup := &model.DNSQueryRollup{
			WindowStart:     input.WindowStart,
			WindowMinutes:   normalizeDNSRollupWindow(input.WindowMinutes),
			WorkerID:        workerID,
			ZoneID:          input.ZoneID,
			ProxyRouteID:    input.ProxyRouteID,
			SourceScope:     normalizeDNSSourceScope(input.SourceScope),
			SourceCountry:   normalizeDNSRollupSourceCountry(input.SourceCountry, input.SourceScope),
			SourceASN:       normalizeDNSRollupSourceASN(input.SourceASN, input.SourceScope),
			SourceOperator:  normalizeDNSRollupSourceOperator(input.SourceOperator, input.SourceScope),
			QName:           normalizeDNSRecordName(input.QName),
			QType:           normalizeAuthoritativeDNSRecordTypeOrDefault(input.QType),
			RCode:           normalizeDNSRCode(input.RCode),
			QueryCount:      input.QueryCount,
			TotalDurationMs: totalDurationMs,
			MaxDurationMs:   maxDurationMs,
			TargetSummary:   string(targetSummaryJSON),
		}
		rollups = append(rollups, rollup)
	}
	if len(rollups) == 0 {
		return nil
	}
	return db.CreateInBatches(rollups, 500).Error
}

func limitDNSQueryRollupInputs(inputs []DNSQueryRollupInput, limit int) []DNSQueryRollupInput {
	if limit <= 0 || len(inputs) <= limit {
		return inputs
	}
	return inputs[:limit]
}

func normalizeDNSSourceScope(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultGSLBScopeKey
	}
	base, suffix, hasSuffix := strings.Cut(value, "|")
	normalizedBase := normalizeDNSSourceScopeBase(base)
	if hasSuffix {
		if bucket := normalizeDNSSourceScopeBucket(suffix); bucket != "" {
			return normalizedBase + "|" + bucket
		}
		return normalizedBase
	}
	return normalizedBase
}

func normalizeDNSSourceScopeBase(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultGSLBScopeKey
	}
	prefix, scopeValue, ok := strings.Cut(value, ":")
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "country") {
		scopeValue = strings.ToUpper(strings.TrimSpace(scopeValue))
		if len(scopeValue) == 2 {
			return "country:" + scopeValue
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "cidr") {
		if cidr, valid := normalizeGSLBCIDR(scopeValue); valid {
			return "cidr:" + cidr
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "operator") {
		if operator := normalizeGSLBOperator(scopeValue); operator != "" {
			return "operator:" + operator
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "asn") {
		asn, err := strconv.ParseUint(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(scopeValue)), "AS"), 10, 32)
		if err == nil && asn > 0 {
			return "asn:" + strconv.FormatUint(asn, 10)
		}
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func normalizeDNSSourceScopeBucket(raw string) string {
	prefix, value, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || !strings.EqualFold(strings.TrimSpace(prefix), "bucket") {
		return ""
	}
	bucket, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || bucket < 0 || bucket > 99 {
		return ""
	}
	return fmt.Sprintf("bucket:%02d", bucket)
}

func normalizeDNSRollupSourceCountry(raw string, sourceScope string) string {
	country := strings.ToUpper(strings.TrimSpace(raw))
	if len(country) == 2 {
		return country
	}
	base := dnsSourceScopeBase(sourceScope)
	if strings.HasPrefix(strings.ToLower(base), "country:") {
		value := strings.ToUpper(strings.TrimSpace(base[strings.Index(base, ":")+1:]))
		if len(value) == 2 {
			return value
		}
	}
	return ""
}

func normalizeDNSRollupSourceASN(raw uint32, sourceScope string) uint32 {
	if raw > 0 {
		return raw
	}
	base := dnsSourceScopeBase(sourceScope)
	if !strings.HasPrefix(strings.ToLower(base), "asn:") {
		return 0
	}
	value := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(base[strings.Index(base, ":")+1:])), "AS")
	asn, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(asn)
}

func normalizeDNSRollupSourceOperator(raw string, sourceScope string) string {
	if operator := normalizeGSLBOperator(raw); operator != "" {
		return operator
	}
	base := dnsSourceScopeBase(sourceScope)
	if !strings.HasPrefix(strings.ToLower(base), "operator:") {
		return ""
	}
	return normalizeGSLBOperator(base[strings.Index(base, ":")+1:])
}

func dnsSourceScopeBase(sourceScope string) string {
	base, _, _ := strings.Cut(normalizeDNSSourceScope(sourceScope), "|")
	return base
}

func decodeDNSTargetSummary(raw string) map[string]int64 {
	var result map[string]int64
	if strings.TrimSpace(raw) == "" {
		return map[string]int64{}
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]int64{}
	}
	return normalizeDNSTargetSummary(result)
}

func normalizeDNSTargetSummary(values map[string]int64) map[string]int64 {
	if len(values) == 0 {
		return map[string]int64{}
	}
	type targetCount struct {
		target string
		count  int64
	}
	counts := make(map[string]int64, len(values))
	for target, count := range values {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" || count <= 0 {
			continue
		}
		counts[trimmed] += count
	}
	items := make([]targetCount, 0, len(counts))
	for target, count := range counts {
		items = append(items, targetCount{target: target, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].target < items[j].target
		}
		return items[i].count > items[j].count
	})
	if len(items) > defaultDNSMaxRollupTargetSummary {
		items = items[:defaultDNSMaxRollupTargetSummary]
	}
	result := make(map[string]int64, len(items))
	for _, item := range items {
		result[item.target] += item.count
	}
	return result
}

func normalizeDNSDurationMs(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeDNSRollupDurations(totalDurationMs int64, maxDurationMs int64) (int64, int64) {
	total := normalizeDNSDurationMs(totalDurationMs)
	maximum := normalizeDNSDurationMs(maxDurationMs)
	if total < maximum {
		total = maximum
	}
	return total, maximum
}

func dnsWorkerLabels() (map[string]string, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string, len(workers))
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		label := strings.TrimSpace(worker.Name)
		if label == "" {
			label = worker.WorkerID
		}
		labels[worker.WorkerID] = label
	}
	return labels, nil
}

func dnsZoneLabels() (map[string]string, error) {
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string, len(zones))
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		labels[fmt.Sprint(zone.ID)] = zone.Name
	}
	return labels, nil
}

func dnsRouteLabels(counts map[uint]int64) (map[string]string, error) {
	labels := make(map[string]string, len(counts))
	routeIDs := make([]uint, 0, len(counts))
	for routeID := range counts {
		if routeID == 0 {
			continue
		}
		routeIDs = append(routeIDs, routeID)
	}
	sort.Slice(routeIDs, func(i, j int) bool {
		return routeIDs[i] < routeIDs[j]
	})
	routes, err := model.ListProxyRoutesByIDs(routeIDs)
	if err != nil {
		return nil, err
	}
	for _, route := range routes {
		if route == nil || route.ID == 0 {
			continue
		}
		label := normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain)
		if label == "" {
			label = fmt.Sprintf("Route %d", route.ID)
		}
		labels[fmt.Sprint(route.ID)] = label
	}
	return labels, nil
}

func uintCountsToStringCounts(input map[uint]int64) map[string]int64 {
	result := make(map[string]int64, len(input))
	for key, count := range input {
		if key == 0 || count <= 0 {
			continue
		}
		result[fmt.Sprint(key)] = count
	}
	return result
}

func normalizeDNSObservabilityWindow(hours int) dnsObservabilityWindow {
	if hours <= 0 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-time.Duration(hours) * time.Hour)
	return dnsObservabilityWindow{
		Hours:       hours,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		QueryStart:  windowStart.Add(-time.Duration(defaultDNSMaxRollupWindowMinutes) * time.Minute),
	}
}

func dnsObservabilityBaseQuery(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) *gorm.DB {
	query := model.DB.Model(&model.DNSQueryRollup{}).
		Where("query_count > 0").
		Where("window_start >= ? AND window_start <= ?", window.QueryStart, window.WindowEnd)
	endExpression, endArgs := dnsRollupEndAfterWindowStartCondition(window.WindowStart)
	query = query.Where(endExpression, endArgs...)
	if input.ZoneID > 0 {
		query = query.Where("zone_id = ?", input.ZoneID)
	}
	if workerID := strings.TrimSpace(input.WorkerID); workerID != "" {
		query = query.Where("worker_id = ?", workerID)
	}
	return query
}

func dnsRollupEndAfterWindowStartCondition(windowStart time.Time) (string, []any) {
	normalizedWindow := "CASE WHEN window_minutes <= 0 THEN 1 WHEN window_minutes > ? THEN ? ELSE window_minutes END"
	switch model.DB.Dialector.Name() {
	case "postgres":
		return "window_start + ((" + normalizedWindow + ") * INTERVAL '1 minute') > ?", []any{defaultDNSMaxRollupWindowMinutes, defaultDNSMaxRollupWindowMinutes, windowStart}
	default:
		return "datetime(window_start, '+' || (" + normalizedWindow + ") || ' minutes') > datetime(?)", []any{defaultDNSMaxRollupWindowMinutes, defaultDNSMaxRollupWindowMinutes, windowStart}
	}
}

func dnsRollupEndSelectExpression() string {
	normalizedWindow := "CASE WHEN window_minutes <= 0 THEN 1 WHEN window_minutes > " + strconv.Itoa(defaultDNSMaxRollupWindowMinutes) + " THEN " + strconv.Itoa(defaultDNSMaxRollupWindowMinutes) + " ELSE window_minutes END"
	switch model.DB.Dialector.Name() {
	case "postgres":
		return "window_start + ((" + normalizedWindow + ") * INTERVAL '1 minute')"
	default:
		return "datetime(window_start, '+' || (" + normalizedWindow + ") || ' minutes')"
	}
}

func dnsRollupEndHourExpression() string {
	endExpression := dnsRollupEndSelectExpression()
	switch model.DB.Dialector.Name() {
	case "postgres":
		return "to_char(date_trunc('hour', " + endExpression + "), 'YYYY-MM-DD HH24:MI:SS')"
	default:
		return "strftime('%Y-%m-%d %H:00:00', " + endExpression + ")"
	}
}

func dnsRollupEndMaxSelectExpression() string {
	endExpression := dnsRollupEndSelectExpression()
	switch model.DB.Dialector.Name() {
	case "postgres":
		return "to_char(MAX(" + endExpression + "), 'YYYY-MM-DD HH24:MI:SS')"
	default:
		return "MAX(" + endExpression + ")"
	}
}

func newDNSObservabilitySummaryQueryData(window dnsObservabilityWindow) *dnsObservabilitySummaryQueryData {
	return &dnsObservabilitySummaryQueryData{
		rcodeCounts:          map[string]int64{},
		qtypeCounts:          map[string]int64{},
		qnameCounts:          map[string]int64{},
		workerCounts:         map[string]int64{},
		sourceScopeCounts:    map[string]int64{},
		sourceCountryCounts:  map[string]int64{},
		sourceASNCounts:      map[uint]int64{},
		sourceOperatorCounts: map[string]int64{},
		zoneCounts:           map[uint]int64{},
		routeCounts:          map[uint]int64{},
		targetCounts:         map[string]int64{},
		trendPoints:          initDNSObservabilityTrendPoints(window.WindowStart, window.WindowEnd, window.Hours),
		workerHealthRollups:  []dnsWorkerHealthRollupRow{},
	}
}

func queryDNSObservabilityAggregatedData(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) (*dnsObservabilitySummaryQueryData, error) {
	var rcodeCounts map[string]int64
	var qtypeCounts map[string]int64
	var qnameCounts map[string]int64
	var workerCounts map[string]int64
	var sourceScopeCounts map[string]int64
	var sourceCountryCounts map[string]int64
	var sourceASNCounts map[uint]int64
	var sourceOperatorCounts map[string]int64
	var zoneCounts map[uint]int64
	var routeCounts map[uint]int64
	var targetCounts map[string]int64
	var dynamicQueries int64
	var trendPoints []DNSObservabilityTrendPointView
	var workerHealthRollups []dnsWorkerHealthRollupRow
	if err := runConcurrentQueries(
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "rcode", 0)
			rcodeCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "qtype", 10)
			qtypeCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "qname", 8)
			qnameCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "worker_id", 8)
			workerCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "source_scope", 8)
			sourceScopeCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "source_country", 8)
			sourceCountryCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityUintCounts(input, window, "source_asn", 8, "source_asn > 0")
			sourceASNCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityStringCounts(input, window, "source_operator", 8)
			sourceOperatorCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityUintCounts(input, window, "zone_id", 8, "zone_id > 0")
			zoneCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityUintCounts(input, window, "proxy_route_id", 8, "proxy_route_id > 0")
			routeCounts = values
			return err
		},
		func() error {
			values, err := queryDNSObservabilityTopTargets(input, window, 8)
			targetCounts = values
			return err
		},
		func() error {
			value, err := queryDNSObservabilityDynamicQueries(input, window)
			dynamicQueries = value
			return err
		},
		func() error {
			values, err := queryDNSObservabilityTrendPoints(input, window)
			trendPoints = values
			return err
		},
		func() error {
			values, err := queryDNSWorkerHealthRollups(input, window)
			workerHealthRollups = values
			return err
		},
	); err != nil {
		return nil, err
	}
	data := newDNSObservabilitySummaryQueryData(window)
	data.rcodeCounts = rcodeCounts
	data.qtypeCounts = qtypeCounts
	data.qnameCounts = qnameCounts
	data.workerCounts = workerCounts
	data.sourceScopeCounts = sourceScopeCounts
	data.sourceCountryCounts = sourceCountryCounts
	data.sourceASNCounts = sourceASNCounts
	data.sourceOperatorCounts = sourceOperatorCounts
	data.zoneCounts = zoneCounts
	data.routeCounts = routeCounts
	data.targetCounts = targetCounts
	data.dynamicQueries = dynamicQueries
	if trendPoints != nil {
		data.trendPoints = trendPoints
	}
	data.workerHealthRollups = workerHealthRollups
	return data, nil
}

func dnsObservabilityErrorQueries(rcode string, count int64) int64 {
	switch normalizeDNSRCode(rcode) {
	case "SERVFAIL", "REFUSED":
		return count
	default:
		return 0
	}
}

func queryDNSObservabilityStringCounts(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, field string, limit int) (map[string]int64, error) {
	expression := dnsObservabilityStringCountExpression(field)
	if expression == "" {
		return map[string]int64{}, nil
	}
	var rows []dnsObservabilityStringCountRow
	query := dnsObservabilityBaseQuery(input, window).
		Select(expression + " AS key, SUM(query_count) AS count").
		Group("key").
		Order("count DESC").
		Order("key ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		key := normalizeDNSObservabilityStringCountKey(field, row.Key)
		if key == "" || row.Count <= 0 {
			continue
		}
		result[key] += row.Count
	}
	return result, nil
}

func dnsObservabilityStringCountExpression(field string) string {
	switch field {
	case "rcode":
		return "COALESCE(NULLIF(TRIM(r_code), ''), 'NOERROR')"
	case "qtype":
		return "COALESCE(NULLIF(TRIM(q_type), ''), 'A')"
	case "qname":
		return "COALESCE(NULLIF(TRIM(q_name), ''), 'unknown')"
	case "worker_id":
		return "COALESCE(NULLIF(TRIM(worker_id), ''), '')"
	case "source_scope":
		return "COALESCE(NULLIF(TRIM(source_scope), ''), '" + defaultGSLBScopeKey + "')"
	case "source_country":
		return "COALESCE(NULLIF(TRIM(source_country), ''), '')"
	case "source_operator":
		return "COALESCE(NULLIF(TRIM(source_operator), ''), '')"
	default:
		return ""
	}
}

func queryDNSObservabilityDynamicQueries(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) (int64, error) {
	var total int64
	if err := dnsObservabilityBaseQuery(input, window).
		Select("COALESCE(SUM(query_count), 0)").
		Where("proxy_route_id > 0").
		Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func normalizeDNSObservabilityStringCountKey(field string, value string) string {
	value = strings.TrimSpace(value)
	switch field {
	case "rcode":
		return normalizeDNSRCode(value)
	case "qtype":
		return normalizeAuthoritativeDNSRecordTypeOrDefault(value)
	case "qname":
		if normalized := normalizeDNSRecordName(value); normalized != "" {
			return normalized
		}
		return "unknown"
	case "worker_id":
		return value
	case "source_scope":
		return normalizeDNSSourceScope(value)
	case "source_country":
		return normalizeDNSRollupSourceCountry(value, "")
	case "source_operator":
		return normalizeDNSRollupSourceOperator(value, "")
	default:
		return value
	}
}

func queryDNSObservabilityUintCounts(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, field string, limit int, nonZeroClause string) (map[uint]int64, error) {
	if field != "zone_id" && field != "proxy_route_id" && field != "source_asn" {
		return map[uint]int64{}, nil
	}
	var rows []dnsObservabilityUintCountRow
	query := dnsObservabilityBaseQuery(input, window).
		Select(field + " AS key, SUM(query_count) AS count").
		Where(nonZeroClause).
		Group(field).
		Order("count DESC").
		Order(field + " ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		if row.Key == 0 || row.Count <= 0 {
			continue
		}
		result[row.Key] += row.Count
	}
	return result, nil
}

func queryDNSObservabilityTopTargets(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, limit int) (map[string]int64, error) {
	var rows []dnsObservabilityTargetSummaryRow
	if err := dnsObservabilityBaseQuery(input, window).
		Select("target_summary").
		Where("target_summary IS NOT NULL AND TRIM(target_summary) <> '' AND TRIM(target_summary) <> '{}'").
		Order("window_start DESC").
		Order("id DESC").
		Limit(normalizeDNSObservabilityHeavyCounterScanLimit()).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, row := range rows {
		for target, targetCount := range decodeDNSTargetSummary(row.TargetSummary) {
			if targetCount <= 0 {
				continue
			}
			counts[target] += targetCount
		}
	}
	return limitDNSObservabilityCounterMap(counts, limit), nil
}

func limitDNSObservabilityCounterMap(counts map[string]int64, limit int) map[string]int64 {
	if limit <= 0 || len(counts) <= limit {
		return counts
	}
	items := buildDNSObservabilityCounters(counts, nil, limit)
	limited := make(map[string]int64, len(items))
	for _, item := range items {
		limited[item.Key] = item.Count
	}
	return limited
}

func limitDNSObservabilityUintCounterMap(counts map[uint]int64, limit int) map[uint]int64 {
	if limit <= 0 || len(counts) <= limit {
		return counts
	}
	items := make([]struct {
		key   uint
		count int64
	}, 0, len(counts))
	for key, count := range counts {
		if key == 0 || count <= 0 {
			continue
		}
		items = append(items, struct {
			key   uint
			count int64
		}{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].key < items[j].key
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	limited := make(map[uint]int64, len(items))
	for _, item := range items {
		limited[item.key] = item.count
	}
	return limited
}

func queryDNSObservabilityTrendPoints(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) ([]DNSObservabilityTrendPointView, error) {
	trendPoints := initDNSObservabilityTrendPoints(window.WindowStart, window.WindowEnd, window.Hours)
	bucketExpression := dnsRollupEndHourExpression()
	rcodeExpression := "COALESCE(NULLIF(TRIM(r_code), ''), 'NOERROR')"
	dynamicExpression := "CASE WHEN proxy_route_id > 0 THEN 1 ELSE 0 END"
	var rows []dnsObservabilityTrendRow
	if err := dnsObservabilityBaseQuery(input, window).
		Select(bucketExpression + " AS bucket, " + rcodeExpression + " AS r_code, " + dynamicExpression + " AS dynamic, SUM(query_count) AS count").
		Group(bucketExpression).
		Group(rcodeExpression).
		Group(dynamicExpression).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Count <= 0 {
			continue
		}
		bucket, ok := parseDNSObservabilityTime(row.Bucket)
		if !ok {
			continue
		}
		applyDNSObservabilityTrendPoint(trendPoints, bucket, normalizeDNSRCode(row.RCode), row.Dynamic > 0, row.Count)
	}
	return trendPoints, nil
}

func queryDNSWorkerHealthRollups(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) ([]dnsWorkerHealthRollupRow, error) {
	var rows []dnsWorkerHealthRollupRow
	if err := dnsObservabilityBaseQuery(input, window).
		Select("worker_id, SUM(query_count) AS query_count, SUM(CASE WHEN r_code IN ('SERVFAIL', 'REFUSED') THEN query_count ELSE 0 END) AS error_queries, SUM(total_duration_ms) AS total_duration_ms, MAX(max_duration_ms) AS max_duration_ms").
		Where("TRIM(worker_id) <> ''").
		Group("worker_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func queryDNSObservabilityLastRollupAt(input DNSObservabilitySummaryInput, window dnsObservabilityWindow) (*time.Time, error) {
	var rows []dnsObservabilityLastRollupRow
	if err := dnsObservabilityBaseQuery(input, window).
		Select("window_start, window_minutes").
		Order("window_start DESC").
		Order("id DESC").
		Limit(1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	value := rows[0].WindowStart.UTC().Add(time.Duration(normalizeDNSRollupWindow(rows[0].WindowMinutes)) * time.Minute)
	return &value, nil
}

func normalizeDNSObservabilityHeavyCounterScanLimit() int {
	if dnsObservabilityHeavyCounterScanLimit <= 0 {
		return 20000
	}
	return dnsObservabilityHeavyCounterScanLimit
}

func parseDNSObservabilityTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func buildDNSObservabilityCounters(counts map[string]int64, labels map[string]string, limit int) []DNSObservabilityCounterView {
	items := make([]DNSObservabilityCounterView, 0, len(counts))
	for key, count := range counts {
		key = strings.TrimSpace(key)
		if key == "" || count <= 0 {
			continue
		}
		label := key
		if labels != nil {
			if value := strings.TrimSpace(labels[key]); value != "" {
				label = value
			}
		}
		items = append(items, DNSObservabilityCounterView{
			Key:   key,
			Label: label,
			Count: count,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Key < items[j].Key
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func initDNSObservabilityTrendPoints(windowStart time.Time, windowEnd time.Time, hours int) []DNSObservabilityTrendPointView {
	if hours <= 0 {
		hours = 24
	}
	start := windowStart.UTC().Truncate(time.Hour)
	end := windowEnd.UTC().Truncate(time.Hour)
	points := make([]DNSObservabilityTrendPointView, 0, hours+1)
	for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Hour) {
		points = append(points, DNSObservabilityTrendPointView{
			BucketStartedAt: bucket,
		})
	}
	return points
}

func applyDNSObservabilityTrendPoint(points []DNSObservabilityTrendPointView, bucketTime time.Time, rcode string, dynamic bool, count int64) {
	if count <= 0 || len(points) == 0 {
		return
	}
	bucket := bucketTime.UTC().Truncate(time.Hour)
	base := points[0].BucketStartedAt.UTC()
	index := int(bucket.Sub(base) / time.Hour)
	if index < 0 && bucket.Equal(base.Add(-time.Hour)) {
		index = 0
	}
	if index < 0 || index >= len(points) {
		return
	}
	points[index].QueryCount += count
	switch rcode {
	case "NOERROR":
		points[index].SuccessfulQueries += count
		points[index].NoErrorQueries += count
	case "SERVFAIL", "REFUSED":
		points[index].ErrorQueries += count
		if rcode == "SERVFAIL" {
			points[index].ServfailQueries += count
		}
	default:
		points[index].NegativeQueries += count
		if rcode == "NXDOMAIN" {
			points[index].NXDomainQueries += count
		}
	}
	if dynamic {
		points[index].DynamicQueries += count
	} else {
		points[index].StaticQueries += count
	}
}

func buildDNSWorkerSnapshotConsistency(now time.Time) DNSWorkerSnapshotConsistencyView {
	snapshotMaxAge := authoritativeDNSSnapshotMaxAge()
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return DNSWorkerSnapshotConsistencyView{
			Status:                dnsSnapshotUnknown,
			CheckedAt:             now,
			SnapshotMaxAgeSeconds: int64(snapshotMaxAge.Seconds()),
		}
	}
	view := DNSWorkerSnapshotConsistencyView{
		Status:                dnsSnapshotNoOnline,
		CheckedAt:             now,
		SnapshotMaxAgeSeconds: int64(snapshotMaxAge.Seconds()),
		TotalWorkerCount:      len(workers),
		Workers:               make([]DNSWorkerSnapshotWorkerView, 0, len(workers)),
	}
	versionGroups := map[string]*DNSWorkerSnapshotVersionView{}
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		status := normalizeDNSWorkerStatus(worker.Status)
		snapshotVersion := strings.TrimSpace(worker.LastSnapshotVersion)
		snapshotAt := normalizeDNSWorkerSnapshotAt(worker.LastSnapshotAt, now, worker.UpdatedAt, worker.CreatedAt)
		stale := status == dnsWorkerStatusOnline && (snapshotAt == nil || now.Sub(snapshotAt.UTC()) > snapshotMaxAge)
		workerName := strings.TrimSpace(worker.Name)
		if workerName == "" {
			workerName = worker.WorkerID
		}
		item := DNSWorkerSnapshotWorkerView{
			WorkerID:              worker.WorkerID,
			Name:                  workerName,
			Status:                status,
			SnapshotVersion:       snapshotVersion,
			LastSnapshotAt:        snapshotAt,
			LastSeenAt:            worker.LastSeenAt,
			LastHeartbeatAt:       worker.LastHeartbeatAt,
			LastRollupAt:          worker.LastRollupAt,
			LastRollupCount:       worker.LastRollupCount,
			Stale:                 stale,
			GeoIPEnabled:          worker.GeoIPEnabled,
			GeoIPLastError:        worker.GeoIPLastError,
			ASNLastError:          worker.ASNLastError,
			GeoIPCountryEnabled:   worker.GeoIPCountryEnabled,
			GeoIPASNEnabled:       worker.GeoIPASNEnabled,
			GeoIPOperatorEnabled:  worker.GeoIPOperatorEnabled,
			OperatorCIDRLastError: worker.OperatorCIDRLastError,
		}
		view.Workers = append(view.Workers, item)
		if status != dnsWorkerStatusOnline {
			continue
		}
		view.OnlineWorkerCount++
		if stale {
			view.StaleWorkerCount++
		}
		versionKey := snapshotVersion
		if versionKey == "" {
			versionKey = "(empty)"
		}
		group := versionGroups[versionKey]
		if group == nil {
			group = &DNSWorkerSnapshotVersionView{
				Version: versionKey,
				Workers: make([]string, 0),
			}
			versionGroups[versionKey] = group
		}
		group.WorkerCount++
		group.Workers = append(group.Workers, workerName)
		if snapshotAt != nil && (group.LatestSnapshotAt == nil || snapshotAt.After(*group.LatestSnapshotAt)) {
			latest := *snapshotAt
			group.LatestSnapshotAt = &latest
		}
		if snapshotVersion != "" && snapshotAt != nil && (view.LatestSnapshotAt == nil || snapshotAt.After(*view.LatestSnapshotAt)) {
			latest := *snapshotAt
			view.LatestSnapshotAt = &latest
			view.LatestSnapshotVersion = snapshotVersion
		}
	}
	for _, group := range versionGroups {
		sort.Strings(group.Workers)
		view.VersionBreakdown = append(view.VersionBreakdown, *group)
	}
	sort.SliceStable(view.VersionBreakdown, func(i, j int) bool {
		if view.VersionBreakdown[i].WorkerCount != view.VersionBreakdown[j].WorkerCount {
			return view.VersionBreakdown[i].WorkerCount > view.VersionBreakdown[j].WorkerCount
		}
		return view.VersionBreakdown[i].Version < view.VersionBreakdown[j].Version
	})
	sort.SliceStable(view.Workers, func(i, j int) bool {
		return view.Workers[i].WorkerID < view.Workers[j].WorkerID
	})
	if view.OnlineWorkerCount == 0 {
		view.Status = dnsSnapshotNoOnline
		return view
	}
	if view.StaleWorkerCount > 0 {
		view.Status = dnsSnapshotStale
	} else if len(view.VersionBreakdown) > 1 {
		view.Status = dnsSnapshotDivergent
	} else {
		view.Status = dnsSnapshotConsistent
	}
	if len(view.VersionBreakdown) > 1 {
		largest := view.VersionBreakdown[0].WorkerCount
		view.DivergentWorkerCount = view.OnlineWorkerCount - largest
	}
	return view
}

type dnsWorkerHealthStats struct {
	queryCount      int64
	errorQueries    int64
	totalDurationMs int64
	maxDurationMs   int64
}

func buildDNSWorkerHealthSummary(now time.Time, rollups []dnsWorkerHealthRollupRow) DNSWorkerHealthSummaryView {
	snapshotMaxAge := authoritativeDNSSnapshotMaxAge()
	workers, err := model.ListDNSWorkers()
	view := DNSWorkerHealthSummaryView{
		CheckedAt: now,
		Workers:   []DNSWorkerHealthItemView{},
	}
	if err != nil {
		return view
	}

	statsByWorker := map[string]*dnsWorkerHealthStats{}
	currentWorkerIDs := make(map[string]struct{}, len(workers))
	workerIDs := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		if worker.UninstallRequested {
			continue
		}
		workerID := strings.TrimSpace(worker.WorkerID)
		if workerID == "" {
			continue
		}
		currentWorkerIDs[workerID] = struct{}{}
		workerIDs = append(workerIDs, workerID)
	}
	var totalQueries int64
	var totalErrors int64
	var totalDurationMs int64
	var maxDurationMs int64
	for _, rollup := range rollups {
		workerID := strings.TrimSpace(rollup.WorkerID)
		if workerID == "" || rollup.QueryCount <= 0 {
			continue
		}
		if _, ok := currentWorkerIDs[workerID]; !ok {
			continue
		}
		stats := statsByWorker[workerID]
		if stats == nil {
			stats = &dnsWorkerHealthStats{}
			statsByWorker[workerID] = stats
		}
		count := rollup.QueryCount
		errorCount := rollup.ErrorQueries
		if errorCount < 0 {
			errorCount = 0
		}
		if errorCount > count {
			errorCount = count
		}
		stats.queryCount += count
		stats.errorQueries += errorCount
		totalQueries += count
		totalErrors += errorCount
		durationMs, rollupMaxDurationMs := normalizeDNSRollupDurations(rollup.TotalDurationMs, rollup.MaxDurationMs)
		stats.totalDurationMs += durationMs
		totalDurationMs += durationMs
		if rollupMaxDurationMs > stats.maxDurationMs {
			stats.maxDurationMs = rollupMaxDurationMs
		}
		if rollupMaxDurationMs > maxDurationMs {
			maxDurationMs = rollupMaxDurationMs
		}
	}

	nodeProbeStatsByWorker := buildDNSWorkerNodeProbeStatsForWorkerIDs(now, workerIDs)
	view.TotalWorkerCount = len(workers)
	view.MaxLatencyMs = maxDurationMs
	view.AverageLatencyMs = averageMilliseconds(totalDurationMs, totalQueries)
	view.ErrorRatePercent = ratioPercent(totalErrors, totalQueries)

	var totalNodeProbeAverageRTTMs float64
	var totalNodeProbeAverageSamples int
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		status := normalizeDNSWorkerStatus(worker.Status)
		if status == dnsWorkerStatusOnline {
			view.OnlineWorkerCount++
		}
		workerName := strings.TrimSpace(worker.Name)
		if workerName == "" {
			workerName = worker.WorkerID
		}
		stats := statsByWorker[worker.WorkerID]
		if stats == nil {
			stats = &dnsWorkerHealthStats{}
		}
		snapshotAt := normalizeDNSWorkerSnapshotAt(worker.LastSnapshotAt, now, worker.UpdatedAt, worker.CreatedAt)
		snapshotAgeSeconds := int64(0)
		if snapshotAt != nil {
			age := now.Sub(snapshotAt.UTC())
			if age > 0 {
				snapshotAgeSeconds = int64(age.Seconds())
			}
		}
		snapshotStale := status == dnsWorkerStatusOnline && (snapshotAt == nil || now.Sub(snapshotAt.UTC()) > snapshotMaxAge)
		probeResults := decodeDNSWorkerProbeResults(worker.LastProbeResult)
		probeAt := normalizeDNSWorkerProbeAt(worker.LastProbeAt, now, worker.UpdatedAt, worker.CreatedAt)
		probeState := evaluateDNSWorkerProbeState(now, probeAt, probeResults)
		if probeState.status != dnsWorkerProbeUnknown {
			view.ProbeCheckedCount++
		}
		if probeState.healthy {
			view.ProbeHealthyCount++
		}
		nodeProbeStats := nodeProbeStatsByWorker[worker.WorkerID]
		if nodeProbeStats == nil {
			nodeProbeStats = &dnsWorkerNodeProbeStats{probes: []DNSWorkerNodeProbeView{}}
		}
		view.NodeProbeCheckedCount += nodeProbeStats.totalCount
		view.NodeProbeHealthyCount += nodeProbeStats.healthyCount
		view.NodeProbeStaleCount += nodeProbeStats.staleCount
		if nodeProbeStats.averageSamples > 0 {
			totalNodeProbeAverageRTTMs += nodeProbeStats.totalAverageRTTMs
			totalNodeProbeAverageSamples += nodeProbeStats.averageSamples
		}
		if nodeProbeStats.maxRTTMs > view.NodeProbeMaxRTTMs {
			view.NodeProbeMaxRTTMs = nodeProbeStats.maxRTTMs
		}
		view.Workers = append(view.Workers, DNSWorkerHealthItemView{
			ID:                       worker.ID,
			WorkerID:                 worker.WorkerID,
			Name:                     workerName,
			Remark:                   worker.Remark,
			Status:                   status,
			PublicAddress:            worker.PublicAddress,
			QueryCount:               stats.queryCount,
			ErrorQueries:             stats.errorQueries,
			ErrorRatePercent:         ratioPercent(stats.errorQueries, stats.queryCount),
			AverageLatencyMs:         averageMilliseconds(stats.totalDurationMs, stats.queryCount),
			MaxLatencyMs:             stats.maxDurationMs,
			LastSeenAt:               worker.LastSeenAt,
			LastHeartbeatAt:          worker.LastHeartbeatAt,
			LastRemoteIP:             worker.LastRemoteIP,
			LastRollupAt:             worker.LastRollupAt,
			LastRollupCount:          worker.LastRollupCount,
			LastSnapshotAt:           snapshotAt,
			SnapshotAgeSeconds:       snapshotAgeSeconds,
			SnapshotStale:            snapshotStale,
			GeoIPEnabled:             worker.GeoIPEnabled,
			GeoIPDatabasePath:        worker.GeoIPDatabasePath,
			GeoIPLastError:           worker.GeoIPLastError,
			ASNDatabasePath:          worker.ASNDatabasePath,
			ASNLastError:             worker.ASNLastError,
			GeoIPDatabaseType:        worker.GeoIPDatabaseType,
			ASNDatabaseType:          worker.ASNDatabaseType,
			GeoIPCountryEnabled:      worker.GeoIPCountryEnabled,
			GeoIPASNEnabled:          worker.GeoIPASNEnabled,
			GeoIPOperatorEnabled:     worker.GeoIPOperatorEnabled,
			OperatorCIDRDatabasePath: worker.OperatorCIDRDatabasePath,
			OperatorCIDRLastError:    worker.OperatorCIDRLastError,
			UpdateRequested:          worker.UpdateRequested,
			UpdateChannel:            normalizeReleaseChannel(worker.UpdateChannel).String(),
			UpdateTag:                worker.UpdateTag,
			UpdateSupported:          worker.UpdateSupported,
			LastUpdateSupportedAt:    worker.LastUpdateSupportedAt,
			UpdateDispatchMode:       worker.UpdateDispatchMode,
			UpdateDispatchMessage:    worker.UpdateDispatchMessage,
			UpdateDispatchedAt:       worker.UpdateDispatchedAt,
			UpdateDispatchedNodeID:   worker.UpdateDispatchedNodeID,
			UninstallSupported:       worker.UninstallSupported,
			LastUninstallSupportedAt: worker.LastUninstallSupportedAt,
			UninstallRequested:       worker.UninstallRequested,
			UninstallRequestedAt:     worker.UninstallRequestedAt,
			LastError:                worker.LastError,
			LastProbeAt:              probeAt,
			LastProbeResults:         probeResults,
			ProbeStatus:              probeState.status,
			ProbeHealthy:             probeState.healthy,
			ProbeAgeSeconds:          probeState.ageSeconds,
			ProbeMessage:             probeState.message,
			NodeProbeTotalCount:      nodeProbeStats.totalCount,
			NodeProbeHealthyCount:    nodeProbeStats.healthyCount,
			NodeProbeStaleCount:      nodeProbeStats.staleCount,
			NodeProbeHealthyPercent:  ratioPercent(int64(nodeProbeStats.healthyCount), int64(nodeProbeStats.totalCount)),
			NodeProbeAverageRTTMs:    averageFloat(nodeProbeStats.totalAverageRTTMs, nodeProbeStats.averageSamples),
			NodeProbeMaxRTTMs:        nodeProbeStats.maxRTTMs,
			NodeProbes:               nodeProbeStats.probes,
		})
	}
	if view.TotalWorkerCount > 0 {
		view.AvailabilityPercent = ratioPercent(int64(view.OnlineWorkerCount), int64(view.TotalWorkerCount))
	}
	if view.ProbeCheckedCount > 0 {
		view.ProbeHealthyPercent = ratioPercent(int64(view.ProbeHealthyCount), int64(view.ProbeCheckedCount))
	}
	if view.NodeProbeCheckedCount > 0 {
		view.NodeProbeHealthyPercent = ratioPercent(int64(view.NodeProbeHealthyCount), int64(view.NodeProbeCheckedCount))
	}
	if totalNodeProbeAverageSamples > 0 {
		view.NodeProbeAverageRTTMs = totalNodeProbeAverageRTTMs / float64(totalNodeProbeAverageSamples)
	}
	sort.SliceStable(view.Workers, func(i, j int) bool {
		if view.Workers[i].Status != view.Workers[j].Status {
			return view.Workers[i].Status == dnsWorkerStatusOnline
		}
		if view.Workers[i].QueryCount != view.Workers[j].QueryCount {
			return view.Workers[i].QueryCount > view.Workers[j].QueryCount
		}
		return view.Workers[i].WorkerID < view.Workers[j].WorkerID
	})
	return view
}

func averageMilliseconds(totalDurationMs int64, count int64) float64 {
	if count <= 0 || totalDurationMs <= 0 {
		return 0
	}
	return float64(totalDurationMs) / float64(count)
}

func averageFloat(total float64, count int) float64 {
	if count <= 0 || total <= 0 {
		return 0
	}
	return total / float64(count)
}

func ratioPercent(numerator int64, denominator int64) float64 {
	if denominator <= 0 || numerator <= 0 {
		return 0
	}
	return (float64(numerator) / float64(denominator)) * 100
}
