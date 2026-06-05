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

本次更新把项目从基础控制面推进到更接近商用交付的 CDN 管理平台：

* 商业化能力：新增离线 Ed25519 商业授权、许可证签发 CLI、授权状态面板、节点/站点额度约束、高级功能开关和强制授权部署参数。
* 生产安全与部署：补齐 `SESSION_SECRET`、Redis 强依赖、CORS 白名单、全局/API/上传下载/敏感接口限流、Turnstile、SMTP TLS、OAuth 状态校验、备份/恢复/诊断脚本和 Compose `.env` 升级流程。
* 主入口重组：面板导航调整为“运营总览、流量调度、边缘资源、证书与域名、交付发布、访问观测、系统治理”，证书旧入口重定向到 canonical 路径，系统治理支持按 tab 深链和角色过滤。
* 调度与 DNS：节点支持节点池、公网 IP 池、标签、池内权重、调度开关和排空模式；Cloudflare 自动解析支持多 A/AAAA 目标、健康优先、池内权重、负载感知和攻击期切换；本地自建解析落地 DNS Worker、只读快照、逐查询 GSLB、来源网段/国家代码匹配、稳定权重分流桶、查询限速、UDP 截断保护、委派检查、Glue 提示和迁移向导。
* 观测与运维：访问日志、请求报告和指标快照按固定分片写入；面板提供查询趋势、SERVFAIL/NXDOMAIN、来源作用域、返回目标分布、Worker 延迟/可用率、快照一致性、节点探测、带宽 P95、TOP URL/IP/地区和节点可用率。
* 站点能力：网站配置详情页聚合负载均衡、缓存策略、CC 防护、地区限制和认证配置；支持目标节点池缓存清理、首页预热、轻量恶意请求规则、计算验证挑战、源站池、回源失败切换和多源站校验。
* 性能优化：消除了多处 N+1 查询，DNS 调度状态与查询汇总改为批量加载/批量写入，观测去重改为固定分片 `UNION ALL` fast path，访问日志 IP 汇总直接在 SQL 中取最新地区/运营商，TLS 证书引用删除校验改为窄字段查询，DNS Worker 健康统计复用已加载 worker 列表。
* 兼容修复：Cloudflare API Token 兼容原始 Token、`Bearer ...` 和 JSON 包装；Agent/OpenResty 安装增强 Debian 13、GeoIP、依赖安装、版本写入和默认服务占用处理；升级脚本避免覆盖已有 DNS Worker 和生产 `.env`。
* 测试覆盖：新增商业授权、Redis、CORS、限流、Turnstile、SMTP、文件模型、配置版本、仪表盘、观测分片、DNS Worker、TLS helper、Agent 安装/升级/观测等回归测试；Server、Agent、Web lint/typecheck/test/build 均已通过。

## 核心能力

* 反向代理网站配置与多域名绑定
* 配置预览、发布、激活与历史回滚
* 节点程序（Agent）自动注册、心跳、同步、校验、重载与失败回滚
* 代理服务主配置、性能参数、缓存参数与 Lua 资源托管
* 本地 GeoIP 地区限制、真实客户端 IP 国家码识别、IP 国家码缓存与在线精确查询 API 回退
* 可选站点级 CC 防护，整合访问频率限制、计算验证挑战和本地轻量恶意请求规则
* 网站配置详情页提供负载均衡、缓存策略、CC 防护、地区限制和认证配置分区
* TLS 证书、域名资产、节点凭证与版本状态管理
* 节点池、公网 IP 池、池内权重调度、排空模式与 Cloudflare 多目标自动解析 / 多节点智能解析
* Cloudflare 自动解析、在线节点自动解析、节点离线解析切换、多节点智能解析防抖状态与攻击自动切换 Cloudflare/自定义清洗池
* 本地自建解析控制面、响应端快照/心跳 API、UDP/TCP 53 查询响应端、查询限速、UDP 响应截断保护、NS 委派检查、Glue 提示、Cloudflare 迁移向导、逐查询实时多节点智能解析、来源网段/国家代码匹配、稳定权重分流桶、当前调度状态、查询趋势、来源作用域分布、响应端 GeoIP 状态、响应端查询延迟/可用性、按需响应端探测健康状态和快照一致性观测
* 站点级缓存策略、缓存清理、首页预热、缓存命中与回源健康统计
* 商业授权管理，支持离线签名许可证、授权状态面板、节点/站点额度、高级能力开关和强制授权部署
* 生产安全治理，支持 Redis 强依赖、CORS 白名单、全局/接口/上传下载/敏感操作限流、Turnstile、SMTP TLS 与 OAuth 状态校验
* 管理端主入口分组、可折叠侧栏、系统治理深链、角色可见性控制和商业/安全/数据保留设置归位
* 观测数据自动保留、手动清理、固定分片写入、批量汇总和大表查询性能优化
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
      SESSION_SECRET: ${SESSION_SECRET:?set SESSION_SECRET in .env}
      DSN: postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
      GIN_MODE: release
      LOG_LEVEL: info
      # 可选：商用/多实例部署建议启用 Redis。
      # REDIS_CONN_STRING: redis://redis:6379/0
      # REDIS_REQUIRED: "true"

