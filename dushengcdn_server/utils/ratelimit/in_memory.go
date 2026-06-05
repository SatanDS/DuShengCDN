package ratelimit

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
	lastCleanupUnix    int64
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems(now int64) {
	if l.expirationDuration <= 0 {
		return
	}
	expirationSeconds := int64(l.expirationDuration.Seconds())
	if expirationSeconds <= 0 || now-l.lastCleanupUnix < expirationSeconds {
		return
	}
	l.lastCleanupUnix = now
	for key, queue := range l.store {
		if queue == nil {
			delete(l.store, key)
			continue
		}
		size := len(*queue)
		if size == 0 || now-(*queue)[size-1] > expirationSeconds {
			delete(l.store, key)
		}
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	l.clearExpiredItems(now)
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		}
		if now-(*queue)[0] >= duration {
			*queue = (*queue)[1:]
			*queue = append(*queue, now)
			return true
		}
		return false
	}
	s := make([]int64, 0, maxRequestNum)
	l.store[key] = &s
	*(l.store[key]) = append(*(l.store[key]), now)
	return true
}
