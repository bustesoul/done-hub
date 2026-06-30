package controller

import (
	"done-hub/common/config"
	"done-hub/model"
	"done-hub/providers"
	"done-hub/providers/claudecode"
	"done-hub/providers/codex"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetChannelSubscriptionQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	channel, err := model.GetChannelById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	if channel.Type != config.ChannelTypeCodex && channel.Type != config.ChannelTypeClaudeCode {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "仅 Codex / Claude Code 渠道支持订阅额度查询",
		})
		return
	}

	provider := providers.GetProvider(channel, c)

	var windows interface{}

	switch channel.Type {
	case config.ChannelTypeCodex:
		p, ok := provider.(*codex.CodexProvider)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "provider not implemented"})
			return
		}
		windows, err = p.SubscriptionQuota()

	case config.ChannelTypeClaudeCode:
		p, ok := provider.(*claudecode.ClaudeCodeProvider)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "provider not implemented"})
			return
		}
		windows, err = p.SubscriptionQuota()
	}

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"windows": windows,
	})
}
