package router_test

import (
	"bytes"
	"dushengcdn/common"
	"dushengcdn/controller"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/router"
	"dushengcdn/service"
	"dushengcdn/utils/security"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
)

func TestPhase2RateLimitOptionsHotReload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)
	model.InitOptionMap()

	oldGlobalApiRateLimitNum := common.GlobalApiRateLimitNum
	oldGlobalApiRateLimitDuration := common.GlobalApiRateLimitDuration
	oldCriticalRateLimitNum := common.CriticalRateLimitNum
	oldCriticalRateLimitDuration := common.CriticalRateLimitDuration
	t.Cleanup(func() {
		common.GlobalApiRateLimitNum = oldGlobalApiRateLimitNum
		common.GlobalApiRateLimitDuration = oldGlobalApiRateLimitDuration
		common.CriticalRateLimitNum = oldCriticalRateLimitNum
		common.CriticalRateLimitDuration = oldCriticalRateLimitDuration
	})

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)

	performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/option/update-batch", map[string]any{
		"options": []map[string]any{
			{
				"key":   "GlobalApiRateLimitNum",
				"value": "450",
			},
			{
				"key":   "GlobalApiRateLimitDuration",
				"value": "240",
			},
			{
				"key":   "CriticalRateLimitNum",
				"value": "150",
			},
			{
				"key":   "CriticalRateLimitDuration",
				"value": "900",
			},
		},
	})

	if common.GlobalApiRateLimitNum != 450 {
		t.Fatalf("expected GlobalApiRateLimitNum to be hot reloaded, got %d", common.GlobalApiRateLimitNum)
	}
	if common.GlobalApiRateLimitDuration != 240 {
		t.Fatalf("expected GlobalApiRateLimitDuration to be hot reloaded, got %d", common.GlobalApiRateLimitDuration)
	}
	if common.CriticalRateLimitNum != 150 {
		t.Fatalf("expected CriticalRateLimitNum to be hot reloaded, got %d", common.CriticalRateLimitNum)
	}
	if common.CriticalRateLimitDuration != 900 {
		t.Fatalf("expected CriticalRateLimitDuration to be hot reloaded, got %d", common.CriticalRateLimitDuration)
	}

	resp := performSessionJSONRequest(t, engine, loginCookie, http.MethodGet, "/api/option/", nil)
	var options []model.Option
	decodeResponseData(t, resp, &options)

	optionMap := make(map[string]string, len(options))
	for _, option := range options {
		optionMap[option.Key] = option.Value
	}

	if optionMap["GlobalApiRateLimitNum"] != "450" {
		t.Fatalf("expected option payload to include GlobalApiRateLimitNum=450, got %q", optionMap["GlobalApiRateLimitNum"])
	}
	if optionMap["CriticalRateLimitDuration"] != "900" {
		t.Fatalf("expected option payload to include CriticalRateLimitDuration=900, got %q", optionMap["CriticalRateLimitDuration"])
	}
}

func TestPhase2BatchOptionUpdateIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)
	model.InitOptionMap()

	oldGlobalAPI := common.GlobalApiRateLimitNum
	t.Cleanup(func() {
		common.GlobalApiRateLimitNum = oldGlobalAPI
	})

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)

	payload, err := json.Marshal(map[string]any{
		"options": []map[string]any{
			{
				"key":   "GlobalApiRateLimitNum",
				"value": "451",
			},
			{
				"key":   "CriticalRateLimitDuration",
				"value": "1800",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal batch payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/option/update-batch", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(loginCookie)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected invalid batch update to fail")
	}

	if common.GlobalApiRateLimitNum != oldGlobalAPI {
		t.Fatalf("expected GlobalApiRateLimitNum to remain %d after failed batch, got %d", oldGlobalAPI, common.GlobalApiRateLimitNum)
	}

	resp = performSessionJSONRequest(t, engine, loginCookie, http.MethodGet, "/api/option/", nil)
	var options []model.Option
	decodeResponseData(t, resp, &options)

	optionMap := make(map[string]string, len(options))
	for _, option := range options {
		optionMap[option.Key] = option.Value
	}

	if optionMap["GlobalApiRateLimitNum"] == "451" {
		t.Fatal("expected failed batch update to avoid persisting partial values")
	}
}

func TestPhase2BatchOptionUpdateValidatesMergedState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)
	model.InitOptionMap()

	oldGitHubClientID := common.GitHubClientId
	oldGitHubOAuthEnabled := common.GitHubOAuthEnabled
	t.Cleanup(func() {
		common.GitHubClientId = oldGitHubClientID
		common.GitHubOAuthEnabled = oldGitHubOAuthEnabled
	})

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)

	performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/option/update-batch", map[string]any{
		"options": []map[string]any{
			{
				"key":   "GitHubClientId",
				"value": "client-id-from-batch",
			},
			{
				"key":   "GitHubOAuthEnabled",
				"value": "true",
			},
		},
	})

	if common.GitHubClientId != "client-id-from-batch" {
		t.Fatalf("expected GitHubClientId to be updated from batch, got %q", common.GitHubClientId)
	}
	if !common.GitHubOAuthEnabled {
		t.Fatal("expected GitHubOAuthEnabled to be enabled by merged batch state")
	}
}

func TestAuthSourceUpdateAcceptsClientSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)

	createResp := performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/auth-sources/", map[string]any{
		"name":          "github-main",
		"type":          "github",
		"display_name":  "GitHub",
		"is_active":     false,
		"client_id":     "github-client-id",
		"client_secret": "initial-secret",
		"scopes":        "user:email",
	})

	var created model.AuthSource
	decodeResponseData(t, createResp, &created)
	if created.ClientSecret != "" {
		t.Fatal("expected create response to avoid exposing client_secret")
	}
	if !created.ClientSecretConfigured {
		t.Fatal("expected create response to mark client_secret as configured")
	}

	updateResp := performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/auth-sources/1/update", map[string]any{
		"name":          "github-main",
		"type":          "github",
		"display_name":  "GitHub",
		"is_active":     true,
		"client_id":     "github-client-id",
		"client_secret": "updated-secret",
		"scopes":        "user:email",
	})

	var updated model.AuthSource
	decodeResponseData(t, updateResp, &updated)
	if updated.ClientSecret != "" {
		t.Fatal("expected update response to avoid exposing client_secret")
	}
	if !updated.ClientSecretConfigured {
		t.Fatal("expected update response to mark client_secret as configured")
	}
	if !updated.IsActive {
		t.Fatal("expected auth source to be active after update")
	}

	stored, err := model.GetAuthSourceByID(1)
	if err != nil {
		t.Fatalf("expected auth source to exist: %v", err)
	}
	if stored.ClientSecret != "updated-secret" {
		t.Fatalf("expected stored client secret to be updated, got %q", stored.ClientSecret)
	}

	performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/auth-sources/1/toggle", map[string]any{
		"is_active": false,
	})
	performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/auth-sources/1/toggle", map[string]any{
		"is_active": true,
	})
}

func TestExternalAccountBindingsCanBeListedAndDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)

	source := &model.AuthSource{
		Name:               "logto",
		Type:               model.AuthSourceTypeOIDC,
		DisplayName:        "Logto",
		ClientID:           "logto-client-id",
		ClientSecret:       "logto-client-secret",
		OpenIDDiscoveryURL: "https://auth.example.com/.well-known/openid-configuration",
	}
	if err := model.CreateAuthSource(source); err != nil {
		t.Fatalf("create auth source: %v", err)
	}
	if err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     source.ID,
		UserID:           1,
		ExternalID:       "logto-user-1",
		ExternalUsername: "ryan",
		Email:            "ryan@example.com",
	}); err != nil {
		t.Fatalf("link external account: %v", err)
	}

	listResp := performSessionJSONRequest(t, engine, loginCookie, http.MethodGet, "/api/oauth/external-accounts/", nil)
	var bindings []model.ExternalAccountView
	decodeResponseData(t, listResp, &bindings)
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].AuthSourceName != "logto" || bindings[0].ExternalUsername != "ryan" {
		t.Fatalf("unexpected binding view: %+v", bindings[0])
	}

	performSessionJSONRequest(t, engine, loginCookie, http.MethodPost, "/api/oauth/external-accounts/1/delete", nil)

	listResp = performSessionJSONRequest(t, engine, loginCookie, http.MethodGet, "/api/oauth/external-accounts/", nil)
	decodeResponseData(t, listResp, &bindings)
	if len(bindings) != 0 {
		t.Fatalf("expected binding to be deleted, got %+v", bindings)
	}
}

func TestLegacyGitHubOAuthAuthorizeStoresState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.GitHubOAuthEnabled
	oldClientID := common.GitHubClientId
	oldClientSecret := common.GitHubClientSecret
	t.Cleanup(func() {
		common.GitHubOAuthEnabled = oldEnabled
		common.GitHubClientId = oldClientID
		common.GitHubClientSecret = oldClientSecret
	})
	common.GitHubOAuthEnabled = true
	common.GitHubClientId = "legacy-client"
	common.GitHubClientSecret = "legacy-secret"

	sessionKey := []byte("test-secret")
	store := cookie.NewStore(sessionKey)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", store))
	router.SetApiRouter(engine)

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github/authorize", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected legacy github authorize to succeed, got %s", resp.Message)
	}
	var payload map[string]string
	decodeResponseData(t, resp, &payload)

	authorizeURL := payload["authorize_url"]
	if !strings.HasPrefix(authorizeURL, "https://github.com/login/oauth/authorize?") {
		t.Fatalf("unexpected authorize url: %s", authorizeURL)
	}
	if !strings.Contains(authorizeURL, "client_id=legacy-client") || !strings.Contains(authorizeURL, "state=") {
		t.Fatalf("expected authorize url to include client id and state, got %s", authorizeURL)
	}

	sessionCookie := firstResponseCookie(t, recorder, "session")
	values := map[interface{}]interface{}{}
	if err := securecookie.DecodeMulti("session", sessionCookie.Value, &values, securecookie.CodecsFromPairs(sessionKey)...); err != nil {
		t.Fatalf("failed to decode session cookie: %v", err)
	}
	if values["legacy_github_oauth_state"] == "" {
		t.Fatalf("expected legacy github oauth state in session, got %#v", values)
	}
}

