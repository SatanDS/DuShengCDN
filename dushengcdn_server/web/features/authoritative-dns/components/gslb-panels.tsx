'use client';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  DNSGSLBSimulationPayload,
  DNSGSLBSimulationResult,
  DNSGSLBSchedulingState,
  DNSGSLBSchedulingStates,
} from '@/features/authoritative-dns/types';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';
import type { GSLBSimulationFormValues } from './authoritative-dns-page.helpers';
import {
  gslbSimulationOperatorOptions,
  proxyRouteDetailPath,
  getGSLBDescription,
  getRouteDisplayName,
  getDefaultSimulationQName,
  getRouteRecordType,
  formatCount,
  formatLatencyMs,
  formatSourceScopeLabel,
  formatGSLBOperatorLabel,
  parseSimulationASN,
  isProbeSchedulingGateMessage,
  formatDiagnosticMessage,
  getProbeStatusVariant,
  getNodeDNSProbeStatusLabel,
  getSchedulingStateStatusLabel,
  getSchedulingStateStatusVariant,
} from './authoritative-dns-page.helpers';
import { InfoTile } from './info-tile';

export function GSLBSchedulingStatesPanel({
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

export function GSLBSchedulingStateCard({
  state,
}: {
  state: DNSGSLBSchedulingState;
}) {
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

export function GSLBSimulationPanel({
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

export function GSLBSimulationDiagnostics({
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

export function GSLBSimulationPoolCard({
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
        {asns.length > 0
          ? ` · ASN ${asns.map((asn) => `AS${asn}`).join(', ')}`
          : ''}
        {sourceCIDRs.length > 0 ? ` · 来源网段 ${sourceCIDRs.join(', ')}` : ''}
      </p>
      <p className="mt-1 text-xs text-[var(--foreground-muted)]">
        {pool.reason}
      </p>
    </div>
  );
}
