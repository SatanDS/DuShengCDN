# Configuration

## Server CLI Flags

| Flag | Purpose | Default |
| --- | --- | --- |
| `--port` | Server listen port | `3000` |
| `--log-dir` | Log directory | empty |
| `--reset-root-password-file` / `--reset-root-password-stdin` | Read the new password from a file or stdin, reset the `root` password, and exit without starting HTTP service | empty |
| `--reset-root-password` | Compatibility argv input; avoid it because it can leak through shell history or process arguments | empty |
| `--create-dns-worker-name` | Create a DNS Worker, print the newly created Token, then exit without starting HTTP service | empty |
| `--create-dns-worker-public-address` | Public address saved for the Worker created by `--create-dns-worker-name` | empty |
| `--version` | Print version and exit | `false` |
| `--help` | Print help and exit | `false` |

## Server Environment Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | Server listen port | `3000` |
| `DUSHENGCDN_LISTEN_ADDRESS` / `LISTEN_ADDRESS` | Server bind address; the commercial systemd installer defaults it to `127.0.0.1`. Use `0.0.0.0` only with firewalling and HTTPS reverse proxy protection | empty, listens on all addresses |
| `GIN_MODE` | Gin mode | release unless `debug` |
| `LOG_LEVEL` | Log level | `info` |
| `SESSION_SECRET` | Session signing secret; release mode requires an explicit value with at least 32 characters | random on startup in debug mode |
| `TRUSTED_PROXIES` | Reverse proxy IP/CIDR list trusted for `X-Forwarded-For` / `X-Real-IP`; separated by comma, semicolon, whitespace, or newline | empty |
| `SQLITE_PATH` | SQLite database path | `dushengcdn.db` |
| `DSN` | PostgreSQL DSN, preferred over SQLite | empty |
| `SQL_DSN` | Legacy PostgreSQL DSN, lower priority than `DSN` | empty |
| `DATABASE_MAX_OPEN_CONNS` | Maximum open database connections | `30` |
| `DATABASE_MAX_IDLE_CONNS` | Maximum idle database connections | `10` |
| `DATABASE_CONN_MAX_LIFETIME_SECONDS` | Maximum database connection lifetime in seconds | `1800` |
| `REDIS_CONN_STRING` | Redis connection string | empty |
| `REDIS_REQUIRED` | Require Redis initialization to succeed; startup fails when Redis is missing or unreachable | `false` |
| `UPLOAD_PATH` | Upload directory | `upload` |
| `DUSHENGCDN_PUBLIC_STATUS_RUNTIME_METADATA` | Allow unauthenticated `/api/status` to include the running Server version, start time, and `ServerAddress`; disabled by default to reduce public fingerprinting | `false` |
| `AGENT_TOKEN` | Legacy global Agent token, ignored unless `DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED=true` | empty |
| `DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED` | Temporarily allow the legacy global Agent token during old Agent migration | `false` |
| `DUSHENGCDN_LICENSE_REQUIRED` | Require a valid commercial license for gated commercial capabilities | `false` |
| `DUSHENGCDN_LICENSE_PUBLIC_KEYS` | Ed25519 public keys for commercial license verification; base64url, standard base64, or hex; separated by comma, semicolon, whitespace, or newline | empty |
| `DUSHENGCDN_LICENSE_ALLOW_UNSIGNED` | Allow unsigned development licenses; not recommended for production | `false` |
| `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED` | Allow the management console to download and replace the Server binary from GitHub Releases | `false` |
| `DUSHENGCDN_SERVER_UPDATE_REPO` | GitHub Release repository used by the top-bar version checker, in `owner/repo` format; commercial private builds should point to a binary-only release repository | `SatanDS/SatanDS-DuShengCDN-releases` |
| `DUSHENGCDN_GITHUB_RELEASE_TOKEN` | Read-only GitHub token for private Release access; grant only the binary release repository, never the source repository | empty |

When `DSN` and `SQL_DSN` both exist, `DSN` wins. PostgreSQL is preferred when configured. If PostgreSQL is empty and a local SQLite file exists, Server migrates SQLite data at startup.

