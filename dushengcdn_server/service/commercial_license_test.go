package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"
)

func TestCommercialLicenseStatusDefaultsToCommunityMode(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, false, "", false)

	view, err := GetCommercialLicenseStatus()
	if err != nil {
		t.Fatalf("GetCommercialLicenseStatus failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusCommunity || !view.CanCreateNodes || !view.CanCreateSites {
		t.Fatalf("unexpected community license view: %+v", view)
	}
}

func TestCommercialLicenseRequiredBlocksCreateWithoutLicense(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", false)

	if err := EnsureCommercialResourceAvailable("node"); err == nil {
		t.Fatal("expected missing required license to block node creation")
	}
}

func TestCommercialLicenseRequiredBlocksCommercialResourceEntrypoints(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", false)

	if _, err := CreateNode(NodeInput{Name: "blocked-node"}); err == nil {
		t.Fatal("expected required license to block manual node creation")
	}
	if _, err := RegisterNodeWithDiscovery(AgentNodePayload{
		Name:         "blocked-agent-node",
		IP:           "203.0.113.10",
		AgentVersion: "v1.0.0",
	}); err == nil {
		t.Fatal("expected required license to block discovery node registration")
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:    "blocked.example.com",
		OriginURL: "http://origin.example.com",
	}); err == nil {
		t.Fatal("expected required license to block site creation")
	}
}

func TestCommercialLicenseFeatureGateDefaultsToCommunityMode(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, false, "", false)

	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		t.Fatalf("expected community mode to allow feature use, got %v", err)
	}
}

func TestCommercialLicenseFeatureGateRequiresLicensedFeature(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err == nil {
		t.Fatal("expected missing required license to block feature use")
	}

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-feature-gate",
		CustomerName: "Feature Gate Ltd.",
		Plan:         "business",
		Features:     []string{"authoritative_dns", "ACME"},
	})
	view, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token})
	if err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	if strings.Join(view.Features, ",") != "authoritative-dns,acme-automation" {
		t.Fatalf("expected normalized feature aliases, got %+v", view.Features)
	}
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureAuthoritativeDNS); err != nil {
		t.Fatalf("expected licensed feature to be allowed, got %v", err)
	}
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureACMEAutomation); err != nil {
		t.Fatalf("expected ACME alias to be allowed, got %v", err)
	}
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureGSLB); err == nil || !strings.Contains(err.Error(), "GSLB") {
		t.Fatalf("expected missing feature to be rejected with label, got %v", err)
	}
}

func TestCommercialLicenseAllFeatureAllowsCommercialFeature(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-all-features",
		CustomerName: "All Features Ltd.",
		Plan:         "enterprise",
		Features:     []string{"all"},
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	if err := EnsureCommercialFeatureEnabled(CommercialFeatureDDoSProtection); err != nil {
		t.Fatalf("expected all feature license to allow DDoS feature, got %v", err)
	}
}

