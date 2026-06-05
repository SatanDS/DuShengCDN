# 命令与脚本

你会学到：DuShengCDN Server、管理端前端、Agent、Swagger 和文档站的常用启动、构建、测试、安装与卸载命令。

## Server

源码启动：

```bash
cd dushengcdn_server
export SESSION_SECRET="$(openssl rand -hex 32)"
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

指定监听端口与日志目录：

```bash
go run . --port 3000 --log-dir ./logs
```

离线重置 root 密码：

```bash
cd dushengcdn_server
./dushengcdn-server --reset-root-password 'replace-with-new-password'
```

创建 DNS Worker 并输出本次创建的 Token：

```bash
cd dushengcdn_server
./dushengcdn-server \
  --create-dns-worker-name 'DNS服务响应端' \
  --create-dns-worker-public-address '203.0.113.10'
```

该命令只创建 Worker 身份并打印 Token，不启动 HTTP 服务，主要供 `scripts/install-server.sh` 在部署面板时自动安装同机 DNS Worker 使用。

源码 Compose 一体化部署面板和同机 DNS Worker：

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh --public-ip 203.0.113.10
```

脚本默认先检查本机是否已有 DNS Worker；发现已有 `dushengcdn-dns-worker.service`、同名 systemd unit 文件、安装目录、环境文件、同名 Docker 容器、Worker 进程或 DuShengCDN 监听 `53` 端口时，会跳过 Worker 自动创建和安装。只部署面板可加 `--skip-dns-worker`，确认要覆盖本机 Worker 配置时再加 `--force-dns-worker-reinstall`。

诊断源码 Compose 面板访问问题：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `--server-dir` | Server compose/source 目录，默认仓库内 `dushengcdn_server` |
| `--compose-file` | Docker Compose 文件，默认 `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose 环境文件，默认读取 `SERVER_DIR/.env` |
| `--server-url` | 要检查的 Server 地址，默认 `http://127.0.0.1:DUSHENGCDN_HTTP_PORT` |
| `--log-tail` | 每个 Compose 服务打印的日志行数，默认 `120` |
| `--curl-timeout` | 健康检查超时时间，默认 `5` 秒 |
| `--skip-logs` | 不打印 Compose 日志 |

该脚本只读收集 `.env` 端口、Compose 状态、`/api/status`、端口监听和最近日志，不会重启服务或修改配置。

Compose 模板：

| 模板 | 说明 |
| --- | --- |
| `examples/compose/server.production.yaml` | GHCR 镜像版 Server + PostgreSQL |
| `examples/compose/server.source.yaml` | 源码构建版 Server + PostgreSQL |
| `examples/compose/server.override.example.yaml` | 端口、数据目录和日志等级 override 示例 |
| `examples/compose/agent.yaml` | Agent Docker Compose 模板 |
| `examples/compose/dns-worker.yaml` | DNS Worker Docker Compose 模板 |

