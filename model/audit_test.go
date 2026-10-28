package model

import (
	"context"
	"done-hub/common/config"
	"done-hub/internal/audit"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func auditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err = db.AutoMigrate(&Option{}, &GatewayEndpoint{}, &Token{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldService := DB, audit.Default
	DB, audit.Default = db, audit.NewService()
	t.Cleanup(func() { DB, audit.Default = oldDB, oldService; _ = sqlDB.Close() })
	return db
}

func TestAuditOptionSaveValidatePersistAndHotReload(t *testing.T) {
	db := auditTestDB(t)
	ctx := context.Background()
	if err := ReloadAuditConfig(ctx); err != nil || audit.Default.Status().Enabled {
		t.Fatal("not default off", err)
	}
	valid := `{"channel_ids":[25],"token_ids":[7],"archive_enabled":true}`
	if err := UpdateOption(audit.OptionKey, valid); err != nil {
		t.Fatal(err)
	}
	if !audit.Default.Config().Matches(25, 7) || audit.Default.Config().Matches(24, 7) || !audit.Default.Config().ArchiveEnabled {
		t.Fatal("save did not publish intersection")
	}
	if err := UpdateOption(audit.OptionKey, `{"token_ids":[-7]}`); err == nil {
		t.Fatal("invalid option accepted")
	}
	stored, err := GetOption(audit.OptionKey)
	if err != nil || stored.Value != valid {
		t.Fatal("invalid value persisted", stored, err)
	}
	// A DB failure must not publish a configuration that won't survive restart.
	if err = db.Callback().Update().Before("gorm:update").Register("audit_fail_save", func(tx *gorm.DB) { tx.AddError(errors.New("database unavailable")) }); err != nil {
		t.Fatal(err)
	}
	if err = UpdateOption(audit.OptionKey, `{"channel_ids":[],"token_ids":[]}`); err == nil {
		t.Fatal("DB failure ignored")
	}
	if !audit.Default.Status().Enabled {
		t.Fatal("failed save changed runtime")
	}
	db.Callback().Update().Remove("audit_fail_save")
	// Simulate another instance writing the single atomic option.
	if err = db.Save(&Option{Key: audit.OptionKey, Value: `{"channel_ids":[],"token_ids":[9]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err = ReloadAuditConfig(ctx); err != nil || !audit.Default.Config().Matches(24, 9) {
		t.Fatal("remote reload failed", err)
	}
	if audit.Default.Config().ArchiveEnabled {
		t.Fatal("legacy configuration did not preserve normal-mode default")
	}
	if err = db.Save(&Option{Key: audit.OptionKey, Value: `broken`}).Error; err != nil {
		t.Fatal(err)
	}
	if err = ReloadAuditConfig(ctx); err == nil || !audit.Default.Config().Matches(24, 9) {
		t.Fatal("bad remote config replaced last good value")
	}
	if err = db.Delete(&Option{Key: audit.OptionKey}).Error; err != nil {
		t.Fatal(err)
	}
	if err = ReloadAuditConfig(ctx); err != nil || audit.Default.Status().Enabled {
		t.Fatal("deleted config did not disable", err)
	}
}

func TestAuditSyncWithoutMemoryCache(t *testing.T) {
	db := auditTestDB(t)
	oldCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = oldCache })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); SyncAuditConfig(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	if err := db.Save(&Option{Key: audit.OptionKey, Value: `{"token_ids":[7]}`}).Error; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(7 * time.Second)
	for !audit.Default.Config().Matches(25, 7) {
		if time.Now().After(deadline) {
			t.Fatal("audit config not refreshed with memory cache disabled")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAuditTargetsAreUngroupedAndContainNoSecrets(t *testing.T) {
	db := auditTestDB(t)
	for _, endpoint := range []GatewayEndpoint{
		{ChannelID: 25, Name: "first", Tags: "same-tag"},
		{ChannelID: 26, Name: "second", Tags: "same-tag"},
		{ChannelID: 27, Name: "deleted"},
	} {
		if err := db.Create(&endpoint).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Where("channel_id = ?", 27).Delete(&GatewayEndpoint{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&Token{Id: 7, UserId: 9, Name: "sample-token", Key: "private-token-secret"}).Error; err != nil {
		t.Fatal(err)
	}
	channels, err := ListAuditTargets(context.Background(), "channels", "")
	if err != nil || len(channels) != 2 {
		t.Fatal("catalogue grouped or included deleted channels", channels, err)
	}
	for _, kind := range []string{"channels", "tokens"} {
		targets, err := ListAuditTargets(context.Background(), kind, map[string]string{"channels": "26", "tokens": "sample"}[kind])
		if err != nil || len(targets) != 1 {
			t.Fatal(targets, err)
		}
		body, _ := json.Marshal(targets)
		if strings.Contains(string(body), "secret") || strings.Contains(string(body), `"key"`) {
			t.Fatal("catalogue leaked credentials", string(body))
		}
	}
}
