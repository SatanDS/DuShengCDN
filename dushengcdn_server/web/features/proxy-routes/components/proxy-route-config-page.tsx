'use client';

import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import type { ReactNode } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { LoadingState } from '@/components/feedback/loading-state';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { getDnsAccounts } from '@/features/dns-accounts/api/dns-accounts';
import type { DnsAccountItem } from '@/features/dns-accounts/types';
import { getManagedDomains } from '@/features/managed-domains/api/managed-domains';
import { getNodes } from '@/features/nodes/api/nodes';
import {
  getProxyRoute,
  purgeProxyRouteCache,
  updateProxyRoute,
  warmProxyRouteCache,
} from '@/features/proxy-routes/api/proxy-routes';
import {
  buildDomainRowsFromRoute,
  DomainListInput,
  type DomainListRow,
} from '@/features/proxy-routes/components/domain-list-input';
import {
  buildNodePoolOptions,
  NodePoolSelect,
} from '@/features/proxy-routes/components/node-pool-select';
import {
  buildPayloadFromRoute,
  buildDefaultGSLBPolicy,
  customHeadersToText,
  getErrorMessage,
  getWebsiteConfigSection,
  linesFromTextarea,
  normalizeLimitRate,
  parseCustomHeadersText,
  parseOriginUrl,
  parseOriginUrls,
  validateCacheRules,
  validateDomains,
  validateLimitRate,
  validateOriginHost,
  websiteConfigSections,
} from '@/features/proxy-routes/helpers';
import type {
  ProxyRouteItem,
  ProxyRouteMutationPayload,
} from '@/features/proxy-routes/types';
import { getTlsCertificates } from '@/features/tls-certificates/api/tls-certificates';
import type { TlsCertificateItem } from '@/features/tls-certificates/types';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';

type FeedbackState = {
  tone: 'success' | 'danger';
  message: string;
};

export type SaveContext = {
  message: string;
};

export type SaveHandler = (
  payload: ProxyRouteMutationPayload,
  context: SaveContext,
) => void;

type ConfigSectionPresentationProps = {
  formId?: string;
  embedded?: boolean;
};

const domainSettingsSchema = z
  .object({
    site_name: z
      .string()
      .trim()
      .min(1, '请输入站点标识')
      .max(255, '站点标识不能超过 255 个字符'),
    domain_rows: z
      .array(
        z.object({
          domain: z.string(),
          certificateId: z.string(),
        }),
      )
      .min(1),
    enabled: z.boolean(),
    redirect_http: z.boolean(),
  })
  .superRefine((value, context) => {
    const domains = value.domain_rows
      .map((item) => item.domain.trim().toLowerCase())
      .filter(Boolean);
    const error = validateDomains(domains);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['domain_rows'],
        message: error,
      });
    }

    const selectedCertificateCount = new Set(
      value.domain_rows
        .map((item) => Number(item.certificateId))
        .filter((item) => Number.isFinite(item) && item > 0),
    ).size;
    if (value.redirect_http && selectedCertificateCount === 0) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['redirect_http'],
        message: '启用 HTTP 跳转前，请先为域名选择证书',
      });
    }
  });

const rateLimitSchema = z
  .object({
    limit_conn_per_server: z.string(),
    limit_conn_per_ip: z.string(),
    limit_rate: z.string(),
  })
  .superRefine((value, context) => {
    for (const field of [
      'limit_conn_per_server',
      'limit_conn_per_ip',
    ] as const) {
      const rawValue = value[field].trim();
      if (!rawValue) {
        continue;
      }
      if (!/^\d+$/.test(rawValue)) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: [field],
          message: '请输入大于等于 0 的整数',
        });
      }
    }

    const limitRateError = validateLimitRate(value.limit_rate);
    if (limitRateError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['limit_rate'],
        message: limitRateError,
      });
    }
  });

const reverseProxySchema = z
  .object({
    origin_urls_text: z.string().trim().min(1, '请至少填写一个源站地址'),
    node_pool: z.string().trim().max(64, '节点池名称不能超过 64 个字符'),
    origin_host: z.string(),
    custom_headers_text: z.string(),
    remark: z.string().max(255, '备注不能超过 255 个字符'),
  })
  .superRefine((value, context) => {
    const { error } = parseOriginUrls(value.origin_urls_text);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['origin_urls_text'],
        message: error,
      });
    }

    const originHostError = validateOriginHost(value.origin_host);
    if (originHostError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['origin_host'],
        message: originHostError,
      });
    }

    const { error: headerError } = parseCustomHeadersText(
      value.custom_headers_text,
    );
    if (headerError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['custom_headers_text'],
        message: headerError,
      });
    }
  });

const cacheSchema = z
  .object({
    cache_enabled: z.boolean(),
    cache_policy: z.enum(['url', 'suffix', 'path_prefix', 'path_exact']),
    cache_rules_text: z.string(),
  })
  .superRefine((value, context) => {
    if (!value.cache_enabled) {
      return;
    }

    const rules = linesFromTextarea(value.cache_rules_text);
    const error = validateCacheRules(value.cache_policy, rules);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['cache_rules_text'],
        message: error,
      });
    }
  });

const regionRestrictionSchema = z
  .object({
    region_restriction_enabled: z.boolean(),
    region_restriction_mode: z.enum(['allow', 'block']),
    region_restriction_countries_text: z.string(),
  })
  .superRefine((value, context) => {
    if (!value.region_restriction_enabled) {
      return;
    }
    const countries = parseRegionCountriesText(
      value.region_restriction_countries_text,
    );
    if (countries.length === 0) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['region_restriction_countries_text'],
        message: '请至少填写一个国家或地区代码',
      });
      return;
    }
    for (const country of countries) {
      if (!/^[A-Z0-9]{2}$/.test(country)) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['region_restriction_countries_text'],
          message: `国家或地区代码格式不合法：${country}`,
        });
        return;
      }
    }
  });

type DomainSettingsValues = z.infer<typeof domainSettingsSchema>;
type RateLimitValues = z.infer<typeof rateLimitSchema>;
type ReverseProxyValues = z.infer<typeof reverseProxySchema>;
type CacheValues = z.infer<typeof cacheSchema>;
type RegionRestrictionValues = z.infer<typeof regionRestrictionSchema>;

type DNSAutomationValues = {
  dns_auto_sync: boolean;
  dns_account_id: string;
  dns_zone_id: string;
  dns_record_type: 'A' | 'AAAA' | 'CNAME';
  dns_record_name: string;
  dns_record_content: string;
  dns_auto_target: boolean;
  dns_target_count: number;
  dns_schedule_mode: 'healthy' | 'weighted' | 'load_aware';
  dns_ttl: number;
  gslb_enabled: boolean;
  gslb_pools_text: string;
  gslb_max_openresty_connections: number;
  gslb_cooldown_seconds: number;
  cloudflare_proxied: boolean;
  ddos_protection_mode: 'off' | 'manual' | 'auto';
};

function normalizeSelectedCertificateIDs(rows: DomainListRow[]) {
  return Array.from(
    new Set(
      rows
        .filter((item) => item.domain.trim() !== '')
        .map((item) => Number(item.certificateId))
        .filter((item) => Number.isFinite(item) && item > 0),
    ),
  );
}

function buildDomainCertificateIDs(rows: DomainListRow[]) {
  return rows
    .filter((item) => item.domain.trim() !== '')
    .map((item) => {
      const certificateID = Number(item.certificateId);
      return Number.isFinite(certificateID) && certificateID > 0
        ? certificateID
        : 0;
    });
}

function buildDomainRows(route: ProxyRouteItem) {
  const selectedCertIDs =
    route.cert_ids.length > 0
      ? route.cert_ids
      : route.cert_id
        ? [route.cert_id]
        : [];

  return buildDomainRowsFromRoute(
    route.domains,
    route.domain_cert_ids,
    selectedCertIDs,
  );
}

function parseRegionCountriesText(value: string) {
  return value
    .split(/[\s,，;；]+/)
    .map((item) => item.trim().toUpperCase())
    .filter(Boolean);
}

