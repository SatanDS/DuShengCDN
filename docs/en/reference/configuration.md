# Configuration

## Server CLI Flags

| Flag | Purpose | Default |
| --- | --- | --- |
| `--port` | Server listen port | `3000` |
| `--log-dir` | Log directory | empty |
| `--reset-root-password` | Reset the `root` password and exit without starting HTTP service | empty |
| `--create-dns-worker-name` | Create a DNS Worker, print the newly created Token, then exit without starting HTTP service | empty |
| `--create-dns-worker-public-address` | Public address saved for the Worker created by `--create-dns-worker-name` | empty |
| `--version` | Print version and exit | `false` |
| `--help` | Print help and exit | `false` |

## Server Environment Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | Server listen port | `3000` |
| `GIN_MODE` | Gin mode | release unless `debug` |
| `LOG_LEVEL` | Log level | `info` |
| `SESSION_SECRET` | Session signing secret | random on startup |
| `SQLITE_PATH` | SQLite database path | `dushengcdn.db` |
| `DSN` | PostgreSQL DSN, preferred over SQLite | empty |
| `SQL_DSN` | Legacy PostgreSQL DSN, lower priority than `DSN` | empty |
| `REDIS_CONN_STRING` | Redis connection string | empty |
| `UPLOAD_PATH` | Upload directory | `upload` |
| `AGENT_TOKEN` | Legacy global Agent token | empty |

When `DSN` and `SQL_DSN` both exist, `DSN` wins. PostgreSQL is preferred when configured. If PostgreSQL is empty and a local SQLite file exists, Server migrates SQLite data at startup.

## Frontend Build Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `NEXT_PUBLIC_API_BASE_URL` | Frontend API base path | `/api` |
| `NEXT_PUBLIC_APP_VERSION` | Static frontend build version; the dashboard top bar prefers the running Server version returned by `/api/status` | `dev` |
| `NEXT_DEV_BACKEND_URL` | Local dev backend proxy target | `http://127.0.0.1:3000` |

## Docker Compose Build Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `DUSHENGCDN_VERSION` | Passed into the Dockerfile for source Compose Server or Agent builds and embedded into the matching binary | `dev` |

For source Compose Server updates, use `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build` so the top-bar version reflects the running Git build. Set the same variable for Agent Compose rebuilds so the node list shows the Git version reported by Agent.

The repository also provides `examples/compose/` templates for image-based Server, source-build Server, override files, Agent, and DNS Worker deployments. `server.env.example` includes:

| Variable | Purpose | Default |
| --- | --- | --- |
| `DUSHENGCDN_REPO_DIR` | Source checkout path used by `server.source.yaml` | `../..` |
| `DUSHENGCDN_HTTP_BIND` | Host bind address used by Server Compose templates | `127.0.0.1` |

## Integrated Server Installer Options

`scripts/install-server.sh` deploys the source Compose Server and, by default, a same-host DNS Worker. When `.env` is missing, it creates it from `.env.example`; fresh installs generate `POSTGRES_PASSWORD`, `SESSION_SECRET`, and `DSN`. If an existing `dushengcdn_server/postgres-data` directory is detected, it preserves the default database password/DSN copied from `.env.example` and only generates `SESSION_SECRET`, avoiding PostgreSQL authentication failures against existing data. Existing `.env` files are not overwritten. After `docker compose up`, the script verifies that the `dushengcdn` service stays running and checks `SERVER_URL/api/status`; if the HTTP check fails, it prints recent logs and hints for database authentication, port mapping, and reverse-proxy upstream port issues.

`scripts/diagnose-server.sh` diagnoses source Compose panel access issues. It also reads `SERVER_DIR/.env`, derives the default `SERVER_URL` from `DUSHENGCDN_HTTP_PORT`, and prints Compose state, `/api/status` checks, port listeners, and recent logs without editing configuration or restarting services.

Before automatic Worker creation, the script checks for an existing `dushengcdn-dns-worker.service`, systemd unit file, install directory, env file, same-name Docker container, Worker process, or DuShengCDN process listening on port `53`. If one is found, Worker creation and installation are skipped unless forced.

