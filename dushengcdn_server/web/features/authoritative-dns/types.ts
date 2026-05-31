export type DNSRecordType = 'A' | 'AAAA' | 'CNAME' | 'TXT' | 'MX' | 'NS' | 'SOA';

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
  created_at: string;
  updated_at: string;
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
