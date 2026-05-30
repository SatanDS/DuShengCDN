import type { NavigationItem } from '@/types/navigation';

export const dashboardNavigation: NavigationItem[] = [
  {
    href: '/',
    label: '总览',
    icon: 'home',
  },
  {
    href: '/node',
    label: '节点/IP池',
    icon: 'node',
  },
  {
    href: '/proxy-route',
    label: '网站配置',
    icon: 'proxy',
    children: [
      {
        href: '/proxy-route?section=dns',
        label: '自动 DNS',
        icon: 'domain',
      },
      {
        href: '/proxy-route?section=cache',
        label: '缓存策略',
        icon: 'performance',
      },
      {
        href: '/proxy-route?section=pow',
        label: 'PoW 防护',
        icon: 'setting',
      },
      {
        href: '/proxy-route?section=waf',
        label: 'WAF 防护',
        icon: 'certificate',
      },
      {
        href: '/proxy-route?section=region',
        label: '地区限制',
        icon: 'website',
      },
      {
        href: '/proxy-route?section=auth',
        label: '认证配置',
        icon: 'user',
      },
    ],
  },
  {
    href: '/website',
    label: '域名资产',
    icon: 'website',
  },
  {
    href: '/dns-account',
    label: 'DNS账号',
    icon: 'domain',
  },
  {
    href: '/certificate',
    label: 'TLS证书',
    icon: 'certificate',
  },
  {
    href: '/origin',
    label: '源站',
    icon: 'origin',
  },
  {
    href: '/config-version',
    label: '发布版本',
    icon: 'release',
  },
  {
    href: '/apply-log',
    label: '应用记录',
    icon: 'log',
  },
  {
    href: '/access-log',
    label: '观测计量',
    icon: 'log',
  },

  {
    href: '/performance',
    label: 'OpenResty配置',
    icon: 'performance',
  },
  {
    href: '/setting',
    label: '设置',
    icon: 'setting',
  },
];
