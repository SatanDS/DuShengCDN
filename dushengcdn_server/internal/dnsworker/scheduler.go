package dnsworker

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Scheduler struct {
	mu     sync.Mutex
	states map[string]debounceState
	now    func() time.Time
}

type debounceState struct {
	Targets       []string
	Desired       []string
	LastChangedAt time.Time
}

type targetCandidate struct {
	NodeID               string
	PoolName             string
	Content              string
	NodeWeight           int
	PoolWeight           int
	LastSeenAt           time.Time
	OpenrestyConnections int64
	CPUUsagePercent      float64
	MemoryUsagePercent   float64
	HasMetric            bool
	Score                float64
}

type sourceSpread struct {
	Key    string
	Bucket int
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		states: map[string]debounceState{},
		now:    time.Now,
	}
}

func (s *Scheduler) SnapshotStates(snapshot *Snapshot) []SnapshotSchedulingState {
	if s == nil || snapshot == nil {
		return nil
	}
	routeTypes := make(map[uint]string, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		if route.ID == 0 {
			continue
		}
		recordType := normalizeAddressRecordType(route.RecordType)
		if recordType != "A" && recordType != "AAAA" {
			continue
		}
		routeTypes[route.ID] = recordType
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	states := make([]SnapshotSchedulingState, 0, len(s.states))
	for key, state := range s.states {
		routeID, recordType, scopeKey, ok := parseSchedulerStateKey(key)
		if !ok {
			continue
		}
		expectedType, exists := routeTypes[routeID]
		if !exists || expectedType != recordType || len(state.Targets) == 0 || state.LastChangedAt.IsZero() {
			continue
		}
		lastChangedAt := state.LastChangedAt.UTC()
		states = append(states, SnapshotSchedulingState{
			RouteID:         routeID,
			RecordType:      recordType,
			ScopeKey:        scopeKey,
			SelectedTargets: append([]string(nil), state.Targets...),
			DesiredTargets:  append([]string(nil), state.Desired...),
			LastChangedAt:   &lastChangedAt,
		})
	}
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].RouteID != states[j].RouteID {
			return states[i].RouteID < states[j].RouteID
		}
		if states[i].RecordType != states[j].RecordType {
			return states[i].RecordType < states[j].RecordType
		}
		return states[i].ScopeKey < states[j].ScopeKey
	})
	return states
}

func (s *Scheduler) LoadSnapshotStates(snapshot *Snapshot) {
	if s == nil || snapshot == nil {
		return
	}
	routeIDs := make(map[uint]struct{}, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		if route.ID != 0 {
			routeIDs[route.ID] = struct{}{}
		}
	}
	snapshotStates := make(map[string]debounceState, len(snapshot.SchedulingStates))
	for _, item := range snapshot.SchedulingStates {
		recordType := normalizeAddressRecordType(item.RecordType)
		scopeKey := normalizeSourceScope(item.ScopeKey)
		targets := normalizeIPList(item.SelectedTargets, recordType)
		desired := normalizeIPList(item.DesiredTargets, recordType)
		if item.RouteID == 0 || len(targets) == 0 || item.LastChangedAt == nil || item.LastChangedAt.IsZero() {
			continue
		}
		if _, ok := routeIDs[item.RouteID]; !ok {
			continue
		}
		snapshotStates[schedulerStateKey(item.RouteID, recordType, scopeKey)] = debounceState{
			Targets:       targets,
			Desired:       desired,
			LastChangedAt: item.LastChangedAt.UTC(),
		}
	}
	s.mu.Lock()
	next := make(map[string]debounceState, len(s.states)+len(snapshotStates))
	for key, state := range s.states {
		routeID, ok := schedulerStateRouteID(key)
		if !ok {
			continue
		}
		if _, exists := routeIDs[routeID]; exists {
			next[key] = state
		}
	}
	for key, state := range snapshotStates {
		next[key] = state
	}
	s.states = next
	s.mu.Unlock()
}

