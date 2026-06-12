package service

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service/configversion"
	"dushengcdn/service/openresty"

	"gorm.io/gorm"
)

var routeUpstreamLookupIPAddr = net.DefaultResolver.LookupIPAddr

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

func loadProxyRouteRuleConfigs(route *model.ProxyRoute) ([]proxyRouteRuleConfig, error) {
	if route == nil || route.ID == 0 {
		return nil, nil
	}
	configsByRouteID, err := loadProxyRouteRuleConfigsByRouteIDs([]uint{route.ID})
	if err != nil {
		return nil, err
	}
	return configsByRouteID[route.ID], nil
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

// Keep the service-level renderer wrappers as compatibility entry points for tests
// and callers in this package; the renderer implementation lives in openresty.
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
	return openresty.RenderRouteConfig(renderRoutes, openresty.ConfigSnapshot(cfg), openRestyRenderOptionsWithContext(context))
}

type routeConfigQueries struct {
	ListTLSCertificatesByIDs func([]uint) ([]*model.TLSCertificate, error)
}

var defaultRouteConfigQueries = routeConfigQueries{
	ListTLSCertificatesByIDs: model.ListTLSCertificatesByIDs,
}

type routeConfigRenderContext struct {
	certificatesByID     map[uint]*model.TLSCertificate
	ruleConfigsByRouteID map[uint][]proxyRouteRuleConfig
	resolvedUpstreams    map[string][]string
}

func (context *routeConfigRenderContext) loadProxyRouteRuleConfigs(route *model.ProxyRoute) ([]proxyRouteRuleConfig, error) {
	if context == nil {
		return loadProxyRouteRuleConfigs(route)
	}
	return loadPreloadedProxyRouteRuleConfigs(context.ruleConfigsByRouteID, route)
}

func openRestyRenderOptions() openresty.RenderOptions {
	return openresty.RenderOptions{
		LookupIPAddr:    routeUpstreamLookupIPAddr,
		LookupIPTimeout: 5 * time.Second,
	}
}

type openRestyRenderOptionsLoader interface {
	openRestyRenderOptions() openresty.RenderOptions
}

func (context *routeConfigRenderContext) openRestyRenderOptions() openresty.RenderOptions {
	options := openRestyRenderOptions()
	if context == nil {
		return options
	}
	if context.resolvedUpstreams == nil {
		context.resolvedUpstreams = make(map[string][]string)
	}
	options.ResolvedUpstreams = context.resolvedUpstreams
	return options
}

