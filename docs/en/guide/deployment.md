# Deployment

You will learn the recommended DuShengCDN deployment model, Server and Agent requirements, source startup workflow, integration steps, upgrade paths, and uninstall entry points.

For production, use PostgreSQL for the Server database, set `SESSION_SECRET` explicitly, and add Redis for multi-instance or commercial deployments. Agent controls OpenResty through the OpenResty binary; Docker deployments run the Agent image that already includes OpenResty.

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
| Redis | Optional; recommended for multi-instance deployments and consistent production rate limiting |
| Port | `3000` by default |

Agent:

| Item | Requirement |
| --- | --- |
| OS | Install script supports Linux and macOS. systemd service is created only on Linux + systemd. |
| Architecture | `amd64` or `arm64` |
| OpenResty | Required for local Agent installs |
| Docker | Required only when running the Agent Docker image |
| Network | Agent node must reach the Server URL |

DNS Worker, for self-hosted authoritative DNS:

| Item | Requirement |
| --- | --- |
| Port | Public UDP `53` and TCP `53` must be open |
| Count | Run at least two Workers in production and configure multiple NS records at the registrar |
| Network | Worker must reach the Server over HTTPS to pull read-only snapshots |
| Data | Worker keeps the last valid snapshot locally with checksum metadata and recoverable GSLB debounce state |
| Safety | Worker rate-limits queries per source IP and truncates oversized UDP responses so resolvers can retry over TCP |

Recommended production sizing:

| Scenario | Suggested resources |
| --- | --- |
| Small management plane, 1-5 nodes, short observability retention | 2 vCPU, 2 GB memory, 20 GB usable disk |
| Medium management plane, 10+ nodes or heavier analytics | 4 vCPU, 4 GB memory, 50 GB+ usable disk |
| PostgreSQL | Dedicated volume or database instance with regular backups |
| Agent node | Start from 1 vCPU and 512 MB memory, then size for real OpenResty traffic, TLS, and cache pressure |

## Docker Compose Server

Reusable Compose templates live under `examples/compose/`:

| Template | Use Case |
| --- | --- |
| `server.production.yaml` + `server.env.example` | Run Server + PostgreSQL from the GHCR image |
| `server.source.yaml` + `server.env.example` | Build Server from the current source checkout |
| `server.override.example.yaml` | Override host bind address, port, data directory, and log level |
| `agent.yaml` + `agent.env.example` | Run Agent with Docker Compose |
| `dns-worker.yaml` + `dns-worker.env.example` | Run DNS Worker with Docker Compose |

Image-based Server example:

```bash
mkdir -p /opt/dushengcdn-compose
cd /opt/dushengcdn-compose
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.production.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.env.example
vi .env
docker compose --env-file .env -f server.production.yaml up -d
docker compose --env-file .env -f server.production.yaml ps
```

Or create an inline `docker-compose.yml`:

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
      SESSION_SECRET: ${SESSION_SECRET:?set SESSION_SECRET in .env}
      DSN: postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
      GIN_MODE: release
      LOG_LEVEL: info
      # Optional: recommended for commercial or multi-instance deployments.
      # REDIS_CONN_STRING: redis://redis:6379/0
      # REDIS_REQUIRED: "true"
      # Optional: enforced private commercial licensing.
      # DUSHENGCDN_LICENSE_REQUIRED: "true"
      # DUSHENGCDN_LICENSE_PUBLIC_KEYS: base64url-or-hex-ed25519-public-key
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

Commercial or multi-instance deployments should also provide a Redis service and set `REDIS_CONN_STRING`. If Redis must not silently degrade to in-process helpers, set `REDIS_REQUIRED=true`. To enforce private commercial licensing, set `DUSHENGCDN_LICENSE_REQUIRED=true` and `DUSHENGCDN_LICENSE_PUBLIC_KEYS`, then install the `dscdn_license_v1...` token from **Settings -> Commercial License**.

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
export SESSION_SECRET="$(openssl rand -hex 32)"
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

If the host port `3000` is already in use, change only the host-side mapping, for example:

```yaml
ports:
  - "3010:3000"
```

