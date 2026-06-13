package router_test

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/router"
	"dushengcdn/service"
	"net/http"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestConfigVersionActivePoolsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)
	token := prepareRootToken(t)

	version := &model.ConfigVersion{
		Version:          "20260612-002",
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
	artifact := &model.ConfigVersionArtifact{
		ConfigVersionID:     version.ID,
		PoolName:            "hk",
		Checksum:            "hk-checksum",
		MainConfigChecksum:  "main-checksum",
		RouteConfigChecksum: "route-checksum",
		RenderedConfig:      "rendered",
		SupportFilesJSON:    "[]",
		RouteCount:          3,
	}
	if err := model.DB.Create(artifact).Error; err != nil {
		t.Fatalf("seed config artifact: %v", err)
	}
	if err := model.DB.Create(&model.ConfigPoolActiveVersion{
		PoolName:        artifact.PoolName,
		ConfigVersionID: version.ID,
		ArtifactID:      artifact.ID,
		Checksum:        artifact.Checksum,
		ActivatedAt:     time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed active pool: %v", err)
	}

	resp := performJSONRequest(t, engine, token, http.MethodGet, "/api/config-versions/active-pools", nil)
	var statuses []service.ActiveConfigPoolStatus
	decodeResponseData(t, resp, &statuses)
	if len(statuses) != 1 {
		t.Fatalf("expected one active pool status, got %+v", statuses)
	}
	status := statuses[0]
	if status.PoolName != "hk" || status.Version != version.Version || status.Checksum != artifact.Checksum {
		t.Fatalf("unexpected active pool status: %+v", status)
	}
	if status.RouteCount != 3 || !status.ReferenceOK {
		t.Fatalf("expected route_count and healthy reference in response, got %+v", status)
	}
}
