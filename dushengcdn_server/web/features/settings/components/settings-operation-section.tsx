import { AppCard } from '@/components/ui/app-card';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { StatusBadge } from '@/components/ui/status-badge';
import type { GeoIPLookupResult } from '@/features/settings/types';
import {
  CodeBlock,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';
import { getErrorMessage } from '@/lib/utils/errors';
import { formatDateTime } from '@/lib/utils/date';
import type { PublicStatus } from '@/types/public-status';

export type OperationSettingsFields = {
  AgentHeartbeatInterval: string;
  AgentWebsocketUpgradeEnabled: boolean;
  AgentLegacyGlobalTokenEnabled: boolean;
  NodeOfflineThreshold: string;
  AgentUpdateRepo: string;
  GeoIPProvider: string;
  AuthoritativeDNSDefaultTTL: string;
  AuthoritativeDNSSnapshotMaxAge: string;
  AuthoritativeDNSWorkerQueryRateLimit: string;
  AuthoritativeDNSWorkerResponseRateLimit: string;
  AuthoritativeDNSWorkerUDPResponseSize: string;
  AuthoritativeDNSWorkerECSEnabled: boolean;
  AuthoritativeDNSWorkerECSIPv4Prefix: string;
  AuthoritativeDNSWorkerECSIPv6Prefix: string;
  GSLBMetricFreshnessSeconds: string;
  GSLBProbeSchedulingEnabled: boolean;
  ServerAddress: string;
};

export type OperationSettingsFieldKey = keyof OperationSettingsFields;
export type DeploymentProtocol = 'http' | 'https';

type SettingsOperationSectionProps = {
  busyKey: string | null;
  operationFields: OperationSettingsFields;
  deploymentProtocol: DeploymentProtocol;
  discoveryCommand: string;
  discoveryToken: string;
  geoIPTestIP: string;
  geoIPLookupResult?: GeoIPLookupResult;
  geoIPLookupIsPending: boolean;
  geoIPLookupIsError: boolean;
  geoIPLookupError: unknown;
  bootstrapTokenIsLoading: boolean;
  bootstrapTokenIsError: boolean;
  bootstrapTokenError: unknown;
  bootstrapTokenRotateIsPending: boolean;
  publicStatus: PublicStatus;
  formatDurationLabel: (value: string) => string;
  onOperationFieldChange: (
    key: OperationSettingsFieldKey,
    value: string | boolean,
  ) => void;
  onDeploymentProtocolChange: (protocol: DeploymentProtocol) => void;
  onServerAddressChange: (value: string) => void;
  onGeoIPTestIPChange: (value: string) => void;
  onGeoIPLookup: () => void;
  onSaveOperationSettings: () => void;
  onSaveAuthoritativeDnsSettings: () => void;
  onRotateBootstrapToken: () => void;
  onCopyDiscoveryCommand: () => void;
};

export function SettingsOperationSection({
  busyKey,
  operationFields,
  deploymentProtocol,
  discoveryCommand,
  discoveryToken,
  geoIPTestIP,
  geoIPLookupResult,
  geoIPLookupIsPending,
  geoIPLookupIsError,
  geoIPLookupError,
  bootstrapTokenIsLoading,
  bootstrapTokenIsError,
  bootstrapTokenError,
  bootstrapTokenRotateIsPending,
  publicStatus,
  formatDurationLabel,
  onOperationFieldChange,
  onDeploymentProtocolChange,
  onServerAddressChange,
  onGeoIPTestIPChange,
  onGeoIPLookup,
  onSaveOperationSettings,
  onSaveAuthoritativeDnsSettings,
  onRotateBootstrapToken,
  onCopyDiscoveryCommand,
}: SettingsOperationSectionProps) {
  return (
    <div className="space-y-6">
      <div className="grid gap-6 xl:grid-cols-[1fr_1fr]">
        <AppCard
          title="Agent 接入设置"
          description="运行参数会通过心跳响应下发到 Agent。"
          action={
            <PrimaryButton type="button" onClick={onSaveOperationSettings}>
              {busyKey === 'operation-settings' ? '保存中...' : '保存设置'}
            </PrimaryButton>
          }
        >
          <div className="space-y-6">
            <div className="space-y-4">
              <div className="space-y-1">
                <p className="text-sm font-medium text-[var(--foreground-primary)]">
                  Agent 运行参数
                </p>
                <p className="text-sm text-[var(--foreground-muted)]">
                  修改后会在下个心跳周期同步到节点。
                </p>
              </div>
              <div className="grid gap-5 md:grid-cols-2">
                <ResourceField
                  label={`心跳间隔 (${formatDurationLabel(operationFields.AgentHeartbeatInterval)})`}
                >
                  <ResourceInput
                    type="number"
                    value={operationFields.AgentHeartbeatInterval}
                    onChange={(event) =>
                      onOperationFieldChange(
                        'AgentHeartbeatInterval',
                        event.target.value,
                      )
                    }
                  />
                </ResourceField>
                <ResourceField
                  label={`离线阈值 (${formatDurationLabel(operationFields.NodeOfflineThreshold)})`}
                >
                  <ResourceInput
                    type="number"
                    value={operationFields.NodeOfflineThreshold}
                    onChange={(event) =>
                      onOperationFieldChange(
                        'NodeOfflineThreshold',
                        event.target.value,
                      )
                    }
                  />
                </ResourceField>
              </div>
              <ToggleField
                label="开启 WS 连接升级"
                description="开启后 Agent 会在 HTTP 心跳成功后尝试升级为 WebSocket，发布配置时可立即收到同步通知。"
                checked={operationFields.AgentWebsocketUpgradeEnabled}
                onChange={(checked) =>
                  onOperationFieldChange(
                    'AgentWebsocketUpgradeEnabled',
                    checked,
                  )
                }
              />
              <ToggleField
                label="允许旧全局 Agent Token"
                description="仅用于迁移旧 Agent；开启后仍要求请求携带已存在的 node_id，迁移完成后应关闭。"
                checked={operationFields.AgentLegacyGlobalTokenEnabled}
                onChange={(checked) =>
                  onOperationFieldChange(
                    'AgentLegacyGlobalTokenEnabled',
                    checked,
                  )
                }
              />
            </div>

            <div className="border-t border-[var(--border-default)] pt-6">
              <div className="space-y-1">
                <p className="text-sm font-medium text-[var(--foreground-primary)]">
                  Agent 更新仓库
                </p>
                <p className="text-sm text-[var(--foreground-muted)]">
                  上游更新仓库
                </p>
              </div>
              <div className="mt-4">
                <ResourceField label="GitHub 仓库">
                  <ResourceInput
                    value={operationFields.AgentUpdateRepo}
                    onChange={(event) =>
                      onOperationFieldChange(
                        'AgentUpdateRepo',
                        event.target.value,
                      )
                    }
                    placeholder="SatanDS/SatanDS-DuShengCDN-releases"
                  />
                </ResourceField>
              </div>
            </div>

            <div className="border-t border-[var(--border-default)] pt-6">
              <div className="space-y-1">
                <p className="text-sm font-medium text-[var(--foreground-primary)]">
                  IP 归属方式
                </p>
                <p className="text-sm text-[var(--foreground-muted)]">
                  控制节点地图等场景使用的 IP
                  归属解析来源。访客访问记录归属地入库固定使用 MaxMind mmdb。
                </p>
              </div>
              <div className="mt-4">
                <ResourceField
                  label="归属方式"
                  hint="disabled 关闭节点归属解析；mmdb 使用本地数据库；其余选项调用外部 GeoIP 服务。"
                >
                  <ResourceSelect
                    value={operationFields.GeoIPProvider}
                    onChange={(event) =>
                      onOperationFieldChange(
                        'GeoIPProvider',
                        event.target.value,
                      )
                    }
                  >
                    <option value="disabled">关闭</option>
                    <option value="mmdb">MaxMind mmdb</option>
                    <option value="ip-api">ip-api.com</option>
                    <option value="geojs">geojs.io</option>
                    <option value="ipinfo">ipinfo.io</option>
                  </ResourceSelect>
                </ResourceField>
              </div>
              <div className="mt-5 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
                  <ResourceField label="测试 IP">
                    <ResourceInput
                      value={geoIPTestIP}
                      onChange={(event) =>
                        onGeoIPTestIPChange(event.target.value)
                      }
                      placeholder="例如 8.8.8.8"
                    />
                  </ResourceField>
                  <PrimaryButton
                    type="button"
                    onClick={onGeoIPLookup}
                    disabled={geoIPLookupIsPending}
                  >
                    {geoIPLookupIsPending ? '查询中...' : '查询归属'}
                  </PrimaryButton>
                </div>

                <div className="mt-4 space-y-3">
                  {geoIPLookupIsError ? (
                    <InlineMessage
                      tone="danger"
                      message={getErrorMessage(geoIPLookupError)}
                    />
                  ) : null}

                  {geoIPLookupResult ? (
                    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                        <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                          查询 IP
                        </p>
                        <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                          {geoIPLookupResult.ip}
                        </p>
                      </div>
                      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                        <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                          国家 / 地区
                        </p>
                        <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                          {geoIPLookupResult.name || '—'}
                        </p>
                      </div>
                      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                        <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                          ISO Code
                        </p>
                        <div className="mt-2">
                          <StatusBadge
                            label={geoIPLookupResult.iso_code || '—'}
                            variant="info"
                          />
                        </div>
                      </div>
                      <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                        <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                          经纬度
                        </p>
                        <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                          {geoIPLookupResult.latitude !== undefined &&
                          geoIPLookupResult.latitude !== null &&
                          geoIPLookupResult.longitude !== undefined &&
                          geoIPLookupResult.longitude !== null
                            ? `${geoIPLookupResult.latitude.toFixed(4)}, ${geoIPLookupResult.longitude.toFixed(4)}`
                            : '—'}
                        </p>
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        </AppCard>

        <AppCard
          title="本地解析运行参数"
          description="控制解析缓存、响应端配置过期保护，以及节点状态数据多久还可以用于自动选 IP。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveAuthoritativeDnsSettings}
              disabled={busyKey === 'operation-authoritative-dns'}
            >
              {busyKey === 'operation-authoritative-dns'
                ? '保存中...'
                : '保存参数'}
            </PrimaryButton>
          }
        >
          <div className="grid gap-5 md:grid-cols-3">
            <ResourceField
              label="默认缓存时间（秒）"
              tooltip="DNS 结果会被递归解析器缓存一段时间。时间越短，切换 IP 越快，但查询量会更高。"
            >
              <ResourceInput
                type="number"
                value={operationFields.AuthoritativeDNSDefaultTTL}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSDefaultTTL',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
            <ResourceField
              label="解析配置有效期（秒）"
              tooltip="DNS 响应端会缓存面板下发的解析配置。超过这个时间还没有拿到新配置时，会提示配置过期。"
            >
              <ResourceInput
                type="number"
                value={operationFields.AuthoritativeDNSSnapshotMaxAge}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSSnapshotMaxAge',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
            <ResourceField
              label="节点状态数据有效时间（秒）"
              hint="不是健康检查间隔。节点上报的连接数、处理器和内存数据超过这个时间后，就不再用于按压力选 IP。"
              tooltip="判断依据是节点最近一次上报的压力数据时间。超过该时间的数据会被视为过旧，避免系统拿旧压力数据继续做多节点智能解析。"
            >
              <ResourceInput
                type="number"
                value={operationFields.GSLBMetricFreshnessSeconds}
                onChange={(event) =>
                  onOperationFieldChange(
                    'GSLBMetricFreshnessSeconds',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
          </div>
          <div className="mt-5 grid gap-5 md:grid-cols-3">
            <ResourceField
              label="普通查询限流（次/秒）"
              hint="按来源 IP 统计，0 表示关闭。"
              tooltip="用于限制单个来源 IP 每秒可发起的 DNS 查询数。"
            >
              <ResourceInput
                type="number"
                min={0}
                value={operationFields.AuthoritativeDNSWorkerQueryRateLimit}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSWorkerQueryRateLimit',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
            <ResourceField
              label="异常响应 RRL（次/秒）"
              hint="按来源、QNAME、RCODE 统计，0 表示关闭。"
              tooltip="用于抑制重复 NXDOMAIN、SERVFAIL、REFUSED 等异常响应被放大滥用。"
            >
              <ResourceInput
                type="number"
                min={0}
                value={operationFields.AuthoritativeDNSWorkerResponseRateLimit}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSWorkerResponseRateLimit',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
            <ResourceField
              label="UDP 响应大小（字节）"
              hint="默认 1232，降低可减少分片风险。"
              tooltip="DNS Worker 会按这里的上限截断 UDP 响应，客户端可改用 TCP 重试。"
            >
              <ResourceInput
                type="number"
                min={512}
                max={65535}
                value={operationFields.AuthoritativeDNSWorkerUDPResponseSize}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSWorkerUDPResponseSize',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
          </div>
          <div className="mt-5 grid gap-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)]">
            <ToggleField
              label="启用 ECS 分流"
              description="允许 DNS Worker 使用 EDNS Client Subnet 作为来源分流依据。"
              tooltip="关闭后只按递归解析器来源 IP 分流；开启后会按下方前缀规范化 ECS 地址。"
              checked={operationFields.AuthoritativeDNSWorkerECSEnabled}
              onChange={(checked) =>
                onOperationFieldChange(
                  'AuthoritativeDNSWorkerECSEnabled',
                  checked,
                )
              }
            />
            <ResourceField
              label="ECS IPv4 前缀"
              hint="默认 /24。"
              tooltip="DNS Worker 会把收到的 IPv4 ECS 地址规范到不超过该前缀后再参与分流和统计。"
            >
              <ResourceInput
                type="number"
                min={0}
                max={32}
                value={operationFields.AuthoritativeDNSWorkerECSIPv4Prefix}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSWorkerECSIPv4Prefix',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
            <ResourceField
              label="ECS IPv6 前缀"
              hint="默认 /56。"
              tooltip="DNS Worker 会把收到的 IPv6 ECS 地址规范到不超过该前缀后再参与分流和统计。"
            >
              <ResourceInput
                type="number"
                min={0}
                max={128}
                value={operationFields.AuthoritativeDNSWorkerECSIPv6Prefix}
                onChange={(event) =>
                  onOperationFieldChange(
                    'AuthoritativeDNSWorkerECSIPv6Prefix',
                    event.target.value,
                  )
                }
              />
            </ResourceField>
          </div>
          <div className="mt-5">
            <ToggleField
              label="按响应端探测结果筛选节点"
              description="开启后，本地自建解析只会使用最近探测成功的边缘节点。"
              tooltip="边缘节点会主动探测 DNS 响应端。如果开启这个筛选，探测失败或探测结果过期的节点不会被选为解析返回 IP。"
              checked={operationFields.GSLBProbeSchedulingEnabled}
              onChange={(checked) =>
                onOperationFieldChange('GSLBProbeSchedulingEnabled', checked)
              }
            />
          </div>
        </AppCard>

        <AppCard
          title="接入令牌与部署命令"
          description="适用于新节点首次接入。安装脚本会自动检测 Linux / macOS 环境，并尝试补齐缺少的代理服务或源码构建依赖。"
          action={
            <div className="flex flex-wrap gap-2">
              <SecondaryButton
                type="button"
                onClick={onRotateBootstrapToken}
                disabled={bootstrapTokenRotateIsPending}
              >
                {bootstrapTokenRotateIsPending
                  ? '生成中...'
                  : '重新生成接入令牌'}
              </SecondaryButton>
              {discoveryCommand ? (
                <PrimaryButton type="button" onClick={onCopyDiscoveryCommand}>
                  复制命令
                </PrimaryButton>
              ) : null}
            </div>
          }
        >
          {bootstrapTokenIsLoading ? (
            <LoadingState />
          ) : bootstrapTokenIsError ? (
            <ErrorState
              title="接入令牌加载失败"
              description={getErrorMessage(bootstrapTokenError)}
            />
          ) : (
            <div className="space-y-4">
              <ResourceField
                label="面板访问地址"
                hint="默认使用当前面板地址，可按需选择 HTTP 或 HTTPS 并改为外部访问地址。"
                container="div"
              >
                <div className="grid gap-2 sm:grid-cols-[auto_minmax(0,1fr)]">
                  <div className="inline-flex rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-1">
                    {(['https', 'http'] as const).map((protocol) => (
                      <button
                        key={protocol}
                        type="button"
                        aria-pressed={deploymentProtocol === protocol}
                        onClick={() => onDeploymentProtocolChange(protocol)}
                        className={`rounded-xl px-3 py-2 text-sm font-medium transition ${
                          deploymentProtocol === protocol
                            ? 'bg-[var(--brand-primary)] text-[var(--foreground-inverse)]'
                            : 'text-[var(--foreground-secondary)] hover:text-[var(--foreground-primary)]'
                        }`}
                      >
                        {protocol.toUpperCase()}
                      </button>
                    ))}
                  </div>
                  <ResourceInput
                    value={operationFields.ServerAddress}
                    placeholder="cdn.example.com:3000"
                    onChange={(event) =>
                      onServerAddressChange(event.target.value)
                    }
                  />
                </div>
              </ResourceField>
              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                  接入令牌
                </p>
                <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
                  {discoveryToken || '未生成'}
                </p>
              </div>
              <ResourceField
                label="一键部署命令"
                hint="命令会使用安装脚本自动注册节点程序，并默认安装缺少的运行依赖；没有发布包时会从源码构建。"
              >
                <CodeBlock className="break-all whitespace-pre-wrap">
                  {discoveryCommand || '请先填写可访问的面板地址。'}
                </CodeBlock>
              </ResourceField>
            </div>
          )}
        </AppCard>
        <AppCard title="版本与构建信息">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                服务端版本
              </p>
              <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                {publicStatus.version || '未公开'}
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                Server 启动时间
              </p>
              <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                {publicStatus.start_time
                  ? formatDateTime(new Date(publicStatus.start_time * 1000))
                  : '未公开'}
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                运行模式
              </p>
              <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                静态导出 + Go Server 托管
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4 md:col-span-2">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                版本入口
              </p>
              <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                点击顶栏“版本”可检查更新、查看 Release
                Notes，并直接触发服务端升级。
              </p>
            </div>
          </div>
        </AppCard>
      </div>
    </div>
  );
}
