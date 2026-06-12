'use client';
import { Settings } from 'lucide-react';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { RankChart } from '@/components/data/rank-chart';
import { TrendChart } from '@/components/data/trend-chart';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  DNSObservabilityCounterItem,
  DNSObservabilitySummary,
  DNSWorkerHealthItem,
  DNSWorkerSnapshotConsistency,
} from '@/features/authoritative-dns/types';
import { SecondaryButton } from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';
import {
  isMeaningfulTime,
  getDNSWorkerDisplayName,
  dnsObservabilityWindowOptions,
  getWorkerStatusVariant,
  formatCount,
  formatPercent,
  formatPercentValue,
  formatLatencyMs,
  formatSourceScopeLabel,
  isSourceScopeASNItem,
  isSourceScopeCountryItem,
  formatSourceCountryLabel,
  formatSourceASNLabel,
  formatDNSRCodeLabel,
  formatDurationSeconds,
  getProbeResultVariant,
  getProbeStatusLabel,
  getProbeStatusVariant,
  getNodeDNSProbeStatusLabel,
  getNodeProbeStatusVariant,
  formatTrendHour,
  getSnapshotConsistencyLabel,
  getSnapshotConsistencyVariant,
  getSnapshotConsistencyMessage,
  getDNSObservabilityRollupHint,
} from './authoritative-dns-page.helpers';
import { InfoTile } from './info-tile';

export function DNSObservabilityWindowSelector({
  selectedKey,
  onChange,
}: {
  selectedKey: string;
  onChange: (key: string) => void;
}) {
  return (
    <div
      className="flex flex-wrap rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-1"
      aria-label="DNS 查询观测时间范围"
    >
      {dnsObservabilityWindowOptions.map((option) => {
        const active = option.key === selectedKey;
        return (
          <button
            key={option.key}
            type="button"
            aria-pressed={active}
            className={cn(
              'rounded-xl px-3 py-1.5 text-xs font-medium transition',
              active
                ? 'bg-[var(--brand-primary)] text-[var(--foreground-inverse)] shadow-[var(--shadow-soft)]'
                : 'text-[var(--foreground-secondary)] hover:bg-[var(--control-background-hover)] hover:text-[var(--foreground-primary)]',
            )}
            onClick={() => onChange(option.key)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

export function DNSObservabilityPanel({
  summary,
  isLoading,
  error,
  selectedWindowKey,
  selectedWindowHours,
  onWindowChange,
  onCopyCommand,
  onOpenWorkerSettings,
}: {
  summary: DNSObservabilitySummary | null;
  isLoading: boolean;
  error: string;
  selectedWindowKey: string;
  selectedWindowHours: number;
  onWindowChange: (key: string) => void;
  onCopyCommand: (value: string, message: string) => void;
  onOpenWorkerSettings?: (worker: DNSWorkerHealthItem) => void;
}) {
  const windowSelector = (
    <DNSObservabilityWindowSelector
      selectedKey={selectedWindowKey}
      onChange={onWindowChange}
    />
  );

  if (isLoading) {
    return (
      <AppCard title="DNS 查询观测" action={windowSelector}>
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
        description={`最近 ${selectedWindowHours} 小时的响应端上报汇总。`}
        action={windowSelector}
      >
        <EmptyState
          title="暂无 DNS 查询数据"
          description="DNS 响应端收到查询并上报后，这里会展示查询量、错误码和返回目标分布。"
        />
      </AppCard>
    );
  }

  const rollupHint = getDNSObservabilityRollupHint(summary);
  const legacySourceASNBreakdown =
    summary.source_scope_breakdown.filter(isSourceScopeASNItem);
  const legacySourceRegionBreakdown = summary.source_scope_breakdown.filter(
    isSourceScopeCountryItem,
  );
  const sourceASNBreakdown =
    summary.source_asn_breakdown.length > 0
      ? summary.source_asn_breakdown
      : legacySourceASNBreakdown;
  const sourceRegionBreakdown =
    summary.source_country_breakdown.length > 0
      ? summary.source_country_breakdown
      : legacySourceRegionBreakdown;
  const formatSourceRegionChartLabel =
    summary.source_country_breakdown.length > 0
      ? formatSourceCountryLabel
      : (item: DNSObservabilityCounterItem) =>
          formatSourceScopeLabel(item.key || item.label);
  const formatSourceASNChartLabel =
    summary.source_asn_breakdown.length > 0
      ? formatSourceASNLabel
      : (item: DNSObservabilityCounterItem) =>
          formatSourceScopeLabel(item.key || item.label);

  return (
    <AppCard
      title="DNS 查询观测"
      description={`最近 ${summary.window_hours} 小时聚合查询；最近上报 ${formatRelativeTime(summary.last_rollup_at)}。`}
      action={windowSelector}
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
        <InlineMessage className="mt-4" tone="warning" message={rollupHint} />
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
          title="来源地区"
          items={sourceRegionBreakdown}
          total={summary.total_queries}
          emptyText="暂无来源地区分布。"
          formatLabel={formatSourceRegionChartLabel}
          color="#14b8a6"
        />
        <CounterChart
          title="来源 ASN"
          items={sourceASNBreakdown}
          total={summary.total_queries}
          emptyText="暂无来源 ASN 分布。"
          formatLabel={formatSourceASNChartLabel}
          color="#8b5cf6"
        />
      </div>
    </AppCard>
  );
}

export function DNSWorkerHealthPanel({
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
            这里统计 DNS
            响应端本地处理查询的耗时、错误率和解析配置新鲜度；不代表用户到各地
            NS 的公网网络耗时。
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

export function DNSWorkerHealthCard({
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
            {getDNSWorkerDisplayName(worker)}
          </p>
          <p className="mt-1 text-xs break-all text-[var(--foreground-muted)]">
            {worker.public_address || worker.worker_id}
          </p>
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
                label={
                  isWaitingForUnsupportedUpdate ? '需手动升级' : '等待更新'
                }
                variant="warning"
              />
            ) : null}
            <StatusBadge
              label={
                worker.geoip_enabled ? '国家识别库已加载' : '国家识别库未加载'
              }
              variant={worker.geoip_enabled ? 'success' : 'warning'}
            />
            <StatusBadge
              label={worker.geoip_asn_enabled ? 'ASN 支持' : 'ASN 未支持'}
              variant={worker.geoip_asn_enabled ? 'success' : 'warning'}
            />
            <StatusBadge
              label={
                worker.geoip_operator_enabled ? '运营商支持' : '运营商未支持'
              }
              variant={worker.geoip_operator_enabled ? 'success' : 'warning'}
            />
          </div>
        </div>
        {onOpenSettings ? (
          <SecondaryButton
            type="button"
            aria-label={`设置 DNS 响应端 ${getDNSWorkerDisplayName(worker)}`}
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

export function DNSQueryTrendPanel({
  summary,
}: {
  summary: DNSObservabilitySummary;
}) {
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

export function DNSSnapshotConsistencyPanel({
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

export function CounterChart({
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
