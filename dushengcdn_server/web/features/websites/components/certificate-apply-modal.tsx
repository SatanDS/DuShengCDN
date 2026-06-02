'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';

import { InlineMessage } from '@/components/feedback/inline-message';
import { AppModal } from '@/components/ui/app-modal';
import { getDNSZones } from '@/features/authoritative-dns/api/authoritative-dns';
import {
  applyTlsCertificate,
  convertTlsCertificateToAcme,
  updateAcmeCertificate,
} from '@/features/tls-certificates/api/tls-certificates';
import type { TlsCertificateItem } from '@/features/tls-certificates/types';
import { getDnsAccounts } from '@/features/dns-accounts/api/dns-accounts';
import { getDefaultAcmeAccount } from '@/features/acme-accounts/api/acme-accounts';
import {
  acmeApplySchema,
  defaultAcmeApplyValues,
  type AcmeApplyFormValues,
} from '@/features/websites/schemas';
import { getErrorMessage } from '@/features/websites/utils';
import {
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

interface CertificateApplyModalProps {
  isOpen: boolean;
  onClose: () => void;
  onApplied?: (certificate: TlsCertificateItem) => void;
  mode?: 'create' | 'edit-acme' | 'convert-upload';
  certificate?: TlsCertificateItem | null;
  defaultPrimaryDomain?: string;
  defaultName?: string;
  defaultDNSProviderMode?: AcmeApplyFormValues['dns_provider_mode'];
  defaultDNSZoneIDRef?: number | null;
}

const emptyAuthoritativeZones: ReturnType<typeof getDNSZones> extends Promise<
  infer T
>
  ? T
  : never = [];

export function CertificateApplyModal({
  isOpen,
  onClose,
  onApplied,
  mode = 'create',
  certificate,
  defaultPrimaryDomain = '',
  defaultName = '',
  defaultDNSProviderMode = 'cloudflare',
  defaultDNSZoneIDRef = null,
}: CertificateApplyModalProps) {
  const queryClient = useQueryClient();
  const [feedback, setFeedback] = useState<{
    tone: 'success' | 'danger';
    message: string;
  } | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [dnsProviderModeTouched, setDNSProviderModeTouched] = useState(false);

  const dnsAccountsQuery = useQuery({
    queryKey: ['dns-accounts'],
    queryFn: getDnsAccounts,
    enabled: isOpen,
  });
  const dnsZonesQuery = useQuery({
    queryKey: ['authoritative-dns', 'zones'],
    queryFn: getDNSZones,
    enabled: isOpen,
  });

  const defaultAcmeAccountQuery = useQuery({
    queryKey: ['acme-accounts', 'default'],
    queryFn: getDefaultAcmeAccount,
    enabled: isOpen,
  });

  const form = useForm<AcmeApplyFormValues>({
    resolver: zodResolver(acmeApplySchema),
    defaultValues: defaultAcmeApplyValues,
  });

  useEffect(() => {
    if (!isOpen) return;
    setFeedback(null);
    setShowAdvanced(false);
    setDNSProviderModeTouched(false);

    if (certificate) {
      const selectedDNSProviderMode =
        mode === 'convert-upload'
          ? defaultDNSProviderMode === 'authoritative'
            ? 'authoritative'
            : 'cloudflare'
          : certificate.dns_provider_mode || defaultDNSProviderMode;
      form.reset({
        name: certificate.name,
        primary_domain:
          mode === 'convert-upload' ? '' : certificate.primary_domain || '',
        other_domains:
          mode === 'convert-upload' ? '' : certificate.other_domains || '',
        remark: certificate.remark || '',
        acme_account_id:
          mode === 'convert-upload' ? 0 : certificate.acme_account_id,
        dns_provider_mode: selectedDNSProviderMode,
        dns_account_id:
          mode === 'convert-upload' ||
          selectedDNSProviderMode === 'authoritative'
            ? 0
            : certificate.dns_account_id,
        dns_zone_id_ref:
          mode === 'convert-upload'
            ? defaultDNSZoneIDRef
            : certificate.dns_zone_id_ref,
        key_algorithm: certificate.key_algorithm || 'EC256',
        auto_renew: mode === 'convert-upload' ? true : certificate.auto_renew,
        dns1: mode === 'convert-upload' ? '' : certificate.dns1 || '',
        dns2: mode === 'convert-upload' ? '' : certificate.dns2 || '',
        disable_cname:
          mode === 'convert-upload' ? false : certificate.disable_cname,
        skip_dns: mode === 'convert-upload' ? false : certificate.skip_dns,
      });
      if (
        mode !== 'convert-upload' &&
        (certificate.dns1 ||
          certificate.dns2 ||
          certificate.disable_cname ||
          certificate.skip_dns)
      ) {
        setShowAdvanced(true);
      }
    } else {
      const primaryDomain = normalizeCertificateDomain(defaultPrimaryDomain);
      const initialDNSProviderMode =
        defaultDNSProviderMode === 'authoritative'
          ? 'authoritative'
          : 'cloudflare';
      form.reset({
        ...defaultAcmeApplyValues,
        name:
          defaultName.trim() ||
          (primaryDomain ? `${primaryDomain} 证书` : ''),
        primary_domain: primaryDomain,
        dns_provider_mode: initialDNSProviderMode,
        dns_account_id:
          initialDNSProviderMode === 'authoritative'
            ? 0
            : defaultAcmeApplyValues.dns_account_id,
        dns_zone_id_ref:
          initialDNSProviderMode === 'authoritative'
            ? defaultDNSZoneIDRef
            : null,
      });
    }
  }, [
    isOpen,
    form,
    mode,
    certificate,
    defaultPrimaryDomain,
    defaultName,
    defaultDNSProviderMode,
    defaultDNSZoneIDRef,
  ]);

  useEffect(() => {
    if (
      defaultAcmeAccountQuery.data &&
      form.getValues('acme_account_id') === 0
    ) {
      form.setValue('acme_account_id', defaultAcmeAccountQuery.data.id);
    }
  }, [defaultAcmeAccountQuery.data, form, isOpen]);

  const applyMutation = useMutation({
    mutationFn: (values: AcmeApplyFormValues) => {
      if (mode === 'edit-acme' && certificate) {
        return updateAcmeCertificate(certificate.id, values);
      }
      if (mode === 'convert-upload' && certificate) {
        return convertTlsCertificateToAcme(certificate.id, values);
      }
      return applyTlsCertificate(values);
    },
    onSuccess: async (certificate) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['tls-certificates'] }),
        queryClient.invalidateQueries({ queryKey: ['managed-domains'] }),
      ]);
      onApplied?.(certificate);
      onClose();
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const onSubmit = form.handleSubmit((values) => {
    setFeedback(null);
    applyMutation.mutate({
      ...values,
      dns_account_id:
        values.dns_provider_mode === 'cloudflare' ? values.dns_account_id : 0,
      dns_zone_id_ref:
        values.dns_provider_mode === 'authoritative'
          ? values.dns_zone_id_ref
          : null,
    });
  });

  const dnsProviderMode = form.watch('dns_provider_mode');
  const primaryDomain = form.watch('primary_domain');
  const otherDomains = form.watch('other_domains');
  const cloudflareAccounts =
    dnsAccountsQuery.data?.filter((account) => account.type === 'cloudflare') ??
    [];
  const authoritativeZones = dnsZonesQuery.data ?? emptyAuthoritativeZones;
  const enabledAuthoritativeZones = useMemo(
    () => authoritativeZones.filter((zone) => zone.enabled),
    [authoritativeZones],
  );
  const certificateDomains = useMemo(
    () => parseCertificateDomains(primaryDomain, otherDomains),
    [primaryDomain, otherDomains],
  );
  const matchingAuthoritativeZones = useMemo(
    () =>
      enabledAuthoritativeZones.filter(
        (zone) =>
          certificateDomains.length > 0 &&
          certificateDomains.every((domain) =>
            certificateDomainBelongsToZone(domain, zone.name),
          ),
      ),
    [enabledAuthoritativeZones, certificateDomains],
  );
  const preferredAuthoritativeZone = useMemo(
    () =>
      matchingAuthoritativeZones
        .slice()
        .sort((left, right) => right.name.length - left.name.length)[0] ??
      null,
    [matchingAuthoritativeZones],
  );
  const shouldPreferAuthoritativeValidation =
    defaultDNSProviderMode === 'authoritative' ||
    Boolean(preferredAuthoritativeZone) ||
    Boolean(normalizeCertificateDomain(defaultPrimaryDomain)) ||
    (dnsAccountsQuery.isSuccess && cloudflareAccounts.length === 0);

  useEffect(() => {
    if (
      !isOpen ||
      mode !== 'create' ||
      certificate ||
      dnsProviderModeTouched ||
      dnsProviderMode !== 'cloudflare' ||
      !preferredAuthoritativeZone ||
      !shouldPreferAuthoritativeValidation
    ) {
      return;
    }

    form.setValue('dns_provider_mode', 'authoritative', {
      shouldDirty: false,
      shouldValidate: true,
    });
    form.setValue('dns_account_id', 0, {
      shouldDirty: false,
      shouldValidate: false,
    });
    form.setValue('dns_zone_id_ref', preferredAuthoritativeZone.id, {
      shouldDirty: false,
      shouldValidate: true,
    });
  }, [
    isOpen,
    mode,
    certificate,
    dnsProviderModeTouched,
    dnsProviderMode,
    preferredAuthoritativeZone,
    shouldPreferAuthoritativeValidation,
    form,
  ]);

  useEffect(() => {
    if (dnsProviderMode !== 'authoritative') {
      return;
    }
    const currentZoneID = Number(form.getValues('dns_zone_id_ref'));
    if (Number.isFinite(currentZoneID) && currentZoneID > 0) {
      if (authoritativeZones.some((zone) => zone.id === currentZoneID)) {
        form.setValue('dns_zone_id_ref', currentZoneID, {
          shouldDirty: false,
          shouldValidate: false,
        });
      }
      return;
    }
    if (preferredAuthoritativeZone) {
      form.setValue('dns_zone_id_ref', preferredAuthoritativeZone.id, {
        shouldDirty: true,
      });
    }
  }, [dnsProviderMode, form, authoritativeZones, preferredAuthoritativeZone]);

  const authoritativeZoneHint = dnsZonesQuery.isError
    ? `本地托管域名加载失败：${getErrorMessage(dnsZonesQuery.error)}`
    : enabledAuthoritativeZones.length === 0
      ? '还没有可用的本地托管域名。先在“本地自建解析”里创建并启用域名，再回来申请证书。'
      : certificateDomains.length > 0 && matchingAuthoritativeZones.length === 0
        ? '当前证书域名没有匹配的本地托管域名，请先创建或启用对应根域名。'
        : matchingAuthoritativeZones.length > 0
          ? '已按证书域名匹配可用的本地托管域名。系统会临时写入并清理验证记录。'
          : '证书里的所有域名都必须属于这里选择的本地托管域名。系统会临时写入并清理验证记录。';

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={
        mode === 'edit-acme'
          ? '编辑并重新申请证书'
          : mode === 'convert-upload'
            ? '转换为申请证书'
            : '申请证书'
      }
      description={
        mode === 'edit-acme'
          ? '修改自动申请配置。保存后将使用新配置重新申请证书。'
          : mode === 'convert-upload'
            ? '填写自动申请资料。申请成功后，当前手动证书会原地转换为可自动续签的申请证书。'
            : "自动向 Let's Encrypt 等证书机构申请和续期证书，支持通配符域名。"
      }
      size="xl"
    >
      <form className="space-y-5" onSubmit={onSubmit}>
        {feedback ? (
          <InlineMessage tone={feedback.tone} message={feedback.message} />
        ) : null}

        <div className="grid gap-4 md:grid-cols-2">
          <ResourceField
            label="证书名称"
            error={form.formState.errors.name?.message}
          >
            <ResourceInput
              placeholder="例如：主站证书"
              {...form.register('name')}
            />
          </ResourceField>
          <ResourceField
            label="主域名"
            error={form.formState.errors.primary_domain?.message}
          >
            <ResourceInput
              placeholder="example.com 或 *.example.com"
              {...form.register('primary_domain')}
            />
          </ResourceField>
        </div>

        <ResourceField
          label="其他域名"
          hint="每行一个域名。如申请通配符证书，请填写对应的根域名以便一并签发。"
          error={form.formState.errors.other_domains?.message}
        >
          <textarea
            className="w-full rounded-xl border border-[var(--border-default)] bg-[var(--control-background)] px-4 py-3 text-sm text-[var(--foreground-primary)] transition outline-none focus:border-[var(--brand-primary)]"
            rows={3}
            placeholder="example.net"
            {...form.register('other_domains')}
          />
        </ResourceField>

        <div className="grid gap-4 md:grid-cols-2">
          <ResourceField
            label="验证方式"
            hint="要用本地自建解析申请，请选择“本地自建解析”。"
            tooltip="证书机构会要求证明这个域名归你管理。选择 Cloudflare 时，面板会调用 Cloudflare 写入 _acme-challenge 验证记录；选择本地自建解析时，面板会写入左侧“本地自建解析”（权威 DNS）的托管域名。"
          >
            <ResourceSelect
              aria-label="验证方式"
              {...form.register('dns_provider_mode', {
                onChange: (event) => {
                  setDNSProviderModeTouched(true);
                  if (event.target.value === 'authoritative') {
                    form.setValue('dns_account_id', 0, { shouldDirty: true });
                  } else {
                    form.setValue('dns_zone_id_ref', null, {
                      shouldDirty: true,
                    });
                  }
                },
              })}
            >
              <option value="cloudflare">Cloudflare 账号</option>
              <option value="authoritative">本地自建解析</option>
            </ResourceSelect>
          </ResourceField>

          <ResourceField
            label="密钥算法"
            error={form.formState.errors.key_algorithm?.message}
          >
            <ResourceSelect {...form.register('key_algorithm')}>
              <option value="RSA2048">RSA 2048</option>
              <option value="RSA4096">RSA 4096</option>
              <option value="EC256">ECC 256</option>
              <option value="EC384">ECC 384</option>
            </ResourceSelect>
          </ResourceField>
        </div>

        {dnsProviderMode === 'authoritative' ? (
          <ResourceField
            label="本地托管域名"
            hint={authoritativeZoneHint}
            error={form.formState.errors.dns_zone_id_ref?.message}
            tooltip="这里选择左侧“本地自建解析”（权威 DNS）中创建的域名，一般是根域名，例如 example.com。申请 www.example.com 或 *.example.com 证书时，都要选择 example.com。"
            container="div"
          >
            <ResourceSelect
              aria-label="本地托管域名"
              disabled={
                dnsZonesQuery.isLoading ||
                dnsZonesQuery.isError ||
                enabledAuthoritativeZones.length === 0
              }
              {...form.register('dns_zone_id_ref')}
            >
              <option value="">
                {dnsZonesQuery.isLoading
                  ? '正在加载本地托管域名...'
                  : dnsZonesQuery.isError
                    ? '本地托管域名加载失败'
                    : enabledAuthoritativeZones.length === 0
                      ? '暂无本地托管域名'
                      : '请选择本地托管域名'}
              </option>
              {authoritativeZones.map((zone) => (
                <option key={zone.id} value={zone.id} disabled={!zone.enabled}>
                  {zone.name}
                  {matchingAuthoritativeZones.some(
                    (matchedZone) => matchedZone.id === zone.id,
                  )
                    ? '（匹配当前域名）'
                    : ''}
                  {zone.enabled ? '' : '（已停用）'}
                </option>
              ))}
            </ResourceSelect>
            {enabledAuthoritativeZones.length === 0 ? (
              <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-muted)] px-4 py-3 text-xs leading-5 text-[var(--foreground-secondary)]">
                还没有可选择的本地托管域名。
                <Link
                  href="/authoritative-dns"
                  className="ml-1 font-medium text-[var(--brand-primary)] hover:underline"
                >
                  去创建本地托管域名
                </Link>
              </div>
            ) : null}
          </ResourceField>
        ) : (
          <ResourceField
            label="Cloudflare 账号"
            hint="API 密钥需要允许读取域名并修改解析记录。"
            tooltip="Cloudflare 里对应的权限名通常是 Zone Read 和 DNS Edit。"
            error={form.formState.errors.dns_account_id?.message}
          >
            <ResourceSelect {...form.register('dns_account_id')}>
              <option value={0}>请选择 Cloudflare 账号</option>
              {cloudflareAccounts.map((acc) => (
                <option key={acc.id} value={acc.id}>
                  {acc.name}
                </option>
              ))}
            </ResourceSelect>
          </ResourceField>
        )}

        <div className="grid gap-4 md:grid-cols-1">
          <ResourceField
            label="备注"
            error={form.formState.errors.remark?.message}
          >
            <ResourceInput
              placeholder="可选，用于记录证书用途。"
              {...form.register('remark')}
            />
          </ResourceField>

          <ToggleField
            label="开启自动续签"
            description="开启后，将在证书过期前 7 天自动续期。"
            checked={form.watch('auto_renew')}
            onChange={(checked) => form.setValue('auto_renew', checked)}
          />
        </div>

        <div className="overflow-hidden rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)]">
          <button
            type="button"
            className="flex w-full items-center justify-between px-4 py-3 text-sm font-medium text-[var(--foreground-primary)] transition hover:bg-[var(--surface-muted)]"
            onClick={() => setShowAdvanced(!showAdvanced)}
          >
            <span>高级选项</span>
            <svg
              className={`h-4 w-4 transition-transform duration-200 ${showAdvanced ? 'rotate-180' : ''}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 9l-7 7-7-7"
              />
            </svg>
          </button>
          {showAdvanced && (
            <div className="space-y-4 border-t border-[var(--border-default)] px-4 py-4">
              <div className="grid gap-4 md:grid-cols-2">
                <ResourceField
                  label="DNS 验证服务器 1"
                  hint="可选，如 8.8.8.8"
                  error={form.formState.errors.dns1?.message}
                >
                  <ResourceInput
                    placeholder="为空则使用默认验证查询服务器"
                    {...form.register('dns1')}
                  />
                </ResourceField>
                <ResourceField
                  label="DNS 验证服务器 2"
                  hint="可选，如 1.1.1.1"
                  error={form.formState.errors.dns2?.message}
                >
                  <ResourceInput
                    placeholder="为空则使用默认验证查询服务器"
                    {...form.register('dns2')}
                  />
                </ResourceField>
              </div>
              <div className="grid gap-4 md:grid-cols-2">
                <ToggleField
                  label="跳过别名检查"
                  description="如果验证记录通过别名跳到其它域名，开启后不会继续追踪。"
                  tooltip="CNAME 也叫别名记录，会把一个域名指向另一个域名。大多数用户保持关闭即可。"
                  checked={form.watch('disable_cname')}
                  onChange={(checked) =>
                    form.setValue('disable_cname', checked)
                  }
                />
                <ToggleField
                  label="跳过 DNS 前置检查"
                  description="不先在本地确认验证记录是否生效，直接让证书机构验证。"
                  tooltip="一般保持关闭。只有确认本地检查被运营商缓存误判时，才建议临时开启。"
                  checked={form.watch('skip_dns')}
                  onChange={(checked) => form.setValue('skip_dns', checked)}
                />
              </div>
            </div>
          )}
        </div>

        <PrimaryButton type="submit" disabled={applyMutation.isPending}>
          {applyMutation.isPending
            ? '提交中...'
            : mode === 'edit-acme'
              ? '保存并申请'
              : mode === 'convert-upload'
                ? '开始转换'
                : '开始申请'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

function normalizeCertificateDomain(value: string) {
  return value.trim().toLowerCase().replace(/\.$/, '');
}

function parseCertificateDomains(primaryDomain: string, otherDomains: string) {
  const domains = [primaryDomain]
    .concat(otherDomains.split(/[\s,，;；]+/))
    .map(normalizeCertificateDomain)
    .filter(Boolean)
    .map((domain) => domain.replace(/^\*\./, ''));

  return Array.from(new Set(domains));
}

function certificateDomainBelongsToZone(domain: string, zoneName: string) {
  const normalizedDomain = normalizeCertificateDomain(domain);
  const normalizedZoneName = normalizeCertificateDomain(zoneName);
  return (
    normalizedDomain === normalizedZoneName ||
    normalizedDomain.endsWith(`.${normalizedZoneName}`)
  );
}
