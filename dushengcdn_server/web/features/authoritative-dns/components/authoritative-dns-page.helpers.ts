import type {
  AuthoritativeDNSMigrationCandidate,
  DNSGSLBSimulationResult,
  DNSGSLBSchedulingStateStatus,
  DNSObservabilityCounterItem,
  DNSObservabilitySummary,
  DNSRecordItem,
  DNSRecordType,
  DNSWorkerHealthItem,
  DNSWorkerItem,
  DNSWorkerProbe,
  DNSWorkerProbeResult,
  DNSWorkerProbeStatus,
  DNSWorkerSnapshotConsistency,
  DNSWorkerSnapshotConsistencyStatus,
  DNSZoneDelegationCheck,
  DNSZoneDelegationStatus,
  DNSZoneItem,
} from '@/features/authoritative-dns/types';
import type { ProxyRouteItem } from '@/features/proxy-routes/types';
import { formatCountryName } from '@/lib/utils/countries';

export type FeedbackState = {
  tone: 'info' | 'success' | 'danger';
  message: string;
};

export type ActiveTab = 'zones' | 'workers' | 'migration';

export type ZoneFormValues = {
  name: string;
  soa_email: string;
  primary_ns: string;
  name_servers_text: string;
  default_ttl: number;
  enabled: boolean;
};

export type RecordFormValues = {
  name: string;
  type: DNSRecordType;
  value: string;
  ip_values: string[];
  ttl: number;
  priority: number;
  enabled: boolean;
};

export type WorkerFormValues = {
  name: string;
  public_address: string;
  remark: string;
};

export type WorkerSettingsFormValues = {
  remark: string;
};

export type GSLBSimulationFormValues = {
  proxy_route_id: string;
  qname: string;
  record_type: 'A' | 'AAAA';
  country: string;
  operator: string;
  asn: string;
  source_ip: string;
};

export type MigrationRecheckStepStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'warning'
  | 'danger';

export type MigrationRecheckStatus =
  | 'running'
  | 'success'
  | 'warning'
  | 'danger';

export type MigrationRecheckStepKey =
  | 'mode'
  | 'delegation'
  | 'worker_probe'
  | 'simulation';

export type MigrationRecheckStep = {
  key: MigrationRecheckStepKey;
  label: string;
  status: MigrationRecheckStepStatus;
  message: string;
};

export type MigrationRecheckResult = {
  routeId: number;
  routeName: string;
  zoneId: number;
  zoneName: string;
  checkedAt: string;
  status: MigrationRecheckStatus;
  steps: MigrationRecheckStep[];
  delegationCheck: DNSZoneDelegationCheck | null;
  workerProbes: DNSWorkerProbe[];
  simulations: DNSGSLBSimulationResult[];
};

export function isMeaningfulTime(value: string | null | undefined) {
  return Boolean(value) && !String(value).startsWith('0001-01-01');
}

export function getDNSWorkerDisplayName(
  worker: Pick<DNSWorkerItem | DNSWorkerHealthItem, 'name' | 'remark'>,
) {
  const remark = worker.remark?.trim();
  return remark || worker.name;
}

export const defaultDNSObservabilityWindowHours = 6;

export const dnsObservabilityWindowOptions = [
  { key: '6h', label: '6 小时', hours: 6 },
  { key: '12h', label: '12 小时', hours: 12 },
  { key: '24h', label: '24 小时', hours: 24 },
  { key: '3d', label: '3 天', hours: 72 },
  { key: '7d', label: '7 天', hours: 168 },
];

export const migrationRecheckStepTemplates: Array<{
  key: MigrationRecheckStepKey;
  label: string;
}> = [
  { key: 'mode', label: '网站解析模式' },
  { key: 'delegation', label: '托管域名指向检查' },
  { key: 'worker_probe', label: '响应端公网探测' },
  { key: 'simulation', label: '智能解析复测' },
];

export const dnsRecordTypes: DNSRecordType[] = [
  'A',
  'AAAA',
  'CNAME',
  'TXT',
  'MX',
  'NS',
  'SOA',
  'CAA',
  'SRV',
  'HTTPS',
  'SVCB',
  'TLSA',
];

