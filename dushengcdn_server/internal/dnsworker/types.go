package dnsworker

import "time"

const (
	DefaultListenAddr        = ":53"
	DefaultSnapshotPath      = "data/dns-worker-snapshot.json"
	DefaultHeartbeatInterval = 10 * time.Second
	DefaultRequestTimeout    = 10 * time.Second
	DefaultSnapshotMaxAge    = 5 * time.Minute
	DefaultAuthoritativeTTL  = 30
	DefaultZoneTTL           = 300
	DefaultQueryRateLimit    = 200
	DefaultUDPResponseSize   = 1232
	DefaultResponseRateLimit = 50
	DefaultECSIPv4Prefix     = 24
	DefaultECSIPv6Prefix     = 56

	WorkerStatusOnline  = "online"
	WorkerStatusOffline = "offline"
)

type Snapshot struct {
	SnapshotVersion            string                    `json:"snapshot_version"`
	GeneratedAt                time.Time                 `json:"generated_at"`
	GSLBProbeSchedulingEnabled bool                      `json:"gslb_probe_scheduling_enabled"`
	WorkerPolicy               WorkerPolicy              `json:"worker_policy"`
	Zones                      []SnapshotZone            `json:"zones"`
	Routes                     []SnapshotRoute           `json:"routes"`
	Nodes                      []SnapshotNode            `json:"nodes"`
	SchedulingStates           []SnapshotSchedulingState `json:"scheduling_states,omitempty"`
}

type WorkerPolicy struct {
	QueryRateLimit    int  `json:"query_rate_limit"`
	ResponseRateLimit int  `json:"response_rate_limit"`
	UDPResponseSize   int  `json:"udp_response_size"`
	ECSEnabled        bool `json:"ecs_enabled"`
	ECSIPv4Prefix     int  `json:"ecs_ipv4_prefix"`
	ECSIPv6Prefix     int  `json:"ecs_ipv6_prefix"`
}

type SnapshotZone struct {
	ID          uint                 `json:"id"`
	Name        string               `json:"name"`
	SOAEmail    string               `json:"soa_email"`
	PrimaryNS   string               `json:"primary_ns"`
	NameServers []string             `json:"name_servers"`
	DefaultTTL  int                  `json:"default_ttl"`
	Serial      uint64               `json:"serial"`
	DNSSEC      SnapshotDNSSECPolicy `json:"dnssec"`
	DNSSECKeys  []SnapshotDNSSECKey  `json:"dnssec_keys,omitempty"`
	Records     []SnapshotRecord     `json:"records"`
}

type SnapshotDNSSECPolicy struct {
	Enabled                  bool   `json:"enabled"`
	DenialMode               string `json:"denial_mode"`
	NSEC3Salt                string `json:"nsec3_salt,omitempty"`
	NSEC3Iterations          int    `json:"nsec3_iterations"`
	SignatureValiditySeconds int    `json:"signature_validity_seconds"`
}

type SnapshotDNSSECKey struct {
	ID                  uint   `json:"id"`
	Role                string `json:"role"`
	Flags               uint16 `json:"flags"`
	Algorithm           uint8  `json:"algorithm"`
	PublicKey           string `json:"public_key"`
	EncryptedPrivateKey string `json:"encrypted_private_key"`
	KeyTag              uint16 `json:"key_tag"`
	Status              string `json:"status"`
}

type SnapshotRecord struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
}

type SnapshotRoute struct {
	ID             uint       `json:"id"`
	SiteName       string     `json:"site_name"`
	Domains        []string   `json:"domains"`
	ZoneID         uint       `json:"zone_id"`
	NodePool       string     `json:"node_pool"`
	RecordType     string     `json:"record_type"`
	TargetCount    int        `json:"target_count"`
	ScheduleMode   string     `json:"schedule_mode"`
	TTL            int        `json:"ttl"`
	GSLBEnabled    bool       `json:"gslb_enabled"`
	GSLBPolicy     GSLBPolicy `json:"gslb_policy"`
	CurrentTargets []string   `json:"current_targets"`
	TargetError    string     `json:"target_error,omitempty"`
	DDOSActive     bool       `json:"ddos_active,omitempty"`
	DDOSProvider   string     `json:"ddos_provider,omitempty"`
	DDOSTarget     string     `json:"ddos_target,omitempty"`
}

type SnapshotNode struct {
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

type SnapshotSchedulingState struct {
	RouteID         uint       `json:"route_id"`
	RecordType      string     `json:"record_type"`
	ScopeKey        string     `json:"scope_key"`
	SelectedTargets []string   `json:"selected_targets"`
	DesiredTargets  []string   `json:"desired_targets"`
	UnhealthyCount  int        `json:"unhealthy_count,omitempty"`
	RecoveryCount   int        `json:"recovery_count,omitempty"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`
}

type GSLBPoolPolicy struct {
	Name        string   `json:"name"`
	Weight      int      `json:"weight"`
	Countries   []string `json:"countries"`
	SourceCIDRs []string `json:"source_cidrs"`
	Operators   []string `json:"operators,omitempty"`
	ASNs        []uint32 `json:"asns,omitempty"`
	NodeIDs     []string `json:"node_ids,omitempty"`
	Enabled     bool     `json:"enabled"`
}

type GSLBSourceIPProvider struct {
	Provider string `json:"provider"`
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
}

type GSLBLoadThresholds struct {
	MaxOpenrestyConnections int64   `json:"max_openresty_connections"`
	MaxCPUPercent           float64 `json:"max_cpu_percent"`
	MaxMemoryPercent        float64 `json:"max_memory_percent"`
}

type GSLBDebounce struct {
	CooldownSeconds    int `json:"cooldown_seconds"`
	UnhealthyThreshold int `json:"unhealthy_threshold"`
	RecoveryThreshold  int `json:"recovery_threshold"`
}

type GSLBPolicy struct {
	Mode                   string               `json:"mode"`
	Strategy               string               `json:"strategy"`
	Pools                  []GSLBPoolPolicy     `json:"pools"`
	TargetCount            int                  `json:"target_count"`
	TTL                    int                  `json:"ttl"`
	SourceIP               GSLBSourceIPProvider `json:"source_ip"`
	SourcePoolFallbackMode string               `json:"source_pool_fallback_mode"`
	LoadThresholds         GSLBLoadThresholds   `json:"load_thresholds"`
	Debounce               GSLBDebounce         `json:"debounce"`
}

type SourceContext struct {
	IP       string
	Country  string
	Operator string
	ASN      uint32
	ScopeKey string
}
