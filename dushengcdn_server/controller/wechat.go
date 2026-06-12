package controller

import (
	"crypto/rand"
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service"
	"dushengcdn/utils/security"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
)

const wechatUsernameMaxAttempts = 20
const wechatOAuthStateSessionKey = "wechat_oauth_state"

var wechatHTTPClient = security.NewPublicHTTPClient(5*time.Second, true)

func SetWeChatHTTPClientForTest(client *http.Client) func() {
	previous := wechatHTTPClient
	if client == nil {
		wechatHTTPClient = security.NewPublicHTTPClient(5*time.Second, true)
	} else {
		wechatHTTPClient = client
	}
	return func() {
		wechatHTTPClient = previous
	}
}

type wechatLoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type wechatBindRequest struct {
	Code string `json:"code"`
}

func WeChatOAuthAuthorize(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	state, err := service.GenerateOAuthState()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	session := sessions.Default(c)
	session.Set(wechatOAuthStateSessionKey, state)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "failed to save oauth state",
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"state": state,
		},
	})
}

func getWeChatIdByCode(code string) (string, error) {
	if code == "" {
		return "", errors.New("无效的参数")
	}
	wechatServerAddress := strings.TrimSpace(common.WeChatServerAddress)
	if wechatServerAddress == "" {
		return "", errors.New("微信登录服务地址未配置")
	}
	if _, err := security.ValidatePublicHTTPURL(wechatServerAddress, true); err != nil {
		return "", fmt.Errorf("微信登录服务地址不安全: %w", err)
	}
	endpoint, err := url.JoinPath(wechatServerAddress, "api", "wechat", "user")
	if err != nil {
		return "", err
	}
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := requestURL.Query()
	query.Set("code", code)
	requestURL.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", common.WeChatServerToken)
	httpResponse, err := wechatHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return "", fmt.Errorf("微信登录服务返回异常状态: %s", httpResponse.Status)
	}
	var res wechatLoginResponse
	err = json.NewDecoder(httpResponse.Body).Decode(&res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", errors.New("微信登录验证失败")
	}
	if res.Data == "" {
		return "", errors.New("验证码错误或已过期")
	}
	return res.Data, nil
}

func createWeChatUser(wechatId string) (*model.User, error) {
	for attempt := 0; attempt < wechatUsernameMaxAttempts; attempt++ {
		username, err := newWeChatUsername()
		if err != nil {
			return nil, err
		}
		user := &model.User{
			Username:    username,
			DisplayName: "WeChat User",
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			WeChatId:    wechatId,
		}
		if err := user.Insert(); err != nil {
			if isWeChatUniqueConstraintError(err) {
				continue
			}
			return nil, err
		}
		return user, nil
	}
	return nil, errors.New("微信用户名生成冲突，请重试")
}

func newWeChatUsername() (string, error) {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "wx_" + hex.EncodeToString(buffer), nil
}

func isWeChatUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

func WeChatAuth(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	if err := validateWeChatOAuthState(c); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	user := model.User{
		WeChatId: wechatId,
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		err := user.FillUserByWeChatId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if common.RegisterEnabled {
			createdUser, err := createWeChatUser(wechatId)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			user = *createdUser
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员关闭了新用户注册",
			})
			return
		}
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}
	setupLogin(&user, c)
}

func validateWeChatOAuthState(c *gin.Context) error {
	session := sessions.Default(c)
	expectedState, _ := session.Get(wechatOAuthStateSessionKey).(string)
	actualState := strings.TrimSpace(c.Query("state"))
	session.Delete(wechatOAuthStateSessionKey)
	_ = session.Save()
	if expectedState == "" || actualState == "" || actualState != expectedState {
		return errors.New("wechat oauth state invalid or expired")
	}
	return nil
}

func WeChatBindPost(c *gin.Context) {
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	var req wechatBindRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "invalid request",
			"success": false,
		})
		return
	}
	bindWeChatAccountWithCode(c, strings.TrimSpace(req.Code))
}

func bindWeChatAccountWithCode(c *gin.Context, code string) {
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该微信账号已被绑定",
		})
		return
	}
	id := c.GetInt("id")
	user := model.User{
		Id: id,
	}
	err = user.FillUserById()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.WeChatId = wechatId
	err = user.Update(false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
