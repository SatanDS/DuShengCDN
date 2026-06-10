package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type proxyRouteDNSTargetSelection struct {
	Targets        []string
	DesiredTargets []string
	UnhealthyCount int
	RecoveryCount  int
	TTL            int
	GSLB           bool
	Reason         string
	LastChangedAt  *time.Time
	ScopeKey       string
}

type gslbDNSTargetCandidate struct {
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
	DNSProbeHealthy      bool
	DNSProbeCheckedCount int
	DNSProbeHealthyCount int
	DNSProbeStaleCount   int
	DNSProbeAverageRTTMs float64
	Score                float64
}

type gslbDNSSchedulingOptions struct {
	RequireHealthyDNSProbe bool
	Data                   *gslbDNSSchedulingData
}

type gslbDNSSchedulingData struct {
	Nodes            []*model.Node
	MetricsByNode    map[string]*model.NodeMetricSnapshot
	ProbeStatsByNode map[string]*dnsWorkerNodeProbeStats
}

type gslbDNSSchedulingDataQueries struct {
	ListNodes func() ([]*model.Node, error)
}

var defaultGSLBDNSSchedulingDataQueries = gslbDNSSchedulingDataQueries{
	ListNodes: model.ListNodes,
}

func loadGSLBDNSSchedulingData(includeProbeStats bool) (*gslbDNSSchedulingData, error) {
	return loadGSLBDNSSchedulingDataWithQueries(includeProbeStats, defaultGSLBDNSSchedulingDataQueries)
}

func loadGSLBDNSSchedulingDataWithQueries(includeProbeStats bool, queries gslbDNSSchedulingDataQueries) (*gslbDNSSchedulingData, error) {
	listNodes := queries.ListNodes
	if listNodes == nil {
		listNodes = model.ListNodes
	}
	nodes, err := listNodes()
	if err != nil {
		return nil, err
	}
	data := &gslbDNSSchedulingData{
		Nodes:         nodes,
		MetricsByNode: latestNodeMetricSnapshots(),
	}
	if includeProbeStats {
		data.ProbeStatsByNode = buildDNSWorkerNodeProbeStatsByNodeForNodes(time.Now().UTC(), nodes)
	}
	return data, nil
}

func gslbDNSSchedulingNodes(options gslbDNSSchedulingOptions) ([]*model.Node, error) {
	if options.Data != nil {
		return options.Data.Nodes, nil
	}
	return model.ListNodes()
}

func gslbDNSSchedulingMetricsByNode(options gslbDNSSchedulingOptions) map[string]*model.NodeMetricSnapshot {
	if options.Data != nil && options.Data.MetricsByNode != nil {
		return options.Data.MetricsByNode
	}
	return latestNodeMetricSnapshots()
}

func gslbDNSSchedulingProbeStatsByNode(options gslbDNSSchedulingOptions) map[string]*dnsWorkerNodeProbeStats {
	if options.Data != nil && options.Data.ProbeStatsByNode != nil {
		return options.Data.ProbeStatsByNode
	}
	return buildDNSWorkerNodeProbeStatsByNode(time.Now().UTC())
}

func selectProxyRouteDNSTargets(route *model.ProxyRoute, recordType string) (proxyRouteDNSTargetSelection, error) {
	return selectProxyRouteDNSTargetsWithOptions(route, recordType, gslbDNSSchedulingOptions{})
}

func selectProxyRouteDNSTargetsWithOptions(route *model.ProxyRoute, recordType string, options gslbDNSSchedulingOptions) (proxyRouteDNSTargetSelection, error) {
	selection := proxyRouteDNSTargetSelection{
		TTL:      cloudflareDefaultRecordTTL,
		ScopeKey: defaultGSLBScopeKey,
	}
	if route == nil {
		return selection, errors.New("proxy route is nil")
	}
	selection.TTL = normalizeDNSTTL(route.DNSTTL)
	if route.GSLBEnabled {
		return selectGSLBDNSTargetsWithOptions(route, recordType, GSLBSourceContext{}, options)
	}
	targets, err := selectHealthyNodeDNSTargetsWithOptions(recordType, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, options)
	if err != nil {
		return selection, err
	}
	selection.Targets = targets
	selection.DesiredTargets = targets
	selection.ScopeKey = defaultGSLBScopeKey
	return selection, nil
}

