package middleware

import (
	"dushengcdn/common"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestTurnstileCheckUsesFormAndStoresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureTurnstileTest(t)

	verifyCalled := false
	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyCalled = true
		if r.Method != http.MethodPost {
			t.Errorf("expected post request, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("secret") != "turnstile-secret" || r.Form.Get("response") != "turnstile-token" || r.Form.Get("remoteip") != "198.51.100.7" {
			t.Errorf("unexpected turnstile form: %#v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(verifyServer.Close)
	turnstileVerifyEndpoint = verifyServer.URL

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.GET("/guarded", TurnstileCheck(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded?turnstile=turnstile-token", nil)
	req.RemoteAddr = "198.51.100.7:12345"
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected guarded endpoint success, got %s", recorder.Body.String())
	}
	if !verifyCalled {
		t.Fatal("expected turnstile verify endpoint to be called")
	}

}

func TestTurnstileCheckRejectsUnexpectedStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureTurnstileTest(t)

	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(verifyServer.Close)
	turnstileVerifyEndpoint = verifyServer.URL

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("test-secret"))))
	router.GET("/guarded", TurnstileCheck(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded?turnstile=turnstile-token", nil)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Turnstile 校验服务异常") {
		t.Fatalf("expected turnstile status failure message, got %s", recorder.Body.String())
	}

}

func configureTurnstileTest(t *testing.T) {
	t.Helper()
	oldEnabled := common.TurnstileCheckEnabled
	oldSecret := common.TurnstileSecretKey
	oldEndpoint := turnstileVerifyEndpoint
	common.TurnstileCheckEnabled = true
	common.TurnstileSecretKey = "turnstile-secret"
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = oldEnabled
		common.TurnstileSecretKey = oldSecret
		turnstileVerifyEndpoint = oldEndpoint
	})
}
