package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dushengcdn-agent/internal/config"
	"dushengcdn-agent/internal/dnsprobe"
	"dushengcdn-agent/internal/observability"
	"dushengcdn-agent/internal/protocol"
	"dushengcdn-agent/internal/state"
)

type HeartbeatService interface {
	Register(ctx context.Context, payload protocol.NodePayload) (*protocol.RegisterNodeResponse, error)
	Heartbeat(ctx context.Context, payload protocol.NodePayload) (*protocol.HeartbeatResult, error)
	SetToken(token string)
}

type SyncService interface {
	SyncOnStartup(ctx context.Context, target *protocol.ActiveConfigMeta) error
	SyncOnce(ctx context.Context, target *protocol.ActiveConfigMeta) error
	ForceSyncOnce(ctx context.Context, target *protocol.ActiveConfigMeta) error
}

type Updater interface {
	CheckAndUpdate(ctx context.Context, repo string, options UpdateOptions) error
}

type RuntimeManager interface {
	CheckHealth(ctx context.Context) error
	Restart(ctx context.Context) error
	PurgeCache(ctx context.Context, operation protocol.CacheOperation) error
	WarmCache(ctx context.Context, operation protocol.CacheOperation) error
}

type WebSocketService interface {
	Connect(ctx context.Context) (protocol.WebSocketConnection, error)
	SetToken(token string)
	URL() string
}

type UpdateOptions struct {
	Channel string
	TagName string
	Force   bool
}

const autoUpdateCheckInterval = 6 * time.Hour

var runSelfUninstallFunc = runSelfUninstall
var runDNSWorkerUpdateFunc = runDNSWorkerUpdate
var selfUninstallDelay = 2 * time.Second
var probeDNSTargetsFunc = dnsprobe.ProbeTargets
var buildProfileFunc = observability.BuildProfile

var dnsProbeRefreshInterval = time.Minute
var systemProfileRefreshInterval = 5 * time.Minute

type Runner struct {
	Config              *config.Config
	StateStore          *state.Store
	ObservabilityBuffer *state.ObservabilityBufferStore
	HeartbeatService    HeartbeatService
	SyncService         SyncService
	Updater             Updater
	RuntimeManager      RuntimeManager
	WebSocketService    WebSocketService

	autoUpdate              bool
	updateNow               bool
	updateRepo              string
	updateChan              string
	updateTag               string
	lastAutoUpdateCheck     time.Time
	uninstallRequested      bool
	restartOpenrestyNow     bool
	websocketUpgradeEnabled bool

	dnsProbeMu         sync.Mutex
	dnsProbeTargets    []protocol.DNSProbeTarget
	dnsProbeResults    []protocol.DNSProbeReport
	dnsProbeInFlight   bool
	dnsProbeGeneration uint64
	dnsProbeNextRun    time.Time

	profileMu              sync.Mutex
	nextSystemProfileCheck time.Time

	dnsWorkerUpdateMu      sync.Mutex
	dnsWorkerUpdateResults []protocol.DNSWorkerUpdateResult
}

func (r *Runner) Run(ctx context.Context) error {
	nodeID, err := r.StateStore.EnsureNodeID()
	if err != nil {
		return err
	}
	slog.Info("agent runner started", "node_id", nodeID, "node", r.Config.NodeName, "ip", r.Config.NodeIP)
	if r.hasAgentToken() {
		if _, hbErr := r.performHeartbeatCycle(ctx, nodeID, true); hbErr != nil {
			slog.Error("agent startup heartbeat failed", "error", hbErr)
		}
	} else if err = r.tryRegister(ctx, &nodeID); err != nil {
		slog.Error("agent initial discovery register failed", "error", err)
	}

	heartbeatTicker := time.NewTicker(r.Config.HeartbeatInterval.Duration())
	defer heartbeatTicker.Stop()
	var wsDone <-chan error
	wsBackoff := newWebSocketBackoff()
	nextWSAttempt := time.Now()
	tryStartWebSocket := func() {
		if wsDone != nil || !r.shouldUseWebSocket() || time.Now().Before(nextWSAttempt) {
			return
		}
		done, startErr := r.startWebSocket(ctx, nodeID)
		if startErr != nil {
			delay := wsBackoff.Next()
			nextWSAttempt = time.Now().Add(delay)
			slog.Debug("agent ws upgrade failed; falling back to http heartbeat",
				"enabled", r.websocketUpgradeEnabled,
				"url", r.websocketURL(),
				"retry_after", delay,
				"error", startErr,
			)
			return
		}
		wsBackoff.Reset()
		wsDone = done
		slog.Debug("agent switched to websocket mode", "url", r.websocketURL())
	}
	tryStartWebSocket()

	for {
		select {
		case <-ctx.Done():
			slog.Info("agent runner shutting down", "error", ctx.Err())
			return ctx.Err()
		case wsErr := <-wsDone:
			wsDone = nil
			delay := wsBackoff.Next()
			nextWSAttempt = time.Now().Add(delay)
			slog.Debug("agent ws disconnected; resuming http heartbeat", "retry_after", delay, "error", wsErr)
			if r.hasAgentToken() {
				if _, hbErr := r.performHeartbeatCycle(ctx, nodeID, false); hbErr != nil {
					slog.Error("agent heartbeat after ws disconnect failed", "error", hbErr)
				}
			}
		case <-heartbeatTicker.C:
			if wsDone != nil {
				continue
			}
			if !r.hasAgentToken() {
				if err = r.tryRegister(ctx, &nodeID); err != nil {
					slog.Error("agent discovery register failed", "error", err)
				}
				continue
			}
			if changed, hbErr := r.performHeartbeatCycle(ctx, nodeID, false); hbErr != nil {
				slog.Error("agent heartbeat failed", "error", hbErr)
			} else {
				if changed {
					heartbeatTicker.Reset(r.Config.HeartbeatInterval.Duration())
				}
				tryStartWebSocket()
			}
		}
	}
}

