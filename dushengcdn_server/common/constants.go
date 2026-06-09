package common

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

var StartTime = time.Now().Unix() // unit: second
var Version = "dev"               // release builds inject the tag version via ldflags
var ReleaseSignaturePublicKey = ""
var SystemName = "DuShengCDN"
var ServerAddress = "http://localhost:3000"
var Footer = ""
var HomePageLink = ""

// Any options with "Secret", "Token" in its key won't be return by GetOptions

var SessionSecret = uuid.New().String()
var SessionCookieSecureConfigured = false
var SessionCookieSecure = false
var SessionCookieSameSite = ""
var InitialRootPassword = ""
var TrustedProxies = ""
var JSONBodyMaxBytes int64 = 2 * 1024 * 1024
var SQLitePath = "dushengcdn.db"
var SQLDSN = ""
var DatabaseMaxOpenConns = 30
var DatabaseMaxIdleConns = 10
var DatabaseConnMaxLifetime = 30 * time.Minute

var OptionMap map[string]string
var OptionMapRWMutex sync.RWMutex

func GetOptionValue(key string) string {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	if OptionMap == nil {
		return ""
	}
	return OptionMap[key]
}

func OptionMapSnapshot() map[string]string {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	snapshot := make(map[string]string, len(OptionMap))
	for key, value := range OptionMap {
		snapshot[key] = value
	}
	return snapshot
}

var ItemsPerPage = 10

var PasswordLoginEnabled = true
var PasswordRegisterEnabled = true
var EmailVerificationEnabled = false
var GitHubOAuthEnabled = false
var WeChatAuthEnabled = false
var TurnstileCheckEnabled = false
var RegisterEnabled = false

var SMTPServer = ""
var SMTPPort = 587
var SMTPAccount = ""
var SMTPToken = ""

var GitHubClientId = ""
var GitHubClientSecret = ""

var WeChatServerAddress = ""
var WeChatServerToken = ""
var WeChatAccountQRCodeImageURL = ""

var TurnstileSiteKey = ""
var TurnstileSecretKey = ""
var AgentToken = ""
var AgentLegacyGlobalTokenEnabled = false
var AgentDiscoveryToken = ""
var NodeOfflineThreshold = 2 * time.Minute
var RedisRequired = false
var CommercialLicenseRequired = false
var CommercialLicensePublicKeys = ""
var CommercialLicenseAllowUnsigned = false
var CommercialLicenseIssuerPrivateKey = ""
var CommercialLicenseIssuerPrivateKeyFile = ""
var CommercialLicenseActivationURL = "https://www.satandu.com"
var CommercialLicenseOnlineActivationRequired = false
var CommercialLicenseActivationServerEnabled = false
var CommercialLicenseLeaseDuration = 72 * time.Hour
var CommercialLicenseLeaseRenewBefore = 6 * time.Hour
var CommercialBuildMode = ""
var CommercialBuildWatermark = ""
var ServerAutoUpgradeEnabled = false
var ServerUpdateRepo = "SatanDS/SatanDS-DuShengCDN-releases"
var GitHubReleaseToken = ""
var DNSSourceDatabaseMirrorPath = ""
var DNSSourceDatabaseCountryURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"
var DNSSourceDatabaseASNURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-ASN.mmdb"
var DNSSourceDatabaseOperatorCIDRBaseURL = "https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists"
var DNSSourceDatabaseOperatorCIDRFiles = "chinanet.txt chinanet6.txt cmcc.txt cmcc6.txt unicom.txt unicom6.txt cernet.txt cernet6.txt cstnet.txt cstnet6.txt drpeng.txt drpeng6.txt googlecn.txt googlecn6.txt"

// V3 operational settings (hot-reloadable via Option table)
var AgentHeartbeatInterval = 10000 // milliseconds
var AgentWebsocketUpgradeEnabled = true
var AgentUpdateRepo = "SatanDS/SatanDS-DuShengCDN-releases"
var GeoIPProvider = "ipinfo"
var DatabaseAutoCleanupEnabled = false
var DatabaseAutoCleanupRetentionDays = 30
var CloudflareDDoSRequestThreshold int64 = 20000
var CloudflareDDoSErrorRateThreshold = 30.0
var AuthoritativeDNSEnabled = false
var AuthoritativeDNSListenAddr = ":53"
var AuthoritativeDNSDefaultTTL = 30
var AuthoritativeDNSSnapshotMaxAge = 300
var AuthoritativeDNSWorkerQueryRateLimit = 200
var AuthoritativeDNSWorkerResponseRateLimit = 50
var AuthoritativeDNSWorkerUDPResponseSize = 1232
var AuthoritativeDNSWorkerECSEnabled = true
var AuthoritativeDNSWorkerECSIPv4Prefix = 24
var AuthoritativeDNSWorkerECSIPv6Prefix = 56
var GSLBMetricFreshnessSeconds = 120
var GSLBProbeSchedulingEnabled = false