volumes:
  postgres-data:
```

```bash
docker compose up -d
```

访问地址：`http://localhost:3000`

更多可复制的 Compose 模板放在 `examples/compose/`：包括 GHCR 镜像生产部署、源码构建部署、端口/数据目录 override、Agent 和 DNS Worker。生产部署推荐从这些模板复制到独立目录后改 `.env`，不要直接修改仓库内 Compose 文件。

商用私有部署可在 `.env` 中设置 `DUSHENGCDN_LICENSE_REQUIRED=true` 与 `DUSHENGCDN_LICENSE_PUBLIC_KEYS`，再到管理端「设置 -> 商业授权」安装许可证；公钥支持 base64url、标准 base64 或 hex 编码的 Ed25519 公钥。源码仓库内置离线签发工具：

```bash
cd dushengcdn_server
go run ./cmd/license keygen
go run ./cmd/license sign \
  -private-key "$DUSHENGCDN_LICENSE_PRIVATE_KEY" \
  -license-id lic-2026-001 \
  -customer-name "Example Ltd." \
  -plan enterprise \
  -features all \
  -max-nodes 20 \
  -max-sites 200 \
  -expires-at 2027-12-31
```

源码 Compose 部署时，也可以在仓库根目录使用一体化脚本启动面板并默认安装同机 DNS Worker。脚本会在首次部署时自动生成 `.env` 里的数据库密码、`SESSION_SECRET` 和 `DSN`；如果升级旧源码部署且检测到已有 `dushengcdn_server/postgres-data`，会保留 `.env.example` 中的数据库密码和 DSN，避免旧 PostgreSQL 数据目录因密码不一致导致面板打不开。脚本会先检查本机是否已部署 DNS Worker；发现已有 `dushengcdn-dns-worker.service`、同名 systemd unit 文件、`/opt/dushengcdn-dns-worker`、Worker 环境文件、同名 Docker 容器、Worker 进程或 DuShengCDN 监听 `53` 端口时，会跳过 Worker 自动安装，避免覆盖现有配置。

```bash
cd /opt/dushengcdn
bash scripts/install-server.sh --public-ip 203.0.113.10
```

只部署面板可使用：

```bash
bash scripts/install-server.sh --skip-dns-worker
```

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

### 2. 配置 Cloudflare 自动解析与防护

如果域名已经接入 Cloudflare，可以让 DuShengCDN 自动创建或更新 DNS 记录，并在节点离线或遇到攻击流量时自动调整解析策略。

准备 Cloudflare API Token：

* 权限需要包含 `Zone Read` 和 `DNS Edit`
* Token 范围建议只授权给需要托管的 Zone
* 不建议使用 Global API Key
* 可以直接填写原始 Token，也兼容 `Bearer ...` 或包含 `api_token` / `apiToken` / `token` 的 JSON

在管理端操作：

