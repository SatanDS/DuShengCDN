package model

import "time"

type ProxyRoute struct {
	ID                         uint       `json:"id" gorm:"primaryKey"`
	SiteName                   string     `json:"site_name" gorm:"size:255;not null;default:''"`
	Domain                     string     `json:"domain" gorm:"uniqueIndex;size:255;not null"`
	Domains                    string     `json:"domains" gorm:"type:text;not null;default:'[]'"`
	OriginID                   *uint      `json:"origin_id" gorm:"index"`
	OriginURL                  string     `json:"origin_url" gorm:"size:2048;not null"`
	OriginHost                 string     `json:"origin_host" gorm:"size:255"`
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
	CacheEnabled               bool       `json:"cache_enabled" gorm:"not null;default:false"`
	CachePolicy                string     `json:"cache_policy" gorm:"size:32;not null;default:''"`
	CacheRules                 string     `json:"cache_rules" gorm:"type:text;not null;default:'[]'"`
	CustomHeaders              string     `json:"custom_headers" gorm:"type:text;not null;default:'[]'"`
	PoWEnabled                 bool       `json:"pow_enabled" gorm:"column:pow_enabled;not null;default:false"`
	PoWConfig                  string     `json:"pow_config" gorm:"column:pow_config;type:text;not null;default:'{}'"`
	WAFEnabled                 bool       `json:"waf_enabled" gorm:"column:waf_enabled;not null;default:false"`
	WAFMode                    string     `json:"waf_mode" gorm:"column:waf_mode;size:16;not null;default:'block'"`
	WAFConfig                  string     `json:"waf_config" gorm:"column:waf_config;type:text;not null;default:'{}'"`
	BasicAuthEnabled           bool       `json:"basic_auth_enabled" gorm:"not null;default:false"`
	BasicAuthUsername          string     `json:"basic_auth_username" gorm:"size:255;not null;default:''"`
	BasicAuthPassword          string     `json:"basic_auth_password" gorm:"size:255;not null;default:''"`
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
	DNSRecordIDs               string     `json:"dns_record_ids" gorm:"type:text;not null;default:'{}'"`
	CloudflareProxied          bool       `json:"cloudflare_proxied" gorm:"not null;default:false"`
	DDOSProtectionMode         string     `json:"ddos_protection_mode" gorm:"size:16;not null;default:'off'"`
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

func (route *ProxyRoute) Insert() error {
	return DB.Create(route).Error
}

func (route *ProxyRoute) Update() error {
	return DB.Model(&ProxyRoute{}).Where("id = ?", route.ID).Updates(map[string]any{
		"site_name":                    route.SiteName,
		"domain":                       route.Domain,
		"domains":                      route.Domains,
		"origin_id":                    route.OriginID,
		"origin_url":                   route.OriginURL,
		"origin_host":                  route.OriginHost,
		"upstreams":                    route.Upstreams,
		"node_pool":                    route.NodePool,
		"enabled":                      route.Enabled,
		"enable_https":                 route.EnableHTTPS,
		"cert_id":                      route.CertID,
		"cert_ids":                     route.CertIDs,
		"domain_cert_ids":              route.DomainCertIDs,
		"redirect_http":                route.RedirectHTTP,
		"limit_conn_per_server":        route.LimitConnPerServer,
		"limit_conn_per_ip":            route.LimitConnPerIP,
		"limit_rate":                   route.LimitRate,
		"cache_enabled":                route.CacheEnabled,
		"cache_policy":                 route.CachePolicy,
		"cache_rules":                  route.CacheRules,
		"custom_headers":               route.CustomHeaders,
		"pow_enabled":                  route.PoWEnabled,
		"pow_config":                   route.PoWConfig,
		"waf_enabled":                  route.WAFEnabled,
		"waf_mode":                     route.WAFMode,
		"waf_config":                   route.WAFConfig,
		"basic_auth_enabled":           route.BasicAuthEnabled,
		"basic_auth_username":          route.BasicAuthUsername,
		"basic_auth_password":          route.BasicAuthPassword,
		"region_restriction_enabled":   route.RegionRestrictionEnabled,
		"region_restriction_mode":      route.RegionRestrictionMode,
		"region_restriction_countries": route.RegionRestrictionCountries,
		"dns_auto_sync":                route.DNSAutoSync,
		"dns_account_id":               route.DNSAccountID,
		"dns_zone_id":                  route.DNSZoneID,
		"dns_record_type":              route.DNSRecordType,
		"dns_record_name":              route.DNSRecordName,
		"dns_record_content":           route.DNSRecordContent,
		"dns_auto_target":              route.DNSAutoTarget,
		"dns_target_count":             route.DNSTargetCount,
		"dns_schedule_mode":            route.DNSScheduleMode,
		"dns_record_ids":               route.DNSRecordIDs,
		"cloudflare_proxied":           route.CloudflareProxied,
		"ddos_protection_mode":         route.DDOSProtectionMode,
		"dns_last_sync_status":         route.DNSLastSyncStatus,
		"dns_last_sync_message":        route.DNSLastSyncMessage,
		"dns_last_synced_at":           route.DNSLastSyncedAt,
		"remark":                       route.Remark,
	}).Error
}

func (route *ProxyRoute) Delete() error {
	return DB.Delete(route).Error
}
