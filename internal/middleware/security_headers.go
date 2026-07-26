package middleware

import "github.com/gin-gonic/gin"

const siteContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline' https: http:; script-src-attr 'none'; style-src 'self' 'unsafe-inline' https: http:; img-src 'self' data: blob: https: http:; media-src 'self' blob: https: http:; font-src 'self' data: https: http:; connect-src 'self' https: http: ws: wss:; frame-src 'self' https: http:; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", siteContentSecurityPolicy)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
