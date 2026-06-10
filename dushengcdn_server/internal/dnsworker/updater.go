package dnsworker

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
	"strings"
	"sync/atomic"
	"time"
)

var dnsWorkerUpdateRunning atomic.Bool
var dnsWorkerUninstallRunning atomic.Bool

func (r *Runner) maybeStartUpdate(settings WorkerSettings) {
	if r == nil || r.Config == nil || !settings.UpdateNow {
		return
	}
	if !r.Config.UpdateEnabled {
		slog.Warn("dns worker update requested but controlled self-update is disabled")
		return
	}
	if !dnsWorkerUpdateRunning.CompareAndSwap(false, true) {
		slog.Info("dns worker update requested but an update is already running")
		return
	}
	go func() {
		defer dnsWorkerUpdateRunning.Store(false)
		if err := r.runUpdate(settings); err != nil {
			slog.Error("dns worker update failed", "error", err)
			r.savePendingUpdateResult(settings, false, err.Error())
			return
		}
		r.savePendingUpdateResult(settings, true, "DNS Worker update command completed")
	}()
}

func (r *Runner) maybeStartUninstall(settings WorkerSettings) {
	if r == nil || r.Config == nil || !settings.UninstallNow {
		return
	}
	if !dnsWorkerUninstallRunning.CompareAndSwap(false, true) {
		slog.Info("dns worker uninstall requested but an uninstall is already running")
		return
	}
	go func() {
		defer dnsWorkerUninstallRunning.Store(false)
		if err := r.runUninstall(); err != nil {
			slog.Error("dns worker uninstall failed", "error", err)
		}
	}()
}

func (r *Runner) runUpdate(settings WorkerSettings) error {
	script := strings.TrimSpace(r.Config.UpdateScriptPath)
	if script == "" {
		return errors.New("dns worker update script is not configured")
	}
	cleanScript := filepath.Clean(script)
	if !filepath.IsAbs(cleanScript) {
		return errors.New("dns worker update script must be an absolute path")
	}
	info, err := os.Stat(cleanScript)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("dns worker update script path is a directory")
	}
	if err := r.validateInstallScript(cleanScript, info, "dns worker update script"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, cleanScript)
	} else {
		cmd = exec.CommandContext(ctx, "bash", cleanScript)
	}
	cmd.Env = append(os.Environ(),
		"DUSHENGCDN_DNS_WORKER_UPDATE_CHANNEL="+normalizeWorkerUpdateChannel(settings.UpdateChannel),
		"DUSHENGCDN_RELEASE_REPO="+strings.TrimSpace(settings.UpdateRepo),
		"DUSHENGCDN_DNS_WORKER_UPDATE_TAG="+strings.TrimSpace(settings.UpdateTag),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Info(
		"dns worker update started",
		"script", cleanScript,
		"repo", strings.TrimSpace(settings.UpdateRepo),
		"channel", normalizeWorkerUpdateChannel(settings.UpdateChannel),
		"tag", strings.TrimSpace(settings.UpdateTag),
	)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("dns worker update timed out")
		}
		return err
	}
	slog.Info("dns worker update command completed")
	return nil
}

func (r *Runner) validateInstallScript(cleanScript string, info os.FileInfo, label string) error {
	if r == nil || r.Config == nil {
		return errors.New("dns worker config is not available")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "dns worker script"
	}
	installDir := strings.TrimSpace(r.Config.InstallDir)
	if installDir == "" {
		return errors.New("dns worker install directory is not configured")
	}
	cleanInstallDir := filepath.Clean(installDir)
	if !filepath.IsAbs(cleanInstallDir) {
		return errors.New("dns worker install directory must be an absolute path")
	}
	resolvedInstallDir, err := filepath.EvalSymlinks(cleanInstallDir)
	if err == nil {
		cleanInstallDir = filepath.Clean(resolvedInstallDir)
	}
	resolvedScript, err := filepath.EvalSymlinks(cleanScript)
	if err == nil {
		cleanScript = filepath.Clean(resolvedScript)
	}
	relative, err := filepath.Rel(cleanInstallDir, cleanScript)
	if err != nil {
		return err
	}
	if relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("%s must be inside the install directory", label)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must not be writable by group or others", label)
	}
	return nil
}

func (r *Runner) runUninstall() error {
	installDir := strings.TrimSpace(r.Config.InstallDir)
	if installDir == "" {
		return errors.New("dns worker install directory is not configured")
	}
	cleanInstallDir := filepath.Clean(installDir)
	if !filepath.IsAbs(cleanInstallDir) {
		return errors.New("dns worker install directory must be an absolute path")
	}
	script := filepath.Join(cleanInstallDir, "uninstall-dns-worker.sh")
	info, err := os.Stat(script)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("dns worker uninstall script path is a directory")
	}
	if err := r.validateInstallScript(script, info, "dns worker uninstall script"); err != nil {
		return err
	}

	serviceName := strings.TrimSpace(os.Getenv("DUSHENGCDN_DNS_WORKER_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "dushengcdn-dns-worker"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, script)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", script, "--install-dir", cleanInstallDir, "--service-name", serviceName, "--self-uninstall")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	slog.Info("dns worker uninstall started", "script", script, "install_dir", cleanInstallDir, "service", serviceName)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("dns worker uninstall timed out")
		}
		return err
	}
	slog.Info("dns worker uninstall command completed")
	return nil
}

func normalizeWorkerUpdateChannel(channel string) string {
	if strings.EqualFold(strings.TrimSpace(channel), "preview") {
		return "preview"
	}
	return "stable"
}

func (r *Runner) updateResultPath() string {
	if r == nil || r.Config == nil {
		return ""
	}
	installDir := strings.TrimSpace(r.Config.InstallDir)
	if installDir == "" {
		return ""
	}
	return filepath.Join(filepath.Clean(installDir), "data", "update-result.json")
}

func (r *Runner) savePendingUpdateResult(settings WorkerSettings, success bool, message string) {
	path := r.updateResultPath()
	if path == "" {
		return
	}
	result := UpdateResultPayload{
		Success:        success,
		Message:        strings.TrimSpace(message),
		Repo:           strings.TrimSpace(settings.UpdateRepo),
		Channel:        normalizeWorkerUpdateChannel(settings.UpdateChannel),
		TagName:        strings.TrimSpace(settings.UpdateTag),
		ReportedAtUnix: time.Now().Unix(),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		slog.Warn("marshal dns worker update result failed", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		slog.Warn("create dns worker update result directory failed", "path", path, "error", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		slog.Warn("write dns worker update result failed", "path", path, "error", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("commit dns worker update result failed", "path", path, "error", err)
	}
}

func (r *Runner) loadPendingUpdateResult() *UpdateResultPayload {
	path := r.updateResultPath()
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var result UpdateResultPayload
	if err := json.Unmarshal(raw, &result); err != nil {
		slog.Warn("decode dns worker update result failed", "path", path, "error", err)
		return nil
	}
	return &result
}

func (r *Runner) clearPendingUpdateResult() {
	path := r.updateResultPath()
	if path != "" {
		_ = os.Remove(path)
	}
}
