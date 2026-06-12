package service

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
)

func ListAuthoritativeDNSMigrationCandidates() ([]AuthoritativeDNSMigrationCandidateView, error) {
	return listAuthoritativeDNSMigrationCandidatesWithQueries(defaultGSLBDNSSchedulingDataQueries)
}

func listAuthoritativeDNSMigrationCandidatesWithQueries(schedulingQueries gslbDNSSchedulingDataQueries) ([]AuthoritativeDNSMigrationCandidateView, error) {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return nil, err
	}
	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	workerStats, err := authoritativeDNSMigrationWorkerStats()
	if err != nil {
		return nil, err
	}
	schedulingOptions := authoritativeDNSSchedulingOptions()
	schedulingData, err := loadGSLBDNSSchedulingDataWithQueries(true, schedulingQueries)
	if err != nil {
		return nil, err
	}
	schedulingOptions.Data = schedulingData
	candidates := make([]AuthoritativeDNSMigrationCandidateView, 0, len(routes))
	for _, route := range routes {
		if route == nil || normalizeDNSProviderMode(route.DNSProviderMode) == DNSProviderModeAuthoritative {
			continue
		}
		if !route.Enabled && !route.DNSAutoSync && !route.GSLBEnabled {
			continue
		}
		candidate, err := buildAuthoritativeDNSMigrationCandidate(route, zones, workerStats, schedulingOptions)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Ready != candidates[j].Ready {
			return candidates[i].Ready
		}
		if candidates[i].SiteName != candidates[j].SiteName {
			return candidates[i].SiteName < candidates[j].SiteName
		}
		return candidates[i].ProxyRouteID < candidates[j].ProxyRouteID
	})
	return candidates, nil
}

func SwitchProxyRouteToAuthoritativeDNS(id uint, input AuthoritativeDNSMigrationInput) (*ProxyRouteView, error) {
	route, err := model.GetProxyRouteByID(id)
	if err != nil {
		return nil, err
	}
	if route.DNSProviderMode == DNSProviderModeAuthoritative {
		return buildProxyRouteView(route)
	}

	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return nil, err
	}
	zone, err := resolveAuthoritativeMigrationZone(input.DNSZoneIDRef, domains)
	if err != nil {
		return nil, err
	}
	if err := validateAuthoritativeDNSReadyWorkers(); err != nil {
		return nil, err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("authoritative DNS migration only supports A/AAAA dynamic records")
	}
	if route.GSLBEnabled {
		if _, err := decodeStoredGSLBPolicy(route.GSLBPolicy); err != nil {
			return nil, err
		}
	}
	if err := validateAuthoritativeProxyRouteStaticRecordConflicts(zone.ID, domains, recordType, route.Enabled); err != nil {
		return nil, err
	}
	if _, err := precheckAuthoritativeRouteDNSTargets(route, recordType); err != nil {
		return nil, err
	}

	now := time.Now()
	zoneID := zone.ID
	route.DNSProviderMode = DNSProviderModeAuthoritative
	route.DNSZoneIDRef = &zoneID
	route.DNSAutoSync = false
	route.DNSAccountID = nil
	route.DNSZoneID = ""
	route.DNSRecordType = recordType
	route.DNSRecordContent = ""
	route.DNSRecordIDs = "{}"
	route.DNSAutoTarget = true
	route.DNSTargetCount = normalizeDNSTargetCount(route.DNSTargetCount)
	route.DNSScheduleMode = normalizeDNSScheduleMode(route.DNSScheduleMode)
	route.DNSTTL = normalizeDNSTTL(route.DNSTTL)
	route.CloudflareProxied = false
	route.DDOSProtectionMode = DDOSProtectionModeOff
	route.DDOSProtectionProvider = DDOSProtectionProviderCloudflare
	route.DDOSProtectionTarget = ""
	route.DNSLastSyncStatus = DNSRecordSyncStatusSuccess
	route.DNSLastSyncMessage = fmt.Sprintf("已切换到本地自建解析托管域名 %s；请在注册商确认 NS 指向。", zone.Name)
	route.DNSLastSyncedAt = &now
	if err := model.DB.Model(route).Select(
		"dns_provider_mode",
		"dns_zone_id_ref",
		"dns_auto_sync",
		"dns_account_id",
		"dns_zone_id",
		"dns_record_type",
		"dns_record_content",
		"dns_record_ids",
		"dns_auto_target",
		"dns_target_count",
		"dns_schedule_mode",
		"dns_ttl",
		"cloudflare_proxied",
		"ddos_protection_mode",
		"ddos_protection_provider",
		"ddos_protection_target",
		"dns_last_sync_status",
		"dns_last_sync_message",
		"dns_last_synced_at",
	).Updates(route).Error; err != nil {
		return nil, err
	}
	if err := model.SyncProxyRouteNormalizedTables(route); err != nil {
		return nil, err
	}
	return buildProxyRouteView(route)
}

