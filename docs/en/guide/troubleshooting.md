# Troubleshooting

You will learn how to debug DuShengCDN Server, database, login, Agent, OpenResty, release, and frontend build issues by symptom.

Start by locating the failing layer: browser, Server, database, Agent, OpenResty, origin, or DNS. DuShengCDN applies configuration only after a version is activated and the Agent discovers it through heartbeat.

## Quick Triage

| Symptom | Check First |
| --- | --- |
| Management UI does not open | Server process/container logs and port binding |
| Login fails | Default account, `SESSION_SECRET`, browser request, Server logs |
| Data cannot be saved | Database connection, SQLite permissions, PostgreSQL health |
| Agent is offline | Agent logs, token, Server URL, network reachability |
| Node does not update after release | Active version, node heartbeat, apply logs |
| OpenResty apply fails | Apply logs, Agent logs, certificates, upstream URL, port conflicts |
| No access analytics | OpenResty status, observability port, Agent replay logs |
| Automatic DNS does not switch | Node pool, public IP pool, scheduling switch, drain mode, Cloudflare Token permissions |
| Authoritative DNS resolves incorrectly after migration | Migration wizard retest, Zone delegation check, Worker public UDP/TCP 53 probe, Agent multi-node probes, GSLB simulation |
| Cache operation fails | Agent WebSocket connection, site node pool, OpenResty cache path |

## Server Does Not Start

1. Check logs:

```bash
docker compose logs -n 200 dushengcdn
```

For source runs, check terminal output.

When deployed or upgraded with `scripts/install-server.sh`, the script verifies that the `dushengcdn` Compose service is still running after `docker compose up` and checks `SERVER_URL/api/status`. If the service exits or the HTTP check fails, the script prints recent logs and hints for common PostgreSQL password/DSN, database connection, port binding, host-port, and reverse-proxy upstream failures.

2. Check port usage:

```bash
lsof -i :3000
```

3. If PostgreSQL is used, check database health:

```bash
docker compose ps postgres
docker compose logs -n 100 postgres
```

4. If SQLite is used, check that the database directory is writable:

```bash
ls -ld "$(dirname /path/to/dushengcdn.db)"
```

Common causes:

| Log or Symptom | Fix |
| --- | --- |
| Database connection failed | Check username, password, host, port, database, and `sslmode` in `DSN` |
| `password authentication failed for user "dushengcdn"` | `POSTGRES_PASSWORD` / `DSN` do not match the password stored in the existing PostgreSQL data directory. If this happened after running `bash scripts/install-server.sh` on an old source deployment, restore the old password and DSN in `.env`, then restart Compose. |
| SQLite cannot create file | Check that the `SQLITE_PATH` directory exists and is writable |
| Port is already in use | Change `PORT` or `--port`, or stop the process using the port |

If `docker compose up -d --build` takes a long time, separate build time from startup time: `load build context` is Docker packaging the source directory, `pnpm build` / `go build` are compilation steps, and `Container ... Started` is the actual container startup. When `load build context` is several GB, runtime data such as `postgres-data`, `dushengcdn-data`, `backups`, `upload`, or `logs` has likely entered the build context; these paths should be excluded by `.dockerignore`. Clean up or move those directories and rebuild. Production data is still mounted through Compose volumes and does not need to be copied into the image.

If an old source deployment did not have `.env`, and the first installer run generated a new random database password while `postgres-data` already existed, restore the old default value first:

```bash
cd /opt/dushengcdn/dushengcdn_server
sed -i 's/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=replace-with-strong-password/' .env
sed -i 's#^DSN=.*#DSN=postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable#' .env
docker compose --env-file .env up -d --build
docker compose --env-file .env logs --tail=100 dushengcdn
```

If you had manually set a different PostgreSQL password, replace `replace-with-strong-password` with the real old password.

## UI Does Not Open or Is Blank

For source Compose deployments or upgrades, run the read-only diagnostic helper from the repository root first:

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

The helper reads `dushengcdn_server/.env`, shows Compose service state, checks `SERVER_URL/api/status`, lists listeners for the host panel port and `3000`, prints recent `dushengcdn` / `postgres` logs, and highlights common database authentication, port binding, and reverse-proxy upstream-port issues. It does not restart services or edit configuration.

1. Confirm that the Server responds. Source Compose defaults to the host port from `.env` (`DUSHENGCDN_HTTP_PORT=3010`), while the container listens on `3000`:

```bash
cd /opt/dushengcdn/dushengcdn_server
panel_port="$(grep -E '^DUSHENGCDN_HTTP_PORT=' .env | tail -n1 | cut -d= -f2-)"
curl -I "http://127.0.0.1:${panel_port:-3010}/api/status"
curl -I http://127.0.0.1:3000/api/status
```

2. For source runs, confirm frontend static assets were built:

```bash
cd dushengcdn_server/web
pnpm build
```

3. Check whether the browser URL matches your reverse proxy setup. Nginx, Nginx Proxy Manager, Baota, or another reverse proxy should point to the local host-mapped port, for example `3010` for the default source Compose deployment. Direct `3000` access only applies if you manually changed the panel to a public mapping such as `0.0.0.0:3000:3000`, which is not recommended for production.

4. If using the frontend dev server, confirm backend proxy configuration:

```bash
cd dushengcdn_server/web
NEXT_DEV_BACKEND_URL=http://127.0.0.1:3000 pnpm dev
```

## Default Account Cannot Sign In

On an empty database, the first-login username is `root`; use `DUSHENGCDN_INITIAL_ROOT_PASSWORD` from `.env`, or read the generated one-time password from the `initial-root-password.txt` file named in the Server log. The log prints the file path, not the password. If the password was changed after first login, use the updated password.

Steps:

1. Confirm the Server is connected to the expected database, not another `SQLITE_PATH` or `DSN`.
2. Check Server logs to see whether it uses `sqlite` or `postgres`.
3. If deployed behind replicas or a reverse proxy, ensure `SESSION_SECRET` is fixed and consistent across instances.
4. Clear browser cookies and try again.
5. If you still have server access, reset the root password offline.

Docker Compose deployment:

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
install -m 0600 /dev/stdin /tmp/dushengcdn-root-password <<'EOF'
replace-with-new-password
EOF
docker compose run --rm -v /tmp/dushengcdn-root-password:/run/secrets/dushengcdn-root-password:ro dushengcdn /dushengcdn --reset-root-password-file /run/secrets/dushengcdn-root-password
rm -f /tmp/dushengcdn-root-password
docker compose up -d
```

Source deployment:

```bash
cd /opt/dushengcdn/dushengcdn_server
export DSN='postgres://dushengcdn:password@127.0.0.1:5432/dushengcdn?sslmode=disable'
install -m 0600 /dev/stdin /run/secrets/dushengcdn-root-password <<'EOF'
replace-with-new-password
EOF
./dushengcdn --reset-root-password-file /run/secrets/dushengcdn-root-password
```

## Agent Cannot Register or Stays Offline

On the Agent node:

```bash
curl -I http://your-server:3000
```

Check Agent logs:

```bash
journalctl -u dushengcdn-agent -n 200 --no-pager
```

Check config:

```bash
sed -n '1,160p' /opt/dushengcdn-agent/agent.json
```

Confirm:

| Config | Notes |
| --- | --- |
| `server_url` | Must be reachable from the Agent node |
| `agent_token` / `discovery_token` | At least one is required |
| `heartbeat_interval` | Supports millisecond integers or Go duration strings |
| `request_timeout` | Increase it for slow networks |

If the log says the token is invalid, prepare a new token in the UI, update `agent.json` or the restricted token file, and restart. When logs contain `Agent authentication failed`, first check `agent_token` / `discovery_token`, `DUSHENGCDN_AGENT_TOKEN_FILE` / `DUSHENGCDN_DISCOVERY_TOKEN_FILE`, or the compatibility variables `DUSHENGCDN_AGENT_TOKEN` / `DUSHENGCDN_DISCOVERY_TOKEN`; first registration must use a Discovery Token, while heartbeat, config pull, and WebSocket should use the node-specific Agent Token.

```bash
systemctl restart dushengcdn-agent
```

## Node Does Not Apply a New Version

Check in order:

1. The target version is active on the versions page.
2. The node is online and heartbeat time is updating.
3. Apply logs contain a success, warning, or failure for the target version.
4. The site configuration is enabled.
5. Agent logs show pull, validation, reload, or rollback messages.

Follow Agent logs:

```bash
journalctl -u dushengcdn-agent -f
```

After a target `version + checksum` fails and rolls back, the Agent blocks repeated attempts for that same target locally. Fix the configuration and publish a new checksum, or activate an old version to roll back.

## OpenResty Apply Fails

Common causes:

| Cause | Check |
| --- | --- |
| Domain or server block conflict | Ensure the same domain is not used by multiple sites |
| Invalid upstream URL | Every upstream must be `http://` or `https://` |
| Invalid multi-upstream format | Multiple upstreams must be plain `scheme://host[:port]` |
| Missing certificate or wrong path | Check domain certificate binding and Agent certificate directory permissions |
| Port conflict | Check local `80` and `443` usage |