func (r *Runner) performHeartbeatCycle(ctx context.Context, nodeID string, startup bool) (bool, error) {
	r.refreshOpenrestyHealth(ctx)
	payload, ackWindows := r.prepareHeartbeatPayload(ctx, nodeID)
	heartbeatResult, err := r.HeartbeatService.Heartbeat(ctx, payload)
	if err != nil {
		r.restoreDNSWorkerUpdateResults(payload.DNSWorkerUpdateResults)
		return false, err
	}
	r.ackObservabilityWindows(ackWindows)
	if heartbeatResult == nil {
		heartbeatResult = &protocol.HeartbeatResult{}
	}
	mode := "periodic"
	if startup {
		mode = "startup"
	}
	slog.Debug("agent heartbeat succeeded", "mode", mode, "node_id", nodeID)
	changed := r.applySettings(heartbeatResult.AgentSettings)
	if startup {
		if err = r.SyncService.SyncOnStartup(ctx, heartbeatResult.ActiveConfig); err != nil {
			r.recordSyncError(err)
			slog.Error("agent startup sync failed", "error", err)
		} else {
			slog.Debug("agent startup sync completed")
		}
	} else if err = r.SyncService.SyncOnce(ctx, heartbeatResult.ActiveConfig); err != nil {
		r.recordSyncError(err)
		slog.Error("agent sync failed", "error", err)
	}
	r.handleDNSWorkerUpdates(ctx, heartbeatResult.DNSWorkerUpdates)
	r.tryRestartOpenresty(ctx)
	r.tryAutoUpdate(ctx)
	return changed, nil
}

func (r *Runner) shouldUseWebSocket() bool {
	enabled := r.WebSocketService != nil && r.websocketUpgradeEnabled && r.hasAgentToken()
	slog.Debug("agent ws upgrade eligibility checked", "enabled", enabled, "server_enabled", r.websocketUpgradeEnabled, "url", r.websocketURL())
	return enabled
}

func (r *Runner) websocketURL() string {
	if r.WebSocketService == nil {
		return ""
	}
	return r.WebSocketService.URL()
}

func (r *Runner) startWebSocket(ctx context.Context, nodeID string) (<-chan error, error) {
	if r.WebSocketService == nil {
		return nil, errors.New("websocket service is not configured")
	}
	conn, err := r.WebSocketService.Connect(ctx)
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			_ = conn.Close()
		}()
		done <- r.runWebSocket(ctx, nodeID, conn)
	}()
	return done, nil
}

func (r *Runner) runWebSocket(ctx context.Context, nodeID string, conn protocol.WebSocketConnection) error {
	slog.Debug("agent ws connected", "url", conn.URL(), "node_id", nodeID)
	statusTicker := time.NewTicker(r.Config.HeartbeatInterval.Duration())
	defer statusTicker.Stop()
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()

	messages := make(chan protocol.WSMessage, 8)
	readDone := make(chan error, 1)
	go func() {
		for {
			message, err := conn.Receive()
			if err != nil {
				select {
				case readDone <- err:
				default:
				}
				return
			}
			select {
			case messages <- message:
			case <-readCtx.Done():
				select {
				case readDone <- readCtx.Err():
				default:
				}
				return
			}
		}
	}()

	if err := r.sendWebSocketStatus(ctx, nodeID, conn); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readDone:
			return err
		case <-statusTicker.C:
			if err := r.sendWebSocketStatus(ctx, nodeID, conn); err != nil {
				return err
			}
		case message := <-messages:
			changed, err := r.handleWebSocketMessage(ctx, message, conn)
			if err != nil {
				return err
			}
			if changed {
				statusTicker.Reset(r.Config.HeartbeatInterval.Duration())
			}
		}
	}
}

func (r *Runner) sendWebSocketStatus(ctx context.Context, nodeID string, conn protocol.WebSocketConnection) error {
	r.refreshOpenrestyHealth(ctx)
	payload, _ := r.prepareHeartbeatPayload(ctx, nodeID)
	if err := conn.SendStatus(payload); err != nil {
		r.restoreDNSWorkerUpdateResults(payload.DNSWorkerUpdateResults)
		return err
	}
	return nil
}