`TRUSTED_PROXIES` defaults to empty, so client-supplied proxy headers are ignored to prevent spoofed `X-Forwarded-For` rate-limit bypasses. Set it only to controlled reverse proxy egress IPs or private CIDR ranges; global ranges such as `0.0.0.0/0` and `::/0` are rejected.

When `REDIS_CONN_STRING` is empty, Redis-backed helpers fall back to in-process behavior. Multi-instance and commercial deployments should configure Redis; set `REDIS_REQUIRED=true` when startup must fail instead of degrading.

Legacy global Agent token compatibility is disabled by default. If old Agents still use `AGENT_TOKEN`, set `DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED=true` or enable the `AgentLegacyGlobalTokenEnabled` runtime option only for the migration window, then move Agents to `discovery_token` or node-specific `agent_token`.

Commercial license tokens use the `dscdn_license_v1.<payload>.<signature>` format with Ed25519 signatures. For enforced private deployments, set `DUSHENGCDN_LICENSE_REQUIRED=true`, configure `DUSHENGCDN_LICENSE_PUBLIC_KEYS`, then install the license from **Settings -> Commercial License**.

Server automatic upgrades are disabled by default. Production deployments should usually check the version in the console, upload a reviewed Server binary, and confirm the manual upgrade. If `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true` is enabled, the Release must include the matching Server binary, a same-name `.sha256` file, and a same-name `.sig` signature file, such as `dushengcdn-server-linux-amd64.sha256`; the downloaded binary is verified before it replaces the executable. For commercial-only deployments, keep the source repository private and publish assets to a binary-only repository such as `SatanDS/SatanDS-DuShengCDN-releases`; set `DUSHENGCDN_SERVER_UPDATE_REPO` and `AgentUpdateRepo` to that repository. The binary repository can be public because it contains no source code and runtime access is controlled by license tokens. If binary downloads must also be private, the Server top bar can use a fine-scoped `DUSHENGCDN_GITHUB_RELEASE_TOKEN`, but Agent self-update should be disabled or moved to a Server proxy/short-lived signed URL flow; do not distribute a GitHub token to Agents.

Production Compose files require `POSTGRES_PASSWORD`, `SESSION_SECRET`, and `DSN` to be set explicitly. The installer can generate them, but Compose templates no longer fall back to public placeholder database credentials.

### Commercial License Issuing Tool

`dushengcdn_server/cmd/license` provides an offline issuer for Ed25519 keys, signed license tokens, and signature inspection:

```bash
cd dushengcdn_server
go run ./cmd/license keygen
# Save the private_key from keygen outside the repo in a restricted file, for example:
# install -m 600 /dev/stdin /run/secrets/dushengcdn-license-private-key
go run ./cmd/license sign \
  -private-key-file /run/secrets/dushengcdn-license-private-key \
  -license-id lic-2026-001 \
  -customer-name "Example Ltd." \
  -plan enterprise \
  -features all \
  -max-nodes 20 \
  -max-sites 200 \
  -expires-at 2027-12-31
go run ./cmd/license inspect -token "$LICENSE_TOKEN" -public-key "$DUSHENGCDN_LICENSE_PUBLIC_KEY"
```

Set the generated `public_key` as `DUSHENGCDN_LICENSE_PUBLIC_KEYS`. Keep the `private_key` offline on the issuing side and pass it with `-private-key-file` from a restricted file; do not put it into shell history, process arguments, Server environment variables, or Compose files. `features` accepts `all` or a comma-separated set of `acme-automation`, `authoritative-dns`, `cloudflare-dns`, `gslb`, `ddos-protection`, `waf`, `cc-protection`, and `geo-access-control`.

## Frontend Build Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `NEXT_PUBLIC_API_BASE_URL` | Frontend API base path | `/api` |
| `NEXT_PUBLIC_APP_VERSION` | Static frontend build version; unauthenticated `/api/status` does not expose the running Server version unless `DUSHENGCDN_PUBLIC_STATUS_RUNTIME_METADATA=true` is enabled | `dev` |
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

