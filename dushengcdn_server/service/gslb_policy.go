package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"dushengcdn/model"
)

const (
	gslbModeCloudflareDNS        = "cloudflare_dns"
	gslbSourceProviderNone       = "none"
	gslbSourceProviderHTTP       = "http"
	gslbPoolMatchModePriority    = "priority"
	gslbPoolMatchModeMixed       = "mixed_weighted"
	gslbSourcePoolFallbackStrict = "strict"
	gslbSourcePoolFallbackGlobal = "fallback_to_global"
	defaultGSLBCooldownSeconds   = 60
	gslbCIDRParseCacheMax        = 4096
)

var gslbCIDRParseCache sync.Map

type gslbCIDRParseCacheEntry struct {
	cidr    string
	network *net.IPNet
}

type ProxyRouteGSLBPoolPolicy struct {
	Name               string   `json:"name"`
	Weight             int      `json:"weight"`
	Countries          []string `json:"countries"`
	SourceCIDRs        []string `json:"source_cidrs"`
	Operators          []string `json:"operators,omitempty"`
	ASNs               []uint32 `json:"asns,omitempty"`
	ExcludeCountries   []string `json:"exclude_countries,omitempty"`
	ExcludeSourceCIDRs []string `json:"exclude_source_cidrs,omitempty"`
	ExcludeOperators   []string `json:"exclude_operators,omitempty"`
	ExcludeASNs        []uint32 `json:"exclude_asns,omitempty"`
	NodeIDs            []string `json:"node_ids,omitempty"`
	Enabled            bool     `json:"enabled"`
}

type ProxyRouteGSLBSourceIPProvider struct {
	Provider string `json:"provider"`
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
}

type ProxyRouteGSLBLoadThresholds struct {
	MaxOpenrestyConnections int64   `json:"max_openresty_connections"`
	MaxCPUPercent           float64 `json:"max_cpu_percent"`
	MaxMemoryPercent        float64 `json:"max_memory_percent"`
}

type ProxyRouteGSLBDebounce struct {
	CooldownSeconds    int `json:"cooldown_seconds"`
	UnhealthyThreshold int `json:"unhealthy_threshold"`
	RecoveryThreshold  int `json:"recovery_threshold"`
}

type ProxyRouteGSLBPolicy struct {
	Mode                   string                         `json:"mode"`
	Strategy               string                         `json:"strategy"`
	PoolMatchMode          string                         `json:"pool_match_mode"`
	Pools                  []ProxyRouteGSLBPoolPolicy     `json:"pools"`
	TargetCount            int                            `json:"target_count"`
	TTL                    int                            `json:"ttl"`
	SourceIP               ProxyRouteGSLBSourceIPProvider `json:"source_ip"`
	SourcePoolFallbackMode string                         `json:"source_pool_fallback_mode"`
	LoadThresholds         ProxyRouteGSLBLoadThresholds   `json:"load_thresholds"`
	Debounce               ProxyRouteGSLBDebounce         `json:"debounce"`
}

func defaultGSLBPolicy(nodePool string, targetCount int, scheduleMode string, ttl int) ProxyRouteGSLBPolicy {
	return ProxyRouteGSLBPolicy{
		Mode:          gslbModeCloudflareDNS,
		Strategy:      normalizeDNSScheduleMode(scheduleMode),
		PoolMatchMode: gslbPoolMatchModePriority,
		TargetCount:   normalizeDNSTargetCount(targetCount),
		TTL:           normalizeDNSTTL(ttl),
		Pools: []ProxyRouteGSLBPoolPolicy{
			{
				Name:               normalizeNodePoolName(nodePool),
				Weight:             100,
				Countries:          []string{},
				SourceCIDRs:        []string{},
				Operators:          []string{},
				ASNs:               []uint32{},
				ExcludeCountries:   []string{},
				ExcludeSourceCIDRs: []string{},
				ExcludeOperators:   []string{},
				ExcludeASNs:        []uint32{},
				Enabled:            true,
			},
		},
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: gslbSourceProviderNone,
		},
		SourcePoolFallbackMode: gslbSourcePoolFallbackStrict,
		Debounce: ProxyRouteGSLBDebounce{
			CooldownSeconds:    defaultGSLBCooldownSeconds,
			UnhealthyThreshold: 1,
			RecoveryThreshold:  1,
		},
	}
}

