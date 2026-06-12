'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

import { EmptyState } from '@/components/feedback/empty-state';
import { ErrorState } from '@/components/feedback/error-state';
import { LoadingState } from '@/components/feedback/loading-state';
import { useConfirmDialog } from '@/components/feedback/confirm-dialog-provider';
import { useToast } from '@/components/feedback/toast-provider';
import { AppModal } from '@/components/ui/app-modal';
import { useAuth } from '@/components/providers/auth-provider';
import { PageHeader } from '@/components/layout/page-header';
import { getOAuthAuthorizeUrl } from '@/features/auth/api/auth';
import { getPublicStatus } from '@/features/auth/api/public';
import {
  activateCommercialLicense,
  bindEmail,
  clearCommercialLicense,
  cleanupDatabaseObservability,
  deleteCommercialLicenseActivation,
  deleteExternalAccountBinding,
  generateAccessToken,
  getAuthSources,
  getCommercialLicenseActivations,
  getCommercialLicenseIssuerStatus,
  getBootstrapToken,
  getCommercialLicenseStatus,
  getDNSSourceDatabaseMirrorStatus,
  getExternalAccountBindings,
  getOptions,
  getSettingsProfile,
  installCommercialLicense,
  issueCommercialLicense,
  lookupGeoIP,
  refreshDNSSourceDatabaseMirror,
  restoreCommercialLicense,
  sendEmailBindVerification,
  revokeCommercialLicense,
  rotateBootstrapToken,
  updateOptions,
  updateSelf,
} from '@/features/settings/api/settings';
import { AuthSourceModal } from '@/features/settings/components/auth-source-modal';
import {
  CommercialLicenseSettingsSection,
  defaultCommercialLicenseIssueFields,
  type CommercialLicenseIssueForm,
} from '@/features/settings/components/settings-commercial-license-section';
import {
  SettingsDatabaseSection,
  type DatabaseSettingsFields,
} from '@/features/settings/components/settings-database-section';
import {
  SettingsOtherSection,
  type OtherSettingsFields,
} from '@/features/settings/components/settings-other-section';
import { SettingsPersonalSection } from '@/features/settings/components/settings-personal-section';
import {
  SettingsOperationSection,
  type DeploymentProtocol,
  type OperationSettingsFieldKey,
} from '@/features/settings/components/settings-operation-section';
import {
  SystemSettingsSection,
  type RateLimitOperationFieldKey,
  type SystemSettingsTextFieldKey,
} from '@/features/settings/components/settings-system-section';
import type {
  BootstrapTokenPayload,
  CommercialLicenseActivationRecord,
  CommercialLicenseIssuePayload,
  CommercialLicenseIssueResult,
  DatabaseCleanupResult,
  DatabaseCleanupTarget,
  GeoIPLookupResult,
  OptionItem,
  UpdateSelfPayload,
} from '@/features/settings/types';
import {
  DangerButton,
  ResourceField,
  ResourceInput,
  SecondaryButton,
} from '@/features/shared/components/resource-primitives';
import { ApiError } from '@/lib/api/client';
import { copyToClipboard } from '@/lib/utils/clipboard';
import { getErrorMessage } from '@/lib/utils/errors';
import { normalizeTrustedExternalUrl } from '@/lib/utils/redirect';
import { shellQuote } from '@/lib/utils/shell';

const settingsQueryKey = ['settings', 'options'] as const;
const authSourcesQueryKey = ['settings', 'auth-sources'] as const;
const externalAccountBindingsQueryKey = [
  'settings',
  'external-accounts',
] as const;
const commercialLicenseQueryKey = ['settings', 'commercial-license'] as const;
const commercialLicenseIssuerQueryKey = [
  'settings',
  'commercial-license-issuer',
] as const;
const commercialLicenseActivationsQueryKey = [
  'settings',
  'commercial-license-activations',
] as const;
const dnsSourceDatabaseMirrorStatusQueryKey = [
  'settings',
  'dns-source-database-mirror-status',
] as const;
const installerScriptUrl =
  'https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh';

const defaultSystemFields = {
  ServerAddress: '',
  PasswordLoginEnabled: true,
  PasswordRegisterEnabled: true,
  EmailVerificationEnabled: false,
  GitHubOAuthEnabled: false,
  WeChatAuthEnabled: false,
  TurnstileCheckEnabled: false,
  SMTPServer: '',
  SMTPPort: '587',
  SMTPAccount: '',
  SMTPToken: '',
  GitHubClientId: '',
  GitHubClientSecret: '',
  WeChatServerAddress: '',
  WeChatServerToken: '',
  WeChatAccountQRCodeImageURL: '',
  TurnstileSiteKey: '',
  TurnstileSecretKey: '',
};

const defaultOperationFields = {
  AgentHeartbeatInterval: '10000',
  AgentWebsocketUpgradeEnabled: true,
  AgentLegacyGlobalTokenEnabled: false,
  NodeOfflineThreshold: '120000',
  AgentUpdateRepo: 'SatanDS/SatanDS-DuShengCDN-releases',
  GeoIPProvider: 'ipinfo',
  OpenRestyWorkerProcesses: 'auto',
  OpenRestyWorkerConnections: '4096',
  OpenRestyWorkerRlimitNofile: '65535',
  OpenRestyEventsUse: 'epoll',
  OpenRestyEventsMultiAcceptEnabled: true,
  OpenRestyKeepaliveTimeout: '20',
  OpenRestyKeepaliveRequests: '1000',
  OpenRestyClientHeaderTimeout: '15',
  OpenRestyClientBodyTimeout: '15',
  OpenRestySendTimeout: '30',
  OpenRestyProxyConnectTimeout: '3',
  OpenRestyProxySendTimeout: '60',
  OpenRestyProxyReadTimeout: '60',
  OpenRestyProxyBufferingEnabled: true,
  OpenRestyProxyBuffers: '16 16k',
  OpenRestyProxyBufferSize: '8k',
  OpenRestyProxyBusyBuffersSize: '64k',
  OpenRestyGzipEnabled: true,
  OpenRestyGzipMinLength: '1024',
  OpenRestyGzipCompLevel: '5',
  OpenRestyCacheEnabled: false,
  OpenRestyCachePath: '',
  OpenRestyCacheLevels: '1:2',
  OpenRestyCacheInactive: '30m',
  OpenRestyCacheMaxSize: '1g',
  OpenRestyCacheKeyTemplate: '$scheme$host$request_uri',
  OpenRestyCacheLockEnabled: true,
  OpenRestyCacheLockTimeout: '5s',
  OpenRestyCacheUseStale:
    'error timeout updating http_500 http_502 http_503 http_504',
  AuthoritativeDNSDefaultTTL: '30',
  AuthoritativeDNSSnapshotMaxAge: '300',
  AuthoritativeDNSWorkerQueryRateLimit: '200',
  AuthoritativeDNSWorkerResponseRateLimit: '50',
  AuthoritativeDNSWorkerUDPResponseSize: '1232',
  AuthoritativeDNSWorkerECSEnabled: true,
  AuthoritativeDNSWorkerECSIPv4Prefix: '24',
  AuthoritativeDNSWorkerECSIPv6Prefix: '56',
  GSLBMetricFreshnessSeconds: '120',
  GSLBProbeSchedulingEnabled: false,
  GlobalApiRateLimitNum: '300',
  GlobalApiRateLimitDuration: '180',
  GlobalWebRateLimitNum: '300',
  GlobalWebRateLimitDuration: '180',
  DNSWorkerAPIRateLimitNum: '600',
  DNSWorkerAPIRateLimitDuration: '60',
  UploadRateLimitNum: '50',
  UploadRateLimitDuration: '60',
  DownloadRateLimitNum: '50',
  DownloadRateLimitDuration: '60',
  CriticalRateLimitNum: '100',
  CriticalRateLimitDuration: '1200',
  ServerAddress: '',
};

const defaultOtherFields: OtherSettingsFields = {
  Notice: '',
  SystemName: '',
  HomePageLink: '',
  About: '',
  Footer: '',
};

const defaultDatabaseFields: DatabaseSettingsFields = {
  DatabaseAutoCleanupEnabled: false,
  DatabaseAutoCleanupRetentionDays: '30',
};

type SystemFieldKey = keyof typeof defaultSystemFields;

const savedSecretSystemFieldKeys = new Set<SystemFieldKey>([
  'SMTPToken',
  'GitHubClientSecret',
  'WeChatServerToken',
  'TurnstileSecretKey',
]);

function mergeSyncedFields<TFields extends Record<string, string | boolean>>(
  current: TFields,
  next: TFields,
  baseline: TFields,
  forceSyncedKeys: ReadonlySet<string> = new Set(),
) {
  let merged = next;

  for (const key of Object.keys(next) as Array<keyof TFields>) {
    if (
      !forceSyncedKeys.has(String(key)) &&
      !Object.is(current[key], baseline[key])
    ) {
      if (merged === next) {
        merged = { ...next };
      }
      merged[key] = current[key];
    }
  }

  return merged;
}

