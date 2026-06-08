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

func GetActiveConfigVersion() (*ConfigVersion, error) {
	version := &ConfigVersion{}
	err := DB.Where("is_active = ?", true).Order("id desc").First(version).Error
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
