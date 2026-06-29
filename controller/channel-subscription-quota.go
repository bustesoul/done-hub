package controller

import (
	"done-hub/common/config"
	"done-hub/model"
	"done-hub/providers"
	"done-hub/providers/codex"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetChannelSubscriptionQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	channel, err := model.GetChannelById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if channel.Type != config.ChannelTypeCodex {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "仅 Codex 渠道支持订阅额度查询",
		})
		return
	}

	provider := providers.GetProvider(channel, c)
	codexProvider, ok := provider.(*codex.CodexProvider)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errors.New("provider not implemented").Error(),
		})
		return
	}

	windows, err := codexProvider.SubscriptionQuota()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"windows": windows,
	})
}
