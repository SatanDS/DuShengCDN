package service

import (
	"strings"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestConfigReleasePlanFailureRollsBackCanaryTargetAndBlocksChecksum(t *testing.T) {
	setupServiceTestDB(t)

	node := seedConfigReleasePlanTestNode(t, "node-canary-1", "canary")
	route, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:  "canary-site",
		Domain:    "canary.example.com",
		OriginURL: "http://8.8.8.8",
		NodePool:  "canary",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute failed: %v", err)
	}
	activeRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	activeArtifact, err := model.GetConfigVersionArtifact(activeRelease.Version.ID, "canary")
	if err != nil {
		t.Fatalf("GetConfigVersionArtifact active failed: %v", err)
	}
	if err := model.DB.Model(node).Updates(map[string]any{
		"current_version":  activeRelease.Version.Version,
		"current_checksum": activeArtifact.Checksum,
	}).Error; err != nil {
		t.Fatalf("seed active node checksum failed: %v", err)
	}

	if _, err = UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:  "canary-site",
		Domain:    "canary.example.com",
		OriginURL: "http://9.9.9.9",
		NodePool:  "canary",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpdateProxyRoute failed: %v", err)
	}
	planView, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		NodePool:       "canary",
		CanaryPercent:  20,
		ObserveSeconds: 1,
	})
	if err != nil {
		t.Fatalf("CreateConfigReleasePlan failed: %v", err)
	}
	if planView.Plan.Status != ConfigReleaseStatusDraft {
		t.Fatalf("expected draft plan, got %+v", planView.Plan)
	}
	if len(planView.Targets) != 1 || planView.Targets[0].NodeID != node.NodeID {
		t.Fatalf("expected one canary target for node, got %+v", planView.Targets)
	}
	target := planView.Targets[0]
	if target.Checksum == "" || target.Checksum == activeArtifact.Checksum {
		t.Fatalf("expected candidate checksum to differ from active, got target=%q active=%q", target.Checksum, activeArtifact.Checksum)
	}

	if _, err = StartConfigReleasePlan(planView.Plan.ID); err != nil {
		t.Fatalf("StartConfigReleasePlan failed: %v", err)
	}
	canaryMeta, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode canary failed: %v", err)
	}
	if canaryMeta.Checksum != target.Checksum {
		t.Fatalf("expected canary target checksum, got %+v target %+v", canaryMeta, target)
	}

	if _, err = ReportApplyLog(ApplyLogPayload{
		NodeID:   node.NodeID,
		Version:  canaryMeta.Version,
		Result:   ApplyResultFailed,
		Message:  "openresty reload failed",
		Checksum: canaryMeta.Checksum,
	}); err != nil {
		t.Fatalf("ReportApplyLog failed: %v", err)
	}
	failedPlan, err := model.GetConfigReleasePlanByID(planView.Plan.ID)
	if err != nil {
		t.Fatalf("GetConfigReleasePlanByID failed: %v", err)
	}
	if failedPlan.Status != ConfigReleaseStatusFailed || !strings.Contains(failedPlan.FailureReason, "openresty reload failed") {
		t.Fatalf("expected failed release plan with reason, got %+v", failedPlan)
	}
	blocked, err := model.GetConfigReleaseBlockedChecksum(target.Checksum)
	if err != nil {
		t.Fatalf("expected blocked checksum: %v", err)
	}
	if blocked.ConfigVersionID != planView.Plan.ConfigVersionID {
		t.Fatalf("unexpected blocked checksum row: %+v", blocked)
	}
	rollbackMeta, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode rollback failed: %v", err)
	}
	if rollbackMeta.Checksum != activeArtifact.Checksum || rollbackMeta.Version != activeRelease.Version.Version {
		t.Fatalf("expected node config meta to roll back to active version, got %+v active version=%s checksum=%s", rollbackMeta, activeRelease.Version.Version, activeArtifact.Checksum)
	}
	if _, err = CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &planView.Plan.ConfigVersionID,
		NodePool:        "canary",
	}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked checksum to reject repeated plan, got %v", err)
	}
}

func seedConfigReleasePlanTestNode(t *testing.T, nodeID string, poolName string) *model.Node {
	t.Helper()
	node := &model.Node{
		NodeID:       nodeID,
		Name:         nodeID,
		IP:           "203.0.113.10",
		PoolName:     poolName,
		Status:       NodeStatusOnline,
		AgentVersion: "v1.0.0",
		NginxVersion: "1.27.1.2",
		LastSeenAt:   time.Now(),
	}
	if err := node.Insert(); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	return node
}
