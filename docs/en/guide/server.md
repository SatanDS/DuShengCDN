# Starting the Server

You will learn: How to build the management console frontend from source, start DuShengCDN Server, select SQLite or PostgreSQL, and access Swagger.

DuShengCDN Server is a Gin + GORM monolithic control plane, responsible for the management console UI, management APIs, Agent APIs, configuration rendering, version releases, data storage, and aggregated queries.

## Prerequisites

| Project | Requirement |
| --- | --- |
| Go | `1.25+` |
| Node.js | `18+` |
| pnpm | Recommended to use the pnpm declared by the project via `corepack enable` |
| Database | SQLite file directory is writable, or an accessible PostgreSQL instance |

In production environments, it is recommended to explicitly configure `SESSION_SECRET` and prioritize PostgreSQL.

## Build the Management Console Frontend

The Go Server hosts the static artifacts in `dushengcdn_server/web/build`. Before starting from source, build the frontend first:

```bash
cd dushengcdn_server/web
corepack enable
pnpm install
pnpm build
```

Common frontend checks:

```bash
pnpm lint
pnpm typecheck
pnpm test
```

## Start with SQLite

```bash
cd dushengcdn_server
export SESSION_SECRET="$(openssl rand -hex 32)"
export SQLITE_PATH='./dushengcdn.db'
export LOG_LEVEL='info'
go run .
```

Listens on port `3000` by default. Access:

```text
http://localhost:3000
```

## Start with PostgreSQL

```bash
cd dushengcdn_server
export SESSION_SECRET="$(openssl rand -hex 32)"
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
export LOG_LEVEL='info'
go run .
```

`DSN` takes precedence over SQLite once set. When `DSN` and the legacy-named `SQL_DSN` both exist, `DSN` takes precedence.

If the target PostgreSQL database is empty and the local `SQLITE_PATH` file exists, the Server will attempt to migrate SQLite data to PostgreSQL during the startup phase and output the migration progress in the logs.

## Command Line Parameters

```bash
go run . --port 3000 --log-dir ./logs
```

| Parameter | Action | Default Value |
| --- | --- | --- |
| `--port` | Specify the Server listening port | `3000` |
| `--log-dir` | Specify the log directory | Empty (outputs to standard output) |
| `--reset-root-password` | Reset the `root` password and exit without starting HTTP service | Empty |
| `--version` | Output the version and exit | `false` |
| `--help` | Output the help information and exit | `false` |

Reset the root password offline with the same database configuration:

```bash
cd dushengcdn_server
export DSN='postgres://dushengcdn:secret@127.0.0.1:5432/dushengcdn?sslmode=disable'
./dushengcdn-server --reset-root-password 'replace-with-new-password'
```

## First Login

Default account:

| Username | Password |
| --- | --- |
| `root` | `123456` |

Please change the default password immediately after logging in for the first time.

## Swagger

Access after logging into the management console:

```text
http://localhost:3000/swagger/index.html
```

Regenerate Swagger locally:

```bash
go install github.com/swaggo/swag/cmd/swag@v1.16.4
cd dushengcdn_server
swag init -g main.go -o docs
```

The generated Swagger files are located in `dushengcdn_server/docs`.
