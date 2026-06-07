# DuShengCDN 用户 README

本文面向部署商业签名二进制版的客户。源码开发、构建、审查和发布流程请看 [开发者 README](README.md)。

商业正式版发布仓库：

https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases

当前正式版为 `v1.0.0`。`/releases/latest` 指向该版本，所有 Server、Agent、DNS Worker 二进制和安装脚本都带同名 `.sha256` 与 `.sig`。

## 安全边界

客户拥有本机 root/Administrator 权限时，任何本地二进制都无法绝对防止反编译、补丁或运行时绕过。DuShengCDN 商业版的保护目标是：

- 强在线授权、短租约续期和吊销审计。
- release 签名、SHA-256 校验和安装器内置公钥。
- `garble -literals` 混淆与商业模式构建标记。
- 不在客户包中包含 license issuer 私钥。

请不要从未知来源下载二进制，也不要手工替换缺少 `.sha256` 或 `.sig` 的文件。

## 部署前准备

推荐环境：

- Linux x86_64 或 arm64，systemd。
- 公网可访问的域名，例如 `cdn.example.com`。
- HTTPS 反向代理，推荐同源代理 `/` 和 `/api`。
- Server 建议 2 核 4 GB 起步；边缘 Agent 节点建议独占 80/443。
- DNS Worker 需要公网 UDP/TCP 53，且域名 NS 已委派到对应响应端。

必须准备：

- 商业授权 token 或授权服务提供的激活信息。
- 管理面板域名和 TLS 证书。
- 节点注册 token：在面板创建节点或开启发现注册后获得。
- DNS Worker token：在面板的自建解析页面创建响应端后获得。

## 安装 Server

安装最新稳定版：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

安装固定版本：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-commercial.sh | bash -s -- \
  --version v1.0.0 \
  --install-dir /opt/dushengcdn \
  --service-name dushengcdn \
  --http-port 3010 \
  --activation-url https://www.satandu.com
```

常用路径：

- 程序目录：`/opt/dushengcdn`
- 配置文件：`/opt/dushengcdn/dushengcdn.env`
- systemd 服务：`dushengcdn.service`
- 默认监听：`127.0.0.1:3010` 或安装器配置的端口

常用命令：

```bash
systemctl status dushengcdn
journalctl -u dushengcdn -f
systemctl restart dushengcdn
```

首次登录使用 `root` 账户。完成首登后请立即修改密码，并确认授权状态为已激活。

## HTTPS 反向代理

生产环境推荐只让 Server 监听本机端口，再用 Nginx/OpenResty 代理 HTTPS：

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

Server 只会在 `TRUSTED_PROXIES` 命中时信任 forwarded headers。反代与 Server 同机时，通常设置：

```env
TRUSTED_PROXIES=127.0.0.1,::1
SESSION_COOKIE_SECURE=true
SESSION_COOKIE_SAME_SITE=lax
```

## 安装 Agent

在面板中创建节点，复制发现注册 token 或节点专属 token，然后在边缘节点执行：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

或使用节点专属 token：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN
```

Agent 默认安装到 `/opt/dushengcdn-agent`。重跑安装脚本只替换二进制，并保留：

- `agent.json`
- `data`
- state
- 证书
- 观测缓冲

清空重装必须显式执行：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --agent-token YOUR_AGENT_TOKEN \
  --reinstall \
  --wipe-data
```

Agent 托管目录带 `.dushengcdn-managed` marker/manifest 保护。没有标记为 DuShengCDN 专用目录时，清理未知文件会被拒绝。

## 安装 DNS Worker

DNS Worker 用于自建权威 DNS 和 GSLB 响应。先在面板中创建 DNS 响应端并复制 token，然后执行：

```bash
curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
  --server-url https://cdn.example.com \
  --token YOUR_DNS_WORKER_TOKEN \
  --listen :53
```

常用选项：

```bash
--source-database-profile full
--no-source-database-download
--release-channel stable
--force-overwrite-env
```

重跑安装脚本会读取旧 `dns-worker.env` 作为默认值。只有显式加 `--force-overwrite-env` 时才覆盖已有配置。

DNS Worker 自更新不会执行 `curl | bash`。本地 updater 会先下载安装器、`.sha256`、`.sig`，通过内置 release 公钥验签后再运行。Server 只有在 Worker 回报升级成功后才清除升级请求。

## 创建第一个站点

1. 登录面板，确认授权状态正常。
2. 添加边缘节点，等待 Agent 心跳在线。
3. 添加源站，填写业务源站地址。
4. 创建网站或代理路由，绑定域名、源站和节点池。
5. 申请或上传证书。
6. 发布配置，等待 Agent 同步成功。
7. 把业务域名 DNS 指向边缘节点或自建权威 DNS。
8. 用浏览器和 `curl -I https://your-domain` 验证响应。

如果启用 DNS Worker，请确认：

- 注册商 NS 已委派到 DNS Worker 所在服务器。
- UDP/TCP 53 都能从公网访问。
- 面板中的 DNS 快照版本与 Worker 心跳版本一致。

## 升级

面板版本页会读取 release 仓库。手动 Server 升级必须同时上传：

- Server 二进制
- 同名 `.sha256`
- 同名 `.sig`

缺少 `.sha256` 或 `.sig` 会失败。默认不允许紧急绕过签名校验。

Agent 和 DNS Worker 推荐通过面板下发升级，或重跑官方安装脚本。不要手工替换未知来源二进制。

## 回滚

1. 保留上一版本二进制、`.sha256` 和 `.sig`。
2. 回滚前备份数据库、配置文件、证书、Agent/DNS Worker 数据目录。
3. Server 回滚使用面板上传三件套或官方安装器指定版本。
4. Agent/DNS Worker 回滚优先重跑带签名校验的安装器。
5. 回滚后检查授权状态、节点在线状态、DNS 快照和配置发布版本。

## 常见排查

Server 无法登录：

```bash
systemctl status dushengcdn
journalctl -u dushengcdn -n 200 --no-pager
```

Agent 不在线：

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -n 200 --no-pager
```

DNS Worker 无响应：

```bash
systemctl status dushengcdn-dns-worker
journalctl -u dushengcdn-dns-worker -n 200 --no-pager
ss -lntu '( sport = :53 )'
dig @YOUR_DNS_WORKER_IP example.com A +tcp
dig @YOUR_DNS_WORKER_IP example.com A
```

签名校验失败：

- 确认下载来自 `SatanDS/SatanDS-DuShengCDN-releases`。
- 确认二进制、`.sha256`、`.sig` 是同一个版本和同一个资产名。
- 不要混用预览版和稳定版资产。
- 检查代理或镜像是否篡改下载内容。

授权异常：

- 确认 Server 可以访问授权服务。
- 检查系统时间是否正确。
- 确认许可证未过期、未吊销，机器 rehost 流程已完成。
- 不要在客户环境中保存或传输 issuer 私钥。

## 备份建议

定期备份：

- Server 数据库。
- `/opt/dushengcdn/dushengcdn.env`。
- 上传目录、证书和 release 缓存。
- Agent 的 `/opt/dushengcdn-agent/agent.json` 与 `data`。
- DNS Worker 的 `/opt/dushengcdn-dns-worker/dns-worker.env`、snapshot 和 source database 缓存。

备份文件请加密保存，并限制访问权限。
