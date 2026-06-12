package controller

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service"
	"dushengcdn/utils/geoip"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	openRestySizePattern          = regexp.MustCompile(`^\d+[kKmMgG]?$`)
	openRestyProxyBuffersPattern  = regexp.MustCompile(`^\d+\s+\d+[kKmMgG]?$`)
	openRestyCacheLevelsPattern   = regexp.MustCompile(`^\d{1,2}(?::\d{1,2}){0,2}$`)
	openRestyDurationTokenPattern = regexp.MustCompile(`^\d+[smhdwSMHDW]$`)
	openRestyResolversPattern     = regexp.MustCompile(`^[a-zA-Z0-9.:\-\s]+$`)
	githubRepoPattern             = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

type optionBatchPayload struct {
	Options []model.Option `json:"options"`
}

func validateRateLimitOption(key string, value string) error {
	maxDurationSeconds := int(common.RateLimitKeyExpirationDuration.Seconds())

	switch key {
	case "GlobalApiRateLimitNum", "GlobalWebRateLimitNum", "UploadRateLimitNum", "DownloadRateLimitNum", "CriticalRateLimitNum":
		intValue, err := strconv.Atoi(value)
		if err != nil || intValue <= 0 {
			return fmt.Errorf("%s 必须为大于 0 的整数", key)
		}
		return nil
	case "GlobalApiRateLimitDuration", "GlobalWebRateLimitDuration", "UploadRateLimitDuration", "DownloadRateLimitDuration", "CriticalRateLimitDuration":
		intValue, err := strconv.Atoi(value)
		if err != nil || intValue <= 0 {
			return fmt.Errorf("%s 必须为大于 0 的整数秒", key)
		}
		if intValue > maxDurationSeconds {
			return fmt.Errorf("%s 不能大于 %d 秒", key, maxDurationSeconds)
		}
		return nil
	default:
		return nil
	}
}

func validatePositiveIntegerOption(key string, value string) error {
	intValue, err := strconv.Atoi(value)
	if err != nil || intValue <= 0 {
		return fmt.Errorf("%s 必须为大于 0 的整数", key)
	}
	return nil
}

func validateBooleanOption(key string, value string) error {
	switch value {
	case "true", "false":
		return nil
	default:
		return fmt.Errorf("%s 必须为 true 或 false", key)
	}
}

func validateGeoIPOption(key string, value string) error {
	if key != "GeoIPProvider" {
		return nil
	}
	if !geoip.IsValidProvider(value) {
		return fmt.Errorf("%s 仅支持 disabled、mmdb、ip-api、geojs、ipinfo", key)
	}
	return nil
}

func validateDatabaseCleanupOption(key string, value string) error {
	switch key {
	case "DatabaseAutoCleanupEnabled":
		return validateBooleanOption(key, value)
	case "DatabaseAutoCleanupRetentionDays":
		intValue, err := strconv.Atoi(value)
		if err != nil || intValue < 1 {
			return fmt.Errorf("%s 必须为大于等于 1 的整数天", key)
		}
		return nil
	default:
		return nil
	}
}

func validateCloudflareOption(key string, value string) error {
	switch key {
	case "CloudflareDDoSRequestThreshold":
		intValue, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || intValue <= 0 {
			return fmt.Errorf("%s 必须为大于 0 的整数", key)
		}
		return nil
	case "CloudflareDDoSErrorRateThreshold":
		floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || floatValue <= 0 || floatValue > 100 {
			return fmt.Errorf("%s 必须为 0 到 100 之间的数字", key)
		}
		return nil
	default:
		return nil
	}
}

func validateAgentOption(key string, value string) error {
	switch key {
	case "AgentUpdateRepo":
		if !githubRepoPattern.MatchString(strings.TrimSpace(value)) {
			return fmt.Errorf("%s 必须为 owner/repo 格式", key)
		}
		return nil
	case "AgentWebsocketUpgradeEnabled", "AgentLegacyGlobalTokenEnabled", "AgentLegacyGlobalAuthEnabled":
		return validateBooleanOption(key, strings.TrimSpace(value))
	default:
		return nil
	}
}

