import type {
  ProxyRouteCustomHeader,
  ProxyRouteGSLBPolicy,
  ProxyRouteItem,
  ProxyRouteMutationPayload,
} from '@/features/proxy-routes/types';

export const websiteConfigSections = [
  {
    key: 'domains',
    label: '域名设置',
    description: '维护站点标识、域名列表和证书绑定。',
  },
  {
    key: 'limits',
    label: '流量限制',
    description: '设置连接数和限速。',
  },
  {
    key: 'proxy',
    label: '反向代理',
    description: '配置主源站地址和源站负载均衡。',
  },
  {
    key: 'dns',
    label: '自动 DNS',
    description: '绑定 Cloudflare，自动解析和切换橙云。',
  },
  {
    key: 'cache',
    label: '缓存策略',
    description: '配置站点缓存策略。',
  },
  {
    key: 'pow',
    label: 'PoW 防护',
    description: '配置 Proof-of-Work 反爬虫策略。',
  },
  {
    key: 'waf',
    label: 'WAF 防护',
    description: '配置本地轻量 WAF 规则。',
  },
  {
    key: 'region',
    label: '地区限制',
    description: '按国家或地区代码放行或拦截访问。',
  },
  {
    key: 'auth',
    label: '认证配置',
    description: '配置基础鉴权访问，需要输入账号密码才能访问网站。',
  },
] as const;

export type WebsiteConfigSectionKey =
  (typeof websiteConfigSections)[number]['key'];

const domainPattern =
  /^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i;
const originHostPattern =
  /^(?:(?:[a-z0-9-]+\.)*[a-z0-9-]+|\[[0-9a-f:.]+\]|[0-9.]+)(?::\d{1,5})?$/i;
const headerKeyPattern = /^[A-Za-z0-9_-]+$/;
const limitRatePattern = /^\d+(?:[kKmM])?$/;

export function buildDefaultGSLBPolicy(
  nodePool = 'default',
): ProxyRouteGSLBPolicy {
  return {
    mode: 'cloudflare_dns',
    strategy: 'load_aware',
    pools: [
      {
        name: nodePool.trim() || 'default',
        weight: 100,
        countries: [],
        enabled: true,
      },
    ],
    target_count: 2,
    ttl: 60,
    source_ip: {
      provider: 'none',
      api_url: '',
      api_token: '',
    },
    load_thresholds: {
      max_openresty_connections: 0,
      max_cpu_percent: 0,
      max_memory_percent: 0,
    },
    debounce: {
      cooldown_seconds: 60,
      unhealthy_threshold: 1,
      recovery_threshold: 1,
    },
  };
}

export function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '请求失败，请稍后重试。';
}

export function getWebsiteConfigSection(
  value: string | null | undefined,
): WebsiteConfigSectionKey {
  return websiteConfigSections.some((section) => section.key === value)
    ? (value as WebsiteConfigSectionKey)
    : 'domains';
}

export function linesFromTextarea(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function validateDomains(domains: string[]) {
  if (domains.length === 0) {
    return '请至少填写一个域名';
  }

  const seen = new Set<string>();
  for (const domain of domains) {
    const normalized = domain.trim().toLowerCase();
    if (!domainPattern.test(normalized)) {
      return `域名格式不合法：${domain}`;
    }
    if (seen.has(normalized)) {
      return `域名重复：${domain}`;
    }
    seen.add(normalized);
  }

  return null;
}

export function parseOriginUrls(value: string) {
  const urls = linesFromTextarea(value);
  if (urls.length === 0) {
    return { urls: [], error: '请至少填写一个源站地址' };
  }

  let sharedScheme = '';
  for (const originUrl of urls) {
    let parsed: URL;
    try {
      parsed = new URL(originUrl);
    } catch {
      return { urls: [], error: `源站地址格式不合法：${originUrl}` };
    }

    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return {
        urls: [],
        error: `源站地址必须以 http:// 或 https:// 开头：${originUrl}`,
      };
    }

    if (!parsed.hostname) {
      return { urls: [], error: `源站地址缺少主机名：${originUrl}` };
    }

    if (urls.length > 1) {
      if ((parsed.pathname && parsed.pathname !== '/') || parsed.search) {
        return {
          urls: [],
          error: '多源站模式暂不支持带路径或查询参数的地址',
        };
      }

      if (!sharedScheme) {
        sharedScheme = parsed.protocol;
      } else if (sharedScheme !== parsed.protocol) {
        return {
          urls: [],
          error: '同一站点的多个源站地址必须使用相同协议',
        };
      }
    }
  }

  return { urls, error: null };
}

export function parseOriginUrl(originUrl: string) {
  const parsed = new URL(originUrl);
  const port = parsed.port || (parsed.protocol === 'http:' ? '80' : '443');
  const path = parsed.pathname === '/' ? '' : parsed.pathname;

  return {
    scheme: parsed.protocol.replace(':', '') as 'http' | 'https',
    address: parsed.hostname,
    port,
    uri: parsed.search ? `${path}${parsed.search}` || parsed.search : path,
  };
}

export function buildOriginUrl(
  scheme: 'http' | 'https',
  address: string,
  port: string,
  uri: string,
) {
  const normalizedAddress = address.trim();
  const normalizedPort = port.trim();
  const normalizedURI = uri.trim();
  if (!normalizedAddress || !normalizedPort) {
    return '';
  }

  const host =
    normalizedAddress.includes(':') && !normalizedAddress.startsWith('[')
      ? `[${normalizedAddress}]`
      : normalizedAddress;

  return `${scheme}://${host}:${normalizedPort}${normalizedURI}`;
}

