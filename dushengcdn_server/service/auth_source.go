package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"gorm.io/gorm"
)

type PublicAuthSource struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	DisplayName  string `json:"display_name"`
	AuthorizeURL string `json:"authorize_url"`
	IconURL      string `json:"icon_url"`
}

type OAuthProfile struct {
	ExternalID       string
	ExternalUsername string
	DisplayName      string
	Email            string
}

type OAuthCallbackResult struct {
	Status    string      `json:"status"`
	User      *model.User `json:"user,omitempty"`
	CSRFToken string      `json:"csrf_token,omitempty"`
}

type LinkExistingRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type PendingExternalAccount struct {
	AuthSourceID     uint   `json:"auth_source_id"`
	ExternalID       string `json:"external_id"`
	ExternalUsername string `json:"external_username"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	Issuer                string `json:"issuer"`
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
	Scope       string `json:"scope"`
}

var oauthHTTPClient = security.NewPublicHTTPClient(8*time.Second, true)

func SetOAuthHTTPClientForTest(client *http.Client) func() {
	previous := oauthHTTPClient
	if client == nil {
		oauthHTTPClient = security.NewPublicHTTPClient(8*time.Second, true)
	} else {
		oauthHTTPClient = client
	}
	return func() {
		oauthHTTPClient = previous
	}
}

var allowedOIDCSignatureAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256,
	jose.RS384,
	jose.RS512,
	jose.PS256,
	jose.PS384,
	jose.PS512,
	jose.ES256,
	jose.ES384,
	jose.ES512,
	jose.EdDSA,
}

func GenerateOAuthState() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func GenerateOIDCNonce() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func PublicAuthSources(baseAPIPath string) ([]PublicAuthSource, error) {
	sources, err := model.GetActiveAuthSources()
	if err != nil {
		return nil, err
	}
	result := make([]PublicAuthSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, PublicAuthSource{
			ID:           source.ID,
			Name:         source.Name,
			Type:         source.Type,
			DisplayName:  source.DisplayName,
			AuthorizeURL: fmt.Sprintf("%s/oauth/%s/authorize", strings.TrimRight(baseAPIPath, "/"), url.PathEscape(source.Name)),
			IconURL:      source.IconURL,
		})
	}
	return result, nil
}

func BuildAuthorizeURL(ctx context.Context, source *model.AuthSource, redirectURL string, state string) (string, error) {
	return BuildAuthorizeURLWithNonce(ctx, source, redirectURL, state, "")
}

func BuildAuthorizeURLWithNonce(ctx context.Context, source *model.AuthSource, redirectURL string, state string, nonce string) (string, error) {
	source.Normalize()
	switch source.Type {
	case model.AuthSourceTypeGitHub:
		authorizeURL, err := url.Parse("https://github.com/login/oauth/authorize")
		if err != nil {
			return "", err
		}
		values := authorizeURL.Query()
		values.Set("client_id", source.ClientID)
		values.Set("redirect_uri", redirectURL)
		values.Set("scope", source.Scopes)
		values.Set("state", state)
		authorizeURL.RawQuery = values.Encode()
		return authorizeURL.String(), nil
	case model.AuthSourceTypeOIDC:
		discovery, err := fetchOIDCDiscovery(ctx, source.OpenIDDiscoveryURL)
		if err != nil {
			return "", err
		}
		authorizeURL, err := url.Parse(discovery.AuthorizationEndpoint)
		if err != nil {
			return "", err
		}
		values := authorizeURL.Query()
		values.Set("client_id", source.ClientID)
		values.Set("redirect_uri", redirectURL)
		values.Set("response_type", "code")
		values.Set("scope", source.Scopes)
		values.Set("state", state)
		if strings.TrimSpace(nonce) != "" {
			values.Set("nonce", strings.TrimSpace(nonce))
		}
		authorizeURL.RawQuery = values.Encode()
		return authorizeURL.String(), nil
	default:
		return "", errors.New("不支持的认证源类型")
	}
}

func ExchangeOAuthProfile(ctx context.Context, source *model.AuthSource, code string, redirectURL string) (*OAuthProfile, error) {
	return ExchangeOAuthProfileWithNonce(ctx, source, code, redirectURL, "")
}

func ExchangeOAuthProfileWithNonce(ctx context.Context, source *model.AuthSource, code string, redirectURL string, nonce string) (*OAuthProfile, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("授权 code 不能为空")
	}
	source.Normalize()
	switch source.Type {
	case model.AuthSourceTypeGitHub:
		return exchangeGitHubProfile(ctx, source, code, redirectURL)
	case model.AuthSourceTypeOIDC:
		return exchangeOIDCProfile(ctx, source, code, redirectURL, nonce)
	default:
		return nil, errors.New("不支持的认证源类型")
	}
}

func CompleteOAuthLogin(source *model.AuthSource, profile *OAuthProfile, currentUserID *int) (*OAuthCallbackResult, *PendingExternalAccount, error) {
	if source == nil || profile == nil || strings.TrimSpace(profile.ExternalID) == "" {
		return nil, nil, errors.New("第三方账号资料不完整")
	}

	account, err := model.FindExternalAccount(source.ID, profile.ExternalID)
	if err == nil {
		user, err := model.GetUserById(account.UserID, false)
		if err != nil {
			return nil, nil, err
		}
		if user.Status != common.UserStatusEnabled {
			return nil, nil, errors.New("用户已被封禁")
		}
		return &OAuthCallbackResult{Status: "logged_in", User: user}, nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}

	if currentUserID != nil && *currentUserID > 0 {
		user, err := model.GetUserById(*currentUserID, false)
		if err != nil {
			return nil, nil, err
		}
		if user.Status != common.UserStatusEnabled {
			return nil, nil, errors.New("用户已被封禁")
		}
		if err := model.LinkExternalAccount(&model.ExternalAccount{
			AuthSourceID:     source.ID,
			UserID:           user.Id,
			ExternalID:       profile.ExternalID,
			ExternalUsername: profile.ExternalUsername,
			Email:            profile.Email,
		}); err != nil {
			return nil, nil, err
		}
		return &OAuthCallbackResult{Status: "linked", User: user}, nil, nil
	}

	if common.RegisterEnabled {
		user, err := createUserFromOAuthProfile(source, profile)
		if err != nil {
			return nil, nil, err
		}
		return &OAuthCallbackResult{Status: "registered", User: user}, nil, nil
	}

	pending := &PendingExternalAccount{
		AuthSourceID:     source.ID,
		ExternalID:       profile.ExternalID,
		ExternalUsername: profile.ExternalUsername,
		DisplayName:      profile.DisplayName,
		Email:            profile.Email,
	}
	return &OAuthCallbackResult{Status: "link_required"}, pending, nil
}

func LinkPendingExternalAccount(pending *PendingExternalAccount, input LinkExistingRequest) (*model.User, error) {
	if pending == nil || pending.AuthSourceID == 0 || pending.ExternalID == "" {
		return nil, errors.New("待绑定第三方账号已失效，请重新登录")
	}
	user := model.User{
		Username: strings.TrimSpace(input.Username),
		Password: input.Password,
	}
	if err := user.ValidateAndFill(); err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, errors.New("用户已被封禁")
	}

	if existing, err := model.FindExternalAccount(pending.AuthSourceID, pending.ExternalID); err == nil {
		if existing.UserID != user.Id {
			return nil, errors.New("该第三方账号已绑定其他用户")
		}
		return &user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     pending.AuthSourceID,
		UserID:           user.Id,
		ExternalID:       pending.ExternalID,
		ExternalUsername: pending.ExternalUsername,
		Email:            pending.Email,
	}); err != nil {
		return nil, err
	}
	return &user, nil
}

func createUserFromOAuthProfile(source *model.AuthSource, profile *OAuthProfile) (*model.User, error) {
	displayName := strings.TrimSpace(profile.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(profile.ExternalUsername)
	}
	if displayName == "" {
		displayName = source.DisplayName + " User"
	}
	if len([]rune(displayName)) > 20 {
		displayName = string([]rune(displayName)[:20])
	}

	user := &model.User{}
	for attempt := 0; attempt < 20; attempt++ {
		username, err := newOAuthUsername(source)
		if err != nil {
			return nil, err
		}
		user = &model.User{
			Username:    username,
			DisplayName: displayName,
			Email:       profile.Email,
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		}
		if err := user.Insert(); err != nil {
			if isAuthSourceUniqueConstraintError(err) {
				continue
			}
			return nil, err
		}
		break
	}
	if user.Id == 0 {
		return nil, errors.New("OAuth 用户名生成冲突，请重试")
	}
	if err := model.LinkExternalAccount(&model.ExternalAccount{
		AuthSourceID:     source.ID,
		UserID:           user.Id,
		ExternalID:       profile.ExternalID,
		ExternalUsername: profile.ExternalUsername,
		Email:            profile.Email,
	}); err != nil {
		return nil, err
	}
	return user, nil
}

func newOAuthUsername(source *model.AuthSource) (string, error) {
	prefix := "oa"
	if source != nil {
		switch source.Type {
		case model.AuthSourceTypeGitHub:
			prefix = "gh"
		case model.AuthSourceTypeOIDC:
			prefix = "oidc"
		}
	}
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	randomPart := hex.EncodeToString(buffer)
	if len(prefix)+1+len(randomPart) > 12 {
		randomPart = randomPart[:12-len(prefix)-1]
	}
	return prefix + "_" + randomPart, nil
}

func exchangeGitHubProfile(ctx context.Context, source *model.AuthSource, code string, redirectURL string) (*OAuthProfile, error) {
	values := map[string]string{
		"client_id":     source.ClientID,
		"client_secret": source.ClientSecret,
		"code":          code,
		"redirect_uri":  redirectURL,
	}
	body, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		slog.Error("github oauth access token request failed", "error", err)
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub token 接口返回异常状态: %s", resp.Status)
	}
	var token oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, errors.New("GitHub 未返回 access token")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err = oauthHTTPClient.Do(req)
	if err != nil {
		slog.Error("github user info request failed", "error", err)
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub 用户接口返回异常状态: %s", resp.Status)
	}
	var githubUser struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		return nil, err
	}
	if githubUser.ID == 0 && githubUser.Login == "" {
		return nil, errors.New("GitHub 用户资料缺少唯一标识")
	}
	if githubUser.ID == 0 {
		return nil, errors.New("GitHub user profile is missing numeric id")
	}
	return &OAuthProfile{
		ExternalID:       fmt.Sprintf("%d", githubUser.ID),
		ExternalUsername: githubUser.Login,
		DisplayName:      firstNonEmpty(githubUser.Name, githubUser.Login),
		Email:            githubUser.Email,
	}, nil
}

func exchangeOIDCProfile(ctx context.Context, source *model.AuthSource, code string, redirectURL string, nonce string) (*OAuthProfile, error) {
	discovery, err := fetchOIDCDiscovery(ctx, source.OpenIDDiscoveryURL)
	if err != nil {
		return nil, err
	}
	token, err := exchangeOIDCToken(ctx, discovery.TokenEndpoint, source, code, redirectURL)
	if err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, errors.New("OIDC 未返回 access token")
	}
	if strings.TrimSpace(token.IDToken) == "" {
		return nil, errors.New("OIDC did not return an ID token")
	}
	idTokenClaims, err := verifyOIDCIDToken(ctx, discovery, source, token.IDToken, nonce)
	if err != nil {
		return nil, err
	}
	claims, err := fetchOIDCUserInfo(ctx, discovery.UserInfoEndpoint, token.AccessToken)
	if err != nil {
		return nil, err
	}
	claims, err = mergeOIDCClaims(idTokenClaims, claims)
	if err != nil {
		return nil, err
	}
	profile := profileFromClaims(claims)
	if profile.ExternalID == "" {
		return nil, errors.New("OIDC 用户资料缺少 sub")
	}
	return profile, nil
}

func fetchOIDCDiscovery(ctx context.Context, discoveryURL string) (*oidcDiscovery, error) {
	discoveryOrigin, err := security.ValidatePublicHTTPURL(discoveryURL, true)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery URL is not allowed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法获取 OIDC discovery 配置: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OIDC discovery 返回异常状态: %s", resp.Status)
	}
	var discovery oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, err
	}
	if discovery.AuthorizationEndpoint == "" || discovery.TokenEndpoint == "" || discovery.JWKSURI == "" || discovery.Issuer == "" {
		return nil, errors.New("OIDC discovery 缺少授权或 token 端点")
	}
	if err := validateOIDCDiscoveryEndpoints(discoveryOrigin, &discovery); err != nil {
		return nil, err
	}
	return &discovery, nil
}

func validateOIDCDiscoveryEndpoints(discoveryOrigin *url.URL, discovery *oidcDiscovery) error {
	if discovery == nil {
		return errors.New("OIDC discovery is empty")
	}
	issuerURL, err := security.ValidatePublicHTTPURL(discovery.Issuer, true)
	if err != nil {
		return fmt.Errorf("OIDC issuer URL is not allowed: %w", err)
	}
	if !security.SameOrigin(discoveryOrigin, issuerURL) {
		return errors.New("OIDC issuer must use the same origin as discovery URL")
	}
	endpoints := map[string]string{
		"authorization_endpoint": discovery.AuthorizationEndpoint,
		"token_endpoint":         discovery.TokenEndpoint,
		"jwks_uri":               discovery.JWKSURI,
	}
	if strings.TrimSpace(discovery.UserInfoEndpoint) != "" {
		endpoints["userinfo_endpoint"] = discovery.UserInfoEndpoint
	}
	for name, endpoint := range endpoints {
		endpointURL, err := security.ValidatePublicHTTPURL(endpoint, true)
		if err != nil {
			return fmt.Errorf("OIDC %s is not allowed: %w", name, err)
		}
		if !security.SameOrigin(issuerURL, endpointURL) {
			return fmt.Errorf("OIDC %s must use the same origin as issuer", name)
		}
	}
	return nil
}

func exchangeOIDCToken(ctx context.Context, tokenEndpoint string, source *model.AuthSource, code string, redirectURL string) (*oauthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", source.ClientID)
	form.Set("client_secret", source.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC token 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OIDC token 接口返回异常状态: %s", resp.Status)
	}
	var token oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}
	return &token, nil
}

func fetchOIDCUserInfo(ctx context.Context, endpoint string, accessToken string) (map[string]any, error) {
	if endpoint == "" {
		return map[string]any{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC userinfo 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OIDC userinfo 返回异常状态: %s", resp.Status)
	}
	var claims map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func fetchOIDCJWKS(ctx context.Context, jwksURL string) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OIDC JWKS request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OIDC JWKS returned %s", resp.Status)
	}
	var jwks jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	if len(jwks.Keys) == 0 {
		return nil, errors.New("OIDC JWKS is empty")
	}
	return &jwks, nil
}

type oidcIDTokenCustomClaims struct {
	Nonce             string `json:"nonce"`
	PreferredUsername string `json:"preferred_username"`
	Nickname          string `json:"nickname"`
	Name              string `json:"name"`
	Email             string `json:"email"`
}

func verifyOIDCIDToken(ctx context.Context, discovery *oidcDiscovery, source *model.AuthSource, rawIDToken string, expectedNonce string) (map[string]any, error) {
	token, err := jwt.ParseSigned(rawIDToken, allowedOIDCSignatureAlgorithms)
	if err != nil {
		return nil, fmt.Errorf("OIDC ID token format is invalid: %w", err)
	}
	jwks, err := fetchOIDCJWKS(ctx, discovery.JWKSURI)
	if err != nil {
		return nil, err
	}

	var claims jwt.Claims
	var custom oidcIDTokenCustomClaims
	if err := token.Claims(jwks, &claims, &custom); err != nil {
		return nil, fmt.Errorf("OIDC ID token signature validation failed: %w", err)
	}
	expected := jwt.Expected{
		Issuer:      discovery.Issuer,
		AnyAudience: jwt.Audience{source.ClientID},
		Time:        time.Now(),
	}
	if err := claims.ValidateWithLeeway(expected, time.Minute); err != nil {
		return nil, fmt.Errorf("OIDC ID token claims validation failed: %w", err)
	}
	expectedNonce = strings.TrimSpace(expectedNonce)
	if expectedNonce != "" {
		if custom.Nonce == "" {
			return nil, errors.New("OIDC ID token is missing nonce")
		}
		if custom.Nonce != expectedNonce {
			return nil, errors.New("OIDC ID token nonce validation failed")
		}
	}

	result := map[string]any{
		"sub": claims.Subject,
	}
	if custom.PreferredUsername != "" {
		result["preferred_username"] = custom.PreferredUsername
	}
	if custom.Nickname != "" {
		result["nickname"] = custom.Nickname
	}
	if custom.Name != "" {
		result["name"] = custom.Name
	}
	if custom.Email != "" {
		result["email"] = custom.Email
	}
	return result, nil
}

func mergeOIDCClaims(idTokenClaims map[string]any, userInfoClaims map[string]any) (map[string]any, error) {
	if len(userInfoClaims) == 0 {
		return idTokenClaims, nil
	}
	merged := make(map[string]any, len(idTokenClaims)+len(userInfoClaims))
	for key, value := range idTokenClaims {
		merged[key] = value
	}
	idTokenSub := strings.TrimSpace(stringClaimValue(idTokenClaims["sub"]))
	for key, value := range userInfoClaims {
		if key == "sub" {
			userInfoSub := strings.TrimSpace(stringClaimValue(value))
			if userInfoSub != "" && userInfoSub != idTokenSub {
				return nil, errors.New("OIDC userinfo subject does not match ID token subject")
			}
			continue
		}
		merged[key] = value
	}
	if strings.TrimSpace(stringClaimValue(merged["sub"])) == "" {
		merged["sub"] = idTokenClaims["sub"]
	}
	return merged, nil
}

func stringClaimValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func profileFromClaims(claims map[string]any) *OAuthProfile {
	stringClaim := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := claims[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	return &OAuthProfile{
		ExternalID:       stringClaim("sub"),
		ExternalUsername: stringClaim("preferred_username", "nickname", "name", "email"),
		DisplayName:      stringClaim("name", "preferred_username", "nickname", "email"),
		Email:            stringClaim("email"),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isAuthSourceUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
