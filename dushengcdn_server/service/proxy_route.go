package service

import (
	"dushengcdn/model"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

var proxyHeaderKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var proxyRouteLimitRatePattern = regexp.MustCompile(`^\d+(?:[kKmM])?$`)
var proxyRouteDomainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var proxyRouteRegionCountryPattern = regexp.MustCompile(`^[A-Z0-9]{2}$`)

const (
	proxyRouteCachePolicyURL          = "url"
	proxyRouteCachePolicySuffix       = "suffix"
	proxyRouteCachePolicyPathPrefix   = "path_prefix"
	proxyRouteCachePolicyPathContains = "path_contains"
	proxyRouteCachePolicyPathExact    = "path_exact"
	proxyRouteRegionModeAllow         = "allow"
	proxyRouteRegionModeBlock         = "block"
	proxyRouteWAFModeLog              = "log"
	proxyRouteWAFModeBlock            = "block"
	DNSProviderModeCloudflare         = "cloudflare"
	DNSProviderModeAuthoritative      = "authoritative"
)

type ProxyRouteCustomHeaderInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ProxyRouteInput struct {
	SiteName                   string                        `json:"site_name"`
	Domain                     string                        `json:"domain"`
	Domains                    []string                      `json:"domains"`
	OriginID                   *uint                         `json:"origin_id"`
	OriginURL                  string                        `json:"origin_url"`
	OriginScheme               string                        `json:"origin_scheme"`
	OriginAddress              string                        `json:"origin_address"`
	OriginPort                 string                        `json:"origin_port"`
	OriginURI                  string                        `json:"origin_uri"`
	OriginHost                 string                        `json:"origin_host"`
	Upstreams                  []string                      `json:"upstreams"`
	NodePool                   string                        `json:"node_pool"`
	Enabled                    bool                          `json:"enabled"`
	EnableHTTPS                bool                          `json:"enable_https"`
	CertID                     *uint                         `json:"cert_id"`
	CertIDs                    []uint                        `json:"cert_ids"`
	DomainCertIDs              []uint                        `json:"domain_cert_ids"`
	RedirectHTTP               bool                          `json:"redirect_http"`
	LimitConnPerServer         int                           `json:"limit_conn_per_server"`
	LimitConnPerIP             int                           `json:"limit_conn_per_ip"`
	LimitRate                  string                        `json:"limit_rate"`
	CacheEnabled               bool                          `json:"cache_enabled"`
	CachePolicy                string                        `json:"cache_policy"`
	CacheRules                 []string                      `json:"cache_rules"`
	CustomHeaders              []ProxyRouteCustomHeaderInput `json:"custom_headers"`
	PoWEnabled                 bool                          `json:"pow_enabled"`
	PoWConfig                  string                        `json:"pow_config"`
	WAFEnabled                 bool                          `json:"waf_enabled"`
	WAFMode                    string                        `json:"waf_mode"`
	WAFConfig                  string                        `json:"waf_config"`
	BasicAuthEnabled           bool                          `json:"basic_auth_enabled"`
	BasicAuthUsername          string                        `json:"basic_auth_username"`
	BasicAuthPassword          string                        `json:"basic_auth_password"`
	RegionRestrictionEnabled   bool                          `json:"region_restriction_enabled"`
	RegionRestrictionMode      string                        `json:"region_restriction_mode"`
	RegionRestrictionCountries []string                      `json:"region_restriction_countries"`
	DNSAutoSync                bool                          `json:"dns_auto_sync"`
	DNSAccountID               *uint                         `json:"dns_account_id"`
	DNSZoneID                  string                        `json:"dns_zone_id"`
	DNSRecordType              string                        `json:"dns_record_type"`
	DNSRecordName              string                        `json:"dns_record_name"`
	DNSRecordContent           string                        `json:"dns_record_content"`
	DNSAutoTarget              bool                          `json:"dns_auto_target"`
	DNSTargetCount             int                           `json:"dns_target_count"`
	DNSScheduleMode            string                        `json:"dns_schedule_mode"`
	DNSTTL                     int                           `json:"dns_ttl"`
	DNSProviderMode            string                        `json:"dns_provider_mode"`
	DNSZoneIDRef               *uint                         `json:"dns_zone_id_ref"`
	GSLBEnabled                bool                          `json:"gslb_enabled"`
	GSLBPolicy                 ProxyRouteGSLBPolicy          `json:"gslb_policy"`
	CloudflareProxied          bool                          `json:"cloudflare_proxied"`
	DDOSProtectionMode         string                        `json:"ddos_protection_mode"`
	DDOSProtectionProvider     string                        `json:"ddos_protection_provider"`
	DDOSProtectionTarget       string                        `json:"ddos_protection_target"`
	Remark                     string                        `json:"remark"`
}

type ProxyRouteView struct {
	ID                         uint                          `json:"id"`
	SiteName                   string                        `json:"site_name"`
	Domain                     string                        `json:"domain"`
	Domains                    []string                      `json:"domains"`
	PrimaryDomain              string                        `json:"primary_domain"`
	DomainCount                int                           `json:"domain_count"`
	OriginID                   *uint                         `json:"origin_id"`
	OriginURL                  string                        `json:"origin_url"`
	OriginHost                 string                        `json:"origin_host"`
	Upstreams                  string                        `json:"upstreams"`
	UpstreamList               []string                      `json:"upstream_list"`
	NodePool                   string                        `json:"node_pool"`
	Enabled                    bool                          `json:"enabled"`
	EnableHTTPS                bool                          `json:"enable_https"`
	CertID                     *uint                         `json:"cert_id"`
	CertIDs                    []uint                        `json:"cert_ids"`
	DomainCertIDs              []uint                        `json:"domain_cert_ids"`
	RedirectHTTP               bool                          `json:"redirect_http"`
	LimitConnPerServer         int                           `json:"limit_conn_per_server"`
	LimitConnPerIP             int                           `json:"limit_conn_per_ip"`
	LimitRate                  string                        `json:"limit_rate"`
	CacheEnabled               bool                          `json:"cache_enabled"`
	CachePolicy                string                        `json:"cache_policy"`
	CacheRules                 string                        `json:"cache_rules"`
	CacheRuleList              []string                      `json:"cache_rule_list"`
	CustomHeaders              string                        `json:"custom_headers"`
	CustomHeaderList           []ProxyRouteCustomHeaderInput `json:"custom_header_list"`
	PoWEnabled                 bool                          `json:"pow_enabled"`
	PoWConfig                  *ProxyRoutePoWConfig          `json:"pow_config"`
	WAFEnabled                 bool                          `json:"waf_enabled"`
	WAFMode                    string                        `json:"waf_mode"`
	WAFConfig                  *ProxyRouteWAFConfig          `json:"waf_config"`
	BasicAuthEnabled           bool                          `json:"basic_auth_enabled"`
	BasicAuthUsername          string                        `json:"basic_auth_username"`
	BasicAuthPassword          string                        `json:"basic_auth_password"`
	RegionRestrictionEnabled   bool                          `json:"region_restriction_enabled"`
	RegionRestrictionMode      string                        `json:"region_restriction_mode"`
	RegionRestrictionCountries []string                      `json:"region_restriction_countries"`
	DNSAutoSync                bool                          `json:"dns_auto_sync"`
	DNSAccountID               *uint                         `json:"dns_account_id"`
	DNSZoneID                  string                        `json:"dns_zone_id"`
	DNSRecordType              string                        `json:"dns_record_type"`
	DNSRecordName              string                        `json:"dns_record_name"`
	DNSRecordContent           string                        `json:"dns_record_content"`
	DNSAutoTarget              bool                          `json:"dns_auto_target"`
	DNSTargetCount             int                           `json:"dns_target_count"`
	DNSScheduleMode            string                        `json:"dns_schedule_mode"`
	DNSTTL                     int                           `json:"dns_ttl"`
	DNSProviderMode            string                        `json:"dns_provider_mode"`
	DNSZoneIDRef               *uint                         `json:"dns_zone_id_ref"`
	GSLBEnabled                bool                          `json:"gslb_enabled"`
	GSLBPolicy                 ProxyRouteGSLBPolicy          `json:"gslb_policy"`
	DNSRecordIDs               map[string]string             `json:"dns_record_ids"`
	CloudflareProxied          bool                          `json:"cloudflare_proxied"`
	DDOSProtectionMode         string                        `json:"ddos_protection_mode"`
	DDOSProtectionProvider     string                        `json:"ddos_protection_provider"`
	DDOSProtectionTarget       string                        `json:"ddos_protection_target"`
	DNSLastSyncStatus          string                        `json:"dns_last_sync_status"`
	DNSLastSyncMessage         string                        `json:"dns_last_sync_message"`
	DNSLastSyncedAt            *time.Time                    `json:"dns_last_synced_at"`
	Remark                     string                        `json:"remark"`
	CreatedAt                  time.Time                     `json:"created_at"`
	UpdatedAt                  time.Time                     `json:"updated_at"`
}

func ListProxyRoutes() ([]*ProxyRouteView, error) {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return nil, err
	}
	return buildProxyRouteViews(routes)
}

