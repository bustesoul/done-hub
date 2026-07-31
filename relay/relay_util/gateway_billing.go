package relay_util

import (
	"context"
	"sync"
	"time"

	"done-hub/common/logger"
	"done-hub/internal/gateway/billing"
	"done-hub/internal/gateway/domain"
	"done-hub/types"

	"github.com/gin-gonic/gin"
)

type GatewayBilling struct {
	mu                sync.Mutex
	session           *billing.Session
	quota             *Quota
	usage             *types.Usage
	committedUsage    *types.Usage
	reportedUsage     domain.Usage
	snapshot          ConsumeSnapshot
	stream            bool
	firstResponseTime func() time.Time
}

func NewGatewayBilling(
	c *gin.Context,
	modelName string,
	promptTokens int,
	usage *types.Usage,
	stream bool,
	firstResponseTime func() time.Time,
) *GatewayBilling {
	gatewayBilling := &GatewayBilling{
		quota:             NewQuota(c, modelName, promptTokens),
		usage:             usage,
		committedUsage:    &types.Usage{},
		snapshot:          NewConsumeSnapshot(c),
		stream:            stream,
		firstResponseTime: firstResponseTime,
	}
	gatewayBilling.session = billing.NewSession(
		c.GetString(logger.RequestIdKey),
		&quotaLedger{billing: gatewayBilling},
	)
	return gatewayBilling
}

func (b *GatewayBilling) Precharge(ctx context.Context, estimate billing.Estimate) error {
	return b.session.Precharge(ctx, estimate)
}

func (b *GatewayBilling) BeginAttempt(
	c *gin.Context,
	modelName string,
	promptTokens int,
	usage *types.Usage,
	stream bool,
	firstResponseTime func() time.Time,
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	nextQuota := NewQuota(c, modelName, promptTokens)
	b.quota.TransferReservationTo(nextQuota)
	b.quota = nextQuota
	b.usage = usage
	b.stream = stream
	b.firstResponseTime = firstResponseTime
}

// RecordAttemptUsage preserves usage reported by an unsuccessful upstream
// attempt before BeginAttempt replaces the provider-owned Usage pointer.
// Local prompt estimates have TotalTokens==0 and are intentionally ignored.
func (b *GatewayBilling) RecordAttemptUsage(ctx context.Context) error {
	b.mu.Lock()
	current := cloneUsage(b.usage)
	b.mu.Unlock()
	if !HasReportedUsage(current) {
		return nil
	}
	if err := b.session.AddUsage(ctx, gatewayDomainUsage(current)); err != nil {
		return err
	}
	b.mu.Lock()
	mergeUsage(b.committedUsage, current)
	b.mu.Unlock()
	return nil
}

func (b *GatewayBilling) Settle(ctx context.Context, outcome domain.Outcome) error {
	b.mu.Lock()
	current := cloneUsage(b.usage)
	b.mu.Unlock()
	return b.session.Settle(ctx, gatewayDomainUsage(current), outcome)
}

func (b *GatewayBilling) Refund(ctx context.Context, reason string) error {
	return b.session.Refund(ctx, reason)
}

func (b *GatewayBilling) Quota() *Quota {
	return b.quota
}

func (b *GatewayBilling) RefundIfReserved(ctx context.Context, reason string) error {
	if b.session.State() != billing.StateReserved {
		return nil
	}
	return b.session.Refund(ctx, reason)
}

type quotaLedger struct {
	billing *GatewayBilling
}

func (l *quotaLedger) Precharge(_ context.Context, requestID string, _ billing.Estimate) (string, error) {
	if err := l.billing.quota.PreQuotaConsumption(); err != nil {
		return "", err
	}
	return requestID, nil
}

func (l *quotaLedger) AddUsage(_ context.Context, _ string, usage domain.Usage) error {
	l.billing.mu.Lock()
	defer l.billing.mu.Unlock()
	l.billing.reportedUsage = l.billing.reportedUsage.Add(usage)
	return nil
}

func (l *quotaLedger) Settle(_ context.Context, _ string, charge billing.FinalCharge) error {
	l.billing.mu.Lock()
	quota := l.billing.quota
	current := cloneUsage(l.billing.usage)
	usage := cloneUsage(l.billing.committedUsage)
	mergeUsage(usage, current)
	applyDomainUsage(usage, charge.Usage)
	firstResponseTime := l.billing.firstResponseTime
	snapshot := l.billing.snapshot
	stream := l.billing.stream
	l.billing.mu.Unlock()

	if firstResponseTime != nil {
		quota.SetFirstResponseTime(firstResponseTime())
	}
	return quota.ConsumeWithSnapshot(snapshot, usage, stream)
}