type authoritativeDNSMigrationWorkerStatsView struct {
	total                       int
	online                      int
	publicReachable             int
	freshSnapshot               int
	ready                       int
	publicReachableWithoutFresh int
	readySnapshotVersionCount   int
}

type authoritativeDNSWorkerReadiness struct {
	online          bool
	publicReachable bool
	freshSnapshot   bool
	ready           bool
}

type authoritativeDNSTargetPrecheckView struct {
	targets     []string
	targetCount int
	recordType  string
	strategy    string
	warnings    []string
}

type authoritativeDNSTargetPrecheckSource struct {
	label  string
	key    string
	source GSLBSourceContext
}

func authoritativeDNSMigrationWorkerStats() (authoritativeDNSMigrationWorkerStatsView, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return authoritativeDNSMigrationWorkerStatsView{}, err
	}
	stats := authoritativeDNSMigrationWorkerStatsView{total: len(workers)}
	now := time.Now().UTC()
	snapshotMaxAge := authoritativeDNSSnapshotMaxAge()
	readySnapshotVersions := map[string]int{}
	for _, worker := range workers {
		readiness := evaluateAuthoritativeDNSWorkerReadiness(now, snapshotMaxAge, worker)
		if readiness.online {
			stats.online++
		}
		if readiness.publicReachable {
			stats.publicReachable++
		}
		if readiness.freshSnapshot {
			stats.freshSnapshot++
		}
		if readiness.ready {
			stats.ready++
			readySnapshotVersions[strings.TrimSpace(worker.LastSnapshotVersion)]++
		}
		if readiness.publicReachable && !readiness.freshSnapshot {
			stats.publicReachableWithoutFresh++
		}
	}
	stats.readySnapshotVersionCount = len(readySnapshotVersions)
	return stats, nil
}

