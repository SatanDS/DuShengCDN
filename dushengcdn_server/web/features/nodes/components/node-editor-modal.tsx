'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo, useEffect } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import { AppModal } from '@/components/ui/app-modal';
import worldGeoJson from '@/features/dashboard/data/world-geo.json';
import type { NodeItem, NodeMutationPayload } from '@/features/nodes/types';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

type GeoJsonGeometry = {
  type: string;
  coordinates: unknown;
};

type GeoJsonFeature = {
  geometry?: GeoJsonGeometry;
  properties?: {
    name?: string;
  };
};

type RegionOption = {
  label: string;
  latitude: number;
  longitude: number;
};

const nodeEditorSchema = z
  .object({
    name: z
      .string()
      .trim()
      .min(1, '请输入节点名')
      .max(128, '节点名不能超过 128 个字符'),
    ip: z.string().trim().max(64, '节点 IP 不能超过 64 个字符'),
    pool_name: z
      .string()
      .trim()
      .max(64, '节点池名称不能超过 64 个字符'),
    tags_text: z.string(),
    weight: z.coerce.number().int().min(1, '权重必须大于 0').max(1000, '权重不能超过 1000'),
    public_ips_text: z.string(),
    scheduling_enabled: z.boolean(),
    drain_mode: z.boolean(),
    auto_update_enabled: z.boolean(),
    geo_manual_override: z.boolean(),
    geo_region: z.string(),
    geo_name: z.string().trim().max(128, '位置名不能超过 128 个字符'),
    geo_latitude: z.string().trim(),
    geo_longitude: z.string().trim(),
  })
  .superRefine((values, ctx) => {
    if (!values.geo_manual_override) {
      return;
    }

    const hasLatitude = values.geo_latitude !== '';
    const hasLongitude = values.geo_longitude !== '';

    if (hasLatitude !== hasLongitude) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['geo_latitude'],
        message: '纬度和经度需要同时填写',
      });
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['geo_longitude'],
        message: '纬度和经度需要同时填写',
      });
      return;
    }

    if (hasLatitude) {
      const latitude = Number(values.geo_latitude);
      if (Number.isNaN(latitude) || latitude < -90 || latitude > 90) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['geo_latitude'],
          message: '纬度必须在 -90 到 90 之间',
        });
      }
    }

    if (hasLongitude) {
      const longitude = Number(values.geo_longitude);
      if (Number.isNaN(longitude) || longitude < -180 || longitude > 180) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['geo_longitude'],
          message: '经度必须在 -180 到 180 之间',
        });
      }
    }
  });

type NodeEditorValues = z.infer<typeof nodeEditorSchema>;

const defaultValues: NodeEditorValues = {
  name: '',
  ip: '',
  pool_name: 'default',
  tags_text: '',
  weight: 100,
  public_ips_text: '',
  scheduling_enabled: true,
  drain_mode: false,
  auto_update_enabled: false,
  geo_manual_override: false,
  geo_region: '',
  geo_name: '',
  geo_latitude: '',
  geo_longitude: '',
};

function collectPoints(value: unknown, points: Array<[number, number]>) {
  if (!Array.isArray(value)) {
    return;
  }
  if (
    value.length >= 2 &&
    typeof value[0] === 'number' &&
    typeof value[1] === 'number'
  ) {
    points.push([value[0], value[1]]);
    return;
  }
  for (const item of value) {
    collectPoints(item, points);
  }
}

function getRegionCenter(feature: GeoJsonFeature) {
  const points: Array<[number, number]> = [];
  collectPoints(feature.geometry?.coordinates, points);
  if (points.length === 0) {
    return null;
  }

  let minLongitude = Number.POSITIVE_INFINITY;
  let maxLongitude = Number.NEGATIVE_INFINITY;
  let minLatitude = Number.POSITIVE_INFINITY;
  let maxLatitude = Number.NEGATIVE_INFINITY;

  for (const [longitude, latitude] of points) {
    minLongitude = Math.min(minLongitude, longitude);
    maxLongitude = Math.max(maxLongitude, longitude);
    minLatitude = Math.min(minLatitude, latitude);
    maxLatitude = Math.max(maxLatitude, latitude);
  }

  return {
    longitude: Number(((minLongitude + maxLongitude) / 2).toFixed(4)),
    latitude: Number(((minLatitude + maxLatitude) / 2).toFixed(4)),
  };
}

function buildRegionOptions() {
  const features = ((worldGeoJson as { features?: GeoJsonFeature[] }).features ??
    []) as GeoJsonFeature[];
  const options = new Map<string, RegionOption>();

  for (const feature of features) {
    const label = feature.properties?.name?.trim();
    if (!label || options.has(label)) {
      continue;
    }
    const center = getRegionCenter(feature);
    if (!center) {
      continue;
    }
    options.set(label, {
      label,
      latitude: center.latitude,
      longitude: center.longitude,
    });
  }

  return Array.from(options.values()).sort((a, b) =>
    a.label.localeCompare(b.label),
  );
}

