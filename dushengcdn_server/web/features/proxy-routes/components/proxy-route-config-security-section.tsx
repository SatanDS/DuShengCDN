'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { Minus, Plus } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import {
  buildPayloadFromRoute,
  linesFromTextarea,
  normalizeLimitRate,
  validateLimitRate,
} from '@/features/proxy-routes/helpers';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import {
  ResourceField,
  ResourceInput,
  ResourceSelect,
  ResourceTextarea,
  SecondaryButton,
  ToggleField,
} from '@/features/shared/components/resource-primitives';

import {
  ConfigSectionShell,
  type ConfigSectionPresentationProps,
  type SaveHandler,
} from './proxy-route-config-shared';

type PowListValues = {
  ips: string;
  ip_cidrs: string;
  paths: string;
  path_regexes: string;
  user_agents: string;
};

type PowListGroup = 'whitelist' | 'blacklist';
type PowListField = keyof PowListValues;

type PowRuleKey = `${PowListGroup}.${PowListField}`;

type PowRuleOption = {
  key: PowRuleKey;
  group: PowListGroup;
  field: PowListField;
  label: string;
  hint: string;
  placeholder: string;
  description: string;
};

const powRuleOptions: PowRuleOption[] = [
  {
    key: 'whitelist.ips',
    group: 'whitelist',
    field: 'ips',
    label: '白名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.ip_cidrs',
    group: 'whitelist',
    field: 'ip_cidrs',
    label: '白名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.paths',
    group: 'whitelist',
    field: 'paths',
    label: '白名单路径',
    hint: '每行一个路径通配符',
    placeholder: '/.well-known/*\n/favicon.ico',
    description: '匹配这些路径的请求会跳过计算验证。',
  },
  {
    key: 'whitelist.path_regexes',
    group: 'whitelist',
    field: 'path_regexes',
    label: '白名单路径正则',
    hint: '每行一个正则表达式',
    placeholder: '^/api/public/',
    description: '路径命中这些正则时跳过计算验证。',
  },
  {
    key: 'whitelist.user_agents',
    group: 'whitelist',
    field: 'user_agents',
    label: '白名单 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'Googlebot\nbingbot',
    description: 'User-Agent 包含这些关键字时跳过计算验证。',
  },
  {
    key: 'blacklist.ips',
    group: 'blacklist',
    field: 'ips',
    label: '黑名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.ip_cidrs',
    group: 'blacklist',
    field: 'ip_cidrs',
    label: '黑名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.paths',
    group: 'blacklist',
    field: 'paths',
    label: '黑名单路径',
    hint: '每行一个路径通配符',
    placeholder: '/admin/*',
    description: '匹配这些路径的请求必须完成计算验证。',
  },
  {
    key: 'blacklist.path_regexes',
    group: 'blacklist',
    field: 'path_regexes',
    label: '黑名单路径正则',
    hint: '每行一个正则表达式',
    placeholder: '^/private/',
    description: '路径命中这些正则时必须完成计算验证。',
  },
  {
    key: 'blacklist.user_agents',
    group: 'blacklist',
    field: 'user_agents',
    label: '黑名单 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'bot\ncrawler',
    description: 'User-Agent 包含这些关键字时必须完成计算验证。',
  },
];

type PowValues = {
  pow_enabled: boolean;
  difficulty: number;
  algorithm: 'fast' | 'slow';
  session_ttl: number;
  challenge_ttl: number;
  whitelist: PowListValues;
  blacklist: PowListValues;
};

type CCListValues = {
  ips: string;
  ip_cidrs: string;
  paths: string;
  user_agents: string;
};

type CCProtectionValues = {
  limit_conn_per_server: string;
  limit_conn_per_ip: string;
  limit_rate: string;
  cc_enabled: boolean;
  cc_mode: 'log' | 'block' | 'pow';
  cc_window_seconds: number;
  cc_max_requests: number;
  cc_path_window_seconds: number;
  cc_path_max_requests: number;
  cc_block_duration_seconds: number;
  cc_whitelist: CCListValues;
  cc_exclude: CCListValues;
  pow_enabled: boolean;
  difficulty: number;
  algorithm: 'fast' | 'slow';
  session_ttl: number;
  challenge_ttl: number;
  pow_whitelist: PowListValues;
  pow_blacklist: PowListValues;
  waf_enabled: boolean;
  waf_mode: 'log' | 'block';
  builtin_rules: WAFRuleKey[];
  waf_whitelist: {
    ips: string;
    ip_cidrs: string;
    paths: string;
  };
  waf_block_rules: {
    path_contains: string;
    path_regexes: string;
    query_contains: string;
    header_contains: string;
    user_agents: string;
  };
};

const ccListSchema = z.object({
  ips: z.string(),
  ip_cidrs: z.string(),
  paths: z.string(),
  user_agents: z.string(),
});

