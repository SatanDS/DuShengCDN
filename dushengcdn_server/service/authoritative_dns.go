package service

import (
	"context"
	"crypto/sha256"
	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	dnsWorkerStatusOnline            = "online"
	dnsWorkerStatusOffline           = "offline"
	dnsDelegationMatched             = "matched"
	dnsDelegationPartial             = "partial"
	dnsDelegationMismatch            = "mismatch"
	dnsDelegationFailed              = "failed"
	dnsDelegationNotConfig           = "not_configured"
	dnsSnapshotConsistent            = "consistent"
	dnsSnapshotDivergent             = "divergent"
	dnsSnapshotStale                 = "stale"
	dnsSnapshotNoOnline              = "no_online_workers"
	dnsSnapshotUnknown               = "unknown"
	dnsWorkerProbeHealthy            = "healthy"
	dnsWorkerProbePartial            = "partial"
	dnsWorkerProbeFailed             = "failed"
	dnsWorkerProbeStale              = "stale"
	dnsWorkerProbeUnknown            = "unknown"
	defaultDNSZoneTTL                = 300
	defaultDNSSnapshotMaxAge         = 5 * time.Minute
	defaultDNSWorkerProbeTimeout     = 3 * time.Second
	defaultDNSWorkerProbeMaxAge      = 24 * time.Hour
	defaultDNSWorkerNodeProbeMaxAge  = 5 * time.Minute
	defaultDNSMaxRollupWindowMinutes = 1440
)

var dnsLookupNS = net.LookupNS
var dnsWorkerProbeExchange = exchangeDNSWorkerProbe
var dnsObservabilityHeavyCounterScanLimit = 20000

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
	Version             string                                    `json:"version"`
	Status              string                                    `json:"status"`
	LastSnapshotVersion string                                    `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time                                `json:"last_snapshot_at"`
	LastError           string                                    `json:"last_error"`
	GeoIPEnabled        bool                                      `json:"geoip_enabled"`
	GeoIPDatabasePath   string                                    `json:"geoip_database_path"`
	GeoIPLastError      string                                    `json:"geoip_last_error"`
	Rollups             []DNSQueryRollupInput                     `json:"rollups"`
	SchedulingStates    []AuthoritativeDNSSnapshotSchedulingState `json:"scheduling_states,omitempty"`
}

type DNSQueryRollupInput struct {
	WindowStart     time.Time        `json:"window_start"`
	WindowMinutes   int              `json:"window_minutes"`
	ZoneID          uint             `json:"zone_id"`
	ProxyRouteID    uint             `json:"proxy_route_id"`
	SourceScope     string           `json:"source_scope"`
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
	GeoIPEnabled        bool                       `json:"geoip_enabled"`
	GeoIPDatabasePath   string                     `json:"geoip_database_path"`
	GeoIPLastError      string                     `json:"geoip_last_error"`
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

type DNSGSLBSimulationInput struct {
	ProxyRouteID uint   `json:"proxy_route_id"`
	QName        string `json:"qname"`
	RecordType   string `json:"record_type"`
	Country      string `json:"country"`
	SourceIP     string `json:"source_ip"`
	Fresh        *bool  `json:"fresh"`
}

type AuthoritativeDNSMigrationInput struct {
	DNSZoneIDRef *uint `json:"dns_zone_id_ref"`
}

type DNSGSLBSimulationView struct {
	ProxyRouteID    uint                        `json:"proxy_route_id"`
	SiteName        string                      `json:"site_name"`
	QName           string                      `json:"qname"`
	RecordType      string                      `json:"record_type"`
	Country         string                      `json:"country"`
	SourceIP        string                      `json:"source_ip"`
	SourceScope     string                      `json:"source_scope"`
	TTL             int                         `json:"ttl"`
	Targets         []string                    `json:"targets"`
	TargetCount     int                         `json:"target_count"`
	Strategy        string                      `json:"strategy"`
	GSLBEnabled     bool                        `json:"gslb_enabled"`
	SnapshotVersion string                      `json:"snapshot_version"`
	SnapshotAt      time.Time                   `json:"snapshot_at"`
	Message         string                      `json:"message"`
	MatchedPools    []DNSGSLBSimulationPoolView `json:"matched_pools"`
	Nodes           []DNSGSLBSimulationNodeView `json:"nodes"`
}

type DNSGSLBSimulationPoolView struct {
	Name        string   `json:"name"`
	Weight      int      `json:"weight"`
	Countries   []string `json:"countries"`
	SourceCIDRs []string `json:"source_cidrs"`
	Matched     bool     `json:"matched"`
	Reason      string   `json:"reason"`
}

type DNSGSLBSimulationNodeView struct {
	NodeID                  string     `json:"node_id"`
	Name                    string     `json:"name"`
	PoolName                string     `json:"pool_name"`
	Status                  string     `json:"status"`
	OpenrestyStatus         string     `json:"openresty_status"`
	SchedulingEnabled       bool       `json:"scheduling_enabled"`
	DrainMode               bool       `json:"drain_mode"`
	LastSeenAt              *time.Time `json:"last_seen_at"`
	PublicIPs               []string   `json:"public_ips"`
	CandidateTargets        []string   `json:"candidate_targets"`
	SelectedTargets         []string   `json:"selected_targets"`
	Eligible                bool       `json:"eligible"`
	Selected                bool       `json:"selected"`
	Reasons                 []string   `json:"reasons"`
	HasMetric               bool       `json:"has_metric"`
	MetricCapturedAt        *time.Time `json:"metric_captured_at,omitempty"`
	OpenrestyConnections    int64      `json:"openresty_connections"`
	CPUUsagePercent         float64    `json:"cpu_usage_percent"`
	MemoryUsagePercent      float64    `json:"memory_usage_percent"`
	Score                   float64    `json:"score"`
	NodeProbeStatus         string     `json:"node_probe_status"`
	NodeProbeMessage        string     `json:"node_probe_message"`
	NodeProbeCheckedCount   int        `json:"node_probe_checked_count"`
	NodeProbeHealthyCount   int        `json:"node_probe_healthy_count"`
	NodeProbeStaleCount     int        `json:"node_probe_stale_count"`
	NodeProbeHealthyPercent float64    `json:"node_probe_healthy_percent"`
	NodeProbeAverageRTTMs   float64    `json:"node_probe_average_rtt_ms"`
	NodeProbeMaxRTTMs       int64      `json:"node_probe_max_rtt_ms"`
}

type AuthoritativeDNSMigrationCandidateView struct {
	ProxyRouteID               uint     `json:"proxy_route_id"`
	SiteName                   string   `json:"site_name"`
	PrimaryDomain              string   `json:"primary_domain"`
	Domains                    []string `json:"domains"`
	Enabled                    bool     `json:"enabled"`
	DNSAutoSync                bool     `json:"dns_auto_sync"`
	DNSProviderMode            string   `json:"dns_provider_mode"`
	DNSRecordType              string   `json:"dns_record_type"`
	GSLBEnabled                bool     `json:"gslb_enabled"`
	MatchingZoneID             *uint    `json:"matching_zone_id"`
	MatchingZoneName           string   `json:"matching_zone_name"`
	MatchingZoneEnabled        bool     `json:"matching_zone_enabled"`
	TotalWorkerCount           int      `json:"total_worker_count"`
	OnlineWorkerCount          int      `json:"online_worker_count"`
	PublicReachableWorkerCount int      `json:"public_reachable_worker_count"`
	FreshSnapshotWorkerCount   int      `json:"fresh_snapshot_worker_count"`
	ReadyWorkerCount           int      `json:"ready_worker_count"`
	Ready                      bool     `json:"ready"`
	Blockers                   []string `json:"blockers"`
	Warnings                   []string `json:"warnings"`
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

type DNSGSLBSchedulingStatesView struct {
	CheckedAt time.Time                    `json:"checked_at"`
	Total     int                          `json:"total"`
	States    []DNSGSLBSchedulingStateView `json:"states"`
}

type DNSGSLBSchedulingStateView struct {
	ID                 uint       `json:"id"`
	ProxyRouteID       uint       `json:"proxy_route_id"`
	SiteName           string     `json:"site_name"`
	PrimaryDomain      string     `json:"primary_domain"`
	Domains            []string   `json:"domains"`
	RouteEnabled       bool       `json:"route_enabled"`
	RouteAuthoritative bool       `json:"route_authoritative"`
	RouteGSLBEnabled   bool       `json:"route_gslb_enabled"`
	RouteRecordType    string     `json:"route_record_type"`
	RecordType         string     `json:"record_type"`
	ScopeKey           string     `json:"scope_key"`
	SelectedTargets    []string   `json:"selected_targets"`
	DesiredTargets     []string   `json:"desired_targets"`
	LastReason         string     `json:"last_reason"`
	LastChangedAt      *time.Time `json:"last_changed_at"`
	LastEvaluatedAt    *time.Time `json:"last_evaluated_at"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type DNSObservabilityCounterView struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type DNSObservabilitySummaryView struct {
	WindowHours          int                              `json:"window_hours"`
	WindowStart          time.Time                        `json:"window_start"`
	WindowEnd            time.Time                        `json:"window_end"`
	LastRollupAt         *time.Time                       `json:"last_rollup_at"`
	TotalQueries         int64                            `json:"total_queries"`
	SuccessfulQueries    int64                            `json:"successful_queries"`
	NegativeQueries      int64                            `json:"negative_queries"`
	ErrorQueries         int64                            `json:"error_queries"`
	DynamicQueries       int64                            `json:"dynamic_queries"`
	StaticQueries        int64                            `json:"static_queries"`
	RCodeBreakdown       []DNSObservabilityCounterView    `json:"rcode_breakdown"`
	QTypeBreakdown       []DNSObservabilityCounterView    `json:"qtype_breakdown"`
	TopQNames            []DNSObservabilityCounterView    `json:"top_qnames"`
	TopTargets           []DNSObservabilityCounterView    `json:"top_targets"`
	WorkerBreakdown      []DNSObservabilityCounterView    `json:"worker_breakdown"`
	ZoneBreakdown        []DNSObservabilityCounterView    `json:"zone_breakdown"`
	RouteBreakdown       []DNSObservabilityCounterView    `json:"route_breakdown"`
	SourceScopeBreakdown []DNSObservabilityCounterView    `json:"source_scope_breakdown"`
	TrendPoints          []DNSObservabilityTrendPointView `json:"trend_points"`
	SnapshotConsistency  DNSWorkerSnapshotConsistencyView `json:"snapshot_consistency"`
	WorkerHealth         DNSWorkerHealthSummaryView       `json:"worker_health"`
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
	rcodeCounts         map[string]int64
	qtypeCounts         map[string]int64
	qnameCounts         map[string]int64
	workerCounts        map[string]int64
	sourceScopeCounts   map[string]int64
	zoneCounts          map[uint]int64
	routeCounts         map[uint]int64
	targetCounts        map[string]int64
	trendPoints         []DNSObservabilityTrendPointView
	workerHealthRollups []dnsWorkerHealthRollupRow
	lastRollupAt        *time.Time
}

type dnsObservabilitySummaryQueries struct {
	queryStringCounts        func(DNSObservabilitySummaryInput, dnsObservabilityWindow, string, int) (map[string]int64, error)
	queryUintCounts          func(DNSObservabilitySummaryInput, dnsObservabilityWindow, string, int, string) (map[uint]int64, error)
	queryTopTargets          func(DNSObservabilitySummaryInput, dnsObservabilityWindow, int) (map[string]int64, error)
	queryTrendPoints         func(DNSObservabilitySummaryInput, dnsObservabilityWindow) ([]DNSObservabilityTrendPointView, error)
	queryWorkerHealthRollups func(DNSObservabilitySummaryInput, dnsObservabilityWindow) ([]dnsWorkerHealthRollupRow, error)
	queryLastRollupAt        func(DNSObservabilitySummaryInput, dnsObservabilityWindow) (*time.Time, error)
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
	queryStringCounts:        queryDNSObservabilityStringCounts,
	queryUintCounts:          queryDNSObservabilityUintCounts,
	queryTopTargets:          queryDNSObservabilityTopTargets,
	queryTrendPoints:         queryDNSObservabilityTrendPoints,
	queryWorkerHealthRollups: queryDNSWorkerHealthRollups,
	queryLastRollupAt:        queryDNSObservabilityLastRollupAt,
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
	WorkerID        string     `json:"worker_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	SnapshotVersion string     `json:"snapshot_version"`
	LastSnapshotAt  *time.Time `json:"last_snapshot_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	Stale           bool       `json:"stale"`
	GeoIPEnabled    bool       `json:"geoip_enabled"`
	GeoIPLastError  string     `json:"geoip_last_error"`
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
	WorkerID                string                     `json:"worker_id"`
	Name                    string                     `json:"name"`
	Status                  string                     `json:"status"`
	PublicAddress           string                     `json:"public_address"`
	QueryCount              int64                      `json:"query_count"`
	ErrorQueries            int64                      `json:"error_queries"`
	ErrorRatePercent        float64                    `json:"error_rate_percent"`
	AverageLatencyMs        float64                    `json:"average_latency_ms"`
	MaxLatencyMs            int64                      `json:"max_latency_ms"`
	LastSeenAt              *time.Time                 `json:"last_seen_at"`
	LastSnapshotAt          *time.Time                 `json:"last_snapshot_at"`
	SnapshotAgeSeconds      int64                      `json:"snapshot_age_seconds"`
	SnapshotStale           bool                       `json:"snapshot_stale"`
	GeoIPEnabled            bool                       `json:"geoip_enabled"`
	GeoIPDatabasePath       string                     `json:"geoip_database_path"`
	GeoIPLastError          string                     `json:"geoip_last_error"`
	LastError               string                     `json:"last_error"`
	LastProbeAt             *time.Time                 `json:"last_probe_at"`
	LastProbeResults        []DNSWorkerProbeResultView `json:"last_probe_results"`
	ProbeStatus             string                     `json:"probe_status"`
	ProbeHealthy            bool                       `json:"probe_healthy"`
	ProbeAgeSeconds         int64                      `json:"probe_age_seconds"`
	ProbeMessage            string                     `json:"probe_message"`
	NodeProbeTotalCount     int                        `json:"node_probe_total_count"`
	NodeProbeHealthyCount   int                        `json:"node_probe_healthy_count"`
	NodeProbeStaleCount     int                        `json:"node_probe_stale_count"`
	NodeProbeHealthyPercent float64                    `json:"node_probe_healthy_percent"`
	NodeProbeAverageRTTMs   float64                    `json:"node_probe_average_rtt_ms"`
	NodeProbeMaxRTTMs       int64                      `json:"node_probe_max_rtt_ms"`
	NodeProbes              []DNSWorkerNodeProbeView   `json:"node_probes"`
}