OpenResty config test:

```bash
openresty -t -c /path/to/dushengcdn/data/etc/nginx/nginx.conf
```

OpenResty runtime:

```bash
ps aux | grep openresty
```

Agent periodic health checks use local `http://127.0.0.1:<openresty_observability_port>/dushengcdn/stub_status` instead of repeatedly running `openresty -t`. If a node is unhealthy, first confirm that the local observability port is listening. If `host not found in upstream` only appears during apply, the failure comes from config validation or reload, not the periodic health probe.

Use the actual `openresty_path` and `main_config_path` from `agent.json`.

## Git Pull Is Blocked by Local Compose Changes

This usually means the server-side repository copy has local edits in Compose files, often for ports, database passwords, DSN, or tokens.

1. Record those local deployment parameters first.
2. If there are no source changes to keep, run `git fetch origin main && git reset --hard origin/main`.
3. Create `dushengcdn_server/.env` from `.env.example` and put real deployment values there.
4. Start with `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build`.

Port conflicts only require changing the host-side mapping, for example `3010:3000`; the container still listens on `3000`. Add the `127.0.0.1:` prefix only when the panel should be reachable through a same-host reverse proxy.

## Automatic DNS Does Not Switch Nodes

Check in order:

1. Cloudflare DNS Token has `Zone Read` and `DNS Edit` permissions.
2. The site has automatic DNS enabled and is bound to the intended node pool.
3. The target node pool has online nodes and OpenResty is not unhealthy.
4. Node public IP pools contain addresses matching the record type: IPv4 for `A`, IPv6 for `AAAA`.
5. Nodes are not disabled for scheduling and are not in drain mode.

## Authoritative DNS Resolves Incorrectly After Migration

Open **Authoritative DNS** -> migration wizard and check the post-switch retest:

1. **Site DNS mode** should show the target Zone. If not, open the site detail **Automatic DNS** section and confirm DNS mode and Zone binding.
2. **Zone delegation check** should match registrar NS records. If it is partial, mismatched, or mentions Glue, update the registrar NS or host records.
3. **Worker public probe** should show at least one online Worker reachable over both UDP and TCP `53`.
4. **Worker snapshot consistency** should show reachable Workers holding a non-expired snapshot under `AuthoritativeDNSSnapshotMaxAge`, with matching versions.
5. **GSLB simulation** should return target IPs. If it returns no target, inspect node pool, online state, OpenResty health, public IP type, drain mode, GSLB weights, load thresholds, and probe gate reasons.

If `dig @PUBLIC_IP example.com SOA` or `dig @PUBLIC_IP example.com NS` says `connection refused` or `no servers could be reached`:

First run the read-only diagnostic helper on the DNS Worker host:

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com
```

The helper checks `dushengcdn-dns-worker.service`, the install directory, `dns-worker.env`, listeners, snapshot file, GeoIP file, and recent logs. When `--public-ip` and `--zone` are provided, it also runs UDP/TCP SOA/NS queries. It does not restart services or edit configuration.
If the panel and DNS Worker run on the same host and you want the full pre-production checklist, run:

```bash
cd /opt/dushengcdn
bash scripts/verify-authoritative-dns.sh --public-ip PUBLIC_IP --zone example.com
```

1. Run `systemctl status dushengcdn-dns-worker`. `Unit dushengcdn-dns-worker.service could not be found` means the panel Zone/registrar NS may exist, but no DNS Worker is deployed on that host.
2. Run `ss -lntup | grep ':53'` and `ss -lnuap | grep ':53'`. Seeing only `systemd-resolved` on `127.0.0.53` or `127.0.0.54` does not mean public port `53` has an authoritative service.
3. Create a DNS Worker Token under **Authoritative DNS**, then install the Worker. If Worker and Server run on the same host, use a Server URL reachable from that host and bind the public address with `--listen PUBLIC_IP:53`.
4. Check `journalctl -u dushengcdn-dns-worker -n 100 --no-pager` for Token, Server URL, or snapshot errors.
5. Ensure firewall, cloud security group, and upstream network allow both UDP and TCP `53`.
6. If NS names are inside the same Zone, configure Glue/host records at the registrar.

If SOA/NS return `NOERROR` but site `A`/`AAAA` records have no targets or Worker logs show `routes=0`:

1. The Worker is serving the Zone, but no site is bound to authoritative DNS in the snapshot.
2. Open site detail -> **Automatic DNS**, switch DNS mode to **Authoritative DNS**, and select the Zone, or use the migration wizard one-click switch.
3. Confirm the site domain belongs to that Zone and does not conflict with enabled static `A`, `AAAA`, or `CNAME` records.
4. Wait for the next Worker heartbeat/snapshot pull or restart the Worker, then check that `routes` increases.
5. Use GSLB simulation to inspect no-target reasons.

If enabling a site or running migration says there is no online DNS Worker, or online Workers have not passed public UDP/TCP `53` probing:

1. Create and deploy at least one DNS Worker with a Worker Token.
2. Confirm the Worker heartbeat is online and its public address is configured.
3. Click **Probe** in the DNS Worker list and confirm both UDP and TCP `53` pass.
4. Re-check firewall, security groups, NAT, and port mapping.

If DNS Worker install fails with port `53` already in use:

1. Run `ss -lntu '( sport = :53 )'` or `lsof -nP -i :53`.
2. Common conflicts are `systemd-resolved`, `named`, `dnsmasq`, or an existing DNS service.
3. If the existing service only binds loopback, and the Worker only needs the public address, install with `--listen PUBLIC_IP:53`.
4. For local development only, use a high port such as `--listen 127.0.0.1:1053` and test with `dig @127.0.0.1 -p 1053 example.com SOA`.

If the migration wizard reports stale or inconsistent Worker snapshots:

1. Confirm at least one Worker is online and the latest public UDP/TCP probe is healthy.
2. Check snapshot consistency for non-empty matching `last_snapshot_version` and fresh `last_snapshot_at`.
3. Inspect Worker logs for invalid Token, unreachable Server URL, TLS trust failures, or snapshot API errors. When logs contain `DNS Worker Token authentication failed`, first check `DUSHENGCDN_DNS_WORKER_TOKEN_FILE` / `--token-file`, or the compatibility `DUSHENGCDN_DNS_WORKER_TOKEN` / `--token`.
4. Confirm the Token is a DNS Worker Token, not an Agent Token or login password.
5. Restart the Worker or wait for the next heartbeat after fixing the issue.

If Server-side Worker probes pass but Agent multi-node probes fail:

1. Confirm Agent nodes can reach Worker public UDP/TCP `53`, for example `dig @ns1.example.net example.com SOA`.
2. Check outbound firewalls or provider policies blocking UDP/TCP `53`.
3. Confirm Agent heartbeat is healthy; probes are reported with heartbeat/status payloads after Server sends probe targets.
4. If the probe gate is enabled in authoritative DNS runtime settings, failed or stale probes can exclude nodes from authoritative DNS GSLB candidates.

## Cache Purge or Warmup Fails

Cache runtime operations are sent over Agent WebSocket. Check:

1. Target nodes are online and WebSocket is available.
2. The site is bound to the intended node pool.
3. Nodes are not in drain mode.
4. `OpenRestyCachePath` is configured and published.
5. Agent logs contain `agent cache purge failed` or `agent cache warm failed`.

## HTTPS Does Not Work

1. Confirm the certificate exists.
2. Confirm the domain is bound to that certificate in the site configuration.
3. Confirm a new version was published and activated.
4. Check apply logs for success.
5. Inspect with `curl`:

```bash
curl -Iv https://your-domain
```

Domains without a bound certificate are not automatically added to HTTPS configuration.

## No Access Analytics

1. Confirm the node applied a configuration that includes observability Lua assets.
2. Confirm OpenResty is running.
3. Check Agent logs for collection or replay failures.
4. Check whether `openresty_observability_port` is occupied. The default is `18081`.
5. Confirm Server cleanup policy did not remove data for that time window.

## Frontend Build Fails

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Common causes:

| Symptom | Fix |
| --- | --- |
| pnpm version mismatch | Run `corepack enable` and reinstall |
| Type errors | Run `pnpm typecheck` to locate files |
| API type mismatch | Check `lib/api/` and `types/` response structures |
| E2E fails | Ensure both the Server and frontend dev server are running |

## Docs Build Fails

```bash
cd docs
pnpm install
pnpm build
```

If the failure is a link error, check that new pages are added to `docs/en/config.ts` and that relative links point to existing Markdown files.
