package service

import (
	"dushengcdn/common"
	"dushengcdn/internal/dnsworker"
	"dushengcdn/model"
	"dushengcdn/utils/geoip/iputil"
	"dushengcdn/utils/security"
	"errors"
	"gorm.io/gorm"
	"strings"
	"time"
)

type DNSWorkerHeartbeatInput struct {
	Version                  string                                    `json:"version"`
	Status                   string                                    `json:"status"`
	RemoteIP                 string                                    `json:"-"`
	LastSnapshotVersion      string                                    `json:"last_snapshot_version"`
	LastSnapshotAt           *time.Time                                `json:"last_snapshot_at"`
	LastError                string                                    `json:"last_error"`
	GeoIPEnabled             bool                                      `json:"geoip_enabled"`
	GeoIPDatabasePath        string                                    `json:"geoip_database_path"`
	ASNDatabasePath          string                                    `json:"asn_database_path"`
	GeoIPLastError           string                                    `json:"geoip_last_error"`
	ASNLastError             string                                    `json:"asn_last_error"`
	GeoIPDatabaseType        string                                    `json:"geoip_database_type"`
	ASNDatabaseType          string                                    `json:"asn_database_type"`
	GeoIPCountryEnabled      bool                                      `json:"geoip_country_enabled"`
	GeoIPASNEnabled          bool                                      `json:"geoip_asn_enabled"`
	GeoIPOperatorEnabled     bool                                      `json:"geoip_operator_enabled"`
	OperatorCIDRDatabasePath string                                    `json:"operator_cidr_database_path"`
	OperatorCIDRLastError    string                                    `json:"operator_cidr_last_error"`
	UpdateSupported          bool                                      `json:"update_supported"`
	UninstallSupported       bool                                      `json:"uninstall_supported"`
	UpdateResult             *DNSWorkerUpdateResultInput               `json:"update_result,omitempty"`
	Rollups                  []DNSQueryRollupInput                     `json:"rollups"`
	SchedulingStates         []AuthoritativeDNSSnapshotSchedulingState `json:"scheduling_states,omitempty"`
}

type DNSWorkerUpdateResultInput struct {
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Channel        string `json:"channel,omitempty"`
	TagName        string `json:"tag_name,omitempty"`
	ReportedAtUnix int64  `json:"reported_at_unix"`
}

type DNSWorkerSettings struct {
	UpdateNow     bool                   `json:"update_now"`
	UninstallNow  bool                   `json:"uninstall_now"`
	UpdateRepo    string                 `json:"update_repo"`
	UpdateChannel string                 `json:"update_channel"`
	UpdateTag     string                 `json:"update_tag"`
	WorkerPolicy  dnsworker.WorkerPolicy `json:"worker_policy"`
}

type DNSWorkerHeartbeatView struct {
	Worker   DNSWorkerView     `json:"worker"`
	Settings DNSWorkerSettings `json:"settings"`
}

