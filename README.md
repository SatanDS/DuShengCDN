# DuShengCDN 开发者 README

DuShengCDN 是一个面向自托管边缘节点的 CDN 控制平面。它用 Server 管理站点、源站、证书、DNS、节点和发布版本，用 Agent 在边缘机器上托管 OpenResty 配置，用 DNS Worker 提供可选的自建权威 DNS 与 GSLB 响应能力。

本仓库是源码社区版仓库。客户可部署的商业二进制包发布在 [SatanDS/SatanDS-DuShengCDN-releases](https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases)，该仓库只发布经过混淆、签名和校验的二进制与安装脚本，不发布源码。

面向客户安装、升级和日常使用的文档请看 [用户 README](USER_README.md)。

> 安全边界说明：客户控制本机二进制时，任何软件都无法绝对防止反编译、补丁或运行时绕过。商业版的目标是强在线授权、短租约续期、吊销审计、签名供应链和混淆加固，用工程手段提高绕过成本，而不是承诺无法被逆向。

## 版本与发布策略

- 正式稳定版只使用纯三段 SemVer tag，例如 `v1.0.0`。
- 带 `private`、`beta`、`rc` 或其他后缀的版本一律视为预览版，发布时必须是 `prerelease=true` 且 `make_latest=false`。
- `main` 分支 push 不再自动发布客户包；正式包由 tag 或手动 workflow 显式发布。
- Server、Agent、DNS Worker、安装脚本都必须带同名 `.sha256` 和 `.sig`，安装和升级入口默认拒绝缺失签名的资产。

商业 release 资产矩阵：

| 资产 | 说明 |
| --- | --- |
| `dushengcdn-server-linux-amd64` / `linux-arm64` / `darwin-*` / `windows-amd64.exe` | 商业 Server 二进制 |
| `dushengcdn-agent-linux-amd64` / `linux-arm64` / `darwin-*` | Agent 二进制 |
| `dushengcdn-dns-worker-linux-amd64` / `linux-arm64` / `darwin-*` / `windows-amd64.exe` | DNS Worker 二进制 |
| `install-commercial.sh` | 商业 Server 安装脚本 |
| `install-agent.sh` | Agent 安装脚本 |
| `install-dns-worker.sh` | DNS Worker 安装脚本 |
| `*.sha256` / `*.sig` | 每个资产对应的 SHA-256 与 Ed25519 release 签名 |

## 架构

- **Server**：管理面板与 API，负责用户、授权、站点、源站、证书、DNS Zone、节点、发布版本、升级和观测数据。
- **Agent**：部署在边缘节点，注册/心跳/同步配置，托管 OpenResty、证书、Lua 资源、缓存清理和健康状态。
- **DNS Worker**：可选组件，部署在权威 DNS 响应端，拉取 Server 快照，响应 UDP/TCP 53，并支持按来源 IP/ECS、国家、ASN、运营商、权重和节点健康状态调度。
- **Release 仓库**：保存客户二进制包、安装脚本、校验和签名。
- **中央授权服务**：只部署在发行方控制的环境，保存 license issuer 私钥，签发许可证、激活租约、续租、吊销和审计。

## 源码社区版部署

源码部署适合开发、自托管验证和社区版使用。生产环境推荐 PostgreSQL，并使用同源 HTTPS 反代把 `/` 和 `/api` 代理到同一个 Server。

```bash
git clone https://github.com/SatanDS/DuShengCDN.git
cd DuShengCDN
cp -n examples/compose/server.env.example dushengcdn_server/.env
```

编辑 `dushengcdn_server/.env`：

```env
DUSHENGCDN_VERSION=v1.0.0
POSTGRES_PASSWORD=replace-with-strong-password
SESSION_SECRET=replace-with-openssl-rand-hex-32
DUSHENGCDN_INITIAL_ROOT_PASSWORD=replace-with-temporary-root-password
DSN=postgres://dushengcdn:replace-with-strong-password@postgres:5432/dushengcdn?sslmode=disable
GIN_MODE=release
TRUSTED_PROXIES=127.0.0.1
SESSION_COOKIE_SECURE=true
SESSION_COOKIE_SAME_SITE=lax
```

启动：

```bash
DUSHENGCDN_REPO_DIR="$PWD" \
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" \
docker compose --env-file dushengcdn_server/.env \
  -f examples/compose/server.source.yaml \
  up -d --build
```

