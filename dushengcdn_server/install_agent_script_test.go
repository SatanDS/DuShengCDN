package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAgentNoServiceDoesNotStopSystemdService(t *testing.T) {
	script := readInstallAgentScript(t)
	requireScriptContains(t, script, `--service-name) SERVICE_NAME="$2"; shift 2 ;;`)
	requireScriptContains(t, script, `SERVICE_NAME="${DUSHENGCDN_AGENT_SERVICE_NAME:-dushengcdn-agent}"`)
	requireScriptContains(t, script, `--service-user) SERVICE_USER="$2"; shift 2 ;;`)
	requireScriptContains(t, script, `SERVICE_USER="${DUSHENGCDN_AGENT_SERVICE_USER:-dushengcdn-agent}"`)
	requireScriptContains(t, script, `validate_service_name() {`)
	requireScriptContains(t, script, `validate_service_user() {`)
	requireScriptContains(t, script, `ensure_service_user() {`)
	requireScriptContains(t, script, `die "refusing to use unsafe systemd service name: ${SERVICE_NAME}"`)
	requireScriptContains(t, script, `die "refusing to use unsafe systemd service user: ${SERVICE_USER}"`)
	requireScriptContains(t, script, `if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" && "$SYSTEMCTL_AVAILABLE" == "true" ]] && systemctl is-active --quiet "$SERVICE_NAME"; then`)
	requireScriptContains(t, script, `write_file_as_root "/etc/systemd/system/${SERVICE_NAME}.service" <<SVCEOF`)
	if strings.Contains(script, `write_file_as_root /etc/systemd/system/dushengcdn-agent.service <<SVCEOF`) {
		t.Fatal("installer must not hard-code the dushengcdn-agent systemd unit path")
	}
	if strings.Count(script, "validate_install_dir() {") != 1 {
		t.Fatal("installer must define validate_install_dir exactly once")
	}
}

func TestInstallAgentSourceBuildKeepsReleaseSignaturePublicKey(t *testing.T) {
	script := readInstallAgentScript(t)
	requireScriptContains(t, script, `effective_release_signature_public_key() {`)
	requireScriptContains(t, script, `release_public_key="$(effective_release_signature_public_key || true)"`)
	requireScriptContains(t, script, `-X=dushengcdn-agent/internal/config.ReleaseSignaturePublicKey=${release_public_key}`)
	requireScriptContains(t, script, `source-built Agent self-upgrade will be unavailable`)
}

func TestServiceNameValidationExistsInInstallerScripts(t *testing.T) {
	checks := map[string]string{
		"scripts/install-commercial.sh":     `validate_service_name() {`,
		"scripts/install-dns-worker.sh":     `validate_service_name() {`,
		"scripts/uninstall-agent.sh":        `validate_service_name() {`,
		"scripts/uninstall-agent-legacy.sh": `validate_service_name() {`,
		"scripts/uninstall-dns-worker.sh":   `validate_service_name() {`,
	}
	for relPath, needle := range checks {
		t.Run(relPath, func(t *testing.T) {
			script := readScript(t, relPath)
			requireScriptContains(t, script, needle)
		})
	}
}