func normalizeGSLBPolicy(input ProxyRouteGSLBPolicy, nodePool string, targetCount int, scheduleMode string, ttl int) (ProxyRouteGSLBPolicy, error) {
	defaultPolicy := defaultGSLBPolicy(nodePool, targetCount, scheduleMode, ttl)
	if len(input.Pools) == 0 &&
		strings.TrimSpace(input.Mode) == "" &&
		strings.TrimSpace(input.Strategy) == "" &&
		input.TargetCount == 0 &&
		input.TTL == 0 &&
		strings.TrimSpace(input.SourceIP.Provider) == "" &&
		strings.TrimSpace(input.SourceIP.APIURL) == "" &&
		strings.TrimSpace(input.SourceIP.APIToken) == "" &&
		strings.TrimSpace(input.PoolMatchMode) == "" &&
		strings.TrimSpace(input.SourcePoolFallbackMode) == "" {
		return defaultPolicy, nil
	}

	policy := defaultPolicy
	if strings.TrimSpace(input.Mode) != "" {
		mode := strings.ToLower(strings.TrimSpace(input.Mode))
		switch mode {
		case gslbModeCloudflareDNS:
			policy.Mode = mode
		default:
			return policy, errors.New("gslb_policy.mode currently only supports cloudflare_dns")
		}
	}
	if strings.TrimSpace(input.Strategy) != "" {
		policy.Strategy = normalizeDNSScheduleMode(input.Strategy)
	}
	if input.TargetCount > 0 {
		policy.TargetCount = normalizeDNSTargetCount(input.TargetCount)
	}
	if input.TTL > 0 {
		policy.TTL = normalizeDNSTTL(input.TTL)
	}
	policy.PoolMatchMode = normalizeGSLBPoolMatchMode(input.PoolMatchMode)

	sourceProvider := strings.ToLower(strings.TrimSpace(input.SourceIP.Provider))
	sourceAPIURL := strings.TrimSpace(input.SourceIP.APIURL)
	sourceAPIToken := strings.TrimSpace(input.SourceIP.APIToken)
	switch sourceProvider {
	case "":
		if sourceAPIURL != "" || sourceAPIToken != "" {
			return policy, errors.New("gslb_policy.source_ip.provider is required when api_url or api_token is set")
		}
	case gslbSourceProviderNone:
		if sourceAPIURL != "" || sourceAPIToken != "" {
			return policy, errors.New("gslb_policy.source_ip.provider=none cannot set api_url or api_token")
		}
		policy.SourceIP.Provider = sourceProvider
	case gslbSourceProviderHTTP:
		return policy, errors.New("gslb_policy.source_ip.provider=http is not supported by authoritative DNS workers")
	default:
		return policy, errors.New("gslb_policy.source_ip.provider is not supported")
	}

	policy.LoadThresholds = normalizeGSLBLoadThresholds(input.LoadThresholds)
	policy.SourcePoolFallbackMode = normalizeGSLBSourcePoolFallbackMode(input.SourcePoolFallbackMode)
	policy.Debounce = normalizeGSLBDebounce(input.Debounce)

	if len(input.Pools) > 0 {
		pools, err := normalizeGSLBPools(input.Pools)
		if err != nil {
			return policy, err
		}
		policy.Pools = pools
	}
	return policy, nil
}

func normalizeGSLBPoolMatchMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case gslbPoolMatchModeMixed:
		return gslbPoolMatchModeMixed
	default:
		return gslbPoolMatchModePriority
	}
}

func normalizeGSLBSourcePoolFallbackMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case gslbSourcePoolFallbackGlobal:
		return gslbSourcePoolFallbackGlobal
	default:
		return gslbSourcePoolFallbackStrict
	}
}

