'use client';

import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Minus, Plus } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { LoadingState } from '@/components/feedback/loading-state';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { getDNSZones } from '@/features/authoritative-dns/api/authoritative-dns';
import type { DNSZoneItem } from '@/features/authoritative-dns/types';
import { getDnsAccounts } from '@/features/dns-accounts/api/dns-accounts';
import type { DnsAccountItem } from '@/features/dns-accounts/types';
import { getManagedDomains } from '@/features/managed-domains/api/managed-domains';
import { getNodes } from '@/features/nodes/api/nodes';
import type { NodeItem } from '@/features/nodes/types';
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
  formatNodeName,
  getNodesForPool,
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
  dns_provider_mode: 'cloudflare' | 'authoritative';
  dns_auto_sync: boolean;
  dns_account_id: string;
  dns_zone_id_ref: string;
  dns_zone_id: string;
  dns_record_type: 'A' | 'AAAA' | 'CNAME';
  dns_record_name: string;
  dns_record_content: string;
  dns_auto_target: boolean;
  dns_target_count: number;
  dns_schedule_mode: 'healthy' | 'weighted' | 'load_aware';
  dns_ttl: number;
  gslb_enabled: boolean;
  gslb_pool_rows: GSLBPoolRow[];
  gslb_max_openresty_connections: number;
  gslb_max_cpu_percent: number;
  gslb_max_memory_percent: number;
  gslb_cooldown_seconds: number;
  cloudflare_proxied: boolean;
  ddos_protection_mode: 'off' | 'auto';
  ddos_protection_provider: 'cloudflare' | 'custom';
  ddos_protection_target: string;
};

type GSLBPoolRow = {
  id: string;
  name: string;
  weight: string;
  countries: string;
  sourceCidrs: string;
};

const dnsTTLHint =
  '0 或 1 表示自动缓存时间；2-29 秒会在保存时提升到 30 秒；30 秒及以上按填写值同步，最高 86400 秒。';
const dnsScheduleModeHints: Record<
  DNSAutomationValues['dns_schedule_mode'],
  string
> = {
  healthy:
    '健康优先只看节点是否在线、代理服务是否正常、是否允许调度；旧目标仍可用且处于冷却期时会保持不动。',
  weighted:
    '权重优先会先排除不可用节点，再按节点池权重和节点池内权重选择。',
  load_aware:
    '按压力优先会参考连接数和主机压力，并可按阈值跳过压力过高的节点。',
};
const autoDNSNodePoolHint =
  '默认节点池用于未开启多节点智能解析时自动选 IP，也用于缓存清理、预热、攻击防护回退和运行时兜底。开启多节点智能解析后，用户访问会返回下方节点池权重里的 IP，不再由这里决定。';
const autoDNSRecordContentHint =
  '固定 IP 时可用逗号、空格或换行填写多个地址。开启自动选择或多节点智能解析后，由系统从节点公网 IP 池生成。';
const ddosProtectionModeHint =
  '关闭时不做自动防护；自动时最近 5 分钟请求量或错误率超过阈值后暂停多节点智能解析，并临时切到所选防护目标，指标恢复后回到正常调度。';
const gslbPoolActionButtonClassName = 'h-11 w-11 shrink-0 rounded-2xl px-0';
const gslbPoolRemoveButtonClassName =
  'border-[var(--border-default)] bg-[var(--surface-elevated)] text-[var(--foreground-secondary)] hover:border-[var(--status-danger-border)] hover:bg-[var(--status-danger-soft)] hover:text-[var(--status-danger-foreground)] disabled:border-[var(--border-default)] disabled:bg-[var(--surface-muted)] disabled:text-[var(--foreground-muted)]';
const gslbPoolAddButtonClassName =
  'border-dashed border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--brand-primary)] hover:bg-[var(--brand-primary-soft)] hover:text-[var(--brand-primary)]';

let gslbPoolRowSequence = 0;

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

function createGSLBPoolRow(
  values: Partial<Omit<GSLBPoolRow, 'id'>> = {},
): GSLBPoolRow {
  gslbPoolRowSequence += 1;
  return {
    id: `gslb-pool-row-${gslbPoolRowSequence}`,
    name: '',
    weight: '100',
    countries: '',
    sourceCidrs: '',
    ...values,
  };
}

function ensureGSLBPoolRows(rows: GSLBPoolRow[]) {
  return rows.length > 0 ? rows : [createGSLBPoolRow()];
}

