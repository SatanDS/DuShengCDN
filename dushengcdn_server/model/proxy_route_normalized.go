package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

const proxyRouteBasicAuthHashMaterial = "dushengcdn basic auth v1\n"

type ProxySite struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProxyRouteID uint      `json:"proxy_route_id" gorm:"uniqueIndex;not null"`
	Name         string    `json:"name" gorm:"size:255;not null;default:''"`
	NodePool     string    `json:"node_pool" gorm:"size:64;not null;default:'default'"`
	Enabled      bool      `json:"enabled" gorm:"not null;default:true"`
	Remark       string    `json:"remark" gorm:"size:255;not null;default:''"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ProxySite) TableName() string {
	return "proxy_sites"
}

type ProxySiteDomain struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProxySiteID  uint      `json:"proxy_site_id" gorm:"not null;index;uniqueIndex:idx_proxy_site_domains_site_domain;index:idx_proxy_site_domains_site_order,priority:1"`
	ProxyRouteID uint      `json:"proxy_route_id" gorm:"not null;index"`
	Domain       string    `json:"domain" gorm:"size:255;not null;index;uniqueIndex:idx_proxy_site_domains_site_domain"`
	IsPrimary    bool      `json:"is_primary" gorm:"not null;default:false"`
	SortOrder    int       `json:"sort_order" gorm:"not null;default:0;index:idx_proxy_site_domains_site_order,priority:2"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ProxySiteDomain) TableName() string {
	return "proxy_site_domains"
}

type OriginGroup struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProxyRouteID uint      `json:"proxy_route_id" gorm:"index;not null"`
	OriginID     *uint     `json:"origin_id" gorm:"index"`
	Name         string    `json:"name" gorm:"size:255;not null;default:''"`
	IsDefault    bool      `json:"is_default" gorm:"not null;default:false;index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (OriginGroup) TableName() string {
	return "origin_groups"
}

type OriginServer struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	OriginGroupID uint      `json:"origin_group_id" gorm:"not null;index;index:idx_origin_servers_group_order,priority:1"`
	ProxyRouteID  uint      `json:"proxy_route_id" gorm:"not null;index"`
	OriginID      *uint     `json:"origin_id" gorm:"index"`
	URL           string    `json:"url" gorm:"size:2048;not null;default:''"`
	Scheme        string    `json:"scheme" gorm:"size:16;not null;default:''"`
	Host          string    `json:"host" gorm:"size:255;not null;default:''"`
	Port          string    `json:"port" gorm:"size:16;not null;default:''"`
	URI           string    `json:"uri" gorm:"type:text;not null;default:''"`
	SortOrder     int       `json:"sort_order" gorm:"not null;default:0;index:idx_origin_servers_group_order,priority:2"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (OriginServer) TableName() string {
	return "origin_servers"
}

type ProxyRouteRule struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ProxyRouteID       uint      `json:"proxy_route_id" gorm:"not null;index;index:idx_proxy_route_rules_route_priority,priority:1"`
	ProxySiteID        uint      `json:"proxy_site_id" gorm:"not null;index"`
	OriginGroupID      uint      `json:"origin_group_id" gorm:"not null;index"`
	CachePolicyID      *uint     `json:"cache_policy_id" gorm:"index"`
	SecurityPolicyID   *uint     `json:"security_policy_id" gorm:"index"`
	Name               string    `json:"name" gorm:"size:255;not null;default:''"`
	MatchType          string    `json:"match_type" gorm:"size:16;not null;default:'default';index:idx_proxy_route_rules_route_match_path,priority:2"`
	Path               string    `json:"path" gorm:"size:512;not null;default:'/';index:idx_proxy_route_rules_route_match_path,priority:3"`
	Priority           int       `json:"priority" gorm:"not null;default:1000000;index:idx_proxy_route_rules_route_priority,priority:2"`
	Enabled            bool      `json:"enabled" gorm:"not null;default:true"`
	OriginHostHeader   string    `json:"origin_host_header" gorm:"size:255;not null;default:''"`
	OriginSNI          string    `json:"origin_sni" gorm:"size:255;not null;default:''"`
	OriginTLSVerify    bool      `json:"origin_tls_verify" gorm:"not null;default:true"`
	OriginCABundle     string    `json:"origin_ca_bundle" gorm:"type:text;not null;default:''"`
	OriginResolveMode  string    `json:"origin_resolve_mode" gorm:"size:32;not null;default:'publish_resolve'"`
	LimitConnPerServer int       `json:"limit_conn_per_server" gorm:"not null;default:0"`
	LimitConnPerIP     int       `json:"limit_conn_per_ip" gorm:"not null;default:0"`
	LimitRate          string    `json:"limit_rate" gorm:"size:32;not null;default:''"`
	ProxyBufferingMode string    `json:"proxy_buffering_mode" gorm:"size:16;not null;default:'default'"`
	CustomHeadersJSON  string    `json:"custom_headers_json" gorm:"column:custom_headers_json;type:text;not null;default:'[]'"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (ProxyRouteRule) TableName() string {
	return "proxy_route_rules"
}

type CachePolicy struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProxyRouteID uint      `json:"proxy_route_id" gorm:"index;not null"`
	Name         string    `json:"name" gorm:"size:255;not null;default:''"`
	IsDefault    bool      `json:"is_default" gorm:"not null;default:false;index"`
	Enabled      bool      `json:"enabled" gorm:"not null;default:false"`
	Policy       string    `json:"policy" gorm:"size:32;not null;default:''"`
	RulesJSON    string    `json:"rules_json" gorm:"column:rules_json;type:text;not null;default:'[]'"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (CachePolicy) TableName() string {
	return "cache_policies"
}

