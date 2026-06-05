import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AuthProvider } from '@/components/providers/auth-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { DashboardTopbar } from '@/components/layout/dashboard-topbar';

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    replace: vi.fn(),
  }),
}));

describe('DashboardTopbar', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function renderTopbar() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <AuthProvider>
            <DashboardTopbar />
          </AuthProvider>
        </ThemeProvider>
      </QueryClientProvider>,
    );
  }

  it('shows the installed server version from status API', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/user/self')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                id: 1,
                username: 'root',
                display_name: '渡笙',
                role: 100,
                status: 1,
              },
            }),
          );
        }

        if (url.includes('/status')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                version: '1.2.3',
                start_time: 0,
                email_verification: false,
                github_oauth: false,
                github_client_id: '',
                system_name: 'DuShengCDN',
                home_page_link: '',
                footer_html: '',
                wechat_qrcode: '',
                wechat_login: false,
                server_address: '',
                turnstile_check: false,
                turnstile_site_key: '',
                register_enabled: false,
                password_register_enabled: false,
                auth_sources: [],
              },
            }),
          );
        }

        if (url.includes('/update/latest-release?channel=stable')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                tag_name: '1.2.3',
                body: '',
                html_url: '',
                published_at: '2026-05-31T00:00:00Z',
                channel: 'stable',
                prerelease: false,
                current_version: '1.2.3',
                has_update: false,
                upgrade_supported: true,
                in_progress: false,
                upgrade_status: 'idle',
                upgrade_logs: [],
              },
            }),
          );
        }

        if (url.includes('/update/latest-release?channel=preview')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                tag_name: '1.2.4-private.1-gabcdef0',
                body: '',
                html_url: '',
                published_at: '2026-05-31T00:00:00Z',
                channel: 'preview',
                prerelease: true,
                current_version: '1.2.3',
                has_update: true,
                upgrade_supported: true,
                automatic_upgrade_enabled: false,
                in_progress: false,
                upgrade_status: 'idle',
                upgrade_logs: [],
              },
            }),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderTopbar();

    expect(await screen.findByText('版本 1.2.3')).toBeInTheDocument();
  });
});
