package controller

import (
	"bytes"
	"dushengcdn/common"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPurgeProxyRouteCacheRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousCachePath := common.OpenRestyCachePath
	common.OpenRestyCachePath = "/var/cache/openresty/dushengcdn"
	t.Cleanup(func() {
		common.OpenRestyCachePath = previousCachePath
	})

	engine := gin.New()
	engine.POST("/api/proxy-routes/:id/cache/purge", PurgeProxyRouteCache)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/proxy-routes/1/cache/purge", bytes.NewBufferString(`{"scope":`))
	request.Header.Set("Content-Type", "application/json")

	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "invalid payload") {
		t.Fatalf("expected invalid payload response, got %s", recorder.Body.String())
	}
}