function gslbPoolsToText(route: ProxyRouteItem) {
  const pools =
    route.gslb_policy?.pools?.length > 0
      ? route.gslb_policy.pools
      : buildDefaultGSLBPolicy(route.node_pool || 'default').pools;
  return pools
    .filter((pool) => pool.enabled !== false)
    .map((pool) => {
      const countries = pool.countries?.length
        ? ` ${pool.countries.join(',')}`
        : '';
      return `${pool.name} ${pool.weight || 100}${countries}`.trim();
    })
    .join('\n');
}

function parseGSLBPoolsText(value: string) {
  const pools = linesFromTextarea(value).map((line) => {
    const [name = '', rawWeight = '100', rawCountries = ''] = line.split(/\s+/, 3);
    const weight = Number(rawWeight);
    return {
      name: name.trim(),
      weight: Number.isFinite(weight) && weight > 0 ? weight : 100,
      countries: rawCountries
        .split(/[,，]/)
        .map((item) => item.trim().toUpperCase())
        .filter((item) => /^[A-Z0-9]{2}$/.test(item)),
      enabled: true,
    };
  });
  return pools.filter((pool) => pool.name !== '');
}

function ConfigSectionShell({
  title,
  description,
  formId,
  saving,
  embedded = false,
  children,
}: {
  title: string;
  description: string;
  formId: string;
  saving: boolean;
  embedded?: boolean;
  children: ReactNode;
}) {
  if (embedded) {
    return (
      <section className="space-y-5">
        <div className="flex flex-col gap-3 border-b border-[var(--border-default)] pb-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
              {title}
            </h3>
            <p className="mt-1 text-sm leading-6 text-[var(--foreground-secondary)]">
              {description}
            </p>
          </div>
          <PrimaryButton type="submit" form={formId} disabled={saving}>
            {saving ? '保存中...' : '保存'}
          </PrimaryButton>
        </div>
        {children}
      </section>
    );
  }

  return (
    <AppCard
      title={title}
      description={description}
      action={
        <PrimaryButton type="submit" form={formId} disabled={saving}>
          {saving ? '保存中...' : '保存'}
        </PrimaryButton>
      }
    >
      {children}
    </AppCard>
  );
}

