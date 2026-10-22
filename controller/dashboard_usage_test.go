package controller

import (
	"done-hub/model"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestDashboardQueryTimeBoundsAndValidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	query := func(url string) (model.DashboardQuery, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", url, nil)
		return dashboardQuery(c, now)
	}
	q, err := query("/?range=24h&timezone=UTC&granularity=hour")
	if err != nil {
		t.Fatal(err)
	}
	if q.End.Sub(q.Start) != 24*time.Hour || !q.PreviousEnd.Equal(q.Start) || q.PreviousEnd.Sub(q.PreviousStart) != 24*time.Hour {
		t.Fatalf("24h is not rolling: %+v", q)
	}
	for _, url := range []string{"/?range=today", "/?range=1", "/?timezone=bad-zone", "/?range=custom&start=x&end=3", "/?range=custom&start=0&end=1", "/?range=custom&start=-10&end=-1", "/?stream=x", "/?granularity=minute", "/?min_first=NaN", "/?status=200", "/?search=oops"} {
		if _, err = query(url); err == nil {
			t.Fatalf("accepted invalid query %s", url)
		}
	}
	q, err = query("/?range=90&granularity=hour&token=")
	if err != nil {
		t.Fatal(err)
	}
	if q.Granularity != "day" || !q.TokenSet {
		t.Fatalf("granularity/empty-name filter: %+v", q)
	}
}

func TestDashboardRequestsAndCSVStayWithinAuthenticatedUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Log{}); err != nil {
		t.Fatal(err)
	}
	old := model.DB
	model.DB = db
	defer func() { model.DB = old }()
	at := time.Now().Add(-time.Hour).Unix()
	logs := []model.Log{{UserId: 8, CreatedAt: at, Type: model.LogTypeConsume, ModelName: "=evil", TokenName: "mine", RequestTime: 1234, Quota: 10, Metadata: datatypes.NewJSONType(map[string]any{"reasoning_effort": "high"})}, {UserId: 9, CreatedAt: at, Type: model.LogTypeConsume, ModelName: "private-model", TokenName: "other", Quota: 999}}
	if err = db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 8) })
	router.GET("/requests", GetUserDashboardRequests)
	router.GET("/export", ExportUserDashboardRequests)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/requests?range=24h&user_id=9&effort=high", nil))
	var res struct {
		Success bool
		Data    model.DashboardRequests
	}
	if err = json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Data.Total != 1 || res.Data.Items[0].TokenName != "mine" {
		t.Fatalf("user isolation failed: %s", w.Body)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/export?range=24h&effort=high", nil))
	body := w.Body.String()
	if !strings.Contains(body, "'=evil") || strings.Contains(body, "private-model") || !strings.Contains(body, "1.23") {
		t.Fatalf("bad export: %s", body)
	}
}
