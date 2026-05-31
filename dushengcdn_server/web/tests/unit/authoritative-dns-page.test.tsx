import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { AuthoritativeDNSPage } from '@/features/authoritative-dns/components/authoritative-dns-page';

function stubMatchMedia() {
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
}

function renderWithProviders(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
      mutations: {
        retry: false,
      },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <ToastProvider>
          <ConfirmDialogProvider>{ui}</ConfirmDialogProvider>
        </ToastProvider>
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

describe('Authoritative DNS page', () => {
  beforeEach(() => {
    stubMatchMedia();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders zones, records and worker token creation', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/dns-zones/1/records')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 31,
                    zone_id: 1,
                    name: 'www.example.com',
                    type: 'A',
                    value: '203.0.113.10',
                    ttl: 300,
                    priority: 0,
                    enabled: true,
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:00:00Z',
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-zones/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    name: 'example.com',
                    soa_email: 'hostmaster@example.com',
                    primary_ns: 'ns1.example.net',
                    name_servers: ['ns1.example.net', 'ns2.example.net'],
                    default_ttl: 300,
                    serial: 2026053101,
                    enabled: true,
                    record_count: 1,
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:00:00Z',
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/') && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  id: 8,
                  worker_id: 'dns-worker-8',
                  name: 'ns2-eu',
                  token: 'created-token',
                  public_address: 'ns2.example.net',
                  version: '',
                  status: 'offline',
                  last_snapshot_version: '',
                  last_snapshot_at: null,
                  last_seen_at: null,
                  last_error: '',
                  created_at: '2026-05-31T08:00:00Z',
                  updated_at: '2026-05-31T08:00:00Z',
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 7,
                    worker_id: 'dns-worker-7',
                    name: 'ns1-hk',
                    public_address: 'ns1.example.net',
                    version: 'v1.2.3',
                    status: 'online',
                    last_snapshot_version: 'snapshot-a',
                    last_snapshot_at: '2026-05-31T08:05:00Z',
                    last_seen_at: '2026-05-31T08:06:00Z',
                    last_error: '',
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:06:00Z',
                  },
                ],
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<AuthoritativeDNSPage />);

    expect(await screen.findAllByText('example.com')).toHaveLength(2);
    expect(screen.getAllByText('ns1.example.net').length).toBeGreaterThan(0);
    expect(await screen.findByText('www.example.com')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: /^DNS Worker/ }));
    expect(await screen.findByText('ns1-hk')).toBeInTheDocument();

    await user.click(screen.getAllByRole('button', { name: '创建 Worker' })[0]);
    const createDialog = await screen.findByRole('dialog', {
      name: '创建 DNS Worker',
    });
    await user.type(within(createDialog).getByPlaceholderText('ns1-hk'), 'ns2-eu');
    await user.type(
      within(createDialog).getByPlaceholderText('ns1.example.net'),
      'ns2.example.net',
    );
    await user.click(within(createDialog).getByRole('button', { name: '创建' }));

    await waitFor(() => {
      expect(
        screen.getByRole('dialog', { name: 'DNS Worker Token' }),
      ).toBeInTheDocument();
    });
    expect(screen.getByDisplayValue('created-token')).toBeInTheDocument();
    expect(screen.getByText(/DUSHENGCDN_DNS_WORKER_TOKEN=created-token/)).toBeInTheDocument();
  });
});