func (r *Runner) handleWebSocketMessage(ctx context.Context, message protocol.WSMessage, conn protocol.WebSocketConnection) (bool, error) {
	switch message.Type {
	case protocol.WSMessageTypeStatusAck:
		var ack protocol.ObservabilityAck
		if err := json.Unmarshal(message.Payload, &ack); err != nil {
			slog.Debug("agent ws status ack decode failed", "error", err)
			return false, nil
		}
		r.ackObservabilityWindows(ack.WindowStartedAtUnix)
		return false, nil
	case protocol.WSMessageTypeSettings:
		var settings protocol.AgentSettings
		if err := json.Unmarshal(message.Payload, &settings); err != nil {
			slog.Debug("agent ws settings decode failed", "error", err)
			return false, nil
		}
		changed := r.applySettings(&settings)
		r.tryRestartOpenresty(ctx)
		r.tryAutoUpdate(ctx)
		if !r.websocketUpgradeEnabled {
			slog.Debug("agent ws disabled by server settings; falling back to http heartbeat")
			return changed, errors.New("websocket upgrade disabled by server")
		}
		return changed, nil
	case protocol.WSMessageTypeActiveConfig:
		var target protocol.ActiveConfigMeta
		if err := json.Unmarshal(message.Payload, &target); err != nil {
			slog.Debug("agent ws active config decode failed", "error", err)
			return false, nil
		}
		slog.Debug("agent ws active config received", "version", target.Version, "checksum", target.Checksum, "trigger_sync", true)
		if err := r.SyncService.SyncOnce(ctx, &target); err != nil {
			r.recordSyncError(err)
			slog.Error("agent ws triggered sync failed", "version", target.Version, "error", err)
		}
		return false, nil
	case protocol.WSMessageTypeForceSyncConfig:
		var target protocol.ActiveConfigMeta
		if err := json.Unmarshal(message.Payload, &target); err != nil {
			slog.Debug("agent ws force sync config decode failed", "error", err)
			return false, nil
		}
		slog.Debug("agent ws force sync config received", "version", target.Version, "checksum", target.Checksum, "trigger_sync", true)
		if err := r.SyncService.ForceSyncOnce(ctx, &target); err != nil {
			r.recordSyncError(err)
			slog.Error("agent ws triggered force sync failed", "version", target.Version, "error", err)
		}
		return false, nil
	case protocol.WSMessageTypeUninstallAgent:
		r.requestSelfUninstall()
		return false, errors.New("agent uninstall requested by server")
	case protocol.WSMessageTypeDNSWorkerUpdate:
		var request protocol.DNSWorkerUpdateRequest
		if err := json.Unmarshal(message.Payload, &request); err != nil {
			slog.Debug("agent ws dns worker update decode failed", "error", err)
			return false, nil
		}
		r.handleDNSWorkerUpdate(ctx, request)
		return false, nil
	case protocol.WSMessageTypeCacheOperation:
		var operation protocol.CacheOperation
		if err := json.Unmarshal(message.Payload, &operation); err != nil {
			slog.Debug("agent ws cache operation decode failed", "error", err)
			return false, nil
		}
		r.handleCacheOperation(ctx, operation)
		return false, nil
	case protocol.WSMessageTypePing:
		slog.Debug("agent ws ping received")
		return false, conn.SendPong()
	case protocol.WSMessageTypePong:
		slog.Debug("agent ws pong received")
		return false, nil
	default:
		slog.Debug("agent ws unsupported message type", "type", message.Type)
		return false, nil
	}
}

func (r *Runner) handleCacheOperation(ctx context.Context, operation protocol.CacheOperation) {
	if r.RuntimeManager == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(operation.Action)) {
	case "purge":
		if err := r.RuntimeManager.PurgeCache(ctx, operation); err != nil {
			slog.Error("agent cache purge failed", "operation_id", operation.OperationID, "error", err)
			return
		}
		slog.Info("agent cache purge completed", "operation_id", operation.OperationID, "scope", operation.Scope)
	case "warm":
		if err := r.RuntimeManager.WarmCache(ctx, operation); err != nil {
			slog.Error("agent cache warm failed", "operation_id", operation.OperationID, "error", err)
			return
		}
		slog.Info("agent cache warm completed", "operation_id", operation.OperationID, "urls", len(operation.URLs))
	default:
		slog.Warn("agent cache operation action is unsupported", "operation_id", operation.OperationID, "action", operation.Action)
	}
}

func (r *Runner) handleDNSWorkerUpdate(ctx context.Context, request protocol.DNSWorkerUpdateRequest) {
	if err := runDNSWorkerUpdateFunc(ctx, request); err != nil {
		slog.Error("agent dns worker update failed",
			"worker_id", strings.TrimSpace(request.WorkerID),
			"worker_name", strings.TrimSpace(request.WorkerName),
			"error", err,
		)
		r.recordDNSWorkerUpdateResult(request, false, err.Error())
		return
	}
	slog.Info("agent dns worker update completed",
		"worker_id", strings.TrimSpace(request.WorkerID),
		"worker_name", strings.TrimSpace(request.WorkerName),
	)
	r.recordDNSWorkerUpdateResult(request, true, "DNS Worker installer completed")
}

func (r *Runner) handleDNSWorkerUpdates(ctx context.Context, requests []protocol.DNSWorkerUpdateRequest) {
	for _, request := range requests {
		r.handleDNSWorkerUpdate(ctx, request)
	}
}

func runDNSWorkerUpdate(ctx context.Context, request protocol.DNSWorkerUpdateRequest) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("DNS Worker remote update is only supported on Linux")
	}
	if runningInContainer() {
		return fmt.Errorf("Agent is running in a container; refusing to modify host DNS Worker")
	}
	if !commandExists("bash") {
		return fmt.Errorf("bash is required to run DNS Worker installer")
	}
	installDir := strings.TrimSpace(request.InstallDir)
	if installDir == "" {
		installDir = "/opt/dushengcdn-dns-worker"
	}
	installDir, err := safeDNSWorkerInstallDir(installDir)
	if err != nil {
		return err
	}
	if installInfo, err := os.Stat(installDir); err != nil {
		return fmt.Errorf("DNS Worker install directory is not available at %s: %w", installDir, err)
	} else if err := validateDNSWorkerUpdateFileOwnership(installDir, installInfo, true); err != nil {
		return err
	}
	envFile := filepath.Join(installDir, "dns-worker.env")
	if _, err := os.Stat(envFile); err != nil {
		return fmt.Errorf("DNS Worker env file is not available at %s: %w", envFile, err)
	}
	updateScript := filepath.Join(installDir, "update-dns-worker.sh")
	info, err := os.Stat(updateScript)
	if err != nil {
		return fmt.Errorf("DNS Worker updater script is not available at %s: %w", updateScript, err)
	}
	if info.IsDir() {
		return fmt.Errorf("DNS Worker updater script path is a directory: %s", updateScript)
	}
	if err := validateDNSWorkerUpdateFileOwnership(updateScript, info, true); err != nil {
		return err
	}
	if envInfo, err := os.Stat(envFile); err != nil {
		return fmt.Errorf("DNS Worker env file is not available at %s: %w", envFile, err)
	} else if err := validateDNSWorkerUpdateFileOwnership(envFile, envInfo, true); err != nil {
		return err
	}
	if err := validateDNSWorkerUpdateIdentity(envFile, request.WorkerID); err != nil {
		return err
	}
	repo := strings.TrimSpace(request.Repo)
	if repo == "" {
		repo = "SatanDS/SatanDS-DuShengCDN-releases"
	}
	if !isSafeGitHubRepo(repo) {
		return fmt.Errorf("DNS Worker release repo is invalid: %s", repo)
	}
	channel := strings.ToLower(strings.TrimSpace(request.Channel))
	if channel != "preview" {
		channel = "stable"
	}
	tagName := strings.TrimSpace(request.TagName)
	if tagName != "" && !isSafeReleaseTag(tagName) {
		return fmt.Errorf("DNS Worker release tag is invalid: %s", tagName)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "bash", updateScript)
	cmd.Env = append(os.Environ(),
		"DUSHENGCDN_RELEASE_REPO="+repo,
		"DUSHENGCDN_DNS_WORKER_UPDATE_CHANNEL="+channel,
		"DUSHENGCDN_DNS_WORKER_UPDATE_TAG="+tagName,
	)
	output, err := cmd.CombinedOutput()
	if cmdCtx.Err() != nil {
		return fmt.Errorf("DNS Worker installer timed out")
	}
	if err != nil {
		return fmt.Errorf("DNS Worker installer failed: %w: %s", err, trimCommandOutput(output, 4000))
	}
	return nil
}

