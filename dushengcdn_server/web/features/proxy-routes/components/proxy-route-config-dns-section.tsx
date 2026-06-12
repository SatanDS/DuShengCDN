'use client';

import { useQuery } from '@tanstack/react-query';
import { Minus, Plus } from 'lucide-react';
import { useEffect, useMemo } from 'react';
import { Controller, useForm } from 'react-hook-form';

import { InlineMessage } from '@/components/feedback/inline-message';
import { getDNSWorkers } from '@/features/authoritative-dns/api/authoritative-dns';
import type {
  DNSWorkerItem,
  DNSZoneItem,
} from '@/features/authoritative-dns/types';
import type { DnsAccountItem } from '@/features/dns-accounts/types';
import type { NodeItem } from '@/features/nodes/types';
import {
  buildNodePoolOptions,
  formatNodeName,
  getNodesForPool,
} from '@/features/proxy-routes/components/node-pool-select';
import {
  buildDefaultGSLBPolicy,
  buildPayloadFromRoute,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  getProxyBufferingMode,
  proxyBufferingModeHint,
  type ConfigSectionPresentationProps,
  type SaveHandler,
} from './proxy-route-config-shared';

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
  gslb_pool_match_mode: 'priority' | 'mixed_weighted';
  gslb_source_pool_fallback_mode: 'strict' | 'fallback_to_global';
  proxy_buffering_mode: 'default' | 'off';
  cloudflare_proxied: boolean;
  ddos_protection_mode: 'off' | 'auto';
  ddos_protection_provider: 'cloudflare' | 'custom';
  ddos_protection_target: string;
};

type GSLBPoolRow = {
  id: string;
  name: string;
  nodeIds: string[];
  weight: string;
  countries: string;
  operators: string[];
  asns: string;
  sourceCidrs: string;
  excludeCountries: string;
  excludeOperators: string[];
  excludeAsns: string;
  excludeSourceCidrs: string;
};

const dnsTTLHint =
  '0 或 1 表示自动缓存时间；2-29 秒会在保存时提升到 30 秒；30 秒及以上按填写值同步，最高 86400 秒。';
const dnsScheduleModeHints: Record<
  DNSAutomationValues['dns_schedule_mode'],
  string
> = {
  healthy:
    '健康优先只看节点是否在线、代理服务是否正常、是否允许调度；旧目标仍可用且处于冷却期时会保持不动。',
  weighted: '权重优先会先排除不可用节点，再按节点池权重和节点池内权重选择。',
  load_aware:
    '按压力优先会参考连接数和主机压力，并可按阈值跳过压力过高的节点。',
};
const autoDNSNodePoolHint =
  '默认节点池用于未开启多节点智能解析时自动选 IP，也用于缓存清理、预热、攻击防护回退和运行时兜底。开启多节点智能解析后，用户访问会返回下方节点池权重里的 IP，不再由这里决定。';
const autoDNSRecordContentHint =
  '固定 IP 时可用逗号、空格或换行填写多个地址。开启自动选择或多节点智能解析后，由系统从节点公网 IP 池生成。';
const ddosProtectionModeHint =
  '关闭时不做自动防护；自动时最近 5 分钟请求量或错误率超过阈值后暂停多节点智能解析，并临时切到所选防护目标，指标恢复后回到正常调度。';
const gslbSourcePoolFallbackHint =
  '严格匹配时，来源命中特定池但池内无可用 IP 会返回空结果；回退到全局时，会继续尝试未配置来源条件的全局节点池。';
const gslbPoolMatchModeHint =
  '专属池优先会保持现有行为，来源命中特定池后只在命中池内选；混合权重会先排除不适用池，再把所有适用池按权重共同分流。';
