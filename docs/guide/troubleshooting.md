# 故障排查

你会学到：如何按症状排查 DuShengCDN Server、数据库、登录、Agent、OpenResty、配置发布和前端构建问题。

排查时先确认问题发生在哪一层：浏览器、Server、数据库、Agent、OpenResty、源站或 DNS。DuShengCDN 的配置不会直接在线写入所有节点，只有激活版本变化后，Agent 才会在 heartbeat 中发现并应用。

## 快速定位

| 现象 | 先看哪里 |
| --- | --- |
| 管理端打不开 | Server 容器或进程日志、端口监听 |
| 登录异常 | 默认账号、Session Secret、浏览器请求、Server 日志 |
| 数据无法保存 | 数据库连接、SQLite 文件权限、PostgreSQL 健康状态 |
| Agent 离线 | Agent 日志、Token、Server 地址、网络连通性 |
| 发布后节点未更新 | 激活版本、节点 heartbeat、应用记录 |
| OpenResty 应用失败 | 应用记录、Agent 日志、证书、源站地址、端口占用 |
| 访问分析无数据 | OpenResty 容器状态、观测端口、Agent 补报日志 |
| 自动解析不切换 | 节点池、公网 IP 池、调度开关、排空状态、Cloudflare Token 权限 |
| 本地自建解析迁移后解析异常 | 迁移向导切换后复测、Zone 委派检查、Worker 公网 UDP/TCP 53 探测、Agent 多节点探测、GSLB 模拟结果 |
| 缓存操作失败 | Agent WebSocket 连接、网站节点池、OpenResty 缓存目录配置 |

## Server 无法启动

1. 查看日志：

```bash
docker compose logs -n 200 dushengcdn
```

源码运行时查看终端输出。

使用 `scripts/install-server.sh` 部署或升级时，脚本会在 `docker compose up` 后确认 `dushengcdn` 服务是否仍在运行，并访问 `SERVER_URL/api/status` 做 HTTP 健康检查；如果服务启动后退出或 HTTP 检查失败，会自动打印最近日志，并对 PostgreSQL 密码/DSN、数据库连接、端口占用、宿主机端口和反向代理上游端口这几类常见错误给出提示。

2. 检查端口占用：

```bash
lsof -i :3000
```

3. 如果使用 PostgreSQL，确认数据库健康：

```bash
docker compose ps postgres
docker compose logs -n 100 postgres
```

4. 如果使用 SQLite，确认数据库文件目录可写：

```bash
ls -ld "$(dirname /path/to/dushengcdn.db)"
```

常见原因：

| 日志或现象 | 处理 |
| --- | --- |
| 数据库连接失败 | 检查 `DSN` 中用户名、密码、主机、端口、库名和 `sslmode` |
| `password authentication failed for user "dushengcdn"` | `POSTGRES_PASSWORD` / `DSN` 与已有 PostgreSQL 数据目录中的真实密码不一致。升级旧源码部署时，如果刚执行过 `bash scripts/install-server.sh` 后网页打不开，先把 `.env` 中的数据库密码和 DSN 改回旧值，再重启容器 |
| `too many clients already` | PostgreSQL 连接数已被打满。先查 `dushengcdn` 日志里是否有持续重复的 SQL 错误，例如 `node_access_logs_xx` 缺少 `operator`、`reason`、`cache_status` 等访问日志字段；再重启 Server 让启动自检修复分表字段，并按需调整 `DATABASE_MAX_OPEN_CONNS` |
| SQLite 无法创建文件 | 检查 `SQLITE_PATH` 所在目录是否存在且可写 |
| 端口被占用 | 修改 `PORT` 或 `--port`，或停止占用端口的进程 |

Docker Compose 部署时如只想改宿主机访问端口，可把 `ports` 改成 `3010:3000`；容器内应用仍监听 `3000`。

如果 `docker compose up -d --build` 用了很久，先区分“构建慢”和“启动慢”：Docker 输出中 `load build context` 表示把源码目录打包发给 Docker，`pnpm build` / `go build` 表示编译，最后 `Container ... Started` 才是容器启动。`load build context` 如果显示数 GB，通常是仓库目录里混入了 `postgres-data`、`dushengcdn-data`、`backups`、`upload`、`logs` 等运行数据；这些目录应被 `.dockerignore` 排除。清理或移动这些目录后再构建，升级时间会明显下降，生产数据仍通过 Compose volume 挂载，不需要打进镜像。