function DomainSettingsSection({
  route,
  certificates,
  saving,
  onSave,
  suggestionSources,
}: {
  route: ProxyRouteItem;
  certificates: TlsCertificateItem[];
  saving: boolean;
  onSave: SaveHandler;
  suggestionSources: string[];
}) {
  const form = useForm<DomainSettingsValues>({
    resolver: zodResolver(domainSettingsSchema),
    defaultValues: {
      site_name: route.site_name,
      domain_rows: buildDomainRows(route),
      enabled: route.enabled,
      redirect_http: route.redirect_http,
    },
  });

  useEffect(() => {
    form.reset({
      site_name: route.site_name,
      domain_rows: buildDomainRows(route),
      enabled: route.enabled,
      redirect_http: route.redirect_http,
    });
  }, [form, route]);

  const selectedCertificateIDs = normalizeSelectedCertificateIDs(
    form.watch('domain_rows'),
  );

  return (
    <ConfigSectionShell
      title="域名设置"
      description="在一个列表里同时维护域名、证书和 HTTPS 跳转。保存时会自动汇总站点证书集合。"
      formId="proxy-route-domains-form"
      saving={saving}
    >
      <form
        id="proxy-route-domains-form"
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const domains = values.domain_rows
            .map((item) => item.domain.trim().toLowerCase())
            .filter(Boolean);
          const domainCertIDs = buildDomainCertificateIDs(values.domain_rows);
          const certIDs = normalizeSelectedCertificateIDs(values.domain_rows);

          onSave(
            buildPayloadFromRoute(route, {
              site_name: values.site_name.trim(),
              domain: domains[0],
              domains,
              enabled: values.enabled,
              enable_https: certIDs.length > 0,
              cert_id: certIDs[0] ?? null,
              cert_ids: certIDs,
              domain_cert_ids: domainCertIDs,
              redirect_http: certIDs.length > 0 ? values.redirect_http : false,
            }),
            { message: '域名设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用站点"
          description="关闭后会保留配置，但不会参与发布。"
          checked={form.watch('enabled')}
          onChange={(checked) =>
            form.setValue('enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="站点标识"
          hint="建议使用稳定、可读的业务标识，不必与域名完全一致。"
          error={form.formState.errors.site_name?.message}
        >
          <ResourceInput
            placeholder="marketing-site"
            {...form.register('site_name')}
          />
        </ResourceField>

        <ResourceField
          label="域名列表"
          hint="每行配置一个域名。可为不同域名选择不同证书，相同证书也可以重复选择。"
          error={
            form.formState.errors.domain_rows?.message as string | undefined
          }
          container="div"
        >
          <Controller
            control={form.control}
            name="domain_rows"
            render={({ field }) => (
              <DomainListInput
                rows={field.value}
                onChange={field.onChange}
                onBlur={field.onBlur}
                suggestionSources={suggestionSources}
                certificates={certificates}
              />
            )}
          />
        </ResourceField>

        <ToggleField
          label="HTTP 自动跳转到 HTTPS"
          description={
            selectedCertificateIDs.length > 0
              ? '开启后会额外生成 80 端口重定向规则。'
              : '至少为一个域名选择证书后才能启用。'
          }
          checked={form.watch('redirect_http')}
          disabled={selectedCertificateIDs.length === 0}
          onChange={(checked) =>
            form.setValue('redirect_http', checked, { shouldDirty: true })
          }
        />
      </form>
    </ConfigSectionShell>
  );
}

function RateLimitSection({
  route,
  saving,
  onSave,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
}) {
  const form = useForm<RateLimitValues>({
    resolver: zodResolver(rateLimitSchema),
    defaultValues: {
      limit_conn_per_server: route.limit_conn_per_server
        ? String(route.limit_conn_per_server)
        : '',
      limit_conn_per_ip: route.limit_conn_per_ip
        ? String(route.limit_conn_per_ip)
        : '',
      limit_rate: route.limit_rate || '',
    },
  });

  useEffect(() => {
    form.reset({
      limit_conn_per_server: route.limit_conn_per_server
        ? String(route.limit_conn_per_server)
        : '',
      limit_conn_per_ip: route.limit_conn_per_ip
        ? String(route.limit_conn_per_ip)
        : '',
      limit_rate: route.limit_rate || '',
    });
  }, [form, route]);

  return (
    <ConfigSectionShell
      title="流量限制"
      description="站点限流，空值或 0 表示关闭。"
      formId="proxy-route-limits-form"
      saving={saving}
    >
      <form
        id="proxy-route-limits-form"
        className="grid gap-5 md:grid-cols-2"
        onSubmit={form.handleSubmit((values) => {
          onSave(
            buildPayloadFromRoute(route, {
              limit_conn_per_server: Number(
                values.limit_conn_per_server.trim() || '0',
              ),
              limit_conn_per_ip: Number(values.limit_conn_per_ip.trim() || '0'),
              limit_rate: normalizeLimitRate(values.limit_rate),
            }),
            { message: '流量限制已保存。' },
          );
        })}
      >
        <ResourceField
          label="并发限制"
          hint="限制当前站点最大并发连接数。"
          error={form.formState.errors.limit_conn_per_server?.message}
        >
          <ResourceInput
            placeholder="120"
            {...form.register('limit_conn_per_server')}
          />
        </ResourceField>

        <ResourceField
          label="单 IP 限制"
          hint="限制单个 IP 的最大并发数。"
          error={form.formState.errors.limit_conn_per_ip?.message}
        >
          <ResourceInput
            placeholder="12"
            {...form.register('limit_conn_per_ip')}
          />
        </ResourceField>

        <ResourceField
          label="限速"
          hint="限制单请求带宽，例如 512k 或 1m。"
          error={form.formState.errors.limit_rate?.message}
          className="md:col-span-2"
        >
          <ResourceInput
            placeholder="512k/1m"
            {...form.register('limit_rate')}
          />
        </ResourceField>
      </form>
    </ConfigSectionShell>
  );
}

function ReverseProxySection({
  route,
  saving,
  onSave,
  nodePoolOptions,
  nodePoolsLoading,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  nodePoolOptions: string[];
  nodePoolsLoading: boolean;
  onSave: SaveHandler;
}) {
  const form = useForm<ReverseProxyValues>({
    resolver: zodResolver(reverseProxySchema),
    defaultValues: {
      origin_urls_text: route.upstream_list.join('\n'),
      node_pool: route.node_pool || 'default',
      origin_host: route.origin_host || '',
      custom_headers_text: customHeadersToText(route.custom_header_list),
      remark: route.remark || '',
    },
  });

  useEffect(() => {
    form.reset({
      origin_urls_text: route.upstream_list.join('\n'),
      node_pool: route.node_pool || 'default',
      origin_host: route.origin_host || '',
      custom_headers_text: customHeadersToText(route.custom_header_list),
      remark: route.remark || '',
    });
  }, [form, route]);

  return (
    <ConfigSectionShell
      title="反向代理"
      description="第一行作为主源站；填写多行时会自动进入多源站负载均衡模式。"
      formId="proxy-route-proxy-form"
      saving={saving}
    >
      <form
        id="proxy-route-proxy-form"
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const { urls } = parseOriginUrls(values.origin_urls_text);
          const primaryOrigin = parseOriginUrl(urls[0]);
          const { headers } = parseCustomHeadersText(
            values.custom_headers_text,
          );

          onSave(
            buildPayloadFromRoute(route, {
              origin_id: null,
              origin_url: urls[0],
              origin_scheme: primaryOrigin.scheme,
              origin_address: primaryOrigin.address,
              origin_port: primaryOrigin.port,
              origin_uri: primaryOrigin.uri,
              node_pool: values.node_pool.trim() || 'default',
              origin_host: values.origin_host.trim(),
              upstreams: urls.slice(1),
              custom_headers: headers,
              remark: values.remark.trim(),
            }),
            { message: '反向代理设置已保存。' },
          );
        })}
      >
        <ResourceField
          label="源站地址"
          hint="每行一个完整 URL，协议和端口都在这里配置，例如 https://origin.internal:443。多源站模式下不要带 path 或 query。"
          error={form.formState.errors.origin_urls_text?.message}
        >
          <ResourceTextarea
            aria-label="源站地址"
            className="min-h-40"
            placeholder={
              'https://origin-a.internal:443\nhttps://origin-b.internal:443'
            }
            {...form.register('origin_urls_text')}
          />
        </ResourceField>

        <ResourceField
          label="节点池"
          hint="自动 DNS 会从该节点池选择公网 IP，缓存运行时操作也会下发到该池在线节点。"
          error={form.formState.errors.node_pool?.message}
          container="div"
        >
          <Controller
            control={form.control}
            name="node_pool"
            render={({ field }) => (
              <NodePoolSelect
                name={field.name}
                value={field.value}
                options={nodePoolOptions}
                disabled={nodePoolsLoading}
                onChange={field.onChange}
                onBlur={field.onBlur}
              />
            )}
          />
        </ResourceField>

        <ResourceField
          label="Origin Host Header"
          hint="留空时默认透传访问域名 $host。"
          error={form.formState.errors.origin_host?.message}
        >
          <ResourceInput
            placeholder="origin.example.internal"
            {...form.register('origin_host')}
          />
        </ResourceField>

        <ResourceField
          label="自定义请求头"
          hint="每行一条，格式为 Key: Value。"
          error={form.formState.errors.custom_headers_text?.message}
        >
          <ResourceTextarea
            className="min-h-32"
            placeholder={'X-Trace-Id: $request_id\nX-Site: marketing'}
            {...form.register('custom_headers_text')}
          />
        </ResourceField>

        <ResourceField
          label="备注"
          error={form.formState.errors.remark?.message}
        >
          <ResourceTextarea
            placeholder="例如：多活回源，优先使用上海入口"
            {...form.register('remark')}
          />
        </ResourceField>
      </form>
    </ConfigSectionShell>
  );
}

export function DNSAutomationSection({
  route,
  dnsAccounts,
  saving,
  onSave,
  formId = 'proxy-route-dns-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  dnsAccounts: DnsAccountItem[];
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<DNSAutomationValues>({
    defaultValues: {
      dns_auto_sync: route.dns_auto_sync,
      dns_account_id: route.dns_account_id ? String(route.dns_account_id) : '',
      dns_zone_id: route.dns_zone_id || '',
      dns_record_type: route.dns_record_type || 'A',
      dns_record_name: route.dns_record_name || '',
      dns_record_content: route.dns_record_content || '',
      dns_auto_target: route.dns_auto_target,
      dns_target_count: route.dns_target_count || 1,
      dns_schedule_mode: route.dns_schedule_mode || 'healthy',
      dns_ttl: route.dns_ttl || 1,
      gslb_enabled: route.gslb_enabled,
      gslb_pools_text: gslbPoolsToText(route),
      gslb_max_openresty_connections:
        route.gslb_policy?.load_thresholds?.max_openresty_connections || 0,
      gslb_cooldown_seconds:
        route.gslb_policy?.debounce?.cooldown_seconds || 60,
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
    },
  });

  useEffect(() => {
    form.reset({
      dns_auto_sync: route.dns_auto_sync,
      dns_account_id: route.dns_account_id ? String(route.dns_account_id) : '',
      dns_zone_id: route.dns_zone_id || '',
      dns_record_type: route.dns_record_type || 'A',
      dns_record_name: route.dns_record_name || '',
      dns_record_content: route.dns_record_content || '',
      dns_auto_target: route.dns_auto_target,
      dns_target_count: route.dns_target_count || 1,
      dns_schedule_mode: route.dns_schedule_mode || 'healthy',
      dns_ttl: route.dns_ttl || 1,
      gslb_enabled: route.gslb_enabled,
      gslb_pools_text: gslbPoolsToText(route),
      gslb_max_openresty_connections:
        route.gslb_policy?.load_thresholds?.max_openresty_connections || 0,
      gslb_cooldown_seconds:
        route.gslb_policy?.debounce?.cooldown_seconds || 60,
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
    });
  }, [form, route]);

  const autoSyncEnabled = form.watch('dns_auto_sync');
  const recordType = form.watch('dns_record_type');
  const autoTarget = form.watch('dns_auto_target');
  const gslbEnabled = form.watch('gslb_enabled');

  return (
    <ConfigSectionShell
      title="自动 DNS"
      description="绑定 Cloudflare 后，创建或保存规则时自动解析域名；节点离线时后台任务会切换到在线节点。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const dnsAccountID = Number(values.dns_account_id);
          const baseGSLBPolicy =
            route.gslb_policy || buildDefaultGSLBPolicy(route.node_pool);
          const gslbPools = parseGSLBPoolsText(values.gslb_pools_text);
          onSave(
            buildPayloadFromRoute(route, {
              dns_auto_sync: values.dns_auto_sync,
              dns_account_id:
                values.dns_auto_sync &&
                Number.isFinite(dnsAccountID) &&
                dnsAccountID > 0
                  ? dnsAccountID
                  : null,
              dns_zone_id: values.dns_zone_id.trim(),
              dns_record_type: values.dns_record_type,
              dns_record_name: values.dns_record_name.trim(),
              dns_record_content: values.dns_record_content.trim(),
              dns_auto_target: values.gslb_enabled
                ? true
                : values.dns_auto_target,
              dns_target_count: values.dns_target_count,
              dns_schedule_mode: values.dns_schedule_mode,
              dns_ttl: values.dns_ttl,
              gslb_enabled: values.gslb_enabled,
              gslb_policy: {
                ...baseGSLBPolicy,
                mode: 'cloudflare_dns',
                strategy: values.dns_schedule_mode,
                target_count: values.dns_target_count,
                ttl: values.dns_ttl,
                pools:
                  gslbPools.length > 0
                    ? gslbPools
                    : buildDefaultGSLBPolicy(route.node_pool).pools,
                load_thresholds: {
                  ...baseGSLBPolicy.load_thresholds,
                  max_openresty_connections:
                    values.gslb_max_openresty_connections,
                },
                debounce: {
                  ...baseGSLBPolicy.debounce,
                  cooldown_seconds: values.gslb_cooldown_seconds,
                },
              },
              cloudflare_proxied: values.cloudflare_proxied,
              ddos_protection_mode: values.ddos_protection_mode,
            }),
            { message: '自动 DNS 设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用 Cloudflare 自动 DNS"
          description="开启后会为当前规则域名创建或更新 Cloudflare DNS 记录。"
          checked={autoSyncEnabled}
          onChange={(checked) =>
            form.setValue('dns_auto_sync', checked, { shouldDirty: true })
          }
        />

        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="DNS 账号"
            hint="需要 Cloudflare API Token 具备 Zone Read 和 DNS Edit 权限。"
            error={
              autoSyncEnabled && !form.watch('dns_account_id')
                ? '启用自动 DNS 时请选择 DNS 账号'
                : undefined
            }
          >
            <ResourceSelect
              disabled={!autoSyncEnabled}
              {...form.register('dns_account_id')}
            >
              <option value="">请选择 DNS 账号</option>
              {dnsAccounts
                .filter((account) => account.type === 'cloudflare')
                .map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.name}
                  </option>
                ))}
            </ResourceSelect>
          </ResourceField>

          <ResourceField
            label="记录类型"
            hint="默认 A 记录。自动选择节点时只支持 A 或 AAAA。"
          >
            <ResourceSelect
              disabled={!autoSyncEnabled}
              {...form.register('dns_record_type')}
            >
              <option value="A">A</option>
              <option value="AAAA">AAAA</option>
              <option value="CNAME">CNAME</option>
            </ResourceSelect>
          </ResourceField>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="Zone ID"
            hint="可留空，系统会按主域名自动查找 Cloudflare Zone。"
          >
            <ResourceInput
              disabled={!autoSyncEnabled}
              placeholder="留空自动识别"
              {...form.register('dns_zone_id')}
            />
          </ResourceField>

          <ResourceField
            label="记录名称"
            hint="可留空，默认同步规则里的所有域名。单域名规则可手动指定。"
          >
            <ResourceInput
              disabled={!autoSyncEnabled}
              placeholder={route.primary_domain}
              {...form.register('dns_record_name')}
            />
          </ResourceField>
        </div>

        <ResourceField
          label="记录内容"
          hint={
            recordType === 'CNAME'
              ? 'CNAME 必须手动填写目标域名。'
              : '启用自动选择时，系统会使用节点池中的在线公网 IP；关闭后可固定多个 A/AAAA 内容。'
          }
        >
          <ResourceInput
            disabled={!autoSyncEnabled || autoTarget}
            placeholder={
              recordType === 'CNAME'
                ? 'target.example.com'
                : '留空自动选择，或填写多个 IP'
            }
            {...form.register('dns_record_content')}
          />
        </ResourceField>

        <ToggleField
          label="自动选择在线节点 IP"
          description="开启后节点离线会自动切换到其他在线节点；手动记录内容不会被后台任务覆盖。"
          checked={autoTarget}
          disabled={!autoSyncEnabled || recordType === 'CNAME'}
          onChange={(checked) =>
            form.setValue('dns_auto_target', checked, { shouldDirty: true })
          }
        />

        {recordType !== 'CNAME' ? (
          <div className="grid gap-5 md:grid-cols-3">
            <ResourceField
              label="目标数量"
              hint="自动选择时最多同步多少个节点 IP。"
            >
              <ResourceInput
                type="number"
                min={1}
                max={20}
                disabled={!autoSyncEnabled || !autoTarget}
                {...form.register('dns_target_count', {
                  valueAsNumber: true,
                })}
              />
            </ResourceField>

            <ResourceField
              label="调度模式"
              hint="负载感知会结合连接数、CPU 和内存指标进行评分。"
            >
              <ResourceSelect
                disabled={!autoSyncEnabled || !autoTarget}
                {...form.register('dns_schedule_mode')}
              >
                <option value="healthy">按健康时间</option>
                <option value="weighted">按权重优先</option>
                <option value="load_aware">负载感知</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField label="DNS TTL" hint="1 表示 Cloudflare 自动 TTL。">
              <ResourceInput
                type="number"
                min={1}
                max={86400}
                disabled={!autoSyncEnabled}
                {...form.register('dns_ttl', {
                  valueAsNumber: true,
                })}
              />
            </ResourceField>
          </div>
        ) : null}

        {recordType !== 'CNAME' ? (
          <div className="space-y-5 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
            <ToggleField
              label="启用 GSLB 多节点池调度"
              description="开启后自动 DNS 会按多个节点池、权重和负载评分选择 A/AAAA 目标。"
              checked={gslbEnabled}
              disabled={!autoSyncEnabled}
              onChange={(checked) => {
                form.setValue('gslb_enabled', checked, { shouldDirty: true });
                if (checked) {
                  form.setValue('dns_auto_target', true, {
                    shouldDirty: true,
                  });
                  form.setValue('dns_schedule_mode', 'load_aware', {
                    shouldDirty: true,
                  });
                }
              }}
            />

            {gslbEnabled ? (
              <>
                <ResourceField
                  label="节点池权重"
                  hint="每行一个节点池：池名 权重 可选国家代码。例：hk 80 HK,TW"
                >
                  <ResourceTextarea
                    className="min-h-28"
                    disabled={!autoSyncEnabled}
                    placeholder={'hk 80 HK,TW\neu 20 DE,FR'}
                    {...form.register('gslb_pools_text')}
                  />
                </ResourceField>

                <div className="grid gap-5 md:grid-cols-2">
                  <ResourceField
                    label="最大连接阈值"
                    hint="0 表示不按连接数剔除节点。"
                  >
                    <ResourceInput
                      type="number"
                      min={0}
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_max_openresty_connections', {
                        valueAsNumber: true,
                      })}
                    />
                  </ResourceField>

                  <ResourceField
                    label="切换冷却时间"
                    hint="旧目标仍健康时，冷却期内不反复切换。"
                  >
                    <ResourceInput
                      type="number"
                      min={1}
                      max={3600}
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_cooldown_seconds', {
                        valueAsNumber: true,
                      })}
                    />
                  </ResourceField>
                </div>
              </>
            ) : null}
          </div>
        ) : null}

        <div className="grid gap-5 md:grid-cols-2">
          <ToggleField
            label="开启 Cloudflare 代理"
            description="开启后 Cloudflare DNS 记录会切到橙云，用于隐藏源站和抗攻击。"
            checked={form.watch('cloudflare_proxied')}
            disabled={!autoSyncEnabled}
            onChange={(checked) =>
              form.setValue('cloudflare_proxied', checked, {
                shouldDirty: true,
              })
            }
          />

          <ResourceField
            label="DDoS 防护模式"
            hint="自动模式会在 5 分钟请求量或错误率超过阈值时打开橙云。"
          >
            <ResourceSelect
              disabled={!autoSyncEnabled}
              {...form.register('ddos_protection_mode')}
            >
              <option value="off">关闭</option>
              <option value="manual">手动</option>
              <option value="auto">自动</option>
            </ResourceSelect>
          </ResourceField>
        </div>

        {route.dns_last_sync_status ? (
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
            <p className="font-medium text-[var(--foreground-primary)]">
              最近同步：
              {route.dns_last_sync_status === 'success' ? '成功' : '失败'}
            </p>
            <p className="mt-1 break-words">{route.dns_last_sync_message}</p>
          </div>
        ) : null}
      </form>
    </ConfigSectionShell>
  );
}