func selectGSLBDNSTargets(route *model.ProxyRoute, recordType string) (proxyRouteDNSTargetSelection, error) {
	return selectGSLBDNSTargetsForSource(route, recordType, GSLBSourceContext{})
}

func selectGSLBDNSTargetsForSource(route *model.ProxyRoute, recordType string, source GSLBSourceContext) (proxyRouteDNSTargetSelection, error) {
	return selectGSLBDNSTargetsWithOptions(route, recordType, source, gslbDNSSchedulingOptions{})
}

func selectGSLBDNSTargetsWithOptions(route *model.ProxyRoute, recordType string, source GSLBSourceContext, options gslbDNSSchedulingOptions) (proxyRouteDNSTargetSelection, error) {
	selection := proxyRouteDNSTargetSelection{
		TTL:      cloudflareDefaultRecordTTL,
		GSLB:     true,
		ScopeKey: defaultGSLBScopeKey,
	}
	if route == nil {
		return selection, errors.New("proxy route is nil")
	}
	selection.TTL = normalizeDNSTTL(route.DNSTTL)
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return selection, errors.New("GSLB scheduling only supports A/AAAA records")
	}
	policy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
	if err != nil {
		return selection, err
	}
	policy, err = normalizeGSLBPolicy(policy, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
	if err != nil {
		return selection, err
	}
	selection.TTL = normalizeDNSTTL(policy.TTL)
	selection.ScopeKey = gslbScopeKeyForPolicy(policy, source)

	candidates, err := buildGSLBDNSTargetCandidatesWithOptions(recordType, policy, source, options)
	if err != nil {
		return selection, err
	}
	desiredTargets := selectWeightedGSLBTargets(candidates, policy)
	if len(desiredTargets) == 0 {
		if options.RequireHealthyDNSProbe && gslbHasCandidatesWithoutDNSProbe(recordType, policy, source, options) {
			return selection, fmt.Errorf("Agent 探测未达到调度门槛，当前来源没有可用于 %s 记录的边缘节点", recordType)
		}
		return selection, fmt.Errorf("no online public node IP is available for %s records in GSLB pools", recordType)
	}

	now := time.Now()
	state, err := model.GetGSLBSchedulingState(route.ID, recordType, selection.ScopeKey)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return selection, err
	}
	previousTargets := decodeGSLBTargetList("")
	var previousChangedAt *time.Time
	if state != nil && state.ID != 0 {
		previousTargets = decodeGSLBTargetList(state.SelectedTargets)
		previousChangedAt = state.LastChangedAt
		selection.UnhealthyCount = normalizeDebounceCounter(state.UnhealthyCount)
		selection.RecoveryCount = normalizeDebounceCounter(state.RecoveryCount)
	}
	if len(previousTargets) == 0 {
		previousTargets = splitDNSRecordContent(route.DNSRecordContent)
	}
	if previousChangedAt == nil {
		previousChangedAt = route.DNSLastSyncedAt
	}

	eligibleTargets := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		eligibleTargets[candidate.Content] = struct{}{}
	}
	cooldown := time.Duration(policy.Debounce.CooldownSeconds) * time.Second
	unhealthyThreshold := normalizeDebounceThreshold(policy.Debounce.UnhealthyThreshold)
	recoveryThreshold := normalizeDebounceThreshold(policy.Debounce.RecoveryThreshold)
	selectedTargets := desiredTargets
	reason := "selected GSLB targets by " + policy.Strategy
	previousTargets, _ = normalizeDNSRecordContents(recordType, previousTargets)
	if !sameStringSet(previousTargets, desiredTargets) && len(previousTargets) > 0 {
		if allTargetsEligible(previousTargets, eligibleTargets) {
			selection.UnhealthyCount = 0
			selection.RecoveryCount++
			if selection.RecoveryCount < recoveryThreshold {
				selectedTargets = previousTargets
				reason = fmt.Sprintf("kept previous GSLB targets until recovery threshold %d/%d", selection.RecoveryCount, recoveryThreshold)
			} else if previousChangedAt != nil && now.Sub(*previousChangedAt) < cooldown {
				selectedTargets = previousTargets
				reason = fmt.Sprintf("kept previous GSLB targets during %ds cooldown", policy.Debounce.CooldownSeconds)
			}
		} else {
			selection.RecoveryCount = 0
			selection.UnhealthyCount++
			if selection.UnhealthyCount < unhealthyThreshold {
				selectedTargets = previousTargets
				reason = fmt.Sprintf("kept previous GSLB targets until unhealthy threshold %d/%d", selection.UnhealthyCount, unhealthyThreshold)
			}
		}
	} else {
		selection.UnhealthyCount = 0
		selection.RecoveryCount = 0
	}

	lastChangedAt := previousChangedAt
	if lastChangedAt == nil || !sameStringSet(previousTargets, selectedTargets) {
		lastChangedAt = &now
		selection.UnhealthyCount = 0
		selection.RecoveryCount = 0
	}
	selection.Targets = selectedTargets
	selection.DesiredTargets = desiredTargets
	selection.Reason = reason
	selection.LastChangedAt = lastChangedAt
	return selection, nil
}