func TestCommercialLicenseRequiredBlocksAdvancedFeatureEntrypoints(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-basic",
		CustomerName: "Basic CDN Ltd.",
		Plan:         "business",
		MaxSites:     10,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	if _, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"}); err == nil || !strings.Contains(err.Error(), "权威 DNS") {
		t.Fatalf("expected authoritative DNS zone creation to require feature, got %v", err)
	}
	if _, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"}); err == nil || !strings.Contains(err.Error(), "权威 DNS") {
		t.Fatalf("expected authoritative DNS worker creation to require feature, got %v", err)
	}
	if _, err := ApplyTLSCertificate(TLSApplyInput{
		Name:          "blocked-acme",
		PrimaryDomain: "example.com",
		KeyAlgorithm:  "RSA2048",
		SkipDNS:       true,
	}); err == nil || !strings.Contains(err.Error(), "ACME") {
		t.Fatalf("expected ACME apply to require feature, got %v", err)
	}

	dnsAccount := &model.DnsAccount{
		Name:          "Cloudflare",
		Type:          "cloudflare",
		Authorization: `{"api_token":"token"}`,
	}
	if err := dnsAccount.Insert(); err != nil {
		t.Fatalf("insert dns account: %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:       "dns-auto.example.com",
		OriginURL:    "http://origin.example.com",
		DNSAutoSync:  true,
		DNSAccountID: &dnsAccount.ID,
		DNSZoneID:    "zone-id",
	}); err == nil || !strings.Contains(err.Error(), "Cloudflare DNS") {
		t.Fatalf("expected Cloudflare DNS auto sync to require feature, got %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "waf.example.com",
		OriginURL:  "http://origin.example.com",
		WAFEnabled: true,
	}); err == nil || !strings.Contains(err.Error(), "WAF") {
		t.Fatalf("expected WAF to require feature, got %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:     "cc.example.com",
		OriginURL:  "http://origin.example.com",
		CCEnabled:  true,
		PoWEnabled: true,
	}); err == nil || !strings.Contains(err.Error(), "CC") {
		t.Fatalf("expected CC/PoW protection to require feature, got %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:                     "region.example.com",
		OriginURL:                  "http://origin.example.com",
		RegionRestrictionEnabled:   true,
		RegionRestrictionCountries: []string{"US"},
	}); err == nil || !strings.Contains(err.Error(), "区域访问控制") {
		t.Fatalf("expected region restriction to require feature, got %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:                 "ddos.example.com",
		OriginURL:              "http://origin.example.com",
		DDOSProtectionMode:     DDOSProtectionModeAuto,
		DDOSProtectionProvider: DDOSProtectionProviderCloudflare,
	}); err == nil || !strings.Contains(err.Error(), "DDoS") {
		t.Fatalf("expected DDoS protection to require feature, got %v", err)
	}

	authOnlyToken := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-auth-only",
		CustomerName: "Auth DNS Ltd.",
		Plan:         "business",
		Features:     []string{CommercialFeatureAuthoritativeDNS},
		MaxSites:     10,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: authOnlyToken}); err != nil {
		t.Fatalf("InstallCommercialLicense auth-only failed: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "gslb.example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone for GSLB gate: %v", err)
	}
	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:          "app.gslb.example.com",
		OriginURL:       "http://origin.example.com",
		Enabled:         false,
		DNSProviderMode: DNSProviderModeAuthoritative,
		DNSZoneIDRef:    &zone.ID,
		GSLBEnabled:     true,
	}); err == nil || !strings.Contains(err.Error(), "GSLB") {
		t.Fatalf("expected GSLB to require feature, got %v", err)
	}
}

