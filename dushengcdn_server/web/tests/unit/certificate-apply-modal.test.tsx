import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { CertificateApplyModal } from '@/features/websites/components/certificate-apply-modal';

function renderWithQueryClient(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
  );

  return queryClient;
}

describe('CertificateApplyModal', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('prefers local authoritative DNS when certificate domain matches a managed zone', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);

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

    renderWithQueryClient(
      <CertificateApplyModal isOpen onClose={vi.fn()} onApplied={vi.fn()} />,
    );

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('主域名'), 'www.example.com');

    await waitFor(() => {
      expect(screen.getByLabelText('验证方式')).toHaveValue('authoritative');
    });
    expect(await screen.findByLabelText('本地托管域名')).toHaveValue('11');
  });

  it('submits certificate application with local authoritative DNS zone', async () => {
    const onClose = vi.fn();
    const onApplied = vi.fn();
    let applyPayload: Record<string, unknown> | null = null;

    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);

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

        if (url.includes('/tls-certificates/apply')) {
          applyPayload = JSON.parse(String(init?.body ?? '{}')) as Record<
            string,
            unknown
          >;
          return Promise.resolve(
            Response.json({
              success: true,
              message: '',
              data: {
                id: 21,
                name: 'example-cert',
                provider: 'acme',
                acme_account_id: applyPayload.acme_account_id,
                dns_account_id: applyPayload.dns_account_id,
                dns_provider_mode: applyPayload.dns_provider_mode,
                dns_zone_id_ref: applyPayload.dns_zone_id_ref,
                key_algorithm: applyPayload.key_algorithm,
                auto_renew: applyPayload.auto_renew,
                primary_domain: applyPayload.primary_domain,
                other_domains: applyPayload.other_domains,
                disable_cname: applyPayload.disable_cname,
                skip_dns: applyPayload.skip_dns,
                dns1: applyPayload.dns1,
                dns2: applyPayload.dns2,
                apply_status: 'applying',
                apply_message: '',
                not_before: '',
                not_after: '',
                remark: '',
                created_at: '',
                updated_at: '',
              },
            }),
          );
        }

        return Promise.reject(new Error(`Unhandled fetch: ${url}`));
      }),
    );

    const queryClient = renderWithQueryClient(
      <CertificateApplyModal
        isOpen
        onClose={onClose}
        onApplied={onApplied}
      />,
    );
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const user = userEvent.setup();
    await user.type(screen.getByLabelText('证书名称'), 'example-cert');
    await user.type(screen.getByLabelText('主域名'), 'www.example.com');
    await user.selectOptions(screen.getByLabelText('验证方式'), 'authoritative');
    await user.selectOptions(
      await screen.findByLabelText('本地托管域名'),
      '11',
    );
    await user.click(screen.getByRole('button', { name: '开始申请' }));

    await waitFor(() => {
      expect(applyPayload).toMatchObject({
        name: 'example-cert',
        primary_domain: 'www.example.com',
        dns_provider_mode: 'authoritative',
        dns_zone_id_ref: 11,
        dns_account_id: 0,
      });
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['tls-certificates'],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['managed-domains'],
    });
    expect(onApplied).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });
});
