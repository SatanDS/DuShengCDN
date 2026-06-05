package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
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
