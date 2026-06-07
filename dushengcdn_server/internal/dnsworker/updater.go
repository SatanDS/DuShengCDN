package dnsworker

import (
	"context"
	"errors"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, cleanScript)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", cleanScript)
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

func normalizeWorkerUpdateChannel(channel string) string {
	if strings.EqualFold(strings.TrimSpace(channel), "preview") {
		return "preview"
	}
	return "stable"
}