func TestLegacyGitHubOAuthRejectsMissingState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.GitHubOAuthEnabled
	oldClientID := common.GitHubClientId
	oldClientSecret := common.GitHubClientSecret
	t.Cleanup(func() {
		common.GitHubOAuthEnabled = oldEnabled
		common.GitHubClientId = oldClientID
		common.GitHubClientSecret = oldClientSecret
	})
	common.GitHubOAuthEnabled = true
	common.GitHubClientId = "legacy-client"
	common.GitHubClientSecret = "legacy-secret"

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/github?code=stolen-code", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected legacy github oauth without state to fail")
	}
	if !strings.Contains(resp.Message, "授权状态无效") {
		t.Fatalf("expected invalid state message, got %q", resp.Message)
	}
	if strings.Contains(resp.Message, "provider leaked detail") {
		t.Fatalf("legacy github oauth must not reflect provider error descriptions, got %q", resp.Message)
	}
}

func TestLegacyGitHubOAuthProviderErrorRequiresStateAndIsGeneric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.GitHubOAuthEnabled
	oldClientID := common.GitHubClientId
	oldClientSecret := common.GitHubClientSecret
	t.Cleanup(func() {
		common.GitHubOAuthEnabled = oldEnabled
		common.GitHubClientId = oldClientID
		common.GitHubClientSecret = oldClientSecret
	})
	common.GitHubOAuthEnabled = true
	common.GitHubClientId = "legacy-client"
	common.GitHubClientSecret = "legacy-secret"

	sessionKey := []byte("test-secret")
	store := cookie.NewStore(sessionKey)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", store))
	router.SetApiRouter(engine)

	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github/authorize", nil)
	authorizeRecorder := httptest.NewRecorder()
	engine.ServeHTTP(authorizeRecorder, authorizeReq)
	if authorizeRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected authorize status %d: %s", authorizeRecorder.Code, authorizeRecorder.Body.String())
	}
	var authorizeResp apiResponse
	if err := json.Unmarshal(authorizeRecorder.Body.Bytes(), &authorizeResp); err != nil {
		t.Fatalf("failed to decode authorize response: %v", err)
	}
	var authorizePayload map[string]string
	decodeResponseData(t, authorizeResp, &authorizePayload)
	authorizeURL, err := url.Parse(authorizePayload["authorize_url"])
	if err != nil {
		t.Fatalf("failed to parse authorize url: %v", err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("expected state in authorize url")
	}
	sessionCookie := firstResponseCookie(t, authorizeRecorder, "session")

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github?error=access_denied&error_description=provider+leaked+detail&state="+url.QueryEscape(state), nil)
	callbackReq.AddCookie(sessionCookie)
	callbackRecorder := httptest.NewRecorder()
	engine.ServeHTTP(callbackRecorder, callbackReq)
	if callbackRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected callback status %d: %s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
	var callbackResp apiResponse
	if err = json.Unmarshal(callbackRecorder.Body.Bytes(), &callbackResp); err != nil {
		t.Fatalf("failed to decode callback response: %v", err)
	}
	if callbackResp.Success {
		t.Fatal("expected provider error to fail")
	}
	if callbackResp.Message != "GitHub 授权失败，请返回登录页重试" {
		t.Fatalf("expected generic provider error, got %q", callbackResp.Message)
	}
	if strings.Contains(callbackResp.Message, "provider leaked detail") {
		t.Fatalf("legacy github oauth must not reflect provider error descriptions, got %q", callbackResp.Message)
	}
}

func TestOAuthCallbackDoesNotBindWithPasswordChangedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldRegisterEnabled := common.RegisterEnabled
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
	})
	common.RegisterEnabled = false

	source := &model.AuthSource{
		Name:         "github-main",
		Type:         model.AuthSourceTypeGitHub,
		DisplayName:  "GitHub",
		IsActive:     true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	if err := model.CreateAuthSource(source); err != nil {
		t.Fatalf("failed to create auth source: %v", err)
	}

	defer service.SetOAuthHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://github.com/login/oauth/access_token":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"access_token":"oauth-token"}`)),
					Header:     make(http.Header),
				}, nil
			case "https://api.github.com/user":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":4242,"login":"attacker","name":"Attacker","email":"attacker@example.com"}`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected OAuth request: %s", req.URL.String())
				return nil, nil
			}
		}),
	})()

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)
	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github-main/authorize", nil)
	authorizeReq.AddCookie(loginCookie)
	authorizeRecorder := httptest.NewRecorder()
	engine.ServeHTTP(authorizeRecorder, authorizeReq)
	if authorizeRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected authorize status %d: %s", authorizeRecorder.Code, authorizeRecorder.Body.String())
	}
	var authorizeResp apiResponse
	if err := json.Unmarshal(authorizeRecorder.Body.Bytes(), &authorizeResp); err != nil {
		t.Fatalf("failed to decode authorize response: %v", err)
	}
	if !authorizeResp.Success {
		t.Fatalf("authorize failed: %s", authorizeResp.Message)
	}
	var authorizePayload map[string]string
	decodeResponseData(t, authorizeResp, &authorizePayload)
	authorizeURL, err := url.Parse(authorizePayload["authorize_url"])
	if err != nil {
		t.Fatalf("failed to parse authorize url: %v", err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("expected OAuth state")
	}
	oauthCookie := firstResponseCookie(t, authorizeRecorder, "session")

	if err := model.ResetUserPasswordByUsername("root", "new-password"); err != nil {
		t.Fatalf("failed to reset root password: %v", err)
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github-main/callback?code=oauth-code&state="+url.QueryEscape(state), nil)
	callbackReq.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	engine.ServeHTTP(callbackRecorder, callbackReq)
	if callbackRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected callback status %d: %s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
	var callbackResp apiResponse
	if err := json.Unmarshal(callbackRecorder.Body.Bytes(), &callbackResp); err != nil {
		t.Fatalf("failed to decode callback response: %v", err)
	}
	if !callbackResp.Success {
		t.Fatalf("callback failed: %s", callbackResp.Message)
	}
	var result service.OAuthCallbackResult
	decodeResponseData(t, callbackResp, &result)
	if result.Status != "link_required" {
		t.Fatalf("expected stale session to be treated as anonymous link_required flow, got %q", result.Status)
	}
	if result.CSRFToken == "" {
		t.Fatal("expected link_required response to include csrf_token")
	}
	if _, err := model.FindExternalAccount(source.ID, "4242"); err == nil {
		t.Fatal("expected stale session not to bind external account")
	}

	linkPayload, err := json.Marshal(map[string]string{
		"username": "root",
		"password": "new-password",
	})
	if err != nil {
		t.Fatalf("failed to marshal link payload: %v", err)
	}
	linkReq := httptest.NewRequest(http.MethodPost, "/api/oauth/link-existing", bytes.NewReader(linkPayload))
	linkReq.Header.Set("Content-Type", "application/json")
	linkReq.AddCookie(firstResponseCookie(t, callbackRecorder, "session"))
	linkRecorder := httptest.NewRecorder()
	engine.ServeHTTP(linkRecorder, linkReq)
	if linkRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected link status %d: %s", linkRecorder.Code, linkRecorder.Body.String())
	}
	var linkResp apiResponse
	if err := json.Unmarshal(linkRecorder.Body.Bytes(), &linkResp); err != nil {
		t.Fatalf("failed to decode link response: %v", err)
	}
	if linkResp.Success || !strings.Contains(linkResp.Message, "CSRF") {
		t.Fatalf("expected link without CSRF to be rejected, got %+v", linkResp)
	}

	linkReq = httptest.NewRequest(http.MethodPost, "/api/oauth/link-existing", bytes.NewReader(linkPayload))
	linkReq.Header.Set("Content-Type", "application/json")
	linkReq.Header.Set("X-CSRF-Token", result.CSRFToken)
	linkReq.AddCookie(firstResponseCookie(t, callbackRecorder, "session"))
	linkRecorder = httptest.NewRecorder()
	engine.ServeHTTP(linkRecorder, linkReq)
	if linkRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected link status %d with csrf: %s", linkRecorder.Code, linkRecorder.Body.String())
	}
	if err := json.Unmarshal(linkRecorder.Body.Bytes(), &linkResp); err != nil {
		t.Fatalf("failed to decode csrf link response: %v", err)
	}
	if !linkResp.Success {
		t.Fatalf("expected link with CSRF to succeed, got %s", linkResp.Message)
	}
}

