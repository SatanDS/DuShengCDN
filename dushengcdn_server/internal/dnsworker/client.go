package dnsworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type apiResponse[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type HeartbeatInput struct {
	Version             string                    `json:"version"`
	Status              string                    `json:"status"`
	LastSnapshotVersion string                    `json:"last_snapshot_version"`
	LastSnapshotAt      *time.Time                `json:"last_snapshot_at"`
	LastError           string                    `json:"last_error"`
	GeoIPEnabled        bool                      `json:"geoip_enabled"`
	GeoIPDatabasePath   string                    `json:"geoip_database_path"`
	GeoIPLastError      string                    `json:"geoip_last_error"`
	Rollups             []QueryRollupPayload      `json:"rollups"`
	SchedulingStates    []SnapshotSchedulingState `json:"scheduling_states,omitempty"`
}

type QueryRollupPayload struct {
	WindowStart     time.Time        `json:"window_start"`
	WindowMinutes   int              `json:"window_minutes"`
	ZoneID          uint             `json:"zone_id"`
	ProxyRouteID    uint             `json:"proxy_route_id"`
	SourceScope     string           `json:"source_scope"`
	QName           string           `json:"qname"`
	QType           string           `json:"qtype"`
	RCode           string           `json:"rcode"`
	QueryCount      int64            `json:"query_count"`
	TotalDurationMs int64            `json:"total_duration_ms"`
	MaxDurationMs   int64            `json:"max_duration_ms"`
	TargetSummary   map[string]int64 `json:"target_summary"`
}

func NewAPIClient(baseURL string, token string, timeout time.Duration) *APIClient {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return &APIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *APIClient) FetchSnapshot(ctx context.Context) (*Snapshot, error) {
	var response apiResponse[Snapshot]
	if err := c.doJSON(ctx, http.MethodGet, "/api/dns-snapshot", nil, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, fmt.Errorf("snapshot request failed: %s", response.Message)
	}
	return &response.Data, nil
}

func (c *APIClient) SendHeartbeat(ctx context.Context, input HeartbeatInput) error {
	var response apiResponse[json.RawMessage]
	if err := c.doJSON(ctx, http.MethodPost, "/api/dns-worker-heartbeat", input, &response); err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("heartbeat request failed: %s", response.Message)
	}
	return nil
}

func (c *APIClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	if c == nil {
		return fmt.Errorf("api client is nil")
	}
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-DNS-Worker-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", path, resp.Status)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}
