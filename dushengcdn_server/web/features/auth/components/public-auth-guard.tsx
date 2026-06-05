'use client';

import type { ReactNode } from 'react';
import { useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

import { LoadingState } from '@/components/feedback/loading-state';
import { useAuth } from '@/components/providers/auth-provider';
import { normalizeInternalRedirect } from '@/lib/utils/redirect';

interface PublicAuthGuardProps {
  children: ReactNode;
}

export function PublicAuthGuard({ children }: PublicAuthGuardProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { isAuthenticated, isLoading } = useAuth();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.replace(normalizeInternalRedirect(searchParams?.get('redirect')));
    }
  }, [isAuthenticated, isLoading, router, searchParams]);

  if (isLoading) {
    return <LoadingState />;
  }

  if (isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