func (s *Scheduler) Select(snapshot *Snapshot, route *SnapshotRoute, recordType string, source SourceContext, fresh bool) ([]string, int, string, error) {
	if route == nil {
		return nil, DefaultAuthoritativeTTL, "global", errors.New("route is nil")
	}
	recordType = normalizeAddressRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return nil, normalizeAuthoritativeTTL(route.TTL), sourceScopeKey(source), errors.New("GSLB only supports A/AAAA")
	}
	if !fresh {
		return nil, normalizeAuthoritativeTTL(route.TTL), sourceScopeKey(source), errors.New("snapshot is stale")
	}
	policy := normalizePolicy(route.GSLBPolicy, *route)
	if !route.GSLBEnabled {
		targets := normalizeIPList(route.CurrentTargets, recordType)
		if len(targets) > 0 {
			return limitTargets(targets, route.TargetCount), normalizeAuthoritativeTTL(route.TTL), "global", nil
		}
		policy = GSLBPolicy{
			Strategy:    normalizeStrategy(route.ScheduleMode),
			TargetCount: normalizeTargetCount(route.TargetCount),
			TTL:         normalizeAuthoritativeTTL(route.TTL),
			Pools: []GSLBPoolPolicy{
				{Name: normalizeNodePoolName(route.NodePool), Weight: 100, Enabled: true},
			},
			Debounce: normalizeDebounce(GSLBDebounce{}),
		}
	}
	baseScopeKey := sourceScopeKeyForPolicy(policy, source)
	spread := sourceSpreadForPolicy(policy, route.ID, recordType, source, baseScopeKey)
	scopeKey := baseScopeKey
	if spread != nil {
		scopeKey = fmt.Sprintf("%s|bucket:%02d", baseScopeKey, spread.Bucket)
		spread.Key = schedulerStateKey(route.ID, recordType, scopeKey)
	}
	candidates := buildCandidates(snapshot, recordType, policy, source)
	if len(candidates) == 0 {
		return nil, normalizeAuthoritativeTTL(policy.TTL), scopeKey, fmt.Errorf("no online public node IP is available for %s records", recordType)
	}
	desired := selectWeightedTargets(candidates, policy, spread)
	if len(desired) == 0 {
		return nil, normalizeAuthoritativeTTL(policy.TTL), scopeKey, fmt.Errorf("no target selected for %s records", recordType)
	}
	selected := s.applyDebounce(route.ID, recordType, scopeKey, desired, candidates, policy)
	return selected, normalizeAuthoritativeTTL(policy.TTL), scopeKey, nil
}

func (s *Scheduler) applyDebounce(routeID uint, recordType string, scopeKey string, desired []string, candidates []targetCandidate, policy GSLBPolicy) []string {
	if s == nil {
		return desired
	}
	key := schedulerStateKey(routeID, recordType, scopeKey)
	eligible := map[string]struct{}{}
	for _, candidate := range candidates {
		eligible[candidate.Content] = struct{}{}
	}
	now := s.now()
	cooldown := time.Duration(policy.Debounce.CooldownSeconds) * time.Second
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	selected := desired
	if len(state.Targets) > 0 &&
		!sameStringSet(state.Targets, desired) &&
		allTargetsEligible(state.Targets, eligible) &&
		!state.LastChangedAt.IsZero() &&
		now.Sub(state.LastChangedAt) < cooldown {
		selected = append([]string(nil), state.Targets...)
	}
	if state.LastChangedAt.IsZero() || !sameStringSet(state.Targets, selected) {
		state.LastChangedAt = now
	}
	state.Targets = append([]string(nil), selected...)
	state.Desired = append([]string(nil), desired...)
	s.states[key] = state
	return selected
}

func schedulerStateKey(routeID uint, recordType string, scopeKey string) string {
	return fmt.Sprintf("%d|%s|%s", routeID, normalizeAddressRecordType(recordType), normalizeSourceScope(scopeKey))
}

