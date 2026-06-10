package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dushengcdn-agent/internal/nginx"
	"dushengcdn-agent/internal/protocol"
	"dushengcdn-agent/internal/state"
)

type fakeExecutor struct {
	testErr   error
	reloadErr error
}

type fakeClient struct {
	config     protocol.ActiveConfigResponse
	reports    []protocol.ApplyLogPayload
	fetchCalls int
}

type fakeManager struct {
	applyOutcome       nginx.ApplyOutcome
	currentChecksum    string
	currentChecksumErr error
	ensureErr          error
	fallbackErr        error
	ensureCalls        []bool
	fallbackReasons    []string
	applyMainContents  []string
	applyRouteContents []string
	applyFiles         [][]protocol.SupportFile
}

func (f *fakeExecutor) Test(ctx context.Context) error {
	return f.testErr
}

func (f *fakeExecutor) Reload(ctx context.Context) error {
	return f.reloadErr
}

func (f *fakeExecutor) EnsureRuntime(ctx context.Context, recreate bool) error {
	return nil
}

func (f *fakeExecutor) CheckHealth(ctx context.Context) error {
	return f.testErr
}

func (f *fakeExecutor) Restart(ctx context.Context) error {
	return f.reloadErr
}

func (f *fakeClient) GetActiveConfig(ctx context.Context) (*protocol.ActiveConfigResponse, error) {
	f.fetchCalls++
	return &f.config, nil
}

func (f *fakeClient) ReportApplyLog(ctx context.Context, payload protocol.ApplyLogPayload) error {
	f.reports = append(f.reports, payload)
	return nil
}

func (m *fakeManager) Apply(ctx context.Context, mainConfig string, routeConfig string, supportFiles []protocol.SupportFile) nginx.ApplyOutcome {
	m.applyMainContents = append(m.applyMainContents, mainConfig)
	m.applyRouteContents = append(m.applyRouteContents, routeConfig)
	m.applyFiles = append(m.applyFiles, append([]protocol.SupportFile(nil), supportFiles...))
	if m.applyOutcome.Status == "" {
		return nginx.ApplyOutcome{Status: nginx.ApplyStatusSuccess}
	}
	return m.applyOutcome
}

func (m *fakeManager) EnsureRuntime(ctx context.Context, recreate bool) error {
	m.ensureCalls = append(m.ensureCalls, recreate)
	return m.ensureErr
}

func (m *fakeManager) EnsureSafeFallbackRuntime(ctx context.Context, reason string) error {
	m.fallbackReasons = append(m.fallbackReasons, reason)
	return m.fallbackErr
}

func (m *fakeManager) CurrentChecksum() (string, error) {
	return m.currentChecksum, m.currentChecksumErr
}

func setBundleChecksum(config *protocol.ActiveConfigResponse) {
	routeConfig := config.RouteConfig
	if routeConfig == "" {
		routeConfig = config.RenderedConfig
	}
	config.Checksum = nginx.BundleChecksum(config.MainConfig, routeConfig, config.SupportFiles)
}

