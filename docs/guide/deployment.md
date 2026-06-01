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

DNS Worker（自建权威 DNS 运行角色）：

| 项目 | 要求 |
| --- | --- |
| 端口 | 对公网开放 UDP `53` 和 TCP `53` |
| 数量 | 生产至少 2 个 Worker，并在注册商配置多个 NS |
| 网络 | Worker 必须能通过 HTTPS 访问 Server 拉取只读调度快照 |
| 数据 | Worker 本地保存最后一次有效快照，缓存文件带 SHA-256 checksum 元数据，并携带可恢复的 GSLB 防抖状态，Server 短暂不可用时继续回答 |
| 安全 | 默认按来源 IP 限制 DNS 查询速率，并限制 UDP 响应大小，超大响应设置 TC 位回退 TCP |

推荐生产规格：

| 场景 | 建议规格 |
| --- | --- |
| 小规模管理端（1-5 个节点，访问明细保留 30 天以内） | 2 vCPU、2 GB 内存、20 GB 可用磁盘 |
| 中等规模管理端（10+ 节点或访问分析较多） | 4 vCPU、4 GB 内存、50 GB 以上可用磁盘 |
| PostgreSQL | 独立卷或独立数据库实例，并纳入常规备份 |
| Agent 节点 | 1 vCPU、512 MB 内存起步，按实际 OpenResty 流量、TLS 和缓存压力扩容 |

观测数据会持续增长。生产环境建议开启观测数据自动清理，并为 PostgreSQL 或 SQLite 文件目录配置外部备份。

## Docker Compose 部署 Server

仓库提供了可复制的 Compose 模板，集中放在 `examples/compose/`：

| 模板 | 用途 |
| --- | --- |
| `server.production.yaml` + `server.env.example` | 使用 GHCR 镜像运行 Server + PostgreSQL，适合生产部署 |
| `server.source.yaml` + `server.env.example` | 从当前仓库源码构建 Server，适合源码部署和升级验证 |
| `server.override.example.yaml` | 演示覆盖宿主机端口、数据目录和日志等级 |
| `agent.yaml` + `agent.env.example` | Docker Compose 方式部署 Agent |
| `dns-worker.yaml` + `dns-worker.env.example` | Docker Compose 方式部署 DNS Worker |

例如使用镜像模板部署 Server：

```bash
mkdir -p /opt/dushengcdn-compose
cd /opt/dushengcdn-compose
curl -fsSLO https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.production.yaml
curl -fsSLo .env https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/examples/compose/server.env.example
vi .env
docker compose --env-file .env -f server.production.yaml up -d
docker compose --env-file .env -f server.production.yaml ps
```

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

如果使用仓库内的源码 Compose 模板部署 Server，先复制环境变量模板：

```bash
cd /opt/dushengcdn/dushengcdn_server
cp -n .env.example .env
```

然后在 `.env` 中修改 `DUSHENGCDN_HTTP_PORT`、`POSTGRES_PASSWORD`、`SESSION_SECRET` 和 `DSN`。后续本地部署参数都保存在 `.env`，不要直接改仓库里的 `docker-compose.yaml`，这样升级时可以继续使用 `git pull --ff-only`。

如果宿主机 `3000` 端口已被占用，可以只改宿主机侧端口，例如：

```yaml
ports:
  - "3010:3000"
```

此时浏览器访问 `http://localhost:3010`，容器内部仍监听 `3000`。

也可以在仓库根目录使用一体化部署脚本。脚本会在 `.env` 不存在时从 `.env.example` 创建环境文件；全新部署会自动生成 `POSTGRES_PASSWORD`、`SESSION_SECRET` 和匹配的 `DSN`。如果升级旧源码部署且已存在 `dushengcdn_server/postgres-data`，脚本会保留 `.env.example` 中的数据库密码和 DSN，只生成 `SESSION_SECRET`，避免旧 PostgreSQL 数据目录因密码不一致导致面板连不上数据库。`docker compose up` 后，脚本会先确认 `dushengcdn` 服务仍在运行，再访问 `SERVER_URL/api/status` 做 HTTP 健康检查；检查失败时会打印最近日志，并提示数据库认证、端口映射和反向代理上游端口等常见原因。源码 Compose 默认宿主机访问端口是 `.env` 中的 `DUSHENGCDN_HTTP_PORT=3010`，容器内仍监听 `3000`。默认还会在面板本机自动部署 DNS Worker：部署前先检查本机是否已有 `dushengcdn-dns-worker.service`、同名 systemd unit 文件、`/opt/dushengcdn-dns-worker`、Worker 环境文件、同名 Docker 容器、Worker 进程或 DuShengCDN 监听 `53` 端口；发现已有部署时会跳过自动创建和安装，避免覆盖现有 Worker。没有发现本地 Worker 时，脚本会自动探测公网 IPv4，在 Server 中创建名为 `DNS服务响应端` 的 DNS Worker，拿到 Token 后调用 `scripts/install-dns-worker.sh` 监听 `PUBLIC_IP:53`。

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh
```

公网 IP 探测失败或需要绑定指定公网地址时显式传入：

```bash
bash scripts/install-server.sh \
  --server-url http://127.0.0.1:3010 \
  --public-ip 203.0.113.10
