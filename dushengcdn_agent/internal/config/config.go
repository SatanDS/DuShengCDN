package config

import (
	"dushengcdn-agent/internal/fileutil"
	"dushengcdn-agent/internal/security"
	"dushengcdn/utils/geoip/iputil"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMainConfigRelativePath          = "etc/nginx/nginx.conf"
	defaultRouteConfigRelativePath         = "etc/nginx/conf.d/dushengcdn_routes.conf"
	defaultCertDirRelativePath             = "etc/nginx/certs"
	defaultLuaDirRelativePath              = "etc/nginx/lua"
	defaultRuntimeConfigDirRelativePath    = "etc/dushengcdn"
	defaultAccessLogRelativePath           = "var/log/dushengcdn/access.log"
	defaultStateRelativePath               = "var/lib/dushengcdn/agent-state.json"
	defaultObservabilityBufferRelativePath = "var/lib/dushengcdn/observability-buffer.json"
	defaultGeoIPDatabaseRelativePath       = "var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb"
	defaultGeoIPDatabaseURL                = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"
	defaultOpenRestyObservabilityPort      = 18081
	defaultObservabilityReplayMinutes      = 15
	defaultGeoIPUpdateInterval             = 24 * time.Hour
)

type Config struct {
	ServerURL                  string              `json:"server_url"`
	AgentToken                 string              `json:"agent_token"`
	DiscoveryToken             string              `json:"discovery_token"`
	NodeName                   string              `json:"node_name"`
	NodeIP                     string              `json:"node_ip"`
	AgentVersion               string              `json:"-"`
	NginxVersion               string              `json:"-"`
	OpenrestyPath              string              `json:"openresty_path"`
	OpenrestyResolvers         []string            `json:"openresty_resolvers,omitempty"`
	OpenrestyContainerName     string              `json:"openresty_container_name,omitempty"`
	OpenrestyDockerImage       string              `json:"openresty_docker_image,omitempty"`
	DockerBinary               string              `json:"docker_binary,omitempty"`
	DataDir                    string              `json:"data_dir"`
	MainConfigPath             string              `json:"main_config_path"`
	RouteConfigPath            string              `json:"route_config_path"`
	AccessLogPath              string              `json:"access_log_path"`
	CertDir                    string              `json:"cert_dir"`
	OpenrestyCertDir           string              `json:"openresty_cert_dir"`
	LuaDir                     string              `json:"lua_dir"`
	OpenrestyLuaDir            string              `json:"openresty_lua_dir"`
	RuntimeConfigDir           string              `json:"runtime_config_dir"`
	OpenrestyObservabilityPort int                 `json:"openresty_observability_port"`
	ObservabilityBufferPath    string              `json:"observability_buffer_path"`
	ObservabilityReplayMinutes int                 `json:"observability_replay_minutes"`
	GeoIPDatabaseURL           string              `json:"geoip_database_url"`
	GeoIPDatabasePath          string              `json:"geoip_database_path"`
	OpenrestyGeoIPDatabasePath string              `json:"openresty_geoip_database_path"`
	GeoIPUpdateInterval        MillisecondDuration `json:"geoip_update_interval"`
	GeoIPLookupAPIURL          string              `json:"geoip_lookup_api_url,omitempty"`
	GeoIPLookupAPIToken        string              `json:"geoip_lookup_api_token,omitempty"`
	GeoIPLookupAPITokenFile    string              `json:"geoip_lookup_api_token_file,omitempty"`
	GeoIPLookupAPITimeout      MillisecondDuration `json:"geoip_lookup_api_timeout,omitempty"`
	StatePath                  string              `json:"state_path"`
	HeartbeatInterval          MillisecondDuration `json:"heartbeat_interval"`
	RequestTimeout             MillisecondDuration `json:"request_timeout"`
	configPath                 string
}

