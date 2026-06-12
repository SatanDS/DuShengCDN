package service

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dushengcdn/model"

	"gorm.io/gorm"
)

func TestBroadcastAgentWSActiveConfigForVersionBatchesNodeAndArtifactMetas(t *testing.T) {
	setupServiceTestDB(t)
	resetAgentWSHubForTest(t)
	t.Cleanup(func() {
		resetAgentWSHubForTest(t)
	})

	version := seedAgentTestActiveConfigVersionWithArtifacts(t, "20260612-001", "checksum-global", map[string]string{
		"default": "checksum-default",
		"edge-a":  "checksum-edge-a",
		"edge-b":  "checksum-edge-b",
	})
	nodes := []*model.Node{
		{
			NodeID:       "node-ws-default-a",
			Name:         "default-a",
			IP:           "10.0.0.41",
			PoolName:     "default",
			AgentVersion: "v0.6.0",
			Status:       NodeStatusOnline,
		},
		{
			NodeID:       "node-ws-default-b",
			Name:         "default-b",
			IP:           "10.0.0.42",
			PoolName:     "default",
			AgentVersion: "v0.6.0",
			Status:       NodeStatusOnline,
		},
		{
			NodeID:       "node-ws-edge-a",
			Name:         "edge-a",
			IP:           "10.0.0.43",
			PoolName:     "edge-a",
			AgentVersion: "v0.6.0",
			Status:       NodeStatusOnline,
		},
		{
			NodeID:       "node-ws-edge-b",
			Name:         "edge-b",
			IP:           "10.0.0.44",
			PoolName:     "edge-b",
			AgentVersion: "v0.6.0",
			Status:       NodeStatusOnline,
		},
		{
			NodeID:       "node-ws-missing-pool",
			Name:         "missing-pool",
			IP:           "10.0.0.45",
			PoolName:     "missing-pool",
			AgentVersion: "v0.6.0",
			Status:       NodeStatusOnline,
		},
	}
	for _, node := range nodes {
		if err := node.Insert(); err != nil {
			t.Fatalf("insert node %s: %v", node.NodeID, err)
		}
	}

	clients := make(map[string]*AgentWSClient, len(nodes))
	for _, node := range nodes {
		clients[node.NodeID] = RegisterAgentWSClient(node.NodeID)
	}

	const callbackName = "dushengcdn:test_agent_ws_broadcast_query_counter"
	var nodeQueries int64
	var artifactQueries int64
	var largeArtifactSelects int64
	queryCallback := model.DB.Callback().Query()
	if err := queryCallback.After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		sql := strings.ToLower(db.Statement.SQL.String())
		table := db.Statement.Table
		schemaTable := ""
		if db.Statement.Schema != nil {
			schemaTable = db.Statement.Schema.Table
		}
		if table == "nodes" || schemaTable == "nodes" || strings.Contains(sql, "nodes") {
			atomic.AddInt64(&nodeQueries, 1)
		}
		if table == "config_version_artifacts" || schemaTable == "config_version_artifacts" || strings.Contains(sql, "config_version_artifacts") {
			atomic.AddInt64(&artifactQueries, 1)
			if strings.Contains(sql, "select *") || strings.Contains(sql, "rendered_config") || strings.Contains(sql, "support_files_json") {
				atomic.AddInt64(&largeArtifactSelects, 1)
			}
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = queryCallback.Remove(callbackName)
	})

	result := BroadcastAgentWSActiveConfigForVersion(version)
	if result.ClientCount != len(nodes) || result.SuccessCount != 4 {
		t.Fatalf("unexpected broadcast result: %+v", result)
	}
	if len(result.FailedNodes) != 1 || result.FailedNodes[0] != "node-ws-missing-pool" {
		t.Fatalf("expected only missing-pool node to fail, got %+v", result.FailedNodes)
	}

	expectedChecksums := map[string]string{
		"node-ws-default-a": "checksum-default",
		"node-ws-default-b": "checksum-default",
		"node-ws-edge-a":    "checksum-edge-a",
		"node-ws-edge-b":    "checksum-edge-b",
	}
	for nodeID, expectedChecksum := range expectedChecksums {
		message := readAgentWSMessageForTest(t, clients[nodeID])
		if message.Type != AgentWSMessageTypeActiveConfig {
			t.Fatalf("unexpected message type for %s: %s", nodeID, message.Type)
		}
		meta, ok := message.Payload.(*ActiveConfigMeta)
		if !ok {
			t.Fatalf("expected active config meta payload for %s, got %T", nodeID, message.Payload)
		}
		if meta.Version != version.Version || meta.Checksum != expectedChecksum {
			t.Fatalf("unexpected active config meta for %s: %+v", nodeID, meta)
		}
	}
	select {
	case message := <-clients["node-ws-missing-pool"].Messages():
		t.Fatalf("did not expect missing artifact node to receive a message: %+v", message)
	default:
	}

	if got := atomic.LoadInt64(&nodeQueries); got != 1 {
		t.Fatalf("expected one batched node query, got %d", got)
	}
	if got := atomic.LoadInt64(&artifactQueries); got != 1 {
		t.Fatalf("expected one batched artifact meta query, got %d", got)
	}
	if got := atomic.LoadInt64(&largeArtifactSelects); got != 0 {
		t.Fatalf("expected artifact query to avoid rendered/support file fields, got %d large selects", got)
	}
}

