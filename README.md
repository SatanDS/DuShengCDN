<div align="center">

# OpenFlare

轻量、自托管的 OpenResty 控制面，用于管理反向代理规则、配置发布、节点同步、TLS 证书与基础可观测能力。

</div>

<p align="center">
  <a href="https://raw.githubusercontent.com/SatanDS/OpenCDN/main/LICENSE">
    <img src="https://img.shields.io/github/license/SatanDS/OpenCDN?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/SatanDS/OpenCDN/releases/latest">
    <img src="https://img.shields.io/github/v/release/SatanDS/OpenCDN?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/SatanDS/OpenCDN/pkgs/container/opencdn">
    <img src="https://img.shields.io/badge/GHCR-ghcr.io%2Fsatands%2Fopencdn-brightgreen" alt="ghcr">
  </a>
</p>

> [!WARNING]
> 使用 `root` 用户初次登录系统后，务必修改默认密码 `123456`。

## 文档

文档内容已随仓库维护，位于 `docs/` 目录。

## 核心能力

* 反向代理网站配置与多域名绑定
* 配置预览、发布、激活与历史回滚
* Agent 自动注册、心跳、同步、校验、reload 与失败回滚
* OpenResty 主配置、性能参数、缓存参数与 Lua 资源托管
* TLS 证书、域名资产、节点凭证与版本状态管理
* 请求聚合、访问分析、资源快照、健康事件与节点详情

## 快速开始

### 1. 启动 Server

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: openflare
      POSTGRES_USER: openflare
      POSTGRES_PASSWORD: replace-with-strong-password
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openflare -d openflare"]
      interval: 10s
      timeout: 5s
      retries: 5

  openflare:
    image: ghcr.io/satands/opencdn:latest
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "3000:3000"
    environment:
      SESSION_SECRET: replace-with-random-string
      DSN: postgres://openflare:replace-with-strong-password@postgres:5432/openflare?sslmode=disable
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

生产环境建议在面板服务器上用 Nginx、OpenResty 或宝塔反向代理对外提供 HTTPS，再转发到 OpenFlare 管理端端口。反代配置必须保留真实客户端 IP 头，否则节点注册和心跳经过反代时可能只能识别到内网 IP。

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

Nginx Proxy Manager 可在 `Proxy Hosts` -> 选择对应域名 -> `Edit Proxy Host` -> 齿轮图标或 `Advanced` -> `Custom Nginx Configuration` 中填入：

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

宝塔面板可在网站的“反向代理 -> 配置文件”里把 `proxy_set_header` 相关配置加入对应的 `location /` 块，保存后重载 Nginx。

### 2. 安装 Agent

本地安装脚本会自动检测 Linux / macOS 环境，缺少 OpenResty 时会尝试通过系统包管理器安装；Docker 方式则使用内置 OpenResty 的 Agent 镜像。
你可以在控制面板的节点管理->详情->节点信息->节点标识与部署复制安装命令，或直接使用下面的脚本：

#### Docker 部署

Docker 部署可直接运行 Agent 镜像：

```bash
docker run -d --name openflare-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e OPENFLARE_SERVER_URL=http://your-server:3000 \
  -e OPENFLARE_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/opencdn-agent:latest
```

#### 本地部署

使用 `discovery_token` 接入：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/OpenCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --discovery-token YOUR_DISCOVERY_TOKEN
```

使用节点专属 `agent_token`：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/OpenCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

安装脚本默认写入 `/opt/openflare-agent`，创建 `openflare-agent.service`，自动查找或安装 `openresty`，并可重复执行以重装或升级 Agent。脚本会优先下载 GitHub Release 中的 Agent 二进制；如果当前仓库还没有 Release，会自动安装 Go 并从源码构建。如需禁用依赖自动安装，可追加 `--no-install-deps`；OpenResty 使用自定义路径时可追加 `--openresty-path /path/to/openresty`。

### 3. 卸载 Agent

如需彻底卸载 Agent 并清空本地数据，可执行：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/OpenCDN/main/scripts/uninstall-agent.sh | bash
```

卸载脚本会先停止并移除 `openflare-agent.service`、删除整个 `/opt/openflare-agent` 目录，不会删除本机 OpenResty。

### 4. 发布第一份配置

1. 登录管理端并新增反代规则
2. 在发布前查看预览或变更摘要
3. 激活新版本
4. Agent 通过 WebSocket 通知或后续 heartbeat 拉取并应用配置

版本号格式固定为 `YYYYMMDD-NNN`，历史版本不可变，回滚通过重新激活旧版本完成。


## 界面预览

### 仪表盘总览

![OpenFlare dashboard overview](./docs/assets/readme/dashboard-overview.png)

### 节点详情

![OpenFlare node detail](./docs/assets/readme/node-detail.png)

### 配置新增

![OpenFlare version release](./docs/assets/readme/proxy-route-detail.png)

## 管理端与接口

管理端当前覆盖：

* 反代规则
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

<a href="https://www.star-history.com/?repos=Rain-kl%2FOpenFlare&type=date&legend=bottom-right">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=SatanDS/OpenCDN&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=SatanDS/OpenCDN&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=SatanDS/OpenCDN&type=date&legend=top-left" />
 </picture>
</a>