如果旧源码部署原先没有 `.env`，升级后第一次运行 `scripts/install-server.sh` 创建了随机数据库密码，旧 `postgres-data` 中的 PostgreSQL 密码不会自动变化。可先按旧默认密码恢复连接：

```bash
cd /opt/dushengcdn/dushengcdn_server
sed -i 's/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=replace-with-strong-password/' .env
sed -i 's#^DSN=.*#DSN=postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable#' .env
docker compose --env-file .env up -d --build
docker compose --env-file .env logs --tail=100 dushengcdn
```

如果你之前手动设置过 PostgreSQL 密码，把上面命令中的 `replace-with-strong-password` 换成真实旧密码。面板恢复后，再按 PostgreSQL 标准流程有计划地轮换数据库密码。

## 管理端打不开或空白

源码 Compose 部署或升级后，优先在仓库根目录运行只读诊断脚本：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

脚本会读取 `dushengcdn_server/.env`，展示 Compose 服务状态、`SERVER_URL/api/status` 健康检查、宿主机面板端口和 `3000` 端口监听、最近 `dushengcdn` / `postgres` 日志，并提示常见的数据库认证、端口占用和反向代理上游端口问题。脚本不会重启服务或修改配置。

1. 确认 Server 正在监听。源码 Compose 默认宿主机端口是 `.env` 中的 `DUSHENGCDN_HTTP_PORT=3010`，不是容器内监听的 `3000`：

```bash
cd /opt/dushengcdn/dushengcdn_server
panel_port="$(grep -E '^DUSHENGCDN_HTTP_PORT=' .env | tail -n1 | cut -d= -f2-)"
curl -I "http://127.0.0.1:${panel_port:-3010}/api/status"
curl -I http://127.0.0.1:3000/api/status
```

2. 如果是源码运行，确认已经构建前端静态产物：

```bash
cd dushengcdn_server/web
pnpm build
```

3. 检查浏览器访问地址是否与反向代理配置一致。Nginx、Nginx Proxy Manager 或宝塔的上游端口应填写宿主机映射端口，例如默认源码 Compose 是 `3010`；只有直接暴露 `3000:3000` 时才填 `3000`。

4. 如果通过前端开发服务器访问，确认后端代理地址：

```bash
cd dushengcdn_server/web
NEXT_DEV_BACKEND_URL=http://127.0.0.1:3000 pnpm dev
```

如果页面突然变成反向代理的 `502 Bad Gateway`，但重启 Server 后恢复，优先查看重启前后的 Server 与 PostgreSQL 日志：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose logs --since "12h" dushengcdn | grep -Ei "node_access_logs|too many clients|ERROR|FATAL"
docker compose logs --since "12h" postgres | grep -Ei "too many clients|ERROR|FATAL"
```

`node_access_logs_00` 到 `node_access_logs_09` 是观测访问日志分表。如果日志持续出现 `column "operator" of relation "node_access_logs_xx" does not exist` 这类字段缺失，再叠加 PostgreSQL `too many clients already`，说明访问日志写入错误把数据库连接耗尽，OpenResty 前面的 `502` 只是管理端上游不可用的表现。新版 Server 启动时会重新校验并补齐当前访问日志分表字段；访问日志写入失败也不会再阻断节点心跳、负载指标和 DNS 探测入库。

## 默认账号或 root 无法登录

首次空库启动会创建 `root` 用户。密码优先使用 `.env` 中的 `DUSHENGCDN_INITIAL_ROOT_PASSWORD`；如果没有配置该值，则查看 Server 首次空库启动日志中的一次性随机密码。首次登录后如果已经修改密码，应使用修改后的密码。

排查步骤：

1. 确认连接的是预期数据库，避免 `SQLITE_PATH` 或 `DSN` 指向了另一个环境。
2. 查看 Server 日志中使用的是 `sqlite` 还是 `postgres`。
3. 如果部署在多副本或反向代理后，确认 `SESSION_SECRET` 固定且各实例一致。
4. 清理浏览器 Cookie 后重新登录。

如果确认需要离线重置 root 密码，并且你可以进入 Server 所在机器，先停止 Server，再用同一数据库配置执行一次性命令。

Docker Compose 部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
```