const ccProtectionSchema = z
  .object({
    limit_conn_per_server: z.string(),
    limit_conn_per_ip: z.string(),
    limit_rate: z.string(),
    cc_enabled: z.boolean(),
    cc_mode: z.enum(['log', 'block', 'pow']),
    cc_window_seconds: z.coerce.number().int().min(1).max(3600),
    cc_max_requests: z.coerce.number().int().min(1).max(1000000),
    cc_path_window_seconds: z.coerce.number().int().min(1).max(3600),
    cc_path_max_requests: z.coerce.number().int().min(1).max(1000000),
    cc_block_duration_seconds: z.coerce.number().int().min(1).max(86400),
    cc_whitelist: ccListSchema,
    cc_exclude: ccListSchema,
    pow_enabled: z.boolean(),
    difficulty: z.coerce.number().int().min(1).max(16),
    algorithm: z.enum(['fast', 'slow']),
    session_ttl: z.coerce.number().int().min(60),
    challenge_ttl: z.coerce.number().int().min(30),
    pow_whitelist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
      path_regexes: z.string(),
      user_agents: z.string(),
    }),
    pow_blacklist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
      path_regexes: z.string(),
      user_agents: z.string(),
    }),
    waf_enabled: z.boolean(),
    waf_mode: z.enum(['log', 'block']),
    builtin_rules: z.array(
      z.enum(['sqli', 'xss', 'path_traversal', 'sensitive_paths', 'bad_bots']),
    ),
    waf_whitelist: z.object({
      ips: z.string(),
      ip_cidrs: z.string(),
      paths: z.string(),
    }),
    waf_block_rules: z.object({
      path_contains: z.string(),
      path_regexes: z.string(),
      query_contains: z.string(),
      header_contains: z.string(),
      user_agents: z.string(),
    }),
  })
  .superRefine((value, context) => {
    for (const field of [
      'limit_conn_per_server',
      'limit_conn_per_ip',
    ] as const) {
      const rawValue = value[field].trim();
      if (!rawValue) {
        continue;
      }
      if (!/^\d+$/.test(rawValue)) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: [field],
          message: '请输入大于等于 0 的整数',
        });
      }
    }

    const limitRateError = validateLimitRate(value.limit_rate);
    if (limitRateError) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['limit_rate'],
        message: limitRateError,
      });
    }

    if (value.cc_enabled) {
      for (const [group, list] of [
        ['cc_whitelist', value.cc_whitelist],
        ['cc_exclude', value.cc_exclude],
      ] as const) {
        for (const path of linesFromTextarea(list.paths)) {
          if (!path.startsWith('/')) {
            context.addIssue({
              code: z.ZodIssueCode.custom,
              path: [group, 'paths'],
              message: `路径必须以 / 开头：${path}`,
            });
            return;
          }
        }
      }
    }

    if (value.pow_enabled || (value.cc_enabled && value.cc_mode === 'pow')) {
      const dimensions: { key: PowListField; label: string }[] = [
        { key: 'ips', label: 'IP' },
        { key: 'ip_cidrs', label: 'IP CIDR' },
        { key: 'paths', label: '路径' },
        { key: 'path_regexes', label: '路径正则' },
        { key: 'user_agents', label: 'User-Agent' },
      ];
      for (const dim of dimensions) {
        const wl = linesFromTextarea(value.pow_whitelist[dim.key] || '');
        const bl = linesFromTextarea(value.pow_blacklist[dim.key] || '');
        if (wl.length > 0 && bl.length > 0) {
          context.addIssue({
            code: z.ZodIssueCode.custom,
            message: `${dim.label} 不能同时配置计算验证白名单和黑名单`,
            path: ['pow_blacklist', dim.key],
          });
        }
      }
    }

    if (value.waf_enabled) {
      for (const path of linesFromTextarea(value.waf_whitelist.paths)) {
        if (!path.startsWith('/')) {
          context.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['waf_whitelist', 'paths'],
            message: `白名单路径必须以 / 开头：${path}`,
          });
          return;
        }
      }

      for (const pattern of linesFromTextarea(
        value.waf_block_rules.path_regexes,
      )) {
        try {
          new RegExp(pattern);
        } catch {
          context.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['waf_block_rules', 'path_regexes'],
            message: `路径正则格式不合法：${pattern}`,
          });
          return;
        }
      }
    }
  });

type WAFRuleKey =
  | 'sqli'
  | 'xss'
  | 'path_traversal'
  | 'sensitive_paths'
  | 'bad_bots';

const wafBuiltinRules: Array<{
  key: WAFRuleKey;
  label: string;
  description: string;
}> = [
  {
    key: 'sqli',
    label: 'SQL 注入',
    description: '匹配常见 SQL 注入探测片段。',
  },
  { key: 'xss', label: 'XSS', description: '匹配脚本注入和危险事件属性。' },
  {
    key: 'path_traversal',
    label: '路径穿越',
    description: '匹配 ../ 和编码后的目录穿越。',
  },
  {
    key: 'sensitive_paths',
    label: '敏感路径',
    description: '拦截 .env、.git、wp-config 等扫描。',
  },
  {
    key: 'bad_bots',
    label: '恶意工具 UA',
    description: '匹配 sqlmap、nikto、nmap 等扫描器。',
  },
];

type WAFValues = {
  waf_enabled: boolean;
  waf_mode: 'log' | 'block';
  builtin_rules: WAFRuleKey[];
  whitelist: {
    ips: string;
    ip_cidrs: string;
    paths: string;
  };
  block_rules: {
    path_contains: string;
    path_regexes: string;
    query_contains: string;
    header_contains: string;
    user_agents: string;
  };
};

type WAFWhitelistField = keyof WAFValues['whitelist'];
type WAFBlockRuleField = keyof WAFValues['block_rules'];
type WAFCustomRuleKey =
  | `whitelist.${WAFWhitelistField}`
  | `block_rules.${WAFBlockRuleField}`;

type WAFCustomRuleOption =
  | {
      key: `whitelist.${WAFWhitelistField}`;
      group: 'whitelist';
      field: WAFWhitelistField;
      label: string;
      hint: string;
      placeholder: string;
      description: string;
    }
  | {
      key: `block_rules.${WAFBlockRuleField}`;
      group: 'block_rules';
      field: WAFBlockRuleField;
      label: string;
      hint: string;
      placeholder: string;
      description: string;
    };

const wafCustomRuleOptions: WAFCustomRuleOption[] = [
  {
    key: 'whitelist.ips',
    group: 'whitelist',
    field: 'ips',
    label: '白名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求会跳过恶意请求防护。',
  },
  {
    key: 'whitelist.ip_cidrs',
    group: 'whitelist',
    field: 'ip_cidrs',
    label: '白名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求会跳过恶意请求防护。',
  },
  {
    key: 'whitelist.paths',
    group: 'whitelist',
    field: 'paths',
    label: '白名单路径',
    hint: '每行一个路径，支持以 * 结尾的前缀匹配。',
    placeholder: '/api/public/*',
    description: '匹配这些路径的请求会跳过恶意请求防护。',
  },
  {
    key: 'block_rules.path_contains',
    group: 'block_rules',
    field: 'path_contains',
    label: '拦截路径包含',
    hint: '每行一个关键字',
    placeholder: '/debug',
    description: '请求路径包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.path_regexes',
    group: 'block_rules',
    field: 'path_regexes',
    label: '拦截路径正则',
    hint: '每行一个正则表达式。',
    placeholder: '^/private/',
    description: '请求路径命中这些正则时命中恶意请求防护。',
  },
  {
    key: 'block_rules.query_contains',
    group: 'block_rules',
    field: 'query_contains',
    label: '拦截查询参数包含',
    hint: '每行一个关键字',
    placeholder: 'debug=true',
    description: '查询参数包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.header_contains',
    group: 'block_rules',
    field: 'header_contains',
    label: '拦截请求头包含',
    hint: '每行一个关键字',
    placeholder: 'X-Scanner',
    description: '请求头包含这些关键字时命中恶意请求防护。',
  },
  {
    key: 'block_rules.user_agents',
    group: 'block_rules',
    field: 'user_agents',
    label: '拦截 User-Agent 包含',
    hint: '每行一个关键字',
    placeholder: 'sqlmap',
    description: 'User-Agent 包含这些关键字时命中恶意请求防护。',
  },
];

