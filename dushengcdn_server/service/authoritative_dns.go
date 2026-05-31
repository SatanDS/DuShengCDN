package service

import (
	"context"
	"crypto/sha256"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"gorm.io/gorm"
)

const (
	dnsWorkerStatusOnline        = "online"
	dnsWorkerStatusOffline       = "offline"
	dnsDelegationMatched         = "matched"
	dnsDelegationPartial         = "partial"
	dnsDelegationMismatch        = "mismatch"
	dnsDelegationFailed          = "failed"
	dnsDelegationNotConfig       = "not_configured"
	dnsSnapshotConsistent        = "consistent"
	dnsSnapshotDivergent         = "divergent"
	dnsSnapshotStale             = "stale"
	dnsSnapshotNoOnline          = "no_online_workers"
	dnsSnapshotUnknown           = "unknown"
	dnsWorkerProbeHealthy        = "healthy"
	dnsWorkerProbePartial        = "partial"
	dnsWorkerProbeFailed         = "failed"
	dnsWorkerProbeStale          = "stale"
	dnsWorkerProbeUnknown        = "unknown"
	defaultDNSZoneTTL            = 300
	defaultDNSSnapshotMaxAge     = 5 * time.Minute
	defaultDNSWorkerProbeTimeout = 3 * time.Second
	defaultDNSWorkerProbeMaxAge  = 24 * time.Hour
)

var dnsLookupNS = net.LookupNS
var dnsWorkerProbeExchange = exchangeDNSWorkerProbe

type DNSZoneInput struct {
	Name        string   `json:"name"`
	SOAEmail    string   `json:"soa_email"`
	PrimaryNS   string   `json:"primary_ns"`
	NameServers []string `json:"name_servers"`
	DefaultTTL  int      `json:"default_ttl"`
	Enabled     *bool    `json:"enabled"`
}

type DNSRecordInput struct {
	ZoneID   uint   `json:"zone_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Enabled  *bool  `json:"enabled"`
}

type DNSWorkerInput struct {
	Name          string `json:"name"`
	PublicAddress string `json:"public_address"`
}

type DNSWorkerHeartbeatInput struct {
	Version             string                `json:"version"`
	Status              string                `json:"status"`
	LastSnapshotVersion string                `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time            `json:"last_snapshot_at"`
	LastError           string                `json:"last_error"`
	Rollups             []DNSQueryRollupInput `json:"rollups"`
}

type DNSQueryRollupInput struct {
	WindowStart     time.Time        `json:"window_start"`
	WindowMinutes   int              `json:"window_minutes"`
	ZoneID          uint             `json:"zone_id"`
	ProxyRouteID    uint             `json:"proxy_route_id"`
	QName           string           `json:"qname"`
	QType           string           `json:"qtype"`
	RCode           string           `json:"rcode"`
	QueryCount      int64            `json:"query_count"`
	TotalDurationMs int64            `json:"total_duration_ms"`
	MaxDurationMs   int64            `json:"max_duration_ms"`
	TargetSummary   map[string]int64 `json:"target_summary"`
}

