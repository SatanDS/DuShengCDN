package dnsworker

import (
	"net"
	"strings"
	"sync"
	"time"
)

type RollupAggregator struct {
	mu      sync.Mutex
	window  time.Duration
	buckets map[rollupKey]*rollupBucket
}

type rollupKey struct {
	WindowStart  time.Time
	ZoneID       uint
	ProxyRouteID uint
	SourceScope  string
	QName        string
	QType        string
	RCode        string
}

type rollupBucket struct {
	count           int64
	totalDurationMs int64
	maxDurationMs   int64
	targets         map[string]int64
}

func NewRollupAggregator(window time.Duration) *RollupAggregator {
	if window <= 0 {
		window = time.Minute
	}
	return &RollupAggregator{
		window:  window,
		buckets: map[rollupKey]*rollupBucket{},
	}
}

func (a *RollupAggregator) Record(zoneID uint, routeID uint, sourceScope string, qname string, qtype string, rcode string, targets []string, duration time.Duration) {
	if a == nil {
		return
	}
	now := time.Now().UTC()
	key := rollupKey{
		WindowStart:  now.Truncate(a.window),
		ZoneID:       zoneID,
		ProxyRouteID: routeID,
		SourceScope:  normalizeSourceScope(sourceScope),
		QName:        normalizeDomain(qname),
		QType:        strings.ToUpper(strings.TrimSpace(qtype)),
		RCode:        normalizeRCode(rcode),
	}
	if key.QType == "" {
		key.QType = "A"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	bucket := a.buckets[key]
	if bucket == nil {
		bucket = &rollupBucket{targets: map[string]int64{}}
		a.buckets[key] = bucket
	}
	bucket.count++
	durationMs := duration.Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	bucket.totalDurationMs += durationMs
	if durationMs > bucket.maxDurationMs {
		bucket.maxDurationMs = durationMs
	}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		bucket.targets[target]++
	}
}

func (a *RollupAggregator) Drain() []QueryRollupPayload {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	payloads := make([]QueryRollupPayload, 0, len(a.buckets))
	for key, bucket := range a.buckets {
		if bucket == nil || bucket.count <= 0 {
			continue
		}
		targets := make(map[string]int64, len(bucket.targets))
		for target, count := range bucket.targets {
			targets[target] = count
		}
		payloads = append(payloads, QueryRollupPayload{
			WindowStart:     key.WindowStart,
			WindowMinutes:   int(a.window / time.Minute),
			ZoneID:          key.ZoneID,
			ProxyRouteID:    key.ProxyRouteID,
			SourceScope:     key.SourceScope,
			QName:           key.QName,
			QType:           key.QType,
			RCode:           key.RCode,
			QueryCount:      bucket.count,
			TotalDurationMs: bucket.totalDurationMs,
			MaxDurationMs:   bucket.maxDurationMs,
			TargetSummary:   targets,
		})
	}
	a.buckets = map[rollupKey]*rollupBucket{}
	return payloads
}

func (a *RollupAggregator) Restore(payloads []QueryRollupPayload) {
	if a == nil || len(payloads) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, payload := range payloads {
		if payload.QueryCount <= 0 {
			continue
		}
		key := rollupKey{
			WindowStart:  payload.WindowStart,
			ZoneID:       payload.ZoneID,
			ProxyRouteID: payload.ProxyRouteID,
			SourceScope:  normalizeSourceScope(payload.SourceScope),
			QName:        normalizeDomain(payload.QName),
			QType:        strings.ToUpper(strings.TrimSpace(payload.QType)),
			RCode:        normalizeRCode(payload.RCode),
		}
		if key.WindowStart.IsZero() {
			key.WindowStart = time.Now().UTC().Truncate(a.window)
		}
		if key.QType == "" {
			key.QType = "A"
		}
		bucket := a.buckets[key]
		if bucket == nil {
			bucket = &rollupBucket{targets: map[string]int64{}}
			a.buckets[key] = bucket
		}
		bucket.count += payload.QueryCount
		if payload.TotalDurationMs > 0 {
			bucket.totalDurationMs += payload.TotalDurationMs
		}
		if payload.MaxDurationMs > bucket.maxDurationMs {
			bucket.maxDurationMs = payload.MaxDurationMs
		}
		for target, count := range payload.TargetSummary {
			if strings.TrimSpace(target) == "" || count <= 0 {
				continue
			}
			bucket.targets[target] += count
		}
	}
}

func normalizeSourceScope(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "global"
	}
	prefix, country, ok := strings.Cut(value, ":")
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "country") {
		country = strings.ToUpper(strings.TrimSpace(country))
		if len(country) == 2 {
			return "country:" + country
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "cidr") {
		if cidr, valid := normalizeCIDR(country); valid {
			return "cidr:" + cidr
		}
		if _, network, err := net.ParseCIDR(strings.TrimSpace(country)); err == nil {
			return "cidr:" + network.String()
		}
	}
	return value
}

func normalizeRCode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return "NOERROR"
	}
}