type DNSWorkerNodeProbeView struct {
	NodeID          string                     `json:"node_id"`
	NodeName        string                     `json:"node_name"`
	PoolName        string                     `json:"pool_name"`
	Status          string                     `json:"status"`
	CheckedAt       time.Time                  `json:"checked_at"`
	Healthy         bool                       `json:"healthy"`
	ProbeStatus     string                     `json:"probe_status"`
	ProbeAgeSeconds int64                      `json:"probe_age_seconds"`
	ProbeMessage    string                     `json:"probe_message"`
	AverageRTTMs    float64                    `json:"average_rtt_ms"`
	MaxRTTMs        int64                      `json:"max_rtt_ms"`
	Results         []DNSWorkerProbeResultView `json:"results"`
	LastError       string                     `json:"last_error"`
	FailureSamples  int                        `json:"failure_samples"`
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
	SnapshotVersion            string                                    `json:"snapshot_version"`
	GeneratedAt                time.Time                                 `json:"generated_at"`
	GSLBProbeSchedulingEnabled bool                                      `json:"gslb_probe_scheduling_enabled"`
	Zones                      []AuthoritativeDNSSnapshotZone            `json:"zones"`
	Routes                     []AuthoritativeDNSSnapshotRoute           `json:"routes"`
	Nodes                      []AuthoritativeDNSSnapshotNode            `json:"nodes"`
	SchedulingStates           []AuthoritativeDNSSnapshotSchedulingState `json:"scheduling_states,omitempty"`
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
	DDOSActive     bool                 `json:"ddos_active,omitempty"`
	DDOSProvider   string               `json:"ddos_provider,omitempty"`
	DDOSTarget     string               `json:"ddos_target,omitempty"`
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
	DNSProbeHealthy      bool       `json:"dns_probe_healthy"`
	DNSProbeCheckedCount int        `json:"dns_probe_checked_count"`
	DNSProbeHealthyCount int        `json:"dns_probe_healthy_count"`
	DNSProbeStaleCount   int        `json:"dns_probe_stale_count"`
	DNSProbeAverageRTTMs float64    `json:"dns_probe_average_rtt_ms"`
	DNSProbeMaxRTTMs     int64      `json:"dns_probe_max_rtt_ms"`
}

type AuthoritativeDNSSnapshotSchedulingState struct {
	RouteID         uint       `json:"route_id"`
	RecordType      string     `json:"record_type"`
	ScopeKey        string     `json:"scope_key"`
	SelectedTargets []string   `json:"selected_targets"`
	DesiredTargets  []string   `json:"desired_targets"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`
}

func ListAuthoritativeDNSZones() ([]DNSZoneView, error) {
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	recordCounts, err := dnsZoneRecordCountMap(zones)
	if err != nil {
		return nil, err
	}
	views := make([]DNSZoneView, 0, len(zones))
	for _, zone := range zones {
		var recordCount int64
		if zone != nil {
			recordCount = recordCounts[zone.ID]
		}
		view, err := buildDNSZoneViewWithRecordCount(zone, false, recordCount)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func dnsZoneRecordCountMap(zones []*model.DNSZone) (map[uint]int64, error) {
	zoneIDs := make([]uint, 0, len(zones))
	for _, zone := range zones {
		if zone != nil && zone.ID != 0 {
			zoneIDs = append(zoneIDs, zone.ID)
		}
	}
	rows, err := model.ListDNSRecordCountsByZoneIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.ZoneID] = row.Count
	}
	return counts, nil
}

func GetAuthoritativeDNSZone(id uint) (*DNSZoneView, error) {
	zone, err := model.GetDNSZoneByID(id)
	if err != nil {
		return nil, err
	}
	return buildDNSZoneView(zone, true)
}

func CreateAuthoritativeDNSZone(input DNSZoneInput) (*DNSZoneView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
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
	var certificateCount int64
	if err := model.DB.Model(&model.TLSCertificate{}).Where("dns_provider_mode = ? AND dns_zone_id_ref = ?", DNSProviderModeAuthoritative, id).Count(&certificateCount).Error; err != nil {
		return err
	}
	if certificateCount > 0 {
		return errors.New("DNS zone is used by certificate validation")
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
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
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
	if err := validateAuthoritativeDNSRecordDynamicConflicts(record); err != nil {
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
	if err := validateAuthoritativeDNSRecordDynamicConflicts(record); err != nil {
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

func ListAuthoritativeDNSMigrationCandidates() ([]AuthoritativeDNSMigrationCandidateView, error) {
	return listAuthoritativeDNSMigrationCandidatesWithQueries(defaultGSLBDNSSchedulingDataQueries)
}

func listAuthoritativeDNSMigrationCandidatesWithQueries(schedulingQueries gslbDNSSchedulingDataQueries) ([]AuthoritativeDNSMigrationCandidateView, error) {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return nil, err
	}
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	workerStats, err := authoritativeDNSMigrationWorkerStats()
	if err != nil {
		return nil, err
	}
	schedulingOptions := authoritativeDNSSchedulingOptions()
	schedulingData, err := loadGSLBDNSSchedulingDataWithQueries(true, schedulingQueries)
	if err != nil {
		return nil, err
	}
	schedulingOptions.Data = schedulingData
	candidates := make([]AuthoritativeDNSMigrationCandidateView, 0, len(routes))
	for _, route := range routes {
		if route == nil || normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeAuthoritative {
			continue
		}
		if !route.Enabled && !route.DNSAutoSync && !route.GSLBEnabled {
			continue
		}
		candidate, err := buildAuthoritativeDNSMigrationCandidate(route, zones, workerStats, schedulingOptions)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Ready != candidates[j].Ready {
			return candidates[i].Ready
		}
		if candidates[i].SiteName != candidates[j].SiteName {
			return candidates[i].SiteName < candidates[j].SiteName
		}
		return candidates[i].ProxyRouteID < candidates[j].ProxyRouteID
	})
	return candidates, nil
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

func SwitchProxyRouteToAuthoritativeDNS(id uint, input AuthoritativeDNSMigrationInput) (*ProxyRouteView, error) {
	route, err := model.GetProxyRouteByID(id)
	if err != nil {
		return nil, err
	}
	if route.DNSProviderMode == DNSProviderModeAuthoritative {
		return buildProxyRouteView(route)
	}

	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return nil, err
	}
	zone, err := resolveAuthoritativeMigrationZone(input.DNSZoneIDRef, domains)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err != nil {
		return nil, err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("authoritative DNS migration only supports A/AAAA dynamic records")
	}
	if route.GSLBEnabled {
		if _, err := decodeStoredGSLBPolicy(route.GSLBPolicy); err != nil {
			return nil, err
		}
	}
	if err := validateAuthoritativeProxyRouteStaticRecordConflicts(zone.ID, domains, recordType, route.Enabled); err != nil {
		return nil, err
	}
	if _, err := precheckAuthoritativeRouteDNSTargets(route, recordType); err != nil {
		return nil, err
	}

	now := time.Now()
	zoneID := zone.ID
	route.DNSProviderMode = DNSProviderModeAuthoritative
	route.DNSZoneIDRef = &zoneID
	route.DNSAutoSync = false
	route.DNSAccountID = nil
	route.DNSZoneID = ""
	route.DNSRecordType = recordType
	route.DNSRecordContent = ""
	route.DNSRecordIDs = "{}"
	route.DNSAutoTarget = true
	route.DNSTargetCount = normalizeDNSTargetCount(route.DNSTargetCount)
	route.DNSScheduleMode = normalizeDNSScheduleMode(route.DNSScheduleMode)
	route.DNSTTL = normalizeDNSTTL(route.DNSTTL)
	route.CloudflareProxied = false
	route.DDOSProtectionMode = DDOSProtectionModeOff
	route.DDOSProtectionProvider = DDOSProtectionProviderCloudflare
	route.DDOSProtectionTarget = ""
	route.DNSLastSyncStatus = DNSRecordSyncStatusSuccess
	route.DNSLastSyncMessage = fmt.Sprintf("已切换到本地自建解析托管域名 %s；请在注册商确认 NS 指向。", zone.Name)
	route.DNSLastSyncedAt = &now
	if err := model.DB.Model(route).Select(
		"dns_provider_mode",
		"dns_zone_id_ref",
		"dns_auto_sync",
		"dns_account_id",
		"dns_zone_id",
		"dns_record_type",
		"dns_record_content",
		"dns_record_ids",
		"dns_auto_target",
		"dns_target_count",
		"dns_schedule_mode",
		"dns_ttl",
		"cloudflare_proxied",
		"ddos_protection_mode",
		"ddos_protection_provider",
		"ddos_protection_target",
		"dns_last_sync_status",
		"dns_last_sync_message",
		"dns_last_synced_at",
	).Updates(route).Error; err != nil {
		return nil, err
	}
	return buildProxyRouteView(route)
}

func ListAuthoritativeDNSGSLBSchedulingStates() (*DNSGSLBSchedulingStatesView, error) {
	var states []*model.GSLBSchedulingState
	if err := model.DB.
		Order("proxy_route_id asc, dns_record_type asc, scope_key asc").
		Find(&states).Error; err != nil {
		return nil, err
	}

	routeIDs := make([]uint, 0, len(states))
	seenRoutes := make(map[uint]struct{}, len(states))
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		if _, ok := seenRoutes[state.ProxyRouteID]; ok {
			continue
		}
		seenRoutes[state.ProxyRouteID] = struct{}{}
		routeIDs = append(routeIDs, state.ProxyRouteID)
	}

	routesByID := make(map[uint]*model.ProxyRoute, len(routeIDs))
	if len(routeIDs) > 0 {
		var routes []*model.ProxyRoute
		if err := model.DB.Where("id IN ?", routeIDs).Find(&routes).Error; err != nil {
			return nil, err
		}
		for _, route := range routes {
			if route == nil || route.ID == 0 {
				continue
			}
			routesByID[route.ID] = route
		}
	}

	view := &DNSGSLBSchedulingStatesView{
		CheckedAt: time.Now().UTC(),
		States:    make([]DNSGSLBSchedulingStateView, 0, len(states)),
	}
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		view.States = append(view.States, buildDNSGSLBSchedulingStateView(state, routesByID[state.ProxyRouteID]))
	}
	view.Total = len(view.States)
	return view, nil
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
	for _, count := range data.routeCounts {
		summary.DynamicQueries += count
	}
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
	checkedAt := time.Now().UTC()
	summary.SnapshotConsistency = buildDNSWorkerSnapshotConsistency(checkedAt)
	summary.WorkerHealth = buildDNSWorkerHealthSummary(checkedAt, data.workerHealthRollups)
	return summary, nil
}

func loadDNSObservabilitySummaryQueryData(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, queries dnsObservabilitySummaryQueries) (*dnsObservabilitySummaryQueryData, error) {
	data := &dnsObservabilitySummaryQueryData{}
	if err := runConcurrentQueries(
		func() error {
			rows, err := queries.queryStringCounts(input, window, "rcode", 0)
			data.rcodeCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryStringCounts(input, window, "qtype", 0)
			data.qtypeCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryStringCounts(input, window, "qname", 8)
			data.qnameCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryStringCounts(input, window, "worker_id", 8)
			data.workerCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryStringCounts(input, window, "source_scope", 8)
			data.sourceScopeCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryUintCounts(input, window, "zone_id", 0, "zone_id > 0")
			data.zoneCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryUintCounts(input, window, "proxy_route_id", 0, "proxy_route_id > 0")
			data.routeCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryTopTargets(input, window, 8)
			data.targetCounts = rows
			return err
		},
		func() error {
			rows, err := queries.queryTrendPoints(input, window)
			data.trendPoints = rows
			return err
		},
		func() error {
			rows, err := queries.queryWorkerHealthRollups(input, window)
			data.workerHealthRollups = rows
			return err
		},
		func() error {
			value, err := queries.queryLastRollupAt(input, window)
			data.lastRollupAt = value
			return err
		},
	); err != nil {
		return nil, err
	}
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
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
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
	worker.LastSnapshotAt = normalizeDNSWorkerSnapshotAt(input.LastSnapshotAt, now)
	worker.LastSeenAt = &now
	worker.LastError = truncateForDatabase(strings.TrimSpace(input.LastError), 16000)
	worker.GeoIPEnabled = input.GeoIPEnabled
	worker.GeoIPDatabasePath = truncateForDatabase(strings.TrimSpace(input.GeoIPDatabasePath), 512)
	worker.GeoIPLastError = truncateForDatabase(strings.TrimSpace(input.GeoIPLastError), 16000)
	if err := worker.Update(); err != nil {
		return nil, err
	}
	if err := persistDNSWorkerSchedulingStates(input.SchedulingStates); err != nil {
		return nil, err
	}
	if err := persistDNSQueryRollups(worker.WorkerID, input.Rollups); err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func GetAuthoritativeDNSSnapshot(worker *model.DNSWorker) (*AuthoritativeDNSSnapshot, error) {
	return getAuthoritativeDNSSnapshotWithQueries(worker, defaultGSLBDNSSchedulingDataQueries)
}

func getAuthoritativeDNSSnapshotWithQueries(worker *model.DNSWorker, schedulingQueries gslbDNSSchedulingDataQueries) (*AuthoritativeDNSSnapshot, error) {
	zones, err := snapshotDNSZones()
	if err != nil {
		return nil, err
	}
	schedulingOptions := authoritativeDNSSchedulingOptions()
	schedulingData, err := loadGSLBDNSSchedulingDataWithQueries(true, schedulingQueries)
	if err != nil {
		return nil, err
	}
	schedulingOptions.Data = schedulingData
	routes, err := snapshotAuthoritativeRoutesWithOptions(schedulingOptions)
	if err != nil {
		return nil, err
	}
	nodes := snapshotNodesWithData(schedulingData)
	schedulingStates, err := snapshotGSLBSchedulingStates(routes)
	if err != nil {
		return nil, err
	}
	snapshot := &AuthoritativeDNSSnapshot{
		GeneratedAt:                time.Now().UTC(),
		GSLBProbeSchedulingEnabled: common.GSLBProbeSchedulingEnabled,
		Zones:                      zones,
		Routes:                     routes,
		Nodes:                      nodes,
		SchedulingStates:           schedulingStates,
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

func SimulateAuthoritativeDNSGSLB(input DNSGSLBSimulationInput) (*DNSGSLBSimulationView, error) {
	if input.ProxyRouteID == 0 {
		return nil, errors.New("proxy_route_id is required")
	}
	recordType := strings.ToUpper(strings.TrimSpace(input.RecordType))
	if recordType == "" {
		recordType = "A"
	}
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("record_type only supports A/AAAA")
	}
	sourceIP := strings.TrimSpace(input.SourceIP)
	if sourceIP != "" && net.ParseIP(sourceIP) == nil {
		return nil, errors.New("source_ip format is invalid")
	}
	country := strings.ToUpper(strings.TrimSpace(input.Country))
	if country != "" && !proxyRouteRegionCountryPattern.MatchString(country) {
		return nil, errors.New("country must be a two-letter country code")
	}

	routeModel, err := model.GetProxyRouteByID(input.ProxyRouteID)
	if err != nil {
		return nil, err
	}
	if routeModel == nil || !routeModel.Enabled {
		return nil, errors.New("selected proxy route is not enabled")
	}
	if routeModel.DNSProviderMode != DNSProviderModeAuthoritative {
		return nil, errors.New("selected proxy route is not using authoritative DNS")
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		return nil, err
	}
	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	var workerRoute *dnsworker.SnapshotRoute
	for index := range workerSnapshot.Routes {
		if workerSnapshot.Routes[index].ID == input.ProxyRouteID {
			workerRoute = &workerSnapshot.Routes[index]
			break
		}
	}
	if workerRoute == nil {
		return nil, errors.New("selected proxy route is not present in authoritative DNS snapshot")
	}

	qname := normalizeDNSRecordName(input.QName)
	if qname == "" {
		domains, err := decodeStoredDomains(routeModel.Domains, routeModel.Domain)
		if err != nil {
			return nil, err
		}
		if len(domains) > 0 {
			qname = normalizeDNSRecordName(domains[0])
		}
	}
	if qname == "" {
		return nil, errors.New("qname is required")
	}
	if !authoritativeRouteHasDomain(workerRoute, qname) {
		return nil, errors.New("qname does not belong to selected proxy route")
	}

	fresh := true
	if input.Fresh != nil {
		fresh = *input.Fresh
	}
	source := dnsworker.SourceContext{
		IP:      sourceIP,
		Country: country,
	}
	scheduler := dnsworker.NewScheduler()
	scheduler.LoadSnapshotStates(workerSnapshot)
	targets, ttl, sourceScope, err := scheduler.Select(workerSnapshot, workerRoute, recordType, source, fresh)
	if err != nil {
		if errors.Is(err, dnsworker.ErrDNSProbeThresholdNotSatisfied) {
			return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, nil, ttl, sourceScope, "Agent 探测未达到调度门槛，当前来源没有可用于 "+recordType+" 记录的边缘节点。请查看下方节点原因确认是未探测、探测过期还是 UDP/TCP 53 未同时可达。"), nil
		}
		if errors.Is(err, dnsworker.ErrNoAvailableTarget) || errors.Is(err, dnsworker.ErrNoTargetSelected) {
			return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, nil, ttl, sourceScope, "当前来源没有可用于 "+recordType+" 记录的边缘节点。请查看下方节点原因确认节点池、在线状态、OpenResty 健康、公网 IP 类型和负载阈值。"), nil
		}
		return nil, err
	}

	return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, targets, ttl, sourceScope, ""), nil
}

