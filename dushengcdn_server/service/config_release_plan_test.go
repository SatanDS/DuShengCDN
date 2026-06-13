package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"dushengcdn/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
		Force:           true,
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

func TestSelectConfigReleasePlanNodesUsesPoolOnlineQuery(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now()
	nodes := []*model.Node{
		{
			NodeID:       "node-hk-new",
			Name:         "node-hk-new",
			IP:           "203.0.113.21",
			PoolName:     "hk",
			Status:       NodeStatusOnline,
			AgentVersion: "v1.0.0",
			LastSeenAt:   now,
		},
		{
			NodeID:       "node-hk-recent-stored-offline",
			Name:         "node-hk-recent-stored-offline",
			IP:           "203.0.113.22",
			PoolName:     "hk",
			Status:       NodeStatusOffline,
			AgentVersion: "v1.0.0",
			LastSeenAt:   now.Add(-time.Second),
		},
		{
			NodeID:       "node-hk-stale",
			Name:         "node-hk-stale",
			IP:           "203.0.113.23",
			PoolName:     "hk",
			Status:       NodeStatusOnline,
			AgentVersion: "v1.0.0",
			LastSeenAt:   now.Add(-10 * time.Minute),
		},
		{
			NodeID:       "node-us-new",
			Name:         "node-us-new",
			IP:           "203.0.113.24",
			PoolName:     "us",
			Status:       NodeStatusOnline,
			AgentVersion: "v1.0.0",
			LastSeenAt:   now,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}

	var selected []*model.Node
	queries, err := captureConfigReleasePlanSQL(t, func() error {
		var selectErr error
		selected, selectErr = selectReleasePlanNodes("hk", 0)
		return selectErr
	})
	if err != nil {
		t.Fatalf("selectReleasePlanNodes failed: %v", err)
	}
	if len(selected) != 2 || selected[0].NodeID != "node-hk-new" || selected[1].NodeID != "node-hk-recent-stored-offline" {
		t.Fatalf("expected only recent hk nodes sorted by heartbeat, got %+v", selected)
	}
	nodeQueries := configReleasePlanNodeSelectQueries(queries)
	if len(nodeQueries) != 1 {
		t.Fatalf("expected one nodes query, got %d: %#v", len(nodeQueries), queries)
	}
	normalizedQuery := strings.ToLower(nodeQueries[0])
	if !strings.Contains(normalizedQuery, "pool_name") || !strings.Contains(normalizedQuery, "last_seen_at") {
		t.Fatalf("expected pool-scoped online nodes query, got %s", nodeQueries[0])
	}
}

func TestEvaluateConfigReleasePlanUsesTargetNodeIDQuery(t *testing.T) {
	setupServiceTestDB(t)

	now := time.Now()
	version := &model.ConfigVersion{
		Version:        "20260613-evaluate-target-query",
		Checksum:       "global-checksum",
		MainConfig:     "events {}",
		RenderedConfig: "server {}",
		CreatedBy:      "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("create config version: %v", err)
	}
	targetNode := &model.Node{
		NodeID:          "node-evaluate-target",
		Name:            "node-evaluate-target",
		IP:              "203.0.113.25",
		PoolName:        "hk",
		Status:          NodeStatusOffline,
		AgentVersion:    "v1.0.0",
		CurrentChecksum: "target-checksum",
		LastSeenAt:      now,
	}
	unrelatedNode := &model.Node{
		NodeID:          "node-evaluate-unrelated",
		Name:            "node-evaluate-unrelated",
		IP:              "203.0.113.26",
		PoolName:        "us",
		Status:          NodeStatusOnline,
		AgentVersion:    "v1.0.0",
		CurrentChecksum: "target-checksum",
		LastSeenAt:      now,
	}
	for _, node := range []*model.Node{targetNode, unrelatedNode} {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}
	plan := &model.ConfigReleasePlan{
		ConfigVersionID: version.ID,
		Status:          ConfigReleaseStatusRunning,
		CanaryPoolName:  "hk",
		ObserveSeconds:  60,
		Checksum:        "target-checksum",
		CreatedBy:       "root",
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("create release plan: %v", err)
	}
	if err := model.DB.Create(&model.ConfigReleaseTarget{
		PlanID:          plan.ID,
		ConfigVersionID: version.ID,
		NodeID:          targetNode.NodeID,
		PoolName:        "hk",
		Checksum:        "target-checksum",
		StageIndex:      1,
		Status:          ConfigReleaseTargetApplying,
		StartedAt:       &now,
	}).Error; err != nil {
		t.Fatalf("create release target: %v", err)
	}

	var evaluation *ConfigReleasePlanEvaluation
	queries, err := captureConfigReleasePlanSQL(t, func() error {
		var evalErr error
		evaluation, evalErr = EvaluateConfigReleasePlan(plan.ID)
		return evalErr
	})
	if err != nil {
		t.Fatalf("EvaluateConfigReleasePlan failed: %v", err)
	}
	if !evaluation.Healthy || evaluation.SucceededTargets != 1 || evaluation.TargetCount != 1 {
		t.Fatalf("unexpected evaluation: %+v", evaluation)
	}
	nodeQueries := configReleasePlanNodeSelectQueries(queries)
	if len(nodeQueries) != 1 {
		t.Fatalf("expected one nodes query, got %d: %#v", len(nodeQueries), queries)
	}
	normalizedQuery := strings.ToLower(nodeQueries[0])
	if !strings.Contains(normalizedQuery, "node_id") || !strings.Contains(normalizedQuery, " in ") {
		t.Fatalf("expected target node_id query, got %s", nodeQueries[0])
	}
	if strings.Contains(normalizedQuery, unrelatedNode.NodeID) {
		t.Fatalf("expected unrelated node to stay out of target lookup query, got %s", nodeQueries[0])
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

func TestBrokenPoolActiveConfigDoesNotFallbackToGlobal(t *testing.T) {
	setupServiceTestDB(t)

	node := seedConfigReleasePlanTestNode(t, "node-broken-pool-active", "hk")
	seedConfigReleasePlanTestRoute(t, "broken-pool-active", "broken-pool-active.example.com", "http://8.8.8.8", "hk")
	activeRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if err := model.DB.Model(&model.ConfigPoolActiveVersion{}).
		Where("pool_name = ?", "hk").
		Updates(map[string]any{
			"config_version_id": activeRelease.Version.ID,
			"artifact_id":       uint(999999),
			"checksum":          "missing-artifact",
			"activated_at":      time.Now(),
		}).Error; err != nil {
		t.Fatalf("break pool active version: %v", err)
	}

	_, err = GetActiveConfigMetaForAgentNode(node)
	if err == nil || !strings.Contains(err.Error(), "active config reference is broken") {
		t.Fatalf("expected broken pool active reference to be reported, got %v", err)
	}
}

func TestCreateConfigReleasePlanRejectsUnchangedPoolUnlessForced(t *testing.T) {
	setupServiceTestDB(t)

	seedConfigReleasePlanTestNode(t, "node-unchanged-us", "us")
	seedConfigReleasePlanTestRoute(t, "unchanged-us", "unchanged-us.example.com", "http://8.8.4.4", "us")
	activeRelease, err := PublishConfigVersion("root", false)
	if err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if _, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &activeRelease.Version.ID,
		NodePool:        "us",
	}); err == nil || !strings.Contains(err.Error(), "no config changes") {
		t.Fatalf("expected unchanged pool release to be rejected, got %v", err)
	}
	forced, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{
		ConfigVersionID: &activeRelease.Version.ID,
		NodePool:        "us",
		Force:           true,
	})
	if err != nil {
		t.Fatalf("expected forced unchanged pool release to be allowed: %v", err)
	}
	if forced.Plan.Checksum == "" {
		t.Fatalf("expected forced plan checksum, got %+v", forced.Plan)
	}
}