const defaultGSLBScopeKey = "global"

func gslbScopeKeyFromSource(source GSLBSourceContext) string {
	if source.ASN > 0 {
		return fmt.Sprintf("asn:%d", source.ASN)
	}
	operator := normalizeGSLBOperator(source.Operator)
	if operator != "" {
		return "operator:" + operator
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		return "country:" + country
	}
	return defaultGSLBScopeKey
}

func gslbScopeKeyForPolicy(policy ProxyRouteGSLBPolicy, source GSLBSourceContext) string {
	for _, pool := range policy.Pools {
		if !pool.Enabled || gslbPoolExcludesSource(pool, source) {
			continue
		}
		if cidr, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			return "cidr:" + cidr
		}
	}
	if source.ASN > 0 {
		for _, pool := range policy.Pools {
			if !pool.Enabled || gslbPoolExcludesSource(pool, source) {
				continue
			}
			for _, asn := range pool.ASNs {
				if source.ASN == asn {
					return fmt.Sprintf("asn:%d", source.ASN)
				}
			}
		}
	}
	operator := normalizeGSLBOperator(source.Operator)
	if operator != "" {
		for _, pool := range policy.Pools {
			if !pool.Enabled || gslbPoolExcludesSource(pool, source) {
				continue
			}
			for _, poolOperator := range pool.Operators {
				if operator == normalizeGSLBOperator(poolOperator) {
					return "operator:" + operator
				}
			}
		}
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		return "country:" + country
	}
	return defaultGSLBScopeKey
}

func buildGSLBDNSTargetCandidates(recordType string, policy ProxyRouteGSLBPolicy, source GSLBSourceContext) ([]gslbDNSTargetCandidate, error) {
	return buildGSLBDNSTargetCandidatesWithOptions(recordType, policy, source, gslbDNSSchedulingOptions{})
}

func gslbHasCandidatesWithoutDNSProbe(recordType string, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions) bool {
	options.RequireHealthyDNSProbe = false
	candidates, err := buildGSLBDNSTargetCandidatesWithOptions(recordType, policy, source, options)
	return err == nil && len(candidates) > 0
}

