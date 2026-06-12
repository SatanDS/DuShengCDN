package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"dushengcdn/model"
	"dushengcdn/service/configversion"
	"dushengcdn/service/openresty"
	"dushengcdn/utils/security"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
	OriginHostHeader           string                        `json:"origin_host_header,omitempty"`
	OriginSNI                  string                        `json:"origin_sni,omitempty"`
	OriginTLSVerify            *bool                         `json:"origin_tls_verify,omitempty"`
	OriginCABundle             string                        `json:"origin_ca_bundle,omitempty"`
	OriginResolveMode          string                        `json:"origin_resolve_mode,omitempty"`
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
	RouteRules                 []snapshotRouteRule           `json:"route_rules,omitempty"`
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

type snapshotRouteRule struct {
	ID                          uint                          `json:"id,omitempty"`
	Name                        string                        `json:"name,omitempty"`
	MatchType                   string                        `json:"match_type"`
	Path                        string                        `json:"path"`
	Priority                    int                           `json:"priority"`
	Enabled                     bool                          `json:"enabled"`
	OriginURL                   string                        `json:"origin_url,omitempty"`
	Upstreams                   []string                      `json:"upstreams,omitempty"`
	OriginHostHeader            string                        `json:"origin_host_header,omitempty"`
	OriginSNI                   string                        `json:"origin_sni,omitempty"`
	OriginTLSVerify             bool                          `json:"origin_tls_verify"`
	OriginCABundle              string                        `json:"origin_ca_bundle,omitempty"`
	OriginResolveMode           string                        `json:"origin_resolve_mode,omitempty"`
	LimitConnPerServer          int                           `json:"limit_conn_per_server,omitempty"`
	LimitConnPerIP              int                           `json:"limit_conn_per_ip,omitempty"`
	LimitRate                   string                        `json:"limit_rate,omitempty"`
	ProxyBufferingMode          string                        `json:"proxy_buffering_mode,omitempty"`
	CacheEnabled                bool                          `json:"cache_enabled,omitempty"`
	CachePolicy                 string                        `json:"cache_policy,omitempty"`
	CacheRules                  []string                      `json:"cache_rules,omitempty"`
	CustomHeaders               []ProxyRouteCustomHeaderInput `json:"custom_headers,omitempty"`
	BasicAuthEnabled            bool                          `json:"basic_auth_enabled,omitempty"`
	BasicAuthUsername           string                        `json:"basic_auth_username,omitempty"`
	BasicAuthPasswordConfigured bool                          `json:"basic_auth_password_configured,omitempty"`
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
	nginxLuaDirPlaceholder              = "__DUSHENGCDN_LUA_DIR__"
	nginxObservabilityListenPlaceholder = "__DUSHENGCDN_OBSERVABILITY_LISTEN__"
	nginxObservabilityPortPlaceholder   = "__DUSHENGCDN_OBSERVABILITY_PORT__"
)

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
		for ruleIndex := range doc.Routes[index].RouteRules {
			doc.Routes[index].RouteRules[ruleIndex].CustomHeaders = redactSensitiveCustomHeaders(doc.Routes[index].RouteRules[ruleIndex].CustomHeaders)
		}
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
		return matches[1] + openresty.QuoteNginxHeaderValue(redactedProxyRouteCustomHeaderValue) + matches[4]
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
	currentSnapshotRoutes := normalizeSnapshotRoutesCopy(bundle.SnapshotRoutes)
	activeSnapshotRoutes := activeSnapshot.Routes
	result.ActiveWebsiteCount = len(activeSnapshotRoutes)
	result.CurrentWebsiteCount = len(currentSnapshotRoutes)
	currentSiteMap := flattenSnapshotRoutesBySite(currentSnapshotRoutes)
	activeSiteMap := flattenSnapshotRoutesBySite(activeSnapshotRoutes)
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
	currentMap := flattenSnapshotRoutesByDomain(currentSnapshotRoutes)
	activeMap := flattenSnapshotRoutesByDomain(activeSnapshotRoutes)
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
	runtimeConfigChanged, err := configBundleRuntimeChangedReadOnly(activeVersion, bundle)
	if err != nil {
		return nil, err
	}
	result.RuntimeConfigChanged = runtimeConfigChanged
	result.ChangedOptionDetails = diffOpenRestyOptionDetails(activeSnapshot.OpenRestyConfig, bundle.OpenRestyConfig)
	result.ChangedOptionKeys = extractOptionDiffKeys(result.ChangedOptionDetails)
	result.SnapshotChanged = snapshotRoutesStateChanged(activeSnapshotRoutes, currentSnapshotRoutes) || len(result.ChangedOptionDetails) > 0
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
	return configBundleRuntimeChangedReadOnly(activeVersion, bundle)
}

