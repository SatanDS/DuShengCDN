package middleware

import "github.com/gin-gonic/gin"

const WebContentSecurityPolicy = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' 'unsafe-inline' https://challenges.cloudflare.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self' http: https: ws: wss:; frame-src https://challenges.cloudflare.com; worker-src 'self' blob:; upgrade-insecure-requests"

func WebSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", WebContentSecurityPolicy)
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=()")
		c.Next()
	}
}
