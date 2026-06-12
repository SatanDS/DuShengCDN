package dnsworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIClientFetchSnapshotVerifiesSignature(t *testing.T) {
	token := "worker-token"
	snapshot := &Snapshot{
		SnapshotVersion: "snap-1",
		GeneratedAt:     time.Now().UTC(),
		Zones:           []SnapshotZone{},
		Routes:          []SnapshotRoute{},
		Nodes:           []SnapshotNode{},
	}
	envelope, err := SignSnapshot(snapshot, token)
	if err != nil {
		t.Fatalf("sign snapshot: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(SnapshotSignatureHeader) != SnapshotSignatureVersion {
			t.Fatalf("expected signed snapshot request header, got %q", r.Header.Get(SnapshotSignatureHeader))
		}
		respondAPIClientTestJSON(t, w, map[string]any{
			"success": true,
			"data":    envelope,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, token, time.Second)
	got, err := client.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}
	if got.SnapshotVersion != "snap-1" {
		t.Fatalf("unexpected snapshot version %q", got.SnapshotVersion)
	}
}

func TestAPIClientFetchSnapshotSinceUsesConditionalHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(SnapshotSignatureHeader) != SnapshotSignatureVersion {
			t.Fatalf("expected signed snapshot request header, got %q", r.Header.Get(SnapshotSignatureHeader))
		}
		if got := r.Header.Get("If-None-Match"); got != `"snap-1"` {
			t.Fatalf("expected If-None-Match header, got %q", got)
		}
		if got := r.Header.Get("X-DNS-Snapshot-Version"); got != "snap-1" {
			t.Fatalf("expected snapshot version header, got %q", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "worker-token", time.Second)
	_, err := client.FetchSnapshotSince(context.Background(), "snap-1")
	if !errors.Is(err, ErrSnapshotNotModified) {
		t.Fatalf("expected ErrSnapshotNotModified, got %v", err)
	}
}

func TestAPIClientFetchSnapshotRejectsBadSignature(t *testing.T) {
	token := "worker-token"
	envelope, err := SignSnapshot(&Snapshot{
		SnapshotVersion: "snap-1",
		GeneratedAt:     time.Now().UTC(),
		Zones:           []SnapshotZone{},
		Routes:          []SnapshotRoute{},
		Nodes:           []SnapshotNode{},
	}, token)
	if err != nil {
		t.Fatalf("sign snapshot: %v", err)
	}
	envelope.Snapshot.SnapshotVersion = "tampered"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondAPIClientTestJSON(t, w, map[string]any{
			"success": true,
			"data":    envelope,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, token, time.Second)
	_, err = client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected signature verification error")
	}
	if !strings.Contains(err.Error(), "snapshot signature check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
		"DUSHENGCDN_DNS_WORKER_TOKEN_FILE/--token-file",
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

func respondAPIClientTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