func GetProxyRoute(id uint) (*ProxyRouteView, error) {
	route, err := model.GetProxyRouteByID(id)
	if err != nil {
		return nil, err
	}
	return buildProxyRouteView(route)
}

func CreateProxyRoute(input ProxyRouteInput) (*ProxyRouteView, error) {
	route, err := buildProxyRoute(nil, input)
	if err != nil {
		return nil, err
	}
	if err = route.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("proxy route identity already exists")
		}
		return nil, err
	}
	if shouldSyncProxyRouteCloudflareDNS(route) {
		if err := SyncProxyRouteDNS(route); err != nil {
			_ = route.Delete()
			return nil, err
		}
	}
	return buildProxyRouteView(route)
}

func UpdateProxyRoute(id uint, input ProxyRouteInput) (*ProxyRouteView, error) {
	route, err := model.GetProxyRouteByID(id)
	if err != nil {
		return nil, err
	}
	route, err = buildProxyRoute(route, input)
	if err != nil {
		return nil, err
	}
	if err = route.Update(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("proxy route identity already exists")
		}
		return nil, err
	}
	if shouldSyncProxyRouteCloudflareDNS(route) {
		if err := SyncProxyRouteDNS(route); err != nil {
			return nil, err
		}
	}
	return buildProxyRouteView(route)
}

func DeleteProxyRoute(id uint) error {
	route, err := model.GetProxyRouteByID(id)
	if err != nil {
		return err
	}
	if shouldSyncProxyRouteCloudflareDNS(route) {
		if err := DeleteProxyRouteDNSRecords(route); err != nil {
			return err
		}
	}
	return route.Delete()
}

func buildProxyRoute(route *model.ProxyRoute, input ProxyRouteInput) (*model.ProxyRoute, error) {
	domains, err := normalizeProxyRouteDomainsInput(route, input.Domain, input.Domains)
	if err != nil {
		return nil, err
	}
	domain := domains[0]
	siteName := normalizeProxyRouteSiteNameInput(route, input.SiteName, domain)

	originURL, originID, err := resolveProxyRoutePrimaryOrigin(input)
	if err != nil {
		return nil, err
	}
	originHost := strings.TrimSpace(input.OriginHost)
	remark := strings.TrimSpace(input.Remark)
	upstreams, err := normalizeUpstreams(originURL, input.Upstreams)
	if err != nil {
		return nil, err
	}
	cachePolicy := strings.TrimSpace(input.CachePolicy)
	cacheRules, err := normalizeCacheRules(input.CacheEnabled, cachePolicy, input.CacheRules)
	if err != nil {
		return nil, err
	}
	customHeaders, err := normalizeCustomHeaders(input.CustomHeaders)
	if err != nil {
		return nil, err
	}
	limitConnPerServer, err := normalizeProxyRouteLimitConnValue(input.LimitConnPerServer, "limit_conn_per_server")
	if err != nil {
		return nil, err
	}
	limitConnPerIP, err := normalizeProxyRouteLimitConnValue(input.LimitConnPerIP, "limit_conn_per_ip")
	if err != nil {
		return nil, err
	}
	limitRate, err := normalizeProxyRouteLimitRate(input.LimitRate)
	if err != nil {
		return nil, err
	}

	cacheRulesJSON, err := json.Marshal(cacheRules)
	if err != nil {
		return nil, err
	}
	upstreamsJSON, err := json.Marshal(upstreams)
	if err != nil {
		return nil, err
	}
	customHeadersJSON, err := json.Marshal(customHeaders)
	if err != nil {
		return nil, err
	}
	regionMode, regionCountries, err := normalizeProxyRouteRegionRestriction(
		input.RegionRestrictionEnabled,
		input.RegionRestrictionMode,
		input.RegionRestrictionCountries,
	)
	if err != nil {
		return nil, err
	}
	regionCountriesJSON, err := json.Marshal(regionCountries)
	if err != nil {
		return nil, err
	}

	powConfig, err := normalizePoWConfig(input.PoWEnabled, input.PoWConfig)
	if err != nil {
		return nil, err
	}
	powConfigJSON, err := json.Marshal(powConfig)
	if err != nil {
		return nil, err
	}
	wafMode := normalizeWAFMode(input.WAFMode)
	wafConfig, err := normalizeWAFConfig(input.WAFEnabled, input.WAFConfig)
	if err != nil {
		return nil, err
	}
	wafConfigJSON, err := json.Marshal(wafConfig)
	if err != nil {
		return nil, err
	}

	if !input.EnableHTTPS {
		input.RedirectHTTP = false
		input.CertID = nil
		input.CertIDs = nil
		input.DomainCertIDs = nil
	}
	domainCertIDs, certIDs, primaryCertID, err := normalizeProxyRouteDomainCertificateIDs(
		domains,
		input.EnableHTTPS,
		input.DomainCertIDs,
		input.CertID,
		input.CertIDs,
	)
	if err != nil {
		return nil, err
	}
	if err := validateProxyRouteDomainCertificateCoverage(domains, domainCertIDs); err != nil {
		return nil, err
	}
	certIDsJSON, err := json.Marshal(certIDs)
	if err != nil {
		return nil, err
	}
	domainCertIDsJSON, err := json.Marshal(domainCertIDs)
	if err != nil {
		return nil, err
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, err
	}

	if err := validateProxyRouteSiteName(siteName); err != nil {
		return nil, err
	}
	if err := validateProxyRouteIdentityUniqueness(route, siteName, domains); err != nil {
		return nil, err
	}
	if err := validateOriginHost(originHost); err != nil {
		return nil, err
	}
	nodePool := normalizeNodePoolName(input.NodePool)
	input.DomainCertIDs = domainCertIDs
	input.CertIDs = certIDs
	input.CertID = primaryCertID
	if input.RedirectHTTP && !input.EnableHTTPS {
		return nil, errors.New("redirect_http requires enable_https")
	}

	if input.BasicAuthEnabled {
		input.BasicAuthUsername = strings.TrimSpace(input.BasicAuthUsername)
		input.BasicAuthPassword = strings.TrimSpace(input.BasicAuthPassword)
		if input.BasicAuthUsername == "" || input.BasicAuthPassword == "" {
			return nil, errors.New("basic_auth_username and basic_auth_password cannot be empty when basic auth is enabled")
		}
	} else {
		input.BasicAuthUsername = ""
		input.BasicAuthPassword = ""
	}

	dnsProviderMode := normalizeDNSProviderMode(input.DNSProviderMode)
	dnsAccountID, dnsZoneID, dnsRecordType, dnsRecordName, dnsRecordContent, dnsAutoTarget, dnsTargetCount, dnsScheduleMode, dnsTTL, gslbEnabled, gslbPolicy, ddosMode, ddosProvider, ddosTarget, err := normalizeProxyRouteDNSSettingsV3(input)
	if err != nil {
		return nil, err
	}
	dnsZoneIDRef, err := normalizeProxyRouteAuthoritativeZone(input, dnsProviderMode, domains)
	if err != nil {
		return nil, err
	}
	gslbPolicyJSON, err := json.Marshal(gslbPolicy)
	if err != nil {
		return nil, err
	}
	if dnsProviderMode == DNSProviderModeAuthoritative && dnsZoneIDRef != nil {
		shouldPrecheckAuthoritativeDNS := shouldPrecheckAuthoritativeDNSRoute(
			route,
			domain,
			string(domainsJSON),
			input.Enabled,
			dnsProviderMode,
			dnsZoneIDRef,
			nodePool,
			dnsRecordType,
			dnsRecordName,
			dnsRecordContent,
			dnsAutoTarget,
			dnsTargetCount,
			dnsScheduleMode,
			dnsTTL,
			gslbEnabled,
			string(gslbPolicyJSON),
		)
		if shouldPrecheckAuthoritativeDNS {
			if err := validateAuthoritativeProxyRouteStaticRecordConflicts(*dnsZoneIDRef, domains, dnsRecordType, input.Enabled); err != nil {
				return nil, err
			}
			precheckRoute := buildAuthoritativeDNSPrecheckProxyRoute(
				route,
				nodePool,
				dnsRecordType,
				dnsRecordName,
				dnsTargetCount,
				dnsScheduleMode,
				dnsTTL,
				gslbEnabled,
				string(gslbPolicyJSON),
				dnsZoneIDRef,
			)
			if _, err := precheckAuthoritativeRouteDNSTargets(precheckRoute, dnsRecordType); err != nil {
				return nil, err
			}
		}
	}
	if input.Enabled && (dnsProviderMode == DNSProviderModeAuthoritative || input.DNSAutoSync) && gslbEnabled {
		if err := validateGSLBPolicyPoolTargets(gslbPolicy, dnsRecordType); err != nil {
			return nil, err
		}
	}

	if route == nil {
		route = &model.ProxyRoute{}
	}
	route.SiteName = siteName
	route.Domain = domain
	route.Domains = string(domainsJSON)
	route.OriginID = originID
	route.OriginURL = upstreams[0]
	route.OriginHost = originHost
	route.Upstreams = string(upstreamsJSON)
	route.NodePool = nodePool
	route.Enabled = input.Enabled
	route.EnableHTTPS = input.EnableHTTPS
	route.CertID = input.CertID
	route.CertIDs = string(certIDsJSON)
	route.DomainCertIDs = string(domainCertIDsJSON)
	route.RedirectHTTP = input.RedirectHTTP
	route.LimitConnPerServer = limitConnPerServer
	route.LimitConnPerIP = limitConnPerIP
	route.LimitRate = limitRate
	route.CacheEnabled = input.CacheEnabled
	route.CachePolicy = normalizeCachePolicy(input.CacheEnabled, cachePolicy)
	route.CacheRules = string(cacheRulesJSON)
	route.CustomHeaders = string(customHeadersJSON)
	route.PoWEnabled = input.PoWEnabled
	route.PoWConfig = string(powConfigJSON)
	route.WAFEnabled = input.WAFEnabled
	route.WAFMode = wafMode
	route.WAFConfig = string(wafConfigJSON)
	route.BasicAuthEnabled = input.BasicAuthEnabled
	route.BasicAuthUsername = input.BasicAuthUsername
	route.BasicAuthPassword = input.BasicAuthPassword
	route.RegionRestrictionEnabled = input.RegionRestrictionEnabled
	route.RegionRestrictionMode = regionMode
	route.RegionRestrictionCountries = string(regionCountriesJSON)
	route.DNSAutoSync = input.DNSAutoSync && dnsProviderMode == DNSProviderModeCloudflare
	route.DNSAccountID = dnsAccountID
	route.DNSZoneID = dnsZoneID
	route.DNSRecordType = dnsRecordType
	route.DNSRecordName = dnsRecordName
	route.DNSRecordContent = dnsRecordContent
	route.DNSAutoTarget = dnsAutoTarget
	route.DNSTargetCount = dnsTargetCount
	route.DNSScheduleMode = dnsScheduleMode
	route.DNSTTL = dnsTTL
	route.DNSProviderMode = dnsProviderMode
	route.DNSZoneIDRef = dnsZoneIDRef
	route.GSLBEnabled = gslbEnabled
	route.GSLBPolicy = string(gslbPolicyJSON)
	route.CloudflareProxied = input.CloudflareProxied
	route.DDOSProtectionMode = ddosMode
	route.DDOSProtectionProvider = ddosProvider
	route.DDOSProtectionTarget = ddosTarget
	route.Remark = remark
	return route, nil
}

