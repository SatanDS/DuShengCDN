package controller

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/utils/security"
	"dushengcdn/utils/validation"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const csrfSessionKey = "csrf_token"
const passwordSessionFingerprintKey = "password_fingerprint"
const loginFailureMaxAttempts = 5
const loginFailureWindow = 10 * time.Minute
const loginFailureLockout = 10 * time.Minute

type loginFailureRecord struct {
	Count       int
	FirstAt     time.Time
	LockedUntil time.Time
}

var (
	loginFailureMutex sync.Mutex
	loginFailures     = map[string]loginFailureRecord{}
)

type userView struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
	Status      int    `json:"status"`
	Email       string `json:"email,omitempty"`
	GitHubId    string `json:"github_id,omitempty"`
	WeChatId    string `json:"wechat_id,omitempty"`
	CSRFToken   string `json:"csrf_token,omitempty"`
}

func ensureSessionCSRFToken(session sessions.Session) (string, error) {
	token, _ := session.Get(csrfSessionKey).(string)
	if token != "" {
		return token, nil
	}
	token = security.GenerateCSRFToken()
	session.Set(csrfSessionKey, token)
	if err := session.Save(); err != nil {
		return "", err
	}
	return token, nil
}

func buildUserView(user *model.User, csrfToken string) userView {
	if user == nil {
		return userView{}
	}
	return userView{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
		Email:       user.Email,
		GitHubId:    user.GitHubId,
		WeChatId:    user.WeChatId,
		CSRFToken:   csrfToken,
	}
}

func authenticatedSessionUser(c *gin.Context) (*model.User, bool) {
	session := sessions.Default(c)
	id, ok := session.Get("id").(int)
	if !ok || id <= 0 {
		return nil, false
	}
	user := &model.User{Id: id}
	if err := user.FillUserById(); err != nil {
		deleteControllerAuthSession(session)
		return nil, false
	}
	expectedFingerprint, _ := session.Get(passwordSessionFingerprintKey).(string)
	if !security.VerifyPasswordSessionFingerprint(user.Password, common.SessionSecret, expectedFingerprint) {
		deleteControllerAuthSession(session)
		return nil, false
	}
	if user.Status != common.UserStatusEnabled {
		deleteControllerAuthSession(session)
		return nil, false
	}
	return user, true
}

func clearControllerAuthSession(session sessions.Session) {
	deleteControllerAuthSession(session)
	_ = session.Save()
}

func deleteControllerAuthSession(session sessions.Session) {
	session.Delete("id")
	session.Delete("username")
	session.Delete("role")
	session.Delete("status")
	session.Delete(csrfSessionKey)
	session.Delete(passwordSessionFingerprintKey)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func Login(c *gin.Context) {
	if !common.PasswordLoginEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员关闭了密码登录",
			"success": false,
		})
		return
	}
	var loginRequest LoginRequest
	err := json.NewDecoder(c.Request.Body).Decode(&loginRequest)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "无效的参数",
			"success": false,
		})
		return
	}
	username := loginRequest.Username
	password := loginRequest.Password
	if username == "" || password == "" {
		c.JSON(http.StatusOK, gin.H{
			"message": "无效的参数",
			"success": false,
		})
		return
	}
	loginKey := loginFailureKeyForRequest(username, c)
	if locked, retryAfter := loginAttemptLocked(loginKey, time.Now()); locked {
		c.JSON(http.StatusOK, gin.H{
			"message":             "登录失败次数过多，请稍后再试",
			"success":             false,
			"retry_after_seconds": int(retryAfter.Seconds()),
		})
		return
	}
	user := model.User{
		Username: username,
		Password: password,
	}
	err = user.ValidateAndFill()
	if err != nil {
		recordLoginFailure(loginKey, time.Now())
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	clearLoginFailure(loginKey)
	setupLogin(&user, c)
}

func loginFailureKeyForRequest(username string, c *gin.Context) string {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return ""
	}
	source := loginFailureSource(c)
	if source == "" {
		source = "unknown"
	}
	return username + "|" + source
}

