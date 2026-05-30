# Commands and Scripts

You will learn: Common commands for starting, building, testing, installing, and uninstalling the DuShengCDN Server, management console frontend, Agent, Swagger, and documentation site.

## Server

Start from source:

```bash
cd dushengcdn_server
export SESSION_SECRET='replace-with-random-string'
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

Specify listening port and log directory:

```bash
go run . --port 3000 --log-dir ./logs
```

Reset the root password offline:

```bash
cd dushengcdn_server
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn-server --reset-root-password 'replace-with-new-password'
```

Test:

```bash
cd dushengcdn_server
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Frontend

Development:

```bash
cd dushengcdn_server/web
pnpm install
pnpm dev
```

Build static artifacts:

```bash
cd dushengcdn_server/web
pnpm build
```

Checks:

```bash
cd dushengcdn_server/web
pnpm lint
pnpm typecheck
pnpm test
```

## Agent

Run from source:

```bash
cd dushengcdn_agent
go run ./cmd/agent -config /path/to/agent.json
```

Compile:

```bash
cd dushengcdn_agent
go build -o dushengcdn-agent ./cmd/agent
```

Test:

```bash
cd dushengcdn_agent
GOCACHE=/tmp/dushengcdn-go-cache go test ./...
```

## Install Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/install-agent.sh | bash -s -- \
  --server-url http://your-server:3000 \
  --agent-token YOUR_AGENT_TOKEN
```

## Uninstall Agent

```bash
curl -fsSL https://raw.githubusercontent.com/SatanDS/DuShengCDN/main/scripts/uninstall-agent.sh | bash
```

## Swagger

Regenerate Swagger documentation:

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

## Docs

Local preview:

```bash
cd docs
pnpm dev
```

Build:

```bash
cd docs
pnpm build
```