func buildDNSGSLBSimulationView(snapshot *AuthoritativeDNSSnapshot, workerRoute *dnsworker.SnapshotRoute, qname string, recordType string, sourceIP string, country string, targets []string, ttl int, sourceScope string, messagePrefix string) *DNSGSLBSimulationView {
	if targets == nil {
		targets = []string{}
	}
	policy := workerRoute.GSLBPolicy
	if !workerRoute.GSLBEnabled {
		policy.Strategy = workerRoute.ScheduleMode
		policy.TargetCount = workerRoute.TargetCount
		policy.Pools = []dnsworker.GSLBPoolPolicy{
			{
				Name:    normalizeNodePoolName(workerRoute.NodePool),
				Weight:  100,
				Enabled: true,
			},
		}
	}
	diagnostics := buildDNSGSLBSimulationDiagnostics(recordType, policy, GSLBSourceContext{IP: sourceIP, Country: country}, targets, snapshot.GSLBProbeSchedulingEnabled)
	message := "模拟结果来自当前面板生成的解析配置，不会写入真实切换状态。"
	if strings.TrimSpace(messagePrefix) != "" {
		message = strings.TrimSpace(messagePrefix) + " " + message
	}
	if sourceScope == defaultGSLBScopeKey && country == "" {
		message += " 未指定国家代码时使用全局作用域。"
	}
	return &DNSGSLBSimulationView{
		ProxyRouteID:    workerRoute.ID,
		SiteName:        workerRoute.SiteName,
		QName:           qname,
		RecordType:      recordType,
		Country:         country,
		SourceIP:        sourceIP,
		SourceScope:     sourceScope,
		TTL:             ttl,
		Targets:         targets,
		TargetCount:     len(targets),
		Strategy:        strings.TrimSpace(policy.Strategy),
		GSLBEnabled:     workerRoute.GSLBEnabled,
		SnapshotVersion: snapshot.SnapshotVersion,
		SnapshotAt:      snapshot.GeneratedAt,
		Message:         message,
		MatchedPools:    diagnostics.pools,
		Nodes:           diagnostics.nodes,
	}
}

func authoritativeRouteHasDomain(route *dnsworker.SnapshotRoute, qname string) bool {
	if route == nil {
		return false
	}
	name := normalizeDNSRecordName(qname)
	for _, domain := range route.Domains {
		if authoritativeDomainMatchesQName(normalizeDNSRecordName(domain), name) {
			return true
		}
	}
	return false
}

func authoritativeDomainMatchesQName(domain string, qname string) bool {
	domain = normalizeDNSRecordName(domain)
	qname = normalizeDNSRecordName(qname)
	if domain == "" || qname == "" {
		return false
	}
	if domain == qname {
		return true
	}
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	base := strings.TrimPrefix(domain, "*.")
	if base == "" || qname == base || !strings.HasSuffix(qname, "."+base) {
		return false
	}
	leftmostLabel := strings.TrimSuffix(qname, "."+base)
	return leftmostLabel != "" && !strings.Contains(leftmostLabel, ".")
}

type dnsGSLBSimulationDiagnostics struct {
	pools []DNSGSLBSimulationPoolView
	nodes []DNSGSLBSimulationNodeView
}

func buildDNSGSLBSimulationDiagnostics(recordType string, policy dnsworker.GSLBPolicy, source GSLBSourceContext, selectedTargets []string, requireHealthyDNSProbe bool) dnsGSLBSimulationDiagnostics {
	return buildDNSGSLBSimulationDiagnosticsWithOptions(recordType, policy, source, selectedTargets, gslbDNSSchedulingOptions{
		RequireHealthyDNSProbe: requireHealthyDNSProbe,
	})
}

func buildDNSGSLBSimulationDiagnosticsWithOptions(recordType string, policy dnsworker.GSLBPolicy, source GSLBSourceContext, selectedTargets []string, options gslbDNSSchedulingOptions) dnsGSLBSimulationDiagnostics {
	servicePolicy := convertWorkerGSLBPolicyToAuthoritative(policy)
	servicePolicy, err := normalizeGSLBPolicy(servicePolicy, "default", servicePolicy.TargetCount, servicePolicy.Strategy, servicePolicy.TTL)
	if err != nil {
		return dnsGSLBSimulationDiagnostics{}
	}
	matchedPools := matchGSLBPoolsForSource(servicePolicy.Pools, source)
	diagnostics := dnsGSLBSimulationDiagnostics{
		pools: buildDNSGSLBSimulationPoolViews(servicePolicy.Pools, matchedPools, source),
		nodes: []DNSGSLBSimulationNodeView{},
	}
	nodes, err := gslbDNSSchedulingNodes(options)
	if err != nil {
		return diagnostics
	}
	metrics := gslbDNSSchedulingMetricsByNode(options)
	nodeProbeStats := gslbDNSSchedulingProbeStatsByNode(options)
	selectedSet := make(map[string]struct{}, len(selectedTargets))
	for _, target := range selectedTargets {
		selectedSet[strings.TrimSpace(target)] = struct{}{}
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		view := buildDNSGSLBSimulationNodeView(node, recordType, servicePolicy, matchedPools, metrics[node.NodeID], selectedSet, nodeProbeStats[node.NodeID], options.RequireHealthyDNSProbe)
		diagnostics.nodes = append(diagnostics.nodes, view)
	}
	sort.SliceStable(diagnostics.nodes, func(i, j int) bool {
		left := diagnostics.nodes[i]
		right := diagnostics.nodes[j]
		if left.Selected != right.Selected {
			return left.Selected
		}
		if left.Eligible != right.Eligible {
			return left.Eligible
		}
		if left.PoolName != right.PoolName {
			return left.PoolName < right.PoolName
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.NodeID < right.NodeID
	})
	return diagnostics
}

func buildDNSGSLBSimulationPoolViews(pools []ProxyRouteGSLBPoolPolicy, matchedPools map[string]ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) []DNSGSLBSimulationPoolView {
	result := make([]DNSGSLBSimulationPoolView, 0, len(pools))
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	cidrMatched := false
	for _, pool := range pools {
		if _, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			cidrMatched = true
			break
		}
	}
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled {
			continue
		}
		_, matched := matchedPools[name]
		reason := "参与全局调度"
		if matchedCIDR, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			reason = "匹配来源网段 " + matchedCIDR
		} else if cidrMatched {
			reason = "未匹配来源网段"
		} else if country != "" {
			reason = "未匹配来源国家"
			for _, poolCountry := range pool.Countries {
				if country == strings.ToUpper(strings.TrimSpace(poolCountry)) {
					reason = "匹配来源国家 " + country
					break
				}
			}
			if len(matchedPools) == len(enabledGSLBPoolNames(pools)) && reason == "未匹配来源国家" {
				reason = "未命中国家专属池，回退参与调度"
			}
		}
		result = append(result, DNSGSLBSimulationPoolView{
			Name:        name,
			Weight:      pool.Weight,
			Countries:   append([]string(nil), pool.Countries...),
			SourceCIDRs: append([]string(nil), pool.SourceCIDRs...),
			Matched:     matched,
			Reason:      reason,
		})
	}
	return result
}

func enabledGSLBPoolNames(pools []ProxyRouteGSLBPoolPolicy) map[string]struct{} {
	result := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func buildDNSGSLBSimulationNodeView(node *model.Node, recordType string, policy ProxyRouteGSLBPolicy, matchedPools map[string]ProxyRouteGSLBPoolPolicy, metric *model.NodeMetricSnapshot, selectedSet map[string]struct{}, probeStats *dnsWorkerNodeProbeStats, requireHealthyDNSProbe bool) DNSGSLBSimulationNodeView {
	poolName := normalizeNodePoolName(node.PoolName)
	poolPolicy, poolMatched := matchedPools[poolName]
	reasons := []string{}
	if !poolMatched {
		reasons = append(reasons, "节点池未匹配当前来源")
	}
	if poolMatched && !gslbPoolAllowsNode(poolPolicy, node.NodeID) {
		reasons = append(reasons, "节点未被当前节点池子集选中")
	}
	if node.DrainMode {
		reasons = append(reasons, "节点处于排空模式")
	}
	if !isNodeSchedulableForDNS(node) {
		reasons = append(reasons, "节点已关闭自动调度")
	}
	if !isNodeOnlineAndOpenRestyHealthy(node) {
		reasons = append(reasons, "节点离线或 OpenResty 不健康")
	}
	publicIPs := resolveNodePublicIPs(node)
	candidateTargets, ipReasons := filterDNSGSLBSimulationTargets(publicIPs, recordType)
	reasons = append(reasons, ipReasons...)
	if requireHealthyDNSProbe && !dnsWorkerNodeProbeStatsSchedulable(probeStats) {
		reasons = append(reasons, dnsWorkerNodeProbeThresholdReason(probeStats))
	}
	hasMetric := metric != nil
	openrestyConnections := int64(0)
	cpuUsage := float64(0)
	memoryUsage := float64(0)
	var metricCapturedAt *time.Time
	if metric != nil {
		capturedAt := metric.CapturedAt
		metricCapturedAt = &capturedAt
		openrestyConnections = metric.OpenrestyConnections
		cpuUsage = metric.CPUUsagePercent
		memoryUsage = nodeMetricMemoryUsagePercent(metric)
		if !metricWithinGSLBThresholds(metric, policy.LoadThresholds) {
			reasons = append(reasons, "节点负载超过 GSLB 阈值")
		}
	}
	selected := []string{}
	for _, target := range candidateTargets {
		if _, ok := selectedSet[target]; ok {
			selected = append(selected, target)
		}
	}
	eligible := poolMatched &&
		gslbPoolAllowsNode(poolPolicy, node.NodeID) &&
		isNodeSchedulableForDNS(node) &&
		isNodeOnlineAndOpenRestyHealthy(node) &&
		(!requireHealthyDNSProbe || dnsWorkerNodeProbeStatsSchedulable(probeStats)) &&
		(!hasMetric || metricWithinGSLBThresholds(metric, policy.LoadThresholds)) &&
		len(candidateTargets) > 0
	if eligible {
		reasons = append(reasons, "可参与当前调度")
		if normalizeDNSScheduleMode(policy.Strategy) == "load_aware" && !hasMetric {
			reasons = append(reasons, "暂无新鲜负载指标，仅作为兜底候选")
		}
	}
	probeSummary := summarizeDNSWorkerNodeProbeStats(probeStats)
	score := float64(0)
	if poolMatched {
		candidate := gslbDNSTargetCandidate{
			NodeID:               node.NodeID,
			PoolName:             poolName,
			NodeWeight:           normalizeNodeWeight(node.Weight),
			PoolWeight:           poolPolicy.Weight,
			LastSeenAt:           node.LastSeenAt,
			OpenrestyConnections: openrestyConnections,
			CPUUsagePercent:      cpuUsage,
			MemoryUsagePercent:   memoryUsage,
			HasMetric:            hasMetric,
		}
		if requireHealthyDNSProbe && probeStats != nil {
			candidate.DNSProbeHealthy = dnsWorkerNodeProbeStatsSchedulable(probeStats)
			candidate.DNSProbeCheckedCount = probeStats.totalCount
			candidate.DNSProbeHealthyCount = probeStats.healthyCount
			candidate.DNSProbeStaleCount = probeStats.staleCount
			candidate.DNSProbeAverageRTTMs = averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples)
		}
		score = scoreGSLBCandidate(candidate, policy.Strategy)
	}
	lastSeenAt := node.LastSeenAt
	return DNSGSLBSimulationNodeView{
		NodeID:                  node.NodeID,
		Name:                    node.Name,
		PoolName:                poolName,
		Status:                  computeNodeStatus(node),
		OpenrestyStatus:         normalizeOpenrestyStatus(node.OpenrestyStatus),
		SchedulingEnabled:       isNodeSchedulableForDNS(node),
		DrainMode:               node.DrainMode,
		LastSeenAt:              &lastSeenAt,
		PublicIPs:               publicIPs,
		CandidateTargets:        candidateTargets,
		SelectedTargets:         selected,
		Eligible:                eligible,
		Selected:                len(selected) > 0,
		Reasons:                 dedupeStrings(reasons),
		HasMetric:               hasMetric,
		MetricCapturedAt:        metricCapturedAt,
		OpenrestyConnections:    openrestyConnections,
		CPUUsagePercent:         cpuUsage,
		MemoryUsagePercent:      memoryUsage,
		Score:                   score,
		NodeProbeStatus:         probeSummary.status,
		NodeProbeMessage:        probeSummary.message,
		NodeProbeCheckedCount:   probeSummary.checkedCount,
		NodeProbeHealthyCount:   probeSummary.healthyCount,
		NodeProbeStaleCount:     probeSummary.staleCount,
		NodeProbeHealthyPercent: probeSummary.healthyPercent,
		NodeProbeAverageRTTMs:   probeSummary.averageRTTMs,
		NodeProbeMaxRTTMs:       probeSummary.maxRTTMs,
	}
}

func filterDNSGSLBSimulationTargets(values []string, recordType string) ([]string, []string) {
	targets := []string{}
	reasons := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		ip := iputil.NormalizeIP(value)
		parsed := net.ParseIP(ip)
		if parsed == nil {
			reasons = append(reasons, "公网 IP 格式无效")
			continue
		}
		if !iputil.IsPublicString(ip) {
			reasons = append(reasons, "公网 IP 不是可路由公网地址")
			continue
		}
		if recordType == "A" && parsed.To4() == nil {
			reasons = append(reasons, "缺少 IPv4 公网 IP")
			continue
		}
		if recordType == "AAAA" && parsed.To4() != nil {
			reasons = append(reasons, "缺少 IPv6 公网 IP")
			continue
		}
		content := parsed.String()
		if _, ok := seen[content]; ok {
			continue
		}
		seen[content] = struct{}{}
		targets = append(targets, content)
	}
	if len(values) == 0 {
		reasons = append(reasons, "未配置节点公网 IP 池")
	} else if len(targets) == 0 {
		reasons = append(reasons, "没有符合记录类型的公网 IP")
	}
	return targets, dedupeStrings(reasons)
}

func convertWorkerGSLBPolicyToAuthoritative(policy dnsworker.GSLBPolicy) ProxyRouteGSLBPolicy {
	result := ProxyRouteGSLBPolicy{
		Mode:        policy.Mode,
		Strategy:    policy.Strategy,
		TargetCount: policy.TargetCount,
		TTL:         policy.TTL,
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: policy.SourceIP.Provider,
			APIURL:   policy.SourceIP.APIURL,
			APIToken: policy.SourceIP.APIToken,
		},
		LoadThresholds: ProxyRouteGSLBLoadThresholds{
			MaxOpenrestyConnections: policy.LoadThresholds.MaxOpenrestyConnections,
			MaxCPUPercent:           policy.LoadThresholds.MaxCPUPercent,
			MaxMemoryPercent:        policy.LoadThresholds.MaxMemoryPercent,
		},
		Debounce: ProxyRouteGSLBDebounce{
			CooldownSeconds:    policy.Debounce.CooldownSeconds,
			UnhealthyThreshold: policy.Debounce.UnhealthyThreshold,
			RecoveryThreshold:  policy.Debounce.RecoveryThreshold,
		},
		Pools: make([]ProxyRouteGSLBPoolPolicy, 0, len(policy.Pools)),
	}
	for _, pool := range policy.Pools {
		result.Pools = append(result.Pools, ProxyRouteGSLBPoolPolicy{
			Name:        pool.Name,
			Weight:      pool.Weight,
			Countries:   append([]string(nil), pool.Countries...),
			SourceCIDRs: append([]string(nil), pool.SourceCIDRs...),
			NodeIDs:     append([]string(nil), pool.NodeIDs...),
			Enabled:     pool.Enabled,
		})
	}
	return result
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
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