function syncGSLBPoolRowsWithOptions(
  rows: GSLBPoolRow[],
  options: string[],
): GSLBPoolRow[] {
  const currentRowsByName = new Map<string, GSLBPoolRow>();
  for (const row of rows) {
    const name = row.name.trim();
    if (name) {
      currentRowsByName.set(name.toLowerCase(), row);
    }
  }

  const syncedRows = options
    .map((option) => option.trim())
    .filter(Boolean)
    .map((name) => {
      const existingRow = currentRowsByName.get(name.toLowerCase());
      return createGSLBPoolRow({
        name,
        weight: existingRow?.weight || '100',
        countries: existingRow?.countries || '',
        sourceCidrs: existingRow?.sourceCidrs || '',
      });
    });

  return ensureGSLBPoolRows(syncedRows);
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

function buildGSLBPoolRows(route: ProxyRouteItem) {
  const pools =
    route.gslb_policy?.pools?.length > 0
      ? route.gslb_policy.pools
      : buildDefaultGSLBPolicy(route.node_pool || 'default').pools;
  return ensureGSLBPoolRows(
    pools
      .filter((pool) => pool.enabled !== false)
      .map((pool) =>
        createGSLBPoolRow({
          name: pool.name,
          weight: String(pool.weight || 100),
          countries: pool.countries?.join(',') || '',
          sourceCidrs: pool.source_cidrs?.join('\n') || '',
        }),
      ),
  );
}

function parseGSLBPoolRows(rows: GSLBPoolRow[]) {
  const pools = rows.map((row) => {
    const weight = Number(row.weight);
    return {
      name: row.name.trim(),
      weight: Number.isFinite(weight) && weight > 0 ? weight : 100,
      countries: row.countries
        .split(/[\s,，;；]+/)
        .map((item) => item.trim().toUpperCase())
        .filter((item) => /^[A-Z0-9]{2}$/.test(item)),
      source_cidrs: row.sourceCidrs
        .split(/[\s,，;；]+/)
        .map((item) => item.trim())
        .filter(Boolean),
      enabled: true,
    };
  });
  return pools.filter((pool) => pool.name !== '');
}

function findUnknownGSLBPoolNames(rows: GSLBPoolRow[], options: string[]) {
  const knownPools = new Set(
    options.map((option) => option.trim().toLowerCase()).filter(Boolean),
  );
  const unknownPools = new Set<string>();
  for (const row of rows) {
    const name = row.name.trim();
    if (!name) {
      continue;
    }
    if (!knownPools.has(name.toLowerCase())) {
      unknownPools.add(name);
    }
  }
  return Array.from(unknownPools);
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
  nodes = [],
  nodePoolsLoading,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  nodePoolOptions: string[];
  nodes?: NodeItem[];
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

  const selectedNodePool = form.watch('node_pool');
  const normalizedSelectedNodePool = selectedNodePool.trim() || 'default';
  const selectedNodePoolUnknown =
    normalizedSelectedNodePool !== '' &&
    !nodePoolOptions.includes(normalizedSelectedNodePool);
  const nodesInSelectedPool = useMemo(
    () => getNodesForPool(nodes, normalizedSelectedNodePool),
    [nodes, normalizedSelectedNodePool],
  );
  const [selectedNodeID, setSelectedNodeID] = useState('');

  useEffect(() => {
    setSelectedNodeID((current) => {
      if (nodesInSelectedPool.some((node) => node.node_id === current)) {
        return current;
      }
      return nodesInSelectedPool[0]?.node_id ?? '';
    });
  }, [nodesInSelectedPool]);

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

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(260px,420px)]">
          <ResourceField
            label="默认节点池"
            hint={autoDNSNodePoolHint}
            error={form.formState.errors.node_pool?.message}
            container="div"
          >
            <ResourceSelect
              name="node_pool"
              aria-label="节点池选择"
              value={normalizedSelectedNodePool}
              disabled={nodePoolsLoading}
              onChange={(event) =>
                form.setValue('node_pool', event.target.value, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            >
              {selectedNodePoolUnknown ? (
                <option value={normalizedSelectedNodePool}>
                  {normalizedSelectedNodePool}（未找到）
                </option>
              ) : null}
              {nodePoolOptions.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </ResourceSelect>
            {selectedNodePoolUnknown ? (
              <p className="mt-2 text-xs text-[var(--status-warning-foreground)]">
                当前节点池不在现有节点池列表里，请从下拉选择真实存在的节点池。
              </p>
            ) : null}
          </ResourceField>

          <ResourceField
            label="池内节点"
            hint={
              nodesInSelectedPool.length > 0
                ? '根据左侧节点池实时同步，只用于确认该池真实节点。'
                : '当前节点池里还没有真实节点。'
            }
            container="div"
          >
            <ResourceSelect
              aria-label="池内节点"
              value={selectedNodeID}
              disabled={nodePoolsLoading || nodesInSelectedPool.length === 0}
              onChange={(event) => setSelectedNodeID(event.target.value)}
            >
              {nodesInSelectedPool.length === 0 ? (
                <option value="">暂无节点</option>
              ) : (
                nodesInSelectedPool.map((node) => (
                  <option key={node.node_id || node.id} value={node.node_id}>
                    {formatNodeName(node)}
                  </option>
                ))
              )}
            </ResourceSelect>
          </ResourceField>
        </div>

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
  dnsZones = [],
  dnsZonesLoading = false,
  nodePoolOptions = [],
  nodePoolsLoading = false,
  saving,
  onSave,
  formId = 'proxy-route-dns-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  dnsAccounts: DnsAccountItem[];
  dnsZones?: DNSZoneItem[];
  dnsZonesLoading?: boolean;
  nodePoolOptions?: string[];
  nodePoolsLoading?: boolean;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<DNSAutomationValues>({
    defaultValues: {
      dns_provider_mode: route.dns_provider_mode || 'cloudflare',
      dns_auto_sync: route.dns_auto_sync,
      dns_account_id: route.dns_account_id ? String(route.dns_account_id) : '',
      dns_zone_id_ref: route.dns_zone_id_ref
        ? String(route.dns_zone_id_ref)
        : '',
      dns_zone_id: route.dns_zone_id || '',
      dns_record_type: route.dns_record_type || 'A',
      dns_record_name: route.dns_record_name || '',
      dns_record_content: route.dns_record_content || '',
      dns_auto_target: route.dns_auto_target,
      dns_target_count: route.dns_target_count || 1,
      dns_schedule_mode: route.dns_schedule_mode || 'healthy',
      dns_ttl: route.dns_ttl || 1,
      gslb_enabled: route.gslb_enabled,
      gslb_pool_rows: buildGSLBPoolRows(route),
      gslb_max_openresty_connections:
        route.gslb_policy?.load_thresholds?.max_openresty_connections || 0,
      gslb_max_cpu_percent:
        route.gslb_policy?.load_thresholds?.max_cpu_percent || 0,
      gslb_max_memory_percent:
        route.gslb_policy?.load_thresholds?.max_memory_percent || 0,
      gslb_cooldown_seconds:
        route.gslb_policy?.debounce?.cooldown_seconds || 60,
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
      ddos_protection_provider:
        route.ddos_protection_provider || 'cloudflare',
      ddos_protection_target:
        route.ddos_protection_target ||
        (route.dns_account_id ? String(route.dns_account_id) : ''),
    },
  });

  useEffect(() => {
    form.reset({
      dns_provider_mode: route.dns_provider_mode || 'cloudflare',
      dns_auto_sync: route.dns_auto_sync,
      dns_account_id: route.dns_account_id ? String(route.dns_account_id) : '',
      dns_zone_id_ref: route.dns_zone_id_ref
        ? String(route.dns_zone_id_ref)
        : '',
      dns_zone_id: route.dns_zone_id || '',
      dns_record_type: route.dns_record_type || 'A',
      dns_record_name: route.dns_record_name || '',
      dns_record_content: route.dns_record_content || '',
      dns_auto_target: route.dns_auto_target,
      dns_target_count: route.dns_target_count || 1,
      dns_schedule_mode: route.dns_schedule_mode || 'healthy',
      dns_ttl: route.dns_ttl || 1,
      gslb_enabled: route.gslb_enabled,
      gslb_pool_rows: buildGSLBPoolRows(route),
      gslb_max_openresty_connections:
        route.gslb_policy?.load_thresholds?.max_openresty_connections || 0,
      gslb_max_cpu_percent:
        route.gslb_policy?.load_thresholds?.max_cpu_percent || 0,
      gslb_max_memory_percent:
        route.gslb_policy?.load_thresholds?.max_memory_percent || 0,
      gslb_cooldown_seconds:
        route.gslb_policy?.debounce?.cooldown_seconds || 60,
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
      ddos_protection_provider:
        route.ddos_protection_provider || 'cloudflare',
      ddos_protection_target:
        route.ddos_protection_target ||
        (route.dns_account_id ? String(route.dns_account_id) : ''),
    });
  }, [form, route]);

  const providerMode = form.watch('dns_provider_mode');
  const isAuthoritativeMode = providerMode === 'authoritative';
  const autoSyncEnabled = isAuthoritativeMode || form.watch('dns_auto_sync');
  const recordType = form.watch('dns_record_type');
  const autoTarget = form.watch('dns_auto_target');
  const gslbEnabled = form.watch('gslb_enabled');
  const dnsScheduleMode = form.watch('dns_schedule_mode');
  const ddosProtectionMode = form.watch('ddos_protection_mode');
  const ddosProtectionProvider = form.watch('ddos_protection_provider');
  const ddosAutoEnabled = ddosProtectionMode === 'auto' && !isAuthoritativeMode;
  const ddosControlsEnabled = autoSyncEnabled && ddosAutoEnabled;
  const cloudflareAccounts = useMemo(
    () => dnsAccounts.filter((account) => account.type === 'cloudflare'),
    [dnsAccounts],
  );
  const ddosTargetOptions = useMemo(
    () =>
      buildNodePoolOptions(
        nodePoolOptions.map((poolName) => ({ pool_name: poolName })),
      ),
    [nodePoolOptions],
  );
  const gslbPoolOptions = useMemo(
    () =>
      buildNodePoolOptions(
        nodePoolOptions.map((poolName) => ({ pool_name: poolName })),
      ),
    [nodePoolOptions],
  );

  return (
    <ConfigSectionShell
      title="负载均衡"
      description="配置域名解析、自动选 IP 和多节点智能解析；开启多节点智能解析后，返回 IP 由这里的节点池权重决定。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const dnsAccountID = Number(values.dns_account_id);
          const dnsZoneIDRef = Number(values.dns_zone_id_ref);
          const baseGSLBPolicy =
            route.gslb_policy || buildDefaultGSLBPolicy(route.node_pool);
          const gslbPools = parseGSLBPoolRows(values.gslb_pool_rows);
          const unknownGSLBPools = values.gslb_enabled
            ? findUnknownGSLBPoolNames(values.gslb_pool_rows, gslbPoolOptions)
            : [];
          const authoritativeMode = values.dns_provider_mode === 'authoritative';
          const ddosAuto =
            !authoritativeMode && values.ddos_protection_mode === 'auto';
          const ddosTarget =
            ddosAuto && values.ddos_protection_provider === 'cloudflare'
              ? values.ddos_protection_target ||
                values.dns_account_id ||
                (route.dns_account_id ? String(route.dns_account_id) : '')
              : ddosAuto && values.ddos_protection_provider === 'custom'
                ? values.ddos_protection_target.trim()
                : '';
          if (
            ddosAuto &&
            values.ddos_protection_provider === 'custom' &&
            ddosTarget === ''
          ) {
            form.setError('ddos_protection_target', {
              type: 'manual',
              message: '请选择清洗池',
            });
            return;
          }
          if (unknownGSLBPools.length > 0) {
            form.setError('gslb_pool_rows', {
              type: 'manual',
              message: `节点池不存在：${unknownGSLBPools.join('、')}。请从已有节点池下拉选择。`,
            });
            return;
          }
          if (values.gslb_enabled && gslbPools.length === 0) {
            form.setError('gslb_pool_rows', {
              type: 'manual',
              message: '请至少选择一个用于多节点智能解析的节点池。',
            });
            return;
          }
          onSave(
            buildPayloadFromRoute(route, {
              dns_provider_mode: values.dns_provider_mode,
              dns_zone_id_ref:
                authoritativeMode &&
                Number.isFinite(dnsZoneIDRef) &&
                dnsZoneIDRef > 0
                  ? dnsZoneIDRef
                  : null,
              dns_auto_sync: authoritativeMode ? false : values.dns_auto_sync,
              dns_account_id:
                !authoritativeMode &&
                values.dns_auto_sync &&
                Number.isFinite(dnsAccountID) &&
                dnsAccountID > 0
                  ? dnsAccountID
                  : null,
              dns_zone_id: authoritativeMode ? '' : values.dns_zone_id.trim(),
              dns_record_type: values.dns_record_type,
              dns_record_name: values.dns_record_name.trim(),
              dns_record_content: authoritativeMode
                ? ''
                : values.dns_record_content.trim(),
              dns_auto_target: authoritativeMode || values.gslb_enabled
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
                  max_cpu_percent: values.gslb_max_cpu_percent,
                  max_memory_percent: values.gslb_max_memory_percent,
                },
                debounce: {
                  ...baseGSLBPolicy.debounce,
                  cooldown_seconds: values.gslb_cooldown_seconds,
                },
              },
              cloudflare_proxied: authoritativeMode
                ? false
                : values.cloudflare_proxied,
              ddos_protection_mode: authoritativeMode
                ? 'off'
                : values.ddos_protection_mode,
              ddos_protection_provider: authoritativeMode
                ? 'cloudflare'
                : values.ddos_protection_provider,
              ddos_protection_target: authoritativeMode ? '' : ddosTarget,
            }),
            {
              message: authoritativeMode
                ? '本地自建解析设置已保存。'
                : '负载均衡设置已保存。',
            },
          );
        })}
      >
        <ResourceField
          label="解析模式"
          hint="Cloudflare 模式会后台同步解析记录；本地自建解析会交给 DNS 响应端，在用户查询时实时选择 IP。"
          tooltip="DNS 是把域名解析成服务器 IP 的服务。选择 Cloudflare 时，系统把记录同步到 Cloudflare；选择本地自建解析时，需要把域名 NS 指向你的 DNS 响应端。"
        >
          <ResourceSelect
            aria-label="解析模式"
            {...form.register('dns_provider_mode', {
              onChange: (event) => {
                const mode = event.target
                  .value as DNSAutomationValues['dns_provider_mode'];
                if (mode === 'authoritative') {
                  form.setValue('dns_auto_sync', false, { shouldDirty: true });
                  form.setValue('dns_auto_target', true, { shouldDirty: true });
                  form.setValue('cloudflare_proxied', false, {
                    shouldDirty: true,
                  });
                  form.setValue('ddos_protection_mode', 'off', {
                    shouldDirty: true,
                  });
                  form.setValue('ddos_protection_provider', 'cloudflare', {
                    shouldDirty: true,
                  });
                  form.setValue('ddos_protection_target', '', {
                    shouldDirty: true,
                  });
                }
              },
            })}
          >
            <option value="cloudflare">Cloudflare 同步</option>
            <option value="authoritative">本地自建解析</option>
          </ResourceSelect>
        </ResourceField>

        {isAuthoritativeMode ? (
          <ResourceField
            label="托管域名"
            hint="网站域名必须属于所选托管域名；可在左侧「本地自建解析」页面创建。"
            tooltip="托管域名一般是根域名，例如 example.com。www.example.com、api.example.com 都应选择 example.com。"
            error={
              !form.watch('dns_zone_id_ref') ? '请选择托管域名' : undefined
            }
          >
            <ResourceSelect
              aria-label="托管域名"
              disabled={dnsZonesLoading}
              {...form.register('dns_zone_id_ref')}
            >
              <option value="">
                {dnsZonesLoading ? '正在加载托管域名...' : '请选择托管域名'}
              </option>
              {dnsZones.map((zone) => (
                <option key={zone.id} value={zone.id}>
                  {zone.name}
                  {zone.enabled ? '' : '（已停用）'}
                </option>
              ))}
            </ResourceSelect>
          </ResourceField>
        ) : (
          <ToggleField
            label="启用 Cloudflare 自动解析"
            description="开启后会为当前网站域名创建或更新 Cloudflare 解析记录。"
            checked={autoSyncEnabled}
            onChange={(checked) => {
              form.setValue('dns_auto_sync', checked, { shouldDirty: true });
              if (!checked) {
                form.setValue('ddos_protection_mode', 'off', {
                  shouldDirty: true,
                });
                form.setValue('ddos_protection_target', '', {
                  shouldDirty: true,
                });
              }
            }}
          />
        )}

        {!isAuthoritativeMode ? (
          <div className="grid gap-5 md:grid-cols-2">
            <ResourceField
              label="Cloudflare 账号"
              hint="API 密钥需要允许读取域名并修改解析记录。"
              tooltip="Cloudflare 里对应的权限名通常是 Zone Read 和 DNS Edit。"
              error={
                autoSyncEnabled && !form.watch('dns_account_id')
                  ? '启用自动解析时请选择 Cloudflare 账号'
                  : undefined
              }
            >
              <ResourceSelect
                disabled={!autoSyncEnabled}
                {...form.register('dns_account_id', {
                  onChange: (event) => {
                    if (
                      form.getValues('ddos_protection_provider') ===
                        'cloudflare' &&
                      !form.getValues('ddos_protection_target')
                    ) {
                      form.setValue(
                        'ddos_protection_target',
                        event.target.value,
                        { shouldDirty: true },
                      );
                    }
                  },
                })}
              >
                <option value="">请选择 Cloudflare 账号</option>
                {cloudflareAccounts.map((account) => (
                  <option key={account.id} value={account.id}>
                    {account.name}
                  </option>
                ))}
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="记录类型"
              hint="默认 IPv4 地址。自动选择节点时只支持 IPv4 或 IPv6 地址。"
              tooltip="A 表示 IPv4 地址，AAAA 表示 IPv6 地址，CNAME 表示别名记录，也就是把一个域名指向另一个域名。"
            >
              <ResourceSelect
                disabled={!autoSyncEnabled}
                {...form.register('dns_record_type')}
              >
                <option value="A">IPv4 地址（A）</option>
                <option value="AAAA">IPv6 地址（AAAA）</option>
                <option value="CNAME">别名记录（CNAME）</option>
              </ResourceSelect>
            </ResourceField>
          </div>
        ) : (
          <ResourceField
            label="动态记录类型"
            hint="本地自建解析的自动选 IP 只支持 IPv4 或 IPv6 地址。"
            tooltip="A 表示 IPv4 地址，AAAA 表示 IPv6 地址。"
          >
            <ResourceSelect {...form.register('dns_record_type')}>
              <option value="A">IPv4 地址（A）</option>
              <option value="AAAA">IPv6 地址（AAAA）</option>
            </ResourceSelect>
          </ResourceField>
        )}

        {!isAuthoritativeMode ? (
          <div className="grid gap-5 md:grid-cols-2">
            <ResourceField
              label="Cloudflare 域名编号"
              hint="可留空，系统会按主域名自动查找 Cloudflare 里的域名。"
              tooltip="这是 Cloudflare 里每个域名区域的 ID。新手可以留空，系统会按域名自动查找。"
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
        ) : (
          <ResourceField
            label="记录名称"
            hint="可留空，默认使用当前网站的全部域名。"
          >
            <ResourceInput
              placeholder={route.primary_domain}
              {...form.register('dns_record_name')}
            />
          </ResourceField>
        )}

        {!isAuthoritativeMode ? (
          <ResourceField
            label="记录内容"
            hint={
              recordType === 'CNAME'
                ? '别名记录必须手动填写目标域名。'
                : autoDNSRecordContentHint
            }
          >
            <ResourceTextarea
              disabled={!autoSyncEnabled || autoTarget}
              placeholder={
                recordType === 'CNAME'
                  ? 'target.example.com'
                  : '留空自动选择，或每行填写一个 IP'
              }
              {...form.register('dns_record_content')}
            />
          </ResourceField>
        ) : null}

        {!isAuthoritativeMode ? (
          <ToggleField
            label="自动选择在线节点 IP"
            description="开启后节点离线会自动切换到其他在线节点；手动填写的 IP 不会被后台任务覆盖。"
            checked={autoTarget}
            disabled={!autoSyncEnabled || recordType === 'CNAME'}
            onChange={(checked) =>
              form.setValue('dns_auto_target', checked, { shouldDirty: true })
            }
          />
        ) : null}

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
              hint={dnsScheduleModeHints[dnsScheduleMode]}
            >
              <ResourceSelect
                disabled={!autoSyncEnabled || !autoTarget}
                {...form.register('dns_schedule_mode')}
              >
                <option value="healthy">健康优先（冷却防抖）</option>
                <option value="weighted">按权重优先</option>
                <option value="load_aware">按压力优先</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="解析缓存时间"
              hint={dnsTTLHint}
              tooltip="也叫 TTL，决定用户本地或运营商 DNS 多久刷新一次记录。时间短切换更快，查询量会更高。"
            >
              <ResourceInput
                type="number"
                min={0}
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
              label="启用多节点智能解析"
              description="开启后返回哪些 IP 由下方节点池权重决定；反向代理里的默认节点池只保留缓存、回退和兜底用途。"
              tooltip="这类能力也常叫 GSLB。它会按访问来源、节点健康和权重从多个节点池里选择 IP。关闭时才从默认节点池选。"
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
                  label={
                    <span className="flex flex-wrap items-center gap-3">
                      <span>节点池权重</span>
                      <SecondaryButton
                        type="button"
                        className="rounded-full px-3 py-1.5 text-xs"
                        disabled={!autoSyncEnabled || nodePoolsLoading}
                        onClick={() => {
                          form.setValue(
                            'gslb_pool_rows',
                            syncGSLBPoolRowsWithOptions(
                              form.getValues('gslb_pool_rows') ?? [],
                              gslbPoolOptions,
                            ),
                            {
                              shouldDirty: true,
                              shouldValidate: true,
                            },
                          );
                          form.clearErrors('gslb_pool_rows');
                        }}
                      >
                        同步现有节点池
                      </SecondaryButton>
                    </span>
                  }
                  hint="请选择节点详情里真实存在的节点池；不要填写节点名称。国家或地区填写两位代码，留空表示全局兜底。"
                  tooltip="来源网段用于把某些用户 IP 段固定到指定节点池，适合有明确区域或线路规划的场景。节点池名要和节点详情里的节点池完全一致。"
                  error={form.formState.errors.gslb_pool_rows?.message}
                  container="div"
                >
                  <Controller
                    control={form.control}
                    name="gslb_pool_rows"
                    render={({ field }) => {
                      const rows = ensureGSLBPoolRows(field.value ?? []);
                      const updateRows = (nextRows: GSLBPoolRow[]) => {
                        field.onChange(ensureGSLBPoolRows(nextRows));
                      };

                      return (
                        <div className="space-y-3">
                          <div className="hidden gap-3 pl-[56px] text-xs font-medium text-[var(--foreground-secondary)] md:grid md:grid-cols-[minmax(0,1fr)_110px_minmax(0,0.9fr)_minmax(0,1.2fr)]">
                            <span>池名</span>
                            <span>权重</span>
                            <span>国家或地区</span>
                            <span>来源网段</span>
                          </div>

                          {rows.map((row, index) => (
                            <div
                              key={row.id}
                              className="grid gap-3 md:grid-cols-[44px_minmax(0,1fr)_110px_minmax(0,0.9fr)_minmax(0,1.2fr)] md:items-start"
                            >
                              <SecondaryButton
                                type="button"
                                aria-label={`删除节点池 ${index + 1}`}
                                className={`${gslbPoolActionButtonClassName} ${gslbPoolRemoveButtonClassName}`}
                                disabled={!autoSyncEnabled || rows.length === 1}
                                onClick={() => {
                                  if (rows.length === 1) {
                                    updateRows([createGSLBPoolRow()]);
                                    return;
                                  }

                                  updateRows(
                                    rows.filter(
                                      (_, rowIndex) => rowIndex !== index,
                                    ),
                                  );
                                }}
                              >
                                <Minus
                                  aria-hidden="true"
                                  className="h-[14px] w-[14px]"
                                />
                              </SecondaryButton>

                              <NodePoolSelect
                                value={row.name}
                                options={gslbPoolOptions}
                                compact
                                name={`gslb_pool_${index + 1}`}
                                selectAriaLabel={`节点池选择 ${index + 1}`}
                                inputAriaLabel={`节点池名称 ${index + 1}`}
                                disabled={!autoSyncEnabled}
                                onChange={(value) => {
                                  const nextRows = rows.slice();
                                  nextRows[index] = {
                                    ...row,
                                    name: value,
                                  };
                                  updateRows(nextRows);
                                }}
                              />

                              <ResourceInput
                                type="number"
                                min={1}
                                max={1000}
                                value={row.weight}
                                aria-label={`节点池权重 ${index + 1}`}
                                placeholder="100"
                                disabled={!autoSyncEnabled}
                                onChange={(event) => {
                                  const nextRows = rows.slice();
                                  nextRows[index] = {
                                    ...row,
                                    weight: event.target.value,
                                  };
                                  updateRows(nextRows);
                                }}
                                className="h-11"
                              />

                              <ResourceInput
                                value={row.countries}
                                aria-label={`节点池国家或地区 ${index + 1}`}
                                placeholder="HK,TW"
                                disabled={!autoSyncEnabled}
                                onChange={(event) => {
                                  const nextRows = rows.slice();
                                  nextRows[index] = {
                                    ...row,
                                    countries: event.target.value,
                                  };
                                  updateRows(nextRows);
                                }}
                                className="h-11"
                              />

                              <ResourceInput
                                value={row.sourceCidrs}
                                aria-label={`节点池来源网段 ${index + 1}`}
                                placeholder="203.0.113.0/24"
                                disabled={!autoSyncEnabled}
                                onChange={(event) => {
                                  const nextRows = rows.slice();
                                  nextRows[index] = {
                                    ...row,
                                    sourceCidrs: event.target.value,
                                  };
                                  updateRows(nextRows);
                                }}
                                className="h-11"
                              />
                            </div>
                          ))}

                          <SecondaryButton
                            type="button"
                            aria-label="新增节点池"
                            className={`${gslbPoolActionButtonClassName} ${gslbPoolAddButtonClassName}`}
                            disabled={!autoSyncEnabled}
                            onClick={() => {
                              updateRows([...rows, createGSLBPoolRow()]);
                            }}
                          >
                            <Plus
                              aria-hidden="true"
                              className="h-[14px] w-[14px]"
                            />
                          </SecondaryButton>
                        </div>
                      );
                    }}
                  />
                </ResourceField>

                <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-4">
                  <ResourceField
                    label="最大连接数"
                    hint="0 表示不按连接数跳过节点。"
                    tooltip="这里对应代理服务当前连接数，超过后会尽量避开该节点。"
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
                    label="最大处理器压力"
                    hint="0 表示不按处理器压力跳过节点。"
                    tooltip="这里对应节点上报的 CPU 使用率，超过后会尽量避开该节点。"
                  >
                    <ResourceInput
                      type="number"
                      min={0}
                      max={100}
                      step={1}
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_max_cpu_percent', {
                        valueAsNumber: true,
                      })}
                    />
                  </ResourceField>

                  <ResourceField
                    label="最大内存压力"
                    hint="0 表示不按内存使用率跳过节点。"
                    tooltip="这里对应节点上报的内存使用率，超过后会尽量避开该节点。"
                  >
                    <ResourceInput
                      type="number"
                      min={0}
                      max={100}
                      step={1}
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_max_memory_percent', {
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

        {!isAuthoritativeMode ? (
          <div className="space-y-5 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
            <ToggleField
              label="平时也开启 Cloudflare 代理"
              description="开启后正常状态下也会同步橙云。攻击自动防护只在攻击期间临时覆盖解析目标，恢复后回到这里的平时设置。"
              tooltip="Cloudflare 代理就是常说的小黄云或橙云，请求会先经过 Cloudflare 再到你的节点。"
              checked={form.watch('cloudflare_proxied')}
              disabled={!autoSyncEnabled}
              onChange={(checked) =>
                form.setValue('cloudflare_proxied', checked, {
                  shouldDirty: true,
                })
              }
            />

            <div className="grid gap-5 md:grid-cols-3">
              <ResourceField
                label="攻击防护模式"
                hint={ddosProtectionModeHint}
                tooltip="系统会根据最近请求量和错误率判断是否疑似攻击。触发后先保护可用性，恢复正常后再回到原解析策略。"
              >
                <ResourceSelect
                  aria-label="攻击防护模式"
                  disabled={!autoSyncEnabled}
                  {...form.register('ddos_protection_mode', {
                    onChange: (event) => {
                      const mode = event.target
                        .value as DNSAutomationValues['ddos_protection_mode'];
                      if (mode === 'off') {
                        form.setValue('ddos_protection_target', '', {
                          shouldDirty: true,
                        });
                      } else if (!form.getValues('ddos_protection_target')) {
                        form.setValue(
                          'ddos_protection_target',
                          form.getValues('dns_account_id') ||
                            (route.dns_account_id
                              ? String(route.dns_account_id)
                              : ''),
                          { shouldDirty: true },
                        );
                      }
                    },
                  })}
                >
                  <option value="off">关闭</option>
                  <option value="auto">自动</option>
                </ResourceSelect>
              </ResourceField>

              <ResourceField
                label="防护提供方"
                hint="Cloudflare 会在攻击期同步橙云；自定义会把解析目标切到指定清洗池。攻击期都会暂停多节点智能解析。"
              >
                <ResourceSelect
                  aria-label="防护提供方"
                  disabled={!ddosControlsEnabled}
                  {...form.register('ddos_protection_provider', {
                    onChange: (event) => {
                      const provider = event.target
                        .value as DNSAutomationValues['ddos_protection_provider'];
                      form.setValue(
                        'ddos_protection_target',
                        provider === 'cloudflare'
                          ? form.getValues('dns_account_id') ||
                              (route.dns_account_id
                                ? String(route.dns_account_id)
                                : '')
                          : route.ddos_protection_target || route.node_pool,
                        { shouldDirty: true },
                      );
                    },
                  })}
                >
                  <option value="cloudflare">Cloudflare</option>
                  <option value="custom">自定义清洗池</option>
                </ResourceSelect>
              </ResourceField>

              <ResourceField
                label={
                  ddosProtectionProvider === 'custom'
                    ? '清洗池'
                    : 'Cloudflare 账号'
                }
                hint={
                  ddosProtectionProvider === 'custom'
                    ? '攻击期只返回该池内在线且可调度的公网 IP；恢复正常后回到原解析策略。'
                    : '攻击期使用该账号同步记录并开启橙云；留空时使用上方自动解析账号。'
                }
                tooltip={
                  ddosProtectionProvider === 'custom'
                    ? '清洗池可以是你提前准备的抗 D 节点池或公网 IP 池，用来在攻击期临时承接流量。'
                    : undefined
                }
                error={form.formState.errors.ddos_protection_target?.message}
              >
                {ddosProtectionProvider === 'custom' ? (
                  <ResourceSelect
                    aria-label="清洗池"
                    disabled={!ddosControlsEnabled || nodePoolsLoading}
                    {...form.register('ddos_protection_target')}
                  >
                    <option value="">
                      {nodePoolsLoading ? '正在加载节点池...' : '请选择清洗池'}
                    </option>
                    {ddosTargetOptions.map((poolName) => (
                      <option key={poolName} value={poolName}>
                        {poolName}
                      </option>
                    ))}
                  </ResourceSelect>
                ) : (
                  <ResourceSelect
                    aria-label="攻击防护 Cloudflare 账号"
                    disabled={!ddosControlsEnabled}
                    {...form.register('ddos_protection_target')}
                  >
                    <option value="">使用上方账号</option>
                    {cloudflareAccounts.map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.name}
                      </option>
                    ))}
                  </ResourceSelect>
                )}
              </ResourceField>
            </div>
          </div>
        ) : null}

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

