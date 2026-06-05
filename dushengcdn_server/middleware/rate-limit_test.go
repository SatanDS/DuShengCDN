package middleware

import (
	"dushengcdn/common"
	"dushengcdn/utils/ratelimit"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRedisRateLimiterFallsBackToMemoryWhenClientUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = true
	common.RDB = nil
	inMemoryRateLimiter = inMemoryRateLimiterZeroValue()
	t.Cleanup(func() {
		common.RedisEnabled = true
		common.RDB = nil
		inMemoryRateLimiter = inMemoryRateLimiterZeroValue()
	})

	handler := rateLimitFactory(1, 60, "test")
	router := gin.New()
	router.GET("/limited", handler, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	router.ServeHTTP(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("expected first request to pass through memory fallback, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "192.0.2.1:12346"
	router.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be limited by memory fallback, got %d", second.Code)
	}
}

func TestRateLimiterRejectsInvalidConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	inMemoryRateLimiter = inMemoryRateLimiterZeroValue()
	t.Cleanup(func() {
		inMemoryRateLimiter = inMemoryRateLimiterZeroValue()
	})

	handler := rateLimitFactory(0, 60, "test")
	router := gin.New()
	router.GET("/limited", handler, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected invalid rate limit config to reject request, got %d", recorder.Code)
	}
}

func inMemoryRateLimiterZeroValue() ratelimit.InMemoryRateLimiter {
	return ratelimit.InMemoryRateLimiter{}
}
