package service

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
)

func SimulateAuthoritativeDNSGSLB(input DNSGSLBSimulationInput) (*DNSGSLBSimulationView, error) {
	if input.ProxyRouteID == 0 {
		return nil, errors.New("proxy_route_id is required")
	}
	recordType := strings.ToUpper(strings.TrimSpace(input.RecordType))
	if recordType == "" {
		recordType = "A"
	}
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("record_type only supports A/AAAA")
	}
	sourceIP := strings.TrimSpace(input.SourceIP)
	if sourceIP != "" && net.ParseIP(sourceIP) == nil {
		return nil, errors.New("source_ip format is invalid")
	}
	country := strings.ToUpper(strings.TrimSpace(input.Country))
	if country != "" && !proxyRouteRegionCountryPattern.MatchString(country) {
		return nil, errors.New("country must be a two-letter country code")
	}
	operator := normalizeGSLBOperator(input.Operator)
	asn := input.ASN

	routeModel, err := model.GetProxyRouteByID(input.ProxyRouteID)
	if err != nil {
		return nil, err
	}
	if routeModel == nil || !routeModel.Enabled {
		return nil, errors.New("selected proxy route is not enabled")
	}
	if routeModel.DNSProviderMode != DNSProviderModeAuthoritative {
		return nil, errors.New("selected proxy route is not using authoritative DNS")
	}

	snapshot, err := GetAuthoritativeDNSSnapshot(nil)
	if err != nil {
		return nil, err
	}
	workerSnapshot := convertAuthoritativeSnapshotToWorker(snapshot)
	var workerRoute *dnsworker.SnapshotRoute
	for index := range workerSnapshot.Routes {
		if workerSnapshot.Routes[index].ID == input.ProxyRouteID {
			workerRoute = &workerSnapshot.Routes[index]
			break
		}
	}
	if workerRoute == nil {
		return nil, errors.New("selected proxy route is not present in authoritative DNS snapshot")
	}

	qname := normalizeDNSRecordName(input.QName)
	if qname == "" {
		domains, err := decodeStoredDomains(routeModel.Domains, routeModel.Domain)
		if err != nil {
			return nil, err
		}
		if len(domains) > 0 {
			qname = normalizeDNSRecordName(domains[0])
		}
	}
	if qname == "" {
		return nil, errors.New("qname is required")
	}
	if !authoritativeRouteHasDomain(workerRoute, qname) {
		return nil, errors.New("qname does not belong to selected proxy route")
	}

	fresh := true
	if input.Fresh != nil {
		fresh = *input.Fresh
	}
	source := dnsworker.SourceContext{
		IP:       sourceIP,
		Country:  country,
		Operator: operator,
		ASN:      asn,
	}
	scheduler := dnsworker.NewScheduler()
	scheduler.LoadSnapshotStates(workerSnapshot)
	targets, ttl, sourceScope, err := scheduler.Select(workerSnapshot, workerRoute, recordType, source, fresh)
	if err != nil {
		if errors.Is(err, dnsworker.ErrDNSProbeThresholdNotSatisfied) {
			return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, operator, asn, nil, ttl, sourceScope, "Agent 探测未达到调度门槛，当前来源没有可用于 "+recordType+" 记录的边缘节点。请查看下方节点原因确认是未探测、探测过期还是 UDP/TCP 53 未同时可达。"), nil
		}
		if errors.Is(err, dnsworker.ErrNoAvailableTarget) || errors.Is(err, dnsworker.ErrNoTargetSelected) {
			return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, operator, asn, nil, ttl, sourceScope, "当前来源没有可用于 "+recordType+" 记录的边缘节点。请查看下方节点原因确认节点池、在线状态、OpenResty 健康、公网 IP 类型和负载阈值。"), nil
		}
		return nil, err
	}

	return buildDNSGSLBSimulationView(snapshot, workerRoute, qname, recordType, sourceIP, country, operator, asn, targets, ttl, sourceScope, ""), nil
}