func normalizeGSLBPools(input []ProxyRouteGSLBPoolPolicy) ([]ProxyRouteGSLBPoolPolicy, error) {
	result := make([]ProxyRouteGSLBPoolPolicy, 0, len(input))
	seen := make(map[string]int, len(input))
	hasExplicitEnabledPool := false
	for _, pool := range input {
		if pool.Enabled {
			hasExplicitEnabledPool = true
			break
		}
	}
	for _, pool := range input {
		if hasExplicitEnabledPool && !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		weight := pool.Weight
		if weight <= 0 {
			weight = 100
		}
		if weight > 1000 {
			weight = 1000
		}
		countries := normalizeGSLBCountryList(pool.Countries)
		sourceCIDRs, err := normalizeGSLBCIDRList(pool.SourceCIDRs)
		if err != nil {
			return nil, err
		}
		operators := normalizeGSLBOperatorList(pool.Operators)
		asns := normalizeGSLBASNList(pool.ASNs)
		excludeCountries := normalizeGSLBCountryList(pool.ExcludeCountries)
		excludeSourceCIDRs, err := normalizeGSLBCIDRListField(pool.ExcludeSourceCIDRs, "gslb_policy.pools.exclude_source_cidrs")
		if err != nil {
			return nil, err
		}
		excludeOperators := normalizeGSLBOperatorList(pool.ExcludeOperators)
		excludeASNs := normalizeGSLBASNList(pool.ExcludeASNs)
		nodeIDs := normalizeGSLBNodeIDList(pool.NodeIDs)
		if existingIndex, ok := seen[name]; ok {
			result[existingIndex].Weight = weight
			result[existingIndex].Countries = mergeGSLBStringLists(result[existingIndex].Countries, countries)
			result[existingIndex].SourceCIDRs = mergeGSLBStringLists(result[existingIndex].SourceCIDRs, sourceCIDRs)
			result[existingIndex].Operators = mergeGSLBStringLists(result[existingIndex].Operators, operators)
			result[existingIndex].ASNs = mergeGSLBASNLists(result[existingIndex].ASNs, asns)
			result[existingIndex].ExcludeCountries = mergeGSLBStringLists(result[existingIndex].ExcludeCountries, excludeCountries)
			result[existingIndex].ExcludeSourceCIDRs = mergeGSLBStringLists(result[existingIndex].ExcludeSourceCIDRs, excludeSourceCIDRs)
			result[existingIndex].ExcludeOperators = mergeGSLBStringLists(result[existingIndex].ExcludeOperators, excludeOperators)
			result[existingIndex].ExcludeASNs = mergeGSLBASNLists(result[existingIndex].ExcludeASNs, excludeASNs)
			if len(result[existingIndex].NodeIDs) == 0 || len(nodeIDs) == 0 {
				result[existingIndex].NodeIDs = nil
			} else {
				result[existingIndex].NodeIDs = mergeGSLBStringLists(result[existingIndex].NodeIDs, nodeIDs)
			}
			continue
		}
		seen[name] = len(result)
		result = append(result, ProxyRouteGSLBPoolPolicy{
			Name:               name,
			Weight:             weight,
			Countries:          countries,
			SourceCIDRs:        sourceCIDRs,
			Operators:          operators,
			ASNs:               asns,
			ExcludeCountries:   excludeCountries,
			ExcludeSourceCIDRs: excludeSourceCIDRs,
			ExcludeOperators:   excludeOperators,
			ExcludeASNs:        excludeASNs,
			NodeIDs:            nodeIDs,
			Enabled:            true,
		})
	}
	if len(result) == 0 {
		return nil, errors.New("gslb_policy.pools requires at least one enabled node pool")
	}
	return result, nil
}

func validateGSLBPolicyPoolTargets(policy ProxyRouteGSLBPolicy, recordType string) error {
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return errors.New("GSLB scheduling only supports A/AAAA records")
	}
	if len(policy.Pools) == 0 {
		return errors.New("gslb_policy.pools requires at least one enabled node pool")
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return err
	}
	poolPolicies := make(map[string]ProxyRouteGSLBPoolPolicy, len(policy.Pools))
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name != "" {
			poolPolicies[name] = pool
		}
	}
	availablePools := make(map[string]int, len(policy.Pools))
	for _, node := range nodes {
		if !isNodeSchedulableForDNS(node) || !isNodeOnlineAndOpenRestyHealthy(node) {
			continue
		}
		poolName := normalizeNodePoolName(node.PoolName)
		poolPolicy, ok := poolPolicies[poolName]
		if !ok || !gslbPoolAllowsNode(poolPolicy, node.NodeID) {
			continue
		}
		for range nodeDNSContents(node, recordType) {
			availablePools[poolName]++
		}
	}
	missingPools := make([]string, 0)
	seen := map[string]struct{}{}
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if availablePools[name] == 0 {
			missingPools = append(missingPools, name)
		}
	}
	if len(missingPools) > 0 {
		return fmt.Errorf("多节点智能解析节点池没有可用于 %s 记录的在线公网 IP：%s。请确认填写的是节点池名称，不是节点名称", recordType, strings.Join(missingPools, "、"))
	}
	return nil
}