type PowListGroup = 'whitelist' | 'blacklist';
type PowListField = keyof PowListValues;

type PowRuleKey = `${PowListGroup}.${PowListField}`;

type PowRuleOption = {
  key: PowRuleKey;
  group: PowListGroup;
  field: PowListField;
  label: string;
  hint: string;
  placeholder: string;
  description: string;
};

const powRuleOptions: PowRuleOption[] = [
  {
    key: 'whitelist.ips',
    group: 'whitelist',
    field: 'ips',
    label: '白名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.ip_cidrs',
    group: 'whitelist',
    field: 'ip_cidrs',
    label: '白名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.paths',
    group: 'whitelist',
    field: 'paths',
    label: '白名单路径',
    hint: '每行一个路径通配符',
    placeholder: '/.well-known/*\n/favicon.ico',
    description: '匹配这些路径的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.path_regexes',
    group: 'whitelist',
    field: 'path_regexes',
    label: '白名单路径正则',
    hint: '每行一个正则表达式',
    placeholder: '^/api/public/',
    description: '路径命中这些正则时跳过计算验证。',
  },
  {
    key: 'whitelist.user_agents',
    group: 'whitelist',
    field: 'user_agents',
    label: '白名单 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'Googlebot\nbingbot',
    description: 'User-Agent 包含这些关键字时跳过计算验证。',
  },
  {
    key: 'blacklist.ips',
    group: 'blacklist',
    field: 'ips',
    label: '黑名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.ip_cidrs',
    group: 'blacklist',
    field: 'ip_cidrs',
    label: '黑名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.paths',
    group: 'blacklist',
    field: 'paths',
    label: '黑名单路径',
    hint: '每行一个路径通配符',
    placeholder: '/admin/*',
    description: '匹配这些路径的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.path_regexes',
    group: 'blacklist',
    field: 'path_regexes',
    label: '黑名单路径正则',
    hint: '每行一个正则表达式',
    placeholder: '^/private/',
    description: '路径命中这些正则时必须完成计算验证。',
  },
  {
    key: 'blacklist.user_agents',
    group: 'blacklist',
    field: 'user_agents',
    label: '黑名单 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'bot\ncrawler',
    description: 'User-Agent 包含这些关键字时必须完成计算验证。',
  },
];

