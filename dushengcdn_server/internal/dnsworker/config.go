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
	GeoIPDatabasePath        string
	ASNDatabasePath          string
	OperatorCIDRDatabasePath string
	HeartbeatInterval        time.Duration
	RequestTimeout           time.Duration
	SnapshotMaxAge           time.Duration
	QueryRateLimit           int
	UDPResponseSize          int
	Version                  string
}

func LoadConfig(args []string, version string) (*Config, error) {
	cfg := &Config{
		ServerURL:                strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_SERVER_URL")),
		Token:                    strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_TOKEN")),
		ListenAddr:               strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_LISTEN_ADDR")),
		SnapshotPath:             strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH")),
		GeoIPDatabasePath:        strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH")),
		ASNDatabasePath:          strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH")),
		OperatorCIDRDatabasePath: strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH")),
		HeartbeatInterval:        parseDurationEnv("DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL", DefaultHeartbeatInterval),
		RequestTimeout:           parseDurationEnv("DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT", DefaultRequestTimeout),
		SnapshotMaxAge:           parseDurationEnv("DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE", DefaultSnapshotMaxAge),
		QueryRateLimit:           parseIntEnv("DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT", DefaultQueryRateLimit),
		UDPResponseSize:          parseIntEnv("DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE", DefaultUDPResponseSize),
		Version:                  version,
	}
	fs := flag.NewFlagSet("dns-worker", flag.ContinueOnError)
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "DuShengCDN Server URL")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "DNS Worker token")
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "DNS UDP/TCP listen address")
	fs.StringVar(&cfg.SnapshotPath, "snapshot-path", cfg.SnapshotPath, "local snapshot cache path")
	fs.StringVar(&cfg.GeoIPDatabasePath, "geoip-database", cfg.GeoIPDatabasePath, "optional MaxMind-compatible country/ASN/ISP MMDB path")
	fs.StringVar(&cfg.ASNDatabasePath, "asn-database", cfg.ASNDatabasePath, "optional MaxMind-compatible ASN MMDB path")
	fs.StringVar(&cfg.OperatorCIDRDatabasePath, "operator-cidr-database", cfg.OperatorCIDRDatabasePath, "optional China operator CIDR list directory or file path")
	fs.DurationVar(&cfg.HeartbeatInterval, "heartbeat-interval", cfg.HeartbeatInterval, "heartbeat and snapshot pull interval")
	fs.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "Server request timeout")
	fs.DurationVar(&cfg.SnapshotMaxAge, "snapshot-max-age", cfg.SnapshotMaxAge, "maximum age for dynamic DNS answers")
	fs.IntVar(&cfg.QueryRateLimit, "query-rate-limit", cfg.QueryRateLimit, "maximum DNS queries per source IP per second, 0 disables rate limiting")
	fs.IntVar(&cfg.UDPResponseSize, "udp-response-size", cfg.UDPResponseSize, "maximum UDP DNS response payload size")
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
	if cfg.UDPResponseSize <= 0 {
		cfg.UDPResponseSize = DefaultUDPResponseSize
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
	if cfg.UDPResponseSize < 512 {
		return errors.New("udp-response-size must be at least 512")
	}
	if cfg.UDPResponseSize > 65535 {
		return errors.New("udp-response-size cannot exceed 65535")
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
