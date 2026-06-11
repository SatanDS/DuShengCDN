package service

import (
	"strings"
	"testing"
	"time"

	"dushengcdn/model"
)

func TestReportAgentOriginHealthUpsertsAndNormalizesReports(t *testing.T) {
	setupServiceTestDB(t)

	reportedAt := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	err := ReportAgentOriginHealth(" node-origin-health ", []AgentOriginHealthReport{
		{
			RouteID:       42,
			OriginURL:     " https://origin.example.com ",
			Status:        "HEALTHY",
			LatencyMS:     -10,
			LastError:     `proxy_set_header Authorization "Bearer origin-secret";`,
			CheckedAtUnix: reportedAt.Add(-time.Minute).Unix(),
		},
		{
			RouteID:       42,
			OriginURL:     "https://origin.example.com",
			Status:        "unhealthy",
			LatencyMS:     87,
			LastError:     "connect timeout",
			CheckedAtUnix: reportedAt.Add(time.Hour).Unix(),
		},
		{
			RouteID:   42,
			OriginURL: "   ",
			Status:    "healthy",
		},
	}, reportedAt)
	if err != nil {
		t.Fatalf("ReportAgentOriginHealth failed: %v", err)
	}

	statuses, err := model.ListOriginHealthStatuses(42, "node-origin-health")
	if err != nil {
		t.Fatalf("ListOriginHealthStatuses failed: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one upserted status, got %+v", statuses)
	}
	status := statuses[0]
	if status.NodeID != "node-origin-health" || status.OriginURL != "https://origin.example.com" {
		t.Fatalf("unexpected status scope: %+v", status)
	}
	if status.Status != "unhealthy" || status.LatencyMS != 87 || status.LastError != "connect timeout" {
		t.Fatalf("expected latest report to win, got %+v", status)
	}
	if !status.ReportedAt.Equal(reportedAt) {
		t.Fatalf("unexpected reported_at: got %s want %s", status.ReportedAt, reportedAt)
	}
	if !status.CheckedAt.Equal(reportedAt) {
		t.Fatalf("expected future checked_at to be clamped to reported_at, got %s", status.CheckedAt)
	}
	if strings.Contains(status.LastError, "origin-secret") {
		t.Fatalf("expected last_error to be redacted, got %q", status.LastError)
	}
}

func TestHeartbeatNodePersistsOriginHealthReports(t *testing.T) {
	setupServiceTestDB(t)

	node := &model.Node{
		NodeID:       "node-origin-heartbeat",
		Name:         "edge",
		IP:           "127.0.0.1",
		Status:       NodeStatusOnline,
		AgentVersion: "v1.0.0",
	}
	if err := model.DB.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	checkedAt := time.Now().UTC().Add(-2 * time.Minute)
	if _, err := HeartbeatNode(node, AgentNodePayload{
		Name:            "edge",
		IP:              "127.0.0.1",
		AgentVersion:    "v1.1.0",
		OpenrestyStatus: OpenrestyStatusHealthy,
		OriginHealthReports: []AgentOriginHealthReport{
			{
				RouteID:       7,
				OriginURL:     "https://origin-a.internal",
				Status:        "healthy",
				LatencyMS:     34,
				CheckedAtUnix: checkedAt.Unix(),
			},
			{
				RouteID:       7,
				OriginURL:     "https://origin-b.internal",
				Status:        "broken",
				LatencyMS:     0,
				LastError:     "status 502",
				CheckedAtUnix: checkedAt.Unix(),
			},
		},
	}); err != nil {
		t.Fatalf("HeartbeatNode failed: %v", err)
	}

	statuses, err := model.ListOriginHealthStatuses(7, node.NodeID)
	if err != nil {
		t.Fatalf("ListOriginHealthStatuses failed: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected two origin statuses, got %+v", statuses)
	}
	byOrigin := map[string]*model.OriginHealthStatus{}
	for _, status := range statuses {
		byOrigin[status.OriginURL] = status
	}
	if byOrigin["https://origin-a.internal"].Status != "healthy" || byOrigin["https://origin-a.internal"].LatencyMS != 34 {
		t.Fatalf("unexpected origin-a status: %+v", byOrigin["https://origin-a.internal"])
	}
	if byOrigin["https://origin-b.internal"].Status != "unknown" || byOrigin["https://origin-b.internal"].LastError != "status 502" {
		t.Fatalf("unexpected origin-b status: %+v", byOrigin["https://origin-b.internal"])
	}
}
