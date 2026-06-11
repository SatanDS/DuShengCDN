package openresty

import (
	"context"
	"net"
)

const (
	CertDirPlaceholder             = "__DUSHENGCDN_CERT_DIR__"
	RouteConfigPlaceholder         = "__DUSHENGCDN_ROUTE_CONFIG__"
	AccessLogPlaceholder           = "__DUSHENGCDN_ACCESS_LOG__"
	LuaDirPlaceholder              = "__DUSHENGCDN_LUA_DIR__"
	ObservabilityListenPlaceholder = "__DUSHENGCDN_OBSERVABILITY_LISTEN__"
	ObservabilityPortPlaceholder   = "__DUSHENGCDN_OBSERVABILITY_PORT__"
	PowStaticDirPlaceholder        = "__DUSHENGCDN_POW_STATIC_DIR__"
	ResolverDirectivePlaceholder   = "__DUSHENGCDN_RESOLVER_DIRECTIVE__"
)

const (
	CachePolicySuffix               = "suffix"
	CachePolicyPathPrefix           = "path_prefix"
	CachePolicyPathContains         = "path_contains"
	CachePolicyPathContainsAll      = "path_contains_all"
	CachePolicyPathExact            = "path_exact"
	ProxyBufferingModeOff           = "off"
	RegionModeAllow                 = "allow"
	RegionModeBlock                 = "block"
	WAFModeBlock                    = "block"
	CCModeBlock                     = "block"
	CCModePoW                       = "pow"
	OriginResolveModeRuntimeDNS     = "runtime_dns"
	OriginResolveModePublishResolve = "publish_resolve"
	OriginResolveModeStaticIP       = "static_ip"
	OriginResolveModeOriginGroup    = "origin_group"
	RouteRuleMatchTypePrefix        = "prefix"
	RouteRuleMatchTypeExact         = "exact"
	RouteRuleMatchTypeRegex         = "regex"
	RouteRuleMatchTypeDefault       = "default"
)

type SupportFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Redacted bool   `json:"redacted,omitempty"`
}

type ConfigSnapshot struct {
	WorkerProcesses          string `json:"worker_processes"`
	WorkerConnections        int    `json:"worker_connections"`
	WorkerRlimitNofile       int    `json:"worker_rlimit_nofile"`
	EventsUse                string `json:"events_use,omitempty"`
	EventsMultiAcceptEnabled bool   `json:"events_multi_accept_enabled"`
	KeepaliveTimeout         int    `json:"keepalive_timeout"`
	KeepaliveRequests        int    `json:"keepalive_requests"`
	ClientHeaderTimeout      int    `json:"client_header_timeout"`
	ClientBodyTimeout        int    `json:"client_body_timeout"`
	ClientMaxBodySize        string `json:"client_max_body_size"`
	LargeClientHeaderBuffers string `json:"large_client_header_buffers"`
	SendTimeout              int    `json:"send_timeout"`
	ProxyConnectTimeout      int    `json:"proxy_connect_timeout"`
	ProxySendTimeout         int    `json:"proxy_send_timeout"`
	ProxyReadTimeout         int    `json:"proxy_read_timeout"`
	WebsocketEnabled         bool   `json:"websocket_enabled"`
	ProxyRequestBuffering    bool   `json:"proxy_request_buffering"`
	ProxyBufferingEnabled    bool   `json:"proxy_buffering_enabled"`
	ProxyBuffers             string `json:"proxy_buffers"`
	ProxyBufferSize          string `json:"proxy_buffer_size"`
	ProxyBusyBuffersSize     string `json:"proxy_busy_buffers_size"`
	GzipEnabled              bool   `json:"gzip_enabled"`
	GzipMinLength            int    `json:"gzip_min_length"`
	GzipCompLevel            int    `json:"gzip_comp_level"`
	Resolvers                string `json:"resolvers,omitempty"`
	CacheEnabled             bool   `json:"cache_enabled"`
	CachePath                string `json:"cache_path,omitempty"`
	CacheLevels              string `json:"cache_levels"`
	CacheInactive            string `json:"cache_inactive"`
	CacheMaxSize             string `json:"cache_max_size"`
	CacheKeyTemplate         string `json:"cache_key_template"`
	CacheLockEnabled         bool   `json:"cache_lock_enabled"`
	CacheLockTimeout         string `json:"cache_lock_timeout"`
	CacheUseStale            string `json:"cache_use_stale"`
}

type CustomHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TLSCertificate struct {
	ID      uint
	CertPEM string
	KeyPEM  string
}

