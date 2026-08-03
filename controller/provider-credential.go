package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"done-hub/common"
	"done-hub/common/utils"
	"done-hub/model"
	"done-hub/providers"
	"done-hub/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createProviderCredentialRequest struct {
	Secret   string `json:"secret" binding:"required"`
	AuthMode string `json:"auth_mode"`
}

func ListProviderCredentials(c *gin.Context) {
	channelID, ok := providerConnectionID(c)
	if !ok {
		return
	}
	credentials, err := model.ListGatewayCredentials(model.DB, channelID)
	if err != nil {
		common.APIRespondWithError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    credentials,
	})
}

func CreateProviderCredential(c *gin.Context) {
	channelID, ok := providerConnectionID(c)
	if !ok {
		return
	}
	var request createProviderCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	channel, err := model.GetChannelById(channelID)
	if err != nil {
		common.APIRespondWithError(c, http.StatusNotFound, err)
		return
	}
	authMode, err := providers.ResolveCredentialAuthMode(channel.Type, request.AuthMode)
	if err != nil {
		common.APIRespondWithError(c, http.StatusBadRequest, err)
		return
	}
	credential, err := model.CreatePendingGatewayCredential(model.DB, channelID, string(authMode), request.Secret)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		common.APIRespondWithError(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    credential,
	})
}

func TestProviderCredential(c *gin.Context) {
	channelID, credentialID, ok := providerCredentialIDs(c)
	if !ok {
		return
	}
	_, plaintext, err := model.LoadGatewayCredentialSecret(model.DB, channelID, credentialID)
	if err != nil {
		common.APIRespondWithError(c, http.StatusNotFound, err)
		return
	}
	channel, err := model.GetChannelById(channelID)
	if err != nil {
		common.APIRespondWithError(c, http.StatusNotFound, err)
		return
	}
	channel.Key = plaintext
	testModel := strings.TrimSpace(c.Query("model"))
	startedAt := time.Now()
	openaiErr, testErr := testChannel(channel, testModel)
	latency := time.Since(startedAt).Milliseconds()
	passed := openaiErr == nil && testErr == nil
	if markErr := model.MarkGatewayCredentialTestResult(model.DB, channelID, credentialID, passed); markErr != nil {
		common.APIRespondWithError(c, http.StatusConflict, markErr)
		return
	}
	if !passed {
		writeProviderProbeError(c, openaiErr, testErr, latency, "credential")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"stage":         "completed",
			"latency_ms":    latency,
			"request_id":    c.GetString("request_id"),
			"credential_id": credentialID,
		},
	})
}

func ActivateProviderCredential(c *gin.Context) {
	channelID, credentialID, ok := providerCredentialIDs(c)
	if !ok {
		return
	}
	if err := model.ActivateGatewayCredential(model.DB, channelID, credentialID); err != nil {
		common.APIRespondWithError(c, http.StatusConflict, err)
		return
	}
	model.ClearChannelTokenCache(channelID)
	model.GatewayRoutes.Load()
	if channel, loadErr := model.GetChannelById(channelID); loadErr == nil {
		channel.UpdateResponseTime(0)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"credential_id": credentialID,
			"status":        model.GatewayCredentialActive,
		},
	})
}

func RevokeProviderCredential(c *gin.Context) {
	channelID, credentialID, ok := providerCredentialIDs(c)
	if !ok {
		return
	}
	if err := model.RevokeGatewayCredential(model.DB, channelID, credentialID); err != nil {
		common.APIRespondWithError(c, http.StatusConflict, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"credential_id": credentialID,
			"status":        model.GatewayCredentialRevoked,
		},
	})
}

func providerConnectionID(c *gin.Context) (int, bool) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("无效的连接 ID"))
		return 0, false
	}
	return channelID, true
}

func providerCredentialIDs(c *gin.Context) (int, uint, bool) {
	channelID, ok := providerConnectionID(c)
	if !ok {
		return 0, 0, false
	}
	rawCredentialID, err := strconv.ParseUint(c.Param("credential_id"), 10, 64)
	if err != nil || rawCredentialID == 0 {
		common.APIRespondWithError(c, http.StatusBadRequest, errors.New("无效的凭据 ID"))
		return 0, 0, false
	}
	return channelID, uint(rawCredentialID), true
}

func writeProviderProbeError(
	c *gin.Context,
	openaiErr *types.OpenAIErrorWithStatusCode,
	testErr error,
	latency int64,
	stage string,
) {
	message := "连接探测失败"
	code := "UPSTREAM_PROBE_FAILED"
	status := http.StatusBadGateway
	if openaiErr != nil {
		message = utils.MaskSensitiveInfo(openaiErr.Error())
		status = openaiErr.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		if openaiErr.Type != "" {
			code = openaiErr.Type
		}
	} else if testErr != nil {
		message = utils.MaskSensitiveInfo(testErr.Error())
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
		"error": gin.H{
			"code":       code,
			"message":    message,
			"stage":      stage,
			"latency_ms": latency,
			"request_id": c.GetString("request_id"),
		},
	})
}
