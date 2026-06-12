'use client';
import { EmptyState } from '@/components/feedback/empty-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  DNSWorkerItem,
  DNSWorkerProbe,
} from '@/features/authoritative-dns/types';
import {
  DangerButton,
  PrimaryButton,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';
import {
  isMeaningfulTime,
  getDNSWorkerDisplayName,
  getWorkerStatusVariant,
  getWorkerSourceCapabilityMessage,
  getWorkerSourceCapabilityTone,
  formatCount,
  formatLatencyMs,
  formatDurationSeconds,
  getProbeResultVariant,
  getProbeStatusLabel,
  getProbeStatusVariant,
  workerProbeToPanelData,
} from './authoritative-dns-page.helpers';
import { InfoTile } from './info-tile';

export function WorkersPanel({
  workers,
  busy,
  rotatingWorkerId,
  revokingWorkerId,
  probingWorkerId,
  probeResults,
  onCreateWorker,
  onProbeWorker,
  onRotateToken,
  onRevokeToken,
}: {
  workers: DNSWorkerItem[];
  busy: boolean;
  rotatingWorkerId: number | null;
  revokingWorkerId: number | null;
  probingWorkerId: number | null;
  probeResults: Record<number, DNSWorkerProbe>;
  onCreateWorker: () => void;
  onProbeWorker: (worker: DNSWorkerItem) => void;
  onRotateToken: (worker: DNSWorkerItem) => void;
  onRevokeToken: (worker: DNSWorkerItem) => void;
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
                      {getDNSWorkerDisplayName(worker)}
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
                        worker.token_revoked_at
                          ? '密钥已吊销'
                          : worker.token_prefix
                            ? '密钥有效'
                            : '密钥未生成'
                      }
                      variant={
                        worker.token_revoked_at
                          ? 'danger'
                          : worker.token_prefix
                            ? 'success'
                            : 'warning'
                      }
                    />
                    <StatusBadge
                      label={
                        worker.geoip_enabled
                          ? '国家识别库已加载'
                          : '国家识别库未加载'
                      }
                      variant={worker.geoip_enabled ? 'success' : 'warning'}
                    />
                    <StatusBadge
                      label={
                        worker.geoip_country_enabled ? '国家支持' : '国家未支持'
                      }
                      variant={
                        worker.geoip_country_enabled ? 'success' : 'warning'
                      }
                    />
                    <StatusBadge
                      label={
                        worker.geoip_asn_enabled ? 'ASN 支持' : 'ASN 未支持'
                      }
                      variant={worker.geoip_asn_enabled ? 'success' : 'warning'}
                    />
                    <StatusBadge
                      label={
                        worker.geoip_operator_enabled
                          ? '运营商支持'
                          : '运营商未支持'
                      }
                      variant={
                        worker.geoip_operator_enabled ? 'success' : 'warning'
                      }
                    />
                  </div>
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
                    <InfoTile label="响应端 ID" value={worker.worker_id} />
                    <InfoTile
                      label="公网地址"
                      value={worker.public_address || '—'}
                    />
                    <InfoTile
                      label="密钥前缀"
                      value={worker.token_prefix || '—'}
                      helper={
                        worker.token_revoked_at
                          ? `已吊销 ${formatRelativeTime(worker.token_revoked_at)}`
                          : '仅显示前缀，完整密钥只在创建或重新生成后显示。'
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
                      <p>
                        国家识别库：{worker.geoip_database_path || '已加载'}
                      </p>
                      <p>
                        ASN 识别库：
                        {worker.asn_database_path || '未配置独立 ASN 库'}
                      </p>
                      <p>
                        运营商 CIDR 库：
                        {worker.operator_cidr_database_path || '未配置'}
                      </p>
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
                  <SecondaryButton
                    type="button"
                    disabled={rotatingWorkerId === worker.id}
                    onClick={() => onRotateToken(worker)}
                  >
                    {rotatingWorkerId === worker.id
                      ? '生成中...'
                      : '重新生成密钥'}
                  </SecondaryButton>
                  <DangerButton
                    type="button"
                    disabled={
                      revokingWorkerId === worker.id ||
                      Boolean(worker.token_revoked_at)
                    }
                    onClick={() => onRevokeToken(worker)}
                  >
                    {revokingWorkerId === worker.id ? '吊销中...' : '吊销密钥'}
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

export function DNSWorkerProbeResultPanel({
  probe,
}: {
  probe?: DNSWorkerProbe;
}) {
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
