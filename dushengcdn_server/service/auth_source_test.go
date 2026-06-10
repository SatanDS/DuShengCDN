package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"dushengcdn/common"
	"dushengcdn/model"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestCompleteOAuthLoginRequiresLinkWhenRegistrationDisabled(t *testing.T) {
	setupServiceTestDB(t)

	source := createTestAuthSource(t)
	result, pending, err := CompleteOAuthLogin(source, &OAuthProfile{
		ExternalID:       "external-1",
		ExternalUsername: "external-user",
		DisplayName:      "External User",
		Email:            "external@example.com",
	}, nil)
	if err != nil {
		t.Fatalf("CompleteOAuthLogin failed: %v", err)
	}
	if result.Status != "link_required" || pending == nil {
		t.Fatalf("expected link_required with pending account, got %#v pending=%#v", result, pending)
	}

	user, err := LinkPendingExternalAccount(pending, LinkExistingRequest{
		Username: "root",
		Password: "123456",
	})
	if err != nil {
		t.Fatalf("LinkPendingExternalAccount failed: %v", err)
	}
	if user.Username != "root" {
		t.Fatalf("expected root user, got %s", user.Username)
	}
	account, err := model.FindExternalAccount(source.ID, "external-1")
	if err != nil {
		t.Fatalf("expected external account to be linked: %v", err)
	}
	if account.UserID != user.Id {
		t.Fatalf("expected external account user %d, got %d", user.Id, account.UserID)
	}
}

func TestLinkExternalAccountRejectsDifferentUser(t *testing.T) {
	setupServiceTestDB(t)

	source := createTestAuthSource(t)
	secondUser := &model.User{
		Username:    "second",
		Password:    "password123",
		DisplayName: "Second User",
		Role:        1,
		Status:      1,
	}
	if err := secondUser.Insert(); err != nil {
		t.Fatalf("failed to create second user: %v", err)
	}

	if err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     source.ID,
		UserID:           1,
		ExternalID:       "external-shared",
		ExternalUsername: "shared",
		Email:            "shared@example.com",
	}); err != nil {
		t.Fatalf("initial LinkExternalAccount failed: %v", err)
	}

	err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     source.ID,
		UserID:           secondUser.Id,
		ExternalID:       "external-shared",
		ExternalUsername: "shared",
		Email:            "shared@example.com",
	})
	if err == nil {
		t.Fatal("expected linking the same external account to another user to fail")
	}

	account, findErr := model.FindExternalAccount(source.ID, "external-shared")
	if findErr != nil {
		t.Fatalf("expected external account to remain linked: %v", findErr)
	}
	if account.UserID != 1 {
		t.Fatalf("expected external account to stay linked to user 1, got %d", account.UserID)
	}
}

func TestCompleteOAuthLoginRegistersWithRandomUsername(t *testing.T) {
	setupServiceTestDB(t)

	oldRegisterEnabled := common.RegisterEnabled
	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
	})
	common.RegisterEnabled = true

	source := createTestAuthSource(t)
	result, pending, err := CompleteOAuthLogin(source, &OAuthProfile{
		ExternalID:       "external-register",
		ExternalUsername: "external-register-user",
		DisplayName:      "External Register User With A Very Long Name",
		Email:            "external-register@example.com",
	}, nil)
	if err != nil {
		t.Fatalf("CompleteOAuthLogin failed: %v", err)
	}
	if pending != nil {
		t.Fatalf("expected no pending link when registration is enabled, got %#v", pending)
	}
	if result.Status != "registered" || result.User == nil {
		t.Fatalf("expected registered result with user, got %#v", result)
	}
	if !strings.HasPrefix(result.User.Username, "oidc_") || len(result.User.Username) > 12 {
		t.Fatalf("expected random oidc username within model limit, got %q", result.User.Username)
	}
	if result.User.Username == "oidc_2" {
		t.Fatal("expected OAuth registration to avoid predictable max-id username")
	}

	account, findErr := model.FindExternalAccount(source.ID, "external-register")
	if findErr != nil {
		t.Fatalf("expected external account to be linked: %v", findErr)
	}
	if account.UserID != result.User.Id {
		t.Fatalf("expected external account user %d, got %d", result.User.Id, account.UserID)
	}
}