1. 在左侧「Cloudflare 账号」准备 Cloudflare 账号。
2. 在节点详情中维护节点池、公网 IP 池、池内权重、调度开关和排空状态。
3. 新建网站配置时选择默认节点池，并开启 `创建时启用负载均衡`；已有站点可在详情页的 `负载均衡` 分区维护。
4. 选择 Cloudflare 账号，记录类型通常选择 `A`；IPv6 节点选择 `AAAA`。
5. `记录内容` 留空时，系统会自动选择该节点池中的在线公网 IP，并把 `自动选择在线节点 IP` 打开。
6. 如需跨 HK、EU 等多个节点池分流，在「负载均衡」分区启用 `多节点智能解析`，点击 `+` 逐行添加真实节点池、池权重、可选国家代码和来源网段，例如池名 `hk`、池权重 `80`、国家代码 `HK,TW`、来源网段 `203.0.113.0/24`。来源网段会优先于国家代码匹配。
7. 如需正常状态也隐藏源站，可开启 `常态开启 Cloudflare 代理`；如需攻击期自动切换，将 `攻击防护模式` 设为 `自动`，再选择 `Cloudflare` 或 `自定义清洗池`。

自动解析行为：

* 创建网站时会立即向 Cloudflare 创建或更新 DNS 记录。
* 后台每 1 分钟巡检一次已开启自动解析的规则。
* 开启 `自动选择在线节点 IP` 后，节点离线、代理服务不健康、节点被排空、关闭调度或节点公网 IP 池没有对应 A/AAAA 地址时会跳过该节点。
* 反向代理里的默认节点池是默认承载池：未启用多节点智能解析时，自动解析从这里选公网 IP，缓存清理/预热也下发到这里。启用多节点智能解析后，A/AAAA 返回 IP 改由「负载均衡」里的节点池权重决定，默认节点池仍作为缓存、攻击防护回退和运行时兜底。
* 自动解析可以按“健康优先（冷却防抖）”、节点池内权重或负载感知评分选择，并支持同步多个 A/AAAA 目标。健康优先只判断在线、代理服务健康、调度开关、排空状态和最近心跳；处理器、内存、连接数只属于负载感知。
* 多节点智能解析模式可绑定多个节点池，按来源网段、国家代码、池权重、节点池内权重、代理服务连接数、处理器压力、内存压力和负载阈值选择解析目标；网站配置里可维护最大连接数、最大处理器压力和最大内存压力。
* 本地自建解析模式下，选择按权重优先或按压力优先时，会按来源 IP/ECS 生成稳定分流桶，所以 HK 池权重 80、EU 池权重 20 这类配置会在不同来源桶之间形成接近 8:2 的解析答案分布；Cloudflare 模式只同步一组静态记录，不具备逐查询来源分流。
* 多节点智能解析会记录最近一次实际目标和期望目标，旧目标仍健康且冷却时间未到时不会反复切换。
* Cloudflare 同步模式不是逐请求实时调度，而是后台巡检重算并同步记录；实际流量还会受到解析缓存时间和运营商 DNS 缓存影响。
* 本地自建解析模式需要把域名 NS 委派到 DuShengCDN DNS 响应端（DNS Worker）；响应端会在查询时实时执行多节点智能解析。左侧「本地自建解析」的「迁移向导」可检查 Cloudflare 模式网站是否已有匹配托管域名、在线响应端、公网 UDP/TCP 53 探测和多节点策略，满足条件时可一键切换到本地自建解析；「GSLB 调度模拟」可按来源 IP 和国家代码预演当前快照返回目标，并解释节点候选/跳过原因、负载指标时间和节点多地探测摘要。节点多地探测默认仅用于观测；在设置页「本地解析运行参数」启用按响应端探测结果筛选节点后，无新鲜成功探测的边缘节点不会进入本地自建解析候选。托管域名详情可检查公网 NS 是否匹配，并在域名内 NS 需要 Glue/主机记录时提示。未配置本地 GeoIP 库时，国家代码匹配会回退到 `global` 作用域，详见 `docs/design/authoritative-dns-gslb.md`。
* ACME 申请证书可选择 Cloudflare 账号验证，也可选择本地自建解析托管域名验证；本地方式会临时写入 `_acme-challenge` TXT 记录并在验证后清理。
* 手动填写的 DNS 记录内容不会被后台覆盖；A/AAAA 可用逗号、空格或换行填写多个目标。
* 多域名规则默认会同步规则里的所有域名；单域名规则可在详情页手动指定记录名称。
* 删除规则时，如果该规则曾由 DuShengCDN 创建 DNS 记录，会尝试同步删除对应 Cloudflare DNS 记录。

