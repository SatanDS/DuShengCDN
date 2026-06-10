package service

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	NodeStatusOnline         = "online"
	NodeStatusOffline        = "offline"
	NodeStatusPending        = "pending"
	ApplyResultOK            = "success"
	ApplyResultWarning       = "warning"
	ApplyResultFailed        = "failed"
	OpenrestyStatusHealthy   = "healthy"
	OpenrestyStatusUnhealthy = "unhealthy"
	OpenrestyStatusUnknown   = "unknown"
	maxAgentDNSProbeTargets  = 4
)

type AgentNodePayload struct {
	NodeID                 string                             `json:"node_id"`
	Name                   string                             `json:"name"`
	IP                     string                             `json:"ip"`
	AgentVersion           string                             `json:"agent_version"`
	NginxVersion           string                             `json:"nginx_version"`
	CurrentVersion         string                             `json:"current_version"`
	CurrentChecksum        string                             `json:"current_checksum"`
	LastError              string                             `json:"last_error"`
	OpenrestyStatus        string                             `json:"openresty_status"`
	OpenrestyMessage       string                             `json:"openresty_message"`
	Profile                *AgentNodeSystemProfile            `json:"profile,omitempty"`
	Snapshot               *AgentNodeMetricSnapshot           `json:"snapshot,omitempty"`
	TrafficReport          *AgentNodeTrafficReport            `json:"traffic_report,omitempty"`
	AccessLogs             []AgentNodeAccessLog               `json:"access_logs,omitempty"`
	BufferedObservability  []AgentBufferedObservabilityRecord `json:"buffered_observability,omitempty"`
	HealthEvents           []AgentNodeHealthEvent             `json:"health_events"`
	DNSProbeResults        []AgentDNSProbeReport              `json:"dns_probe_results,omitempty"`
	DNSWorkerUpdateResults []AgentDNSWorkerUpdateResult       `json:"dns_worker_update_results,omitempty"`
}

type ApplyLogPayload struct {
	NodeID              string `json:"node_id"`
	Version             string `json:"version"`
	Result              string `json:"result"`
	Message             string `json:"message"`
	Checksum            string `json:"checksum"`
	MainConfigChecksum  string `json:"main_config_checksum"`
	RouteConfigChecksum string `json:"route_config_checksum"`
	SupportFileCount    int    `json:"support_file_count"`
}

type ApplyLogListQuery struct {
	NodeID   string `json:"node_id"`
	PageNo   int    `json:"pageNo"`
	PageSize int    `json:"pageSize"`
}

type ApplyLogListResult struct {
	Rows      []*model.ApplyLog `json:"rows"`
	Current   int               `json:"current"`
	Total     int               `json:"total"`
	TotalPage int               `json:"totalPage"`
}

type ApplyLogCleanupInput struct {
	DeleteAll     bool `json:"delete_all"`
	RetentionDays int  `json:"retention_days"`
}

type ApplyLogCleanupResult struct {
	DeleteAll     bool       `json:"delete_all"`
	RetentionDays int        `json:"retention_days"`
	DeletedCount  int64      `json:"deleted_count"`
	Cutoff        *time.Time `json:"cutoff,omitempty"`
}

type AgentConfigResponse struct {
	Version        string        `json:"version"`
	Checksum       string        `json:"checksum"`
	MainConfig     string        `json:"main_config"`
	RouteConfig    string        `json:"route_config"`
	RenderedConfig string        `json:"rendered_config"`
	SupportFiles   []SupportFile `json:"support_files"`
	CreatedAt      time.Time     `json:"created_at"`
}

type AgentSettings struct {
	HeartbeatInterval       int                   `json:"heartbeat_interval"`
	WebsocketUpgradeEnabled bool                  `json:"websocket_upgrade_enabled"`
	AutoUpdate              bool                  `json:"auto_update"`
	UpdateRepo              string                `json:"update_repo"`
	UpdateNow               bool                  `json:"update_now"`
	UpdateChannel           string                `json:"update_channel"`
	UpdateTag               string                `json:"update_tag"`
	RestartOpenrestyNow     bool                  `json:"restart_openresty_now"`
	DNSProbeTargets         []AgentDNSProbeTarget `json:"dns_probe_targets,omitempty"`
}

