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
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { AppModal } from '@/components/ui/app-modal';
import { StatusBadge } from '@/components/ui/status-badge';
import {
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
  updateDNSRecord,
  updateDNSZone,
} from '@/features/authoritative-dns/api/authoritative-dns';
import type {
  DNSObservabilityCounterItem,
  DNSObservabilitySummary,
  DNSRecordItem,
  DNSRecordMutationPayload,
  DNSRecordType,
  DNSWorkerItem,
  DNSZoneItem,
  DNSZoneMutationPayload,
} from '@/features/authoritative-dns/types';
import { getErrorMessage } from '@/features/proxy-routes/helpers';
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

type ActiveTab = 'zones' | 'workers';

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

function linesFromText(value: string) {
  return value
    .split(/[\r\n,，;；]+/)
    .map((item) => item.trim())
    .filter(Boolean);
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
  const [editingRecord, setEditingRecord] = useState<DNSRecordItem | null>(null);
  const [recordZone, setRecordZone] = useState<DNSZoneItem | null>(null);
  const [isWorkerModalOpen, setIsWorkerModalOpen] = useState(false);
  const [createdWorker, setCreatedWorker] = useState<DNSWorkerItem | null>(null);
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
  const observability = observabilityQuery.data ?? null;
  const selectedZone = useMemo(
    () =>
      zones.find((zone) => zone.id === selectedZoneId) ??
      zones[0] ??
      null,
    [selectedZoneId, zones],
  );
  const selectedZoneRecordsQuery = useQuery({
    queryKey: ['authoritative-dns', 'zone-records', selectedZone?.id],
    queryFn: () => getDNSZoneRecords(selectedZone?.id ?? 0),
    enabled: Boolean(selectedZone?.id),
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
        ) : (
          <WorkersPanel
            workers={workers}
            onCreateWorker={() => setIsWorkerModalOpen(true)}
            onDeleteWorker={handleDeleteWorker}
            busy={deleteWorkerMutation.isPending}
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
  busy,
  onSelectZone,
  onCreateZone,
  onEditZone,
  onDeleteZone,
  onCreateRecord,
  onEditRecord,
  onDeleteRecord,
}: {
  zones: DNSZoneItem[];
  selectedZone: DNSZoneItem | null;
  records: DNSRecordItem[];
  recordsLoading: boolean;
  recordsError: string;
  busy: boolean;
  onSelectZone: (zone: DNSZoneItem) => void;
  onCreateZone: () => void;
  onEditZone: (zone: DNSZoneItem) => void;
  onDeleteZone: (zone: DNSZoneItem) => void;
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
                    <ErrorState title="记录加载失败" description={recordsError} />
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
                                <p className="break-all text-sm font-semibold text-[var(--foreground-primary)]">
                                  {record.name}
                                </p>
                              </div>
                              <p className="break-all text-sm text-[var(--foreground-secondary)]">
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

function WorkersPanel({
  workers,
  busy,
  onCreateWorker,
  onDeleteWorker,
}: {
  workers: DNSWorkerItem[];
  busy: boolean;
  onCreateWorker: () => void;
  onDeleteWorker: (worker: DNSWorkerItem) => void;
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
                </div>
                <DangerButton
                  type="button"
                  disabled={busy}
                  onClick={() => onDeleteWorker(worker)}
                >
                  删除
                </DangerButton>
              </div>
            </div>
          ))}
        </div>
      )}
    </AppCard>
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

  if (!summary || summary.total_queries <= 0) {
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
          value={formatPercent(summary.successful_queries, summary.total_queries)}
        />
        <InfoTile
          label="动态 GSLB"
          value={formatCount(summary.dynamic_queries)}
        />
        <InfoTile
          label="错误查询"
          value={formatCount(summary.error_queries)}
        />
      </div>

      <div className="mt-5 grid gap-4 xl:grid-cols-2">
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
      </div>
    </AppCard>
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
            const percent = total > 0 ? Math.min(100, (item.count / total) * 100) : 0;
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
      <p className="mt-2 break-all text-sm text-[var(--foreground-primary)]">
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
          <CodeBlock className="whitespace-pre-wrap break-all">
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
          <CodeBlock className="whitespace-pre-wrap break-all">
            {sourceCommand}
          </CodeBlock>
        </div>
      </div>
    </AppModal>
  );
}