function buildWAFValuesFromRoute(route: ProxyRouteItem): WAFValues {
  const config = route.waf_config;
  return {
    waf_enabled: route.waf_enabled,
    waf_mode: route.waf_mode || 'block',
    builtin_rules: config?.builtin_rules ?? [
      'sqli',
      'xss',
      'path_traversal',
      'sensitive_paths',
      'bad_bots',
    ],
    whitelist: {
      ips: (config?.whitelist?.ips ?? []).join('\n'),
      ip_cidrs: (config?.whitelist?.ip_cidrs ?? []).join('\n'),
      paths: (config?.whitelist?.paths ?? []).join('\n'),
    },
    block_rules: {
      path_contains: (config?.block_rules?.path_contains ?? []).join('\n'),
      path_regexes: (config?.block_rules?.path_regexes ?? []).join('\n'),
      query_contains: (config?.block_rules?.query_contains ?? []).join('\n'),
      header_contains: (config?.block_rules?.header_contains ?? []).join('\n'),
      user_agents: (config?.block_rules?.user_agents ?? []).join('\n'),
    },
  };
}

function buildPowListFromConfig(
  list:
    | {
        ips?: string[];
        ip_cidrs?: string[];
        paths?: string[];
        path_regexes?: string[];
        user_agents?: string[];
      }
    | undefined,
): PowListValues {
  return {
    ips: (list?.ips ?? []).join('\n'),
    ip_cidrs: (list?.ip_cidrs ?? []).join('\n'),
    paths: (list?.paths ?? []).join('\n'),
    path_regexes: (list?.path_regexes ?? []).join('\n'),
    user_agents: (list?.user_agents ?? []).join('\n'),
  };
}

function buildPowValuesFromRoute(route: ProxyRouteItem): PowValues {
  const config = route.pow_config;
  return {
    pow_enabled: route.pow_enabled,
    difficulty: config?.difficulty ?? 4,
    algorithm: config?.algorithm ?? 'fast',
    session_ttl: config?.session_ttl ?? 600,
    challenge_ttl: config?.challenge_ttl ?? 300,
    whitelist: buildPowListFromConfig(config?.whitelist),
    blacklist: buildPowListFromConfig(config?.blacklist),
  };
}

type CCRuleGroup = 'cc_whitelist' | 'cc_exclude';
type CCRuleField = keyof CCListValues;
type CCRuleKey = `${CCRuleGroup}.${CCRuleField}`;

type CCRuleOption = {
  key: CCRuleKey;
  group: CCRuleGroup;
  field: CCRuleField;
  label: string;
  hint: string;
  placeholder: string;
  description: string;
};

const ccRuleOptions: CCRuleOption[] = [
  {
    key: 'cc_whitelist.ips',
    group: 'cc_whitelist',
    field: 'ips',
    label: '白名单 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求会跳过 CC 频率防护。',
  },
  {
    key: 'cc_whitelist.ip_cidrs',
    group: 'cc_whitelist',
    field: 'ip_cidrs',
    label: '白名单 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求会跳过 CC 频率防护。',
  },
  {
    key: 'cc_whitelist.paths',
    group: 'cc_whitelist',
    field: 'paths',
    label: '白名单路径',
    hint: '每行一个路径，支持以 * 结尾的前缀匹配。',
    placeholder: '/api/health\n/static/*',
    description: '匹配这些路径的请求会跳过 CC 频率防护。',
  },
  {
    key: 'cc_whitelist.user_agents',
    group: 'cc_whitelist',
    field: 'user_agents',
    label: '白名单 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'Googlebot\nbingbot',
    description: 'User-Agent 包含这些关键字时跳过 CC 频率防护。',
  },
  {
    key: 'cc_exclude.ips',
    group: 'cc_exclude',
    field: 'ips',
    label: '排除统计 IP',
    hint: '每行一个 IP 地址',
    placeholder: '1.2.3.4',
    description: '匹配这些 IP 的请求不计入 CC 频率计数。',
  },
  {
    key: 'cc_exclude.ip_cidrs',
    group: 'cc_exclude',
    field: 'ip_cidrs',
    label: '排除统计 IP CIDR',
    hint: '每行一个 CIDR 范围',
    placeholder: '10.0.0.0/8',
    description: '匹配这些网段的请求不计入 CC 频率计数。',
  },
  {
    key: 'cc_exclude.paths',
    group: 'cc_exclude',
    field: 'paths',
    label: '排除统计路径',
    hint: '每行一个路径，支持以 * 结尾的前缀匹配。',
    placeholder: '/assets/*\n/favicon.ico',
    description: '匹配这些路径的请求不计入 CC 频率计数。',
  },
  {
    key: 'cc_exclude.user_agents',
    group: 'cc_exclude',
    field: 'user_agents',
    label: '排除统计 User-Agent',
    hint: '每行一个关键字',
    placeholder: 'monitor\nhealthcheck',
    description: 'User-Agent 包含这些关键字时不计入 CC 频率计数。',
  },
];

const ccRuleOptionMap = new Map(
  ccRuleOptions.map((option) => [option.key, option]),
);

