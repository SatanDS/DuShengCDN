package service

import (
	"dushengcdn/model"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
)

const (
	AgentWSMessageTypeStatus          = "status"
	AgentWSMessageTypeStatusAck       = "status_ack"
	AgentWSMessageTypeSettings        = "settings"
	AgentWSMessageTypeActiveConfig    = "active_config"
	AgentWSMessageTypeForceSyncConfig = "force_sync_config"
	AgentWSMessageTypeUninstallAgent  = "uninstall_agent"
	AgentWSMessageTypeDNSWorkerUpdate = "dns_worker_update"
	AgentWSMessageTypeCacheOperation  = "cache_operation"
	AgentWSMessageTypePing            = "ping"
	AgentWSMessageTypePong            = "pong"

	AgentWSConnectedLastSeenValue = "__DUSHENGCDN_WS_CONNECTED__"
)

type AgentWSInboundMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type AgentWSOutboundMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type AgentObservabilityAck struct {
	WindowStartedAtUnix []int64 `json:"window_started_at_unix,omitempty"`
}

type AgentWSBroadcastResult struct {
	Version      string   `json:"version"`
	Checksum     string   `json:"checksum"`
	ClientCount  int      `json:"client_count"`
	SuccessCount int      `json:"success_count"`
	FailedNodes  []string `json:"failed_nodes"`
}

type AgentWSClient struct {
	nodeID string
	send   chan AgentWSOutboundMessage
	done   chan struct{}
	once   sync.Once
}

func (client *AgentWSClient) NodeID() string {
	if client == nil {
		return ""
	}
	return client.nodeID
}

func (client *AgentWSClient) Messages() <-chan AgentWSOutboundMessage {
	if client == nil {
		return nil
	}
	return client.send
}

func (client *AgentWSClient) Done() <-chan struct{} {
	if client == nil {
		return nil
	}
	return client.done
}

func (client *AgentWSClient) Send(message AgentWSOutboundMessage) bool {
	if client == nil {
		return false
	}
	select {
	case <-client.done:
		return false
	case client.send <- message:
		return true
	default:
		return false
	}
}

func (client *AgentWSClient) Close() {
	if client == nil {
		return
	}
	client.once.Do(func() {
		close(client.done)
	})
}

type agentWSHub struct {
	mu      sync.RWMutex
	clients map[string]*AgentWSClient
}

type agentWSBroadcastNode struct {
	NodeID   string
	PoolName string
}

type agentWSBroadcastContext struct {
	client   *AgentWSClient
	nodeID   string
	poolName string
}

var defaultAgentWSHub = &agentWSHub{
	clients: make(map[string]*AgentWSClient),
}

func RegisterAgentWSClient(nodeID string) *AgentWSClient {
	client := &AgentWSClient{
		nodeID: nodeID,
		send:   make(chan AgentWSOutboundMessage, 16),
		done:   make(chan struct{}),
	}
	defaultAgentWSHub.mu.Lock()
	if existing := defaultAgentWSHub.clients[nodeID]; existing != nil {
		slog.Debug("agent ws replacing existing connection", "node_id", nodeID)
		existing.Close()
	}
	defaultAgentWSHub.clients[nodeID] = client
	count := len(defaultAgentWSHub.clients)
	defaultAgentWSHub.mu.Unlock()
	slog.Debug("agent ws connection registered", "node_id", nodeID, "client_count", count)
	return client
}

func UnregisterAgentWSClient(client *AgentWSClient) {
	if client == nil {
		return
	}
	defaultAgentWSHub.mu.Lock()
	if current := defaultAgentWSHub.clients[client.nodeID]; current == client {
		delete(defaultAgentWSHub.clients, client.nodeID)
	}
	count := len(defaultAgentWSHub.clients)
	defaultAgentWSHub.mu.Unlock()
	client.Close()
	slog.Debug("agent ws connection unregistered", "node_id", client.nodeID, "client_count", count)
}

