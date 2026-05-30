# 升级与维护

你会学到：如何升级 Server 与 Agent、如何清理观测数据，以及维护前后应该执行哪些验证命令。

升级前建议先确认当前激活版本、最近一次 Agent 应用结果和数据库备份策略。生产环境不要在发布配置、Agent 大规模重连或数据库迁移进行中同时升级。

## Server 升级

Root 用户可以在管理端顶栏检查并升级 Server 正式版。也可以通过上传 Server 二进制的方式执行确认升级。

如需尝试 preview 版本，可手动检查对应发布。生产环境建议优先使用正式版。

源码目录 + Docker Compose 部署时，推荐流程：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
docker compose up -d --build
docker compose ps
```

如果服务器上的 Compose 文件曾经手动改过端口、密码或 DSN，`git pull --ff-only` 可能被本地改动阻塞。确认没有需要保留的源码修改后，可以先备份本地部署参数，再执行：

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
docker compose up -d --build
docker compose ps
```

为了减少后续冲突，建议把本地端口映射、数据库密码、`SESSION_SECRET` 和 DSN 保存到独立部署记录或 Compose override 中，而不是长期直接改仓库模板。

升级后确认：

```bash
docker compose ps
docker compose logs -n 100 dushengcdn
```

如果是源码部署，重新启动 Server 后确认日志中没有数据库迁移或启动错误。

## Agent 升级

节点 Agent 默认只跟随正式版自动更新。preview 升级需要手动触发。

安装脚本可重复执行，用于重装或升级 Agent：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

注意：当前安装脚本重装时会删除整个安装目录，包括旧 `agent.json`、本地状态、缓存数据和下载的二进制。执行前请确认手头仍有可用 Token。

升级后确认：

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -n 100 --no-pager
```

## 数据维护

管理端设置页可以维护观测数据自动清理策略：

| 配置项 | 说明 |
| --- | --- |
| `DatabaseAutoCleanupEnabled` | 是否启用每日自动清理 |
| `DatabaseAutoCleanupRetentionDays` | 自动清理保留天数，至少 1 天 |

开启后，Server 会在每天凌晨 3 点清理访问日志、指标快照与请求报告。

## 备份与恢复

升级前至少备份数据库和上传目录。

PostgreSQL Compose 部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
docker compose exec -T postgres pg_dump -U dushengcdn -d dushengcdn > backups/dushengcdn-$(date +%F-%H%M%S).sql
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

SQLite 部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
cp dushengcdn-data/dushengcdn.db backups/dushengcdn-$(date +%F-%H%M%S).db
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

恢复时先停止 Server，再恢复数据库和上传目录，最后启动 Server 并检查日志、版本页面、节点详情和应用记录。

## 常用验证命令

Server：

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

Agent：

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

Frontend：

```bash
cd dushengcdn_server/web
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Docs：

```bash
cd docs
pnpm build
```