```

如只部署面板、不自动部署 DNS Worker：

```bash
bash scripts/install-server.sh --skip-dns-worker
```

如确认要覆盖本机已有 Worker 配置，可追加 `--force-dns-worker-reinstall`。自动安装 DNS Worker 仍要求宿主机放行公网 UDP/TCP `53`；如果该地址的 `53` 已被其它 DNS 服务占用，Worker 安装脚本会提示先停用或改端口。

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

生产环境建议在节点详情中维护节点池、公网 IP 池、调度权重和排空状态。自动 DNS 默认会按网站绑定的节点池选择在线且 OpenResty 健康的公网 IP；启用网站 GSLB 后，可在自动 DNS 配置中绑定多个节点池，按池权重、节点负载和防抖冷却时间同步 Cloudflare A/AAAA 记录。缓存清理和预热仍下发到网站默认节点池内的在线 Agent。

当前 Cloudflare DNS 模式是后台重算并同步记录，不是逐个用户请求实时调度。自建权威 DNS 模式已经提供管理端 Zone/记录/Worker 入口、网站模式选择、DNS Worker 查询面、心跳、只读快照 API、查询趋势、SERVFAIL/NXDOMAIN 观测、Worker 快照一致性告警、Worker 查询延迟/可用性看板、Server 侧按需 Worker UDP/TCP 探测、Agent 多点 Worker 探测、Zone 委派检查和迁移向导；如需按每次 DNS 查询来源返回不同节点，应在左侧「权威 DNS」创建 Zone 和 DNS Worker，通过「迁移向导」检查候选站点，满足条件时可一键切换到自建权威 DNS，也可到网站详情「自动 DNS」里手动切换。切换或保存启用站点前，公网可达的在线 Worker 都必须已拉取未过期调度快照，且快照版本一致。

## 自建权威 DNS 部署规划

自建权威 DNS 使用独立 DNS Worker 运行角色。Server 控制面负责管理 Zone、静态记录和 Worker Token，并通过 `GET /api/dns-snapshot` 向 Worker 下发只读调度快照，通过 `POST /api/dns-worker-heartbeat` 接收 Worker 状态与聚合指标。DNS Worker 监听 UDP/TCP `53`，只使用本地内存快照回答查询，不访问数据库，也不在查询路径调用外部 HTTP GeoIP API。面板本机可以同时部署 DNS Worker，但面板服务本身不会监听公网 `53`；使用 `scripts/install-server.sh` 部署面板时可默认一起创建并安装本机 DNS Worker，脚本会先检查本地是否已有 Worker，避免重复部署。单独安装或多机部署 Worker 时，也可以继续在管理端创建 Token 后运行安装脚本、Docker 或源码命令。

Worker 上报的聚合指标会在左侧「权威 DNS」展示最近 24 小时查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、Worker 快照一致性、Worker 查询延迟、可用率、错误率、最近公网探测健康状态、Agent 多节点探测通过率/RTT、GeoIP 国家库加载状态、来源作用域、Worker/Zone/站点维度、返回目标分布和当前 GSLB 调度状态，可用于检查实时 GSLB 是否按来源 CIDR、国家代码、来源分流桶、节点池权重、健康状态和负载阈值返回预期边缘 IP。「GSLB 调度状态」展示当前实际目标、期望目标、最近评估时间和防抖冷却状态；「GSLB 调度模拟」还可以在真实流量到达前按站点、记录类型、来源 IP 和来源国家代码预演当前快照返回目标，并解释节点池匹配、候选节点、跳过节点和原因；即使没有可返回目标，模拟也会保留节点诊断和无目标原因，便于上线前定位节点池、健康状态、公网 IP、负载阈值或探测门槛问题。这里的 Worker 查询延迟是 Worker 本地处理真实 DNS 查询的耗时；Agent 多节点探测 RTT 表示各边缘节点到 Worker NS 的主动探测耗时，默认只用于观测与排障；设置页「权威 DNS 运行参数」启用 Agent 探测调度门槛后，无新鲜成功探测的边缘节点不会进入自建权威 DNS GSLB 候选。DNS Worker 列表里的「探测」会由 Server 对该 Worker 公网地址发起 UDP/TCP 53 SOA 查询，适合确认防火墙、端口映射和公网地址是否可达；最近一次探测结果会保存在 Worker 列表和可用性面板中，并会作为迁移向导的切换准备条件。Worker 快照一致性会显示快照版本和最近拉取时间，迁移向导会要求公网可达 Worker 均持有未过期且版本一致的快照。Zone 详情里的「委派检查」可以对比注册商当前公网 NS 与面板配置的 NS；如果 NS 名称位于同一个 Zone 内，会提示需要在注册商配置 Glue/主机记录。

管理端操作顺序：

1. 在左侧「权威 DNS」创建 Zone，填写注册商需要委派的 NS。
2. 在同一页面创建 DNS Worker，复制创建后弹出的 Token 或部署命令。
3. 部署至少两个 DNS Worker，并在注册商处把域名 NS 委派到 Worker；NS 位于当前 Zone 内时同步配置 Glue/主机记录。
4. 打开「权威 DNS」的「迁移向导」，确认待迁移网站已经匹配到启用 Zone、存在在线 Worker、公网可达 Worker 均持有未过期且版本一致的调度快照，并按需启用站点 GSLB。
5. 在迁移向导点击「一键切换」，或到网站详情的「自动 DNS」分区手动把 `DNS 模式` 切换为 `自建权威 DNS` 并选择对应 Zone。
6. 迁移向导会在一键切换成功后自动复测：刷新网站 DNS 模式、执行 Zone 委派检查、探测在线 Worker 的公网 UDP/TCP `53`，并用当前快照按 global 与来源国家模拟 GSLB 返回目标。若委派结果不是已匹配，仍需到注册商调整 NS 或 Glue。
7. 如需更细的来源验证，可继续使用「GSLB 调度模拟」按来源 IP、HK、EU、global 等来源作用域预演返回目标，再到 Zone 详情执行「委派检查」，确认公网 NS 与期望 NS 匹配。

推荐使用安装脚本部署 DNS Worker：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN
```

