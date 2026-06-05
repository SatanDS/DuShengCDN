package middleware

import (
	"dushengcdn/common"
	"dushengcdn/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
)

func authHelper(c *gin.Context, minRole int) {
	session := sessions.Default(c)
	var user *model.User
	authByToken := false

	if session.Get("username") != nil {
		id, ok := session.Get("id").(int)
		if !ok || id <= 0 {
			clearAuthSession(session)
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "登录状态已失效，请重新登录",
			})
			c.Abort()
			return
		}
		currentUser := &model.User{Id: id}
		if err := currentUser.FillUserById(); err != nil {
			clearAuthSession(session)
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "登录状态已失效，请重新登录",
			})
			c.Abort()
			return
		}
		user = currentUser
	} else {
		// Check token
		token := c.Request.Header.Get("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录或 token 无效",
			})
			c.Abort()
			return
		}
		user = model.ValidateUserToken(token)
		if user == nil || user.Username == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，token 无效",
			})
			c.Abort()
			return
		}
		authByToken = true
	}
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	if user.Role < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，权限不足",
		})
		c.Abort()
		return
	}
	c.Set("username", user.Username)
	c.Set("role", user.Role)
	c.Set("status", user.Status)
	c.Set("id", user.Id)
	c.Set("authByToken", authByToken)
	c.Next()
}

func clearAuthSession(session sessions.Session) {
	session.Delete("id")
	session.Delete("username")
	session.Delete("role")
	session.Delete("status")
	_ = session.Save()
}

func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

// NoTokenAuth You should always use this after normal auth middlewares.
func NoTokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authByToken := c.GetBool("authByToken")
		if authByToken {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "本接口不支持使用 token 进行验证",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// TokenOnlyAuth You should always use this after normal auth middlewares.
func TokenOnlyAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authByToken := c.GetBool("authByToken")
		if !authByToken {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "本接口仅支持使用 token 进行验证",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
