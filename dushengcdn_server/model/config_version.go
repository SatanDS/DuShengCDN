package model

import "time"

type ConfigVersionSummary struct {
	ID        uint      `json:"id"`
	Version   string    `json:"version"`
	Checksum  string    `json:"checksum"`
	IsActive  bool      `json:"is_active"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type ConfigVersion struct {
	ID               uint      `json:"id" gorm:"column:id;primaryKey"`
	Version          string    `json:"version" gorm:"column:version;uniqueIndex;size:32;not null"`
	SnapshotJSON     string    `json:"snapshot_json" gorm:"column:snapshot_json;type:text;not null"`
	MainConfig       string    `json:"main_config" gorm:"column:main_config;type:text;not null;default:''"`
	RenderedConfig   string    `json:"rendered_config" gorm:"column:rendered_config;type:text;not null"`
	SupportFilesJSON string    `json:"support_files_json" gorm:"column:support_files_json;type:text;not null;default:'[]'"`
	Checksum         string    `json:"checksum" gorm:"column:checksum;size:64;not null"`
	IsActive         bool      `json:"is_active" gorm:"column:is_active;not null;default:false;index"`
	CreatedBy        string    `json:"created_by" gorm:"column:created_by;size:64;not null"`
	CreatedAt        time.Time `json:"created_at" gorm:"column:created_at"`
}

type ConfigVersionArtifact struct {
	ID                  uint      `json:"id" gorm:"column:id;primaryKey"`
	ConfigVersionID     uint      `json:"config_version_id" gorm:"column:config_version_id;not null;uniqueIndex:idx_config_version_artifact_pool"`
	PoolName            string    `json:"pool_name" gorm:"column:pool_name;size:64;not null;uniqueIndex:idx_config_version_artifact_pool"`
	Checksum            string    `json:"checksum" gorm:"column:checksum;size:64;not null"`
	MainConfigChecksum  string    `json:"main_config_checksum" gorm:"column:main_config_checksum;size:64;not null;default:''"`
	RouteConfigChecksum string    `json:"route_config_checksum" gorm:"column:route_config_checksum;size:64;not null;default:''"`
	RenderedConfig      string    `json:"rendered_config" gorm:"column:rendered_config;type:text;not null"`
	SupportFilesJSON    string    `json:"support_files_json" gorm:"column:support_files_json;type:text;not null;default:'[]'"`
	RouteCount          int       `json:"route_count" gorm:"column:route_count;not null;default:0"`
	CreatedAt           time.Time `json:"created_at" gorm:"column:created_at"`
}

type ConfigPoolActiveVersion struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	PoolName          string    `json:"pool_name" gorm:"size:64;not null;uniqueIndex"`
	ConfigVersionID   uint      `json:"config_version_id" gorm:"not null;index"`
	ArtifactID        uint      `json:"artifact_id" gorm:"not null;index"`
	Checksum          string    `json:"checksum" gorm:"size:64;not null;index"`
	ActivatedByPlanID *uint     `json:"activated_by_plan_id" gorm:"index"`
	ActivatedAt       time.Time `json:"activated_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func ListConfigVersionSummaries() (versions []*ConfigVersionSummary, err error) {
	err = DB.Model(&ConfigVersion{}).
		Select("id", "version", "checksum", "is_active", "created_by", "created_at").
		Order("id desc").
		Find(&versions).Error
	return versions, err
}

func GetConfigVersionByID(id uint) (*ConfigVersion, error) {
	version := &ConfigVersion{}
	err := DB.First(version, id).Error
	return version, err
}

func GetConfigVersionMetaByID(id uint) (*ConfigVersion, error) {
	version := &ConfigVersion{}
	err := DB.Select("id", "version", "checksum", "is_active", "created_by", "created_at").First(version, id).Error
	return version, err
}

func GetActiveConfigVersion() (*ConfigVersion, error) {
	version := &ConfigVersion{}
	err := DB.Where("is_active = ?", true).Order("id desc").First(version).Error
	return version, err
}

func GetActiveConfigVersionMeta() (*ConfigVersion, error) {
	version := &ConfigVersion{}
	err := DB.Select("id", "version", "checksum", "is_active", "created_by", "created_at").
		Where("is_active = ?", true).
		Order("id desc").
		First(version).Error
	return version, err
}

func ListConfigVersionArtifacts(versionID uint) (artifacts []*ConfigVersionArtifact, err error) {
	err = DB.Where("config_version_id = ?", versionID).Order("pool_name asc").Find(&artifacts).Error
	return artifacts, err
}

func GetConfigVersionArtifact(versionID uint, poolName string) (*ConfigVersionArtifact, error) {
	artifact := &ConfigVersionArtifact{}
	err := DB.Where("config_version_id = ? AND pool_name = ?", versionID, poolName).First(artifact).Error
	return artifact, err
}

func GetConfigVersionArtifactMeta(versionID uint, poolName string) (*ConfigVersionArtifact, error) {
	artifact := &ConfigVersionArtifact{}
	err := DB.Select("id", "config_version_id", "pool_name", "checksum", "main_config_checksum", "route_config_checksum", "route_count", "created_at").
		Where("config_version_id = ? AND pool_name = ?", versionID, poolName).
		First(artifact).Error
	return artifact, err
}

func GetActiveConfigVersionArtifactForPool(poolName string) (*ConfigVersion, *ConfigVersionArtifact, error) {
	active := &ConfigPoolActiveVersion{}
	if err := DB.Where("pool_name = ?", poolName).First(active).Error; err != nil {
		return nil, nil, err
	}
	version, err := GetConfigVersionByID(active.ConfigVersionID)
	if err != nil {
		return nil, nil, err
	}
	artifact, err := GetConfigVersionArtifact(version.ID, poolName)
	if err != nil {
		return nil, nil, err
	}
	return version, artifact, nil
}

func GetActiveConfigVersionArtifactMetaForPool(poolName string) (*ConfigVersion, *ConfigVersionArtifact, error) {
	active := &ConfigPoolActiveVersion{}
	if err := DB.Where("pool_name = ?", poolName).First(active).Error; err != nil {
		return nil, nil, err
	}
	version, err := GetConfigVersionMetaByID(active.ConfigVersionID)
	if err != nil {
		return nil, nil, err
	}
	artifact, err := GetConfigVersionArtifactMeta(version.ID, poolName)
	if err != nil {
		return nil, nil, err
	}
	return version, artifact, nil
}

func ListConfigVersionArtifactMetas(versionID uint, poolNames []string) (artifacts []*ConfigVersionArtifact, err error) {
	if len(poolNames) == 0 {
		return []*ConfigVersionArtifact{}, nil
	}
	err = DB.Select("id", "config_version_id", "pool_name", "checksum", "main_config_checksum", "route_config_checksum", "route_count", "created_at").
		Where("config_version_id = ? AND pool_name IN ?", versionID, poolNames).
		Order("pool_name asc").
		Find(&artifacts).Error
	return artifacts, err
}
