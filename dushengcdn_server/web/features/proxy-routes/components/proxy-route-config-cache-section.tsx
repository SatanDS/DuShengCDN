'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Minus, Plus } from 'lucide-react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { InlineMessage } from '@/components/feedback/inline-message';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import {
  purgeProxyRouteCache,
  warmProxyRouteCache,
} from '@/features/proxy-routes/api/proxy-routes';
import {
  buildPayloadFromRoute,
  getErrorMessage,
  validateCacheRules,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import { getOptions } from '@/features/settings/api/settings';
import {
  ResourceField,
  ResourceInput,
  ResourceSelect,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  type ConfigSectionPresentationProps,
  type FeedbackState,
  type SaveHandler,
} from './proxy-route-config-shared';

function optionValueEnabled(value: string | undefined) {
  return ['1', 'true', 'yes', 'on'].includes(
    (value ?? '').trim().toLowerCase(),
  );
}

const cacheSchema = z
  .object({
    cache_enabled: z.boolean(),
    cache_policy: z.enum([
      'url',
      'suffix',
      'path_prefix',
      'path_contains',
      'path_contains_all',
      'path_exact',
    ]),
    cache_rules: z.array(z.string()),
  })
  .superRefine((value, context) => {
    if (!value.cache_enabled) {
      return;
    }

    const rules = value.cache_rules.map((item) => item.trim()).filter(Boolean);
    const error = validateCacheRules(value.cache_policy, rules);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['cache_rules'],
        message: error,
      });
    }
  });

type CacheValues = z.infer<typeof cacheSchema>;

