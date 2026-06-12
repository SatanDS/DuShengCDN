package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type ProxyRoute struct {
	ID                         uint       `json:"id" gorm:"primaryKey"`
	SiteName                   string     `json:"site_name" gorm:"size:255;not null;default:''"`
	Domain                     string     `json:"domain" gorm:"uniqueIndex;size:255;not null"`
	Domains                    string     `json:"domains" gorm:"type:text;not null;default:'[]'"`
	OriginID                   *uint      `json:"origin_id" gorm:"index"`
	OriginURL                  string     `json:"origin_url" gorm:"size:2048;not null"`
	OriginHost                 string     `json:"origin_host" gorm:"size:255"`
	OriginHostHeader           string     `json:"origin_host_header" gorm:"size:255;not null;default:''"`
	OriginSNI                  string     `json:"origin_sni" gorm:"size:255;not null;default:''"`
	OriginTLSVerify            bool       `json:"origin_tls_verify" gorm:"not null;default:true"`
	OriginCABundle             string     `json:"origin_ca_bundle" gorm:"type:text;not null;default:''"`
	OriginResolveMode          string     `json:"origin_resolve_mode" gorm:"size:32;not null;default:'publish_resolve'"`
	Upstreams                  string     `json:"upstreams" gorm:"type:text;not null;default:'[]'"`
	NodePool                   string     `json:"node_pool" gorm:"size:64;not null;default:'default'"`
	Enabled                    bool       `json:"enabled" gorm:"not null;default:true"`
	EnableHTTPS                bool       `json:"enable_https" gorm:"column:enable_https;not null;default:false"`
	CertID                     *uint      `json:"cert_id"`
	CertIDs                    string     `json:"cert_ids" gorm:"type:text;not null;default:'[]'"`
	DomainCertIDs              string     `json:"domain_cert_ids" gorm:"type:text;not null;default:'[]'"`
	RedirectHTTP               bool       `json:"redirect_http" gorm:"not null;default:false"`
	LimitConnPerServer         int        `json:"limit_conn_per_server" gorm:"not null;default:0"`
	LimitConnPerIP             int        `json:"limit_conn_per_ip" gorm:"not null;default:0"`
	LimitRate                  string     `json:"limit_rate" gorm:"size:32;not null;default:''"`
	ProxyBufferingMode         string     `json:"proxy_buffering_mode" gorm:"size:16;not null;default:'default'"`
	CacheEnabled               bool       `json:"cache_enabled" gorm:"not null;default:false"`
	CachePolicy                string     `json:"cache_policy" gorm:"size:32;not null;default:''"`
	CacheRules                 string     `json:"cache_rules" gorm:"type:text;not null;default:'[]'"`
	CustomHeaders              string     `json:"custom_headers" gorm:"type:text;not null;default:'[]'"`
	PoWEnabled                 bool       `json:"pow_enabled" gorm:"column:pow_enabled;not null;default:false"`
	PoWConfig                  string     `json:"pow_config" gorm:"column:pow_config;type:text;not null;default:'{}'"`
	WAFEnabled                 bool       `json:"waf_enabled" gorm:"column:waf_enabled;not null;default:false"`
	WAFMode                    string     `json:"waf_mode" gorm:"column:waf_mode;size:16;not null;default:'block'"`
	WAFConfig                  string     `json:"waf_config" gorm:"column:waf_config;type:text;not null;default:'{}'"`
	CCEnabled                  bool       `json:"cc_enabled" gorm:"column:cc_enabled;not null;default:false"`
	CCMode                     string     `json:"cc_mode" gorm:"column:cc_mode;size:16;not null;default:'block'"`
	CCConfig                   string     `json:"cc_config" gorm:"column:cc_config;type:text;not null;default:'{}'"`
	BasicAuthEnabled           bool       `json:"basic_auth_enabled" gorm:"not null;default:false"`
	BasicAuthUsername          string     `json:"basic_auth_username" gorm:"size:255;not null;default:''"`
	BasicAuthPasswordHash      string     `json:"-" gorm:"size:64;not null;default:''"`
	BasicAuthPasswordUpdatedAt *time.Time `json:"basic_auth_password_updated_at"`
	BasicAuthPassword          string     `json:"-" gorm:"-"`
	RegionRestrictionEnabled   bool       `json:"region_restriction_enabled" gorm:"not null;default:false"`
	RegionRestrictionMode      string     `json:"region_restriction_mode" gorm:"size:16;not null;default:'block'"`
	RegionRestrictionCountries string     `json:"region_restriction_countries" gorm:"type:text;not null;default:'[]'"`
	DNSAutoSync                bool       `json:"dns_auto_sync" gorm:"not null;default:false"`
	DNSAccountID               *uint      `json:"dns_account_id" gorm:"index"`
	DNSZoneID                  string     `json:"dns_zone_id" gorm:"size:128;not null;default:''"`
	DNSRecordType              string     `json:"dns_record_type" gorm:"size:16;not null;default:'A'"`
	DNSRecordName              string     `json:"dns_record_name" gorm:"size:255;not null;default:''"`
	DNSRecordContent           string     `json:"dns_record_content" gorm:"type:text;not null;default:''"`
	DNSAutoTarget              bool       `json:"dns_auto_target" gorm:"not null;default:false"`
	DNSTargetCount             int        `json:"dns_target_count" gorm:"not null;default:1"`
	DNSScheduleMode            string     `json:"dns_schedule_mode" gorm:"size:32;not null;default:'healthy'"`
	DNSTTL                     int        `json:"dns_ttl" gorm:"not null;default:1"`
	DNSProviderMode            string     `json:"dns_provider_mode" gorm:"size:32;not null;default:'cloudflare'"`
	DNSZoneIDRef               *uint      `json:"dns_zone_id_ref" gorm:"index"`
	GSLBEnabled                bool       `json:"gslb_enabled" gorm:"not null;default:false"`
	GSLBPolicy                 string     `json:"gslb_policy" gorm:"type:text;not null;default:'{}'"`
	DNSRecordIDs               string     `json:"dns_record_ids" gorm:"type:text;not null;default:'{}'"`
	CloudflareProxied          bool       `json:"cloudflare_proxied" gorm:"not null;default:false"`
	DDOSProtectionMode         string     `json:"ddos_protection_mode" gorm:"size:16;not null;default:'off'"`
	DDOSProtectionProvider     string     `json:"ddos_protection_provider" gorm:"size:32;not null;default:'cloudflare'"`
	DDOSProtectionTarget       string     `json:"ddos_protection_target" gorm:"size:128;not null;default:''"`
	DNSLastSyncStatus          string     `json:"dns_last_sync_status" gorm:"size:16;not null;default:''"`
	DNSLastSyncMessage         string     `json:"dns_last_sync_message" gorm:"type:text"`
	DNSLastSyncedAt            *time.Time `json:"dns_last_synced_at"`
	Remark                     string     `json:"remark" gorm:"size:255"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

func ListProxyRoutes() (routes []*ProxyRoute, err error) {
	err = DB.Order("id desc").Find(&routes).Error
	return routes, err
}

func ListProxyRouteCertificateReferenceFields() (routes []*ProxyRoute, err error) {
	err = DB.Select("id", "cert_id", "cert_ids", "domain_cert_ids").Order("id desc").Find(&routes).Error
	return routes, err
}

func GetEnabledProxyRoutes() (routes []*ProxyRoute, err error) {
	err = DB.Where("enabled = ?", true).Order("site_name asc").Order("domain asc").Find(&routes).Error
	return routes, err
}

func GetProxyRouteByID(id uint) (*ProxyRoute, error) {
	route := &ProxyRoute{}
	err := DB.First(route, id).Error
	return route, err
}

func ListProxyRoutesByOriginID(originID uint) (routes []*ProxyRoute, err error) {
	err = DB.Where("origin_id = ?", originID).Order("id desc").Find(&routes).Error
	return routes, err
}

func ListProxyRoutesByIDs(ids []uint) (routes []*ProxyRoute, err error) {
	if len(ids) == 0 {
		return []*ProxyRoute{}, nil
	}
	err = DB.Where("id IN ?", ids).Find(&routes).Error
	return routes, err
}

func ListProxyRouteIdentityCandidates(siteName string, domains []string) (routes []*ProxyRoute, err error) {
	conditions := make([]string, 0, len(domains)+3)
	args := make([]any, 0, len(domains)+3)
	siteName = strings.TrimSpace(siteName)
	hasNormalizedDomains := proxyRouteRuntimeTableAvailable(DB, &ProxySiteDomain{})
	if siteName != "" {
		conditions = append(conditions, "proxy_routes.site_name = ?", "proxy_routes.domain = ?")
		args = append(args, siteName, siteName)
	}

	domainValues := make([]string, 0, len(domains))
	seenDomains := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		domain = normalizeProxyRouteDomainForMigration(domain)
		if domain == "" {
			continue
		}
		if _, ok := seenDomains[domain]; ok {
			continue
		}
		seenDomains[domain] = struct{}{}
		domainValues = append(domainValues, domain)
	}
	if len(domainValues) > 0 {
		conditions = append(conditions, "proxy_routes.domain IN ?")
		args = append(args, domainValues)
		if hasNormalizedDomains {
			conditions = append(conditions, "proxy_site_domains.domain IN ?")
			args = append(args, domainValues)
		} else {
			for _, domain := range domainValues {
				conditions = append(conditions, "domains LIKE ?")
				args = append(args, "%\""+domain+"\"%")
			}
		}
	}
	if len(conditions) == 0 {
		return []*ProxyRoute{}, nil
	}

	query := DB.Model(&ProxyRoute{})
	if len(domainValues) > 0 && hasNormalizedDomains {
		query = query.Distinct("proxy_routes.*").
			Joins("LEFT JOIN proxy_site_domains ON proxy_site_domains.proxy_route_id = proxy_routes.id")
	}
	err = query.Where(strings.Join(conditions, " OR "), args...).Order("proxy_routes.id asc").Find(&routes).Error
	return routes, err
}

func (route *ProxyRoute) Insert() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return route.InsertWithDB(tx)
	})
}

func (route *ProxyRoute) InsertWithDB(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	route.prepareBasicAuthStorage()
	enabled := route.Enabled
	originTLSVerify := route.OriginTLSVerify
	if err := db.Create(route).Error; err != nil {
		return err
	}
	if !enabled {
		if err := db.Model(route).UpdateColumn("enabled", false).Error; err != nil {
			return err
		}
		route.Enabled = false
	}
	if !originTLSVerify {
		if err := db.Model(route).UpdateColumn("origin_tls_verify", false).Error; err != nil {
			return err
		}
		route.OriginTLSVerify = false
	}
	return SyncProxyRouteNormalizedTablesWithDB(db, route)
}

func (route *ProxyRoute) Update() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return route.UpdateWithDB(tx)
	})
}

func (route *ProxyRoute) UpdateWithDB(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	route.prepareBasicAuthStorage()
	var basicAuthPasswordUpdatedAt any = gorm.Expr("NULL")
	if route.BasicAuthPasswordUpdatedAt != nil {
		basicAuthPasswordUpdatedAt = route.BasicAuthPasswordUpdatedAt
	}
	if err := db.Model(&ProxyRoute{}).Where("id = ?", route.ID).Updates(map[string]any{
		"site_name":                      route.SiteName,
		"domain":                         route.Domain,
		"domains":                        route.Domains,
		"origin_id":                      route.OriginID,
		"origin_url":                     route.OriginURL,
		"origin_host":                    route.OriginHost,
		"origin_host_header":             route.OriginHostHeader,
		"origin_sni":                     route.OriginSNI,
		"origin_tls_verify":              route.OriginTLSVerify,
		"origin_ca_bundle":               route.OriginCABundle,
		"origin_resolve_mode":            route.OriginResolveMode,
		"upstreams":                      route.Upstreams,
		"node_pool":                      route.NodePool,
		"enabled":                        route.Enabled,
		"enable_https":                   route.EnableHTTPS,
		"cert_id":                        route.CertID,
		"cert_ids":                       route.CertIDs,
		"domain_cert_ids":                route.DomainCertIDs,
		"redirect_http":                  route.RedirectHTTP,
		"limit_conn_per_server":          route.LimitConnPerServer,
		"limit_conn_per_ip":              route.LimitConnPerIP,
		"limit_rate":                     route.LimitRate,
		"proxy_buffering_mode":           route.ProxyBufferingMode,
		"cache_enabled":                  route.CacheEnabled,
		"cache_policy":                   route.CachePolicy,
		"cache_rules":                    route.CacheRules,
		"custom_headers":                 route.CustomHeaders,
		"pow_enabled":                    route.PoWEnabled,
		"pow_config":                     route.PoWConfig,
		"waf_enabled":                    route.WAFEnabled,
		"waf_mode":                       route.WAFMode,
		"waf_config":                     route.WAFConfig,
		"cc_enabled":                     route.CCEnabled,
		"cc_mode":                        route.CCMode,
		"cc_config":                      route.CCConfig,
		"basic_auth_enabled":             route.BasicAuthEnabled,
		"basic_auth_username":            route.BasicAuthUsername,
		"basic_auth_password_hash":       route.BasicAuthPasswordHash,
		"basic_auth_password_updated_at": basicAuthPasswordUpdatedAt,
		"region_restriction_enabled":     route.RegionRestrictionEnabled,
		"region_restriction_mode":        route.RegionRestrictionMode,
		"region_restriction_countries":   route.RegionRestrictionCountries,
		"dns_auto_sync":                  route.DNSAutoSync,
		"dns_account_id":                 route.DNSAccountID,
		"dns_zone_id":                    route.DNSZoneID,
		"dns_record_type":                route.DNSRecordType,
		"dns_record_name":                route.DNSRecordName,
		"dns_record_content":             route.DNSRecordContent,
		"dns_auto_target":                route.DNSAutoTarget,
		"dns_target_count":               route.DNSTargetCount,
		"dns_schedule_mode":              route.DNSScheduleMode,
		"dns_ttl":                        route.DNSTTL,
		"dns_provider_mode":              route.DNSProviderMode,
		"dns_zone_id_ref":                route.DNSZoneIDRef,
		"gslb_enabled":                   route.GSLBEnabled,
		"gslb_policy":                    route.GSLBPolicy,
		"dns_record_ids":                 route.DNSRecordIDs,
		"cloudflare_proxied":             route.CloudflareProxied,
		"ddos_protection_mode":           route.DDOSProtectionMode,
		"ddos_protection_provider":       route.DDOSProtectionProvider,
		"ddos_protection_target":         route.DDOSProtectionTarget,
		"dns_last_sync_status":           route.DNSLastSyncStatus,
		"dns_last_sync_message":          route.DNSLastSyncMessage,
		"dns_last_synced_at":             route.DNSLastSyncedAt,
		"remark":                         route.Remark,
	}).Error; err != nil {
		return err
	}
	return SyncProxyRouteNormalizedTablesWithDB(db, route)
}

func (route *ProxyRoute) prepareBasicAuthStorage() {
	if route == nil {
		return
	}
	if !route.BasicAuthEnabled {
		route.BasicAuthUsername = ""
		route.BasicAuthPasswordHash = ""
		route.BasicAuthPasswordUpdatedAt = nil
		route.BasicAuthPassword = ""
		return
	}
	route.BasicAuthUsername = strings.TrimSpace(route.BasicAuthUsername)
	route.BasicAuthPasswordHash = strings.TrimSpace(route.BasicAuthPasswordHash)
	if route.BasicAuthPasswordHash == "" && strings.TrimSpace(route.BasicAuthPassword) != "" {
		route.BasicAuthPasswordHash = BasicAuthCredentialHash(route.BasicAuthUsername, route.BasicAuthPassword)
		if route.BasicAuthPasswordUpdatedAt == nil {
			now := time.Now()
			route.BasicAuthPasswordUpdatedAt = &now
		}
	}
	if route.BasicAuthPasswordHash != "" && route.BasicAuthPasswordUpdatedAt == nil {
		now := time.Now()
		route.BasicAuthPasswordUpdatedAt = &now
	}
	route.BasicAuthPassword = ""
}

func (route *ProxyRoute) Delete() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return route.DeleteWithDB(tx)
	})
}

func (route *ProxyRoute) DeleteWithDB(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if route != nil && route.ID != 0 {
		if err := DeleteProxyRouteNormalizedTablesWithDB(db, route.ID); err != nil {
			return err
		}
	}
	return db.Delete(route).Error
}
