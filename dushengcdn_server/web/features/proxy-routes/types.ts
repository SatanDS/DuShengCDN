export interface ProxyRouteCustomHeader {
  key: string;
  value: string;
}

export interface ProxyRoutePoWListConfig {
  ips: string[];
  ip_cidrs: string[];
  paths: string[];
  path_regexes: string[];
  user_agents: string[];
}

export interface ProxyRoutePoWConfig {
  difficulty: number;
  algorithm: 'fast' | 'slow';
  session_ttl: number;
  challenge_ttl: number;
  whitelist: ProxyRoutePoWListConfig;
  blacklist: ProxyRoutePoWListConfig;
}

export interface ProxyRouteWAFWhitelistConfig {
  ips: string[];
  ip_cidrs: string[];
  paths: string[];
}

export interface ProxyRouteWAFCustomRules {
  path_contains: string[];
  path_regexes: string[];
  query_contains: string[];
  header_contains: string[];
  user_agents: string[];
}

export interface ProxyRouteWAFConfig {
  builtin_rules: Array<
    'sqli' | 'xss' | 'path_traversal' | 'sensitive_paths' | 'bad_bots'
  >;
  whitelist: ProxyRouteWAFWhitelistConfig;
  block_rules: ProxyRouteWAFCustomRules;
}

export interface ProxyRouteCCListConfig {
  ips: string[];
  ip_cidrs: string[];
  paths: string[];
  user_agents: string[];
}

export interface ProxyRouteCCConfig {
  window_seconds: number;
  max_requests: number;
  path_window_seconds: number;
  path_max_requests: number;
  block_duration_seconds: number;
  whitelist: ProxyRouteCCListConfig;
  exclude: ProxyRouteCCListConfig;
}

export interface ProxyRouteGSLBPoolPolicy {
  name: string;
  weight: number;
  countries: string[];
  source_cidrs: string[];
  operators?: string[];
  asns?: number[];
  exclude_countries?: string[];
  exclude_source_cidrs?: string[];
  exclude_operators?: string[];
  exclude_asns?: number[];
  node_ids?: string[];
  enabled: boolean;
}

export interface ProxyRouteGSLBPolicy {
  mode: 'cloudflare_dns';
  strategy: 'healthy' | 'weighted' | 'load_aware';
  pool_match_mode?: 'priority' | 'mixed_weighted';
  pools: ProxyRouteGSLBPoolPolicy[];
  target_count: number;
  ttl: number;
  source_pool_fallback_mode?: 'strict' | 'fallback_to_global';
  source_ip: {
    provider: 'none' | 'http';
    api_url: string;
    api_token: string;
  };
  load_thresholds: {
    max_openresty_connections: number;
    max_cpu_percent: number;
    max_memory_percent: number;
  };
  debounce: {
    cooldown_seconds: number;
    unhealthy_threshold: number;
    recovery_threshold: number;
  };
}

export interface ProxyRouteItem {
  id: number;
  site_name: string;
  domain: string;
  domains: string[];
  primary_domain: string;
  domain_count: number;
  origin_id: number | null;
  origin_url: string;
  origin_host: string;
  upstreams: string;
  upstream_list: string[];
  node_pool: string;
  enabled: boolean;
  enable_https: boolean;
  cert_id: number | null;
  cert_ids: number[];
  domain_cert_ids: number[];
  redirect_http: boolean;
  limit_conn_per_server: number;
  limit_conn_per_ip: number;
  limit_rate: string;
  proxy_buffering_mode: 'default' | 'off';
  cache_enabled: boolean;
  cache_policy: string;
  cache_rules: string;
  cache_rule_list: string[];
  custom_headers: string;
  custom_header_list: ProxyRouteCustomHeader[];
  pow_enabled: boolean;
  pow_config: ProxyRoutePoWConfig;
  waf_enabled: boolean;
  waf_mode: 'log' | 'block';
  waf_config: ProxyRouteWAFConfig;
  cc_enabled: boolean;
  cc_mode: 'log' | 'block' | 'pow';
  cc_config: ProxyRouteCCConfig;
  basic_auth_enabled: boolean;
  basic_auth_username: string;
  basic_auth_password: string;
  region_restriction_enabled: boolean;
  region_restriction_mode: 'allow' | 'block';
  region_restriction_countries: string[];
  dns_auto_sync: boolean;
  dns_account_id: number | null;
  dns_zone_id: string;
  dns_record_type: 'A' | 'AAAA' | 'CNAME';
  dns_record_name: string;
  dns_record_content: string;
  dns_auto_target: boolean;
  dns_target_count: number;
  dns_schedule_mode: 'healthy' | 'weighted' | 'load_aware';
  dns_ttl: number;
  gslb_enabled: boolean;
  gslb_policy: ProxyRouteGSLBPolicy;
  dns_record_ids: Record<string, string>;
  cloudflare_proxied: boolean;
  ddos_protection_mode: 'off' | 'auto';
  ddos_protection_provider: 'cloudflare' | 'custom';
  ddos_protection_target: string;
  dns_last_sync_status: string;
  dns_last_sync_message: string;
  dns_last_synced_at?: string | null;
  dns_provider_mode: 'cloudflare' | 'authoritative';
  dns_zone_id_ref: number | null;
  remark: string;
  created_at: string;
  updated_at: string;
}