const gslbPoolActionButtonClassName = 'h-11 w-11 shrink-0 rounded-2xl px-0';
const gslbPoolRemoveButtonClassName =
  'border-[var(--border-default)] bg-[var(--surface-elevated)] text-[var(--foreground-secondary)] hover:border-[var(--status-danger-border)] hover:bg-[var(--status-danger-soft)] hover:text-[var(--status-danger-foreground)] disabled:border-[var(--border-default)] disabled:bg-[var(--surface-muted)] disabled:text-[var(--foreground-muted)]';
const gslbPoolAddButtonClassName =
  'border-dashed border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--brand-primary)] hover:bg-[var(--brand-primary-soft)] hover:text-[var(--brand-primary)]';
const gslbOperatorOptions = [
  { value: 'cn-telecom', label: '电信' },
  { value: 'cn-unicom', label: '联通' },
  { value: 'cn-mobile', label: '移动' },
  { value: 'cn-broadcast', label: '广电' },
  { value: 'cernet', label: '教育网' },
] as const;

let gslbPoolRowSequence = 0;

function getDNSWorkerCapabilitySummary(workers: DNSWorkerItem[]) {
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  const source = onlineWorkers.length > 0 ? onlineWorkers : workers;
  return {
    total: source.length,
    country: source.filter((worker) => worker.geoip_country_enabled).length,
    asn: source.filter((worker) => worker.geoip_asn_enabled).length,
    operator: source.filter((worker) => worker.geoip_operator_enabled).length,
  };
}

function formatDNSWorkerCapabilitySummary(workers: DNSWorkerItem[]) {
  const summary = getDNSWorkerCapabilitySummary(workers);
  if (summary.total === 0) {
    return '尚未检测到 DNS 响应端，运营商/ASN 调度会在响应端部署并上报识别库后生效。';
  }
  return `当前响应端识别库：国家 ${summary.country}/${summary.total}，ASN ${summary.asn}/${summary.total}，运营商 ${summary.operator}/${summary.total}。运营商识别使用 gaoyifan/china-operator-ip CIDR 库；ASN 识别使用 GeoLite2-ASN 或兼容 MMDB。`;
}

function createGSLBPoolRow(
  values: Partial<Omit<GSLBPoolRow, 'id'>> = {},
): GSLBPoolRow {
  gslbPoolRowSequence += 1;
  return {
    id: `gslb-pool-row-${gslbPoolRowSequence}`,
    name: '',
    nodeIds: [],
    weight: '100',
    countries: '',
    operators: [],
    asns: '',
    sourceCidrs: '',
    excludeCountries: '',
    excludeOperators: [],
    excludeAsns: '',
    excludeSourceCidrs: '',
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
        nodeIds: existingRow?.nodeIds ?? [],
        weight: existingRow?.weight || '100',
        countries: existingRow?.countries || '',
        operators: existingRow?.operators ?? [],
        asns: existingRow?.asns || '',
        sourceCidrs: existingRow?.sourceCidrs || '',
        excludeCountries: existingRow?.excludeCountries || '',
        excludeOperators: existingRow?.excludeOperators ?? [],
        excludeAsns: existingRow?.excludeAsns || '',
        excludeSourceCidrs: existingRow?.excludeSourceCidrs || '',
      });
    });

  return ensureGSLBPoolRows(syncedRows);
}

function normalizeGSLBOperatorList(values: string[] | null | undefined) {
  const operators = new Set<string>();
  for (const value of values ?? []) {
    const operator = value.trim().toLowerCase();
    if (operator) {
      operators.add(operator);
    }
  }
  return Array.from(operators);
}

