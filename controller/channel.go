package controller

import (
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/utils"
	"done-hub/model"
	"done-hub/providers"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetChannelsList(c *gin.Context) {
	var params model.SearchChannelsParams
	if err := c.ShouldBindQuery(&params); err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	channels, err := model.GetChannelsList(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channels,
	})
}

func GetChannel(c *gin.Context) {
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
	if channel.ProtocolProfileID == "" {
		if profileID, profileErr := providers.DefaultProtocolProfile(channel.Type); profileErr == nil {
			channel.ProtocolProfileID = string(profileID)
		}
	}
	channel.Key = ""
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
}

func AddChannel(c *gin.Context) {
	channel := model.Channel{}
	err := c.ShouldBindJSON(&channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if err = model.CheckTagTypeConsistency(channel.Tag, channel.Type, 0); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if err = providers.ValidateChannelConfig(&channel, true); err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	verified := false
	if c.GetBool("require_provider_validation") && !channel.SaveUnverified {
		if err = verifyProviderValidationToken(&channel); err != nil {
			common.APIRespondWithError(c, http.StatusConflict, err)
			return
		}
		verified = true
	}
	if channel.SaveUnverified {
		channel.Status = config.ChannelStatusManuallyDisabled
		channel.TestTime = 0
		channel.ResponseTime = 0
	} else if verified {
		channel.TestTime = utils.GetTimestamp()
	}
	channel.CreatedTime = utils.GetTimestamp()
	keys := strings.Split(channel.Key, "\n")
	allowEmptyCredential := providers.AllowsEmptyCredential(channel.Type)

	baseUrls := []string{}
	if channel.BaseURL != nil && *channel.BaseURL != "" {
		baseUrls = strings.Split(*channel.BaseURL, "\n")
	}
	channels := make([]model.Channel, 0, len(keys))
	for index, key := range keys {
		if key == "" && !allowEmptyCredential {
			continue
		}
		if key == "" && index > 0 {
			continue
		}
		localChannel := channel
		localChannel.Key = key
		if index > 0 {
			localChannel.Name = localChannel.Name + "_" + strconv.Itoa(index+1)
		}

		if len(baseUrls) > index && baseUrls[index] != "" {
			localChannel.BaseURL = &baseUrls[index]
		} else if len(baseUrls) > 0 {
			localChannel.BaseURL = &baseUrls[0]
		}

		channels = append(channels, localChannel)
	}
	err = model.BatchInsertChannels(channels)
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
	})
}

func AddProviderConnection(c *gin.Context) {
	c.Set("require_provider_validation", true)
	AddChannel(c)
}

func DeleteChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("无效的连接 ID"))
		return
	}
	channel := model.Channel{Id: id}
	err = channel.Delete()
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
	})
}

func CloneProviderConnection(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	source, err := model.GetChannelById(id)
	if err != nil {
		common.APIRespondWithError(c, http.StatusNotFound, err)
		return
	}
	source.Id = 0
	source.Name += "_copy"
	source.Key = ""
	source.CredentialConfigured = false
	source.Status = config.ChannelStatusManuallyDisabled
	source.Tag = ""
	source.CreatedTime = utils.GetTimestamp()
	source.TestTime = 0
	source.ResponseTime = 0
	source.Balance = 0
	source.BalanceUpdatedTime = 0
	source.UsedQuota = 0
	source.DeletedAt = gorm.DeletedAt{}
	if err := source.Insert(); err != nil {
		common.APIRespondWithError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "连接已复制；副本未携带凭据且保持停用",
		"data":    source,
	})
}

type providerConnectionStatusRequest struct {
	Status int `json:"status"`
}

func SetProviderConnectionStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	var request providerConnectionStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	if request.Status != config.ChannelStatusEnabled && request.Status != config.ChannelStatusManuallyDisabled {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("只允许启用或手动停用连接"))
		return
	}
	if request.Status == config.ChannelStatusEnabled {
		var endpoint model.GatewayEndpoint
		if err := model.DB.Where("channel_id = ?", id).First(&endpoint).Error; err != nil {
			common.APIRespondWithError(c, http.StatusNotFound, err)
			return
		}
		if endpoint.ActiveCredentialID == 0 {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("连接没有已激活凭据，不能启用"))
			return
		}
		var credential model.GatewayCredential
		if err := model.DB.Where("id = ? AND channel_id = ?", endpoint.ActiveCredentialID, id).
			First(&credential).Error; err != nil {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("连接的活动凭据不存在，不能启用"))
			return
		}
		if credential.AuthMode == "none" && !providers.AllowsEmptyCredential(endpoint.ProviderID) {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("该 Provider 不支持无凭据连接"))
			return
		}
		if credential.TestedAt == nil {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("连接的活动凭据尚未通过测试，不能启用"))
			return
		}
		var routeCount int64
		if err := model.DB.Model(&model.GatewayModelRoute{}).
			Where("channel_id = ?", id).
			Count(&routeCount).Error; err != nil {
			common.APIRespondWithError(c, http.StatusInternalServerError, err)
			return
		}
		if routeCount == 0 {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("连接没有可用模型路由，不能启用"))
			return
		}
		var health model.GatewayHealthState
		if err := model.DB.Where("channel_id = ?", id).First(&health).Error; err != nil || health.TestTime == 0 {
			common.APIRespondWithError(c, http.StatusConflict, errors.New("连接尚未通过探测，不能启用"))
			return
		}
	}
	if err := model.UpdateChannelStatusById(id, request.Status); err != nil {
		common.APIRespondWithError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":     id,
			"status": request.Status,
		},
	})
}

type providerConnectionSchedulingRequest struct {
	Priority        *int64   `json:"priority"`
	Weight          *uint    `json:"weight"`
	CostRatio       *float64 `json:"cost_ratio"`
	AffinityEnabled *bool    `json:"affinity_enabled"`
}

func UpdateProviderConnectionScheduling(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	var request providerConnectionSchedulingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	if request.Priority == nil && request.Weight == nil && request.CostRatio == nil && request.AffinityEnabled == nil {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("至少提供一个调度字段"))
		return
	}
	if request.Weight != nil && *request.Weight < 1 {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("权重必须大于等于 1"))
		return
	}
	if request.CostRatio != nil && *request.CostRatio < 0 {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("成本倍率不能小于 0"))
		return
	}
	if request.AffinityEnabled != nil {
		channel, loadErr := model.GetChannelById(id)
		if loadErr != nil {
			common.APIRespondWithError(c, http.StatusNotFound, loadErr)
			return
		}
		channel.AffinityEnabled = *request.AffinityEnabled
		if validateErr := providers.ValidateChannelConfig(channel, false); validateErr != nil {
			common.APIRespondWithError(c, http.StatusBadRequest, validateErr)
			return
		}
	}
	updates := make(map[string]any, 4)
	if request.Priority != nil {
		updates["priority"] = *request.Priority
	}
	if request.Weight != nil {
		updates["weight"] = *request.Weight
	}
	if request.CostRatio != nil {
		updates["cost_ratio"] = *request.CostRatio
	}
	if request.AffinityEnabled != nil {
		updates["affinity_enabled"] = *request.AffinityEnabled
	}
	if err := model.UpdateGatewayConnectionScheduling(id, updates); err != nil {
		common.APIRespondWithError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updates,
	})
}

func DeleteChannelTag(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteChannelTag(id)
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
	})
}

func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
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
		"data":    rows,
	})
}