func TestSyncOnceSuccess(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-001",
			MainConfig:     "worker_processes auto;",
			RouteConfig:    "server { listen 80; }",
			RenderedConfig: "server { listen 80; }",
			SupportFiles:   []protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	setBundleChecksum(&client.config)

	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	snapshot, _ := stateStore.Load()
	snapshot.NodeID = nodeID
	if err = stateStore.Save(snapshot); err != nil {
		t.Fatalf("failed to save initial state: %v", err)
	}

	routePath := filepath.Join(t.TempDir(), "routes.conf")
	service := New(client, &nginx.Manager{
		MainConfigPath:  filepath.Join(filepath.Dir(routePath), "nginx.conf"),
		RouteConfigPath: routePath,
		Executor:        &fakeExecutor{},
	}, stateStore)

	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}

	data, err := os.ReadFile(routePath)
	if err != nil {
		t.Fatalf("failed to read route config: %v", err)
	}
	if string(data) != "server { listen 80; }" {
		t.Fatal("expected rendered config to be written to route file")
	}
	mainData, err := os.ReadFile(filepath.Join(filepath.Dir(routePath), "nginx.conf"))
	if err != nil {
		t.Fatalf("failed to read main config: %v", err)
	}
	if string(mainData) != "worker_processes auto;" {
		t.Fatal("expected main config to be written")
	}
	snapshot, err = stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.CurrentVersion != "20260309-001" || snapshot.CurrentChecksum != client.config.Checksum {
		t.Fatal("expected state store to persist current version and checksum")
	}
	if len(client.reports) != 1 || client.reports[0].Result != ApplyResultSuccess {
		t.Fatal("expected successful apply report to be sent")
	}
	if client.reports[0].Checksum != client.config.Checksum {
		t.Fatalf("expected config checksum to be reported, got %q", client.reports[0].Checksum)
	}
	if client.reports[0].MainConfigChecksum == "" || client.reports[0].RouteConfigChecksum == "" {
		t.Fatal("expected main and route config checksums to be reported")
	}
	if client.reports[0].SupportFileCount != 1 {
		t.Fatalf("expected support file count to be reported, got %d", client.reports[0].SupportFileCount)
	}
}

func TestSyncOnceRollbackOnNginxFailure(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-002",
			MainConfig:     "worker_processes 2;",
			RouteConfig:    "server { listen 81; }",
			RenderedConfig: "server { listen 81; }",
			SupportFiles:   []protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	setBundleChecksum(&client.config)

	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-001",
		CurrentChecksum: "previous-checksum",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	service := New(client, &fakeManager{
		applyOutcome: nginx.ApplyOutcome{
			Status:  nginx.ApplyStatusFatal,
			Message: "openresty failed after rollback",
		},
	}, stateStore)

	err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	})
	if err == nil {
		t.Fatal("expected SyncOnce to fail when apply outcome is fatal")
	}
	snapshot, loadErr := stateStore.Load()
	if loadErr != nil {
		t.Fatalf("failed to load state: %v", loadErr)
	}
	if snapshot.CurrentVersion != "20260309-001" {
		t.Fatal("expected failed sync not to overwrite current version")
	}
	if snapshot.BlockedVersion != "20260309-002" || snapshot.BlockedChecksum != client.config.Checksum {
		t.Fatalf("expected failed target version to be blocked, got %+v", snapshot)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusUnhealthy {
		t.Fatalf("expected unhealthy openresty status, got %q", snapshot.OpenrestyStatus)
	}
	if len(client.reports) != 1 || client.reports[0].Result != ApplyResultFailed {
		t.Fatal("expected failed apply report to be sent")
	}
	if client.reports[0].Checksum != client.config.Checksum {
		t.Fatalf("expected failed report to retain target checksum, got %q", client.reports[0].Checksum)
	}
	if client.reports[0].MainConfigChecksum == "" || client.reports[0].RouteConfigChecksum == "" {
		t.Fatal("expected failed report to include main and route config checksums")
	}
	if client.reports[0].SupportFileCount != 1 {
		t.Fatalf("expected failed report to include support file count, got %d", client.reports[0].SupportFileCount)
	}
}

func TestSyncOnceReportsWarningWhenRollbackKeepsOpenrestyHealthy(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-002",
			MainConfig:     "worker_processes 2;",
			RouteConfig:    "server { listen 81; }",
			RenderedConfig: "server { listen 81; }",
			SupportFiles:   []protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	setBundleChecksum(&client.config)

	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-001",
		CurrentChecksum: "previous-checksum",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	service := New(client, &fakeManager{
		applyOutcome: nginx.ApplyOutcome{
			Status:  nginx.ApplyStatusWarning,
			Message: "apply failed, rolled back to previous config",
		},
	}, stateStore)

	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("expected warning outcome to keep sync successful, got %v", err)
	}

	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.CurrentVersion != "20260309-001" || snapshot.CurrentChecksum != "previous-checksum" {
		t.Fatal("expected warning apply to keep previous version state")
	}
	if snapshot.BlockedVersion != "20260309-002" || snapshot.BlockedChecksum != client.config.Checksum {
		t.Fatalf("expected rolled-back target version to be blocked, got %+v", snapshot)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusHealthy {
		t.Fatalf("expected healthy openresty after rollback, got %q", snapshot.OpenrestyStatus)
	}
	if snapshot.LastError == "" {
		t.Fatal("expected rollback warning to be recorded")
	}
	if len(client.reports) != 1 || client.reports[0].Result != ApplyResultWarning {
		t.Fatal("expected warning apply report to be sent")
	}
}