func validateAuthoritativeDNSRecordDynamicConflicts(record *model.DNSRecord) error {
	if record == nil || !record.Enabled {
		return nil
	}
	recordType := strings.ToUpper(strings.TrimSpace(record.Type))
	if recordType != "A" && recordType != "AAAA" && recordType != "CNAME" {
		return nil
	}
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref = ?", true, DNSProviderModeAuthoritative, record.ZoneID).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil {
			continue
		}
		routeType := normalizeDNSRecordType(route.DNSRecordType)
		if recordType != "CNAME" && routeType != recordType {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return fmt.Errorf("existing authoritative route %d domains are invalid: %w", route.ID, err)
		}
		for _, domain := range domains {
			if authoritativeDomainMatchesQName(domain, record.Name) {
				return fmt.Errorf("静态 DNS 记录 %s %s 与网站配置「%s」的本地自建解析自动 %s 记录冲突。请到左侧「网站配置」打开该站点的「负载均衡」检查自动解析，或禁用该网站自动解析后再添加静态记录", record.Name, recordType, route.SiteName, routeType)
			}
		}
	}
	return nil
}

func validateAuthoritativeProxyRouteStaticRecordConflicts(zoneID uint, domains []string, recordType string, enabled bool) error {
	if !enabled || zoneID == 0 {
		return nil
	}
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return errors.New("本地自建解析自动选 IP 只支持 A/AAAA")
	}
	normalizedDomains := make([]string, 0, len(domains))
	wildcardSuffixes := make([]string, 0, len(domains))
	for _, domain := range domains {
		normalized := normalizeDNSRecordName(domain)
		if normalized == "" {
			continue
		}
		normalizedDomains = append(normalizedDomains, normalized)
		if strings.HasPrefix(normalized, "*.") {
			suffix := strings.TrimPrefix(normalized, "*.")
			if suffix != "" {
				wildcardSuffixes = append(wildcardSuffixes, suffix)
			}
		}
	}
	if len(normalizedDomains) == 0 && len(wildcardSuffixes) == 0 {
		return nil
	}
	records, err := model.ListDNSRecordsByZoneIDNameCandidates(zoneID, normalizedDomains, wildcardSuffixes)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record == nil || !record.Enabled {
			continue
		}
		if !authoritativeDomainsMatchRecordName(normalizedDomains, record.Name) {
			continue
		}
		staticType := strings.ToUpper(strings.TrimSpace(record.Type))
		if staticType != "CNAME" && staticType != recordType {
			continue
		}
		return fmt.Errorf("本地自建解析网站的自动 %s 记录与托管域名「%s」里的静态记录「%s %s」冲突。位置：左侧「本地自建解析」-> 托管域名「%s」-> 静态记录。请删除或禁用该静态记录后再创建网站配置；如果希望网站配置自动接管解析，不要保留同名 A/AAAA/CNAME 静态记录", recordType, authoritativeDNSZoneDisplayName(zoneID), record.Name, staticType, authoritativeDNSZoneDisplayName(zoneID))
	}
	return nil
}

func authoritativeDNSZoneDisplayName(zoneID uint) string {
	zone, err := model.GetDNSZoneByID(zoneID)
	if err == nil && zone != nil && strings.TrimSpace(zone.Name) != "" {
		return strings.TrimSpace(zone.Name)
	}
	if zoneID > 0 {
		return fmt.Sprintf("ID %d", zoneID)
	}
	return "未选择托管域名"
}

func authoritativeDomainsMatchRecordName(domains []string, recordName string) bool {
	for _, domain := range domains {
		if authoritativeDomainMatchesQName(domain, recordName) {
			return true
		}
	}
	return false
}

type authoritativeDNSMigrationWorkerStatsView struct {
	total                       int
	online                      int
	publicReachable             int
	freshSnapshot               int
	ready                       int
	publicReachableWithoutFresh int
	readySnapshotVersionCount   int
}

type authoritativeDNSWorkerReadiness struct {
	online          bool
	publicReachable bool
	freshSnapshot   bool
	ready           bool
}

type authoritativeDNSTargetPrecheckView struct {
	targets     []string
	targetCount int
	recordType  string
	strategy    string
	warnings    []string
}

type authoritativeDNSTargetPrecheckSource struct {
	label  string
	key    string
	source GSLBSourceContext
}

func authoritativeDNSMigrationWorkerStats() (authoritativeDNSMigrationWorkerStatsView, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return authoritativeDNSMigrationWorkerStatsView{}, err
	}
	stats := authoritativeDNSMigrationWorkerStatsView{total: len(workers)}
	now := time.Now().UTC()
	snapshotMaxAge := authoritativeDNSSnapshotMaxAge()
	readySnapshotVersions := map[string]int{}
	for _, worker := range workers {
		readiness := evaluateAuthoritativeDNSWorkerReadiness(now, snapshotMaxAge, worker)
		if readiness.online {
			stats.online++
		}
		if readiness.publicReachable {
			stats.publicReachable++
		}
		if readiness.freshSnapshot {
			stats.freshSnapshot++
		}
		if readiness.ready {
			stats.ready++
			readySnapshotVersions[strings.TrimSpace(worker.LastSnapshotVersion)]++
		}
		if readiness.publicReachable && !readiness.freshSnapshot {
			stats.publicReachableWithoutFresh++
		}
	}
	stats.readySnapshotVersionCount = len(readySnapshotVersions)
	return stats, nil
}

func buildAuthoritativeDNSMigrationCandidate(route *model.ProxyRoute, zones []*model.DNSZone, workerStats authoritativeDNSMigrationWorkerStatsView, schedulingOptions gslbDNSSchedulingOptions) (AuthoritativeDNSMigrationCandidateView, error) {
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return AuthoritativeDNSMigrationCandidateView{}, err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	candidate := AuthoritativeDNSMigrationCandidateView{
		ProxyRouteID:               route.ID,
		SiteName:                   normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain),
		PrimaryDomain:              route.Domain,
		Domains:                    domains,
		Enabled:                    route.Enabled,
		DNSAutoSync:                route.DNSAutoSync,
		DNSProviderMode:            normalizeDNSProviderMode(route.DNSProviderMode),
		DNSRecordType:              recordType,
		GSLBEnabled:                route.GSLBEnabled,
		TotalWorkerCount:           workerStats.total,
		OnlineWorkerCount:          workerStats.online,
		PublicReachableWorkerCount: workerStats.publicReachable,
		FreshSnapshotWorkerCount:   workerStats.freshSnapshot,
		ReadyWorkerCount:           workerStats.ready,
		Blockers:                   []string{},
		Warnings:                   []string{},
	}
	zone := bestAuthoritativeZoneForDomains(zones, domains)
	if zone != nil {
		zoneID := zone.ID
		candidate.MatchingZoneID = &zoneID
		candidate.MatchingZoneName = zone.Name
		candidate.MatchingZoneEnabled = zone.Enabled
	}
	if len(domains) == 0 {
		candidate.Blockers = append(candidate.Blockers, "网站未配置域名")
	}
	if zone == nil {
		candidate.Blockers = append(candidate.Blockers, "没有匹配的托管域名")
	} else if !zone.Enabled {
		candidate.Blockers = append(candidate.Blockers, "匹配的托管域名已停用")
	} else if err := validateAuthoritativeProxyRouteStaticRecordConflicts(zone.ID, domains, recordType, route.Enabled); err != nil {
		candidate.Blockers = append(candidate.Blockers, err.Error())
	}
	targetPrecheck, targetErr := precheckAuthoritativeRouteDNSTargetsWithOptions(route, recordType, schedulingOptions)
	if targetErr != nil {
		candidate.Blockers = append(candidate.Blockers, targetErr.Error())
	}
	candidate.Warnings = append(candidate.Warnings, targetPrecheck.warnings...)
	if workerStats.online == 0 {
		candidate.Blockers = append(candidate.Blockers, "没有在线 DNS 响应端")
	} else if workerStats.publicReachable == 0 {
		candidate.Blockers = append(candidate.Blockers, "在线 DNS 响应端尚未通过公网 UDP/TCP 53 探测")
	} else if workerStats.ready == 0 {
		candidate.Blockers = append(candidate.Blockers, "公网可达 DNS 响应端尚未拉取未过期的解析配置")
	} else if workerStats.publicReachableWithoutFresh > 0 {
		candidate.Blockers = append(candidate.Blockers, "部分公网可达 DNS 响应端尚未拉取未过期的解析配置")
	} else if workerStats.readySnapshotVersionCount > 1 {
		candidate.Blockers = append(candidate.Blockers, "公网可达 DNS 响应端的解析配置版本不一致")
	}
	if !route.GSLBEnabled {
		candidate.Warnings = append(candidate.Warnings, "未启用 GSLB，多节点池实时分流不会生效")
	}
	if workerStats.total < 2 {
		candidate.Warnings = append(candidate.Warnings, "生产环境建议至少部署 2 个 DNS 响应端")
	}
	if workerStats.online > workerStats.publicReachable {
		candidate.Warnings = append(candidate.Warnings, "部分在线响应端未通过最新公网探测，迁移前建议逐个点击「探测」")
	}
	if workerStats.publicReachableWithoutFresh == 0 && workerStats.online > workerStats.freshSnapshot {
		candidate.Warnings = append(candidate.Warnings, "部分在线响应端尚未拉取未过期解析配置，请确认配置同步正常")
	}
	if !route.DNSAutoSync {
		candidate.Warnings = append(candidate.Warnings, "当前未启用 Cloudflare 自动 DNS，请确认是否仍需要迁移")
	}
	candidate.Ready = len(candidate.Blockers) == 0
	return candidate, nil
}

func bestAuthoritativeZoneForDomains(zones []*model.DNSZone, domains []string) *model.DNSZone {
	if len(domains) == 0 {
		return nil
	}
	var best *model.DNSZone
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		matched := true
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if best == nil || len(normalizeDNSRecordName(zone.Name)) > len(normalizeDNSRecordName(best.Name)) {
			best = zone
		}
	}
	return best
}

func precheckAuthoritativeRouteDNSTargets(route *model.ProxyRoute, recordType string) (authoritativeDNSTargetPrecheckView, error) {
	return precheckAuthoritativeRouteDNSTargetsWithOptions(route, recordType, authoritativeDNSSchedulingOptions())
}

func precheckAuthoritativeRouteDNSTargetsWithOptions(route *model.ProxyRoute, recordType string, schedulingOptions gslbDNSSchedulingOptions) (authoritativeDNSTargetPrecheckView, error) {
	view := authoritativeDNSTargetPrecheckView{
		targetCount: 1,
		recordType:  normalizeDNSRecordType(recordType),
		strategy:    "healthy",
		warnings:    []string{},
	}
	if route == nil {
		return view, errors.New("网站配置不存在")
	}
	recordType = normalizeDNSRecordType(recordType)
	view.recordType = recordType
	if recordType != "A" && recordType != "AAAA" {
		return view, errors.New("本地自建解析自动选 IP 只支持 A/AAAA")
	}
	view.targetCount = normalizeDNSTargetCount(route.DNSTargetCount)
	view.strategy = normalizeDNSScheduleMode(route.DNSScheduleMode)
	policy := defaultGSLBPolicy(route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
	if route.GSLBEnabled {
		decodedPolicy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return view, err
		}
		policy, err = normalizeGSLBPolicy(decodedPolicy, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
		if err != nil {
			return view, err
		}
		view.targetCount = normalizeDNSTargetCount(policy.TargetCount)
		view.strategy = normalizeDNSScheduleMode(policy.Strategy)
	}
	selection, err := selectProxyRouteDNSTargetsWithOptions(route, recordType, schedulingOptions)
	if err != nil {
		return view, formatAuthoritativeDNSTargetPrecheckError("当前节点池/GSLB", recordType, err, policy, GSLBSourceContext{}, schedulingOptions, true)
	}
	targets, err := normalizeDNSRecordContents(recordType, selection.Targets)
	if err != nil {
		return view, fmt.Errorf("当前节点池/GSLB 返回的 %s 边缘 IP 无效：%w", recordType, err)
	}
	if len(targets) == 0 {
		return view, fmt.Errorf("当前节点池/GSLB 没有可用于 %s 记录的边缘 IP", recordType)
	}
	view.targets = targets
	if view.targetCount > len(targets) {
		view.warnings = append(view.warnings, fmt.Sprintf("全局当前只能返回 %d / %d 个 %s 边缘 IP，请检查节点池容量", len(targets), view.targetCount, view.recordType))
	}
	if route.GSLBEnabled {
		blockers := []string{}
		for _, source := range authoritativeDNSTargetPrecheckSources(policy) {
			selection, err := selectGSLBDNSTargetsWithOptions(route, recordType, source.source, schedulingOptions)
			if err != nil {
				blockers = append(blockers, formatAuthoritativeDNSTargetPrecheckError(source.label, recordType, err, policy, source.source, schedulingOptions, false).Error())
				continue
			}
			targets, err := normalizeDNSRecordContents(recordType, selection.Targets)
			if err != nil {
				blockers = append(blockers, fmt.Sprintf("%s 返回的 %s 边缘 IP 无效：%v", source.label, recordType, err))
				continue
			}
			if len(targets) == 0 {
				blockers = append(blockers, fmt.Sprintf("%s 没有可用于 %s 记录的边缘 IP", source.label, recordType))
				continue
			}
			if view.targetCount > len(targets) {
				view.warnings = append(view.warnings, fmt.Sprintf("%s 当前只能返回 %d / %d 个 %s 边缘 IP，请检查匹配节点池容量", source.label, len(targets), view.targetCount, recordType))
			}
		}
		if len(blockers) > 0 {
			return view, errors.New(strings.Join(blockers, "；"))
		}
	}
	return view, nil
}

func formatAuthoritativeDNSTargetPrecheckError(label string, recordType string, err error, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions, includeChecklist bool) error {
	message := fmt.Sprintf("%s 无法返回 %s 边缘 IP", label, recordType)
	if includeChecklist {
		message += "，请检查节点池、公网 IP、节点在线状态、OpenResty 健康、GSLB 负载阈值和 Agent 探测调度门槛"
	}
	if detail := summarizeAuthoritativeDNSTargetPrecheckDiagnostics(recordType, policy, source, options); detail != "" {
		message += "；诊断：" + detail
	}
	return fmt.Errorf("%s：%w", message, err)
}

func summarizeAuthoritativeDNSTargetPrecheckDiagnostics(recordType string, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions) string {
	diagnostics := buildDNSGSLBSimulationDiagnosticsWithOptions(recordType, convertAuthoritativeGSLBPolicyToWorker(policy), source, nil, options)
	matchedPoolLabels := []string{}
	matchedPoolNames := []string{}
	matchedPools := map[string]struct{}{}
	for _, pool := range diagnostics.pools {
		if !pool.Matched {
			continue
		}
		matchedPools[pool.Name] = struct{}{}
		matchedPoolNames = append(matchedPoolNames, pool.Name)
		label := pool.Name
		if pool.Reason != "" && pool.Reason != "参与全局调度" {
			label += "（" + pool.Reason + "）"
		}
		matchedPoolLabels = append(matchedPoolLabels, label)
	}
	nodeDetails := summarizeAuthoritativeDNSTargetPrecheckNodes(diagnostics.nodes, matchedPools)
	if len(nodeDetails) == 0 {
		poolLabels := matchedPoolLabels
		if len(poolLabels) == 0 {
			poolLabels = matchedPoolNames
		}
		if len(poolLabels) > 0 {
			return "匹配节点池 " + strings.Join(poolLabels, "、") + " 没有节点"
		}
		return "没有启用的 GSLB 节点池"
	}
	parts := []string{}
	if len(matchedPoolLabels) > 0 {
		parts = append(parts, "匹配节点池 "+strings.Join(matchedPoolLabels, "、"))
	}
	parts = append(parts, nodeDetails...)
	return strings.Join(parts, "；")
}

func summarizeAuthoritativeDNSTargetPrecheckNodes(nodes []DNSGSLBSimulationNodeView, matchedPools map[string]struct{}) []string {
	details := []string{}
	omitted := 0
	for _, node := range nodes {
		if len(matchedPools) > 0 {
			if _, ok := matchedPools[node.PoolName]; !ok {
				continue
			}
		}
		reasons := compactAuthoritativeDNSTargetPrecheckNodeReasons(node)
		if len(reasons) == 0 {
			continue
		}
		if len(details) >= 3 {
			omitted++
			continue
		}
		details = append(details, fmt.Sprintf("节点 %s：%s", authoritativeDNSTargetPrecheckNodeLabel(node), strings.Join(reasons, "、")))
	}
	if omitted > 0 {
		details = append(details, fmt.Sprintf("另有 %d 个节点被排除", omitted))
	}
	return details
}

func compactAuthoritativeDNSTargetPrecheckNodeReasons(node DNSGSLBSimulationNodeView) []string {
	reasons := []string{}
	for _, reason := range node.Reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" || reason == "节点池未匹配当前来源" || reason == "可参与当前调度" {
			continue
		}
		reasons = append(reasons, reason)
		if len(reasons) >= 2 {
			break
		}
	}
	if len(reasons) == 0 && !node.Eligible {
		reasons = append(reasons, "未满足当前调度条件")
	}
	return dedupeStrings(reasons)
}

