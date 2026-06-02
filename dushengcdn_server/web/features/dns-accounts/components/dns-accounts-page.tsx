'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToastFeedback } from '@/components/feedback/toast-provider';
import { PageHeader } from '@/components/layout/page-header';
import { AppCard } from '@/components/ui/app-card';
import { AppModal } from '@/components/ui/app-modal';
import {
  deleteDnsAccount,
  getDnsAccounts,
  createDnsAccount,
} from '@/features/dns-accounts/api/dns-accounts';
import type { DnsAccountItem } from '@/features/dns-accounts/types';
import { getErrorMessage } from '@/features/websites/utils';
import {
  DangerButton,
  PrimaryButton,
  ResourceField,
  ResourceInput,
  ResourceSelect,
} from '@/features/shared/components/resource-primitives';
import { formatDateTime } from '@/lib/utils/date';

export function DnsAccountsPage() {
  const queryClient = useQueryClient();
  const { setFeedback } = useToastFeedback<{ tone: 'info' | 'success' | 'danger'; message: string }>();
  const confirmDialog = useConfirmDialog();
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  const dnsAccountsQuery = useQuery({
    queryKey: ['dns-accounts'],
    queryFn: getDnsAccounts,
  });

  const deleteMutation = useMutation({
    mutationFn: deleteDnsAccount,
    onSuccess: async () => {
      setFeedback({ tone: 'success', message: 'Cloudflare 账号已删除。' });
      await queryClient.invalidateQueries({ queryKey: ['dns-accounts'] });
    },
    onError: (error) => {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    },
  });

  const handleDelete = async (account: DnsAccountItem) => {
    const confirmed = await confirmDialog({
      title: '删除 Cloudflare 账号',
      message: `确认删除 Cloudflare 账号“${account.name}”吗？`,
      confirmLabel: '删除',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    setFeedback(null);
    deleteMutation.mutate(account.id);
  };

  const accounts = useMemo(() => dnsAccountsQuery.data ?? [], [dnsAccountsQuery.data]);

  return (
    <>
      <div className="space-y-6">
        <PageHeader
          title="Cloudflare 账号"
          description="统一管理 Cloudflare 授权，用于申请证书时写验证记录，以及同步 Cloudflare 解析。"
          action={
            <div className="flex flex-wrap gap-3">
              <PrimaryButton type="button" onClick={() => setIsCreateOpen(true)}>
                添加账号
              </PrimaryButton>
            </div>
          }
        />

        <AppCard title="Cloudflare 账号列表">
          {dnsAccountsQuery.isLoading ? (
            <LoadingState />
          ) : dnsAccountsQuery.isError ? (
            <ErrorState title="加载失败" description={getErrorMessage(dnsAccountsQuery.error)} />
          ) : accounts.length === 0 ? (
            <EmptyState title="暂无 Cloudflare 账号" description="点击右上角“添加账号”开始录入。" />
          ) : (
            <div className="space-y-3">
              {accounts.map((account) => (
                <div key={account.id} className="rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="space-y-2">
                      <p className="text-sm font-semibold text-[var(--foreground-primary)]">
                        {account.name} <span className="ml-2 text-xs font-normal text-[var(--foreground-secondary)]">({account.type})</span>
                      </p>
                      <div className="text-xs leading-5 text-[var(--foreground-secondary)]">
                        <p>创建于：{formatDateTime(account.created_at)}</p>
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <DangerButton
                        type="button"
                        onClick={() => handleDelete(account)}
                        disabled={deleteMutation.isPending}
                        className="px-3 py-2 text-xs"
                      >
                        删除
                      </DangerButton>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </AppCard>
      </div>
      
      {isCreateOpen && (
        <DnsAccountCreateModal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} onCreated={() => {
          setFeedback({ tone: 'success', message: 'Cloudflare 账号已添加。' });
          setIsCreateOpen(false);
          queryClient.invalidateQueries({ queryKey: ['dns-accounts'] });
        }} />
      )}
    </>
  );
}

function DnsAccountCreateModal({ isOpen, onClose, onCreated }: { isOpen: boolean; onClose: () => void; onCreated: () => void }) {
  const [error, setError] = useState('');
  const { register, handleSubmit, formState } = useForm({
    defaultValues: { name: '', type: 'cloudflare', authorization: '' },
  });

  const createMutation = useMutation({
    mutationFn: createDnsAccount,
    onSuccess: onCreated,
    onError: (err) => setError(getErrorMessage(err)),
  });

  const onSubmit = handleSubmit((values) => {
    setError('');
    createMutation.mutate({ ...values, authorization: values.authorization.trim() });
  });

  return (
    <AppModal isOpen={isOpen} onClose={onClose} title="添加 Cloudflare 账号">
      <form onSubmit={onSubmit} className="space-y-5">
        {error && <InlineMessage tone="danger" message={error} />}
        <ResourceField label="账号名称" error={formState.errors.name?.message as string}>
          <ResourceInput placeholder="Cloudflare 账号" {...register('name', { required: '请输入名称' })} />
        </ResourceField>
        <ResourceField
          label="服务商"
          tooltip="当前这里用于保存 Cloudflare 授权。后续如果支持其它 DNS 服务商，再在这里扩展。"
        >
          <ResourceSelect {...register('type')}>
            <option value="cloudflare">Cloudflare</option>
          </ResourceSelect>
        </ResourceField>
        <ResourceField
          label="API 密钥"
          hint="请填写 Cloudflare 里创建的 API 密钥，不要填写全局密钥。权限至少包含读取域名和修改解析记录。"
          tooltip="API 密钥就是面板代你去 Cloudflare 添加、修改解析记录时使用的授权。建议只授权需要管理的域名。"
        >
          <ResourceInput {...register('authorization', { required: '请输入 API 密钥' })} />
        </ResourceField>
        <PrimaryButton type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? '提交中...' : '提交'}
        </PrimaryButton>
      </form>
    </AppModal>
  );
}
