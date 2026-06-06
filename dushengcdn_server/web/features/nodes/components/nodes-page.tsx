'use client';

import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { ArrowLeft } from 'lucide-react';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { AppModal } from '@/components/ui/app-modal';
import { StatusBadge } from '@/components/ui/status-badge';
import { getConfigVersions } from '@/features/config-versions/api/config-versions';
import {
  createNode,
  deleteNode,
  getNodes,
  updateNode,
} from '@/features/nodes/api/nodes';
import { NodeEditorModal } from '@/features/nodes/components/node-editor-modal';
import type { NodeItem, NodeMutationPayload } from '@/features/nodes/types';
import {
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { formatDateTime, formatRelativeTime } from '@/lib/utils/date';
import {
  getApplyLabel,
  getApplyVariant,
  getNodeStatusLabel,
  getNodeStatusVariant,
  getOpenrestyStatusLabel,
  getOpenrestyStatusVariant,
  isMeaningfulTime,
  isWSConnectedLastSeen,
} from '@/features/nodes/utils';

const nodesQueryKey = ['nodes'];
const supportedRiskFilters = [
  'all',
  'offline',
  'unhealthy',
  'lagging',
] as const;

type NodeRiskFilter = (typeof supportedRiskFilters)[number];

type FeedbackState = {
  tone: 'info' | 'success' | 'danger';
  message: string;
};

type NodePoolSummary = {
  name: string;
  nodes: NodeItem[];
  onlineCount: number;
  schedulableCount: number;
  drainedCount: number;
  unhealthyCount: number;
  laggingCount: number;
  publicIPs: string[];
  latestHeartbeat: string | null;
  latestApplyResult: NodeItem['latest_apply_result'];
  currentVersions: string[];
};

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '请求失败，请稍后重试。';
}

function normalizePoolName(value: string | null | undefined) {
  const normalized = value?.trim().toLowerCase();
  return normalized || 'default';
}

function buildPoolHref(poolName: string, riskFilter: NodeRiskFilter) {
  const params = new URLSearchParams({ pool: poolName });
  if (riskFilter !== 'all') {
    params.set('risk', riskFilter);
  }
  return `/node?${params.toString()}`;
}

function createEmptyNodeForPool(poolName: string): Partial<NodeItem> {
  return {
    pool_name: poolName,
    weight: 100,
    scheduling_enabled: true,
    drain_mode: false,
    auto_update_enabled: false,
    tags: [],
    public_ips: [],
    geo_manual_override: false,
  };
}

function hasLaggingVersion(node: NodeItem, activeVersion: string) {
  return Boolean(activeVersion) && node.current_version !== activeVersion;
}

function shouldShowNode(
  node: NodeItem,
  riskFilter: NodeRiskFilter,
  activeVersion: string,
) {
  switch (riskFilter) {
    case 'offline':
      return node.status === 'offline';
    case 'unhealthy':
      return node.openresty_status === 'unhealthy';
    case 'lagging':
      return hasLaggingVersion(node, activeVersion);
    default:
      return true;
  }
}

function getLatestHeartbeat(nodes: NodeItem[]) {
  if (nodes.some((node) => isWSConnectedLastSeen(node.last_seen_at))) {
    return 'WS 已连接';
  }

  let latest: string | null = null;
  for (const node of nodes) {
    if (!isMeaningfulTime(node.last_seen_at)) {
      continue;
    }
    if (!latest || Date.parse(node.last_seen_at) > Date.parse(latest)) {
      latest = node.last_seen_at;
    }
  }
  return latest;
}

function getLatestApplyResult(
  nodes: NodeItem[],
): NodeItem['latest_apply_result'] {
  if (nodes.some((node) => node.latest_apply_result === 'failed')) {
    return 'failed';
  }
  if (nodes.some((node) => node.latest_apply_result === 'warning')) {
    return 'warning';
  }
  if (nodes.some((node) => node.latest_apply_result === 'success')) {
    return 'success';
  }
  return '';
}

function formatHeartbeat(value: string | null) {
  if (!value) {
    return '暂无';
  }
  if (value === 'WS 已连接') {
    return value;
  }
  return `${formatRelativeTime(value)} · ${formatDateTime(value)}`;
}

