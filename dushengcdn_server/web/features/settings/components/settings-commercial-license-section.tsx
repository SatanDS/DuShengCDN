import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import type {
  CommercialLicenseActivationRecord,
  CommercialLicenseIssuePayload,
  CommercialLicenseIssueResult,
  CommercialLicenseIssuerStatusPayload,
  CommercialLicenseStatusPayload,
} from '@/features/settings/types';
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
import { formatDateTime } from '@/lib/utils/date';
import { getErrorMessage } from '@/lib/utils/errors';

export type CommercialLicenseIssueForm = Omit<
  CommercialLicenseIssuePayload,
  'max_nodes' | 'max_sites'
> & {
  max_nodes: string;
  max_sites: string;
};

export const defaultCommercialLicenseIssueFields: CommercialLicenseIssueForm = {
  license_id: '',
  customer_id: '',
  customer_name: '',
  plan: 'business',
  features: ['all'],
  max_nodes: '20',
  max_sites: '200',
  issued_at: '',
  expires_at: '',
};

const commercialLicensePlanOptions = [
  { value: 'business', label: '商业版' },
  { value: 'professional', label: '专业版' },
  { value: 'enterprise', label: '企业版' },
];

const commercialLicenseFeatureOptions = [
  { value: 'all', label: '全部商业能力' },
  { value: 'acme-automation', label: 'ACME 自动证书' },
  { value: 'authoritative-dns', label: '自建权威 DNS' },
  { value: 'cloudflare-dns', label: 'Cloudflare DNS' },
  { value: 'gslb', label: 'GSLB 智能调度' },
  { value: 'ddos-protection', label: 'DDoS 自动防护' },
  { value: 'waf', label: 'WAF' },
  { value: 'cc-protection', label: 'CC 防护' },
  { value: 'country-region-access-control', label: '国家/地区访问控制' },
  { value: 'operator-access-control', label: '运营商访问控制' },
  { value: 'source-cidr-access-control', label: '来源网段访问控制' },
  { value: 'asn-access-control', label: 'ASN 访问控制' },
];

const commercialLicenseFeatureLabels = new Map(
  commercialLicenseFeatureOptions.map((option) => [option.value, option.label]),
);

function getCommercialLicenseFeatureLabel(feature: string) {
  return commercialLicenseFeatureLabels.get(feature) ?? feature;
}

function formatLicenseLimit(current: number, max: number) {
  return max > 0 ? `${current} / ${max}` : `${current} / 不限`;
}

function getLicenseBadgeVariant(
  status: CommercialLicenseStatusPayload['status'],
) {
  switch (status) {
    case 'valid':
      return 'success';
    case 'expiring':
      return 'warning';
    case 'activation_required':
    case 'lease_expired':
      return 'warning';
    case 'community':
      return 'info';
    default:
      return 'danger';
  }
}

function getActivationStatusBadge(record: CommercialLicenseActivationRecord) {
  switch (record.lease_status) {
    case 'active':
      return { label: '租约有效', variant: 'success' as const };
    case 'license_revoked':
      return { label: '授权已停用', variant: 'danger' as const };
    case 'activation_revoked':
      return { label: '机器已停用', variant: 'danger' as const };
    case 'expired':
      return { label: '租约过期', variant: 'warning' as const };
    default:
      return { label: '未签租约', variant: 'info' as const };
  }
}

type CommercialLicenseSettingsSectionProps = {
  busyKey: string | null;
  commercialLicense: CommercialLicenseStatusPayload | undefined;
  commercialLicenseError: unknown;
  commercialLicenseIsError: boolean;
  commercialLicenseIsLoading: boolean;
  commercialLicenseIssuer: CommercialLicenseIssuerStatusPayload | undefined;
  commercialLicenseIssuerError: unknown;
  commercialLicenseIssuerIsError: boolean;
  commercialLicenseIssuerIsLoading: boolean;
  commercialLicenseActivations: CommercialLicenseActivationRecord[] | undefined;
  commercialLicenseActivationsError: unknown;
  commercialLicenseActivationsIsError: boolean;
  commercialLicenseActivationsIsLoading: boolean;
  commercialLicenseToken: string;
  commercialLicenseIssueFields: CommercialLicenseIssueForm;
  issuedCommercialLicense: CommercialLicenseIssueResult | null;
  setCommercialLicenseToken: (value: string) => void;
  handleInstallCommercialLicense: () => void;
  handleActivateCommercialLicense: () => void;
  handleClearCommercialLicense: () => void | Promise<void>;
  updateCommercialLicenseIssueField: <
    Key extends keyof CommercialLicenseIssueForm,
  >(
    key: Key,
    value: CommercialLicenseIssueForm[Key],
  ) => void;
  toggleCommercialLicenseIssueFeature: (
    feature: string,
    checked: boolean,
  ) => void;
  handleIssueCommercialLicense: () => void;
  handleRevokeCommercialLicense: (
    record: CommercialLicenseActivationRecord,
  ) => void | Promise<void>;
  handleRestoreCommercialLicense: (
    record: CommercialLicenseActivationRecord,
  ) => void;
  handleDeleteCommercialLicenseActivation: (
    record: CommercialLicenseActivationRecord,
  ) => void | Promise<void>;
  handleCopy: (value: string, message: string) => void | Promise<void>;
  refreshCommercialLicenseActivations: () => void | Promise<unknown>;
};

