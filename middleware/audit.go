package middleware

import (
	"done-hub/common/logger"
	"done-hub/internal/audit"
	"github.com/gin-gonic/gin"
)

// AuditLog runs after token authentication and before any body decoding.
// It is independent of request/usage log settings and preserves panic recovery.
func AuditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		request, session := audit.Default.Observe(c.Request, c.GetString(logger.RequestIdKey), c.GetInt("token_id"))
		c.Request = request
		defer session.Finish()
		c.Next()
	}
}
