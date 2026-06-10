import type { NodeItem } from '@/features/nodes/types';
import { shellQuote } from '@/lib/utils/shell';

export const WS_CONNECTED_LAST_SEEN = '__DUSHENGCDN_WS_CONNECTED__';

export function isWSConnectedLastSeen(value: string | null | undefined) {
  return value === WS_CONNECTED_LAST_SEEN;
}

export function isMeaningfulTime(value: string | null | undefined) {
  return (
    Boolean(value) &&
    !isWSConnectedLastSeen(value) &&
    !String(value).startsWith('0001-01-01')
  );
}

export function getNodeStatusVariant(status: NodeItem['status']) {
  if (status === 'online') {
    return 'success';
  }

  if (status === 'pending') {
    return 'warning';
  }

  return 'danger';
}

export function getNodeStatusLabel(status: NodeItem['status']) {
  if (status === 'online') {
    return '在线';
  }

  if (status === 'pending') {
    return '待接入';
  }

  return '离线';
}

export function getApplyVariant(result: NodeItem['latest_apply_result']) {
  if (result === 'success') {
    return 'success';
  }

  if (result === 'warning') {
    return 'warning';
  }

  if (result === 'failed') {
    return 'danger';
  }

  return 'warning';
}

export function getApplyLabel(result: NodeItem['latest_apply_result']) {
  if (result === 'success') {
    return '成功';
  }

  if (result === 'warning') {
    return '警告';
  }

  if (result === 'failed') {
    return '失败';
  }

  return '暂无';
}

export function hasTargetConfigFields(node: NodeItem) {
  return (
    'target_config_available' in node ||
    'config_in_sync' in node ||
    'target_config_version' in node ||
    'target_config_checksum' in node ||
    'target_config_pool' in node
  );
}

export function isTargetConfigAvailable(node: NodeItem) {
  return node.target_config_available !== false;
}

export function hasLaggingConfig(node: NodeItem, activeVersion: string) {
  if (typeof node.config_in_sync === 'boolean') {
    return node.config_in_sync === false;
  }

  return Boolean(activeVersion) && node.current_version !== activeVersion;
}

export function getUpdateMode(node: NodeItem) {
  if (node.update_requested) {
    if (node.update_channel === 'preview') {
      return { label: '等待预览更新', variant: 'warning' as const };
    }

    return { label: '等待更新', variant: 'warning' as const };
  }

  if (node.auto_update_enabled) {
    return { label: '自动', variant: 'success' as const };
  }

  return { label: '手动', variant: 'info' as const };
}

export function getOpenrestyStatusVariant(
  status: NodeItem['openresty_status'],
) {
  if (status === 'healthy') {
    return 'success';
  }

  if (status === 'unhealthy') {
    return 'danger';
  }

  return 'warning';
}

export function getOpenrestyStatusLabel(status: NodeItem['openresty_status']) {
  if (status === 'healthy') {
    return '健康';
  }

  if (status === 'unhealthy') {
    return '异常';
  }

  return '未知';
}

function parseVersionParts(version: string) {
  const normalized = version.trim().replace(/^v/i, '');
  if (!normalized || normalized.toLowerCase() === 'unknown') {
    return null;
  }

  return normalized.split('.').map((segment) => {
    const matched = segment.trim().match(/^\d+/);
    return matched ? Number.parseInt(matched[0], 10) : 0;
  });
}

function isOlderVersion(current: string, target: string) {
  const currentParts = parseVersionParts(current);
  const targetParts = parseVersionParts(target);
  if (!currentParts || !targetParts) {
    return false;
  }

  const maxLength = Math.max(currentParts.length, targetParts.length);
  for (let index = 0; index < maxLength; index += 1) {
    const currentPart = currentParts[index] ?? 0;
    const targetPart = targetParts[index] ?? 0;
    if (currentPart < targetPart) {
      return true;
    }
    if (currentPart > targetPart) {
      return false;
    }
  }

  return false;
}

export function shouldShowManualUpdate(
  agentVersion: string,
  serverVersion: string,
) {
  const normalizedServerVersion = serverVersion.trim();
  const normalizedAgentVersion = agentVersion.trim();

  if (
    !normalizedServerVersion ||
    normalizedServerVersion.toLowerCase() === 'dev' ||
    !normalizedAgentVersion ||
    normalizedAgentVersion.toLowerCase() === 'unknown'
  ) {
    return false;
  }

  return isOlderVersion(normalizedAgentVersion, normalizedServerVersion);
}

export function getServerUrl(value: string) {
  return value.trim().replace(/\/+$/, '');
}

export type DeploymentProtocol = 'http' | 'https';

export function getDeploymentProtocol(value: string): DeploymentProtocol {
  return value.trim().toLowerCase().startsWith('http://') ? 'http' : 'https';
}

export function stripServerUrlProtocol(value: string) {
  return getServerUrl(value).replace(/^https?:\/\//i, '');
}

export function buildDeploymentServerUrl(
  protocol: DeploymentProtocol,
  value: string,
) {
  const endpoint = stripServerUrlProtocol(value);
  return endpoint ? `${protocol}://${endpoint}` : '';
}

const installerScriptUrl =
  'https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh';

export function buildNodeInstallCommand(serverUrl: string, agentToken: string) {
  void agentToken;
  const quotedServerUrl = shellQuote(serverUrl);
  return [
    `token_file="$(mktemp)"`,
    `chmod 600 "$token_file"`,
    `trap 'stty echo 2>/dev/null || true; rm -f "$token_file"' EXIT`,
    `printf 'Agent token: ' >&2`,
    `stty -echo 2>/dev/null || true`,
    `IFS= read -r agent_token`,
    `stty echo 2>/dev/null || true`,
    `printf '\\n' >&2`,
    `printf '%s\\n' "$agent_token" > "$token_file"`,
    `unset agent_token`,
    `curl -fsSL ${installerScriptUrl} | bash -s -- \\`,
    `  --server-url ${quotedServerUrl} \\`,
    `  --agent-token-file "$token_file"`,
    `rm -f "$token_file"`,
    `trap - EXIT`,
  ].join('\n');
}

export function buildNodeDockerInstallCommand(
  serverUrl: string,
  agentToken: string,
) {
  void agentToken;
  const quotedServerUrl = shellQuote(serverUrl);
  return [
    `secret_dir="\${XDG_CONFIG_HOME:-$HOME/.config}/dushengcdn-agent"`,
    `mkdir -p "$secret_dir"`,
    `chmod 700 "$secret_dir"`,
    `token_file="$secret_dir/agent-token"`,
    `printf 'Agent token: ' >&2`,
    `stty -echo 2>/dev/null || true`,
    `IFS= read -r agent_token`,
    `stty echo 2>/dev/null || true`,
    `printf '\\n' >&2`,
    `printf '%s\\n' "$agent_token" > "$token_file"`,
    `chmod 600 "$token_file"`,
    `unset agent_token`,
    `docker run -d --name dushengcdn-agent --restart unless-stopped \\`,
    `  -p 80:80 -p 443:443 \\`,
    `  -v "$token_file":/run/secrets/dushengcdn_agent_token:ro \\`,
    `  -e DUSHENGCDN_SERVER_URL=${quotedServerUrl} \\`,
    `  -e DUSHENGCDN_AGENT_TOKEN_FILE=/run/secrets/dushengcdn_agent_token \\`,
    `  ghcr.io/satands/dushengcdn-agent:latest`,
  ].join('\n');
}
