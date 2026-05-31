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
  --agent-token YOUR_AGENT_TOKEN
```

Or with discovery:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token YOUR_DISCOVERY_TOKEN
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

Without `openresty_path`, Agent runs `openresty` by default.

Agent self-update requires the GitHub Release to include both the target binary and a matching `.sha256` file. The downloaded binary is verified before it replaces the local executable.

## Docker

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:latest
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
