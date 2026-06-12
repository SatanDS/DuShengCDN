package middleware

import (
	"context"
	"dushengcdn/common"
	"dushengcdn/utils/ratelimit"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const redisRateLimitTimeout = time.Second

var inMemoryRateLimiter ratelimit.InMemoryRateLimiter
var redisRateLimitSequence uint64
var redisRateLimitInstanceID = uuid.NewString()

func ResetRateLimiterForTest() {
	inMemoryRateLimiter = ratelimit.InMemoryRateLimiter{}
	atomic.StoreUint64(&redisRateLimitSequence, 0)
}

var redisRateLimitScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local ttl = tonumber(ARGV[5])

redis.call("ZREMRANGEBYSCORE", key, "-inf", now - window)
local count = redis.call("ZCARD", key)
if count >= limit then
	redis.call("PEXPIRE", key, ttl)
	return 0
end

redis.call("ZADD", key, now, member)
redis.call("PEXPIRE", key, ttl)
return 1
`)

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	if !validRateLimitConfig(c, maxRequestNum, duration) {
		return
	}
	rdb := common.RDB
	if rdb == nil {
		slog.Warn("redis rate limiter unavailable; falling back to memory limiter")
		memoryRateLimiter(c, maxRequestNum, duration, mark)
		return
	}

	key := "rateLimit:" + mark + c.ClientIP()
	nowMillis := time.Now().UnixMilli()
	windowMillis := duration * int64(time.Second/time.Millisecond)
	ttlMillis := int64(common.RateLimitKeyExpirationDuration / time.Millisecond)
	if ttlMillis < windowMillis {
		ttlMillis = windowMillis
	}
	member := fmt.Sprintf("%d:%s:%d", nowMillis, redisRateLimitInstanceID, atomic.AddUint64(&redisRateLimitSequence, 1))

	ctx, cancel := context.WithTimeout(c.Request.Context(), redisRateLimitTimeout)
	defer cancel()

	result, err := redisRateLimitScript.Run(
		ctx,
		rdb,
		[]string{key},
		nowMillis,
		windowMillis,
		maxRequestNum,
		member,
		ttlMillis,
	).Result()
	if err != nil {
		slog.Warn("redis rate limiter script failed; falling back to memory limiter", "error", err)
		memoryRateLimiter(c, maxRequestNum, duration, mark)
		return
	}
	allowed, ok := result.(int64)
	if !ok {
		slog.Warn("redis rate limiter returned unexpected result; falling back to memory limiter", "result", result)
		memoryRateLimiter(c, maxRequestNum, duration, mark)
		return
	}
	if allowed != 1 {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	if !validRateLimitConfig(c, maxRequestNum, duration) {
		return
	}
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func validRateLimitConfig(c *gin.Context, maxRequestNum int, duration int64) bool {
	if maxRequestNum > 0 && duration > 0 {
		return true
	}
	slog.Error("invalid rate limiter config", "max_request_num", maxRequestNum, "duration", duration)
	c.Status(http.StatusTooManyRequests)
	c.Abort()
	return false
}

type rateLimitConfigProvider func() (int, int64)

func rateLimitFactory(config rateLimitConfigProvider, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			maxRequestNum, duration := config()
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			maxRequestNum, duration := config()
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration
	}, "GW")
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	}, "GA")
}

func CriticalRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	}, "CT")
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.DownloadRateLimitNum, common.DownloadRateLimitDuration
	}, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.UploadRateLimitNum, common.UploadRateLimitDuration
	}, "UP")
}

func DNSWorkerAPIRateLimit() func(c *gin.Context) {
	return rateLimitFactory(func() (int, int64) {
		return common.DNSWorkerAPIRateLimitNum, common.DNSWorkerAPIRateLimitDuration
	}, "DWK")
}