func buildAuthoritativeDNSMigrationCandidate(route *model.ProxyRoute, zones []*model.DNSZone, workerStats authoritativeDNSMigrationWorkerStatsView, schedulingOptions gslbDNSSchedulingOptions) (AuthoritativeDNSMigrationCandidateView, error) {
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		return AuthoritativeDNSMigrationCandidateView{}, err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	candidate := AuthoritativeDNSMigrationCandidateView{
		ProxyRouteID:               route.ID,
		SiteName:                   normalizeProxyRouteSiteNameInput(route, route.SiteName, route.Domain),
		PrimaryDomain:              route.Domain,
		Domains:                    domains,
		Enabled:                    route.Enabled,
		DNSAutoSync:                route.DNSAutoSync,
		DNSProviderMode:            normalizeDNSProviderMode(route.DNSProviderMode),
		DNSRecordType:              recordType,
		GSLBEnabled:                route.GSLBEnabled,
		TotalWorkerCount:           workerStats.total,
		OnlineWorkerCount:          workerStats.online,
		PublicReachableWorkerCount: workerStats.publicReachable,
		FreshSnapshotWorkerCount:   workerStats.freshSnapshot,
		ReadyWorkerCount:           workerStats.ready,
		Blockers:                   []string{},
		Warnings:                   []string{},
	}
	zone := bestAuthoritativeZoneForDomains(zones, domains)
	if zone != nil {
		zoneID := zone.ID
		candidate.MatchingZoneID = &zoneID
		candidate.MatchingZoneName = zone.Name
		candidate.MatchingZoneEnabled = zone.Enabled
	}
	if len(domains) == 0 {
		candidate.Blockers = append(candidate.Blockers, "网站未配置域名")
	}
	if zone == nil {
		candidate.Blockers = append(candidate.Blockers, "没有匹配的托管域名")
	} else if !zone.Enabled {
		candidate.Blockers = append(candidate.Blockers, "匹配的托管域名已停用")
	} else if err := validateAuthoritativeProxyRouteStaticRecordConflicts(zone.ID, domains, recordType, route.Enabled); err != nil {
		candidate.Blockers = append(candidate.Blockers, err.Error())
	}
	targetPrecheck, targetErr := precheckAuthoritativeRouteDNSTargetsWithOptions(route, recordType, schedulingOptions)
	if targetErr != nil {
		candidate.Blockers = append(candidate.Blockers, targetErr.Error())
	}
	candidate.Warnings = append(candidate.Warnings, targetPrecheck.warnings...)
	if workerStats.online == 0 {
		candidate.Blockers = append(candidate.Blockers, "没有在线 DNS 响应端")
	} else if workerStats.publicReachable == 0 {
		candidate.Blockers = append(candidate.Blockers, "在线 DNS 响应端尚未通过公网 UDP/TCP 53 探测")
	} else if workerStats.ready == 0 {
		candidate.Blockers = append(candidate.Blockers, "公网可达 DNS 响应端尚未拉取未过期的解析配置")
	} else if workerStats.publicReachableWithoutFresh > 0 {
		candidate.Blockers = append(candidate.Blockers, "部分公网可达 DNS 响应端尚未拉取未过期的解析配置")
	} else if workerStats.readySnapshotVersionCount > 1 {
		candidate.Blockers = append(candidate.Blockers, "公网可达 DNS 响应端的解析配置版本不一致")
	}
	if !route.GSLBEnabled {
		candidate.Warnings = append(candidate.Warnings, "未启用 GSLB，多节点池实时分流不会生效")
	}
	if workerStats.total < 2 {
		candidate.Warnings = append(candidate.Warnings, "生产环境建议至少部署 2 个 DNS 响应端")
	}
	if workerStats.online > workerStats.publicReachable {
		candidate.Warnings = append(candidate.Warnings, "部分在线响应端未通过最新公网探测，迁移前建议逐个点击「探测」")
	}
	if workerStats.publicReachableWithoutFresh == 0 && workerStats.online > workerStats.freshSnapshot {
		candidate.Warnings = append(candidate.Warnings, "部分在线响应端尚未拉取未过期解析配置，请确认配置同步正常")
	}
	if !route.DNSAutoSync {
		candidate.Warnings = append(candidate.Warnings, "当前未启用 Cloudflare 自动 DNS，请确认是否仍需要迁移")
	}
	candidate.Ready = len(candidate.Blockers) == 0
	return candidate, nil
}

func bestAuthoritativeZoneForDomains(zones []*model.DNSZone, domains []string) *model.DNSZone {
	if len(domains) == 0 {
		return nil
	}
	var best *model.DNSZone
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		matched := true
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if best == nil || len(normalizeDNSRecordName(zone.Name)) > len(normalizeDNSRecordName(best.Name)) {
			best = zone
		}
	}
	return best
}

func precheckAuthoritativeRouteDNSTargets(route *model.ProxyRoute, recordType string) (authoritativeDNSTargetPrecheckView, error) {
	return precheckAuthoritativeRouteDNSTargetsWithOptions(route, recordType, authoritativeDNSSchedulingOptions())
}