首登账户为 `root`。密码优先使用 `.env` 中的 `DUSHENGCDN_INITIAL_ROOT_PASSWORD`；如果为空，查看 Server 首次空库启动日志中的一次性随机密码。登录后请立即修改 root 密码，并移除或轮换该启动变量。

## 商业签名二进制部署

商业包默认从 release 仓库下载稳定版，并校验二进制、`.sha256` 和 `.sig`。安装器内置 release 公钥占位，正式发布时由 workflow 注入真实公钥。

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

安装指定版本：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --version v1.0.0 \
  --install-dir /opt/dushengcdn \
  --service-name dushengcdn
```

安装器会创建 `/opt/dushengcdn/dushengcdn.env`、`dushengcdn.service`，并保留已有 env 与数据目录。生产环境建议在本机只监听内网端口，再用 Nginx/OpenResty/宝塔反代 HTTPS。

Nginx 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name cdn.example.com;

    ssl_certificate /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:3010;
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

Server 只在 `TRUSTED_PROXIES` 命中时信任 forwarded headers。Agent 节点公网 IP 使用 Gin 的 `ClientIP()` 解析结果，避免伪造 `X-Forwarded-For` 覆盖节点 IP。

## 授权模型

商业 Server 构建使用 `garble -literals`，并通过 ldflags 固化：

- `CommercialBuildMode=required-online`
- 必须安装有效商业许可证
- 必须在线激活并保持短租约
- `DUSHENGCDN_LICENSE_ALLOW_UNSIGNED` 在正式商业构建中无效

客户包不包含 issuer 私钥。签发、激活、续租、吊销和 rehost 流程只应部署在中央授权服务。Server 机器指纹会使用隐私哈希后的 machine-id/实例信息；换机需要走 rehost 或重新激活流程。租约过期、激活被吊销、许可证被吊销或签名无效时，高级能力和商业资源创建会被阻断。

源码开发环境仍可使用本地签发工具：

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

不要把 issuer 私钥写入客户部署、README、Compose 文件、日志或 release 资产。

## Agent 部署

在 Server 面板创建节点或复制节点专属 token 后安装：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN
```

使用 discovery token 自动注册：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

脚本默认写入 `/opt/dushengcdn-agent`。重跑安装脚本时只替换 Agent 二进制并保留 `agent.json`、`data`、state、证书和观测缓冲。清空重装必须显式使用：

```bash
bash install-agent.sh --server-url https://cdn.example.com --agent-token YOUR_AGENT_TOKEN --reinstall --wipe-data
```

`--wipe-data` 不能单独使用。Agent 托管目录清理带 `.dushengcdn-managed` marker/manifest 保护；目录未标记为 DuShengCDN 专用时，清理未知文件会被拒绝。边缘节点建议独占 80/443，避免和系统自带 Nginx/OpenResty 混跑同一配置目录。

Docker Agent 也可使用：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=https://cdn.example.com \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:v1.0.0
```

## DNS Worker 部署

DNS Worker 用于自建权威 DNS。它需要公网 UDP/TCP 53 可达，并且域名 NS 已委派到对应响应端。

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen 203.0.113.10:53
```

脚本默认写入 `/opt/dushengcdn-dns-worker`，保留旧 `dns-worker.env` 作为重跑默认值。只有传入 `--force-overwrite-env` 时才主动覆盖已有配置。脚本安装模式会写入 `DUSHENGCDN_DNS_WORKER_UPDATE_ENABLED=true`，用于允许 Server 下发受控自更新；容器模式默认 `UpdateEnabled=false`，避免容器内自更新破坏不可变镜像。

DNS Worker 自更新不再使用 `curl | bash`。本地 updater 会先下载 installer、`.sha256`、`.sig`，使用内置 release 公钥验证签名后再执行，默认 channel 为 `stable`。Server 只在 Worker 回报升级成功后清除 update request。

验证：

```bash
systemctl status dushengcdn-dns-worker --no-pager
ss -lntup | grep ':53'
ss -lnuap | grep ':53'
dig @203.0.113.10 example.com SOA
```

可选诊断脚本：

```bash
cd /opt/dushengcdn
bash scripts/diagnose-dns-worker.sh --public-ip 203.0.113.10 --zone example.com
bash scripts/verify-authoritative-dns.sh --public-ip 203.0.113.10 --zone example.com
```

## 认证与 API 安全