func validateAuthoritativeDNSOption(key string, value string) error {
	trimmed := strings.TrimSpace(value)
	switch key {
	case "AuthoritativeDNSEnabled":
		return validateBooleanOption(key, trimmed)
	case "AuthoritativeDNSListenAddr":
		if trimmed == "" {
			return fmt.Errorf("%s 不能为空", key)
		}
		if strings.ContainsAny(trimmed, "\r\n\t ") {
			return fmt.Errorf("%s 不能包含空白字符", key)
		}
		return nil
	case "AuthoritativeDNSDefaultTTL":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue <= 0 || intValue > 86400 {
			return fmt.Errorf("%s 必须为 1 到 86400 之间的整数秒", key)
		}
		return nil
	case "AuthoritativeDNSSnapshotMaxAge":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue <= 0 {
			return fmt.Errorf("%s 必须为大于 0 的整数秒", key)
		}
		return nil
	case "AuthoritativeDNSWorkerQueryRateLimit", "AuthoritativeDNSWorkerResponseRateLimit":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue < 0 {
			return fmt.Errorf("%s 必须为大于或等于 0 的整数，0 表示关闭", key)
		}
		return nil
	case "AuthoritativeDNSWorkerUDPResponseSize":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue < 512 || intValue > 65535 {
			return fmt.Errorf("%s 必须为 512 到 65535 之间的整数字节", key)
		}
		return nil
	case "AuthoritativeDNSWorkerECSEnabled":
		return validateBooleanOption(key, trimmed)
	case "AuthoritativeDNSWorkerECSIPv4Prefix":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue < 0 || intValue > 32 {
			return fmt.Errorf("%s 必须为 0 到 32 之间的整数", key)
		}
		return nil
	case "AuthoritativeDNSWorkerECSIPv6Prefix":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue < 0 || intValue > 128 {
			return fmt.Errorf("%s 必须为 0 到 128 之间的整数", key)
		}
		return nil
	case "GSLBMetricFreshnessSeconds":
		intValue, err := strconv.Atoi(trimmed)
		if err != nil || intValue <= 0 {
			return fmt.Errorf("%s 必须为大于 0 的整数秒", key)
		}
		return nil
	case "GSLBProbeSchedulingEnabled":
		return validateBooleanOption(key, trimmed)
	default:
		return nil
	}
}

func validateOpenRestyOption(key string, value string) error {
	trimmed := strings.TrimSpace(value)

	switch key {
	case "OpenRestyWorkerProcesses":
		if trimmed == "auto" {
			return nil
		}
		return validatePositiveIntegerOption(key, trimmed)
	case "OpenRestyWorkerConnections",
		"OpenRestyWorkerRlimitNofile",
		"OpenRestyKeepaliveTimeout",
		"OpenRestyKeepaliveRequests",
		"OpenRestyClientHeaderTimeout",
		"OpenRestyClientBodyTimeout",
		"OpenRestySendTimeout",
		"OpenRestyProxyConnectTimeout",
		"OpenRestyProxySendTimeout",
		"OpenRestyProxyReadTimeout",
		"OpenRestyGzipMinLength":
		return validatePositiveIntegerOption(key, trimmed)
	case "OpenRestyGzipCompLevel":
		if err := validatePositiveIntegerOption(key, trimmed); err != nil {
			return err
		}
		level, _ := strconv.Atoi(trimmed)
		if level > 9 {
			return fmt.Errorf("%s 不能大于 9", key)
		}
		return nil
	case "OpenRestyEventsUse":
		if trimmed == "" {
			return nil
		}
		switch trimmed {
		case "epoll", "kqueue", "poll", "select", "rtsig", "/dev/poll", "eventport":
			return nil
		default:
			return fmt.Errorf("%s 仅支持 epoll、kqueue、poll、select、rtsig、/dev/poll、eventport 或留空", key)
		}
	case "OpenRestyResolvers":
		if trimmed == "" {
			return nil
		}
		if !openRestyResolversPattern.MatchString(trimmed) {
			return fmt.Errorf("%s 包含非法字符，请填入有效的 IP 地址或域名，以空格分隔", key)
		}
		return nil
	case "OpenRestyEventsMultiAcceptEnabled",
		"OpenRestyWebsocketEnabled",
		"OpenRestyProxyRequestBufferingEnabled",
		"OpenRestyProxyBufferingEnabled",
		"OpenRestyGzipEnabled",
		"OpenRestyCacheEnabled",
		"OpenRestyCacheLockEnabled":
		return validateBooleanOption(key, trimmed)
	case "OpenRestyProxyBuffers", "OpenRestyLargeClientHeaderBuffers":
		if openRestyProxyBuffersPattern.MatchString(trimmed) {
			return nil
		}
		return fmt.Errorf("%s 格式必须类似 \"16 16k\"", key)
	case "OpenRestyProxyBufferSize", "OpenRestyProxyBusyBuffersSize", "OpenRestyCacheMaxSize", "OpenRestyClientMaxBodySize":
		if openRestySizePattern.MatchString(trimmed) {
			return nil
		}
		return fmt.Errorf("%s 格式必须为整数或带 k/m/g 单位的大小值", key)
	case "OpenRestyCachePath":
		if strings.ContainsAny(trimmed, "\r\n\t") {
			return fmt.Errorf("%s 不能包含换行或制表符", key)
		}
		if err := service.ValidateAgentCachePath(trimmed); err != nil {
			return err
		}
		return nil
	case "OpenRestyCacheLevels":
		if openRestyCacheLevelsPattern.MatchString(trimmed) {
			return nil
		}
		return fmt.Errorf("%s 格式必须类似 \"1:2\" 或 \"1:2:2\"", key)
	case "OpenRestyCacheInactive", "OpenRestyCacheLockTimeout":
		if openRestyDurationTokenPattern.MatchString(trimmed) {
			return nil
		}
		return fmt.Errorf("%s 格式必须为带单位的时长，例如 30m 或 5s", key)
	case "OpenRestyCacheKeyTemplate":
		if trimmed == "" {
			return fmt.Errorf("%s 不能为空", key)
		}
		if strings.ContainsAny(trimmed, "\r\n") {
			return fmt.Errorf("%s 不能包含换行", key)
		}
		return nil
	case "OpenRestyCacheUseStale":
		if trimmed == "" {
			return fmt.Errorf("%s 不能为空", key)
		}
		allowedTokens := map[string]struct{}{
			"error": {}, "timeout": {}, "invalid_header": {}, "updating": {},
			"http_500": {}, "http_502": {}, "http_503": {}, "http_504": {},
			"http_403": {}, "http_404": {}, "http_429": {}, "off": {},
		}
		for _, token := range strings.Fields(trimmed) {
			if _, ok := allowedTokens[token]; !ok {
				return fmt.Errorf("%s 包含不支持的值 %q", key, token)
			}
		}
		return nil
	case "OpenRestyMainConfigTemplate":
		return service.ValidateOpenRestyMainConfigTemplate(value)
	default:
		return nil
	}
}

