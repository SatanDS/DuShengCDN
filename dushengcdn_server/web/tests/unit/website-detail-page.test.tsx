import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { WebsiteDetailPage } from '@/features/websites/components/website-detail-page';

const pushMock = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

function renderWithProviders(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <ConfirmDialogProvider>{ui}</ConfirmDialogProvider>
      </ToastProvider>
    </QueryClientProvider>,
  );
}

describe('WebsiteDetailPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    pushMock.mockReset();
  });

  it('opens certificate application with local authoritative DNS selection', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/managed-domains/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [
                {
                  id: 4,
                  domain: 'www.example.com',
                  cert_id: null,
                  enabled: true,
                  remark: '',
                  created_at: '2026-06-01T00:00:00Z',
                  updated_at: '2026-06-01T00:00:00Z',
                },
              ],
            }),
          );
        }

        if (url.includes('/proxy-routes/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [
                {
                  id: 9,
                  site_name: 'www-site',
                  domain: 'www.example.com',
                  domains: ['www.example.com'],
                  primary_domain: 'www.example.com',
                  domain_count: 1,
                  origin_url: 'http://127.0.0.1:8080',
                  origin_host: '',
                  enabled: true,
                  enable_https: false,
                  cert_id: null,
                  dns_provider_mode: 'authoritative',
                  dns_zone_id_ref: 11,
                  remark: '',
                },
              ],
            }),
          );
        }

        if (url.includes('/tls-certificates/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [],
            }),
          );
        }

        if (url.includes('/acme-accounts/default')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                id: 3,
                email: 'admin@example.com',
                url: 'https://acme-v02.api.letsencrypt.org/directory',
                created_at: '',
                updated_at: '',
              },
            }),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
            }),
          );
        }

        if (url.includes('/dns-zones/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [
                {
                  id: 11,
                  name: 'example.com',
                  soa_email: 'admin.example.com',
                  primary_ns: 'ns1.example.com',
                  name_servers: ['ns1.example.com'],
                  default_ttl: 60,
                  serial: 1,
                  enabled: true,
                  record_count: 0,
                  created_at: '',
                  updated_at: '',
                },
              ],
            }),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<WebsiteDetailPage websiteId="4" />);

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole('button', { name: '申请证书' }),
    );

    expect(screen.getByLabelText('主域名')).toHaveValue('www.example.com');
    expect(screen.getByLabelText('验证方式')).toHaveValue('authoritative');
    const zoneSelect = await screen.findByLabelText('权威 DNS 托管域名');
    expect(zoneSelect).toBeEnabled();
    expect(zoneSelect).toHaveValue('11');
    expect(
      screen.getByRole('option', { name: 'example.com（匹配当前域名）' }),
    ).toBeInTheDocument();
  });
});
