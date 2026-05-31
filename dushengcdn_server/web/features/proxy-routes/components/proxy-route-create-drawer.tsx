'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

import { Drawer } from '@/components/ui/drawer';
import { getDnsAccounts } from '@/features/dns-accounts/api/dns-accounts';
import { getManagedDomains } from '@/features/managed-domains/api/managed-domains';
import { getNodes } from '@/features/nodes/api/nodes';
import { createProxyRoute } from '@/features/proxy-routes/api/proxy-routes';
import {
  DomainListInput,
  type DomainListRow,
} from '@/features/proxy-routes/components/domain-list-input';
import {
  buildNodePoolOptions,
  NodePoolSelect,
} from '@/features/proxy-routes/components/node-pool-select';
import {
  buildOriginUrl,
  getErrorMessage,
  parseOriginUrl,
  parseOriginUrls,
  validateDomains,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import { getTlsCertificates } from '@/features/tls-certificates/api/tls-certificates';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

const domainRowSchema = z.object({
  domain: z.string(),
  certificateId: z.string(),
});

const createWebsiteSchema = z
  .object({
    site_name: z.string().trim().max(255, '站点标识不能超过 255 个字符'),
    node_pool: z
      .string()
      .trim()
      .max(64, '节点池名称不能超过 64 个字符'),
    domain_rows: z.array(domainRowSchema).min(1),
    origin_urls_text: z.string().trim().min(1, '请至少填写一个源站地址'),
    dns_auto_sync: z.boolean(),
    dns_account_id: z.string(),
    dns_record_type: z.enum(['A', 'AAAA', 'CNAME']),
    dns_record_content: z.string(),
    dns_target_count: z.coerce.number().int().min(1).max(20),
    dns_schedule_mode: z.enum(['healthy', 'weighted']),
    cloudflare_proxied: z.boolean(),
    ddos_protection_mode: z.enum(['off', 'manual', 'auto']),
    enabled: z.boolean(),
    redirect_http: z.boolean(),
    remark: z.string().max(255, '备注不能超过 255 个字符'),
  })
  .superRefine((value, context) => {
    const domains = value.domain_rows
      .map((item) => item.domain.trim().toLowerCase())
      .filter(Boolean);
    const domainError = validateDomains(domains);
    if (domainError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['domain_rows'],
        message: domainError,
      });
    }

    const { error } = parseOriginUrls(value.origin_urls_text);
    if (error) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['origin_urls_text'],
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

    if (value.dns_auto_sync && !value.dns_account_id) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_account_id'],
        message: '启用自动 DNS 时请选择 DNS 账号',
      });
    }
    if (
      value.dns_auto_sync &&
      value.dns_record_type === 'CNAME' &&
      value.dns_record_content.trim() === ''
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_record_content'],
        message: 'CNAME 记录必须填写目标域名',
      });
    }
  });

type CreateWebsiteFormValues = z.infer<typeof createWebsiteSchema>;

const defaultValues: CreateWebsiteFormValues = {
  site_name: '',
  node_pool: 'default',
  domain_rows: [{ domain: '', certificateId: '' }],
  origin_urls_text: '',
  dns_auto_sync: false,
  dns_account_id: '',
  dns_record_type: 'A',
  dns_record_content: '',
  dns_target_count: 1,
  dns_schedule_mode: 'healthy',
  cloudflare_proxied: false,
  ddos_protection_mode: 'off',
  enabled: true,
  redirect_http: false,
  remark: '',
};

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

