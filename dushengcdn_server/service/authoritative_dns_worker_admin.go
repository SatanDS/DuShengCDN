package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"

	"gorm.io/gorm"
)

func ListAuthoritativeDNSWorkers() ([]DNSWorkerView, error) {
	workers, err := model.ListDNSWorkers()
	if err != nil {
		return nil, err
	}
	views := make([]DNSWorkerView, 0, len(workers))
	for _, worker := range workers {
		if worker.UninstallRequested {
			continue
		}
		views = append(views, buildDNSWorkerView(worker, false))
	}
	return views, nil
}

func ListAuthoritativeDNSGSLBSchedulingStates() (*DNSGSLBSchedulingStatesView, error) {
	var states []*model.GSLBSchedulingState
	if err := model.DB.
		Order("proxy_route_id asc, dns_record_type asc, scope_key asc").
		Find(&states).Error; err != nil {
		return nil, err
	}

	routeIDs := make([]uint, 0, len(states))
	seenRoutes := make(map[uint]struct{}, len(states))
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		if _, ok := seenRoutes[state.ProxyRouteID]; ok {
			continue
		}
		seenRoutes[state.ProxyRouteID] = struct{}{}
		routeIDs = append(routeIDs, state.ProxyRouteID)
	}

	routesByID := make(map[uint]*model.ProxyRoute, len(routeIDs))
	if len(routeIDs) > 0 {
		var routes []*model.ProxyRoute
		if err := model.DB.Where("id IN ?", routeIDs).Find(&routes).Error; err != nil {
			return nil, err
		}
		for _, route := range routes {
			if route == nil || route.ID == 0 {
				continue
			}
			routesByID[route.ID] = route
		}
	}

	view := &DNSGSLBSchedulingStatesView{
		CheckedAt: time.Now().UTC(),
		States:    make([]DNSGSLBSchedulingStateView, 0, len(states)),
	}
	for _, state := range states {
		if state == nil || state.ProxyRouteID == 0 {
			continue
		}
		view.States = append(view.States, buildDNSGSLBSchedulingStateView(state, routesByID[state.ProxyRouteID]))
	}
	view.Total = len(view.States)
	return view, nil
}

func CreateAuthoritativeDNSWorker(input DNSWorkerInput) (*DNSWorkerView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("DNS worker name cannot be empty")
	}
	if len(name) > 128 {
		return nil, errors.New("DNS worker name is too long")
	}
	remark := strings.TrimSpace(input.Remark)
	if len(remark) > 255 {
		return nil, errors.New("DNS worker remark is too long")
	}
	publicAddress, err := validateDNSWorkerPublicAddressForStorage(input.PublicAddress)
	if err != nil {
		return nil, err
	}
	token, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	workerIDSeed, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	worker := &model.DNSWorker{
		WorkerID:      "dns-" + workerIDSeed,
		Name:          name,
		Remark:        remark,
		Token:         "",
		TokenHash:     dnsWorkerTokenHash(token),
		TokenPrefix:   dnsWorkerTokenPrefix(token),
		PublicAddress: publicAddress,
		Status:        dnsWorkerStatusOffline,
	}
	if err := worker.Insert(); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("DNS worker identity collision, please retry")
		}
		return nil, err
	}
	view := buildDNSWorkerView(worker, true)
	view.Token = token
	return ptrDNSWorkerView(view), nil
}

func RotateAuthoritativeDNSWorkerToken(id uint) (*DNSWorkerView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	token, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	worker.Token = ""
	worker.TokenHash = dnsWorkerTokenHash(token)
	worker.TokenPrefix = dnsWorkerTokenPrefix(token)
	worker.TokenRevokedAt = nil
	if err := model.DB.Model(worker).Select("token", "token_hash", "token_prefix", "token_revoked_at").Updates(worker).Error; err != nil {
		return nil, err
	}
	view := buildDNSWorkerView(worker, true)
	view.Token = token
	return ptrDNSWorkerView(view), nil
}