func buildDNSGSLBSimulationView(snapshot *AuthoritativeDNSSnapshot, workerRoute *dnsworker.SnapshotRoute, qname string, recordType string, sourceIP string, country string, operator string, asn uint32, targets []string, ttl int, sourceScope string, messagePrefix string) *DNSGSLBSimulationView {
	if targets == nil {
		targets = []string{}
	}
	policy := workerRoute.GSLBPolicy
	if !workerRoute.GSLBEnabled {
		policy.Strategy = workerRoute.ScheduleMode
		policy.TargetCount = workerRoute.TargetCount
		policy.Pools = []dnsworker.GSLBPoolPolicy{
			{
				Name:    normalizeNodePoolName(workerRoute.NodePool),
				Weight:  100,
				Enabled: true,
			},
		}
	}
	diagnostics := buildDNSGSLBSimulationDiagnostics(recordType, policy, GSLBSourceContext{IP: sourceIP, Country: country, Operator: operator, ASN: asn}, targets, snapshot.GSLBProbeSchedulingEnabled)
	message := "模拟结果来自当前面板生成的解析配置，不会写入真实切换状态。"
	if strings.TrimSpace(messagePrefix) != "" {
		message = strings.TrimSpace(messagePrefix) + " " + message
	}
	if sourceScope == defaultGSLBScopeKey && country == "" && operator == "" && asn == 0 {
		message += " 未指定来源条件时使用全局作用域。"
	}
	return &DNSGSLBSimulationView{
		ProxyRouteID:    workerRoute.ID,
		SiteName:        workerRoute.SiteName,
		QName:           qname,
		RecordType:      recordType,
		Country:         country,
		SourceIP:        sourceIP,
		Operator:        operator,
		ASN:             asn,
		SourceScope:     sourceScope,
		TTL:             ttl,
		Targets:         targets,
		TargetCount:     len(targets),
		Strategy:        strings.TrimSpace(policy.Strategy),
		GSLBEnabled:     workerRoute.GSLBEnabled,
		SnapshotVersion: snapshot.SnapshotVersion,
		SnapshotAt:      snapshot.GeneratedAt,
		Message:         message,
		MatchedPools:    diagnostics.pools,
		Nodes:           diagnostics.nodes,
	}
}

func authoritativeRouteHasDomain(route *dnsworker.SnapshotRoute, qname string) bool {
	if route == nil {
		return false
	}
	name := normalizeDNSRecordName(qname)
	for _, domain := range route.Domains {
		if authoritativeDomainMatchesQName(normalizeDNSRecordName(domain), name) {
			return true
		}
	}
	return false
}

func authoritativeDomainMatchesQName(domain string, qname string) bool {
	domain = normalizeDNSRecordName(domain)
	qname = normalizeDNSRecordName(qname)
	if domain == "" || qname == "" {
		return false
	}
	if domain == qname {
		return true
	}
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	base := strings.TrimPrefix(domain, "*.")
	if base == "" || qname == base || !strings.HasSuffix(qname, "."+base) {
		return false
	}
	leftmostLabel := strings.TrimSuffix(qname, "."+base)
	return leftmostLabel != "" && !strings.Contains(leftmostLabel, ".")
}

type dnsGSLBSimulationDiagnostics struct {
	pools []DNSGSLBSimulationPoolView
	nodes []DNSGSLBSimulationNodeView
}

func buildDNSGSLBSimulationDiagnostics(recordType string, policy dnsworker.GSLBPolicy, source GSLBSourceContext, selectedTargets []string, requireHealthyDNSProbe bool) dnsGSLBSimulationDiagnostics {
	return buildDNSGSLBSimulationDiagnosticsWithOptions(recordType, policy, source, selectedTargets, gslbDNSSchedulingOptions{
		RequireHealthyDNSProbe: requireHealthyDNSProbe,
	})
}

