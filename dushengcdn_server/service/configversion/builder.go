package configversion

import (
	"crypto/sha256"
	"dushengcdn/model"
	"dushengcdn/service/openresty"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type SupportFile = openresty.SupportFile

type ArtifactBundle struct {
	PoolName            string
	RouteConfig         string
	SupportFiles        []SupportFile
	Checksum            string
	MainConfigChecksum  string
	RouteConfigChecksum string
	SupportFilesJSON    string
	RouteCount          int
}

type ArtifactManifestItem struct {
	PoolName            string `json:"pool_name"`
	Checksum            string `json:"checksum"`
	MainConfigChecksum  string `json:"main_config_checksum"`
	RouteConfigChecksum string `json:"route_config_checksum"`
	RouteCount          int    `json:"route_count"`
}

func Checksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func ChecksumBundle(mainConfig string, routeConfig string, supportFiles []SupportFile) string {
	var builder strings.Builder
	builder.WriteString(mainConfig)
	builder.WriteString("\n--route-config--\n")
	builder.WriteString(routeConfig)
	builder.WriteString("\n--support-files--\n")
	files := openresty.DedupeSupportFiles(supportFiles)
	sort.Slice(files, func(i int, j int) bool {
		return files[i].Path < files[j].Path
	})
	for _, file := range files {
		builder.WriteString(file.Path)
		builder.WriteString("\n")
		builder.WriteString(file.Content)
		builder.WriteString("\n")
	}
	return Checksum(builder.String())
}

func CreateArtifacts(tx *gorm.DB, versionID uint, bundles []ArtifactBundle) error {
	if len(bundles) == 0 {
		return nil
	}
	artifacts := make([]model.ConfigVersionArtifact, 0, len(bundles))
	for _, bundle := range bundles {
		artifacts = append(artifacts, model.ConfigVersionArtifact{
			ConfigVersionID:     versionID,
			PoolName:            bundle.PoolName,
			Checksum:            bundle.Checksum,
			MainConfigChecksum:  bundle.MainConfigChecksum,
			RouteConfigChecksum: bundle.RouteConfigChecksum,
			RenderedConfig:      bundle.RouteConfig,
			SupportFilesJSON:    bundle.SupportFilesJSON,
			RouteCount:          bundle.RouteCount,
		})
	}
	return tx.Create(&artifacts).Error
}

func ArtifactBundleManifestChecksum(bundles []ArtifactBundle, normalizePoolName func(string) string) string {
	items := make([]ArtifactManifestItem, 0, len(bundles))
	for _, bundle := range bundles {
		items = append(items, ArtifactManifestItem{
			PoolName:            normalizePoolName(bundle.PoolName),
			Checksum:            strings.TrimSpace(bundle.Checksum),
			MainConfigChecksum:  strings.TrimSpace(bundle.MainConfigChecksum),
			RouteConfigChecksum: strings.TrimSpace(bundle.RouteConfigChecksum),
			RouteCount:          bundle.RouteCount,
		})
	}
	return ChecksumArtifactManifest(items)
}

func ChecksumArtifactManifest(items []ArtifactManifestItem) string {
	sort.Slice(items, func(i int, j int) bool {
		return items[i].PoolName < items[j].PoolName
	})
	raw, err := json.Marshal(items)
	if err != nil {
		return Checksum("")
	}
	return Checksum(string(raw))
}