func TestCommercialLicenseFeaturesAllowAdvancedFeatureEntrypoints(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-advanced",
		CustomerName: "Advanced CDN Ltd.",
		Plan:         "enterprise",
		Features: []string{
			CommercialFeatureAuthoritativeDNS,
			CommercialFeatureCloudflareDNS,
			CommercialFeatureACMEAutomation,
			CommercialFeatureGSLB,
			CommercialFeatureWAF,
			CommercialFeatureCCProtection,
			CommercialFeatureGeoAccessControl,
		},
		MaxSites: 10,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	restore := SetTLSCertificateObtainFuncForTest(func(c *model.TLSCertificate) error {
		return nil
	})
	t.Cleanup(restore)

	dnsAccount := &model.DnsAccount{
		Name:          "Cloudflare",
		Type:          "cloudflare",
		Authorization: `{"api_token":"token"}`,
	}
	if err := dnsAccount.Insert(); err != nil {
		t.Fatalf("insert dns account: %v", err)
	}
	zone, err := CreateAuthoritativeDNSZone(DNSZoneInput{Name: "example.com"})
	if err != nil {
		t.Fatalf("CreateAuthoritativeDNSZone: %v", err)
	}
	if _, err := CreateAuthoritativeDNSRecord(zone.ID, DNSRecordInput{Name: "www", Type: "A", Value: "203.0.113.10"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSRecord: %v", err)
	}
	if _, err := CreateAuthoritativeDNSWorker(DNSWorkerInput{Name: "ns1"}); err != nil {
		t.Fatalf("CreateAuthoritativeDNSWorker: %v", err)
	}
	if _, err := ApplyTLSCertificate(TLSApplyInput{
		Name:          "allowed-acme",
		PrimaryDomain: "example.com",
		DnsAccountID:  dnsAccount.ID,
		KeyAlgorithm:  "RSA2048",
		SkipDNS:       true,
	}); err != nil {
		t.Fatalf("ApplyTLSCertificate: %v", err)
	}

	if _, err := CreateProxyRoute(ProxyRouteInput{
		Domain:                     "advanced.example.com",
		OriginURL:                  "http://origin.example.com",
		Enabled:                    false,
		DNSProviderMode:            DNSProviderModeAuthoritative,
		DNSZoneIDRef:               &zone.ID,
		GSLBEnabled:                true,
		WAFEnabled:                 true,
		CCEnabled:                  true,
		RegionRestrictionEnabled:   true,
		RegionRestrictionCountries: []string{"US"},
	}); err != nil {
		t.Fatalf("CreateProxyRoute with advanced features: %v", err)
	}
}

func TestInstallCommercialLicenseVerifiesSignatureAndAppliesLimits(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	withCommercialLicenseTestConfig(t, true, base64.RawURLEncoding.EncodeToString(publicKey), false)

	now := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	expiresAt := now.Add(30 * 24 * time.Hour)
	token := buildSignedCommercialLicenseToken(t, privateKey, CommercialLicensePayload{
		LicenseID:    "lic-enterprise-001",
		CustomerName: "Example CDN Ltd.",
		Plan:         "enterprise",
		Features:     []string{"metering", "priority-support", "metering"},
		MaxNodes:     1,
		MaxSites:     1,
		IssuedAt:     &now,
		ExpiresAt:    &expiresAt,
	})

	view, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token})
	if err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusValid || !view.SignatureVerified {
		t.Fatalf("expected valid signed license, got %+v", view)
	}
	if view.LicenseID != "lic-enterprise-001" || view.Plan != "enterprise" || view.Fingerprint == "" {
		t.Fatalf("unexpected installed license view: %+v", view)
	}
	if strings.Join(view.Features, ",") != "metering,priority-support" {
		t.Fatalf("expected normalized features, got %+v", view.Features)
	}

	if err := model.DB.Create(&model.Node{NodeID: "node-a", Name: "edge-a"}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := model.DB.Create(&model.ProxyRoute{Domain: "a.example.com", OriginURL: "http://origin"}).Error; err != nil {
		t.Fatalf("seed proxy route: %v", err)
	}

	if err := EnsureCommercialResourceAvailable("node"); err == nil {
		t.Fatal("expected node quota to block creation")
	}
	if err := EnsureCommercialResourceAvailable("site"); err == nil {
		t.Fatal("expected site quota to block creation")
	}
}

func TestIssueCommercialLicenseSignsInstallableToken(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	privateKeyRaw := base64.RawURLEncoding.EncodeToString(privateKey)
	publicKeyRaw := base64.RawURLEncoding.EncodeToString(publicKey)
	withCommercialLicenseTestConfig(t, true, publicKeyRaw, false)
	withCommercialLicenseIssuerTestConfig(t, privateKeyRaw, "")

	now := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	result, err := IssueCommercialLicense(CommercialLicenseIssueInput{
		LicenseID:    "lic-panel-001",
		CustomerID:   "cust-001",
		CustomerName: "Panel Customer",
		Plan:         "enterprise",
		Features:     []string{"all"},
		MaxNodes:     10,
		MaxSites:     50,
		ExpiresAt:    "2027-06-04",
	})
	if err != nil {
		t.Fatalf("IssueCommercialLicense failed: %v", err)
	}
	if !strings.HasPrefix(result.Token, commercialLicenseTokenPrefix) {
		t.Fatalf("expected signed token, got %q", result.Token)
	}
	if !result.SignatureVerified || result.PublicKey != publicKeyRaw || result.PublicKeyFingerprint == "" {
		t.Fatalf("unexpected issue result: %+v", result)
	}

	view, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: result.Token})
	if err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusValid || !view.SignatureVerified {
		t.Fatalf("expected valid installed license, got %+v", view)
	}
	if view.LicenseID != "lic-panel-001" || view.CustomerID != "cust-001" || view.MaxNodes != 10 || view.MaxSites != 50 {
		t.Fatalf("unexpected installed license view: %+v", view)
	}
}

