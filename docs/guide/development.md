# 本地开发

你会学到：如何搭建 DuShengCDN 的本地开发环境、启动 Server、Agent 和管理端前端，运行测试与构建命令，并理解贡献代码前需要遵守的边界。

本页面向贡献者。产品边界、数据模型约束、API 约定和前端分层规范以 [开发约束](../design/development.md) 为准；本页只提供可执行的本地开发流程。

## 仓库结构

| 路径 | 职责 |
| --- | --- |
| `dushengcdn_server` | Gin + GORM + SQLite/PostgreSQL 单体控制面 |
| `dushengcdn_server/cmd/dns-worker` | 自建权威 DNS Worker 运行入口 |
| `dushengcdn_server/web` | Next.js 管理端前端，静态导出后由 Go Server 托管 |
| `dushengcdn_agent` | Go 单体 Agent，运行在节点侧 |
| `scripts` | Agent 安装与卸载脚本 |
| `docs` | VitePress 文档站 |

## 环境要求

| 项目 | 要求 |
| --- | --- |
| Go | `1.26.4+` |
| Node.js | `18+` |
| pnpm | 推荐通过 `corepack enable` 使用项目声明版本 |
| Docker | Server 容器、本地联调和 Agent Docker 镜像需要 |
| OpenResty | 本地运行 Agent 时需要可执行 `openresty` |
| PostgreSQL | 可选；未配置时 Server 使用 SQLite |

## 初始化前端依赖

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
```

构建供 Go Server 托管的静态产物：

```bash
pnpm build
```

## 启动 Server

SQLite 模式：

```bash
cd dushengcdn_server
export SESSION_SECRET='dev-session-secret'
export SQLITE_PATH='./dushengcdn-dev.db'
export LOG_LEVEL='debug'
go run .
```

PostgreSQL 模式：

```bash
cd dushengcdn_server
export SESSION_SECRET='dev-session-secret'
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
export LOG_LEVEL='debug'
go run .
```

默认访问地址：

```text
http://localhost:3000
```

首次空库启动会创建 `root` 用户。密码使用 `.env` 中的 `DUSHENGCDN_INITIAL_ROOT_PASSWORD`；如果没有配置该值，则查看 Server 日志提示的 `initial-root-password.txt` 文件，日志不会打印密码本身。

## 启动前端开发服务器

前端开发服务器默认监听 `3001`，并通过 `NEXT_DEV_BACKEND_URL` 代理到后端：

```bash
cd dushengcdn_server/web
export NEXT_DEV_BACKEND_URL='http://127.0.0.1:3000'
pnpm dev
```

访问：

```text
http://localhost:3001
```

## 启动 Agent

创建本地 `agent.json`：

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "./data",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

运行：

```bash
cd dushengcdn_agent
export LOG_LEVEL='debug'
go run ./cmd/agent -config ./agent.json
```

未配置 `openresty_path` 时，Agent 默认调用 `openresty`。调试时可显式配置 `openresty_path`、`main_config_path`、`route_config_path`、`access_log_path`、`cert_dir`、`lua_dir` 和 `runtime_config_dir`。

## 启动 DNS Worker

DNS Worker 从 Server 拉取只读调度快照，并监听 UDP/TCP DNS 查询。普通本地开发可先监听高位端口，避免占用系统 `53`：

```bash
cd dushengcdn_server
export LOG_LEVEL='debug'
go run ./cmd/dns-worker \
  --server-url http://127.0.0.1:3000 \
  --token-file /run/secrets/dushengcdn-dns-worker-token \
  --listen 127.0.0.1:1053 \
  --snapshot-path ./data/dns-worker-snapshot.json \
  --query-rate-limit 200 \
  --udp-response-size 1232
```

验证：

```bash
dig @127.0.0.1 -p 1053 example.com SOA
dig @127.0.0.1 -p 1053 www.example.com A
```

如果要测试按国家代码匹配 GSLB 节点池，可追加 `--geoip-database /path/to/GeoLite2-Country.mmdb`。如果节点池配置了来源 CIDR，可直接用带 EDNS Client Subnet 的查询验证 `cidr:...` 作用域；未配置本地 GeoIP 库且未命中来源 CIDR 时，Worker 不调用外部 HTTP GeoIP API，来源作用域会回退为 `global`。

## 测试

Server：

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

商业配额的 Postgres 行锁集成测试默认跳过；需要验证真实 PostgreSQL 并发语义时，提供独立测试库 DSN 后运行：

```bash
cd dushengcdn_server
POSTGRES_TEST_DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn_test?sslmode=disable' go test ./service -run TestPostgresCommercialLicenseQuotaSerializesConcurrentNodeCreates -count=1
```

DNS Worker 单包：

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./internal/dnsworker
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
pnpm test:e2e
```

Docs：

```bash
cd docs
corepack enable
pnpm install --frozen-lockfile
pnpm build
```

GitHub Actions 里的 `Docs CI` 会在 `docs/**` 或文档构建 workflow 变化时执行同样的安装与 `pnpm build`，用于检查新增页面、侧边栏和 VitePress 构建是否仍然有效。

## 构建

管理端静态产物：

```bash
cd dushengcdn_server/web
pnpm build
```

Server 二进制：

```bash
cd dushengcdn_server
go build -o dushengcdn-server .
```

Agent 二进制：

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
```

DNS Worker 二进制：

```bash
cd dushengcdn_server
go build -o dushengcdn-dns-worker ./cmd/dns-worker
```

## 调试入口

| 场景 | 命令或位置 |
| --- | --- |
| Server 日志 | `LOG_LEVEL=debug go run .` |
| Agent 日志 | `LOG_LEVEL=debug go run ./cmd/agent -config ./agent.json` |
| DNS Worker 日志 | `LOG_LEVEL=debug go run ./cmd/dns-worker --listen 127.0.0.1:1053 ...` |
| Swagger | `http://localhost:3000/swagger/index.html` |
| 前端 API 代理 | `NEXT_DEV_BACKEND_URL=http://127.0.0.1:3000 pnpm dev` |
| OpenResty 配置校验 | `openresty -t -c ./data/etc/nginx/nginx.conf` |

## 代码风格与变更准入

贡献前先确认：

1. 需求符合 [产品边界](../design/index.md)。
2. 实现符合 [开发约束](../design/development.md)。
3. 不破坏发布、同步、回滚或升级主链路。
4. 涉及配置、部署、API 或产品边界时同步更新文档。
5. 风险较高的修改补充测试或等效联调验证。

数据库结构变更必须提升数据库版本号，并补充从上一版本到新版本的显式迁移方法和校验逻辑。
