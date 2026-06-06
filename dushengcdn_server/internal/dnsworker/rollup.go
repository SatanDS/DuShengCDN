package dnsworker

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRollupMaxBuckets       = 4096
	DefaultRollupMaxTargetSummary = 64
	rollupOverflowQName           = "_overflow"
)

type RollupAggregator struct {
	mu         sync.Mutex
	window     time.Duration
	maxBuckets int
	buckets    map[rollupKey]*rollupBucket
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
		window:     window,
		maxBuckets: DefaultRollupMaxBuckets,
		buckets:    map[rollupKey]*rollupBucket{},
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
	if bucket == nil && a.maxBuckets > 0 && len(a.buckets) >= a.maxBuckets {
		key = overflowRollupKey(key)
		bucket = a.buckets[key]
	}
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
		if _, ok := bucket.targets[target]; !ok && len(bucket.targets) >= DefaultRollupMaxTargetSummary {
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
		targets := topRollupTargets(bucket.targets, DefaultRollupMaxTargetSummary)
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
		if bucket == nil && a.maxBuckets > 0 && len(a.buckets) >= a.maxBuckets {
			key = overflowRollupKey(key)
			bucket = a.buckets[key]
		}
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
			target = strings.TrimSpace(target)
			if target == "" || count <= 0 {
				continue
			}
			if _, ok := bucket.targets[target]; !ok && len(bucket.targets) >= DefaultRollupMaxTargetSummary {
				continue
			}
			bucket.targets[target] += count
		}
	}
}

func overflowRollupKey(key rollupKey) rollupKey {
	return rollupKey{
		WindowStart: key.WindowStart,
		SourceScope: "global",
		QName:       rollupOverflowQName,
		QType:       "ANY",
		RCode:       key.RCode,
	}
}

func topRollupTargets(values map[string]int64, limit int) map[string]int64 {
	if len(values) == 0 {
		return map[string]int64{}
	}
	type targetCount struct {
		target string
		count  int64
	}
	items := make([]targetCount, 0, len(values))
	for target, count := range values {
		target = strings.TrimSpace(target)
		if target == "" || count <= 0 {
			continue
		}
		items = append(items, targetCount{target: target, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].target < items[j].target
		}
		return items[i].count > items[j].count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	result := make(map[string]int64, len(items))
	for _, item := range items {
		result[item.target] += item.count
	}
	return result
}

func normalizeSourceScope(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "global"
	}
	base, suffix, hasSuffix := strings.Cut(value, "|")
	normalizedBase := normalizeSourceScopeBase(base)
	if hasSuffix {
		if bucket := normalizeSourceScopeBucket(suffix); bucket != "" {
			return normalizedBase + "|" + bucket
		}
		return normalizedBase
	}
	return normalizedBase
}

func normalizeSourceScopeBase(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "global"
	}
	prefix, scopeValue, ok := strings.Cut(value, ":")
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "country") {
		country := strings.ToUpper(strings.TrimSpace(scopeValue))
		if len(country) == 2 {
			return "country:" + country
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "cidr") {
		if cidr, valid := normalizeCIDR(scopeValue); valid {
			return "cidr:" + cidr
		}
		if _, network, err := net.ParseCIDR(strings.TrimSpace(scopeValue)); err == nil {
			return "cidr:" + network.String()
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "operator") {
		if operator := normalizeOperator(scopeValue); operator != "" {
			return "operator:" + operator
		}
	}
	if ok && strings.EqualFold(strings.TrimSpace(prefix), "asn") {
		asn, err := strconv.ParseUint(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(scopeValue)), "AS"), 10, 32)
		if err == nil && asn > 0 {
			return "asn:" + strconv.FormatUint(asn, 10)
		}
	}
	return value
}

func normalizeSourceScopeBucket(raw string) string {
	prefix, value, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || !strings.EqualFold(strings.TrimSpace(prefix), "bucket") {
		return ""
	}
	bucket, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || bucket < 0 || bucket > 99 {
		return ""
	}
	return fmt.Sprintf("bucket:%02d", bucket)
}

func normalizeRCode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "NOERROR", "NXDOMAIN", "NODATA", "SERVFAIL", "REFUSED":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return "NOERROR"
	}
}
