'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  DNSGSLBSimulationPayload,
  DNSGSLBSimulationResult,
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
import { getProxyRoutes } from '@/features/proxy-routes/api/proxy-routes';
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

const dnsObservabilityWindowHours = 24;

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

function normalizeDNSName(value: string) {
  return value.trim().toLowerCase().replace(/\.$/, '');
}

function domainBelongsToZone(domain: string, zoneName: string) {
  const normalizedDomain = normalizeDNSName(domain);
  const normalizedZoneName = normalizeDNSName(zoneName);
  return (
    normalizedDomain === normalizedZoneName ||
    normalizedDomain.endsWith(`.${normalizedZoneName}`)
  );
}

function getRouteDomains(route: ProxyRouteItem) {
  const domains = route.domains?.length ? route.domains : [route.domain];
  return domains.map((domain) => domain.trim()).filter(Boolean);
}

function findMatchingZone(route: ProxyRouteItem, zones: DNSZoneItem[]) {
  const domains = getRouteDomains(route);
  if (domains.length === 0) {
    return null;
  }

  return (
    zones
      .filter((zone) =>
        domains.every((domain) => domainBelongsToZone(domain, zone.name)),
      )
      .sort((left, right) => right.name.length - left.name.length)[0] ?? null
  );
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
  return getRouteDomains(route)[0] ?? route.primary_domain ?? route.domain ?? '';
}

function getRouteRecordType(route?: ProxyRouteItem | null): 'A' | 'AAAA' {
  return route?.dns_record_type === 'AAAA' ? 'AAAA' : 'A';
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
  return {
    name: record?.name ?? '@',
    type: record?.type ?? 'A',
    value: record?.value ?? '',
    ttl: record?.ttl ?? 0,
    priority: record?.priority ?? 0,
    enabled: record?.enabled ?? true,
  };
}

function getRecordValueHint(type: DNSRecordType) {
  switch (type) {
    case 'A':
      return '填写 IPv4 地址。';
    case 'AAAA':
      return '填写 IPv6 地址。';
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

async function copyToClipboard(value: string) {
  await navigator.clipboard.writeText(value);
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

  const zones = useMemo(() => zonesQuery.data ?? [], [zonesQuery.data]);
  const workers = useMemo(() => workersQuery.data ?? [], [workersQuery.data]);
  const proxyRoutes = useMemo(
    () => proxyRoutesQuery.data ?? [],
    [proxyRoutesQuery.data],
  );
  const observability = observabilityQuery.data ?? null;
  const authoritativeRoutes = useMemo(
    () =>
      proxyRoutes
        .filter((route) => route.dns_provider_mode === 'authoritative')
        .filter((route) => route.enabled || route.gslb_enabled),
    [proxyRoutes],
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

        <DNSObservabilityPanel
          summary={observability}
          isLoading={observabilityQuery.isLoading}
          error={
            observabilityQuery.isError
              ? getErrorMessage(observabilityQuery.error)
              : ''
          }
        />

        <GSLBSimulationPanel
          routes={authoritativeRoutes}
          routesLoading={proxyRoutesQuery.isLoading}
          routesError={
            proxyRoutesQuery.isError ? getErrorMessage(proxyRoutesQuery.error) : ''
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
            zones={zones}
            workers={workers}
            routesLoading={proxyRoutesQuery.isLoading}
            routesError={
              proxyRoutesQuery.isError
                ? getErrorMessage(proxyRoutesQuery.error)
                : ''
            }
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
                  {worker.probe_message ? (
                    <p className="text-xs text-[var(--foreground-secondary)]">
                      探测状态：{worker.probe_message}
                      {worker.probe_age_seconds > 0
                        ? ` · ${formatDurationSeconds(worker.probe_age_seconds)}前`
                        : ''}
                    </p>
                  ) : null}
                  <DNSWorkerProbeResultPanel
                    probe={probeResults[worker.id] ?? workerProbeToPanelData(worker)}
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
      description="按站点、记录类型和来源国家预演当前权威 DNS 快照会返回的边缘 IP。"
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
                hint="例如 HK、DE；留空使用 global。"
              >
                <ResourceInput
                  maxLength={2}
                  placeholder="HK"
                  {...form.register('country')}
                />
              </ResourceField>
            </div>
            <ResourceField label="来源 IP" hint="可选，仅用于记录模拟输入。">
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
                  <InfoTile label="作用域" value={result.source_scope} />
                  <InfoTile label="TTL" value={`${result.ttl} 秒`} />
                  <InfoTile
                    label="策略"
                    value={result.strategy || (result.gslb_enabled ? 'GSLB' : '节点池')}
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
                  <InlineMessage tone="info" message={result.message} />
                ) : null}
              </div>
            ) : (
              <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
                选择站点和来源后点击模拟，可看到 DNS Worker 当前会返回的 A/AAAA 目标。
              </p>
            )}
          </div>
        </div>
      )}
    </AppCard>
  );
}

