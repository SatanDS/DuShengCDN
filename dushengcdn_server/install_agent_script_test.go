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
	requireScriptContains(t, script, `validate_service_name() {`)
	requireScriptContains(t, script, `die "refusing to use unsafe systemd service name: ${SERVICE_NAME}"`)
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
func TestInstallAgentFallsBackToVerifiedOpenRestySourceOnDebianNext(t *testing.T) {
	agentScript := readScript(t, "scripts/install-agent.sh")
	requireScriptContains(t, agentScript, `is_debian_next_apt_release() {`)
	requireScriptContains(t, agentScript, `install_openresty_from_source() {`)
	requireScriptContains(t, agentScript, `verify_openresty_source_signature "$archive" "$signature"`)
	requireScriptContains(t, agentScript, `DUSHENGCDN_OPENRESTY_VERSION:-1.31.1.1`)
	requireScriptContains(t, agentScript, `DUSHENGCDN_OPENRESTY_PGP_FINGERPRINT:-25451EB088460026195BD62CB550E09EA0E98066`)
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