源码或二进制部署：

```bash
cd /opt/dushengcdn/dushengcdn_server
export SESSION_SECRET='same-session-secret'
export DSN='postgres://dushengcdn:password@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn --reset-root-password 'replace-with-new-password'
```

如果使用 SQLite，把 `DSN` 换成实际 `SQLITE_PATH`。重置完成后请重新启动 Server，并登录后再次设置一个只自己知道的强密码。

## 更新代码时提示本地改动冲突

典型报错：

```text
error: Your local changes to the following files would be overwritten by merge:
        dushengcdn_server/docker-compose.yaml
```

原因通常是服务器上直接改过仓库里的 Compose 文件，例如为了改端口、数据库密码或 Token。处理方式：

1. 先把本地 Compose 文件中的端口、密码、DSN、`SESSION_SECRET`、Token 记录下来。
2. 如果没有需要保留的源码修改，执行 `git fetch origin main && git reset --hard origin/main` 拉回仓库新版。
3. 在 `dushengcdn_server` 目录执行 `cp -n .env.example .env`，把真实部署参数写入 `.env`。
4. 再执行 `DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build`。

端口冲突只需要改宿主机侧端口，例如 `3010:3000`；容器内仍监听 `3000`。

## Agent 无法注册或一直离线

在 Agent 节点执行：

```bash
curl -I http://your-server:3000
```

查看 Agent 日志：

```bash
journalctl -u dushengcdn-agent -n 200 --no-pager
```

检查配置文件：

```bash
sed -n '1,160p' /opt/dushengcdn-agent/agent.json
```

重点确认：

| 配置 | 说明 |
| --- | --- |
| `server_url` | 必须是 Agent 节点能访问的 Server 地址 |
| `agent_token` / `discovery_token` | 至少填写一个 |
| `heartbeat_interval` | 支持毫秒整数或 Go duration 字符串 |
| `request_timeout` | 网络较慢时可适当增大 |

如果日志提示 Token 无效，重新在管理端准备 Token 并更新 `agent.json`，然后重启。日志中出现 `Agent authentication failed` 时，优先核对 `agent_token` / `discovery_token` 或 `DUSHENGCDN_AGENT_TOKEN` / `DUSHENGCDN_DISCOVERY_TOKEN`；首次注册应使用 Discovery Token，注册后心跳、拉取配置和 WebSocket 应使用节点专属 Agent Token。日志中出现 `request to Server URL ... failed` 时，优先核对 `server_url`、DNS 解析、防火墙和证书信任。

如果是升级 Server/面板后，升级前已经部署的旧 Agent 全部离线，先确认 Server 进程或容器仍然配置了旧版 `AGENT_TOKEN`。旧 Agent 可能还在使用这个全局 Token 上报心跳；Server 会兼容这类请求，但要求心跳或应用日志 payload 中携带已存在的 `node_id`，并且该节点尚未切换为新的节点专属 Agent Token。兼容成功后节点会恢复 HTTP 心跳、配置拉取和应用日志上报；建议后续在节点详情复制新的节点专属安装/配置命令，逐步把旧 Agent 配置迁移到专属 Token。

```bash
systemctl restart dushengcdn-agent
```

## 发布后节点没有应用新版本

按顺序检查：

1. 版本页面中是否已经激活目标版本。
2. 节点是否在线，最近心跳时间是否更新。
3. 应用记录中是否有目标版本的成功、警告或失败记录。
4. 网站配置是否启用；未启用的网站不会参与发布渲染。
5. Agent 日志是否出现拉取、校验、reload 或回滚信息。

查看 Agent 日志：

```bash
journalctl -u dushengcdn-agent -f
```

注意：某个目标 `version + checksum` 一旦应用失败并回退，Agent 会在本地状态中阻断该目标重复应用。修正配置后需要重新发布生成新的 checksum，或激活旧版本回滚。