func TestOAuthCallbackExchangeFailureIsGeneric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	source := &model.AuthSource{
		Name:         "github-main",
		Type:         model.AuthSourceTypeGitHub,
		DisplayName:  "GitHub",
		IsActive:     true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	if err := model.CreateAuthSource(source); err != nil {
		t.Fatalf("failed to create auth source: %v", err)
	}

	defer service.SetOAuthHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://github.com/login/oauth/access_token" {
				t.Fatalf("unexpected OAuth request: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 internal endpoint detail",
				Body:       io.NopCloser(strings.NewReader(`{"error":"internal endpoint detail"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	})()

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github-main/authorize", nil)
	authorizeRecorder := httptest.NewRecorder()
	engine.ServeHTTP(authorizeRecorder, authorizeReq)
	if authorizeRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected authorize status %d: %s", authorizeRecorder.Code, authorizeRecorder.Body.String())
	}
	var authorizeResp apiResponse
	if err := json.Unmarshal(authorizeRecorder.Body.Bytes(), &authorizeResp); err != nil {
		t.Fatalf("failed to decode authorize response: %v", err)
	}
	var authorizePayload map[string]string
	decodeResponseData(t, authorizeResp, &authorizePayload)
	authorizeURL, err := url.Parse(authorizePayload["authorize_url"])
	if err != nil {
		t.Fatalf("failed to parse authorize url: %v", err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("expected OAuth state")
	}
	oauthCookie := firstResponseCookie(t, authorizeRecorder, "session")

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/github-main/callback?code=oauth-code&state="+url.QueryEscape(state), nil)
	callbackReq.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	engine.ServeHTTP(callbackRecorder, callbackReq)
	if callbackRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected callback status %d: %s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
	var callbackResp apiResponse
	if err = json.Unmarshal(callbackRecorder.Body.Bytes(), &callbackResp); err != nil {
		t.Fatalf("failed to decode callback response: %v", err)
	}
	if callbackResp.Success {
		t.Fatal("expected exchange failure to fail")
	}
	if callbackResp.Message != "第三方授权失败，请返回登录页重试" {
		t.Fatalf("expected generic exchange failure, got %q", callbackResp.Message)
	}
	if strings.Contains(callbackResp.Message, "internal endpoint detail") {
		t.Fatalf("oauth callback must not reflect provider/internal error details, got %q", callbackResp.Message)
	}
}

func TestOAuthLinkExistingUsesLoginFailureLockout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldRegisterEnabled := common.RegisterEnabled
	oldCriticalRateLimitNum := common.CriticalRateLimitNum
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
		common.CriticalRateLimitNum = oldCriticalRateLimitNum
	})
	common.RegisterEnabled = false
	common.CriticalRateLimitNum = 1000

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	source, result, pendingCookie := createOAuthLinkRequiredSession(t, engine, "7777")
	for i := 0; i < 5; i++ {
		resp := performOAuthLinkExistingForTest(t, engine, pendingCookie, result.CSRFToken, "root", "wrong-password")
		if resp.Success {
			t.Fatal("expected wrong password OAuth link attempt to fail")
		}
		if strings.Contains(resp.Message, "次数过多") && i < 4 {
			t.Fatalf("OAuth link-existing locked too early on attempt %d: %s", i+1, resp.Message)
		}
	}

	resp := performOAuthLinkExistingForTest(t, engine, pendingCookie, result.CSRFToken, "root", "123456")
	if resp.Success || !strings.Contains(resp.Message, "次数过多") {
		t.Fatalf("expected OAuth link-existing to use login lockout, got %+v", resp)
	}
	if _, err := model.FindExternalAccount(source.ID, "7777"); err == nil {
		t.Fatal("expected locked OAuth link-existing request not to link account")
	}
}

func TestLoginLocksAccountSourceAfterRepeatedFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	attackerRemote := "198.51.100.10:1234"
	ownerRemote := "198.51.100.11:1234"

	for i := 0; i < 5; i++ {
		resp := performLoginNoFatalFrom(t, engine, "root", "wrong-password", attackerRemote)
		if resp.Success {
			t.Fatal("expected wrong password login to fail")
		}
		if strings.Contains(resp.Message, "次数过多") && i < 4 {
			t.Fatalf("account locked too early on attempt %d: %s", i+1, resp.Message)
		}
	}

	resp := performLoginNoFatalFrom(t, engine, "root", "123456", attackerRemote)
	if resp.Success || !strings.Contains(resp.Message, "次数过多") {
		t.Fatalf("expected locked source to reject even correct password, got %+v", resp)
	}

	resp = performLoginNoFatalFrom(t, engine, "root", "123456", ownerRemote)
	if !resp.Success {
		t.Fatalf("expected same account to remain usable from a different source, got %+v", resp)
	}
}

func TestPublicEmailActionsRequirePostBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	if err := model.DB.Model(&model.User{}).Where("username = ?", "root").Update("email", "root@example.com").Error; err != nil {
		t.Fatalf("set root email: %v", err)
	}

	getResp := performPublicJSONRequestNoFatal(t, engine, http.MethodGet, "/api/reset_password?email=missing@example.com")
	if getResp.Success || !strings.Contains(getResp.Message, "404") {
		t.Fatalf("expected legacy GET reset endpoint to be unavailable, got %+v", getResp)
	}

	postResp := performPublicJSONRequestNoFatal(t, engine, http.MethodPost, "/api/reset_password", map[string]any{
		"email": "missing@example.com",
	})
	if !postResp.Success {
		t.Fatalf("expected POST reset request for unknown email to be accepted, got %+v", postResp)
	}

	resetExistingResp := performPublicJSONRequestNoFatal(t, engine, http.MethodPost, "/api/reset_password", map[string]any{
		"email": "root@example.com",
	})
	if !resetExistingResp.Success || strings.Contains(strings.ToLower(resetExistingResp.Message), "smtp") {
		t.Fatalf("expected POST reset request for known email to avoid SMTP/account enumeration, got %+v", resetExistingResp)
	}

	verificationResp := performPublicJSONRequestNoFatal(t, engine, http.MethodPost, "/api/verification", map[string]any{
		"email": "root@example.com",
	})
	if !verificationResp.Success || strings.Contains(verificationResp.Message, "already in use") {
		t.Fatalf("expected POST verification to avoid account enumeration, got %+v", verificationResp)
	}

	newEmailVerificationResp := performPublicJSONRequestNoFatal(t, engine, http.MethodPost, "/api/verification", map[string]any{
		"email": "new-public@example.com",
	})
	if !newEmailVerificationResp.Success || strings.Contains(strings.ToLower(newEmailVerificationResp.Message), "smtp") {
		t.Fatalf("expected POST verification to avoid SMTP/config leakage, got %+v", newEmailVerificationResp)
	}
}

func TestEmailBindVerificationAvoidsAccountEnumeration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	other := model.User{
		Username: "other-user",
		Password: "irrelevant-password",
		Email:    "taken@example.com",
	}
	if err := other.Insert(); err != nil {
		t.Fatalf("create secondary user: %v", err)
	}

	sessionCookie := loginAsRoot(t, engine)
	resp := performSessionJSONRequestNoFatal(t, engine, sessionCookie, http.MethodPost, "/api/oauth/email/verification", map[string]any{
		"email": "taken@example.com",
	})
	if !resp.Success || strings.Contains(resp.Message, "already in use") {
		t.Fatalf("expected email bind verification to avoid account enumeration, got %+v", resp)
	}
}

func TestWeChatAuthEncodesCodeAndRegistersRandomUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.WeChatAuthEnabled
	oldRegisterEnabled := common.RegisterEnabled
	oldServerAddress := common.WeChatServerAddress
	oldServerToken := common.WeChatServerToken
	t.Cleanup(func() {
		common.WeChatAuthEnabled = oldEnabled
		common.RegisterEnabled = oldRegisterEnabled
		common.WeChatServerAddress = oldServerAddress
		common.WeChatServerToken = oldServerToken
	})
	common.WeChatAuthEnabled = true
	common.RegisterEnabled = true
	common.WeChatServerToken = "wechat-token"

	const oauthCode = "wx code+slash/?"
	const wechatID = "wechat-openid-1"
	wechatServerCalled := false
	wechatServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wechatServerCalled = true
		if r.URL.Path != "/api/wechat/user" {
			t.Errorf("expected wechat user path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("code"); got != oauthCode {
			t.Errorf("expected encoded code to round-trip, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != common.WeChatServerToken {
			t.Errorf("expected authorization header %q, got %q", common.WeChatServerToken, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":"` + wechatID + `"}`))
	}))
	t.Cleanup(wechatServer.Close)
	defer controller.SetWeChatHTTPClientForTest(wechatClientForTLSServer(t, wechatServer))()
	common.WeChatServerAddress = "https://wechat.example.test"

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	state, sessionCookie := authorizeWeChatOAuthForTest(t, engine)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code="+url.QueryEscape(oauthCode)+"&state="+url.QueryEscape(state), nil)
	req.AddCookie(sessionCookie)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal wechat response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected wechat login to succeed, got %s", resp.Message)
	}
	if !wechatServerCalled {
		t.Fatal("expected wechat server to be called")
	}
	var loggedInUser model.User
	decodeResponseData(t, resp, &loggedInUser)
	if !strings.HasPrefix(loggedInUser.Username, "wx_") || len(loggedInUser.Username) != 11 {
		t.Fatalf("expected short random wechat username, got %q", loggedInUser.Username)
	}

	storedUser := model.User{WeChatId: wechatID}
	if err := storedUser.FillUserByWeChatId(); err != nil {
		t.Fatalf("expected stored wechat user: %v", err)
	}
	if storedUser.Username != loggedInUser.Username {
		t.Fatalf("expected response user %q to match stored user %q", loggedInUser.Username, storedUser.Username)
	}
}

func TestWeChatAuthRejectsMissingState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.WeChatAuthEnabled
	oldRegisterEnabled := common.RegisterEnabled
	oldServerAddress := common.WeChatServerAddress
	oldServerToken := common.WeChatServerToken
	t.Cleanup(func() {
		common.WeChatAuthEnabled = oldEnabled
		common.RegisterEnabled = oldRegisterEnabled
		common.WeChatServerAddress = oldServerAddress
		common.WeChatServerToken = oldServerToken
	})
	common.WeChatAuthEnabled = true
	common.RegisterEnabled = true
	common.WeChatServerToken = "wechat-token"

	wechatServerCalled := false
	wechatServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wechatServerCalled = true
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":"wechat-openid"}`))
	}))
	t.Cleanup(wechatServer.Close)
	defer controller.SetWeChatHTTPClientForTest(wechatClientForTLSServer(t, wechatServer))()
	common.WeChatServerAddress = "https://wechat.example.test"

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	resp := performPublicJSONRequestNoFatal(t, engine, http.MethodGet, "/api/oauth/wechat?code=stolen-code")
	if resp.Success {
		t.Fatal("expected wechat login without state to fail")
	}
	if wechatServerCalled {
		t.Fatal("wechat upstream must not be called before state validation")
	}
}

