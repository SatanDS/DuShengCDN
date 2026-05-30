package service

import (
	"encoding/json"
	"log/slog"
	"sync"
)

const (
	AgentWSMessageTypeStatus          = "status"
	AgentWSMessageTypeSettings        = "settings"
	AgentWSMessageTypeActiveConfig    = "active_config"
	AgentWSMessageTypeForceSyncConfig = "force_sync_config"
	AgentWSMessageTypeUninstallAgent  = "uninstall_agent"
	AgentWSMessageTypePing            = "ping"
	AgentWSMessageTypePong            = "pong"

	AgentWSConnectedLastSeenValue = "__OPENFLARE_WS_CONNECTED__"
)

type AgentWSInboundMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type AgentWSOutboundMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
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

func SendAgentWSPong(nodeID string) bool {
	return sendAgentWSMessage(nodeID, AgentWSOutboundMessage{
		Type: AgentWSMessageTypePong,
	})
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

	defaultAgentWSHub.mu.RLock()
	clients := make([]*AgentWSClient, 0, len(defaultAgentWSHub.clients))
	for _, client := range defaultAgentWSHub.clients {
		clients = append(clients, client)
	}
	defaultAgentWSHub.mu.RUnlock()

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
