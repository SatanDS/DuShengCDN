package service

import (
	"regexp"
	"strings"
	"time"

	"dushengcdn/model"
)

var proxyHeaderKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var proxyRouteLimitRatePattern = regexp.MustCompile(`^\d+(?:[kKmM])?$`)
var proxyRouteDomainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var proxyRouteRegionCountryPattern = regexp.MustCompile(`^[A-Z0-9]{2}$`)

const (
	proxyRouteCachePolicyURL             = "url"
	proxyRouteCachePolicySuffix          = "suffix"
	proxyRouteCachePolicyPathPrefix      = "path_prefix"
	proxyRouteCachePolicyPathContains    = "path_contains"
	proxyRouteCachePolicyPathContainsAll = "path_contains_all"
	proxyRouteCachePolicyPathExact       = "path_exact"
	proxyRouteRegionModeAllow            = "allow"
	proxyRouteRegionModeBlock            = "block"
	proxyRouteWAFModeLog                 = "log"
	proxyRouteWAFModeBlock               = "block"
	proxyRouteCCModeLog                  = "log"
	proxyRouteCCModeBlock                = "block"
	proxyRouteCCModePoW                  = "pow"
	proxyRouteProxyBufferingModeDefault  = "default"
	proxyRouteProxyBufferingModeOff      = "off"
	proxyRouteOriginResolveRuntimeDNS    = "runtime_dns"
	proxyRouteOriginResolvePublish       = "publish_resolve"
	proxyRouteOriginResolveStaticIP      = "static_ip"
	proxyRouteOriginResolveOriginGroup   = "origin_group"
	proxyRouteRuleMatchDefault           = "default"
	proxyRouteRuleMatchPrefix            = "prefix"
	proxyRouteRuleMatchExact             = "exact"
	proxyRouteRuleMatchRegex             = "regex"
	redactedProxyRouteCustomHeaderValue  = "[redacted sensitive header; preserved on save]"
	DNSProviderModeCloudflare            = "cloudflare"
	DNSProviderModeAuthoritative         = "authoritative"
)

type ProxyRouteCustomHeaderInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ProxyRouteRuleInput struct {
	ID                 uint                          `json:"id"`
	Name               string                        `json:"name"`
	MatchType          string                        `json:"match_type"`
	Path               string                        `json:"path"`
	Priority           int                           `json:"priority"`
	Enabled            *bool                         `json:"enabled"`
	OriginURL          string                        `json:"origin_url"`
	Upstreams          []string                      `json:"upstreams"`
	OriginHostHeader   string                        `json:"origin_host_header"`
	OriginSNI          string                        `json:"origin_sni"`
	OriginTLSVerify    *bool                         `json:"origin_tls_verify"`
	OriginCABundle     string                        `json:"origin_ca_bundle"`
	OriginResolveMode  string                        `json:"origin_resolve_mode"`
	LimitConnPerServer int                           `json:"limit_conn_per_server"`
	LimitConnPerIP     int                           `json:"limit_conn_per_ip"`
	LimitRate          string                        `json:"limit_rate"`
	ProxyBufferingMode string                        `json:"proxy_buffering_mode"`
	CacheEnabled       *bool                         `json:"cache_enabled"`
	CachePolicy        string                        `json:"cache_policy"`
	CacheRules         []string                      `json:"cache_rules"`
	CustomHeaders      []ProxyRouteCustomHeaderInput `json:"custom_headers"`
	BasicAuthEnabled   *bool                         `json:"basic_auth_enabled"`
	BasicAuthUsername  string                        `json:"basic_auth_username"`
	BasicAuthPassword  string                        `json:"basic_auth_password"`
}

