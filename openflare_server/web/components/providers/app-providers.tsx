'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { type ReactNode, useState } from 'react';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { AuthProvider } from '@/components/providers/auth-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';

interface AppProvidersProps {
  children: ReactNode;
}

export function AppProviders({ children }: AppProvidersProps) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            refetchOnWindowFocus: false,
            retry: 1,
            staleTime: 30_000,
          },
          mutations: {
            retry: 0,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <ToastProvider>
            <ConfirmDialogProvider>{children}</ConfirmDialogProvider>
          </ToastProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