func RecordDNSWorkerHeartbeat(worker *model.DNSWorker, input DNSWorkerHeartbeatInput) (*DNSWorkerHeartbeatView, error) {
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		return nil, err
	}
	if worker == nil {
		return nil, errors.New("DNS worker is nil")
	}
	now := time.Now()
	uninstallNow := worker.UninstallRequested
	updateStateBefore := dnsWorkerUpdateStateSnapshotFromWorker(worker)
	applyDNSWorkerHeartbeatUpdateResult(worker, input.UpdateResult, now)
	applyLegacyAgentDNSWorkerUpdateAck(worker, input, now)
	applyDNSWorkerHeartbeatUpdateAck(worker, input, now)
	updateNow := worker.UpdateRequested && input.UpdateSupported && shouldDeliverDNSWorkerHeartbeatUpdate(worker)
	updateChannel := normalizeReleaseChannel(worker.UpdateChannel)
	updateTag := strings.TrimSpace(worker.UpdateTag)
	if updateNow {
		markDNSWorkerHeartbeatUpdateDelivered(worker, now)
	}
	includeUpdateState := !updateStateBefore.equal(dnsWorkerUpdateStateSnapshotFromWorker(worker))
	worker.Status = normalizeDNSWorkerStatus(input.Status)
	worker.Version = strings.TrimSpace(input.Version)
	worker.LastSnapshotVersion = strings.TrimSpace(input.LastSnapshotVersion)
	worker.LastSnapshotAt = normalizeDNSWorkerSnapshotAt(input.LastSnapshotAt, now)
	worker.LastSeenAt = &now
	worker.LastHeartbeatAt = &now
	if remoteIP := iputil.NormalizeIP(input.RemoteIP); remoteIP != "" {
		worker.LastRemoteIP = remoteIP
	}
	worker.LastError = truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(input.LastError)), 16000)
	worker.GeoIPEnabled = input.GeoIPEnabled
	worker.GeoIPDatabasePath = truncateForDatabase(strings.TrimSpace(input.GeoIPDatabasePath), 512)
	worker.ASNDatabasePath = truncateForDatabase(strings.TrimSpace(input.ASNDatabasePath), 512)
	worker.GeoIPLastError = truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(input.GeoIPLastError)), 16000)
	worker.ASNLastError = truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(input.ASNLastError)), 16000)
	worker.GeoIPDatabaseType = truncateForDatabase(strings.TrimSpace(input.GeoIPDatabaseType), 128)
	worker.ASNDatabaseType = truncateForDatabase(strings.TrimSpace(input.ASNDatabaseType), 128)
	worker.GeoIPCountryEnabled = input.GeoIPCountryEnabled
	worker.GeoIPASNEnabled = input.GeoIPASNEnabled
	worker.GeoIPOperatorEnabled = input.GeoIPOperatorEnabled
	worker.OperatorCIDRDatabasePath = truncateForDatabase(strings.TrimSpace(input.OperatorCIDRDatabasePath), 512)
	worker.OperatorCIDRLastError = truncateForDatabase(security.RedactSensitiveText(strings.TrimSpace(input.OperatorCIDRLastError)), 16000)
	worker.UpdateSupported = input.UpdateSupported
	if input.UpdateSupported {
		worker.LastUpdateSupportedAt = &now
	}
	worker.UninstallSupported = input.UninstallSupported
	if input.UninstallSupported {
		worker.LastUninstallSupportedAt = &now
	}
	db := model.DB.Session(&gorm.Session{DisableNestedTransaction: true})
	if err := db.Transaction(func(tx *gorm.DB) error {
		acl, err := buildDNSWorkerHeartbeatACL(tx, worker)
		if err != nil {
			return err
		}
		filteredRollups := limitDNSQueryRollupInputs(filterDNSQueryRollupInputsForACL(input.Rollups, acl), defaultDNSMaxHeartbeatRollups)
		rollupMeta := summarizeDNSQueryRollupInputs(filteredRollups)
		if rollupMeta.count > 0 {
			worker.LastRollupAt = &rollupMeta.lastRollupAt
			worker.LastRollupCount = rollupMeta.count
		}
		if uninstallNow {
			if err := deleteDNSWorkerRuntimeDataWithDB(tx, worker.WorkerID); err != nil {
				return err
			}
			return tx.Delete(worker).Error
		}
		if err := worker.UpdateHeartbeatWithDB(tx, includeUpdateState); err != nil {
			return err
		}
		if err := persistDNSWorkerSchedulingStatesWithDB(tx, worker, input.SchedulingStates); err != nil {
			return err
		}
		return persistDNSQueryRollupsWithACL(tx, worker, filteredRollups, acl)
	}); err != nil {
		return nil, err
	}
	return &DNSWorkerHeartbeatView{
		Worker: buildDNSWorkerView(worker, false),
		Settings: DNSWorkerSettings{
			UpdateNow:     updateNow,
			UninstallNow:  uninstallNow,
			UpdateRepo:    common.ServerUpdateRepo,
			UpdateChannel: updateChannel.String(),
			UpdateTag:     updateTag,
			WorkerPolicy:  authoritativeDNSWorkerPolicy(),
		},
	}, nil
}

type dnsWorkerUpdateStateSnapshot struct {
	updateRequested        bool
	updateTag              string
	updateDispatchMode     string
	updateDispatchMessage  string
	updateDispatchedAtUnix int64
}

func dnsWorkerUpdateStateSnapshotFromWorker(worker *model.DNSWorker) dnsWorkerUpdateStateSnapshot {
	if worker == nil {
		return dnsWorkerUpdateStateSnapshot{}
	}
	snapshot := dnsWorkerUpdateStateSnapshot{
		updateRequested:       worker.UpdateRequested,
		updateTag:             strings.TrimSpace(worker.UpdateTag),
		updateDispatchMode:    strings.TrimSpace(worker.UpdateDispatchMode),
		updateDispatchMessage: strings.TrimSpace(worker.UpdateDispatchMessage),
	}
	if worker.UpdateDispatchedAt != nil {
		snapshot.updateDispatchedAtUnix = worker.UpdateDispatchedAt.UnixNano()
	}
	return snapshot
}