func TestIssueCommercialLicenseRequiresConfiguredIssuerKey(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", false)
	withCommercialLicenseIssuerTestConfig(t, "", "")

	if _, err := IssueCommercialLicense(CommercialLicenseIssueInput{
		LicenseID:    "lic-missing-key",
		CustomerName: "Panel Customer",
		Plan:         "business",
	}); err == nil {
		t.Fatal("expected missing issuer key error")
	}

	status := GetCommercialLicenseIssuerStatus()
	if status.Available {
		t.Fatalf("expected unavailable issuer status, got %+v", status)
	}
}

func TestIssueCommercialLicenseRejectsExpiredPayload(t *testing.T) {
	setupServiceTestDB(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	withCommercialLicenseTestConfig(t, true, "", false)
	withCommercialLicenseIssuerTestConfig(t, base64.RawURLEncoding.EncodeToString(privateKey), "")

	now := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	if _, err := IssueCommercialLicense(CommercialLicenseIssueInput{
		LicenseID:    "lic-expired",
		CustomerName: "Panel Customer",
		Plan:         "business",
		ExpiresAt:    "2026-01-01",
	}); err == nil {
		t.Fatal("expected expired payload error")
	}
}

func TestCommercialLicenseOnlineActivationIssuesAndStores72HourLease(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	publicKeyRaw := base64.RawURLEncoding.EncodeToString(publicKey)
	privateKeyRaw := base64.RawURLEncoding.EncodeToString(privateKey)
	withCommercialLicenseTestConfig(t, true, publicKeyRaw, false)
	withCommercialLicenseIssuerTestConfig(t, privateKeyRaw, "")
	withCommercialLicenseOnlineActivationTestConfig(t, true, "", 72*time.Hour, 6*time.Hour)

	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	token := buildSignedCommercialLicenseToken(t, privateKey, CommercialLicensePayload{
		LicenseID:    "lic-online",
		CustomerID:   "cust-online",
		CustomerName: "Online Customer",
		Plan:         "enterprise",
		Features:     []string{"all"},
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}
	view, err := GetCommercialLicenseStatus()
	if err != nil {
		t.Fatalf("GetCommercialLicenseStatus failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusActivationRequired || view.Licensed {
		t.Fatalf("expected activation required before lease, got %+v", view)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input CommercialLicenseActivationRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode activation request: %v", err)
		}
		result, err := ServeCommercialLicenseActivation(input)
		if err != nil {
			t.Fatalf("ServeCommercialLicenseActivation failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    result,
		})
	}))
	defer server.Close()
	common.CommercialLicenseActivationURL = server.URL

	view, err = ActivateCommercialLicense(CommercialLicenseActivateInput{})
	if err != nil {
		t.Fatalf("ActivateCommercialLicense failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusValid || !view.Licensed {
		t.Fatalf("expected valid activated license, got %+v", view)
	}
	if view.LeaseExpiresAt == nil || !view.LeaseExpiresAt.Equal(now.Add(72*time.Hour)) {
		t.Fatalf("unexpected lease expiry: %+v", view.LeaseExpiresAt)
	}
	if view.LeaseRenewBeforeAt == nil || !view.LeaseRenewBeforeAt.Equal(now.Add(66*time.Hour)) {
		t.Fatalf("unexpected renew-before time: %+v", view.LeaseRenewBeforeAt)
	}
}

func TestCommercialLicenseRevocationBlocksLeaseRenewalByLicenseID(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	withCommercialLicenseTestConfig(t, true, base64.RawURLEncoding.EncodeToString(publicKey), false)
	withCommercialLicenseIssuerTestConfig(t, base64.RawURLEncoding.EncodeToString(privateKey), "")
	withCommercialLicenseOnlineActivationTestConfig(t, true, "", 72*time.Hour, 6*time.Hour)

	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	token := buildSignedCommercialLicenseToken(t, privateKey, CommercialLicensePayload{
		LicenseID:    "lic-revoke",
		CustomerID:   "cust-revoke",
		CustomerName: "Revoke Customer",
		Plan:         "enterprise",
		Features:     []string{"all"},
	})
	initial, err := ServeCommercialLicenseActivation(CommercialLicenseActivationRequest{
		LicenseToken:       token,
		MachineFingerprint: "machine-a",
		ServerVersion:      "v1",
	})
	if err != nil {
		t.Fatalf("initial activation failed: %v", err)
	}
	if initial.LeaseToken == "" || initial.ActivationID == "" {
		t.Fatalf("expected initial lease and activation id, got %+v", initial)
	}
	activations, err := ListCommercialLicenseActivations()
	if err != nil {
		t.Fatalf("ListCommercialLicenseActivations failed: %v", err)
	}
	if len(activations) != 1 || activations[0].LicenseID != "lic-revoke" || activations[0].LeaseStatus != "active" {
		t.Fatalf("unexpected activation list: %+v", activations)
	}

	revoked, err := RevokeCommercialLicense(CommercialLicenseRevocationInput{
		LicenseID:  "lic-revoke",
		CustomerID: "cust-revoke",
		Reason:     "unpaid",
	})
	if err != nil {
		t.Fatalf("RevokeCommercialLicense failed: %v", err)
	}
	if len(revoked) != 1 || revoked[0].LicenseRevokedAt == nil || revoked[0].LeaseStatus != "license_revoked" {
		t.Fatalf("expected revoked activation view, got %+v", revoked)
	}

	_, err = ServeCommercialLicenseActivation(CommercialLicenseActivationRequest{
		LicenseToken:       token,
		LeaseToken:         initial.LeaseToken,
		ActivationID:       initial.ActivationID,
		MachineFingerprint: "machine-a",
		ServerVersion:      "v2",
	})
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("expected revoked license renewal error, got %v", err)
	}

	if _, err = RestoreCommercialLicense(CommercialLicenseRevocationInput{LicenseID: "lic-revoke"}); err != nil {
		t.Fatalf("RestoreCommercialLicense failed: %v", err)
	}
	renewed, err := ServeCommercialLicenseActivation(CommercialLicenseActivationRequest{
		LicenseToken:       token,
		LeaseToken:         initial.LeaseToken,
		ActivationID:       initial.ActivationID,
		MachineFingerprint: "machine-a",
		ServerVersion:      "v3",
	})
	if err != nil {
		t.Fatalf("expected restored license to renew: %v", err)
	}
	if renewed.ActivationID != initial.ActivationID {
		t.Fatalf("expected restored renewal to keep activation id, got %s want %s", renewed.ActivationID, initial.ActivationID)
	}
}