| Option | Purpose | Default |
| --- | --- | --- |
| `--server-dir` | Server compose/source directory | repository `dushengcdn_server` |
| `--compose-file` | Docker Compose file | `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Docker Compose env file | `SERVER_DIR/.env` |
| `--server-url` | Server URL used by the DNS Worker | `http://127.0.0.1:DUSHENGCDN_HTTP_PORT` |
| `--public-ip` | Public IP saved to the Worker and used for default `--listen` | auto-detected IPv4 |
| `--skip-dns-worker` | Deploy panel only; do not create/install DNS Worker | `false` |
| `--force-dns-worker-reinstall` | Reinstall Worker even when local deployment is detected | `false` |
| `--dns-worker-name` | Worker name created in Server | `DNS服务响应端` |
| `--dns-worker-install-dir` | DNS Worker install directory | `/opt/dushengcdn-dns-worker` |
| `--dns-worker-listen` | DNS Worker listen address | `PUBLIC_IP:53` |
| `--dns-worker-query-rate-limit` | DNS queries per second per source IP | `200` |
| `--dns-worker-udp-response-size` | Maximum UDP DNS response payload size | `1232` |
| `--dns-worker-repo` | GitHub repository used by the Worker installer | `SatanDS/DuShengCDN` |
| `--dns-worker-source-ref` | Git branch, tag, or commit used for source build fallback | `main` |
| `--dns-worker-no-geoip-download` | Do not download Country MMDB automatically | `false` |

## Runtime Options

The settings page maintains these hot-updatable options:

| Option | Purpose | Default |
| --- | --- | --- |
| `AgentHeartbeatInterval` | Agent heartbeat interval in milliseconds | `10000` |
| `AgentWebsocketUpgradeEnabled` | Allow Agent to upgrade from HTTP heartbeat to WebSocket | `true` |
| `NodeOfflineThreshold` | Node offline threshold in milliseconds | `120000` |
| `AgentUpdateRepo` | Agent update repository | `SatanDS/DuShengCDN` |
| `GeoIPProvider` | Node/IP region provider | `ipinfo` |
| `DatabaseAutoCleanupEnabled` | Enable daily observability cleanup | `false` |
| `DatabaseAutoCleanupRetentionDays` | Retention days | `30` |
| `AuthoritativeDNSDefaultTTL` | TTL used by authoritative DNS mode when UI value is automatic | `30` |
| `AuthoritativeDNSSnapshotMaxAge` | Maximum age of the last valid DNS Worker snapshot | `300` |
| `GSLBMetricFreshnessSeconds` | Maximum node load metric age accepted by GSLB snapshot generation and simulation diagnostics | `120` |
| `GSLBProbeSchedulingEnabled` | Whether Agent DNS Worker probe results participate in authoritative DNS GSLB selection; enabled mode filters candidates without fresh successful probes, then applies a bounded quality factor from healthy ratio, stale ratio, and average RTT to the base weight/load-aware score | `false` |

OpenResty performance and cache options are also stored in the Option table, including `OpenRestyWorkerProcesses`, `OpenRestyWorkerConnections`, `OpenRestyProxyConnectTimeout`, `OpenRestyProxyReadTimeout`, `OpenRestyCacheEnabled`, `OpenRestyCachePath`, and `OpenRestyCacheMaxSize`.

`AgentUpdateRepo` releases must publish a matching `.sha256` file for each Agent binary, such as `dushengcdn-agent-linux-amd64.sha256`. Agent self-update verifies the SHA-256 digest before replacing the executable.

## Agent Configuration

Agent supports the `-config` CLI flag, an `agent.json` file, and the `LOG_LEVEL` environment variable.

Agent environment variables can override config-file values:

| Variable | Purpose |
| --- | --- |
| `DUSHENGCDN_SERVER_URL` | Control plane URL |
| `DUSHENGCDN_AGENT_TOKEN` | Node-specific Agent Token |
| `DUSHENGCDN_DISCOVERY_TOKEN` | First-registration Discovery Token |
| `DUSHENGCDN_NODE_NAME` | Node name |
| `DUSHENGCDN_NODE_IP` | Node IP |
| `DUSHENGCDN_DATA_DIR` | Agent data directory |
| `DUSHENGCDN_OPENRESTY_PATH` | OpenResty binary path |
| `DUSHENGCDN_HEARTBEAT_INTERVAL` | Heartbeat interval |
| `DUSHENGCDN_REQUEST_TIMEOUT` | Request timeout |
| `DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT` | Local observability port |

| Field | Purpose | Required | Default / behavior |
| --- | --- | --- | --- |
| `server_url` | Control plane URL | yes | none |
| `agent_token` | Node-specific auth token | one of `agent_token` / `discovery_token` | empty |
| `discovery_token` | Global token for first registration | one of `agent_token` / `discovery_token` | empty |
| `node_name` | Node name | no | host name |
| `node_ip` | Node IP | no | auto-detected |
| `openresty_path` | OpenResty binary path | no | `openresty` |
| `openresty_container_name` | Deprecated Docker-control field, read for compatibility only | no | empty |
| `openresty_docker_image` | Deprecated Docker-control field, read for compatibility only | no | empty |
| `openresty_observability_port` | Local observability and OpenResty health-check port | no | `18081` |
| `docker_binary` | Deprecated Docker-control field, read for compatibility only | no | empty |
| `data_dir` | Agent data directory | no | `data` under config directory |
| `access_log_path` | OpenResty access log path | no | `data_dir/var/log/dushengcdn/access.log` |
| `runtime_config_dir` | Runtime config directory, including `pow_config.json` | no | `data_dir/etc/dushengcdn` |
| `heartbeat_interval` | Heartbeat interval | no | `10000` ms |
| `request_timeout` | HTTP timeout | no | `10000` ms |

`heartbeat_interval` and `request_timeout` accept milliseconds or Go duration strings.

## DNS Worker Environment and Flags

DNS Worker can be configured by environment variables or CLI flags. The install script writes these values into `/opt/dushengcdn-dns-worker/dns-worker.env` and creates `dushengcdn-dns-worker.service`.

| Environment Variable | Purpose | Default |
| --- | --- | --- |
| `DUSHENGCDN_DNS_WORKER_SERVER_URL` | Server URL | empty |
| `DUSHENGCDN_DNS_WORKER_TOKEN` | DNS Worker Token | empty |
| `DUSHENGCDN_DNS_WORKER_LISTEN_ADDR` | UDP/TCP listen address | `:53` |
| `DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH` | Local snapshot cache path | `data/dns-worker-snapshot.json` |
| `DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL` | Heartbeat, snapshot pull, and rollup upload interval | `10s` |
| `DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT` | Server request timeout | `10s` |
| `DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE` | Maximum dynamic-answer snapshot age | `5m` |
| `DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT` | Per-source-IP DNS queries per second; `0` disables | `200` |
| `DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE` | Maximum UDP response payload size before setting TC | `1232` |
| `DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH` | Optional local MaxMind Country MMDB path | empty |

| Flag | Purpose | Default |
| --- | --- | --- |
| `--server-url` | Server URL | environment variable |
| `--token` | DNS Worker Token | environment variable |
| `--listen` | UDP/TCP listen address | `:53` |
| `--snapshot-path` | Local snapshot cache path | `data/dns-worker-snapshot.json` |
| `--heartbeat-interval` | Heartbeat and snapshot pull interval | `10s` |
| `--request-timeout` | Server request timeout | `10s` |
| `--snapshot-max-age` | Maximum dynamic-answer snapshot age | `5m` |
| `--query-rate-limit` | Per-source-IP DNS queries per second; `0` disables | `200` |
| `--udp-response-size` | Maximum UDP response payload size | `1232` |
| `--geoip-database` | Optional local MaxMind Country MMDB path | empty |

DNS Worker heartbeats report GeoIP Country database status to Server. If GeoIP is not loaded, source CIDR and `global` scheduling still work, but country-code GSLB pools will not match.