function DNSMigrationGuidePanel({
  routes,
  zones,
  workers,
  routesLoading,
  routesError,
}: {
  routes: ProxyRouteItem[];
  zones: DNSZoneItem[];
  workers: DNSWorkerItem[];
  routesLoading: boolean;
  routesError: string;
}) {
  const enabledZones = zones.filter((zone) => zone.enabled);
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  const publicReachableWorkers = onlineWorkers.filter(
    (worker) => worker.probe_healthy,
  );
  const candidates = routes
    .filter((route) => route.dns_provider_mode !== 'authoritative')
    .filter((route) => route.enabled || route.dns_auto_sync || route.gslb_enabled)
    .map((route) => {
      const matchingZone = findMatchingZone(route, zones);
      const blockers = [
        getRouteDomains(route).length === 0 ? '网站未配置域名' : '',
        !matchingZone ? '没有匹配的网站 Zone' : '',
        matchingZone && !matchingZone.enabled ? '匹配 Zone 已停用' : '',
        onlineWorkers.length === 0 ? '没有在线 DNS Worker' : '',
        onlineWorkers.length > 0 && publicReachableWorkers.length === 0
          ? '在线 DNS Worker 尚未通过公网 UDP/TCP 53 探测'
          : '',
      ].filter(Boolean);
      const warnings = [
        !route.gslb_enabled ? '未启用 GSLB，多节点池实时分流不会生效' : '',
        workers.length < 2 ? '生产环境建议至少部署 2 个 DNS Worker' : '',
        onlineWorkers.length > publicReachableWorkers.length
          ? '部分在线 Worker 未通过最新公网探测，迁移前建议逐个点击「探测」'
          : '',
        route.dns_auto_sync ? '' : '当前未启用 Cloudflare 自动 DNS，请确认是否仍需要迁移',
      ].filter(Boolean);

      return {
        route,
        matchingZone,
        blockers,
        warnings,
      };
    });
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
            candidates.map(({ route, matchingZone, blockers, warnings }) => {
              const ready = blockers.length === 0;
              const routeDomains = getRouteDomains(route);
              return (
                <div
                  key={route.id}
                  className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
                >
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0 space-y-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-base font-semibold break-all text-[var(--foreground-primary)]">
                          {route.site_name || route.primary_domain}
                        </h3>
                        <StatusBadge
                          label={ready ? '可切换' : '需处理'}
                          variant={ready ? 'success' : 'warning'}
                        />
                        <StatusBadge
                          label={
                            route.dns_auto_sync
                              ? 'Cloudflare 自动 DNS'
                              : 'Cloudflare 模式'
                          }
                          variant={route.dns_auto_sync ? 'info' : 'warning'}
                        />
                        {route.gslb_enabled ? (
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
                            matchingZone
                              ? `匹配 Zone ${matchingZone.name}`
                              : '未匹配'
                          }
                        />
                        <InfoTile
                          label="当前节点池"
                          value={getGSLBDescription(route)}
                        />
                        <InfoTile
                          label="记录类型"
                          value={route.dns_record_type || 'A'}
                        />
                        <InfoTile
                          label="公网可达 Worker"
                          value={`${publicReachableWorkers.length} / ${onlineWorkers.length}`}
                        />
                      </div>
                      {blockers.length > 0 ? (
                        <CheckList title="阻断项" items={blockers} tone="danger" />
                      ) : (
                        <InlineMessage
                          tone="success"
                          message="Zone、域名归属、在线 Worker 和公网 UDP/TCP 53 探测已满足切换条件。"
                        />
                      )}
                      {warnings.length > 0 ? (
                        <CheckList title="建议项" items={warnings} tone="info" />
                      ) : null}
                    </div>
                    <a
                      href={getRouteDetailHref(route)}
                      className="inline-flex shrink-0 items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
                    >
                      去网站详情
                    </a>
                  </div>
                </div>
              );
            })
          )}
        </div>

        <div className="space-y-4">
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              切换顺序
            </h3>
            <ol className="mt-3 space-y-3 text-sm leading-6 text-[var(--foreground-secondary)]">
              <li>1. 创建覆盖网站域名的 Zone，并填写注册商要使用的 NS。</li>
              <li>2. 部署至少两个 DNS Worker，确认 Worker 在线、能拉取快照，并通过公网 UDP/TCP 53 探测。</li>
              <li>3. 在网站详情的「自动 DNS」里切换为自建权威 DNS 并选择 Zone。</li>
              <li>4. 到注册商把域名 NS 委派到 DNS Worker，再回到 Zone 详情检查委派。</li>
            </ol>
          </div>
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              回滚路径
            </h3>
            <p className="mt-3 text-sm leading-6 text-[var(--foreground-secondary)]">
              如需回退，在网站详情把 DNS 模式改回 Cloudflare 同步，并在注册商把 NS 改回原 DNS 服务商；DNS TTL 到期后解析会逐步回到原模式。
            </p>
          </div>
        </div>
      </div>
    </AppCard>
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
        />
      </div>
    </AppCard>
  );
}

