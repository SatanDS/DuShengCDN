package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type ConfigReleasePlan struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	ConfigVersionID   uint       `json:"config_version_id" gorm:"not null;index"`
	RollbackVersionID *uint      `json:"rollback_version_id" gorm:"index"`
	Status            string     `json:"status" gorm:"size:32;not null;default:'draft';index"`
	Strategy          string     `json:"strategy" gorm:"size:32;not null;default:'canary'"`
	CanaryPoolName    string     `json:"canary_pool_name" gorm:"size:64;not null;default:'default';index"`
	CurrentStage      int        `json:"current_stage" gorm:"not null;default:0"`
	CanaryPercent     int        `json:"canary_percent" gorm:"not null;default:1"`
	ObserveSeconds    int        `json:"observe_seconds" gorm:"not null;default:120"`
	Checksum          string     `json:"checksum" gorm:"size:64;not null;default:'';index"`
	FailureReason     string     `json:"failure_reason" gorm:"type:text"`
	CreatedBy         string     `json:"created_by" gorm:"size:64;not null;default:''"`
	StartedAt         *time.Time `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	FailedAt          *time.Time `json:"failed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type ConfigReleaseTarget struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	PlanID          uint       `json:"plan_id" gorm:"not null;index;uniqueIndex:idx_config_release_target_plan_node,priority:1"`
	ConfigVersionID uint       `json:"config_version_id" gorm:"not null;index"`
	NodeID          string     `json:"node_id" gorm:"size:64;not null;index;uniqueIndex:idx_config_release_target_plan_node,priority:2"`
	PoolName        string     `json:"pool_name" gorm:"size:64;not null;default:'default';index"`
	Checksum        string     `json:"checksum" gorm:"size:64;not null;default:'';index"`
	StageIndex      int        `json:"stage_index" gorm:"not null;default:0;index"`
	Status          string     `json:"status" gorm:"size:32;not null;default:'pending';index"`
	FailureReason   string     `json:"failure_reason" gorm:"type:text"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ConfigReleaseBlockedChecksum struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	PoolName        string     `json:"pool_name" gorm:"size:64;not null;default:'default';uniqueIndex:idx_config_release_blocked_pool_checksum,priority:1;index"`
	ConfigVersionID uint       `json:"config_version_id" gorm:"not null;index"`
	PlanID          *uint      `json:"plan_id" gorm:"index"`
	Version         string     `json:"version" gorm:"size:32;not null;default:''"`
	Checksum        string     `json:"checksum" gorm:"size:64;not null;uniqueIndex:idx_config_release_blocked_pool_checksum,priority:2;index"`
	Reason          string     `json:"reason" gorm:"type:text"`
	ExpiresAt       *time.Time `json:"expires_at" gorm:"index"`
	UnblockedAt     *time.Time `json:"unblocked_at" gorm:"index"`
	UnblockedBy     string     `json:"unblocked_by" gorm:"size:64;not null;default:''"`
	UnblockReason   string     `json:"unblock_reason" gorm:"type:text;not null;default:''"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ConfigReleaseBlockedChecksumAudit struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	BlockedChecksumID uint      `json:"blocked_checksum_id" gorm:"not null;index"`
	PoolName          string    `json:"pool_name" gorm:"size:64;not null;default:'default';index"`
	Checksum          string    `json:"checksum" gorm:"size:64;not null;index"`
	Action            string    `json:"action" gorm:"size:32;not null;default:'';index"`
	Operator          string    `json:"operator" gorm:"size:64;not null;default:''"`
	OriginalReason    string    `json:"original_reason" gorm:"type:text;not null;default:''"`
	Reason            string    `json:"reason" gorm:"type:text;not null;default:''"`
	CreatedAt         time.Time `json:"created_at"`
}

func ListConfigReleasePlans() (plans []*ConfigReleasePlan, err error) {
	err = DB.Order("id desc").Find(&plans).Error
	return plans, err
}

func GetConfigReleasePlanByID(id uint) (*ConfigReleasePlan, error) {
	plan := &ConfigReleasePlan{}
	err := DB.First(plan, id).Error
	return plan, err
}

func ListConfigReleaseTargets(planID uint) (targets []*ConfigReleaseTarget, err error) {
	err = DB.Where("plan_id = ?", planID).Order("stage_index asc").Order("id asc").Find(&targets).Error
	return targets, err
}

