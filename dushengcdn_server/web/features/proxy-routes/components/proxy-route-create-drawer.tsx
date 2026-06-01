'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

import { Drawer } from '@/components/ui/drawer';
import { getDNSZones } from '@/features/authoritative-dns/api/authoritative-dns';
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
  buildDefaultGSLBPolicy,
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
    dns_provider_mode: z.enum(['cloudflare', 'authoritative']),
    dns_account_id: z.string(),
    dns_zone_id_ref: z.string(),
    dns_record_type: z.enum(['A', 'AAAA', 'CNAME']),
    dns_record_content: z.string(),
    dns_target_count: z.coerce.number().int().min(1).max(20),
    dns_schedule_mode: z.enum(['healthy', 'weighted', 'load_aware']),
    dns_ttl: z.coerce.number().int().min(0).max(86400),
    cloudflare_proxied: z.boolean(),
    ddos_protection_mode: z.enum(['off', 'auto']),
    ddos_protection_provider: z.enum(['cloudflare', 'custom']),
    ddos_protection_target: z.string(),
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

    if (
      value.dns_auto_sync &&
      value.dns_provider_mode === 'cloudflare' &&
      !value.dns_account_id
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_account_id'],
        message: '请选择 Cloudflare 账号',
      });
    }
    if (
      value.dns_auto_sync &&
      value.dns_provider_mode === 'authoritative' &&
      !value.dns_zone_id_ref
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_zone_id_ref'],
        message: '请选择托管域名',
      });
    }
    if (
      value.dns_auto_sync &&
      value.dns_provider_mode === 'authoritative' &&
      value.dns_record_type === 'CNAME'
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_record_type'],
        message: '本地自建解析自动选 IP 只支持 A 或 AAAA',
      });
    }
    if (
      value.dns_auto_sync &&
      value.dns_provider_mode === 'cloudflare' &&
      value.dns_record_type === 'CNAME' &&
      value.dns_record_content.trim() === ''
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['dns_record_content'],
        message: 'CNAME 记录必须填写目标域名',
      });
    }
    if (
      value.dns_auto_sync &&
      value.ddos_protection_mode === 'auto' &&
      value.ddos_protection_provider === 'custom' &&
      value.ddos_protection_target.trim() === ''
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['ddos_protection_target'],
        message: '请选择清洗节点/IP池',
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
  dns_provider_mode: 'cloudflare',
  dns_account_id: '',
  dns_zone_id_ref: '',
  dns_record_type: 'A',
  dns_record_content: '',
  dns_target_count: 1,
  dns_schedule_mode: 'healthy',
  dns_ttl: 1,
  cloudflare_proxied: false,
  ddos_protection_mode: 'off',
  ddos_protection_provider: 'cloudflare',
  ddos_protection_target: '',
  enabled: true,
  redirect_http: false,
  remark: '',
};

const dnsTTLHint =
  '0 表示自动 TTL；1 表示 Cloudflare 自动 TTL；2-29 秒会在保存时提升到 30 秒；30 秒及以上按填写值同步，最高 86400 秒。';
const dnsScheduleModeHints: Record<
  CreateWebsiteFormValues['dns_schedule_mode'],
  string
