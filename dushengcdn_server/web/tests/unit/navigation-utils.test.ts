import { describe, expect, it } from 'vitest';

import { dashboardNavigation } from '@/lib/constants/navigation';
import {
  filterNavigationItemsByRole,
  flattenNavigationItems,
  getCurrentNavigationItem,
  isNavigationItemActive,
  isPathActive,
} from '@/lib/utils/navigation';

describe('navigation utils', () => {
  it('marks root path as active only for home', () => {
    expect(isPathActive('/', '/')).toBe(true);
    expect(isPathActive('/node', '/')).toBe(false);
  });

  it('resolves current navigation item for nested paths', () => {
    expect(getCurrentNavigationItem('/node/abc')?.label).toBe('边缘节点');
    expect(getCurrentNavigationItem('/website')?.label).toBe('域名资产');
    expect(getCurrentNavigationItem('/proxy-route?section=cache')?.label).toBe(
      '站点接入',
    );
    expect(
      getCurrentNavigationItem('/proxy-route/detail?id=1&section=waf')?.label,
    ).toBe('站点接入');
    expect(getCurrentNavigationItem('/access-log')?.label).toBe('访问观测');
    expect(getCurrentNavigationItem('/performance')?.label).toBe('代理性能');
    expect(getCurrentNavigationItem('/setting')?.label).toBe('系统治理');
    expect(getCurrentNavigationItem('/setting?tab=system')?.label).toBe(
      '系统治理',
    );
    expect(getCurrentNavigationItem('/setting?tab=license')?.label).toBe(
      '系统治理',
    );
  });

  it('keeps certificate deep links attached to TLS certificates', () => {
    expect(getCurrentNavigationItem('/website/certificate')?.label).toBe(
      'TLS 证书',
    );
    expect(getCurrentNavigationItem('/certificate')?.label).toBe('TLS 证书');
    expect(getCurrentNavigationItem('/dns-account')?.label).toBe('DNS 账号');
  });

  it('keeps system governance as a single sidebar entry', () => {
    const userNavigation = filterNavigationItemsByRole(dashboardNavigation, 0);
    const labels = flattenNavigationItems(userNavigation).map(
      (item) => item.label,
    );

    expect(labels).toContain('系统治理');
    expect(labels).not.toContain('个人设置');
    expect(labels).not.toContain('商业授权');
    expect(labels).not.toContain('认证与安全');
  });

  it('keeps system governance active for settings tabs', () => {
    const systemGovernance = dashboardNavigation.find(
      (item) => item.label === '系统治理',
    );

    expect(systemGovernance).toBeDefined();
    expect(systemGovernance?.children).toBeUndefined();
    if (!systemGovernance) {
      return;
    }

    expect(
      isNavigationItemActive('/setting?tab=system', systemGovernance),
    ).toBe(true);
    expect(
      isNavigationItemActive('/setting?tab=license', systemGovernance),
    ).toBe(true);
  });
});