export function CacheSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-cache-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<FeedbackState>();
  const confirmDialog = useConfirmDialog();
  const optionsQuery = useQuery({
    queryKey: ['settings', 'options'],
    queryFn: getOptions,
    staleTime: 60_000,
  });
  const form = useForm<CacheValues>({
    resolver: zodResolver(cacheSchema),
    defaultValues: {
      cache_enabled: route.cache_enabled,
      cache_policy: (route.cache_policy ||
        'url') as CacheValues['cache_policy'],
      cache_rules:
        route.cache_rule_list.length > 0 ? route.cache_rule_list : [''],
    },
  });

  useEffect(() => {
    form.reset({
      cache_enabled: route.cache_enabled,
      cache_policy: (route.cache_policy ||
        'url') as CacheValues['cache_policy'],
      cache_rules:
        route.cache_rule_list.length > 0 ? route.cache_rule_list : [''],
    });
  }, [form, route]);

  const watchedEnabled = form.watch('cache_enabled');
  const watchedPolicy = form.watch('cache_policy');
  const watchedRules = form.watch('cache_rules');
  const globalCacheEnabled = optionValueEnabled(
    optionsQuery.data?.find((item) => item.key === 'OpenRestyCacheEnabled')
      ?.value,
  );
  const cacheRulesError = form.formState.errors.cache_rules;
  const cacheRulePlaceholder =
    watchedPolicy === 'suffix'
      ? 'jpg'
      : watchedPolicy === 'path_prefix'
        ? '/assets'
        : watchedPolicy === 'path_contains'
          ? '/Images'
          : watchedPolicy === 'path_contains_all'
            ? '/emby/Items/'
            : watchedPolicy === 'path_exact'
              ? '/robots.txt'
              : '按 URL 缓存时无需额外规则';
  const purgeMutation = useMutation({
    mutationFn: () => purgeProxyRouteCache(route.id, { scope: 'all' }),
    onSuccess: async (result) => {
      setFeedback({
        tone: 'success',
        message: `已下发缓存清理到 ${result.target_nodes.length} 个节点。`,
      });
      await queryClient.invalidateQueries({
        queryKey: ['proxy-routes', 'detail', route.id],
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });
  const warmMutation = useMutation({
    mutationFn: () =>
      warmProxyRouteCache(route.id, {
        scope: 'url',
        urls: route.domains.map(
          (domain) => `${route.enable_https ? 'https' : 'http'}://${domain}/`,
        ),
      }),
    onSuccess: (result) => {
      setFeedback({
        tone: 'success',
        message: `已下发首页预热到 ${result.target_nodes.length} 个节点。`,
      });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const handlePurgeCache = async () => {
    const confirmed = await confirmDialog({
      title: '清理全部缓存',
      message: `确认清理站点“${route.site_name}”的全部缓存吗？该操作会下发到目标节点，短时间内可能增加回源压力。`,
      confirmLabel: '清理缓存',
      tone: 'danger',
    });

    if (!confirmed) {
      return;
    }

    purgeMutation.mutate();
  };

  return (
    <ConfigSectionShell
      title="缓存"
      description="保留现有安全绕过逻辑，只对当前站点生效。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const rules = values.cache_rules
            .map((item) => item.trim())
            .filter(Boolean);
          onSave(
            buildPayloadFromRoute(route, {
              cache_enabled: values.cache_enabled,
              cache_policy: values.cache_enabled ? values.cache_policy : 'url',
              cache_rules:
                values.cache_enabled && values.cache_policy !== 'url'
                  ? rules
                  : [],
            }),
            { message: '缓存设置已保存。' },
          );
        })}
      >
        <ToggleField
          label="启用站点缓存"
          description="系统仍会自动绕过非 GET、带 Authorization 或常见登录态 Cookie 的请求。"
          checked={watchedEnabled}
          onChange={(checked) =>
            form.setValue('cache_enabled', checked, { shouldDirty: true })
          }
        />
        {watchedEnabled && optionsQuery.data && !globalCacheEnabled ? (
          <InlineMessage
            tone="warning"
            message="当前站点已开启缓存，但代理服务配置里的全局缓存基础设施未开启；发布后 OpenResty 不会生成缓存区，缓存命中率会一直没有数据。请到「代理服务配置 / 性能参数」启用缓存基础设施并重新发布配置。"
          />
        ) : null}

        <ResourceField label="缓存策略">
          <ResourceSelect
            disabled={!watchedEnabled}
            {...form.register('cache_policy')}
          >
            <option value="url">按 URL 缓存</option>
            <option value="suffix">按后缀缓存</option>
            <option value="path_prefix">按路径前缀缓存</option>
            <option value="path_contains">按路径包含缓存</option>
            <option value="path_contains_all">按路径多片段缓存</option>
            <option value="path_exact">按精确路径缓存</option>
          </ResourceSelect>
        </ResourceField>

        <ResourceField
          label="缓存规则"
          error={cacheRulesError?.message}
          hint={
            watchedPolicy === 'suffix'
              ? '每条填写一个后缀，例如 jpg、css、js。'
              : watchedPolicy === 'path_prefix'
                ? '每条填写一个路径前缀，例如 /assets、/static。'
                : watchedPolicy === 'path_contains'
                  ? '每条填写一个会出现在路径中的片段，例如 /Images、/thumb；/Images 会匹配 /emby/Items/12039/Images，数字变化不影响。'
                  : watchedPolicy === 'path_contains_all'
                    ? '每条规则都必须同时出现在路径中，例如填写 /emby/Items/ 和 /Images，可匹配 /emby/Items/12039/Images。'
                    : watchedPolicy === 'path_exact'
                      ? '每条填写一个精确路径，例如 /robots.txt。'
                      : '按 URL 缓存时无需额外规则。'
          }
        >
          <div className="space-y-3">
            {(watchedRules.length > 0 ? watchedRules : ['']).map(
              (_rule, index) => (
                <div key={index} className="flex items-center gap-2">
                  <ResourceInput
                    aria-label={`缓存规则 ${index + 1}`}
                    disabled={!watchedEnabled || watchedPolicy === 'url'}
                    placeholder={cacheRulePlaceholder}
                    {...form.register(`cache_rules.${index}`)}
                  />
                  <button
                    type="button"
                    aria-label={`删除缓存规则 ${index + 1}`}
                    title="删除规则"
                    disabled={!watchedEnabled || watchedPolicy === 'url'}
                    onClick={() => {
                      const nextRules = [
                        ...(form.getValues('cache_rules') ?? []),
                      ];
                      nextRules.splice(index, 1);
                      form.setValue(
                        'cache_rules',
                        nextRules.length > 0 ? nextRules : [''],
                        {
                          shouldDirty: true,
                          shouldValidate: true,
                        },
                      );
                    }}
                    className="grid h-10 w-10 shrink-0 place-items-center rounded-full border border-[var(--border-default)] bg-[var(--surface-elevated)] text-[var(--foreground-secondary)] transition hover:border-[var(--border-strong)] disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    <Minus className="h-4 w-4" aria-hidden="true" />
                  </button>
                </div>
              ),
            )}
            <button
              type="button"
              aria-label="添加缓存规则"
              title="添加规则"
              disabled={!watchedEnabled || watchedPolicy === 'url'}
              onClick={() => {
                const currentRules = form.getValues('cache_rules') ?? [];
                form.setValue('cache_rules', [...currentRules, ''], {
                  shouldDirty: true,
                  shouldValidate: true,
                });
              }}
              className="grid h-10 w-10 place-items-center rounded-full border border-[var(--border-default)] bg-[var(--surface-elevated)] text-[var(--foreground-secondary)] transition hover:border-[var(--status-info-border)] hover:text-[var(--status-info-foreground)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </ResourceField>

        <div className="flex flex-wrap gap-3">
          <SecondaryButton
            type="button"
            disabled={purgeMutation.isPending}
            onClick={() => void handlePurgeCache()}
          >
            {purgeMutation.isPending ? '清理中...' : '清理全部缓存'}
          </SecondaryButton>
          <SecondaryButton
            type="button"
            disabled={warmMutation.isPending || route.domains.length === 0}
            onClick={() => warmMutation.mutate()}
          >
            {warmMutation.isPending ? '预热中...' : '预热站点首页'}
          </SecondaryButton>
        </div>
      </form>
    </ConfigSectionShell>
  );
}
