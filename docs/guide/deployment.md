# 部署说明

你会学到：DuShengCDN 的推荐部署方式、Server 与 Agent 的运行要求、源码启动方式、联调步骤、升级与卸载入口。

生产环境建议使用 PostgreSQL 作为 Server 数据库，并为 Server 显式配置 `SESSION_SECRET`。Agent 统一通过 OpenResty 二进制控制运行时；Docker 部署请直接使用内置 OpenResty 的 Agent 镜像。

## 部署拓扑

```text
Browser
  |
  v
DuShengCDN Server :3000
  |
  | Agent API / heartbeat / config pull
  v
DuShengCDN Agent
  |
  v
OpenResty binary
  |
  v
Origin service
```

## 前置条件

Server：

| 项目 | 要求 |
| --- | --- |
| Go | `1.25+`，仅源码运行需要 |
| Node.js | `18+`，仅源码构建管理端需要 |
| 数据库 | 可写 SQLite 文件目录，或可访问的 PostgreSQL 实例 |
| 端口 | 默认监听 `3000` |

Agent：

| 项目 | 要求 |
| --- | --- |
| 系统 | 安装脚本支持 Linux 和 macOS；systemd 服务仅在 Linux + systemd 环境创建 |
| 架构 | `amd64` 或 `arm64` |
| OpenResty | 本地部署需要可执行 `openresty`，或通过 `--openresty-path` 指定路径 |
| Docker | 仅 Docker 部署 Agent 镜像时需要 |
| 网络 | Agent 节点必须能访问 Server 地址 |

[需要确认：生产环境推荐的最低 CPU、内存与磁盘容量]

## Docker Compose 部署 Server

创建 `docker-compose.yml`：

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: dushengcdn
      POSTGRES_USER: dushengcdn
      POSTGRES_PASSWORD: replace-with-strong-password
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dushengcdn -d dushengcdn"]
      interval: 10s
      timeout: 5s
      retries: 5

  dushengcdn:
    image: ghcr.io/satands/dushengcdn:latest
    container_name: dushengcdn
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "3000:3000"
    environment:
      SESSION_SECRET: replace-with-a-long-random-string
      DSN: postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
      GIN_MODE: release
      LOG_LEVEL: info
    volumes:
      - dushengcdn-data:/data

volumes:
  postgres-data:
  dushengcdn-data:
```

启动：

```bash
docker compose up -d
docker compose ps
docker compose logs -f dushengcdn
```

首次访问 `http://localhost:3000`，默认账号为 `root` / `123456`。登录后请立即修改默认密码。

## 源码启动 Server

先构建管理端前端：

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm build
```

再启动 Server：

```bash
cd dushengcdn_server
export SESSION_SECRET='replace-with-a-long-random-string'
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
# 可选：设置后优先使用 PostgreSQL。
# export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
go run .
```

默认监听 `3000` 端口。也可以显式指定：

```bash
go run . --port 3000 --log-dir ./logs
```

## Agent 接入

使用 `discovery_token` 自动注册：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

使用节点专属 `agent_token`：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

安装脚本支持参数：

| 参数 | 说明 |
| --- | --- |
| `--server-url` | Server 地址，必填 |
| `--discovery-token` | 首次自动注册 Token，与 `--agent-token` 二选一 |
| `--agent-token` | 节点专属 Token，与 `--discovery-token` 二选一 |
| `--install-dir` | 安装目录，默认 `/opt/dushengcdn-agent` |
| `--openresty-path` | OpenResty 二进制路径，未传时自动查找 `openresty` |
| `--repo` | 下载 Agent 的 GitHub 仓库，默认 `SatanDS/DuShengCDN` |
| `--no-service` | 不创建 systemd 服务 |

确认状态：

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -f
```

## Docker 运行 Agent

Docker 部署时直接运行 Agent 镜像。该镜像基于 OpenResty 镜像制作，内置 Agent 控制器与 OpenResty 二进制。

挂载配置文件：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -v dushengcdn-agent-data:/data \
  -v ./agent.json:/etc/dushengcdn/agent.json:ro \
  ghcr.io/satands/dushengcdn-agent:latest
```

使用环境变量：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:latest
```

## 手动运行 Agent

源码运行：

```bash
cd dushengcdn_agent
export LOG_LEVEL='info'
go run ./cmd/agent -config /path/to/agent.json
```

编译后二进制运行：

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
export LOG_LEVEL='info'
./dushengcdn-agent -config /path/to/agent.json
```

最小 `agent.json` 示例：

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "./data",
  "openresty_path": "openresty",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

未配置 `openresty_path` 时，Agent 默认调用 `openresty`。

默认情况下，Agent 在 HTTP 心跳成功后会尝试升级为 WebSocket。升级成功时，Server 发布或激活配置会立即通知 Agent；如果 WebSocket 无法建立或意外断开，Agent 会自动退回 HTTP 心跳同步。

## 最小联调步骤

1. 启动 Server 并完成首次登录。
2. 在管理端准备 `agent_token` 或 `discovery_token`。
3. 启动 Agent，并确认节点在线。
4. 新增一条启用的网站配置。
5. 发布并激活新版本。
6. 查看节点详情和应用记录，确认版本应用成功。
7. 访问绑定域名或用 `curl` 验证反代结果。

## 升级与卸载

Server：

* Root 用户可在管理端顶栏检查并升级正式版。
* 如需尝试 preview 版本，可手动检查对应发布。
* 也可通过上传 Server 二进制的方式执行确认升级。

Agent：

* Agent 默认只跟随正式版自动更新。
* Agent 自更新会要求 GitHub Release 同时包含目标二进制和同名 `.sha256` 校验文件，下载后必须通过 SHA-256 校验才会替换本地可执行文件。
* 安装脚本可重复执行，用于重装或升级 Agent。
* preview 升级需要手动触发。

卸载 Agent：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

卸载脚本会停止 Agent、删除 systemd 服务和安装目录，不会删除本机 OpenResty。

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
pnpm build
```

Swagger：

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```