func schedulerStateRouteID(key string) (uint, bool) {
	routeID, _, _, ok := parseSchedulerStateKey(key)
	return routeID, ok
}

func parseSchedulerStateKey(key string) (uint, string, string, bool) {
	rawRouteID, rest, ok := strings.Cut(key, "|")
	if !ok {
		return 0, "", "", false
	}
	parts := strings.SplitN(rest, "|", 3)
	if len(parts) < 2 {
		return 0, "", "", false
	}
	recordType := parts[0]
	scopeKey := parts[1]
	if len(parts) == 3 {
		scopeKey += "|" + parts[2]
	}
	routeID, err := strconv.ParseUint(rawRouteID, 10, 0)
	if err != nil || routeID == 0 {
		return 0, "", "", false
	}
	return uint(routeID), normalizeAddressRecordType(recordType), normalizeSourceScope(scopeKey), true
}

func buildCandidates(snapshot *Snapshot, recordType string, policy GSLBPolicy, source SourceContext) []targetCandidate {
	if snapshot == nil {
		return nil
	}
	pools := matchPoolsForSource(policy.Pools, source)
	candidates := make([]targetCandidate, 0)
	seen := map[string]struct{}{}
	for _, node := range snapshot.Nodes {
		pool, ok := pools[normalizeNodePoolName(node.PoolName)]
		if !ok {
			continue
		}
		if !isNodeSchedulable(node) || !metricWithinThresholds(node, policy.LoadThresholds) {
			continue
		}
		for _, value := range node.PublicIPs {
			ip := net.ParseIP(strings.TrimSpace(value))
			if ip == nil {
				continue
			}
			if ipv4 := ip.To4(); ipv4 != nil {
				ip = ipv4
			}
			if recordType == "A" && ip.To4() == nil {
				continue
			}
			if recordType == "AAAA" && ip.To4() != nil {
				continue
			}
			content := ip.String()
			if _, ok := seen[content]; ok {
				continue
			}
			seen[content] = struct{}{}
			candidate := targetCandidate{
				NodeID:               node.NodeID,
				PoolName:             normalizeNodePoolName(node.PoolName),
				Content:              content,
				NodeWeight:           normalizeWeight(node.Weight),
				PoolWeight:           normalizeWeight(pool.Weight),
				LastSeenAt:           node.LastSeenAt,
				OpenrestyConnections: node.OpenrestyConnections,
				CPUUsagePercent:      node.CPUUsagePercent,
				MemoryUsagePercent:   node.MemoryUsagePercent,
				HasMetric:            node.MetricCapturedAt != nil,
			}
			candidate.Score = scoreCandidate(candidate, policy.Strategy)
			candidates = append(candidates, candidate)
		}
	}
	sortCandidates(candidates, policy.Strategy)
	return candidates
}

func matchPoolsForSource(pools []GSLBPoolPolicy, source SourceContext) map[string]GSLBPoolPolicy {
	all := map[string]GSLBPoolPolicy{}
	cidrMatched := map[string]GSLBPoolPolicy{}
	matched := map[string]GSLBPoolPolicy{}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled {
			continue
		}
		all[name] = pool
		if _, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			cidrMatched[name] = pool
			continue
		}
		if country == "" {
			continue
		}
		for _, item := range pool.Countries {
			if country == strings.ToUpper(strings.TrimSpace(item)) {
				matched[name] = pool
				break
			}
		}
	}
	if len(cidrMatched) > 0 {
		return cidrMatched
	}
	if len(matched) > 0 {
		return matched
	}
	return all
}

func sourceScopeKeyForPolicy(policy GSLBPolicy, source SourceContext) string {
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		if cidr, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			return "cidr:" + cidr
		}
	}
	return sourceScopeKey(source)
}

