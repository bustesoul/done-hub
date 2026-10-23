package controller

import (
	"done-hub/model"
	"done-hub/providers"
	providersBase "done-hub/providers/base"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetModelList(c *gin.Context) {
	channel := &model.Channel{}
	err := c.ShouldBindJSON(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if channel.Keys != nil {
		channel.Key = ""
		enabled := false
		for _, key := range channel.Keys {
			if key.Enabled {
				channel.Key = key.Secret
				enabled = true
				break
			}
		}
		if !enabled {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "请先启用一个 Key"})
			return
		}
	} else if !strings.HasPrefix(strings.TrimSpace(channel.Key), "{") {
		channel.Key = strings.Split(channel.Key, "\n")[0]
	}

	if channel.Key == "" && !providers.AllowsEmptyCredential(channel.Type) {
		if channel.Id == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "key is required",
			})
			return
		}

		channel, err = model.GetChannelById(channel.Id)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	provider := providers.GetProvider(channel, providerRequestContext(c))
	if provider == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "provider not found",
		})
		return
	}

	modelProvider, ok := provider.(providersBase.ModelListInterface)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel not implemented",
		})
		return
	}

	modelList, err := modelProvider.GetModelList()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 去除重复的模型名称
	uniqueModels := removeDuplicates(modelList)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    uniqueModels,
	})
}

// 辅助函数：去除切片中的重复元素
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
