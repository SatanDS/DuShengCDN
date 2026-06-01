# 后续完善清单

你会学到：当前项目接下来更适合补强哪些地方、为什么排这个优先级，以及每项完成时应该用什么证据验收。

本清单基于当前仓库状态和 `docs/reference/audit-report.md`，用于后续接手时避免在已完成能力里反复打转。每次完成或调整方向时，应同步更新本页。

## 高优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| Server Compose 本地参数隔离 | 已提供 `dushengcdn_server/.env.example`，`dushengcdn_server/docker-compose.yaml` 已改为读取 `.env` 变量；后续继续观察用户升级是否还会直接改仓库模板 | 文档说明可执行；源码 Compose 更新时不需要修改仓库内 `docker-compose.yaml` |
| 文档构建进入 CI 或本地一键验证 | 已新增 `.github/workflows/docs-ci.yml`，在 `docs/**` 或该 workflow 变化时安装依赖并执行 `pnpm build`；本地开发文档也补充了同样命令 | GitHub Actions `Docs CI` 通过；本地 `cd docs && pnpm install --frozen-lockfile && pnpm build` 通过 |
| 升级后旧 Agent 兼容窗口 | Server 已兼容旧全局 `AGENT_TOKEN` 的 HTTP 心跳、配置拉取和应用日志上报；旧 Token 只允许绑定已有 `node_id` 且不覆盖专属 Token 节点 | `go test ./...` 覆盖旧 Agent 兼容用例；排障文档说明旧 Token 迁移路径 |
| 面板与同机 DNS Worker 一体化部署 | 已新增 `scripts/install-server.sh`；全新部署首次创建 `.env` 时自动生成数据库密码、`SESSION_SECRET` 和 `DSN`，升级旧源码部署且已有 `postgres-data` 时保留旧默认数据库密码/DSN 以避免认证失败；启动面板后默认自动探测公网 IP、创建 `DNS服务响应端` Worker 并安装 DNS Worker；安装前会检查本机已有 Worker、同名 Docker 容器和 systemd unit 文件，避免覆盖；Compose 启动后会确认 `dushengcdn` 服务处于 running，并访问 `SERVER_URL/api/status` 做 HTTP 健康检查，不运行或 HTTP 检查失败时打印最近日志并提示数据库密码/DSN、连接、端口映射或反向代理上游端口方向 | `bash -n scripts/install-server.sh`、`scripts/install-server.sh --help`、全新 `.env` 生成回归、已有 `postgres-data` 时保留数据库凭据回归、服务未 running 时日志诊断回归、HTTP 健康检查失败诊断回归、go/前端/文档构建通过；文档已说明 `--skip-dns-worker` 与 `--force-dns-worker-reinstall` |
| 升级后面板访问诊断脚本 | 已新增只读 `scripts/diagnose-server.sh`，读取 `.env` 推导 `DUSHENGCDN_HTTP_PORT`，输出 Compose 状态、`SERVER_URL/api/status`、默认宿主机端口与 `3000` 健康检查、端口监听、最近 `dushengcdn` / `postgres` 日志和反向代理上游端口提示，专门用于升级后“网页打不开”的快速收集现场 | `bash -n scripts/diagnose-server.sh`、`scripts/diagnose-server.sh --help` 通过；在无本地 Docker/面板环境下运行能完成只读诊断并以失败状态提示不可达；README、升级、部署、排障、CLI 和配置参考已补充入口 |

## 中优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| 备份恢复脚本或管理端备份提示 | 已新增 `scripts/backup-server.sh` 和 `scripts/restore-server.sh`；备份支持 PostgreSQL Compose 与 SQLite，恢复会校验 manifest、要求 `--yes`、默认阻止运行中 Server 覆盖，并在恢复前生成当前数据安全备份；部署、升级、完整教程和 CLI 参考已补充用法 | `bash -n scripts/backup-server.sh`、`bash -n scripts/restore-server.sh`、`scripts/restore-server.sh --help` 通过；SQLite 备份/恢复试跑能生成数据库备份、数据目录归档、恢复前安全备份和 `manifest.txt` |
| root 密码重置端到端 CLI 回归 | 已新增 `dushengcdn_server/reset_root_cli_test.go`，通过 `go run . --reset-root-password ...` 覆盖创建 root、拒绝旧密码、以及重置被禁用/降级 root 的完整命令入口 | `cd dushengcdn_server && go test . -run TestResetRootPasswordCLI -count=1` 和 `go test ./...` 通过 |
| 网站配置分区前端测试补强 | 已覆盖列表入口、分区展开、创建网站、域名、反向代理节点池、自动 DNS/GSLB、权威 DNS、缓存保存失败/加载态、WAF、PoW、Basic Auth 和地区限制保存回归 | `cd dushengcdn_server/web && pnpm test -- tests/unit/proxy-routes-page.test.tsx` 通过 |
| 权威 DNS 调度评分增强 | 已在 `GSLBProbeSchedulingEnabled` 开启时保留新鲜成功探测门槛，并将健康比例、过期比例和平均 RTT 转换为 `0.25` 到 `1.0` 的探测质量系数，参与 Worker 查询面和 Server 模拟/预检路径评分；默认关闭时仍保持现有调度行为 | `go test ./internal/dnsworker -run ProbeScheduling -count=1` 和 `go test ./service -run "AuthoritativeDNS.*ProbeScheduling|SimulateAuthoritativeDNSGSLBProbeScheduling|AuthoritativeDNSSnapshotProbeScheduling" -count=1` 通过；后续如扩展丢包率、区域覆盖或独立探测网络，继续先更新设计再补测试 |
| 权威 DNS 新手部署闭环实机验证 | 一体化脚本已覆盖自动创建 Worker Token 和本机安装路径，但尚未在真实 Linux 服务器上完成从空目录到 `dig @PUBLIC_IP zone SOA` 的端到端演练 | 在 Debian/Ubuntu 空机执行 `bash scripts/install-server.sh --public-ip PUBLIC_IP`，确认 `docker compose ps`、`systemctl status dushengcdn-dns-worker`、`ss -lntup/ss -lnuap :53` 和 `dig @PUBLIC_IP example.com SOA` 均符合预期 |

## 低优先级

| 项目 | 当前状态 | 验收证据 |
| --- | --- | --- |
| 英文文档同步中文新增内容 | 英文 Quick Start、Deployment、Upgrade、Troubleshooting、CLI 和 Configuration 已同步一体化面板/DNS Worker 部署、`.env` 升级保护、示例 Compose、备份恢复脚本和权威 DNS 常见排障；后续只需按新增功能继续增量维护长篇教程与深层设计页 | `cd docs && pnpm build` 通过 |
| 示例 Compose 细化 | 已新增 `examples/compose/`，包含 Server 镜像生产模板、Server 源码构建模板、override 示例、Agent 模板、DNS Worker 模板和对应 `.env.example`；部署、快速开始、CLI 和配置参考已补充入口 | `docker compose --env-file ... config` 可解析 Server/Agent/DNS Worker 示例；文档构建通过 |

## 维护规则

* 已完成项目应移动到对应审计报告或在本页标注完成证据，不继续作为开放待办。
* 新增超出当前产品边界的项目，应先更新设计文档。
* 每次修改部署、升级、配置或排障路径时，同步更新 README、指南和配置参考。
