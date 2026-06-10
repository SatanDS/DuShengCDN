package nginx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"dushengcdn-agent/internal/protocol"
	"dushengcdn-agent/internal/security"
)

const CertDirPlaceholder = "__DUSHENGCDN_CERT_DIR__"
const RouteConfigPlaceholder = "__DUSHENGCDN_ROUTE_CONFIG__"
const AccessLogPlaceholder = "__DUSHENGCDN_ACCESS_LOG__"
const LuaDirPlaceholder = "__DUSHENGCDN_LUA_DIR__"
const RuntimeConfigDirPlaceholder = "__DUSHENGCDN_RUNTIME_CONFIG_DIR__"
const GeoIPDatabasePathPlaceholder = "__DUSHENGCDN_GEOIP_DATABASE_PATH__"
const GeoIPLookupAPIURLPlaceholder = "__DUSHENGCDN_GEOIP_LOOKUP_API_URL__"
const GeoIPLookupAPITokenPlaceholder = "__DUSHENGCDN_GEOIP_LOOKUP_API_TOKEN__"
const GeoIPLookupAPITimeoutPlaceholder = "__DUSHENGCDN_GEOIP_LOOKUP_API_TIMEOUT__"
const ObservabilityListenPlaceholder = "__DUSHENGCDN_OBSERVABILITY_LISTEN__"
const ObservabilityPortPlaceholder = "__DUSHENGCDN_OBSERVABILITY_PORT__"
const ResolverDirectivePlaceholder = "__DUSHENGCDN_RESOLVER_DIRECTIVE__"
const PowStaticDirPlaceholder = "__DUSHENGCDN_POW_STATIC_DIR__"

const (
	cachePurgeReadBatchSize = 256
	maxWarmCacheRedirects   = 5
	managedDirMarkerName    = ".dushengcdn-managed"
)

var blockedWarmIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
}

