import type { Metadata } from 'next';
import Script from 'next/script';
import type { ReactNode } from 'react';

import { AppProviders } from '@/components/providers/app-providers';
import { getThemeInitScript } from '@/lib/theme/theme';

import './globals.css';

export const metadata: Metadata = {
  title: {
    default: 'DuShengCDN 控制台',
    template: '%s | DuShengCDN',
  },
  description: 'DuShengCDN 管理端',
  applicationName: 'DuShengCDN',
};

interface RootLayoutProps {
  children: ReactNode;
}

export default function RootLayout({ children }: RootLayoutProps) {
  return (
    <html lang="zh-CN" suppressHydrationWarning className="font-sans">
      <body>
        <Script id="theme-init" strategy="beforeInteractive">
          {getThemeInitScript()}
        </Script>
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