func RevokeAuthoritativeDNSWorkerToken(id uint) (*DNSWorkerView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	worker.Token = ""
	worker.TokenHash = ""
	worker.TokenPrefix = ""
	worker.TokenRevokedAt = &now
	if err := model.DB.Model(worker).Select("token", "token_hash", "token_prefix", "token_revoked_at").Updates(worker).Error; err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func UpdateAuthoritativeDNSWorker(id uint, input DNSWorkerMutationInput) (*DNSWorkerView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	remark := strings.TrimSpace(input.Remark)
	if len(remark) > 255 {
		return nil, errors.New("DNS worker remark is too long")
	}
	worker.Remark = remark
	if err := model.DB.Model(worker).Select("remark").Updates(worker).Error; err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func DeleteAuthoritativeDNSWorker(id uint) error {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return err
	}
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return err
	}
	if !worker.UninstallSupported {
		return errors.New("该 DNS 响应端当前版本不支持远程卸载，请先强制更新一次，或登录机器手动执行 uninstall-dns-worker.sh")
	}
	now := time.Now()
	worker.UninstallRequested = true
	worker.UninstallRequestedAt = &now
	worker.Status = dnsWorkerStatusOffline
	if err := model.DB.Model(worker).Select(
		"uninstall_requested",
		"uninstall_requested_at",
		"status",
	).Updates(worker).Error; err != nil {
		return err
	}
	return nil
}

func deleteDNSWorkerRuntimeDataWithDB(db *gorm.DB, workerID string) error {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil
	}
	if err := db.Where("worker_id = ?", workerID).Delete(&model.DNSQueryRollup{}).Error; err != nil {
		return err
	}
	if err := db.Where("worker_id = ?", workerID).Delete(&model.DNSWorkerNodeProbe{}).Error; err != nil {
		return err
	}
	return nil
}

func RequestAuthoritativeDNSWorkerUpdate(id uint, input DNSWorkerUpdateInput) (*DNSWorkerView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	worker, err := model.GetDNSWorkerByID(id)
	if err != nil {
		return nil, err
	}
	channel := normalizeReleaseChannel(input.Channel)
	tagName := strings.TrimSpace(input.TagName)
	if tagName != "" {
		release, releaseErr := fetchGitHubReleaseByTag(context.Background(), common.ServerUpdateRepo, tagName)
		if releaseErr != nil {
			return nil, releaseErr
		}
		if channel == ReleaseChannelPreview && !release.Prerelease {
			return nil, errors.New("指定版本不是 preview 发布")
		}
		if channel == ReleaseChannelStable && release.Prerelease {
			return nil, errors.New("正式版更新不能选择 preview 发布")
		}
	}
	mode, message, nodeID := dispatchDNSWorkerUpdateViaAgent(worker, channel, tagName)
	worker.UpdateRequested = true
	worker.UpdateChannel = channel.String()
	worker.UpdateTag = tagName
	now := time.Now()
	if mode != "" {
		worker.UpdateDispatchMode = mode
		worker.UpdateDispatchMessage = message
		worker.UpdateDispatchedNodeID = nodeID
		worker.UpdateDispatchedAt = &now
	}
	if err = model.DB.Model(worker).Select(
		"update_requested",
		"update_channel",
		"update_tag",
		"update_dispatch_mode",
		"update_dispatch_message",
		"update_dispatched_node_id",
		"update_dispatched_at",
	).Updates(worker).Error; err != nil {
		return nil, err
	}
	return ptrDNSWorkerView(buildDNSWorkerView(worker, false)), nil
}

func dispatchDNSWorkerUpdateViaAgent(worker *model.DNSWorker, channel ReleaseChannel, tagName string) (string, string, string) {
	node := findDNSWorkerHostNode(worker)
	if node == nil {
		return "worker_heartbeat", "未匹配到同机 Agent，已回退为 DNS 响应端心跳更新。", ""
	}
	request := buildAgentDNSWorkerUpdateRequest(worker, channel, tagName)
	if SendAgentWSDNSWorkerUpdate(node.NodeID, &request) {
		return "agent_ws", fmt.Sprintf("已通过同机 Agent %s 立即下发 DNS Worker 更新。", strings.TrimSpace(node.Name)), node.NodeID
	}
	return "agent_heartbeat", fmt.Sprintf("已匹配同机 Agent %s，等待该 Agent 下一次心跳执行 DNS Worker 更新。", strings.TrimSpace(node.Name)), node.NodeID
}