func buildDNSGSLBSimulationDiagnosticsWithOptions(recordType string, policy dnsworker.GSLBPolicy, source GSLBSourceContext, selectedTargets []string, options gslbDNSSchedulingOptions) dnsGSLBSimulationDiagnostics {
	servicePolicy := convertWorkerGSLBPolicyToAuthoritative(policy)
	servicePolicy, err := normalizeGSLBPolicy(servicePolicy, "default", servicePolicy.TargetCount, servicePolicy.Strategy, servicePolicy.TTL)
	if err != nil {
		return dnsGSLBSimulationDiagnostics{}
	}
	matchedPools := matchGSLBPoolsForSourceWithMode(servicePolicy.Pools, source, servicePolicy.PoolMatchMode)
	diagnostics := dnsGSLBSimulationDiagnostics{
		pools: buildDNSGSLBSimulationPoolViews(servicePolicy.Pools, matchedPools, source),
		nodes: []DNSGSLBSimulationNodeView{},
	}
	nodes, err := gslbDNSSchedulingNodes(options)
	if err != nil {
		return diagnostics
	}
	metrics := gslbDNSSchedulingMetricsByNode(options)
	nodeProbeStats := gslbDNSSchedulingProbeStatsByNode(options)
	selectedSet := make(map[string]struct{}, len(selectedTargets))
	for _, target := range selectedTargets {
		selectedSet[strings.TrimSpace(target)] = struct{}{}
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		view := buildDNSGSLBSimulationNodeView(node, recordType, servicePolicy, matchedPools, metrics[node.NodeID], selectedSet, nodeProbeStats[node.NodeID], options.RequireHealthyDNSProbe)
		diagnostics.nodes = append(diagnostics.nodes, view)
	}
	sort.SliceStable(diagnostics.nodes, func(i, j int) bool {
		left := diagnostics.nodes[i]
		right := diagnostics.nodes[j]
		if left.Selected != right.Selected {
			return left.Selected
		}
		if left.Eligible != right.Eligible {
			return left.Eligible
		}
		if left.PoolName != right.PoolName {
			return left.PoolName < right.PoolName
		}
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.NodeID < right.NodeID
	})
	return diagnostics
}

func buildDNSGSLBSimulationPoolViews(pools []ProxyRouteGSLBPoolPolicy, matchedPools map[string]ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) []DNSGSLBSimulationPoolView {
	result := make([]DNSGSLBSimulationPoolView, 0, len(pools))
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	operator := normalizeGSLBOperator(source.Operator)
	cidrMatched := false
	asnMatched := false
	operatorMatched := false
	for _, pool := range pools {
		if _, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			cidrMatched = true
			break
		}
	}
	if !cidrMatched && source.ASN > 0 {
		for _, pool := range pools {
			if gslbPoolMatchesASN(pool, source.ASN) {
				asnMatched = true
				break
			}
		}
	}
	if !cidrMatched && !asnMatched && operator != "" {
		for _, pool := range pools {
			if gslbPoolMatchesOperator(pool, operator) {
				operatorMatched = true
				break
			}
		}
	}
	for _, pool := range pools {
		name := normalizeNodePoolName(pool.Name)
		if name == "" || !pool.Enabled {
			continue
		}
		_, matched := matchedPools[name]
		reason := "参与全局调度"
		if excluded, excludeReason := gslbPoolExclusionReason(pool, source); excluded {
			reason = excludeReason
		} else if matchedCIDR, ok := sourceIPMatchesCIDRList(source.IP, pool.SourceCIDRs); ok {
			reason = "匹配来源网段 " + matchedCIDR
		} else if cidrMatched {
			reason = "未匹配来源网段"
		} else if source.ASN > 0 && asnMatched {
			if gslbPoolMatchesASN(pool, source.ASN) {
				reason = fmt.Sprintf("匹配来源 ASN %d", source.ASN)
			} else {
				reason = "未匹配来源 ASN"
			}
		} else if operator != "" && operatorMatched {
			if gslbPoolMatchesOperator(pool, operator) {
				reason = "匹配来源运营商 " + operator
			} else {
				reason = "未匹配来源运营商"
			}
		} else if country != "" {
			reason = "未匹配来源国家"
			for _, poolCountry := range pool.Countries {
				if country == strings.ToUpper(strings.TrimSpace(poolCountry)) {
					reason = "匹配来源国家 " + country
					break
				}
			}
			if len(matchedPools) == len(enabledGSLBPoolNames(pools)) && reason == "未匹配来源国家" {
				reason = "未命中国家专属池，回退参与调度"
			}
		}
		result = append(result, DNSGSLBSimulationPoolView{
			Name:               name,
			Weight:             pool.Weight,
			Countries:          append([]string(nil), pool.Countries...),
			SourceCIDRs:        append([]string(nil), pool.SourceCIDRs...),
			Operators:          append([]string(nil), pool.Operators...),
			ASNs:               append([]uint32(nil), pool.ASNs...),
			ExcludeCountries:   append([]string(nil), pool.ExcludeCountries...),
			ExcludeSourceCIDRs: append([]string(nil), pool.ExcludeSourceCIDRs...),
			ExcludeOperators:   append([]string(nil), pool.ExcludeOperators...),
			ExcludeASNs:        append([]uint32(nil), pool.ExcludeASNs...),
			Matched:            matched,
			Reason:             reason,
		})
	}
	return result
}