type AgentDNSProbeTarget struct {
	WorkerID      string `json:"worker_id"`
	Name          string `json:"name"`
	PublicAddress string `json:"public_address"`
	QueryName     string `json:"query_name"`
	QueryType     string `json:"query_type"`
}

type AgentDNSWorkerUpdateRequest struct {
	WorkerID   string `json:"worker_id"`
	WorkerName string `json:"worker_name"`
	Repo       string `json:"repo"`
	Channel    string `json:"channel"`
	TagName    string `json:"tag_name,omitempty"`
	InstallDir string `json:"install_dir,omitempty"`
}

type AgentDNSWorkerUpdateResult struct {
	WorkerID       string `json:"worker_id"`
	WorkerName     string `json:"worker_name,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Channel        string `json:"channel,omitempty"`
	TagName        string `json:"tag_name,omitempty"`
	InstallDir     string `json:"install_dir,omitempty"`
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
	ReportedAtUnix int64  `json:"reported_at_unix"`
}

type ActiveConfigMeta struct {
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

type HeartbeatResponse struct {
	Node             *model.Node                   `json:"node"`
	AgentSettings    *AgentSettings                `json:"agent_settings"`
	ActiveConfig     *ActiveConfigMeta             `json:"active_config"`
	DNSWorkerUpdates []AgentDNSWorkerUpdateRequest `json:"dns_worker_updates,omitempty"`
}

type NodeView struct {
	ID                        uint       `json:"id"`
	NodeID                    string     `json:"node_id"`
	Name                      string     `json:"name"`
	IP                        string     `json:"ip"`
	PoolName                  string     `json:"pool_name"`
	Tags                      []string   `json:"tags"`
	Weight                    int        `json:"weight"`
	PublicIPs                 []string   `json:"public_ips"`
	SchedulingEnabled         bool       `json:"scheduling_enabled"`
	DrainMode                 bool       `json:"drain_mode"`
	GeoName                   string     `json:"geo_name"`
	GeoLatitude               *float64   `json:"geo_latitude"`
	GeoLongitude              *float64   `json:"geo_longitude"`
	GeoManualOverride         bool       `json:"geo_manual_override"`
	AgentToken                string     `json:"agent_token,omitempty"`
	AgentTokenPrefix          string     `json:"agent_token_prefix"`
	AgentTokenAvailable       bool       `json:"agent_token_available"`
	AutoUpdateEnabled         bool       `json:"auto_update_enabled"`
	UpdateRequested           bool       `json:"update_requested"`
	UpdateChannel             string     `json:"update_channel"`
	UpdateTag                 string     `json:"update_tag"`
	RestartOpenrestyRequested bool       `json:"restart_openresty_requested"`
	AgentVersion              string     `json:"agent_version"`
	NginxVersion              string     `json:"nginx_version"`
	OpenrestyStatus           string     `json:"openresty_status"`
	OpenrestyMessage          string     `json:"openresty_message"`
	Status                    string     `json:"status"`
	CurrentVersion            string     `json:"current_version"`
	CurrentChecksum           string     `json:"current_checksum"`
	TargetConfigVersion       string     `json:"target_config_version"`
	TargetConfigChecksum      string     `json:"target_config_checksum"`
	TargetConfigPool          string     `json:"target_config_pool"`
	TargetConfigAvailable     bool       `json:"target_config_available"`
	ConfigInSync              bool       `json:"config_in_sync"`
	LastSeenAt                any        `json:"last_seen_at"`
	LastError                 string     `json:"last_error"`
	LatestApplyResult         string     `json:"latest_apply_result"`
	LatestApplyMessage        string     `json:"latest_apply_message"`
	LatestApplyChecksum       string     `json:"latest_apply_checksum"`
	LatestMainConfigChecksum  string     `json:"latest_main_config_checksum"`
	LatestRouteConfigChecksum string     `json:"latest_route_config_checksum"`
	LatestSupportFileCount    int        `json:"latest_support_file_count"`
	LatestApplyAt             *time.Time `json:"latest_apply_at"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

func RegisterNode(node *model.Node, payload AgentNodePayload) (*AgentRegistrationResponse, error) {
	return RegisterNodeWithAgentToken(node, payload)
}

func HeartbeatNode(node *model.Node, payload AgentNodePayload) (*HeartbeatResponse, error) {
	slog.Debug("agent heartbeat received", "node_id", node.NodeID, "current_version", strings.TrimSpace(payload.CurrentVersion))
	payload.NodeID = node.NodeID
	payload = normalizeAgentNodePayload(payload)
	if err := validateAgentNodePayload(payload); err != nil {
		return nil, err
	}
	previous := *node
	updateNow := node.UpdateRequested
	restartOpenrestyNow := node.RestartOpenrestyRequested
	updateChannel := normalizeReleaseChannel(node.UpdateChannel)
	updateTag := strings.TrimSpace(node.UpdateTag)
	applyNodeRuntime(node, payload, true)
	node.UpdateRequested = false
	node.UpdateChannel = ReleaseChannelStable.String()
	node.UpdateTag = ""
	node.RestartOpenrestyRequested = false
	changes := collectNodeHeartbeatChanges(&previous, node)
	if len(changes) > 0 {
		if err := model.DB.Model(node).Updates(changes).Error; err != nil {
			return nil, err
		}
	}
	if err := recordAgentDNSWorkerUpdateResults(node, payload.DNSWorkerUpdateResults); err != nil {
		return nil, err
	}
	refreshAgentTokenCache(node)
	dnsProbeTargets := buildAgentDNSProbeTargets()
	persistHeartbeatObservability(node.NodeID, payload, node.LastSeenAt, dnsProbeTargets)
	activeConfig, err := GetActiveConfigMetaForAgentNode(node)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &HeartbeatResponse{
		Node:             node,
		AgentSettings:    buildAgentSettingsWithDNSProbeTargets(node, updateNow, updateChannel.String(), updateTag, restartOpenrestyNow, dnsProbeTargets),
		ActiveConfig:     activeConfig,
		DNSWorkerUpdates: pendingAgentDNSWorkerUpdatesForNode(node),
	}, nil
}

func recordAgentDNSWorkerUpdateResults(node *model.Node, results []AgentDNSWorkerUpdateResult) error {
	if len(results) == 0 {
		return nil
	}
	now := time.Now()
	for _, result := range results {
		workerID := strings.TrimSpace(result.WorkerID)
		if workerID == "" {
			continue
		}
		worker, err := model.GetDNSWorkerByWorkerID(workerID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Debug("agent dns worker update result ignored for unknown worker", "worker_id", workerID)
				continue
			}
			return err
		}
		if worker.UpdateDispatchedNodeID != "" && node != nil && worker.UpdateDispatchedNodeID != node.NodeID {
			slog.Debug("agent dns worker update result ignored for mismatched node", "worker_id", workerID, "node_id", node.NodeID, "expected_node_id", worker.UpdateDispatchedNodeID)
			continue
		}
		mode := strings.TrimSpace(worker.UpdateDispatchMode)
		if mode != "agent_ws" && mode != "agent_heartbeat" && mode != "agent_heartbeat_sent" {
			slog.Debug("agent dns worker update result ignored for non-agent update mode", "worker_id", workerID, "mode", mode)
			continue
		}
		if isStaleAgentDNSWorkerUpdateResult(worker, result) {
			slog.Debug("agent dns worker update result ignored because it predates the current request", "worker_id", workerID)
			continue
		}
		if worker.UpdateRequested {
			worker.UpdateDispatchMode = "agent_result"
			worker.UpdateDispatchMessage = truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(result.Message)), 16000)
			worker.UpdateDispatchedAt = &now
			worker.UpdateRequested = false
			worker.UpdateTag = ""
			if result.Success {
				if worker.UpdateDispatchMessage == "" {
					worker.UpdateDispatchMessage = "DNS Worker update completed successfully."
				} else {
					worker.UpdateDispatchMessage = "DNS Worker update completed successfully: " + worker.UpdateDispatchMessage
				}
			} else {
				if worker.UpdateDispatchMessage == "" {
					worker.UpdateDispatchMessage = "DNS Worker update failed."
				} else {
					worker.UpdateDispatchMessage = "DNS Worker update failed: " + worker.UpdateDispatchMessage
				}
			}
			if err := model.DB.Model(worker).Select(
				"update_requested",
				"update_tag",
				"update_dispatch_mode",
				"update_dispatch_message",
				"update_dispatched_at",
			).Updates(worker).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func isStaleAgentDNSWorkerUpdateResult(worker *model.DNSWorker, result AgentDNSWorkerUpdateResult) bool {
	if worker == nil || worker.UpdateDispatchedAt == nil || result.ReportedAtUnix <= 0 {
		return false
	}
	reportedAt := time.Unix(result.ReportedAtUnix, 0)
	return reportedAt.Add(time.Second).Before(*worker.UpdateDispatchedAt)
}

func buildAgentSettings(node *model.Node, updateNow bool, updateChannel string, updateTag string, restartOpenrestyNow bool) *AgentSettings {
	return buildAgentSettingsWithDNSProbeTargets(node, updateNow, updateChannel, updateTag, restartOpenrestyNow, buildAgentDNSProbeTargets())
}

func buildAgentSettingsWithDNSProbeTargets(node *model.Node, updateNow bool, updateChannel string, updateTag string, restartOpenrestyNow bool, dnsProbeTargets []AgentDNSProbeTarget) *AgentSettings {
	if strings.TrimSpace(updateChannel) == "" {
		updateChannel = ReleaseChannelStable.String()
	}
	return &AgentSettings{
		HeartbeatInterval:       common.AgentHeartbeatInterval,
		WebsocketUpgradeEnabled: common.AgentWebsocketUpgradeEnabled,
		AutoUpdate:              node.AutoUpdateEnabled,
		UpdateRepo:              common.AgentUpdateRepo,
		UpdateNow:               updateNow,
		UpdateChannel:           updateChannel,
		UpdateTag:               strings.TrimSpace(updateTag),
		RestartOpenrestyNow:     restartOpenrestyNow,
		DNSProbeTargets:         dnsProbeTargets,
	}
}

func buildAgentDNSProbeTargets() []AgentDNSProbeTarget {
	workers, err := model.ListDNSWorkers()
	if err != nil || len(workers) == 0 {
		return nil
	}
	queryName, err := dnsWorkerProbeQueryName(0)
	if err != nil {
		return nil
	}
	targets := make([]AgentDNSProbeTarget, 0, len(workers))
	for _, worker := range workers {
		if worker == nil || normalizeDNSWorkerStatus(worker.Status) != dnsWorkerStatusOnline {
			continue
		}
		if strings.TrimSpace(worker.WorkerID) == "" || strings.TrimSpace(worker.PublicAddress) == "" {
			continue
		}
		publicAddress, err := validateDNSWorkerPublicAddressForStorage(worker.PublicAddress)
		if err != nil {
			continue
		}
		targets = append(targets, AgentDNSProbeTarget{
			WorkerID:      worker.WorkerID,
			Name:          worker.Name,
			PublicAddress: publicAddress,
			QueryName:     queryName,
			QueryType:     "SOA",
		})
		if len(targets) >= maxAgentDNSProbeTargets {
			break
		}
	}
	return targets
}

func GetActiveConfigMetaForAgent() (*ActiveConfigMeta, error) {
	version, err := model.GetActiveConfigVersion()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &ActiveConfigMeta{
		Version:  version.Version,
		Checksum: version.Checksum,
	}, nil
}

func GetActiveConfigMetaForAgentNode(node *model.Node) (*ActiveConfigMeta, error) {
	version, artifact, err := getActiveConfigVersionArtifactForNode(node)
	if err != nil {
		return nil, err
	}
	return &ActiveConfigMeta{
		Version:  version.Version,
		Checksum: artifact.Checksum,
	}, nil
}

func GetActiveConfigForAgent() (*AgentConfigResponse, error) {
	version, err := model.GetActiveConfigVersion()
	if err != nil {
		slog.Error("agent requested active config but no active version is available")
		return nil, err
	}
	var supportFiles []SupportFile
	if version.SupportFilesJSON != "" {
		if err = json.Unmarshal([]byte(version.SupportFilesJSON), &supportFiles); err != nil {
			return nil, err
		}
	}
	supportFiles = filterAgentSupportFiles(supportFiles)
	slog.Debug("agent fetched active config", "version", version.Version, "checksum", version.Checksum)
	return &AgentConfigResponse{
		Version:        version.Version,
		Checksum:       version.Checksum,
		MainConfig:     version.MainConfig,
		RouteConfig:    version.RenderedConfig,
		RenderedConfig: version.RenderedConfig,
		SupportFiles:   supportFiles,
		CreatedAt:      version.CreatedAt,
	}, nil
}

func GetActiveConfigForAgentNode(node *model.Node) (*AgentConfigResponse, error) {
	version, artifact, err := getActiveConfigVersionArtifactForNode(node)
	if err != nil {
		slog.Error("agent requested active pool config but it is unavailable", "error", err)
		return nil, err
	}
	var supportFiles []SupportFile
	if strings.TrimSpace(artifact.SupportFilesJSON) != "" {
		if err = json.Unmarshal([]byte(artifact.SupportFilesJSON), &supportFiles); err != nil {
			return nil, err
		}
	}
	supportFiles = filterAgentSupportFiles(supportFiles)
	slog.Debug("agent fetched active pool config", "version", version.Version, "pool", artifact.PoolName, "checksum", artifact.Checksum)
	return &AgentConfigResponse{
		Version:        version.Version,
		Checksum:       artifact.Checksum,
		MainConfig:     version.MainConfig,
		RouteConfig:    artifact.RenderedConfig,
		RenderedConfig: artifact.RenderedConfig,
		SupportFiles:   supportFiles,
		CreatedAt:      version.CreatedAt,
	}, nil
}

func getActiveConfigVersionArtifactForNode(node *model.Node) (*model.ConfigVersion, *model.ConfigVersionArtifact, error) {
	version, err := model.GetActiveConfigVersion()
	if err != nil {
		return nil, nil, err
	}
	poolName := normalizeNodePoolName("default")
	if node != nil {
		poolName = normalizeNodePoolName(node.PoolName)
	}
	if poolName == "" {
		poolName = normalizeNodePoolName("default")
	}
	artifact, err := model.GetConfigVersionArtifact(version.ID, poolName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if ensureErr := ensureConfigVersionArtifactsForPools(version, []string{poolName}); ensureErr != nil {
				return nil, nil, ensureErr
			}
			artifact, err = model.GetConfigVersionArtifact(version.ID, poolName)
		}
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("当前激活版本没有节点池 %s 的配置包，请重新发布配置", poolName)
		}
		return nil, nil, err
	}
	return version, artifact, nil
}

