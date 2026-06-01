package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"dushengcdn-agent/internal/protocol"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL string, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) RegisterNode(ctx context.Context, payload protocol.NodePayload) (*protocol.RegisterNodeResponse, error) {
	slog.Debug("http register node request", "node_id", payload.NodeID, "current_version", payload.CurrentVersion)
	resp := protocol.APIResponse[protocol.RegisterNodeResponse]{}
	if err := c.postJSON(ctx, "/api/agent/nodes/register", payload, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, errors.New(resp.Message)
	}
	slog.Debug("http register node response", "node_id", resp.Data.NodeID)
	return &resp.Data, nil
}

func (c *Client) Heartbeat(ctx context.Context, payload protocol.NodePayload) (*protocol.HeartbeatResult, error) {
	resp := protocol.HeartbeatAPIResponse{}
	if err := c.postJSON(ctx, "/api/agent/nodes/heartbeat", payload, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, errors.New(resp.Message)
	}
	return &protocol.HeartbeatResult{
		AgentSettings: resp.AgentSettings,
		ActiveConfig:  resp.ActiveConfig,
	}, nil
}

func (c *Client) GetActiveConfig(ctx context.Context) (*protocol.ActiveConfigResponse, error) {
	resp := protocol.APIResponse[protocol.ActiveConfigResponse]{}
	if err := c.getJSON(ctx, "/api/agent/config-versions/active", &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, errors.New(resp.Message)
	}
	slog.Debug("http get active config response", "version", resp.Data.Version, "checksum", resp.Data.Checksum, "support_files", len(resp.Data.SupportFiles))
	return &resp.Data, nil
}

func (c *Client) ReportApplyLog(ctx context.Context, payload protocol.ApplyLogPayload) error {
	slog.Debug("http report apply log request", "node_id", payload.NodeID, "version", payload.Version, "result", payload.Result)
	return c.postJSON(ctx, "/api/agent/apply-logs", payload, nil)
}

func (c *Client) SetToken(token string) {
	c.token = strings.TrimSpace(token)
	slog.Debug("http client token updated")
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Agent-Token", c.token)
	return c.do(req, target)
}

func (c *Client) postJSON(ctx context.Context, path string, body any, target any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", c.token)
	return c.do(req, target)
}

func (c *Client) do(req *http.Request, target any) error {
	res, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("http request failed", "method", req.Method, "path", req.URL.Path, "error", err)
		return fmt.Errorf(
			"%s request to Server URL %s failed: %w. Check agent server_url, DNS/firewall connectivity, and HTTPS certificate trust",
			req.URL.Path,
			c.baseURL,
			err,
		)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.Warn("http request returned non-200", "method", req.Method, "path", req.URL.Path, "status", res.Status)
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
		if readErr != nil {
			return readErr
		}
		return c.formatHTTPError(req.URL.Path, res.StatusCode, res.Status, raw)
	}
	if target == nil {
		var wrapper protocol.APIResponse[json.RawMessage]
		if err = json.NewDecoder(res.Body).Decode(&wrapper); err != nil {
			slog.Error("http response decode failed", "method", req.Method, "path", req.URL.Path, "error", err)
			return err
		}
		if !wrapper.Success {
			slog.Warn("http api response failed", "method", req.Method, "path", req.URL.Path, "message", wrapper.Message)
			return errors.New(wrapper.Message)
		}
		return nil
	}
	if err = json.NewDecoder(res.Body).Decode(target); err != nil {
		slog.Error("http response decode failed", "method", req.Method, "path", req.URL.Path, "error", err)
		return err
	}
	return nil
}

func (c *Client) formatHTTPError(path string, statusCode int, status string, raw []byte) error {
	message := extractAPIErrorMessage(raw)
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		if message == "" {
			message = "authentication failed"
		}
		return fmt.Errorf(
			"%s returned %s: %s. Agent authentication failed; check agent_token/discovery_token in agent.json or DUSHENGCDN_AGENT_TOKEN/DUSHENGCDN_DISCOVERY_TOKEN, and make sure registration uses Discovery Token while heartbeat/config pull uses the node Agent Token",
			path,
			status,
			message,
		)
	}
	if statusCode == http.StatusNotFound {
		if message == "" {
			message = "endpoint not found"
		}
		return fmt.Errorf(
			"%s returned %s: %s. Check agent server_url points to the DuShengCDN Server API root",
			path,
			status,
			message,
		)
	}
	if message == "" {
		return errors.New(status)
	}
	return fmt.Errorf("%s returned %s: %s", path, status, message)
}

func extractAPIErrorMessage(raw []byte) string {
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return strings.TrimSpace(response.Message)
}
