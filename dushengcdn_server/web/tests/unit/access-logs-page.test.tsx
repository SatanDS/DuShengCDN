import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ToastProvider } from '@/components/feedback/toast-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { AccessLogsPage } from '@/features/access-logs/components/access-logs-page';

vi.mock('echarts-for-react', () => ({
  default: () => <div data-testid="echarts-mock" />,
}));

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

function renderAccessLogsPage() {
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
        <ToastProvider>
          <AccessLogsPage />
        </ToastProvider>
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

function jsonResponse(data: unknown) {
  return Promise.resolve(
    new Response(
      JSON.stringify({
        success: true,
        message: '',
        data,
      }),
    ),
  );
}

describe('AccessLogsPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows status reason help in detail logs', async () => {
    stubMatchMedia();

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/access-logs/metering-overview')) {
          return jsonResponse({
            generated_at: '2026-06-02T14:30:00Z',
            window_started_at: '2026-06-01T14:30:00Z',
            window_ended_at: '2026-06-02T14:30:00Z',
            request_count: 1,
            response_bytes: 0,
            request_bytes: 0,
            upstream_bytes: 0,
            upstream_bytes_supported: true,
            cache_hit_count: 0,
            cache_classified_count: 0,
            cache_hit_rate_percent: 0,
            bandwidth_p95_bps: 0,
            node_availability_percent: 100,
            online_nodes: 1,
            total_nodes: 1,
            site_traffic: [],
            node_traffic: [],
            status_codes: [],
            top_urls: [],
            top_ips: [],
            top_regions: [],
            bandwidth_trend: [],
          });
        }

        if (url.includes('/access-logs/ip-summary')) {
          return jsonResponse({
            items: [],
            page: 0,
            page_size: 20,
            has_more: false,
            total_ip: 0,
            sort_by: 'total_requests',
            sort_order: 'desc',
          });
        }

        if (url.includes('/access-logs/')) {
          return jsonResponse({
            items: [
              {
                id: 1,
                node_id: 'node-akko',
                node_name: 'AKKO GB',
                logged_at: '2026-06-02T14:19:56Z',
                remote_addr: '20.220.145.212',
                region: 'Canada',
                host: 'uk.dusheng.xyz',
                path: '/database.php',
                status_code: 404,
                reason: '恶意请求防护拦截: sensitive_paths',
              },
            ],
            page: 0,
            page_size: 20,
            has_more: false,
            total_record: 1,
            total_ip: 1,
          });
        }

        return jsonResponse({});
      }),
    );

    renderAccessLogsPage();

    await userEvent.click(await screen.findByRole('button', { name: /明细日志/ }));

    const help = await screen.findByLabelText('查看状态码原因');
    await waitFor(() =>
      expect(help).toHaveAttribute(
        'title',
        '命中原因：恶意请求防护拦截: 敏感路径扫描（sensitive_paths）',
      ),
    );
  });

  it('applies the host filter from the detail log search field', async () => {
    stubMatchMedia();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/access-logs/metering-overview')) {
        return jsonResponse({
          generated_at: '2026-06-02T14:30:00Z',
          window_started_at: '2026-06-01T14:30:00Z',
          window_ended_at: '2026-06-02T14:30:00Z',
          request_count: 0,
          response_bytes: 0,
          request_bytes: 0,
          upstream_bytes: 0,
          upstream_bytes_supported: true,
          cache_hit_count: 0,
          cache_classified_count: 0,
          cache_hit_rate_percent: 0,
          bandwidth_p95_bps: 0,
          node_availability_percent: 100,
          online_nodes: 0,
          total_nodes: 0,
          site_traffic: [],
          node_traffic: [],
          status_codes: [],
          top_urls: [],
          top_ips: [],
          top_regions: [],
          bandwidth_trend: [],
        });
      }

      if (url.includes('/access-logs/ip-summary')) {
        return jsonResponse({
          items: [],
          page: 0,
          page_size: 20,
          has_more: false,
          total_ip: 0,
          sort_by: 'total_requests',
          sort_order: 'desc',
        });
      }

      if (url.includes('/access-logs/')) {
        return jsonResponse({
          items: [],
          page: 0,
          page_size: 20,
          has_more: false,
          total_record: 0,
          total_ip: 0,
        });
      }

      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);

    renderAccessLogsPage();

    await userEvent.click(await screen.findByRole('button', { name: /明细日志/ }));
    await userEvent.type(screen.getByPlaceholderText('按域名搜索'), 'satandu.com');
    await userEvent.keyboard('{Enter}');

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([input]) => {
          const url = String(input);
          return (
            url.includes('/access-logs/') &&
            url.includes('host=satandu.com')
          );
        }),
      ).toBe(true),
    );
  });
});
