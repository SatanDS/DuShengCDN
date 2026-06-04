package nginx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"dushengcdn-agent/internal/protocol"
)

type runCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []runCall
	runFn func(name string, args ...string) ([]byte, error)
}

type fakeExecutor struct {
	testErr   error
	reloadErr error
}

type scriptedExecutor struct {
	testErrors   []error
	testCalls    int
	reloadErrors []error
	reloadCalls  int
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runCall{name: name, args: append([]string{}, args...)})
	if r.runFn != nil {
		return r.runFn(name, args...)
	}
	return nil, nil
}

func (e *fakeExecutor) Test(ctx context.Context) error {
	return e.testErr
}

func (e *fakeExecutor) Reload(ctx context.Context) error {
	return e.reloadErr
}

func (e *fakeExecutor) EnsureRuntime(ctx context.Context, recreate bool) error {
	return nil
}

func (e *fakeExecutor) CheckHealth(ctx context.Context) error {
	return e.testErr
}

func (e *fakeExecutor) Restart(ctx context.Context) error {
	return e.reloadErr
}

func (e *scriptedExecutor) Test(ctx context.Context) error {
	index := e.testCalls
	e.testCalls++
	if index >= len(e.testErrors) {
		return nil
	}
	return e.testErrors[index]
}

func (e *scriptedExecutor) Reload(ctx context.Context) error {
	index := e.reloadCalls
	e.reloadCalls++
	if index >= len(e.reloadErrors) {
		return nil
	}
	return e.reloadErrors[index]
}

func (e *scriptedExecutor) EnsureRuntime(ctx context.Context, recreate bool) error {
	return nil
}

func (e *scriptedExecutor) CheckHealth(ctx context.Context) error {
	return nil
}

func (e *scriptedExecutor) Restart(ctx context.Context) error {
	return nil
}

func TestPathExecutorCommands(t *testing.T) {
	runner := &fakeRunner{}
	executor := &PathExecutor{
		Path:       "/usr/local/openresty/nginx/sbin/openresty",
		ConfigPath: "/data/etc/nginx/nginx.conf",
		Runner:     runner,
	}

	if err := executor.Test(context.Background()); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if err := executor.Reload(context.Background()); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	expected := []runCall{
		{name: "/usr/local/openresty/nginx/sbin/openresty", args: []string{"-t", "-c", "/data/etc/nginx/nginx.conf"}},
		{name: "/usr/local/openresty/nginx/sbin/openresty", args: []string{"-s", "reload", "-c", "/data/etc/nginx/nginx.conf"}},
	}
	if !reflect.DeepEqual(runner.calls, expected) {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestPathExecutorEnsureRuntimeNoop(t *testing.T) {
	runner := &fakeRunner{}
	executor := &PathExecutor{
		Path:       "/usr/local/openresty/nginx/sbin/openresty",
		ConfigPath: "/data/etc/nginx/nginx.conf",
		Runner:     runner,
	}
	if err := executor.EnsureRuntime(context.Background(), true); err != nil {
		t.Fatalf("EnsureRuntime failed: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected test and reload calls, got %d", len(runner.calls))
	}
}

func TestPathExecutorRestartIgnoresMissingPID(t *testing.T) {
	runner := &fakeRunner{
		runFn: func(name string, args ...string) ([]byte, error) {
			if len(args) == 2 && args[0] == "-s" && args[1] == "quit" {
				return []byte("openresty: [error] invalid PID number \"\" in \"/usr/local/openresty/nginx/logs/nginx.pid\""), errors.New("exit status 1")
			}
			return []byte(""), nil
		},
	}
	executor := &PathExecutor{
		Path:       "/usr/local/openresty/nginx/sbin/openresty",
		ConfigPath: "/data/etc/nginx/nginx.conf",
		Runner:     runner,
	}
	if err := executor.Restart(context.Background()); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 restart calls, got %d", len(runner.calls))
	}
}

func TestPathExecutorReloadStartsWhenRuntimeIsNotRunning(t *testing.T) {
	runner := &fakeRunner{
		runFn: func(name string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "-s" && args[1] == "reload" {
				return []byte("openresty: [error] invalid PID number \"\" in \"/usr/local/openresty/nginx/logs/nginx.pid\""), errors.New("exit status 1")
			}
			return []byte(""), nil
		},
	}
	executor := &PathExecutor{
		Path:       "/usr/local/openresty/nginx/sbin/openresty",
		ConfigPath: "/data/etc/nginx/nginx.conf",
		Runner:     runner,
	}
	if err := executor.Reload(context.Background()); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	expected := []runCall{
		{name: "/usr/local/openresty/nginx/sbin/openresty", args: []string{"-s", "reload", "-c", "/data/etc/nginx/nginx.conf"}},
		{name: "/usr/local/openresty/nginx/sbin/openresty", args: []string{"-c", "/data/etc/nginx/nginx.conf"}},
	}
	if !reflect.DeepEqual(runner.calls, expected) {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestDetectVersionFromBinary(t *testing.T) {
	version, err := detectVersion(context.Background(), ExecutorOptions{
		NginxPath: "/usr/local/openresty/nginx/sbin/openresty",
	}, &fakeRunner{
		runFn: func(name string, args ...string) ([]byte, error) {
			return []byte("nginx version: openresty/1.27.1.2\n"), nil
		},
	})
	if err != nil {
		t.Fatalf("detectVersion failed: %v", err)
	}
	if version != "1.27.1.2" {
		t.Fatalf("unexpected version: %s", version)
	}
}

func TestManagerApplyAndChecksumIncludeMainConfig(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "nginx.conf")
	routePath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	certDir := filepath.Join(tempDir, "certs")
	accessLogPath := filepath.Join(tempDir, "var", "log", "dushengcdn", "access.log")
	manager := &Manager{
		MainConfigPath:  mainPath,
		RouteConfigPath: routePath,
		AccessLogPath:   accessLogPath,
		CertDir:         certDir,
		NginxCertDir:    "/etc/nginx/dushengcdn-certs",
		LuaDir:          filepath.Join(tempDir, "lua"),
		NginxLuaDir:     "/etc/nginx/dushengcdn-lua",
		Executor:        &fakeExecutor{},
	}

	outcome := manager.Apply(
		context.Background(),
		"include __DUSHENGCDN_ROUTE_CONFIG__;\naccess_log __DUSHENGCDN_ACCESS_LOG__ dushengcdn_json;\n",
		"ssl_certificate __DUSHENGCDN_CERT_DIR__/1.crt;\n",
		[]protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
	)
	if outcome.Status != ApplyStatusSuccess {
		t.Fatalf("Apply failed: %#v", outcome)
	}

	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	expectedMain := "include " + routePath + ";\naccess_log " + filepath.ToSlash(accessLogPath) + " dushengcdn_json;\n"
	if string(mainData) != expectedMain {
		t.Fatalf("unexpected main config: %s", string(mainData))
	}

	routeData, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatalf("failed to read route config: %v", err)
	}
	if string(routeData) != "ssl_certificate /etc/nginx/dushengcdn-certs/1.crt;\n" {
		t.Fatalf("unexpected route config: %s", string(routeData))
	}

	value, err := manager.CurrentChecksum()
	if err != nil {
		t.Fatalf("CurrentChecksum failed: %v", err)
	}
	expected := bundleChecksum(
		"include __DUSHENGCDN_ROUTE_CONFIG__;\naccess_log __DUSHENGCDN_ACCESS_LOG__ dushengcdn_json;\n",
		"ssl_certificate __DUSHENGCDN_CERT_DIR__/1.crt;\n",
		[]protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
	)
	if value != expected {
		t.Fatalf("unexpected checksum: got %s want %s", value, expected)
	}
}

func TestManagerApplyCreatesProxyCachePath(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "var", "cache", "openresty", "dushengcdn")
	manager := &Manager{
		MainConfigPath:  filepath.Join(tempDir, "etc", "nginx.conf"),
		RouteConfigPath: filepath.Join(tempDir, "etc", "routes.conf"),
		LuaDir:          filepath.Join(tempDir, "lua"),
		Executor:        &fakeExecutor{},
	}

	outcome := manager.Apply(
		context.Background(),
		fmt.Sprintf("http {\n    proxy_cache_path %s levels=1:2 keys_zone=dushengcdn_cache:10m inactive=30m max_size=1g;\n}\n", filepath.ToSlash(cachePath)),
		"",
		nil,
	)
	if outcome.Status != ApplyStatusSuccess {
		t.Fatalf("Apply failed: %#v", outcome)
	}
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		t.Fatalf("expected proxy cache directory to be created, stat=%v err=%v", info, err)
	}
}

