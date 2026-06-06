package job

import (
	"context"
	"dushengcdn/service"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var dnsSourceDatabaseMirrorJobRunning atomic.Bool

type DNSSourceDatabaseMirrorJob struct{}

func (j *DNSSourceDatabaseMirrorJob) Run() {
	if !dnsSourceDatabaseMirrorJobRunning.CompareAndSwap(false, true) {
		slog.Info("skip DNS source database mirror refresh because previous run is still active")
		return
	}
	defer dnsSourceDatabaseMirrorJobRunning.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := service.RefreshDNSSourceDatabaseMirror(ctx); err != nil {
		slog.Warn("DNS source database mirror refresh failed", "error", err)
		return
	}
	slog.Info("DNS source database mirror refresh completed")
}

func WarmupDNSSourceDatabaseMirror() {
	path := filepath.Join(service.DNSSourceDatabaseMirrorRoot(), "current", "manifest.json")
	if _, err := os.Stat(path); err == nil {
		return
	}
	go (&DNSSourceDatabaseMirrorJob{}).Run()
}
