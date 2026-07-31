package relay

import (
	"done-hub/common/logger"
	"done-hub/internal/gateway/requeststate"
	gatewaystream "done-hub/internal/gateway/stream"
	"net/http"

	"github.com/gin-gonic/gin"
)

func gatewayRequestState(c *gin.Context) *requeststate.State {
	if c == nil {
		return nil
	}
	if c.Request == nil {
		c.Request = &http.Request{}
	}
	request, state := requeststate.Ensure(c.Request)
	c.Request = request
	return state
}

func beginGatewayAttempt(c *gin.Context) *gatewaystream.Session {
	session := gatewaystream.NewSession()
	setGatewayStreamSession(c, session)
	return session
}

func setGatewayStreamSession(c *gin.Context, session *gatewaystream.Session) {
	gatewayRequestState(c).SetStreamSession(session)
}

func gatewayStreamSession(c *gin.Context) *gatewaystream.Session {
	return gatewayRequestState(c).StreamSession()
}

func recordGatewayOutput(c *gin.Context, size int) {
	session := gatewayStreamSession(c)
	if session == nil {
		return
	}
	if err := session.RecordData(size); err != nil {
		logger.LogError(c.Request.Context(), "gateway stream state: "+err.Error())
	}
}

func terminateGatewayOutput(c *gin.Context, terminalErr error) {
	session := gatewayStreamSession(c)
	if session == nil {
		return
	}
	if err := session.Terminate(terminalErr); err != nil {
		logger.LogError(c.Request.Context(), "gateway stream state: "+err.Error())
	}
}
