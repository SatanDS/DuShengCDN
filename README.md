<div align="center">

# DuShengCDN

轻量、自托管的 OpenResty 控制面，用于管理反向代理规则、配置发布、节点同步、TLS 证书与基础可观测能力。

</div>

<p align="center">
  <a href="https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/LICENSE">
    <img src="https://img.shields.io/github/license/SatanDS/DuShengCDN?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/SatanDS/DuShengCDN/releases/latest">
    <img src="https://img.shields.io/github/v/release/SatanDS/DuShengCDN?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/SatanDS/DuShengCDN/pkgs/container/dushengcdn">
    <img src="https://img.shields.io/badge/GHCR-ghcr.io%2Fsatands%2Fdushengcdn-brightgreen" alt="ghcr">
  </a>
</p>

> [!WARNING]
> 使用 `root` 用户初次登录系统后，务必修改默认密码 `123456`。

## 文档

文档内容已随仓库维护，位于 `docs/` 目录。

## 本次更新说明

本次更新补齐了基础 CDN 调度与缓存闭环能力：

* 节点支持节点池、公网 IP 池、标签、权重、调度开关和排空模式。
* Cloudflare 自动 DNS 支持同一域名同步多个 A/AAAA 目标，并可按节点池、健康状态、权重和 GSLB 负载感知策略选择在线节点。
* 自建权威 DNS 与实时 GSLB 已落地 Server 控制面和 DNS Worker MVP，包含 Zone、DNS Worker Token、心跳/聚合、只读调度快照 API、UDP/TCP 53 查询回答、逐查询 GSLB 选点、来源 CIDR/国家代码匹配、稳定权重分流桶、按来源 IP 查询限速、UDP 响应大小保护、管理端 DNS 查询趋势、SERVFAIL/NXDOMAIN 观测、来源作用域分布、当前调度状态、Worker 查询延迟/可用性看板、Server 侧按需 Worker UDP/TCP 探测与健康状态、Worker 快照一致性告警、Zone 委派检查、Glue 提示和 Cloudflare 到权威 DNS 的迁移向导。
* 网站缓存页支持向目标节点池下发全量缓存清理和首页预热。
* 观测计量合并访问日志与计量统计，支持站点/节点流量、缓存命中率、回源流量、状态码分布、TOP URL/IP/地区、带宽 P95 和节点可用率。
* OpenResty 回源失败保护增强，默认对常见 5xx/超时错误尝试切换下一源站。
* Cloudflare API Token 校验兼容原始 Token、`Bearer ...` 和包含 `api_token` / `apiToken` / `token` 的 JSON。

## 核心能力

* 反向代理网站配置与多域名绑定
* 配置预览、发布、激活与历史回滚
* Agent 自动注册、心跳、同步、校验、reload 与失败回滚
* OpenResty 主配置、性能参数、缓存参数与 Lua 资源托管
* 本地 GeoIP 地区限制、真实客户端 IP 国家码识别、IP 国家码缓存与在线精确查询 API 回退
* 可选本地轻量 WAF，支持观察/拦截模式、内置攻击规则、白名单与自定义规则
* 网站配置详情页提供自动 DNS、缓存策略、PoW、WAF、地区限制和认证配置分区
* TLS 证书、域名资产、节点凭证与版本状态管理
* 节点池、公网 IP 池、权重调度、排空模式与 Cloudflare 多目标自动 DNS / GSLB 多节点池调度
* Cloudflare 自动 DNS、在线节点自动解析、节点离线 DNS 切换、GSLB 防抖状态与 DDoS 自动切换橙云
* 自建权威 DNS 控制面、Worker 快照/心跳 API、UDP/TCP 53 查询 Worker、查询限速、UDP 响应截断保护、NS 委派检查、Glue 提示、Cloudflare 迁移向导、逐查询实时 GSLB、来源 CIDR/国家代码匹配、稳定权重分流桶、当前调度状态、查询趋势、来源作用域分布、Worker 查询延迟/可用性、按需 Worker 探测健康状态和快照一致性观测
* 站点级缓存策略、缓存清理、首页预热、缓存命中与回源健康统计
* Agent 安装环境检测、Release 二进制下载、自动更新与删除节点联动卸载
* 节点真实 IP 识别、节点详情静默刷新、全局操作提示与主题化确认弹窗
* 观测计量、请求聚合、访问分析、资源快照、健康事件与节点详情

## 快速开始

