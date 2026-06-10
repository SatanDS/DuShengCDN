import { afterEach, describe, expect, it, vi } from 'vitest';

import { apiRequest, setCSRFToken } from '@/lib/api/client';

describe('apiRequest', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    setCSRFToken('');
  });

  it('keeps the processed CSRF header when callers provide custom headers', async () => {
    setCSRFToken('csrf-token');
    const fetchMock = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        Response.json({ success: true, data: { ok: true } }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await apiRequest<{ ok: boolean }>('/dangerous-action', {
      method: 'POST',
      headers: {
        'X-Custom-Header': 'custom',
      },
    });

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit | undefined;
    expect(init).toBeTruthy();
    const headers = new Headers(init?.headers);
    expect(headers.get('X-CSRF-Token')).toBe('csrf-token');
    expect(headers.get('X-Custom-Header')).toBe('custom');
    expect(init?.credentials).toBe('include');
  });
});