> = {
  healthy:
    '健康优先只看节点是否在线、OpenResty 是否健康、是否允许调度和最近心跳时间；旧目标仍健康且处于冷却期时会保持不动。',
  weighted: '权重优先会先过滤健康候选，再按节点权重排序。',
  load_aware:
    '按压力优先会在健康候选中参考最新连接数、CPU、内存，尽量避开压力高的节点。',
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
  const dnsAutoSync = form.watch('dns_auto_sync');
  const dnsProviderMode = form.watch('dns_provider_mode');
  const isAuthoritativeMode = dnsAutoSync && dnsProviderMode === 'authoritative';
  const dnsRecordType = form.watch('dns_record_type');
  const dnsScheduleMode = form.watch('dns_schedule_mode');
  const ddosProtectionMode = form.watch('ddos_protection_mode');
  const ddosProtectionProvider = form.watch('ddos_protection_provider');
  const ddosControlsEnabled =
    dnsAutoSync && !isAuthoritativeMode && ddosProtectionMode === 'auto';
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
  const dnsZonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
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
  const cloudflareAccounts = useMemo(
    () =>
      (dnsAccountsQuery.data ?? []).filter(
        (account) => account.type === 'cloudflare',
      ),
    [dnsAccountsQuery.data],
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
      const dnsZoneIDRef = Number(values.dns_zone_id_ref);
      const authoritativeMode =
        values.dns_auto_sync && values.dns_provider_mode === 'authoritative';
      const cloudflareMode =
        values.dns_auto_sync && values.dns_provider_mode === 'cloudflare';
      const gslbPolicy = buildDefaultGSLBPolicy(
        values.node_pool.trim() || 'default',
      );

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
        dns_auto_sync: cloudflareMode,
        dns_account_id:
          cloudflareMode && Number.isFinite(dnsAccountID) && dnsAccountID > 0
            ? dnsAccountID
            : null,
        dns_zone_id: '',
        dns_record_type: authoritativeMode
          ? values.dns_record_type === 'AAAA'
            ? 'AAAA'
            : 'A'
          : values.dns_record_type,
        dns_record_name: '',
        dns_record_content: authoritativeMode
          ? ''
          : values.dns_record_content.trim(),
        dns_auto_target:
          authoritativeMode ||
          (cloudflareMode && values.dns_record_content.trim() === ''),
        dns_target_count: values.dns_target_count,
        dns_schedule_mode: values.dns_schedule_mode,
        dns_ttl: values.dns_ttl,
        dns_provider_mode: values.dns_auto_sync
          ? values.dns_provider_mode
          : 'cloudflare',
        dns_zone_id_ref:
          authoritativeMode &&
          Number.isFinite(dnsZoneIDRef) &&
          dnsZoneIDRef > 0
            ? dnsZoneIDRef
            : null,
        gslb_enabled: false,
        gslb_policy: {
          ...gslbPolicy,
          strategy: values.dns_schedule_mode,
          target_count: values.dns_target_count,
          ttl: values.dns_ttl,
        },
        cloudflare_proxied: authoritativeMode ? false : values.cloudflare_proxied,
        ddos_protection_mode: authoritativeMode
          ? 'off'
          : values.ddos_protection_mode,
        ddos_protection_provider: authoritativeMode
          ? 'cloudflare'
          : values.ddos_protection_provider,
        ddos_protection_target:
          !authoritativeMode &&
          values.ddos_protection_mode === 'auto' &&
          values.ddos_protection_provider === 'cloudflare'
            ? values.ddos_protection_target || values.dns_account_id
            : !authoritativeMode && values.ddos_protection_mode === 'auto'
              ? values.ddos_protection_target.trim()
              : '',
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
          description="可选择 Cloudflare 后台同步，或使用本地自建解析托管域名；记录值留空会自动选择在线节点 IP。"
          checked={dnsAutoSync}
          onChange={(checked) => {
            form.setValue('dns_auto_sync', checked, { shouldDirty: true });
            if (!checked) {
              form.setValue('ddos_protection_mode', 'off', {
                shouldDirty: true,
              });
              form.setValue('ddos_protection_target', '', {
                shouldDirty: true,
              });
            }
          }}
        />

        {dnsAutoSync ? (
          <div className="grid gap-4 md:grid-cols-2">
            <ResourceField
              label="解析方式"
              hint="Cloudflare 会同步到账号里的 DNS；本地自建解析会交给左侧「本地自建解析」里的托管域名。"
            >
              <ResourceSelect
                aria-label="解析方式"
                {...form.register('dns_provider_mode', {
                  onChange: (event) => {
                    if (event.target.value === 'authoritative') {
                      form.setValue('dns_account_id', '', {
                        shouldDirty: true,
                      });
                      form.setValue('dns_record_content', '', {
                        shouldDirty: true,
                      });
                      form.setValue('cloudflare_proxied', false, {
                        shouldDirty: true,
                      });
                      form.setValue('ddos_protection_mode', 'off', {
                        shouldDirty: true,
                      });
                      form.setValue('ddos_protection_target', '', {
                        shouldDirty: true,
                      });
                      if (form.getValues('dns_record_type') === 'CNAME') {
                        form.setValue('dns_record_type', 'A', {
                          shouldDirty: true,
                        });
                      }
                    } else {
                      form.setValue('dns_zone_id_ref', '', {
                        shouldDirty: true,
                      });
                    }
                  },
                })}
              >
                <option value="cloudflare">Cloudflare 同步</option>
                <option value="authoritative">本地自建解析</option>
              </ResourceSelect>
            </ResourceField>

            {isAuthoritativeMode ? (
              <ResourceField
                label="托管域名"
                hint="网站域名必须属于所选托管域名。"
                error={form.formState.errors.dns_zone_id_ref?.message}
              >
                <ResourceSelect
                  aria-label="托管域名"
                  disabled={dnsZonesQuery.isLoading}
                  {...form.register('dns_zone_id_ref')}
                >
                  <option value="">
                    {dnsZonesQuery.isLoading
                      ? '正在加载托管域名...'
                      : '请选择托管域名'}
                  </option>
                  {dnsZonesQuery.data?.map((zone) => (
                    <option key={zone.id} value={zone.id}>
                      {zone.name}
                      {zone.enabled ? '' : '（已停用）'}
                    </option>
                  ))}
                </ResourceSelect>
              </ResourceField>
            ) : (
              <ResourceField
                label="Cloudflare 账号"
                hint="需要 API 密钥具备 Zone Read 和 DNS Edit 权限。"
                error={form.formState.errors.dns_account_id?.message}
              >
                <ResourceSelect
                  aria-label="Cloudflare 账号"
                  {...form.register('dns_account_id', {
                    onChange: (event) => {
                      if (
                        form.getValues('ddos_protection_provider') ===
                          'cloudflare' &&
                        !form.getValues('ddos_protection_target')
                      ) {
                        form.setValue(
                          'ddos_protection_target',
                          event.target.value,
                          { shouldDirty: true },
                        );
                      }
                    },
                  })}
                >
                  <option value="">请选择 Cloudflare 账号</option>
                  {cloudflareAccounts.map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.name}
                    </option>
                  ))}
                </ResourceSelect>
              </ResourceField>
            )}

            <ResourceField label="记录类型">
              <ResourceSelect {...form.register('dns_record_type')}>
                <option value="A">A</option>
                <option value="AAAA">AAAA</option>
                {!isAuthoritativeMode ? (
                  <option value="CNAME">CNAME</option>
                ) : null}
              </ResourceSelect>
            </ResourceField>

            {!isAuthoritativeMode ? (
              <ResourceField
                label="IP 或目标域名"
                hint={
                  dnsRecordType === 'CNAME'
                    ? '填写要指向的目标域名。'
                    : '固定 IP 时可用逗号、空格或换行填写多个 IP；留空会按节点池自动选择在线节点 IP。'
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
            ) : null}

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

                <ResourceField
                  label="调度模式"
                  hint={dnsScheduleModeHints[dnsScheduleMode]}
                >
                  <ResourceSelect {...form.register('dns_schedule_mode')}>
                    <option value="healthy">健康优先（冷却防抖）</option>
                    <option value="weighted">按权重优先</option>
                    <option value="load_aware">按压力优先</option>
                  </ResourceSelect>
                </ResourceField>

                <ResourceField
                  label="DNS 缓存时间"
                  hint={dnsTTLHint}
                  tooltip="这个时间决定用户本地或运营商 DNS 多久刷新一次记录。时间短切换更快，查询量会更高。"
                >
                  <ResourceInput
                    type="number"
                    min={0}
                    max={86400}
                    {...form.register('dns_ttl', {
                      valueAsNumber: true,
                    })}
                  />
                </ResourceField>
              </>
            ) : null}

            <ToggleField
              label="常态开启 Cloudflare 代理"
              description="正常状态也同步橙云；攻击自动防护恢复后会回到这个常态设置。"
              checked={form.watch('cloudflare_proxied')}
              disabled={isAuthoritativeMode}
              onChange={(checked) =>
                form.setValue('cloudflare_proxied', checked, {
                  shouldDirty: true,
                })
              }
            />

            <ResourceField
              label="攻击防护模式"
              hint="关闭时不做自动切换；自动时攻击期暂停多节点智能解析，并临时切到所选防护目标。"
              tooltip="系统会根据最近请求量和错误率判断是否疑似攻击。触发后先保护可用性，恢复正常后再回到原解析策略。"
            >
              <ResourceSelect
                aria-label="攻击防护模式"
                disabled={isAuthoritativeMode}
                {...form.register('ddos_protection_mode', {
                  onChange: (event) => {
                    if (event.target.value === 'off') {
                      form.setValue('ddos_protection_target', '', {
                        shouldDirty: true,
                      });
                    } else if (!form.getValues('ddos_protection_target')) {
                      form.setValue(
                        'ddos_protection_target',
                        form.getValues('dns_account_id'),
                        { shouldDirty: true },
                      );
                    }
                  },
                })}
              >
                <option value="off">关闭</option>
                <option value="auto">自动</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="防护提供方"
              hint="Cloudflare 会开启橙云；自定义会切到指定清洗节点/IP 池。"
            >
              <ResourceSelect
                aria-label="防护提供方"
                disabled={!ddosControlsEnabled || isAuthoritativeMode}
                {...form.register('ddos_protection_provider', {
                  onChange: (event) => {
                    form.setValue(
                      'ddos_protection_target',
                      event.target.value === 'cloudflare'
                        ? form.getValues('dns_account_id')
                        : form.getValues('node_pool'),
                      { shouldDirty: true },
                    );
                  },
                })}
              >
                <option value="cloudflare">Cloudflare</option>
                <option value="custom">自定义清洗池</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label={
                ddosProtectionProvider === 'custom'
                  ? '清洗节点/IP池'
                  : 'Cloudflare 账号'
              }
              hint={
                ddosProtectionProvider === 'custom'
                  ? '攻击期只返回该池内在线且可调度的公网 IP。'
                  : '留空时使用上方自动解析账号。'
              }
              error={form.formState.errors.ddos_protection_target?.message}
            >
              {ddosProtectionProvider === 'custom' ? (
                <ResourceSelect
                  aria-label="清洗节点/IP池"
                  disabled={!ddosControlsEnabled || nodesQuery.isLoading}
                  {...form.register('ddos_protection_target')}
                >
                  <option value="">请选择清洗池</option>
                  {nodePoolOptions.map((poolName) => (
                    <option key={poolName} value={poolName}>
                      {poolName}
                    </option>
                  ))}
                </ResourceSelect>
              ) : (
                <ResourceSelect
                  aria-label="攻击防护 Cloudflare 账号"
                  disabled={!ddosControlsEnabled || isAuthoritativeMode}
                  {...form.register('ddos_protection_target')}
                >
                  <option value="">使用上方账号</option>
                  {cloudflareAccounts.map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.name}
                    </option>
                  ))}
                </ResourceSelect>
              )}
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