脚本默认写入 `/opt/dushengcdn-dns-worker`，创建 `dushengcdn-dns-worker.service`，监听 UDP/TCP `53`，并把快照缓存保存在安装目录的 `data/dns-worker-snapshot.json`。启动服务前会检查默认监听端口是否已被其它进程占用；如果本机已有 `systemd-resolved`、`named`、`dnsmasq` 等本地 DNS 服务，请先停用/改端口，或用 `--listen PUBLIC_IP:53` 只绑定 Worker 公网地址。脚本会优先下载 GitHub Release 中的 DNS Worker 二进制；如果当前仓库还没有 Release，会自动安装 Go 并从源码构建，源码构建会把当前 Git 版本写入 Worker，避免版本显示为 `dev`。脚本还会默认下载 Country MMDB 到 `data/geoip/GeoLite2-Country.mmdb`，让国家代码节点池匹配开箱可用；下载失败不会阻断安装，Worker 会继续按来源 CIDR 或 `global` 作用域运行。

如果 Server 面板和 DNS Worker 部署在同一台机器，`--server-url` 可以使用容器或宿主机可访问的面板地址；`--listen` 建议显式写公网地址，例如 `--listen 203.0.113.10:53`，避免只想对公网提供权威 DNS 时和本机回环 DNS 服务混淆。
安装后可在 Worker 主机运行 `bash scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com`，一次性检查 systemd 服务、安装目录、环境文件、监听端口、快照、GeoIP、日志和 UDP/TCP SOA/NS 查询结果。

可选参数：

| 参数 | 说明 |
| --- | --- |
| `--server-url` | Server 地址，必填 |
| `--token` | DNS Worker 专属 Token，必填 |
| `--install-dir` | 安装目录，默认 `/opt/dushengcdn-dns-worker` |
| `--listen` | UDP/TCP 监听地址，默认 `:53` |
| `--snapshot-path` | 快照缓存路径，默认安装目录下的 `data/dns-worker-snapshot.json` |
| `--geoip-database` | 可选本地 MaxMind Country MMDB 路径 |
| `--geoip-database-url` | Country MMDB 下载地址，默认使用 Loyalsoldier GeoLite2-Country |
| `--no-geoip-download` | 不自动下载 Country MMDB |
| `--query-rate-limit` | 按来源 IP 每秒查询上限，默认 `200` |
| `--udp-response-size` | UDP 响应最大字节数，默认 `1232` |
| `--no-service` | 不创建 systemd 服务 |

