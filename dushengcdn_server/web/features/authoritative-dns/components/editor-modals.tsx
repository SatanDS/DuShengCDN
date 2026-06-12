'use client';
import { useMutation } from '@tanstack/react-query';
import { Minus, Plus } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { InlineMessage } from '@/components/feedback/inline-message';
import { AppModal } from '@/components/ui/app-modal';
import { StatusBadge } from '@/components/ui/status-badge';
import {
  createDNSWorker,
  createDNSZone,
  createDNSZoneRecord,
  updateDNSRecord,
  updateDNSZone,
} from '@/features/authoritative-dns/api/authoritative-dns';
import type {
  DNSRecordItem,
  DNSRecordMutationPayload,
  DNSWorkerHealthItem,
  DNSWorkerItem,
  DNSZoneItem,
  DNSZoneMutationPayload,
} from '@/features/authoritative-dns/types';
import { getErrorMessage } from '@/features/proxy-routes/helpers';
import {
  CodeBlock,
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';
import { copyToClipboard } from '@/lib/utils/clipboard';
import { shellQuote } from '@/lib/utils/shell';
import type {
  ZoneFormValues,
  RecordFormValues,
  WorkerFormValues,
  WorkerSettingsFormValues,
} from './authoritative-dns-page.helpers';
import {
  getDNSWorkerDisplayName,
  dnsRecordTypes,
  linesFromText,
  zoneToFormValues,
  recordToFormValues,
  isAddressRecordType,
  isPriorityRecordType,
  getRecordValueLabel,
  getRecordValueHint,
  getRecordValuePlaceholder,
} from './authoritative-dns-page.helpers';

export function ZoneEditorModal({
  isOpen,
  zone,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  zone: DNSZoneItem | null;
  onClose: () => void;
  onSaved: (zone: DNSZoneItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<ZoneFormValues>({
    defaultValues: zoneToFormValues(zone),
  });
  const saveMutation = useMutation({
    mutationFn: (values: ZoneFormValues) => {
      const payload: DNSZoneMutationPayload = {
        name: values.name.trim(),
        soa_email: values.soa_email.trim(),
        primary_ns: values.primary_ns.trim(),
        name_servers: linesFromText(values.name_servers_text),
        default_ttl: values.default_ttl,
        enabled: values.enabled,
      };
      return zone ? updateDNSZone(zone.id, payload) : createDNSZone(payload);
    },
    onSuccess: onSaved,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset(zoneToFormValues(zone));
    }
  }, [form, isOpen, zone]);

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={zone ? '编辑托管域名' : '创建托管域名'}
      description="托管域名保存后会规范化为根域名格式；生产环境建议至少填写两个可公网访问的 DNS 响应端名称。"
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          saveMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <ResourceField
          label="托管域名"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="example.com"
            {...form.register('name', { required: '请输入托管域名' })}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="基础邮箱"
            hint="留空时后端使用 hostmaster@托管域名。"
          >
            <ResourceInput
              placeholder="hostmaster@example.com"
              {...form.register('soa_email')}
            />
          </ResourceField>
          <ResourceField
            label="主解析服务器"
            hint="留空时默认使用 NS 列表第一项。"
          >
            <ResourceInput
              placeholder="ns1.example.net"
              {...form.register('primary_ns')}
            />
          </ResourceField>
        </div>
        <ResourceField
          label="注册商 NS 列表"
          hint="每行一个 NS，也可用逗号或分号分隔；这些值需要填写到域名注册商后台。"
          tooltip="NS 是注册商后台里的“域名服务器”。这里填 ns1.example.net、ns2.example.net 这类地址后，还要去注册商把域名指向它们。"
        >
          <ResourceTextarea
            placeholder={'ns1.example.net\nns2.example.net'}
            {...form.register('name_servers_text')}
          />
        </ResourceField>
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField label="默认缓存时间">
            <ResourceInput
              type="number"
              min={1}
              max={86400}
              {...form.register('default_ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ToggleField
            label="启用托管域名"
            description="停用后不会下发给 DNS 响应端。"
            checked={form.watch('enabled')}
            onChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
        </div>
        <PrimaryButton type="submit" disabled={saveMutation.isPending}>
          {saveMutation.isPending ? '保存中...' : '保存'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

export function RecordEditorModal({
  isOpen,
  zone,
  record,
  onClose,
  onSaved,
}: {
  isOpen: boolean;
  zone: DNSZoneItem;
  record: DNSRecordItem | null;
  onClose: () => void;
  onSaved: (record: DNSRecordItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<RecordFormValues>({
    defaultValues: recordToFormValues(record),
  });
  const recordType = form.watch('type');
  const ipValues = form.watch('ip_values');
  const isAddressRecord = isAddressRecordType(recordType);
  const valuePlaceholder = getRecordValuePlaceholder(recordType);
  const saveMutation = useMutation({
    mutationFn: (values: RecordFormValues) => {
      const basePayload: DNSRecordMutationPayload = {
        zone_id: zone.id,
        name: values.name.trim(),
        type: values.type,
        value: values.value.trim(),
        ttl: values.ttl,
        priority: isPriorityRecordType(values.type) ? values.priority : 0,
        enabled: values.enabled,
      };
      if (isAddressRecordType(values.type)) {
        const addresses = (values.ip_values ?? [])
          .map((item) => item.trim())
          .filter(Boolean);
        if (addresses.length === 0) {
          throw new Error(
            values.type === 'A' ? '请输入 IPv4 地址' : '请输入 IPv6 地址',
          );
        }
        if (record) {
          return updateDNSRecord(record.id, {
            ...basePayload,
            value: addresses[0],
          });
        }
        return Promise.all(
          addresses.map((address) =>
            createDNSZoneRecord(zone.id, {
              ...basePayload,
              value: address,
            }),
          ),
        ).then((records) => records[0]);
      }
      return record
        ? updateDNSRecord(record.id, basePayload)
        : createDNSZoneRecord(zone.id, basePayload);
    },
    onSuccess: onSaved,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset(recordToFormValues(record));
    }
  }, [form, isOpen, record]);

  useEffect(() => {
    if (!isAddressRecord) {
      return;
    }
    const current = form.getValues('ip_values') ?? [];
    if (current.length === 0) {
      form.setValue('ip_values', [''], { shouldDirty: false });
    }
  }, [form, isAddressRecord, recordType]);

  const updateIPAddressValue = (index: number, value: string) => {
    const current = form.getValues('ip_values') ?? [''];
    const next = current.length > 0 ? [...current] : [''];
    next[index] = value;
    form.setValue('ip_values', next, { shouldDirty: true });
  };

  const addIPAddressValue = () => {
    const current = form.getValues('ip_values') ?? [''];
    form.setValue('ip_values', [...current, ''], { shouldDirty: true });
  };

  const removeIPAddressValue = (index: number) => {
    const current = form.getValues('ip_values') ?? [''];
    const next = current.filter((_, itemIndex) => itemIndex !== index);
    form.setValue('ip_values', next.length > 0 ? next : [''], {
      shouldDirty: true,
    });
  };

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title={record ? '编辑 DNS 记录' : '新增 DNS 记录'}
      description={`当前托管域名：${zone.name}。记录名可填写 @、完整域名，或填写 www 这类相对名称。`}
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          saveMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <div className="grid gap-5 md:grid-cols-2">
          <ResourceField
            label="记录名"
            hint="@ 表示当前托管域名根域。"
            error={form.formState.errors.name?.message}
          >
            <ResourceInput
              placeholder="@"
              {...form.register('name', { required: '请输入记录名' })}
            />
          </ResourceField>
          <ResourceField label="记录类型">
            <ResourceSelect {...form.register('type')}>
              {dnsRecordTypes.map((type) => (
                <option key={type} value={type}>
                  {type}
                </option>
              ))}
            </ResourceSelect>
          </ResourceField>
        </div>
        {isAddressRecord ? (
          <ResourceField
            label={getRecordValueLabel(recordType)}
            hint={getRecordValueHint(recordType)}
            container="div"
          >
            <div className="space-y-3">
              {(ipValues?.length ? ipValues : ['']).map((value, index) => (
                <div
                  key={index}
                  className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]"
                >
                  <ResourceInput
                    value={value}
                    placeholder={valuePlaceholder}
                    onChange={(event) =>
                      updateIPAddressValue(index, event.target.value)
                    }
                  />
                  <div className="flex gap-2">
                    {!record && index === (ipValues?.length ?? 1) - 1 ? (
                      <SecondaryButton
                        type="button"
                        aria-label="增加 IP 地址"
                        title="增加 IP 地址"
                        className="h-12 w-12 shrink-0 px-0"
                        onClick={addIPAddressValue}
                      >
                        <Plus className="h-4 w-4" aria-hidden="true" />
                      </SecondaryButton>
                    ) : null}
                    {!record && (ipValues?.length ?? 1) > 1 ? (
                      <SecondaryButton
                        type="button"
                        aria-label="删除 IP 地址"
                        title="删除 IP 地址"
                        className="h-12 w-12 shrink-0 px-0"
                        onClick={() => removeIPAddressValue(index)}
                      >
                        <Minus className="h-4 w-4" aria-hidden="true" />
                      </SecondaryButton>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          </ResourceField>
        ) : (
          <ResourceField
            label={getRecordValueLabel(recordType)}
            hint={getRecordValueHint(recordType)}
            error={form.formState.errors.value?.message}
          >
            <ResourceTextarea
              placeholder={valuePlaceholder}
              {...form.register('value', { required: '请输入记录内容' })}
            />
          </ResourceField>
        )}
        <div className="grid gap-5 md:grid-cols-3">
          <ResourceField
            label="缓存时间"
            hint="0 表示使用托管域名默认缓存时间。"
          >
            <ResourceInput
              type="number"
              min={0}
              max={86400}
              {...form.register('ttl', { valueAsNumber: true })}
            />
          </ResourceField>
          <ResourceField
            label="记录优先级"
            hint={
              isPriorityRecordType(recordType)
                ? recordType === 'MX'
                  ? '同一域名有多个 MX 时，邮件会先投递到数字更小的服务器，常见主服务器填 10，备用服务器填 20。'
                  : '对 SRV、HTTPS 和 SVCB 生效；记录值里不要重复填写最前面的优先级数字。'
                : '仅 MX、SRV、HTTPS 和 SVCB 需要填写优先级；其它记录会自动保存为 0。'
            }
          >
            <ResourceInput
              type="number"
              min={0}
              disabled={!isPriorityRecordType(recordType)}
              {...form.register('priority', { valueAsNumber: true })}
            />
          </ResourceField>
          <ToggleField
            label="启用记录"
            description="停用后不会下发给 DNS 响应端。"
            checked={form.watch('enabled')}
            onChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
        </div>
        <PrimaryButton type="submit" disabled={saveMutation.isPending}>
          {saveMutation.isPending ? '保存中...' : '保存'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

export function WorkerCreateModal({
  isOpen,
  onClose,
  onCreated,
}: {
  isOpen: boolean;
  onClose: () => void;
  onCreated: (worker: DNSWorkerItem) => void;
}) {
  const [error, setError] = useState('');
  const form = useForm<WorkerFormValues>({
    defaultValues: {
      name: '',
      public_address: '',
      remark: '',
    },
  });
  const createMutation = useMutation({
    mutationFn: (values: WorkerFormValues) =>
      createDNSWorker({
        name: values.name.trim(),
        public_address: values.public_address.trim(),
        remark: values.remark.trim(),
      }),
    onSuccess: onCreated,
    onError: (err) => setError(getErrorMessage(err)),
  });

  useEffect(() => {
    if (isOpen) {
      setError('');
      form.reset({ name: '', public_address: '', remark: '' });
    }
  }, [form, isOpen]);

  return (
    <AppModal
      isOpen={isOpen}
      onClose={onClose}
      title="创建 DNS 响应端"
      description="响应端密钥只会在创建后返回一次；请在弹窗中复制部署命令。"
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          setError('');
          createMutation.mutate(values);
        })}
      >
        {error ? <InlineMessage tone="danger" message={error} /> : null}
        <ResourceField
          label="响应端名称"
          error={form.formState.errors.name?.message}
        >
          <ResourceInput
            placeholder="ns1-hk"
            {...form.register('name', { required: '请输入响应端名称' })}
          />
        </ResourceField>
        <ResourceField
          label="公网地址"
          hint="可填写 ns1.example.net 或 203.0.113.10，便于管理端展示和排障。"
        >
          <ResourceInput
            placeholder="ns1.example.net"
            {...form.register('public_address')}
          />
        </ResourceField>
        <ResourceField label="显示名称" hint="可选，用于替换卡片标题。">
          <ResourceTextarea
            maxLength={255}
            placeholder="例如：香港阿里云 / 主响应端"
            {...form.register('remark')}
          />
        </ResourceField>
        <PrimaryButton type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? '创建中...' : '创建'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}

export function WorkerSettingsModal({
  worker,
  isSaving,
  isRequestingUpdate,
  isDeleting,
  onClose,
  onSave,
  onRequestUpdate,
  onDelete,
}: {
  worker: DNSWorkerHealthItem;
  isSaving: boolean;
  isRequestingUpdate: boolean;
  isDeleting: boolean;
  onClose: () => void;
  onSave: (values: WorkerSettingsFormValues) => void;
  onRequestUpdate: () => void;
  onDelete: () => void;
}) {
  const form = useForm<WorkerSettingsFormValues>({
    defaultValues: {
      remark: worker.remark ?? '',
    },
  });
  const isWaitingForUnsupportedUpdate =
    worker.update_requested && !worker.update_supported;
  const deleteDisabled = isDeleting || !worker.uninstall_supported;
  const dispatchMode = worker.update_dispatch_mode || '';
  const hasAgentDispatch =
    dispatchMode === 'agent_ws' ||
    dispatchMode === 'agent_heartbeat' ||
    dispatchMode === 'agent_heartbeat_sent';
  const updateDisabled =
    isRequestingUpdate || (worker.update_requested && !hasAgentDispatch);
  const dispatchBadge =
    dispatchMode === 'agent_ws'
      ? { label: 'Agent 已立即下发', variant: 'success' as const }
      : dispatchMode === 'agent_heartbeat_sent'
        ? { label: '已随 Agent 心跳下发', variant: 'success' as const }
        : dispatchMode === 'agent_heartbeat'
          ? { label: '等待 Agent 心跳', variant: 'info' as const }
          : dispatchMode === 'worker_heartbeat'
            ? { label: '回退响应端心跳', variant: 'warning' as const }
            : {
                label: worker.update_supported
                  ? '支持远程更新'
                  : '需先手动升级',
                variant: worker.update_supported
                  ? ('success' as const)
                  : ('warning' as const),
              };
  const updateButtonLabel = isRequestingUpdate
    ? '下发中...'
    : worker.update_requested
      ? hasAgentDispatch
        ? '再次由 Agent 执行'
        : isWaitingForUnsupportedUpdate
          ? '需先手动升级'
          : '等待响应端心跳'
      : '由 Agent 执行更新';

  useEffect(() => {
    form.reset({ remark: worker.remark ?? '' });
  }, [form, worker.id, worker.remark]);

  return (
    <AppModal
      isOpen
      onClose={onClose}
      title="DNS 响应端设置"
      description={`${getDNSWorkerDisplayName(worker)} · ${worker.public_address || worker.worker_id}`}
    >
      <form
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => onSave(values))}
      >
        <ResourceField
          label="显示名称"
          hint="用于替换卡片标题；留空时显示创建时的响应端名称。不会改变密钥或 NS 配置。"
        >
          <ResourceTextarea
            maxLength={255}
            placeholder="例如：香港阿里云 / 主响应端"
            {...form.register('remark')}
          />
        </ResourceField>
        <div className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                强制更新
              </p>
              <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                优先匹配同机 Agent 直接执行安装器；未匹配到 Agent
                时回退为响应端心跳更新。若要走
                Agent，请把公网地址填成该机器的公网 IP。
              </p>
            </div>
            <StatusBadge
              label={dispatchBadge.label}
              variant={dispatchBadge.variant}
            />
          </div>
          {worker.update_dispatch_message ? (
            <InlineMessage
              className="mt-3"
              tone={dispatchMode === 'worker_heartbeat' ? 'warning' : 'info'}
              message={worker.update_dispatch_message}
            />
          ) : null}
          {isWaitingForUnsupportedUpdate ? (
            <InlineMessage
              className="mt-3"
              tone="warning"
              message={
                hasAgentDispatch
                  ? '已交给同机 Agent 执行；响应端当前版本未声明自更新能力，等待 Agent 侧安装器完成后重新心跳上报。'
                  : '该响应端已有待执行更新，但当前版本未声明支持远程自更新。'
              }
            />
          ) : null}
          <SecondaryButton
            type="button"
            className="mt-4"
            disabled={updateDisabled}
            onClick={onRequestUpdate}
          >
            {updateButtonLabel}
          </SecondaryButton>
        </div>
        <div className="rounded-2xl border border-red-500/30 bg-red-500/10 px-4 py-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                删除并卸载
              </p>
              <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                删除后面板会隐藏该响应端，并等待它下一次心跳执行本机卸载脚本，清理服务、定时器和本地数据目录。
              </p>
            </div>
            <StatusBadge
              label={
                worker.uninstall_supported ? '支持远程卸载' : '需先强制更新'
              }
              variant={worker.uninstall_supported ? 'success' : 'warning'}
            />
          </div>
          {!worker.uninstall_supported ? (
            <InlineMessage
              className="mt-3"
              tone="warning"
              message="该响应端尚未上报卸载脚本能力。请先使用上方“强制下发更新”，或登录机器手动执行 uninstall-dns-worker.sh。"
            />
          ) : null}
          <DangerButton
            type="button"
            className="mt-4"
            disabled={deleteDisabled}
            onClick={onDelete}
          >
            {isDeleting ? '删除中...' : '删除并卸载响应端'}
          </DangerButton>
        </div>
        <div className="flex flex-wrap justify-end gap-3">
          <SecondaryButton type="button" onClick={onClose}>
            取消
          </SecondaryButton>
          <PrimaryButton type="submit" disabled={isSaving}>
            {isSaving ? '保存中...' : '保存显示名称'}
          </PrimaryButton>
        </div>
      </form>
    </AppModal>
  );
}

export function WorkerTokenModal({
  worker,
  serverUrl,
  title,
  description,
  onClose,
}: {
  worker: DNSWorkerItem;
  serverUrl: string;
  title: string;
  description: string;
  onClose: () => void;
}) {
  const [copyFeedback, setCopyFeedback] = useState<{
    tone: 'success' | 'danger';
    message: string;
  } | null>(null);
  const token = worker.token ?? '';
  const quotedServerUrl = shellQuote(serverUrl);
  const quotedWorkerId = shellQuote(worker.worker_id);
  const installCommand = `token_file="$(mktemp)"
chmod 600 "$token_file"
trap 'stty echo 2>/dev/null || true; rm -f "$token_file"' EXIT
printf 'DNS Worker token: ' >&2
stty -echo 2>/dev/null || true
IFS= read -r dns_worker_token
stty echo 2>/dev/null || true
printf '\\n' >&2
printf '%s\\n' "$dns_worker_token" > "$token_file"
unset dns_worker_token
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \\
  --server-url ${quotedServerUrl} \\
  --worker-id ${quotedWorkerId} \\
  --token-file "$token_file" \\
  --source-database-profile full \\
  --query-rate-limit 200 \\
  --udp-response-size 1232
rm -f "$token_file"
trap - EXIT`;
  const dockerCommand = `secret_dir="\${XDG_CONFIG_HOME:-$HOME/.config}/dushengcdn-dns-worker"
mkdir -p "$secret_dir"
chmod 700 "$secret_dir"
token_file="$secret_dir/dns-worker-token"
printf 'DNS Worker token: ' >&2
stty -echo 2>/dev/null || true
IFS= read -r dns_worker_token
stty echo 2>/dev/null || true
printf '\\n' >&2
printf '%s\\n' "$dns_worker_token" > "$token_file"
chmod 600 "$token_file"
unset dns_worker_token
docker run -d --name dushengcdn-dns-worker --restart unless-stopped \\
  -p 53:53/udp -p 53:53/tcp \\
  -v dushengcdn-dns-worker-data:/data \\
  -v "$token_file":/run/secrets/dushengcdn_dns_worker_token:ro \\
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=${quotedServerUrl} \\
  -e DUSHENGCDN_DNS_WORKER_ID=${quotedWorkerId} \\
  -e DUSHENGCDN_DNS_WORKER_TOKEN_FILE=/run/secrets/dushengcdn_dns_worker_token \\
  -e DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=/data/geoip/GeoLite2-Country.mmdb \\
  -e DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=/data/geoip/GeoLite2-ASN.mmdb \\
  -e DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=/data/operator-cidr \\
  -e DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200 \\
  -e DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=1232 \\
  ghcr.io/satands/dushengcdn-dns-worker:latest`;
  const sourceCommand = `token_file="$(mktemp)"
chmod 600 "$token_file"
trap 'stty echo 2>/dev/null || true; rm -f "$token_file"' EXIT
printf 'DNS Worker token: ' >&2
stty -echo 2>/dev/null || true
IFS= read -r dns_worker_token
stty echo 2>/dev/null || true
printf '\\n' >&2
printf '%s\\n' "$dns_worker_token" > "$token_file"
unset dns_worker_token
cd dushengcdn_server
go run ./cmd/dns-worker \\
  --server-url ${quotedServerUrl} \\
  --token-file "$token_file" \\
  --listen :53 \\
  --snapshot-path /var/lib/dushengcdn-dns-worker/snapshot.json \\
  --geoip-database /var/lib/dushengcdn-dns-worker/geoip/GeoLite2-Country.mmdb \\
  --asn-database /var/lib/dushengcdn-dns-worker/geoip/GeoLite2-ASN.mmdb \\
  --operator-cidr-database /var/lib/dushengcdn-dns-worker/operator-cidr \\
  --query-rate-limit 200 \\
  --udp-response-size 1232
rm -f "$token_file"
trap - EXIT`;

  const handleCopy = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setCopyFeedback({ tone: 'success', message });
    } catch (error) {
      setCopyFeedback({ tone: 'danger', message: getErrorMessage(error) });
    }
  };

  return (
    <AppModal
      isOpen
      onClose={onClose}
      title={title}
      description={description}
      size="xl"
    >
      <div className="space-y-5">
        {copyFeedback ? (
          <InlineMessage
            tone={copyFeedback.tone}
            message={copyFeedback.message}
          />
        ) : null}
        <ResourceField
          label="响应端密钥"
          tooltip="这是 DNS 响应端连接面板用的专属密钥，只在创建后显示一次；不是节点 Agent 密钥，也不是登录密码。"
        >
          <div className="flex flex-col gap-3 md:flex-row">
            <ResourceInput readOnly value={token} className="font-mono" />
            <SecondaryButton
              type="button"
              onClick={() => void handleCopy(token, '响应端密钥已复制。')}
            >
              复制密钥
            </SecondaryButton>
          </div>
        </ResourceField>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              安装脚本命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(installCommand, '安装脚本命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {installCommand}
          </CodeBlock>
        </div>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              Docker 部署命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(dockerCommand, 'Docker 命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {dockerCommand}
          </CodeBlock>
        </div>
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-sm font-semibold text-[var(--foreground-primary)]">
              源码运行命令
            </h3>
            <SecondaryButton
              type="button"
              onClick={() =>
                void handleCopy(sourceCommand, '源码运行命令已复制。')
              }
            >
              复制命令
            </SecondaryButton>
          </div>
          <CodeBlock className="break-all whitespace-pre-wrap">
            {sourceCommand}
          </CodeBlock>
        </div>
      </div>
    </AppModal>
  );
}
