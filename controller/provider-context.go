package controller

import (
	"done-hub/internal/gateway/requeststate"
	providersBase "done-hub/providers/base"

	"github.com/gin-gonic/gin"
)

func providerRequestContext(c *gin.Context) *providersBase.RequestContext {
	request, state := requeststate.Ensure(c.Request)
	c.Request = request
	state.SetParam("version", c.Param("version"))
	return providersBase.NewRequestContext(request, state)
}
