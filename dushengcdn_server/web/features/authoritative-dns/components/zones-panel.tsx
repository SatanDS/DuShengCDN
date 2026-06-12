'use client';
import { Copy, KeyRound, RotateCw, ShieldCheck } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  DNSRecordItem,
  DNSSECDenialMode,
  DNSSECStatus,
  DNSWorkerItem,
  DNSZoneDelegationCheck,
  DNSZoneItem,
  DNSZoneWorkerAssignment,
} from '@/features/authoritative-dns/types';
import {
  CodeBlock,
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';
import { copyToClipboard } from '@/lib/utils/clipboard';
import { formatDateTime } from '@/lib/utils/date';
import {
  getDNSWorkerDisplayName,
  isPriorityRecordType,
  formatDurationSeconds,
  getDelegationStatusLabel,
  getDelegationStatusVariant,
} from './authoritative-dns-page.helpers';
import { InfoTile } from './info-tile';

export function ZonesPanel({
  zones,
  selectedZone,
  records,
  recordsLoading,
  recordsError,
  delegationCheck,
  delegationLoading,
  delegationError,
  workers,
  zoneWorkerAssignment,
  zoneWorkerAssignmentLoading,
  zoneWorkerAssignmentError,
  zoneWorkerAssignmentSaving,
  dnssec,
  dnssecLoading,
  dnssecError,
  dnssecBusy,
  busy,
  onSelectZone,
  onCreateZone,
  onEditZone,
  onDeleteZone,
  onCheckDelegation,
  onSaveZoneWorkers,
  onEnableDNSSEC,
  onDisableDNSSEC,
  onRotateZSK,
  onRotateKSK,
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
  workers: DNSWorkerItem[];
  zoneWorkerAssignment: DNSZoneWorkerAssignment | null;
  zoneWorkerAssignmentLoading: boolean;
  zoneWorkerAssignmentError: string;
  zoneWorkerAssignmentSaving: boolean;
  dnssec: DNSSECStatus | null;
  dnssecLoading: boolean;
  dnssecError: string;
  dnssecBusy: boolean;
  busy: boolean;
  onSelectZone: (zone: DNSZoneItem) => void;
  onCreateZone: () => void;
  onEditZone: (zone: DNSZoneItem) => void;
  onDeleteZone: (zone: DNSZoneItem) => void;
  onCheckDelegation: () => void;
  onSaveZoneWorkers: (zone: DNSZoneItem, workerIds: number[]) => void;
  onEnableDNSSEC: (
    zone: DNSZoneItem,
    denialMode: DNSSECDenialMode,
    nsec3Iterations: number,
  ) => void;
  onDisableDNSSEC: (zone: DNSZoneItem) => void;
  onRotateZSK: (zone: DNSZoneItem) => void;
  onRotateKSK: (zone: DNSZoneItem) => void;
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
                    {zone.dnssec_enabled ? (
                      <StatusBadge label="DNSSEC" variant="success" />
                    ) : null}
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
                      <StatusBadge
                        label={
                          selectedZone.dnssec_enabled
                            ? `DNSSEC ${selectedZone.dnssec_denial_mode.toUpperCase()}`
                            : 'DNSSEC 未启用'
                        }
                        variant={
                          selectedZone.dnssec_enabled ? 'success' : 'warning'
                        }
                      />
                    </div>
                    <p className="text-sm leading-6 text-[var(--foreground-secondary)]">
                      基础邮箱 {selectedZone.soa_email || 'hostmaster'}
                      ，默认缓存时间 {selectedZone.default_ttl} 秒。
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
                      暂未配置注册商 NS。生产环境至少配置两个 DNS 响应端对应的
                      NS。
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

              <DNSSECPanel
                zone={selectedZone}
                dnssec={dnssec}
                isLoading={dnssecLoading}
                isBusy={dnssecBusy}
                error={dnssecError}
                onEnable={(denialMode, nsec3Iterations) =>
                  onEnableDNSSEC(selectedZone, denialMode, nsec3Iterations)
                }
                onDisable={() => onDisableDNSSEC(selectedZone)}
                onRotateZSK={() => onRotateZSK(selectedZone)}
                onRotateKSK={() => onRotateKSK(selectedZone)}
              />

              <ZoneWorkerAssignmentPanel
                zone={selectedZone}
                workers={workers}
                assignment={zoneWorkerAssignment}
                isLoading={zoneWorkerAssignmentLoading}
                isSaving={zoneWorkerAssignmentSaving}
                error={zoneWorkerAssignmentError}
                onSave={(workerIds) =>
                  onSaveZoneWorkers(selectedZone, workerIds)
                }
              />

              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
                <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                  <div>
                    <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
                      静态记录
                    </h3>
                    <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
                      手动固定回答的记录；同名 A/AAAA/CNAME
                      会和网站配置里的本地自建解析自动记录互斥。
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
                                {isPriorityRecordType(record.type)
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

export function DelegationCheckPanel({
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

export function DNSSECPanel({
  zone,
  dnssec,
  isLoading,
  isBusy,
  error,
  onEnable,
  onDisable,
  onRotateZSK,
  onRotateKSK,
}: {
  zone: DNSZoneItem;
  dnssec: DNSSECStatus | null;
  isLoading: boolean;
  isBusy: boolean;
  error: string;
  onEnable: (denialMode: DNSSECDenialMode, nsec3Iterations: number) => void;
  onDisable: () => void;
  onRotateZSK: () => void;
  onRotateKSK: () => void;
}) {
  const [denialMode, setDenialMode] = useState<DNSSECDenialMode>(
    zone.dnssec_denial_mode === 'nsec3' ? 'nsec3' : 'nsec',
  );
  const [nsec3Iterations, setNsec3Iterations] = useState(
    String(zone.dnssec_nsec3_iterations ?? 0),
  );

  useEffect(() => {
    setDenialMode(zone.dnssec_denial_mode === 'nsec3' ? 'nsec3' : 'nsec');
    setNsec3Iterations(String(zone.dnssec_nsec3_iterations ?? 0));
  }, [zone.id, zone.dnssec_denial_mode, zone.dnssec_nsec3_iterations]);

  const visibleDNSSEC = dnssec?.zone_id === zone.id ? dnssec : null;
  const enabled = Boolean(visibleDNSSEC?.enabled ?? zone.dnssec_enabled);
  const encryptionConfigured = Boolean(
    visibleDNSSEC?.key_encryption_configured,
  );
  const activeKeys =
    visibleDNSSEC?.keys.filter((key) => key.status === 'active') ?? [];
  const dsRecords = visibleDNSSEC?.ds_records ?? [];
  const parsedIterations = Number.parseInt(nsec3Iterations, 10);
  const canEnable =
    encryptionConfigured &&
    !isBusy &&
    !Number.isNaN(parsedIterations) &&
    parsedIterations >= 0 &&
    parsedIterations <= 50;

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
              DNSSEC
            </h3>
            <StatusBadge
              label={enabled ? '已启用' : '未启用'}
              variant={enabled ? 'success' : 'warning'}
            />
            <StatusBadge
              label={encryptionConfigured ? '主密钥已配置' : '主密钥未配置'}
              variant={encryptionConfigured ? 'success' : 'danger'}
            />
          </div>
          <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
            为该托管域名生成 DNSKEY/RRSIG 和否定证明，并导出注册商需要配置的 DS
            记录。
          </p>
        </div>
        {enabled ? (
          <div className="flex flex-wrap gap-2">
            <SecondaryButton
              type="button"
              disabled={isBusy}
              onClick={onRotateZSK}
            >
              <RotateCw className="mr-2 h-4 w-4" aria-hidden="true" />
              轮换 ZSK
            </SecondaryButton>
            <SecondaryButton
              type="button"
              disabled={isBusy}
              onClick={onRotateKSK}
            >
              <KeyRound className="mr-2 h-4 w-4" aria-hidden="true" />
              轮换 KSK
            </SecondaryButton>
            <DangerButton type="button" disabled={isBusy} onClick={onDisable}>
              关闭 DNSSEC
            </DangerButton>
          </div>
        ) : null}
      </div>

      <div className="mt-4">
        {isLoading ? (
          <LoadingState />
        ) : error ? (
          <InlineMessage tone="danger" message={error} />
        ) : !visibleDNSSEC ? (
          <InlineMessage tone="warning" message="DNSSEC 状态暂未加载。" />
        ) : !enabled ? (
          <div className="space-y-4">
            {!encryptionConfigured ? (
              <InlineMessage
                tone="danger"
                message="Server 未配置 DUSHENGCDN_DNSSEC_KEY_ENCRYPTION_KEY，不能启用 DNSSEC。"
              />
            ) : null}
            <div className="grid gap-4 md:grid-cols-3">
              <ResourceField label="否定证明模式" container="div">
                <ResourceSelect
                  value={denialMode}
                  disabled={isBusy}
                  onChange={(event) =>
                    setDenialMode(
                      event.target.value === 'nsec3' ? 'nsec3' : 'nsec',
                    )
                  }
                >
                  <option value="nsec">NSEC</option>
                  <option value="nsec3">NSEC3</option>
                </ResourceSelect>
              </ResourceField>
              <ResourceField
                label="NSEC3 iterations"
                hint="NSEC 模式会自动使用 0。"
              >
                <ResourceInput
                  type="number"
                  min={0}
                  max={50}
                  value={nsec3Iterations}
                  disabled={isBusy || denialMode !== 'nsec3'}
                  onChange={(event) => setNsec3Iterations(event.target.value)}
                />
              </ResourceField>
              <div className="flex items-end">
                <PrimaryButton
                  type="button"
                  disabled={!canEnable}
                  onClick={() =>
                    onEnable(
                      denialMode,
                      denialMode === 'nsec3' ? parsedIterations : 0,
                    )
                  }
                >
                  <ShieldCheck className="mr-2 h-4 w-4" aria-hidden="true" />
                  {isBusy ? '启用中...' : '启用 DNSSEC'}
                </PrimaryButton>
              </div>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="grid gap-3 md:grid-cols-4">
              <InfoTile
                label="算法"
                value={
                  visibleDNSSEC.algorithm_name ||
                  String(visibleDNSSEC.algorithm)
                }
              />
              <InfoTile
                label="否定证明"
                value={visibleDNSSEC.denial_mode.toUpperCase()}
                helper={
                  visibleDNSSEC.denial_mode === 'nsec3'
                    ? `salt ${visibleDNSSEC.nsec3_salt || '-'} · ${visibleDNSSEC.nsec3_iterations} iterations`
                    : 'NSEC'
                }
              />
              <InfoTile
                label="签名有效期"
                value={formatDurationSeconds(
                  visibleDNSSEC.signature_validity_seconds,
                )}
              />
              <InfoTile label="活动密钥" value={`${activeKeys.length} 个`} />
            </div>

            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                  注册商 DS 记录
                </p>
                {dsRecords.length > 0 ? (
                  <SecondaryButton
                    type="button"
                    onClick={() =>
                      void copyToClipboard(
                        dsRecords.map((item) => item.record).join('\n'),
                      )
                    }
                  >
                    <Copy className="mr-2 h-4 w-4" aria-hidden="true" />
                    复制 DS
                  </SecondaryButton>
                ) : null}
              </div>
              {dsRecords.length > 0 ? (
                <div className="mt-3 space-y-2">
                  {dsRecords.map((record) => (
                    <CodeBlock
                      key={`${record.key_tag}-${record.digest_type}-${record.digest}`}
                      className="break-all whitespace-pre-wrap"
                    >
                      {record.record}
                    </CodeBlock>
                  ))}
                </div>
              ) : (
                <p className="mt-2 text-sm text-[var(--foreground-secondary)]">
                  暂无 DS 记录。
                </p>
              )}
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              {visibleDNSSEC.keys.map((key) => (
                <div
                  key={key.id}
                  className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge
                      label={key.role.toUpperCase()}
                      variant={key.role === 'ksk' ? 'info' : 'success'}
                    />
                    <StatusBadge
                      label={key.status === 'active' ? '活动' : '已退役'}
                      variant={key.status === 'active' ? 'success' : 'warning'}
                    />
                    <span className="text-sm font-semibold text-[var(--foreground-primary)]">
                      Key Tag {key.key_tag}
                    </span>
                  </div>
                  <p className="mt-2 text-xs break-all text-[var(--foreground-secondary)]">
                    {key.algorithm_name || key.algorithm} · flags {key.flags}
                  </p>
                  <p className="mt-1 text-xs text-[var(--foreground-muted)]">
                    创建时间：{formatDateTime(key.created_at)}
                  </p>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export function ZoneWorkerAssignmentPanel({
  zone,
  workers,
  assignment,
  isLoading,
  isSaving,
  error,
  onSave,
}: {
  zone: DNSZoneItem;
  workers: DNSWorkerItem[];
  assignment: DNSZoneWorkerAssignment | null;
  isLoading: boolean;
  isSaving: boolean;
  error: string;
  onSave: (workerIds: number[]) => void;
}) {
  const assignmentForZone =
    assignment && assignment.zone_id === zone.id ? assignment : null;
  const assignedWorkerIds = useMemo(
    () => assignmentForZone?.worker_ids ?? [],
    [assignmentForZone?.worker_ids],
  );
  const [selectedWorkerIds, setSelectedWorkerIds] = useState<number[]>([]);

  useEffect(() => {
    setSelectedWorkerIds(assignedWorkerIds);
  }, [assignmentForZone?.zone_id, assignedWorkerIds]);

  const selectedSet = new Set(selectedWorkerIds);
  const allWorkersSelected =
    selectedWorkerIds.length === 0 ||
    (workers.length > 0 && selectedWorkerIds.length === workers.length);
  const activeWorkers = allWorkersSelected
    ? workers
    : workers.filter((worker) => selectedSet.has(worker.id));

  const setWorkerChecked = (workerId: number, checked: boolean) => {
    setSelectedWorkerIds((current) => {
      const currentAll =
        current.length === 0 ||
        (workers.length > 0 && current.length === workers.length);
      const next = new Set(
        currentAll ? workers.map((worker) => worker.id) : current,
      );
      if (checked) {
        next.add(workerId);
      } else {
        next.delete(workerId);
      }
      return Array.from(next);
    });
  };

  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
              响应端范围
            </h3>
            <StatusBadge
              label={
                allWorkersSelected
                  ? '全部响应端'
                  : `${activeWorkers.length} 个响应端`
              }
              variant={allWorkersSelected ? 'info' : 'success'}
            />
          </div>
          <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
            限制该托管域名下发到哪些 DNS 响应端。留空表示所有响应端都可回答。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <SecondaryButton
            type="button"
            disabled={isLoading || isSaving}
            onClick={() => setSelectedWorkerIds([])}
          >
            使用全部
          </SecondaryButton>
          <PrimaryButton
            type="button"
            disabled={isLoading || isSaving}
            onClick={() => onSave(allWorkersSelected ? [] : selectedWorkerIds)}
          >
            {isSaving ? '保存中...' : '保存范围'}
          </PrimaryButton>
        </div>
      </div>

      <div className="mt-4">
        {isLoading ? (
          <LoadingState />
        ) : error ? (
          <InlineMessage tone="danger" message={error} />
        ) : workers.length === 0 ? (
          <EmptyState
            title="暂无 DNS 响应端"
            description="创建 DNS 响应端后，可按托管域名限制哪些响应端接收解析配置。"
          />
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {workers.map((worker) => {
              const checked = allWorkersSelected || selectedSet.has(worker.id);
              return (
                <label
                  key={worker.id}
                  className="flex min-h-14 items-start gap-3 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-panel)] px-4 py-3 text-sm text-[var(--foreground-primary)]"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    disabled={isSaving}
                    onChange={(event) =>
                      setWorkerChecked(worker.id, event.target.checked)
                    }
                    className="mt-1 h-4 w-4 shrink-0 accent-[var(--brand-primary)]"
                  />
                  <span className="min-w-0">
                    <span className="block font-medium break-all">
                      {getDNSWorkerDisplayName(worker)}
                    </span>
                    <span className="mt-1 block text-xs break-all text-[var(--foreground-secondary)]">
                      {worker.public_address || worker.worker_id}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

export function NameServerList({
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