function buildFormValues(node?: Partial<NodeItem> | null): NodeEditorValues {
  if (!node) {
    return defaultValues;
  }

  return {
    name: node.name ?? '',
    ip: node.ip ?? '',
    pool_name: node.pool_name ?? 'default',
    tags_text: (node.tags ?? []).join('\n'),
    weight: node.weight ?? 100,
    public_ips_text: (node.public_ips ?? []).join('\n'),
    scheduling_enabled: node.scheduling_enabled ?? true,
    drain_mode: node.drain_mode ?? false,
    auto_update_enabled: node.auto_update_enabled ?? false,
    geo_manual_override: node.geo_manual_override ?? false,
    geo_region: node.geo_manual_override ? node.geo_name ?? '' : '',
    geo_name: node.geo_name ?? '',
    geo_latitude:
      node.geo_latitude === undefined || node.geo_latitude === null
        ? ''
        : String(node.geo_latitude),
    geo_longitude:
      node.geo_longitude === undefined || node.geo_longitude === null
        ? ''
        : String(node.geo_longitude),
  };
}

function toPayload(values: NodeEditorValues): NodeMutationPayload {
  const basePayload = {
    name: values.name.trim(),
    ip: values.ip.trim(),
    pool_name: values.pool_name.trim() || 'default',
    tags: linesFromTextarea(values.tags_text),
    weight: values.weight,
    public_ips: linesFromTextarea(values.public_ips_text),
    scheduling_enabled: values.scheduling_enabled,
    drain_mode: values.drain_mode,
    auto_update_enabled: values.auto_update_enabled,
  };

  if (!values.geo_manual_override) {
    return {
      ...basePayload,
      geo_manual_override: false,
      geo_name: '',
      geo_latitude: null,
      geo_longitude: null,
    };
  }

  return {
    ...basePayload,
    geo_manual_override: true,
    geo_name: values.geo_name.trim(),
    geo_latitude:
      values.geo_latitude.trim() === '' ? null : Number(values.geo_latitude),
    geo_longitude:
      values.geo_longitude.trim() === '' ? null : Number(values.geo_longitude),
  };
}