func TestWeChatAuthRejectsUpstreamHTTPError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	oldEnabled := common.WeChatAuthEnabled
	oldRegisterEnabled := common.RegisterEnabled
	oldServerAddress := common.WeChatServerAddress
	oldServerToken := common.WeChatServerToken
	t.Cleanup(func() {
		common.WeChatAuthEnabled = oldEnabled
		common.RegisterEnabled = oldRegisterEnabled
		common.WeChatServerAddress = oldServerAddress
		common.WeChatServerToken = oldServerToken
	})
	common.WeChatAuthEnabled = true
	common.RegisterEnabled = true
	common.WeChatServerToken = "wechat-token"

	wechatServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream broken", http.StatusInternalServerError)
	}))
	t.Cleanup(wechatServer.Close)
	defer controller.SetWeChatHTTPClientForTest(wechatClientForTLSServer(t, wechatServer))()
	common.WeChatServerAddress = "https://wechat.example.test"

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	state, sessionCookie := authorizeWeChatOAuthForTest(t, engine)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat?code=abc&state="+url.QueryEscape(state), nil)
	req.AddCookie(sessionCookie)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal wechat response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected wechat login to fail when upstream returns 500")
	}
	if !strings.Contains(resp.Message, "微信登录服务返回异常状态") {
		t.Fatalf("expected upstream status error message, got %q", resp.Message)
	}
	if strings.Contains(resp.Message, "upstream broken") {
		t.Fatalf("expected upstream body to be hidden, got %q", resp.Message)
	}
}

func TestSessionAuthRejectsDisabledCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)
	if err := model.DB.Model(&model.User{}).Where("username = ?", "root").Update("status", common.UserStatusDisabled).Error; err != nil {
		t.Fatalf("failed to disable root user: %v", err)
	}

	resp := performSessionJSONRequestNoFatal(t, engine, loginCookie, http.MethodGet, "/api/user/self", nil)
	if resp.Success {
		t.Fatal("expected disabled user session to be rejected")
	}
	if !strings.Contains(resp.Message, "用户已被封禁") && !strings.Contains(resp.Message, "登录状态已失效") {
		t.Fatalf("expected disabled or expired session message, got %q", resp.Message)
	}
}

func TestSessionAuthUsesCurrentRoleAfterDowngrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)
	if err := model.DB.Model(&model.User{}).Where("username = ?", "root").Update("role", common.RoleCommonUser).Error; err != nil {
		t.Fatalf("failed to downgrade root user: %v", err)
	}

	resp := performSessionJSONRequestNoFatal(t, engine, loginCookie, http.MethodGet, "/api/option/", nil)
	if resp.Success {
		t.Fatal("expected downgraded root session to be rejected by root route")
	}
	if !strings.Contains(resp.Message, "权限不足") {
		t.Fatalf("expected permission denied message, got %q", resp.Message)
	}
}

func TestSessionAuthRejectsPasswordChangedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	loginCookie := loginAsRoot(t, engine)
	if err := model.ResetUserPasswordByUsername("root", "new-password"); err != nil {
		t.Fatalf("failed to reset root password: %v", err)
	}

	resp := performSessionJSONRequestNoFatal(t, engine, loginCookie, http.MethodGet, "/api/user/self", nil)
	if resp.Success {
		t.Fatal("expected old session to be rejected after password change")
	}
}

func TestBearerTokenRejectedForHighRiskAdminRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	req := httptest.NewRequest(http.MethodPost, "/api/config-versions/publish", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success || !strings.Contains(resp.Message, "token") {
		t.Fatalf("expected bearer token to be rejected, got %+v", resp)
	}
}

func firstResponseCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, item := range recorder.Result().Cookies() {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("expected %s cookie", name)
	return nil
}

func createOAuthLinkRequiredSession(t *testing.T, engine http.Handler, externalID string) (*model.AuthSource, service.OAuthCallbackResult, *http.Cookie) {
	t.Helper()
	oldRegisterEnabled := common.RegisterEnabled
	common.RegisterEnabled = false
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
	})

	source := &model.AuthSource{
		Name:         "github-link-" + externalID,
		Type:         model.AuthSourceTypeGitHub,
		DisplayName:  "GitHub",
		IsActive:     true,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}
	if err := model.CreateAuthSource(source); err != nil {
		t.Fatalf("failed to create auth source: %v", err)
	}

	defer service.SetOAuthHTTPClientForTest(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://github.com/login/oauth/access_token":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"access_token":"oauth-token"}`)),
					Header:     make(http.Header),
				}, nil
			case "https://api.github.com/user":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":` + externalID + `,"login":"attacker","name":"Attacker","email":"attacker@example.com"}`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected OAuth request: %s", req.URL.String())
				return nil, nil
			}
		}),
	})()

	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/oauth/"+source.Name+"/authorize", nil)
	authorizeRecorder := httptest.NewRecorder()
	engine.ServeHTTP(authorizeRecorder, authorizeReq)
	if authorizeRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected authorize status %d: %s", authorizeRecorder.Code, authorizeRecorder.Body.String())
	}
	var authorizeResp apiResponse
	if err := json.Unmarshal(authorizeRecorder.Body.Bytes(), &authorizeResp); err != nil {
		t.Fatalf("failed to decode authorize response: %v", err)
	}
	if !authorizeResp.Success {
		t.Fatalf("authorize failed: %s", authorizeResp.Message)
	}
	var authorizePayload map[string]string
	decodeResponseData(t, authorizeResp, &authorizePayload)
	authorizeURL, err := url.Parse(authorizePayload["authorize_url"])
	if err != nil {
		t.Fatalf("failed to parse authorize url: %v", err)
	}
	state := authorizeURL.Query().Get("state")
	if state == "" {
		t.Fatal("expected OAuth state")
	}
	oauthCookie := firstResponseCookie(t, authorizeRecorder, "session")

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/"+source.Name+"/callback?code=oauth-code&state="+url.QueryEscape(state), nil)
	callbackReq.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	engine.ServeHTTP(callbackRecorder, callbackReq)
	if callbackRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected callback status %d: %s", callbackRecorder.Code, callbackRecorder.Body.String())
	}
	var callbackResp apiResponse
	if err := json.Unmarshal(callbackRecorder.Body.Bytes(), &callbackResp); err != nil {
		t.Fatalf("failed to decode callback response: %v", err)
	}
	if !callbackResp.Success {
		t.Fatalf("callback failed: %s", callbackResp.Message)
	}
	var result service.OAuthCallbackResult
	decodeResponseData(t, callbackResp, &result)
	if result.Status != "link_required" {
		t.Fatalf("expected link_required flow, got %q", result.Status)
	}
	if result.CSRFToken == "" {
		t.Fatal("expected link_required response to include csrf_token")
	}
	return source, result, firstResponseCookie(t, callbackRecorder, "session")
}

func performOAuthLinkExistingForTest(t *testing.T, engine http.Handler, sessionCookie *http.Cookie, csrfToken string, username string, password string) apiResponse {
	t.Helper()
	linkPayload, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		t.Fatalf("failed to marshal link payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/link-existing", bytes.NewReader(linkPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected link-existing status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode link-existing response: %v", err)
	}
	return resp
}

func authorizeWeChatOAuthForTest(t *testing.T, engine http.Handler) (string, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/wechat/authorize", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected wechat authorize status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal wechat authorize response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("wechat authorize failed: %s", resp.Message)
	}
	var payload map[string]string
	decodeResponseData(t, resp, &payload)
	state := payload["state"]
	if state == "" {
		t.Fatal("expected wechat oauth state")
	}
	return state, firstResponseCookie(t, recorder, "session")
}

func wechatClientForTLSServer(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse TLS server URL: %v", err)
	}
	baseClient := server.Client()
	baseTransport := baseClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	baseClient.Transport = wechatTestTransport{
		base:     baseTransport,
		upstream: upstream,
	}
	return baseClient
}

type wechatTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (transport wechatTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rewritten := request.Clone(request.Context())
	rewritten.URL = cloneURL(request.URL)
	rewritten.URL.Scheme = transport.upstream.Scheme
	rewritten.URL.Host = transport.upstream.Host
	rewritten.Host = request.URL.Host
	return transport.base.RoundTrip(rewritten)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	clone := *value
	return &clone
}

func loginAsRoot(t *testing.T, engine http.Handler) *http.Cookie {
	t.Helper()
	recorder, resp := performLoginRequest(t, engine, "root", "123456")
	if !resp.Success {
		t.Fatalf("root login failed: %s", resp.Message)
	}

	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session" {
			return cookie
		}
	}
	t.Fatal("expected session cookie after root login")
	return nil
}

func performLoginNoFatal(t *testing.T, engine http.Handler, username string, password string) apiResponse {
	t.Helper()
	_, resp := performLoginRequest(t, engine, username, password)
	return resp
}

func performLoginNoFatalFrom(t *testing.T, engine http.Handler, username string, password string, remoteAddr string) apiResponse {
	t.Helper()
	_, resp := performLoginRequestFrom(t, engine, username, password, remoteAddr)
	return resp
}

func performLoginRequest(t *testing.T, engine http.Handler, username string, password string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()
	return performLoginRequestFrom(t, engine, username, password, "")
}

