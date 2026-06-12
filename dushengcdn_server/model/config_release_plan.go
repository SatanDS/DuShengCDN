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
	ID              uint      `json:"id" gorm:"primaryKey"`
	ConfigVersionID uint      `json:"config_version_id" gorm:"not null;index"`
	PlanID          *uint     `json:"plan_id" gorm:"index"`
	Version         string    `json:"version" gorm:"size:32;not null;default:''"`
	Checksum        string    `json:"checksum" gorm:"size:64;not null;uniqueIndex"`
	Reason          string    `json:"reason" gorm:"type:text"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	target := &ConfigReleaseTarget{}
	err := DB.Where("node_id = ? AND status IN ?", nodeID, []string{"pending", "applying", "observing", "succeeded"}).
		Order("id desc").
		First(target).Error
	if err != nil {
		return nil, nil, err
	}
	plan := &ConfigReleasePlan{}
	if err = DB.Where("id = ? AND status IN ?", target.PlanID, []string{"running", "observing"}).First(plan).Error; err != nil {
		return nil, nil, err
	}
	return plan, target, nil
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

func GetConfigReleaseBlockedChecksum(checksum string) (*ConfigReleaseBlockedChecksum, error) {
	blocked := &ConfigReleaseBlockedChecksum{}
	err := DB.Where("checksum = ?", strings.TrimSpace(checksum)).First(blocked).Error
	return blocked, err
}