export interface ProxyRouteMutationPayload {
  site_name?: string;
  domain: string;
  domains?: string[];
  origin_id: number | null;
  origin_url: string;
  origin_scheme: 'http' | 'https';
  origin_address: string;
  origin_port: string;
  origin_uri: string;
  origin_host: string;
  upstreams: string[];
  node_pool?: string;
  enabled: boolean;
  enable_https: boolean;
  cert_id: number | null;
  cert_ids?: number[];
  domain_cert_ids?: number[];
  redirect_http: boolean;
  limit_conn_per_server?: number;
  limit_conn_per_ip?: number;
  limit_rate?: string;
  proxy_buffering_mode?: 'default' | 'off';
  cache_enabled: boolean;
  cache_policy: string;
  cache_rules: string[];
  custom_headers: ProxyRouteCustomHeader[];
  pow_enabled: boolean;
  pow_config: string;
  waf_enabled?: boolean;
  waf_mode?: 'log' | 'block';
  waf_config?: string;
  cc_enabled?: boolean;
  cc_mode?: 'log' | 'block' | 'pow';
  cc_config?: string;
  basic_auth_enabled: boolean;
  basic_auth_username?: string;
  basic_auth_password?: string;
  region_restriction_enabled?: boolean;
  region_restriction_mode?: 'allow' | 'block';
  region_restriction_countries?: string[];
  dns_auto_sync?: boolean;
  dns_account_id?: number | null;
  dns_zone_id?: string;
  dns_record_type?: 'A' | 'AAAA' | 'CNAME';
  dns_record_name?: string;
  dns_record_content?: string;
  dns_auto_target?: boolean;
  dns_target_count?: number;
  dns_schedule_mode?: 'healthy' | 'weighted' | 'load_aware';
  dns_ttl?: number;
  dns_provider_mode?: 'cloudflare' | 'authoritative';
  dns_zone_id_ref?: number | null;
  gslb_enabled?: boolean;
  gslb_policy?: ProxyRouteGSLBPolicy;
  cloudflare_proxied?: boolean;
  ddos_protection_mode?: 'off' | 'auto';
  ddos_protection_provider?: 'cloudflare' | 'custom';
  ddos_protection_target?: string;
  remark: string;
}

export interface TlsCertificateItem {
  id: number;
  name: string;
  not_after?: string | null;
}

export interface ManagedDomainMatchCandidate {
  managed_domain_id: number;
  domain: string;
  match_type: 'exact' | 'wildcard';
  certificate_id: number;
  certificate_name: string;
}

export interface ManagedDomainMatchResult {
  domain: string;
  matched: boolean;
  candidate?: ManagedDomainMatchCandidate;
  candidates: ManagedDomainMatchCandidate[];
}

export interface ProxyRouteCacheOperationPayload {
  scope?: 'all' | 'url' | 'prefix';
  urls?: string[];
  prefixes?: string[];
}

export interface ProxyRouteCacheOperationResult {
  operation_id: string;
  action: 'purge' | 'warm';
  target_nodes: string[];
  failed_nodes: string[];
}
