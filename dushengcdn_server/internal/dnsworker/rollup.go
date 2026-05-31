package dnsworker

import (
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
	QName        string
	QType        string
	RCode        string
}

type rollupBucket struct {
	count   int64
	targets map[string]int64
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

func (a *RollupAggregator) Record(zoneID uint, routeID uint, qname string, qtype string, rcode string, targets []string) {
	if a == nil {
		return
	}
	now := time.Now().UTC()
	key := rollupKey{
		WindowStart:  now.Truncate(a.window),
		ZoneID:       zoneID,
		ProxyRouteID: routeID,
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
			WindowStart:   key.WindowStart,
			WindowMinutes: int(a.window / time.Minute),
			ZoneID:        key.ZoneID,
			ProxyRouteID:  key.ProxyRouteID,
			QName:         key.QName,
			QType:         key.QType,
			RCode:         key.RCode,
			QueryCount:    bucket.count,
			TargetSummary: targets,
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
		for target, count := range payload.TargetSummary {
			if strings.TrimSpace(target) == "" || count <= 0 {
				continue
			}
			bucket.targets[target] += count
		}
	}
}

func normalizeRCode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return "NOERROR"
	}
}
