package common

import (
	"testing"

	"github.com/go-redis/redis/v8"
)

func TestInitRedisClientDisablesRedisWhenConnectionStringMissing(t *testing.T) {
	t.Setenv("REDIS_CONN_STRING", "")
	RedisRequired = false
	RedisEnabled = true
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		RDB = nil
		RedisEnabled = true
		RedisRequired = false
	})

	if err := InitRedisClient(); err != nil {
		t.Fatalf("expected missing optional redis connection string to be ignored: %v", err)
	}
	if RedisEnabled {
		t.Fatal("expected redis to be disabled")
	}
	if RDB != nil {
		t.Fatal("expected redis client to be cleared")
	}
}

func TestInitRedisClientFailsWhenRedisRequiredAndConnectionStringMissing(t *testing.T) {
	t.Setenv("REDIS_CONN_STRING", "")
	RedisRequired = true
	t.Cleanup(func() {
		RedisRequired = false
		RedisEnabled = true
	})

	if err := InitRedisClient(); err == nil {
		t.Fatal("expected required redis initialization to fail without connection string")
	}
	if RedisEnabled {
		t.Fatal("expected redis to remain disabled after required initialization failure")
	}
}

func TestInitRedisClientDisablesRedisWhenConnectionStringInvalid(t *testing.T) {
	t.Setenv("REDIS_CONN_STRING", "not-a-redis-url")
	RedisRequired = false
	RedisEnabled = true
	t.Cleanup(func() {
		RedisRequired = false
		RedisEnabled = true
	})

	if err := InitRedisClient(); err != nil {
		t.Fatalf("expected invalid optional redis connection string to degrade gracefully: %v", err)
	}
	if RedisEnabled {
		t.Fatal("expected redis to be disabled")
	}
	if RDB != nil {
		t.Fatal("expected redis client to be nil")
	}
}

func TestParseRedisOptionReturnsErrorInsteadOfPanicking(t *testing.T) {
	t.Setenv("REDIS_CONN_STRING", "not-a-redis-url")

	if _, err := ParseRedisOption(); err == nil {
		t.Fatal("expected invalid redis connection string to return an error")
	}
}
