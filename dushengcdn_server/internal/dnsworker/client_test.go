package dnsworker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIClientFetchSnapshotExplainsTokenAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dns-snapshot" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"message":"invalid DNS Worker Token"}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "wrong-token", time.Second)
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected token auth error")
	}
	message := err.Error()
	for _, want := range []string{
		"401 Unauthorized",
		"invalid DNS Worker Token",
		"DNS Worker Token authentication failed",
		"DUSHENGCDN_DNS_WORKER_TOKEN/--token",
		"not an Agent Token or login password",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
	if strings.Contains(message, "wrong-token") {
		t.Fatalf("error leaked token value: %q", message)
	}
}

func TestAPIClientFetchSnapshotExplainsServerURLNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := NewAPIClient(server.URL, "worker-token", time.Second)
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected not found error")
	}
	message := err.Error()
	for _, want := range []string{
		"404 Not Found",
		"Server API root",
		"DUSHENGCDN_DNS_WORKER_SERVER_URL/--server-url",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
}

func TestAPIClientFetchSnapshotExplainsServerURLConnectivityFailure(t *testing.T) {
	client := NewAPIClient("http://127.0.0.1:1", "worker-token", time.Second)
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected connectivity error")
	}
	message := err.Error()
	for _, want := range []string{
		"request to Server URL http://127.0.0.1:1 failed",
		"DUSHENGCDN_DNS_WORKER_SERVER_URL/--server-url",
		"DNS/firewall connectivity",
		"HTTPS certificate trust",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to contain %q, got %q", want, message)
		}
	}
	if strings.Contains(message, "worker-token") {
		t.Fatalf("error leaked token value: %q", message)
	}
}