export function validateOriginHost(value: string) {
  const normalized = value.trim();
  if (!normalized) {
    return null;
  }
  if (
    normalized.includes('://') ||
    /[\/\\\s]/.test(normalized) ||
    !originHostPattern.test(normalized)
  ) {
    return '回源 Host 格式不合法';
  }
  return null;
}

export function parseCustomHeadersText(value: string) {
  const lines = linesFromTextarea(value);
  const headers: ProxyRouteCustomHeader[] = [];

  for (const line of lines) {
    const separatorIndex = line.indexOf(':');
    if (separatorIndex <= 0) {
      return {
        headers: [],
        error: `自定义请求头格式不合法：${line}`,
      };
    }

    const key = line.slice(0, separatorIndex).trim();
    const headerValue = line.slice(separatorIndex + 1).trim();

    if (!headerKeyPattern.test(key)) {
      return {
        headers: [],
        error: `自定义请求头名称不合法：${key}`,
      };
    }

    headers.push({ key, value: headerValue });
  }

  return { headers, error: null };
}

export function customHeadersToText(headers: ProxyRouteCustomHeader[]) {
  return headers.map((header) => `${header.key}: ${header.value}`).join('\n');
}

export function validateLimitRate(value: string) {
  const normalized = value.trim();
  if (!normalized || normalized === '0') {
    return null;
  }
  if (!limitRatePattern.test(normalized)) {
    return '限速格式不合法，请使用 512k、1m 或纯数字';
  }
  return null;
}

export function normalizeLimitRate(value: string) {
  const normalized = value.trim().toLowerCase();
  return normalized === '0' ? '' : normalized;
}

export function validateCacheRules(
  policy: 'url' | 'suffix' | 'path_prefix' | 'path_exact',
  rules: string[],
) {
  if (policy === 'url') {
    return null;
  }

  if (rules.length === 0) {
    return '当前缓存策略至少需要一条规则';
  }

  if (policy === 'suffix') {
    for (const rule of rules) {
      const normalized = rule.replace(/^\./, '');
      if (!normalized || /[\/\\\s]/.test(normalized)) {
        return `缓存后缀格式不合法：${rule}`;
      }
    }
    return null;
  }

  for (const rule of rules) {
    if (!rule.startsWith('/') || rule.includes('://') || /[\s]/.test(rule)) {
      return `缓存路径规则格式不合法：${rule}`;
    }
  }

  return null;
}

export function buildPayloadFromRoute(
  route: ProxyRouteItem,
  overrides: Partial<ProxyRouteMutationPayload>,
): ProxyRouteMutationPayload {
  const primaryOrigin = parseOriginUrl(route.origin_url);

  return {
    site_name: route.site_name,
    domain: route.primary_domain,
    domains: route.domains,
    origin_id: null,
    origin_url: route.origin_url,
    origin_scheme: primaryOrigin.scheme,
    origin_address: primaryOrigin.address,
    origin_port: primaryOrigin.port,
    origin_uri: primaryOrigin.uri,
    origin_host: route.origin_host || '',
    upstreams: route.upstream_list.slice(1),
    node_pool: route.node_pool || 'default',
    enabled: route.enabled,
    enable_https: route.enable_https,
    cert_id: route.cert_id,
    cert_ids: route.cert_ids,
    domain_cert_ids: route.domain_cert_ids,
    redirect_http: route.redirect_http,
    limit_conn_per_server: route.limit_conn_per_server,
    limit_conn_per_ip: route.limit_conn_per_ip,
    limit_rate: route.limit_rate,
    cache_enabled: route.cache_enabled,
    cache_policy: route.cache_policy || 'url',
    cache_rules: route.cache_rule_list,
    custom_headers: route.custom_header_list,
    remark: route.remark || '',
    pow_enabled: route.pow_enabled,
    pow_config: JSON.stringify(route.pow_config),
    waf_enabled: route.waf_enabled,
    waf_mode: route.waf_mode,
    waf_config: JSON.stringify(route.waf_config),
    basic_auth_enabled: route.basic_auth_enabled,
    basic_auth_username: route.basic_auth_username,
    basic_auth_password: route.basic_auth_password,
    region_restriction_enabled: route.region_restriction_enabled,
    region_restriction_mode: route.region_restriction_mode,
    region_restriction_countries: route.region_restriction_countries,
    dns_auto_sync: route.dns_auto_sync,
    dns_account_id: route.dns_account_id,
    dns_zone_id: route.dns_zone_id,
    dns_record_type: route.dns_record_type,
    dns_record_name: route.dns_record_name,
    dns_record_content: route.dns_record_content,
    dns_auto_target: route.dns_auto_target,
    dns_target_count: route.dns_target_count || 1,
    dns_schedule_mode: route.dns_schedule_mode || 'healthy',
    dns_ttl: route.dns_ttl || 1,
    gslb_enabled: route.gslb_enabled,
    gslb_policy:
      route.gslb_policy || buildDefaultGSLBPolicy(route.node_pool || 'default'),
    cloudflare_proxied: route.cloudflare_proxied,
    ddos_protection_mode: route.ddos_protection_mode,
    ...overrides,
  };
}

export function getUpstreamSummary(route: ProxyRouteItem) {
  if (route.upstream_list.length <= 1) {
    return route.origin_url;
  }
  return `${route.upstream_list.length} 个源站地址，主源站 ${route.origin_url}`;
}

export function getWebsiteStatusBadges(route: ProxyRouteItem) {
  return [
    route.enabled
      ? { label: '已启用', variant: 'success' as const }
      : { label: '已停用', variant: 'warning' as const },
    route.enable_https
      ? { label: 'HTTPS', variant: 'info' as const }
      : { label: 'HTTP', variant: 'warning' as const },
  ];
}
