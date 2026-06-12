'use client';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  AuthoritativeDNSMigrationCandidate,
  DNSWorkerItem,
  DNSZoneItem,
} from '@/features/authoritative-dns/types';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import { PrimaryButton } from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';
import { formatDateTime } from '@/lib/utils/date';
import type { MigrationRecheckResult } from './authoritative-dns-page.helpers';
import {
  proxyRouteDetailPath,
  getRouteDomains,
  getRouteDetailHref,
  getGSLBDescription,
  getCandidateRecordType,
  getRecheckStepBadgeVariant,
  getRecheckStepStatusLabel,
  getMigrationRecheckTone,
  formatCount,
  formatSourceScopeLabel,
  getDelegationStatusLabel,
} from './authoritative-dns-page.helpers';
import { InfoTile } from './info-tile';

export function DNSMigrationGuidePanel({
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
                          <StatusBadge
                            label="多节点解析已启用"
                            variant="success"
                          />
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
                2. 部署至少两个 DNS
                响应端，确认响应端在线、能拉取解析配置，并通过公网 UDP/TCP 53
                探测。
              </li>
              <li>
                3. 在网站详情的「负载均衡」里切换为本地自建解析并选择托管域名。
              </li>
              <li>
                4. 到注册商把域名 NS 指向 DNS
                响应端，再回到托管域名详情检查指向。
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

export function MigrationRecheckPanel({
  result,
}: {
  result: MigrationRecheckResult;
}) {
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

export function CheckList({
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