func TestCommercialLicenseActivationEndpointNormalizesSatanduDefault(t *testing.T) {
	cases := map[string]string{
		"https://www.satandu.com":                                 "https://www.satandu.com/api/license/activation/activate",
		"https://www.satandu.com/":                                "https://www.satandu.com/api/license/activation/activate",
		"https://www.satandu.com/api/license/activation":          "https://www.satandu.com/api/license/activation/activate",
		"https://www.satandu.com/api/license/activation/activate": "https://www.satandu.com/api/license/activation/activate",
	}
	for input, expected := range cases {
		endpoint, err := commercialLicenseActivationEndpoint(input)
		if err != nil {
			t.Fatalf("commercialLicenseActivationEndpoint(%q) failed: %v", input, err)
		}
		if endpoint != expected {
			t.Fatalf("commercialLicenseActivationEndpoint(%q) = %q, want %q", input, endpoint, expected)
		}
	}
}

func TestCommercialLicenseLeaseExpiresAndRenewerRefreshesBeforeDeadline(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	withCommercialLicenseTestConfig(t, true, base64.RawURLEncoding.EncodeToString(publicKey), false)
	withCommercialLicenseIssuerTestConfig(t, base64.RawURLEncoding.EncodeToString(privateKey), "")
	withCommercialLicenseOnlineActivationTestConfig(t, true, "", 72*time.Hour, 6*time.Hour)

	now := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	restoreNow := setCommercialLicenseNowForTest(t, now)
	defer restoreNow()

	token := buildSignedCommercialLicenseToken(t, privateKey, CommercialLicensePayload{
		LicenseID:    "lic-renew",
		CustomerName: "Renew Customer",
		Plan:         "enterprise",
		Features:     []string{"all"},
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input CommercialLicenseActivationRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode activation request: %v", err)
		}
		result, err := ServeCommercialLicenseActivation(input)
		if err != nil {
			t.Fatalf("ServeCommercialLicenseActivation failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": result})
	}))
	defer server.Close()
	common.CommercialLicenseActivationURL = server.URL

	if _, err := ActivateCommercialLicense(CommercialLicenseActivateInput{}); err != nil {
		t.Fatalf("initial activation failed: %v", err)
	}

	renewNow := now.Add(66*time.Hour + time.Minute)
	restoreNow()
	restoreNow = setCommercialLicenseNowForTest(t, renewNow)
	defer restoreNow()
	next := runCommercialLicenseLeaseRenewOnce()
	if next != time.Hour {
		t.Fatalf("expected default next interval after renewal, got %v", next)
	}
	view, err := GetCommercialLicenseStatus()
	if err != nil {
		t.Fatalf("GetCommercialLicenseStatus failed: %v", err)
	}
	if view.LeaseExpiresAt == nil || !view.LeaseExpiresAt.Equal(renewNow.Add(72*time.Hour)) {
		t.Fatalf("expected renewed lease expiry, got %+v", view.LeaseExpiresAt)
	}

	expiredNow := renewNow.Add(73 * time.Hour)
	restoreNow()
	restoreNow = setCommercialLicenseNowForTest(t, expiredNow)
	view, err = GetCommercialLicenseStatus()
	if err != nil {
		t.Fatalf("GetCommercialLicenseStatus after expiry failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusLeaseExpired {
		t.Fatalf("expected lease expired status, got %+v", view)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartCommercialLicenseLeaseRenewer(ctx)
}

func TestCommercialLicenseNodeQuotaSerializesConcurrentCreates(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-node-quota",
		CustomerName: "Quota Ltd.",
		Plan:         "business",
		MaxNodes:     1,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"edge-a", "edge-b"} {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			_, err := CreateNode(NodeInput{Name: nodeName, IP: "203.0.113.10"})
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), "节点") {
			t.Fatalf("expected quota error mentioning node limit, got %v", err)
		}
		failures++
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one successful create and one quota failure, got successes=%d failures=%d", successes, failures)
	}

	var count int64
	if err := model.DB.Model(&model.Node{}).Count(&count).Error; err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one node after concurrent creates, got %d", count)
	}
}

