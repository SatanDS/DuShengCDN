package service

import (
	"context"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"dushengcdn/utils/security"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

func SyncProxyRouteDNS(route *model.ProxyRoute) error {
	return syncProxyRouteDNSWithContext(route, nil)
}

type proxyRouteDNSSyncContext struct {
	accountsByID      map[uint]*model.DnsAccount
	ddosActiveLoaded  bool
	ddosActive        bool
	trafficSummaryErr error
}

var getRequestReportTrafficSummaryForDNSProtection = model.GetRequestReportTrafficSummary

func newProxyRouteDNSSyncContext(routes []*model.ProxyRoute) (*proxyRouteDNSSyncContext, error) {
	accountIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for _, route := range routes {
		if route == nil || !shouldSyncProxyRouteCloudflareDNS(route) {
			continue
		}
		for _, accountID := range proxyRouteDNSAccountCandidateIDs(route) {
			if accountID == 0 {
				continue
			}
			if _, ok := seen[accountID]; ok {
				continue
			}
			seen[accountID] = struct{}{}
			accountIDs = append(accountIDs, accountID)
		}
	}
	accounts, err := model.ListDnsAccountsByIDs(accountIDs)
	if err != nil {
		return nil, err
	}
	accountsByID := make(map[uint]*model.DnsAccount, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		accountsByID[account.ID] = account
	}
	return &proxyRouteDNSSyncContext{accountsByID: accountsByID}, nil
}

func (context *proxyRouteDNSSyncContext) accountByID(id uint) (*model.DnsAccount, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if context == nil {
		return model.GetDnsAccountByID(id)
	}
	account := context.accountsByID[id]
	if account == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return account, nil
}

func (context *proxyRouteDNSSyncContext) ddosProtectionActive(route *model.ProxyRoute) bool {
	if route == nil || normalizeDDOSProtectionMode(route.DDOSProtectionMode) != DDOSProtectionModeAuto {
		return false
	}
	if !shouldEnableDDOSProtectionWithContext(context) {
		return false
	}
	return true
}

func syncProxyRouteDNSWithContext(route *model.ProxyRoute, syncContext *proxyRouteDNSSyncContext) error {
	if route == nil || !shouldSyncProxyRouteCloudflareDNS(route) {
		return nil
	}
	ddosActive := routeDDOSProtectionActiveWithContext(route, syncContext)
	ddosProvider := normalizeDDOSProtectionProvider(route.DDOSProtectionProvider)
	account, err := proxyRouteDNSAccountForSyncWithContext(route, ddosActive, syncContext)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	client, err := newCloudflareClientFromAccount(account)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}

	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	recordType := normalizeDNSRecordType(route.DNSRecordType)
	storedContent := strings.TrimSpace(route.DNSRecordContent)
	content := storedContent
	targets := splitDNSRecordContent(storedContent)
	selection := proxyRouteDNSTargetSelection{
		Targets:        targets,
		DesiredTargets: targets,
		TTL:            normalizeDNSTTL(route.DNSTTL),
	}
	switch {
	case ddosActive && ddosProvider == DDOSProtectionProviderCustom:
		selection, err = selectProxyRouteDDOSProtectionTargets(route, recordType)
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
		targets = selection.Targets
	case ddosActive && ddosProvider == DDOSProtectionProviderCloudflare && isAddressDNSRecordType(recordType):
		selection, err = selectProxyRouteDNSDefaultPoolTargets(route, recordType)
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
		targets = selection.Targets
	case route.GSLBEnabled || route.DNSAutoTarget || content == "":
		selection, err = selectProxyRouteDNSTargets(route, recordType)
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
		targets = selection.Targets
	default:
		targets = selection.Targets
	}
	targets, err = normalizeDNSRecordContents(recordType, targets)
	if err != nil {
		recordProxyRouteDNSSyncFailure(route, err)
		return err
	}
	content = strings.Join(targets, ",")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	zoneID := strings.TrimSpace(route.DNSZoneID)
	if zoneID == "" {
		zone, err := client.FindBestZoneForDomain(ctx, domains[0])
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, err)
			return err
		}
		zoneID = zone.ID
	}

	recordIDs := decodeDNSRecordIDs(route.DNSRecordIDs)
	nextRecordIDs := make(map[string]string, len(domains))
	for _, domain := range domains {
		recordName := normalizeDNSRecordName(domain)
		if strings.TrimSpace(route.DNSRecordName) != "" && len(domains) == 1 {
			recordName = normalizeDNSRecordName(route.DNSRecordName)
		}
		records, err := client.SyncDNSRecords(ctx, CloudflareDNSSyncInput{
			ZoneID:           zoneID,
			Type:             recordType,
			Name:             recordName,
			Contents:         targets,
			Proxied:          effectiveCloudflareProxied(route, ddosActive),
			TTL:              selection.TTL,
			ManagedRecordIDs: filterDNSRecordIDsForName(recordIDs, recordName),
		})
		if err != nil {
			recordProxyRouteDNSSyncFailure(route, fmt.Errorf("同步 DNS 记录 %s 失败：%w", recordName, err))
			return err
		}
		for _, record := range records {
			nextRecordIDs[dnsRecordStorageKey(recordName, record.Content)] = record.ID
		}
	}

	nextRecordIDSet := make(map[string]struct{}, len(nextRecordIDs))
	for _, recordID := range nextRecordIDs {
		if strings.TrimSpace(recordID) != "" {
			nextRecordIDSet[recordID] = struct{}{}
		}
	}
	for recordName, recordID := range recordIDs {
		if _, ok := nextRecordIDs[recordName]; ok {
			continue
		}
		if _, ok := nextRecordIDSet[recordID]; ok {
			continue
		}
		if err := client.DeleteDNSRecord(ctx, zoneID, recordID); err != nil {
			slog.Warn("delete stale cloudflare dns record failed", "route_id", route.ID, "record_name", recordName, "error", err)
		}
	}

	now := time.Now()
	route.DNSZoneID = zoneID
	route.DNSRecordType = recordType
	route.DNSRecordIDs = encodeDNSRecordIDs(nextRecordIDs)
	route.DNSTTL = selection.TTL
	route.DNSLastSyncStatus = DNSRecordSyncStatusSuccess
	route.DNSLastSyncMessage = formatCloudflareDNSSyncMessage(len(nextRecordIDs), content, ddosActive, ddosProvider)
	route.DNSLastSyncedAt = &now
	if err := recordProxyRouteGSLBDecision(route, recordType, selection); err != nil {
		slog.Warn("record gslb dns decision failed", "route_id", route.ID, "site_name", route.SiteName, "error", err)
	}
	updateColumns := []string{
		"dns_zone_id",
		"dns_record_type",
		"dns_ttl",
		"dns_record_ids",
		"dns_last_sync_status",
		"dns_last_sync_message",
		"dns_last_synced_at",
	}
	if !ddosActive {
		route.DNSRecordContent = content
		updateColumns = append(updateColumns, "dns_record_content", "dns_auto_target")
	}
	if err := model.DB.Model(route).Select(updateColumns).Updates(route).Error; err != nil {
		return err
	}
	if err := model.SyncProxyRouteNormalizedTables(route); err != nil {
		return err
	}
	slog.Info("cloudflare dns synced", "route_id", route.ID, "site_name", route.SiteName, "records", len(nextRecordIDs), "content", content, "proxied", effectiveCloudflareProxied(route, ddosActive), "ddos_active", ddosActive, "ddos_provider", ddosProvider)
	return nil
}

