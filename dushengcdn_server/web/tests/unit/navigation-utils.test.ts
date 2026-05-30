import { describe, expect, it } from 'vitest';

import { getCurrentNavigationItem, isPathActive } from '@/lib/utils/navigation';

describe('navigation utils', () => {
  it('marks root path as active only for home', () => {
    expect(isPathActive('/', '/')).toBe(true);
    expect(isPathActive('/node', '/')).toBe(false);
  });

  it('resolves current navigation item for nested paths', () => {
    expect(getCurrentNavigationItem('/node/abc')?.label).toBe('节点/IP池');
    expect(getCurrentNavigationItem('/website')?.label).toBe('域名资产');
    expect(getCurrentNavigationItem('/proxy-route?section=cache')?.label).toBe(
      '缓存策略',
    );
    expect(
      getCurrentNavigationItem('/proxy-route/detail?id=1&section=waf')?.label,
    ).toBe('WAF 防护');
    expect(getCurrentNavigationItem('/access-log')?.label).toBe('观测计量');
    expect(getCurrentNavigationItem('/performance')?.label).toBe(
      'OpenResty配置',
    );
    expect(getCurrentNavigationItem('/setting')?.label).toBe('设置');
  });

  it('keeps legacy deep links attached to promoted main entries', () => {
    expect(getCurrentNavigationItem('/website/certificate')?.label).toBe(
      'TLS证书',
    );
    expect(getCurrentNavigationItem('/website/dns-account')?.label).toBe(
      'DNS账号',
    );
    expect(getCurrentNavigationItem('/certificate')?.label).toBe('TLS证书');
    expect(getCurrentNavigationItem('/dns-account')?.label).toBe('DNS账号');
  });
});
