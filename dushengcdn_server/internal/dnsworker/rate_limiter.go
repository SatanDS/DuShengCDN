package dnsworker

import (
	"net"
	"strings"
	"sync"
	"time"
)

type QueryRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	buckets map[string]rateLimitBucket
}

type ResponseRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	buckets map[string]rateLimitBucket
}

type rateLimitBucket struct {
	WindowStart time.Time
	Count       int
	LastSeenAt  time.Time
}

func NewQueryRateLimiter(limit int) *QueryRateLimiter {
	if limit <= 0 {
		return nil
	}
	return &QueryRateLimiter{
		limit:   limit,
		window:  time.Second,
		now:     time.Now,
		buckets: map[string]rateLimitBucket{},
	}
}

func NewResponseRateLimiter(limit int) *ResponseRateLimiter {
	if limit <= 0 {
		return nil
	}
	return &ResponseRateLimiter{
		limit:   limit,
		window:  time.Second,
		now:     time.Now,
		buckets: map[string]rateLimitBucket{},
	}
}

func (l *QueryRateLimiter) Allow(remoteAddr net.Addr) bool {
	if l == nil {
		return true
	}
	key := remoteRateLimitKey(remoteAddr)
	if key == "" {
		key = "unknown"
	}
	now := l.now().UTC()
	windowStart := now.Truncate(l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) > 4096 {
		l.prune(now)
	}
	bucket := l.buckets[key]
	if bucket.WindowStart.IsZero() || !bucket.WindowStart.Equal(windowStart) {
		bucket = rateLimitBucket{WindowStart: windowStart}
	}
	bucket.Count++
	bucket.LastSeenAt = now
	l.buckets[key] = bucket
	return bucket.Count <= l.limit
}

func (l *ResponseRateLimiter) Allow(remoteAddr net.Addr, qname string, rcode string) bool {
	if l == nil {
		return true
	}
	rcode = strings.ToUpper(strings.TrimSpace(rcode))
	if rcode == "" || rcode == "NOERROR" || rcode == "NODATA" {
		return true
	}
	source := remoteRateLimitKey(remoteAddr)
	if source == "" {
		source = "unknown"
	}
	key := source + "|" + strings.ToLower(strings.TrimSpace(qname)) + "|" + rcode
	now := l.now().UTC()
	windowStart := now.Truncate(l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) > 8192 {
		l.prune(now)
	}
	bucket := l.buckets[key]
	if bucket.WindowStart.IsZero() || !bucket.WindowStart.Equal(windowStart) {
		bucket = rateLimitBucket{WindowStart: windowStart}
	}
	bucket.Count++
	bucket.LastSeenAt = now
	l.buckets[key] = bucket
	return bucket.Count <= l.limit
}

func (l *QueryRateLimiter) prune(now time.Time) {
	cutoff := now.Add(-2 * l.window)
	for key, bucket := range l.buckets {
		if bucket.LastSeenAt.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

func (l *ResponseRateLimiter) prune(now time.Time) {
	cutoff := now.Add(-2 * l.window)
	for key, bucket := range l.buckets {
		if bucket.LastSeenAt.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

func remoteRateLimitKey(remoteAddr net.Addr) string {
	if remoteAddr == nil {
		return ""
	}
	switch addr := remoteAddr.(type) {
	case *net.UDPAddr:
		if addr.IP != nil {
			return addr.IP.String()
		}
	case *net.TCPAddr:
		if addr.IP != nil {
			return addr.IP.String()
		}
	}
	host, _, err := net.SplitHostPort(remoteAddr.String())
	if err == nil && strings.TrimSpace(host) != "" {
		return strings.Trim(host, "[]")
	}
	return strings.TrimSpace(remoteAddr.String())
}
