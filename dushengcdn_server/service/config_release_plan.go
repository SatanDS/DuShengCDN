package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ConfigReleaseStatusDraft     = "draft"
	ConfigReleaseStatusRunning   = "running"
	ConfigReleaseStatusObserving = "observing"
	ConfigReleaseStatusFailed    = "failed"
	ConfigReleaseStatusCompleted = "completed"
	ConfigReleaseStatusCanceled  = "canceled"

	ConfigReleaseTargetPending    = "pending"
	ConfigReleaseTargetApplying   = "applying"
	ConfigReleaseTargetObserving  = "observing"
	ConfigReleaseTargetSucceeded  = "succeeded"
	ConfigReleaseTargetFailed     = "failed"
	ConfigReleaseTargetRolledBack = "rolled_back"
)

type ConfigReleasePlanInput struct {
	ConfigVersionID *uint  `json:"config_version_id"`
	NodePool        string `json:"node_pool"`
	CanaryPercent   int    `json:"canary_percent"`
	ObserveSeconds  int    `json:"observe_seconds"`
	Force           bool   `json:"force"`
}

type ConfigReleasePlanView struct {
	Plan    *model.ConfigReleasePlan     `json:"plan"`
	Targets []*model.ConfigReleaseTarget `json:"targets"`
}

type ConfigReleasePlanEvaluation struct {
	Plan             *model.ConfigReleasePlan `json:"plan"`
	Healthy          bool                     `json:"healthy"`
	Reason           string                   `json:"reason,omitempty"`
	TargetCount      int                      `json:"target_count"`
	SucceededTargets int                      `json:"succeeded_targets"`
	FailedTargets    int                      `json:"failed_targets"`
}

type ConfigReleaseBlockedChecksumUnblockInput struct {
	Reason string `json:"reason"`
}

type configReleaseForceSyncNotification struct {
	NodeID   string
	Version  string
	Checksum string
}

func ListConfigReleasePlans() ([]*model.ConfigReleasePlan, error) {
	return model.ListConfigReleasePlans()
}

func ListConfigReleaseBlockedChecksums(includeUnblocked bool) ([]*model.ConfigReleaseBlockedChecksum, error) {
	return model.ListConfigReleaseBlockedChecksums(includeUnblocked)
}

func UnblockConfigReleaseBlockedChecksum(id uint, operator string, input ConfigReleaseBlockedChecksumUnblockInput) (*model.ConfigReleaseBlockedChecksum, error) {
	blocked, err := model.GetConfigReleaseBlockedChecksumByID(id)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, errors.New("unblock reason is required")
	}
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "system"
	}
	now := time.Now()
	originalReason := strings.TrimSpace(blocked.Reason)
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(blocked).Updates(map[string]any{
			"unblocked_at":   &now,
			"unblocked_by":   operator,
			"unblock_reason": reason,
		}).Error; err != nil {
			return err
		}
		audit := &model.ConfigReleaseBlockedChecksumAudit{
			BlockedChecksumID: blocked.ID,
			PoolName:          blocked.PoolName,
			Checksum:          blocked.Checksum,
			Action:            "unblock",
			Operator:          operator,
			OriginalReason:    originalReason,
			Reason:            reason,
		}
		return tx.Create(audit).Error
	}); err != nil {
		return nil, err
	}
	return model.GetConfigReleaseBlockedChecksumByID(id)
}

func GetConfigReleasePlan(id uint) (*ConfigReleasePlanView, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	targets, err := model.ListConfigReleaseTargets(plan.ID)
	if err != nil {
		return nil, err
	}
	return &ConfigReleasePlanView{Plan: plan, Targets: targets}, nil
}

func configReleasePlanActive(plan *model.ConfigReleasePlan) bool {
	return plan != nil && (plan.Status == ConfigReleaseStatusRunning || plan.Status == ConfigReleaseStatusObserving)
}

func ensureConfigReleaseChecksumNotBlocked(checksum string) error {
	blocked, err := model.GetConfigReleaseBlockedChecksum(checksum)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return fmt.Errorf("config checksum %s is blocked by failed release plan: %s", checksum, strings.TrimSpace(blocked.Reason))
}