func gslbPoolExclusionReason(pool ProxyRouteGSLBPoolPolicy, source GSLBSourceContext) (bool, string) {
	if cidr, ok := sourceIPMatchesCIDRList(source.IP, pool.ExcludeSourceCIDRs); ok {
		return true, "被排除来源网段 " + cidr + " 命中"
	}
	if source.ASN > 0 {
		for _, asn := range pool.ExcludeASNs {
			if source.ASN == asn {
				return true, fmt.Sprintf("被排除 ASN %d 命中", source.ASN)
			}
		}
	}
	operator := normalizeGSLBOperator(source.Operator)
	if operator != "" {
		for _, item := range pool.ExcludeOperators {
			if operator == normalizeGSLBOperator(item) {
				return true, "被排除运营商 " + operator + " 命中"
			}
		}
	}
	country := strings.ToUpper(strings.TrimSpace(source.Country))
	if country != "" {
		for _, item := range pool.ExcludeCountries {
			if country == strings.ToUpper(strings.TrimSpace(item)) {
				return true, "被排除国家 " + country + " 命中"
			}
		}
	}
	return false, ""
}

func gslbPoolMatchesASN(pool ProxyRouteGSLBPoolPolicy, asn uint32) bool {
	if asn == 0 {
		return false
	}
	for _, poolASN := range pool.ASNs {
		if asn == poolASN {
			return true
		}
	}
	return false
}

func gslbPoolMatchesOperator(pool ProxyRouteGSLBPoolPolicy, operator string) bool {
	normalized := normalizeGSLBOperator(operator)
	if normalized == "" {
		return false
	}
	for _, poolOperator := range pool.Operators {
		if normalized == normalizeGSLBOperator(poolOperator) {
			return true
		}
	}
	return false
}

