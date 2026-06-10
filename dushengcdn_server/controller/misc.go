package controller

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"dushengcdn/service"
	"dushengcdn/utils/mail"
	"dushengcdn/utils/security"
	"dushengcdn/utils/validation"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

var sendEmailVerificationCodeAsyncFunc = sendEmailVerificationCodeAsync

// GetStatus godoc
// @Summary Get server status
// @Tags Public
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/status [get]
func GetStatus(c *gin.Context) {
	authSources, err := service.PublicAuthSources("/api")
	if err != nil {
		authSources = []service.PublicAuthSource{}
	}
	data := gin.H{
		"email_verification":        common.EmailVerificationEnabled,
		"github_oauth":              common.GitHubOAuthEnabled,
		"github_client_id":          common.GitHubClientId,
		"system_name":               common.SystemName,
		"home_page_link":            common.HomePageLink,
		"footer_html":               common.Footer,
		"wechat_qrcode":             common.WeChatAccountQRCodeImageURL,
		"wechat_login":              common.WeChatAuthEnabled,
		"turnstile_check":           common.TurnstileCheckEnabled,
		"turnstile_site_key":        common.TurnstileSiteKey,
		"register_enabled":          common.RegisterEnabled,
		"password_register_enabled": common.PasswordRegisterEnabled,
		"auth_sources":              authSources,
	}
	if common.PublicStatusRuntimeMetadataEnabled {
		data["version"] = common.Version
		data["start_time"] = common.StartTime
		data["server_address"] = common.ServerAddress
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func GetNotice(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.GetOptionValue("Notice"),
	})
}

func GetAbout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.GetOptionValue("About"),
	})
}

func SendEmailVerification(c *gin.Context) {
	var req emailVerificationRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := validation.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid email",
		})
		return
	}
	if !publicEmailVerificationEnabled() {
		respondEmailActionAccepted(c)
		return
	}
	if model.IsEmailAlreadyTaken(email) {
		respondEmailActionAccepted(c)
		return
	}
	sendEmailVerificationCodeAsyncFunc(email, email, security.EmailVerificationPurpose, "public email verification send failed")
	respondEmailActionAccepted(c)
}

func publicEmailVerificationEnabled() bool {
	return common.RegisterEnabled && common.PasswordRegisterEnabled && common.EmailVerificationEnabled
}

type emailVerificationRequest struct {
	Email string `json:"email"`
}

func SendEmailBindVerification(c *gin.Context) {
	var req emailVerificationRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := validation.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid email",
		})
		return
	}
	id := c.GetInt("id")
	user := model.User{Id: id}
	if err := user.FillUserById(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if model.IsEmailAlreadyTaken(email) && !strings.EqualFold(strings.TrimSpace(user.Email), email) {
		respondEmailActionAccepted(c)
		return
	}
	sendEmailVerificationCodeAsyncFunc(email, emailBindVerificationKey(id, email), security.EmailVerificationPurpose, "email bind verification send failed")
	respondEmailActionAccepted(c)
}

func sendEmailVerificationCodeAsync(email string, key string, purpose string, logMessage string) {
	email = strings.TrimSpace(email)
	key = strings.TrimSpace(key)
	purpose = strings.TrimSpace(purpose)
	go func() {
		if err := sendEmailVerificationCode(email, key, purpose); err != nil {
			slog.Warn(logMessage, "error", err)
		}
	}()
}

func sendEmailVerificationCode(email string, key string, purpose string) error {
	code := security.GenerateVerificationCode(6)
	subject := fmt.Sprintf("%s email verification", common.SystemName)
	content := fmt.Sprintf("<p>Hello, you are verifying an email address for %s.</p>"+
		"<p>Your verification code is: <strong>%s</strong></p>"+
		"<p>The code is valid for %d minutes. Ignore this email if you did not request it.</p>", common.SystemName, code, security.VerificationValidMinutes)
	if err := mail.SendEmail(subject, email, content); err != nil {
		return err
	}
	security.RegisterVerificationCodeWithKey(key, code, purpose)
	return nil
}

func emailBindVerificationKey(userID int, email string) string {
	return fmt.Sprintf("%d:%s", userID, strings.ToLower(strings.TrimSpace(email)))
}

func passwordResetVerificationKey(userID int, email string) string {
	return fmt.Sprintf("password-reset:%d:%s", userID, strings.ToLower(strings.TrimSpace(email)))
}

func SendPasswordResetEmail(c *gin.Context) {
	var req emailVerificationRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := validation.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid email",
		})
		return
	}
	if !model.IsEmailAlreadyTaken(email) {
		respondPasswordResetEmailAccepted(c)
		return
	}
	user, err := model.GetSingleUserByEmail(email)
	if err != nil {
		respondPasswordResetEmailAccepted(c)
		return
	}
	sendPasswordResetEmailAsync(user.Id, email)
	respondPasswordResetEmailAccepted(c)
}

func sendPasswordResetEmailAsync(userID int, email string) {
	email = strings.TrimSpace(email)
	go func() {
		if err := sendPasswordResetEmail(userID, email); err != nil {
			slog.Warn("password reset email send failed", "error", err)
		}
	}()
}

func sendPasswordResetEmail(userID int, email string) error {
	code := security.GenerateVerificationCode(0)
	link := fmt.Sprintf("%s/user/reset#email=%s&token=%s", strings.TrimRight(common.ServerAddress, "/"), url.QueryEscape(email), url.QueryEscape(code))
	subject := fmt.Sprintf("%s password reset", common.SystemName)
	content := fmt.Sprintf("<p>Hello, you requested a password reset for %s.</p>"+
		"<p>Click <a href='%s'>here</a> to reset your password.</p>"+
		"<p>The reset link is valid for %d minutes. Ignore this email if you did not request it.</p>", common.SystemName, link, security.VerificationValidMinutes)
	if err := mail.SendEmail(subject, email, content); err != nil {
		return err
	}
	security.RegisterVerificationCodeWithKey(passwordResetVerificationKey(userID, email), code, security.PasswordResetPurpose)
	return nil
}

func respondEmailActionAccepted(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func respondPasswordResetEmailAccepted(c *gin.Context) {
	respondEmailActionAccepted(c)
}

type PasswordResetRequest struct {
	Email       string `json:"email"`
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	req.Email = strings.TrimSpace(req.Email)
	req.Token = strings.TrimSpace(req.Token)
	if err != nil || req.Email == "" || req.Token == "" || req.NewPassword == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request",
		})
		return
	}
	if err := validation.Validate.Var(req.Email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid email",
		})
		return
	}
	if err := validation.Validate.Var(req.NewPassword, "required,min=8,max=20"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "password length must be 8-20 characters",
		})
		return
	}
	user, err := model.GetSingleUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "password reset link is invalid or expired",
		})
		return
	}
	if !security.VerifyCodeWithKeyAndDelete(passwordResetVerificationKey(user.Id, req.Email), req.Token, security.PasswordResetPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "password reset link is invalid or expired",
		})
		return
	}
	err = model.ResetUserPasswordByID(user.Id, req.NewPassword)
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