func buildOptionValidationState(options []model.Option) map[string]string {
	state := common.OptionMapSnapshot()
	for _, option := range options {
		state[option.Key] = option.Value
	}
	return state
}

func validateOptionWithState(option model.Option, state map[string]string) error {
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && strings.TrimSpace(state["GitHubClientId"]) == "" {
			return fmt.Errorf("无法启用 GitHub OAuth，请先填入 GitHub Client ID 以及 GitHub Client Secret！")
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && strings.TrimSpace(state["WeChatServerAddress"]) == "" {
			return fmt.Errorf("无法启用微信登录，请先填入微信登录相关配置信息！")
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && strings.TrimSpace(state["TurnstileSiteKey"]) == "" {
			return fmt.Errorf("无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！")
		}
	}

	if err := validateRateLimitOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateOpenRestyOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateGeoIPOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateDatabaseCleanupOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateCloudflareOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateAgentOption(option.Key, option.Value); err != nil {
		return err
	}
	if err := validateAuthoritativeDNSOption(option.Key, option.Value); err != nil {
		return err
	}
	return nil
}

func updateOptions(options []model.Option) error {
	if len(options) == 0 {
		return fmt.Errorf("无效的参数")
	}

	state := buildOptionValidationState(options)
	for _, option := range options {
		if strings.TrimSpace(option.Key) == "" {
			return fmt.Errorf("无效的参数")
		}
		if err := validateOptionWithState(option, state); err != nil {
			return err
		}
	}

	return model.UpdateOptions(options)
}

// GetOptions godoc
// @Summary List editable options
// @Tags Options
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/option/ [get]
func GetOptions(c *gin.Context) {
	var options []*model.Option
	for k, v := range common.OptionMapSnapshot() {
		if isSensitiveOptionKey(k) {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: v,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

func isSensitiveOptionKey(key string) bool {
	if key == "AgentLegacyGlobalTokenEnabled" {
		return false
	}
	return strings.Contains(key, "Token") || strings.Contains(key, "Secret")
}

// UpdateOption godoc
// @Summary Update option
// @Tags Options
// @Accept json
// @Produce json
// @Param payload body model.Option true "Option payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/option/update [post]
func UpdateOption(c *gin.Context) {
	var option model.Option
	err := json.NewDecoder(c.Request.Body).Decode(&option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	// Reuse the batch path so single-key updates share one validation chain.
	if err = updateOptions([]model.Option{option}); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// UpdateOptionsBatch godoc
// @Summary Batch update options
// @Tags Options
// @Accept json
// @Produce json
// @Param payload body optionBatchPayload true "Batch option payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/option/update-batch [post]
func UpdateOptionsBatch(c *gin.Context) {
	var payload optionBatchPayload
	if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil || len(payload.Options) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	if err := updateOptions(payload.Options); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
