package service

import (
	"dushengcdn/model"
	"testing"
	"time"
)

func TestListActiveConfigPoolsReturnsJoinedStatus(t *testing.T) {
	setupServiceTestDB(t)

	activatedAt := time.Now().UTC().Truncate(time.Second)
	version := &model.ConfigVersion{
		Version:          "20260612-001",
		SnapshotJSON:     "{}",
		MainConfig:       "main",
		RenderedConfig:   "rendered",
		SupportFilesJSON: "[]",
		Checksum:         "global-checksum",
		IsActive:         true,
		CreatedBy:        "root",
	}
	if err := model.DB.Create(version).Error; err != nil {
		t.Fatalf("seed config version: %v", err)
	}
	artifacts := []*model.ConfigVersionArtifact{
		{
			ConfigVersionID:     version.ID,
			PoolName:            "hk",
			Checksum:            "hk-checksum",
			MainConfigChecksum:  "main-checksum",
			RouteConfigChecksum: "hk-route-checksum",
			RenderedConfig:      "hk rendered",
			SupportFilesJSON:    "[]",
			RouteCount:          2,
		},
		{
			ConfigVersionID:     version.ID,
			PoolName:            "idle",
			Checksum:            "idle-checksum",
			MainConfigChecksum:  "main-checksum",
			RouteConfigChecksum: "idle-route-checksum",
			RenderedConfig:      "idle rendered",
			SupportFilesJSON:    "[]",
			RouteCount:          0,
		},
	}
	for _, artifact := range artifacts {
		if err := model.DB.Create(artifact).Error; err != nil {
			t.Fatalf("seed artifact %s: %v", artifact.PoolName, err)
		}
		if err := model.DB.Create(&model.ConfigPoolActiveVersion{
			PoolName:        artifact.PoolName,
			ConfigVersionID: version.ID,
			ArtifactID:      artifact.ID,
			Checksum:        artifact.Checksum,
			ActivatedAt:     activatedAt,
		}).Error; err != nil {
			t.Fatalf("seed active pool %s: %v", artifact.PoolName, err)
		}
	}

	statuses, err := ListActiveConfigPools()
	if err != nil {
		t.Fatalf("ListActiveConfigPools failed: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 active pool statuses, got %+v", statuses)
	}
	byPool := map[string]ActiveConfigPoolStatus{}
	for _, status := range statuses {
		byPool[status.PoolName] = status
	}
	hk := byPool["hk"]
	if hk.Version != version.Version || hk.ConfigVersionID != version.ID || hk.Checksum != "hk-checksum" {
		t.Fatalf("unexpected hk active pool status: %+v", hk)
	}
	if hk.RouteCount != 2 || hk.MainConfigChecksum != "main-checksum" || hk.RouteConfigChecksum != "hk-route-checksum" {
		t.Fatalf("expected hk artifact metadata in status, got %+v", hk)
	}
	if !hk.ReferenceOK || hk.ReferenceError != "" {
		t.Fatalf("expected hk reference to be healthy, got %+v", hk)
	}
	if byPool["idle"].RouteCount != 0 {
		t.Fatalf("expected idle route count 0, got %+v", byPool["idle"])
	}
}

func TestHasConfigChangesForPoolDetectsPoolOnlyArtifactChanges(t *testing.T) {
	setupServiceTestDB(t)

	if _, err := CreateNode(NodeInput{Name: "default-edge", PoolName: "default"}); err != nil {
		t.Fatalf("CreateNode default failed: %v", err)
	}
	if _, err := CreateNode(NodeInput{Name: "hk-edge", PoolName: "hk"}); err != nil {
		t.Fatalf("CreateNode hk failed: %v", err)
	}
	route := &model.ProxyRoute{
		SiteName:                   "pool-status",
		Domain:                     "pool-status.example.com",
		Domains:                    mustJSON(t, []string{"pool-status.example.com"}),
		OriginURL:                  "https://origin.internal",
		Upstreams:                  mustJSON(t, []string{"https://origin.internal"}),
		NodePool:                   "default",
		Enabled:                    true,
		CustomHeaders:              "[]",
		CacheRules:                 "[]",
		PoWConfig:                  "{}",
		WAFMode:                    proxyRouteWAFModeBlock,
		WAFConfig:                  "{}",
		CCMode:                     proxyRouteCCModeBlock,
		CCConfig:                   "{}",
		RegionRestrictionMode:      proxyRouteRegionModeBlock,
		RegionRestrictionCountries: "[]",
		DNSRecordType:              "A",
		DNSTargetCount:             1,
		DNSScheduleMode:            "healthy",
		DNSTTL:                     1,
		DNSProviderMode:            DNSProviderModeCloudflare,
		GSLBPolicy:                 "{}",
		DDOSProtectionMode:         DDOSProtectionModeOff,
		DDOSProtectionProvider:     DDOSProtectionProviderCloudflare,
	}
	seedConfigVersionArtifactTestRoute(t, route)
	if _, err := PublishConfigVersion("root", false); err != nil {
		t.Fatalf("initial PublishConfigVersion failed: %v", err)
	}

	changed, err := HasConfigChangesForPool("default")
	if err != nil {
		t.Fatalf("HasConfigChangesForPool default before move failed: %v", err)
	}
	if changed {
		t.Fatal("expected default pool to be clean immediately after publish")
	}

	route.NodePool = "hk"
	if err := route.Update(); err != nil {
		t.Fatalf("move route pool: %v", err)
	}
	defaultChanged, err := HasConfigChangesForPool("default")
	if err != nil {
		t.Fatalf("HasConfigChangesForPool default failed: %v", err)
	}
	if !defaultChanged {
		t.Fatal("expected default pool to detect route removal")
	}
	hkChanged, err := HasConfigChangesForPool("hk")
	if err != nil {
		t.Fatalf("HasConfigChangesForPool hk failed: %v", err)
	}
	if !hkChanged {
		t.Fatal("expected hk pool to detect route addition")
	}
	missingChanged, err := HasConfigChangesForPool("missing")
	if err != nil {
		t.Fatalf("HasConfigChangesForPool missing failed: %v", err)
	}
	if missingChanged {
		t.Fatal("expected unrelated missing pool to have no changes")
	}
}