export function CacheSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-cache-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<FeedbackState>();
  const form = useForm<CacheValues>({
    resolver: zodResolver(cacheSchema),
    defaultValues: {
      cache_enabled: route.cache_enabled,
      cache_policy: (route.cache_policy ||
        'url') as CacheValues['cache_policy'],
      cache_rules_text: route.cache_rule_list.join('\n'),
    },
  });

  useEffect(() => {
    form.reset({
      cache_enabled: route.cache_enabled,
      cache_policy: (route.cache_policy ||
        'url') as CacheValues['cache_policy'],
      cache_rules_text: route.cache_rule_list.join('\n'),
    });
  }, [form, route]);

  const watchedEnabled = form.watch('cache_enabled');
  const watchedPolicy = form.watch('cache_policy');
  const purgeMutation = useMutation({
    mutationFn: () => purgeProxyRouteCache(route.id, { scope: 'all' }),
    onSuccess: async (result) => {
      setFeedback({
        tone: 'success',
        message: `已下发缓存清理到 ${result.target_nodes.length} 个节点。`,
      });
      await queryClient.invalidateQueries({
        queryKey: ['proxy-routes', 'detail', route.id],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const warmMutation = useMutation({
    mutationFn: () =>
      warmProxyRouteCache(route.id, {
        scope: 'url',
        urls: route.domains.map(
          (domain) => `${route.enable_https ? 'https' : 'http'}://${domain}/`,
        ),
      }),
    onSuccess: (result) => {
      setFeedback({
        tone: 'success',
        message: `已下发首页预热到 ${result.target_nodes.length} 个节点。`,
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  return (
    <ConfigSectionShell
      title="缓存"
      description="保留现有安全绕过逻辑，只对当前站点生效。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const rules = linesFromTextarea(values.cache_rules_text);
          onSave(
            buildPayloadFromRoute(route, {
              cache_enabled: values.cache_enabled,
              cache_policy: values.cache_enabled ? values.cache_policy : 'url',
              cache_rules:
                values.cache_enabled && values.cache_policy !== 'url'
                  ? rules
                  : [],
            }),
            { message: '缓存设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用站点缓存"
          description="系统仍会自动绕过非 GET、带 Authorization 或常见登录态 Cookie 的请求。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('cache_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField label="缓存策略">
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('cache_policy')}
          >
            <option value="url">按 URL 缓存</option>
            <option value="suffix">按后缀缓存</option>
            <option value="path_prefix">按路径前缀缓存</option>
            <option value="path_exact">按精确路径缓存</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="缓存规则"
          error={form.formState.errors.cache_rules_text?.message}
          hint={
            watchedPolicy === 'suffix'
              ? '每行一个后缀，例如 jpg、css、js。'
              : watchedPolicy === 'path_prefix'
                ? '每行一个路径前缀，例如 /assets、/static。'
                : watchedPolicy === 'path_exact'
                  ? '每行一个精确路径，例如 /robots.txt。'
                  : '按 URL 缓存时无需额外规则。'
          }
        >
          <ResourceTextarea
            disabled={!watchedEnabled || watchedPolicy === 'url'}
            className="min-h-32"
            placeholder={
              watchedPolicy === 'suffix'
                ? 'jpg\ncss\njs'
                : watchedPolicy === 'path_prefix'
                  ? '/assets\n/static'
                  : watchedPolicy === 'path_exact'
                    ? '/robots.txt\n/manifest.json'
                    : '按 URL 缓存时无需额外规则'
            }
            {...form.register('cache_rules_text')}
          />
        </ResourceField>

        <div className="flex flex-wrap gap-3">
          <SecondaryButton
            type="button"
            disabled={purgeMutation.isPending}
            onClick={() => purgeMutation.mutate()}
          >
            {purgeMutation.isPending ? '清理中...' : '清理全部缓存'}
          </SecondaryButton>
          <SecondaryButton
            type="button"
            disabled={warmMutation.isPending || route.domains.length === 0}
            onClick={() => warmMutation.mutate()}
          >
            {warmMutation.isPending ? '预热中...' : '预热站点首页'}
          </SecondaryButton>
        </div>
      </form>
    </ConfigSectionShell>
  );
}

type PowListValues = {
  ips: string;
  ip_cidrs: string;
  paths: string;
  path_regexes: string;
  user_agents: string;
};

const powSchema = z
  .object({
    pow_enabled: z.boolean(),
    difficulty: z.coerce.number().int().min(1).max(16),
    algorithm: z.enum(['fast', 'slow']),
    session_ttl: z.coerce.number().int().min(60),
    challenge_ttl: z.coerce.number().int().min(30),
    whitelist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
      path_regexes: z.string(),
      user_agents: z.string(),
    }),
    blacklist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
      path_regexes: z.string(),
      user_agents: z.string(),
    }),
  })
  .superRefine((value, context) => {
    if (!value.pow_enabled) return;
    const dimensions: { key: string; label: string }[] = [
      { key: 'ips', label: 'IP' },
      { key: 'ip_cidrs', label: 'IP CIDR' },
      { key: 'paths', label: '路径' },
      { key: 'path_regexes', label: '路径正则' },
      { key: 'user_agents', label: 'User-Agent' },
    ];
    for (const dim of dimensions) {
      const wl = linesFromTextarea(
        (value.whitelist as Record<string, string>)[dim.key] || '',
      );
      const bl = linesFromTextarea(
        (value.blacklist as Record<string, string>)[dim.key] || '',
      );
      if (wl.length > 0 && bl.length > 0) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          message: `${dim.label} 不能同时配置白名单和黑名单`,
          path: ['blacklist', dim.key],
        });
      }
    }
  });

type PowValues = z.infer<typeof powSchema>;

type WAFRuleKey =
  | 'sqli'
  | 'xss'
  | 'path_traversal'
  | 'sensitive_paths'
  | 'bad_bots';

const wafBuiltinRules: Array<{
  key: WAFRuleKey;
  label: string;
  description: string;
}> = [
  {
    key: 'sqli',
    label: 'SQL 注入',
    description: '匹配常见 SQL 注入探测片段。',
  },
  { key: 'xss', label: 'XSS', description: '匹配脚本注入和危险事件属性。' },
  {
    key: 'path_traversal',
    label: '路径穿越',
    description: '匹配 ../ 和编码后的目录穿越。',
  },
  {
    key: 'sensitive_paths',
    label: '敏感路径',
    description: '拦截 .env、.git、wp-config 等扫描。',
  },
  {
    key: 'bad_bots',
    label: '恶意工具 UA',
    description: '匹配 sqlmap、nikto、nmap 等扫描器。',
  },
];

const wafSchema = z
  .object({
    waf_enabled: z.boolean(),
    waf_mode: z.enum(['log', 'block']),
    builtin_rules: z.array(
      z.enum(['sqli', 'xss', 'path_traversal', 'sensitive_paths', 'bad_bots']),
    ),
    whitelist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
    }),
    block_rules: z.object({
      path_contains: z.string(),
      path_regexes: z.string(),
      query_contains: z.string(),
      header_contains: z.string(),
      user_agents: z.string(),
    }),
  })
  .superRefine((value, context) => {
    if (!value.waf_enabled) return;

    for (const path of linesFromTextarea(value.whitelist.paths)) {
      if (!path.startsWith('/')) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['whitelist', 'paths'],
          message: `白名单路径必须以 / 开头：${path}`,
        });
        return;
      }
    }

    for (const pattern of linesFromTextarea(value.block_rules.path_regexes)) {
      try {
        new RegExp(pattern);
      } catch {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['block_rules', 'path_regexes'],
          message: `路径正则格式不合法：${pattern}`,
        });
        return;
      }
    }
  });