type Executor interface {
	Test(ctx context.Context) error
	Reload(ctx context.Context) error
	EnsureRuntime(ctx context.Context, recreate bool) error
	CheckHealth(ctx context.Context) error
	Restart(ctx context.Context) error
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type OSCommandRunner struct{}

func (r *OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	slog.Debug("OSCommandRunner starting command", "name", name, "args", args)
	tmpFile, err := os.CreateTemp("", "dushengcdn-cmd-*")
	if err != nil {
		slog.Error("OSCommandRunner failed to create temp file, falling back to CombinedOutput", "error", err)
		cmd := exec.CommandContext(ctx, name, args...)
		output, outErr := cmd.CombinedOutput()
		slog.Debug("OSCommandRunner finished CombinedOutput", "name", name, "error", outErr)
		return output, outErr
	}
	defer os.Remove(tmpFile.Name())

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = tmpFile
	cmd.Stderr = tmpFile

	slog.Debug("OSCommandRunner executing cmd.Run()", "name", name)
	runErr := cmd.Run()
	slog.Debug("OSCommandRunner cmd.Run() returned", "name", name, "error", runErr)
	tmpFile.Close()

	output, _ := os.ReadFile(tmpFile.Name())
	slog.Debug("OSCommandRunner command complete", "name", name, "output_len", len(output))
	return output, runErr
}

type PathExecutor struct {
	Path       string
	ConfigPath string
	Runner     CommandRunner
}

func (e *PathExecutor) Test(ctx context.Context) error {
	slog.Debug("running openresty test with binary", "path", e.Path, "config", e.ConfigPath)
	output, err := e.Runner.Run(ctx, e.Path, "-t", "-c", e.ConfigPath)
	if err != nil {
		return fmt.Errorf("openresty -t failed: %w: %s", err, sanitizeOpenRestyCommandOutput(output))
	}
	slog.Debug("openresty test succeeded with binary", "path", e.Path)
	return nil
}

func (e *PathExecutor) Reload(ctx context.Context) error {
	slog.Debug("running openresty reload with binary", "path", e.Path, "config", e.ConfigPath)
	output, err := e.Runner.Run(ctx, e.Path, "-s", "reload", "-c", e.ConfigPath)
	if err != nil {
		outputText := sanitizeOpenRestyCommandOutput(output)
		if isOpenrestyNotRunningError(string(output)) {
			slog.Warn("openresty reload reported runtime is not running, starting binary", "path", e.Path)
			startOutput, startErr := e.Runner.Run(ctx, e.Path, "-c", e.ConfigPath)
			if startErr != nil {
				return fmt.Errorf("openresty reload failed: %w: %s; start failed: %v: %s", err, outputText, startErr, sanitizeOpenRestyCommandOutput(startOutput))
			}
			return nil
		}
		return fmt.Errorf("openresty reload failed: %w: %s", err, outputText)
	}
	slog.Debug("openresty reload succeeded with binary", "path", e.Path)
	return nil
}

func (e *PathExecutor) EnsureRuntime(ctx context.Context, recreate bool) error {
	if err := e.Test(ctx); err != nil {
		return err
	}
	return e.Reload(ctx)
}

func (e *PathExecutor) CheckHealth(ctx context.Context) error {
	return e.Test(ctx)
}

func (e *PathExecutor) Restart(ctx context.Context) error {
	slog.Info("restarting openresty with binary", "path", e.Path, "config", e.ConfigPath)
	output, err := e.Runner.Run(ctx, e.Path, "-s", "quit", "-c", e.ConfigPath)
	if err != nil {
		text := sanitizeOpenRestyCommandOutput(output)
		if !isIgnorableOpenrestyStopError(text) {
			return fmt.Errorf("openresty stop failed: %w: %s", err, text)
		}
	}
	output, err = e.Runner.Run(ctx, e.Path, "-c", e.ConfigPath)
	if err != nil {
		return fmt.Errorf("openresty start failed: %w: %s", err, sanitizeOpenRestyCommandOutput(output))
	}
	slog.Info("openresty restart succeeded with binary", "path", e.Path)
	return nil
}

type Manager struct {
	MainConfigPath               string
	RouteConfigPath              string
	AccessLogPath                string
	CertDir                      string
	NginxCertDir                 string
	LuaDir                       string
	NginxLuaDir                  string
	RuntimeConfigDir             string
	GeoIPDatabasePath            string
	NginxGeoIPDatabasePath       string
	GeoIPLookupAPIURL            string
	GeoIPLookupAPIToken          string
	GeoIPLookupAPITimeout        time.Duration
	OpenrestyObservabilityListen string
	OpenrestyObservabilityPort   int
	OpenrestyResolverDirective   string
	Executor                     Executor
}

type ApplyStatus string

const (
	ApplyStatusSuccess ApplyStatus = "success"
	ApplyStatusWarning ApplyStatus = "warning"
	ApplyStatusFatal   ApplyStatus = "fatal"
)

const safeDefaultFallbackMainConfig = `# This file is generated by DuShengCDN safe default fallback.
worker_processes auto;
pid logs/nginx.pid;

events {
    worker_connections 1024;
}

http {
    server_tokens off;
    default_type text/plain;

    server {
        listen 80 default_server;
        server_name _;
        return 503 "DuShengCDN: No Valid Configuration\n";
    }
%s
}
`

const safeDefaultFallbackObservabilityServerBlock = `
    server {
        listen %s;
        server_name dushengcdn-observability;
        access_log off;
        allow 127.0.0.1;
        allow ::1;
        deny all;

        location = /dushengcdn/stub_status {
            stub_status;
        }
    }
`

type ApplyOutcome struct {
	Status  ApplyStatus
	Message string
}

func (m *Manager) Apply(ctx context.Context, mainConfig string, routeConfig string, supportFiles []protocol.SupportFile) ApplyOutcome {
	slog.Info("openresty apply started", "main_config", m.MainConfigPath, "route_config", m.RouteConfigPath, "cert_files", len(supportFiles))
	backup, err := m.backup()
	if err != nil {
		return fatalApplyOutcome(fmt.Errorf("backup openresty config failed: %w", err))
	}
	if err = m.writeTargetFiles(mainConfig, routeConfig, supportFiles); err != nil {
		return m.rollbackAfterFailedApply(ctx, backup, fmt.Errorf("write openresty config failed: %w", err))
	}
	if err = m.activateConfig(ctx); err != nil {
		return m.rollbackAfterFailedApply(ctx, backup, fmt.Errorf("activate openresty runtime failed: %w", err))
	}
	slog.Info("openresty apply completed successfully", "main_config", m.MainConfigPath, "route_config", m.RouteConfigPath)
	return ApplyOutcome{Status: ApplyStatusSuccess}
}

func (m *Manager) writeTargetFiles(mainConfig string, routeConfig string, supportFiles []protocol.SupportFile) error {
	if err := m.EnsureLuaAssets(); err != nil {
		return err
	}
	if err := m.writeCertFiles(supportFiles); err != nil {
		return err
	}
	if err := m.writePowConfig(supportFiles); err != nil {
		return err
	}
	if err := m.writeRegionConfig(supportFiles); err != nil {
		return err
	}
	if err := m.writeWAFConfig(supportFiles); err != nil {
		return err
	}
	if err := m.writeCCConfig(supportFiles); err != nil {
		return err
	}
	if err := m.ensureMimeTypes(); err != nil {
		return err
	}
	if strings.TrimSpace(m.OpenrestyResolverDirective) == "" && strings.Contains(routeConfig, "set $dushengcdn_upstream ") {
		slog.Warn("runtime-resolved hostname upstreams detected without available resolvers; hostname origin requests may fail until resolvers are configured")
	}
	renderedMainConfig := m.renderMainConfig(mainConfig)
	if err := ensureProxyCacheDirectories(renderedMainConfig); err != nil {
		return err
	}
	if err := os.WriteFile(m.MainConfigPath, []byte(renderedMainConfig), 0o644); err != nil {
		return err
	}
	renderedRouteConfig := m.renderRouteConfig(routeConfig)
	if err := os.WriteFile(m.RouteConfigPath, []byte(renderedRouteConfig), 0o644); err != nil {
		return err
	}
	return nil
}

func (m *Manager) activateConfig(ctx context.Context) error {
	if m.Executor == nil {
		return errors.New("executor 未配置")
	}
	if err := m.Executor.Test(ctx); err != nil {
		return err
	}
	return m.Executor.Reload(ctx)
}

func (m *Manager) rollbackAfterFailedApply(ctx context.Context, backup *backupState, applyErr error) ApplyOutcome {
	slog.Warn("openresty apply failed, restoring previous config", "error", applyErr)
	if err := m.restore(backup); err != nil {
		return fatalApplyOutcome(fmt.Errorf("restore openresty backup failed after apply error %v: %w", applyErr, err))
	}
	if err := m.activateConfig(ctx); err != nil {
		if backup != nil && backup.MainExisted {
			return fatalApplyOutcome(fmt.Errorf("apply failed: %v; rollback recovery failed: %w", applyErr, err))
		}
		if fallbackErr := m.EnsureSafeFallbackRuntime(ctx, fmt.Sprintf("apply failed: %v; rollback recovery failed: %v", applyErr, err)); fallbackErr != nil {
			return fatalApplyOutcome(fmt.Errorf("apply failed: %v; rollback recovery failed: %w; fallback recovery failed: %v", applyErr, err, fallbackErr))
		}
		message := fmt.Sprintf("apply failed, but fallback runtime started: %v; rollback recovery failed: %v", applyErr, err)
		slog.Warn("openresty apply recovered with safe default fallback", "message", message)
		return ApplyOutcome{
			Status:  ApplyStatusWarning,
			Message: message,
		}
	}
	message := fmt.Sprintf("apply failed, rolled back to previous config: %v", applyErr)
	slog.Warn("openresty apply rolled back successfully", "message", message)
	return ApplyOutcome{
		Status:  ApplyStatusWarning,
		Message: message,
	}
}

func fatalApplyOutcome(err error) ApplyOutcome {
	if err == nil {
		return ApplyOutcome{Status: ApplyStatusFatal}
	}
	return ApplyOutcome{
		Status:  ApplyStatusFatal,
		Message: security.RedactSensitiveText(strings.TrimSpace(err.Error())),
	}
}

func sanitizeOpenRestyCommandOutput(output []byte) string {
	return security.RedactSensitiveText(strings.TrimSpace(string(output)))
}

func (m *Manager) EnsureLuaAssets() error {
	if strings.TrimSpace(m.LuaDir) == "" {
		return nil
	}
	allSupportFiles := append(ManagedObservabilityLuaFiles(), ManagedSharedLuaFiles()...)
	allSupportFiles = append(allSupportFiles, m.managedPowLuaFiles()...)
	allSupportFiles = append(allSupportFiles, m.managedGeoIPLuaFiles()...)
	allSupportFiles = append(allSupportFiles, m.managedWAFLuaFiles()...)
	allSupportFiles = append(allSupportFiles, m.managedCCLuaFiles()...)
	powStaticFiles, err := ManagedPowStaticFiles()
	if err != nil {
		return fmt.Errorf("load pow static files: %w", err)
	}
	allSupportFiles = append(allSupportFiles, powStaticFiles...)
	files := make([]managedFile, 0, len(allSupportFiles))
	for _, file := range allSupportFiles {
		targetPath, err := luaFileTargetPath(m.LuaDir, file.Path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(m.LuaDir, targetPath)
		if err != nil {
			return err
		}
		files = append(files, managedFile{
			Path:    filepath.ToSlash(relativePath),
			Content: []byte(file.Content),
			Mode:    0o644,
		})
	}
	return syncManagedFiles(m.LuaDir, files)
}

func (m *Manager) EnsureRuntime(ctx context.Context, recreate bool) error {
	if m.Executor == nil {
		return errors.New("executor 未配置")
	}
	slog.Info("openresty ensure runtime requested", "recreate", recreate)
	return m.Executor.EnsureRuntime(ctx, recreate)
}

func (m *Manager) EnsureSafeFallbackRuntime(ctx context.Context, reason string) error {
	if m.Executor == nil {
		return errors.New("executor 未配置")
	}
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "no valid local openresty config is available"
	}
	slog.Warn("starting openresty safe default fallback runtime", "reason", trimmedReason)
	if err := m.writeSafeDefaultFallbackFiles(); err != nil {
		return fmt.Errorf("write safe default fallback config failed: %w", err)
	}
	if err := m.activateConfig(ctx); err != nil {
		return fmt.Errorf("activate safe default fallback runtime failed: %w", err)
	}
	slog.Warn("openresty safe default fallback runtime started", "main_config", m.MainConfigPath, "route_config", m.RouteConfigPath)
	return nil
}

func (m *Manager) CheckHealth(ctx context.Context) error {
	if m.Executor == nil {
		return errors.New("executor 未配置")
	}
	if m.MainConfigPath != "" {
		if _, err := os.Stat(m.MainConfigPath); os.IsNotExist(err) {
			return errors.New("openresty config not exists: waiting for initial sync")
		}
	}
	if m.OpenrestyObservabilityPort <= 0 {
		return m.Executor.CheckHealth(ctx)
	}
	return m.checkStubStatus(ctx)
}

func (m *Manager) Restart(ctx context.Context) error {
	if m.Executor == nil {
		return errors.New("executor 未配置")
	}
	slog.Info("openresty restart requested")
	return m.Executor.Restart(ctx)
}

func (m *Manager) PurgeCache(ctx context.Context, operation protocol.CacheOperation) error {
	scope := strings.ToLower(strings.TrimSpace(operation.Scope))
	if scope == "" {
		scope = "all"
	}
	if scope != "all" {
		return fmt.Errorf("cache purge scope %q is not supported yet", operation.Scope)
	}
	cachePath := strings.TrimSpace(operation.CachePath)
	if cachePath == "" {
		return errors.New("cache_path is empty")
	}
	cleanPath, err := safeCacheDirectory(cachePath)
	if err != nil {
		return err
	}
	return removeCacheEntries(ctx, cleanPath)
}

func (m *Manager) WarmCache(ctx context.Context, operation protocol.CacheOperation) error {
	allowedHosts := normalizeWarmAllowedHosts(operation.AllowedHosts)
	if len(allowedHosts) == 0 {
		return errors.New("cache warm requires allowed route hosts")
	}
	urls := normalizeWarmURLs(operation.URLs, allowedHosts)
	if len(urls) == 0 {
		return errors.New("cache warm requires at least one URL")
	}
	client := newWarmCacheHTTPClient(allowedHosts)
	defer client.CloseIdleConnections()
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return fmt.Errorf("warm cache %s failed: %w", targetURL, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("warm cache %s returned %s", targetURL, resp.Status)
		}
	}
	return nil
}