function parseGSLBASNList(value: string) {
  const asns = new Set<number>();
  for (const item of value.split(/[\s,，;；]+/)) {
    const normalized = item
      .trim()
      .replace(/^asn:\s*/i, '')
      .replace(/^as/i, '');
    if (!normalized) {
      continue;
    }
    const asn = Number(normalized);
    if (Number.isInteger(asn) && asn > 0 && asn <= 4294967295) {
      asns.add(asn);
    }
  }
  return Array.from(asns);
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
          nodeIds: pool.node_ids ?? [],
          weight: String(pool.weight || 100),
          countries: pool.countries?.join(',') || '',
          operators: normalizeGSLBOperatorList(pool.operators),
          asns: pool.asns?.join(',') || '',
          sourceCidrs: pool.source_cidrs?.join('\n') || '',
          excludeCountries: pool.exclude_countries?.join(',') || '',
          excludeOperators: normalizeGSLBOperatorList(pool.exclude_operators),
          excludeAsns: pool.exclude_asns?.join(',') || '',
          excludeSourceCidrs: pool.exclude_source_cidrs?.join('\n') || '',
        }),
      ),
  );
}

function parseGSLBPoolRows(
  rows: GSLBPoolRow[],
  matchMode: DNSAutomationValues['gslb_pool_match_mode'],
) {
  const mixedWeighted = matchMode === 'mixed_weighted';
  const pools = rows.map((row) => {
    const weight = Number(row.weight);
    return {
      name: row.name.trim(),
      weight: Number.isFinite(weight) && weight > 0 ? weight : 100,
      countries: mixedWeighted
        ? []
        : row.countries
            .split(/[\s,，;；]+/)
            .map((item) => item.trim().toUpperCase())
            .filter((item) => /^[A-Z0-9]{2}$/.test(item)),
      operators: mixedWeighted ? [] : normalizeGSLBOperatorList(row.operators),
      asns: mixedWeighted ? [] : parseGSLBASNList(row.asns),
      source_cidrs: mixedWeighted
        ? []
        : row.sourceCidrs
            .split(/[\s,，;；]+/)
            .map((item) => item.trim())
            .filter(Boolean),
      exclude_countries: mixedWeighted
        ? row.excludeCountries
            .split(/[\s,，;；]+/)
            .map((item) => item.trim().toUpperCase())
            .filter((item) => /^[A-Z0-9]{2}$/.test(item))
        : [],
      exclude_operators: mixedWeighted
        ? normalizeGSLBOperatorList(row.excludeOperators)
        : [],
      exclude_asns: mixedWeighted ? parseGSLBASNList(row.excludeAsns) : [],
      exclude_source_cidrs: mixedWeighted
        ? row.excludeSourceCidrs
            .split(/[\s,，;；]+/)
            .map((item) => item.trim())
            .filter(Boolean)
        : [],
      node_ids: row.nodeIds.map((item) => item.trim()).filter(Boolean),
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

export function DNSAutomationSection({
  route,
  dnsAccounts,
  dnsZones = [],
  dnsZonesLoading = false,
  nodePoolOptions = [],
  nodes = [],
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
  nodes?: NodeItem[];
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
      gslb_pool_match_mode:
        route.gslb_policy?.pool_match_mode === 'mixed_weighted'
          ? 'mixed_weighted'
          : 'priority',
      gslb_source_pool_fallback_mode:
        route.gslb_policy?.source_pool_fallback_mode === 'fallback_to_global'
          ? 'fallback_to_global'
          : 'strict',
      proxy_buffering_mode: getProxyBufferingMode(route),
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
      ddos_protection_provider: route.ddos_protection_provider || 'cloudflare',
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
      gslb_pool_match_mode:
        route.gslb_policy?.pool_match_mode === 'mixed_weighted'
          ? 'mixed_weighted'
          : 'priority',
      gslb_source_pool_fallback_mode:
        route.gslb_policy?.source_pool_fallback_mode === 'fallback_to_global'
          ? 'fallback_to_global'
          : 'strict',
      proxy_buffering_mode: getProxyBufferingMode(route),
      cloudflare_proxied: route.cloudflare_proxied,
      ddos_protection_mode: route.ddos_protection_mode || 'off',
      ddos_protection_provider: route.ddos_protection_provider || 'cloudflare',
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
  const gslbPoolMatchMode = form.watch('gslb_pool_match_mode');
  const mixedGSLBPoolMatchMode = gslbPoolMatchMode === 'mixed_weighted';
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
  const dnsWorkersQuery = useQuery({
    queryKey: ['authoritative-dns', 'workers', 'source-capabilities'],
    queryFn: getDNSWorkers,
  });

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
          const gslbPools = parseGSLBPoolRows(
            values.gslb_pool_rows,
            values.gslb_pool_match_mode,
          );
          const unknownGSLBPools = values.gslb_enabled
            ? findUnknownGSLBPoolNames(values.gslb_pool_rows, gslbPoolOptions)
            : [];
          const authoritativeMode =
            values.dns_provider_mode === 'authoritative';
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
              dns_auto_target:
                authoritativeMode || values.gslb_enabled
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
                pool_match_mode: values.gslb_pool_match_mode,
                target_count: values.dns_target_count,
                ttl: values.dns_ttl,
                source_pool_fallback_mode:
                  values.gslb_source_pool_fallback_mode,
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
              proxy_buffering_mode: values.proxy_buffering_mode,
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
                {isAuthoritativeMode ? (
                  <InlineMessage
                    tone="info"
                    message={formatDNSWorkerCapabilitySummary(
                      dnsWorkersQuery.data ?? [],
                    )}
                  />
                ) : null}
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
                  hint={
                    mixedGSLBPoolMatchMode
                      ? '请选择节点详情里真实存在的节点池；当前规则列为排除条件，全留空时该节点池参与所有来源的权重分流。'
                      : '请选择节点详情里真实存在的节点池；不要填写节点名称。来源网段、ASN、运营商、国家或地区都留空时作为全局兜底。'
                  }
                  tooltip={
                    mixedGSLBPoolMatchMode
                      ? '混合权重会先剔除命中排除条件的节点池，再把剩余适用池按权重共同分流。运营商/ASN 需要 DNS 响应端配置离线 ISP/ASN 库。'
                      : '来源匹配优先级为：网段、ASN、运营商、国家或地区、全局兜底。运营商/ASN 需要 DNS 响应端配置离线 ISP/ASN 库。'
                  }
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
                          <div className="hidden gap-3 pl-[56px] text-xs font-medium text-[var(--foreground-secondary)] md:grid md:grid-cols-[minmax(220px,1.35fr)_96px_minmax(0,0.75fr)_minmax(0,1fr)_minmax(0,0.85fr)_minmax(0,1fr)]">
                            <span>池名 / 池内节点</span>
                            <span>权重</span>
                            <span>
                              {mixedGSLBPoolMatchMode
                                ? '排除国家或地区'
                                : '国家或地区'}
                            </span>
                            <span>
                              {mixedGSLBPoolMatchMode
                                ? '排除访问运营商'
                                : '访问运营商'}
                            </span>
                            <span>
                              {mixedGSLBPoolMatchMode ? '排除 ASN' : 'ASN'}
                            </span>
                            <span>
                              {mixedGSLBPoolMatchMode
                                ? '排除来源网段'
                                : '来源网段'}
                            </span>
                          </div>

                          {rows.map((row, index) => {
                            const normalizedRowPoolName = row.name.trim();
                            const rowPoolUnknown =
                              normalizedRowPoolName !== '' &&
                              !gslbPoolOptions.includes(normalizedRowPoolName);
                            const rowPoolNodes =
                              normalizedRowPoolName === '' || rowPoolUnknown
                                ? []
                                : getNodesForPool(nodes, normalizedRowPoolName);
                            const rowPoolNodeIDs = rowPoolNodes.map(
                              (node) => node.node_id || String(node.id),
                            );
                            const explicitNodeIDs = row.nodeIds.filter(
                              (nodeID) => rowPoolNodeIDs.includes(nodeID),
                            );
                            const hasExplicitNodeSelection =
                              row.nodeIds.length > 0;
                            const effectiveNodeIDs = hasExplicitNodeSelection
                              ? explicitNodeIDs
                              : rowPoolNodeIDs;
                            const allNodesSelected =
                              rowPoolNodeIDs.length > 0 &&
                              effectiveNodeIDs.length === rowPoolNodeIDs.length;
                            const updateRowNodeIDs = (nodeIDs: string[]) => {
                              const uniqueNodeIDs = Array.from(
                                new Set(
                                  nodeIDs.filter((nodeID) =>
                                    rowPoolNodeIDs.includes(nodeID),
                                  ),
                                ),
                              );
                              if (
                                rowPoolNodeIDs.length > 0 &&
                                uniqueNodeIDs.length === 0
                              ) {
                                return;
                              }
                              const nextRows = rows.slice();
                              nextRows[index] = {
                                ...row,
                                nodeIds:
                                  uniqueNodeIDs.length === rowPoolNodeIDs.length
                                    ? []
                                    : uniqueNodeIDs,
                              };
                              updateRows(nextRows);
                            };

                            return (
                              <div
                                key={row.id}
                                className="grid gap-3 md:grid-cols-[44px_minmax(220px,1.35fr)_96px_minmax(0,0.75fr)_minmax(0,1fr)_minmax(0,0.85fr)_minmax(0,1fr)] md:items-start"
                              >
                                <SecondaryButton
                                  type="button"
                                  aria-label={`删除节点池 ${index + 1}`}
                                  className={`${gslbPoolActionButtonClassName} ${gslbPoolRemoveButtonClassName}`}
                                  disabled={
                                    !autoSyncEnabled || rows.length === 1
                                  }
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

                                <div className="grid gap-2">
                                  <ResourceSelect
                                    name={`gslb_pool_${index + 1}`}
                                    aria-label={`节点池选择 ${index + 1}`}
                                    value={normalizedRowPoolName}
                                    disabled={!autoSyncEnabled}
                                    onChange={(event) => {
                                      const nextRows = rows.slice();
                                      nextRows[index] = {
                                        ...row,
                                        name: event.target.value,
                                        nodeIds: [],
                                      };
                                      updateRows(nextRows);
                                    }}
                                  >
                                    {normalizedRowPoolName === '' ? (
                                      <option value="" disabled>
                                        请选择现有节点池
                                      </option>
                                    ) : null}
                                    {rowPoolUnknown ? (
                                      <option value={normalizedRowPoolName}>
                                        {normalizedRowPoolName}（未找到）
                                      </option>
                                    ) : null}
                                    {gslbPoolOptions.map((option) => (
                                      <option key={option} value={option}>
                                        {option}
                                      </option>
                                    ))}
                                  </ResourceSelect>

                                  <div
                                    aria-label={`节点池内节点 ${index + 1}`}
                                    role="group"
                                    className="rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] p-3"
                                  >
                                    {rowPoolNodes.length === 0 ? (
                                      <p className="text-xs text-[var(--foreground-muted)]">
                                        暂无节点
                                      </p>
                                    ) : (
                                      <div className="space-y-3">
                                        <div className="flex flex-wrap items-center justify-between gap-2">
                                          <span className="text-xs text-[var(--foreground-secondary)]">
                                            {allNodesSelected
                                              ? `已选择全部 ${rowPoolNodes.length} 个节点`
                                              : `已选择 ${effectiveNodeIDs.length} / ${rowPoolNodes.length} 个节点`}
                                          </span>
                                          <div className="flex gap-2">
                                            <button
                                              type="button"
                                              className="text-xs font-medium text-[var(--brand-primary)] transition hover:text-[var(--foreground-primary)] disabled:text-[var(--foreground-muted)]"
                                              disabled={!autoSyncEnabled}
                                              onClick={() =>
                                                updateRowNodeIDs(rowPoolNodeIDs)
                                              }
                                            >
                                              全选
                                            </button>
                                          </div>
                                        </div>
                                        <div className="grid gap-2">
                                          {rowPoolNodes.map((node) => {
                                            const nodeID =
                                              node.node_id || String(node.id);
                                            return (
                                              <label
                                                key={nodeID}
                                                className="flex min-h-9 items-center gap-2 rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-2 text-sm text-[var(--foreground-primary)]"
                                              >
                                                <input
                                                  type="checkbox"
                                                  aria-label={`节点池 ${normalizedRowPoolName} 节点 ${formatNodeName(node)}`}
                                                  checked={effectiveNodeIDs.includes(
                                                    nodeID,
                                                  )}
                                                  disabled={!autoSyncEnabled}
                                                  onChange={(event) => {
                                                    if (event.target.checked) {
                                                      updateRowNodeIDs([
                                                        ...effectiveNodeIDs,
                                                        nodeID,
                                                      ]);
                                                    } else {
                                                      updateRowNodeIDs(
                                                        effectiveNodeIDs.filter(
                                                          (currentNodeID) =>
                                                            currentNodeID !==
                                                            nodeID,
                                                        ),
                                                      );
                                                    }
                                                  }}
                                                  className="h-4 w-4 accent-[var(--brand-primary)]"
                                                />
                                                <span className="min-w-0 truncate">
                                                  {formatNodeName(node)}
                                                </span>
                                              </label>
                                            );
                                          })}
                                        </div>
                                      </div>
                                    )}
                                  </div>

                                  {rowPoolUnknown ? (
                                    <p className="text-xs text-[var(--status-warning-foreground)]">
                                      {`当前填写的“${normalizedRowPoolName}”不在现有节点池里，请从下拉选择或先到边缘节点 / IP 池创建对应节点池。`}
                                    </p>
                                  ) : null}
                                </div>

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
                                  value={
                                    mixedGSLBPoolMatchMode
                                      ? row.excludeCountries
                                      : row.countries
                                  }
                                  aria-label={`节点池${
                                    mixedGSLBPoolMatchMode ? '排除' : ''
                                  }国家或地区 ${index + 1}`}
                                  placeholder={
                                    mixedGSLBPoolMatchMode
                                      ? '排除国家，例如 CN'
                                      : 'HK,TW'
                                  }
                                  disabled={!autoSyncEnabled}
                                  onChange={(event) => {
                                    const nextRows = rows.slice();
                                    nextRows[index] = {
                                      ...row,
                                      [mixedGSLBPoolMatchMode
                                        ? 'excludeCountries'
                                        : 'countries']: event.target.value,
                                    };
                                    updateRows(nextRows);
                                  }}
                                  className="h-11"
                                />

                                <div
                                  aria-label={`节点池${
                                    mixedGSLBPoolMatchMode ? '排除' : ''
                                  }访问运营商 ${index + 1}`}
                                  role="group"
                                  className="grid min-h-11 gap-2 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-2"
                                >
                                  {gslbOperatorOptions.map((option) => (
                                    <label
                                      key={option.value}
                                      className="flex min-w-0 items-center gap-2 text-xs text-[var(--foreground-primary)]"
                                    >
                                      <input
                                        type="checkbox"
                                        checked={(mixedGSLBPoolMatchMode
                                          ? row.excludeOperators
                                          : row.operators
                                        ).includes(option.value)}
                                        disabled={!autoSyncEnabled}
                                        onChange={(event) => {
                                          const currentOperators =
                                            mixedGSLBPoolMatchMode
                                              ? row.excludeOperators
                                              : row.operators;
                                          const nextOperators = event.target
                                            .checked
                                            ? [
                                                ...currentOperators,
                                                option.value,
                                              ]
                                            : currentOperators.filter(
                                                (item) => item !== option.value,
                                              );
                                          const nextRows = rows.slice();
                                          nextRows[index] = {
                                            ...row,
                                            [mixedGSLBPoolMatchMode
                                              ? 'excludeOperators'
                                              : 'operators']: Array.from(
                                              new Set(nextOperators),
                                            ),
                                          };
                                          updateRows(nextRows);
                                        }}
                                        className="h-4 w-4 shrink-0 accent-[var(--brand-primary)]"
                                      />
                                      <span className="truncate">
                                        {mixedGSLBPoolMatchMode ? '排除' : ''}
                                        {option.label}
                                      </span>
                                    </label>
                                  ))}
                                </div>

                                <ResourceInput
                                  value={
                                    mixedGSLBPoolMatchMode
                                      ? row.excludeAsns
                                      : row.asns
                                  }
                                  aria-label={`节点池${
                                    mixedGSLBPoolMatchMode ? '排除 ' : ''
                                  }ASN ${index + 1}`}
                                  placeholder={
                                    mixedGSLBPoolMatchMode
                                      ? '排除 ASN，例如 AS4134'
                                      : 'AS4134'
                                  }
                                  disabled={!autoSyncEnabled}
                                  onChange={(event) => {
                                    const nextRows = rows.slice();
                                    nextRows[index] = {
                                      ...row,
                                      [mixedGSLBPoolMatchMode
                                        ? 'excludeAsns'
                                        : 'asns']: event.target.value,
                                    };
                                    updateRows(nextRows);
                                  }}
                                  className="h-11"
                                />

                                <ResourceInput
                                  value={
                                    mixedGSLBPoolMatchMode
                                      ? row.excludeSourceCidrs
                                      : row.sourceCidrs
                                  }
                                  aria-label={`节点池${
                                    mixedGSLBPoolMatchMode ? '排除' : ''
                                  }来源网段 ${index + 1}`}
                                  placeholder={
                                    mixedGSLBPoolMatchMode
                                      ? '排除网段，例如 203.0.113.0/24'
                                      : '203.0.113.0/24'
                                  }
                                  disabled={!autoSyncEnabled}
                                  onChange={(event) => {
                                    const nextRows = rows.slice();
                                    nextRows[index] = {
                                      ...row,
                                      [mixedGSLBPoolMatchMode
                                        ? 'excludeSourceCidrs'
                                        : 'sourceCidrs']: event.target.value,
                                    };
                                    updateRows(nextRows);
                                  }}
                                  className="h-11"
                                />
                              </div>
                            );
                          })}

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

                <div className="grid gap-5 md:grid-cols-2">
                  <ResourceField
                    label="节点池匹配模式"
                    hint={gslbPoolMatchModeHint}
                  >
                    <ResourceSelect
                      aria-label="节点池匹配模式"
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_pool_match_mode')}
                    >
                      <option value="priority">专属池优先</option>
                      <option value="mixed_weighted">混合权重</option>
                    </ResourceSelect>
                  </ResourceField>

                  <ResourceField
                    label="来源池无可用目标时"
                    hint={gslbSourcePoolFallbackHint}
                  >
                    <ResourceSelect
                      aria-label="来源池无可用目标时"
                      disabled={!autoSyncEnabled}
                      {...form.register('gslb_source_pool_fallback_mode')}
                    >
                      <option value="strict">严格匹配，返回空结果</option>
                      <option value="fallback_to_global">
                        回退到全局节点池
                      </option>
                    </ResourceSelect>
                  </ResourceField>
                </div>

                <ResourceField
                  label="代理缓冲模式"
                  hint={proxyBufferingModeHint}
                >
                  <ResourceSelect
                    aria-label="代理缓冲模式"
                    disabled={!autoSyncEnabled}
                    {...form.register('proxy_buffering_mode')}
                  >
                    <option value="default">默认模式：开启代理缓冲</option>
                    <option value="off">流媒体模式：关闭代理缓冲</option>
                  </ResourceSelect>
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