func normalizeGSLBNodeIDList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		nodeID := strings.TrimSpace(value)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		result = append(result, nodeID)
	}
	return result
}

func gslbPoolAllowsNode(pool ProxyRouteGSLBPoolPolicy, nodeID string) bool {
	if len(pool.NodeIDs) == 0 {
		return true
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false
	}
	for _, allowedNodeID := range pool.NodeIDs {
		if strings.TrimSpace(allowedNodeID) == nodeID {
			return true
		}
	}
	return false
}

func normalizeGSLBCountryList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		country := strings.ToUpper(strings.TrimSpace(value))
		if country == "" || !proxyRouteRegionCountryPattern.MatchString(country) {
			continue
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		result = append(result, country)
	}
	return result
}

func normalizeGSLBOperatorList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		operator := normalizeGSLBOperator(value)
		if operator == "" {
			continue
		}
		if _, ok := seen[operator]; ok {
			continue
		}
		seen[operator] = struct{}{}
		result = append(result, operator)
	}
	return result
}

func normalizeGSLBOperator(value string) string {
	operator := strings.ToLower(strings.TrimSpace(value))
	operator = strings.ReplaceAll(operator, "_", "-")
	operator = strings.ReplaceAll(operator, " ", "-")
	switch operator {
	case "telecom", "china-telecom", "ct", "cn-telecom", "chinatelecom", "中国电信", "电信":
		return "cn-telecom"
	case "unicom", "china-unicom", "cu", "cn-unicom", "chinaunicom", "中国联通", "联通":
		return "cn-unicom"
	case "mobile", "china-mobile", "cmcc", "cn-mobile", "chinamobile", "中国移动", "移动":
		return "cn-mobile"
	case "broadcast", "cbn", "china-broadcast", "cn-broadcast", "广电", "中国广电":
		return "cn-broadcast"
	case "cernet", "edu", "education", "教育网", "中国教育网":
		return "cernet"
	default:
		if len(operator) > 64 {
			operator = operator[:64]
		}
		return operator
	}
}

func normalizeGSLBASNList(values []uint32) []uint32 {
	result := make([]uint32, 0, len(values))
	seen := make(map[uint32]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeGSLBCIDRList(values []string) ([]string, error) {
	return normalizeGSLBCIDRListField(values, "gslb_policy.pools.source_cidrs")
}

func normalizeGSLBCIDRListField(values []string, field string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		cidr, ok := normalizeGSLBCIDR(value)
		if !ok {
			if strings.TrimSpace(value) == "" {
				continue
			}
			return nil, fmt.Errorf("%s contains invalid CIDR", field)
		}
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, cidr)
	}
	return result, nil
}

func normalizeGSLBCIDR(value string) (string, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", false
	}
	if strings.Contains(text, "/") {
		ip, network, err := net.ParseCIDR(text)
		if err != nil {
			return "", false
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			ip = ipv4
		}
		network.IP = ip.Mask(network.Mask)
		return network.String(), true
	}
	ip := net.ParseIP(text)
	if ip == nil {
		return "", false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		network := net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)}
		return network.String(), true
	}
	network := net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
	return network.String(), true
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
		// CIDR lists are normalized at write time (normalizeGSLBCIDRList), so the
		// matching path only needs a single parse; plain-IP legacy values without
		// a prefix length still go through normalizeGSLBCIDR first.
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		cidr, network, ok := parseGSLBCIDRForMatch(text)
		if !ok {
			continue
		}
		if network.Contains(ip) {
			return cidr, true
		}
	}
	return "", false
}