export function ProxyRouteCreateDrawer({
  open,
  onOpenChange,
  onCreated,
  domainSuggestionSources = [],
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (route: ProxyRouteItem) => void;
  domainSuggestionSources?: string[];
}) {
  const form = useForm<CreateWebsiteFormValues>({
    resolver: zodResolver(createWebsiteSchema),
    defaultValues,
  });
  const managedDomainsQuery = useQuery({
    queryKey: ['managed-domains'],
    queryFn: getManagedDomains,
    enabled: open,
  });
  const certificatesQuery = useQuery({
    queryKey: ['tls-certificates', 'list'],
    queryFn: getTlsCertificates,
    enabled: open,
  });
  const dnsAccountsQuery = useQuery({
    queryKey: ['dns-accounts'],
    queryFn: getDnsAccounts,
    enabled: open,
  });
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: getNodes,
    enabled: open,
  });

  const combinedDomainSuggestions = useMemo(
    () => [
      ...domainSuggestionSources,
      ...(managedDomainsQuery.data?.map((item) => item.domain) ?? []),
    ],
    [domainSuggestionSources, managedDomainsQuery.data],
  );
  const selectedCertificateIDs = normalizeSelectedCertificateIDs(
    form.watch('domain_rows'),
  );
  const nodePoolValue = form.watch('node_pool');
  const nodePoolOptions = useMemo(
    () => buildNodePoolOptions(nodesQuery.data ?? [], nodePoolValue),
    [nodePoolValue, nodesQuery.data],
  );

  const createMutation = useMutation({
    mutationFn: async (values: CreateWebsiteFormValues) => {
      const domains = values.domain_rows
        .map((item) => item.domain.trim().toLowerCase())
        .filter(Boolean);
      const domainCertIDs = buildDomainCertificateIDs(values.domain_rows);
      const selectedCertIDs = normalizeSelectedCertificateIDs(values.domain_rows);
      const { urls } = parseOriginUrls(values.origin_urls_text);
      const primaryOrigin = parseOriginUrl(urls[0]);
      const dnsAccountID = Number(values.dns_account_id);

      return createProxyRoute({
        site_name: values.site_name.trim() || domains[0],
        domain: domains[0],
        domains,
        origin_id: null,
        origin_url: buildOriginUrl(
          primaryOrigin.scheme,
          primaryOrigin.address,
          primaryOrigin.port,
          primaryOrigin.uri,
        ),
        origin_scheme: primaryOrigin.scheme,
        origin_address: primaryOrigin.address,
        origin_port: primaryOrigin.port,
        origin_uri: primaryOrigin.uri,
        origin_host: '',
        upstreams: urls.slice(1),
        node_pool: values.node_pool.trim() || 'default',
        enabled: values.enabled,
        enable_https: selectedCertIDs.length > 0,
        cert_id: selectedCertIDs[0] ?? null,
        cert_ids: selectedCertIDs,
        domain_cert_ids: domainCertIDs,
        redirect_http: selectedCertIDs.length > 0 ? values.redirect_http : false,
        limit_conn_per_server: 0,
        limit_conn_per_ip: 0,
        limit_rate: '',
        cache_enabled: false,
        cache_policy: 'url',
        cache_rules: [],
        custom_headers: [],
        pow_enabled: false,
        pow_config: '{}',
        waf_enabled: false,
        waf_mode: 'block',
        waf_config: '{}',
        basic_auth_enabled: false,
        basic_auth_username: '',
        basic_auth_password: '',
        region_restriction_enabled: false,
        region_restriction_mode: 'block',
        region_restriction_countries: [],
        dns_auto_sync: values.dns_auto_sync,
        dns_account_id:
          values.dns_auto_sync && Number.isFinite(dnsAccountID) && dnsAccountID > 0
            ? dnsAccountID
            : null,
        dns_zone_id: '',
        dns_record_type: values.dns_record_type,
        dns_record_name: '',
        dns_record_content: values.dns_record_content.trim(),
        dns_auto_target: values.dns_auto_sync && values.dns_record_content.trim() === '',
        dns_target_count: values.dns_target_count,
        dns_schedule_mode: values.dns_schedule_mode,
        cloudflare_proxied: values.cloudflare_proxied,
        ddos_protection_mode: values.ddos_protection_mode,
        remark: values.remark.trim(),
      });
    },
    onSuccess: (route) => {
      form.reset(defaultValues);
      onOpenChange(false);
      onCreated(route);
    },
  });

  useEffect(() => {
    if (!open) {
      form.reset(defaultValues);
    }
  }, [form, open]);
  const dnsAutoSync = form.watch('dns_auto_sync');
  const dnsRecordType = form.watch('dns_record_type');

  return (
    <Drawer
      open={open}
      onOpenChange={onOpenChange}
      direction="right"
      title="新建规则"
      footer={
        <div className="flex items-center justify-end gap-3">
          <PrimaryButton
            type="submit"
            form="create-website-form"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? '创建中...' : '创建'}
          </PrimaryButton>
        </div>
      }
    >
      <form
        id="create-website-form"
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}
      >
        <ResourceField
          label="站点标识"
          hint="可选，留空时会自动使用第一个域名。"
          error={form.formState.errors.site_name?.message}
        >
          <ResourceInput
            {...form.register('site_name')}
            placeholder="marketing-site"
          />
        </ResourceField>

        <ResourceField
          label="节点池"
          hint="自动 DNS 会从该节点池选择公网 IP，缓存清理/预热也会下发到该池在线节点。"
          error={form.formState.errors.node_pool?.message}
          container="div"
        >
          <Controller
            control={form.control}
            name="node_pool"
            render={({ field }) => (
              <NodePoolSelect
                name={field.name}
                value={field.value}
                options={nodePoolOptions}
                disabled={nodesQuery.isLoading}
                onChange={field.onChange}
                onBlur={field.onBlur}
              />
            )}
          />
        </ResourceField>

        <ResourceField
          label="域名列表"
          hint="每行配置一个域名，可按需为该行选择证书。保存时会自动汇总站点证书集合。"
          error={form.formState.errors.domain_rows?.message as string | undefined}
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
                suggestionSources={combinedDomainSuggestions}
                certificates={certificatesQuery.data ?? []}
              />
            )}
          />
        </ResourceField>

        <ToggleField
          label="HTTP 自动跳转到 HTTPS"
          description={
            selectedCertificateIDs.length > 0
              ? '勾选后会额外生成 80 端口重定向规则。'
              : '至少为一个域名选择证书后才能启用。'
          }
          checked={form.watch('redirect_http')}
          disabled={selectedCertificateIDs.length === 0}
          onChange={(checked) =>
            form.setValue('redirect_http', checked, { shouldDirty: true })
          }
        />

        <ToggleField
          label="创建时自动解析 DNS"
          description="绑定 Cloudflare 后，创建规则时同步 DNS；记录内容留空会自动选择在线节点公网 IP。"
          checked={dnsAutoSync}
          onChange={(checked) =>
            form.setValue('dns_auto_sync', checked, { shouldDirty: true })
          }
        />

        {dnsAutoSync ? (
          <div className="grid gap-4 md:grid-cols-2">
            <ResourceField
              label="DNS 账号"
              hint="需要 Cloudflare API Token 具备 Zone Read 和 DNS Edit 权限。"
              error={form.formState.errors.dns_account_id?.message}
            >
              <ResourceSelect {...form.register('dns_account_id')}>
                <option value="">请选择 DNS 账号</option>
                {(dnsAccountsQuery.data ?? [])
                  .filter((account) => account.type === 'cloudflare')
                  .map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.name}
                    </option>
                  ))}
              </ResourceSelect>
            </ResourceField>

            <ResourceField label="记录类型">
              <ResourceSelect {...form.register('dns_record_type')}>
                <option value="A">A</option>
                <option value="AAAA">AAAA</option>
                <option value="CNAME">CNAME</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="记录内容"
              hint={
                dnsRecordType === 'CNAME'
                  ? '填写 CNAME 目标域名。'
                  : '可填写多个 A/AAAA 内容；留空会按节点池自动选择在线节点 IP。'
              }
              error={form.formState.errors.dns_record_content?.message}
              className="md:col-span-2"
            >
              <ResourceInput
                placeholder={
                  dnsRecordType === 'CNAME'
                    ? 'target.example.com'
                    : '留空自动选择，或填写多个 IP'
                }
                {...form.register('dns_record_content')}
              />
            </ResourceField>

            {dnsRecordType !== 'CNAME' ? (
              <>
                <ResourceField
                  label="目标数量"
                  hint="自动选择时最多同步多少个节点 IP。"
                >
                  <ResourceInput
                    type="number"
                    min={1}
                    max={20}
                    {...form.register('dns_target_count', {
                      valueAsNumber: true,
                    })}
                  />
                </ResourceField>

                <ResourceField label="调度模式">
                  <ResourceSelect {...form.register('dns_schedule_mode')}>
                    <option value="healthy">按健康时间</option>
                    <option value="weighted">按权重优先</option>
                  </ResourceSelect>
                </ResourceField>
              </>
            ) : null}

            <ToggleField
              label="开启 Cloudflare 代理"
              description="创建后直接切换为橙云，适合需要隐藏源站或抗攻击的域名。"
              checked={form.watch('cloudflare_proxied')}
              onChange={(checked) =>
                form.setValue('cloudflare_proxied', checked, {
                  shouldDirty: true,
                })
              }
            />

            <ResourceField label="DDoS 防护模式">
              <ResourceSelect {...form.register('ddos_protection_mode')}>
                <option value="off">关闭</option>
                <option value="manual">手动</option>
                <option value="auto">自动</option>
              </ResourceSelect>
            </ResourceField>
          </div>
        ) : null}

        <ResourceField
          label="源站地址"
          hint="每行一个完整 URL，协议和端口都在这里配置，例如 https://origin.internal:443。第一行作为主源站，多源站模式请保持相同协议且不要包含 path 或 query。"
          error={form.formState.errors.origin_urls_text?.message}
        >
          <ResourceTextarea
            aria-label="源站地址"
            placeholder={'https://origin-a.internal:443\nhttps://origin-b.internal:443'}
            {...form.register('origin_urls_text')}
          />
        </ResourceField>

        <ToggleField
          label="创建后立即启用"
          description="关闭后站点会以草稿保存，后续仍可继续编辑。"
          checked={form.watch('enabled')}
          onChange={(checked) =>
            form.setValue('enabled', checked, { shouldDirty: true })
          }
        />

        <ResourceField
          label="备注"
          error={form.formState.errors.remark?.message}
        >
          <ResourceTextarea {...form.register('remark')} />
        </ResourceField>

        {createMutation.isError ? (
          <p className="text-sm text-[var(--status-danger-foreground)]">
            {getErrorMessage(createMutation.error)}
          </p>
        ) : null}
      </form>
    </Drawer>
  );
}