func pendingAgentDNSWorkerUpdatesForNode(node *model.Node) []AgentDNSWorkerUpdateRequest {
	return newAgentHeartbeatDNSWorkerContext().pendingUpdatesForNode(node)
}

func (ctx *agentHeartbeatDNSWorkerContext) pendingUpdatesForNode(node *model.Node) []AgentDNSWorkerUpdateRequest {
	if node == nil {
		return nil
	}
	if ctx == nil || ctx.workersErr != nil || len(ctx.workers) == 0 {
		return nil
	}
	updates := make([]AgentDNSWorkerUpdateRequest, 0, 1)
	for _, worker := range ctx.workers {
		if worker == nil || !worker.UpdateRequested {
			continue
		}
		if strings.TrimSpace(worker.UpdateDispatchMode) != "agent_heartbeat" {
			continue
		}
		if strings.TrimSpace(worker.UpdateDispatchedNodeID) != strings.TrimSpace(node.NodeID) {
			continue
		}
		updates = append(updates, buildAgentDNSWorkerUpdateRequest(worker, normalizeReleaseChannel(worker.UpdateChannel), strings.TrimSpace(worker.UpdateTag)))
		markDNSWorkerUpdateDispatched(worker, "agent_heartbeat_sent", fmt.Sprintf("已随 Agent %s 心跳返回 DNS Worker 更新任务。", strings.TrimSpace(node.Name)), node.NodeID)
	}
	return updates
}

func buildAgentDNSWorkerUpdateRequest(worker *model.DNSWorker, channel ReleaseChannel, tagName string) AgentDNSWorkerUpdateRequest {
	request := AgentDNSWorkerUpdateRequest{
		Repo:    common.ServerUpdateRepo,
		Channel: channel.String(),
		TagName: strings.TrimSpace(tagName),
	}
	if worker != nil {
		request.WorkerID = strings.TrimSpace(worker.WorkerID)
		request.WorkerName = strings.TrimSpace(worker.Name)
	}
	return request
}

func markDNSWorkerUpdateDispatched(worker *model.DNSWorker, mode string, message string, nodeID string) {
	if worker == nil {
		return
	}
	now := time.Now()
	updates := map[string]any{
		"update_dispatch_mode":      strings.TrimSpace(mode),
		"update_dispatch_message":   truncateForDatabase(strings.TrimSpace(message), 16000),
		"update_dispatched_node_id": strings.TrimSpace(nodeID),
		"update_dispatched_at":      &now,
	}
	if err := model.DB.Model(worker).Updates(updates).Error; err != nil {
		return
	}
	worker.UpdateDispatchMode = updates["update_dispatch_mode"].(string)
	worker.UpdateDispatchMessage = updates["update_dispatch_message"].(string)
	worker.UpdateDispatchedNodeID = updates["update_dispatched_node_id"].(string)
	worker.UpdateDispatchedAt = &now
}

func findDNSWorkerHostNode(worker *model.DNSWorker) *model.Node {
	workerIPs := dnsWorkerHostCandidateIPs(worker)
	if len(workerIPs) == 0 {
		return nil
	}
	nodes, err := model.ListNodes()
	if err != nil {
		return nil
	}
	for _, node := range nodes {
		for _, workerIP := range workerIPs {
			if nodeMatchesDNSWorkerIP(node, workerIP) {
				return node
			}
		}
	}
	return nil
}

func dnsWorkerHostCandidateIPs(worker *model.DNSWorker) []string {
	if worker == nil {
		return nil
	}
	candidates := []string{dnsWorkerPublicAddressIP(worker), worker.LastRemoteIP}
	result := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		ip := iputil.NormalizeIP(candidate)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		result = append(result, ip)
	}
	return result
}

func dnsWorkerPublicAddressIP(worker *model.DNSWorker) string {
	if worker == nil {
		return ""
	}
	value := strings.TrimSpace(worker.PublicAddress)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.Trim(value, "[]")
	}
	return iputil.NormalizeIP(value)
}

