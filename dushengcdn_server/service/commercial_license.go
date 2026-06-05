package service

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
)

const (
	commercialLicenseTokenPrefix     = "dscdn_license_v1."
	CommercialLicenseStatusCommunity = "community"
	CommercialLicenseStatusMissing   = "missing"
	CommercialLicenseStatusValid     = "valid"
	CommercialLicenseStatusExpiring  = "expiring"
	CommercialLicenseStatusExpired   = "expired"
	CommercialLicenseStatusInvalid   = "invalid"

	CommercialFeatureACMEAutomation   = "acme-automation"
	CommercialFeatureAuthoritativeDNS = "authoritative-dns"
	CommercialFeatureCloudflareDNS    = "cloudflare-dns"
	CommercialFeatureGSLB             = "gslb"
	CommercialFeatureDDoSProtection   = "ddos-protection"
	CommercialFeatureWAF              = "waf"
	CommercialFeatureCCProtection     = "cc-protection"
	CommercialFeatureGeoAccessControl = "geo-access-control"
	commercialFeatureAll              = "*"
)

var commercialLicenseNow = time.Now

type CommercialLicenseInstallInput struct {
	Token string `json:"token"`
}

type CommercialLicensePayload struct {
	LicenseID    string     `json:"license_id"`
	CustomerID   string     `json:"customer_id"`
	CustomerName string     `json:"customer_name"`
	Plan         string     `json:"plan"`
	Features     []string   `json:"features"`
	MaxNodes     int        `json:"max_nodes"`
	MaxSites     int        `json:"max_sites"`
	IssuedAt     *time.Time `json:"issued_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

type CommercialLicenseView struct {
	Status              string     `json:"status"`
	StatusLabel         string     `json:"status_label"`
	Licensed            bool       `json:"licensed"`
	Required            bool       `json:"required"`
	LicenseID           string     `json:"license_id"`
	CustomerID          string     `json:"customer_id"`
	CustomerName        string     `json:"customer_name"`
	Plan                string     `json:"plan"`
	PlanLabel           string     `json:"plan_label"`
	Fingerprint         string     `json:"fingerprint"`
	Features            []string   `json:"features"`
	MaxNodes            int        `json:"max_nodes"`
	MaxSites            int        `json:"max_sites"`
	CurrentNodes        int64      `json:"current_nodes"`
	CurrentSites        int64      `json:"current_sites"`
	NodeLimitExceeded   bool       `json:"node_limit_exceeded"`
	SiteLimitExceeded   bool       `json:"site_limit_exceeded"`
	CanCreateNodes      bool       `json:"can_create_nodes"`
	CanCreateSites      bool       `json:"can_create_sites"`
	IssuedAt            *time.Time `json:"issued_at"`
	ExpiresAt           *time.Time `json:"expires_at"`
	DaysUntilExpiry     *int       `json:"days_until_expiry"`
	LastValidatedAt     *time.Time `json:"last_validated_at"`
	LastValidationError string     `json:"last_validation_error"`
	SignatureVerified   bool       `json:"signature_verified"`
}

type parsedCommercialLicense struct {
	payload           CommercialLicensePayload
	rawPayload        []byte
	status            string
	validationError   string
	signatureVerified bool
}

func GetCommercialLicenseStatus() (*CommercialLicenseView, error) {
	license, err := model.GetCommercialLicense()
	if err != nil {
		return nil, err
	}
	return buildCommercialLicenseView(license)
}

func InstallCommercialLicense(input CommercialLicenseInstallInput) (*CommercialLicenseView, error) {
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return nil, errors.New("许可证内容不能为空")
	}
	parsed, err := parseCommercialLicenseToken(token, commercialLicenseNow())
	if err != nil {
		return nil, err
	}
	if parsed.status == CommercialLicenseStatusInvalid {
		return nil, errors.New(parsed.validationError)
	}
	now := commercialLicenseNow().UTC()
	license := commercialLicenseModelFromParsed(token, parsed, now)
	if err := model.SaveCommercialLicense(license); err != nil {
		return nil, err
	}
	return buildCommercialLicenseView(license)
}

func ClearCommercialLicense() (*CommercialLicenseView, error) {
	if err := model.DeleteCommercialLicense(); err != nil {
		return nil, err
	}
	return buildCommercialLicenseView(nil)
}

func EnsureCommercialResourceAvailable(resource string) error {
	view, err := GetCommercialLicenseStatus()
	if err != nil {
		return err
	}
	if view == nil {
		return nil
	}
	if view.Status == CommercialLicenseStatusCommunity && !view.Required {
		return nil
	}
	if view.Status == CommercialLicenseStatusValid || view.Status == CommercialLicenseStatusExpiring {
		switch resource {
		case "node":
			if !view.CanCreateNodes {
				return fmt.Errorf("当前授权最多允许 %d 个节点", view.MaxNodes)
			}
		case "site":
			if !view.CanCreateSites {
				return fmt.Errorf("当前授权最多允许 %d 个站点", view.MaxSites)
			}
		}
		return nil
	}
	return commercialLicenseStatusOperationError(view.Status)
}

func EnsureCommercialFeatureEnabled(feature string) error {
	return ensureCommercialFeaturesEnabled(feature)
}

func ensureCommercialFeaturesEnabled(features ...string) error {
	normalized := normalizeCommercialLicenseFeatures(features)
	if len(normalized) == 0 {
		return nil
	}
	view, err := GetCommercialLicenseStatus()
	if err != nil {
		return err
	}
	if view == nil {
		return nil
	}
	if view.Status == CommercialLicenseStatusCommunity && !view.Required {
		return nil
	}
	if view.Status != CommercialLicenseStatusValid && view.Status != CommercialLicenseStatusExpiring {
		return commercialLicenseStatusOperationError(view.Status)
	}
	for _, feature := range normalized {
		if !commercialLicenseFeaturesContain(view.Features, feature) {
			return fmt.Errorf("当前授权未包含 %s 能力", commercialLicenseFeatureLabel(feature))
		}
	}
	return nil
}

func commercialLicenseStatusOperationError(status string) error {
	switch status {
	case CommercialLicenseStatusMissing:
		return errors.New("当前部署要求安装有效商业许可证")
	case CommercialLicenseStatusExpired:
		return errors.New("商业许可证已过期，请更新授权")
	case CommercialLicenseStatusInvalid:
		return errors.New("商业许可证无效，请重新安装授权")
	default:
		return errors.New("商业许可证状态不允许执行该操作")
	}
}

func commercialLicenseFeaturesContain(features []string, feature string) bool {
	feature = normalizeCommercialLicenseFeature(feature)
	if feature == "" {
		return true
	}
	for _, item := range features {
		item = normalizeCommercialLicenseFeature(item)
		if item == feature || item == commercialFeatureAll || item == "all" {
			return true
		}
	}
	return false
}

func commercialLicenseModelFromParsed(token string, parsed *parsedCommercialLicense, now time.Time) *model.CommercialLicense {
	payload := parsed.payload
	featuresJSON, _ := json.Marshal(normalizeCommercialLicenseFeatures(payload.Features))
	return &model.CommercialLicense{
		LicenseID:           strings.TrimSpace(payload.LicenseID),
		CustomerID:          strings.TrimSpace(payload.CustomerID),
		CustomerName:        strings.TrimSpace(payload.CustomerName),
		Plan:                normalizeCommercialLicensePlan(payload.Plan),
		Status:              parsed.status,
		Token:               token,
		FeaturesJSON:        string(featuresJSON),
		MaxNodes:            normalizeCommercialLicenseLimit(payload.MaxNodes),
		MaxSites:            normalizeCommercialLicenseLimit(payload.MaxSites),
		IssuedAt:            payload.IssuedAt,
		ExpiresAt:           payload.ExpiresAt,
		LastValidatedAt:     &now,
		LastValidationError: parsed.validationError,
	}
}

func buildCommercialLicenseView(license *model.CommercialLicense) (*CommercialLicenseView, error) {
	nodeCount, siteCount, err := commercialLicenseUsageCounts()
	if err != nil {
		return nil, err
	}
	if license == nil {
		status := CommercialLicenseStatusCommunity
		if common.CommercialLicenseRequired {
			status = CommercialLicenseStatusMissing
		}
		return buildCommercialLicenseViewFromParts(CommercialLicensePayload{}, nil, nodeCount, siteCount, status, "", false), nil
	}

	parsed, err := parseCommercialLicenseToken(license.Token, commercialLicenseNow())
	if err != nil {
		status := CommercialLicenseStatusInvalid
		message := err.Error()
		payload := CommercialLicensePayload{
			LicenseID:    license.LicenseID,
			CustomerID:   license.CustomerID,
			CustomerName: license.CustomerName,
			Plan:         license.Plan,
			Features:     decodeCommercialLicenseFeatures(license.FeaturesJSON),
			MaxNodes:     license.MaxNodes,
			MaxSites:     license.MaxSites,
			IssuedAt:     license.IssuedAt,
			ExpiresAt:    license.ExpiresAt,
		}
		view := buildCommercialLicenseViewFromParts(payload, license, nodeCount, siteCount, status, message, false)
		view.Fingerprint = license.Fingerprint
		return view, nil
	}
	view := buildCommercialLicenseViewFromParts(
		parsed.payload,
		license,
		nodeCount,
		siteCount,
		parsed.status,
		parsed.validationError,
		parsed.signatureVerified,
	)
	if view.Fingerprint == "" {
		view.Fingerprint = commercialLicenseFingerprint(license.Token)
	}
	return view, nil
}

func buildCommercialLicenseViewFromParts(payload CommercialLicensePayload, license *model.CommercialLicense, nodeCount int64, siteCount int64, status string, validationError string, signatureVerified bool) *CommercialLicenseView {
	features := normalizeCommercialLicenseFeatures(payload.Features)
	maxNodes := normalizeCommercialLicenseLimit(payload.MaxNodes)
	maxSites := normalizeCommercialLicenseLimit(payload.MaxSites)
	view := &CommercialLicenseView{
		Status:              status,
		StatusLabel:         commercialLicenseStatusLabel(status),
		Licensed:            status == CommercialLicenseStatusValid || status == CommercialLicenseStatusExpiring,
		Required:            common.CommercialLicenseRequired,
		LicenseID:           strings.TrimSpace(payload.LicenseID),
		CustomerID:          strings.TrimSpace(payload.CustomerID),
		CustomerName:        strings.TrimSpace(payload.CustomerName),
		Plan:                normalizeCommercialLicensePlan(payload.Plan),
		Features:            features,
		MaxNodes:            maxNodes,
		MaxSites:            maxSites,
		CurrentNodes:        nodeCount,
		CurrentSites:        siteCount,
		IssuedAt:            payload.IssuedAt,
		ExpiresAt:           payload.ExpiresAt,
		LastValidationError: validationError,
		SignatureVerified:   signatureVerified,
	}
	view.PlanLabel = commercialLicensePlanLabel(view.Plan)
	view.NodeLimitExceeded = maxNodes > 0 && nodeCount > int64(maxNodes)
	view.SiteLimitExceeded = maxSites > 0 && siteCount > int64(maxSites)
	view.CanCreateNodes = view.Licensed && (maxNodes <= 0 || nodeCount < int64(maxNodes))
	view.CanCreateSites = view.Licensed && (maxSites <= 0 || siteCount < int64(maxSites))
	if status == CommercialLicenseStatusCommunity && !view.Required {
		view.CanCreateNodes = true
		view.CanCreateSites = true
	}
	if license != nil {
		view.Fingerprint = strings.TrimSpace(license.Fingerprint)
		view.LastValidatedAt = license.LastValidatedAt
	}
	if view.Fingerprint == "" && license != nil {
		view.Fingerprint = commercialLicenseFingerprint(license.Token)
	}
	if payload.ExpiresAt != nil {
		days := int(time.Until(payload.ExpiresAt.UTC()).Hours() / 24)
		view.DaysUntilExpiry = &days
	}
	return view
}

func parseCommercialLicenseToken(token string, now time.Time) (*parsedCommercialLicense, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("许可证内容不能为空")
	}

	payloadBytes, accepted, signatureVerified, err := decodeCommercialLicenseToken(token)
	if err != nil {
		return nil, err
	}

	var payload CommercialLicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("许可证载荷格式无效")
	}
	payload = normalizeCommercialLicensePayload(payload)
	status, validationError := validateCommercialLicensePayload(payload, now)
	if !accepted && status != CommercialLicenseStatusInvalid {
		status = CommercialLicenseStatusInvalid
		validationError = "许可证未通过签名校验"
	}
	return &parsedCommercialLicense{
		payload:           payload,
		rawPayload:        payloadBytes,
		status:            status,
		validationError:   validationError,
		signatureVerified: signatureVerified,
	}, nil
}

func decodeCommercialLicenseToken(token string) ([]byte, bool, bool, error) {
	if strings.HasPrefix(token, "{") {
		if commercialLicenseAllowUnsigned() {
			return []byte(token), true, false, nil
		}
		return nil, false, false, errors.New("未签名许可证未启用")
	}

	compact := strings.TrimPrefix(token, commercialLicenseTokenPrefix)
	parts := strings.Split(compact, ".")
	if len(parts) != 2 {
		return nil, false, false, errors.New("许可证格式无效")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false, false, errors.New("许可证载荷编码无效")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false, false, errors.New("许可证签名编码无效")
	}
	publicKeys, err := commercialLicensePublicKeys()
	if err != nil {
		return nil, false, false, err
	}
	if len(publicKeys) == 0 {
		if commercialLicenseAllowUnsigned() {
			return payloadBytes, true, false, nil
		}
		return nil, false, false, errors.New("未配置许可证公钥")
	}
	for _, publicKey := range publicKeys {
		if ed25519.Verify(publicKey, payloadBytes, signature) {
			return payloadBytes, true, true, nil
		}
	}
	return nil, false, false, errors.New("许可证签名无效")
}

func validateCommercialLicensePayload(payload CommercialLicensePayload, now time.Time) (string, string) {
	if payload.LicenseID == "" {
		return CommercialLicenseStatusInvalid, "许可证缺少 license_id"
	}
	if payload.CustomerName == "" && payload.CustomerID == "" {
		return CommercialLicenseStatusInvalid, "许可证缺少客户信息"
	}
	if payload.Plan == "" {
		return CommercialLicenseStatusInvalid, "许可证缺少授权版本"
	}
	if payload.IssuedAt != nil && payload.IssuedAt.After(now.Add(5*time.Minute)) {
		return CommercialLicenseStatusInvalid, "许可证签发时间晚于当前时间"
	}
	if payload.ExpiresAt != nil && !payload.ExpiresAt.After(now) {
		return CommercialLicenseStatusExpired, "许可证已过期"
	}
	if payload.ExpiresAt != nil && payload.ExpiresAt.Sub(now) <= 14*24*time.Hour {
		return CommercialLicenseStatusExpiring, ""
	}
	return CommercialLicenseStatusValid, ""
}

func normalizeCommercialLicensePayload(payload CommercialLicensePayload) CommercialLicensePayload {
	payload.LicenseID = truncateForDatabase(strings.TrimSpace(payload.LicenseID), 128)
	payload.CustomerID = truncateForDatabase(strings.TrimSpace(payload.CustomerID), 128)
	payload.CustomerName = truncateForDatabase(strings.TrimSpace(payload.CustomerName), 255)
	payload.Plan = normalizeCommercialLicensePlan(payload.Plan)
	payload.Features = normalizeCommercialLicenseFeatures(payload.Features)
	payload.MaxNodes = normalizeCommercialLicenseLimit(payload.MaxNodes)
	payload.MaxSites = normalizeCommercialLicenseLimit(payload.MaxSites)
	return payload
}

func normalizeCommercialLicensePlan(plan string) string {
	plan = strings.ToLower(strings.TrimSpace(plan))
	if plan == "" {
		return ""
	}
	if len(plan) > 64 {
		return plan[:64]
	}
	return plan
}

func normalizeCommercialLicenseLimit(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}

func normalizeCommercialLicenseFeatures(features []string) []string {
	seen := make(map[string]struct{}, len(features))
	result := make([]string, 0, len(features))
	for _, feature := range features {
		feature = normalizeCommercialLicenseFeature(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; ok {
			continue
		}
		seen[feature] = struct{}{}
		result = append(result, feature)
	}
	return result
}

func normalizeCommercialLicenseFeature(feature string) string {
	feature = strings.ToLower(strings.TrimSpace(feature))
	feature = strings.ReplaceAll(feature, "_", "-")
	if len(feature) > 64 {
		feature = feature[:64]
	}
	switch feature {
	case "all", commercialFeatureAll:
		return feature
	case "acme", "tls-acme", "certificate-automation":
		return CommercialFeatureACMEAutomation
	case "authoritative-dns", "dns-authoritative", "local-dns":
		return CommercialFeatureAuthoritativeDNS
	case "cloudflare", "cloudflare-dns-automation":
		return CommercialFeatureCloudflareDNS
	case "ddos", "ddos-auto":
		return CommercialFeatureDDoSProtection
	case "cc", "cc-defense":
		return CommercialFeatureCCProtection
	case "geo-acl", "region-restriction":
		return CommercialFeatureGeoAccessControl
	default:
		return feature
	}
}

func decodeCommercialLicenseFeatures(raw string) []string {
	var features []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &features); err != nil {
		return []string{}
	}
	return normalizeCommercialLicenseFeatures(features)
}

func commercialLicensePublicKeys() ([]ed25519.PublicKey, error) {
	raw := strings.TrimSpace(common.CommercialLicensePublicKeys)
	if envRaw := strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_PUBLIC_KEYS")); envRaw != "" {
		raw = envRaw
	}
	if raw == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	keys := make([]ed25519.PublicKey, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		decoded, err := base64.RawURLEncoding.DecodeString(field)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(field)
		}
		if err != nil {
			decoded, err = hex.DecodeString(field)
		}
		if err != nil {
			return nil, errors.New("许可证公钥编码无效")
		}
		if len(decoded) != ed25519.PublicKeySize {
			return nil, errors.New("许可证公钥长度无效")
		}
		keys = append(keys, ed25519.PublicKey(decoded))
	}
	return keys, nil
}

func commercialLicenseAllowUnsigned() bool {
	if common.CommercialLicenseAllowUnsigned {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_ALLOW_UNSIGNED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func commercialLicenseUsageCounts() (int64, int64, error) {
	var nodeCount int64
	if err := model.DB.Model(&model.Node{}).Count(&nodeCount).Error; err != nil {
		return 0, 0, err
	}
	var siteCount int64
	if err := model.DB.Model(&model.ProxyRoute{}).Count(&siteCount).Error; err != nil {
		return 0, 0, err
	}
	return nodeCount, siteCount, nil
}

func commercialLicenseFingerprint(token string) string {
	hash := model.CommercialLicenseTokenHash(token)
	if len(hash) < 16 {
		return ""
	}
	return hash[:16]
}

func commercialLicenseStatusLabel(status string) string {
	switch status {
	case CommercialLicenseStatusCommunity:
		return "社区模式"
	case CommercialLicenseStatusMissing:
		return "未安装授权"
	case CommercialLicenseStatusValid:
		return "授权有效"
	case CommercialLicenseStatusExpiring:
		return "即将到期"
	case CommercialLicenseStatusExpired:
		return "授权过期"
	case CommercialLicenseStatusInvalid:
		return "授权无效"
	default:
		return "未知状态"
	}
}

func commercialLicensePlanLabel(plan string) string {
	normalized := normalizeCommercialLicensePlan(plan)
	switch normalized {
	case "enterprise":
		return "企业版"
	case "business":
		return "商业版"
	case "professional":
		return "专业版"
	case "community":
		return "社区版"
	case "":
		return "未授权"
	default:
		return strings.ToUpper(normalized[:1]) + normalized[1:]
	}
}

func commercialLicenseFeatureLabel(feature string) string {
	switch normalizeCommercialLicenseFeature(feature) {
	case CommercialFeatureACMEAutomation:
		return "ACME 自动证书"
	case CommercialFeatureAuthoritativeDNS:
		return "自建权威 DNS"
	case CommercialFeatureCloudflareDNS:
		return "Cloudflare DNS 自动化"
	case CommercialFeatureGSLB:
		return "GSLB 智能调度"
	case CommercialFeatureDDoSProtection:
		return "DDoS 自动防护"
	case CommercialFeatureWAF:
		return "WAF"
	case CommercialFeatureCCProtection:
		return "CC 防护"
	case CommercialFeatureGeoAccessControl:
		return "区域访问控制"
	default:
		return feature
	}
}
