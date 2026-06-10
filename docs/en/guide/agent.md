# Connect Agent

DuShengCDN Agent runs on proxy nodes. It handles registration, heartbeat, configuration sync, OpenResty file writes, validation, reload, rollback, and self-update.

## Authentication

| Method | Use case |
| --- | --- |
| `agent_token` | The node already exists or has a dedicated credential |
| `discovery_token` | First-time auto-registration; Server exchanges it for a node token |

At least one of them is required.

## Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token-file /run/secrets/dushengcdn-agent-token
```

Or with discovery:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token-file /run/secrets/dushengcdn-discovery-token
```

The script prefers GitHub Release binaries. If no matching asset exists, it builds from source and embeds the current Git version instead of reporting `dev`.

## Configuration Example

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "./data",
  "openresty_path": "openresty",
  "openresty_observability_port": 18081,
  "observability_replay_minutes": 15,
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

`openresty_observability_port` is only for local Agent health checks and `stub_status`. Keep it bound to loopback; do not publish `18081` to the public Internet in Docker or firewall rules.

Without `openresty_path`, Agent runs `openresty` by default.

Agent self-update and the install script require the GitHub Release to include the target binary plus matching `.sha256` and `.sig` files. The downloaded binary is verified before it replaces or installs the local executable.

## Docker

```bash
DUSHENGCDN_VERSION=v1.0.0
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -v /run/secrets/dushengcdn-agent-token:/run/secrets/dushengcdn_agent_token:ro \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN_FILE=/run/secrets/dushengcdn_agent_token \
  ghcr.io/satands/dushengcdn-agent:${DUSHENGCDN_VERSION:?set DUSHENGCDN_VERSION}
```

## Run from Source

```bash
cd dushengcdn_agent
export LOG_LEVEL='info'
go run ./cmd/agent -config /path/to/agent.json
```

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```
