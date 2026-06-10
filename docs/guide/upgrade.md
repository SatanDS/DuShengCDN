# 升级与维护

你会学到：如何升级 Server 与 Agent、如何清理观测数据，以及维护前后应该执行哪些验证命令。

升级前建议先确认当前激活版本、最近一次 Agent 应用结果和数据库备份策略。生产环境不要在发布配置、Agent 大规模重连或数据库迁移进行中同时升级。

## Server 升级

Root 用户可以在管理端顶栏检查 Server 正式版。Server 自动升级默认关闭，生产环境推荐上传已审阅的 Server 二进制后确认升级；如需允许一键自动升级，设置 `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true`，并确保 Release 包含当前平台 Server 二进制、同名 `.sha256` 校验文件和 `.sig` 签名文件。

如需尝试 preview 版本，可手动检查对应发布。生产环境建议优先使用正式版。

源码目录 + Docker Compose 部署时，推荐流程：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

首次使用源码 Compose 模板时，把真实配置写入 `dushengcdn_server/.env`，例如 `DUSHENGCDN_HTTP_PORT`、`POSTGRES_PASSWORD`、`SESSION_SECRET` 和 `DSN`。只有升级兼容旧 Agent 时才保留 `AGENT_TOKEN` 并临时设置 `DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED=true`。后续升级只拉取仓库模板，`.env` 继续保留在服务器本地。

如果服务器上的 Compose 文件曾经手动改过端口、密码或 DSN，`git pull --ff-only` 可能被本地改动阻塞。确认没有需要保留的源码修改后，可以先把本地部署参数迁移到 `.env`，再执行：

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

为了减少后续冲突，建议把本地端口映射、数据库密码、`SESSION_SECRET` 和 DSN 保存到 `.env` 或 Compose override 中，而不是长期直接改仓库模板。
源码 Compose 构建时，`DUSHENGCDN_VERSION` 会写入 Server 或 Agent 二进制；管理端顶栏“版本”显示的是当前运行中的 Server 版本，节点列表显示 Agent 上报的版本。

升级后确认：

```bash
docker compose ps
docker compose logs -n 100 dushengcdn
```

如果升级命令执行完成但面板打不开，先运行只读诊断脚本收集端口、健康接口、Compose 状态和最近日志：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

源码 Compose 默认宿主机面板端口来自 `.env` 的 `DUSHENGCDN_HTTP_PORT=3010`，容器内才是 `3000`。如果诊断显示 `http://127.0.0.1:3010/api/status` 正常，但浏览器域名打不开，通常是 Nginx、Nginx Proxy Manager、宝塔或其它反向代理仍指向旧的 `127.0.0.1:3000`，应把上游改到 `.env` 中的宿主机端口。

如果是源码部署，重新启动 Server 后确认日志中没有数据库迁移或启动错误。

## Agent 升级

节点 Agent 默认只跟随正式版 GitHub Release 自动更新。只把代码推到 `main` 分支不会触发 Agent 自更新；仓库需要先发布新的正式 Release，并且 Release 中包含对应平台 Agent 二进制、同名 `.sha256` 校验文件和 `.sig` 签名文件。preview 升级需要手动触发。

安装脚本可重复执行，用于重装或升级 Agent：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token-file /run/secrets/dushengcdn-agent-token
```

脚本会优先下载 GitHub Release 中的 Agent 二进制；没有匹配资产时会从源码构建，并写入当前 Git 版本，避免节点版本显示为 `dev`。这个源码构建回退只发生在你手动重新执行安装脚本时，不等同于 Agent 自动跟随 `main` 分支更新。

Docker Compose 部署 Agent 时：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build
docker compose -f docker-compose.agent.yaml ps
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

升级前至少备份数据库和上传目录。源码部署可以直接使用仓库内的备份脚本：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

脚本默认读取 `dushengcdn_server/.env`，在 `auto` 模式下优先备份可访问的 Compose PostgreSQL，否则备份 SQLite 文件，并同时归档 `dushengcdn-data` 目录。备份文件会写入 `dushengcdn_server/backups/<timestamp>/`，并生成 `manifest.txt` 记录模式、路径和校验信息。脚本只创建备份，不会停止、恢复、覆盖或删除生产数据。

恢复时先停止 Server，再执行恢复脚本，最后启动 Server 并检查日志、版本页面、节点详情和应用记录。PostgreSQL Compose 部署只需要停止 `dushengcdn` 服务，保留 `postgres` 服务运行供脚本恢复数据库：

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

恢复脚本会校验备份 manifest 中的 SHA-256 信息，并在覆盖前把当前数据库和 `dushengcdn-data` 再备份到 `dushengcdn_server/backups/pre-restore/<timestamp>/`。默认会拒绝在 `dushengcdn` 服务仍运行时恢复；只有在确认运行态检查不适用时才加 `--force`。

PostgreSQL Compose 部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
docker compose exec -T postgres pg_dump -U dushengcdn -d dushengcdn > backups/dushengcdn-$(date +%F-%H%M%S).sql
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

SQLite 手工备份：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
cp dushengcdn-data/dushengcdn.db backups/dushengcdn-$(date +%F-%H%M%S).db
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

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
