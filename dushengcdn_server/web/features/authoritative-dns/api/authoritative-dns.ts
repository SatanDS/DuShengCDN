import { apiRequest } from '@/lib/api/client';

import type {
  DNSGSLBSimulationPayload,
  DNSGSLBSimulationResult,
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
  return apiRequest<DNSZoneItem[]>('/dns-zones/');
}

export function getDNSZone(id: number) {
  return apiRequest<DNSZoneItem>(`/dns-zones/${id}`);
}

export function createDNSZone(payload: DNSZoneMutationPayload) {
  return apiRequest<DNSZoneItem>('/dns-zones/', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateDNSZone(id: number, payload: DNSZoneMutationPayload) {
  return apiRequest<DNSZoneItem>(`/dns-zones/${id}/update`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function deleteDNSZone(id: number) {
  return apiRequest<void>(`/dns-zones/${id}/delete`, {
    method: 'POST',
  });
}

export function checkDNSZoneDelegation(id: number) {
  return apiRequest<DNSZoneDelegationCheck>(
    `/dns-zones/${id}/delegation-check`,
  );
}

export function getDNSZoneRecords(zoneId: number) {
  return apiRequest<DNSRecordItem[]>(`/dns-zones/${zoneId}/records`);
}

export function createDNSZoneRecord(
  zoneId: number,
  payload: DNSRecordMutationPayload,
) {
  return apiRequest<DNSRecordItem>(`/dns-zones/${zoneId}/records`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateDNSRecord(
  id: number,
  payload: DNSRecordMutationPayload,
) {
  return apiRequest<DNSRecordItem>(`/dns-records/${id}/update`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function deleteDNSRecord(id: number) {
  return apiRequest<void>(`/dns-records/${id}/delete`, {
    method: 'POST',
  });
}

export function getDNSWorkers() {
  return apiRequest<DNSWorkerItem[]>('/dns-workers/');
}

export function getDNSObservability(hours = 24) {
  const params = new URLSearchParams({ hours: String(hours) });
  return apiRequest<DNSObservabilitySummary>(
    `/dns-workers/observability?${params.toString()}`,
  );
}

export function createDNSWorker(payload: DNSWorkerMutationPayload) {
  return apiRequest<DNSWorkerItem>('/dns-workers/', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function deleteDNSWorker(id: number) {
  return apiRequest<void>(`/dns-workers/${id}/delete`, {
    method: 'POST',
  });
}

export function probeDNSWorker(id: number, payload: DNSWorkerProbePayload = {}) {
  return apiRequest<DNSWorkerProbe>(`/dns-workers/${id}/probe`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function simulateDNSGSLB(payload: DNSGSLBSimulationPayload) {
  return apiRequest<DNSGSLBSimulationResult>('/dns-workers/simulate', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}
