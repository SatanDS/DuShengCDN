# 接入 Agent

你会学到：Agent 的职责、两种接入 Token 的区别、安装脚本参数、`agent.json` 配置方式，以及如何确认节点已经上线。

DuShengCDN Agent 运行在代理节点侧。它不会接收远程 shell 指令，而是通过 Agent API 拉取控制面发布的配置版本，在本地写入 OpenResty 文件、执行配置校验、reload，并在失败时尝试回滚到可运行配置。

## 接入方式

| 方式 | 适用场景 |
| --- | --- |
| `discovery_token` | 首次自动注册节点，由 Server 置换为节点专属凭证 |
| `agent_token` | 已在管理端创建或分配节点，直接使用节点专属凭证接入 |

`agent_token` 与 `discovery_token` 至少填写一个。

获取路径：

| 凭证 | 管理端位置 | 说明 |
| --- | --- | --- |
| `discovery_token` | 左侧「设置」->「运维设置」->「Discovery Token 与部署命令」 | 适合批量接入新节点，必要时可重新生成 |
| `agent_token` | 左侧「节点/IP池」-> 新增或选择节点 ->「详情」->「节点信息」->「节点标识与部署」 | 适合单节点预创建和定向接入 |

## 一键安装

使用 `discovery_token`：

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

安装脚本会下载最新 Agent，默认写入 `/opt/dushengcdn-agent`，生成 `agent.json`，并在 Linux + systemd 环境创建 `dushengcdn-agent.service`。脚本优先下载 GitHub Release 中的二进制；没有匹配资产时会从源码构建，并写入当前 Git 版本，避免节点版本显示为 `dev`。

支持参数：

| 参数 | 说明 |
| --- | --- |
| `--server-url` | Server 地址，必填 |
| `--discovery-token` | 首次自动注册 Token |
| `--agent-token` | 节点专属 Token |
| `--install-dir` | 安装目录，默认 `/opt/dushengcdn-agent` |
| `--openresty-path` | OpenResty 二进制路径，未传时自动查找 `openresty` |
| `--repo` | 下载 Agent 的 GitHub 仓库，默认 `SatanDS/DuShengCDN` |
| `--source-ref` | Release 不可用、回退源码构建时使用的分支、标签或提交，默认 `main` |
| `--geoip-api-url` | 可选的在线 IP 精确查询 API，只有本地 GeoIP 未识别国家码时使用 |
| `--geoip-api-token` | 可选的在线 IP 精确查询 API Bearer Token |
| `--install-deps` | 自动安装缺少的运行依赖，默认启用 |
| `--no-install-deps` | 不自动安装依赖 |
| `--no-service` | 不创建 systemd 服务 |

重复执行安装脚本会先删除整个安装目录，包括旧 `agent.json`、本地状态、缓存数据和下载的二进制。重装前请确认 Token 仍然可用。

## 配置文件

默认配置文件路径：

```text
/opt/dushengcdn-agent/agent.json
```

本地配置示例：

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "./data",
  "openresty_path": "openresty",
  "openresty_observability_port": 18081,
  "observability_replay_minutes": 15,
  "geoip_database_url": "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb",
  "geoip_database_path": "./data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "openresty_geoip_database_path": "./data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "geoip_update_interval": 86400000,
  "geoip_lookup_api_url": "",
  "geoip_lookup_api_token": "",
  "geoip_lookup_api_timeout": 250,
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

自定义 OpenResty 路径示例：

```json
{
  "server_url": "http://127.0.0.1:3000",
  "agent_token": "replace-with-node-auth-token",
  "data_dir": "/var/lib/dushengcdn-agent",
  "openresty_path": "/usr/local/openresty/nginx/sbin/openresty",
  "main_config_path": "/var/lib/dushengcdn-agent/etc/nginx/nginx.conf",
  "route_config_path": "/var/lib/dushengcdn-agent/etc/nginx/conf.d/dushengcdn_routes.conf",
  "access_log_path": "/var/lib/dushengcdn-agent/var/log/dushengcdn/access.log",
  "cert_dir": "/var/lib/dushengcdn-agent/etc/nginx/certs",
  "lua_dir": "/var/lib/dushengcdn-agent/etc/nginx/lua",
  "runtime_config_dir": "/var/lib/dushengcdn-agent/etc/dushengcdn",
  "heartbeat_interval": 10000,
  "request_timeout": 10000
}
```

如果不配置 `openresty_path`，Agent 默认调用 `openresty`。完整字段见 [配置项参考](../reference/configuration.md#agent-配置字段)。

## Docker 运行

Docker 部署时直接运行内置 OpenResty 的 Agent 镜像：

```bash
docker run -d --name dushengcdn-agent --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -e DUSHENGCDN_SERVER_URL=http://your-server:3000 \
  -e DUSHENGCDN_AGENT_TOKEN=YOUR_AGENT_TOKEN \
  ghcr.io/satands/dushengcdn-agent:latest
```

## 启动与验证

systemd 环境：

```bash
systemctl status dushengcdn-agent
journalctl -u dushengcdn-agent -f
```

手动启动：

```bash
/opt/dushengcdn-agent/dushengcdn-agent -config /opt/dushengcdn-agent/agent.json
```

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

在管理端确认：

| 位置 | 期望结果 |
| --- | --- |
| 节点列表 | 节点在线 |
| 节点详情 | 能看到心跳时间、当前版本和基础资源信息 |
| 应用记录 | 发布配置后出现应用结果 |

## 卸载

如需彻底卸载 Agent 并清空本地数据：

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

支持参数：

| 参数 | 说明 |
| --- | --- |
| `--install-dir` | 安装目录，默认 `/opt/dushengcdn-agent` |
| `--service-name` | systemd 服务名，默认 `dushengcdn-agent` |

卸载脚本只移除 Agent 服务、进程和安装目录，不会删除本机 OpenResty。

## 常见问题

| 现象 | 处理步骤 |
| --- | --- |
| `agent_token 和 discovery_token 不能同时为空` | 检查 `agent.json` 至少配置了一个 Token |
| 节点一直离线 | 在 Agent 节点执行 `curl -I http://your-server:3000`，确认 Server 地址可达 |
| OpenResty 没有启动 | 查看 `journalctl -u dushengcdn-agent`，确认 `openresty_path` 可执行且 80/443 端口未被占用 |
| 发布后重复失败 | Agent 会阻断同一 `version + checksum` 的重复应用；需要修正配置后重新发布，或激活旧版本回滚 |