攻击自动防护：

* `攻击防护模式` 设为 `自动` 后，系统会按最近 5 分钟请求聚合判断是否进入攻击期。
* 默认请求量阈值为 `20000`，默认错误率阈值为 `30%`。
* 防护提供方选 `Cloudflare` 时，攻击期会暂停多节点智能解析，多 A/AAAA 目标临时回到网站默认节点池，并强制同步 Cloudflare 橙云；指标恢复正常后，下一轮巡检回到原来的固定记录、默认节点池或多节点智能解析策略，并恢复 `常态开启 Cloudflare 代理` 的设置。
* 防护提供方选 `自定义清洗池` 时，攻击期会暂停 GSLB，只把 DNS 解析到指定节点/IP 池里的在线公网 IP，并关闭 Cloudflare 橙云代理，适合切到自有抗 D 清洗入口；指标恢复正常后自动回到正常调度。
* 阈值可在管理端设置项中调整：`CloudflareDDoSRequestThreshold` 和 `CloudflareDDoSErrorRateThreshold`。

注意：Cloudflare 自动解析只负责 DNS 记录与橙云状态，不会替代发布流程。反向代理配置修改后仍需发布并激活版本，Agent 才会拉取并应用 OpenResty 配置。

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

安装脚本默认写入 `/opt/dushengcdn-agent`，创建 `dushengcdn-agent.service`，自动查找或安装 `openresty`，并可重复执行以重装或升级 Agent。脚本会优先下载 GitHub Release 中的 Agent 二进制；如果当前仓库还没有 Release，会自动安装 Go 并从源码构建，源码构建会把当前 Git 版本写入 Agent，避免节点版本显示为 `dev`。源码构建会优先复用本机已有 Go；确实需要下载 Go 时会按多个官方源重试，也可通过 `DUSHENGCDN_GO_DOWNLOAD_BASE_URLS` 或 `DUSHENGCDN_GO_DOWNLOAD_URL` 指定下载源。如需禁用依赖自动安装，可追加 `--no-install-deps`；OpenResty 使用自定义路径时可追加 `--openresty-path /path/to/openresty`。

依赖安装兼容性：

* Linux / macOS 会自动检查 `curl`、`tar`、`OpenResty`、`libmaxminddb`、`lua-resty-maxminddb`、`lua-resty-http`、构建工具等运行依赖，缺少时尝试通过系统包管理器和 `opm` 安装。
* Debian 13 `trixie` 暂无 OpenResty 官方源时，脚本会回退到 Debian `bookworm` 源；遇到新版 apt 拒绝旧签名策略时，会临时使用 OpenResty 官方 HTTPS 源完成安装，并在安装后移除临时源。
* 新装 OpenResty 后，脚本会阻止系统自带 `openresty` 服务自动启动，避免它提前占用 `80` / `443` 端口；端口由 `dushengcdn-agent` 托管。

### 4. 可选：部署本地自建解析 Worker

如果要让域名按每次 DNS 查询来源实时调度到不同边缘节点，需要在管理端左侧「本地自建解析」创建 DNS Zone 和 DNS Worker，然后打开「迁移向导」检查 Cloudflare 模式网站的 Zone、Worker、公网探测和 GSLB 准备状态；满足条件时可点击「一键切换」，也可以在网站详情「负载均衡」里手动切换到本地自建解析。之后把域名 NS 委派到 DNS Worker。完成注册商 NS 配置后，可以在 Zone 详情点击「检查委派」确认公网 NS 是否匹配；如果使用 `ns1.example.com` 这类 Zone 内 NS，还需要在注册商配置 Glue/主机记录。面板本机可以同时部署 DNS Worker；使用 `scripts/install-server.sh` 部署面板时，脚本可默认自动创建名为 `DNS服务响应端` 的 Worker、探测公网 IP 并安装同机 Worker。手动或多机部署时，仍可复制 Token 后单独运行 DNS Worker 安装脚本、Docker 或源码命令。

