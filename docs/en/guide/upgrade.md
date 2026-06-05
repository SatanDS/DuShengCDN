# Upgrade and Maintenance

You will learn: How to upgrade the Server and Agent, how to clean up observability data, and which verification commands to execute before and after maintenance.

Before upgrading, it is recommended to confirm the current activated version, the latest Agent application result, and the database backup policy. Do not upgrade in production environments while configuration publishing, large-scale Agent reconnection, or database migrations are in progress.

## Server Upgrade

Root users can check the Server stable version from the top bar of the management console. Server automatic upgrades are disabled by default; production deployments should usually upload a reviewed Server binary and confirm the manual upgrade. To allow one-click automatic upgrades, set `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true` and ensure the Release includes the matching Server binary plus a same-name `.sha256` file.

To try a preview version, you can manually check the corresponding release. It is recommended to prioritize the stable version in production environments.

For source directory + Docker Compose deployments, use `.env` for local deployment values:

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

Store `DUSHENGCDN_HTTP_PORT`, `POSTGRES_PASSWORD`, `SESSION_SECRET`, `DSN`, and any legacy `AGENT_TOKEN` in `dushengcdn_server/.env`. Do not keep editing the tracked `docker-compose.yaml`; otherwise future `git pull --ff-only` can be blocked by local changes.

If old local Compose edits block the pull, first record host ports, passwords, DSN, `SESSION_SECRET`, and tokens. After confirming there are no source changes to keep:

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

The integrated installer can also be used for source Compose upgrades:

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh
```

When `.env` is missing, the script creates it from `.env.example`. A fresh install gets generated database credentials and `SESSION_SECRET`. If an existing `dushengcdn_server/postgres-data` directory is detected, it preserves the default database password and DSN copied from `.env.example` and only generates `SESSION_SECRET`, avoiding PostgreSQL authentication failures against existing data. The script also checks that the `dushengcdn` Compose service remains running after startup and verifies `SERVER_URL/api/status`; failures print recent logs plus likely causes for PostgreSQL auth, database connection, port binding, host-port, or reverse-proxy upstream issues.

After upgrading, confirm:

```bash
docker compose ps
docker compose logs -n 100 dushengcdn
```

If the upgrade command finishes but the management UI does not open, run the read-only diagnostic helper first:

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

For source Compose deployments, the default host panel port comes from `.env` as `DUSHENGCDN_HTTP_PORT=3010`; the container listens on `3000`. If the diagnostic output shows `http://127.0.0.1:3010/api/status` is healthy but the browser domain still fails, Nginx, Nginx Proxy Manager, Baota, or another reverse proxy is usually still pointing at the old `127.0.0.1:3000` upstream. Point it to the host port from `.env` instead.

If it is a source deployment, confirm that there are no database migration or startup errors in the logs after restarting the Server.

## DNS Worker Upgrade

DNS Worker installations created by the script can be reinstalled or upgraded by rerunning the install command with the same Server URL and Worker Token. The script prefers GitHub Release binaries and verifies matching `.sha256` files; if no release asset exists, it builds from source and embeds the current Git version.

Uninstall DNS Worker:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

## Agent Upgrade

Node Agents follow stable versions by default for automatic updates. Preview upgrades must be triggered manually.

The installation script can be executed repeatedly to reinstall or upgrade the Agent:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

The script prefers GitHub Release binaries. If no matching asset exists, it builds from source and embeds the current Git version instead of reporting `dev`.

For Docker Compose Agent deployments:

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build
docker compose -f docker-compose.agent.yaml ps
```

Note: Currently, the installation script will delete the entire installation directory during reinstallation, including the old `agent.json`, local state, cache data, and downloaded binaries. Please confirm that you still have a usable Token on hand before executing.

After upgrading, confirm:

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -n 100 --no-pager
```

## Data Maintenance

The settings page of the management console can maintain the observability data automatic cleanup policy:

| Configuration Item | Description |
| --- | --- |
| `DatabaseAutoCleanupEnabled` | Whether to enable daily automatic cleanup |
| `DatabaseAutoCleanupRetentionDays` | Automatic cleanup retention days, at least 1 day |

Once enabled, the Server will clean up access logs, metric snapshots, and request reports at 3 AM every day.

## Backup and Restore

Before upgrading, back up the database and the Server data directory. Source deployments can use the repository script:

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

The script reads `dushengcdn_server/.env` by default. In `auto` mode it backs up reachable Compose PostgreSQL with `pg_dump`; otherwise it backs up SQLite. It also archives `dushengcdn-data` and writes a `manifest.txt` with paths and checksum metadata under `dushengcdn_server/backups/<timestamp>/`.

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

The restore script verifies manifest SHA-256 data when possible, refuses to restore while the Compose `dushengcdn` service is still running by default, and creates a pre-restore safety backup of the current database and `dushengcdn-data`.

## Common Verification Commands

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
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Docs:

```bash
cd docs
pnpm build
```