const powRuleOptionMap = new Map(
  powRuleOptions.map((option) => [option.key, option]),
);

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

type WAFWhitelistField = keyof WAFValues['whitelist'];
type WAFBlockRuleField = keyof WAFValues['block_rules'];
type WAFCustomRuleKey =
  | `whitelist.${WAFWhitelistField}`
  | `block_rules.${WAFBlockRuleField}`;

type WAFCustomRuleOption =
  | {
      key: `whitelist.${WAFWhitelistField}`;
      group: 'whitelist';
      field: WAFWhitelistField;
      label: string;
      hint: string;
      placeholder: string;
      description: string;
    }
  | {
      key: `block_rules.${WAFBlockRuleField}`;
      group: 'block_rules';
      field: WAFBlockRuleField;
      label: string;
      hint: string;
      placeholder: string;
      description: string;
    };

const wafCustomRuleOptions: WAFCustomRuleOption[] = [
  {
    key: 'whitelist.ips',
    group: 'whitelist',
    field: 'ips',
    label: '白名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求会跳过恶意请求防护。',
  },
  {
    key: 'whitelist.ip_cidrs',
    group: 'whitelist',
    field: 'ip_cidrs',
    label: '白名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求会跳过恶意请求防护。',
  },
  {
    key: 'whitelist.paths',
    group: 'whitelist',
    field: 'paths',
    label: '白名单路径',
    hint: '每行一个路径，支持以 * 结尾的前缀匹配。',
    placeholder: '/api/public/*',
    description: '匹配这些路径的请求会跳过恶意请求防护。',
  },
  {
    key: 'block_rules.path_contains',
    group: 'block_rules',
    field: 'path_contains',
    label: '拦截路径包含',
    hint: '每行一个关键字',
    placeholder: '/debug',
    description: '请求路径包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.path_regexes',
    group: 'block_rules',
    field: 'path_regexes',
    label: '拦截路径正则',
    hint: '每行一个正则表达式。',
    placeholder: '^/private/',
    description: '请求路径命中这些正则时命中恶意请求防护。',
  },
  {
    key: 'block_rules.query_contains',
    group: 'block_rules',
    field: 'query_contains',
    label: '拦截查询参数包含',
    hint: '每行一个关键字',
    placeholder: 'debug=true',
    description: '查询参数包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.header_contains',
    group: 'block_rules',
    field: 'header_contains',
    label: '拦截请求头包含',
    hint: '每行一个关键字',
    placeholder: 'X-Scanner',
    description: '请求头包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.user_agents',
    group: 'block_rules',
    field: 'user_agents',
    label: '拦截 User-Agent 包含',
    hint: '每行一个关键字',
    placeholder: 'sqlmap',
    description: 'User-Agent 包含这些关键字时命中恶意请求防护。',
  },
];