推荐使用安装脚本部署 DNS Worker：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN
```

脚本默认写入 `/opt/dushengcdn-dns-worker`，创建 `dushengcdn-dns-worker.service`，监听 UDP/TCP `53`，并把快照缓存保存在安装目录的 `data/dns-worker-snapshot.json`。启动服务前会检查默认监听端口是否已被其它进程占用；如果本机已有 `systemd-resolved`、`named`、`dnsmasq` 等本地 DNS 服务，请先停用/改端口，或用 `--listen PUBLIC_IP:53` 只绑定 Worker 公网地址。脚本会优先下载 GitHub Release 中的 DNS Worker 二进制；如果当前仓库还没有 Release，会自动安装 Go 并从源码构建，源码构建会把当前 Git 版本写入 Worker，避免版本显示为 `dev`。源码构建同样会复用本机已有 Go，并在自动下载 Go 时多源重试。脚本还会默认下载 Country MMDB 到 `data/geoip/GeoLite2-Country.mmdb`，用于国家代码节点池匹配；可用 `--geoip-database` 指向已有文件、用 `--geoip-database-url` 指定自建下载源，或用 `--no-geoip-download` 关闭自动下载。

如果 Worker 和面板在同一台机器，`--server-url` 可以使用面板本机可访问地址，`--listen` 建议显式绑定公网地址，例如 `--listen 203.0.113.10:53`。安装后用 `systemctl status dushengcdn-dns-worker`、`ss -lntup | grep ':53'`、`ss -lnuap | grep ':53'` 和 `dig @PUBLIC_IP example.com SOA` 验证。

也可以在 Worker 主机运行只读诊断脚本，一次性检查服务、监听、快照、日志和 SOA/NS 查询：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip PUBLIC_IP --zone example.com
```

面板和 DNS Worker 同机部署时，正式切换 NS 前可运行闭环验收脚本：

```bash
cd /opt/dushengcdn
bash scripts/verify-authoritative-dns.sh --public-ip PUBLIC_IP --zone example.com
```

Docker 运行示例也可继续使用：

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
  --query-rate-limit 200 \
  --udp-response-size 1232
```

验证示例：

```bash
dig @YOUR_DNS_WORKER_IP example.com SOA
dig @YOUR_DNS_WORKER_IP www.example.com A
```

生产环境建议至少部署两个 DNS Worker，并同时放行 UDP/TCP `53`。如果安装脚本或 Docker 启动提示 `address already in use` / 端口占用，先用 `ss -lntu '( sport = :53 )'` 或 `lsof -nP -i :53` 找到占用者；常见占用来自 `systemd-resolved`、`named` 或 `dnsmasq`。Worker 本地快照缓存会写入 SHA-256 checksum 元数据，启动加载时会校验完整性，并从快照中的 GSLB 防抖状态恢复最近可用选择；运行中产生的新防抖状态会随 heartbeat 批量回传 Server，同时兼容旧版本生成的裸快照 JSON。Worker 默认按来源 IP 每秒限制 `200` 次查询，并把 UDP 响应上限限制为 `1232` 字节；超大响应会设置 TC 位让递归解析器回退 TCP。安装脚本会默认准备本地 Country MMDB；如果使用 Docker 或源码方式部署且要按国家代码匹配 GSLB 节点池，需要自行配置本地 MaxMind Country MMDB。如果在节点池里配置来源 CIDR，则会优先按来源 IP/ECS 命中 `cidr:...` 作用域；启用 `weighted` 或 `load_aware` 时会追加 `|bucket:xx` 分流桶，未命中且无法识别国家时会回退到 `global` 作用域。

DNS 响应端上报心跳后，左侧「本地自建解析」会展示最近 24 小时的查询量、查询趋势、SERVFAIL/NXDOMAIN 趋势、响应端快照一致性、响应端查询延迟、可用率、错误率、最近公网探测健康状态、GeoIP 国家库加载状态、来源作用域、响应端/托管域名/站点维度、返回目标分布和当前调度状态，便于确认实时多节点智能解析是否按预期分流；「GSLB 调度状态」会列出每个站点、A/AAAA、`global`、`country:HK`、`cidr:203.0.113.0/24` 或 `global|bucket:42` 等来源作用域的当前实际目标、期望目标和防抖状态；「GSLB 调度模拟」可在真实流量到达前按站点、记录类型、来源 IP 和来源国家代码预演当前快照会返回的边缘 IP，并展示节点池匹配、候选节点、跳过节点、负载指标时间和节点多地探测摘要。这里的响应端延迟是 DNS 响应端本地处理真实 DNS 查询的耗时；节点多地探测 RTT 表示各边缘节点到响应端 NS 的主动探测耗时，默认只用于观测与排障；开启「按响应端探测结果筛选节点」后，才会影响本地自建解析选点。DNS 响应端列表的「探测」会由面板对响应端公网地址发起 UDP/TCP 53 SOA 查询，适合检查端口映射、防火墙和公网可达性；最近一次探测结果会保存在响应端列表和可用性面板中，并会作为迁移向导的切换准备条件。托管域名详情的委派检查用于确认注册商 NS 和 Glue 配置是否到位。

卸载 DNS Worker：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-dns-worker.sh | bash
```

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

