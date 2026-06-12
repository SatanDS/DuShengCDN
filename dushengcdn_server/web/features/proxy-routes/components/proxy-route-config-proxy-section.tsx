'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import type { NodeItem } from '@/features/nodes/types';
import {
  formatNodeName,
  getNodesForPool,
} from '@/features/proxy-routes/components/node-pool-select';
import {
  buildPayloadFromRoute,
  customHeadersToText,
  parseCustomHeadersText,
  parseOriginUrl,
  parseOriginUrls,
  validateOriginHost,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
} from '@/features/shared/components/resource-primitives';

import {
  autoDNSNodePoolHint,
  ConfigSectionShell,
  getProxyBufferingMode,
  proxyBufferingModeHint,
  type SaveHandler,
} from './proxy-route-config-shared';

const reverseProxySchema = z
  .object({
    origin_urls_text: z.string().trim().min(1, '请至少填写一个源站地址'),
    node_pool: z.string().trim().max(64, '节点池名称不能超过 64 个字符'),
    origin_host: z.string(),
    proxy_buffering_mode: z.enum(['default', 'off']),
    custom_headers_text: z.string(),
    remark: z.string().max(255, '备注不能超过 255 个字符'),
  })
  .superRefine((value, context) => {
    const { error } = parseOriginUrls(value.origin_urls_text);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['origin_urls_text'],
        message: error,
      });
    }

    const originHostError = validateOriginHost(value.origin_host);
    if (originHostError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['origin_host'],
        message: originHostError,
      });
    }

    const { error: headerError } = parseCustomHeadersText(
      value.custom_headers_text,
    );
    if (headerError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['custom_headers_text'],
        message: headerError,
      });
    }
  });

type ReverseProxyValues = z.infer<typeof reverseProxySchema>;