export function CommercialLicenseSettingsSection({
  busyKey,
  commercialLicense,
  commercialLicenseError,
  commercialLicenseIsError,
  commercialLicenseIsLoading,
  commercialLicenseIssuer,
  commercialLicenseIssuerError,
  commercialLicenseIssuerIsError,
  commercialLicenseIssuerIsLoading,
  commercialLicenseActivations,
  commercialLicenseActivationsError,
  commercialLicenseActivationsIsError,
  commercialLicenseActivationsIsLoading,
  commercialLicenseToken,
  commercialLicenseIssueFields,
  issuedCommercialLicense,
  setCommercialLicenseToken,
  handleInstallCommercialLicense,
  handleActivateCommercialLicense,
  handleClearCommercialLicense,
  updateCommercialLicenseIssueField,
  toggleCommercialLicenseIssueFeature,
  handleIssueCommercialLicense,
  handleRevokeCommercialLicense,
  handleRestoreCommercialLicense,
  handleDeleteCommercialLicenseActivation,
  handleCopy,
  refreshCommercialLicenseActivations,
}: CommercialLicenseSettingsSectionProps) {
  if (commercialLicenseIsLoading || commercialLicenseIssuerIsLoading) {
    return <LoadingState />;
  }

  if (commercialLicenseIsError) {
    return (
      <ErrorState
        title="商业授权加载失败"
        description={getErrorMessage(commercialLicenseError)}
      />
    );
  }

  if (commercialLicenseIssuerIsError) {
    return (
      <ErrorState
        title="商业授权签发器加载失败"
        description={getErrorMessage(commercialLicenseIssuerError)}
      />
    );
  }

  const license = commercialLicense;
  if (!license) {
    return (
      <EmptyState
        title="商业授权状态不可用"
        description="服务端未返回授权状态。"
      />
    );
  }
  const issuer = commercialLicenseIssuer;

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)] xl:items-start">
      <AppCard
        title="授权状态"
        description="服务端会根据授权有效期和资源额度控制节点、站点创建。"
        action={
          license.fingerprint ? (
            <div className="flex flex-wrap gap-3">
              {license.online_activation_required ? (
                <SecondaryButton
                  type="button"
                  onClick={handleActivateCommercialLicense}
                  disabled={busyKey === 'commercial-license-activate'}
                >
                  {busyKey === 'commercial-license-activate'
                    ? '续约中...'
                    : '在线激活/续约'}
                </SecondaryButton>
              ) : null}
              <DangerButton
                type="button"
                onClick={() => void handleClearCommercialLicense()}
                disabled={busyKey === 'commercial-license-clear'}
              >
                {busyKey === 'commercial-license-clear'
                  ? '删除中...'
                  : '删除本机授权'}
              </DangerButton>
            </div>
          ) : null
        }
      >
        <div className="space-y-5">
          <div className="flex flex-wrap items-center gap-3">
            <StatusBadge
              label={license.status_label}
              variant={getLicenseBadgeVariant(license.status)}
            />
            <StatusBadge
              label={license.required ? '强制授权' : '兼容社区模式'}
              variant={license.required ? 'warning' : 'info'}
            />
            {license.signature_verified ? (
              <StatusBadge label="签名已验证" variant="success" />
            ) : null}
            {license.online_activation_required ? (
              <StatusBadge
                label={license.lease_status_label || '在线租约'}
                variant={
                  license.lease_status === 'valid' ? 'success' : 'warning'
                }
              />
            ) : null}
          </div>

          {license.last_validation_error ? (
            <InlineMessage
              tone="danger"
              message={license.last_validation_error}
            />
          ) : null}

          <div className="grid gap-4 md:grid-cols-2">
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                授权版本
              </p>
              <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                {license.plan_label}
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                授权指纹
              </p>
              <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
                {license.fingerprint || '未安装'}
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                授权客户
              </p>
              <p className="mt-2 text-sm font-semibold text-[var(--foreground-primary)]">
                {license.customer_name || license.customer_id || '未授权'}
              </p>
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                到期时间
              </p>
              <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                {license.expires_at
                  ? formatDateTime(new Date(license.expires_at))
                  : '长期有效'}
              </p>
              {license.days_until_expiry !== null &&
              license.days_until_expiry !== undefined ? (
                <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
                  剩余 {license.days_until_expiry} 天
                </p>
              ) : null}
            </div>
            {license.online_activation_required ? (
              <>
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                  <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                    在线租约
                  </p>
                  <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                    {license.lease_expires_at
                      ? formatDateTime(new Date(license.lease_expires_at))
                      : '未激活'}
                  </p>
                  {license.lease_renew_before_at ? (
                    <p className="mt-1 text-xs text-[var(--foreground-secondary)]">
                      提前续约：
                      {formatDateTime(new Date(license.lease_renew_before_at))}
                    </p>
                  ) : null}
                </div>
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                  <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                    激活标识
                  </p>
                  <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
                    {license.activation_id || '未激活'}
                  </p>
                </div>
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4 md:col-span-2">
                  <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                    机器指纹
                  </p>
                  <p className="mt-2 text-xs break-all text-[var(--foreground-primary)]">
                    {license.machine_fingerprint || '未生成'}
                  </p>
                  {license.build_watermark ? (
                    <p className="mt-1 text-xs break-all text-[var(--foreground-secondary)]">
                      构建水印：{license.build_watermark}
                    </p>
                  ) : null}
                </div>
              </>
            ) : null}
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                节点额度
              </p>
              <p className="mt-2 text-lg font-semibold text-[var(--foreground-primary)]">
                {formatLicenseLimit(license.current_nodes, license.max_nodes)}
              </p>
              {license.node_limit_exceeded ? (
                <p className="mt-1 text-xs text-[var(--status-danger-foreground)]">
                  当前节点数已超过授权额度
                </p>
              ) : null}
            </div>
            <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
              <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                站点额度
              </p>
              <p className="mt-2 text-lg font-semibold text-[var(--foreground-primary)]">
                {formatLicenseLimit(license.current_sites, license.max_sites)}
              </p>
              {license.site_limit_exceeded ? (
                <p className="mt-1 text-xs text-[var(--status-danger-foreground)]">
                  当前站点数已超过授权额度
                </p>
              ) : null}
            </div>
          </div>

          {license.features.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {license.features.map((feature) => (
                <StatusBadge
                  key={feature}
                  label={getCommercialLicenseFeatureLabel(feature)}
                  variant="info"
                />
              ))}
            </div>
          ) : null}

          <div className="space-y-4 border-t border-[var(--border-default)] pt-5">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
                  激活记录
                </h3>
                <p className="mt-1 text-sm text-[var(--foreground-secondary)]">
                  查看客户机器最近签发的在线租约，并按授权编号停用后续续租。
                </p>
              </div>
              <SecondaryButton
                type="button"
                onClick={() => void refreshCommercialLicenseActivations()}
              >
                刷新记录
              </SecondaryButton>
            </div>

            {commercialLicenseActivationsIsLoading ? (
              <LoadingState />
            ) : commercialLicenseActivationsIsError ? (
              <ErrorState
                title="激活记录加载失败"
                description={getErrorMessage(commercialLicenseActivationsError)}
              />
            ) : (commercialLicenseActivations?.length ?? 0) > 0 ? (
              <div className="space-y-3">
                {commercialLicenseActivations?.map((record) => {
                  const badge = getActivationStatusBadge(record);
                  const isRevoked = Boolean(record.license_revoked_at);
                  const revokeBusyKey = `commercial-license-revoke-${record.license_id}`;
                  const restoreBusyKey = `commercial-license-restore-${record.license_id}`;
                  const deleteBusyKey = `commercial-license-delete-${record.license_id}`;
                  return (
                    <div
                      key={record.activation_id || record.id}
                      className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4"
                    >
                      <div className="flex flex-wrap items-start justify-between gap-4">
                        <div className="min-w-0 space-y-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <StatusBadge
                              label={badge.label}
                              variant={badge.variant}
                            />
                            <span className="text-sm font-semibold break-all text-[var(--foreground-primary)]">
                              {record.license_id || '未知授权编号'}
                            </span>
                            {record.customer_id ? (
                              <span className="text-sm text-[var(--foreground-secondary)]">
                                {record.customer_id}
                              </span>
                            ) : null}
                          </div>
                          <div className="grid gap-2 text-xs text-[var(--foreground-secondary)] md:grid-cols-2">
                            <span className="break-all">
                              激活 ID：{record.activation_id || '—'}
                            </span>
                            <span className="break-all">
                              机器指纹：{record.machine_fingerprint || '—'}
                            </span>
                            <span>
                              服务端版本：{record.server_version || '—'}
                            </span>
                            <span>
                              主机名：{record.instance_hostname || '—'}
                            </span>
                            <span>
                              最近签发：
                              {record.last_lease_issued_at
                                ? formatDateTime(record.last_lease_issued_at)
                                : '—'}
                            </span>
                            <span>
                              租约到期：
                              {record.last_lease_expires_at
                                ? formatDateTime(record.last_lease_expires_at)
                                : '—'}
                            </span>
                          </div>
                          {isRevoked ? (
                            <p className="text-xs text-[var(--status-danger-foreground)]">
                              停用时间：
                              {formatDateTime(record.license_revoked_at)}
                              {record.license_revoke_reason
                                ? ` · ${record.license_revoke_reason}`
                                : ''}
                            </p>
                          ) : null}
                        </div>
                        <div className="flex flex-col items-stretch gap-2 sm:items-end">
                          {isRevoked ? (
                            <SecondaryButton
                              type="button"
                              onClick={() =>
                                handleRestoreCommercialLicense(record)
                              }
                              disabled={busyKey === restoreBusyKey}
                            >
                              {busyKey === restoreBusyKey
                                ? '恢复中...'
                                : '恢复授权'}
                            </SecondaryButton>
                          ) : (
                            <DangerButton
                              type="button"
                              onClick={() =>
                                void handleRevokeCommercialLicense(record)
                              }
                              disabled={busyKey === revokeBusyKey}
                            >
                              {busyKey === revokeBusyKey
                                ? '停用中...'
                                : '停用授权'}
                            </DangerButton>
                          )}
                          {isRevoked ? (
                            <DangerButton
                              type="button"
                              onClick={() =>
                                void handleDeleteCommercialLicenseActivation(
                                  record,
                                )
                              }
                              disabled={busyKey === deleteBusyKey}
                            >
                              {busyKey === deleteBusyKey
                                ? '删除中...'
                                : '删除授权'}
                            </DangerButton>
                          ) : null}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <EmptyState
                title="暂无激活记录"
                description="客户安装授权并完成在线激活后，会在这里显示最近签发的租约记录。"
              />
            )}
          </div>
        </div>
      </AppCard>

      <div className="space-y-6">
        <AppCard
          title="安装授权"
          description="许可证支持离线签名校验，安装后会立即刷新资源额度。"
          action={
            <PrimaryButton
              type="button"
              onClick={handleInstallCommercialLicense}
              disabled={busyKey === 'commercial-license-install'}
            >
              {busyKey === 'commercial-license-install'
                ? '安装中...'
                : '安装授权'}
            </PrimaryButton>
          }
        >
          <div className="space-y-5">
            <ResourceField
              label="许可证内容"
              hint="粘贴 dscdn_license_v1 开头的授权令牌。"
            >
              <ResourceTextarea
                value={commercialLicenseToken}
                onChange={(event) =>
                  setCommercialLicenseToken(event.target.value)
                }
                placeholder="dscdn_license_v1..."
                className="min-h-52 font-mono"
              />
            </ResourceField>
            <div className="flex flex-wrap gap-3">
              <SecondaryButton
                type="button"
                onClick={() =>
                  void handleCopy(commercialLicenseToken, '授权 token 已复制。')
                }
                disabled={!commercialLicenseToken.trim()}
              >
                复制 token
              </SecondaryButton>
              {license.last_validated_at ? (
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
                  最近校验：
                  {formatDateTime(new Date(license.last_validated_at))}
                </div>
              ) : null}
            </div>
          </div>
        </AppCard>

        <AppCard
          title="开发者签发"
          description="填写客户信息、到期时间和资源额度，直接生成可交付的商业授权 token。"
          action={
            <PrimaryButton
              type="button"
              onClick={handleIssueCommercialLicense}
              disabled={busyKey === 'commercial-license-issue' || !issuer?.available}
            >
              {busyKey === 'commercial-license-issue'
                ? '生成中...'
                : '生成授权'}
            </PrimaryButton>
          }
        >
          <div className="space-y-5">
            <InlineMessage
              tone={issuer?.available ? 'success' : 'warning'}
              message={
                issuer?.message ||
                '未配置签发私钥时，此处只显示安装授权能力。'
              }
            />

            {issuer?.available ? (
              <div className="grid gap-4 md:grid-cols-2">
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                  <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                    签发公钥指纹
                  </p>
                  <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
                    {issuer.public_key_fingerprint}
                  </p>
                </div>
                <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                  <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                    客户验签公钥
                  </p>
                  <button
                    type="button"
                    className="mt-2 text-left text-sm break-all text-[var(--brand-primary)]"
                    onClick={() =>
                      void handleCopy(issuer.public_key, '签发公钥已复制。')
                    }
                  >
                    {issuer.public_key}
                  </button>
                </div>
              </div>
            ) : null}

            <div className="grid gap-4 md:grid-cols-2">
              <ResourceField label="授权编号">
                <ResourceInput
                  value={commercialLicenseIssueFields.license_id}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'license_id',
                      event.target.value,
                    )
                  }
                  placeholder="lic-2026-001"
                />
              </ResourceField>
              <ResourceField label="授权版本">
                <ResourceSelect
                  value={commercialLicenseIssueFields.plan}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'plan',
                      event.target.value,
                    )
                  }
                >
                  {commercialLicensePlanOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </ResourceSelect>
              </ResourceField>
              <ResourceField
                label="客户名称"
                hint="客户名称和客户编号至少填写一项。"
              >
                <ResourceInput
                  value={commercialLicenseIssueFields.customer_name}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'customer_name',
                      event.target.value,
                    )
                  }
                  placeholder="Example Ltd."
                />
              </ResourceField>
              <ResourceField label="客户编号">
                <ResourceInput
                  value={commercialLicenseIssueFields.customer_id}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'customer_id',
                      event.target.value,
                    )
                  }
                  placeholder="cust-001"
                />
              </ResourceField>
              <ResourceField label="节点额度" hint="填写 0 表示不限。">
                <ResourceInput
                  type="number"
                  min="0"
                  value={commercialLicenseIssueFields.max_nodes}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'max_nodes',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField label="站点额度" hint="填写 0 表示不限。">
                <ResourceInput
                  type="number"
                  min="0"
                  value={commercialLicenseIssueFields.max_sites}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'max_sites',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label="签发时间"
                hint="留空表示当前时间，支持 YYYY-MM-DD 或 RFC3339。"
              >
                <ResourceInput
                  value={commercialLicenseIssueFields.issued_at ?? ''}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'issued_at',
                      event.target.value,
                    )
                  }
                  placeholder="now"
                />
              </ResourceField>
              <ResourceField
                label="到期时间"
                hint="留空表示长期有效，支持 YYYY-MM-DD 或 RFC3339。"
              >
                <ResourceInput
                  value={commercialLicenseIssueFields.expires_at ?? ''}
                  onChange={(event) =>
                    updateCommercialLicenseIssueField(
                      'expires_at',
                      event.target.value,
                    )
                  }
                  placeholder="2027-12-31"
                />
              </ResourceField>
            </div>

            <ResourceField label="授权能力" container="div">
              <div className="grid gap-3 md:grid-cols-2">
                {commercialLicenseFeatureOptions.map((option) => (
                  <ToggleField
                    key={option.value}
                    label={option.label}
                    checked={commercialLicenseIssueFields.features.includes(
                      option.value,
                    )}
                    disabled={
                      option.value !== 'all' &&
                      commercialLicenseIssueFields.features.includes('all')
                    }
                    onChange={(checked) =>
                      toggleCommercialLicenseIssueFeature(option.value, checked)
                    }
                  />
                ))}
              </div>
            </ResourceField>

            {issuedCommercialLicense ? (
              <div className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                <div className="flex flex-wrap items-center gap-3">
                  <StatusBadge
                    label={issuedCommercialLicense.status_label}
                    variant={getLicenseBadgeVariant(
                      issuedCommercialLicense.status,
                    )}
                  />
                  {issuedCommercialLicense.signature_verified ? (
                    <StatusBadge label="签发验签通过" variant="success" />
                  ) : null}
                  <SecondaryButton
                    type="button"
                    onClick={() =>
                      void handleCopy(
                        issuedCommercialLicense.token,
                        '授权 token 已复制。',
                      )
                    }
                  >
                    复制新 token
                  </SecondaryButton>
                </div>
                <CodeBlock className="max-h-40 break-all whitespace-pre-wrap">
                  {issuedCommercialLicense.token}
                </CodeBlock>
              </div>
            ) : null}
          </div>
        </AppCard>
      </div>
    </div>
  );
}