func buildGSLBDNSTargetCandidatesWithOptions(recordType string, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions) ([]gslbDNSTargetCandidate, error) {
	nodes, err := gslbDNSSchedulingNodes(options)
	if err != nil {
		return nil, err
	}
	metrics := gslbDNSSchedulingMetricsByNode(options)
	probeStatsByNode := map[string]*dnsWorkerNodeProbeStats{}
	if options.RequireHealthyDNSProbe {
		probeStatsByNode = gslbDNSSchedulingProbeStatsByNode(options)
	}
	poolPolicies := matchGSLBPoolsForSourceWithMode(policy.Pools, source, policy.PoolMatchMode)
	candidates := buildGSLBDNSTargetCandidatesForPools(nodes, metrics, probeStatsByNode, recordType, policy, options, poolPolicies)
	if len(candidates) == 0 && normalizeGSLBSourcePoolFallbackMode(policy.SourcePoolFallbackMode) == gslbSourcePoolFallbackGlobal && gslbMatchedPoolsHaveSourceConditions(poolPolicies) {
		if fallbackCandidates := buildGSLBDNSTargetCandidatesForPools(nodes, metrics, probeStatsByNode, recordType, policy, options, gslbGlobalPoolsForFallback(policy.Pools, source)); len(fallbackCandidates) > 0 {
			return fallbackCandidates, nil
		}
	}
	return candidates, nil
}

func buildGSLBDNSTargetCandidatesForPools(nodes []*model.Node, metrics map[string]*model.NodeMetricSnapshot, probeStatsByNode map[string]*dnsWorkerNodeProbeStats, recordType string, policy ProxyRouteGSLBPolicy, options gslbDNSSchedulingOptions, poolPolicies map[string]ProxyRouteGSLBPoolPolicy) []gslbDNSTargetCandidate {
	candidates := make([]gslbDNSTargetCandidate, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		poolPolicy, ok := poolPolicies[normalizeNodePoolName(node.PoolName)]
		if !ok {
			continue
		}
		if !isNodeSchedulableForDNS(node) || !isNodeOnlineAndOpenRestyHealthy(node) {
			continue
		}
		if !gslbPoolAllowsNode(poolPolicy, node.NodeID) {
			continue
		}
		probeStats := probeStatsByNode[node.NodeID]
		if options.RequireHealthyDNSProbe && !dnsWorkerNodeProbeStatsSchedulable(probeStats) {
			continue
		}
		metric, hasMetric := metrics[node.NodeID]
		if hasMetric && !metricWithinGSLBThresholds(metric, policy.LoadThresholds) {
			continue
		}
		for _, value := range resolveNodePublicIPs(node) {
			ip := iputil.NormalizeIP(value)
			parsed := net.ParseIP(ip)
			if parsed == nil || !iputil.IsPublicString(ip) {
				continue
			}
			if recordType == "A" && parsed.To4() == nil {
				continue
			}
			if recordType == "AAAA" && parsed.To4() != nil {
				continue
			}
			content := parsed.String()
			if _, ok := seen[content]; ok {
				continue
			}
			seen[content] = struct{}{}
			candidate := gslbDNSTargetCandidate{
				NodeID:     node.NodeID,
				PoolName:   normalizeNodePoolName(node.PoolName),
				Content:    content,
				NodeWeight: normalizeNodeWeight(node.Weight),
				PoolWeight: poolPolicy.Weight,
				LastSeenAt: node.LastSeenAt,
				HasMetric:  hasMetric,
			}
			if options.RequireHealthyDNSProbe && probeStats != nil {
				candidate.DNSProbeHealthy = dnsWorkerNodeProbeStatsSchedulable(probeStats)
				candidate.DNSProbeCheckedCount = probeStats.totalCount
				candidate.DNSProbeHealthyCount = probeStats.healthyCount
				candidate.DNSProbeStaleCount = probeStats.staleCount
				candidate.DNSProbeAverageRTTMs = averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples)
			}
			if hasMetric {
				candidate.OpenrestyConnections = metric.OpenrestyConnections
				candidate.CPUUsagePercent = metric.CPUUsagePercent
				candidate.MemoryUsagePercent = nodeMetricMemoryUsagePercent(metric)
			}
			candidate.Score = scoreGSLBCandidate(candidate, policy.Strategy)
			candidates = append(candidates, candidate)
		}
	}
	sortGSLBCandidates(candidates, policy.Strategy)
	return candidates
}