export function ReverseProxySection({
  route,
  saving,
  onSave,
  nodePoolOptions,
  nodes = [],
  nodePoolsLoading,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  nodePoolOptions: string[];
  nodes?: NodeItem[];
  nodePoolsLoading: boolean;
  onSave: SaveHandler;
}) {
  const form = useForm<ReverseProxyValues>({
    resolver: zodResolver(reverseProxySchema),
    defaultValues: {
      origin_urls_text: route.upstream_list.join('\n'),
      node_pool: route.node_pool || 'default',
      origin_host: route.origin_host || '',
      proxy_buffering_mode: getProxyBufferingMode(route),
      custom_headers_text: customHeadersToText(route.custom_header_list),
      remark: route.remark || '',
    },
  });

  useEffect(() => {
    form.reset({
      origin_urls_text: route.upstream_list.join('\n'),
      node_pool: route.node_pool || 'default',
      origin_host: route.origin_host || '',
      proxy_buffering_mode: getProxyBufferingMode(route),
      custom_headers_text: customHeadersToText(route.custom_header_list),
      remark: route.remark || '',
    });
  }, [form, route]);

  const selectedNodePool = form.watch('node_pool');
  const normalizedSelectedNodePool = selectedNodePool.trim() || 'default';
  const selectedNodePoolUnknown =
    normalizedSelectedNodePool !== '' &&
    !nodePoolOptions.includes(normalizedSelectedNodePool);
  const nodesInSelectedPool = useMemo(
    () => getNodesForPool(nodes, normalizedSelectedNodePool),
    [nodes, normalizedSelectedNodePool],
  );
  const [selectedNodeID, setSelectedNodeID] = useState('');

  useEffect(() => {
    setSelectedNodeID((current) => {
      if (nodesInSelectedPool.some((node) => node.node_id === current)) {
        return current;
      }
      return nodesInSelectedPool[0]?.node_id ?? '';
    });
  }, [nodesInSelectedPool]);

  return (
    <ConfigSectionShell
      title="反向代理"
      description="第一行作为主源站；填写多行时会自动进入多源站负载均衡模式。"
      formId="proxy-route-proxy-form"
      saving={saving}
    >
      <form
        id="proxy-route-proxy-form"
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const { urls } = parseOriginUrls(values.origin_urls_text);
          const primaryOrigin = parseOriginUrl(urls[0]);
          const { headers } = parseCustomHeadersText(
            values.custom_headers_text,
          );

          onSave(
            buildPayloadFromRoute(route, {
              origin_id: null,
              origin_url: urls[0],
              origin_scheme: primaryOrigin.scheme,
              origin_address: primaryOrigin.address,
              origin_port: primaryOrigin.port,
              origin_uri: primaryOrigin.uri,
              node_pool: values.node_pool.trim() || 'default',
              origin_host: values.origin_host.trim(),
              origin_host_header: values.origin_host.trim(),
              proxy_buffering_mode: values.proxy_buffering_mode,
              upstreams: urls.slice(1),
              custom_headers: headers,
              remark: values.remark.trim(),
            }),
            { message: '反向代理设置已保存。' },
          );
        })}
      >
        <ResourceField
          label="源站地址"
          hint="每行一个完整 URL，协议和端口都在这里配置，例如 https://origin.internal:443。多源站模式下不要带 path 或 query。"
          error={form.formState.errors.origin_urls_text?.message}
        >
          <ResourceTextarea
            aria-label="源站地址"
            className="min-h-40"
            placeholder={
              'https://origin-a.internal:443\nhttps://origin-b.internal:443'
            }
            {...form.register('origin_urls_text')}
          />
        </ResourceField>

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(260px,420px)]">
          <ResourceField
            label="默认节点池"
            hint={autoDNSNodePoolHint}
            error={form.formState.errors.node_pool?.message}
            container="div"
          >
            <ResourceSelect
              name="node_pool"
              aria-label="节点池选择"
              value={normalizedSelectedNodePool}
              disabled={nodePoolsLoading}
              onChange={(event) =>
                form.setValue('node_pool', event.target.value, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            >
              {selectedNodePoolUnknown ? (
                <option value={normalizedSelectedNodePool}>
                  {normalizedSelectedNodePool}（未找到）
                </option>
              ) : null}
              {nodePoolOptions.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </ResourceSelect>
            {selectedNodePoolUnknown ? (
              <p className="mt-2 text-xs text-[var(--status-warning-foreground)]">
                当前节点池不在现有节点池列表里，请从下拉选择真实存在的节点池。
              </p>
            ) : null}
          </ResourceField>

          <ResourceField
            label="池内节点"
            hint={
              nodesInSelectedPool.length > 0
                ? '根据左侧节点池实时同步，只用于确认该池真实节点。'
                : '当前节点池里还没有真实节点。'
            }
            container="div"
          >
            <ResourceSelect
              aria-label="池内节点"
              value={selectedNodeID}
              disabled={nodePoolsLoading || nodesInSelectedPool.length === 0}
              onChange={(event) => setSelectedNodeID(event.target.value)}
            >
              {nodesInSelectedPool.length === 0 ? (
                <option value="">暂无节点</option>
              ) : (
                nodesInSelectedPool.map((node) => (
                  <option key={node.node_id || node.id} value={node.node_id}>
                    {formatNodeName(node)}
                  </option>
                ))
              )}
            </ResourceSelect>
          </ResourceField>
        </div>

        <ResourceField
          label="Origin Host Header"
          hint="留空时默认透传访问域名 $host。"
          error={form.formState.errors.origin_host?.message}
        >
          <ResourceInput
            placeholder="origin.example.internal"
            {...form.register('origin_host')}
          />
        </ResourceField>

        <ResourceField label="代理缓冲模式" hint={proxyBufferingModeHint}>
          <ResourceSelect
            aria-label="代理缓冲模式"
            {...form.register('proxy_buffering_mode')}
          >
            <option value="default">默认模式：开启代理缓冲</option>
            <option value="off">流媒体模式：关闭代理缓冲</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="自定义请求头"
          hint="每行一条，格式为 Key: Value。"
          error={form.formState.errors.custom_headers_text?.message}
        >
          <ResourceTextarea
            className="min-h-32"
            placeholder={'X-Trace-Id: $request_id\nX-Site: marketing'}
            {...form.register('custom_headers_text')}
          />
        </ResourceField>

        <ResourceField
          label="备注"
          error={form.formState.errors.remark?.message}
        >
          <ResourceTextarea
            placeholder="例如：多活回源，优先使用上海入口"
            {...form.register('remark')}
          />
        </ResourceField>
      </form>
    </ConfigSectionShell>
  );
}
