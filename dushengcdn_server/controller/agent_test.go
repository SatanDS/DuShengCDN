package controller

import (
	"dushengcdn/common"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResolveAgentNodeIPIgnoresForwardedHeadersWhenProxyIsUntrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies failed: %v", err)
	}
	resolved := ""
	engine.POST("/test", func(c *gin.Context) {
		resolved = resolveAgentNodeIP(c, "10.0.0.8")
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	req.Header.Set("X-Real-IP", "8.8.4.4")
	req.Header.Set("Forwarded", "for=1.1.1.1")
	engine.ServeHTTP(recorder, req)
	if resolved != "198.51.100.20" {
		t.Fatalf("expected untrusted forwarded headers to be ignored, got %q", resolved)
	}
}

func TestWebSocketOriginAllowsNonBrowserClients(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "panel.example.com"
	if !isAllowedWebSocketOrigin(request) {
		t.Fatal("expected websocket requests without Origin to be allowed")
	}
}

func TestWebSocketOriginAllowsSameHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "panel.example.com:3000"
	request.Header.Set("Origin", "https://panel.example.com:3000")
	if !isAllowedWebSocketOrigin(request) {
		t.Fatal("expected same-host websocket origin to be allowed")
	}
}

func TestWebSocketOriginRejectsHTTPOriginForHTTPSPanel(t *testing.T) {
	oldServerAddress := common.ServerAddress
	common.ServerAddress = "https://panel.example.com"
	defer func() {
		common.ServerAddress = oldServerAddress
	}()

	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "panel.example.com"
	request.Header.Set("Origin", "http://panel.example.com")
	if isAllowedWebSocketOrigin(request) {
		t.Fatal("expected http websocket origin to be rejected for https panel")
	}
}

func TestWebSocketOriginAllowsConfiguredServerAddress(t *testing.T) {
	oldServerAddress := common.ServerAddress
	common.ServerAddress = "https://panel.example.com/admin"
	defer func() {
		common.ServerAddress = oldServerAddress
	}()

	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "127.0.0.1:3000"
	request.Header.Set("Origin", "https://panel.example.com")
	if !isAllowedWebSocketOrigin(request) {
		t.Fatal("expected configured websocket origin to be allowed")
	}
}

func TestWebSocketOriginRejectsCrossOrigin(t *testing.T) {
	oldServerAddress := common.ServerAddress
	common.ServerAddress = "https://panel.example.com"
	defer func() {
		common.ServerAddress = oldServerAddress
	}()

	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "panel.example.com"
	request.Header.Set("Origin", "https://evil.example")
	if isAllowedWebSocketOrigin(request) {
		t.Fatal("expected cross-origin websocket request to be rejected")
	}
}

func TestWebSocketOriginRejectsMalformedOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/update/logs/ws", nil)
	request.Host = "panel.example.com"
	request.Header.Set("Origin", "https://panel.example.com/path")
	if isAllowedWebSocketOrigin(request) {
		t.Fatal("expected malformed websocket origin to be rejected")
	}
}