func TestCompleteOAuthLoginFailsWhenLinkedUserIsMissing(t *testing.T) {
	setupServiceTestDB(t)

	source := createTestAuthSource(t)
	if err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     source.ID,
		UserID:           999,
		ExternalID:       "external-orphan",
		ExternalUsername: "external-orphan-user",
		Email:            "external-orphan@example.com",
	}); err != nil {
		t.Fatalf("LinkExternalAccount failed: %v", err)
	}

	result, pending, err := CompleteOAuthLogin(source, &OAuthProfile{
		ExternalID:       "external-orphan",
		ExternalUsername: "external-orphan-user",
		DisplayName:      "External Orphan",
		Email:            "external-orphan@example.com",
	}, nil)
	if err == nil {
		t.Fatalf("expected linked missing user to fail, got result=%#v pending=%#v", result, pending)
	}
}

func TestExchangeOIDCProfileVerifiesJWKSNonceAudienceAndIssuer(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	nonce := "nonce-123"
	issuer := "https://idp.example.test"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"userinfo_endpoint":      issuer + "/userinfo",
				"jwks_uri":               issuer + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key:       publicKey,
				KeyID:     "test-key",
				Algorithm: string(jose.EdDSA),
				Use:       "sig",
			}}})
		case "/token":
			rawIDToken := signedOIDCIDTokenForTest(t, privateKey, issuer, "client-id", "oidc-subject", nonce)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"id_token":     rawIDToken,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":                "oidc-subject",
				"preferred_username": "alice",
				"name":               "Alice",
				"email":              "alice@example.com",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer SetOAuthHTTPClientForTest(oauthClientForTLSServer(t, server))()

	source := &model.AuthSource{
		Name:               "oidc-test",
		Type:               model.AuthSourceTypeOIDC,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		OpenIDDiscoveryURL: issuer + "/.well-known/openid-configuration",
		Scopes:             "openid profile email",
	}
	profile, err := ExchangeOAuthProfileWithNonce(context.Background(), source, "code", "https://app.example.com/oauth/oidc-test", nonce)
	if err != nil {
		t.Fatalf("ExchangeOAuthProfileWithNonce failed: %v", err)
	}
	if profile.ExternalID != "oidc-subject" || profile.ExternalUsername != "alice" || profile.Email != "alice@example.com" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestExchangeOIDCProfileRejectsMismatchedUserInfoSubject(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	nonce := "nonce-123"
	issuer := "https://idp.example.test"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"userinfo_endpoint":      issuer + "/userinfo",
				"jwks_uri":               issuer + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
				Key:       publicKey,
				KeyID:     "test-key",
				Algorithm: string(jose.EdDSA),
				Use:       "sig",
			}}})
		case "/token":
			rawIDToken := signedOIDCIDTokenForTest(t, privateKey, issuer, "client-id", "oidc-subject", nonce)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"id_token":     rawIDToken,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":                "other-subject",
				"preferred_username": "alice",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer SetOAuthHTTPClientForTest(oauthClientForTLSServer(t, server))()

	source := &model.AuthSource{
		Name:               "oidc-test",
		Type:               model.AuthSourceTypeOIDC,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		OpenIDDiscoveryURL: issuer + "/.well-known/openid-configuration",
		Scopes:             "openid profile email",
	}
	_, err = ExchangeOAuthProfileWithNonce(context.Background(), source, "code", "https://app.example.com/oauth/oidc-test", nonce)
	if err == nil || !strings.Contains(err.Error(), "subject does not match") {
		t.Fatalf("expected mismatched subject rejection, got %v", err)
	}
}