func validateDNSWorkerUpdateIdentity(envFile string, expectedWorkerID string) error {
	expectedWorkerID = strings.TrimSpace(expectedWorkerID)
	if expectedWorkerID == "" {
		return fmt.Errorf("DNS Worker update request is missing worker_id")
	}
	localWorkerID, err := readDNSWorkerEnvValue(envFile, "DUSHENGCDN_DNS_WORKER_ID")
	if err != nil {
		return fmt.Errorf("read DNS Worker identity from env file: %w", err)
	}
	if localWorkerID == "" {
		return fmt.Errorf("DNS Worker env file does not declare DUSHENGCDN_DNS_WORKER_ID; rerun install-dns-worker.sh with --worker-id before Agent-mediated updates")
	}
	if localWorkerID != expectedWorkerID {
		return fmt.Errorf("DNS Worker update request worker_id %q does not match local env worker_id %q", expectedWorkerID, localWorkerID)
	}
	return nil
}

func readDNSWorkerEnvValue(path string, key string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return decodeDNSWorkerEnvValue(value), nil
	}
	return "", nil
}

func decodeDNSWorkerEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		value, err := strconv.Unquote(raw)
		if err == nil {
			return value
		}
		return strings.Trim(raw, `"`)
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func validateDNSWorkerUpdateFileOwnership(path string, info os.FileInfo, requireRootOwner bool) error {
	if info == nil {
		return fmt.Errorf("DNS Worker update file is not available: %s", path)
	}
	if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("DNS Worker update file is writable by group/other and is unsafe to execute or source: %s", path)
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	uid, ok := dnsWorkerUpdateFileUID(info)
	if !ok {
		return fmt.Errorf("cannot inspect DNS Worker update file ownership: %s", path)
	}
	if requireRootOwner && uid != 0 {
		return fmt.Errorf("DNS Worker updater script must be owned by root: %s", path)
	}
	return nil
}

func (r *Runner) recordDNSWorkerUpdateResult(request protocol.DNSWorkerUpdateRequest, success bool, message string) {
	if r == nil {
		return
	}
	result := protocol.DNSWorkerUpdateResult{
		WorkerID:       strings.TrimSpace(request.WorkerID),
		WorkerName:     strings.TrimSpace(request.WorkerName),
		Repo:           strings.TrimSpace(request.Repo),
		Channel:        strings.TrimSpace(request.Channel),
		TagName:        strings.TrimSpace(request.TagName),
		InstallDir:     strings.TrimSpace(request.InstallDir),
		Success:        success,
		Message:        trimString(message, 4000),
		ReportedAtUnix: time.Now().Unix(),
	}
	if result.WorkerID == "" && result.WorkerName == "" {
		return
	}
	r.dnsWorkerUpdateMu.Lock()
	defer r.dnsWorkerUpdateMu.Unlock()
	r.dnsWorkerUpdateResults = append(r.dnsWorkerUpdateResults, result)
}

func safeDNSWorkerInstallDir(path string) (string, error) {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		return "", errors.New("DNS Worker install directory cannot be empty")
	}
	if strings.Contains(candidate, "\x00") || strings.ContainsAny(candidate, "\r\n") {
		return "", errors.New("DNS Worker install directory contains invalid characters")
	}
	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("DNS Worker install directory %q must be absolute", path)
	}
	if pathComponentCount(cleaned) < 2 {
		return "", fmt.Errorf("DNS Worker install directory %q is too shallow", path)
	}
	base := strings.ToLower(filepath.Base(cleaned))
	base = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(base)
	if !strings.Contains(base, "dushengcdn") || !strings.Contains(base, "dnsworker") {
		return "", fmt.Errorf("DNS Worker install directory %q does not look like a DuShengCDN DNS Worker directory", path)
	}
	return cleaned, nil
}

func isSafeGitHubRepo(repo string) bool {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if !isSafeGitHubPathPart(part) {
			return false
		}
	}
	return true
}

