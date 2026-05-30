# Deployment

You will learn the recommended DuShengCDN deployment model, Server and Agent requirements, source startup workflow, integration steps, upgrade paths, and uninstall entry points.

For production, use PostgreSQL for the Server database and set `SESSION_SECRET` explicitly. Agent controls OpenResty through the OpenResty binary; Docker deployments run the Agent image that already includes OpenResty.

## Topology

```text
Browser
  |
  v
DuShengCDN Server :3000
  |
  | Agent API / heartbeat / config pull
  v
DuShengCDN Agent
  |
  v
OpenResty binary
  |
  v
Origin service
```

## Requirements

Server:

| Item | Requirement |
| --- | --- |
| Go | `1.25+`, source run only |
| Node.js | `18+`, frontend source build only |
| Database | Writable SQLite directory or reachable PostgreSQL instance |
| Port | `3000` by default |

Agent:

| Item | Requirement |
| --- | --- |
| OS | Install script supports Linux and macOS. systemd service is created only on Linux + systemd. |
| Architecture | `amd64` or `arm64` |
| OpenResty | Required for local Agent installs |
| Docker | Required only when running the Agent Docker image |
| Network | Agent node must reach the Server URL |

[Needs confirmation: recommended production CPU, memory, and disk size]

## Docker Compose Server

Create `docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: dushengcdn
      POSTGRES_USER: dushengcdn
      POSTGRES_PASSWORD: replace-with-strong-password
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dushengcdn -d dushengcdn"]
      interval: 10s
      timeout: 5s
      retries: 5

  dushengcdn:
    image: ghcr.io/satands/dushengcdn:latest
    container_name: dushengcdn
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "3000:3000"
    environment:
      SESSION_SECRET: replace-with-a-long-random-string
      DSN: postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
      GIN_MODE: release
      LOG_LEVEL: info
    volumes:
      - dushengcdn-data:/data

volumes:
  postgres-data:
  dushengcdn-data:
```

Start:

```bash
docker compose up -d
docker compose ps
docker compose logs -f dushengcdn
```

Open `http://localhost:3000`. The default account is `root` / `123456`; change it immediately.

## Run Server from Source

Build the management UI first:

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm build
```

Then start Server:

```bash
cd dushengcdn_server
export SESSION_SECRET='replace-with-a-long-random-string'
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
# Optional: PostgreSQL takes precedence when set.
# export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
go run .
```

Default port is `3000`. You can also set it explicitly:

```bash
go run . --port 3000 --log-dir ./logs
```

## Connect Agent

With `discovery_token`:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

With node-specific `agent_token`:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

Supported options:

| Option | Description |
| --- | --- |
| `--server-url` | Server URL, required |
| `--discovery-token` | First-registration token, mutually exclusive with `--agent-token` |
| `--agent-token` | Node-specific token, mutually exclusive with `--discovery-token` |
| `--install-dir` | Install directory, default `/opt/dushengcdn-agent` |
| `--openresty-path` | OpenResty binary path, auto-detected when omitted |
| `--repo` | GitHub repository for Agent downloads, default `SatanDS/DuShengCDN` |
| `--no-service` | Do not create a systemd service |

Check status:

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -f
```

## Run Agent Manually

From source:

```bash
cd dushengcdn_agent
export LOG_LEVEL='info'
go run ./cmd/agent -config /path/to/agent.json
```

Build and run:

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
export LOG_LEVEL='info'
./dushengcdn-agent -config /path/to/agent.json
```

Minimal `agent.json`:

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "./data",
  "openresty_path": "openresty",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

When `openresty_path` is not configured, Agent runs `openresty`.

## Minimal Integration Flow

1. Start Server and sign in.
2. Prepare `agent_token` or `discovery_token`.
3. Start Agent and confirm the node is online.
4. Create an enabled site configuration.
5. Publish and activate a new version.
6. Check node detail and apply logs.
7. Visit the domain or verify with `curl`.

## Upgrade and Uninstall

Server:

* Root users can check and upgrade stable Server releases from the top bar.
* Preview releases can be checked manually.
* Binary upload upgrades are also supported.

Agent:

* Agents follow stable releases by default.
* The install script can be rerun to reinstall or upgrade.
* Preview upgrades require manual action.

Uninstall Agent:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

The uninstall script stops Agent and removes the systemd service and install directory. It does not remove the local OpenResty installation.

## Validation Commands

Server:

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

Agent:

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

Frontend:

```bash
cd dushengcdn_server/web
pnpm build
```

Swagger:

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```