func openRestyRenderOptionsWithContext(context proxyRouteTLSCertificateLoader) openresty.RenderOptions {
	if loader, ok := context.(openRestyRenderOptionsLoader); ok {
		return loader.openRestyRenderOptions()
	}
	return openRestyRenderOptions()
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
		regionCountries, err := decodeStoredRegionRestrictionCountries(route.RegionRestrictionCountries)
		if err != nil {
			return nil, fmt.Errorf("路由 %s 地区限制配置无效", route.Domain)
		}
		renderRoute := openresty.Route{
			ID:                         route.ID,
			SiteName:                   route.SiteName,
			Domain:                     route.Domain,
			Domains:                    domains,
			OriginURL:                  route.OriginURL,
			OriginHost:                 normalizeStoredOriginHostHeader(route),
			OriginHostHeader:           normalizeStoredOriginHostHeader(route),
			OriginSNI:                  strings.TrimSpace(route.OriginSNI),
			OriginTLSVerify:            boolPointer(normalizeStoredOriginTLSVerify(route)),
			OriginCABundle:             strings.TrimSpace(route.OriginCABundle),
			OriginResolveMode:          normalizeStoredOriginResolveMode(route.OriginResolveMode),
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
		routeRules, err := buildOpenRestyRouteRules(route, context)
		if err != nil {
			return nil, fmt.Errorf("route %s route_rules are invalid: %w", route.Domain, err)
		}
		renderRoute.Rules = routeRules
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

func buildOpenRestyRouteRules(route *model.ProxyRoute, context proxyRouteTLSCertificateLoader) ([]openresty.RouteRule, error) {
	configs, err := loadProxyRouteRuleConfigsWithContext(context, route)
	if err != nil {
		return nil, err
	}
	result := make([]openresty.RouteRule, 0, len(configs))
	for _, config := range configs {
		rule := config.Rule
		if rule.MatchType == proxyRouteRuleMatchDefault {
			continue
		}
		renderRule := openresty.RouteRule{
			ID:                rule.ID,
			MatchType:         rule.MatchType,
			Path:              rule.Path,
			Priority:          rule.Priority,
			Enabled:           rule.Enabled,
			OriginURL:         firstString(config.Upstreams),
			OriginHostHeader:  strings.TrimSpace(rule.OriginHostHeader),
			OriginSNI:         strings.TrimSpace(rule.OriginSNI),
			OriginTLSVerify:   &rule.OriginTLSVerify,
			OriginCABundle:    strings.TrimSpace(rule.OriginCABundle),
			OriginResolveMode: strings.TrimSpace(rule.OriginResolveMode),
			LimitRate:         strings.TrimSpace(rule.LimitRate),
			ProxyBufferingMode: normalizeProxyRouteProxyBufferingMode(
				rule.ProxyBufferingMode,
			),
			CustomHeaders: toOpenRestyCustomHeaders(config.CustomHeaders),
		}
		if len(config.Upstreams) > 0 {
			renderRule.Upstreams = config.Upstreams
		}
		limitConnPerServer := rule.LimitConnPerServer
		renderRule.LimitConnPerServer = &limitConnPerServer
		limitConnPerIP := rule.LimitConnPerIP
		renderRule.LimitConnPerIP = &limitConnPerIP
		if config.CachePolicy != nil {
			cacheEnabled := config.CachePolicy.Enabled
			renderRule.CacheEnabled = &cacheEnabled
			renderRule.CachePolicy = config.CachePolicy.Policy
			renderRule.CacheRules = config.CacheRules
		}
		if config.SecurityPolicy != nil {
			basicAuthEnabled := config.SecurityPolicy.BasicAuthEnabled
			renderRule.BasicAuthEnabled = &basicAuthEnabled
			renderRule.BasicAuthUsername = config.SecurityPolicy.BasicAuthUsername
			renderRule.BasicAuthPasswordHash = config.SecurityPolicy.BasicAuthPasswordHash
		}
		result = append(result, renderRule)
	}
	return result, nil
}

func toOpenRestyTLSCertificates(certificates []*model.TLSCertificate) []openresty.TLSCertificate {
	result := make([]openresty.TLSCertificate, 0, len(certificates))
	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		result = append(result, toOpenRestyTLSCertificate(certificate))
	}
	return result
}

func newRouteConfigRenderContext(routes []*model.ProxyRoute, queries routeConfigQueries) (*routeConfigRenderContext, error) {
	routeIDs := make([]uint, 0, len(routes))
	certIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for _, route := range routes {
		if route == nil {
			continue
		}
		if route.ID != 0 {
			routeIDs = append(routeIDs, route.ID)
		}
		if !route.EnableHTTPS {
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
	ruleConfigsByRouteID, err := loadProxyRouteRuleConfigsByRouteIDs(routeIDs)
	if err != nil {
		return nil, err
	}
	context := &routeConfigRenderContext{
		certificatesByID:     map[uint]*model.TLSCertificate{},
		ruleConfigsByRouteID: ruleConfigsByRouteID,
		resolvedUpstreams:    map[string][]string{},
	}
	if len(certIDs) == 0 {
		return context, nil
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
	context.certificatesByID = certificatesByID
	return context, nil
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
	return openresty.RenderRouteConfig(renderRoutes, openresty.ConfigSnapshot(cfg), context.openRestyRenderOptions())
}

func renderMainConfig(cfg openRestyConfigSnapshot) string {
	return openresty.RenderMainConfig(openresty.ConfigSnapshot(cfg))
}

func ValidateOpenRestyMainConfigTemplate(templateText string) error {
	return openresty.ValidateMainConfigTemplate(templateText)
}

func basicAuthCredentialHash(username, password string) string {
	return openresty.BasicAuthCredentialHash(username, password)
}

func validateCertificateCoverage(certificate *model.TLSCertificate, domains []string) error {
	if certificate == nil {
		return errors.New("certificate is nil")
	}
	return openresty.ValidateCertificateCoverage(toOpenRestyTLSCertificate(certificate), domains)
}

func validateCertificateCoverageSet(certificates []*model.TLSCertificate, domains []string) error {
	if len(certificates) == 0 {
		return errors.New("certificate set is empty")
	}
	renderCertificates := make([]openresty.TLSCertificate, 0, len(certificates))
	for _, certificate := range certificates {
		if certificate == nil {
			return errors.New("certificate is nil")
		}
		renderCertificates = append(renderCertificates, toOpenRestyTLSCertificate(certificate))
	}
	for _, domain := range domains {
		covered := false
		for _, certificate := range renderCertificates {
			if openresty.ValidateCertificateCoverage(certificate, []string{domain}) == nil {
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

func toOpenRestyTLSCertificate(certificate *model.TLSCertificate) openresty.TLSCertificate {
	if certificate == nil {
		return openresty.TLSCertificate{}
	}
	return openresty.TLSCertificate{
		ID:      certificate.ID,
		CertPEM: certificate.CertPEM,
		KeyPEM:  certificate.KeyPEM,
	}
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

func certificateCertFileName(id uint) string {
	return openresty.CertFileName(id)
}

func certificateKeyFileName(id uint) string {
	return openresty.KeyFileName(id)
}