For source Compose deployments, keep local deployment parameters in `dushengcdn_server/.env`:

```bash
cd /opt/dushengcdn/dushengcdn_server
cp -n .env.example .env
```

Edit `DUSHENGCDN_HTTP_PORT`, `POSTGRES_PASSWORD`, `SESSION_SECRET`, and `DSN` in `.env`. Avoid editing the tracked `docker-compose.yaml`; keeping local values in `.env` lets future `git pull --ff-only` upgrades continue cleanly.

You can also use the integrated installer from the repository root:

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh
```

When `.env` does not exist, the script creates it from `.env.example`. A fresh install gets generated `POSTGRES_PASSWORD`, `SESSION_SECRET`, and a matching `DSN`. If an older source deployment already has `dushengcdn_server/postgres-data`, the script preserves the default database password and DSN from `.env.example` and only generates `SESSION_SECRET`, avoiding a password mismatch with the existing PostgreSQL data directory. After `docker compose up`, the script verifies that the `dushengcdn` service stays running and checks `SERVER_URL/api/status`; if the HTTP check fails, it prints recent logs and hints for database authentication, port mapping, and reverse-proxy upstream port issues. Source Compose defaults to host port `DUSHENGCDN_HTTP_PORT=3010` while the container still listens on `3000`.

The integrated installer also deploys a same-host DNS Worker by default. It first checks whether a local Worker already exists by looking for `dushengcdn-dns-worker.service`, unit files, `/opt/dushengcdn-dns-worker`, the Worker env file, a same-name Docker container, a Worker process, or a DuShengCDN process listening on port `53`. If found, it skips automatic Worker creation and installation.

Explicit public IP and Server URL:

```bash
bash scripts/install-server.sh \
  --server-url http://127.0.0.1:3010 \
  --public-ip 203.0.113.10
```

Panel-only install:

```bash
bash scripts/install-server.sh --skip-dns-worker
```

Use `--force-dns-worker-reinstall` only when you intentionally want to replace the local Worker configuration.

## Connect Agent

In production, maintain each node's node pool, public IP pool, scheduling weight, and drain mode from the node detail page. Automatic DNS selects online, healthy public IPs from the site node pool. Site-level GSLB can bind multiple node pools and use pool weight, node load, and debounce settings. Cache purge and warmup commands are still sent to online Agents in the site's default node pool.

Cloudflare DNS mode syncs records in the background; it is not per-query routing. Authoritative DNS mode uses DNS Workers and the latest Server snapshot to answer each DNS query.

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

## Authoritative DNS Worker

The authoritative DNS query plane runs as an independent DNS Worker role. Server manages Zones, static records, and Worker Tokens, exposes read-only snapshots through `GET /api/dns-snapshot`, and receives Worker status through `POST /api/dns-worker-heartbeat`. Worker listens on UDP/TCP `53` and answers from its in-memory snapshot; it does not access the database or call external GeoIP APIs on the query path.

Recommended install script:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN
```

Defaults:

| Item | Default |
| --- | --- |
| Install directory | `/opt/dushengcdn-dns-worker` |
| Service | `dushengcdn-dns-worker.service` |
| Listen address | `:53` unless `--listen` is provided |
| Snapshot cache | `INSTALL_DIR/data/dns-worker-snapshot.json` |
| Country MMDB | downloaded to `INSTALL_DIR/data/geoip/GeoLite2-Country.mmdb` unless disabled |
| Query rate limit | `200` queries per second per source IP |
| UDP response size | `1232` bytes before setting TC and falling back to TCP |

If Server and Worker are on the same host, pass an explicit public bind address when needed:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url http://127.0.0.1:3000 \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen 203.0.113.10:53
```

After installation, run a read-only diagnosis on the Worker host:

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com
```

The script checks the systemd service, install directory, env file, listeners, snapshot, GeoIP file, logs, and UDP/TCP SOA/NS query results.

For a same-host panel + Worker deployment, run the end-to-end read-only verification before switching registrar NS or production traffic:

```bash
cd /opt/dushengcdn
bash scripts/verify-authoritative-dns.sh --public-ip PUBLIC_IP --zone example.com
```