func TestProxyCacheDirectoriesFromConfig(t *testing.T) {
	mainConfig := strings.Join([]string{
		"http {",
		"    proxy_cache_path /var/cache/openresty/dushengcdn levels=1:2 keys_zone=dushengcdn_cache:10m;",
		"    proxy_cache_path '/srv/openresty cache/site-a' levels=1:2 keys_zone=site_a:10m;",
		"    proxy_cache_path /var/cache/openresty/dushengcdn levels=1:2 keys_zone=duplicate:10m;",
		"}",
	}, "\n")

	paths := proxyCacheDirectoriesFromConfig(mainConfig)
	expected := []string{
		"/var/cache/openresty/dushengcdn",
		"/srv/openresty cache/site-a",
	}
	if !reflect.DeepEqual(paths, expected) {
		t.Fatalf("unexpected cache paths: got %#v want %#v", paths, expected)
	}
}

func TestParseNginxVersionIgnoresDockerEntrypointPaths(t *testing.T) {
	output := strings.Join([]string{
		"/docker-entrypoint.sh: /docker-entrypoint.d/10-listen-on-ipv6-by-default.sh: info: can not modify /etc/nginx/conf.d/default.conf (read-only file system?)",
		"nginx version: openresty/1.27.1.2",
	}, "\n")

	version := parseNginxVersion(output)
	if version != "1.27.1.2" {
		t.Fatalf("unexpected version: %s", version)
	}
}