type WAFValues = z.infer<typeof wafSchema>;

function buildWAFValuesFromRoute(route: ProxyRouteItem): WAFValues {
  const config = route.waf_config;
  return {
    waf_enabled: route.waf_enabled,
    waf_mode: route.waf_mode || 'block',
    builtin_rules: config?.builtin_rules ?? [
      'sqli',
      'xss',
      'path_traversal',
      'sensitive_paths',
      'bad_bots',
    ],
    whitelist: {
      ips: (config?.whitelist?.ips ?? []).join('\n'),
      ip_cidrs: (config?.whitelist?.ip_cidrs ?? []).join('\n'),
      paths: (config?.whitelist?.paths ?? []).join('\n'),
    },
    block_rules: {
      path_contains: (config?.block_rules?.path_contains ?? []).join('\n'),
      path_regexes: (config?.block_rules?.path_regexes ?? []).join('\n'),
      query_contains: (config?.block_rules?.query_contains ?? []).join('\n'),
      header_contains: (config?.block_rules?.header_contains ?? []).join('\n'),
      user_agents: (config?.block_rules?.user_agents ?? []).join('\n'),
    },
  };
}

function buildPowListFromConfig(
  list:
    | {
        ips?: string[];
        ip_cidrs?: string[];
        paths?: string[];
        path_regexes?: string[];
        user_agents?: string[];
      }
    | undefined,
): PowListValues {
  return {
    ips: (list?.ips ?? []).join('\n'),
    ip_cidrs: (list?.ip_cidrs ?? []).join('\n'),
    paths: (list?.paths ?? []).join('\n'),
    path_regexes: (list?.path_regexes ?? []).join('\n'),
    user_agents: (list?.user_agents ?? []).join('\n'),
  };
}