func TestInstallerScriptsAvoidSecretArgvExposure(t *testing.T) {
	agentScript := readScript(t, "scripts/install-agent.sh")
	requireScriptContains(t, agentScript, `--discovery-token-file) DISCOVERY_TOKEN_FILE="$2"; shift 2 ;;`)
	requireScriptContains(t, agentScript, `DISCOVERY_TOKEN="$(tr -d '\r\n' < "$DISCOVERY_TOKEN_FILE")"`)
	requireScriptContains(t, agentScript, `--agent-token-file) AGENT_TOKEN_FILE="$2"; shift 2 ;;`)
	requireScriptContains(t, agentScript, `AGENT_TOKEN="$(tr -d '\r\n' < "$AGENT_TOKEN_FILE")"`)
	requireScriptContains(t, agentScript, `--geoip-api-token-file) GEOIP_LOOKUP_API_TOKEN_FILE="$2"; shift 2 ;;`)
	requireScriptContains(t, agentScript, `ALLOW_INSECURE_TOKEN_ARGV="false"`)
	requireScriptContains(t, agentScript, `--allow-insecure-token-argv) ALLOW_INSECURE_TOKEN_ARGV="true"; shift ;;`)
	requireScriptContains(t, agentScript, `accept_insecure_token_arg "--agent-token"`)
	requireScriptContains(t, agentScript, `pass --allow-insecure-token-argv only for legacy automation`)
	requireScriptContains(t, agentScript, `"geoip_lookup_api_token_file": "%s"`)
	if strings.Contains(agentScript, `GEOIP_LOOKUP_API_TOKEN="$(tr -d '\r\n' < "$GEOIP_LOOKUP_API_TOKEN_FILE")"`) {
		t.Fatal("agent installer must not copy GeoIP lookup token file contents into agent.json")
	}
	requireScriptContains(t, agentScript, `write_file_as_root "$CONFIG_FILE" 0600 <<CFGEOF`)
	requireScriptContains(t, agentScript, `chmod 0600 "$CONFIG_FILE"`)
	requireScriptContains(t, agentScript, `run_as_root useradd --system --home-dir "$INSTALL_DIR" --shell "$nologin_shell" --user-group "$SERVICE_USER"`)
	requireScriptContains(t, agentScript, `harden_agent_install_permissions() {`)
	requireScriptContains(t, agentScript, `run_as_root chown root:root "$INSTALL_DIR" "${INSTALL_DIR}/dushengcdn-agent"`)
	requireScriptContains(t, agentScript, `run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "${INSTALL_DIR}/data"`)
	requireScriptContains(t, agentScript, `run_as_root chown "${SERVICE_USER}:${SERVICE_USER}" "$CONFIG_FILE"`)
	requireScriptContains(t, agentScript, `User=${SERVICE_USER}`)
	requireScriptContains(t, agentScript, `Group=${SERVICE_USER}`)
	requireScriptContains(t, agentScript, `AmbientCapabilities=CAP_NET_BIND_SERVICE`)
	requireScriptContains(t, agentScript, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`)
	requireScriptContains(t, agentScript, `NoNewPrivileges=true`)
	requireScriptContains(t, agentScript, `ProtectSystem=strict`)
	requireScriptContains(t, agentScript, `ReadWritePaths=${INSTALL_DIR}/data ${CONFIG_FILE}`)
	if strings.Contains(agentScript, `run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR"`) {
		t.Fatal("agent installer must not make the binary and install scripts service-user writable")
	}

	dnsWorkerScript := readScript(t, "scripts/install-dns-worker.sh")
	requireScriptContains(t, dnsWorkerScript, `--worker-id) WORKER_ID="$2"; shift 2 ;;`)
	requireScriptContains(t, dnsWorkerScript, `--token-file) TOKEN_FILE="$2"; shift 2 ;;`)
	requireScriptContains(t, dnsWorkerScript, `--service-user) SERVICE_USER="$2"; shift 2 ;;`)
	requireScriptContains(t, dnsWorkerScript, `SERVICE_USER="${DUSHENGCDN_DNS_WORKER_SERVICE_USER:-dushengcdn-dns-worker}"`)
	requireScriptContains(t, dnsWorkerScript, `validate_service_user() {`)
	requireScriptContains(t, dnsWorkerScript, `ensure_service_user() {`)
	requireScriptContains(t, dnsWorkerScript, `run_as_root useradd --system --home-dir "$INSTALL_DIR" --shell "$nologin_shell" --user-group "$SERVICE_USER"`)
	requireScriptContains(t, dnsWorkerScript, `load_token_file`)
	requireScriptContains(t, dnsWorkerScript, `read_token_file_value()`)
	requireScriptContains(t, dnsWorkerScript, `persist_dns_worker_token_file()`)
	requireScriptContains(t, dnsWorkerScript, `TOKEN_FILE_DIR="${INSTALL_DIR}/secrets"`)
	requireScriptContains(t, dnsWorkerScript, `RUNTIME_TOKEN_FILE="${TOKEN_FILE_DIR}/dns-worker-token"`)
	requireScriptContains(t, dnsWorkerScript, `persist_dns_worker_token_file "$TOKEN_FILE_DIR" "$RUNTIME_TOKEN_FILE"`)
	requireScriptContains(t, dnsWorkerScript, `DUSHENGCDN_DNS_WORKER_TOKEN_FILE=$(env_quote "$PERSISTED_TOKEN_FILE")`)
	requireScriptContains(t, dnsWorkerScript, `DUSHENGCDN_DNS_WORKER_ID=$(env_quote "$WORKER_ID")`)
	if strings.Contains(dnsWorkerScript, `DUSHENGCDN_DNS_WORKER_TOKEN=$(env_quote "$TOKEN")`) {
		t.Fatal("dns worker installer must store the runtime token in a token file, not in systemd EnvironmentFile")
	}
	requireScriptContains(t, dnsWorkerScript, `curl_with_dns_worker_token()`)
	requireScriptContains(t, dnsWorkerScript, `curl -q --config "$config_file" "$@"`)
	requireScriptContains(t, dnsWorkerScript, `--token-file "$token_file"`)
	requireScriptContains(t, dnsWorkerScript, `ALLOW_INSECURE_TOKEN_ARGV="false"`)
	requireScriptContains(t, dnsWorkerScript, `--allow-insecure-token-argv) ALLOW_INSECURE_TOKEN_ARGV="true"; shift ;;`)
	requireScriptContains(t, dnsWorkerScript, `accept_insecure_token_arg "$1"; TOKEN="$2"; shift 2 ;;`)
	requireScriptContains(t, dnsWorkerScript, `use --token-file or pass --allow-insecure-token-argv only for legacy automation`)
	requireScriptContains(t, dnsWorkerScript, `ensure_trusted_existing_env_file "$env_file"`)
	requireScriptContains(t, dnsWorkerScript, `die "refusing to source writable existing env file: ${env_file}; rerun with --force-overwrite-env to replace it"`)
	requireScriptContains(t, dnsWorkerScript, `ENV_MODE="0640"`)
	requireScriptContains(t, dnsWorkerScript, `chown_file_as_root "$ENV_FILE" root "$SERVICE_USER"`)
	requireScriptContains(t, dnsWorkerScript, `chown_file_as_root "$token_file_path" root "$SERVICE_USER"`)
	requireScriptContains(t, dnsWorkerScript, `run_as_root chmod 0750 "$token_file_dir"`)
	requireScriptContains(t, dnsWorkerScript, `chown_dns_worker_writable_paths`)
	requireScriptContains(t, dnsWorkerScript, `User=${SERVICE_USER}`)
	requireScriptContains(t, dnsWorkerScript, `Group=${SERVICE_USER}`)
	requireScriptContains(t, dnsWorkerScript, `AmbientCapabilities=CAP_NET_BIND_SERVICE`)
	requireScriptContains(t, dnsWorkerScript, `ProtectSystem=strict`)
	requireScriptContains(t, dnsWorkerScript, `DNS_WORKER_READ_WRITE_PATHS="$(dns_worker_writable_paths)"`)
	requireScriptContains(t, dnsWorkerScript, `ReadWritePaths=${DNS_WORKER_READ_WRITE_PATHS}`)
	requireScriptContains(t, dnsWorkerScript, `run_as_root chown root:root "$INSTALL_DIR"`)
	requireScriptContains(t, dnsWorkerScript, `run_as_root chmod 0755 "$INSTALL_DIR"`)
	if strings.Contains(dnsWorkerScript, `run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR"`) {
		t.Fatal("dns worker installer must not make scripts and env service-user writable")
	}
	requireScriptContains(t, dnsWorkerScript, `UPDATE_ENABLED_VALUE="false"`)
	requireScriptContains(t, dnsWorkerScript, `DNS Worker controlled self-update is enabled while the service runs as ${SERVICE_USER}`)
	if strings.Contains(dnsWorkerScript, `curl -fsSL -H "X-DNS-Worker-Token: ${TOKEN}"`) {
		t.Fatal("dns worker installer must not pass DNS Worker token in curl argv")
	}
	if strings.Contains(dnsWorkerScript, `--token "$TOKEN"`) {
		t.Fatal("dns worker updater must not pass DNS Worker token in installer argv")
	}
	requireScriptContains(t, dnsWorkerScript, `TOKEN_FILE="${DUSHENGCDN_DNS_WORKER_TOKEN_FILE:-}"`)
	requireScriptContains(t, dnsWorkerScript, `TOKEN="$(head -n 1 "$TOKEN_FILE" | tr -d '\r\n')"`)
	requireScriptContains(t, dnsWorkerScript, `--token-file "$token_file"`)
	requireScriptContains(t, dnsWorkerScript, `cleanup_token_file`)
	if strings.Contains(dnsWorkerScript, `rm -f "$installer" "$sha_file" "$sig_file" "$release_json" "$token_file"`) {
		t.Fatal("dns worker updater must not delete the persistent token file")
	}

	agentRunner := readScript(t, "dushengcdn_agent/internal/agent/runner.go")
	requireScriptContains(t, agentRunner, `validateDNSWorkerUpdateFileOwnership(installDir, installInfo, true)`)
	requireScriptContains(t, agentRunner, `validateDNSWorkerUpdateFileOwnership(updateScript, info, true)`)
	requireScriptContains(t, agentRunner, `validateDNSWorkerUpdateFileOwnership(envFile, envInfo, true)`)
	requireScriptContains(t, agentRunner, `validateDNSWorkerUpdateIdentity(envFile, request.WorkerID)`)
	requireScriptContains(t, agentRunner, `info.Mode().Perm()&0022 != 0`)

	serverScript := readScript(t, "scripts/install-server.sh")
	requireScriptContains(t, serverScript, `harden_env_file_permissions() {`)
	requireScriptContains(t, serverScript, `(umask 077 && cp "${SERVER_DIR}/.env.example" "$ENV_FILE")`)
	requireScriptContains(t, serverScript, `chmod 0600 "$ENV_FILE"`)
	requireScriptContains(t, serverScript, `--token-file "$token_file"`)
	requireScriptContains(t, serverScript, `trap cleanup_install_dns_worker_token_file RETURN`)
	requireScriptContains(t, serverScript, `cleanup_install_dns_worker_token_file`)
	requireScriptContains(t, serverScript, `set +e`)
	requireScriptContains(t, serverScript, `set -e`)
	requireScriptContains(t, serverScript, `write_env_key DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE "/data/initial-root-password.txt"`)
	requireScriptContains(t, serverScript, `printf 'username=root\npassword=%s\n' "$initial_root_password" > "$initial_root_password_file"`)
	if strings.Contains(serverScript, `--token "$token"`) {
		t.Fatal("server installer must not pass DNS Worker token in installer argv")
	}
	if strings.Contains(serverScript, `Initial root password from ${ENV_FILE}: ${password}`) {
		t.Fatal("server installer must not print initial root password")
	}

	commercialScript := readScript(t, "scripts/install-commercial.sh")
	requireScriptContains(t, commercialScript, `DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE=${root_password_file}`)
	requireScriptContains(t, commercialScript, `--license-token-file) LICENSE_TOKEN_FILE="$2"; shift 2 ;;`)
	requireScriptContains(t, commercialScript, `LICENSE_TOKEN="$(tr -d '\r\n' < "$LICENSE_TOKEN_FILE")"`)
	requireScriptContains(t, commercialScript, `ALLOW_INSECURE_TOKEN_ARGV="false"`)
	requireScriptContains(t, commercialScript, `--allow-insecure-token-argv) ALLOW_INSECURE_TOKEN_ARGV="true"; shift ;;`)
	requireScriptContains(t, commercialScript, `accept_insecure_token_arg "--license-token"`)
	requireScriptContains(t, commercialScript, `pass --allow-insecure-token-argv only for legacy automation`)
	requireScriptContains(t, commercialScript, `User=${SERVICE_USER}`)
	requireScriptContains(t, commercialScript, `ProtectSystem=strict`)
	requireScriptContains(t, commercialScript, `harden_server_install_permissions() {`)
	requireScriptContains(t, commercialScript, `run_as_root chown root:root "$INSTALL_DIR" "$INSTALL_DIR/dushengcdn"`)
	requireScriptContains(t, commercialScript, `run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"`)
	requireScriptContains(t, commercialScript, `run_as_root chown root:"$SERVICE_USER" "$env_file"`)
	requireScriptContains(t, commercialScript, `run_as_root chmod 0640 "$env_file"`)
	requireScriptContains(t, commercialScript, `"$root_password_file" == "$INSTALL_DIR/data/initial-root-password.txt"`)
	requireScriptContains(t, commercialScript, `ReadWritePaths=${INSTALL_DIR}/data ${INSTALL_DIR}/logs`)
	if strings.Contains(commercialScript, `run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR"`) {
		t.Fatal("commercial installer must not make the binary and environment file service-user writable")
	}
	if strings.Contains(commercialScript, `echo "Initial root password:"`) {
		t.Fatal("commercial installer must not print initial root password")
	}
	if strings.Contains(commercialScript, `run_as_root grep '^DUSHENGCDN_INITIAL_ROOT_PASSWORD=' "$env_file" | sed`) {
		t.Fatal("commercial installer must not print initial root password from env file")
	}
}

