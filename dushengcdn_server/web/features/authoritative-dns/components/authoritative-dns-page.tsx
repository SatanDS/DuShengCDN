'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Minus, Plus } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToastFeedback } from '@/components/feedback/toast-provider';
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
  simulateDNSGSLB,
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
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';

type FeedbackState = {
  tone: 'info' | 'success' | 'danger';
  message: string;
};

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
};

type GSLBSimulationFormValues = {
  proxy_route_id: string;
  qname: string;
  record_type: 'A' | 'AAAA';
  country: string;
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

const dnsObservabilityWindowHours = 24;

const migrationRecheckStepTemplates: Array<{
  key: MigrationRecheckStepKey;
  label: string;
}> = [
  { key: 'mode', label: '网站 DNS 模式' },
  { key: 'delegation', label: 'Zone 委派检查' },
  { key: 'worker_probe', label: 'Worker 公网探测' },
  { key: 'simulation', label: 'GSLB 模拟复测' },
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
      return 'IPv4 地址';
    case 'AAAA':
      return 'IPv6 地址';
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
      return '每个输入框填写一个 IPv4 地址，可点 + 增加多个地址。';
    case 'AAAA':
      return '每个输入框填写一个 IPv6 地址，可点 + 增加多个地址。';
    case 'CNAME':
      return '填写目标域名，同名下不要再添加其它记录。';
    case 'MX':
      return '填写邮件服务器域名，并设置优先级。';
    case 'NS':
      return '填写权威 NS 域名。';
    case 'SOA':
      return '填写 SOA 内容，通常由 Zone 自动生成即可。';
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
    return country ? `国家 ${country}` : text;
  }
  if (lower.startsWith('cidr:')) {
    const cidr = text.slice(text.indexOf(':') + 1).trim();
    return cidr ? `网段 ${cidr}` : text;
  }
  return text;
}

