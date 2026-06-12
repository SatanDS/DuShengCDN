package controller

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service"
	"dushengcdn/utils/security"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const legacyGitHubOAuthStateSessionKey = "legacy_github_oauth_state"

var legacyGitHubHTTPClient = security.NewPublicHTTPClient(5*time.Second, true)

type GitHubOAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type GitHubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func getGitHubUserInfoByCode(code string, redirectURL string) (*GitHubUser, error) {
	if code == "" {
		return nil, errors.New("无效的参数")
	}
	values := map[string]string{
		"client_id":     common.GitHubClientId,
		"client_secret": common.GitHubClientSecret,
		"code":          code,
	}
	if redirectURL != "" {
		values["redirect_uri"] = redirectURL
	}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := legacyGitHubHTTPClient.Do(req)
	if err != nil {
		slog.Error("github oauth access token request failed", "error", err)
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub token 接口返回异常状态: %s", res.Status)
	}
	var oAuthResponse GitHubOAuthResponse
	if err := json.NewDecoder(res.Body).Decode(&oAuthResponse); err != nil {
		return nil, err
	}
	if strings.TrimSpace(oAuthResponse.AccessToken) == "" {
		return nil, errors.New("GitHub 未返回 access token")
	}
	req, err = http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oAuthResponse.AccessToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	res2, err := legacyGitHubHTTPClient.Do(req)
	if err != nil {
		slog.Error("github user info request failed", "error", err)
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	if res2.StatusCode < 200 || res2.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub 用户接口返回异常状态: %s", res2.Status)
	}
	var githubUser GitHubUser
	if err := json.NewDecoder(res2.Body).Decode(&githubUser); err != nil {
		return nil, err
	}
	if githubUser.Login == "" {
		return nil, errors.New("返回值非法，用户字段为空，请稍后重试！")
	}
	return &githubUser, nil
}

func GitHubOAuth(c *gin.Context) {
	if !common.GitHubOAuthEnabled {
		legacyGitHubOAuthFailure(c, "管理员未开启通过 GitHub 登录以及注册")
		return
	}
	if oauthError := strings.TrimSpace(c.Query("error")); oauthError != "" {
		if err := validateLegacyGitHubOAuthState(c); err != nil {
			legacyGitHubOAuthFailure(c, err.Error())
			return
		}
		slog.Warn("legacy github oauth provider returned an error",
			"oauth_error", oauthError,
			"description_present", strings.TrimSpace(c.Query("error_description")) != "",
		)
		legacyGitHubOAuthFailure(c, "GitHub 授权失败，请返回登录页重试")
		return
	}
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		GitHubOAuthAuthorize(c)
		return
	}
	if err := validateLegacyGitHubOAuthState(c); err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	githubUser, err := getGitHubUserInfoByCode(code, legacyGitHubOAuthFrontendCallbackURL(c))
	if err != nil {
		slog.Warn("legacy github oauth profile exchange failed", "error", err)
		legacyGitHubOAuthFailure(c, "GitHub 授权失败，请返回登录页重试")
		return
	}
	if _, ok := authenticatedSessionUser(c); ok {
		completeGitHubBind(c, githubUser)
		return
	}
	completeGitHubLogin(c, githubUser)
}

func GitHubOAuthAuthorize(c *gin.Context) {
	if !common.GitHubOAuthEnabled {
		legacyGitHubOAuthFailure(c, "管理员未开启通过 GitHub 登录以及注册")
		return
	}
	if strings.TrimSpace(common.GitHubClientId) == "" || strings.TrimSpace(common.GitHubClientSecret) == "" {
		legacyGitHubOAuthFailure(c, "GitHub OAuth 配置不完整，请联系管理员")
		return
	}
	state, err := service.GenerateOAuthState()
	if err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	session := sessions.Default(c)
	session.Set(legacyGitHubOAuthStateSessionKey, state)
	if err := session.Save(); err != nil {
		legacyGitHubOAuthFailure(c, "无法保存授权状态，请重试")
		return
	}
	authorizeURL, err := legacyGitHubOAuthAuthorizeURL(c, state)
	if err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"authorize_url": authorizeURL,
		},
	})
}

