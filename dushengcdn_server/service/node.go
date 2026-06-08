package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/geoip"
	"dushengcdn/utils/geoip/iputil"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
)

type NodeInput struct {
	Name              string   `json:"name"`
	IP                string   `json:"ip"`
	PoolName          string   `json:"pool_name"`
	Tags              []string `json:"tags"`
	Weight            int      `json:"weight"`
	PublicIPs         []string `json:"public_ips"`
	SchedulingEnabled *bool    `json:"scheduling_enabled"`
	DrainMode         bool     `json:"drain_mode"`
	AutoUpdateEnabled bool     `json:"auto_update_enabled"`
	GeoName           string   `json:"geo_name"`
	GeoLatitude       *float64 `json:"geo_latitude"`
	GeoLongitude      *float64 `json:"geo_longitude"`
	GeoManualOverride bool     `json:"geo_manual_override"`
}

type NodeAgentUpdateInput struct {
	Channel string `json:"channel"`
	TagName string `json:"tag_name"`
}

type NodeAgentReleaseInfo struct {
	TagName          string `json:"tag_name"`
	Body             string `json:"body"`
	HTMLURL          string `json:"html_url"`
	PublishedAt      string `json:"published_at"`
	CurrentVersion   string `json:"current_version"`
	HasUpdate        bool   `json:"has_update"`
	Channel          string `json:"channel"`
	Prerelease       bool   `json:"prerelease"`
	UpdateRequested  bool   `json:"update_requested"`
	RequestedChannel string `json:"requested_channel"`
	RequestedTag     string `json:"requested_tag"`
}

type NodeBootstrapView struct {
	DiscoveryToken string `json:"discovery_token"`
}

type NodeDeleteResult struct {
	NodeID                  string `json:"node_id"`
	Name                    string `json:"name"`
	UninstallAgentRequested bool   `json:"uninstall_agent_requested"`
	UninstallAgentMessage   string `json:"uninstall_agent_message"`
}

type AgentRegistrationResponse struct {
	NodeID     string `json:"node_id"`
	AgentToken string `json:"agent_token"`
	Name       string `json:"name"`
}

func CreateNode(input NodeInput) (*NodeView, error) {
	normalized, err := normalizeNodeInputV2(input, true)
	if normalized.Name == "" {
		return nil, errors.New("节点名不能为空")
	}
	node := &model.Node{
		Name:              normalized.Name,
		IP:                normalized.IP,
		PoolName:          normalized.PoolName,
		Tags:              normalized.TagsJSON,
		Weight:            normalized.Weight,
		PublicIPs:         normalized.PublicIPsJSON,
		SchedulingEnabled: normalized.SchedulingEnabled,
		DrainMode:         normalized.DrainMode,
		GeoName:           normalized.GeoName,
		GeoLatitude:       normalized.GeoLatitude,
		GeoLongitude:      normalized.GeoLongitude,
		GeoManualOverride: normalized.GeoManualOverride,
		AgentVersion:      "",
		NginxVersion:      "",
		Status:            NodeStatusPending,
		AutoUpdateEnabled: input.AutoUpdateEnabled,
	}
	node.NodeID, err = newServerNodeID()
	if err != nil {
		return nil, err
	}
	node.AgentToken, err = newRandomToken()
	if err != nil {
		return nil, err
	}
	if !node.GeoManualOverride {
		applyGeoInfoFromIP(node, node.IP)
	}
	if err := withCommercialResourceCreation("node", func(tx *gorm.DB) error {
		return node.InsertWithDB(tx)
	}); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("节点标识生成冲突，请重试")
		}
		return nil, err
	}
	refreshAgentTokenCache(node)
	slog.Info("node created", "name", node.Name, "node_id", node.NodeID)
	return buildNodeView(node), nil
}

