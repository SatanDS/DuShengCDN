'use client';

import { useMutation } from '@tanstack/react-query';
import { useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';

import { InlineMessage } from '@/components/feedback/inline-message';
import { LoadingState } from '@/components/feedback/loading-state';
import { useAuth } from '@/components/providers/auth-provider';
import { AppCard } from '@/components/ui/app-card';
import { exchangeGitHubCode } from '@/features/auth/api/auth';
import { OAuthCallback } from '@/features/auth/components/oauth-callback';

export function GitHubOAuthCallback() {
  const searchParams = useSearchParams();
  const state = searchParams?.get('state')?.trim() ?? '';
  const legacy = searchParams?.get('legacy')?.trim() === '1';

  if (state && !legacy) {
    return <OAuthCallback />;
  }

  return <LegacyGitHubOAuthCallback />;
}

function LegacyGitHubOAuthCallback() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { setUser } = useAuth();
  const handledRef = useRef<string | null>(null);
  const [prompt, setPrompt] = useState('正在处理 GitHub 授权结果...');
  const [message, setMessage] = useState<{ tone: 'danger' | 'success'; text: string } | null>(null);
  const code = searchParams?.get('code')?.trim() ?? '';
  const state = searchParams?.get('state')?.trim() ?? '';
  const oauthError = searchParams?.get('error')?.trim() ?? '';
  const oauthErrorDescription =
    searchParams?.get('error_description')?.trim() ?? '';

  const mutation = useMutation({
    mutationFn: () => exchangeGitHubCode(code, state),
    onSuccess: (user) => {
      setUser(user);
      setMessage({ tone: 'success', text: '登录成功，正在跳转...' });
      router.replace('/');
    },
    onError: (error: Error) => {
      setPrompt('授权处理失败');
      setMessage({ tone: 'danger', text: error.message || 'GitHub 授权失败，请稍后重试。' });
    },
  });
  const mutateGitHubCode = mutation.mutate;
  const isExchangePending = mutation.isPending;

  useEffect(() => {
    if (oauthError || oauthErrorDescription) {
      setPrompt('授权处理失败');
      setMessage({ tone: 'danger', text: 'GitHub 授权失败，请返回登录页重试。' });
      return;
    }

    if (!code || !state) {
      setPrompt('缺少授权参数');
      setMessage({ tone: 'danger', text: '未收到完整 GitHub 授权参数，请返回登录页重试。' });
      return;
    }

    const key = `${code}:${state}`;
    if (handledRef.current === key) {
      return;
    }

    handledRef.current = key;
    mutateGitHubCode();
  }, [code, mutateGitHubCode, oauthError, oauthErrorDescription, state]);

  return (
    <AppCard title='GitHub OAuth 回调' description={prompt}>
      <div className='space-y-4'>
        {isExchangePending ? <LoadingState /> : null}
        {message ? <InlineMessage tone={message.tone} message={message.text} /> : null}
      </div>
    </AppCard>
  );
}
