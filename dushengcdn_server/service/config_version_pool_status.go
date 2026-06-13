package service

import (
	"errors"
	"fmt"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

type ActiveConfigPoolStatus struct {
	PoolName            string    `json:"pool_name" gorm:"column:pool_name"`
	Version             string    `json:"version,omitempty" gorm:"column:version"`
	ConfigVersionID     uint      `json:"config_version_id" gorm:"column:config_version_id"`
	ArtifactID          uint      `json:"artifact_id" gorm:"column:artifact_id"`
	Checksum            string    `json:"checksum" gorm:"column:checksum"`
	VersionChecksum     string    `json:"version_checksum,omitempty" gorm:"column:version_checksum"`
	MainConfigChecksum  string    `json:"main_config_checksum,omitempty" gorm:"column:main_config_checksum"`
	RouteConfigChecksum string    `json:"route_config_checksum,omitempty" gorm:"column:route_config_checksum"`
	RouteCount          int       `json:"route_count" gorm:"column:route_count"`
	ActivatedByPlanID   *uint     `json:"activated_by_plan_id,omitempty" gorm:"column:activated_by_plan_id"`
	ActivatedAt         time.Time `json:"activated_at" gorm:"column:activated_at"`
	ReferenceOK         bool      `json:"reference_ok" gorm:"-"`
	ReferenceError      string    `json:"reference_error,omitempty" gorm:"-"`

	VersionID               uint   `json:"-" gorm:"column:version_id"`
	FoundArtifactID         uint   `json:"-" gorm:"column:found_artifact_id"`
	ArtifactChecksum        string `json:"-" gorm:"column:artifact_checksum"`
	ArtifactConfigVersionID uint   `json:"-" gorm:"column:artifact_config_version_id"`
	ArtifactPoolName        string `json:"-" gorm:"column:artifact_pool_name"`
}

func ListActiveConfigPools() ([]ActiveConfigPoolStatus, error) {
	statuses := []ActiveConfigPoolStatus{}
	err := model.DB.Table("config_pool_active_versions AS active").
		Select(`
			active.pool_name,
			active.config_version_id,
			active.artifact_id,
			active.checksum,
			active.activated_by_plan_id,
			active.activated_at,
			config_versions.id AS version_id,
			config_versions.version,
			config_versions.checksum AS version_checksum,
			config_version_artifacts.id AS found_artifact_id,
			config_version_artifacts.checksum AS artifact_checksum,
			config_version_artifacts.main_config_checksum,
			config_version_artifacts.route_config_checksum,
			config_version_artifacts.route_count,
			config_version_artifacts.config_version_id AS artifact_config_version_id,
			config_version_artifacts.pool_name AS artifact_pool_name`).
		Joins("LEFT JOIN config_versions ON config_versions.id = active.config_version_id").
		Joins("LEFT JOIN config_version_artifacts ON config_version_artifacts.id = active.artifact_id").
		Order("active.pool_name asc").
		Scan(&statuses).Error
	if err != nil {
		return nil, err
	}
	for index := range statuses {
		statuses[index].markReferenceState()
	}
	return statuses, nil
}

func (status *ActiveConfigPoolStatus) markReferenceState() {
	if status == nil {
		return
	}
	status.ReferenceOK = true
	status.ReferenceError = ""
	switch {
	case status.VersionID == 0:
		status.ReferenceOK = false
		status.ReferenceError = fmt.Sprintf("missing config version %d", status.ConfigVersionID)
	case status.FoundArtifactID == 0:
		status.ReferenceOK = false
		status.ReferenceError = fmt.Sprintf("missing config artifact %d", status.ArtifactID)
	case status.ArtifactConfigVersionID != status.ConfigVersionID:
		status.ReferenceOK = false
		status.ReferenceError = fmt.Sprintf("artifact version mismatch: got %d", status.ArtifactConfigVersionID)
	case normalizeNodePoolName(status.ArtifactPoolName) != normalizeNodePoolName(status.PoolName):
		status.ReferenceOK = false
		status.ReferenceError = fmt.Sprintf("artifact pool mismatch: got %q", status.ArtifactPoolName)
	case status.ArtifactChecksum != "" && status.Checksum != status.ArtifactChecksum:
		status.ReferenceOK = false
		status.ReferenceError = "active checksum mismatch"
	}
}

func HasConfigChangesForPool(poolName string) (bool, error) {
	poolName = normalizeNodePoolName(poolName)
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	bundle, err := buildCurrentConfigBundle(false)
	if err != nil {
		return false, err
	}
	currentArtifact := currentConfigArtifactBundleForPool(bundle, poolName)
	_, activeArtifact, err := model.GetActiveConfigVersionArtifactMetaForPool(poolName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return currentArtifact != nil && len(bundle.Routes) > 0, nil
		}
		return false, err
	}
	if currentArtifact == nil {
		return true, nil
	}
	if activeArtifact.Checksum != currentArtifact.Checksum {
		return true, nil
	}
	if activeArtifact.MainConfigChecksum != currentArtifact.MainConfigChecksum {
		return true, nil
	}
	if activeArtifact.RouteConfigChecksum != currentArtifact.RouteConfigChecksum {
		return true, nil
	}
	return activeArtifact.RouteCount != currentArtifact.RouteCount, nil
}

func currentConfigArtifactBundleForPool(bundle *configBundle, poolName string) *configVersionArtifactBundle {
	if bundle == nil {
		return nil
	}
	for index := range bundle.Artifacts {
		if normalizeNodePoolName(bundle.Artifacts[index].PoolName) == poolName {
			return &bundle.Artifacts[index]
		}
	}
	return nil
}