func isSafeGitHubPathPart(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeReleaseTag(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func trimCommandOutput(output []byte, limit int) string {
	text := strings.TrimSpace(string(output))
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}

func trimString(value string, limit int) string {
	text := strings.TrimSpace(value)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}

type webSocketBackoff struct {
	delays []time.Duration
	index  int
}

func newWebSocketBackoff() *webSocketBackoff {
	return &webSocketBackoff{
		delays: []time.Duration{
			time.Second,
			2 * time.Second,
			5 * time.Second,
			10 * time.Second,
			30 * time.Second,
		},
	}
}

func (backoff *webSocketBackoff) Next() time.Duration {
	if backoff == nil || len(backoff.delays) == 0 {
		return 30 * time.Second
	}
	if backoff.index >= len(backoff.delays) {
		return backoff.delays[len(backoff.delays)-1]
	}
	delay := backoff.delays[backoff.index]
	backoff.index++
	return delay
}

func (backoff *webSocketBackoff) Reset() {
	if backoff != nil {
		backoff.index = 0
	}
}

func (r *Runner) hasAgentToken() bool {
	return strings.TrimSpace(r.Config.AgentToken) != ""
}

func (r *Runner) requestSelfUninstall() {
	if r.uninstallRequested {
		return
	}
	r.uninstallRequested = true
	slog.Warn("agent uninstall requested by server")
	go func() {
		time.Sleep(selfUninstallDelay)
		if err := runSelfUninstallFunc(r.Config); err != nil {
			slog.Error("agent uninstall failed", "error", err)
			return
		}
	}()
}

func runSelfUninstall(cfg *config.Config) error {
	if runningInContainer() {
		slog.Warn("agent appears to run in a container; exiting process, remove the container from the Docker host if needed")
		os.Exit(0)
	}
	installDir, err := safeInstallDirForRemoval(detectInstallDir(cfg))
	if err != nil {
		return fmt.Errorf("unsafe install directory, refusing to uninstall: %w", err)
	}
	if runtime.GOOS == "linux" && commandExists("systemd-run") {
		return startSystemdUninstall(installDir)
	}
	return removeInstallDirAndExit(installDir)
}

func startSystemdUninstall(installDir string) error {
	safeInstallDir, err := safeInstallDirForRemoval(installDir)
	if err != nil {
		return err
	}
	script := strings.Join([]string{
		"sleep 1",
		"systemctl stop dushengcdn-agent || true",
		"systemctl disable dushengcdn-agent || true",
		"rm -f /etc/systemd/system/dushengcdn-agent.service",
		"systemctl daemon-reload || true",
		"systemctl reset-failed dushengcdn-agent || true",
		"rm -rf -- " + shellQuote(safeInstallDir),
	}, "; ")
	return exec.Command(
		"systemd-run",
		"--unit=dushengcdn-agent-uninstall",
		"--collect",
		"/bin/sh",
		"-c",
		script,
	).Start()
}

func detectInstallDir(cfg *config.Config) string {
	if cfg != nil {
		if dataDir := strings.TrimSpace(cfg.DataDir); dataDir != "" {
			if filepath.Base(dataDir) == "data" {
				return filepath.Clean(filepath.Dir(dataDir))
			}
			return filepath.Clean(dataDir)
		}
	}
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(execPath)
}

func removeInstallDirAndExit(installDir string) error {
	safeInstallDir, err := safeInstallDirForRemoval(installDir)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return scheduleWindowsInstallDirRemoval(safeInstallDir)
	}
	if err := os.RemoveAll(safeInstallDir); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func safeInstallDirForRemoval(path string) (string, error) {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		return "", errors.New("install directory cannot be empty")
	}
	if strings.Contains(candidate, "\x00") || strings.ContainsAny(candidate, "\r\n") {
		return "", errors.New("install directory contains invalid characters")
	}

	cleaned := filepath.Clean(candidate)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("install directory %q must be absolute", path)
	}
	if pathComponentCount(cleaned) < 2 {
		return "", fmt.Errorf("install directory %q is too shallow", path)
	}
	if isProtectedInstallDir(cleaned) {
		return "", fmt.Errorf("install directory %q is a protected system path", path)
	}
	if !looksLikeDuShengCDNInstallDir(cleaned) {
		return "", fmt.Errorf("install directory %q does not look like a DuShengCDN agent directory", path)
	}
	return cleaned, nil
}

