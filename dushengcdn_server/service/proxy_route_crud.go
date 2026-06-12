package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

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
	if err := ensureCommercialProxyRouteInputFeaturesEnabled(input); err != nil {
		return nil, err
	}
	route, err := buildProxyRoute(nil, input)
	if err != nil {
		return nil, err
	}
	if err = withCommercialResourceCreation("site", func(tx *gorm.DB) error {
		if err := route.InsertWithDB(tx); err != nil {
			return err
		}
		if input.RouteRules != nil {
			return replaceProxyRouteRuleInputsWithDB(tx, route, input.RouteRules)
		}
		return nil
	}); err != nil {
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
	if err := ensureCommercialProxyRouteInputFeaturesEnabled(input); err != nil {
		return nil, err
	}
	route, err = buildProxyRoute(route, input)
	if err != nil {
		return nil, err
	}
	if err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := route.UpdateWithDB(tx); err != nil {
			return err
		}
		if input.RouteRules != nil {
			return replaceProxyRouteRuleInputsWithDB(tx, route, input.RouteRules)
		}
		return nil
	}); err != nil {
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
	originHostHeader := strings.TrimSpace(input.OriginHostHeader)
	if originHostHeader == "" {
		originHostHeader = strings.TrimSpace(input.OriginHost)
	}
	originHost := originHostHeader
	originSNI := strings.TrimSpace(input.OriginSNI)
	originTLSVerify := normalizeOriginTLSVerify(input.OriginTLSVerify)
	originCABundle := strings.TrimSpace(input.OriginCABundle)
	originResolveMode, err := normalizeOriginResolveMode(input.OriginResolveMode)
	if err != nil {
		return nil, err
	}
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
	customHeaders, err = restoreRedactedCustomHeaders(customHeaders, route)
	if err != nil {
		return nil, err
	}
	if err := validateCustomHeaderValues(customHeaders); err != nil {
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
	proxyBufferingMode := normalizeProxyRouteProxyBufferingMode(input.ProxyBufferingMode)

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

	ccMode := normalizeCCMode(input.CCMode)
	powConfigEnabled := input.PoWEnabled || (input.CCEnabled && ccMode == proxyRouteCCModePoW)
	powConfig, err := normalizePoWConfig(powConfigEnabled, input.PoWConfig)
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
	ccConfig, err := normalizeCCConfig(input.CCEnabled, input.CCConfig)
	if err != nil {
		return nil, err
	}
	ccConfigJSON, err := json.Marshal(ccConfig)
	if err != nil {
		return nil, err
	}

	if !input.EnableHTTPS {
		input.RedirectHTTP = false
		input.CertID = nil
		input.CertIDs = nil
		input.DomainCertIDs = nil
	}
	domainCertIDs, certIDs, primaryCertID, coverageValidated, err := normalizeProxyRouteDomainCertificateIDs(
		domains,
		input.EnableHTTPS,
		input.DomainCertIDs,
		input.CertID,
		input.CertIDs,
	)
	if err != nil {
		return nil, err
	}
	if !coverageValidated {
		if err := validateProxyRouteDomainCertificateCoverage(domains, domainCertIDs); err != nil {
			return nil, err
		}
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
	if err := validateOriginHostHeader(originHostHeader); err != nil {
		return nil, err
	}
	if err := validateOriginSNI(originSNI); err != nil {
		return nil, err
	}
	nodePool := normalizeNodePoolName(input.NodePool)
	input.DomainCertIDs = domainCertIDs
	input.CertIDs = certIDs
	input.CertID = primaryCertID
	if input.RedirectHTTP && !input.EnableHTTPS {
		return nil, errors.New("redirect_http requires enable_https")
	}

	basicAuthUsername, basicAuthPasswordHash, basicAuthPasswordUpdatedAt, err := normalizeProxyRouteBasicAuth(route, input)
	if err != nil {
		return nil, err
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
	route.OriginGroupID = input.OriginGroupID
	route.OriginURL = upstreams[0]
	route.OriginHost = originHost
	route.OriginHostHeader = originHostHeader
	route.OriginSNI = originSNI
	route.OriginTLSVerify = originTLSVerify
	route.OriginCABundle = originCABundle
	route.OriginResolveMode = originResolveMode
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
	route.ProxyBufferingMode = proxyBufferingMode
	route.CachePolicyID = input.CachePolicyID
	route.CacheEnabled = input.CacheEnabled
	route.CachePolicy = normalizeCachePolicy(input.CacheEnabled, cachePolicy)
	route.CacheRules = string(cacheRulesJSON)
	route.CustomHeaders = string(customHeadersJSON)
	route.PoWEnabled = input.PoWEnabled
	route.PoWConfig = string(powConfigJSON)
	route.WAFEnabled = input.WAFEnabled
	route.WAFMode = wafMode
	route.WAFConfig = string(wafConfigJSON)
	route.CCEnabled = input.CCEnabled
	route.CCMode = ccMode
	route.CCConfig = string(ccConfigJSON)
	route.BasicAuthEnabled = input.BasicAuthEnabled
	route.BasicAuthUsername = basicAuthUsername
	route.BasicAuthPasswordHash = basicAuthPasswordHash
	route.BasicAuthPasswordUpdatedAt = basicAuthPasswordUpdatedAt
	route.BasicAuthPassword = ""
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
	if err := ensureCommercialProxyRouteFeaturesEnabled(route); err != nil {
		return nil, err
	}
	return route, nil
}

func normalizeProxyRouteBasicAuth(route *model.ProxyRoute, input ProxyRouteInput) (string, string, *time.Time, error) {
	if !input.BasicAuthEnabled {
		return "", "", nil, nil
	}
	username := strings.TrimSpace(input.BasicAuthUsername)
	password := strings.TrimSpace(input.BasicAuthPassword)
	if username == "" {
		return "", "", nil, errors.New("basic_auth_username and basic_auth_password cannot be empty when basic auth is enabled")
	}
	if password != "" {
		now := time.Now()
		return username, model.BasicAuthCredentialHash(username, password), &now, nil
	}
	if route != nil && route.BasicAuthEnabled && strings.TrimSpace(route.BasicAuthUsername) == username {
		passwordHash := strings.TrimSpace(route.BasicAuthPasswordHash)
		if passwordHash == "" && strings.TrimSpace(route.BasicAuthPassword) != "" {
			passwordHash = model.BasicAuthCredentialHash(username, route.BasicAuthPassword)
		}
		if passwordHash != "" {
			updatedAt := route.BasicAuthPasswordUpdatedAt
			if updatedAt == nil {
				now := time.Now()
				updatedAt = &now
			}
			return username, passwordHash, updatedAt, nil
		}
	}
	return "", "", nil, errors.New("basic_auth_username and basic_auth_password cannot be empty when basic auth is enabled")
}

func ensureCommercialProxyRouteInputFeaturesEnabled(input ProxyRouteInput) error {
	features := make([]string, 0, 8)
	dnsProviderMode := normalizeDNSProviderMode(input.DNSProviderMode)
	if dnsProviderMode == DNSProviderModeAuthoritative {
		features = append(features, CommercialFeatureAuthoritativeDNS)
	}
	if input.DNSAutoSync && dnsProviderMode == DNSProviderModeCloudflare {
		features = append(features, CommercialFeatureCloudflareDNS)
	}
	gslbInputEnabled := input.GSLBEnabled && (dnsProviderMode == DNSProviderModeAuthoritative || input.DNSAutoSync)
	if gslbInputEnabled {
		features = append(features, CommercialFeatureGSLB)
	}
	if normalizeDDOSProtectionMode(input.DDOSProtectionMode) == DDOSProtectionModeAuto {
		features = append(features, CommercialFeatureDDoSProtection)
	}
	if input.WAFEnabled {
		features = append(features, CommercialFeatureWAF)
	}
	if input.CCEnabled || input.PoWEnabled {
		features = append(features, CommercialFeatureCCProtection)
	}
	if input.RegionRestrictionEnabled {
		features = append(features, CommercialFeatureCountryRegionAccessControl)
	}
	if gslbInputEnabled {
		features = append(features, commercialGSLBPolicyAccessControlFeatures(input.GSLBPolicy)...)
	}
	return ensureCommercialFeaturesEnabled(features...)
}

func ensureCommercialProxyRouteFeaturesEnabled(route *model.ProxyRoute) error {
	if route == nil {
		return nil
	}
	features := make([]string, 0, 8)
	if normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeAuthoritative {
		features = append(features, CommercialFeatureAuthoritativeDNS)
	}
	if route.DNSAutoSync && normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeCloudflare {
		features = append(features, CommercialFeatureCloudflareDNS)
	}
	if route.GSLBEnabled {
		features = append(features, CommercialFeatureGSLB)
	}
	if normalizeDDOSProtectionMode(route.DDOSProtectionMode) == DDOSProtectionModeAuto {
		features = append(features, CommercialFeatureDDoSProtection)
	}
	if route.WAFEnabled {
		features = append(features, CommercialFeatureWAF)
	}
	if route.CCEnabled || route.PoWEnabled {
		features = append(features, CommercialFeatureCCProtection)
	}
	if route.RegionRestrictionEnabled {
		features = append(features, CommercialFeatureCountryRegionAccessControl)
	}
	if route.GSLBEnabled {
		gslbPolicy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return err
		}
		features = append(features, commercialGSLBPolicyAccessControlFeatures(gslbPolicy)...)
	}
	return ensureCommercialFeaturesEnabled(features...)
}

func commercialGSLBPolicyAccessControlFeatures(policy ProxyRouteGSLBPolicy) []string {
	features := make([]string, 0, 4)
	hasCountry := false
	hasOperator := false
	hasSourceCIDR := false
	hasASN := false
	hasExplicitEnabledPool := false
	for _, pool := range policy.Pools {
		if pool.Enabled {
			hasExplicitEnabledPool = true
			break
		}
	}
	for _, pool := range policy.Pools {
		if hasExplicitEnabledPool && !pool.Enabled {
			continue
		}
		if hasNonBlankCommercialFeatureValues(pool.Countries) {
			hasCountry = true
		}
		if hasNonBlankCommercialFeatureValues(pool.Operators) {
			hasOperator = true
		}
		if hasNonBlankCommercialFeatureValues(pool.SourceCIDRs) {
			hasSourceCIDR = true
		}
		if hasCommercialFeatureASNValues(pool.ASNs) {
			hasASN = true
		}
	}
	if hasCountry {
		features = append(features, CommercialFeatureCountryRegionAccessControl)
	}
	if hasOperator {
		features = append(features, CommercialFeatureOperatorAccessControl)
	}
	if hasSourceCIDR {
		features = append(features, CommercialFeatureSourceCIDRAccessControl)
	}
	if hasASN {
		features = append(features, CommercialFeatureASNAccessControl)
	}
	return features
}

func hasNonBlankCommercialFeatureValues(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func hasCommercialFeatureASNValues(values []uint32) bool {
	for _, value := range values {
		if value > 0 {
			return true
		}
	}
	return false
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