function linesFromTextarea(value: string) {
  return value
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function NodeEditorModal({
  isOpen,
  node,
  isSubmitting,
  title,
  description,
  submitLabel,
  onClose,
  onSubmit,
}: {
  isOpen: boolean;
  node?: Partial<NodeItem> | null;
  isSubmitting: boolean;
  title: string;
  description: string;
  submitLabel: string;
  onClose: () => void;
  onSubmit: (payload: NodeMutationPayload) => void;
}) {
  const form = useForm<NodeEditorValues>({
    resolver: zodResolver(nodeEditorSchema),
    defaultValues,
  });

  const watchedGeoManualOverride = useWatch({
    control: form.control,
    name: 'geo_manual_override',
  });
  const regionOptions = useMemo(() => buildRegionOptions(), []);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    form.reset(buildFormValues(node));
  }, [form, isOpen, node]);

  const handleSubmit = form.handleSubmit((values) => {
    onSubmit(toPayload(values));
  });

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={title}
      description={description}
      footer={
        <div className="flex flex-wrap justify-end gap-3">
          <SecondaryButton
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
          >
            取消
          </SecondaryButton>
          <PrimaryButton
            type="submit"
            form="node-editor-form"
            disabled={isSubmitting}
          >
            {isSubmitting ? '保存中...' : submitLabel}
          </PrimaryButton>
        </div>
      }
    >
      <form id="node-editor-form" className="space-y-5" onSubmit={handleSubmit}>
        <ResourceField
          label="节点名"
          hint="示例：shanghai-edge-1"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="shanghai-edge-1"
            {...form.register('name')}
          />
        </ResourceField>

        <ResourceField
          label="节点 IP"
          hint="可手动维护节点当前对外 IP；留空则等待 Agent 注册或心跳自动回填。"
          error={form.formState.errors.ip?.message}
        >
          <ResourceInput placeholder="203.0.113.10" {...form.register('ip')} />
        </ResourceField>

        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="节点池"
            hint="同一节点池会作为 DNS 调度和缓存操作的默认目标范围。"
            error={form.formState.errors.pool_name?.message}
          >
            <ResourceInput
              placeholder="default"
              {...form.register('pool_name')}
            />
          </ResourceField>

          <ResourceField
            label="调度权重"
            hint="加权模式下优先选择权重更高的节点。"
            error={form.formState.errors.weight?.message}
          >
            <ResourceInput
              type="number"
              min={1}
              max={1000}
              {...form.register('weight', { valueAsNumber: true })}
            />
          </ResourceField>
        </div>

        <ResourceField
          label="公网 IP 池"
          hint="每行一个 A/AAAA 地址；留空时使用节点 IP。自动 DNS 会从这里选取目标。"
        >
          <ResourceTextarea
            className="min-h-24"
            placeholder={'203.0.113.10\n2001:db8::10'}
            {...form.register('public_ips_text')}
          />
        </ResourceField>

        <ResourceField
          label="节点标签"
          hint="每行一个标签，用于后续筛选和运维标记。"
        >
          <ResourceTextarea
            className="min-h-20"
            placeholder={'edge\nasia'}
            {...form.register('tags_text')}
          />
        </ResourceField>

        <div className="grid gap-4 md:grid-cols-2">
          <ToggleField
            label="参与自动调度"
            description="关闭后自动 DNS 不再选择该节点。"
            checked={form.watch('scheduling_enabled')}
            onChange={(checked) => {
              form.setValue('scheduling_enabled', checked, {
                shouldDirty: true,
                shouldValidate: true,
              });
            }}
          />

          <ToggleField
            label="排空模式"
            description="开启后自动 DNS 和缓存运行时操作都会跳过该节点。"
            checked={form.watch('drain_mode')}
            onChange={(checked) => {
              form.setValue('drain_mode', checked, {
                shouldDirty: true,
                shouldValidate: true,
              });
            }}
          />
        </div>

        <ToggleField
          label="Agent 自动更新"
          description="开启后，Agent 会在心跳或 WebSocket 状态同步时自动检查正式版 Release，并在发现新版本后自更新。"
          checked={form.watch('auto_update_enabled')}
          onChange={(checked) => {
            form.setValue('auto_update_enabled', checked, {
              shouldDirty: true,
              shouldValidate: true,
            });
          }}
        />

        <ToggleField
          label="手动指定地图地区"
          description="关闭时，节点会根据当前 IP 自动解析归属地；开启后使用你手动选择的地区与坐标。"
          checked={watchedGeoManualOverride}
          onChange={(checked) => {
            form.setValue('geo_manual_override', checked, {
              shouldDirty: true,
              shouldValidate: true,
            });
            if (!checked) {
              form.setValue('geo_region', '', { shouldDirty: true });
              form.setValue('geo_name', '', { shouldDirty: true });
              form.setValue('geo_latitude', '', { shouldDirty: true });
              form.setValue('geo_longitude', '', { shouldDirty: true });
            }
          }}
        />

        {watchedGeoManualOverride ? (
          <ResourceField
            label="地区选择"
            hint="选择后会自动填充位置名与地图坐标，你也可以继续微调。"
          >
            <ResourceSelect
              value={form.watch('geo_region')}
              onChange={(event) => {
                const regionName = event.target.value;
                form.setValue('geo_region', regionName, {
                  shouldDirty: true,
                });
                const selectedRegion = regionOptions.find(
                  (item) => item.label === regionName,
                );
                if (!selectedRegion) {
                  return;
                }
                form.setValue('geo_name', selectedRegion.label, {
                  shouldDirty: true,
                  shouldValidate: true,
                });
                form.setValue('geo_latitude', String(selectedRegion.latitude), {
                  shouldDirty: true,
                  shouldValidate: true,
                });
                form.setValue(
                  'geo_longitude',
                  String(selectedRegion.longitude),
                  {
                    shouldDirty: true,
                    shouldValidate: true,
                  },
                );
              }}
            >
              <option value="">请选择地区</option>
              {regionOptions.map((option) => (
                <option key={option.label} value={option.label}>
                  {option.label}
                </option>
              ))}
            </ResourceSelect>
          </ResourceField>
        ) : null}

        <ResourceField
          label="地图位置名"
          hint={
            watchedGeoManualOverride
              ? '可在自动填充后继续修改，例如使用更贴近业务的展示名称。'
              : '自动解析模式下，该字段会由系统根据节点 IP 回填。'
          }
          error={form.formState.errors.geo_name?.message}
        >
          <ResourceInput
            placeholder="Shanghai"
            disabled={!watchedGeoManualOverride}
            {...form.register('geo_name')}
          />
        </ResourceField>

        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="纬度"
            hint="范围 -90 到 90，例如上海约为 31.2304"
            error={form.formState.errors.geo_latitude?.message}
          >
            <ResourceInput
              placeholder="31.2304"
              disabled={!watchedGeoManualOverride}
              {...form.register('geo_latitude')}
            />
          </ResourceField>

          <ResourceField
            label="经度"
            hint="范围 -180 到 180，例如上海约为 121.4737"
            error={form.formState.errors.geo_longitude?.message}
          >
            <ResourceInput
              placeholder="121.4737"
              disabled={!watchedGeoManualOverride}
              {...form.register('geo_longitude')}
            />
          </ResourceField>
        </div>
      </form>
    </AppModal>
  );
}