func authoritativeDNSTargetPrecheckNodeLabel(node DNSGSLBSimulationNodeView) string {
	nodeID := strings.TrimSpace(node.NodeID)
	name := strings.TrimSpace(node.Name)
	if nodeID == "" {
		return name
	}
	if name != "" && name != nodeID {
		return nodeID + "/" + name
	}
	return nodeID
}

func authoritativeDNSSchedulingOptions() gslbDNSSchedulingOptions {
	return gslbDNSSchedulingOptions{
		RequireHealthyDNSProbe: common.GSLBProbeSchedulingEnabled,
	}
}

func authoritativeDNSTargetPrecheckSources(policy ProxyRouteGSLBPolicy) []authoritativeDNSTargetPrecheckSource {
	sources := make([]authoritativeDNSTargetPrecheckSource, 0)
	seen := map[string]struct{}{}
	appendSource := func(source authoritativeDNSTargetPrecheckSource) {
		if strings.TrimSpace(source.key) == "" {
			return
		}
		if _, ok := seen[source.key]; ok {
			return
		}
		seen[source.key] = struct{}{}
		sources = append(sources, source)
	}
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		for _, value := range pool.SourceCIDRs {
			cidr, ok := normalizeGSLBCIDR(value)
			if !ok {
				continue
			}
			sourceIP := sampleIPForGSLBCIDR(cidr)
			if sourceIP == "" {
				continue
			}
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源网段 " + cidr,
				key:    "cidr:" + cidr,
				source: GSLBSourceContext{IP: sourceIP},
			})
		}
		for _, value := range pool.Countries {
			country := strings.ToUpper(strings.TrimSpace(value))
			if country == "" || !proxyRouteRegionCountryPattern.MatchString(country) {
				continue
			}
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源国家 " + country,
				key:    "country:" + country,
				source: GSLBSourceContext{Country: country},
			})
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].key < sources[j].key
	})
	return sources
}

func sampleIPForGSLBCIDR(raw string) string {
	cidr, ok := normalizeGSLBCIDR(raw)
	if !ok {
		return ""
	}
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil || network == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	network.IP = ip.Mask(network.Mask)
	if ipv4 := network.IP.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return network.IP.String()
}

func buildDNSZoneView(zone *model.DNSZone, includeRecords bool) (*DNSZoneView, error) {
	if zone == nil {
		return nil, errors.New("DNS zone is nil")
	}
	var recordCount int64
	if err := model.DB.Model(&model.DNSRecord{}).Where("zone_id = ?", zone.ID).Count(&recordCount).Error; err != nil {
		return nil, err
	}
	return buildDNSZoneViewWithRecordCount(zone, includeRecords, recordCount)
}

func buildDNSZoneViewWithRecordCount(zone *model.DNSZone, includeRecords bool, recordCount int64) (*DNSZoneView, error) {
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
		RecordCount: recordCount,
		CreatedAt:   zone.CreatedAt,
		UpdatedAt:   zone.UpdatedAt,
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
	now := time.Now().UTC()
	probeResults := decodeDNSWorkerProbeResults(worker.LastProbeResult)
	probeAt := normalizeDNSWorkerProbeAt(worker.LastProbeAt, now, worker.UpdatedAt, worker.CreatedAt)
	probeState := evaluateDNSWorkerProbeState(now, probeAt, probeResults)
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
		GeoIPEnabled:        worker.GeoIPEnabled,
		GeoIPDatabasePath:   worker.GeoIPDatabasePath,
		GeoIPLastError:      worker.GeoIPLastError,
		LastProbeAt:         probeAt,
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

func resolveAuthoritativeMigrationZone(zoneIDRef *uint, domains []string) (*model.DNSZone, error) {
	if len(domains) == 0 {
		return nil, errors.New("proxy route has no domains")
	}
	if zoneIDRef != nil && *zoneIDRef > 0 {
		zone, err := model.GetDNSZoneByID(*zoneIDRef)
		if err != nil {
			return nil, errors.New("selected DNS zone does not exist")
		}
		if !zone.Enabled {
			return nil, errors.New("selected DNS zone is disabled")
		}
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				return nil, fmt.Errorf("domain %s is not under DNS zone %s", domain, zone.Name)
			}
		}
		return zone, nil
	}

	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	var best *model.DNSZone
	for _, zone := range zones {
		if zone == nil || !zone.Enabled {
			continue
		}
		matched := true
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if best == nil || len(normalizeDNSRecordName(zone.Name)) > len(normalizeDNSRecordName(best.Name)) {
			best = zone
		}
	}
	if best == nil {
		return nil, errors.New("no enabled DNS zone covers all route domains")
	}
	return best, nil
}

func validateAuthoritativeDNSReadyWorkers() error {
	stats, err := authoritativeDNSMigrationWorkerStats()
	if err != nil {
		return err
	}
	if stats.online == 0 {
		return errors.New("没有在线 DNS 响应端")
	}
	if stats.publicReachable == 0 {
		return errors.New("在线 DNS 响应端尚未通过公网 UDP/TCP 53 探测")
	}
	if stats.ready == 0 {
		return errors.New("公网可达 DNS 响应端尚未拉取未过期的解析配置")
	}
	if stats.publicReachableWithoutFresh > 0 {
		return errors.New("部分公网可达 DNS 响应端尚未拉取未过期的解析配置")
	}
	if stats.readySnapshotVersionCount > 1 {
		return errors.New("公网可达 DNS 响应端的解析配置版本不一致")
	}
	return nil
}

func evaluateAuthoritativeDNSWorkerReadiness(now time.Time, snapshotMaxAge time.Duration, worker *model.DNSWorker) authoritativeDNSWorkerReadiness {
	if worker == nil || normalizeDNSWorkerStatus(worker.Status) != dnsWorkerStatusOnline {
		return authoritativeDNSWorkerReadiness{}
	}
	readiness := authoritativeDNSWorkerReadiness{online: true}
	probeState := evaluateDNSWorkerProbeState(now, normalizeDNSWorkerProbeAt(worker.LastProbeAt, now, worker.UpdatedAt, worker.CreatedAt), decodeDNSWorkerProbeResults(worker.LastProbeResult))
	readiness.publicReachable = probeState.healthy
	readiness.freshSnapshot = hasFreshAuthoritativeDNSWorkerSnapshot(now, snapshotMaxAge, worker)
	readiness.ready = readiness.publicReachable && readiness.freshSnapshot
	return readiness
}

func hasFreshAuthoritativeDNSWorkerSnapshot(now time.Time, snapshotMaxAge time.Duration, worker *model.DNSWorker) bool {
	if worker == nil || worker.LastSnapshotAt == nil || strings.TrimSpace(worker.LastSnapshotVersion) == "" {
		return false
	}
	if snapshotMaxAge <= 0 {
		snapshotMaxAge = defaultDNSSnapshotMaxAge
	}
	snapshotAt := normalizeDNSWorkerSnapshotAt(worker.LastSnapshotAt, now, worker.UpdatedAt, worker.CreatedAt)
	if snapshotAt == nil {
		return false
	}
	return now.Sub(snapshotAt.UTC()) <= snapshotMaxAge
}

func buildDNSGSLBSchedulingStateView(state *model.GSLBSchedulingState, route *model.ProxyRoute) DNSGSLBSchedulingStateView {
	if state == nil {
		return DNSGSLBSchedulingStateView{}
	}
	recordType := normalizeDNSRecordType(state.DNSRecordType)
	view := DNSGSLBSchedulingStateView{
		ID:              state.ID,
		ProxyRouteID:    state.ProxyRouteID,
		RecordType:      recordType,
		ScopeKey:        normalizeDNSSourceScope(state.ScopeKey),
		SelectedTargets: decodeGSLBTargetList(state.SelectedTargets),
		DesiredTargets:  decodeGSLBTargetList(state.DesiredTargets),
		LastReason:      state.LastReason,
		LastChangedAt:   state.LastChangedAt,
		LastEvaluatedAt: state.LastEvaluatedAt,
		CreatedAt:       state.CreatedAt,
		UpdatedAt:       state.UpdatedAt,
	}
	if route == nil {
		view.Status = "orphaned"
		return view
	}
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		domains = normalizeStringList([]string{route.Domain})
	}
	view.Domains = domains
	if len(domains) > 0 {
		view.PrimaryDomain = domains[0]
	} else {
		view.PrimaryDomain = normalizeDNSRecordName(route.Domain)
	}
	view.SiteName = normalizeProxyRouteSiteNameInput(route, route.SiteName, view.PrimaryDomain)
	view.RouteEnabled = route.Enabled
	view.RouteAuthoritative = route.DNSProviderMode == DNSProviderModeAuthoritative
	view.RouteGSLBEnabled = route.GSLBEnabled
	view.RouteRecordType = normalizeDNSRecordType(route.DNSRecordType)
	view.Status = evaluateDNSGSLBSchedulingStateStatus(view)
	return view
}

func evaluateDNSGSLBSchedulingStateStatus(view DNSGSLBSchedulingStateView) string {
	if !view.RouteEnabled || !view.RouteGSLBEnabled {
		return "inactive"
	}
	if view.RouteRecordType != "" && view.RouteRecordType != view.RecordType {
		return "stale"
	}
	if len(view.SelectedTargets) == 0 {
		return "empty"
	}
	if len(view.DesiredTargets) > 0 && !sameStringSet(view.SelectedTargets, view.DesiredTargets) {
		return "debouncing"
	}
	return "active"
}

func snapshotDNSZones() ([]AuthoritativeDNSSnapshotZone, error) {
	return snapshotDNSZonesWithQueries(defaultAuthoritativeDNSSnapshotZoneQueries)
}

type authoritativeDNSSnapshotZoneQueries struct {
	ListDNSRecordsByZoneIDs func([]uint) ([]*model.DNSRecord, error)
}

var defaultAuthoritativeDNSSnapshotZoneQueries = authoritativeDNSSnapshotZoneQueries{
	ListDNSRecordsByZoneIDs: model.ListDNSRecordsByZoneIDs,
}

func snapshotDNSZonesWithQueries(queries authoritativeDNSSnapshotZoneQueries) ([]AuthoritativeDNSSnapshotZone, error) {
	var zones []*model.DNSZone
	if err := model.DB.Where("enabled = ?", true).Order("name asc").Find(&zones).Error; err != nil {
		return nil, err
	}
	zoneIDs := make([]uint, 0, len(zones))
	for _, zone := range zones {
		if zone != nil && zone.ID != 0 {
			zoneIDs = append(zoneIDs, zone.ID)
		}
	}
	listRecordsByZoneIDs := queries.ListDNSRecordsByZoneIDs
	if listRecordsByZoneIDs == nil {
		listRecordsByZoneIDs = model.ListDNSRecordsByZoneIDs
	}
	records, err := listRecordsByZoneIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	recordsByZoneID := make(map[uint][]*model.DNSRecord, len(zones))
	for _, record := range records {
		if record == nil {
			continue
		}
		recordsByZoneID[record.ZoneID] = append(recordsByZoneID[record.ZoneID], record)
	}
	result := make([]AuthoritativeDNSSnapshotZone, 0, len(zones))
	for _, zone := range zones {
		records := recordsByZoneID[zone.ID]
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
	return snapshotAuthoritativeRoutesWithOptions(authoritativeDNSSchedulingOptions())
}

func snapshotAuthoritativeRoutesWithOptions(schedulingOptions gslbDNSSchedulingOptions) ([]AuthoritativeDNSSnapshotRoute, error) {
	return snapshotAuthoritativeRoutesWithQueries(schedulingOptions, defaultAuthoritativeDNSSnapshotRouteQueries)
}

type authoritativeDNSSnapshotRouteQueries struct {
	ListDNSZonesByIDs func([]uint) ([]*model.DNSZone, error)
}

var defaultAuthoritativeDNSSnapshotRouteQueries = authoritativeDNSSnapshotRouteQueries{
	ListDNSZonesByIDs: model.ListDNSZonesByIDs,
}

func snapshotAuthoritativeRoutesWithQueries(schedulingOptions gslbDNSSchedulingOptions, queries authoritativeDNSSnapshotRouteQueries) ([]AuthoritativeDNSSnapshotRoute, error) {
	var routes []*model.ProxyRoute
	if err := model.DB.
		Where("enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", true, DNSProviderModeAuthoritative).
		Order("site_name asc").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	zoneIDs := make([]uint, 0, len(routes))
	seenZoneIDs := make(map[uint]struct{}, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zoneID := *route.DNSZoneIDRef
		if _, ok := seenZoneIDs[zoneID]; ok {
			continue
		}
		seenZoneIDs[zoneID] = struct{}{}
		zoneIDs = append(zoneIDs, zoneID)
	}
	listZonesByIDs := queries.ListDNSZonesByIDs
	if listZonesByIDs == nil {
		listZonesByIDs = model.ListDNSZonesByIDs
	}
	zones, err := listZonesByIDs(zoneIDs)
	if err != nil {
		return nil, err
	}
	zonesByID := make(map[uint]*model.DNSZone, len(zones))
	for _, zone := range zones {
		if zone == nil || zone.ID == 0 {
			continue
		}
		zonesByID[zone.ID] = zone
	}
	result := make([]AuthoritativeDNSSnapshotRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil || route.DNSZoneIDRef == nil || *route.DNSZoneIDRef == 0 {
			continue
		}
		zone := zonesByID[*route.DNSZoneIDRef]
		if zone == nil || !zone.Enabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		policy := defaultGSLBPolicy(route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
		if route.GSLBEnabled {
			policy, err = decodeStoredGSLBPolicy(route.GSLBPolicy)
			if err != nil {
				return nil, err
			}
		}
		ddosActive := routeDDOSProtectionActive(route) &&
			normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCustom
		effectiveGSLBEnabled := route.GSLBEnabled && !ddosActive
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
			GSLBEnabled:  effectiveGSLBEnabled,
			GSLBPolicy:   policy,
			DDOSActive:   ddosActive,
			DDOSProvider: normalizeDDOSProtectionProvider(route.DDOSProtectionProvider),
			DDOSTarget:   normalizeNodePoolName(route.DDOSProtectionTarget),
		}
		selectRoute := route
		if ddosActive {
			selectRouteCopy := *route
			selectRouteCopy.GSLBEnabled = false
			selectRouteCopy.NodePool = normalizeNodePoolName(route.DDOSProtectionTarget)
			selectRoute = &selectRouteCopy
		}
		selection, selectErr := selectProxyRouteDNSTargetsWithOptions(selectRoute, recordType, schedulingOptions)
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
	data := &gslbDNSSchedulingData{
		Nodes:         nodes,
		MetricsByNode: latestNodeMetricSnapshots(),
	}
	if common.GSLBProbeSchedulingEnabled {
		data.ProbeStatsByNode = buildDNSWorkerNodeProbeStatsByNodeForNodes(time.Now().UTC(), nodes)
	}
	return snapshotNodesWithData(data), nil
}

func snapshotNodesWithData(data *gslbDNSSchedulingData) []AuthoritativeDNSSnapshotNode {
	if data == nil {
		data = &gslbDNSSchedulingData{}
	}
	nodes := data.Nodes
	metrics := data.MetricsByNode
	probeStatsByNode := map[string]*dnsWorkerNodeProbeStats{}
	if data.ProbeStatsByNode != nil {
		probeStatsByNode = data.ProbeStatsByNode
	}
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
		if probeStats := probeStatsByNode[node.NodeID]; probeStats != nil {
			item.DNSProbeHealthy = dnsWorkerNodeProbeStatsSchedulable(probeStats)
			item.DNSProbeCheckedCount = probeStats.totalCount
			item.DNSProbeHealthyCount = probeStats.healthyCount
			item.DNSProbeStaleCount = probeStats.staleCount
			item.DNSProbeAverageRTTMs = averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples)
			item.DNSProbeMaxRTTMs = probeStats.maxRTTMs
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].PoolName != result[j].PoolName {
			return result[i].PoolName < result[j].PoolName
		}
		return result[i].NodeID < result[j].NodeID
	})
	return result
}