func shouldPrecheckAuthoritativeDNSRoute(
	route *model.ProxyRoute,
	domain string,
	domainsJSON string,
	enabled bool,
	dnsProviderMode string,
	dnsZoneIDRef *uint,
	nodePool string,
	dnsRecordType string,
	dnsRecordName string,
	dnsRecordContent string,
	dnsAutoTarget bool,
	dnsTargetCount int,
	dnsScheduleMode string,
	dnsTTL int,
	gslbEnabled bool,
	gslbPolicyJSON string,
) bool {
	if !enabled || dnsProviderMode != DNSProviderModeAuthoritative || dnsZoneIDRef == nil {
		return false
	}
	if route == nil {
		return true
	}
	if !route.Enabled || normalizeDNSProviderMode(route.DNSProviderMode) != DNSProviderModeAuthoritative {
		return true
	}
	if route.DNSZoneIDRef == nil || *route.DNSZoneIDRef != *dnsZoneIDRef {
		return true
	}
	return route.Domain != domain ||
		route.Domains != domainsJSON ||
		route.NodePool != nodePool ||
		route.DNSRecordType != dnsRecordType ||
		route.DNSRecordName != dnsRecordName ||
		route.DNSRecordContent != dnsRecordContent ||
		route.DNSAutoTarget != dnsAutoTarget ||
		route.DNSTargetCount != dnsTargetCount ||
		route.DNSScheduleMode != dnsScheduleMode ||
		route.DNSTTL != dnsTTL ||
		route.GSLBEnabled != gslbEnabled ||
		route.GSLBPolicy != gslbPolicyJSON
}

func buildAuthoritativeDNSPrecheckProxyRoute(
	route *model.ProxyRoute,
	nodePool string,
	dnsRecordType string,
	dnsRecordName string,
	dnsTargetCount int,
	dnsScheduleMode string,
	dnsTTL int,
	gslbEnabled bool,
	gslbPolicyJSON string,
	dnsZoneIDRef *uint,
) *model.ProxyRoute {
	precheckRoute := &model.ProxyRoute{}
	if route != nil {
		copied := *route
		precheckRoute = &copied
	}
	precheckRoute.NodePool = nodePool
	precheckRoute.Enabled = true
	precheckRoute.DNSProviderMode = DNSProviderModeAuthoritative
	precheckRoute.DNSZoneIDRef = dnsZoneIDRef
	precheckRoute.DNSRecordType = dnsRecordType
	precheckRoute.DNSRecordName = dnsRecordName
	precheckRoute.DNSRecordContent = ""
	precheckRoute.DNSAutoTarget = true
	precheckRoute.DNSTargetCount = dnsTargetCount
	precheckRoute.DNSScheduleMode = dnsScheduleMode
	precheckRoute.DNSTTL = dnsTTL
	precheckRoute.GSLBEnabled = gslbEnabled
	precheckRoute.GSLBPolicy = gslbPolicyJSON
	return precheckRoute
}