备份 Server 数据：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `--server-dir` | Server compose/source 目录，默认仓库内 `dushengcdn_server` |
| `--backup-dir` | 备份输出目录，默认 `SERVER_DIR/backups` |
| `--mode auto|postgres|sqlite` | 备份模式，默认 `auto` |
| `--compose-file` | Docker Compose 文件，默认 `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose 环境文件，默认读取 `SERVER_DIR/.env` |
| `--data-dir` | 需要归档的 Server 数据目录，默认 `SERVER_DIR/dushengcdn-data` |
| `--sqlite-path` | SQLite 数据库路径，默认按 `.env` 或 `DATA_DIR/dushengcdn.db` 推导 |
| `--postgres-service` | Compose PostgreSQL 服务名，默认 `postgres` |
| `--postgres-db` | PostgreSQL 数据库名，默认读取 `.env` 或 `dushengcdn` |
| `--postgres-user` | PostgreSQL 用户名，默认读取 `.env` 或 `dushengcdn` |

`auto` 模式会优先对可访问的 Compose PostgreSQL 执行 `pg_dump`，否则备份 SQLite 文件，并归档 `dushengcdn-data`。脚本会在备份目录写入 `manifest.txt`，但不会停止、恢复、覆盖或删除生产数据。

恢复 Server 数据：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
cd /opt/dushengcdn
bash scripts/restore-server.sh --backup-path dushengcdn_server/backups/20260601-120000 --yes
cd dushengcdn_server
docker compose up -d
```

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `--backup-path` | `backup-server.sh` 生成的备份目录，必填 |
| `--mode auto|postgres|sqlite` | 恢复模式，默认按 `manifest.txt` 或备份文件自动判断 |
| `--server-dir` | Server compose/source 目录，默认仓库内 `dushengcdn_server` |
| `--compose-file` | Docker Compose 文件，默认 `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose 环境文件，默认读取 `SERVER_DIR/.env` |
| `--data-dir` | 要恢复的 Server 数据目录，默认 `SERVER_DIR/dushengcdn-data` |
| `--sqlite-path` | SQLite 数据库目标路径，默认按目标 `.env` 或 `DATA_DIR/dushengcdn.db` 推导 |
| `--postgres-service` | Compose PostgreSQL 服务名，默认 `postgres` |
| `--postgres-db` | PostgreSQL 数据库名，默认读取 manifest、`.env` 或 `dushengcdn` |
| `--postgres-user` | PostgreSQL 用户名，默认读取 manifest、`.env` 或 `dushengcdn` |
| `--pre-restore-backup-dir` | 覆盖前安全备份目录，默认 `SERVER_DIR/backups/pre-restore` |
| `--skip-data-dir` | 只恢复数据库，不恢复 `dushengcdn-data` 归档 |
| `--skip-current-backup` | 覆盖前不创建当前数据安全备份，仅在已另行备份时使用 |
| `--force` | 跳过 Server 运行态保护，仅在 Compose 状态检查不适用时使用 |
| `--yes` | 确认覆盖当前数据，恢复必填 |

恢复脚本会优先校验 `manifest.txt` 中的 SHA-256 信息，默认拒绝在 Compose `dushengcdn` 服务仍运行时恢复，并在覆盖前备份当前数据库和数据目录。

诊断 DNS Worker：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip 203.0.113.10 --zone example.com
```

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `--install-dir` | DNS Worker 安装目录，默认 `/opt/dushengcdn-dns-worker` |
| `--service-name` | systemd 服务名，默认 `dushengcdn-dns-worker` |
| `--env-file` | DNS Worker 环境文件，默认 `INSTALL_DIR/dns-worker.env` |
| `--public-ip` | 用于 `dig` 查询的 Worker 公网 IP |
| `--zone` | 配合 `--public-ip` 查询 SOA/NS 的 Zone |
| `--dns-port` | DNS 查询和监听端口，默认从监听地址解析或使用 `53` |
| `--log-tail` | 打印的 journal 日志行数，默认 `120` |
| `--skip-logs` | 不打印 journal 日志 |

该脚本只读检查 systemd 服务、安装目录、环境文件、监听端口、快照、GeoIP、日志和 UDP/TCP SOA/NS 查询结果，不会重启服务或修改配置。

权威 DNS 同机部署闭环验收：

```bash
cd /opt/dushengcdn
bash scripts/verify-authoritative-dns.sh --public-ip 203.0.113.10 --zone example.com
```

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `--public-ip` | DNS Worker 公网 IP，必填 |
| `--zone` | 要查询 SOA/NS 的 Zone，必填 |
| `--server-dir` | Server compose/source 目录，默认仓库内 `dushengcdn_server` |
| `--compose-file` | Docker Compose 文件，默认 `SERVER_DIR/docker-compose.yaml` |
| `--env-file` | Compose 环境文件，默认 `SERVER_DIR/.env` |
| `--server-url` | 要检查的 Server 地址，默认 `http://127.0.0.1:DUSHENGCDN_HTTP_PORT` |
| `--dns-worker-install-dir` | DNS Worker 安装目录，默认 `/opt/dushengcdn-dns-worker` |
| `--dns-worker-service` | DNS Worker systemd 服务名，默认 `dushengcdn-dns-worker` |
| `--dns-port` | DNS 查询和监听端口，默认从 Worker 监听地址解析或使用 `53` |
| `--skip-logs` | 不打印服务日志 |

该脚本只读执行上线验收顺序：Server Compose、`/api/status`、DNS Worker systemd、安装文件、DNS 监听、快照文件和 UDP/TCP SOA/NS 查询。

测试：

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Frontend

开发：

```bash
cd dushengcdn_server/web
pnpm install
pnpm dev
```

构建静态产物：

```bash
cd dushengcdn_server/web
pnpm build
```

检查：

```bash
cd dushengcdn_server/web
pnpm lint
pnpm typecheck
pnpm test
```

## Agent

源码运行：

```bash
cd dushengcdn_agent
go run ./cmd/agent -config /path/to/agent.json
```

编译：

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
```

测试：

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## 安装 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

## 卸载 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

## Swagger

重新生成 Swagger 文档：

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

## Docs

本地预览：

```bash
cd docs
pnpm dev
```

构建：

```bash
cd docs
pnpm build
```
