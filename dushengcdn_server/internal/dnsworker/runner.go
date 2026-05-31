package dnsworker

import (
	"context"
	"log/slog"
	"time"
)

type Runner struct {
	Config    *Config
	Client    *APIClient
	Store     *SnapshotStore
	DNSServer *DNSServer
	Rollups   *RollupAggregator
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
	sourceResolver, err := NewSourceResolver(cfg.GeoIPDatabasePath)
	if err != nil {
		slog.Warn("open dns worker geoip database failed", "path", cfg.GeoIPDatabasePath, "error", err)
		sourceResolver = &SourceResolver{}
	}
	rollups := NewRollupAggregator(time.Minute)
	client := NewAPIClient(cfg.ServerURL, cfg.Token, cfg.RequestTimeout)
	server := NewDNSServerWithLimits(store, scheduler, rollups, sourceResolver, cfg.ListenAddr, cfg.QueryRateLimit, cfg.UDPResponseSize)
	return &Runner{
		Config:    cfg,
		Client:    client,
		Store:     store,
		DNSServer: server,
		Rollups:   rollups,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if err := r.pullSnapshot(ctx); err != nil {
		slog.Warn("initial dns snapshot pull failed", "error", err)
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
	err := r.Client.SendHeartbeat(ctx, HeartbeatInput{
		Version:             r.Config.Version,
		Status:              status,
		LastSnapshotVersion: r.Store.Version(),
		LastSnapshotAt:      r.Store.LoadedAt(),
		LastError:           r.Store.LastError(),
		Rollups:             rollups,
		SchedulingStates:    schedulingStates,
	})
	if err != nil {
		r.Rollups.Restore(rollups)
	}
	return err
}