func nodeMatchesDNSWorkerIP(node *model.Node, workerIP string) bool {
	workerIP = iputil.NormalizeIP(workerIP)
	if node == nil || workerIP == "" {
		return false
	}
	if iputil.NormalizeIP(node.IP) == workerIP {
		return true
	}
	for _, value := range resolveNodePublicIPs(node) {
		if iputil.NormalizeIP(value) == workerIP {
			return true
		}
	}
	return false
}

func AuthenticateDNSWorkerToken(token string) (*model.DNSWorker, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing DNS Worker Token")
	}
	prefix := dnsWorkerTokenPrefix(token)
	hash := dnsWorkerTokenHash(token)
	workers, err := model.ListDNSWorkersByTokenPrefix(prefix)
	if err != nil {
		return nil, err
	}
	for _, worker := range workers {
		if worker == nil || worker.TokenRevokedAt != nil || strings.TrimSpace(worker.TokenHash) == "" {
			continue
		}
		if constantTimeTokenEqual(worker.TokenHash, hash) {
			return worker, nil
		}
	}
	legacyWorker, legacyErr := model.GetDNSWorkerByToken(token)
	if legacyErr == nil && legacyWorker != nil && legacyWorker.TokenRevokedAt == nil {
		legacyWorker.TokenHash = hash
		legacyWorker.TokenPrefix = prefix
		legacyWorker.Token = ""
		_ = model.DB.Model(legacyWorker).Select("token", "token_hash", "token_prefix").Updates(legacyWorker).Error
		return legacyWorker, nil
	}
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return nil, legacyErr
	}
	return nil, errors.New("invalid DNS Worker Token")
}

func buildDNSWorkerView(worker *model.DNSWorker, includeToken bool) DNSWorkerView {
	if worker == nil {
		return DNSWorkerView{}
	}
	now := time.Now().UTC()
	probeResults := decodeDNSWorkerProbeResults(worker.LastProbeResult)
	probeAt := normalizeDNSWorkerProbeAt(worker.LastProbeAt, now, worker.UpdatedAt, worker.CreatedAt)
	probeState := evaluateDNSWorkerProbeState(now, probeAt, probeResults)
	view := DNSWorkerView{
		ID:                       worker.ID,
		WorkerID:                 worker.WorkerID,
		Name:                     worker.Name,
		Remark:                   worker.Remark,
		TokenPrefix:              worker.TokenPrefix,
		TokenRevokedAt:           worker.TokenRevokedAt,
		PublicAddress:            worker.PublicAddress,
		Version:                  worker.Version,
		Status:                   normalizeDNSWorkerStatus(worker.Status),
		LastSnapshotVersion:      worker.LastSnapshotVersion,
		LastSnapshotAt:           worker.LastSnapshotAt,
		LastSeenAt:               worker.LastSeenAt,
		LastHeartbeatAt:          worker.LastHeartbeatAt,
		LastRemoteIP:             worker.LastRemoteIP,
		LastRollupAt:             worker.LastRollupAt,
		LastRollupCount:          worker.LastRollupCount,
		LastError:                worker.LastError,
		GeoIPEnabled:             worker.GeoIPEnabled,
		GeoIPDatabasePath:        worker.GeoIPDatabasePath,
		ASNDatabasePath:          worker.ASNDatabasePath,
		GeoIPLastError:           worker.GeoIPLastError,
		ASNLastError:             worker.ASNLastError,
		GeoIPDatabaseType:        worker.GeoIPDatabaseType,
		ASNDatabaseType:          worker.ASNDatabaseType,
		GeoIPCountryEnabled:      worker.GeoIPCountryEnabled,
		GeoIPASNEnabled:          worker.GeoIPASNEnabled,
		GeoIPOperatorEnabled:     worker.GeoIPOperatorEnabled,
		OperatorCIDRDatabasePath: worker.OperatorCIDRDatabasePath,
		OperatorCIDRLastError:    worker.OperatorCIDRLastError,
		UpdateRequested:          worker.UpdateRequested,
		UpdateChannel:            normalizeReleaseChannel(worker.UpdateChannel).String(),
		UpdateTag:                worker.UpdateTag,
		UpdateSupported:          worker.UpdateSupported,
		LastUpdateSupportedAt:    worker.LastUpdateSupportedAt,
		UpdateDispatchMode:       worker.UpdateDispatchMode,
		UpdateDispatchMessage:    worker.UpdateDispatchMessage,
		UpdateDispatchedAt:       worker.UpdateDispatchedAt,
		UpdateDispatchedNodeID:   worker.UpdateDispatchedNodeID,
		UninstallSupported:       worker.UninstallSupported,
		LastUninstallSupportedAt: worker.LastUninstallSupportedAt,
		UninstallRequested:       worker.UninstallRequested,
		UninstallRequestedAt:     worker.UninstallRequestedAt,
		LastProbeAt:              probeAt,
		LastProbeQuery:           worker.LastProbeQuery,
		LastProbeResults:         probeResults,
		ProbeStatus:              probeState.status,
		ProbeHealthy:             probeState.healthy,
		ProbeAgeSeconds:          probeState.ageSeconds,
		ProbeMessage:             probeState.message,
		CreatedAt:                worker.CreatedAt,
		UpdatedAt:                worker.UpdatedAt,
	}
	if includeToken {
		view.Token = worker.Token
	}
	return view
}