func precheckAuthoritativeRouteDNSTargetsWithOptions(route *model.ProxyRoute, recordType string, schedulingOptions gslbDNSSchedulingOptions) (authoritativeDNSTargetPrecheckView, error) {
	view := authoritativeDNSTargetPrecheckView{
		targetCount: 1,
		recordType:  normalizeDNSRecordType(recordType),
		strategy:    "healthy",
		warnings:    []string{},
	}
	if route == nil {
		return view, errors.New("网站配置不存在")
	}
	recordType = normalizeDNSRecordType(recordType)
	view.recordType = recordType
	if recordType != "A" && recordType != "AAAA" {
		return view, errors.New("本地自建解析自动选 IP 只支持 A/AAAA")
	}
	view.targetCount = normalizeDNSTargetCount(route.DNSTargetCount)
	view.strategy = normalizeDNSScheduleMode(route.DNSScheduleMode)
	policy := defaultGSLBPolicy(route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
	if route.GSLBEnabled {
		decodedPolicy, err := decodeStoredGSLBPolicy(route.GSLBPolicy)
		if err != nil {
			return view, err
		}
		policy, err = normalizeGSLBPolicy(decodedPolicy, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode, route.DNSTTL)
		if err != nil {
			return view, err
		}
		view.targetCount = normalizeDNSTargetCount(policy.TargetCount)
		view.strategy = normalizeDNSScheduleMode(policy.Strategy)
	}
	selection, err := selectProxyRouteDNSTargetsWithOptions(route, recordType, schedulingOptions)
	if err != nil {
		return view, formatAuthoritativeDNSTargetPrecheckError("当前节点池/GSLB", recordType, err, policy, GSLBSourceContext{}, schedulingOptions, true)
	}
	targets, err := normalizeDNSRecordContents(recordType, selection.Targets)
	if err != nil {
		return view, fmt.Errorf("当前节点池/GSLB 返回的 %s 边缘 IP 无效：%w", recordType, err)
	}
	if len(targets) == 0 {
		return view, fmt.Errorf("当前节点池/GSLB 没有可用于 %s 记录的边缘 IP", recordType)
	}
	view.targets = targets
	if view.targetCount > len(targets) {
		view.warnings = append(view.warnings, fmt.Sprintf("全局当前只能返回 %d / %d 个 %s 边缘 IP，请检查节点池容量", len(targets), view.targetCount, view.recordType))
	}
	if route.GSLBEnabled {
		blockers := []string{}
		for _, source := range authoritativeDNSTargetPrecheckSources(policy) {
			selection, err := selectGSLBDNSTargetsWithOptions(route, recordType, source.source, schedulingOptions)
			if err != nil {
				blockers = append(blockers, formatAuthoritativeDNSTargetPrecheckError(source.label, recordType, err, policy, source.source, schedulingOptions, false).Error())
				continue
			}
			targets, err := normalizeDNSRecordContents(recordType, selection.Targets)
			if err != nil {
				blockers = append(blockers, fmt.Sprintf("%s 返回的 %s 边缘 IP 无效：%v", source.label, recordType, err))
				continue
			}
			if len(targets) == 0 {
				blockers = append(blockers, fmt.Sprintf("%s 没有可用于 %s 记录的边缘 IP", source.label, recordType))
				continue
			}
			if view.targetCount > len(targets) {
				view.warnings = append(view.warnings, fmt.Sprintf("%s 当前只能返回 %d / %d 个 %s 边缘 IP，请检查匹配节点池容量", source.label, len(targets), view.targetCount, recordType))
			}
		}
		if len(blockers) > 0 {
			return view, errors.New(strings.Join(blockers, "；"))
		}
	}
	return view, nil
}

func formatAuthoritativeDNSTargetPrecheckError(label string, recordType string, err error, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions, includeChecklist bool) error {
	message := fmt.Sprintf("%s 无法返回 %s 边缘 IP", label, recordType)
	if includeChecklist {
		message += "，请检查节点池、公网 IP、节点在线状态、OpenResty 健康、GSLB 负载阈值和 Agent 探测调度门槛"
	}
	if detail := summarizeAuthoritativeDNSTargetPrecheckDiagnostics(recordType, policy, source, options); detail != "" {
		message += "；诊断：" + detail
	}
	return fmt.Errorf("%s：%w", message, err)
}

func summarizeAuthoritativeDNSTargetPrecheckDiagnostics(recordType string, policy ProxyRouteGSLBPolicy, source GSLBSourceContext, options gslbDNSSchedulingOptions) string {
	diagnostics := buildDNSGSLBSimulationDiagnosticsWithOptions(recordType, convertAuthoritativeGSLBPolicyToWorker(policy), source, nil, options)
	matchedPoolLabels := []string{}
	matchedPoolNames := []string{}
	matchedPools := map[string]struct{}{}
	for _, pool := range diagnostics.pools {
		if !pool.Matched {
			continue
		}
		matchedPools[pool.Name] = struct{}{}
		matchedPoolNames = append(matchedPoolNames, pool.Name)
		label := pool.Name
		if pool.Reason != "" && pool.Reason != "参与全局调度" {
			label += "（" + pool.Reason + "）"
		}
		matchedPoolLabels = append(matchedPoolLabels, label)
	}
	nodeDetails := summarizeAuthoritativeDNSTargetPrecheckNodes(diagnostics.nodes, matchedPools)
	if len(nodeDetails) == 0 {
		poolLabels := matchedPoolLabels
		if len(poolLabels) == 0 {
			poolLabels = matchedPoolNames
		}
		if len(poolLabels) > 0 {
			return "匹配节点池 " + strings.Join(poolLabels, "、") + " 没有节点"
		}
		return "没有启用的 GSLB 节点池"
	}
	parts := []string{}
	if len(matchedPoolLabels) > 0 {
		parts = append(parts, "匹配节点池 "+strings.Join(matchedPoolLabels, "、"))
	}
	parts = append(parts, nodeDetails...)
	return strings.Join(parts, "；")
}

func summarizeAuthoritativeDNSTargetPrecheckNodes(nodes []DNSGSLBSimulationNodeView, matchedPools map[string]struct{}) []string {
	details := []string{}
	omitted := 0
	for _, node := range nodes {
		if len(matchedPools) > 0 {
			if _, ok := matchedPools[node.PoolName]; !ok {
				continue
			}
		}
		reasons := compactAuthoritativeDNSTargetPrecheckNodeReasons(node)
		if len(reasons) == 0 {
			continue
		}
		if len(details) >= 3 {
			omitted++
			continue
		}
		details = append(details, fmt.Sprintf("节点 %s：%s", authoritativeDNSTargetPrecheckNodeLabel(node), strings.Join(reasons, "、")))
	}
	if omitted > 0 {
		details = append(details, fmt.Sprintf("另有 %d 个节点被排除", omitted))
	}
	return details
}

func compactAuthoritativeDNSTargetPrecheckNodeReasons(node DNSGSLBSimulationNodeView) []string {
	reasons := []string{}
	for _, reason := range node.Reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" || reason == "节点池未匹配当前来源" || reason == "可参与当前调度" {
			continue
		}
		reasons = append(reasons, reason)
		if len(reasons) >= 2 {
			break
		}
	}
	if len(reasons) == 0 && !node.Eligible {
		reasons = append(reasons, "未满足当前调度条件")
	}
	return dedupeStrings(reasons)
}