### 1. 启动 Server

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
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "3000:3000"
    environment:
      SESSION_SECRET: replace-with-random-string
      DSN: postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
      GIN_MODE: release
      LOG_LEVEL: info

volumes:
  postgres-data:
```

```bash
docker compose up -d
```

访问地址：`http://localhost:3000`

默认账号：

* 用户名：`root`
* 密码：`123456`

#### HTTPS 反代到管理端

生产环境建议在面板服务器上用 Nginx、OpenResty 或宝塔反向代理对外提供 HTTPS，再转发到 DuShengCDN 管理端端口。反代配置必须保留真实客户端 IP 头，否则节点注册和心跳经过反代时可能只能识别到内网 IP。

如果 Docker Compose 使用默认端口映射 `3000:3000`，反代目标是 `http://127.0.0.1:3000`；如果你改成 `3010:3000`，反代目标则改为 `http://127.0.0.1:3010`。

Nginx / OpenResty 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name cdn.example.com;

    ssl_certificate /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

Nginx Proxy Manager 可在 `Proxy Hosts` -> `Add Proxy Host` 或 `Edit Proxy Host` 中配置：

* `Forward Hostname / IP` 填写面板容器所在机器，例如 `127.0.0.1`
* `Forward Port` 填写宿主机映射端口，例如你使用 `3010:3000` 时这里填写 `3010`
* 建议开启 `Websockets Support`，否则 Agent 的 WebSocket 通知可能退回到普通心跳轮询
* 在齿轮图标或 `Advanced` -> `Custom Nginx Configuration` 中填入下面的请求头配置

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

宝塔面板可在网站的“反向代理 -> 配置文件”里把 `proxy_set_header` 相关配置加入对应的 `location /` 块，保存后重载 Nginx。

### 2. 配置 Cloudflare 自动 DNS 与防护

如果域名已经接入 Cloudflare，可以让 DuShengCDN 自动创建或更新 DNS 记录，并在节点离线或遇到攻击流量时自动调整解析策略。

准备 Cloudflare API Token：

* 权限需要包含 `Zone Read` 和 `DNS Edit`
* Token 范围建议只授权给需要托管的 Zone
* 不建议使用 Global API Key
* 可以直接填写原始 Token，也兼容 `Bearer ...` 或包含 `api_token` / `apiToken` / `token` 的 JSON

在管理端操作：

1. 在左侧「DNS账号」准备 Cloudflare DNS 账号。
2. 在节点详情中维护节点池、公网 IP 池、权重、调度开关和排空状态。
3. 新建网站配置时选择节点池，并开启 `创建时自动解析 DNS`；已有站点可在详情页的 `自动 DNS` 分区维护。
4. 选择 Cloudflare DNS 账号，记录类型通常选择 `A`；IPv6 节点选择 `AAAA`。
5. `记录内容` 留空时，系统会自动选择该节点池中的在线公网 IP，并把 `自动选择在线节点 IP` 打开。
6. 如需跨 HK、EU 等多个节点池分流，在「自动 DNS」分区启用 `GSLB 多节点池调度`，点击 `+` 逐行添加节点池、权重、可选国家代码和来源 CIDR，例如池名 `hk`、权重 `80`、国家代码 `HK,TW`、来源 CIDR `203.0.113.0/24`。来源 CIDR 会优先于国家代码匹配。
7. 如需隐藏源站或抗攻击，可开启 `Cloudflare 代理`；如需自动切换橙云，将 `DDoS 防护模式` 设置为 `自动`。

自动 DNS 行为：

