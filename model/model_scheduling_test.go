package model

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/secret"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func schedulingTestSetup(t *testing.T) (*gorm.DB, []Channel) {
	t.Helper()
	db := explorerTestDB(t)
	oldLogger, oldKey := logger.Logger, viper.GetString("gateway_secret_key")
	logger.Logger = zap.NewNop()
	viper.Set("gateway_secret_key", "scheduling-test")
	GatewayRoutes = GatewayRouteIndex{}
	t.Cleanup(func() {
		logger.Logger = oldLogger
		viper.Set("gateway_secret_key", oldKey)
		GatewayRoutes = GatewayRouteIndex{}
	})
	channels := []Channel{
		{Type: 1, ProtocolProfileID: "openai-chat-completions", Key: "test", Name: "A", Status: 1, Models: "alpha,beta,wild-*", Group: "default,vip", AffinityEnabled: true},
		{Type: 1, ProtocolProfileID: "openai-chat-completions", Key: "test", Name: "B", Status: 1, Models: "alpha,beta,wild-*", Group: "default,vip", AffinityEnabled: true},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}
	cipher, _ := secret.New("scheduling-test")
	if err := BackfillGatewayResources(db, cipher); err != nil {
		t.Fatal(err)
	}
	GatewayRoutes.Load()
	return db, channels
}