func buildProxyRouteViews(routes []*model.ProxyRoute) ([]*ProxyRouteView, error) {
	views := make([]*ProxyRouteView, 0, len(routes))
	for _, route := range routes {
		view, err := buildProxyRouteView(route)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func buildProxyRouteView(route *model.ProxyRoute) (*ProxyRouteView, error) {
	if route == nil {
		return nil, errors.New("proxy route is nil")
	}
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return nil, err
	}
	upstreams, err := decodeStoredUpstreams(route.Upstreams, route.OriginURL)
	if err != nil {
		return nil, err
	}
	cacheRules, err := decodeStoredCacheRules(route.CacheRules)
	if err != nil {
		return nil, err
	}
	customHeaders, err := decodeStoredCustomHeaders(route.CustomHeaders)
	if err != nil {
		return nil, err
	}
	powConfig, err := decodeStoredPoWConfig(route.PoWEnabled, route.PoWConfig)
	if err != nil {
		return nil, err
	}
	wafConfig, err := decodeStoredWAFConfig(route.WAFEnabled, route.WAFConfig)
	if err != nil {
		return nil, err
	}
	regionCountries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
	if err != nil {
		return nil, err
	}
	gslbPolicy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
	if err != nil {
		return nil, err
	}
	certIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
	if err != nil {
		return nil, err
	}
	domainCertIDs, err := resolveProxyRouteDomainCertIDs(route, domains, certIDs)
	if err != nil {
		return nil, err
	}
	var certID *uint
	if len(certIDs) > 0 {
		certID = &certIDs[0]
	}
	primaryDomain := domains[0]
	return &ProxyRouteView{
		ID:                         route.ID,
		SiteName:                   normalizeProxyRouteSiteNameInput(route, route.SiteName, primaryDomain),
		Domain:                     primaryDomain,
		Domains:                    domains,
		PrimaryDomain:              primaryDomain,
		DomainCount:                len(domains),
		OriginID:                   route.OriginID,
		OriginURL:                  route.OriginURL,
		OriginHost:                 route.OriginHost,
		Upstreams:                  route.Upstreams,
		UpstreamList:               upstreams,
		NodePool:                   normalizeNodePoolName(route.NodePool),
		Enabled:                    route.Enabled,
		EnableHTTPS:                route.EnableHTTPS,
		CertID:                     certID,
		CertIDs:                    certIDs,
		DomainCertIDs:              domainCertIDs,
		RedirectHTTP:               route.RedirectHTTP,
		LimitConnPerServer:         route.LimitConnPerServer,
		LimitConnPerIP:             route.LimitConnPerIP,
		LimitRate:                  route.LimitRate,
		CacheEnabled:               route.CacheEnabled,
		CachePolicy:                route.CachePolicy,
		CacheRules:                 route.CacheRules,
		CacheRuleList:              cacheRules,
		CustomHeaders:              route.CustomHeaders,
		CustomHeaderList:           customHeaders,
		PoWEnabled:                 route.PoWEnabled,
		PoWConfig:                  powConfig,
		WAFEnabled:                 route.WAFEnabled,
		WAFMode:                    normalizeWAFMode(route.WAFMode),
		WAFConfig:                  wafConfig,
		BasicAuthEnabled:           route.BasicAuthEnabled,
		BasicAuthUsername:          route.BasicAuthUsername,
		BasicAuthPassword:          route.BasicAuthPassword,
		RegionRestrictionEnabled:   route.RegionRestrictionEnabled,
		RegionRestrictionMode:      normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
		RegionRestrictionCountries: regionCountries,
		DNSAutoSync:                route.DNSAutoSync,
		DNSAccountID:               route.DNSAccountID,
		DNSZoneID:                  route.DNSZoneID,
		DNSRecordType:              normalizeDNSRecordType(route.DNSRecordType),
		DNSRecordName:              route.DNSRecordName,
		DNSRecordContent:           route.DNSRecordContent,
		DNSAutoTarget:              route.DNSAutoTarget,
		DNSTargetCount:             normalizeDNSTargetCount(route.DNSTargetCount),
		DNSScheduleMode:            normalizeDNSScheduleMode(route.DNSScheduleMode),
		DNSTTL:                     normalizeDNSTTL(route.DNSTTL),
		DNSProviderMode:            normalizeDNSProviderMode(route.DNSProviderMode),
		DNSZoneIDRef:               route.DNSZoneIDRef,
		GSLBEnabled:                route.GSLBEnabled,
		GSLBPolicy:                 gslbPolicy,
		DNSRecordIDs:               decodeDNSRecordIDs(route.DNSRecordIDs),
		CloudflareProxied:          route.CloudflareProxied,
		DDOSProtectionMode:         normalizeDDOSProtectionMode(route.DDOSProtectionMode),
		DDOSProtectionProvider:     normalizeDDOSProtectionProvider(route.DDOSProtectionProvider),
		DDOSProtectionTarget:       strings.TrimSpace(route.DDOSProtectionTarget),
		DNSLastSyncStatus:          route.DNSLastSyncStatus,
		DNSLastSyncMessage:         route.DNSLastSyncMessage,
		DNSLastSyncedAt:            route.DNSLastSyncedAt,
		Remark:                     route.Remark,
		CreatedAt:                  route.CreatedAt,
		UpdatedAt:                  route.UpdatedAt,
	}, nil
}

func normalizeProxyRouteSiteNameInput(route *model.ProxyRoute, raw string, primaryDomain string) string {
	siteName := strings.TrimSpace(raw)
	if siteName != "" {
		return siteName
	}
	if route != nil && strings.TrimSpace(route.SiteName) != "" {
		return strings.TrimSpace(route.SiteName)
	}
	return primaryDomain
}

func normalizeProxyRouteDomainValue(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeProxyRouteDomainsInput(route *model.ProxyRoute, rawDomain string, rawDomains []string) ([]string, error) {
	if len(rawDomains) > 0 {
		domains, err := normalizeProxyRouteDomains(rawDomains)
		if err != nil {
			return nil, err
		}
		domain := normalizeProxyRouteDomainValue(rawDomain)
		if domain != "" && domain != domains[0] {
			return nil, errors.New("domain must match domains[0]")
		}
		return domains, nil
	}

	if route != nil {
		existingDomains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err == nil && len(existingDomains) > 0 {
			domain := normalizeProxyRouteDomainValue(rawDomain)
			if domain == "" || domain == existingDomains[0] {
				return existingDomains, nil
			}
		}
	}

	return normalizeProxyRouteDomains([]string{rawDomain})
}

func normalizeProxyRouteDomains(rawDomains []string) ([]string, error) {
	normalized := make([]string, 0, len(rawDomains))
	seen := make(map[string]struct{}, len(rawDomains))
	for _, rawDomain := range rawDomains {
		domain := normalizeProxyRouteDomainValue(rawDomain)
		if domain == "" {
			continue
		}
		if !isValidProxyRouteDomain(domain) {
			return nil, errors.New("domain format is invalid")
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		normalized = append(normalized, domain)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one domain is required")
	}
	return normalized, nil
}

func isValidProxyRouteDomain(domain string) bool {
	if domain == "" || len(domain) > 253 {
		return false
	}
	if strings.ContainsAny(domain, " \t\r\n;{}\"'`$:\\/") || strings.Contains(domain, "://") {
		return false
	}
	if strings.HasSuffix(domain, ".") {
		domain = strings.TrimSuffix(domain, ".")
	}
	if strings.HasPrefix(domain, "*.") {
		domain = strings.TrimPrefix(domain, "*.")
		if domain == "" {
			return false
		}
	} else if strings.Contains(domain, "*") {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !proxyRouteDomainLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func validateProxyRouteSiteName(siteName string) error {
	if strings.TrimSpace(siteName) == "" {
		return errors.New("site_name cannot be empty")
	}
	return nil
}

func validateProxyRouteIdentityUniqueness(route *model.ProxyRoute, siteName string, domains []string) error {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return err
	}

	currentID := uint(0)
	if route != nil {
		currentID = route.ID
	}

	for _, item := range routes {
		if item == nil || item.ID == currentID {
			continue
		}
		existingSiteName := normalizeProxyRouteSiteNameInput(item, item.SiteName, item.Domain)
		if existingSiteName == siteName {
			return errors.New("site_name already exists")
		}

		existingDomains, err := decodeStoredDomains(item.Domains, item.Domain)
		if err != nil {
			return fmt.Errorf("existing route %d domains are invalid: %w", item.ID, err)
		}
		existingSet := make(map[string]struct{}, len(existingDomains))
		for _, existingDomain := range existingDomains {
			existingSet[existingDomain] = struct{}{}
		}
		for _, domain := range domains {
			if _, ok := existingSet[domain]; ok {
				return fmt.Errorf("domain %s already exists", domain)
			}
		}
	}

	return nil
}

func normalizeProxyRouteDNSSettings(input ProxyRouteInput) (*uint, string, string, string, string, bool, string, error) {
	ddosMode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	if !input.DNSAutoSync {
		return nil, "", normalizeDNSRecordType(input.DNSRecordType), "", "", false, ddosMode, nil
	}

	if input.DNSAccountID == nil || *input.DNSAccountID == 0 {
		return nil, "", "", "", "", false, "", errors.New("启用自动 DNS 时必须选择 DNS 账号")
	}
	account, err := model.GetDnsAccountByID(*input.DNSAccountID)
	if err != nil {
		return nil, "", "", "", "", false, "", errors.New("选择的 DNS 账号不存在")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, "", "", "", "", false, "", errors.New("自动 DNS 目前仅支持 Cloudflare DNS 账号")
	}

	recordType := normalizeDNSRecordType(input.DNSRecordType)
	recordName := normalizeDNSRecordName(input.DNSRecordName)
	recordContent := strings.TrimSpace(input.DNSRecordContent)
	if recordName != "" && !isValidProxyRouteDomain(recordName) {
		return nil, "", "", "", "", false, "", errors.New("DNS 记录名格式无效")
	}
	if recordContent != "" {
		if err := validateDNSRecordContent(recordType, recordContent); err != nil {
			return nil, "", "", "", "", false, "", err
		}
	}
	dnsAutoTarget := input.DNSAutoTarget || recordContent == ""
	if recordType == "CNAME" && dnsAutoTarget {
		return nil, "", "", "", "", false, "", errors.New("CNAME 记录必须手动填写记录内容")
	}

	dnsAccountID := *input.DNSAccountID
	return &dnsAccountID, strings.TrimSpace(input.DNSZoneID), recordType, recordName, recordContent, dnsAutoTarget, ddosMode, nil
}

func normalizeProxyRouteDNSSettingsV2(input ProxyRouteInput) (*uint, string, string, string, string, bool, int, string, string, error) {
	ddosMode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	dnsTargetCount := normalizeDNSTargetCount(input.DNSTargetCount)
	dnsScheduleMode := normalizeDNSScheduleMode(input.DNSScheduleMode)
	if !input.DNSAutoSync {
		return nil, "", normalizeDNSRecordType(input.DNSRecordType), "", "", false, dnsTargetCount, dnsScheduleMode, ddosMode, nil
	}
	if input.DNSAccountID == nil || *input.DNSAccountID == 0 {
		return nil, "", "", "", "", false, 0, "", "", errors.New("automatic DNS requires a DNS account")
	}
	account, err := model.GetDnsAccountByID(*input.DNSAccountID)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", "", errors.New("selected DNS account does not exist")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, "", "", "", "", false, 0, "", "", errors.New("automatic DNS currently only supports Cloudflare DNS accounts")
	}
	dnsRecordType := normalizeDNSRecordType(input.DNSRecordType)
	dnsRecordName := normalizeDNSRecordName(input.DNSRecordName)
	if dnsRecordName != "" && !isValidProxyRouteDomain(dnsRecordName) {
		return nil, "", "", "", "", false, 0, "", "", errors.New("DNS record name format is invalid")
	}
	dnsRecordContent := strings.TrimSpace(input.DNSRecordContent)
	if dnsRecordContent != "" {
		contents, err := normalizeDNSRecordContents(dnsRecordType, splitDNSRecordContent(dnsRecordContent))
		if err != nil {
			return nil, "", "", "", "", false, 0, "", "", err
		}
		if dnsRecordType == "CNAME" && len(contents) > 1 {
			return nil, "", "", "", "", false, 0, "", "", errors.New("CNAME record only supports one target")
		}
		dnsRecordContent = strings.Join(contents, ",")
	}
	dnsAutoTarget := input.DNSAutoTarget || dnsRecordContent == ""
	if dnsRecordType == "CNAME" && dnsAutoTarget {
		return nil, "", "", "", "", false, 0, "", "", errors.New("CNAME record requires manual content")
	}
	dnsAccountID := *input.DNSAccountID
	return &dnsAccountID,
		strings.TrimSpace(input.DNSZoneID),
		dnsRecordType,
		dnsRecordName,
		dnsRecordContent,
		dnsAutoTarget,
		dnsTargetCount,
		dnsScheduleMode,
		ddosMode,
		nil
}

func normalizeProxyRouteDNSSettingsV3(input ProxyRouteInput) (*uint, string, string, string, string, bool, int, string, int, bool, ProxyRouteGSLBPolicy, string, string, string, error) {
	ddosMode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	dnsTargetCount := normalizeDNSTargetCount(input.DNSTargetCount)
	dnsScheduleMode := normalizeDNSScheduleMode(input.DNSScheduleMode)
	dnsTTL := normalizeDNSTTL(input.DNSTTL)
	gslbPolicy, err := normalizeGSLBPolicy(input.GSLBPolicy, input.NodePool, dnsTargetCount, dnsScheduleMode, dnsTTL)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
	}
	gslbEnabled := input.GSLBEnabled
	dnsProviderMode := normalizeDNSProviderMode(input.DNSProviderMode)
	if dnsProviderMode == DNSProviderModeAuthoritative {
		ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		dnsRecordType := normalizeDNSRecordType(input.DNSRecordType)
		if gslbEnabled && dnsRecordType != "A" && dnsRecordType != "AAAA" {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("GSLB scheduling only supports A/AAAA records")
		}
		return nil,
			"",
			dnsRecordType,
			normalizeDNSRecordName(input.DNSRecordName),
			"",
			true,
			dnsTargetCount,
			dnsScheduleMode,
			dnsTTL,
			gslbEnabled,
			gslbPolicy,
			ddosMode,
			ddosProvider,
			ddosTarget,
			nil
	}
	if !input.DNSAutoSync {
		ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		return nil, "", normalizeDNSRecordType(input.DNSRecordType), "", "", false, dnsTargetCount, dnsScheduleMode, dnsTTL, false, gslbPolicy, ddosMode, ddosProvider, ddosTarget, nil
	}
	if input.DNSAccountID == nil || *input.DNSAccountID == 0 {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("automatic DNS requires a DNS account")
	}
	account, err := model.GetDnsAccountByID(*input.DNSAccountID)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("selected DNS account does not exist")
	}
	if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("automatic DNS currently only supports Cloudflare DNS accounts")
	}
	dnsRecordType := normalizeDNSRecordType(input.DNSRecordType)
	dnsRecordName := normalizeDNSRecordName(input.DNSRecordName)
	if dnsRecordName != "" && !isValidProxyRouteDomain(dnsRecordName) {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("DNS record name format is invalid")
	}
	dnsRecordContent := strings.TrimSpace(input.DNSRecordContent)
	if dnsRecordContent != "" {
		contents, err := normalizeDNSRecordContents(dnsRecordType, splitDNSRecordContent(dnsRecordContent))
		if err != nil {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
		}
		if dnsRecordType == "CNAME" && len(contents) > 1 {
			return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("CNAME record only supports one target")
		}
		dnsRecordContent = strings.Join(contents, ",")
	}
	dnsAutoTarget := input.DNSAutoTarget || dnsRecordContent == "" || gslbEnabled
	if dnsRecordType == "CNAME" && dnsAutoTarget {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("CNAME record requires manual content")
	}
	if gslbEnabled && dnsRecordType != "A" && dnsRecordType != "AAAA" {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", errors.New("GSLB scheduling only supports A/AAAA records")
	}
	ddosProvider, ddosTarget, err := normalizeDDOSProtectionTarget(input, dnsProviderMode)
	if err != nil {
		return nil, "", "", "", "", false, 0, "", 0, false, ProxyRouteGSLBPolicy{}, "", "", "", err
	}
	dnsAccountID := *input.DNSAccountID
	return &dnsAccountID,
		strings.TrimSpace(input.DNSZoneID),
		dnsRecordType,
		dnsRecordName,
		dnsRecordContent,
		dnsAutoTarget,
		dnsTargetCount,
		dnsScheduleMode,
		dnsTTL,
		gslbEnabled,
		gslbPolicy,
		ddosMode,
		ddosProvider,
		ddosTarget,
		nil
}

func normalizeDNSProviderMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case DNSProviderModeAuthoritative:
		return DNSProviderModeAuthoritative
	default:
		return DNSProviderModeCloudflare
	}
}

func normalizeProxyRouteAuthoritativeZone(input ProxyRouteInput, providerMode string, domains []string) (*uint, error) {
	if providerMode != DNSProviderModeAuthoritative {
		return nil, nil
	}
	if input.DNSZoneIDRef == nil || *input.DNSZoneIDRef == 0 {
		return nil, errors.New("authoritative DNS mode requires a DNS zone")
	}
	zone, err := model.GetDNSZoneByID(*input.DNSZoneIDRef)
	if err != nil {
		return nil, errors.New("selected DNS zone does not exist")
	}
	if !zone.Enabled {
		return nil, errors.New("selected DNS zone is disabled")
	}
	for _, domain := range domains {
		if !domainBelongsToZone(domain, zone.Name) {
			return nil, fmt.Errorf("domain %s is not under DNS zone %s", domain, zone.Name)
		}
	}
	zoneID := *input.DNSZoneIDRef
	return &zoneID, nil
}

func domainBelongsToZone(domain string, zoneName string) bool {
	domain = normalizeDNSRecordName(domain)
	zoneName = normalizeDNSRecordName(zoneName)
	return domain == zoneName || strings.HasSuffix(domain, "."+zoneName)
}

func shouldSyncProxyRouteCloudflareDNS(route *model.ProxyRoute) bool {
	if route == nil || !route.DNSAutoSync {
		return false
	}
	return normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeCloudflare
}

func normalizeDNSTargetCount(value int) int {
	if value <= 0 {
		return 1
	}
	if value > 20 {
		return 20
	}
	return value
}

func normalizeDNSScheduleMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "load_aware":
		return "load_aware"
	case "weighted":
		return "weighted"
	default:
		return "healthy"
	}
}

func normalizeDNSTTL(value int) int {
	if value <= 0 {
		return cloudflareDefaultRecordTTL
	}
	if value == cloudflareDefaultRecordTTL {
		return cloudflareDefaultRecordTTL
	}
	if value < 30 {
		return 30
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func normalizeDDOSProtectionMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DDOSProtectionModeAuto:
		return DDOSProtectionModeAuto
	default:
		return DDOSProtectionModeOff
	}
}

func normalizeDDOSProtectionProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DDOSProtectionProviderCustom:
		return DDOSProtectionProviderCustom
	default:
		return DDOSProtectionProviderCloudflare
	}
}

func normalizeDDOSProtectionTarget(input ProxyRouteInput, dnsProviderMode string) (string, string, error) {
	mode := normalizeDDOSProtectionMode(input.DDOSProtectionMode)
	provider := normalizeDDOSProtectionProvider(input.DDOSProtectionProvider)
	rawTarget := strings.TrimSpace(input.DDOSProtectionTarget)
	if mode != DDOSProtectionModeAuto {
		return provider, "", nil
	}
	if dnsProviderMode == DNSProviderModeAuthoritative && provider == DDOSProtectionProviderCloudflare {
		return provider, "", errors.New("authoritative DNS mode cannot enable Cloudflare DDoS protection provider")
	}
	if provider == DDOSProtectionProviderCloudflare {
		if dnsProviderMode != DNSProviderModeCloudflare {
			return provider, "", nil
		}
		targetID := uint(0)
		if rawTarget != "" {
			var parsed uint64
			if _, err := fmt.Sscan(rawTarget, &parsed); err != nil || parsed == 0 {
				return provider, "", errors.New("Cloudflare DDoS protection account is invalid")
			}
			targetID = uint(parsed)
		} else if input.DNSAccountID != nil {
			targetID = *input.DNSAccountID
		}
		if targetID > 0 {
			account, err := model.GetDnsAccountByID(targetID)
			if err != nil {
				return provider, "", errors.New("selected Cloudflare DDoS protection account does not exist")
			}
			if strings.ToLower(strings.TrimSpace(account.Type)) != cloudflareDNSProviderType {
				return provider, "", errors.New("Cloudflare DDoS protection account must be a Cloudflare DNS account")
			}
			return provider, fmt.Sprint(targetID), nil
		}
		return provider, "", nil
	}
	if provider == DDOSProtectionProviderCustom {
		target := normalizeNodePoolName(rawTarget)
		if target == "" {
			return provider, "", errors.New("custom DDoS protection requires a node/IP pool")
		}
		if normalizeDNSRecordType(input.DNSRecordType) != "A" && normalizeDNSRecordType(input.DNSRecordType) != "AAAA" {
			return provider, "", errors.New("custom DDoS protection only supports A/AAAA records")
		}
		return provider, target, nil
	}
	return provider, "", nil
}

