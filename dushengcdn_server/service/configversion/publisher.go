package configversion

import (
	"dushengcdn/model"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

func NextVersionNumber(now time.Time) (string, error) {
	prefix := now.Format("20060102")
	var latest model.ConfigVersion
	err := model.DB.
		Select("version").
		Where("version LIKE ?", prefix+"-%").
		Order("version desc").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Sprintf("%s-%03d", prefix, 1), nil
	}
	if err != nil {
		return "", err
	}
	suffix := strings.TrimPrefix(latest.Version, prefix+"-")
	sequence, err := strconv.Atoi(suffix)
	if err != nil {
		return "", fmt.Errorf("invalid config version sequence %q: %w", latest.Version, err)
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
	var deleteIDs []uint
	if err := model.DB.Model(&model.ConfigVersion{}).
		Select("id").
		Where("is_active = ?", false).
		Where("id NOT IN ?", keepIDs).
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