func TestManagerApplyWritesSupportFilesAndReplacesPlaceholder(t *testing.T) {
	tempDir := t.TempDir()
	manager := &Manager{
		MainConfigPath:               filepath.Join(tempDir, "nginx.conf"),
		RouteConfigPath:              filepath.Join(tempDir, "routes.conf"),
		CertDir:                      filepath.Join(tempDir, "certs"),
		NginxCertDir:                 "/etc/nginx/dushengcdn-certs",
		LuaDir:                       filepath.Join(tempDir, "lua"),
		NginxLuaDir:                  "/etc/nginx/dushengcdn-lua",
		OpenrestyObservabilityListen: "18081",
		OpenrestyResolverDirective:   "    resolver 127.0.0.11 valid=30s ipv6=off;\n    resolver_timeout 5s;\n",
		Executor:                     &fakeExecutor{},
	}

	outcome := manager.Apply(context.Background(), "include __DUSHENGCDN_ROUTE_CONFIG__;\n__DUSHENGCDN_RESOLVER_DIRECTIVE__server { listen __DUSHENGCDN_OBSERVABILITY_LISTEN__; }", "ssl_certificate __DUSHENGCDN_CERT_DIR__/1.crt;", []protocol.SupportFile{
		{Path: "1.crt", Content: "cert-data"},
		{Path: "1.key", Content: "key-data"},
	})
	if outcome.Status != ApplyStatusSuccess {
		t.Fatalf("Apply failed: %#v", outcome)
	}

	routeData, err := os.ReadFile(manager.RouteConfigPath)
	if err != nil {
		t.Fatalf("failed to read route config: %v", err)
	}
	if !strings.Contains(string(routeData), "/etc/nginx/dushengcdn-certs/1.crt") {
		t.Fatalf("expected placeholder replacement in route config, got %s", string(routeData))
	}
	renderedRoute := manager.renderRouteConfig("access_by_lua_file __DUSHENGCDN_LUA_DIR__/access.lua;\nlocation /.within.website/x/cmd/anubis/static/ { alias __DUSHENGCDN_POW_STATIC_DIR__/; }\n")
	if !strings.Contains(renderedRoute, "access_by_lua_file /etc/nginx/dushengcdn-lua/access.lua;") {
		t.Fatalf("expected lua dir placeholder replacement in route config, got %s", renderedRoute)
	}
	if !strings.Contains(renderedRoute, "alias /etc/nginx/dushengcdn-lua/pow/static/;") {
		t.Fatalf("expected pow static dir placeholder replacement in route config, got %s", renderedRoute)
	}
	mainData, err := os.ReadFile(manager.MainConfigPath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	if !strings.Contains(string(mainData), "listen 18081;") {
		t.Fatalf("expected observability listen placeholder replacement in main config, got %s", string(mainData))
	}
	if !strings.Contains(string(mainData), "resolver 127.0.0.11 valid=30s ipv6=off;") {
		t.Fatalf("expected resolver directive placeholder replacement in main config, got %s", string(mainData))
	}
	certData, err := os.ReadFile(filepath.Join(manager.CertDir, "1.crt"))
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}
	if string(certData) != "cert-data" {
		t.Fatalf("unexpected cert file content: %s", string(certData))
	}
	luaInfo, err := os.Stat(filepath.Join(manager.LuaDir, "log.lua"))
	if err != nil {
		t.Fatalf("expected managed lua file to exist, stat err = %v", err)
	}
	if runtime.GOOS != "windows" && luaInfo.Mode().Perm() != 0o644 {
		t.Fatalf("unexpected lua mode: %o", luaInfo.Mode().Perm())
	}
}

func TestManagerCheckHealthUsesStubStatusInsteadOfConfigTest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/dushengcdn/stub_status" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Active connections: 1\n"))
		}),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Shutdown(context.Background())

	mainPath := filepath.Join(t.TempDir(), "nginx.conf")
	if err := os.WriteFile(mainPath, []byte("main"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{
		MainConfigPath:             mainPath,
		OpenrestyObservabilityPort: port,
		Executor: &fakeExecutor{
			testErr: errors.New("openresty -t should not be called"),
		},
	}
	if err := manager.CheckHealth(context.Background()); err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}
}