func filterAgentSupportFiles(files []SupportFile) []SupportFile {
	if len(files) == 0 {
		return nil
	}
	filtered := make([]SupportFile, 0, len(files))
	for _, file := range files {
		path := strings.ToLower(strings.TrimSpace(file.Path))
		switch {
		case strings.HasSuffix(path, ".crt"), strings.HasSuffix(path, ".key"), strings.HasSuffix(path, ".pem"):
			filtered = append(filtered, file)
		case path == "pow_config.json", path == "region_config.json", path == "waf_config.json", path == "cc_config.json":
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func ReportApplyLog(payload ApplyLogPayload) (*model.ApplyLog, error) {
	now := time.Now()
	payload.NodeID = strings.TrimSpace(payload.NodeID)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.Result = strings.TrimSpace(strings.ToLower(payload.Result))
	payload.Message = security.RedactSensitiveText(strings.TrimSpace(payload.Message))
	payload.Checksum = strings.TrimSpace(payload.Checksum)
	payload.MainConfigChecksum = strings.TrimSpace(payload.MainConfigChecksum)
	payload.RouteConfigChecksum = strings.TrimSpace(payload.RouteConfigChecksum)
	payload.Message = truncateForDatabase(payload.Message, 16000)
	if payload.NodeID == "" {
		return nil, errors.New("node_id 不能为空")
	}
	if payload.Version == "" {
		return nil, errors.New("version 不能为空")
	}
	if payload.Result != ApplyResultOK && payload.Result != ApplyResultWarning && payload.Result != ApplyResultFailed {
		return nil, errors.New("result 仅支持 success、warning 或 failed")
	}
	slog.Debug("agent apply log received", "node_id", payload.NodeID, "version", payload.Version, "result", payload.Result)

	log := &model.ApplyLog{
		NodeID:              payload.NodeID,
		Version:             payload.Version,
		Result:              payload.Result,
		Message:             payload.Message,
		Checksum:            payload.Checksum,
		MainConfigChecksum:  payload.MainConfigChecksum,
		RouteConfigChecksum: payload.RouteConfigChecksum,
		SupportFileCount:    payload.SupportFileCount,
		CreatedAt:           now,
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		node := &model.Node{}
		if err := tx.Where("node_id = ?", payload.NodeID).First(node).Error; err != nil {
			return err
		}
		node.Status = NodeStatusOnline
		node.LastSeenAt = now
		if payload.Result == ApplyResultOK {
			node.CurrentVersion = payload.Version
			node.CurrentChecksum = payload.Checksum
			node.LastError = ""
		} else {
			node.LastError = payload.Message
		}
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		return tx.Model(node).Select("status", "last_seen_at", "current_version", "current_checksum", "last_error").Updates(node).Error
	})
	if err != nil {
		return nil, err
	}
	if payload.Result == ApplyResultOK {
		slog.Debug("agent apply reported success", "node_id", payload.NodeID, "version", payload.Version)
	} else if payload.Result == ApplyResultWarning {
		slog.Warn("agent apply reported warning", "node_id", payload.NodeID, "version", payload.Version, "message", payload.Message)
	} else {
		slog.Error("agent apply reported failure", "node_id", payload.NodeID, "version", payload.Version, "message", payload.Message)
	}
	return log, nil
}

func ListNodeViews() ([]*NodeView, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.NodeID)
	}
	latestLogs, err := model.GetLatestApplyLogsByNodeIDs(nodeIDs)
	if err != nil {
		return nil, err
	}
	views := make([]*NodeView, 0, len(nodes))
	for _, node := range nodes {
		computedStatus := computeNodeStatus(node)
		view := buildNodeView(node)
		view.Status = computedStatus
		if log, ok := latestLogs[node.NodeID]; ok {
			view.LatestApplyResult = log.Result
			view.LatestApplyMessage = log.Message
			view.LatestApplyChecksum = log.Checksum
			view.LatestMainConfigChecksum = log.MainConfigChecksum
			view.LatestRouteConfigChecksum = log.RouteConfigChecksum
			view.LatestSupportFileCount = log.SupportFileCount
			view.LatestApplyAt = &log.CreatedAt
		}
		views = append(views, view)
	}
	return views, nil
}