func removeCacheEntries(ctx context.Context, cleanPath string) error {
	dir, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer dir.Close()

	for {
		names, err := dir.Readdirnames(cachePurgeReadBatchSize)
		for _, name := range names {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			target := filepath.Join(cleanPath, name)
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove cache entry %s: %w", target, err)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func safeCacheDirectory(path string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return "", errors.New("cache path is unsafe")
	}
	if !isCacheAbsPath(cleanPath) {
		return "", errors.New("cache path must be absolute")
	}
	volume := filepath.VolumeName(cleanPath)
	root := volume + string(os.PathSeparator)
	if cleanPath == root || cleanPath == volume || len(cleanPath) < len(root)+4 {
		return "", errors.New("cache path is too broad")
	}
	components := cachePathComponents(cleanPath)
	if len(components) < 3 {
		return "", errors.New("cache path is too shallow")
	}
	if isProtectedCacheDirectory(components) {
		return "", errors.New("cache path points to a protected directory")
	}
	if !looksLikeCDNCacheDirectory(components) {
		return "", errors.New("cache path must clearly be a dushengcdn/openresty cache directory")
	}
	if resolvedPath, changed, err := resolveExistingCachePath(cleanPath); err == nil && changed {
		if err := validateResolvedCachePath(resolvedPath); err != nil {
			return "", err
		}
		return resolvedPath, nil
	} else if err != nil {
		return "", err
	}
	return cleanPath, nil
}

func isCacheAbsPath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(filepath.ToSlash(path), "/")
}

func cachePathComponents(path string) []string {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	parts := strings.FieldsFunc(strings.Trim(remainder, `/\`), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func isProtectedCacheDirectory(components []string) bool {
	if len(components) == 0 {
		return true
	}
	first := components[0]
	if len(components) <= 2 {
		switch first {
		case "bin", "boot", "dev", "etc", "home", "lib", "lib64", "opt", "proc", "root", "run", "sbin", "sys", "tmp", "usr", "var", "windows":
			return true
		}
	}
	return false
}

func looksLikeCDNCacheDirectory(components []string) bool {
	hasCache := false
	hasDuShengCDN := false
	hasRuntimeMarker := false
	for _, component := range components {
		compacted := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(component)
		if strings.Contains(compacted, "cache") {
			hasCache = true
		}
		if strings.Contains(compacted, "dushengcdn") {
			hasDuShengCDN = true
		}
		if strings.Contains(compacted, "openresty") || strings.Contains(compacted, "nginx") {
			hasRuntimeMarker = true
		}
	}
	return hasDuShengCDN && (hasCache || hasRuntimeMarker)
}

func resolveExistingCachePath(cleanPath string) (string, bool, error) {
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		resolvedPath = filepath.Clean(resolvedPath)
		return resolvedPath, resolvedPath != cleanPath, nil
	}
	if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("resolve cache path symlink: %w", err)
	}
	missingParts := make([]string, 0)
	current := cleanPath
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		missingParts = append(missingParts, filepath.Base(current))
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr == nil {
			resolved := filepath.Clean(resolvedParent)
			for index := len(missingParts) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missingParts[index])
			}
			resolved = filepath.Clean(resolved)
			return resolved, resolved != cleanPath, nil
		}
		if !os.IsNotExist(parentErr) {
			return "", false, fmt.Errorf("resolve cache path parent symlink: %w", parentErr)
		}
		current = parent
	}
}

func validateResolvedCachePath(resolvedPath string) error {
	resolvedComponents := cachePathComponents(resolvedPath)
	if !isCacheAbsPath(resolvedPath) ||
		len(resolvedComponents) < 3 ||
		isProtectedCacheDirectory(resolvedComponents) ||
		!looksLikeCDNCacheDirectory(resolvedComponents) {
		return errors.New("cache path symlink resolves outside a dushengcdn cache directory")
	}
	return nil
}

func ensureProxyCacheDirectories(mainConfig string) error {
	for _, cachePath := range proxyCacheDirectoriesFromConfig(mainConfig) {
		cleanPath, err := safeCacheDirectory(cachePath)
		if err != nil {
			return fmt.Errorf("proxy_cache_path %q is invalid: %w", cachePath, err)
		}
		if err := os.MkdirAll(cleanPath, 0o755); err != nil {
			return fmt.Errorf("create proxy cache path %s: %w", cleanPath, err)
		}
	}
	return nil
}

func proxyCacheDirectoriesFromConfig(mainConfig string) []string {
	matches := proxyCachePathPattern.FindAllStringSubmatch(mainConfig, -1)
	if len(matches) == 0 {
		return nil
	}
	result := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		cachePath := strings.Trim(strings.TrimSpace(match[1]), `"'`)
		if cachePath == "" {
			continue
		}
		cleanKey := filepath.Clean(cachePath)
		if _, exists := seen[cleanKey]; exists {
			continue
		}
		seen[cleanKey] = struct{}{}
		result = append(result, cachePath)
	}
	return result
}

func normalizeWarmURLs(values []string, allowedHosts []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		raw := strings.TrimSpace(value)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		if !isWarmURLAllowed(parsed, allowedHosts) {
			continue
		}
		parsed.Fragment = ""
		parsed.Host = strings.ToLower(parsed.Host)
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func newWarmCacheHTTPClient(allowedHosts []string) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           safeWarmDialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxWarmCacheRedirects {
				return errors.New("warm cache redirect limit exceeded")
			}
			if req == nil || req.URL == nil || !isWarmURLAllowed(req.URL, allowedHosts) {
				return errors.New("warm cache redirect target is unsafe")
			}
			if len(via) > 0 && !strings.EqualFold(warmURLHostname(req.URL), warmURLHostname(via[0].URL)) {
				return errors.New("warm cache redirect changed host")
			}
			return nil
		},
	}
}