func TestCommercialLicenseSiteQuotaSerializesConcurrentCreates(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	token := buildUnsignedCommercialLicenseToken(t, CommercialLicensePayload{
		LicenseID:    "lic-site-quota",
		CustomerName: "Quota Ltd.",
		Plan:         "business",
		MaxSites:     1,
	})
	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err != nil {
		t.Fatalf("InstallCommercialLicense failed: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, domain := range []string{"a.example.com", "b.example.com"} {
		wg.Add(1)
		go func(routeDomain string) {
			defer wg.Done()
			_, err := CreateProxyRoute(ProxyRouteInput{
				Domain:    routeDomain,
				OriginURL: "http://origin.example.com",
			})
			errs <- err
		}(domain)
	}
	wg.Wait()
	close(errs)

	successes := 0
	failures := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), "站点") {
			t.Fatalf("expected quota error mentioning site limit, got %v", err)
		}
		failures++
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one successful create and one quota failure, got successes=%d failures=%d", successes, failures)
	}

	var count int64
	if err := model.DB.Model(&model.ProxyRoute{}).Count(&count).Error; err != nil {
		t.Fatalf("count proxy routes: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one proxy route after concurrent creates, got %d", count)
	}
}

func TestInstallCommercialLicenseRejectsInvalidSignature(t *testing.T) {
	setupServiceTestDB(t)
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey public key failed: %v", err)
	}
	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey signing key failed: %v", err)
	}
	withCommercialLicenseTestConfig(t, true, base64.RawURLEncoding.EncodeToString(publicKey), false)

	token := buildSignedCommercialLicenseToken(t, otherPrivateKey, CommercialLicensePayload{
		LicenseID:    "lic-invalid",
		CustomerName: "Example CDN Ltd.",
		Plan:         "enterprise",
	})

	if _, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: token}); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
}

func TestInstallCommercialLicenseAllowsUnsignedOnlyWhenConfigured(t *testing.T) {
	setupServiceTestDB(t)
	withCommercialLicenseTestConfig(t, true, "", true)

	payload := CommercialLicensePayload{
		LicenseID:    "lic-dev",
		CustomerName: "Development",
		Plan:         "business",
		MaxNodes:     2,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal unsigned payload: %v", err)
	}
	view, err := InstallCommercialLicense(CommercialLicenseInstallInput{Token: string(raw)})
	if err != nil {
		t.Fatalf("InstallCommercialLicense unsigned failed: %v", err)
	}
	if view.Status != CommercialLicenseStatusValid {
		t.Fatalf("expected unsigned dev license to be accepted, got %+v", view)
	}
	if view.SignatureVerified {
		t.Fatal("unsigned dev license must not be reported as signature verified")
	}
}