func performLoginRequestFrom(t *testing.T, engine http.Handler, username string, password string, remoteAddr string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"username": username,
		"password": password,
	})
	if err != nil {
		t.Fatalf("failed to marshal login payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected login status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	return recorder, resp
}

func csrfTokenForSession(t *testing.T, engine http.Handler, sessionCookie *http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	req.AddCookie(sessionCookie)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected csrf bootstrap status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode csrf bootstrap response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("csrf bootstrap failed: %s", resp.Message)
	}
	var data struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("failed to decode csrf token: %v", err)
	}
	if data.CSRFToken == "" {
		t.Fatal("expected csrf token in /api/user/self response")
	}
	return data.CSRFToken
}

func addSessionCSRFHeader(t *testing.T, engine http.Handler, req *http.Request, sessionCookie *http.Cookie) {
	t.Helper()
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		return
	}
	req.Header.Set("X-CSRF-Token", csrfTokenForSession(t, engine, sessionCookie))
}

func performSessionJSONRequest(t *testing.T, engine http.Handler, sessionCookie *http.Cookie, method string, path string, body any) apiResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(sessionCookie)
	addSessionCSRFHeader(t, engine, req, sessionCookie)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d for %s %s: %s", recorder.Code, method, path, recorder.Body.String())
	}

	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("request %s %s failed: %s", method, path, resp.Message)
	}
	return resp
}

func performSessionJSONRequestNoFatal(t *testing.T, engine http.Handler, sessionCookie *http.Cookie, method string, path string, body any) apiResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(sessionCookie)
	addSessionCSRFHeader(t, engine, req, sessionCookie)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status %d for %s %s: %s", recorder.Code, method, path, recorder.Body.String())
	}

	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	return resp
}

func performPublicJSONRequestNoFatal(t *testing.T, engine http.Handler, method string, path string, body ...any) apiResponse {
	t.Helper()
	var reader io.Reader
	if len(body) > 0 && body[0] != nil {
		payload, err := json.Marshal(body[0])
		if err != nil {
			t.Fatalf("failed to marshal public request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d for %s %s: %s", recorder.Code, method, path, recorder.Body.String())
	}
	if recorder.Code == http.StatusNotFound {
		return apiResponse{Success: false, Message: "404 not found"}
	}
	var resp apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	return resp
}

func TestPublicStatusOmitsRuntimeMetadataByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)
	model.InitOptionMap()

	oldExposeRuntimeMetadata := common.PublicStatusRuntimeMetadataEnabled
	oldVersion := common.Version
	oldStartTime := common.StartTime
	oldServerAddress := common.ServerAddress
	t.Cleanup(func() {
		common.PublicStatusRuntimeMetadataEnabled = oldExposeRuntimeMetadata
		common.Version = oldVersion
		common.StartTime = oldStartTime
		common.ServerAddress = oldServerAddress
	})

	common.PublicStatusRuntimeMetadataEnabled = false
	common.Version = "v9.9.9-test"
	common.StartTime = 1781080000
	common.ServerAddress = "https://panel.example.test"

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	resp := performPublicJSONRequestNoFatal(t, engine, http.MethodGet, "/api/status")
	if !resp.Success {
		t.Fatalf("expected public status to succeed: %s", resp.Message)
	}

	var data map[string]any
	decodeResponseData(t, resp, &data)
	for _, key := range []string{"version", "start_time", "server_address"} {
		if _, ok := data[key]; ok {
			t.Fatalf("expected public status to omit %s by default, got %#v", key, data[key])
		}
	}
	if _, ok := data["register_enabled"]; !ok {
		t.Fatal("expected public status to keep login/register capability flags")
	}

	common.PublicStatusRuntimeMetadataEnabled = true
	resp = performPublicJSONRequestNoFatal(t, engine, http.MethodGet, "/api/status")
	decodeResponseData(t, resp, &data)
	if data["version"] != common.Version {
		t.Fatalf("expected version metadata when enabled, got %#v", data["version"])
	}
	if data["server_address"] != common.ServerAddress {
		t.Fatalf("expected server address metadata when enabled, got %#v", data["server_address"])
	}
}

