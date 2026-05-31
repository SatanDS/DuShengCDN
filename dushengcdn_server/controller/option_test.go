package controller

import "testing"

func TestValidateOpenRestyOption(t *testing.T) {
	testCases := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "worker processes auto", key: "OpenRestyWorkerProcesses", value: "auto"},
		{name: "worker processes number", key: "OpenRestyWorkerProcesses", value: "8"},
		{name: "worker processes invalid", key: "OpenRestyWorkerProcesses", value: "0", wantErr: true},
		{name: "events use empty", key: "OpenRestyEventsUse", value: ""},
		{name: "events use invalid", key: "OpenRestyEventsUse", value: "io_uring", wantErr: true},
		{name: "resolvers valid", key: "OpenRestyResolvers", value: "1.1.1.1 8.8.8.8"},
		{name: "resolvers invalid", key: "OpenRestyResolvers", value: "1.1.1.1; 8.8.8.8", wantErr: true},
		{name: "proxy buffers valid", key: "OpenRestyProxyBuffers", value: "16 16k"},
		{name: "proxy buffers invalid", key: "OpenRestyProxyBuffers", value: "16x16k", wantErr: true},
		{name: "cache max size valid", key: "OpenRestyCacheMaxSize", value: "2g"},
		{name: "cache max size invalid", key: "OpenRestyCacheMaxSize", value: "2gb", wantErr: true},
		{name: "client max body size valid", key: "OpenRestyClientMaxBodySize", value: "64m"},
		{name: "client max body size invalid", key: "OpenRestyClientMaxBodySize", value: "64mb", wantErr: true},
		{name: "large client header buffers valid", key: "OpenRestyLargeClientHeaderBuffers", value: "4 16k"},
		{name: "large client header buffers invalid", key: "OpenRestyLargeClientHeaderBuffers", value: "4x16k", wantErr: true},
		{name: "proxy request buffering valid", key: "OpenRestyProxyRequestBufferingEnabled", value: "true"},
		{name: "proxy request buffering invalid", key: "OpenRestyProxyRequestBufferingEnabled", value: "on", wantErr: true},
		{name: "websocket valid", key: "OpenRestyWebsocketEnabled", value: "false"},
		{name: "websocket invalid", key: "OpenRestyWebsocketEnabled", value: "off", wantErr: true},
		{name: "cache inactive valid", key: "OpenRestyCacheInactive", value: "30m"},
		{name: "cache inactive invalid", key: "OpenRestyCacheInactive", value: "30", wantErr: true},
		{name: "cache use stale valid", key: "OpenRestyCacheUseStale", value: "error timeout http_500"},
		{name: "cache use stale invalid", key: "OpenRestyCacheUseStale", value: "error whatever", wantErr: true},
		{name: "gzip level valid", key: "OpenRestyGzipCompLevel", value: "9"},
		{name: "gzip level invalid", key: "OpenRestyGzipCompLevel", value: "10", wantErr: true},
	}

	for _, testCase := range testCases {
		err := validateOpenRestyOption(testCase.key, testCase.value)
		if testCase.wantErr && err == nil {
			t.Fatalf("%s: expected error", testCase.name)
		}
		if !testCase.wantErr && err != nil {
			t.Fatalf("%s: unexpected error: %v", testCase.name, err)
		}
	}
}

func TestValidateAgentOption(t *testing.T) {
	if err := validateAgentOption("AgentWebsocketUpgradeEnabled", "true"); err != nil {
		t.Fatalf("expected websocket upgrade option to accept true: %v", err)
	}
	if err := validateAgentOption("AgentWebsocketUpgradeEnabled", "false"); err != nil {
		t.Fatalf("expected websocket upgrade option to accept false: %v", err)
	}
	if err := validateAgentOption("AgentWebsocketUpgradeEnabled", "on"); err == nil {
		t.Fatal("expected websocket upgrade option to reject non-boolean value")
	}
}

func TestValidateAuthoritativeDNSOption(t *testing.T) {
	testCases := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "enabled true", key: "AuthoritativeDNSEnabled", value: "true"},
		{name: "enabled invalid", key: "AuthoritativeDNSEnabled", value: "yes", wantErr: true},
		{name: "listen addr", key: "AuthoritativeDNSListenAddr", value: ":53"},
		{name: "listen addr empty", key: "AuthoritativeDNSListenAddr", value: "", wantErr: true},
		{name: "default ttl", key: "AuthoritativeDNSDefaultTTL", value: "30"},
		{name: "default ttl too high", key: "AuthoritativeDNSDefaultTTL", value: "86401", wantErr: true},
		{name: "snapshot max age", key: "AuthoritativeDNSSnapshotMaxAge", value: "300"},
		{name: "snapshot max age invalid", key: "AuthoritativeDNSSnapshotMaxAge", value: "0", wantErr: true},
	}

	for _, testCase := range testCases {
		err := validateAuthoritativeDNSOption(testCase.key, testCase.value)
		if testCase.wantErr && err == nil {
			t.Fatalf("%s: expected error", testCase.name)
		}
		if !testCase.wantErr && err != nil {
			t.Fatalf("%s: unexpected error: %v", testCase.name, err)
		}
	}
}