func UpdateNode(id uint, input NodeInput) (*NodeView, error) {
	normalized, err := normalizeNodeInputV2(input, true)
	if normalized.Name == "" {
		return nil, errors.New("节点名不能为空")
	}
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	if input.SchedulingEnabled == nil {
		normalized.SchedulingEnabled = node.SchedulingEnabled
	}
	node.Name = normalized.Name
	node.IP = normalized.IP
	node.PoolName = normalized.PoolName
	node.Tags = normalized.TagsJSON
	node.Weight = normalized.Weight
	node.PublicIPs = normalized.PublicIPsJSON
	node.SchedulingEnabled = normalized.SchedulingEnabled
	node.DrainMode = normalized.DrainMode
	node.GeoName = normalized.GeoName
	node.GeoLatitude = normalized.GeoLatitude
	node.GeoLongitude = normalized.GeoLongitude
	node.GeoManualOverride = normalized.GeoManualOverride
	node.AutoUpdateEnabled = input.AutoUpdateEnabled
	if !node.GeoManualOverride {
		applyGeoInfoFromIP(node, strings.TrimSpace(node.IP))
	}
	if err = node.Update(); err != nil {
		return nil, err
	}
	refreshAgentTokenCache(node)
	slog.Info("node updated", "name", node.Name, "node_id", node.NodeID)
	return buildNodeView(node), nil
}

func DeleteNode(id uint) (*NodeDeleteResult, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	result := &NodeDeleteResult{
		NodeID: node.NodeID,
		Name:   node.Name,
	}
	if SendAgentWSUninstallAgent(node.NodeID) {
		result.UninstallAgentRequested = true
		result.UninstallAgentMessage = "已向在线节点下发 Agent 卸载指令"
	} else {
		result.UninstallAgentMessage = "节点未通过 WebSocket 在线，已删除面板记录但未能远程卸载 Agent"
	}
	slog.Info("node deleted", "name", node.Name, "node_id", node.NodeID)
	if err := node.Delete(); err != nil {
		return nil, err
	}
	invalidateAgentTokenCache(node.AgentToken)
	return result, nil
}

func GetNodeAgentRelease(ctx context.Context, id uint, channel string) (*NodeAgentReleaseInfo, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	release, err := fetchLatestGitHubRelease(ctx, common.AgentUpdateRepo, normalizeReleaseChannel(channel))
	if err != nil {
		return nil, err
	}
	return buildNodeAgentReleaseView(node, release, normalizeReleaseChannel(channel)), nil
}

func RequestNodeAgentUpdate(id uint, input NodeAgentUpdateInput) (*NodeView, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	channel := normalizeReleaseChannel(input.Channel)
	tagName := strings.TrimSpace(input.TagName)
	if tagName != "" {
		release, releaseErr := fetchGitHubReleaseByTag(context.Background(), common.AgentUpdateRepo, tagName)
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
	node.UpdateRequested = true
	node.UpdateChannel = channel.String()
	node.UpdateTag = tagName
	if err = model.DB.Model(node).Select("update_requested", "update_channel", "update_tag").Updates(node).Error; err != nil {
		return nil, err
	}
	refreshAgentTokenCache(node)
	if SendAgentWSSettings(node.NodeID, buildAgentSettings(node, true, channel.String(), tagName, node.RestartOpenrestyRequested)) {
		slog.Debug("agent manual update pushed via ws", "node_id", node.NodeID, "channel", channel.String(), "tag", tagName)
	} else {
		slog.Debug("agent manual update waiting for next heartbeat", "node_id", node.NodeID, "channel", channel.String(), "tag", tagName)
	}
	slog.Info("agent manual update requested", "node_id", node.NodeID, "name", node.Name, "channel", channel.String(), "tag", tagName)
	return buildNodeView(node), nil
}

func RequestNodeOpenrestyRestart(id uint) (*NodeView, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	node.RestartOpenrestyRequested = true
	if err = model.DB.Model(node).Select("restart_openresty_requested").Updates(node).Error; err != nil {
		return nil, err
	}
	refreshAgentTokenCache(node)
	slog.Info("openresty restart requested", "node_id", node.NodeID, "name", node.Name)
	return buildNodeView(node), nil
}

func RequestNodeForceSync(id uint) (*NodeView, error) {
	node, err := model.GetNodeByID(id)
	if err != nil {
		return nil, err
	}
	activeConfig, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil {
		return nil, errors.New("无法获取当前激活的配置版本：" + err.Error())
	}
	if !SendAgentWSForceSyncConfig(node.NodeID, activeConfig) {
		return nil, errors.New("节点不在线或通过 WebSocket 发送同步指令失败")
	}
	slog.Info("force sync requested via ws", "node_id", node.NodeID, "name", node.Name)
	return buildNodeView(node), nil
}

func AuthenticateAgentToken(token string) (*model.Node, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("缺少 Agent Token")
	}
	return authenticateAgentTokenWithCache(token)
}

