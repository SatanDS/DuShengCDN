package service

import (
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	NodeHealthEventStatusActive   = "active"
	NodeHealthEventStatusResolved = "resolved"
	NodeHealthSeverityInfo        = "info"
	NodeHealthSeverityWarning     = "warning"
	NodeHealthSeverityCritical    = "critical"
	nodeAccessLogRetentionWindow  = nodeAccessLogRetentionDays * 24 * time.Hour
	nodeAccessLogPathMaxLength    = 100
)

type AgentNodeSystemProfile struct {
	Hostname         string `json:"hostname"`
	OSName           string `json:"os_name"`
	OSVersion        string `json:"os_version"`
	KernelVersion    string `json:"kernel_version"`
	Architecture     string `json:"architecture"`
	CPUModel         string `json:"cpu_model"`
	CPUCores         int    `json:"cpu_cores"`
	TotalMemoryBytes int64  `json:"total_memory_bytes"`
	TotalDiskBytes   int64  `json:"total_disk_bytes"`
	UptimeSeconds    int64  `json:"uptime_seconds"`
	ReportedAtUnix   int64  `json:"reported_at_unix"`
}

type AgentNodeMetricSnapshot struct {
	CapturedAtUnix       int64   `json:"captured_at_unix"`
	CPUUsagePercent      float64 `json:"cpu_usage_percent"`
	MemoryUsedBytes      int64   `json:"memory_used_bytes"`
	MemoryTotalBytes     int64   `json:"memory_total_bytes"`
	StorageUsedBytes     int64   `json:"storage_used_bytes"`
	StorageTotalBytes    int64   `json:"storage_total_bytes"`
	DiskReadBytes        int64   `json:"disk_read_bytes"`
	DiskWriteBytes       int64   `json:"disk_write_bytes"`
	NetworkRxBytes       int64   `json:"network_rx_bytes"`
	NetworkTxBytes       int64   `json:"network_tx_bytes"`
	OpenrestyRxBytes     int64   `json:"openresty_rx_bytes"`
	OpenrestyTxBytes     int64   `json:"openresty_tx_bytes"`
	OpenrestyConnections int64   `json:"openresty_connections"`
}

type AgentNodeTrafficReport struct {
	WindowStartedAtUnix int64            `json:"window_started_at_unix"`
	WindowEndedAtUnix   int64            `json:"window_ended_at_unix"`
	RequestCount        int64            `json:"request_count"`
	ErrorCount          int64            `json:"error_count"`
	CacheHitCount       int64            `json:"cache_hit_count"`
	CacheMissCount      int64            `json:"cache_miss_count"`
	CacheBypassCount    int64            `json:"cache_bypass_count"`
	CacheExpiredCount   int64            `json:"cache_expired_count"`
	CacheStaleCount     int64            `json:"cache_stale_count"`
	UpstreamErrorCount  int64            `json:"upstream_error_count"`
	UpstreamResponseMS  int64            `json:"upstream_response_ms"`
	UniqueVisitorCount  int64            `json:"unique_visitor_count"`
	StatusCodes         map[string]int64 `json:"status_codes"`
	TopDomains          map[string]int64 `json:"top_domains"`
	SourceCountries     map[string]int64 `json:"source_countries"`
}

type AgentNodeAccessLog struct {
	LoggedAtUnix  int64  `json:"logged_at_unix"`
	RemoteAddr    string `json:"remote_addr"`
	Host          string `json:"host"`
	Path          string `json:"path"`
	StatusCode    int    `json:"status_code"`
	Reason        string `json:"reason,omitempty"`
	CacheStatus   string `json:"cache_status,omitempty"`
	RequestBytes  int64  `json:"request_bytes"`
	ResponseBytes int64  `json:"response_bytes"`
	UpstreamBytes int64  `json:"upstream_bytes"`
}

type AgentBufferedObservabilityRecord struct {
	WindowStartedAtUnix int64                    `json:"window_started_at_unix"`
	Snapshot            *AgentNodeMetricSnapshot `json:"snapshot,omitempty"`
	TrafficReport       *AgentNodeTrafficReport  `json:"traffic_report,omitempty"`
	AccessLogs          []AgentNodeAccessLog     `json:"access_logs,omitempty"`
}