func TestManagerCheckHealthFailsWhenStubStatusUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("listener close failed: %v", err)
	}

	mainPath := filepath.Join(t.TempDir(), "nginx.conf")
	if err := os.WriteFile(mainPath, []byte("main"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{
		MainConfigPath:             mainPath,
		OpenrestyObservabilityPort: port,
		Executor:                   &fakeExecutor{},
	}
	if err := manager.CheckHealth(context.Background()); err == nil {
		t.Fatal("expected CheckHealth to fail when stub_status is unavailable")
	}
}

func TestResolverDirectiveUsesExplicitResolvers(t *testing.T) {
	got := ResolverDirective("", []string{"10.0.0.2", "1.1.1.1"})
	if !strings.Contains(got, "resolver 10.0.0.2 1.1.1.1") {
		t.Fatalf("expected explicit resolver directive, got %q", got)
	}
}

func TestParseResolverAddressesFiltersLoopbackForDocker(t *testing.T) {
	content := strings.Join([]string{
		"nameserver 127.0.0.53",
		"nameserver 10.0.0.2",
		"nameserver ::1",
		"nameserver 1.1.1.1",
	}, "\n")
	got := parseResolverAddresses(content, true)
	expected := []string{"10.0.0.2", "1.1.1.1"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected docker resolvers: got %#v want %#v", got, expected)
	}
}

func TestParseResolverAddressesKeepsLoopbackForLocalBinary(t *testing.T) {
	content := strings.Join([]string{
		"nameserver 127.0.0.53",
		"nameserver 10.0.0.2",
	}, "\n")
	got := parseResolverAddresses(content, false)
	expected := []string{"127.0.0.53", "10.0.0.2"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected local resolvers: got %#v want %#v", got, expected)
	}
}

func TestRequiresRuntimeResolver(t *testing.T) {
	testCases := []struct {
		name      string
		originURL string
		want      bool
	}{
		{name: "hostname", originURL: "https://origin.internal", want: true},
		{name: "ipv4", originURL: "https://10.0.0.8", want: false},
		{name: "ipv6", originURL: "https://[2001:db8::1]", want: false},
		{name: "invalid", originURL: "://bad", want: false},
	}

	for _, testCase := range testCases {
		if got := RequiresRuntimeResolver(testCase.originURL); got != testCase.want {
			t.Fatalf("%s: got %v want %v", testCase.name, got, testCase.want)
		}
	}
}

func TestWriteCertFilesKeepsBaseDirAndRemovesStaleFiles(t *testing.T) {
	tempDir := t.TempDir()
	certDir := filepath.Join(tempDir, "certs")
	if err := os.MkdirAll(filepath.Join(certDir, "stale"), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "stale", "old.crt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{CertDir: certDir}

	if err := manager.writeCertFiles([]protocol.SupportFile{
		{Path: "1.crt", Content: "cert"},
		{Path: "1.key", Content: "key"},
	}); err != nil {
		t.Fatalf("writeCertFiles failed: %v", err)
	}

	if _, err := os.Stat(certDir); err != nil {
		t.Fatalf("expected cert dir to persist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(certDir, "stale", "old.crt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale cert file to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(certDir, "1.crt")); err != nil {
		t.Fatalf("expected new cert file to exist, stat err = %v", err)
	}
}

func TestEnsureLuaAssetsKeepsBaseDirAndRemovesStaleFiles(t *testing.T) {
	tempDir := t.TempDir()
	luaDir := filepath.Join(tempDir, "lua")
	if err := os.MkdirAll(filepath.Join(luaDir, "stale"), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(luaDir, "stale", "old.lua"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{LuaDir: luaDir}

	if err := manager.EnsureLuaAssets(); err != nil {
		t.Fatalf("EnsureLuaAssets failed: %v", err)
	}

	if _, err := os.Stat(luaDir); err != nil {
		t.Fatalf("expected lua dir to persist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "stale", "old.lua")); !os.IsNotExist(err) {
		t.Fatalf("expected stale lua file to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "log.lua")); err != nil {
		t.Fatalf("expected managed lua file to exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "pow", "check.lua")); err != nil {
		t.Fatalf("expected managed pow lua file to exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "waf", "check.lua")); err != nil {
		t.Fatalf("expected managed waf lua file to exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "cc", "check.lua")); err != nil {
		t.Fatalf("expected managed cc lua file to exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "geoip", "access.lua")); err != nil {
		t.Fatalf("expected managed geoip lua file to exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "pow", "static", "js", "main.mjs")); err != nil {
		t.Fatalf("expected managed pow static asset to exist, stat err = %v", err)
	}
}

func TestCertFileMode(t *testing.T) {
	testCases := []struct {
		path string
		want os.FileMode
	}{
		{path: "1.crt", want: 0o644},
		{path: "1.pem", want: 0o644},
		{path: "1.key", want: 0o600},
		{path: "misc.txt", want: 0o644},
	}

	for _, testCase := range testCases {
		if got := certFileMode(testCase.path); got != testCase.want {
			t.Fatalf("unexpected mode for %s: got %o want %o", testCase.path, got, testCase.want)
		}
	}
}

func TestManagerEnsureLuaAssetsWritesReadableFiles(t *testing.T) {
	tempDir := t.TempDir()
	manager := &Manager{
		LuaDir:           filepath.Join(tempDir, "lua"),
		NginxLuaDir:      "/etc/nginx/dushengcdn-lua",
		RuntimeConfigDir: filepath.Join(tempDir, "runtime"),
	}

	err := manager.EnsureLuaAssets()
	if err != nil {
		t.Fatalf("EnsureLuaAssets failed: %v", err)
	}

	luaInfo, err := os.Stat(filepath.Join(manager.LuaDir, "log.lua"))
	if err != nil {
		t.Fatalf("failed to stat lua file: %v", err)
	}
	if runtime.GOOS != "windows" && luaInfo.Mode().Perm() != 0o644 {
		t.Fatalf("unexpected lua mode: %o", luaInfo.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(manager.LuaDir, "pow", "check.lua")); err != nil {
		t.Fatalf("failed to stat pow lua file: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(manager.LuaDir, "pow", "check.lua"))
	if err != nil {
		t.Fatalf("failed to read pow lua file: %v", err)
	}
	if !strings.Contains(string(data), filepath.ToSlash(manager.RuntimeConfigDir)+"/pow_config.json") {
		t.Fatalf("expected pow lua to read runtime config dir, got %s", string(data))
	}
	wafData, err := os.ReadFile(filepath.Join(manager.LuaDir, "waf", "check.lua"))
	if err != nil {
		t.Fatalf("failed to read waf lua file: %v", err)
	}
	if !strings.Contains(string(wafData), filepath.ToSlash(manager.RuntimeConfigDir)+"/waf_config.json") {
		t.Fatalf("expected waf lua to read runtime config dir, got %s", string(wafData))
	}
	ccData, err := os.ReadFile(filepath.Join(manager.LuaDir, "cc", "check.lua"))
	if err != nil {
		t.Fatalf("failed to read cc lua file: %v", err)
	}
	if !strings.Contains(string(ccData), filepath.ToSlash(manager.RuntimeConfigDir)+"/cc_config.json") {
		t.Fatalf("expected cc lua to read runtime config dir, got %s", string(ccData))
	}
	sharedData, err := os.ReadFile(filepath.Join(manager.LuaDir, "shared", "ipmatcher.lua"))
	if err != nil {
		t.Fatalf("failed to read shared ip matcher lua file: %v", err)
	}
	if !strings.Contains(string(sharedData), "local function parse_ipv6") {
		t.Fatalf("expected shared ip matcher to support IPv6, got %s", string(sharedData))
	}
	geoipData, err := os.ReadFile(filepath.Join(manager.LuaDir, "geoip", "access.lua"))
	if err != nil {
		t.Fatalf("failed to read geoip lua file: %v", err)
	}
	if !strings.Contains(string(geoipData), `local db_path = ""`) {
		t.Fatalf("expected empty geoip database path placeholder to be rendered, got %s", string(geoipData))
	}
}

func TestManagerEnsureLuaAssetsRendersGeoIPRuntimeSettings(t *testing.T) {
	tempDir := t.TempDir()
	manager := &Manager{
		LuaDir:                 filepath.Join(tempDir, "lua"),
		NginxLuaDir:            "/etc/nginx/dushengcdn-lua",
		RuntimeConfigDir:       filepath.Join(tempDir, "runtime"),
		NginxGeoIPDatabasePath: "/data/GeoLite2-Country.mmdb",
		GeoIPLookupAPIURL:      "https://ipdb.example.com/lookup",
		GeoIPLookupAPIToken:    "secret",
		GeoIPLookupAPITimeout:  350 * time.Millisecond,
	}

	if err := manager.EnsureLuaAssets(); err != nil {
		t.Fatalf("EnsureLuaAssets failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(manager.LuaDir, "geoip", "access.lua"))
	if err != nil {
		t.Fatalf("failed to read geoip lua file: %v", err)
	}
	text := string(data)
	for _, expected := range []string{
		`local db_path = "/data/GeoLite2-Country.mmdb"`,
		`local api_url = "https://ipdb.example.com/lookup"`,
		`local api_token = "secret"`,
		`local api_timeout = 350`,
		`Authorization"] = "Bearer " .. api_token`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected geoip lua to contain %q, got %s", expected, text)
		}
	}
}

func TestEnsureLuaAssetsLeavesRuntimePowConfigOutsideLuaDir(t *testing.T) {
	tempDir := t.TempDir()
	luaDir := filepath.Join(tempDir, "lua")
	runtimeConfigDir := filepath.Join(tempDir, "runtime")
	if err := os.MkdirAll(runtimeConfigDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	powConfigPath := filepath.Join(runtimeConfigDir, "pow_config.json")
	ccConfigPath := filepath.Join(runtimeConfigDir, "cc_config.json")
	wantPow := `[{"domains":["pow.example.com"],"enabled":true}]`
	wantCC := `[{"domains":["cc.example.com"],"enabled":true}]`
	if err := os.WriteFile(powConfigPath, []byte(wantPow), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(ccConfigPath, []byte(wantCC), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{LuaDir: luaDir, RuntimeConfigDir: runtimeConfigDir}

	if err := manager.EnsureLuaAssets(); err != nil {
		t.Fatalf("EnsureLuaAssets failed: %v", err)
	}

	got, err := os.ReadFile(powConfigPath)
	if err != nil {
		t.Fatalf("expected pow_config.json to remain after EnsureLuaAssets: %v", err)
	}
	if string(got) != wantPow {
		t.Fatalf("unexpected pow_config.json content: got %s want %s", string(got), wantPow)
	}
	got, err = os.ReadFile(ccConfigPath)
	if err != nil {
		t.Fatalf("expected cc_config.json to remain after EnsureLuaAssets: %v", err)
	}
	if string(got) != wantCC {
		t.Fatalf("unexpected cc_config.json content: got %s want %s", string(got), wantCC)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "pow_config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected lua pow_config.json to stay absent, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "region_config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected lua region_config.json to stay absent, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "waf_config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected lua waf_config.json to stay absent, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(luaDir, "cc_config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected lua cc_config.json to stay absent, stat err = %v", err)
	}
}

func TestManagerApplyWritesRuntimeConfigFilesAndCleansLegacyCopies(t *testing.T) {
	tempDir := t.TempDir()
	certDir := filepath.Join(tempDir, "certs")
	luaDir := filepath.Join(tempDir, "lua")
	runtimeConfigDir := filepath.Join(tempDir, "runtime")
	for _, dir := range []string{certDir, luaDir, runtimeConfigDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
	}
	for _, path := range []string{filepath.Join(certDir, "pow_config.json"), filepath.Join(luaDir, "pow_config.json")} {
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}
	manager := &Manager{
		MainConfigPath:   filepath.Join(tempDir, "nginx.conf"),
		RouteConfigPath:  filepath.Join(tempDir, "routes.conf"),
		CertDir:          certDir,
		LuaDir:           luaDir,
		RuntimeConfigDir: runtimeConfigDir,
		Executor:         &fakeExecutor{},
	}
	outcome := manager.Apply(context.Background(), "main", "route", []protocol.SupportFile{
		{Path: "pow_config.json", Content: "pow-runtime"},
		{Path: "region_config.json", Content: "region-runtime"},
		{Path: "waf_config.json", Content: "waf-runtime"},
		{Path: "cc_config.json", Content: "cc-runtime"},
	})
	if outcome.Status != ApplyStatusSuccess {
		t.Fatalf("Apply failed: %#v", outcome)
	}
	data, err := os.ReadFile(filepath.Join(runtimeConfigDir, "pow_config.json"))
	if err != nil {
		t.Fatalf("failed to read runtime pow config: %v", err)
	}
	if string(data) != "pow-runtime" {
		t.Fatalf("unexpected runtime pow config: %s", string(data))
	}
	data, err = os.ReadFile(filepath.Join(runtimeConfigDir, "region_config.json"))
	if err != nil {
		t.Fatalf("failed to read runtime region config: %v", err)
	}
	if string(data) != "region-runtime" {
		t.Fatalf("unexpected runtime region config: %s", string(data))
	}
	data, err = os.ReadFile(filepath.Join(runtimeConfigDir, "waf_config.json"))
	if err != nil {
		t.Fatalf("failed to read runtime waf config: %v", err)
	}
	if string(data) != "waf-runtime" {
		t.Fatalf("unexpected runtime waf config: %s", string(data))
	}
	data, err = os.ReadFile(filepath.Join(runtimeConfigDir, "cc_config.json"))
	if err != nil {
		t.Fatalf("failed to read runtime cc config: %v", err)
	}
	if string(data) != "cc-runtime" {
		t.Fatalf("unexpected runtime cc config: %s", string(data))
	}
	for _, path := range []string{filepath.Join(certDir, "pow_config.json"), filepath.Join(luaDir, "pow_config.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected legacy pow config to be removed from %s, stat err = %v", path, err)
		}
	}
}

func TestManagerCurrentChecksumIncludesRuntimeConfigFiles(t *testing.T) {
	tempDir := t.TempDir()
	mainPath := filepath.Join(tempDir, "nginx.conf")
	routePath := filepath.Join(tempDir, "routes.conf")
	luaDir := filepath.Join(tempDir, "lua")
	runtimeConfigDir := filepath.Join(tempDir, "runtime")
	manager := &Manager{
		MainConfigPath:   mainPath,
		RouteConfigPath:  routePath,
		LuaDir:           luaDir,
		NginxLuaDir:      "/etc/nginx/dushengcdn-lua",
		RuntimeConfigDir: runtimeConfigDir,
		Executor:         &fakeExecutor{},
	}

	outcome := manager.Apply(
		context.Background(),
		"access_log __DUSHENGCDN_ACCESS_LOG__ dushengcdn_json;\n",
		"location /.within.website/x/cmd/anubis/static/ { alias __DUSHENGCDN_POW_STATIC_DIR__/; }\n",
		[]protocol.SupportFile{
			{Path: "pow_config.json", Content: `[{"domains":["pow.example.com"],"enabled":true}]`},
			{Path: "region_config.json", Content: `[{"domains":["region.example.com"],"enabled":true}]`},
			{Path: "waf_config.json", Content: `[{"domains":["waf.example.com"],"enabled":true}]`},
			{Path: "cc_config.json", Content: `[{"domains":["cc.example.com"],"enabled":true}]`},
		},
	)
	if outcome.Status != ApplyStatusSuccess {
		t.Fatalf("Apply failed: %#v", outcome)
	}

	value, err := manager.CurrentChecksum()
	if err != nil {
		t.Fatalf("CurrentChecksum failed: %v", err)
	}
	expected := bundleChecksum(
		"access_log __DUSHENGCDN_ACCESS_LOG__ dushengcdn_json;\n",
		"location /.within.website/x/cmd/anubis/static/ { alias __DUSHENGCDN_POW_STATIC_DIR__/; }\n",
		[]protocol.SupportFile{
			{Path: "pow_config.json", Content: `[{"domains":["pow.example.com"],"enabled":true}]`},
			{Path: "region_config.json", Content: `[{"domains":["region.example.com"],"enabled":true}]`},
			{Path: "waf_config.json", Content: `[{"domains":["waf.example.com"],"enabled":true}]`},
			{Path: "cc_config.json", Content: `[{"domains":["cc.example.com"],"enabled":true}]`},
		},
	)
	if value != expected {
		t.Fatalf("unexpected checksum with pow config: got %s want %s", value, expected)
	}
}

func TestManagedPowLuaFilesUseInternalChallengeFlow(t *testing.T) {
	if !strings.Contains(openRestyPowCheckLua, `function M.run()`) {
		t.Fatal("expected check.lua to expose a callable run function for the unified access handler")
	}
	if !strings.Contains(openRestyPowCheckLua, `return ngx.exec("/.within.website/x/cmd/anubis/api/make-challenge")`) {
		t.Fatal("expected check.lua to internally execute make-challenge instead of issuing a 302 redirect")
	}
	if strings.Contains(openRestyPowCheckLua, "ngx.redirect(") {
		t.Fatal("expected check.lua to avoid external redirects for challenge rendering")
	}
	if !strings.Contains(openRestyPowChallengeLua, `<h1 id="title" class="centered-div">`) {
		t.Fatal("expected challenge html to include Anubis-compatible title node")
	}
	if !strings.Contains(openRestyPowChallengeLua, `<div id="progress" role="progressbar" aria-labelledby="status"><div class="bar-inner"></div></div>`) {
		t.Fatal("expected challenge html to include Anubis-compatible progress markup")
	}
	if !strings.Contains(openRestyPowChallengeLua, `<script id="anubis_public_url" type="application/json">"__dushengcdn_internal__"</script>`) {
		t.Fatal("expected challenge html to force Anubis frontend to reuse the current URL as redir target")
	}
	if !strings.Contains(openRestyPowCheckLua, `pow_sessions:set(session_key, "1", session_ttl)`) {
		t.Fatal("expected check.lua to refresh the PoW session TTL on each valid request")
	}
	if !strings.Contains(openRestyPowCheckLua, `ngx.header["Set-Cookie"] = session_cookie(cookie_val, session_ttl)`) {
		t.Fatal("expected check.lua to refresh the browser session cookie on each valid request")
	}
	if !strings.Contains(openRestyPowCheckLua, `string.sub(uri, 1, #agent_api_prefix) == agent_api_prefix and ngx.var.http_x_agent_token`) {
		t.Fatal("expected check.lua to bypass authenticated agent API calls")
	}
	if !strings.Contains(openRestyPowCheckLua, `uri == "/api/dns-snapshot" or uri == "/api/dns-worker-heartbeat"`) {
		t.Fatal("expected check.lua to recognize DNS Worker API calls")
	}
	if !strings.Contains(openRestyPowCheckLua, `ngx.var.http_x_dns_worker_token`) {
		t.Fatal("expected check.lua to require DNS Worker token header for DNS Worker API bypass")
	}
	if !strings.Contains(openRestyPowChallengeLua, `local session_ttl = config.session_ttl or 600`) {
		t.Fatal("expected challenge.lua to default session TTL to 10 minutes")
	}
	if !strings.Contains(openRestyPowVerifyLua, `local session_ttl = challenge_info.session_ttl or 600`) {
		t.Fatal("expected verify.lua to default session TTL to 10 minutes")
	}
	if !strings.Contains(openRestyPowVerifyLua, `if ngx.var.scheme == "https" then`) {
		t.Fatal("expected verify.lua to only mark the session cookie as Secure for HTTPS requests")
	}
}

func TestProtectionLuaUsesSharedIPv4IPv6Matcher(t *testing.T) {
	for _, expected := range []string{
		`local function parse_ipv4`,
		`local function parse_ipv6`,
		`prefix > parsed.bits`,
		`function M.match_cidrs`,
		`mapped_v4`,
		`bytes[11] == 255 and bytes[12] == 255`,
		`parsed_ip.mapped_v4 and parsed_network.family == 4`,
		`parsed_network.mapped_v4 and parsed_ip.family == 4`,
		`prefix - 96`,
		`bit.band`,
	} {
		if !strings.Contains(openRestyIPMatcherLua, expected) {
			t.Fatalf("expected shared IP matcher lua to contain %q", expected)
		}
	}
	for name, lua := range map[string]string{
		"pow": openRestyPowPolicyLua,
		"waf": openRestyWAFCheckLua,
		"cc":  openRestyCCCheckLua,
	} {
		if !strings.Contains(lua, `require "shared.ipmatcher"`) {
			t.Fatalf("expected %s lua to require shared ip matcher", name)
		}
		if strings.Contains(lua, "ip_to_number") {
			t.Fatalf("expected %s lua to avoid IPv4-only ip_to_number matcher", name)
		}
	}
}

func TestGeoIPAccessLuaUsesLocalDatabaseThenFallbackAPIAndCache(t *testing.T) {
	checks := []string{
		`local ok_http, http = pcall(require, "resty.http")`,
		"local cached = ip_cache:get(ip)",
		"if ensure_mmdb() then",
		`local request_url = build_api_lookup_url(ip)`,
		`headers["Authorization"] = "Bearer " .. api_token`,
		`local function country_from_table(value)`,
		`country_from_api_payload(res.body)`,
		"local api_code = lookup_country_with_api(ip)",
		"ip_cache:set(ip, code, ttl)",
		"return ngx.exit(ngx.HTTP_FORBIDDEN)",
	}
	for _, expected := range checks {
		if !strings.Contains(openRestyGeoIPAccessLua, expected) {
			t.Fatalf("expected geoip access lua to contain %q", expected)
		}
	}
	if strings.Index(openRestyGeoIPAccessLua, "if ensure_mmdb() then") > strings.Index(openRestyGeoIPAccessLua, "local api_code = lookup_country_with_api(ip)") {
		t.Fatal("expected geoip access lua to query local mmdb before fallback API")
	}
}

func TestUnifiedAccessLuaRunsGeoIPWAFCCBeforePoW(t *testing.T) {
	regionIndex := strings.Index(openRestyGeoIPAccessLua, "M.check_region()")
	wafIndex := strings.Index(openRestyGeoIPAccessLua, `pcall(require, "waf.check")`)
	ccIndex := strings.Index(openRestyGeoIPAccessLua, `pcall(require, "cc.check")`)
	powIndex := strings.Index(openRestyGeoIPAccessLua, `pcall(require, "pow.check")`)
	if regionIndex < 0 || wafIndex < 0 || ccIndex < 0 || powIndex < 0 {
		t.Fatalf("expected access lua to include GeoIP, WAF, CC, and PoW hooks")
	}
	if !(regionIndex < wafIndex && wafIndex < ccIndex && ccIndex < powIndex) {
		t.Fatalf("expected access order to be GeoIP -> WAF -> CC -> PoW")
	}
	for _, expected := range []string{
		`local waf_config_dict = ngx.shared.dushengcdn_waf_config`,
		`"__DUSHENGCDN_RUNTIME_CONFIG_DIR__/waf_config.json"`,
		`function M.run()`,
		`return ngx.exit(ngx.HTTP_FORBIDDEN)`,
		`ngx.var.dushengcdn_request_reason = "恶意请求防护" .. action .. ": " .. label`,
		`ngx.header["X-DuShengCDN-WAF"] = "matched; mode=log; rule=" .. reason`,
		`local bots = {"sqlmap", "nikto", "acunetix", "masscan", "nessus", "nmap", "zgrab", "gobuster", "dirbuster"}`,
	} {
		if !strings.Contains(openRestyWAFCheckLua, expected) {
			t.Fatalf("expected waf lua to contain %q", expected)
		}
	}
	for _, expected := range []string{
		`local cc_config_dict = ngx.shared.dushengcdn_cc_config`,
		`local cc_counters = ngx.shared.dushengcdn_cc_counters`,
		`"__DUSHENGCDN_RUNTIME_CONFIG_DIR__/cc_config.json"`,
		`function M.run()`,
		`ngx.ctx.dushengcdn_force_pow = true`,
		`ngx.header["X-DuShengCDN-CC"] = "matched; mode=pow; rule=" .. reason`,
		`return ngx.exit(429)`,
		`CC 防护：同一来源`,
	} {
		if !strings.Contains(openRestyCCCheckLua, expected) {
			t.Fatalf("expected cc lua to contain %q", expected)
		}
	}
	if strings.Index(openRestyPowCheckLua, `if force_pow then`) > strings.Index(openRestyPowCheckLua, `if not force_pow then`) {
		t.Fatal("expected PoW force flag to be evaluated before normal whitelist bypass")
	}
}

func TestManagerRollbackRestoresCertFiles(t *testing.T) {
	tempDir := t.TempDir()
	routePath := filepath.Join(tempDir, "routes.conf")
	mainPath := filepath.Join(tempDir, "nginx.conf")
	certDir := filepath.Join(tempDir, "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("old-main"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(routePath, []byte("old-route"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "1.crt"), []byte("old-cert"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{
		MainConfigPath:  mainPath,
		RouteConfigPath: routePath,
		CertDir:         certDir,
		NginxCertDir:    "/etc/nginx/dushengcdn-certs",
		LuaDir:          filepath.Join(tempDir, "lua"),
		NginxLuaDir:     "/etc/nginx/dushengcdn-lua",
		Executor: &fakeExecutor{
			reloadErr: errors.New("openresty reload failed"),
		},
	}

	outcome := manager.Apply(context.Background(), "new-main", "new-route", []protocol.SupportFile{
		{Path: "1.crt", Content: "new-cert"},
	})
	if outcome.Status != ApplyStatusFatal {
		t.Fatalf("expected fatal apply outcome, got %#v", outcome)
	}

	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	if string(mainData) != "old-main" {
		t.Fatalf("expected main rollback, got %s", string(mainData))
	}
	routeData, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatalf("failed to read route config: %v", err)
	}
	if string(routeData) != "old-route" {
		t.Fatalf("expected route rollback, got %s", string(routeData))
	}
	certData, err := os.ReadFile(filepath.Join(certDir, "1.crt"))
	if err != nil {
		t.Fatalf("failed to read cert file: %v", err)
	}
	if string(certData) != "old-cert" {
		t.Fatalf("expected cert rollback, got %s", string(certData))
	}
}

func TestManagerApplyReturnsWarningWhenRollbackRecoversRuntime(t *testing.T) {
	tempDir := t.TempDir()
	routePath := filepath.Join(tempDir, "routes.conf")
	mainPath := filepath.Join(tempDir, "nginx.conf")
	certDir := filepath.Join(tempDir, "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("old-main"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(routePath, []byte("old-route"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "1.crt"), []byte("old-cert"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	manager := &Manager{
		MainConfigPath:  mainPath,
		RouteConfigPath: routePath,
		CertDir:         certDir,
		NginxCertDir:    "/etc/nginx/dushengcdn-certs",
		LuaDir:          filepath.Join(tempDir, "lua"),
		NginxLuaDir:     "/etc/nginx/dushengcdn-lua",
		Executor: &scriptedExecutor{
			reloadErrors: []error{errors.New("target config failed"), nil},
		},
	}

	outcome := manager.Apply(context.Background(), "new-main", "new-route", []protocol.SupportFile{
		{Path: "1.crt", Content: "new-cert"},
	})
	if outcome.Status != ApplyStatusWarning {
		t.Fatalf("expected warning apply outcome, got %#v", outcome)
	}

	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	if string(mainData) != "old-main" {
		t.Fatalf("expected main rollback, got %s", string(mainData))
	}
}

func TestManagerApplyStartsSafeFallbackWhenNoRollbackConfigExists(t *testing.T) {
	tempDir := t.TempDir()
	routePath := filepath.Join(tempDir, "routes.conf")
	mainPath := filepath.Join(tempDir, "nginx.conf")
	executor := &scriptedExecutor{
		testErrors: []error{errors.New("target config failed"), errors.New("rollback config missing"), nil},
	}
	manager := &Manager{
		MainConfigPath:               mainPath,
		RouteConfigPath:              routePath,
		OpenrestyObservabilityListen: "127.0.0.1:18081",
		Executor:                     executor,
	}

	outcome := manager.Apply(context.Background(), "bad-main", "bad-route", nil)
	if outcome.Status != ApplyStatusWarning {
		t.Fatalf("expected warning apply outcome, got %#v", outcome)
	}
	if !strings.Contains(outcome.Message, "fallback runtime started") {
		t.Fatalf("expected fallback message, got %q", outcome.Message)
	}
	if executor.testCalls != 3 {
		t.Fatalf("expected target, rollback, and fallback tests, got %d", executor.testCalls)
	}
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	if !strings.Contains(string(mainData), "DuShengCDN: No Valid Configuration") {
		t.Fatalf("expected safe fallback main config, got %s", string(mainData))
	}
	if !strings.Contains(string(mainData), "listen 80 default_server") {
		t.Fatalf("expected fallback to listen on port 80, got %s", string(mainData))
	}
	if !strings.Contains(string(mainData), "listen 127.0.0.1:18081") {
		t.Fatalf("expected fallback to expose local stub_status port, got %s", string(mainData))
	}
	if !strings.Contains(string(mainData), "stub_status;") {
		t.Fatalf("expected fallback to expose stub_status, got %s", string(mainData))
	}
	routeData, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatalf("failed to read route config: %v", err)
	}
	if len(routeData) != 0 {
		t.Fatalf("expected fallback route config to be empty, got %q", string(routeData))
	}
}

func TestManagerCertFileTargetPathRejectsEscapes(t *testing.T) {
	manager := &Manager{CertDir: filepath.Join(t.TempDir(), "certs")}
	if err := os.MkdirAll(manager.CertDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	absolutePath := "/tmp/evil.crt"
	if runtime.GOOS == "windows" {
		absolutePath = `C:/tmp/evil.crt`
	}

	testCases := []struct {
		path      string
		shouldErr bool
	}{
		{path: "nested/1.crt", shouldErr: false},
		{path: "../escape.crt", shouldErr: true},
		{path: "..\\escape.crt", shouldErr: true},
		{path: absolutePath, shouldErr: true},
		{path: "", shouldErr: true},
	}

	for _, testCase := range testCases {
		targetPath, err := manager.certFileTargetPath(testCase.path)
		if testCase.shouldErr {
			if err == nil {
				t.Fatalf("expected path %q to be rejected, got target %q", testCase.path, targetPath)
			}
			continue
		}
		if err != nil {
			t.Fatalf("expected path %q to be accepted: %v", testCase.path, err)
		}
		if !strings.HasPrefix(targetPath, manager.CertDir) {
			t.Fatalf("expected target path %q to stay under %q", targetPath, manager.CertDir)
		}
	}
}

func TestManagerApplyRejectsCertFilePathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	manager := &Manager{
		MainConfigPath:  filepath.Join(tempDir, "nginx.conf"),
		RouteConfigPath: filepath.Join(tempDir, "routes.conf"),
		CertDir:         filepath.Join(tempDir, "certs"),
		NginxCertDir:    "/etc/nginx/dushengcdn-certs",
		LuaDir:          filepath.Join(tempDir, "lua"),
		NginxLuaDir:     "/etc/nginx/dushengcdn-lua",
		Executor:        &fakeExecutor{},
	}

	outcome := manager.Apply(context.Background(), "main", "route", []protocol.SupportFile{
		{Path: "../escape.crt", Content: "bad"},
	})
	if outcome.Status != ApplyStatusWarning {
		t.Fatalf("expected warning apply outcome, got %#v", outcome)
	}

	if _, statErr := os.Stat(filepath.Join(tempDir, "escape.crt")); !os.IsNotExist(statErr) {
		t.Fatalf("expected escaped file to not exist, stat err = %v", statErr)
	}
}

func TestObservabilityListenAddress(t *testing.T) {
	if got := ObservabilityListenAddress("", 18081); got != "127.0.0.1:18081" {
		t.Fatalf("unexpected default observability listen address: %s", got)
	}
	if got := ObservabilityListenAddress("/usr/local/openresty/nginx/sbin/openresty", 18081); got != "127.0.0.1:18081" {
		t.Fatalf("unexpected path observability listen address: %s", got)
	}
}
