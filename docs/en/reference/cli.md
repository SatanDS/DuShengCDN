# Commands and Scripts

You will learn: Common commands for starting, building, testing, installing, and uninstalling the DuShengCDN Server, management console frontend, Agent, Swagger, and documentation site.

## Server

Start from source:

```bash
cd dushengcdn_server
export SESSION_SECRET='replace-with-random-string'
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

Specify listening port and log directory:

```bash
go run . --port 3000 --log-dir ./logs
```

Reset the root password offline:

```bash
cd dushengcdn_server
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn-server --reset-root-password 'replace-with-new-password'
```

Create a DNS Worker and print the newly created Token:

```bash
cd dushengcdn_server
./dushengcdn-server \
  --create-dns-worker-name 'DNS service responder' \
  --create-dns-worker-public-address '203.0.113.10'
```

This command creates only the Worker identity and Token, then exits without starting the HTTP service. `scripts/install-server.sh` uses it to automate same-host DNS Worker deployment.

Integrated source Compose panel + same-host DNS Worker install:

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh --public-ip 203.0.113.10
```

The script checks for an existing DNS Worker before automatic creation and installation. Use `--skip-dns-worker` for panel-only installs, and `--force-dns-worker-reinstall` only when you intentionally want to replace local Worker configuration.

Reusable Compose templates:

| Template | Description |
| --- | --- |
| `examples/compose/server.production.yaml` | GHCR image Server + PostgreSQL |
| `examples/compose/server.source.yaml` | Source-build Server + PostgreSQL |
| `examples/compose/server.override.example.yaml` | Override example for port, data directory, and log level |
| `examples/compose/agent.yaml` | Agent Docker Compose template |
| `examples/compose/dns-worker.yaml` | DNS Worker Docker Compose template |

Back up Server data:

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

Common backup options:

| Option | Description |
| --- | --- |
| `--server-dir` | Server compose/source directory, default repository `dushengcdn_server` |
| `--backup-dir` | Backup output directory, default `SERVER_DIR/backups` |
| `--mode auto|postgres|sqlite` | Database backup mode, default `auto` |
| `--compose-file` | Docker Compose file, default `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose env file, default `SERVER_DIR/.env` when present |
| `--data-dir` | Server data directory to archive, default `SERVER_DIR/dushengcdn-data` |
| `--sqlite-path` | SQLite database path, default from `.env` or `DATA_DIR/dushengcdn.db` |
| `--postgres-service` | Compose PostgreSQL service name, default `postgres` |
| `--postgres-db` | PostgreSQL database name, default from `.env` or `dushengcdn` |
| `--postgres-user` | PostgreSQL user name, default from `.env` or `dushengcdn` |

Restore Server data:

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
cd /opt/dushengcdn
bash scripts/restore-server.sh --backup-path dushengcdn_server/backups/20260601-120000 --yes
cd dushengcdn_server
docker compose up -d
```

Common restore options:

| Option | Description |
| --- | --- |
| `--backup-path` | Backup directory created by `backup-server.sh`, required |
| `--mode auto|postgres|sqlite` | Restore mode, default manifest mode or auto-detect |
| `--server-dir` | Server compose/source directory, default repository `dushengcdn_server` |
| `--compose-file` | Docker Compose file, default `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose env file, default `SERVER_DIR/.env` when present |
| `--data-dir` | Server data directory to restore, default `SERVER_DIR/dushengcdn-data` |
| `--skip-data-dir` | Restore database only |
| `--skip-current-backup` | Do not create a pre-restore safety backup |
| `--force` | Continue when Server running-state protection is not applicable |
| `--yes` | Confirm overwrite, required |

Test:

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Frontend

Development:

```bash
cd dushengcdn_server/web
pnpm install
pnpm dev
```

Build static artifacts:

```bash
cd dushengcdn_server/web
pnpm build
```

Checks:

```bash
cd dushengcdn_server/web
pnpm lint
pnpm typecheck
pnpm test
```

## Agent

Run from source:

```bash
cd dushengcdn_agent
go run ./cmd/agent -config /path/to/agent.json
```

Compile:

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
```

Test:

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Install Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

## Uninstall Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

## Install DNS Worker

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN
```

Common options:

| Option | Description |
| --- | --- |
| `--server-url` | Server URL, required |
| `--token` / `--dns-worker-token` | DNS Worker Token, required |
| `--install-dir` | Install directory, default `/opt/dushengcdn-dns-worker` |
| `--listen` | UDP/TCP listen address, default `:53` |
| `--snapshot-path` | Snapshot cache path |
| `--geoip-database` | Optional local MaxMind Country MMDB path |
| `--geoip-database-url` | Country MMDB download URL |
| `--no-geoip-download` | Do not download Country MMDB automatically |
| `--query-rate-limit` | Per-source-IP DNS queries per second; `0` disables |
| `--udp-response-size` | Maximum UDP DNS response payload size |
| `--repo` | GitHub repository, default `SatanDS/DuShengCDN` |
| `--source-ref` | Git branch, tag, or commit used when building from source |
| `--no-service` | Do not create a systemd service |

## Uninstall DNS Worker

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

## Swagger

Regenerate Swagger documentation:

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

## Docs

Local preview:

```bash
cd docs
pnpm dev
```

Build:

```bash
cd docs
pnpm build
```
