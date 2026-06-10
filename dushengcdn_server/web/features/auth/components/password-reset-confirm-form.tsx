'use client';

import { useMutation } from '@tanstack/react-query';
import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';

import { InlineMessage } from '@/components/feedback/inline-message';
import { AppCard } from '@/components/ui/app-card';
import { resetPassword } from '@/features/auth/api/auth';
import {
  AuthButton,
  AuthFormField,
  AuthInput,
} from '@/features/auth/components/auth-form-primitives';

function readResetParams() {
  if (typeof window === 'undefined') {
    return { email: '', token: '' };
  }
  const hash = window.location.hash.startsWith('#')
    ? window.location.hash.slice(1)
    : window.location.hash;
  const search = window.location.search.startsWith('?')
    ? window.location.search.slice(1)
    : window.location.search;
  const params = new URLSearchParams(hash || search);
  return {
    email: params.get('email') || '',
    token: params.get('token') || '',
  };
}

export function PasswordResetConfirmForm() {
  const [params, setParams] = useState({ email: '', token: '' });
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [message, setMessage] = useState<{
    tone: 'success' | 'danger';
    text: string;
  } | null>(null);

  useEffect(() => {
    const nextParams = readResetParams();
    setParams(nextParams);
    if (
      typeof window !== 'undefined' &&
      (window.location.hash || window.location.search)
    ) {
      window.history.replaceState(null, '', window.location.pathname);
    }
  }, []);

  const missingParams = !params.email || !params.token;
  const passwordMismatch =
    newPassword !== '' &&
    confirmPassword !== '' &&
    newPassword !== confirmPassword;
  const passwordInvalid =
    newPassword.length > 0 &&
    (newPassword.length < 8 || newPassword.length > 20);
  const canSubmit = useMemo(
    () =>
      !missingParams &&
      !passwordMismatch &&
      !passwordInvalid &&
      newPassword !== '' &&
      confirmPassword !== '',
    [
      confirmPassword,
      missingParams,
      newPassword,
      passwordInvalid,
      passwordMismatch,
    ],
  );

  const mutation = useMutation({
    mutationFn: () =>
      resetPassword({
        email: params.email,
        token: params.token,
        new_password: newPassword,
      }),
    onSuccess: () => {
      setNewPassword('');
      setConfirmPassword('');
      setMessage({ tone: 'success', text: '密码已重置，请使用新密码登录。' });
    },
    onError: (error: Error) => {
      setMessage({
        tone: 'danger',
        text: error.message || '密码重置失败，请重新获取链接。',
      });
    },
  });

  return (
    <AppCard title="密码重置确认" description="请输入新的登录密码。">
      <div className="space-y-4">
        <AuthFormField label="邮箱地址">
          <AuthInput value={params.email} readOnly />
        </AuthFormField>
        <AuthFormField label="新密码" hint="长度 8-20 个字符。">
          <AuthInput
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
          />
        </AuthFormField>
        <AuthFormField label="确认新密码">
          <AuthInput
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
        </AuthFormField>

        {missingParams ? (
          <InlineMessage
            tone="danger"
            message="重置链接缺少必要参数，请重新发起密码重置。"
          />
        ) : null}
        {passwordInvalid ? (
          <InlineMessage tone="danger" message="密码长度需要 8-20 个字符。" />
        ) : null}
        {passwordMismatch ? (
          <InlineMessage tone="danger" message="两次输入的新密码不一致。" />
        ) : null}
        {message ? (
          <InlineMessage tone={message.tone} message={message.text} />
        ) : null}

        <AuthButton
          type="button"
          disabled={!canSubmit || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? '处理中...' : '确认重置密码'}
        </AuthButton>

        <div className="text-sm text-[var(--foreground-secondary)]">
          处理完成后可返回
          <Link
            href="/login"
            className="ml-2 text-[var(--brand-primary)] transition hover:opacity-80"
          >
            登录页
          </Link>
        </div>
      </div>
    </AppCard>
  );
}
