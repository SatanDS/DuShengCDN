export type DNSRecordType =
  | 'A'
  | 'AAAA'
  | 'CNAME'
  | 'TXT'
  | 'MX'
  | 'NS'
  | 'SOA';

export interface DNSZoneItem {
  id: number;
  name: string;
  soa_email: string;
  primary_ns: string;
  name_servers: string[];
  default_ttl: number;
  serial: number;
  enabled: boolean;
  record_count: number;
  records?: DNSRecordItem[];
  created_at: string;
  updated_at: string;
}

export interface DNSRecordItem {
  id: number;
  zone_id: number;
  name: string;
  type: DNSRecordType;
  value: string;
  ttl: number;
  priority: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface DNSWorkerItem {
  id: number;
  worker_id: string;
  name: string;
  token?: string;
  public_address: string;
  version: string;
  status: 'online' | 'offline';
  last_snapshot_version: string;
  last_snapshot_at?: string | null;
  last_seen_at?: string | null;
  last_error: string;
  last_probe_at?: string | null;
  last_probe_query: string;
  last_probe_results: DNSWorkerProbeResult[];
  probe_status: DNSWorkerProbeStatus;
  probe_healthy: boolean;
  probe_age_seconds: number;
  probe_message: string;
  created_at: string;
  updated_at: string;
}

export type DNSWorkerProbeStatus =
  | 'healthy'
  | 'partial'
  | 'failed'
  | 'stale'
  | 'unknown';

export interface DNSWorkerProbeResult {
  network: string;
  reachable: boolean;
  duration_ms: number;
  rcode: string;
  answer_count: number;
  error?: string;
}

export interface DNSWorkerProbe {
  worker_id: string;
  name: string;
  public_address: string;
  query_name: string;
  query_type: string;
  checked_at: string;
  results: DNSWorkerProbeResult[];
}

export interface DNSWorkerProbePayload {
  zone_id?: number;
}

export interface DNSGSLBSimulationPayload {
  proxy_route_id: number;
  qname: string;
  record_type: 'A' | 'AAAA';
  country?: string;
  source_ip?: string;
  fresh?: boolean;
}

export interface DNSGSLBSimulationResult {
  proxy_route_id: number;
  site_name: string;
  qname: string;
  record_type: 'A' | 'AAAA';
  country: string;
  source_ip: string;
  source_scope: string;
  ttl: number;
  targets: string[];
  target_count: number;
  strategy: string;
  gslb_enabled: boolean;
  snapshot_version: string;
  snapshot_at: string;
  message: string;
}

export interface DNSObservabilityCounterItem {
  key: string;
  label: string;
  count: number;
}

export interface DNSObservabilitySummary {
  window_hours: number;
  window_start: string;
  window_end: string;
  last_rollup_at?: string | null;
  total_queries: number;
  successful_queries: number;
  negative_queries: number;
  error_queries: number;
  dynamic_queries: number;
  static_queries: number;
  rcode_breakdown: DNSObservabilityCounterItem[];
  qtype_breakdown: DNSObservabilityCounterItem[];
  top_qnames: DNSObservabilityCounterItem[];
  top_targets: DNSObservabilityCounterItem[];
  worker_breakdown: DNSObservabilityCounterItem[];
  zone_breakdown: DNSObservabilityCounterItem[];
  route_breakdown: DNSObservabilityCounterItem[];
  source_scope_breakdown: DNSObservabilityCounterItem[];
  trend_points: DNSObservabilityTrendPoint[];
  snapshot_consistency: DNSWorkerSnapshotConsistency;
  worker_health: DNSWorkerHealthSummary;
}

export interface DNSObservabilityTrendPoint {
  bucket_started_at: string;
  query_count: number;
  successful_queries: number;
  negative_queries: number;
  error_queries: number;
  dynamic_queries: number;
  static_queries: number;
  noerror_queries: number;
  nxdomain_queries: number;
  servfail_queries: number;
}

export type DNSWorkerSnapshotConsistencyStatus =
  | 'consistent'
  | 'divergent'
  | 'stale'
  | 'no_online_workers'
  | 'unknown';

export interface DNSWorkerSnapshotVersion {
  version: string;
  worker_count: number;
  latest_snapshot_at?: string | null;
  workers: string[];
}

export interface DNSWorkerSnapshotWorker {
  worker_id: string;
  name: string;
  status: 'online' | 'offline';
  snapshot_version: string;
  last_snapshot_at?: string | null;
  last_seen_at?: string | null;
  stale: boolean;
}

export interface DNSWorkerSnapshotConsistency {
  status: DNSWorkerSnapshotConsistencyStatus;
  checked_at: string;
  snapshot_max_age_seconds: number;
  total_worker_count: number;
  online_worker_count: number;
  stale_worker_count: number;
  divergent_worker_count: number;
  latest_snapshot_version: string;
  latest_snapshot_at?: string | null;
  version_breakdown: DNSWorkerSnapshotVersion[];
  workers: DNSWorkerSnapshotWorker[];
}

export interface DNSWorkerHealthSummary {
  checked_at: string;
  total_worker_count: number;
  online_worker_count: number;
  probe_healthy_count: number;
  probe_checked_count: number;
  probe_healthy_percent: number;
  availability_percent: number;
  average_latency_ms: number;
  max_latency_ms: number;
  error_rate_percent: number;
  workers: DNSWorkerHealthItem[];
}

export interface DNSWorkerHealthItem {
  worker_id: string;
  name: string;
  status: 'online' | 'offline';
  public_address: string;
  query_count: number;
  error_queries: number;
  error_rate_percent: number;
  average_latency_ms: number;
  max_latency_ms: number;
  last_seen_at?: string | null;
  last_snapshot_at?: string | null;
  snapshot_age_seconds: number;
  snapshot_stale: boolean;
  last_error: string;
  last_probe_at?: string | null;
  last_probe_results: DNSWorkerProbeResult[];
  probe_status: DNSWorkerProbeStatus;
  probe_healthy: boolean;
  probe_age_seconds: number;
  probe_message: string;
}

export type DNSZoneDelegationStatus =
  | 'matched'
  | 'partial'
  | 'mismatch'
  | 'failed'
  | 'not_configured';

export interface DNSZoneDelegationCheck {
  zone_id: number;
  zone_name: string;
  expected_name_servers: string[];
  actual_name_servers: string[];
  matched_name_servers: string[];
  missing_name_servers: string[];
  extra_name_servers: string[];
  glue_required: boolean;
  glue_name_servers: string[];
  status: DNSZoneDelegationStatus;
  checked_at: string;
  error?: string;
}

export interface DNSZoneMutationPayload {
  name: string;
  soa_email: string;
  primary_ns: string;
  name_servers: string[];
  default_ttl: number;
  enabled: boolean;
}

export interface DNSRecordMutationPayload {
  zone_id?: number;
  name: string;
  type: DNSRecordType;
  value: string;
  ttl: number;
  priority: number;
  enabled: boolean;
}

export interface DNSWorkerMutationPayload {
  name: string;
  public_address: string;
}