Docker 运行示例也可继续使用：

```bash
docker run -d --name dushengcdn-dns-worker --restart unless-stopped \
  -p 53:53/udp -p 53:53/tcp \
  -v dushengcdn-dns-worker-data:/data \
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=https://cdn.example.com \
  -e DUSHENGCDN_DNS_WORKER_TOKEN=YOUR_DNS_WORKER_TOKEN \
  ghcr.io/satands/dushengcdn-dns-worker:latest
```

需要按国家代码匹配节点池时，再额外挂载本地 Country MMDB 并设置路径：

```bash
  -v /path/to/GeoLite2-Country.mmdb:/geoip/GeoLite2-Country.mmdb:ro \
  -e DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=/geoip/GeoLite2-Country.mmdb \
```

只使用来源 CIDR 或全局调度时可以省略 GeoIP。

源码运行示例：

```bash
cd dushengcdn_server
go run ./cmd/dns-worker \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen :53 \
  --snapshot-path /var/lib/dushengcdn-dns-worker/snapshot.json \
  --query-rate-limit 200 \
  --udp-response-size 1232
```

本地 Compose 示例见仓库根目录 `docker-compose.dns-worker.yaml`。如果需要按国家代码匹配 GSLB 节点池，可给 Worker 配置本地 MaxMind Country MMDB；如果在网站 GSLB 节点池里配置来源 CIDR，Worker 会直接按来源 IP/ECS 优先匹配，不依赖 GeoIP：

```bash
--geoip-database /var/lib/dushengcdn-dns-worker/GeoLite2-Country.mmdb
```

未配置本地 GeoIP 库或安装脚本下载 Country MMDB 失败时，Worker 仍会优先读取 EDNS Client Subnet 的来源 IP；来源 CIDR 命中时作用域为 `cidr:...`，未命中时国家代码为空并回退为 `global`。启用 `weighted` 或 `load_aware` 后，Worker 会在来源作用域后追加 `|bucket:xx` 分流桶，用于让 80/20 这类权重在逐查询答案中稳定生效。Worker 会在心跳里上报 GeoIP 是否加载、数据库路径和最近加载错误；如果面板显示「GeoIP 未加载」，国家代码节点池不会命中，但来源 CIDR 与 global 调度仍可继续工作。

生产部署原则：

* 至少部署两个 DNS Worker，例如 `ns1.example.net` 和 `ns2.example.net`。
* 在注册商处将需要托管的域名 NS 委派到这些 Worker，并按需配置 Glue 记录。
* 防火墙必须同时放行 UDP `53` 和 TCP `53`。
* Worker 主机上不要让 `systemd-resolved`、`named`、`dnsmasq` 或其它本地 DNS 服务占用同一个监听地址的 `53` 端口；排查时可用 `ss -lntu '( sport = :53 )'` 或 `lsof -nP -i :53`。
* Worker 到 Server 的快照拉取接口必须使用 HTTPS 和专属 Worker Token。
* Server 短暂不可用时，Worker 使用最后一次校验通过的有效快照继续回答；快照超过最大有效期后动态 GSLB 记录应返回 `SERVFAIL`。本地快照缓存会写入 SHA-256 checksum 元数据并携带可恢复的 GSLB 防抖状态，启动加载时校验完整性并恢复最近可用选择；Worker 运行中产生的新防抖状态会随 heartbeat 批量回传 Server；旧版本生成的裸快照 JSON 仍兼容读取。
* Worker 默认按来源 IP 每秒最多处理 `200` 次查询，超过后返回 `REFUSED`；可通过 `--query-rate-limit` 或 `DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT` 调整，设为 `0` 表示关闭。
* Worker 默认把 UDP 响应上限限制为 `1232` 字节；超过时设置 TC 位让递归解析器回退 TCP，可通过 `--udp-response-size` 或 `DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE` 调整。
* DNS Worker 不替代 Agent/OpenResty。反向代理配置修改后仍需发布并激活版本，Agent 才会应用。
* 在线 Agent 会自动接收 Server 下发的少量 DNS Worker 探测目标，并在心跳中上报 UDP/TCP `53` 可达性与 RTT；该能力不需要新增 Agent 配置项，但要求 Agent 节点能访问 DNS Worker 公网地址的 UDP/TCP `53`。

卸载 DNS Worker：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

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
* 源码或 Compose 部署时，把端口映射、密码、DSN、`SESSION_SECRET` 和旧版 `AGENT_TOKEN` 这类本地部署配置放到 `dushengcdn_server/.env`，不要直接修改仓库里的 `docker-compose.yaml`。

