package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dushengcdn-agent/internal/protocol"
)

func TestClientHeartbeatExplainsAgentTokenAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/nodes/heartbeat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"message":"无权进行此操作，Agent Token 无效"}`))
	}))
	defer server.Close()

	client := New(server.URL, "wrong-agent-token", time.Second)
	_, err := client.Heartbeat(context.Background(), protocol.NodePayload{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	message := err.Error()
	for _, want := range []string{
		"401 Unauthorized",
		"Agent Token 无效",
		"Agent authentication failed",
		"agent_token/discovery_token",
		"DUSHENGCDN_AGENT_TOKEN_FILE/DUSHENGCDN_DISCOVERY_TOKEN_FILE",
		"DUSHENGCDN_AGENT_TOKEN/DUSHENGCDN_DISCOVERY_TOKEN",
		"registration uses Discovery Token",
		"heartbeat/config pull uses the node Agent Token",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
	if strings.Contains(message, "wrong-agent-token") {
		t.Fatalf("error leaked token value: %q", message)
	}
}

func TestClientHeartbeatReadsServerNodeID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/nodes/heartbeat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"node_id":"node-server-bound"},"active_config":{"version":"20260614-001","checksum":"checksum-1"}}`))
	}))
	defer server.Close()

	client := New(server.URL, "agent-token", time.Second)
	result, err := client.Heartbeat(context.Background(), protocol.NodePayload{NodeID: "node-local-random"})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	if result.ServerNodeID != "node-server-bound" {
		t.Fatalf("expected server node id to be decoded, got %q", result.ServerNodeID)
	}
	if result.ActiveConfig == nil || result.ActiveConfig.Checksum != "checksum-1" {
		t.Fatalf("expected active config to be decoded, got %+v", result.ActiveConfig)
	}
}

func TestClientRegisterExplainsServerURLNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := New(server.URL, "discovery-token", time.Second)
	_, err := client.RegisterNode(context.Background(), protocol.NodePayload{})
	if err == nil {
		t.Fatal("expected server url error")
	}
	message := err.Error()
	for _, want := range []string{
		"404 Not Found",
		"server_url",
		"Server API root",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
}

func TestClientHeartbeatExplainsServerURLConnectivityFailure(t *testing.T) {
	client := New("http://127.0.0.1:1", "agent-token", time.Second)
	_, err := client.Heartbeat(context.Background(), protocol.NodePayload{})
	if err == nil {
		t.Fatal("expected connectivity error")
	}
	message := err.Error()
	for _, want := range []string{
		"request to Server URL http://127.0.0.1:1 failed",
		"agent server_url",
		"DNS/firewall connectivity",
		"HTTPS certificate trust",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
	if strings.Contains(message, "agent-token") {
		t.Fatalf("error leaked token value: %q", message)
	}
}