func TestSyncOnceRedactsSensitiveApplyMessage(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:     "20260309-002",
			MainConfig:  "worker_processes auto;",
			RouteConfig: "server { listen 82; }",
			CreatedAt:   time.Now().Format(time.RFC3339),
		},
	}
	setBundleChecksum(&client.config)

	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{NodeID: nodeID}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	service := New(client, &fakeManager{
		applyOutcome: nginx.ApplyOutcome{
			Status: nginx.ApplyStatusWarning,
			Message: `openresty -t failed:
local expected_hash = "abcdef123456"
proxy_set_header Authorization "Bearer origin-token";
callback /oauth?code=oauth-code&state=csrf-state`,
		},
	}, stateStore)

	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("expected warning outcome to keep sync successful, got %v", err)
	}
	if len(client.reports) != 1 {
		t.Fatalf("expected one apply report, got %+v", client.reports)
	}
	reported := client.reports[0].Message
	for _, leaked := range []string{"abcdef123456", "origin-token", "oauth-code", "csrf-state"} {
		if strings.Contains(reported, leaked) {
			t.Fatalf("expected %q to be redacted from %q", leaked, reported)
		}
	}
}

func TestSyncOnStartupRecreatesRuntimeWhenChecksumMatches(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-003",
			Checksum:       "checksum-3",
			MainConfig:     "worker_processes auto;",
			RouteConfig:    "server { listen 82; }",
			RenderedConfig: "server { listen 82; }",
			SupportFiles:   []protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{NodeID: nodeID}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: "checksum-3"}
	service := New(client, manager, stateStore)
	if err = service.SyncOnStartup(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("SyncOnStartup failed: %v", err)
	}
	if len(manager.ensureCalls) != 1 || !manager.ensureCalls[0] {
		t.Fatal("expected startup sync to recreate runtime")
	}
	if len(client.reports) != 0 {
		t.Fatal("expected no apply report when checksum already matches")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.CurrentChecksum != "checksum-3" || snapshot.CurrentVersion != "20260309-003" {
		t.Fatal("expected snapshot to be refreshed from active config")
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusHealthy || snapshot.OpenrestyMessage != "" {
		t.Fatal("expected startup sync to mark openresty healthy")
	}
}

func TestSyncOnStartupRecordsRuntimeFailure(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-004",
			Checksum:       "checksum-4",
			MainConfig:     "worker_processes 4;",
			RouteConfig:    "server { listen 83; }",
			RenderedConfig: "server { listen 83; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{NodeID: nodeID}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{
		currentChecksum: "checksum-4",
		ensureErr:       context.DeadlineExceeded,
	}
	service := New(client, manager, stateStore)
	if err = service.SyncOnStartup(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err == nil {
		t.Fatal("expected SyncOnStartup to fail when runtime recreation fails")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusUnhealthy {
		t.Fatalf("expected unhealthy openresty status, got %q", snapshot.OpenrestyStatus)
	}
	if snapshot.OpenrestyMessage == "" {
		t.Fatal("expected runtime error message to be recorded")
	}
}

func TestSyncOnceSkipsPreviouslyBlockedVersion(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-006",
			Checksum:       "checksum-6",
			MainConfig:     "worker_processes 6;",
			RouteConfig:    "server { listen 86; }",
			RenderedConfig: "server { listen 86; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-005",
		CurrentChecksum: "checksum-5",
		BlockedVersion:  "20260309-006",
		BlockedChecksum: "checksum-6",
		BlockedReason:   "apply failed, rolled back to previous config",
		LastError:       "apply failed, rolled back to previous config",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: "checksum-5"}
	service := New(client, manager, stateStore)
	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-006",
		Checksum: "checksum-6",
	}); err != nil {
		t.Fatalf("expected blocked version to be skipped, got %v", err)
	}
	if client.fetchCalls != 0 {
		t.Fatalf("expected blocked version to skip fetch, got %d", client.fetchCalls)
	}
	if len(manager.applyMainContents) != 0 {
		t.Fatal("expected blocked version to skip apply")
	}
	if len(client.reports) != 0 {
		t.Fatal("expected blocked version to skip reporting duplicate apply result")
	}
}