// V5 OpenResty performance settings (hot-reloadable via Option table)
var OpenRestyWorkerProcesses = "auto"
var OpenRestyWorkerConnections = 4096
var OpenRestyWorkerRlimitNofile = 65535
var OpenRestyEventsUse = "epoll"
var OpenRestyEventsMultiAcceptEnabled = true
var OpenRestyKeepaliveTimeout = 20
var OpenRestyKeepaliveRequests = 1000
var OpenRestyClientHeaderTimeout = 15
var OpenRestyClientBodyTimeout = 15
var OpenRestyClientMaxBodySize = "64m"
var OpenRestyLargeClientHeaderBuffers = "4 16k"
var OpenRestySendTimeout = 30
var OpenRestyResolvers = ""
var OpenRestyProxyConnectTimeout = 3
var OpenRestyProxySendTimeout = 60
var OpenRestyProxyReadTimeout = 60
var OpenRestyWebsocketEnabled = true
var OpenRestyProxyRequestBufferingEnabled = false
var OpenRestyProxyBufferingEnabled = true
var OpenRestyProxyBuffers = "16 16k"
var OpenRestyProxyBufferSize = "8k"
var OpenRestyProxyBusyBuffersSize = "64k"
var OpenRestyGzipEnabled = true
var OpenRestyGzipMinLength = 1024
var OpenRestyGzipCompLevel = 5
var OpenRestyCacheEnabled = false
var OpenRestyCachePath = ""
var OpenRestyCacheLevels = "1:2"
var OpenRestyCacheInactive = "30m"
var OpenRestyCacheMaxSize = "1g"
var OpenRestyCacheKeyTemplate = "$scheme$host$request_uri"
var OpenRestyCacheLockEnabled = true
var OpenRestyCacheLockTimeout = "5s"
var OpenRestyCacheUseStale = "error timeout updating http_500 http_502 http_503 http_504"
var OpenRestyMainConfigTemplate = `# This file is generated by DuShengCDN. Do not edit manually.
worker_processes {{OpenRestyWorkerProcesses}};
worker_rlimit_nofile {{OpenRestyWorkerRlimitNofile}};
pid logs/nginx.pid;

events {
    worker_connections {{OpenRestyWorkerConnections}};
{{OpenRestyEventsUseDirective}}{{OpenRestyEventsMultiAcceptDirective}}}

http {
    include       mime.types;
    default_type  application/octet-stream;
{{OpenRestyConnectionUpgradeMap}}{{OpenRestyDefaultServerBlock}}    log_format dushengcdn_json escape=json '{"ts":"$time_iso8601","host":"$host","path":"$request_uri","remote_addr":"$remote_addr","status":$status,"reason":"$dushengcdn_request_reason","request_time":$request_time,"bytes_sent":$body_bytes_sent,"request_length":$request_length,"upstream_response_length":"$upstream_response_length","cache_status":"$upstream_cache_status","upstream_status":"$upstream_status","upstream_response_time":"$upstream_response_time"}';
    access_log {{OpenRestyAccessLogPath}} dushengcdn_json;
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout {{OpenRestyKeepaliveTimeout}};
    keepalive_requests {{OpenRestyKeepaliveRequests}};
    client_header_timeout {{OpenRestyClientHeaderTimeout}};
    client_body_timeout {{OpenRestyClientBodyTimeout}};
    client_max_body_size {{OpenRestyClientMaxBodySize}};
    large_client_header_buffers {{OpenRestyLargeClientHeaderBuffers}};
    send_timeout {{OpenRestySendTimeout}};
    proxy_connect_timeout {{OpenRestyProxyConnectTimeout}};
    proxy_send_timeout {{OpenRestyProxySendTimeout}};
    proxy_read_timeout {{OpenRestyProxyReadTimeout}};
    proxy_request_buffering {{OpenRestyProxyRequestBuffering}};
    proxy_buffering {{OpenRestyProxyBuffering}};
    proxy_buffers {{OpenRestyProxyBuffers}};
    proxy_buffer_size {{OpenRestyProxyBufferSize}};
    proxy_busy_buffers_size {{OpenRestyProxyBusyBuffersSize}};
    gzip {{OpenRestyGzip}};
    gzip_min_length {{OpenRestyGzipMinLength}};
    gzip_comp_level {{OpenRestyGzipCompLevel}};
{{OpenRestyResolverDirective}}{{OpenRestyCacheBlock}}    include {{OpenRestyRouteConfigInclude}};
}
`

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

var (
	FileUploadPermission    = RoleGuestUser
	FileDownloadPermission  = RoleGuestUser
	ImageUploadPermission   = RoleGuestUser
	ImageDownloadPermission = RoleGuestUser
)

// All duration's unit is seconds
// Shouldn't larger then RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitNum            = 300
	GlobalApiRateLimitDuration int64 = 3 * 60

	GlobalWebRateLimitNum            = 300
	GlobalWebRateLimitDuration int64 = 3 * 60

	UploadRateLimitNum            = 50
	UploadRateLimitDuration int64 = 60

	DownloadRateLimitNum            = 50
	DownloadRateLimitDuration int64 = 60

	CriticalRateLimitNum            = 100
	CriticalRateLimitDuration int64 = 20 * 60

	DNSWorkerAPIRateLimitNum            = 600
	DNSWorkerAPIRateLimitDuration int64 = 60
)

var RateLimitKeyExpirationDuration = 20 * time.Minute

const (
	UserStatusEnabled  = 1 // don't use 0, 0 is the default value!
	UserStatusDisabled = 2 // also don't use 0
)