export const gslbSimulationOperatorOptions = [
  { value: 'cn-telecom', label: '电信' },
  { value: 'cn-unicom', label: '联通' },
  { value: 'cn-mobile', label: '移动' },
  { value: 'cn-broadcast', label: '广电' },
  { value: 'cernet', label: '教育网' },
];

export const proxyRouteDetailPath = '/proxy-route/detail';

export function linesFromText(value: string) {
  return value
    .split(/[\r\n,，;；]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function getRouteDomains(route: ProxyRouteItem) {
  const domains = route.domains?.length ? route.domains : [route.domain];
  return domains.map((domain) => domain.trim()).filter(Boolean);
}

export function getRouteDetailHref(route: ProxyRouteItem) {
  return `${proxyRouteDetailPath}?id=${route.id}`;
}

export function getGSLBDescription(route: ProxyRouteItem) {
  if (!route.gslb_enabled) {
    return route.node_pool || 'default';
  }

  const enabledPools =
    route.gslb_policy?.pools?.filter((pool) => pool.enabled) ?? [];
  if (enabledPools.length === 0) {
    return '已启用，未配置节点池';
  }

  return enabledPools
    .map((pool) => `${pool.name || '未命名'}:${pool.weight}`)
    .join(' / ');
}

export function getRouteDisplayName(route: ProxyRouteItem) {
  return route.site_name || route.primary_domain || route.domain;
}

export function getDefaultSimulationQName(route?: ProxyRouteItem | null) {
  if (!route) {
    return '';
  }
  const domain =
    getRouteDomains(route)[0] ?? route.primary_domain ?? route.domain ?? '';
  if (domain.startsWith('*.')) {
    return `www.${domain.slice(2)}`;
  }
  return domain;
}

export function getRouteRecordType(
  route?: ProxyRouteItem | null,
): 'A' | 'AAAA' {
  return route?.dns_record_type === 'AAAA' ? 'AAAA' : 'A';
}

export function getCandidateRecordType(
  candidate: AuthoritativeDNSMigrationCandidate,
): 'A' | 'AAAA' {
  return candidate.dns_record_type === 'AAAA' ? 'AAAA' : 'A';
}

export function createMigrationRecheckResult(
  route: ProxyRouteItem,
  zone: DNSZoneItem,
  status: MigrationRecheckStatus = 'running',
): MigrationRecheckResult {
  return {
    routeId: route.id,
    routeName: getRouteDisplayName(route),
    zoneId: zone.id,
    zoneName: zone.name,
    checkedAt: new Date().toISOString(),
    status,
    steps: migrationRecheckStepTemplates.map((step) => ({
      ...step,
      status: 'pending',
      message: '等待复测',
    })),
    delegationCheck: null,
    workerProbes: [],
    simulations: [],
  };
}

export function updateMigrationRecheckStep(
  result: MigrationRecheckResult,
  key: MigrationRecheckStepKey,
  status: MigrationRecheckStepStatus,
  message: string,
): MigrationRecheckResult {
  return {
    ...result,
    checkedAt: new Date().toISOString(),
    steps: result.steps.map((step) =>
      step.key === key ? { ...step, status, message } : step,
    ),
  };
}

export function finalizeMigrationRecheck(
  result: MigrationRecheckResult,
): MigrationRecheckResult {
  const hasDanger = result.steps.some((step) => step.status === 'danger');
  const hasWarning = result.steps.some((step) => step.status === 'warning');
  return {
    ...result,
    checkedAt: new Date().toISOString(),
    status: hasDanger ? 'danger' : hasWarning ? 'warning' : 'success',
  };
}

export function mergeUpdatedRoute(
  routes: ProxyRouteItem[],
  updatedRoute: ProxyRouteItem,
) {
  let found = false;
  const nextRoutes = routes.map((route) => {
    if (route.id !== updatedRoute.id) {
      return route;
    }
    found = true;
    return updatedRoute;
  });
  return found ? nextRoutes : [updatedRoute, ...nextRoutes];
}

export function getRouteSimulationCountries(route: ProxyRouteItem) {
  const countries = new Set<string>();
  for (const pool of route.gslb_policy?.pools ?? []) {
    if (!pool.enabled) {
      continue;
    }
    for (const country of pool.countries ?? []) {
      const normalized = country.trim().toUpperCase();
      if (normalized) {
        countries.add(normalized);
      }
      if (countries.size >= 2) {
        break;
      }
    }
    if (countries.size >= 2) {
      break;
    }
  }
  return ['', ...Array.from(countries)].slice(0, 3);
}

export function getRecheckStepBadgeVariant(status: MigrationRecheckStepStatus) {
  switch (status) {
    case 'success':
      return 'success' as const;
    case 'warning':
      return 'warning' as const;
    case 'danger':
      return 'danger' as const;
    case 'running':
      return 'info' as const;
    case 'pending':
      return 'warning' as const;
  }
}

export function getRecheckStepStatusLabel(status: MigrationRecheckStepStatus) {
  switch (status) {
    case 'success':
      return '通过';
    case 'warning':
      return '注意';
    case 'danger':
      return '失败';
    case 'running':
      return '执行中';
    case 'pending':
      return '等待';
  }
}

export function getMigrationRecheckTone(status: MigrationRecheckStatus) {
  return status === 'success'
    ? ('success' as const)
    : status === 'danger'
      ? ('danger' as const)
      : ('info' as const);
}

export function zoneToFormValues(zone?: DNSZoneItem | null): ZoneFormValues {
  return {
    name: zone?.name ?? '',
    soa_email: zone?.soa_email ?? '',
    primary_ns: zone?.primary_ns ?? '',
    name_servers_text: zone?.name_servers.join('\n') ?? '',
    default_ttl: zone?.default_ttl ?? 300,
    enabled: zone?.enabled ?? true,
  };
}

export function recordToFormValues(
  record?: DNSRecordItem | null,
): RecordFormValues {
  const value = record?.value ?? '';
  return {
    name: record?.name ?? '@',
    type: record?.type ?? 'A',
    value,
    ip_values: value ? [value] : [''],
    ttl: record?.ttl ?? 0,
    priority: record?.priority ?? 0,
    enabled: record?.enabled ?? true,
  };
}

export function isAddressRecordType(type: DNSRecordType) {
  return type === 'A' || type === 'AAAA';
}

export function isPriorityRecordType(type: DNSRecordType) {
  return type === 'MX' || type === 'SRV' || type === 'HTTPS' || type === 'SVCB';
}

export function getRecordValueLabel(type: DNSRecordType) {
  switch (type) {
    case 'A':
      return 'IP 地址';
    case 'AAAA':
      return 'IP 地址';
    case 'MX':
      return '邮件服务器';
    case 'CNAME':
      return '目标域名';
    case 'NS':
      return 'NS 域名';
    case 'TXT':
      return 'TXT 内容';
    case 'SOA':
      return 'SOA 内容';
    case 'CAA':
      return 'CAA 内容';
    case 'SRV':
      return 'SRV 目标';
    case 'HTTPS':
      return 'HTTPS 参数';
    case 'SVCB':
      return 'SVCB 参数';
    case 'TLSA':
      return 'TLSA 内容';
  }
}

export function getRecordValueHint(type: DNSRecordType) {
  switch (type) {
    case 'A':
      return '这里填写 IPv4，一个输入框一个 IP；点 + 可继续添加，保存时会创建多条 A 记录。';
    case 'AAAA':
      return '这里填写 IPv6，一个输入框一个 IP；点 + 可继续添加，保存时会创建多条 AAAA 记录。';
    case 'CNAME':
      return '填写目标域名，同名下不要再添加其它记录。';
    case 'MX':
      return '记录值填写邮件服务器域名；MX 优先级数字越小越优先，例如 10 会早于 20。';
    case 'NS':
      return '填写注册商要指向的解析服务器域名。';
    case 'SOA':
      return '填写域名基础信息，通常由系统自动生成即可。';
    case 'TXT':
      return '填写 TXT 文本内容。';
    case 'CAA':
      return '填写 CAA RDATA，例如 0 issue "letsencrypt.org"。';
    case 'SRV':
      return '填写权重、端口和目标，例如 5 443 sip.example.com；优先级在下方单独填写。';
    case 'HTTPS':
      return '填写 HTTPS SVCB 参数，例如 . alpn=h2,h3 ipv4hint=203.0.113.10；优先级在下方单独填写。';
    case 'SVCB':
      return '填写 SVCB 参数，例如 svc.example.com alpn=h2；优先级在下方单独填写。';
    case 'TLSA':
      return '填写 TLSA RDATA，例如 3 1 1 后接证书摘要。';
  }
}

export function getRecordValuePlaceholder(type: DNSRecordType) {
  switch (type) {
    case 'A':
      return '203.0.113.10';
    case 'AAAA':
      return '2001:db8::10';
    case 'MX':
      return 'mail.example.com';
    case 'CNAME':
      return 'target.example.com';
    case 'NS':
      return 'ns1.example.com';
    case 'TXT':
      return 'v=spf1 ...';
    case 'SOA':
      return 'ns1.example.com hostmaster.example.com ...';
    case 'CAA':
      return '0 issue "letsencrypt.org"';
    case 'SRV':
      return '5 443 sip.example.com';
    case 'HTTPS':
      return '. alpn=h2,h3';
    case 'SVCB':
      return 'svc.example.com alpn=h2';
    case 'TLSA':
      return '3 1 1 d2abde240d7cd3ee...';
  }
}

export function getWorkerStatusVariant(status: DNSWorkerItem['status']) {
  return status === 'online' ? 'success' : 'warning';
}

export function getWorkerSourceCapabilityCounts(workers: DNSWorkerItem[]) {
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  const source = onlineWorkers.length > 0 ? onlineWorkers : workers;
  return {
    total: source.length,
    country: source.filter((worker) => worker.geoip_country_enabled).length,
    asn: source.filter((worker) => worker.geoip_asn_enabled).length,
    operator: source.filter((worker) => worker.geoip_operator_enabled).length,
  };
}

export function getWorkerSourceCapabilityMessage(workers: DNSWorkerItem[]) {
  const counts = getWorkerSourceCapabilityCounts(workers);
  if (counts.total === 0) {
    return '尚未创建 DNS 响应端，运营商/ASN 规则会在响应端部署并上报识别库后生效。';
  }
  return `识别库能力：国家 ${counts.country}/${counts.total}，ASN ${counts.asn}/${counts.total}，运营商 ${counts.operator}/${counts.total}。运营商来自 gaoyifan/china-operator-ip CIDR 库，ASN 来自 GeoLite2-ASN 或兼容 MMDB。`;
}

export function getWorkerSourceCapabilityTone(workers: DNSWorkerItem[]) {
  const counts = getWorkerSourceCapabilityCounts(workers);
  if (counts.total === 0) {
    return 'info' as const;
  }
  if (counts.asn === counts.total && counts.operator === counts.total) {
    return 'success' as const;
  }
  return 'info' as const;
}

export function formatCount(value: number) {
  return value.toLocaleString('zh-CN');
}

export function formatPercent(numerator: number, denominator: number) {
  if (denominator <= 0) {
    return '0%';
  }
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

export function formatPercentValue(value: number) {
  return `${value.toFixed(1)}%`;
}

export function formatLatencyMs(value: number) {
  if (value <= 0) {
    return '0 ms';
  }
  return `${Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)} ms`;
}

export function formatSourceScopeLabel(value: string) {
  const text = value.trim();
  if (!text) {
    return '全局';
  }
  const parts = splitSourceScopeParts(text);
  const base = getSourceScopeBase(text);
  const bucket = parts.find((item) => item.toLowerCase().startsWith('bucket:'));
  const baseLabel = formatSourceScopeBaseLabel(base);
  if (!bucket) {
    return baseLabel;
  }
  const bucketValue = bucket.split(':')[1]?.trim();
  return bucketValue ? `${baseLabel} / 分流桶 ${bucketValue}` : baseLabel;
}

export function formatSourceScopeBaseLabel(value: string) {
  const text = value.trim();
  const lower = text.toLowerCase();
  if (lower === 'global') {
    return '全局';
  }
  if (lower.startsWith('country:')) {
    const country = text
      .slice(text.indexOf(':') + 1)
      .trim()
      .toUpperCase();
    return country ? formatCountryName(country, country) : text;
  }
  if (lower.startsWith('cidr:')) {
    const cidr = text.slice(text.indexOf(':') + 1).trim();
    return cidr ? `网段 ${cidr}` : text;
  }
  if (lower.startsWith('operator:')) {
    const operator = text.slice(text.indexOf(':') + 1).trim();
    return operator ? `运营商 ${formatGSLBOperatorLabel(operator)}` : text;
  }
  if (lower.startsWith('asn:')) {
    const asn = text
      .slice(text.indexOf(':') + 1)
      .trim()
      .replace(/^AS/i, '');
    return asn ? `ASN AS${asn}` : text;
  }
  return text;
}

export function splitSourceScopeParts(value: string) {
  return value
    .split('|')
    .map((item) => item.trim())
    .filter(Boolean);
}

export function getSourceScopeBase(value: string) {
  return splitSourceScopeParts(value)[0] ?? 'global';
}

export function getSourceScopeCounterKey(item: DNSObservabilityCounterItem) {
  return item.key || item.label || '';
}

export function isSourceScopeASNItem(item: DNSObservabilityCounterItem) {
  return getSourceScopeBase(getSourceScopeCounterKey(item))
    .toLowerCase()
    .startsWith('asn:');
}

export function isSourceScopeCountryItem(item: DNSObservabilityCounterItem) {
  return getSourceScopeBase(getSourceScopeCounterKey(item))
    .toLowerCase()
    .startsWith('country:');
}

export function formatSourceCountryLabel(item: DNSObservabilityCounterItem) {
  const value = (item.key || item.label || '').trim();
  if (!value) {
    return item.label || item.key;
  }
  return formatCountryName(value.toUpperCase(), value.toUpperCase());
}

export function formatSourceASNLabel(item: DNSObservabilityCounterItem) {
  const value = (item.key || item.label || '').trim().replace(/^AS/i, '');
  return value ? `ASN AS${value}` : item.label || item.key;
}

export function formatGSLBOperatorLabel(value: string) {
  const normalized = value.trim().toLowerCase();
  return (
    gslbSimulationOperatorOptions.find((option) => option.value === normalized)
      ?.label ?? value
  );
}

export function parseSimulationASN(value: string) {
  const normalized = value
    .trim()
    .replace(/^asn:\s*/i, '')
    .replace(/^as/i, '');
  if (!normalized) {
    return undefined;
  }
  const asn = Number(normalized);
  if (!Number.isInteger(asn) || asn <= 0 || asn > 4294967295) {
    return undefined;
  }
  return asn;
}

export function formatDNSRCodeLabel(item: DNSObservabilityCounterItem) {
  const code = (item.key || item.label || '').trim().toUpperCase();
  const labels: Record<string, string> = {
    NOERROR: '正常响应',
    NODATA: '无记录数据',
    NXDOMAIN: '域名不存在',
    SERVFAIL: '响应端故障',
    REFUSED: '拒绝查询',
    FORMERR: '请求格式错误',
    NOTIMP: '暂不支持',
  };
  return labels[code] || item.label || item.key || '未命名';
}

export function isProbeSchedulingGateMessage(message: string) {
  return message.includes('Agent 探测未达到调度门槛');
}

export function formatDiagnosticMessage(message: string) {
  return message
    .replaceAll('节点负载超过 GSLB 阈值', '节点压力超过上限')
    .replaceAll(
      '当前 Server 生成的权威 DNS 快照',
      '当前面板生成的本地解析配置快照',
    )
    .replaceAll('OpenResty 健康', '代理服务是否正常')
    .replaceAll('GSLB 阈值', '压力上限')
    .replaceAll('GSLB 负载阈值', '压力上限')
    .replaceAll('GSLB', '多节点智能解析')
    .replaceAll('DNS Worker 多点探测', '响应端多地探测')
    .replaceAll('DNS Worker', 'DNS 响应端')
    .replaceAll('调度防抖状态', '切换冷却状态')
    .replaceAll('DNS 防抖状态', '解析切换冷却状态')
    .replaceAll('Agent 探测未达到调度门槛', '节点到响应端探测未达要求')
    .replaceAll('调度门槛', '选择要求')
    .replaceAll('来源 CIDR', '来源网段')
    .replaceAll('到 响应端', '到响应端')
    .replaceAll('的 响应端', '的响应端');
}

export function formatDurationSeconds(value: number) {
  if (value <= 0) {
    return '—';
  }
  if (value < 60) {
    return `${value} 秒`;
  }
  if (value < 3600) {
    return `${Math.round(value / 60)} 分钟`;
  }
  return `${Math.round(value / 3600)} 小时`;
}

export function getProbeResultVariant(result: DNSWorkerProbeResult) {
  return result.reachable ? ('success' as const) : ('danger' as const);
}

export function getProbeStatusLabel(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return '公网可达';
    case 'partial':
      return '部分可达';
    case 'failed':
      return '探测失败';
    case 'stale':
      return '公网探测过期';
    case 'unknown':
      return '未探测';
  }
}

export function getProbeStatusVariant(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return 'success' as const;
    case 'partial':
    case 'stale':
    case 'unknown':
      return 'warning' as const;
    case 'failed':
      return 'danger' as const;
  }
}

export function getNodeDNSProbeStatusLabel(status: DNSWorkerProbeStatus) {
  switch (status) {
    case 'healthy':
      return '公网可达';
    case 'partial':
      return '部分可达';
    case 'failed':
      return '探测失败';
    case 'stale':
      return '多节点探测过期';
    case 'unknown':
      return '未探测';
  }
}

export function getNodeProbeStatusVariant(status: string) {
  switch (status) {
    case 'online':
      return 'success' as const;
    case 'pending':
      return 'warning' as const;
    default:
      return 'danger' as const;
  }
}

export function workerProbeToPanelData(
  worker: DNSWorkerItem,
): DNSWorkerProbe | null {
  if (!worker.last_probe_at || worker.last_probe_results.length === 0) {
    return null;
  }
  const [queryName = '', queryType = ''] = worker.last_probe_query.split(/\s+/);
  return {
    worker_id: worker.worker_id,
    name: worker.name,
    public_address: worker.public_address,
    query_name: queryName,
    query_type: queryType,
    checked_at: worker.last_probe_at,
    results: worker.last_probe_results,
  };
}

export function formatTrendHour(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function getSnapshotConsistencyLabel(
  status: DNSWorkerSnapshotConsistencyStatus,
) {
  switch (status) {
    case 'consistent':
      return '解析配置一致';
    case 'divergent':
      return '解析配置不一致';
    case 'stale':
      return '解析配置过期';
    case 'no_online_workers':
      return '无在线响应端';
    case 'unknown':
      return '状态未知';
  }
}

export function getSnapshotConsistencyVariant(
  status: DNSWorkerSnapshotConsistencyStatus,
) {
  switch (status) {
    case 'consistent':
      return 'success' as const;
    case 'divergent':
    case 'stale':
      return 'danger' as const;
    case 'no_online_workers':
    case 'unknown':
      return 'warning' as const;
  }
}

export function getSnapshotConsistencyMessage(
  consistency: DNSWorkerSnapshotConsistency,
) {
  if (consistency.status === 'stale') {
    return `解析配置过期：有 ${formatCount(consistency.stale_worker_count)} 个在线响应端超过 ${formatDurationSeconds(consistency.snapshot_max_age_seconds)} 没有拉取最新解析配置。请检查响应端能否访问面板地址、响应端密钥是否有效，以及服务日志中 /api/dns-snapshot 的 HTTP 状态。`;
  }
  if (consistency.status === 'divergent') {
    const versionCount = consistency.version_breakdown.length;
    const latestVersion = consistency.latest_snapshot_version || '未知版本';
    return `解析配置不一致：在线响应端当前使用了 ${formatCount(versionCount)} 个配置版本，查询结果可能不一致。最新版本 ${latestVersion}；请检查落后的响应端是否能访问面板、响应端密钥是否仍有效，必要时重启响应端重新拉取配置。`;
  }
  return '';
}

export function getDNSObservabilityRollupHint(
  summary: DNSObservabilitySummary,
) {
  if (summary.total_queries > 0) {
    return '';
  }
  const healthWorkers = summary.worker_health?.workers ?? [];
  const consistencyWorkers = summary.snapshot_consistency?.workers ?? [];
  const workers = healthWorkers.length > 0 ? healthWorkers : consistencyWorkers;
  if (workers.length === 0) {
    return '当前还没有 DNS 响应端。创建并部署响应端后，页面才会收到 DNS 查询观测数据。';
  }
  const onlineWorkers = workers.filter((worker) => worker.status === 'online');
  if (onlineWorkers.length === 0) {
    return '当前没有在线 DNS 响应端，查询观测不会产生数据。请先确认响应端进程、53 端口和面板连接。';
  }
  const heartbeatWorkers = onlineWorkers.filter((worker) =>
    isMeaningfulTime(worker.last_heartbeat_at),
  );
  if (heartbeatWorkers.length === 0) {
    return '响应端能拉取解析配置，但还没有成功发送心跳。请检查响应端到面板的 /api/dns-worker-heartbeat 请求、响应端密钥和反代限制。';
  }
  const rollupWorkers = onlineWorkers.filter((worker) =>
    isMeaningfulTime(worker.last_rollup_at),
  );
  if (rollupWorkers.length === 0) {
    return '响应端心跳正常，但还没有上报 DNS 查询 rollup。请确认公网 NS 流量是否真正打到这些响应端，以及响应端版本是否包含查询观测上报。';
  }
  return '最近窗口内没有 DNS 查询 rollup。若业务确实有查询，请检查客户端使用的 NS、域名委派、响应端时间和面板时间是否一致。';
}

export function shouldShowNoAuthoritativeRoutesNotice({
  zones,
  workers,
  routes,
  routesLoading,
  routesError,
}: {
  zones: DNSZoneItem[];
  workers: DNSWorkerItem[];
  routes: ProxyRouteItem[];
  routesLoading: boolean;
  routesError: boolean;
}) {
  if (routesLoading || routesError) {
    return false;
  }
  const hasEnabledZone = zones.some((zone) => zone.enabled);
  const hasReadyWorker = workers.some(
    (worker) =>
      worker.status === 'online' &&
      Boolean(worker.last_snapshot_version || worker.last_snapshot_at),
  );
  const hasAuthoritativeRoute = routes.some(
    (route) =>
      route.enabled &&
      route.dns_provider_mode === 'authoritative' &&
      route.dns_zone_id_ref != null,
  );
  return hasEnabledZone && hasReadyWorker && !hasAuthoritativeRoute;
}

export function getSchedulingStateStatusLabel(
  status: DNSGSLBSchedulingStateStatus,
) {
  switch (status) {
    case 'active':
      return '已生效';
    case 'debouncing':
      return '冷却中';
    case 'inactive':
      return '站点停用';
    case 'stale':
      return '记录过期';
    case 'empty':
      return '无目标';
    case 'orphaned':
      return '站点已删除';
  }
}

export function getSchedulingStateStatusVariant(
  status: DNSGSLBSchedulingStateStatus,
) {
  switch (status) {
    case 'active':
      return 'success' as const;
    case 'debouncing':
      return 'info' as const;
    case 'inactive':
    case 'stale':
    case 'empty':
      return 'warning' as const;
    case 'orphaned':
      return 'danger' as const;
  }
}

export function getDelegationStatusLabel(status: DNSZoneDelegationStatus) {
  switch (status) {
    case 'matched':
      return '已匹配';
    case 'partial':
      return '部分匹配';
    case 'mismatch':
      return '不匹配';
    case 'failed':
      return '检查失败';
    case 'not_configured':
      return '未配置';
  }
}

export function getDelegationStatusVariant(status: DNSZoneDelegationStatus) {
  switch (status) {
    case 'matched':
      return 'success' as const;
    case 'partial':
    case 'not_configured':
      return 'warning' as const;
    case 'mismatch':
    case 'failed':
      return 'danger' as const;
  }
}