func sourceSpreadForPolicy(policy GSLBPolicy, routeID uint, recordType string, source SourceContext, baseScopeKey string) *sourceSpread {
	if !usesSourceWeightedSpread(policy) {
		return nil
	}
	ip := net.ParseIP(strings.TrimSpace(source.IP))
	if ip == nil {
		return nil
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	key := fmt.Sprintf("%d|%s|%s|ip:%s", routeID, normalizeAddressRecordType(recordType), normalizeSourceScope(baseScopeKey), ip.String())
	return &sourceSpread{
		Key:    key,
		Bucket: int(stableHashUint64(key) % 100),
	}
}

func usesSourceWeightedSpread(policy GSLBPolicy) bool {
	switch normalizeStrategy(policy.Strategy) {
	case "weighted", "load_aware":
		return true
	default:
		return false
	}
}

func sourceIPMatchesCIDRList(sourceIP string, cidrs []string) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(sourceIP))
	if ip == nil {
		return "", false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	for _, value := range cidrs {
		cidr, ok := normalizeCIDR(value)
		if !ok {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return cidr, true
		}
	}
	return "", false
}

func isNodeSchedulable(node SnapshotNode) bool {
	if node.DrainMode || !node.SchedulingEnabled {
		return false
	}
	if strings.ToLower(strings.TrimSpace(node.Status)) != "online" {
		return false
	}
	openrestyStatus := strings.ToLower(strings.TrimSpace(node.OpenrestyStatus))
	if openrestyStatus == "unhealthy" {
		return false
	}
	return len(node.PublicIPs) > 0
}

func metricWithinThresholds(node SnapshotNode, thresholds GSLBLoadThresholds) bool {
	if thresholds.MaxOpenrestyConnections > 0 && node.OpenrestyConnections > thresholds.MaxOpenrestyConnections {
		return false
	}
	if thresholds.MaxCPUPercent > 0 && node.CPUUsagePercent > thresholds.MaxCPUPercent {
		return false
	}
	if thresholds.MaxMemoryPercent > 0 && node.MemoryUsagePercent > thresholds.MaxMemoryPercent {
		return false
	}
	return true
}

func scoreCandidate(candidate targetCandidate, strategy string) float64 {
	base := float64(candidate.NodeWeight * candidate.PoolWeight)
	if base <= 0 {
		base = 1
	}
	if strategy != "load_aware" {
		return base
	}
	penalty := 1.0
	if candidate.OpenrestyConnections > 0 {
		penalty += float64(candidate.OpenrestyConnections) / 100
	}
	if candidate.CPUUsagePercent > 0 {
		penalty += candidate.CPUUsagePercent / 100
	}
	if candidate.MemoryUsagePercent > 0 {
		penalty += candidate.MemoryUsagePercent / 100
	}
	return base / penalty
}

func sortCandidates(candidates []targetCandidate, strategy string) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if strategy == "weighted" || strategy == "load_aware" {
			if left.Score != right.Score {
				return left.Score > right.Score
			}
		}
		if strategy == "load_aware" && left.OpenrestyConnections != right.OpenrestyConnections {
			return left.OpenrestyConnections < right.OpenrestyConnections
		}
		if !left.LastSeenAt.Equal(right.LastSeenAt) {
			return left.LastSeenAt.After(right.LastSeenAt)
		}
		if left.NodeID != right.NodeID {
			return left.NodeID < right.NodeID
		}
		return left.Content < right.Content
	})
}

func selectWeightedTargets(candidates []targetCandidate, policy GSLBPolicy, spread *sourceSpread) []string {
	if spread != nil && usesSourceWeightedSpread(policy) {
		return selectSourceSpreadTargets(candidates, policy, *spread)
	}
	return selectRankedTargets(candidates, policy)
}

