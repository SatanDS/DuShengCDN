package dnsworker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Runner struct {
	Config         *Config
	Client         *APIClient
	Store          *SnapshotStore
	DNSServer      *DNSServer
	Rollups        *RollupAggregator
	SourceResolver *SourceResolver
}

func NewRunner(cfg *Config) (*Runner, error) {
	store := NewSnapshotStore(cfg.SnapshotPath, cfg.SnapshotMaxAge)
	if err := store.LoadFromDisk(); err != nil {
		slog.Warn("load dns worker snapshot cache failed", "error", err)
	}
	scheduler := NewScheduler()
	if snapshot, _, _, _ := store.Current(); snapshot != nil {
		scheduler.LoadSnapshotStates(snapshot)
	}
	sourceResolver, err := NewSourceResolver(cfg.GeoIPDatabasePath, cfg.ASNDatabasePath, cfg.OperatorCIDRDatabasePath)
	if err != nil {
		slog.Warn(
			"open dns worker source database failed",
			"geoip_path", cfg.GeoIPDatabasePath,
			"asn_path", cfg.ASNDatabasePath,
			"operator_cidr_path", cfg.OperatorCIDRDatabasePath,
			"error", err,
		)
	}
	if sourceResolver != nil {
		WithECS(cfg.ECSEnabled, cfg.ECSIPv4Prefix, cfg.ECSIPv6Prefix)(sourceResolver)
	}
	rollups := NewRollupAggregator(time.Minute)
	client := NewAPIClient(cfg.ServerURL, cfg.Token, cfg.RequestTimeout)
	server := NewDNSServerWithProtection(store, scheduler, rollups, sourceResolver, cfg.ListenAddr, cfg.QueryRateLimit, cfg.ResponseRateLimit, cfg.UDPResponseSize)
	return &Runner{
		Config:         cfg,
		Client:         client,
		Store:          store,
		DNSServer:      server,
		Rollups:        rollups,
		SourceResolver: sourceResolver,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if err := r.pullSnapshot(ctx); err != nil {
		r.Store.SetLastError(err)
		slog.Warn("initial dns snapshot pull failed", "error", err)
	}
	if err := r.sendHeartbeat(ctx); err != nil {
		r.Store.SetLastError(err)
		slog.Warn("initial dns worker heartbeat failed", "error", err)
	}
	go r.syncLoop(ctx)
	return r.DNSServer.Run(ctx)
}

func (r *Runner) syncLoop(ctx context.Context) {
	interval := r.Config.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.pullSnapshot(ctx); err != nil {
				r.Store.SetLastError(err)
				slog.Warn("pull dns snapshot failed", "error", err)
			}
			if err := r.sendHeartbeat(ctx); err != nil {
				r.Store.SetLastError(err)
				slog.Warn("send dns worker heartbeat failed", "error", err)
			}
		}
	}
}

func (r *Runner) pullSnapshot(ctx context.Context) error {
	snapshot, err := r.Client.FetchSnapshot(ctx)
	if err != nil {
		return err
	}
	if err := r.Store.Set(snapshot); err != nil {
		return err
	}
	if r.DNSServer != nil && r.DNSServer.Scheduler != nil {
		if loaded, _, _, _ := r.Store.Current(); loaded != nil {
			r.DNSServer.Scheduler.LoadSnapshotStates(loaded)
		}
	}
	if err := r.Store.Save(snapshot); err != nil {
		return err
	}
	slog.Info("dns snapshot loaded", "version", snapshot.SnapshotVersion, "zones", len(snapshot.Zones), "routes", len(snapshot.Routes), "nodes", len(snapshot.Nodes))
	return nil
}

func (r *Runner) sendHeartbeat(ctx context.Context) error {
	status := WorkerStatusOffline
	if r.Store.Version() != "" {
		status = WorkerStatusOnline
	}
	rollups := r.Rollups.Drain()
	var schedulingStates []SnapshotSchedulingState
	if r.DNSServer != nil && r.DNSServer.Scheduler != nil {
		if snapshot, _, _, _ := r.Store.Current(); snapshot != nil {
			schedulingStates = r.DNSServer.Scheduler.SnapshotStates(snapshot)
		}
	}
	sourceStatus := SourceResolverStatus{}
	if r.SourceResolver != nil {
		sourceStatus = r.SourceResolver.Status()
	}
	updateResult := r.loadPendingUpdateResult()
	response, err := r.Client.SendHeartbeat(ctx, HeartbeatInput{
		Version:                  r.Config.Version,
		Status:                   status,
		LastSnapshotVersion:      r.Store.Version(),
		LastSnapshotAt:           r.Store.LoadedAt(),
		LastError:                r.Store.LastError(),
		GeoIPEnabled:             sourceStatus.Enabled,
		GeoIPDatabasePath:        sourceStatus.DatabasePath,
		ASNDatabasePath:          sourceStatus.ASNDatabasePath,
		GeoIPLastError:           sourceStatus.LastError,
		ASNLastError:             sourceStatus.ASNLastError,
		GeoIPDatabaseType:        sourceStatus.DatabaseType,
		ASNDatabaseType:          sourceStatus.ASNDatabaseType,
		GeoIPCountryEnabled:      sourceStatus.CountryEnabled,
		GeoIPASNEnabled:          sourceStatus.ASNEnabled,
		GeoIPOperatorEnabled:     sourceStatus.OperatorEnabled,
		OperatorCIDRDatabasePath: sourceStatus.OperatorCIDRDatabasePath,
		OperatorCIDRLastError:    sourceStatus.OperatorCIDRLastError,
		UpdateSupported:          r.Config.UpdateEnabled,
		UninstallSupported:       r.uninstallSupported(),
		UpdateResult:             updateResult,
		Rollups:                  rollups,
		SchedulingStates:         schedulingStates,
	})
	if err != nil {
		r.Rollups.Restore(rollups)
		slog.Warn(
			"dns worker heartbeat failed",
			"snapshot_version", r.Store.Version(),
			"rollups", len(rollups),
			"error", err,
		)
		return err
	}
	if updateResult != nil {
		r.clearPendingUpdateResult()
	}
	if response != nil {
		r.maybeStartUpdate(response.Settings)
		r.maybeStartUninstall(response.Settings)
	}
	slog.Info(
		"dns worker heartbeat sent",
		"status", status,
		"snapshot_version", r.Store.Version(),
		"rollups", len(rollups),
	)
	return nil
}

func (r *Runner) uninstallSupported() bool {
	if r == nil || r.Config == nil {
		return false
	}
	installDir := strings.TrimSpace(r.Config.InstallDir)
	if installDir == "" {
		return false
	}
	script := filepath.Join(filepath.Clean(installDir), "uninstall-dns-worker.sh")
	info, err := os.Stat(script)
	return err == nil && !info.IsDir()
}