func PublishConfigVersion(createdBy string, force bool) (*ReleaseResult, error) {
	return createConfigVersionRecord(createdBy, force, true)
}

func CreateInactiveConfigVersion(createdBy string, force bool) (*ReleaseResult, error) {
	return createConfigVersionRecord(createdBy, force, false)
}

func createConfigVersionRecord(createdBy string, force bool, activate bool) (*ReleaseResult, error) {
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
		IsActive:         activate,
		CreatedBy:        createdBy,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if activate {
			if err := tx.Model(&model.ConfigVersion{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
				return err
			}
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
	if activate {
		BroadcastAgentWSActiveConfigForVersion(record)
	}
	routeViews, err := buildProxyRouteViews(bundle.Routes)
	if err != nil {
		slog.Error("build proxy route views after config version commit failed", "config_version_id", record.ID, "version", record.Version, "active", activate, "error", err)
		routeViews = []*ProxyRouteView{}
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
		routeRules, err := buildSnapshotRouteRules(route, context)
		if err != nil {
			return nil, fmt.Errorf("route %s route_rules are invalid: %w", route.Domain, err)
		}
		regionEnabled := route.RegionRestrictionEnabled && len(regionCountries) > 0
		items = append(items, snapshotRoute{
			SiteName:                   normalizeProxyRouteSiteNameInput(route, route.SiteName, domains[0]),
			Domain:                     domains[0],
			Domains:                    domains,
			OriginURL:                  route.OriginURL,
			OriginHost:                 normalizeStoredOriginHostHeader(route),
			OriginHostHeader:           normalizeStoredOriginHostHeader(route),
			OriginSNI:                  strings.TrimSpace(route.OriginSNI),
			OriginTLSVerify:            boolPointer(normalizeStoredOriginTLSVerify(route)),
			OriginCABundle:             strings.TrimSpace(route.OriginCABundle),
			OriginResolveMode:          normalizeStoredOriginResolveMode(route.OriginResolveMode),
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
			RouteRules:                 routeRules,
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

func buildSnapshotRouteRules(route *model.ProxyRoute, context proxyRouteTLSCertificateLoader) ([]snapshotRouteRule, error) {
	if route == nil || route.ID == 0 {
		return nil, nil
	}
	configs, err := loadProxyRouteRuleConfigsWithContext(context, route)
	if err != nil {
		return nil, err
	}
	result := make([]snapshotRouteRule, 0, len(configs))
	for _, config := range configs {
		result = append(result, snapshotRouteRule{
			ID:                          config.Rule.ID,
			Name:                        config.Rule.Name,
			MatchType:                   config.Rule.MatchType,
			Path:                        config.Rule.Path,
			Priority:                    config.Rule.Priority,
			Enabled:                     config.Rule.Enabled,
			OriginURL:                   firstString(config.Upstreams),
			Upstreams:                   config.Upstreams,
			OriginHostHeader:            config.Rule.OriginHostHeader,
			OriginSNI:                   config.Rule.OriginSNI,
			OriginTLSVerify:             config.Rule.OriginTLSVerify,
			OriginCABundle:              config.Rule.OriginCABundle,
			OriginResolveMode:           config.Rule.OriginResolveMode,
			LimitConnPerServer:          config.Rule.LimitConnPerServer,
			LimitConnPerIP:              config.Rule.LimitConnPerIP,
			LimitRate:                   config.Rule.LimitRate,
			ProxyBufferingMode:          config.Rule.ProxyBufferingMode,
			CacheEnabled:                config.CachePolicy != nil && config.CachePolicy.Enabled,
			CachePolicy:                 routeRuleCachePolicyName(config.CachePolicy),
			CacheRules:                  config.CacheRules,
			CustomHeaders:               config.CustomHeaders,
			BasicAuthEnabled:            config.SecurityPolicy != nil && config.SecurityPolicy.BasicAuthEnabled,
			BasicAuthUsername:           routeRuleBasicAuthUsername(config.SecurityPolicy),
			BasicAuthPasswordConfigured: config.SecurityPolicy != nil && strings.TrimSpace(config.SecurityPolicy.BasicAuthPasswordHash) != "",
		})
	}
	return result, nil
}

type proxyRouteRuleConfig struct {
	Rule           model.ProxyRouteRule
	Upstreams      []string
	CustomHeaders  []ProxyRouteCustomHeaderInput
	CachePolicy    *model.CachePolicy
	CacheRules     []string
	SecurityPolicy *model.SecurityPolicy
}

// loadProxyRouteRuleConfigsByRouteIDs loads the rule configurations for all
// given routes with a fixed number of queries. Every requested route ID gets
// an entry in the result map, so callers can distinguish "preloaded with no
// rules" from "not preloaded".
func loadProxyRouteRuleConfigsByRouteIDs(routeIDs []uint) (map[uint][]proxyRouteRuleConfig, error) {
	result := make(map[uint][]proxyRouteRuleConfig, len(routeIDs))
	for _, routeID := range routeIDs {
		result[routeID] = nil
	}
	if len(routeIDs) == 0 {
		return result, nil
	}
	rules, err := model.ListProxyRouteRulesByRouteIDs(routeIDs)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return result, nil
	}
	groupIDs := make([]uint, 0, len(rules))
	cachePolicyIDs := make([]uint, 0, len(rules))
	securityPolicyIDs := make([]uint, 0, len(rules))
	for _, rule := range rules {
		if rule.OriginGroupID != 0 {
			groupIDs = append(groupIDs, rule.OriginGroupID)
		}
		if rule.CachePolicyID != nil && *rule.CachePolicyID != 0 {
			cachePolicyIDs = append(cachePolicyIDs, *rule.CachePolicyID)
		}
		if rule.SecurityPolicyID != nil && *rule.SecurityPolicyID != 0 {
			securityPolicyIDs = append(securityPolicyIDs, *rule.SecurityPolicyID)
		}
	}
	originServers, err := model.ListOriginServersByGroupIDs(groupIDs)
	if err != nil {
		return nil, err
	}
	upstreamsByGroupID := make(map[uint][]string)
	for _, server := range originServers {
		if strings.TrimSpace(server.URL) == "" {
			continue
		}
		upstreamsByGroupID[server.OriginGroupID] = append(upstreamsByGroupID[server.OriginGroupID], server.URL)
	}
	cachePolicies := make(map[uint]model.CachePolicy)
	if len(cachePolicyIDs) > 0 {
		var rows []model.CachePolicy
		if err := model.DB.Where("id IN ?", cachePolicyIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			cachePolicies[row.ID] = row
		}
	}
	securityPolicies := make(map[uint]model.SecurityPolicy)
	if len(securityPolicyIDs) > 0 {
		var rows []model.SecurityPolicy
		if err := model.DB.Where("id IN ?", securityPolicyIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			securityPolicies[row.ID] = row
		}
	}
	for _, rule := range rules {
		customHeaders, err := decodeStoredCustomHeaders(rule.CustomHeadersJSON)
		if err != nil {
			return nil, err
		}
		item := proxyRouteRuleConfig{
			Rule:          rule,
			Upstreams:     upstreamsByGroupID[rule.OriginGroupID],
			CustomHeaders: customHeaders,
		}
		if rule.CachePolicyID != nil {
			if policy, ok := cachePolicies[*rule.CachePolicyID]; ok {
				policyCopy := policy
				cacheRules, err := decodeStoredCacheRules(policy.RulesJSON)
				if err != nil {
					return nil, err
				}
				item.CachePolicy = &policyCopy
				item.CacheRules = cacheRules
			}
		}
		if rule.SecurityPolicyID != nil {
			if policy, ok := securityPolicies[*rule.SecurityPolicyID]; ok {
				policyCopy := policy
				item.SecurityPolicy = &policyCopy
			}
		}
		result[rule.ProxyRouteID] = append(result[rule.ProxyRouteID], item)
	}
	return result, nil
}

// loadPreloadedProxyRouteRuleConfigs serves rule configs from a preloaded
// per-route map, falling back to a direct query for routes outside it.
func loadPreloadedProxyRouteRuleConfigs(preloaded map[uint][]proxyRouteRuleConfig, route *model.ProxyRoute) ([]proxyRouteRuleConfig, error) {
	if route == nil || route.ID == 0 {
		return nil, nil
	}
	if preloaded != nil {
		if configs, ok := preloaded[route.ID]; ok {
			return configs, nil
		}
	}
	return loadProxyRouteRuleConfigs(route)
}

type proxyRouteRuleConfigLoader interface {
	loadProxyRouteRuleConfigs(route *model.ProxyRoute) ([]proxyRouteRuleConfig, error)
}

func loadProxyRouteRuleConfigsWithContext(context proxyRouteTLSCertificateLoader, route *model.ProxyRoute) ([]proxyRouteRuleConfig, error) {
	if loader, ok := context.(proxyRouteRuleConfigLoader); ok {
		return loader.loadProxyRouteRuleConfigs(route)
	}
	return loadProxyRouteRuleConfigs(route)
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func routeRuleCachePolicyName(policy *model.CachePolicy) string {
	if policy == nil {
		return ""
	}
	return strings.TrimSpace(policy.Policy)
}

func routeRuleBasicAuthUsername(policy *model.SecurityPolicy) string {
	if policy == nil {
		return ""
	}
	return strings.TrimSpace(policy.BasicAuthUsername)
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
		return &snapshotDocument{Routes: normalizeSnapshotRoutesCopy(routes)}, nil
	}
	var snapshot snapshotDocument
	if err := json.Unmarshal([]byte(text), &snapshot); err != nil {
		return nil, errors.New("历史版本快照格式不合法")
	}
	snapshot.Routes = normalizeSnapshotRoutesCopy(snapshot.Routes)
	return &snapshot, nil
}

func normalizeSnapshotRoutesCopy(routes []snapshotRoute) []snapshotRoute {
	if len(routes) == 0 {
		return []snapshotRoute{}
	}
	normalized := append([]snapshotRoute(nil), routes...)
	return normalizeSnapshotRoutes(normalized)
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
		if strings.TrimSpace(routes[index].OriginHostHeader) == "" {
			routes[index].OriginHostHeader = strings.TrimSpace(routes[index].OriginHost)
		}
		routes[index].OriginHostHeader = strings.TrimSpace(routes[index].OriginHostHeader)
		routes[index].OriginHost = routes[index].OriginHostHeader
		routes[index].OriginSNI = strings.TrimSpace(routes[index].OriginSNI)
		routes[index].OriginCABundle = strings.TrimSpace(routes[index].OriginCABundle)
		if routes[index].OriginTLSVerify == nil {
			routes[index].OriginTLSVerify = boolPointer(true)
		}
		routes[index].OriginResolveMode = normalizeStoredOriginResolveMode(routes[index].OriginResolveMode)
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

// flattenSnapshotRoutesBySite expects routes that have already been normalized.
func flattenSnapshotRoutesBySite(routes []snapshotRoute) map[string]snapshotRoute {
	siteMap := make(map[string]snapshotRoute)
	for _, route := range routes {
		siteMap[route.SiteName] = route
	}
	return siteMap
}

// flattenSnapshotRoutesByDomain expects routes that have already been normalized.
func flattenSnapshotRoutesByDomain(routes []snapshotRoute) map[string]snapshotRoute {
	domainMap := make(map[string]snapshotRoute)
	for _, route := range routes {
		for _, domain := range route.Domains {
			item := route
			item.Domain = domain
			domainMap[domain] = item
		}
	}
	return domainMap
}

func snapshotRouteConfigEqual(left snapshotRoute, right snapshotRoute) bool {
	if left.SiteName != right.SiteName || left.Domain != right.Domain || left.OriginURL != right.OriginURL || left.OriginHost != right.OriginHost || left.OriginHostHeader != right.OriginHostHeader || left.OriginSNI != right.OriginSNI || snapshotOriginTLSVerify(left) != snapshotOriginTLSVerify(right) || left.OriginCABundle != right.OriginCABundle || left.OriginResolveMode != right.OriginResolveMode || normalizeNodePoolName(left.NodePool) != normalizeNodePoolName(right.NodePool) || left.EnableHTTPS != right.EnableHTTPS || left.RedirectHTTP != right.RedirectHTTP || left.LimitConnPerServer != right.LimitConnPerServer || left.LimitConnPerIP != right.LimitConnPerIP || left.LimitRate != right.LimitRate || normalizeProxyRouteProxyBufferingMode(left.ProxyBufferingMode) != normalizeProxyRouteProxyBufferingMode(right.ProxyBufferingMode) || left.CacheEnabled != right.CacheEnabled || left.CachePolicy != right.CachePolicy || left.PoWEnabled != right.PoWEnabled || left.WAFEnabled != right.WAFEnabled || left.CCEnabled != right.CCEnabled || left.BasicAuthEnabled != right.BasicAuthEnabled || left.BasicAuthUsername != right.BasicAuthUsername || left.BasicAuthPasswordHash != right.BasicAuthPasswordHash || left.RegionRestrictionEnabled != right.RegionRestrictionEnabled || !uintSliceEqual(left.CertIDs, right.CertIDs) || !uintSliceEqual(left.DomainCertIDs, right.DomainCertIDs) {
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
	if !snapshotRouteRulesEqual(left.RouteRules, right.RouteRules) {
		return false
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

func snapshotRouteRulesEqual(left []snapshotRouteRule, right []snapshotRouteRule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !snapshotRouteRuleEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

// snapshotRouteRuleEqual ignores rule IDs: rule rows are recreated on every
// route save, so IDs differ between snapshots even when the configuration is
// unchanged.
func snapshotRouteRuleEqual(left snapshotRouteRule, right snapshotRouteRule) bool {
	if left.Name != right.Name || left.MatchType != right.MatchType || left.Path != right.Path ||
		left.Priority != right.Priority || left.Enabled != right.Enabled ||
		left.OriginURL != right.OriginURL || left.OriginHostHeader != right.OriginHostHeader ||
		left.OriginSNI != right.OriginSNI || left.OriginTLSVerify != right.OriginTLSVerify ||
		left.OriginCABundle != right.OriginCABundle || left.OriginResolveMode != right.OriginResolveMode ||
		left.LimitConnPerServer != right.LimitConnPerServer || left.LimitConnPerIP != right.LimitConnPerIP ||
		left.LimitRate != right.LimitRate ||
		normalizeProxyRouteProxyBufferingMode(left.ProxyBufferingMode) != normalizeProxyRouteProxyBufferingMode(right.ProxyBufferingMode) ||
		left.CacheEnabled != right.CacheEnabled || left.CachePolicy != right.CachePolicy ||
		left.BasicAuthEnabled != right.BasicAuthEnabled || left.BasicAuthUsername != right.BasicAuthUsername ||
		left.BasicAuthPasswordConfigured != right.BasicAuthPasswordConfigured {
		return false
	}
	if !stringSliceEqual(left.Upstreams, right.Upstreams) || !stringSliceEqual(left.CacheRules, right.CacheRules) {
		return false
	}
	if len(left.CustomHeaders) != len(right.CustomHeaders) {
		return false
	}
	for index := range left.CustomHeaders {
		if left.CustomHeaders[index] != right.CustomHeaders[index] {
			return false
		}
	}
	return true
}

func snapshotOriginTLSVerify(route snapshotRoute) bool {
	if route.OriginTLSVerify == nil {
		return true
	}
	return *route.OriginTLSVerify
}

func boolPointer(value bool) *bool {
	return &value
}

func snapshotRoutesStateChanged(left []snapshotRoute, right []snapshotRoute) bool {
	if len(left) != len(right) {
		return true
	}
	normalizedLeft := append([]snapshotRoute(nil), left...)
	normalizedRight := append([]snapshotRoute(nil), right...)
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
	return configBundleRuntimeChangedWithArtifactBackfill(activeVersion, bundle, true)
}

func configBundleRuntimeChangedReadOnly(activeVersion *model.ConfigVersion, bundle *configBundle) (bool, error) {
	return configBundleRuntimeChangedWithArtifactBackfill(activeVersion, bundle, false)
}

func configBundleRuntimeChangedWithArtifactBackfill(activeVersion *model.ConfigVersion, bundle *configBundle, backfillArtifacts bool) (bool, error) {
	if activeVersion == nil || bundle == nil {
		return true, nil
	}
	if activeVersion.Checksum != bundle.Checksum {
		return true, nil
	}
	activeManifest, err := activeConfigVersionArtifactManifestChecksum(activeVersion, backfillArtifacts)
	if err != nil {
		return false, err
	}
	currentManifest := configVersionArtifactBundleManifestChecksum(bundle.Artifacts)
	return activeManifest != currentManifest, nil
}

func activeConfigVersionArtifactManifestChecksum(version *model.ConfigVersion, persistMissing bool) (string, error) {
	if version == nil {
		return "", nil
	}
	if persistMissing {
		if err := ensureConfigVersionArtifactsForPools(version, nil); err != nil {
			return "", err
		}
	}
	artifacts, err := model.ListConfigVersionArtifacts(version.ID)
	if err != nil {
		return "", err
	}
	items := make([]configVersionArtifactManifestItem, 0, len(artifacts))
	existingPools := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		poolName := normalizeNodePoolName(artifact.PoolName)
		existingPools[poolName] = struct{}{}
		items = append(items, configVersionArtifactManifestItem{
			PoolName:            poolName,
			Checksum:            strings.TrimSpace(artifact.Checksum),
			MainConfigChecksum:  strings.TrimSpace(artifact.MainConfigChecksum),
			RouteConfigChecksum: strings.TrimSpace(artifact.RouteConfigChecksum),
			RouteCount:          artifact.RouteCount,
		})
	}
	if !persistMissing {
		compatibilityItems, err := missingCompatibilityArtifactManifestItems(version, existingPools)
		if err != nil {
			return "", err
		}
		items = append(items, compatibilityItems...)
	}
	return checksumConfigVersionArtifactManifest(items), nil
}

func missingCompatibilityArtifactManifestItems(version *model.ConfigVersion, existingPools map[string]struct{}) ([]configVersionArtifactManifestItem, error) {
	if version == nil {
		return nil, nil
	}
	pools, err := compatibilityArtifactPools(nil)
	if err != nil {
		return nil, err
	}
	mainConfigChecksum := checksum(version.MainConfig)
	routeConfigChecksum := checksum(version.RenderedConfig)
	routeCount := len(versionRoutesFromSnapshot(version.SnapshotJSON))
	items := make([]configVersionArtifactManifestItem, 0, len(pools))
	for _, poolName := range pools {
		poolName = normalizeNodePoolName(poolName)
		if poolName == "" {
			continue
		}
		if _, ok := existingPools[poolName]; ok {
			continue
		}
		items = append(items, configVersionArtifactManifestItem{
			PoolName:            poolName,
			Checksum:            strings.TrimSpace(version.Checksum),
			MainConfigChecksum:  mainConfigChecksum,
			RouteConfigChecksum: routeConfigChecksum,
			RouteCount:          routeCount,
		})
	}
	return items, nil
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