const defaultProfileFields: UpdateSelfPayload = {
  username: '',
  display_name: '',
  password: '',
};

type CleanupModalState = {
  target: DatabaseCleanupTarget;
  label: string;
};

type SettingsTab =
  | 'personal'
  | 'operation'
  | 'license'
  | 'database'
  | 'system'
  | 'other';

const systemSettingsTabs = new Set<SettingsTab>([
  'operation',
  'license',
  'database',
  'system',
  'other',
]);

function isSettingsTab(value: string | null): value is SettingsTab {
  return (
    value === 'personal' ||
    value === 'operation' ||
    value === 'license' ||
    value === 'database' ||
    value === 'system' ||
    value === 'other'
  );
}

function normalizeSettingsTab(
  value: string | null,
  isRoot: boolean,
): SettingsTab {
  if (!isSettingsTab(value)) {
    return 'personal';
  }

  if (!isRoot && systemSettingsTabs.has(value)) {
    return 'personal';
  }

  return value;
}

function getDetailedErrorMessage(error: unknown) {
  const message = getErrorMessage(error);

  if (error instanceof ApiError) {
    return `HTTP ${error.status}：${message}`;
  }

  return message;
}

function formatActionError(action: string, error: unknown) {
  return `${action}失败：${getDetailedErrorMessage(error)}`;
}

function optionsToMap(options: OptionItem[] | undefined) {
  return (options ?? []).reduce<Record<string, string>>(
    (accumulator, option) => {
      accumulator[option.key] = option.value;
      return accumulator;
    },
    {},
  );
}

function toBoolean(value: string | undefined, fallback: boolean) {
  if (value === undefined) {
    return fallback;
  }

  return value === 'true';
}

function normalizeServerUrl(value: string) {
  return value.trim().replace(/\/+$/, '');
}

function getDeploymentProtocol(value: string): DeploymentProtocol {
  return value.trim().toLowerCase().startsWith('http://') ? 'http' : 'https';
}