function buildPoolSummaries(nodes: NodeItem[], activeVersion: string) {
  const poolMap = new Map<string, NodeItem[]>();
  for (const node of nodes) {
    const poolName = normalizePoolName(node.pool_name);
    const poolNodes = poolMap.get(poolName) ?? [];
    poolNodes.push(node);
    poolMap.set(poolName, poolNodes);
  }

  return Array.from(poolMap.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, poolNodes]) => {
      const publicIPs = Array.from(
        new Set(poolNodes.flatMap((node) => node.public_ips)),
      );
      const currentVersions = Array.from(
        new Set(
          poolNodes
            .map((node) => node.current_version)
            .filter((version) => version.trim() !== ''),
        ),
      );
      const sortedNodes = poolNodes
        .slice()
        .sort((left, right) => left.name.localeCompare(right.name));
      return {
        name,
        nodes: sortedNodes,
        onlineCount: poolNodes.filter((node) => node.status === 'online').length,
        schedulableCount: poolNodes.filter(
          (node) => node.scheduling_enabled && !node.drain_mode,
        ).length,
        drainedCount: poolNodes.filter((node) => node.drain_mode).length,
        unhealthyCount: poolNodes.filter(
          (node) => node.openresty_status === 'unhealthy',
        ).length,
        laggingCount: poolNodes.filter((node) =>
          hasLaggingVersion(node, activeVersion),
        ).length,
        publicIPs,
        latestHeartbeat: getLatestHeartbeat(poolNodes),
        latestApplyResult: getLatestApplyResult(poolNodes),
        currentVersions,
      } satisfies NodePoolSummary;
    });
}

function getPoolHealthBadge(pool: NodePoolSummary) {
  if (pool.nodes.length === 0) {
    return { label: '空池', variant: 'warning' as const };
  }
  if (pool.onlineCount === 0) {
    return { label: '离线', variant: 'danger' as const };
  }
  if (pool.unhealthyCount > 0) {
    return { label: '代理异常', variant: 'danger' as const };
  }
  return { label: '健康', variant: 'success' as const };
}

function NodeStack({ nodes }: { nodes: NodeItem[] }) {
  return (
    <div className="scrollbar-none max-h-64 min-w-[360px] space-y-2 overflow-y-auto pr-1">
      {nodes.map((node) => (
        <div
          key={node.id}
          className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-3 py-3"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 space-y-1">
              <p className="truncate text-sm font-semibold text-[var(--foreground-primary)]">
                {node.name}
              </p>
              <p className="text-xs text-[var(--foreground-secondary)]">
                IP：{node.ip || '未回填'}
              </p>
              <p className="text-xs text-[var(--foreground-secondary)]">
                公网 IP：{node.public_ips.join(' / ') || '未配置'}
              </p>
              <p className="text-xs text-[var(--foreground-secondary)]">
                位置：{node.geo_name || '未配置地图点位'}
              </p>
            </div>
            <StatusBadge
              label={getNodeStatusLabel(node.status)}
              variant={getNodeStatusVariant(node.status)}
            />
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            <StatusBadge label={`池内权重 ${node.weight}`} variant="info" />
            <StatusBadge
              label={node.scheduling_enabled ? '参与调度' : '暂停调度'}
              variant={node.scheduling_enabled ? 'success' : 'warning'}
            />
            {node.drain_mode ? (
              <StatusBadge label="排空" variant="warning" />
            ) : null}
          </div>
        </div>
      ))}
    </div>
  );
}

function NodeVersionStack({ nodes }: { nodes: NodeItem[] }) {
  const versions = nodes
    .map((node) => ({
      id: node.id,
      name: node.name || node.node_id,
      label: `${node.agent_version || 'unknown'} / ${
        node.nginx_version || 'unknown'
      }`,
    }));

  if (versions.length === 0) {
    return <span>unknown</span>;
  }

  return (
    <div className="scrollbar-none max-h-28 min-w-0 max-w-full space-y-2 overflow-y-auto pr-1">
      {versions.map((item) => (
        <div
          key={`${item.id}-${item.label}`}
          className="min-w-0 rounded-lg border border-[var(--border-default)] bg-[var(--surface-elevated)] px-2.5 py-2"
        >
          <p className="truncate text-xs font-medium text-[var(--foreground-primary)]">
            {item.name}
          </p>
          <p className="mt-1 min-w-0 break-all text-xs text-[var(--foreground-secondary)]">
            {item.label}
          </p>
        </div>
      ))}
    </div>
  );
}