func ensureConfigReleaseChecksumNotBlockedForPool(poolName string, checksum string) error {
	poolName = normalizeConfigReleasePoolName(poolName)
	blocked, err := model.GetConfigReleaseBlockedChecksumForPool(poolName, checksum)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return fmt.Errorf("config checksum %s is blocked for node pool %s by failed release plan: %s", checksum, poolName, strings.TrimSpace(blocked.Reason))
}

func ensureNoActiveConfigReleasePlan(excludeID uint) error {
	count, err := model.CountActiveConfigReleasePlans(excludeID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("another config release plan is already in progress")
	}
	return nil
}

func ensureNoActiveConfigReleasePlanForPool(poolName string, excludeID uint) error {
	poolName = normalizeConfigReleasePoolName(poolName)
	count, err := model.CountActiveConfigReleasePlansByPool(poolName, excludeID)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("node pool %s already has a config release plan in progress", poolName)
	}
	return nil
}

func normalizeConfigReleasePoolName(poolName string) string {
	poolName = normalizeNodePoolName(poolName)
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	return poolName
}

// summarizeConfigReleasePlan reports the stored state of a plan that is not
// actively releasing (draft, completed or failed) without mutating targets.
func summarizeConfigReleasePlan(plan *model.ConfigReleasePlan, targets []*model.ConfigReleaseTarget) *ConfigReleasePlanEvaluation {
	evaluation := &ConfigReleasePlanEvaluation{Plan: plan, Healthy: true, TargetCount: len(targets)}
	for _, target := range targets {
		switch target.Status {
		case ConfigReleaseTargetSucceeded:
			evaluation.SucceededTargets++
		case ConfigReleaseTargetFailed, ConfigReleaseTargetRolledBack:
			evaluation.FailedTargets++
		}
	}
	if plan.Status == ConfigReleaseStatusFailed {
		evaluation.Healthy = false
		evaluation.Reason = strings.TrimSpace(plan.FailureReason)
	}
	return evaluation
}

func logConfigReleaseTargetUpdateError(err error, target *model.ConfigReleaseTarget) {
	if err != nil {
		slog.Error("update config release target failed", "target_id", target.ID, "node_id", target.NodeID, "error", err)
	}
}

