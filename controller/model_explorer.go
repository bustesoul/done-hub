package controller

import (
	"context"
	"net/http"
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
