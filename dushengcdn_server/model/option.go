package model

import (
	"dushengcdn/common"
	"dushengcdn/utils/geoip"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["PasswordLoginEnabled"] = strconv.FormatBool(common.PasswordLoginEnabled)
	common.OptionMap["PasswordRegisterEnabled"] = strconv.FormatBool(common.PasswordRegisterEnabled)
	common.OptionMap["EmailVerificationEnabled"] = strconv.FormatBool(common.EmailVerificationEnabled)
	common.OptionMap["GitHubOAuthEnabled"] = strconv.FormatBool(common.GitHubOAuthEnabled)
	common.OptionMap["WeChatAuthEnabled"] = strconv.FormatBool(common.WeChatAuthEnabled)
	common.OptionMap["TurnstileCheckEnabled"] = strconv.FormatBool(common.TurnstileCheckEnabled)
	common.OptionMap["SMTPServer"] = ""
	common.OptionMap["SMTPPort"] = strconv.Itoa(common.SMTPPort)
	common.OptionMap["SMTPAccount"] = ""
	common.OptionMap["SMTPToken"] = ""
	common.OptionMap["Notice"] = ""
	common.OptionMap["About"] = ""
	common.OptionMap["Footer"] = common.Footer
	common.OptionMap["HomePageLink"] = common.HomePageLink
	common.OptionMap["SystemName"] = common.SystemName
	common.OptionMap["ServerAddress"] = common.ServerAddress
	common.OptionMap["GitHubClientId"] = ""
	common.OptionMap["GitHubClientSecret"] = ""
	common.OptionMap["WeChatServerAddress"] = ""
	common.OptionMap["WeChatServerToken"] = ""
	common.OptionMap["WeChatAccountQRCodeImageURL"] = ""
	common.OptionMap["TurnstileSiteKey"] = ""
	common.OptionMap["TurnstileSecretKey"] = ""
	common.OptionMap["AgentLegacyGlobalTokenEnabled"] = strconv.FormatBool(common.AgentLegacyGlobalTokenEnabled)
	common.OptionMap["AgentLegacyGlobalAuthEnabled"] = strconv.FormatBool(common.AgentLegacyGlobalTokenEnabled)
	common.OptionMap["AgentDiscoveryToken"] = ""
	common.OptionMap["AgentHeartbeatInterval"] = strconv.Itoa(common.AgentHeartbeatInterval)
	common.OptionMap["AgentWebsocketUpgradeEnabled"] = strconv.FormatBool(common.AgentWebsocketUpgradeEnabled)
	common.OptionMap["NodeOfflineThreshold"] = strconv.Itoa(int(common.NodeOfflineThreshold.Milliseconds()))
	common.OptionMap["AgentUpdateRepo"] = common.AgentUpdateRepo
	common.OptionMap["GeoIPProvider"] = common.GeoIPProvider
	common.OptionMap["DatabaseAutoCleanupEnabled"] = strconv.FormatBool(common.DatabaseAutoCleanupEnabled)
	common.OptionMap["DatabaseAutoCleanupRetentionDays"] = strconv.Itoa(common.DatabaseAutoCleanupRetentionDays)
	common.OptionMap["CloudflareDDoSRequestThreshold"] = strconv.FormatInt(common.CloudflareDDoSRequestThreshold, 10)
	common.OptionMap["CloudflareDDoSErrorRateThreshold"] = strconv.FormatFloat(common.CloudflareDDoSErrorRateThreshold, 'f', -1, 64)
	common.OptionMap["AuthoritativeDNSEnabled"] = strconv.FormatBool(common.AuthoritativeDNSEnabled)
	common.OptionMap["AuthoritativeDNSListenAddr"] = common.AuthoritativeDNSListenAddr
	common.OptionMap["AuthoritativeDNSDefaultTTL"] = strconv.Itoa(common.AuthoritativeDNSDefaultTTL)
	common.OptionMap["AuthoritativeDNSSnapshotMaxAge"] = strconv.Itoa(common.AuthoritativeDNSSnapshotMaxAge)
	common.OptionMap["AuthoritativeDNSWorkerQueryRateLimit"] = strconv.Itoa(common.AuthoritativeDNSWorkerQueryRateLimit)
	common.OptionMap["AuthoritativeDNSWorkerResponseRateLimit"] = strconv.Itoa(common.AuthoritativeDNSWorkerResponseRateLimit)
	common.OptionMap["AuthoritativeDNSWorkerUDPResponseSize"] = strconv.Itoa(common.AuthoritativeDNSWorkerUDPResponseSize)
	common.OptionMap["AuthoritativeDNSWorkerECSEnabled"] = strconv.FormatBool(common.AuthoritativeDNSWorkerECSEnabled)
	common.OptionMap["AuthoritativeDNSWorkerECSIPv4Prefix"] = strconv.Itoa(common.AuthoritativeDNSWorkerECSIPv4Prefix)
	common.OptionMap["AuthoritativeDNSWorkerECSIPv6Prefix"] = strconv.Itoa(common.AuthoritativeDNSWorkerECSIPv6Prefix)
	common.OptionMap["GSLBMetricFreshnessSeconds"] = strconv.Itoa(common.GSLBMetricFreshnessSeconds)
	common.OptionMap["GSLBProbeSchedulingEnabled"] = strconv.FormatBool(common.GSLBProbeSchedulingEnabled)
	common.OptionMap["OpenRestyWorkerProcesses"] = common.OpenRestyWorkerProcesses
	common.OptionMap["OpenRestyWorkerConnections"] = strconv.Itoa(common.OpenRestyWorkerConnections)
	common.OptionMap["OpenRestyWorkerRlimitNofile"] = strconv.Itoa(common.OpenRestyWorkerRlimitNofile)
	common.OptionMap["OpenRestyEventsUse"] = common.OpenRestyEventsUse
	common.OptionMap["OpenRestyEventsMultiAcceptEnabled"] = strconv.FormatBool(common.OpenRestyEventsMultiAcceptEnabled)
	common.OptionMap["OpenRestyKeepaliveTimeout"] = strconv.Itoa(common.OpenRestyKeepaliveTimeout)
	common.OptionMap["OpenRestyKeepaliveRequests"] = strconv.Itoa(common.OpenRestyKeepaliveRequests)
	common.OptionMap["OpenRestyClientHeaderTimeout"] = strconv.Itoa(common.OpenRestyClientHeaderTimeout)
	common.OptionMap["OpenRestyClientBodyTimeout"] = strconv.Itoa(common.OpenRestyClientBodyTimeout)
	common.OptionMap["OpenRestyClientMaxBodySize"] = common.OpenRestyClientMaxBodySize
	common.OptionMap["OpenRestyLargeClientHeaderBuffers"] = common.OpenRestyLargeClientHeaderBuffers
	common.OptionMap["OpenRestySendTimeout"] = strconv.Itoa(common.OpenRestySendTimeout)
	common.OptionMap["OpenRestyProxyConnectTimeout"] = strconv.Itoa(common.OpenRestyProxyConnectTimeout)
	common.OptionMap["OpenRestyProxySendTimeout"] = strconv.Itoa(common.OpenRestyProxySendTimeout)
	common.OptionMap["OpenRestyProxyReadTimeout"] = strconv.Itoa(common.OpenRestyProxyReadTimeout)
	common.OptionMap["OpenRestyWebsocketEnabled"] = strconv.FormatBool(common.OpenRestyWebsocketEnabled)
	common.OptionMap["OpenRestyProxyRequestBufferingEnabled"] = strconv.FormatBool(common.OpenRestyProxyRequestBufferingEnabled)
	common.OptionMap["OpenRestyProxyBufferingEnabled"] = strconv.FormatBool(common.OpenRestyProxyBufferingEnabled)
	common.OptionMap["OpenRestyProxyBuffers"] = common.OpenRestyProxyBuffers
	common.OptionMap["OpenRestyProxyBufferSize"] = common.OpenRestyProxyBufferSize
	common.OptionMap["OpenRestyProxyBusyBuffersSize"] = common.OpenRestyProxyBusyBuffersSize
	common.OptionMap["OpenRestyGzipEnabled"] = strconv.FormatBool(common.OpenRestyGzipEnabled)
	common.OptionMap["OpenRestyGzipMinLength"] = strconv.Itoa(common.OpenRestyGzipMinLength)
	common.OptionMap["OpenRestyGzipCompLevel"] = strconv.Itoa(common.OpenRestyGzipCompLevel)
	common.OptionMap["OpenRestyCacheEnabled"] = strconv.FormatBool(common.OpenRestyCacheEnabled)
	common.OptionMap["OpenRestyCachePath"] = common.OpenRestyCachePath
	common.OptionMap["OpenRestyCacheLevels"] = common.OpenRestyCacheLevels
	common.OptionMap["OpenRestyCacheInactive"] = common.OpenRestyCacheInactive
	common.OptionMap["OpenRestyCacheMaxSize"] = common.OpenRestyCacheMaxSize
	common.OptionMap["OpenRestyCacheKeyTemplate"] = common.OpenRestyCacheKeyTemplate
	common.OptionMap["OpenRestyCacheLockEnabled"] = strconv.FormatBool(common.OpenRestyCacheLockEnabled)
	common.OptionMap["OpenRestyCacheLockTimeout"] = common.OpenRestyCacheLockTimeout
	common.OptionMap["OpenRestyCacheUseStale"] = common.OpenRestyCacheUseStale
	common.OptionMap["OpenRestyMainConfigTemplate"] = common.OpenRestyMainConfigTemplate
	common.OptionMap["GlobalApiRateLimitNum"] = strconv.Itoa(common.GlobalApiRateLimitNum)
	common.OptionMap["GlobalApiRateLimitDuration"] = strconv.FormatInt(common.GlobalApiRateLimitDuration, 10)
	common.OptionMap["GlobalWebRateLimitNum"] = strconv.Itoa(common.GlobalWebRateLimitNum)
	common.OptionMap["GlobalWebRateLimitDuration"] = strconv.FormatInt(common.GlobalWebRateLimitDuration, 10)
	common.OptionMap["UploadRateLimitNum"] = strconv.Itoa(common.UploadRateLimitNum)
	common.OptionMap["UploadRateLimitDuration"] = strconv.FormatInt(common.UploadRateLimitDuration, 10)
	common.OptionMap["DownloadRateLimitNum"] = strconv.Itoa(common.DownloadRateLimitNum)
	common.OptionMap["DownloadRateLimitDuration"] = strconv.FormatInt(common.DownloadRateLimitDuration, 10)
	common.OptionMap["CriticalRateLimitNum"] = strconv.Itoa(common.CriticalRateLimitNum)
	common.OptionMap["CriticalRateLimitDuration"] = strconv.FormatInt(common.CriticalRateLimitDuration, 10)
	common.OptionMap["DNSWorkerAPIRateLimitNum"] = strconv.Itoa(common.DNSWorkerAPIRateLimitNum)
	common.OptionMap["DNSWorkerAPIRateLimitDuration"] = strconv.FormatInt(common.DNSWorkerAPIRateLimitDuration, 10)
	common.OptionMapRWMutex.Unlock()
	options, _ := AllOption()
	for _, option := range options {
		updateOptionMap(option.Key, option.Value)
	}
}