func CreateConfigReleasePlan(createdBy string, input ConfigReleasePlanInput) (*ConfigReleasePlanView, error) {
	nodePool := normalizeNodePoolName(input.NodePool)
	if nodePool == "" {
		nodePool = normalizeNodePoolName("default")
	}
	observeSeconds := input.ObserveSeconds
	if observeSeconds <= 0 {
		observeSeconds = 120
	}
	canaryPercent := input.CanaryPercent
	if canaryPercent <= 0 {
		canaryPercent = 20
	}
	if canaryPercent > 100 {
		canaryPercent = 100
	}

	if err := ensureNoActiveConfigReleasePlanForPool(nodePool, 0); err != nil {
		return nil, err
	}

	version, err := releasePlanVersion(createdBy, input)
	if err != nil {
		return nil, err
	}
	if err := ensureConfigReleaseChecksumNotBlockedForPool(nodePool, version.Checksum); err != nil {
		return nil, err
	}
	if err := ensureConfigVersionArtifactsForPools(version, []string{nodePool}); err != nil {
		return nil, err
	}
	artifact, err := model.GetConfigVersionArtifact(version.ID, nodePool)
	if err != nil {
		return nil, err
	}
	// Failed plans block the pool-level artifact checksum, which only matches
	// the version checksum when every route lives in that pool.
	if artifact.Checksum != version.Checksum {
		if err := ensureConfigReleaseChecksumNotBlockedForPool(nodePool, artifact.Checksum); err != nil {
			return nil, err
		}
	}
	if activeVersion, activeArtifact, err := model.GetActiveConfigVersionArtifactForPool(nodePool); err == nil {
		if activeVersion.ID != 0 && activeArtifact.Checksum == artifact.Checksum && !input.Force {
			return nil, fmt.Errorf("node pool %s has no config changes to release", nodePool)
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	activeVersion, err := activeVersionForReleasePlanRollback(nodePool)
	var rollbackVersionID *uint
	if err == nil && activeVersion.ID != version.ID {
		rollbackVersionID = &activeVersion.ID
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	targetNodes, err := selectReleasePlanNodes(nodePool, 1)
	if err != nil {
		return nil, err
	}
	if len(targetNodes) == 0 {
		return nil, fmt.Errorf("node pool %s has no available nodes for canary release", nodePool)
	}
	plan := &model.ConfigReleasePlan{
		ConfigVersionID:   version.ID,
		RollbackVersionID: rollbackVersionID,
		Status:            ConfigReleaseStatusDraft,
		Strategy:          "canary",
		CanaryPoolName:    nodePool,
		CanaryPercent:     canaryPercent,
		ObserveSeconds:    observeSeconds,
		Checksum:          artifact.Checksum,
		CreatedBy:         strings.TrimSpace(createdBy),
	}
	targets := make([]*model.ConfigReleaseTarget, 0, len(targetNodes))
	for _, node := range targetNodes {
		targets = append(targets, &model.ConfigReleaseTarget{
			ConfigVersionID: version.ID,
			NodeID:          node.NodeID,
			PoolName:        nodePool,
			Checksum:        artifact.Checksum,
			StageIndex:      1,
			Status:          ConfigReleaseTargetPending,
		})
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		for _, target := range targets {
			target.PlanID = plan.ID
			if err := tx.Create(target).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return GetConfigReleasePlan(plan.ID)
}

func StartConfigReleasePlan(id uint) (*ConfigReleasePlanView, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	switch plan.Status {
	case ConfigReleaseStatusDraft:
	case ConfigReleaseStatusRunning, ConfigReleaseStatusObserving:
		return GetConfigReleasePlan(id)
	default:
		return nil, fmt.Errorf("release plan %d cannot be started from status %s", id, plan.Status)
	}
	if err := ensureNoActiveConfigReleasePlanForPool(plan.CanaryPoolName, plan.ID); err != nil {
		return nil, err
	}
	version, err := model.GetConfigVersionByID(plan.ConfigVersionID)
	if err != nil {
		return nil, err
	}
	targets, err := model.ListConfigReleaseTargets(plan.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	notifications := make([]configReleaseForceSyncNotification, 0, len(targets))
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(plan).Updates(map[string]any{
			"status":        ConfigReleaseStatusRunning,
			"current_stage": 1,
			"started_at":    &now,
		}).Error; err != nil {
			return err
		}
		for _, target := range targets {
			if target.StageIndex != 1 || target.Status != ConfigReleaseTargetPending {
				continue
			}
			if err := tx.Model(target).Updates(map[string]any{
				"status":     ConfigReleaseTargetApplying,
				"started_at": &now,
			}).Error; err != nil {
				return err
			}
			notifications = append(notifications, configReleaseForceSyncNotification{
				NodeID:   target.NodeID,
				Version:  version.Version,
				Checksum: target.Checksum,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sendConfigReleaseForceSyncNotifications(notifications)
	return GetConfigReleasePlan(id)
}

func EvaluateConfigReleasePlan(id uint) (*ConfigReleasePlanEvaluation, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	targets, err := model.ListConfigReleaseTargets(plan.ID)
	if err != nil {
		return nil, err
	}
	// Draft plans have not pushed anything yet and terminal plans must stay
	// terminal: evaluating them is read-only and never fails the plan or
	// rolls nodes back.
	if !configReleasePlanActive(plan) {
		return summarizeConfigReleasePlan(plan, targets), nil
	}
	nodes, err := model.ListNodesByNodeIDs(configReleaseTargetNodeIDs(targets))
	if err != nil {
		// A transient DB error must not be mistaken for missing nodes.
		return nil, err
	}
	nodesByID := make(map[string]*model.Node, len(nodes))
	for _, node := range nodes {
		nodesByID[strings.TrimSpace(node.NodeID)] = node
	}
	evaluation := &ConfigReleasePlanEvaluation{Plan: plan, Healthy: true, TargetCount: len(targets)}
	now := time.Now()
	for _, target := range targets {
		switch target.Status {
		case ConfigReleaseTargetSucceeded:
			evaluation.SucceededTargets++
			continue
		case ConfigReleaseTargetFailed, ConfigReleaseTargetRolledBack:
			evaluation.Healthy = false
			evaluation.FailedTargets++
			if evaluation.Reason == "" {
				evaluation.Reason = strings.TrimSpace(target.FailureReason)
			}
			continue
		}
		node, ok := nodesByID[strings.TrimSpace(target.NodeID)]
		if !ok {
			evaluation.Healthy = false
			evaluation.FailedTargets++
			evaluation.Reason = fmt.Sprintf("node %s is missing", target.NodeID)
			logConfigReleaseTargetUpdateError(markConfigReleaseTargetFailed(target, evaluation.Reason), target)
			continue
		}
		if computeNodeStatus(node) != NodeStatusOnline {
			evaluation.Healthy = false
			evaluation.FailedTargets++
			evaluation.Reason = fmt.Sprintf("node %s is offline", target.NodeID)
			logConfigReleaseTargetUpdateError(markConfigReleaseTargetFailed(target, evaluation.Reason), target)
			continue
		}
		if strings.TrimSpace(node.CurrentChecksum) == strings.TrimSpace(target.Checksum) {
			evaluation.SucceededTargets++
			logConfigReleaseTargetUpdateError(model.DB.Model(target).Updates(map[string]any{
				"status":       ConfigReleaseTargetSucceeded,
				"completed_at": &now,
			}).Error, target)
			continue
		}
		if target.StartedAt != nil && time.Since(*target.StartedAt) > time.Duration(plan.ObserveSeconds)*time.Second {
			evaluation.Healthy = false
			evaluation.FailedTargets++
			evaluation.Reason = fmt.Sprintf("node %s did not apply checksum %s within %ds", target.NodeID, target.Checksum, plan.ObserveSeconds)
			logConfigReleaseTargetUpdateError(markConfigReleaseTargetFailed(target, evaluation.Reason), target)
			continue
		}
		if target.Status == ConfigReleaseTargetApplying {
			evaluation.Healthy = false
			evaluation.Reason = "release target is still applying"
		}
	}
	if evaluation.FailedTargets > 0 {
		if err := FailConfigReleasePlan(plan.ID, evaluation.Reason); err != nil {
			return nil, err
		}
		plan, _ = model.GetConfigReleasePlanByID(plan.ID)
		evaluation.Plan = plan
		evaluation.Healthy = false
	}
	return evaluation, nil
}

func AdvanceConfigReleasePlan(id uint) (*ConfigReleasePlanView, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	switch plan.Status {
	case ConfigReleaseStatusDraft:
		return StartConfigReleasePlan(id)
	case ConfigReleaseStatusRunning, ConfigReleaseStatusObserving:
	default:
		return nil, fmt.Errorf("release plan %d cannot be advanced from status %s", id, plan.Status)
	}
	evaluation, err := EvaluateConfigReleasePlan(id)
	if err != nil {
		return nil, err
	}
	if !evaluation.Healthy || evaluation.SucceededTargets < evaluation.TargetCount {
		return nil, errors.New("release plan is not healthy enough to advance")
	}
	plan = evaluation.Plan
	switch plan.CurrentStage {
	case 1:
		return expandConfigReleasePlan(plan, plan.CanaryPercent)
	default:
		return completeConfigReleasePlan(plan)
	}
}

func CompleteConfigReleasePlan(id uint) (*ConfigReleasePlanView, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	switch plan.Status {
	case ConfigReleaseStatusCompleted:
		return GetConfigReleasePlan(id)
	case ConfigReleaseStatusRunning, ConfigReleaseStatusObserving:
	default:
		return nil, fmt.Errorf("release plan %d cannot be completed from status %s", id, plan.Status)
	}
	evaluation, err := EvaluateConfigReleasePlan(id)
	if err != nil {
		return nil, err
	}
	if !evaluation.Healthy || evaluation.SucceededTargets < evaluation.TargetCount {
		return nil, errors.New("release plan is not healthy enough to complete")
	}
	return completeConfigReleasePlan(evaluation.Plan)
}

func FailConfigReleasePlan(id uint, reason string) error {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return err
	}
	switch plan.Status {
	case ConfigReleaseStatusRunning, ConfigReleaseStatusObserving:
	case ConfigReleaseStatusFailed:
		return nil
	case ConfigReleaseStatusCompleted:
		return fmt.Errorf("release plan %d is already completed", id)
	case ConfigReleaseStatusDraft:
		return errors.New("draft release plan should be canceled, not failed")
	case ConfigReleaseStatusCanceled:
		return fmt.Errorf("release plan %d is already canceled", id)
	default:
		return fmt.Errorf("release plan %d cannot fail from status %s", id, plan.Status)
	}
	targets, err := model.ListConfigReleaseTargets(plan.ID)
	if err != nil {
		return err
	}
	now := time.Now()
	version, versionErr := model.GetConfigVersionByID(plan.ConfigVersionID)
	if versionErr != nil {
		return versionErr
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "release plan failed"
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(plan).Updates(map[string]any{
			"status":         ConfigReleaseStatusFailed,
			"failure_reason": reason,
			"failed_at":      &now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ConfigReleaseTarget{}).
			Where("plan_id = ? AND status IN ?", plan.ID, []string{ConfigReleaseTargetPending, ConfigReleaseTargetApplying, ConfigReleaseTargetObserving, ConfigReleaseTargetSucceeded}).
			Updates(map[string]any{"status": ConfigReleaseTargetRolledBack, "failure_reason": reason, "completed_at": &now}).Error; err != nil {
			return err
		}
		blocked := &model.ConfigReleaseBlockedChecksum{
			PoolName:        normalizeConfigReleasePoolName(plan.CanaryPoolName),
			ConfigVersionID: version.ID,
			PlanID:          &plan.ID,
			Version:         version.Version,
			Checksum:        plan.Checksum,
			Reason:          reason,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "pool_name"}, {Name: "checksum"}},
			DoUpdates: clause.Assignments(map[string]any{
				"config_version_id": blocked.ConfigVersionID,
				"plan_id":           blocked.PlanID,
				"version":           blocked.Version,
				"reason":            blocked.Reason,
				"expires_at":        gorm.Expr("NULL"),
				"unblocked_at":      gorm.Expr("NULL"),
				"unblocked_by":      "",
				"unblock_reason":    "",
				"updated_at":        now,
			}),
		}).Create(blocked).Error
	}); err != nil {
		return err
	}
	forceSyncConfigReleaseRollbackTargets(plan, targets)
	return nil
}

func CancelConfigReleasePlan(id uint) (*ConfigReleasePlanView, error) {
	plan, err := model.GetConfigReleasePlanByID(id)
	if err != nil {
		return nil, err
	}
	switch plan.Status {
	case ConfigReleaseStatusCanceled:
		return GetConfigReleasePlan(id)
	case ConfigReleaseStatusDraft:
	default:
		return nil, fmt.Errorf("release plan %d cannot be canceled from status %s", id, plan.Status)
	}
	now := time.Now()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(plan).Updates(map[string]any{
			"status":       ConfigReleaseStatusCanceled,
			"completed_at": &now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.ConfigReleaseTarget{}).
			Where("plan_id = ? AND status = ?", plan.ID, ConfigReleaseTargetPending).
			Updates(map[string]any{"status": ConfigReleaseTargetRolledBack, "completed_at": &now}).Error
	}); err != nil {
		return nil, err
	}
	return GetConfigReleasePlan(id)
}

func forceSyncConfigReleaseRollbackTargets(plan *model.ConfigReleasePlan, targets []*model.ConfigReleaseTarget) {
	if plan == nil || len(targets) == 0 {
		return
	}
	// All targets in the same pool roll back to the same version artifact;
	// resolve it once per pool instead of once per node.
	metaByPool := make(map[string]*ActiveConfigMeta, 1)
	for _, target := range targets {
		if target == nil || strings.TrimSpace(target.NodeID) == "" {
			continue
		}
		poolName := normalizeNodePoolName(target.PoolName)
		if poolName == "" {
			poolName = normalizeNodePoolName("default")
		}
		meta, cached := metaByPool[poolName]
		if !cached {
			var err error
			meta, err = rollbackConfigMetaForReleaseTarget(plan, target)
			if err != nil {
				slog.Error("resolve rollback config for release plan failed", "plan_id", plan.ID, "pool", poolName, "error", err)
				meta = nil
			}
			metaByPool[poolName] = meta
		}
		if meta == nil {
			continue
		}
		SendAgentWSForceSyncConfig(target.NodeID, meta)
	}
}

func rollbackConfigMetaForReleaseTarget(plan *model.ConfigReleasePlan, target *model.ConfigReleaseTarget) (*ActiveConfigMeta, error) {
	if plan == nil || target == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var version *model.ConfigVersion
	var err error
	if plan.RollbackVersionID != nil && *plan.RollbackVersionID != 0 {
		version, err = model.GetConfigVersionByID(*plan.RollbackVersionID)
	} else {
		version, err = activeVersionForReleasePlanRollback(target.PoolName)
	}
	if err != nil {
		return nil, err
	}
	poolName := normalizeNodePoolName(target.PoolName)
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	artifact, err := model.GetConfigVersionArtifact(version.ID, poolName)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		if ensureErr := ensureConfigVersionArtifactsForPools(version, []string{poolName}); ensureErr != nil {
			return nil, ensureErr
		}
		artifact, err = model.GetConfigVersionArtifact(version.ID, poolName)
	}
	if err != nil {
		return nil, err
	}
	return &ActiveConfigMeta{Version: version.Version, Checksum: artifact.Checksum}, nil
}

func activeVersionForReleasePlanRollback(poolName string) (*model.ConfigVersion, error) {
	poolName = normalizeConfigReleasePoolName(poolName)
	version, _, err := model.GetActiveConfigVersionArtifactForPool(poolName)
	if err == nil {
		return version, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return model.GetActiveConfigVersion()
}

func releasePlanVersion(createdBy string, input ConfigReleasePlanInput) (*model.ConfigVersion, error) {
	if input.ConfigVersionID != nil && *input.ConfigVersionID != 0 {
		return model.GetConfigVersionByID(*input.ConfigVersionID)
	}
	result, err := CreateInactiveConfigVersion(createdBy, input.Force)
	if err != nil {
		return nil, err
	}
	return result.Version, nil
}

func selectReleasePlanNodes(poolName string, limit int) ([]*model.Node, error) {
	poolName = normalizeConfigReleasePoolName(poolName)
	nodes, err := model.ListOnlineNodesByPool(
		poolName,
		time.Now().Add(-common.NodeOfflineThreshold),
		uniqueAgentWSClientNodeIDs(snapshotAgentWSClients()),
	)
	if err != nil {
		return nil, err
	}
	selected := make([]*model.Node, 0, len(nodes))
	for _, node := range nodes {
		if normalizeNodePoolName(node.PoolName) != poolName {
			continue
		}
		if computeNodeStatus(node) != NodeStatusOnline {
			continue
		}
		selected = append(selected, node)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].LastSeenAt.After(selected[j].LastSeenAt)
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	return selected, nil
}

func configReleaseTargetNodeIDs(targets []*model.ConfigReleaseTarget) []string {
	seen := make(map[string]struct{}, len(targets))
	nodeIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}
		nodeID := strings.TrimSpace(target.NodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	return nodeIDs
}

func expandConfigReleasePlan(plan *model.ConfigReleasePlan, percent int) (*ConfigReleasePlanView, error) {
	version, err := model.GetConfigVersionByID(plan.ConfigVersionID)
	if err != nil {
		return nil, err
	}
	nodes, err := selectReleasePlanNodes(plan.CanaryPoolName, 0)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("node pool %s has no online nodes", plan.CanaryPoolName)
	}
	desired := int(math.Ceil(float64(len(nodes)) * float64(percent) / 100.0))
	if desired < 1 {
		desired = 1
	}
	existingTargets, err := model.ListConfigReleaseTargets(plan.ID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(existingTargets))
	for _, target := range existingTargets {
		existing[target.NodeID] = struct{}{}
	}
	artifact, err := model.GetConfigVersionArtifact(version.ID, plan.CanaryPoolName)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	newTargets := make([]*model.ConfigReleaseTarget, 0)
	for _, node := range nodes {
		if _, ok := existing[node.NodeID]; ok {
			continue
		}
		newTargets = append(newTargets, &model.ConfigReleaseTarget{
			PlanID:          plan.ID,
			ConfigVersionID: version.ID,
			NodeID:          node.NodeID,
			PoolName:        plan.CanaryPoolName,
			Checksum:        artifact.Checksum,
			StageIndex:      2,
			Status:          ConfigReleaseTargetApplying,
			StartedAt:       &now,
		})
		if len(existing)+len(newTargets) >= desired {
			break
		}
	}
	if len(newTargets) == 0 {
		return completeConfigReleasePlan(plan)
	}
	notifications := make([]configReleaseForceSyncNotification, 0, len(newTargets))
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(plan).Updates(map[string]any{
			"status":        ConfigReleaseStatusRunning,
			"current_stage": 2,
		}).Error; err != nil {
			return err
		}
		for _, target := range newTargets {
			if err := tx.Create(target).Error; err != nil {
				return err
			}
			notifications = append(notifications, configReleaseForceSyncNotification{
				NodeID:   target.NodeID,
				Version:  version.Version,
				Checksum: target.Checksum,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sendConfigReleaseForceSyncNotifications(notifications)
	return GetConfigReleasePlan(plan.ID)
}

func sendConfigReleaseForceSyncNotifications(notifications []configReleaseForceSyncNotification) {
	for _, notification := range notifications {
		if strings.TrimSpace(notification.NodeID) == "" {
			continue
		}
		SendAgentWSForceSyncConfig(notification.NodeID, &ActiveConfigMeta{
			Version:  notification.Version,
			Checksum: notification.Checksum,
		})
	}
}

func completeConfigReleasePlan(plan *model.ConfigReleasePlan) (*ConfigReleasePlanView, error) {
	version, artifact, err := ActivateConfigVersionForPool(plan.ConfigVersionID, plan.CanaryPoolName, &plan.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := model.DB.Model(plan).Updates(map[string]any{
		"status":        ConfigReleaseStatusCompleted,
		"current_stage": 3,
		"completed_at":  &now,
		"checksum":      artifact.Checksum,
	}).Error; err != nil {
		return nil, err
	}
	BroadcastAgentWSActiveConfigForPool(version, artifact.PoolName)
	return GetConfigReleasePlan(plan.ID)
}

func markConfigReleaseTargetFailed(target *model.ConfigReleaseTarget, reason string) error {
	now := time.Now()
	return model.DB.Model(target).Updates(map[string]any{
		"status":         ConfigReleaseTargetFailed,
		"failure_reason": reason,
		"completed_at":   &now,
	}).Error
}

func updateConfigReleaseTargetFromApplyLog(payload ApplyLogPayload) {
	plan, target, err := model.GetActiveConfigReleaseTargetForNodeID(payload.NodeID)
	if err != nil || plan == nil || target == nil {
		return
	}
	if checksum := strings.TrimSpace(payload.Checksum); checksum != "" {
		if checksum != strings.TrimSpace(target.Checksum) {
			return
		}
	} else {
		// Without a checksum the apply log can only be attributed to this
		// plan via its version; ignore reports about other configs.
		version, err := model.GetConfigVersionByID(plan.ConfigVersionID)
		if err != nil || strings.TrimSpace(payload.Version) != strings.TrimSpace(version.Version) {
			return
		}
	}
	now := time.Now()
	switch payload.Result {
	case ApplyResultOK:
		_ = model.DB.Model(target).Updates(map[string]any{
			"status":       ConfigReleaseTargetSucceeded,
			"completed_at": &now,
		}).Error
	case ApplyResultWarning, ApplyResultFailed:
		reason := strings.TrimSpace(payload.Message)
		if reason == "" {
			reason = fmt.Sprintf("node %s reported %s while applying canary config", payload.NodeID, payload.Result)
		}
		_ = markConfigReleaseTargetFailed(target, reason)
		_ = FailConfigReleasePlan(plan.ID, reason)
	}
}

func evaluateConfigReleaseTargetFromHeartbeat(node *model.Node) {
	if node == nil {
		return
	}
	plan, target, err := model.GetActiveConfigReleaseTargetForNodeID(node.NodeID)
	if err != nil || plan == nil || target == nil {
		return
	}
	if strings.TrimSpace(node.CurrentChecksum) == strings.TrimSpace(target.Checksum) && target.Status != ConfigReleaseTargetSucceeded {
		now := time.Now()
		_ = model.DB.Model(target).Updates(map[string]any{
			"status":       ConfigReleaseTargetSucceeded,
			"completed_at": &now,
		}).Error
		return
	}
	if target.Status != ConfigReleaseTargetSucceeded &&
		target.StartedAt != nil &&
		time.Since(*target.StartedAt) > time.Duration(plan.ObserveSeconds)*time.Second {
		reason := fmt.Sprintf("node %s did not report checksum %s within %ds", target.NodeID, target.Checksum, plan.ObserveSeconds)
		_ = markConfigReleaseTargetFailed(target, reason)
		_ = FailConfigReleasePlan(plan.ID, reason)
	}
}
