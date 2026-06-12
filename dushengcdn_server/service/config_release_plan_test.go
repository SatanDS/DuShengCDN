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

	unblocked, err := UnblockConfigReleaseBlockedChecksum(blocked.ID, "root", ConfigReleaseBlockedChecksumUnblockInput{
		Reason: "failure was caused by a transient offline node",
	})
	if err != nil {
		t.Fatalf("UnblockConfigReleaseBlockedChecksum failed: %v", err)
	}
	if unblocked.UnblockedAt == nil || unblocked.UnblockedBy != "root" || !strings.Contains(unblocked.UnblockReason, "transient") {
		t.Fatalf("unexpected unblocked checksum row: %+v", unblocked)
	}
	var audit model.ConfigReleaseBlockedChecksumAudit
	if err := model.DB.Where("blocked_checksum_id = ? AND action = ?", blocked.ID, "unblock").First(&audit).Error; err != nil {
		t.Fatalf("expected unblock audit row: %v", err)
	}
	if audit.Operator != "root" || audit.OriginalReason == "" || audit.Reason != unblocked.UnblockReason {
		t.Fatalf("unexpected unblock audit row: %+v", audit)
	}
	if _, err = CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &planView.Plan.ConfigVersionID,
		NodePool:        "canary",
	}); err != nil {
		t.Fatalf("expected unblocked checksum to allow repeated plan: %v", err)
	}
}

func TestConfigReleasePlansAreIsolatedByNodePool(t *testing.T) {
	setupServiceTestDB(t)

	seedConfigReleasePlanTestNode(t, "node-hk-1", "hk")
	seedConfigReleasePlanTestNode(t, "node-us-1", "us")
	hkRoute := seedConfigReleasePlanTestRoute(t, "hk-site", "hk.example.com", "http://8.8.8.8", "hk")
	seedConfigReleasePlanTestRoute(t, "us-site", "us.example.com", "http://8.8.4.4", "us")
	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if _, err := UpdateProxyRoute(hkRoute.ID, ProxyRouteInput{
		SiteName:  "hk-site",
		Domain:    "hk.example.com",
		OriginURL: "http://9.9.9.9",
		NodePool:  "hk",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpdateProxyRoute failed: %v", err)
	}
	candidate, err := CreateInactiveConfigVersion("root", false)
	if err != nil {
		t.Fatalf("CreateInactiveConfigVersion failed: %v", err)
	}

	hkPlan, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &candidate.Version.ID,
		NodePool:        "hk",
	})
	if err != nil {
		t.Fatalf("CreateConfigReleasePlan hk failed: %v", err)
	}
	if _, err = StartConfigReleasePlan(hkPlan.Plan.ID); err != nil {
		t.Fatalf("StartConfigReleasePlan hk failed: %v", err)
	}
	usPlan, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &candidate.Version.ID,
		NodePool:        "us",
	})
	if err != nil {
		t.Fatalf("expected us plan to be allowed while hk plan is running: %v", err)
	}
	if _, err = StartConfigReleasePlan(usPlan.Plan.ID); err != nil {
		t.Fatalf("expected us plan to start while hk plan is running: %v", err)
	}
	if _, err = CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &candidate.Version.ID,
		NodePool:        "hk",
	}); err == nil || !strings.Contains(err.Error(), "hk") {
		t.Fatalf("expected second hk plan to be rejected by pool mutex, got %v", err)
	}
}