func DeleteProxyRouteDNSRecords(route *model.ProxyRoute) error {
	if route == nil || !shouldSyncProxyRouteCloudflareDNS(route) || route.DNSAccountID == nil {
		return nil
	}
	recordIDs := decodeDNSRecordIDs(route.DNSRecordIDs)
	if len(recordIDs) == 0 || strings.TrimSpace(route.DNSZoneID) == "" {
		return nil
	}
	account, err := proxyRouteDNSAccount(route)
	if err != nil {
		return err
	}
	client, err := newCloudflareClientFromAccount(account)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for recordName, recordID := range recordIDs {
		if err := client.DeleteDNSRecord(ctx, route.DNSZoneID, recordID); err != nil {
			return fmt.Errorf("删除 Cloudflare DNS 记录 %s 失败：%w", recordName, err)
		}
	}
	return nil
}

func ReconcileCloudflareDNSAutomation() error {
	routes, err := model.ListProxyRoutes()
	if err != nil {
		return err
	}
	context, err := newProxyRouteDNSSyncContext(routes)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route == nil || !shouldSyncProxyRouteCloudflareDNS(route) {
			continue
		}
		if route.GSLBEnabled || route.DNSAutoTarget || strings.TrimSpace(route.DNSRecordContent) == "" {
			previousContent := strings.TrimSpace(route.DNSRecordContent)
			selection, selectErr := selectProxyRouteDNSTargets(route, normalizeDNSRecordType(route.DNSRecordType))
			desiredContent := strings.Join(selection.Targets, ",")
			if selectErr == nil && desiredContent != "" && desiredContent != previousContent {
				route.DNSRecordContent = desiredContent
				route.DNSAutoTarget = true
				route.DNSTTL = selection.TTL
			}
		}
		if err := syncProxyRouteDNSWithContext(route, context); err != nil {
			slog.Warn("cloudflare dns reconcile failed", "route_id", route.ID, "site_name", route.SiteName, "error", err)
			continue
		}
	}
	return nil
}

