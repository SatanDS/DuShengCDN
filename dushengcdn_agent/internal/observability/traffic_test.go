package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dushengcdn-agent/internal/config"
	"dushengcdn-agent/internal/protocol"
	"dushengcdn-agent/internal/state"
)

func TestBuildTrafficReportAggregatesManagedAccessLog(t *testing.T) {
	tempDir := t.TempDir()
	routeConfigPath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	if err := os.MkdirAll(filepath.Dir(routeConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(routeConfigPath), "dushengcdn_access.log")
	content := []byte(
		"{\"ts\":\"2026-03-14T08:00:00Z\",\"host\":\"app.example.com\",\"path\":\"/\",\"remote_addr\":\"10.0.0.1\",\"status\":200}\n" +
			"{\"ts\":\"2026-03-14T08:00:05Z\",\"host\":\"app.example.com\",\"path\":\"/healthz\",\"remote_addr\":\"10.0.0.2\",\"status\":503}\n" +
			"{\"ts\":\"2026-03-14T08:00:08Z\",\"host\":\"api.example.com\",\"path\":\"/api\",\"remote_addr\":\"10.0.0.1\",\"status\":200}\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	report := BuildTrafficReport(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if report == nil {
		t.Fatal("expected traffic report")
	}
	if report.RequestCount != 3 || report.ErrorCount != 1 || report.UniqueVisitorCount != 2 {
		t.Fatalf("unexpected traffic report counters: %+v", report)
	}
	if report.StatusCodes["200"] != 2 || report.StatusCodes["503"] != 1 {
		t.Fatalf("unexpected status codes: %+v", report.StatusCodes)
	}
	if report.TopDomains["app.example.com"] != 2 || report.TopDomains["api.example.com"] != 1 {
		t.Fatalf("unexpected top domains: %+v", report.TopDomains)
	}

	snapshot, err := stateStore.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if snapshot.AccessLogOffset != int64(len(content)) {
		t.Fatalf("unexpected access log offset: %d", snapshot.AccessLogOffset)
	}

	secondReport := BuildTrafficReport(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if secondReport != nil {
		t.Fatalf("expected no report without appended lines, got %+v", secondReport)
	}
}

func TestBuildTrafficReportResetsOffsetAfterTruncate(t *testing.T) {
	tempDir := t.TempDir()
	routeConfigPath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	if err := os.MkdirAll(filepath.Dir(routeConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(routeConfigPath), "dushengcdn_access.log")
	if err := os.WriteFile(logPath, []byte("{\"ts\":\"2026-03-14T09:00:00Z\",\"host\":\"app.example.com\",\"path\":\"/\",\"remote_addr\":\"10.0.0.3\",\"status\":200}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	if err := stateStore.Save(&state.Snapshot{AccessLogOffset: 4096}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	report := BuildTrafficReport(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if report == nil || report.RequestCount != 1 {
		t.Fatalf("expected one request after truncate reset, got %+v", report)
	}
}

func TestBuildTrafficObservabilityReturnsAccessLogs(t *testing.T) {
	tempDir := t.TempDir()
	routeConfigPath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	if err := os.MkdirAll(filepath.Dir(routeConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(routeConfigPath), "dushengcdn_access.log")
	content := []byte(
		"{\"ts\":\"2026-03-14T08:00:00Z\",\"host\":\"app.example.com\",\"path\":\"/login\",\"remote_addr\":\"10.0.0.1\",\"status\":200,\"reason\":\"恶意请求防护观察记录: sensitive_paths\",\"request_length\":128,\"bytes_sent\":512}\n" +
			"{\"ts\":\"2026-03-14T08:00:05Z\",\"host\":\"api.example.com\",\"path\":\"/v1/ping\",\"remote_addr\":\"10.0.0.2\",\"status\":502,\"request_length\":64,\"bytes_sent\":256}\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	report, accessLogs, fallbackMetrics := BuildTrafficObservability(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if report == nil || report.RequestCount != 2 {
		t.Fatalf("expected traffic report, got %+v", report)
	}
	if len(accessLogs) != 2 {
		t.Fatalf("expected access logs, got %+v", accessLogs)
	}
	if fallbackMetrics == nil || fallbackMetrics.OpenrestyRxBytes != 192 || fallbackMetrics.OpenrestyTxBytes != 768 {
		t.Fatalf("expected fallback throughput metrics, got %+v", fallbackMetrics)
	}
	if accessLogs[0].RequestBytes != 128 || accessLogs[0].ResponseBytes != 512 {
		t.Fatalf("expected byte fields in access logs, got %+v", accessLogs)
	}
	if accessLogs[0].Reason != "恶意请求防护观察记录: sensitive_paths" {
		t.Fatalf("expected access log reason to be parsed, got %+v", accessLogs[0])
	}
	if accessLogs[0].Path != "/login" || accessLogs[1].Path != "/v1/ping" {
		t.Fatalf("unexpected access log paths: %+v", accessLogs)
	}
}

func TestBuildTrafficObservabilityParsesUpstreamBytes(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "dushengcdn_access.log")
	content := []byte(
		"{\"ts\":\"2026-03-14T08:00:00Z\",\"host\":\"app.example.com\",\"path\":\"/asset\",\"remote_addr\":\"10.0.0.1\",\"status\":200,\"request_length\":128,\"bytes_sent\":512,\"upstream_response_length\":\"1024, 256\"}\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	_, accessLogs, _ := BuildTrafficObservability(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if len(accessLogs) != 1 {
		t.Fatalf("expected one access log, got %+v", accessLogs)
	}
	if accessLogs[0].UpstreamBytes != 1280 {
		t.Fatalf("expected upstream bytes 1280, got %+v", accessLogs[0])
	}
}

func TestBuildTrafficReportAggregatesCacheAndUpstreamFields(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "dushengcdn_access.log")
	content := []byte(
		"{\"ts\":\"2026-03-14T08:00:00Z\",\"host\":\"app.example.com\",\"path\":\"/hit\",\"remote_addr\":\"10.0.0.1\",\"status\":200,\"cache_status\":\"HIT\",\"upstream_status\":\"200\",\"upstream_response_time\":\"0.010\"}\n" +
			"{\"ts\":\"2026-03-14T08:00:01Z\",\"host\":\"app.example.com\",\"path\":\"/miss\",\"remote_addr\":\"10.0.0.2\",\"status\":502,\"cache_status\":\"MISS\",\"upstream_status\":\"502\",\"upstream_response_time\":\"0.250\"}\n" +
			"{\"ts\":\"2026-03-14T08:00:02Z\",\"host\":\"app.example.com\",\"path\":\"/stale\",\"remote_addr\":\"10.0.0.3\",\"status\":200,\"cache_status\":\"STALE\",\"upstream_status\":\"200, 503\",\"upstream_response_time\":\"0.020, 0.030\"}\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	report := BuildTrafficReport(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if report == nil {
		t.Fatal("expected traffic report")
	}
	if report.CacheHitCount != 1 || report.CacheMissCount != 1 || report.CacheStaleCount != 1 {
		t.Fatalf("unexpected cache counters: %+v", report)
	}
	if report.UpstreamErrorCount != 2 {
		t.Fatalf("expected two upstream 5xx statuses, got %d", report.UpstreamErrorCount)
	}
	if report.UpstreamResponseMS != 310 {
		t.Fatalf("unexpected upstream response time sum: %d", report.UpstreamResponseMS)
	}
}

func TestBuildTrafficObservabilityTruncatesLongAccessLogPath(t *testing.T) {
	tempDir := t.TempDir()
	routeConfigPath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	if err := os.MkdirAll(filepath.Dir(routeConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(routeConfigPath), "dushengcdn_access.log")
	longPath := "/" + strings.Repeat("a", 140)
	content := []byte(
		"{\"ts\":\"2026-03-14T08:00:00Z\",\"host\":\"app.example.com\",\"path\":\"" + longPath + "\",\"remote_addr\":\"10.0.0.1\",\"status\":200}\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	_, accessLogs, _ := BuildTrafficObservability(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if len(accessLogs) != 1 {
		t.Fatalf("expected one access log, got %+v", accessLogs)
	}
	if got := len([]rune(accessLogs[0].Path)); got != accessLogPathMaxRunes {
		t.Fatalf("expected truncated path length %d, got %d (%q)", accessLogPathMaxRunes, got, accessLogs[0].Path)
	}
}

func TestBuildTrafficReportParsesCombinedAccessLog(t *testing.T) {
	tempDir := t.TempDir()
	routeConfigPath := filepath.Join(tempDir, "conf.d", "dushengcdn_routes.conf")
	if err := os.MkdirAll(filepath.Dir(routeConfigPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	logPath := filepath.Join(filepath.Dir(routeConfigPath), "dushengcdn_access.log")
	content := []byte(
		"10.0.0.1 - - [14/Mar/2026:08:00:00 +0000] \"GET / HTTP/1.1\" 200 123 \"-\" \"curl/8.0\"\n" +
			"10.0.0.2 - - [14/Mar/2026:08:00:05 +0000] \"GET /healthz HTTP/1.1\" 502 64 \"-\" \"curl/8.0\"\n" +
			"10.0.0.1 - - [14/Mar/2026:08:00:10 +0000] \"GET /api HTTP/1.1\" 200 256 \"-\" \"curl/8.0\"\n",
	)
	if err := os.WriteFile(logPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	stateStore := state.NewStore(filepath.Join(tempDir, "state.json"))
	report := BuildTrafficReport(&config.Config{AccessLogPath: logPath}, stateStore, nil)
	if report == nil {
		t.Fatal("expected traffic report from combined access log")
	}
	if report.RequestCount != 3 || report.ErrorCount != 1 || report.UniqueVisitorCount != 2 {
		t.Fatalf("unexpected combined log counters: %+v", report)
	}
	if report.StatusCodes["200"] != 2 || report.StatusCodes["502"] != 1 {
		t.Fatalf("unexpected combined log status codes: %+v", report.StatusCodes)
	}
	if len(report.TopDomains) != 0 {
		t.Fatalf("expected combined access log to omit top domains when host is unavailable, got %+v", report.TopDomains)
	}
}

func TestBuildTrafficReportReturnsManagedWindowEvenWhenRequestCountZero(t *testing.T) {
	report := BuildTrafficReport(nil, nil, &managedOpenRestyMetrics{
		TrafficReport: &protocol.NodeTrafficReport{
			WindowStartedAtUnix: 1710403200,
			WindowEndedAtUnix:   1710403260,
			RequestCount:        0,
			ErrorCount:          0,
			UniqueVisitorCount:  0,
			StatusCodes:         map[string]int64{},
			TopDomains:          map[string]int64{},
			SourceCountries:     map[string]int64{},
		},
	})
	if report == nil {
		t.Fatal("expected managed traffic report to be returned even when request count is zero")
	}
	if report.RequestCount != 0 || report.WindowStartedAtUnix != 1710403200 || report.WindowEndedAtUnix != 1710403260 {
		t.Fatalf("unexpected managed traffic report: %+v", report)
	}
}
