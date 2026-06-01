# DuShengCDN Compose Examples

这些模板用于把不同部署角色拆清楚，避免直接修改仓库里的正式 Compose 文件。

## 文件说明

| 文件 | 用途 |
| --- | --- |
| `server.production.yaml` | 使用 GHCR 镜像运行 Server + PostgreSQL，适合有 Release 镜像时的生产部署 |
| `server.source.yaml` | 从当前仓库源码构建 Server，适合源码部署和 `git pull` 后重建 |
| `server.override.example.yaml` | 覆盖端口、数据目录和日志级别的示例 |
| `server.env.example` | Server 环境变量示例 |
| `agent.yaml` / `agent.env.example` | Docker Compose 方式部署 Agent |
| `dns-worker.yaml` / `dns-worker.env.example` | Docker Compose 方式部署 DNS Worker |

## Server 镜像部署

```bash
mkdir -p /opt/dushengcdn-compose
cd /opt/dushengcdn-compose
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.production.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.env.example
vi .env
docker compose --env-file .env -f server.production.yaml up -d
docker compose --env-file .env -f server.production.yaml ps
```

生产环境建议保留默认 `DUSHENGCDN_HTTP_BIND=127.0.0.1`，再用 Nginx、OpenResty、宝塔或其它反向代理提供 HTTPS。

## Server 源码部署

在仓库根目录执行：

```bash
cp -n examples/compose/server.env.example dushengcdn_server/.env
vi dushengcdn_server/.env
DUSHENGCDN_REPO_DIR="$PWD" \
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" \
  docker compose --env-file dushengcdn_server/.env -f examples/compose/server.source.yaml up -d --build
```

源码模板把 PostgreSQL 数据写到 `dushengcdn_server/postgres-data`，把 Server 数据写到 `dushengcdn_server/dushengcdn-data`，与仓库内备份脚本默认路径一致。

## 覆盖端口或数据目录

复制 `server.override.example.yaml` 到部署目录，按需修改后叠加：

```bash
docker compose --env-file .env \
  -f server.production.yaml \
  -f server.override.yaml \
  up -d
```

## Agent

```bash
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/agent.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/agent.env.example
vi .env
docker compose --env-file .env -f agent.yaml up -d
```

## DNS Worker

```bash
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/dns-worker.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/dns-worker.env.example
vi .env
docker compose --env-file .env -f dns-worker.yaml up -d
```

DNS Worker 需要公网 UDP/TCP `53` 可达。生产环境至少部署两个 Worker，并在注册商配置多个 NS。