如果这是 Agent 首次应用配置，且本地没有历史 `nginx.conf` 可回滚，失败目标仍会被阻断，但 Agent 会尝试进入安全兜底运行态。此时应用记录和 Agent 日志会包含 `fallback runtime started`，OpenResty 对外只监听 `80` 端口并统一返回 `503` 与 `DuShengCDN: No Valid Configuration`，同时保留本地 `stub_status` 健康检查入口。修正配置并重新发布新版本后，Agent 会覆盖兜底配置并恢复正常代理。

## OpenResty 应用失败

常见原因：

| 原因 | 排查 |
| --- | --- |
| 域名或 server 块冲突 | 检查同一域名是否被多个网站配置使用 |
| 源站地址不合法 | 确认所有源站都是 `http://` 或 `https://` |
| 多源站格式不符合约束 | 多源站必须是纯 `scheme://host[:port]` |
| 证书缺失或路径错误 | 检查域名是否绑定证书，以及 Agent 证书目录是否可写 |
| 端口被占用 | 检查本机 `80`、`443` 端口 |

OpenResty 配置校验：

```bash
openresty -t -c /path/to/dushengcdn/data/etc/nginx/nginx.conf
```

OpenResty 运行状态：

```bash
ps aux | grep openresty
```

Agent 周期性健康检查通过本地 `http://127.0.0.1:<openresty_observability_port>/dushengcdn/stub_status` 判断 OpenResty 是否存活，不会反复执行 `openresty -t`。如果节点被标记为 unhealthy，优先确认该本地观测端口是否正在监听；如果只在应用配置时出现 `host not found in upstream`，说明失败来自配置校验或 reload，而不是周期性健康探针。

实际二进制路径和主配置路径以 `agent.json` 中的 `openresty_path` 与 `main_config_path` 为准。

## HTTPS 不生效

1. 确认证书已经上传或托管。
2. 确认网站配置中对应域名已经绑定证书。
3. 确认发布并激活了新版本。
4. 查看应用记录是否成功。
5. 用 `curl` 查看证书和状态码：

```bash
curl -Iv https://your-domain
```

没有绑定证书的域名不会被自动加入 HTTPS 配置，这是预期行为。

## 访问分析没有数据

1. 确认节点已经成功应用包含观测 Lua 资源的配置。
2. 确认 OpenResty 正在运行。
3. 查看 Agent 日志是否有观测采集或补报失败信息。
4. 检查 `openresty_observability_port` 是否被占用，默认是 `18081`。
5. 确认 Server 侧没有因数据库清理策略删除对应时间窗口数据。

## 自动解析未按节点切换

按顺序检查：

1. Cloudflare 账号 Token 是否具备 `Zone Read` 和 `DNS Edit` 权限。
2. Token 是否保存为原始 Token、`Bearer ...` 或 JSON 中的 `api_token` / `apiToken` / `token`。
3. 网站配置是否启用自动解析，并绑定了正确节点池。
4. 目标节点池内是否有在线节点，且 OpenResty 状态不是 unhealthy。
5. 节点公网 IP 池是否包含对应记录类型的公网地址；A 记录需要 IPv4，AAAA 记录需要 IPv6。
6. 节点是否被关闭调度或开启排空模式。

## 本地自建解析迁移后解析异常

如果从 Cloudflare 同步切到本地自建解析后解析不符合预期，先打开左侧「本地自建解析」的「迁移向导」查看「切换后复测」：

1. 「网站解析模式」应显示已绑定目标 Zone；若失败，回到网站详情的「负载均衡」确认解析模式和 Zone。
2. 「Zone 委派检查」应为已匹配；若部分匹配、不匹配或提示 Glue，登录注册商补齐 NS 或 Glue/主机记录。
3. 「Worker 公网探测」至少应有一个在线 Worker UDP/TCP `53` 可达；若失败，检查 Worker 公网地址、防火墙、端口映射和安全组。
4. 「Worker 快照一致性」应显示公网可达 Worker 都持有未超过 `AuthoritativeDNSSnapshotMaxAge` 的调度快照，且版本一致；若快照为空、过期或版本不一致，检查 Worker 到 Server 的 HTTPS 访问、Token、心跳日志和快照拉取错误。
5. 「GSLB 模拟复测」通常应返回目标 IP；若无目标，打开模拟节点诊断，检查节点是否在线、OpenResty 是否健康、公网 IP 池、节点池、排空模式、GSLB 权重、负载阈值和探测门槛原因。

