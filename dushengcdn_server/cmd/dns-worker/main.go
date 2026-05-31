package main

import (
	"context"
	"dushengcdn/internal/dnsworker"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var version = "dev"

func main() {
	setupLog()
	cfg, err := dnsworker.LoadConfig(os.Args[1:], version)
	if err != nil {
		slog.Error("load dns worker config failed", "error", err)
		os.Exit(1)
	}
	runner, err := dnsworker.NewRunner(cfg)
	if err != nil {
		slog.Error("initialize dns worker failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	slog.Info("dns worker process started",
		"version", cfg.Version,
		"server_url", cfg.ServerURL,
		"listen", cfg.ListenAddr,
		"snapshot_path", cfg.SnapshotPath,
	)
	if err := runner.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("dns worker exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("dns worker process stopped")
}

func setupLog() {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
