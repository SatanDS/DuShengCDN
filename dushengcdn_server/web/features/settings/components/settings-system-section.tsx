import { AppCard } from '@/components/ui/app-card';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

export type SystemSettingsFields = {
  PasswordLoginEnabled: boolean;
  PasswordRegisterEnabled: boolean;
  EmailVerificationEnabled: boolean;
  ServerAddress: string;
  SMTPServer: string;
  SMTPPort: string;
  SMTPAccount: string;
  SMTPToken: string;
};

export type SystemSettingsTextFieldKey =
  | 'ServerAddress'
  | 'SMTPServer'
  | 'SMTPPort'
  | 'SMTPAccount'
  | 'SMTPToken';

export type SystemSettingsToggleKey =
  | 'PasswordLoginEnabled'
  | 'PasswordRegisterEnabled'
  | 'EmailVerificationEnabled';

export type RateLimitOperationFields = {
  GlobalApiRateLimitNum: string;
  GlobalApiRateLimitDuration: string;
  GlobalWebRateLimitNum: string;
  GlobalWebRateLimitDuration: string;
  DNSWorkerAPIRateLimitNum: string;
  DNSWorkerAPIRateLimitDuration: string;
  UploadRateLimitNum: string;
  UploadRateLimitDuration: string;
  DownloadRateLimitNum: string;
  DownloadRateLimitDuration: string;
  CriticalRateLimitNum: string;
  CriticalRateLimitDuration: string;
};

export type RateLimitOperationFieldKey = keyof RateLimitOperationFields;

type SystemSettingsSectionProps = {
  authSourcesCount: number;
  busyKey: string | null;
  systemFields: SystemSettingsFields;
  operationFields: RateLimitOperationFields;
  formatSecondsLabel: (value: string) => string;
  onOpenAuthSources: () => void;
  onToggleOption: (key: SystemSettingsToggleKey, nextValue: boolean) => void;
  onSystemFieldChange: (
    key: SystemSettingsTextFieldKey,
    value: string,
  ) => void;
  onOperationFieldChange: (
    key: RateLimitOperationFieldKey,
    value: string,
  ) => void;
  onSaveGeneralSettings: () => void;
  onSaveSmtpSettings: () => void;
  onSaveRateLimitSettings: () => void;
};

