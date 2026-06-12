import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  DNSSourceDatabaseMirrorSourceStatus,
  DNSSourceDatabaseMirrorStatus,
} from '@/features/settings/types';
import { SecondaryButton } from '@/features/shared/components/resource-primitives';
import { formatDateTime } from '@/lib/utils/date';

const fallbackDNSSourceDatabaseMirrorSources: DNSSourceDatabaseMirrorSourceStatus[] =
  [
    {
      kind: 'operator',
      label: '运营商库',
      name: 'gaoyifan/china-operator-ip',
      available: false,
      file_count: 0,
      total_size: 0,
    },
    {
      kind: 'asn',
      label: 'ASN 库',
      name: 'GeoLite2-ASN',
      available: false,
      file_count: 0,
      total_size: 0,
    },
    {
      kind: 'country',
      label: 'Country 库',
      name: 'GeoLite2-Country',
      available: false,
      file_count: 0,
      total_size: 0,
    },
  ];

function formatMirrorSize(size: number) {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const fractionDigits = value >= 10 || unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(fractionDigits)} ${units[unitIndex]}`;
}

type DNSSourceDatabaseMirrorCardProps = {
  busyKey: string | null;
  isLoading: boolean;
  status: DNSSourceDatabaseMirrorStatus | undefined;
  onRefresh: () => void;
};

export function DNSSourceDatabaseMirrorCard({
  busyKey,
  isLoading,
  status: mirrorStatus,
  onRefresh,
}: DNSSourceDatabaseMirrorCardProps) {
  const sourceStatusMap = new Map(
    mirrorStatus?.sources?.map((source) => [source.kind, source]) ?? [],
  );
  const mirrorSources = fallbackDNSSourceDatabaseMirrorSources.map(
    (source) => sourceStatusMap.get(source.kind) ?? source,
  );

  return (
    <AppCard
      title="DNS 源库镜像"
      description="在面板服务器端刷新 gaoyifan/china-operator-ip、GeoLite2-ASN 与 GeoLite2-Country 的本地镜像，供 DNS 响应端 GitHub 下载失败时回退使用。"
      action={
        <div className="flex flex-wrap items-center justify-end gap-2">
          <StatusBadge
            label={
              isLoading
                ? '检测中'
                : mirrorStatus?.available
                  ? '已备份'
                  : '未备份'
            }
            variant={
              isLoading ? 'info' : mirrorStatus?.available ? 'success' : 'warning'
            }
          />
          <SecondaryButton
            type="button"
            onClick={onRefresh}
            disabled={busyKey === 'dns-source-database-refresh'}
          >
            {busyKey === 'dns-source-database-refresh'
              ? '触发中...'
              : '刷新源库镜像'}
          </SecondaryButton>
        </div>
      }
    >
      <div className="mb-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
              备份状态
            </p>
            <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
              {isLoading
                ? '正在检测源库镜像'
                : (mirrorStatus?.message ?? '面板端暂未完成源库备份。')}
            </p>
          </div>
          <p className="text-right text-sm text-[var(--foreground-secondary)]">
            {mirrorStatus?.updated_at
              ? `备份时间 ${formatDateTime(new Date(mirrorStatus.updated_at))}`
              : '暂无备份时间'}
          </p>
        </div>
        {mirrorStatus?.available ? (
          <p className="mt-3 text-xs text-[var(--foreground-muted)]">
            已备份 {mirrorStatus.source_count} 类源库，
            {mirrorStatus.file_count} 个文件，总体积{' '}
            {formatMirrorSize(mirrorStatus.total_size)}。
          </p>
        ) : mirrorStatus?.missing_kinds.length ? (
          <p className="mt-3 text-xs text-[var(--foreground-muted)]">
            缺少：{mirrorStatus.missing_kinds.join('、')}
          </p>
        ) : null}
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        {mirrorSources.map((source) => (
          <div
            key={source.kind}
            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
          >
            <div className="flex items-start justify-between gap-3">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                {source.label}
              </p>
              <StatusBadge
                className="shrink-0"
                label={isLoading ? '检测中' : source.available ? '已备份' : '未备份'}
                variant={isLoading ? 'info' : source.available ? 'success' : 'warning'}
              />
            </div>
            <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
              {source.name}
            </p>
            <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--foreground-muted)]">
              <span>{source.file_count} 个文件</span>
              <span>{formatMirrorSize(source.total_size)}</span>
            </div>
            <p className="mt-2 text-xs text-[var(--foreground-muted)]">
              {source.updated_at
                ? `备份时间 ${formatDateTime(new Date(source.updated_at))}`
                : '暂无备份时间'}
            </p>
          </div>
        ))}
      </div>
      <p className="mt-4 text-sm leading-6 text-[var(--foreground-secondary)]">
        面板启动时如果没有镜像会自动预热一次，之后每 7 天自动刷新；DNS
        响应端安装脚本也会创建 7 天更新定时器。
      </p>
    </AppCard>
  );
}
