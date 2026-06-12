package controller

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service"
	"dushengcdn/utils/security"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const pendingExternalAccountSessionKey = "pending_external_account"
const pendingExternalAccountCSRFSessionKey = "pending_external_account_csrf"

type authSourceTogglePayload struct {
	IsActive bool `json:"is_active"`
}

type authSourcePayload struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	DisplayName        string `json:"display_name"`
	IsActive           bool   `json:"is_active"`
	ClientID           string `json:"client_id"`
	ClientSecret       string `json:"client_secret"`
	OpenIDDiscoveryURL string `json:"openid_discovery_url"`
	Scopes             string `json:"scopes"`
	IconURL            string `json:"icon_url"`
}

func (payload authSourcePayload) toModel() model.AuthSource {
	return model.AuthSource{
		Name:               payload.Name,
		Type:               payload.Type,
		DisplayName:        payload.DisplayName,
		IsActive:           payload.IsActive,
		ClientID:           payload.ClientID,
		ClientSecret:       payload.ClientSecret,
		OpenIDDiscoveryURL: payload.OpenIDDiscoveryURL,
		Scopes:             payload.Scopes,
		IconURL:            payload.IconURL,
	}
}

func ListAuthSources(c *gin.Context) {
	sources, err := model.GetAuthSources()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, sources)
}

func CreateAuthSource(c *gin.Context) {
	var payload authSourcePayload
	if err := decodeJSONBody(c.Request.Body, &payload); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	source := payload.toModel()
	if err := model.CreateAuthSource(&source); err != nil {
		respondFailure(c, err.Error())
		return
	}
	source.Sanitize()
	respondSuccess(c, source)
}

func UpdateAuthSource(c *gin.Context) {
	id, err := parseAuthSourceID(c)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	var payload authSourcePayload
	if err := decodeJSONBody(c.Request.Body, &payload); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	source := payload.toModel()
	source.ID = id
	keepSecret := strings.TrimSpace(source.ClientSecret) == ""
	if err := model.UpdateAuthSource(&source, keepSecret); err != nil {
		respondFailure(c, err.Error())
		return
	}
	updated, err := model.GetAuthSourceByID(id)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	updated.Sanitize()
	respondSuccess(c, updated)
}

func DeleteAuthSource(c *gin.Context) {
	id, err := parseAuthSourceID(c)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if err := model.DeleteAuthSource(id); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}

func ToggleAuthSource(c *gin.Context) {
	id, err := parseAuthSourceID(c)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	var payload authSourceTogglePayload
	if err := decodeJSONBody(c.Request.Body, &payload); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	if err := model.ToggleAuthSource(id, payload.IsActive); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}

func OAuthAuthorize(c *gin.Context) {
	source, err := getAuthSourceFromRoute(c)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if !source.IsActive {
		respondFailure(c, "认证源未启用")
		return
	}
	if err := source.Validate(); err != nil {
		respondFailure(c, err.Error())
		return
	}
	state, err := service.GenerateOAuthState()
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	nonce := ""
	if source.Type == model.AuthSourceTypeOIDC {
		nonce, err = service.GenerateOIDCNonce()
		if err != nil {
			respondFailure(c, err.Error())
			return
		}
	}
	session := sessions.Default(c)
	session.Set(oauthStateSessionKey(source.ID), state)
	if nonce != "" {
		session.Set(oauthNonceSessionKey(source.ID), nonce)
	}
	if err := session.Save(); err != nil {
		respondFailure(c, "无法保存授权状态，请重试")
		return
	}
	redirectURL := oauthFrontendCallbackURL(c, source)
	authorizeURL, err := service.BuildAuthorizeURLWithNonce(c.Request.Context(), source, redirectURL, state, nonce)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, gin.H{"authorize_url": authorizeURL})
}