func TestSendAgentWSStatusAckQueuesCurrentAndBufferedWindows(t *testing.T) {
	resetAgentWSHubForTest(t)
	t.Cleanup(func() {
		resetAgentWSHubForTest(t)
	})

	client := RegisterAgentWSClient("node-ws-ack")
	if !SendAgentWSStatusAck("node-ws-ack", AgentNodePayload{
		TrafficReport: &AgentNodeTrafficReport{
			WindowStartedAtUnix: 1710403261,
			WindowEndedAtUnix:   1710403321,
		},
		BufferedObservability: []AgentBufferedObservabilityRecord{
			{WindowStartedAtUnix: 1710403140},
			{WindowStartedAtUnix: 1710403200},
			{WindowStartedAtUnix: 1710403200},
			{},
		},
	}) {
		t.Fatal("expected status ack to be queued")
	}

	message := readAgentWSMessageForTest(t, client)
	if message.Type != AgentWSMessageTypeStatusAck {
		t.Fatalf("unexpected message type: %s", message.Type)
	}
	ack, ok := message.Payload.(*AgentObservabilityAck)
	if !ok {
		t.Fatalf("expected observability ack payload, got %T", message.Payload)
	}
	expected := []int64{1710403140, 1710403200, 1710403260}
	if len(ack.WindowStartedAtUnix) != len(expected) {
		t.Fatalf("unexpected ack windows: %+v", ack.WindowStartedAtUnix)
	}
	for index, want := range expected {
		if ack.WindowStartedAtUnix[index] != want {
			t.Fatalf("unexpected ack windows: got %+v want %+v", ack.WindowStartedAtUnix, expected)
		}
	}

	if SendAgentWSStatusAck("node-ws-ack", AgentNodePayload{}) {
		t.Fatal("expected empty observability payload not to queue status ack")
	}
}

func readAgentWSMessageForTest(t *testing.T, client *AgentWSClient) AgentWSOutboundMessage {
	t.Helper()
	select {
	case message := <-client.Messages():
		return message
	case <-time.After(time.Second):
		t.Fatal("expected websocket broadcast message")
		return AgentWSOutboundMessage{}
	}
}

func resetAgentWSHubForTest(t *testing.T) {
	t.Helper()
	defaultAgentWSHub.mu.Lock()
	for _, client := range defaultAgentWSHub.clients {
		client.Close()
	}
	defaultAgentWSHub.clients = make(map[string]*AgentWSClient)
	defaultAgentWSHub.mu.Unlock()
}