func TestInstallCommercialPostsSensitiveJSONBodiesViaStdin(t *testing.T) {
	commercialScript := readScript(t, "scripts/install-commercial.sh")
	requireScriptContains(t, commercialScript, `printf '%s' "$login_body" | curl`)
	requireScriptContains(t, commercialScript, `printf '%s' "$install_body" | curl`)
	requireScriptContains(t, commercialScript, `--data-binary @- "${base_url}/api/user/login"`)
	requireScriptContains(t, commercialScript, `--data-binary @- "${base_url}/api/license/install"`)
	for _, forbidden := range []string{
		`-d "$login_body"`,
		`-d "$install_body"`,
		`--data "$login_body"`,
		`--data "$install_body"`,
		`--data-binary "$login_body"`,
		`--data-binary "$install_body"`,
	} {
		if strings.Contains(commercialScript, forbidden) {
			t.Fatalf("commercial installer must not pass sensitive JSON body through argv: %s", forbidden)
		}
	}
}

func TestDocsAvoidLicensePrivateKeyArgvExamples(t *testing.T) {
	for _, relPath := range []string{
		"README.md",
		"docs/reference/configuration.md",
		"docs/en/reference/configuration.md",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			requireScriptContains(t, content, `-private-key-file /run/secrets/dushengcdn-license-private-key`)
			if strings.Contains(content, `-private-key "$DUSHENGCDN_LICENSE_PRIVATE_KEY"`) {
				t.Fatal("license issuer private key must not be shown as a command-line argument")
			}
		})
	}

	for _, relPath := range []string{
		"dushengcdn_server/docker-compose.yaml",
		"dushengcdn_server/.env.example",
		"examples/compose/server.source.yaml",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			requireScriptContains(t, content, `DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY_FILE`)
			if strings.Contains(content, `DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY:`) ||
				strings.Contains(content, `DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY=base64`) {
				t.Fatal("compose/env examples must not encourage inline license issuer private keys")
			}
		})
	}
}

