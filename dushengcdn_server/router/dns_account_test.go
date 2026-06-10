package router_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/router"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestDNSAccountCreateDoesNotRequireCloudflareOnlineVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	resp := performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/", map[string]any{
		"name":          "cloudflare-test",
		"type":          " CloudFlare ",
		"authorization": "not-a-real-cloudflare-token",
	})

	var created model.DnsAccount
	decodeResponseData(t, resp, &created)
	if created.ID == 0 || created.Name != "cloudflare-test" || created.Type != "cloudflare" {
		t.Fatalf("unexpected created DNS account: %+v", created)
	}

	var stored model.DnsAccount
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("failed to load created DNS account: %v", err)
	}
	if stored.Authorization != `{"api_token":"not-a-real-cloudflare-token"}` {
		t.Fatalf("expected normalized authorization, got %q", stored.Authorization)
	}
}

func TestDNSAccountCreateRejectsInvalidCloudflareAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	resp := performJSONRequestNoFatal(t, engine, token, http.MethodPost, "/api/dns-accounts/", map[string]any{
		"name":          "cloudflare-empty",
		"type":          "cloudflare",
		"authorization": `{"api_token":`,
	})
	if resp.Success {
		t.Fatal("expected invalid Cloudflare authorization to fail")
	}
	if !strings.Contains(resp.Message, "DNS 账号凭据格式无效") {
		t.Fatalf("unexpected error message: %s", resp.Message)
	}

	var count int64
	if err := model.DB.Model(&model.DnsAccount{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count DNS accounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected invalid DNS account not to persist, got %d records", count)
	}
}

func TestDNSAccountCreateOmitsAuthorizationFromResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	resp := performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/", map[string]any{
		"name":          "cloudflare-hidden",
		"type":          "cloudflare",
		"authorization": "hidden-token",
	})

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		t.Fatalf("failed to decode DNS account response: %v", err)
	}
	if _, ok := raw["authorization"]; ok {
		t.Fatal("expected DNS account response to omit authorization")
	}
}

func TestDNSAccountUpdateWithEmptyAuthorizationPreservesStoredSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	createResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/", map[string]any{
		"name":          "cloudflare-preserve",
		"type":          "cloudflare",
		"authorization": "original-token",
	})
	var created model.DnsAccount
	decodeResponseData(t, createResp, &created)

	updateResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/"+toString(created.ID)+"/update", map[string]any{
		"name":          "cloudflare-renamed",
		"type":          "cloudflare",
		"authorization": "   ",
	})

	var updated model.DnsAccount
	decodeResponseData(t, updateResp, &updated)
	if updated.Name != "cloudflare-renamed" {
		t.Fatalf("expected DNS account name to update, got %q", updated.Name)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(updateResp.Data, &raw); err != nil {
		t.Fatalf("failed to decode DNS account update response: %v", err)
	}
	if _, ok := raw["authorization"]; ok {
		t.Fatal("expected DNS account update response to omit authorization")
	}

	var stored model.DnsAccount
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("failed to load updated DNS account: %v", err)
	}
	if stored.Authorization != `{"api_token":"original-token"}` {
		t.Fatalf("expected blank update to preserve authorization, got %q", stored.Authorization)
	}
}

func TestDNSAccountUpdateWithAuthorizationRotatesStoredSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	createResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/", map[string]any{
		"name":          "cloudflare-rotate",
		"type":          "cloudflare",
		"authorization": "original-token",
	})
	var created model.DnsAccount
	decodeResponseData(t, createResp, &created)

	performJSONRequest(t, engine, token, http.MethodPost, "/api/dns-accounts/"+toString(created.ID)+"/update", map[string]any{
		"name":          "cloudflare-rotate",
		"type":          "cloudflare",
		"authorization": "Bearer rotated-token",
	})

	var stored model.DnsAccount
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("failed to load rotated DNS account: %v", err)
	}
	if stored.Authorization != `{"api_token":"rotated-token"}` {
		t.Fatalf("expected non-empty update to rotate authorization, got %q", stored.Authorization)
	}
}
