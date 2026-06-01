import { apiRequest } from '@/lib/api/client';

import type {
  AuthoritativeDNSMigrationCandidate,
  DNSGSLBSimulationPayload,
  DNSGSLBSimulationResult,
  DNSGSLBSchedulingStates,
  DNSObservabilitySummary,
  DNSRecordItem,
  DNSRecordMutationPayload,
  DNSWorkerItem,
  DNSWorkerMutationPayload,
  DNSWorkerProbe,
  DNSWorkerProbePayload,
  DNSZoneDelegationCheck,
  DNSZoneItem,
  DNSZoneMutationPayload,
} from '@/features/authoritative-dns/types';

export function getDNSZones() {
  return apiRequest<DNSZoneItem[]>('/dns-zones/').then(normalizeDNSZones);
}

export function getDNSZone(id: number) {
  return apiRequest<DNSZoneItem>(`/dns-zones/${id}`).then(normalizeDNSZone);
}

export function createDNSZone(payload: DNSZoneMutationPayload) {
  return apiRequest<DNSZoneItem>('/dns-zones/', {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSZone);
}

export function updateDNSZone(id: number, payload: DNSZoneMutationPayload) {
  return apiRequest<DNSZoneItem>(`/dns-zones/${id}/update`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSZone);
}

export function deleteDNSZone(id: number) {
  return apiRequest<void>(`/dns-zones/${id}/delete`, {
    method: 'POST',
  });
}

export function checkDNSZoneDelegation(id: number) {
  return apiRequest<DNSZoneDelegationCheck>(
    `/dns-zones/${id}/delegation-check`,
  ).then(normalizeDNSZoneDelegationCheck);
}

export function getDNSZoneRecords(zoneId: number) {
  return apiRequest<DNSRecordItem[]>(`/dns-zones/${zoneId}/records`).then(
    normalizeDNSRecords,
  );
}

export function createDNSZoneRecord(
  zoneId: number,
  payload: DNSRecordMutationPayload,
) {
  return apiRequest<DNSRecordItem>(`/dns-zones/${zoneId}/records`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSRecord);
}

export function updateDNSRecord(id: number, payload: DNSRecordMutationPayload) {
  return apiRequest<DNSRecordItem>(`/dns-records/${id}/update`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSRecord);
}

export function deleteDNSRecord(id: number) {
  return apiRequest<void>(`/dns-records/${id}/delete`, {
    method: 'POST',
  });
}

export function getDNSWorkers() {
  return apiRequest<DNSWorkerItem[]>('/dns-workers/').then(
    normalizeDNSWorkers,
  );
}

export function getDNSObservability(hours = 24) {
  const params = new URLSearchParams({ hours: String(hours) });
  return apiRequest<DNSObservabilitySummary>(
    `/dns-workers/observability?${params.toString()}`,
  ).then(normalizeDNSObservabilitySummary);
}

export function getDNSGSLBSchedulingStates() {
  return apiRequest<DNSGSLBSchedulingStates>(
    '/dns-workers/scheduling-states',
  ).then(normalizeDNSGSLBSchedulingStates);
}

export function getDNSMigrationCandidates() {
  return apiRequest<AuthoritativeDNSMigrationCandidate[]>(
    '/dns-workers/migration-candidates',
  ).then(normalizeDNSMigrationCandidates);
}

export function createDNSWorker(payload: DNSWorkerMutationPayload) {
  return apiRequest<DNSWorkerItem>('/dns-workers/', {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSWorker);
}

export function deleteDNSWorker(id: number) {
  return apiRequest<void>(`/dns-workers/${id}/delete`, {
    method: 'POST',
  });
}

export function probeDNSWorker(
  id: number,
  payload: DNSWorkerProbePayload = {},
) {
  return apiRequest<DNSWorkerProbe>(`/dns-workers/${id}/probe`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSWorkerProbe);
}

export function simulateDNSGSLB(payload: DNSGSLBSimulationPayload) {
  return apiRequest<DNSGSLBSimulationResult>('/dns-workers/simulate', {
    method: 'POST',
    body: JSON.stringify(payload),
  }).then(normalizeDNSGSLBSimulationResult);
}

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeDNSZones(value: DNSZoneItem[] | null | undefined) {
  return asArray(value).map(normalizeDNSZone);
}

function normalizeDNSZone(zone: DNSZoneItem) {
  return {
    ...zone,
    name_servers: asArray(zone.name_servers),
    records: zone.records ? normalizeDNSRecords(zone.records) : zone.records,
  };
}

function normalizeDNSRecords(value: DNSRecordItem[] | null | undefined) {
  return asArray(value).map(normalizeDNSRecord);
}

function normalizeDNSRecord(record: DNSRecordItem) {
  return record;
}

function normalizeDNSWorkers(value: DNSWorkerItem[] | null | undefined) {
  return asArray(value).map(normalizeDNSWorker);
}

function normalizeDNSWorker(worker: DNSWorkerItem) {
  return {
    ...worker,
    last_probe_results: asArray(worker.last_probe_results),
    probe_status: worker.probe_status || 'unknown',
  };
}

function normalizeDNSWorkerProbe(probe: DNSWorkerProbe) {
  return {
    ...probe,
    results: asArray(probe.results),
  };
}

function normalizeDNSGSLBSchedulingStates(
  value: DNSGSLBSchedulingStates | null | undefined,
) {
  return {
    checked_at: value?.checked_at ?? '',
    total: value?.total ?? 0,
    states: asArray(value?.states).map((state) => ({
      ...state,
      domains: asArray(state.domains),
      selected_targets: asArray(state.selected_targets),
      desired_targets: asArray(state.desired_targets),
    })),
  };
}

function normalizeDNSMigrationCandidates(
  value: AuthoritativeDNSMigrationCandidate[] | null | undefined,
) {
  return asArray(value).map((candidate) => ({
    ...candidate,
    domains: asArray(candidate.domains),
    blockers: asArray(candidate.blockers),
    warnings: asArray(candidate.warnings),
  }));
}

function normalizeDNSZoneDelegationCheck(check: DNSZoneDelegationCheck) {
  return {
    ...check,
    expected_name_servers: asArray(check.expected_name_servers),
    actual_name_servers: asArray(check.actual_name_servers),
    matched_name_servers: asArray(check.matched_name_servers),
    missing_name_servers: asArray(check.missing_name_servers),
    extra_name_servers: asArray(check.extra_name_servers),
    glue_name_servers: asArray(check.glue_name_servers),
  };
}

function normalizeDNSGSLBSimulationResult(result: DNSGSLBSimulationResult) {
  return {
    ...result,
    targets: asArray(result.targets),
    matched_pools: asArray(result.matched_pools).map((pool) => ({
      ...pool,
      countries: asArray(pool.countries),
      source_cidrs: asArray(pool.source_cidrs),
    })),
    nodes: asArray(result.nodes).map((node) => ({
      ...node,
      public_ips: asArray(node.public_ips),
      candidate_targets: asArray(node.candidate_targets),
      selected_targets: asArray(node.selected_targets),
      reasons: asArray(node.reasons),
      node_probe_status: node.node_probe_status || 'unknown',
    })),
  };
}

function normalizeDNSObservabilitySummary(
  summary: DNSObservabilitySummary | null | undefined,
): DNSObservabilitySummary | null | undefined {
  if (!summary) {
    return summary;
  }
  return {
    ...summary,
    rcode_breakdown: asArray(summary.rcode_breakdown),
    qtype_breakdown: asArray(summary.qtype_breakdown),
    top_qnames: asArray(summary.top_qnames),
    top_targets: asArray(summary.top_targets),
    worker_breakdown: asArray(summary.worker_breakdown),
    zone_breakdown: asArray(summary.zone_breakdown),
    route_breakdown: asArray(summary.route_breakdown),
    source_scope_breakdown: asArray(summary.source_scope_breakdown),
    trend_points: asArray(summary.trend_points),
    snapshot_consistency: normalizeDNSWorkerSnapshotConsistency(
      summary.snapshot_consistency,
    ),
    worker_health: normalizeDNSWorkerHealthSummary(summary.worker_health),
  };
}

function normalizeDNSWorkerSnapshotConsistency(
  consistency: DNSObservabilitySummary['snapshot_consistency'] | undefined,
) {
  if (!consistency) {
    return {
      status: 'unknown' as const,
      checked_at: '',
      snapshot_max_age_seconds: 0,
      total_worker_count: 0,
      online_worker_count: 0,
      stale_worker_count: 0,
      divergent_worker_count: 0,
      latest_snapshot_version: '',
      latest_snapshot_at: null,
      version_breakdown: [],
      workers: [],
    };
  }
  return {
    ...consistency,
    version_breakdown: asArray(consistency.version_breakdown).map((version) => ({
      ...version,
      workers: asArray(version.workers),
    })),
    workers: asArray(consistency.workers),
  };
}

function normalizeDNSWorkerHealthSummary(
  health: DNSObservabilitySummary['worker_health'] | undefined,
) {
  if (!health) {
    return {
      checked_at: '',
      total_worker_count: 0,
      online_worker_count: 0,
      probe_healthy_count: 0,
      probe_checked_count: 0,
      probe_healthy_percent: 0,
      node_probe_healthy_count: 0,
      node_probe_checked_count: 0,
      node_probe_stale_count: 0,
      node_probe_healthy_percent: 0,
      node_probe_average_rtt_ms: 0,
      node_probe_max_rtt_ms: 0,
      availability_percent: 0,
      average_latency_ms: 0,
      max_latency_ms: 0,
      error_rate_percent: 0,
      workers: [],
    };
  }
  return {
    ...health,
    workers: asArray(health.workers).map((worker) => ({
      ...worker,
      last_probe_results: asArray(worker.last_probe_results),
      probe_status: worker.probe_status || 'unknown',
      node_probes: asArray(worker.node_probes).map((probe) => ({
        ...probe,
        probe_status: probe.probe_status || 'unknown',
        results: asArray(probe.results),
      })),
    })),
  };
}