type configFile struct {
	ServerURL                  string              `json:"server_url"`
	AgentToken                 string              `json:"agent_token"`
	DiscoveryToken             string              `json:"discovery_token"`
	NodeName                   string              `json:"node_name"`
	NodeIP                     string              `json:"node_ip"`
	OpenrestyPath              string              `json:"openresty_path"`
	OpenrestyResolvers         []string            `json:"openresty_resolvers"`
	OpenrestyContainerName     string              `json:"openresty_container_name"`
	OpenrestyDockerImage       string              `json:"openresty_docker_image"`
	DockerBinary               string              `json:"docker_binary"`
	DataDir                    string              `json:"data_dir"`
	MainConfigPath             string              `json:"main_config_path"`
	RouteConfigPath            string              `json:"route_config_path"`
	AccessLogPath              string              `json:"access_log_path"`
	CertDir                    string              `json:"cert_dir"`
	OpenrestyCertDir           string              `json:"openresty_cert_dir"`
	LuaDir                     string              `json:"lua_dir"`
	OpenrestyLuaDir            string              `json:"openresty_lua_dir"`
	RuntimeConfigDir           string              `json:"runtime_config_dir"`
	OpenrestyObservabilityPort int                 `json:"openresty_observability_port"`
	ObservabilityBufferPath    string              `json:"observability_buffer_path"`
	ObservabilityReplayMinutes int                 `json:"observability_replay_minutes"`
	GeoIPDatabaseURL           string              `json:"geoip_database_url"`
	GeoIPDatabasePath          string              `json:"geoip_database_path"`
	OpenrestyGeoIPDatabasePath string              `json:"openresty_geoip_database_path"`
	GeoIPUpdateInterval        MillisecondDuration `json:"geoip_update_interval"`
	GeoIPLookupAPIURL          string              `json:"geoip_lookup_api_url"`
	GeoIPLookupAPIToken        string              `json:"geoip_lookup_api_token"`
	GeoIPLookupAPITokenFile    string              `json:"geoip_lookup_api_token_file"`
	GeoIPLookupAPITimeout      MillisecondDuration `json:"geoip_lookup_api_timeout"`
	StatePath                  string              `json:"state_path"`
	HeartbeatInterval          MillisecondDuration `json:"heartbeat_interval"`
	RequestTimeout             MillisecondDuration `json:"request_timeout"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		_ = tightenConfigFilePermissions(path)
	}
	file := &configFile{}
	if err == nil {
		if err = json.Unmarshal(data, file); err != nil {
			return nil, err
		}
		if strings.TrimSpace(file.GeoIPLookupAPIToken) != "" && strings.TrimSpace(file.GeoIPLookupAPITokenFile) != "" {
			return nil, errors.New("use only one of geoip_lookup_api_token and geoip_lookup_api_token_file")
		}
	}
	if err != nil && !hasEnvConfig() {
		return nil, err
	}
	cfg := &Config{
		ServerURL:                  file.ServerURL,
		AgentToken:                 file.AgentToken,
		DiscoveryToken:             file.DiscoveryToken,
		NodeName:                   file.NodeName,
		NodeIP:                     file.NodeIP,
		OpenrestyPath:              file.OpenrestyPath,
		OpenrestyResolvers:         append([]string{}, file.OpenrestyResolvers...),
		OpenrestyContainerName:     file.OpenrestyContainerName,
		OpenrestyDockerImage:       file.OpenrestyDockerImage,
		DockerBinary:               file.DockerBinary,
		DataDir:                    file.DataDir,
		MainConfigPath:             file.MainConfigPath,
		RouteConfigPath:            file.RouteConfigPath,
		AccessLogPath:              file.AccessLogPath,
		CertDir:                    file.CertDir,
		OpenrestyCertDir:           file.OpenrestyCertDir,
		LuaDir:                     file.LuaDir,
		OpenrestyLuaDir:            file.OpenrestyLuaDir,
		RuntimeConfigDir:           file.RuntimeConfigDir,
		OpenrestyObservabilityPort: file.OpenrestyObservabilityPort,
		ObservabilityBufferPath:    file.ObservabilityBufferPath,
		ObservabilityReplayMinutes: file.ObservabilityReplayMinutes,
		GeoIPDatabaseURL:           file.GeoIPDatabaseURL,
		GeoIPDatabasePath:          file.GeoIPDatabasePath,
		OpenrestyGeoIPDatabasePath: file.OpenrestyGeoIPDatabasePath,
		GeoIPUpdateInterval:        file.GeoIPUpdateInterval,
		GeoIPLookupAPIURL:          file.GeoIPLookupAPIURL,
		GeoIPLookupAPIToken:        file.GeoIPLookupAPIToken,
		GeoIPLookupAPITokenFile:    file.GeoIPLookupAPITokenFile,
		GeoIPLookupAPITimeout:      file.GeoIPLookupAPITimeout,
		StatePath:                  file.StatePath,
		HeartbeatInterval:          file.HeartbeatInterval,
		RequestTimeout:             file.RequestTimeout,
	}
	cfg.configPath = path
	if err = applyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	applyDefaults(cfg, filepath.Dir(path))
	if err = validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config, baseDir string) {
	baseDir = filepath.Clean(baseDir)
	cfg.AgentVersion = AgentVersion
	cfg.OpenrestyResolvers = normalizeResolverList(cfg.OpenrestyResolvers)
	if cfg.OpenrestyPath == "" {
		cfg.OpenrestyPath = "openresty"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(baseDir, "data")
	}
	if cfg.NodeName == "" {
		cfg.NodeName = detectHostname()
	}
	if cfg.NodeIP == "" {
		cfg.NodeIP = detectNodeIP()
	}
	if cfg.MainConfigPath == "" {
		cfg.MainConfigPath = joinManagedPath(cfg.DataDir, defaultMainConfigRelativePath)
	}
	if cfg.RouteConfigPath == "" {
		cfg.RouteConfigPath = joinManagedPath(cfg.DataDir, defaultRouteConfigRelativePath)
	}
	if cfg.AccessLogPath == "" {
		cfg.AccessLogPath = joinManagedPath(cfg.DataDir, defaultAccessLogRelativePath)
	}
	if cfg.StatePath == "" {
		cfg.StatePath = joinManagedPath(cfg.DataDir, defaultStateRelativePath)
	}
	if cfg.CertDir == "" {
		cfg.CertDir = joinManagedPath(cfg.DataDir, defaultCertDirRelativePath)
	}
	if cfg.OpenrestyCertDir == "" {
		cfg.OpenrestyCertDir = cfg.CertDir
	}
	if cfg.LuaDir == "" {
		cfg.LuaDir = joinManagedPath(cfg.DataDir, defaultLuaDirRelativePath)
	}
	if cfg.OpenrestyLuaDir == "" {
		cfg.OpenrestyLuaDir = cfg.LuaDir
	}
	if cfg.RuntimeConfigDir == "" {
		cfg.RuntimeConfigDir = joinManagedPath(cfg.DataDir, defaultRuntimeConfigDirRelativePath)
	}
	if cfg.OpenrestyObservabilityPort <= 0 {
		cfg.OpenrestyObservabilityPort = defaultOpenRestyObservabilityPort
	}
	if cfg.ObservabilityBufferPath == "" {
		cfg.ObservabilityBufferPath = joinManagedPath(cfg.DataDir, defaultObservabilityBufferRelativePath)
	}
	if cfg.ObservabilityReplayMinutes <= 0 {
		cfg.ObservabilityReplayMinutes = defaultObservabilityReplayMinutes
	}
	if cfg.GeoIPDatabaseURL == "" {
		cfg.GeoIPDatabaseURL = defaultGeoIPDatabaseURL
	}
	if cfg.GeoIPDatabasePath == "" {
		cfg.GeoIPDatabasePath = joinManagedPath(cfg.DataDir, defaultGeoIPDatabaseRelativePath)
	}
	if cfg.OpenrestyGeoIPDatabasePath == "" {
		cfg.OpenrestyGeoIPDatabasePath = cfg.GeoIPDatabasePath
	}
	if cfg.GeoIPUpdateInterval <= 0 {
		cfg.GeoIPUpdateInterval = MillisecondDuration(defaultGeoIPUpdateInterval)
	}
	if cfg.GeoIPLookupAPITimeout <= 0 {
		cfg.GeoIPLookupAPITimeout = MillisecondDuration(250 * time.Millisecond)
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = MillisecondDuration(10 * time.Second)
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = MillisecondDuration(10 * time.Second)
	}
	normalizeManagedPaths(cfg)
}

func normalizeManagedPaths(cfg *Config) {
	if cfg == nil {
		return
	}
	if usesSlashPath(cfg.DataDir) {
		cfg.DataDir = filepath.ToSlash(cfg.DataDir)
	}
	if usesSlashPath(cfg.MainConfigPath) {
		cfg.MainConfigPath = filepath.ToSlash(cfg.MainConfigPath)
	}
	if usesSlashPath(cfg.RouteConfigPath) {
		cfg.RouteConfigPath = filepath.ToSlash(cfg.RouteConfigPath)
	}
	if usesSlashPath(cfg.AccessLogPath) {
		cfg.AccessLogPath = filepath.ToSlash(cfg.AccessLogPath)
	}
	if usesSlashPath(cfg.CertDir) {
		cfg.CertDir = filepath.ToSlash(cfg.CertDir)
	}
	if usesSlashPath(cfg.OpenrestyCertDir) {
		cfg.OpenrestyCertDir = filepath.ToSlash(cfg.OpenrestyCertDir)
	}
	if usesSlashPath(cfg.LuaDir) {
		cfg.LuaDir = filepath.ToSlash(cfg.LuaDir)
	}
	if usesSlashPath(cfg.OpenrestyLuaDir) {
		cfg.OpenrestyLuaDir = filepath.ToSlash(cfg.OpenrestyLuaDir)
	}
	if usesSlashPath(cfg.RuntimeConfigDir) {
		cfg.RuntimeConfigDir = filepath.ToSlash(cfg.RuntimeConfigDir)
	}
	if usesSlashPath(cfg.StatePath) {
		cfg.StatePath = filepath.ToSlash(cfg.StatePath)
	}
	if usesSlashPath(cfg.ObservabilityBufferPath) {
		cfg.ObservabilityBufferPath = filepath.ToSlash(cfg.ObservabilityBufferPath)
	}
	if usesSlashPath(cfg.GeoIPDatabasePath) {
		cfg.GeoIPDatabasePath = filepath.ToSlash(cfg.GeoIPDatabasePath)
	}
	if usesSlashPath(cfg.OpenrestyGeoIPDatabasePath) {
		cfg.OpenrestyGeoIPDatabasePath = filepath.ToSlash(cfg.OpenrestyGeoIPDatabasePath)
	}
	cfg.GeoIPLookupAPITokenFile = strings.TrimSpace(cfg.GeoIPLookupAPITokenFile)
	if usesSlashPath(cfg.GeoIPLookupAPITokenFile) {
		cfg.GeoIPLookupAPITokenFile = filepath.ToSlash(cfg.GeoIPLookupAPITokenFile)
	}
	cfg.GeoIPLookupAPIURL = strings.TrimSpace(cfg.GeoIPLookupAPIURL)
	cfg.GeoIPLookupAPIToken = strings.TrimSpace(cfg.GeoIPLookupAPIToken)
}

func hasEnvConfig() bool {
	for _, key := range []string{
		"DUSHENGCDN_SERVER_URL",
		"DUSHENGCDN_AGENT_TOKEN",
		"DUSHENGCDN_AGENT_TOKEN_FILE",
		"DUSHENGCDN_DISCOVERY_TOKEN",
		"DUSHENGCDN_DISCOVERY_TOKEN_FILE",
		"DUSHENGCDN_NODE_NAME",
		"DUSHENGCDN_NODE_IP",
		"DUSHENGCDN_DATA_DIR",
		"DUSHENGCDN_OPENRESTY_PATH",
		"DUSHENGCDN_GEOIP_DATABASE_URL",
		"DUSHENGCDN_GEOIP_DATABASE_PATH",
		"DUSHENGCDN_OPENRESTY_GEOIP_DATABASE_PATH",
		"DUSHENGCDN_GEOIP_LOOKUP_API_URL",
		"DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN",
		"DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN_FILE",
		"DUSHENGCDN_HEARTBEAT_INTERVAL",
		"DUSHENGCDN_GEOIP_UPDATE_INTERVAL",
		"DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT",
		"DUSHENGCDN_REQUEST_TIMEOUT",
		"DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func applyEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	overrideString := func(key string, target *string) {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			*target = value
		}
	}
	overrideString("DUSHENGCDN_SERVER_URL", &cfg.ServerURL)
	if err := overrideSecretFromEnvOrFile("DUSHENGCDN_AGENT_TOKEN", "DUSHENGCDN_AGENT_TOKEN_FILE", &cfg.AgentToken); err != nil {
		return err
	}
	if err := overrideSecretFromEnvOrFile("DUSHENGCDN_DISCOVERY_TOKEN", "DUSHENGCDN_DISCOVERY_TOKEN_FILE", &cfg.DiscoveryToken); err != nil {
		return err
	}
	overrideString("DUSHENGCDN_NODE_NAME", &cfg.NodeName)
	overrideString("DUSHENGCDN_NODE_IP", &cfg.NodeIP)
	overrideString("DUSHENGCDN_DATA_DIR", &cfg.DataDir)
	overrideString("DUSHENGCDN_OPENRESTY_PATH", &cfg.OpenrestyPath)
	overrideString("DUSHENGCDN_GEOIP_DATABASE_URL", &cfg.GeoIPDatabaseURL)
	overrideString("DUSHENGCDN_GEOIP_DATABASE_PATH", &cfg.GeoIPDatabasePath)
	overrideString("DUSHENGCDN_OPENRESTY_GEOIP_DATABASE_PATH", &cfg.OpenrestyGeoIPDatabasePath)
	overrideString("DUSHENGCDN_GEOIP_LOOKUP_API_URL", &cfg.GeoIPLookupAPIURL)
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN_FILE")); value != "" {
		cfg.GeoIPLookupAPITokenFile = value
	}
	if err := applySecretValueOrFile("DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN", cfg.GeoIPLookupAPITokenFile, &cfg.GeoIPLookupAPIToken); err != nil {
		return err
	}
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_HEARTBEAT_INTERVAL")); value != "" {
		if duration, err := parseDurationValue(value); err == nil {
			cfg.HeartbeatInterval = duration
		}
	}
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT")); value != "" {
		if duration, err := parseDurationValue(value); err == nil {
			cfg.GeoIPLookupAPITimeout = duration
		}
	}
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_GEOIP_UPDATE_INTERVAL")); value != "" {
		if duration, err := parseDurationValue(value); err == nil {
			cfg.GeoIPUpdateInterval = duration
		}
	}
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_REQUEST_TIMEOUT")); value != "" {
		if duration, err := parseDurationValue(value); err == nil {
			cfg.RequestTimeout = duration
		}
	}
	if value := strings.TrimSpace(os.Getenv("DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT")); value != "" {
		var port int
		if _, err := fmt.Sscanf(value, "%d", &port); err == nil {
			cfg.OpenrestyObservabilityPort = port
		}
	}
	return nil
}

func overrideSecretFromEnvOrFile(valueKey string, fileKey string, target *string) error {
	filePath := strings.TrimSpace(os.Getenv(fileKey))
	return applySecretValueOrFile(valueKey, filePath, target)
}

func applySecretValueOrFile(valueKey string, filePath string, target *string) error {
	value := strings.TrimSpace(os.Getenv(valueKey))
	filePath = strings.TrimSpace(filePath)
	if value != "" && filePath != "" {
		return fmt.Errorf("use only one of %s and a token file", valueKey)
	}
	if value != "" {
		*target = value
		return nil
	}
	if filePath == "" {
		return nil
	}
	return readSecretFile(filePath, target)
}

func readSecretFile(filePath string, target *string) error {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	*target = strings.TrimSpace(string(content))
	return nil
}

func parseDurationValue(value string) (MillisecondDuration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	if parsed, err := time.ParseDuration(trimmed); err == nil {
		return MillisecondDuration(parsed), nil
	}
	ms, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	return MillisecondDuration(time.Duration(ms) * time.Millisecond), nil
}

func usesSlashPath(path string) bool {
	return strings.HasPrefix(path, "/")
}

func joinManagedPath(base string, relative string) string {
	if usesSlashPath(base) {
		return pathpkg.Join(filepath.ToSlash(base), relative)
	}
	return filepath.Join(base, relative)
}

func validate(cfg *Config) error {
	if err := validateServerURL(cfg.ServerURL); err != nil {
		return err
	}
	if cfg.ServerURL == "" {
		return errors.New("server_url 不能为空")
	}
	if strings.TrimSpace(cfg.AgentToken) == "" && strings.TrimSpace(cfg.DiscoveryToken) == "" {
		return errors.New("agent_token 和 discovery_token 不能同时为空")
	}
	if cfg.NodeName == "" {
		return errors.New("node_name 不能为空")
	}
	if cfg.NodeIP == "" {
		return errors.New("node_ip 不能为空")
	}
	if cfg.OpenrestyObservabilityPort <= 0 || cfg.OpenrestyObservabilityPort > 65535 {
		return errors.New("openresty_observability_port 必须在 1-65535 之间")
	}
	if cfg.ObservabilityReplayMinutes <= 0 {
		return errors.New("observability_replay_minutes 必须大于 0")
	}
	if strings.TrimSpace(cfg.GeoIPDatabaseURL) == "" {
		return errors.New("geoip_database_url cannot be empty")
	}
	if _, err := security.ValidatePublicHTTPURL(cfg.GeoIPDatabaseURL, true); err != nil {
		return fmt.Errorf("geoip_database_url is unsafe: %w", err)
	}
	if strings.TrimSpace(cfg.GeoIPDatabasePath) == "" {
		return errors.New("geoip_database_path cannot be empty")
	}
	if strings.TrimSpace(cfg.OpenrestyGeoIPDatabasePath) == "" {
		return errors.New("openresty_geoip_database_path cannot be empty")
	}
	if cfg.GeoIPUpdateInterval <= 0 {
		return errors.New("geoip_update_interval must be greater than 0")
	}
	if cfg.GeoIPLookupAPITimeout <= 0 {
		return errors.New("geoip_lookup_api_timeout must be greater than 0")
	}
	if strings.TrimSpace(cfg.GeoIPLookupAPIURL) != "" {
		if _, err := security.ValidatePublicHTTPURL(cfg.GeoIPLookupAPIURL, true); err != nil {
			return fmt.Errorf("geoip_lookup_api_url is unsafe: %w", err)
		}
	}
	return nil
}

func validateServerURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("server_url format is invalid")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "http", "https":
		return nil
	default:
		return errors.New("server_url scheme must be http or https")
	}
}

func (cfg *Config) InitialAuthToken() string {
	if cfg == nil {
		return ""
	}
	if token := strings.TrimSpace(cfg.AgentToken); token != "" {
		return token
	}
	return strings.TrimSpace(cfg.DiscoveryToken)
}

func (cfg *Config) Save() error {
	if cfg == nil {
		return errors.New("config 不能为空")
	}
	if cfg.configPath == "" {
		return errors.New("config path 未初始化")
	}
	saved := *cfg
	if strings.TrimSpace(saved.GeoIPLookupAPITokenFile) != "" {
		saved.GeoIPLookupAPIToken = ""
	}
	data, err := json.MarshalIndent(&saved, "", "  ")
	if err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomicIfChanged(cfg.configPath, data, 0o600); err != nil {
		return err
	}
	return tightenConfigFilePermissions(cfg.configPath)
}

func tightenConfigFilePermissions(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if os.PathSeparator == '\\' {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func detectHostname() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(host)
}

func normalizeResolverList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func detectNodeIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	bestIP := ""
	bestPriority := -1
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ipv4 := normalizeIPv4(ipNet.IP)
			priority := nodeIPPriority(ipv4)
			if priority > bestPriority {
				bestIP = ipv4.String()
				bestPriority = priority
			}
			if bestPriority == 2 {
				return bestIP
			}
		}
	}
	return bestIP
}

func normalizeIPv4(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	return ip.To4()
}

func nodeIPPriority(ip net.IP) int {
	return iputil.Score(ip)
}
