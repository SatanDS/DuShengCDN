import { AppCard } from '@/components/ui/app-card';
import { DNSSourceDatabaseMirrorCard } from '@/features/settings/components/settings-dns-source-database-mirror-card';
import type {
  DatabaseCleanupTarget,
  DNSSourceDatabaseMirrorStatus,
} from '@/features/settings/types';
import {
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

export type DatabaseSettingsFields = {
  DatabaseAutoCleanupEnabled: boolean;
  DatabaseAutoCleanupRetentionDays: string;
};

type DatabaseSettingsFieldKey = keyof DatabaseSettingsFields;

type DatabaseCleanupTargetCard = {
  target: DatabaseCleanupTarget;
  label: string;
  description: string;
};

const databaseCleanupTargets: DatabaseCleanupTargetCard[] = [
  {
    target: 'node_access_logs',
    label: '访问日志',
    description: '清理 node_access_logs，影响访问明细、IP 汇总与相关趋势查询。',
  },
  {
    target: 'node_metric_snapshots',
    label: '性能快照',
    description: '清理 node_metric_snapshots，影响节点资源趋势和总览资源统计。',
  },
  {
    target: 'node_request_reports',
    label: '请求聚合',
    description:
      '清理 node_request_reports，影响请求量、错误量与来源聚合展示。',
  },
  {
    target: 'dns_query_rollups',
    label: 'DNS 查询聚合',
    description:
      '清理 dns_query_rollups，影响本地自建解析查询量、响应码和来源作用域趋势。',
  },
];

type SettingsDatabaseSectionProps = {
  busyKey: string | null;
  databaseFields: DatabaseSettingsFields;
  mirrorIsLoading: boolean;
  mirrorStatus: DNSSourceDatabaseMirrorStatus | undefined;
  onAutoCleanupFieldChange: (
    key: DatabaseSettingsFieldKey,
    value: DatabaseSettingsFields[DatabaseSettingsFieldKey],
  ) => void;
  onRefreshMirror: () => void;
  onSaveAutoCleanup: () => void;
  onOpenCleanup: (target: DatabaseCleanupTarget, label: string) => void;
};

export function SettingsDatabaseSection({
  busyKey,
  databaseFields,
  mirrorIsLoading,
  mirrorStatus,
  onAutoCleanupFieldChange,
  onRefreshMirror,
  onSaveAutoCleanup,
  onOpenCleanup,
}: SettingsDatabaseSectionProps) {
  return (
    <div className="grid gap-6 xl:grid-cols-2 xl:items-start">
      <div className="space-y-6">
        <AppCard
          title="自动数据清理"
          description="每天凌晨 3 点自动清理超过保留期的观测数据，统一作用于访问日志、性能快照和请求聚合。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveAutoCleanup}
              disabled={busyKey === 'database-auto-cleanup'}
            >
              {busyKey === 'database-auto-cleanup'
                ? '保存中...'
                : '保存自动清理'}
            </PrimaryButton>
          }
        >
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <div className="space-y-5">
              <ToggleField
                label="启用每日自动清理"
                description="开启后，服务端每天自动删除保留天数之外的观测数据。"
                checked={databaseFields.DatabaseAutoCleanupEnabled}
                onChange={(checked) =>
                  onAutoCleanupFieldChange(
                    'DatabaseAutoCleanupEnabled',
                    checked,
                  )
                }
              />
              <div className="border-t border-[var(--border-default)] pt-5">
                <ResourceField
                  label="自动清理保留天数"
                  hint="必须至少保留 1 天，服务端不允许配置为 24 小时以内。"
                >
                  <ResourceInput
                    type="number"
                    min={1}
                    value={databaseFields.DatabaseAutoCleanupRetentionDays}
                    onChange={(event) =>
                      onAutoCleanupFieldChange(
                        'DatabaseAutoCleanupRetentionDays',
                        event.target.value,
                      )
                    }
                    placeholder="例如 30"
                  />
                </ResourceField>
                <div className="mt-4 grid gap-4 md:grid-cols-3">
                  <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                    <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                      触发频率
                    </p>
                    <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                      每天一次
                    </p>
                  </div>
                  <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                    <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                      默认执行时间
                    </p>
                    <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                      凌晨 3:00
                    </p>
                  </div>
                  <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-base)] px-4 py-4">
                    <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                      生效范围
                    </p>
                    <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                      四类观测表
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </AppCard>

        <DNSSourceDatabaseMirrorCard
          busyKey={busyKey}
          isLoading={mirrorIsLoading}
          status={mirrorStatus}
          onRefresh={onRefreshMirror}
        />
      </div>

      <AppCard
        title="数据清理"
        description="用于手动清理单类观测数据。保留天数留空时会直接删除该类数据的全部历史记录。"
      >
        <div className="grid gap-5 xl:grid-cols-3">
          {databaseCleanupTargets.map((item) => (
            <div
              key={item.target}
              className="rounded-[28px] border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5"
            >
              <div className="space-y-3">
                <div>
                  <p className="text-lg font-semibold text-[var(--foreground-primary)]">
                    {item.label}
                  </p>
                  <p className="mt-2 text-sm leading-6 text-[var(--foreground-secondary)]">
                    {item.description}
                  </p>
                </div>
                <DangerButton
                  type="button"
                  onClick={() => onOpenCleanup(item.target, item.label)}
                >
                  清理数据
                </DangerButton>
              </div>
            </div>
          ))}
        </div>
      </AppCard>
    </div>
  );
}
