'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

import {
  buildDomainRowsFromRoute,
  DomainListInput,
  type DomainListRow,
} from '@/features/proxy-routes/components/domain-list-input';
import {
  buildPayloadFromRoute,
  validateDomains,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import type { TlsCertificateItem } from '@/features/tls-certificates/types';
import {
  ResourceField,
  ResourceInput,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  type SaveHandler,
} from './proxy-route-config-shared';

const domainSettingsSchema = z
  .object({
    site_name: z
      .string()
      .trim()
      .min(1, '请输入站点标识')
      .max(255, '站点标识不能超过 255 个字符'),
    domain_rows: z
      .array(
        z.object({
          domain: z.string(),
          certificateId: z.string(),
        }),
      )
      .min(1),
    enabled: z.boolean(),
    redirect_http: z.boolean(),
  })
  .superRefine((value, context) => {
    const domains = value.domain_rows
      .map((item) => item.domain.trim().toLowerCase())
      .filter(Boolean);
    const error = validateDomains(domains);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['domain_rows'],
        message: error,
      });
    }

    const selectedCertificateCount = new Set(
      value.domain_rows
        .map((item) => Number(item.certificateId))
        .filter((item) => Number.isFinite(item) && item > 0),
    ).size;
    if (value.redirect_http && selectedCertificateCount === 0) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['redirect_http'],
        message: '启用 HTTP 跳转前，请先为域名选择证书',
      });
    }
  });

type DomainSettingsValues = z.infer<typeof domainSettingsSchema>;

function normalizeSelectedCertificateIDs(rows: DomainListRow[]) {
  return Array.from(
    new Set(
      rows
        .filter((item) => item.domain.trim() !== '')
        .map((item) => Number(item.certificateId))
        .filter((item) => Number.isFinite(item) && item > 0),
    ),
  );
}

function buildDomainCertificateIDs(rows: DomainListRow[]) {
  return rows
    .filter((item) => item.domain.trim() !== '')
    .map((item) => {
      const certificateID = Number(item.certificateId);
      return Number.isFinite(certificateID) && certificateID > 0
        ? certificateID
        : 0;
    });
}

function buildDomainRows(route: ProxyRouteItem) {
  const selectedCertIDs =
    route.cert_ids.length > 0
      ? route.cert_ids
      : route.cert_id
        ? [route.cert_id]
        : [];

  return buildDomainRowsFromRoute(
    route.domains,
    route.domain_cert_ids,
    selectedCertIDs,
  );
}

export function DomainSettingsSection({
  route,
  certificates,
  saving,
  onSave,
  suggestionSources,
}: {
  route: ProxyRouteItem;
  certificates: TlsCertificateItem[];
  saving: boolean;
  onSave: SaveHandler;
  suggestionSources: string[];
}) {
  const form = useForm<DomainSettingsValues>({
    resolver: zodResolver(domainSettingsSchema),
    defaultValues: {
      site_name: route.site_name,
      domain_rows: buildDomainRows(route),
      enabled: route.enabled,
      redirect_http: route.redirect_http,
    },
  });

  useEffect(() => {
    form.reset({
      site_name: route.site_name,
      domain_rows: buildDomainRows(route),
      enabled: route.enabled,
      redirect_http: route.redirect_http,
    });
  }, [form, route]);

  const selectedCertificateIDs = normalizeSelectedCertificateIDs(
    form.watch('domain_rows'),
  );

  return (
    <ConfigSectionShell
      title="域名设置"
      description="在一个列表里同时维护域名、证书和 HTTPS 跳转。保存时会自动汇总站点证书集合。"
      formId="proxy-route-domains-form"
      saving={saving}
    >
      <form
        id="proxy-route-domains-form"
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const domains = values.domain_rows
            .map((item) => item.domain.trim().toLowerCase())
            .filter(Boolean);
          const domainCertIDs = buildDomainCertificateIDs(values.domain_rows);
          const certIDs = normalizeSelectedCertificateIDs(values.domain_rows);

          onSave(
            buildPayloadFromRoute(route, {
              site_name: values.site_name.trim(),
              domain: domains[0],
              domains,
              enabled: values.enabled,
              enable_https: certIDs.length > 0,
              cert_id: certIDs[0] ?? null,
              cert_ids: certIDs,
              domain_cert_ids: domainCertIDs,
              redirect_http: certIDs.length > 0 ? values.redirect_http : false,
            }),
            { message: '域名设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用站点"
          description="关闭后会保留配置，但不会参与发布。"
          checked={form.watch('enabled')}
          onChange={(checked) =>
            form.setValue('enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="站点标识"
          hint="建议使用稳定、可读的业务标识，不必与域名完全一致。"
          error={form.formState.errors.site_name?.message}
        >
          <ResourceInput
            placeholder="marketing-site"
            {...form.register('site_name')}
          />
        </ResourceField>

        <ResourceField
          label="域名列表"
          hint="每行配置一个域名。可为不同域名选择不同证书，相同证书也可以重复选择。"
          error={
            form.formState.errors.domain_rows?.message as string | undefined
          }
          container="div"
        >
          <Controller
            control={form.control}
            name="domain_rows"
            render={({ field }) => (
              <DomainListInput
                rows={field.value}
                onChange={field.onChange}
                onBlur={field.onBlur}
                suggestionSources={suggestionSources}
                certificates={certificates}
              />
            )}
          />
        </ResourceField>

        <ToggleField
          label="HTTP 自动跳转到 HTTPS"
          description={
            selectedCertificateIDs.length > 0
              ? '开启后会额外生成 80 端口重定向规则。'
              : '至少为一个域名选择证书后才能启用。'
          }
          checked={form.watch('redirect_http')}
          disabled={selectedCertificateIDs.length === 0}
          onChange={(checked) =>
            form.setValue('redirect_http', checked, { shouldDirty: true })
          }
        />
      </form>
    </ConfigSectionShell>
  );
}