func dnsWorkerTokenHash(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func dnsWorkerTokenPrefix(token string) string {
	token = strings.TrimSpace(token)
	if len(token) > 12 {
		return token[:12]
	}
	return token
}

func validateAuthoritativeDNSReadyWorkers() error {
	stats, err := authoritativeDNSMigrationWorkerStats()
	if err != nil {
		return err
	}
	if stats.online == 0 {
		return errors.New("没有在线 DNS 响应端")
	}
	if stats.publicReachable == 0 {
		return errors.New("在线 DNS 响应端尚未通过公网 UDP/TCP 53 探测")
	}
	if stats.ready == 0 {
		return errors.New("公网可达 DNS 响应端尚未拉取未过期的解析配置")
	}
	if stats.publicReachableWithoutFresh > 0 {
		return errors.New("部分公网可达 DNS 响应端尚未拉取未过期的解析配置")
	}
	if stats.readySnapshotVersionCount > 1 {
		return errors.New("公网可达 DNS 响应端的解析配置版本不一致")
	}
	return nil
}

func evaluateAuthoritativeDNSWorkerReadiness(now time.Time, snapshotMaxAge time.Duration, worker *model.DNSWorker) authoritativeDNSWorkerReadiness {
	if worker == nil || normalizeDNSWorkerStatus(worker.Status) != dnsWorkerStatusOnline {
		return authoritativeDNSWorkerReadiness{}
	}
	readiness := authoritativeDNSWorkerReadiness{online: true}
	probeState := evaluateDNSWorkerProbeState(now, normalizeDNSWorkerProbeAt(worker.LastProbeAt, now, worker.UpdatedAt, worker.CreatedAt), decodeDNSWorkerProbeResults(worker.LastProbeResult))
	readiness.publicReachable = probeState.healthy
	readiness.freshSnapshot = hasFreshAuthoritativeDNSWorkerSnapshot(now, snapshotMaxAge, worker)
	readiness.ready = readiness.publicReachable && readiness.freshSnapshot
	return readiness
}

func hasFreshAuthoritativeDNSWorkerSnapshot(now time.Time, snapshotMaxAge time.Duration, worker *model.DNSWorker) bool {
	if worker == nil || worker.LastSnapshotAt == nil || strings.TrimSpace(worker.LastSnapshotVersion) == "" {
		return false
	}
	if snapshotMaxAge <= 0 {
		snapshotMaxAge = defaultDNSSnapshotMaxAge
	}
	snapshotAt := normalizeDNSWorkerSnapshotAt(worker.LastSnapshotAt, now, worker.UpdatedAt, worker.CreatedAt)
	if snapshotAt == nil {
		return false
	}
	return now.Sub(snapshotAt.UTC()) <= snapshotMaxAge
}

func buildDNSGSLBSchedulingStateView(state *model.GSLBSchedulingState, route *model.ProxyRoute) DNSGSLBSchedulingStateView {
	if state == nil {
		return DNSGSLBSchedulingStateView{}
	}
	recordType := normalizeDNSRecordType(state.DNSRecordType)
	view := DNSGSLBSchedulingStateView{
		ID:              state.ID,
		ProxyRouteID:    state.ProxyRouteID,
		RecordType:      recordType,
		ScopeKey:        normalizeDNSSourceScope(state.ScopeKey),
		SelectedTargets: decodeGSLBTargetList(state.SelectedTargets),
		DesiredTargets:  decodeGSLBTargetList(state.DesiredTargets),
		UnhealthyCount:  normalizeDebounceCounter(state.UnhealthyCount),
		RecoveryCount:   normalizeDebounceCounter(state.RecoveryCount),
		LastReason:      state.LastReason,
		LastChangedAt:   state.LastChangedAt,
		LastEvaluatedAt: state.LastEvaluatedAt,
		CreatedAt:       state.CreatedAt,
		UpdatedAt:       state.UpdatedAt,
	}
	if route == nil {
		view.Status = "orphaned"
		return view
	}
	domains, err := decodeStoredDomains(route.Domains, route.Domain)
	if err != nil {
		domains = normalizeStringList([]string{route.Domain})
	}
	view.Domains = domains
	if len(domains) > 0 {
		view.PrimaryDomain = domains[0]
	} else {
		view.PrimaryDomain = normalizeDNSRecordName(route.Domain)
	}
	view.SiteName = normalizeProxyRouteSiteNameInput(route, route.SiteName, view.PrimaryDomain)
	view.RouteEnabled = route.Enabled
	view.RouteAuthoritative = route.DNSProviderMode == DNSProviderModeAuthoritative
	view.RouteGSLBEnabled = route.GSLBEnabled
	view.RouteRecordType = normalizeDNSRecordType(route.DNSRecordType)
	view.Status = evaluateDNSGSLBSchedulingStateStatus(view)
	return view
}

func evaluateDNSGSLBSchedulingStateStatus(view DNSGSLBSchedulingStateView) string {
	if !view.RouteEnabled || !view.RouteGSLBEnabled {
		return "inactive"
	}
	if view.RouteRecordType != "" && view.RouteRecordType != view.RecordType {
		return "stale"
	}
	if len(view.SelectedTargets) == 0 {
		return "empty"
	}
	if len(view.DesiredTargets) > 0 && !sameStringSet(view.SelectedTargets, view.DesiredTargets) {
		return "debouncing"
	}
	return "active"
}

func normalizeDNSWorkerStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dnsWorkerStatusOnline:
		return dnsWorkerStatusOnline
	default:
		return dnsWorkerStatusOffline
	}
}