func TestPhase2AgentLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	adminToken := prepareRootToken(t)

	createRouteAndPublishVersion(t, engine, adminToken)

	dashboardResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/dashboard/overview", nil)
	var dashboard struct {
		Summary service.DashboardSummary `json:"summary"`
	}
	decodeResponseData(t, dashboardResp, &dashboard)
	if dashboard.Summary.TotalNodes != 0 {
		t.Fatalf("expected empty dashboard node summary before node registration, got %+v", dashboard.Summary)
	}

	unauthorizedRequest := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/register", bytes.NewReader([]byte(`{}`)))
	unauthorizedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(unauthorizedRecorder, unauthorizedRequest)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status for missing discovery token, got %d", unauthorizedRecorder.Code)
	}

	createdNodeResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/", map[string]any{
		"name":                "shanghai-edge-1",
		"geo_manual_override": true,
		"geo_name":            "Shanghai",
		"geo_latitude":        31.2304,
		"geo_longitude":       121.4737,
	})
	var createdNode service.NodeView
	decodeResponseData(t, createdNodeResp, &createdNode)
	if createdNode.AgentToken == "" || createdNode.Status != service.NodeStatusPending {
		t.Fatal("expected created node to expose agent token with pending status")
	}
	if createdNode.GeoName != "Shanghai" || createdNode.GeoLatitude == nil || createdNode.GeoLongitude == nil {
		t.Fatalf("expected created node to expose geo metadata, got %+v", createdNode)
	}

	heartbeatPayload := map[string]any{
		"node_id":           "spoofed-node-id",
		"name":              "shanghai-edge-1",
		"ip":                "10.0.0.9",
		"agent_version":     "0.1.1",
		"nginx_version":     "1.27.1.2",
		"openresty_status":  service.OpenrestyStatusUnhealthy,
		"openresty_message": "docker run openresty failed: bind 80 already allocated",
		"current_version":   "",
		"last_error":        "",
	}
	resp := performAgentJSONRequestWithTokenAndRemote(t, engine, createdNode.AgentToken, http.MethodPost, "/api/agent/nodes/heartbeat", heartbeatPayload, "198.51.100.10:1234")
	var registeredNode model.Node
	decodeResponseData(t, resp, &registeredNode)
	if registeredNode.IP != "198.51.100.10" || registeredNode.AgentVersion != "0.1.1" || registeredNode.NodeID != createdNode.NodeID {
		t.Fatal("expected heartbeat to update node metadata")
	}
	if registeredNode.OpenrestyStatus != service.OpenrestyStatusUnhealthy {
		t.Fatal("expected heartbeat to update openresty status")
	}

	activeConfigResp := performAgentJSONRequestWithToken(t, engine, createdNode.AgentToken, http.MethodGet, "/api/agent/config-versions/active", nil)
	var activeConfig service.AgentConfigResponse
	decodeResponseData(t, activeConfigResp, &activeConfig)
	if activeConfig.Version == "" || activeConfig.RenderedConfig == "" || activeConfig.Checksum == "" {
		t.Fatal("expected active config response to contain version payload")
	}

	successApplyResp := performAgentJSONRequestWithToken(t, engine, createdNode.AgentToken, http.MethodPost, "/api/agent/apply-logs", map[string]any{
		"node_id": "spoofed-node-id",
		"version": activeConfig.Version,
		"result":  service.ApplyResultOK,
		"message": "apply ok",
	})
	var successApplyLog model.ApplyLog
	decodeResponseData(t, successApplyResp, &successApplyLog)
	if successApplyLog.Result != service.ApplyResultOK {
		t.Fatal("expected apply log success to be recorded")
	}

	failedApplyResp := performAgentJSONRequestWithToken(t, engine, createdNode.AgentToken, http.MethodPost, "/api/agent/apply-logs", map[string]any{
		"node_id": "spoofed-node-id",
		"version": activeConfig.Version,
		"result":  service.ApplyResultFailed,
		"message": "openresty reload failed",
	})
	var failedApplyLog model.ApplyLog
	decodeResponseData(t, failedApplyResp, &failedApplyLog)
	if failedApplyLog.Result != service.ApplyResultFailed {
		t.Fatal("expected failed apply log to be recorded")
	}

	nodesResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/nodes/", nil)
	var nodes []service.NodeView
	decodeResponseData(t, nodesResp, &nodes)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Status != service.NodeStatusOnline {
		t.Fatal("expected registered node to become online")
	}
	if nodes[0].AgentToken != "" || !nodes[0].AgentTokenAvailable || nodes[0].AgentTokenPrefix == "" {
		t.Fatal("expected node list to hide full agent token and expose only token status")
	}
	if nodes[0].LatestApplyResult != service.ApplyResultFailed || nodes[0].LatestApplyMessage != "openresty reload failed" {
		t.Fatal("expected node list to expose latest apply status")
	}
	if nodes[0].CurrentVersion != activeConfig.Version {
		t.Fatal("expected node current_version to remain at last successful version")
	}
	if nodes[0].LastError != "openresty reload failed" {
		t.Fatal("expected node last_error to reflect failed apply")
	}
	if nodes[0].OpenrestyStatus != service.OpenrestyStatusUnhealthy {
		t.Fatal("expected node list to expose openresty status")
	}
	if nodes[0].OpenrestyMessage != "docker run openresty failed: bind 80 already allocated" {
		t.Fatal("expected node list to expose openresty message")
	}

	if err := model.DB.Create(&model.NodeHealthEvent{
		NodeID:           createdNode.NodeID,
		EventType:        "openresty_down",
		Severity:         service.NodeHealthSeverityCritical,
		Status:           service.NodeHealthEventStatusActive,
		Message:          "docker run openresty failed: bind 80 already allocated",
		FirstTriggeredAt: time.Now().Add(-2 * time.Minute),
		LastTriggeredAt:  time.Now().Add(-time.Minute),
		ReportedAt:       time.Now().Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("failed to insert node health event: %v", err)
	}

	observabilityResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/nodes/"+toString(createdNode.ID)+"/observability?hours=24&limit=20", nil)
	var observability service.NodeObservabilityView
	decodeResponseData(t, observabilityResp, &observability)
	if observability.NodeID != createdNode.NodeID {
		t.Fatalf("expected observability response for node %s, got %s", createdNode.NodeID, observability.NodeID)
	}
	if len(observability.HealthEvents) != 1 {
		t.Fatalf("expected observability response to include health events, got %+v", observability.HealthEvents)
	}

	cleanupHealthResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/"+toString(createdNode.ID)+"/observability/cleanup", nil)
	var cleanupHealthResult service.NodeHealthEventCleanupResult
	decodeResponseData(t, cleanupHealthResp, &cleanupHealthResult)
	if cleanupHealthResult.NodeID != createdNode.NodeID || cleanupHealthResult.DeletedCount != 1 {
		t.Fatalf("unexpected node health cleanup result: %+v", cleanupHealthResult)
	}

	observabilityAfterCleanupResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/nodes/"+toString(createdNode.ID)+"/observability?hours=24&limit=20", nil)
	decodeResponseData(t, observabilityAfterCleanupResp, &observability)
	if len(observability.HealthEvents) != 0 {
		t.Fatalf("expected health events to be cleaned up, got %+v", observability.HealthEvents)
	}

	restartResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/"+toString(createdNode.ID)+"/openresty-restart", nil)
	decodeResponseData(t, restartResp, &createdNode)
	if !createdNode.RestartOpenrestyRequested {
		t.Fatal("expected openresty restart request flag to be set")
	}

	rawHeartbeatPayload, err := json.Marshal(heartbeatPayload)
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}
	restartHeartbeatReq := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/heartbeat", bytes.NewReader(rawHeartbeatPayload))
	restartHeartbeatReq.Header.Set("Content-Type", "application/json")
	restartHeartbeatReq.Header.Set("X-Agent-Token", createdNode.AgentToken)
	restartHeartbeatReq.RemoteAddr = "198.51.100.10:1234"
	restartHeartbeatRecorder := httptest.NewRecorder()
	engine.ServeHTTP(restartHeartbeatRecorder, restartHeartbeatReq)
	if restartHeartbeatRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected heartbeat status %d: %s", restartHeartbeatRecorder.Code, restartHeartbeatRecorder.Body.String())
	}
	var restartHeartbeatBody struct {
		Success       bool                      `json:"success"`
		Message       string                    `json:"message"`
		AgentSettings service.AgentSettings     `json:"agent_settings"`
		ActiveConfig  *service.ActiveConfigMeta `json:"active_config"`
	}
	if err = json.Unmarshal(restartHeartbeatRecorder.Body.Bytes(), &restartHeartbeatBody); err != nil {
		t.Fatalf("failed to decode heartbeat response: %v", err)
	}
	if !restartHeartbeatBody.Success {
		t.Fatalf("expected heartbeat request success, got %s", restartHeartbeatBody.Message)
	}
	if !restartHeartbeatBody.AgentSettings.RestartOpenrestyNow {
		t.Fatal("expected heartbeat response to instruct openresty restart")
	}
	if restartHeartbeatBody.ActiveConfig == nil || restartHeartbeatBody.ActiveConfig.Version == "" || restartHeartbeatBody.ActiveConfig.Checksum == "" {
		t.Fatal("expected heartbeat response to include active config summary")
	}

	logsResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/apply-logs/?node_id="+createdNode.NodeID+"&pageNo=1&pageSize=1", nil)
	var logs service.ApplyLogListResult
	decodeResponseData(t, logsResp, &logs)
	if logs.Current != 1 || logs.Total != 2 || logs.TotalPage != 2 {
		t.Fatalf("unexpected paged apply logs result: %+v", logs)
	}
	if len(logs.Rows) != 1 {
		t.Fatalf("expected 1 apply log row on page 1, got %d", len(logs.Rows))
	}
	if logs.Rows[0].Result != service.ApplyResultFailed {
		t.Fatalf("expected newest apply log first, got %s", logs.Rows[0].Result)
	}
	oldApplyLogTime := time.Now().Add(-48 * time.Hour)
	if err := model.DB.Model(&model.ApplyLog{}).Where("id = ?", successApplyLog.ID).Update("created_at", oldApplyLogTime).Error; err != nil {
		t.Fatalf("failed to backdate apply log: %v", err)
	}
	cleanupResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/apply-logs/cleanup", map[string]any{
		"retention_days": 1,
	})
	var cleanupResult service.ApplyLogCleanupResult
	decodeResponseData(t, cleanupResp, &cleanupResult)
	if cleanupResult.DeleteAll {
		t.Fatal("expected retention cleanup instead of delete-all cleanup")
	}
	if cleanupResult.RetentionDays != 1 || cleanupResult.DeletedCount != 1 {
		t.Fatalf("unexpected cleanup result: %+v", cleanupResult)
	}
	postCleanupResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/apply-logs/?node_id="+createdNode.NodeID, nil)
	decodeResponseData(t, postCleanupResp, &logs)
	if logs.Total != 1 || len(logs.Rows) != 1 {
		t.Fatalf("expected one apply log after retention cleanup, got %+v", logs)
	}
	deleteAllResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/apply-logs/cleanup", map[string]any{
		"delete_all": true,
	})
	decodeResponseData(t, deleteAllResp, &cleanupResult)
	if !cleanupResult.DeleteAll || cleanupResult.DeletedCount != 1 {
		t.Fatalf("unexpected delete-all cleanup result: %+v", cleanupResult)
	}
	emptyLogsResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/apply-logs/?node_id="+createdNode.NodeID, nil)
	decodeResponseData(t, emptyLogsResp, &logs)
	if logs.Total != 0 || len(logs.Rows) != 0 || logs.Current != 1 || logs.TotalPage != 0 {
		t.Fatalf("expected empty apply log page after delete-all cleanup, got %+v", logs)
	}

	updatedNodeResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/"+toString(createdNode.ID)+"/update", map[string]any{
		"name":                "shanghai-edge-1-renamed",
		"geo_manual_override": true,
		"geo_name":            "Tokyo",
		"geo_latitude":        35.6762,
		"geo_longitude":       139.6503,
	})
	decodeResponseData(t, updatedNodeResp, &createdNode)
	if createdNode.Name != "shanghai-edge-1-renamed" {
		t.Fatal("expected node name to be editable")
	}
	if createdNode.GeoName != "Tokyo" || createdNode.GeoLatitude == nil || createdNode.GeoLongitude == nil {
		t.Fatalf("expected node geo metadata to be editable, got %+v", createdNode)
	}

	oldTime := time.Now().Add(-common.NodeOfflineThreshold - time.Minute)
	if err := model.DB.Model(&model.Node{}).Where("node_id = ?", createdNode.NodeID).Update("last_seen_at", oldTime).Error; err != nil {
		t.Fatalf("failed to update node last_seen_at: %v", err)
	}
	nodesResp = performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/nodes/", nil)
	decodeResponseData(t, nodesResp, &nodes)
	if nodes[0].Status != service.NodeStatusOffline {
		t.Fatal("expected node to be shown as offline after timeout")
	}

	deleteResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/"+toString(createdNode.ID)+"/delete", nil)
	if !deleteResp.Success {
		t.Fatalf("expected delete node success, got %s", deleteResp.Message)
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/heartbeat", bytes.NewReader([]byte(`{"ip":"10.0.0.9","agent_version":"0.1.1"}`)))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.Header.Set("X-Agent-Token", createdNode.AgentToken)
	deniedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted node token to be rejected, got %d", deniedRecorder.Code)
	}
}

