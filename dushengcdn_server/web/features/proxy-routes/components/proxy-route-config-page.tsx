'use client';

import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { LoadingState } from '@/components/feedback/loading-state';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { getDNSZones } from '@/features/authoritative-dns/api/authoritative-dns';
import { getDnsAccounts } from '@/features/dns-accounts/api/dns-accounts';
import { getManagedDomains } from '@/features/managed-domains/api/managed-domains';
import { getNodes } from '@/features/nodes/api/nodes';
import {
  getProxyRoute,
  updateProxyRoute,
} from '@/features/proxy-routes/api/proxy-routes';
import { buildNodePoolOptions } from '@/features/proxy-routes/components/node-pool-select';
import {
  getErrorMessage,
  getWebsiteConfigSection,
  websiteConfigSections,
} from '@/features/proxy-routes/helpers';
import { getTlsCertificates } from '@/features/tls-certificates/api/tls-certificates';
import { SecondaryButton } from '@/features/shared/components/resource-primitives';
import { cn } from '@/lib/utils/cn';

import { BasicAuthSection } from './proxy-route-config-auth-section';
import { CacheSection } from './proxy-route-config-cache-section';
import { DNSAutomationSection } from './proxy-route-config-dns-section';
import { DomainSettingsSection } from './proxy-route-config-domain-section';
import { ReverseProxySection } from './proxy-route-config-proxy-section';
import { RegionRestrictionSection } from './proxy-route-config-region-section';
import { CCProtectionSection } from './proxy-route-config-security-section';
import type { FeedbackState, SaveContext } from './proxy-route-config-shared';

export { BasicAuthSection } from './proxy-route-config-auth-section';
export { CacheSection } from './proxy-route-config-cache-section';
export { DNSAutomationSection } from './proxy-route-config-dns-section';
export { RegionRestrictionSection } from './proxy-route-config-region-section';
export { CCProtectionSection } from './proxy-route-config-security-section';
export type { SaveContext, SaveHandler } from './proxy-route-config-shared';