func TestUserReadmesAvoidInlineTokenPasteExamples(t *testing.T) {
	for _, relPath := range []string{
		"USER_README.md",
		"RELEASE_README.md",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			for _, forbidden := range []string{
				"YOUR_LICENSE_TOKEN",
				"YOUR_DISCOVERY_TOKEN",
				"YOUR_AGENT_TOKEN",
				"YOUR_DNS_WORKER_TOKEN",
				"install -m 0600 /dev/stdin /run/secrets/dushengcdn-license-token <<'EOF'",
				"install -m 0600 /dev/stdin /run/secrets/dushengcdn-discovery-token <<'EOF'",
				"install -m 0600 /dev/stdin /run/secrets/dushengcdn-agent-token <<'EOF'",
				"install -m 0600 /dev/stdin /run/secrets/dushengcdn-dns-worker-token <<'EOF'",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s must not encourage pasting tokens into shell history via %q", relPath, forbidden)
				}
			}
			requireScriptContains(t, content, `stty -echo`)
			requireScriptContains(t, content, `--license-token-file /run/secrets/dushengcdn-license-token`)
			requireScriptContains(t, content, `--discovery-token-file /run/secrets/dushengcdn-discovery-token`)
			requireScriptContains(t, content, `--agent-token-file /run/secrets/dushengcdn-agent-token`)
			requireScriptContains(t, content, `--token-file /run/secrets/dushengcdn-dns-worker-token`)
		})
	}
}