func loginFailureSource(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return "unknown"
	}
	if ip := strings.TrimSpace(c.ClientIP()); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(c.Request.RemoteAddr)
}

func loginAttemptLocked(key string, now time.Time) (bool, time.Duration) {
	if key == "" {
		return false, 0
	}
	loginFailureMutex.Lock()
	defer loginFailureMutex.Unlock()
	record, ok := loginFailures[key]
	if !ok {
		return false, 0
	}
	if !record.LockedUntil.IsZero() {
		if now.Before(record.LockedUntil) {
			return true, record.LockedUntil.Sub(now)
		}
		delete(loginFailures, key)
		return false, 0
	}
	if now.Sub(record.FirstAt) > loginFailureWindow {
		delete(loginFailures, key)
	}
	return false, 0
}

func recordLoginFailure(key string, now time.Time) {
	if key == "" {
		return
	}
	loginFailureMutex.Lock()
	defer loginFailureMutex.Unlock()
	record := loginFailures[key]
	if record.FirstAt.IsZero() || now.Sub(record.FirstAt) > loginFailureWindow {
		record = loginFailureRecord{FirstAt: now}
	}
	record.Count++
	if record.Count >= loginFailureMaxAttempts {
		record.LockedUntil = now.Add(loginFailureLockout)
	}
	loginFailures[key] = record
}

func clearLoginFailure(key string) {
	if key == "" {
		return
	}
	loginFailureMutex.Lock()
	defer loginFailureMutex.Unlock()
	delete(loginFailures, key)
}

func ResetLoginFailuresForTest() {
	loginFailureMutex.Lock()
	defer loginFailureMutex.Unlock()
	loginFailures = map[string]loginFailureRecord{}
}

// setup session & cookies and then return user info
func setLoginSession(user *model.User, c *gin.Context) (*model.User, error) {
	session := sessions.Default(c)
	csrfToken := security.GenerateCSRFToken()
	currentPasswordHash := user.Password
	if user.Id > 0 {
		if currentUser, err := model.GetUserById(user.Id, true); err == nil && currentUser != nil {
			currentPasswordHash = currentUser.Password
		}
	}
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	session.Set(csrfSessionKey, csrfToken)
	session.Set(passwordSessionFingerprintKey, security.PasswordSessionFingerprint(currentPasswordHash, common.SessionSecret))
	err := session.Save()
	if err != nil {
		return nil, err
	}
	cleanUser := &model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
	}
	cleanUser.VerificationCode = csrfToken
	cleanUser.CSRFToken = csrfToken
	return cleanUser, nil
}

func setupLogin(user *model.User, c *gin.Context) {
	cleanUser, err := setLoginSession(user, c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "无法保存会话信息，请重试",
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data":    buildUserView(cleanUser, cleanUser.VerificationCode),
	})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
	})
}

func Register(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "非法请求",
		"success": false,
	})
	return
}

func GetAllUsers(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	users, err := model.GetAllUsers(p*common.ItemsPerPage, common.ItemsPerPage)
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
		"data":    users,
	})
	return
}

func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	users, err := model.SearchUsers(keyword)
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
		"data":    users,
	})
	return
}

func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权获取同级或更高等级用户的信息",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user,
	})
	return
}

func GenerateToken(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	token, err := security.GenerateSecretToken()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.Token = security.HashSecretToken(token)

	if model.DB.Where("token = ?", user.Token).First(&model.User{}).RowsAffected != 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请重试，系统生成的 UUID 竟然重复了！",
		})
		return
	}

	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    token,
	})
	return
}

func GetSelf(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	csrfToken, err := ensureSessionCSRFToken(sessions.Default(c))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "failed to save CSRF token",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildUserView(user, csrfToken),
	})
	return
}