It checks Server Compose, `/api/status`, DNS Worker systemd state, install files, the DNS listener, snapshot file, and UDP/TCP SOA/NS responses from `PUBLIC_IP`.

Docker example:

```bash
docker run -d --name dushengcdn-dns-worker --restart unless-stopped \
  -p 53:53/udp -p 53:53/tcp \
  -v dushengcdn-dns-worker-data:/data \
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=https://cdn.example.com \
  -e DUSHENGCDN_DNS_WORKER_TOKEN=YOUR_DNS_WORKER_TOKEN \
  ghcr.io/satands/dushengcdn-dns-worker:latest
```

For country-code GSLB pools, mount a local MaxMind Country MMDB and set `DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH`. Source CIDR and global scheduling continue to work without GeoIP.

Operational flow:

1. Create a Zone under **Authoritative DNS** and enter the NS names that will be delegated at the registrar.
2. Create a DNS Worker and copy the Token or install command.
3. Deploy at least two Workers and configure registrar NS records; add Glue/host records when NS names are inside the same Zone.
4. Use the migration wizard to verify matching Zone, online Workers, public UDP/TCP `53` reachability, fresh consistent snapshots, and optional site GSLB readiness.
5. Switch the site **Automatic DNS** mode to **Authoritative DNS** from the wizard or site detail page.

Uninstall Worker:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

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

* Root users can check stable Server releases from the top bar. Server automatic upgrades are disabled by default; production deployments should usually upload a reviewed Server binary and confirm the manual upgrade.
* To allow one-click automatic upgrades, set `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true` and ensure the Release includes the matching Server binary plus a same-name `.sha256` file.
* Preview releases can be checked manually.
* For source or Compose deployments, back up local `docker-compose.yaml` settings or move them into an override before pulling new code.

Source directory + Compose upgrade:

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose up -d --build
docker compose ps
```

If local Compose edits block the pull, record host ports, DSN, passwords, and tokens first. After confirming there are no source changes to keep:

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose up -d --build
docker compose ps
```

If the panel still does not open after an upgrade, run `bash scripts/diagnose-server.sh` from the repository root to collect Compose state, `/api/status`, port listeners, and recent logs. Source Compose uses the host port from `.env` (`DUSHENGCDN_HTTP_PORT`, default `3010`); the container listens on `3000`, so reverse proxies should point to the host port.

For source Compose builds, `DUSHENGCDN_VERSION` is passed into the Dockerfile and embedded into the Server or Agent binary. The top-bar version reads the running Server version from `/api/status`, and the node list shows the version reported by Agent.

Agent:

* Agents follow stable releases by default.
* The install script can be rerun to reinstall or upgrade. When no matching Release asset exists, it builds from source and embeds the current Git version instead of reporting `dev`.
* For Docker Compose Agent deployments, rebuild with `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build`.
* Preview upgrades require manual action.

Uninstall Agent:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

The uninstall script stops Agent and removes the systemd service and install directory. It does not remove the local OpenResty installation.

## Backup and Root Password Reset

Before upgrades, back up the database and the Server data directory. Source deployments can use the repository backup script:

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

The script reads `dushengcdn_server/.env` by default. In `auto` mode it prefers a reachable Compose PostgreSQL service and runs `pg_dump`; otherwise it backs up SQLite. It also archives `dushengcdn-data` and writes a `manifest.txt` under `dushengcdn_server/backups/<timestamp>/`. It does not stop, restore, overwrite, or delete production data.

Restore example for PostgreSQL Compose:

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
cd /opt/dushengcdn
bash scripts/restore-server.sh \
  --backup-path dushengcdn_server/backups/20260601-120000 \
  --yes
cd dushengcdn_server
docker compose up -d
```

`restore-server.sh` verifies manifest checksums when available, refuses to restore while the `dushengcdn` service is still running by default, and creates a pre-restore safety backup of the current database and `dushengcdn-data`.

Manual PostgreSQL Compose backup example:

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
docker compose exec -T postgres pg_dump -U dushengcdn -d dushengcdn > backups/dushengcdn-$(date +%F-%H%M%S).sql
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

If the root password is lost but you still have server access:

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
```

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