func TestConfigReleasePlanCompletionActivatesOnlyItsNodePool(t *testing.T) {
	setupServiceTestDB(t)

	hkNode := seedConfigReleasePlanTestNode(t, "node-hk-complete", "hk")
	usNode := seedConfigReleasePlanTestNode(t, "node-us-complete", "us")
	hkRoute := seedConfigReleasePlanTestRoute(t, "hk-complete", "hk-complete.example.com", "http://8.8.8.8", "hk")
	seedConfigReleasePlanTestRoute(t, "us-complete", "us-complete.example.com", "http://8.8.4.4", "us")
	activeRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	activeHKArtifact, err := model.GetConfigVersionArtifact(activeRelease.Version.ID, "hk")
	if err != nil {
		t.Fatalf("load active hk artifact: %v", err)
	}
	activeUSArtifact, err := model.GetConfigVersionArtifact(activeRelease.Version.ID, "us")
	if err != nil {
		t.Fatalf("load active us artifact: %v", err)
	}
	if _, err := UpdateProxyRoute(hkRoute.ID, ProxyRouteInput{
		SiteName:  "hk-complete",
		Domain:    "hk-complete.example.com",
		OriginURL: "http://9.9.9.9",
		NodePool:  "hk",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpdateProxyRoute failed: %v", err)
	}
	candidate, err := CreateInactiveConfigVersion("root", false)
	if err != nil {
		t.Fatalf("CreateInactiveConfigVersion failed: %v", err)
	}
	candidateHKArtifact, err := model.GetConfigVersionArtifact(candidate.Version.ID, "hk")
	if err != nil {
		t.Fatalf("load candidate hk artifact: %v", err)
	}
	if candidateHKArtifact.Checksum == activeHKArtifact.Checksum {
		t.Fatal("expected hk candidate artifact checksum to differ from active")
	}

	plan, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &candidate.Version.ID,
		NodePool:        "hk",
		ObserveSeconds:  60,
	})
	if err != nil {
		t.Fatalf("CreateConfigReleasePlan failed: %v", err)
	}
	if _, err = StartConfigReleasePlan(plan.Plan.ID); err != nil {
		t.Fatalf("StartConfigReleasePlan failed: %v", err)
	}
	if err := model.DB.Model(hkNode).Updates(map[string]any{
		"current_version":  candidate.Version.Version,
		"current_checksum": candidateHKArtifact.Checksum,
	}).Error; err != nil {
		t.Fatalf("mark hk node current checksum failed: %v", err)
	}
	if _, err = CompleteConfigReleasePlan(plan.Plan.ID); err != nil {
		t.Fatalf("CompleteConfigReleasePlan failed: %v", err)
	}

	hkActiveVersion, hkActiveArtifact, err := model.GetActiveConfigVersionArtifactForPool("hk")
	if err != nil {
		t.Fatalf("GetActiveConfigVersionArtifactForPool hk failed: %v", err)
	}
	if hkActiveVersion.ID != candidate.Version.ID || hkActiveArtifact.Checksum != candidateHKArtifact.Checksum {
		t.Fatalf("expected hk pool active candidate, got version=%+v artifact=%+v", hkActiveVersion, hkActiveArtifact)
	}
	usActiveVersion, usActiveArtifact, err := model.GetActiveConfigVersionArtifactForPool("us")
	if err != nil {
		t.Fatalf("GetActiveConfigVersionArtifactForPool us failed: %v", err)
	}
	if usActiveVersion.ID != activeRelease.Version.ID || usActiveArtifact.Checksum != activeUSArtifact.Checksum {
		t.Fatalf("expected us pool to remain on active release, got version=%+v artifact=%+v", usActiveVersion, usActiveArtifact)
	}
	hkMeta, err := GetActiveConfigMetaForAgentNode(hkNode)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode hk failed: %v", err)
	}
	if hkMeta.Checksum != candidateHKArtifact.Checksum {
		t.Fatalf("expected hk agent meta to use candidate checksum, got %+v", hkMeta)
	}
	usMeta, err := GetActiveConfigMetaForAgentNode(usNode)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode us failed: %v", err)
	}
	if usMeta.Checksum != activeUSArtifact.Checksum {
		t.Fatalf("expected us agent meta to stay on active checksum, got %+v", usMeta)
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

func seedConfigReleasePlanTestRoute(t *testing.T, siteName string, domain string, originURL string, nodePool string) *ProxyRouteView {
	t.Helper()
	route, err := CreateProxyRoute(ProxyRouteInput{
		SiteName:  siteName,
		Domain:    domain,
		OriginURL: originURL,
		NodePool:  nodePool,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateProxyRoute(%s) failed: %v", domain, err)
	}
	return route
}