站点 CC 防护说明：

* CC 防护是站点级入口，默认关闭；里面包含访问频率防护、计算验证和恶意请求规则。
* 进入 `网站配置` -> 选择站点 `配置` -> `CC 防护`，可以为单个站点启用短时间高频访问识别、命中后拦截或转入计算验证，并按需打开本地恶意请求规则。
* 节点访问处理顺序为：真实 IP 识别 -> GeoIP 地区限制 -> 恶意请求规则 -> CC 频率判断 -> 计算验证 -> 反向代理源站。
* CC 防护、计算验证和恶意请求规则里的 IP/CIDR 白名单、黑名单、排除规则均支持 IPv4 与 IPv6，例如 `203.0.113.0/24`、`2001:db8::/32`。
* CC 频率命中可选择 `观察模式`、`拦截模式` 或 `转入计算验证`；转入计算验证时会使用同一入口下方的计算验证算法、难度、白名单和黑名单配置。
* 恶意请求规则支持观察/拦截模式，内置规则包括 SQL 注入、XSS、路径穿越、敏感路径扫描和常见恶意工具 User-Agent。
* 白名单、排除名单和自定义拦截规则按需添加；路径支持精确匹配，也支持以 `*` 结尾的前缀匹配，例如 `/api/public/*`。
* 当前恶意请求规则是轻量本地规则引擎，不是完整 ModSecurity / OWASP CRS；优点是部署简单、资源占用低，复杂规则集可后续再扩展。
* 修改 CC 防护后需要发布并激活新配置，Agent 应用 OpenResty 配置后才会在节点侧生效。命中规则发布到节点并产生新访问后，可在「观测计量」->「访问明细」的状态码旁查看 `!` 了解原因。

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

服务器使用 Docker Compose 源码部署时，更新面板端：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh

git fetch origin main
git pull --ff-only origin main

cd dushengcdn_server
cp -n .env.example .env

DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
docker compose logs -n 100 dushengcdn
```

首次部署时先编辑 `dushengcdn_server/.env`，写入真实的 `DUSHENGCDN_HTTP_PORT`、`POSTGRES_PASSWORD`、`SESSION_SECRET`、`DSN` 和旧版 Agent 兼容需要的 `AGENT_TOKEN`。后续升级不要在命令里重新生成 `SESSION_SECRET` 或替换数据库密码，避免已经登录的会话失效或 PostgreSQL 连接失败。

如果服务器上直接改过仓库里的 `docker-compose.yaml`，例如改端口到 `3010:3000`，拉取时可能提示本地改动会被覆盖。请先记录本地端口、DSN、密码和 Token，迁移到 `dushengcdn_server/.env`；确认没有需要保留的源码修改后，再使用强制同步流程：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh

git fetch origin main
git reset --hard origin/main

cd dushengcdn_server
cp -n .env.example .env

DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
docker compose logs -n 100 dushengcdn
```

升级后可以直接检查面板健康接口：

```bash
cd /opt/dushengcdn/dushengcdn_server
panel_port="$(grep -E '^DUSHENGCDN_HTTP_PORT=' .env | tail -n1 | cut -d= -f2-)"
curl -I "http://127.0.0.1:${panel_port:-3010}/api/status"
```

