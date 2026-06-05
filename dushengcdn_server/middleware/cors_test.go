package middleware

import (
	"dushengcdn/common"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSRejectsCrossOriginWhenServerAddressEmpty(t *testing.T) {
	recorder := requestWithOrigin(t, "", "https://evil.example")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for unconfigured CORS origin, got %d", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected allow-origin header: %s", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSAllowsConfiguredServerOrigin(t *testing.T) {
	recorder := requestWithOrigin(t, "https://panel.example.com/", "https://panel.example.com")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected configured origin to pass, got %d", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://panel.example.com" {
		t.Fatalf("unexpected allow-origin header: %s", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSNormalizesConfiguredServerURLPath(t *testing.T) {
	recorder := requestWithOrigin(t, "https://panel.example.com/admin", "https://panel.example.com")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected configured origin with path to pass, got %d", recorder.Code)
	}
}

func TestCORSNormalizesHostCaseAndDefaultPort(t *testing.T) {
	recorder := requestWithOrigin(t, "https://PANEL.example.com:443/admin", "https://panel.example.com")
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected configured origin with default port to pass, got %d", recorder.Code)
	}
}

func TestCORSRejectsMalformedOrigin(t *testing.T) {
	recorder := requestWithOrigin(t, "https://panel.example.com", "https://panel.example.com/path")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected malformed Origin with path to be rejected, got %d", recorder.Code)
	}
}

func requestWithOrigin(t *testing.T, serverAddress string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	oldServerAddress := common.ServerAddress
	oldMode := gin.Mode()
	common.ServerAddress = serverAddress
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		common.ServerAddress = oldServerAddress
		gin.SetMode(oldMode)
	})

	router := gin.New()
	router.Use(CORS())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	request.Host = "api.example.com"
	request.Header.Set("Origin", origin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
