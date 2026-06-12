package service

import (
	"errors"
	"strings"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func buildProxyRouteViews(routes []*model.ProxyRoute) ([]*ProxyRouteView, error) {
	context, err := newProxyRouteViewBuildContext(routes)
	if err != nil {
		return nil, err
	}
	views := make([]*ProxyRouteView, 0, len(routes))
	for _, route := range routes {
		view, err := buildProxyRouteViewWithContext(route, context)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func buildProxyRouteView(route *model.ProxyRoute) (*ProxyRouteView, error) {
	return buildProxyRouteViewWithContext(route, nil)
}

func buildProxyRouteViewWithContext(route *model.ProxyRoute, context *proxyRouteViewBuildContext) (*ProxyRouteView, error) {
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
	ccConfig, err := decodeStoredCCConfig(route.CCEnabled, route.CCConfig)
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
	domainCertIDs, err := resolveProxyRouteDomainCertIDsWithContext(route, domains, certIDs, context)
	if err != nil {
		return nil, err
	}
	var certID *uint
	if len(certIDs) > 0 {
		certID = &certIDs[0]
	}
	primaryDomain := domains[0]
	originHostHeader := normalizeStoredOriginHostHeader(route)
	originResolveMode := normalizeStoredOriginResolveMode(route.OriginResolveMode)
	routeRules, err := buildProxyRouteRuleViews(route, context)
	if err != nil {
		return nil, err
	}
	return &ProxyRouteView{
		ID:                          route.ID,
		SiteName:                    normalizeProxyRouteSiteNameInput(route, route.SiteName, primaryDomain),
		Domain:                      primaryDomain,
		Domains:                     domains,
		PrimaryDomain:               primaryDomain,
		DomainCount:                 len(domains),
		OriginID:                    route.OriginID,
		OriginURL:                   route.OriginURL,
		OriginHost:                  originHostHeader,
		OriginHostHeader:            originHostHeader,
		OriginSNI:                   strings.TrimSpace(route.OriginSNI),
		OriginTLSVerify:             normalizeStoredOriginTLSVerify(route),
		OriginCABundle:              strings.TrimSpace(route.OriginCABundle),
		OriginResolveMode:           originResolveMode,
		Upstreams:                   route.Upstreams,
		UpstreamList:                upstreams,
		NodePool:                    normalizeNodePoolName(route.NodePool),
		Enabled:                     route.Enabled,
		EnableHTTPS:                 route.EnableHTTPS,
		CertID:                      certID,
		CertIDs:                     certIDs,
		DomainCertIDs:               domainCertIDs,
		RedirectHTTP:                route.RedirectHTTP,
		LimitConnPerServer:          route.LimitConnPerServer,
		LimitConnPerIP:              route.LimitConnPerIP,
		LimitRate:                   route.LimitRate,
		ProxyBufferingMode:          normalizeProxyRouteProxyBufferingMode(route.ProxyBufferingMode),
		CacheEnabled:                route.CacheEnabled,
		CachePolicy:                 route.CachePolicy,
		CacheRules:                  route.CacheRules,
		CacheRuleList:               cacheRules,
		CustomHeaders:               marshalCustomHeadersForView(customHeaders),
		CustomHeaderList:            redactSensitiveCustomHeaders(customHeaders),
		RouteRules:                  routeRules,
		PoWEnabled:                  route.PoWEnabled,
		PoWConfig:                   powConfig,
		WAFEnabled:                  route.WAFEnabled,
		WAFMode:                     normalizeWAFMode(route.WAFMode),
		WAFConfig:                   wafConfig,
		CCEnabled:                   route.CCEnabled,
		CCMode:                      normalizeCCMode(route.CCMode),
		CCConfig:                    ccConfig,
		BasicAuthEnabled:            route.BasicAuthEnabled,
		BasicAuthUsername:           route.BasicAuthUsername,
		BasicAuthPasswordConfigured: route.BasicAuthEnabled && strings.TrimSpace(proxyRouteBasicAuthPasswordHashForView(route)) != "",
		BasicAuthPasswordUpdatedAt:  route.BasicAuthPasswordUpdatedAt,
		RegionRestrictionEnabled:    route.RegionRestrictionEnabled,
		RegionRestrictionMode:       normalizeProxyRouteRegionRestrictionMode(route.RegionRestrictionMode),
		RegionRestrictionCountries:  regionCountries,
		DNSAutoSync:                 route.DNSAutoSync,
		DNSAccountID:                route.DNSAccountID,
		DNSZoneID:                   route.DNSZoneID,
		DNSRecordType:               normalizeDNSRecordType(route.DNSRecordType),
		DNSRecordName:               route.DNSRecordName,
		DNSRecordContent:            route.DNSRecordContent,
		DNSAutoTarget:               route.DNSAutoTarget,
		DNSTargetCount:              normalizeDNSTargetCount(route.DNSTargetCount),
		DNSScheduleMode:             normalizeDNSScheduleMode(route.DNSScheduleMode),
		DNSTTL:                      normalizeDNSTTL(route.DNSTTL),
		DNSProviderMode:             normalizeDNSProviderMode(route.DNSProviderMode),
		DNSZoneIDRef:                route.DNSZoneIDRef,
		GSLBEnabled:                 route.GSLBEnabled,
		GSLBPolicy:                  gslbPolicy,
		DNSRecordIDs:                decodeDNSRecordIDs(route.DNSRecordIDs),
		CloudflareProxied:           route.CloudflareProxied,
		DDOSProtectionMode:          normalizeDDOSProtectionMode(route.DDOSProtectionMode),
		DDOSProtectionProvider:      normalizeDDOSProtectionProvider(route.DDOSProtectionProvider),
		DDOSProtectionTarget:        strings.TrimSpace(route.DDOSProtectionTarget),
		DNSLastSyncStatus:           route.DNSLastSyncStatus,
		DNSLastSyncMessage:          route.DNSLastSyncMessage,
		DNSLastSyncedAt:             route.DNSLastSyncedAt,
		Remark:                      route.Remark,
		CreatedAt:                   route.CreatedAt,
		UpdatedAt:                   route.UpdatedAt,
	}, nil
}

type proxyRouteTLSCertificateLoader interface {
	loadTLSCertificates([]uint) ([]*model.TLSCertificate, error)
}

type proxyRouteViewBuildContext struct {
	certificatesByID     map[uint]*model.TLSCertificate
	ruleConfigsByRouteID map[uint][]proxyRouteRuleConfig
}

func (context *proxyRouteViewBuildContext) loadProxyRouteRuleConfigs(route *model.ProxyRoute) ([]proxyRouteRuleConfig, error) {
	if context == nil {
		return loadProxyRouteRuleConfigs(route)
	}
	return loadPreloadedProxyRouteRuleConfigs(context.ruleConfigsByRouteID, route)
}

func newProxyRouteViewBuildContext(routes []*model.ProxyRoute) (*proxyRouteViewBuildContext, error) {
	routeIDs := make([]uint, 0, len(routes))
	certIDs := make([]uint, 0)
	seenCertIDs := make(map[uint]struct{})
	for _, route := range routes {
		if route == nil {
			continue
		}
		if route.ID != 0 {
			routeIDs = append(routeIDs, route.ID)
		}
		domains, err := decodeStoredDomains(route.Domains, route.Domain)
		if err != nil {
			return nil, err
		}
		routeCertIDs, err := decodeStoredCertIDs(route.CertIDs, route.CertID)
		if err != nil {
			return nil, err
		}
		domainCertIDs, err := decodeStoredDomainCertIDs(route.DomainCertIDs, len(domains))
		if err != nil {
			return nil, err
		}
		if len(domainCertIDs) > 0 || len(routeCertIDs) == 0 {
			continue
		}
		for _, certID := range routeCertIDs {
			if certID == 0 {
				continue
			}
			if _, ok := seenCertIDs[certID]; ok {
				continue
			}
			seenCertIDs[certID] = struct{}{}
			certIDs = append(certIDs, certID)
		}
	}

	ruleConfigsByRouteID, err := loadProxyRouteRuleConfigsByRouteIDs(routeIDs)
	if err != nil {
		return nil, err
	}
	certificates, err := loadTLSCertificates(certIDs)
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
	return &proxyRouteViewBuildContext{
		certificatesByID:     certificatesByID,
		ruleConfigsByRouteID: ruleConfigsByRouteID,
	}, nil
}

func (context *proxyRouteViewBuildContext) loadTLSCertificates(certIDs []uint) ([]*model.TLSCertificate, error) {
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
