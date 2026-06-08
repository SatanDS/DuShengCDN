# DuShengCDN 商业版

DuShengCDN 是面向自托管边缘节点的 CDN 管理平台。它把站点、源站、证书、节点、缓存、访问观测、配置发布、自建权威 DNS 和节点池级配置分发放在同一个管理面板里，适合需要自己掌控边缘机器、证书和调度策略的团队。

商业正式版发布页：

https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases

当前正式版：`v1.0.0`

## 功能亮点

- **统一管理面板**：集中管理网站配置、源站、TLS 证书、节点、节点池、公网 IP 池、配置版本和操作记录。
- **全局版本 + 节点池配置**：一次发布保留全局版本，同时按节点池自动分发对应配置，节点列表和详情页可直接看到当前/目标配置状态，减少大范围重载和抖动。
- **边缘 Agent 托管**：Agent 在边缘节点接入面板，负责心跳、配置同步、OpenResty 应用、reload、失败回滚、缓存清理和运行状态上报。
- **移动端自适应**：面板在手机浏览器、平板和桌面端都能顺畅使用，常用节点查看、发布和授权操作都更适合直接在浏览器里完成。
- **完整版本发布**：每次发布都会生成完整配置版本。历史版本不可变，回滚通过重新激活旧版本完成。
- **HTTPS 与证书管理**：支持上传证书，也支持托管证书流程；证书按域名绑定，适合一个站点包含多个域名的场景。
- **缓存与运行时操作**：支持按 URL、后缀、路径前缀、路径包含、路径多片段等规则缓存，并可从面板下发缓存清理和预热。
- **Cloudflare 与自建解析**：既可同步 Cloudflare A/AAAA 记录，也可部署 DNS Worker 自建权威 DNS。
- **GSLB 智能调度**：自建解析模式下可按来源 IP/ECS、来源网段、国家/地区、ASN、运营商、节点池权重、节点健康和压力指标返回边缘 IP。
- **可观测能力**：面板展示节点在线状态、应用记录、访问日志、缓存命中率、状态码分布、TOP URL/IP、节点流量和 DNS 响应端健康状态。
- **签名发布资产**：发布页中的程序文件和安装脚本都带同名 `.sha256` 与 `.sig`，安装和升级时会校验来源与完整性。
- **商业授权**：商业 Server 需要有效授权并保持在线激活租约，支持授权到期、吊销和换机流程。

## 界面预览

截图请放在 `docs/assets/readme/`。之后替换截图时也建议继续使用下列文件名，README 会自动引用新图。

| 预览 | 文件位置 |
| --- | --- |
| 管理面板总览 | `docs/assets/readme/dashboard-overview.png` |
| 网站/代理路由详情 | `docs/assets/readme/proxy-route-detail.png` |
| 节点详情与状态 | `docs/assets/readme/node-detail.png` |

![管理面板总览](docs/assets/readme/dashboard-overview.png)

![网站和代理路由详情](docs/assets/readme/proxy-route-detail.png)

![节点详情与状态](docs/assets/readme/node-detail.png)

## 发布资产说明

在 GitHub Releases 中，每个主资产都应同时存在同名 `.sha256` 和 `.sig` 文件。请始终从官方 release 页下载安装脚本和程序文件，不要混用不同版本的资产。

| 资产 | 用途 |
| --- | --- |
| `install-commercial.sh` | Linux Server 一键安装脚本 |
| `install-agent.sh` | Linux Agent 一键安装脚本 |
| `install-dns-worker.sh` | Linux DNS Worker 一键安装脚本 |
| `dushengcdn-server-linux-amd64` / `linux-arm64` | Linux Server 程序文件 |
| `dushengcdn-server-darwin-amd64` / `darwin-arm64` | macOS Server 程序文件 |
| `dushengcdn-server-windows-amd64.exe` | Windows Server 程序文件 |
| `dushengcdn-agent-linux-amd64` / `linux-arm64` | Linux Agent 程序文件 |
| `dushengcdn-agent-darwin-amd64` / `darwin-arm64` | macOS Agent 程序文件 |
| `dushengcdn-dns-worker-linux-amd64` / `linux-arm64` | Linux DNS Worker 程序文件 |
| `dushengcdn-dns-worker-darwin-amd64` / `darwin-arm64` | macOS DNS Worker 程序文件 |
| `dushengcdn-dns-worker-windows-amd64.exe` | Windows DNS Worker 程序文件 |
| `*.sha256` | 对应资产的 SHA-256 校验文件 |
| `*.sig` | 对应资产的 release 签名文件 |

Linux + systemd 环境推荐使用安装脚本。其它平台可以下载对应程序文件，并按自己的服务管理方式运行。

## 部署前准备

推荐环境：