func isWarmURLAllowed(parsed *url.URL, allowedHosts []string) bool {
	if parsed == nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	host := warmURLHostname(parsed)
	if host == "" || !warmHostAllowed(host, allowedHosts) {
		return false
	}
	port := parsed.Port()
	return port == "" ||
		(parsed.Scheme == "http" && port == "80") ||
		(parsed.Scheme == "https" && port == "443")
}

func normalizeWarmAllowedHosts(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		host := warmURLHostname(&url.URL{Host: strings.TrimSpace(value)})
		if host == "" {
			host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
		}
		if host == "" || strings.ContainsAny(host, `/\:`) {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		result = append(result, host)
	}
	return result
}

func warmHostAllowed(host string, allowedHosts []string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	for _, allowedHost := range allowedHosts {
		allowed := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(allowedHost)), ".")
		if allowed == "" {
			continue
		}
		if allowed == host {
			return true
		}
		if !strings.HasPrefix(allowed, "*.") {
			continue
		}
		suffix := strings.TrimPrefix(allowed, "*.")
		if suffix == "" || !strings.HasSuffix(host, "."+suffix) {
			continue
		}
		prefix := strings.TrimSuffix(host, "."+suffix)
		if prefix != "" && !strings.Contains(prefix, ".") {
			return true
		}
	}
	return false
}

func warmURLHostname(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
}

func safeWarmDialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("warm cache network %q is not allowed", network)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := resolveWarmHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("warm cache host %q resolved to no addresses", host)
	}
	for _, ip := range ips {
		if !isSafeWarmIP(ip) {
			return nil, fmt.Errorf("warm cache target %q resolved to blocked address %s", host, ip.String())
		}
	}

	var lastErr error
	var dialer net.Dialer
	for _, ip := range ips {
		if network == "tcp4" && ip.To4() == nil {
			continue
		}
		if network == "tcp6" && ip.To4() != nil {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("warm cache host %q has no address for network %s", host, network)
}

func resolveWarmHost(ctx context.Context, host string) ([]net.IP, error) {
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if addr.IP != nil {
			ips = append(ips, addr.IP)
		}
	}
	return ips, nil
}

func isSafeWarmIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedWarmIPPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func (m *Manager) CurrentChecksum() (string, error) {
	if m.RouteConfigPath == "" {
		return "", errors.New("route config path 不能为空")
	}
	if m.MainConfigPath == "" {
		return "", errors.New("main config path 不能为空")
	}
	mainData, err := os.ReadFile(m.MainConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	data, err := os.ReadFile(m.RouteConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	normalizedMain := string(mainData)
	if includePath := m.routeConfigIncludePath(); includePath != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, includePath, RouteConfigPlaceholder)
	}
	if accessLogPath := m.accessLogRuntimePath(); accessLogPath != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, accessLogPath, AccessLogPlaceholder)
	}
	if luaDir := m.luaRuntimePath(); luaDir != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, luaDir, LuaDirPlaceholder)
	}
	if geoIPDatabasePath := m.geoIPDatabaseRuntimePath(); geoIPDatabasePath != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, geoIPDatabasePath, GeoIPDatabasePathPlaceholder)
	}
	if listen := strings.TrimSpace(m.OpenrestyObservabilityListen); listen != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, listen, ObservabilityListenPlaceholder)
	}
	if m.OpenrestyObservabilityPort > 0 {
		normalizedMain = strings.ReplaceAll(normalizedMain, fmt.Sprintf("%d", m.OpenrestyObservabilityPort), ObservabilityPortPlaceholder)
	}
	if resolverDirective := strings.TrimSpace(m.OpenrestyResolverDirective); resolverDirective != "" {
		normalizedMain = strings.ReplaceAll(normalizedMain, resolverDirective, ResolverDirectivePlaceholder)
	}
	normalizedRoute := string(data)
	if m.NginxCertDir != "" {
		normalizedRoute = strings.ReplaceAll(normalizedRoute, m.NginxCertDir, CertDirPlaceholder)
	}
	if luaDir := m.luaRuntimePath(); luaDir != "" {
		normalizedRoute = strings.ReplaceAll(normalizedRoute, luaDir+"/pow/static", PowStaticDirPlaceholder)
		normalizedRoute = strings.ReplaceAll(normalizedRoute, luaDir, LuaDirPlaceholder)
	}
	if geoIPDatabasePath := m.geoIPDatabaseRuntimePath(); geoIPDatabasePath != "" {
		normalizedRoute = strings.ReplaceAll(normalizedRoute, geoIPDatabasePath, GeoIPDatabasePathPlaceholder)
	}
	files, err := m.readManagedSupportFiles()
	if err != nil {
		return "", err
	}
	result := bundleChecksum(normalizedMain, normalizedRoute, files)
	slog.Debug("openresty current checksum calculated", "main_config", m.MainConfigPath, "route_config", m.RouteConfigPath, "checksum", result, "cert_files", len(files))
	return result, nil
}

type ExecutorOptions struct {
	NginxPath                  string
	MainConfigPath             string
	RouteConfigPath            string
	CertDir                    string
	NginxCertDir               string
	LuaDir                     string
	NginxLuaDir                string
	GeoIPDatabasePath          string
	NginxGeoIPDatabasePath     string
	GeoIPLookupAPIURL          string
	GeoIPLookupAPIToken        string
	GeoIPLookupAPITimeout      time.Duration
	OpenrestyObservabilityPort int
}

func NewExecutor(options ExecutorOptions) Executor {
	runner := &OSCommandRunner{}
	return &PathExecutor{
		Path:       strings.TrimSpace(options.NginxPath),
		ConfigPath: strings.TrimSpace(options.MainConfigPath),
		Runner:     runner,
	}
}