type TLSBinding struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProxyRouteID uint      `json:"proxy_route_id" gorm:"not null;index"`
	ProxySiteID  uint      `json:"proxy_site_id" gorm:"not null;index"`
	Domain       string    `json:"domain" gorm:"size:255;not null;index"`
	CertID       *uint     `json:"cert_id" gorm:"index"`
	EnableHTTPS  bool      `json:"enable_https" gorm:"column:enable_https;not null;default:false"`
	RedirectHTTP bool      `json:"redirect_http" gorm:"not null;default:false"`
	IsPrimary    bool      `json:"is_primary" gorm:"not null;default:false"`
	SortOrder    int       `json:"sort_order" gorm:"not null;default:0;index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (TLSBinding) TableName() string {
	return "tls_bindings"
}

type DNSBinding struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	ProxyRouteID      uint       `json:"proxy_route_id" gorm:"uniqueIndex;not null"`
	DNSAutoSync       bool       `json:"dns_auto_sync" gorm:"column:dns_auto_sync;not null;default:false"`
	DNSAccountID      *uint      `json:"dns_account_id" gorm:"column:dns_account_id;index"`
	DNSZoneID         string     `json:"dns_zone_id" gorm:"column:dns_zone_id;size:128;not null;default:''"`
	DNSRecordType     string     `json:"dns_record_type" gorm:"column:dns_record_type;size:16;not null;default:'A'"`
	DNSRecordName     string     `json:"dns_record_name" gorm:"column:dns_record_name;size:255;not null;default:''"`
	DNSRecordContent  string     `json:"dns_record_content" gorm:"column:dns_record_content;type:text;not null;default:''"`
	DNSAutoTarget     bool       `json:"dns_auto_target" gorm:"column:dns_auto_target;not null;default:false"`
	DNSTargetCount    int        `json:"dns_target_count" gorm:"column:dns_target_count;not null;default:1"`
	DNSScheduleMode   string     `json:"dns_schedule_mode" gorm:"column:dns_schedule_mode;size:32;not null;default:'healthy'"`
	DNSTTL            int        `json:"dns_ttl" gorm:"column:dns_ttl;not null;default:1"`
	DNSProviderMode   string     `json:"dns_provider_mode" gorm:"column:dns_provider_mode;size:32;not null;default:'cloudflare'"`
	DNSZoneIDRef      *uint      `json:"dns_zone_id_ref" gorm:"column:dns_zone_id_ref;index"`
	GSLBEnabled       bool       `json:"gslb_enabled" gorm:"column:gslb_enabled;not null;default:false"`
	GSLBPolicyJSON    string     `json:"gslb_policy_json" gorm:"column:gslb_policy_json;type:text;not null;default:'{}'"`
	DNSRecordIDsJSON  string     `json:"dns_record_ids_json" gorm:"column:dns_record_ids_json;type:text;not null;default:'{}'"`
	CloudflareProxied bool       `json:"cloudflare_proxied" gorm:"not null;default:false"`
	LastSyncStatus    string     `json:"last_sync_status" gorm:"size:16;not null;default:''"`
	LastSyncMessage   string     `json:"last_sync_message" gorm:"type:text"`
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (DNSBinding) TableName() string {
	return "dns_bindings"
}

type SecurityPolicy struct {
	ID                             uint       `json:"id" gorm:"primaryKey"`
	ProxyRouteID                   uint       `json:"proxy_route_id" gorm:"index;not null"`
	Name                           string     `json:"name" gorm:"size:255;not null;default:''"`
	IsDefault                      bool       `json:"is_default" gorm:"not null;default:false;index"`
	PoWEnabled                     bool       `json:"pow_enabled" gorm:"column:pow_enabled;not null;default:false"`
	PoWConfig                      string     `json:"pow_config" gorm:"column:pow_config;type:text;not null;default:'{}'"`
	WAFEnabled                     bool       `json:"waf_enabled" gorm:"column:waf_enabled;not null;default:false"`
	WAFMode                        string     `json:"waf_mode" gorm:"column:waf_mode;size:16;not null;default:'block'"`
	WAFConfig                      string     `json:"waf_config" gorm:"column:waf_config;type:text;not null;default:'{}'"`
	CCEnabled                      bool       `json:"cc_enabled" gorm:"column:cc_enabled;not null;default:false"`
	CCMode                         string     `json:"cc_mode" gorm:"column:cc_mode;size:16;not null;default:'block'"`
	CCConfig                       string     `json:"cc_config" gorm:"column:cc_config;type:text;not null;default:'{}'"`
	BasicAuthEnabled               bool       `json:"basic_auth_enabled" gorm:"not null;default:false"`
	BasicAuthUsername              string     `json:"basic_auth_username" gorm:"size:255;not null;default:''"`
	BasicAuthPasswordHash          string     `json:"basic_auth_password_hash" gorm:"size:64;not null;default:''"`
	BasicAuthPasswordUpdatedAt     *time.Time `json:"basic_auth_password_updated_at"`
	RegionRestrictionEnabled       bool       `json:"region_restriction_enabled" gorm:"not null;default:false"`
	RegionRestrictionMode          string     `json:"region_restriction_mode" gorm:"size:16;not null;default:'block'"`
	RegionRestrictionCountriesJSON string     `json:"region_restriction_countries_json" gorm:"column:region_restriction_countries_json;type:text;not null;default:'[]'"`
	DDOSProtectionMode             string     `json:"ddos_protection_mode" gorm:"column:ddos_protection_mode;size:16;not null;default:'off'"`
	DDOSProtectionProvider         string     `json:"ddos_protection_provider" gorm:"column:ddos_protection_provider;size:32;not null;default:'cloudflare'"`
	DDOSProtectionTarget           string     `json:"ddos_protection_target" gorm:"column:ddos_protection_target;size:128;not null;default:''"`
	CreatedAt                      time.Time  `json:"created_at"`
	UpdatedAt                      time.Time  `json:"updated_at"`
}

func (SecurityPolicy) TableName() string {
	return "security_policies"
}

func GetProxySiteByRouteID(routeID uint) (*ProxySite, error) {
	return GetProxySiteByRouteIDWithDB(DB, routeID)
}

func GetProxySiteByRouteIDWithDB(db *gorm.DB, routeID uint) (*ProxySite, error) {
	if db == nil {
		db = DB
	}
	site := &ProxySite{}
	err := db.Where("proxy_route_id = ?", routeID).First(site).Error
	return site, err
}

func GetProxySiteDomainByDomain(domain string) (*ProxySiteDomain, error) {
	return GetProxySiteDomainByDomainWithDB(DB, domain)
}

func GetProxySiteDomainByDomainWithDB(db *gorm.DB, domain string) (*ProxySiteDomain, error) {
	if db == nil {
		db = DB
	}
	domain = normalizeProxyRouteDomainForMigration(domain)
	if domain == "" || !db.Migrator().HasTable(&ProxySiteDomain{}) {
		return nil, gorm.ErrRecordNotFound
	}
	item := &ProxySiteDomain{}
	err := db.Where("domain = ?", domain).Order("proxy_route_id asc").First(item).Error
	return item, err
}

func ListProxySiteDomainsByRouteID(routeID uint) ([]ProxySiteDomain, error) {
	var domains []ProxySiteDomain
	if DB == nil || !DB.Migrator().HasTable(&ProxySiteDomain{}) {
		return domains, nil
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("sort_order asc").Find(&domains).Error
	return domains, err
}

func ListOriginServersByRouteID(routeID uint) ([]OriginServer, error) {
	var servers []OriginServer
	if DB == nil || !DB.Migrator().HasTable(&OriginServer{}) {
		return servers, nil
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("sort_order asc").Find(&servers).Error
	return servers, err
}

func ListOriginServersByGroupIDs(groupIDs []uint) ([]OriginServer, error) {
	var servers []OriginServer
	if DB == nil || !DB.Migrator().HasTable(&OriginServer{}) || len(groupIDs) == 0 {
		return servers, nil
	}
	err := DB.Where("origin_group_id IN ?", groupIDs).Order("origin_group_id asc").Order("sort_order asc").Find(&servers).Error
	return servers, err
}

func ListProxyRouteRulesByRouteID(routeID uint) ([]ProxyRouteRule, error) {
	var rules []ProxyRouteRule
	if DB == nil || !DB.Migrator().HasTable(&ProxyRouteRule{}) {
		return rules, nil
	}
	err := DB.Where("proxy_route_id = ?", routeID).
		Order("priority asc").
		Order("id asc").
		Find(&rules).Error
	return rules, err
}

func ListProxyRouteRulesByRouteIDs(routeIDs []uint) ([]ProxyRouteRule, error) {
	var rules []ProxyRouteRule
	if DB == nil || !DB.Migrator().HasTable(&ProxyRouteRule{}) || len(routeIDs) == 0 {
		return rules, nil
	}
	err := DB.Where("proxy_route_id IN ?", routeIDs).
		Order("proxy_route_id asc").
		Order("priority asc").
		Order("id asc").
		Find(&rules).Error
	return rules, err
}

func GetCachePolicyByRouteID(routeID uint) (*CachePolicy, error) {
	policy := &CachePolicy{}
	if DB == nil || !DB.Migrator().HasTable(policy) {
		return nil, gorm.ErrRecordNotFound
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("is_default desc").Order("id asc").First(policy).Error
	return policy, err
}

func ListCachePoliciesByRouteID(routeID uint) ([]CachePolicy, error) {
	var policies []CachePolicy
	if DB == nil || !DB.Migrator().HasTable(&CachePolicy{}) {
		return policies, nil
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("is_default desc").Order("id asc").Find(&policies).Error
	return policies, err
}

func GetSecurityPolicyByRouteID(routeID uint) (*SecurityPolicy, error) {
	policy := &SecurityPolicy{}
	if DB == nil || !DB.Migrator().HasTable(policy) {
		return nil, gorm.ErrRecordNotFound
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("is_default desc").Order("id asc").First(policy).Error
	return policy, err
}

func ListSecurityPoliciesByRouteID(routeID uint) ([]SecurityPolicy, error) {
	var policies []SecurityPolicy
	if DB == nil || !DB.Migrator().HasTable(&SecurityPolicy{}) {
		return policies, nil
	}
	err := DB.Where("proxy_route_id = ?", routeID).Order("is_default desc").Order("id asc").Find(&policies).Error
	return policies, err
}

func SyncProxyRouteNormalizedTables(route *ProxyRoute) error {
	if DB == nil {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return SyncProxyRouteNormalizedTablesWithDB(tx, route)
	})
}

func SyncProxyRouteNormalizedTablesWithDB(db *gorm.DB, route *ProxyRoute) error {
	if db == nil {
		db = DB
	}
	if db == nil || route == nil || route.ID == 0 {
		return nil
	}
	if !proxyRouteNormalizedTablesAvailable(db) {
		return nil
	}

	domains, err := decodeProxyRouteDomainsForMigration(route.Domains, route.Domain)
	if err != nil {
		fallbackDomain := normalizeProxyRouteDomainForMigration(route.Domain)
		if fallbackDomain == "" {
			return nil
		}
		domains = []string{fallbackDomain}
	}
	if err := ensureProxyRouteDomainSidecarConflicts(db, route.ID, domains); err != nil {
		return err
	}

	siteName := normalizeProxyRouteSiteNameForMigration(route.SiteName, domains[0])
	site, err := upsertProxySite(db, &ProxySite{
		ProxyRouteID: route.ID,
		Name:         siteName,
		NodePool:     normalizeMigrationPoolName(route.NodePool),
		Enabled:      route.Enabled,
		Remark:       strings.TrimSpace(route.Remark),
	})
	if err != nil {
		return err
	}
	if err := replaceProxySiteDomains(db, site.ID, route.ID, domains); err != nil {
		return err
	}

	originGroup, err := upsertOriginGroup(db, &OriginGroup{
		ProxyRouteID: route.ID,
		OriginID:     route.OriginID,
		Name:         siteName,
		IsDefault:    true,
	})
	if err != nil {
		return err
	}
	if err := replaceOriginServers(db, originGroup.ID, route, route.OriginID); err != nil {
		return err
	}

	if err := upsertProxyRouteRule(db, &ProxyRouteRule{
		ProxyRouteID:       route.ID,
		ProxySiteID:        site.ID,
		OriginGroupID:      originGroup.ID,
		CachePolicyID:      nil,
		SecurityPolicyID:   nil,
		Name:               "default",
		MatchType:          "default",
		Path:               "/",
		Priority:           1000000,
		Enabled:            true,
		OriginHostHeader:   strings.TrimSpace(route.OriginHostHeader),
		OriginSNI:          strings.TrimSpace(route.OriginSNI),
		OriginTLSVerify:    route.OriginTLSVerify,
		OriginCABundle:     strings.TrimSpace(route.OriginCABundle),
		OriginResolveMode:  strings.TrimSpace(route.OriginResolveMode),
		LimitConnPerServer: route.LimitConnPerServer,
		LimitConnPerIP:     route.LimitConnPerIP,
		LimitRate:          strings.TrimSpace(route.LimitRate),
		ProxyBufferingMode: strings.TrimSpace(route.ProxyBufferingMode),
		CustomHeadersJSON:  normalizedJSONString(route.CustomHeaders, "[]"),
	}); err != nil {
		return err
	}
	if err := upsertCachePolicy(db, &CachePolicy{
		ProxyRouteID: route.ID,
		Name:         "default",
		IsDefault:    true,
		Enabled:      route.CacheEnabled,
		Policy:       strings.TrimSpace(route.CachePolicy),
		RulesJSON:    normalizedJSONString(route.CacheRules, "[]"),
	}); err != nil {
		return err
	}
	if err := replaceTLSBindings(db, site.ID, route, domains); err != nil {
		return err
	}
	if err := upsertDNSBinding(db, &DNSBinding{
		ProxyRouteID:      route.ID,
		DNSAutoSync:       route.DNSAutoSync,
		DNSAccountID:      route.DNSAccountID,
		DNSZoneID:         strings.TrimSpace(route.DNSZoneID),
		DNSRecordType:     strings.TrimSpace(route.DNSRecordType),
		DNSRecordName:     strings.TrimSpace(route.DNSRecordName),
		DNSRecordContent:  strings.TrimSpace(route.DNSRecordContent),
		DNSAutoTarget:     route.DNSAutoTarget,
		DNSTargetCount:    route.DNSTargetCount,
		DNSScheduleMode:   strings.TrimSpace(route.DNSScheduleMode),
		DNSTTL:            route.DNSTTL,
		DNSProviderMode:   strings.TrimSpace(route.DNSProviderMode),
		DNSZoneIDRef:      route.DNSZoneIDRef,
		GSLBEnabled:       route.GSLBEnabled,
		GSLBPolicyJSON:    normalizedJSONString(route.GSLBPolicy, "{}"),
		DNSRecordIDsJSON:  normalizedJSONString(route.DNSRecordIDs, "{}"),
		CloudflareProxied: route.CloudflareProxied,
		LastSyncStatus:    strings.TrimSpace(route.DNSLastSyncStatus),
		LastSyncMessage:   strings.TrimSpace(route.DNSLastSyncMessage),
		LastSyncedAt:      route.DNSLastSyncedAt,
	}); err != nil {
		return err
	}
	basicAuthPasswordHash := proxyRouteBasicAuthPasswordHash(route)
	basicAuthPasswordUpdatedAt := route.BasicAuthPasswordUpdatedAt
	if basicAuthPasswordHash == "" {
		basicAuthPasswordUpdatedAt = nil
	}
	return upsertSecurityPolicy(db, &SecurityPolicy{
		ProxyRouteID:                   route.ID,
		Name:                           "default",
		IsDefault:                      true,
		PoWEnabled:                     route.PoWEnabled,
		PoWConfig:                      normalizedJSONString(route.PoWConfig, "{}"),
		WAFEnabled:                     route.WAFEnabled,
		WAFMode:                        strings.TrimSpace(route.WAFMode),
		WAFConfig:                      normalizedJSONString(route.WAFConfig, "{}"),
		CCEnabled:                      route.CCEnabled,
		CCMode:                         strings.TrimSpace(route.CCMode),
		CCConfig:                       normalizedJSONString(route.CCConfig, "{}"),
		BasicAuthEnabled:               route.BasicAuthEnabled,
		BasicAuthUsername:              strings.TrimSpace(route.BasicAuthUsername),
		BasicAuthPasswordHash:          basicAuthPasswordHash,
		BasicAuthPasswordUpdatedAt:     basicAuthPasswordUpdatedAt,
		RegionRestrictionEnabled:       route.RegionRestrictionEnabled,
		RegionRestrictionMode:          strings.TrimSpace(route.RegionRestrictionMode),
		RegionRestrictionCountriesJSON: normalizedJSONString(route.RegionRestrictionCountries, "[]"),
		DDOSProtectionMode:             strings.TrimSpace(route.DDOSProtectionMode),
		DDOSProtectionProvider:         strings.TrimSpace(route.DDOSProtectionProvider),
		DDOSProtectionTarget:           strings.TrimSpace(route.DDOSProtectionTarget),
	})
}

func DeleteProxyRouteNormalizedTables(routeID uint) error {
	if DB == nil {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return DeleteProxyRouteNormalizedTablesWithDB(tx, routeID)
	})
}

func DeleteProxyRouteNormalizedTablesWithDB(db *gorm.DB, routeID uint) error {
	if db == nil {
		db = DB
	}
	if db == nil || routeID == 0 || !proxyRouteNormalizedTablesAvailable(db) {
		return nil
	}
	deleteModels := []any{
		&ProxySiteDomain{},
		&TLSBinding{},
		&OriginServer{},
		&CachePolicy{},
		&SecurityPolicy{},
		&DNSBinding{},
		&ProxyRouteRule{},
		&OriginGroup{},
		&ProxySite{},
	}
	for _, modelValue := range deleteModels {
		if err := db.Where("proxy_route_id = ?", routeID).Delete(modelValue).Error; err != nil {
			return err
		}
	}
	return nil
}

func BackfillProxyRouteNormalizedTables(db *gorm.DB) error {
	db = migrationSession(db)
	if db == nil || !db.Migrator().HasTable(&ProxyRoute{}) || !proxyRouteNormalizedTablesAvailable(db) {
		return nil
	}
	var routes []ProxyRoute
	if err := db.Order("id asc").Find(&routes).Error; err != nil {
		return fmt.Errorf("list proxy routes for normalized table backfill failed: %w", err)
	}
	if err := clearProxyRouteNormalizedTables(db); err != nil {
		return err
	}
	for index := range routes {
		if err := SyncProxyRouteNormalizedTablesWithDB(db, &routes[index]); err != nil {
			return fmt.Errorf("sync proxy route %d normalized tables failed: %w", routes[index].ID, err)
		}
	}
	return nil
}

func clearProxyRouteNormalizedTables(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	db = db.Session(&gorm.Session{AllowGlobalUpdate: true})
	for _, modelValue := range []any{
		&ProxySiteDomain{},
		&TLSBinding{},
		&OriginServer{},
		&CachePolicy{},
		&SecurityPolicy{},
		&DNSBinding{},
		&ProxyRouteRule{},
		&OriginGroup{},
		&ProxySite{},
	} {
		if err := db.Delete(modelValue).Error; err != nil {
			return err
		}
	}
	return nil
}

func EnsureProxyRouteNormalizedTablesBackfilled(db *gorm.DB) error {
	db = migrationSession(db)
	if db == nil || !db.Migrator().HasTable(&ProxyRoute{}) || !proxyRouteNormalizedTablesAvailable(db) {
		return nil
	}
	var routeCount int64
	if err := db.Model(&ProxyRoute{}).Count(&routeCount).Error; err != nil {
		return err
	}
	if routeCount == 0 {
		return nil
	}
	var siteCount int64
	if err := db.Model(&ProxySite{}).Count(&siteCount).Error; err != nil {
		return err
	}
	if siteCount == routeCount {
		return nil
	}
	return BackfillProxyRouteNormalizedTables(db)
}

func proxyRouteNormalizedTablesAvailable(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	for _, modelValue := range []any{
		&ProxySite{},
		&ProxySiteDomain{},
		&OriginGroup{},
		&OriginServer{},
		&ProxyRouteRule{},
		&CachePolicy{},
		&TLSBinding{},
		&DNSBinding{},
		&SecurityPolicy{},
	} {
		if !db.Migrator().HasTable(modelValue) {
			return false
		}
	}
	return true
}

func ensureProxyRouteDomainSidecarConflicts(db *gorm.DB, routeID uint, domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	var rows []ProxySiteDomain
	if err := db.Where("domain IN ? AND proxy_route_id <> ?", domains, routeID).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return fmt.Errorf("proxy route domain %s already belongs to route %d", rows[0].Domain, rows[0].ProxyRouteID)
}

func upsertProxySite(db *gorm.DB, next *ProxySite) (*ProxySite, error) {
	var current ProxySite
	err := db.Where("proxy_route_id = ?", next.ProxyRouteID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(next).Error; err != nil {
			return nil, err
		}
		return next, nil
	}
	if err != nil {
		return nil, err
	}
	current.Name = next.Name
	current.NodePool = next.NodePool
	current.Enabled = next.Enabled
	current.Remark = next.Remark
	if err := db.Save(&current).Error; err != nil {
		return nil, err
	}
	return &current, nil
}

func upsertOriginGroup(db *gorm.DB, next *OriginGroup) (*OriginGroup, error) {
	var current OriginGroup
	query := db.Where("proxy_route_id = ?", next.ProxyRouteID)
	if next.IsDefault {
		query = query.Where("is_default = ?", true)
	} else if strings.TrimSpace(next.Name) != "" {
		query = query.Where("name = ?", next.Name)
	}
	err := query.First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(next).Error; err != nil {
			return nil, err
		}
		return next, nil
	}
	if err != nil {
		return nil, err
	}
	current.OriginID = next.OriginID
	current.Name = next.Name
	current.IsDefault = next.IsDefault
	if err := db.Save(&current).Error; err != nil {
		return nil, err
	}
	return &current, nil
}

func upsertProxyRouteRule(db *gorm.DB, next *ProxyRouteRule) error {
	var current ProxyRouteRule
	err := db.Where("proxy_route_id = ? AND match_type = ? AND path = ?", next.ProxyRouteID, next.MatchType, next.Path).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(next).Error
	}
	if err != nil {
		return err
	}
	current.ProxySiteID = next.ProxySiteID
	current.OriginGroupID = next.OriginGroupID
	current.CachePolicyID = next.CachePolicyID
	current.SecurityPolicyID = next.SecurityPolicyID
	current.Name = next.Name
	current.MatchType = next.MatchType
	current.Path = next.Path
	current.Priority = next.Priority
	current.Enabled = next.Enabled
	current.OriginHostHeader = next.OriginHostHeader
	current.OriginSNI = next.OriginSNI
	current.OriginTLSVerify = next.OriginTLSVerify
	current.OriginCABundle = next.OriginCABundle
	current.OriginResolveMode = next.OriginResolveMode
	current.LimitConnPerServer = next.LimitConnPerServer
	current.LimitConnPerIP = next.LimitConnPerIP
	current.LimitRate = next.LimitRate
	current.ProxyBufferingMode = next.ProxyBufferingMode
	current.CustomHeadersJSON = next.CustomHeadersJSON
	return db.Save(&current).Error
}

func upsertCachePolicy(db *gorm.DB, next *CachePolicy) error {
	var current CachePolicy
	query := db.Where("proxy_route_id = ?", next.ProxyRouteID)
	if next.IsDefault {
		query = query.Where("is_default = ?", true)
	} else if strings.TrimSpace(next.Name) != "" {
		query = query.Where("name = ?", next.Name)
	}
	err := query.First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(next).Error
	}
	if err != nil {
		return err
	}
	current.Name = next.Name
	current.IsDefault = next.IsDefault
	current.Enabled = next.Enabled
	current.Policy = next.Policy
	current.RulesJSON = next.RulesJSON
	return db.Save(&current).Error
}

func upsertDNSBinding(db *gorm.DB, next *DNSBinding) error {
	var current DNSBinding
	err := db.Where("proxy_route_id = ?", next.ProxyRouteID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(next).Error
	}
	if err != nil {
		return err
	}
	current.DNSAutoSync = next.DNSAutoSync
	current.DNSAccountID = next.DNSAccountID
	current.DNSZoneID = next.DNSZoneID
	current.DNSRecordType = next.DNSRecordType
	current.DNSRecordName = next.DNSRecordName
	current.DNSRecordContent = next.DNSRecordContent
	current.DNSAutoTarget = next.DNSAutoTarget
	current.DNSTargetCount = next.DNSTargetCount
	current.DNSScheduleMode = next.DNSScheduleMode
	current.DNSTTL = next.DNSTTL
	current.DNSProviderMode = next.DNSProviderMode
	current.DNSZoneIDRef = next.DNSZoneIDRef
	current.GSLBEnabled = next.GSLBEnabled
	current.GSLBPolicyJSON = next.GSLBPolicyJSON
	current.DNSRecordIDsJSON = next.DNSRecordIDsJSON
	current.CloudflareProxied = next.CloudflareProxied
	current.LastSyncStatus = next.LastSyncStatus
	current.LastSyncMessage = next.LastSyncMessage
	current.LastSyncedAt = next.LastSyncedAt
	return db.Save(&current).Error
}

func upsertSecurityPolicy(db *gorm.DB, next *SecurityPolicy) error {
	var current SecurityPolicy
	query := db.Where("proxy_route_id = ?", next.ProxyRouteID)
	if next.IsDefault {
		query = query.Where("is_default = ?", true)
	} else if strings.TrimSpace(next.Name) != "" {
		query = query.Where("name = ?", next.Name)
	}
	err := query.First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(next).Error
	}
	if err != nil {
		return err
	}
	var basicAuthPasswordUpdatedAt any = gorm.Expr("NULL")
	if next.BasicAuthPasswordUpdatedAt != nil {
		basicAuthPasswordUpdatedAt = next.BasicAuthPasswordUpdatedAt
	}
	return db.Model(&SecurityPolicy{}).Where("id = ?", current.ID).Updates(map[string]any{
		"name":                              next.Name,
		"is_default":                        next.IsDefault,
		"pow_enabled":                       next.PoWEnabled,
		"pow_config":                        next.PoWConfig,
		"waf_enabled":                       next.WAFEnabled,
		"waf_mode":                          next.WAFMode,
		"waf_config":                        next.WAFConfig,
		"cc_enabled":                        next.CCEnabled,
		"cc_mode":                           next.CCMode,
		"cc_config":                         next.CCConfig,
		"basic_auth_enabled":                next.BasicAuthEnabled,
		"basic_auth_username":               next.BasicAuthUsername,
		"basic_auth_password_hash":          next.BasicAuthPasswordHash,
		"basic_auth_password_updated_at":    basicAuthPasswordUpdatedAt,
		"region_restriction_enabled":        next.RegionRestrictionEnabled,
		"region_restriction_mode":           next.RegionRestrictionMode,
		"region_restriction_countries_json": next.RegionRestrictionCountriesJSON,
		"ddos_protection_mode":              next.DDOSProtectionMode,
		"ddos_protection_provider":          next.DDOSProtectionProvider,
		"ddos_protection_target":            next.DDOSProtectionTarget,
	}).Error
}

func replaceProxySiteDomains(db *gorm.DB, siteID uint, routeID uint, domains []string) error {
	if err := db.Where("proxy_route_id = ?", routeID).Delete(&ProxySiteDomain{}).Error; err != nil {
		return err
	}
	for index, domain := range domains {
		item := &ProxySiteDomain{
			ProxySiteID:  siteID,
			ProxyRouteID: routeID,
			Domain:       domain,
			IsPrimary:    index == 0,
			SortOrder:    index,
		}
		if err := db.Create(item).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceOriginServers(db *gorm.DB, groupID uint, route *ProxyRoute, originID *uint) error {
	if err := db.Where("origin_group_id = ?", groupID).Delete(&OriginServer{}).Error; err != nil {
		return err
	}
	upstreams, err := decodeProxyRouteUpstreamsForNormalized(route.Upstreams, route.OriginURL)
	if err != nil {
		return err
	}
	if len(upstreams) == 0 {
		return nil
	}
	for index, upstream := range upstreams {
		parsed, err := url.Parse(upstream)
		if err != nil {
			return fmt.Errorf("parse proxy route %d upstream failed: %w", route.ID, err)
		}
		uri := parsed.RequestURI()
		if uri == "" {
			uri = "/"
		}
		item := &OriginServer{
			OriginGroupID: groupID,
			ProxyRouteID:  route.ID,
			OriginID:      originID,
			URL:           upstream,
			Scheme:        parsed.Scheme,
			Host:          parsed.Hostname(),
			Port:          parsed.Port(),
			URI:           uri,
			SortOrder:     index,
		}
		if err := db.Create(item).Error; err != nil {
			return err
		}
	}
	return nil
}

func ReplaceOriginServersForProxyRouteRule(db *gorm.DB, groupID uint, route *ProxyRoute, originID *uint) error {
	if db == nil {
		db = DB
	}
	return replaceOriginServers(db, groupID, route, originID)
}

func replaceTLSBindings(db *gorm.DB, siteID uint, route *ProxyRoute, domains []string) error {
	if err := db.Where("proxy_route_id = ?", route.ID).Delete(&TLSBinding{}).Error; err != nil {
		return err
	}
	if !route.EnableHTTPS {
		return nil
	}
	certIDs, err := decodeProxyRouteCertIDsForMigration(route.CertIDs, route.CertID)
	if err != nil {
		certIDs = []uint{}
		if route.CertID != nil && *route.CertID != 0 {
			certIDs = append(certIDs, *route.CertID)
		}
	}
	domainCertIDs, err := decodeProxyRouteDomainCertIDsForMigration(route.DomainCertIDs, len(domains))
	if err != nil {
		domainCertIDs = []uint{}
	}
	for index, domain := range domains {
		var certID *uint
		if index < len(domainCertIDs) && domainCertIDs[index] != 0 {
			value := domainCertIDs[index]
			certID = &value
		} else if len(certIDs) == 1 && certIDs[0] != 0 {
			value := certIDs[0]
			certID = &value
		}
		item := &TLSBinding{
			ProxyRouteID: route.ID,
			ProxySiteID:  siteID,
			Domain:       domain,
			CertID:       certID,
			EnableHTTPS:  route.EnableHTTPS,
			RedirectHTTP: route.RedirectHTTP,
			IsPrimary:    index == 0,
			SortOrder:    index,
		}
		if err := db.Create(item).Error; err != nil {
			return err
		}
	}
	return nil
}

func decodeProxyRouteUpstreamsForNormalized(raw string, fallbackOriginURL string) ([]string, error) {
	values := make([]string, 0)
	text := strings.TrimSpace(raw)
	if text != "" {
		if err := json.Unmarshal([]byte(text), &values); err != nil {
			return nil, fmt.Errorf("decode proxy route upstreams failed: %w", err)
		}
	}
	if len(values) == 0 && strings.TrimSpace(fallbackOriginURL) != "" {
		values = append(values, fallbackOriginURL)
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
	return normalized, nil
}

func normalizedJSONString(raw string, fallback string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback
	}
	return text
}

func BasicAuthCredentialHash(username, password string) string {
	credentials := strings.TrimSpace(username) + ":" + strings.TrimSpace(password)
	if credentials == ":" {
		return ""
	}
	sum := sha256.Sum256([]byte(proxyRouteBasicAuthHashMaterial + credentials))
	return hex.EncodeToString(sum[:])
}

func proxyRouteBasicAuthPasswordHash(route *ProxyRoute) string {
	if route == nil || !route.BasicAuthEnabled {
		return ""
	}
	passwordHash := strings.TrimSpace(route.BasicAuthPasswordHash)
	if passwordHash != "" {
		return passwordHash
	}
	return BasicAuthCredentialHash(route.BasicAuthUsername, route.BasicAuthPassword)
}