func TestPhase2CustomHeadersPreviewAndDiffLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)

	createResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/", map[string]any{
		"domain":      "preview.example.com",
		"origin_url":  "https://origin-a.internal",
		"origin_host": "preview-origin.internal",
		"enabled":     true,
		"custom_headers": []map[string]any{
			{"key": "X-Trace-Id", "value": "$request_id"},
			{"key": "Authorization", "value": "Bearer preview-secret"},
		},
	})
	var createdRoute service.ProxyRouteView
	decodeResponseData(t, createResp, &createdRoute)
	if !strings.Contains(createdRoute.CustomHeaders, "X-Trace-Id") {
		t.Fatalf("expected custom headers to be stored as json, got %s", createdRoute.CustomHeaders)
	}
	if createdRoute.OriginHost != "preview-origin.internal" {
		t.Fatalf("expected origin_host to be stored, got %s", createdRoute.OriginHost)
	}
	if createdRoute.SiteName != "preview.example.com" || createdRoute.PrimaryDomain != "preview.example.com" || createdRoute.DomainCount != 1 {
		t.Fatalf("expected website identity fields in create response, got %+v", createdRoute)
	}

	performJSONRequest(t, engine, token, http.MethodPost, "/api/config-versions/publish", nil)

	performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/"+toString(createdRoute.ID)+"/update", map[string]any{
		"domain":      "preview.example.com",
		"origin_url":  "https://origin-b.internal",
		"origin_host": "preview-upstream.internal",
		"enabled":     true,
		"custom_headers": []map[string]any{
			{"key": "X-Trace-Id", "value": "$request_id"},
			{"key": "X-Release", "value": "candidate"},
			{"key": "Authorization", "value": "[redacted sensitive header; preserved on save]"},
		},
	})
	performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/", map[string]any{
		"domain":     "new-preview.example.com",
		"origin_url": "https://origin-new.internal",
		"enabled":    true,
	})

	previewResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/config-versions/preview", nil)
	var preview map[string]any
	decodeResponseData(t, previewResp, &preview)
	renderedConfig, _ := preview["rendered_config"].(string)
	if websiteCount, ok := preview["website_count"].(float64); !ok || int(websiteCount) != 2 {
		t.Fatalf("expected preview website_count=2, got %#v", preview["website_count"])
	}
	if !strings.Contains(renderedConfig, `proxy_set_header X-Release "candidate";`) {
		t.Fatalf("expected preview endpoint to return custom header, got %s", renderedConfig)
	}
	if strings.Contains(renderedConfig, "Bearer preview-secret") {
		t.Fatalf("expected preview endpoint to redact sensitive custom header, got %s", renderedConfig)
	}
	routeConfig, _ := preview["route_config"].(string)
	if strings.Contains(routeConfig, "Bearer preview-secret") {
		t.Fatalf("expected preview route_config to redact sensitive custom header, got %s", routeConfig)
	}
	if !strings.Contains(renderedConfig, `proxy_set_header Host "preview-upstream.internal";`) {
		t.Fatalf("expected preview endpoint to return overridden host header, got %s", renderedConfig)
	}
	if !strings.Contains(renderedConfig, "proxy_ssl_server_name on;") {
		t.Fatalf("expected preview endpoint to enable proxy ssl server name, got %s", renderedConfig)
	}
	if !strings.Contains(renderedConfig, `proxy_ssl_name "preview-upstream.internal";`) {
		t.Fatalf("expected preview endpoint to return proxy ssl name, got %s", renderedConfig)
	}

	diffResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/config-versions/diff", nil)
	var diff map[string]any
	decodeResponseData(t, diffResp, &diff)
	modifiedDomains, ok := diff["modified_domains"].([]any)
	if !ok || len(modifiedDomains) != 1 || modifiedDomains[0].(string) != "preview.example.com" {
		t.Fatalf("unexpected modified domains: %#v", diff["modified_domains"])
	}
	addedDomains, ok := diff["added_domains"].([]any)
	if !ok || len(addedDomains) != 1 || addedDomains[0].(string) != "new-preview.example.com" {
		t.Fatalf("unexpected added domains: %#v", diff["added_domains"])
	}
	modifiedSites, ok := diff["modified_sites"].([]any)
	if !ok || len(modifiedSites) != 1 || modifiedSites[0].(string) != "preview.example.com" {
		t.Fatalf("unexpected modified sites: %#v", diff["modified_sites"])
	}
	addedSites, ok := diff["added_sites"].([]any)
	if !ok || len(addedSites) != 1 || addedSites[0].(string) != "new-preview.example.com" {
		t.Fatalf("unexpected added sites: %#v", diff["added_sites"])
	}
	if snapshotChanged, ok := diff["snapshot_changed"].(bool); !ok || !snapshotChanged {
		t.Fatalf("expected snapshot_changed=true, got %#v", diff["snapshot_changed"])
	}
	if runtimeConfigChanged, ok := diff["runtime_config_changed"].(bool); !ok || !runtimeConfigChanged {
		t.Fatalf("expected runtime_config_changed=true, got %#v", diff["runtime_config_changed"])
	}
}

func TestPhase2ProxyRouteWebsiteDetailAndLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)

	createResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/", map[string]any{
		"site_name":             "marketing-site",
		"domains":               []string{"app.example.com", "www.example.com"},
		"origin_url":            "https://origin.internal",
		"enabled":               true,
		"limit_conn_per_server": 120,
		"limit_conn_per_ip":     12,
		"limit_rate":            "512K",
	})
	var createdRoute service.ProxyRouteView
	decodeResponseData(t, createResp, &createdRoute)
	if createdRoute.SiteName != "marketing-site" || createdRoute.PrimaryDomain != "app.example.com" {
		t.Fatalf("unexpected create payload: %+v", createdRoute)
	}
	if createdRoute.DomainCount != 2 || len(createdRoute.Domains) != 2 || createdRoute.Domains[1] != "www.example.com" {
		t.Fatalf("expected multi-domain website view, got %+v", createdRoute)
	}
	if createdRoute.LimitConnPerServer != 120 || createdRoute.LimitConnPerIP != 12 || createdRoute.LimitRate != "512k" {
		t.Fatalf("expected normalized rate limit fields, got %+v", createdRoute)
	}
	if len(createdRoute.UpstreamList) != 1 || createdRoute.UpstreamList[0] != "https://origin.internal" {
		t.Fatalf("expected structured upstream list, got %+v", createdRoute.UpstreamList)
	}

	detailResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/proxy-routes/"+toString(createdRoute.ID), nil)
	var detail service.ProxyRouteView
	decodeResponseData(t, detailResp, &detail)
	if detail.ID != createdRoute.ID || detail.SiteName != "marketing-site" || detail.LimitRate != "512k" {
		t.Fatalf("unexpected detail response: %+v", detail)
	}
	if len(detail.Domains) != 2 || detail.Domains[0] != "app.example.com" || detail.Domains[1] != "www.example.com" {
		t.Fatalf("expected detail response to expose full domain list, got %+v", detail.Domains)
	}

	listResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/proxy-routes/", nil)
	var routes []service.ProxyRouteView
	decodeResponseData(t, listResp, &routes)
	if len(routes) != 1 || routes[0].SiteName != "marketing-site" || routes[0].LimitConnPerServer != 120 {
		t.Fatalf("unexpected proxy route list response: %+v", routes)
	}
}

func TestProxyRouteBasicAuthPasswordIsRedactedAndPreservedOnBlankUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	token := prepareRootToken(t)
	createResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/", map[string]any{
		"domain":              "auth-redacted.example.com",
		"origin_url":          "https://8.8.8.31:8443",
		"enabled":             true,
		"basic_auth_enabled":  true,
		"basic_auth_username": "edge-admin",
		"basic_auth_password": "edge-secret",
	})
	var createRaw map[string]json.RawMessage
	if err := json.Unmarshal(createResp.Data, &createRaw); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if _, ok := createRaw["basic_auth_password"]; ok {
		t.Fatal("expected create response to omit basic_auth_password")
	}

	var created service.ProxyRouteView
	decodeResponseData(t, createResp, &created)
	if !created.BasicAuthEnabled || !created.BasicAuthPasswordConfigured {
		t.Fatalf("expected basic auth to be configured without revealing password, got %+v", created)
	}

	detailResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/proxy-routes/"+toString(created.ID), nil)
	var detailRaw map[string]json.RawMessage
	if err := json.Unmarshal(detailResp.Data, &detailRaw); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if _, ok := detailRaw["basic_auth_password"]; ok {
		t.Fatal("expected detail response to omit basic_auth_password")
	}

	listResp := performJSONRequest(t, engine, token, http.MethodGet, "/api/proxy-routes/", nil)
	var listRaw []map[string]json.RawMessage
	decodeResponseData(t, listResp, &listRaw)
	if len(listRaw) != 1 {
		t.Fatalf("expected one route, got %d", len(listRaw))
	}
	if _, ok := listRaw[0]["basic_auth_password"]; ok {
		t.Fatal("expected list response to omit basic_auth_password")
	}

	updateResp := performJSONRequest(t, engine, token, http.MethodPost, "/api/proxy-routes/"+toString(created.ID)+"/update", map[string]any{
		"domain":              "auth-redacted.example.com",
		"origin_url":          "https://8.8.8.32:8443",
		"enabled":             true,
		"basic_auth_enabled":  true,
		"basic_auth_username": "edge-admin",
		"basic_auth_password": "",
	})
	var updated service.ProxyRouteView
	decodeResponseData(t, updateResp, &updated)
	if !updated.BasicAuthPasswordConfigured {
		t.Fatalf("expected blank update to preserve configured password, got %+v", updated)
	}

	var stored model.ProxyRoute
	if err := model.DB.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load stored proxy route: %v", err)
	}
	if stored.BasicAuthPassword != "edge-secret" {
		t.Fatalf("expected blank update to preserve stored password, got %q", stored.BasicAuthPassword)
	}
}

func TestPhase2GlobalDiscoveryRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	adminToken := prepareRootToken(t)
	bootstrapResp := performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/nodes/bootstrap-token/rotate", nil)
	var bootstrap service.NodeBootstrapView
	decodeResponseData(t, bootstrapResp, &bootstrap)
	if bootstrap.DiscoveryToken == "" {
		t.Fatal("expected global discovery token to be available")
	}

	resp := performAgentJSONRequestWithTokenAndRemote(t, engine, bootstrap.DiscoveryToken, http.MethodPost, "/api/agent/nodes/register", map[string]any{
		"node_id":         "local-node-id",
		"name":            "bulk-edge-1",
		"ip":              "10.0.0.18",
		"agent_version":   "0.2.0",
		"nginx_version":   "1.25.5",
		"current_version": "",
		"last_error":      "",
	}, "203.0.113.18:4321")
	var registration service.AgentRegistrationResponse
	decodeResponseData(t, resp, &registration)
	if registration.AgentToken == "" || registration.NodeID == "" {
		t.Fatal("expected discovery registration to issue node-specific agent token")
	}

	nodesResp := performJSONRequest(t, engine, adminToken, http.MethodGet, "/api/nodes/", nil)
	var nodes []service.NodeView
	decodeResponseData(t, nodesResp, &nodes)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 discovered node, got %d", len(nodes))
	}
	if nodes[0].Name != "bulk-edge-1" || nodes[0].Status != service.NodeStatusOnline {
		t.Fatal("expected discovered node to be created online")
	}
	if nodes[0].AgentToken != "" || !nodes[0].AgentTokenAvailable || nodes[0].AgentTokenPrefix == "" {
		t.Fatal("expected discovered node list entry to hide full agent token")
	}
	if nodes[0].IP != "203.0.113.18" {
		t.Fatalf("expected discovered node to keep public source ip, got %s", nodes[0].IP)
	}
}

func TestDNSSnapshotRequiresSignedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	worker, err := service.CreateAuthoritativeDNSWorker(service.DNSWorkerInput{Name: "ns-signed"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}

	engine := gin.New()
	router.SetApiRouter(engine)

	unsignedReq := httptest.NewRequest(http.MethodGet, "/api/dns-snapshot", nil)
	unsignedReq.Header.Set("X-DNS-Worker-Token", worker.Token)
	unsignedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(unsignedRecorder, unsignedReq)
	if unsignedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected unsigned snapshot request to fail with 400, got %d: %s", unsignedRecorder.Code, unsignedRecorder.Body.String())
	}

	signedReq := httptest.NewRequest(http.MethodGet, "/api/dns-snapshot", nil)
	signedReq.Header.Set("X-DNS-Worker-Token", worker.Token)
	signedReq.Header.Set(dnsworker.SnapshotSignatureHeader, dnsworker.SnapshotSignatureVersion)
	signedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(signedRecorder, signedReq)
	if signedRecorder.Code != http.StatusOK {
		t.Fatalf("expected signed snapshot request to succeed, got %d: %s", signedRecorder.Code, signedRecorder.Body.String())
	}
	var resp apiResponse
	if err = json.Unmarshal(signedRecorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode signed snapshot response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected signed snapshot success, got %s", resp.Message)
	}
	var envelope dnsworker.SignedSnapshot
	decodeResponseData(t, resp, &envelope)
	if envelope.SignatureVersion != dnsworker.SnapshotSignatureVersion || envelope.Signature == "" {
		t.Fatalf("expected signed snapshot envelope, got %+v", envelope)
	}
}

func TestPhase2LegacyGlobalAgentTokenKeepsExistingNodeOnline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)
	previousAgentLegacyGlobalTokenEnabled := common.AgentLegacyGlobalTokenEnabled
	common.AgentLegacyGlobalTokenEnabled = true
	t.Cleanup(func() {
		common.AgentLegacyGlobalTokenEnabled = previousAgentLegacyGlobalTokenEnabled
	})

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	adminToken := prepareRootToken(t)
	createRouteAndPublishVersion(t, engine, adminToken)

	legacyNode := &model.Node{
		NodeID:          "legacy-node-1",
		Name:            "legacy-edge-1",
		IP:              "10.0.0.18",
		AgentToken:      "",
		AgentVersion:    "0.1.0",
		NginxVersion:    "1.25.5",
		OpenrestyStatus: service.OpenrestyStatusUnknown,
		Status:          service.NodeStatusOffline,
		LastSeenAt:      time.Now().Add(-common.NodeOfflineThreshold - time.Minute),
	}
	if err := legacyNode.Insert(); err != nil {
		t.Fatalf("failed to seed legacy node: %v", err)
	}

	heartbeatPayload := map[string]any{
		"node_id":          legacyNode.NodeID,
		"name":             "legacy-edge-1",
		"ip":               "10.0.0.19",
		"agent_version":    "0.1.1",
		"nginx_version":    "1.25.5",
		"openresty_status": service.OpenrestyStatusHealthy,
		"current_version":  "",
		"last_error":       "",
	}
	rawHeartbeatPayload, err := json.Marshal(heartbeatPayload)
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}
	heartbeatReq := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/heartbeat", bytes.NewReader(rawHeartbeatPayload))
	heartbeatReq.Header.Set("Content-Type", "application/json")
	heartbeatReq.Header.Set("X-Agent-Token", common.AgentToken)
	heartbeatRecorder := httptest.NewRecorder()
	engine.ServeHTTP(heartbeatRecorder, heartbeatReq)
	if heartbeatRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected legacy heartbeat status %d: %s", heartbeatRecorder.Code, heartbeatRecorder.Body.String())
	}
	var heartbeatResp struct {
		Success       bool                      `json:"success"`
		Message       string                    `json:"message"`
		Data          model.Node                `json:"data"`
		AgentSettings service.AgentSettings     `json:"agent_settings"`
		ActiveConfig  *service.ActiveConfigMeta `json:"active_config"`
	}
	if err = json.Unmarshal(heartbeatRecorder.Body.Bytes(), &heartbeatResp); err != nil {
		t.Fatalf("failed to decode legacy heartbeat response: %v", err)
	}
	if !heartbeatResp.Success {
		t.Fatalf("expected legacy heartbeat success, got %s", heartbeatResp.Message)
	}
	if heartbeatResp.Data.NodeID != legacyNode.NodeID || heartbeatResp.Data.AgentVersion != "0.1.1" {
		t.Fatalf("expected legacy heartbeat to update existing node, got %+v", heartbeatResp.Data)
	}
	if heartbeatResp.AgentSettings.WebsocketUpgradeEnabled {
		t.Fatal("expected legacy global token heartbeat to keep websocket upgrade disabled")
	}
	if heartbeatResp.ActiveConfig == nil || heartbeatResp.ActiveConfig.Version == "" {
		t.Fatal("expected legacy heartbeat response to include active config summary")
	}

	activeConfigReq := httptest.NewRequest(http.MethodGet, "/api/agent/config-versions/active", nil)
	activeConfigReq.Header.Set("X-Agent-Token", common.AgentToken)
	activeConfigRecorder := httptest.NewRecorder()
	engine.ServeHTTP(activeConfigRecorder, activeConfigReq)
	if activeConfigRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected legacy global token to be rejected for unbound active config fetch, got %d: %s", activeConfigRecorder.Code, activeConfigRecorder.Body.String())
	}

	applyResp := performAgentJSONRequestWithToken(t, engine, common.AgentToken, http.MethodPost, "/api/agent/apply-logs", map[string]any{
		"node_id": legacyNode.NodeID,
		"version": heartbeatResp.ActiveConfig.Version,
		"result":  service.ApplyResultOK,
		"message": "legacy apply ok",
	})
	var applyLog model.ApplyLog
	decodeResponseData(t, applyResp, &applyLog)
	if applyLog.NodeID != legacyNode.NodeID || applyLog.Result != service.ApplyResultOK {
		t.Fatalf("expected legacy apply log to bind existing node, got %+v", applyLog)
	}

	protectedNode := &model.Node{
		NodeID:           "dedicated-node-1",
		Name:             "dedicated-edge-1",
		IP:               "10.0.0.20",
		AgentTokenHash:   security.HashSecretToken("dedicated-token"),
		AgentTokenPrefix: security.SecretTokenPrefix("dedicated-token"),
		AgentVersion:     "0.2.0",
		NginxVersion:     "1.25.5",
		OpenrestyStatus:  service.OpenrestyStatusUnknown,
		Status:           service.NodeStatusOnline,
		LastSeenAt:       time.Now(),
	}
	if err = protectedNode.Insert(); err != nil {
		t.Fatalf("failed to seed dedicated node: %v", err)
	}
	deniedPayload, err := json.Marshal(map[string]any{
		"node_id":       protectedNode.NodeID,
		"ip":            "10.0.0.21",
		"agent_version": "0.2.1",
	})
	if err != nil {
		t.Fatalf("failed to marshal denied payload: %v", err)
	}
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/heartbeat", bytes.NewReader(deniedPayload))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedReq.Header.Set("X-Agent-Token", common.AgentToken)
	deniedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(deniedRecorder, deniedReq)
	if deniedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected legacy global token to be rejected for dedicated node, got %d", deniedRecorder.Code)
	}
}

func TestPhase2LegacyGlobalAgentTokenDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	setupTestDB(t)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.SetApiRouter(engine)

	legacyNode := &model.Node{
		NodeID:       "legacy-disabled-node",
		Name:         "legacy-disabled-node",
		IP:           "10.0.0.22",
		AgentVersion: "0.1.0",
		Status:       service.NodeStatusOffline,
		LastSeenAt:   time.Now().Add(-common.NodeOfflineThreshold - time.Minute),
	}
	if err := legacyNode.Insert(); err != nil {
		t.Fatalf("failed to seed legacy node: %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"node_id":       legacyNode.NodeID,
		"ip":            "10.0.0.23",
		"agent_version": "0.1.1",
	})
	if err != nil {
		t.Fatalf("failed to marshal disabled legacy payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/nodes/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", common.AgentToken)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected legacy global token to be rejected by default, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode disabled legacy response: %v", err)
	}
	if resp.Success || !strings.Contains(resp.Message, "compatibility is disabled") {
		t.Fatalf("expected legacy global token to be disabled by default, got %+v", resp)
	}
}

func performAgentJSONRequestWithToken(t *testing.T, engine http.Handler, token string, method string, path string, body any) apiResponse {
	return performAgentJSONRequestWithTokenAndRemote(t, engine, token, method, path, body, "")
}

func performAgentJSONRequestWithTokenAndRemote(t *testing.T, engine http.Handler, token string, method string, path string, body any, remoteAddr string) apiResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	req.Header.Set("X-Agent-Token", token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d for %s %s: %s", recorder.Code, method, path, recorder.Body.String())
	}
	var resp apiResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("request %s %s failed: %s", method, path, resp.Message)
	}
	return resp
}

func createRouteAndPublishVersion(t *testing.T, engine http.Handler, adminToken string) {
	t.Helper()
	createBody := map[string]any{
		"domain":     "agent.example.com",
		"origin_url": "https://agent-origin.internal",
		"enabled":    true,
		"remark":     "agent route",
	}
	performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/proxy-routes/", createBody)
	performJSONRequest(t, engine, adminToken, http.MethodPost, "/api/config-versions/publish", nil)
}
