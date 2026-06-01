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
