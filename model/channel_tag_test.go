package model

import (
	"testing"

	commonlogger "done-hub/common/logger"

	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestChannelTagQueriesNeverReturnCredentials(t *testing.T) {
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
		{Name: "first", Type: 1, Key: "first-secret", Tag: "shared", Status: 1},
		{Name: "second", Type: 1, Key: "second-secret", Tag: "shared", Status: 1},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}

	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	oldLogger := commonlogger.Logger
	commonlogger.Logger = zap.NewNop()
	defer func() { commonlogger.Logger = oldLogger }()

	result, err := GetChannelsTagList(&SearchChannelsTagParams{
		Tag: "shared",
		PaginationParams: PaginationParams{
			Page: 1,
			Size: 10,
		},
	})
	if err != nil {
		t.Fatalf("get tag list: %v", err)
	}
	for _, channel := range *result.Data {
		if channel.Key != "" {
			t.Fatalf("tag member exposed credential: %q", channel.Key)
		}
	}

	collection, err := GetChannelsTag("shared")
	if err != nil {
		t.Fatalf("get tag collection: %v", err)
	}
	if collection.Key != "" || len(collection.KeyMap) != 0 {
		t.Fatalf("tag collection exposed credential material: %#v", collection)
	}
}

func TestUpdateChannelsTagPreservesIndependentGatewayCredentials(t *testing.T) {
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

	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	oldLogger := commonlogger.Logger
	commonlogger.Logger = zap.NewNop()
	defer func() { commonlogger.Logger = oldLogger }()
	viper.Set("gateway_secret_key", "tag-credential-test")
	defer viper.Set("gateway_secret_key", "")

	channels := []Channel{
		{Name: "first", Type: 1, Key: "first-secret", Tag: "shared", Status: 1, Models: "gpt-test", Group: "default", AffinityEnabled: true},
		{Name: "second", Type: 1, Key: "second-secret", Tag: "shared", Status: 1, Models: "gpt-test", Group: "default", AffinityEnabled: true},
	}
	if err := BatchInsertChannels(channels); err != nil {
		t.Fatalf("insert channels: %v", err)
	}

	baseURL := "https://gateway.example.invalid"
	update := &Channel{
		Type:    1,
		Tag:     "shared",
		BaseURL: &baseURL,
		Models:  "gpt-test",
		Group:   "premium",
	}
	if err := UpdateChannelsTag("shared", update); err != nil {
		t.Fatalf("update tag: %v", err)
	}

	for index, want := range []string{"first-secret", "second-secret"} {
		_, got, err := LoadActiveGatewayCredentialSecret(db, channels[index].Id)
		if err != nil {
			t.Fatalf("load credential %d: %v", index, err)
		}
		if got != want {
			t.Fatalf("credential %d changed: want %q, got %q", index, want, got)
		}
		var stored Channel
		if err := db.First(&stored, channels[index].Id).Error; err != nil {
			t.Fatalf("reload channel %d: %v", index, err)
		}
		if stored.Key != "" {
			t.Fatalf("channel %d retained plaintext key %q", index, stored.Key)
		}
		if stored.Group != "premium" {
			t.Fatalf("channel %d shared config was not updated: %q", index, stored.Group)
		}
		if stored.AffinityEnabled {
			t.Fatalf("channel %d affinity disable was not persisted", index)
		}
		var policy GatewayPolicy
		if err := db.Where("channel_id = ?", channels[index].Id).First(&policy).Error; err != nil {
			t.Fatalf("load gateway policy %d: %v", index, err)
		}
		if policy.AffinityEnabled {
			t.Fatalf("gateway policy %d affinity disable was not projected", index)
		}
	}
}