type Route struct {
	ID                         uint
	SiteName                   string
	Domain                     string
	Domains                    []string
	OriginURL                  string
	OriginHost                 string
	OriginHostHeader           string
	OriginSNI                  string
	OriginTLSVerify            *bool
	OriginCABundle             string
	OriginResolveMode          string
	Upstreams                  []string
	EnableHTTPS                bool
	CertID                     *uint
	CertIDs                    []uint
	DomainCertIDs              []uint
	RedirectHTTP               bool
	LimitConnPerServer         int
	LimitConnPerIP             int
	LimitRate                  string
	ProxyBufferingMode         string
	CacheEnabled               bool
	CachePolicy                string
	CacheRules                 []string
	CustomHeaders              []CustomHeader
	PoWEnabled                 bool
	WAFEnabled                 bool
	WAFMode                    string
	CCEnabled                  bool
	CCMode                     string
	BasicAuthEnabled           bool
	BasicAuthUsername          string
	BasicAuthPasswordHash      string
	RegionRestrictionEnabled   bool
	RegionRestrictionMode      string
	RegionRestrictionCountries []string
	Certificates               []TLSCertificate
	Rules                      []RouteRule
}

type RouteRule struct {
	ID                         uint
	MatchType                  string
	Path                       string
	Priority                   int
	Enabled                    bool
	OriginURL                  string
	OriginHost                 string
	OriginHostHeader           string
	OriginSNI                  string
	OriginTLSVerify            *bool
	OriginCABundle             string
	OriginResolveMode          string
	Upstreams                  []string
	Limit                      RouteRuleLimit
	LimitConnPerServer         *int
	LimitConnPerIP             *int
	LimitRate                  string
	ProxyBuffering             RouteRuleProxyBuffering
	ProxyBufferingMode         string
	Cache                      RouteRuleCache
	CacheEnabled               *bool
	CachePolicy                string
	CacheRules                 []string
	CustomHeaders              []CustomHeader
	PoW                        RouteRulePoW
	PoWEnabled                 *bool
	WAF                        RouteRuleWAF
	WAFEnabled                 *bool
	WAFMode                    string
	CC                         RouteRuleCC
	CCEnabled                  *bool
	CCMode                     string
	BasicAuth                  RouteRuleBasicAuth
	BasicAuthEnabled           *bool
	BasicAuthUsername          string
	BasicAuthPasswordHash      string
	Region                     RouteRuleRegion
	RegionRestrictionEnabled   *bool
	RegionRestrictionMode      string
	RegionRestrictionCountries []string
}

type RouteRuleLimit struct {
	LimitConnPerServer int
	LimitConnPerIP     int
	LimitRate          string
}

type RouteRuleProxyBuffering struct {
	Mode string
}

type RouteRuleCache struct {
	Enabled *bool
	Policy  string
	Rules   []string
}

type RouteRuleBasicAuth struct {
	Enabled      *bool
	Username     string
	PasswordHash string
}

type RouteRuleWAF struct {
	Enabled *bool
	Mode    string
}

type RouteRuleCC struct {
	Enabled *bool
	Mode    string
}

type RouteRulePoW struct {
	Enabled *bool
}

type RouteRuleRegion struct {
	Enabled   *bool
	Mode      string
	Countries []string
}

type AccessRoute struct {
	Domain                     string
	Domains                    []string
	PoWEnabled                 bool
	PoWConfigJSON              string
	DefaultPoWConfig           any
	WAFEnabled                 bool
	WAFMode                    string
	WAFConfig                  any
	CCEnabled                  bool
	CCMode                     string
	CCConfig                   any
	RegionRestrictionEnabled   bool
	RegionRestrictionMode      string
	RegionRestrictionCountries []string
}

type RenderOptions struct {
	LookupIPAddr func(context.Context, string) ([]net.IPAddr, error)
}

type routeCacheConfig struct {
	Enabled bool
	Policy  string
	Rules   []string
}

type routeLimitConfig struct {
	LimitConnPerServer int
	LimitConnPerIP     int
	LimitRate          string
}

type routeProxyBufferingConfig struct {
	Mode string
}

type routeRegionRestrictionConfig struct {
	Enabled   bool
	Mode      string
	Countries []string
}

type routeWAFConfig struct {
	Enabled bool
	Mode    string
}

type routeCCConfig struct {
	Enabled bool
	Mode    string
}

type routeUpstreamConfig struct {
	Name              string
	Scheme            string
	ProxyPassURI      string
	RuntimeProxyPass  string
	Servers           []string
	UsesNamedUpstream bool
	UsesRuntimeDNS    bool
}

type routeOriginConfig struct {
	URL          string
	HostHeader   string
	SNI          string
	TLSVerify    bool
	CABundlePath string
}
