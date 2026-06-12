import { EmptyState } from '@/components/feedback/empty-state';
import { AppCard } from '@/components/ui/app-card';
import { StatusBadge } from '@/components/ui/status-badge';
import { SettingsAccountBindingsCard } from '@/features/settings/components/settings-account-bindings-card';
import type {
  ExternalAccountBinding,
  SettingsProfile,
  UpdateSelfPayload,
} from '@/features/settings/types';
import {
  CodeBlock,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import type { AuthUser } from '@/types/auth';
import type { PublicStatus } from '@/types/public-status';

type ProfileFieldKey = keyof UpdateSelfPayload;

type SettingsPersonalSectionProps = {
  accessToken: string;
  accessTokenIsPending: boolean;
  busyKey: string | null;
  currentUser: AuthUser | null;
  emailAddress: string;
  emailCode: string;
  externalAccountMap: Map<string, ExternalAccountBinding>;
  profile: SettingsProfile;
  profileFields: UpdateSelfPayload;
  publicStatus: PublicStatus;
  onBindAuthSource: (sourceName: string) => void;
  onBindEmail: () => void;
  onCopyAccessToken: (token: string) => void | Promise<void>;
  onEmailAddressChange: (value: string) => void;
  onEmailCodeChange: (value: string) => void;
  onEmailTurnstileTokenChange: (value: string) => void;
  onEmailVerification: () => void;
  onGenerateAccessToken: () => void;
  onProfileFieldChange: (key: ProfileFieldKey, value: string) => void;
  onSaveProfile: () => void;
  onUnbindAuthSource: (id: number, label: string) => void | Promise<void>;
};

export function SettingsPersonalSection({
  accessToken,
  accessTokenIsPending,
  busyKey,
  currentUser,
  emailAddress,
  emailCode,
  externalAccountMap,
  profile,
  profileFields,
  publicStatus,
  onBindAuthSource,
  onBindEmail,
  onCopyAccessToken,
  onEmailAddressChange,
  onEmailCodeChange,
  onEmailTurnstileTokenChange,
  onEmailVerification,
  onGenerateAccessToken,
  onProfileFieldChange,
  onSaveProfile,
  onUnbindAuthSource,
}: SettingsPersonalSectionProps) {
  const roleLabel =
    currentUser?.role === 100
      ? '超级管理员'
      : currentUser?.role === 10
        ? '管理员'
        : '普通用户';
  const roleVariant =
    currentUser?.role === 100
      ? 'warning'
      : currentUser?.role === 10
        ? 'info'
        : 'success';

  return (
    <div className="space-y-6">
      <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <AppCard
          title="个人资料"
          description="可更新用户名、显示名称和密码。留空密码表示保持当前密码不变。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveProfile}
              disabled={busyKey === 'profile'}
            >
              {busyKey === 'profile' ? '保存中...' : '保存资料'}
            </PrimaryButton>
          }
        >
          <div className="space-y-5">
            <ResourceField label="用户名">
              <ResourceInput
                value={profileFields.username}
                onChange={(event) =>
                  onProfileFieldChange('username', event.target.value)
                }
                placeholder="请输入用户名"
              />
            </ResourceField>

            <ResourceField label="显示名称">
              <ResourceInput
                value={profileFields.display_name}
                onChange={(event) =>
                  onProfileFieldChange('display_name', event.target.value)
                }
                placeholder="请输入显示名称"
              />
            </ResourceField>

            <ResourceField label="新密码" hint="留空表示不修改密码。">
              <ResourceInput
                type="password"
                value={profileFields.password}
                onChange={(event) =>
                  onProfileFieldChange('password', event.target.value)
                }
                placeholder="请输入新密码"
              />
            </ResourceField>
          </div>
        </AppCard>

        <AppCard
          title="访问令牌"
          description="重置后会立即生成新的访问令牌，可用于自动化请求。"
          action={
            <PrimaryButton
              type="button"
              onClick={onGenerateAccessToken}
              disabled={accessTokenIsPending}
            >
              {accessTokenIsPending ? '生成中...' : '重置令牌'}
            </PrimaryButton>
          }
        >
          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                  当前角色
                </p>
                <div className="mt-2">
                  <StatusBadge label={roleLabel} variant={roleVariant} />
                </div>
              </div>
              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                  已绑定邮箱
                </p>
                <p className="mt-2 text-sm break-all text-[var(--foreground-primary)]">
                  {profile.email || '未绑定'}
                </p>
              </div>
            </div>

            {accessToken ? (
              <div className="space-y-3">
                <CodeBlock className="break-all whitespace-pre-wrap">
                  {accessToken}
                </CodeBlock>
                <SecondaryButton
                  type="button"
                  onClick={() => void onCopyAccessToken(accessToken)}
                >
                  复制令牌
                </SecondaryButton>
              </div>
            ) : (
              <EmptyState
                title="尚未生成令牌"
                description="点击“重置令牌”后，新的访问令牌会显示在这里。"
              />
            )}
          </div>
        </AppCard>
      </div>

      <SettingsAccountBindingsCard
        busyKey={busyKey}
        emailAddress={emailAddress}
        emailCode={emailCode}
        externalAccountMap={externalAccountMap}
        profile={profile}
        publicStatus={publicStatus}
        onBindAuthSource={onBindAuthSource}
        onUnbindAuthSource={onUnbindAuthSource}
        onEmailAddressChange={onEmailAddressChange}
        onEmailCodeChange={onEmailCodeChange}
        onEmailTurnstileTokenChange={onEmailTurnstileTokenChange}
        onEmailVerification={onEmailVerification}
        onBindEmail={onBindEmail}
      />
    </div>
  );
}