function PoolCreateModal({
  isOpen,
  isSubmitting,
  onClose,
  onSubmit,
}: {
  isOpen: boolean;
  isSubmitting: boolean;
  onClose: () => void;
  onSubmit: (poolName: string) => void;
}) {
  const [poolName, setPoolName] = useState('');

  useEffect(() => {
    if (!isOpen) {
      setPoolName('');
    }
  }, [isOpen]);

  return (
    <AppModal
      isOpen={isOpen}
      title="新增边缘池"
      description="先创建一个池入口，进入详情后再添加这个池里的节点和公网 IP。"
      onClose={onClose}
      size="sm"
      footer={
        <div className="flex flex-wrap justify-end gap-3">
          <SecondaryButton type="button" onClick={onClose} disabled={isSubmitting}>
            取消
          </SecondaryButton>
          <PrimaryButton
            type="submit"
            form="pool-create-form"
            disabled={isSubmitting || poolName.trim() === ''}
          >
            {isSubmitting ? '创建中...' : '创建池'}
          </PrimaryButton>
        </div>
      }
    >
      <form
        id="pool-create-form"
        className="space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          const normalizedPoolName = poolName.trim();
          if (!normalizedPoolName) {
            return;
          }
          onSubmit(normalizedPoolName);
        }}
      >
        <ResourceField
          label="边缘池名称"
          hint="示例：hongkong、europe、jp。保存后会先生成一个待接入节点，你可以在池详情继续编辑节点和 IP。"
        >
          <ResourceInput
            value={poolName}
            placeholder="hongkong"
            onChange={(event) => setPoolName(event.target.value)}
          />
        </ResourceField>
      </form>
    </AppModal>
  );
}

