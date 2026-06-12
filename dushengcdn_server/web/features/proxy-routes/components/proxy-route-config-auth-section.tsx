'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { buildPayloadFromRoute } from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  ResourceField,
  ResourceInput,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  type ConfigSectionPresentationProps,
  type SaveHandler,
} from './proxy-route-config-shared';

const basicAuthSchema = z
  .object({
    basic_auth_enabled: z.boolean(),
    basic_auth_username: z.string(),
    basic_auth_password: z.string(),
    basic_auth_password_configured: z.boolean(),
  })
  .superRefine((value, context) => {
    if (value.basic_auth_enabled) {
      if (!value.basic_auth_username.trim()) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['basic_auth_username'],
          message: '请输入账号',
        });
      }
      if (
        !value.basic_auth_password_configured &&
        !value.basic_auth_password.trim()
      ) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['basic_auth_password'],
          message: '请输入密码',
        });
      }
    }
  });

type BasicAuthValues = z.infer<typeof basicAuthSchema>;

export function BasicAuthSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-auth-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<BasicAuthValues>({
    resolver: zodResolver(basicAuthSchema),
    defaultValues: {
      basic_auth_enabled: route.basic_auth_enabled,
      basic_auth_username: route.basic_auth_username || '',
      basic_auth_password: '',
      basic_auth_password_configured: Boolean(
        route.basic_auth_password_configured,
      ),
    },
  });

  useEffect(() => {
    form.reset({
      basic_auth_enabled: route.basic_auth_enabled,
      basic_auth_username: route.basic_auth_username || '',
      basic_auth_password: '',
      basic_auth_password_configured: Boolean(
        route.basic_auth_password_configured,
      ),
    });
  }, [form, route]);

  const watchedEnabled = form.watch('basic_auth_enabled');

  return (
    <ConfigSectionShell
      title="认证配置"
      description="配置基础鉴权访问，需要输入账号密码才能访问网站。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          onSave(
            buildPayloadFromRoute(route, {
              basic_auth_enabled: values.basic_auth_enabled,
              basic_auth_username: values.basic_auth_username.trim(),
              basic_auth_password: values.basic_auth_password.trim(),
            }),
            { message: '认证配置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用 Basic Auth 鉴权"
          description="拦截所有请求，需要输入正确的账号和密码才能访问。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('basic_auth_enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="账号"
          error={form.formState.errors.basic_auth_username?.message}
        >
          <ResourceInput
            disabled={!watchedEnabled}
            placeholder="admin"
            {...form.register('basic_auth_username')}
          />
        </ResourceField>

        <ResourceField
          label="密码"
          error={form.formState.errors.basic_auth_password?.message}
        >
          <ResourceInput
            disabled={!watchedEnabled}
            type="password"
            autoComplete="new-password"
            placeholder="secret123"
            {...form.register('basic_auth_password')}
          />
        </ResourceField>
      </form>
    </ConfigSectionShell>
  );
}