func TestDraftConfigReleasePlanCanBeCanceledButNotFailed(t *testing.T) {
	setupServiceTestDB(t)

	seedConfigReleasePlanTestNode(t, "node-cancel-draft", "hk")
	route := seedConfigReleasePlanTestRoute(t, "cancel-draft", "cancel-draft.example.com", "http://8.8.8.8", "hk")
	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("PublishConfigVersion failed: %v", err)
	}
	if _, err := UpdateProxyRoute(route.ID, ProxyRouteInput{
		SiteName:  "cancel-draft",
		Domain:    "cancel-draft.example.com",
		OriginURL: "http://9.9.9.9",
		NodePool:  "hk",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("UpdateProxyRoute failed: %v", err)
	}
	plan, err := CreateConfigReleasePlan("root", ConfigReleasePlanInput{NodePool: "hk"})
	if err != nil {
		t.Fatalf("CreateConfigReleasePlan failed: %v", err)
	}
	if err := FailConfigReleasePlan(plan.Plan.ID, "operator clicked fail"); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected draft fail to be rejected with cancel guidance, got %v", err)
	}
	if _, err := model.GetConfigReleaseBlockedChecksumForPool("hk", plan.Plan.Checksum); err == nil {
		t.Fatal("expected draft fail rejection not to create blocked checksum")
	}
	canceled, err := CancelConfigReleasePlan(plan.Plan.ID)
	if err != nil {
		t.Fatalf("CancelConfigReleasePlan failed: %v", err)
	}
	if canceled.Plan.Status != ConfigReleaseStatusCanceled {
		t.Fatalf("expected canceled plan, got %+v", canceled.Plan)
	}
}

