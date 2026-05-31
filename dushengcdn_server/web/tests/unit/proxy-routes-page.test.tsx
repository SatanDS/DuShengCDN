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
    dns_last_sync_status: '',
    dns_last_sync_message: '',
    dns_last_synced_at: null,
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
                data: [buildRoute()],
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
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      '/proxy-route/detail?id=9&section=domains',
    );
  });

  it('renders selected feature section as an expandable configuration page', async () => {
    searchParamsMock = new URLSearchParams('section=cache');
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
      }),
    );

    renderWithProviders(<ProxyRoutesPage />);

    expect(await screen.findByText('缓存策略配置')).toBeInTheDocument();
    expect(screen.getByText('缓存已启用')).toBeInTheDocument();
    expect(screen.getByText('启用站点缓存')).toBeInTheDocument();
    expect(screen.getByText('缓存规则')).toBeInTheDocument();
    expect(
      screen.getByDisplayValue(
        (value) => value.includes('/assets') && value.includes('/static'),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: '清理全部缓存' }),
    ).toBeInTheDocument();
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
            dns_last_sync_status: '',
            dns_last_sync_message: '',
            dns_last_synced_at: null,
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
                data: [{ id: 1, name: 'example-cert', not_after: null }],
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
                data: [{ id: 1, pool_name: 'edge-hk' }],
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
      within(dialog).getByRole('checkbox', { name: /创建时自动解析 DNS/ }),
    );
    await waitFor(() => {
      expect(within(dialog).getByText('DNS 账号')).toBeInTheDocument();
    });
    await user.selectOptions(within(dialog).getByLabelText(/DNS 账号/), '7');
    await user.click(
      within(dialog).getByRole('checkbox', { name: /开启 Cloudflare 代理/ }),
    );
    await user.selectOptions(
      within(dialog).getByLabelText(/DDoS 防护模式/),
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
      dns_record_type: 'A',
      dns_record_content: '',
      dns_auto_target: true,
      cloudflare_proxied: true,
      ddos_protection_mode: 'auto',
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
                data: [{ id: 1, pool_name: 'edge-hk' }],
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
      expect(
        screen.getByRole('option', { name: 'edge-hk' }),
      ).toBeInTheDocument();
    });
    await user.selectOptions(screen.getByLabelText('节点池选择'), 'edge-hk');
    expect(screen.getByLabelText('节点池名称')).toHaveValue('edge-hk');

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
                data: [{ id: 1, name: 'example-cert', not_after: null }],
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

        if (
          url.includes('/tls-certificates/') ||
          url.includes('/managed-domains/') ||
          url.includes('/nodes/')
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
      await screen.findByRole('heading', { name: '自动 DNS' }),
    ).toBeInTheDocument();
    expect(screen.getByText(/2-29 秒会在保存时提升到 30 秒/)).toBeInTheDocument();

    await user.clear(screen.getByLabelText('节点池名称 1'));
    await user.type(screen.getByLabelText('节点池名称 1'), 'hk');
    await user.clear(screen.getByLabelText('节点池权重 1'));
    await user.type(screen.getByLabelText('节点池权重 1'), '80');
    await user.type(screen.getByLabelText('节点池国家代码 1'), 'HK,TW');

    await user.click(screen.getByLabelText('新增节点池'));
    await user.type(screen.getByLabelText('节点池名称 2'), 'eu');
    await user.clear(screen.getByLabelText('节点池权重 2'));
    await user.type(screen.getByLabelText('节点池权重 2'), '20');
    await user.type(screen.getByLabelText('节点池国家代码 2'), 'DE FR');

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
        pools: [
          {
            name: 'hk',
            weight: 80,
            countries: ['HK', 'TW'],
            enabled: true,
          },
          {
            name: 'eu',
            weight: 20,
            countries: ['DE', 'FR'],
            enabled: true,
          },
        ],
      },
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
});
