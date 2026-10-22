package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/requeststate"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func requestLogTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err = db.AutoMigrate(&model.Log{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.User{Id: 7, Username: "tester"}).Error; err != nil {
		t.Fatal(err)
	}
	oldDB, oldLogger, oldClient := model.DB, logger.Logger, requester.HTTPClient
	oldConsume, oldError, oldBatch, oldRedis := config.LogConsumeEnabled, config.LogErrorEnabled, config.BatchUpdateEnabled, config.RedisEnabled
	model.DB, logger.Logger, requester.HTTPClient = db, zap.NewNop(), http.DefaultClient
	config.LogConsumeEnabled, config.LogErrorEnabled, config.BatchUpdateEnabled, config.RedisEnabled = true, true, true, false
	t.Cleanup(func() {
		model.DB, logger.Logger, requester.HTTPClient = oldDB, oldLogger, oldClient
		config.LogConsumeEnabled, config.LogErrorEnabled, config.BatchUpdateEnabled, config.RedisEnabled = oldConsume, oldError, oldBatch, oldRedis
		sqlDB.Close()
	})
}

func logTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestId(), func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("username", "tester")
		c.Set("token_name", "test-token")
		c.Next()
	}, RequestLog())
	engine.POST("/v1/chat/completions", handler)
	return engine
}

func TestRequestLogVisibleBeforeUpstreamCompletes(t *testing.T) {
	requestLogTestDB(t)
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[stream], func(t *testing.T) {
			model.DB.Where("1=1").Delete(&model.Log{})
			entered, release := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"input":"hello"}` {
					t.Errorf("wire body changed: %s", body)
				}
				close(entered)
				<-release
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: OK\n\n")
				} else {
					io.WriteString(w, `{"ok":true}`)
				}
			}))
			defer upstream.Close()
			router := logTestRouter(func(c *gin.Context) {
				ctx := c.Request.Context()
				model.UpdateRequestLog(ctx, "test-model", stream)
				state := requeststate.From(ctx)
				state.SetSelection(requeststate.Selection{ChannelID: 18, UpstreamModel: "mapped-model"})
				state.SetStreamSession(gatewaystream.NewSession())
				client := requester.NewHTTPRequester("", nil)
				client.Context = ctx
				req, _ := client.NewRequest(http.MethodPost, upstream.URL+"/custom-endpoint", client.WithBody([]byte(`{"input":"hello"}`)))
				req = requester.WithRequestProtocol(req, "openai_responses")
				resp, apiErr := client.SendRequestRaw(req)
				if apiErr != nil {
					t.Error(apiErr)
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				model.RecordConsumeLog(ctx, 7, 18, 10, 4, "mapped-model", "test-token", 12, 8, "", 100, stream, map[string]any{"public_model": "test-model"}, "127.0.0.1")
				c.Status(http.StatusOK)
			})
			done := make(chan struct{})
			go func() {
				defer close(done)
				router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("upstream not reached")
			}
			var pending model.Log
			if err := model.DB.First(&pending).Error; err != nil {
				close(release)
				<-done
				t.Fatal(err)
			}
			if pending.RequestStatus != "processing" || pending.Type != model.LogTypeRequest || pending.StartedAt == 0 || pending.InboundProtocol != "openai_chat" || pending.UpstreamProtocol != "openai_responses" || pending.ChannelId != 18 {
				t.Errorf("unexpected pending row: %+v", pending)
			}
			if model.SumUsedQuota(&model.LogsListParams{}) != 0 {
				t.Error("pending row affected billing stats")
			}
			userLogs, err := model.GetUserLogsList(99, &model.LogsListParams{})
			if err != nil || userLogs.TotalCount != 0 {
				t.Errorf("user isolation: %+v %v", userLogs, err)
			}
			close(release)
			<-done
			var completed model.Log
			if err := model.DB.First(&completed).Error; err != nil {
				t.Fatal(err)
			}
			var count int64
			model.DB.Model(&model.Log{}).Count(&count)
			if count != 1 || completed.Id != pending.Id || completed.RequestStatus != "succeeded" || completed.Quota != 12 || completed.PromptTokens != 10 || completed.CompletedAt < completed.StartedAt || completed.StartedAt != pending.StartedAt {
				t.Fatalf("not finalized in place: count=%d row=%+v", count, completed)
			}
			ownLogs, err := model.GetUserLogsList(7, &model.LogsListParams{PaginationParams: model.PaginationParams{Page: 1, Size: 10}})
			if err != nil || ownLogs.TotalCount != 1 || (*ownLogs.Data)[0].Id != completed.Id || (*ownLogs.Data)[0].CostQuota != 0 {
				t.Errorf("self log identity/privacy: %+v %v", ownLogs, err)
			}
			if model.SumUsedQuota(&model.LogsListParams{}) != 12 {
				t.Error("final quota missing")
			}
		})
	}
}

func TestRequestLogRetryFailureCancelAndDisabled(t *testing.T) {
	requestLogTestDB(t)
	for _, scenario := range []string{"retry", "failed", "canceled", "disabled", "panic", "late_billing"} {
		t.Run(scenario, func(t *testing.T) {
			model.DB.Where("1=1").Delete(&model.Log{})
			config.LogConsumeEnabled, config.LogErrorEnabled = scenario != "disabled", scenario != "disabled"
			var late context.Context
			router := logTestRouter(func(c *gin.Context) {
				ctx := c.Request.Context()
				late = ctx
				state := requeststate.From(ctx)
				state.SetSelection(requeststate.Selection{ChannelID: 1})
				state.ObserveUpstreamProtocol("anthropic_messages")
				if scenario == "retry" {
					state.SetSelection(requeststate.Selection{ChannelID: 2})
					state.SetStreamSession(gatewaystream.NewSession())
					var pending model.Log
					model.DB.First(&pending)
					if pending.UpstreamProtocol != "" {
						t.Error("retry inherited previous protocol")
					}
					state.ObserveUpstreamProtocol("openai_responses")
				}
				if scenario == "failed" {
					model.RecordErrorLog(ctx, 7, 1, "test", "token", "upstream failure", 100, true, map[string]any{"status_code": 502}, "")
					c.Status(200) // SSE already committed headers.
				}
				if scenario == "panic" {
					panic("test panic")
				}
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if scenario == "canceled" {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			func() {
				defer func() {
					if r := recover(); r != nil && scenario != "panic" {
						t.Fatal(r)
					}
				}()
				router.ServeHTTP(httptest.NewRecorder(), req)
			}()
			if scenario == "late_billing" {
				model.RecordConsumeLog(late, 7, 1, 2, 3, "test", "token", 9, 5, "", 10, false, nil, "")
			}
			var rows []model.Log
			model.DB.Find(&rows)
			if scenario == "disabled" {
				if len(rows) != 0 {
					t.Fatal("disabled logs persisted")
				}
				return
			}
			if len(rows) != 1 {
				t.Fatalf("rows=%d", len(rows))
			}
			expected := "succeeded"
			if scenario == "failed" || scenario == "panic" {
				expected = "failed"
			}
			if scenario == "canceled" {
				expected = "canceled"
			}
			if rows[0].RequestStatus != expected {
				t.Errorf("status=%s want %s", rows[0].RequestStatus, expected)
			}
			if scenario == "retry" && (rows[0].UpstreamProtocol != "openai_responses" || rows[0].ChannelId != 2) {
				t.Error("final attempt not reflected")
			}
			if scenario == "late_billing" && rows[0].Quota != 9 {
				t.Error("late billing lost")
			}
		})
	}
}
