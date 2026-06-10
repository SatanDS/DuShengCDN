package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserMarshalJSONOmitsSecretsButUnmarshalAcceptsPassword(t *testing.T) {
	user := User{
		Id:               7,
		Username:         "alice",
		Password:         "plain-password",
		DisplayName:      "Alice",
		Role:             1,
		Status:           1,
		Token:            "bearer-token",
		Email:            "alice@example.com",
		GitHubId:         "gh-1",
		WeChatId:         "wx-1",
		VerificationCode: "123456",
		CSRFToken:        "csrf-token",
	}
	raw, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Marshal user: %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{`"password"`, `"token"`, `"verification_code"`, "plain-password", "bearer-token", "123456"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("marshaled user leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"csrf_token":"csrf-token"`) {
		t.Fatalf("expected csrf token to remain in login user view JSON: %s", text)
	}

	var decoded User
	if err := json.Unmarshal([]byte(`{"username":"bob","password":"secret123","token":"legacy-token","verification_code":"654321"}`), &decoded); err != nil {
		t.Fatalf("Unmarshal user: %v", err)
	}
	if decoded.Username != "bob" || decoded.Password != "secret123" || decoded.Token != "legacy-token" || decoded.VerificationCode != "654321" {
		t.Fatalf("expected request decoding to keep input fields, got %+v", decoded)
	}
}