func routeDDOSProtectionActive(route *model.ProxyRoute) bool {
	return routeDDOSProtectionActiveWithContext(route, nil)
}

func routeDDOSProtectionActiveWithContext(route *model.ProxyRoute, context *proxyRouteDNSSyncContext) bool {
	if context != nil {
		return context.ddosProtectionActive(route)
	}
	return route != nil &&
		normalizeDDOSProtectionMode(route.DDOSProtectionMode) == DDOSProtectionModeAuto &&
		shouldEnableDDOSProtection()
}

func effectiveCloudflareProxied(route *model.ProxyRoute, ddosActive bool) bool {
	if route == nil {
		return false
	}
	if ddosActive {
		return normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCloudflare
	}
	return route.CloudflareProxied
}

func formatCloudflareDNSSyncMessage(recordCount int, content string, ddosActive bool, ddosProvider string) string {
	if ddosActive {
		switch normalizeDDOSProtectionProvider(ddosProvider) {
		case DDOSProtectionProviderCustom:
			return fmt.Sprintf("DDoS 自动防护已生效，暂停 GSLB 并同步 %d 条记录到自定义防护池 %s", recordCount, content)
		default:
			return fmt.Sprintf("DDoS 自动防护已生效，暂停 GSLB 并同步 %d 条 Cloudflare 橙云记录到 %s", recordCount, content)
		}
	}
	return fmt.Sprintf("已同步 %d 条 Cloudflare DNS 记录到 %s", recordCount, content)
}

func selectProxyRouteDDOSProtectionTargets(route *model.ProxyRoute, recordType string) (proxyRouteDNSTargetSelection, error) {
	selection := proxyRouteDNSTargetSelection{
		TTL:      cloudflareDefaultRecordTTL,
		ScopeKey: defaultGSLBScopeKey,
		Reason:   "DDoS protection override",
	}
	if route == nil {
		return selection, errors.New("proxy route is nil")
	}
	selection.TTL = normalizeDNSTTL(route.DNSTTL)
	targetPool := normalizeNodePoolName(route.DDOSProtectionTarget)
	if targetPool == "" {
		return selection, errors.New("DDoS custom protection pool is not configured")
	}
	targets, err := selectHealthyNodeDNSTargets(recordType, targetPool, route.DNSTargetCount, route.DNSScheduleMode)
	if err != nil {
		return selection, err
	}
	selection.Targets = targets
	selection.DesiredTargets = targets
	return selection, nil
}