func TestSyncOnStartupKeepsBlockedVersionSuppressedUntilNewTargetArrives(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-007",
			Checksum:       "checksum-7",
			MainConfig:     "worker_processes 7;",
			RouteConfig:    "server { listen 87; }",
			RenderedConfig: "server { listen 87; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:           nodeID,
		CurrentVersion:   "20260309-005",
		CurrentChecksum:  "checksum-5",
		BlockedVersion:   "20260309-007",
		BlockedChecksum:  "checksum-7",
		BlockedReason:    "apply failed, rolled back to previous config",
		OpenrestyStatus:  protocol.OpenrestyStatusUnhealthy,
		OpenrestyMessage: "apply failed, rolled back to previous config",
		LastError:        "apply failed, rolled back to previous config",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: "checksum-5"}
	service := New(client, manager, stateStore)
	if err = service.SyncOnStartup(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-007",
		Checksum: "checksum-7",
	}); err != nil {
		t.Fatalf("expected blocked startup target to be skipped, got %v", err)
	}
	if len(manager.ensureCalls) != 1 || !manager.ensureCalls[0] {
		t.Fatal("expected startup skip to ensure runtime with current local config")
	}
	if client.fetchCalls != 0 {
		t.Fatalf("expected blocked startup target to skip fetch, got %d", client.fetchCalls)
	}
	if len(client.reports) != 0 {
		t.Fatal("expected blocked startup target to skip duplicate apply report")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.BlockedVersion != "20260309-007" || snapshot.BlockedChecksum != "checksum-7" {
		t.Fatalf("expected blocked target to remain recorded, got %+v", snapshot)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusHealthy {
		t.Fatalf("expected startup runtime recovery to mark openresty healthy, got %q", snapshot.OpenrestyStatus)
	}
}

func TestSyncOnStartupStartsFallbackWhenBlockedVersionHasNoLocalConfig(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-007",
			Checksum:       "checksum-7",
			MainConfig:     "worker_processes 7;",
			RouteConfig:    "server { listen 87; }",
			RenderedConfig: "server { listen 87; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:           nodeID,
		BlockedVersion:   "20260309-007",
		BlockedChecksum:  "checksum-7",
		BlockedReason:    "apply failed, but fallback runtime started",
		OpenrestyStatus:  protocol.OpenrestyStatusUnhealthy,
		OpenrestyMessage: "apply failed, but fallback runtime started",
		LastError:        "apply failed, but fallback runtime started",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{}
	service := New(client, manager, stateStore)
	if err = service.SyncOnStartup(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-007",
		Checksum: "checksum-7",
	}); err != nil {
		t.Fatalf("expected blocked startup target to start fallback, got %v", err)
	}
	if len(manager.fallbackReasons) != 1 {
		t.Fatalf("expected fallback runtime to be started once, got %d", len(manager.fallbackReasons))
	}
	if client.fetchCalls != 0 {
		t.Fatalf("expected blocked startup target to skip fetch, got %d", client.fetchCalls)
	}
	if len(client.reports) != 0 {
		t.Fatal("expected blocked startup target to skip duplicate apply report")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.BlockedVersion != "20260309-007" || snapshot.BlockedChecksum != "checksum-7" {
		t.Fatalf("expected blocked target to remain recorded, got %+v", snapshot)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusHealthy {
		t.Fatalf("expected fallback startup recovery to mark openresty healthy, got %q", snapshot.OpenrestyStatus)
	}
	if snapshot.OpenrestyMessage != "safe default fallback runtime started" {
		t.Fatalf("expected fallback status message, got %q", snapshot.OpenrestyMessage)
	}
}

func TestSyncOnStartupStartsFallbackWhenResidualConfigCannotRecover(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-007",
			Checksum:       "checksum-7",
			MainConfig:     "worker_processes 7;",
			RouteConfig:    "server { listen 87; }",
			RenderedConfig: "server { listen 87; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		BlockedVersion:  "20260309-007",
		BlockedChecksum: "checksum-7",
		BlockedReason:   "apply failed, but fallback runtime started",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{
		currentChecksum: "residual-checksum",
		ensureErr:       context.DeadlineExceeded,
	}
	service := New(client, manager, stateStore)
	if err = service.SyncOnStartup(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-007",
		Checksum: "checksum-7",
	}); err != nil {
		t.Fatalf("expected residual config failure to start fallback, got %v", err)
	}
	if len(manager.ensureCalls) != 1 {
		t.Fatalf("expected residual config to be tested once, got %d", len(manager.ensureCalls))
	}
	if len(manager.fallbackReasons) != 1 {
		t.Fatalf("expected fallback runtime to be started once, got %d", len(manager.fallbackReasons))
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.OpenrestyStatus != protocol.OpenrestyStatusHealthy {
		t.Fatalf("expected fallback startup recovery to mark openresty healthy, got %q", snapshot.OpenrestyStatus)
	}
	if snapshot.BlockedVersion != "20260309-007" || snapshot.BlockedChecksum != "checksum-7" {
		t.Fatalf("expected blocked target to remain recorded, got %+v", snapshot)
	}
}

func TestSyncOnceClearsBlockedTargetWhenNewVersionArrives(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-008",
			MainConfig:     "worker_processes 8;",
			RouteConfig:    "server { listen 88; }",
			RenderedConfig: "server { listen 88; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	setBundleChecksum(&client.config)
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-005",
		CurrentChecksum: "previous-checksum",
		BlockedVersion:  "20260309-007",
		BlockedChecksum: "blocked-checksum",
		BlockedReason:   "apply failed, rolled back to previous config",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{}
	service := New(client, manager, stateStore)
	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-008",
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("expected new target version to be applied, got %v", err)
	}
	if client.fetchCalls != 1 {
		t.Fatalf("expected new target to trigger fetch, got %d", client.fetchCalls)
	}
	if len(manager.applyMainContents) != 1 {
		t.Fatal("expected new target to trigger apply")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.BlockedVersion != "" || snapshot.BlockedChecksum != "" {
		t.Fatalf("expected blocked target to be cleared after new version succeeds, got %+v", snapshot)
	}
	if snapshot.CurrentVersion != "20260309-008" || snapshot.CurrentChecksum != client.config.Checksum {
		t.Fatalf("expected current version to move to new target, got %+v", snapshot)
	}
}

func TestSyncOnceRejectsFetchedConfigWithMismatchedBundleChecksum(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-011",
			Checksum:       "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			MainConfig:     "worker_processes 11;",
			RouteConfig:    "server { listen 91; }",
			RenderedConfig: "server { listen 91; }",
			SupportFiles:   []protocol.SupportFile{{Path: "1.crt", Content: "cert"}},
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-010",
		CurrentChecksum: "previous-checksum",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: "previous-checksum"}
	service := New(client, manager, stateStore)
	err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	})
	if err == nil {
		t.Fatal("expected mismatched active config checksum to fail")
	}
	if len(manager.applyMainContents) != 0 {
		t.Fatal("expected mismatched active config not to be applied")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.CurrentVersion != "20260309-010" || snapshot.CurrentChecksum != "previous-checksum" {
		t.Fatalf("expected current config state to remain unchanged, got %+v", snapshot)
	}
	if snapshot.BlockedVersion != client.config.Version || snapshot.BlockedChecksum != client.config.Checksum {
		t.Fatalf("expected mismatched target to be blocked, got %+v", snapshot)
	}
	if snapshot.LastError == "" {
		t.Fatal("expected integrity failure to be recorded")
	}
	if len(client.reports) != 1 || client.reports[0].Result != ApplyResultFailed {
		t.Fatalf("expected one failed apply log for integrity failure, got %+v", client.reports)
	}
	if client.reports[0].Checksum != client.config.Checksum {
		t.Fatalf("expected failed report to keep declared checksum, got %q", client.reports[0].Checksum)
	}
}

