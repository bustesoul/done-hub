package middleware

import (
	"net/http"
	"strings"

	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/requeststate"
	"done-hub/model"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// RequestLog runs after authentication. Inference requests are tracked; model lists,
// OAuth, task polling and raw resource administration are not inference logs.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		protocol := requester.ProtocolForPath(path)
		track := c.Request.Method == http.MethodPost && (protocol != "" || strings.HasPrefix(path, "/gemini/") || strings.Contains(path, "/submit/") || strings.HasSuffix(path, "/rerank") || strings.HasPrefix(path, "/kling/v1/") || strings.HasPrefix(path, "/recraftAI/v1/") || strings.HasSuffix(path, "/mj/insight-face/swap"))
		track = track || (strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/v1/models"))
		track = track || (c.Request.Method == http.MethodDelete && strings.HasPrefix(path, "/v1/models/"))
		if !track {
			c.Next()
			return
		}
		c.Request, _ = requeststate.Ensure(c.Request)
		ctx, session := model.BeginRequestLog(c.Request.Context(), model.Log{
			RequestId: c.GetString(logger.RequestIdKey), UserId: c.GetInt("id"), Username: c.GetString("username"),
			TokenName: c.GetString("token_name"), SourceIp: c.ClientIP(), InboundProtocol: protocol,
			Metadata: datatypes.NewJSONType(map[string]any{"group_name": c.GetString("token_group")}),
		}, c.GetTime("requestStartTime"))
		c.Request = c.Request.WithContext(ctx)
		if session != nil {
			requeststate.From(ctx).SetUpstreamObserver(func(protocol string) { session.ObserveUpstream(ctx, protocol) })
		}
		// Re-panic so the existing protocol-specific recovery still owns the error response.
		defer func() {
			if recovered := recover(); recovered != nil {
				session.Finish(ctx, http.StatusInternalServerError, true)
				panic(recovered)
			}
			session.Finish(ctx, c.Writer.Status(), false)
		}()
		c.Next()
	}
}