const wafCustomRuleOptionMap = new Map(
  wafCustomRuleOptions.map((option) => [option.key, option]),
);

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

function buildPowValuesFromRoute(route: ProxyRouteItem): PowValues {
  const config = route.pow_config;
  return {
    pow_enabled: route.pow_enabled,
    difficulty: config?.difficulty ?? 4,
    algorithm: config?.algorithm ?? 'fast',
    session_ttl: config?.session_ttl ?? 600,
    challenge_ttl: config?.challenge_ttl ?? 300,
    whitelist: buildPowListFromConfig(config?.whitelist),
    blacklist: buildPowListFromConfig(config?.blacklist),
  };
}

function buildInitialPowRuleKeys(values: Pick<PowValues, 'whitelist' | 'blacklist'>) {
  return powRuleOptions
    .filter((option) => values[option.group][option.field].trim() !== '')
    .map((option) => option.key);
}

function buildInitialWAFCustomRuleKeys(values: Pick<WAFValues, 'whitelist' | 'block_rules'>) {
  return wafCustomRuleOptions
    .filter((option) =>
      option.group === 'whitelist'
        ? values.whitelist[option.field].trim() !== ''
        : values.block_rules[option.field].trim() !== '',
    )
    .map((option) => option.key);
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
  const form = useForm<PowValues>({
    resolver: zodResolver(powSchema),
    defaultValues: buildPowValuesFromRoute(route),
  });
  const [selectedPowRuleKey, setSelectedPowRuleKey] = useState<PowRuleKey>(
    powRuleOptions[0].key,
  );
  const [activePowRuleKeys, setActivePowRuleKeys] = useState<PowRuleKey[]>(() =>
    buildInitialPowRuleKeys(buildPowValuesFromRoute(route)),
  );

  useEffect(() => {
    const nextValues = buildPowValuesFromRoute(route);
    form.reset(nextValues);
    setActivePowRuleKeys(buildInitialPowRuleKeys(nextValues));
  }, [form, route]);

  const watchedEnabled = form.watch('pow_enabled');
  const addPowRule = () => {
    setActivePowRuleKeys((current) =>
      current.includes(selectedPowRuleKey)
        ? current
        : [...current, selectedPowRuleKey],
    );
  };
  const removePowRule = (option: PowRuleOption) => {
    form.setValue(option.key, '', { shouldDirty: true, shouldValidate: true });
    setActivePowRuleKeys((current) =>
      current.filter((key) => key !== option.key),
    );
  };

  const parseList = (text: string): string[] =>
    linesFromTextarea(text).filter(Boolean);

  const visiblePowRuleOptions = activePowRuleKeys
    .map((key) => powRuleOptionMap.get(key))
    .filter((option): option is PowRuleOption => Boolean(option));

  return (
    <ConfigSectionShell
      title="计算验证防护"
      description="首次访问的浏览器需要完成一次轻量计算后才能继续，用来拦住自动化爬虫。"
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
            { message: '计算验证防护设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用计算验证防护"
          description="对访问此站点的请求进行浏览器计算验证，阻止自动化爬虫。"
          tooltip="这类能力也常叫 PoW 或 Proof-of-Work。正常浏览器通常能自动完成，自动化请求会被消耗更多成本。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('pow_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField label="验证算法">
          <ResourceSelect
            aria-label="验证算法"
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

        <ResourceField
          label="按需添加规则"
          hint="选择一种规则类型，点击加号后再填写。未添加的规则不会生效。"
          container="div"
        >
          <div className="flex items-center gap-2">
            <ResourceSelect
              aria-label="按需添加规则"
              disabled={!watchedEnabled}
              value={selectedPowRuleKey}
              onChange={(event) =>
                setSelectedPowRuleKey(event.target.value as PowRuleKey)
              }
            >
              {powRuleOptions.map((option) => (
                <option key={option.key} value={option.key}>
                  {option.label}
                </option>
              ))}
            </ResourceSelect>
            <SecondaryButton
              type="button"
              aria-label="添加计算验证规则"
              title="添加规则"
              disabled={
                !watchedEnabled || activePowRuleKeys.includes(selectedPowRuleKey)
              }
              onClick={addPowRule}
              className="h-11 w-11 shrink-0 rounded-lg p-0"
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </SecondaryButton>
          </div>
        </ResourceField>

        {visiblePowRuleOptions.length === 0 ? (
          <p className="rounded-lg border border-dashed border-[var(--border-default)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
            暂未添加额外规则。默认按上方计算验证设置处理请求。
          </p>
        ) : (
          <div className="space-y-3">
            {visiblePowRuleOptions.map((option) => (
              <div
                key={option.key}
                className="rounded-lg border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium text-[var(--foreground-primary)]">
                      {option.label}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                      {option.description}
                    </p>
                  </div>
                  <SecondaryButton
                    type="button"
                    aria-label={`移除${option.label}`}
                    title={`移除${option.label}`}
                    disabled={!watchedEnabled}
                    onClick={() => removePowRule(option)}
                    className="h-9 w-9 shrink-0 rounded-lg p-0"
                  >
                    <Minus className="h-4 w-4" aria-hidden="true" />
                  </SecondaryButton>
                </div>
                <ResourceField
                  label="匹配内容"
                  hint={option.hint}
                  className="mt-3"
                >
                  <ResourceTextarea
                    className="min-h-20"
                    disabled={!watchedEnabled}
                    placeholder={option.placeholder}
                    {...form.register(option.key)}
                  />
                </ResourceField>
              </div>
            ))}
          </div>
        )}
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
  const [selectedWAFCustomRuleKey, setSelectedWAFCustomRuleKey] =
    useState<WAFCustomRuleKey>(wafCustomRuleOptions[0].key);
  const [activeWAFCustomRuleKeys, setActiveWAFCustomRuleKeys] = useState<
    WAFCustomRuleKey[]
  >(() => buildInitialWAFCustomRuleKeys(buildWAFValuesFromRoute(route)));

  useEffect(() => {
    const nextValues = buildWAFValuesFromRoute(route);
    form.reset(nextValues);
    setActiveWAFCustomRuleKeys(buildInitialWAFCustomRuleKeys(nextValues));
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
  const addWAFCustomRule = () => {
    setActiveWAFCustomRuleKeys((current) =>
      current.includes(selectedWAFCustomRuleKey)
        ? current
        : [...current, selectedWAFCustomRuleKey],
    );
  };
  const removeWAFCustomRule = (option: WAFCustomRuleOption) => {
    if (option.group === 'whitelist') {
      form.setValue(`whitelist.${option.field}`, '', {
        shouldDirty: true,
        shouldValidate: true,
      });
    } else {
      form.setValue(`block_rules.${option.field}`, '', {
        shouldDirty: true,
        shouldValidate: true,
      });
    }
    setActiveWAFCustomRuleKeys((current) =>
      current.filter((key) => key !== option.key),
    );
  };
  const visibleWAFCustomRuleOptions = activeWAFCustomRuleKeys
    .map((key) => wafCustomRuleOptionMap.get(key))
    .filter((option): option is WAFCustomRuleOption => Boolean(option));
  const getWAFCustomRuleError = (option: WAFCustomRuleOption) => {
    if (option.group === 'whitelist' && option.field === 'paths') {
      return form.formState.errors.whitelist?.paths?.message;
    }
    if (option.group === 'block_rules' && option.field === 'path_regexes') {
      return form.formState.errors.block_rules?.path_regexes?.message;
    }
    return undefined;
  };

  return (
    <ConfigSectionShell
      title="恶意请求防护"
      description="启用节点本地轻量规则，在地区限制之后、计算验证之前检查恶意请求。"
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
            { message: '恶意请求防护设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用恶意请求防护"
          description="默认关闭。开启并发布配置后，节点会在本地检查常见攻击和自定义规则。"
          tooltip="这类能力也常叫 WAF。这里是节点本地的轻量规则，不依赖第三方防护服务。"
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
            aria-label="运行模式"
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

        <ResourceField
          label="按需添加白名单或拦截规则"
          hint="选择一种规则类型，点击加号后再填写。未添加的规则不会生效。"
          container="div"
        >
          <div className="flex items-center gap-2">
            <ResourceSelect
              aria-label="按需添加白名单或拦截规则"
              disabled={!watchedEnabled}
              value={selectedWAFCustomRuleKey}
              onChange={(event) =>
                setSelectedWAFCustomRuleKey(
                  event.target.value as WAFCustomRuleKey,
                )
              }
            >
              {wafCustomRuleOptions.map((option) => (
                <option key={option.key} value={option.key}>
                  {option.label}
                </option>
              ))}
            </ResourceSelect>
            <SecondaryButton
              type="button"
              aria-label="添加恶意请求防护规则"
              title="添加规则"
              disabled={
                !watchedEnabled ||
                activeWAFCustomRuleKeys.includes(selectedWAFCustomRuleKey)
              }
              onClick={addWAFCustomRule}
              className="h-11 w-11 shrink-0 rounded-lg p-0"
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </SecondaryButton>
          </div>
        </ResourceField>

        {visibleWAFCustomRuleOptions.length === 0 ? (
          <p className="rounded-lg border border-dashed border-[var(--border-default)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
            暂未添加自定义规则。当前仅使用上方内置规则。
          </p>
        ) : (
          <div className="space-y-3">
            {visibleWAFCustomRuleOptions.map((option) => (
              <div
                key={option.key}
                className="rounded-lg border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium text-[var(--foreground-primary)]">
                      {option.label}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                      {option.description}
                    </p>
                  </div>
                  <SecondaryButton
                    type="button"
                    aria-label={`移除${option.label}`}
                    title={`移除${option.label}`}
                    disabled={!watchedEnabled}
                    onClick={() => removeWAFCustomRule(option)}
                    className="h-9 w-9 shrink-0 rounded-lg p-0"
                  >
                    <Minus className="h-4 w-4" aria-hidden="true" />
                  </SecondaryButton>
                </div>
                <ResourceField
                  label="匹配内容"
                  hint={option.hint}
                  error={getWAFCustomRuleError(option)}
                  className="mt-3"
                >
                  <ResourceTextarea
                    className="min-h-20"
                    disabled={!watchedEnabled}
                    placeholder={option.placeholder}
                    {...form.register(option.key)}
                  />
                </ResourceField>
              </div>
            ))}
          </div>
        )}
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
  const dnsZonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
    enabled: currentSection === 'dns',
  });
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: getNodes,
    enabled: currentSection === 'proxy' || currentSection === 'dns',
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
  const dnsZones = useMemo(
    () => dnsZonesQuery.data ?? [],
    [dnsZonesQuery.data],
  );
  const domainSuggestionSources = useMemo(
    () => [
      ...(route?.domains ?? []),
      ...(managedDomainsQuery.data?.map((item) => item.domain) ?? []),
    ],
    [managedDomainsQuery.data, route?.domains],
  );
  const nodePoolOptions = useMemo(
    () => buildNodePoolOptions(nodesQuery.data ?? []),
    [nodesQuery.data],
  );
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

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
        title="Cloudflare 账号列表加载失败"
        description={getErrorMessage(dnsAccountsQuery.error)}
      />
    );
  }

  if (currentSection === 'dns' && dnsZonesQuery.isError) {
    return (
      <ErrorState
        title="托管域名加载失败"
        description={getErrorMessage(dnsZonesQuery.error)}
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
              nodes={nodes}
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
              dnsZones={dnsZones}
              dnsZonesLoading={dnsZonesQuery.isLoading}
              nodePoolOptions={nodePoolOptions}
              nodePoolsLoading={nodesQuery.isLoading}
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