func selectProxyRouteDNSDefaultPoolTargets(route *model.ProxyRoute, recordType string) (proxyRouteDNSTargetSelection, error) {
	selection := proxyRouteDNSTargetSelection{
		TTL:      cloudflareDefaultRecordTTL,
		ScopeKey: defaultGSLBScopeKey,
		Reason:   "DDoS protection Cloudflare override",
	}
	if route == nil {
		return selection, errors.New("proxy route is nil")
	}
	selection.TTL = normalizeDNSTTL(route.DNSTTL)
	targets, err := selectHealthyNodeDNSTargets(recordType, route.NodePool, route.DNSTargetCount, route.DNSScheduleMode)
	if err != nil {
		return selection, err
	}
	selection.Targets = targets
	selection.DesiredTargets = targets
	return selection, nil
}

func proxyRouteDNSAccount(route *model.ProxyRoute) (*model.DnsAccount, error) {
	return proxyRouteDNSAccountWithContext(route, nil)
}

func proxyRouteDNSAccountWithContext(route *model.ProxyRoute, context *proxyRouteDNSSyncContext) (*model.DnsAccount, error) {
	if route == nil || route.DNSAccountID == nil || *route.DNSAccountID == 0 {
		return nil, errors.New("规则未绑定 DNS 账号")
	}
	return dnsAccountByIDWithContext(*route.DNSAccountID, context)
}

func proxyRouteDNSAccountForSync(route *model.ProxyRoute, ddosActive bool) (*model.DnsAccount, error) {
	return proxyRouteDNSAccountForSyncWithContext(route, ddosActive, nil)
}

func proxyRouteDNSAccountForSyncWithContext(route *model.ProxyRoute, ddosActive bool, context *proxyRouteDNSSyncContext) (*model.DnsAccount, error) {
	if route == nil {
		return nil, errors.New("规则未绑定 DNS 账号")
	}
	if !ddosActive || normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) != DDOSProtectionProviderCloudflare {
		return proxyRouteDNSAccountWithContext(route, context)
	}
	if id, ok := parseDDOSProtectionCloudflareAccountID(route.DDOSProtectionTarget); ok {
		return dnsAccountByIDWithContext(id, context)
	}
	return proxyRouteDNSAccountWithContext(route, context)
}

func dnsAccountByIDWithContext(id uint, context *proxyRouteDNSSyncContext) (*model.DnsAccount, error) {
	if context == nil {
		return model.GetDnsAccountByID(id)
	}
	return context.accountByID(id)
}

