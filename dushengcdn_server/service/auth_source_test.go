package service

import (
	"strings"
	"testing"

	"dushengcdn/common"
	"dushengcdn/model"
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