type DNSZoneView struct {
	ID          uint            `json:"id"`
	Name        string          `json:"name"`
	SOAEmail    string          `json:"soa_email"`
	PrimaryNS   string          `json:"primary_ns"`
	NameServers []string        `json:"name_servers"`
	DefaultTTL  int             `json:"default_ttl"`
	Serial      uint64          `json:"serial"`
	Enabled     bool            `json:"enabled"`
	RecordCount int64           `json:"record_count"`
	Records     []DNSRecordView `json:"records,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DNSRecordView struct {
	ID        uint      `json:"id"`
	ZoneID    uint      `json:"zone_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	TTL       int       `json:"ttl"`
	Priority  int       `json:"priority"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DNSWorkerView struct {
	ID                  uint                       `json:"id"`
	WorkerID            string                     `json:"worker_id"`
	Name                string                     `json:"name"`
	Token               string                     `json:"token,omitempty"`
	PublicAddress       string                     `json:"public_address"`
	Version             string                     `json:"version"`
	Status              string                     `json:"status"`
	LastSnapshotVersion string                     `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time                 `json:"last_snapshot_at"`
	LastSeenAt          *time.Time                 `json:"last_seen_at"`
	LastError           string                     `json:"last_error"`
	LastProbeAt         *time.Time                 `json:"last_probe_at"`
	LastProbeQuery      string                     `json:"last_probe_query"`
	LastProbeResults    []DNSWorkerProbeResultView `json:"last_probe_results"`
	ProbeStatus         string                     `json:"probe_status"`
	ProbeHealthy        bool                       `json:"probe_healthy"`
	ProbeAgeSeconds     int64                      `json:"probe_age_seconds"`
	ProbeMessage        string                     `json:"probe_message"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

type DNSWorkerProbeInput struct {
	ZoneID uint `json:"zone_id"`
}

type DNSWorkerProbeView struct {
	WorkerID      string                     `json:"worker_id"`
	Name          string                     `json:"name"`
	PublicAddress string                     `json:"public_address"`
	QueryName     string                     `json:"query_name"`
	QueryType     string                     `json:"query_type"`
	CheckedAt     time.Time                  `json:"checked_at"`
	Results       []DNSWorkerProbeResultView `json:"results"`
}

type DNSWorkerProbeResultView struct {
	Network     string `json:"network"`
	Reachable   bool   `json:"reachable"`
	DurationMs  int64  `json:"duration_ms"`
	RCode       string `json:"rcode"`
	AnswerCount int    `json:"answer_count"`
	Error       string `json:"error,omitempty"`
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
	WindowHours         int                              `json:"window_hours"`
	WindowStart         time.Time                        `json:"window_start"`
	WindowEnd           time.Time                        `json:"window_end"`
	LastRollupAt        *time.Time                       `json:"last_rollup_at"`
	TotalQueries        int64                            `json:"total_queries"`
	SuccessfulQueries   int64                            `json:"successful_queries"`
	NegativeQueries     int64                            `json:"negative_queries"`
	ErrorQueries        int64                            `json:"error_queries"`
	DynamicQueries      int64                            `json:"dynamic_queries"`
	StaticQueries       int64                            `json:"static_queries"`
	RCodeBreakdown      []DNSObservabilityCounterView    `json:"rcode_breakdown"`
	QTypeBreakdown      []DNSObservabilityCounterView    `json:"qtype_breakdown"`
	TopQNames           []DNSObservabilityCounterView    `json:"top_qnames"`
	TopTargets          []DNSObservabilityCounterView    `json:"top_targets"`
	WorkerBreakdown     []DNSObservabilityCounterView    `json:"worker_breakdown"`
	ZoneBreakdown       []DNSObservabilityCounterView    `json:"zone_breakdown"`
	RouteBreakdown      []DNSObservabilityCounterView    `json:"route_breakdown"`
	TrendPoints         []DNSObservabilityTrendPointView `json:"trend_points"`
	SnapshotConsistency DNSWorkerSnapshotConsistencyView `json:"snapshot_consistency"`
	WorkerHealth        DNSWorkerHealthSummaryView       `json:"worker_health"`
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
	WorkerID        string     `json:"worker_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	SnapshotVersion string     `json:"snapshot_version"`
	LastSnapshotAt  *time.Time `json:"last_snapshot_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	Stale           bool       `json:"stale"`
}

type DNSWorkerHealthSummaryView struct {
	CheckedAt           time.Time                 `json:"checked_at"`
	TotalWorkerCount    int                       `json:"total_worker_count"`
	OnlineWorkerCount   int                       `json:"online_worker_count"`
	ProbeHealthyCount   int                       `json:"probe_healthy_count"`
	ProbeCheckedCount   int                       `json:"probe_checked_count"`
	ProbeHealthyPercent float64                   `json:"probe_healthy_percent"`
	AvailabilityPercent float64                   `json:"availability_percent"`
	AverageLatencyMs    float64                   `json:"average_latency_ms"`
	MaxLatencyMs        int64                     `json:"max_latency_ms"`
	ErrorRatePercent    float64                   `json:"error_rate_percent"`
	Workers             []DNSWorkerHealthItemView `json:"workers"`
}

type DNSWorkerHealthItemView struct {
	WorkerID           string                     `json:"worker_id"`
	Name               string                     `json:"name"`
	Status             string                     `json:"status"`
	PublicAddress      string                     `json:"public_address"`
	QueryCount         int64                      `json:"query_count"`
	ErrorQueries       int64                      `json:"error_queries"`
	ErrorRatePercent   float64                    `json:"error_rate_percent"`
	AverageLatencyMs   float64                    `json:"average_latency_ms"`
	MaxLatencyMs       int64                      `json:"max_latency_ms"`
	LastSeenAt         *time.Time                 `json:"last_seen_at"`
	LastSnapshotAt     *time.Time                 `json:"last_snapshot_at"`
	SnapshotAgeSeconds int64                      `json:"snapshot_age_seconds"`
	SnapshotStale      bool                       `json:"snapshot_stale"`
	LastError          string                     `json:"last_error"`
	LastProbeAt        *time.Time                 `json:"last_probe_at"`
	LastProbeResults   []DNSWorkerProbeResultView `json:"last_probe_results"`
	ProbeStatus        string                     `json:"probe_status"`
	ProbeHealthy       bool                       `json:"probe_healthy"`
	ProbeAgeSeconds    int64                      `json:"probe_age_seconds"`
	ProbeMessage       string                     `json:"probe_message"`
}

type DNSZoneDelegationCheckView struct {
	ZoneID              uint      `json:"zone_id"`
	ZoneName            string    `json:"zone_name"`
	ExpectedNameServers []string  `json:"expected_name_servers"`
	ActualNameServers   []string  `json:"actual_name_servers"`
	MatchedNameServers  []string  `json:"matched_name_servers"`
	MissingNameServers  []string  `json:"missing_name_servers"`
	ExtraNameServers    []string  `json:"extra_name_servers"`
	GlueRequired        bool      `json:"glue_required"`
	GlueNameServers     []string  `json:"glue_name_servers"`
	Status              string    `json:"status"`
	CheckedAt           time.Time `json:"checked_at"`
	Error               string    `json:"error,omitempty"`
}

type AuthoritativeDNSSnapshot struct {
	SnapshotVersion string                          `json:"snapshot_version"`
	GeneratedAt     time.Time                       `json:"generated_at"`
	Zones           []AuthoritativeDNSSnapshotZone  `json:"zones"`
	Routes          []AuthoritativeDNSSnapshotRoute `json:"routes"`
	Nodes           []AuthoritativeDNSSnapshotNode  `json:"nodes"`
}

type AuthoritativeDNSSnapshotZone struct {
	ID          uint                             `json:"id"`
	Name        string                           `json:"name"`
	SOAEmail    string                           `json:"soa_email"`
	PrimaryNS   string                           `json:"primary_ns"`
	NameServers []string                         `json:"name_servers"`
	DefaultTTL  int                              `json:"default_ttl"`
	Serial      uint64                           `json:"serial"`
	Records     []AuthoritativeDNSSnapshotRecord `json:"records"`
}

type AuthoritativeDNSSnapshotRecord struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
}

type AuthoritativeDNSSnapshotRoute struct {
	ID             uint                 `json:"id"`
	SiteName       string               `json:"site_name"`
	Domains        []string             `json:"domains"`
	ZoneID         uint                 `json:"zone_id"`
	NodePool       string               `json:"node_pool"`
	RecordType     string               `json:"record_type"`
	TargetCount    int                  `json:"target_count"`
	ScheduleMode   string               `json:"schedule_mode"`
	TTL            int                  `json:"ttl"`
	GSLBEnabled    bool                 `json:"gslb_enabled"`
	GSLBPolicy     ProxyRouteGSLBPolicy `json:"gslb_policy"`
	CurrentTargets []string             `json:"current_targets"`
	TargetError    string               `json:"target_error,omitempty"`
}

type AuthoritativeDNSSnapshotNode struct {
	NodeID               string     `json:"node_id"`
	Name                 string     `json:"name"`
	PoolName             string     `json:"pool_name"`
	PublicIPs            []string   `json:"public_ips"`
	Weight               int        `json:"weight"`
	SchedulingEnabled    bool       `json:"scheduling_enabled"`
	DrainMode            bool       `json:"drain_mode"`
	Status               string     `json:"status"`
	OpenrestyStatus      string     `json:"openresty_status"`
	LastSeenAt           time.Time  `json:"last_seen_at"`
	OpenrestyConnections int64      `json:"openresty_connections"`
	CPUUsagePercent      float64    `json:"cpu_usage_percent"`
	MemoryUsagePercent   float64    `json:"memory_usage_percent"`
	MetricCapturedAt     *time.Time `json:"metric_captured_at,omitempty"`
}

func ListAuthoritativeDNSZones() ([]DNSZoneView, error) {
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	views := make([]DNSZoneView, 0, len(zones))
	for _, zone := range zones {
		view, err := buildDNSZoneView(zone, false)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func GetAuthoritativeDNSZone(id uint) (*DNSZoneView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func CreateAuthoritativeDNSZone(input DNSZoneInput) (*DNSZoneView, error) {
	zone, err := buildDNSZone(nil, input)
	if err != nil {
		return nil, err
	}
	if err := zone.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS zone already exists")
		}
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func UpdateAuthoritativeDNSZone(id uint, input DNSZoneInput) (*DNSZoneView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	zone, err = buildDNSZone(zone, input)
	if err != nil {
		return nil, err
	}
	if err := zone.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS zone already exists")
		}
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func DeleteAuthoritativeDNSZone(id uint) error {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return err
	}
	var routeCount int64
	if err := model.DB.Model(&model.ProxyRoute{}).Where("dns_zone_id_ref = ?", id).Count(&routeCount).Error; err != nil {
		return err
	}
	if routeCount > 0 {
		return errors.New("DNS zone is used by authoritative proxy routes")
	}
	if err := model.DB.Where("zone_id = ?", zone.ID).Delete(&model.DNSRecord{}).Error; err != nil {
		return err
	}
	return zone.Delete()
}

func ListAuthoritativeDNSRecords(zoneID uint) ([]DNSRecordView, error) {
	if _, err := model.GetDNSZoneByID(zoneID); err != nil {
		return nil, err
	}
	records, err := model.ListDNSRecordsByZoneID(zoneID)
	if err != nil {
		return nil, err
	}
	views := make([]DNSRecordView, 0, len(records))
	for _, record := range records {
		views = append(views, buildDNSRecordView(record))
	}
	return views, nil
}

func CreateAuthoritativeDNSRecord(zoneID uint, input DNSRecordInput) (*DNSRecordView, error) {
	if input.ZoneID == 0 {
		input.ZoneID = zoneID
	}
	if input.ZoneID != zoneID {
		return nil, errors.New("record zone_id does not match request zone")
	}
	record, err := buildDNSRecord(nil, input)
	if err != nil {
		return nil, err
	}
	if err := record.Insert(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func UpdateAuthoritativeDNSRecord(id uint, input DNSRecordInput) (*DNSRecordView, error) {
	record, err := model.GetDNSRecordByID(id)
	if err != nil {
		return nil, err
	}
	if input.ZoneID == 0 {
		input.ZoneID = record.ZoneID
	}
	if input.ZoneID != record.ZoneID {
		return nil, errors.New("moving DNS records between zones is not supported")
	}
	record, err = buildDNSRecord(record, input)
	if err != nil {
		return nil, err
	}
	if err := record.Update(); err != nil {
		return nil, err
	}
	if err := bumpDNSZoneSerial(record.ZoneID); err != nil {
		return nil, err
	}
	return ptrDNSRecordView(buildDNSRecordView(record)), nil
}

func DeleteAuthoritativeDNSRecord(id uint) error {
	record, err := model.GetDNSRecordByID(id)
	if err != nil {
		return err
	}
	zoneID := record.ZoneID
	if err := record.Delete(); err != nil {
		return err
	}
	return bumpDNSZoneSerial(zoneID)
}

func ListAuthoritativeDNSWorkers() ([]DNSWorkerView, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	views := make([]DNSWorkerView, 0, len(workers))
	for _, worker := range workers {
		views = append(views, buildDNSWorkerView(worker, false))
	}
	return views, nil
}

func GetAuthoritativeDNSObservabilitySummary(input DNSObservabilitySummaryInput) (*DNSObservabilitySummaryView, error) {
	hours := input.Hours
	if hours <= 0 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-time.Duration(hours) * time.Hour)
	var rollups []model.DNSQueryRollup
	query := model.DB.Where("window_start >= ? AND window_start <= ?", windowStart, windowEnd)
	if input.ZoneID > 0 {
		query = query.Where("zone_id = ?", input.ZoneID)
	}
	if strings.TrimSpace(input.WorkerID) != "" {
		query = query.Where("worker_id = ?", strings.TrimSpace(input.WorkerID))
	}
	if err := query.Order("window_start asc").Find(&rollups).Error; err != nil {
		return nil, err
	}

	summary := &DNSObservabilitySummaryView{
		WindowHours: hours,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}
	rcodeCounts := map[string]int64{}
	qtypeCounts := map[string]int64{}
	qnameCounts := map[string]int64{}
	targetCounts := map[string]int64{}
	workerCounts := map[string]int64{}
	zoneCounts := map[uint]int64{}
	routeCounts := map[uint]int64{}
	trendPoints := initDNSObservabilityTrendPoints(windowEnd, hours)

	for _, rollup := range rollups {
		if rollup.QueryCount <= 0 {
			continue
		}
		count := rollup.QueryCount
		rcode := normalizeDNSRCode(rollup.RCode)
		qtype := normalizeAuthoritativeDNSRecordTypeOrDefault(rollup.QType)
		qname := normalizeDNSRecordName(rollup.QName)
		if qname == "" {
			qname = "unknown"
		}
		summary.TotalQueries += count
		switch rcode {
		case "NOERROR":
			summary.SuccessfulQueries += count
		case "SERVFAIL", "REFUSED":
			summary.ErrorQueries += count
		default:
			summary.NegativeQueries += count
		}
		if rollup.ProxyRouteID > 0 {
			summary.DynamicQueries += count
			routeCounts[rollup.ProxyRouteID] += count
		} else {
			summary.StaticQueries += count
		}
		applyDNSObservabilityTrendPoint(trendPoints, rollup.WindowStart, rcode, rollup.ProxyRouteID > 0, count)
		rcodeCounts[rcode] += count
		qtypeCounts[qtype] += count
		qnameCounts[qname] += count
		if strings.TrimSpace(rollup.WorkerID) != "" {
			workerCounts[strings.TrimSpace(rollup.WorkerID)] += count
		}
		if rollup.ZoneID > 0 {
			zoneCounts[rollup.ZoneID] += count
		}
		for target, targetCount := range decodeDNSTargetSummary(rollup.TargetSummary) {
			if targetCount <= 0 {
				continue
			}
			targetCounts[target] += targetCount
		}
		rollupEnd := rollup.WindowStart.Add(time.Duration(normalizeDNSRollupWindow(rollup.WindowMinutes)) * time.Minute)
		if summary.LastRollupAt == nil || rollupEnd.After(*summary.LastRollupAt) {
			lastRollupAt := rollupEnd
			summary.LastRollupAt = &lastRollupAt
		}
	}

	workerLabels, err := dnsWorkerLabels()
	if err != nil {
		return nil, err
	}
	zoneLabels, err := dnsZoneLabels()
	if err != nil {
		return nil, err
	}
	routeLabels, err := dnsRouteLabels(routeCounts)
	if err != nil {
		return nil, err
	}

	summary.RCodeBreakdown = buildDNSObservabilityCounters(rcodeCounts, nil, 10)
	summary.QTypeBreakdown = buildDNSObservabilityCounters(qtypeCounts, nil, 10)
	summary.TopQNames = buildDNSObservabilityCounters(qnameCounts, nil, 8)
	summary.TopTargets = buildDNSObservabilityCounters(targetCounts, nil, 8)
	summary.WorkerBreakdown = buildDNSObservabilityCounters(workerCounts, workerLabels, 8)
	summary.ZoneBreakdown = buildDNSObservabilityCounters(uintCountsToStringCounts(zoneCounts), zoneLabels, 8)
	summary.RouteBreakdown = buildDNSObservabilityCounters(uintCountsToStringCounts(routeCounts), routeLabels, 8)
	summary.TrendPoints = trendPoints
	checkedAt := time.Now().UTC()
	summary.SnapshotConsistency = buildDNSWorkerSnapshotConsistency(checkedAt)
	summary.WorkerHealth = buildDNSWorkerHealthSummary(checkedAt, rollups)
	return summary, nil
}

func CheckAuthoritativeDNSZoneDelegation(id uint) (*DNSZoneDelegationCheckView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	expected := normalizeDNSNameServerSet(decodeStoredStringList(zone.NameServers))
	view := &DNSZoneDelegationCheckView{
		ZoneID:              zone.ID,
		ZoneName:            zone.Name,
		ExpectedNameServers: expected,
		GlueNameServers:     glueNameServersForZone(zone.Name, expected),
		CheckedAt:           time.Now().UTC(),
	}
	view.GlueRequired = len(view.GlueNameServers) > 0
	if len(expected) == 0 {
		view.Status = dnsDelegationNotConfig
		return view, nil
	}

	records, err := dnsLookupNS(zone.Name)
	if err != nil {
		view.Status = dnsDelegationFailed
		view.Error = err.Error()
		return view, nil
	}
	actual := make([]string, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		actual = append(actual, record.Host)
	}
	view.ActualNameServers = normalizeDNSNameServerSet(actual)
	view.MatchedNameServers, view.MissingNameServers, view.ExtraNameServers = compareNameServerSets(expected, view.ActualNameServers)
	view.Status = dnsDelegationStatus(expected, view.ActualNameServers, view.MatchedNameServers, view.MissingNameServers, view.ExtraNameServers)
	return view, nil
}

func CreateAuthoritativeDNSWorker(input DNSWorkerInput) (*DNSWorkerView, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("DNS worker name cannot be empty")
	}
	if len(name) > 128 {
		return nil, errors.New("DNS worker name is too long")
	}
	token, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	workerIDSeed, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	worker := &model.DNSWorker{
		WorkerID:      "dns-" + workerIDSeed,
		Name:          name,
		Token:         token,
		PublicAddress: strings.TrimSpace(input.PublicAddress),
		Status:        dnsWorkerStatusOffline,
	}
	if err := worker.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS worker identity collision, please retry")
		}
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, true)), nil
}

func DeleteAuthoritativeDNSWorker(id uint) error {
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return err
	}
	return worker.Delete()
}

func ProbeAuthoritativeDNSWorker(id uint, input DNSWorkerProbeInput) (*DNSWorkerProbeView, error) {
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	target, err := normalizeDNSWorkerProbeAddress(worker.PublicAddress)
	if err != nil {
		return nil, err
	}
	queryName, err := dnsWorkerProbeQueryName(input.ZoneID)
	if err != nil {
		return nil, err
	}

	view := &DNSWorkerProbeView{
		WorkerID:      worker.WorkerID,
		Name:          worker.Name,
		PublicAddress: worker.PublicAddress,
		QueryName:     queryName,
		QueryType:     "SOA",
		CheckedAt:     time.Now().UTC(),
		Results:       make([]DNSWorkerProbeResultView, 0, 2),
	}
	for _, network := range []string{"udp", "tcp"} {
		view.Results = append(view.Results, dnsWorkerProbeExchange(context.Background(), target, network, queryName, dns.TypeSOA, defaultDNSWorkerProbeTimeout))
	}
	if err := persistDNSWorkerProbeResult(worker, view); err != nil {
		return nil, err
	}
	return view, nil
}

func AuthenticateDNSWorkerToken(token string) (*model.DNSWorker, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing DNS Worker Token")
	}
	worker, err := model.GetDNSWorkerByToken(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid DNS Worker Token")
		}
		return nil, err
	}
	return worker, nil
}

func RecordDNSWorkerHeartbeat(worker *model.DNSWorker, input DNSWorkerHeartbeatInput) (*DNSWorkerView, error) {
	if worker == nil {
		return nil, errors.New("DNS worker is nil")
	}
	now := time.Now()
	worker.Status = normalizeDNSWorkerStatus(input.Status)
	worker.Version = strings.TrimSpace(input.Version)
	worker.LastSnapshotVersion = strings.TrimSpace(input.LastSnapshotVersion)
	worker.LastSnapshotAt = input.LastSnapshotAt
	worker.LastSeenAt = &now
	worker.LastError = truncateForDatabase(strings.TrimSpace(input.LastError), 16000)
	if err := worker.Update(); err != nil {
		return nil, err
	}
	if err := persistDNSQueryRollups(worker.WorkerID, input.Rollups); err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func GetAuthoritativeDNSSnapshot(worker *model.DNSWorker) (*AuthoritativeDNSSnapshot, error) {
	zones, err := snapshotDNSZones()
	if err != nil {
		return nil, err
	}
	routes, err := snapshotAuthoritativeRoutes()
	if err != nil {
		return nil, err
	}
	nodes, err := snapshotNodes()
	if err != nil {
		return nil, err
	}
	snapshot := &AuthoritativeDNSSnapshot{
		GeneratedAt: time.Now().UTC(),
		Zones:       zones,
		Routes:      routes,
		Nodes:       nodes,
	}
	version, err := authoritativeDNSSnapshotVersion(snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.SnapshotVersion = version
	if worker != nil {
		_ = recordDNSWorkerSnapshotPull(worker, version)
	}
	return snapshot, nil
}

func buildDNSZone(zone *model.DNSZone, input DNSZoneInput) (*model.DNSZone, error) {
	name, err := normalizeDNSZoneName(input.Name)
	if err != nil {
		return nil, err
	}
	nameServers, err := normalizeNameServers(input.NameServers)
	if err != nil {
		return nil, err
	}
	primaryNS := normalizeDNSRecordName(input.PrimaryNS)
	if primaryNS == "" && len(nameServers) > 0 {
		primaryNS = nameServers[0]
	}
	if primaryNS != "" && !isValidProxyRouteDomain(primaryNS) {
		return nil, errors.New("primary_ns format is invalid")
	}
	soaEmail := normalizeSOAEmail(input.SOAEmail, name)
	enabled := true
	if zone != nil {
		enabled = zone.Enabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	nameServersJSON, err := json.Marshal(nameServers)
	if err != nil {
		return nil, err
	}
	if zone == nil {
		zone = &model.DNSZone{
			Serial: nextDNSZoneSerial(0),
		}
	}
	zoneChanged := zone.Name != "" && (zone.Name != name ||
		zone.SOAEmail != soaEmail ||
		zone.PrimaryNS != primaryNS ||
		zone.NameServers != string(nameServersJSON) ||
		normalizeDNSZoneTTL(zone.DefaultTTL) != normalizeDNSZoneTTL(input.DefaultTTL) ||
		zone.Enabled != enabled)
	if zoneChanged {
		zone.Serial = nextDNSZoneSerial(zone.Serial)
	}
	zone.Name = name
	zone.SOAEmail = soaEmail
	zone.PrimaryNS = primaryNS
	zone.NameServers = string(nameServersJSON)
	zone.DefaultTTL = normalizeDNSZoneTTL(input.DefaultTTL)
	zone.Enabled = enabled
	if zone.Serial == 0 {
		zone.Serial = nextDNSZoneSerial(0)
	}
	return zone, nil
}

func buildDNSRecord(record *model.DNSRecord, input DNSRecordInput) (*model.DNSRecord, error) {
	zone, err := model.GetDNSZoneByID(input.ZoneID)
	if err != nil {
		return nil, errors.New("selected DNS zone does not exist")
	}
	recordType, err := normalizeAuthoritativeDNSRecordType(input.Type)
	if err != nil {
		return nil, err
	}
	name, err := normalizeAuthoritativeDNSRecordName(zone.Name, input.Name)
	if err != nil {
		return nil, err
	}
	value, priority, err := normalizeAuthoritativeDNSRecordValue(recordType, input.Value, input.Priority)
	if err != nil {
		return nil, err
	}
	enabled := true
	if record != nil {
		enabled = record.Enabled
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if record == nil {
		record = &model.DNSRecord{}
	}
	record.ZoneID = input.ZoneID
	record.Name = name
	record.Type = recordType
	record.Value = value
	record.TTL = normalizeAuthoritativeTTL(input.TTL, zone.DefaultTTL)
	record.Priority = priority
	record.Enabled = enabled
	return record, nil
}

func buildDNSZoneView(zone *model.DNSZone, includeRecords bool) (*DNSZoneView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	view := &DNSZoneView{
		ID:          zone.ID,
		Name:        zone.Name,
		SOAEmail:    zone.SOAEmail,
		PrimaryNS:   zone.PrimaryNS,
		NameServers: decodeStoredStringList(zone.NameServers),
		DefaultTTL:  normalizeDNSZoneTTL(zone.DefaultTTL),
		Serial:      zone.Serial,
		Enabled:     zone.Enabled,
		CreatedAt:   zone.CreatedAt,
		UpdatedAt:   zone.UpdatedAt,
	}
	if err := model.DB.Model(&model.DNSRecord{}).Where("zone_id = ?", zone.ID).Count(&view.RecordCount).Error; err != nil {
		return nil, err
	}
	if includeRecords {
		records, err := model.ListDNSRecordsByZoneID(zone.ID)
		if err != nil {
			return nil, err
		}
		view.Records = make([]DNSRecordView, 0, len(records))
		for _, record := range records {
			view.Records = append(view.Records, buildDNSRecordView(record))
		}
	}
	return view, nil
}

func buildDNSRecordView(record *model.DNSRecord) DNSRecordView {
	if record == nil {
		return DNSRecordView{}
	}
	return DNSRecordView{
		ID:        record.ID,
		ZoneID:    record.ZoneID,
		Name:      record.Name,
		Type:      record.Type,
		Value:     record.Value,
		TTL:       record.TTL,
		Priority:  record.Priority,
		Enabled:   record.Enabled,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func buildDNSWorkerView(worker *model.DNSWorker, includeToken bool) DNSWorkerView {
	if worker == nil {
		return DNSWorkerView{}
	}
	probeResults := decodeDNSWorkerProbeResults(worker.LastProbeResult)
	probeState := evaluateDNSWorkerProbeState(time.Now().UTC(), worker.LastProbeAt, probeResults)
	view := DNSWorkerView{
		ID:                  worker.ID,
		WorkerID:            worker.WorkerID,
		Name:                worker.Name,
		PublicAddress:       worker.PublicAddress,
		Version:             worker.Version,
		Status:              normalizeDNSWorkerStatus(worker.Status),
		LastSnapshotVersion: worker.LastSnapshotVersion,
		LastSnapshotAt:      worker.LastSnapshotAt,
		LastSeenAt:          worker.LastSeenAt,
		LastError:           worker.LastError,
		LastProbeAt:         worker.LastProbeAt,
		LastProbeQuery:      worker.LastProbeQuery,
		LastProbeResults:    probeResults,
		ProbeStatus:         probeState.status,
		ProbeHealthy:        probeState.healthy,
		ProbeAgeSeconds:     probeState.ageSeconds,
		ProbeMessage:        probeState.message,
		CreatedAt:           worker.CreatedAt,
		UpdatedAt:           worker.UpdatedAt,
	}
	if includeToken {
		view.Token = worker.Token
	}
	return view
}

func snapshotDNSZones() ([]AuthoritativeDNSSnapshotZone, error) {
	var zones []*model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("name asc").Find(&zones).Error; err != nil {
		return nil, err
	}
	result := make([]AuthoritativeDNSSnapshotZone, 0, len(zones))
	for _, zone := range zones {
		records, err := model.ListDNSRecordsByZoneID(zone.ID)
		if err != nil {
			return nil, err
		}
		item := AuthoritativeDNSSnapshotZone{
			ID:          zone.ID,
			Name:        zone.Name,
			SOAEmail:    zone.SOAEmail,
			PrimaryNS:   zone.PrimaryNS,
			NameServers: decodeStoredStringList(zone.NameServers),
			DefaultTTL:  normalizeDNSZoneTTL(zone.DefaultTTL),
			Serial:      zone.Serial,
			Records:     make([]AuthoritativeDNSSnapshotRecord, 0, len(records)),
		}
		for _, record := range records {
			if record == nil || !record.Enabled {
				continue
			}
			item.Records = append(item.Records, AuthoritativeDNSSnapshotRecord{
				ID:       record.ID,
				Name:     record.Name,
				Type:     record.Type,
				Value:    record.Value,
				TTL:      normalizeAuthoritativeTTL(record.TTL, item.DefaultTTL),
				Priority: record.Priority,
			})
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotAuthoritativeRoutes() ([]AuthoritativeDNSSnapshotRoute, error) {
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", true, DNSProviderModeAuthoritative).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	result := make([]AuthoritativeDNSSnapshotRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zone, err := model.GetDNSZoneByID(*route.DNSZoneIDRef)
		if err != nil || zone == nil || !zone.Enabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return nil, err
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		item := AuthoritativeDNSSnapshotRoute{
			ID:           route.ID,
			SiteName:     normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain),
			Domains:      domains,
			ZoneID:       *route.DNSZoneIDRef,
			NodePool:     normalizeNodePoolName(route.NodePool),
			RecordType:   recordType,
			TargetCount:  normalizeDNSTargetCount(route.DNSTargetCount),
			ScheduleMode: normalizeDNSScheduleMode(route.DNSScheduleMode),
			TTL:          normalizeAuthoritativeRouteTTL(route.DNSTTL),
			GSLBEnabled:  route.GSLBEnabled,
			GSLBPolicy:   policy,
		}
		selection, selectErr := selectProxyRouteDNSTargets(route, recordType)
		if selectErr != nil {
			item.TargetError = selectErr.Error()
		} else {
			item.CurrentTargets = selection.Targets
			item.TTL = normalizeAuthoritativeRouteTTL(selection.TTL)
		}
		result = append(result, item)
	}
	return result, nil
}

func snapshotNodes() ([]AuthoritativeDNSSnapshotNode, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	metrics := latestNodeMetricSnapshots()
	result := make([]AuthoritativeDNSSnapshotNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		item := AuthoritativeDNSSnapshotNode{
			NodeID:            node.NodeID,
			Name:              node.Name,
			PoolName:          normalizeNodePoolName(node.PoolName),
			PublicIPs:         publicNodeIPsForSnapshot(node),
			Weight:            normalizeNodeWeight(node.Weight),
			SchedulingEnabled: isNodeSchedulableForDNS(node),
			DrainMode:         node.DrainMode,
			Status:            computeNodeStatus(node),
			OpenrestyStatus:   normalizeOpenrestyStatus(node.OpenrestyStatus),
			LastSeenAt:        node.LastSeenAt,
		}
		if metric := metrics[node.NodeID]; metric != nil {
			capturedAt := metric.CapturedAt
			item.OpenrestyConnections = metric.OpenrestyConnections
			item.CPUUsagePercent = metric.CPUUsagePercent
			item.MemoryUsagePercent = nodeMetricMemoryUsagePercent(metric)
			item.MetricCapturedAt = &capturedAt
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PoolName != result[j].PoolName {
			return result[i].PoolName < result[j].PoolName
		}
		return result[i].NodeID < result[j].NodeID
	})
	return result, nil
}

func authoritativeDNSSnapshotVersion(snapshot *AuthoritativeDNSSnapshot) (string, error) {
	payload := struct {
		Zones  []AuthoritativeDNSSnapshotZone  `json:"zones"`
		Routes []AuthoritativeDNSSnapshotRoute `json:"routes"`
		Nodes  []AuthoritativeDNSSnapshotNode  `json:"nodes"`
	}{
		Zones:  snapshot.Zones,
		Routes: snapshot.Routes,
		Nodes:  snapshot.Nodes,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:24], nil
}

func recordDNSWorkerSnapshotPull(worker *model.DNSWorker, version string) error {
	if worker == nil {
		return nil
	}
	now := time.Now()
	worker.Status = dnsWorkerStatusOnline
	worker.LastSeenAt = &now
	worker.LastSnapshotAt = &now
	worker.LastSnapshotVersion = strings.TrimSpace(version)
	return worker.Update()
}

func persistDNSWorkerProbeResult(worker *model.DNSWorker, view *DNSWorkerProbeView) error {
	if worker == nil || view == nil {
		return nil
	}
	raw, err := json.Marshal(view.Results)
	if err != nil {
		return err
	}
	checkedAt := view.CheckedAt
	worker.LastProbeAt = &checkedAt
	worker.LastProbeQuery = strings.TrimSpace(view.QueryName + " " + view.QueryType)
	worker.LastProbeResult = string(raw)
	return worker.UpdateProbeResult()
}

func persistDNSQueryRollups(workerID string, inputs []DNSQueryRollupInput) error {
	for _, input := range inputs {
		if input.QueryCount <= 0 {
			continue
		}
		targetSummary := input.TargetSummary
		if targetSummary == nil {
			targetSummary = map[string]int64{}
		}
		targetSummaryJSON, err := json.Marshal(targetSummary)
		if err != nil {
			return err
		}
		rollup := &model.DNSQueryRollup{
			WindowStart:     input.WindowStart,
			WindowMinutes:   normalizeDNSRollupWindow(input.WindowMinutes),
			WorkerID:        workerID,
			ZoneID:          input.ZoneID,
			ProxyRouteID:    input.ProxyRouteID,
			QName:           normalizeDNSRecordName(input.QName),
			QType:           normalizeAuthoritativeDNSRecordTypeOrDefault(input.QType),
			RCode:           normalizeDNSRCode(input.RCode),
			QueryCount:      input.QueryCount,
			TotalDurationMs: normalizeDNSDurationMs(input.TotalDurationMs),
			MaxDurationMs:   normalizeDNSDurationMs(input.MaxDurationMs),
			TargetSummary:   string(targetSummaryJSON),
		}
		if rollup.WindowStart.IsZero() {
			rollup.WindowStart = time.Now().UTC().Truncate(time.Minute)
		}
		if err := rollup.Insert(); err != nil {
			return err
		}
	}
	return nil
}

func decodeDNSTargetSummary(raw string) map[string]int64 {
	var result map[string]int64
	if strings.TrimSpace(raw) == "" {
		return map[string]int64{}
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]int64{}
	}
	for target, count := range result {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" || count <= 0 {
			delete(result, target)
			continue
		}
		if trimmed != target {
			delete(result, target)
			result[trimmed] += count
		}
	}
	return result
}

func normalizeDNSDurationMs(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func decodeDNSWorkerProbeResults(raw string) []DNSWorkerProbeResultView {
	if strings.TrimSpace(raw) == "" {
		return []DNSWorkerProbeResultView{}
	}
	var results []DNSWorkerProbeResultView
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return []DNSWorkerProbeResultView{}
	}
	cleaned := make([]DNSWorkerProbeResultView, 0, len(results))
	for _, result := range results {
		result.Network = strings.ToUpper(strings.TrimSpace(result.Network))
		if result.Network == "" {
			continue
		}
		if result.DurationMs < 0 {
			result.DurationMs = 0
		}
		result.RCode = strings.ToUpper(strings.TrimSpace(result.RCode))
		cleaned = append(cleaned, result)
	}
	return cleaned
}

type dnsWorkerProbeState struct {
	status     string
	healthy    bool
	ageSeconds int64
	message    string
}

func evaluateDNSWorkerProbeState(now time.Time, checkedAt *time.Time, results []DNSWorkerProbeResultView) dnsWorkerProbeState {
	if checkedAt == nil || len(results) == 0 {
		return dnsWorkerProbeState{
			status:  dnsWorkerProbeUnknown,
			message: "尚未执行公网 UDP/TCP 53 探测",
		}
	}
	checked := checkedAt.UTC()
	age := now.UTC().Sub(checked)
	ageSeconds := int64(0)
	if age > 0 {
		ageSeconds = int64(age.Seconds())
	}
	if age > defaultDNSWorkerProbeMaxAge {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeStale,
			ageSeconds: ageSeconds,
			message:    "最近一次公网探测已过期",
		}
	}
	reachableByNetwork := map[string]bool{}
	for _, result := range results {
		network := strings.ToUpper(strings.TrimSpace(result.Network))
		if network == "" {
			continue
		}
		reachableByNetwork[network] = result.Reachable
	}
	udpReachable := reachableByNetwork["UDP"]
	tcpReachable := reachableByNetwork["TCP"]
	switch {
	case udpReachable && tcpReachable:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeHealthy,
			healthy:    true,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 均可达",
		}
	case udpReachable || tcpReachable:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbePartial,
			ageSeconds: ageSeconds,
			message:    "仅部分 DNS 协议可达",
		}
	default:
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeFailed,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 探测均失败",
		}
	}
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
	for routeID := range counts {
		if routeID == 0 {
			continue
		}
		route := &model.ProxyRoute{}
		if err := model.DB.First(route, routeID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		label := normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain)
		if label == "" {
			label = fmt.Sprintf("Route %d", routeID)
		}
		labels[fmt.Sprint(routeID)] = label
	}
	return labels, nil
}

func normalizeDNSWorkerProbeAddress(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	if address == "" {
		return "", errors.New("DNS Worker public address is empty")
	}
	if strings.Contains(address, "://") {
		if parsed, err := dnsWorkerProbeURLHost(address); err == nil {
			address = parsed
		}
	}
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		host = strings.Trim(host, "[]")
		if strings.TrimSpace(host) == "" {
			return "", errors.New("DNS Worker public address host is empty")
		}
		if strings.TrimSpace(port) == "" {
			port = "53"
		}
		return net.JoinHostPort(host, port), nil
	}
	if strings.Count(address, ":") > 1 {
		if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
			return net.JoinHostPort(ip.String(), "53"), nil
		}
		return "", errors.New("DNS Worker IPv6 address must be wrapped as [addr]:port or be a valid IPv6 literal")
	}
	if strings.Contains(address, ":") {
		host, port, ok := strings.Cut(address, ":")
		if !ok || strings.TrimSpace(host) == "" {
			return "", errors.New("DNS Worker public address format is invalid")
		}
		if strings.TrimSpace(port) == "" {
			port = "53"
		}
		return net.JoinHostPort(strings.TrimSpace(host), strings.TrimSpace(port)), nil
	}
	return net.JoinHostPort(address, "53"), nil
}

func dnsWorkerProbeURLHost(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" {
		return parsed.Host, nil
	}
	return parsed.Path, nil
}

func dnsWorkerProbeQueryName(zoneID uint) (string, error) {
	if zoneID > 0 {
		zone, err := model.GetDNSZoneByID(zoneID)
		if err != nil {
			return "", err
		}
		return dns.Fqdn(zone.Name), nil
	}
	var zone model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("id asc").First(&zone).Error; err == nil {
		return dns.Fqdn(zone.Name), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	return ".", nil
}

func exchangeDNSWorkerProbe(ctx context.Context, target string, network string, qname string, qtype uint16, timeout time.Duration) DNSWorkerProbeResultView {
	result := DNSWorkerProbeResultView{
		Network: strings.ToUpper(network),
	}
	if timeout <= 0 {
		timeout = defaultDNSWorkerProbeTimeout
	}
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(qname), qtype)
	client := &dns.Client{
		Net:     network,
		Timeout: timeout,
	}
	startedAt := time.Now()
	response, _, err := client.ExchangeContext(ctx, message, target)
	result.DurationMs = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if response == nil {
		result.Error = "empty DNS response"
		return result
	}
	result.Reachable = true
	result.RCode = dns.RcodeToString[response.Rcode]
	if result.RCode == "" {
		result.RCode = fmt.Sprintf("RCODE%d", response.Rcode)
	}
	result.AnswerCount = len(response.Answer)
	return result
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

func initDNSObservabilityTrendPoints(windowEnd time.Time, hours int) []DNSObservabilityTrendPointView {
	if hours <= 0 {
		hours = 24
	}
	end := windowEnd.UTC().Truncate(time.Hour)
	start := end.Add(-time.Duration(hours-1) * time.Hour)
	points := make([]DNSObservabilityTrendPointView, 0, hours)
	for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Hour) {
		points = append(points, DNSObservabilityTrendPointView{
			BucketStartedAt: bucket,
		})
	}
	return points
}

func applyDNSObservabilityTrendPoint(points []DNSObservabilityTrendPointView, windowStart time.Time, rcode string, dynamic bool, count int64) {
	if count <= 0 || len(points) == 0 {
		return
	}
	bucket := windowStart.UTC().Truncate(time.Hour)
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
		stale := status == dnsWorkerStatusOnline && (worker.LastSnapshotAt == nil || now.Sub(worker.LastSnapshotAt.UTC()) > snapshotMaxAge)
		workerName := strings.TrimSpace(worker.Name)
		if workerName == "" {
			workerName = worker.WorkerID
		}
		item := DNSWorkerSnapshotWorkerView{
			WorkerID:        worker.WorkerID,
			Name:            workerName,
			Status:          status,
			SnapshotVersion: snapshotVersion,
			LastSnapshotAt:  worker.LastSnapshotAt,
			LastSeenAt:      worker.LastSeenAt,
			Stale:           stale,
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
		if worker.LastSnapshotAt != nil && (group.LatestSnapshotAt == nil || worker.LastSnapshotAt.After(*group.LatestSnapshotAt)) {
			latest := *worker.LastSnapshotAt
			group.LatestSnapshotAt = &latest
		}
		if snapshotVersion != "" && worker.LastSnapshotAt != nil && (view.LatestSnapshotAt == nil || worker.LastSnapshotAt.After(*view.LatestSnapshotAt)) {
			latest := *worker.LastSnapshotAt
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

func buildDNSWorkerHealthSummary(now time.Time, rollups []model.DNSQueryRollup) DNSWorkerHealthSummaryView {
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
	var totalQueries int64
	var totalErrors int64
	var totalDurationMs int64
	var maxDurationMs int64
	for _, rollup := range rollups {
		workerID := strings.TrimSpace(rollup.WorkerID)
		if workerID == "" || rollup.QueryCount <= 0 {
			continue
		}
		stats := statsByWorker[workerID]
		if stats == nil {
			stats = &dnsWorkerHealthStats{}
			statsByWorker[workerID] = stats
		}
		count := rollup.QueryCount
		stats.queryCount += count
		totalQueries += count
		rcode := normalizeDNSRCode(rollup.RCode)
		if rcode == "SERVFAIL" || rcode == "REFUSED" {
			stats.errorQueries += count
			totalErrors += count
		}
		durationMs := normalizeDNSDurationMs(rollup.TotalDurationMs)
		stats.totalDurationMs += durationMs
		totalDurationMs += durationMs
		if rollup.MaxDurationMs > stats.maxDurationMs {
			stats.maxDurationMs = normalizeDNSDurationMs(rollup.MaxDurationMs)
		}
		if rollup.MaxDurationMs > maxDurationMs {
			maxDurationMs = normalizeDNSDurationMs(rollup.MaxDurationMs)
		}
	}

	view.TotalWorkerCount = len(workers)
	view.MaxLatencyMs = maxDurationMs
	view.AverageLatencyMs = averageMilliseconds(totalDurationMs, totalQueries)
	view.ErrorRatePercent = ratioPercent(totalErrors, totalQueries)

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
		snapshotAgeSeconds := int64(0)
		if worker.LastSnapshotAt != nil {
			age := now.Sub(worker.LastSnapshotAt.UTC())
			if age > 0 {
				snapshotAgeSeconds = int64(age.Seconds())
			}
		}
		snapshotStale := status == dnsWorkerStatusOnline && (worker.LastSnapshotAt == nil || now.Sub(worker.LastSnapshotAt.UTC()) > snapshotMaxAge)
		probeResults := decodeDNSWorkerProbeResults(worker.LastProbeResult)
		probeState := evaluateDNSWorkerProbeState(now, worker.LastProbeAt, probeResults)
		if probeState.status != dnsWorkerProbeUnknown {
			view.ProbeCheckedCount++
		}
		if probeState.healthy {
			view.ProbeHealthyCount++
		}
		view.Workers = append(view.Workers, DNSWorkerHealthItemView{
			WorkerID:           worker.WorkerID,
			Name:               workerName,
			Status:             status,
			PublicAddress:      worker.PublicAddress,
			QueryCount:         stats.queryCount,
			ErrorQueries:       stats.errorQueries,
			ErrorRatePercent:   ratioPercent(stats.errorQueries, stats.queryCount),
			AverageLatencyMs:   averageMilliseconds(stats.totalDurationMs, stats.queryCount),
			MaxLatencyMs:       stats.maxDurationMs,
			LastSeenAt:         worker.LastSeenAt,
			LastSnapshotAt:     worker.LastSnapshotAt,
			SnapshotAgeSeconds: snapshotAgeSeconds,
			SnapshotStale:      snapshotStale,
			LastError:          worker.LastError,
			LastProbeAt:        worker.LastProbeAt,
			LastProbeResults:   probeResults,
			ProbeStatus:        probeState.status,
			ProbeHealthy:       probeState.healthy,
			ProbeAgeSeconds:    probeState.ageSeconds,
			ProbeMessage:       probeState.message,
		})
	}
	if view.TotalWorkerCount > 0 {
		view.AvailabilityPercent = ratioPercent(int64(view.OnlineWorkerCount), int64(view.TotalWorkerCount))
	}
	if view.ProbeCheckedCount > 0 {
		view.ProbeHealthyPercent = ratioPercent(int64(view.ProbeHealthyCount), int64(view.ProbeCheckedCount))
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

func ratioPercent(numerator int64, denominator int64) float64 {
	if denominator <= 0 || numerator <= 0 {
		return 0
	}
	return (float64(numerator) / float64(denominator)) * 100
}

func normalizeDNSNameServerSet(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		ns := normalizeDNSRecordName(value)
		if ns == "" {
			continue
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		result = append(result, ns)
	}
	sort.Strings(result)
	return result
}

func compareNameServerSets(expected []string, actual []string) ([]string, []string, []string) {
	expected = normalizeDNSNameServerSet(expected)
	actual = normalizeDNSNameServerSet(actual)
	actualSet := make(map[string]struct{}, len(actual))
	for _, ns := range actual {
		actualSet[ns] = struct{}{}
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, ns := range expected {
		expectedSet[ns] = struct{}{}
	}
	matched := make([]string, 0)
	missing := make([]string, 0)
	extra := make([]string, 0)
	for _, ns := range expected {
		if _, ok := actualSet[ns]; ok {
			matched = append(matched, ns)
		} else {
			missing = append(missing, ns)
		}
	}
	for _, ns := range actual {
		if _, ok := expectedSet[ns]; !ok {
			extra = append(extra, ns)
		}
	}
	return matched, missing, extra
}

func dnsDelegationStatus(expected []string, actual []string, matched []string, missing []string, extra []string) string {
	if len(expected) == 0 {
		return dnsDelegationNotConfig
	}
	if len(actual) == 0 {
		return dnsDelegationMismatch
	}
	if len(missing) == 0 && len(extra) == 0 {
		return dnsDelegationMatched
	}
	if len(matched) > 0 {
		return dnsDelegationPartial
	}
	return dnsDelegationMismatch
}

func glueNameServersForZone(zoneName string, nameServers []string) []string {
	zoneName = normalizeDNSRecordName(zoneName)
	result := make([]string, 0, len(nameServers))
	for _, nameServer := range normalizeDNSNameServerSet(nameServers) {
		if domainBelongsToZone(nameServer, zoneName) {
			result = append(result, nameServer)
		}
	}
	return result
}

func normalizeDNSZoneName(raw string) (string, error) {
	name := normalizeDNSRecordName(raw)
	if name == "" {
		return "", errors.New("DNS zone name cannot be empty")
	}
	if strings.HasPrefix(name, "*.") || !isValidProxyRouteDomain(name) {
		return "", errors.New("DNS zone name format is invalid")
	}
	return name, nil
}

func normalizeNameServers(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		ns := normalizeDNSRecordName(value)
		if ns == "" {
			continue
		}
		if !isValidProxyRouteDomain(ns) {
			return nil, errors.New("name_servers contains invalid domain")
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		result = append(result, ns)
	}
	return result, nil
}

func normalizeSOAEmail(raw string, zoneName string) string {
	email := strings.TrimSpace(raw)
	if email != "" {
		return email
	}
	return "hostmaster@" + zoneName
}

func normalizeDNSZoneTTL(value int) int {
	if value <= 0 {
		return defaultDNSZoneTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeTTL(value int, fallback int) int {
	if fallback <= 0 {
		fallback = defaultDNSZoneTTL
	}
	if value <= 0 {
		return fallback
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeAuthoritativeRouteTTL(value int) int {
	defaultTTL := authoritativeDNSDefaultTTL()
	if value <= 1 {
		return defaultTTL
	}
	if value < defaultTTL {
		return defaultTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func authoritativeDNSDefaultTTL() int {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = strings.TrimSpace(common.OptionMap["AuthoritativeDNSDefaultTTL"])
	}
	common.OptionMapRWMutex.RUnlock()
	if raw == "" {
		return common.AuthoritativeDNSDefaultTTL
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return common.AuthoritativeDNSDefaultTTL
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func authoritativeDNSSnapshotMaxAge() time.Duration {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = strings.TrimSpace(common.OptionMap["AuthoritativeDNSSnapshotMaxAge"])
	}
	common.OptionMapRWMutex.RUnlock()
	if raw == "" {
		if common.AuthoritativeDNSSnapshotMaxAge > 0 {
			return time.Duration(common.AuthoritativeDNSSnapshotMaxAge) * time.Second
		}
		return defaultDNSSnapshotMaxAge
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultDNSSnapshotMaxAge
	}
	return time.Duration(value) * time.Second
}

func normalizeAuthoritativeDNSRecordType(raw string) (string, error) {
	recordType := strings.ToUpper(strings.TrimSpace(raw))
	switch recordType {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SOA":
		return recordType, nil
	default:
		return "", errors.New("unsupported DNS record type")
	}
}

func normalizeAuthoritativeDNSRecordTypeOrDefault(raw string) string {
	recordType, err := normalizeAuthoritativeDNSRecordType(raw)
	if err != nil {
		return "A"
	}
	return recordType
}

func normalizeAuthoritativeDNSRecordName(zoneName string, raw string) (string, error) {
	zoneName = normalizeDNSRecordName(zoneName)
	name := normalizeDNSRecordName(raw)
	if name == "" || name == "@" {
		name = zoneName
	} else if !strings.Contains(name, ".") {
		name += "." + zoneName
	}
	if !isValidProxyRouteDomain(name) {
		return "", errors.New("DNS record name format is invalid")
	}
	if !domainBelongsToZone(name, zoneName) {
		return "", errors.New("DNS record name is outside the zone")
	}
	return name, nil
}

func normalizeAuthoritativeDNSRecordValue(recordType string, raw string, priority int) (string, int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", 0, errors.New("DNS record value cannot be empty")
	}
	switch recordType {
	case "A":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil {
			return "", 0, errors.New("A record value must be an IPv4 address")
		}
		return ip.String(), 0, nil
	case "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() != nil {
			return "", 0, errors.New("AAAA record value must be an IPv6 address")
		}
		return ip.String(), 0, nil
	case "CNAME", "NS":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, fmt.Errorf("%s record value must be a domain name", recordType)
		}
		return target, 0, nil
	case "MX":
		target := normalizeDNSRecordName(value)
		if target == "" || !isValidProxyRouteDomain(target) || net.ParseIP(target) != nil {
			return "", 0, errors.New("MX record value must be a mail server domain name")
		}
		if priority < 0 {
			priority = 0
		}
		return target, priority, nil
	case "TXT", "SOA":
		return value, priority, nil
	default:
		return "", 0, errors.New("unsupported DNS record type")
	}
}

func bumpDNSZoneSerial(zoneID uint) error {
	zone, err := model.GetDNSZoneByID(zoneID)
	if err != nil {
		return err
	}
	zone.Serial = nextDNSZoneSerial(zone.Serial)
	return zone.Update()
}

func nextDNSZoneSerial(current uint64) uint64 {
	next := uint64(time.Now().UTC().Unix())
	if next <= current {
		return current + 1
	}
	return next
}

func normalizeDNSWorkerStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dnsWorkerStatusOnline:
		return dnsWorkerStatusOnline
	default:
		return dnsWorkerStatusOffline
	}
}

func normalizeDNSRollupWindow(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func normalizeDNSRCode(raw string) string {
	rcode := strings.ToUpper(strings.TrimSpace(raw))
	switch rcode {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return rcode
	default:
		return "NOERROR"
	}
}

func publicNodeIPsForSnapshot(node *model.Node) []string {
	ips := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range resolveNodePublicIPs(node) {
		ip := iputil.NormalizeIP(value)
		if ip == "" || !iputil.IsPublicString(ip) {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

func ptrDNSRecordView(view DNSRecordView) *DNSRecordView {
	return &view
}

func ptrDNSWorkerView(view DNSWorkerView) *DNSWorkerView {
	return &view
}