func normalizeProxyRouteLimitConnValue(value int, field string) (int, error) {
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", field)
	}
	return value, nil
}

func normalizeProxyRouteCertificateIDs(enableHTTPS bool, certID *uint, certIDs []uint) ([]uint, error) {
	if !enableHTTPS {
		return []uint{}, nil
	}

	candidates := make([]uint, 0, len(certIDs)+1)
	if certID != nil && *certID != 0 {
		candidates = append(candidates, *certID)
	}
	candidates = append(candidates, certIDs...)

	normalized := make([]uint, 0, len(candidates))
	seen := make(map[uint]struct{}, len(candidates))
	for _, item := range candidates {
		if item == 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		if _, err := model.GetTLSCertificateByID(item); err != nil {
			return nil, errors.New("selected certificate does not exist")
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("must select a certificate when HTTPS is enabled")
	}
	return normalized, nil
}

func normalizeProxyRouteDomainCertificateIDs(
	domains []string,
	enableHTTPS bool,
	rawDomainCertIDs []uint,
	certID *uint,
	certIDs []uint,
) ([]uint, []uint, *uint, error) {
	if !enableHTTPS {
		return []uint{}, []uint{}, nil, nil
	}

	if len(rawDomainCertIDs) > 0 {
		if len(rawDomainCertIDs) != len(domains) {
			return nil, nil, nil, errors.New("domain_cert_ids must match domains length")
		}

		normalizedDomainCertIDs := make([]uint, len(rawDomainCertIDs))
		uniqueCertIDs := make([]uint, 0, len(rawDomainCertIDs))
		seen := make(map[uint]struct{}, len(rawDomainCertIDs))
		hasAssignedCertificate := false
		for index, item := range rawDomainCertIDs {
			if item == 0 {
				continue
			}
			if _, err := model.GetTLSCertificateByID(item); err != nil {
				return nil, nil, nil, errors.New("selected certificate does not exist")
			}
			normalizedDomainCertIDs[index] = item
			hasAssignedCertificate = true
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			uniqueCertIDs = append(uniqueCertIDs, item)
		}
		if !hasAssignedCertificate {
			return nil, nil, nil, errors.New("must select a certificate when HTTPS is enabled")
		}

		primaryCertID := &uniqueCertIDs[0]
		return normalizedDomainCertIDs, uniqueCertIDs, primaryCertID, nil
	}

	normalizedCertIDs, err := normalizeProxyRouteCertificateIDs(
		enableHTTPS,
		certID,
		certIDs,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	switch {
	case len(normalizedCertIDs) == 0:
		return nil, nil, nil, errors.New("must select a certificate when HTTPS is enabled")
	case len(normalizedCertIDs) == 1:
		domainCertIDs := make([]uint, len(domains))
		for index := range domainCertIDs {
			domainCertIDs[index] = normalizedCertIDs[0]
		}
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, nil
	case len(normalizedCertIDs) == len(domains):
		domainCertIDs := make([]uint, len(normalizedCertIDs))
		copy(domainCertIDs, normalizedCertIDs)
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, nil
	default:
		domainCertIDs, err := deriveDomainCertIDsFromCertificateSet(
			domains,
			normalizedCertIDs,
		)
		if err != nil {
			return nil, nil, nil, err
		}
		primaryCertID := &normalizedCertIDs[0]
		return domainCertIDs, normalizedCertIDs, primaryCertID, nil
	}
}

func validateProxyRouteDomainCertificateCoverage(
	domains []string,
	domainCertIDs []uint,
) error {
	if len(domainCertIDs) == 0 {
		return nil
	}

	domainsByCertID := make(map[uint][]string)
	for index, certID := range domainCertIDs {
		if certID == 0 {
			continue
		}
		domainsByCertID[certID] = append(domainsByCertID[certID], domains[index])
	}

	for certID, assignedDomains := range domainsByCertID {
		certificate, err := model.GetTLSCertificateByID(certID)
		if err != nil {
			return errors.New("selected certificate does not exist")
		}
		if err := validateCertificateCoverage(certificate, assignedDomains); err != nil {
			return err
		}
	}
	return nil
}

func deriveDomainCertIDsFromCertificateSet(
	domains []string,
	certIDs []uint,
) ([]uint, error) {
	certificates, err := loadTLSCertificates(certIDs)
	if err != nil {
		return nil, err
	}

	result := make([]uint, len(domains))
	for domainIndex, domain := range domains {
		if domainIndex < len(certificates) &&
			certificates[domainIndex] != nil &&
			validateCertificateCoverage(certificates[domainIndex], []string{domain}) == nil {
			result[domainIndex] = certificates[domainIndex].ID
			continue
		}

		assigned := uint(0)
		for _, certificate := range certificates {
			if certificate != nil &&
				validateCertificateCoverage(certificate, []string{domain}) == nil {
				assigned = certificate.ID
				break
			}
		}
		if assigned == 0 {
			return nil, fmt.Errorf("certificate does not cover domain %s", domain)
		}
		result[domainIndex] = assigned
	}
	return result, nil
}

func decodeStoredDomainCertIDs(raw string, domainCount int) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []uint{}, nil
	}

	var domainCertIDs []uint
	if err := json.Unmarshal([]byte(text), &domainCertIDs); err != nil {
		return nil, errors.New("domain_cert_ids payload is invalid")
	}
	if len(domainCertIDs) == 0 {
		return []uint{}, nil
	}
	if domainCount > 0 && len(domainCertIDs) != domainCount {
		return nil, errors.New("domain_cert_ids length does not match domains")
	}

	normalized := make([]uint, len(domainCertIDs))
	copy(normalized, domainCertIDs)
	return normalized, nil
}

func resolveProxyRouteDomainCertIDs(
	route *model.ProxyRoute,
	domains []string,
	certIDs []uint,
) ([]uint, error) {
	domainCertIDs, err := decodeStoredDomainCertIDs(route.DomainCertIDs, len(domains))
	if err != nil {
		return nil, err
	}
	if len(domainCertIDs) > 0 || len(certIDs) == 0 {
		return domainCertIDs, nil
	}
	return deriveDomainCertIDsFromCertificateSet(domains, certIDs)
}

func normalizeProxyRouteLimitRate(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" || normalized == "0" {
		return "", nil
	}
	if !proxyRouteLimitRatePattern.MatchString(normalized) {
		return "", errors.New("limit_rate must be a number or use the 512k / 1m format")
	}
	if strings.TrimRight(normalized, "km") == "" {
		return "", nil
	}
	return normalized, nil
}

func resolveProxyRoutePrimaryOrigin(input ProxyRouteInput) (string, *uint, error) {
	if hasStructuredOriginInput(input) {
		scheme, err := normalizeOriginScheme(input.OriginScheme)
		if err != nil {
			return "", nil, err
		}
		port, err := normalizeOriginPort(input.OriginPort)
		if err != nil {
			return "", nil, err
		}
		uri, err := normalizeOriginURI(input.OriginURI)
		if err != nil {
			return "", nil, err
		}
		if input.OriginID != nil && *input.OriginID != 0 {
			origin, err := model.GetOriginByID(*input.OriginID)
			if err != nil {
				return "", nil, errors.New("selected origin does not exist")
			}
			originURL, err := buildOriginURLFromParts(
				scheme,
				origin.Address,
				port,
				uri,
			)
			if err != nil {
				return "", nil, err
			}
			return originURL, &origin.ID, nil
		}

		address := normalizeOriginAddress(input.OriginAddress)
		if err := validateOriginAddress(address); err != nil {
			return "", nil, err
		}
		originURL, err := buildOriginURLFromParts(scheme, address, port, uri)
		if err != nil {
			return "", nil, err
		}
		origin, err := getOrCreateOriginByAddress(address)
		if err != nil {
			return "", nil, err
		}
		return originURL, &origin.ID, nil
	}

	originURL := strings.TrimSpace(input.OriginURL)
	if originURL == "" {
		return "", nil, errors.New("origin_url cannot be empty")
	}
	address, err := extractOriginAddress(originURL)
	if err != nil {
		return "", nil, err
	}
	origin, findErr := model.GetOriginByAddress(address)
	if findErr == nil {
		return originURL, &origin.ID, nil
	}
	if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return "", nil, findErr
	}
	return originURL, nil, nil
}