func snapshotGSLBSchedulingStates(routes []AuthoritativeDNSSnapshotRoute) ([]AuthoritativeDNSSnapshotSchedulingState, error) {
	routeIDs := make([]uint, 0, len(routes))
	routeRecordTypes := make(map[uint]string, len(routes))
	for _, route := range routes {
		if route.ID == 0 {
			continue
		}
		recordType := normalizeDNSRecordType(route.RecordType)
		if recordType != "A" && recordType != "AAAA" {
			continue
		}
		routeIDs = append(routeIDs, route.ID)
		routeRecordTypes[route.ID] = recordType
	}
	if len(routeIDs) == 0 {
		return []AuthoritativeDNSSnapshotSchedulingState{}, nil
	}
	var states []*model.GSLBSchedulingState
	if err := model.DB.
		Where("proxy_route_id IN ?", routeIDs).
		Order("proxy_route_id asc, dns_record_type asc, scope_key asc").
		Find(&states).Error; err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := make([]AuthoritativeDNSSnapshotSchedulingState, 0, len(states))
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		expectedType, ok := routeRecordTypes[state.ProxyRouteID]
		if !ok {
			continue
		}
		recordType := normalizeDNSRecordType(state.DNSRecordType)
		if recordType != expectedType {
			continue
		}
		selectedTargets, err := normalizeDNSRecordContents(recordType, decodeGSLBTargetList(state.SelectedTargets))
		if err != nil || len(selectedTargets) == 0 {
			continue
		}
		desiredTargets := decodeGSLBTargetList(state.DesiredTargets)
		if len(desiredTargets) > 0 {
			desiredTargets, err = normalizeDNSRecordContents(recordType, desiredTargets)
			if err != nil {
				desiredTargets = []string{}
			}
		}
		result = append(result, AuthoritativeDNSSnapshotSchedulingState{
			RouteID:         state.ProxyRouteID,
			RecordType:      recordType,
			ScopeKey:        normalizeDNSSourceScope(state.ScopeKey),
			SelectedTargets: selectedTargets,
			DesiredTargets:  desiredTargets,
			LastChangedAt:   normalizeGSLBSchedulingStateChangedAt(state.LastChangedAt, now, state.LastEvaluatedAt, &state.UpdatedAt, &state.CreatedAt),
		})
	}
	return result, nil
}

func convertAuthoritativeSnapshotToWorker(snapshot *AuthoritativeDNSSnapshot) *dnsworker.Snapshot {
	if snapshot == nil {
		return nil
	}
	result := &dnsworker.Snapshot{
		SnapshotVersion:            snapshot.SnapshotVersion,
		GeneratedAt:                snapshot.GeneratedAt,
		GSLBProbeSchedulingEnabled: snapshot.GSLBProbeSchedulingEnabled,
		Zones:                      make([]dnsworker.SnapshotZone, 0, len(snapshot.Zones)),
		Routes:                     make([]dnsworker.SnapshotRoute, 0, len(snapshot.Routes)),
		Nodes:                      make([]dnsworker.SnapshotNode, 0, len(snapshot.Nodes)),
		SchedulingStates:           make([]dnsworker.SnapshotSchedulingState, 0, len(snapshot.SchedulingStates)),
	}
	for _, zone := range snapshot.Zones {
		item := dnsworker.SnapshotZone{
			ID:          zone.ID,
			Name:        zone.Name,
			SOAEmail:    zone.SOAEmail,
			PrimaryNS:   zone.PrimaryNS,
			NameServers: append([]string(nil), zone.NameServers...),
			DefaultTTL:  zone.DefaultTTL,
			Serial:      zone.Serial,
			Records:     make([]dnsworker.SnapshotRecord, 0, len(zone.Records)),
		}
		for _, record := range zone.Records {
			item.Records = append(item.Records, dnsworker.SnapshotRecord{
				ID:       record.ID,
				Name:     record.Name,
				Type:     record.Type,
				Value:    record.Value,
				TTL:      record.TTL,
				Priority: record.Priority,
			})
		}
		result.Zones = append(result.Zones, item)
	}
	for _, route := range snapshot.Routes {
		result.Routes = append(result.Routes, dnsworker.SnapshotRoute{
			ID:             route.ID,
			SiteName:       route.SiteName,
			Domains:        append([]string(nil), route.Domains...),
			ZoneID:         route.ZoneID,
			NodePool:       route.NodePool,
			RecordType:     route.RecordType,
			TargetCount:    route.TargetCount,
			ScheduleMode:   route.ScheduleMode,
			TTL:            route.TTL,
			GSLBEnabled:    route.GSLBEnabled,
			GSLBPolicy:     convertAuthoritativeGSLBPolicyToWorker(route.GSLBPolicy),
			CurrentTargets: append([]string(nil), route.CurrentTargets...),
			TargetError:    route.TargetError,
			DDOSActive:     route.DDOSActive,
			DDOSProvider:   route.DDOSProvider,
			DDOSTarget:     route.DDOSTarget,
		})
	}
	for _, node := range snapshot.Nodes {
		result.Nodes = append(result.Nodes, dnsworker.SnapshotNode{
			NodeID:               node.NodeID,
			Name:                 node.Name,
			PoolName:             node.PoolName,
			PublicIPs:            append([]string(nil), node.PublicIPs...),
			Weight:               node.Weight,
			SchedulingEnabled:    node.SchedulingEnabled,
			DrainMode:            node.DrainMode,
			Status:               node.Status,
			OpenrestyStatus:      node.OpenrestyStatus,
			LastSeenAt:           node.LastSeenAt,
			OpenrestyConnections: node.OpenrestyConnections,
			CPUUsagePercent:      node.CPUUsagePercent,
			MemoryUsagePercent:   node.MemoryUsagePercent,
			MetricCapturedAt:     node.MetricCapturedAt,
			DNSProbeHealthy:      node.DNSProbeHealthy,
			DNSProbeCheckedCount: node.DNSProbeCheckedCount,
			DNSProbeHealthyCount: node.DNSProbeHealthyCount,
			DNSProbeStaleCount:   node.DNSProbeStaleCount,
			DNSProbeAverageRTTMs: node.DNSProbeAverageRTTMs,
			DNSProbeMaxRTTMs:     node.DNSProbeMaxRTTMs,
		})
	}
	for _, state := range snapshot.SchedulingStates {
		result.SchedulingStates = append(result.SchedulingStates, dnsworker.SnapshotSchedulingState{
			RouteID:         state.RouteID,
			RecordType:      state.RecordType,
			ScopeKey:        state.ScopeKey,
			SelectedTargets: append([]string(nil), state.SelectedTargets...),
			DesiredTargets:  append([]string(nil), state.DesiredTargets...),
			LastChangedAt:   state.LastChangedAt,
		})
	}
	return result
}

func convertAuthoritativeGSLBPolicyToWorker(policy ProxyRouteGSLBPolicy) dnsworker.GSLBPolicy {
	result := dnsworker.GSLBPolicy{
		Mode:        policy.Mode,
		Strategy:    policy.Strategy,
		TargetCount: policy.TargetCount,
		TTL:         policy.TTL,
		SourceIP: dnsworker.GSLBSourceIPProvider{
			Provider: policy.SourceIP.Provider,
			APIURL:   policy.SourceIP.APIURL,
			APIToken: policy.SourceIP.APIToken,
		},
		LoadThresholds: dnsworker.GSLBLoadThresholds{
			MaxOpenrestyConnections: policy.LoadThresholds.MaxOpenrestyConnections,
			MaxCPUPercent:           policy.LoadThresholds.MaxCPUPercent,
			MaxMemoryPercent:        policy.LoadThresholds.MaxMemoryPercent,
		},
		Debounce: dnsworker.GSLBDebounce{
			CooldownSeconds:    policy.Debounce.CooldownSeconds,
			UnhealthyThreshold: policy.Debounce.UnhealthyThreshold,
			RecoveryThreshold:  policy.Debounce.RecoveryThreshold,
		},
		Pools: make([]dnsworker.GSLBPoolPolicy, 0, len(policy.Pools)),
	}
	for _, pool := range policy.Pools {
		result.Pools = append(result.Pools, dnsworker.GSLBPoolPolicy{
			Name:        pool.Name,
			Weight:      pool.Weight,
			Countries:   append([]string(nil), pool.Countries...),
			SourceCIDRs: append([]string(nil), pool.SourceCIDRs...),
			NodeIDs:     append([]string(nil), pool.NodeIDs...),
			Enabled:     pool.Enabled,
		})
	}
	return result
}

func authoritativeDNSSnapshotVersion(snapshot *AuthoritativeDNSSnapshot) (string, error) {
	payload := struct {
		GSLBProbeSchedulingEnabled bool                                      `json:"gslb_probe_scheduling_enabled"`
		Zones                      []AuthoritativeDNSSnapshotZone            `json:"zones"`
		Routes                     []AuthoritativeDNSSnapshotRoute           `json:"routes"`
		Nodes                      []authoritativeDNSSnapshotVersionNode     `json:"nodes"`
		SchedulingStates           []AuthoritativeDNSSnapshotSchedulingState `json:"scheduling_states,omitempty"`
	}{
		GSLBProbeSchedulingEnabled: snapshot.GSLBProbeSchedulingEnabled,
		Zones:                      snapshot.Zones,
		Routes:                     snapshot.Routes,
		Nodes:                      authoritativeDNSSnapshotVersionNodes(snapshot.Nodes),
		SchedulingStates:           snapshot.SchedulingStates,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:24], nil
}

type authoritativeDNSSnapshotVersionNode struct {
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
	DNSProbeHealthy      bool       `json:"dns_probe_healthy"`
}

func authoritativeDNSSnapshotVersionNodes(nodes []AuthoritativeDNSSnapshotNode) []authoritativeDNSSnapshotVersionNode {
	result := make([]authoritativeDNSSnapshotVersionNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, authoritativeDNSSnapshotVersionNode{
			NodeID:               node.NodeID,
			Name:                 node.Name,
			PoolName:             node.PoolName,
			PublicIPs:            append([]string(nil), node.PublicIPs...),
			Weight:               node.Weight,
			SchedulingEnabled:    node.SchedulingEnabled,
			DrainMode:            node.DrainMode,
			Status:               node.Status,
			OpenrestyStatus:      node.OpenrestyStatus,
			LastSeenAt:           node.LastSeenAt,
			OpenrestyConnections: node.OpenrestyConnections,
			CPUUsagePercent:      node.CPUUsagePercent,
			MemoryUsagePercent:   node.MemoryUsagePercent,
			MetricCapturedAt:     node.MetricCapturedAt,
			DNSProbeHealthy:      node.DNSProbeHealthy,
		})
	}
	return result
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
			QName:           normalizeDNSRecordName(input.QName),
			QType:           normalizeAuthoritativeDNSRecordTypeOrDefault(input.QType),
			RCode:           normalizeDNSRCode(input.RCode),
			QueryCount:      input.QueryCount,
			TotalDurationMs: totalDurationMs,
			MaxDurationMs:   maxDurationMs,
			TargetSummary:   string(targetSummaryJSON),
		}
		if rollup.WindowStart.IsZero() {
			rollup.WindowStart = time.Now().UTC().Truncate(time.Minute)
		}
		rollups = append(rollups, rollup)
	}
	if len(rollups) == 0 {
		return nil
	}
	return model.DB.CreateInBatches(rollups, 500).Error
}