func TestSyncOnceSkipsFetchWhenHeartbeatChecksumMatches(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-005",
			Checksum:       "checksum-5",
			MainConfig:     "worker_processes auto;",
			RouteConfig:    "server { listen 84; }",
			RenderedConfig: "server { listen 84; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  client.config.Version,
		CurrentChecksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: client.config.Checksum}
	service := New(client, manager, stateStore)
	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  client.config.Version,
		Checksum: client.config.Checksum,
	}); err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}
	if client.fetchCalls != 0 {
		t.Fatalf("expected no active config fetch when heartbeat checksum matches, got %d", client.fetchCalls)
	}
	if len(client.reports) != 0 {
		t.Fatal("expected no apply log when no config change is needed")
	}
}

func TestSyncOnceSameChecksumDifferentVersionSkipsFetchApplyAndUpdatesVersion(t *testing.T) {
	client := &fakeClient{
		config: protocol.ActiveConfigResponse{
			Version:        "20260309-010",
			Checksum:       "checksum-9",
			MainConfig:     "worker_processes auto;",
			RouteConfig:    "server { listen 89; }",
			RenderedConfig: "server { listen 89; }",
			CreatedAt:      time.Now().Format(time.RFC3339),
		},
	}
	stateStore := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	nodeID, err := stateStore.EnsureNodeID()
	if err != nil {
		t.Fatalf("EnsureNodeID failed: %v", err)
	}
	if err = stateStore.Save(&state.Snapshot{
		NodeID:          nodeID,
		CurrentVersion:  "20260309-009",
		CurrentChecksum: "checksum-9",
		BlockedVersion:  "20260309-008",
		BlockedChecksum: "checksum-8",
		BlockedReason:   "older apply failed",
		LastError:       "older apply failed",
	}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	manager := &fakeManager{currentChecksum: "checksum-9"}
	service := New(client, manager, stateStore)
	if err = service.SyncOnce(context.Background(), &protocol.ActiveConfigMeta{
		Version:  "20260309-010",
		Checksum: "checksum-9",
	}); err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}
	if client.fetchCalls != 0 {
		t.Fatalf("expected same-checksum version change to skip fetch, got %d", client.fetchCalls)
	}
	if len(manager.applyMainContents) != 0 {
		t.Fatal("expected same-checksum version change to skip apply")
	}
	if len(manager.ensureCalls) != 0 {
		t.Fatal("expected periodic same-checksum version change not to reload runtime")
	}
	if len(client.reports) != 0 {
		t.Fatal("expected no apply report when no config apply occurred")
	}
	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if snapshot.CurrentVersion != "20260309-010" || snapshot.CurrentChecksum != "checksum-9" {
		t.Fatalf("expected snapshot to record new version with existing checksum, got %+v", snapshot)
	}
	if snapshot.BlockedVersion != "" || snapshot.BlockedChecksum != "" || snapshot.BlockedReason != "" {
		t.Fatalf("expected stale blocked target to be cleared, got %+v", snapshot)
	}
	if snapshot.LastError != "" {
		t.Fatalf("expected last error to be cleared, got %q", snapshot.LastError)
	}
}
