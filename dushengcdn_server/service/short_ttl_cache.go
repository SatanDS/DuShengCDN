package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type shortTTLResultCache[T any] struct {
	sync.Mutex
	values     map[string]shortTTLResultCacheEntry[T]
	group      singleflight.Group
	ttl        time.Duration
	maxEntries int
}

type shortTTLResultCacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func (cache *shortTTLResultCache[T]) load(key string, load func() (T, error)) (T, error) {
	if value, ok := cache.get(key, time.Now()); ok {
		return value, nil
	}
	value, err, _ := cache.group.Do(key, func() (any, error) {
		if value, ok := cache.get(key, time.Now()); ok {
			return value, nil
		}
		loaded, err := load()
		if err != nil {
			var zero T
			return zero, err
		}
		cache.set(key, loaded, time.Now())
		return loaded, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	result, ok := value.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("invalid cached result for %s", key)
	}
	return result, nil
}

func (cache *shortTTLResultCache[T]) get(key string, now time.Time) (T, bool) {
	cache.Lock()
	defer cache.Unlock()
	entry, ok := cache.values[key]
	if !ok {
		var zero T
		return zero, false
	}
	if !entry.expiresAt.After(now) {
		delete(cache.values, key)
		var zero T
		return zero, false
	}
	return entry.value, true
}

func (cache *shortTTLResultCache[T]) set(key string, value T, now time.Time) {
	cache.Lock()
	defer cache.Unlock()
	if cache.values == nil {
		cache.values = make(map[string]shortTTLResultCacheEntry[T])
	}
	cache.pruneLocked(now)
	cache.values[key] = shortTTLResultCacheEntry[T]{
		value:     value,
		expiresAt: now.Add(cache.ttlOrDefault()),
	}
}

func (cache *shortTTLResultCache[T]) reset() {
	cache.Lock()
	defer cache.Unlock()
	cache.values = make(map[string]shortTTLResultCacheEntry[T])
}

func (cache *shortTTLResultCache[T]) pruneLocked(now time.Time) {
	maxEntries := cache.maxEntriesOrDefault()
	if len(cache.values) < maxEntries {
		return
	}
	for key, entry := range cache.values {
		if !entry.expiresAt.After(now) {
			delete(cache.values, key)
		}
	}
	for len(cache.values) >= maxEntries {
		for key := range cache.values {
			delete(cache.values, key)
			break
		}
	}
}

func (cache *shortTTLResultCache[T]) ttlOrDefault() time.Duration {
	if cache.ttl > 0 {
		return cache.ttl
	}
	return 10 * time.Second
}

func (cache *shortTTLResultCache[T]) maxEntriesOrDefault() int {
	if cache.maxEntries > 0 {
		return cache.maxEntries
	}
	return 16
}

func serviceRuntimeCacheKey(name string) string {
	return fmt.Sprintf("%s\x00%p\x00%s\x00%s", name, model.DB, common.SQLDSN, common.SQLitePath)
}