func TestReleaseSigningScriptsUsePrivateKeyFiles(t *testing.T) {
	signTool := readScript(t, "scripts/sign-release-asset.go")
	requireScriptContains(t, signTool, `private-key-file`)
	requireScriptContains(t, signTool, `DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY_FILE`)
	if strings.Contains(signTool, `flag.String("private-key"`) ||
		strings.Contains(signTool, `DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY")`) {
		t.Fatal("release signing tool must not accept private keys through argv or child-process environment values")
	}

	for _, relPath := range []string{
		"scripts/build-local-release.ps1",
		"scripts/publish-signed-installer-assets.ps1",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			requireScriptContains(t, content, `DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY_FILE`)
			requireScriptContains(t, content, `-private-key-file $privateKeyTemp`)
			if strings.Contains(content, `DUSHENGCDN_RELEASE_SIGNING_PRIVATE_KEY = $ReleaseSigningPrivateKey`) {
				t.Fatal("release signing scripts must not pass the private key through child-process environment variables")
			}
		})
	}

	workflow := readScript(t, ".github/workflows/release.yml")
	requireScriptContains(t, workflow, `-private-key-file "$private_key_file"`)
	requireScriptContains(t, workflow, `unset RELEASE_SIGNING_PRIVATE_KEY`)
	if strings.Contains(workflow, `-private-key "$RELEASE_SIGNING_PRIVATE_KEY"`) {
		t.Fatal("release workflow must not pass the signing private key through argv")
	}
}