- Linux x86_64 或 arm64，使用 systemd 管理服务。
- 一个公网可访问的面板域名，例如 `cdn.example.com`。
- 面板域名的 TLS 证书，以及 Nginx/OpenResty/宝塔等 HTTPS 反向代理。
- Server 建议从 2 核 4 GB 起步。
- Agent 边缘节点建议独占 80/443 端口。
- DNS Worker 需要公网 UDP/TCP 53 可达，并在域名注册商处完成 NS 委派。

安装前请准备：

- 商业授权 token，或授权服务提供的激活信息。
- 管理面板域名和证书。
- Agent 接入 token：在面板创建节点或开启发现注册后获得。
- DNS Worker token：在面板的本地自建解析页面创建响应端后获得。

## 安装 Server

安装最新稳定版：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

如果已经拿到商业授权 token，可在安装时一并写入并请求激活：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --http-port 3010 \
  --activation-url https://www.satandu.com \
  --license-token YOUR_LICENSE_TOKEN
```

安装指定版本：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --version v1.0.0 \
  --install-dir /opt/dushengcdn \
  --service-name dushengcdn \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

安装完成后常用路径：

| 项目 | 默认值 |
| --- | --- |
| 安装目录 | `/opt/dushengcdn` |
| 配置文件 | `/opt/dushengcdn/dushengcdn.env` |
| 数据目录 | `/opt/dushengcdn/data` |
| 日志目录 | `/opt/dushengcdn/logs` |
| systemd 服务 | `dushengcdn.service` |
| 面板端口 | `127.0.0.1:3010` 或安装时指定的端口 |

常用命令：

```bash
systemctl status dushengcdn --no-pager
journalctl -u dushengcdn -f
systemctl restart dushengcdn
```

首次登录使用 `root` 账户。安装脚本会在结束时打印面板地址和初始 root 密码；登录后请立即修改密码，并妥善保护 `/opt/dushengcdn/dushengcdn.env`。

## 配置 HTTPS 反向代理

生产环境建议让 Server 只监听本机端口，再用 HTTPS 反向代理对外服务：

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

反向代理与 Server 同机时，通常在 `/opt/dushengcdn/dushengcdn.env` 中设置：

```env
TRUSTED_PROXIES=127.0.0.1,::1
SESSION_COOKIE_SECURE=true
SESSION_COOKIE_SAME_SITE=lax
```

修改配置后重启服务：

```bash
systemctl restart dushengcdn
```

## 安装 Agent

Agent 负责接入边缘节点。它不接收远程 shell 指令，只会拉取面板发布的配置，在本机应用 OpenResty 并上报结果。

使用发现注册 token：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

使用节点专属 token：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN
```

Agent 默认安装到 `/opt/dushengcdn-agent`，服务名为 `dushengcdn-agent.service`。重跑安装脚本会替换程序文件，并保留 `agent.json`、`data`、状态、证书和观测缓冲。

查看状态：

```bash
systemctl status dushengcdn-agent --no-pager
journalctl -u dushengcdn-agent -f
```

灰度、验收或同机多实例时，请同时更换安装目录和服务名：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN \
  --install-dir /opt/dushengcdn-agent-test \
  --service-name dushengcdn-agent-test
```

确实需要清空重装时，必须显式传入 `--reinstall --wipe-data`：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN \
  --reinstall \
  --wipe-data
```

## 安装 DNS Worker

DNS Worker 用于自建权威 DNS 和 GSLB 响应。它需要公网 UDP/TCP 53 可达，并且域名 NS 已委派到对应响应端。

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen :53
```

指定公网监听地址：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen 203.0.113.10:53
```

常用选项：

```bash
--source-database-profile full
--no-source-database-download
--release-channel stable
--force-overwrite-env
```

DNS Worker 默认安装到 `/opt/dushengcdn-dns-worker`，服务名为 `dushengcdn-dns-worker.service`。重跑安装脚本会读取旧 `dns-worker.env` 作为默认值；只有显式加 `--force-overwrite-env` 时才覆盖已有配置。

验证：

```bash
systemctl status dushengcdn-dns-worker --no-pager
ss -lntu '( sport = :53 )'
dig @203.0.113.10 example.com SOA
```

## 创建第一个站点

推荐顺序：

1. 登录面板，确认商业授权状态正常。
2. 在「节点和IP池」确认至少一个 Agent 在线。
3. 添加源站，填写业务源站地址。
4. 创建网站或代理路由，绑定域名、源站和节点池。
5. 上传或申请 TLS 证书。
6. 保存配置后，进入预览或发布版本页面检查变更。
7. 发布并激活新版本，等待 Agent 应用成功。
8. 将业务域名解析到边缘节点，或切换到本地自建解析。
9. 用浏览器和 `curl -I https://your-domain` 验证访问结果。