export function ProxyRouteConfigPage({
  routeId,
  initialSection,
}: {
  routeId: string;
  initialSection?: string;
}) {
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<FeedbackState>();

  const numericRouteID = Number(routeId);
  const currentSection = getWebsiteConfigSection(initialSection);

  const routeQuery = useQuery({
    queryKey: ['proxy-routes', 'detail', numericRouteID],
    queryFn: () => getProxyRoute(numericRouteID),
    enabled: Number.isFinite(numericRouteID) && numericRouteID > 0,
  });
  const certificatesQuery = useQuery({
    queryKey: ['tls-certificates', 'list'],
    queryFn: getTlsCertificates,
  });
  const dnsAccountsQuery = useQuery({
    queryKey: ['dns-accounts'],
    queryFn: getDnsAccounts,
  });
  const dnsZonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
    enabled: currentSection === 'dns',
  });
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: getNodes,
    enabled: currentSection === 'proxy' || currentSection === 'dns',
  });
  const managedDomainsQuery = useQuery({
    queryKey: ['managed-domains'],
    queryFn: getManagedDomains,
  });

  const saveMutation = useMutation({
    mutationFn: async ({
      payload,
      context,
    }: {
      payload: Parameters<typeof updateProxyRoute>[1];
      context: SaveContext;
    }) => {
      const updatedRoute = await updateProxyRoute(numericRouteID, payload);
      return { updatedRoute, context };
    },
    onSuccess: async ({ updatedRoute, context }) => {
      queryClient.setQueryData(
        ['proxy-routes', 'detail', numericRouteID],
        updatedRoute,
      );
      setFeedback({ tone: 'success', message: context.message });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['proxy-routes'] }),
        queryClient.invalidateQueries({
          queryKey: ['config-versions', 'diff'],
        }),
      ]);
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const route = routeQuery.data;
  const certificates = useMemo(
    () => certificatesQuery.data ?? [],
    [certificatesQuery.data],
  );
  const dnsAccounts = useMemo(
    () => dnsAccountsQuery.data ?? [],
    [dnsAccountsQuery.data],
  );
  const dnsZones = useMemo(
    () => dnsZonesQuery.data ?? [],
    [dnsZonesQuery.data],
  );
  const domainSuggestionSources = useMemo(
    () => [
      ...(route?.domains ?? []),
      ...(managedDomainsQuery.data?.map((item) => item.domain) ?? []),
    ],
    [managedDomainsQuery.data, route?.domains],
  );
  const nodePoolOptions = useMemo(
    () => buildNodePoolOptions(nodesQuery.data ?? []),
    [nodesQuery.data],
  );
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  if (!Number.isFinite(numericRouteID) || numericRouteID <= 0) {
    return (
      <EmptyState
        title="缺少站点 ID"
        description="请从站点列表进入配置页面。"
      />
    );
  }

  if (
    routeQuery.isLoading ||
    certificatesQuery.isLoading ||
    dnsAccountsQuery.isLoading
  ) {
    return <LoadingState />;
  }

  if (routeQuery.isError) {
    return (
      <ErrorState
        title="站点详情加载失败"
        description={getErrorMessage(routeQuery.error)}
      />
    );
  }

  if (certificatesQuery.isError) {
    return (
      <ErrorState
        title="证书列表加载失败"
        description={getErrorMessage(certificatesQuery.error)}
      />
    );
  }

  if (dnsAccountsQuery.isError) {
    return (
      <ErrorState
        title="Cloudflare 账号列表加载失败"
        description={getErrorMessage(dnsAccountsQuery.error)}
      />
    );
  }

  if (currentSection === 'dns' && dnsZonesQuery.isError) {
    return (
      <ErrorState
        title="托管域名加载失败"
        description={getErrorMessage(dnsZonesQuery.error)}
      />
    );
  }

  if (currentSection === 'proxy' && nodesQuery.isError) {
    return (
      <ErrorState
        title="节点列表加载失败"
        description={getErrorMessage(nodesQuery.error)}
      />
    );
  }

  if (!route) {
    return (
      <EmptyState
        title="站点不存在"
        description="该站点可能已被删除，或当前 ID 无法匹配到记录。"
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={route.site_name}
        description={`主域名 ${route.primary_domain}，共 ${route.domain_count} 个域名`}
        action={
          <div className="flex flex-wrap gap-3">
            <Link
              href="/proxy-route"
              className="inline-flex items-center justify-center rounded-2xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--control-background-hover)]"
            >
              返回列表
            </Link>
            <SecondaryButton
              type="button"
              onClick={() =>
                queryClient.invalidateQueries({
                  queryKey: ['proxy-routes', 'detail', numericRouteID],
                })
              }
            >
              刷新详情
            </SecondaryButton>
          </div>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="space-y-4">
          <AppCard title="配置分区">
            <div className="space-y-2">
              {websiteConfigSections.map((section) => {
                const active = section.key === currentSection;
                return (
                  <Link
                    key={section.key}
                    href={`/proxy-route/detail?id=${route.id}&section=${section.key}`}
                    className={cn(
                      'block rounded-2xl border px-4 py-3 transition',
                      active
                        ? 'border-[var(--border-strong)] bg-[var(--accent-soft)]'
                        : 'border-[var(--border-default)] bg-[var(--surface-elevated)] hover:border-[var(--border-strong)]',
                    )}
                  >
                    <p className="text-sm font-medium text-[var(--foreground-primary)]">
                      {section.label}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                      {section.description}
                    </p>
                  </Link>
                );
              })}
            </div>
          </AppCard>
        </aside>

        <div className="min-w-0 space-y-6">
          {currentSection === 'domains' ? (
            <DomainSettingsSection
              route={route}
              certificates={certificates}
              saving={saveMutation.isPending}
              suggestionSources={domainSuggestionSources}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'cc' ? (
            <CCProtectionSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'proxy' ? (
            <ReverseProxySection
              route={route}
              saving={saveMutation.isPending}
              nodePoolOptions={nodePoolOptions}
              nodes={nodes}
              nodePoolsLoading={nodesQuery.isLoading}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'dns' ? (
            <DNSAutomationSection
              route={route}
              dnsAccounts={dnsAccounts}
              dnsZones={dnsZones}
              dnsZonesLoading={dnsZonesQuery.isLoading}
              nodePoolOptions={nodePoolOptions}
              nodes={nodes}
              nodePoolsLoading={nodesQuery.isLoading}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'cache' ? (
            <CacheSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'region' ? (
            <RegionRestrictionSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}

          {currentSection === 'auth' ? (
            <BasicAuthSection
              route={route}
              saving={saveMutation.isPending}
              onSave={(payload, context) =>
                saveMutation.mutate({ payload, context })
              }
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}
