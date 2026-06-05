import { describe, expect, it } from 'vitest';

import {
  normalizeInternalRedirect,
  normalizeTrustedExternalUrl,
} from '@/lib/utils/redirect';

describe('normalizeInternalRedirect', () => {
  it('keeps internal paths', () => {
    expect(normalizeInternalRedirect('/node/detail?id=1')).toBe(
      '/node/detail?id=1',
    );
  });

  it('falls back for external or unsafe redirects', () => {
    expect(normalizeInternalRedirect('https://example.com')).toBe('/');
    expect(normalizeInternalRedirect('//example.com')).toBe('/');
    expect(normalizeInternalRedirect('javascript:alert(1)')).toBe('/');
    expect(normalizeInternalRedirect('/\\example')).toBe('/');
    expect(normalizeInternalRedirect('settings')).toBe('/');
  });
});

describe('normalizeTrustedExternalUrl', () => {
  it('allows http and https URLs', () => {
    expect(normalizeTrustedExternalUrl('https://example.com/oauth')).toBe(
      'https://example.com/oauth',
    );
    expect(normalizeTrustedExternalUrl('http://localhost:3000/oauth')).toBe(
      'http://localhost:3000/oauth',
    );
  });

  it('rejects empty, relative and unsafe protocol URLs', () => {
    expect(() => normalizeTrustedExternalUrl('')).toThrow('跳转地址为空。');
    expect(() => normalizeTrustedExternalUrl('/oauth')).toThrow(
      '跳转地址格式无效。',
    );
    expect(() => normalizeTrustedExternalUrl('javascript:alert(1)')).toThrow(
      '跳转地址协议不受支持。',
    );
  });
});