* 创建网站时会立即向 Cloudflare 创建或更新 DNS 记录。
* 后台每 1 分钟巡检一次已开启自动 DNS 的规则。
* 开启 `自动选择在线节点 IP` 后，节点离线、OpenResty 不健康、节点被排空、关闭调度或节点公网 IP 池没有对应 A/AAAA 地址时会跳过该节点。
* 自动 DNS 可以按健康时间、节点权重或负载感知评分选择，并支持同步多个 A/AAAA 目标。
* GSLB 模式可绑定多个节点池，按来源 CIDR、国家代码、池权重、节点权重、OpenResty 连接数、CPU、内存和负载阈值选择 DNS 目标；网站配置里可维护最大连接数、最大 CPU 使用率和最大内存使用率。
* 自建权威 DNS 模式下，`weighted` / `load_aware` 会按来源 IP/ECS 生成稳定分流桶，所以 HK 池权重 80、EU 池权重 20 这类配置会在不同来源桶之间形成接近 8:2 的 DNS 答案分布；Cloudflare 模式只同步一组静态记录，不具备逐查询来源分流。
* GSLB 会记录最近一次实际目标和期望目标，旧目标仍健康且冷却时间未到时不会反复切换。
* Cloudflare DNS 模式不是逐请求实时调度，而是后台巡检重算并同步记录；实际流量还会受到 DNS TTL 与递归解析缓存影响。
* 自建权威 DNS 模式需要把域名 NS 委派到 DuShengCDN DNS Worker；Worker 会在查询时实时执行 GSLB 调度。左侧「权威 DNS」的「迁移向导」可检查 Cloudflare 模式网站是否已有匹配 Zone、在线 Worker 和 GSLB 配置，「GSLB 调度模拟」可按来源 IP 和国家代码预演当前快照返回目标并解释节点候选/跳过原因，Zone 详情可检查公网 NS 是否匹配，并在 Zone 内 NS 需要 Glue/主机记录时提示。未配置本地 GeoIP 库时，国家代码匹配会回退到 `global` 作用域，详见 `docs/design/authoritative-dns-gslb.md`。
* 手动填写的 DNS 记录内容不会被后台覆盖；A/AAAA 可用逗号、空格或换行填写多个目标。
* 多域名规则默认会同步规则里的所有域名；单域名规则可在详情页手动指定记录名称。
* 删除规则时，如果该规则曾由 DuShengCDN 创建 DNS 记录，会尝试同步删除对应 Cloudflare DNS 记录。

DDoS 自动切换橙云：

* `DDoS 防护模式` 设为 `自动` 后，系统会按最近 5 分钟请求聚合判断是否需要打开 Cloudflare 代理。
* 默认请求量阈值为 `20000`，默认错误率阈值为 `30%`。
* 达到任一阈值后，后台巡检会把该规则的 Cloudflare DNS 记录切换为橙云代理。
* 阈值可在管理端设置项中调整：`CloudflareDDoSRequestThreshold` 和 `CloudflareDDoSErrorRateThreshold`。

注意：Cloudflare 自动 DNS 只负责 DNS 记录与橙云状态，不会替代发布流程。反向代理配置修改后仍需发布并激活版本，Agent 才会拉取并应用 OpenResty 配置。

### 3. 安装 Agent

本地安装脚本会自动检测 Linux / macOS 环境，缺少 OpenResty 时会尝试通过系统包管理器安装；Docker 方式则使用内置 OpenResty 的 Agent 镜像。
你可以在控制面板的节点管理->详情->节点信息->节点标识与部署复制安装命令，或直接使用下面的脚本：

#### Docker 部署

Docker 部署可直接运行 Agent 镜像：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:latest
```

Docker Agent 镜像已内置 OpenResty、`libmaxminddb`、`lua-resty-maxminddb` 和 `lua-resty-http`。如需接入自建 IP 精确查询 API，可追加：
```bash
-e DUSHENGCDN_GEOIP_LOOKUP_API_URL=https://ipdb.example.com/lookup \
-e DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN=YOUR_API_TOKEN
```

#### 本地部署

使用 `discovery_token` 接入：

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

可选接入自建 IP 精确查询 API：
```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN \
  --geoip-api-url https://ipdb.example.com/lookup \
  --geoip-api-token YOUR_API_TOKEN