func DetectVersion(ctx context.Context, options ExecutorOptions) string {
	version, err := detectVersion(ctx, options, &OSCommandRunner{})
	if err != nil {
		slog.Error("detect openresty version failed", "error", err)
		return ""
	}
	slog.Info("detected openresty version", "version", version)
	return version
}

func detectVersion(ctx context.Context, options ExecutorOptions, runner CommandRunner) (string, error) {
	if runner == nil {
		runner = &OSCommandRunner{}
	}
	if options.NginxPath != "" {
		output, err := runner.Run(ctx, options.NginxPath, "-v")
		if err != nil {
			return "", fmt.Errorf("run runtime -v failed: %w: %s", err, string(output))
		}
		version := parseNginxVersion(string(output))
		if version == "" {
			return "", errors.New("cannot parse runtime version from binary output")
		}
		return version, nil
	}
	return "", errors.New("openresty path is empty")
}

func parseNginxVersion(output string) string {
	matches := nginxVersionPattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

var nginxVersionPattern = regexp.MustCompile(`(?im)(?:nginx|openresty) version:\s*(?:nginx|openresty)/([^\s]+)`)
var proxyCachePathPattern = regexp.MustCompile(`(?im)^\s*proxy_cache_path\s+("[^"]+"|'[^']+'|[^\s;]+)`)

func isIgnorableOpenrestyStopError(output string) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if text == "" {
		return false
	}
	return strings.Contains(text, "invalid pid") || strings.Contains(text, "no such process")
}

func isOpenrestyNotRunningError(output string) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if text == "" {
		return false
	}
	return strings.Contains(text, "invalid pid") ||
		strings.Contains(text, "no such process") ||
		strings.Contains(text, "open()") && strings.Contains(text, "nginx.pid") && strings.Contains(text, "failed")
}

type backupState struct {
	MainExisted  bool
	MainData     []byte
	RouteExisted bool
	RouteData    []byte
	Files        []protocol.SupportFile
	PowConfig    *protocol.SupportFile
	RegionConfig *protocol.SupportFile
	WAFConfig    *protocol.SupportFile
	CCConfig     *protocol.SupportFile
}

type managedFile struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