export function SystemSettingsSection({
  authSourcesCount,
  busyKey,
  systemFields,
  operationFields,
  formatSecondsLabel,
  onOpenAuthSources,
  onToggleOption,
  onSystemFieldChange,
  onOperationFieldChange,
  onSaveGeneralSettings,
  onSaveSmtpSettings,
  onSaveRateLimitSettings,
}: SystemSettingsSectionProps) {
  return (
    <div className="space-y-6">
      <AppCard
        title="登录与注册开关"
        description="切换后立即生效，无需重启服务。"
        action={
          <SecondaryButton type="button" onClick={onOpenAuthSources}>
            配置认证源
          </SecondaryButton>
        }
      >
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <ToggleField
            label="允许密码登录"
            description="关闭后将无法使用用户名密码登录。"
            checked={systemFields.PasswordLoginEnabled}
            onChange={(checked) =>
              onToggleOption('PasswordLoginEnabled', checked)
            }
            disabled={busyKey === 'toggle-PasswordLoginEnabled'}
          />
          <ToggleField
            label="允许密码注册"
            description="关闭后新用户不能通过密码方式注册。"
            checked={systemFields.PasswordRegisterEnabled}
            onChange={(checked) =>
              onToggleOption('PasswordRegisterEnabled', checked)
            }
            disabled={busyKey === 'toggle-PasswordRegisterEnabled'}
          />
          <ToggleField
            label="注册需要邮箱验证"
            description="开启后，新用户注册必须先完成邮箱验证码校验。"
            checked={systemFields.EmailVerificationEnabled}
            onChange={(checked) =>
              onToggleOption('EmailVerificationEnabled', checked)
            }
            disabled={busyKey === 'toggle-EmailVerificationEnabled'}
          />
        </div>
        <div className="mt-5 text-sm text-[var(--foreground-secondary)]">
          当前已配置 {authSourcesCount} 个认证源。
        </div>
      </AppCard>

      <div className="grid gap-6 xl:grid-cols-[1fr_1fr]">
        <AppCard
          title="通用设置"
          description="服务器地址会影响邮件链接、OAuth 回调和部署命令展示。"
          action={
            <div className="flex flex-wrap gap-2">
              <SecondaryButton
                type="button"
                onClick={() =>
                  window.open(
                    '/swagger/index.html',
                    '_blank',
                    'noopener,noreferrer',
                  )
                }
              >
                打开接口文档
              </SecondaryButton>
              <PrimaryButton
                type="button"
                onClick={onSaveGeneralSettings}
                disabled={busyKey === 'system-general'}
              >
                {busyKey === 'system-general' ? '保存中...' : '保存通用设置'}
              </PrimaryButton>
            </div>
          }
        >
          <ResourceField label="服务器地址">
            <ResourceInput
              value={systemFields.ServerAddress}
              onChange={(event) =>
                onSystemFieldChange('ServerAddress', event.target.value)
              }
              placeholder="https://yourdomain.com"
            />
          </ResourceField>
        </AppCard>

        <AppCard
          title="SMTP 设置"
          description="用于邮件验证码、密码重置和其他邮件通知发送。"
          action={
            <PrimaryButton
              type="button"
              onClick={onSaveSmtpSettings}
              disabled={busyKey === 'system-smtp'}
            >
              {busyKey === 'system-smtp' ? '保存中...' : '保存 SMTP 设置'}
            </PrimaryButton>
          }
        >
          <div className="grid gap-5 md:grid-cols-2">
            <ResourceField label="SMTP 服务器">
              <ResourceInput
                value={systemFields.SMTPServer}
                onChange={(event) =>
                  onSystemFieldChange('SMTPServer', event.target.value)
                }
                placeholder="smtp.qq.com"
              />
            </ResourceField>
            <ResourceField label="SMTP 端口">
              <ResourceInput
                value={systemFields.SMTPPort}
                onChange={(event) =>
                  onSystemFieldChange('SMTPPort', event.target.value)
                }
                placeholder="587"
              />
            </ResourceField>
            <ResourceField label="SMTP 账户">
              <ResourceInput
                value={systemFields.SMTPAccount}
                onChange={(event) =>
                  onSystemFieldChange('SMTPAccount', event.target.value)
                }
                placeholder="name@example.com"
              />
            </ResourceField>
            <ResourceField
              label="SMTP 凭证"
              hint="因安全原因不会回显历史密钥，留空表示不更新。"
            >
              <ResourceInput
                type="password"
                value={systemFields.SMTPToken}
                onChange={(event) =>
                  onSystemFieldChange('SMTPToken', event.target.value)
                }
                placeholder="请输入新的 SMTP 凭证"
              />
            </ResourceField>
          </div>
        </AppCard>
      </div>
      <AppCard
        title="请求限流设置"
        description="按来源 IP 生效，保存后立即影响 Web、API、上传下载及登录注册等敏感接口。时间单位均为秒。"
        action={
          <PrimaryButton
            type="button"
            onClick={onSaveRateLimitSettings}
            disabled={busyKey === 'operation-rate-limit'}
          >
            {busyKey === 'operation-rate-limit' ? '保存中...' : '保存限流设置'}
          </PrimaryButton>
        }
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <p className="text-sm font-semibold text-[var(--foreground-primary)]">
              全局 API 限流
            </p>
            <p className="mt-1 text-sm text-[var(--foreground-muted)]">
              作用于 `/api` 下的通用请求。
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <ResourceField label="请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.GlobalApiRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'GlobalApiRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`时间窗口 (${formatSecondsLabel(operationFields.GlobalApiRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.GlobalApiRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'GlobalApiRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
            </div>
          </div>

          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <p className="text-sm font-semibold text-[var(--foreground-primary)]">
              全局 Web 限流
            </p>
            <p className="mt-1 text-sm text-[var(--foreground-muted)]">
              作用于页面和静态资源请求，过低会更容易触发 429。
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <ResourceField label="请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.GlobalWebRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'GlobalWebRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`时间窗口 (${formatSecondsLabel(operationFields.GlobalWebRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.GlobalWebRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'GlobalWebRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
            </div>
          </div>

          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <p className="text-sm font-semibold text-[var(--foreground-primary)]">
              DNS Worker API 限流
            </p>
            <p className="mt-1 text-sm text-[var(--foreground-muted)]">
              作用于 DNS 响应端心跳、快照拉取和统计上报接口。
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <ResourceField label="请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.DNSWorkerAPIRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'DNSWorkerAPIRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`时间窗口 (${formatSecondsLabel(operationFields.DNSWorkerAPIRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.DNSWorkerAPIRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'DNSWorkerAPIRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
            </div>
          </div>

          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <p className="text-sm font-semibold text-[var(--foreground-primary)]">
              上传 / 下载限流
            </p>
            <p className="mt-1 text-sm text-[var(--foreground-muted)]">
              用于文件上传与下载接口，建议保留相对严格的阈值。
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <ResourceField label="上传请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.UploadRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'UploadRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`上传窗口 (${formatSecondsLabel(operationFields.UploadRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.UploadRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'UploadRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField label="下载请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.DownloadRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'DownloadRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`下载窗口 (${formatSecondsLabel(operationFields.DownloadRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.DownloadRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'DownloadRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
            </div>
          </div>

          <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5">
            <p className="text-sm font-semibold text-[var(--foreground-primary)]">
              敏感接口限流
            </p>
            <p className="mt-1 text-sm text-[var(--foreground-muted)]">
              用于登录、注册、验证码、重置密码和 OAuth 等接口。
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-2">
              <ResourceField label="请求次数">
                <ResourceInput
                  type="number"
                  value={operationFields.CriticalRateLimitNum}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'CriticalRateLimitNum',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
              <ResourceField
                label={`时间窗口 (${formatSecondsLabel(operationFields.CriticalRateLimitDuration)})`}
              >
                <ResourceInput
                  type="number"
                  value={operationFields.CriticalRateLimitDuration}
                  onChange={(event) =>
                    onOperationFieldChange(
                      'CriticalRateLimitDuration',
                      event.target.value,
                    )
                  }
                />
              </ResourceField>
            </div>
          </div>
        </div>
      </AppCard>
    </div>
  );
}