```

安装脚本默认写入 `/opt/dushengcdn-agent`，创建 `dushengcdn-agent.service`，自动查找或安装 `openresty`，并可重复执行以重装或升级 Agent。脚本会优先下载 GitHub Release 中的 Agent 二进制；如果当前仓库还没有 Release，会自动安装 Go 并从源码构建，源码构建会把当前 Git 版本写入 Agent，避免节点版本显示为 `dev`。如需禁用依赖自动安装，可追加 `--no-install-deps`；OpenResty 使用自定义路径时可追加 `--openresty-path /path/to/openresty`。

依赖安装兼容性：

* Linux / macOS 会自动检查 `curl`、`tar`、`OpenResty`、`libmaxminddb`、`lua-resty-maxminddb`、`lua-resty-http`、构建工具等运行依赖，缺少时尝试通过系统包管理器和 `opm` 安装。
* Debian 13 `trixie` 暂无 OpenResty 官方源时，脚本会回退到 Debian `bookworm` 源；遇到新版 apt 拒绝旧签名策略时，会临时使用 OpenResty 官方 HTTPS 源完成安装，并在安装后移除临时源。
* 新装 OpenResty 后，脚本会阻止系统自带 `openresty` 服务自动启动，避免它提前占用 `80` / `443` 端口；端口由 `dushengcdn-agent` 托管。

### 4. 可选：部署自建权威 DNS Worker

如果要让域名按每次 DNS 查询来源实时调度到不同边缘节点，需要在管理端左侧「权威 DNS」创建 DNS Zone 和 DNS Worker Token，然后打开「迁移向导」检查 Cloudflare 模式网站的 Zone、Worker 和 GSLB 准备状态，再把域名 NS 委派到 DNS Worker，并在网站详情「自动 DNS」里切换到自建权威 DNS。完成注册商 NS 配置后，可以在 Zone 详情点击「检查委派」确认公网 NS 是否匹配；如果使用 `ns1.example.com` 这类 Zone 内 NS，还需要在注册商配置 Glue/主机记录。

Docker 运行示例：

```bash
docker run -d --name dushengcdn-dns-worker --restart unless-stopped \
  -p 53:53/udp -p 53:53/tcp \
  -v dushengcdn-dns-worker-data:/data \
  -e DUSHENGCDN_DNS_WORKER_SERVER_URL=https://cdn.example.com \
  -e DUSHENGCDN_DNS_WORKER_TOKEN=YOUR_DNS_WORKER_TOKEN \
  -e DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=200 \
  -e DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=1232 \
  ghcr.io/satands/dushengcdn-dns-worker:latest
```

源码运行示例：

```bash
cd dushengcdn_server
go run ./cmd/dns-worker \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen :53 \
  --query-rate-limit 200 \
  --udp-response-size 1232
```

验证示例：

```bash
dig @YOUR_DNS_WORKER_IP example.com SOA
dig @YOUR_DNS_WORKER_IP www.example.com A
```

生产环境建议至少部署两个 DNS Worker，并同时放行 UDP/TCP `53`。Worker 本地快照缓存会写入 SHA-256 checksum 元数据，启动加载时会校验完整性，并从快照中的 GSLB 防抖状态恢复最近可用选择；运行中产生的新防抖状态会随 heartbeat 批量回传 Server，同时兼容旧版本生成的裸快照 JSON。Worker 默认按来源 IP 每秒限制 `200` 次查询，并把 UDP 响应上限限制为 `1232` 字节；超大响应会设置 TC 位让递归解析器回退 TCP。如果要按国家代码匹配 GSLB 节点池，可配置本地 MaxMind Country MMDB；如果在节点池里配置来源 CIDR，则会优先按来源 IP/ECS 命中 `cidr:...` 作用域；启用 `weighted` 或 `load_aware` 时会追加 `|bucket:xx` 分流桶，未命中且无法识别国家时会回退到 `global` 作用域。

Worker 上报心跳后，左侧「权威 DNS」会展示最近 24 小时的查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、Worker 快照一致性、Worker 查询延迟、可用率、错误率、最近公网探测健康状态、来源作用域、Worker/Zone/站点维度、返回目标分布和当前调度状态，便于确认实时 GSLB 是否按预期分流；「GSLB 调度状态」会列出每个站点、A/AAAA、`global`、`country:HK`、`cidr:203.0.113.0/24` 或 `global|bucket:42` 等来源作用域的当前实际目标、期望目标和防抖状态；「GSLB 调度模拟」可在真实流量到达前按站点、记录类型、来源 IP 和来源国家代码预演当前快照会返回的边缘 IP，并展示节点池匹配、候选节点、跳过节点和原因。这里的延迟是 Worker 本地处理真实 DNS 查询的耗时，不是用户到多地 NS 的公网 RTT。DNS Worker 列表的「探测」会由 Server 对 Worker 公网地址发起 UDP/TCP 53 SOA 查询，适合检查端口映射、防火墙和公网可达性；最近一次探测结果会保存在 Worker 列表和可用性面板中，并会作为迁移向导的切换准备条件。Zone 详情的委派检查用于确认注册商 NS 和 Glue 配置是否到位。

### 5. 卸载 Agent

如需彻底卸载 Agent 并清空本地数据，可执行：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

卸载脚本会先停止并移除 `dushengcdn-agent.service`、删除整个 `/opt/dushengcdn-agent` 目录，不会删除本机 OpenResty。

在管理端删除在线节点时，Server 会通过 Agent 连接下发卸载指令；Agent 收到后会执行本机卸载流程并退出。节点离线时，面板只会删除节点记录，需要你后续在节点服务器上手动执行卸载脚本。

### 6. 发布第一份配置

1. 登录管理端并新增网站配置
2. 在发布前查看预览或变更摘要
3. 激活新版本
4. Agent 通过 WebSocket 通知或后续 heartbeat 拉取并应用配置

版本号格式固定为 `YYYYMMDD-NNN`，历史版本不可变，回滚通过重新激活旧版本完成。

源站填写说明：

* `源站` 页面维护的是可复用地址目录，只填写 IP、域名或主机名，例如 `10.0.0.10`、`origin.internal`，不要填写协议和端口。
* `规则配置` 里的 `源站地址` 需要填写完整 URL，协议和端口都在这里配置，例如 `https://origin.internal:443`。
* 多源站负载均衡时，每行一个完整 URL，并保持相同协议；多源站模式不要填写 path 或 query。
* 界面中已统一使用 `源站地址` 命名；旧文档或旧习惯里的“上游地址”在这里都对应 `源站地址`。

