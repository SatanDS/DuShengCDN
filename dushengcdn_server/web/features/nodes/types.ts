import type { ReleaseChannel } from '@/features/update/types';

export interface NodeItem {
  id: number;
  node_id: string;
  name: string;
  ip: string;
  pool_name: string;
  tags: string[];
  weight: number;
  public_ips: string[];
  scheduling_enabled: boolean;
  drain_mode: boolean;
  geo_name: string;
  geo_latitude?: number | null;
  geo_longitude?: number | null;
  geo_manual_override: boolean;
  agent_token?: string;
  agent_token_prefix: string;
  agent_token_available: boolean;
  auto_update_enabled: boolean;
  update_requested: boolean;
  update_channel: ReleaseChannel;
  update_tag: string;
  restart_openresty_requested: boolean;
  agent_version: string;
  nginx_version: string;
  openresty_status: 'healthy' | 'unhealthy' | 'unknown';
  openresty_message: string;
  status: 'online' | 'offline' | 'pending';
  current_version: string;
  current_checksum: string;
  target_config_version: string;
  target_config_checksum: string;
  target_config_pool: string;
  target_config_available: boolean;
  config_in_sync: boolean;
  last_seen_at: string;
  last_error: string;
  latest_apply_result: 'success' | 'warning' | 'failed' | '';
  latest_apply_message: string;
  latest_apply_checksum: string;
  latest_main_config_checksum: string;
  latest_route_config_checksum: string;
  latest_support_file_count: number;
  latest_apply_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface NodeBootstrapToken {
  discovery_token?: string;
  discovery_token_prefix: string;
  discovery_token_available: boolean;
}

export interface NodeAgentToken {
  agent_token?: string;
  agent_token_prefix: string;
  agent_token_available: boolean;
}

export interface NodeDeleteResult {
  node_id: string;
  name: string;
  uninstall_agent_requested: boolean;
  uninstall_agent_message: string;
}

export interface NodeMutationPayload {
  name: string;
  ip: string;
  pool_name?: string;
  tags?: string[];
  weight?: number;
  public_ips?: string[];
  scheduling_enabled?: boolean;
  drain_mode?: boolean;
  auto_update_enabled: boolean;
  geo_name: string;
  geo_latitude?: number | null;
  geo_longitude?: number | null;
  geo_manual_override: boolean;
}

export interface NodeAgentReleaseInfo {
  tag_name: string;
  body: string;
  html_url: string;
  published_at: string;
  current_version: string;
  has_update: boolean;
  channel: ReleaseChannel;
  prerelease: boolean;
  update_requested: boolean;
  requested_channel: ReleaseChannel;
  requested_tag: string;
}

export interface NodeAgentUpdatePayload {
  channel?: ReleaseChannel;
  tag_name?: string;
}

export interface NodeSystemProfile {
  hostname: string;
  os_name: string;
  os_version: string;
  kernel_version: string;
  architecture: string;
  cpu_model: string;
  cpu_cores: number;
  total_memory_bytes: number;
  total_disk_bytes: number;
  uptime_seconds: number;
  reported_at: string;
}

export interface NodeMetricSnapshot {
  captured_at: string;
  cpu_usage_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  storage_used_bytes: number;
  storage_total_bytes: number;
  disk_read_bytes: number;
  disk_write_bytes: number;
  network_rx_bytes: number;
  network_tx_bytes: number;
  openresty_rx_bytes: number;
  openresty_tx_bytes: number;
  openresty_connections: number;
}

export interface NodeTrafficReport {
  window_started_at: string;
  window_ended_at: string;
  request_count: number;
  error_count: number;
  cache_hit_count: number;
  cache_miss_count: number;
  cache_bypass_count: number;
  cache_expired_count: number;
  cache_stale_count: number;
  upstream_error_count: number;
  upstream_response_ms: number;
  unique_visitor_count: number;
  status_codes_json: string;
  top_domains_json: string;
  source_countries_json: string;
}

export interface NodeTrafficTrendPoint {
  bucket_started_at: string;
  request_count: number;
  error_count: number;
  unique_visitor_count: number;
}

export interface NodeCapacityTrendPoint {
  bucket_started_at: string;
  average_cpu_usage_percent: number;
  average_memory_usage_percent: number;
  reported_nodes: number;
}

export interface NodeNetworkTrendPoint {
  bucket_started_at: string;
  network_rx_bytes: number;
  network_tx_bytes: number;
  openresty_rx_bytes: number;
  openresty_tx_bytes: number;
  reported_nodes: number;
}

export interface NodeDiskIOTrendPoint {
  bucket_started_at: string;
  disk_read_bytes: number;
  disk_write_bytes: number;
  reported_nodes: number;
}

export interface NodeDistributionItem {
  key: string;
  value: number;
}

export interface NodeTrafficDistributions {
  status_codes: NodeDistributionItem[];
  top_domains: NodeDistributionItem[];
  source_countries: NodeDistributionItem[];
}

export interface NodeTrafficSummary {
  window_started_at: string;
  window_ended_at: string;
  request_count: number;
  unique_visitor_count: number;
  error_count: number;
  cache_hit_count: number;
  cache_miss_count: number;
  cache_bypass_count: number;
  cache_expired_count: number;
  cache_stale_count: number;
  upstream_error_count: number;
  upstream_response_ms: number;
  cache_hit_rate_percent: number;
  average_upstream_response_ms: number;
  estimated_qps: number;
  error_rate_percent: number;
}

export interface NodeHealthSummary {
  active_alerts: number;
  critical_alerts: number;
  warning_alerts: number;
  info_alerts: number;
  resolved_alerts: number;
  has_capacity_risk: boolean;
  has_traffic_risk: boolean;
  has_runtime_risk: boolean;
}

export interface NodeObservabilityAnalytics {
  traffic: NodeTrafficSummary;
  distributions: NodeTrafficDistributions;
  health: NodeHealthSummary;
}

export interface NodeObservabilityTrends {
  traffic_24h: NodeTrafficTrendPoint[];
  capacity_24h: NodeCapacityTrendPoint[];
  network_24h: NodeNetworkTrendPoint[];
  disk_io_24h: NodeDiskIOTrendPoint[];
}

export interface NodeHealthEvent {
  event_type: string;
  severity: string;
  status: string;
  message: string;
  metadata_json?: string;
  first_triggered_at: string;
  last_triggered_at: string;
  reported_at: string;
  resolved_at?: string | null;
}

export interface NodeObservability {
  node_id: string;
  profile: NodeSystemProfile | null;
  metric_snapshots: NodeMetricSnapshot[];
  traffic_reports: NodeTrafficReport[];
  health_events: NodeHealthEvent[];
  analytics: NodeObservabilityAnalytics;
  trends: NodeObservabilityTrends;
}
