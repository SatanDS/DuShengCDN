'use client';

import type { ReactNode } from 'react';

import { AppCard } from '@/components/ui/app-card';
import type {
  ProxyRouteItem,
  ProxyRouteMutationPayload,
} from '@/features/proxy-routes/types';
import { PrimaryButton } from '@/features/shared/components/resource-primitives';

export type FeedbackState = {
  tone: 'success' | 'danger';
  message: string;
};

export type SaveContext = {
  message: string;
};

export type SaveHandler = (
  payload: ProxyRouteMutationPayload,
  context: SaveContext,
) => void;

export type ConfigSectionPresentationProps = {
  formId?: string;
  embedded?: boolean;
};

export function getProxyBufferingMode(
  route: ProxyRouteItem,
): 'default' | 'off' {
  return route.proxy_buffering_mode === 'off' ? 'off' : 'default';
}

export const proxyBufferingModeHint =
  '该设置与反向代理页、负载均衡页同步；Emby、Jellyfin、大文件下载和 Range 视频流建议关闭，避免边缘节点提前从源站读取大量内容。';

export const autoDNSNodePoolHint =
  '默认节点池用于未开启多节点智能解析时自动选 IP，也用于缓存清理、预热、攻击防护回退和运行时兜底。开启多节点智能解析后，用户访问会返回下方节点池权重里的 IP，不再由这里决定。';

export function ConfigSectionShell({
  title,
  description,
  formId,
  saving,
  embedded = false,
  children,
}: {
  title: string;
  description: string;
  formId: string;
  saving: boolean;
  embedded?: boolean;
  children: ReactNode;
}) {
  if (embedded) {
    return (
      <section className="space-y-5">
        <div className="flex flex-col gap-3 border-b border-[var(--border-default)] pb-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-[var(--foreground-primary)]">
              {title}
            </h3>
            <p className="mt-1 text-sm leading-6 text-[var(--foreground-secondary)]">
              {description}
            </p>
          </div>
          <PrimaryButton type="submit" form={formId} disabled={saving}>
            {saving ? '保存中...' : '保存'}
          </PrimaryButton>
        </div>
        {children}
      </section>
    );
  }

  return (
    <AppCard
      title={title}
      description={description}
      action={
        <PrimaryButton type="submit" form={formId} disabled={saving}>
          {saving ? '保存中...' : '保存'}
        </PrimaryButton>
      }
    >
      {children}
    </AppCard>
  );
}
