package task

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	gatewayretry "done-hub/internal/gateway/retry"
	"done-hub/model"
	"done-hub/relay/task/base"

	"github.com/gin-gonic/gin"
)

func TestShouldRetryTaskUsesGatewayPolicyAndAdapterVeto(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		taskErr        *base.TaskError
		adapterAllows  bool
		specificID     int
		ignoreSpecific bool
		want           bool
	}{
		{
			name:          "transient error retries",
			taskErr:       taskPolicyError(http.StatusBadGateway, false),
			adapterAllows: true,
			want:          true,
		},
		{
			name:          "adapter can veto non idempotent task",
			taskErr:       taskPolicyError(http.StatusBadGateway, false),
			adapterAllows: false,
		},
		{
			name:          "local error never retries",
			taskErr:       taskPolicyError(http.StatusBadRequest, true),
			adapterAllows: true,
		},
		{
			name:          "specific channel never retries",
			taskErr:       taskPolicyError(http.StatusBadGateway, false),
			adapterAllows: true,
			specificID:    10,
		},
		{
			name:          "zero specific channel does not disable retry",
			taskErr:       taskPolicyError(http.StatusBadGateway, false),
			adapterAllows: true,
			specificID:    -1,
			want:          true,
		},
		{
			name:           "ignored specific channel does not disable retry",
			taskErr:        taskPolicyError(http.StatusBadGateway, false),
			adapterAllows:  true,
			specificID:     10,
			ignoreSpecific: true,
			want:           true,
		},
		{
			name:          "temporary redirect retries",
			taskErr:       taskPolicyError(http.StatusTemporaryRedirect, false),
			adapterAllows: true,
			want:          true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if test.specificID != 0 {
				specificID := test.specificID
				if specificID < 0 {
					specificID = 0
				}
				context.Set("specific_channel_id", specificID)
			}
			if test.ignoreSpecific {
				context.Set("specific_channel_id_ignore", true)
			}
			upstreamErr := classifyTaskError(test.taskErr)
			if !test.adapterAllows ||
				(context.GetInt("specific_channel_id") > 0 && !context.GetBool("specific_channel_id_ignore")) {
				upstreamErr.Class = gatewayretry.ErrorClassProtocol
			}
			got := gatewayretry.DefaultPolicy().Decide(upstreamErr, gatewayretry.DecisionContext{
				Attempt:     1,
				MaxAttempts: 3,
			}).Retry
			if got != test.want {
				t.Fatalf("expected retry=%t, got %t", test.want, got)
			}
		})
	}
}

func TestTaskRetryConsumesPolicyCooldown(t *testing.T) {
	config.SetRetryCooldownPerStatusMap(map[int]int{})
	defer config.SetRetryCooldownPerStatusMap(map[int]int{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/suno/submit/test", nil)
	channel := &model.Channel{Id: 987655}
	upstreamErr := classifyTaskError(taskPolicyError(http.StatusBadGateway, false))
	decision := gatewayretry.DefaultPolicy().Decide(upstreamErr, gatewayretry.DecisionContext{
		Attempt:     1,
		MaxAttempts: 3,
	})
	store := &taskCooldownStore{context: context}
	if err := store.Cooldown(context.Request.Context(), domain.Endpoint{ID: channel.Id}, "task-cooldown-model", upstreamErr, decision); err != nil {
		t.Fatalf("apply cooldown: %v", err)
	}
	if !model.GatewayRoutes.IsInCooldown(channel.Id, "task-cooldown-model") {
		t.Fatal("task failed endpoint was not placed in cooldown")
	}
	model.GatewayRoutes.ClearChannelCooldowns(channel.Id)
}

func taskPolicyError(statusCode int, local bool) *base.TaskError {
	return &base.TaskError{
		StatusCode: statusCode,
		Message:    http.StatusText(statusCode),
		LocalError: local,
	}
}