export function PowSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-pow-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const powConfig = route.pow_config;
  const form = useForm<PowValues>({
    resolver: zodResolver(powSchema),
    defaultValues: {
      pow_enabled: route.pow_enabled,
      difficulty: powConfig?.difficulty ?? 4,
      algorithm: powConfig?.algorithm ?? 'fast',
      session_ttl: powConfig?.session_ttl ?? 600,
      challenge_ttl: powConfig?.challenge_ttl ?? 300,
      whitelist: buildPowListFromConfig(powConfig?.whitelist),
      blacklist: buildPowListFromConfig(powConfig?.blacklist),
    },
  });

  useEffect(() => {
    form.reset({
      pow_enabled: route.pow_enabled,
      difficulty: powConfig?.difficulty ?? 4,
      algorithm: powConfig?.algorithm ?? 'fast',
      session_ttl: powConfig?.session_ttl ?? 600,
      challenge_ttl: powConfig?.challenge_ttl ?? 300,
      whitelist: buildPowListFromConfig(powConfig?.whitelist),
      blacklist: buildPowListFromConfig(powConfig?.blacklist),
    });
  }, [form, route, powConfig]);

  const watchedEnabled = form.watch('pow_enabled');

  const parseList = (text: string): string[] =>
    linesFromTextarea(text).filter(Boolean);

  return (
    <ConfigSectionShell
      title="PoW 防护"
      description="启用 Proof-of-Work 反爬虫验证。首次访问的浏览器需要完成计算挑战才能继续。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const powConfigPayload = JSON.stringify({
            difficulty: values.difficulty,
            algorithm: values.algorithm,
            session_ttl: values.session_ttl,
            challenge_ttl: values.challenge_ttl,
            whitelist: {
              ips: parseList(values.whitelist.ips),
              ip_cidrs: parseList(values.whitelist.ip_cidrs),
              paths: parseList(values.whitelist.paths),
              path_regexes: parseList(values.whitelist.path_regexes),
              user_agents: parseList(values.whitelist.user_agents),
            },
            blacklist: {
              ips: parseList(values.blacklist.ips),
              ip_cidrs: parseList(values.blacklist.ip_cidrs),
              paths: parseList(values.blacklist.paths),
              path_regexes: parseList(values.blacklist.path_regexes),
              user_agents: parseList(values.blacklist.user_agents),
            },
          });
          onSave(
            buildPayloadFromRoute(route, {
              pow_enabled: values.pow_enabled,
              pow_config: powConfigPayload,
            }),
            { message: 'PoW 防护设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用 PoW 防护"
          description="对访问此站点的请求进行 Proof-of-Work 验证，阻止自动化爬虫。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('pow_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField label="验证算法">
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('algorithm')}
          >
            <option value="fast">Fast（WebCrypto SHA-256）</option>
            <option value="slow">Slow（兼容模式）</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="难度"
          hint="数值越高验证越慢，1-16。推荐 3-5。"
          error={form.formState.errors.difficulty?.message}
        >
          <ResourceInput
            type="number"
            min={1}
            max={16}
            disabled={!watchedEnabled}
            {...form.register('difficulty')}
          />
        </ResourceField>

        <ResourceField
          label="会话空闲有效期（秒）"
          hint="通过验证后，若在此时间内没有新请求，Cookie 会失效；每次访问会自动续期。默认 600 秒。"
          error={form.formState.errors.session_ttl?.message}
        >
          <ResourceInput
            type="number"
            min={60}
            disabled={!watchedEnabled}
            {...form.register('session_ttl')}
          />
        </ResourceField>

        <ResourceField
          label="挑战有效期（秒）"
          hint="挑战令牌的有效期。"
          error={form.formState.errors.challenge_ttl?.message}
        >
          <ResourceInput
            type="number"
            min={30}
            disabled={!watchedEnabled}
            {...form.register('challenge_ttl')}
          />
        </ResourceField>

        <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
          <fieldset disabled={!watchedEnabled} className="space-y-4">
            <legend className="mb-2 text-sm font-medium text-[var(--foreground-primary)]">
              白名单（匹配的请求跳过 PoW）
            </legend>
            <ResourceField label="IP" hint="每行一个 IP 地址">
              <ResourceTextarea
                className="min-h-20"
                placeholder="1.2.3.4&#10;5.6.7.8"
                {...form.register('whitelist.ips')}
              />
            </ResourceField>
            <ResourceField label="IP CIDR" hint="每行一个 CIDR 范围">
              <ResourceTextarea
                className="min-h-20"
                placeholder="10.0.0.0/8&#10;192.168.0.0/16"
                {...form.register('whitelist.ip_cidrs')}
              />
            </ResourceField>
            <ResourceField label="路径" hint="每行一个路径通配符">
              <ResourceTextarea
                className="min-h-20"
                placeholder="/.well-known/*&#10;/favicon.ico"
                {...form.register('whitelist.paths')}
              />
            </ResourceField>
            <ResourceField label="路径正则" hint="每行一个正则表达式">
              <ResourceTextarea
                className="min-h-20"
                placeholder="^/api/public/"
                {...form.register('whitelist.path_regexes')}
              />
            </ResourceField>
            <ResourceField label="User-Agent" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="Googlebot&#10;bingbot"
                {...form.register('whitelist.user_agents')}
              />
            </ResourceField>
          </fieldset>

          <fieldset disabled={!watchedEnabled} className="space-y-4">
            <legend className="mb-2 text-sm font-medium text-[var(--foreground-primary)]">
              黑名单（匹配的请求必须 PoW）
            </legend>
            <ResourceField label="IP" hint="每行一个 IP 地址">
              <ResourceTextarea
                className="min-h-20"
                placeholder="1.2.3.4"
                {...form.register('blacklist.ips')}
              />
            </ResourceField>
            <ResourceField label="IP CIDR" hint="每行一个 CIDR 范围">
              <ResourceTextarea
                className="min-h-20"
                placeholder="10.0.0.0/8"
                {...form.register('blacklist.ip_cidrs')}
              />
            </ResourceField>
            <ResourceField label="路径" hint="每行一个路径通配符">
              <ResourceTextarea
                className="min-h-20"
                placeholder="/admin/*"
                {...form.register('blacklist.paths')}
              />
            </ResourceField>
            <ResourceField label="路径正则" hint="每行一个正则表达式">
              <ResourceTextarea
                className="min-h-20"
                placeholder="^/private/"
                {...form.register('blacklist.path_regexes')}
              />
            </ResourceField>
            <ResourceField label="User-Agent" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="bot&#10;crawler"
                {...form.register('blacklist.user_agents')}
              />
            </ResourceField>
          </fieldset>
        </div>
        {form.formState.errors.blacklist && (
          <p className="text-sm text-[var(--color-danger)]">
            {Object.values(form.formState.errors.blacklist)
              .flatMap((e) =>
                e && typeof e === 'object' && 'message' in e
                  ? [e.message as string]
                  : [],
              )
              .join('; ')}
          </p>
        )}
      </form>
    </ConfigSectionShell>
  );
}