type AgentNodeHealthEvent struct {
	EventType       string            `json:"event_type"`
	Severity        string            `json:"severity"`
	Message         string            `json:"message"`
	TriggeredAtUnix int64             `json:"triggered_at_unix"`
	Metadata        map[string]string `json:"metadata"`
}

type AgentOriginHealthReport struct {
	RouteID       uint   `json:"route_id"`
	OriginURL     string `json:"origin_url"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	LastError     string `json:"last_error,omitempty"`
	CheckedAtUnix int64  `json:"checked_at_unix"`
}

type AgentDNSProbeReport struct {
	WorkerID      string                `json:"worker_id"`
	Name          string                `json:"name"`
	PublicAddress string                `json:"public_address"`
	QueryName     string                `json:"query_name"`
	QueryType     string                `json:"query_type"`
	CheckedAtUnix int64                 `json:"checked_at_unix"`
	Results       []AgentDNSProbeResult `json:"results"`
}

type AgentDNSProbeResult struct {
	Network     string `json:"network"`
	Reachable   bool   `json:"reachable"`
	DurationMs  int64  `json:"duration_ms"`
	RCode       string `json:"rcode"`
	AnswerCount int    `json:"answer_count"`
	Error       string `json:"error,omitempty"`
}

func persistHeartbeatObservability(nodeID string, payload AgentNodePayload, reportedAt time.Time, dnsProbeTargets ...[]AgentDNSProbeTarget) {
	if strings.TrimSpace(nodeID) == "" {
		return
	}
	if payload.Profile == nil &&
		payload.Snapshot == nil &&
		payload.TrafficReport == nil &&
		len(payload.AccessLogs) == 0 &&
		len(payload.BufferedObservability) == 0 &&
		payload.HealthEvents == nil &&
		len(payload.OriginHealthReports) == 0 &&
		len(payload.DNSProbeResults) == 0 {
		return
	}

	accessLogs := append([]AgentNodeAccessLog(nil), payload.AccessLogs...)
	bufferedRecords := append([]AgentBufferedObservabilityRecord(nil), payload.BufferedObservability...)
	allowedDNSProbeTargets, enforceDNSProbeTargets := agentDNSProbeTargetsByWorkerID(dnsProbeTargets...)
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := persistNodeSystemProfile(tx, nodeID, payload.Profile, reportedAt); err != nil {
			return err
		}
		if err := persistBufferedObservability(tx, nodeID, bufferedRecords, reportedAt); err != nil {
			return err
		}
		if err := persistNodeMetricSnapshot(tx, nodeID, payload.Snapshot, reportedAt); err != nil {
			return err
		}
		if err := persistNodeTrafficReport(tx, nodeID, payload.TrafficReport, reportedAt); err != nil {
			return err
		}
		if payload.HealthEvents != nil {
			if err := reconcileNodeHealthEvents(tx, nodeID, payload.HealthEvents, reportedAt); err != nil {
				return err
			}
		}
		if err := persistAgentOriginHealthReports(tx, nodeID, payload.OriginHealthReports, reportedAt); err != nil {
			return err
		}
		if err := persistAgentDNSProbeReports(tx, nodeID, payload.DNSProbeResults, reportedAt, allowedDNSProbeTargets, enforceDNSProbeTargets); err != nil {
			return err
		}
		return nil
	}); err != nil {
		slog.Error("persist heartbeat observability failed", "node_id", nodeID, "error", err)
	}
	persistAccessLogsBestEffort(nodeID, accessLogs, bufferedRecords, reportedAt)
}

func ReportAgentOriginHealth(nodeID string, reports []AgentOriginHealthReport, reportedAt time.Time) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("node_id 涓嶈兘涓虹┖")
	}
	if reportedAt.IsZero() {
		reportedAt = time.Now().UTC()
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return persistAgentOriginHealthReports(tx, nodeID, reports, reportedAt)
	})
}

func persistAgentOriginHealthReports(tx *gorm.DB, nodeID string, reports []AgentOriginHealthReport, reportedAt time.Time) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || len(reports) == 0 {
		return nil
	}
	for _, report := range reports {
		record := buildOriginHealthStatusRecord(nodeID, report, reportedAt)
		if record == nil {
			continue
		}
		if err := model.UpsertOriginHealthStatus(tx, record); err != nil {
			return err
		}
	}
	return nil
}

func buildOriginHealthStatusRecord(nodeID string, report AgentOriginHealthReport, reportedAt time.Time) *model.OriginHealthStatus {
	originURL := strings.TrimSpace(report.OriginURL)
	if originURL == "" {
		return nil
	}
	status := normalizeOriginHealthStatus(report.Status)
	checkedAt := timeFromUnix(report.CheckedAtUnix, reportedAt)
	if checkedAt.After(reportedAt.UTC()) {
		checkedAt = reportedAt.UTC()
	}
	return &model.OriginHealthStatus{
		RouteID:    report.RouteID,
		NodeID:     strings.TrimSpace(nodeID),
		OriginURL:  originURL,
		Status:     status,
		LatencyMS:  nonNegativeInt64(report.LatencyMS),
		LastError:  truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(report.LastError)), 4096),
		ReportedAt: reportedAt.UTC(),
		CheckedAt:  checkedAt,
	}
}

func normalizeOriginHealthStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "healthy":
		return "healthy"
	case "unhealthy":
		return "unhealthy"
	default:
		return "unknown"
	}
}

func persistAgentDNSProbeReports(tx *gorm.DB, nodeID string, reports []AgentDNSProbeReport, reportedAt time.Time, allowedTargets map[string]AgentDNSProbeTarget, enforceTargets bool) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || len(reports) == 0 {
		return nil
	}
	for _, report := range reports {
		workerID := strings.TrimSpace(report.WorkerID)
		if workerID == "" {
			continue
		}
		target := AgentDNSProbeTarget{
			WorkerID:      workerID,
			PublicAddress: strings.TrimSpace(report.PublicAddress),
			QueryName:     strings.TrimSpace(report.QueryName),
			QueryType:     normalizeAgentDNSProbeQueryType(report.QueryType),
		}
		if enforceTargets {
			var ok bool
			target, ok = allowedTargets[workerID]
			if !ok {
				slog.Debug("agent dns probe report ignored for unassigned worker", "node_id", nodeID, "worker_id", workerID)
				continue
			}
			if !agentDNSProbeReportMatchesTarget(report, target) {
				slog.Debug("agent dns probe report ignored for mismatched target", "node_id", nodeID, "worker_id", workerID)
				continue
			}
		}
		results := normalizeAgentDNSProbeResults(report.Results)
		checkedAt := timeFromUnix(report.CheckedAtUnix, reportedAt)
		if checkedAt.After(reportedAt.UTC()) {
			checkedAt = reportedAt.UTC()
		}
		healthy, averageRTTMs, maxRTTMs, failureSamples, lastError := summarizeAgentDNSProbeResults(results)
		record := &model.DNSWorkerNodeProbe{
			WorkerID:       workerID,
			NodeID:         nodeID,
			PublicAddress:  strings.TrimSpace(target.PublicAddress),
			QueryName:      strings.TrimSpace(target.QueryName),
			QueryType:      normalizeAgentDNSProbeQueryType(target.QueryType),
			CheckedAt:      checkedAt,
			ResultsJSON:    marshalJSON(results),
			Healthy:        healthy,
			AverageRTTMs:   averageRTTMs,
			MaxRTTMs:       maxRTTMs,
			LastError:      truncateForDatabase(lastError, 2048),
			FailureSamples: failureSamples,
		}
		if record.ResultsJSON == "" {
			record.ResultsJSON = "[]"
		}
		if err := model.UpsertDNSWorkerNodeProbe(tx, record); err != nil {
			return err
		}
	}
	return nil
}

func agentDNSProbeTargetsByWorkerID(targetSets ...[]AgentDNSProbeTarget) (map[string]AgentDNSProbeTarget, bool) {
	result := map[string]AgentDNSProbeTarget{}
	enforce := len(targetSets) > 0
	for _, targets := range targetSets {
		for _, target := range targets {
			workerID := strings.TrimSpace(target.WorkerID)
			if workerID == "" {
				continue
			}
			if _, exists := result[workerID]; exists {
				continue
			}
			target.WorkerID = workerID
			target.Name = strings.TrimSpace(target.Name)
			target.PublicAddress = strings.TrimSpace(target.PublicAddress)
			target.QueryName = strings.TrimSpace(target.QueryName)
			target.QueryType = normalizeAgentDNSProbeQueryType(target.QueryType)
			result[workerID] = target
		}
	}
	return result, enforce
}

func agentDNSProbeReportMatchesTarget(report AgentDNSProbeReport, target AgentDNSProbeTarget) bool {
	if !strings.EqualFold(strings.TrimSpace(report.WorkerID), strings.TrimSpace(target.WorkerID)) {
		return false
	}
	if strings.TrimSpace(report.PublicAddress) != strings.TrimSpace(target.PublicAddress) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(report.QueryName), strings.TrimSpace(target.QueryName)) {
		return false
	}
	return normalizeAgentDNSProbeQueryType(report.QueryType) == normalizeAgentDNSProbeQueryType(target.QueryType)
}

func normalizeAgentDNSProbeResults(results []AgentDNSProbeResult) []AgentDNSProbeResult {
	if len(results) == 0 {
		return []AgentDNSProbeResult{}
	}
	cleaned := make([]AgentDNSProbeResult, 0, len(results))
	seen := map[string]struct{}{}
	for _, result := range results {
		network := strings.ToUpper(strings.TrimSpace(result.Network))
		if network == "" {
			continue
		}
		if _, ok := seen[network]; ok {
			continue
		}
		seen[network] = struct{}{}
		result.Network = network
		if result.DurationMs < 0 {
			result.DurationMs = 0
		}
		if result.AnswerCount < 0 {
			result.AnswerCount = 0
		}
		result.RCode = strings.ToUpper(strings.TrimSpace(result.RCode))
		result.Error = truncateForDatabase(result.Error, 1024)
		cleaned = append(cleaned, result)
	}
	return cleaned
}

func normalizeAgentDNSProbeQueryType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "A", "AAAA", "NS", "SOA":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "SOA"
	}
}

func summarizeAgentDNSProbeResults(results []AgentDNSProbeResult) (bool, float64, int64, int, string) {
	if len(results) == 0 {
		return false, 0, 0, 0, "Agent 未返回 DNS Worker 探测结果"
	}
	reachableByNetwork := map[string]bool{}
	var totalRTT int64
	var rttSamples int64
	var maxRTT int64
	failureSamples := 0
	lastError := ""
	for _, result := range results {
		network := strings.ToUpper(strings.TrimSpace(result.Network))
		if network != "" {
			reachableByNetwork[network] = result.Reachable
		}
		if result.Reachable {
			duration := nonNegativeInt64(result.DurationMs)
			totalRTT += duration
			rttSamples++
			if duration > maxRTT {
				maxRTT = duration
			}
			continue
		}
		failureSamples++
		if lastError == "" {
			if strings.TrimSpace(result.Error) != "" {
				lastError = strings.TrimSpace(result.Error)
			} else if network != "" {
				lastError = network + " 53 探测失败"
			}
		}
	}
	healthy := reachableByNetwork["UDP"] && reachableByNetwork["TCP"]
	if !healthy && lastError == "" {
		lastError = "UDP/TCP 53 未同时可达"
	}
	averageRTT := 0.0
	if rttSamples > 0 {
		averageRTT = float64(totalRTT) / float64(rttSamples)
	}
	return healthy, averageRTT, maxRTT, failureSamples, lastError
}

func persistBufferedObservability(tx *gorm.DB, nodeID string, records []AgentBufferedObservabilityRecord, reportedAt time.Time) error {
	snapshots := make([]*model.NodeMetricSnapshot, 0, len(records))
	reports := make([]*model.NodeRequestReport, 0, len(records))
	for _, record := range records {
		if snapshot := buildNodeMetricSnapshotRecord(nodeID, record.Snapshot, reportedAt); snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
		report, err := buildNodeRequestReportRecord(nodeID, record.TrafficReport, reportedAt)
		if err != nil {
			return err
		}
		if report != nil {
			reports = append(reports, report)
		}
	}
	if _, err := model.InsertNewNodeMetricSnapshots(tx, snapshots); err != nil {
		return err
	}
	if _, err := model.InsertNewNodeRequestReports(tx, reports); err != nil {
		return err
	}
	return nil
}

func persistAccessLogsBestEffort(nodeID string, logs []AgentNodeAccessLog, bufferedRecords []AgentBufferedObservabilityRecord, reportedAt time.Time) {
	if len(logs) == 0 && len(bufferedRecords) == 0 {
		return
	}
	if len(logs) > 0 {
		if err := persistNodeAccessLogsWithTransaction(nodeID, logs, reportedAt); err != nil {
			slog.Warn("persist access logs failed", "node_id", nodeID, "count", len(logs), "error", err)
		}
	}
	for _, record := range bufferedRecords {
		if len(record.AccessLogs) == 0 {
			continue
		}
		if err := persistNodeAccessLogsWithTransaction(nodeID, record.AccessLogs, reportedAt); err != nil {
			slog.Warn("persist buffered access logs failed", "node_id", nodeID, "count", len(record.AccessLogs), "error", err)
		}
	}
}

func persistNodeAccessLogsWithTransaction(nodeID string, logs []AgentNodeAccessLog, reportedAt time.Time) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return persistNodeAccessLogs(tx, nodeID, logs, reportedAt)
	})
}

func persistNodeSystemProfile(tx *gorm.DB, nodeID string, profile *AgentNodeSystemProfile, reportedAt time.Time) error {
	if profile == nil {
		return nil
	}
	record := &model.NodeSystemProfile{
		NodeID:           nodeID,
		Hostname:         strings.TrimSpace(profile.Hostname),
		OSName:           strings.TrimSpace(profile.OSName),
		OSVersion:        strings.TrimSpace(profile.OSVersion),
		KernelVersion:    strings.TrimSpace(profile.KernelVersion),
		Architecture:     strings.TrimSpace(profile.Architecture),
		CPUModel:         strings.TrimSpace(profile.CPUModel),
		CPUCores:         profile.CPUCores,
		TotalMemoryBytes: profile.TotalMemoryBytes,
		TotalDiskBytes:   profile.TotalDiskBytes,
		UptimeSeconds:    profile.UptimeSeconds,
		ReportedAt:       timeFromUnix(profile.ReportedAtUnix, reportedAt),
	}
	return tx.Model(&model.NodeSystemProfile{}).Where("node_id = ?", nodeID).Assign(record).FirstOrCreate(record).Error
}

func persistNodeMetricSnapshot(tx *gorm.DB, nodeID string, snapshot *AgentNodeMetricSnapshot, reportedAt time.Time) error {
	record := buildNodeMetricSnapshotRecord(nodeID, snapshot, reportedAt)
	if record == nil {
		return nil
	}
	_, err := model.InsertNewNodeMetricSnapshots(tx, []*model.NodeMetricSnapshot{record})
	return err
}

func buildNodeMetricSnapshotRecord(nodeID string, snapshot *AgentNodeMetricSnapshot, reportedAt time.Time) *model.NodeMetricSnapshot {
	if snapshot == nil {
		return nil
	}
	capturedAt := timeFromUnix(snapshot.CapturedAtUnix, reportedAt)
	if capturedAt.After(reportedAt.UTC()) {
		capturedAt = reportedAt.UTC()
	}
	return &model.NodeMetricSnapshot{
		NodeID:               nodeID,
		CapturedAt:           capturedAt,
		CPUUsagePercent:      snapshot.CPUUsagePercent,
		MemoryUsedBytes:      snapshot.MemoryUsedBytes,
		MemoryTotalBytes:     snapshot.MemoryTotalBytes,
		StorageUsedBytes:     snapshot.StorageUsedBytes,
		StorageTotalBytes:    snapshot.StorageTotalBytes,
		DiskReadBytes:        snapshot.DiskReadBytes,
		DiskWriteBytes:       snapshot.DiskWriteBytes,
		NetworkRxBytes:       snapshot.NetworkRxBytes,
		NetworkTxBytes:       snapshot.NetworkTxBytes,
		OpenrestyRxBytes:     snapshot.OpenrestyRxBytes,
		OpenrestyTxBytes:     snapshot.OpenrestyTxBytes,
		OpenrestyConnections: snapshot.OpenrestyConnections,
	}
}

func persistNodeTrafficReport(tx *gorm.DB, nodeID string, report *AgentNodeTrafficReport, reportedAt time.Time) error {
	record, err := buildNodeRequestReportRecord(nodeID, report, reportedAt)
	if err != nil {
		return err
	}
	if record == nil {
		return nil
	}
	_, err = model.InsertNewNodeRequestReports(tx, []*model.NodeRequestReport{record})
	return err
}

func buildNodeRequestReportRecord(nodeID string, report *AgentNodeTrafficReport, reportedAt time.Time) (*model.NodeRequestReport, error) {
	if report == nil {
		return nil, nil
	}
	if report.WindowEndedAtUnix > 0 && report.WindowStartedAtUnix > report.WindowEndedAtUnix {
		return nil, errors.New("traffic report window_started_at_unix 不能大于 window_ended_at_unix")
	}
	return &model.NodeRequestReport{
		NodeID:              nodeID,
		WindowStartedAt:     timeFromUnix(report.WindowStartedAtUnix, reportedAt),
		WindowEndedAt:       timeFromUnix(report.WindowEndedAtUnix, reportedAt),
		RequestCount:        report.RequestCount,
		ErrorCount:          report.ErrorCount,
		CacheHitCount:       report.CacheHitCount,
		CacheMissCount:      report.CacheMissCount,
		CacheBypassCount:    report.CacheBypassCount,
		CacheExpiredCount:   report.CacheExpiredCount,
		CacheStaleCount:     report.CacheStaleCount,
		UpstreamErrorCount:  report.UpstreamErrorCount,
		UpstreamResponseMS:  report.UpstreamResponseMS,
		UniqueVisitorCount:  report.UniqueVisitorCount,
		StatusCodesJSON:     marshalJSON(report.StatusCodes),
		TopDomainsJSON:      marshalJSON(report.TopDomains),
		SourceCountriesJSON: marshalJSON(report.SourceCountries),
	}, nil
}

func persistNodeAccessLogs(tx *gorm.DB, nodeID string, logs []AgentNodeAccessLog, reportedAt time.Time) error {
	if len(logs) == 0 {
		return nil
	}
	resolver, err := newAccessLogRegionResolver()
	if err != nil {
		slog.Warn("initialize access log geo resolver failed", "node_id", nodeID, "error", err)
	}
	if resolver != nil {
		defer resolver.Close()
	}
	records := make([]*model.NodeAccessLog, 0, len(logs))
	for _, item := range logs {
		record := &model.NodeAccessLog{
			NodeID:        nodeID,
			LoggedAt:      timeFromUnix(item.LoggedAtUnix, reportedAt),
			RemoteAddr:    strings.TrimSpace(item.RemoteAddr),
			Region:        "",
			Operator:      "",
			Host:          strings.TrimSpace(item.Host),
			Path:          truncateForDatabase(normalizePersistedAccessLogPath(item.Path), nodeAccessLogPathMaxLength),
			StatusCode:    item.StatusCode,
			Reason:        truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(item.Reason)), 512),
			CacheStatus:   normalizeAccessLogCacheStatus(item.CacheStatus),
			RequestBytes:  nonNegativeInt64(item.RequestBytes),
			ResponseBytes: nonNegativeInt64(item.ResponseBytes),
			UpstreamBytes: nonNegativeInt64(item.UpstreamBytes),
		}
		if resolver != nil {
			geoResult := resolver.ResolveInfo(record.RemoteAddr)
			record.Region = geoResult.region
			record.Operator = truncateForDatabase(geoResult.operator, 255)
		}
		records = append(records, record)
	}
	if _, err := model.InsertNewNodeAccessLogs(tx, records); err != nil {
		return err
	}
	_, err = model.DeleteNodeAccessLogsByNodeBefore(tx, nodeID, reportedAt.Add(-nodeAccessLogRetentionWindow))
	return err
}

func normalizePersistedAccessLogPath(value string) string {
	trimmed := strings.TrimSpace(security.RedactSensitiveText(value))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return parsed.String()
		}
	}
	if index := strings.IndexAny(trimmed, "?#"); index >= 0 {
		return trimmed[:index]
	}
	return trimmed
}

func reconcileNodeHealthEvents(tx *gorm.DB, nodeID string, events []AgentNodeHealthEvent, reportedAt time.Time) error {
	activeTypes := make(map[string]AgentNodeHealthEvent, len(events))
	for _, event := range events {
		eventType := normalizeHealthEventType(event.EventType)
		if eventType == "" {
			continue
		}
		event.EventType = eventType
		event.Severity = normalizeHealthSeverity(event.Severity)
		if event.TriggeredAtUnix <= 0 {
			event.TriggeredAtUnix = reportedAt.Unix()
		}
		activeTypes[eventType] = event
	}

	var activeEvents []*model.NodeHealthEvent
	if err := tx.Where("node_id = ? AND status = ?", nodeID, NodeHealthEventStatusActive).Find(&activeEvents).Error; err != nil {
		return err
	}

	activeByType := make(map[string]*model.NodeHealthEvent, len(activeEvents))
	for _, event := range activeEvents {
		activeByType[event.EventType] = event
	}

	for eventType, event := range activeTypes {
		triggeredAt := timeFromUnix(event.TriggeredAtUnix, reportedAt)
		if existing, ok := activeByType[eventType]; ok {
			existing.Severity = event.Severity
			existing.Message = normalizeHealthEventMessage(event.Message)
			existing.LastTriggeredAt = triggeredAt
			existing.ReportedAt = reportedAt
			existing.MetadataJSON = marshalJSON(event.Metadata)
			existing.ResolvedAt = nil
			if err := tx.Save(existing).Error; err != nil {
				return err
			}
			continue
		}
		record := &model.NodeHealthEvent{
			NodeID:           nodeID,
			EventType:        eventType,
			Severity:         event.Severity,
			Status:           NodeHealthEventStatusActive,
			Message:          normalizeHealthEventMessage(event.Message),
			FirstTriggeredAt: triggeredAt,
			LastTriggeredAt:  triggeredAt,
			ReportedAt:       reportedAt,
			MetadataJSON:     marshalJSON(event.Metadata),
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
	}

	for _, existing := range activeEvents {
		if _, ok := activeTypes[existing.EventType]; ok {
			continue
		}
		resolvedAt := reportedAt
		existing.Status = NodeHealthEventStatusResolved
		existing.ReportedAt = reportedAt
		existing.ResolvedAt = &resolvedAt
		if err := tx.Save(existing).Error; err != nil {
			return err
		}
	}

	return nil
}

func normalizeHealthEventType(eventType string) string {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	eventType = strings.ReplaceAll(eventType, " ", "_")
	return eventType
}

func normalizeHealthSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case NodeHealthSeverityCritical:
		return NodeHealthSeverityCritical
	case NodeHealthSeverityInfo:
		return NodeHealthSeverityInfo
	default:
		return NodeHealthSeverityWarning
	}
}

func normalizeHealthEventMessage(message string) string {
	return truncateForDatabase(message, 4096)
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func timeFromUnix(unixSeconds int64, fallback time.Time) time.Time {
	if unixSeconds <= 0 {
		return fallback
	}
	return time.Unix(unixSeconds, 0).UTC()
}

func marshalJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