GeoIP 地区限制说明：

* GeoIP 地区限制基于节点侧 OpenResty 实时执行，用于按国家或地区代码放行或拦截访问；功能不依赖 Cloudflare 橙云，适合自建 CDN 节点直接使用。
* 进入 `网站配置` -> 选择站点 `配置` -> `地区限制` 分区，可以按国家或地区代码限制访问。
* `拦截列表内地区`：列表内地区返回 `403`，无法识别地区的请求继续放行。
* `只允许列表内地区`：只有列表内地区可以访问，无法识别地区的请求也会返回 `403`。
* 国家或地区代码使用 ISO 3166-1 两位代码，例如 `CN`、`US`、`HK`，可一行一个，也可以用逗号分隔。
* 地区识别默认依赖 Agent 节点本地 `GeoLite2-Country.mmdb` 数据库，不再要求 Cloudflare 橙云或 `CF-IPCountry` 请求头；OpenResty 会按真实客户端 IP 查询国家码。
* 真实 IP 优先读取 `CF-Connecting-IP`、`X-Real-IP`、`X-Forwarded-For`，最后使用连接 IP；前置 HTTPS 反代需要正确透传这些请求头。
* OpenResty 会缓存 `IP -> 国家码`，默认缓存有效识别结果 24 小时；本地库和在线 API 都查不到时会短暂缓存未知结果，避免每个请求重复查库。
* Agent 会自动下载并更新 Country 数据库，默认来源为 `https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb`，默认路径为 Agent 数据目录下的 `var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb`，默认每 24 小时检查更新一次。
* 如你后续搭建自己的在线 IP 精确查询服务，可在 Agent 配置里设置 `geoip_lookup_api_url` 和 `geoip_lookup_api_token`，或安装时追加 `--geoip-api-url`、`--geoip-api-token`；查询顺序是先本地 GeoIP，未识别到国家码时再请求 API。API 返回 JSON 中的 `country_code`、`countryCode`、`iso_code`、`isoCode` 或 `country` 字段均可识别。
* 修改地区限制后需要发布并激活新配置，Agent 应用 OpenResty 配置后才会在节点侧生效。

本地 WAF 说明：

* WAF 是独立于 GeoIP 的可选本地模块，默认关闭；GeoIP 负责“哪个地区能访问”，WAF 负责“请求是否像攻击或扫描”。
* 进入 `网站配置` -> 选择站点 `配置` -> `WAF 防护` 分区，可以为单个站点启用 WAF。
* 节点访问处理顺序为：真实 IP 识别 -> GeoIP 地区限制 -> WAF -> PoW -> 反向代理源站。
* `观察模式` 只记录命中的规则并继续放行，适合上线前观察误杀；`拦截模式` 会对命中的请求直接返回 `403`。
* 内置规则支持 SQL 注入、XSS、路径穿越、敏感路径扫描和常见恶意工具 User-Agent。
* 白名单支持 IP、IP CIDR、路径；路径支持精确匹配，也支持以 `*` 结尾的前缀匹配，例如 `/api/public/*`。
* 自定义拦截规则支持路径包含、路径正则、查询参数包含、请求头包含和 User-Agent 包含。
* 当前 WAF 是轻量本地规则引擎，不是完整 ModSecurity / OWASP CRS；优点是部署简单、资源占用低，复杂规则集可后续再扩展。
* 修改 WAF 后需要发布并激活新配置，Agent 应用 OpenResty 配置后才会在节点侧生效。

