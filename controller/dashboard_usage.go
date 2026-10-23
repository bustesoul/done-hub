package controller

import (
	"bytes"
	"done-hub/common/config"
	"done-hub/model"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"
)

func dashboardQuery(c *gin.Context, now time.Time) (model.DashboardQuery, error) {
	q := model.DashboardQuery{}
	zone := c.DefaultQuery("timezone", "Asia/Shanghai")
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return q, fmt.Errorf("无效时区")
	}
	now = now.In(loc).Truncate(time.Second)
	period := c.Query("range")
	if period == "" {
		period = c.DefaultQuery("days", "30")
	}
	q.End = now
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	switch period {
	case "24h":
		q.Start = now.Add(-24 * time.Hour)
		q.PreviousStart = q.Start.Add(-24 * time.Hour)
		q.PreviousEnd = q.Start
	case "7", "30", "90":
		days, _ := strconv.Atoi(period)
		q.Start = day.AddDate(0, 0, 1-days)
		q.PreviousStart = q.Start.AddDate(0, 0, -days)
		q.PreviousEnd = q.End.AddDate(0, 0, -days)
	case "custom":
		a, e1 := strconv.ParseInt(c.Query("start"), 10, 64)
		b, e2 := strconv.ParseInt(c.Query("end"), 10, 64)
		if e1 != nil || e2 != nil {
			return q, fmt.Errorf("自定义时间需要 start/end Unix 秒")
		}
		q.Start = time.Unix(a, 0).In(loc)
		q.End = time.Unix(b, 0).In(loc)
		q.PreviousEnd = q.Start
		q.PreviousStart = q.Start.Add(-q.End.Sub(q.Start))
	default:
		return q, fmt.Errorf("range 仅支持 24h、7、30、90、custom")
	}
	if !q.End.After(q.Start) || q.End.After(now) || q.Start.Before(now.Add(-MaxLogQueryDays*24*time.Hour)) {
		return q, fmt.Errorf("时间范围无效或结束时间晚于当前时间")
	}
	if err := validateLogTimeRange(q.Start.Unix(), q.End.Unix()); err != nil {
		return q, err
	}
	q.Compare = c.DefaultQuery("compare", "true") != "false" && !q.PreviousStart.Before(now.Add(-MaxLogQueryDays*24*time.Hour))
	q.Granularity = c.DefaultQuery("granularity", "auto")
	switch q.Granularity {
	case "auto":
		q.Granularity = "day"
		if q.End.Sub(q.Start) <= 48*time.Hour {
			q.Granularity = "hour"
		} else if q.End.Sub(q.Start) > 45*24*time.Hour {
			q.Granularity = "week"
		}
	case "hour", "day", "week":
	default:
		return q, fmt.Errorf("无效时间粒度")
	}
	if q.Granularity == "hour" && q.End.Sub(q.Start) > 168*time.Hour {
		q.Granularity = "day"
	}
	q.ModelSet = c.Request.URL.Query().Has("model")
	q.TokenSet = c.Request.URL.Query().Has("token")
	q.Model = c.Query("model")
	q.Token = c.Query("token")
	q.Stream = c.Query("stream")
	q.Effort = c.Query("effort")
	q.Status = c.Query("status")
	q.Search = c.Query("search")
	if q.Stream != "" && q.Stream != "stream" && q.Stream != "nonstream" {
		return q, fmt.Errorf("无效请求类型")
	}
	if len(q.Model) > 200 || len(q.Token) > 200 || len(q.Effort) > 64 {
		return q, fmt.Errorf("筛选字段过长")
	}
	if q.Status != "" && q.Status != "success" && q.Status != "error" {
		status, e := strconv.Atoi(q.Status)
		if e != nil || status < 400 || status > 599 {
			return q, fmt.Errorf("无效请求状态")
		}
	}
	if q.Search != "" {
		id, e := strconv.Atoi(q.Search)
		if e != nil || id < 1 {
			return q, fmt.Errorf("请输入有效日志 ID")
		}
	}
	if c.Query("min_first") != "" {
		q.MinFirst, err = strconv.ParseFloat(c.Query("min_first"), 64)
		if err != nil || math.IsNaN(q.MinFirst) || math.IsInf(q.MinFirst, 0) || q.MinFirst < 0 || q.MinFirst > 86400000 {
			return q, fmt.Errorf("无效首响应阈值")
		}
	}
	return q, nil
}
func GetUserDashboardUsage(c *gin.Context) {
	q, err := dashboardQuery(c, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	usage, err := model.GetDashboardAnalysis(c.Request.Context(), c.GetInt("id"), q, config.LogErrorEnabled, config.LogConsumeEnabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无法获取用量概览：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": usage})
}
func GetUserDashboardRequests(c *gin.Context) {
	q, err := dashboardQuery(c, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	page, e1 := strconv.Atoi(c.DefaultQuery("page", "0"))
	size, e2 := strconv.Atoi(c.DefaultQuery("size", "10"))
	if e1 != nil || e2 != nil || page < 0 || page > model.DashboardMaxRecords || size < 1 || size > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效分页参数"})
		return
	}
	result, err := model.GetDashboardRequests(c.Request.Context(), c.GetInt("id"), q, page, size, nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
func ExportUserDashboardRequests(c *gin.Context) {
	q, err := dashboardQuery(c, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	// Buffer until the scan succeeds, so a limit/cancellation never downloads a
	// silently truncated CSV. Reuse the existing user log billing CSV formatter.
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"日志ID", "时间 (" + q.Start.Location().String() + ")", "分组", "Key 名称", "类型", "模型", "请求总耗时(秒)", "输入Token", "输出Token", "额度USD", "详情", "首响应毫秒", "流式", "思考档位", "已记录HTTP状态码", "错误信息"})
	_, err = model.GetDashboardRequests(c.Request.Context(), c.GetInt("id"), q, 0, 100, func(l *model.Log) error {
		row := formatLogToCSVRow(l, false)[:11] // This export has its own timing/status columns.
		row[0] = time.Unix(l.CreatedAt, 0).In(q.Start.Location()).Format("2006-01-02 15:04:05")
		row = append(row[:9], row[10:]...) // Source IP is intentionally not queried by the dashboard.
		row = append([]string{strconv.Itoa(l.Id)}, row...)
		m := l.Metadata.Data()
		first := ""
		if v := model.LogFirstResponseMs(l); v > 0 {
			first = strconv.FormatFloat(v, 'f', 3, 64)
		}
		effort, _ := m["reasoning_effort"].(string)
		status, message := "", ""
		if value, ok := m["status_code"]; ok {
			status = fmt.Sprint(value)
		}
		if value, ok := m["error_message"].(string); ok {
			message = value
		}
		row = append(row, first, strconv.FormatBool(l.IsStream), effort, status, message)
		// User-controlled names must remain data when opened by a spreadsheet.
		for i, v := range row {
			if len(v) > 0 && (v[0] == '=' || v[0] == '+' || v[0] == '-' || v[0] == '@' || v[0] == '\t' || v[0] == '\r') {
				row[i] = "'" + v
			}
		}
		return w.Write(row)
	})
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=dashboard-requests.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", b.Bytes())
}
