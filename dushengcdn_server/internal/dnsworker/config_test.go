package dnsworker

import "testing"

func TestLoadConfigAppliesDNSProtectionOptions(t *testing.T) {
	t.Setenv("DUSHENGCDN_DNS_WORKER_SERVER_URL", "https://cdn.example.com")
	t.Setenv("DUSHENGCDN_DNS_WORKER_TOKEN", "token")
	t.Setenv("DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH", "/var/lib/GeoLite2-ASN.mmdb")
	t.Setenv("DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH", "/var/lib/china-operator-ip")
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
	if cfg.ASNDatabasePath != "/var/lib/GeoLite2-ASN.mmdb" {
		t.Fatalf("expected ASN database path from env, got %q", cfg.ASNDatabasePath)
	}
	if cfg.OperatorCIDRDatabasePath != "/var/lib/china-operator-ip" {
		t.Fatalf("expected operator CIDR database path from env, got %q", cfg.OperatorCIDRDatabasePath)
	}

	cfg, err = LoadConfig([]string{
		"--query-rate-limit", "0",
		"--udp-response-size", "1232",
		"--asn-database", "/tmp/asn.mmdb",
		"--operator-cidr-database", "/tmp/operators",
	}, "test")
	if err != nil {
		t.Fatalf("load config with args: %v", err)
	}
	if cfg.QueryRateLimit != 0 {
		t.Fatalf("expected disabled query rate limit, got %d", cfg.QueryRateLimit)
	}
	if cfg.UDPResponseSize != 1232 {
		t.Fatalf("expected arg UDP response size, got %d", cfg.UDPResponseSize)
	}
	if cfg.ASNDatabasePath != "/tmp/asn.mmdb" {
		t.Fatalf("expected arg ASN database path, got %q", cfg.ASNDatabasePath)
	}
	if cfg.OperatorCIDRDatabasePath != "/tmp/operators" {
		t.Fatalf("expected arg operator CIDR database path, got %q", cfg.OperatorCIDRDatabasePath)
	}
}