func TestActiveConfigReleaseTargetIgnoresCompletedPlanTargets(t *testing.T) {
	setupServiceTestDB(t)

	node := seedConfigReleasePlanTestNode(t, "node-active-target-history", "hk")
	version := &model.ConfigVersion{
		Version:        "20260613-active-target",
		Checksum:       "global-checksum",
		MainConfig:     "events {}",
		RenderedConfig: "server {}",
		CreatedBy:      "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("create config version: %v", err)
	}
	artifact := &model.ConfigVersionArtifact{
		ConfigVersionID: version.ID,
		PoolName:        "hk",
		Checksum:        "running-checksum",
		RenderedConfig:  "server { # hk }",
	}
	if err := model.DB.Create(artifact).Error; err != nil {
		t.Fatalf("create config version artifact: %v", err)
	}

	runningPlan := &model.ConfigReleasePlan{
		ConfigVersionID: version.ID,
		Status:          ConfigReleaseStatusRunning,
		CanaryPoolName:  "hk",
		Checksum:        "running-checksum",
		CreatedBy:       "root",
	}
	if err := model.DB.Create(runningPlan).Error; err != nil {
		t.Fatalf("create running plan: %v", err)
	}
	runningTarget := &model.ConfigReleaseTarget{
		PlanID:          runningPlan.ID,
		ConfigVersionID: version.ID,
		NodeID:          node.NodeID,
		PoolName:        "hk",
		Checksum:        "running-checksum",
		StageIndex:      1,
		Status:          ConfigReleaseTargetApplying,
	}
	if err := model.DB.Create(runningTarget).Error; err != nil {
		t.Fatalf("create running target: %v", err)
	}

	completedPlan := &model.ConfigReleasePlan{
		ConfigVersionID: version.ID,
		Status:          ConfigReleaseStatusCompleted,
		CanaryPoolName:  "hk",
		Checksum:        "completed-checksum",
		CreatedBy:       "root",
	}
	if err := model.DB.Create(completedPlan).Error; err != nil {
		t.Fatalf("create completed plan: %v", err)
	}
	if err := model.DB.Create(&model.ConfigReleaseTarget{
		PlanID:          completedPlan.ID,
		ConfigVersionID: version.ID,
		NodeID:          node.NodeID,
		PoolName:        "hk",
		Checksum:        "completed-checksum",
		StageIndex:      1,
		Status:          ConfigReleaseTargetSucceeded,
	}).Error; err != nil {
		t.Fatalf("create completed target: %v", err)
	}

	plan, target, err := model.GetActiveConfigReleaseTargetForNodeID(node.NodeID)
	if err != nil {
		t.Fatalf("GetActiveConfigReleaseTargetForNodeID failed: %v", err)
	}
	if plan.ID != runningPlan.ID || target.ID != runningTarget.ID || target.Checksum != runningTarget.Checksum {
		t.Fatalf("expected running target despite newer completed target, got plan=%+v target=%+v", plan, target)
	}
	meta, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode failed: %v", err)
	}
	if meta.Checksum != runningTarget.Checksum {
		t.Fatalf("expected active config meta to use running target checksum, got %+v", meta)
	}
}