func (snapshot dnsWorkerUpdateStateSnapshot) equal(other dnsWorkerUpdateStateSnapshot) bool {
	return snapshot == other
}

func shouldDeliverDNSWorkerHeartbeatUpdate(worker *model.DNSWorker) bool {
	if worker == nil || !worker.UpdateRequested {
		return false
	}
	mode := strings.TrimSpace(worker.UpdateDispatchMode)
	return mode == "" || mode == "worker_heartbeat"
}

func markDNSWorkerHeartbeatUpdateDelivered(worker *model.DNSWorker, now time.Time) {
	if worker == nil {
		return
	}
	worker.UpdateDispatchMode = "worker_heartbeat_sent"
	worker.UpdateDispatchMessage = "已随 DNS 响应端心跳返回更新任务，正在等待更新结果或后续心跳确认。"
	worker.UpdateDispatchedAt = &now
}

func applyDNSWorkerHeartbeatUpdateResult(worker *model.DNSWorker, result *DNSWorkerUpdateResultInput, now time.Time) {
	if worker == nil || result == nil || !worker.UpdateRequested {
		return
	}
	if isStaleDNSWorkerUpdateResult(worker, result) {
		return
	}
	worker.UpdateDispatchMode = "worker_result"
	message := truncateForDatabase(strings.TrimSpace(result.Message), 16000)
	worker.UpdateRequested = false
	worker.UpdateTag = ""
	if result.Success {
		if message == "" {
			message = "DNS Worker update completed successfully."
		} else {
			message = "DNS Worker update completed successfully: " + message
		}
	} else {
		if message == "" {
			message = "DNS Worker update failed."
		} else {
			message = "DNS Worker update failed: " + message
		}
	}
	worker.UpdateDispatchMessage = truncateForDatabase(message, 16000)
	worker.UpdateDispatchedAt = &now
}

func isStaleDNSWorkerUpdateResult(worker *model.DNSWorker, result *DNSWorkerUpdateResultInput) bool {
	if worker == nil || worker.UpdateDispatchedAt == nil || result == nil || result.ReportedAtUnix <= 0 {
		return false
	}
	reportedAt := time.Unix(result.ReportedAtUnix, 0)
	return reportedAt.Add(time.Second).Before(*worker.UpdateDispatchedAt)
}

func applyLegacyAgentDNSWorkerUpdateAck(worker *model.DNSWorker, input DNSWorkerHeartbeatInput, now time.Time) {
	if worker == nil || !worker.UpdateRequested || !input.UpdateSupported || worker.UpdateDispatchedAt == nil {
		return
	}
	mode := strings.TrimSpace(worker.UpdateDispatchMode)
	if mode != "agent_ws" && mode != "agent_heartbeat" && mode != "agent_heartbeat_sent" && mode != "agent_result" {
		return
	}
	if now.Sub(*worker.UpdateDispatchedAt) < dnsWorkerAgentUpdateAckDelay {
		return
	}
	worker.UpdateRequested = false
	worker.UpdateTag = ""
	worker.UpdateDispatchMode = "agent_heartbeat_ack"
	worker.UpdateDispatchMessage = "Agent 更新任务下发后已收到 DNS 响应端心跳；为兼容旧版 Agent，已结束本次更新等待。"
	worker.UpdateDispatchedAt = &now
}

func applyDNSWorkerHeartbeatUpdateAck(worker *model.DNSWorker, input DNSWorkerHeartbeatInput, now time.Time) {
	if worker == nil || !worker.UpdateRequested || worker.UpdateDispatchedAt == nil {
		return
	}
	mode := strings.TrimSpace(worker.UpdateDispatchMode)
	if mode != "worker_heartbeat" && mode != "worker_heartbeat_sent" {
		return
	}
	workerVersion := strings.TrimSpace(input.Version)
	updateTag := strings.TrimSpace(worker.UpdateTag)
	switch {
	case updateTag != "" && workerVersion == updateTag:
	case updateTag == "" && now.Sub(*worker.UpdateDispatchedAt) >= dnsWorkerAgentUpdateAckDelay:
	default:
		return
	}
	worker.UpdateRequested = false
	worker.UpdateTag = ""
	worker.UpdateDispatchMode = "worker_heartbeat_ack"
	if updateTag != "" {
		worker.UpdateDispatchMessage = "DNS 响应端已上报目标版本，已结束响应端心跳更新等待。"
	} else {
		worker.UpdateDispatchMessage = "DNS 响应端在更新任务下发后持续心跳，已结束本次更新等待。"
	}
	worker.UpdateDispatchedAt = &now
}