func UpdateOption(key string, value string) error {
	return UpdateOptions([]Option{{
		Key:   key,
		Value: value,
	}})
}

func UpdateOptions(options []Option) error {
	if len(options) == 0 {
		return nil
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range options {
			option := Option{
				Key: item.Key,
			}
			if err := tx.FirstOrCreate(&option, Option{Key: item.Key}).Error; err != nil {
				return err
			}
			option.Value = item.Value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, item := range options {
		updateOptionMap(item.Key, item.Value)
	}
	return nil
}

func updateOptionMap(key string, value string) {
	shouldRefreshGeoIP := false
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[key] = value
	if key == "AgentLegacyGlobalAuthEnabled" {
		common.OptionMap["AgentLegacyGlobalTokenEnabled"] = value
	} else if key == "AgentLegacyGlobalTokenEnabled" {
		common.OptionMap["AgentLegacyGlobalAuthEnabled"] = value
	}
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		}
	}
	if strings.HasSuffix(key, "Enabled") {
		boolValue := value == "true"
		switch key {
		case "PasswordRegisterEnabled":
			common.PasswordRegisterEnabled = boolValue
		case "PasswordLoginEnabled":
			common.PasswordLoginEnabled = boolValue
		case "EmailVerificationEnabled":
			common.EmailVerificationEnabled = boolValue
		case "GitHubOAuthEnabled":
			common.GitHubOAuthEnabled = boolValue
		case "WeChatAuthEnabled":
			common.WeChatAuthEnabled = boolValue
		case "TurnstileCheckEnabled":
			common.TurnstileCheckEnabled = boolValue
		}
	}
	switch key {
	case "SMTPServer":
		common.SMTPServer = value
	case "SMTPPort":
		intValue, _ := strconv.Atoi(value)
		common.SMTPPort = intValue
	case "SMTPAccount":
		common.SMTPAccount = value
	case "SMTPToken":
		common.SMTPToken = value
	case "ServerAddress":
		common.ServerAddress = value
	case "GitHubClientId":
		common.GitHubClientId = value
	case "GitHubClientSecret":
		common.GitHubClientSecret = value
	case "Footer":
		common.Footer = value
	case "HomePageLink":
		common.HomePageLink = value
	case "SystemName":
		common.SystemName = value
	case "WeChatServerAddress":
		common.WeChatServerAddress = value
	case "WeChatServerToken":
		common.WeChatServerToken = value
	case "WeChatAccountQRCodeImageURL":
		common.WeChatAccountQRCodeImageURL = value
	case "TurnstileSiteKey":
		common.TurnstileSiteKey = value
	case "TurnstileSecretKey":
		common.TurnstileSecretKey = value
	case "AgentLegacyGlobalTokenEnabled", "AgentLegacyGlobalAuthEnabled":
		common.AgentLegacyGlobalTokenEnabled = value == "true"
	case "AgentDiscoveryToken":
		common.AgentDiscoveryToken = value
	case "AgentHeartbeatInterval":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.AgentHeartbeatInterval = v
		}
	case "AgentWebsocketUpgradeEnabled":
		common.AgentWebsocketUpgradeEnabled = value == "true"
	case "NodeOfflineThreshold":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.NodeOfflineThreshold = time.Duration(v) * time.Millisecond
		}
	case "AgentUpdateRepo":
		if value != "" {
			common.AgentUpdateRepo = value
		}
	case "GeoIPProvider":
		if geoip.IsValidProvider(value) {
			common.GeoIPProvider = value
			shouldRefreshGeoIP = true
		}
	case "DatabaseAutoCleanupEnabled":
		common.DatabaseAutoCleanupEnabled = value == "true"
	case "DatabaseAutoCleanupRetentionDays":
		if v, err := strconv.Atoi(value); err == nil && v >= 1 {
			common.DatabaseAutoCleanupRetentionDays = v
		}
	case "CloudflareDDoSRequestThreshold":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.CloudflareDDoSRequestThreshold = v
		}
	case "CloudflareDDoSErrorRateThreshold":
		if v, err := strconv.ParseFloat(value, 64); err == nil && v > 0 {
			common.CloudflareDDoSErrorRateThreshold = v
		}
	case "AuthoritativeDNSEnabled":
		common.AuthoritativeDNSEnabled = value == "true"
	case "AuthoritativeDNSListenAddr":
		if strings.TrimSpace(value) != "" {
			common.AuthoritativeDNSListenAddr = value
		}
	case "AuthoritativeDNSDefaultTTL":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.AuthoritativeDNSDefaultTTL = v
		}
	case "AuthoritativeDNSSnapshotMaxAge":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.AuthoritativeDNSSnapshotMaxAge = v
		}
	case "AuthoritativeDNSWorkerQueryRateLimit":
		if v, err := strconv.Atoi(value); err == nil && v >= 0 {
			common.AuthoritativeDNSWorkerQueryRateLimit = v
		}
	case "AuthoritativeDNSWorkerResponseRateLimit":
		if v, err := strconv.Atoi(value); err == nil && v >= 0 {
			common.AuthoritativeDNSWorkerResponseRateLimit = v
		}
	case "AuthoritativeDNSWorkerUDPResponseSize":
		if v, err := strconv.Atoi(value); err == nil && v >= 512 && v <= 65535 {
			common.AuthoritativeDNSWorkerUDPResponseSize = v
		}
	case "AuthoritativeDNSWorkerECSEnabled":
		common.AuthoritativeDNSWorkerECSEnabled = value == "true"
	case "AuthoritativeDNSWorkerECSIPv4Prefix":
		if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 32 {
			common.AuthoritativeDNSWorkerECSIPv4Prefix = v
		}
	case "AuthoritativeDNSWorkerECSIPv6Prefix":
		if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 128 {
			common.AuthoritativeDNSWorkerECSIPv6Prefix = v
		}
	case "GSLBMetricFreshnessSeconds":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.GSLBMetricFreshnessSeconds = v
		}
	case "GSLBProbeSchedulingEnabled":
		common.GSLBProbeSchedulingEnabled = value == "true"
	case "OpenRestyWorkerProcesses":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyWorkerProcesses = value
		}
	case "OpenRestyWorkerConnections":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyWorkerConnections = v
		}
	case "OpenRestyWorkerRlimitNofile":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyWorkerRlimitNofile = v
		}
	case "OpenRestyEventsUse":
		common.OpenRestyEventsUse = value
	case "OpenRestyResolvers":
		common.OpenRestyResolvers = value
	case "OpenRestyEventsMultiAcceptEnabled":
		common.OpenRestyEventsMultiAcceptEnabled = value == "true"
	case "OpenRestyKeepaliveTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyKeepaliveTimeout = v
		}
	case "OpenRestyKeepaliveRequests":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyKeepaliveRequests = v
		}
	case "OpenRestyClientHeaderTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyClientHeaderTimeout = v
		}
	case "OpenRestyClientBodyTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyClientBodyTimeout = v
		}
	case "OpenRestyClientMaxBodySize":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyClientMaxBodySize = value
		}
	case "OpenRestyLargeClientHeaderBuffers":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyLargeClientHeaderBuffers = value
		}
	case "OpenRestySendTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestySendTimeout = v
		}
	case "OpenRestyProxyConnectTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyProxyConnectTimeout = v
		}
	case "OpenRestyProxySendTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyProxySendTimeout = v
		}
	case "OpenRestyProxyReadTimeout":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyProxyReadTimeout = v
		}
	case "OpenRestyWebsocketEnabled":
		common.OpenRestyWebsocketEnabled = value == "true"
	case "OpenRestyProxyRequestBufferingEnabled":
		common.OpenRestyProxyRequestBufferingEnabled = value == "true"
	case "OpenRestyProxyBufferingEnabled":
		common.OpenRestyProxyBufferingEnabled = value == "true"
	case "OpenRestyProxyBuffers":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyProxyBuffers = value
		}
	case "OpenRestyProxyBufferSize":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyProxyBufferSize = value
		}
	case "OpenRestyProxyBusyBuffersSize":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyProxyBusyBuffersSize = value
		}
	case "OpenRestyGzipEnabled":
		common.OpenRestyGzipEnabled = value == "true"
	case "OpenRestyGzipMinLength":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyGzipMinLength = v
		}
	case "OpenRestyGzipCompLevel":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.OpenRestyGzipCompLevel = v
		}
	case "OpenRestyCacheEnabled":
		common.OpenRestyCacheEnabled = value == "true"
	case "OpenRestyCachePath":
		common.OpenRestyCachePath = value
	case "OpenRestyCacheLevels":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheLevels = value
		}
	case "OpenRestyCacheInactive":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheInactive = value
		}
	case "OpenRestyCacheMaxSize":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheMaxSize = value
		}
	case "OpenRestyCacheKeyTemplate":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheKeyTemplate = value
		}
	case "OpenRestyCacheLockEnabled":
		common.OpenRestyCacheLockEnabled = value == "true"
	case "OpenRestyCacheLockTimeout":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheLockTimeout = value
		}
	case "OpenRestyCacheUseStale":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyCacheUseStale = value
		}
	case "OpenRestyMainConfigTemplate":
		if strings.TrimSpace(value) != "" {
			common.OpenRestyMainConfigTemplate = value
		}
	case "GlobalApiRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.GlobalApiRateLimitNum = v
		}
	case "GlobalApiRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.GlobalApiRateLimitDuration = v
		}
	case "GlobalWebRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.GlobalWebRateLimitNum = v
		}
	case "GlobalWebRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.GlobalWebRateLimitDuration = v
		}
	case "UploadRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.UploadRateLimitNum = v
		}
	case "UploadRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.UploadRateLimitDuration = v
		}
	case "DownloadRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.DownloadRateLimitNum = v
		}
	case "DownloadRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.DownloadRateLimitDuration = v
		}
	case "CriticalRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.CriticalRateLimitNum = v
		}
	case "CriticalRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.CriticalRateLimitDuration = v
		}
	case "DNSWorkerAPIRateLimitNum":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			common.DNSWorkerAPIRateLimitNum = v
		}
	case "DNSWorkerAPIRateLimitDuration":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
			common.DNSWorkerAPIRateLimitDuration = v
		}
	}
	common.OptionMapRWMutex.Unlock()
	if shouldRefreshGeoIP {
		geoip.InitGeoIP()
	}
}