func enabledGSLBPoolNames(pools []ProxyRouteGSLBPoolPolicy) map[string]struct{} {
	result := make(map[string]struct{}, len(pools))
	for _, pool := range pools {
		if !pool.Enabled {
			continue
		}
		name := normalizeNodePoolName(pool.Name)
		if name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

func buildDNSGSLBSimulationNodeView(node *model.Node, recordType string, policy ProxyRouteGSLBPolicy, matchedPools map[string]ProxyRouteGSLBPoolPolicy, metric *model.NodeMetricSnapshot, selectedSet map[string]struct{}, probeStats *dnsWorkerNodeProbeStats, requireHealthyDNSProbe bool) DNSGSLBSimulationNodeView {
	poolName := normalizeNodePoolName(node.PoolName)
	poolPolicy, poolMatched := matchedPools[poolName]
	reasons := []string{}
	if !poolMatched {
		reasons = append(reasons, "节点池未匹配当前来源")
	}
	if poolMatched && !gslbPoolAllowsNode(poolPolicy, node.NodeID) {
		reasons = append(reasons, "节点未被当前节点池子集选中")
	}
	if node.DrainMode {
		reasons = append(reasons, "节点处于排空模式")
	}
	if !isNodeSchedulableForDNS(node) {
		reasons = append(reasons, "节点已关闭自动调度")
	}
	if !isNodeOnlineAndOpenRestyHealthy(node) {
		reasons = append(reasons, "节点离线或 OpenResty 不健康")
	}
	publicIPs := resolveNodePublicIPs(node)
	candidateTargets, ipReasons := filterDNSGSLBSimulationTargets(publicIPs, recordType)
	reasons = append(reasons, ipReasons...)
	if requireHealthyDNSProbe && !dnsWorkerNodeProbeStatsSchedulable(probeStats) {
		reasons = append(reasons, dnsWorkerNodeProbeThresholdReason(probeStats))
	}
	hasMetric := metric != nil
	openrestyConnections := int64(0)
	cpuUsage := float64(0)
	memoryUsage := float64(0)
	var metricCapturedAt *time.Time
	if metric != nil {
		capturedAt := metric.CapturedAt
		metricCapturedAt = &capturedAt
		openrestyConnections = metric.OpenrestyConnections
		cpuUsage = metric.CPUUsagePercent
		memoryUsage = nodeMetricMemoryUsagePercent(metric)
		if !metricWithinGSLBThresholds(metric, policy.LoadThresholds) {
			reasons = append(reasons, "节点负载超过 GSLB 阈值")
		}
	}
	selected := []string{}
	for _, target := range candidateTargets {
		if _, ok := selectedSet[target]; ok {
			selected = append(selected, target)
		}
	}
	eligible := poolMatched &&
		gslbPoolAllowsNode(poolPolicy, node.NodeID) &&
		isNodeSchedulableForDNS(node) &&
		isNodeOnlineAndOpenRestyHealthy(node) &&
		(!requireHealthyDNSProbe || dnsWorkerNodeProbeStatsSchedulable(probeStats)) &&
		(!hasMetric || metricWithinGSLBThresholds(metric, policy.LoadThresholds)) &&
		len(candidateTargets) > 0
	if eligible {
		reasons = append(reasons, "可参与当前调度")
		if normalizeDNSScheduleMode(policy.Strategy) == "load_aware" && !hasMetric {
			reasons = append(reasons, "暂无新鲜负载指标，仅作为兜底候选")
		}
	}
	probeSummary := summarizeDNSWorkerNodeProbeStats(probeStats)
	score := float64(0)
	if poolMatched {
		candidate := gslbDNSTargetCandidate{
			NodeID:               node.NodeID,
			PoolName:             poolName,
			NodeWeight:           normalizeNodeWeight(node.Weight),
			PoolWeight:           poolPolicy.Weight,
			LastSeenAt:           node.LastSeenAt,
			OpenrestyConnections: openrestyConnections,
			CPUUsagePercent:      cpuUsage,
			MemoryUsagePercent:   memoryUsage,
			HasMetric:            hasMetric,
		}
		if requireHealthyDNSProbe && probeStats != nil {
			candidate.DNSProbeHealthy = dnsWorkerNodeProbeStatsSchedulable(probeStats)
			candidate.DNSProbeCheckedCount = probeStats.totalCount
			candidate.DNSProbeHealthyCount = probeStats.healthyCount
			candidate.DNSProbeStaleCount = probeStats.staleCount
			candidate.DNSProbeAverageRTTMs = averageFloat(probeStats.totalAverageRTTMs, probeStats.averageSamples)
		}
		score = scoreGSLBCandidate(candidate, policy.Strategy)
	}
	lastSeenAt := node.LastSeenAt
	return DNSGSLBSimulationNodeView{
		NodeID:                  node.NodeID,
		Name:                    node.Name,
		PoolName:                poolName,
		Status:                  computeNodeStatus(node),
		OpenrestyStatus:         normalizeOpenrestyStatus(node.OpenrestyStatus),
		SchedulingEnabled:       isNodeSchedulableForDNS(node),
		DrainMode:               node.DrainMode,
		LastSeenAt:              &lastSeenAt,
		PublicIPs:               publicIPs,
		CandidateTargets:        candidateTargets,
		SelectedTargets:         selected,
		Eligible:                eligible,
		Selected:                len(selected) > 0,
		Reasons:                 dedupeStrings(reasons),
		HasMetric:               hasMetric,
		MetricCapturedAt:        metricCapturedAt,
		OpenrestyConnections:    openrestyConnections,
		CPUUsagePercent:         cpuUsage,
		MemoryUsagePercent:      memoryUsage,
		Score:                   score,
		NodeProbeStatus:         probeSummary.status,
		NodeProbeMessage:        probeSummary.message,
		NodeProbeCheckedCount:   probeSummary.checkedCount,
		NodeProbeHealthyCount:   probeSummary.healthyCount,
		NodeProbeStaleCount:     probeSummary.staleCount,
		NodeProbeHealthyPercent: probeSummary.healthyPercent,
		NodeProbeAverageRTTMs:   probeSummary.averageRTTMs,
		NodeProbeMaxRTTMs:       probeSummary.maxRTTMs,
	}
}

func filterDNSGSLBSimulationTargets(values []string, recordType string) ([]string, []string) {
	targets := normalizeNodeDNSContents(values, recordType)
	reasons := []string{}
	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target] = struct{}{}
	}
	for _, value := range values {
		normalized := iputil.NormalizeIP(value)
		parsed := net.ParseIP(normalized)
		if parsed == nil {
			reasons = append(reasons, "公网 IP 格式无效")
			continue
		}
		content := parsed.String()
		if _, ok := targetSet[content]; ok {
			continue
		}
		if !iputil.IsPublicString(normalized) {
			reasons = append(reasons, "公网 IP 不是可路由公网地址")
			continue
		}
		if recordType == "A" && parsed.To4() == nil {
			reasons = append(reasons, "缺少 IPv4 公网 IP")
			continue
		}
		if recordType == "AAAA" && parsed.To4() != nil {
			reasons = append(reasons, "缺少 IPv6 公网 IP")
			continue
		}
	}
	if len(values) == 0 {
		reasons = append(reasons, "未配置节点公网 IP 池")
	} else if len(targets) == 0 {
		reasons = append(reasons, "没有符合记录类型的公网 IP")
	}
	return targets, dedupeStrings(reasons)
}