func IsLegacyGlobalAgentToken(token string) bool {
	if !common.AgentLegacyGlobalTokenEnabled {
		return false
	}
	token = strings.TrimSpace(token)
	legacyToken := strings.TrimSpace(common.AgentToken)
	return constantTimeTokenEqual(token, legacyToken)
}

func IsConfiguredLegacyGlobalAgentToken(token string) bool {
	token = strings.TrimSpace(token)
	legacyToken := strings.TrimSpace(common.AgentToken)
	return constantTimeTokenEqual(token, legacyToken)
}

func AuthenticateLegacyAgentTokenForNode(token string, nodeID string) (*model.Node, error) {
	if !common.AgentLegacyGlobalTokenEnabled {
		return nil, errors.New("legacy global Agent Token compatibility is disabled; set DUSHENGCDN_AGENT_LEGACY_GLOBAL_TOKEN_ENABLED=true temporarily and migrate Agents to node-specific agent_token or discovery_token")
	}
	if !IsLegacyGlobalAgentToken(token) {
		return nil, errors.New("旧版全局 Agent Token 无效")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("缺少 node_id")
	}
	node, err := model.GetNodeByNodeID(nodeID)
	if err != nil {
		return nil, err
	}
	nodeAgentToken := strings.TrimSpace(node.AgentToken)
	if nodeAgentToken != "" && nodeAgentToken != strings.TrimSpace(common.AgentToken) {
		return nil, errors.New("节点已切换为专属 Agent Token")
	}
	slog.Warn("legacy global Agent Token accepted; migrate this Agent to its node-specific agent_token",
		"node_id", node.NodeID,
		"name", node.Name,
	)
	return node, nil
}

func ValidateDiscoveryToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("缺少 Discovery Token")
	}
	discoveryToken, err := EnsureGlobalDiscoveryToken()
	if err != nil {
		return err
	}
	if !constantTimeTokenEqual(token, discoveryToken) {
		return errors.New("Discovery Token 无效")
	}
	return nil
}

