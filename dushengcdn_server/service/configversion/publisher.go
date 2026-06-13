package configversion

import (
	"dushengcdn/model"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

func NextVersionNumber(now time.Time) (string, error) {
	prefix := now.Format("20060102")
	var versions []string
	if err := model.DB.Model(&model.ConfigVersion{}).
		Select("version").
		Where("version LIKE ?", prefix+"-%").
		Pluck("version", &versions).Error; err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return fmt.Sprintf("%s-%03d", prefix, 1), nil
	}
	sequence := 0
	for _, version := range versions {
		suffix := strings.TrimPrefix(version, prefix+"-")
		value, err := strconv.Atoi(suffix)
		if err != nil {
			return "", fmt.Errorf("invalid config version sequence %q: %w", version, err)
		}
		if value > sequence {
			sequence = value
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, sequence+1), nil
}

func CleanupVersions(keepCount int) (int64, error) {
	if keepCount < 3 {
		keepCount = 3
	}
	var keepIDs []uint
	if err := model.DB.Model(&model.ConfigVersion{}).
		Select("id").
		Order("id desc").
		Limit(keepCount).
		Pluck("id", &keepIDs).Error; err != nil {
		return 0, err
	}
	if len(keepIDs) < keepCount {
		return 0, nil
	}
	protectedIDs := append([]uint{}, keepIDs...)
	var activePoolVersionIDs []uint
	if err := model.DB.Model(&model.ConfigPoolActiveVersion{}).
		Select("config_version_id").
		Pluck("config_version_id", &activePoolVersionIDs).Error; err != nil {
		return 0, err
	}
	protectedIDs = append(protectedIDs, activePoolVersionIDs...)
	var activePlanVersionIDs []uint
	if err := model.DB.Model(&model.ConfigReleasePlan{}).
		Select("config_version_id").
		Where("status IN ?", []string{"running", "observing"}).
		Pluck("config_version_id", &activePlanVersionIDs).Error; err != nil {
		return 0, err
	}
	protectedIDs = append(protectedIDs, activePlanVersionIDs...)
	var activePlanRollbackIDs []uint
	if err := model.DB.Model(&model.ConfigReleasePlan{}).
		Select("rollback_version_id").
		Where("status IN ? AND rollback_version_id IS NOT NULL", []string{"running", "observing"}).
		Pluck("rollback_version_id", &activePlanRollbackIDs).Error; err != nil {
		return 0, err
	}
	protectedIDs = append(protectedIDs, activePlanRollbackIDs...)
	var deleteIDs []uint
	if err := model.DB.Model(&model.ConfigVersion{}).
		Select("id").
		Where("is_active = ?", false).
		Where("id NOT IN ?", uniqueConfigVersionIDs(protectedIDs)).
		Pluck("id", &deleteIDs).Error; err != nil {
		return 0, err
	}
	if len(deleteIDs) == 0 {
		return 0, nil
	}
	var deleted int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("config_version_id IN ?", deleteIDs).Delete(&model.ConfigVersionArtifact{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", deleteIDs).Delete(&model.ConfigVersion{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}

func uniqueConfigVersionIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return ids
	}
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