func convertWorkerGSLBPolicyToAuthoritative(policy dnsworker.GSLBPolicy) ProxyRouteGSLBPolicy {
	result := ProxyRouteGSLBPolicy{
		Mode:                   policy.Mode,
		Strategy:               policy.Strategy,
		PoolMatchMode:          policy.PoolMatchMode,
		TargetCount:            policy.TargetCount,
		TTL:                    policy.TTL,
		SourcePoolFallbackMode: policy.SourcePoolFallbackMode,
		SourceIP: ProxyRouteGSLBSourceIPProvider{
			Provider: policy.SourceIP.Provider,
			APIURL:   policy.SourceIP.APIURL,
			APIToken: policy.SourceIP.APIToken,
		},
		LoadThresholds: ProxyRouteGSLBLoadThresholds{
			MaxOpenrestyConnections: policy.LoadThresholds.MaxOpenrestyConnections,
			MaxCPUPercent:           policy.LoadThresholds.MaxCPUPercent,
			MaxMemoryPercent:        policy.LoadThresholds.MaxMemoryPercent,
		},
		Debounce: ProxyRouteGSLBDebounce{
			CooldownSeconds:    policy.Debounce.CooldownSeconds,
			UnhealthyThreshold: policy.Debounce.UnhealthyThreshold,
			RecoveryThreshold:  policy.Debounce.RecoveryThreshold,
		},
		Pools: make([]ProxyRouteGSLBPoolPolicy, 0, len(policy.Pools)),
	}
	for _, pool := range policy.Pools {
		result.Pools = append(result.Pools, ProxyRouteGSLBPoolPolicy{
			Name:               pool.Name,
			Weight:             pool.Weight,
			Countries:          append([]string(nil), pool.Countries...),
			SourceCIDRs:        append([]string(nil), pool.SourceCIDRs...),
			Operators:          append([]string(nil), pool.Operators...),
			ASNs:               append([]uint32(nil), pool.ASNs...),
			ExcludeCountries:   append([]string(nil), pool.ExcludeCountries...),
			ExcludeSourceCIDRs: append([]string(nil), pool.ExcludeSourceCIDRs...),
			ExcludeOperators:   append([]string(nil), pool.ExcludeOperators...),
			ExcludeASNs:        append([]uint32(nil), pool.ExcludeASNs...),
			NodeIDs:            append([]string(nil), pool.NodeIDs...),
			Enabled:            pool.Enabled,
		})
	}
	return result
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