func constantTimeTokenEqual(a string, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func EnsureGlobalDiscoveryToken() (string, error) {
	common.OptionMapRWMutex.RLock()
	needsInit := common.OptionMap == nil
	common.OptionMapRWMutex.RUnlock()
	if needsInit {
		model.InitOptionMap()
	}
	token := strings.TrimSpace(common.GetOptionValue("AgentDiscoveryToken"))
	if token != "" {
		return token, nil
	}
	token, err := newRandomToken()
	if err != nil {
		return "", err
	}
	if err = model.UpdateOption("AgentDiscoveryToken", token); err != nil {
		return "", err
	}
	return token, nil
}

func GetNodeBootstrapView() (*NodeBootstrapView, error) {
	token, err := EnsureGlobalDiscoveryToken()
	if err != nil {
		return nil, err
	}
	return &NodeBootstrapView{DiscoveryToken: token}, nil
}

func RotateGlobalDiscoveryToken() (*NodeBootstrapView, error) {
	token, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	if err = model.UpdateOption("AgentDiscoveryToken", token); err != nil {
		return nil, err
	}
	return &NodeBootstrapView{DiscoveryToken: token}, nil
}

func buildNodeView(node *model.Node) *NodeView {
	status := computeNodeStatus(node)
	view := &NodeView{
		ID:                        node.ID,
		NodeID:                    node.NodeID,
		Name:                      node.Name,
		IP:                        node.IP,
		PoolName:                  normalizeNodePoolName(node.PoolName),
		Tags:                      decodeStoredStringList(node.Tags),
		Weight:                    normalizeNodeWeight(node.Weight),
		PublicIPs:                 resolveNodePublicIPs(node),
		SchedulingEnabled:         node.SchedulingEnabled,
		DrainMode:                 node.DrainMode,
		GeoName:                   strings.TrimSpace(node.GeoName),
		GeoLatitude:               node.GeoLatitude,
		GeoLongitude:              node.GeoLongitude,
		GeoManualOverride:         node.GeoManualOverride,
		AgentToken:                node.AgentToken,
		UpdateChannel:             strings.TrimSpace(node.UpdateChannel),
		UpdateTag:                 strings.TrimSpace(node.UpdateTag),
		RestartOpenrestyRequested: node.RestartOpenrestyRequested,
		AgentVersion:              node.AgentVersion,
		NginxVersion:              node.NginxVersion,
		OpenrestyStatus:           normalizeOpenrestyStatus(node.OpenrestyStatus),
		OpenrestyMessage:          strings.TrimSpace(node.OpenrestyMessage),
		Status:                    status,
		CurrentVersion:            node.CurrentVersion,
		CurrentChecksum:           node.CurrentChecksum,
		LastSeenAt:                nodeViewLastSeenAt(node),
		LastError:                 node.LastError,
		CreatedAt:                 node.CreatedAt,
		UpdatedAt:                 node.UpdatedAt,
		AutoUpdateEnabled:         node.AutoUpdateEnabled,
		UpdateRequested:           node.UpdateRequested,
	}
	if view.UpdateChannel == "" {
		view.UpdateChannel = ReleaseChannelStable.String()
	}
	applyNodeViewTargetConfig(view, node)
	return view
}

func applyNodeViewTargetConfig(view *NodeView, node *model.Node) {
	if view == nil || node == nil {
		return
	}
	poolName := normalizeNodePoolName(node.PoolName)
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	view.TargetConfigPool = poolName
	version, artifact, err := getActiveConfigVersionArtifactForNode(node)
	if err != nil {
		view.TargetConfigAvailable = false
		view.ConfigInSync = strings.TrimSpace(node.CurrentVersion) == ""
		return
	}
	view.TargetConfigAvailable = true
	view.TargetConfigVersion = version.Version
	view.TargetConfigChecksum = artifact.Checksum
	currentChecksum := strings.TrimSpace(node.CurrentChecksum)
	if currentChecksum != "" {
		view.ConfigInSync = currentChecksum == artifact.Checksum
		return
	}
	view.ConfigInSync = strings.TrimSpace(node.CurrentVersion) == version.Version
}

func nodeViewLastSeenAt(node *model.Node) any {
	if node != nil && IsAgentWSConnected(node.NodeID) {
		return AgentWSConnectedLastSeenValue
	}
	if node == nil {
		return time.Time{}
	}
	return node.LastSeenAt
}

type normalizedNodeInput struct {
	Name              string
	IP                string
	PoolName          string
	TagsJSON          string
	Weight            int
	PublicIPsJSON     string
	SchedulingEnabled bool
	DrainMode         bool
	GeoName           string
	GeoLatitude       *float64
	GeoLongitude      *float64
	GeoManualOverride bool
}

func normalizeNodeInputV2(input NodeInput, defaultSchedulingEnabled bool) (normalizedNodeInput, error) {
	name, ip, geoName, geoLatitude, geoLongitude, geoManualOverride, err := normalizeNodeInput(input)
	if err != nil {
		return normalizedNodeInput{}, err
	}
	tags := normalizeStringList(input.Tags)
	publicIPs, err := normalizeNodePublicIPs(ip, input.PublicIPs)
	if err != nil {
		return normalizedNodeInput{}, err
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return normalizedNodeInput{}, err
	}
	publicIPsJSON, err := json.Marshal(publicIPs)
	if err != nil {
		return normalizedNodeInput{}, err
	}
	schedulingEnabled := defaultSchedulingEnabled
	if input.SchedulingEnabled != nil {
		schedulingEnabled = *input.SchedulingEnabled
	}
	return normalizedNodeInput{
		Name:              name,
		IP:                ip,
		PoolName:          normalizeNodePoolName(input.PoolName),
		TagsJSON:          string(tagsJSON),
		Weight:            normalizeNodeWeight(input.Weight),
		PublicIPsJSON:     string(publicIPsJSON),
		SchedulingEnabled: schedulingEnabled,
		DrainMode:         input.DrainMode,
		GeoName:           geoName,
		GeoLatitude:       geoLatitude,
		GeoLongitude:      geoLongitude,
		GeoManualOverride: geoManualOverride,
	}, nil
}

func normalizeNodePoolName(raw string) string {
	poolName := strings.ToLower(strings.TrimSpace(raw))
	if poolName == "" {
		return "default"
	}
	if len(poolName) > 64 {
		return poolName[:64]
	}
	return poolName
}

func normalizeNodeWeight(weight int) int {
	if weight <= 0 {
		return 100
	}
	if weight > 1000 {
		return 1000
	}
	return weight
}

func normalizeNodePublicIPs(primaryIP string, values []string) ([]string, error) {
	candidates := make([]string, 0, len(values)+1)
	candidates = append(candidates, values...)
	if strings.TrimSpace(primaryIP) != "" {
		candidates = append(candidates, primaryIP)
	}
	result := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, value := range candidates {
		ip := iputil.NormalizeIP(value)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			return nil, errors.New("public_ips contains invalid IP")
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		result = append(result, ip)
	}
	return result, nil
}

func decodeStoredStringList(raw string) []string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return []string{}
	}
	return normalizeStringList(values)
}

