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

	WorkerStatusOnline  = "online"
	WorkerStatusOffline = "offline"
)

type Snapshot struct {
	SnapshotVersion string          `json:"snapshot_version"`
	GeneratedAt     time.Time       `json:"generated_at"`
	Zones           []SnapshotZone  `json:"zones"`
	Routes          []SnapshotRoute `json:"routes"`
	Nodes           []SnapshotNode  `json:"nodes"`
}

type SnapshotZone struct {
	ID          uint             `json:"id"`
	Name        string           `json:"name"`
	SOAEmail    string           `json:"soa_email"`
	PrimaryNS   string           `json:"primary_ns"`
	NameServers []string         `json:"name_servers"`
	DefaultTTL  int              `json:"default_ttl"`
	Serial      uint64           `json:"serial"`
	Records     []SnapshotRecord `json:"records"`
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
}

type GSLBPoolPolicy struct {
	Name      string   `json:"name"`
	Weight    int      `json:"weight"`
	Countries []string `json:"countries"`
	Enabled   bool     `json:"enabled"`
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
	Mode           string               `json:"mode"`
	Strategy       string               `json:"strategy"`
	Pools          []GSLBPoolPolicy     `json:"pools"`
	TargetCount    int                  `json:"target_count"`
	TTL            int                  `json:"ttl"`
	SourceIP       GSLBSourceIPProvider `json:"source_ip"`
	LoadThresholds GSLBLoadThresholds   `json:"load_thresholds"`
	Debounce       GSLBDebounce         `json:"debounce"`
}

type SourceContext struct {
	IP       string
	Country  string
	ScopeKey string
}