func hasStructuredOriginInput(input ProxyRouteInput) bool {
	return (input.OriginID != nil && *input.OriginID != 0) ||
		strings.TrimSpace(input.OriginScheme) != "" ||
		strings.TrimSpace(input.OriginAddress) != "" ||
		strings.TrimSpace(input.OriginPort) != "" ||
		strings.TrimSpace(input.OriginURI) != ""
}

func normalizeCustomHeaders(headers []ProxyRouteCustomHeaderInput) ([]ProxyRouteCustomHeaderInput, error) {
	if len(headers) == 0 {
		return []ProxyRouteCustomHeaderInput{}, nil
	}
	normalized := make([]ProxyRouteCustomHeaderInput, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		value := strings.TrimSpace(header.Value)
		if key == "" && value == "" {
			continue
		}
		if key == "" {
			return nil, errors.New("custom header key cannot be empty")
		}
		if !proxyHeaderKeyPattern.MatchString(key) {
			return nil, errors.New("custom header key format is invalid")
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("custom headers cannot contain newlines")
		}
		normalized = append(normalized, ProxyRouteCustomHeaderInput{
			Key:   key,
			Value: value,
		})
	}
	return normalized, nil
}

func normalizeUpstreams(originURL string, upstreams []string) ([]string, error) {
	candidates := make([]string, 0, len(upstreams)+1)
	if strings.TrimSpace(originURL) != "" {
		candidates = append(candidates, originURL)
	}
	candidates = append(candidates, upstreams...)
	trimmed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		item := strings.TrimSpace(candidate)
		if item == "" {
			continue
		}
		trimmed = append(trimmed, item)
	}
	unique := make([]string, 0, len(trimmed))
	seen := make(map[string]struct{}, len(trimmed))
	for _, item := range trimmed {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		unique = append(unique, item)
	}
	normalized := make([]string, 0, len(unique))
	var scheme string
	multiUpstream := len(unique) > 1
	for _, item := range unique {
		if err := validateOriginURL(item); err != nil {
			return nil, err
		}
		parsed, err := url.ParseRequestURI(item)
		if err != nil {
			return nil, errors.New("origin URL format is invalid")
		}
		if multiUpstream && parsed.Path != "" && parsed.Path != "/" {
			return nil, errors.New("multi-upstream mode does not support origin paths")
		}
		if multiUpstream && parsed.RawQuery != "" {
			return nil, errors.New("multi-upstream mode does not support origin query strings")
		}
		if scheme == "" {
			scheme = parsed.Scheme
		} else if scheme != parsed.Scheme {
			return nil, errors.New("all upstreams must use the same scheme")
		}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one upstream is required")
	}
	return normalized, nil
}

func decodeStoredCustomHeaders(raw string) ([]ProxyRouteCustomHeaderInput, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []ProxyRouteCustomHeaderInput{}, nil
	}
	var headers []ProxyRouteCustomHeaderInput
	if err := json.Unmarshal([]byte(text), &headers); err != nil {
		return nil, errors.New("custom_headers payload is invalid")
	}
	return normalizeCustomHeaders(headers)
}

func normalizeProxyRouteRegionRestrictionMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case proxyRouteRegionModeAllow, proxyRouteRegionModeBlock:
		return mode
	default:
		return proxyRouteRegionModeBlock
	}
}

func normalizeProxyRouteRegionRestriction(enabled bool, rawMode string, rawCountries []string) (string, []string, error) {
	mode := normalizeProxyRouteRegionRestrictionMode(rawMode)
	if !enabled {
		return mode, []string{}, nil
	}
	countries := make([]string, 0, len(rawCountries))
	seen := make(map[string]struct{}, len(rawCountries))
	for _, rawCountry := range rawCountries {
		country := strings.ToUpper(strings.TrimSpace(rawCountry))
		if country == "" {
			continue
		}
		if !proxyRouteRegionCountryPattern.MatchString(country) {
			return "", nil, fmt.Errorf("region country code %q is invalid", rawCountry)
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		countries = append(countries, country)
	}
	if len(countries) == 0 {
		return "", nil, errors.New("region restriction requires at least one country code")
	}
	return mode, countries, nil
}

func decodeStoredRegionRestrictionCountries(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}, nil
	}
	var countries []string
	if err := json.Unmarshal([]byte(text), &countries); err != nil {
		return nil, errors.New("region_restriction_countries payload is invalid")
	}
	_, normalized, err := normalizeProxyRouteRegionRestriction(len(countries) > 0, proxyRouteRegionModeBlock, countries)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeCachePolicy(enabled bool, raw string) string {
	if !enabled {
		return ""
	}
	policy := strings.TrimSpace(raw)
	if policy == "" {
		return proxyRouteCachePolicyURL
	}
	return policy
}

func normalizeCacheRules(enabled bool, rawPolicy string, rules []string) ([]string, error) {
	if !enabled {
		return []string{}, nil
	}
	policy := normalizeCachePolicy(enabled, rawPolicy)
	switch policy {
	case proxyRouteCachePolicyURL:
		return []string{}, nil
	case proxyRouteCachePolicySuffix:
		return normalizeCacheSuffixRules(rules)
	case proxyRouteCachePolicyPathPrefix:
		return normalizeCachePathRules(rules, true)
	case proxyRouteCachePolicyPathContains:
		return normalizeCachePathRules(rules, true)
	case proxyRouteCachePolicyPathExact:
		return normalizeCachePathRules(rules, false)
	default:
		return nil, errors.New("cache policy is not supported")
	}
}

func normalizeCacheSuffixRules(rules []string) ([]string, error) {
	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(strings.TrimPrefix(rule, "."))
		if item == "" {
			continue
		}
		if strings.ContainsAny(item, "/\\ \t\r\n") {
			return nil, errors.New("cache suffix format is invalid")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one suffix is required")
	}
	return normalized, nil
}

func normalizeCachePathRules(rules []string, allowPrefix bool) ([]string, error) {
	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(rule)
		if item == "" {
			continue
		}
		if !strings.HasPrefix(item, "/") || strings.Contains(item, "://") || strings.ContainsAny(item, " \t\r\n") {
			return nil, errors.New("cache path rule format is invalid")
		}
		if !allowPrefix && strings.HasSuffix(item, "/") && len(item) > 1 {
			item = strings.TrimRight(item, "/")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		if allowPrefix {
			return nil, errors.New("at least one path prefix is required")
		}
		return nil, errors.New("at least one exact path is required")
	}
	return normalized, nil
}

func decodeStoredCacheRules(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}, nil
	}
	var rules []string
	if err := json.Unmarshal([]byte(text), &rules); err != nil {
		return nil, errors.New("cache_rules payload is invalid")
	}
	normalized := make([]string, 0, len(rules))
	for _, rule := range rules {
		item := strings.TrimSpace(rule)
		if item == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func decodeStoredUpstreams(raw string, fallbackOriginURL string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return normalizeUpstreams(fallbackOriginURL, nil)
	}
	var upstreams []string
	if err := json.Unmarshal([]byte(text), &upstreams); err != nil {
		return nil, errors.New("upstreams payload is invalid")
	}
	return normalizeUpstreams(fallbackOriginURL, upstreams)
}

func decodeStoredDomains(raw string, fallbackDomain string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return normalizeProxyRouteDomains([]string{fallbackDomain})
	}
	var domains []string
	if err := json.Unmarshal([]byte(text), &domains); err != nil {
		return nil, errors.New("domains payload is invalid")
	}
	return normalizeProxyRouteDomains(domains)
}

func decodeStoredCertIDs(raw string, fallbackCertID *uint) ([]uint, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		if fallbackCertID == nil || *fallbackCertID == 0 {
			return []uint{}, nil
		}
		return []uint{*fallbackCertID}, nil
	}
	var certIDs []uint
	if err := json.Unmarshal([]byte(text), &certIDs); err != nil {
		return nil, errors.New("cert_ids payload is invalid")
	}
	normalized := make([]uint, 0, len(certIDs))
	seen := make(map[uint]struct{}, len(certIDs))
	for _, certID := range certIDs {
		if certID == 0 {
			continue
		}
		if _, ok := seen[certID]; ok {
			continue
		}
		seen[certID] = struct{}{}
		normalized = append(normalized, certID)
	}
	if len(normalized) == 0 && fallbackCertID != nil && *fallbackCertID != 0 {
		return []uint{*fallbackCertID}, nil
	}
	return normalized, nil
}

