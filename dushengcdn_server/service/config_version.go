package service

import (
	"context"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service/configversion"
	"dushengcdn/service/openresty"
	"dushengcdn/utils/security"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var routeUpstreamLookupIPAddr = net.DefaultResolver.LookupIPAddr

type ReleaseResult struct {
	Version *model.ConfigVersion `json:"version"`
	Routes  []*ProxyRouteView    `json:"routes"`
}

type SupportFile = openresty.SupportFile

type ConfigPreviewResult struct {
	SnapshotJSON   string        `json:"snapshot_json"`
	MainConfig     string        `json:"main_config"`
	RouteConfig    string        `json:"route_config"`
	RenderedConfig string        `json:"rendered_config"`
	SupportFiles   []SupportFile `json:"support_files"`
	Checksum       string        `json:"checksum"`
	RouteCount     int           `json:"route_count"`
	WebsiteCount   int           `json:"website_count"`
}

type ConfigVersionSummary = model.ConfigVersionSummary

type ConfigVersionDetail struct {
	ID               uint      `json:"id"`
	Version          string    `json:"version"`
	SnapshotJSON     string    `json:"snapshot_json"`
	MainConfig       string    `json:"main_config"`
	RenderedConfig   string    `json:"rendered_config"`
	SupportFilesJSON string    `json:"support_files_json"`
	Checksum         string    `json:"checksum"`
	IsActive         bool      `json:"is_active"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

type ConfigDiffResult struct {
	ActiveVersion        string                 `json:"active_version,omitempty"`
	AddedSites           []string               `json:"added_sites"`
	RemovedSites         []string               `json:"removed_sites"`
	ModifiedSites        []string               `json:"modified_sites"`
	AddedDomains         []string               `json:"added_domains"`
	RemovedDomains       []string               `json:"removed_domains"`
	ModifiedDomains      []string               `json:"modified_domains"`
	MainConfigChanged    bool                   `json:"main_config_changed"`
	SnapshotChanged      bool                   `json:"snapshot_changed"`
	RuntimeConfigChanged bool                   `json:"runtime_config_changed"`
	ChangedOptionKeys    []string               `json:"changed_option_keys"`
	ChangedOptionDetails []ConfigOptionDiffItem `json:"changed_option_details"`
	CurrentWebsiteCount  int                    `json:"current_website_count"`
	ActiveWebsiteCount   int                    `json:"active_website_count"`
}

type ConfigOptionDiffItem = configversion.OptionDiffItem

type snapshotRoute struct {
	SiteName                   string                        `json:"site_name,omitempty"`
	Domain                     string                        `json:"domain"`
	Domains                    []string                      `json:"domains,omitempty"`
	OriginURL                  string                        `json:"origin_url"`
	OriginHost                 string                        `json:"origin_host,omitempty"`
	Upstreams                  []string                      `json:"upstreams,omitempty"`
	NodePool                   string                        `json:"node_pool,omitempty"`
	Enabled                    bool                          `json:"enabled"`
	EnableHTTPS                bool                          `json:"enable_https"`
	CertID                     *uint                         `json:"cert_id,omitempty"`
	CertIDs                    []uint                        `json:"cert_ids,omitempty"`
	DomainCertIDs              []uint                        `json:"domain_cert_ids,omitempty"`
	RedirectHTTP               bool                          `json:"redirect_http"`
	LimitConnPerServer         int                           `json:"limit_conn_per_server,omitempty"`
	LimitConnPerIP             int                           `json:"limit_conn_per_ip,omitempty"`
	LimitRate                  string                        `json:"limit_rate,omitempty"`
	ProxyBufferingMode         string                        `json:"proxy_buffering_mode,omitempty"`
	CacheEnabled               bool                          `json:"cache_enabled"`
	CachePolicy                string                        `json:"cache_policy,omitempty"`
	CacheRules                 []string                      `json:"cache_rules,omitempty"`
	CustomHeaders              []ProxyRouteCustomHeaderInput `json:"custom_headers,omitempty"`
	PoWEnabled                 bool                          `json:"pow_enabled,omitempty"`
	PoWConfig                  *ProxyRoutePoWConfig          `json:"pow_config,omitempty"`
	WAFEnabled                 bool                          `json:"waf_enabled,omitempty"`
	WAFMode                    string                        `json:"waf_mode,omitempty"`
	WAFConfig                  *ProxyRouteWAFConfig          `json:"waf_config,omitempty"`
	CCEnabled                  bool                          `json:"cc_enabled,omitempty"`
	CCMode                     string                        `json:"cc_mode,omitempty"`
	CCConfig                   *ProxyRouteCCConfig           `json:"cc_config,omitempty"`
	BasicAuthEnabled           bool                          `json:"basic_auth_enabled,omitempty"`
	BasicAuthUsername          string                        `json:"basic_auth_username,omitempty"`
	BasicAuthPasswordHash      string                        `json:"basic_auth_password_hash,omitempty"`
	BasicAuthPassword          string                        `json:"-"`
	RegionRestrictionEnabled   bool                          `json:"region_restriction_enabled,omitempty"`
	RegionRestrictionMode      string                        `json:"region_restriction_mode,omitempty"`
	RegionRestrictionCountries []string                      `json:"region_restriction_countries,omitempty"`
	Remark                     string                        `json:"remark,omitempty"`
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
	Servers           []string
	UsesNamedUpstream bool
}

type openRestyConfigSnapshot struct {
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

type snapshotDocument struct {
	Routes          []snapshotRoute         `json:"routes"`
	OpenRestyConfig openRestyConfigSnapshot `json:"openresty_config"`
}

type configBundle struct {
	Routes            []*model.ProxyRoute
	SnapshotRoutes    []snapshotRoute
	OpenRestyConfig   openRestyConfigSnapshot
	SnapshotJSON      string
	MainConfig        string
	RouteConfig       string
	SupportFiles      []SupportFile
	Checksum          string
	Artifacts         []configVersionArtifactBundle
	ChangedOptionKeys []string
}

type configVersionArtifactBundle = configversion.ArtifactBundle

const (
	nginxCertDirPlaceholder             = "__DUSHENGCDN_CERT_DIR__"
	nginxRouteConfigPlaceholder         = "__DUSHENGCDN_ROUTE_CONFIG__"
	nginxAccessLogPlaceholder           = "__DUSHENGCDN_ACCESS_LOG__"
	nginxLuaDirPlaceholder              = "__DUSHENGCDN_LUA_DIR__"
	nginxObservabilityListenPlaceholder = "__DUSHENGCDN_OBSERVABILITY_LISTEN__"
	nginxObservabilityPortPlaceholder   = "__DUSHENGCDN_OBSERVABILITY_PORT__"
)

var requiredMainConfigTemplatePlaceholders = []string{
	"{{OpenRestyWorkerProcesses}}",
	"{{OpenRestyWorkerConnections}}",
	"{{OpenRestyWorkerRlimitNofile}}",
	"{{OpenRestyConnectionUpgradeMap}}",
	"{{OpenRestyDefaultServerBlock}}",
	"{{OpenRestyAccessLogPath}}",
	"{{OpenRestyEventsUseDirective}}",
	"{{OpenRestyEventsMultiAcceptDirective}}",
	"{{OpenRestyKeepaliveTimeout}}",
	"{{OpenRestyKeepaliveRequests}}",
	"{{OpenRestyClientHeaderTimeout}}",
	"{{OpenRestyClientBodyTimeout}}",
	"{{OpenRestyClientMaxBodySize}}",
	"{{OpenRestyLargeClientHeaderBuffers}}",
	"{{OpenRestySendTimeout}}",
	"{{OpenRestyProxyConnectTimeout}}",
	"{{OpenRestyProxySendTimeout}}",
	"{{OpenRestyProxyReadTimeout}}",
	"{{OpenRestyProxyRequestBuffering}}",
	"{{OpenRestyProxyBuffering}}",
	"{{OpenRestyProxyBuffers}}",
	"{{OpenRestyProxyBufferSize}}",
	"{{OpenRestyProxyBusyBuffersSize}}",
	"{{OpenRestyGzip}}",
	"{{OpenRestyGzipMinLength}}",
	"{{OpenRestyGzipCompLevel}}",
	"{{OpenRestyCacheBlock}}",
	"{{OpenRestyRouteConfigInclude}}",
}

func ListConfigVersions() ([]*ConfigVersionSummary, error) {
	return model.ListConfigVersionSummaries()
}

func GetConfigVersionDetail(id uint) (*ConfigVersionDetail, error) {
	version, err := model.GetConfigVersionByID(id)
	if err != nil {
		return nil, err
	}
	return BuildConfigVersionDetailForAdmin(version), nil
}

func GetActiveConfigVersion() (*ConfigVersionDetail, error) {
	version, err := model.GetActiveConfigVersion()
	if err != nil {
		return nil, err
	}
	return BuildConfigVersionDetailForAdmin(version), nil
}

func BuildConfigVersionDetailForAdmin(version *model.ConfigVersion) *ConfigVersionDetail {
	if version == nil {
		return nil
	}
	return &ConfigVersionDetail{
		ID:               version.ID,
		Version:          version.Version,
		SnapshotJSON:     redactConfigSnapshotForAdmin(version.SnapshotJSON),
		MainConfig:       version.MainConfig,
		RenderedConfig:   redactRenderedConfigForAdmin(version.RenderedConfig),
		SupportFilesJSON: redactSupportFilesJSONForAdmin(version.SupportFilesJSON),
		Checksum:         version.Checksum,
		IsActive:         version.IsActive,
		CreatedBy:        version.CreatedBy,
		CreatedAt:        version.CreatedAt,
	}
}

func redactConfigSnapshotForAdmin(snapshotJSON string) string {
	if strings.TrimSpace(snapshotJSON) == "" {
		return snapshotJSON
	}
	var doc snapshotDocument
	if err := json.Unmarshal([]byte(snapshotJSON), &doc); err != nil {
		return snapshotJSON
	}
	for index := range doc.Routes {
		doc.Routes[index].BasicAuthPasswordHash = ""
		doc.Routes[index].BasicAuthPassword = ""
		doc.Routes[index].CustomHeaders = redactSensitiveCustomHeaders(doc.Routes[index].CustomHeaders)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return snapshotJSON
	}
	return string(raw)
}

func redactRenderedConfigForAdmin(renderedConfig string) string {
	if strings.TrimSpace(renderedConfig) == "" {
		return renderedConfig
	}
	linePattern := regexp.MustCompile(`(?m)^(\s*proxy_set_header\s+([A-Za-z0-9_-]+)\s+)([^;]*)(;\s*)$`)
	redacted := linePattern.ReplaceAllStringFunc(renderedConfig, func(line string) string {
		matches := linePattern.FindStringSubmatch(line)
		if len(matches) != 5 || !isSensitiveProxyRouteCustomHeader(matches[2]) {
			return line
		}
		return matches[1] + quoteNginxHeaderValue(redactedProxyRouteCustomHeaderValue) + matches[4]
	})
	return security.RedactSensitiveText(redacted)
}

func redactSupportFilesJSONForAdmin(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var files []SupportFile
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return raw
	}
	redacted, err := json.Marshal(redactSupportFilesForAdmin(files))
	if err != nil {
		return raw
	}
	return string(redacted)
}

func redactSupportFilesForAdmin(files []SupportFile) []SupportFile {
	if len(files) == 0 {
		return files
	}
	result := make([]SupportFile, 0, len(files))
	for _, file := range files {
		file.Redacted = false
		if supportFileContainsSensitiveKey(file) {
			file.Content = "[redacted private key; available only through the agent configuration channel]"
			file.Redacted = true
		}
		result = append(result, file)
	}
	return result
}

func supportFileContainsSensitiveKey(file SupportFile) bool {
	path := strings.ToLower(strings.TrimSpace(file.Path))
	content := strings.ToUpper(file.Content)
	if strings.HasSuffix(path, ".key") {
		return true
	}
	if strings.Contains(content, "PRIVATE KEY") {
		return true
	}
	return false
}

func PreviewConfigVersion() (*ConfigPreviewResult, error) {
	bundle, err := buildCurrentConfigBundle(false)
	if err != nil {
		return nil, err
	}
	return &ConfigPreviewResult{
		SnapshotJSON:   redactConfigSnapshotForAdmin(bundle.SnapshotJSON),
		MainConfig:     bundle.MainConfig,
		RouteConfig:    redactRenderedConfigForAdmin(bundle.RouteConfig),
		RenderedConfig: redactRenderedConfigForAdmin(bundle.RouteConfig),
		SupportFiles:   redactSupportFilesForAdmin(bundle.SupportFiles),
		Checksum:       bundle.Checksum,
		RouteCount:     len(bundle.Routes),
		WebsiteCount:   len(bundle.SnapshotRoutes),
	}, nil
}

func DiffConfigVersion() (*ConfigDiffResult, error) {
	bundle, err := buildCurrentConfigBundle(false)
	if err != nil {
		return nil, err
	}
	result := &ConfigDiffResult{
		AddedSites:           []string{},
		RemovedSites:         []string{},
		ModifiedSites:        []string{},
		AddedDomains:         []string{},
		RemovedDomains:       []string{},
		ModifiedDomains:      []string{},
		ChangedOptionKeys:    []string{},
		ChangedOptionDetails: []ConfigOptionDiffItem{},
		CurrentWebsiteCount:  len(bundle.SnapshotRoutes),
	}
	activeVersion, err := model.GetActiveConfigVersion()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			for _, route := range bundle.SnapshotRoutes {
				result.AddedSites = append(result.AddedSites, route.SiteName)
				result.AddedDomains = append(result.AddedDomains, route.Domains...)
			}
			result.MainConfigChanged = true
			result.ChangedOptionKeys = openRestyOptionKeys()
			result.ChangedOptionDetails = buildInitialOpenRestyOptionDiffs(bundle.OpenRestyConfig)
			result.SnapshotChanged = len(bundle.SnapshotRoutes) > 0
			result.RuntimeConfigChanged = len(bundle.Routes) > 0
			sort.Strings(result.AddedSites)
			sort.Strings(result.AddedDomains)
			sort.Strings(result.ChangedOptionKeys)
			return result, nil
		}
		return nil, err
	}
	result.ActiveVersion = activeVersion.Version
	activeSnapshot, err := parseSnapshotDocument(activeVersion.SnapshotJSON)
	if err != nil {
		return nil, err
	}
	result.ActiveWebsiteCount = len(activeSnapshot.Routes)
	currentSiteMap := flattenSnapshotRoutesBySite(bundle.SnapshotRoutes)
	activeSiteMap := flattenSnapshotRoutesBySite(activeSnapshot.Routes)
	for siteName, currentRoute := range currentSiteMap {
		activeRoute, ok := activeSiteMap[siteName]
		if !ok {
			result.AddedSites = append(result.AddedSites, siteName)
			continue
		}
		if !snapshotRouteConfigEqual(activeRoute, currentRoute) {
			result.ModifiedSites = append(result.ModifiedSites, siteName)
		}
	}
	for siteName := range activeSiteMap {
		if _, ok := currentSiteMap[siteName]; !ok {
			result.RemovedSites = append(result.RemovedSites, siteName)
		}
	}
	currentMap := flattenSnapshotRoutesByDomain(bundle.SnapshotRoutes)
	activeMap := flattenSnapshotRoutesByDomain(activeSnapshot.Routes)
	for domain, currentRoute := range currentMap {
		activeRoute, ok := activeMap[domain]
		if !ok {
			result.AddedDomains = append(result.AddedDomains, domain)
			continue
		}
		if !snapshotRouteConfigEqual(activeRoute, currentRoute) {
			result.ModifiedDomains = append(result.ModifiedDomains, domain)
		}
	}
	for domain := range activeMap {
		if _, ok := currentMap[domain]; !ok {
			result.RemovedDomains = append(result.RemovedDomains, domain)
		}
	}
	result.MainConfigChanged = activeVersion.MainConfig != bundle.MainConfig
	runtimeConfigChanged, err := configBundleRuntimeChanged(activeVersion, bundle)
	if err != nil {
		return nil, err
	}
	result.RuntimeConfigChanged = runtimeConfigChanged
	result.ChangedOptionDetails = diffOpenRestyOptionDetails(activeSnapshot.OpenRestyConfig, bundle.OpenRestyConfig)
	result.ChangedOptionKeys = extractOptionDiffKeys(result.ChangedOptionDetails)
	result.SnapshotChanged = snapshotRoutesStateChanged(activeSnapshot.Routes, bundle.SnapshotRoutes) || len(result.ChangedOptionDetails) > 0
	sort.Strings(result.AddedSites)
	sort.Strings(result.RemovedSites)
	sort.Strings(result.ModifiedSites)
	sort.Strings(result.AddedDomains)
	sort.Strings(result.RemovedDomains)
	sort.Strings(result.ModifiedDomains)
	sort.Strings(result.ChangedOptionKeys)
	return result, nil
}

func HasConfigChanges() (bool, error) {
	bundle, err := buildCurrentConfigBundle(false)
	if err != nil {
		return false, err
	}
	activeVersion, err := model.GetActiveConfigVersion()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return len(bundle.Routes) > 0, nil
		}
		return false, err
	}
	return configBundleRuntimeChanged(activeVersion, bundle)
}

func PublishConfigVersion(createdBy string, force bool) (*ReleaseResult, error) {
	bundle, err := buildCurrentConfigBundle(true)
	if err != nil {
		return nil, err
	}
	if len(bundle.Routes) == 0 {
		return nil, errors.New("没有可发布的启用规则")
	}
	activeVersion, err := model.GetActiveConfigVersion()
	if err == nil {
		changed, compareErr := configBundleRuntimeChanged(activeVersion, bundle)
		if compareErr != nil {
			return nil, compareErr
		}
		if !force && !changed {
			return nil, errors.New("当前运行配置没有变更，无需重复发布")
		}
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	supportFilesJSON, err := json.Marshal(bundle.SupportFiles)
	if err != nil {
		return nil, err
	}
	version, err := nextVersionNumber(time.Now())
	if err != nil {
		return nil, err
	}
	record := &model.ConfigVersion{
		Version:          version,
		SnapshotJSON:     bundle.SnapshotJSON,
		MainConfig:       bundle.MainConfig,
		RenderedConfig:   bundle.RouteConfig,
		SupportFilesJSON: string(supportFilesJSON),
		Checksum:         bundle.Checksum,
		IsActive:         true,
		CreatedBy:        createdBy,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ConfigVersion{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if err := createConfigVersionArtifacts(tx, record.ID, bundle.Artifacts); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("版本号生成冲突，请重试")
		}
		return nil, err
	}
	BroadcastAgentWSActiveConfigForVersion(record)
	routeViews, err := buildProxyRouteViews(bundle.Routes)
	if err != nil {
		return nil, err
	}
	return &ReleaseResult{
		Version: record,
		Routes:  routeViews,
	}, nil
}

func ActivateConfigVersion(id uint) (*model.ConfigVersion, error) {
	version, err := model.GetConfigVersionByID(id)
	if err != nil {
		return nil, err
	}
	if err := ensureConfigVersionArtifacts(version); err != nil {
		return nil, err
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ConfigVersion{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		if err := tx.Model(version).Update("is_active", true).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	version.IsActive = true
	BroadcastAgentWSActiveConfigForVersion(version)
	return version, nil
}

func ensureConfigVersionArtifacts(version *model.ConfigVersion) error {
	return ensureConfigVersionArtifactsForPools(version, nil)
}

func ensureConfigVersionArtifactsForPools(version *model.ConfigVersion, requestedPools []string) error {
	if version == nil {
		return nil
	}
	existing, err := model.ListConfigVersionArtifacts(version.ID)
	if err != nil {
		return err
	}
	existingPools := make(map[string]struct{}, len(existing))
	for _, artifact := range existing {
		existingPools[normalizeNodePoolName(artifact.PoolName)] = struct{}{}
	}
	pools, err := compatibilityArtifactPools(requestedPools)
	if err != nil {
		return err
	}
	missingPools := make([]string, 0, len(pools))
	for _, poolName := range pools {
		if _, ok := existingPools[poolName]; ok {
			continue
		}
		missingPools = append(missingPools, poolName)
	}
	if len(missingPools) == 0 {
		return nil
	}
	var supportFiles []SupportFile
	if strings.TrimSpace(version.SupportFilesJSON) != "" {
		if err := json.Unmarshal([]byte(version.SupportFilesJSON), &supportFiles); err != nil {
			return err
		}
	}
	supportFilesJSON, err := json.Marshal(supportFiles)
	if err != nil {
		return err
	}
	mainConfigChecksum := checksum(version.MainConfig)
	routeConfigChecksum := checksum(version.RenderedConfig)
	routeCount := len(versionRoutesFromSnapshot(version.SnapshotJSON))
	return model.DB.Transaction(func(tx *gorm.DB) error {
		for _, poolName := range missingPools {
			record := &model.ConfigVersionArtifact{
				ConfigVersionID:     version.ID,
				PoolName:            poolName,
				Checksum:            version.Checksum,
				MainConfigChecksum:  mainConfigChecksum,
				RouteConfigChecksum: routeConfigChecksum,
				RenderedConfig:      version.RenderedConfig,
				SupportFilesJSON:    string(supportFilesJSON),
				RouteCount:          routeCount,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func compatibilityArtifactPools(requestedPools []string) ([]string, error) {
	poolSet := map[string]struct{}{
		normalizeNodePoolName("default"): {},
	}
	for _, raw := range requestedPools {
		poolSet[normalizeNodePoolName(raw)] = struct{}{}
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		poolSet[normalizeNodePoolName(node.PoolName)] = struct{}{}
	}
	pools := make([]string, 0, len(poolSet))
	for poolName := range poolSet {
		if poolName != "" {
			pools = append(pools, poolName)
		}
	}
	sort.Strings(pools)
	return pools, nil
}

func versionRoutesFromSnapshot(snapshotJSON string) []snapshotRoute {
	snapshot, err := parseSnapshotDocument(snapshotJSON)
	if err != nil || snapshot == nil {
		return []snapshotRoute{}
	}
	return snapshot.Routes
}

func createConfigVersionArtifacts(tx *gorm.DB, versionID uint, bundles []configVersionArtifactBundle) error {
	return configversion.CreateArtifacts(tx, versionID, bundles)
}

func CleanupConfigVersions(keepCount int) (int64, error) {
	return configversion.CleanupVersions(keepCount)
}

func buildCurrentConfigBundle(requireRoutes bool) (*configBundle, error) {
	routes, err := model.GetEnabledProxyRoutes()
	if err != nil {
		return nil, err
	}
	if requireRoutes && len(routes) == 0 {
		return nil, errors.New("没有可发布的启用规则")
	}
	routeConfigContext, err := newRouteConfigRenderContext(routes, defaultRouteConfigQueries)
	if err != nil {
		return nil, err
	}
	snapshotRoutes, err := buildSnapshotRoutesWithContext(routes, routeConfigContext)
	if err != nil {
		return nil, err
	}
	openRestyConfig := buildOpenRestyConfigSnapshot()
	snapshotDoc := snapshotDocument{
		Routes:          snapshotRoutes,
		OpenRestyConfig: openRestyConfig,
	}
	snapshotJSON, err := json.Marshal(snapshotDoc)
	if err != nil {
		return nil, err
	}
	routeConfig, supportFiles, err := renderRouteConfigWithContext(routes, openRestyConfig, routeConfigContext)
	if err != nil {
		return nil, err
	}
	accessSupportFiles, err := renderPoolAccessSupportFiles(routes)
	if err != nil {
		return nil, err
	}
	supportFiles = append(supportFiles, accessSupportFiles...)
	mainConfig := renderMainConfig(openRestyConfig)
	artifacts, err := buildConfigVersionArtifacts(routes, openRestyConfig, mainConfig, routeConfigContext)
	if err != nil {
		return nil, err
	}
	return &configBundle{
		Routes:            routes,
		SnapshotRoutes:    snapshotRoutes,
		OpenRestyConfig:   openRestyConfig,
		SnapshotJSON:      string(snapshotJSON),
		MainConfig:        mainConfig,
		RouteConfig:       routeConfig,
		SupportFiles:      supportFiles,
		Checksum:          checksumBundle(mainConfig, routeConfig, supportFiles),
		Artifacts:         artifacts,
		ChangedOptionKeys: openRestyOptionKeys(),
	}, nil
}

func buildConfigVersionArtifacts(routes []*model.ProxyRoute, cfg openRestyConfigSnapshot, mainConfig string, context proxyRouteTLSCertificateLoader) ([]configVersionArtifactBundle, error) {
	poolRoutes, err := buildConfigVersionArtifactRouteMap(routes)
	if err != nil {
		return nil, err
	}
	poolNames := make([]string, 0, len(poolRoutes))
	for poolName := range poolRoutes {
		poolNames = append(poolNames, poolName)
	}
	sort.Strings(poolNames)
	result := make([]configVersionArtifactBundle, 0, len(poolNames))
	for _, poolName := range poolNames {
		routesForPool := poolRoutes[poolName]
		routeConfig, supportFiles, err := renderRouteConfigWithContext(routesForPool, cfg, context)
		if err != nil {
			return nil, err
		}
		accessSupportFiles, err := renderPoolAccessSupportFiles(routesForPool)
		if err != nil {
			return nil, err
		}
		supportFiles = append(supportFiles, accessSupportFiles...)
		supportFilesJSON, err := json.Marshal(supportFiles)
		if err != nil {
			return nil, err
		}
		result = append(result, configVersionArtifactBundle{
			PoolName:            poolName,
			RouteConfig:         routeConfig,
			SupportFiles:        supportFiles,
			Checksum:            checksumBundle(mainConfig, routeConfig, supportFiles),
			MainConfigChecksum:  checksum(mainConfig),
			RouteConfigChecksum: checksum(routeConfig),
			SupportFilesJSON:    string(supportFilesJSON),
			RouteCount:          len(routesForPool),
		})
	}
	return result, nil
}

func renderPoolAccessSupportFiles(routes []*model.ProxyRoute) ([]SupportFile, error) {
	accessRoutes, err := buildOpenRestyAccessRoutes(routes)
	if err != nil {
		return nil, err
	}
	return openresty.RenderAccessSupportFiles(accessRoutes)
}

func buildOpenRestyAccessRoutes(routes []*model.ProxyRoute) ([]openresty.AccessRoute, error) {
	result := make([]openresty.AccessRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		wafConfig, err := decodeStoredWAFConfig(route.WAFEnabled, route.WAFConfig)
		if err != nil {
			return nil, fmt.Errorf("route %s waf_config is invalid", route.Domain)
		}
		ccConfig, err := decodeStoredCCConfig(route.CCEnabled, route.CCConfig)
		if err != nil {
			return nil, fmt.Errorf("route %s cc_config is invalid", route.Domain)
		}
		countries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return nil, err
		}
		result = append(result, openresty.AccessRoute{
			Domain:                     route.Domain,
			Domains:                    domains,
			PoWEnabled:                 route.PoWEnabled,
			PoWConfigJSON:              route.PoWConfig,
			DefaultPoWConfig:           defaultPoWConfig(),
			WAFEnabled:                 route.WAFEnabled,
			WAFMode:                    normalizeWAFMode(route.WAFMode),
			WAFConfig:                  wafConfig,
			CCEnabled:                  route.CCEnabled,
			CCMode:                     normalizeCCMode(route.CCMode),
			CCConfig:                   ccConfig,
			RegionRestrictionEnabled:   route.RegionRestrictionEnabled && len(countries) > 0,
			RegionRestrictionMode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
			RegionRestrictionCountries: countries,
		})
	}
	return result, nil
}

func buildConfigVersionArtifactRouteMap(routes []*model.ProxyRoute) (map[string][]*model.ProxyRoute, error) {
	poolRoutes := map[string][]*model.ProxyRoute{normalizeNodePoolName("default"): {}}
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		poolName := normalizeNodePoolName(node.PoolName)
		if poolName != "" {
			if _, ok := poolRoutes[poolName]; !ok {
				poolRoutes[poolName] = []*model.ProxyRoute{}
			}
		}
	}
	for _, route := range routes {
		poolNames, err := configVersionRouteTargetPools(route)
		if err != nil {
			return nil, err
		}
		for _, poolName := range poolNames {
			if poolName == "" {
				continue
			}
			poolRoutes[poolName] = append(poolRoutes[poolName], route)
		}
	}
	return poolRoutes, nil
}

func configVersionRouteTargetPools(route *model.ProxyRoute) ([]string, error) {
	if route == nil {
		return []string{normalizeNodePoolName("default")}, nil
	}
	seen := make(map[string]struct{})
	addPool := func(raw string) {
		poolName := normalizeNodePoolName(raw)
		if poolName == "" {
			poolName = normalizeNodePoolName("default")
		}
		seen[poolName] = struct{}{}
	}
	addPool(route.NodePool)
	if normalizeDDOSProtectionMode(route.DDOSProtectionMode) == DDOSProtectionModeAuto &&
		normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCustom {
		addPool(route.DDOSProtectionTarget)
	}
	if route.GSLBEnabled && strings.TrimSpace(route.GSLBPolicy) != "" {
		policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return nil, err
		}
		for _, pool := range policy.Pools {
			if pool.Enabled {
				addPool(pool.Name)
			}
		}
	}
	result := make([]string, 0, len(seen))
	for poolName := range seen {
		result = append(result, poolName)
	}
	sort.Strings(result)
	return result, nil
}

func buildSnapshotRoutes(routes []*model.ProxyRoute) ([]snapshotRoute, error) {
	context, err := newRouteConfigRenderContext(routes, defaultRouteConfigQueries)
	if err != nil {
		return nil, err
	}
	return buildSnapshotRoutesWithContext(routes, context)
}

func buildSnapshotRoutesWithContext(
	routes []*model.ProxyRoute,
	context proxyRouteTLSCertificateLoader,
) ([]snapshotRoute, error) {
	items := make([]snapshotRoute, 0, len(routes))
	for _, route := range routes {
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, fmt.Errorf("route %s domains are invalid", route.Domain)
		}
		customHeaders, err := decodeStoredCustomHeaders(route.CustomHeaders)
		if err != nil {
			return nil, fmt.Errorf("路由 %s 自定义请求头无效", route.Domain)
		}
		upstreams, err := decodeStoredUpstreams(route.Upstreams, route.OriginURL)
		if err != nil {
			return nil, fmt.Errorf("路由 %s 源站配置无效", route.Domain)
		}
		cacheRules, err := decodeStoredCacheRules(route.CacheRules)
		if err != nil {
			return nil, fmt.Errorf("路由 %s 缓存规则无效", route.Domain)
		}
		powConfig, err := decodeStoredPoWConfig(route.PoWEnabled, route.PoWConfig)
		if err != nil {
			return nil, fmt.Errorf("路由 %s PoW 配置无效", route.Domain)
		}
		if !route.PoWEnabled {
			powConfig = nil
		}
		wafConfig, err := decodeStoredWAFConfig(route.WAFEnabled, route.WAFConfig)
		if err != nil {
			return nil, fmt.Errorf("路由 %s WAF 配置无效", route.Domain)
		}
		if !route.WAFEnabled {
			wafConfig = nil
		}
		ccConfig, err := decodeStoredCCConfig(route.CCEnabled, route.CCConfig)
		if err != nil {
			return nil, fmt.Errorf("路由 %s CC 防护配置无效", route.Domain)
		}
		if !route.CCEnabled {
			ccConfig = nil
		}
		regionCountries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return nil, fmt.Errorf("路由 %s 地区限制配置无效", route.Domain)
		}
		regionEnabled := route.RegionRestrictionEnabled && len(regionCountries) > 0
		items = append(items, snapshotRoute{
			SiteName:                   normalizeProxyRouteSiteNameInput(route, route.SiteName, domains[0]),
			Domain:                     domains[0],
			Domains:                    domains,
			OriginURL:                  route.OriginURL,
			OriginHost:                 route.OriginHost,
			Upstreams:                  upstreams,
			NodePool:                   normalizeNodePoolName(route.NodePool),
			Enabled:                    route.Enabled,
			EnableHTTPS:                route.EnableHTTPS,
			CertID:                     route.CertID,
			CertIDs:                    mustDecodeSnapshotCertIDs(route),
			DomainCertIDs:              mustDecodeSnapshotDomainCertIDs(route, domains, context),
			RedirectHTTP:               route.RedirectHTTP,
			LimitConnPerServer:         route.LimitConnPerServer,
			LimitConnPerIP:             route.LimitConnPerIP,
			LimitRate:                  route.LimitRate,
			ProxyBufferingMode:         normalizeProxyRouteProxyBufferingMode(route.ProxyBufferingMode),
			CacheEnabled:               route.CacheEnabled,
			CachePolicy:                route.CachePolicy,
			CacheRules:                 cacheRules,
			CustomHeaders:              customHeaders,
			PoWEnabled:                 route.PoWEnabled,
			PoWConfig:                  powConfig,
			WAFEnabled:                 route.WAFEnabled,
			WAFMode:                    normalizeWAFMode(route.WAFMode),
			WAFConfig:                  wafConfig,
			CCEnabled:                  route.CCEnabled,
			CCMode:                     normalizeCCMode(route.CCMode),
			CCConfig:                   ccConfig,
			BasicAuthEnabled:           route.BasicAuthEnabled,
			BasicAuthUsername:          route.BasicAuthUsername,
			BasicAuthPasswordHash:      proxyRouteBasicAuthPasswordHashForView(route),
			RegionRestrictionEnabled:   regionEnabled,
			RegionRestrictionMode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
			RegionRestrictionCountries: regionCountries,
			Remark:                     route.Remark,
		})
	}
	return items, nil
}

func mustDecodeSnapshotCertIDs(route *model.ProxyRoute) []uint {
	if route == nil {
		return []uint{}
	}
	certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
	if err != nil {
		return []uint{}
	}
	return certIDs
}

func mustDecodeSnapshotDomainCertIDs(
	route *model.ProxyRoute,
	domains []string,
	context proxyRouteTLSCertificateLoader,
) []uint {
	if route == nil {
		return []uint{}
	}
	certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
	if err != nil {
		return []uint{}
	}
	domainCertIDs, err := resolveProxyRouteDomainCertIDsWithContext(route, domains, certIDs, context)
	if err != nil {
		return []uint{}
	}
	return domainCertIDs
}

func parseSnapshotDocument(snapshotJSON string) (*snapshotDocument, error) {
	text := strings.TrimSpace(snapshotJSON)
	if text == "" {
		return &snapshotDocument{Routes: []snapshotRoute{}}, nil
	}
	if strings.HasPrefix(text, "[") {
		var routes []snapshotRoute
		if err := json.Unmarshal([]byte(text), &routes); err != nil {
			return nil, errors.New("历史版本快照格式不合法")
		}
		return &snapshotDocument{Routes: normalizeSnapshotRoutes(routes)}, nil
	}
	var snapshot snapshotDocument
	if err := json.Unmarshal([]byte(text), &snapshot); err != nil {
		return nil, errors.New("历史版本快照格式不合法")
	}
	snapshot.Routes = normalizeSnapshotRoutes(snapshot.Routes)
	return &snapshot, nil
}

func normalizeSnapshotRoutes(routes []snapshotRoute) []snapshotRoute {
	if len(routes) == 0 {
		return []snapshotRoute{}
	}
	for index := range routes {
		normalizedDomains, err := decodeStoredDomains("", routes[index].Domain)
		if len(routes[index].Domains) > 0 {
			normalizedDomains, err = normalizeProxyRouteDomains(routes[index].Domains)
		}
		if err == nil && len(normalizedDomains) > 0 {
			routes[index].Domains = normalizedDomains
			routes[index].Domain = normalizedDomains[0]
			routes[index].SiteName = normalizeProxyRouteSiteNameInput(
				&model.ProxyRoute{SiteName: routes[index].SiteName},
				routes[index].SiteName,
				normalizedDomains[0],
			)
		}
		routes[index].NodePool = normalizeNodePoolName(routes[index].NodePool)
		normalizedHeaders, err := normalizeCustomHeaders(routes[index].CustomHeaders)
		if err == nil {
			routes[index].CustomHeaders = normalizedHeaders
		}
		normalizedCertIDs, primaryCertID, err := normalizeSnapshotCertificateIDs(routes[index].CertID, routes[index].CertIDs)
		if err == nil {
			routes[index].CertID = primaryCertID
			routes[index].CertIDs = normalizedCertIDs
		}
		normalizedDomainCertIDs, err := normalizeSnapshotDomainCertificateIDs(
			routes[index].Domains,
			routes[index].CertIDs,
			routes[index].DomainCertIDs,
		)
		if err == nil {
			routes[index].DomainCertIDs = normalizedDomainCertIDs
		}
		normalizedUpstreams, err := normalizeUpstreams(routes[index].OriginURL, routes[index].Upstreams)
		if err == nil {
			routes[index].OriginURL = normalizedUpstreams[0]
			routes[index].Upstreams = normalizedUpstreams
		}
		normalizedCacheRules, err := normalizeCacheRules(routes[index].CacheEnabled, routes[index].CachePolicy, routes[index].CacheRules)
		if err == nil {
			routes[index].CachePolicy = normalizeCachePolicy(routes[index].CacheEnabled, routes[index].CachePolicy)
			routes[index].CacheRules = normalizedCacheRules
		}
		normalizedLimitRate, err := normalizeProxyRouteLimitRate(routes[index].LimitRate)
		if err == nil {
			routes[index].LimitRate = normalizedLimitRate
		}
		routes[index].ProxyBufferingMode = normalizeProxyRouteProxyBufferingMode(routes[index].ProxyBufferingMode)
		if routes[index].PoWEnabled {
			raw, err := json.Marshal(routes[index].PoWConfig)
			if err == nil {
				normalizedPoWConfig, err := normalizePoWConfig(true, string(raw))
				if err == nil {
					routes[index].PoWConfig = &normalizedPoWConfig
				}
			}
		} else {
			routes[index].PoWConfig = nil
		}
		if routes[index].WAFEnabled {
			raw, err := json.Marshal(routes[index].WAFConfig)
			if err == nil {
				normalizedWAFConfig, err := normalizeWAFConfig(true, string(raw))
				if err == nil {
					routes[index].WAFConfig = &normalizedWAFConfig
					routes[index].WAFMode = normalizeWAFMode(routes[index].WAFMode)
				}
			}
		} else {
			routes[index].WAFConfig = nil
			routes[index].WAFMode = proxyRouteWAFModeBlock
		}
		if routes[index].CCEnabled {
			raw, err := json.Marshal(routes[index].CCConfig)
			if err == nil {
				normalizedCCConfig, err := normalizeCCConfig(true, string(raw))
				if err == nil {
					routes[index].CCConfig = &normalizedCCConfig
					routes[index].CCMode = normalizeCCMode(routes[index].CCMode)
				}
			}
		} else {
			routes[index].CCConfig = nil
			routes[index].CCMode = proxyRouteCCModeBlock
		}
		if !routes[index].BasicAuthEnabled {
			routes[index].BasicAuthUsername = ""
			routes[index].BasicAuthPasswordHash = ""
			routes[index].BasicAuthPassword = ""
		} else if routes[index].BasicAuthPasswordHash == "" {
			routes[index].BasicAuthPasswordHash = basicAuthCredentialHash(routes[index].BasicAuthUsername, routes[index].BasicAuthPassword)
			routes[index].BasicAuthPassword = ""
		}
		regionMode, regionCountries, err := normalizeProxyRouteRegionRestriction(
			routes[index].RegionRestrictionEnabled,
			routes[index].RegionRestrictionMode,
			routes[index].RegionRestrictionCountries,
		)
		if err == nil {
			routes[index].RegionRestrictionMode = regionMode
			routes[index].RegionRestrictionCountries = regionCountries
			routes[index].RegionRestrictionEnabled = routes[index].RegionRestrictionEnabled && len(regionCountries) > 0
		} else {
			routes[index].RegionRestrictionEnabled = false
			routes[index].RegionRestrictionCountries = []string{}
			routes[index].RegionRestrictionMode = proxyRouteRegionModeBlock
		}
	}
	return routes
}

func flattenSnapshotRoutesBySite(routes []snapshotRoute) map[string]snapshotRoute {
	siteMap := make(map[string]snapshotRoute)
	for _, route := range normalizeSnapshotRoutes(routes) {
		siteMap[route.SiteName] = route
	}
	return siteMap
}

func flattenSnapshotRoutesByDomain(routes []snapshotRoute) map[string]snapshotRoute {
	domainMap := make(map[string]snapshotRoute)
	for _, route := range normalizeSnapshotRoutes(routes) {
		for _, domain := range route.Domains {
			item := route
			item.Domain = domain
			domainMap[domain] = item
		}
	}
	return domainMap
}

func snapshotRouteConfigEqual(left snapshotRoute, right snapshotRoute) bool {
	if left.SiteName != right.SiteName || left.Domain != right.Domain || left.OriginURL != right.OriginURL || left.OriginHost != right.OriginHost || normalizeNodePoolName(left.NodePool) != normalizeNodePoolName(right.NodePool) || left.EnableHTTPS != right.EnableHTTPS || left.RedirectHTTP != right.RedirectHTTP || left.LimitConnPerServer != right.LimitConnPerServer || left.LimitConnPerIP != right.LimitConnPerIP || left.LimitRate != right.LimitRate || normalizeProxyRouteProxyBufferingMode(left.ProxyBufferingMode) != normalizeProxyRouteProxyBufferingMode(right.ProxyBufferingMode) || left.CacheEnabled != right.CacheEnabled || left.CachePolicy != right.CachePolicy || left.PoWEnabled != right.PoWEnabled || left.WAFEnabled != right.WAFEnabled || left.CCEnabled != right.CCEnabled || left.BasicAuthEnabled != right.BasicAuthEnabled || left.BasicAuthUsername != right.BasicAuthUsername || left.BasicAuthPasswordHash != right.BasicAuthPasswordHash || left.RegionRestrictionEnabled != right.RegionRestrictionEnabled || !uintSliceEqual(left.CertIDs, right.CertIDs) || !uintSliceEqual(left.DomainCertIDs, right.DomainCertIDs) {
		return false
	}
	if len(left.Domains) != len(right.Domains) {
		return false
	}
	for index := range left.Domains {
		if left.Domains[index] != right.Domains[index] {
			return false
		}
	}
	if len(left.Upstreams) != len(right.Upstreams) {
		return false
	}
	for index := range left.Upstreams {
		if left.Upstreams[index] != right.Upstreams[index] {
			return false
		}
	}
	if len(left.CacheRules) != len(right.CacheRules) {
		return false
	}
	for index := range left.CacheRules {
		if left.CacheRules[index] != right.CacheRules[index] {
			return false
		}
	}
	if len(left.CustomHeaders) != len(right.CustomHeaders) {
		return false
	}
	for index := range left.CustomHeaders {
		if left.CustomHeaders[index] != right.CustomHeaders[index] {
			return false
		}
	}
	if left.RegionRestrictionEnabled {
		if left.RegionRestrictionMode != right.RegionRestrictionMode {
			return false
		}
		if len(left.RegionRestrictionCountries) != len(right.RegionRestrictionCountries) {
			return false
		}
		for index := range left.RegionRestrictionCountries {
			if left.RegionRestrictionCountries[index] != right.RegionRestrictionCountries[index] {
				return false
			}
		}
	}
	if !snapshotPoWConfigEqual(left.PoWConfig, right.PoWConfig) {
		return false
	}
	if !snapshotWAFConfigEqual(left.WAFEnabled, left.WAFMode, left.WAFConfig, right.WAFEnabled, right.WAFMode, right.WAFConfig) {
		return false
	}
	if !snapshotCCConfigEqual(left.CCEnabled, left.CCMode, left.CCConfig, right.CCEnabled, right.CCMode, right.CCConfig) {
		return false
	}
	return true
}

func snapshotRoutesStateChanged(left []snapshotRoute, right []snapshotRoute) bool {
	if len(left) != len(right) {
		return true
	}
	normalizedLeft := normalizeSnapshotRoutes(append([]snapshotRoute{}, left...))
	normalizedRight := normalizeSnapshotRoutes(append([]snapshotRoute{}, right...))
	sort.Slice(normalizedLeft, func(i, j int) bool {
		return normalizedLeft[i].SiteName < normalizedLeft[j].SiteName
	})
	sort.Slice(normalizedRight, func(i, j int) bool {
		return normalizedRight[i].SiteName < normalizedRight[j].SiteName
	})
	for index := range normalizedLeft {
		if !snapshotRouteConfigEqual(normalizedLeft[index], normalizedRight[index]) {
			return true
		}
		if strings.TrimSpace(normalizedLeft[index].Remark) != strings.TrimSpace(normalizedRight[index].Remark) {
			return true
		}
	}
	return false
}

func snapshotPoWConfigEqual(left *ProxyRoutePoWConfig, right *ProxyRoutePoWConfig) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Difficulty == right.Difficulty &&
		left.Algorithm == right.Algorithm &&
		left.SessionTTL == right.SessionTTL &&
		left.ChallengeTTL == right.ChallengeTTL &&
		stringSliceEqual(left.Whitelist.IPs, right.Whitelist.IPs) &&
		stringSliceEqual(left.Whitelist.IPCidrs, right.Whitelist.IPCidrs) &&
		stringSliceEqual(left.Whitelist.Paths, right.Whitelist.Paths) &&
		stringSliceEqual(left.Whitelist.PathRegexes, right.Whitelist.PathRegexes) &&
		stringSliceEqual(left.Whitelist.UserAgents, right.Whitelist.UserAgents) &&
		stringSliceEqual(left.Blacklist.IPs, right.Blacklist.IPs) &&
		stringSliceEqual(left.Blacklist.IPCidrs, right.Blacklist.IPCidrs) &&
		stringSliceEqual(left.Blacklist.Paths, right.Blacklist.Paths) &&
		stringSliceEqual(left.Blacklist.PathRegexes, right.Blacklist.PathRegexes) &&
		stringSliceEqual(left.Blacklist.UserAgents, right.Blacklist.UserAgents)
}

func snapshotWAFConfigEqual(leftEnabled bool, leftMode string, left *ProxyRouteWAFConfig, rightEnabled bool, rightMode string, right *ProxyRouteWAFConfig) bool {
	if !leftEnabled && !rightEnabled {
		return true
	}
	if leftEnabled != rightEnabled || normalizeWAFMode(leftMode) != normalizeWAFMode(rightMode) {
		return false
	}
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return stringSliceEqual(left.BuiltinRules, right.BuiltinRules) &&
		stringSliceEqual(left.Whitelist.IPs, right.Whitelist.IPs) &&
		stringSliceEqual(left.Whitelist.IPCidrs, right.Whitelist.IPCidrs) &&
		stringSliceEqual(left.Whitelist.Paths, right.Whitelist.Paths) &&
		stringSliceEqual(left.BlockRules.PathContains, right.BlockRules.PathContains) &&
		stringSliceEqual(left.BlockRules.PathRegexes, right.BlockRules.PathRegexes) &&
		stringSliceEqual(left.BlockRules.QueryContains, right.BlockRules.QueryContains) &&
		stringSliceEqual(left.BlockRules.HeaderContains, right.BlockRules.HeaderContains) &&
		stringSliceEqual(left.BlockRules.UserAgents, right.BlockRules.UserAgents)
}

func snapshotCCConfigEqual(leftEnabled bool, leftMode string, left *ProxyRouteCCConfig, rightEnabled bool, rightMode string, right *ProxyRouteCCConfig) bool {
	if !leftEnabled && !rightEnabled {
		return true
	}
	if leftEnabled != rightEnabled || normalizeCCMode(leftMode) != normalizeCCMode(rightMode) {
		return false
	}
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.WindowSeconds == right.WindowSeconds &&
		left.MaxRequests == right.MaxRequests &&
		left.PathWindowSeconds == right.PathWindowSeconds &&
		left.PathMaxRequests == right.PathMaxRequests &&
		left.BlockDurationSeconds == right.BlockDurationSeconds &&
		stringSliceEqual(left.Whitelist.IPs, right.Whitelist.IPs) &&
		stringSliceEqual(left.Whitelist.IPCidrs, right.Whitelist.IPCidrs) &&
		stringSliceEqual(left.Whitelist.Paths, right.Whitelist.Paths) &&
		stringSliceEqual(left.Whitelist.UserAgents, right.Whitelist.UserAgents) &&
		stringSliceEqual(left.Exclude.IPs, right.Exclude.IPs) &&
		stringSliceEqual(left.Exclude.IPCidrs, right.Exclude.IPCidrs) &&
		stringSliceEqual(left.Exclude.Paths, right.Exclude.Paths) &&
		stringSliceEqual(left.Exclude.UserAgents, right.Exclude.UserAgents)
}

func stringSliceEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func buildOpenRestyConfigSnapshot() openRestyConfigSnapshot {
	return openRestyConfigSnapshot{
		WorkerProcesses:          common.OpenRestyWorkerProcesses,
		WorkerConnections:        common.OpenRestyWorkerConnections,
		WorkerRlimitNofile:       common.OpenRestyWorkerRlimitNofile,
		EventsUse:                common.OpenRestyEventsUse,
		EventsMultiAcceptEnabled: common.OpenRestyEventsMultiAcceptEnabled,
		KeepaliveTimeout:         common.OpenRestyKeepaliveTimeout,
		KeepaliveRequests:        common.OpenRestyKeepaliveRequests,
		ClientHeaderTimeout:      common.OpenRestyClientHeaderTimeout,
		ClientBodyTimeout:        common.OpenRestyClientBodyTimeout,
		ClientMaxBodySize:        common.OpenRestyClientMaxBodySize,
		LargeClientHeaderBuffers: common.OpenRestyLargeClientHeaderBuffers,
		SendTimeout:              common.OpenRestySendTimeout,
		ProxyConnectTimeout:      common.OpenRestyProxyConnectTimeout,
		ProxySendTimeout:         common.OpenRestyProxySendTimeout,
		ProxyReadTimeout:         common.OpenRestyProxyReadTimeout,
		WebsocketEnabled:         common.OpenRestyWebsocketEnabled,
		ProxyRequestBuffering:    common.OpenRestyProxyRequestBufferingEnabled,
		ProxyBufferingEnabled:    common.OpenRestyProxyBufferingEnabled,
		ProxyBuffers:             common.OpenRestyProxyBuffers,
		ProxyBufferSize:          common.OpenRestyProxyBufferSize,
		ProxyBusyBuffersSize:     common.OpenRestyProxyBusyBuffersSize,
		GzipEnabled:              common.OpenRestyGzipEnabled,
		GzipMinLength:            common.OpenRestyGzipMinLength,
		GzipCompLevel:            common.OpenRestyGzipCompLevel,
		Resolvers:                common.OpenRestyResolvers,
		CacheEnabled:             common.OpenRestyCacheEnabled,
		CachePath:                common.OpenRestyCachePath,
		CacheLevels:              common.OpenRestyCacheLevels,
		CacheInactive:            common.OpenRestyCacheInactive,
		CacheMaxSize:             common.OpenRestyCacheMaxSize,
		CacheKeyTemplate:         common.OpenRestyCacheKeyTemplate,
		CacheLockEnabled:         common.OpenRestyCacheLockEnabled,
		CacheLockTimeout:         common.OpenRestyCacheLockTimeout,
		CacheUseStale:            common.OpenRestyCacheUseStale,
	}
}

func diffOpenRestyOptionKeys(left openRestyConfigSnapshot, right openRestyConfigSnapshot) []string {
	return configversion.ExtractOptionDiffKeys(diffOpenRestyOptionDetails(left, right))
}

func buildInitialOpenRestyOptionDiffs(current openRestyConfigSnapshot) []ConfigOptionDiffItem {
	return configversion.BuildInitialOpenRestyOptionDiffs(openresty.ConfigSnapshot(current))
}

func diffOpenRestyOptionDetails(left openRestyConfigSnapshot, right openRestyConfigSnapshot) []ConfigOptionDiffItem {
	return configversion.DiffOpenRestyOptionDetails(openresty.ConfigSnapshot(left), openresty.ConfigSnapshot(right))
}

func extractOptionDiffKeys(details []ConfigOptionDiffItem) []string {
	return configversion.ExtractOptionDiffKeys(details)
}

func openRestyOptionKeys() []string {
	return configversion.OpenRestyOptionKeys()
}

func renderRouteConfig(routes []*model.ProxyRoute, cfg openRestyConfigSnapshot) (string, []SupportFile, error) {
	return renderRouteConfigWithQueries(routes, cfg, defaultRouteConfigQueries)
}

func renderRouteConfigWithContext(
	routes []*model.ProxyRoute,
	cfg openRestyConfigSnapshot,
	context proxyRouteTLSCertificateLoader,
) (string, []SupportFile, error) {
	if context == nil {
		return renderRouteConfig(routes, cfg)
	}
	renderRoutes, err := buildOpenRestyRenderRoutes(routes, context)
	if err != nil {
		return "", nil, err
	}
	return openresty.RenderRouteConfig(renderRoutes, openresty.ConfigSnapshot(cfg), openRestyRenderOptions())
}

type routeConfigQueries struct {
	ListTLSCertificatesByIDs func([]uint) ([]*model.TLSCertificate, error)
}

var defaultRouteConfigQueries = routeConfigQueries{
	ListTLSCertificatesByIDs: model.ListTLSCertificatesByIDs,
}

type routeConfigRenderContext struct {
	certificatesByID map[uint]*model.TLSCertificate
}

func openRestyRenderOptions() openresty.RenderOptions {
	return openresty.RenderOptions{LookupIPAddr: routeUpstreamLookupIPAddr}
}

func buildOpenRestyRenderRoutes(routes []*model.ProxyRoute, context proxyRouteTLSCertificateLoader) ([]openresty.Route, error) {
	result := make([]openresty.Route, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, fmt.Errorf("route %s domains are invalid", route.Domain)
		}
		customHeaders, err := decodeStoredCustomHeaders(route.CustomHeaders)
		if err != nil {
			return nil, fmt.Errorf("璺敱 %s 鑷畾涔夎姹傚ご鏃犳晥", route.Domain)
		}
		upstreams, err := decodeStoredUpstreams(route.Upstreams, route.OriginURL)
		if err != nil {
			return nil, fmt.Errorf("璺敱 %s 婧愮珯閰嶇疆鏃犳晥", route.Domain)
		}
		cacheRules, err := decodeStoredCacheRules(route.CacheRules)
		if err != nil {
			return nil, fmt.Errorf("璺敱 %s 缂撳瓨瑙勫垯鏃犳晥", route.Domain)
		}
		regionCountries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return nil, fmt.Errorf("璺敱 %s 鍦板尯闄愬埗閰嶇疆鏃犳晥", route.Domain)
		}
		renderRoute := openresty.Route{
			ID:                         route.ID,
			SiteName:                   route.SiteName,
			Domain:                     route.Domain,
			Domains:                    domains,
			OriginURL:                  route.OriginURL,
			OriginHost:                 route.OriginHost,
			Upstreams:                  upstreams,
			EnableHTTPS:                route.EnableHTTPS,
			CertID:                     route.CertID,
			RedirectHTTP:               route.RedirectHTTP,
			LimitConnPerServer:         route.LimitConnPerServer,
			LimitConnPerIP:             route.LimitConnPerIP,
			LimitRate:                  route.LimitRate,
			ProxyBufferingMode:         normalizeProxyRouteProxyBufferingMode(route.ProxyBufferingMode),
			CacheEnabled:               route.CacheEnabled,
			CachePolicy:                route.CachePolicy,
			CacheRules:                 cacheRules,
			CustomHeaders:              toOpenRestyCustomHeaders(customHeaders),
			PoWEnabled:                 route.PoWEnabled,
			WAFEnabled:                 route.WAFEnabled,
			WAFMode:                    normalizeWAFMode(route.WAFMode),
			CCEnabled:                  route.CCEnabled,
			CCMode:                     normalizeCCMode(route.CCMode),
			BasicAuthEnabled:           route.BasicAuthEnabled,
			BasicAuthUsername:          route.BasicAuthUsername,
			BasicAuthPasswordHash:      proxyRouteBasicAuthPasswordHashForView(route),
			RegionRestrictionEnabled:   route.RegionRestrictionEnabled && len(regionCountries) > 0,
			RegionRestrictionMode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
			RegionRestrictionCountries: regionCountries,
		}
		if route.EnableHTTPS {
			certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
			if err != nil {
				return nil, fmt.Errorf("route %s cert_ids are invalid: %w", route.Domain, err)
			}
			domainCertIDs, err := resolveProxyRouteDomainCertIDsWithContext(route, domains, certIDs, context)
			if err != nil {
				return nil, fmt.Errorf("route %s domain_cert_ids are invalid: %w", route.Domain, err)
			}
			certificates, err := loadTLSCertificatesWithContext(context, certIDs)
			if err != nil {
				return nil, fmt.Errorf("route %s certificate lookup failed: %w", route.Domain, err)
			}
			renderRoute.CertIDs = certIDs
			renderRoute.DomainCertIDs = domainCertIDs
			renderRoute.Certificates = toOpenRestyTLSCertificates(certificates)
		}
		result = append(result, renderRoute)
	}
	return result, nil
}

func toOpenRestyCustomHeaders(headers []ProxyRouteCustomHeaderInput) []openresty.CustomHeader {
	result := make([]openresty.CustomHeader, 0, len(headers))
	for _, header := range headers {
		result = append(result, openresty.CustomHeader{Key: header.Key, Value: header.Value})
	}
	return result
}

func toOpenRestyTLSCertificates(certificates []*model.TLSCertificate) []openresty.TLSCertificate {
	result := make([]openresty.TLSCertificate, 0, len(certificates))
	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		result = append(result, openresty.TLSCertificate{
			ID:      certificate.ID,
			CertPEM: certificate.CertPEM,
			KeyPEM:  certificate.KeyPEM,
		})
	}
	return result
}

func newRouteConfigRenderContext(routes []*model.ProxyRoute, queries routeConfigQueries) (*routeConfigRenderContext, error) {
	certIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for _, route := range routes {
		if route == nil || !route.EnableHTTPS {
			continue
		}
		routeCertIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
		if err != nil {
			continue
		}
		for _, certID := range routeCertIDs {
			if certID == 0 {
				continue
			}
			if _, ok := seen[certID]; ok {
				continue
			}
			seen[certID] = struct{}{}
			certIDs = append(certIDs, certID)
		}
	}
	if len(certIDs) == 0 {
		return &routeConfigRenderContext{certificatesByID: map[uint]*model.TLSCertificate{}}, nil
	}
	listCertificates := queries.ListTLSCertificatesByIDs
	if listCertificates == nil {
		listCertificates = model.ListTLSCertificatesByIDs
	}
	certificates, err := listCertificates(certIDs)
	if err != nil {
		return nil, err
	}
	certificatesByID := make(map[uint]*model.TLSCertificate, len(certificates))
	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		certificatesByID[certificate.ID] = certificate
	}
	return &routeConfigRenderContext{certificatesByID: certificatesByID}, nil
}

func (context *routeConfigRenderContext) loadTLSCertificates(certIDs []uint) ([]*model.TLSCertificate, error) {
	if context == nil {
		return loadTLSCertificates(certIDs)
	}
	certificates := make([]*model.TLSCertificate, 0, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		certificate := context.certificatesByID[certID]
		if certificate == nil {
			return nil, gorm.ErrRecordNotFound
		}
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func renderRouteConfigWithQueries(routes []*model.ProxyRoute, cfg openRestyConfigSnapshot, queries routeConfigQueries) (string, []SupportFile, error) {
	context, err := newRouteConfigRenderContext(routes, queries)
	if err != nil {
		return "", nil, err
	}
	renderRoutes, err := buildOpenRestyRenderRoutes(routes, context)
	if err != nil {
		return "", nil, err
	}
	return openresty.RenderRouteConfig(renderRoutes, openresty.ConfigSnapshot(cfg), openRestyRenderOptions())
}

func renderRouteConfigWithContextAndQueries(
	routes []*model.ProxyRoute,
	cfg openRestyConfigSnapshot,
	context proxyRouteTLSCertificateLoader,
) (string, []SupportFile, error) {
	var builder strings.Builder
	builder.WriteString("# This file is generated by DuShengCDN. Do not edit manually.\n")
	supportFiles := make([]SupportFile, 0)
	for _, route := range routes {
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return "", nil, fmt.Errorf("route %s domains are invalid", route.Domain)
		}
		serverNames := renderServerNames(domains)
		displayName := route.SiteName
		if strings.TrimSpace(displayName) == "" {
			displayName = domains[0]
		}
		customHeaders, err := decodeStoredCustomHeaders(route.CustomHeaders)
		if err != nil {
			return "", nil, fmt.Errorf("路由 %s 自定义请求头无效", route.Domain)
		}
		upstreams, err := decodeStoredUpstreams(route.Upstreams, route.OriginURL)
		if err != nil {
			return "", nil, fmt.Errorf("路由 %s 源站配置无效", route.Domain)
		}
		cacheRules, err := decodeStoredCacheRules(route.CacheRules)
		if err != nil {
			return "", nil, fmt.Errorf("路由 %s 缓存规则无效", route.Domain)
		}
		cacheConfig := routeCacheConfig{
			Enabled: route.CacheEnabled,
			Policy:  route.CachePolicy,
			Rules:   cacheRules,
		}
		limitConfig := routeLimitConfig{
			LimitConnPerServer: route.LimitConnPerServer,
			LimitConnPerIP:     route.LimitConnPerIP,
			LimitRate:          route.LimitRate,
		}
		proxyBufferingConfig := routeProxyBufferingConfig{
			Mode: normalizeProxyRouteProxyBufferingMode(route.ProxyBufferingMode),
		}
		regionCountries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return "", nil, fmt.Errorf("路由 %s 地区限制配置无效", route.Domain)
		}
		regionConfig := routeRegionRestrictionConfig{
			Enabled:   route.RegionRestrictionEnabled && len(regionCountries) > 0,
			Mode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
			Countries: regionCountries,
		}
		wafConfig := routeWAFConfig{
			Enabled: route.WAFEnabled,
			Mode:    normalizeWAFMode(route.WAFMode),
		}
		ccConfig := routeCCConfig{
			Enabled: route.CCEnabled,
			Mode:    normalizeCCMode(route.CCMode),
		}
		powRequired := route.PoWEnabled || (ccConfig.Enabled && ccConfig.Mode == proxyRouteCCModePoW)
		upstreamConfig, err := buildRouteUpstreamConfig(route, upstreams)
		if err != nil {
			return "", nil, fmt.Errorf("路由 %s 源站解析不安全: %w", route.Domain, err)
		}
		if upstreamConfig.UsesNamedUpstream {
			builder.WriteString(renderNamedUpstreamBlock(upstreamConfig))
		}
		basicAuthPasswordHash := proxyRouteBasicAuthPasswordHashForView(route)
		if !route.EnableHTTPS {
			builder.WriteString(renderHTTPProxyServer(serverNames, route.OriginURL, route.OriginHost, customHeaders, cacheConfig, limitConfig, proxyBufferingConfig, regionConfig, wafConfig, ccConfig, upstreamConfig, powRequired, route.BasicAuthEnabled, route.BasicAuthUsername, basicAuthPasswordHash, cfg))
			continue
		}
		certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
		if err != nil {
			return "", nil, fmt.Errorf("route %s cert_ids are invalid: %w", route.Domain, err)
		}
		domainCertIDs, err := resolveProxyRouteDomainCertIDsWithContext(route, domains, certIDs, context)
		if err != nil {
			return "", nil, fmt.Errorf("route %s domain_cert_ids are invalid: %w", route.Domain, err)
		}
		if route.CertID == nil || *route.CertID == 0 {
			return "", nil, fmt.Errorf("路由 %s 未配置证书", route.Domain)
		}
		if len(certIDs) == 0 {
			return "", nil, fmt.Errorf("路由 %s 未配置证书", route.Domain)
		}
		certificates, err := context.loadTLSCertificates(certIDs)
		if err != nil {
			return "", nil, fmt.Errorf("route %s certificate lookup failed: %w", route.Domain, err)
		}
		certificateByID := make(map[uint]*model.TLSCertificate, len(certificates))
		for _, certificate := range certificates {
			if certificate == nil {
				continue
			}
			certificateByID[certificate.ID] = certificate
			supportFiles = append(supportFiles,
				SupportFile{Path: certificateCertFileName(certificate.ID), Content: normalizePEM(certificate.CertPEM)},
				SupportFile{Path: certificateKeyFileName(certificate.ID), Content: normalizePEM(certificate.KeyPEM)},
			)
		}

		httpOnlyDomains := make([]string, 0, len(domains))
		domainsByCertID := make(map[uint][]string, len(certIDs))
		for index, domain := range domains {
			if index >= len(domainCertIDs) || domainCertIDs[index] == 0 {
				httpOnlyDomains = append(httpOnlyDomains, domain)
				continue
			}
			domainsByCertID[domainCertIDs[index]] = append(
				domainsByCertID[domainCertIDs[index]],
				domain,
			)
		}
		for _, certID := range certIDs {
			assignedDomains := domainsByCertID[certID]
			if len(assignedDomains) == 0 {
				continue
			}
			certificate := certificateByID[certID]
			if certificate == nil {
				return "", nil, fmt.Errorf("route %s certificate %d does not exist", route.Domain, certID)
			}
			if err := validateCertificateCoverage(certificate, assignedDomains); err != nil {
				return "", nil, fmt.Errorf("site %s certificate validation failed: %w", displayName, err)
			}
		}

		if route.RedirectHTTP {
			if len(httpOnlyDomains) > 0 {
				builder.WriteString(renderHTTPProxyServer(renderServerNames(httpOnlyDomains), route.OriginURL, route.OriginHost, customHeaders, cacheConfig, limitConfig, proxyBufferingConfig, regionConfig, wafConfig, ccConfig, upstreamConfig, powRequired, route.BasicAuthEnabled, route.BasicAuthUsername, basicAuthPasswordHash, cfg))
			}
			for _, certID := range certIDs {
				assignedDomains := domainsByCertID[certID]
				if len(assignedDomains) == 0 {
					continue
				}
				builder.WriteString(renderHTTPRedirectServer(renderServerNames(assignedDomains), regionConfig, wafConfig, ccConfig))
			}
		} else {
			builder.WriteString(renderHTTPProxyServer(serverNames, route.OriginURL, route.OriginHost, customHeaders, cacheConfig, limitConfig, proxyBufferingConfig, regionConfig, wafConfig, ccConfig, upstreamConfig, powRequired, route.BasicAuthEnabled, route.BasicAuthUsername, basicAuthPasswordHash, cfg))
		}
		for _, certID := range certIDs {
			assignedDomains := domainsByCertID[certID]
			if len(assignedDomains) == 0 {
				continue
			}
			builder.WriteString(renderHTTPSServer(renderServerNames(assignedDomains), route.OriginURL, route.OriginHost, certID, customHeaders, cacheConfig, limitConfig, proxyBufferingConfig, regionConfig, wafConfig, ccConfig, upstreamConfig, powRequired, route.BasicAuthEnabled, route.BasicAuthUsername, basicAuthPasswordHash, cfg))
		}
	}
	return builder.String(), dedupeSupportFiles(supportFiles), nil
}

func renderMainConfig(cfg openRestyConfigSnapshot) string {
	return openresty.RenderMainConfig(openresty.ConfigSnapshot(cfg))
}

func ValidateOpenRestyMainConfigTemplate(templateText string) error {
	trimmed := strings.TrimSpace(templateText)
	if trimmed == "" {
		return errors.New("OpenRestyMainConfigTemplate 不能为空")
	}
	for _, placeholder := range requiredMainConfigTemplatePlaceholders {
		if !strings.Contains(trimmed, placeholder) {
			return fmt.Errorf("OpenRestyMainConfigTemplate 必须保留占位符 %s", placeholder)
		}
	}
	return nil
}

func defaultOpenRestyMainConfigTemplate() string {
	return openresty.DefaultMainConfigTemplate()
}

func renderMainConfigTemplate(templateText string, cfg openRestyConfigSnapshot) string {
	replacer := strings.NewReplacer(
		"{{OpenRestyWorkerProcesses}}", cfg.WorkerProcesses,
		"{{OpenRestyWorkerConnections}}", fmt.Sprintf("%d", cfg.WorkerConnections),
		"{{OpenRestyWorkerRlimitNofile}}", fmt.Sprintf("%d", cfg.WorkerRlimitNofile),
		"{{OpenRestyConnectionUpgradeMap}}", renderConnectionUpgradeMap(),
		"{{OpenRestyDefaultServerBlock}}", renderDefaultServerBlock(),
		"{{OpenRestyAccessLogPath}}", nginxAccessLogPlaceholder,
		"{{OpenRestyEventsUseDirective}}", renderTemplateDirective(cfg.EventsUse != "", fmt.Sprintf("use %s;", cfg.EventsUse)),
		"{{OpenRestyEventsMultiAcceptDirective}}", renderTemplateDirective(cfg.EventsMultiAcceptEnabled, "multi_accept on;"),
		"{{OpenRestyKeepaliveTimeout}}", fmt.Sprintf("%d", cfg.KeepaliveTimeout),
		"{{OpenRestyKeepaliveRequests}}", fmt.Sprintf("%d", cfg.KeepaliveRequests),
		"{{OpenRestyClientHeaderTimeout}}", fmt.Sprintf("%d", cfg.ClientHeaderTimeout),
		"{{OpenRestyClientBodyTimeout}}", fmt.Sprintf("%d", cfg.ClientBodyTimeout),
		"{{OpenRestyClientMaxBodySize}}", cfg.ClientMaxBodySize,
		"{{OpenRestyLargeClientHeaderBuffers}}", cfg.LargeClientHeaderBuffers,
		"{{OpenRestySendTimeout}}", fmt.Sprintf("%d", cfg.SendTimeout),
		"{{OpenRestyProxyConnectTimeout}}", fmt.Sprintf("%d", cfg.ProxyConnectTimeout),
		"{{OpenRestyProxySendTimeout}}", fmt.Sprintf("%d", cfg.ProxySendTimeout),
		"{{OpenRestyProxyReadTimeout}}", fmt.Sprintf("%d", cfg.ProxyReadTimeout),
		"{{OpenRestyProxyRequestBuffering}}", onOff(cfg.ProxyRequestBuffering),
		"{{OpenRestyProxyBuffering}}", onOff(cfg.ProxyBufferingEnabled),
		"{{OpenRestyProxyBuffers}}", cfg.ProxyBuffers,
		"{{OpenRestyProxyBufferSize}}", cfg.ProxyBufferSize,
		"{{OpenRestyProxyBusyBuffersSize}}", cfg.ProxyBusyBuffersSize,
		"{{OpenRestyGzip}}", onOff(cfg.GzipEnabled),
		"{{OpenRestyGzipMinLength}}", fmt.Sprintf("%d", cfg.GzipMinLength),
		"{{OpenRestyGzipCompLevel}}", fmt.Sprintf("%d", cfg.GzipCompLevel),
		"{{OpenRestyResolverDirective}}", renderOpenRestyResolverDirective(cfg.Resolvers),
		"{{OpenRestyCacheBlock}}", renderOpenRestyCacheTemplateBlock(cfg),
		"{{OpenRestyRouteConfigInclude}}", nginxRouteConfigPlaceholder,
	)
	return replacer.Replace(templateText)
}

func renderTemplateDirective(enabled bool, statement string) string {
	if !enabled {
		return ""
	}
	return fmt.Sprintf("    %s\n", statement)
}

func renderOpenRestyResolverDirective(resolvers string) string {
	trimmed := strings.TrimSpace(resolvers)
	if trimmed != "" {
		return renderTemplateDirective(true, fmt.Sprintf("resolver %s;", trimmed))
	}
	return fmt.Sprintf("    %s\n", "__DUSHENGCDN_RESOLVER_DIRECTIVE__")
}

func renderOpenRestyCacheTemplateBlock(cfg openRestyConfigSnapshot) string {
	lines := make([]string, 0, 12)
	lines = append(lines, renderOpenRestyLimitZoneBlock())
	if !cfg.CacheEnabled {
		lines = append(lines, renderOpenRestyObservabilityTemplateBlock())
		return strings.Join(lines, "")
	}
	lines = append(lines, strings.Join([]string{
		fmt.Sprintf("    proxy_cache_path %s levels=%s keys_zone=dushengcdn_cache:10m inactive=%s max_size=%s;", cfg.CachePath, cfg.CacheLevels, cfg.CacheInactive, cfg.CacheMaxSize),
		fmt.Sprintf("    proxy_cache_key %s;", quoteNginxStringLiteral(cfg.CacheKeyTemplate)),
		fmt.Sprintf("    proxy_cache_lock %s;", onOff(cfg.CacheLockEnabled)),
		fmt.Sprintf("    proxy_cache_lock_timeout %s;", cfg.CacheLockTimeout),
		fmt.Sprintf("    proxy_cache_use_stale %s;", cfg.CacheUseStale),
		"",
	}, "\n"))
	lines = append(lines, renderOpenRestyObservabilityTemplateBlock())
	return strings.Join(lines, "")
}

func renderOpenRestyLimitZoneBlock() string {
	return strings.Join([]string{
		"    limit_conn_zone $server_name zone=dushengcdn_conn_per_server:10m;",
		"    limit_conn_zone $binary_remote_addr zone=dushengcdn_conn_per_ip:10m;",
		"",
	}, "\n")
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

const (
	nginxPowStaticDirPlaceholder    = "__DUSHENGCDN_POW_STATIC_DIR__"
	basicAuthCredentialHashMaterial = "dushengcdn basic auth v1\n"
)

func renderPowAccessBlock(powEnabled bool) string {
	return renderUnifiedAccessBlock(powEnabled)
}

func renderRouteAccessBlock(powEnabled bool, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return renderUnifiedAccessBlock(powEnabled || wafConfig.Enabled || ccConfig.Enabled || (regionConfig.Enabled && len(regionConfig.Countries) > 0))
}

func renderUnifiedAccessBlock(enabled bool) string {
	if !enabled {
		return ""
	}
	return fmt.Sprintf("    access_by_lua_file %s/access.lua;\n", nginxLuaDirPlaceholder)
}

func renderBasicAuthBlock(enabled bool, username, passwordHash string) string {
	expectedHash := strings.TrimSpace(passwordHash)
	if !enabled || username == "" || expectedHash == "" {
		return ""
	}
	return fmt.Sprintf(`        rewrite_by_lua_block {
            local expected_hash = "%s"
            local auth = ngx.var.http_authorization or ""
            local credential = nil
            if string.sub(auth, 1, 6) == "Basic " then
                credential = ngx.decode_base64(string.sub(auth, 7))
            end
            local ok = false
            if credential then
                local sha256 = require "resty.sha256"
                local str = require "resty.string"
                local hasher = sha256:new()
                hasher:update(%s)
                hasher:update(credential)
                ok = str.to_hex(hasher:final()) == expected_hash
            end
            if not ok then
                ngx.header["WWW-Authenticate"] = 'Basic realm="Restricted"'
                return ngx.exit(401)
            end
        }
`, expectedHash, luaStringLiteral(basicAuthCredentialHashMaterial))
}

func basicAuthCredentialHash(username, password string) string {
	return openresty.BasicAuthCredentialHash(username, password)
}

func renderRegionRestrictionBlock(config routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return renderUnifiedAccessBlock(wafConfig.Enabled || ccConfig.Enabled || (config.Enabled && len(config.Countries) > 0))
}

func renderPowLocationBlocks(powEnabled bool) string {
	if !powEnabled {
		return ""
	}
	return fmt.Sprintf("\n    location = %spass-challenge {\n        content_by_lua_file %s/pow/verify.lua;\n    }\n\n    location = %smake-challenge {\n        content_by_lua_file %s/pow/challenge.lua;\n    }\n\n", anubisAPIPrefix, nginxLuaDirPlaceholder, anubisAPIPrefix, nginxLuaDirPlaceholder)
}

func renderPowStaticLocationBlock(powEnabled bool) string {
	if !powEnabled {
		return ""
	}
	return fmt.Sprintf("    location %s {\n        alias %s/;\n        types {\n            text/css css;\n            application/javascript js mjs;\n            application/json json;\n            image/webp webp;\n            font/woff2 woff2;\n        }\n    }\n\n", anubisStaticPrefix, nginxPowStaticDirPlaceholder)
}

const anubisStaticPrefix = "/.within.website/x/cmd/anubis/static/"
const anubisAPIPrefix = "/.within.website/x/cmd/anubis/api/"

func normalizeSnapshotCertificateIDs(primaryCertID *uint, certIDs []uint) ([]uint, *uint, error) {
	candidates := make([]uint, 0, len(certIDs)+1)
	if primaryCertID != nil && *primaryCertID != 0 {
		candidates = append(candidates, *primaryCertID)
	}
	candidates = append(candidates, certIDs...)

	normalized := make([]uint, 0, len(candidates))
	seen := make(map[uint]struct{}, len(candidates))
	for _, certID := range candidates {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		normalized = append(normalized, certID)
	}

	var normalizedPrimary *uint
	if len(normalized) > 0 {
		normalizedPrimary = &normalized[0]
	}
	return normalized, normalizedPrimary, nil
}

func normalizeSnapshotDomainCertificateIDs(
	domains []string,
	certIDs []uint,
	domainCertIDs []uint,
) ([]uint, error) {
	if len(domainCertIDs) > 0 {
		if len(domains) > 0 && len(domainCertIDs) != len(domains) {
			return nil, errors.New("snapshot domain_cert_ids length is invalid")
		}
		normalized := make([]uint, len(domainCertIDs))
		copy(normalized, domainCertIDs)
		return normalized, nil
	}
	if len(certIDs) == 0 {
		return []uint{}, nil
	}
	if len(certIDs) == 1 {
		normalized := make([]uint, len(domains))
		for index := range normalized {
			normalized[index] = certIDs[0]
		}
		return normalized, nil
	}
	if len(certIDs) == len(domains) {
		normalized := make([]uint, len(certIDs))
		copy(normalized, certIDs)
		return normalized, nil
	}
	return []uint{}, nil
}

func uintPointerEqual(left *uint, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func uintSliceEqual(left []uint, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func checksum(content string) string {
	return configversion.Checksum(content)
}

func checksumBundle(mainConfig string, routeConfig string, supportFiles []SupportFile) string {
	return configversion.ChecksumBundle(mainConfig, routeConfig, supportFiles)
}

func configBundleRuntimeChanged(activeVersion *model.ConfigVersion, bundle *configBundle) (bool, error) {
	if activeVersion == nil || bundle == nil {
		return true, nil
	}
	if activeVersion.Checksum != bundle.Checksum {
		return true, nil
	}
	activeManifest, err := activeConfigVersionArtifactManifestChecksum(activeVersion)
	if err != nil {
		return false, err
	}
	currentManifest := configVersionArtifactBundleManifestChecksum(bundle.Artifacts)
	return activeManifest != currentManifest, nil
}

func activeConfigVersionArtifactManifestChecksum(version *model.ConfigVersion) (string, error) {
	if version == nil {
		return "", nil
	}
	if err := ensureConfigVersionArtifactsForPools(version, nil); err != nil {
		return "", err
	}
	artifacts, err := model.ListConfigVersionArtifacts(version.ID)
	if err != nil {
		return "", err
	}
	items := make([]configVersionArtifactManifestItem, 0, len(artifacts))
	for _, artifact := range artifacts {
		items = append(items, configVersionArtifactManifestItem{
			PoolName:            normalizeNodePoolName(artifact.PoolName),
			Checksum:            strings.TrimSpace(artifact.Checksum),
			MainConfigChecksum:  strings.TrimSpace(artifact.MainConfigChecksum),
			RouteConfigChecksum: strings.TrimSpace(artifact.RouteConfigChecksum),
			RouteCount:          artifact.RouteCount,
		})
	}
	return checksumConfigVersionArtifactManifest(items), nil
}

func configVersionArtifactBundleManifestChecksum(bundles []configVersionArtifactBundle) string {
	return configversion.ArtifactBundleManifestChecksum(bundles, normalizeNodePoolName)
}

type configVersionArtifactManifestItem = configversion.ArtifactManifestItem

func checksumConfigVersionArtifactManifest(items []configVersionArtifactManifestItem) string {
	return configversion.ChecksumArtifactManifest(items)
}

func nextVersionNumber(now time.Time) (string, error) {
	return configversion.NextVersionNumber(now)
}

func renderHTTPProxyServer(serverNames string, originURL string, originHost string, customHeaders []ProxyRouteCustomHeaderInput, cacheConfig routeCacheConfig, limitConfig routeLimitConfig, proxyBufferingConfig routeProxyBufferingConfig, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig, upstreamConfig routeUpstreamConfig, powEnabled bool, basicAuthEnabled bool, basicAuthUsername string, basicAuthPasswordHash string, cfg openRestyConfigSnapshot) string {
	return fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    set $dushengcdn_request_reason \"\";\n%s%s    location / {\n%s%s%s%s%s%s    }\n%s}\n\n", serverNames, renderRouteAccessBlock(powEnabled, regionConfig, wafConfig, ccConfig), renderPowLocationBlocks(powEnabled), renderBasicAuthBlock(basicAuthEnabled, basicAuthUsername, basicAuthPasswordHash), renderProxyHeaderBlock(originURL, originHost, customHeaders, upstreamConfig), renderRouteProxyBufferingBlock(proxyBufferingConfig), renderRouteLimitBlock(limitConfig), renderRouteCacheBlock(cacheConfig, cfg), renderProxyPassBlock(originURL, upstreamConfig), renderPowStaticLocationBlock(powEnabled))
}

func renderHTTPRedirectServer(serverNames string, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig) string {
	return fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    set $dushengcdn_request_reason \"\";\n%s\n    return 301 https://$host$request_uri;\n}\n\n", serverNames, renderRegionRestrictionBlock(regionConfig, wafConfig, ccConfig))
}

func renderHTTPSServer(serverNames string, originURL string, originHost string, certificateID uint, customHeaders []ProxyRouteCustomHeaderInput, cacheConfig routeCacheConfig, limitConfig routeLimitConfig, proxyBufferingConfig routeProxyBufferingConfig, regionConfig routeRegionRestrictionConfig, wafConfig routeWAFConfig, ccConfig routeCCConfig, upstreamConfig routeUpstreamConfig, powEnabled bool, basicAuthEnabled bool, basicAuthUsername string, basicAuthPasswordHash string, cfg openRestyConfigSnapshot) string {
	certPath := fmt.Sprintf("%s/%s", nginxCertDirPlaceholder, certificateCertFileName(certificateID))
	keyPath := fmt.Sprintf("%s/%s", nginxCertDirPlaceholder, certificateKeyFileName(certificateID))
	return fmt.Sprintf("server {\n    listen 443 ssl;\n    http2 on;\n    server_name %s;\n    ssl_certificate %s;\n    ssl_certificate_key %s;\n    set $dushengcdn_request_reason \"\";\n%s%s    location / {\n%s%s%s%s%s%s    }\n%s}\n\n", serverNames, certPath, keyPath, renderRouteAccessBlock(powEnabled, regionConfig, wafConfig, ccConfig), renderPowLocationBlocks(powEnabled), renderBasicAuthBlock(basicAuthEnabled, basicAuthUsername, basicAuthPasswordHash), renderProxyHeaderBlock(originURL, originHost, customHeaders, upstreamConfig), renderRouteProxyBufferingBlock(proxyBufferingConfig), renderRouteLimitBlock(limitConfig), renderRouteCacheBlock(cacheConfig, cfg), renderProxyPassBlock(originURL, upstreamConfig), renderPowStaticLocationBlock(powEnabled))
}

func renderServerNames(domains []string) string {
	return strings.Join(domains, " ")
}

func validateCertificateCoverage(certificate *model.TLSCertificate, domains []string) error {
	if certificate == nil {
		return errors.New("certificate is nil")
	}
	leaf, err := parseLeafCertificate(certificate.CertPEM)
	if err != nil {
		return err
	}
	for _, domain := range domains {
		if err := leaf.VerifyHostname(domain); err != nil {
			return fmt.Errorf("certificate does not cover domain %s", domain)
		}
	}
	return nil
}

func validateCertificateCoverageSet(certificates []*model.TLSCertificate, domains []string) error {
	if len(certificates) == 0 {
		return errors.New("certificate set is empty")
	}
	leaves := make([]interface{ VerifyHostname(string) error }, 0, len(certificates))
	for _, certificate := range certificates {
		if certificate == nil {
			return errors.New("certificate is nil")
		}
		leaf, err := parseLeafCertificate(certificate.CertPEM)
		if err != nil {
			return err
		}
		leaves = append(leaves, leaf)
	}
	for _, domain := range domains {
		covered := false
		for _, leaf := range leaves {
			if leaf.VerifyHostname(domain) == nil {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf("certificate does not cover domain %s", domain)
		}
	}
	return nil
}

func loadTLSCertificates(certIDs []uint) ([]*model.TLSCertificate, error) {
	uniqueIDs := make([]uint, 0, len(certIDs))
	seen := make(map[uint]struct{}, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		uniqueIDs = append(uniqueIDs, certID)
	}
	loaded, err := model.ListTLSCertificatesByIDs(uniqueIDs)
	if err != nil {
		return nil, err
	}
	certificatesByID := make(map[uint]*model.TLSCertificate, len(loaded))
	for _, certificate := range loaded {
		if certificate == nil {
			continue
		}
		certificatesByID[certificate.ID] = certificate
	}
	certificates := make([]*model.TLSCertificate, 0, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		certificate := certificatesByID[certID]
		if certificate == nil {
			return nil, gorm.ErrRecordNotFound
		}
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func renderConnectionUpgradeMap() string {
	return "    map $http_upgrade $connection_upgrade {\n        default upgrade;\n        ''      \"\";\n    }\n\n"
}

func renderDefaultServerBlock() string {
	return strings.Join([]string{
		"    server {",
		"        listen 80 default_server;",
		"        server_name _;",
		"        set $dushengcdn_request_reason \"\";",
		"",
		"        return 404;",
		"    }",
		"",
		"    server {",
		"        listen 443 ssl default_server;",
		"        server_name _;",
		"        set $dushengcdn_request_reason \"\";",
		"",
		"        ssl_reject_handshake on;",
		"    }",
		"",
	}, "\n")
}

func renderProxyHeaderBlock(originURL string, originHost string, customHeaders []ProxyRouteCustomHeaderInput, upstreamConfig routeUpstreamConfig) string {
	var builder strings.Builder
	if strings.TrimSpace(originHost) != "" {
		builder.WriteString(fmt.Sprintf("        proxy_set_header Host %s;\n", quoteNginxHeaderValue(originHost)))
	} else {
		builder.WriteString("        proxy_set_header Host $host;\n")
	}
	if upstreamServerName := resolveUpstreamServerName(originURL, originHost); upstreamServerName != "" {
		builder.WriteString("        proxy_ssl_server_name on;\n")
		builder.WriteString(fmt.Sprintf("        proxy_ssl_name %s;\n", quoteNginxHeaderValue(upstreamServerName)))
		builder.WriteString("        proxy_ssl_verify on;\n")
		builder.WriteString("        proxy_ssl_verify_depth 3;\n")
		builder.WriteString("        proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;\n")
	}
	builder.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	builder.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	builder.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	builder.WriteString("        proxy_next_upstream error timeout invalid_header http_500 http_502 http_503 http_504;\n")
	builder.WriteString("        proxy_next_upstream_tries 3;\n")
	builder.WriteString("        proxy_next_upstream_timeout 10s;\n")
	if common.OpenRestyWebsocketEnabled {
		builder.WriteString("        proxy_http_version 1.1;\n")
		builder.WriteString("        proxy_set_header Connection $connection_upgrade;\n")
		builder.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
	} else if upstreamConfig.UsesNamedUpstream {
		builder.WriteString("        proxy_http_version 1.1;\n")
		builder.WriteString("        proxy_set_header Connection \"\";\n")
	}
	for _, header := range customHeaders {
		builder.WriteString(fmt.Sprintf("        proxy_set_header %s %s;\n", header.Key, quoteNginxHeaderValue(header.Value)))
	}
	return builder.String()
}

func renderRouteCacheBlock(cacheConfig routeCacheConfig, cfg openRestyConfigSnapshot) string {
	if !cfg.CacheEnabled || !cacheConfig.Enabled {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("        set $dushengcdn_skip_cache 0;\n")
	builder.WriteString("        if ($request_method != GET) {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_authorization != \"\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_cookie ~* \"(session|sess|token|auth|jwt|logged_in|remember|laravel_session|connect\\\\.sid|_session)\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_cache_control ~* \"(no-cache|no-store|private)\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	builder.WriteString("        if ($http_range != \"\") {\n            set $dushengcdn_skip_cache 1;\n        }\n")
	if policyCondition := renderRouteCachePolicyCondition(cacheConfig); policyCondition != "" {
		builder.WriteString(policyCondition)
	}
	builder.WriteString("        proxy_cache dushengcdn_cache;\n")
	builder.WriteString("        proxy_cache_methods GET;\n")
	builder.WriteString("        proxy_cache_valid 200 301 302 10m;\n")
	builder.WriteString("        add_header X-DuShengCDN-Cache $upstream_cache_status always;\n")
	builder.WriteString("        proxy_cache_bypass $dushengcdn_skip_cache;\n")
	builder.WriteString("        proxy_no_cache $dushengcdn_skip_cache;\n")
	return builder.String()
}

func renderRouteProxyBufferingBlock(config routeProxyBufferingConfig) string {
	if normalizeProxyRouteProxyBufferingMode(config.Mode) != proxyRouteProxyBufferingModeOff {
		return ""
	}
	return "        proxy_buffering off;\n        proxy_request_buffering off;\n        proxy_max_temp_file_size 0;\n"
}

func renderRouteLimitBlock(limitConfig routeLimitConfig) string {
	if limitConfig.LimitConnPerServer <= 0 && limitConfig.LimitConnPerIP <= 0 && strings.TrimSpace(limitConfig.LimitRate) == "" {
		return ""
	}
	var builder strings.Builder
	if limitConfig.LimitConnPerServer > 0 {
		builder.WriteString(fmt.Sprintf("        limit_conn dushengcdn_conn_per_server %d;\n", limitConfig.LimitConnPerServer))
	}
	if limitConfig.LimitConnPerIP > 0 {
		builder.WriteString(fmt.Sprintf("        limit_conn dushengcdn_conn_per_ip %d;\n", limitConfig.LimitConnPerIP))
	}
	if strings.TrimSpace(limitConfig.LimitRate) != "" {
		builder.WriteString(fmt.Sprintf("        limit_rate %s;\n", limitConfig.LimitRate))
	}
	return builder.String()
}

func renderRouteCachePolicyCondition(cacheConfig routeCacheConfig) string {
	switch cacheConfig.Policy {
	case proxyRouteCachePolicySuffix:
		return fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", quoteNginxStringLiteral(buildSuffixMatchPattern(cacheConfig.Rules)))
	case proxyRouteCachePolicyPathPrefix:
		return fmt.Sprintf("        if ($uri !~ %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", quoteNginxStringLiteral(buildPathPrefixMatchPattern(cacheConfig.Rules)))
	case proxyRouteCachePolicyPathContains:
		return fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", quoteNginxStringLiteral(buildPathContainsMatchPattern(cacheConfig.Rules)))
	case proxyRouteCachePolicyPathContainsAll:
		return renderPathContainsAllCachePolicyCondition(cacheConfig.Rules)
	case proxyRouteCachePolicyPathExact:
		return fmt.Sprintf("        if ($uri !~ %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", quoteNginxStringLiteral(buildPathExactMatchPattern(cacheConfig.Rules)))
	default:
		return ""
	}
}

func renderPathContainsAllCachePolicyCondition(rules []string) string {
	var builder strings.Builder
	for _, rule := range rules {
		builder.WriteString(fmt.Sprintf("        if ($uri !~* %s) {\n            set $dushengcdn_skip_cache 1;\n        }\n", quoteNginxStringLiteral(regexp.QuoteMeta(rule))))
	}
	return builder.String()
}

func buildSuffixMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("\\.(?:%s)$", strings.Join(parts, "|"))
}

func buildPathPrefixMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		trimmed := strings.TrimRight(rule, "/")
		if trimmed == "" {
			trimmed = "/"
		}
		if trimmed == "/" {
			parts = append(parts, "/")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(?:/|$)", regexp.QuoteMeta(trimmed)))
	}
	return fmt.Sprintf("^(?:%s)", strings.Join(parts, "|"))
}

func buildPathContainsMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("(?:%s)", strings.Join(parts, "|"))
}

func buildPathExactMatchPattern(rules []string) string {
	parts := make([]string, 0, len(rules))
	for _, rule := range rules {
		parts = append(parts, regexp.QuoteMeta(rule))
	}
	return fmt.Sprintf("^(?:%s)$", strings.Join(parts, "|"))
}

func renderProxyPassBlock(originURL string, upstreamConfig routeUpstreamConfig) string {
	parsed, err := url.Parse(originURL)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return fmt.Sprintf("        proxy_pass %s;\n", originURL)
	}
	if upstreamConfig.UsesNamedUpstream {
		return fmt.Sprintf("        proxy_pass %s://%s%s;\n", upstreamConfig.Scheme, upstreamConfig.Name, upstreamConfig.ProxyPassURI)
	}
	return fmt.Sprintf("        proxy_pass %s;\n", originURL)
}

func buildRouteUpstreamConfig(route *model.ProxyRoute, upstreams []string) (routeUpstreamConfig, error) {
	if len(upstreams) == 0 {
		return routeUpstreamConfig{}, nil
	}
	if len(upstreams) == 1 {
		parsed, err := url.Parse(strings.TrimSpace(upstreams[0]))
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return routeUpstreamConfig{}, nil
		}
		servers, err := resolvePublicUpstreamServers(context.Background(), parsed.Scheme, parsed.Host)
		if err != nil {
			return routeUpstreamConfig{}, err
		}
		return routeUpstreamConfig{
			Name:              buildRouteUpstreamName(route),
			Scheme:            parsed.Scheme,
			ProxyPassURI:      buildUpstreamProxyPassURI(parsed),
			Servers:           servers,
			UsesNamedUpstream: true,
		}, nil
	}
	servers := make([]string, 0, len(upstreams))
	var scheme string
	for _, upstream := range upstreams {
		parsed, err := url.Parse(strings.TrimSpace(upstream))
		if err != nil || parsed.Host == "" || parsed.Scheme == "" {
			return routeUpstreamConfig{}, nil
		}
		if strings.TrimSpace(parsed.EscapedPath()) != "" && strings.TrimSpace(parsed.EscapedPath()) != "/" {
			return routeUpstreamConfig{}, nil
		}
		if parsed.RawQuery != "" {
			return routeUpstreamConfig{}, nil
		}
		if scheme == "" {
			scheme = parsed.Scheme
		} else if scheme != parsed.Scheme {
			return routeUpstreamConfig{}, nil
		}
		resolvedServers, err := resolvePublicUpstreamServers(context.Background(), parsed.Scheme, parsed.Host)
		if err != nil {
			return routeUpstreamConfig{}, err
		}
		servers = append(servers, resolvedServers...)
	}
	return routeUpstreamConfig{
		Name:              buildRouteUpstreamName(route),
		Scheme:            scheme,
		Servers:           servers,
		UsesNamedUpstream: true,
	}, nil
}

func buildUpstreamProxyPassURI(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := parsed.EscapedPath()
	if path == "/" {
		path = ""
	}
	if parsed.RawQuery == "" {
		return path
	}
	return fmt.Sprintf("%s?%s", path, parsed.RawQuery)
}

func resolvePublicUpstreamServers(ctx context.Context, scheme string, hostPort string) ([]string, error) {
	host, port := splitUpstreamHostPort(scheme, hostPort)
	normalizedPort, err := normalizeOriginPort(port)
	if err != nil {
		return nil, err
	}
	port = normalizedPort
	if err := security.ValidatePublicHostname(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if err := security.ValidatePublicIP(ip); err != nil {
			return nil, err
		}
		return []string{formatUpstreamServer(ip.String(), port)}, nil
	}
	addresses, err := routeUpstreamLookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("origin host %s has no addresses", host)
	}
	servers := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if err := security.ValidatePublicIP(address.IP); err != nil {
			return nil, fmt.Errorf("origin host %s resolved to unsafe ip: %w", host, err)
		}
		server := formatUpstreamServer(address.IP.String(), port)
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		servers = append(servers, server)
	}
	return servers, nil
}

func splitUpstreamHostPort(scheme string, hostPort string) (string, string) {
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		return host, port
	}
	parsed := &url.URL{Scheme: scheme, Host: hostPort}
	host = parsed.Hostname()
	port = parsed.Port()
	if port == "" {
		if strings.EqualFold(scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port
}

func formatUpstreamServer(host string, port string) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), port)
}

func buildRouteUpstreamName(route *model.ProxyRoute) string {
	identity := strings.TrimSpace(route.SiteName)
	if identity == "" {
		identity = route.Domain
	}
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, identity)
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		sanitized = "backend"
	}
	return fmt.Sprintf("backend_%s_%d", sanitized, route.ID)
}

func renderNamedUpstreamBlock(upstreamConfig routeUpstreamConfig) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("upstream %s {\n", upstreamConfig.Name))
	for _, server := range upstreamConfig.Servers {
		builder.WriteString(fmt.Sprintf("    server %s max_fails=3 fail_timeout=10s;\n", server))
	}
	builder.WriteString("    keepalive 128;\n}\n\n")
	return builder.String()
}

func resolveUpstreamServerName(originURL string, originHost string) string {
	parsed, err := url.Parse(originURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	if strings.TrimSpace(originHost) != "" {
		parsedHost, err := url.Parse("//" + originHost)
		if err == nil && parsedHost.Hostname() != "" {
			return parsedHost.Hostname()
		}
		return originHost
	}
	return parsed.Hostname()
}

func quoteNginxHeaderValue(value string) string {
	return openresty.QuoteNginxHeaderValue(value)
}

func quoteNginxStringLiteral(value string) string {
	return openresty.QuoteNginxStringLiteral(value)
}

func luaStringLiteral(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return fmt.Sprintf(`"%s"`, escaped)
}

func certificateCertFileName(id uint) string {
	return openresty.CertFileName(id)
}

func certificateKeyFileName(id uint) string {
	return openresty.KeyFileName(id)
}

func normalizePEM(content string) string {
	return openresty.NormalizePEM(content)
}

func dedupeSupportFiles(files []SupportFile) []SupportFile {
	return openresty.DedupeSupportFiles(files)
}

func renderPowConfigBundle(routes []*model.ProxyRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string               `json:"domains"`
		Enabled bool                   `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasPow := false
	for _, route := range routes {
		ccMode := normalizeCCMode(route.CCMode)
		ccRequiresPow := route.CCEnabled && ccMode == proxyRouteCCModePoW
		if !route.PoWEnabled && !ccRequiresPow {
			continue
		}
		hasPow = true
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return "", nil, err
		}
		var cfg map[string]interface{}
		powConfig := route.PoWConfig
		if strings.TrimSpace(powConfig) == "" || strings.TrimSpace(powConfig) == "{}" {
			defaultCfg := defaultPoWConfig()
			data, err := json.Marshal(defaultCfg)
			if err != nil {
				return "", nil, err
			}
			powConfig = string(data)
		}
		if err := json.Unmarshal([]byte(powConfig), &cfg); err != nil {
			return "", nil, fmt.Errorf("route %s pow_config is invalid", route.Domain)
		}
		if !route.PoWEnabled && ccRequiresPow {
			cfg["force_only"] = true
		}
		entries = append(entries, domainEntry{
			Domains: domains,
			Enabled: true,
			Config:  cfg,
		})
	}
	if !hasPow {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderCCConfigBundle(routes []*model.ProxyRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string            `json:"domains"`
		Enabled bool                `json:"enabled"`
		Mode    string              `json:"mode"`
		Config  *ProxyRouteCCConfig `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasCC := false
	for _, route := range routes {
		if !route.CCEnabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return "", nil, err
		}
		cfg, err := decodeStoredCCConfig(route.CCEnabled, route.CCConfig)
		if err != nil {
			return "", nil, fmt.Errorf("route %s cc_config is invalid", route.Domain)
		}
		hasCC = true
		entries = append(entries, domainEntry{
			Domains: domains,
			Enabled: true,
			Mode:    normalizeCCMode(route.CCMode),
			Config:  cfg,
		})
	}
	if !hasCC {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderRegionConfigBundle(routes []*model.ProxyRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains   []string `json:"domains"`
		Enabled   bool     `json:"enabled"`
		Mode      string   `json:"mode"`
		Countries []string `json:"countries"`
	}
	entries := make([]domainEntry, 0)
	hasRegionRestriction := false
	for _, route := range routes {
		if !route.RegionRestrictionEnabled {
			continue
		}
		countries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return "", nil, err
		}
		if len(countries) == 0 {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return "", nil, err
		}
		hasRegionRestriction = true
		entries = append(entries, domainEntry{
			Domains:   domains,
			Enabled:   true,
			Mode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
			Countries: countries,
		})
	}
	if !hasRegionRestriction {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}

func renderWAFConfigBundle(routes []*model.ProxyRoute) (string, []SupportFile, error) {
	type domainEntry struct {
		Domains []string             `json:"domains"`
		Enabled bool                 `json:"enabled"`
		Mode    string               `json:"mode"`
		Config  *ProxyRouteWAFConfig `json:"config"`
	}
	entries := make([]domainEntry, 0)
	hasWAF := false
	for _, route := range routes {
		if !route.WAFEnabled {
			continue
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return "", nil, err
		}
		cfg, err := decodeStoredWAFConfig(route.WAFEnabled, route.WAFConfig)
		if err != nil {
			return "", nil, fmt.Errorf("route %s waf_config is invalid", route.Domain)
		}
		hasWAF = true
		entries = append(entries, domainEntry{
			Domains: domains,
			Enabled: true,
			Mode:    normalizeWAFMode(route.WAFMode),
			Config:  cfg,
		})
	}
	if !hasWAF {
		return "{}", nil, nil
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "", nil, err
	}
	return string(data), nil, nil
}
