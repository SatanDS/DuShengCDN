import { EmptyState } from '@/components/feedback/empty-state';
import { TurnstileWidget } from '@/components/forms/turnstile-widget';
import { AppCard } from '@/components/ui/app-card';
import type {
  ExternalAccountBinding,
  SettingsProfile,
} from '@/features/settings/types';
import {
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { formatDateTime } from '@/lib/utils/date';
import type { PublicStatus } from '@/types/public-status';

type SettingsAccountBindingsCardProps = {
  busyKey: string | null;
  emailAddress: string;
  emailCode: string;
  externalAccountMap: Map<string, ExternalAccountBinding>;
  profile: SettingsProfile;
  publicStatus: PublicStatus;
  onBindAuthSource: (sourceName: string) => void;
  onUnbindAuthSource: (id: number, label: string) => void | Promise<void>;
  onEmailAddressChange: (value: string) => void;
  onEmailCodeChange: (value: string) => void;
  onEmailTurnstileTokenChange: (value: string) => void;
  onEmailVerification: () => void;
  onBindEmail: () => void;
};

export function SettingsAccountBindingsCard({
  busyKey,
  emailAddress,
  emailCode,
  externalAccountMap,
  profile,
  publicStatus,
  onBindAuthSource,
  onUnbindAuthSource,
  onEmailAddressChange,
  onEmailCodeChange,
  onEmailTurnstileTokenChange,
  onEmailVerification,
  onBindEmail,
}: SettingsAccountBindingsCardProps) {
  return (
    <AppCard
      title="账号绑定"
      description="支持绑定已启用的认证源和邮箱地址，用于统一个人身份入口。"
    >
      <div className="grid gap-6 xl:grid-cols-2">
        <div className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
          <div className="space-y-1">
            <p className="text-base font-semibold text-[var(--foreground-primary)]">
              第三方认证源
            </p>
            <p className="text-sm leading-6 text-[var(--foreground-secondary)]">
              登录状态下发起授权会直接绑定到当前账号。
            </p>
          </div>
          <div className="space-y-3">
            {(publicStatus.auth_sources ?? []).length > 0 ? (
              publicStatus.auth_sources.map((source) => {
                const binding = externalAccountMap.get(source.name);
                const label = source.display_name || source.name;

                return (
                  <div
                    key={source.id}
                    className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-card)] px-4 py-3"
                  >
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                      <div className="min-w-0 space-y-1">
                        <p className="text-sm font-medium text-[var(--foreground-primary)]">
                          {label}
                        </p>
                        {binding ? (
                          <>
                            <p className="text-sm break-all text-[var(--foreground-secondary)]">
                              已绑定：
                              {binding.external_username ||
                                binding.email ||
                                '第三方账号'}
                            </p>
                            {binding.email ? (
                              <p className="text-xs break-all text-[var(--foreground-muted)]">
                                邮箱：{binding.email}
                              </p>
                            ) : null}
                            <p className="text-xs text-[var(--foreground-muted)]">
                              绑定时间：{formatDateTime(binding.created_at)}
                            </p>
                          </>
                        ) : (
                          <p className="text-sm text-[var(--foreground-secondary)]">
                            未绑定
                          </p>
                        )}
                      </div>
                      {binding ? (
                        <DangerButton
                          type="button"
                          onClick={() =>
                            void onUnbindAuthSource(binding.id, label)
                          }
                          disabled={
                            busyKey === `auth-source-unbind-${binding.id}`
                          }
                        >
                          解绑
                        </DangerButton>
                      ) : (
                        <PrimaryButton
                          type="button"
                          onClick={() => onBindAuthSource(source.name)}
                          disabled={
                            busyKey === `auth-source-bind-${source.name}`
                          }
                        >
                          绑定 {label}
                        </PrimaryButton>
                      )}
                    </div>
                  </div>
                );
              })
            ) : (
              <span className="text-sm text-[var(--foreground-secondary)]">
                当前未启用认证源。
              </span>
            )}
          </div>
        </div>

        <div className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
          <div className="space-y-1">
            <p className="text-base font-semibold text-[var(--foreground-primary)]">
              邮箱地址
            </p>
            <p className="text-sm leading-6 text-[var(--foreground-secondary)]">
              当前状态：{profile.email ? `已绑定 ${profile.email}` : '未绑定'}
            </p>
          </div>
          <div className="space-y-4">
            <ResourceField label="邮箱地址">
              <ResourceInput
                value={emailAddress}
                onChange={(event) => onEmailAddressChange(event.target.value)}
                placeholder="请输入邮箱地址"
              />
            </ResourceField>
            <ResourceField label="验证码">
              <ResourceInput
                value={emailCode}
                onChange={(event) => onEmailCodeChange(event.target.value)}
                placeholder="请输入邮箱验证码"
              />
            </ResourceField>
            {publicStatus.turnstile_check ? (
              publicStatus.turnstile_site_key ? (
                <TurnstileWidget
                  siteKey={publicStatus.turnstile_site_key}
                  onVerify={(token) => onEmailTurnstileTokenChange(token)}
                  onExpire={() => onEmailTurnstileTokenChange('')}
                  onError={() => onEmailTurnstileTokenChange('')}
                />
              ) : (
                <EmptyState
                  title="Turnstile 配置不完整"
                  description="当前系统已启用 Turnstile，但未配置 Site Key，邮箱绑定暂不可用。"
                />
              )
            ) : null}
            <div className="flex flex-wrap gap-2">
              <SecondaryButton
                type="button"
                onClick={onEmailVerification}
                disabled={busyKey === 'email-send'}
              >
                {busyKey === 'email-send' ? '发送中...' : '发送验证码'}
              </SecondaryButton>
              <PrimaryButton
                type="button"
                onClick={onBindEmail}
                disabled={busyKey === 'email-bind'}
              >
                {busyKey === 'email-bind' ? '绑定中...' : '绑定邮箱'}
              </PrimaryButton>
            </div>
          </div>
        </div>
      </div>
    </AppCard>
  );
}
