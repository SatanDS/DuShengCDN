import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialogProvider } from '@/components/feedback/confirm-dialog-provider';
import { ToastProvider } from '@/components/feedback/toast-provider';
import { ThemeProvider } from '@/components/providers/theme-provider';
import { ProxyRouteConfigPage } from '@/features/proxy-routes/components/proxy-route-config-page';
import { ProxyRoutesPage } from '@/features/proxy-routes/components/proxy-routes-page';

const pushMock = vi.fn();
let searchParamsMock = new URLSearchParams();

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: pushMock,
  }),
  useSearchParams: () => searchParamsMock,
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

function buildRoute(overrides: Record<string, unknown> = {}) {
  return {
    id: 9,
    site_name: 'marketing-site',
    domain: 'app.example.com',
    domains: ['app.example.com', 'www.example.com'],
    primary_domain: 'app.example.com',
    domain_count: 2,
    origin_id: null,
    origin_url: 'https://origin-a.internal:443',
    origin_host: '',
    upstreams: JSON.stringify([
      'https://origin-a.internal:443',
      'https://origin-b.internal:443',
    ]),
    upstream_list: [
      'https://origin-a.internal:443',
      'https://origin-b.internal:443',
    ],
    node_pool: 'default',
    enabled: true,
    enable_https: true,
    cert_id: 1,
    cert_ids: [1],
    domain_cert_ids: [1, 0],
    redirect_http: true,
    limit_conn_per_server: 120,
    limit_conn_per_ip: 12,
    limit_rate: '512k',
    proxy_buffering_mode: 'default',
    cache_enabled: true,
    cache_policy: 'path_prefix',
    cache_rules: JSON.stringify(['/assets']),
    cache_rule_list: ['/assets'],
    custom_headers: JSON.stringify([{ key: 'X-Site', value: 'marketing' }]),
    custom_header_list: [{ key: 'X-Site', value: 'marketing' }],
    pow_enabled: false,
    pow_config: {
      difficulty: 4,
      algorithm: 'fast',
      session_ttl: 600,
      challenge_ttl: 300,
      whitelist: {
        ips: [],
        ip_cidrs: [],
        paths: [],
        path_regexes: [],
        user_agents: [],
      },
      blacklist: {
        ips: [],
        ip_cidrs: [],
        paths: [],
        path_regexes: [],
        user_agents: [],
      },
    },
    waf_enabled: false,
    waf_mode: 'block',
    waf_config: {
      builtin_rules: [
        'sqli',
        'xss',
        'path_traversal',
        'sensitive_paths',
        'bad_bots',
      ],
      whitelist: {
        ips: [],
        ip_cidrs: [],
        paths: [],
      },
      block_rules: {
        path_contains: [],
        path_regexes: [],
        query_contains: [],
        header_contains: [],
        user_agents: [],
      },
    },
    cc_enabled: false,
    cc_mode: 'block',
    cc_config: {
      window_seconds: 10,
      max_requests: 120,
      path_window_seconds: 10,
      path_max_requests: 60,
      block_duration_seconds: 300,
      whitelist: {
        ips: [],
        ip_cidrs: [],
        paths: [],
        user_agents: [],
      },
      exclude: {
        ips: [],
        ip_cidrs: [],
        paths: [],
        user_agents: [],
      },
    },
    basic_auth_enabled: false,
    basic_auth_username: '',
    basic_auth_password: '',
    region_restriction_enabled: false,
    region_restriction_mode: 'block',
    region_restriction_countries: [],
    dns_auto_sync: false,
    dns_account_id: null,
    dns_zone_id: '',
    dns_record_type: 'A',
    dns_record_name: '',
    dns_record_content: '',
    dns_auto_target: false,
    dns_target_count: 1,
    dns_schedule_mode: 'healthy',
    dns_ttl: 1,
    gslb_enabled: false,
    gslb_policy: {
      mode: 'cloudflare_dns',
      strategy: 'load_aware',
      pools: [
        {
          name: 'default',
          weight: 100,
          countries: [],
          enabled: true,
        },
      ],
      target_count: 2,
      ttl: 60,
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
    dns_record_ids: {},
    cloudflare_proxied: false,
    ddos_protection_mode: 'off',
    ddos_protection_provider: 'cloudflare',
    ddos_protection_target: '',
    dns_last_sync_status: '',
    dns_last_sync_message: '',
    dns_last_synced_at: null,
    dns_provider_mode: 'cloudflare',
    dns_zone_id_ref: null,
    remark: 'Marketing website',
    created_at: '2026-03-20T08:00:00Z',
    updated_at: '2026-03-21T08:00:00Z',
    ...overrides,
  };
}

function buildDiff(overrides: Record<string, unknown> = {}) {
  return {
    active_version: '20260330-001',
    added_sites: [],
    removed_sites: [],
    modified_sites: [],
    added_domains: [],
    removed_domains: [],
    modified_domains: [],
    main_config_changed: false,
    snapshot_changed: false,
    runtime_config_changed: false,
    changed_option_keys: [],
    changed_option_details: [],
    current_website_count: 1,
    active_website_count: 1,
    ...overrides,
  };
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

describe('Proxy route website pages', () => {
  beforeEach(() => {
    pushMock.mockReset();
    searchParamsMock = new URLSearchParams();
    stubMatchMedia();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders website list summary with config entry', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/proxy-routes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  buildRoute({
                    node_pool: '香港',
                  }),
                  buildRoute({
                    id: 10,
                    site_name: 'gslb-site',
                    domain: 'gslb.example.com',
                    domains: ['gslb.example.com'],
                    primary_domain: 'gslb.example.com',
                    domain_count: 1,
                    gslb_enabled: true,
                    gslb_policy: {
                      ...buildRoute().gslb_policy,
                      pools: [
                        {
                          name: '香港',
                          weight: 80,
                          countries: [],
                          source_cidrs: [],
                          enabled: true,
                        },
                        {
                          name: '欧洲',
                          weight: 20,
                          countries: [],
                          source_cidrs: [],
                          enabled: true,
                        },
                      ],
                    },
                  }),
                ],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-hk',
                    name: 'Aliyun HK',
                    ip: '47.83.141.29',
                    pool_name: '香港',
                  },
                  {
                    id: 2,
                    node_id: 'node-eu',
                    name: 'AKKO GB',
                    ip: '38.59.224.50',
                    pool_name: '欧洲',
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/config-versions/diff')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildDiff({
                  modified_sites: ['marketing-site'],
                  snapshot_changed: true,
                  runtime_config_changed: true,
                }),
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<ProxyRoutesPage />);

    expect(await screen.findByText('marketing-site')).toBeInTheDocument();
    expect(screen.getAllByText(/app\.example\.com/).length).toBeGreaterThan(0);
    expect(await screen.findByText('香港 · Aliyun HK')).toBeInTheDocument();
    expect(screen.getByText('负载均衡')).toBeInTheDocument();
    expect(screen.getByText('香港 / 欧洲')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: '配置' })[0]).toHaveAttribute(
      'href',
      '/proxy-route/detail?id=9&section=domains',
    );
  });

  it('does not publish control panel only draft changes', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/proxy-routes/')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              message: '',
              data: [buildRoute()],
            }),
          ),
        );
      }

      if (url.includes('/nodes/')) {
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

      if (url.includes('/config-versions/diff')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              message: '',
              data: buildDiff({
                snapshot_changed: true,
                runtime_config_changed: false,
              }),
            }),
          ),
        );
      }

      if (url.includes('/config-versions/publish')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: false,
              message: 'publish should not be called',
            }),
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderWithProviders(<ProxyRoutesPage />);

    expect(await screen.findByText('仅面板信息变更')).toBeInTheDocument();
    expect(await screen.findByText('无需发布')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '发布配置' }));

    await waitFor(() => {
      expect(
        screen.getByText('当前只有面板展示信息变更，不会改变节点运行配置，无需发布。'),
      ).toBeInTheDocument();
    });
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes('/config-versions/publish'),
      ),
    ).toBe(false);
  });

  it('shows proxy service option changes in the publish confirmation', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';

      if (url.includes('/proxy-routes/')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              message: '',
              data: [buildRoute()],
            }),
          ),
        );
      }

      if (url.includes('/nodes/')) {
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

      if (url.includes('/config-versions/diff')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              message: '',
              data: buildDiff({
                snapshot_changed: true,
                runtime_config_changed: true,
                main_config_changed: true,
                changed_option_keys: ['OpenRestyWorkerProcesses'],
              }),
            }),
          ),
        );
      }

      if (url.includes('/config-versions/publish') && method === 'POST') {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              message: '',
              data: {
                id: 2,
                version: '20260401-001',
                checksum: 'checksum',
                is_active: true,
                created_by: 'root',
                created_at: '2026-04-01T00:00:00Z',
                snapshot_json: '{}',
                main_config: '',
                rendered_config: '',
                support_files_json: '[]',
              },
            }),
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    renderWithProviders(<ProxyRoutesPage />);

    expect(await screen.findByText('有待发布变更')).toBeInTheDocument();
    expect(await screen.findByText('代理服务参数：1')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '发布配置' }));

    const dialog = await screen.findByRole('dialog', {
      name: '发布当前配置',
    });
    expect(within(dialog).getByText(/代理服务参数 1 项/)).toBeInTheDocument();
    expect(within(dialog).getByText(/主配置变更/)).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: '发布' }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes('/config-versions/publish'),
        ),
      ).toBe(true);
    });
  });

  it('renders selected feature section as an expandable configuration page', async () => {
    const user = userEvent.setup();
    searchParamsMock = new URLSearchParams('section=cache');
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/proxy-routes/9/cache/purge')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: {
                  target_nodes: ['node-default'],
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
                  buildRoute({
                    cache_enabled: true,
                    cache_policy: 'path_prefix',
                    cache_rule_list: ['/assets', '/static'],
                  }),
                ],
              }),
            ),
          );
        }

        if (url.includes('/config-versions/diff')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildDiff(),
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      });
    vi.stubGlobal('fetch', fetchMock);

    renderWithProviders(<ProxyRoutesPage />);

    expect(await screen.findByText('缓存策略配置')).toBeInTheDocument();
    expect(screen.getByText('缓存已启用')).toBeInTheDocument();
    expect(screen.getByText('启用站点缓存')).toBeInTheDocument();
    expect(screen.getByText('缓存规则')).toBeInTheDocument();
    expect(screen.getByLabelText('缓存规则 1')).toHaveValue('/assets');
    expect(screen.getByLabelText('缓存规则 2')).toHaveValue('/static');
    await user.click(screen.getByRole('button', { name: '清理全部缓存' }));
    expect(screen.getByRole('dialog', { name: '清理全部缓存' })).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some((call) =>
        String(call[0]).includes('/proxy-routes/9/cache/purge'),
      ),
    ).toBe(false);

    await user.click(screen.getByRole('button', { name: '清理缓存' }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some((call) =>
          String(call[0]).includes('/proxy-routes/9/cache/purge'),
        ),
      ).toBe(true);
    });
    expect(screen.getByRole('link', { name: '配置缓存策略' })).toHaveAttribute(
      'href',
      '/proxy-route/detail?id=9&section=cache',
    );
  });

  it('creates a website and navigates to config page', async () => {
    const routes: Array<Record<string, unknown>> = [];
    const createRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/') && method === 'POST') {
          const payload = JSON.parse(String(init?.body));
          createRequests.push(payload);
          const created = buildRoute({
            id: 21,
            site_name: payload.site_name,
            domain: payload.domain,
            domains: payload.domains,
            primary_domain: payload.domain,
            domain_count: payload.domains.length,
            origin_url: payload.origin_url,
            upstreams: JSON.stringify([
              payload.origin_url,
              ...payload.upstreams,
            ]),
            upstream_list: [payload.origin_url, ...payload.upstreams],
            enabled: payload.enabled,
            enable_https: payload.enable_https,
            cert_id: payload.cert_id,
            cert_ids: payload.cert_ids ?? [],
            domain_cert_ids: payload.domain_cert_ids ?? [],
            node_pool: payload.node_pool ?? 'default',
            redirect_http: payload.redirect_http,
            limit_conn_per_server: 0,
            limit_conn_per_ip: 0,
            limit_rate: '',
            cache_enabled: false,
            cache_policy: 'url',
            cache_rules: '[]',
            cache_rule_list: [],
            custom_headers: '[]',
            custom_header_list: [],
            dns_auto_sync: payload.dns_auto_sync ?? false,
            dns_account_id: payload.dns_account_id ?? null,
            dns_zone_id: payload.dns_zone_id ?? '',
            dns_record_type: payload.dns_record_type ?? 'A',
            dns_record_name: payload.dns_record_name ?? '',
            dns_record_content: payload.dns_record_content ?? '',
            dns_auto_target: payload.dns_auto_target ?? false,
            dns_record_ids: {},
            cloudflare_proxied: payload.cloudflare_proxied ?? false,
            ddos_protection_mode: payload.ddos_protection_mode ?? 'off',
            ddos_protection_provider:
              payload.ddos_protection_provider ?? 'cloudflare',
            ddos_protection_target: payload.ddos_protection_target ?? '',
            dns_last_sync_status: '',
            dns_last_sync_message: '',
            dns_last_synced_at: null,
            dns_provider_mode: payload.dns_provider_mode ?? 'cloudflare',
            dns_zone_id_ref: payload.dns_zone_id_ref ?? null,
            region_restriction_enabled:
              payload.region_restriction_enabled ?? false,
            region_restriction_mode: payload.region_restriction_mode ?? 'block',
            region_restriction_countries:
              payload.region_restriction_countries ?? [],
            remark: payload.remark,
          });
          routes.splice(0, routes.length, created);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: created,
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
                data: routes,
              }),
            ),
          );
        }

        if (url.includes('/config-versions/diff')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildDiff(),
              }),
            ),
          );
        }

        if (url.includes('/managed-domains/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  { id: 1, domain: '*.example.com', cert_id: 1, enabled: true },
                ],
              }),
            ),
          );
        }

        if (url.includes('/tls-certificates/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    name: 'example-cert',
                    primary_domain: '*.example.com',
                    other_domains: '',
                    not_after: null,
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
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
                data: [{ id: 11, name: 'example.com', enabled: true }],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-default',
                    name: 'Default Edge',
                    ip: '203.0.113.10',
                    pool_name: 'default',
                  },
                  {
                    id: 2,
                    node_id: 'node-hk',
                    name: 'HK Edge',
                    ip: '203.0.113.20',
                    pool_name: 'edge-hk',
                  },
                ],
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<ProxyRoutesPage />);

    const user = userEvent.setup();
    const pageButtons = await screen.findAllByRole('button');
    await user.click(pageButtons[1]);

    const dialog = await screen.findByRole('dialog');
    expect(dialog).toBeInTheDocument();

    await waitFor(() => {
      expect(
        within(dialog).getByRole('option', { name: 'edge-hk' }),
      ).toBeInTheDocument();
    });
    await user.selectOptions(
      within(dialog).getByLabelText('节点池选择'),
      'edge-hk',
    );
    expect(within(dialog).getByLabelText('节点池名称')).toHaveValue('edge-hk');

    await user.type(
      within(dialog).getByPlaceholderText('marketing-site'),
      'launch-site',
    );

    const primaryDomainInput = within(dialog).getByLabelText('域名 1');
    await user.type(primaryDomainInput, 'app.exam');
    await user.click(
      await within(dialog).findByRole('button', { name: 'app.example.com' }),
    );

    await user.click(within(dialog).getByLabelText('新增域名输入框'));
    await user.type(within(dialog).getByLabelText('域名 2'), 'www.example.com');

    await user.type(
      within(dialog).getByLabelText('源站地址'),
      'https://origin-a.internal:443{enter}https://origin-b.internal:443',
    );

    await user.click(
      within(dialog).getByRole('checkbox', { name: /创建时启用负载均衡/ }),
    );
    let dnsAccountSelect: HTMLElement | undefined;
    await waitFor(() => {
      dnsAccountSelect = within(dialog)
        .getAllByLabelText(/Cloudflare 账号/)
        .find((item) => item.getAttribute('name') === 'dns_account_id');
      expect(dnsAccountSelect).toBeInTheDocument();
    });
    if (!dnsAccountSelect) {
      throw new Error('missing Cloudflare DNS account select');
    }
    await user.selectOptions(dnsAccountSelect, '7');
    await user.click(
      within(dialog).getByRole('checkbox', {
        name: /平时也开启 Cloudflare 代理/,
      }),
    );
    await user.selectOptions(
      within(dialog).getByLabelText(/攻击防护模式/),
      'auto',
    );

    const submitButton = document.querySelector(
      'button[form="create-website-form"]',
    ) as HTMLButtonElement | null;
    expect(submitButton).toBeInstanceOf(HTMLButtonElement);
    if (!submitButton) {
      throw new Error('missing create submit button');
    }
    await user.click(submitButton);

    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith(
        '/proxy-route/detail?id=21&section=domains',
      );
    });
    expect(createRequests[0]).toMatchObject({
      node_pool: 'edge-hk',
      dns_auto_sync: true,
      dns_account_id: 7,
      dns_provider_mode: 'cloudflare',
      dns_zone_id_ref: null,
      dns_record_type: 'A',
      dns_record_content: '',
      dns_auto_target: true,
      cloudflare_proxied: true,
      ddos_protection_mode: 'auto',
      ddos_protection_provider: 'cloudflare',
      ddos_protection_target: '7',
    });
  });

  it('only offers certificates that cover the typed website domain', async () => {
    const routes: Array<Record<string, unknown>> = [];
    const createRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/') && method === 'POST') {
          const payload = JSON.parse(String(init?.body));
          createRequests.push(payload);
          const created = buildRoute({
            id: 23,
            site_name: payload.site_name,
            domain: payload.domain,
            domains: payload.domains,
            primary_domain: payload.domain,
            domain_count: payload.domains.length,
            origin_url: payload.origin_url,
            upstreams: JSON.stringify([payload.origin_url]),
            upstream_list: [payload.origin_url],
            enabled: payload.enabled,
            enable_https: payload.enable_https,
            cert_id: payload.cert_id,
            cert_ids: payload.cert_ids ?? [],
            domain_cert_ids: payload.domain_cert_ids ?? [],
            node_pool: payload.node_pool ?? 'default',
            redirect_http: payload.redirect_http,
            remark: payload.remark,
          });
          routes.splice(0, routes.length, created);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: created,
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
                data: routes,
              }),
            ),
          );
        }

        if (url.includes('/config-versions/diff')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildDiff(),
              }),
            ),
          );
        }

        if (url.includes('/managed-domains/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  { id: 1, domain: '*.satandu.com', cert_id: 1, enabled: true },
                  { id: 2, domain: '*.satancu.com', cert_id: 2, enabled: true },
                ],
              }),
            ),
          );
        }

        if (url.includes('/tls-certificates/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    name: 'satandu-cert',
                    primary_domain: '*.satandu.com',
                    other_domains: '',
                    not_after: null,
                  },
                  {
                    id: 2,
                    name: 'satancu-cert',
                    primary_domain: '*.satancu.com',
                    other_domains: '',
                    not_after: null,
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
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
                data: [],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-default',
                    name: 'Default Edge',
                    ip: '203.0.113.10',
                    pool_name: 'default',
                  },
                ],
              }),
            ),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    renderWithProviders(<ProxyRoutesPage />);

    const user = userEvent.setup();
    const pageButtons = await screen.findAllByRole('button');
    await user.click(pageButtons[1]);

    const dialog = await screen.findByRole('dialog');
    const domainInput = within(dialog).getByLabelText('域名 1');
    await user.type(domainInput, '2fa.satancu.com');

    const certificateSelect = within(dialog).getByLabelText('证书 1');
    await waitFor(() => {
      expect(
        within(certificateSelect).getByRole('option', {
          name: 'satancu-cert (*.satancu.com)',
        }),
      ).toBeInTheDocument();
    });
    expect(
      within(certificateSelect).queryByRole('option', {
        name: 'satandu-cert (*.satandu.com)',
      }),
    ).not.toBeInTheDocument();

    await user.selectOptions(certificateSelect, '2');
    await user.click(
      within(dialog).getByRole('checkbox', {
        name: /HTTP 自动跳转到 HTTPS/,
      }),
    );
    await user.type(
      within(dialog).getByLabelText('源站地址'),
      'http://145.239.140.145:8070',
    );

    const submitButton = document.querySelector(
      'button[form="create-website-form"]',
    ) as HTMLButtonElement | null;
    expect(submitButton).toBeInstanceOf(HTMLButtonElement);
    if (!submitButton) {
      throw new Error('missing create submit button');
    }
    await user.click(submitButton);

    await waitFor(() => {
      expect(createRequests).toHaveLength(1);
    });
    expect(createRequests[0]).toMatchObject({
      domain: '2fa.satancu.com',
      domains: ['2fa.satancu.com'],
      enable_https: true,
      cert_id: 2,
      cert_ids: [2],
      domain_cert_ids: [2],
      redirect_http: true,
    });
  });

  it('creates a website with local authoritative DNS mode', async () => {
    const routes: Array<Record<string, unknown>> = [];
    const createRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/') && method === 'POST') {
          const payload = JSON.parse(String(init?.body));
          createRequests.push(payload);
          const created = buildRoute({
            id: 22,
            site_name: payload.site_name,
            domain: payload.domain,
            domains: payload.domains,
            primary_domain: payload.domain,
            domain_count: payload.domains.length,
            origin_url: payload.origin_url,
            upstreams: JSON.stringify([payload.origin_url]),
            upstream_list: [payload.origin_url],
            enabled: payload.enabled,
            enable_https: payload.enable_https,
            cert_id: payload.cert_id,
            cert_ids: payload.cert_ids ?? [],
            domain_cert_ids: payload.domain_cert_ids ?? [],
            node_pool: payload.node_pool ?? 'default',
            redirect_http: payload.redirect_http,
            dns_auto_sync: payload.dns_auto_sync ?? false,
            dns_account_id: payload.dns_account_id ?? null,
            dns_provider_mode: payload.dns_provider_mode ?? 'cloudflare',
            dns_zone_id_ref: payload.dns_zone_id_ref ?? null,
            dns_record_type: payload.dns_record_type ?? 'A',
            dns_record_content: payload.dns_record_content ?? '',
            dns_auto_target: payload.dns_auto_target ?? false,
            cloudflare_proxied: payload.cloudflare_proxied ?? false,
            ddos_protection_mode: payload.ddos_protection_mode ?? 'off',
            ddos_protection_provider:
              payload.ddos_protection_provider ?? 'cloudflare',
            ddos_protection_target: payload.ddos_protection_target ?? '',
            remark: payload.remark,
          });
          routes.splice(0, routes.length, created);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: created,
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
                data: routes,
              }),
            ),
          );
        }

        if (url.includes('/config-versions/diff')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildDiff(),
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
                data: [{ id: 11, name: 'example.com', enabled: true }],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-default',
                    name: 'Default Edge',
                    ip: '203.0.113.10',
                    pool_name: 'default',
                  },
                  {
                    id: 2,
                    node_id: 'node-hk',
                    name: 'HK Edge',
                    ip: '203.0.113.20',
                    pool_name: 'edge-hk',
                  },
                ],
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(<ProxyRoutesPage />);

    const user = userEvent.setup();
    const pageButtons = await screen.findAllByRole('button');
    await user.click(pageButtons[1]);

    const dialog = await screen.findByRole('dialog');
    await user.type(
      within(dialog).getByPlaceholderText('marketing-site'),
      'authoritative-site',
    );
    await user.type(within(dialog).getByLabelText('域名 1'), 'app.example.com');
    await user.type(
      within(dialog).getByLabelText('源站地址'),
      'https://origin.internal:443',
    );
    await user.click(
      within(dialog).getByRole('checkbox', { name: /创建时启用负载均衡/ }),
    );
    await user.selectOptions(within(dialog).getByLabelText('解析方式'), [
      'authoritative',
    ]);
    await waitFor(() => {
      expect(within(dialog).getByText('托管域名')).toBeInTheDocument();
    });
    await user.selectOptions(within(dialog).getByLabelText('托管域名'), '11');

    const submitButton = document.querySelector(
      'button[form="create-website-form"]',
    ) as HTMLButtonElement | null;
    expect(submitButton).toBeInstanceOf(HTMLButtonElement);
    if (!submitButton) {
      throw new Error('missing create submit button');
    }
    await user.click(submitButton);

    await waitFor(() => {
      expect(pushMock).toHaveBeenCalledWith(
        '/proxy-route/detail?id=22&section=domains',
      );
    });
    expect(createRequests[0]).toMatchObject({
      dns_provider_mode: 'authoritative',
      dns_zone_id_ref: 11,
      dns_auto_sync: false,
      dns_account_id: null,
      dns_record_type: 'A',
      dns_record_content: '',
      dns_auto_target: true,
      cloudflare_proxied: false,
      ddos_protection_mode: 'off',
      ddos_protection_target: '',
    });
  });

  it('saves selected node pool from reverse proxy config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  node_pool: payload.node_pool,
                  origin_url: payload.origin_url,
                  upstream_list: [
                    payload.origin_url,
                    ...(payload.upstreams as string[]),
                  ],
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute(),
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-default',
                    name: 'Default Edge',
                    ip: '203.0.113.10',
                    pool_name: 'default',
                  },
                  {
                    id: 2,
                    node_id: 'node-hk',
                    name: 'HK Edge',
                    ip: '203.0.113.20',
                    pool_name: 'edge-hk',
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
                data: [],
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="proxy" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '反向代理' }),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByLabelText('池内节点')).toHaveValue('node-default');
    });

    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'edge-hk' }),
      ).toBeInTheDocument();
    });
    await user.selectOptions(screen.getByLabelText('节点池选择'), 'edge-hk');
    expect(screen.getByLabelText('节点池选择')).toHaveValue('edge-hk');
    await user.selectOptions(screen.getByLabelText('代理缓冲模式'), 'off');
    await waitFor(() => {
      expect(screen.getByLabelText('池内节点')).toHaveValue('node-hk');
    });

    const saveButton = document.querySelector(
      'button[form="proxy-route-proxy-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing proxy save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      node_pool: 'edge-hk',
      origin_url: 'https://origin-a.internal:443',
      proxy_buffering_mode: 'off',
      upstreams: ['https://origin-b.internal:443'],
    });
  });

  it('saves domain settings from config page by section', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  site_name: payload.site_name,
                  domain: (payload.domains as string[])[0],
                  domains: payload.domains,
                  primary_domain: (payload.domains as string[])[0],
                  domain_count: (payload.domains as string[]).length,
                  enabled: payload.enabled,
                  enable_https: payload.enable_https,
                  cert_id: payload.cert_id,
                  cert_ids: payload.cert_ids,
                  domain_cert_ids: payload.domain_cert_ids,
                  redirect_http: payload.redirect_http,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute(),
              }),
            ),
          );
        }

        if (url.includes('/tls-certificates/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    name: 'example-cert',
                    primary_domain: '*.example.com',
                    other_domains: '',
                    not_after: null,
                  },
                ],
              }),
            ),
          );
        }

        if (url.includes('/managed-domains/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  { id: 1, domain: '*.example.com', cert_id: 1, enabled: true },
                ],
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="domains" />,
    );

    const user = userEvent.setup();
    expect(await screen.findByText('marketing-site')).toBeInTheDocument();

    const siteNameInput = screen.getByPlaceholderText('marketing-site');
    await user.clear(siteNameInput);
    await user.type(siteNameInput, 'brand-site');

    const primaryDomainInput = screen.getByLabelText('域名 1');
    await user.clear(primaryDomainInput);
    await user.type(primaryDomainInput, 'brand.example.com');

    const secondaryDomainInput = screen.getByLabelText('域名 2');
    await user.clear(secondaryDomainInput);
    await user.type(secondaryDomainInput, 'www.brand.example.com');

    await user.selectOptions(screen.getByLabelText('证书 1'), '1');
    await user.selectOptions(screen.getByLabelText('证书 2'), '');

    const saveButton = document.querySelector(
      'button[form="proxy-route-domains-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing domain save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      site_name: 'brand-site',
      domain: 'brand.example.com',
      domains: ['brand.example.com', 'www.brand.example.com'],
      enabled: true,
      enable_https: true,
      cert_id: 1,
      cert_ids: [1],
      domain_cert_ids: [1, 0],
      redirect_http: true,
    });
  });

  it('saves GSLB pool rows from automatic DNS config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];
    const dnsRoute = buildRoute({
      dns_auto_sync: true,
      dns_account_id: 7,
      dns_record_type: 'A',
      dns_auto_target: true,
      dns_target_count: 2,
      dns_schedule_mode: 'load_aware',
      dns_ttl: 60,
      gslb_enabled: true,
      gslb_policy: {
        mode: 'cloudflare_dns',
        strategy: 'load_aware',
        pools: [
          {
            name: 'default',
            weight: 100,
            countries: [],
            enabled: true,
          },
        ],
        target_count: 2,
        ttl: 60,
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
    });

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  ...dnsRoute,
                  dns_ttl: payload.dns_ttl,
                  gslb_enabled: payload.gslb_enabled,
                  gslb_policy: payload.gslb_policy,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: dnsRoute,
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
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
                data: [],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-hk',
                    name: 'HK Edge',
                    ip: '203.0.113.10',
                    pool_name: 'hk',
                  },
                  {
                    id: 2,
                    node_id: 'node-hk-backup',
                    name: 'HK Backup',
                    ip: '203.0.113.11',
                    pool_name: 'hk',
                  },
                  {
                    id: 3,
                    node_id: 'node-eu',
                    name: 'EU Edge',
                    ip: '203.0.113.20',
                    pool_name: 'eu',
                  },
                ],
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="dns" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '负载均衡' }),
    ).toBeInTheDocument();
    expect(screen.getByText(/2-29 秒会在保存时提升到 30 秒/)).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('节点池选择 1'), 'hk');
    expect(
      screen.getByRole('checkbox', { name: /节点池 hk 节点 HK Edge/ }),
    ).toBeChecked();
    expect(
      screen.getByRole('checkbox', { name: /节点池 hk 节点 HK Backup/ }),
    ).toBeChecked();
    await user.click(
      screen.getByRole('checkbox', { name: /节点池 hk 节点 HK Backup/ }),
    );
    await user.clear(screen.getByLabelText('节点池权重 1'));
    await user.type(screen.getByLabelText('节点池权重 1'), '80');
    await user.type(screen.getByLabelText('节点池国家或地区 1'), 'HK,TW');
    await user.click(
      within(
        screen.getByRole('group', { name: '节点池访问运营商 1' }),
      ).getByRole('checkbox', { name: '电信' }),
    );
    await user.type(
      screen.getByLabelText('节点池 ASN 1'),
      'AS4134 as4837 asn:4134 4294967296 0 bad',
    );

    await user.click(screen.getByLabelText('新增节点池'));
    await user.selectOptions(screen.getByLabelText('节点池选择 2'), 'eu');
    expect(
      screen.getByRole('checkbox', { name: /节点池 eu 节点 EU Edge/ }),
    ).toBeChecked();
    await user.clear(screen.getByLabelText('节点池权重 2'));
    await user.type(screen.getByLabelText('节点池权重 2'), '20');
    await user.type(screen.getByLabelText('节点池国家或地区 2'), 'DE FR');

    const saveButton = document.querySelector(
      'button[form="proxy-route-dns-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing dns save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      dns_ttl: 60,
      gslb_enabled: true,
      gslb_policy: {
        source_pool_fallback_mode: 'strict',
        pools: [
          {
            name: 'hk',
            weight: 80,
            countries: ['HK', 'TW'],
            operators: ['cn-telecom'],
            asns: [4134, 4837],
            node_ids: ['node-hk'],
            enabled: true,
          },
          {
            name: 'eu',
            weight: 20,
            countries: ['DE', 'FR'],
            node_ids: [],
            enabled: true,
          },
        ],
      },
    });
  });

  it('syncs GSLB pool rows from current node pools only', async () => {
    const dnsRoute = buildRoute({
      node_pool: 'jp',
      dns_auto_sync: true,
      dns_account_id: 7,
      dns_record_type: 'A',
      dns_auto_target: true,
      dns_target_count: 2,
      dns_schedule_mode: 'weighted',
      dns_ttl: 60,
      gslb_enabled: true,
      gslb_policy: {
        mode: 'cloudflare_dns',
        strategy: 'weighted',
        pools: [
          {
            name: 'jp',
            weight: 1,
            countries: ['JP'],
            enabled: true,
          },
          {
            name: 'eu',
            weight: 99,
            countries: ['DE'],
            enabled: true,
          },
        ],
        target_count: 2,
        ttl: 60,
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
    });

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: dnsRoute,
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
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
                data: [],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  {
                    id: 1,
                    node_id: 'node-hk',
                    name: '香港节点',
                    ip: '203.0.113.10',
                    pool_name: '香港',
                  },
                  {
                    id: 2,
                    node_id: 'node-eu',
                    name: '欧洲节点',
                    ip: '203.0.113.20',
                    pool_name: '欧洲',
                  },
                ],
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="dns" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '负载均衡' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/当前填写的“jp”不在现有节点池里/),
    ).toBeInTheDocument();

    const firstPoolSelect = screen.getByLabelText(
      '节点池选择 1',
    ) as HTMLSelectElement;
    expect(
      within(firstPoolSelect).queryByRole('option', { name: 'jp' }),
    ).not.toBeInTheDocument();
    expect(
      within(firstPoolSelect).getByRole('option', { name: '香港' }),
    ).toBeInTheDocument();
    expect(
      within(firstPoolSelect).getByRole('option', { name: '欧洲' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '同步现有节点池' }));

    expect(
      screen.queryByText(/当前填写的“jp”不在现有节点池里/),
    ).not.toBeInTheDocument();
    expect(screen.getAllByDisplayValue('香港').length).toBeGreaterThan(0);
    expect(screen.getAllByDisplayValue('欧洲').length).toBeGreaterThan(0);
    expect(
      screen.getByRole('checkbox', { name: /节点池 香港 节点 香港节点/ }),
    ).toBeChecked();
    expect(
      screen.getByRole('checkbox', { name: /节点池 欧洲 节点 欧洲节点/ }),
    ).toBeChecked();
  });

  it('saves authoritative DNS mode from automatic DNS config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  dns_provider_mode: payload.dns_provider_mode,
                  dns_zone_id_ref: payload.dns_zone_id_ref,
                  dns_auto_sync: payload.dns_auto_sync,
                  dns_account_id: payload.dns_account_id,
                  cloudflare_proxied: payload.cloudflare_proxied,
                  ddos_protection_mode: payload.ddos_protection_mode,
                  ddos_protection_provider: payload.ddos_protection_provider,
                  ddos_protection_target: payload.ddos_protection_target,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute(),
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
                    id: 11,
                    name: 'example.com',
                    soa_email: 'hostmaster@example.com',
                    primary_ns: 'ns1.example.net',
                    name_servers: ['ns1.example.net', 'ns2.example.net'],
                    default_ttl: 300,
                    serial: 2026053101,
                    enabled: true,
                    record_count: 0,
                    created_at: '2026-05-31T08:00:00Z',
                    updated_at: '2026-05-31T08:00:00Z',
                  },
                ],
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/nodes/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="dns" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '负载均衡' }),
    ).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('解析模式'), 'authoritative');
    await user.selectOptions(screen.getByLabelText('托管域名'), '11');

    const saveButton = document.querySelector(
      'button[form="proxy-route-dns-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing dns save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      dns_provider_mode: 'authoritative',
      dns_zone_id_ref: 11,
      dns_auto_sync: false,
      dns_account_id: null,
      dns_auto_target: true,
      cloudflare_proxied: false,
      ddos_protection_mode: 'off',
      ddos_protection_provider: 'cloudflare',
      ddos_protection_target: '',
    });
  });

  it('saves custom DDoS protection target from automatic DNS config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];
    const dnsRoute = buildRoute({
      dns_auto_sync: true,
      dns_account_id: 7,
      dns_auto_target: true,
      dns_provider_mode: 'cloudflare',
      ddos_protection_mode: 'off',
      ddos_protection_provider: 'cloudflare',
      ddos_protection_target: '',
    });

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  ...dnsRoute,
                  ddos_protection_mode: payload.ddos_protection_mode,
                  ddos_protection_provider: payload.ddos_protection_provider,
                  ddos_protection_target: payload.ddos_protection_target,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: dnsRoute,
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
              }),
            ),
          );
        }

        if (url.includes('/nodes/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [
                  { id: 1, pool_name: 'default' },
                  { id: 2, pool_name: 'anti-ddos' },
                ],
              }),
            ),
          );
        }

        if (
          url.includes('/dns-zones/') ||
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="dns" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '负载均衡' }),
    ).toBeInTheDocument();

    await user.selectOptions(
      await screen.findByLabelText('攻击防护模式'),
      'auto',
    );
    await user.selectOptions(screen.getByLabelText('防护提供方'), 'custom');
    await user.selectOptions(screen.getByLabelText('清洗池'), 'anti-ddos');

    const saveButton = document.querySelector(
      'button[form="proxy-route-dns-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing dns save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      ddos_protection_mode: 'auto',
      ddos_protection_provider: 'custom',
      ddos_protection_target: 'anti-ddos',
    });
  });

  it('explains automatic DNS scheduling semantics on config page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  dns_auto_sync: true,
                  dns_account_id: 7,
                  dns_auto_target: true,
                  dns_schedule_mode: 'healthy',
                }),
              }),
            ),
          );
        }

        if (url.includes('/dns-accounts/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: [{ id: 7, name: 'cf-main', type: 'cloudflare' }],
              }),
            ),
          );
        }

        if (
          url.includes('/dns-zones/') ||
          url.includes('/nodes/') ||
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="dns" />,
    );

    expect(
      await screen.findByRole('heading', { name: '负载均衡' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/健康优先只看节点是否在线、代理服务是否正常/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('option', { name: '健康优先（冷却防抖）' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/返回哪些 IP 由下方节点池权重决定/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/攻击自动防护只在攻击期间临时覆盖解析目标/),
    ).toBeInTheDocument();
  });

  it('saves WAF settings from CC protection config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  waf_enabled: payload.waf_enabled,
                  waf_mode: payload.waf_mode,
                  waf_config: JSON.parse(String(payload.waf_config)),
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  waf_enabled: false,
                  waf_mode: 'block',
                  waf_config: {
                    builtin_rules: [
                      'sqli',
                      'xss',
                      'path_traversal',
                      'sensitive_paths',
                      'bad_bots',
                    ],
                    whitelist: {
                      ips: [],
                      ip_cidrs: [],
                      paths: [],
                    },
                    block_rules: {
                      path_contains: [],
                      path_regexes: [],
                      query_contains: [],
                      header_contains: [],
                      user_agents: [],
                    },
                  },
                }),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="cc" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: 'CC 防护' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('checkbox', { name: /启用恶意请求规则/ }));
    await user.selectOptions(screen.getByLabelText('恶意请求规则运行模式'), 'log');
    await user.click(screen.getByRole('checkbox', { name: /恶意工具 UA/ }));

    expect(screen.queryByPlaceholderText('1.2.3.4')).not.toBeInTheDocument();
    const wafRuleSelect = screen.getByLabelText(
      '按需添加白名单或拦截规则',
    );
    const addWAFRuleButton = screen.getByRole('button', {
      name: '添加恶意请求规则',
    });
    for (const ruleKey of [
      'waf_whitelist.ips',
      'waf_whitelist.ip_cidrs',
      'waf_whitelist.paths',
      'waf_block_rules.path_contains',
      'waf_block_rules.path_regexes',
      'waf_block_rules.query_contains',
      'waf_block_rules.header_contains',
      'waf_block_rules.user_agents',
    ]) {
      await user.selectOptions(wafRuleSelect, ruleKey);
      await user.click(addWAFRuleButton);
    }

    await user.type(screen.getByPlaceholderText('1.2.3.4'), '203.0.113.8');
    await user.type(screen.getByPlaceholderText('10.0.0.0/8'), '10.10.0.0/16');
    await user.type(screen.getByPlaceholderText('/api/public/*'), '/health/*');
    await user.type(screen.getByPlaceholderText('/debug'), '/debug');
    await user.type(screen.getByPlaceholderText('^/private/'), '^/admin/');
    await user.type(screen.getByPlaceholderText('debug=true'), 'trace=1');
    await user.type(screen.getByPlaceholderText('X-Scanner'), 'X-Exploit');
    await user.type(screen.getByPlaceholderText('sqlmap'), 'curl');

    const saveButton = document.querySelector(
      'button[form="proxy-route-cc-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing CC save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      waf_enabled: true,
      waf_mode: 'log',
    });
    expect(JSON.parse(String(updateRequests[0].waf_config))).toEqual({
      builtin_rules: ['sqli', 'xss', 'path_traversal', 'sensitive_paths'],
      whitelist: {
        ips: ['203.0.113.8'],
        ip_cidrs: ['10.10.0.0/16'],
        paths: ['/health/*'],
      },
      block_rules: {
        path_contains: ['/debug'],
        path_regexes: ['^/admin/'],
        query_contains: ['trace=1'],
        header_contains: ['X-Exploit'],
        user_agents: ['curl'],
      },
    });
  });

  it('saves CC frequency settings from config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  cc_enabled: payload.cc_enabled,
                  cc_mode: payload.cc_mode,
                  cc_config: JSON.parse(String(payload.cc_config)),
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute(),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="cc" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: 'CC 防护' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('checkbox', { name: /启用 CC 频率防护/ }));
    await user.selectOptions(screen.getByLabelText('CC 触发动作'), 'pow');

    const numberInputs = screen.getAllByRole('spinbutton');
    await user.clear(numberInputs[0]);
    await user.type(numberInputs[0], '20');
    await user.clear(numberInputs[1]);
    await user.type(numberInputs[1], '200');
    await user.clear(numberInputs[2]);
    await user.type(numberInputs[2], '15');
    await user.clear(numberInputs[3]);
    await user.type(numberInputs[3], '80');
    await user.clear(numberInputs[4]);
    await user.type(numberInputs[4], '600');

    const ccRuleSelect = screen.getByLabelText('按需添加 CC 规则');
    const addCCRuleButton = screen.getByRole('button', {
      name: '添加 CC 防护规则',
    });
    for (const ruleKey of ['cc_whitelist.paths', 'cc_exclude.user_agents']) {
      await user.selectOptions(ccRuleSelect, ruleKey);
      await user.click(addCCRuleButton);
    }

    const textareas = Array.from(document.querySelectorAll('textarea'));
    expect(textareas).toHaveLength(2);
    await user.type(textareas[0], '/api/internal/*');
    await user.type(textareas[1], 'monitor');

    const saveButton = document.querySelector(
      'button[form="proxy-route-cc-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing CC save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      cc_enabled: true,
      cc_mode: 'pow',
    });
    expect(JSON.parse(String(updateRequests[0].cc_config))).toEqual({
      window_seconds: 20,
      max_requests: 200,
      path_window_seconds: 15,
      path_max_requests: 80,
      block_duration_seconds: 600,
      whitelist: {
        ips: [],
        ip_cidrs: [],
        paths: ['/api/internal/*'],
        user_agents: [],
      },
      exclude: {
        ips: [],
        ip_cidrs: [],
        paths: [],
        user_agents: ['monitor'],
      },
    });
  });

  it('saves PoW settings from CC protection config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  pow_enabled: payload.pow_enabled,
                  pow_config: JSON.parse(String(payload.pow_config)),
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  pow_enabled: false,
                  pow_config: {
                    difficulty: 4,
                    algorithm: 'fast',
                    session_ttl: 600,
                    challenge_ttl: 300,
                    whitelist: {
                      ips: [],
                      ip_cidrs: [],
                      paths: [],
                      path_regexes: [],
                      user_agents: [],
                    },
                    blacklist: {
                      ips: [],
                      ip_cidrs: [],
                      paths: [],
                      path_regexes: [],
                      user_agents: [],
                    },
                  },
                }),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="cc" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: 'CC 防护' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('checkbox', { name: /启用计算验证/ }));
    await user.selectOptions(screen.getByLabelText('验证算法'), 'slow');

    const [
      ,
      ,
      ,
      ,
      ,
      difficultyInput,
      sessionTTLInput,
      challengeTTLInput,
    ] =
      screen.getAllByRole('spinbutton');

    await user.clear(difficultyInput);
    await user.type(difficultyInput, '6');

    await user.clear(sessionTTLInput);
    await user.type(sessionTTLInput, '900');

    await user.clear(challengeTTLInput);
    await user.type(challengeTTLInput, '120');

    const powRuleSelect = screen.getByLabelText('按需添加计算验证规则');
    const addPowRuleButton = screen.getByRole('button', {
      name: '添加计算验证规则',
    });
    for (const ruleKey of [
      'pow_whitelist.ips',
      'pow_whitelist.paths',
      'pow_whitelist.user_agents',
      'pow_blacklist.ip_cidrs',
      'pow_blacklist.path_regexes',
    ]) {
      await user.selectOptions(powRuleSelect, ruleKey);
      await user.click(addPowRuleButton);
    }

    const powTextareas = Array.from(document.querySelectorAll('textarea'));
    expect(powTextareas).toHaveLength(5);

    await user.type(powTextareas[0], '203.0.113.8');
    await user.type(powTextareas[1], '/health/*');
    await user.type(powTextareas[2], 'Googlebot');
    await user.type(powTextareas[3], '10.10.0.0/16');
    await user.type(powTextareas[4], '^/private/');

    const saveButton = document.querySelector(
      'button[form="proxy-route-cc-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing CC save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      pow_enabled: true,
    });
    expect(JSON.parse(String(updateRequests[0].pow_config))).toEqual({
      difficulty: 6,
      algorithm: 'slow',
      session_ttl: 900,
      challenge_ttl: 120,
      whitelist: {
        ips: ['203.0.113.8'],
        ip_cidrs: [],
        paths: ['/health/*'],
        path_regexes: [],
        user_agents: ['Googlebot'],
      },
      blacklist: {
        ips: [],
        ip_cidrs: ['10.10.0.0/16'],
        paths: [],
        path_regexes: ['^/private/'],
        user_agents: [],
      },
    });
  });

  it('saves Basic Auth settings from config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  basic_auth_enabled: payload.basic_auth_enabled,
                  basic_auth_username: payload.basic_auth_username,
                  basic_auth_password: payload.basic_auth_password,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  basic_auth_enabled: false,
                  basic_auth_username: '',
                  basic_auth_password: '',
                }),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="auth" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '认证配置' }),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('checkbox', { name: /启用 Basic Auth 鉴权/ }),
    );
    await user.type(screen.getByPlaceholderText('admin'), ' edge-admin ');
    await user.type(screen.getByPlaceholderText('secret123'), ' edge-secret ');

    const saveButton = document.querySelector(
      'button[form="proxy-route-auth-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing Basic Auth save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      basic_auth_enabled: true,
      basic_auth_username: 'edge-admin',
      basic_auth_password: 'edge-secret',
    });
  });

  it('saves region restriction settings from config page', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  region_restriction_enabled:
                    payload.region_restriction_enabled,
                  region_restriction_mode: payload.region_restriction_mode,
                  region_restriction_countries:
                    payload.region_restriction_countries,
                }),
              }),
            ),
          );
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute(),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="region" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '地区限制' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('checkbox', { name: /启用地区限制/ }));
    await user.selectOptions(screen.getByRole('combobox'), 'allow');
    await user.type(screen.getByRole('textbox'), 'CN, US');

    const saveButton = document.querySelector(
      'button[form="proxy-route-region-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing region save button');
    }
    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
    });

    expect(updateRequests[0]).toMatchObject({
      region_restriction_enabled: true,
      region_restriction_mode: 'allow',
      region_restriction_countries: ['CN', 'US'],
    });
  });

  it('shows loading state while cache config dependencies are loading', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

        if (url.includes('/proxy-routes/9')) {
          return new Promise<Response>(() => undefined);
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="cache" />,
    );

    expect(
      document.querySelector('.animate-pulse'),
    ).toBeInstanceOf(HTMLElement);
    expect(screen.queryByText('缓存')).not.toBeInTheDocument();
  });

  it('keeps cache save pending state visible and shows server failure toast', async () => {
    const updateRequests: Array<Record<string, unknown>> = [];
    let rejectUpdate: ((reason?: unknown) => void) | undefined;

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method?.toUpperCase() ?? 'GET';

        if (url.includes('/proxy-routes/9/update') && method === 'POST') {
          const payload = JSON.parse(String(init?.body)) as Record<
            string,
            unknown
          >;
          updateRequests.push(payload);

          return new Promise<Response>((_resolve, reject) => {
            rejectUpdate = reject;
          });
        }

        if (url.includes('/proxy-routes/9')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                success: true,
                message: '',
                data: buildRoute({
                  cache_enabled: true,
                  cache_policy: 'path_prefix',
                  cache_rule_list: ['/assets'],
                }),
              }),
            ),
          );
        }

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/dns-accounts/')
        ) {
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

    renderWithProviders(
      <ProxyRouteConfigPage routeId="9" initialSection="cache" />,
    );

    const user = userEvent.setup();
    expect(
      await screen.findByRole('heading', { name: '缓存' }),
    ).toBeInTheDocument();

    await user.selectOptions(screen.getByRole('combobox'), 'path_contains_all');
    const firstRuleInput = screen.getByLabelText('缓存规则 1');
    await user.clear(firstRuleInput);
    await user.type(firstRuleInput, '/emby/Items/');
    await user.click(screen.getByRole('button', { name: '添加缓存规则' }));
    await user.type(screen.getByLabelText('缓存规则 2'), '/Images');

    const saveButton = document.querySelector(
      'button[form="proxy-route-cache-form"]',
    ) as HTMLButtonElement | null;
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    if (!saveButton) {
      throw new Error('missing cache save button');
    }

    await user.click(saveButton);

    await waitFor(() => {
      expect(updateRequests).toHaveLength(1);
      expect(saveButton).toHaveTextContent('保存中...');
      expect(saveButton).toBeDisabled();
    });

    expect(updateRequests[0]).toMatchObject({
      cache_enabled: true,
      cache_policy: 'path_contains_all',
      cache_rules: ['/emby/Items/', '/Images'],
    });

    rejectUpdate?.(new Error('缓存策略保存失败：节点池不可用'));

    expect(
      await screen.findByText('缓存策略保存失败：节点池不可用'),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(saveButton).toHaveTextContent('保存');
      expect(saveButton).not.toBeDisabled();
    });
  });
});