- OIDC 登录使用标准 discovery/JWKS 验证，校验 issuer、签名、aud/client_id、exp/iat 与 nonce。
- OIDC 授权请求会把 nonce 写入 session，回调后一次性删除。
- 认证源名称禁止使用保留名：`github`、`wechat`、`email`、`callback`、`link`。
- 前端第三方登录按钮使用后端返回的 `authorize_url`，避免前端自行拼接授权参数。
- 高危 Admin/Root 运维路由要求 session cookie，不接受长期 Bearer 用户 token。发布配置、节点重启/升级、证书、DNS、站点写操作都属于高危范围。
- 默认 JSON body 上限为 2 MiB，可用 `DUSHENGCDN_JSON_BODY_MAX_BYTES` 调整。手动 Server 二进制上传和 DNS Worker 心跳使用专用上限。
- 推荐同源 `/api` 反代，生产设置 `SESSION_COOKIE_SECURE=true`、`SESSION_COOKIE_SAME_SITE=lax` 或按需 `strict`。

## 升级与回滚

Server 自动升级默认关闭：

```env
DUSHENGCDN_SERVER_AUTO_UPGRADE_ENABLED=false
```

需要手动升级时，在面板版本弹窗同时上传：

- Server 二进制
- 同名 `.sha256`
- 同名 `.sig`

Server 会读取上传二进制的版本，并使用内置 release 公钥验证签名。缺少 `.sha256` 或 `.sig` 会失败。紧急绕过默认关闭；如未来引入 break-glass 入口，必须有显式配置与审计日志。

回滚建议：

1. 保留上一个 release 的二进制、`.sha256` 和 `.sig`。
2. 回滚前备份数据库、`/opt/dushengcdn/*.env`、Agent/DNS Worker 的 `data` 目录。
3. Server 回滚后先确认 `/api/status`、root 登录、授权状态和节点心跳。
4. Agent/DNS Worker 回滚优先使用带签名的安装脚本或 release 资产，不手工替换未知来源二进制。

## 容量与生产建议

- Server 使用 PostgreSQL；多实例或生产限流/session 场景建议配置 Redis，并按需设置 `REDIS_REQUIRED=true`。
- 对外只暴露 HTTPS 反代端口；Server 后端端口绑定 `127.0.0.1` 或内网地址。
- `TRUSTED_PROXIES` 只填写受控反代 IP/CIDR，不要填全网段。
- Agent 节点建议独占边缘机器的 80/443，证书和 OpenResty 配置目录不要混用。
- DNS Worker 至少部署两个公网响应端，并在注册商配置多个 NS；确认 UDP/TCP 53 都可达。
- 定期备份数据库、`upload`、证书、Agent state、DNS Worker snapshot 与 env 文件。
- 访问日志、DNS 热路径、观测聚合等性能优化会持续推进；v1.0.0 重点解决安全、误删、发布污染和升级绕过阻断项。

## 开发验证

Server：

```bash
cd dushengcdn_server
go test ./...
go vet ./...
```

Agent：

```bash
cd dushengcdn_agent
go test ./...
go vet ./...
```

Web：

```bash
cd dushengcdn_server/web
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
```

在 Windows 上构建 Next.js 时，如果同一仓库曾以不同大小写路径打开，例如 `D:\DuShengCDN` 与 `D:\DushengCDN`，可能触发路径大小写冲突。正式构建应在 Linux CI 或干净、大小写一致的路径下执行。

## v1.0.0 发布流程

1. 确认本地验证通过，并清理验证副作用，尤其是 `dushengcdn_server/web/next-env.d.ts` 中 Next 自动加入的 `.next/types` 引用。
2. 确认工作树只包含预期安全加固、脚本、测试、workflow 和 README 改动。
3. 给源码仓库打 tag：`v1.0.0`。
4. 导出 release 仓库旧资产清单作为本地审计记录。
5. 清空 `SatanDS/SatanDS-DuShengCDN-releases` 旧 releases 和旧 tags。
6. 触发 release workflow 发布 `v1.0.0`。
7. 发布后校验 `/releases/latest` 指向 `v1.0.0`、`prerelease=false`，并确认所有资产、`.sha256`、`.sig` 齐全。
8. 在干净 VPS 上部署 Server、Agent、DNS Worker，验证 root 首登、授权安装、在线激活/续租、创建站点/节点、发布配置、Agent 心跳/同步、DNS 快照与 UDP/TCP 53。

凭据只在执行阶段通过安全方式传入，不写入 README、提交、issue、日志或 release notes。
