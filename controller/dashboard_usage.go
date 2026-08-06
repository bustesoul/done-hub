package controller

import (
	"done-hub/common/config"
	"done-hub/model"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func GetUserDashboardUsage(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || (days != 7 && days != 30 && days != 90) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "days 仅支持 7、30 或 90"})
		return
	}

	location := time.Local
	if tz := os.Getenv("TZ"); tz != "" {
		if loaded, loadErr := time.LoadLocation(tz); loadErr == nil {
			location = loaded
		}
	}
	now := time.Now().In(location)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -days)

	usage, err := model.GetUserDashboardUsage(c.GetInt("id"), days, start, end, config.LogErrorEnabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法获取用量概览"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": usage})
}
