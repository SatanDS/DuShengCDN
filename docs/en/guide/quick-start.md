# Quick Start

You will learn how to start DuShengCDN Server with Docker Compose, sign in for the first time, connect the first Agent, and verify that a configuration was published to a node.

The minimal DuShengCDN setup contains:

| Component | Responsibility |
| --- | --- |
| Server | Management UI, management API, Agent API, configuration rendering, release publishing, and state storage |
| Agent | Runs on proxy nodes, pulls configuration, writes OpenResty files, validates, and reloads |
| OpenResty | Receives traffic and proxies requests to origins |
| DNS Worker (optional) | Authoritative DNS query plane that answers A/AAAA records from real-time GSLB snapshots |

Agent controls OpenResty through the OpenResty binary. Local installs need an `openresty` executable on the node; Docker installs can run the Agent image that already includes OpenResty.

## Requirements

| Item | Requirement |
| --- | --- |
| Docker / Docker Compose | Used to start Server and PostgreSQL; also used if you run the Agent or DNS Worker Docker image |
| OpenResty | Required for local Agent installs unless `--openresty-path` points to a custom binary |
| Reachable ports | Server listens on `3000` by default. Agent nodes must reach the Server URL. |
| Browser | Used to open the management UI |

Docker Engine 24+ and Docker Compose v2 are recommended. Older versions may still work, but Compose v2 is the tested command style used by the documentation.

## 1. Start Server

The fastest production-style path is to use the repository Compose templates:

```bash
mkdir -p /opt/dushengcdn-compose
cd /opt/dushengcdn-compose
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.production.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.env.example
vi .env
docker compose --env-file .env -f server.production.yaml up -d
```

The inline example below is equivalent and useful when you want a single file.

Create `docker-compose.yml` in an empty directory:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: dushengcdn
      POSTGRES_USER: dushengcdn
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dushengcdn -d dushengcdn"]
      interval: 10s
      timeout: 5s
      retries: 5

  dushengcdn:
    image: ghcr.io/satands/dushengcdn:${DUSHENGCDN_VERSION:?set DUSHENGCDN_VERSION in .env}
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "127.0.0.1:3000:3000"
    environment:
      SESSION_SECRET: ${SESSION_SECRET:?set SESSION_SECRET in .env}
      DSN: ${DSN:?set DSN in .env}
      GIN_MODE: release
      LOG_LEVEL: info

volumes:
  postgres-data:
```

Start:

```bash
cat > .env <<'EOF'
DUSHENGCDN_VERSION=v1.0.0
POSTGRES_PASSWORD=change-this-database-password
SESSION_SECRET=replace-with-openssl-rand-hex-32
DSN=postgres://dushengcdn:change-this-database-password@postgres:5432/dushengcdn?sslmode=disable
EOF
docker compose up -d
```

If you deploy from a source checkout, you can also run the integrated installer from the repository root:

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh --public-ip 203.0.113.10
```

On first install, the script creates `dushengcdn_server/.env` from `.env.example` and generates `POSTGRES_PASSWORD`, `SESSION_SECRET`, and a matching `DSN`. When upgrading an older source Compose deployment that already has `dushengcdn_server/postgres-data`, it preserves the default database password/DSN from `.env.example` and only generates `SESSION_SECRET`, avoiding PostgreSQL authentication failures against existing data. After Compose starts, the script verifies that the `dushengcdn` service stays running and checks `SERVER_URL/api/status`; if the HTTP check fails, it prints recent logs and hints for database authentication, port mapping, and reverse-proxy upstream port issues. Source Compose defaults to host port `DUSHENGCDN_HTTP_PORT=3010` while the container still listens on `3000`.

By default, the script also tries to deploy a same-host DNS Worker. Before doing so it checks for an existing `dushengcdn-dns-worker.service`, systemd unit file, `/opt/dushengcdn-dns-worker`, Worker env file, same-name Docker container, Worker process, or DuShengCDN process already listening on port `53`. If any of those are found, Worker creation and installation are skipped.

Panel-only install:

```bash
bash scripts/install-server.sh --skip-dns-worker
```

Verify:

```bash
docker compose ps
docker compose logs -f dushengcdn
```

The example publishes the management port on the host. When the `dushengcdn` container is running and logs show `server listening`, open it directly; for HTTPS-only access set `DUSHENGCDN_HTTP_BIND=127.0.0.1` and put it behind a reverse proxy:

```text
http://SERVER_IP:3000
```

First login:

| Username | Password |
| --- | --- |
| `root` | `DUSHENGCDN_INITIAL_ROOT_PASSWORD` from `.env`, or the generated one-time password in the `initial-root-password.txt` file named in the Server log |

Change the root password immediately after first login, then remove or rotate the bootstrap value in `.env`.

## 2. Prepare an Agent Token

Agents can connect with either:

| Credential | Use Case |
| --- | --- |
| `discovery_token` | First-time automatic node registration. Server exchanges it for a node-specific token. |
| `agent_token` | A node-specific token created or assigned in the management UI. |

Prepare one of them in the management UI before continuing:

| Credential | UI Path |
| --- | --- |
| `discovery_token` | Settings -> Operations -> Discovery Token and deployment commands |
| `agent_token` | Nodes/IP Pools -> create or select a node -> Details -> Node information -> Node identity and deployment |

## 3. Install Agent

Run the install script on the proxy node.

With `discovery_token`:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token-file /run/secrets/dushengcdn-discovery-token
```

With node-specific `agent_token`:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token-file /run/secrets/dushengcdn-agent-token
```

The script defaults to:

| Item | Default |
| --- | --- |
| Install directory | `/opt/dushengcdn-agent` |
| Config file | `/opt/dushengcdn-agent/agent.json` |
| systemd service | `dushengcdn-agent.service` |
| OpenResty path | Auto-detects `openresty` unless `--openresty-path` is provided |

Check status:

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -f
```

If systemd is unavailable, the script prints a manual start command.

## 4. Publish the First Configuration

In the management UI:

1. Create a site configuration with a site name, domain, and origin URL.
2. Ensure the site is enabled.
3. Preview the rendered configuration or review the diff.
4. Publish and activate a new version.
5. Wait for the Agent to discover and apply the version through heartbeat.

Version numbers use `YYYYMMDD-NNN`. Historical versions are immutable; rollback reactivates an old version.

## 5. Optional: Enable Authoritative DNS

Use this only when you want domains to be delegated to DuShengCDN DNS Workers and answered from real-time GSLB snapshots. The integrated Server installer can create and install a same-host Worker automatically, or you can create a DNS Worker Token in the left sidebar under **Authoritative DNS** and install Workers manually:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --token-file /run/secrets/dushengcdn-dns-worker-token
```

The script installs to `/opt/dushengcdn-dns-worker`, creates `dushengcdn-dns-worker.service`, listens on UDP/TCP `53`, stores a local snapshot cache, and downloads a Country MMDB by default for country-code GSLB pools. Docker is also supported:

```bash
DUSHENGCDN_VERSION=v1.0.0
docker run -d --name dushengcdn-dns-worker --restart unless-stopped \
  -p 53:53/udp -p 53:53/tcp \
  -v dushengcdn-dns-worker-data:/data \
  -v /run/secrets/dushengcdn-dns-worker-token:/run/secrets/dushengcdn_dns_worker_token:ro \
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_DNS_WORKER_TOKEN_FILE=/run/secrets/dushengcdn_dns_worker_token \
  -e DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200 \
  -e DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=1232 \
  ghcr.io/satands/dushengcdn-dns-worker:${DUSHENGCDN_VERSION:?set DUSHENGCDN_VERSION}
```

After the Worker is online, delegate the zone at your registrar, then switch the site detail **Automatic DNS** section to **Authoritative DNS** and select the Zone. Production should run at least two Workers and allow both UDP and TCP `53`.

## 6. Verify Success

In the UI:

| Location | Expected Result |
| --- | --- |
| Node list | Agent node is online |
| Node detail | Current version matches the active version |
| Apply logs | Latest apply succeeded |
| Versions page | New version is active |

On the Agent node:

```bash
journalctl -u dushengcdn-agent -n 100 --no-pager
```

## Common Failures

| Symptom | What to Check |
| --- | --- |
| Cannot open the UI | Confirm `docker compose ps` shows Server running and host port `3000` is free |
| Login works but data cannot be saved | Check PostgreSQL health and the username/password/database in `DSN` |
| Agent cannot register | Confirm the Agent node can reach `--server-url`, and check whether the token is wrong or expired |
| Agent is online but does not apply | Confirm the site is enabled and a version was published and activated |
| OpenResty apply fails | Check apply logs and `journalctl -u dushengcdn-agent`, especially domains, certificates, upstream URLs, and port conflicts |
| Automatic DNS does not return node IPs | Confirm the site node pool has online nodes, public IPs of the right A/AAAA type, scheduling enabled, and drain mode off |

See [Troubleshooting](./troubleshooting.md) for deeper diagnostics.
