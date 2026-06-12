package dnsworker

import (
	"sync"
	"testing"
)

func TestCachedSchedulerCIDRListCompilesNetipPrefixesBehindStableKey(t *testing.T) {
	schedulerCIDRListCache = sync.Map{}
	cidrs := []string{
		" 203.0.113.7 ",
		"203.0.113.0/24",
		"bad-cidr",
		"203.0.113.0/24",
		"2001:db8::1/64",
	}

	compiled := cachedSchedulerCIDRList(cidrs)
	if len(compiled.items) != 3 {
		t.Fatalf("expected three compiled CIDRs after invalid/duplicate entries are removed, got %+v", compiled.items)
	}
	if got := compiled.items[0].text; got != "203.0.113.7/32" {
		t.Fatalf("expected host CIDR to normalize to /32, got %q", got)
	}
	if got := compiled.items[1].text; got != "203.0.113.0/24" {
		t.Fatalf("expected network CIDR to remain normalized, got %q", got)
	}
	if got := compiled.items[2].text; got != "2001:db8::/64" {
		t.Fatalf("expected IPv6 CIDR to be masked, got %q", got)
	}

	if cidr, ok := sourceIPMatchesCIDRList("203.0.113.7", cidrs); !ok || cidr != "203.0.113.7/32" {
		t.Fatalf("expected source host match, got cidr=%q ok=%v", cidr, ok)
	}
	if cidr, ok := sourceIPMatchesCIDRList("203.0.113.99", cidrs); !ok || cidr != "203.0.113.0/24" {
		t.Fatalf("expected source network match, got cidr=%q ok=%v", cidr, ok)
	}
	if cidr, ok := sourceIPMatchesCIDRList("2001:db8::abcd", cidrs); !ok || cidr != "2001:db8::/64" {
		t.Fatalf("expected IPv6 source match, got cidr=%q ok=%v", cidr, ok)
	}

	entriesAfterFirstCompile := schedulerCIDRCacheEntryCount()
	if entriesAfterFirstCompile != 1 {
		t.Fatalf("expected one cache entry, got %d", entriesAfterFirstCompile)
	}
	_ = cachedSchedulerCIDRList(cidrs)
	if entries := schedulerCIDRCacheEntryCount(); entries != entriesAfterFirstCompile {
		t.Fatalf("expected repeated lookup to reuse cache entry count %d, got %d", entriesAfterFirstCompile, entries)
	}
}

func TestNormalizePolicyPrecompilesPoolCIDRs(t *testing.T) {
	schedulerCIDRListCache = sync.Map{}
	policy := normalizePolicy(GSLBPolicy{
		Pools: []GSLBPoolPolicy{
			{
				Name:               "edge",
				Weight:             100,
				SourceCIDRs:        []string{"203.0.113.0/24", "bad-cidr"},
				ExcludeSourceCIDRs: []string{"2001:db8::/64"},
				Enabled:            true,
			},
		},
	}, SnapshotRoute{ID: 101, NodePool: "edge", RecordType: "A", TargetCount: 1, TTL: 30})

	if len(policy.Pools) != 1 {
		t.Fatalf("expected one pool, got %+v", policy.Pools)
	}
	pool := policy.Pools[0]
	if len(pool.compiledSourceCIDRs.items) != 1 || pool.compiledSourceCIDRs.items[0].text != "203.0.113.0/24" {
		t.Fatalf("expected source CIDR to be precompiled, got %+v", pool.compiledSourceCIDRs.items)
	}
	if len(pool.compiledExcludeSourceCIDRs.items) != 1 || pool.compiledExcludeSourceCIDRs.items[0].text != "2001:db8::/64" {
		t.Fatalf("expected exclude CIDR to be precompiled, got %+v", pool.compiledExcludeSourceCIDRs.items)
	}
	if entries := schedulerCIDRCacheEntryCount(); entries != 0 {
		t.Fatalf("expected normalization precompile not to populate global fallback cache, got %d entries", entries)
	}
	if cidr, ok := sourceIPMatchesPoolCIDRList("203.0.113.99", pool, false); !ok || cidr != "203.0.113.0/24" {
		t.Fatalf("expected source match from precompiled pool CIDR, got cidr=%q ok=%v", cidr, ok)
	}
	if cidr, ok := sourceIPMatchesPoolCIDRList("2001:db8::abcd", pool, true); !ok || cidr != "2001:db8::/64" {
		t.Fatalf("expected exclude match from precompiled pool CIDR, got cidr=%q ok=%v", cidr, ok)
	}
	if entries := schedulerCIDRCacheEntryCount(); entries != 0 {
		t.Fatalf("expected precompiled pool matching not to populate global fallback cache, got %d entries", entries)
	}
}