func TestExchangeOIDCProfileRejectsUnsignedIDToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 serverIssuer(r),
				"authorization_endpoint": serverIssuer(r) + "/authorize",
				"token_endpoint":         serverIssuer(r) + "/token",
				"userinfo_endpoint":      "",
				"jwks_uri":               serverIssuer(r) + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"id_token":     "eyJhbGciOiJub25lIn0.eyJzdWIiOiJmb3JnZWQifQ.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer SetOAuthHTTPClientForTest(oauthClientForTLSServer(t, server))()

	issuer := "https://idp.example.test"
	source := &model.AuthSource{
		Name:               "oidc-test",
		Type:               model.AuthSourceTypeOIDC,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		OpenIDDiscoveryURL: issuer + "/.well-known/openid-configuration",
	}
	if _, err := ExchangeOAuthProfileWithNonce(context.Background(), source, "code", "https://app.example.com/oauth/oidc-test", "nonce"); err == nil {
		t.Fatal("expected unsigned id token to be rejected")
	}
}

func TestAuthSourceValidateRejectsUnsafeOIDCDiscoveryURL(t *testing.T) {
	source := &model.AuthSource{
		Name:               "unsafe-oidc",
		Type:               model.AuthSourceTypeOIDC,
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		OpenIDDiscoveryURL: "http://127.0.0.1/.well-known/openid-configuration",
	}
	if err := source.Validate(); err == nil || !strings.Contains(err.Error(), "public HTTPS") {
		t.Fatalf("expected unsafe OIDC discovery URL rejection, got %v", err)
	}
}

func TestFetchOIDCDiscoveryRejectsUnsafeReturnedEndpoints(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 serverIssuer(r),
			"authorization_endpoint": serverIssuer(r) + "/authorize",
			"token_endpoint":         "https://127.0.0.1/token",
			"userinfo_endpoint":      serverIssuer(r) + "/userinfo",
			"jwks_uri":               serverIssuer(r) + "/jwks",
		})
	}))
	defer server.Close()
	defer SetOAuthHTTPClientForTest(oauthClientForTLSServer(t, server))()

	_, err := fetchOIDCDiscovery(context.Background(), "https://idp.example.test/.well-known/openid-configuration")
	if err == nil || !strings.Contains(err.Error(), "token_endpoint") {
		t.Fatalf("expected unsafe token endpoint rejection, got %v", err)
	}
}

func oauthClientForTLSServer(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	upstream, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse TLS server URL: %v", err)
	}
	baseClient := server.Client()
	baseTransport := baseClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	baseClient.Transport = oidcTestTransport{
		base:     baseTransport,
		upstream: upstream,
	}
	return baseClient
}

type oidcTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (transport oidcTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rewritten := request.Clone(request.Context())
	rewritten.URL = cloneURL(request.URL)
	rewritten.URL.Scheme = transport.upstream.Scheme
	rewritten.URL.Host = transport.upstream.Host
	rewritten.Host = request.URL.Host
	return transport.base.RoundTrip(rewritten)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	clone := *value
	return &clone
}

func signedOIDCIDTokenForTest(t *testing.T, privateKey ed25519.PrivateKey, issuer string, audience string, subject string, nonce string) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "test-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: privateKey}, options)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	raw, err := jwt.Signed(signer).
		Claims(jwt.Claims{
			Issuer:   issuer,
			Subject:  subject,
			Audience: jwt.Audience{audience},
			Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		}).
		Claims(map[string]any{
			"nonce":              nonce,
			"preferred_username": "alice",
			"name":               "Alice",
			"email":              "alice@example.com",
		}).
		Serialize()
	if err != nil {
		t.Fatalf("serialize token: %v", err)
	}
	return raw
}

func serverIssuer(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func createTestAuthSource(t *testing.T) *model.AuthSource {
	t.Helper()
	source := &model.AuthSource{
		Name:               "test-oidc",
		Type:               model.AuthSourceTypeOIDC,
		DisplayName:        "Test OIDC",
		ClientID:           "client-id",
		ClientSecret:       "client-secret",
		Scopes:             "openid profile email",
		OpenIDDiscoveryURL: "https://idp.example.com/.well-known/openid-configuration",
	}
	if err := model.CreateAuthSource(source); err != nil {
		t.Fatalf("CreateAuthSource failed: %v", err)
	}
	return source
}