function DNSWorkerHealthPanel({ summary }: { summary: DNSObservabilitySummary }) {
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
            这里统计 DNS Worker 本地处理查询的耗时、错误率和快照新鲜度，不代表用户到各地 NS 的网络 RTT。
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
      </div>

      {worker.last_error ? (
        <InlineMessage
          className="mt-3"
          tone="danger"
          message={worker.last_error}
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
          message={
            consistency.status === 'stale'
              ? '存在在线 Worker 快照超过最大有效时间，请检查 Worker 到 Server 的网络和 Token。'
              : '在线 Worker 当前使用了不同快照版本，查询结果可能不一致。'
          }
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
}: {
  title: string;
  items: DNSObservabilityCounterItem[];
  total: number;
  emptyText?: string;
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
                    {item.label}
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

function InfoTile({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
      <p className="text-xs tracking-[0.18em] text-[var(--foreground-muted)] uppercase">
        {label}
      </p>
      <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
        {value}
      </p>
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
  const saveMutation = useMutation({
    mutationFn: (values: RecordFormValues) => {
      const payload: DNSRecordMutationPayload = {
        zone_id: zone.id,
        name: values.name.trim(),
        type: values.type,
        value: values.value.trim(),
        ttl: values.ttl,
        priority: values.type === 'MX' ? values.priority : 0,
        enabled: values.enabled,
      };
      return record
        ? updateDNSRecord(record.id, payload)
        : createDNSZoneRecord(zone.id, payload);
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
        <ResourceField
          label="记录值"
          hint={getRecordValueHint(recordType)}
          error={form.formState.errors.value?.message}
        >
          <ResourceTextarea
            placeholder={recordType === 'TXT' ? 'v=spf1 ...' : '记录值'}
            {...form.register('value', { required: '请输入记录值' })}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-3">
          <ResourceField label="TTL" hint="0 表示使用 Zone 默认 TTL。">
            <ResourceInput
              type="number"
              min={0}
              max={86400}
              {...form.register('ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ResourceField label="MX 优先级">
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
  const [copyMessage, setCopyMessage] = useState('');
  const token = worker.token ?? '';
  const dockerCommand = `docker run -d --name dushengcdn-dns-worker --restart unless-stopped \\
  -p 53:53/udp -p 53:53/tcp \\
  -v dushengcdn-dns-worker-data:/data \\
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=${serverUrl} \\
  -e DUSHENGCDN_DNS_WORKER_TOKEN=${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  ghcr.io/satands/dushengcdn-dns-worker:latest`;
  const sourceCommand = `cd dushengcdn_server
go run ./cmd/dns-worker \\
  --server-url ${serverUrl} \\
  --token ${token || 'YOUR_DNS_WORKER_TOKEN'} \\
  --listen :53 \\
  --snapshot-path /var/lib/dushengcdn-dns-worker/snapshot.json`;

  const handleCopy = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setCopyMessage(message);
    } catch (error) {
      setCopyMessage(getErrorMessage(error));
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
        {copyMessage ? (
          <InlineMessage tone="success" message={copyMessage} />
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
