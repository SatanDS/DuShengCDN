package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	commercialLicenseTokenPrefix              = "dscdn_license_v1."
	commercialLicenseLeaseTokenPrefix         = "dscdn_lease_v1."
	CommercialLicenseStatusCommunity          = "community"
	CommercialLicenseStatusMissing            = "missing"
	CommercialLicenseStatusValid              = "valid"
	CommercialLicenseStatusExpiring           = "expiring"
	CommercialLicenseStatusExpired            = "expired"
	CommercialLicenseStatusInvalid            = "invalid"
	CommercialLicenseStatusActivationRequired = "activation_required"
	CommercialLicenseStatusLeaseExpired       = "lease_expired"

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
var commercialLicenseResourceMu sync.Mutex
var commercialLicenseHTTPClient = &http.Client{Timeout: 15 * time.Second}

type CommercialLicenseInstallInput struct {
	Token string `json:"token"`
}

type CommercialLicenseActivateInput struct {
	ActivationURL string `json:"activation_url"`
}

type CommercialLicenseIssueInput struct {
	LicenseID    string   `json:"license_id"`
	CustomerID   string   `json:"customer_id"`
	CustomerName string   `json:"customer_name"`
	Plan         string   `json:"plan"`
	Features     []string `json:"features"`
	MaxNodes     int      `json:"max_nodes"`
	MaxSites     int      `json:"max_sites"`
	IssuedAt     string   `json:"issued_at"`
	ExpiresAt    string   `json:"expires_at"`
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

type CommercialLicenseLeasePayload struct {
	LeaseID            string     `json:"lease_id"`
	ActivationID       string     `json:"activation_id"`
	LicenseID          string     `json:"license_id"`
	CustomerID         string     `json:"customer_id"`
	MachineFingerprint string     `json:"machine_fingerprint"`
	ServerVersion      string     `json:"server_version"`
	BuildWatermark     string     `json:"build_watermark"`
	IssuedAt           *time.Time `json:"issued_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

type CommercialLicenseView struct {
	Status                   string     `json:"status"`
	StatusLabel              string     `json:"status_label"`
	Licensed                 bool       `json:"licensed"`
	Required                 bool       `json:"required"`
	LicenseID                string     `json:"license_id"`
	CustomerID               string     `json:"customer_id"`
	CustomerName             string     `json:"customer_name"`
	Plan                     string     `json:"plan"`
	PlanLabel                string     `json:"plan_label"`
	Fingerprint              string     `json:"fingerprint"`
	Features                 []string   `json:"features"`
	MaxNodes                 int        `json:"max_nodes"`
	MaxSites                 int        `json:"max_sites"`
	CurrentNodes             int64      `json:"current_nodes"`
	CurrentSites             int64      `json:"current_sites"`
	NodeLimitExceeded        bool       `json:"node_limit_exceeded"`
	SiteLimitExceeded        bool       `json:"site_limit_exceeded"`
	CanCreateNodes           bool       `json:"can_create_nodes"`
	CanCreateSites           bool       `json:"can_create_sites"`
	IssuedAt                 *time.Time `json:"issued_at"`
	ExpiresAt                *time.Time `json:"expires_at"`
	DaysUntilExpiry          *int       `json:"days_until_expiry"`
	OnlineActivationRequired bool       `json:"online_activation_required"`
	ActivationConfigured     bool       `json:"activation_configured"`
	ActivationID             string     `json:"activation_id"`
	MachineFingerprint       string     `json:"machine_fingerprint"`
	LeaseExpiresAt           *time.Time `json:"lease_expires_at"`
	LeaseRenewBeforeAt       *time.Time `json:"lease_renew_before_at"`
	LastLeaseRenewedAt       *time.Time `json:"last_lease_renewed_at"`
	LeaseStatus              string     `json:"lease_status"`
	LeaseStatusLabel         string     `json:"lease_status_label"`
	LeaseSecondsRemaining    int64      `json:"lease_seconds_remaining"`
	BuildWatermark           string     `json:"build_watermark"`
	LastValidatedAt          *time.Time `json:"last_validated_at"`
	LastValidationError      string     `json:"last_validation_error"`
	SignatureVerified        bool       `json:"signature_verified"`
}

type CommercialLicenseIssuerStatus struct {
	Available            bool   `json:"available"`
	PublicKey            string `json:"public_key"`
	PublicKeyFingerprint string `json:"public_key_fingerprint"`
	Message              string `json:"message"`
}

type CommercialLicenseIssueResult struct {
	Token                string                   `json:"token"`
	Payload              CommercialLicensePayload `json:"payload"`
	Status               string                   `json:"status"`
	StatusLabel          string                   `json:"status_label"`
	PublicKey            string                   `json:"public_key"`
	PublicKeyFingerprint string                   `json:"public_key_fingerprint"`
	SignatureVerified    bool                     `json:"signature_verified"`
}

type CommercialLicenseActivationRequest struct {
	LicenseToken       string `json:"license_token"`
	LeaseToken         string `json:"lease_token"`
	ActivationID       string `json:"activation_id"`
	MachineFingerprint string `json:"machine_fingerprint"`
	ServerVersion      string `json:"server_version"`
	BuildWatermark     string `json:"build_watermark"`
	InstanceHostname   string `json:"instance_hostname"`
}

type CommercialLicenseActivationResponse struct {
	LeaseToken     string    `json:"lease_token"`
	ActivationID   string    `json:"activation_id"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
	RenewBeforeAt  time.Time `json:"renew_before_at"`
}

type parsedCommercialLicense struct {
	payload           CommercialLicensePayload
	rawPayload        []byte
	status            string
	validationError   string
	signatureVerified bool
}

type parsedCommercialLicenseLease struct {
	payload           CommercialLicenseLeasePayload
	rawPayload        []byte
	validationError   string
	signatureVerified bool
}

type commercialLicenseGateState struct {
	Status   string
	Required bool
	Features []string
}

func GetCommercialLicenseStatus() (*CommercialLicenseView, error) {
	license, err := getCommercialLicenseWithDB(model.DB)
	if err != nil {
		return nil, err
	}
	return buildCommercialLicenseViewWithDB(model.DB, license)
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
	view, err := buildCommercialLicenseView(license)
	if err != nil {
		return nil, err
	}
	if common.CommercialLicenseOnlineActivationRequired && strings.TrimSpace(common.CommercialLicenseActivationURL) != "" {
		activatedView, activateErr := ActivateCommercialLicense(CommercialLicenseActivateInput{})
		if activateErr == nil {
			return activatedView, nil
		}
		view.LastValidationError = activateErr.Error()
	}
	return view, nil
}

func ClearCommercialLicense() (*CommercialLicenseView, error) {
	if err := model.DeleteCommercialLicense(); err != nil {
		return nil, err
	}
	return buildCommercialLicenseViewWithDB(model.DB, nil)
}

func ActivateCommercialLicense(input CommercialLicenseActivateInput) (*CommercialLicenseView, error) {
	license, err := getCommercialLicenseWithDB(model.DB)
	if err != nil {
		return nil, err
	}
	if license == nil || strings.TrimSpace(license.Token) == "" {
		return nil, errors.New("commercial license is not installed")
	}
	activationURL := strings.TrimSpace(input.ActivationURL)
	if activationURL == "" {
		activationURL = strings.TrimSpace(common.CommercialLicenseActivationURL)
	}
	if activationURL == "" {
		return nil, errors.New("commercial license activation URL is not configured")
	}
	response, err := requestCommercialLicenseLease(activationURL, license)
	if err != nil {
		return nil, err
	}
	if err := applyCommercialLicenseLease(model.DB, license, response, commercialLicenseNow().UTC()); err != nil {
		return nil, err
	}
	return GetCommercialLicenseStatus()
}

func RenewCommercialLicenseLease() (*CommercialLicenseView, error) {
	return ActivateCommercialLicense(CommercialLicenseActivateInput{})
}

func ServeCommercialLicenseActivation(input CommercialLicenseActivationRequest) (*CommercialLicenseActivationResponse, error) {
	if !common.CommercialLicenseActivationServerEnabled {
		return nil, errors.New("commercial license activation server is not enabled")
	}
	privateKey, err := commercialLicenseIssuerPrivateKey()
	if err != nil {
		return nil, err
	}
	if privateKey == nil {
		return nil, errors.New("commercial license issuer private key is not configured")
	}
	licenseToken := strings.TrimSpace(input.LicenseToken)
	if licenseToken == "" {
		return nil, errors.New("license_token is required")
	}
	parsed, err := parseCommercialLicenseToken(licenseToken, commercialLicenseNow())
	if err != nil {
		return nil, err
	}
	if parsed.status != CommercialLicenseStatusValid && parsed.status != CommercialLicenseStatusExpiring {
		if parsed.validationError != "" {
			return nil, errors.New(parsed.validationError)
		}
		return nil, commercialLicenseStatusOperationError(parsed.status)
	}

	machineFingerprint := normalizeCommercialMachineFingerprint(input.MachineFingerprint)
	if machineFingerprint == "" {
		return nil, errors.New("machine_fingerprint is required")
	}
	activationID := strings.TrimSpace(input.ActivationID)
	if activationID == "" {
		activationID = uuid.NewString()
	}
	if strings.TrimSpace(input.LeaseToken) != "" {
		lease, err := parseCommercialLicenseLeaseToken(input.LeaseToken, commercialLicenseNow())
		if err == nil {
			if lease.payload.LicenseID != parsed.payload.LicenseID {
				return nil, errors.New("lease does not match license")
			}
			if lease.payload.MachineFingerprint != machineFingerprint {
				return nil, errors.New("lease does not match machine fingerprint")
			}
			if strings.TrimSpace(lease.payload.ActivationID) != "" {
				activationID = lease.payload.ActivationID
			}
		}
	}

	now := commercialLicenseNow().UTC().Truncate(time.Second)
	expiresAt := now.Add(commercialLicenseLeaseDuration())
	if err := upsertCommercialLicenseActivationRecord(activationID, parsed.payload, input, machineFingerprint, now, expiresAt); err != nil {
		return nil, err
	}
	payload := CommercialLicenseLeasePayload{
		LeaseID:            uuid.NewString(),
		ActivationID:       activationID,
		LicenseID:          parsed.payload.LicenseID,
		CustomerID:         parsed.payload.CustomerID,
		MachineFingerprint: machineFingerprint,
		ServerVersion:      truncateForDatabase(strings.TrimSpace(input.ServerVersion), 64),
		BuildWatermark:     truncateForDatabase(strings.TrimSpace(input.BuildWatermark), 128),
		IssuedAt:           &now,
		ExpiresAt:          &expiresAt,
	}
	token, err := signCommercialLicenseLease(privateKey, payload)
	if err != nil {
		return nil, err
	}
	return &CommercialLicenseActivationResponse{
		LeaseToken:     token,
		ActivationID:   activationID,
		LeaseExpiresAt: expiresAt,
		RenewBeforeAt:  expiresAt.Add(-commercialLicenseLeaseRenewBefore()),
	}, nil
}

func GetCommercialLicenseIssuerStatus() CommercialLicenseIssuerStatus {
	privateKey, err := commercialLicenseIssuerPrivateKey()
	if err != nil {
		return CommercialLicenseIssuerStatus{
			Available: false,
			Message:   err.Error(),
		}
	}
	if privateKey == nil {
		return CommercialLicenseIssuerStatus{
			Available: false,
			Message:   "未配置许可证签发私钥",
		}
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicKeyEncoded := base64.RawURLEncoding.EncodeToString(publicKey)
	return CommercialLicenseIssuerStatus{
		Available:            true,
		PublicKey:            publicKeyEncoded,
		PublicKeyFingerprint: commercialLicenseKeyFingerprint(publicKey),
		Message:              "签发器可用",
	}
}

func IssueCommercialLicense(input CommercialLicenseIssueInput) (*CommercialLicenseIssueResult, error) {
	privateKey, err := commercialLicenseIssuerPrivateKey()
	if err != nil {
		return nil, err
	}
	if privateKey == nil {
		return nil, errors.New("未配置许可证签发私钥")
	}

	payload, err := buildCommercialLicenseIssuePayload(input)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(privateKey, raw)
	token := commercialLicenseTokenPrefix +
		base64.RawURLEncoding.EncodeToString(raw) +
		"." +
		base64.RawURLEncoding.EncodeToString(signature)

	publicKey := privateKey.Public().(ed25519.PublicKey)
	status, validationError := validateCommercialLicensePayload(payload, commercialLicenseNow())
	if status == CommercialLicenseStatusInvalid {
		return nil, errors.New(validationError)
	}
	return &CommercialLicenseIssueResult{
		Token:                token,
		Payload:              payload,
		Status:               status,
		StatusLabel:          commercialLicenseStatusLabel(status),
		PublicKey:            base64.RawURLEncoding.EncodeToString(publicKey),
		PublicKeyFingerprint: commercialLicenseKeyFingerprint(publicKey),
		SignatureVerified:    ed25519.Verify(publicKey, raw, signature),
	}, nil
}

func EnsureCommercialResourceAvailable(resource string) error {
	return ensureCommercialResourceAvailableWithDB(model.DB, resource)
}

func ensureCommercialResourceAvailableWithDB(db *gorm.DB, resource string) error {
	license, err := getCommercialLicenseWithDB(db)
	if err != nil {
		return err
	}
	view, err := buildCommercialLicenseViewWithDB(db, license)
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

func withCommercialResourceCreation(resource string, create func(*gorm.DB) error) error {
	if create == nil {
		return errors.New("commercial resource creation callback is nil")
	}
	commercialLicenseResourceMu.Lock()
	defer commercialLicenseResourceMu.Unlock()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockCommercialLicenseForQuota(tx); err != nil {
			return err
		}
		if err := ensureCommercialResourceAvailableWithDB(tx, resource); err != nil {
			return err
		}
		return create(tx)
	})
}

func EnsureCommercialFeatureEnabled(feature string) error {
	return ensureCommercialFeaturesEnabled(feature)
}

func ensureCommercialFeaturesEnabled(features ...string) error {
	normalized := normalizeCommercialLicenseFeatures(features)
	if len(normalized) == 0 {
		return nil
	}
	state, err := getCommercialLicenseGateState()
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	if state.Status == CommercialLicenseStatusCommunity && !state.Required {
		return nil
	}
	if state.Status != CommercialLicenseStatusValid && state.Status != CommercialLicenseStatusExpiring {
		return commercialLicenseStatusOperationError(state.Status)
	}
	for _, feature := range normalized {
		if !commercialLicenseFeaturesContain(state.Features, feature) {
			return fmt.Errorf("当前授权未包含 %s 能力", commercialLicenseFeatureLabel(feature))
		}
	}
	return nil
}

func getCommercialLicenseGateState() (*commercialLicenseGateState, error) {
	license, err := getCommercialLicenseWithDB(model.DB)
	if err != nil {
		return nil, err
	}
	state := &commercialLicenseGateState{
		Status:   CommercialLicenseStatusCommunity,
		Required: common.CommercialLicenseRequired,
	}
	if license == nil {
		if state.Required {
			state.Status = CommercialLicenseStatusMissing
		}
		return state, nil
	}
	parsed, err := parseCommercialLicenseToken(license.Token, commercialLicenseNow())
	if err != nil {
		state.Status = CommercialLicenseStatusInvalid
		state.Features = decodeCommercialLicenseFeatures(license.FeaturesJSON)
		return state, nil
	}
	state.Status, _ = commercialLicenseStatusWithLease(license, parsed.status, "")
	state.Features = normalizeCommercialLicenseFeatures(parsed.payload.Features)
	return state, nil
}

func lockCommercialLicenseForQuota(tx *gorm.DB) error {
	if model.DatabaseDialectorName(tx) != "postgres" {
		return nil
	}
	var license model.CommercialLicense
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", 1).First(&license).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func getCommercialLicenseWithDB(db *gorm.DB) (*model.CommercialLicense, error) {
	if db == nil {
		db = model.DB
	}
	license := &model.CommercialLicense{}
	err := db.Where("id = ?", 1).First(license).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return license, nil
}

func commercialLicenseStatusOperationError(status string) error {
	switch status {
	case CommercialLicenseStatusMissing:
		return errors.New("当前部署要求安装有效商业许可证")
	case CommercialLicenseStatusExpired:
		return errors.New("商业许可证已过期，请更新授权")
	case CommercialLicenseStatusInvalid:
		return errors.New("商业许可证无效，请重新安装授权")
	case CommercialLicenseStatusActivationRequired:
		return errors.New("commercial license online activation is required")
	case CommercialLicenseStatusLeaseExpired:
		return errors.New("commercial license lease has expired; renew activation")
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
	return buildCommercialLicenseViewWithDB(model.DB, license)
}

func buildCommercialLicenseViewWithDB(db *gorm.DB, license *model.CommercialLicense) (*CommercialLicenseView, error) {
	nodeCount, siteCount, err := commercialLicenseUsageCountsWithDB(db)
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
	status, validationError = commercialLicenseStatusWithLease(license, status, validationError)
	maxSites := normalizeCommercialLicenseLimit(payload.MaxSites)
	view := &CommercialLicenseView{
		Status:                   status,
		StatusLabel:              commercialLicenseStatusLabel(status),
		Licensed:                 status == CommercialLicenseStatusValid || status == CommercialLicenseStatusExpiring,
		Required:                 common.CommercialLicenseRequired,
		LicenseID:                strings.TrimSpace(payload.LicenseID),
		CustomerID:               strings.TrimSpace(payload.CustomerID),
		CustomerName:             strings.TrimSpace(payload.CustomerName),
		Plan:                     normalizeCommercialLicensePlan(payload.Plan),
		Features:                 features,
		MaxNodes:                 maxNodes,
		MaxSites:                 maxSites,
		CurrentNodes:             nodeCount,
		CurrentSites:             siteCount,
		IssuedAt:                 payload.IssuedAt,
		ExpiresAt:                payload.ExpiresAt,
		OnlineActivationRequired: common.CommercialLicenseOnlineActivationRequired,
		ActivationConfigured:     strings.TrimSpace(common.CommercialLicenseActivationURL) != "",
		MachineFingerprint:       currentCommercialMachineFingerprint(),
		BuildWatermark:           strings.TrimSpace(common.CommercialBuildWatermark),
		LastValidationError:      validationError,
		SignatureVerified:        signatureVerified,
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
		view.ActivationID = strings.TrimSpace(license.ActivationID)
		view.MachineFingerprint = valueOrFallback(strings.TrimSpace(license.MachineFingerprint), view.MachineFingerprint)
		view.LeaseExpiresAt = license.LeaseExpiresAt
		view.LastLeaseRenewedAt = license.LastLeaseRenewedAt
	}
	view.LeaseStatus, view.LeaseStatusLabel, view.LeaseSecondsRemaining = commercialLicenseLeaseViewState(license)
	if view.LeaseExpiresAt != nil {
		renewBeforeAt := view.LeaseExpiresAt.UTC().Add(-commercialLicenseLeaseRenewBefore())
		view.LeaseRenewBeforeAt = &renewBeforeAt
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

func buildCommercialLicenseIssuePayload(input CommercialLicenseIssueInput) (CommercialLicensePayload, error) {
	payload := normalizeCommercialLicensePayload(CommercialLicensePayload{
		LicenseID:    input.LicenseID,
		CustomerID:   input.CustomerID,
		CustomerName: input.CustomerName,
		Plan:         input.Plan,
		Features:     input.Features,
		MaxNodes:     input.MaxNodes,
		MaxSites:     input.MaxSites,
	})
	if payload.Plan == "" {
		payload.Plan = "business"
	}
	now := commercialLicenseNow().UTC().Truncate(time.Second)
	issuedAt := now
	if strings.TrimSpace(input.IssuedAt) != "" {
		parsed, err := parseCommercialLicenseIssueTime(input.IssuedAt, true)
		if err != nil {
			return payload, fmt.Errorf("签发时间无效: %w", err)
		}
		if parsed != nil {
			issuedAt = *parsed
		}
	}
	payload.IssuedAt = &issuedAt
	if strings.TrimSpace(input.ExpiresAt) != "" {
		expiresAt, err := parseCommercialLicenseIssueTime(input.ExpiresAt, false)
		if err != nil {
			return payload, fmt.Errorf("到期时间无效: %w", err)
		}
		payload.ExpiresAt = expiresAt
	}
	if payload.ExpiresAt != nil && !payload.ExpiresAt.After(issuedAt) {
		return payload, errors.New("到期时间必须晚于签发时间")
	}
	status, validationError := validateCommercialLicensePayload(payload, now)
	if status == CommercialLicenseStatusInvalid {
		return payload, errors.New(validationError)
	}
	return payload, nil
}

func parseCommercialLicenseIssueTime(raw string, allowNow bool) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if allowNow && strings.EqualFold(value, "now") {
		now := commercialLicenseNow().UTC().Truncate(time.Second)
		return &now, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	return nil, errors.New("需要填写 RFC3339 或 YYYY-MM-DD")
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

func commercialLicenseIssuerPrivateKey() (ed25519.PrivateKey, error) {
	raw := strings.TrimSpace(common.CommercialLicenseIssuerPrivateKey)
	if envRaw := strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY")); envRaw != "" {
		raw = envRaw
	}
	path := strings.TrimSpace(common.CommercialLicenseIssuerPrivateKeyFile)
	if envPath := strings.TrimSpace(os.Getenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY_FILE")); envPath != "" {
		path = envPath
	}
	if raw != "" && path != "" {
		return nil, errors.New("许可证签发私钥不能同时配置 inline 和 file")
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取许可证签发私钥失败: %w", err)
		}
		raw = strings.TrimSpace(string(data))
	}
	if raw == "" {
		return nil, nil
	}
	decoded, err := decodeCommercialLicenseKey(raw, ed25519.PrivateKeySize, "许可证签发私钥")
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(decoded), nil
}

func decodeCommercialLicenseKey(raw string, expectedSize int, label string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "base64url:")
	value = strings.TrimPrefix(value, "base64:")
	value = strings.TrimPrefix(value, "hex:")
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		decoded, err = hex.DecodeString(value)
	}
	if err != nil {
		return nil, fmt.Errorf("%s编码无效", label)
	}
	if len(decoded) != expectedSize {
		return nil, fmt.Errorf("%s长度无效", label)
	}
	return decoded, nil
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
	return commercialLicenseUsageCountsWithDB(model.DB)
}

func commercialLicenseUsageCountsWithDB(db *gorm.DB) (int64, int64, error) {
	if db == nil {
		db = model.DB
	}
	var nodeCount int64
	if err := db.Model(&model.Node{}).Count(&nodeCount).Error; err != nil {
		return 0, 0, err
	}
	var siteCount int64
	if err := db.Model(&model.ProxyRoute{}).Count(&siteCount).Error; err != nil {
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

func commercialLicenseKeyFingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])[:16]
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
	case CommercialLicenseStatusActivationRequired:
		return "待在线激活"
	case CommercialLicenseStatusLeaseExpired:
		return "在线租约过期"
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

func requestCommercialLicenseLease(activationURL string, license *model.CommercialLicense) (*CommercialLicenseActivationResponse, error) {
	endpoint, err := commercialLicenseActivationEndpoint(activationURL)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	payload := CommercialLicenseActivationRequest{
		LicenseToken:       strings.TrimSpace(license.Token),
		LeaseToken:         strings.TrimSpace(license.LeaseToken),
		ActivationID:       strings.TrimSpace(license.ActivationID),
		MachineFingerprint: currentCommercialMachineFingerprint(),
		ServerVersion:      common.Version,
		BuildWatermark:     strings.TrimSpace(common.CommercialBuildWatermark),
		InstanceHostname:   truncateForDatabase(strings.TrimSpace(hostname), 255),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := commercialLicenseHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request commercial license activation failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read commercial license activation response failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("commercial license activation failed with HTTP %d", response.StatusCode)
	}
	var wrapped struct {
		Success bool                                `json:"success"`
		Message string                              `json:"message"`
		Data    CommercialLicenseActivationResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && (wrapped.Success || wrapped.Data.LeaseToken != "") {
		if !wrapped.Success {
			return nil, errors.New(valueOrFallback(wrapped.Message, "commercial license activation failed"))
		}
		return &wrapped.Data, nil
	}
	var direct CommercialLicenseActivationResponse
	if err := json.Unmarshal(body, &direct); err != nil {
		return nil, fmt.Errorf("decode commercial license activation response failed: %w", err)
	}
	if strings.TrimSpace(direct.LeaseToken) == "" {
		return nil, errors.New("commercial license activation response did not include a lease token")
	}
	return &direct, nil
}

func commercialLicenseActivationEndpoint(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", errors.New("commercial license activation URL is not configured")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("commercial license activation URL is invalid")
	}
	if strings.HasSuffix(parsed.Path, "/activation/activate") {
		return parsed.String(), nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/license/activation/activate"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func applyCommercialLicenseLease(db *gorm.DB, license *model.CommercialLicense, response *CommercialLicenseActivationResponse, now time.Time) error {
	if db == nil {
		db = model.DB
	}
	if license == nil || response == nil {
		return errors.New("commercial license lease update is incomplete")
	}
	lease, err := parseCommercialLicenseLeaseToken(response.LeaseToken, now)
	if err != nil {
		return err
	}
	parsedLicense, err := parseCommercialLicenseToken(license.Token, now)
	if err != nil {
		return err
	}
	if lease.payload.LicenseID != parsedLicense.payload.LicenseID {
		return errors.New("commercial license lease does not match installed license")
	}
	machineFingerprint := currentCommercialMachineFingerprint()
	if lease.payload.MachineFingerprint != machineFingerprint {
		return errors.New("commercial license lease does not match this machine")
	}
	updates := map[string]any{
		"activation_id":         valueOrFallback(strings.TrimSpace(response.ActivationID), strings.TrimSpace(lease.payload.ActivationID)),
		"machine_fingerprint":   machineFingerprint,
		"lease_token":           strings.TrimSpace(response.LeaseToken),
		"lease_expires_at":      lease.payload.ExpiresAt,
		"last_lease_renewed_at": &now,
		"last_validated_at":     &now,
		"last_validation_error": "",
	}
	return db.Model(&model.CommercialLicense{}).Where("id = ?", 1).Updates(updates).Error
}

func parseCommercialLicenseLeaseToken(token string, now time.Time) (*parsedCommercialLicenseLease, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("commercial license lease token is empty")
	}
	compact := strings.TrimPrefix(token, commercialLicenseLeaseTokenPrefix)
	parts := strings.Split(compact, ".")
	if len(parts) != 2 {
		return nil, errors.New("commercial license lease token format is invalid")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("commercial license lease payload encoding is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("commercial license lease signature encoding is invalid")
	}
	publicKeys, err := commercialLicensePublicKeys()
	if err != nil {
		return nil, err
	}
	if len(publicKeys) == 0 {
		return nil, errors.New("commercial license public key is not configured")
	}
	verified := false
	for _, publicKey := range publicKeys {
		if ed25519.Verify(publicKey, payloadBytes, signature) {
			verified = true
			break
		}
	}
	if !verified {
		return nil, errors.New("commercial license lease signature is invalid")
	}
	var payload CommercialLicenseLeasePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("commercial license lease payload is invalid")
	}
	payload.MachineFingerprint = normalizeCommercialMachineFingerprint(payload.MachineFingerprint)
	if strings.TrimSpace(payload.LeaseID) == "" || strings.TrimSpace(payload.LicenseID) == "" {
		return nil, errors.New("commercial license lease payload is missing required fields")
	}
	if payload.MachineFingerprint == "" {
		return nil, errors.New("commercial license lease payload is missing machine fingerprint")
	}
	if payload.IssuedAt != nil && payload.IssuedAt.After(now.Add(5*time.Minute)) {
		return nil, errors.New("commercial license lease was issued in the future")
	}
	if payload.ExpiresAt == nil || !payload.ExpiresAt.After(now) {
		return nil, errors.New("commercial license lease has expired")
	}
	return &parsedCommercialLicenseLease{
		payload:           payload,
		rawPayload:        payloadBytes,
		signatureVerified: true,
	}, nil
}

func signCommercialLicenseLease(privateKey ed25519.PrivateKey, payload CommercialLicenseLeasePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(privateKey, raw)
	return commercialLicenseLeaseTokenPrefix +
		base64.RawURLEncoding.EncodeToString(raw) +
		"." +
		base64.RawURLEncoding.EncodeToString(signature), nil
}

func upsertCommercialLicenseActivationRecord(activationID string, payload CommercialLicensePayload, input CommercialLicenseActivationRequest, machineFingerprint string, issuedAt time.Time, expiresAt time.Time) error {
	if model.DB == nil {
		return nil
	}
	var existing model.CommercialLicenseActivation
	err := model.DB.Where("activation_id = ?", activationID).First(&existing).Error
	if err == nil && existing.RevokedAt != nil {
		return errors.New("commercial license activation has been revoked")
	}
	record := model.CommercialLicenseActivation{
		ActivationID:       activationID,
		LicenseID:          strings.TrimSpace(payload.LicenseID),
		CustomerID:         strings.TrimSpace(payload.CustomerID),
		MachineFingerprint: machineFingerprint,
		ServerVersion:      truncateForDatabase(strings.TrimSpace(input.ServerVersion), 64),
		BuildWatermark:     truncateForDatabase(strings.TrimSpace(input.BuildWatermark), 128),
		InstanceHostname:   truncateForDatabase(strings.TrimSpace(input.InstanceHostname), 255),
		LastLeaseIssuedAt:  &issuedAt,
		LastLeaseExpiresAt: &expiresAt,
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DB.Create(&record).Error
	}
	if err != nil {
		return err
	}
	return model.DB.Model(&existing).Updates(map[string]any{
		"license_id":            record.LicenseID,
		"customer_id":           record.CustomerID,
		"machine_fingerprint":   record.MachineFingerprint,
		"server_version":        record.ServerVersion,
		"build_watermark":       record.BuildWatermark,
		"instance_hostname":     record.InstanceHostname,
		"last_lease_issued_at":  record.LastLeaseIssuedAt,
		"last_lease_expires_at": record.LastLeaseExpiresAt,
	}).Error
}

func commercialLicenseStatusWithLease(license *model.CommercialLicense, status string, validationError string) (string, string) {
	if status != CommercialLicenseStatusValid && status != CommercialLicenseStatusExpiring {
		return status, validationError
	}
	if !common.CommercialLicenseOnlineActivationRequired {
		return status, validationError
	}
	if license == nil || strings.TrimSpace(license.LeaseToken) == "" {
		return CommercialLicenseStatusActivationRequired, valueOrFallback(validationError, "commercial license online activation is required")
	}
	if _, err := parseCommercialLicenseLeaseToken(license.LeaseToken, commercialLicenseNow()); err != nil {
		return CommercialLicenseStatusLeaseExpired, err.Error()
	}
	return status, validationError
}

func commercialLicenseLeaseViewState(license *model.CommercialLicense) (string, string, int64) {
	if !common.CommercialLicenseOnlineActivationRequired {
		return "not_required", "不需要在线激活", 0
	}
	if license == nil || strings.TrimSpace(license.LeaseToken) == "" {
		return "missing", "未激活", 0
	}
	lease, err := parseCommercialLicenseLeaseToken(license.LeaseToken, commercialLicenseNow())
	if err != nil {
		return "expired", "租约过期", 0
	}
	if lease.payload.ExpiresAt == nil {
		return "invalid", "租约无效", 0
	}
	remaining := int64(lease.payload.ExpiresAt.Sub(commercialLicenseNow()).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return "valid", "租约有效", remaining
}

func commercialLicenseLeaseDuration() time.Duration {
	if common.CommercialLicenseLeaseDuration > 0 {
		return common.CommercialLicenseLeaseDuration
	}
	return 72 * time.Hour
}

func commercialLicenseLeaseRenewBefore() time.Duration {
	if common.CommercialLicenseLeaseRenewBefore > 0 {
		return common.CommercialLicenseLeaseRenewBefore
	}
	return 6 * time.Hour
}

func currentCommercialMachineFingerprint() string {
	hostname, _ := os.Hostname()
	parts := []string{
		"dushengcdn-machine-v1",
		runtime.GOOS,
		runtime.GOARCH,
		strings.ToLower(strings.TrimSpace(hostname)),
		strings.TrimSpace(common.SQLitePath),
		strings.TrimSpace(common.SQLDSN),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func normalizeCommercialMachineFingerprint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func valueOrFallback(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func StartCommercialLicenseLeaseRenewer(ctx context.Context) {
	if !common.CommercialLicenseOnlineActivationRequired || strings.TrimSpace(common.CommercialLicenseActivationURL) == "" {
		return
	}
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				next := runCommercialLicenseLeaseRenewOnce()
				timer.Reset(next)
			}
		}
	}()
}

func runCommercialLicenseLeaseRenewOnce() time.Duration {
	next := time.Hour
	license, err := getCommercialLicenseWithDB(model.DB)
	if err != nil {
		slog.Warn("load commercial license for lease renewal failed", "error", err)
		return next
	}
	if license == nil || strings.TrimSpace(license.Token) == "" {
		return next
	}
	now := commercialLicenseNow().UTC()
	if license.LeaseExpiresAt != nil {
		renewAt := license.LeaseExpiresAt.UTC().Add(-commercialLicenseLeaseRenewBefore())
		if now.Before(renewAt) {
			wait := renewAt.Sub(now)
			if wait < next {
				next = wait
			}
			return next
		}
	}
	if _, err := RenewCommercialLicenseLease(); err != nil {
		slog.Warn("commercial license lease renewal failed", "error", err)
		return 30 * time.Minute
	}
	slog.Info("commercial license lease renewed")
	return next
}