func IsAgentWSConnected(nodeID string) bool {
	defaultAgentWSHub.mu.RLock()
	client := defaultAgentWSHub.clients[nodeID]
	defaultAgentWSHub.mu.RUnlock()
	if client == nil {
		return false
	}
	select {
	case <-client.done:
		return false
	default:
		return true
	}
}

func AgentWSClientCount() int {
	defaultAgentWSHub.mu.RLock()
	defer defaultAgentWSHub.mu.RUnlock()
	return len(defaultAgentWSHub.clients)
}

func SendAgentWSSettings(nodeID string, settings *AgentSettings) bool {
	if settings == nil {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeSettings,
		Payload: settings,
	})
}

func SendAgentWSStatusAck(nodeID string, payload AgentNodePayload) bool {
	windows := agentObservabilityAckWindows(payload)
	if len(windows) == 0 {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type: AgentWSMessageTypeStatusAck,
		Payload: &AgentObservabilityAck{
			WindowStartedAtUnix: windows,
		},
	})
}

func SendAgentWSActiveConfig(nodeID string, activeConfig *ActiveConfigMeta) bool {
	if activeConfig == nil {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeActiveConfig,
		Payload: activeConfig,
	})
}

func SendAgentWSForceSyncConfig(nodeID string, activeConfig *ActiveConfigMeta) bool {
	if activeConfig == nil {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeForceSyncConfig,
		Payload: activeConfig,
	})
}

func SendAgentWSUninstallAgent(nodeID string) bool {
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type: AgentWSMessageTypeUninstallAgent,
	})
}

func SendAgentWSDNSWorkerUpdate(nodeID string, request *AgentDNSWorkerUpdateRequest) bool {
	if request == nil {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeDNSWorkerUpdate,
		Payload: request,
	})
}

func SendAgentWSCacheOperation(nodeID string, operation *AgentCacheOperation) bool {
	if operation == nil {
		return false
	}
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeCacheOperation,
		Payload: operation,
	})
}

func SendAgentWSPong(nodeID string) bool {
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type: AgentWSMessageTypePong,
	})
}