func authoritativeDNSTargetPrecheckNodeLabel(node DNSGSLBSimulationNodeView) string {
	nodeID := strings.TrimSpace(node.NodeID)
	name := strings.TrimSpace(node.Name)
	if nodeID == "" {
		return name
	}
	if name != "" && name != nodeID {
		return nodeID + "/" + name
	}
	return nodeID
}

func authoritativeDNSSchedulingOptions() gslbDNSSchedulingOptions {
	return gslbDNSSchedulingOptions{
		RequireHealthyDNSProbe: common.GSLBProbeSchedulingEnabled,
	}
}

func authoritativeDNSTargetPrecheckSources(policy ProxyRouteGSLBPolicy) []authoritativeDNSTargetPrecheckSource {
	sources := make([]authoritativeDNSTargetPrecheckSource, 0)
	seen := map[string]struct{}{}
	appendSource := func(source authoritativeDNSTargetPrecheckSource) {
		if strings.TrimSpace(source.key) == "" {
			return
		}
		if _, ok := seen[source.key]; ok {
			return
		}
		seen[source.key] = struct{}{}
		sources = append(sources, source)
	}
	for _, pool := range policy.Pools {
		if !pool.Enabled {
			continue
		}
		for _, value := range pool.SourceCIDRs {
			cidr, ok := normalizeGSLBCIDR(value)
			if !ok {
				continue
			}
			sourceIP := sampleIPForGSLBCIDR(cidr)
			if sourceIP == "" {
				continue
			}
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源网段 " + cidr,
				key:    "cidr:" + cidr,
				source: GSLBSourceContext{IP: sourceIP},
			})
		}
		for _, value := range pool.Countries {
			country := strings.ToUpper(strings.TrimSpace(value))
			if country == "" || !proxyRouteRegionCountryPattern.MatchString(country) {
				continue
			}
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源国家 " + country,
				key:    "country:" + country,
				source: GSLBSourceContext{Country: country},
			})
		}
		for _, value := range pool.ASNs {
			if value == 0 {
				continue
			}
			asn := strconv.FormatUint(uint64(value), 10)
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源 ASN " + asn,
				key:    "asn:" + asn,
				source: GSLBSourceContext{ASN: value},
			})
		}
		for _, value := range pool.Operators {
			operator := normalizeGSLBOperator(value)
			if operator == "" {
				continue
			}
			appendSource(authoritativeDNSTargetPrecheckSource{
				label:  "来源运营商 " + operator,
				key:    "operator:" + operator,
				source: GSLBSourceContext{Operator: operator},
			})
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].key < sources[j].key
	})
	return sources
}

