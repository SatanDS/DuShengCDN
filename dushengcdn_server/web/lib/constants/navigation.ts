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
  },
  {
    href: '/website',
    label: '域名资产',
    icon: 'website',
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