源码目录部署且只想更新面板端时：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
panel_port="$(grep -E '^DUSHENGCDN_HTTP_PORT=' .env | tail -n1 | cut -d= -f2-)"
curl -I "http://127.0.0.1:${panel_port:-3010}/api/status"
```

如果使用 Nginx、Nginx Proxy Manager、宝塔或其它反向代理对外提供 HTTPS，升级后也要确认反代上游端口指向 `.env` 中的宿主机端口；源码 Compose 默认是 `3010`，不是容器内的 `3000`。
如果面板仍打不开，可以在仓库根目录运行 `bash scripts/diagnose-server.sh`，一次性查看 Compose 状态、`/api/status`、端口监听和最近日志。

如果服务器上曾经直接改过仓库里的 `docker-compose.yaml`，`git pull` 可能提示本地改动会被覆盖。推荐先把本地端口、密码、DSN、`SESSION_SECRET` 和 Token 迁移到 `dushengcdn_server/.env`，再执行：

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

DNS Worker 使用安装脚本部署时，可重复执行安装命令进行重装或升级；脚本会优先下载当前仓库 Release 中的 DNS Worker 二进制并校验 `.sha256`。没有 Release 资产时，脚本会从源码构建并写入当前 Git 版本。卸载 DNS Worker：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

执行 `git reset --hard` 前请确认仓库内没有需要保留的源码修改。
源码 Compose 构建时，`DUSHENGCDN_VERSION` 会传给 Dockerfile 并写入 Server 或 Agent 二进制；管理端顶栏“版本”读取的是当前运行中 Server 的 `/api/status` 版本值，节点列表显示 Agent 上报的版本值。

Agent：

* Agent 默认只跟随正式版自动更新。
* Agent 自更新会要求 GitHub Release 同时包含目标二进制和同名 `.sha256` 校验文件，下载后必须通过 SHA-256 校验才会替换本地可执行文件。
* 安装脚本可重复执行，用于重装或升级 Agent；没有 Release 资产时会从源码构建，并写入当前 Git 版本，避免显示为 `dev`。
* Docker Compose 部署 Agent 时，使用 `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build` 重新构建。
* preview 升级需要手动触发。

卸载 Agent：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

卸载脚本会停止 Agent、删除 systemd 服务和安装目录，不会删除本机 OpenResty。

## 备份与恢复

升级前至少备份数据库和上传目录。源码部署可以直接使用仓库内的备份脚本：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

脚本默认读取 `dushengcdn_server/.env`，在 `auto` 模式下优先对可访问的 Compose PostgreSQL 执行 `pg_dump`，否则备份 SQLite 文件，并同时归档 `dushengcdn-data` 目录。输出目录默认为 `dushengcdn_server/backups/<timestamp>/`，其中包含数据库备份、数据目录归档和 `manifest.txt`。也可以显式指定目录和模式：

```bash
bash scripts/backup-server.sh \
  --server-dir /opt/dushengcdn/dushengcdn_server \
  --mode auto
```

脚本只创建备份文件，不会停止、恢复、覆盖或删除生产数据。恢复时可以使用仓库内的恢复脚本。PostgreSQL Compose 部署需要先停止 `dushengcdn` 服务，但保持 `postgres` 服务可访问：

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

`restore-server.sh` 会优先读取备份目录中的 `manifest.txt`，可用时校验 SHA-256，默认拒绝在 Compose `dushengcdn` 服务仍运行时恢复。覆盖前脚本会把当前数据库和 `dushengcdn-data` 归档到 `dushengcdn_server/backups/pre-restore/<timestamp>/`；确认不需要恢复上传目录时可加 `--skip-data-dir`，只有在明确接受风险时才使用 `--skip-current-backup` 或 `--force`。

手工 PostgreSQL Compose 备份示例：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
docker compose exec -T postgres pg_dump -U dushengcdn -d dushengcdn > backups/dushengcdn-$(date +%F-%H%M%S).sql
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

SQLite 部署示例：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
cp dushengcdn-data/dushengcdn.db backups/dushengcdn-$(date +%F-%H%M%S).db
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

手工 PostgreSQL 恢复示例：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose exec -T postgres psql -U dushengcdn -d dushengcdn -c "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"
docker compose exec -T postgres psql -U dushengcdn -d dushengcdn < backups/your-backup.sql
docker compose up -d
```

如果忘记 root 密码，但仍能进入服务器，可以在停止 Server 后用同一数据库配置执行：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
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
pnpm build
```

Swagger：

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```