func (m *Manager) backup() (*backupState, error) {
	if m.MainConfigPath == "" {
		return nil, errors.New("main config path 不能为空")
	}
	if m.RouteConfigPath == "" {
		return nil, errors.New("route config path 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(m.MainConfigPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(m.RouteConfigPath), 0o755); err != nil {
		return nil, err
	}
	if m.AccessLogPath != "" {
		if err := os.MkdirAll(filepath.Dir(m.AccessLogPath), 0o755); err != nil {
			return nil, err
		}
	}
	if m.CertDir != "" {
		if err := os.MkdirAll(m.CertDir, 0o755); err != nil {
			return nil, err
		}
	}
	if m.RuntimeConfigDir != "" {
		if err := os.MkdirAll(m.RuntimeConfigDir, 0o755); err != nil {
			return nil, err
		}
	}
	state := &backupState{}
	mainData, err := os.ReadFile(m.MainConfigPath)
	if err == nil {
		state.MainExisted = true
		state.MainData = mainData
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	data, err := os.ReadFile(m.RouteConfigPath)
	if err == nil {
		state.RouteExisted = true
		state.RouteData = data
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	files, err := m.readCertFiles()
	if err != nil {
		return nil, err
	}
	state.Files = files
	powConfig, err := m.readPowConfigFile()
	if err != nil {
		return nil, err
	}
	state.PowConfig = powConfig
	regionConfig, err := m.readRegionConfigFile()
	if err != nil {
		return nil, err
	}
	state.RegionConfig = regionConfig
	wafConfig, err := m.readWAFConfigFile()
	if err != nil {
		return nil, err
	}
	state.WAFConfig = wafConfig
	ccConfig, err := m.readCCConfigFile()
	if err != nil {
		return nil, err
	}
	state.CCConfig = ccConfig
	slog.Debug("backup captured", "main_exists", state.MainExisted, "route_exists", state.RouteExisted, "cert_files", len(state.Files))
	return state, nil
}

func (m *Manager) restore(state *backupState) error {
	if state == nil {
		return nil
	}
	slog.Warn("restoring nginx backup", "main_existed", state.MainExisted, "route_existed", state.RouteExisted, "cert_files", len(state.Files))
	if state.MainExisted {
		if err := os.WriteFile(m.MainConfigPath, state.MainData, 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(m.MainConfigPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if state.RouteExisted {
		if err := os.WriteFile(m.RouteConfigPath, state.RouteData, 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(m.RouteConfigPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if m.CertDir != "" {
		if err := m.writeManagedCertFiles(state.Files); err != nil {
			return err
		}
	}
	if err := m.restorePowConfig(state); err != nil {
		return err
	}
	if err := m.restoreRegionConfig(state); err != nil {
		return err
	}
	if err := m.restoreWAFConfig(state); err != nil {
		return err
	}
	return m.restoreCCConfig(state)
}

func (m *Manager) writeCertFiles(certFiles []protocol.SupportFile) error {
	if m.CertDir == "" {
		return nil
	}
	return m.writeManagedCertFiles(certFiles)
}

func (m *Manager) writePowConfig(supportFiles []protocol.SupportFile) error {
	if m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "pow_config.json")
	for _, file := range supportFiles {
		if file.Path == "pow_config.json" {
			if err := os.WriteFile(configPath, []byte(file.Content), 0o644); err != nil {
				return fmt.Errorf("write pow_config.json: %w", err)
			}
			slog.Info("wrote pow config", "path", configPath, "size", len(file.Content))
			return nil
		}
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pow_config.json: %w", err)
	}
	if err := removeLegacyPowConfig(filepath.Join(m.LuaDir, "pow_config.json")); err != nil {
		return err
	}
	if err := removeLegacyPowConfig(filepath.Join(m.CertDir, "pow_config.json")); err != nil {
		return err
	}
	return nil
}

func (m *Manager) writeRegionConfig(supportFiles []protocol.SupportFile) error {
	if m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "region_config.json")
	for _, file := range supportFiles {
		if file.Path == "region_config.json" {
			if err := os.WriteFile(configPath, []byte(file.Content), 0o644); err != nil {
				return fmt.Errorf("write region_config.json: %w", err)
			}
			slog.Info("wrote region config", "path", configPath, "size", len(file.Content))
			return nil
		}
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove region_config.json: %w", err)
	}
	return nil
}

func (m *Manager) writeWAFConfig(supportFiles []protocol.SupportFile) error {
	if m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "waf_config.json")
	for _, file := range supportFiles {
		if file.Path == "waf_config.json" {
			if err := os.WriteFile(configPath, []byte(file.Content), 0o644); err != nil {
				return fmt.Errorf("write waf_config.json: %w", err)
			}
			slog.Info("wrote waf config", "path", configPath, "size", len(file.Content))
			return nil
		}
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove waf_config.json: %w", err)
	}
	return nil
}

func (m *Manager) writeCCConfig(supportFiles []protocol.SupportFile) error {
	if m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "cc_config.json")
	for _, file := range supportFiles {
		if file.Path == "cc_config.json" {
			if err := os.WriteFile(configPath, []byte(file.Content), 0o644); err != nil {
				return fmt.Errorf("write cc_config.json: %w", err)
			}
			slog.Info("wrote cc config", "path", configPath, "size", len(file.Content))
			return nil
		}
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cc_config.json: %w", err)
	}
	return nil
}

func (m *Manager) writeManagedCertFiles(certFiles []protocol.SupportFile) error {
	files := make([]managedFile, 0, len(certFiles))
	for _, file := range certFiles {
		if file.Path == "pow_config.json" || file.Path == "region_config.json" || file.Path == "waf_config.json" || file.Path == "cc_config.json" {
			continue
		}
		targetPath, err := m.certFileTargetPath(file.Path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(m.CertDir, targetPath)
		if err != nil {
			return err
		}
		files = append(files, managedFile{
			Path:    filepath.ToSlash(relativePath),
			Content: []byte(file.Content),
			Mode:    certFileMode(file.Path),
		})
	}
	return syncManagedFiles(m.CertDir, files)
}

func (m *Manager) readCertFiles() ([]protocol.SupportFile, error) {
	if m.CertDir == "" {
		return nil, nil
	}
	if _, err := os.Stat(m.CertDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]protocol.SupportFile, 0)
	err := filepath.Walk(m.CertDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(m.CertDir, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(relativePath) == "pow_config.json" {
			return nil
		}
		if filepath.ToSlash(relativePath) == "region_config.json" {
			return nil
		}
		if filepath.ToSlash(relativePath) == "waf_config.json" {
			return nil
		}
		if filepath.ToSlash(relativePath) == "cc_config.json" {
			return nil
		}
		if filepath.ToSlash(relativePath) == managedDirMarkerName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, protocol.SupportFile{
			Path:    filepath.ToSlash(relativePath),
			Content: string(data),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i int, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (m *Manager) readPowConfigFile() (*protocol.SupportFile, error) {
	if m.RuntimeConfigDir == "" {
		return nil, nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "pow_config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &protocol.SupportFile{
		Path:    "pow_config.json",
		Content: string(data),
	}, nil
}

func (m *Manager) readRegionConfigFile() (*protocol.SupportFile, error) {
	if m.RuntimeConfigDir == "" {
		return nil, nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "region_config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &protocol.SupportFile{
		Path:    "region_config.json",
		Content: string(data),
	}, nil
}

func (m *Manager) readWAFConfigFile() (*protocol.SupportFile, error) {
	if m.RuntimeConfigDir == "" {
		return nil, nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "waf_config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &protocol.SupportFile{
		Path:    "waf_config.json",
		Content: string(data),
	}, nil
}

func (m *Manager) readCCConfigFile() (*protocol.SupportFile, error) {
	if m.RuntimeConfigDir == "" {
		return nil, nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "cc_config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &protocol.SupportFile{
		Path:    "cc_config.json",
		Content: string(data),
	}, nil
}

func (m *Manager) readManagedSupportFiles() ([]protocol.SupportFile, error) {
	files, err := m.readCertFiles()
	if err != nil {
		return nil, err
	}
	powConfig, err := m.readPowConfigFile()
	if err != nil {
		return nil, err
	}
	if powConfig != nil {
		files = append(files, *powConfig)
	}
	regionConfig, err := m.readRegionConfigFile()
	if err != nil {
		return nil, err
	}
	if regionConfig != nil {
		files = append(files, *regionConfig)
	}
	wafConfig, err := m.readWAFConfigFile()
	if err != nil {
		return nil, err
	}
	if wafConfig != nil {
		files = append(files, *wafConfig)
	}
	ccConfig, err := m.readCCConfigFile()
	if err != nil {
		return nil, err
	}
	if ccConfig != nil {
		files = append(files, *ccConfig)
	}
	return files, nil
}

func (m *Manager) restorePowConfig(state *backupState) error {
	if state == nil || m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "pow_config.json")
	if state.PowConfig == nil {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(configPath, []byte(state.PowConfig.Content), 0o644)
}

func (m *Manager) restoreRegionConfig(state *backupState) error {
	if state == nil || m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "region_config.json")
	if state.RegionConfig == nil {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(configPath, []byte(state.RegionConfig.Content), 0o644)
}

func (m *Manager) restoreWAFConfig(state *backupState) error {
	if state == nil || m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "waf_config.json")
	if state.WAFConfig == nil {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(configPath, []byte(state.WAFConfig.Content), 0o644)
}

func (m *Manager) restoreCCConfig(state *backupState) error {
	if state == nil || m.RuntimeConfigDir == "" {
		return nil
	}
	configPath := filepath.Join(m.RuntimeConfigDir, "cc_config.json")
	if state.CCConfig == nil {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(configPath, []byte(state.CCConfig.Content), 0o644)
}

func (m *Manager) writeSafeDefaultFallbackFiles() error {
	if strings.TrimSpace(m.MainConfigPath) == "" {
		return errors.New("main config path 不能为空")
	}
	if strings.TrimSpace(m.RouteConfigPath) == "" {
		return errors.New("route config path 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(m.MainConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.RouteConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.RouteConfigPath, nil, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(m.MainConfigPath, []byte(m.safeDefaultFallbackMainConfig()), 0o644); err != nil {
		return err
	}
	return nil
}

func (m *Manager) safeDefaultFallbackMainConfig() string {
	observabilityBlock := ""
	if listen := strings.TrimSpace(m.OpenrestyObservabilityListen); listen != "" {
		observabilityBlock = fmt.Sprintf(safeDefaultFallbackObservabilityServerBlock, listen)
	}
	return fmt.Sprintf(safeDefaultFallbackMainConfig, observabilityBlock)
}

func (m *Manager) checkStubStatus(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	openrestyStubUrl := fmt.Sprintf("http://127.0.0.1:%d/dushengcdn/stub_status", m.OpenrestyObservabilityPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openrestyStubUrl, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("openresty health endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openresty health endpoint returned %s", resp.Status)
	}
	slog.Debug("openresty health endpoint is healthy", "url", openrestyStubUrl)
	return nil
}

func removeLegacyPowConfig(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy pow_config.json %q: %w", path, err)
	}
	return nil
}

func (m *Manager) certFileTargetPath(relativePath string) (string, error) {
	if strings.TrimSpace(m.CertDir) == "" {
		return "", errors.New("cert dir 不能为空")
	}
	candidate := strings.TrimSpace(relativePath)
	if strings.Contains(candidate, `\`) {
		candidate = strings.ReplaceAll(candidate, `\`, "/")
	}
	normalizedPath := filepath.Clean(filepath.FromSlash(candidate))
	if normalizedPath == "." || normalizedPath == "" {
		return "", errors.New("cert file path 不能为空")
	}
	if filepath.IsAbs(normalizedPath) || filepath.VolumeName(normalizedPath) != "" {
		return "", fmt.Errorf("cert file path %q must be relative", relativePath)
	}
	targetPath := filepath.Join(m.CertDir, normalizedPath)
	relativeToBase, err := filepath.Rel(m.CertDir, targetPath)
	if err != nil {
		return "", err
	}
	if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("cert file path %q escapes cert dir", relativePath)
	}
	return targetPath, nil
}

func certFileMode(relativePath string) fs.FileMode {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(relativePath))) {
	case ".crt", ".pem":
		return 0o644
	case ".key":
		return 0o600
	default:
		return 0o644
	}
}

func luaFileTargetPath(baseDir string, relativePath string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", errors.New("lua dir 不能为空")
	}
	candidate := strings.TrimSpace(relativePath)
	if strings.Contains(candidate, `\`) {
		candidate = strings.ReplaceAll(candidate, `\`, "/")
	}
	normalizedPath := filepath.Clean(filepath.FromSlash(candidate))
	if normalizedPath == "." || normalizedPath == "" {
		return "", errors.New("lua file path 不能为空")
	}
	if filepath.IsAbs(normalizedPath) || filepath.VolumeName(normalizedPath) != "" {
		return "", fmt.Errorf("lua file path %q must be relative", relativePath)
	}
	targetPath := filepath.Join(baseDir, normalizedPath)
	relativeToBase, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return "", err
	}
	if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("lua file path %q escapes lua dir", relativePath)
	}
	return targetPath, nil
}

func syncManagedFiles(baseDir string, files []managedFile) error {
	if strings.TrimSpace(baseDir) == "" {
		return errors.New("managed dir cannot be empty")
	}
	if info, err := os.Stat(baseDir); err == nil && !info.IsDir() {
		return fmt.Errorf("managed dir %q is not a directory", baseDir)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	desired := make(map[string]managedFile, len(files))
	for _, file := range files {
		cleanPath, err := cleanManagedFilePath(file.Path)
		if err != nil {
			return err
		}
		if cleanPath == managedDirMarkerName {
			return fmt.Errorf("managed file path %q is reserved", file.Path)
		}
		desired[cleanPath] = managedFile{
			Path:    cleanPath,
			Content: file.Content,
			Mode:    file.Mode,
		}
	}

	if err := ensureManagedDirCanBeCleaned(baseDir, desired); err != nil {
		return err
	}

	if err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		if _, ok := desired[filepath.Clean(relativePath)]; ok {
			return nil
		}
		if filepath.Clean(relativePath) == managedDirMarkerName {
			return nil
		}
		return os.Remove(path)
	}); err != nil {
		return err
	}

	for _, file := range desired {
		targetPath := filepath.Join(baseDir, file.Path)
		relativeToBase, err := filepath.Rel(baseDir, targetPath)
		if err != nil {
			return err
		}
		if relativeToBase == ".." || strings.HasPrefix(relativeToBase, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("managed file path %q escapes managed dir", file.Path)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, file.Content, file.Mode); err != nil {
			return err
		}
	}

	if err := writeManagedDirMarker(baseDir); err != nil {
		return err
	}
	return removeEmptyManagedDirs(baseDir)
}

func ensureManagedDirCanBeCleaned(baseDir string, desired map[string]managedFile) error {
	hasMarker, err := managedDirHasMarker(baseDir)
	if err != nil {
		return err
	}
	if hasMarker {
		return nil
	}
	unknownFiles, err := unmanagedExistingFiles(baseDir, desired)
	if err != nil {
		return err
	}
	if len(unknownFiles) == 0 {
		return nil
	}
	sort.Strings(unknownFiles)
	return fmt.Errorf("managed dir %q is missing %s and contains unknown files: %s", baseDir, managedDirMarkerName, strings.Join(unknownFiles, ", "))
}

func managedDirHasMarker(baseDir string) (bool, error) {
	info, err := os.Stat(filepath.Join(baseDir, managedDirMarkerName))
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func unmanagedExistingFiles(baseDir string, desired map[string]managedFile) ([]string, error) {
	unknownFiles := make([]string, 0)
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		cleanPath := filepath.Clean(relativePath)
		if cleanPath == managedDirMarkerName {
			return nil
		}
		if isKnownLegacyManagedFile(cleanPath) {
			return nil
		}
		if _, ok := desired[cleanPath]; ok {
			return nil
		}
		unknownFiles = append(unknownFiles, filepath.ToSlash(cleanPath))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unknownFiles, nil
}

func isKnownLegacyManagedFile(path string) bool {
	switch filepath.ToSlash(filepath.Clean(path)) {
	case "pow_config.json", "region_config.json", "waf_config.json", "cc_config.json":
		return true
	default:
		return false
	}
}

func writeManagedDirMarker(baseDir string) error {
	markerPath := filepath.Join(baseDir, managedDirMarkerName)
	return os.WriteFile(markerPath, []byte("DuShengCDN managed directory\n"), 0o644)
}

func cleanManagedFilePath(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if strings.Contains(candidate, `\`) {
		candidate = strings.ReplaceAll(candidate, `\`, "/")
	}
	cleanPath := filepath.Clean(filepath.FromSlash(candidate))
	if cleanPath == "." || cleanPath == "" {
		return "", errors.New("managed file path cannot be empty")
	}
	if filepath.IsAbs(cleanPath) || filepath.VolumeName(cleanPath) != "" {
		return "", fmt.Errorf("managed file path %q must be relative", raw)
	}
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("managed file path %q escapes managed dir", raw)
	}
	return cleanPath, nil
}

func removeEmptyManagedDirs(baseDir string) error {
	dirs := make([]string, 0)
	if err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != baseDir {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(dirs, func(i int, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) ensureMimeTypes() error {
	if m.MainConfigPath == "" {
		return nil
	}
	configDir := filepath.Dir(m.MainConfigPath)
	mimeTypesPath := filepath.Join(configDir, "mime.types")
	if _, err := os.Stat(mimeTypesPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(mimeTypesPath, []byte(DefaultMimeTypes), 0o644)
}

func (m *Manager) renderRouteConfig(content string) string {
	rendered := content
	if m.NginxCertDir != "" {
		rendered = strings.ReplaceAll(rendered, CertDirPlaceholder, m.NginxCertDir)
	}
	if luaDir := m.luaRuntimePath(); luaDir != "" {
		rendered = strings.ReplaceAll(rendered, LuaDirPlaceholder, luaDir)
		rendered = strings.ReplaceAll(rendered, PowStaticDirPlaceholder, luaDir+"/pow/static")
	}
	if geoIPDatabasePath := m.geoIPDatabaseRuntimePath(); geoIPDatabasePath != "" {
		rendered = strings.ReplaceAll(rendered, GeoIPDatabasePathPlaceholder, geoIPDatabasePath)
	}
	return rendered
}

func (m *Manager) renderMainConfig(content string) string {
	rendered := content
	if includePath := m.routeConfigIncludePath(); includePath != "" {
		rendered = strings.ReplaceAll(rendered, RouteConfigPlaceholder, includePath)
	}
	if accessLogPath := m.accessLogRuntimePath(); accessLogPath != "" {
		rendered = strings.ReplaceAll(rendered, AccessLogPlaceholder, accessLogPath)
	}
	if luaDir := m.luaRuntimePath(); luaDir != "" {
		rendered = strings.ReplaceAll(rendered, LuaDirPlaceholder, luaDir)
	}
	if geoIPDatabasePath := m.geoIPDatabaseRuntimePath(); geoIPDatabasePath != "" {
		rendered = strings.ReplaceAll(rendered, GeoIPDatabasePathPlaceholder, geoIPDatabasePath)
	}
	if listen := strings.TrimSpace(m.OpenrestyObservabilityListen); listen != "" {
		rendered = strings.ReplaceAll(rendered, ObservabilityListenPlaceholder, listen)
	}
	if m.OpenrestyObservabilityPort > 0 {
		rendered = strings.ReplaceAll(rendered, ObservabilityPortPlaceholder, fmt.Sprintf("%d", m.OpenrestyObservabilityPort))
	}
	if resolverDirective := strings.TrimSpace(m.OpenrestyResolverDirective); resolverDirective != "" {
		rendered = strings.ReplaceAll(rendered, ResolverDirectivePlaceholder, resolverDirective)
	}
	return rendered
}

func (m *Manager) managedPowLuaFiles() []protocol.SupportFile {
	files := ManagedPowLuaFiles()
	runtimeConfigDir := filepath.ToSlash(strings.TrimSpace(m.RuntimeConfigDir))
	for index := range files {
		files[index].Content = strings.ReplaceAll(files[index].Content, RuntimeConfigDirPlaceholder, runtimeConfigDir)
		files[index].Content = strings.ReplaceAll(files[index].Content, GeoIPDatabasePathPlaceholder, m.geoIPDatabaseRuntimePath())
	}
	return files
}

func (m *Manager) managedGeoIPLuaFiles() []protocol.SupportFile {
	files := ManagedGeoIPLuaFiles()
	runtimeConfigDir := filepath.ToSlash(strings.TrimSpace(m.RuntimeConfigDir))
	for index := range files {
		files[index].Content = strings.ReplaceAll(files[index].Content, RuntimeConfigDirPlaceholder, runtimeConfigDir)
		files[index].Content = strings.ReplaceAll(files[index].Content, GeoIPDatabasePathPlaceholder, m.geoIPDatabaseRuntimePath())
		files[index].Content = strings.ReplaceAll(files[index].Content, GeoIPLookupAPIURLPlaceholder, m.luaStringLiteral(m.GeoIPLookupAPIURL))
		files[index].Content = strings.ReplaceAll(files[index].Content, GeoIPLookupAPITokenPlaceholder, m.luaStringLiteral(m.GeoIPLookupAPIToken))
		files[index].Content = strings.ReplaceAll(files[index].Content, GeoIPLookupAPITimeoutPlaceholder, fmt.Sprintf("%d", m.geoIPLookupAPITimeoutMilliseconds()))
	}
	return files
}

func (m *Manager) managedWAFLuaFiles() []protocol.SupportFile {
	files := ManagedWAFLuaFiles()
	runtimeConfigDir := filepath.ToSlash(strings.TrimSpace(m.RuntimeConfigDir))
	for index := range files {
		files[index].Content = strings.ReplaceAll(files[index].Content, RuntimeConfigDirPlaceholder, runtimeConfigDir)
	}
	return files
}

func (m *Manager) managedCCLuaFiles() []protocol.SupportFile {
	files := ManagedCCLuaFiles()
	runtimeConfigDir := filepath.ToSlash(strings.TrimSpace(m.RuntimeConfigDir))
	for index := range files {
		files[index].Content = strings.ReplaceAll(files[index].Content, RuntimeConfigDirPlaceholder, runtimeConfigDir)
	}
	return files
}

func ObservabilityListenAddress(openrestyPath string, port int) string {
	if port <= 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func ResolverDirective(openrestyPath string, explicitResolvers []string) string {
	resolvers := resolverAddresses(openrestyPath, explicitResolvers)
	if len(resolvers) == 0 {
		return ""
	}
	return fmt.Sprintf("    resolver %s valid=30s ipv6=off;\n    resolver_timeout 5s;\n", strings.Join(resolvers, " "))
}

func resolverAddresses(openrestyPath string, explicitResolvers []string) []string {
	if resolvers := normalizeResolverAddresses(explicitResolvers); len(resolvers) > 0 {
		return resolvers
	}
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	return parseResolverAddresses(string(data), false)
}

func parseResolverAddresses(content string, dockerMode bool) []string {
	lines := strings.Split(content, "\n")
	resolvers := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr := strings.TrimSpace(fields[1])
		if addr == "" {
			continue
		}
		if dockerMode && !isUsableDockerResolver(addr) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		resolvers = append(resolvers, addr)
	}
	return resolvers
}

func isUsableDockerResolver(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsUnspecified()
}

func normalizeResolverAddresses(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	resolvers := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		addr := strings.TrimSpace(value)
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		resolvers = append(resolvers, addr)
	}
	if len(resolvers) == 0 {
		return nil
	}
	return resolvers
}

func RequiresRuntimeResolver(originURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(originURL))
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	return net.ParseIP(parsed.Hostname()) == nil
}

func (m *Manager) routeConfigIncludePath() string {
	return strings.TrimSpace(m.RouteConfigPath)
}

func (m *Manager) accessLogRuntimePath() string {
	return filepath.ToSlash(strings.TrimSpace(m.AccessLogPath))
}

func (m *Manager) luaRuntimePath() string {
	if strings.TrimSpace(m.NginxLuaDir) == "" {
		return ""
	}
	return filepath.ToSlash(m.NginxLuaDir)
}

func (m *Manager) geoIPDatabaseRuntimePath() string {
	if path := strings.TrimSpace(m.NginxGeoIPDatabasePath); path != "" {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(strings.TrimSpace(m.GeoIPDatabasePath))
}

func (m *Manager) geoIPLookupAPITimeoutMilliseconds() int {
	if m.GeoIPLookupAPITimeout <= 0 {
		return 250
	}
	return int(m.GeoIPLookupAPITimeout / time.Millisecond)
}

func (m *Manager) luaStringLiteral(value string) string {
	escaped := strings.ReplaceAll(strings.TrimSpace(value), `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func BundleChecksum(mainConfig string, routeConfig string, supportFiles []protocol.SupportFile) string {
	files := append([]protocol.SupportFile(nil), supportFiles...)
	sort.Slice(files, func(i int, j int) bool {
		return files[i].Path < files[j].Path
	})
	var builder strings.Builder
	builder.WriteString(mainConfig)
	builder.WriteString("\n--route-config--\n")
	builder.WriteString(routeConfig)
	builder.WriteString("\n--support-files--\n")
	for _, file := range files {
		builder.WriteString(file.Path)
		builder.WriteString("\n")
		builder.WriteString(file.Content)
		builder.WriteString("\n")
	}
	return checksum(builder.String())
}

func bundleChecksum(mainConfig string, routeConfig string, supportFiles []protocol.SupportFile) string {
	return BundleChecksum(mainConfig, routeConfig, supportFiles)
}
