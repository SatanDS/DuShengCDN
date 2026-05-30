# Agent Unified OpenResty Binary Control Scheme

# Agent 统一 OpenResty 二进制控制方案

## Summary

将 Agent 运行模型统一为“写入受管配置文件，然后调用 `openresty` 二进制执行 `-t`、reload、start/restart”。Docker 部署不再由 Agent 控制另一个 OpenResty 容器，而是提供独立的 `ghcr.io/satands/dushengcdn-agent` 镜像；该镜像基于 `openresty/openresty`，内置 Agent 控制器和 OpenResty 二进制。

## Key Changes

- Agent runtime：
    - 移除生产路径中的 DockerExecutor / Docker 容器管理逻辑。
    - `openresty_path` 未配置时默认使用 `openresty`。
    - 二进制执行统一带 `-c <main_config_path>`，避免误读 OpenResty 默认配置。
    - apply 流程为：备份 -> 写入文件 -> `openresty -t -c ...` -> reload；若 reload 表明未运行，则 start。
    - restart 使用 `openresty -c ... -s quit` 后再 `openresty -c ...` 启动，保留缺失 PID 的容错。

- 配置与文件职责：
    - 保留旧字段 `openresty_container_name`、`openresty_docker_image`、`docker_binary` 的解析兼容，但标记废弃且不再参与控制逻辑。
    - 新增 `access_log_path`，默认 `data_dir/var/log/dushengcdn/access.log`，不再把访问日志放进 `conf.d`。
    - 新增 `runtime_config_dir`，默认 `data_dir/etc/dushengcdn`，`pow_config.json` 写入这里。
    - `cert_dir` 只写证书/密钥文件；`lua_dir` 只写 Lua 代码与静态资源。
    - 支持文件写入前先拆分：证书文件进入 `cert_dir`，`pow_config.json` 进入 `runtime_config_dir`。

- Docker Agent 镜像：
    - 新增 `dushengcdn_agent/Dockerfile`，运行镜像基于 `openresty/openresty:alpine`。
    - 默认 `DUSHENGCDN_OPENRESTY_PATH=openresty`、`DUSHENGCDN_DATA_DIR=/data`。
    - 暴露 `80`、`443`、`18081`。
    - 支持挂载 `/etc/dushengcdn/agent.json`，也支持环境变量配置。
    - CI 发布独立多架构镜像：`ghcr.io/satands/dushengcdn-agent:<version>` 和 `latest`。

- Agent 配置入口：
    - 保留 `-config` + `agent.json`。
    - 新增环境变量覆盖/兜底：`DUSHENGCDN_SERVER_URL`、`DUSHENGCDN_AGENT_TOKEN`、`DUSHENGCDN_DISCOVERY_TOKEN`、`DUSHENGCDN_NODE_NAME`、`DUSHENGCDN_NODE_IP`、`DUSHENGCDN_DATA_DIR`、`DUSHENGCDN_OPENRESTY_PATH`、`DUSHENGCDN_HEARTBEAT_INTERVAL`、`DUSHENGCDN_REQUEST_TIMEOUT`、`DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT`。
    - 若配置文件不存在但环境变量足够，Agent 可直接启动；若两者都存在，环境变量覆盖文件值。

- 脚本与文档：
    - `install-agent.sh` 转为本地 OpenResty 部署脚本，增加 `--openresty-path`，未传时自动查找 `openresty`。
    - `uninstall-agent.sh` 只卸载 Agent 本身，不再删除 Docker OpenResty 容器或镜像。
    - 更新架构、开发约束、部署说明、Agent 指南、配置项参考、README，以及英文镜像文档中的旧 Docker 控制说明。

## Public Interfaces

- 新增 Agent 配置字段：
    - `access_log_path`
    - `runtime_config_dir`

- 废弃但兼容读取：
    - `openresty_container_name`
    - `openresty_docker_image`
    - `docker_binary`

- 新增 Docker 镜像：
    - `ghcr.io/satands/dushengcdn-agent`

- Docker 运行方式示例目标：
    - 挂载配置文件：`-v ./agent.json:/etc/dushengcdn/agent.json`
    - 或环境变量：`-e DUSHENGCDN_SERVER_URL=... -e DUSHENGCDN_AGENT_TOKEN=...`

## Test Plan

- `dushengcdn_agent/internal/config`：
    - 默认 `openresty_path` 为 `openresty`。
    - 旧 Docker 字段可读取但不影响 executor。
    - 环境变量可在无配置文件时启动，并可覆盖配置文件。
    - 新默认路径符合职责边界。

- `dushengcdn_agent/internal/nginx`：
    - 二进制命令都包含 `-c <main_config_path>`。
    - apply 成功、reload 失败后回滚、未运行时 start fallback。
    - `pow_config.json` 不再写入 `cert_dir` 或 `lua_dir`。
    - stale `cert_dir/pow_config.json` 与 `lua_dir/pow_config.json` 会被清理。
    - access log 渲染到 `access_log_path`。
    - checksum 仍能把主配置、路由配置、证书和 PoW 配置统一纳入比较。

- 集成回归：
    - `cd dushengcdn_agent && GOCACHE=/tmp/dushengcdn-go-cache go test ./...`
    - `cd dushengcdn_server && GOCACHE=/tmp/dushengcdn-go-cache go test ./...`
    - Dockerfile 构建 smoke test：构建 Agent 镜像并用 env-only 配置启动到可执行阶段。

## Assumptions

- Docker Agent 镜像名固定为 `ghcr.io/satands/dushengcdn-agent`。
- 旧 Docker 控制字段保留兼容，但不再作为受支持行为。
- 本次不改 Server API、不改数据库模型、不引入远程命令能力。
- OpenResty 主配置模板继续由 Server 生成；Agent 只负责本地路径替换、文件落盘和二进制控制。