func persistDNSWorkerSchedulingStates(inputs []AuthoritativeDNSSnapshotSchedulingState) error {
	if len(inputs) == 0 {
		return nil
	}
	routeIDs := make([]uint, 0, len(inputs))
	seenRoutes := map[uint]struct{}{}
	for _, input := range inputs {
		if input.RouteID == 0 {
			continue
		}
		if _, ok := seenRoutes[input.RouteID]; ok {
			continue
		}
		seenRoutes[input.RouteID] = struct{}{}
		routeIDs = append(routeIDs, input.RouteID)
	}
	if len(routeIDs) == 0 {
		return nil
	}
	var routes []*model.ProxyRoute
	if err := model.DB.
		Select("id", "dns_record_type").
		Where("id IN ? AND enabled = ? AND dns_provider_mode = ? AND dns_zone_id_ref IS NOT NULL", routeIDs, true, DNSProviderModeAuthoritative).
		Find(&routes).Error; err != nil {
		return err
	}
	routeRecordTypes := make(map[uint]string, len(routes))
	for _, route := range routes {
		if route == nil || route.ID == 0 {
			continue
		}
		recordType := normalizeDNSRecordType(route.DNSRecordType)
		if recordType != "A" && recordType != "AAAA" {
			continue
		}
		routeRecordTypes[route.ID] = recordType
	}
	now := time.Now().UTC()
	updates := make([]dnsWorkerSchedulingStateUpdate, 0, len(inputs))
	for _, input := range inputs {
		expectedType, ok := routeRecordTypes[input.RouteID]
		if !ok {
			continue
		}
		recordType := normalizeDNSRecordType(input.RecordType)
		if recordType != expectedType {
			continue
		}
		scopeKey := normalizeDNSSourceScope(input.ScopeKey)
		selectedTargets, err := normalizeDNSRecordContents(recordType, input.SelectedTargets)
		if err != nil || len(selectedTargets) == 0 {
			continue
		}
		desiredTargets := input.DesiredTargets
		if len(desiredTargets) > 0 {
			desiredTargets, err = normalizeDNSRecordContents(recordType, desiredTargets)
			if err != nil {
				desiredTargets = []string{}
			}
		}
		lastChangedAt := normalizeGSLBSchedulingStateChangedAt(input.LastChangedAt, now)
		if lastChangedAt == nil || lastChangedAt.IsZero() {
			lastChangedAt = &now
		}
		updates = append(updates, dnsWorkerSchedulingStateUpdate{
			key: dnsWorkerSchedulingStateKey{
				routeID:    input.RouteID,
				recordType: recordType,
				scopeKey:   scopeKey,
			},
			selectedTargets: selectedTargets,
			desiredTargets:  desiredTargets,
			lastChangedAt:   *lastChangedAt,
		})
	}
	if len(updates) == 0 {
		return nil
	}
	statesByKey, err := loadGSLBSchedulingStatesForUpdates(updates)
	if err != nil {
		return err
	}
	dirtyKeys := make([]dnsWorkerSchedulingStateKey, 0, len(updates))
	seenDirtyKeys := make(map[dnsWorkerSchedulingStateKey]struct{}, len(updates))
	for _, update := range updates {
		state := statesByKey[update.key]
		if state == nil {
			state = &model.GSLBSchedulingState{
				ProxyRouteID:  update.key.routeID,
				DNSRecordType: update.key.recordType,
				ScopeKey:      update.key.scopeKey,
				CreatedAt:     now,
			}
			statesByKey[update.key] = state
		}
		if !applyDNSWorkerSchedulingStateUpdate(state, update, now) {
			continue
		}
		if _, ok := seenDirtyKeys[update.key]; ok {
			continue
		}
		seenDirtyKeys[update.key] = struct{}{}
		dirtyKeys = append(dirtyKeys, update.key)
	}
	if len(dirtyKeys) == 0 {
		return nil
	}
	rows := make([]*model.GSLBSchedulingState, 0, len(dirtyKeys))
	for _, key := range dirtyKeys {
		state := statesByKey[key]
		if state == nil {
			continue
		}
		rows = append(rows, &model.GSLBSchedulingState{
			ProxyRouteID:    state.ProxyRouteID,
			DNSRecordType:   state.DNSRecordType,
			ScopeKey:        state.ScopeKey,
			SelectedTargets: state.SelectedTargets,
			DesiredTargets:  state.DesiredTargets,
			LastReason:      state.LastReason,
			LastChangedAt:   state.LastChangedAt,
			LastEvaluatedAt: state.LastEvaluatedAt,
			CreatedAt:       state.CreatedAt,
			UpdatedAt:       now,
		})
	}
	return model.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "proxy_route_id"},
			{Name: "dns_record_type"},
			{Name: "scope_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"selected_targets",
			"desired_targets",
			"last_reason",
			"last_changed_at",
			"last_evaluated_at",
			"updated_at",
		}),
	}).CreateInBatches(rows, 100).Error
}

type dnsWorkerSchedulingStateKey struct {
	routeID    uint
	recordType string
	scopeKey   string
}

type dnsWorkerSchedulingStateUpdate struct {
	key             dnsWorkerSchedulingStateKey
	selectedTargets []string
	desiredTargets  []string
	lastChangedAt   time.Time
}

func loadGSLBSchedulingStatesForUpdates(updates []dnsWorkerSchedulingStateUpdate) (map[dnsWorkerSchedulingStateKey]*model.GSLBSchedulingState, error) {
	statesByKey := make(map[dnsWorkerSchedulingStateKey]*model.GSLBSchedulingState, len(updates))
	routeIDs := make([]uint, 0, len(updates))
	recordTypes := make([]string, 0, len(updates))
	scopeKeys := make([]string, 0, len(updates))
	seenRouteIDs := make(map[uint]struct{}, len(updates))
	seenRecordTypes := make(map[string]struct{}, len(updates))
	seenScopeKeys := make(map[string]struct{}, len(updates))
	updateKeys := make(map[dnsWorkerSchedulingStateKey]struct{}, len(updates))
	for _, update := range updates {
		key := update.key
		updateKeys[key] = struct{}{}
		if _, ok := seenRouteIDs[key.routeID]; !ok {
			seenRouteIDs[key.routeID] = struct{}{}
			routeIDs = append(routeIDs, key.routeID)
		}
		if _, ok := seenRecordTypes[key.recordType]; !ok {
			seenRecordTypes[key.recordType] = struct{}{}
			recordTypes = append(recordTypes, key.recordType)
		}
		if _, ok := seenScopeKeys[key.scopeKey]; !ok {
			seenScopeKeys[key.scopeKey] = struct{}{}
			scopeKeys = append(scopeKeys, key.scopeKey)
		}
	}
	if len(routeIDs) == 0 || len(recordTypes) == 0 || len(scopeKeys) == 0 {
		return statesByKey, nil
	}
	var states []*model.GSLBSchedulingState
	if err := model.DB.
		Where("proxy_route_id IN ? AND dns_record_type IN ? AND scope_key IN ?", routeIDs, recordTypes, scopeKeys).
		Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		key := dnsWorkerSchedulingStateKey{
			routeID:    state.ProxyRouteID,
			recordType: normalizeDNSRecordType(state.DNSRecordType),
			scopeKey:   normalizeDNSSourceScope(state.ScopeKey),
		}
		if _, ok := updateKeys[key]; !ok {
			continue
		}
		statesByKey[key] = state
	}
	return statesByKey, nil
}

func applyDNSWorkerSchedulingStateUpdate(state *model.GSLBSchedulingState, update dnsWorkerSchedulingStateUpdate, evaluatedAt time.Time) bool {
	if state == nil {
		return false
	}
	changedAt := update.lastChangedAt.UTC()
	evaluated := evaluatedAt.UTC()
	if state.LastChangedAt != nil {
		existingChangedAt := state.LastChangedAt.UTC()
		if !existingChangedAt.After(evaluated) && existingChangedAt.After(changedAt) {
			return false
		}
	}
	state.DNSRecordType = update.key.recordType
	state.ScopeKey = update.key.scopeKey
	state.SelectedTargets = encodeGSLBTargetList(update.selectedTargets)
	state.DesiredTargets = encodeGSLBTargetList(update.desiredTargets)
	state.LastReason = "reported by DNS Worker heartbeat"
	state.LastChangedAt = &changedAt
	state.LastEvaluatedAt = &evaluated
	return true
}

func normalizeGSLBSchedulingStateChangedAt(changedAt *time.Time, now time.Time, fallbacks ...*time.Time) *time.Time {
	if changedAt == nil {
		return nil
	}
	normalizedNow := now.UTC()
	normalized := changedAt.UTC()
	if !normalized.After(normalizedNow) {
		return &normalized
	}
	for _, fallback := range fallbacks {
		if fallback == nil || fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return &normalized
		}
	}
	normalized = normalizedNow
	return &normalized
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
	result := make(map[string]int64, len(values))
	for target, count := range values {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" || count <= 0 {
			continue
		}
		result[trimmed] += count
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
	checked := normalizeDNSWorkerCheckedAt(checkedAt, now, time.Time{})
	age := now.UTC().Sub(checked.UTC())
	ageSeconds := int64(0)
	if age > 0 {
		ageSeconds = int64(age.Seconds())
	}
	if age > defaultDNSWorkerProbeMaxAge {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeStale,
			ageSeconds: ageSeconds,
			message:    fmt.Sprintf("最近一次公网 UDP/TCP 53 探测超过 %s 未刷新", formatDNSWorkerNodeProbeMaxAge(defaultDNSWorkerProbeMaxAge)),
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

func queryDNSObservabilityStringCounts(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, field string, limit int) (map[string]int64, error) {
	expression := dnsObservabilityStringCountExpression(field)
	if expression == "" {
		return map[string]int64{}, nil
	}
	if field == "qname" && limit > 0 {
		return queryDNSObservabilityStringCountsFromRecentRows(input, window, field, expression, limit)
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

func queryDNSObservabilityStringCountsFromRecentRows(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, field string, expression string, limit int) (map[string]int64, error) {
	scanLimit := normalizeDNSObservabilityHeavyCounterScanLimit()
	var rows []dnsObservabilityStringCountRow
	if err := dnsObservabilityBaseQuery(input, window).
		Select(expression + " AS key, query_count AS count").
		Order("window_start DESC").
		Order("id DESC").
		Limit(scanLimit).
		Scan(&rows).Error; err != nil {
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
	return limitDNSObservabilityCounterMap(result, limit), nil
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
	default:
		return ""
	}
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
	default:
		return value
	}
}

func queryDNSObservabilityUintCounts(input DNSObservabilitySummaryInput, window dnsObservabilityWindow, field string, limit int, nonZeroClause string) (map[uint]int64, error) {
	if field != "zone_id" && field != "proxy_route_id" {
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
			WorkerID:        worker.WorkerID,
			Name:            workerName,
			Status:          status,
			SnapshotVersion: snapshotVersion,
			LastSnapshotAt:  snapshotAt,
			LastSeenAt:      worker.LastSeenAt,
			Stale:           stale,
			GeoIPEnabled:    worker.GeoIPEnabled,
			GeoIPLastError:  worker.GeoIPLastError,
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

type dnsWorkerNodeProbeStats struct {
	totalCount        int
	healthyCount      int
	partialCount      int
	failedCount       int
	unknownCount      int
	staleCount        int
	totalAverageRTTMs float64
	averageSamples    int
	maxRTTMs          int64
	probes            []DNSWorkerNodeProbeView
}

type dnsWorkerProbeTargetNode struct {
	NodeID   string
	Name     string
	PoolName string
	Status   string
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
			WorkerID:                worker.WorkerID,
			Name:                    workerName,
			Status:                  status,
			PublicAddress:           worker.PublicAddress,
			QueryCount:              stats.queryCount,
			ErrorQueries:            stats.errorQueries,
			ErrorRatePercent:        ratioPercent(stats.errorQueries, stats.queryCount),
			AverageLatencyMs:        averageMilliseconds(stats.totalDurationMs, stats.queryCount),
			MaxLatencyMs:            stats.maxDurationMs,
			LastSeenAt:              worker.LastSeenAt,
			LastSnapshotAt:          snapshotAt,
			SnapshotAgeSeconds:      snapshotAgeSeconds,
			SnapshotStale:           snapshotStale,
			GeoIPEnabled:            worker.GeoIPEnabled,
			GeoIPDatabasePath:       worker.GeoIPDatabasePath,
			GeoIPLastError:          worker.GeoIPLastError,
			LastError:               worker.LastError,
			LastProbeAt:             probeAt,
			LastProbeResults:        probeResults,
			ProbeStatus:             probeState.status,
			ProbeHealthy:            probeState.healthy,
			ProbeAgeSeconds:         probeState.ageSeconds,
			ProbeMessage:            probeState.message,
			NodeProbeTotalCount:     nodeProbeStats.totalCount,
			NodeProbeHealthyCount:   nodeProbeStats.healthyCount,
			NodeProbeStaleCount:     nodeProbeStats.staleCount,
			NodeProbeHealthyPercent: ratioPercent(int64(nodeProbeStats.healthyCount), int64(nodeProbeStats.totalCount)),
			NodeProbeAverageRTTMs:   averageFloat(nodeProbeStats.totalAverageRTTMs, nodeProbeStats.averageSamples),
			NodeProbeMaxRTTMs:       nodeProbeStats.maxRTTMs,
			NodeProbes:              nodeProbeStats.probes,
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

func buildDNSWorkerNodeProbeStats(now time.Time) map[string]*dnsWorkerNodeProbeStats {
	return buildDNSWorkerNodeProbeStatsForWorkerIDs(now, activeDNSWorkerIDs())
}

func buildDNSWorkerNodeProbeStatsForWorkerIDs(now time.Time, workerIDs []string) map[string]*dnsWorkerNodeProbeStats {
	nodes, err := model.ListNodes()
	if err != nil {
		return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nil, workerIDs)
	}
	return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nodes, workerIDs)
}

func buildDNSWorkerNodeProbeStatsForNodes(now time.Time, nodes []*model.Node) map[string]*dnsWorkerNodeProbeStats {
	return buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now, nodes, activeDNSWorkerIDs())
}

func buildDNSWorkerNodeProbeStatsForNodesAndWorkers(now time.Time, nodes []*model.Node, workerIDs []string) map[string]*dnsWorkerNodeProbeStats {
	nodesByID := make(map[string]*model.Node, len(nodes))
	targetNodes := make([]dnsWorkerProbeTargetNode, 0, len(nodes))
	targetNodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		nodesByID[node.NodeID] = node
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		targetNodes = append(targetNodes, dnsWorkerProbeTargetNode{
			NodeID:   strings.TrimSpace(node.NodeID),
			Name:     displayNodeName(node),
			PoolName: strings.TrimSpace(node.PoolName),
			Status:   computeNodeStatus(node),
		})
		targetNodeIDs = append(targetNodeIDs, strings.TrimSpace(node.NodeID))
	}
	sort.SliceStable(targetNodes, func(i, j int) bool {
		if targetNodes[i].PoolName != targetNodes[j].PoolName {
			return targetNodes[i].PoolName < targetNodes[j].PoolName
		}
		if targetNodes[i].Name != targetNodes[j].Name {
			return targetNodes[i].Name < targetNodes[j].Name
		}
		return targetNodes[i].NodeID < targetNodes[j].NodeID
	})

	statsByWorker := make(map[string]*dnsWorkerNodeProbeStats)
	for _, workerID := range workerIDs {
		stats := &dnsWorkerNodeProbeStats{probes: []DNSWorkerNodeProbeView{}}
		statsByWorker[workerID] = stats
		for _, node := range targetNodes {
			if node.NodeID == "" {
				continue
			}
			stats.probes = append(stats.probes, DNSWorkerNodeProbeView{
				NodeID:       node.NodeID,
				NodeName:     node.Name,
				PoolName:     node.PoolName,
				Status:       node.Status,
				ProbeStatus:  dnsWorkerProbeUnknown,
				ProbeMessage: "尚未收到 Agent 多节点探测结果",
				Results:      []DNSWorkerProbeResultView{},
			})
		}
	}

	probes, err := model.ListDNSWorkerNodeProbesByScope(workerIDs, targetNodeIDs)
	if err != nil {
		return statsByWorker
	}
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		workerID := strings.TrimSpace(probe.WorkerID)
		nodeID := strings.TrimSpace(probe.NodeID)
		if workerID == "" || nodeID == "" {
			continue
		}
		node := nodesByID[nodeID]
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		stats := statsByWorker[workerID]
		if stats == nil {
			stats = &dnsWorkerNodeProbeStats{probes: []DNSWorkerNodeProbeView{}}
			statsByWorker[workerID] = stats
		}
		nodeName := displayNodeName(node)
		poolName := strings.TrimSpace(node.PoolName)
		nodeStatus := computeNodeStatus(node)
		probeState := evaluateDNSWorkerNodeProbeState(now, probe)
		checkedAt := normalizeDNSWorkerCheckedAt(&probe.CheckedAt, now, probe.UpdatedAt, probe.CreatedAt)
		existingIndex := findDNSWorkerNodeProbeViewIndex(stats.probes, nodeID)
		averageRTTMs, maxRTTMs := normalizeDNSWorkerNodeProbeRTT(probe.AverageRTTMs, probe.MaxRTTMs)
		view := DNSWorkerNodeProbeView{
			NodeID:          nodeID,
			NodeName:        nodeName,
			PoolName:        poolName,
			Status:          nodeStatus,
			CheckedAt:       checkedAt,
			Healthy:         probeState.healthy,
			ProbeStatus:     probeState.status,
			ProbeAgeSeconds: probeState.ageSeconds,
			ProbeMessage:    probeState.message,
			AverageRTTMs:    averageRTTMs,
			MaxRTTMs:        maxRTTMs,
			Results:         decodeDNSWorkerProbeResults(probe.ResultsJSON),
			LastError:       probe.LastError,
			FailureSamples:  probe.FailureSamples,
		}
		if existingIndex >= 0 {
			stats.probes[existingIndex] = view
		} else {
			stats.probes = append(stats.probes, view)
		}
	}
	for _, stats := range statsByWorker {
		recomputeDNSWorkerNodeProbeStats(stats)
		sort.SliceStable(stats.probes, func(i, j int) bool {
			if stats.probes[i].ProbeStatus != stats.probes[j].ProbeStatus {
				return dnsWorkerProbeStatusSortRank(stats.probes[i].ProbeStatus) < dnsWorkerProbeStatusSortRank(stats.probes[j].ProbeStatus)
			}
			if stats.probes[i].Healthy != stats.probes[j].Healthy {
				return stats.probes[i].Healthy
			}
			if !stats.probes[i].CheckedAt.Equal(stats.probes[j].CheckedAt) {
				return stats.probes[i].CheckedAt.After(stats.probes[j].CheckedAt)
			}
			if stats.probes[i].PoolName != stats.probes[j].PoolName {
				return stats.probes[i].PoolName < stats.probes[j].PoolName
			}
			if stats.probes[i].NodeName != stats.probes[j].NodeName {
				return stats.probes[i].NodeName < stats.probes[j].NodeName
			}
			return stats.probes[i].NodeID < stats.probes[j].NodeID
		})
	}
	return statsByWorker
}

