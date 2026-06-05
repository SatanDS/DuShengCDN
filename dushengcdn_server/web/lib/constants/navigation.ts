import type { NavigationItem } from '@/types/navigation';

export const dashboardNavigation: NavigationItem[] = [
  {
    href: '/',
    label: '运营总览',
    icon: 'home',
  },
  {
    href: '/proxy-route',
    label: '流量调度',
    icon: 'traffic',
    children: [
      {
        href: '/proxy-route',
        label: '站点接入',
        icon: 'proxy',
      },
      {
        href: '/origin',
        label: '源站池',
        icon: 'origin',
      },
      {
        href: '/authoritative-dns',
        label: '智能解析',
        icon: 'dns',
      },
      {
        href: '/dns-account',
        label: 'DNS 账号',
        icon: 'domain',
      },
    ],
  },
  {
    href: '/node',
    label: '边缘资源',
    icon: 'node',
    children: [
      {
        href: '/node',
        label: '边缘节点',
        icon: 'node',
      },
      {
        href: '/performance',
        label: '代理性能',
        icon: 'performance',
      },
    ],
  },
  {
    href: '/website',
    label: '证书与域名',
    icon: 'website',
    children: [
      {
        href: '/website',
        label: '域名资产',
        icon: 'website',
      },
      {
        href: '/website/certificate',
        label: 'TLS 证书',
        icon: 'certificate',
      },
    ],
  },
  {
    href: '/config-version',
    label: '交付发布',
    icon: 'release',
    children: [
      {
        href: '/config-version',
        label: '配置版本',
        icon: 'release',
      },
      {
        href: '/apply-log',
        label: '应用记录',
        icon: 'log',
      },
    ],
  },
  {
    href: '/access-log',
    label: '访问观测',
    icon: 'log',
  },
  {
    href: '/setting',
    label: '系统治理',
    icon: 'setting',
    children: [
      {
        href: '/setting',
        label: '个人设置',
        icon: 'user',
      },
      {
        href: '/setting?tab=operation',
        label: '运维参数',
        icon: 'performance',
        minRole: 100,
      },
      {
        href: '/setting?tab=license',
        label: '商业授权',
        icon: 'certificate',
        minRole: 100,
      },
      {
        href: '/setting?tab=system',
        label: '认证与安全',
        icon: 'shield',
        minRole: 100,
      },
      {
        href: '/setting?tab=database',
        label: '数据保留',
        icon: 'log',
        minRole: 100,
      },
      {
        href: '/setting?tab=other',
        label: '品牌公告',
        icon: 'setting',
        minRole: 100,
      },
    ],
  },
];