func TestInstallCommercialWritesSecretsWithoutTee(t *testing.T) {
	commercialScript := readScript(t, "scripts/install-commercial.sh")
	requireScriptContains(t, commercialScript, `install_secret_file_from_stdin() {`)
	requireScriptContains(t, commercialScript, `if ! cat > "$tmp"; then`)
	requireScriptContains(t, commercialScript, `if ! run_as_root install -m 0600 "$tmp" "$target"; then`)
	requireScriptContains(t, commercialScript, `install_secret_file_from_stdin "$root_password_file"`)
	requireScriptContains(t, commercialScript, `install_secret_file_from_stdin "$env_file"`)
	for _, forbidden := range []string{
		`tee "$root_password_file"`,
		`tee "$env_file"`,
		`tee "${root_password_file}"`,
		`tee "${env_file}"`,
	} {
		if strings.Contains(commercialScript, forbidden) {
			t.Fatalf("commercial installer must not pipe secrets through tee: %s", forbidden)
		}
	}
}

func TestComposeFilesAvoidDebugAndHostGatewayDefaults(t *testing.T) {
	for _, relPath := range []string{
		"docker-compose.yaml",
		"docker-compose.agent.yaml",
		"docker-compose.dns-worker.yaml",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			requireScriptContains(t, content, `LOG_LEVEL: ${LOG_LEVEL:-info}`)
			if strings.Contains(content, `host.docker.internal:host-gateway`) {
				t.Fatal("compose defaults must not grant containers implicit host-gateway access")
			}
			if strings.Contains(content, `LOG_LEVEL: ${LOG_LEVEL:-debug}`) {
				t.Fatal("compose defaults must not enable debug logs")
			}
		})
	}
}

func TestSwaggerDoesNotSuggestBearerForHighRiskAdminRoutes(t *testing.T) {
	for _, relPath := range []string{
		"dushengcdn_server/main.go",
		"dushengcdn_server/docs/swagger.yaml",
		"dushengcdn_server/docs/swagger.json",
		"dushengcdn_server/docs/docs.go",
	} {
		t.Run(relPath, func(t *testing.T) {
			content := readScript(t, relPath)
			requireScriptContains(t, content, `高危管理接口要求浏览器 session cookie 和 CSRF Token`)
			if strings.Contains(content, `管理端可使用 Bearer Token`) {
				t.Fatal("swagger docs must not imply high-risk admin routes accept long-lived bearer tokens")
			}
		})
	}

	routerMain := readScript(t, "dushengcdn_server/router/main.go")
	requireScriptContains(t, routerMain, `swaggerRoute.Use(middleware.AdminAuth(), middleware.NoTokenAuth())`)
	requireScriptContains(t, routerMain, `ginSwagger.PersistAuthorization(false)`)
	if strings.Contains(routerMain, `ginSwagger.PersistAuthorization(true)`) {
		t.Fatal("swagger UI must not persist authorization tokens in browser storage")
	}
}