func pathComponentCount(path string) int {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	remainder = strings.Trim(remainder, `/\`)
	if remainder == "" {
		return 0
	}
	return len(strings.FieldsFunc(remainder, func(r rune) bool {
		return r == '/' || r == '\\'
	}))
}

func isProtectedInstallDir(path string) bool {
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	components := strings.FieldsFunc(strings.Trim(remainder, `/\`), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(components) == 0 {
		return true
	}
	first := strings.ToLower(components[0])
	if len(components) < 3 {
		switch first {
		case "bin", "boot", "dev", "etc", "home", "lib", "lib64", "proc", "root", "run", "sbin", "sys", "tmp", "usr", "var", "windows":
			return true
		}
	}
	return false
}

func looksLikeDuShengCDNInstallDir(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	base = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(base)
	return strings.Contains(base, "dushengcdn")
}

func scheduleWindowsInstallDirRemoval(installDir string) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("dushengcdn-agent-uninstall-%d.cmd", os.Getpid()))
	script := strings.Join([]string{
		"@echo off",
		"ping 127.0.0.1 -n 3 >nul",
		"rmdir /S /Q " + quoteWindowsBatchArg(installDir),
		`del /Q "%~f0" >nul 2>nul`,
		"",
	}, "\r\n")
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/C", "start", "", scriptPath)
	if err := cmd.Start(); err != nil {
		os.Remove(scriptPath)
		return err
	}
	os.Exit(0)
	return nil
}

func quoteWindowsBatchArg(value string) string {
	value = strings.ReplaceAll(value, `%`, `%%`)
	value = strings.ReplaceAll(value, `"`, `""`)
	return `"` + value + `"`
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return strings.TrimSpace(os.Getenv("container")) != ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (r *Runner) applySettings(settings *protocol.AgentSettings) bool {
	if settings == nil {
		return false
	}
	changed := false
	if settings.HeartbeatInterval > 0 {
		newInterval := config.MillisecondDuration(time.Duration(settings.HeartbeatInterval) * time.Millisecond)
		if newInterval != r.Config.HeartbeatInterval {
			slog.Info("agent heartbeat interval updated", "from", r.Config.HeartbeatInterval, "to", newInterval)
			r.Config.HeartbeatInterval = newInterval
			changed = true
		}
	}
	if settings.WebsocketUpgradeEnabled != r.websocketUpgradeEnabled {
		slog.Debug("agent websocket upgrade setting updated", "from", r.websocketUpgradeEnabled, "to", settings.WebsocketUpgradeEnabled)
	}
	r.websocketUpgradeEnabled = settings.WebsocketUpgradeEnabled
	r.autoUpdate = settings.AutoUpdate
	r.updateNow = settings.UpdateNow
	r.updateRepo = strings.TrimSpace(settings.UpdateRepo)
	r.updateChan = strings.TrimSpace(settings.UpdateChannel)
	r.updateTag = strings.TrimSpace(settings.UpdateTag)
	r.restartOpenrestyNow = settings.RestartOpenrestyNow
	r.setDNSProbeTargets(settings.DNSProbeTargets)
	return changed
}

func (r *Runner) refreshDNSProbeResults(ctx context.Context) {
	targets, generation, ok := r.beginDNSProbeRefresh()
	if !ok {
		return
	}
	go func() {
		results := probeDNSTargetsFunc(ctx, targets)
		r.finishDNSProbeRefresh(generation, results)
	}()
}

func (r *Runner) setDNSProbeTargets(targets []protocol.DNSProbeTarget) {
	normalized := normalizeDNSProbeTargets(targets)
	r.dnsProbeMu.Lock()
	defer r.dnsProbeMu.Unlock()
	if dnsProbeTargetsEqual(r.dnsProbeTargets, normalized) {
		return
	}
	r.dnsProbeGeneration++
	r.dnsProbeTargets = normalized
	r.dnsProbeResults = nil
	r.dnsProbeInFlight = false
	r.dnsProbeNextRun = time.Time{}
}

func (r *Runner) beginDNSProbeRefresh() ([]protocol.DNSProbeTarget, uint64, bool) {
	r.dnsProbeMu.Lock()
	defer r.dnsProbeMu.Unlock()
	if len(r.dnsProbeTargets) == 0 {
		r.dnsProbeResults = nil
		r.dnsProbeInFlight = false
		r.dnsProbeNextRun = time.Time{}
		return nil, 0, false
	}
	now := time.Now()
	if r.dnsProbeInFlight || (!r.dnsProbeNextRun.IsZero() && now.Before(r.dnsProbeNextRun)) {
		return nil, 0, false
	}
	r.dnsProbeInFlight = true
	r.dnsProbeNextRun = now.Add(dnsProbeRefreshInterval)
	return append([]protocol.DNSProbeTarget(nil), r.dnsProbeTargets...), r.dnsProbeGeneration, true
}

func (r *Runner) finishDNSProbeRefresh(generation uint64, results []protocol.DNSProbeReport) {
	r.dnsProbeMu.Lock()
	defer r.dnsProbeMu.Unlock()
	if generation != r.dnsProbeGeneration {
		return
	}
	r.dnsProbeInFlight = false
	r.dnsProbeResults = append([]protocol.DNSProbeReport(nil), results...)
}

func (r *Runner) consumeDNSProbeResults() []protocol.DNSProbeReport {
	r.dnsProbeMu.Lock()
	defer r.dnsProbeMu.Unlock()
	if len(r.dnsProbeResults) == 0 {
		return nil
	}
	results := append([]protocol.DNSProbeReport(nil), r.dnsProbeResults...)
	r.dnsProbeResults = nil
	return results
}

func (r *Runner) consumeDNSWorkerUpdateResults() []protocol.DNSWorkerUpdateResult {
	r.dnsWorkerUpdateMu.Lock()
	defer r.dnsWorkerUpdateMu.Unlock()
	if len(r.dnsWorkerUpdateResults) == 0 {
		return nil
	}
	results := append([]protocol.DNSWorkerUpdateResult(nil), r.dnsWorkerUpdateResults...)
	r.dnsWorkerUpdateResults = nil
	return results
}

func (r *Runner) restoreDNSWorkerUpdateResults(results []protocol.DNSWorkerUpdateResult) {
	if len(results) == 0 {
		return
	}
	r.dnsWorkerUpdateMu.Lock()
	defer r.dnsWorkerUpdateMu.Unlock()
	r.dnsWorkerUpdateResults = append(results, r.dnsWorkerUpdateResults...)
}

func normalizeDNSProbeTargets(targets []protocol.DNSProbeTarget) []protocol.DNSProbeTarget {
	result := make([]protocol.DNSProbeTarget, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		workerID := strings.TrimSpace(target.WorkerID)
		if workerID == "" {
			continue
		}
		if _, ok := seen[workerID]; ok {
			continue
		}
		seen[workerID] = struct{}{}
		target.WorkerID = workerID
		target.Name = strings.TrimSpace(target.Name)
		target.PublicAddress = strings.TrimSpace(target.PublicAddress)
		target.QueryName = strings.TrimSpace(target.QueryName)
		target.QueryType = strings.TrimSpace(target.QueryType)
		result = append(result, target)
	}
	return result
}

func dnsProbeTargetsEqual(left []protocol.DNSProbeTarget, right []protocol.DNSProbeTarget) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (r *Runner) tryRestartOpenresty(ctx context.Context) {
	if !r.restartOpenrestyNow {
		return
	}
	r.restartOpenrestyNow = false
	if r.RuntimeManager == nil {
		return
	}
	slog.Info("agent openresty restart requested by server")
	if err := r.RuntimeManager.Restart(ctx); err != nil {
		slog.Error("agent openresty restart failed", "error", err)
		r.recordOpenrestyUnhealthy(err, false)
		return
	}
	slog.Info("agent openresty restart succeeded")
	r.recordOpenrestyHealthy()
}

func (r *Runner) tryAutoUpdate(ctx context.Context) {
	force := r.updateNow
	shouldCheck := force || r.shouldCheckAutoUpdate()
	r.updateNow = false
	r.updateTag = strings.TrimSpace(r.updateTag)
	if !shouldCheck || r.Updater == nil || r.updateRepo == "" {
		return
	}
	channel := "stable"
	if force && r.updateChan != "" {
		channel = r.updateChan
	}
	if err := r.Updater.CheckAndUpdate(ctx, r.updateRepo, UpdateOptions{
		Channel: channel,
		TagName: r.updateTag,
		Force:   force,
	}); err != nil {
		slog.Error("agent update check failed", "error", err)
	}
	if !force {
		r.lastAutoUpdateCheck = time.Now()
	}
	if force {
		r.updateTag = ""
		r.updateChan = ""
	}
}

func (r *Runner) shouldCheckAutoUpdate() bool {
	if !r.autoUpdate {
		return false
	}
	if r.lastAutoUpdateCheck.IsZero() {
		return true
	}
	return time.Since(r.lastAutoUpdateCheck) >= autoUpdateCheckInterval
}

func (r *Runner) tryRegister(ctx context.Context, nodeID *string) error {
	if strings.TrimSpace(r.Config.DiscoveryToken) == "" {
		return errors.New("agent_token 为空且未配置 discovery_token")
	}
	slog.Info("agent discovery registration started")
	response, err := r.HeartbeatService.Register(ctx, r.nodePayloadWithContext(ctx, *nodeID))
	if err != nil {
		return err
	}
	if response == nil || strings.TrimSpace(response.AgentToken) == "" || strings.TrimSpace(response.NodeID) == "" {
		return errors.New("discovery register response 缺少 node_id 或 agent_token")
	}
	snapshot, err := r.StateStore.Load()
	if err != nil {
		return err
	}
	snapshot.NodeID = response.NodeID
	if err = r.StateStore.Save(snapshot); err != nil {
		return err
	}
	r.Config.AgentToken = response.AgentToken
	r.Config.DiscoveryToken = ""
	if err = r.Config.Save(); err != nil {
		return err
	}
	r.HeartbeatService.SetToken(response.AgentToken)
	if r.WebSocketService != nil {
		r.WebSocketService.SetToken(response.AgentToken)
	}
	*nodeID = response.NodeID
	slog.Info("agent discovery registration succeeded", "node_id", response.NodeID)
	r.refreshOpenrestyHealth(ctx)
	payload, ackWindows := r.prepareHeartbeatPayload(ctx, *nodeID)
	heartbeatResult, heartbeatErr := r.HeartbeatService.Heartbeat(ctx, payload)
	if heartbeatErr != nil {
		slog.Error("agent post-register heartbeat failed", "error", heartbeatErr)
		return nil
	}
	r.ackObservabilityWindows(ackWindows)
	if heartbeatResult == nil {
		heartbeatResult = &protocol.HeartbeatResult{}
	}
	r.applySettings(heartbeatResult.AgentSettings)
	if err = r.SyncService.SyncOnStartup(ctx, heartbeatResult.ActiveConfig); err != nil {
		r.recordSyncError(err)
		slog.Error("agent post-register startup sync failed", "error", err)
	} else {
		slog.Debug("agent post-register startup sync completed")
	}
	r.tryRestartOpenresty(ctx)
	r.tryAutoUpdate(ctx)
	return nil
}

func (r *Runner) recordSyncError(err error) {
	if err == nil || r.StateStore == nil {
		return
	}
	snapshot, loadErr := r.StateStore.Load()
	if loadErr != nil {
		slog.Error("load state before recording sync error failed", "error", loadErr)
		return
	}
	snapshot.LastError = err.Error()
	slog.Warn("recording sync error into state", "error", snapshot.LastError)
	if saveErr := r.StateStore.Save(snapshot); saveErr != nil {
		slog.Error("save state after sync error failed", "error", saveErr)
	}
}

func (r *Runner) refreshOpenrestyHealth(ctx context.Context) {
	if r.RuntimeManager == nil || r.StateStore == nil {
		return
	}
	if err := r.RuntimeManager.CheckHealth(ctx); err != nil {
		if strings.Contains(err.Error(), "openresty config not exists") {
			return
		}
		r.recordOpenrestyUnhealthy(err, true)
		return
	}
	r.recordOpenrestyHealthy()
}

func (r *Runner) recordOpenrestyHealthy() {
	if r.StateStore == nil {
		return
	}
	snapshot, err := r.StateStore.Load()
	if err != nil {
		slog.Error("load state before recording openresty health failed", "error", err)
		return
	}
	if snapshot.OpenrestyStatus == protocol.OpenrestyStatusHealthy && strings.TrimSpace(snapshot.OpenrestyMessage) == "" {
		return
	}
	snapshot.OpenrestyStatus = protocol.OpenrestyStatusHealthy
	snapshot.OpenrestyMessage = ""
	if err = r.StateStore.Save(snapshot); err != nil {
		slog.Error("save state after recording openresty health failed", "error", err)
	}
}

func (r *Runner) recordOpenrestyUnhealthy(err error, fallbackOnly bool) {
	if err == nil || r.StateStore == nil {
		return
	}
	snapshot, loadErr := r.StateStore.Load()
	if loadErr != nil {
		slog.Error("load state before recording openresty error failed", "error", loadErr)
		return
	}
	message := strings.TrimSpace(err.Error())
	if !fallbackOnly || strings.TrimSpace(snapshot.OpenrestyMessage) == "" {
		snapshot.OpenrestyMessage = message
	}
	snapshot.OpenrestyStatus = protocol.OpenrestyStatusUnhealthy
	if saveErr := r.StateStore.Save(snapshot); saveErr != nil {
		slog.Error("save state after recording openresty error failed", "error", saveErr)
	}
}

func (r *Runner) nodePayload(nodeID string) protocol.NodePayload {
	return r.nodePayloadWithContext(context.Background(), nodeID)
}

func (r *Runner) nodePayloadWithContext(ctx context.Context, nodeID string) protocol.NodePayload {
	r.refreshDNSProbeResults(ctx)
	snapshot, _ := r.StateStore.Load()
	openrestyStatus := strings.TrimSpace(snapshot.OpenrestyStatus)
	if openrestyStatus == "" {
		openrestyStatus = protocol.OpenrestyStatusUnknown
	}
	profile := r.cachedSystemProfile()
	managedOpenRestyMetrics := observability.CollectManagedOpenRestyMetrics(r.Config)
	trafficReport, accessLogs, fallbackMetrics := observability.BuildTrafficObservability(r.Config, r.StateStore, managedOpenRestyMetrics)
	if managedOpenRestyMetrics == nil {
		managedOpenRestyMetrics = fallbackMetrics
	}
	metricSnapshot := observability.BuildSnapshot(r.Config, r.StateStore, managedOpenRestyMetrics)
	healthEvents := observability.BuildHealthEvents(snapshot)
	dnsProbeResults := r.consumeDNSProbeResults()
	dnsWorkerUpdateResults := r.consumeDNSWorkerUpdateResults()
	return protocol.NodePayload{
		NodeID:                 nodeID,
		Name:                   r.Config.NodeName,
		IP:                     r.Config.NodeIP,
		AgentVersion:           r.Config.AgentVersion,
		NginxVersion:           r.Config.NginxVersion,
		CurrentVersion:         snapshot.CurrentVersion,
		CurrentChecksum:        snapshot.CurrentChecksum,
		LastError:              snapshot.LastError,
		OpenrestyStatus:        openrestyStatus,
		OpenrestyMessage:       snapshot.OpenrestyMessage,
		Profile:                profile,
		Snapshot:               metricSnapshot,
		TrafficReport:          trafficReport,
		AccessLogs:             accessLogs,
		HealthEvents:           healthEvents,
		DNSProbeResults:        dnsProbeResults,
		DNSWorkerUpdateResults: dnsWorkerUpdateResults,
	}
}

func (r *Runner) cachedSystemProfile() *protocol.NodeSystemProfile {
	now := time.Now()
	r.profileMu.Lock()
	if systemProfileRefreshInterval > 0 && !r.nextSystemProfileCheck.IsZero() && now.Before(r.nextSystemProfileCheck) {
		r.profileMu.Unlock()
		return nil
	}
	if systemProfileRefreshInterval > 0 {
		r.nextSystemProfileCheck = now.Add(systemProfileRefreshInterval)
	}
	r.profileMu.Unlock()
	return buildProfileFunc(r.Config, r.StateStore)
}

func (r *Runner) prepareHeartbeatPayload(ctx context.Context, nodeID string) (protocol.NodePayload, []int64) {
	payload := r.nodePayloadWithContext(ctx, nodeID)
	if r.ObservabilityBuffer == nil || (payload.Snapshot == nil && payload.TrafficReport == nil && len(payload.AccessLogs) == 0) {
		return payload, nil
	}
	now := time.Now().UTC()
	retainAfterUnix := now.Add(-time.Duration(r.Config.ObservabilityReplayMinutes) * time.Minute).Unix()
	windowStartedAtUnix := state.ObservabilityWindowStartedAt(payload.Snapshot, payload.TrafficReport)
	if windowStartedAtUnix <= 0 {
		return payload, nil
	}

	record := state.ObservabilityBufferRecord{
		WindowStartedAtUnix: windowStartedAtUnix,
		Snapshot:            payload.Snapshot,
		TrafficReport:       payload.TrafficReport,
		AccessLogs:          payload.AccessLogs,
		QueuedAtUnix:        now.Unix(),
	}
	records, err := r.ObservabilityBuffer.UpsertAndReplayable(record, windowStartedAtUnix, retainAfterUnix)
	if err != nil {
		slog.Error("upsert observability buffer and load replayable records failed", "error", err)
		return payload, nil
	}

	ackWindows := make([]int64, 0, len(records)+1)
	buffered := make([]protocol.BufferedObservabilityRecord, 0, len(records))
	for _, item := range records {
		if item.WindowStartedAtUnix <= 0 {
			continue
		}
		buffered = append(buffered, protocol.BufferedObservabilityRecord{
			WindowStartedAtUnix: item.WindowStartedAtUnix,
			Snapshot:            item.Snapshot,
			TrafficReport:       item.TrafficReport,
			AccessLogs:          item.AccessLogs,
		})
		ackWindows = append(ackWindows, item.WindowStartedAtUnix)
	}
	payload.BufferedObservability = buffered
	ackWindows = append(ackWindows, windowStartedAtUnix)
	return payload, ackWindows
}

func (r *Runner) ackObservabilityWindows(windowStartedAtUnix []int64) {
	if r.ObservabilityBuffer == nil || len(windowStartedAtUnix) == 0 {
		return
	}
	retainAfterUnix := time.Now().UTC().Add(-time.Duration(r.Config.ObservabilityReplayMinutes) * time.Minute).Unix()
	if err := r.ObservabilityBuffer.Ack(windowStartedAtUnix, retainAfterUnix); err != nil {
		slog.Error("ack observability buffer failed", "error", err)
	}
}