function stripServerUrlProtocol(value: string) {
  return normalizeServerUrl(value).replace(/^https?:\/\//i, '');
}

function buildDeploymentServerUrl(protocol: DeploymentProtocol, value: string) {
  const endpoint = stripServerUrlProtocol(value);
  return endpoint ? `${protocol}://${endpoint}` : '';
}

function getBrowserOrigin() {
  if (typeof window === 'undefined') {
    return '';
  }

  return normalizeServerUrl(window.location.origin);
}

function formatDurationLabel(value: string) {
  const milliseconds = Number.parseInt(value, 10);
  if (Number.isNaN(milliseconds)) {
    return value;
  }

  if (milliseconds >= 60000) {
    return `${milliseconds / 60000} 分钟`;
  }

  return `${milliseconds / 1000} 秒`;
}

function formatSecondsLabel(value: string) {
  const seconds = Number.parseInt(value, 10);
  if (Number.isNaN(seconds)) {
    return value;
  }

  if (seconds >= 3600 && seconds % 3600 === 0) {
    return `${seconds / 3600} 小时`;
  }

  if (seconds >= 60 && seconds % 60 === 0) {
    return `${seconds / 60} 分钟`;
  }

  return `${seconds} 秒`;
}

function parseLicenseLimitInput(value: string) {
  const limit = Number.parseInt(value.trim(), 10);
  return Number.isFinite(limit) && limit > 0 ? limit : 0;
}

function buildDiscoveryCommand(serverUrl: string, discoveryToken: string) {
  void discoveryToken;
  const quotedServerUrl = shellQuote(normalizeServerUrl(serverUrl));
  return [
    `token_file="$(mktemp)"`,
    `chmod 600 "$token_file"`,
    `trap 'stty echo 2>/dev/null || true; rm -f "$token_file"' EXIT`,
    `printf 'Discovery token: ' >&2`,
    `stty -echo 2>/dev/null || true`,
    `IFS= read -r discovery_token`,
    `stty echo 2>/dev/null || true`,
    `printf '\\n' >&2`,
    `printf '%s\\n' "$discovery_token" > "$token_file"`,
    `unset discovery_token`,
    `curl -fsSL ${installerScriptUrl} | bash -s -- \\`,
    `  --server-url ${quotedServerUrl} \\`,
    `  --discovery-token-file "$token_file"`,
    `rm -f "$token_file"`,
    `trap - EXIT`,
  ].join('\n');
}

export function SettingsPage() {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { refreshUser, user } = useAuth();
  const { showToast, dismissToast } = useToast();
  const confirmDialog = useConfirmDialog();
  const [activeTab, setActiveTab] = useState<SettingsTab>('personal');
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [profileFields, setProfileFields] = useState(defaultProfileFields);
  const [systemFields, setSystemFields] = useState(defaultSystemFields);
  const [operationFields, setOperationFields] = useState(
    defaultOperationFields,
  );
  const [deploymentProtocol, setDeploymentProtocol] =
    useState<DeploymentProtocol>('https');
  const [otherFields, setOtherFields] = useState(defaultOtherFields);
  const [databaseFields, setDatabaseFields] = useState(defaultDatabaseFields);
  const [accessToken, setAccessToken] = useState('');
  const [emailAddress, setEmailAddress] = useState('');
  const [emailCode, setEmailCode] = useState('');
  const [emailTurnstileToken, setEmailTurnstileToken] = useState('');
  const [authSourceModalOpen, setAuthSourceModalOpen] = useState(false);
  const [geoIPTestIP, setGeoIPTestIP] = useState('8.8.8.8');
  const [cleanupModalState, setCleanupModalState] =
    useState<CleanupModalState | null>(null);
  const [cleanupRetentionDays, setCleanupRetentionDays] = useState('');
  const [commercialLicenseToken, setCommercialLicenseToken] = useState('');
  const [commercialLicenseIssueFields, setCommercialLicenseIssueFields] =
    useState(defaultCommercialLicenseIssueFields);
  const [issuedCommercialLicense, setIssuedCommercialLicense] =
    useState<CommercialLicenseIssueResult | null>(null);
  const systemFieldsBaselineRef = useRef(defaultSystemFields);
  const operationFieldsBaselineRef = useRef(defaultOperationFields);
  const otherFieldsBaselineRef = useRef(defaultOtherFields);
  const databaseFieldsBaselineRef = useRef(defaultDatabaseFields);
  const deploymentProtocolBaselineRef = useRef<DeploymentProtocol>('https');
  const syncedOptionKeysRef = useRef<Set<string>>(new Set());

  const isRoot = (user?.role ?? 0) >= 100;

  const setFeedback = showToast;

  useEffect(() => {
    setActiveTab(normalizeSettingsTab(searchParams.get('tab'), isRoot));
  }, [isRoot, searchParams]);

  const handleTabChange = (tab: SettingsTab) => {
    setActiveTab(tab);

    const nextParams = new URLSearchParams(searchParams.toString());
    if (tab === 'personal') {
      nextParams.delete('tab');
    } else {
      nextParams.set('tab', tab);
    }

    const nextSearch = nextParams.toString();
    router.replace(
      `${pathname ?? '/setting'}${nextSearch ? `?${nextSearch}` : ''}`,
    );
  };

  const handleCopy = async (value: string, message: string) => {
    try {
      await copyToClipboard(value);
      setFeedback({ tone: 'success', message });
    } catch (error) {
      setFeedback({ tone: 'danger', message: getErrorMessage(error) });
    }
  };

  const publicStatusQuery = useQuery({
    queryKey: ['public-status'],
    queryFn: getPublicStatus,
  });

  const profileQuery = useQuery({
    queryKey: ['settings', 'profile'],
    queryFn: getSettingsProfile,
  });

  const externalAccountsQuery = useQuery({
    queryKey: externalAccountBindingsQueryKey,
    queryFn: getExternalAccountBindings,
  });

  const optionsQuery = useQuery({
    queryKey: settingsQueryKey,
    queryFn: getOptions,
    enabled: isRoot,
  });

  const authSourcesQuery = useQuery({
    queryKey: authSourcesQueryKey,
    queryFn: getAuthSources,
    enabled: isRoot,
  });

  const bootstrapQuery = useQuery({
    queryKey: ['settings', 'bootstrap-token'],
    queryFn: getBootstrapToken,
    enabled: isRoot,
  });

  const commercialLicenseQuery = useQuery({
    queryKey: commercialLicenseQueryKey,
    queryFn: getCommercialLicenseStatus,
    enabled: isRoot,
  });

  const commercialLicenseIssuerQuery = useQuery({
    queryKey: commercialLicenseIssuerQueryKey,
    queryFn: getCommercialLicenseIssuerStatus,
    enabled: isRoot,
  });

  const commercialLicenseActivationsQuery = useQuery({
    queryKey: commercialLicenseActivationsQueryKey,
    queryFn: getCommercialLicenseActivations,
    enabled: isRoot && activeTab === 'license',
  });

  const dnsSourceDatabaseMirrorStatusQuery = useQuery({
    queryKey: dnsSourceDatabaseMirrorStatusQueryKey,
    queryFn: getDNSSourceDatabaseMirrorStatus,
    enabled: isRoot && activeTab === 'database',
  });

  useEffect(() => {
    if (profileQuery.data) {
      setProfileFields({
        username: profileQuery.data.username,
        display_name: profileQuery.data.display_name || '',
        password: '',
      });
      setEmailAddress(profileQuery.data.email || '');
    }
  }, [profileQuery.data]);

  useEffect(() => {
    const publicStatus = publicStatusQuery.data;
    if (!publicStatus || optionsQuery.data) {
      return;
    }

    const resolvedServerAddress =
      publicStatus.server_address || getBrowserOrigin();

    setSystemFields((previous) => ({
      ...previous,
      ServerAddress: resolvedServerAddress || previous.ServerAddress,
      GitHubClientId: publicStatus.github_client_id || previous.GitHubClientId,
      WeChatAccountQRCodeImageURL:
        publicStatus.wechat_qrcode || previous.WeChatAccountQRCodeImageURL,
      TurnstileSiteKey:
        publicStatus.turnstile_site_key || previous.TurnstileSiteKey,
    }));
    setOtherFields((previous) => ({
      ...previous,
      SystemName: publicStatus.system_name || previous.SystemName,
      HomePageLink: publicStatus.home_page_link || previous.HomePageLink,
      Footer: publicStatus.footer_html || previous.Footer,
    }));
    setOperationFields((previous) => ({
      ...previous,
      ServerAddress: resolvedServerAddress
        ? stripServerUrlProtocol(resolvedServerAddress)
        : previous.ServerAddress,
    }));
    if (resolvedServerAddress) {
      const nextDeploymentProtocol = getDeploymentProtocol(
        resolvedServerAddress,
      );
      if (deploymentProtocol === deploymentProtocolBaselineRef.current) {
        setDeploymentProtocol(nextDeploymentProtocol);
      }
      deploymentProtocolBaselineRef.current = nextDeploymentProtocol;
    }
  }, [deploymentProtocol, optionsQuery.data, publicStatusQuery.data]);

  useEffect(() => {
    if (!optionsQuery.data) {
      return;
    }

    const optionMap = optionsToMap(optionsQuery.data);
    const resolvedServerAddress =
      optionMap.ServerAddress ||
      publicStatusQuery.data?.server_address ||
      getBrowserOrigin();

    const nextSystemFields = {
      ServerAddress: resolvedServerAddress,
      PasswordLoginEnabled: toBoolean(optionMap.PasswordLoginEnabled, true),
      PasswordRegisterEnabled: toBoolean(
        optionMap.PasswordRegisterEnabled,
        true,
      ),
      EmailVerificationEnabled: toBoolean(
        optionMap.EmailVerificationEnabled,
        false,
      ),
      GitHubOAuthEnabled: toBoolean(optionMap.GitHubOAuthEnabled, false),
      WeChatAuthEnabled: toBoolean(optionMap.WeChatAuthEnabled, false),
      TurnstileCheckEnabled: toBoolean(optionMap.TurnstileCheckEnabled, false),
      SMTPServer: optionMap.SMTPServer ?? '',
      SMTPPort: optionMap.SMTPPort ?? '587',
      SMTPAccount: optionMap.SMTPAccount ?? '',
      SMTPToken: '',
      GitHubClientId: optionMap.GitHubClientId ?? '',
      GitHubClientSecret: '',
      WeChatServerAddress: optionMap.WeChatServerAddress ?? '',
      WeChatServerToken: '',
      WeChatAccountQRCodeImageURL: optionMap.WeChatAccountQRCodeImageURL ?? '',
      TurnstileSiteKey: optionMap.TurnstileSiteKey ?? '',
      TurnstileSecretKey: '',
    };

    setDeploymentProtocol(getDeploymentProtocol(resolvedServerAddress));
    const nextOperationFields = {
      ServerAddress: stripServerUrlProtocol(resolvedServerAddress),
      AgentHeartbeatInterval: optionMap.AgentHeartbeatInterval ?? '10000',
      AgentWebsocketUpgradeEnabled: toBoolean(
        optionMap.AgentWebsocketUpgradeEnabled,
        true,
      ),
      AgentLegacyGlobalTokenEnabled: toBoolean(
        optionMap.AgentLegacyGlobalTokenEnabled,
        false,
      ),
      NodeOfflineThreshold: optionMap.NodeOfflineThreshold ?? '120000',
      AgentUpdateRepo:
        optionMap.AgentUpdateRepo ?? 'SatanDS/SatanDS-DuShengCDN-releases',
      GeoIPProvider: optionMap.GeoIPProvider ?? 'ipinfo',
      OpenRestyWorkerProcesses: optionMap.OpenRestyWorkerProcesses ?? 'auto',
      OpenRestyWorkerConnections:
        optionMap.OpenRestyWorkerConnections ?? '4096',
      OpenRestyWorkerRlimitNofile:
        optionMap.OpenRestyWorkerRlimitNofile ?? '65535',
      OpenRestyEventsUse: optionMap.OpenRestyEventsUse ?? 'epoll',
      OpenRestyEventsMultiAcceptEnabled: toBoolean(
        optionMap.OpenRestyEventsMultiAcceptEnabled,
        true,
      ),
      OpenRestyKeepaliveTimeout: optionMap.OpenRestyKeepaliveTimeout ?? '20',
      OpenRestyKeepaliveRequests:
        optionMap.OpenRestyKeepaliveRequests ?? '1000',
      OpenRestyClientHeaderTimeout:
        optionMap.OpenRestyClientHeaderTimeout ?? '15',
      OpenRestyClientBodyTimeout: optionMap.OpenRestyClientBodyTimeout ?? '15',
      OpenRestySendTimeout: optionMap.OpenRestySendTimeout ?? '30',
      OpenRestyProxyConnectTimeout:
        optionMap.OpenRestyProxyConnectTimeout ?? '3',
      OpenRestyProxySendTimeout: optionMap.OpenRestyProxySendTimeout ?? '60',
      OpenRestyProxyReadTimeout: optionMap.OpenRestyProxyReadTimeout ?? '60',
      OpenRestyProxyBufferingEnabled: toBoolean(
        optionMap.OpenRestyProxyBufferingEnabled,
        true,
      ),
      OpenRestyProxyBuffers: optionMap.OpenRestyProxyBuffers ?? '16 16k',
      OpenRestyProxyBufferSize: optionMap.OpenRestyProxyBufferSize ?? '8k',
      OpenRestyProxyBusyBuffersSize:
        optionMap.OpenRestyProxyBusyBuffersSize ?? '64k',
      OpenRestyGzipEnabled: toBoolean(optionMap.OpenRestyGzipEnabled, true),
      OpenRestyGzipMinLength: optionMap.OpenRestyGzipMinLength ?? '1024',
      OpenRestyGzipCompLevel: optionMap.OpenRestyGzipCompLevel ?? '5',
      OpenRestyCacheEnabled: toBoolean(optionMap.OpenRestyCacheEnabled, false),
      OpenRestyCachePath: optionMap.OpenRestyCachePath ?? '',
      OpenRestyCacheLevels: optionMap.OpenRestyCacheLevels ?? '1:2',
      OpenRestyCacheInactive: optionMap.OpenRestyCacheInactive ?? '30m',
      OpenRestyCacheMaxSize: optionMap.OpenRestyCacheMaxSize ?? '1g',
      OpenRestyCacheKeyTemplate:
        optionMap.OpenRestyCacheKeyTemplate ?? '$scheme$host$request_uri',
      OpenRestyCacheLockEnabled: toBoolean(
        optionMap.OpenRestyCacheLockEnabled,
        true,
      ),
      OpenRestyCacheLockTimeout: optionMap.OpenRestyCacheLockTimeout ?? '5s',
      OpenRestyCacheUseStale:
        optionMap.OpenRestyCacheUseStale ??
        'error timeout updating http_500 http_502 http_503 http_504',
      AuthoritativeDNSDefaultTTL: optionMap.AuthoritativeDNSDefaultTTL ?? '30',
      AuthoritativeDNSSnapshotMaxAge:
        optionMap.AuthoritativeDNSSnapshotMaxAge ?? '300',
      AuthoritativeDNSWorkerQueryRateLimit:
        optionMap.AuthoritativeDNSWorkerQueryRateLimit ?? '200',
      AuthoritativeDNSWorkerResponseRateLimit:
        optionMap.AuthoritativeDNSWorkerResponseRateLimit ?? '50',
      AuthoritativeDNSWorkerUDPResponseSize:
        optionMap.AuthoritativeDNSWorkerUDPResponseSize ?? '1232',
      AuthoritativeDNSWorkerECSEnabled: toBoolean(
        optionMap.AuthoritativeDNSWorkerECSEnabled,
        true,
      ),
      AuthoritativeDNSWorkerECSIPv4Prefix:
        optionMap.AuthoritativeDNSWorkerECSIPv4Prefix ?? '24',
      AuthoritativeDNSWorkerECSIPv6Prefix:
        optionMap.AuthoritativeDNSWorkerECSIPv6Prefix ?? '56',
      GSLBMetricFreshnessSeconds: optionMap.GSLBMetricFreshnessSeconds ?? '120',
      GSLBProbeSchedulingEnabled: toBoolean(
        optionMap.GSLBProbeSchedulingEnabled,
        false,
      ),
      GlobalApiRateLimitNum: optionMap.GlobalApiRateLimitNum ?? '300',
      GlobalApiRateLimitDuration: optionMap.GlobalApiRateLimitDuration ?? '180',
      GlobalWebRateLimitNum: optionMap.GlobalWebRateLimitNum ?? '300',
      GlobalWebRateLimitDuration: optionMap.GlobalWebRateLimitDuration ?? '180',
      DNSWorkerAPIRateLimitNum: optionMap.DNSWorkerAPIRateLimitNum ?? '600',
      DNSWorkerAPIRateLimitDuration:
        optionMap.DNSWorkerAPIRateLimitDuration ?? '60',
      UploadRateLimitNum: optionMap.UploadRateLimitNum ?? '50',
      UploadRateLimitDuration: optionMap.UploadRateLimitDuration ?? '60',
      DownloadRateLimitNum: optionMap.DownloadRateLimitNum ?? '50',
      DownloadRateLimitDuration: optionMap.DownloadRateLimitDuration ?? '60',
      CriticalRateLimitNum: optionMap.CriticalRateLimitNum ?? '100',
      CriticalRateLimitDuration: optionMap.CriticalRateLimitDuration ?? '1200',
    };

    const nextOtherFields = {
      Notice: optionMap.Notice ?? '',
      SystemName: optionMap.SystemName ?? '',
      HomePageLink: optionMap.HomePageLink ?? '',
      About: optionMap.About ?? '',
      Footer: optionMap.Footer ?? '',
    };
    const nextDatabaseFields = {
      DatabaseAutoCleanupEnabled: toBoolean(
        optionMap.DatabaseAutoCleanupEnabled,
        false,
      ),
      DatabaseAutoCleanupRetentionDays:
        optionMap.DatabaseAutoCleanupRetentionDays ?? '30',
    };

    const forceSyncedKeys = syncedOptionKeysRef.current;
    const forceSyncedOperationKeys = new Set(forceSyncedKeys);
    if (forceSyncedKeys.has('ServerAddress')) {
      forceSyncedOperationKeys.add('ServerAddress');
    }
    const nextDeploymentProtocol = getDeploymentProtocol(resolvedServerAddress);
    const shouldForceServerAddress = forceSyncedKeys.has('ServerAddress');
    const hasUnsavedServerAddressChange =
      operationFields.ServerAddress !==
        operationFieldsBaselineRef.current.ServerAddress ||
      systemFields.ServerAddress !==
        systemFieldsBaselineRef.current.ServerAddress ||
      deploymentProtocol !== deploymentProtocolBaselineRef.current;

    setSystemFields((previous) =>
      mergeSyncedFields(
        previous,
        nextSystemFields,
        systemFieldsBaselineRef.current,
        forceSyncedKeys,
      ),
    );
    setOperationFields((previous) =>
      mergeSyncedFields(
        previous,
        nextOperationFields,
        operationFieldsBaselineRef.current,
        forceSyncedOperationKeys,
      ),
    );
    setOtherFields((previous) =>
      mergeSyncedFields(
        previous,
        nextOtherFields,
        otherFieldsBaselineRef.current,
        forceSyncedKeys,
      ),
    );
    setDatabaseFields((previous) =>
      mergeSyncedFields(
        previous,
        nextDatabaseFields,
        databaseFieldsBaselineRef.current,
        forceSyncedKeys,
      ),
    );

    systemFieldsBaselineRef.current = nextSystemFields;
    operationFieldsBaselineRef.current = nextOperationFields;
    otherFieldsBaselineRef.current = nextOtherFields;
    databaseFieldsBaselineRef.current = nextDatabaseFields;
    if (!hasUnsavedServerAddressChange || shouldForceServerAddress) {
      setDeploymentProtocol(nextDeploymentProtocol);
    }
    deploymentProtocolBaselineRef.current = nextDeploymentProtocol;

    syncedOptionKeysRef.current = new Set(
      Array.from(forceSyncedKeys).filter((key) =>
        savedSecretSystemFieldKeys.has(key as SystemFieldKey),
      ),
    );
    if (syncedOptionKeysRef.current.size > 0) {
      setSystemFields((previous) => {
        let nextFields = previous;
        for (const key of syncedOptionKeysRef.current) {
          if (
            savedSecretSystemFieldKeys.has(key as SystemFieldKey) &&
            previous[key as SystemFieldKey]
          ) {
            nextFields = { ...nextFields, [key]: '' };
          }
        }
        return nextFields;
      });
      systemFieldsBaselineRef.current = {
        ...systemFieldsBaselineRef.current,
        ...Object.fromEntries(
          Array.from(syncedOptionKeysRef.current)
            .filter((key) =>
              savedSecretSystemFieldKeys.has(key as SystemFieldKey),
            )
            .map((key) => [key, '']),
        ),
      };
      syncedOptionKeysRef.current = new Set();
    }
  }, [optionsQuery.data, publicStatusQuery.data?.server_address]);

  useEffect(() => {
    if (!publicStatusQuery.data || optionsQuery.data) {
      return;
    }

    systemFieldsBaselineRef.current = {
      ...systemFieldsBaselineRef.current,
      ServerAddress: systemFields.ServerAddress,
      GitHubClientId: systemFields.GitHubClientId,
      WeChatAccountQRCodeImageURL: systemFields.WeChatAccountQRCodeImageURL,
      TurnstileSiteKey: systemFields.TurnstileSiteKey,
    };
    operationFieldsBaselineRef.current = {
      ...operationFieldsBaselineRef.current,
      ServerAddress: operationFields.ServerAddress,
    };
    otherFieldsBaselineRef.current = {
      ...otherFieldsBaselineRef.current,
      SystemName: otherFields.SystemName,
      HomePageLink: otherFields.HomePageLink,
      Footer: otherFields.Footer,
    };
  }, [
    operationFields.ServerAddress,
    optionsQuery.data,
    otherFields.Footer,
    otherFields.HomePageLink,
    otherFields.SystemName,
    publicStatusQuery.data,
    systemFields.GitHubClientId,
    systemFields.ServerAddress,
    systemFields.TurnstileSiteKey,
    systemFields.WeChatAccountQRCodeImageURL,
  ]);

  const rotateTokenMutation = useMutation({
    mutationFn: rotateBootstrapToken,
    onSuccess: async (data: BootstrapTokenPayload) => {
      setFeedback({ tone: 'success', message: '接入令牌已重新生成。' });
      await queryClient.invalidateQueries({
        queryKey: ['settings', 'bootstrap-token'],
      });
      if (data.discovery_token) {
        try {
          await copyToClipboard(data.discovery_token);
        } catch (error) {
          setFeedback({
            tone: 'success',
            message: '接入令牌已重新生成，但未能自动复制。',
            detail: getErrorMessage(error),
          });
        }
      }
    },
    onError: (error) => {
      setFeedback({
        tone: 'danger',
        message: formatActionError('重新生成接入令牌', error),
      });
    },
  });

  const accessTokenMutation = useMutation({
    mutationFn: generateAccessToken,
    onSuccess: (token) => {
      setAccessToken(token);
      setFeedback({
        tone: 'success',
        message: '访问令牌已重置，并已在当前页面展示。',
      });
    },
    onError: (error) => {
      setFeedback({
        tone: 'danger',
        message: formatActionError('重置访问令牌', error),
      });
    },
  });

  const geoIPLookupMutation = useMutation({
    mutationFn: ({ provider, ip }: { provider: string; ip: string }) =>
      lookupGeoIP(provider, ip),
  });

  const databaseCleanupMutation = useMutation({
    mutationFn: cleanupDatabaseObservability,
    onSuccess: (result: DatabaseCleanupResult) => {
      setCleanupModalState(null);
      setCleanupRetentionDays('');
      const targetName = result.target_label || result.target;
      const emptyHint =
        result.deleted_count === 0
          ? '没有符合条件的数据；如果要清空，请把保留天数留空后再执行。'
          : '';
      setFeedback({
        tone: 'success',
        message: result.delete_all
          ? `已清空${targetName}数据，共删除 ${result.deleted_count} 条。${emptyHint}`
          : `已清理${targetName}中超出保留期的数据，共删除 ${result.deleted_count} 条。${emptyHint}`,
      });
    },
    onError: (error) => {
      setFeedback({
        tone: 'danger',
        message: formatActionError('清理数据库观测数据', error),
      });
    },
  });

  const discoveryToken = bootstrapQuery.data?.discovery_token ?? '';
  const deploymentServerUrl = buildDeploymentServerUrl(
    deploymentProtocol,
    operationFields.ServerAddress,
  );
  const discoveryCommand =
    isRoot && deploymentServerUrl && discoveryToken
      ? buildDiscoveryCommand(deploymentServerUrl, discoveryToken)
      : '';

  const tabs = useMemo(
    () => [
      {
        key: 'personal' as const,
        label: '个人设置',
        description: '更新个人资料、绑定账号与访问令牌。',
      },
      ...(isRoot
        ? [
            {
              key: 'operation' as const,
              label: '运维设置',
              description: '节点程序参数、接入令牌与部署命令。',
            },
            {
              key: 'license' as const,
              label: '商业授权',
              description: '查看授权状态、有效期和资源额度。',
            },
            {
              key: 'system' as const,
              label: '系统设置',
              description: '登录注册、SMTP、认证源、限流与风控开关。',
            },
            {
              key: 'database' as const,
              label: '数据库',
              description: '观测数据清理与每日自动保留策略。',
            },
            {
              key: 'other' as const,
              label: '其他设置',
              description: '公告、关于与品牌信息。',
            },
          ]
        : []),
    ],
    [isRoot],
  );

  const runBusyAction = async (
    key: string,
    action: () => Promise<void>,
    errorContext = '执行操作',
  ) => {
    setBusyKey(key);
    dismissToast();

    try {
      await action();
    } catch (error) {
      setFeedback({
        tone: 'danger',
        message: formatActionError(errorContext, error),
      });
    } finally {
      setBusyKey(null);
    }
  };

  const saveOptionEntries = async (
    entries: Array<[string, string]>,
    successMessage: string,
  ) => {
    syncedOptionKeysRef.current = new Set(entries.map(([key]) => key));
    await updateOptions(entries.map(([key, value]) => ({ key, value })));

    await queryClient.invalidateQueries({ queryKey: settingsQueryKey });
    await queryClient.invalidateQueries({ queryKey: ['public-status'] });
    setFeedback({ tone: 'success', message: successMessage });
  };

  const handleGeoIPLookup = () => {
    geoIPLookupMutation.reset();
    geoIPLookupMutation.mutate({
      provider: operationFields.GeoIPProvider,
      ip: geoIPTestIP.trim(),
    });
  };

  const geoIPLookupResult: GeoIPLookupResult | undefined =
    geoIPLookupMutation.data;

  const updateOperationField = (
    key: OperationSettingsFieldKey,
    value: string | boolean,
  ) => {
    setOperationFields((previous) => ({ ...previous, [key]: value }));
  };

  const handleOperationServerAddressChange = (value: string) => {
    setDeploymentProtocol(getDeploymentProtocol(value));
    setOperationFields((previous) => ({
      ...previous,
      ServerAddress: stripServerUrlProtocol(value),
    }));
  };

  const handleSaveOperationSettings = () =>
    void runBusyAction(
      'operation-settings',
      async () => {
        const heartbeat = Number.parseInt(
          operationFields.AgentHeartbeatInterval,
          10,
        );
        const offline = Number.parseInt(
          operationFields.NodeOfflineThreshold,
          10,
        );

        if (Number.isNaN(heartbeat) || heartbeat < 5000) {
          throw new Error('心跳间隔不能小于 5000 毫秒。');
        }
        if (Number.isNaN(offline) || offline < 10000) {
          throw new Error('离线阈值不能小于 10000 毫秒。');
        }

        await saveOptionEntries(
          [
            ['AgentHeartbeatInterval', String(heartbeat)],
            [
              'AgentWebsocketUpgradeEnabled',
              String(operationFields.AgentWebsocketUpgradeEnabled),
            ],
            [
              'AgentLegacyGlobalTokenEnabled',
              String(operationFields.AgentLegacyGlobalTokenEnabled),
            ],
            ['NodeOfflineThreshold', String(offline)],
            ['AgentUpdateRepo', operationFields.AgentUpdateRepo.trim()],
            ['GeoIPProvider', operationFields.GeoIPProvider],
          ],
          '运维设置已保存。',
        );
      },
      '保存运维设置',
    );

  const handleSaveAuthoritativeDnsSettings = () =>
    void runBusyAction(
      'operation-authoritative-dns',
      async () => {
        const defaultTtl = Number.parseInt(
          operationFields.AuthoritativeDNSDefaultTTL,
          10,
        );
        const snapshotMaxAge = Number.parseInt(
          operationFields.AuthoritativeDNSSnapshotMaxAge,
          10,
        );
        const metricFreshness = Number.parseInt(
          operationFields.GSLBMetricFreshnessSeconds,
          10,
        );
        const queryRateLimit = Number.parseInt(
          operationFields.AuthoritativeDNSWorkerQueryRateLimit,
          10,
        );
        const responseRateLimit = Number.parseInt(
          operationFields.AuthoritativeDNSWorkerResponseRateLimit,
          10,
        );
        const udpResponseSize = Number.parseInt(
          operationFields.AuthoritativeDNSWorkerUDPResponseSize,
          10,
        );
        const ecsIPv4Prefix = Number.parseInt(
          operationFields.AuthoritativeDNSWorkerECSIPv4Prefix,
          10,
        );
        const ecsIPv6Prefix = Number.parseInt(
          operationFields.AuthoritativeDNSWorkerECSIPv6Prefix,
          10,
        );

        if (Number.isNaN(defaultTtl) || defaultTtl <= 0 || defaultTtl > 86400) {
          throw new Error('默认缓存时间必须为 1 到 86400 之间的整数秒。');
        }
        if (Number.isNaN(snapshotMaxAge) || snapshotMaxAge <= 0) {
          throw new Error('解析配置有效期必须为大于 0 的整数秒。');
        }
        if (Number.isNaN(metricFreshness) || metricFreshness <= 0) {
          throw new Error('节点状态数据有效时间必须为大于 0 的整数秒。');
        }
        if (Number.isNaN(queryRateLimit) || queryRateLimit < 0) {
          throw new Error(
            '普通查询限流必须为大于或等于 0 的整数，0 表示关闭。',
          );
        }
        if (Number.isNaN(responseRateLimit) || responseRateLimit < 0) {
          throw new Error(
            '异常响应 RRL 必须为大于或等于 0 的整数，0 表示关闭。',
          );
        }
        if (
          Number.isNaN(udpResponseSize) ||
          udpResponseSize < 512 ||
          udpResponseSize > 65535
        ) {
          throw new Error('UDP 响应大小必须为 512 到 65535 之间的整数。');
        }
        if (
          Number.isNaN(ecsIPv4Prefix) ||
          ecsIPv4Prefix < 0 ||
          ecsIPv4Prefix > 32
        ) {
          throw new Error('ECS IPv4 前缀必须为 0 到 32。');
        }
        if (
          Number.isNaN(ecsIPv6Prefix) ||
          ecsIPv6Prefix < 0 ||
          ecsIPv6Prefix > 128
        ) {
          throw new Error('ECS IPv6 前缀必须为 0 到 128。');
        }

        await saveOptionEntries(
          [
            ['AuthoritativeDNSDefaultTTL', String(defaultTtl)],
            ['AuthoritativeDNSSnapshotMaxAge', String(snapshotMaxAge)],
            ['GSLBMetricFreshnessSeconds', String(metricFreshness)],
            ['AuthoritativeDNSWorkerQueryRateLimit', String(queryRateLimit)],
            [
              'AuthoritativeDNSWorkerResponseRateLimit',
              String(responseRateLimit),
            ],
            ['AuthoritativeDNSWorkerUDPResponseSize', String(udpResponseSize)],
            [
              'AuthoritativeDNSWorkerECSEnabled',
              String(operationFields.AuthoritativeDNSWorkerECSEnabled),
            ],
            ['AuthoritativeDNSWorkerECSIPv4Prefix', String(ecsIPv4Prefix)],
            ['AuthoritativeDNSWorkerECSIPv6Prefix', String(ecsIPv6Prefix)],
            [
              'GSLBProbeSchedulingEnabled',
              String(operationFields.GSLBProbeSchedulingEnabled),
            ],
          ],
          '本地解析参数已保存。',
        );
      },
      '保存本地解析参数',
    );

  const handleProfileSave = () => {
    void runBusyAction(
      'profile',
      async () => {
        await updateSelf({
          username: profileFields.username.trim(),
          display_name: profileFields.display_name.trim(),
          password: profileFields.password,
        });
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['settings', 'profile'] }),
          refreshUser(),
        ]);
        setProfileFields((previous) => ({ ...previous, password: '' }));
        setFeedback({ tone: 'success', message: '个人资料已更新。' });
      },
      '保存个人资料',
    );
  };

  const handleEmailVerification = () => {
    if (!emailAddress.trim()) {
      setFeedback({ tone: 'danger', message: '请输入要绑定的邮箱地址。' });
      return;
    }

    if (publicStatusQuery.data?.turnstile_check && !emailTurnstileToken) {
      setFeedback({ tone: 'info', message: '请先完成人机验证。' });
      return;
    }

    void runBusyAction(
      'email-send',
      async () => {
        await sendEmailBindVerification(
          emailAddress.trim(),
          emailTurnstileToken || undefined,
        );
        setFeedback({ tone: 'success', message: '验证码已发送，请检查邮箱。' });
      },
      '发送邮箱验证码',
    );
  };

  const handleBindEmail = () => {
    if (!emailAddress.trim() || !emailCode.trim()) {
      setFeedback({ tone: 'danger', message: '请输入邮箱地址和验证码。' });
      return;
    }

    void runBusyAction(
      'email-bind',
      async () => {
        await bindEmail(emailAddress.trim(), emailCode.trim());
        setEmailCode('');
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['settings', 'profile'] }),
          refreshUser(),
        ]);
        setFeedback({ tone: 'success', message: '邮箱已绑定。' });
      },
      '绑定邮箱',
    );
  };

  const handleBindAuthSource = (sourceName: string) => {
    void runBusyAction(
      `auth-source-bind-${sourceName}`,
      async () => {
        const result = await getOAuthAuthorizeUrl(sourceName);
        window.location.href = normalizeTrustedExternalUrl(
          result.authorize_url,
        );
      },
      '发起第三方账号绑定',
    );
  };

  const handleUnbindAuthSource = async (id: number, label: string) => {
    const confirmed = await confirmDialog({
      title: '解绑第三方账号',
      message: `确定解绑“${label}”吗？`,
      confirmLabel: '解绑',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    void runBusyAction(
      `auth-source-unbind-${id}`,
      async () => {
        await deleteExternalAccountBinding(id);
        await queryClient.invalidateQueries({
          queryKey: externalAccountBindingsQueryKey,
        });
        setFeedback({ tone: 'success', message: '第三方账号已解绑。' });
      },
      '解绑第三方账号',
    );
  };

  const handleToggleOption = (
    key: keyof typeof systemFields,
    nextValue: boolean,
  ) => {
    setSystemFields((previous) => ({ ...previous, [key]: nextValue }));

    void runBusyAction(
      `toggle-${key}`,
      async () => {
        await saveOptionEntries([[key, String(nextValue)]], '系统开关已更新。');
      },
      '更新系统开关',
    );
  };

  const updateSystemTextField = (
    key: SystemSettingsTextFieldKey,
    value: string,
  ) => {
    setSystemFields((previous) => ({ ...previous, [key]: value }));
  };

  const updateRateLimitOperationField = (
    key: RateLimitOperationFieldKey,
    value: string,
  ) => {
    setOperationFields((previous) => ({ ...previous, [key]: value }));
  };

  const handleSaveSystemGeneralSettings = () => {
    void runBusyAction(
      'system-general',
      async () => {
        await saveOptionEntries(
          [['ServerAddress', normalizeServerUrl(systemFields.ServerAddress)]],
          '通用设置已保存。',
        );
      },
      '保存通用设置',
    );
  };

  const handleSaveSmtpSettings = () => {
    void runBusyAction(
      'system-smtp',
      async () => {
        await saveOptionEntries(
          [
            ['SMTPServer', systemFields.SMTPServer.trim()],
            ['SMTPPort', systemFields.SMTPPort.trim()],
            ['SMTPAccount', systemFields.SMTPAccount.trim()],
            ['SMTPToken', systemFields.SMTPToken.trim()],
          ],
          'SMTP 设置已保存。',
        );
      },
      '保存 SMTP 设置',
    );
  };

  const handleSaveRateLimitSettings = () => {
    void runBusyAction(
      'operation-rate-limit',
      async () => {
        const entries = [
          ['GlobalApiRateLimitNum', operationFields.GlobalApiRateLimitNum],
          [
            'GlobalApiRateLimitDuration',
            operationFields.GlobalApiRateLimitDuration,
          ],
          ['GlobalWebRateLimitNum', operationFields.GlobalWebRateLimitNum],
          [
            'GlobalWebRateLimitDuration',
            operationFields.GlobalWebRateLimitDuration,
          ],
          [
            'DNSWorkerAPIRateLimitNum',
            operationFields.DNSWorkerAPIRateLimitNum,
          ],
          [
            'DNSWorkerAPIRateLimitDuration',
            operationFields.DNSWorkerAPIRateLimitDuration,
          ],
          ['UploadRateLimitNum', operationFields.UploadRateLimitNum],
          ['UploadRateLimitDuration', operationFields.UploadRateLimitDuration],
          ['DownloadRateLimitNum', operationFields.DownloadRateLimitNum],
          [
            'DownloadRateLimitDuration',
            operationFields.DownloadRateLimitDuration,
          ],
          ['CriticalRateLimitNum', operationFields.CriticalRateLimitNum],
          [
            'CriticalRateLimitDuration',
            operationFields.CriticalRateLimitDuration,
          ],
        ] as const;

        for (const [key, rawValue] of entries) {
          const parsedValue = Number.parseInt(rawValue, 10);
          if (Number.isNaN(parsedValue) || parsedValue <= 0) {
            throw new Error(`${key} 必须为大于 0 的整数。`);
          }
          if (key.endsWith('Duration') && parsedValue > 1200) {
            throw new Error(`${key} 不能超过 1200 秒。`);
          }
        }

        await saveOptionEntries(
          entries.map(([key, value]) => [
            key,
            String(Number.parseInt(value, 10)),
          ]),
          '限流设置已保存。',
        );
      },
      '保存限流设置',
    );
  };

  const handleInstallCommercialLicense = () => {
    const token = commercialLicenseToken.trim();
    if (!token) {
      setFeedback({ tone: 'danger', message: '请输入许可证内容。' });
      return;
    }

    void runBusyAction(
      'commercial-license-install',
      async () => {
        await installCommercialLicense(token);
        setCommercialLicenseToken('');
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseQueryKey,
        });
        setFeedback({ tone: 'success', message: '商业授权已安装。' });
      },
      '安装商业授权',
    );
  };

  const handleActivateCommercialLicense = () => {
    void runBusyAction(
      'commercial-license-activate',
      async () => {
        await activateCommercialLicense();
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseQueryKey,
        });
        setFeedback({ tone: 'success', message: '在线授权租约已更新。' });
      },
      '更新在线授权租约',
    );
  };

  const updateCommercialLicenseIssueField = <
    Key extends keyof CommercialLicenseIssueForm,
  >(
    key: Key,
    value: CommercialLicenseIssueForm[Key],
  ) => {
    setCommercialLicenseIssueFields((previous) => ({
      ...previous,
      [key]: value,
    }));
  };

  const toggleCommercialLicenseIssueFeature = (
    feature: string,
    checked: boolean,
  ) => {
    setCommercialLicenseIssueFields((previous) => {
      if (feature === 'all') {
        return {
          ...previous,
          features: checked ? ['all'] : [],
        };
      }

      const withoutAll = previous.features.filter(
        (item) => item !== 'all' && item !== feature,
      );
      return {
        ...previous,
        features: checked ? [...withoutAll, feature] : withoutAll,
      };
    });
  };

  const handleIssueCommercialLicense = () => {
    const payload: CommercialLicenseIssuePayload = {
      license_id: commercialLicenseIssueFields.license_id.trim(),
      customer_id: commercialLicenseIssueFields.customer_id.trim(),
      customer_name: commercialLicenseIssueFields.customer_name.trim(),
      plan: commercialLicenseIssueFields.plan.trim(),
      features: commercialLicenseIssueFields.features,
      max_nodes: parseLicenseLimitInput(commercialLicenseIssueFields.max_nodes),
      max_sites: parseLicenseLimitInput(commercialLicenseIssueFields.max_sites),
      issued_at: commercialLicenseIssueFields.issued_at?.trim() || undefined,
      expires_at: commercialLicenseIssueFields.expires_at?.trim() || undefined,
    };

    if (!payload.license_id) {
      setFeedback({ tone: 'danger', message: '请输入授权编号。' });
      return;
    }
    if (!payload.customer_id && !payload.customer_name) {
      setFeedback({ tone: 'danger', message: '请输入客户名称或客户编号。' });
      return;
    }
    if (!payload.plan) {
      setFeedback({ tone: 'danger', message: '请选择授权版本。' });
      return;
    }
    if (payload.features.length === 0) {
      setFeedback({ tone: 'danger', message: '请至少选择一项授权能力。' });
      return;
    }

    void runBusyAction(
      'commercial-license-issue',
      async () => {
        const result = await issueCommercialLicense(payload);
        setIssuedCommercialLicense(result);
        setCommercialLicenseToken(result.token);
        setFeedback({
          tone: 'success',
          message: '商业授权 token 已生成，并已填入安装框。',
        });
      },
      '生成商业授权',
    );
  };

  const handleRevokeCommercialLicense = async (
    record: CommercialLicenseActivationRecord,
  ) => {
    const licenseID = record.license_id.trim();
    if (!licenseID) {
      return;
    }
    const confirmed = await confirmDialog({
      title: '停用商业授权',
      message: `停用授权 ${licenseID} 后，授权服务器将拒绝它后续激活和续租。客户已有租约最长会在 72 小时后失效。`,
      confirmLabel: '停用授权',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    void runBusyAction(
      `commercial-license-revoke-${licenseID}`,
      async () => {
        await revokeCommercialLicense({
          license_id: licenseID,
          customer_id: record.customer_id,
          reason: 'manual revoke',
        });
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseActivationsQueryKey,
        });
        setFeedback({
          tone: 'success',
          message: '授权已停用，后续续租会被拒绝。',
        });
      },
      '停用商业授权',
    );
  };

  const handleRestoreCommercialLicense = (
    record: CommercialLicenseActivationRecord,
  ) => {
    const licenseID = record.license_id.trim();
    if (!licenseID) {
      return;
    }
    void runBusyAction(
      `commercial-license-restore-${licenseID}`,
      async () => {
        await restoreCommercialLicense({ license_id: licenseID });
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseActivationsQueryKey,
        });
        setFeedback({
          tone: 'success',
          message: '授权已恢复，可以继续激活和续租。',
        });
      },
      '恢复商业授权',
    );
  };

  const handleClearCommercialLicense = async () => {
    const installedLicenseID =
      commercialLicenseQuery.data?.license_id?.trim() ?? '';
    const confirmed = await confirmDialog({
      title: '删除本机授权',
      message: `确认删除本机已安装的商业授权${installedLicenseID ? ` ${installedLicenseID}` : ''}？\n\n删除后会清除本机授权 token 和在线租约状态，不会停用或删除授权服务器上的激活记录。若服务端启用了强制授权，节点或站点创建会被阻断。`,
      confirmLabel: '删除本机授权',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    void runBusyAction(
      'commercial-license-clear',
      async () => {
        await clearCommercialLicense();
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseQueryKey,
        });
        setFeedback({
          tone: 'success',
          message: '本机商业授权和在线租约已删除。',
        });
      },
      '删除商业授权',
    );
  };

  const handleDeleteCommercialLicenseActivation = async (
    record: CommercialLicenseActivationRecord,
  ) => {
    const licenseID = record.license_id.trim();
    if (!licenseID) {
      return;
    }
    const confirmed = await confirmDialog({
      title: '删除授权记录',
      message: `确认删除授权 ${licenseID} 的激活记录和停用记录？\n\n删除后授权服务器将不再显示这组历史激活；如果当前面板安装的正是这个授权，本机授权 token 和在线租约也会一并清除。`,
      confirmLabel: '删除授权',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    void runBusyAction(
      `commercial-license-delete-${licenseID}`,
      async () => {
        await deleteCommercialLicenseActivation({
          license_id: licenseID,
          customer_id: record.customer_id,
        });
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseActivationsQueryKey,
        });
        await queryClient.invalidateQueries({
          queryKey: commercialLicenseQueryKey,
        });
        setFeedback({ tone: 'success', message: '授权记录已删除。' });
      },
      '删除商业授权记录',
    );
  };

  const handleSaveDatabaseAutoCleanup = () => {
    void runBusyAction(
      'database-auto-cleanup',
      async () => {
        const retentionDays = Number.parseInt(
          databaseFields.DatabaseAutoCleanupRetentionDays,
          10,
        );
        if (Number.isNaN(retentionDays) || retentionDays < 1) {
          throw new Error('自动清理保留天数至少为 1 天。');
        }
        await saveOptionEntries(
          [
            [
              'DatabaseAutoCleanupEnabled',
              String(databaseFields.DatabaseAutoCleanupEnabled),
            ],
            ['DatabaseAutoCleanupRetentionDays', String(retentionDays)],
          ],
          '数据库自动清理设置已保存。',
        );
      },
      '保存数据库自动清理设置',
    );
  };

  const handleRefreshDNSSourceDatabaseMirror = () => {
    void runBusyAction(
      'dns-source-database-refresh',
      async () => {
        const result = await refreshDNSSourceDatabaseMirror();
        setFeedback({
          tone: result.started ? 'success' : 'info',
          message: result.message,
        });
        await queryClient.invalidateQueries({
          queryKey: dnsSourceDatabaseMirrorStatusQueryKey,
        });
      },
      '刷新 DNS 源库镜像',
    );
  };

  const handleSaveOtherBrandSettings = () => {
    void runBusyAction(
      'other-brand',
      async () => {
        await saveOptionEntries(
          [
            ['Notice', otherFields.Notice],
            ['SystemName', otherFields.SystemName.trim()],
            ['HomePageLink', otherFields.HomePageLink.trim()],
            ['Footer', otherFields.Footer],
          ],
          '公告与品牌设置已保存。',
        );
      },
      '保存公告与品牌设置',
    );
  };

  const handleSaveOtherAboutSettings = () => {
    void runBusyAction(
      'other-about',
      async () => {
        await saveOptionEntries(
          [['About', otherFields.About]],
          '关于页内容已保存。',
        );
      },
      '保存关于页内容',
    );
  };

  const renderTabContent = () => {
    if (profileQuery.isLoading || publicStatusQuery.isLoading) {
      return <LoadingState />;
    }

    if (profileQuery.isError) {
      return (
        <ErrorState
          title="个人设置加载失败"
          description={getErrorMessage(profileQuery.error)}
        />
      );
    }

    if (publicStatusQuery.isError) {
      return (
        <ErrorState
          title="系统状态加载失败"
          description={getErrorMessage(publicStatusQuery.error)}
        />
      );
    }

    if (externalAccountsQuery.isError) {
      return (
        <ErrorState
          title="账号绑定加载失败"
          description={getErrorMessage(externalAccountsQuery.error)}
        />
      );
    }

    const publicStatus = publicStatusQuery.data;
    const profile = profileQuery.data;
    const externalAccounts = externalAccountsQuery.data ?? [];
    const externalAccountMap = new Map(
      externalAccounts.map((account) => [account.auth_source_name, account]),
    );

    if (!publicStatus || !profile) {
      return (
        <EmptyState
          title="设置暂不可用"
          description="未获取到当前用户或系统状态信息。"
        />
      );
    }

    if (activeTab === 'personal') {
      return (
        <SettingsPersonalSection
          accessToken={accessToken}
          accessTokenIsPending={accessTokenMutation.isPending}
          busyKey={busyKey}
          currentUser={user}
          emailAddress={emailAddress}
          emailCode={emailCode}
          externalAccountMap={externalAccountMap}
          profile={profile}
          profileFields={profileFields}
          publicStatus={publicStatus}
          onBindAuthSource={handleBindAuthSource}
          onBindEmail={handleBindEmail}
          onCopyAccessToken={(token) => handleCopy(token, '访问令牌已复制。')}
          onEmailAddressChange={setEmailAddress}
          onEmailCodeChange={setEmailCode}
          onEmailTurnstileTokenChange={setEmailTurnstileToken}
          onEmailVerification={handleEmailVerification}
          onGenerateAccessToken={() => accessTokenMutation.mutate()}
          onProfileFieldChange={(key, value) =>
            setProfileFields((previous) => ({
              ...previous,
              [key]: value,
            }))
          }
          onSaveProfile={handleProfileSave}
          onUnbindAuthSource={handleUnbindAuthSource}
        />
      );
    }

    if (!isRoot) {
      return (
        <EmptyState
          title="权限不足"
          description="只有超级管理员可以访问系统级设置。"
        />
      );
    }

    if (optionsQuery.isLoading) {
      return <LoadingState />;
    }

    if (optionsQuery.isError) {
      return (
        <ErrorState
          title="设置项加载失败"
          description={getErrorMessage(optionsQuery.error)}
        />
      );
    }

    if (activeTab === 'operation') {
      return (
        <SettingsOperationSection
          busyKey={busyKey}
          operationFields={operationFields}
          deploymentProtocol={deploymentProtocol}
          discoveryCommand={discoveryCommand}
          discoveryToken={discoveryToken}
          geoIPTestIP={geoIPTestIP}
          geoIPLookupResult={geoIPLookupResult}
          geoIPLookupIsPending={geoIPLookupMutation.isPending}
          geoIPLookupIsError={geoIPLookupMutation.isError}
          geoIPLookupError={geoIPLookupMutation.error}
          bootstrapTokenIsLoading={bootstrapQuery.isLoading}
          bootstrapTokenIsError={bootstrapQuery.isError}
          bootstrapTokenError={bootstrapQuery.error}
          bootstrapTokenRotateIsPending={rotateTokenMutation.isPending}
          publicStatus={publicStatus}
          formatDurationLabel={formatDurationLabel}
          onOperationFieldChange={updateOperationField}
          onDeploymentProtocolChange={setDeploymentProtocol}
          onServerAddressChange={handleOperationServerAddressChange}
          onGeoIPTestIPChange={setGeoIPTestIP}
          onGeoIPLookup={handleGeoIPLookup}
          onSaveOperationSettings={handleSaveOperationSettings}
          onSaveAuthoritativeDnsSettings={handleSaveAuthoritativeDnsSettings}
          onRotateBootstrapToken={() => rotateTokenMutation.mutate()}
          onCopyDiscoveryCommand={() =>
            void handleCopy(discoveryCommand, '部署命令已复制。')
          }
        />
      );
    }

    if (activeTab === 'database') {
      return (
        <SettingsDatabaseSection
          busyKey={busyKey}
          databaseFields={databaseFields}
          mirrorIsLoading={dnsSourceDatabaseMirrorStatusQuery.isLoading}
          mirrorStatus={dnsSourceDatabaseMirrorStatusQuery.data}
          onAutoCleanupFieldChange={(key, value) =>
            setDatabaseFields((previous) => ({
              ...previous,
              [key]: value,
            }))
          }
          onRefreshMirror={handleRefreshDNSSourceDatabaseMirror}
          onSaveAutoCleanup={handleSaveDatabaseAutoCleanup}
          onOpenCleanup={(target, label) => {
            setCleanupRetentionDays('');
            setCleanupModalState({ target, label });
          }}
        />
      );
    }
    if (activeTab === 'license') {
      return (
        <CommercialLicenseSettingsSection
          busyKey={busyKey}
          commercialLicense={commercialLicenseQuery.data}
          commercialLicenseError={commercialLicenseQuery.error}
          commercialLicenseIsError={commercialLicenseQuery.isError}
          commercialLicenseIsLoading={commercialLicenseQuery.isLoading}
          commercialLicenseIssuer={commercialLicenseIssuerQuery.data}
          commercialLicenseIssuerError={commercialLicenseIssuerQuery.error}
          commercialLicenseIssuerIsError={commercialLicenseIssuerQuery.isError}
          commercialLicenseIssuerIsLoading={
            commercialLicenseIssuerQuery.isLoading
          }
          commercialLicenseActivations={commercialLicenseActivationsQuery.data}
          commercialLicenseActivationsError={
            commercialLicenseActivationsQuery.error
          }
          commercialLicenseActivationsIsError={
            commercialLicenseActivationsQuery.isError
          }
          commercialLicenseActivationsIsLoading={
            commercialLicenseActivationsQuery.isLoading
          }
          commercialLicenseToken={commercialLicenseToken}
          commercialLicenseIssueFields={commercialLicenseIssueFields}
          issuedCommercialLicense={issuedCommercialLicense}
          setCommercialLicenseToken={setCommercialLicenseToken}
          handleInstallCommercialLicense={handleInstallCommercialLicense}
          handleActivateCommercialLicense={handleActivateCommercialLicense}
          handleClearCommercialLicense={handleClearCommercialLicense}
          updateCommercialLicenseIssueField={updateCommercialLicenseIssueField}
          toggleCommercialLicenseIssueFeature={
            toggleCommercialLicenseIssueFeature
          }
          handleIssueCommercialLicense={handleIssueCommercialLicense}
          handleRevokeCommercialLicense={handleRevokeCommercialLicense}
          handleRestoreCommercialLicense={handleRestoreCommercialLicense}
          handleDeleteCommercialLicenseActivation={
            handleDeleteCommercialLicenseActivation
          }
          handleCopy={handleCopy}
          refreshCommercialLicenseActivations={() =>
            commercialLicenseActivationsQuery.refetch()
          }
        />
      );
    }
    if (activeTab === 'system') {
      return (
        <SystemSettingsSection
          authSourcesCount={authSourcesQuery.data?.length ?? 0}
          busyKey={busyKey}
          systemFields={systemFields}
          operationFields={operationFields}
          formatSecondsLabel={formatSecondsLabel}
          onOpenAuthSources={() => setAuthSourceModalOpen(true)}
          onToggleOption={handleToggleOption}
          onSystemFieldChange={updateSystemTextField}
          onOperationFieldChange={updateRateLimitOperationField}
          onSaveGeneralSettings={handleSaveSystemGeneralSettings}
          onSaveSmtpSettings={handleSaveSmtpSettings}
          onSaveRateLimitSettings={handleSaveRateLimitSettings}
        />
      );
    }
    return (
      <SettingsOtherSection
        busyKey={busyKey}
        otherFields={otherFields}
        onFieldChange={(key, value) =>
          setOtherFields((previous) => ({
            ...previous,
            [key]: value,
          }))
        }
        onSaveAbout={handleSaveOtherAboutSettings}
        onSaveBrand={handleSaveOtherBrandSettings}
      />
    );
  };

  return (
    <div className="space-y-6">
      <PageHeader title="设置" />

      <div className="flex flex-wrap gap-3">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => handleTabChange(tab.key)}
            className={[
              'rounded-2xl border px-4 py-3 text-left transition',
              activeTab === tab.key
                ? 'border-[var(--border-strong)] bg-[var(--accent-soft)] text-[var(--foreground-primary)]'
                : 'border-[var(--border-default)] bg-[var(--surface-muted)] text-[var(--foreground-secondary)] hover:border-[var(--border-strong)] hover:text-[var(--foreground-primary)]',
            ].join(' ')}
          >
            <p className="text-sm font-semibold">{tab.label}</p>
            <p className="mt-1 text-xs leading-5 text-inherit/80">
              {tab.description}
            </p>
          </button>
        ))}
      </div>

      {renderTabContent()}

      <AuthSourceModal
        isOpen={authSourceModalOpen}
        sources={authSourcesQuery.data ?? []}
        isLoading={authSourcesQuery.isLoading}
        error={authSourcesQuery.error}
        onClose={() => setAuthSourceModalOpen(false)}
        onChanged={async () => {
          await Promise.all([
            queryClient.invalidateQueries({ queryKey: authSourcesQueryKey }),
            queryClient.invalidateQueries({ queryKey: ['public-status'] }),
          ]);
        }}
      />

      <AppModal
        isOpen={cleanupModalState !== null}
        title={`清理${cleanupModalState?.label ?? ''}`}
        description="输入保留天数后，将只保留该天数范围内的数据；如果留空，则会直接删除该类数据的全部历史记录。"
        onClose={() => {
          if (databaseCleanupMutation.isPending) {
            return;
          }
          setCleanupModalState(null);
        }}
        footer={
          <div className="flex flex-wrap justify-end gap-2">
            <SecondaryButton
              type="button"
              onClick={() => setCleanupModalState(null)}
              disabled={databaseCleanupMutation.isPending}
            >
              取消
            </SecondaryButton>
            <DangerButton
              type="button"
              disabled={databaseCleanupMutation.isPending}
              onClick={() => {
                if (!cleanupModalState) {
                  return;
                }
                const trimmed = cleanupRetentionDays.trim();
                if (trimmed !== '') {
                  const retentionDays = Number.parseInt(trimmed, 10);
                  if (
                    !/^\d+$/.test(trimmed) ||
                    Number.isNaN(retentionDays) ||
                    retentionDays < 1
                  ) {
                    setFeedback({
                      tone: 'danger',
                      message: '手动清理保留天数至少为 1 天。',
                      detail: `当前输入：${trimmed}`,
                    });
                    return;
                  }
                  databaseCleanupMutation.mutate({
                    target: cleanupModalState.target,
                    retention_days: retentionDays,
                  });
                  return;
                }
                databaseCleanupMutation.mutate({
                  target: cleanupModalState.target,
                });
              }}
            >
              {databaseCleanupMutation.isPending ? '清理中...' : '确认清理'}
            </DangerButton>
          </div>
        }
      >
        <div className="space-y-5">
          <div className="rounded-2xl border border-[var(--status-danger-border)] bg-[var(--status-danger-soft)] px-4 py-4 text-sm leading-6 text-[var(--status-danger-foreground)]">
            该操作会直接删除数据库中的历史观测数据，删除后无法恢复，请确认当前选择的数据类型和保留范围无误。
          </div>
          <ResourceField
            label="保留天数"
            hint="留空表示全部删除；填写时必须为大于等于 1 的整数。"
          >
            <ResourceInput
              type="number"
              min={1}
              value={cleanupRetentionDays}
              onChange={(event) => setCleanupRetentionDays(event.target.value)}
              placeholder="例如 30；留空则全部删除"
            />
          </ResourceField>
          {databaseCleanupMutation.isError ? (
            <ErrorState
              title="数据库清理失败"
              description={getErrorMessage(databaseCleanupMutation.error)}
            />
          ) : null}
        </div>
      </AppModal>
    </div>
  );
}