func resolveNodePublicIPs(node *model.Node) []string {
	if node == nil {
		return []string{}
	}
	values := decodeStoredStringList(node.PublicIPs)
	if len(values) > 0 {
		return values
	}
	ips, err := normalizeNodePublicIPs(node.IP, nil)
	if err != nil {
		return []string{}
	}
	return ips
}

func normalizeNodeInput(input NodeInput) (string, string, string, *float64, *float64, bool, error) {
	name := strings.TrimSpace(input.Name)
	ip := strings.TrimSpace(input.IP)
	geoName := strings.TrimSpace(input.GeoName)
	manualOverride := input.GeoManualOverride || geoName != "" || input.GeoLatitude != nil || input.GeoLongitude != nil
	if len(ip) > 64 {
		return "", "", "", nil, nil, false, errors.New("节点 IP 不能超过 64 个字符")
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return "", "", "", nil, nil, false, errors.New("节点 IP 格式无效")
	}
	if len(geoName) > 128 {
		return "", "", "", nil, nil, false, errors.New("节点位置名不能超过 128 个字符")
	}

	geoLatitude := cloneCoordinate(input.GeoLatitude)
	geoLongitude := cloneCoordinate(input.GeoLongitude)
	if (geoLatitude == nil) != (geoLongitude == nil) {
		return "", "", "", nil, nil, false, errors.New("地图坐标必须同时填写纬度和经度")
	}
	if geoLatitude != nil && (*geoLatitude < -90 || *geoLatitude > 90) {
		return "", "", "", nil, nil, false, errors.New("纬度必须在 -90 到 90 之间")
	}
	if geoLongitude != nil && (*geoLongitude < -180 || *geoLongitude > 180) {
		return "", "", "", nil, nil, false, errors.New("经度必须在 -180 到 180 之间")
	}

	if !manualOverride {
		return name, ip, "", nil, nil, false, nil
	}
	if geoLatitude == nil && geoLongitude == nil && geoName == "" {
		return name, ip, "", nil, nil, false, nil
	}

	return name, ip, geoName, geoLatitude, geoLongitude, true, nil
}