func completeGitHubLogin(c *gin.Context, githubUser *GitHubUser) {
	githubID, err := stableGitHubUserID(githubUser)
	if err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	user := model.User{
		GitHubId: githubID,
	}
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		if err := user.FillUserByGitHubId(); err != nil {
			legacyGitHubOAuthFailure(c, err.Error())
			return
		}
	} else {
		if common.RegisterEnabled {
			username, err := newLegacyGitHubUsername()
			if err != nil {
				legacyGitHubOAuthFailure(c, err.Error())
				return
			}
			user.Username = username
			if githubUser.Name != "" {
				user.DisplayName = githubUser.Name
			} else {
				user.DisplayName = "GitHub User"
			}
			user.Email = githubUser.Email
			user.Role = common.RoleCommonUser
			user.Status = common.UserStatusEnabled

			if err := user.Insert(); err != nil {
				legacyGitHubOAuthFailure(c, err.Error())
				return
			}
		} else {
			legacyGitHubOAuthFailure(c, "管理员关闭了新用户注册")
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		legacyGitHubOAuthFailure(c, "用户已被封禁")
		return
	}
	setupLogin(&user, c)
}

func completeGitHubBind(c *gin.Context, githubUser *GitHubUser) {
	githubID, err := stableGitHubUserID(githubUser)
	if err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	user := model.User{
		GitHubId: githubID,
	}
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		legacyGitHubOAuthFailure(c, "该 GitHub 账户已被绑定")
		return
	}
	currentUser, ok := authenticatedSessionUser(c)
	if !ok {
		legacyGitHubOAuthFailure(c, "登录状态已失效，请重新登录后再绑定 GitHub")
		return
	}
	user = *currentUser
	user.GitHubId = githubID
	if err := user.Update(false); err != nil {
		legacyGitHubOAuthFailure(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
}

func stableGitHubUserID(githubUser *GitHubUser) (string, error) {
	if githubUser == nil || githubUser.ID == 0 {
		return "", errors.New("GitHub user profile is missing numeric id")
	}
	return fmt.Sprintf("%d", githubUser.ID), nil
}

func validateLegacyGitHubOAuthState(c *gin.Context) error {
	session := sessions.Default(c)
	expectedState, _ := session.Get(legacyGitHubOAuthStateSessionKey).(string)
	actualState := strings.TrimSpace(c.Query("state"))
	session.Delete(legacyGitHubOAuthStateSessionKey)
	if err := session.Save(); err != nil {
		return errors.New("无法更新授权状态，请重试")
	}
	if expectedState == "" || actualState == "" || subtle.ConstantTimeCompare([]byte(expectedState), []byte(actualState)) != 1 {
		return errors.New("授权状态无效，请重新登录")
	}
	return nil
}

func legacyGitHubOAuthAuthorizeURL(c *gin.Context, state string) (string, error) {
	authorizeURL, err := url.Parse("https://github.com/login/oauth/authorize")
	if err != nil {
		return "", err
	}
	values := authorizeURL.Query()
	values.Set("client_id", common.GitHubClientId)
	values.Set("redirect_uri", legacyGitHubOAuthFrontendCallbackURL(c))
	values.Set("scope", "user:email")
	values.Set("state", state)
	authorizeURL.RawQuery = values.Encode()
	return authorizeURL.String(), nil
}

func legacyGitHubOAuthFrontendCallbackURL(c *gin.Context) string {
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
	callback, _ := url.JoinPath(base, "oauth", "github")
	parsed, err := url.Parse(callback)
	if err != nil {
		return callback
	}
	query := parsed.Query()
	query.Set("legacy", "1")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func newLegacyGitHubUsername() (string, error) {
	for attempt := 0; attempt < 20; attempt++ {
		buffer := make([]byte, 4)
		if _, err := rand.Read(buffer); err != nil {
			return "", err
		}
		username := "gh_" + hex.EncodeToString(buffer)
		if !model.IsUsernameAlreadyTaken(username) {
			return username, nil
		}
	}
	return "", errors.New("GitHub 用户名生成冲突，请重试")
}

func legacyGitHubOAuthFailure(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": message,
	})
}
