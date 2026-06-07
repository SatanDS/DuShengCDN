package controller

import (
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