func currentSchedulableDNSProbeNodeIDs() map[string]struct{} {
	nodes, err := model.ListNodes()
	if err != nil {
		return map[string]struct{}{}
	}
	result := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			continue
		}
		result[nodeID] = struct{}{}
	}
	return result
}

func recomputeDNSWorkerNodeProbeStats(stats *dnsWorkerNodeProbeStats) {
	if stats == nil {
		return
	}
	stats.totalCount = 0
	stats.healthyCount = 0
	stats.partialCount = 0
	stats.failedCount = 0
	stats.unknownCount = 0
	stats.staleCount = 0
	stats.totalAverageRTTMs = 0
	stats.averageSamples = 0
	stats.maxRTTMs = 0
	for _, probe := range stats.probes {
		stats.totalCount++
		switch probe.ProbeStatus {
		case dnsWorkerProbeHealthy:
			stats.healthyCount++
		case dnsWorkerProbePartial:
			stats.partialCount++
		case dnsWorkerProbeFailed:
			stats.failedCount++
		case dnsWorkerProbeStale:
			stats.staleCount++
		case dnsWorkerProbeUnknown:
			stats.unknownCount++
		}
		if probe.ProbeStatus != dnsWorkerProbeStale && probe.AverageRTTMs > 0 {
			stats.totalAverageRTTMs += probe.AverageRTTMs
			stats.averageSamples++
		}
		if probe.ProbeStatus != dnsWorkerProbeStale && probe.MaxRTTMs > stats.maxRTTMs {
			stats.maxRTTMs = probe.MaxRTTMs
		}
	}
}

func shouldExpectAgentDNSProbeForNode(node *model.Node) bool {
	if node == nil {
		return false
	}
	if strings.TrimSpace(node.NodeID) == "" {
		return false
	}
	if !node.SchedulingEnabled || node.DrainMode {
		return false
	}
	return computeNodeStatus(node) == NodeStatusOnline
}

func displayNodeName(node *model.Node) string {
	if node == nil {
		return ""
	}
	name := strings.TrimSpace(node.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(node.NodeID)
}

func expectedAgentDNSProbeWorkerIDs() []string {
	targets := buildAgentDNSProbeTargets()
	workerIDs := make([]string, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		workerID := strings.TrimSpace(target.WorkerID)
		if workerID == "" {
			continue
		}
		if _, ok := seen[workerID]; ok {
			continue
		}
		seen[workerID] = struct{}{}
		workerIDs = append(workerIDs, workerID)
	}
	return workerIDs
}

func activeDNSWorkerIDs() []string {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return []string{}
	}
	workerIDs := make([]string, 0, len(workers))
	seen := make(map[string]struct{}, len(workers))
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		workerID := strings.TrimSpace(worker.WorkerID)
		if workerID == "" {
			continue
		}
		if _, ok := seen[workerID]; ok {
			continue
		}
		seen[workerID] = struct{}{}
		workerIDs = append(workerIDs, workerID)
	}
	return workerIDs
}

func findDNSWorkerNodeProbeViewIndex(probes []DNSWorkerNodeProbeView, nodeID string) int {
	nodeID = strings.TrimSpace(nodeID)
	for index, probe := range probes {
		if strings.TrimSpace(probe.NodeID) == nodeID {
			return index
		}
	}
	return -1
}

func dnsWorkerProbeStatusSortRank(status string) int {
	switch status {
	case dnsWorkerProbeFailed, dnsWorkerProbePartial:
		return 0
	case dnsWorkerProbeStale:
		return 1
	case dnsWorkerProbeUnknown:
		return 2
	case dnsWorkerProbeHealthy:
		return 3
	default:
		return 4
	}
}

func buildDNSWorkerNodeProbeStatsByNode(now time.Time) map[string]*dnsWorkerNodeProbeStats {
	nodes, err := model.ListNodes()
	if err != nil {
		return map[string]*dnsWorkerNodeProbeStats{}
	}
	return buildDNSWorkerNodeProbeStatsByNodeForNodes(now, nodes)
}

func buildDNSWorkerNodeProbeStatsByNodeForNodes(now time.Time, nodes []*model.Node) map[string]*dnsWorkerNodeProbeStats {
	currentNodeIDs := currentSchedulableDNSProbeNodeIDsForNodes(nodes)
	nodeIDs := make([]string, 0, len(currentNodeIDs))
	for nodeID := range currentNodeIDs {
		nodeIDs = append(nodeIDs, nodeID)
	}
	probes, err := model.ListDNSWorkerNodeProbesByScope(activeDNSWorkerIDs(), nodeIDs)
	if err != nil || len(probes) == 0 {
		return map[string]*dnsWorkerNodeProbeStats{}
	}
	statsByNode := make(map[string]*dnsWorkerNodeProbeStats)
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		nodeID := strings.TrimSpace(probe.NodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := currentNodeIDs[nodeID]; !ok {
			continue
		}
		stats := statsByNode[nodeID]
		if stats == nil {
			stats = &dnsWorkerNodeProbeStats{}
			statsByNode[nodeID] = stats
		}
		probeState := evaluateDNSWorkerNodeProbeState(now, probe)
		stats.totalCount++
		switch probeState.status {
		case dnsWorkerProbeHealthy:
			stats.healthyCount++
		case dnsWorkerProbePartial:
			stats.partialCount++
		case dnsWorkerProbeFailed:
			stats.failedCount++
		case dnsWorkerProbeStale:
			stats.staleCount++
		case dnsWorkerProbeUnknown:
			stats.unknownCount++
		}
		averageRTTMs, maxRTTMs := normalizeDNSWorkerNodeProbeRTT(probe.AverageRTTMs, probe.MaxRTTMs)
		if probeState.status != dnsWorkerProbeStale && averageRTTMs > 0 {
			stats.totalAverageRTTMs += averageRTTMs
			stats.averageSamples++
		}
		if probeState.status != dnsWorkerProbeStale && maxRTTMs > stats.maxRTTMs {
			stats.maxRTTMs = maxRTTMs
		}
	}
	return statsByNode
}

func currentSchedulableDNSProbeNodeIDsForNodes(nodes []*model.Node) map[string]struct{} {
	result := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if !shouldExpectAgentDNSProbeForNode(node) {
			continue
		}
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID == "" {
			continue
		}
		result[nodeID] = struct{}{}
	}
	return result
}

func normalizeDNSWorkerNodeProbeRTT(averageRTTMs float64, maxRTTMs int64) (float64, int64) {
	if averageRTTMs < 0 {
		averageRTTMs = 0
	}
	if maxRTTMs < 0 {
		maxRTTMs = 0
	}
	if averageRTTMs > 0 && float64(maxRTTMs) < averageRTTMs {
		maxRTTMs = int64(math.Ceil(averageRTTMs))
	}
	return averageRTTMs, maxRTTMs
}

type dnsWorkerNodeProbeSummary struct {
	status         string
	message        string
	checkedCount   int
	healthyCount   int
	staleCount     int
	healthyPercent float64
	averageRTTMs   float64
	maxRTTMs       int64
}

func summarizeDNSWorkerNodeProbeStats(stats *dnsWorkerNodeProbeStats) dnsWorkerNodeProbeSummary {
	if stats == nil || stats.totalCount == 0 {
		return dnsWorkerNodeProbeSummary{
			status:  dnsWorkerProbeUnknown,
			message: "尚未收到该节点的 DNS Worker 多点探测结果",
		}
	}
	freshCount := stats.totalCount - stats.staleCount
	summary := dnsWorkerNodeProbeSummary{
		checkedCount:   stats.totalCount,
		healthyCount:   stats.healthyCount,
		staleCount:     stats.staleCount,
		healthyPercent: ratioPercent(int64(stats.healthyCount), int64(stats.totalCount)),
		averageRTTMs:   averageFloat(stats.totalAverageRTTMs, stats.averageSamples),
		maxRTTMs:       stats.maxRTTMs,
	}
	switch {
	case freshCount <= 0:
		summary.status = dnsWorkerProbeStale
		summary.message = "该节点的 DNS Worker 多点探测结果均已过期"
	case stats.healthyCount == freshCount:
		summary.status = dnsWorkerProbeHealthy
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测全部可达（%d/%d）", stats.healthyCount, freshCount)
	case stats.healthyCount > 0:
		summary.status = dnsWorkerProbePartial
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测部分可达（%d/%d）", stats.healthyCount, freshCount)
	case stats.partialCount > 0:
		summary.status = dnsWorkerProbePartial
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测 UDP/TCP 53 未同时可达（0/%d）", freshCount)
	case stats.failedCount > 0:
		summary.status = dnsWorkerProbeFailed
		summary.message = fmt.Sprintf("该节点到 DNS Worker 多点探测均失败（0/%d）", freshCount)
	default:
		summary.status = dnsWorkerProbeUnknown
		summary.message = "尚未收到该节点的 DNS Worker 多点探测结果"
	}
	if stats.staleCount > 0 && freshCount > 0 {
		summary.message += fmt.Sprintf("，另有 %d 个过期", stats.staleCount)
	}
	return summary
}

func dnsWorkerNodeProbeStatsSchedulable(stats *dnsWorkerNodeProbeStats) bool {
	if stats == nil {
		return false
	}
	return stats.healthyCount > 0 && stats.totalCount > stats.staleCount
}

func dnsWorkerNodeProbeThresholdReason(stats *dnsWorkerNodeProbeStats) string {
	summary := summarizeDNSWorkerNodeProbeStats(stats)
	switch summary.status {
	case dnsWorkerProbeStale:
		return "Agent 探测未达到调度门槛：探测结果已过期"
	case dnsWorkerProbePartial:
		return "Agent 探测未达到调度门槛：UDP/TCP 53 未同时可达"
	case dnsWorkerProbeFailed:
		return "Agent 探测未达到调度门槛：UDP/TCP 53 探测均失败"
	default:
		return "Agent 探测未达到调度门槛：尚未收到新鲜成功探测"
	}
}

func evaluateDNSWorkerNodeProbeState(now time.Time, probe *model.DNSWorkerNodeProbe) dnsWorkerProbeState {
	if probe == nil || probe.CheckedAt.IsZero() {
		return dnsWorkerProbeState{
			status:  dnsWorkerProbeUnknown,
			healthy: false,
			message: "尚未收到 Agent 多节点探测结果",
		}
	}
	checkedAt := normalizeDNSWorkerCheckedAt(&probe.CheckedAt, now, probe.UpdatedAt, probe.CreatedAt)
	age := now.Sub(checkedAt)
	ageSeconds := int64(0)
	if age > 0 {
		ageSeconds = int64(age.Seconds())
	}
	if age > defaultDNSWorkerNodeProbeMaxAge {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeStale,
			healthy:    false,
			ageSeconds: ageSeconds,
			message:    fmt.Sprintf("Agent 多节点探测结果超过 %s 未刷新", formatDNSWorkerNodeProbeMaxAge(defaultDNSWorkerNodeProbeMaxAge)),
		}
	}
	results := decodeDNSWorkerProbeResults(probe.ResultsJSON)
	if probe.Healthy {
		return dnsWorkerProbeState{
			status:     dnsWorkerProbeHealthy,
			healthy:    true,
			ageSeconds: ageSeconds,
			message:    "UDP/TCP 53 均可达",
		}
	}
	for _, result := range results {
		if result.Reachable {
			return dnsWorkerProbeState{
				status:     dnsWorkerProbePartial,
				healthy:    false,
				ageSeconds: ageSeconds,
				message:    strings.TrimSpace(firstNonEmpty(probe.LastError, "UDP/TCP 53 未同时可达")),
			}
		}
	}
	return dnsWorkerProbeState{
		status:     dnsWorkerProbeFailed,
		healthy:    false,
		ageSeconds: ageSeconds,
		message:    strings.TrimSpace(firstNonEmpty(probe.LastError, "Agent 多节点探测失败")),
	}
}

func formatDNSWorkerNodeProbeMaxAge(value time.Duration) string {
	if value <= 0 {
		return "0 秒"
	}
	if value < time.Minute {
		return fmt.Sprintf("%d 秒", int(value.Seconds()))
	}
	if value < time.Hour {
		return fmt.Sprintf("%d 分钟", int(value.Minutes()))
	}
	return fmt.Sprintf("%d 小时", int(value.Hours()))
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
	raw := strings.TrimSpace(common.GetOptionValue("AuthoritativeDNSDefaultTTL"))
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
	raw := strings.TrimSpace(common.GetOptionValue("AuthoritativeDNSSnapshotMaxAge"))
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
	if !isValidAuthoritativeDNSRecordName(name) {
		return "", errors.New("DNS record name format is invalid")
	}
	if !domainBelongsToZone(name, zoneName) {
		return "", errors.New("DNS record name is outside the zone")
	}
	return name, nil
}

func isValidAuthoritativeDNSRecordName(name string) bool {
	name = normalizeDNSRecordName(name)
	if name == "" || len(name) > 253 || strings.ContainsAny(name, " \t\r\n;{}\"'`$:\\/") || strings.Contains(name, "://") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				continue
			}
			return false
		}
	}
	return true
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

func normalizeDNSWorkerSnapshotAt(snapshotAt *time.Time, now time.Time, fallbacks ...time.Time) *time.Time {
	if snapshotAt == nil {
		return nil
	}
	normalizedNow := now.UTC()
	normalized := snapshotAt.UTC()
	if !normalized.After(normalizedNow) {
		return &normalized
	}
	for _, fallback := range fallbacks {
		if fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return &normalized
		}
	}
	normalized = normalizedNow
	return &normalized
}

func normalizeDNSWorkerProbeAt(probeAt *time.Time, now time.Time, fallbacks ...time.Time) *time.Time {
	if probeAt == nil {
		return nil
	}
	normalized := normalizeDNSWorkerCheckedAt(probeAt, now, fallbacks...)
	return &normalized
}

func normalizeDNSWorkerCheckedAt(checkedAt *time.Time, now time.Time, fallbacks ...time.Time) time.Time {
	if checkedAt == nil {
		return now.UTC()
	}
	normalizedNow := now.UTC()
	normalized := checkedAt.UTC()
	if !normalized.After(normalizedNow) {
		return normalized
	}
	for _, fallback := range fallbacks {
		if fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return normalized
		}
	}
	return normalizedNow
}

func normalizeDNSRollupWindow(value int) int {
	if value <= 0 {
		return 1
	}
	if value > defaultDNSMaxRollupWindowMinutes {
		return defaultDNSMaxRollupWindowMinutes
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
