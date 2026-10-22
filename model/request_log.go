package model

import (
	"context"
	"sync"
	"time"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/requeststate"
	"gorm.io/datatypes"
)

type requestLogKey struct{}

// RequestLog owns one persisted row per client request, including retries and late billing.
// Persistence stays in model; providers only publish wire protocol observations.
type RequestLog struct {
	mu        sync.Mutex
	row       Log
	started   time.Time
	ended     bool
	hasResult bool
}

func BeginRequestLog(ctx context.Context, row Log, started time.Time) (context.Context, *RequestLog) {
	if !config.LogConsumeEnabled && !config.LogErrorEnabled {
		return ctx, nil
	}
	if started.IsZero() {
		started = time.Now()
	}
	if row.Username == "" {
		row.Username, _ = CacheGetUsername(row.UserId)
	}
	row.Type = LogTypeRequest
	row.CreatedAt = started.Unix()
	row.StartedAt = started.UnixMilli()
	row.RequestStatus = "processing"
	if err := DB.Create(&row).Error; err != nil {
		logger.LogError(ctx, "start request log: "+err.Error())
		return ctx, nil
	}
	session := &RequestLog{row: row, started: started}
	return context.WithValue(ctx, requestLogKey{}, session), session
}

func requestLogFrom(ctx context.Context) *RequestLog {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(requestLogKey{}).(*RequestLog)
	return session
}

func (s *RequestLog) save(ctx context.Context) {
	// Select(*) intentionally writes zero values. Updates (not Save) never resurrect deleted logs.
	if err := DB.Model(&Log{}).Where("id = ?", s.row.Id).Select("*").Omit("id", "Channel").Updates(&s.row).Error; err != nil {
		logger.LogError(ctx, "update request log: "+err.Error())
	}
}

func UpdateRequestLog(ctx context.Context, modelName string, stream bool) {
	s := requestLogFrom(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.row.ModelName, s.row.IsStream = modelName, stream
	s.save(ctx)
}

func (s *RequestLog) ObserveUpstream(ctx context.Context, protocol string) {
	if s == nil {
		return
	}
	selection := requeststate.From(ctx).Selection()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.row.UpstreamProtocol = protocol
	s.row.ChannelId = selection.ChannelID
	if selection.UpstreamModel != "" {
		s.row.ModelName = selection.UpstreamModel
	}
	s.save(ctx)
}

func recordRequestLogResult(ctx context.Context, result *Log) bool {
	s := requestLogFrom(ctx)
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A charged failed stream can produce both usage and an error. Preserve actual usage.
	if result.Type == LogTypeConsume || s.row.Type != LogTypeConsume {
		s.row.Type = result.Type
		s.row.Quota, s.row.CostQuota = result.Quota, result.CostQuota
		s.row.PromptTokens, s.row.CompletionTokens = result.PromptTokens, result.CompletionTokens
	}
	s.row.Content = result.Content
	s.row.ChannelId, s.row.ModelName = result.ChannelId, result.ModelName
	s.row.IsStream = result.IsStream
	metadata := s.row.Metadata.Data()
	if metadata == nil {
		metadata = map[string]any{}
	}
	for key, value := range result.Metadata.Data() {
		metadata[key] = value
	}
	s.row.Metadata = datatypes.NewJSONType(metadata)
	if result.Type == LogTypeError {
		s.row.RequestStatus = "failed"
	}
	s.hasResult = true
	if s.ended {
		s.save(ctx)
	}
	return true
}

func (s *RequestLog) Finish(ctx context.Context, statusCode int, panicked bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
	s.row.CompletedAt = time.Now().UnixMilli()
	s.row.RequestTime = int(time.Since(s.started).Milliseconds())
	switch {
	case panicked:
		s.row.RequestStatus = "failed"
	case ctx.Err() != nil || s.row.RequestStatus == "canceled":
		s.row.RequestStatus = "canceled"
	case statusCode >= 400 || s.row.RequestStatus == "failed":
		s.row.RequestStatus = "failed"
	default:
		s.row.RequestStatus = "succeeded"
	}
	failed := s.row.RequestStatus != "succeeded"
	if (!failed && !config.LogConsumeEnabled) || (failed && !config.LogErrorEnabled && s.row.Type != LogTypeConsume) {
		if err := DB.Delete(&Log{}, s.row.Id).Error; err != nil {
			logger.LogError(ctx, "remove disabled request log: "+err.Error())
		}
		return
	}
	if failed && !s.hasResult {
		s.row.Type = LogTypeError
	}
	metadata := s.row.Metadata.Data()
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["request_time_unit"] = "ms"
	if _, exists := metadata["status_code"]; !exists {
		metadata["status_code"] = statusCode
	}
	s.row.Metadata = datatypes.NewJSONType(metadata)
	s.save(ctx)
}

// MarkRequestLogFailed also covers streamed errors after HTTP headers were committed.
func MarkRequestLogFailed(ctx context.Context) {
	SetRequestLogOutcome(ctx, "failed")
}

// SetRequestLogOutcome accepts terminal outcomes from asynchronous usage settlement.
func SetRequestLogOutcome(ctx context.Context, status string) {
	s := requestLogFrom(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	s.row.RequestStatus = status
	if s.ended {
		s.save(ctx)
	}
	s.mu.Unlock()
}
