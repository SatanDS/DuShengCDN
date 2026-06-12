import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { AuthoritativeDNSPage } from '@/features/authoritative-dns/components/authoritative-dns-page';
import { formatDateTime } from '@/lib/utils/date';

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
    const simulateRequests: Array<Record<string, unknown>> = [];

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
                  source_country_breakdown: [
                    { key: 'HK', label: 'HK', count: 80 },
                    { key: 'DE', label: 'DE', count: 20 },
                  ],
                  source_asn_breakdown: [],
                  source_operator_breakdown: [],
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
                      query_count: 0,
                      successful_queries: 0,
                      negative_queries: 0,
                      error_queries: 0,
                      dynamic_queries: 0,
                      static_queries: 0,
                      noerror_queries: 0,
                      nxdomain_queries: 0,
                      servfail_queries: 0,
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
                    node_probe_healthy_count: 1,
                    node_probe_checked_count: 2,
                    node_probe_stale_count: 1,
                    node_probe_healthy_percent: 50,
                    node_probe_average_rtt_ms: 31,
                    node_probe_max_rtt_ms: 70,
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
                        node_probe_total_count: 2,
                        node_probe_healthy_count: 1,
                        node_probe_stale_count: 1,
                        node_probe_healthy_percent: 50,
                        node_probe_average_rtt_ms: 31,
                        node_probe_max_rtt_ms: 70,
                        node_probes: [
                          {
                            node_id: 'node-hk-1',
                            node_name: 'hk-edge-1',
                            pool_name: 'HK',
                            status: 'online',
                            checked_at: '2026-05-31T08:11:00Z',
                            healthy: true,
                            probe_status: 'healthy',
                            probe_age_seconds: 60,
                            probe_message: 'UDP/TCP 53 均可达',
                            average_rtt_ms: 21,
                            max_rtt_ms: 24,
                            last_error: '',
                            failure_samples: 0,
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
                          {
                            node_id: 'node-eu-1',
                            node_name: 'eu-edge-1',
                            pool_name: 'EU',
                            status: 'online',
                            checked_at: '2026-05-31T07:01:00Z',
                            healthy: false,
                            probe_status: 'stale',
                            probe_age_seconds: 4200,
                            probe_message:
                              'Agent 多节点探测结果超过 5 分钟未刷新',
                            average_rtt_ms: 41,
                            max_rtt_ms: 70,
                            last_error: 'TCP 53 探测失败',
                            failure_samples: 1,
                            results: [
                              {
                                network: 'UDP',
                                reachable: true,
                                duration_ms: 41,
                                rcode: 'NOERROR',
                                answer_count: 1,
                              },
                              {
                                network: 'TCP',
                                reachable: false,
                                duration_ms: 70,
                                rcode: '',
                                answer_count: 0,
                                error: 'i/o timeout',
                              },
                            ],
                          },
                        ],
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
                        node_probe_total_count: 0,
                        node_probe_healthy_count: 0,
                        node_probe_stale_count: 0,
                        node_probe_healthy_percent: 0,
                        node_probe_average_rtt_ms: 0,
                        node_probe_max_rtt_ms: 0,
                        node_probes: [],
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
          const payload = JSON.parse(String(init?.body ?? '{}')) as {
            country?: string;
            operator?: string;
            asn?: number;
            source_ip?: string;
          };
          simulateRequests.push(payload as Record<string, unknown>);
          if (payload.source_ip === '203.0.113.23') {
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
                    country: '',
                    source_ip: '203.0.113.23',
                    source_scope: 'cidr:203.0.113.0/24|bucket:17',
                    ttl: 30,
                    targets: ['203.0.113.80'],
                    target_count: 1,
                    strategy: 'weighted',
                    gslb_enabled: true,
                    snapshot_version: 'snapshot-cidr',
                    snapshot_at: '2026-05-31T08:28:00Z',
                    message:
                      '模拟结果来自当前 Server 生成的权威 DNS 快照，不会写入真实调度防抖状态。',
                    matched_pools: [
                      {
                        name: 'vip',
                        weight: 100,
                        countries: [],
                        source_cidrs: ['203.0.113.0/24'],
                        matched: true,
                        reason: '匹配来源 CIDR 203.0.113.0/24',
                      },
                    ],
                    nodes: [
                      {
                        node_id: 'node-cidr',
                        name: 'cidr-edge',
                        pool_name: 'vip',
                        status: 'online',
                        openresty_status: 'healthy',
                        scheduling_enabled: true,
                        drain_mode: false,
                        last_seen_at: '2026-05-31T08:27:00Z',
                        public_ips: ['203.0.113.80'],
                        candidate_targets: ['203.0.113.80'],
                        selected_targets: ['203.0.113.80'],
                        eligible: true,
                        selected: true,
                        reasons: ['可参与当前调度'],
                        has_metric: true,
                        metric_captured_at: '2026-05-31T08:27:10Z',
                        openresty_connections: 8,
                        cpu_usage_percent: 9,
                        memory_usage_percent: 18,
                        score: 10000,
                        node_probe_status: 'healthy',
                        node_probe_message:
                          '该节点到 DNS Worker 多点探测全部可达（1/1）',
                        node_probe_checked_count: 1,
                        node_probe_healthy_count: 1,
                        node_probe_stale_count: 0,
                        node_probe_healthy_percent: 100,
                        node_probe_average_rtt_ms: 18,
                        node_probe_max_rtt_ms: 20,
                      },
                    ],
                  },
                }),
              ),
            );
          }
          if (payload.country === 'DE') {
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
                    country: 'DE',
                    source_ip: '',
                    source_scope: 'country:DE',
                    ttl: 30,
                    targets: [],
                    target_count: 0,
                    strategy: 'load_aware',
                    gslb_enabled: true,
                    snapshot_version: 'snapshot-d',
                    snapshot_at: '2026-05-31T08:25:00Z',
                    message:
                      '当前来源没有可用于 A 记录的边缘节点。请查看下方节点原因确认节点池、在线状态、OpenResty 健康、公网 IP 类型和负载阈值。 模拟结果来自当前 Server 生成的权威 DNS 快照，不会写入真实调度防抖状态。',
                    matched_pools: [
                      {
                        name: 'hk',
                        weight: 80,
                        countries: ['HK'],
                        matched: false,
                        reason: '未匹配来源国家',
                      },
                      {
                        name: 'eu',
                        weight: 20,
                        countries: ['DE'],
                        matched: true,
                        reason: '匹配来源国家 DE',
                      },
                    ],
                    nodes: [
                      {
                        node_id: 'node-eu-hot',
                        name: 'eu-hot',
                        pool_name: 'eu',
                        status: 'online',
                        openresty_status: 'healthy',
                        scheduling_enabled: true,
                        drain_mode: false,
                        last_seen_at: '2026-05-31T08:24:00Z',
                        public_ips: ['1.1.1.1'],
                        candidate_targets: ['1.1.1.1'],
                        selected_targets: [],
                        eligible: false,
                        selected: false,
                        reasons: ['节点负载超过 GSLB 阈值'],
                        has_metric: true,
                        metric_captured_at: '2026-05-31T08:24:10Z',
                        openresty_connections: 120,
                        cpu_usage_percent: 95,
                        memory_usage_percent: 70,
                        score: 0,
                        node_probe_status: 'unknown',
                        node_probe_message:
                          '尚未收到该节点的 DNS Worker 多点探测结果',
                        node_probe_checked_count: 0,
                        node_probe_healthy_count: 0,
                        node_probe_stale_count: 0,
                        node_probe_healthy_percent: 0,
                        node_probe_average_rtt_ms: 0,
                        node_probe_max_rtt_ms: 0,
                      },
                    ],
                  },
                }),
              ),
            );
          }
          if (payload.country === 'JP') {
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
                    country: 'JP',
                    source_ip: '',
                    source_scope: 'country:JP',
                    ttl: 30,
                    targets: [],
                    target_count: 0,
                    strategy: 'weighted',
                    gslb_enabled: true,
                    snapshot_version: 'snapshot-probe-gate',
                    snapshot_at: '2026-05-31T08:29:00Z',
                    message:
                      'Agent 探测未达到调度门槛，当前来源没有可用于 A 记录的边缘节点。请查看下方节点原因确认是未探测、探测过期还是 UDP/TCP 53 未同时可达。',
                    matched_pools: [
                      {
                        name: 'jp',
                        weight: 100,
                        countries: ['JP'],
                        matched: true,
                        reason: '匹配来源国家 JP',
                      },
                    ],
                    nodes: [
                      {
                        node_id: 'node-jp-stale',
                        name: 'jp-stale',
                        pool_name: 'jp',
                        status: 'online',
                        openresty_status: 'healthy',
                        scheduling_enabled: true,
                        drain_mode: false,
                        last_seen_at: '2026-05-31T08:28:00Z',
                        public_ips: ['203.0.113.90'],
                        candidate_targets: ['203.0.113.90'],
                        selected_targets: [],
                        eligible: false,
                        selected: false,
                        reasons: ['Agent 探测未达到调度门槛：探测结果已过期'],
                        has_metric: true,
                        metric_captured_at: '2026-05-31T08:28:10Z',
                        openresty_connections: 7,
                        cpu_usage_percent: 11,
                        memory_usage_percent: 22,
                        score: 0,
                        node_probe_status: 'stale',
                        node_probe_message:
                          'Agent 多节点探测结果超过 5 分钟未刷新',
                        node_probe_checked_count: 1,
                        node_probe_healthy_count: 0,
                        node_probe_stale_count: 1,
                        node_probe_healthy_percent: 0,
                        node_probe_average_rtt_ms: 42,
                        node_probe_max_rtt_ms: 70,
                      },
                    ],
                  },
                }),
              ),
            );
          }
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
                      metric_captured_at: '2026-05-31T08:19:10Z',
                      openresty_connections: 12,
                      cpu_usage_percent: 10,
                      memory_usage_percent: 30,
                      score: 8000,
                      node_probe_status: 'healthy',
                      node_probe_message:
                        '该节点到 DNS Worker 多点探测全部可达（1/1）',
                      node_probe_checked_count: 1,
                      node_probe_healthy_count: 1,
                      node_probe_stale_count: 0,
                      node_probe_healthy_percent: 100,
                      node_probe_average_rtt_ms: 21,
                      node_probe_max_rtt_ms: 24,
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
                      metric_captured_at: '2026-05-31T08:18:50Z',
                      openresty_connections: 99,
                      cpu_usage_percent: 20,
                      memory_usage_percent: 40,
                      score: 0,
                      node_probe_status: 'unknown',
                      node_probe_message:
                        '尚未收到该节点的 DNS Worker 多点探测结果',
                      node_probe_checked_count: 0,
                      node_probe_healthy_count: 0,
                      node_probe_stale_count: 0,
                      node_probe_healthy_percent: 0,
                      node_probe_average_rtt_ms: 0,
                      node_probe_max_rtt_ms: 0,
                    },
                  ],
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/migration-candidates')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    proxy_route_id: 91,
                    site_name: 'edge-site',
                    primary_domain: 'www.example.com',
                    domains: ['www.example.com'],
                    enabled: true,
                    dns_auto_sync: true,
                    dns_provider_mode: 'cloudflare',
                    dns_record_type: 'A',
                    gslb_enabled: true,
                    matching_zone_id: 1,
                    matching_zone_name: 'example.com',
                    matching_zone_enabled: true,
                    total_worker_count: 1,
                    online_worker_count: 1,
                    public_reachable_worker_count: 1,
                    fresh_snapshot_worker_count: 1,
                    ready_worker_count: 1,
                    ready: true,
                    blockers: [],
                    warnings: ['生产环境建议至少部署 2 个 DNS Worker'],
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
                  last_probe_at: null,
                  last_probe_query: '',
                  last_probe_results: [],
                  probe_status: 'unknown',
                  probe_healthy: false,
                  probe_age_seconds: 0,
                  probe_message: '尚未执行公网 UDP/TCP 53 探测',
                  node_probe_total_count: 0,
                  node_probe_healthy_count: 0,
                  node_probe_stale_count: 0,
                  node_probe_healthy_percent: 0,
                  node_probe_average_rtt_ms: 0,
                  node_probe_max_rtt_ms: 0,
                  node_probes: [],
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
                    node_probe_total_count: 0,
                    node_probe_healthy_count: 0,
                    node_probe_stale_count: 0,
                    node_probe_healthy_percent: 0,
                    node_probe_average_rtt_ms: 0,
                    node_probe_max_rtt_ms: 0,
                    node_probes: [],
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

        if (
          url.includes('/proxy-routes/91/dns/switch-authoritative') &&
          method === 'POST'
        ) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  id: 91,
                  site_name: 'edge-site',
                  domain: 'www.example.com',
                  domains: ['www.example.com'],
                  primary_domain: 'www.example.com',
                  domain_count: 1,
                  enabled: true,
                  node_pool: 'hk',
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
                  updated_at: '2026-05-31T08:30:00Z',
                },
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
                  {
                    id: 93,
                    site_name: 'wildcard-authoritative',
                    domain: '*.wild.example.com',
                    domains: ['*.wild.example.com'],
                    primary_domain: '*.wild.example.com',
                    domain_count: 1,
                    enabled: true,
                    node_pool: 'default',
                    dns_auto_sync: false,
                    dns_record_type: 'A',
                    dns_provider_mode: 'authoritative',
                    dns_zone_id_ref: 1,
                    gslb_enabled: false,
                    gslb_policy: null,
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
    expect(screen.getByText('正常响应')).toBeInTheDocument();
    expect(screen.getByText('响应端故障')).toBeInTheDocument();
    expect(screen.getAllByText('203.0.113.10').length).toBeGreaterThan(0);
    expect(screen.getByText('edge-site')).toBeInTheDocument();
    expect(screen.getByText('来源地区')).toBeInTheDocument();
    expect(screen.getByText('香港')).toBeInTheDocument();
    expect(await screen.findByText('查询趋势')).toBeInTheDocument();
    const trendPanel = screen.getByText('查询趋势').parentElement;
    expect(
      within(trendPanel as HTMLElement)
        .getByText('查询量')
        .closest('div')?.parentElement,
    ).toHaveTextContent('40');
    expect(
      within(trendPanel as HTMLElement)
        .getByText('服务异常')
        .closest('div')?.parentElement,
    ).toHaveTextContent('2');
    expect(
      within(trendPanel as HTMLElement)
        .getByText('域名不存在')
        .closest('div')?.parentElement,
    ).toHaveTextContent('2');
    expect(screen.getByText('解析配置不一致')).toBeInTheDocument();
    expect(
      screen.getByText(/在线响应端当前使用了 2 个配置版本/),
    ).toBeInTheDocument();
    expect(screen.getByText(/最新版本 snapshot-b/)).toBeInTheDocument();
    expect(screen.getByText(/能访问面板/)).toBeInTheDocument();
    expect(screen.getByText(/响应端密钥是否仍有效/)).toBeInTheDocument();
    expect(screen.getByText(/重启响应端重新拉取配置/)).toBeInTheDocument();
    expect(screen.getByText('重新拉取解析配置')).toBeInTheDocument();
    expect(screen.getByText('snapshot-a')).toBeInTheDocument();
    expect(screen.getAllByText('snapshot-b').length).toBeGreaterThan(0);
    expect(screen.getByText('响应端可用性')).toBeInTheDocument();
    expect(screen.getByText('多节点探测通过')).toBeInTheDocument();
    expect(screen.getByText('Agent 多节点探测')).toBeInTheDocument();
    expect(screen.getByText('hk-edge-1')).toBeInTheDocument();
    expect(screen.getByText('eu-edge-1')).toBeInTheDocument();
    expect(screen.getByText('TCP 53 探测失败')).toBeInTheDocument();
    expect(screen.getAllByText('平均延迟').length).toBeGreaterThan(0);
    expect(screen.getAllByText('最大延迟').length).toBeGreaterThan(0);
    expect(screen.getAllByText('12.5 ms').length).toBeGreaterThan(0);
    expect(screen.getAllByText('48 ms').length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/按国家\/地区匹配的节点池会回退到全局规则/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText('智能解析模拟')).toBeInTheDocument();
    expect(
      screen.getByText(
        '按站点、记录类型、来源国家、运营商、ASN 和来源 IP，预演当前解析配置会返回哪些边缘 IP。',
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('例如 HK、DE；留空使用全局。')).toBeInTheDocument();
    expect(
      screen.getByText('可选；本地 DNS 响应端配置离线 ISP/ASN 库后生效。'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('可选；优先级高于运营商，例如 AS4134。'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('可选；填写后会优先按来源网段规则预演。'),
    ).toBeInTheDocument();
    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText('网站配置'), '93');
    expect(
      screen.getByDisplayValue('www.wild.example.com'),
    ).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('网站配置'), '92');
    expect(screen.getByDisplayValue('api.example.com')).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText('HK'), 'HK');
    await user.selectOptions(screen.getByLabelText('访问运营商'), 'cn-telecom');
    await user.type(screen.getByPlaceholderText('AS4134'), 'AS4134');
    await user.click(screen.getByRole('button', { name: '开始模拟' }));
    await waitFor(() => {
      expect(screen.getAllByText('8.8.4.4').length).toBeGreaterThan(0);
    });
    expect(simulateRequests[0]).toMatchObject({
      country: 'HK',
      operator: 'cn-telecom',
      asn: 4134,
    });
    expect(screen.getAllByText('香港').length).toBeGreaterThan(0);
    expect(screen.getByText(/snapshot-c/)).toBeInTheDocument();
    expect(screen.getByText('节点池匹配')).toBeInTheDocument();
    expect(screen.getByText('节点诊断')).toBeInTheDocument();
    expect(screen.getByText('hk-edge')).toBeInTheDocument();
    expect(screen.getByText('hot-edge')).toBeInTheDocument();
    expect(screen.getByText('节点压力超过上限')).toBeInTheDocument();
    expect(screen.getAllByText('负载数据时间').length).toBeGreaterThan(0);
    expect(
      screen.getByText(formatDateTime('2026-05-31T08:19:10Z')),
    ).toBeInTheDocument();
    expect(screen.getAllByText('响应端探测').length).toBeGreaterThan(0);
    expect(
      screen.getAllByText('该节点到响应端多地探测全部可达（1/1）').length,
    ).toBeGreaterThan(0);
    expect(screen.getByText('21 ms')).toBeInTheDocument();
    expect(screen.getAllByText('最大 24 ms').length).toBeGreaterThan(0);
    await user.clear(screen.getByPlaceholderText('HK'));
    await user.selectOptions(screen.getByLabelText('访问运营商'), '');
    await user.clear(screen.getByPlaceholderText('AS4134'));
    await user.type(screen.getByPlaceholderText('HK'), 'DE');
    await user.click(screen.getByRole('button', { name: '开始模拟' }));
    await waitFor(() => {
      expect(screen.getByText('当前没有可返回目标。')).toBeInTheDocument();
    });
    expect(
      screen.getByText(/当前来源没有可用于 A 记录的边缘节点/),
    ).toBeInTheDocument();
    expect(screen.getAllByText('德国').length).toBeGreaterThan(0);
    expect(screen.getByText(/snapshot-d/)).toBeInTheDocument();
    expect(screen.getByText('eu-hot')).toBeInTheDocument();
    expect(screen.getAllByText('节点压力超过上限').length).toBeGreaterThan(0);
    await user.clear(screen.getByPlaceholderText('HK'));
    await user.type(
      screen.getByPlaceholderText('203.0.113.10'),
      '203.0.113.23',
    );
    await user.click(screen.getByRole('button', { name: '开始模拟' }));
    await waitFor(() => {
      expect(screen.getAllByText('203.0.113.80').length).toBeGreaterThan(0);
    });
    expect(
      screen.getByText('网段 203.0.113.0/24 / 分流桶 17'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('cidr:203.0.113.0/24|bucket:17'),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(/CIDR 203\.0\.113\.0\/24/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByText('匹配来源 CIDR 203.0.113.0/24'),
    ).toBeInTheDocument();
    expect(screen.getByText('cidr-edge')).toBeInTheDocument();
    await user.clear(screen.getByPlaceholderText('203.0.113.10'));
    await user.type(screen.getByPlaceholderText('HK'), 'JP');
    await user.click(screen.getByRole('button', { name: '开始模拟' }));
    await waitFor(() => {
      expect(screen.getByText(/snapshot-probe-gate/)).toBeInTheDocument();
    });
    expect(screen.getByText('当前没有可返回目标。')).toBeInTheDocument();
    expect(
      screen.getAllByText(/节点到响应端探测未达要求/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText('jp-stale')).toBeInTheDocument();
    expect(
      screen.getByText('节点到响应端探测未达要求：探测结果已过期'),
    ).toBeInTheDocument();
    expect(screen.getAllByText('1 个过期').length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: '检查委派' }));
    expect(await screen.findByText('部分匹配')).toBeInTheDocument();
    expect(await screen.findByText('缺失 NS')).toBeInTheDocument();
    expect(screen.getAllByText('ns2.example.net').length).toBeGreaterThan(0);
    expect(screen.getByText(/主机记录提示/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /^迁移向导/ }));
    expect(await screen.findByText('待迁移站点')).toBeInTheDocument();
    expect(screen.getByText('已用自建解析')).toBeInTheDocument();
    expect(screen.getAllByText('edge-site').length).toBeGreaterThan(0);
    expect(screen.getByText('匹配 example.com')).toBeInTheDocument();
    expect(screen.getAllByText('在线响应端').length).toBeGreaterThan(0);
    expect(screen.getAllByText('公网可达').length).toBeGreaterThan(0);
    expect(screen.getAllByText('公网可达响应端').length).toBeGreaterThan(0);
    expect(screen.getAllByText('1 / 1').length).toBeGreaterThan(0);
    expect(screen.getByRole('link', { name: '去网站详情' })).toHaveAttribute(
      'href',
      '/proxy-route/detail?id=91',
    );
    await user.click(screen.getByRole('button', { name: '一键切换' }));
    const switchDialog = await screen.findByRole('dialog', {
      name: '切换到本地自建解析',
    });
    await user.click(
      within(switchDialog).getByRole('button', { name: '切换' }),
    );
    await waitFor(() => {
      expect(screen.getByText(/“edge-site”切换后复测完成/)).toBeInTheDocument();
    });
    expect(screen.getByText('切换后复测')).toBeInTheDocument();
    expect(screen.getByText('网站解析模式')).toBeInTheDocument();
    expect(screen.getByText('托管域名指向检查')).toBeInTheDocument();
    expect(screen.getByText('响应端公网探测')).toBeInTheDocument();
    expect(screen.getByText('智能解析复测')).toBeInTheDocument();
    expect(screen.getByText('需要确认')).toBeInTheDocument();
    expect(screen.getByText(/当前委派状态：部分匹配/)).toBeInTheDocument();
    expect(
      screen.getByText(/1 \/ 1 个在线响应端 UDP\/TCP 53 可达/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/已完成 3 组模拟，其中 1 组无返回目标/),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(/当前来源没有可用于 A 记录的边缘节点/).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText('全局').length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: /^DNS 响应端/ }));
    await waitFor(() => {
      expect(screen.getAllByText('ns1-hk').length).toBeGreaterThan(0);
    });
    expect(await screen.findByText('最近探测')).toBeInTheDocument();
    expect((await screen.findAllByText('UDP 可达')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('TCP 可达')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('18 ms').length).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: '探测' }));
    expect(await screen.findByText(/DNS 响应端探测完成/)).toBeInTheDocument();

    await user.click(
      screen.getAllByRole('button', { name: '创建 DNS 响应端' })[0],
    );
    const createDialog = await screen.findByRole('dialog', {
      name: '创建 DNS 响应端',
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
        screen.getByRole('dialog', { name: 'DNS 响应端密钥' }),
      ).toBeInTheDocument();
    });
    expect(screen.getByDisplayValue('created-token')).toBeInTheDocument();
    expect(screen.getByText(/install-dns-worker\.sh/)).toBeInTheDocument();
    expect(screen.getByText(/--worker-id 'dns-worker-8'/)).toBeInTheDocument();
    expect(
      screen.getAllByText(/--token-file "\$token_file"/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/read -r dns_worker_token/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByText(
        /DUSHENGCDN_DNS_WORKER_TOKEN_FILE=\/run\/secrets\/dushengcdn_dns_worker_token/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/DUSHENGCDN_DNS_WORKER_ID='dns-worker-8'/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200/),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(/--udp-response-size 1232/).length,
    ).toBeGreaterThan(0);
  });

  it('renders empty authoritative DNS data when API list fields are null', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/dns-workers/observability')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  window_hours: 24,
                  window_start: '2026-06-01T00:00:00Z',
                  window_end: '2026-06-02T00:00:00Z',
                  last_rollup_at: null,
                  total_queries: 0,
                  successful_queries: 0,
                  negative_queries: 0,
                  error_queries: 0,
                  dynamic_queries: 0,
                  static_queries: 0,
                  rcode_breakdown: null,
                  qtype_breakdown: null,
                  top_qnames: null,
                  top_targets: null,
                  worker_breakdown: null,
                  zone_breakdown: null,
                  route_breakdown: null,
                  source_scope_breakdown: null,
                  source_country_breakdown: null,
                  source_asn_breakdown: null,
                  source_operator_breakdown: null,
                  trend_points: null,
                  snapshot_consistency: {
                    status: 'no_online_workers',
                    checked_at: '2026-06-02T00:00:00Z',
                    snapshot_max_age_seconds: 300,
                    total_worker_count: 0,
                    online_worker_count: 0,
                    stale_worker_count: 0,
                    divergent_worker_count: 0,
                    latest_snapshot_version: '',
                    latest_snapshot_at: null,
                    version_breakdown: null,
                    workers: null,
                  },
                  worker_health: {
                    checked_at: '2026-06-02T00:00:00Z',
                    total_worker_count: 0,
                    online_worker_count: 0,
                    probe_healthy_count: 0,
                    probe_checked_count: 0,
                    probe_healthy_percent: 0,
                    node_probe_healthy_count: 0,
                    node_probe_checked_count: 0,
                    node_probe_stale_count: 0,
                    node_probe_healthy_percent: 0,
                    node_probe_average_rtt_ms: 0,
                    node_probe_max_rtt_ms: 0,
                    availability_percent: 0,
                    average_latency_ms: 0,
                    max_latency_ms: 0,
                    error_rate_percent: 0,
                    workers: null,
                  },
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/scheduling-states')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  checked_at: '2026-06-02T00:00:00Z',
                  total: 0,
                  states: null,
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/migration-candidates')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: null,
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
                data: null,
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
                data: null,
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
                data: [],
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<AuthoritativeDNSPage />);

    expect(await screen.findByText('本地自建解析')).toBeInTheDocument();
    expect(screen.getByText('暂无托管域名')).toBeInTheDocument();
    expect(screen.getByText('暂无调度状态')).toBeInTheDocument();
    expect(screen.getByText('暂无本地自建解析站点')).toBeInTheDocument();
    expect(screen.getByText('无在线响应端')).toBeInTheDocument();
    expect(screen.getByText('响应端可用性')).toBeInTheDocument();
    expect(screen.getByText('暂无 DNS 响应端。')).toBeInTheDocument();
  });

  it('warns when workers are ready but no site is bound to authoritative DNS', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/dns-zones/1/records')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [],
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
                data: null,
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/scheduling-states')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  checked_at: '2026-06-01T12:00:00Z',
                  total: 0,
                  states: [],
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/migration-candidates')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [],
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
                    name: 'satandu.com',
                    soa_email: 'hostmaster@satandu.com',
                    primary_ns: 'ns1.satandu.com',
                    name_servers: ['ns1.satandu.com', 'ns2.satandu.com'],
                    default_ttl: 30,
                    serial: 1780323564,
                    enabled: true,
                    record_count: 0,
                    created_at: '2026-06-01T12:00:00Z',
                    updated_at: '2026-06-01T12:00:00Z',
                  },
                ],
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
                    id: 1,
                    worker_id: 'dns-worker-main',
                    name: 'panel-worker',
                    public_address: '145.239.140.145',
                    version: 'afb0ace',
                    status: 'online',
                    last_snapshot_version: 'snapshot-zone-only',
                    last_snapshot_at: '2026-06-01T12:00:00Z',
                    last_seen_at: '2026-06-01T12:00:10Z',
                    last_error: '',
                    geoip_enabled: false,
                    geoip_database_path: '',
                    geoip_last_error: '',
                    last_probe_at: '2026-06-01T12:00:20Z',
                    last_probe_query: 'satandu.com. SOA',
                    last_probe_results: [],
                    probe_status: 'healthy',
                    probe_healthy: true,
                    probe_age_seconds: 10,
                    probe_message: 'UDP/TCP 53 均可达',
                    created_at: '2026-06-01T12:00:00Z',
                    updated_at: '2026-06-01T12:00:00Z',
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
                data: [],
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<AuthoritativeDNSPage />);

    expect(
      await screen.findByText(/DNS 响应端已经能拉取解析配置/),
    ).toBeInTheDocument();
    expect(screen.getByText(/只能回答基础记录和静态记录/)).toBeInTheDocument();
    expect(screen.getByText(/业务域名需要到网站详情/)).toBeInTheDocument();
    expect(screen.getByText('暂无本地自建解析站点')).toBeInTheDocument();
  });

  it('creates one A record per IP input', async () => {
    const createdPayloads: Array<Record<string, unknown>> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/dns-zones/1/records') && method === 'POST') {
          const payload = JSON.parse(String(init?.body ?? '{}')) as Record<
            string,
            unknown
          >;
          createdPayloads.push(payload);
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  id: 100 + createdPayloads.length,
                  zone_id: 1,
                  ...payload,
                  created_at: '2026-06-01T12:00:00Z',
                  updated_at: '2026-06-01T12:00:00Z',
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-zones/1/records')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [],
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/observability')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ success: true, message: '', data: null }),
            ),
          );
        }

        if (url.includes('/dns-workers/scheduling-states')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  checked_at: '2026-06-01T12:00:00Z',
                  total: 0,
                  states: [],
                },
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/migration-candidates')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ success: true, message: '', data: [] }),
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
                    name: 'satandu.com',
                    soa_email: 'hostmaster@satandu.com',
                    primary_ns: 'ns1.satandu.com',
                    name_servers: ['ns1.satandu.com'],
                    default_ttl: 30,
                    serial: 1780323564,
                    enabled: true,
                    record_count: 0,
                    created_at: '2026-06-01T12:00:00Z',
                    updated_at: '2026-06-01T12:00:00Z',
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-workers/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ success: true, message: '', data: [] }),
            ),
          );
        }

        if (url.includes('/proxy-routes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ success: true, message: '', data: [] }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<AuthoritativeDNSPage />);

    const createRecordButton = await screen.findByRole('button', {
      name: '新增记录',
    });
    await user.click(createRecordButton);
    const dialog = await screen.findByRole('dialog', { name: '新增 DNS 记录' });
    expect(within(dialog).getByText('IP 地址')).toBeInTheDocument();
    expect(
      within(dialog).getByText(/仅 MX、SRV、HTTPS 和 SVCB/),
    ).toBeInTheDocument();

    const nameInput = within(dialog).getByPlaceholderText('@');
    await user.clear(nameInput);
    await user.type(nameInput, 'cdn');
    await user.type(
      within(dialog).getByPlaceholderText('203.0.113.10'),
      '145.239.140.145',
    );
    await user.click(
      within(dialog).getByRole('button', { name: '增加 IP 地址' }),
    );
    const ipInputs = within(dialog).getAllByPlaceholderText('203.0.113.10');
    await user.type(ipInputs[1], '145.239.140.146');
    await user.click(within(dialog).getByRole('button', { name: '保存' }));

    await waitFor(() => {
      expect(createdPayloads).toHaveLength(2);
    });
    expect(createdPayloads).toEqual([
      expect.objectContaining({
        name: 'cdn',
        type: 'A',
        value: '145.239.140.145',
        priority: 0,
        enabled: true,
      }),
      expect.objectContaining({
        name: 'cdn',
        type: 'A',
        value: '145.239.140.146',
        priority: 0,
        enabled: true,
      }),
    ]);
  });
});