type ProxyRouteRuleView struct {
	ID                          uint                          `json:"id"`
	Name                        string                        `json:"name"`
	MatchType                   string                        `json:"match_type"`
	Path                        string                        `json:"path"`
	Priority                    int                           `json:"priority"`
	Enabled                     bool                          `json:"enabled"`
	OriginGroupID               uint                          `json:"origin_group_id"`
	OriginURL                   string                        `json:"origin_url"`
	Upstreams                   []string                      `json:"upstreams"`
	OriginHostHeader            string                        `json:"origin_host_header"`
	OriginSNI                   string                        `json:"origin_sni"`
	OriginTLSVerify             bool                          `json:"origin_tls_verify"`
	OriginCABundle              string                        `json:"origin_ca_bundle"`
	OriginResolveMode           string                        `json:"origin_resolve_mode"`
	LimitConnPerServer          int                           `json:"limit_conn_per_server"`
	LimitConnPerIP              int                           `json:"limit_conn_per_ip"`
	LimitRate                   string                        `json:"limit_rate"`
	ProxyBufferingMode          string                        `json:"proxy_buffering_mode"`
	CustomHeaders               []ProxyRouteCustomHeaderInput `json:"custom_headers"`
	CachePolicyID               *uint                         `json:"cache_policy_id,omitempty"`
	CacheEnabled                bool                          `json:"cache_enabled"`
	CachePolicy                 string                        `json:"cache_policy"`
	CacheRules                  []string                      `json:"cache_rules"`
	SecurityPolicyID            *uint                         `json:"security_policy_id,omitempty"`
	BasicAuthEnabled            bool                          `json:"basic_auth_enabled"`
	BasicAuthUsername           string                        `json:"basic_auth_username"`
	BasicAuthPasswordConfigured bool                          `json:"basic_auth_password_configured"`
	CreatedAt                   time.Time                     `json:"created_at"`
	UpdatedAt                   time.Time                     `json:"updated_at"`
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
	OriginHostHeader           string                        `json:"origin_host_header"`
	OriginSNI                  string                        `json:"origin_sni"`
	OriginTLSVerify            *bool                         `json:"origin_tls_verify"`
	OriginCABundle             string                        `json:"origin_ca_bundle"`
	OriginResolveMode          string                        `json:"origin_resolve_mode"`
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
	ProxyBufferingMode         string                        `json:"proxy_buffering_mode"`
	CacheEnabled               bool                          `json:"cache_enabled"`
	CachePolicy                string                        `json:"cache_policy"`
	CacheRules                 []string                      `json:"cache_rules"`
	CustomHeaders              []ProxyRouteCustomHeaderInput `json:"custom_headers"`
	RouteRules                 []ProxyRouteRuleInput         `json:"route_rules"`
	PoWEnabled                 bool                          `json:"pow_enabled"`
	PoWConfig                  string                        `json:"pow_config"`
	WAFEnabled                 bool                          `json:"waf_enabled"`
	WAFMode                    string                        `json:"waf_mode"`
	WAFConfig                  string                        `json:"waf_config"`
	CCEnabled                  bool                          `json:"cc_enabled"`
	CCMode                     string                        `json:"cc_mode"`
	CCConfig                   string                        `json:"cc_config"`
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
	ID                          uint                          `json:"id"`
	SiteName                    string                        `json:"site_name"`
	Domain                      string                        `json:"domain"`
	Domains                     []string                      `json:"domains"`
	PrimaryDomain               string                        `json:"primary_domain"`
	DomainCount                 int                           `json:"domain_count"`
	OriginID                    *uint                         `json:"origin_id"`
	OriginURL                   string                        `json:"origin_url"`
	OriginHost                  string                        `json:"origin_host"`
	OriginHostHeader            string                        `json:"origin_host_header"`
	OriginSNI                   string                        `json:"origin_sni"`
	OriginTLSVerify             bool                          `json:"origin_tls_verify"`
	OriginCABundle              string                        `json:"origin_ca_bundle"`
	OriginResolveMode           string                        `json:"origin_resolve_mode"`
	Upstreams                   string                        `json:"upstreams"`
	UpstreamList                []string                      `json:"upstream_list"`
	NodePool                    string                        `json:"node_pool"`
	Enabled                     bool                          `json:"enabled"`
	EnableHTTPS                 bool                          `json:"enable_https"`
	CertID                      *uint                         `json:"cert_id"`
	CertIDs                     []uint                        `json:"cert_ids"`
	DomainCertIDs               []uint                        `json:"domain_cert_ids"`
	RedirectHTTP                bool                          `json:"redirect_http"`
	LimitConnPerServer          int                           `json:"limit_conn_per_server"`
	LimitConnPerIP              int                           `json:"limit_conn_per_ip"`
	LimitRate                   string                        `json:"limit_rate"`
	ProxyBufferingMode          string                        `json:"proxy_buffering_mode"`
	CacheEnabled                bool                          `json:"cache_enabled"`
	CachePolicy                 string                        `json:"cache_policy"`
	CacheRules                  string                        `json:"cache_rules"`
	CacheRuleList               []string                      `json:"cache_rule_list"`
	CustomHeaders               string                        `json:"custom_headers"`
	CustomHeaderList            []ProxyRouteCustomHeaderInput `json:"custom_header_list"`
	RouteRules                  []ProxyRouteRuleView          `json:"route_rules"`
	PoWEnabled                  bool                          `json:"pow_enabled"`
	PoWConfig                   *ProxyRoutePoWConfig          `json:"pow_config"`
	WAFEnabled                  bool                          `json:"waf_enabled"`
	WAFMode                     string                        `json:"waf_mode"`
	WAFConfig                   *ProxyRouteWAFConfig          `json:"waf_config"`
	CCEnabled                   bool                          `json:"cc_enabled"`
	CCMode                      string                        `json:"cc_mode"`
	CCConfig                    *ProxyRouteCCConfig           `json:"cc_config"`
	BasicAuthEnabled            bool                          `json:"basic_auth_enabled"`
	BasicAuthUsername           string                        `json:"basic_auth_username"`
	BasicAuthPassword           string                        `json:"-"`
	BasicAuthPasswordConfigured bool                          `json:"basic_auth_password_configured"`
	BasicAuthPasswordUpdatedAt  *time.Time                    `json:"basic_auth_password_updated_at"`
	RegionRestrictionEnabled    bool                          `json:"region_restriction_enabled"`
	RegionRestrictionMode       string                        `json:"region_restriction_mode"`
	RegionRestrictionCountries  []string                      `json:"region_restriction_countries"`
	DNSAutoSync                 bool                          `json:"dns_auto_sync"`
	DNSAccountID                *uint                         `json:"dns_account_id"`
	DNSZoneID                   string                        `json:"dns_zone_id"`
	DNSRecordType               string                        `json:"dns_record_type"`
	DNSRecordName               string                        `json:"dns_record_name"`
	DNSRecordContent            string                        `json:"dns_record_content"`
	DNSAutoTarget               bool                          `json:"dns_auto_target"`
	DNSTargetCount              int                           `json:"dns_target_count"`
	DNSScheduleMode             string                        `json:"dns_schedule_mode"`
	DNSTTL                      int                           `json:"dns_ttl"`
	DNSProviderMode             string                        `json:"dns_provider_mode"`
	DNSZoneIDRef                *uint                         `json:"dns_zone_id_ref"`
	GSLBEnabled                 bool                          `json:"gslb_enabled"`
	GSLBPolicy                  ProxyRouteGSLBPolicy          `json:"gslb_policy"`
	DNSRecordIDs                map[string]string             `json:"dns_record_ids"`
	CloudflareProxied           bool                          `json:"cloudflare_proxied"`
	DDOSProtectionMode          string                        `json:"ddos_protection_mode"`
	DDOSProtectionProvider      string                        `json:"ddos_protection_provider"`
	DDOSProtectionTarget        string                        `json:"ddos_protection_target"`
	DNSLastSyncStatus           string                        `json:"dns_last_sync_status"`
	DNSLastSyncMessage          string                        `json:"dns_last_sync_message"`
	DNSLastSyncedAt             *time.Time                    `json:"dns_last_synced_at"`
	Remark                      string                        `json:"remark"`
	CreatedAt                   time.Time                     `json:"created_at"`
	UpdatedAt                   time.Time                     `json:"updated_at"`
}

func proxyRouteBasicAuthPasswordHashForView(route *model.ProxyRoute) string {
	if route == nil || !route.BasicAuthEnabled {
		return ""
	}
	passwordHash := strings.TrimSpace(route.BasicAuthPasswordHash)
	if passwordHash != "" {
		return passwordHash
	}
	return model.BasicAuthCredentialHash(route.BasicAuthUsername, route.BasicAuthPassword)
}

// PoW configuration types and validation

// WAF configuration types and validation

// CC protection configuration types and validation
