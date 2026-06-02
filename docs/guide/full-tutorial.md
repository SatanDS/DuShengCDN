# 完整使用教程

你会学到：如何从 0 部署 DuShengCDN、接入 Agent、创建第一个网站配置、启用 DNS/TLS/缓存/WAF/地区限制、发布回滚，并完成日常运维。

本文按实际管理端路径编写。DuShengCDN 不直接 SSH 到节点，所有节点通过 Agent 主动拉取配置；你在管理端保存配置后，必须发布并激活版本，节点才会应用到 OpenResty。

## 1. 准备环境

推荐生产形态：

| 组件 | 建议 |
| --- | --- |
| Server | Docker Compose + PostgreSQL |
| Agent | Docker Agent 镜像，或安装脚本 + 本机 OpenResty |
| 数据库 | PostgreSQL 优先；SQLite 适合测试或小规模自用 |
| 端口 | Server 默认 `3000`；Agent 节点对外使用 `80` / `443` |
| 备份 | 升级前备份数据库和 Server 数据目录 |

如果宿主机 `3000` 已被占用，只改宿主机侧端口，例如 `3010:3000`；容器内部仍监听 `3000`。

## 2. 部署 Server

在服务器创建 `docker-compose.yml`：

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

浏览器访问 `http://服务器IP:3000`。默认账号是 `root` / `123456`，首次登录后立即修改密码。

## 3. 初始化运维设置

登录后先做这些基础设置：

1. 左侧「设置」->「系统设置」->「通用设置」，填写外部可访问的服务器地址。
2. 左侧「设置」->「运维设置」，确认 Agent 心跳间隔、离线阈值、WS 连接升级和 Agent 更新仓库。
3. 左侧「设置」->「数据库」，开启观测数据自动清理，并设置保留天数。
4. 左侧「用户」或「设置」->「个人设置」，修改 root 密码和显示名称。

如果需要 GitHub/OIDC 登录，按 [SSO 登录配置](./sso.md) 添加认证源。

## 4. 接入第一个 Agent

Agent 可以用两种 Token 接入：

| Token | 获取路径 |
| --- | --- |
| `discovery_token` | 左侧「设置」->「运维设置」->「Discovery Token 与部署命令」 |
| `agent_token` | 左侧「节点和IP池」-> 新增或选择节点 ->「详情」->「节点信息」->「节点标识与部署」 |

使用安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

Docker Agent：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:latest
```

确认节点上线：

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -f
```

管理端左侧「节点和IP池」应能看到节点在线。

## 5. 配置节点池与公网 IP

进入左侧「节点和IP池」-> 节点详情，维护：

| 字段 | 用途 |
| --- | --- |
| 节点池 | 自动解析和缓存运行时操作的目标分组 |
| 公网 IP 池 | 可写入 A/AAAA 记录的节点公网地址 |
| 权重 | 加权调度时的优先级 |
| 参与自动调度 | 关闭后自动解析跳过该节点 |
| 排空模式 | 开启后自动解析和缓存操作都会跳过该节点 |

多节点时，建议按地域或业务建立节点池，例如 `default`、`asia`、`edge-hk`。

## 6. 准备 Cloudflare 账号

如需 Cloudflare 自动解析：

1. 在 Cloudflare 创建 API Token，权限包含 `Zone Read` 和 `DNS Edit`。
2. 进入左侧「Cloudflare 账号」添加 Cloudflare 账号。
3. Token 可填写原始 Token、`Bearer ...` 或包含 `api_token` / `apiToken` / `token` 的 JSON。

Cloudflare 账号是独立资源，左侧主菜单会单独展示「Cloudflare 账号」，不归入 TLS 证书页。

## 7. 准备 TLS 证书

左侧「域名资产」右上角的「TLS 证书」模块支持：

| 操作 | 说明 |
| --- | --- |
| 导入证书 | 上传已有证书 PEM 和私钥 |
| 申请证书 | 通过 ACME DNS-01 申请证书 |
| 续期证书 | 对 ACME 证书提交续期任务 |
| 编辑证书 | 修改备注、内容或 ACME 配置 |

HTTPS 是按域名绑定证书。没有绑定证书的域名不会自动进入 `443 ssl` server 块。

## 8. 创建网站配置

进入左侧「网站配置」->「新增网站」。

最小必填：

| 字段 | 示例 |
| --- | --- |
| 网站名称 | `app` |
| 域名 | `app.example.com` |
| 源站地址 | `http://10.0.0.20:8080` |
| 节点池 | `default` |
| 启用状态 | 开启 |

源站规则：

* 单源站可以带 path 或 query。
* 多源站每行一个，必须是纯 `scheme://host[:port]`，且协议一致。
* 回源 Host 可单独覆盖。

进入站点详情后，可以在分区内继续配置：

| 分区 | 能力 |
| --- | --- |
| 域名设置 | 域名列表、证书绑定、HTTP 跳转 |
| 流量限制 | 连接数和限速 |
| 反向代理 | 源站、节点池、回源 Host、自定义请求头 |
| 自动解析域名 | Cloudflare 记录、节点自动选点、橙云模式 |
| 缓存策略 | URL / 后缀 / 路径缓存规则 |
| 计算验证防护 | 反爬虫挑战、白名单、黑名单 |
| 恶意请求防护 | 观察/拦截模式、内置规则、白名单、自定义规则 |
| 地区限制 | 按国家或地区代码放行/拦截 |
| 认证配置 | 站点基础认证 |