func truncateForDatabase(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

const (
	defaultApplyLogPageSize  = 20
	maxApplyLogPageSize      = 200
	maxApplyLogRetentionDays = 3650
)

func ListApplyLogsPage(input ApplyLogListQuery) (*ApplyLogListResult, error) {
	pageNo := normalizeApplyLogPageNo(input.PageNo)
	pageSize := normalizeApplyLogPageSize(input.PageSize)
	nodeID := strings.TrimSpace(input.NodeID)

	rows, total, err := loadApplyLogPageData(nodeID, pageNo, pageSize, defaultApplyLogPageQueries)
	if err != nil {
		return nil, err
	}
	totalPage := 0
	if total > 0 {
		totalPage = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &ApplyLogListResult{
		Rows:      rows,
		Current:   pageNo,
		Total:     int(total),
		TotalPage: totalPage,
	}, nil
}

type applyLogPageQueries struct {
	listApplyLogs  func(model.ApplyLogQuery) ([]*model.ApplyLog, error)
	countApplyLogs func(string) (int64, error)
}

var defaultApplyLogPageQueries = applyLogPageQueries{
	listApplyLogs:  model.ListApplyLogs,
	countApplyLogs: model.CountApplyLogs,
}

func loadApplyLogPageData(nodeID string, pageNo int, pageSize int, queries applyLogPageQueries) ([]*model.ApplyLog, int64, error) {
	var rows []*model.ApplyLog
	var total int64
	if err := runConcurrentQueries(
		func() error {
			value, err := queries.listApplyLogs(model.ApplyLogQuery{
				NodeID:   nodeID,
				PageNo:   pageNo,
				PageSize: pageSize,
			})
			rows = value
			return err
		},
		func() error {
			value, err := queries.countApplyLogs(nodeID)
			total = value
			return err
		},
	); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func CleanupApplyLogs(input ApplyLogCleanupInput) (*ApplyLogCleanupResult, error) {
	if input.DeleteAll {
		deleted, err := model.DeleteAllApplyLogs()
		if err != nil {
			return nil, err
		}
		return &ApplyLogCleanupResult{
			DeleteAll:    true,
			DeletedCount: deleted,
		}, nil
	}
	if input.RetentionDays <= 0 || input.RetentionDays > maxApplyLogRetentionDays {
		return nil, errors.New("retention_days 必须在 1 到 3650 之间")
	}
	cutoff := time.Now().UTC().Add(-time.Duration(input.RetentionDays) * 24 * time.Hour)
	deleted, err := model.DeleteApplyLogsBefore(cutoff)
	if err != nil {
		return nil, err
	}
	return &ApplyLogCleanupResult{
		RetentionDays: input.RetentionDays,
		DeletedCount:  deleted,
		Cutoff:        &cutoff,
	}, nil
}

func normalizeApplyLogPageNo(pageNo int) int {
	if pageNo <= 0 {
		return 1
	}
	return pageNo
}

func normalizeApplyLogPageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultApplyLogPageSize
	}
	if pageSize > maxApplyLogPageSize {
		return maxApplyLogPageSize
	}
	return pageSize
}

func upsertNode(payload AgentNodePayload) (*model.Node, error) {
	return nil, errors.New("不再支持匿名自动注册")
}

func computeNodeStatus(node *model.Node) string {
	if node == nil {
		return NodeStatusOffline
	}
	if IsAgentWSConnected(node.NodeID) {
		return NodeStatusOnline
	}
	if node.LastSeenAt.IsZero() {
		return NodeStatusPending
	}
	if time.Since(node.LastSeenAt) > common.NodeOfflineThreshold {
		return NodeStatusOffline
	}
	return NodeStatusOnline
}

func collectNodeHeartbeatChanges(previous *model.Node, current *model.Node) map[string]any {
	if previous == nil || current == nil {
		return map[string]any{}
	}
	changes := make(map[string]any)
	appendIfChanged := func(key string, before any, after any) {
		if before != after {
			changes[key] = after
		}
	}
	appendIfChanged("name", previous.Name, current.Name)
	appendIfChanged("ip", previous.IP, current.IP)
	appendIfChanged("geo_name", previous.GeoName, current.GeoName)
	appendIfChanged("agent_version", previous.AgentVersion, current.AgentVersion)
	appendIfChanged("nginx_version", previous.NginxVersion, current.NginxVersion)
	appendIfChanged("openresty_status", previous.OpenrestyStatus, current.OpenrestyStatus)
	appendIfChanged("openresty_message", previous.OpenrestyMessage, current.OpenrestyMessage)
	appendIfChanged("status", previous.Status, current.Status)
	appendIfChanged("current_version", previous.CurrentVersion, current.CurrentVersion)
	appendIfChanged("current_checksum", previous.CurrentChecksum, current.CurrentChecksum)
	appendIfChanged("last_error", previous.LastError, current.LastError)
	appendIfChanged("update_requested", previous.UpdateRequested, current.UpdateRequested)
	appendIfChanged("update_channel", previous.UpdateChannel, current.UpdateChannel)
	appendIfChanged("update_tag", previous.UpdateTag, current.UpdateTag)
	appendIfChanged("restart_openresty_requested", previous.RestartOpenrestyRequested, current.RestartOpenrestyRequested)
	if !coordinatesEqual(previous.GeoLatitude, current.GeoLatitude) {
		changes["geo_latitude"] = current.GeoLatitude
	}
	if !coordinatesEqual(previous.GeoLongitude, current.GeoLongitude) {
		changes["geo_longitude"] = current.GeoLongitude
	}
	if !previous.LastSeenAt.Equal(current.LastSeenAt) {
		changes["last_seen_at"] = current.LastSeenAt
	}
	return changes
}

func coordinatesEqual(before *float64, after *float64) bool {
	if before == nil || after == nil {
		return before == after
	}
	return *before == *after
}
