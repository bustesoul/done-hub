package controller

import (
	"context"
	"done-hub/internal/audit"
	"done-hub/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func GetAuditStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": audit.Default.Status()})
}

func GetAuditTargets(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	kind := c.Query("kind")
	if kind != "channels" && kind != "tokens" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "kind 必须为 channels 或 tokens"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	targets, err := model.ListAuditTargets(ctx, kind, c.Query("q"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": targets})
}