func normalizeDNSWorkerSnapshotAt(snapshotAt *time.Time, now time.Time, fallbacks ...time.Time) *time.Time {
	if snapshotAt == nil {
		return nil
	}
	normalizedNow := now.UTC()
	normalized := snapshotAt.UTC()
	if !normalized.After(normalizedNow) {
		return &normalized
	}
	for _, fallback := range fallbacks {
		if fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return &normalized
		}
	}
	normalized = normalizedNow
	return &normalized
}

func normalizeDNSWorkerProbeAt(probeAt *time.Time, now time.Time, fallbacks ...time.Time) *time.Time {
	if probeAt == nil {
		return nil
	}
	normalized := normalizeDNSWorkerCheckedAt(probeAt, now, fallbacks...)
	return &normalized
}

func normalizeDNSWorkerCheckedAt(checkedAt *time.Time, now time.Time, fallbacks ...time.Time) time.Time {
	if checkedAt == nil {
		return now.UTC()
	}
	normalizedNow := now.UTC()
	normalized := checkedAt.UTC()
	if !normalized.After(normalizedNow) {
		return normalized
	}
	for _, fallback := range fallbacks {
		if fallback.IsZero() {
			continue
		}
		normalized = fallback.UTC()
		if !normalized.After(normalizedNow) {
			return normalized
		}
	}
	return normalizedNow
}

func ptrDNSWorkerView(view DNSWorkerView) *DNSWorkerView {
	return &view
}
