package ratelimit

import (
	"testing"
	"time"
)

func TestInMemoryRateLimiterCleansExpiredItemsOnRequest(t *testing.T) {
	limiter := &InMemoryRateLimiter{}
	limiter.Init(time.Second)
	oldQueue := []int64{time.Now().Add(-2 * time.Second).Unix()}
	limiter.store["old"] = &oldQueue

	if !limiter.Request("current", 1, 60) {
		t.Fatal("expected first current request to be allowed")
	}
	if _, ok := limiter.store["old"]; ok {
		t.Fatalf("expected old key to be cleaned, got %+v", limiter.store)
	}
	if limiter.Request("current", 1, 60) {
		t.Fatal("expected second current request to be limited")
	}
}
