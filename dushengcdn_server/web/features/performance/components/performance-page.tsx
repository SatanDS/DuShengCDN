'use client';

import {useEffect, useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';

import {EmptyState} from '@/components/feedback/empty-state';
import {ErrorState} from '@/components/feedback/error-state';
import {LoadingState} from '@/components/feedback/loading-state';
import {useToastFeedback} from '@/components/feedback/toast-provider';
import {PageHeader} from '@/components/layout/page-header';
import {useAuth} from '@/components/providers/auth-provider';
import {AppCard} from '@/components/ui/app-card';
import {StatusBadge} from '@/components/ui/status-badge';
import {getConfigVersionPreview} from '@/features/config-versions/api/config-versions';
import {getOptions, updateOptions} from '@/features/settings/api/settings';
import type {OptionItem} from '@/features/settings/types';
import {
    CodeBlock,
    PrimaryButton,
    ResourceField,
    ResourceInput,
    ResourceSelect,
    ResourceTextarea,
    SecondaryButton,
    ToggleField,
} from '@/features/shared/components/resource-primitives';

const settingsQueryKey = ['settings', 'options'] as const;
const previewQueryKey = ['performance', 'preview'] as const;
const templatePanelClassName =
    'h-[560px] w-full box-border overflow-auto rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-3 font-mono text-xs leading-6';

const defaultPerformanceFields = {
    OpenRestyWorkerProcesses: 'auto',
    OpenRestyWorkerConnections: '4096',
    OpenRestyWorkerRlimitNofile: '65535',
    OpenRestyEventsUse: 'epoll',
    OpenRestyEventsMultiAcceptEnabled: true,
    OpenRestyKeepaliveTimeout: '20',
    OpenRestyKeepaliveRequests: '1000',
    OpenRestyClientHeaderTimeout: '15',
    OpenRestyClientBodyTimeout: '15',
    OpenRestyClientMaxBodySize: '64m',
    OpenRestyLargeClientHeaderBuffers: '4 16k',
    OpenRestySendTimeout: '30',
    OpenRestyProxyConnectTimeout: '3',
    OpenRestyProxySendTimeout: '60',
    OpenRestyProxyReadTimeout: '60',
    OpenRestyWebsocketEnabled: true,
    OpenRestyProxyRequestBufferingEnabled: false,
    OpenRestyProxyBufferingEnabled: true,
    OpenRestyProxyBuffers: '16 16k',
    OpenRestyProxyBufferSize: '8k',
    OpenRestyProxyBusyBuffersSize: '64k',
    OpenRestyGzipEnabled: true,
    OpenRestyGzipMinLength: '1024',
    OpenRestyGzipCompLevel: '5',
    OpenRestyCacheEnabled: false,
    OpenRestyCachePath: '',
    OpenRestyCacheLevels: '1:2',
    OpenRestyCacheInactive: '30m',
    OpenRestyCacheMaxSize: '1g',
    OpenRestyCacheKeyTemplate: '$scheme$host$request_uri',
    OpenRestyCacheLockEnabled: true,
    OpenRestyCacheLockTimeout: '5s',
    OpenRestyCacheUseStale:
        'error timeout updating http_500 http_502 http_503 http_504',
    OpenRestyResolvers: '',
};

const performanceFieldTooltips: Record<string, string> = {
    worker_processes:
        '底层指令：worker_processes。表示代理服务启动多少个工作进程；通常保持 auto，让系统按处理器核心数自动分配。',
    worker_connections:
        '底层指令：worker_connections。表示每个工作进程可同时处理的最大连接数；值越大，可承载的并发连接越多。',
    worker_rlimit_nofile:
        '底层指令：worker_rlimit_nofile。提升工作进程可打开的文件句柄上限，避免高并发下连接或文件句柄不足。',
    events_use:
        '底层指令：events use。指定事件驱动模型；默认使用 epoll，Linux 高并发场景通常优先选择它。',
    multi_accept:
        '底层指令：multi_accept。默认开启后，工作进程会尽可能一次接受多个新连接，适合高吞吐接入场景。',
    keepalive_timeout: '底层指令：keepalive_timeout。客户端长连接空闲保持时间，单位秒。',
    keepalive_requests: '底层指令：keepalive_requests。单个长连接允许复用的最大请求数。',
    client_header_timeout: '底层指令：client_header_timeout。读取客户端请求头的超时时间，单位秒。',
    client_body_timeout: '底层指令：client_body_timeout。读取客户端请求体的超时时间，单位秒。',
    client_max_body_size:
        '底层指令：client_max_body_size。限制客户端请求体大小，常用于上传文件大小控制，例如 64m、128m。',
    large_client_header_buffers:
        '底层指令：large_client_header_buffers。控制大请求头使用的缓冲区数量和大小，例如 4 16k。',
    send_timeout: '底层指令：send_timeout。向客户端发送响应时的超时时间，单位秒。',
    proxy_connect_timeout: '底层指令：proxy_connect_timeout。连接源站的超时时间，单位秒。',
    proxy_send_timeout: '底层指令：proxy_send_timeout。向源站发送请求的超时时间，单位秒。',
    proxy_read_timeout: '底层指令：proxy_read_timeout。等待源站返回响应的超时时间，单位秒。',
    websocket:
        '控制是否为反向代理规则自动注入 WebSocket 升级所需的 HTTP/1.1、Upgrade 和 Connection 头。',
    proxy_request_buffering:
        '底层指令：proxy_request_buffering。控制请求体是否先在代理服务侧缓冲后再转发给源站，上传和流式场景经常会用到。',
    proxy_buffering:
        '底层指令：proxy_buffering。控制是否启用代理响应缓冲。开启后通常有更平滑的吞吐，但会增加内存占用。',
    proxy_buffers: '底层指令：proxy_buffers。设置代理响应缓冲区的数量和大小，例如 16 16k。',
    proxy_buffer_size: '底层指令：proxy_buffer_size。保存响应头等小块数据的基础缓冲区大小。',
    proxy_busy_buffers_size: '底层指令：proxy_busy_buffers_size。限制忙碌状态下可同时占用的缓冲区总大小。',
    gzip: '底层指令：gzip。控制是否启用 gzip 压缩响应。',
    gzip_min_length:
        '底层指令：gzip_min_length。只有响应体超过该字节数时才会启用 gzip，避免对极小响应做无意义压缩。',
    gzip_comp_level: '底层指令：gzip_comp_level。gzip 压缩等级，1 更省处理器，9 压缩更高但更耗处理器。',
    proxy_cache_path:
        '底层指令：proxy_cache_path。缓存文件保存在节点磁盘上的位置；是否缓存某条路径仍由网站配置里的缓存策略决定。',
    levels:
        '底层参数：levels。只控制缓存文件在磁盘里按哈希拆几层目录，例如 1:2；一般保持默认，不是路径匹配规则。',
    inactive: '底层参数：inactive。缓存对象在未被访问时的失活时间，例如 30m。',
    max_size:
        '底层参数：max_size。缓存目录允许占用的最大磁盘空间，会渲染到 proxy_cache_path。',
    cache_key_template:
        '底层指令：proxy_cache_key。这里只决定同一个请求如何生成缓存对象唯一标识；哪些路径进入缓存，请在网站配置的缓存策略里设置。',
    proxy_cache_lock:
        '底层指令：proxy_cache_lock。启用后，同一缓存 Key 未命中时只允许一个请求回源，减少击穿。',
    proxy_cache_lock_timeout: '底层指令：proxy_cache_lock_timeout。等待缓存锁的最长时间，例如 5s。',
    proxy_cache_use_stale:
        '底层指令：proxy_cache_use_stale。源站异常时允许返回旧缓存的条件列表，例如 error、timeout、http_500。',
    resolvers:
        '底层指令：resolver。自定义解析服务器，留空表示不配置。若源站需要动态解析，可在这里填写例如 1.1.1.1 8.8.8.8。',
};

type PerformanceTab = 'settings' | 'editor';

type FeedbackState = {
    tone: 'info' | 'success' | 'danger';
    message: string;
};

function getErrorMessage(error: unknown) {
    return error instanceof Error ? error.message : '请求失败，请稍后重试。';
}

function optionsToMap(options: OptionItem[] | undefined) {
    return (options ?? []).reduce<Record<string, string>>(
        (accumulator, option) => {
            accumulator[option.key] = option.value;
            return accumulator;
        },
        {},
    );
}

function toBoolean(value: string | undefined, fallback: boolean) {
    if (value === undefined) {
        return fallback;
    }

    return value === 'true';
}

function isPositiveInteger(value: string) {
    const parsed = Number.parseInt(value, 10);
    return !Number.isNaN(parsed) && parsed > 0;
}

function isSizeValue(value: string) {
    return /^\d+[kKmMgG]?$/.test(value.trim());
}

function isProxyBuffersValue(value: string) {
    return /^\d+\s+\d+[kKmMgG]?$/.test(value.trim());
}

function isDurationToken(value: string) {
    return /^\d+[smhdwSMHDW]$/.test(value.trim());
}

function isCacheLevelsValue(value: string) {
    return /^\d{1,2}(?::\d{1,2}){0,2}$/.test(value.trim());
}

export function PerformancePage() {
    const queryClient = useQueryClient();
    const {user} = useAuth();
    const [activeTab, setActiveTab] = useState<PerformanceTab>('settings');
    const {setFeedback} = useToastFeedback<FeedbackState>();
    const [busyKey, setBusyKey] = useState<string | null>(null);
    const [performanceFields, setPerformanceFields] = useState(
        defaultPerformanceFields,
    );
    const [templateContent, setTemplateContent] = useState('');

    const isRoot = (user?.role ?? 0) >= 100;

    const optionsQuery = useQuery({
        queryKey: settingsQueryKey,
        queryFn: getOptions,
        enabled: isRoot,
    });

    const previewQuery = useQuery({
        queryKey: previewQueryKey,
        queryFn: getConfigVersionPreview,
        enabled: isRoot,
    });

    useEffect(() => {
        if (!optionsQuery.data) {
            return;
        }

        const optionMap = optionsToMap(optionsQuery.data);
        setPerformanceFields({
            OpenRestyWorkerProcesses: optionMap.OpenRestyWorkerProcesses ?? 'auto',
            OpenRestyWorkerConnections:
                optionMap.OpenRestyWorkerConnections ?? '4096',
            OpenRestyWorkerRlimitNofile:
                optionMap.OpenRestyWorkerRlimitNofile ?? '65535',
            OpenRestyEventsUse: optionMap.OpenRestyEventsUse ?? 'epoll',
            OpenRestyEventsMultiAcceptEnabled: toBoolean(
                optionMap.OpenRestyEventsMultiAcceptEnabled,
                true,
            ),
            OpenRestyKeepaliveTimeout: optionMap.OpenRestyKeepaliveTimeout ?? '20',
            OpenRestyKeepaliveRequests:
                optionMap.OpenRestyKeepaliveRequests ?? '1000',
            OpenRestyClientHeaderTimeout:
                optionMap.OpenRestyClientHeaderTimeout ?? '15',
            OpenRestyClientBodyTimeout: optionMap.OpenRestyClientBodyTimeout ?? '15',
            OpenRestyClientMaxBodySize: optionMap.OpenRestyClientMaxBodySize ?? '64m',
            OpenRestyLargeClientHeaderBuffers:
                optionMap.OpenRestyLargeClientHeaderBuffers ?? '4 16k',
            OpenRestySendTimeout: optionMap.OpenRestySendTimeout ?? '30',
            OpenRestyProxyConnectTimeout:
                optionMap.OpenRestyProxyConnectTimeout ?? '3',
            OpenRestyProxySendTimeout: optionMap.OpenRestyProxySendTimeout ?? '60',
            OpenRestyProxyReadTimeout: optionMap.OpenRestyProxyReadTimeout ?? '60',
            OpenRestyWebsocketEnabled: toBoolean(
                optionMap.OpenRestyWebsocketEnabled,
                true,
            ),
            OpenRestyProxyRequestBufferingEnabled: toBoolean(
                optionMap.OpenRestyProxyRequestBufferingEnabled,
                false,
            ),
            OpenRestyProxyBufferingEnabled: toBoolean(
                optionMap.OpenRestyProxyBufferingEnabled,
                true,
            ),
            OpenRestyProxyBuffers: optionMap.OpenRestyProxyBuffers ?? '16 16k',
            OpenRestyProxyBufferSize: optionMap.OpenRestyProxyBufferSize ?? '8k',
            OpenRestyProxyBusyBuffersSize:
                optionMap.OpenRestyProxyBusyBuffersSize ?? '64k',
            OpenRestyGzipEnabled: toBoolean(optionMap.OpenRestyGzipEnabled, true),
            OpenRestyGzipMinLength: optionMap.OpenRestyGzipMinLength ?? '1024',
            OpenRestyGzipCompLevel: optionMap.OpenRestyGzipCompLevel ?? '5',
            OpenRestyCacheEnabled: toBoolean(optionMap.OpenRestyCacheEnabled, false),
            OpenRestyCachePath: optionMap.OpenRestyCachePath ?? '',
            OpenRestyCacheLevels: optionMap.OpenRestyCacheLevels ?? '1:2',
            OpenRestyCacheInactive: optionMap.OpenRestyCacheInactive ?? '30m',
            OpenRestyCacheMaxSize: optionMap.OpenRestyCacheMaxSize ?? '1g',
            OpenRestyCacheKeyTemplate:
                optionMap.OpenRestyCacheKeyTemplate ?? '$scheme$host$request_uri',
            OpenRestyCacheLockEnabled: toBoolean(
                optionMap.OpenRestyCacheLockEnabled,
                true,
            ),
            OpenRestyCacheLockTimeout: optionMap.OpenRestyCacheLockTimeout ?? '5s',
            OpenRestyCacheUseStale:
                optionMap.OpenRestyCacheUseStale ??
                'error timeout updating http_500 http_502 http_503 http_504',
            OpenRestyResolvers: optionMap.OpenRestyResolvers ?? '',
        });
        setTemplateContent(optionMap.OpenRestyMainConfigTemplate ?? '');
    }, [optionsQuery.data]);

    const runBusyAction = async (key: string, action: () => Promise<void>) => {
        setBusyKey(key);
        setFeedback(null);

        try {
            await action();
        } catch (error) {
            setFeedback({tone: 'danger', message: getErrorMessage(error)});
        } finally {
            setBusyKey(null);
        }
    };

    const saveOptionEntries = async (
        entries: Array<[string, string]>,
        successMessage: string,
    ) => {
        await updateOptions(entries.map(([key, value]) => ({key, value})));

        await Promise.all([
            queryClient.invalidateQueries({queryKey: settingsQueryKey}),
            queryClient.invalidateQueries({queryKey: previewQueryKey}),
            queryClient.invalidateQueries({queryKey: ['config-versions']}),
        ]);
        setFeedback({tone: 'success', message: successMessage});
    };

    const handleRuntimeSave = () => {
        void runBusyAction('performance-runtime', async () => {
            if (
                performanceFields.OpenRestyWorkerProcesses !== 'auto' &&
                !isPositiveInteger(performanceFields.OpenRestyWorkerProcesses)
            ) {
                throw new Error('worker_processes 必须为 auto 或大于 0 的整数。');
            }

            const integerFields = [
                ['worker_connections', performanceFields.OpenRestyWorkerConnections],
                ['worker_rlimit_nofile', performanceFields.OpenRestyWorkerRlimitNofile],
                ['keepalive_timeout', performanceFields.OpenRestyKeepaliveTimeout],
                ['keepalive_requests', performanceFields.OpenRestyKeepaliveRequests],
                [
                    'client_header_timeout',
                    performanceFields.OpenRestyClientHeaderTimeout,
                ],
                ['client_body_timeout', performanceFields.OpenRestyClientBodyTimeout],
                ['send_timeout', performanceFields.OpenRestySendTimeout],
            ] as const;

            for (const [label, value] of integerFields) {
                if (!isPositiveInteger(value)) {
                    throw new Error(`${label} 必须为大于 0 的整数。`);
                }
            }
            if (!isSizeValue(performanceFields.OpenRestyClientMaxBodySize)) {
                throw new Error(
                    'client_max_body_size 必须为整数或带 k/m/g 单位的大小值。',
                );
            }
            if (
                !isProxyBuffersValue(
                    performanceFields.OpenRestyLargeClientHeaderBuffers,
                )
            ) {
                throw new Error('large_client_header_buffers 格式必须类似 "4 16k"。');
            }

            await saveOptionEntries(
                [
                    [
                        'OpenRestyWorkerProcesses',
                        performanceFields.OpenRestyWorkerProcesses.trim(),
                    ],
                    [
                        'OpenRestyResolvers',
                        performanceFields.OpenRestyResolvers.trim(),
                    ],
                    [
                        'OpenRestyWorkerConnections',
                        performanceFields.OpenRestyWorkerConnections.trim(),
                    ],
                    [
                        'OpenRestyWorkerRlimitNofile',
                        performanceFields.OpenRestyWorkerRlimitNofile.trim(),
                    ],
                    ['OpenRestyEventsUse', performanceFields.OpenRestyEventsUse.trim()],
                    [
                        'OpenRestyEventsMultiAcceptEnabled',
                        String(performanceFields.OpenRestyEventsMultiAcceptEnabled),
                    ],
                    [
                        'OpenRestyKeepaliveTimeout',
                        performanceFields.OpenRestyKeepaliveTimeout.trim(),
                    ],
                    [
                        'OpenRestyKeepaliveRequests',
                        performanceFields.OpenRestyKeepaliveRequests.trim(),
                    ],
                    [
                        'OpenRestyClientHeaderTimeout',
                        performanceFields.OpenRestyClientHeaderTimeout.trim(),
                    ],
                    [
                        'OpenRestyClientBodyTimeout',
                        performanceFields.OpenRestyClientBodyTimeout.trim(),
                    ],
                    [
                        'OpenRestyClientMaxBodySize',
                        performanceFields.OpenRestyClientMaxBodySize.trim(),
                    ],
                    [
                        'OpenRestyLargeClientHeaderBuffers',
                        performanceFields.OpenRestyLargeClientHeaderBuffers.trim(),
                    ],
                    [
                        'OpenRestySendTimeout',
                        performanceFields.OpenRestySendTimeout.trim(),
                    ],
                ],
                '代理服务连接与事件参数已保存。',
            );
        });
    };

    const handleProxySave = () => {
        void runBusyAction('performance-proxy', async () => {
            const timeoutFields = [
                performanceFields.OpenRestyProxyConnectTimeout,
                performanceFields.OpenRestyProxySendTimeout,
                performanceFields.OpenRestyProxyReadTimeout,
            ];

            for (const value of timeoutFields) {
                if (!isPositiveInteger(value)) {
                    throw new Error('代理超时参数必须为大于 0 的整数秒。');
                }
            }

            if (!isProxyBuffersValue(performanceFields.OpenRestyProxyBuffers)) {
                throw new Error('proxy_buffers 格式必须类似 "16 16k"。');
            }
            if (
                !isSizeValue(performanceFields.OpenRestyProxyBufferSize) ||
                !isSizeValue(performanceFields.OpenRestyProxyBusyBuffersSize)
            ) {
                throw new Error('缓冲大小必须为整数或带 k/m/g 单位的值。');
            }

            await saveOptionEntries(
                [
                    [
                        'OpenRestyProxyConnectTimeout',
                        performanceFields.OpenRestyProxyConnectTimeout.trim(),
                    ],
                    [
                        'OpenRestyProxySendTimeout',
                        performanceFields.OpenRestyProxySendTimeout.trim(),
                    ],
                    [
                        'OpenRestyProxyReadTimeout',
                        performanceFields.OpenRestyProxyReadTimeout.trim(),
                    ],
                    [
                        'OpenRestyWebsocketEnabled',
                        String(performanceFields.OpenRestyWebsocketEnabled),
                    ],
                    [
                        'OpenRestyProxyRequestBufferingEnabled',
                        String(performanceFields.OpenRestyProxyRequestBufferingEnabled),
                    ],
                    [
                        'OpenRestyProxyBufferingEnabled',
                        String(performanceFields.OpenRestyProxyBufferingEnabled),
                    ],
                    [
                        'OpenRestyProxyBuffers',
                        performanceFields.OpenRestyProxyBuffers.trim(),
                    ],
                    [
                        'OpenRestyProxyBufferSize',
                        performanceFields.OpenRestyProxyBufferSize.trim(),
                    ],
                    [
                        'OpenRestyProxyBusyBuffersSize',
                        performanceFields.OpenRestyProxyBusyBuffersSize.trim(),
                    ],
                ],
                'OpenResty 反代缓冲参数已保存。',
            );
        });
    };

    const handleCacheSave = () => {
        void runBusyAction('performance-cache', async () => {
            if (performanceFields.OpenRestyCacheEnabled) {
                if (!performanceFields.OpenRestyCachePath.trim()) {
                    throw new Error('启用缓存时必须填写节点缓存目录。');
                }
                if (
                    !isCacheLevelsValue(performanceFields.OpenRestyCacheLevels) ||
                    !isDurationToken(performanceFields.OpenRestyCacheInactive) ||
                    !isSizeValue(performanceFields.OpenRestyCacheMaxSize) ||
                    !isDurationToken(performanceFields.OpenRestyCacheLockTimeout)
                ) {
                    throw new Error(
                        '缓存文件分层、失活时间、磁盘上限或等待时间格式不合法。',
                    );
                }
                if (!performanceFields.OpenRestyCacheKeyTemplate.trim()) {
                    throw new Error('启用缓存时必须填写缓存 Key。');
                }
            }

            await saveOptionEntries(
                [
                    ['OpenRestyCachePath', performanceFields.OpenRestyCachePath.trim()],
                    [
                        'OpenRestyCacheLevels',
                        performanceFields.OpenRestyCacheLevels.trim(),
                    ],
                    [
                        'OpenRestyCacheInactive',
                        performanceFields.OpenRestyCacheInactive.trim(),
                    ],
                    [
                        'OpenRestyCacheMaxSize',
                        performanceFields.OpenRestyCacheMaxSize.trim(),
                    ],
                    [
                        'OpenRestyCacheKeyTemplate',
                        performanceFields.OpenRestyCacheKeyTemplate.trim(),
                    ],
                    [
                        'OpenRestyCacheLockEnabled',
                        String(performanceFields.OpenRestyCacheLockEnabled),
                    ],
                    [
                        'OpenRestyCacheLockTimeout',
                        performanceFields.OpenRestyCacheLockTimeout.trim(),
                    ],
                    [
                        'OpenRestyCacheUseStale',
                        performanceFields.OpenRestyCacheUseStale.trim(),
                    ],
                    [
                        'OpenRestyCacheEnabled',
                        String(performanceFields.OpenRestyCacheEnabled),
                    ],
                ],
                '代理服务压缩与缓存参数已保存。',
            );
        });
    };

    const handleGzipSave = () => {
        void runBusyAction('performance-gzip', async () => {
            if (!isPositiveInteger(performanceFields.OpenRestyGzipMinLength)) {
                throw new Error('gzip_min_length 必须为大于 0 的整数。');
            }
            const gzipLevel = Number.parseInt(
                performanceFields.OpenRestyGzipCompLevel,
                10,
            );
            if (Number.isNaN(gzipLevel) || gzipLevel < 1 || gzipLevel > 9) {
                throw new Error('gzip_comp_level 必须在 1 到 9 之间。');
            }

            await saveOptionEntries(
                [
                    [
                        'OpenRestyGzipEnabled',
                        String(performanceFields.OpenRestyGzipEnabled),
                    ],
                    [
                        'OpenRestyGzipMinLength',
                        performanceFields.OpenRestyGzipMinLength.trim(),
                    ],
                    [
                        'OpenRestyGzipCompLevel',
                        performanceFields.OpenRestyGzipCompLevel.trim(),
                    ],
                ],
                '代理服务压缩参数已保存。',
            );
        });
    };

    const handleTemplateSave = () => {
        void runBusyAction('performance-template', async () => {
            await saveOptionEntries(
                [['OpenRestyMainConfigTemplate', templateContent]],
                '代理服务主配置模板已保存。',
            );
        });
    };

    if (!isRoot) {
        return (
            <div className="space-y-6">
                <PageHeader
                    title="代理服务配置"
                    description="集中管理代理服务性能参数和主配置模板。"
                />
                <EmptyState
                    title="权限不足"
                    description="只有超级管理员可以访问代理服务配置。"
                />
            </div>
        );
    }

    if (optionsQuery.isLoading || previewQuery.isLoading) {
        return <LoadingState/>;
    }

    if (optionsQuery.isError) {
        return (
            <ErrorState
                title="代理服务配置加载失败"
                description={getErrorMessage(optionsQuery.error)}
            />
        );
    }

    if (previewQuery.isError) {
        return (
            <ErrorState
                title="代理服务配置预览加载失败"
                description={getErrorMessage(previewQuery.error)}
            />
        );
    }

    const preview = previewQuery.data;
    if (!preview) {
        return (
            <EmptyState
                title="代理服务配置预览不可用"
                description="当前未获取到代理服务配置预览。"
            />
        );
    }

    return (
        <div className="space-y-6">
            <PageHeader
                title="代理服务配置"
                description="统一维护代理服务性能参数和主配置模板。"
            />

            <div className="flex flex-wrap gap-3">
                {[
                    {
                        key: 'settings' as const,
                        label: '设置',
                        description: '维护连接、缓冲、压缩与缓存参数。',
                    },
                    {
                        key: 'editor' as const,
                        label: '编辑',
                        description:
                            '编辑主配置模板并查看当前渲染结果；底层文件仍是 nginx.conf。',
                    },
                ].map((tab) => (
                    <button
                        key={tab.key}
                        type="button"
                        onClick={() => setActiveTab(tab.key)}
                        className={[
                            'rounded-2xl border px-4 py-3 text-left transition',
                            activeTab === tab.key
                                ? 'border-[var(--border-strong)] bg-[var(--accent-soft)] text-[var(--foreground-primary)]'
                                : 'border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--border-strong)] hover:text-[var(--foreground-primary)]',
                        ].join(' ')}
                    >
                        <p className="text-sm font-semibold">{tab.label}</p>
                        <p className="mt-1 text-xs leading-5 text-inherit/80">
                            {tab.description}
                        </p>
                    </button>
                ))}
            </div>

            {activeTab === 'settings' ? (
                <div className="space-y-6">
                    <div className="grid gap-4 md:grid-cols-3">
                        <div
                            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                            <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                                主配置校验
                            </p>
                            <div className="mt-2">
                                <StatusBadge label="发布链路受管" variant="info"/>
                            </div>
                        </div>
                        <div
                            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                            <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                                当前预览规则数
                            </p>
                            <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                                {preview.route_count} 条
                            </p>
                        </div>
                        <div
                            className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                            <p className="text-xs tracking-[0.2em] text-[var(--foreground-muted)] uppercase">
                                当前模板
                            </p>
                            <p className="mt-2 text-sm text-[var(--foreground-primary)]">
                                受控模板 + 占位符渲染
                            </p>
                        </div>
                    </div>

                    <AppCard
                        title="代理服务连接与事件"
                        action={
                            <PrimaryButton
                                type="button"
                                onClick={handleRuntimeSave}
                                disabled={busyKey === 'performance-runtime'}
                            >
                                {busyKey === 'performance-runtime'
                                    ? '保存中...'
                                    : '保存运行调优'}
                            </PrimaryButton>
                        }
                    >
                        <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
                            <ResourceField
                                label="自定义解析服务器"
                                hint="例如：1.1.1.1 8.8.8.8"
                                tooltip={performanceFieldTooltips.resolvers}
                            >
                                <ResourceInput
                                    value={performanceFields.OpenRestyResolvers}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyResolvers: event.target.value,
                                        }))
                                    }
                                    placeholder="留空表示不配置"
                                />
                            </ResourceField>
                            <ResourceField
                                label="工作进程数量"
                                tooltip={performanceFieldTooltips.worker_processes}
                            >
                                <ResourceInput
                                    value={performanceFields.OpenRestyWorkerProcesses}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyWorkerProcesses: event.target.value,
                                        }))
                                    }
                                    placeholder="auto"
                                />
                            </ResourceField>
                            <ResourceField
                                label="单进程最大连接数"
                                tooltip={performanceFieldTooltips.worker_connections}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyWorkerConnections}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyWorkerConnections: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="文件句柄上限"
                                tooltip={performanceFieldTooltips.worker_rlimit_nofile}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyWorkerRlimitNofile}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyWorkerRlimitNofile: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="事件模型"
                                hint="留空表示不显式渲染。"
                                tooltip={performanceFieldTooltips.events_use}
                            >
                                <ResourceSelect
                                    value={performanceFields.OpenRestyEventsUse}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyEventsUse: event.target.value,
                                        }))
                                    }
                                >
                                    <option value="">默认</option>
                                    <option value="epoll">epoll</option>
                                    <option value="kqueue">kqueue</option>
                                    <option value="poll">poll</option>
                                    <option value="select">select</option>
                                </ResourceSelect>
                            </ResourceField>
                            <ToggleField
                                label="一次接受多个连接"
                                tooltip={performanceFieldTooltips.multi_accept}
                                checked={performanceFields.OpenRestyEventsMultiAcceptEnabled}
                                onChange={(checked) =>
                                    setPerformanceFields((previous) => ({
                                        ...previous,
                                        OpenRestyEventsMultiAcceptEnabled: checked,
                                    }))
                                }
                            />
                            <ResourceField
                                label="长连接空闲时间（秒）"
                                tooltip={performanceFieldTooltips.keepalive_timeout}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyKeepaliveTimeout}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyKeepaliveTimeout: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="长连接复用次数"
                                tooltip={performanceFieldTooltips.keepalive_requests}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyKeepaliveRequests}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyKeepaliveRequests: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="请求头读取超时（秒）"
                                tooltip={performanceFieldTooltips.client_header_timeout}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyClientHeaderTimeout}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyClientHeaderTimeout: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="请求体读取超时（秒）"
                                tooltip={performanceFieldTooltips.client_body_timeout}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestyClientBodyTimeout}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyClientBodyTimeout: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                            <ResourceField
                                label="请求体大小上限"
                                tooltip={performanceFieldTooltips.client_max_body_size}
                            >
                                <ResourceInput
                                    value={performanceFields.OpenRestyClientMaxBodySize}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyClientMaxBodySize: event.target.value,
                                        }))
                                    }
                                    placeholder="64m"
                                />
                            </ResourceField>
                            <ResourceField
                                label="大请求头缓冲"
                                tooltip={performanceFieldTooltips.large_client_header_buffers}
                            >
                                <ResourceInput
                                    value={performanceFields.OpenRestyLargeClientHeaderBuffers}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyLargeClientHeaderBuffers: event.target.value,
                                        }))
                                    }
                                    placeholder="4 16k"
                                />
                            </ResourceField>
                            <ResourceField
                                label="响应发送超时（秒）"
                                tooltip={performanceFieldTooltips.send_timeout}
                            >
                                <ResourceInput
                                    type="number"
                                    value={performanceFields.OpenRestySendTimeout}
                                    onChange={(event) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestySendTimeout: event.target.value,
                                        }))
                                    }
                                />
                            </ResourceField>
                        </div>
                    </AppCard>

                    <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                        <AppCard
                            title="代理服务反代缓冲与超时"
                            description="用于控制 upstream 连接、发送、读取超时，以及常用代理缓冲参数。"
                            action={
                                <PrimaryButton
                                    type="button"
                                    onClick={handleProxySave}
                                    disabled={busyKey === 'performance-proxy'}
                                >
                                    {busyKey === 'performance-proxy'
                                        ? '保存中...'
                                        : '保存'}
                                </PrimaryButton>
                            }
                        >
                            <div className="grid gap-5 md:grid-cols-2">
                                <ResourceField
                                    label="源站连接超时（秒）"
                                    tooltip={performanceFieldTooltips.proxy_connect_timeout}
                                >
                                    <ResourceInput
                                        type="number"
                                        value={performanceFields.OpenRestyProxyConnectTimeout}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxyConnectTimeout: event.target.value,
                                            }))
                                        }
                                    />
                                </ResourceField>
                                <ResourceField
                                    label="发送到源站超时（秒）"
                                    tooltip={performanceFieldTooltips.proxy_send_timeout}
                                >
                                    <ResourceInput
                                        type="number"
                                        value={performanceFields.OpenRestyProxySendTimeout}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxySendTimeout: event.target.value,
                                            }))
                                        }
                                    />
                                </ResourceField>
                                <ResourceField
                                    label="等待源站响应超时（秒）"
                                    tooltip={performanceFieldTooltips.proxy_read_timeout}
                                >
                                    <ResourceInput
                                        type="number"
                                        value={performanceFields.OpenRestyProxyReadTimeout}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxyReadTimeout: event.target.value,
                                            }))
                                        }
                                    />
                                </ResourceField>
                                <ToggleField
                                    label="支持 WebSocket"
                                    tooltip={performanceFieldTooltips.websocket}
                                    checked={performanceFields.OpenRestyWebsocketEnabled}
                                    onChange={(checked) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyWebsocketEnabled: checked,
                                        }))
                                    }
                                />
                                <ToggleField
                                    label="先缓冲请求体"
                                    tooltip={performanceFieldTooltips.proxy_request_buffering}
                                    checked={
                                        performanceFields.OpenRestyProxyRequestBufferingEnabled
                                    }
                                    onChange={(checked) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyProxyRequestBufferingEnabled: checked,
                                        }))
                                    }
                                />
                                <ToggleField
                                    label="启用响应缓冲"
                                    tooltip={performanceFieldTooltips.proxy_buffering}
                                    checked={performanceFields.OpenRestyProxyBufferingEnabled}
                                    onChange={(checked) =>
                                        setPerformanceFields((previous) => ({
                                            ...previous,
                                            OpenRestyProxyBufferingEnabled: checked,
                                        }))
                                    }
                                />
                                <ResourceField
                                    label="响应缓冲数量和大小"
                                    tooltip={performanceFieldTooltips.proxy_buffers}
                                >
                                    <ResourceInput
                                        value={performanceFields.OpenRestyProxyBuffers}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxyBuffers: event.target.value,
                                            }))
                                        }
                                        placeholder="16 16k"
                                    />
                                </ResourceField>
                                <ResourceField
                                    label="响应头缓冲大小"
                                    tooltip={performanceFieldTooltips.proxy_buffer_size}
                                >
                                    <ResourceInput
                                        value={performanceFields.OpenRestyProxyBufferSize}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxyBufferSize: event.target.value,
                                            }))
                                        }
                                        placeholder="8k"
                                    />
                                </ResourceField>
                                <ResourceField
                                    label="忙碌缓冲上限"
                                    tooltip={performanceFieldTooltips.proxy_busy_buffers_size}
                                >
                                    <ResourceInput
                                        value={performanceFields.OpenRestyProxyBusyBuffersSize}
                                        onChange={(event) =>
                                            setPerformanceFields((previous) => ({
                                                ...previous,
                                                OpenRestyProxyBusyBuffersSize: event.target.value,
                                            }))
                                        }
                                        placeholder="64k"
                                    />
                                </ResourceField>
                            </div>
                        </AppCard>

                        <div className="grid content-start gap-6">
                            <AppCard
                                title="代理服务压缩"
                                action={
                                    <PrimaryButton
                                        type="button"
                                        onClick={handleGzipSave}
                                        disabled={busyKey === 'performance-gzip'}
                                    >
                                        {busyKey === 'performance-gzip'
                                            ? '保存中...'
                                            : '保存'}
                                    </PrimaryButton>
                                }
                            >
                                <div className="space-y-5">
                                    <div className="grid gap-5 md:grid-cols-2">
                                        <ToggleField
                                            label="启用响应压缩"
                                            tooltip={performanceFieldTooltips.gzip}
                                            checked={performanceFields.OpenRestyGzipEnabled}
                                            onChange={(checked) =>
                                                setPerformanceFields((previous) => ({
                                                    ...previous,
                                                    OpenRestyGzipEnabled: checked,
                                                }))
                                            }
                                        />
                                        <ResourceField
                                            label="压缩最小响应大小"
                                            tooltip={performanceFieldTooltips.gzip_min_length}
                                        >
                                            <ResourceInput
                                                type="number"
                                                value={performanceFields.OpenRestyGzipMinLength}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyGzipMinLength: event.target.value,
                                                    }))
                                                }
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="压缩等级"
                                            tooltip={performanceFieldTooltips.gzip_comp_level}
                                        >
                                            <ResourceInput
                                                type="number"
                                                min="1"
                                                max="9"
                                                value={performanceFields.OpenRestyGzipCompLevel}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyGzipCompLevel: event.target.value,
                                                    }))
                                                }
                                            />
                                        </ResourceField>
                                    </div>
                                </div>
                            </AppCard>

                            <AppCard
                                title="代理服务缓存"
                                description="缓存能力限定在单节点反代优化场景。"
                                action={
                                    <div className="flex flex-wrap gap-2">
                                        <SecondaryButton
                                            type="button"
                                            onClick={() =>
                                                setPerformanceFields((previous) => ({
                                                    ...previous,
                                                    OpenRestyCacheEnabled:
                                                        !previous.OpenRestyCacheEnabled,
                                                }))
                                            }
                                        >
                                            {performanceFields.OpenRestyCacheEnabled
                                                ? '关闭'
                                                : '启用'}
                                        </SecondaryButton>
                                        <PrimaryButton
                                            type="button"
                                            onClick={handleCacheSave}
                                            disabled={busyKey === 'performance-cache'}
                                        >
                                            {busyKey === 'performance-cache'
                                                ? '保存中...' : '保存'}
                                        </PrimaryButton>
                                    </div>
                                }
                            >
                                <div className="space-y-5">
                                    <div
                                        className={[
                                            'grid gap-5 transition md:grid-cols-2',
                                            performanceFields.OpenRestyCacheEnabled
                                                ? 'opacity-100'
                                                : 'opacity-60',
                                        ].join(' ')}
                                    >
                                        <ResourceField
                                            label="缓存目录"
                                            tooltip={performanceFieldTooltips.proxy_cache_path}
                                            hint={
                                                performanceFields.OpenRestyCacheEnabled
                                                    ? '节点本地磁盘目录；缓存关闭时不会写入。'
                                                    : '缓存关闭时暂不生效。'
                                            }
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCachePath}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCachePath: event.target.value,
                                                    }))
                                                }
                                                placeholder="/var/cache/openresty/dushengcdn"
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="缓存文件分层"
                                            tooltip={performanceFieldTooltips.levels}
                                            hint="默认 1:2；只影响缓存文件在磁盘里的存放层级，不影响哪些路径会缓存。"
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheLevels}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheLevels: event.target.value,
                                                    }))
                                                }
                                                placeholder="1:2"
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="缓存失活时间"
                                            tooltip={performanceFieldTooltips.inactive}
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheInactive}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheInactive: event.target.value,
                                                    }))
                                                }
                                                placeholder="30m"
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="缓存磁盘上限"
                                            tooltip={performanceFieldTooltips.max_size}
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheMaxSize}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheMaxSize: event.target.value,
                                                    }))
                                                }
                                                placeholder="1g"
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="缓存 Key（命中唯一标识）"
                                            tooltip={performanceFieldTooltips.cache_key_template}
                                            hint="决定同一个请求如何识别为同一份缓存；哪些路径缓存请到网站配置的缓存策略设置。"
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheKeyTemplate}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheKeyTemplate: event.target.value,
                                                    }))
                                                }
                                            />
                                        </ResourceField>
                                        <ToggleField
                                            label="防止缓存击穿"
                                            tooltip={performanceFieldTooltips.proxy_cache_lock}
                                            checked={performanceFields.OpenRestyCacheLockEnabled}
                                            disabled={!performanceFields.OpenRestyCacheEnabled}
                                            onChange={(checked) =>
                                                setPerformanceFields((previous) => ({
                                                    ...previous,
                                                    OpenRestyCacheLockEnabled: checked,
                                                }))
                                            }
                                        />
                                        <ResourceField
                                            label="缓存锁等待时间"
                                            tooltip={
                                                performanceFieldTooltips.proxy_cache_lock_timeout
                                            }
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheLockTimeout}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheLockTimeout: event.target.value,
                                                    }))
                                                }
                                                placeholder="5s"
                                            />
                                        </ResourceField>
                                        <ResourceField
                                            label="源站异常时使用旧缓存"
                                            tooltip={performanceFieldTooltips.proxy_cache_use_stale}
                                        >
                                            <ResourceInput
                                                value={performanceFields.OpenRestyCacheUseStale}
                                                disabled={!performanceFields.OpenRestyCacheEnabled}
                                                onChange={(event) =>
                                                    setPerformanceFields((previous) => ({
                                                        ...previous,
                                                        OpenRestyCacheUseStale: event.target.value,
                                                    }))
                                                }
                                            />
                                        </ResourceField>
                                    </div>
                                </div>
                            </AppCard>

                        </div>

                    </div>
                </div>
            ) : (
                <div className="space-y-6">
                    <AppCard
                        title="主配置模板编辑"
                        description="编辑 DuShengCDN 管理的代理服务主配置模板。系统占位符必须保留，保存后会进入统一发布链路；底层文件名仍是 nginx.conf。"
                        action={
                            <div className="flex flex-wrap gap-2">
                                <SecondaryButton
                                    type="button"
                                    onClick={() =>
                                        void queryClient.invalidateQueries({
                                            queryKey: previewQueryKey,
                                        })
                                    }
                                >
                                    刷新预览
                                </SecondaryButton>
                                <PrimaryButton
                                    type="button"
                                    onClick={handleTemplateSave}
                                    disabled={busyKey === 'performance-template'}
                                >
                                    {busyKey === 'performance-template'
                                        ? '保存中...'
                                        : '保存模板'}
                                </PrimaryButton>
                            </div>
                        }
                    >
                        <div className="grid gap-6 xl:grid-cols-2">
                            <div className="space-y-4">
                                <ResourceField
                                    label="模板原文"
                                    hint="请保留系统占位符，例如 {{OpenRestyRouteConfigInclude}} 和各项 OpenResty 参数占位符。"
                                >
                                    <ResourceTextarea
                                        value={templateContent}
                                        onChange={(event) => setTemplateContent(event.target.value)}
                                        spellCheck={false}
                                        className={`${templatePanelClassName} resize-none`}
                                        placeholder="请输入受控 nginx.conf 模板"
                                    />
                                </ResourceField>
                            </div>

                            <div className="space-y-4">
                                <div className="flex items-center justify-between gap-3">
                                    <div>
                                        <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                                            当前渲染预览
                                        </p>
                                    </div>
                                    <StatusBadge
                                        label={`${preview.route_count} 条规则`}
                                        variant="info"
                                    />
                                </div>
                                <CodeBlock
                                    className={`${templatePanelClassName} whitespace-pre-wrap break-words`}>
                                    {preview.main_config}
                                </CodeBlock>
                            </div>
                        </div>
                    </AppCard>
                </div>
            )}
        </div>
    );
}
