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

vi.mock('echarts-for-react/lib/core', () => ({
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

function meteringOverviewResponse() {
  return {
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
  };
}

function ipSummaryResponse() {
  return {
    items: [],
    page: 0,
    page_size: 20,
    has_more: false,
    total_ip: 0,
    sort_by: 'total_requests',
    sort_order: 'desc',
  };
}

function accessLogItem(id: number) {
  return {
    id,
    node_id: 'node-akko',
    node_name: 'AKKO GB',
    logged_at: `2026-06-02T14:${20 - id}:00Z`,
    remote_addr: `203.0.113.${id}`,
    region: 'Canada',
    host: 'uk.dusheng.xyz',
    path: `/cursor/${id}`,
    status_code: 200,
  };
}

function isDetailAccessLogRequest(url: string) {
  return (
    url.includes('/access-logs/') &&
    !url.includes('/access-logs/folds') &&
    !url.includes('/access-logs/ip-summary') &&
    !url.includes('/access-logs/metering-overview')
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

    await userEvent.click(
      await screen.findByRole('button', { name: /明细日志/ }),
    );

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

    await userEvent.click(
      await screen.findByRole('button', { name: /明细日志/ }),
    );
    await userEvent.type(
      screen.getByPlaceholderText('按域名搜索'),
      'satandu.com',
    );
    await userEvent.keyboard('{Enter}');

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([input]) => {
          const url = String(input);
          return (
            url.includes('/access-logs/') && url.includes('host=satandu.com')
          );
        }),
      ).toBe(true),
    );
  });

  it('uses next_cursor for the next detail log page', async () => {
    stubMatchMedia();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/access-logs/metering-overview')) {
        return jsonResponse(meteringOverviewResponse());
      }

      if (url.includes('/access-logs/ip-summary')) {
        return jsonResponse(ipSummaryResponse());
      }

      if (isDetailAccessLogRequest(url)) {
        const request = new URL(url, 'http://localhost');
        const cursor = request.searchParams.get('cursor');
        return jsonResponse({
          items: [accessLogItem(cursor ? 2 : 1)],
          page: 0,
          page_size: 20,
          has_more: !cursor,
          next_cursor: cursor ? undefined : 'cursor-page-2',
          total_record: 2,
          total_ip: 2,
        });
      }

      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);

    const detailRequests = () =>
      fetchMock.mock.calls
        .map(([input]) => String(input))
        .filter(isDetailAccessLogRequest);

    renderAccessLogsPage();

    await userEvent.click(
      await screen.findByRole('button', { name: /明细日志/ }),
    );

    await waitFor(() => expect(detailRequests().length).toBeGreaterThanOrEqual(1));
    const firstRequest = new URL(detailRequests()[0], 'http://localhost');
    expect(firstRequest.searchParams.get('cursor')).toBeNull();

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /下一页/ })).toBeEnabled(),
    );
    await userEvent.click(screen.getByRole('button', { name: /下一页/ }));

    await waitFor(() => expect(detailRequests().length).toBeGreaterThanOrEqual(2));
    const secondRequest = new URL(detailRequests()[1], 'http://localhost');
    expect(secondRequest.searchParams.get('cursor')).toBe('cursor-page-2');
    expect(secondRequest.searchParams.get('p')).toBeNull();
  });

  it('resets the detail cursor when filters change', async () => {
    stubMatchMedia();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/access-logs/metering-overview')) {
        return jsonResponse(meteringOverviewResponse());
      }

      if (url.includes('/access-logs/ip-summary')) {
        return jsonResponse(ipSummaryResponse());
      }

      if (isDetailAccessLogRequest(url)) {
        return jsonResponse({
          items: [accessLogItem(1)],
          page: 0,
          page_size: 20,
          has_more: true,
          next_cursor: 'cursor-before-filter',
          total_record: 3,
          total_ip: 3,
        });
      }

      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);

    const detailRequests = () =>
      fetchMock.mock.calls
        .map(([input]) => String(input))
        .filter(isDetailAccessLogRequest);

    renderAccessLogsPage();

    await userEvent.click(
      await screen.findByRole('button', { name: /明细日志/ }),
    );
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /下一页/ })).toBeEnabled(),
    );
    await userEvent.click(screen.getByRole('button', { name: /下一页/ }));
    await waitFor(() =>
      expect(
        detailRequests().some((url) => {
          const request = new URL(url, 'http://localhost');
          return request.searchParams.get('cursor') === 'cursor-before-filter';
        }),
      ).toBe(true),
    );

    await userEvent.type(
      screen.getByPlaceholderText('按域名搜索'),
      'satandu.com',
    );
    await userEvent.keyboard('{Enter}');

    await waitFor(() =>
      expect(
        detailRequests().some((url) => {
          const request = new URL(url, 'http://localhost');
          return request.searchParams.get('host') === 'satandu.com';
        }),
      ).toBe(true),
    );
    const filteredRequest = detailRequests()
      .map((url) => new URL(url, 'http://localhost'))
      .find((url) => url.searchParams.get('host') === 'satandu.com');
    expect(filteredRequest?.searchParams.get('cursor')).toBeNull();
    expect(filteredRequest?.searchParams.get('p')).toBe('0');
  });

  it('resets the detail cursor when sorting changes', async () => {
    stubMatchMedia();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/access-logs/metering-overview')) {
        return jsonResponse(meteringOverviewResponse());
      }

      if (url.includes('/access-logs/ip-summary')) {
        return jsonResponse(ipSummaryResponse());
      }

      if (isDetailAccessLogRequest(url)) {
        return jsonResponse({
          items: [accessLogItem(1)],
          page: 0,
          page_size: 20,
          has_more: true,
          next_cursor: 'cursor-before-sort',
          total_record: 3,
          total_ip: 3,
        });
      }

      return jsonResponse({});
    });
    vi.stubGlobal('fetch', fetchMock);

    const detailRequests = () =>
      fetchMock.mock.calls
        .map(([input]) => String(input))
        .filter(isDetailAccessLogRequest);

    renderAccessLogsPage();

    await userEvent.click(
      await screen.findByRole('button', { name: /明细日志/ }),
    );
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /下一页/ })).toBeEnabled(),
    );
    await userEvent.click(screen.getByRole('button', { name: /下一页/ }));
    await waitFor(() =>
      expect(
        detailRequests().some((url) => {
          const request = new URL(url, 'http://localhost');
          return request.searchParams.get('cursor') === 'cursor-before-sort';
        }),
      ).toBe(true),
    );

    await userEvent.selectOptions(
      screen.getAllByRole('combobox')[1],
      'status_code:desc',
    );

    await waitFor(() =>
      expect(
        detailRequests().some((url) => {
          const request = new URL(url, 'http://localhost');
          return request.searchParams.get('sort_by') === 'status_code';
        }),
      ).toBe(true),
    );
    const sortedRequest = detailRequests()
      .map((url) => new URL(url, 'http://localhost'))
      .find((url) => url.searchParams.get('sort_by') === 'status_code');
    expect(sortedRequest?.searchParams.get('cursor')).toBeNull();
    expect(sortedRequest?.searchParams.get('p')).toBe('0');
  });
});