func TestActiveConfigReleaseTargetIgnoresExpiredSucceededTarget(t *testing.T) {
	setupServiceTestDB(t)

	node := seedConfigReleasePlanTestNode(t, "node-expired-succeeded-target", "hk")
	activeVersion := seedAgentTestActiveConfigVersionWithArtifacts(t, "20260613-active-pool", "global-active-checksum", map[string]string{
		"hk": "active-pool-checksum",
	})
	if _, _, err := ActivateConfigVersionForPool(activeVersion.ID, "hk", nil); err != nil {
		t.Fatalf("ActivateConfigVersionForPool failed: %v", err)
	}
	if err := model.DB.Model(node).Updates(map[string]any{
		"current_version":  activeVersion.Version,
		"current_checksum": "active-pool-checksum",
	}).Error; err != nil {
		t.Fatalf("seed node active checksum failed: %v", err)
	}

	candidateVersion := &model.ConfigVersion{
		Version:        "20260613-expired-target",
		Checksum:       "candidate-global-checksum",
		MainConfig:     "events {}",
		RenderedConfig: "server {}",
		CreatedBy:      "root",
	}
	if err := model.DB.Create(candidateVersion).Error; err != nil {
		t.Fatalf("create candidate config version: %v", err)
	}
	candidateArtifact := &model.ConfigVersionArtifact{
		ConfigVersionID:  candidateVersion.ID,
		PoolName:         "hk",
		Checksum:         "expired-target-checksum",
		RenderedConfig:   "server { # expired }",
		SupportFilesJSON: "[]",
	}
	if err := model.DB.Create(candidateArtifact).Error; err != nil {
		t.Fatalf("create candidate artifact: %v", err)
	}
	completedAt := time.Now().Add(-10 * time.Minute)
	plan := &model.ConfigReleasePlan{
		ConfigVersionID: candidateVersion.ID,
		Status:          ConfigReleaseStatusRunning,
		CanaryPoolName:  "hk",
		ObserveSeconds:  1,
		Checksum:        candidateArtifact.Checksum,
		CreatedBy:       "root",
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("create running plan: %v", err)
	}
	if err := model.DB.Create(&model.ConfigReleaseTarget{
		PlanID:          plan.ID,
		ConfigVersionID: candidateVersion.ID,
		NodeID:          node.NodeID,
		PoolName:        "hk",
		Checksum:        candidateArtifact.Checksum,
		StageIndex:      1,
		Status:          ConfigReleaseTargetSucceeded,
		CompletedAt:     &completedAt,
	}).Error; err != nil {
		t.Fatalf("create expired succeeded target: %v", err)
	}

	if _, target, err := model.GetActiveConfigReleaseTargetForNodeID(node.NodeID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected expired succeeded target to be ignored, target=%+v err=%v", target, err)
	}
	meta, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode failed: %v", err)
	}
	if meta.Checksum != "active-pool-checksum" {
		t.Fatalf("expected active pool checksum after expired succeeded target, got %+v", meta)
	}
	view, err := GetNodeView(node.ID)
	if err != nil {
		t.Fatalf("GetNodeView failed: %v", err)
	}
	if !view.ConfigInSync || view.TargetConfigChecksum != "active-pool-checksum" {
		t.Fatalf("expected node view to be synced against active pool checksum, got %+v", view)
	}
}