export function NodesPage() {
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<FeedbackState>();
  const confirmDialog = useConfirmDialog();
  const [editingNode, setEditingNode] = useState<Partial<NodeItem> | null>(null);
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [isPoolCreateOpen, setIsPoolCreateOpen] = useState(false);

  const selectedPoolName = normalizePoolName(searchParams.get('pool'));
  const isPoolDetail = Boolean(searchParams.get('pool'));

  const nodesQuery = useQuery({
    queryKey: nodesQueryKey,
    queryFn: getNodes,
    refetchInterval: 5000,
  });

  const configVersionsQuery = useQuery({
    queryKey: ['config-versions'],
    queryFn: getConfigVersions,
    refetchInterval: 5000,
  });

  const saveMutation = useMutation({
    mutationFn: async (payload: NodeMutationPayload) => {
      return editingNode && 'id' in editingNode && editingNode.id
        ? updateNode(editingNode.id, payload)
        : createNode(payload);
    },
    onSuccess: async () => {
      setFeedback({
        tone: 'success',
        message:
          editingNode && 'id' in editingNode && editingNode.id
            ? '节点已更新。'
            : '节点已创建。',
      });
      setEditingNode(null);
      setIsEditorOpen(false);
      await queryClient.invalidateQueries({ queryKey: nodesQueryKey });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const createPoolMutation = useMutation({
    mutationFn: async (poolName: string) => {
      const normalizedPoolName = normalizePoolName(poolName);
      return createNode({
        name: `${normalizedPoolName}-edge-1`,
        ip: '',
        pool_name: normalizedPoolName,
        tags: [],
        weight: 100,
        public_ips: [],
        scheduling_enabled: true,
        drain_mode: false,
        auto_update_enabled: false,
        geo_name: '',
        geo_latitude: null,
        geo_longitude: null,
        geo_manual_override: false,
      });
    },
    onSuccess: async (node) => {
      setFeedback({
        tone: 'success',
        message: `边缘池 ${node.pool_name} 已创建，请进入详情补充节点 IP。`,
      });
      setIsPoolCreateOpen(false);
      await queryClient.invalidateQueries({ queryKey: nodesQueryKey });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteNode,
    onSuccess: async (result) => {
      setFeedback({
        tone: result.uninstall_agent_requested ? 'success' : 'info',
        message: result.uninstall_agent_message || '节点已删除。',
      });
      setEditingNode(null);
      await queryClient.invalidateQueries({ queryKey: nodesQueryKey });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const activeVersion = useMemo(
    () =>
      (configVersionsQuery.data ?? []).find((item) => item.is_active)
        ?.version ?? '',
    [configVersionsQuery.data],
  );
  const riskFilter = useMemo<NodeRiskFilter>(() => {
    const current = searchParams.get('risk')?.trim().toLowerCase() ?? 'all';
    return supportedRiskFilters.includes(current as NodeRiskFilter)
      ? (current as NodeRiskFilter)
      : 'all';
  }, [searchParams]);
  const filteredNodes = useMemo(
    () =>
      nodes.filter((node) => shouldShowNode(node, riskFilter, activeVersion)),
    [activeVersion, nodes, riskFilter],
  );
  const poolSummaries = useMemo(
    () => buildPoolSummaries(filteredNodes, activeVersion),
    [activeVersion, filteredNodes],
  );
  const selectedPoolNodes = useMemo(
    () =>
      filteredNodes.filter(
        (node) => normalizePoolName(node.pool_name) === selectedPoolName,
      ),
    [filteredNodes, selectedPoolName],
  );
  const selectedPoolSummary = useMemo(
    () => buildPoolSummaries(selectedPoolNodes, activeVersion)[0] ?? null,
    [activeVersion, selectedPoolNodes],
  );

  const filterDescription = useMemo(() => {
    switch (riskFilter) {
      case 'offline':
        return '当前仅展示包含离线节点的边缘池。';
      case 'unhealthy':
        return '当前仅展示包含代理服务异常节点的边缘池。';
      case 'lagging':
        return activeVersion
          ? `当前仅展示未追平激活版本 ${activeVersion} 的边缘池。`
          : '当前没有激活版本，无法筛选配置落后节点。';
      default:
        return '列表每 5 秒自动刷新一次。';
    }
  }, [activeVersion, riskFilter]);

  const handleReset = () => {
    setFeedback(null);
    setEditingNode(null);
    setIsEditorOpen(false);
  };

  const handleCreateNode = (poolName: string) => {
    setFeedback(null);
    setEditingNode(createEmptyNodeForPool(poolName));
    setIsEditorOpen(true);
  };

  const handleEditNode = (node: NodeItem) => {
    setFeedback(null);
    setEditingNode(node);
    setIsEditorOpen(true);
  };

  const handleDeleteNode = async (nodeId: number, nodeName: string) => {
    const confirmed = await confirmDialog({
      title: '删除节点',
      message: `确认删除节点“${nodeName}”吗？如果节点在线，系统会同时下发 Agent 卸载指令；离线节点只会删除面板记录。`,
      confirmLabel: '删除节点',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    setFeedback(null);
    deleteMutation.mutate(nodeId);
  };

  const handleDeletePool = async (pool: NodePoolSummary) => {
    const confirmed = await confirmDialog({
      title: '删除边缘池',
      message: `确认删除边缘池“${pool.name}”里的 ${pool.nodes.length} 个节点吗？在线节点会尝试下发 Agent 卸载指令。`,
      confirmLabel: '删除池',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    setFeedback(null);
    for (const node of pool.nodes) {
      deleteMutation.mutate(node.id);
    }
  };

  const renderRiskFilters = () => (
    <div className="flex flex-wrap gap-2">
      {[
        {
          href: isPoolDetail ? buildPoolHref(selectedPoolName, 'all') : '/node',
          key: 'all',
          label: '全部池',
        },
        {
          href: isPoolDetail
            ? buildPoolHref(selectedPoolName, 'offline')
            : '/node?risk=offline',
          key: 'offline',
          label: '离线节点',
        },
        {
          href: isPoolDetail
            ? buildPoolHref(selectedPoolName, 'unhealthy')
            : '/node?risk=unhealthy',
          key: 'unhealthy',
          label: '代理服务异常',
        },
        {
          href: isPoolDetail
            ? buildPoolHref(selectedPoolName, 'lagging')
            : '/node?risk=lagging',
          key: 'lagging',
          label: '配置落后',
        },
      ].map((item) => (
        <Link
          key={item.key}
          href={item.href}
          className={`inline-flex items-center rounded-full border px-3 py-1.5 text-xs transition ${
            riskFilter === item.key
              ? 'border-[var(--border-strong)] bg-[var(--accent-soft)] text-[var(--foreground-primary)]'
              : 'border-[var(--border-default)] text-[var(--foreground-secondary)] hover:bg-[var(--control-background-hover)]'
          }`}
        >
          {item.label}
        </Link>
      ))}
    </div>
  );

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="边缘节点 / IP 池"
          description={
            isPoolDetail
              ? `管理边缘池 ${selectedPoolName} 下的节点、公网 IP、池内权重和排空状态。`
              : '统一维护边缘池、池内节点、公网 IP、池内权重和排空状态。'
          }
          action={
            <>
              <SecondaryButton
                type="button"
                onClick={() => setIsPoolCreateOpen(true)}
              >
                新增边缘池
              </SecondaryButton>
              {isPoolDetail ? (
                <SecondaryButton
                  type="button"
                  onClick={() => handleCreateNode(selectedPoolName)}
                >
                  添加节点
                </SecondaryButton>
              ) : null}
              <Link
                href="/apply-log"
                className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
              >
                应用记录
              </Link>
            </>
          }
        />

        {isPoolDetail ? (
          <div className="flex items-center">
            <Link
              href={riskFilter === 'all' ? '/node' : `/node?risk=${riskFilter}`}
              className="inline-flex items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--control-background)] px-3 py-1.5 text-xs font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
            >
              <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
              返回边缘池列表
            </Link>
          </div>
        ) : riskFilter !== 'all' ? (
          <div className="flex items-center">
            <Link
              href="/node"
              className="inline-flex items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--control-background)] px-3 py-1.5 text-xs font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
            >
              <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />
              返回全部池
            </Link>
          </div>
        ) : null}

        {isPoolDetail ? (
          <AppCard
            title={`边缘池：${selectedPoolName}`}
            description="在这里添加或编辑这个池里的节点和公网 IP。"
            action={
              <SecondaryButton
                type="button"
                onClick={() =>
                  void queryClient.invalidateQueries({ queryKey: nodesQueryKey })
                }
              >
                立即刷新
              </SecondaryButton>
            }
          >
            {nodesQuery.isLoading ? (
              <LoadingState />
            ) : nodesQuery.isError ? (
              <ErrorState
                title="边缘池加载失败"
                description={getErrorMessage(nodesQuery.error)}
              />
            ) : selectedPoolNodes.length === 0 ? (
              <EmptyState
                title="这个池里还没有节点"
                description="点击右上角“添加节点”录入节点 IP 和公网 IP 池。"
              />
            ) : (
              <div className="space-y-5">
                {renderRiskFilters()}
                {selectedPoolSummary ? (
                  <div className="grid gap-3 md:grid-cols-4">
                    <MetricTile label="节点数" value={selectedPoolSummary.nodes.length} />
                    <MetricTile label="可调度" value={selectedPoolSummary.schedulableCount} />
                    <MetricTile label="公网 IP" value={selectedPoolSummary.publicIPs.length} />
                    <MetricTile
                      label="最近心跳"
                      value={formatHeartbeat(selectedPoolSummary.latestHeartbeat)}
                    />
                  </div>
                ) : null}

                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-[var(--border-default)] text-left text-sm">
                    <thead>
                      <tr className="text-[var(--foreground-secondary)]">
                        <th className="px-3 py-3 font-medium">节点</th>
                        <th className="px-3 py-3 font-medium">状态</th>
                        <th className="px-3 py-3 font-medium">调度</th>
                        <th className="px-3 py-3 font-medium">
                          Agent / 代理服务版本
                        </th>
                        <th className="px-3 py-3 font-medium">运行健康</th>
                        <th className="px-3 py-3 font-medium">当前版本</th>
                        <th className="px-3 py-3 font-medium">最近应用</th>
                        <th className="px-3 py-3 font-medium">最近心跳</th>
                        <th className="px-3 py-3 font-medium">操作</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-[var(--border-default)]">
                      {selectedPoolNodes.map((node) => (
                        <tr key={node.id} className="align-top">
                          <td className="px-3 py-4">
                            <div className="space-y-1">
                              <p className="font-medium text-[var(--foreground-primary)]">
                                {node.name}
                              </p>
                              <p className="text-xs text-[var(--foreground-secondary)]">
                                IP：{node.ip || '未回填'}
                              </p>
                              <p className="text-xs text-[var(--foreground-secondary)]">
                                公网 IP：{node.public_ips.join(' / ') || '未配置'}
                              </p>
                              <p className="text-xs text-[var(--foreground-secondary)]">
                                位置：{node.geo_name || '未配置地图点位'}
                              </p>
                              <p className="text-xs text-[var(--foreground-secondary)]">
                                池内权重：{node.weight}
                              </p>
                            </div>
                          </td>
                          <td className="px-3 py-4">
                            <StatusBadge
                              label={getNodeStatusLabel(node.status)}
                              variant={getNodeStatusVariant(node.status)}
                            />
                          </td>
                          <td className="px-3 py-4">
                            <div className="flex flex-wrap gap-2">
                              <StatusBadge
                                label={
                                  node.scheduling_enabled
                                    ? '参与调度'
                                    : '暂停调度'
                                }
                                variant={
                                  node.scheduling_enabled ? 'success' : 'warning'
                                }
                              />
                              {node.drain_mode ? (
                                <StatusBadge label="排空" variant="warning" />
                              ) : null}
                            </div>
                          </td>
                          <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                            {node.agent_version || 'unknown'} /{' '}
                            {node.nginx_version || 'unknown'}
                          </td>
                          <td className="px-3 py-4">
                            <StatusBadge
                              label={getOpenrestyStatusLabel(
                                node.openresty_status,
                              )}
                              variant={getOpenrestyStatusVariant(
                                node.openresty_status,
                              )}
                            />
                          </td>
                          <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                            {node.current_version || '未应用'}
                          </td>
                          <td className="px-3 py-4">
                            <StatusBadge
                              label={getApplyLabel(node.latest_apply_result)}
                              variant={getApplyVariant(node.latest_apply_result)}
                            />
                          </td>
                          <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                            {isWSConnectedLastSeen(node.last_seen_at)
                              ? 'WS 已连接'
                              : isMeaningfulTime(node.last_seen_at)
                              ? `${formatRelativeTime(
                                  node.last_seen_at,
                                )} · ${formatDateTime(node.last_seen_at)}`
                              : '暂无'}
                          </td>
                          <td className="px-3 py-4">
                            <div className="flex flex-wrap gap-2">
                              <Link
                                href={`/node/detail?id=${node.id}`}
                                className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-3 py-2 text-xs font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
                              >
                                详情
                              </Link>
                              <SecondaryButton
                                type="button"
                                onClick={() => handleEditNode(node)}
                                className="px-3 py-2 text-xs"
                              >
                                编辑
                              </SecondaryButton>
                              <DangerButton
                                type="button"
                                onClick={() =>
                                  void handleDeleteNode(node.id, node.name)
                                }
                                disabled={deleteMutation.isPending}
                                className="px-3 py-2 text-xs"
                              >
                                删除
                              </DangerButton>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </AppCard>
        ) : (
          <AppCard
            title="边缘池列表"
            description={filterDescription}
            action={
              <SecondaryButton
                type="button"
                onClick={() =>
                  void queryClient.invalidateQueries({ queryKey: nodesQueryKey })
                }
              >
                立即刷新
              </SecondaryButton>
            }
          >
            {nodesQuery.isLoading ? (
              <LoadingState />
            ) : nodesQuery.isError ? (
              <ErrorState
                title="边缘池加载失败"
                description={getErrorMessage(nodesQuery.error)}
              />
            ) : poolSummaries.length === 0 ? (
              <EmptyState
                title={riskFilter === 'all' ? '暂无边缘池' : '当前筛选无结果'}
                description={
                  riskFilter === 'all'
                    ? '点击右上角“新增边缘池”，再进入详情页添加节点和公网 IP。'
                    : '可以返回总览继续排查，或切换到全部池查看完整列表。'
                }
              />
            ) : (
              <div className="space-y-4">
                {renderRiskFilters()}

                <div className="overflow-x-auto">
                  <table className="min-w-full divide-y divide-[var(--border-default)] text-left text-sm">
                    <thead>
                      <tr className="text-[var(--foreground-secondary)]">
                        <th className="px-3 py-3 font-medium">池</th>
                        <th className="px-3 py-3 font-medium">节点</th>
                        <th className="px-3 py-3 font-medium">状态</th>
                        <th className="px-3 py-3 font-medium">调度</th>
                        <th className="px-3 py-3 font-medium">
                          Agent / 代理服务版本
                        </th>
                        <th className="px-3 py-3 font-medium">运行健康</th>
                        <th className="px-3 py-3 font-medium">当前版本</th>
                        <th className="px-3 py-3 font-medium">最近应用</th>
                        <th className="px-3 py-3 font-medium">最近心跳</th>
                        <th className="px-3 py-3 font-medium">操作</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-[var(--border-default)]">
                      {poolSummaries.map((pool) => {
                        const health = getPoolHealthBadge(pool);
                        return (
                          <tr key={pool.name} className="align-top">
                            <td className="px-3 py-4">
                              <div className="space-y-2">
                                <p className="font-semibold text-[var(--foreground-primary)]">
                                  {pool.name}
                                </p>
                                <p className="text-xs text-[var(--foreground-secondary)]">
                                  {pool.nodes.length} 个节点 · {pool.publicIPs.length}{' '}
                                  个公网 IP
                                </p>
                                <p className="text-xs text-[var(--foreground-secondary)]">
                                  在线 {pool.onlineCount} / 可调度{' '}
                                  {pool.schedulableCount}
                                </p>
                              </div>
                            </td>
                            <td className="px-3 py-4">
                              <NodeStack nodes={pool.nodes} />
                            </td>
                            <td className="px-3 py-4">
                              <StatusBadge
                                label={health.label}
                                variant={health.variant}
                              />
                            </td>
                            <td className="px-3 py-4">
                              <div className="flex flex-wrap gap-2">
                                <StatusBadge
                                  label={`${pool.schedulableCount} 个参与调度`}
                                  variant={
                                    pool.schedulableCount > 0
                                      ? 'success'
                                      : 'warning'
                                  }
                                />
                                {pool.drainedCount > 0 ? (
                                  <StatusBadge
                                    label={`${pool.drainedCount} 个排空`}
                                    variant="warning"
                                  />
                                ) : null}
                              </div>
                            </td>
                            <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                              <NodeVersionStack nodes={pool.nodes} />
                            </td>
                            <td className="px-3 py-4">
                              <StatusBadge
                                label={health.label}
                                variant={health.variant}
                              />
                            </td>
                            <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                              {pool.currentVersions.slice(0, 3).join('、') ||
                                '未应用'}
                              {pool.laggingCount > 0 ? (
                                <span className="mt-2 block text-xs text-[var(--status-warning-foreground)]">
                                  {pool.laggingCount} 个节点配置落后
                                </span>
                              ) : null}
                            </td>
                            <td className="px-3 py-4">
                              <StatusBadge
                                label={getApplyLabel(pool.latestApplyResult)}
                                variant={getApplyVariant(pool.latestApplyResult)}
                              />
                            </td>
                            <td className="px-3 py-4 text-[var(--foreground-secondary)]">
                              {formatHeartbeat(pool.latestHeartbeat)}
                            </td>
                            <td className="px-3 py-4">
                              <div className="flex flex-wrap gap-2">
                                <Link
                                  href={buildPoolHref(pool.name, riskFilter)}
                                  className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-3 py-2 text-xs font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
                                >
                                  详情
                                </Link>
                                <DangerButton
                                  type="button"
                                  onClick={() => void handleDeletePool(pool)}
                                  disabled={deleteMutation.isPending}
                                  className="px-3 py-2 text-xs"
                                >
                                  删除
                                </DangerButton>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </AppCard>
        )}
      </div>

      <PoolCreateModal
        isOpen={isPoolCreateOpen}
        isSubmitting={createPoolMutation.isPending}
        onClose={() => setIsPoolCreateOpen(false)}
        onSubmit={(poolName) => {
          setFeedback(null);
          createPoolMutation.mutate(poolName);
        }}
      />

      <NodeEditorModal
        isOpen={isEditorOpen}
        node={editingNode}
        isSubmitting={saveMutation.isPending}
        title={
          editingNode && 'id' in editingNode && editingNode.id
            ? '编辑节点'
            : '添加节点'
        }
        onClose={handleReset}
        description={
          editingNode && 'id' in editingNode && editingNode.id
            ? '更新节点 IP、公网 IP、池内权重、调度和排空状态。'
            : `向边缘池 ${editingNode?.pool_name ?? selectedPoolName} 添加节点和公网 IP。`
        }
        submitLabel={
          editingNode && 'id' in editingNode && editingNode.id
            ? '保存修改'
            : '添加节点'
        }
        onSubmit={(payload) => {
          setFeedback(null);
          saveMutation.mutate({
            ...payload,
            pool_name: payload.pool_name || selectedPoolName,
          });
        }}
      />
    </>
  );
}

function MetricTile({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3">
      <p className="text-xs text-[var(--foreground-secondary)]">{label}</p>
      <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
        {value}
      </p>
    </div>
  );
}