export function WAFSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-waf-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<WAFValues>({
    resolver: zodResolver(wafSchema),
    defaultValues: buildWAFValuesFromRoute(route),
  });

  useEffect(() => {
    form.reset(buildWAFValuesFromRoute(route));
  }, [form, route]);

  const watchedEnabled = form.watch('waf_enabled');
  const watchedMode = form.watch('waf_mode');
  const watchedBuiltinRules = form.watch('builtin_rules');
  const toggleBuiltinRule = (rule: WAFRuleKey, checked: boolean) => {
    const current = new Set(form.getValues('builtin_rules'));
    if (checked) {
      current.add(rule);
    } else {
      current.delete(rule);
    }
    form.setValue('builtin_rules', Array.from(current) as WAFRuleKey[], {
      shouldDirty: true,
    });
  };

  return (
    <ConfigSectionShell
      title="WAF 防护"
      description="启用节点本地轻量 WAF，在地区限制之后、PoW 之前检查恶意请求。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const wafConfigPayload = JSON.stringify({
            builtin_rules: values.builtin_rules,
            whitelist: {
              ips: linesFromTextarea(values.whitelist.ips),
              ip_cidrs: linesFromTextarea(values.whitelist.ip_cidrs),
              paths: linesFromTextarea(values.whitelist.paths),
            },
            block_rules: {
              path_contains: linesFromTextarea(
                values.block_rules.path_contains,
              ),
              path_regexes: linesFromTextarea(values.block_rules.path_regexes),
              query_contains: linesFromTextarea(
                values.block_rules.query_contains,
              ),
              header_contains: linesFromTextarea(
                values.block_rules.header_contains,
              ),
              user_agents: linesFromTextarea(values.block_rules.user_agents),
            },
          });
          onSave(
            buildPayloadFromRoute(route, {
              waf_enabled: values.waf_enabled,
              waf_mode: values.waf_mode,
              waf_config: wafConfigPayload,
            }),
            { message: 'WAF 防护设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用 WAF 防护"
          description="默认关闭。开启并发布配置后，节点会在本地检查常见攻击和自定义规则。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('waf_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="运行模式"
          hint={
            watchedMode === 'log'
              ? '只记录命中规则并继续放行，适合先观察误杀。'
              : '命中规则后直接返回 403。'
          }
        >
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('waf_mode')}
          >
            <option value="block">拦截模式</option>
            <option value="log">观察模式</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="内置规则"
          hint="建议先使用观察模式确认误杀情况，再切换为拦截模式。"
          container="div"
        >
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {wafBuiltinRules.map((rule) => (
              <label
                key={rule.key}
                className="flex min-h-20 items-start gap-3 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3"
              >
                <input
                  type="checkbox"
                  disabled={!watchedEnabled}
                  checked={watchedBuiltinRules.includes(rule.key)}
                  onChange={(event) =>
                    toggleBuiltinRule(rule.key, event.target.checked)
                  }
                  className="mt-1 h-4 w-4 rounded border-[var(--border-default)] accent-[var(--brand-primary)]"
                />
                <span>
                  <span className="block text-sm font-medium text-[var(--foreground-primary)]">
                    {rule.label}
                  </span>
                  <span className="mt-1 block text-xs leading-5 text-[var(--foreground-secondary)]">
                    {rule.description}
                  </span>
                </span>
              </label>
            ))}
          </div>
        </ResourceField>

        <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
          <fieldset disabled={!watchedEnabled} className="space-y-4">
            <legend className="mb-2 text-sm font-medium text-[var(--foreground-primary)]">
              白名单（匹配后跳过 WAF）
            </legend>
            <ResourceField label="IP" hint="每行一个 IP 地址">
              <ResourceTextarea
                className="min-h-20"
                placeholder="1.2.3.4"
                {...form.register('whitelist.ips')}
              />
            </ResourceField>
            <ResourceField label="IP CIDR" hint="每行一个 CIDR 范围">
              <ResourceTextarea
                className="min-h-20"
                placeholder="10.0.0.0/8"
                {...form.register('whitelist.ip_cidrs')}
              />
            </ResourceField>
            <ResourceField
              label="路径"
              hint="每行一个路径，支持以 * 结尾的前缀匹配。"
              error={form.formState.errors.whitelist?.paths?.message}
            >
              <ResourceTextarea
                className="min-h-20"
                placeholder="/api/public/*"
                {...form.register('whitelist.paths')}
              />
            </ResourceField>
          </fieldset>

          <fieldset disabled={!watchedEnabled} className="space-y-4">
            <legend className="mb-2 text-sm font-medium text-[var(--foreground-primary)]">
              自定义拦截规则
            </legend>
            <ResourceField label="路径包含" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="/debug"
                {...form.register('block_rules.path_contains')}
              />
            </ResourceField>
            <ResourceField
              label="路径正则"
              hint="每行一个正则表达式。"
              error={form.formState.errors.block_rules?.path_regexes?.message}
            >
              <ResourceTextarea
                className="min-h-20"
                placeholder="^/private/"
                {...form.register('block_rules.path_regexes')}
              />
            </ResourceField>
            <ResourceField label="查询参数包含" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="debug=true"
                {...form.register('block_rules.query_contains')}
              />
            </ResourceField>
            <ResourceField label="请求头包含" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="X-Scanner"
                {...form.register('block_rules.header_contains')}
              />
            </ResourceField>
            <ResourceField label="User-Agent 包含" hint="每行一个关键字">
              <ResourceTextarea
                className="min-h-20"
                placeholder="sqlmap"
                {...form.register('block_rules.user_agents')}
              />
            </ResourceField>
          </fieldset>
        </div>
      </form>
    </ConfigSectionShell>
  );
}

export function RegionRestrictionSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-region-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<RegionRestrictionValues>({
    resolver: zodResolver(regionRestrictionSchema),
    defaultValues: {
      region_restriction_enabled: route.region_restriction_enabled,
      region_restriction_mode: route.region_restriction_mode || 'block',
      region_restriction_countries_text:
        route.region_restriction_countries.join('\n'),
    },
  });

  useEffect(() => {
    form.reset({
      region_restriction_enabled: route.region_restriction_enabled,
      region_restriction_mode: route.region_restriction_mode || 'block',
      region_restriction_countries_text:
        route.region_restriction_countries.join('\n'),
    });
  }, [form, route]);

  const watchedEnabled = form.watch('region_restriction_enabled');
  const watchedMode = form.watch('region_restriction_mode');

  return (
    <ConfigSectionShell
      title="地区限制"
      description="基于节点本地 GeoIP 数据库按国家或地区代码放行或拦截访问。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const countries = parseRegionCountriesText(
            values.region_restriction_countries_text,
          );
          onSave(
            buildPayloadFromRoute(route, {
              region_restriction_enabled: values.region_restriction_enabled,
              region_restriction_mode: values.region_restriction_mode,
              region_restriction_countries: values.region_restriction_enabled
                ? countries
                : [],
            }),
            { message: '地区限制已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用地区限制"
          description="开启后，发布配置并由 Agent 应用后才会在节点侧生效。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('region_restriction_enabled', checked, {
              shouldDirty: true,
            })
          }
        />

        <ResourceField
          label="限制模式"
          hint={
            watchedMode === 'allow'
              ? '只允许列表内地区访问；无法识别地区的请求也会被拒绝。'
              : '拒绝列表内地区访问；无法识别地区的请求会继续放行。'
          }
        >
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('region_restriction_mode')}
          >
            <option value="block">拦截列表内地区</option>
            <option value="allow">只允许列表内地区</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="国家/地区代码"
          hint="每行或用逗号分隔一个 ISO 3166-1 两位代码，例如 CN、US、HK。"
          error={
            form.formState.errors.region_restriction_countries_text?.message
          }
        >
          <ResourceTextarea
            disabled={!watchedEnabled}
            className="min-h-36"
            placeholder={'CN\nUS\nHK'}
            {...form.register('region_restriction_countries_text')}
          />
        </ResourceField>

        <div className="rounded-2xl border border-[var(--status-info-border)] bg-[var(--status-info-soft)] px-4 py-3 text-sm leading-6 text-[var(--status-info-foreground)]">
          该功能由 Agent 节点本地 GeoIP 数据库识别真实客户端
          IP，前置反代需要正确透传 CF-Connecting-IP、X-Real-IP 或
          X-Forwarded-For。
        </div>
      </form>
    </ConfigSectionShell>
  );
}