func gslbMatchedPoolsHaveSourceConditions(pools map[string]ProxyRouteGSLBPoolPolicy) bool {
	for _, pool := range pools {
		if gslbPoolHasSourceConditions(pool) {
			return true
		}
	}
	return false
}

func gslbGlobalPoolsForFallback(pools []ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) map[string]ProxyRouteGSLBPoolPolicy {
	global := map[string]ProxyRouteGSLBPoolPolicy{}
	all := map[string]ProxyRouteGSLBPoolPolicy{}
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled || gslbPoolExcludesSource(pool, source) {
			continue
		}
		all[name] = pool
		if !gslbPoolHasSourceConditions(pool) {
			global[name] = pool
		}
	}
	if len(global) > 0 {
		return global
	}
	return all
}

func latestNodeMetricSnapshots() map[string]*model.NodeMetricSnapshot {
	freshness := time.Duration(common.GSLBMetricFreshnessSeconds) * time.Second
	if freshness <= 0 {
		freshness = 120 * time.Second
	}
	now := time.Now()
	rows, err := model.ListLatestMetricSnapshotsByNode(now.Add(-freshness), now)
	if err != nil {
		return map[string]*model.NodeMetricSnapshot{}
	}
	result := make(map[string]*model.NodeMetricSnapshot, len(rows))
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.NodeID) == "" {
			continue
		}
		if row.CapturedAt.After(now) {
			continue
		}
		if _, ok := result[row.NodeID]; ok {
			continue
		}
		result[row.NodeID] = row
	}
	return result
}

func matchGSLBPoolsForSource(pools []ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) map[string]ProxyRouteGSLBPoolPolicy {
	return matchGSLBPoolsForSourceWithMode(pools, source, gslbPoolMatchModePriority)
}

func matchGSLBPoolsForSourceWithMode(pools []ProxyRouteGSLBPoolPolicy, source GSLBSourceContext, mode string) map[string]ProxyRouteGSLBPoolPolicy {
	if normalizeGSLBPoolMatchMode(mode) == gslbPoolMatchModeMixed {
		return matchMixedGSLBPoolsForSource(pools, source)
	}
	return matchPriorityGSLBPoolsForSource(pools, source)
}

func matchMixedGSLBPoolsForSource(pools []ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) map[string]ProxyRouteGSLBPoolPolicy {
	result := make(map[string]ProxyRouteGSLBPoolPolicy, len(pools))
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled || gslbPoolExcludesSource(pool, source) {
			continue
		}
		if !gslbPoolHasSourceConditions(pool) || gslbPoolMatchesSource(pool, source) {
			result[name] = pool
		}
	}
	return result
}

