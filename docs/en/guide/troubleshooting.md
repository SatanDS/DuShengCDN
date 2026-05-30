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

## Server Does Not Start

1. Check logs:

```bash
docker compose logs -n 200 dushengcdn
```

For source runs, check terminal output.

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
| SQLite cannot create file | Check that the `SQLITE_PATH` directory exists and is writable |
| Port is already in use | Change `PORT` or `--port`, or stop the process using the port |

## UI Does Not Open or Is Blank

1. Confirm that the Server responds:

```bash
curl -I http://127.0.0.1:3000
```

2. For source runs, confirm frontend static assets were built:

```bash
cd dushengcdn_server/web
pnpm build
```

3. Check whether the browser URL matches your reverse proxy setup.

4. If using the frontend dev server, confirm backend proxy configuration:

```bash
cd dushengcdn_server/web
NEXT_DEV_BACKEND_URL=http://127.0.0.1:3000 pnpm dev
```

## Default Account Cannot Sign In

The default account is `root` / `123456`. If the password was changed after first login, use the updated password.

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
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
```

Source deployment:

```bash
cd /opt/dushengcdn/dushengcdn_server
export DSN='postgres://dushengcdn:password@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn --reset-root-password 'replace-with-new-password'
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

If the log says the token is invalid, prepare a new token in the UI, update `agent.json`, and restart:

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
3. Reapply local deployment parameters through an external deployment note or Compose override.

Port conflicts only require changing the host-side mapping, for example `8080:3000`; the container still listens on `3000`.

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