const basicAuthSchema = z
  .object({
    basic_auth_enabled: z.boolean(),
    basic_auth_username: z.string(),
    basic_auth_password: z.string(),
  })
  .superRefine((value, context) => {
    if (value.basic_auth_enabled) {
      if (!value.basic_auth_username.trim()) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['basic_auth_username'],
          message: '请输入账号',
        });
      }
      if (!value.basic_auth_password.trim()) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['basic_auth_password'],
          message: '请输入密码',
        });
      }
    }
  });

type BasicAuthValues = z.infer<typeof basicAuthSchema>;

export function BasicAuthSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-auth-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<BasicAuthValues>({
    resolver: zodResolver(basicAuthSchema),
    defaultValues: {
      basic_auth_enabled: route.basic_auth_enabled,
      basic_auth_username: route.basic_auth_username || '',
      basic_auth_password: route.basic_auth_password || '',
    },
  });

  useEffect(() => {
    form.reset({
      basic_auth_enabled: route.basic_auth_enabled,
      basic_auth_username: route.basic_auth_username || '',
      basic_auth_password: route.basic_auth_password || '',
    });
  }, [form, route]);

  const watchedEnabled = form.watch('basic_auth_enabled');

  return (
    <ConfigSectionShell
      title="认证配置"
      description="配置基础鉴权访问，需要输入账号密码才能访问网站。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          onSave(
            buildPayloadFromRoute(route, {
              basic_auth_enabled: values.basic_auth_enabled,
              basic_auth_username: values.basic_auth_username.trim(),
              basic_auth_password: values.basic_auth_password.trim(),
            }),
            { message: '认证配置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用 Basic Auth 鉴权"
          description="拦截所有请求，需要输入正确的账号和密码才能访问。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('basic_auth_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="账号"
          error={form.formState.errors.basic_auth_username?.message}
        >
          <ResourceInput
            disabled={!watchedEnabled}
            placeholder="admin"
            {...form.register('basic_auth_username')}
          />
        </ResourceField>

        <ResourceField
          label="密码"
          error={form.formState.errors.basic_auth_password?.message}
        >
          <ResourceInput
            disabled={!watchedEnabled}
            type="text"
            placeholder="secret123"
            {...form.register('basic_auth_password')}
          />
        </ResourceField>
      </form>
    </ConfigSectionShell>
  );
}

export function ProxyRouteConfigPage({
  routeId,
  initialSection,
}: {
  routeId: string;
  initialSection?: string;
}) {
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<FeedbackState>();

  const numericRouteID = Number(routeId);
  const currentSection = getWebsiteConfigSection(initialSection);

  const routeQuery = useQuery({
    queryKey: ['proxy-routes', 'detail', numericRouteID],
    queryFn: () => getProxyRoute(numericRouteID),
    enabled: Number.isFinite(numericRouteID) && numericRouteID > 0,
  });
  const certificatesQuery = useQuery({
    queryKey: ['tls-certificates', 'list'],
    queryFn: getTlsCertificates,
  });
  const dnsAccountsQuery = useQuery({
    queryKey: ['dns-accounts'],
    queryFn: getDnsAccounts,
  });
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: getNodes,
    enabled: currentSection === 'proxy',
  });
  const managedDomainsQuery = useQuery({
    queryKey: ['managed-domains'],
    queryFn: getManagedDomains,
  });

  const saveMutation = useMutation({
    mutationFn: async ({
      payload,
      context,
    }: {
      payload: Parameters<typeof updateProxyRoute>[1];
      context: SaveContext;
    }) => {
      const updatedRoute = await updateProxyRoute(numericRouteID, payload);
      return { updatedRoute, context };
    },
    onSuccess: async ({ updatedRoute, context }) => {
      queryClient.setQueryData(
        ['proxy-routes', 'detail', numericRouteID],
        updatedRoute,
      );
      setFeedback({ tone: 'success', message: context.message });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['proxy-routes'] }),
        queryClient.invalidateQueries({
          queryKey: ['config-versions', 'diff'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const route = routeQuery.data;
  const certificates = useMemo(
    () => certificatesQuery.data ?? [],
    [certificatesQuery.data],
  );
  const dnsAccounts = useMemo(
    () => dnsAccountsQuery.data ?? [],
    [dnsAccountsQuery.data],
  );
  const domainSuggestionSources = useMemo(
    () => [
      ...(route?.domains ?? []),
      ...(managedDomainsQuery.data?.map((item) => item.domain) ?? []),
    ],
    [managedDomainsQuery.data, route?.domains],
  );
  const nodePoolOptions = useMemo(
    () => buildNodePoolOptions(nodesQuery.data ?? [], route?.node_pool),
    [nodesQuery.data, route?.node_pool],
  );

  if (!Number.isFinite(numericRouteID) || numericRouteID <= 0) {
    return (
      <EmptyState
        title="缺少站点 ID"
        description="请从站点列表进入配置页面。"
      />
    );
  }

  if (
    routeQuery.isLoading ||
    certificatesQuery.isLoading ||
    dnsAccountsQuery.isLoading
  ) {
    return <LoadingState />;
  }

  if (routeQuery.isError) {
    return (
      <ErrorState
        title="站点详情加载失败"
        description={getErrorMessage(routeQuery.error)}
      />
    );
  }

  if (certificatesQuery.isError) {
    return (
      <ErrorState
        title="证书列表加载失败"
        description={getErrorMessage(certificatesQuery.error)}
      />
    );
  }

  if (dnsAccountsQuery.isError) {
    return (
      <ErrorState
        title="DNS 账号列表加载失败"
        description={getErrorMessage(dnsAccountsQuery.error)}
      />
    );
  }

  if (currentSection === 'proxy' && nodesQuery.isError) {
    return (
      <ErrorState
        title="节点列表加载失败"
        description={getErrorMessage(nodesQuery.error)}
      />
    );
  }

  if (!route) {
    return (
      <EmptyState
        title="站点不存在"
        description="该站点可能已被删除，或当前 ID 无法匹配到记录。"
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={route.site_name}
        description={`主域名 ${route.primary_domain}，共 ${route.domain_count} 个域名`}
        action={
          <div className="flex flex-wrap gap-3">
            <Link
              href="/proxy-route"
              className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
            >
              返回列表
            </Link>
            <SecondaryButton
              type="button"
              onClick={() =>
                queryClient.invalidateQueries({
                  queryKey: ['proxy-routes', 'detail', numericRouteID],
                })
              }
            >
              刷新详情
            </SecondaryButton>
          </div>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="space-y-4">
          <AppCard title="配置分区">
            <div className="space-y-2">
              {websiteConfigSections.map((section) => {
                const active = section.key === currentSection;
                return (
                  <Link
                    key={section.key}
                    href={`/proxy-route/detail?id=${route.id}&section=${section.key}`}
                    className={cn(
                      'block rounded-2xl border px-4 py-3 transition',
                      active
                        ? 'border-[var(--border-strong)] bg-[var(--accent-soft)]'
                        : 'border-[var(--border-default)] bg-[var(--surface-elevated)] hover:border-[var(--border-strong)]',
                    )}
                  >
                    <p className="text-sm font-medium text-[var(--foreground-primary)]">
                      {section.label}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                      {section.description}
                    </p>
                  </Link>
                );
              })}
            </div>
          </AppCard>
        </aside>

        <div className="min-w-0 space-y-6">
          {currentSection === 'domains' ? (
            <DomainSettingsSection
              route={route}
              certificates={certificates}
              saving={saveMutation.isPending}
              suggestionSources={domainSuggestionSources}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'limits' ? (
            <RateLimitSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'proxy' ? (
            <ReverseProxySection
              route={route}
              saving={saveMutation.isPending}
              nodePoolOptions={nodePoolOptions}
              nodePoolsLoading={nodesQuery.isLoading}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'dns' ? (
            <DNSAutomationSection
              route={route}
              dnsAccounts={dnsAccounts}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'cache' ? (
            <CacheSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'pow' ? (
            <PowSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'waf' ? (
            <WAFSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'region' ? (
            <RegionRestrictionSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'auth' ? (
            <BasicAuthSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}