func OAuthCallback(c *gin.Context) {
	source, err := getAuthSourceFromRoute(c)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if !source.IsActive {
		respondFailure(c, "认证源未启用")
		return
	}
	session := sessions.Default(c)
	expectedState, _ := session.Get(oauthStateSessionKey(source.ID)).(string)
	expectedNonce, _ := session.Get(oauthNonceSessionKey(source.ID)).(string)
	state := c.Query("state")
	if expectedState == "" || state == "" || state != expectedState {
		respondFailure(c, "授权状态无效，请重新登录")
		return
	}
	if source.Type == model.AuthSourceTypeOIDC && expectedNonce == "" {
		respondFailure(c, "OIDC nonce is missing; please start login again")
		return
	}
	session.Delete(oauthStateSessionKey(source.ID))
	session.Delete(oauthNonceSessionKey(source.ID))
	if oauthError := c.Query("error"); oauthError != "" {
		_ = session.Save()
		slog.Warn("oauth provider returned an error",
			"source_id", source.ID,
			"source_name", source.Name,
			"oauth_error", strings.TrimSpace(oauthError),
			"description_present", strings.TrimSpace(c.Query("error_description")) != "",
		)
		respondFailure(c, "第三方授权失败，请返回登录页重试")
		return
	}

	profile, err := service.ExchangeOAuthProfileWithNonce(c.Request.Context(), source, c.Query("code"), oauthFrontendCallbackURL(c, source), expectedNonce)
	if err != nil {
		_ = session.Save()
		slog.Warn("oauth profile exchange failed", "source_id", source.ID, "source_name", source.Name, "error", err)
		respondFailure(c, "第三方授权失败，请返回登录页重试")
		return
	}
	var currentUserID *int
	if currentUser, ok := authenticatedSessionUser(c); ok {
		currentUserID = &currentUser.Id
	}
	result, pending, err := service.CompleteOAuthLogin(source, profile, currentUserID)
	if err != nil {
		_ = session.Save()
		slog.Warn("oauth login completion failed", "source_id", source.ID, "source_name", source.Name, "error", err)
		respondFailure(c, "第三方授权失败，请返回登录页重试")
		return
	}
	if pending != nil {
		raw, err := json.Marshal(pending)
		if err != nil {
			respondFailure(c, err.Error())
			return
		}
		session.Set(pendingExternalAccountSessionKey, string(raw))
		csrfToken, err := ensurePendingExternalAccountCSRFToken(session)
		if err != nil {
			respondFailure(c, "无法保存待绑定账号，请重试")
			return
		}
		result.CSRFToken = csrfToken
		if err := session.Save(); err != nil {
			respondFailure(c, "无法保存待绑定账号，请重试")
			return
		}
		respondSuccess(c, result)
		return
	}
	if result.User != nil {
		cleanUser, err := setLoginSession(result.User, c)
		if err != nil {
			respondFailure(c, "无法保存会话信息，请重试")
			return
		}
		result.User = cleanUser
	} else if err := session.Save(); err != nil {
		respondFailure(c, "无法更新授权状态，请重试")
		return
	}
	respondSuccess(c, result)
}

func LinkExistingOAuthAccount(c *gin.Context) {
	session := sessions.Default(c)
	raw, _ := session.Get(pendingExternalAccountSessionKey).(string)
	if raw == "" {
		respondFailure(c, "待绑定第三方账号已失效，请重新登录")
		return
	}
	if !verifyPendingExternalAccountCSRF(c, session) {
		respondFailure(c, "CSRF token invalid or missing")
		return
	}
	var pending service.PendingExternalAccount
	if err := json.Unmarshal([]byte(raw), &pending); err != nil {
		respondFailure(c, "待绑定第三方账号无效，请重新登录")
		return
	}
	var input service.LinkExistingRequest
	if err := decodeJSONBody(c.Request.Body, &input); err != nil {
		respondBadRequest(c, "无效的参数")
		return
	}
	loginKey := loginFailureKeyForRequest(input.Username, c)
	if locked, retryAfter := loginAttemptLocked(loginKey, time.Now()); locked {
		c.JSON(http.StatusOK, gin.H{
			"message":             "登录失败次数过多，请稍后再试",
			"success":             false,
			"retry_after_seconds": int(retryAfter.Seconds()),
		})
		return
	}
	user, err := service.LinkPendingExternalAccount(&pending, input)
	if err != nil {
		recordLoginFailure(loginKey, time.Now())
		respondFailure(c, err.Error())
		return
	}
	clearLoginFailure(loginKey)
	session.Delete(pendingExternalAccountSessionKey)
	session.Delete(pendingExternalAccountCSRFSessionKey)
	if err := session.Save(); err != nil {
		respondFailure(c, "无法更新会话信息，请重试")
		return
	}
	cleanUser, err := setLoginSession(user, c)
	if err != nil {
		respondFailure(c, "无法保存会话信息，请重试")
		return
	}
	respondSuccess(c, service.OAuthCallbackResult{Status: "linked", User: cleanUser})
}