func UpdateChannel(c *gin.Context) {
	channel := model.Channel{}
	err := c.ShouldBindJSON(&channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// 校验类型一致性：标签变化（加入/切换标签）时校验；标签未变但「在组内单独编辑改了类型」时
	// 也要校验，防止单渠道编辑把同一标签分组变成混合类型。仅编辑已有成员且类型未变则放行，
	// 避免阻断历史遗留的混合分组成员的正常编辑。
	oldChannel, getErr := model.GetChannelById(channel.Id)
	if getErr != nil {
		common.APIRespondWithError(c, http.StatusNotFound, getErr)
		return
	}
	if oldChannel.Tag != channel.Tag || (channel.Tag != "" && oldChannel.Type != channel.Type) {
		if err = model.CheckTagTypeConsistency(channel.Tag, channel.Type, channel.Id); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	if channel.Key != "" && channel.Key != oldChannel.Key {
		common.APIRespondWithError(c, http.StatusConflict, errors.New("普通编辑不能替换凭据，请使用“替换凭据”流程"))
		return
	}
	channel.Key = oldChannel.Key
	channelToValidate := channel
	if err = providers.ValidateChannelConfig(&channelToValidate, false); err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	channel = channelToValidate
	configChanged := providerConnectionProbeConfigChanged(oldChannel, &channel)
	if configChanged {
		if channel.SaveUnverified {
			channel.Status = config.ChannelStatusManuallyDisabled
			channel.TestTime = 0
			channel.ResponseTime = 0
		} else {
			if err = verifyProviderValidationToken(&channel); err != nil {
				common.APIRespondWithError(c, http.StatusConflict, err)
				return
			}
			channel.TestTime = utils.GetTimestamp()
			channel.ResponseTime = 0
		}
	} else {
		channel.TestTime = oldChannel.TestTime
		channel.ResponseTime = oldChannel.ResponseTime
	}
	if channel.Models == "" {
		err = channel.Update(false)
	} else {
		err = channel.Update(true)
	}
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
		"data":    channel,
	})
}

func providerConnectionProbeConfigChanged(before, after *model.Channel) bool {
	if before == nil || after == nil {
		return true
	}
	return before.Type != after.Type ||
		before.ProtocolProfileID != after.ProtocolProfileID ||
		before.GetBaseURL() != after.GetBaseURL() ||
		before.GetProxy() != after.GetProxy() ||
		before.Other != after.Other ||
		before.TestModel != after.TestModel
}

func BatchUpdateChannelsAzureApi(c *gin.Context) {
	var params model.BatchChannelsParams
	err := c.ShouldBindJSON(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	if params.Ids == nil || len(params.Ids) == 0 {
		common.APIRespondWithError(c, http.StatusOK, errors.New("ids不能为空"))
		return
	}
	var count int64
	count, err = model.BatchUpdateChannelsAzureApi(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":    count,
		"success": true,
		"message": "更新成功",
	})
}

func BatchDelModelChannels(c *gin.Context) {
	var params model.BatchChannelsParams
	err := c.ShouldBindJSON(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	if params.Ids == nil || len(params.Ids) == 0 {
		common.APIRespondWithError(c, http.StatusOK, errors.New("ids不能为空"))
		return
	}

	var count int64
	count, err = model.BatchDelModelChannels(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":    count,
		"success": true,
		"message": "更新成功",
	})
}

func BatchAddUserGroupToChannels(c *gin.Context) {
	var params model.BatchChannelsParams
	err := c.ShouldBindJSON(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	if params.Ids == nil || len(params.Ids) == 0 {
		common.APIRespondWithError(c, http.StatusOK, errors.New("ids不能为空"))
		return
	}

	if params.Value == "" {
		common.APIRespondWithError(c, http.StatusOK, errors.New("用户分组不能为空"))
		return
	}

	var count int64
	count, err = model.BatchAddUserGroupToChannels(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":    count,
		"success": true,
		"message": "批量添加用户分组成功",
	})
}

func BatchAddModelToChannels(c *gin.Context) {
	var params model.BatchChannelsParams
	err := c.ShouldBindJSON(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	if params.Ids == nil || len(params.Ids) == 0 {
		common.APIRespondWithError(c, http.StatusOK, errors.New("ids不能为空"))
		return
	}

	if params.Value == "" {
		common.APIRespondWithError(c, http.StatusOK, errors.New("模型不能为空"))
		return
	}

	var count int64
	count, err = model.BatchAddModelToChannels(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":    count,
		"success": true,
		"message": "批量添加模型成功",
	})
}

func BatchDeleteChannel(c *gin.Context) {
	var params model.BatchChannelsParams
	err := c.ShouldBindJSON(&params)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}

	if params.Ids == nil || len(params.Ids) == 0 {
		common.APIRespondWithError(c, http.StatusOK, errors.New("ids不能为空"))
		return
	}

	count, err := model.BatchDeleteChannel(params.Ids)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}
