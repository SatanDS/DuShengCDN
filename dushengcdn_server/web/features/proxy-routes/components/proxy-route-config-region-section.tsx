'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { buildPayloadFromRoute } from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  ResourceField,
  ResourceSelect,
  ResourceTextarea,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  type ConfigSectionPresentationProps,
  type SaveHandler,
} from './proxy-route-config-shared';

const regionRestrictionSchema = z
  .object({
    region_restriction_enabled: z.boolean(),
    region_restriction_mode: z.enum(['allow', 'block']),
    region_restriction_countries_text: z.string(),
  })
  .superRefine((value, context) => {
    if (!value.region_restriction_enabled) {
      return;
    }
    const countries = parseRegionCountriesText(
      value.region_restriction_countries_text,
    );
    if (countries.length === 0) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['region_restriction_countries_text'],
        message: '请至少填写一个国家或地区代码',
      });
      return;
    }
    for (const country of countries) {
      if (!/^[A-Z0-9]{2}$/.test(country)) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['region_restriction_countries_text'],
          message: `国家或地区代码格式不合法：${country}`,
        });
        return;
      }
    }
  });

type RegionRestrictionValues = z.infer<typeof regionRestrictionSchema>;

function parseRegionCountriesText(value: string) {
  return value
    .split(/[\s,，;；]+/)
    .map((item) => item.trim().toUpperCase())
    .filter(Boolean);
}

export function RegionRestrictionSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-region-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<RegionRestrictionValues>({
    resolver: zodResolver(regionRestrictionSchema),
    defaultValues: {
      region_restriction_enabled: route.region_restriction_enabled,
      region_restriction_mode: route.region_restriction_mode || 'block',
      region_restriction_countries_text:
        route.region_restriction_countries.join('\n'),
    },
  });

  useEffect(() => {
    form.reset({
      region_restriction_enabled: route.region_restriction_enabled,
      region_restriction_mode: route.region_restriction_mode || 'block',
      region_restriction_countries_text:
        route.region_restriction_countries.join('\n'),
    });
  }, [form, route]);

  const watchedEnabled = form.watch('region_restriction_enabled');
  const watchedMode = form.watch('region_restriction_mode');

  return (
    <ConfigSectionShell
      title="地区限制"
      description="基于节点本地 GeoIP 数据库按国家或地区代码放行或拦截访问。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const countries = parseRegionCountriesText(
            values.region_restriction_countries_text,
          );
          onSave(
            buildPayloadFromRoute(route, {
              region_restriction_enabled: values.region_restriction_enabled,
              region_restriction_mode: values.region_restriction_mode,
              region_restriction_countries: values.region_restriction_enabled
                ? countries
                : [],
            }),
            { message: '地区限制已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用地区限制"
          description="开启后，发布配置并由 Agent 应用后才会在节点侧生效。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('region_restriction_enabled', checked, {
              shouldDirty: true,
            })
          }
        />

        <ResourceField
          label="限制模式"
          hint={
            watchedMode === 'allow'
              ? '只允许列表内地区访问；无法识别地区的请求也会被拒绝。'
              : '拒绝列表内地区访问；无法识别地区的请求会继续放行。'
          }
        >
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('region_restriction_mode')}
          >
            <option value="block">拦截列表内地区</option>
            <option value="allow">只允许列表内地区</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="国家/地区代码"
          hint="每行或用逗号分隔一个 ISO 3166-1 两位代码，例如 CN、US、HK。"
          error={
            form.formState.errors.region_restriction_countries_text?.message
          }
        >
          <ResourceTextarea
            disabled={!watchedEnabled}
            className="min-h-36"
            placeholder={'CN\nUS\nHK'}
            {...form.register('region_restriction_countries_text')}
          />
        </ResourceField>

        <div className="rounded-2xl border border-[var(--status-info-border)] bg-[var(--status-info-soft)] px-4 py-3 text-sm leading-6 text-[var(--status-info-foreground)]">
          该功能由 Agent 节点本地 GeoIP 数据库识别真实客户端
          IP，前置反代需要正确透传 CF-Connecting-IP、X-Real-IP 或
          X-Forwarded-For。
        </div>
      </form>
    </ConfigSectionShell>
  );
}