func buildUnsignedCommercialLicenseToken(t *testing.T, payload CommercialLicensePayload) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal unsigned license payload: %v", err)
	}
	return string(raw)
}

func buildSignedCommercialLicenseToken(t *testing.T, privateKey ed25519.PrivateKey, payload CommercialLicensePayload) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal license payload: %v", err)
	}
	signature := ed25519.Sign(privateKey, raw)
	return commercialLicenseTokenPrefix +
		base64.RawURLEncoding.EncodeToString(raw) +
		"." +
		base64.RawURLEncoding.EncodeToString(signature)
}

func withCommercialLicenseTestConfig(t *testing.T, required bool, publicKeys string, allowUnsigned bool) {
	t.Helper()
	previousRequired := common.CommercialLicenseRequired
	previousPublicKeys := common.CommercialLicensePublicKeys
	previousAllowUnsigned := common.CommercialLicenseAllowUnsigned
	t.Setenv("DUSHENGCDN_LICENSE_PUBLIC_KEYS", "")
	t.Setenv("DUSHENGCDN_LICENSE_ALLOW_UNSIGNED", "")
	common.CommercialLicenseRequired = required
	common.CommercialLicensePublicKeys = publicKeys
	common.CommercialLicenseAllowUnsigned = allowUnsigned
	t.Cleanup(func() {
		common.CommercialLicenseRequired = previousRequired
		common.CommercialLicensePublicKeys = previousPublicKeys
		common.CommercialLicenseAllowUnsigned = previousAllowUnsigned
	})
}

func withCommercialLicenseIssuerTestConfig(t *testing.T, privateKey string, privateKeyFile string) {
	t.Helper()
	previousPrivateKey := common.CommercialLicenseIssuerPrivateKey
	previousPrivateKeyFile := common.CommercialLicenseIssuerPrivateKeyFile
	t.Setenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY", "")
	t.Setenv("DUSHENGCDN_LICENSE_ISSUER_PRIVATE_KEY_FILE", "")
	common.CommercialLicenseIssuerPrivateKey = privateKey
	common.CommercialLicenseIssuerPrivateKeyFile = privateKeyFile
	t.Cleanup(func() {
		common.CommercialLicenseIssuerPrivateKey = previousPrivateKey
		common.CommercialLicenseIssuerPrivateKeyFile = previousPrivateKeyFile
	})
}

func withCommercialLicenseOnlineActivationTestConfig(t *testing.T, required bool, activationURL string, leaseDuration time.Duration, renewBefore time.Duration) {
	t.Helper()
	previousRequired := common.CommercialLicenseOnlineActivationRequired
	previousActivationURL := common.CommercialLicenseActivationURL
	previousActivationServerEnabled := common.CommercialLicenseActivationServerEnabled
	previousLeaseDuration := common.CommercialLicenseLeaseDuration
	previousRenewBefore := common.CommercialLicenseLeaseRenewBefore
	common.CommercialLicenseOnlineActivationRequired = required
	common.CommercialLicenseActivationURL = activationURL
	common.CommercialLicenseActivationServerEnabled = true
	common.CommercialLicenseLeaseDuration = leaseDuration
	common.CommercialLicenseLeaseRenewBefore = renewBefore
	t.Cleanup(func() {
		common.CommercialLicenseOnlineActivationRequired = previousRequired
		common.CommercialLicenseActivationURL = previousActivationURL
		common.CommercialLicenseActivationServerEnabled = previousActivationServerEnabled
		common.CommercialLicenseLeaseDuration = previousLeaseDuration
		common.CommercialLicenseLeaseRenewBefore = previousRenewBefore
	})
}

func setCommercialLicenseNowForTest(t *testing.T, now time.Time) func() {
	t.Helper()
	previous := commercialLicenseNow
	commercialLicenseNow = func() time.Time {
		return now
	}
	return func() {
		commercialLicenseNow = previous
	}
}
