package dnsworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerSendsHeartbeatImmediatelyAfterInitialSnapshot(t *testing.T) {
	var heartbeatCount atomic.Int32
	heartbeatSeen := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/dns-snapshot":
			if token := strings.TrimSpace(r.Header.Get("X-DNS-Worker-Token")); token != "dns-worker-token" {
				t.Fatalf("unexpected snapshot token %q", token)
			}
			respondDNSWorkerTestJSON(t, w, map[string]any{
				"success": true,
				"data": map[string]any{
					"snapshot_version": "snap-1",
					"generated_at":     time.Now().UTC(),
					"zones":            []any{},
					"routes":           []any{},
					"nodes":            []any{},
				},
			})
		case "/api/dns-worker-heartbeat":
			if token := strings.TrimSpace(r.Header.Get("X-DNS-Worker-Token")); token != "dns-worker-token" {
				t.Fatalf("unexpected heartbeat token %q", token)
			}
			var input HeartbeatInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			if input.Status != WorkerStatusOnline || input.LastSnapshotVersion != "snap-1" || input.LastSnapshotAt == nil {
				t.Fatalf("unexpected heartbeat payload: %+v", input)
			}
			if heartbeatCount.Add(1) == 1 {
				close(heartbeatSeen)
				cancel()
			}
			respondDNSWorkerTestJSON(t, w, map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runner, err := NewRunner(&Config{
		ServerURL:         server.URL,
		Token:             "dns-worker-token",
		ListenAddr:        "127.0.0.1:0",
		SnapshotPath:      t.TempDir() + "/snapshot.json",
		HeartbeatInterval: time.Hour,
		RequestTimeout:    time.Second,
		SnapshotMaxAge:    time.Minute,
		QueryRateLimit:    DefaultQueryRateLimit,
		UDPResponseSize:   DefaultUDPResponseSize,
		Version:           "test-version",
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	select {
	case <-heartbeatSeen:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("expected initial heartbeat before the first ticker interval")
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Runner.Run returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("timed out waiting for runner shutdown")
	}
}

func TestRunnerSendsPendingUpdateResultOnce(t *testing.T) {
	var heartbeatCount atomic.Int32
	var firstResult *UpdateResultPayload
	var secondResult *UpdateResultPayload
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/dns-worker-heartbeat":
			var input HeartbeatInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			switch heartbeatCount.Add(1) {
			case 1:
				firstResult = input.UpdateResult
			case 2:
				secondResult = input.UpdateResult
			}
			respondDNSWorkerTestJSON(t, w, map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	runner := &Runner{
		Config: &Config{
			ServerURL:         server.URL,
			Token:             "dns-worker-token",
			InstallDir:        installDir,
			ListenAddr:        "127.0.0.1:0",
			HeartbeatInterval: time.Hour,
			RequestTimeout:    time.Second,
			Version:           "test-version",
		},
		Client:  NewAPIClient(server.URL, "dns-worker-token", time.Second),
		Store:   NewSnapshotStore(filepath.Join(installDir, "snapshot.json"), time.Minute),
		Rollups: NewRollupAggregator(time.Minute),
	}
	runner.savePendingUpdateResult(WorkerSettings{
		UpdateRepo:    "SatanDS/SatanDS-DuShengCDN-releases",
		UpdateChannel: "stable",
	}, true, "installer completed")
	if runner.loadPendingUpdateResult() == nil {
		t.Fatal("expected pending update result file to be written")
	}
	if err := runner.sendHeartbeat(ctx); err != nil {
		t.Fatalf("sendHeartbeat first: %v", err)
	}
	if firstResult == nil || !firstResult.Success || !strings.Contains(firstResult.Message, "completed") {
		t.Fatalf("expected first heartbeat to include update result, got %+v", firstResult)
	}
	if runner.loadPendingUpdateResult() != nil {
		t.Fatal("expected pending update result to be cleared after successful heartbeat")
	}
	if err := runner.sendHeartbeat(ctx); err != nil {
		t.Fatalf("sendHeartbeat second: %v", err)
	}
	if secondResult != nil {
		t.Fatalf("expected second heartbeat not to repeat update result, got %+v", secondResult)
	}
}

func respondDNSWorkerTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
