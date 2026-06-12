package service

import (
	"dushengcdn/utils/geoip"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"
)

var accessLogGeoProviderFactory = func() (geoip.GeoIPService, error) {
	return geoip.NewMaxMindGeoIPService()
}

const (
	// accessLogRegionProviderRetryInterval throttles background provider
	// initialization attempts after a failure (e.g. database download error).
	accessLogRegionProviderRetryInterval = 5 * time.Minute
	// accessLogRegionCacheTTL bounds how long a resolved IP -> region entry
	// may be served, so GeoIP database updates eventually become visible.
	accessLogRegionCacheTTL = 48 * time.Hour
)

type accessLogGeoResult struct {
	region   string
	operator string
}

// accessLogRegionState holds the process-level GeoIP provider used on the
// log-ingest hot path. The provider is initialized lazily in a background
// goroutine because building the MaxMind provider may synchronously download
// the database (up to minutes of network I/O), which must never block a node
// heartbeat. While the provider is not ready, logs are stored with an empty
// region.
var accessLogRegionState = struct {
	sync.Mutex
	provider     geoip.GeoIPService
	initializing bool
	lastAttempt  time.Time
	generation   uint64
}{}

// accessLogRegionCache is a bounded IP -> region cache shared across ingest
// batches (cost 1 per entry, at most ~20k entries).
var (
	accessLogRegionCache     *ristretto.Cache[string, accessLogGeoResult]
	accessLogRegionCacheOnce sync.Once
)

func accessLogRegionIPCache() *ristretto.Cache[string, accessLogGeoResult] {
	accessLogRegionCacheOnce.Do(func() {
		cache, err := ristretto.NewCache(&ristretto.Config[string, accessLogGeoResult]{
			NumCounters: 1e5,
			MaxCost:     2e4,
			BufferItems: 64,
		})
		if err != nil {
			slog.Warn("access log region cache disabled", "error", err)
			return
		}
		accessLogRegionCache = cache
	})
	return accessLogRegionCache
}

// currentAccessLogRegionProvider returns the shared GeoIP provider if it is
// ready. When it is not, it kicks a background initialization (at most once
// per retry interval) and returns nil immediately without blocking.
func currentAccessLogRegionProvider() geoip.GeoIPService {
	accessLogRegionState.Lock()
	defer accessLogRegionState.Unlock()
	if accessLogRegionState.provider != nil {
		return accessLogRegionState.provider
	}
	if accessLogRegionState.initializing {
		return nil
	}
	if !accessLogRegionState.lastAttempt.IsZero() && time.Since(accessLogRegionState.lastAttempt) < accessLogRegionProviderRetryInterval {
		return nil
	}
	accessLogRegionState.initializing = true
	accessLogRegionState.lastAttempt = time.Now()
	factory := accessLogGeoProviderFactory
	generation := accessLogRegionState.generation
	go initializeAccessLogRegionProvider(factory, generation)
	return nil
}

func initializeAccessLogRegionProvider(factory func() (geoip.GeoIPService, error), generation uint64) {
	provider, err := factory()
	accessLogRegionState.Lock()
	if accessLogRegionState.generation != generation {
		// The provider state was reset while this initialization was running
		// (e.g. by a test swapping the factory); discard the stale provider.
		accessLogRegionState.Unlock()
		if provider != nil {
			_ = provider.Close()
		}
		return
	}
	accessLogRegionState.initializing = false
	if err != nil {
		accessLogRegionState.Unlock()
		slog.Warn("initialize access log geo provider failed", "error", err)
		return
	}
	accessLogRegionState.provider = provider
	accessLogRegionState.Unlock()
}

// resolveAccessLogGeoInfo resolves an access-log client IP using the bounded
// shared cache first, falling back to the supplied provider. A nil provider
// (not initialized yet) resolves to an empty result; the IP is not cached in
// that case so it can be retried once the provider becomes available.
func resolveAccessLogGeoInfo(provider geoip.GeoIPService, rawIP string) accessLogGeoResult {
	normalizedIP := normalizeAccessLogIP(rawIP)
	if normalizedIP == "" {
		return accessLogGeoResult{}
	}
	cache := accessLogRegionIPCache()
	if cache != nil {
		if cached, ok := cache.Get(normalizedIP); ok {
			return cached
		}
	}
	if provider == nil {
		return accessLogGeoResult{}
	}

	result := accessLogGeoResult{}
	if info, err := provider.GetGeoInfo(net.ParseIP(normalizedIP)); err == nil && info != nil {
		region := strings.TrimSpace(info.Name)
		if region == "" {
			region = strings.TrimSpace(info.ISOCode)
		}
		result = accessLogGeoResult{
			region:   region,
			operator: strings.TrimSpace(info.Operator),
		}
	}
	if cache != nil {
		cache.SetWithTTL(normalizedIP, result, 1, accessLogRegionCacheTTL)
	}
	return result
}

// resetAccessLogRegionProviderForTest clears the shared provider, pending
// initialization state, and the IP cache.
func resetAccessLogRegionProviderForTest() {
	accessLogRegionState.Lock()
	accessLogRegionState.generation++
	provider := accessLogRegionState.provider
	accessLogRegionState.provider = nil
	accessLogRegionState.initializing = false
	accessLogRegionState.lastAttempt = time.Time{}
	accessLogRegionState.Unlock()
	if provider != nil {
		if err := provider.Close(); err != nil {
			slog.Warn("close access log geo provider failed", "error", err)
		}
	}
	if cache := accessLogRegionIPCache(); cache != nil {
		cache.Clear()
	}
}

// setAccessLogGeoProviderFactoryForTest swaps the provider factory, resets the
// shared state, and builds the provider synchronously so tests resolve
// regions deterministically. It returns a restore function.
func setAccessLogGeoProviderFactoryForTest(factory func() (geoip.GeoIPService, error)) func() {
	accessLogRegionState.Lock()
	previous := accessLogGeoProviderFactory
	accessLogGeoProviderFactory = factory
	accessLogRegionState.Unlock()
	resetAccessLogRegionProviderForTest()
	if factory != nil {
		if provider, err := factory(); err == nil {
			accessLogRegionState.Lock()
			accessLogRegionState.provider = provider
			accessLogRegionState.Unlock()
		}
	}
	return func() {
		accessLogRegionState.Lock()
		accessLogGeoProviderFactory = previous
		accessLogRegionState.Unlock()
		resetAccessLogRegionProviderForTest()
	}
}

func normalizeAccessLogIP(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}

	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}