如果使用 DNS Worker，请额外确认：

- 注册商 NS 已委派到 DNS Worker 所在服务器。
- UDP/TCP 53 都能从公网访问。
- 面板中的 DNS 快照版本与 Worker 心跳版本一致。
- 多个响应端的快照版本一致，避免不同 NS 返回不一致结果。

## 升级

升级前建议先备份数据库、配置文件、证书、上传目录、Agent 数据目录和 DNS Worker 快照。

Server 升级可使用两种方式：

- 在面板版本页上传 Server 程序文件、同名 `.sha256` 和同名 `.sig`。
- 在服务器上重跑官方安装脚本，并通过 `--version` 指定目标版本。

示例：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --version v1.0.0 \
  --install-dir /opt/dushengcdn \
  --service-name dushengcdn \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

Agent 和 DNS Worker 推荐通过面板下发升级，或重跑官方安装脚本。请不要手工替换未知来源程序文件，也不要缺少 `.sha256` 或 `.sig` 时继续升级。

升级后检查：

```bash
systemctl status dushengcdn --no-pager
systemctl status dushengcdn-agent --no-pager
systemctl status dushengcdn-dns-worker --no-pager
```

## 回滚

1. 保留上一版本的程序文件、`.sha256` 和 `.sig`。
2. 回滚前备份 Server 数据库、配置文件、证书、上传目录、Agent 数据和 DNS Worker 快照。
3. Server 可通过面板上传三件套，或重跑官方安装脚本并指定上一版本。
4. Agent 和 DNS Worker 优先重跑带签名校验的官方安装脚本。
5. 回滚后检查授权状态、节点在线状态、DNS 快照、配置版本和应用记录。

## 授权说明

商业 Server 需要有效授权才能启用商业能力。授权通常通过商业授权 token 安装，并连接授权服务完成在线激活。

需要注意：

- Server 所在机器需要能访问安装时配置的 `--activation-url`。
- 系统时间应保持准确，建议启用 NTP。
- 授权会绑定机器指纹；换机、迁移或重装后可能需要走 rehost 或重新激活流程。
- 授权到期、租约续期失败、授权被吊销或机器绑定不匹配时，商业能力可能被限制。
- 客户包只包含程序、安装脚本、校验文件和验签所需公钥，不包含授权签发材料。

## 安全与备份建议

- 始终从 `SatanDS/SatanDS-DuShengCDN-releases` 下载资产。
- 不要混用不同版本的程序文件、`.sha256` 和 `.sig`。
- 不要把授权 token、root 密码、数据库密码写进工单、聊天记录或公开截图。
- 面板建议只通过 HTTPS 暴露，Server 后端端口绑定本机或内网。
- `TRUSTED_PROXIES` 只填写受控反向代理 IP/CIDR。
- Agent 节点建议独占 80/443，不要和其它 Web 服务混用同一套 OpenResty 配置目录。
- DNS Worker 生产环境至少部署两个公网响应端，并同时放行 UDP/TCP 53。
- 定期备份 Server 数据库、`/opt/dushengcdn/dushengcdn.env`、上传目录、证书、Agent 数据目录、DNS Worker 配置和快照。
- 备份文件请加密保存，并限制访问权限。

## 常见排查

Server 无法访问：

```bash
systemctl status dushengcdn --no-pager
journalctl -u dushengcdn -n 200 --no-pager
curl -I http://127.0.0.1:3010/api/status
```

Agent 不在线：

```bash
systemctl status dushengcdn-agent --no-pager
journalctl -u dushengcdn-agent -n 200 --no-pager
curl -I https://cdn.example.com/api/status
```

DNS Worker 无响应：

```bash
systemctl status dushengcdn-dns-worker --no-pager
journalctl -u dushengcdn-dns-worker -n 200 --no-pager
ss -lntu '( sport = :53 )'
dig @YOUR_DNS_WORKER_IP example.com A +tcp
dig @YOUR_DNS_WORKER_IP example.com A
```

签名或校验失败：

- 确认下载来自官方 release 页。
- 确认程序文件、`.sha256`、`.sig` 是同一个版本和同一个资产名。
- 不要混用预览版和稳定版资产。
- 检查代理、缓存或镜像是否改写了下载内容。

授权异常：

- 确认 Server 可以访问授权服务。
- 检查系统时间是否准确。
- 确认授权未过期、未吊销。
- 如果更换过机器，确认 rehost 或重新激活流程已完成。

## 截图放置约定

README 中的界面预览图统一放在：

```text
docs/assets/readme/
```

建议文件名：

```text
dashboard-overview.png
proxy-route-detail.png
node-detail.png
```

后续替换界面截图时，直接更新上述同名 PNG 文件即可，README 中的图片链接无需修改。
