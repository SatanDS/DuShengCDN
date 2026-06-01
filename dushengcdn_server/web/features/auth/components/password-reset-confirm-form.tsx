'use client';

import { useMutation } from '@tanstack/react-query';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useState } from 'react';

import { InlineMessage } from '@/components/feedback/inline-message';
import { AppCard } from '@/components/ui/app-card';
import { resetPassword } from '@/features/auth/api/auth';
import {
  AuthButton,
  AuthFormField,
  AuthInput,
  SecondaryButton,
} from '@/features/auth/components/auth-form-primitives';
import { copyToClipboard } from '@/lib/utils/clipboard';

function getCopyErrorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : '复制失败：浏览器拒绝写入剪贴板，请手动复制新密码。';
}

export function PasswordResetConfirmForm() {
  const searchParams = useSearchParams();
  const email = searchParams?.get('email') || '';
  const token = searchParams?.get('token') || '';
  const [resetPasswordValue, setResetPasswordValue] = useState('');
  const [message, setMessage] = useState<{ tone: 'success' | 'danger'; text: string } | null>(null);

  const mutation = useMutation({
    mutationFn: () => resetPassword({ email, token }),
    onSuccess: async (password) => {
      setResetPasswordValue(password);
      try {
        await copyToClipboard(password);
        setMessage({ tone: 'success', text: `密码已重置，新密码已复制到剪贴板：${password}` });
      } catch (error) {
        setMessage({ tone: 'success', text: `密码已重置：${password}。${getCopyErrorMessage(error)}` });
      }
    },
    onError: (error: Error) => {
      setMessage({ tone: 'danger', text: error.message || '密码重置失败，请重新获取链接。' });
    },
  });

  const missingParams = !email || !token;

  return (
    <AppCard title='密码重置确认' description='确认后，系统会生成新的随机密码。'>
      <div className='space-y-4'>
        <AuthFormField label='邮箱地址'>
          <AuthInput value={email} readOnly />
        </AuthFormField>

        {missingParams ? (
          <InlineMessage tone='danger' message='重置链接缺少必要参数，请重新发起密码重置。' />
        ) : null}

        {message ? <InlineMessage tone={message.tone} message={message.text} /> : null}

        <div className='flex flex-col gap-3 sm:flex-row'>
          <AuthButton type='button' disabled={missingParams || mutation.isPending} onClick={() => mutation.mutate()}>
            {mutation.isPending ? '处理中...' : '确认重置密码'}
          </AuthButton>
          {resetPasswordValue ? (
            <SecondaryButton
              type='button'
              onClick={async () => {
                if (resetPasswordValue) {
                  try {
                    await copyToClipboard(resetPasswordValue);
                    setMessage({ tone: 'success', text: `新密码已复制到剪贴板：${resetPasswordValue}` });
                  } catch (error) {
                    setMessage({
                      tone: 'danger',
                      text: `新密码：${resetPasswordValue}。${getCopyErrorMessage(error)}`,
                    });
                  }
                }
              }}
            >
              再次复制密码
            </SecondaryButton>
          ) : null}
        </div>

        <div className='text-sm text-[var(--foreground-secondary)]'>
          处理完成后可返回
          <Link href='/login' className='ml-2 text-[var(--brand-primary)] transition hover:opacity-80'>
            登录页
          </Link>
        </div>
      </div>
    </AppCard>
  );
}
