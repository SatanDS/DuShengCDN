package dnsworker

import (
	"errors"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerURL                string
	Token                    string
	ListenAddr               string
	SnapshotPath             string
	InstallDir               string
	UpdateScriptPath         string
	UpdateEnabled            bool
	GeoIPDatabasePath        string
	ASNDatabasePath          string
	OperatorCIDRDatabasePath string
	HeartbeatInterval        time.Duration
	RequestTimeout           time.Duration
	SnapshotMaxAge           time.Duration
	QueryRateLimit           int
	ResponseRateLimit        int
	UDPResponseSize          int
	ECSEnabled               bool
	ECSIPv4Prefix            int
	ECSIPv6Prefix            int
	Version                  string
}

func LoadConfig(args []string, version string) (*Config, error) {
	cfg := &Config{
		ServerURL:                strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_SERVER_URL")),
		Token:                    strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_TOKEN")),
		ListenAddr:               strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_LISTEN_ADDR")),
		SnapshotPath:             strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH")),
		InstallDir:               strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_INSTALL_DIR")),
		UpdateScriptPath:         strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_UPDATE_SCRIPT")),
		UpdateEnabled:            parseBoolEnv("DUSHENGCDN_DNS_WORKER_UPDATE_ENABLED", false),
		GeoIPDatabasePath:        strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH")),
		ASNDatabasePath:          strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH")),
		OperatorCIDRDatabasePath: strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH")),
		HeartbeatInterval:        parseDurationEnv("DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL", DefaultHeartbeatInterval),
		RequestTimeout:           parseDurationEnv("DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT", DefaultRequestTimeout),
		SnapshotMaxAge:           parseDurationEnv("DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE", DefaultSnapshotMaxAge),
		QueryRateLimit:           parseIntEnv("DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT", DefaultQueryRateLimit),
		ResponseRateLimit:        parseIntEnv("DUSHENGCDN_DNS_WORKER_RESPONSE_RATE_LIMIT", DefaultResponseRateLimit),
		UDPResponseSize:          parseIntEnv("DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE", DefaultUDPResponseSize),
		ECSEnabled:               parseBoolEnv("DUSHENGCDN_DNS_WORKER_ECS_ENABLED", true),
		ECSIPv4Prefix:            parseIntEnv("DUSHENGCDN_DNS_WORKER_ECS_IPV4_PREFIX", DefaultECSIPv4Prefix),
		ECSIPv6Prefix:            parseIntEnv("DUSHENGCDN_DNS_WORKER_ECS_IPV6_PREFIX", DefaultECSIPv6Prefix),
		Version:                  version,
	}
	fs := flag.NewFlagSet("dns-worker", flag.ContinueOnError)
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "DuShengCDN Server URL")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "DNS Worker token")
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "DNS UDP/TCP listen address")
	fs.StringVar(&cfg.SnapshotPath, "snapshot-path", cfg.SnapshotPath, "local snapshot cache path")
	fs.StringVar(&cfg.InstallDir, "install-dir", cfg.InstallDir, "DNS Worker install directory used by controlled self-update")
	fs.StringVar(&cfg.UpdateScriptPath, "update-script", cfg.UpdateScriptPath, "DNS Worker update script path")
	fs.BoolVar(&cfg.UpdateEnabled, "update-enabled", cfg.UpdateEnabled, "allow controlled DNS Worker self-update when requested by Server")
	fs.StringVar(&cfg.GeoIPDatabasePath, "geoip-database", cfg.GeoIPDatabasePath, "optional MaxMind-compatible country/ASN/ISP MMDB path")
	fs.StringVar(&cfg.ASNDatabasePath, "asn-database", cfg.ASNDatabasePath, "optional MaxMind-compatible ASN MMDB path")
	fs.StringVar(&cfg.OperatorCIDRDatabasePath, "operator-cidr-database", cfg.OperatorCIDRDatabasePath, "optional China operator CIDR list directory or file path")
	fs.DurationVar(&cfg.HeartbeatInterval, "heartbeat-interval", cfg.HeartbeatInterval, "heartbeat and snapshot pull interval")
	fs.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "Server request timeout")
	fs.DurationVar(&cfg.SnapshotMaxAge, "snapshot-max-age", cfg.SnapshotMaxAge, "maximum age for dynamic DNS answers")
	fs.IntVar(&cfg.QueryRateLimit, "query-rate-limit", cfg.QueryRateLimit, "maximum DNS queries per source IP per second, 0 disables rate limiting")
	fs.IntVar(&cfg.ResponseRateLimit, "response-rate-limit", cfg.ResponseRateLimit, "maximum abnormal DNS responses per source IP/qname/rcode per second, 0 disables response rate limiting")
	fs.IntVar(&cfg.UDPResponseSize, "udp-response-size", cfg.UDPResponseSize, "maximum UDP DNS response payload size")
	fs.BoolVar(&cfg.ECSEnabled, "ecs-enabled", cfg.ECSEnabled, "use EDNS Client Subnet for source classification")
	fs.IntVar(&cfg.ECSIPv4Prefix, "ecs-ipv4-prefix", cfg.ECSIPv4Prefix, "IPv4 ECS source prefix used for source classification")
	fs.IntVar(&cfg.ECSIPv6Prefix, "ecs-ipv6-prefix", cfg.ECSIPv6Prefix, "IPv6 ECS source prefix used for source classification")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	applyConfigDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if strings.TrimSpace(cfg.SnapshotPath) == "" {
		cfg.SnapshotPath = DefaultSnapshotPath
	}
	cfg.SnapshotPath = filepath.Clean(cfg.SnapshotPath)
	if strings.TrimSpace(cfg.InstallDir) == "" {
		cfg.InstallDir = filepath.Dir(cfg.SnapshotPath)
		if filepath.Base(cfg.InstallDir) == "data" {
			cfg.InstallDir = filepath.Dir(cfg.InstallDir)
		}
	}
	cfg.InstallDir = filepath.Clean(cfg.InstallDir)
	if strings.TrimSpace(cfg.UpdateScriptPath) == "" {
		cfg.UpdateScriptPath = filepath.Join(cfg.InstallDir, "update-dns-worker.sh")
	}
	cfg.UpdateScriptPath = filepath.Clean(cfg.UpdateScriptPath)
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.SnapshotMaxAge <= 0 {
		cfg.SnapshotMaxAge = DefaultSnapshotMaxAge
	}
	if cfg.QueryRateLimit < 0 {
		cfg.QueryRateLimit = 0
	}
	if cfg.ResponseRateLimit < 0 {
		cfg.ResponseRateLimit = 0
	}
	if cfg.UDPResponseSize <= 0 {
		cfg.UDPResponseSize = DefaultUDPResponseSize
	}
	if cfg.ECSIPv4Prefix < 0 {
		cfg.ECSIPv4Prefix = 0
	}
	if cfg.ECSIPv4Prefix > 32 {
		cfg.ECSIPv4Prefix = 32
	}
	if cfg.ECSIPv6Prefix < 0 {
		cfg.ECSIPv6Prefix = 0
	}
	if cfg.ECSIPv6Prefix > 128 {
		cfg.ECSIPv6Prefix = 128
	}
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = "dev"
	}
}

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return errors.New("server-url cannot be empty")
	}
	parsed, err := url.Parse(cfg.ServerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("server-url format is invalid")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("token cannot be empty")
	}
	if cfg.HeartbeatInterval < time.Second {
		return errors.New("heartbeat-interval must be at least 1s")
	}
	if cfg.RequestTimeout < time.Second {
		return errors.New("request-timeout must be at least 1s")
	}
	if cfg.SnapshotMaxAge < time.Second {
		return errors.New("snapshot-max-age must be at least 1s")
	}
	if cfg.QueryRateLimit < 0 {
		return errors.New("query-rate-limit cannot be negative")
	}
	if cfg.ResponseRateLimit < 0 {
		return errors.New("response-rate-limit cannot be negative")
	}
	if cfg.UDPResponseSize < 512 {
		return errors.New("udp-response-size must be at least 512")
	}
	if cfg.UDPResponseSize > 65535 {
		return errors.New("udp-response-size cannot exceed 65535")
	}
	if cfg.ECSIPv4Prefix < 0 || cfg.ECSIPv4Prefix > 32 {
		return errors.New("ecs-ipv4-prefix must be between 0 and 32")
	}
	if cfg.ECSIPv6Prefix < 0 || cfg.ECSIPv6Prefix > 128 {
		return errors.New("ecs-ipv6-prefix must be between 0 and 128")
	}
	return nil
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func parseIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseBoolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