func matchPriorityGSLBPoolsForSource(pools []ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) map[string]ProxyRouteGSLBPoolPolicy {
	result := make(map[string]ProxyRouteGSLBPoolPolicy, len(pools))
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	operator := normalizeGSLBOperator(source.Operator)
	matchedByCIDR := make(map[string]ProxyRouteGSLBPoolPolicy)
	matchedByASN := make(map[string]ProxyRouteGSLBPoolPolicy)
	matchedByOperator := make(map[string]ProxyRouteGSLBPoolPolicy)
	matchedByCountry := make(map[string]ProxyRouteGSLBPoolPolicy)
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled || gslbPoolExcludesSource(pool, source) {
			continue
		}
		result[name] = pool
		if _, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			matchedByCIDR[name] = pool
			continue
		}
		if source.ASN > 0 {
			for _, asn := range pool.ASNs {
				if source.ASN == asn {
					matchedByASN[name] = pool
					break
				}
			}
			if _, ok := matchedByASN[name]; ok {
				continue
			}
		}
		if operator != "" {
			for _, poolOperator := range pool.Operators {
				if operator == normalizeGSLBOperator(poolOperator) {
					matchedByOperator[name] = pool
					break
				}
			}
			if _, ok := matchedByOperator[name]; ok {
				continue
			}
		}
		if country == "" {
			continue
		}
		for _, poolCountry := range pool.Countries {
			if country == strings.ToUpper(strings.TrimSpace(poolCountry)) {
				matchedByCountry[name] = pool
				break
			}
		}
	}
	if len(matchedByCIDR) > 0 {
		return matchedByCIDR
	}
	if len(matchedByASN) > 0 {
		return matchedByASN
	}
	if len(matchedByOperator) > 0 {
		return matchedByOperator
	}
	if len(matchedByCountry) > 0 {
		return matchedByCountry
	}
	return result
}

func gslbPoolHasSourceConditions(pool ProxyRouteGSLBPoolPolicy) bool {
	return len(pool.SourceCIDRs) > 0 || len(pool.ASNs) > 0 || len(pool.Operators) > 0 || len(pool.Countries) > 0
}

func gslbPoolMatchesSource(pool ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) bool {
	if _, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
		return true
	}
	if source.ASN > 0 {
		for _, asn := range pool.ASNs {
			if source.ASN == asn {
				return true
			}
		}
	}
	operator := normalizeGSLBOperator(source.Operator)
	if operator != "" {
		for _, item := range pool.Operators {
			if operator == normalizeGSLBOperator(item) {
				return true
			}
		}
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		for _, item := range pool.Countries {
			if country == strings.ToUpper(strings.TrimSpace(item)) {
				return true
			}
		}
	}
	return false
}

func gslbPoolExcludesSource(pool ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) bool {
	if _, ok := sourceIPMatchesCIDRList(source.IP, pool.ExcludeSourceCIDRs); ok {
		return true
	}
	if source.ASN > 0 {
		for _, asn := range pool.ExcludeASNs {
			if source.ASN == asn {
				return true
			}
		}
	}
	operator := normalizeGSLBOperator(source.Operator)
	if operator != "" {
		for _, item := range pool.ExcludeOperators {
			if operator == normalizeGSLBOperator(item) {
				return true
			}
		}
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		for _, item := range pool.ExcludeCountries {
			if country == strings.ToUpper(strings.TrimSpace(item)) {
				return true
			}
		}
	}
	return false
}

func metricWithinGSLBThresholds(metric *model.NodeMetricSnapshot, thresholds ProxyRouteGSLBLoadThresholds) bool {
	if metric == nil {
		return true
	}
	if thresholds.MaxOpenrestyConnections > 0 && metric.OpenrestyConnections > thresholds.MaxOpenrestyConnections {
		return false
	}
	if thresholds.MaxCPUPercent > 0 && metric.CPUUsagePercent > thresholds.MaxCPUPercent {
		return false
	}
	memoryUsage := nodeMetricMemoryUsagePercent(metric)
	if thresholds.MaxMemoryPercent > 0 && memoryUsage > thresholds.MaxMemoryPercent {
		return false
	}
	return true
}

func nodeMetricMemoryUsagePercent(metric *model.NodeMetricSnapshot) float64 {
	if metric == nil || metric.MemoryTotalBytes <= 0 {
		return 0
	}
	return float64(metric.MemoryUsedBytes) / float64(metric.MemoryTotalBytes) * 100
}