func parseGSLBCIDRForMatch(value string) (string, *net.IPNet, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return "", nil, false
	}
	if !strings.Contains(text, "/") {
		normalized, ok := normalizeGSLBCIDR(text)
		if !ok {
			return "", nil, false
		}
		text = normalized
	}
	if cached, ok := gslbCIDRParseCache.Load(text); ok {
		entry, ok := cached.(gslbCIDRParseCacheEntry)
		if ok && entry.network != nil {
			return entry.cidr, entry.network, true
		}
	}
	_, network, err := net.ParseCIDR(text)
	if err != nil {
		return "", nil, false
	}
	cidr := network.String()
	if approximateSyncMapLen(&gslbCIDRParseCache, gslbCIDRParseCacheMax+1) >= gslbCIDRParseCacheMax {
		gslbCIDRParseCache.Range(func(key, _ any) bool {
			gslbCIDRParseCache.Delete(key)
			return false
		})
	}
	gslbCIDRParseCache.Store(text, gslbCIDRParseCacheEntry{cidr: cidr, network: network})
	return cidr, network, true
}

func approximateSyncMapLen(values *sync.Map, limit int) int {
	if values == nil || limit <= 0 {
		return 0
	}
	count := 0
	values.Range(func(_, _ any) bool {
		count++
		return count < limit
	})
	return count
}

func mergeGSLBStringLists(left []string, right []string) []string {
	result := append([]string(nil), left...)
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mergeGSLBASNLists(left []uint32, right []uint32) []uint32 {
	result := append([]uint32(nil), left...)
	seen := make(map[uint32]struct{}, len(left)+len(right))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeGSLBLoadThresholds(input ProxyRouteGSLBLoadThresholds) ProxyRouteGSLBLoadThresholds {
	thresholds := input
	if thresholds.MaxOpenrestyConnections < 0 {
		thresholds.MaxOpenrestyConnections = 0
	}
	if thresholds.MaxCPUPercent < 0 {
		thresholds.MaxCPUPercent = 0
	}
	if thresholds.MaxCPUPercent > 100 {
		thresholds.MaxCPUPercent = 100
	}
	if thresholds.MaxMemoryPercent < 0 {
		thresholds.MaxMemoryPercent = 0
	}
	if thresholds.MaxMemoryPercent > 100 {
		thresholds.MaxMemoryPercent = 100
	}
	return thresholds
}

func normalizeGSLBDebounce(input ProxyRouteGSLBDebounce) ProxyRouteGSLBDebounce {
	debounce := input
	if debounce.CooldownSeconds <= 0 {
		debounce.CooldownSeconds = defaultGSLBCooldownSeconds
	}
	if debounce.CooldownSeconds > 3600 {
		debounce.CooldownSeconds = 3600
	}
	if debounce.UnhealthyThreshold <= 0 {
		debounce.UnhealthyThreshold = 1
	}
	if debounce.RecoveryThreshold <= 0 {
		debounce.RecoveryThreshold = 1
	}
	return debounce
}

func normalizeDebounceThreshold(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func normalizeDebounceCounter(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func decodeStoredGSLBPolicy(raw string) (ProxyRouteGSLBPolicy, error) {
	text := strings.TrimSpace(raw)
	if text == "" || text == "{}" {
		return defaultGSLBPolicy("default", 1, "healthy", cloudflareDefaultRecordTTL), nil
	}
	var policy ProxyRouteGSLBPolicy
	if err := json.Unmarshal([]byte(text), &policy); err != nil {
		return policy, errors.New("gslb_policy payload is invalid")
	}
	policy = downgradeUnsupportedStoredGSLBPolicy(policy)
	return normalizeGSLBPolicy(policy, "default", policy.TargetCount, policy.Strategy, policy.TTL)
}

func downgradeUnsupportedStoredGSLBPolicy(policy ProxyRouteGSLBPolicy) ProxyRouteGSLBPolicy {
	provider := strings.ToLower(strings.TrimSpace(policy.SourceIP.Provider))
	if provider == "" || provider == gslbSourceProviderNone || provider == gslbSourceProviderHTTP {
		policy.SourceIP = ProxyRouteGSLBSourceIPProvider{
			Provider: gslbSourceProviderNone,
		}
	}
	return policy
}
