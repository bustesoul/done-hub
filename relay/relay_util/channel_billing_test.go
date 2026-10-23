package relay_util

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/requeststate"
	"done-hub/model"
	"done-hub/types"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestChannelBillingRatesAndPriceRules(t *testing.T) {
	for _, ratio := range []float64{0, 1, .5, .3333, .0001, 2} {
		q := &Quota{price: model.Price{Type: model.TokensPriceType, Input: 1, Output: 5}, groupRatio: 9, serviceTierRatio: 1, promptTokens: 349}
		q.applyChannelPricing(ratio)
		effective := ratio
		if effective == 0 {
			effective = 1
		}
		u := &types.Usage{PromptTokens: 349, CompletionTokens: 188}
		want := int(math.Ceil(1289 * effective))
		if got := q.GetTotalQuotaByUsage(u); got != want {
			t.Fatalf("ratio=%v charge=%d want=%d", ratio, got, want)
		}
		if q.GetCostQuotaByUsage(u) != want {
			t.Fatal("cost differs from debit")
		}
		meta := q.GetLogMeta(u)
		if meta["effective_ratio"] != effective || meta["channel_ratio"] != effective || meta["group_ratio"] != float64(9) {
			t.Fatal(meta)
		}
	}
	tier := datatypes.NewJSONType(model.LongContextTier{Threshold: 50, InputRatio: 2, OutputRatio: 1.5})
	q := &Quota{price: model.Price{Type: model.TokensPriceType, Input: 1, Output: 5, LongContext: &tier}, groupRatio: 9, serviceTierRatio: 1.5}
	q.applyChannelPricing(.5)
	u := &types.Usage{PromptTokens: 100, CompletionTokens: 20, PromptTokensDetails: types.PromptTokensDetails{CachedTokens: 50}}
	// 55 billable input after cache; input 2x, output 1.5x, channel .5x, priority 1.5x.
	if got := q.GetTotalQuotaByUsage(u); got != 195 {
		t.Fatalf("cache/context/tier charge=%d", got)
	}
	q.price.LongContext = nil
	q.extraBillingData = map[string]ExtraBillingData{"search": {Price: .002, CallCount: 2}}
	if got := q.GetTotalQuota(100, 20, nil); got != 1650 {
		t.Fatalf("extra service charge=%d", got)
	}
	q.price = model.Price{Type: model.TimesPriceType, Input: 2}
	q.extraBillingData = nil
	q.applyChannelPricing(.5)
	if got := q.GetTotalQuota(1, 0, nil); got != 1500 || q.estimatedQuota() != 1500 {
		t.Fatalf("per-call charge=%d estimate=%d", got, q.estimatedQuota())
	}
}

func TestChannelBillingDebitRetryRefundAndHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}); err != nil {
		t.Fatal(err)
	}
	oldErrorLog := config.LogErrorEnabled
	config.LogErrorEnabled = true
	defer func() { config.LogErrorEnabled = oldErrorLog }()
	oldDB, oldLogger := model.DB, logger.Logger
	oldRedis, oldBatch, oldLog, oldRemind, oldPre := config.RedisEnabled, config.BatchUpdateEnabled, config.LogConsumeEnabled, config.QuotaRemindEnabled, config.PreConsumedQuota
	model.DB, logger.Logger = db, zap.NewNop()
	config.RedisEnabled = false
	config.BatchUpdateEnabled = false
	config.LogConsumeEnabled = true
	config.QuotaRemindEnabled = false
	config.PreConsumedQuota = 100
	defer func() {
		model.DB, logger.Logger = oldDB, oldLogger
		config.RedisEnabled, config.BatchUpdateEnabled, config.LogConsumeEnabled, config.QuotaRemindEnabled, config.PreConsumedQuota = oldRedis, oldBatch, oldLog, oldRemind, oldPre
	}()
	for _, row := range []any{&model.User{Id: 1, Username: "billing-test", Quota: 2000}, &model.Token{Id: 1, UserId: 1, Key: "test-only", RemainQuota: 2000}, &model.Channel{Id: 1, Name: "test"}} {
		if err = db.Session(&gorm.Session{SkipHooks: true}).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	history := model.Log{UserId: 1, Type: model.LogTypeConsume, Quota: 1289, CostQuota: 0, Metadata: datatypes.NewJSONType(map[string]any{"group_ratio": 2.0})}
	if err = db.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	fresh := func(ratio float64) *Quota {
		q := &Quota{price: model.Price{Type: model.TokensPriceType, Input: 1, Output: 5}, groupRatio: 3, serviceTierRatio: 1, promptTokens: 349, userId: 1, tokenId: 1, channelId: 1}
		q.applyChannelPricing(ratio)
		return q
	}
	first := fresh(.5)
	if err := first.PreQuotaConsumption(); err != nil {
		t.Fatal(err)
	}
	if first.preConsumedQuota != 224 {
		t.Fatalf("expected discounted reservation, got %d", first.preConsumedQuota)
	}
	final := fresh(.25)
	first.TransferReservationTo(final)
	if first.HandelStatus || first.preConsumedQuota != 0 {
		t.Fatal("first attempt still owns reservation")
	}
	req, state := requeststate.Ensure(httptest.NewRequest("POST", "/v1/chat/completions", nil))
	started := time.Now().Add(-100 * time.Millisecond)
	ctx, record := model.BeginRequestLog(req.Context(), model.Log{UserId: 1, ModelName: "test"}, started)
	timing := state.BeginUpstreamRequest()
	timing.RecordData()
	if err = final.ConsumeWithSnapshot(ConsumeSnapshot{Ctx: ctx, StartTime: started}, &types.Usage{PromptTokens: 349, CompletionTokens: 188}, true); err != nil {
		t.Fatal(err)
	}
	record.Finish(ctx, 200, false)
	assertBalances := func(want int) {
		var u model.User
		var token model.Token
		db.First(&u, 1)
		db.First(&token, 1)
		if u.Quota != want || token.RemainQuota != want {
			t.Fatalf("balances user=%d token=%d want=%d", u.Quota, token.RemainQuota, want)
		}
	}
	assertBalances(1677) // ceil((349 + 188*5) * .25) = 323, not the first channel's rate.
	var log model.Log
	db.Order("id DESC").First(&log)
	if log.Quota != 323 || log.CostQuota != 323 || model.LogBillingRatio(log.Metadata.Data()) != .25 {
		t.Fatalf("charge snapshot: %+v", log)
	}
	if model.LogFirstResponseMs(&log) <= 0 || model.LogResponseDurationMs(&log) <= 0 || log.RequestTime < 100 {
		t.Fatal("timing snapshot lost")
	}
	var unchanged model.Log
	db.First(&unchanged, history.Id)
	if unchanged.Quota != 1289 || unchanged.CostQuota != 0 || unchanged.Metadata.Data()["billing_version"] != nil {
		t.Fatal("rewrote historical bill")
	}
	failed := fresh(.5)
	if err := failed.PreQuotaConsumption(); err != nil {
		t.Fatal(err)
	}
	failed.UndoWithContext(context.Background())
	assertBalances(1677)
	log = model.Log{}
	model.RecordErrorLog(req.Context(), 1, 1, "test", "", "failure", 1, false, nil, "")
	db.Order("id DESC").First(&log)
	if log.Type != model.LogTypeError || model.LogFirstResponseMs(&log) <= 0 || model.LogResponseDurationMs(&log) <= 0 {
		t.Fatal("standalone error log lost response timing")
	}
}
