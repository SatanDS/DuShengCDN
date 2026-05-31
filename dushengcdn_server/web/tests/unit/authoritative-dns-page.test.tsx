import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { AuthoritativeDNSPage } from '@/features/authoritative-dns/components/authoritative-dns-page';

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

        if (url.includes('/dns-workers/observability')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  window_hours: 24,
                  window_start: '2026-05-30T08:00:00Z',
                  window_end: '2026-05-31T08:00:00Z',
                  last_rollup_at: '2026-05-31T08:06:00Z',
                  total_queries: 128,
                  successful_queries: 120,
                  negative_queries: 5,
                  error_queries: 3,
                  dynamic_queries: 100,
                  static_queries: 28,
                  rcode_breakdown: [
                    { key: 'NOERROR', label: 'NOERROR', count: 120 },
                    { key: 'SERVFAIL', label: 'SERVFAIL', count: 3 },
                  ],
                  qtype_breakdown: [{ key: 'A', label: 'A', count: 128 }],
                  top_qnames: [
                    {
                      key: 'www.example.com',
                      label: 'www.example.com',
                      count: 100,
                    },
                  ],
                  top_targets: [
                    { key: '203.0.113.10', label: '203.0.113.10', count: 80 },
                  ],
                  worker_breakdown: [
                    { key: 'dns-worker-7', label: 'ns1-hk', count: 128 },
                  ],
                  zone_breakdown: [
                    { key: '1', label: 'example.com', count: 128 },
                  ],
                  route_breakdown: [
                    { key: '1', label: 'edge-site', count: 100 },
                  ],
                  source_scope_breakdown: [
                    { key: 'country:HK', label: 'country:HK', count: 80 },
                    { key: 'country:DE', label: 'country:DE', count: 20 },
                    { key: 'global', label: 'global', count: 28 },
                  ],
                  trend_points: [
                    {
                      bucket_started_at: '2026-05-31T07:00:00Z',
                      query_count: 40,
                      successful_queries: 36,
                      negative_queries: 2,
                      error_queries: 2,
                      dynamic_queries: 30,
                      static_queries: 10,
                      noerror_queries: 36,
                      nxdomain_queries: 2,
                      servfail_queries: 2,
                    },
                    {
                      bucket_started_at: '2026-05-31T08:00:00Z',
                      query_count: 88,
                      successful_queries: 84,
                      negative_queries: 3,
                      error_queries: 1,
                      dynamic_queries: 70,
                      static_queries: 18,
                      noerror_queries: 84,
                      nxdomain_queries: 3,
                      servfail_queries: 1,
                    },
                  ],
                  snapshot_consistency: {
                    status: 'divergent',
                    checked_at: '2026-05-31T08:10:00Z',
                    snapshot_max_age_seconds: 300,
                    total_worker_count: 2,
                    online_worker_count: 2,
                    stale_worker_count: 0,
                    divergent_worker_count: 1,
                    latest_snapshot_version: 'snapshot-b',
                    latest_snapshot_at: '2026-05-31T08:08:00Z',
                    version_breakdown: [
                      {
                        version: 'snapshot-a',
                        worker_count: 1,
                        latest_snapshot_at: '2026-05-31T08:05:00Z',
                        workers: ['ns1-hk'],
                      },
                      {
                        version: 'snapshot-b',
                        worker_count: 1,
                        latest_snapshot_at: '2026-05-31T08:08:00Z',
                        workers: ['ns2-eu'],
                      },
                    ],
                    workers: [],
                  },
                  worker_health: {
                    checked_at: '2026-05-31T08:10:00Z',
                    total_worker_count: 2,
                    online_worker_count: 2,
                    probe_healthy_count: 1,
                    probe_checked_count: 1,
                    probe_healthy_percent: 100,
                    availability_percent: 100,
                    average_latency_ms: 12.5,
                    max_latency_ms: 48,
                    error_rate_percent: 2.3,
                    workers: [
                      {
                        worker_id: 'dns-worker-7',
                        name: 'ns1-hk',
                        status: 'online',
                        public_address: 'ns1.example.net',
                        query_count: 128,
                        error_queries: 3,
                        error_rate_percent: 2.3,
                        average_latency_ms: 12.5,
                        max_latency_ms: 48,
                        last_seen_at: '2026-05-31T08:06:00Z',
                        last_snapshot_at: '2026-05-31T08:05:00Z',
                        snapshot_age_seconds: 300,
                        snapshot_stale: false,
                        last_error: '',
                        last_probe_at: '2026-05-31T08:12:00Z',
                        probe_status: 'healthy',
                        probe_healthy: true,
                        probe_age_seconds: 120,
                        probe_message: 'UDP/TCP 53 均可达',
                        last_probe_results: [
                          {
                            network: 'UDP',
                            reachable: true,
                            duration_ms: 18,
                            rcode: 'NOERROR',
                            answer_count: 1,
                          },
                          {
                            network: 'TCP',
                            reachable: true,
                            duration_ms: 24,
                            rcode: 'NOERROR',
                            answer_count: 1,
                          },
                        ],
                      },
                      {
                        worker_id: 'dns-worker-8',
                        name: 'ns2-eu',
                        status: 'online',
                        public_address: 'ns2.example.net',
                        query_count: 0,
                        error_queries: 0,
                        error_rate_percent: 0,
                        average_latency_ms: 0,
                        max_latency_ms: 0,
                        last_seen_at: '2026-05-31T08:06:00Z',
                        last_snapshot_at: '2026-05-31T08:08:00Z',
                        snapshot_age_seconds: 120,
                        snapshot_stale: false,
                        last_error: '',
                        last_probe_at: null,
                        probe_status: 'unknown',
                        probe_healthy: false,
                        probe_age_seconds: 0,
                        probe_message: '尚未执行公网 UDP/TCP 53 探测',
                        last_probe_results: [],
                      },
                    ],
                  },
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-zones/1/delegation-check')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  zone_id: 1,
                  zone_name: 'example.com',
                  expected_name_servers: ['ns1.example.net', 'ns2.example.net'],
                  actual_name_servers: ['ns1.example.net'],
                  matched_name_servers: ['ns1.example.net'],
                  missing_name_servers: ['ns2.example.net'],
                  extra_name_servers: [],
                  glue_required: true,
                  glue_name_servers: ['ns1.example.net'],
                  status: 'partial',
                  checked_at: '2026-05-31T08:10:00Z',
                },
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

        if (url.includes('/dns-workers/7/probe') && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  worker_id: 'dns-worker-7',
                  name: 'ns1-hk',
                  public_address: 'ns1.example.net',
                  query_name: 'example.com.',
                  query_type: 'SOA',
                  checked_at: '2026-05-31T08:12:00Z',
                  results: [
                    {
                      network: 'UDP',
                      reachable: true,
                      duration_ms: 18,
                      rcode: 'NOERROR',
                      answer_count: 1,
                    },
                    {
                      network: 'TCP',
                      reachable: true,
                      duration_ms: 24,
                      rcode: 'NOERROR',
                      answer_count: 1,
                    },
                  ],
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/simulate') && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  proxy_route_id: 92,
                  site_name: 'authoritative-site',
                  qname: 'api.example.com',
                  record_type: 'A',
                  country: 'HK',
                  source_ip: '',
                  source_scope: 'country:HK',
                  ttl: 30,
                  targets: ['8.8.4.4'],
                  target_count: 1,
                  strategy: 'weighted',
                  gslb_enabled: true,
                  snapshot_version: 'snapshot-c',
                  snapshot_at: '2026-05-31T08:20:00Z',
                  message:
                    '模拟结果来自当前 Server 生成的权威 DNS 快照，不会写入真实调度防抖状态。',
                  matched_pools: [
                    {
                      name: 'hk',
                      weight: 80,
                      countries: ['HK'],
                      matched: true,
                      reason: '匹配来源国家 HK',
                    },
                    {
                      name: 'eu',
                      weight: 20,
                      countries: ['DE'],
                      matched: false,
                      reason: '未匹配来源国家',
                    },
                  ],
                  nodes: [
                    {
                      node_id: 'node-hk',
                      name: 'hk-edge',
                      pool_name: 'hk',
                      status: 'online',
                      openresty_status: 'healthy',
                      scheduling_enabled: true,
                      drain_mode: false,
                      last_seen_at: '2026-05-31T08:19:00Z',
                      public_ips: ['8.8.4.4'],
                      candidate_targets: ['8.8.4.4'],
                      selected_targets: ['8.8.4.4'],
                      eligible: true,
                      selected: true,
                      reasons: ['可参与当前调度'],
                      has_metric: true,
                      openresty_connections: 12,
                      cpu_usage_percent: 10,
                      memory_usage_percent: 30,
                      score: 8000,
                    },
                    {
                      node_id: 'node-hot',
                      name: 'hot-edge',
                      pool_name: 'hk',
                      status: 'online',
                      openresty_status: 'healthy',
                      scheduling_enabled: true,
                      drain_mode: false,
                      last_seen_at: '2026-05-31T08:19:00Z',
                      public_ips: ['9.9.9.9'],
                      candidate_targets: ['9.9.9.9'],
                      selected_targets: [],
                      eligible: false,
                      selected: false,
                      reasons: ['节点负载超过 GSLB 阈值'],
                      has_metric: true,
                      openresty_connections: 99,
                      cpu_usage_percent: 20,
                      memory_usage_percent: 40,
                      score: 0,
                    },
                  ],
                },
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
                  last_probe_at: null,
                  last_probe_query: '',
                  last_probe_results: [],
                  probe_status: 'unknown',
                  probe_healthy: false,
                  probe_age_seconds: 0,
                  probe_message: '尚未执行公网 UDP/TCP 53 探测',
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
                    last_probe_at: '2026-05-31T08:12:00Z',
                    last_probe_query: 'example.com. SOA',
                    probe_status: 'healthy',
                    probe_healthy: true,
                    probe_age_seconds: 120,
                    probe_message: 'UDP/TCP 53 均可达',
                    last_probe_results: [
                      {
                        network: 'UDP',
                        reachable: true,
                        duration_ms: 18,
                        rcode: 'NOERROR',
                        answer_count: 1,
                      },
                      {
                        network: 'TCP',
                        reachable: true,
                        duration_ms: 24,
                        rcode: 'NOERROR',
                        answer_count: 1,
                      },
                    ],
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:06:00Z',
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 91,
                    site_name: 'edge-site',
                    domain: 'www.example.com',
                    domains: ['www.example.com'],
                    primary_domain: 'www.example.com',
                    domain_count: 1,
                    enabled: true,
                    node_pool: 'hk',
                    dns_auto_sync: true,
                    dns_record_type: 'A',
                    dns_provider_mode: 'cloudflare',
                    dns_zone_id_ref: null,
                    gslb_enabled: true,
                    gslb_policy: {
                      mode: 'cloudflare_dns',
                      strategy: 'weighted',
                      pools: [
                        {
                          name: 'hk',
                          weight: 80,
                          countries: ['HK'],
                          enabled: true,
                        },
                        {
                          name: 'eu',
                          weight: 20,
                          countries: ['DE'],
                          enabled: true,
                        },
                      ],
                      target_count: 1,
                      ttl: 30,
                      source_ip: {
                        provider: 'none',
                        api_url: '',
                        api_token: '',
                      },
                      load_thresholds: {
                        max_openresty_connections: 0,
                        max_cpu_percent: 0,
                        max_memory_percent: 0,
                      },
                      debounce: {
                        cooldown_seconds: 60,
                        unhealthy_threshold: 1,
                        recovery_threshold: 1,
                      },
                    },
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:00:00Z',
                  },
                  {
                    id: 92,
                    site_name: 'authoritative-site',
                    domain: 'api.example.com',
                    domains: ['api.example.com'],
                    primary_domain: 'api.example.com',
                    domain_count: 1,
                    enabled: true,
                    node_pool: 'default',
                    dns_auto_sync: false,
                    dns_record_type: 'A',
                    dns_provider_mode: 'authoritative',
                    dns_zone_id_ref: 1,
                    gslb_enabled: true,
                    gslb_policy: {
                      mode: 'cloudflare_dns',
                      strategy: 'weighted',
                      pools: [
                        {
                          name: 'hk',
                          weight: 80,
                          countries: ['HK'],
                          enabled: true,
                        },
                        {
                          name: 'eu',
                          weight: 20,
                          countries: ['DE'],
                          enabled: true,
                        },
                      ],
                      target_count: 1,
                      ttl: 30,
                      source_ip: {
                        provider: 'none',
                        api_url: '',
                        api_token: '',
                      },
                      load_thresholds: {
                        max_openresty_connections: 0,
                        max_cpu_percent: 0,
                        max_memory_percent: 0,
                      },
                      debounce: {
                        cooldown_seconds: 60,
                        unhealthy_threshold: 1,
                        recovery_threshold: 1,
                      },
                    },
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:00:00Z',
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
    expect(await screen.findByText('DNS 查询观测')).toBeInTheDocument();
    expect(screen.getAllByText('203.0.113.10').length).toBeGreaterThan(0);
    expect(screen.getByText('edge-site')).toBeInTheDocument();
    expect(screen.getByText('来源作用域')).toBeInTheDocument();
    expect(screen.getByText('country:HK')).toBeInTheDocument();
    expect(await screen.findByText('查询趋势')).toBeInTheDocument();
    expect(screen.getByText('快照不一致')).toBeInTheDocument();
    expect(
      screen.getByText(/在线 Worker 当前使用了不同快照版本/),
    ).toBeInTheDocument();
    expect(screen.getByText('snapshot-a')).toBeInTheDocument();
    expect(screen.getAllByText('snapshot-b').length).toBeGreaterThan(0);
    expect(screen.getByText('Worker 可用性')).toBeInTheDocument();
    expect(screen.getAllByText('平均延迟').length).toBeGreaterThan(0);
    expect(screen.getAllByText('最大延迟').length).toBeGreaterThan(0);
    expect(screen.getAllByText('12.5 ms').length).toBeGreaterThan(0);
    expect(screen.getAllByText('48 ms').length).toBeGreaterThan(0);
    expect(screen.getByText('GSLB 调度模拟')).toBeInTheDocument();
    await userEvent.setup().type(screen.getByPlaceholderText('HK'), 'HK');
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: '模拟调度' }));
    await waitFor(() => {
      expect(screen.getAllByText('8.8.4.4').length).toBeGreaterThan(0);
    });
    expect(screen.getAllByText('country:HK').length).toBeGreaterThan(0);
    expect(screen.getByText(/snapshot-c/)).toBeInTheDocument();
    expect(screen.getByText('节点池匹配')).toBeInTheDocument();
    expect(screen.getByText('节点诊断')).toBeInTheDocument();
    expect(screen.getByText('hk-edge')).toBeInTheDocument();
    expect(screen.getByText('hot-edge')).toBeInTheDocument();
    expect(screen.getByText('节点负载超过 GSLB 阈值')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: '检查委派' }));
    expect(await screen.findByText('部分匹配')).toBeInTheDocument();
    expect(await screen.findByText('缺失 NS')).toBeInTheDocument();
    expect(screen.getAllByText('ns2.example.net').length).toBeGreaterThan(0);
    expect(screen.getByText(/Glue 提示/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^迁移向导/ }));
    expect(await screen.findByText('待迁移站点')).toBeInTheDocument();
    expect(screen.getByText('已权威 DNS')).toBeInTheDocument();
    expect(screen.getAllByText('edge-site').length).toBeGreaterThan(0);
    expect(screen.getByText('匹配 Zone example.com')).toBeInTheDocument();
    expect(screen.getAllByText('在线 Worker').length).toBeGreaterThan(0);
    expect(screen.getAllByText('公网可达').length).toBeGreaterThan(0);
    expect(screen.getAllByText('公网可达 Worker').length).toBeGreaterThan(0);
    expect(screen.getAllByText('1 / 1').length).toBeGreaterThan(0);
    expect(screen.getByRole('link', { name: '去网站详情' })).toHaveAttribute(
      'href',
      '/proxy-route/detail?id=91',
    );

    await user.click(screen.getByRole('button', { name: /^DNS Worker/ }));
    await waitFor(() => {
      expect(screen.getAllByText('ns1-hk').length).toBeGreaterThan(0);
    });
    expect(await screen.findByText('最近探测')).toBeInTheDocument();
    expect((await screen.findAllByText('UDP 可达')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('TCP 可达')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('18 ms').length).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: '探测' }));
    expect(await screen.findByText(/DNS Worker 探测完成/)).toBeInTheDocument();

    await user.click(screen.getAllByRole('button', { name: '创建 Worker' })[0]);
    const createDialog = await screen.findByRole('dialog', {
      name: '创建 DNS Worker',
    });
    await user.type(
      within(createDialog).getByPlaceholderText('ns1-hk'),
      'ns2-eu',
    );
    await user.type(
      within(createDialog).getByPlaceholderText('ns1.example.net'),
      'ns2.example.net',
    );
    await user.click(
      within(createDialog).getByRole('button', { name: '创建' }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole('dialog', { name: 'DNS Worker Token' }),
      ).toBeInTheDocument();
    });
    expect(screen.getByDisplayValue('created-token')).toBeInTheDocument();
    expect(
      screen.getByText(/DUSHENGCDN_DNS_WORKER_TOKEN=created-token/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200/),
    ).toBeInTheDocument();
    expect(screen.getByText(/--udp-response-size 1232/)).toBeInTheDocument();
  });
});