## 9. 发布并激活

配置保存后，进入左侧「发布版本」：

1. 查看预览或 diff。
2. 点击发布。
3. 确认新版本处于激活状态。
4. 进入左侧「应用记录」确认节点应用成功。

标准链路是：

```text
修改配置 -> 预览 / diff -> 发布 -> 激活版本 -> Agent 拉取 -> OpenResty 校验与 reload -> 上报结果
```

回滚时，在「发布版本」重新激活旧版本即可。

## 10. 验证访问

DNS 未正式切换前，可以用 Host 头测试：

```bash
curl -I -H 'Host: app.example.com' http://NODE_IP
```

正式解析后：

```bash
curl -I http://app.example.com
curl -Iv https://app.example.com
```

如果失败，优先查看：

1. 左侧「应用记录」。
2. 节点详情中的当前版本和 OpenResty 状态。
3. Agent 日志。
4. OpenResty 配置校验输出。

## 11. 观测与日常运维

左侧「总览」用于看全局状态、流量、缓存命中率、P95 和节点健康概览。

左侧「观测计量」用于查看：

* 站点/节点流量。
* 缓存命中率。
* 回源流量。
* 状态码分布。
* TOP URL / IP / 地区。
* 带宽峰值 P95。
* 节点可用率。

左侧「节点和IP池」适合排查节点离线、OpenResty 异常、版本落后和健康事件。

## 12. 缓存清理与预热

缓存策略需要发布后生效。运行时操作在网站配置详情的「缓存策略」分区执行：

| 操作 | 行为 |
| --- | --- |
| 清理全部缓存 | 下发到站点节点池内在线且健康的 Agent |
| 预热站点首页 | 节点主动请求站点首页 |

这两类操作不生成配置版本，不会替代发布流程。

## 13. 升级

源码目录 + Compose 部署：

```bash
cd /opt/dushengcdn
git fetch origin main
git pull --ff-only origin main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

本地端口、DSN、密码、`SESSION_SECRET` 和 Token 建议写入 `dushengcdn_server/.env`，不要直接改仓库内 Compose 模板。如果本地改过仓库内 Compose 文件导致拉取冲突，先记录这些参数并迁移到 `.env`。确认没有需要保留的源码修改后：

```bash
cd /opt/dushengcdn
git fetch origin main
git reset --hard origin/main
cd dushengcdn_server
cp -n .env.example .env
DUSHENGCDN_VERSION="$(git describe --tags --always --dirty)" docker compose --env-file .env up -d --build
docker compose ps
```

源码 Compose 构建时，`DUSHENGCDN_VERSION` 会写入 Server 或 Agent 二进制；管理端顶栏“版本”显示的是当前运行中的 Server 版本，节点列表显示 Agent 上报的版本。

Agent 使用安装脚本部署时，可重复执行安装命令重装或升级。脚本没有找到 Release 资产时会从源码构建，并写入当前 Git 版本，避免节点版本显示为 `dev`。注意安装脚本会删除旧安装目录，执行前确认 Token 可用。

## 14. 备份与恢复

源码部署可以直接使用仓库内的备份脚本：

```bash
cd /opt/dushengcdn
bash scripts/backup-server.sh
```

脚本默认读取 `dushengcdn_server/.env`，优先备份可访问的 Compose PostgreSQL，否则备份 SQLite 文件，并同时归档 `dushengcdn-data` 目录。输出目录为 `dushengcdn_server/backups/<timestamp>/`，其中包含 `manifest.txt`。脚本只创建备份文件，不会停止或恢复生产数据。

需要恢复时，先停止 Server，再执行恢复脚本。PostgreSQL Compose 部署只停止 `dushengcdn` 服务，保持 `postgres` 服务运行：

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

恢复脚本会校验 `manifest.txt` 中的校验信息，覆盖前把当前数据库和数据目录保存到 `dushengcdn_server/backups/pre-restore/<timestamp>/`，并默认拒绝在 Server 仍运行时恢复。

PostgreSQL Compose 手工备份：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
docker compose exec -T postgres pg_dump -U dushengcdn -d dushengcdn > backups/dushengcdn-$(date +%F-%H%M%S).sql
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

SQLite 备份：

```bash
cd /opt/dushengcdn/dushengcdn_server
mkdir -p backups
cp dushengcdn-data/dushengcdn.db backups/dushengcdn-$(date +%F-%H%M%S).db
tar -czf backups/dushengcdn-data-$(date +%F-%H%M%S).tar.gz dushengcdn-data
```

忘记 root 密码时：

```bash
cd /opt/dushengcdn/dushengcdn_server
docker compose stop dushengcdn
docker compose run --rm dushengcdn /dushengcdn --reset-root-password 'replace-with-new-password'
docker compose up -d
```

## 15. 常见检查清单

每次发布后检查：

* 「发布版本」中新版本已激活。
* 「应用记录」里目标节点成功或仅有可接受警告。
* 「节点和IP池」中节点在线且 OpenResty 健康。
* 访问域名返回预期状态码。

每次升级前检查：

* 数据库和数据目录已备份。
* 没有正在进行的配置发布或大规模节点重连。
* 本地 Compose 改动已记录。
* 升级后能登录管理端，并能查看节点详情。