func UpdateUser(c *gin.Context) {
	var updatedUser model.User
	err := json.NewDecoder(c.Request.Body).Decode(&updatedUser)
	if err != nil || updatedUser.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if updatedUser.Password == "" {
		updatedUser.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := validation.Validate.Struct(&updatedUser); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + err.Error(),
		})
		return
	}
	originUser, err := model.GetUserById(updatedUser.Id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权更新同权限等级或更高权限等级的用户信息",
		})
		return
	}
	if myRole <= updatedUser.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权将其他用户权限等级提升到大于等于自己的权限等级",
		})
		return
	}
	if updatedUser.Password == "$I_LOVE_U" {
		updatedUser.Password = "" // rollback to what it should be
	}
	updatePassword := updatedUser.Password != ""
	if err := updatedUser.Update(updatePassword); err != nil {
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
	return
}

func UpdateSelf(c *gin.Context) {
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if user.Password == "" {
		user.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := validation.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "输入不合法 " + err.Error(),
		})
		return
	}

	cleanUser := model.User{
		Id:          c.GetInt("id"),
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if user.Password == "$I_LOVE_U" {
		user.Password = "" // rollback to what it should be
		cleanUser.Password = ""
	}
	updatePassword := user.Password != ""
	if err := cleanUser.Update(updatePassword); err != nil {
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
	return
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	originUser, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权删除同权限等级或更高权限等级的用户",
		})
		return
	}
	err = model.DeleteUserById(id)
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

func DeleteSelf(c *gin.Context) {
	id := c.GetInt("id")
	err := model.DeleteUserById(id)
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
	return
}

func CreateUser(c *gin.Context) {
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil || user.Username == "" || user.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	myRole := c.GetInt("role")
	if user.Role >= myRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法创建权限大于等于自己的用户",
		})
		return
	}
	// Even for admin users, we cannot fully trust them!
	cleanUser := model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if err := cleanUser.Insert(); err != nil {
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
	return
}

type ManageRequest struct {
	Username string `json:"username"`
	Action   string `json:"action"`
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	user := model.User{
		Username: req.Username,
	}
	// Fill attributes
	model.DB.Where(&user).First(&user)
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权更新同权限等级或更高权限等级的用户信息",
		})
		return
	}
	switch req.Action {
	case "disable":
		user.Status = common.UserStatusDisabled
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法禁用超级管理员用户",
			})
			return
		}
	case "enable":
		user.Status = common.UserStatusEnabled
	case "delete":
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法删除超级管理员用户",
			})
			return
		}
		if err := user.Delete(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "promote":
		if myRole != common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "普通管理员用户无法提升其他用户为管理员",
			})
			return
		}
		if user.Role >= common.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该用户已经是管理员",
			})
			return
		}
		user.Role = common.RoleAdminUser
	case "demote":
		if user.Role == common.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法降级超级管理员用户",
			})
			return
		}
		if user.Role == common.RoleCommonUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该用户已经是普通用户",
			})
			return
		}
		user.Role = common.RoleCommonUser
	}

	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	clearUser := model.User{
		Role:   user.Role,
		Status: user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    clearUser,
	})
	return
}

type EmailBindRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func EmailBindPost(c *gin.Context) {
	var req EmailBindRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	email := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.Code)
	if err := validation.Validate.Var(email, "required,email"); err != nil || code == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	id := c.GetInt("id")
	if !security.VerifyCodeWithKeyAndDelete(emailBindVerificationKey(id, email), code, security.EmailVerificationPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "verification code is invalid or expired",
		})
		return
	}
	user := model.User{
		Id: id,
	}
	if err := user.FillUserById(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if model.IsEmailAlreadyTaken(email) && !strings.EqualFold(strings.TrimSpace(user.Email), email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "email bind failed",
		})
		return
	}
	user.Email = email
	if err := user.Update(false); err != nil {
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

func EmailBind(c *gin.Context) {
	email := c.Query("email")
	code := c.Query("code")
	if !security.VerifyCodeWithKey(email, code, security.EmailVerificationPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "验证码错误或已过期",
		})
		return
	}
	id := c.GetInt("id")
	user := model.User{
		Id: id,
	}
	err := user.FillUserById()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.Email = email
	// no need to check if this email already taken, because we have used verification code to check it
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
	return
}
