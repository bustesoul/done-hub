package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"done-hub/common/config"
	"done-hub/model"
	"github.com/gin-gonic/gin"
)

func GetModelExplorer(protocols func(string) []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		models, err := model.GetExplorerModels(ctx)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取模型路由失败：" + err.Error()})
			return
		}
		for _, m := range models {
			for _, r := range m.Routes {
				r.Protocols = protocols(r.Profile)
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": models})
	}

}

func GetModelExplorerUsage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	models, err := model.GetExplorerModels(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取模型路由失败：" + err.Error()})
		return
	}
	result, err := model.GetExplorerHistory(ctx, models, time.Now().Truncate(time.Second), config.LogConsumeEnabled, config.LogErrorEnabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "读取近24小时统计失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func SetModelExplorerBinding(c *gin.Context) {
	var body struct {
		ChannelID int    `json:"channel_id" binding:"required,gt=0"`
		Model     string `json:"model" binding:"required,max=255"`
		Enabled   *bool  `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "模型绑定参数无效"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := model.SetModelChannelEnabled(ctx, body.ChannelID, body.Model, *body.Enabled); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "模型绑定已更新"})
}

func SetModelExplorerScheduling(c *gin.Context) {
	var body struct {
		ChannelID int    `json:"channel_id" binding:"required,gt=0"`
		Model     string `json:"model" binding:"required,max=255"`
		Priority  *int64 `json:"priority" binding:"required"`
		Weight    *uint  `json:"weight" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "模型调度参数无效"})
		return
	}
	if *body.Weight < 1 || *body.Weight > 1000000 || *body.Priority < -9007199254740991 || *body.Priority > 9007199254740991 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "优先级须为安全整数，权重须为 1 到 1000000 的整数"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := model.SetModelChannelScheduling(ctx, body.ChannelID, body.Model, *body.Priority, *body.Weight); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "更新模型调度失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "模型调度已更新"})
}

// Manual binding probes do not update channel health, credentials, switches or
// historical usage. Duration is the complete probe, not a streaming first response.
func ProbeModelExplorerBinding(c *gin.Context) {
	var body struct {
		ChannelID int    `json:"channel_id" binding:"required,gt=0"`
		Model     string `json:"model" binding:"required,max=255"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "模型测试参数无效"})
		return
	}
	if strings.Contains(body.Model, "*") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "通配绑定不能直接测试，请使用具体模型绑定"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	if _, err := model.GetModelChannelRoute(ctx, body.ChannelID, body.Model); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "模型渠道绑定不存在"})
		return
	}
	channel, err := model.GetChannelById(body.ChannelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "渠道不存在"})
		return
	}
	started := time.Now()
	apiErr, testErr := testChannelWithContext(ctx, channel, body.Model)
	latency := time.Since(started).Milliseconds()
	if apiErr != nil || testErr != nil {
		writeProviderProbeError(c, apiErr, testErr, latency, "inference")
		return
	}
	writeProviderProbeSuccess(c, latency, channel.ProtocolProfileID, nil, "")
}