type CCPowRuleGroup = 'pow_whitelist' | 'pow_blacklist';
type CCPowRuleKey = `${CCPowRuleGroup}.${PowListField}`;
type CCPowRuleOption = Omit<PowRuleOption, 'key' | 'group'> & {
  key: CCPowRuleKey;
  group: CCPowRuleGroup;
};

const ccPowRuleOptions: CCPowRuleOption[] = powRuleOptions.map((option) => ({
  ...option,
  key: `pow_${option.group}.${option.field}` as CCPowRuleKey,
  group: `pow_${option.group}` as CCPowRuleGroup,
}));

const ccPowRuleOptionMap = new Map(
  ccPowRuleOptions.map((option) => [option.key, option]),
);

type CCWAFCustomRuleKey = `waf_${WAFCustomRuleKey}`;
type CCWAFCustomRuleOption =
  | (Omit<
      Extract<WAFCustomRuleOption, { group: 'whitelist' }>,
      'key' | 'group'
    > & {
      key: `waf_whitelist.${WAFWhitelistField}`;
      group: 'waf_whitelist';
    })
  | (Omit<
      Extract<WAFCustomRuleOption, { group: 'block_rules' }>,
      'key' | 'group'
    > & {
      key: `waf_block_rules.${WAFBlockRuleField}`;
      group: 'waf_block_rules';
    });

const ccWAFCustomRuleOptions: CCWAFCustomRuleOption[] =
  wafCustomRuleOptions.map((option) =>
    option.group === 'whitelist'
      ? {
          ...option,
          key: `waf_whitelist.${option.field}` as `waf_whitelist.${WAFWhitelistField}`,
          group: 'waf_whitelist' as const,
        }
      : {
          ...option,
          key: `waf_block_rules.${option.field}` as `waf_block_rules.${WAFBlockRuleField}`,
          group: 'waf_block_rules' as const,
        },
  );

const ccWAFCustomRuleOptionMap = new Map(
  ccWAFCustomRuleOptions.map((option) => [option.key, option]),
);

function buildCCListValues(list?: {
  ips?: string[];
  ip_cidrs?: string[];
  paths?: string[];
  user_agents?: string[];
}): CCListValues {
  return {
    ips: (list?.ips ?? []).join('\n'),
    ip_cidrs: (list?.ip_cidrs ?? []).join('\n'),
    paths: (list?.paths ?? []).join('\n'),
    user_agents: (list?.user_agents ?? []).join('\n'),
  };
}

function buildCCProtectionValuesFromRoute(
  route: ProxyRouteItem,
): CCProtectionValues {
  const powValues = buildPowValuesFromRoute(route);
  const wafValues = buildWAFValuesFromRoute(route);
  const ccConfig = route.cc_config;
  return {
    limit_conn_per_server: route.limit_conn_per_server
      ? String(route.limit_conn_per_server)
      : '',
    limit_conn_per_ip: route.limit_conn_per_ip
      ? String(route.limit_conn_per_ip)
      : '',
    limit_rate: route.limit_rate || '',
    cc_enabled: route.cc_enabled,
    cc_mode: route.cc_mode || 'block',
    cc_window_seconds: ccConfig?.window_seconds ?? 10,
    cc_max_requests: ccConfig?.max_requests ?? 120,
    cc_path_window_seconds: ccConfig?.path_window_seconds ?? 10,
    cc_path_max_requests: ccConfig?.path_max_requests ?? 60,
    cc_block_duration_seconds: ccConfig?.block_duration_seconds ?? 300,
    cc_whitelist: buildCCListValues(ccConfig?.whitelist),
    cc_exclude: buildCCListValues(ccConfig?.exclude),
    pow_enabled: powValues.pow_enabled,
    difficulty: powValues.difficulty,
    algorithm: powValues.algorithm,
    session_ttl: powValues.session_ttl,
    challenge_ttl: powValues.challenge_ttl,
    pow_whitelist: powValues.whitelist,
    pow_blacklist: powValues.blacklist,
    waf_enabled: wafValues.waf_enabled,
    waf_mode: wafValues.waf_mode,
    builtin_rules: wafValues.builtin_rules,
    waf_whitelist: wafValues.whitelist,
    waf_block_rules: wafValues.block_rules,
  };
}

function buildInitialCCRuleKeys(
  values: Pick<CCProtectionValues, 'cc_whitelist' | 'cc_exclude'>,
) {
  return ccRuleOptions
    .filter((option) => values[option.group][option.field].trim() !== '')
    .map((option) => option.key);
}

function buildInitialCCPowRuleKeys(
  values: Pick<CCProtectionValues, 'pow_whitelist' | 'pow_blacklist'>,
) {
  return ccPowRuleOptions
    .filter((option) => values[option.group][option.field].trim() !== '')
    .map((option) => option.key);
}

function buildInitialCCWAFCustomRuleKeys(
  values: Pick<CCProtectionValues, 'waf_whitelist' | 'waf_block_rules'>,
) {
  return ccWAFCustomRuleOptions
    .filter((option) =>
      option.group === 'waf_whitelist'
        ? values.waf_whitelist[option.field].trim() !== ''
        : values.waf_block_rules[option.field].trim() !== '',
    )
    .map((option) => option.key);
}

function ProtectionSubsection({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-4 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-elevated)] px-4 py-4">
      <div>
        <h4 className="text-sm font-semibold text-[var(--foreground-primary)]">
          {title}
        </h4>
        <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
          {description}
        </p>
      </div>
      {children}
    </section>
  );
}

