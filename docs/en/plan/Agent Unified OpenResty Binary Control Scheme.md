# Agent Unified OpenResty Binary Control Scheme

## Summary

Unify the Agent running model as "write to the managed configuration file, then call the `openresty` binary to execute `-t`, reload, start/restart". Docker deployment no longer has the Agent control another OpenResty container, but instead provides an independent `ghcr.io/satands/dushengcdn-agent` image; this image is based on `openresty/openresty`, with the Agent controller and OpenResty binary built-in.

## Key Changes

- Agent runtime:
    - Remove the DockerExecutor / Docker container management logic from the production path.
    - Default to using `openresty` when `openresty_path` is not configured.
    - Uniformly execute binary calls with `-c <main_config_path>` to avoid misreading the OpenResty default configuration.
    - The apply flow is: backup -> write files -> `openresty -t -c ...` -> reload; if reload indicates it is not running, then start.
    - restart uses `openresty -c ... -s quit` followed by `openresty -c ...` to start, keeping fault tolerance for missing PIDs.

- Configurations and File Responsibilities:
    - Keep parser compatibility for old fields `openresty_container_name`, `openresty_docker_image`, and `docker_binary`, but mark them as deprecated and no longer involved in control logic.
    - Add `access_log_path`, defaulting to `data_dir/var/log/dushengcdn/access.log`, and no longer placing access logs inside `conf.d`.
    - Add `runtime_config_dir`, defaulting to `data_dir/etc/dushengcdn`, where `pow_config.json` is written.
    - `cert_dir` only writes certificate/key files; `lua_dir` only writes Lua code and static resources.
    - Support splitting files before writing: certificate files go into `cert_dir`, and `pow_config.json` goes into `runtime_config_dir`.

- Docker Agent Image:
    - Add `dushengcdn_agent/Dockerfile`, with the runtime image based on `openresty/openresty:alpine`.
    - Defaults to `DUSHENGCDN_OPENRESTY_PATH=openresty` and `DUSHENGCDN_DATA_DIR=/data`.
    - Expose `80`, `443`, and `18081`.
    - Support mounting `/etc/dushengcdn/agent.json`, and also support environment variable configurations.
    - CI publishes independent multi-architecture images: `ghcr.io/satands/dushengcdn-agent:<version>` and `latest`.

- Agent Configuration Entry:
    - Keep `-config` + `agent.json`.
    - Add environment variable overrides/fallbacks: `DUSHENGCDN_SERVER_URL`, `DUSHENGCDN_AGENT_TOKEN`, `DUSHENGCDN_DISCOVERY_TOKEN`, `DUSHENGCDN_NODE_NAME`, `DUSHENGCDN_NODE_IP`, `DUSHENGCDN_DATA_DIR`, `DUSHENGCDN_OPENRESTY_PATH`, `DUSHENGCDN_HEARTBEAT_INTERVAL`, `DUSHENGCDN_REQUEST_TIMEOUT`, `DUSHENGCDN_OPENRESTY_OBSERVABILITY_PORT`.
    - If the configuration file does not exist but environment variables are sufficient, the Agent can start directly; if both exist, environment variables override file values.

- Scripts and Documentation:
    - `install-agent.sh` becomes a local OpenResty deployment script, adding `--openresty-path` and automatically finding `openresty` when not passed.
    - `uninstall-agent.sh` only uninstalls the Agent itself, and no longer deletes the Docker OpenResty container or image.
    - Update architecture, development constraints, deployment instructions, Agent guide, configuration item reference, README, and old Docker control descriptions in English image documents.

## Public Interfaces

- Add Agent configuration fields:
    - `access_log_path`
    - `runtime_config_dir`

- Deprecated but compatibly read:
    - `openresty_container_name`
    - `openresty_docker_image`
    - `docker_binary`

- Add Docker image:
    - `ghcr.io/satands/dushengcdn-agent`

- Target Docker execution method examples:
    - Mount configuration file: `-v ./agent.json:/etc/dushengcdn/agent.json`
    - Or environment variables: `-e DUSHENGCDN_SERVER_URL=... -e DUSHENGCDN_AGENT_TOKEN=...`

## Test Plan

- `dushengcdn_agent/internal/config`:
    - Default `openresty_path` is `openresty`.
    - Old Docker fields can be read but do not affect the executor.
    - Environment variables can start the Agent without a configuration file and can override the configuration file.
    - New default paths conform to responsibility boundaries.

- `dushengcdn_agent/internal/nginx`:
    - Binary commands all include `-c <main_config_path>`.
    - Apply success, reload failure rollback, and start fallback when not running.
    - `pow_config.json` is no longer written to `cert_dir` or `lua_dir`.
    - Stale `cert_dir/pow_config.json` and `lua_dir/pow_config.json` will be cleaned up.
    - access log is rendered to `access_log_path`.
    - checksum can still uniformly include the main config, route config, certificates, and PoW config into comparisons.

- Integration Regression:
    - `cd dushengcdn_agent && GOCACHE=/tmp/dushengcdn-go-cache go test ./...`
    - `cd dushengcdn_server && GOCACHE=/tmp/dushengcdn-go-cache go test ./...`
    - Dockerfile build smoke test: build the Agent image and start it using env-only configuration to the executable stage.

## Assumptions

- Docker Agent image name is fixed as `ghcr.io/satands/dushengcdn-agent`.
- Old Docker control fields are compatibly preserved but are no longer a supported behavior.
- This phase does not modify Server APIs, does not modify database models, and does not introduce remote command capabilities.
- The OpenResty main configuration template continues to be generated by the Server; the Agent is only responsible for local path replacement, file landing, and binary control.