界面交互说明：

* 操作结果提示已统一为右上角浮层，提示内容会展示更具体的错误信息，按内容自动适配宽度，并在默认 8 秒后自动消失。
* 删除、回滚、禁用等需要确认的高风险操作会使用页面居中的主题化确认弹窗，避免浅色主题白底白字或深色主题黑底黑字。
* 节点详情页会静默刷新运行状态，不再因为自动刷新反复显示“刷新中”；只有手动点击更新、同步等按钮时才显示操作中的状态。
* 节点 IP 会优先结合 `X-Forwarded-For`、`X-Real-IP`、`CF-Connecting-IP` 等反代头识别真实公网 IP，所以 HTTPS 反代必须保留上面的请求头配置。

### 7. 更新面板与 Agent

如果服务器上还没有源码目录，先克隆自己的仓库：

```bash
git clone https://github.com/SatanDS/DuShengCDN.git /opt/dushengcdn
```

服务器使用 Docker Compose 部署时，更新面板端：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose up -d --build
docker compose ps
```

节点使用 Docker Compose 部署 Agent 时，更新节点端：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build
docker compose -f docker-compose.agent.yaml ps
```

如果服务器上直接改过仓库里的 `docker-compose.yaml`，例如改端口到 `3010:3000`，拉取时可能提示本地改动会被覆盖。请先记录本地端口、DSN、密码和 Token；确认没有需要保留的源码修改后，再使用 `git fetch origin main && git reset --hard origin/main` 拉回新版。源码 Compose 构建时会通过 `DUSHENGCDN_VERSION` 把当前 Git 版本写入 Server 或 Agent；顶栏“版本”显示当前运行中的后端版本，节点列表显示 Agent 上报的版本。

节点使用安装脚本部署 Agent 时，可重复执行安装命令进行重装或升级；Agent 自动更新开启后，会从当前仓库 Release 下载对应平台二进制并校验 `.sha256` 后替换本地可执行文件。没有 Release 资产时，安装脚本会从源码构建并写入当前 Git 版本。

### 7. 发布 Release 与 latest

仓库已提供 GitHub Actions 用于生成 GitHub Release 和 GHCR 镜像，方便后续直接部署：

* 发布二进制：进入 GitHub 仓库 `Actions` -> `Release` -> `Run workflow`，填写版本号，例如 `v1.0.0` 或 `v1.0.0-beta`。工作流会构建 Server、Agent 多平台二进制并上传到 Release。
* `v1.0.0` 这类纯数字版本会作为正式 Release；`v1.0.0-beta` 这类带后缀版本会作为 prerelease。GitHub 的 `releases/latest` 会指向最新正式 Release。
* 发布 Docker 镜像：进入 `Actions` -> `Docker image builds` -> `Run workflow`，填写同一个版本号。工作流会推送 `ghcr.io/satands/dushengcdn:<version>`、`ghcr.io/satands/dushengcdn:latest`、`ghcr.io/satands/dushengcdn-agent:<version>` 和 `ghcr.io/satands/dushengcdn-agent:latest`。
* 安装脚本会优先读取 `https://github.com/SatanDS/DuShengCDN/releases/latest` 中的 Agent 资产；没有匹配资产时才回退到源码构建。
* GitHub Actions 已切换到支持 Node.js 24 的动作版本，并显式启用 `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24`，用于规避 Node.js 20 弃用警告。


## 界面预览

### 仪表盘总览

![DuShengCDN dashboard overview](./docs/assets/readme/dashboard-overview.png)

### 节点详情

![DuShengCDN node detail](./docs/assets/readme/node-detail.png)

### 配置新增

![DuShengCDN version release](./docs/assets/readme/proxy-route-detail.png)

## 管理端与接口

管理端当前覆盖：

* 网站配置
* 配置版本
* 节点管理
* 应用记录
* TLS 证书
* 域名管理
* 用户管理
* 设置
* 版本更新
* POW 规则

登录管理端后，可访问 Swagger UI：`/swagger/index.html`

## 开源协议

本项目采用 [Apache License 2.0](./LICENSE) 开源。

## Star History

<a href="https://www.star-history.com/?repos=SatanDS%2FDuShengCDN&type=date&legend=bottom-right">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=SatanDS/DuShengCDN&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=SatanDS/DuShengCDN&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=SatanDS/DuShengCDN&type=date&legend=top-left" />
 </picture>
</a>
