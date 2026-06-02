import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { NodesPage } from '@/features/nodes/components/nodes-page';

let searchParams = new URLSearchParams();

vi.mock('next/navigation', () => ({
  useSearchParams: () => searchParams,
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

function nodeFixture(overrides: Partial<Record<string, unknown>>) {
  return {
    id: 1,
    node_id: 'node-1',
    name: 'HK Edge',
    ip: '203.0.113.10',
    pool_name: 'hongkong',
    tags: [],
    weight: 100,
    public_ips: ['203.0.113.10'],
    scheduling_enabled: true,
    drain_mode: false,
    geo_name: 'HK',
    geo_latitude: null,
    geo_longitude: null,
    geo_manual_override: false,
    agent_token: 'token',
    auto_update_enabled: false,
    update_requested: false,
    update_channel: 'stable',
    update_tag: '',
    restart_openresty_requested: false,
    agent_version: '1.0.0',
    nginx_version: '1.29.2',
    openresty_status: 'healthy',
    openresty_message: '',
    status: 'online',
    current_version: '20260602-001',
    last_seen_at: '2026-06-02T10:00:00Z',
    last_error: '',
    latest_apply_result: 'success',
    latest_apply_message: '',
    latest_apply_checksum: '',
    latest_main_config_checksum: '',
    latest_route_config_checksum: '',
    latest_support_file_count: 0,
    latest_apply_at: '2026-06-02T10:00:00Z',
    created_at: '2026-06-02T09:00:00Z',
    updated_at: '2026-06-02T10:00:00Z',
    ...overrides,
  };
}

describe('NodesPage', () => {
  afterEach(() => {
    searchParams = new URLSearchParams();
    vi.unstubAllGlobals();
  });

  it('groups nodes by edge pool on the list page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.endsWith('/nodes/') || url.includes('/nodes/?')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [
                nodeFixture({
                  id: 1,
                  name: 'HK Edge 1',
                  pool_name: 'hongkong',
                  public_ips: ['203.0.113.10'],
                }),
                nodeFixture({
                  id: 2,
                  name: 'HK Edge 2',
                  pool_name: 'hongkong',
                  ip: '203.0.113.11',
                  public_ips: ['203.0.113.11'],
                }),
                nodeFixture({
                  id: 3,
                  name: 'EU Edge 1',
                  pool_name: 'europe',
                  ip: '198.51.100.10',
                  public_ips: ['198.51.100.10'],
                }),
              ],
            }),
          );
        }

        if (url.includes('/config-versions/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [{ id: 1, version: '20260602-001', is_active: true }],
            }),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<NodesPage />);

    expect(await screen.findByText('边缘池列表')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '新增边缘池' })).toBeInTheDocument();

    const hongkongRow = (await screen.findByText('hongkong')).closest('tr');
    expect(hongkongRow).not.toBeNull();
    expect(within(hongkongRow as HTMLElement).getByText('HK Edge 1')).toBeInTheDocument();
    expect(within(hongkongRow as HTMLElement).getByText('HK Edge 2')).toBeInTheDocument();
    expect(
      within(hongkongRow as HTMLElement).getByRole('link', { name: '详情' }),
    ).toHaveAttribute('href', '/node?pool=hongkong');
  });

  it('shows pool detail with node management actions', async () => {
    searchParams = new URLSearchParams('pool=hongkong');
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.endsWith('/nodes/') || url.includes('/nodes/?')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [
                nodeFixture({
                  id: 1,
                  name: 'HK Edge 1',
                  pool_name: 'hongkong',
                }),
                nodeFixture({
                  id: 2,
                  name: 'EU Edge 1',
                  pool_name: 'europe',
                }),
              ],
            }),
          );
        }

        if (url.includes('/config-versions/')) {
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: [],
            }),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<NodesPage />);

    expect(await screen.findByText('边缘池：hongkong')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '添加节点' })).toBeInTheDocument();
    expect(await screen.findByText('HK Edge 1')).toBeInTheDocument();
    expect(screen.queryByText('EU Edge 1')).not.toBeInTheDocument();
  });
});