function isProbeSchedulingGateMessage(message: string) {
  return message.includes('Agent 探测未达到调度门槛');
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
      return '探测过期';
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
      return '探测过期';
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
      return '快照一致';
    case 'divergent':
      return '快照不一致';
    case 'stale':
      return '快照过期';
    case 'no_online_workers':
      return '无在线 Worker';
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
    return `快照过期：存在 ${formatCount(consistency.stale_worker_count)} 个在线 Worker 超过 ${formatDurationSeconds(consistency.snapshot_max_age_seconds)} 未拉取新快照。请检查 DNS Worker 的 Server URL 是否可达、Worker Token 是否有效，以及服务日志中 /api/dns-snapshot 的 HTTP 状态。`;
  }
  if (consistency.status === 'divergent') {
    const versionCount = consistency.version_breakdown.length;
    const latestVersion = consistency.latest_snapshot_version || '未知版本';
    return `快照不一致：在线 Worker 当前使用了 ${formatCount(versionCount)} 个快照版本，查询结果可能不一致。最新版本 ${latestVersion}；请检查落后 Worker 到 Server URL 的网络、Worker Token 是否仍有效，必要时重启 Worker 触发重新拉取快照。`;
  }
  return '';
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
  const authoritativeRoutes = useMemo(
    () =>
      proxyRoutes
        .filter((route) => route.dns_provider_mode === 'authoritative')
        .filter((route) => route.enabled || route.gslb_enabled),
    [proxyRoutes],
  );
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
      setFeedback({ tone: 'success', message: 'DNS Zone 已删除。' });
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
      setFeedback({ tone: 'success', message: 'DNS Worker 已删除。' });
      await queryClient.invalidateQueries({
        queryKey: ['authoritative-dns', 'workers'],
      });
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
        message: `DNS Worker 探测完成：${reachableCount} / ${result.results.length} 可达。`,
      });
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
        message: `已切换“${getRouteDisplayName(updatedRoute)}”到自建权威 DNS，正在自动复测。`,
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
      '正在刷新网站 DNS 模式',
    );
    setMigrationRecheck(result);
    setRecheckingRouteId(route.id);
    setFeedback({
      tone: 'info',
      message: `正在复测“${getRouteDisplayName(route)}”的权威 DNS 切换结果。`,
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
            ? `已绑定 Zone ${zone.name}`
            : '网站尚未切换到自建权威 DNS',
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
        '正在检查注册商 NS 委派',
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
            ? '公网 NS 已与 Zone 配置匹配'
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
        '正在探测在线 DNS Worker 的 UDP/TCP 53',
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
            ? '没有在线 DNS Worker'
            : `${healthyProbeCount} / ${onlineWorkersForProbe.length} 个在线 Worker UDP/TCP 53 可达`,
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
        '正在按当前快照模拟全局和来源国家调度',
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
            ? '当前快照没有返回可用目标'
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
      title: '删除 DNS Zone',
      message: `确认删除 Zone“${zone.name}”吗？会同时删除该 Zone 下的静态记录；已被网站权威 DNS 模式引用时后端会阻止删除。`,
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
      title: '删除 DNS Worker',
      message: `确认删除 Worker“${worker.name}”吗？删除后该 Worker Token 将不可再拉取快照。`,
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
      title: '切换到权威 DNS',
      message: `确认把“${getRouteDisplayName(route)}”切换到自建权威 DNS，并绑定 Zone“${zone.name}”吗？切换后请到注册商确认 NS 委派。`,
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
        title="DNS Zone 加载失败"
        description={getErrorMessage(zonesQuery.error)}
      />
    );
  }

  if (workersQuery.isError) {
    return (
      <ErrorState
        title="DNS Worker 加载失败"
        description={getErrorMessage(workersQuery.error)}
      />
    );
  }

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="权威 DNS"
          description="管理自建权威 DNS Zone、静态记录和 DNS Worker，用于按来源与节点状态实时执行 GSLB 调度。"
          action={
            <div className="flex flex-wrap gap-3">
              <SecondaryButton
                type="button"
                onClick={() => setIsWorkerModalOpen(true)}
              >
                创建 Worker
              </SecondaryButton>
              <PrimaryButton type="button" onClick={openCreateZone}>
                创建 Zone
              </PrimaryButton>
            </div>
          }
        />

        <div className="grid gap-4 md:grid-cols-3">
          <AppCard title="托管 Zone">
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
                不含网站动态 GSLB 记录
              </p>
            </div>
          </AppCard>
          <AppCard title="DNS Worker">
            <div className="space-y-2">
              <p className="text-3xl font-semibold text-[var(--foreground-primary)]">
                {workers.filter((worker) => worker.status === 'online').length}
                <span className="text-base text-[var(--foreground-secondary)]">
                  {' '}
                  / {workers.length}
                </span>
              </p>
              <p className="text-sm text-[var(--foreground-secondary)]">
                在线 / 全部 Worker
              </p>
            </div>
          </AppCard>
        </div>

        {showNoAuthoritativeRoutesNotice ? (
          <InlineMessage
            tone="warning"
            message="DNS Worker 已能拉取快照，但当前没有启用的网站绑定到自建权威 DNS Zone。此时 Worker 只能回答 Zone 的 SOA/NS 和静态记录；业务域名的 A/AAAA 动态调度需要到网站详情「自动 DNS」切换为自建权威 DNS 并选择对应 Zone，或使用迁移向导一键切换。"
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
        />

        <GSLBSchedulingStatesPanel
          states={schedulingStates}
          isLoading={schedulingStatesQuery.isLoading}
          error={
            schedulingStatesQuery.isError
              ? getErrorMessage(schedulingStatesQuery.error)
              : ''
          }
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
              label: 'Zone 与记录',
              description: '托管域名、NS、SOA 和静态 DNS 记录。',
            },
            {
              key: 'workers' as const,
              label: 'DNS Worker',
              description: '管理权威 DNS 查询节点和快照状态。',
            },
            {
              key: 'migration' as const,
              label: '迁移向导',
              description: '检查 Cloudflare 站点切换到自建权威 DNS 的准备项。',
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
              message: editingZone ? 'DNS Zone 已保存。' : 'DNS Zone 已创建。',
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
            setFeedback({ tone: 'success', message: 'DNS Worker 已创建。' });
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
      title="Zone 与记录"
      description="Zone 用于承接注册商 NS 委派；静态记录和网站权威 DNS 动态记录会一起进入 Worker 快照。"
      action={
        <PrimaryButton type="button" onClick={onCreateZone}>
          创建 Zone
        </PrimaryButton>
      }
    >
      {zones.length === 0 ? (
        <EmptyState
          title="暂无 DNS Zone"
          description="创建 Zone 后，再到网站配置的自动 DNS 分区切换为自建权威 DNS。"
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
                      SOA {selectedZone.soa_email || 'hostmaster'}，默认 TTL{' '}
                      {selectedZone.default_ttl} 秒。
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <SecondaryButton
                      type="button"
                      onClick={() => onEditZone(selectedZone)}
                    >
                      编辑 Zone
                    </SecondaryButton>
                    <DangerButton
                      type="button"
                      disabled={busy}
                      onClick={() => onDeleteZone(selectedZone)}
                    >
                      删除 Zone
                    </DangerButton>
                  </div>
                </div>

                <div className="mt-4 grid gap-3 md:grid-cols-2">
                  <InfoTile
                    label="Primary NS"
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
                      暂未配置 NS。生产环境至少配置两个 Worker 对应的 NS。
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
                      A/AAAA/CNAME/TXT/MX/NS/SOA 会由 DNS Worker 从快照回答。
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
                      description="网站绑定权威 DNS 后，A/AAAA 动态记录可由 GSLB 实时生成。"
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
            对比当前公网 NS 与 Zone 配置的注册商 NS。
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
                message={`Glue 提示：${visibleCheck.glue_name_servers.join('、')} 位于 ${zone.name} 内，需要在注册商配置主机记录/Glue。`}
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
            点击后检查注册商是否已经把 {zone.name} 委派到当前 NS。
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
      title="DNS Worker"
      description="Worker 使用专属 Token 拉取只读 DNS 快照，并监听 UDP/TCP 53 回答权威查询。"
      action={
        <PrimaryButton type="button" onClick={onCreateWorker}>
          创建 Worker
        </PrimaryButton>
      }
    >
      {workers.length === 0 ? (
        <EmptyState
          title="暂无 DNS Worker"
          description="创建 Worker 后复制部署命令，并在注册商处把 Zone NS 委派到 Worker 地址。"
        />
      ) : (
        <div className="space-y-4">
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
                        worker.geoip_enabled ? 'GeoIP 已加载' : 'GeoIP 未加载'
                      }
                      variant={worker.geoip_enabled ? 'success' : 'warning'}
                    />
                  </div>
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                    <InfoTile label="Worker ID" value={worker.worker_id} />
                    <InfoTile
                      label="公网地址"
                      value={worker.public_address || '—'}
                    />
                    <InfoTile
                      label="最近心跳"
                      value={formatRelativeTime(worker.last_seen_at)}
                    />
                    <InfoTile
                      label="最近快照"
                      value={formatRelativeTime(worker.last_snapshot_at)}
                    />
                  </div>
                  <p className="text-xs leading-5 text-[var(--foreground-secondary)]">
                    快照版本：{worker.last_snapshot_version || '—'}
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
                      message={`GeoIP 国家库加载失败：${worker.geoip_last_error}`}
                    />
                  ) : !worker.geoip_enabled ? (
                    <InlineMessage
                      tone="info"
                      message="未加载 GeoIP 国家库；国家代码节点池不会命中，仍可按来源 CIDR 或全局调度。"
                    />
                  ) : (
                    <p className="text-xs break-all text-[var(--foreground-secondary)]">
                      GeoIP 国家库：{worker.geoip_database_path || '已加载'}
                    </p>
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
              RCODE：{result.rcode || '—'} · 应答 {result.answer_count}
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
  if (isLoading) {
    return (
      <AppCard title="GSLB 调度状态">
        <LoadingState />
      </AppCard>
    );
  }

  if (error) {
    return <ErrorState title="GSLB 调度状态加载失败" description={error} />;
  }

  const rows = states?.states ?? [];
  const debouncingCount = rows.filter(
    (item) => item.status === 'debouncing',
  ).length;
  const activeCount = rows.filter((item) => item.status === 'active').length;

  return (
    <AppCard
      title="GSLB 调度状态"
      description="展示 Worker 回传和 Server 记录的当前实际目标、期望目标与逐来源防抖状态。"
    >
      {rows.length === 0 ? (
        <EmptyState
          title="暂无调度状态"
          description="权威 DNS 站点收到 A/AAAA 查询，或 Cloudflare 模式触发 GSLB 重算后，这里会显示当前选中的边缘 IP。"
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
          <div className="grid gap-3 xl:grid-cols-2">
            {rows.map((state) => (
              <GSLBSchedulingStateCard key={state.id} state={state} />
            ))}
          </div>
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
          label="来源作用域/分流桶"
          value={formatSourceScopeLabel(state.scope_key)}
          helper={state.scope_key}
        />
        <InfoTile
          label="最近评估"
          value={formatRelativeTime(state.last_evaluated_at)}
        />
        <InfoTile label="当前目标" value={selectedText} />
        <InfoTile label="期望目标" value={desiredText} />
      </div>

      {state.status === 'debouncing' ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message="期望目标已变化，但当前目标仍处于防抖冷却或旧目标仍健康，所以暂未切换。"
        />
      ) : null}
      {state.last_reason ? (
        <p className="mt-3 text-xs leading-5 break-all text-[var(--foreground-secondary)]">
          {state.last_reason}
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
      title="GSLB 调度模拟"
      description="按站点、记录类型、来源国家和来源 IP 预演当前权威 DNS 快照会返回的边缘 IP。"
    >
      {routesLoading ? (
        <LoadingState />
      ) : routesError ? (
        <ErrorState title="调度模拟加载失败" description={routesError} />
      ) : routes.length === 0 ? (
        <EmptyState
          title="暂无权威 DNS 站点"
          description="把网站配置的自动 DNS 模式切换为自建权威 DNS 后，可在这里模拟实时 GSLB 选点。"
        />
      ) : (
        <div className="grid gap-5 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
          <form
            className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] p-4"
            onSubmit={form.handleSubmit((values) => {
              onSimulate({
                proxy_route_id: Number(values.proxy_route_id),
                qname: values.qname.trim(),
                record_type: values.record_type,
                country: values.country.trim().toUpperCase(),
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
            <ResourceField
              label="来源 IP"
              hint="可选；填写后会优先参与来源 CIDR 匹配预演。"
            >
              <ResourceInput
                placeholder="203.0.113.10"
                {...form.register('source_ip')}
              />
            </ResourceField>
            {selectedRoute ? (
              <div className="grid gap-3 md:grid-cols-2">
                <InfoTile
                  label="GSLB"
                  value={selectedRoute.gslb_enabled ? '已启用' : '未启用'}
                />
                <InfoTile
                  label="节点池"
                  value={getGSLBDescription(selectedRoute)}
                />
              </div>
            ) : null}
            <PrimaryButton type="submit" disabled={isPending}>
              {isPending ? '模拟中...' : '模拟调度'}
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
                    label="作用域/分流桶"
                    value={formatSourceScopeLabel(result.source_scope)}
                    helper={result.source_scope}
                  />
                  <InfoTile label="TTL" value={`${result.ttl} 秒`} />
                  <InfoTile
                    label="策略"
                    value={
                      result.strategy ||
                      (result.gslb_enabled ? 'GSLB' : '节点池')
                    }
                  />
                </div>
                {result.targets.length > 0 ? (
                  <div className="space-y-2">
                    <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
                      返回目标
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
                  {result.qname} {result.record_type} · 快照{' '}
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
                    message={result.message}
                  />
                ) : null}
                <GSLBSimulationDiagnostics result={result} />
              </div>
            ) : (
              <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
                选择站点和来源后点击模拟，可看到 DNS Worker 当前会返回的 A/AAAA
                目标。
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
                    label="候选目标"
                    value={node.candidate_targets.join(', ') || '—'}
                  />
                  <InfoTile
                    label="选中目标"
                    value={node.selected_targets.join(', ') || '—'}
                  />
                  <InfoTile
                    label="连接数"
                    value={
                      node.has_metric
                        ? formatCount(node.openresty_connections)
                        : '无指标'
                    }
                  />
                  <InfoTile
                    label="指标时间"
                    value={
                      node.has_metric
                        ? formatRelativeTime(node.metric_captured_at)
                        : '无新鲜指标'
                    }
                    helper={
                      node.has_metric
                        ? formatDateTime(node.metric_captured_at)
                        : undefined
                    }
                  />
                  <InfoTile
                    label="评分"
                    value={node.score > 0 ? node.score.toFixed(2) : '—'}
                  />
                  <InfoTile
                    label="Agent 探测"
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
                    label="探测 RTT"
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
                      {node.node_probe_message}
                    </span>
                  ) : null}
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {node.reasons.map((reason) => (
                    <span
                      key={`${node.node_id}-${reason}`}
                      className="rounded-xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-2.5 py-1 text-xs text-[var(--foreground-secondary)]"
                    >
                      {reason}
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
        {sourceCIDRs.length > 0 ? ` · CIDR ${sourceCIDRs.join(', ')}` : ''}
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
      description="把 Cloudflare 同步站点切换到自建权威 DNS 前，先检查 Zone、Worker、域名归属和站点 GSLB 配置。"
    >
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <InfoTile label="待迁移站点" value={formatCount(candidates.length)} />
        <InfoTile
          label="已权威 DNS"
          value={formatCount(authoritativeRoutes.length)}
        />
        <InfoTile
          label="可用 Zone"
          value={`${enabledZones.length} / ${zones.length}`}
        />
        <InfoTile
          label="在线 Worker"
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
                              ? 'Cloudflare 自动 DNS'
                              : 'Cloudflare 模式'
                          }
                          variant={candidate.dns_auto_sync ? 'info' : 'warning'}
                        />
                        {candidate.gslb_enabled ? (
                          <StatusBadge label="GSLB 已启用" variant="success" />
                        ) : null}
                      </div>
                      <p className="text-sm break-all text-[var(--foreground-secondary)]">
                        域名：{routeDomains.join('、') || '—'}
                      </p>
                      <div className="grid gap-3 md:grid-cols-3">
                        <InfoTile
                          label="匹配 Zone"
                          value={
                            candidate.matching_zone_name
                              ? `匹配 Zone ${candidate.matching_zone_name}`
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
                          label="公网可达 Worker"
                          value={`${candidate.public_reachable_worker_count} / ${candidate.online_worker_count}`}
                        />
                        <InfoTile
                          label="快照就绪 Worker"
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
                          message="Zone、域名归属、公网 UDP/TCP 53 探测和 Worker 调度快照已满足切换条件。"
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
              <li>1. 创建覆盖网站域名的 Zone，并填写注册商要使用的 NS。</li>
              <li>
                2. 部署至少两个 DNS Worker，确认 Worker
                在线、能拉取快照，并通过公网 UDP/TCP 53 探测。
              </li>
              <li>
                3. 在网站详情的「自动 DNS」里切换为自建权威 DNS 并选择 Zone。
              </li>
              <li>
                4. 到注册商把域名 NS 委派到 DNS Worker，再回到 Zone
                详情检查委派。
              </li>
            </ol>
          </div>
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              回滚路径
            </h3>
            <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
              如需回退，在网站详情把 DNS 模式改回 Cloudflare 同步，并在注册商把
              NS 改回原 DNS 服务商；DNS TTL 到期后解析会逐步回到原模式。
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
            {result.routeName} · Zone {result.zoneName}
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
          label="Worker 探测"
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
            ? '切换、委派、Worker 探测和 GSLB 模拟都已通过。'
            : '复测会帮助确认切换状态；委派不匹配时仍需要到注册商调整 NS 或 Glue。'
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
}: {
  summary: DNSObservabilitySummary | null;
  isLoading: boolean;
  error: string;
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
        description={`最近 ${dnsObservabilityWindowHours} 小时的 Worker 心跳聚合结果。`}
      >
        <EmptyState
          title="暂无 DNS 查询数据"
          description="DNS Worker 收到查询并上报心跳后，这里会展示查询量、错误码和返回目标分布。"
        />
      </AppCard>
    );
  }

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
          label="动态 GSLB"
          value={formatCount(summary.dynamic_queries)}
        />
        <InfoTile label="错误查询" value={formatCount(summary.error_queries)} />
      </div>

      <div className="mt-5 grid gap-4 xl:grid-cols-2">
        <DNSQueryTrendPanel summary={summary} />
        <DNSSnapshotConsistencyPanel
          consistency={summary.snapshot_consistency}
        />
        <DNSWorkerHealthPanel summary={summary} />
        <CounterList
          title="返回码"
          items={summary.rcode_breakdown}
          total={summary.total_queries}
        />
        <CounterList
          title="返回目标"
          items={summary.top_targets}
          total={summary.dynamic_queries || summary.total_queries}
          emptyText="暂无 A/AAAA 目标分布。"
        />
        <CounterList
          title="Worker 查询"
          items={summary.worker_breakdown}
          total={summary.total_queries}
        />
        <CounterList
          title="动态站点"
          items={summary.route_breakdown}
          total={summary.dynamic_queries}
          emptyText="暂无动态 GSLB 站点查询。"
        />
        <CounterList
          title="来源作用域"
          items={summary.source_scope_breakdown}
          total={summary.total_queries}
          emptyText="暂无来源作用域分布。"
          formatLabel={(item) => formatSourceScopeLabel(item.key || item.label)}
        />
      </div>
    </AppCard>
  );
}

function DNSWorkerHealthPanel({
  summary,
}: {
  summary: DNSObservabilitySummary;
}) {
  const health = summary.worker_health;

  if (!health) {
    return (
      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4 xl:col-span-2">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          Worker 可用性
        </h3>
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          暂无 Worker 可用性数据。
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4 xl:col-span-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
            Worker 可用性
          </h3>
          <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
            这里统计 DNS Worker
            本地处理查询的耗时、错误率和快照新鲜度，不代表用户到各地 NS 的网络
            RTT。
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
          label="多节点平均 RTT"
          value={formatLatencyMs(health.node_probe_average_rtt_ms ?? 0)}
        />
        <InfoTile
          label="多节点最大 RTT"
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
          暂无 DNS Worker。
        </p>
      ) : (
        <div className="mt-4 grid gap-3 lg:grid-cols-2">
          {health.workers.map((worker) => (
            <DNSWorkerHealthCard key={worker.worker_id} worker={worker} />
          ))}
        </div>
      )}
    </div>
  );
}

function DNSWorkerHealthCard({ worker }: { worker: DNSWorkerHealthItem }) {
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
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          <StatusBadge
            label={worker.status}
            variant={getWorkerStatusVariant(worker.status)}
          />
          <StatusBadge
            label={getProbeStatusLabel(worker.probe_status)}
            variant={getProbeStatusVariant(worker.probe_status)}
          />
          {worker.snapshot_stale ? (
            <StatusBadge label="快照过期" variant="danger" />
          ) : null}
          <StatusBadge
            label={worker.geoip_enabled ? 'GeoIP 已加载' : 'GeoIP 未加载'}
            variant={worker.geoip_enabled ? 'success' : 'warning'}
          />
        </div>
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
          label="快照年龄"
          value={formatDurationSeconds(worker.snapshot_age_seconds)}
        />
        <InfoTile
          label="最近心跳"
          value={formatRelativeTime(worker.last_seen_at)}
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
          label="多节点 RTT"
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
      {worker.geoip_last_error ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message={`GeoIP 国家库加载失败：${worker.geoip_last_error}`}
        />
      ) : !worker.geoip_enabled ? (
        <InlineMessage
          className="mt-3"
          tone="info"
          message="未加载 GeoIP 国家库；按国家代码匹配的 GSLB 节点池会回退到全局，来源 CIDR 匹配不受影响。"
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
      {(worker.node_probes ?? []).length > 0 ? (
        <div className="mt-4 space-y-2">
          <p className="text-xs font-medium text-[var(--foreground-primary)]">
            Agent 多节点探测
          </p>
          <div className="grid gap-2">
            {(worker.node_probes ?? []).map((probe) => (
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
                      {formatRelativeTime(probe.checked_at)}
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
              variant: 'area',
              valueFormatter: formatCount,
            },
            {
              label: 'SERVFAIL',
              color: '#dc2626',
              values: summary.trend_points.map(
                (point) => point.servfail_queries,
              ),
              valueFormatter: formatCount,
            },
            {
              label: 'NXDOMAIN',
              color: '#f59e0b',
              values: summary.trend_points.map(
                (point) => point.nxdomain_queries,
              ),
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
}: {
  consistency?: DNSWorkerSnapshotConsistency;
}) {
  if (!consistency) {
    return (
      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4">
        <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
          快照一致性
        </h3>
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          暂无 Worker 快照状态。
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
          快照一致性
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
      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <InfoTile
          label="在线 Worker"
          value={`${consistency.online_worker_count} / ${consistency.total_worker_count}`}
        />
        <InfoTile
          label="最新快照"
          value={consistency.latest_snapshot_version || '—'}
        />
        <InfoTile
          label="过期 Worker"
          value={formatCount(consistency.stale_worker_count)}
        />
        <InfoTile
          label="最大快照年龄"
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
                  {version.worker_count} 个 Worker
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

function CounterList({
  title,
  items,
  total,
  emptyText = '暂无数据。',
  formatLabel,
}: {
  title: string;
  items: DNSObservabilityCounterItem[];
  total: number;
  emptyText?: string;
  formatLabel?: (item: DNSObservabilityCounterItem) => string;
}) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-4">
      <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
        {title}
      </h3>
      {items.length === 0 ? (
        <p className="mt-3 text-sm text-[var(--foreground-secondary)]">
          {emptyText}
        </p>
      ) : (
        <div className="mt-3 space-y-3">
          {items.map((item) => {
            const percent =
              total > 0 ? Math.min(100, (item.count / total) * 100) : 0;
            return (
              <div key={`${title}-${item.key}`} className="space-y-1">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="min-w-0 truncate text-[var(--foreground-primary)]">
                    {formatLabel ? formatLabel(item) : item.label}
                  </span>
                  <span className="shrink-0 text-[var(--foreground-secondary)]">
                    {formatCount(item.count)}
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-[var(--surface-muted)]">
                  <div
                    className="h-full rounded-full bg-[var(--accent-primary)]"
                    style={{ width: `${percent}%` }}
                  />
                </div>
              </div>
            );
          })}
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
      title={zone ? '编辑 DNS Zone' : '创建 DNS Zone'}
      description="Zone 名称保存后会规范化为根域名格式；NS 至少建议填写两个可公网访问的 DNS Worker 名称。"
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
          label="Zone 名称"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="example.com"
            {...form.register('name', { required: '请输入 Zone 名称' })}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="SOA 邮箱"
            hint="留空时后端使用 hostmaster@zone。"
          >
            <ResourceInput
              placeholder="hostmaster@example.com"
              {...form.register('soa_email')}
            />
          </ResourceField>
          <ResourceField
            label="Primary NS"
            hint="留空时默认使用 NS 列表第一项。"
          >
            <ResourceInput
              placeholder="ns1.example.net"
              {...form.register('primary_ns')}
            />
          </ResourceField>
        </div>
        <ResourceField
          label="NS 列表"
          hint="每行一个 NS，也可用逗号或分号分隔。"
        >
          <ResourceTextarea
            placeholder={'ns1.example.net\nns2.example.net'}
            {...form.register('name_servers_text')}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField label="默认 TTL">
            <ResourceInput
              type="number"
              min={1}
              max={86400}
              {...form.register('default_ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ToggleField
            label="启用 Zone"
            description="停用后不会进入 DNS Worker 快照。"
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
      description={`当前 Zone：${zone.name}。记录名可填写 @、完整域名，或填写 www 这类相对名称。`}
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
            hint="@ 表示 Zone 根域。"
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
          <ResourceField label="TTL" hint="0 表示使用 Zone 默认 TTL。">
            <ResourceInput
              type="number"
              min={0}
              max={86400}
              {...form.register('ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ResourceField
            label="MX 优先级"
            hint="数字越小优先级越高；同一域名有多个 MX 时，邮件会优先投递到较小数值的服务器。"
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
            description="停用后不会进入 DNS Worker 快照。"
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
    },
  });
  const createMutation = useMutation({
    mutationFn: (values: WorkerFormValues) =>
      createDNSWorker({
        name: values.name.trim(),
        public_address: values.public_address.trim(),
      }),
    onSuccess: onCreated,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset({ name: '', public_address: '' });
    }
  }, [form, isOpen]);

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title="创建 DNS Worker"
      description="Token 只会在创建后返回一次；请在弹窗中复制部署命令。"
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
          label="Worker 名称"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="ns1-hk"
            {...form.register('name', { required: '请输入 Worker 名称' })}
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
        <PrimaryButton type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? '创建中...' : '创建'}
        </PrimaryButton>
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
  const installCommand = `curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \\
  --server-url ${serverUrl} \\
  --token ${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  --query-rate-limit 200 \\
  --udp-response-size 1232`;
  const dockerCommand = `docker run -d --name dushengcdn-dns-worker --restart unless-stopped \\
  -p 53:53/udp -p 53:53/tcp \\
  -v dushengcdn-dns-worker-data:/data \\
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=${serverUrl} \\
  -e DUSHENGCDN_DNS_WORKER_TOKEN=${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  -e DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200 \\
  -e DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=1232 \\
  ghcr.io/satands/dushengcdn-dns-worker:latest`;
  const sourceCommand = `cd dushengcdn_server
go run ./cmd/dns-worker \\
  --server-url ${serverUrl} \\
  --token ${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  --listen :53 \\
  --snapshot-path /var/lib/dushengcdn-dns-worker/snapshot.json \\
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
      title="DNS Worker Token"
      description={`Worker ${worker.name} 已创建。Token 离开弹窗后不会再次显示。`}
      size="xl"
    >
      <div className="space-y-5">
        {copyFeedback ? (
          <InlineMessage
            tone={copyFeedback.tone}
            message={copyFeedback.message}
          />
        ) : null}
        <ResourceField label="Worker Token">
          <div className="flex flex-col gap-3 md:flex-row">
            <ResourceInput readOnly value={token} className="font-mono" />
            <SecondaryButton
              type="button"
              onClick={() => void handleCopy(token, 'Token 已复制。')}
            >
              复制 Token
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
