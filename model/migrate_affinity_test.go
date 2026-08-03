package model

import (
	"testing"

	"done-hub/common/config"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestEnableLegacySubscriptionAffinityMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channels: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channels := []Channel{
		{Name: "codex", Type: config.ChannelTypeCodex},
		{Name: "claude", Type: config.ChannelTypeClaudeCode},
		{Name: "gemini", Type: config.ChannelTypeGeminiCli},
		{Name: "ordinary", Type: config.ChannelTypeOpenAI},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	for _, channel := range channels {
		if err := db.Create(&GatewayPolicy{ChannelID: channel.Id}).Error; err != nil {
			t.Fatalf("create policy: %v", err)
		}
	}
	if err := enableLegacySubscriptionAffinity().Migrate(db); err != nil {
		t.Fatalf("run migration: %v", err)
	}
	for index, channel := range channels {
		var stored Channel
		if err := db.First(&stored, channel.Id).Error; err != nil {
			t.Fatalf("load channel: %v", err)
		}
		want := index < 3
		if stored.AffinityEnabled != want {
			t.Fatalf("channel %q affinity=%t want %t", stored.Name, stored.AffinityEnabled, want)
		}
		var policy GatewayPolicy
		if err := db.Where("channel_id = ?", channel.Id).First(&policy).Error; err != nil {
			t.Fatalf("load policy: %v", err)
		}
		if policy.AffinityEnabled != want {
			t.Fatalf("policy %q affinity=%t want %t", stored.Name, policy.AffinityEnabled, want)
		}
	}
}
