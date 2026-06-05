import { describe, expect, it } from 'vitest';

import { normalizeApiBaseUrl } from '@/lib/env/public-env';

describe('normalizeApiBaseUrl', () => {
  it('keeps supported same-origin and absolute http urls', () => {
    expect(normalizeApiBaseUrl('/api/')).toBe('/api');
    expect(normalizeApiBaseUrl('/')).toBe('');
    expect(normalizeApiBaseUrl('')).toBe('/api');
    expect(normalizeApiBaseUrl('https://panel.example.com/api/')).toBe(
      'https://panel.example.com/api',
    );
    expect(normalizeApiBaseUrl('http://127.0.0.1:3000/api')).toBe(
      'http://127.0.0.1:3000/api',
    );
  });

  it('rejects unsafe or ambiguous api bases', () => {
    expect(() => normalizeApiBaseUrl('//evil.example/api')).toThrow(
      'NEXT_PUBLIC_API_BASE_URL',
    );
    expect(() => normalizeApiBaseUrl('javascript:alert(1)')).toThrow(
      'NEXT_PUBLIC_API_BASE_URL',
    );
    expect(() => normalizeApiBaseUrl('api')).toThrow(
      'NEXT_PUBLIC_API_BASE_URL',
    );
    expect(() => normalizeApiBaseUrl('https://user:pass@example.com/api')).toThrow(
      'NEXT_PUBLIC_API_BASE_URL',
    );
    expect(() => normalizeApiBaseUrl('https://example.com/api?token=1')).toThrow(
      'NEXT_PUBLIC_API_BASE_URL',
    );
  });
});
