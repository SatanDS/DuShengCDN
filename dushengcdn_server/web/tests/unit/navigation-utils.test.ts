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
    expect(getCurrentNavigationItem('/setting')?.label).toBe('个人设置');
    expect(getCurrentNavigationItem('/setting?tab=system')?.label).toBe(
      '认证与安全',
    );
    expect(getCurrentNavigationItem('/setting?tab=license')?.label).toBe(
      '商业授权',
    );
  });

  it('keeps certificate deep links attached to TLS certificates', () => {
    expect(getCurrentNavigationItem('/website/certificate')?.label).toBe(
      'TLS 证书',
    );
    expect(getCurrentNavigationItem('/certificate')?.label).toBe('TLS 证书');
    expect(getCurrentNavigationItem('/dns-account')?.label).toBe('DNS 账号');
  });

  it('filters system governance entries for non-root users', () => {
    const userNavigation = filterNavigationItemsByRole(dashboardNavigation, 0);
    const labels = flattenNavigationItems(userNavigation).map(
      (item) => item.label,
    );

    expect(labels).toContain('个人设置');
    expect(labels).not.toContain('商业授权');
    expect(labels).not.toContain('认证与安全');
  });

  it('marks only the best matching navigation branch as active', () => {
    const systemGovernance = dashboardNavigation.find(
      (item) => item.label === '系统治理',
    );
    const personalSettings = systemGovernance?.children?.find(
      (item) => item.label === '个人设置',
    );
    const securitySettings = systemGovernance?.children?.find(
      (item) => item.label === '认证与安全',
    );

    expect(systemGovernance).toBeDefined();
    expect(personalSettings).toBeDefined();
    expect(securitySettings).toBeDefined();
    if (!systemGovernance || !personalSettings || !securitySettings) {
      return;
    }

    expect(
      isNavigationItemActive('/setting?tab=system', systemGovernance),
    ).toBe(true);
    expect(
      isNavigationItemActive('/setting?tab=system', personalSettings),
    ).toBe(false);
    expect(
      isNavigationItemActive('/setting?tab=system', securitySettings),
    ).toBe(true);
  });
});