如需从备份恢复，先停止 Server 容器但保持 PostgreSQL 服务可访问，再使用恢复脚本：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
cd /opt/dushengcdn
bash scripts/restore-server.sh --backup-path dushengcdn_server/backups/20260601-120000 --yes
cd dushengcdn_server
docker compose up -d
```

恢复脚本会校验 `manifest.txt` 中的 SHA-256 信息，在覆盖前为当前数据库和 `dushengcdn-data` 再生成一份 `backups/pre-restore/<timestamp>/` 安全备份，并且默认拒绝在 `dushengcdn` 服务仍运行时恢复。

后续端口、数据库密码、`SESSION_SECRET`、DSN、旧版 `AGENT_TOKEN` 等本地部署参数都改 `.env`，不要直接改仓库里的 `dushengcdn_server/docker-compose.yaml`。这样后续 `git pull --ff-only origin main` 不会因为本地 Compose 模板改动被阻塞。源码 Compose 构建时会通过 `DUSHENGCDN_VERSION` 把当前 Git 版本写入 Server 或 Agent；顶栏“版本”显示当前运行中的后端版本，节点列表显示 Agent 上报的版本。

如果升级后面板打不开，先运行只读诊断脚本：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-server.sh
```

脚本会输出 `.env` 中的面板宿主机端口、Compose 状态、`/api/status` 检查、端口监听和最近日志。源码 Compose 默认宿主机端口是 `3010`，容器内才是 `3000`；如果 `127.0.0.1:3010/api/status` 正常但域名打不开，通常需要把 Nginx、Nginx Proxy Manager、宝塔或其它反向代理上游改到 `127.0.0.1:3010`。

节点使用 Docker Compose 部署 Agent 时，更新节点端：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose -f docker-compose.agent.yaml up -d --build
docker compose -f docker-compose.agent.yaml ps
```

Server 自动升级默认关闭，生产环境推荐在顶栏检查版本后上传已审阅的 Server 二进制确认升级；如需启用自动升级，设置 `DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=true`，Release 必须同时包含当前平台 Server 二进制和同名 `.sha256` 校验文件。节点使用安装脚本部署 Agent 时，可重复执行安装命令进行重装或升级；Agent 自动更新开启后，会从当前仓库 Release 下载对应平台二进制并校验 `.sha256` 后替换本地可执行文件。DNS Worker 使用安装脚本部署时也可重复执行脚本升级，脚本会优先下载对应平台的 DNS Worker Release 资产并校验 `.sha256`。没有 Release 资产时，安装脚本会从源码构建并写入当前 Git 版本；源码构建会复用本机 Go，自动下载 Go 时会多源重试。

### 7. 发布 Release 与 latest

仓库已提供 GitHub Actions 用于生成 GitHub Release 和 GHCR 镜像，方便后续直接部署：

* 发布二进制：进入 GitHub 仓库 `Actions` -> `Release` -> `Run workflow`，填写版本号，例如 `v1.0.0` 或 `v1.0.0-beta`。工作流会构建 Server、Agent、DNS Worker 多平台二进制，并为每个资产上传同名 `.sha256` 校验文件。
* `v1.0.0` 这类纯数字版本会作为正式 Release；`v1.0.0-beta` 这类带后缀版本会作为 prerelease。GitHub 的 `releases/latest` 会指向最新正式 Release。
* 发布 Docker 镜像：进入 `Actions` -> `Docker image builds` -> `Run workflow`，填写同一个版本号。工作流会推送 `ghcr.io/satands/dushengcdn:<version>`、`ghcr.io/satands/dushengcdn:latest`、`ghcr.io/satands/dushengcdn-agent:<version>`、`ghcr.io/satands/dushengcdn-agent:latest`、`ghcr.io/satands/dushengcdn-dns-worker:<version>` 和 `ghcr.io/satands/dushengcdn-dns-worker:latest`。
* Server 自动升级、Agent 自更新和 DNS Worker 安装脚本都会优先读取 `https://github.com/SatanDS/DuShengCDN/releases/latest` 中的对应资产；自动/脚本升级要求二进制通过同名 `.sha256` 校验，没有匹配资产时 Agent 或 DNS Worker 安装脚本才回退到源码构建。
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
