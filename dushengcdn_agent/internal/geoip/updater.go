package geoip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const minDatabaseSize = 1024 * 1024

var maxDatabaseSize int64 = 128 * 1024 * 1024

type Updater struct {
	URL      string
	Path     string
	Interval time.Duration
	Client   *http.Client
}

func (u *Updater) Ensure(ctx context.Context) error {
	if strings.TrimSpace(u.Path) == "" {
		return errors.New("geoip database path is empty")
	}
	if strings.TrimSpace(u.URL) == "" {
		return errors.New("geoip database url is empty")
	}
	if !u.needsUpdate() {
		return nil
	}
	return u.Download(ctx)
}

func (u *Updater) Download(ctx context.Context) error {
	url := strings.TrimSpace(u.URL)
	targetPath := strings.TrimSpace(u.Path)
	if url == "" {
		return errors.New("geoip database url is empty")
	}
	if targetPath == "" {
		return errors.New("geoip database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create geoip database directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".GeoLite2-Country-*.mmdb")
	if err != nil {
		return fmt.Errorf("create geoip temporary database: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("download geoip database: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = tmp.Close()
		return fmt.Errorf("download geoip database returned %s", resp.Status)
	}
	if resp.ContentLength > maxDatabaseSize {
		_ = tmp.Close()
		return fmt.Errorf("downloaded geoip database exceeds maximum size: %d bytes", resp.ContentLength)
	}

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxDatabaseSize+1))
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write geoip database: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close geoip database: %w", err)
	}
	if written < minDatabaseSize {
		return fmt.Errorf("downloaded geoip database is too small: %d bytes", written)
	}
	if written > maxDatabaseSize {
		return fmt.Errorf("downloaded geoip database exceeds maximum size: %d bytes", written)
	}
	if err = os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("activate geoip database: %w", err)
	}
	slog.Info("geoip country database updated", "path", targetPath, "bytes", written)
	return nil
}

func (u *Updater) needsUpdate() bool {
	info, err := os.Stat(strings.TrimSpace(u.Path))
	if err != nil {
		return true
	}
	if info.Size() < minDatabaseSize {
		return true
	}
	interval := u.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return time.Since(info.ModTime()) >= interval
}

func (u *Updater) Run(ctx context.Context) {
	if err := u.Ensure(ctx); err != nil {
		slog.Error("geoip database ensure failed", "error", err)
	}
	interval := u.Interval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := u.Download(ctx); err != nil {
				slog.Error("geoip database update failed", "error", err)
			}
		}
	}
}