export function CCProtectionSection({
  route,
  saving,
  onSave,
  formId = 'proxy-route-cc-form',
  embedded = false,
}: {
  route: ProxyRouteItem;
  saving: boolean;
  onSave: SaveHandler;
} & ConfigSectionPresentationProps) {
  const form = useForm<CCProtectionValues>({
    resolver: zodResolver(ccProtectionSchema),
    defaultValues: buildCCProtectionValuesFromRoute(route),
  });
  const [selectedCCRuleKey, setSelectedCCRuleKey] = useState<CCRuleKey>(
    ccRuleOptions[0].key,
  );
  const [activeCCRuleKeys, setActiveCCRuleKeys] = useState<CCRuleKey[]>(() =>
    buildInitialCCRuleKeys(buildCCProtectionValuesFromRoute(route)),
  );
  const [selectedPowRuleKey, setSelectedPowRuleKey] = useState<CCPowRuleKey>(
    ccPowRuleOptions[0].key,
  );
  const [activePowRuleKeys, setActivePowRuleKeys] = useState<CCPowRuleKey[]>(
    () => buildInitialCCPowRuleKeys(buildCCProtectionValuesFromRoute(route)),
  );
  const [selectedWAFCustomRuleKey, setSelectedWAFCustomRuleKey] =
    useState<CCWAFCustomRuleKey>(ccWAFCustomRuleOptions[0].key);
  const [activeWAFCustomRuleKeys, setActiveWAFCustomRuleKeys] = useState<
    CCWAFCustomRuleKey[]
  >(() =>
    buildInitialCCWAFCustomRuleKeys(buildCCProtectionValuesFromRoute(route)),
  );

  useEffect(() => {
    const nextValues = buildCCProtectionValuesFromRoute(route);
    form.reset(nextValues);
    setActiveCCRuleKeys(buildInitialCCRuleKeys(nextValues));
    setActivePowRuleKeys(buildInitialCCPowRuleKeys(nextValues));
    setActiveWAFCustomRuleKeys(buildInitialCCWAFCustomRuleKeys(nextValues));
  }, [form, route]);

  const ccEnabled = form.watch('cc_enabled');
  const ccMode = form.watch('cc_mode');
  const powEnabled = form.watch('pow_enabled');
  const wafEnabled = form.watch('waf_enabled');
  const wafMode = form.watch('waf_mode');
  const watchedBuiltinRules = form.watch('builtin_rules');
  const powControlsEnabled = powEnabled || (ccEnabled && ccMode === 'pow');
  const parseList = (text: string): string[] =>
    linesFromTextarea(text).filter(Boolean);

  const visibleCCRuleOptions = activeCCRuleKeys
    .map((key) => ccRuleOptionMap.get(key))
    .filter((option): option is CCRuleOption => Boolean(option));
  const visiblePowRuleOptions = activePowRuleKeys
    .map((key) => ccPowRuleOptionMap.get(key))
    .filter((option): option is CCPowRuleOption => Boolean(option));
  const visibleWAFCustomRuleOptions = activeWAFCustomRuleKeys
    .map((key) => ccWAFCustomRuleOptionMap.get(key))
    .filter((option): option is CCWAFCustomRuleOption => Boolean(option));

  const addCCRule = () => {
    setActiveCCRuleKeys((current) =>
      current.includes(selectedCCRuleKey)
        ? current
        : [...current, selectedCCRuleKey],
    );
  };
  const removeCCRule = (option: CCRuleOption) => {
    form.setValue(option.key, '', { shouldDirty: true, shouldValidate: true });
    setActiveCCRuleKeys((current) =>
      current.filter((key) => key !== option.key),
    );
  };
  const addPowRule = () => {
    setActivePowRuleKeys((current) =>
      current.includes(selectedPowRuleKey)
        ? current
        : [...current, selectedPowRuleKey],
    );
  };
  const removePowRule = (option: CCPowRuleOption) => {
    form.setValue(option.key, '', { shouldDirty: true, shouldValidate: true });
    setActivePowRuleKeys((current) =>
      current.filter((key) => key !== option.key),
    );
  };
  const addWAFCustomRule = () => {
    setActiveWAFCustomRuleKeys((current) =>
      current.includes(selectedWAFCustomRuleKey)
        ? current
        : [...current, selectedWAFCustomRuleKey],
    );
  };
  const removeWAFCustomRule = (option: CCWAFCustomRuleOption) => {
    form.setValue(option.key, '', { shouldDirty: true, shouldValidate: true });
    setActiveWAFCustomRuleKeys((current) =>
      current.filter((key) => key !== option.key),
    );
  };
  const toggleBuiltinRule = (rule: WAFRuleKey, checked: boolean) => {
    const current = new Set(form.getValues('builtin_rules'));
    if (checked) {
      current.add(rule);
    } else {
      current.delete(rule);
    }
    form.setValue('builtin_rules', Array.from(current) as WAFRuleKey[], {
      shouldDirty: true,
    });
  };

  const getCCRuleError = (option: CCRuleOption) =>
    option.field === 'paths'
      ? form.formState.errors[option.group]?.paths?.message
      : undefined;
  const getPowRuleError = (option: CCPowRuleOption) =>
    option.group === 'pow_blacklist'
      ? form.formState.errors.pow_blacklist?.[option.field]?.message
      : undefined;
  const getWAFCustomRuleError = (option: CCWAFCustomRuleOption) => {
    if (option.group === 'waf_whitelist' && option.field === 'paths') {
      return form.formState.errors.waf_whitelist?.paths?.message;
    }
    if (option.group === 'waf_block_rules' && option.field === 'path_regexes') {
      return form.formState.errors.waf_block_rules?.path_regexes?.message;
    }
    return undefined;
  };

  return (
    <ConfigSectionShell
      title="CC 防护"
      description="统一管理访问频率、连接限流、计算验证和恶意请求规则。"
      formId={formId}
      saving={saving}
      embedded={embedded}
    >
      <form
        id={formId}
        className="space-y-5"
        onSubmit={form.handleSubmit((values) => {
          const ccConfigPayload = JSON.stringify({
            window_seconds: values.cc_window_seconds,
            max_requests: values.cc_max_requests,
            path_window_seconds: values.cc_path_window_seconds,
            path_max_requests: values.cc_path_max_requests,
            block_duration_seconds: values.cc_block_duration_seconds,
            whitelist: {
              ips: parseList(values.cc_whitelist.ips),
              ip_cidrs: parseList(values.cc_whitelist.ip_cidrs),
              paths: parseList(values.cc_whitelist.paths),
              user_agents: parseList(values.cc_whitelist.user_agents),
            },
            exclude: {
              ips: parseList(values.cc_exclude.ips),
              ip_cidrs: parseList(values.cc_exclude.ip_cidrs),
              paths: parseList(values.cc_exclude.paths),
              user_agents: parseList(values.cc_exclude.user_agents),
            },
          });
          const powConfigPayload = JSON.stringify({
            difficulty: values.difficulty,
            algorithm: values.algorithm,
            session_ttl: values.session_ttl,
            challenge_ttl: values.challenge_ttl,
            whitelist: {
              ips: parseList(values.pow_whitelist.ips),
              ip_cidrs: parseList(values.pow_whitelist.ip_cidrs),
              paths: parseList(values.pow_whitelist.paths),
              path_regexes: parseList(values.pow_whitelist.path_regexes),
              user_agents: parseList(values.pow_whitelist.user_agents),
            },
            blacklist: {
              ips: parseList(values.pow_blacklist.ips),
              ip_cidrs: parseList(values.pow_blacklist.ip_cidrs),
              paths: parseList(values.pow_blacklist.paths),
              path_regexes: parseList(values.pow_blacklist.path_regexes),
              user_agents: parseList(values.pow_blacklist.user_agents),
            },
          });
          const wafConfigPayload = JSON.stringify({
            builtin_rules: values.builtin_rules,
            whitelist: {
              ips: parseList(values.waf_whitelist.ips),
              ip_cidrs: parseList(values.waf_whitelist.ip_cidrs),
              paths: parseList(values.waf_whitelist.paths),
            },
            block_rules: {
              path_contains: parseList(values.waf_block_rules.path_contains),
              path_regexes: parseList(values.waf_block_rules.path_regexes),
              query_contains: parseList(values.waf_block_rules.query_contains),
              header_contains: parseList(
                values.waf_block_rules.header_contains,
              ),
              user_agents: parseList(values.waf_block_rules.user_agents),
            },
          });

          onSave(
            buildPayloadFromRoute(route, {
              limit_conn_per_server: Number(
                values.limit_conn_per_server.trim() || '0',
              ),
              limit_conn_per_ip: Number(values.limit_conn_per_ip.trim() || '0'),
              limit_rate: normalizeLimitRate(values.limit_rate),
              cc_enabled: values.cc_enabled,
              cc_mode: values.cc_mode,
              cc_config: ccConfigPayload,
              pow_enabled: values.pow_enabled,
              pow_config: powConfigPayload,
              waf_enabled: values.waf_enabled,
              waf_mode: values.waf_mode,
              waf_config: wafConfigPayload,
            }),
            { message: 'CC 防护设置已保存。' },
          );
        })}
      >
        <ProtectionSubsection
          title="访问频率防护"
          description="按真实客户端 IP 统计请求频率，超过阈值后可记录、拦截或转入计算验证。"
        >
          <ToggleField
            label="启用 CC 频率防护"
            description="开启后，节点会按下方阈值识别短时间高频访问。"
            tooltip="这里的 CC 防护主要处理单个客户端短时间大量请求；与连接数限制、计算验证和恶意请求规则一起组成站点级防护。"
            checked={ccEnabled}
            onChange={(checked) =>
              form.setValue('cc_enabled', checked, { shouldDirty: true })
            }
          />

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <ResourceField
              label="触发动作"
              hint={
                ccMode === 'pow'
                  ? '超过阈值后让浏览器完成计算验证，通过后继续访问。'
                  : ccMode === 'log'
                    ? '只记录命中原因并继续放行，适合先观察阈值。'
                    : '超过阈值后直接返回 429。'
              }
            >
              <ResourceSelect
                aria-label="CC 触发动作"
                disabled={!ccEnabled}
                {...form.register('cc_mode')}
              >
                <option value="block">拦截模式</option>
                <option value="pow">转入计算验证</option>
                <option value="log">观察模式</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="全站统计窗口（秒）"
              hint="在这个时间窗口内统计同一 IP 对本站的请求数。"
              error={form.formState.errors.cc_window_seconds?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                max={3600}
                disabled={!ccEnabled}
                {...form.register('cc_window_seconds')}
              />
            </ResourceField>

            <ResourceField
              label="全站请求上限"
              hint="同一 IP 在全站窗口内允许的最大请求数。"
              error={form.formState.errors.cc_max_requests?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                disabled={!ccEnabled}
                {...form.register('cc_max_requests')}
              />
            </ResourceField>

            <ResourceField
              label="单路径统计窗口（秒）"
              hint="用于识别同一路径被短时间反复刷新的请求。"
              error={form.formState.errors.cc_path_window_seconds?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                max={3600}
                disabled={!ccEnabled}
                {...form.register('cc_path_window_seconds')}
              />
            </ResourceField>

            <ResourceField
              label="单路径请求上限"
              hint="同一 IP 对同一路径在窗口内允许的最大请求数。"
              error={form.formState.errors.cc_path_max_requests?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                disabled={!ccEnabled}
                {...form.register('cc_path_max_requests')}
              />
            </ResourceField>

            <ResourceField
              label="触发后封禁时间（秒）"
              hint="拦截模式下命中后保持拦截的时间。"
              error={form.formState.errors.cc_block_duration_seconds?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                max={86400}
                disabled={!ccEnabled}
                {...form.register('cc_block_duration_seconds')}
              />
            </ResourceField>
          </div>

          <ResourceField
            label="按需添加 CC 白名单或排除规则"
            hint="选择一种规则类型，点击加号后再填写。未添加的规则不会生效。"
            container="div"
          >
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <ResourceSelect
                aria-label="按需添加 CC 规则"
                disabled={!ccEnabled}
                value={selectedCCRuleKey}
                onChange={(event) =>
                  setSelectedCCRuleKey(event.target.value as CCRuleKey)
                }
              >
                {ccRuleOptions.map((option) => (
                  <option key={option.key} value={option.key}>
                    {option.label}
                  </option>
                ))}
              </ResourceSelect>
              <SecondaryButton
                type="button"
                aria-label="添加 CC 防护规则"
                title="添加规则"
                disabled={
                  !ccEnabled || activeCCRuleKeys.includes(selectedCCRuleKey)
                }
                onClick={addCCRule}
                className="h-11 w-11 shrink-0 rounded-lg p-0"
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </SecondaryButton>
            </div>
          </ResourceField>

          {visibleCCRuleOptions.length === 0 ? (
            <p className="rounded-lg border border-dashed border-[var(--border-default)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
              暂未添加 CC 白名单或排除规则。所有正常请求都会参与频率统计。
            </p>
          ) : (
            <div className="space-y-3">
              {visibleCCRuleOptions.map((option) => (
                <div
                  key={option.key}
                  className="rounded-lg border border-[var(--border-default)] bg-[var(--surface-muted)] px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium text-[var(--foreground-primary)]">
                        {option.label}
                      </p>
                      <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                        {option.description}
                      </p>
                    </div>
                    <SecondaryButton
                      type="button"
                      aria-label={`移除${option.label}`}
                      title={`移除${option.label}`}
                      disabled={!ccEnabled}
                      onClick={() => removeCCRule(option)}
                      className="h-9 w-9 shrink-0 rounded-lg p-0"
                    >
                      <Minus className="h-4 w-4" aria-hidden="true" />
                    </SecondaryButton>
                  </div>
                  <ResourceField
                    label="匹配内容"
                    hint={option.hint}
                    error={getCCRuleError(option)}
                    className="mt-3"
                  >
                    <ResourceTextarea
                      className="min-h-20"
                      disabled={!ccEnabled}
                      placeholder={option.placeholder}
                      {...form.register(option.key)}
                    />
                  </ResourceField>
                </div>
              ))}
            </div>
          )}
        </ProtectionSubsection>

        <ProtectionSubsection
          title="连接与限速"
          description="限制并发连接和单请求带宽，适合给源站兜底减压。"
        >
          <div className="grid gap-4 md:grid-cols-3">
            <ResourceField
              label="并发限制"
              hint="限制当前站点最大并发连接数，空值或 0 表示关闭。"
              error={form.formState.errors.limit_conn_per_server?.message}
            >
              <ResourceInput
                placeholder="120"
                {...form.register('limit_conn_per_server')}
              />
            </ResourceField>

            <ResourceField
              label="单 IP 并发限制"
              hint="限制单个 IP 的最大并发连接数，空值或 0 表示关闭。"
              error={form.formState.errors.limit_conn_per_ip?.message}
            >
              <ResourceInput
                placeholder="12"
                {...form.register('limit_conn_per_ip')}
              />
            </ResourceField>

            <ResourceField
              label="单请求限速"
              hint="例如 512k 或 1m；空值或 0 表示关闭。"
              error={form.formState.errors.limit_rate?.message}
            >
              <ResourceInput
                placeholder="512k/1m"
                {...form.register('limit_rate')}
              />
            </ResourceField>
          </div>
        </ProtectionSubsection>

        <ProtectionSubsection
          title="计算验证"
          description="让浏览器完成轻量计算后继续访问，可单独开启，也可作为 CC 频率命中后的动作。"
        >
          <ToggleField
            label="启用计算验证"
            description="开启后，匹配规则的请求需要通过浏览器计算验证。"
            tooltip="这类能力也常叫 PoW 或 Proof-of-Work。正常浏览器通常能自动完成，自动化请求会被消耗更多成本。"
            checked={powEnabled}
            onChange={(checked) =>
              form.setValue('pow_enabled', checked, { shouldDirty: true })
            }
          />

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <ResourceField label="验证算法">
              <ResourceSelect
                aria-label="验证算法"
                disabled={!powControlsEnabled}
                {...form.register('algorithm')}
              >
                <option value="fast">Fast（WebCrypto SHA-256）</option>
                <option value="slow">Slow（兼容模式）</option>
              </ResourceSelect>
            </ResourceField>

            <ResourceField
              label="难度"
              hint="数值越高验证越慢，1-16。推荐 3-5。"
              error={form.formState.errors.difficulty?.message}
            >
              <ResourceInput
                type="number"
                min={1}
                max={16}
                disabled={!powControlsEnabled}
                {...form.register('difficulty')}
              />
            </ResourceField>

            <ResourceField
              label="会话空闲有效期（秒）"
              hint="通过验证后，若在此时间内没有新请求，Cookie 会失效。"
              error={form.formState.errors.session_ttl?.message}
            >
              <ResourceInput
                type="number"
                min={60}
                disabled={!powControlsEnabled}
                {...form.register('session_ttl')}
              />
            </ResourceField>

            <ResourceField
              label="挑战有效期（秒）"
              hint="挑战令牌的有效期。"
              error={form.formState.errors.challenge_ttl?.message}
            >
              <ResourceInput
                type="number"
                min={30}
                disabled={!powControlsEnabled}
                {...form.register('challenge_ttl')}
              />
            </ResourceField>
          </div>

          <ResourceField
            label="按需添加计算验证规则"
            hint="选择一种规则类型，点击加号后再填写。未添加的规则不会生效。"
            container="div"
          >
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <ResourceSelect
                aria-label="按需添加计算验证规则"
                disabled={!powControlsEnabled}
                value={selectedPowRuleKey}
                onChange={(event) =>
                  setSelectedPowRuleKey(event.target.value as CCPowRuleKey)
                }
              >
                {ccPowRuleOptions.map((option) => (
                  <option key={option.key} value={option.key}>
                    {option.label}
                  </option>
                ))}
              </ResourceSelect>
              <SecondaryButton
                type="button"
                aria-label="添加计算验证规则"
                title="添加规则"
                disabled={
                  !powControlsEnabled ||
                  activePowRuleKeys.includes(selectedPowRuleKey)
                }
                onClick={addPowRule}
                className="h-11 w-11 shrink-0 rounded-lg p-0"
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </SecondaryButton>
            </div>
          </ResourceField>

          {visiblePowRuleOptions.length === 0 ? (
            <p className="rounded-lg border border-dashed border-[var(--border-default)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
              暂未添加额外计算验证规则。默认按上方开关处理请求。
            </p>
          ) : (
            <div className="space-y-3">
              {visiblePowRuleOptions.map((option) => (
                <div
                  key={option.key}
                  className="rounded-lg border border-[var(--border-default)] bg-[var(--surface-muted)] px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium text-[var(--foreground-primary)]">
                        {option.label}
                      </p>
                      <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                        {option.description}
                      </p>
                    </div>
                    <SecondaryButton
                      type="button"
                      aria-label={`移除${option.label}`}
                      title={`移除${option.label}`}
                      disabled={!powControlsEnabled}
                      onClick={() => removePowRule(option)}
                      className="h-9 w-9 shrink-0 rounded-lg p-0"
                    >
                      <Minus className="h-4 w-4" aria-hidden="true" />
                    </SecondaryButton>
                  </div>
                  <ResourceField
                    label="匹配内容"
                    hint={option.hint}
                    error={getPowRuleError(option)}
                    className="mt-3"
                  >
                    <ResourceTextarea
                      className="min-h-20"
                      disabled={!powControlsEnabled}
                      placeholder={option.placeholder}
                      {...form.register(option.key)}
                    />
                  </ResourceField>
                </div>
              ))}
            </div>
          )}
        </ProtectionSubsection>

        <ProtectionSubsection
          title="恶意请求规则"
          description="节点本地轻量规则，用来识别 SQL 注入、路径扫描、恶意工具 UA 等请求。"
        >
          <ToggleField
            label="启用恶意请求规则"
            description="开启并发布配置后，节点会在本地检查常见攻击和自定义规则。"
            tooltip="这类能力也常叫 WAF。这里是节点本地的轻量规则，不依赖第三方防护服务。"
            checked={wafEnabled}
            onChange={(checked) =>
              form.setValue('waf_enabled', checked, { shouldDirty: true })
            }
          />

          <ResourceField
            label="运行模式"
            hint={
              wafMode === 'log'
                ? '只记录命中规则并继续放行，适合先观察误杀。'
                : '命中规则后直接返回 403。'
            }
          >
            <ResourceSelect
              aria-label="恶意请求规则运行模式"
              disabled={!wafEnabled}
              {...form.register('waf_mode')}
            >
              <option value="block">拦截模式</option>
              <option value="log">观察模式</option>
            </ResourceSelect>
          </ResourceField>

          <ResourceField
            label="内置规则"
            hint="建议先使用观察模式确认误杀情况，再切换为拦截模式。"
            container="div"
          >
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
              {wafBuiltinRules.map((rule) => (
                <label
                  key={rule.key}
                  className="flex min-h-20 items-start gap-3 rounded-2xl border border-[var(--border-default)] bg-[var(--surface-muted)] px-4 py-3"
                >
                  <input
                    type="checkbox"
                    disabled={!wafEnabled}
                    checked={watchedBuiltinRules.includes(rule.key)}
                    onChange={(event) =>
                      toggleBuiltinRule(rule.key, event.target.checked)
                    }
                    className="mt-1 h-4 w-4 rounded border-[var(--border-default)] accent-[var(--brand-primary)]"
                  />
                  <span>
                    <span className="block text-sm font-medium text-[var(--foreground-primary)]">
                      {rule.label}
                    </span>
                    <span className="mt-1 block text-xs leading-5 text-[var(--foreground-secondary)]">
                      {rule.description}
                    </span>
                  </span>
                </label>
              ))}
            </div>
          </ResourceField>

          <ResourceField
            label="按需添加白名单或拦截规则"
            hint="选择一种规则类型，点击加号后再填写。未添加的规则不会生效。"
            container="div"
          >
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <ResourceSelect
                aria-label="按需添加白名单或拦截规则"
                disabled={!wafEnabled}
                value={selectedWAFCustomRuleKey}
                onChange={(event) =>
                  setSelectedWAFCustomRuleKey(
                    event.target.value as CCWAFCustomRuleKey,
                  )
                }
              >
                {ccWAFCustomRuleOptions.map((option) => (
                  <option key={option.key} value={option.key}>
                    {option.label}
                  </option>
                ))}
              </ResourceSelect>
              <SecondaryButton
                type="button"
                aria-label="添加恶意请求规则"
                title="添加规则"
                disabled={
                  !wafEnabled ||
                  activeWAFCustomRuleKeys.includes(selectedWAFCustomRuleKey)
                }
                onClick={addWAFCustomRule}
                className="h-11 w-11 shrink-0 rounded-lg p-0"
              >
                <Plus className="h-4 w-4" aria-hidden="true" />
              </SecondaryButton>
            </div>
          </ResourceField>

          {visibleWAFCustomRuleOptions.length === 0 ? (
            <p className="rounded-lg border border-dashed border-[var(--border-default)] px-4 py-3 text-sm text-[var(--foreground-secondary)]">
              暂未添加自定义规则。当前仅使用上方内置规则。
            </p>
          ) : (
            <div className="space-y-3">
              {visibleWAFCustomRuleOptions.map((option) => (
                <div
                  key={option.key}
                  className="rounded-lg border border-[var(--border-default)] bg-[var(--surface-muted)] px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium text-[var(--foreground-primary)]">
                        {option.label}
                      </p>
                      <p className="mt-1 text-xs leading-5 text-[var(--foreground-secondary)]">
                        {option.description}
                      </p>
                    </div>
                    <SecondaryButton
                      type="button"
                      aria-label={`移除${option.label}`}
                      title={`移除${option.label}`}
                      disabled={!wafEnabled}
                      onClick={() => removeWAFCustomRule(option)}
                      className="h-9 w-9 shrink-0 rounded-lg p-0"
                    >
                      <Minus className="h-4 w-4" aria-hidden="true" />
                    </SecondaryButton>
                  </div>
                  <ResourceField
                    label="匹配内容"
                    hint={option.hint}
                    error={getWAFCustomRuleError(option)}
                    className="mt-3"
                  >
                    <ResourceTextarea
                      className="min-h-20"
                      disabled={!wafEnabled}
                      placeholder={option.placeholder}
                      {...form.register(option.key)}
                    />
                  </ResourceField>
                </div>
              ))}
            </div>
          )}
        </ProtectionSubsection>
      </form>
    </ConfigSectionShell>
  );
}