func proxyRouteDNSAccountCandidateIDs(route *model.ProxyRoute) []uint {
	if route == nil {
		return nil
	}
	ids := make([]uint, 0, 2)
	if route.DNSAccountID != nil && *route.DNSAccountID != 0 {
		ids = append(ids, *route.DNSAccountID)
	}
	if normalizeDDOSProtectionMode(route.DDOSProtectionMode) == DDOSProtectionModeAuto &&
		normalizeDDOSProtectionProvider(route.DDOSProtectionProvider) == DDOSProtectionProviderCloudflare {
		if id, ok := parseDDOSProtectionCloudflareAccountID(route.DDOSProtectionTarget); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseDDOSProtectionCloudflareAccountID(raw string) (uint, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	var parsed uint64
	if _, err := fmt.Sscan(text, &parsed); err != nil || parsed == 0 {
		return 0, false
	}
	return uint(parsed), true
}

func isAddressDNSRecordType(recordType string) bool {
	recordType = normalizeDNSRecordType(recordType)
	return recordType == "A" || recordType == "AAAA"
}

func recordProxyRouteDNSSyncFailure(route *model.ProxyRoute, syncErr error) {
	if route == nil || route.ID == 0 || syncErr == nil {
		return
	}
	now := time.Now()
	route.DNSLastSyncStatus = DNSRecordSyncStatusFailed
	route.DNSLastSyncMessage = security.RedactSensitiveText(syncErr.Error())
	route.DNSLastSyncedAt = &now
	if err := model.DB.Model(route).Select("dns_last_sync_status", "dns_last_sync_message", "dns_last_synced_at").Updates(route).Error; err != nil {
		slog.Warn("record dns sync failure failed", "route_id", route.ID, "error", err)
	}
}

func filterDNSRecordIDsForName(recordIDs map[string]string, recordName string) map[string]string {
	recordName = normalizeDNSRecordName(recordName)
	filtered := make(map[string]string)
	for key, recordID := range recordIDs {
		name, _, ok := strings.Cut(key, "|")
		if !ok {
			name = key
		}
		if normalizeDNSRecordName(name) != recordName {
			continue
		}
		filtered[key] = recordID
	}
	return filtered
}

func selectHealthyNodeDNSContent(recordType string) (string, error) {
	targets, err := selectHealthyNodeDNSTargets(recordType, "default", 1, "healthy")
	if err != nil {
		return "", err
	}
	return targets[0], nil
}

type nodeDNSTargetCandidate struct {
	NodeID     string
	Content    string
	Weight     int
	LastSeenAt time.Time
}

func selectHealthyNodeDNSTargets(recordType string, poolName string, count int, scheduleMode string) ([]string, error) {
	return selectHealthyNodeDNSTargetsWithOptions(recordType, poolName, count, scheduleMode, gslbDNSSchedulingOptions{})
}

func selectHealthyNodeDNSTargetsWithOptions(recordType string, poolName string, count int, scheduleMode string, options gslbDNSSchedulingOptions) ([]string, error) {
	nodes, err := gslbDNSSchedulingNodes(options)
	if err != nil {
		return nil, err
	}
	recordType = normalizeDNSRecordType(recordType)
	if recordType != "A" && recordType != "AAAA" {
		return nil, errors.New("automatic DNS target selection only supports A/AAAA records")
	}
	poolName = normalizeNodePoolName(poolName)
	count = normalizeDNSTargetCount(count)
	scheduleMode = normalizeDNSScheduleMode(scheduleMode)

	candidates := make([]nodeDNSTargetCandidate, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if normalizeNodePoolName(node.PoolName) != poolName {
			continue
		}
		if !isNodeSchedulableForDNS(node) || !isNodeOnlineAndOpenRestyHealthy(node) {
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
			candidates = append(candidates, nodeDNSTargetCandidate{
				NodeID:     node.NodeID,
				Content:    content,
				Weight:     normalizeNodeWeight(node.Weight),
				LastSeenAt: node.LastSeenAt,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no online public node IP is available for %s records in pool %s", recordType, poolName)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if scheduleMode == "weighted" && left.Weight != right.Weight {
			return left.Weight > right.Weight
		}
		if !left.LastSeenAt.Equal(right.LastSeenAt) {
			return left.LastSeenAt.After(right.LastSeenAt)
		}
		if left.NodeID != right.NodeID {
			return left.NodeID < right.NodeID
		}
		return left.Content < right.Content
	})
	if len(candidates) > count {
		candidates = candidates[:count]
	}
	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		targets = append(targets, candidate.Content)
	}
	return targets, nil
}

func selectHealthyNodeDNSContentLegacy(recordType string) (string, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return "", err
	}
	recordType = normalizeDNSRecordType(recordType)
	for _, node := range nodes {
		if !isNodeHealthyForDNS(node) {
			continue
		}
		ip := iputil.NormalizeIP(node.IP)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		switch recordType {
		case "A":
			if parsed.To4() != nil {
				return parsed.String(), nil
			}
		case "AAAA":
			if parsed.To4() == nil {
				return parsed.String(), nil
			}
		default:
			return "", errors.New("自动选择节点仅支持 A/AAAA 记录")
		}
	}
	return "", fmt.Errorf("没有可用于 %s 记录的在线节点公网 IP，请先部署 Agent 或手动填写 DNS 记录内容", recordType)
}

func isNodeHealthyForDNS(node *model.Node) bool {
	if node == nil {
		return false
	}
	if iputil.NormalizeIP(node.IP) == "" || !iputil.IsPublicString(node.IP) {
		return false
	}
	return isNodeOnlineAndOpenRestyHealthy(node)
}

func isNodeOnlineAndOpenRestyHealthy(node *model.Node) bool {
	if node == nil {
		return false
	}
	if node.OpenrestyStatus != "" && normalizeOpenrestyStatus(node.OpenrestyStatus) == OpenrestyStatusUnhealthy {
		return false
	}
	if IsAgentWSConnected(node.NodeID) {
		return true
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return time.Since(node.LastSeenAt) <= common.NodeOfflineThreshold
}

func isNodeEligibleForCacheOperation(node *model.Node) bool {
	if node == nil || node.DrainMode {
		return false
	}
	return isNodeOnlineAndOpenRestyHealthy(node)
}

func isNodeSchedulableForDNS(node *model.Node) bool {
	if node == nil {
		return false
	}
	if node.DrainMode {
		return false
	}
	if !node.SchedulingEnabled && (strings.TrimSpace(node.PoolName) != "" || strings.TrimSpace(node.PublicIPs) != "" || node.Weight != 0) {
		return false
	}
	// Old rows and tests may not have scheduling_enabled populated. Treat zero-value
	// nodes as schedulable for backward compatibility unless drain mode is enabled.
	return true
}

func shouldEnableDDOSProtection() bool {
	return shouldEnableDDOSProtectionWithContext(nil)
}

func shouldEnableDDOSProtectionWithContext(context *proxyRouteDNSSyncContext) bool {
	if context != nil && context.ddosActiveLoaded {
		return context.ddosActive
	}
	requestThreshold := strings.TrimSpace(common.GetOptionValue("CloudflareDDoSRequestThreshold"))
	errorRateThreshold := strings.TrimSpace(common.GetOptionValue("CloudflareDDoSErrorRateThreshold"))
	maxRequests := int64(0)
	maxErrorRate := float64(0)
	if requestThreshold != "" {
		_, _ = fmt.Sscan(requestThreshold, &maxRequests)
	}
	if errorRateThreshold != "" {
		_, _ = fmt.Sscan(errorRateThreshold, &maxErrorRate)
	}
	if maxRequests <= 0 {
		maxRequests = 20000
	}
	if maxErrorRate <= 0 {
		maxErrorRate = 30
	}
	summary, err := getRequestReportTrafficSummaryForDNSProtection(time.Now().Add(-5*time.Minute), time.Now())
	if err != nil {
		if context != nil {
			context.ddosActiveLoaded = true
			context.trafficSummaryErr = err
		}
		slog.Warn("load request reports for ddos protection failed", "error", err)
		return false
	}
	if summary == nil {
		if context != nil {
			context.ddosActiveLoaded = true
			context.ddosActive = false
		}
		return false
	}
	active := false
	if summary.RequestCount >= maxRequests {
		active = true
	} else if summary.RequestCount > 0 {
		errorRate := float64(summary.ErrorCount) / float64(summary.RequestCount) * 100
		active = errorRate >= maxErrorRate
	}
	if context != nil {
		context.ddosActiveLoaded = true
		context.ddosActive = active
	}
	return active
}

func shouldEnableCloudflareProxyForDDOS() bool {
	return shouldEnableDDOSProtection()
}
