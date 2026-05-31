package dnsworker

import "testing"

func TestLoadConfigAppliesDNSProtectionOptions(t *testing.T) {
	t.Setenv("DUSHENGCDN_DNS_WORKER_SERVER_URL", "https://cdn.example.com")
	t.Setenv("DUSHENGCDN_DNS_WORKER_TOKEN", "token")
	t.Setenv("DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT", "12")
	t.Setenv("DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE", "900")

	cfg, err := LoadConfig(nil, "test")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.QueryRateLimit != 12 {
		t.Fatalf("expected env query rate limit, got %d", cfg.QueryRateLimit)
	}
	if cfg.UDPResponseSize != 900 {
		t.Fatalf("expected env UDP response size, got %d", cfg.UDPResponseSize)
	}

	cfg, err = LoadConfig([]string{"--query-rate-limit", "0", "--udp-response-size", "1232"}, "test")
	if err != nil {
		t.Fatalf("load config with args: %v", err)
	}
	if cfg.QueryRateLimit != 0 {
		t.Fatalf("expected disabled query rate limit, got %d", cfg.QueryRateLimit)
	}
	if cfg.UDPResponseSize != 1232 {
		t.Fatalf("expected arg UDP response size, got %d", cfg.UDPResponseSize)
	}
}