func selectRankedTargets(candidates []targetCandidate, policy GSLBPolicy) []string {
	targetCount := normalizeTargetCount(policy.TargetCount)
	if len(candidates) == 0 {
		return nil
	}
	byPool := map[string][]targetCandidate{}
	for _, candidate := range candidates {
		byPool[candidate.PoolName] = append(byPool[candidate.PoolName], candidate)
	}
	quotas := allocatePoolQuotas(policy.Pools, targetCount)
	selected := make([]targetCandidate, 0, targetCount)
	used := map[string]struct{}{}
	for _, pool := range policy.Pools {
		poolName := normalizeNodePoolName(pool.Name)
		quota := quotas[poolName]
		for _, candidate := range byPool[poolName] {
			if quota <= 0 || len(selected) >= targetCount {
				break
			}
			if _, ok := used[candidate.Content]; ok {
				continue
			}
			selected = append(selected, candidate)
			used[candidate.Content] = struct{}{}
			quota--
		}
	}
	if len(selected) < targetCount {
		for _, candidate := range candidates {
			if len(selected) >= targetCount {
				break
			}
			if _, ok := used[candidate.Content]; ok {
				continue
			}
			selected = append(selected, candidate)
			used[candidate.Content] = struct{}{}
		}
	}
	sortCandidates(selected, policy.Strategy)
	targets := make([]string, 0, len(selected))
	for _, candidate := range selected {
		targets = append(targets, candidate.Content)
	}
	return targets
}

func selectSourceSpreadTargets(candidates []targetCandidate, policy GSLBPolicy, spread sourceSpread) []string {
	targetCount := normalizeTargetCount(policy.TargetCount)
	if len(candidates) == 0 {
		return nil
	}
	byPool := map[string][]targetCandidate{}
	for _, candidate := range candidates {
		byPool[candidate.PoolName] = append(byPool[candidate.PoolName], candidate)
	}
	pools := weightedAvailablePools(policy.Pools, byPool)
	if len(pools) == 0 {
		return nil
	}
	selected := make([]targetCandidate, 0, targetCount)
	usedTargets := map[string]struct{}{}
	for slot := 0; slot < targetCount && len(selected) < len(candidates); slot++ {
		bucket := (spread.Bucket + slot*37) % 100
		poolName := selectPoolByBucket(pools, bucket)
		if poolName == "" {
			continue
		}
		candidate, ok := selectCandidateBySpread(byPool[poolName], policy.Strategy, spread.Key, slot, usedTargets)
		if !ok {
			continue
		}
		selected = append(selected, candidate)
		usedTargets[candidate.Content] = struct{}{}
	}
	if len(selected) < targetCount {
		for _, candidate := range selectRankedCandidates(candidates, policy.Strategy) {
			if len(selected) >= targetCount {
				break
			}
			if _, ok := usedTargets[candidate.Content]; ok {
				continue
			}
			selected = append(selected, candidate)
			usedTargets[candidate.Content] = struct{}{}
		}
	}
	sortCandidates(selected, policy.Strategy)
	targets := make([]string, 0, len(selected))
	for _, candidate := range selected {
		targets = append(targets, candidate.Content)
	}
	return targets
}

type weightedAvailablePool struct {
	Name   string
	Weight int
}

func weightedAvailablePools(pools []GSLBPoolPolicy, byPool map[string][]targetCandidate) []weightedAvailablePool {
	result := make([]weightedAvailablePool, 0, len(pools))
	seen := map[string]struct{}{}
	for _, pool := range pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		if len(byPool[name]) == 0 {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, weightedAvailablePool{Name: name, Weight: normalizeWeight(pool.Weight)})
	}
	return result
}

func selectPoolByBucket(pools []weightedAvailablePool, bucket int) string {
	if len(pools) == 0 {
		return ""
	}
	totalWeight := 0
	for _, pool := range pools {
		totalWeight += normalizeWeight(pool.Weight)
	}
	if totalWeight <= 0 {
		return ""
	}
	if bucket < 0 {
		bucket = 0
	}
	if bucket > 99 {
		bucket = bucket % 100
	}
	point := bucket * totalWeight / 100
	accumulated := 0
	for _, pool := range pools {
		accumulated += normalizeWeight(pool.Weight)
		if point < accumulated {
			return pool.Name
		}
	}
	return pools[len(pools)-1].Name
}