func cloneCoordinate(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func ResolveReportedNodeIP(reportedIP string, remoteAddr string, forwardedHeaders ...string) string {
	reported := iputil.NormalizeIP(reportedIP)
	remote := iputil.NormalizeRemoteAddr(remoteAddr)
	forwarded := publicNodeIPFromForwardedHeaders(remote, forwardedHeaders)
	if reported == "" {
		if forwarded != "" {
			return forwarded
		}
		return remote
	}
	if !shouldPreferRemoteNodeIP(reported) {
		return reported
	}
	if forwarded != "" {
		return forwarded
	}
	if isPublicNodeIP(remote) {
		return remote
	}
	return reported
}

func shouldPreferRemoteNodeIP(ip string) bool {
	return !isPublicNodeIP(ip)
}

func isPublicNodeIP(raw string) bool {
	return iputil.IsPublicString(raw)
}

func publicNodeIPFromForwardedHeaders(remote string, headers []string) string {
	if !shouldTrustForwardedNodeIP(remote) {
		return ""
	}
	for _, header := range headers {
		for _, candidate := range forwardedNodeIPCandidates(header) {
			normalized := normalizeForwardedNodeIP(candidate)
			if isPublicNodeIP(normalized) {
				return normalized
			}
		}
	}
	return ""
}

func shouldTrustForwardedNodeIP(remote string) bool {
	if remote == "" {
		return false
	}
	return !isPublicNodeIP(remote)
}

func forwardedNodeIPCandidates(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	candidates := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		for _, segment := range strings.Split(candidate, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(segment), "=")
			if !ok {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(key), "for") {
				candidate = strings.TrimSpace(value)
				break
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func normalizeForwardedNodeIP(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	candidate = strings.Trim(candidate, `"`)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || strings.EqualFold(candidate, "unknown") {
		return ""
	}
	if strings.HasPrefix(candidate, "[") {
		if end := strings.Index(candidate, "]"); end > 0 {
			return iputil.NormalizeIP(candidate[1:end])
		}
	}
	if normalized := iputil.NormalizeRemoteAddr(candidate); normalized != "" {
		return normalized
	}
	return iputil.NormalizeIP(candidate)
}

func buildNodeAgentReleaseView(node *model.Node, release *githubReleaseResponse, channel ReleaseChannel) *NodeAgentReleaseInfo {
	currentVersion := strings.TrimSpace(node.AgentVersion)
	view := &NodeAgentReleaseInfo{
		CurrentVersion:   currentVersion,
		Channel:          channel.String(),
		UpdateRequested:  node.UpdateRequested,
		RequestedChannel: normalizeReleaseChannel(node.UpdateChannel).String(),
		RequestedTag:     strings.TrimSpace(node.UpdateTag),
	}
	if release == nil {
		return view
	}
	view.TagName = release.TagName
	view.Body = release.Body
	view.HTMLURL = release.HTMLURL
	view.PublishedAt = release.PublishedAt
	view.Prerelease = release.Prerelease
	view.HasUpdate = isVersionNewer(currentVersion, release.TagName)
	return view
}

func RegisterNodeWithAgentToken(node *model.Node, payload AgentNodePayload) (*AgentRegistrationResponse, error) {
	payload = normalizeAgentNodePayload(payload)
	if node == nil {
		return nil, errors.New("节点不存在")
	}
	if err := validateAgentNodePayload(payload); err != nil {
		return nil, err
	}
	applyNodeRuntime(node, payload, true)
	if err := node.Update(); err != nil {
		return nil, err
	}
	refreshAgentTokenCache(node)
	slog.Info("agent register succeeded on reserved node", "node_id", node.NodeID, "name", node.Name)
	return &AgentRegistrationResponse{
		NodeID:     node.NodeID,
		AgentToken: node.AgentToken,
		Name:       node.Name,
	}, nil
}

func RegisterNodeWithDiscovery(payload AgentNodePayload) (*AgentRegistrationResponse, error) {
	payload = normalizeAgentNodePayload(payload)
	if err := validateAgentNodePayload(payload); err != nil {
		return nil, err
	}
	nodeID, err := newServerNodeID()
	if err != nil {
		return nil, err
	}
	agentToken, err := newRandomToken()
	if err != nil {
		return nil, err
	}
	nodeName := payload.Name
	if nodeName == "" {
		nodeName = nodeID
	}
	node := &model.Node{
		NodeID:     nodeID,
		Name:       nodeName,
		AgentToken: agentToken,
	}
	applyNodeRuntime(node, payload, false)
	if err = withCommercialResourceCreation("node", func(tx *gorm.DB) error {
		return node.InsertWithDB(tx)
	}); err != nil {
		if isUniqueConstraintError(err) {
			return nil, errors.New("节点标识生成冲突，请重试")
		}
		return nil, err
	}
	refreshAgentTokenCache(node)
	slog.Info("agent discovery register succeeded", "node_id", node.NodeID, "name", node.Name)
	return &AgentRegistrationResponse{
		NodeID:     node.NodeID,
		AgentToken: node.AgentToken,
		Name:       node.Name,
	}, nil
}

func normalizeAgentNodePayload(payload AgentNodePayload) AgentNodePayload {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.IP = strings.TrimSpace(payload.IP)
	payload.AgentVersion = strings.TrimSpace(payload.AgentVersion)
	payload.NginxVersion = strings.TrimSpace(payload.NginxVersion)
	payload.CurrentVersion = strings.TrimSpace(payload.CurrentVersion)
	payload.CurrentChecksum = strings.TrimSpace(payload.CurrentChecksum)
	payload.LastError = truncateForDatabase(payload.LastError, 16000)
	payload.OpenrestyStatus = normalizeOpenrestyStatus(payload.OpenrestyStatus)
	payload.OpenrestyMessage = truncateForDatabase(payload.OpenrestyMessage, 16000)
	return payload
}

func validateAgentNodePayload(payload AgentNodePayload) error {
	if payload.IP == "" {
		return errors.New("ip 不能为空")
	}
	if net.ParseIP(payload.IP) == nil {
		return errors.New("ip 格式无效")
	}
	if payload.AgentVersion == "" {
		return errors.New("agent_version 不能为空")
	}
	return nil
}

func applyNodeRuntime(node *model.Node, payload AgentNodePayload, preserveName bool) {
	if !preserveName || strings.TrimSpace(node.Name) == "" {
		if strings.TrimSpace(payload.Name) != "" {
			node.Name = strings.TrimSpace(payload.Name)
		}
	}
	node.IP = strings.TrimSpace(payload.IP)
	node.AgentVersion = strings.TrimSpace(payload.AgentVersion)
	node.NginxVersion = strings.TrimSpace(payload.NginxVersion)
	node.OpenrestyStatus = normalizeOpenrestyStatus(payload.OpenrestyStatus)
	node.OpenrestyMessage = truncateForDatabase(payload.OpenrestyMessage, 16000)
	node.Status = NodeStatusOnline
	node.CurrentVersion = strings.TrimSpace(payload.CurrentVersion)
	node.CurrentChecksum = strings.TrimSpace(payload.CurrentChecksum)
	node.LastSeenAt = time.Now()
	node.LastError = truncateForDatabase(payload.LastError, 16000)
	if !node.GeoManualOverride {
		applyGeoInfoFromIP(node, node.IP)
	}
}

func applyGeoInfoFromIP(node *model.Node, rawIP string) {
	if node == nil {
		return
	}
	node.GeoName = ""
	node.GeoLatitude = nil
	node.GeoLongitude = nil
	ip := net.ParseIP(strings.TrimSpace(rawIP))
	if ip == nil {
		return
	}
	info, err := geoip.GetGeoInfo(ip)
	if err != nil || info == nil {
		return
	}
	if strings.TrimSpace(info.Name) != "" {
		node.GeoName = strings.TrimSpace(info.Name)
	}
	if info.Latitude != nil && info.Longitude != nil {
		node.GeoLatitude = cloneCoordinate(info.Latitude)
		node.GeoLongitude = cloneCoordinate(info.Longitude)
	}
}

func normalizeOpenrestyStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case OpenrestyStatusHealthy:
		return OpenrestyStatusHealthy
	case OpenrestyStatusUnhealthy:
		return OpenrestyStatusUnhealthy
	default:
		return OpenrestyStatusUnknown
	}
}

func newRandomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newServerNodeID() (string, error) {
	token, err := newRandomToken()
	if err != nil {
		return "", err
	}
	return "node-" + token, nil
}
