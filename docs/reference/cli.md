# 命令与脚本

你会学到：DuShengCDN Server、管理端前端、Agent、Swagger 和文档站的常用启动、构建、测试、安装与卸载命令。

## Server

源码启动：

```bash
cd dushengcdn_server
export SESSION_SECRET='replace-with-random-string'
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

指定监听端口与日志目录：

```bash
go run . --port 3000 --log-dir ./logs
```

测试：

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Frontend

开发：

```bash
cd dushengcdn_server/web
pnpm install
pnpm dev
```

构建静态产物：

```bash
cd dushengcdn_server/web
pnpm build
```

检查：

```bash
cd dushengcdn_server/web
pnpm lint
pnpm typecheck
pnpm test
```

## Agent

源码运行：

```bash
cd dushengcdn_agent
go run ./cmd/agent -config /path/to/agent.json
```

编译：

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
```

测试：

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## 安装 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

## 卸载 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

## Swagger

重新生成 Swagger 文档：

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

## Docs

本地预览：

```bash
cd docs
pnpm dev
```

构建：

```bash
cd docs
pnpm build
```
