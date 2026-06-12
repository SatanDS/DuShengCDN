package service

import (
	"time"

	"dushengcdn/internal/dnsworker"
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
	defaultDNSMaxHeartbeatRollups    = 5000
	defaultDNSMaxRollupTargetSummary = 64
	defaultDNSRollupFutureTolerance  = time.Minute
	dnsWorkerAgentUpdateAckDelay     = 2 * time.Minute
)

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
	Remark        string `json:"remark"`
}

type DNSZoneWorkerAssignmentInput struct {
	WorkerIDs []uint `json:"worker_ids"`
}

type DNSZoneWorkerAssignmentView struct {
	ZoneID    uint            `json:"zone_id"`
	WorkerIDs []uint          `json:"worker_ids"`
	Workers   []DNSWorkerView `json:"workers"`
}

type DNSZoneView struct {
	ID                      uint            `json:"id"`
	Name                    string          `json:"name"`
	SOAEmail                string          `json:"soa_email"`
	PrimaryNS               string          `json:"primary_ns"`
	NameServers             []string        `json:"name_servers"`
	DefaultTTL              int             `json:"default_ttl"`
	Serial                  uint64          `json:"serial"`
	DNSSECEnabled           bool            `json:"dnssec_enabled"`
	DNSSECDenialMode        string          `json:"dnssec_denial_mode"`
	DNSSECNSEC3Salt         string          `json:"dnssec_nsec3_salt,omitempty"`
	DNSSECNSEC3Iterations   int             `json:"dnssec_nsec3_iterations"`
	DNSSECSignatureValidity int             `json:"dnssec_signature_validity"`
	Enabled                 bool            `json:"enabled"`
	RecordCount             int64           `json:"record_count"`
	Records                 []DNSRecordView `json:"records,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
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
	ID                       uint                       `json:"id"`
	WorkerID                 string                     `json:"worker_id"`
	Name                     string                     `json:"name"`
	Remark                   string                     `json:"remark"`
	Token                    string                     `json:"token,omitempty"`
	TokenPrefix              string                     `json:"token_prefix"`
	TokenRevokedAt           *time.Time                 `json:"token_revoked_at"`
	PublicAddress            string                     `json:"public_address"`
	Version                  string                     `json:"version"`
	Status                   string                     `json:"status"`
	LastSnapshotVersion      string                     `json:"last_snapshot_version"`
	LastSnapshotAt           *time.Time                 `json:"last_snapshot_at"`
	LastSeenAt               *time.Time                 `json:"last_seen_at"`
	LastHeartbeatAt          *time.Time                 `json:"last_heartbeat_at"`
	LastRemoteIP             string                     `json:"last_remote_ip"`
	LastRollupAt             *time.Time                 `json:"last_rollup_at"`
	LastRollupCount          int64                      `json:"last_rollup_count"`
	LastError                string                     `json:"last_error"`
	GeoIPEnabled             bool                       `json:"geoip_enabled"`
	GeoIPDatabasePath        string                     `json:"geoip_database_path"`
	ASNDatabasePath          string                     `json:"asn_database_path"`
	GeoIPLastError           string                     `json:"geoip_last_error"`
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
	LastProbeAt              *time.Time                 `json:"last_probe_at"`
	LastProbeQuery           string                     `json:"last_probe_query"`
	LastProbeResults         []DNSWorkerProbeResultView `json:"last_probe_results"`
	ProbeStatus              string                     `json:"probe_status"`
	ProbeHealthy             bool                       `json:"probe_healthy"`
	ProbeAgeSeconds          int64                      `json:"probe_age_seconds"`
	ProbeMessage             string                     `json:"probe_message"`
	CreatedAt                time.Time                  `json:"created_at"`
	UpdatedAt                time.Time                  `json:"updated_at"`
}

type DNSWorkerUpdateInput struct {
	Channel string `json:"channel"`
	TagName string `json:"tag_name"`
}

type DNSWorkerMutationInput struct {
	Remark string `json:"remark"`
}

type DNSGSLBSimulationInput struct {
	ProxyRouteID uint   `json:"proxy_route_id"`
	QName        string `json:"qname"`
	RecordType   string `json:"record_type"`
	Country      string `json:"country"`
	SourceIP     string `json:"source_ip"`
	Operator     string `json:"operator"`
	ASN          uint32 `json:"asn"`
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
	Operator        string                      `json:"operator"`
	ASN             uint32                      `json:"asn"`
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
	Name               string   `json:"name"`
	Weight             int      `json:"weight"`
	Countries          []string `json:"countries"`
	SourceCIDRs        []string `json:"source_cidrs"`
	Operators          []string `json:"operators"`
	ASNs               []uint32 `json:"asns"`
	ExcludeCountries   []string `json:"exclude_countries"`
	ExcludeSourceCIDRs []string `json:"exclude_source_cidrs"`
	ExcludeOperators   []string `json:"exclude_operators"`
	ExcludeASNs        []uint32 `json:"exclude_asns"`
	Matched            bool     `json:"matched"`
	Reason             string   `json:"reason"`
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
	UnhealthyCount     int        `json:"unhealthy_count"`
	RecoveryCount      int        `json:"recovery_count"`
	LastReason         string     `json:"last_reason"`
	LastChangedAt      *time.Time `json:"last_changed_at"`
	LastEvaluatedAt    *time.Time `json:"last_evaluated_at"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
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
	WorkerPolicy               dnsworker.WorkerPolicy                    `json:"worker_policy"`
	Zones                      []AuthoritativeDNSSnapshotZone            `json:"zones"`
	Routes                     []AuthoritativeDNSSnapshotRoute           `json:"routes"`
	Nodes                      []AuthoritativeDNSSnapshotNode            `json:"nodes"`
	SchedulingStates           []AuthoritativeDNSSnapshotSchedulingState `json:"scheduling_states,omitempty"`
}

type AuthoritativeDNSSnapshotZone struct {
	ID          uint                                 `json:"id"`
	Name        string                               `json:"name"`
	SOAEmail    string                               `json:"soa_email"`
	PrimaryNS   string                               `json:"primary_ns"`
	NameServers []string                             `json:"name_servers"`
	DefaultTTL  int                                  `json:"default_ttl"`
	Serial      uint64                               `json:"serial"`
	DNSSEC      AuthoritativeDNSSnapshotDNSSECPolicy `json:"dnssec"`
	DNSSECKeys  []AuthoritativeDNSSnapshotDNSSECKey  `json:"dnssec_keys,omitempty"`
	Records     []AuthoritativeDNSSnapshotRecord     `json:"records"`
}

type AuthoritativeDNSSnapshotDNSSECPolicy struct {
	Enabled                  bool   `json:"enabled"`
	DenialMode               string `json:"denial_mode"`
	NSEC3Salt                string `json:"nsec3_salt,omitempty"`
	NSEC3Iterations          int    `json:"nsec3_iterations"`
	SignatureValiditySeconds int    `json:"signature_validity_seconds"`
}

type AuthoritativeDNSSnapshotDNSSECKey struct {
	ID                  uint   `json:"id"`
	Role                string `json:"role"`
	Flags               uint16 `json:"flags"`
	Algorithm           uint8  `json:"algorithm"`
	PublicKey           string `json:"public_key"`
	EncryptedPrivateKey string `json:"encrypted_private_key"`
	KeyTag              uint16 `json:"key_tag"`
	Status              string `json:"status"`
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
	UnhealthyCount  int        `json:"unhealthy_count,omitempty"`
	RecoveryCount   int        `json:"recovery_count,omitempty"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`
}
