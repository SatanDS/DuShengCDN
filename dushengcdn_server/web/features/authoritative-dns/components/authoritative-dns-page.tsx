'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Minus, Plus, Settings } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { RankChart } from '@/components/data/rank-chart';
import { TrendChart } from '@/components/data/trend-chart';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { AppModal } from '@/components/ui/app-modal';
import { StatusBadge } from '@/components/ui/status-badge';
import {
  checkDNSZoneDelegation,
  createDNSWorker,
  createDNSZone,
  createDNSZoneRecord,
  deleteDNSRecord,
  deleteDNSWorker,
  deleteDNSZone,
  getDNSGSLBSchedulingStates,
  getDNSMigrationCandidates,
  getDNSObservability,
  getDNSWorkers,
  getDNSZoneRecords,
  getDNSZones,
  probeDNSWorker,
  requestDNSWorkerUpdate,
  simulateDNSGSLB,
  updateDNSWorker,
  updateDNSRecord,
  updateDNSZone,
} from '@/features/authoritative-dns/api/authoritative-dns';
import type {
  AuthoritativeDNSMigrationCandidate,
  DNSGSLBSimulationPayload,
  DNSGSLBSimulationResult,
  DNSGSLBSchedulingState,
  DNSGSLBSchedulingStateStatus,
  DNSGSLBSchedulingStates,
  DNSObservabilityCounterItem,
  DNSObservabilitySummary,
  DNSRecordItem,
  DNSRecordMutationPayload,
  DNSRecordType,
  DNSWorkerHealthItem,
  DNSWorkerItem,
  DNSWorkerProbe,
  DNSWorkerProbeResult,
  DNSWorkerProbeStatus,
  DNSWorkerSnapshotConsistency,
  DNSWorkerSnapshotConsistencyStatus,
  DNSZoneDelegationCheck,
  DNSZoneDelegationStatus,
  DNSZoneItem,
  DNSZoneMutationPayload,
} from '@/features/authoritative-dns/types';
import {
  getProxyRoutes,
  switchProxyRouteToAuthoritativeDNS,
} from '@/features/proxy-routes/api/proxy-routes';
import { getErrorMessage } from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  CodeBlock,
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';
import { copyToClipboard } from '@/lib/utils/clipboard';
import { formatCountryName } from '@/lib/utils/countries';
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';

type FeedbackState = {
  tone: 'info' | 'success' | 'danger';
  message: string;
};

function isMeaningfulTime(value: string | null | undefined) {
  return Boolean(value) && !String(value).startsWith('0001-01-01');
}

type ActiveTab = 'zones' | 'workers' | 'migration';

type ZoneFormValues = {
  name: string;
  soa_email: string;
  primary_ns: string;
  name_servers_text: string;
  default_ttl: number;
  enabled: boolean;
};

type RecordFormValues = {
  name: string;
  type: DNSRecordType;
  value: string;
  ip_values: string[];
  ttl: number;
  priority: number;
  enabled: boolean;
};

type WorkerFormValues = {
  name: string;
  public_address: string;
  remark: string;
};

type WorkerSettingsFormValues = {
  remark: string;
};

type GSLBSimulationFormValues = {
  proxy_route_id: string;
  qname: string;
  record_type: 'A' | 'AAAA';
  country: string;
  operator: string;
  asn: string;
  source_ip: string;
};

type MigrationRecheckStepStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'warning'
  | 'danger';

type MigrationRecheckStatus = 'running' | 'success' | 'warning' | 'danger';

type MigrationRecheckStepKey =
  | 'mode'
  | 'delegation'
  | 'worker_probe'
  | 'simulation';

type MigrationRecheckStep = {
  key: MigrationRecheckStepKey;
  label: string;
  status: MigrationRecheckStepStatus;
  message: string;
};

type MigrationRecheckResult = {
  routeId: number;
  routeName: string;
  zoneId: number;
  zoneName: string;
  checkedAt: string;
  status: MigrationRecheckStatus;
  steps: MigrationRecheckStep[];
  delegationCheck: DNSZoneDelegationCheck | null;
  workerProbes: DNSWorkerProbe[];
  simulations: DNSGSLBSimulationResult[];
};

const dnsObservabilityWindowHours = 6;

const migrationRecheckStepTemplates: Array<{
  key: MigrationRecheckStepKey;
  label: string;
}> = [
  { key: 'mode', label: '网站解析模式' },
  { key: 'delegation', label: '托管域名指向检查' },
  { key: 'worker_probe', label: '响应端公网探测' },
  { key: 'simulation', label: '智能解析复测' },
];

const dnsRecordTypes: DNSRecordType[] = [
  'A',
  'AAAA',
  'CNAME',
  'TXT',
  'MX',
  'NS',
  'SOA',
];

const gslbSimulationOperatorOptions = [
  { value: 'cn-telecom', label: '电信' },
  { value: 'cn-unicom', label: '联通' },
  { value: 'cn-mobile', label: '移动' },
  { value: 'cn-broadcast', label: '广电' },
  { value: 'cernet', label: '教育网' },
];

const proxyRouteDetailPath = '/proxy-route/detail';

function linesFromText(value: string) {
  return value
    .split(/[\r\n,，;；]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function getRouteDomains(route: ProxyRouteItem) {
  const domains = route.domains?.length ? route.domains : [route.domain];
  return domains.map((domain) => domain.trim()).filter(Boolean);
}

function getRouteDetailHref(route: ProxyRouteItem) {
  return `${proxyRouteDetailPath}?id=${route.id}`;
}

function getGSLBDescription(route: ProxyRouteItem) {
  if (!route.gslb_enabled) {
    return route.node_pool || 'default';
  }

  const enabledPools =
    route.gslb_policy?.pools?.filter((pool) => pool.enabled) ?? [];
  if (enabledPools.length === 0) {
    return '已启用，未配置节点池';
  }

  return enabledPools
    .map((pool) => `${pool.name || '未命名'}:${pool.weight}`)
    .join(' / ');
}

function getRouteDisplayName(route: ProxyRouteItem) {
  return route.site_name || route.primary_domain || route.domain;
}

function getDefaultSimulationQName(route?: ProxyRouteItem | null) {
  if (!route) {
    return '';
  }
  const domain =
    getRouteDomains(route)[0] ?? route.primary_domain ?? route.domain ?? '';
  if (domain.startsWith('*.')) {
    return `www.${domain.slice(2)}`;
  }
  return domain;
}

function getRouteRecordType(route?: ProxyRouteItem | null): 'A' | 'AAAA' {
  return route?.dns_record_type === 'AAAA' ? 'AAAA' : 'A';
}

function getCandidateRecordType(
  candidate: AuthoritativeDNSMigrationCandidate,
): 'A' | 'AAAA' {
  return candidate.dns_record_type === 'AAAA' ? 'AAAA' : 'A';
}

function createMigrationRecheckResult(
  route: ProxyRouteItem,
  zone: DNSZoneItem,
  status: MigrationRecheckStatus = 'running',
): MigrationRecheckResult {
  return {
    routeId: route.id,
    routeName: getRouteDisplayName(route),
    zoneId: zone.id,
    zoneName: zone.name,
    checkedAt: new Date().toISOString(),
    status,
    steps: migrationRecheckStepTemplates.map((step) => ({
      ...step,
      status: 'pending',
      message: '等待复测',
    })),
    delegationCheck: null,
    workerProbes: [],
    simulations: [],
  };
}

function updateMigrationRecheckStep(
  result: MigrationRecheckResult,
  key: MigrationRecheckStepKey,
  status: MigrationRecheckStepStatus,
  message: string,
): MigrationRecheckResult {
  return {
    ...result,
    checkedAt: new Date().toISOString(),
    steps: result.steps.map((step) =>
      step.key === key ? { ...step, status, message } : step,
    ),
  };
}

function finalizeMigrationRecheck(
  result: MigrationRecheckResult,
): MigrationRecheckResult {
  const hasDanger = result.steps.some((step) => step.status === 'danger');
  const hasWarning = result.steps.some((step) => step.status === 'warning');
  return {
    ...result,
    checkedAt: new Date().toISOString(),
    status: hasDanger ? 'danger' : hasWarning ? 'warning' : 'success',
  };
}

function mergeUpdatedRoute(
  routes: ProxyRouteItem[],
  updatedRoute: ProxyRouteItem,
) {
  let found = false;
  const nextRoutes = routes.map((route) => {
    if (route.id !== updatedRoute.id) {
      return route;
    }
    found = true;
    return updatedRoute;
  });
  return found ? nextRoutes : [updatedRoute, ...nextRoutes];
}

function getRouteSimulationCountries(route: ProxyRouteItem) {
  const countries = new Set<string>();
  for (const pool of route.gslb_policy?.pools ?? []) {
    if (!pool.enabled) {
      continue;
    }
    for (const country of pool.countries ?? []) {
      const normalized = country.trim().toUpperCase();
      if (normalized) {
        countries.add(normalized);
      }
      if (countries.size >= 2) {
        break;
      }
    }
    if (countries.size >= 2) {
      break;
    }
  }
  return ['', ...Array.from(countries)].slice(0, 3);
}

function getRecheckStepBadgeVariant(status: MigrationRecheckStepStatus) {
  switch (status) {
    case 'success':
      return 'success' as const;
    case 'warning':
      return 'warning' as const;
    case 'danger':
      return 'danger' as const;
    case 'running':
      return 'info' as const;
    case 'pending':
      return 'warning' as const;
  }
}

function getRecheckStepStatusLabel(status: MigrationRecheckStepStatus) {
  switch (status) {
    case 'success':
      return '通过';
    case 'warning':
      return '注意';
    case 'danger':
      return '失败';
    case 'running':
      return '执行中';
    case 'pending':
      return '等待';
  }
}

function getMigrationRecheckTone(status: MigrationRecheckStatus) {
  return status === 'success'
    ? ('success' as const)
    : status === 'danger'
      ? ('danger' as const)
      : ('info' as const);
}

function zoneToFormValues(zone?: DNSZoneItem | null): ZoneFormValues {
  return {
    name: zone?.name ?? '',
    soa_email: zone?.soa_email ?? '',
    primary_ns: zone?.primary_ns ?? '',
    name_servers_text: zone?.name_servers.join('\n') ?? '',
    default_ttl: zone?.default_ttl ?? 300,
    enabled: zone?.enabled ?? true,
  };
}

function recordToFormValues(record?: DNSRecordItem | null): RecordFormValues {
  const value = record?.value ?? '';
  return {
    name: record?.name ?? '@',
    type: record?.type ?? 'A',
    value,
    ip_values: value ? [value] : [''],
    ttl: record?.ttl ?? 0,
    priority: record?.priority ?? 0,
    enabled: record?.enabled ?? true,
  };
}

function isAddressRecordType(type: DNSRecordType) {
  return type === 'A' || type === 'AAAA';
}

function getRecordValueLabel(type: DNSRecordType) {
	switch (type) {
	case 'A':
		return 'IP 地址';
	case 'AAAA':
		return 'IP 地址';
    case 'MX':
      return '邮件服务器';
    case 'CNAME':
      return '目标域名';
    case 'NS':
      return 'NS 域名';
    case 'TXT':
      return 'TXT 内容';
    case 'SOA':
      return 'SOA 内容';
  }
}

function getRecordValueHint(type: DNSRecordType) {
	switch (type) {
	case 'A':
		return '这里填写 IPv4，一个输入框一个 IP；点 + 可继续添加，保存时会创建多条 A 记录。';
	case 'AAAA':
		return '这里填写 IPv6，一个输入框一个 IP；点 + 可继续添加，保存时会创建多条 AAAA 记录。';
	case 'CNAME':
		return '填写目标域名，同名下不要再添加其它记录。';
	case 'MX':
		return '记录值填写邮件服务器域名；MX 优先级数字越小越优先，例如 10 会早于 20。';
    case 'NS':
      return '填写注册商要指向的解析服务器域名。';
    case 'SOA':
      return '填写域名基础信息，通常由系统自动生成即可。';
    case 'TXT':
      return '填写 TXT 文本内容。';
  }
}

function getRecordValuePlaceholder(type: DNSRecordType) {
  switch (type) {
    case 'A':
      return '203.0.113.10';
    case 'AAAA':
      return '2001:db8::10';
    case 'MX':
      return 'mail.example.com';
    case 'CNAME':
      return 'target.example.com';
    case 'NS':
      return 'ns1.example.com';
    case 'TXT':
      return 'v=spf1 ...';
    case 'SOA':
      return 'ns1.example.com hostmaster.example.com ...';
  }
}

function getWorkerStatusVariant(status: DNSWorkerItem['status']) {
  return status === 'online' ? 'success' : 'warning';
}

function getWorkerSourceCapabilityCounts(workers: DNSWorkerItem[]) {
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  const source = onlineWorkers.length > 0 ? onlineWorkers : workers;
  return {
    total: source.length,
    country: source.filter((worker) => worker.geoip_country_enabled).length,
    asn: source.filter((worker) => worker.geoip_asn_enabled).length,
    operator: source.filter((worker) => worker.geoip_operator_enabled).length,
  };
}

function getWorkerSourceCapabilityMessage(workers: DNSWorkerItem[]) {
  const counts = getWorkerSourceCapabilityCounts(workers);
  if (counts.total === 0) {
    return '尚未创建 DNS 响应端，运营商/ASN 规则会在响应端部署并上报识别库后生效。';
  }
  return `识别库能力：国家 ${counts.country}/${counts.total}，ASN ${counts.asn}/${counts.total}，运营商 ${counts.operator}/${counts.total}。运营商来自 gaoyifan/china-operator-ip CIDR 库，ASN 来自 GeoLite2-ASN 或兼容 MMDB。`;
}

function getWorkerSourceCapabilityTone(workers: DNSWorkerItem[]) {
  const counts = getWorkerSourceCapabilityCounts(workers);
  if (counts.total === 0) {
    return 'info' as const;
  }
  if (counts.asn === counts.total && counts.operator === counts.total) {
    return 'success' as const;
  }
  return 'info' as const;
}

function formatCount(value: number) {
  return value.toLocaleString('zh-CN');
}

function formatPercent(numerator: number, denominator: number) {
  if (denominator <= 0) {
    return '0%';
  }
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

function formatPercentValue(value: number) {
  return `${value.toFixed(1)}%`;
}

function formatLatencyMs(value: number) {
  if (value <= 0) {
    return '0 ms';
  }
  return `${Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)} ms`;
}

function formatSourceScopeLabel(value: string) {
  const text = value.trim();
  if (!text) {
    return '全局';
  }
  const parts = text
    .split('|')
    .map((item) => item.trim())
    .filter(Boolean);
  const base = parts[0] ?? 'global';
  const bucket = parts.find((item) => item.toLowerCase().startsWith('bucket:'));
  const baseLabel = formatSourceScopeBaseLabel(base);
  if (!bucket) {
    return baseLabel;
  }
  const bucketValue = bucket.split(':')[1]?.trim();
  return bucketValue ? `${baseLabel} / 分流桶 ${bucketValue}` : baseLabel;
}

function formatSourceScopeBaseLabel(value: string) {
  const text = value.trim();
  const lower = text.toLowerCase();
  if (lower === 'global') {
    return '全局';
  }
  if (lower.startsWith('country:')) {
    const country = text
      .slice(text.indexOf(':') + 1)
      .trim()
      .toUpperCase();
    return country ? formatCountryName(country, country) : text;
  }
  if (lower.startsWith('cidr:')) {
    const cidr = text.slice(text.indexOf(':') + 1).trim();
    return cidr ? `网段 ${cidr}` : text;
  }
  if (lower.startsWith('operator:')) {
    const operator = text.slice(text.indexOf(':') + 1).trim();
    return operator ? `运营商 ${formatGSLBOperatorLabel(operator)}` : text;
  }
  if (lower.startsWith('asn:')) {
    const asn = text.slice(text.indexOf(':') + 1).trim().replace(/^AS/i, '');
    return asn ? `ASN AS${asn}` : text;
  }
  return text;
}

function formatGSLBOperatorLabel(value: string) {
  const normalized = value.trim().toLowerCase();
  return (
    gslbSimulationOperatorOptions.find((option) => option.value === normalized)
      ?.label ?? value
  );
}

function parseSimulationASN(value: string) {
  const normalized = value
    .trim()
    .replace(/^asn:\s*/i, '')
    .replace(/^as/i, '');
  if (!normalized) {
    return undefined;
  }
  const asn = Number(normalized);
  if (!Number.isInteger(asn) || asn <= 0 || asn > 4294967295) {
    return undefined;
  }
  return asn;
}

function formatDNSRCodeLabel(item: DNSObservabilityCounterItem) {
  const code = (item.key || item.label || '').trim().toUpperCase();
  const labels: Record<string, string> = {
    NOERROR: '正常响应',
    NODATA: '无记录数据',
    NXDOMAIN: '域名不存在',
    SERVFAIL: '响应端故障',
    REFUSED: '拒绝查询',
    FORMERR: '请求格式错误',
    NOTIMP: '暂不支持',
  };
  return labels[code] || item.label || item.key || '未命名';
}

function isProbeSchedulingGateMessage(message: string) {
  return message.includes('Agent 探测未达到调度门槛');
}

function formatDiagnosticMessage(message: string) {
  return message
    .replaceAll('节点负载超过 GSLB 阈值', '节点压力超过上限')
    .replaceAll('当前 Server 生成的权威 DNS 快照', '当前面板生成的本地解析配置快照')
    .replaceAll('OpenResty 健康', '代理服务是否正常')
    .replaceAll('GSLB 阈值', '压力上限')
    .replaceAll('GSLB 负载阈值', '压力上限')
    .replaceAll('GSLB', '多节点智能解析')
    .replaceAll('DNS Worker 多点探测', '响应端多地探测')
    .replaceAll('DNS Worker', 'DNS 响应端')
    .replaceAll('调度防抖状态', '切换冷却状态')
    .replaceAll('DNS 防抖状态', '解析切换冷却状态')
    .replaceAll('Agent 探测未达到调度门槛', '节点到响应端探测未达要求')
    .replaceAll('调度门槛', '选择要求')
    .replaceAll('来源 CIDR', '来源网段')
    .replaceAll('到 响应端', '到响应端')
    .replaceAll('的 响应端', '的响应端');
}

function formatDurationSeconds(value: number) {
  if (value <= 0) {
    return '—';
  }
  if (value < 60) {
    return `${value} 秒`;
  }
  if (value < 3600) {
    return `${Math.round(value / 60)} 分钟`;
  }
  return `${Math.round(value / 3600)} 小时`;
}

function getProbeResultVariant(result: DNSWorkerProbeResult) {
  return result.reachable ? ('success' as const) : ('danger' as const);
}

function getProbeStatusLabel(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return '公网可达';
    case 'partial':
      return '部分可达';
    case 'failed':
      return '探测失败';
    case 'stale':
      return '公网探测过期';
    case 'unknown':
      return '未探测';
  }
}

function getProbeStatusVariant(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return 'success' as const;
    case 'partial':
    case 'stale':
    case 'unknown':
      return 'warning' as const;
    case 'failed':
      return 'danger' as const;
  }
}

function getNodeDNSProbeStatusLabel(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return '公网可达';
    case 'partial':
      return '部分可达';
    case 'failed':
      return '探测失败';
    case 'stale':
      return '多节点探测过期';
    case 'unknown':
      return '未探测';
  }
}