func TestDiagnosticScriptsRedactSecretsAndAcceptTokenFiles(t *testing.T) {
	serverDiagnoseScript := readScript(t, "scripts/diagnose-server.sh")
	requireScriptContains(t, serverDiagnoseScript, `redact_logs() {`)
	requireScriptContains(t, serverDiagnoseScript, `postgres(ql)?://`)
	requireScriptContains(t, serverDiagnoseScript, `token|secret|authorization`)

	dnsDiagnoseScript := readScript(t, "scripts/diagnose-dns-worker.sh")
	requireScriptContains(t, dnsDiagnoseScript, `redact_logs() {`)
	requireScriptContains(t, dnsDiagnoseScript, `env_present DUSHENGCDN_DNS_WORKER_TOKEN || env_present DUSHENGCDN_DNS_WORKER_TOKEN_FILE`)

	verifyScript := readScript(t, "scripts/verify-authoritative-dns.sh")
	requireScriptContains(t, verifyScript, `redact_logs() {`)
	requireScriptContains(t, verifyScript, `env_file_present "$worker_env" DUSHENGCDN_DNS_WORKER_TOKEN || env_file_present "$worker_env" DUSHENGCDN_DNS_WORKER_TOKEN_FILE`)
	requireScriptContains(t, verifyScript, `journalctl -u "$DNS_WORKER_SERVICE" -n "$LOG_TAIL" --no-pager 2>/dev/null | redact_logs`)
	if strings.Contains(verifyScript, `journalctl -u "$DNS_WORKER_SERVICE" -n "$LOG_TAIL" --no-pager 2>/dev/null || true`) {
		t.Fatal("authoritative DNS verifier must redact DNS Worker journal output")
	}
}

func TestInstallAgentFallsBackToVerifiedOpenRestySourceOnDebianNext(t *testing.T) {
	agentScript := readScript(t, "scripts/install-agent.sh")
	requireScriptContains(t, agentScript, `is_debian_next_apt_release() {`)
	requireScriptContains(t, agentScript, `install_openresty_from_source() {`)
	requireScriptContains(t, agentScript, `verify_openresty_source_signature "$archive" "$signature"`)
	requireScriptContains(t, agentScript, `DUSHENGCDN_OPENRESTY_VERSION:-1.31.1.1`)
	requireScriptContains(t, agentScript, `DUSHENGCDN_OPENRESTY_PGP_FINGERPRINT:-25451EB088460026195BD62CB550E09EA0E98066`)
	requireScriptContains(t, agentScript, `primary_fingerprint="$(printf '%s\n' "$verify_output" | awk '/^\[GNUPG:\] VALIDSIG / { print $NF; exit }')"`)
	requireScriptContains(t, agentScript, `"$fingerprint" != "$expected_fingerprint" && "$primary_fingerprint" != "$expected_fingerprint"`)
	requireScriptContains(t, agentScript, `--with-http_stub_status_module`)
	requireScriptContains(t, agentScript, `OpenResty apt repository signature is not accepted by this Debian release; falling back to verified source build.`)
	if strings.Contains(agentScript, `source_line="deb [trusted=yes`) {
		t.Fatal("OpenResty apt fallback must not bypass apt signature verification in the apt source line")
	}
}

func readInstallAgentScript(t *testing.T) string {
	t.Helper()
	return readScript(t, filepath.Join("scripts", "install-agent.sh"))
}

func readScript(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Clean(filepath.Join("..", relPath)))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(content)
}

func requireScriptContains(t *testing.T, script string, needle string) {
	t.Helper()
	if !strings.Contains(script, needle) {
		t.Fatalf("install-agent.sh is missing expected snippet: %s", needle)
	}
}