func GetActiveConfigReleaseTargetForNodeID(nodeID string) (*ConfigReleasePlan, *ConfigReleaseTarget, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, gorm.ErrRecordNotFound
	}
	target, err := firstActiveConfigReleaseTargetForNodeID(nodeID, []string{"pending", "applying", "observing"})
	if err == nil {
		plan, err := GetConfigReleasePlanByID(target.PlanID)
		if err != nil {
			return nil, nil, err
		}
		return plan, target, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, err
	}

	targets, err := listActiveConfigReleaseTargetsForNodeID(nodeID, []string{"succeeded"})
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	for _, target := range targets {
		plan, err := GetConfigReleasePlanByID(target.PlanID)
		if err != nil {
			return nil, nil, err
		}
		ok, err := succeededConfigReleaseTargetStillActive(plan, target, now)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			return plan, target, nil
		}
	}
	return nil, nil, gorm.ErrRecordNotFound
}

func firstActiveConfigReleaseTargetForNodeID(nodeID string, statuses []string) (*ConfigReleaseTarget, error) {
	target := &ConfigReleaseTarget{}
	result := DB.Model(&ConfigReleaseTarget{}).
		Select("config_release_targets.*").
		Joins("JOIN config_release_plans ON config_release_plans.id = config_release_targets.plan_id").
		Where("config_release_targets.node_id = ? AND config_release_targets.status IN ?", nodeID, statuses).
		Where("config_release_plans.status IN ?", []string{"running", "observing"}).
		Order("config_release_targets.id desc").
		Limit(1).
		First(target)
	if result.Error != nil {
		return nil, result.Error
	}
	return target, nil
}

func listActiveConfigReleaseTargetsForNodeID(nodeID string, statuses []string) ([]*ConfigReleaseTarget, error) {
	var targets []*ConfigReleaseTarget
	err := DB.Model(&ConfigReleaseTarget{}).
		Select("config_release_targets.*").
		Joins("JOIN config_release_plans ON config_release_plans.id = config_release_targets.plan_id").
		Where("config_release_targets.node_id = ? AND config_release_targets.status IN ?", nodeID, statuses).
		Where("config_release_plans.status IN ?", []string{"running", "observing"}).
		Order("config_release_targets.id desc").
		Find(&targets).Error
	return targets, err
}

func succeededConfigReleaseTargetStillActive(plan *ConfigReleasePlan, target *ConfigReleaseTarget, now time.Time) (bool, error) {
	if plan == nil || target == nil {
		return false, nil
	}
	if target.CompletedAt != nil && plan.ObserveSeconds > 0 {
		expiresAt := target.CompletedAt.Add(time.Duration(plan.ObserveSeconds) * time.Second)
		if !expiresAt.Before(now) {
			return true, nil
		}
	}
	var activeTargetCount int64
	err := DB.Model(&ConfigReleaseTarget{}).
		Where("plan_id = ? AND status IN ?", plan.ID, []string{"pending", "applying", "observing"}).
		Count(&activeTargetCount).Error
	if err != nil {
		return false, err
	}
	return activeTargetCount > 0, nil
}

func CountActiveConfigReleasePlans(excludeID uint) (int64, error) {
	var count int64
	query := DB.Model(&ConfigReleasePlan{}).Where("status IN ?", []string{"running", "observing"})
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Count(&count).Error
	return count, err
}

func CountActiveConfigReleasePlansByPool(poolName string, excludeID uint) (int64, error) {
	var count int64
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		poolName = "default"
	}
	query := DB.Model(&ConfigReleasePlan{}).
		Where("status IN ?", []string{"running", "observing"}).
		Where("canary_pool_name = ?", poolName)
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	err := query.Count(&count).Error
	return count, err
}

func GetConfigReleaseBlockedChecksum(checksum string) (*ConfigReleaseBlockedChecksum, error) {
	blocked := &ConfigReleaseBlockedChecksum{}
	err := DB.Where("checksum = ? AND unblocked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", strings.TrimSpace(checksum), time.Now()).
		Order("id desc").
		First(blocked).Error
	return blocked, err
}

func GetConfigReleaseBlockedChecksumForPool(poolName string, checksum string) (*ConfigReleaseBlockedChecksum, error) {
	blocked := &ConfigReleaseBlockedChecksum{}
	err := DB.Where(
		"pool_name = ? AND checksum = ? AND unblocked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)",
		strings.TrimSpace(poolName),
		strings.TrimSpace(checksum),
		time.Now(),
	).First(blocked).Error
	return blocked, err
}

func GetConfigReleaseBlockedChecksumByID(id uint) (*ConfigReleaseBlockedChecksum, error) {
	blocked := &ConfigReleaseBlockedChecksum{}
	err := DB.First(blocked, id).Error
	return blocked, err
}

func ListConfigReleaseBlockedChecksums(includeUnblocked bool) ([]*ConfigReleaseBlockedChecksum, error) {
	var blocked []*ConfigReleaseBlockedChecksum
	query := DB.Order("id desc")
	if !includeUnblocked {
		query = query.Where("unblocked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", time.Now())
	}
	err := query.Find(&blocked).Error
	return blocked, err
}