func ensurePendingExternalAccountCSRFToken(session sessions.Session) (string, error) {
	token, _ := session.Get(pendingExternalAccountCSRFSessionKey).(string)
	if token != "" {
		return token, nil
	}
	token = security.GenerateCSRFToken()
	session.Set(pendingExternalAccountCSRFSessionKey, token)
	return token, nil
}

func verifyPendingExternalAccountCSRF(c *gin.Context, session sessions.Session) bool {
	expected, _ := session.Get(pendingExternalAccountCSRFSessionKey).(string)
	provided := c.GetHeader("X-CSRF-Token")
	return security.VerifyCSRFToken(expected, provided)
}

func ListExternalAccounts(c *gin.Context) {
	userID := c.GetInt("id")
	accounts, err := model.ListExternalAccountsByUserID(userID)
	if err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccess(c, accounts)
}

func DeleteExternalAccount(c *gin.Context) {
	rawID := strings.TrimSpace(c.Param("id"))
	parsedID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || parsedID == 0 {
		respondBadRequest(c, "绑定记录 ID 无效")
		return
	}
	if err := model.DeleteExternalAccountForUser(uint(parsedID), c.GetInt("id")); err != nil {
		respondFailure(c, err.Error())
		return
	}
	respondSuccessMessage(c, "")
}

func parseAuthSourceID(c *gin.Context) (uint, error) {
	raw := c.Param("source_id")
	if raw == "" {
		raw = c.Param("id")
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("认证源 ID 无效")
	}
	return uint(parsed), nil
}

func getAuthSourceFromRoute(c *gin.Context) (*model.AuthSource, error) {
	raw := strings.TrimSpace(c.Param("source"))
	if raw == "" {
		raw = strings.TrimSpace(c.Param("source_id"))
	}
	if raw == "" {
		raw = strings.TrimSpace(c.Param("id"))
	}
	if raw == "" {
		return nil, fmt.Errorf("认证源不能为空")
	}
	if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil && parsed > 0 {
		source, err := model.GetAuthSourceByID(uint(parsed))
		if err != nil {
			return nil, err
		}
		return source, nil
	}
	source, err := model.GetAuthSourceByName(raw)
	if err != nil {
		return nil, err
	}
	return source, nil
}

func oauthStateSessionKey(sourceID uint) string {
	return fmt.Sprintf("oauth_state_%d", sourceID)
}

func oauthNonceSessionKey(sourceID uint) string {
	return fmt.Sprintf("oauth_nonce_%d", sourceID)
}

func oauthFrontendCallbackURL(c *gin.Context, source *model.AuthSource) string {
	base := strings.TrimRight(common.ServerAddress, "/")
	if base == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := c.Request.Host
		if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
			host = forwardedHost
		}
		base = scheme + "://" + host
	}
	sourceName := strconv.FormatUint(uint64(source.ID), 10)
	if strings.TrimSpace(source.Name) != "" {
		sourceName = source.Name
	}
	callback, _ := url.JoinPath(base, "oauth", sourceName)
	return callback
}