func scoreGSLBCandidate(candidate gslbDNSTargetCandidate, strategy string) float64 {
	base := float64(candidate.NodeWeight * candidate.PoolWeight)
	if base <= 0 {
		base = 1
	}
	score := base
	if strategy == "load_aware" {
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
		score = base / penalty
	}
	return score * dnsProbeQualityFactorForGSLB(
		candidate.DNSProbeHealthy,
		candidate.DNSProbeCheckedCount,
		candidate.DNSProbeHealthyCount,
		candidate.DNSProbeStaleCount,
		candidate.DNSProbeAverageRTTMs,
	)
}

func dnsProbeQualityFactorForGSLB(healthy bool, checkedCount int, healthyCount int, staleCount int, averageRTTMs float64) float64 {
	if !healthy {
		return 1
	}
	factor := 1.0
	if checkedCount > 0 {
		healthyRatio := clampGSLBFloat(float64(healthyCount)/float64(checkedCount), 0, 1)
		staleRatio := clampGSLBFloat(float64(staleCount)/float64(checkedCount), 0, 1)
		factor *= 0.5 + 0.5*healthyRatio
		if staleRatio > 0.5 {
			staleRatio = 0.5
		}
		factor *= 1 - staleRatio*0.2
	}
	if averageRTTMs > 0 {
		factor *= clampGSLBFloat(200/(200+averageRTTMs), 0.25, 1)
	}
	return clampGSLBFloat(factor, 0.25, 1)
}

