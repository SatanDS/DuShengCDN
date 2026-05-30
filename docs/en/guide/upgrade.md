# Upgrade and Maintenance

You will learn: How to upgrade the Server and Agent, how to clean up observability data, and which verification commands to execute before and after maintenance.

Before upgrading, it is recommended to confirm the current activated version, the latest Agent application result, and the database backup policy. Do not upgrade in production environments while configuration publishing, large-scale Agent reconnection, or database migrations are in progress.

## Server Upgrade

Root users can check and upgrade the Server stable version from the top bar of the management console. Upgrades can also be confirmed and executed by uploading the Server binary.

To try a preview version, you can manually check the corresponding release. It is recommended to prioritize the stable version in production environments.

After upgrading, confirm:

```bash
docker compose ps
docker compose logs -n 100 dushengcdn
```

If it is a source deployment, confirm that there are no database migration or startup errors in the logs after restarting the Server.

## Agent Upgrade

Node Agents follow stable versions by default for automatic updates. Preview upgrades must be triggered manually.

The installation script can be executed repeatedly to reinstall or upgrade the Agent:

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
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