func TestActiveConfigReleaseTargetKeepsRecentSucceededTargetDuringObserveWindow(t *testing.T) {
	setupServiceTestDB(t)

	node := seedConfigReleasePlanTestNode(t, "node-recent-succeeded-target", "hk")
	activeVersion := seedAgentTestActiveConfigVersionWithArtifacts(t, "20260613-observe-active", "global-active-checksum", map[string]string{
		"hk": "observe-active-checksum",
	})
	if _, _, err := ActivateConfigVersionForPool(activeVersion.ID, "hk", nil); err != nil {
		t.Fatalf("ActivateConfigVersionForPool failed: %v", err)
	}
	candidateVersion := &model.ConfigVersion{
		Version:        "20260613-observe-target",
		Checksum:       "observe-candidate-global",
		MainConfig:     "events {}",
		RenderedConfig: "server {}",
		CreatedBy:      "root",
	}
	if err := model.DB.Create(candidateVersion).Error; err != nil {
		t.Fatalf("create candidate config version: %v", err)
	}
	candidateArtifact := &model.ConfigVersionArtifact{
		ConfigVersionID:  candidateVersion.ID,
		PoolName:         "hk",
		Checksum:         "observe-target-checksum",
		RenderedConfig:   "server { # observe }",
		SupportFilesJSON: "[]",
	}
	if err := model.DB.Create(candidateArtifact).Error; err != nil {
		t.Fatalf("create candidate artifact: %v", err)
	}
	completedAt := time.Now()
	plan := &model.ConfigReleasePlan{
		ConfigVersionID: candidateVersion.ID,
		Status:          ConfigReleaseStatusRunning,
		CanaryPoolName:  "hk",
		ObserveSeconds:  120,
		Checksum:        candidateArtifact.Checksum,
		CreatedBy:       "root",
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("create running plan: %v", err)
	}
	target := &model.ConfigReleaseTarget{
		PlanID:          plan.ID,
		ConfigVersionID: candidateVersion.ID,
		NodeID:          node.NodeID,
		PoolName:        "hk",
		Checksum:        candidateArtifact.Checksum,
		StageIndex:      1,
		Status:          ConfigReleaseTargetSucceeded,
		CompletedAt:     &completedAt,
	}
	if err := model.DB.Create(target).Error; err != nil {
		t.Fatalf("create recent succeeded target: %v", err)
	}

	activePlan, activeTarget, err := model.GetActiveConfigReleaseTargetForNodeID(node.NodeID)
	if err != nil {
		t.Fatalf("GetActiveConfigReleaseTargetForNodeID failed: %v", err)
	}
	if activePlan.ID != plan.ID || activeTarget.ID != target.ID {
		t.Fatalf("expected recent succeeded target during observe window, got plan=%+v target=%+v", activePlan, activeTarget)
	}
	meta, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		t.Fatalf("GetActiveConfigMetaForAgentNode failed: %v", err)
	}
	if meta.Checksum != candidateArtifact.Checksum {
		t.Fatalf("expected observe target checksum, got %+v", meta)
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

type releasePlanSQLCaptureLogger struct {
	mu      sync.Mutex
	queries []string
}

func (capture *releasePlanSQLCaptureLogger) LogMode(logger.LogLevel) logger.Interface {
	return capture
}

func (capture *releasePlanSQLCaptureLogger) Info(context.Context, string, ...interface{}) {
}

func (capture *releasePlanSQLCaptureLogger) Warn(context.Context, string, ...interface{}) {
}

func (capture *releasePlanSQLCaptureLogger) Error(context.Context, string, ...interface{}) {
}

func (capture *releasePlanSQLCaptureLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	capture.mu.Lock()
	capture.queries = append(capture.queries, sql)
	capture.mu.Unlock()
}

func (capture *releasePlanSQLCaptureLogger) snapshot() []string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	queries := make([]string, len(capture.queries))
	copy(queries, capture.queries)
	return queries
}

func captureConfigReleasePlanSQL(t *testing.T, fn func() error) ([]string, error) {
	t.Helper()
	capture := &releasePlanSQLCaptureLogger{}
	originalDB := model.DB
	model.DB = model.DB.Session(&gorm.Session{Logger: capture})
	defer func() {
		model.DB = originalDB
	}()
	err := fn()
	return capture.snapshot(), err
}

func configReleasePlanNodeSelectQueries(queries []string) []string {
	nodeQueries := make([]string, 0)
	for _, query := range queries {
		normalizedQuery := strings.ToLower(query)
		if strings.Contains(normalizedQuery, "from `nodes`") ||
			strings.Contains(normalizedQuery, `from "nodes"`) ||
			strings.Contains(normalizedQuery, "from nodes") {
			nodeQueries = append(nodeQueries, query)
		}
	}
	return nodeQueries
}