func agentObservabilityAckWindows(payload AgentNodePayload) []int64 {
	windows := make(map[int64]struct{}, len(payload.BufferedObservability)+1)
	if window := agentObservabilityWindowStartedAt(payload.Snapshot, payload.TrafficReport); window > 0 {
		windows[window] = struct{}{}
	}
	for _, record := range payload.BufferedObservability {
		if record.WindowStartedAtUnix <= 0 {
			continue
		}
		windows[record.WindowStartedAtUnix] = struct{}{}
	}
	if len(windows) == 0 {
		return nil
	}
	result := make([]int64, 0, len(windows))
	for window := range windows {
		result = append(result, window)
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func agentObservabilityWindowStartedAt(snapshot *AgentNodeMetricSnapshot, traffic *AgentNodeTrafficReport) int64 {
	if traffic != nil && traffic.WindowStartedAtUnix > 0 {
		return traffic.WindowStartedAtUnix - (traffic.WindowStartedAtUnix % 60)
	}
	if snapshot == nil || snapshot.CapturedAtUnix <= 0 {
		return 0
	}
	return snapshot.CapturedAtUnix - (snapshot.CapturedAtUnix % 60)
}

func sendAgentWSMessage(nodeID string, message AgentWSOutboundMessage) bool {
	defaultAgentWSHub.mu.RLock()
	client := defaultAgentWSHub.clients[nodeID]
	defaultAgentWSHub.mu.RUnlock()
	if client == nil {
		return false
	}
	ok := client.Send(message)
	if !ok {
		slog.Debug("agent ws send queued message failed", "node_id", nodeID, "type", message.Type)
	}
	return ok
}

func BroadcastAgentWSActiveConfig(activeConfig *ActiveConfigMeta) AgentWSBroadcastResult {
	result := AgentWSBroadcastResult{}
	if activeConfig == nil {
		slog.Debug("agent ws broadcast skipped because active config is nil")
		return result
	}
	result.Version = activeConfig.Version
	result.Checksum = activeConfig.Checksum

	clients := snapshotAgentWSClients()
	result.ClientCount = len(clients)
	message := AgentWSOutboundMessage{
		Type:    AgentWSMessageTypeActiveConfig,
		Payload: activeConfig,
	}
	for _, client := range clients {
		if client.Send(message) {
			result.SuccessCount++
			continue
		}
		result.FailedNodes = append(result.FailedNodes, client.NodeID())
	}
	slog.Debug("agent ws broadcast active config",
		"version", result.Version,
		"checksum", result.Checksum,
		"client_count", result.ClientCount,
		"success_count", result.SuccessCount,
		"failed_nodes", result.FailedNodes,
	)
	return result
}

func BroadcastAgentWSActiveConfigForVersion(version *model.ConfigVersion) AgentWSBroadcastResult {
	result := AgentWSBroadcastResult{}
	if version == nil {
		slog.Debug("agent ws pool broadcast skipped because active config version is nil")
		return result
	}
	result.Version = version.Version
	result.Checksum = version.Checksum

	clients := snapshotAgentWSClients()
	result.ClientCount = len(clients)
	contexts, err := buildAgentWSBroadcastContexts(clients)
	if err != nil {
		for _, client := range clients {
			result.FailedNodes = append(result.FailedNodes, client.NodeID())
		}
		slog.Debug("agent ws pool broadcast skipped because node batch lookup failed", "error", err)
		logAgentWSBroadcastActivePoolConfig(result)
		return result
	}
	for _, client := range clients {
		if _, ok := contexts[client.NodeID()]; !ok {
			result.FailedNodes = append(result.FailedNodes, client.NodeID())
		}
	}

	artifactsByPool, err := loadAgentWSBroadcastArtifactMetas(version.ID, contexts)
	if err != nil {
		for _, context := range contexts {
			result.FailedNodes = append(result.FailedNodes, context.nodeID)
		}
		slog.Debug("agent ws pool broadcast skipped because artifact meta batch lookup failed", "version_id", version.ID, "error", err)
		logAgentWSBroadcastActivePoolConfig(result)
		return result
	}

	for _, context := range contexts {
		artifact := artifactsByPool[context.poolName]
		if artifact == nil {
			slog.Debug("agent ws pool broadcast skipped missing artifact", "node_id", context.nodeID, "pool", context.poolName)
			result.FailedNodes = append(result.FailedNodes, context.nodeID)
			continue
		}
		message := AgentWSOutboundMessage{
			Type:    AgentWSMessageTypeActiveConfig,
			Payload: activeConfigMetaForVersionArtifact(version, artifact),
		}
		if context.client.Send(message) {
			result.SuccessCount++
			continue
		}
		result.FailedNodes = append(result.FailedNodes, context.nodeID)
	}
	logAgentWSBroadcastActivePoolConfig(result)
	return result
}

func BroadcastAgentWSActiveConfigForPool(version *model.ConfigVersion, poolName string) AgentWSBroadcastResult {
	result := AgentWSBroadcastResult{}
	if version == nil {
		slog.Debug("agent ws pool-specific broadcast skipped because active config version is nil")
		return result
	}
	poolName = normalizeNodePoolName(poolName)
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	result.Version = version.Version
	result.Checksum = version.Checksum

	clients := snapshotAgentWSClients()
	contexts, err := buildAgentWSBroadcastContexts(clients)
	if err != nil {
		for _, client := range clients {
			result.FailedNodes = append(result.FailedNodes, client.NodeID())
		}
		slog.Debug("agent ws pool-specific broadcast skipped because node batch lookup failed", "pool", poolName, "error", err)
		logAgentWSBroadcastActivePoolConfig(result)
		return result
	}
	artifact, err := model.GetConfigVersionArtifactMeta(version.ID, poolName)
	if err != nil {
		for _, context := range contexts {
			if context.poolName == poolName {
				result.FailedNodes = append(result.FailedNodes, context.nodeID)
			}
		}
		slog.Debug("agent ws pool-specific broadcast skipped because artifact meta lookup failed", "version_id", version.ID, "pool", poolName, "error", err)
		logAgentWSBroadcastActivePoolConfig(result)
		return result
	}
	result.Checksum = artifact.Checksum
	for _, context := range contexts {
		if context.poolName != poolName {
			continue
		}
		result.ClientCount++
		message := AgentWSOutboundMessage{
			Type:    AgentWSMessageTypeActiveConfig,
			Payload: activeConfigMetaForVersionArtifact(version, artifact),
		}
		if context.client.Send(message) {
			result.SuccessCount++
			continue
		}
		result.FailedNodes = append(result.FailedNodes, context.nodeID)
	}
	logAgentWSBroadcastActivePoolConfig(result)
	return result
}

func snapshotAgentWSClients() []*AgentWSClient {
	defaultAgentWSHub.mu.RLock()
	clients := make([]*AgentWSClient, 0, len(defaultAgentWSHub.clients))
	for _, client := range defaultAgentWSHub.clients {
		clients = append(clients, client)
	}
	defaultAgentWSHub.mu.RUnlock()
	return clients
}

func buildAgentWSBroadcastContexts(clients []*AgentWSClient) (map[string]agentWSBroadcastContext, error) {
	nodesByID, err := loadAgentWSBroadcastNodes(clients)
	if err != nil {
		return nil, err
	}
	contexts := make(map[string]agentWSBroadcastContext, len(clients))
	for _, client := range clients {
		nodeID := client.NodeID()
		node, ok := nodesByID[nodeID]
		if !ok {
			slog.Debug("agent ws pool broadcast skipped missing node", "node_id", nodeID)
			continue
		}
		contexts[nodeID] = agentWSBroadcastContext{
			client:   client,
			nodeID:   nodeID,
			poolName: normalizeNodePoolName(node.PoolName),
		}
	}
	return contexts, nil
}

func loadAgentWSBroadcastNodes(clients []*AgentWSClient) (map[string]agentWSBroadcastNode, error) {
	nodeIDs := uniqueAgentWSClientNodeIDs(clients)
	if len(nodeIDs) == 0 {
		return map[string]agentWSBroadcastNode{}, nil
	}
	var nodes []agentWSBroadcastNode
	if err := model.DB.Model(&model.Node{}).
		Select("node_id", "pool_name").
		Where("node_id IN ?", nodeIDs).
		Find(&nodes).Error; err != nil {
		return nil, err
	}
	nodesByID := make(map[string]agentWSBroadcastNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.NodeID] = node
	}
	return nodesByID, nil
}

func uniqueAgentWSClientNodeIDs(clients []*AgentWSClient) []string {
	seen := make(map[string]struct{}, len(clients))
	nodeIDs := make([]string, 0, len(clients))
	for _, client := range clients {
		nodeID := client.NodeID()
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs
}

func loadAgentWSBroadcastArtifactMetas(versionID uint, contexts map[string]agentWSBroadcastContext) (map[string]*model.ConfigVersionArtifact, error) {
	poolNames := uniqueAgentWSBroadcastPoolNames(contexts)
	artifacts, err := model.ListConfigVersionArtifactMetas(versionID, poolNames)
	if err != nil {
		return nil, err
	}
	artifactsByPool := make(map[string]*model.ConfigVersionArtifact, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		artifactsByPool[normalizeNodePoolName(artifact.PoolName)] = artifact
	}
	return artifactsByPool, nil
}

func uniqueAgentWSBroadcastPoolNames(contexts map[string]agentWSBroadcastContext) []string {
	seen := make(map[string]struct{}, len(contexts))
	poolNames := make([]string, 0, len(contexts))
	for _, context := range contexts {
		if _, ok := seen[context.poolName]; ok {
			continue
		}
		seen[context.poolName] = struct{}{}
		poolNames = append(poolNames, context.poolName)
	}
	sort.Strings(poolNames)
	return poolNames
}

func logAgentWSBroadcastActivePoolConfig(result AgentWSBroadcastResult) {
	slog.Debug("agent ws broadcast active pool config",
		"version", result.Version,
		"checksum", result.Checksum,
		"client_count", result.ClientCount,
		"success_count", result.SuccessCount,
		"failed_nodes", result.FailedNodes,
	)
}