func sampleIPForGSLBCIDR(raw string) string {
	cidr, ok := normalizeGSLBCIDR(raw)
	if !ok {
		return ""
	}
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil || network == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	network.IP = ip.Mask(network.Mask)
	if ipv4 := network.IP.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return network.IP.String()
}

func resolveAuthoritativeMigrationZone(zoneIDRef *uint, domains []string) (*model.DNSZone, error) {
	if len(domains) == 0 {
		return nil, errors.New("proxy route has no domains")
	}
	if zoneIDRef != nil && *zoneIDRef > 0 {
		zone, err := model.GetDNSZoneByID(*zoneIDRef)
		if err != nil {
			return nil, errors.New("selected DNS zone does not exist")
		}
		if !zone.Enabled {
			return nil, errors.New("selected DNS zone is disabled")
		}
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				return nil, fmt.Errorf("domain %s is not under DNS zone %s", domain, zone.Name)
			}
		}
		return zone, nil
	}

	zones, err := model.ListDNSZones()
	if err != nil {
		return nil, err
	}
	var best *model.DNSZone
	for _, zone := range zones {
		if zone == nil || !zone.Enabled {
			continue
		}
		matched := true
		for _, domain := range domains {
			if !domainBelongsToZone(domain, zone.Name) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if best == nil || len(normalizeDNSRecordName(zone.Name)) > len(normalizeDNSRecordName(best.Name)) {
			best = zone
		}
	}
	if best == nil {
		return nil, errors.New("no enabled DNS zone covers all route domains")
	}
	return best, nil
}