`scripts/diagnose-dns-worker.sh` diagnoses DNS Worker hosts. It reads `INSTALL_DIR/dns-worker.env`, checks the systemd service, install directory, listeners, snapshot, GeoIP file, and recent logs, and can run UDP/TCP SOA/NS queries when `--public-ip` and `--zone` are provided. It does not edit configuration or restart services.

`scripts/verify-authoritative-dns.sh` verifies same-host panel + DNS Worker deployments before production use. It reads the Server `.env` and Worker `dns-worker.env`, then checks Server Compose, `/api/status`, DNS Worker systemd state, install files, listeners, snapshot file, and UDP/TCP SOA/NS responses from `PUBLIC_IP`.

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
| `AgentLegacyGlobalTokenEnabled` | Temporarily allow the legacy global `AGENT_TOKEN` during old Agent migration | `false` |
| `AgentDiscoveryToken` | First-registration Discovery Token for Agents; registered nodes receive node-specific `agent_token` values | generated on first read |
| `NodeOfflineThreshold` | Node offline threshold in milliseconds | `120000` |
| `AgentUpdateRepo` | Agent update repository; commercial private builds should point to the binary-only release repository | `SatanDS/SatanDS-DuShengCDN-releases` |
| `GeoIPProvider` | Node/IP region provider | `ipinfo` |
| `DatabaseAutoCleanupEnabled` | Enable daily observability cleanup | `false` |
| `DatabaseAutoCleanupRetentionDays` | Retention days | `30` |
| `AuthoritativeDNSDefaultTTL` | TTL used by authoritative DNS mode when UI value is automatic | `30` |
| `AuthoritativeDNSSnapshotMaxAge` | Maximum age of the last valid DNS Worker snapshot | `300` |
| `GSLBMetricFreshnessSeconds` | Maximum node load metric age accepted by GSLB snapshot generation and simulation diagnostics | `120` |
| `GSLBProbeSchedulingEnabled` | Whether Agent DNS Worker probe results participate in authoritative DNS GSLB selection; enabled mode filters candidates without fresh successful probes, then applies a bounded quality factor from healthy ratio, stale ratio, and average RTT to the base weight/load-aware score | `false` |

OpenResty performance and cache options are also stored in the Option table, including `OpenRestyWorkerProcesses`, `OpenRestyWorkerConnections`, `OpenRestyProxyConnectTimeout`, `OpenRestyProxyReadTimeout`, `OpenRestyCacheEnabled`, `OpenRestyCachePath`, and `OpenRestyCacheMaxSize`.

`AgentUpdateRepo` releases must publish matching `.sha256` and `.sig` files for each Agent binary, such as `dushengcdn-agent-linux-amd64.sha256` and `dushengcdn-agent-linux-amd64.sig`. Agent self-update and the install script verify the SHA-256 digest and Ed25519 signature before replacing or installing the executable.

## Agent Configuration

Agent supports the `-config` CLI flag, an `agent.json` file, and the `LOG_LEVEL` environment variable.

Agent environment variables can override config-file values:

| Variable | Purpose |
| --- | --- |
| `DUSHENGCDN_SERVER_URL` | Control plane URL |
| `DUSHENGCDN_AGENT_TOKEN_FILE` | File containing the node-specific Agent Token; preferred |
| `DUSHENGCDN_DISCOVERY_TOKEN_FILE` | File containing the first-registration Discovery Token; preferred |
| `DUSHENGCDN_AGENT_TOKEN` | Node-specific Agent Token; compatibility variable, avoid in shared environments |
| `DUSHENGCDN_DISCOVERY_TOKEN` | First-registration Discovery Token; compatibility variable, avoid in shared environments |
| `DUSHENGCDN_NODE_NAME` | Node name |
| `DUSHENGCDN_NODE_IP` | Node IP |
| `DUSHENGCDN_DATA_DIR` | Agent data directory |
| `DUSHENGCDN_OPENRESTY_PATH` | OpenResty binary path |
| `DUSHENGCDN_GEOIP_DATABASE_URL` | Agent GeoIP Country database download URL |
| `DUSHENGCDN_GEOIP_DATABASE_PATH` | Agent local GeoIP database path |
| `DUSHENGCDN_OPENRESTY_GEOIP_DATABASE_PATH` | GeoIP database path used by OpenResty/Lua |
| `DUSHENGCDN_GEOIP_UPDATE_INTERVAL` | Agent GeoIP database update interval |
| `DUSHENGCDN_GEOIP_LOOKUP_API_URL` | Optional precise online IP lookup API URL |
| `DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN_FILE` | File containing the optional precise IP lookup API bearer token; preferred |
| `DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN` | Optional precise IP lookup API bearer token; compatibility variable, avoid in shared environments |
| `DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT` | Optional precise IP lookup API timeout |
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
| `openresty_observability_port` | Local observability and OpenResty health-check port; keep it loopback-only and never publish it publicly | no | `18081` |
| `docker_binary` | Deprecated Docker-control field, read for compatibility only | no | empty |
| `data_dir` | Agent data directory | no | `data` under config directory |
| `access_log_path` | OpenResty access log path | no | `data_dir/var/log/dushengcdn/access.log` |
| `runtime_config_dir` | Runtime config directory, including `pow_config.json` | no | `data_dir/etc/dushengcdn` |
| `geoip_database_url` | GeoIP Country database download URL | no | public GeoLite2 mirror |
| `geoip_database_path` | Agent local GeoIP database path | no | `data_dir/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb` |
| `openresty_geoip_database_path` | GeoIP database path used by OpenResty/Lua | no | same as `geoip_database_path` |
| `geoip_update_interval` | GeoIP database update interval | no | `24h` |
| `geoip_lookup_api_url` | Optional precise online IP lookup API URL, used only when local GeoIP has no country | no | empty |
| `geoip_lookup_api_token_file` | File containing the optional precise IP lookup API bearer token; preferred | no | empty |
| `geoip_lookup_api_token` | Optional precise IP lookup API bearer token; compatibility field, avoid persisting plaintext | no | empty |
| `geoip_lookup_api_timeout` | Optional precise IP lookup API timeout | no | `250ms` |
| `heartbeat_interval` | Heartbeat interval | no | `10000` ms |
| `request_timeout` | HTTP timeout | no | `10000` ms |

`heartbeat_interval` and `request_timeout` accept milliseconds or Go duration strings.

## DNS Worker Environment and Flags

DNS Worker can be configured by environment variables or CLI flags. The install script writes these values into `/opt/dushengcdn-dns-worker/dns-worker.env` and creates `dushengcdn-dns-worker.service`.

| Environment Variable | Purpose | Default |
| --- | --- | --- |
| `DUSHENGCDN_DNS_WORKER_SERVER_URL` | Server URL | empty |
| `DUSHENGCDN_DNS_WORKER_ID` | DNS Worker ID from the panel; used to verify local identity before Agent-mediated updates | empty |
| `DUSHENGCDN_DNS_WORKER_TOKEN_FILE` | File containing the DNS Worker Token; preferred | empty |
| `DUSHENGCDN_DNS_WORKER_TOKEN` | DNS Worker Token; compatibility variable, avoid in shared environments | empty |
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
| `--worker-id` | DNS Worker ID from the panel, used to bind Agent-mediated updates to this local install | environment variable |
| `--token-file` | Read DNS Worker Token from a file | environment variable |
| `--token` | DNS Worker Token; compatibility option. Rejected by default unless `--allow-insecure-token-argv` is explicitly passed | environment variable |
| `--listen` | UDP/TCP listen address | `:53` |
| `--snapshot-path` | Local snapshot cache path | `data/dns-worker-snapshot.json` |
| `--heartbeat-interval` | Heartbeat and snapshot pull interval | `10s` |
| `--request-timeout` | Server request timeout | `10s` |
| `--snapshot-max-age` | Maximum dynamic-answer snapshot age | `5m` |
| `--query-rate-limit` | Per-source-IP DNS queries per second; `0` disables | `200` |
| `--udp-response-size` | Maximum UDP response payload size | `1232` |
| `--geoip-database` | Optional local MaxMind Country MMDB path | empty |

DNS Worker heartbeats report GeoIP Country database status to Server. If GeoIP is not loaded, source CIDR and `global` scheduling still work, but country-code GSLB pools will not match.
