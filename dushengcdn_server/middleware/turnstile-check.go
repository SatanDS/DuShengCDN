package middleware

import (
	"context"
	"dushengcdn/common"
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyTimeout = 5 * time.Second

var turnstileVerifyEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
var turnstileHTTPClient = &http.Client{Timeout: turnstileVerifyTimeout}

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			session := sessions.Default(c)
			turnstileChecked := session.Get("turnstile")
			if turnstileChecked != nil {
				c.Next()
				return
			}
			response := c.Query("turnstile")
			if response == "" {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}
			verifyCtx, cancel := context.WithTimeout(c.Request.Context(), turnstileVerifyTimeout)
			defer cancel()
			form := url.Values{
				"secret":   {common.TurnstileSecretKey},
				"response": {response},
				"remoteip": {c.ClientIP()},
			}
			req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, turnstileVerifyEndpoint, strings.NewReader(form.Encode()))
			if err != nil {
				slog.Error("build turnstile verification request failed", "error", err)
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rawRes, err := turnstileHTTPClient.Do(req)
			if err != nil {
				slog.Error("turnstile verification request failed", "error", err)
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验服务不可用，请稍后重试",
				})
				c.Abort()
				return
			}
			defer rawRes.Body.Close()
			if rawRes.StatusCode < 200 || rawRes.StatusCode >= 300 {
				raw, _ := io.ReadAll(io.LimitReader(rawRes.Body, 1024))
				slog.Error("turnstile verification returned unexpected status", "status", rawRes.Status, "body", strings.TrimSpace(string(raw)))
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验服务异常，请稍后重试",
				})
				c.Abort()
				return
			}
			var res turnstileCheckResponse
			err = json.NewDecoder(rawRes.Body).Decode(&res)
			if err != nil {
				slog.Error("decode turnstile verification response failed", "error", err)
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验响应无效，请稍后重试",
				})
				c.Abort()
				return
			}
			if !res.Success {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}
			session.Set("turnstile", true)
			err = session.Save()
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"message": "无法保存会话信息，请重试",
					"success": false,
				})
				return
			}
		}
		c.Next()
	}
}