function getNodeProbeStatusVariant(status: string) {
  switch (status) {
    case 'online':
      return 'success' as const;
    case 'pending':
      return 'warning' as const;
    default:
      return 'danger' as const;
  }
}

function workerProbeToPanelData(worker: DNSWorkerItem): DNSWorkerProbe | null {
  if (!worker.last_probe_at || worker.last_probe_results.length === 0) {
    return null;
  }
  const [queryName = '', queryType = ''] = worker.last_probe_query.split(/\s+/);
  return {
    worker_id: worker.worker_id,
    name: worker.name,
    public_address: worker.public_address,
    query_name: queryName,
    query_type: queryType,
    checked_at: worker.last_probe_at,
    results: worker.last_probe_results,
  };
}

function formatTrendHour(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getSnapshotConsistencyLabel(
  status: DNSWorkerSnapshotConsistencyStatus,
) {
  switch (status) {
    case 'consistent':
      return '解析配置一致';
    case 'divergent':
      return '解析配置不一致';
    case 'stale':
      return '解析配置过期';
    case 'no_online_workers':
      return '无在线响应端';
    case 'unknown':
      return '状态未知';
  }
}

function getSnapshotConsistencyVariant(
  status: DNSWorkerSnapshotConsistencyStatus,
) {
  switch (status) {
    case 'consistent':
      return 'success' as const;
    case 'divergent':
    case 'stale':
      return 'danger' as const;
    case 'no_online_workers':
    case 'unknown':
      return 'warning' as const;
  }
}

function getSnapshotConsistencyMessage(
  consistency: DNSWorkerSnapshotConsistency,
) {
  if (consistency.status === 'stale') {
    return `解析配置过期：有 ${formatCount(consistency.stale_worker_count)} 个在线响应端超过 ${formatDurationSeconds(consistency.snapshot_max_age_seconds)} 没有拉取最新解析配置。请检查响应端能否访问面板地址、响应端密钥是否有效，以及服务日志中 /api/dns-snapshot 的 HTTP 状态。`;
  }
  if (consistency.status === 'divergent') {
    const versionCount = consistency.version_breakdown.length;
    const latestVersion = consistency.latest_snapshot_version || '未知版本';
    return `解析配置不一致：在线响应端当前使用了 ${formatCount(versionCount)} 个配置版本，查询结果可能不一致。最新版本 ${latestVersion}；请检查落后的响应端是否能访问面板、响应端密钥是否仍有效，必要时重启响应端重新拉取配置。`;
  }
  return '';
}

function getDNSObservabilityRollupHint(summary: DNSObservabilitySummary) {
  if (summary.total_queries > 0) {
    return '';
  }
  const healthWorkers = summary.worker_health?.workers ?? [];
  const consistencyWorkers = summary.snapshot_consistency?.workers ?? [];
  const workers = healthWorkers.length > 0 ? healthWorkers : consistencyWorkers;
  if (workers.length === 0) {
    return '当前还没有 DNS 响应端。创建并部署响应端后，页面才会收到 DNS 查询观测数据。';
  }
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  if (onlineWorkers.length === 0) {
    return '当前没有在线 DNS 响应端，查询观测不会产生数据。请先确认响应端进程、53 端口和面板连接。';
  }
  const heartbeatWorkers = onlineWorkers.filter((worker) =>
    isMeaningfulTime(worker.last_heartbeat_at),
  );
  if (heartbeatWorkers.length === 0) {
    return '响应端能拉取解析配置，但还没有成功发送心跳。请检查响应端到面板的 /api/dns-worker-heartbeat 请求、响应端密钥和反代限制。';
  }
  const rollupWorkers = onlineWorkers.filter((worker) =>
    isMeaningfulTime(worker.last_rollup_at),
  );
  if (rollupWorkers.length === 0) {
    return '响应端心跳正常，但还没有上报 DNS 查询 rollup。请确认公网 NS 流量是否真正打到这些响应端，以及响应端版本是否包含查询观测上报。';
  }
  return '最近窗口内没有 DNS 查询 rollup。若业务确实有查询，请检查客户端使用的 NS、域名委派、响应端时间和面板时间是否一致。';
}

function shouldShowNoAuthoritativeRoutesNotice({
  zones,
  workers,
  routes,
  routesLoading,
  routesError,
}: {
  zones: DNSZoneItem[];
  workers: DNSWorkerItem[];
  routes: ProxyRouteItem[];
  routesLoading: boolean;
  routesError: boolean;
}) {
  if (routesLoading || routesError) {
    return false;
  }
  const hasEnabledZone = zones.some((zone) => zone.enabled);
  const hasReadyWorker = workers.some(
    (worker) =>
      worker.status === 'online' &&
      Boolean(worker.last_snapshot_version || worker.last_snapshot_at),
  );
  const hasAuthoritativeRoute = routes.some(
    (route) =>
      route.enabled &&
      route.dns_provider_mode === 'authoritative' &&
      route.dns_zone_id_ref != null,
  );
  return hasEnabledZone && hasReadyWorker && !hasAuthoritativeRoute;
}

function getSchedulingStateStatusLabel(status: DNSGSLBSchedulingStateStatus) {
  switch (status) {
    case 'active':
      return '已生效';
    case 'debouncing':
      return '冷却中';
    case 'inactive':
      return '站点停用';
    case 'stale':
      return '记录过期';
    case 'empty':
      return '无目标';
    case 'orphaned':
      return '站点已删除';
  }
}

function getSchedulingStateStatusVariant(status: DNSGSLBSchedulingStateStatus) {
  switch (status) {
    case 'active':
      return 'success' as const;
    case 'debouncing':
      return 'info' as const;
    case 'inactive':
    case 'stale':
    case 'empty':
      return 'warning' as const;
    case 'orphaned':
      return 'danger' as const;
  }
}

function getDelegationStatusLabel(status: DNSZoneDelegationStatus) {
  switch (status) {
    case 'matched':
      return '已匹配';
    case 'partial':
      return '部分匹配';
    case 'mismatch':
      return '不匹配';
    case 'failed':
      return '检查失败';
    case 'not_configured':
      return '未配置';
  }
}

function getDelegationStatusVariant(status: DNSZoneDelegationStatus) {
  switch (status) {
    case 'matched':
      return 'success' as const;
    case 'partial':
    case 'not_configured':
      return 'warning' as const;
    case 'mismatch':
    case 'failed':
      return 'danger' as const;
  }
}

export function AuthoritativeDNSPage() {
  const queryClient = useQueryClient();
  const confirmDialog = useConfirmDialog();
  const { setFeedback } = useToastFeedback<FeedbackState>();
  const [activeTab, setActiveTab] = useState<ActiveTab>('zones');
  const [selectedZoneId, setSelectedZoneId] = useState<number | null>(null);
  const [editingZone, setEditingZone] = useState<DNSZoneItem | null>(null);
  const [isZoneModalOpen, setIsZoneModalOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<DNSRecordItem | null>(
    null,
  );
  const [recordZone, setRecordZone] = useState<DNSZoneItem | null>(null);
  const [isWorkerModalOpen, setIsWorkerModalOpen] = useState(false);
  const [workerSettingsTarget, setWorkerSettingsTarget] =
    useState<DNSWorkerHealthItem | null>(null);
  const [createdWorker, setCreatedWorker] = useState<DNSWorkerItem | null>(
    null,
  );
  const [workerProbeResults, setWorkerProbeResults] = useState<
    Record<number, DNSWorkerProbe>
  >({});
  const [migrationRecheck, setMigrationRecheck] =
    useState<MigrationRecheckResult | null>(null);
  const [recheckingRouteId, setRecheckingRouteId] = useState<number | null>(
    null,
  );
  const [serverUrl, setServerUrl] = useState('https://cdn.example.com');

  useEffect(() => {
    setServerUrl(window.location.origin);
  }, []);

  const zonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
  });
  const workersQuery = useQuery({
    queryKey: ['authoritative-dns', 'workers'],
    queryFn: getDNSWorkers,
  });
  const proxyRoutesQuery = useQuery({
    queryKey: ['proxy-routes'],
    queryFn: getProxyRoutes,
  });
  const observabilityQuery = useQuery({
    queryKey: [
      'authoritative-dns',
      'observability',
      dnsObservabilityWindowHours,
    ],
    queryFn: () => getDNSObservability(dnsObservabilityWindowHours),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    placeholderData: (previousData) => previousData,
  });
  const schedulingStatesQuery = useQuery({
    queryKey: ['authoritative-dns', 'scheduling-states'],
    queryFn: getDNSGSLBSchedulingStates,
  });
  const migrationCandidatesQuery = useQuery({
    queryKey: ['authoritative-dns', 'migration-candidates'],
    queryFn: getDNSMigrationCandidates,
  });

  const zones = useMemo(() => zonesQuery.data ?? [], [zonesQuery.data]);
  const workers = useMemo(() => workersQuery.data ?? [], [workersQuery.data]);
  const migrationCandidates = useMemo(
    () => migrationCandidatesQuery.data ?? [],
    [migrationCandidatesQuery.data],
  );
  const proxyRoutes = useMemo(
    () => proxyRoutesQuery.data ?? [],
    [proxyRoutesQuery.data],
  );
  const observability = observabilityQuery.data ?? null;
  const schedulingStates = schedulingStatesQuery.data ?? null;
  useEffect(() => {
    if (!workerSettingsTarget || !observability?.worker_health?.workers) {
      return;
    }
    const refreshed = observability.worker_health.workers.find(
      (worker) => worker.id === workerSettingsTarget.id,
    );
    if (refreshed && refreshed !== workerSettingsTarget) {
      setWorkerSettingsTarget(refreshed);
    }
  }, [observability?.worker_health?.workers, workerSettingsTarget]);
  const authoritativeRoutes = useMemo(
    () =>
      proxyRoutes
        .filter((route) => route.dns_provider_mode === 'authoritative')
        .filter((route) => route.enabled || route.gslb_enabled),
    [proxyRoutes],
  );
  const handleCopyDNSWorkerCommand = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setFeedback({ tone: 'success', message });
    } catch (error) {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    }
  };
  const showNoAuthoritativeRoutesNotice = shouldShowNoAuthoritativeRoutesNotice(
    {
      zones,
      workers,
      routes: proxyRoutes,
      routesLoading: proxyRoutesQuery.isLoading,
      routesError: proxyRoutesQuery.isError,
    },
  );
  const selectedZone = useMemo(
    () => zones.find((zone) => zone.id === selectedZoneId) ?? zones[0] ?? null,
    [selectedZoneId, zones],
  );
  const selectedZoneRecordsQuery = useQuery({
    queryKey: ['authoritative-dns', 'zone-records', selectedZone?.id],
    queryFn: () => getDNSZoneRecords(selectedZone?.id ?? 0),
    enabled: Boolean(selectedZone?.id),
  });
  const delegationCheckQuery = useQuery({
    queryKey: ['authoritative-dns', 'delegation-check', selectedZone?.id],
    queryFn: () => checkDNSZoneDelegation(selectedZone?.id ?? 0),
    enabled: false,
  });
  const records = useMemo(
    () => selectedZoneRecordsQuery.data ?? [],
    [selectedZoneRecordsQuery.data],
  );

  const deleteZoneMutation = useMutation({
    mutationFn: deleteDNSZone,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: '托管域名已删除。' });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'zones'],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const deleteRecordMutation = useMutation({
    mutationFn: deleteDNSRecord,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'DNS 记录已删除。' });
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zones'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'zone-records'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const deleteWorkerMutation = useMutation({
    mutationFn: deleteDNSWorker,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'DNS 响应端已删除。' });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const updateWorkerMutation = useMutation({
    mutationFn: ({
      id,
      values,
    }: {
      id: number;
      values: WorkerSettingsFormValues;
    }) =>
      updateDNSWorker(id, {
        remark: values.remark.trim(),
      }),
    onSuccess: async (worker) => {
      setFeedback({
        tone: 'success',
        message: `DNS 响应端“${worker.name}”备注已保存。`,
      });
      setWorkerSettingsTarget(null);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'observability'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const probeWorkerMutation = useMutation({
    mutationFn: (worker: DNSWorkerItem) =>
      probeDNSWorker(worker.id, { zone_id: selectedZone?.id }),
    onSuccess: (result, worker) => {
      setWorkerProbeResults((current) => ({
        ...current,
        [worker.id]: result,
      }));
      const reachableCount = result.results.filter(
        (item) => item.reachable,
      ).length;
      setFeedback({
        tone:
          reachableCount === result.results.length && result.results.length > 0
            ? 'success'
            : 'danger',
        message: `DNS 响应端探测完成：${reachableCount} / ${result.results.length} 可达。`,
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const requestWorkerUpdateMutation = useMutation({
    mutationFn: requestDNSWorkerUpdate,
    onSuccess: async (worker) => {
      setFeedback({
        tone: 'success',
        message: `已向 DNS 响应端“${worker.name}”下发更新启动命令，等待下一次心跳执行。`,
      });
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'workers'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'observability'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const simulateGSLBMutation = useMutation({
    mutationFn: simulateDNSGSLB,
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const switchAuthoritativeMutation = useMutation({
    mutationFn: ({
      route,
      zone,
    }: {
      route: ProxyRouteItem;
      zone: DNSZoneItem;
    }) =>
      switchProxyRouteToAuthoritativeDNS(route.id, {
        dns_zone_id_ref: zone.id,
      }),
    onSuccess: async (updatedRoute, variables) => {
      setFeedback({
        tone: 'success',
        message: `已切换“${getRouteDisplayName(updatedRoute)}”到本地自建解析，正在自动复测。`,
      });
      queryClient.setQueryData<ProxyRouteItem[]>(['proxy-routes'], (current) =>
        mergeUpdatedRoute(current ?? proxyRoutes, updatedRoute),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['proxy-routes'] }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'migration-candidates'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['authoritative-dns', 'scheduling-states'],
        }),
      ]);
      await runMigrationRecheck(updatedRoute, variables.zone);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  async function runMigrationRecheck(route: ProxyRouteItem, zone: DNSZoneItem) {
    let result = updateMigrationRecheckStep(
      createMigrationRecheckResult(route, zone),
      'mode',
      'running',
      '正在刷新网站解析模式',
    );
    setMigrationRecheck(result);
    setRecheckingRouteId(route.id);
    setFeedback({
      tone: 'info',
      message: `正在复测“${getRouteDisplayName(route)}”的本地自建解析切换结果。`,
    });

    try {
      let latestRoute = route;
      let latestRoutes = proxyRoutes;
      try {
        latestRoutes = await queryClient.fetchQuery({
          queryKey: ['proxy-routes'],
          queryFn: getProxyRoutes,
        });
        latestRoute =
          latestRoutes.find((item) => item.id === route.id) ?? route;
        if (
          latestRoute.dns_provider_mode !== 'authoritative' &&
          route.dns_provider_mode === 'authoritative' &&
          route.dns_zone_id_ref === zone.id
        ) {
          latestRoute = route;
        }
        result = updateMigrationRecheckStep(
          result,
          'mode',
          latestRoute.dns_provider_mode === 'authoritative' &&
            latestRoute.dns_zone_id_ref === zone.id
            ? 'success'
            : 'danger',
          latestRoute.dns_provider_mode === 'authoritative'
            ? `已绑定托管域名 ${zone.name}`
            : '网站尚未切换到本地自建解析',
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'mode',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);

      result = updateMigrationRecheckStep(
        result,
        'delegation',
        'running',
        '正在检查注册商指向',
      );
      setMigrationRecheck(result);
      try {
        const delegationCheck = await checkDNSZoneDelegation(zone.id);
        result = {
          ...result,
          delegationCheck,
        };
        const delegationStatus =
          delegationCheck.status === 'matched'
            ? 'success'
            : delegationCheck.status === 'failed' ||
                delegationCheck.status === 'mismatch'
              ? 'danger'
              : 'warning';
        result = updateMigrationRecheckStep(
          result,
          'delegation',
          delegationStatus,
          delegationCheck.status === 'matched'
            ? '公网解析服务器已与托管域名配置匹配'
            : `当前委派状态：${getDelegationStatusLabel(delegationCheck.status)}`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'delegation',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);

      result = updateMigrationRecheckStep(
        result,
        'worker_probe',
        'running',
        '正在探测在线 DNS 响应端的 UDP/TCP 53',
      );
      setMigrationRecheck(result);
      try {
        let latestWorkers =
          (await queryClient.fetchQuery({
            queryKey: ['authoritative-dns', 'workers'],
            queryFn: getDNSWorkers,
          })) ?? workers;
        if (latestWorkers.length === 0 && workers.length > 0) {
          latestWorkers = workers;
        }
        const onlineWorkersForProbe = latestWorkers.filter(
          (worker) => worker.status === 'online',
        );
        const workerProbePairs = await Promise.allSettled(
          onlineWorkersForProbe.map(async (worker) => ({
            worker,
            probe: await probeDNSWorker(worker.id, { zone_id: zone.id }),
          })),
        );
        const successfulWorkerProbes: DNSWorkerProbe[] = [];
        const nextWorkerProbeResults: Record<number, DNSWorkerProbe> = {};
        for (const item of workerProbePairs) {
          if (item.status === 'fulfilled') {
            successfulWorkerProbes.push(item.value.probe);
            nextWorkerProbeResults[item.value.worker.id] = item.value.probe;
          }
        }
        if (Object.keys(nextWorkerProbeResults).length > 0) {
          setWorkerProbeResults((current) => ({
            ...current,
            ...nextWorkerProbeResults,
          }));
        }
        const healthyProbeCount = successfulWorkerProbes.filter(
          (probe) =>
            probe.results.length > 0 &&
            probe.results.every((probeResult) => probeResult.reachable),
        ).length;
        result = {
          ...result,
          workerProbes: successfulWorkerProbes,
        };
        result = updateMigrationRecheckStep(
          result,
          'worker_probe',
          healthyProbeCount > 0
            ? healthyProbeCount === onlineWorkersForProbe.length
              ? 'success'
              : 'warning'
            : 'danger',
          onlineWorkersForProbe.length === 0
            ? '没有在线 DNS 响应端'
            : `${healthyProbeCount} / ${onlineWorkersForProbe.length} 个在线响应端 UDP/TCP 53 可达`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'worker_probe',
          'danger',
          getErrorMessage(error),
        );
      }
      setMigrationRecheck(result);
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'observability'],
      });

      result = updateMigrationRecheckStep(
        result,
        'simulation',
        'running',
        '正在按当前解析配置模拟全局和来源国家的返回 IP',
      );
      setMigrationRecheck(result);
      try {
        const routeForSimulation = {
          ...latestRoute,
          dns_provider_mode: 'authoritative' as const,
          dns_zone_id_ref: zone.id,
        };
        const routeListForSimulation = mergeUpdatedRoute(
          latestRoutes,
          routeForSimulation,
        );
        queryClient.setQueryData(['proxy-routes'], routeListForSimulation);
        const simulationCountries =
          getRouteSimulationCountries(routeForSimulation);
        const simulationResults = await Promise.all(
          simulationCountries.map((country) =>
            simulateDNSGSLB({
              proxy_route_id: routeForSimulation.id,
              qname: getDefaultSimulationQName(routeForSimulation),
              record_type: getRouteRecordType(routeForSimulation),
              country,
              source_ip: '',
              fresh: true,
            }),
          ),
        );
        result = {
          ...result,
          simulations: simulationResults,
        };
        const returnedTargetCount = simulationResults.reduce(
          (count, simulation) => count + simulation.targets.length,
          0,
        );
        const noTargetSimulationCount = simulationResults.filter(
          (simulation) => simulation.targets.length === 0,
        ).length;
        result = updateMigrationRecheckStep(
          result,
          'simulation',
          returnedTargetCount === 0
            ? 'danger'
            : noTargetSimulationCount > 0
              ? 'warning'
              : 'success',
          returnedTargetCount === 0
            ? '当前解析配置没有返回可用目标'
            : noTargetSimulationCount > 0
              ? `已完成 ${simulationResults.length} 组模拟，其中 ${noTargetSimulationCount} 组无返回目标`
              : `已完成 ${simulationResults.length} 组模拟，返回 ${returnedTargetCount} 个目标`,
        );
      } catch (error) {
        result = updateMigrationRecheckStep(
          result,
          'simulation',
          'danger',
          getErrorMessage(error),
        );
      }

      result = finalizeMigrationRecheck(result);
      setMigrationRecheck(result);
      setFeedback({
        tone: getMigrationRecheckTone(result.status),
        message:
          result.status === 'success'
            ? `“${result.routeName}”切换后复测通过。`
            : `“${result.routeName}”切换后复测完成，请查看迁移向导右侧结果。`,
      });
    } finally {
      setRecheckingRouteId(null);
    }
  }

  const openCreateZone = () => {
    setEditingZone(null);
    setIsZoneModalOpen(true);
  };
  const openEditZone = (zone: DNSZoneItem) => {
    setEditingZone(zone);
    setIsZoneModalOpen(true);
  };
  const openCreateRecord = (zone: DNSZoneItem) => {
    setRecordZone(zone);
    setEditingRecord(null);
  };
  const openEditRecord = (zone: DNSZoneItem, record: DNSRecordItem) => {
    setRecordZone(zone);
    setEditingRecord(record);
  };

  const handleDeleteZone = async (zone: DNSZoneItem) => {
    const confirmed = await confirmDialog({
      title: '删除托管域名',
      message: `确认删除托管域名“${zone.name}”吗？会同时删除该域名下的静态记录；已被网站本地自建解析引用时后端会阻止删除。`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteZoneMutation.mutate(zone.id);
    }
  };

  const handleDeleteRecord = async (record: DNSRecordItem) => {
    const confirmed = await confirmDialog({
      title: '删除 DNS 记录',
      message: `确认删除 ${record.name} 的 ${record.type} 记录吗？`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteRecordMutation.mutate(record.id);
    }
  };

  const handleDeleteWorker = async (worker: DNSWorkerItem) => {
    const confirmed = await confirmDialog({
      title: '删除 DNS 响应端',
      message: `确认删除响应端“${worker.name}”吗？删除后该响应端密钥将不能再拉取解析配置。`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (confirmed) {
      setFeedback(null);
      deleteWorkerMutation.mutate(worker.id);
    }
  };

  const handleProbeWorker = (worker: DNSWorkerItem) => {
    setFeedback(null);
    probeWorkerMutation.mutate(worker);
  };

  const handleSimulateGSLB = (payload: DNSGSLBSimulationPayload) => {
    setFeedback(null);
    simulateGSLBMutation.mutate(payload);
  };

  const handleSwitchAuthoritative = async (
    route: ProxyRouteItem,
    zone: DNSZoneItem,
  ) => {
    const confirmed = await confirmDialog({
      title: '切换到本地自建解析',
      message: `确认把“${getRouteDisplayName(route)}”切换到本地自建解析，并绑定托管域名“${zone.name}”吗？切换后请到注册商确认 NS 已指向你的 DNS 响应端。`,
      confirmLabel: '切换',
    });
    if (confirmed) {
      setFeedback(null);
      switchAuthoritativeMutation.mutate({ route, zone });
    }
  };

  if (zonesQuery.isLoading || workersQuery.isLoading) {
    return <LoadingState />;
  }

  if (zonesQuery.isError) {
    return (
      <ErrorState
        title="托管域名加载失败"
        description={getErrorMessage(zonesQuery.error)}
      />
    );
  }

  if (workersQuery.isError) {
    return (
      <ErrorState
        title="DNS 响应端加载失败"
        description={getErrorMessage(workersQuery.error)}
      />
    );
  }

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="本地自建解析"
          description="管理本地自建解析的托管域名、静态记录和 DNS 响应端，用于按访问来源与节点状态自动返回合适的边缘 IP。"
          action={
            <div className="flex flex-wrap gap-3">
              <SecondaryButton
                type="button"
                onClick={() => setIsWorkerModalOpen(true)}
              >
                创建 DNS 响应端
              </SecondaryButton>
              <PrimaryButton type="button" onClick={openCreateZone}>
                创建托管域名
              </PrimaryButton>
            </div>
          }
        />

        <div className="grid gap-4 md:grid-cols-3">
          <AppCard title="托管域名">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {zones.length}
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                启用 {zones.filter((zone) => zone.enabled).length} 个
              </p>
            </div>
          </AppCard>
          <AppCard title="静态记录">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {zones
                  .reduce((sum, zone) => sum + zone.record_count, 0)
                  .toLocaleString('zh-CN')}
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                不含网站自动返回的动态记录
              </p>
            </div>
          </AppCard>
          <AppCard title="DNS 响应端">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {workers.filter((worker) => worker.status === 'online').length}
                <span className="text-base text-[var(--foreground-secondary)]">
                  {' '}
                  / {workers.length}
                </span>
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                在线 / 全部响应端
              </p>
            </div>
          </AppCard>
        </div>

        {showNoAuthoritativeRoutesNotice ? (
          <InlineMessage
            tone="warning"
            message="DNS 响应端已经能拉取解析配置，但当前没有启用的网站绑定到本地自建解析。此时响应端只能回答基础记录和静态记录；业务域名需要到网站详情「负载均衡」切换为本地自建解析并选择对应托管域名，或使用迁移向导一键切换。"
          />
        ) : null}

        <DNSObservabilityPanel
          summary={observability}
          isLoading={observabilityQuery.isLoading}
          error={
            observabilityQuery.isError
              ? getErrorMessage(observabilityQuery.error)
              : ''
          }
          onCopyCommand={handleCopyDNSWorkerCommand}
          onOpenWorkerSettings={setWorkerSettingsTarget}
        />

        <GSLBSimulationPanel
          routes={authoritativeRoutes}
          routesLoading={proxyRoutesQuery.isLoading}
          routesError={
            proxyRoutesQuery.isError
              ? getErrorMessage(proxyRoutesQuery.error)
              : ''
          }
          result={simulateGSLBMutation.data ?? null}
          error={
            simulateGSLBMutation.isError
              ? getErrorMessage(simulateGSLBMutation.error)
              : ''
          }
          isPending={simulateGSLBMutation.isPending}
          onSimulate={handleSimulateGSLB}
        />

        <div className="flex flex-wrap gap-3">
          {[
            {
              key: 'zones' as const,
              label: '托管域名与记录',
              description: '托管域名、注册商 NS 和静态 DNS 记录。',
            },
            {
              key: 'workers' as const,
              label: 'DNS 响应端',
              description: '管理对外回答 DNS 查询的服务和配置状态。',
            },
            {
              key: 'migration' as const,
              label: '迁移向导',
              description: '检查 Cloudflare 站点切换到本地自建解析的准备项。',
            },
          ].map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'rounded-2xl border px-4 py-3 text-left transition',
                activeTab === tab.key
                  ? 'border-[var(--border-strong)] bg-[var(--accent-soft)] text-[var(--foreground-primary)]'
                  : 'border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--border-strong)] hover:text-[var(--foreground-primary)]',
              )}
            >
              <p className="text-sm font-semibold">{tab.label}</p>
              <p className="mt-1 text-xs leading-5 text-inherit/80">
                {tab.description}
              </p>
            </button>
          ))}
        </div>

        {activeTab === 'zones' ? (
          <ZonesPanel
            zones={zones}
            selectedZone={selectedZone}
            records={records}
            recordsLoading={selectedZoneRecordsQuery.isLoading}
            recordsError={
              selectedZoneRecordsQuery.isError
                ? getErrorMessage(selectedZoneRecordsQuery.error)
                : ''
            }
            delegationCheck={delegationCheckQuery.data ?? null}
            delegationLoading={delegationCheckQuery.isFetching}
            delegationError={
              delegationCheckQuery.isError
                ? getErrorMessage(delegationCheckQuery.error)
                : ''
            }
            onCheckDelegation={() => {
              setFeedback(null);
              void delegationCheckQuery.refetch();
            }}
            onSelectZone={(zone) => setSelectedZoneId(zone.id)}
            onCreateZone={openCreateZone}
            onEditZone={openEditZone}
            onDeleteZone={handleDeleteZone}
            onCreateRecord={openCreateRecord}
            onEditRecord={openEditRecord}
            onDeleteRecord={handleDeleteRecord}
            busy={
              deleteZoneMutation.isPending || deleteRecordMutation.isPending
            }
          />
        ) : activeTab === 'workers' ? (
          <WorkersPanel
            workers={workers}
            onCreateWorker={() => setIsWorkerModalOpen(true)}
            onDeleteWorker={handleDeleteWorker}
            onProbeWorker={handleProbeWorker}
            busy={deleteWorkerMutation.isPending}
            probingWorkerId={
              probeWorkerMutation.isPending
                ? (probeWorkerMutation.variables?.id ?? null)
                : null
            }
            probeResults={workerProbeResults}
          />
        ) : (
          <DNSMigrationGuidePanel
            routes={proxyRoutes}
            migrationCandidates={migrationCandidates}
            zones={zones}
            workers={workers}
            routesLoading={
              proxyRoutesQuery.isLoading || migrationCandidatesQuery.isLoading
            }
            routesError={
              proxyRoutesQuery.isError
                ? getErrorMessage(proxyRoutesQuery.error)
                : migrationCandidatesQuery.isError
                  ? getErrorMessage(migrationCandidatesQuery.error)
                  : ''
            }
            switchingRouteId={
              switchAuthoritativeMutation.isPending
                ? switchAuthoritativeMutation.variables.route.id
                : null
            }
            recheckingRouteId={recheckingRouteId}
            recheckResult={migrationRecheck}
            onSwitchAuthoritative={handleSwitchAuthoritative}
          />
        )}

        <GSLBSchedulingStatesPanel
          states={schedulingStates}
          isLoading={schedulingStatesQuery.isLoading}
          error={
            schedulingStatesQuery.isError
              ? getErrorMessage(schedulingStatesQuery.error)
              : ''
          }
        />
      </div>

      {isZoneModalOpen ? (
        <ZoneEditorModal
          isOpen={isZoneModalOpen}
          zone={editingZone}
          onClose={() => setIsZoneModalOpen(false)}
          onSaved={async (zone) => {
            setSelectedZoneId(zone.id);
            setIsZoneModalOpen(false);
            setFeedback({
              tone: 'success',
              message: editingZone ? '托管域名已保存。' : '托管域名已创建。',
            });
            await queryClient.invalidateQueries({
              queryKey: ['authoritative-dns', 'zones'],
            });
          }}
        />
      ) : null}

      {recordZone ? (
        <RecordEditorModal
          isOpen={Boolean(recordZone)}
          zone={recordZone}
          record={editingRecord}
          onClose={() => {
            setRecordZone(null);
            setEditingRecord(null);
          }}
          onSaved={async () => {
            setRecordZone(null);
            setEditingRecord(null);
            setFeedback({
              tone: 'success',
              message: editingRecord ? 'DNS 记录已保存。' : 'DNS 记录已创建。',
            });
            await Promise.all([
              queryClient.invalidateQueries({
                queryKey: ['authoritative-dns', 'zones'],
              }),
              queryClient.invalidateQueries({
                queryKey: ['authoritative-dns', 'zone-records'],
              }),
            ]);
          }}
        />
      ) : null}

      {isWorkerModalOpen ? (
        <WorkerCreateModal
          isOpen={isWorkerModalOpen}
          onClose={() => setIsWorkerModalOpen(false)}
          onCreated={async (worker) => {
            setIsWorkerModalOpen(false);
            setCreatedWorker(worker);
            setFeedback({ tone: 'success', message: 'DNS 响应端已创建。' });
            await queryClient.invalidateQueries({
              queryKey: ['authoritative-dns', 'workers'],
            });
          }}
        />
      ) : null}

      {createdWorker ? (
        <WorkerTokenModal
          worker={createdWorker}
          serverUrl={serverUrl}
          onClose={() => setCreatedWorker(null)}
        />
      ) : null}

      {workerSettingsTarget ? (
        <WorkerSettingsModal
          worker={workerSettingsTarget}
          isSaving={updateWorkerMutation.isPending}
          isRequestingUpdate={
            requestWorkerUpdateMutation.isPending &&
            requestWorkerUpdateMutation.variables === workerSettingsTarget.id
          }
          onClose={() => setWorkerSettingsTarget(null)}
          onSave={(values) =>
            updateWorkerMutation.mutate({
              id: workerSettingsTarget.id,
              values,
            })
          }
          onRequestUpdate={() =>
            requestWorkerUpdateMutation.mutate(workerSettingsTarget.id)
          }
        />
      ) : null}
    </>
  );
}

function ZonesPanel({
  zones,
  selectedZone,
  records,
  recordsLoading,
  recordsError,
  delegationCheck,
  delegationLoading,
  delegationError,
  busy,
  onSelectZone,
  onCreateZone,
  onEditZone,
  onDeleteZone,
  onCheckDelegation,
  onCreateRecord,
  onEditRecord,
  onDeleteRecord,
}: {
  zones: DNSZoneItem[];
  selectedZone: DNSZoneItem | null;
  records: DNSRecordItem[];
  recordsLoading: boolean;
  recordsError: string;
  delegationCheck: DNSZoneDelegationCheck | null;
  delegationLoading: boolean;
  delegationError: string;
  busy: boolean;
  onSelectZone: (zone: DNSZoneItem) => void;
  onCreateZone: () => void;
  onEditZone: (zone: DNSZoneItem) => void;
  onDeleteZone: (zone: DNSZoneItem) => void;
  onCheckDelegation: () => void;
  onCreateRecord: (zone: DNSZoneItem) => void;
  onEditRecord: (zone: DNSZoneItem, record: DNSRecordItem) => void;
  onDeleteRecord: (record: DNSRecordItem) => void;
}) {
  return (
    <AppCard
      title="托管域名与记录"
      description="托管域名用来承接注册商 NS 指向；静态记录和网站自动选 IP 记录会一起下发到 DNS 响应端。"
      action={
        <PrimaryButton type="button" onClick={onCreateZone}>
          创建托管域名
        </PrimaryButton>
      }
    >
      {zones.length === 0 ? (
        <EmptyState
          title="暂无托管域名"
          description="创建托管域名后，再到网站配置的负载均衡分区切换为本地自建解析。"
        />
      ) : (
        <div className="grid gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
          <div className="space-y-3">
            {zones.map((zone) => {
              const active = selectedZone?.id === zone.id;
              return (
                <button
                  key={zone.id}
                  type="button"
                  onClick={() => onSelectZone(zone)}
                  className={cn(
                    'w-full rounded-2xl border px-4 py-4 text-left transition',
                    active
                      ? 'border-[var(--border-strong)] bg-[var(--accent-soft)]'
                      : 'border-[var(--border-default)] bg-[var(--surface-elevated)] hover:border-[var(--border-strong)]',
                  )}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-[var(--foreground-primary)]">
                        {zone.name}
                      </p>
                      <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
                        Serial {zone.serial} · {zone.record_count} 条记录
                      </p>
                    </div>
                    <StatusBadge
                      label={zone.enabled ? '启用' : '停用'}
                      variant={zone.enabled ? 'success' : 'warning'}
                    />
                  </div>
                </button>
              );
            })}
          </div>

          {selectedZone ? (
            <div className="min-w-0 space-y-5">
              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
                <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                  <div className="min-w-0 space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="text-lg font-semibold text-[var(--foreground-primary)]">
                        {selectedZone.name}
                      </h2>
                      <StatusBadge
                        label={selectedZone.enabled ? '启用' : '停用'}
                        variant={selectedZone.enabled ? 'success' : 'warning'}
                      />
                    </div>
                    <p className="text-sm leading-6 text-[var(--foreground-secondary)]">
                      基础邮箱 {selectedZone.soa_email || 'hostmaster'}，默认缓存时间{' '}
                      {selectedZone.default_ttl} 秒。
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <SecondaryButton
                      type="button"
                      onClick={() => onEditZone(selectedZone)}
                    >
                      编辑托管域名
                    </SecondaryButton>
                    <DangerButton
                      type="button"
                      disabled={busy}
                      onClick={() => onDeleteZone(selectedZone)}
                    >
                      删除托管域名
                    </DangerButton>
                  </div>
                </div>

                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <InfoTile
                    label="主解析服务器"
                    value={selectedZone.primary_ns || '—'}
                  />
                  <InfoTile
                    label="更新时间"
                    value={formatDateTime(selectedZone.updated_at)}
                  />
                </div>

                <div className="mt-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
                  <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
                    注册商 NS
                  </p>
                  {selectedZone.name_servers.length > 0 ? (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {selectedZone.name_servers.map((nameServer) => (
                        <span
                          key={nameServer}
                          className="rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-1.5 text-xs text-[var(--foreground-primary)]"
                        >
                          {nameServer}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-2 text-sm text-[var(--foreground-secondary)]">
                      暂未配置注册商 NS。生产环境至少配置两个 DNS 响应端对应的 NS。
                    </p>
                  )}
                </div>
              </div>

              <DelegationCheckPanel
                zone={selectedZone}
                check={delegationCheck}
                isLoading={delegationLoading}
                error={delegationError}
                onCheck={onCheckDelegation}
              />

              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
                <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                  <div>
                    <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
                      静态记录
                    </h3>
                    <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
                      手动固定回答的记录；同名 A/AAAA/CNAME 会和网站配置里的本地自建解析自动记录互斥。
                    </p>
                  </div>
                  <PrimaryButton
                    type="button"
                    onClick={() => onCreateRecord(selectedZone)}
                  >
                    新增记录
                  </PrimaryButton>
                </div>

                <div className="mt-4">
                  {recordsLoading ? (
                    <LoadingState />
                  ) : recordsError ? (
                    <ErrorState
                      title="记录加载失败"
                      description={recordsError}
                    />
                  ) : records.length === 0 ? (
                    <EmptyState
                      title="暂无静态记录"
                      description="网站绑定本地自建解析后，A/AAAA 动态记录可由系统自动选择边缘 IP。"
                    />
                  ) : (
                    <div className="space-y-3">
                      {records.map((record) => (
                        <div
                          key={record.id}
                          className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4"
                        >
                          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                            <div className="min-w-0 space-y-2">
                              <div className="flex flex-wrap items-center gap-2">
                                <StatusBadge label={record.type} />
                                <StatusBadge
                                  label={record.enabled ? '启用' : '停用'}
                                  variant={
                                    record.enabled ? 'success' : 'warning'
                                  }
                                />
                                <p className="text-sm font-semibold break-all text-[var(--foreground-primary)]">
                                  {record.name}
                                </p>
                              </div>
                              <p className="text-sm break-all text-[var(--foreground-secondary)]">
                                {record.value}
                              </p>
                              <p className="text-xs text-[var(--foreground-muted)]">
                                TTL {record.ttl} 秒
                                {record.type === 'MX'
                                  ? ` · 优先级 ${record.priority}`
                                  : ''}
                              </p>
                            </div>
                            <div className="flex flex-wrap gap-2">
                              <SecondaryButton
                                type="button"
                                onClick={() =>
                                  onEditRecord(selectedZone, record)
                                }
                              >
                                编辑
                              </SecondaryButton>
                              <DangerButton
                                type="button"
                                disabled={busy}
                                onClick={() => onDeleteRecord(record)}
                              >
                                删除
                              </DangerButton>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>
          ) : null}
        </div>
      )}
    </AppCard>
  );
}

function DelegationCheckPanel({
  zone,
  check,
  isLoading,
  error,
  onCheck,
}: {
  zone: DNSZoneItem;
  check: DNSZoneDelegationCheck | null;
  isLoading: boolean;
  error: string;
  onCheck: () => void;
}) {
  const visibleCheck = check?.zone_id === zone.id ? check : null;

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
              委派检查
            </h3>
            {visibleCheck ? (
              <StatusBadge
                label={getDelegationStatusLabel(visibleCheck.status)}
                variant={getDelegationStatusVariant(visibleCheck.status)}
              />
            ) : null}
          </div>
          <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
            对比注册商当前公开的 NS，确认域名是否已经指向这里配置的解析服务器。
          </p>
        </div>
        <SecondaryButton type="button" disabled={isLoading} onClick={onCheck}>
          {isLoading ? '检查中...' : '检查委派'}
        </SecondaryButton>
      </div>

      <div className="mt-4">
        {error ? (
          <InlineMessage tone="danger" message={error} />
        ) : visibleCheck ? (
          <div className="space-y-4">
            {visibleCheck.error ? (
              <InlineMessage tone="danger" message={visibleCheck.error} />
            ) : null}
            {visibleCheck.glue_required ? (
              <InlineMessage
                tone="info"
                message={`主机记录提示：${visibleCheck.glue_name_servers.join('、')} 位于 ${zone.name} 内，需要在注册商配置主机记录，把这些 NS 名称对应到实际 IP，外部才能找到它们。`}
              />
            ) : null}
            <div className="grid gap-3 md:grid-cols-2">
              <NameServerList
                title="期望 NS"
                items={visibleCheck.expected_name_servers}
              />
              <NameServerList
                title="公网 NS"
                items={visibleCheck.actual_name_servers}
                emptyText={
                  visibleCheck.status === 'failed'
                    ? '查询失败。'
                    : '未查询到公网 NS。'
                }
              />
              <NameServerList
                title="缺失 NS"
                items={visibleCheck.missing_name_servers}
                emptyText="无缺失。"
              />
              <NameServerList
                title="额外 NS"
                items={visibleCheck.extra_name_servers}
                emptyText="无额外。"
              />
            </div>
            <p className="text-xs text-[var(--foreground-muted)]">
              检查时间：{formatDateTime(visibleCheck.checked_at)}
            </p>
          </div>
        ) : (
            <p className="text-sm text-[var(--foreground-secondary)]">
            点击后检查注册商是否已经把 {zone.name} 指向当前 NS。
          </p>
        )}
      </div>
    </div>
  );
}

function NameServerList({
  title,
  items,
  emptyText = '暂无。',
}: {
  title: string;
  items: string[];
  emptyText?: string;
}) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
        {title}
      </p>
      {items.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {items.map((item) => (
            <span
              key={`${title}-${item}`}
              className="rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-1.5 text-xs text-[var(--foreground-primary)]"
            >
              {item}
            </span>
          ))}
        </div>
      ) : (
        <p className="mt-2 text-sm text-[var(--foreground-secondary)]">
          {emptyText}
        </p>
      )}
    </div>
  );
}

function WorkersPanel({
  workers,
  busy,
  probingWorkerId,
  probeResults,
  onCreateWorker,
  onDeleteWorker,
  onProbeWorker,
}: {
  workers: DNSWorkerItem[];
  busy: boolean;
  probingWorkerId: number | null;
  probeResults: Record<number, DNSWorkerProbe>;
  onCreateWorker: () => void;
  onDeleteWorker: (worker: DNSWorkerItem) => void;
  onProbeWorker: (worker: DNSWorkerItem) => void;
}) {
  return (
    <AppCard
      title="DNS 响应端"
      description="响应端使用专属密钥拉取只读解析配置，并监听 UDP/TCP 53 对外回答 DNS 查询。"
      action={
        <PrimaryButton type="button" onClick={onCreateWorker}>
          创建 DNS 响应端
        </PrimaryButton>
      }
    >
      {workers.length === 0 ? (
        <EmptyState
          title="暂无 DNS 响应端"
          description="创建 DNS 响应端后复制部署命令，并在注册商处把托管域名的 NS 指向响应端地址。"
        />
      ) : (
        <div className="space-y-4">
          <InlineMessage
            tone={getWorkerSourceCapabilityTone(workers)}
            message={getWorkerSourceCapabilityMessage(workers)}
          />
          {workers.map((worker) => (
            <div
              key={worker.id}
              className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
            >
              <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                <div className="min-w-0 space-y-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="text-base font-semibold text-[var(--foreground-primary)]">
                      {worker.name}
                    </h2>
                    <StatusBadge
                      label={worker.status === 'online' ? '在线' : '离线'}
                      variant={getWorkerStatusVariant(worker.status)}
                    />
                    <StatusBadge
                      label={getProbeStatusLabel(worker.probe_status)}
                      variant={getProbeStatusVariant(worker.probe_status)}
                    />
                    <StatusBadge
                      label={worker.version || '未上报版本'}
                      variant="info"
                    />
                    <StatusBadge
                      label={
                        worker.geoip_enabled ? '国家识别库已加载' : '国家识别库未加载'
                      }
                      variant={worker.geoip_enabled ? 'success' : 'warning'}
                    />
                    <StatusBadge
                      label={worker.geoip_country_enabled ? '国家支持' : '国家未支持'}
                      variant={worker.geoip_country_enabled ? 'success' : 'warning'}
                    />
                    <StatusBadge
                      label={worker.geoip_asn_enabled ? 'ASN 支持' : 'ASN 未支持'}
                      variant={worker.geoip_asn_enabled ? 'success' : 'warning'}
                    />
                    <StatusBadge
                      label={worker.geoip_operator_enabled ? '运营商支持' : '运营商未支持'}
                      variant={worker.geoip_operator_enabled ? 'success' : 'warning'}
                    />
                  </div>
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
                    <InfoTile label="响应端 ID" value={worker.worker_id} />
                    <InfoTile
                      label="公网地址"
                      value={worker.public_address || '—'}
                    />
                    <InfoTile
                      label="最近心跳"
                      value={formatRelativeTime(worker.last_seen_at)}
                      helper={
                        isMeaningfulTime(worker.last_heartbeat_at)
                          ? `统计心跳 ${formatRelativeTime(worker.last_heartbeat_at)}`
                          : '尚未收到统计心跳'
                      }
                    />
                    <InfoTile
                      label="最近查询统计"
                      value={formatRelativeTime(worker.last_rollup_at)}
                      helper={
                        worker.last_rollup_count > 0
                          ? `上次上报 ${formatCount(worker.last_rollup_count)} 次查询`
                          : '尚未上报 DNS 查询 rollup'
                      }
                    />
                    <InfoTile
                      label="最近配置"
                      value={formatRelativeTime(worker.last_snapshot_at)}
                    />
                  </div>
                  <p className="text-xs leading-5 text-[var(--foreground-secondary)]">
                    配置版本：{worker.last_snapshot_version || '—'}
                    {worker.last_snapshot_at
                      ? ` · ${formatDateTime(worker.last_snapshot_at)}`
                      : ''}
                  </p>
                  {worker.last_error ? (
                    <InlineMessage tone="danger" message={worker.last_error} />
                  ) : null}
                  {worker.geoip_last_error ? (
                    <InlineMessage
                      tone="info"
                      message={`国家识别库加载失败：${worker.geoip_last_error}`}
                    />
                  ) : worker.asn_last_error ? (
                    <InlineMessage
                      tone="info"
                      message={`ASN 识别库加载失败：${worker.asn_last_error}`}
                    />
                  ) : worker.operator_cidr_last_error ? (
                    <InlineMessage
                      tone="info"
                      message={`运营商 CIDR 库加载失败：${worker.operator_cidr_last_error}`}
                    />
                  ) : !worker.geoip_enabled ? (
                    <InlineMessage
                      tone="info"
                      message="未加载国家识别库；按国家匹配节点池不会命中，仍可按来源网段或全局规则调度。"
                    />
                  ) : (
                    <div className="space-y-1 text-xs break-all text-[var(--foreground-secondary)]">
                      <p>国家识别库：{worker.geoip_database_path || '已加载'}</p>
                      <p>ASN 识别库：{worker.asn_database_path || '未配置独立 ASN 库'}</p>
                      <p>运营商 CIDR 库：{worker.operator_cidr_database_path || '未配置'}</p>
                    </div>
                  )}
                  {worker.probe_message ? (
                    <p className="text-xs text-[var(--foreground-secondary)]">
                      探测状态：{worker.probe_message}
                      {worker.probe_age_seconds > 0
                        ? ` · ${formatDurationSeconds(worker.probe_age_seconds)}前`
                        : ''}
                    </p>
                  ) : null}
                  <DNSWorkerProbeResultPanel
                    probe={
                      probeResults[worker.id] ?? workerProbeToPanelData(worker)
                    }
                  />
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <SecondaryButton
                    type="button"
                    disabled={
                      busy ||
                      probingWorkerId === worker.id ||
                      !worker.public_address
                    }
                    onClick={() => onProbeWorker(worker)}
                  >
                    {probingWorkerId === worker.id ? '探测中...' : '探测'}
                  </SecondaryButton>
                  <DangerButton
                    type="button"
                    disabled={busy}
                    onClick={() => onDeleteWorker(worker)}
                  >
                    删除
                  </DangerButton>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </AppCard>
  );
}

function DNSWorkerProbeResultPanel({ probe }: { probe?: DNSWorkerProbe }) {
  if (!probe) {
    return null;
  }

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-[var(--foreground-primary)]">
            最近探测
          </p>
          <p className="mt-1 text-xs text-[var(--foreground-muted)]">
            {probe.query_name} {probe.query_type} ·{' '}
            {formatDateTime(probe.checked_at)}
          </p>
        </div>
        <span className="text-xs text-[var(--foreground-secondary)]">
          {probe.public_address}
        </span>
      </div>
      <div className="mt-3 grid gap-2 sm:grid-cols-2">
        {probe.results.map((result) => (
          <div
            key={result.network}
            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-3"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <StatusBadge
                label={`${result.network} ${result.reachable ? '可达' : '失败'}`}
                variant={getProbeResultVariant(result)}
              />
              <span className="text-xs text-[var(--foreground-secondary)]">
                {formatLatencyMs(result.duration_ms)}
              </span>
            </div>
            <p className="mt-2 text-xs text-[var(--foreground-secondary)]">
              返回码：{result.rcode || '—'} · 应答 {result.answer_count}
            </p>
            {result.error ? (
              <p className="mt-2 text-xs leading-5 break-all text-[var(--status-danger-foreground)]">
                {result.error}
              </p>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

function GSLBSchedulingStatesPanel({
  states,
  isLoading,
  error,
}: {
  states: DNSGSLBSchedulingStates | null;
  isLoading: boolean;
  error: string;
}) {
  const [expanded, setExpanded] = useState(false);

  if (isLoading) {
    return (
      <AppCard title="智能解析状态">
        <LoadingState />
      </AppCard>
    );
  }

  if (error) {
    return <ErrorState title="智能解析状态加载失败" description={error} />;
  }

  const rows = states?.states ?? [];
  const debouncingCount = rows.filter(
    (item) => item.status === 'debouncing',
  ).length;
  const activeCount = rows.filter((item) => item.status === 'active').length;

  return (
    <AppCard
      title="智能解析状态"
      description="展示响应端和面板记录的当前返回 IP、期望返回 IP，以及不同访问来源的切换冷却状态。"
      action={
        rows.length > 0 ? (
          <SecondaryButton
            type="button"
            onClick={() => setExpanded((current) => !current)}
          >
            {expanded ? '收起状态' : '展开状态'}
          </SecondaryButton>
        ) : null
      }
    >
      {rows.length === 0 ? (
        <EmptyState
          title="暂无调度状态"
          description="本地自建解析站点收到 A/AAAA 查询，或 Cloudflare 模式触发自动选 IP 后，这里会显示当前选中的边缘 IP。"
        />
      ) : (
        <div className="space-y-4">
          <div className="grid gap-3 md:grid-cols-3">
            <InfoTile
              label="状态条目"
              value={formatCount(states?.total ?? rows.length)}
            />
            <InfoTile label="已生效" value={formatCount(activeCount)} />
            <InfoTile label="冷却中" value={formatCount(debouncingCount)} />
          </div>
          {expanded ? (
            <div className="grid gap-3 xl:grid-cols-2">
              {rows.map((state) => (
                <GSLBSchedulingStateCard key={state.id} state={state} />
              ))}
            </div>
          ) : (
            <InlineMessage
              tone="info"
              message="状态明细已折叠，点击“展开状态”查看每个访问来源当前返回的 IP。"
            />
          )}
          {states?.checked_at ? (
            <p className="text-xs text-[var(--foreground-muted)]">
              检查时间：{formatDateTime(states.checked_at)}
            </p>
          ) : null}
        </div>
      )}
    </AppCard>
  );
}

function GSLBSchedulingStateCard({ state }: { state: DNSGSLBSchedulingState }) {
  const selectedText = state.selected_targets.join(', ') || '—';
  const desiredText = state.desired_targets.join(', ') || selectedText;

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold break-all text-[var(--foreground-primary)]">
              {state.site_name ||
                state.primary_domain ||
                `Route ${state.proxy_route_id}`}
            </h3>
            <StatusBadge label={state.record_type} variant="info" />
            <StatusBadge
              label={getSchedulingStateStatusLabel(state.status)}
              variant={getSchedulingStateStatusVariant(state.status)}
            />
          </div>
          <p className="mt-1 text-xs break-all text-[var(--foreground-secondary)]">
            {state.primary_domain || state.domains.join('、') || '站点已删除'}
          </p>
        </div>
        {state.proxy_route_id > 0 ? (
          <a
            href={`${proxyRouteDetailPath}?id=${state.proxy_route_id}`}
            className="inline-flex shrink-0 items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-3 py-2 text-xs font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
          >
            网站详情
          </a>
        ) : null}
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <InfoTile
          label="访问来源/分流组"
          value={formatSourceScopeLabel(state.scope_key)}
          helper={state.scope_key}
        />
        <InfoTile
          label="最近评估"
          value={formatRelativeTime(state.last_evaluated_at)}
        />
        <InfoTile label="当前返回 IP" value={selectedText} />
        <InfoTile label="期望返回 IP" value={desiredText} />
      </div>

      {state.status === 'debouncing' ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message="期望返回 IP 已变化，但当前返回 IP 仍处于切换冷却，或旧 IP 仍健康，所以暂未切换。"
        />
      ) : null}
      {state.last_reason ? (
        <p className="mt-3 text-xs leading-5 break-all text-[var(--foreground-secondary)]">
          {formatDiagnosticMessage(state.last_reason)}
        </p>
      ) : null}
      <p className="mt-3 text-xs text-[var(--foreground-muted)]">
        最近切换：{formatRelativeTime(state.last_changed_at)}
      </p>
    </div>
  );
}

function GSLBSimulationPanel({
  routes,
  routesLoading,
  routesError,
  result,
  error,
  isPending,
  onSimulate,
}: {
  routes: ProxyRouteItem[];
  routesLoading: boolean;
  routesError: string;
  result: DNSGSLBSimulationResult | null;
  error: string;
  isPending: boolean;
  onSimulate: (payload: DNSGSLBSimulationPayload) => void;
}) {
  const defaultRoute = routes[0] ?? null;
  const form = useForm<GSLBSimulationFormValues>({
    defaultValues: {
      proxy_route_id: defaultRoute ? String(defaultRoute.id) : '',
      qname: getDefaultSimulationQName(defaultRoute),
      record_type: getRouteRecordType(defaultRoute),
      country: '',
      operator: '',
      asn: '',
      source_ip: '',
    },
  });
  const selectedRouteId = Number(form.watch('proxy_route_id'));
  const selectedRoute =
    routes.find((route) => route.id === selectedRouteId) ?? defaultRoute;

  useEffect(() => {
    if (!selectedRoute) {
      form.reset({
        proxy_route_id: '',
        qname: '',
        record_type: 'A',
        country: '',
        operator: '',
        asn: '',
        source_ip: '',
      });
      return;
    }
    const currentRouteID = form.getValues('proxy_route_id');
    if (currentRouteID) {
      return;
    }
    form.reset({
      proxy_route_id: String(selectedRoute.id),
      qname: getDefaultSimulationQName(selectedRoute),
      record_type: getRouteRecordType(selectedRoute),
      country: '',
      operator: '',
      asn: '',
      source_ip: '',
    });
  }, [form, selectedRoute]);

  const handleRouteChange = (routeId: string) => {
    const nextRoute = routes.find((route) => String(route.id) === routeId);
    form.setValue('proxy_route_id', routeId, { shouldDirty: true });
    if (nextRoute) {
      form.setValue('qname', getDefaultSimulationQName(nextRoute), {
        shouldDirty: true,
      });
      form.setValue('record_type', getRouteRecordType(nextRoute), {
        shouldDirty: true,
      });
    }
  };

  return (
    <AppCard
      title="智能解析模拟"
      description="按站点、记录类型、来源国家、运营商、ASN 和来源 IP，预演当前解析配置会返回哪些边缘 IP。"
    >
      {routesLoading ? (
        <LoadingState />
      ) : routesError ? (
        <ErrorState title="智能解析模拟加载失败" description={routesError} />
      ) : routes.length === 0 ? (
        <EmptyState
          title="暂无本地自建解析站点"
          description="把网站配置的负载均衡切换为本地自建解析后，可在这里模拟系统会返回哪个边缘 IP。"
        />
      ) : (
        <div className="grid gap-5 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
          <form
            className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] p-4"
            onSubmit={form.handleSubmit((values) => {
              const asn = parseSimulationASN(values.asn);
              onSimulate({
                proxy_route_id: Number(values.proxy_route_id),
                qname: values.qname.trim(),
                record_type: values.record_type,
                country: values.country.trim().toUpperCase(),
                operator: values.operator.trim(),
                asn,
                source_ip: values.source_ip.trim(),
                fresh: true,
              });
            })}
          >
            <ResourceField label="网站配置">
              <ResourceSelect
                value={form.watch('proxy_route_id')}
                onChange={(event) => handleRouteChange(event.target.value)}
              >
                {routes.map((route) => (
                  <option key={route.id} value={route.id}>
                    {getRouteDisplayName(route)}
                  </option>
                ))}
              </ResourceSelect>
            </ResourceField>
            <ResourceField label="查询域名">
              <ResourceInput
                placeholder="www.example.com"
                {...form.register('qname', { required: true })}
              />
            </ResourceField>
            <div className="grid gap-4 md:grid-cols-2">
              <ResourceField label="记录类型">
                <ResourceSelect {...form.register('record_type')}>
                  <option value="A">A</option>
                  <option value="AAAA">AAAA</option>
                </ResourceSelect>
              </ResourceField>
              <ResourceField
                label="来源国家"
                hint="例如 HK、DE；留空使用全局。"
              >
                <ResourceInput
                  maxLength={2}
                  placeholder="HK"
                  {...form.register('country')}
                />
              </ResourceField>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <ResourceField
                label="访问运营商"
                hint="可选；本地 DNS 响应端配置离线 ISP/ASN 库后生效。"
              >
                <ResourceSelect
                  aria-label="访问运营商"
                  {...form.register('operator')}
                >
                  <option value="">全局</option>
                  {gslbSimulationOperatorOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </ResourceSelect>
              </ResourceField>
              <ResourceField
                label="来源 ASN"
                hint="可选；优先级高于运营商，例如 AS4134。"
              >
                <ResourceInput
                  aria-label="来源 ASN"
                  placeholder="AS4134"
                  {...form.register('asn')}
                />
              </ResourceField>
            </div>
            <ResourceField
              label="来源 IP"
              hint="可选；填写后会优先按来源网段规则预演。"
            >
              <ResourceInput
                placeholder="203.0.113.10"
                {...form.register('source_ip')}
              />
            </ResourceField>
            {selectedRoute ? (
              <div className="grid gap-3 md:grid-cols-2">
                <InfoTile
                  label="多节点智能解析"
                  value={selectedRoute.gslb_enabled ? '已启用' : '未启用'}
                />
                <InfoTile
                  label="节点池"
                  value={getGSLBDescription(selectedRoute)}
                />
              </div>
            ) : null}
            <PrimaryButton type="submit" disabled={isPending}>
              {isPending ? '模拟中...' : '开始模拟'}
            </PrimaryButton>
          </form>

          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] p-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              模拟结果
            </h3>
            {error ? (
              <InlineMessage className="mt-3" tone="danger" message={error} />
            ) : result ? (
              <div className="mt-4 space-y-4">
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                  <InfoTile label="站点" value={result.site_name || '—'} />
                  <InfoTile
                    label="访问来源/分流组"
                    value={formatSourceScopeLabel(result.source_scope)}
                    helper={result.source_scope}
                  />
                  <InfoTile label="缓存时间" value={`${result.ttl} 秒`} />
                  <InfoTile
                    label="策略"
                    value={
                      result.strategy ||
                      (result.gslb_enabled ? '智能解析' : '节点池')
                    }
                  />
                </div>
                {result.targets.length > 0 ? (
                  <div className="space-y-2">
                    <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
                      返回 IP
                    </p>
                    <div className="flex flex-wrap gap-2">
                      {result.targets.map((target) => (
                        <span
                          key={target}
                          className="rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-1.5 text-xs text-[var(--foreground-primary)]"
                        >
                          {target}
                        </span>
                      ))}
                    </div>
                  </div>
                ) : (
                  <InlineMessage tone="danger" message="当前没有可返回目标。" />
                )}
                <p className="text-xs leading-5 text-[var(--foreground-secondary)]">
                  {result.qname} {result.record_type} · 配置{' '}
                  {result.snapshot_version || '—'} ·{' '}
                  {formatDateTime(result.snapshot_at)}
                </p>
                {result.message ? (
                  <InlineMessage
                    tone={
                      isProbeSchedulingGateMessage(result.message)
                        ? 'warning'
                        : 'info'
                    }
                    message={formatDiagnosticMessage(result.message)}
                  />
                ) : null}
                <GSLBSimulationDiagnostics result={result} />
              </div>
            ) : (
              <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
                选择站点和来源后点击模拟，可看到 DNS 响应端当前会返回的 A/AAAA
                IP。
              </p>
            )}
          </div>
        </div>
      )}
    </AppCard>
  );
}

function GSLBSimulationDiagnostics({
  result,
}: {
  result: DNSGSLBSimulationResult;
}) {
  return (
    <div className="space-y-4">
      {result.matched_pools.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
            节点池匹配
          </p>
          <div className="grid gap-2 md:grid-cols-2">
            {result.matched_pools.map((pool) => (
              <GSLBSimulationPoolCard key={pool.name} pool={pool} />
            ))}
          </div>
        </div>
      ) : null}

      {result.nodes.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
            节点诊断
          </p>
          <div className="grid gap-2 xl:grid-cols-2">
            {result.nodes.map((node) => (
              <div
                key={node.node_id}
                className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-3"
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-[var(--foreground-primary)]">
                      {node.name || node.node_id}
                    </p>
                    <p className="mt-1 text-xs text-[var(--foreground-muted)]">
                      {node.pool_name} · {node.node_id}
                    </p>
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <StatusBadge
                      label={
                        node.selected
                          ? '已选中'
                          : node.eligible
                            ? '候选'
                            : '跳过'
                      }
                      variant={
                        node.selected
                          ? 'success'
                          : node.eligible
                            ? 'info'
                            : 'warning'
                      }
                    />
                    <StatusBadge label={node.status || 'unknown'} />
                  </div>
                </div>
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                  <InfoTile
                    label="可返回 IP"
                    value={node.candidate_targets.join(', ') || '—'}
                  />
                  <InfoTile
                    label="已选 IP"
                    value={node.selected_targets.join(', ') || '—'}
                  />
                  <InfoTile
                    label="连接数"
                    value={
                      node.has_metric
                        ? formatCount(node.openresty_connections)
                        : '无负载数据'
                    }
                  />
                  <InfoTile
                    label="负载数据时间"
                    value={
                      node.has_metric
                        ? formatRelativeTime(node.metric_captured_at)
                        : '无新负载数据'
                    }
                    helper={
                      node.has_metric
                        ? formatDateTime(node.metric_captured_at)
                        : undefined
                    }
                  />
                  <InfoTile
                    label="权重评分"
                    value={node.score > 0 ? node.score.toFixed(2) : '—'}
                  />
                  <InfoTile
                    label="响应端探测"
                    value={`${node.node_probe_healthy_count ?? 0} / ${
                      node.node_probe_checked_count ?? 0
                    }`}
                    helper={
                      (node.node_probe_stale_count ?? 0) > 0
                        ? `${node.node_probe_stale_count} 个过期`
                        : node.node_probe_message || undefined
                    }
                  />
                  <InfoTile
                    label="探测耗时"
                    value={formatLatencyMs(node.node_probe_average_rtt_ms ?? 0)}
                    helper={
                      (node.node_probe_max_rtt_ms ?? 0) > 0
                        ? `最大 ${formatLatencyMs(node.node_probe_max_rtt_ms)}`
                        : undefined
                    }
                  />
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <StatusBadge
                    label={getNodeDNSProbeStatusLabel(
                      node.node_probe_status ?? 'unknown',
                    )}
                    variant={getProbeStatusVariant(
                      node.node_probe_status ?? 'unknown',
                    )}
                  />
                  {node.node_probe_message ? (
                    <span className="text-xs text-[var(--foreground-secondary)]">
                      {formatDiagnosticMessage(node.node_probe_message)}
                    </span>
                  ) : null}
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {node.reasons.map((reason) => (
                    <span
                      key={`${node.node_id}-${reason}`}
                      className="rounded-xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-2.5 py-1 text-xs text-[var(--foreground-secondary)]"
                    >
                      {formatDiagnosticMessage(reason)}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function GSLBSimulationPoolCard({
  pool,
}: {
  pool: DNSGSLBSimulationResult['matched_pools'][number];
}) {
  const countries = pool.countries ?? [];
  const sourceCIDRs = pool.source_cidrs ?? [];
  const operators = pool.operators ?? [];
  const asns = pool.asns ?? [];

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-medium text-[var(--foreground-primary)]">
          {pool.name}
        </span>
        <StatusBadge
          label={pool.matched ? '参与' : '跳过'}
          variant={pool.matched ? 'success' : 'warning'}
        />
      </div>
      <p className="mt-2 text-xs text-[var(--foreground-secondary)]">
        权重 {pool.weight}
        {countries.length > 0 ? ` · 国家 ${countries.join(', ')}` : ''}
        {operators.length > 0
          ? ` · 运营商 ${operators.map(formatGSLBOperatorLabel).join(', ')}`
          : ''}
        {asns.length > 0 ? ` · ASN ${asns.map((asn) => `AS${asn}`).join(', ')}` : ''}
        {sourceCIDRs.length > 0 ? ` · 来源网段 ${sourceCIDRs.join(', ')}` : ''}
      </p>
      <p className="mt-1 text-xs text-[var(--foreground-muted)]">
        {pool.reason}
      </p>
    </div>
  );
}

function DNSMigrationGuidePanel({
  routes,
  migrationCandidates,
  zones,
  workers,
  routesLoading,
  routesError,
  switchingRouteId,
  recheckingRouteId,
  recheckResult,
  onSwitchAuthoritative,
}: {
  routes: ProxyRouteItem[];
  migrationCandidates: AuthoritativeDNSMigrationCandidate[];
  zones: DNSZoneItem[];
  workers: DNSWorkerItem[];
  routesLoading: boolean;
  routesError: string;
  switchingRouteId: number | null;
  recheckingRouteId: number | null;
  recheckResult: MigrationRecheckResult | null;
  onSwitchAuthoritative: (route: ProxyRouteItem, zone: DNSZoneItem) => void;
}) {
  const enabledZones = zones.filter((zone) => zone.enabled);
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  const publicReachableWorkers = onlineWorkers.filter(
    (worker) => worker.probe_healthy,
  );
  const routeById = new Map(routes.map((route) => [route.id, route]));
  const zoneById = new Map(zones.map((zone) => [zone.id, zone]));
  const candidates = migrationCandidates.map((candidate) => ({
    candidate,
    route: routeById.get(candidate.proxy_route_id) ?? null,
    matchingZone:
      candidate.matching_zone_id != null
        ? (zoneById.get(candidate.matching_zone_id) ?? null)
        : null,
  }));
  const authoritativeRoutes = routes.filter(
    (route) => route.dns_provider_mode === 'authoritative',
  );

  if (routesLoading) {
    return (
      <AppCard title="迁移向导">
        <LoadingState />
      </AppCard>
    );
  }

  if (routesError) {
    return <ErrorState title="迁移向导加载失败" description={routesError} />;
  }

  return (
    <AppCard
      title="迁移向导"
      description="把 Cloudflare 同步站点切换到本地自建解析前，先检查托管域名、DNS 响应端、域名归属和多节点解析配置。"
    >
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <InfoTile label="待迁移站点" value={formatCount(candidates.length)} />
        <InfoTile
          label="已用自建解析"
          value={formatCount(authoritativeRoutes.length)}
        />
        <InfoTile
          label="可用托管域名"
          value={`${enabledZones.length} / ${zones.length}`}
        />
        <InfoTile
          label="在线响应端"
          value={`${onlineWorkers.length} / ${workers.length}`}
        />
        <InfoTile
          label="公网可达"
          value={`${publicReachableWorkers.length} / ${onlineWorkers.length}`}
        />
      </div>

      <div className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-4">
          {candidates.length === 0 ? (
            <EmptyState
              title="暂无需要迁移的网站"
              description="当前网站配置已经没有 Cloudflare 同步候选，或仍未创建网站配置。"
            />
          ) : (
            candidates.map(({ candidate, route, matchingZone }) => {
              const ready = candidate.ready && Boolean(route && matchingZone);
              const routeDomains =
                candidate.domains ?? (route ? getRouteDomains(route) : []);
              return (
                <div
                  key={candidate.proxy_route_id}
                  className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
                >
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0 space-y-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-base font-semibold break-all text-[var(--foreground-primary)]">
                          {candidate.site_name || candidate.primary_domain}
                        </h3>
                        <StatusBadge
                          label={ready ? '可切换' : '需处理'}
                          variant={ready ? 'success' : 'warning'}
                        />
                        <StatusBadge
                          label={
                            candidate.dns_auto_sync
                              ? 'Cloudflare 自动解析'
                              : 'Cloudflare 模式'
                          }
                          variant={candidate.dns_auto_sync ? 'info' : 'warning'}
                        />
                        {candidate.gslb_enabled ? (
                          <StatusBadge label="多节点解析已启用" variant="success" />
                        ) : null}
                      </div>
                      <p className="text-sm break-all text-[var(--foreground-secondary)]">
                        域名：{routeDomains.join('、') || '—'}
                      </p>
                      <div className="grid gap-3 md:grid-cols-3">
                        <InfoTile
                          label="匹配托管域名"
                          value={
                            candidate.matching_zone_name
                              ? `匹配 ${candidate.matching_zone_name}`
                              : '未匹配'
                          }
                        />
                        <InfoTile
                          label="当前节点池"
                          value={route ? getGSLBDescription(route) : '—'}
                        />
                        <InfoTile
                          label="记录类型"
                          value={getCandidateRecordType(candidate)}
                        />
                        <InfoTile
                          label="公网可达响应端"
                          value={`${candidate.public_reachable_worker_count} / ${candidate.online_worker_count}`}
                        />
                        <InfoTile
                          label="配置就绪响应端"
                          value={`${candidate.ready_worker_count} / ${candidate.public_reachable_worker_count}`}
                        />
                      </div>
                      {candidate.blockers.length > 0 ? (
                        <CheckList
                          title="阻断项"
                          items={candidate.blockers}
                          tone="danger"
                        />
                      ) : (
                        <InlineMessage
                          tone="success"
                          message="托管域名、域名归属、公网 UDP/TCP 53 探测和响应端解析配置都已满足切换条件。"
                        />
                      )}
                      {candidate.warnings.length > 0 ? (
                        <CheckList
                          title="建议项"
                          items={candidate.warnings}
                          tone="info"
                        />
                      ) : null}
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-2 md:justify-end">
                      {matchingZone && route ? (
                        <PrimaryButton
                          type="button"
                          disabled={
                            !ready ||
                            switchingRouteId === candidate.proxy_route_id ||
                            recheckingRouteId === candidate.proxy_route_id
                          }
                          onClick={() =>
                            onSwitchAuthoritative(route, matchingZone)
                          }
                        >
                          {switchingRouteId === candidate.proxy_route_id
                            ? '切换中...'
                            : recheckingRouteId === candidate.proxy_route_id
                              ? '复测中...'
                              : '一键切换'}
                        </PrimaryButton>
                      ) : null}
                      <a
                        href={
                          route
                            ? getRouteDetailHref(route)
                            : `${proxyRouteDetailPath}?id=${candidate.proxy_route_id}`
                        }
                        className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
                      >
                        去网站详情
                      </a>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>

        <div className="space-y-4">
          {recheckResult ? (
            <MigrationRecheckPanel result={recheckResult} />
          ) : null}
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              切换顺序
            </h3>
            <ol className="mt-3 space-y-3 text-sm leading-6 text-[var(--foreground-secondary)]">
              <li>1. 创建覆盖网站域名的托管域名，并填写注册商要使用的 NS。</li>
              <li>
                2. 部署至少两个 DNS 响应端，确认响应端在线、能拉取解析配置，并通过公网 UDP/TCP 53 探测。
              </li>
              <li>
                3. 在网站详情的「负载均衡」里切换为本地自建解析并选择托管域名。
              </li>
              <li>
                4. 到注册商把域名 NS 指向 DNS 响应端，再回到托管域名详情检查指向。
              </li>
            </ol>
          </div>
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              回滚路径
            </h3>
            <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
              如需回退，在网站详情把解析模式改回 Cloudflare 同步，并在注册商把
              NS 改回原 DNS 服务商；解析缓存时间到期后解析会逐步回到原模式。
            </p>
          </div>
        </div>
      </div>
    </AppCard>
  );
}

function MigrationRecheckPanel({ result }: { result: MigrationRecheckResult }) {
  const healthyProbeCount = result.workerProbes.filter(
    (probe) =>
      probe.results.length > 0 &&
      probe.results.every((probeResult) => probeResult.reachable),
  ).length;
  const targetCount = result.simulations.reduce(
    (count, simulation) => count + simulation.targets.length,
    0,
  );
  const statusLabel =
    result.status === 'running'
      ? '复测中'
      : result.status === 'success'
        ? '复测通过'
        : result.status === 'warning'
          ? '需要确认'
          : '复测异常';

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
            切换后复测
          </h3>
          <p className="mt-1 text-xs break-all text-[var(--foreground-secondary)]">
            {result.routeName} · 托管域名 {result.zoneName}
          </p>
        </div>
        <StatusBadge
          label={statusLabel}
          variant={
            result.status === 'success'
              ? 'success'
              : result.status === 'danger'
                ? 'danger'
                : result.status === 'running'
                  ? 'info'
                  : 'warning'
          }
        />
      </div>

      <div className="mt-4 space-y-2">
        {result.steps.map((step) => (
          <div
            key={step.key}
            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-3 py-3"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium text-[var(--foreground-primary)]">
                {step.label}
              </span>
              <StatusBadge
                label={getRecheckStepStatusLabel(step.status)}
                variant={getRecheckStepBadgeVariant(step.status)}
              />
            </div>
            <p className="mt-2 text-xs leading-5 text-[var(--foreground-secondary)]">
              {step.message}
            </p>
          </div>
        ))}
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <InfoTile
          label="委派状态"
          value={
            result.delegationCheck
              ? getDelegationStatusLabel(result.delegationCheck.status)
              : '—'
          }
        />
        <InfoTile
          label="响应端探测"
          value={`${healthyProbeCount} / ${result.workerProbes.length}`}
        />
        <InfoTile
          label="模拟组数"
          value={formatCount(result.simulations.length)}
        />
        <InfoTile label="返回目标" value={formatCount(targetCount)} />
      </div>

      {result.delegationCheck?.missing_name_servers.length ? (
        <div className="mt-4">
          <CheckList
            title="缺失 NS"
            items={result.delegationCheck.missing_name_servers}
            tone="info"
          />
        </div>
      ) : null}

      {result.simulations.length > 0 ? (
        <div className="mt-4 space-y-2">
          <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
            模拟结果
          </p>
          <div className="space-y-2">
            {result.simulations.map((simulation, index) => (
              <div
                key={`${simulation.proxy_route_id}-${simulation.source_scope}-${simulation.record_type}-${index}`}
                className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-3 py-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium text-[var(--foreground-primary)]">
                    {formatSourceScopeLabel(simulation.source_scope)}
                  </span>
                  <StatusBadge
                    label={simulation.targets.length > 0 ? '有目标' : '无目标'}
                    variant={
                      simulation.targets.length > 0 ? 'success' : 'danger'
                    }
                  />
                </div>
                <p className="mt-2 text-xs break-all text-[var(--foreground-secondary)]">
                  {simulation.qname} {simulation.record_type} ·{' '}
                  {simulation.targets.join(', ') || '—'}
                </p>
                {simulation.message ? (
                  <p className="mt-1 text-xs leading-5 text-[var(--foreground-muted)]">
                    {simulation.message}
                  </p>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      ) : null}

      <InlineMessage
        className="mt-4"
        tone={getMigrationRecheckTone(result.status)}
        message={
          result.status === 'success'
            ? '切换、注册商指向、响应端探测和智能解析模拟都已通过。'
            : '复测会帮助确认切换状态；指向不匹配时仍需要到注册商调整 NS 或主机记录。'
        }
      />
      <p className="mt-3 text-xs text-[var(--foreground-muted)]">
        最近复测：{formatDateTime(result.checkedAt)}
      </p>
    </div>
  );
}

function CheckList({
  title,
  items,
  tone,
}: {
  title: string;
  items: string[];
  tone: 'danger' | 'info';
}) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
        {title}
      </p>
      <ul className="mt-2 space-y-1.5 text-sm leading-6 text-[var(--foreground-secondary)]">
        {items.map((item) => (
          <li key={`${title}-${item}`} className="flex gap-2">
            <span
              className={cn(
                'mt-2 h-1.5 w-1.5 shrink-0 rounded-full',
                tone === 'danger'
                  ? 'bg-[var(--status-danger-foreground)]'
                  : 'bg-[var(--status-info-foreground)]',
              )}
            />
            <span>{item}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function DNSObservabilityPanel({
  summary,
  isLoading,
  error,
  onCopyCommand,
  onOpenWorkerSettings,
}: {
  summary: DNSObservabilitySummary | null;
  isLoading: boolean;
  error: string;
  onCopyCommand: (value: string, message: string) => void;
  onOpenWorkerSettings?: (worker: DNSWorkerHealthItem) => void;
}) {
  if (isLoading) {
    return (
      <AppCard title="DNS 查询观测">
        <LoadingState />
      </AppCard>
    );
  }

  if (error) {
    return <ErrorState title="DNS 查询观测加载失败" description={error} />;
  }

  if (!summary) {
    return (
      <AppCard
        title="DNS 查询观测"
        description={`最近 ${dnsObservabilityWindowHours} 小时的响应端上报汇总。`}
      >
        <EmptyState
          title="暂无 DNS 查询数据"
          description="DNS 响应端收到查询并上报后，这里会展示查询量、错误码和返回目标分布。"
        />
      </AppCard>
    );
  }

  const rollupHint = getDNSObservabilityRollupHint(summary);

  return (
    <AppCard
      title="DNS 查询观测"
      description={`最近 ${summary.window_hours} 小时聚合查询；最近上报 ${formatRelativeTime(summary.last_rollup_at)}。`}
    >
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <InfoTile label="查询量" value={formatCount(summary.total_queries)} />
        <InfoTile
          label="成功率"
          value={formatPercent(
            summary.successful_queries,
            summary.total_queries,
          )}
        />
        <InfoTile
          label="动态解析"
          value={formatCount(summary.dynamic_queries)}
        />
        <InfoTile label="错误查询" value={formatCount(summary.error_queries)} />
      </div>
      {rollupHint ? (
        <InlineMessage
          className="mt-4"
          tone="warning"
          message={rollupHint}
        />
      ) : null}

      <div className="mt-5 grid gap-4 xl:grid-cols-2">
        <DNSQueryTrendPanel summary={summary} />
        <DNSSnapshotConsistencyPanel
          consistency={summary.snapshot_consistency}
          onCopyCommand={onCopyCommand}
        />
        <DNSWorkerHealthPanel
          summary={summary}
          onOpenWorkerSettings={onOpenWorkerSettings}
        />
        <CounterChart
          title="返回码"
          items={summary.rcode_breakdown}
          total={summary.total_queries}
          formatLabel={formatDNSRCodeLabel}
          color="#2563eb"
        />
        <CounterChart
          title="返回目标"
          items={summary.top_targets}
          total={summary.dynamic_queries || summary.total_queries}
          emptyText="暂无 A/AAAA 目标分布。"
          color="#0ea5e9"
        />
        <CounterChart
          title="响应端查询"
          items={summary.worker_breakdown}
          total={summary.total_queries}
          color="#22c55e"
        />
        <CounterChart
          title="动态站点"
          items={summary.route_breakdown}
          total={summary.dynamic_queries}
          emptyText="暂无动态解析站点查询。"
          color="#f59e0b"
        />
        <CounterChart
          title="来源作用域"
          items={summary.source_scope_breakdown}
          total={summary.total_queries}
          emptyText="暂无来源作用域分布。"
          formatLabel={(item) => formatSourceScopeLabel(item.key || item.label)}
          color="#14b8a6"
        />
      </div>
    </AppCard>
  );
}

function DNSWorkerHealthPanel({
  summary,
  onOpenWorkerSettings,
}: {
  summary: DNSObservabilitySummary;
  onOpenWorkerSettings?: (worker: DNSWorkerHealthItem) => void;
}) {
  const health = summary.worker_health;

  if (!health) {
    return (
      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4 xl:col-span-2">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          响应端可用性
        </h3>
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          暂无响应端可用性数据。
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4 xl:col-span-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
            响应端可用性
          </h3>
          <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
            这里统计 DNS 响应端本地处理查询的耗时、错误率和解析配置新鲜度；不代表用户到各地 NS 的公网网络耗时。
          </p>
        </div>
        <StatusBadge
          label={`${health.online_worker_count} / ${health.total_worker_count} 在线`}
          variant={
            health.online_worker_count === health.total_worker_count &&
            health.total_worker_count > 0
              ? 'success'
              : 'warning'
          }
        />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <InfoTile
          label="可用率"
          value={formatPercentValue(health.availability_percent)}
        />
        <InfoTile
          label="公网探测通过"
          value={`${health.probe_healthy_count} / ${health.probe_checked_count || health.total_worker_count}`}
        />
        <InfoTile
          label="多节点探测通过"
          value={`${health.node_probe_healthy_count ?? 0} / ${health.node_probe_checked_count ?? 0}`}
          helper={
            (health.node_probe_stale_count ?? 0) > 0
              ? `${health.node_probe_stale_count} 个过期`
              : undefined
          }
        />
        <InfoTile
          label="多节点平均耗时"
          value={formatLatencyMs(health.node_probe_average_rtt_ms ?? 0)}
        />
        <InfoTile
          label="多节点最大耗时"
          value={formatLatencyMs(health.node_probe_max_rtt_ms ?? 0)}
        />
        <InfoTile
          label="平均延迟"
          value={formatLatencyMs(health.average_latency_ms)}
        />
        <InfoTile
          label="最大延迟"
          value={formatLatencyMs(health.max_latency_ms)}
        />
        <InfoTile
          label="错误率"
          value={formatPercentValue(health.error_rate_percent)}
        />
      </div>

      {health.workers.length === 0 ? (
        <p className="mt-4 text-sm text-[var(--foreground-secondary)]">
          暂无 DNS 响应端。
        </p>
      ) : (
        <div className="mt-4 grid gap-3 lg:grid-cols-2">
          {health.workers.map((worker) => (
            <DNSWorkerHealthCard
              key={worker.worker_id}
              worker={worker}
              onOpenSettings={onOpenWorkerSettings}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function DNSWorkerHealthCard({
  worker,
  onOpenSettings,
}: {
  worker: DNSWorkerHealthItem;
  onOpenSettings?: (worker: DNSWorkerHealthItem) => void;
}) {
  const visibleNodeProbes = (worker.node_probes ?? []).filter((probe) =>
    probe.node_name?.trim(),
  );
  const isWaitingForUnsupportedUpdate =
    worker.update_requested && !worker.update_supported;

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-[var(--foreground-primary)]">
            {worker.name}
          </p>
          <p className="mt-1 text-xs break-all text-[var(--foreground-muted)]">
            {worker.public_address || worker.worker_id}
          </p>
          {worker.remark ? (
            <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
              {worker.remark}
            </p>
          ) : null}
          <div className="mt-2 flex flex-wrap gap-2">
            <StatusBadge
              label={worker.status}
              variant={getWorkerStatusVariant(worker.status)}
            />
            <StatusBadge
              label={getProbeStatusLabel(worker.probe_status)}
              variant={getProbeStatusVariant(worker.probe_status)}
            />
            {worker.snapshot_stale ? (
              <StatusBadge label="解析配置过期" variant="danger" />
            ) : null}
            {worker.update_requested ? (
              <StatusBadge
                label={isWaitingForUnsupportedUpdate ? '需手动升级' : '等待更新'}
                variant="warning"
              />
            ) : null}
            <StatusBadge
              label={worker.geoip_enabled ? '国家识别库已加载' : '国家识别库未加载'}
              variant={worker.geoip_enabled ? 'success' : 'warning'}
            />
            <StatusBadge
              label={worker.geoip_asn_enabled ? 'ASN 支持' : 'ASN 未支持'}
              variant={worker.geoip_asn_enabled ? 'success' : 'warning'}
            />
            <StatusBadge
              label={worker.geoip_operator_enabled ? '运营商支持' : '运营商未支持'}
              variant={worker.geoip_operator_enabled ? 'success' : 'warning'}
            />
          </div>
        </div>
        {onOpenSettings ? (
          <SecondaryButton
            type="button"
            aria-label={`设置 DNS 响应端 ${worker.name}`}
            title="设置 DNS 响应端"
            className="h-9 w-9 shrink-0 rounded-xl px-0 py-0"
            onClick={() => onOpenSettings(worker)}
          >
            <Settings className="h-4 w-4" aria-hidden="true" />
          </SecondaryButton>
        ) : null}
      </div>

      <div className="mt-3 grid gap-2 sm:grid-cols-2">
        <InfoTile label="查询量" value={formatCount(worker.query_count)} />
        <InfoTile
          label="错误率"
          value={formatPercentValue(worker.error_rate_percent)}
        />
        <InfoTile
          label="平均延迟"
          value={formatLatencyMs(worker.average_latency_ms)}
        />
        <InfoTile
          label="最大延迟"
          value={formatLatencyMs(worker.max_latency_ms)}
        />
        <InfoTile
          label="响应端配置年龄"
          value={formatDurationSeconds(worker.snapshot_age_seconds)}
          helper={
            worker.snapshot_stale
              ? '超过 DNS 响应端过期阈值，需要检查响应端访问面板或重启重新拉取。'
              : '响应端当前缓存解析配置的时间。'
          }
        />
        <InfoTile
          label="最近心跳"
          value={formatRelativeTime(worker.last_seen_at)}
          helper={
            isMeaningfulTime(worker.last_heartbeat_at)
              ? `统计心跳 ${formatRelativeTime(worker.last_heartbeat_at)}`
              : '尚未收到统计心跳'
          }
        />
        <InfoTile
          label="最近查询统计"
          value={formatRelativeTime(worker.last_rollup_at)}
          helper={
            worker.last_rollup_count > 0
              ? `上次上报 ${formatCount(worker.last_rollup_count)} 次查询`
              : '尚未上报 DNS 查询 rollup'
          }
        />
        <InfoTile
          label="多节点探测"
          value={`${worker.node_probe_healthy_count ?? 0} / ${worker.node_probe_total_count ?? 0}`}
          helper={
            (worker.node_probe_stale_count ?? 0) > 0
              ? `${worker.node_probe_stale_count} 个过期`
              : undefined
          }
        />
        <InfoTile
          label="多节点耗时"
          value={formatLatencyMs(worker.node_probe_average_rtt_ms ?? 0)}
        />
      </div>

      {worker.last_error ? (
        <InlineMessage
          className="mt-3"
          tone="danger"
          message={worker.last_error}
        />
      ) : null}
      {isWaitingForUnsupportedUpdate ? (
        <InlineMessage
          className="mt-3"
          tone="warning"
          message="该 DNS 响应端正在心跳，但当前版本未声明支持远程自更新；请先在这台响应端手动执行一次新版 install-dns-worker.sh，之后面板下发更新才会被心跳消费。"
        />
      ) : null}
      {worker.geoip_last_error ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message={`国家识别库加载失败：${worker.geoip_last_error}`}
        />
      ) : worker.asn_last_error ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message={`ASN 识别库加载失败：${worker.asn_last_error}`}
        />
      ) : worker.operator_cidr_last_error ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message={`运营商 CIDR 库加载失败：${worker.operator_cidr_last_error}`}
        />
      ) : !worker.geoip_enabled ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message="未加载国家识别库；按国家/地区匹配的节点池会回退到全局规则，来源网段匹配不受影响。"
        />
      ) : null}
      {worker.probe_message ? (
        <p className="mt-3 text-xs text-[var(--foreground-secondary)]">
          探测状态：{worker.probe_message}
          {worker.probe_age_seconds > 0
            ? ` · ${formatDurationSeconds(worker.probe_age_seconds)}前`
            : ''}
        </p>
      ) : null}
      {worker.last_probe_at && worker.last_probe_results.length > 0 ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-[var(--foreground-secondary)]">
          <span>最近探测 {formatRelativeTime(worker.last_probe_at)}</span>
          {worker.last_probe_results.map((result) => (
            <StatusBadge
              key={`${worker.worker_id}-${result.network}`}
              label={`${result.network} ${result.reachable ? '可达' : '失败'}`}
              variant={getProbeResultVariant(result)}
            />
          ))}
        </div>
      ) : null}
      {visibleNodeProbes.length > 0 ? (
        <div className="mt-4 space-y-2">
          <p className="text-xs font-medium text-[var(--foreground-primary)]">
            Agent 多节点探测
          </p>
          <div className="grid gap-2">
            {visibleNodeProbes.map((probe) => (
              <div
                key={`${worker.worker_id}-${probe.node_id}`}
                className="rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-panel)] px-3 py-2"
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-xs font-medium text-[var(--foreground-primary)]">
                      {probe.node_name || probe.node_id}
                    </p>
                    <p className="mt-1 text-[11px] text-[var(--foreground-muted)]">
                      {probe.pool_name || 'default'} ·{' '}
                      {isMeaningfulTime(probe.checked_at)
                        ? formatRelativeTime(probe.checked_at)
                        : '尚未探测'}
                    </p>
                  </div>
                  <div className="flex flex-wrap justify-end gap-2">
                    <StatusBadge
                      label={getNodeDNSProbeStatusLabel(probe.probe_status)}
                      variant={getProbeStatusVariant(probe.probe_status)}
                    />
                    <StatusBadge
                      label={probe.status}
                      variant={getNodeProbeStatusVariant(probe.status)}
                    />
                  </div>
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--foreground-secondary)]">
                  <span>平均 {formatLatencyMs(probe.average_rtt_ms)}</span>
                  <span>最大 {formatLatencyMs(probe.max_rtt_ms)}</span>
                  {probe.probe_age_seconds > 0 ? (
                    <span>
                      {formatDurationSeconds(probe.probe_age_seconds)}前
                    </span>
                  ) : null}
                  {probe.results.map((result) => (
                    <StatusBadge
                      key={`${probe.node_id}-${result.network}`}
                      label={`${result.network} ${result.reachable ? '可达' : '失败'}`}
                      variant={getProbeResultVariant(result)}
                    />
                  ))}
                </div>
                {probe.last_error ? (
                  <p className="mt-2 text-xs text-[var(--status-danger-foreground)]">
                    {probe.last_error}
                  </p>
                ) : null}
                {probe.probe_message ? (
                  <p className="mt-2 text-xs text-[var(--foreground-secondary)]">
                    {probe.probe_message}
                  </p>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function DNSQueryTrendPanel({ summary }: { summary: DNSObservabilitySummary }) {
  const labels = summary.trend_points.map((point) =>
    formatTrendHour(point.bucket_started_at),
  );
  const trendTotals = summary.trend_points.reduce(
    (totals, point) => ({
      queryCount: totals.queryCount + point.query_count,
      servfailQueries: totals.servfailQueries + point.servfail_queries,
      nxdomainQueries: totals.nxdomainQueries + point.nxdomain_queries,
    }),
    { queryCount: 0, servfailQueries: 0, nxdomainQueries: 0 },
  );
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4 xl:col-span-2">
      <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
        查询趋势
      </h3>
      <div className="mt-4">
        <TrendChart
          labels={labels}
          height={240}
          series={[
            {
              label: '查询量',
              color: '#2563eb',
              fillColor: 'rgba(37,99,235,0.14)',
              values: summary.trend_points.map((point) => point.query_count),
              summaryValue: trendTotals.queryCount,
              variant: 'area',
              valueFormatter: formatCount,
            },
            {
              label: '服务异常',
              color: '#dc2626',
              values: summary.trend_points.map(
                (point) => point.servfail_queries,
              ),
              summaryValue: trendTotals.servfailQueries,
              valueFormatter: formatCount,
            },
            {
              label: '域名不存在',
              color: '#f59e0b',
              values: summary.trend_points.map(
                (point) => point.nxdomain_queries,
              ),
              summaryValue: trendTotals.nxdomainQueries,
              valueFormatter: formatCount,
            },
          ]}
          yAxisValueFormatter={formatCount}
        />
      </div>
    </div>
  );
}

function DNSSnapshotConsistencyPanel({
  consistency,
  onCopyCommand,
}: {
  consistency?: DNSWorkerSnapshotConsistency;
  onCopyCommand: (value: string, message: string) => void;
}) {
  if (!consistency) {
    return (
      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          解析配置一致性
        </h3>
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          暂无响应端解析配置状态。
        </p>
      </div>
    );
  }

  const isRisk =
    consistency.status === 'divergent' || consistency.status === 'stale';

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          解析配置一致性
        </h3>
        <StatusBadge
          label={getSnapshotConsistencyLabel(consistency.status)}
          variant={getSnapshotConsistencyVariant(consistency.status)}
        />
      </div>
      {isRisk ? (
        <InlineMessage
          className="mt-3"
          tone="danger"
          message={getSnapshotConsistencyMessage(consistency)}
        />
      ) : null}
      <div className="mt-3 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium text-[var(--foreground-primary)]">
              重新拉取解析配置
            </p>
            <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
              响应端会定时拉取；如果版本不一致或长时间过期，可在响应端服务器执行重启命令强制重新拉取。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <SecondaryButton
              type="button"
              onClick={() =>
                onCopyCommand(
                  'systemctl restart dushengcdn-dns-worker',
                  'systemd 重启命令已复制。',
                )
              }
            >
              复制 systemd 命令
            </SecondaryButton>
            <SecondaryButton
              type="button"
              onClick={() =>
                onCopyCommand(
                  'docker restart dushengcdn-dns-worker',
                  'Docker 重启命令已复制。',
                )
              }
            >
              复制 Docker 命令
            </SecondaryButton>
          </div>
        </div>
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <InfoTile
          label="在线响应端"
          value={`${consistency.online_worker_count} / ${consistency.total_worker_count}`}
        />
        <InfoTile
          label="最新配置"
          value={consistency.latest_snapshot_version || '—'}
        />
        <InfoTile
          label="过期响应端"
          value={formatCount(consistency.stale_worker_count)}
        />
        <InfoTile
          label="DNS 响应端过期阈值"
          value={`${consistency.snapshot_max_age_seconds} 秒`}
        />
      </div>
      {consistency.version_breakdown.length > 0 ? (
        <div className="mt-4 space-y-3">
          {consistency.version_breakdown.map((version) => (
            <div
              key={version.version}
              className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3"
            >
              <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
                <span className="font-medium break-all text-[var(--foreground-primary)]">
                  {version.version}
                </span>
                <span className="text-[var(--foreground-secondary)]">
                  {version.worker_count} 个响应端
                </span>
              </div>
              <p className="mt-2 text-xs text-[var(--foreground-muted)]">
                {version.workers.join('、')}
                {version.latest_snapshot_at
                  ? ` · ${formatDateTime(version.latest_snapshot_at)}`
                  : ''}
              </p>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function CounterChart({
  title,
  items,
  total,
  color,
  emptyText = '暂无数据。',
  formatLabel,
}: {
  title: string;
  items: DNSObservabilityCounterItem[];
  total: number;
  color: string;
  emptyText?: string;
  formatLabel?: (item: DNSObservabilityCounterItem) => string;
}) {
  const visibleItems = items.map((item) => ({
    ...item,
    label: formatLabel ? formatLabel(item) : item.label || item.key || '未命名',
  }));
  const chartItems = visibleItems
    .map((item) => ({ label: item.label, value: item.count }))
    .reverse();
  const topItems = visibleItems.slice(0, 3);

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          {title}
        </h3>
        <span className="text-xs text-[var(--foreground-muted)]">
          共 {formatCount(total)} 次
        </span>
      </div>
      {items.length === 0 ? (
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          {emptyText}
        </p>
      ) : (
        <div className="mt-3 space-y-4">
          <div className="flex flex-wrap gap-2">
            {topItems.map((item) => (
              <div
                key={`${title}-${item.key}`}
                className="inline-flex max-w-full items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-1.5 text-xs"
              >
                <span className="truncate text-[var(--foreground-secondary)]">
                  {item.label}
                </span>
                <strong className="shrink-0 font-semibold text-[var(--foreground-primary)]">
                  {formatCount(item.count)}
                </strong>
                {total > 0 ? (
                  <span className="shrink-0 text-[var(--foreground-muted)]">
                    {formatPercent(item.count, total)}
                  </span>
                ) : null}
              </div>
            ))}
          </div>
          <div className="overflow-hidden rounded-[28px] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0))] px-3 py-3">
            <RankChart
              items={chartItems}
              color={color}
              valueFormatter={formatCount}
              emptyMessage={emptyText}
            />
          </div>
        </div>
      )}
    </div>
  );
}

function InfoTile({
  label,
  value,
  helper,
}: {
  label: string;
  value: string | number;
  helper?: string;
}) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
        {label}
      </p>
      <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
        {value}
      </p>
      {helper ? (
        <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
          {helper}
        </p>
      ) : null}
    </div>
  );
}

function ZoneEditorModal({
  isOpen,
  zone,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  zone: DNSZoneItem | null;
  onClose: () => void;
  onSaved: (zone: DNSZoneItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<ZoneFormValues>({
    defaultValues: zoneToFormValues(zone),
  });
  const saveMutation = useMutation({
    mutationFn: (values: ZoneFormValues) => {
      const payload: DNSZoneMutationPayload = {
        name: values.name.trim(),
        soa_email: values.soa_email.trim(),
        primary_ns: values.primary_ns.trim(),
        name_servers: linesFromText(values.name_servers_text),
        default_ttl: values.default_ttl,
        enabled: values.enabled,
      };
      return zone ? updateDNSZone(zone.id, payload) : createDNSZone(payload);
    },
    onSuccess: onSaved,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset(zoneToFormValues(zone));
    }
  }, [form, isOpen, zone]);

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={zone ? '编辑托管域名' : '创建托管域名'}
      description="托管域名保存后会规范化为根域名格式；生产环境建议至少填写两个可公网访问的 DNS 响应端名称。"
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          saveMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <ResourceField
          label="托管域名"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="example.com"
            {...form.register('name', { required: '请输入托管域名' })}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="基础邮箱"
            hint="留空时后端使用 hostmaster@托管域名。"
          >
            <ResourceInput
              placeholder="hostmaster@example.com"
              {...form.register('soa_email')}
            />
          </ResourceField>
          <ResourceField
            label="主解析服务器"
            hint="留空时默认使用 NS 列表第一项。"
          >
            <ResourceInput
              placeholder="ns1.example.net"
              {...form.register('primary_ns')}
            />
          </ResourceField>
        </div>
        <ResourceField
          label="注册商 NS 列表"
          hint="每行一个 NS，也可用逗号或分号分隔；这些值需要填写到域名注册商后台。"
          tooltip="NS 是注册商后台里的“域名服务器”。这里填 ns1.example.net、ns2.example.net 这类地址后，还要去注册商把域名指向它们。"
        >
          <ResourceTextarea
            placeholder={'ns1.example.net\nns2.example.net'}
            {...form.register('name_servers_text')}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField label="默认缓存时间">
            <ResourceInput
              type="number"
              min={1}
              max={86400}
              {...form.register('default_ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ToggleField
            label="启用托管域名"
            description="停用后不会下发给 DNS 响应端。"
            checked={form.watch('enabled')}
            onChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
        </div>
        <PrimaryButton type="submit" disabled={saveMutation.isPending}>
          {saveMutation.isPending ? '保存中...' : '保存'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

function RecordEditorModal({
  isOpen,
  zone,
  record,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  zone: DNSZoneItem;
  record: DNSRecordItem | null;
  onClose: () => void;
  onSaved: (record: DNSRecordItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<RecordFormValues>({
    defaultValues: recordToFormValues(record),
  });
  const recordType = form.watch('type');
  const ipValues = form.watch('ip_values');
  const isAddressRecord = isAddressRecordType(recordType);
  const valuePlaceholder = getRecordValuePlaceholder(recordType);
  const saveMutation = useMutation({
    mutationFn: (values: RecordFormValues) => {
      const basePayload: DNSRecordMutationPayload = {
        zone_id: zone.id,
        name: values.name.trim(),
        type: values.type,
        value: values.value.trim(),
        ttl: values.ttl,
        priority: values.type === 'MX' ? values.priority : 0,
        enabled: values.enabled,
      };
      if (isAddressRecordType(values.type)) {
        const addresses = (values.ip_values ?? [])
          .map((item) => item.trim())
          .filter(Boolean);
        if (addresses.length === 0) {
          throw new Error(
            values.type === 'A' ? '请输入 IPv4 地址' : '请输入 IPv6 地址',
          );
        }
        if (record) {
          return updateDNSRecord(record.id, {
            ...basePayload,
            value: addresses[0],
          });
        }
        return Promise.all(
          addresses.map((address) =>
            createDNSZoneRecord(zone.id, {
              ...basePayload,
              value: address,
            }),
          ),
        ).then((records) => records[0]);
      }
      return record
        ? updateDNSRecord(record.id, basePayload)
        : createDNSZoneRecord(zone.id, basePayload);
    },
    onSuccess: onSaved,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset(recordToFormValues(record));
    }
  }, [form, isOpen, record]);

  useEffect(() => {
    if (!isAddressRecord) {
      return;
    }
    const current = form.getValues('ip_values') ?? [];
    if (current.length === 0) {
      form.setValue('ip_values', [''], { shouldDirty: false });
    }
  }, [form, isAddressRecord, recordType]);

  const updateIPAddressValue = (index: number, value: string) => {
    const current = form.getValues('ip_values') ?? [''];
    const next = current.length > 0 ? [...current] : [''];
    next[index] = value;
    form.setValue('ip_values', next, { shouldDirty: true });
  };

  const addIPAddressValue = () => {
    const current = form.getValues('ip_values') ?? [''];
    form.setValue('ip_values', [...current, ''], { shouldDirty: true });
  };

  const removeIPAddressValue = (index: number) => {
    const current = form.getValues('ip_values') ?? [''];
    const next = current.filter((_, itemIndex) => itemIndex !== index);
    form.setValue('ip_values', next.length > 0 ? next : [''], {
      shouldDirty: true,
    });
  };

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={record ? '编辑 DNS 记录' : '新增 DNS 记录'}
      description={`当前托管域名：${zone.name}。记录名可填写 @、完整域名，或填写 www 这类相对名称。`}
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          saveMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="记录名"
            hint="@ 表示当前托管域名根域。"
            error={form.formState.errors.name?.message}
          >
            <ResourceInput
              placeholder="@"
              {...form.register('name', { required: '请输入记录名' })}
            />
          </ResourceField>
          <ResourceField label="记录类型">
            <ResourceSelect {...form.register('type')}>
              {dnsRecordTypes.map((type) => (
                <option key={type} value={type}>
                  {type}
                </option>
              ))}
            </ResourceSelect>
          </ResourceField>
        </div>
        {isAddressRecord ? (
          <ResourceField
            label={getRecordValueLabel(recordType)}
            hint={getRecordValueHint(recordType)}
            container="div"
          >
            <div className="space-y-3">
              {(ipValues?.length ? ipValues : ['']).map((value, index) => (
                <div
                  key={index}
                  className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]"
                >
                  <ResourceInput
                    value={value}
                    placeholder={valuePlaceholder}
                    onChange={(event) =>
                      updateIPAddressValue(index, event.target.value)
                    }
                  />
                  <div className="flex gap-2">
                    {!record && index === (ipValues?.length ?? 1) - 1 ? (
                      <SecondaryButton
                        type="button"
                        aria-label="增加 IP 地址"
                        title="增加 IP 地址"
                        className="h-12 w-12 shrink-0 px-0"
                        onClick={addIPAddressValue}
                      >
                        <Plus className="h-4 w-4" aria-hidden="true" />
                      </SecondaryButton>
                    ) : null}
                    {!record && (ipValues?.length ?? 1) > 1 ? (
                      <SecondaryButton
                        type="button"
                        aria-label="删除 IP 地址"
                        title="删除 IP 地址"
                        className="h-12 w-12 shrink-0 px-0"
                        onClick={() => removeIPAddressValue(index)}
                      >
                        <Minus className="h-4 w-4" aria-hidden="true" />
                      </SecondaryButton>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          </ResourceField>
        ) : (
          <ResourceField
            label={getRecordValueLabel(recordType)}
            hint={getRecordValueHint(recordType)}
            error={form.formState.errors.value?.message}
          >
            <ResourceTextarea
              placeholder={valuePlaceholder}
              {...form.register('value', { required: '请输入记录内容' })}
            />
          </ResourceField>
        )}
        <div className="grid gap-5 md:grid-cols-3">
          <ResourceField label="缓存时间" hint="0 表示使用托管域名默认缓存时间。">
            <ResourceInput
              type="number"
              min={0}
              max={86400}
              {...form.register('ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ResourceField
            label="MX 优先级"
            hint={
              recordType === 'MX'
                ? '只对 MX 生效；同一域名有多个 MX 时，邮件会先投递到数字更小的服务器，常见主服务器填 10，备用服务器填 20。'
                : '仅 MX 记录需要填写优先级，数字越小越优先；其它记录会自动保存为 0。'
            }
          >
            <ResourceInput
              type="number"
              min={0}
              disabled={recordType !== 'MX'}
              {...form.register('priority', { valueAsNumber: true })}
            />
          </ResourceField>
          <ToggleField
            label="启用记录"
            description="停用后不会下发给 DNS 响应端。"
            checked={form.watch('enabled')}
            onChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
        </div>
        <PrimaryButton type="submit" disabled={saveMutation.isPending}>
          {saveMutation.isPending ? '保存中...' : '保存'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

function WorkerCreateModal({
  isOpen,
  onClose,
  onCreated,
}: {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (worker: DNSWorkerItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<WorkerFormValues>({
    defaultValues: {
      name: '',
      public_address: '',
      remark: '',
    },
  });
  const createMutation = useMutation({
    mutationFn: (values: WorkerFormValues) =>
      createDNSWorker({
        name: values.name.trim(),
        public_address: values.public_address.trim(),
        remark: values.remark.trim(),
      }),
    onSuccess: onCreated,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset({ name: '', public_address: '', remark: '' });
    }
  }, [form, isOpen]);

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title="创建 DNS 响应端"
      description="响应端密钥只会在创建后返回一次；请在弹窗中复制部署命令。"
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          createMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <ResourceField
          label="响应端名称"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="ns1-hk"
            {...form.register('name', { required: '请输入响应端名称' })}
          />
        </ResourceField>
        <ResourceField
          label="公网地址"
          hint="可填写 ns1.example.net 或 203.0.113.10，便于管理端展示和排障。"
        >
          <ResourceInput
            placeholder="ns1.example.net"
            {...form.register('public_address')}
          />
        </ResourceField>
        <ResourceField label="备注" hint="可选，只用于面板展示。">
          <ResourceTextarea
            maxLength={255}
            placeholder="例如：香港阿里云 / 主响应端"
            {...form.register('remark')}
          />
        </ResourceField>
        <PrimaryButton type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? '创建中...' : '创建'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

function WorkerSettingsModal({
  worker,
  isSaving,
  isRequestingUpdate,
  onClose,
  onSave,
  onRequestUpdate,
}: {
  worker: DNSWorkerHealthItem;
  isSaving: boolean;
  isRequestingUpdate: boolean;
  onClose: () => void;
  onSave: (values: WorkerSettingsFormValues) => void;
  onRequestUpdate: () => void;
}) {
  const form = useForm<WorkerSettingsFormValues>({
    defaultValues: {
      remark: worker.remark ?? '',
    },
  });
  const isWaitingForUnsupportedUpdate =
    worker.update_requested && !worker.update_supported;
  const updateDisabled = isRequestingUpdate || worker.update_requested;
  const updateButtonLabel = isRequestingUpdate
    ? '下发中...'
    : isWaitingForUnsupportedUpdate
      ? '需先手动升级'
      : worker.update_requested
        ? '等待心跳执行'
        : '强制下发更新';

  useEffect(() => {
    form.reset({ remark: worker.remark ?? '' });
  }, [form, worker.id, worker.remark]);

  return (
    <AppModal
      isOpen
      onClose={onClose}
      title="DNS 响应端设置"
      description={`${worker.name} · ${worker.public_address || worker.worker_id}`}
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => onSave(values))}
      >
        <ResourceField
          label="备注"
          hint="备注只用于面板展示，不会改变响应端名称、密钥或 NS 配置。"
        >
          <ResourceTextarea
            maxLength={255}
            placeholder="例如：香港阿里云 / 主响应端"
            {...form.register('remark')}
          />
        </ResourceField>
        <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                强制更新
              </p>
              <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                下发后由响应端下一次心跳消费；旧版响应端未声明支持自更新时，需要先手动执行新版安装脚本。
              </p>
            </div>
            <StatusBadge
              label={
                worker.update_supported ? '支持远程更新' : '需先手动升级'
              }
              variant={worker.update_supported ? 'success' : 'warning'}
            />
          </div>
          {isWaitingForUnsupportedUpdate ? (
            <InlineMessage
              className="mt-3"
              tone="warning"
              message="该响应端已有待执行更新，但当前版本未声明支持远程自更新。"
            />
          ) : null}
          <SecondaryButton
            type="button"
            className="mt-4"
            disabled={updateDisabled}
            onClick={onRequestUpdate}
          >
            {updateButtonLabel}
          </SecondaryButton>
        </div>
        <div className="flex flex-wrap justify-end gap-3">
          <SecondaryButton type="button" onClick={onClose}>
            取消
          </SecondaryButton>
          <PrimaryButton type="submit" disabled={isSaving}>
            {isSaving ? '保存中...' : '保存备注'}
          </PrimaryButton>
        </div>
      </form>
    </AppModal>
  );
}

function WorkerTokenModal({
  worker,
  serverUrl,
  onClose,
}: {
  worker: DNSWorkerItem;
  serverUrl: string;
  onClose: () => void;
}) {
  const [copyFeedback, setCopyFeedback] = useState<{
    tone: 'success' | 'danger';
    message: string;
  } | null>(null);
  const token = worker.token ?? '';
  const installCommand = `curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \\
  --server-url ${serverUrl} \\
  --token ${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  --source-database-profile full \\
  --query-rate-limit 200 \\
  --udp-response-size 1232`;
  const dockerCommand = `docker run -d --name dushengcdn-dns-worker --restart unless-stopped \\
  -p 53:53/udp -p 53:53/tcp \\
  -v dushengcdn-dns-worker-data:/data \\
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=${serverUrl} \\
  -e DUSHENGCDN_DNS_WORKER_TOKEN=${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  -e DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=/data/geoip/GeoLite2-Country.mmdb \\
  -e DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=/data/geoip/GeoLite2-ASN.mmdb \\
  -e DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=/data/operator-cidr \\
  -e DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200 \\
  -e DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=1232 \\
  ghcr.io/satands/dushengcdn-dns-worker:latest`;
  const sourceCommand = `cd dushengcdn_server
go run ./cmd/dns-worker \\
  --server-url ${serverUrl} \\
  --token ${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  --listen :53 \\
  --snapshot-path /var/lib/dushengcdn-dns-worker/snapshot.json \\
  --geoip-database /var/lib/dushengcdn-dns-worker/geoip/GeoLite2-Country.mmdb \\
  --asn-database /var/lib/dushengcdn-dns-worker/geoip/GeoLite2-ASN.mmdb \\
  --operator-cidr-database /var/lib/dushengcdn-dns-worker/operator-cidr \\
  --query-rate-limit 200 \\
  --udp-response-size 1232`;

  const handleCopy = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setCopyFeedback({ tone: 'success', message });
    } catch (error) {
      setCopyFeedback({ tone: 'danger', message: getErrorMessage(error) });
    }
  };

  return (
    <AppModal
      isOpen
      onClose={onClose}
      title="DNS 响应端密钥"
      description={`响应端 ${worker.name} 已创建。密钥离开弹窗后不会再次显示。`}
      size="xl"
    >
      <div className="space-y-5">
        {copyFeedback ? (
          <InlineMessage
            tone={copyFeedback.tone}
            message={copyFeedback.message}
          />
        ) : null}
        <ResourceField
          label="响应端密钥"
          tooltip="这是 DNS 响应端连接面板用的专属密钥，只在创建后显示一次；不是节点 Agent 密钥，也不是登录密码。"
        >
          <div className="flex flex-col gap-3 md:flex-row">
            <ResourceInput readOnly value={token} className="font-mono" />
            <SecondaryButton
              type="button"
              onClick={() => void handleCopy(token, '响应端密钥已复制。')}
            >
              复制密钥
            </SecondaryButton>
          </div>
        </ResourceField>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              安装脚本命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(installCommand, '安装脚本命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {installCommand}
          </CodeBlock>
        </div>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              Docker 部署命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(dockerCommand, 'Docker 命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {dockerCommand}
          </CodeBlock>
        </div>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              源码运行命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(sourceCommand, '源码运行命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {sourceCommand}
          </CodeBlock>
        </div>
      </div>
    </AppModal>
  );
}