func validateOriginURL(raw string) error {
	if raw == "" {
		return errors.New("origin URL cannot be empty")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return errors.New("origin URL format is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("origin URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return errors.New("origin URL format is invalid")
	}
	return nil
}

func validateOriginHost(raw string) error {
	if raw == "" {
		return nil
	}
	if strings.ContainsAny(raw, "/\\ \t\r\n") || strings.Contains(raw, "://") {
		return errors.New("origin_host format is invalid")
	}
	parsed, err := url.Parse("//" + raw)
	if err != nil || parsed.Host == "" || parsed.Host != raw {
		return errors.New("origin_host format is invalid")
	}
	if parsed.Hostname() == "" {
		return errors.New("origin_host format is invalid")
	}
	return nil
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

// PoW configuration types and validation

type ProxyRoutePoWListConfig struct {
	IPs         []string `json:"ips"`
	IPCidrs     []string `json:"ip_cidrs"`
	Paths       []string `json:"paths"`
	PathRegexes []string `json:"path_regexes"`
	UserAgents  []string `json:"user_agents"`
}

type ProxyRoutePoWConfig struct {
	Difficulty   int                     `json:"difficulty"`
	Algorithm    string                  `json:"algorithm"`
	SessionTTL   int                     `json:"session_ttl"`
	ChallengeTTL int                     `json:"challenge_ttl"`
	Whitelist    ProxyRoutePoWListConfig `json:"whitelist"`
	Blacklist    ProxyRoutePoWListConfig `json:"blacklist"`
}

var powAlgorithmValues = map[string]bool{"fast": true, "slow": true}

func defaultPoWConfig() ProxyRoutePoWConfig {
	return ProxyRoutePoWConfig{
		Difficulty:   4,
		Algorithm:    "fast",
		SessionTTL:   600,
		ChallengeTTL: 300,
		Whitelist:    ProxyRoutePoWListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, PathRegexes: []string{}, UserAgents: []string{}},
		Blacklist:    ProxyRoutePoWListConfig{IPs: []string{}, IPCidrs: []string{}, Paths: []string{}, PathRegexes: []string{}, UserAgents: []string{}},
	}
}

func normalizePoWConfig(enabled bool, raw string) (ProxyRoutePoWConfig, error) {
	if !enabled {
		return defaultPoWConfig(), nil
	}

	cfg := defaultPoWConfig()
	text := strings.TrimSpace(raw)
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &cfg); err != nil {
			return cfg, errors.New("pow_config 格式无效")
		}
	}

	if cfg.Difficulty < 1 || cfg.Difficulty > 16 {
		return cfg, errors.New("pow_config.difficulty 必须在 1-16 之间")
	}
	if !powAlgorithmValues[cfg.Algorithm] {
		return cfg, errors.New("pow_config.algorithm 必须为 fast 或 slow")
	}
	if cfg.SessionTTL < 60 {
		return cfg, errors.New("pow_config.session_ttl 不能小于 60 秒")
	}
	if cfg.ChallengeTTL < 30 {
		return cfg, errors.New("pow_config.challenge_ttl 不能小于 30 秒")
	}

	for _, cidr := range cfg.Whitelist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("pow_config 白名单 IP CIDR 格式无效: %s", cidr)
		}
	}
	for _, cidr := range cfg.Blacklist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("pow_config 黑名单 IP CIDR 格式无效: %s", cidr)
		}
	}

	for _, re := range cfg.Whitelist.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("pow_config 白名单路径正则格式无效: %s", re)
		}
	}
	for _, re := range cfg.Blacklist.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("pow_config 黑名单路径正则格式无效: %s", re)
		}
	}

	for _, ip := range cfg.Whitelist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("pow_config 白名单 IP 格式无效: %s", ip)
		}
	}
	for _, ip := range cfg.Blacklist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("pow_config 黑名单 IP 格式无效: %s", ip)
		}
	}

	type dimension struct {
		name string
		wl   []string
		bl   []string
	}
	dimensions := []dimension{
		{"IP", cfg.Whitelist.IPs, cfg.Blacklist.IPs},
		{"IP CIDR", cfg.Whitelist.IPCidrs, cfg.Blacklist.IPCidrs},
		{"路径", cfg.Whitelist.Paths, cfg.Blacklist.Paths},
		{"路径正则", cfg.Whitelist.PathRegexes, cfg.Blacklist.PathRegexes},
		{"User-Agent", cfg.Whitelist.UserAgents, cfg.Blacklist.UserAgents},
	}
	for _, dim := range dimensions {
		if len(dim.wl) > 0 && len(dim.bl) > 0 {
			return cfg, fmt.Errorf("pow_config %s 不能同时配置白名单和黑名单", dim.name)
		}
	}

	return cfg, nil
}

func decodeStoredPoWConfig(enabled bool, raw string) (*ProxyRoutePoWConfig, error) {
	if !enabled {
		cfg := defaultPoWConfig()
		return &cfg, nil
	}
	text := strings.TrimSpace(raw)
	if text == "" || text == "{}" {
		cfg := defaultPoWConfig()
		return &cfg, nil
	}
	var cfg ProxyRoutePoWConfig
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return nil, errors.New("pow_config 格式无效")
	}
	return &cfg, nil
}

// WAF configuration types and validation

type ProxyRouteWAFCustomRules struct {
	PathContains   []string `json:"path_contains"`
	PathRegexes    []string `json:"path_regexes"`
	QueryContains  []string `json:"query_contains"`
	HeaderContains []string `json:"header_contains"`
	UserAgents     []string `json:"user_agents"`
}

type ProxyRouteWAFWhitelistConfig struct {
	IPs     []string `json:"ips"`
	IPCidrs []string `json:"ip_cidrs"`
	Paths   []string `json:"paths"`
}

type ProxyRouteWAFConfig struct {
	BuiltinRules []string                     `json:"builtin_rules"`
	Whitelist    ProxyRouteWAFWhitelistConfig `json:"whitelist"`
	BlockRules   ProxyRouteWAFCustomRules     `json:"block_rules"`
}

var wafBuiltinRuleValues = map[string]bool{
	"sqli":            true,
	"xss":             true,
	"path_traversal":  true,
	"sensitive_paths": true,
	"bad_bots":        true,
}

func defaultWAFConfig() ProxyRouteWAFConfig {
	return ProxyRouteWAFConfig{
		BuiltinRules: []string{"sqli", "xss", "path_traversal", "sensitive_paths", "bad_bots"},
		Whitelist: ProxyRouteWAFWhitelistConfig{
			IPs:     []string{},
			IPCidrs: []string{},
			Paths:   []string{},
		},
		BlockRules: ProxyRouteWAFCustomRules{
			PathContains:   []string{},
			PathRegexes:    []string{},
			QueryContains:  []string{},
			HeaderContains: []string{},
			UserAgents:     []string{},
		},
	}
}

func normalizeWAFMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case proxyRouteWAFModeLog, proxyRouteWAFModeBlock:
		return mode
	default:
		return proxyRouteWAFModeBlock
	}
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeWAFConfig(enabled bool, raw string) (ProxyRouteWAFConfig, error) {
	cfg := defaultWAFConfig()
	if !enabled {
		return cfg, nil
	}
	text := strings.TrimSpace(raw)
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &cfg); err != nil {
			return cfg, errors.New("waf_config 格式无效")
		}
	}

	cfg.BuiltinRules = normalizeStringList(cfg.BuiltinRules)
	for _, rule := range cfg.BuiltinRules {
		if !wafBuiltinRuleValues[rule] {
			return cfg, fmt.Errorf("waf_config.builtin_rules 不支持规则: %s", rule)
		}
	}

	cfg.Whitelist.IPs = normalizeStringList(cfg.Whitelist.IPs)
	cfg.Whitelist.IPCidrs = normalizeStringList(cfg.Whitelist.IPCidrs)
	cfg.Whitelist.Paths = normalizeStringList(cfg.Whitelist.Paths)
	cfg.BlockRules.PathContains = normalizeStringList(cfg.BlockRules.PathContains)
	cfg.BlockRules.PathRegexes = normalizeStringList(cfg.BlockRules.PathRegexes)
	cfg.BlockRules.QueryContains = normalizeStringList(cfg.BlockRules.QueryContains)
	cfg.BlockRules.HeaderContains = normalizeStringList(cfg.BlockRules.HeaderContains)
	cfg.BlockRules.UserAgents = normalizeStringList(cfg.BlockRules.UserAgents)

	for _, ip := range cfg.Whitelist.IPs {
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("waf_config 白名单 IP 格式无效: %s", ip)
		}
	}
	for _, cidr := range cfg.Whitelist.IPCidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return cfg, fmt.Errorf("waf_config 白名单 IP CIDR 格式无效: %s", cidr)
		}
	}
	for _, path := range cfg.Whitelist.Paths {
		if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n") {
			return cfg, fmt.Errorf("waf_config 白名单路径格式无效: %s", path)
		}
	}
	for _, re := range cfg.BlockRules.PathRegexes {
		if _, err := regexp.Compile(re); err != nil {
			return cfg, fmt.Errorf("waf_config 路径正则格式无效: %s", re)
		}
	}

	return cfg, nil
}

func decodeStoredWAFConfig(enabled bool, raw string) (*ProxyRouteWAFConfig, error) {
	cfg, err := normalizeWAFConfig(enabled, raw)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