如果注册商已经配置 NS/Glue，但 `dig @PUBLIC_IP example.com SOA` 提示 `connection refused`、`no servers could be reached`，或面板委派检查仍提示上游无法访问权威服务器：

先在 DNS Worker 主机运行只读诊断脚本：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com
```

脚本会检查 `dushengcdn-dns-worker.service`、安装目录、`dns-worker.env`、监听端口、快照文件、GeoIP 文件和最近日志，并在提供 `--public-ip` / `--zone` 后执行 UDP/TCP SOA/NS 查询。脚本不会重启服务或修改配置。
如果面板和 DNS Worker 在同一台主机，想一次性按上线验收顺序检查 Server 与 Worker，可运行：

```bash
cd /opt/dushengcdn
bash scripts/verify-authoritative-dns.sh --public-ip PUBLIC_IP --zone example.com
```

1. 在 DNS Worker 主机执行 `systemctl status dushengcdn-dns-worker`。如果提示 `Unit dushengcdn-dns-worker.service could not be found`，说明只配置了面板 Zone/注册商 NS，还没有部署 DNS Worker。
2. 查看 `ss -lntup | grep ':53'` 和 `ss -lnuap | grep ':53'`。只看到 `systemd-resolved` 监听 `127.0.0.53` 或 `127.0.0.54` 不代表公网 `53` 已经有权威 DNS 服务；公网地址仍可能没有任何进程监听。
3. 在左侧「本地自建解析」创建 DNS Worker Token 后，在 Worker 主机运行安装脚本；如果 Worker 和面板在同一台机器，可使用面板本机可访问地址作为 `--server-url`，并用 `--listen PUBLIC_IP:53` 绑定公网地址。
4. 安装后执行 `systemctl status dushengcdn-dns-worker`、`journalctl -u dushengcdn-dns-worker -n 100 --no-pager`，确认服务已启动且没有 Token、Server URL 或快照拉取错误。
5. 确认服务器防火墙、云安全组或上游网络同时放行 UDP `53` 和 TCP `53`，再执行 `dig @PUBLIC_IP example.com SOA` 和 `dig @PUBLIC_IP example.com NS`。
6. 如果使用 `ns1.example.com` / `ns2.example.com` 这类位于同一 Zone 内的 NS，注册商还必须配置 Glue/主机记录，把这些 NS 名称指向 Worker 公网 IP。

如果 `dig @PUBLIC_IP example.com SOA` 和 `NS` 已返回 `NOERROR`，但业务域名的 `A`/`AAAA` 仍没有返回目标，或 Worker 日志里的快照显示 `routes=0`：

1. 这说明 DNS Worker 已经能回答 Zone 基础记录，但当前快照里还没有绑定到本地自建解析的网站动态路由。
2. 到网站详情「负载均衡」把 `解析模式` 切换为 `本地自建解析`，并选择对应 Zone；也可以在「本地自建解析」的「迁移向导」里对候选站点执行一键切换。
3. 确认网站至少有一个域名落在该 Zone 下，且没有同名静态 `CNAME` 或启用的同名静态 `A`/`AAAA` 与动态记录冲突。
4. 切换后等待 DNS Worker 下一次心跳/快照拉取，或重启 Worker，再查看日志里的 `routes` 数量是否增加。
5. 使用「GSLB 调度模拟」检查该站点的 A/AAAA 是否能返回边缘 IP；如果无目标，继续按节点池、公网 IP、在线状态、OpenResty 健康、排空模式、负载阈值和 Agent 探测门槛排查。

如果保存 Zone 静态记录、网站配置或迁移向导切换时提示“静态记录冲突”：

1. 到左侧「本地自建解析」进入对应 Zone，检查是否已有同名启用的静态 `A`、`AAAA` 或 `CNAME`。
2. 如果该域名已经由网站配置的本地自建解析动态 GSLB 接管，删除或禁用同名同类型静态 `A`/`AAAA`，并删除或改名同名 `CNAME`。
3. 如果想保留静态解析，不要把该网站切换到本地自建解析动态模式，或改用另一个不冲突的域名。
4. `TXT`、`MX`、`NS`、`SOA` 等其它类型记录不属于该冲突范围，可继续保留。

迁移向导会在候选列表提前展示这类阻断项；如果列表显示“需处理”，先处理阻断项，再重新刷新候选或点击迁移向导。

如果迁移向导、一键切换或网站详情保存时提示“无法返回 A/AAAA 边缘 IP”，或提示某个“来源国家 / 来源网段”无法返回边缘 IP：

1. 确认网站绑定的默认节点池或 GSLB 节点池名称和节点列表里的池名一致。
2. 确认目标节点在线、OpenResty 状态不是 unhealthy，且没有开启排空模式。
3. 确认节点公网 IP 池包含对应记录类型的公网地址；A 记录需要 IPv4，AAAA 记录需要 IPv6。
4. 如果提示的是特定国家或 CIDR，检查该来源匹配到的节点池是否至少有一个可用节点；来源 CIDR 会优先于国家代码匹配。
5. 如果启用了 `负载感知` 或负载阈值，检查当前连接数、CPU 和内存快照是否把全部节点剔除了；可临时放宽最大连接数、最大 CPU 或最大内存阈值后再试。
6. 检查「GSLB 调度模拟」里的指标时间和节点原因；即使当前无返回目标，模拟结果也会尽量保留匹配节点池与每个节点的跳过原因，便于判断是节点池、在线状态、公网 IP 类型还是负载阈值导致。超过 `GSLBMetricFreshnessSeconds` 的旧指标不会参与评分，缺少新鲜指标的节点只作为兜底候选。如果所有节点都显示无新鲜指标，先确认 Agent 心跳和指标上报是否正常，或临时放宽设置页「权威 DNS 运行参数」里的 GSLB 指标新鲜度。
7. 到「本地自建解析」里的「GSLB 调度模拟」按同一站点、记录类型和来源再次模拟，查看每个节点的跳过原因、Agent 探测摘要和探测 RTT。Agent 探测异常说明边缘节点到 DNS Worker NS 的 UDP/TCP `53` 可达性存在风险；默认不会直接把该边缘节点从 GSLB 选点中剔除，但如果设置页「权威 DNS 运行参数」开启了「启用 Agent 探测调度门槛」，无新鲜成功探测的节点会被排除，进入候选后还会按探测健康比例、过期比例和平均 RTT 对权重或负载感知评分做有界修正。若模拟结果显示「Agent 探测未达到调度门槛」且无返回目标，说明按节点池、状态、负载和公网 IP 仍有候选，但这些候选都缺少新鲜成功探测；模拟节点原因会继续区分「尚未收到新鲜成功探测」「探测结果已过期」「UDP/TCP 53 未同时可达」或「UDP/TCP 53 探测均失败」，分别对应 Agent 尚未上报、上报中断、单协议被阻断或双协议均不可达。

如果迁移向导、一键切换或网站详情保存时提示“没有在线 DNS Worker”或“在线 DNS Worker 尚未通过公网 UDP/TCP 53 探测”：

1. 先在「本地自建解析」创建 DNS Worker，并用面板生成的 Token 部署 Worker。
2. 确认 Worker 心跳状态为在线，并且填写了可从公网访问的 DNS Worker 地址。
3. 在 DNS Worker 列表点击「探测」，确认 UDP 和 TCP `53` 都可达；只通过其中一个协议时仍不视为可迁移/可启用。
4. 检查 Worker 服务器防火墙、云安全组、NAT 和端口映射是否同时放行 UDP `53` 与 TCP `53`。
5. 如果最近一次探测显示过期，重新点击「探测」后再保存网站或执行一键切换。

如果安装 DNS Worker 时提示 UDP/TCP `53` 端口已被占用，或服务日志出现 `address already in use`：

1. 在 Worker 主机执行 `ss -lntu '( sport = :53 )'` 或 `lsof -nP -i :53`，确认占用 `53` 的进程。
2. 常见占用者是 `systemd-resolved`、`named`、`dnsmasq` 或同机已有 DNS 服务；停止或改端口后再重跑安装脚本。
3. 如果现有服务只绑定回环地址，而 DNS Worker 只需要绑定公网地址，可以使用 `--listen PUBLIC_IP:53`，不要使用默认的 `:53` 全地址监听。
4. 仅本地开发时改用高位端口，例如 `--listen 127.0.0.1:1053`，再用 `dig @127.0.0.1 -p 1053 example.com SOA` 验证。

如果迁移向导、一键切换或网站详情保存时提示“公网可达 DNS Worker 尚未拉取未过期的调度快照”、“部分公网可达 DNS Worker 尚未拉取未过期的调度快照”或“公网可达 DNS Worker 的调度快照版本不一致”：

1. 先确认至少一个 Worker 在列表中为在线，并且最近一次公网 UDP/TCP `53` 探测为健康。
2. 查看「Worker 快照一致性」，确认公网可达 Worker 的 `last_snapshot_version` 不为空且一致，`last_snapshot_at` 没有超过 `AuthoritativeDNSSnapshotMaxAge`。
3. 在 Worker 服务器查看服务日志，重点检查 Token 无效、Server URL 不可达、HTTPS 证书校验失败、快照接口返回错误等信息。日志中出现 `DNS Worker Token authentication failed` 时，优先核对 `DUSHENGCDN_DNS_WORKER_TOKEN` 或 `--token`；出现 `request to Server URL ... failed` 时，优先核对 `DUSHENGCDN_DNS_WORKER_SERVER_URL` 或 `--server-url`、DNS 解析、防火墙和证书信任。
4. 确认 Worker 使用的 Token 是左侧「本地自建解析」中创建的 DNS Worker Token，不是 Agent Token 或登录密码。
5. 修复后等待下一次 Worker 心跳/快照拉取，或重启 Worker，再刷新迁移向导或重新保存网站。

如果「Worker 可用性」里 Server 侧公网探测正常，但「Agent 多节点探测」异常：

1. 确认对应 Agent 节点可以直接访问 DNS Worker 公网地址的 UDP/TCP `53`，例如在节点上执行 `dig @ns1.example.net example.com SOA`。
2. 检查节点所在机房、云厂商安全组或出站防火墙是否阻断 UDP `53` 或 TCP `53`。
3. 查看 `journalctl -u dushengcdn-agent -n 200 --no-pager`，确认 Agent 心跳是否正常；Agent 只有在收到 Server 下发的探测目标后，才会在下一次心跳或 WebSocket status 上报结果。
4. 如果某个 Worker 没有出现在多节点探测中，确认该 Worker 已在线且填写了公网地址；Server 每次只下发少量在线 Worker 目标，避免心跳被大量探测拖慢。
5. 如果显示「探测过期」，说明 Server 最近没有收到该 Agent 对 Worker 的新探测结果；先确认 Agent 已升级到支持 DNS 探测的版本，再检查心跳或 WebSocket status 是否持续上报。
6. 如果已开启「启用 Agent 探测调度门槛」，多节点探测失败、过期或平均 RTT 明显偏高都会影响本地自建解析 GSLB 选点；探测质量系数有上下限，不会完全压倒节点池权重、节点权重和负载感知评分。排障期间可临时关闭该开关，让节点先按在线状态、OpenResty 健康和负载阈值参与调度。

迁移向导不会直接修改注册商 NS。注册商侧 NS 生效还受上级 DNS 缓存和 TTL 影响，调整后可再次执行 Zone 委派检查。

## 缓存清理或预热失败

缓存运行时操作通过 Agent WebSocket 下发。排查时确认：

1. 节点列表中目标节点是否显示在线，且 WebSocket 可用。
2. 网站绑定的节点池是否正确。
3. 节点没有开启排空模式。
4. 已在性能设置中配置并发布 `OpenRestyCachePath`。
5. Agent 日志中是否出现 `agent cache purge failed` 或 `agent cache warm failed`。

## 前端构建失败

执行：

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

常见原因：

| 现象 | 处理 |
| --- | --- |
| pnpm 版本不一致 | 使用 `corepack enable` 后重新安装 |
| 类型错误 | 先运行 `pnpm typecheck` 定位具体文件 |
| API 类型不一致 | 检查 `lib/api/` 和 `types/` 中的响应结构 |
| E2E 失败 | 确认 Server 和前端开发服务器都已启动 |

## 文档站构建失败

```bash
cd docs
pnpm install
pnpm build
```

如果是链接错误，检查新增页面是否已经加入 `docs/config.ts` 侧边栏，或者相对链接是否指向存在的 Markdown 文件。
