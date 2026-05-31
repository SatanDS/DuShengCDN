import { apiRequest } from '@/lib/api/client';

import type {
  DNSRecordItem,
  DNSRecordMutationPayload,
  DNSWorkerItem,
  DNSWorkerMutationPayload,
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