func selectCandidateBySpread(candidates []targetCandidate, strategy string, key string, slot int, used map[string]struct{}) (targetCandidate, bool) {
	best := targetCandidate{}
	bestScore := math.Inf(1)
	found := false
	for _, candidate := range candidates {
		if _, ok := used[candidate.Content]; ok {
			continue
		}
		weight := spreadCandidateWeight(candidate, strategy)
		if weight <= 0 {
			weight = 1
		}
		hashKey := fmt.Sprintf("%s|slot:%d|candidate:%s", key, slot, candidate.Content)
		unit := stableHashUnit(hashKey)
		score := -math.Log(unit) / weight
		if !found || score < bestScore || (score == bestScore && candidate.Content < best.Content) {
			best = candidate
			bestScore = score
			found = true
		}
	}
	return best, found
}

func spreadCandidateWeight(candidate targetCandidate, strategy string) float64 {
	if normalizeStrategy(strategy) == "load_aware" {
		return candidate.Score
	}
	return float64(normalizeWeight(candidate.NodeWeight))
}

func selectRankedCandidates(candidates []targetCandidate, strategy string) []targetCandidate {
	result := append([]targetCandidate(nil), candidates...)
	sortCandidates(result, strategy)
	return result
}

func stableHashUint64(value string) uint64 {
	sum := sha256.Sum256([]byte(value))
	return binary.BigEndian.Uint64(sum[:8])
}

func stableHashUnit(value string) float64 {
	hash := stableHashUint64(value)
	return (float64(hash) + 1) / (float64(^uint64(0)) + 2)
}

func allocatePoolQuotas(pools []GSLBPoolPolicy, targetCount int) map[string]int {
	type weightedPool struct {
		Name      string
		Weight    int
		Quota     int
		Remainder int
	}
	targetCount = normalizeTargetCount(targetCount)
	weighted := make([]weightedPool, 0, len(pools))
	totalWeight := 0
	for _, pool := range pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		weight := normalizeWeight(pool.Weight)
		totalWeight += weight
		weighted = append(weighted, weightedPool{Name: name, Weight: weight})
	}
	quotas := map[string]int{}
	if len(weighted) == 0 || totalWeight <= 0 {
		return quotas
	}
	assigned := 0
	for i := range weighted {
		product := weighted[i].Weight * targetCount
		weighted[i].Quota = product / totalWeight
		weighted[i].Remainder = product % totalWeight
		assigned += weighted[i].Quota
	}
	sort.SliceStable(weighted, func(i, j int) bool {
		if weighted[i].Remainder != weighted[j].Remainder {
			return weighted[i].Remainder > weighted[j].Remainder
		}
		if weighted[i].Weight != weighted[j].Weight {
			return weighted[i].Weight > weighted[j].Weight
		}
		return weighted[i].Name < weighted[j].Name
	})
	for assigned < targetCount {
		for i := range weighted {
			if assigned >= targetCount {
				break
			}
			weighted[i].Quota++
			assigned++
		}
	}
	for _, pool := range weighted {
		quotas[pool.Name] = pool.Quota
	}
	return quotas
}

func limitTargets(targets []string, count int) []string {
	count = normalizeTargetCount(count)
	if len(targets) > count {
		return append([]string(nil), targets[:count]...)
	}
	return append([]string(nil), targets...)
}

func allTargetsEligible(targets []string, eligible map[string]struct{}) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if _, ok := eligible[strings.TrimSpace(target)]; !ok {
			return false
		}
	}
	return true
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counter := map[string]int{}
	for _, value := range left {
		counter[strings.TrimSpace(value)]++
	}
	for _, value := range right {
		key := strings.TrimSpace(value)
		if counter[key] <= 0 {
			return false
		}
		counter[key]--
	}
	return true
}
