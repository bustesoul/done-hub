package controller

import (
	"net/http"

	"done-hub/providers"

	"github.com/gin-gonic/gin"
)

func GetProviderDefinitions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    providers.ProviderDefinitions(),
	})
}

func GetConnectionProfiles(c *gin.Context) {
	profiles, err := providers.ConnectionProfiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    profiles,
	})
}