func clampGSLBFloat(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func sortGSLBCandidates(candidates []gslbDNSTargetCandidate, strategy string) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if strategy == "load_aware" && left.HasMetric != right.HasMetric {
			return left.HasMetric
		}
		if strategy == "weighted" || strategy == "load_aware" {
			if left.Score != right.Score {
				return left.Score > right.Score
			}
		}
		if strategy == "load_aware" && left.OpenrestyConnections != right.OpenrestyConnections {
			return left.OpenrestyConnections < right.OpenrestyConnections
		}
		if left.DNSProbeHealthy && right.DNSProbeHealthy &&
			left.DNSProbeAverageRTTMs > 0 && right.DNSProbeAverageRTTMs > 0 &&
			left.DNSProbeAverageRTTMs != right.DNSProbeAverageRTTMs {
			return left.DNSProbeAverageRTTMs < right.DNSProbeAverageRTTMs
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

func selectWeightedGSLBTargets(candidates []gslbDNSTargetCandidate, policy ProxyRouteGSLBPolicy) []string {
	targetCount := normalizeDNSTargetCount(policy.TargetCount)
	if len(candidates) == 0 {
		return []string{}
	}
	candidatesByPool := make(map[string][]gslbDNSTargetCandidate)
	for _, candidate := range candidates {
		candidatesByPool[candidate.PoolName] = append(candidatesByPool[candidate.PoolName], candidate)
	}
	quotas := allocateGSLBPoolQuotas(policy.Pools, targetCount, candidatesByPool)
	selected := make([]gslbDNSTargetCandidate, 0, targetCount)
	used := make(map[string]struct{}, targetCount)
	for _, pool := range policy.Pools {
		poolName := normalizeNodePoolName(pool.Name)
		poolCandidates := candidatesByPool[poolName]
		quota := quotas[poolName]
		for index := 0; index < len(poolCandidates) && quota > 0 && len(selected) < targetCount; index++ {
			candidate := poolCandidates[index]
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
	sortGSLBCandidates(selected, policy.Strategy)
	targets := make([]string, 0, len(selected))
	for _, candidate := range selected {
		targets = append(targets, candidate.Content)
	}
	return targets
}

func allocateGSLBPoolQuotas(pools []ProxyRouteGSLBPoolPolicy, targetCount int, candidatesByPool map[string][]gslbDNSTargetCandidate) map[string]int {
	type weightedPool struct {
		Name      string
		Weight    int
		Quota     int
		Remainder int
	}
	targetCount = normalizeDNSTargetCount(targetCount)
	weightedPools := make([]weightedPool, 0, len(pools))
	totalWeight := 0
	for _, pool := range pools {
		if !pool.Enabled {
			continue
		}
		weight := pool.Weight
		if weight <= 0 {
			weight = 100
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		if len(candidatesByPool[name]) == 0 {
			continue
		}
		totalWeight += weight
		weightedPools = append(weightedPools, weightedPool{Name: name, Weight: weight})
	}
	quotas := make(map[string]int, len(weightedPools))
	if len(weightedPools) == 0 || totalWeight <= 0 {
		return quotas
	}
	assigned := 0
	for index := range weightedPools {
		product := weightedPools[index].Weight * targetCount
		weightedPools[index].Quota = product / totalWeight
		weightedPools[index].Remainder = product % totalWeight
		assigned += weightedPools[index].Quota
	}
	sort.SliceStable(weightedPools, func(i, j int) bool {
		if weightedPools[i].Remainder != weightedPools[j].Remainder {
			return weightedPools[i].Remainder > weightedPools[j].Remainder
		}
		if weightedPools[i].Weight != weightedPools[j].Weight {
			return weightedPools[i].Weight > weightedPools[j].Weight
		}
		return weightedPools[i].Name < weightedPools[j].Name
	})
	for assigned < targetCount {
		for index := range weightedPools {
			if assigned >= targetCount {
				break
			}
			weightedPools[index].Quota++
			assigned++
		}
	}
	for _, pool := range weightedPools {
		quotas[pool.Name] = pool.Quota
	}
	return quotas
}

func allTargetsEligible(targets []string, eligibleTargets map[string]struct{}) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if _, ok := eligibleTargets[strings.TrimSpace(target)]; !ok {
			return false
		}
	}
	return true
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := make(map[string]int, len(left))
	for _, value := range left {
		leftSet[strings.TrimSpace(value)]++
	}
	for _, value := range right {
		key := strings.TrimSpace(value)
		if leftSet[key] == 0 {
			return false
		}
		leftSet[key]--
	}
	return true
}

func encodeGSLBTargetList(targets []string) string {
	raw, err := json.Marshal(targets)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeGSLBTargetList(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}
	}
	var targets []string
	if err := json.Unmarshal([]byte(text), &targets); err != nil {
		return []string{}
	}
	return normalizeStringList(targets)
}

func recordProxyRouteGSLBDecision(route *model.ProxyRoute, recordType string, selection proxyRouteDNSTargetSelection) error {
	if route == nil || route.ID == 0 || !selection.GSLB {
		return nil
	}
	now := time.Now()
	lastChangedAt := selection.LastChangedAt
	if lastChangedAt == nil {
		lastChangedAt = &now
	}
	state := model.GSLBSchedulingState{}
	scopeKey := strings.TrimSpace(selection.ScopeKey)
	if scopeKey == "" {
		scopeKey = defaultGSLBScopeKey
	}
	recordType = normalizeDNSRecordType(recordType)
	err := model.DB.Where("proxy_route_id = ? AND dns_record_type = ? AND scope_key = ?", route.ID, recordType, scopeKey).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = model.GSLBSchedulingState{
			ProxyRouteID:  route.ID,
			DNSRecordType: recordType,
			ScopeKey:      scopeKey,
			CreatedAt:     now,
		}
	} else if err != nil {
		return err
	}
	state.DNSRecordType = recordType
	state.ScopeKey = scopeKey
	state.SelectedTargets = encodeGSLBTargetList(selection.Targets)
	state.DesiredTargets = encodeGSLBTargetList(selection.DesiredTargets)
	state.UnhealthyCount = normalizeDebounceCounter(selection.UnhealthyCount)
	state.RecoveryCount = normalizeDebounceCounter(selection.RecoveryCount)
	state.LastReason = selection.Reason
	state.LastChangedAt = lastChangedAt
	state.LastEvaluatedAt = &now
	return model.DB.Save(&state).Error
}