func TestSchedulerCachesFallbackNormalizedPolicyPerSnapshot(t *testing.T) {
	scheduler := NewScheduler()
	first := schedulerPolicyCacheTestSnapshot("first", "edge", "8.8.4.4")

	targets, _, _, err := scheduler.Select(first, &first.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select first snapshot: %v", err)
	}
	if len(targets) != 1 || targets[0] != "8.8.4.4" {
		t.Fatalf("expected first snapshot target, got %+v", targets)
	}
	if snapshot, entries := schedulerPolicyCacheState(scheduler); snapshot != first || entries != 1 {
		t.Fatalf("expected first snapshot policy cache with one entry, got snapshot=%p entries=%d", snapshot, entries)
	}

	targets, _, _, err = scheduler.Select(first, &first.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select first snapshot again: %v", err)
	}
	if len(targets) != 1 || targets[0] != "8.8.4.4" {
		t.Fatalf("expected cached first snapshot target, got %+v", targets)
	}
	if snapshot, entries := schedulerPolicyCacheState(scheduler); snapshot != first || entries != 1 {
		t.Fatalf("expected first snapshot policy cache to stay bounded, got snapshot=%p entries=%d", snapshot, entries)
	}

	second := schedulerPolicyCacheTestSnapshot("second", "next", "9.9.9.9")
	targets, _, _, err = scheduler.Select(second, &second.Routes[0], "A", SourceContext{}, true)
	if err != nil {
		t.Fatalf("select second snapshot: %v", err)
	}
	if len(targets) != 1 || targets[0] != "9.9.9.9" {
		t.Fatalf("expected second snapshot target, got %+v", targets)
	}
	if snapshot, entries := schedulerPolicyCacheState(scheduler); snapshot != second || entries != 1 {
		t.Fatalf("expected cache to reset to second snapshot with one entry, got snapshot=%p entries=%d", snapshot, entries)
	}
}

func TestSchedulerHotPathCachesAreConcurrentSafe(t *testing.T) {
	schedulerCIDRListCache = sync.Map{}
	scheduler := NewScheduler()
	snapshot := schedulerPolicyCacheTestSnapshot("concurrent", "edge", "8.8.4.4")
	route := &snapshot.Routes[0]
	cidrs := []string{"203.0.113.0/24", "2001:db8::/64"}

	const workers = 32
	const iterations = 100
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				targets, _, _, err := scheduler.Select(snapshot, route, "A", SourceContext{}, true)
				if err != nil {
					errs <- err.Error()
					return
				}
				if len(targets) != 1 || targets[0] != "8.8.4.4" {
					errs <- "unexpected scheduler target"
					return
				}
				if cidr, ok := sourceIPMatchesCIDRList("203.0.113.99", cidrs); !ok || cidr != "203.0.113.0/24" {
					errs <- "unexpected IPv4 CIDR match"
					return
				}
				if cidr, ok := sourceIPMatchesCIDRList("2001:db8::abcd", cidrs); !ok || cidr != "2001:db8::/64" {
					errs <- "unexpected IPv6 CIDR match"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if cachedSnapshot, entries := schedulerPolicyCacheState(scheduler); cachedSnapshot != snapshot || entries != 1 {
		t.Fatalf("expected concurrent policy cache to stay on one snapshot entry, got snapshot=%p entries=%d", cachedSnapshot, entries)
	}
}

func schedulerCIDRCacheEntryCount() int {
	count := 0
	schedulerCIDRListCache.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func schedulerPolicyCacheState(scheduler *Scheduler) (*Snapshot, int) {
	scheduler.policyMu.RLock()
	defer scheduler.policyMu.RUnlock()
	return scheduler.policyCacheSnapshot, len(scheduler.policyCache)
}

func schedulerPolicyCacheTestSnapshot(version string, pool string, ip string) *Snapshot {
	snapshot := baseSnapshot()
	snapshot.SnapshotVersion = version
	snapshot.Routes = []SnapshotRoute{
		{
			ID:           101,
			Domains:      []string{"edge.example.com"},
			ZoneID:       1,
			NodePool:     pool,
			RecordType:   "A",
			TargetCount:  1,
			ScheduleMode: "weighted",
			TTL:          30,
			GSLBEnabled:  true,
			GSLBPolicy: GSLBPolicy{
				Strategy:    "weighted",
				TargetCount: 1,
				TTL:         30,
				Pools: []GSLBPoolPolicy{
					{Name: pool, Weight: 100},
				},
			},
		},
	}
	snapshot.Nodes = []SnapshotNode{
		testNode(pool+"-node", pool, ip, 100, 1),
	}
	return snapshot
}