func TestModelSchedulingMigrationPreservesEffectiveValuesAndRunsOnce(t *testing.T) {
	db, channels := schedulingTestSetup(t)
	for _, table := range []string{"channels", "gateway_endpoints"} {
		for _, col := range []string{"priority", "weight"} {
			if err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " bigint").Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// Endpoint values were the actual running source even if the old source row differed.
	if err := db.Table("channels").Where("id > 0").Updates(map[string]any{"priority": 999, "weight": 999}).Error; err != nil {
		t.Fatal(err)
	}
	for i, ch := range channels {
		if err := db.Table("gateway_endpoints").Where("channel_id = ?", ch.Id).Updates(map[string]any{"priority": 100 - i*200, "weight": i * 3}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&GatewayMigrationState{Name: gatewayCoreCutoverMigration, CompletedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	var routes []GatewayModelRoute
	db.Order("channel_id,public_model").Find(&routes)
	if len(routes) != 6 {
		t.Fatal("missing bindings", len(routes))
	}
	for _, r := range routes {
		wantP, wantW := int64(100), uint(1)
		if r.ChannelID == channels[1].Id {
			wantP, wantW = -100, 3
		}
		if r.Priority != wantP || r.Weight != wantW {
			t.Fatalf("migration changed effective routing: %+v", r)
		}
	}
	ctx := context.Background()
	if err := SetModelChannelScheduling(ctx, channels[0].Id, "alpha", 42, 7); err != nil {
		t.Fatal(err)
	}
	if err := MigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	r, err := GetModelChannelRoute(ctx, channels[0].Id, "alpha")
	if err != nil || r.Priority != 42 || r.Weight != 7 {
		t.Fatal("restart overwrote model policy", r, err)
	}
	// Removed fields must not leak back into channel API payloads.
	raw, _ := json.Marshal(channels[0])
	var payload map[string]any
	json.Unmarshal(raw, &payload)
	for _, key := range []string{"priority", "weight", "ModelRoutes"} {
		if _, ok := payload[key]; ok {
			t.Fatal("legacy field exposed", key)
		}
	}
}

func TestModelSchedulingIsolationSelectionAndLifecycle(t *testing.T) {
	db, channels := schedulingTestSetup(t)
	ctx := context.Background()
	set := func(id int, m string, p int64, w uint) {
		t.Helper()
		if err := SetModelChannelScheduling(ctx, id, m, p, w); err != nil {
			t.Fatal(err)
		}
	}
	set(channels[0].Id, "alpha", 100, 3)
	set(channels[1].Id, "alpha", 0, 1)
	set(channels[0].Id, "beta", 0, 1)
	set(channels[1].Id, "beta", 100, 3)
	set(channels[0].Id, "wild-*", 0, 1)
	set(channels[1].Id, "wild-*", 100, 3)
	for _, group := range []string{"default", "vip"} {
		for name, want := range map[string]int{"alpha": channels[0].Id, "beta": channels[1].Id, "wild-example": channels[1].Id} {
			got, err := GatewayRoutes.Next(group, name)
			if err != nil || got.Id != want {
				t.Fatalf("%s/%s: %v %v", group, name, got, err)
			}
		}
	}
	// A failed attempt falls back without affecting other models.
	got, err := GatewayRoutes.NextByValidatedModel("default", "alpha", nil, FilterChannelId([]int{channels[0].Id}))
	if err != nil || got.Id != channels[1].Id {
		t.Fatal("retry fallback", got, err)
	}
	GatewayRoutes.SetCooldownsWithDuration(channels[0].Id, "alpha", 60)
	got, err = GatewayRoutes.Next("default", "alpha")
	if err != nil || got.Id != channels[1].Id {
		t.Fatal("cooldown fallback", got, err)
	}
	GatewayRoutes.ClearChannelCooldowns(channels[0].Id)
	before, _ := GetModelChannelRoute(ctx, channels[0].Id, "alpha")
	channels[0].Name = "Renamed"
	channels[0].Models += ",new-model"
	if err := channels[0].Update(true); err != nil {
		t.Fatal(err)
	}
	after, _ := GetModelChannelRoute(ctx, channels[0].Id, "alpha")
	if before.Priority != after.Priority || before.Weight != after.Weight {
		t.Fatal("channel edit overwrote schedule")
	}
	added, _ := GetModelChannelRoute(ctx, channels[0].Id, "new-model")
	if added.Weight != 1 || added.Priority != 0 {
		t.Fatal("new route defaults", added)
	}
	clone := channels[0]
	clone.Id = 0
	clone.Status = 2
	clone.Key = ""
	if err := clone.InsertClone(channels[0].Id); err != nil {
		t.Fatal(err)
	}
	copied, _ := GetModelChannelRoute(ctx, clone.Id, "alpha")
	if copied.Priority != 100 || copied.Weight != 3 {
		t.Fatal("clone lost schedule", copied)
	}
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "alpha", false); err != nil {
		t.Fatal(err)
	}
	set(channels[0].Id, "alpha", 200, 9)
	if !GatewayRoutes.IsModelDisabled(channels[0].Id, "alpha") {
		t.Fatal("schedule edit enabled binding")
	}
	if err := UpdateChannelStatusById(channels[0].Id, config.ChannelStatusManuallyDisabled); err != nil {
		t.Fatal(err)
	}
	set(channels[0].Id, "alpha", 300, 10)
	got, err = GatewayRoutes.Next("default", "alpha")
	if err != nil || got.Id != channels[1].Id {
		t.Fatal("global off bypass", got, err)
	}
	for _, w := range []uint{0, 1000001} {
		if err := SetModelChannelScheduling(ctx, channels[1].Id, "alpha", 0, w); err == nil {
			t.Fatal("invalid weight accepted")
		}
	}
	if err := SetModelChannelScheduling(ctx, channels[1].Id, "absent", 0, 1); err == nil {
		t.Fatal("missing binding accepted")
	}
	var all []GatewayModelRoute
	db.Find(&all)
	if len(all) == 0 {
		t.Fatal("routes lost")
	}
}

func TestModelSpecificWeightsDriveRandomAndAffinitySelection(t *testing.T) {
	_, channels := schedulingTestSetup(t)
	ctx := context.Background()
	for _, entry := range []struct {
		id int
		m  string
		w  uint
	}{{channels[0].Id, "alpha", 3}, {channels[1].Id, "alpha", 1}, {channels[0].Id, "beta", 1}, {channels[1].Id, "beta", 3}} {
		if err := SetModelChannelScheduling(ctx, entry.id, entry.m, 0, entry.w); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []string{"alpha", "beta"} {
		count := 0
		for i := 0; i < 4000; i++ {
			ch, err := GatewayRoutes.Next("default", m)
			if err != nil {
				t.Fatal(err)
			}
			if ch.Id == channels[0].Id {
				count++
			}
		}
		share := float64(count) / 4000
		if (m == "alpha" && (share < 0.65 || share > 0.85)) || (m == "beta" && (share < 0.15 || share > 0.35)) {
			t.Fatalf("%s weight share=%v", m, share)
		}
	}
	choices := []*ChannelChoice{GatewayRoutes.Channels[channels[0].Id], GatewayRoutes.Channels[channels[1].Id]}
	if choices[0].modelWeight("alpha") != 3 || choices[0].modelWeight("beta") != 1 {
		t.Fatal("weights not model scoped")
	}
	// Compare route-aware affinity to identical per-route candidates, not old channel fields.
	for _, m := range []string{"alpha", "beta"} {
		c := affinityTestContext(t, domain.ProtocolOpenAIResponses, `{"prompt_cache_key":"model-policy"}`)
		a := selectAffinityChannel(choices, m, c)
		b := selectAffinityChannel(choices, m, c)
		if a == nil || b == nil || a.Id != b.Id {
			t.Fatal("affinity unstable")
		}
	}
	oldRules := GatewayRoutes.Rule
	GatewayRoutes.Load()
	if !reflect.DeepEqual(oldRules, GatewayRoutes.Rule) {
		t.Fatal("reload changed route tiers")
	}
}
