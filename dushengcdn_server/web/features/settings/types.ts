import type { AuthUser } from '@/types/auth';

export interface OptionItem {
  key: string;
  value: string;
}

export interface OptionBatchPayload {
  options: OptionItem[];
}

export type AuthSourceType = 'github' | 'oidc';

export interface AuthSource {
  id: number;
  name: string;
  type: AuthSourceType;
  display_name: string;
  is_active: boolean;
  client_id: string;
  client_secret?: string;
  client_secret_configured?: boolean;
  openid_discovery_url: string;
  scopes: string;
  icon_url: string;
}

export interface ExternalAccountBinding {
  id: number;
  auth_source_id: number;
  auth_source_name: string;
  auth_source_type: AuthSourceType;
  auth_source_label: string;
  external_username: string;
  email: string;
  created_at: string;
}

export type AuthSourcePayload = Omit<
  AuthSource,
  'id' | 'client_secret_configured'
> & {
  id?: number;
  client_secret: string;
};

export interface BootstrapTokenPayload {
  discovery_token: string;
}

export interface GeoIPLookupResult {
  provider: string;
  ip: string;
  iso_code: string;
  name: string;
  latitude?: number | null;
  longitude?: number | null;
}

export type DatabaseCleanupTarget =
  | 'node_access_logs'
  | 'node_metric_snapshots'
  | 'node_request_reports';

export interface DatabaseCleanupPayload {
  target: DatabaseCleanupTarget;
  retention_days?: number;
}

export interface DatabaseCleanupResult {
  target: DatabaseCleanupTarget;
  target_label: string;
  deleted_count: number;
  delete_all: boolean;
  retention_days?: number;
  cutoff?: string;
}

export type CommercialLicenseStatus =
  | 'community'
  | 'missing'
  | 'valid'
  | 'expiring'
  | 'expired'
  | 'invalid'
  | 'activation_required'
  | 'lease_expired';

export interface CommercialLicenseStatusPayload {
  status: CommercialLicenseStatus;
  status_label: string;
  licensed: boolean;
  required: boolean;
  license_id: string;
  customer_id: string;
  customer_name: string;
  plan: string;
  plan_label: string;
  fingerprint: string;
  features: string[];
  max_nodes: number;
  max_sites: number;
  current_nodes: number;
  current_sites: number;
  node_limit_exceeded: boolean;
  site_limit_exceeded: boolean;
  can_create_nodes: boolean;
  can_create_sites: boolean;
  issued_at?: string | null;
  expires_at?: string | null;
  days_until_expiry?: number | null;
  online_activation_required: boolean;
  activation_configured: boolean;
  activation_id: string;
  machine_fingerprint: string;
  lease_expires_at?: string | null;
  lease_renew_before_at?: string | null;
  last_lease_renewed_at?: string | null;
  lease_status: string;
  lease_status_label: string;
  lease_seconds_remaining: number;
  build_watermark: string;
  last_validated_at?: string | null;
  last_validation_error: string;
  signature_verified: boolean;
}

export interface CommercialLicenseIssuerStatusPayload {
  available: boolean;
  public_key: string;
  public_key_fingerprint: string;
  message: string;
}

export interface CommercialLicenseIssuePayload {
  license_id: string;
  customer_id: string;
  customer_name: string;
  plan: string;
  features: string[];
  max_nodes: number;
  max_sites: number;
  issued_at?: string;
  expires_at?: string;
}

export interface CommercialLicenseIssueResult {
  token: string;
  payload: {
    license_id: string;
    customer_id: string;
    customer_name: string;
    plan: string;
    features: string[];
    max_nodes: number;
    max_sites: number;
    issued_at?: string | null;
    expires_at?: string | null;
  };
  status: CommercialLicenseStatus;
  status_label: string;
  public_key: string;
  public_key_fingerprint: string;
  signature_verified: boolean;
}

export interface CommercialLicenseActivationRecord {
  id: number;
  activation_id: string;
  license_id: string;
  customer_id: string;
  machine_fingerprint: string;
  server_version: string;
  build_watermark: string;
  instance_hostname: string;
  revoked_at?: string | null;
  license_revoked_at?: string | null;
  license_revoke_reason: string;
  last_lease_issued_at?: string | null;
  last_lease_expires_at?: string | null;
  created_at: string;
  updated_at: string;
  lease_status: string;
}

export interface CommercialLicenseRevocationPayload {
  license_id: string;
  customer_id?: string;
  reason?: string;
}

export interface UpdateSelfPayload {
  username: string;
  display_name: string;
  password: string;
}

export type SettingsProfile = AuthUser;
