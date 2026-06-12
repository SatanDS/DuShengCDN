package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"sort"
	"strings"
)

func backfillDNSWorkerTokenHashes(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&DNSWorker{}) {
		return nil
	}
	var workers []DNSWorker
	if err := db.Find(&workers).Error; err != nil {
		return err
	}
	for i := range workers {
		token := strings.TrimSpace(workers[i].Token)
		if token == "" || strings.TrimSpace(workers[i].TokenHash) != "" {
			continue
		}
		sum := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(sum[:])
		tokenPrefix := token
		if len(tokenPrefix) > 12 {
			tokenPrefix = tokenPrefix[:12]
		}
		if err := db.Model(&workers[i]).Updates(map[string]any{
			"token":        "",
			"token_hash":   tokenHash,
			"token_prefix": tokenPrefix,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillLegacyConfigVersionArtifacts(db *gorm.DB) error {
	db = migrationSession(db)
	if db == nil || !db.Migrator().HasTable(&ConfigVersion{}) || !db.Migrator().HasTable(&ConfigVersionArtifact{}) {
		return nil
	}
	db = db.Session(&gorm.Session{SkipDefaultTransaction: true})
	var versions []ConfigVersion
	if err := db.Order("id asc").Find(&versions).Error; err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}
	poolNames, err := legacyConfigVersionArtifactPoolNames(db)
	if err != nil {
		return err
	}
	for _, version := range versions {
		var count int64
		if err := db.Model(&ConfigVersionArtifact{}).Where("config_version_id = ?", version.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		supportFilesJSON := strings.TrimSpace(version.SupportFilesJSON)
		if supportFilesJSON == "" {
			supportFilesJSON = "[]"
		}
		routeCount := len(legacySnapshotRouteDomains(version.SnapshotJSON))
		mainChecksum := checksumMigrationString(version.MainConfig)
		routeChecksum := checksumMigrationString(version.RenderedConfig)
		for _, poolName := range poolNames {
			artifact := &ConfigVersionArtifact{
				ConfigVersionID:     version.ID,
				PoolName:            poolName,
				Checksum:            version.Checksum,
				MainConfigChecksum:  mainChecksum,
				RouteConfigChecksum: routeChecksum,
				RenderedConfig:      version.RenderedConfig,
				SupportFilesJSON:    supportFilesJSON,
				RouteCount:          routeCount,
			}
			if err := db.Create(artifact).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyConfigVersionArtifactPoolNames(db *gorm.DB) ([]string, error) {
	poolSet := map[string]struct{}{"default": {}}
	if db.Migrator().HasTable(&Node{}) && db.Migrator().HasColumn(&Node{}, "pool_name") {
		var nodes []Node
		if err := db.Select("pool_name").Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, node := range nodes {
			poolSet[normalizeMigrationPoolName(node.PoolName)] = struct{}{}
		}
	}
	poolNames := make([]string, 0, len(poolSet))
	for poolName := range poolSet {
		if poolName != "" {
			poolNames = append(poolNames, poolName)
		}
	}
	sort.Strings(poolNames)
	return poolNames, nil
}

func normalizeMigrationPoolName(raw string) string {
	poolName := strings.ToLower(strings.TrimSpace(raw))
	if poolName == "" {
		return "default"
	}
	if len(poolName) > 64 {
		return poolName[:64]
	}
	return poolName
}

func checksumMigrationString(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func legacySnapshotRouteDomains(snapshotJSON string) []string {
	var snapshot struct {
		Routes []struct {
			Domain string `json:"domain"`
		} `json:"routes"`
	}
	if strings.TrimSpace(snapshotJSON) == "" || json.Unmarshal([]byte(snapshotJSON), &snapshot) != nil {
		return nil
	}
	return make([]string, len(snapshot.Routes))
}