func HasReportedUsage(usage *types.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.TotalTokens > 0 || usage.CompletionTokens > 0 ||
		usage.PromptTokensDetails.CachedTokens > 0 ||
		usage.PromptTokensDetails.CachedReadTokens > 0 ||
		usage.PromptTokensDetails.CachedWriteTokens > 0 ||
		len(usage.ExtraTokens) > 0 || len(usage.ExtraBilling) > 0
}

func DomainUsage(usage *types.Usage) domain.Usage {
	return gatewayDomainUsage(usage)
}

func cloneUsage(source *types.Usage) *types.Usage {
	if source == nil {
		return &types.Usage{}
	}
	cloned := *source
	cloned.ExtraTokens = make(map[string]int, len(source.ExtraTokens))
	for key, value := range source.ExtraTokens {
		cloned.ExtraTokens[key] = value
	}
	cloned.ExtraBilling = make(map[string]types.ExtraBilling, len(source.ExtraBilling))
	for key, value := range source.ExtraBilling {
		cloned.ExtraBilling[key] = value
	}
	return &cloned
}

func mergeUsage(target, delta *types.Usage) {
	if target == nil || delta == nil {
		return
	}
	target.PromptTokens += delta.PromptTokens
	target.CompletionTokens += delta.CompletionTokens
	target.TotalTokens += delta.TotalTokens
	target.CacheCreationInputTokens += delta.CacheCreationInputTokens
	target.CacheReadInputTokens += delta.CacheReadInputTokens
	target.PromptTokensDetails.AudioTokens += delta.PromptTokensDetails.AudioTokens
	target.PromptTokensDetails.CachedTokens += delta.PromptTokensDetails.CachedTokens
	target.PromptTokensDetails.TextTokens += delta.PromptTokensDetails.TextTokens
	target.PromptTokensDetails.ImageTokens += delta.PromptTokensDetails.ImageTokens
	target.PromptTokensDetails.CachedTokensInternal += delta.PromptTokensDetails.CachedTokensInternal
	target.PromptTokensDetails.CachedWriteTokens += delta.PromptTokensDetails.CachedWriteTokens
	target.PromptTokensDetails.CachedWrite1hTokens += delta.PromptTokensDetails.CachedWrite1hTokens
	target.PromptTokensDetails.CachedReadTokens += delta.PromptTokensDetails.CachedReadTokens
	target.PromptTokensDetails.OpenAICacheWriteTokens += delta.PromptTokensDetails.OpenAICacheWriteTokens
	target.CompletionTokensDetails.AudioTokens += delta.CompletionTokensDetails.AudioTokens
	target.CompletionTokensDetails.TextTokens += delta.CompletionTokensDetails.TextTokens
	target.CompletionTokensDetails.ReasoningTokens += delta.CompletionTokensDetails.ReasoningTokens
	target.CompletionTokensDetails.AcceptedPredictionTokens += delta.CompletionTokensDetails.AcceptedPredictionTokens
	target.CompletionTokensDetails.RejectedPredictionTokens += delta.CompletionTokensDetails.RejectedPredictionTokens
	target.CompletionTokensDetails.ImageTokens += delta.CompletionTokensDetails.ImageTokens
	if target.ExtraTokens == nil {
		target.ExtraTokens = make(map[string]int)
	}
	for key, value := range delta.ExtraTokens {
		target.ExtraTokens[key] += value
	}
	if target.ExtraBilling == nil {
		target.ExtraBilling = make(map[string]types.ExtraBilling)
	}
	for key, value := range delta.ExtraBilling {
		current := target.ExtraBilling[key]
		current.Type = value.Type
		current.CallCount += value.CallCount
		target.ExtraBilling[key] = current
	}
}

func applyDomainUsage(usage *types.Usage, aggregate domain.Usage) {
	usage.PromptTokens = aggregate.InputTokens
	usage.CompletionTokens = aggregate.OutputTokens
	usage.TotalTokens = aggregate.InputTokens + aggregate.OutputTokens
	usage.CompletionTokensDetails.ReasoningTokens = aggregate.ReasoningTokens
}

func (l *quotaLedger) Refund(ctx context.Context, _ string, _ string) error {
	l.billing.quota.UndoWithContext(ctx)
	return nil
}

func gatewayDomainUsage(usage *types.Usage) domain.Usage {
	if usage == nil {
		return domain.Usage{}
	}
	return domain.Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		CachedTokens: usage.PromptTokensDetails.CachedTokens +
			usage.PromptTokensDetails.CachedReadTokens,
		ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
	}
}
