package model

import (
	"testing"
	"time"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/secret"

	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestHydrateGatewayCredentialSummaries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()

	channels := []*Channel{
		{Type: config.ChannelTypeOpenAI, Name: "configured", Status: 1},
		{Type: config.ChannelTypeOllama, Name: "keyless", Status: 1},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	testedAt := time.Now().UTC().Truncate(time.Second)
	credentials := []GatewayCredential{
		{ChannelID: channels[0].Id, AuthMode: "opaque", Status: GatewayCredentialActive, SecretVersion: 1, TestedAt: &testedAt},
		{ChannelID: channels[1].Id, AuthMode: "none", Status: GatewayCredentialActive, SecretVersion: 1, TestedAt: &testedAt},
	}
	if err := db.Create(&credentials).Error; err != nil {
		t.Fatalf("create credentials: %v", err)
	}
	endpoints := []GatewayEndpoint{
		{ChannelID: channels[0].Id, ProviderID: channels[0].Type, Name: channels[0].Name, ActiveCredentialID: credentials[0].ID},
		{ChannelID: channels[1].Id, ProviderID: channels[1].Type, Name: channels[1].Name, ActiveCredentialID: credentials[1].ID},
	}
	if err := db.Create(&endpoints).Error; err != nil {
		t.Fatalf("create endpoints: %v", err)
	}

	if err := hydrateGatewayCredentialSummaries(channels, false); err != nil {
		t.Fatalf("hydrate summaries: %v", err)
	}
	if !channels[0].CredentialConfigured || channels[0].CredentialStatus != GatewayCredentialActive || channels[0].CredentialTestedAt == nil {
		t.Fatalf("unexpected configured summary: %#v", channels[0])
	}
	if !channels[1].CredentialConfigured || channels[1].CredentialStatus != "keyless" || channels[1].CredentialAuthMode != "none" {
		t.Fatalf("unexpected keyless summary: %#v", channels[1])
	}

	tagged := []*Channel{
		{Type: config.ChannelTypeOpenAI, Name: "tag-active", Tag: "shared", Status: 1},
		{Type: config.ChannelTypeOpenAI, Name: "tag-missing", Tag: "shared", Status: 1},
	}
	if err := db.Create(&tagged).Error; err != nil {
		t.Fatalf("create tagged channels: %v", err)
	}
	tagCredential := GatewayCredential{
		ChannelID: tagged[0].Id, AuthMode: "opaque", Status: GatewayCredentialActive, SecretVersion: 1,
	}
	if err := db.Create(&tagCredential).Error; err != nil {
		t.Fatalf("create tagged credential: %v", err)
	}
	if err := db.Create(&GatewayEndpoint{
		ChannelID: tagged[0].Id, ProviderID: tagged[0].Type, Name: tagged[0].Name, ActiveCredentialID: tagCredential.ID,
	}).Error; err != nil {
		t.Fatalf("create tagged endpoint: %v", err)
	}
	if err := hydrateGatewayCredentialSummaries(tagged[:1], true); err != nil {
		t.Fatalf("hydrate tag summary: %v", err)
	}
	if tagged[0].CredentialConfigured || tagged[0].CredentialStatus != "mixed" {
		t.Fatalf("unexpected aggregate tag summary: %#v", tagged[0])
	}
}

func TestHydrateGatewayCredentialsUsesBatchQueries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	viper.Set("gateway_secret_key", "batch-hydration-test")
	defer viper.Set("gateway_secret_key", "")
	secretCipher, err := secret.New("batch-hydration-test")
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	channels := []*Channel{
		{Type: config.ChannelTypeOpenAI, Name: "secret", Key: "legacy-secret"},
		{Type: config.ChannelTypeOllama, Name: "keyless", Key: "legacy-keyless"},
		{Type: config.ChannelTypeOpenAI, Name: "missing", Key: "preserved"},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	encryptedSecret, _ := secretCipher.Encrypt("active-secret")
	encryptedEmpty, _ := secretCipher.Encrypt("")
	credentials := []GatewayCredential{
		{ChannelID: channels[0].Id, AuthMode: "opaque", SecretCiphertext: encryptedSecret, SecretVersion: 1, Status: GatewayCredentialActive},
		{ChannelID: channels[1].Id, AuthMode: "none", SecretCiphertext: encryptedEmpty, SecretVersion: 1, Status: GatewayCredentialActive},
	}
	if err := db.Create(&credentials).Error; err != nil {
		t.Fatalf("create credentials: %v", err)
	}
	endpoints := []GatewayEndpoint{
		{ChannelID: channels[0].Id, ProviderID: channels[0].Type, Name: channels[0].Name, ActiveCredentialID: credentials[0].ID},
		{ChannelID: channels[1].Id, ProviderID: channels[1].Type, Name: channels[1].Name, ActiveCredentialID: credentials[1].ID},
	}
	if err := db.Create(&endpoints).Error; err != nil {
		t.Fatalf("create endpoints: %v", err)
	}

	queryCount := 0
	if err := db.Callback().Query().Before("gorm:query").Register("test:count_batch_hydration_queries", func(*gorm.DB) {
		queryCount++
	}); err != nil {
		t.Fatalf("register query counter: %v", err)
	}
	hydrateGatewayCredentials(channels)

	if queryCount != 2 {
		t.Fatalf("expected two batch queries, got %d", queryCount)
	}
	if channels[0].Key != "active-secret" || channels[1].Key != "" || channels[2].Key != "preserved" {
		t.Fatalf("unexpected hydrated keys: %q %q %q", channels[0].Key, channels[1].Key, channels[2].Key)
	}
}

func TestEnablingDraftReloadsGatewayRouteIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}

	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	GatewayRoutes = GatewayRouteIndex{}
	defer func() { GatewayRoutes = GatewayRouteIndex{} }()
	oldLogger := logger.Logger
	logger.Logger = zap.NewNop()
	defer func() { logger.Logger = oldLogger }()
	viper.Set("gateway_secret_key", "enable-draft-route-test")
	defer viper.Set("gateway_secret_key", "")

	weight := uint(1)
	priority := int64(0)
	baseURL := "https://example.invalid"
	channel := Channel{
		Type:              config.ChannelTypeOpenAI,
		ProtocolProfileID: "openai-responses",
		Key:               "candidate-secret",
		Status:            config.ChannelStatusManuallyDisabled,
		Name:              "draft",
		Weight:            &weight,
		Priority:          &priority,
		BaseURL:           &baseURL,
		Models:            "gpt-test",
		TestModel:         "gpt-test",
		Group:             "default",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	secretCipher, err := secret.New("enable-draft-route-test")
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("backfill gateway resources: %v", err)
	}
	GatewayRoutes.Load()
	if GatewayRoutes.GetChannel(channel.Id) != nil {
		t.Fatal("disabled draft unexpectedly entered route index")
	}

	if err := UpdateChannelStatusById(channel.Id, config.ChannelStatusEnabled); err != nil {
		t.Fatalf("enable channel: %v", err)
	}
	loaded := GatewayRoutes.GetChannel(channel.Id)
	if loaded == nil || loaded.Status != config.ChannelStatusEnabled {
		t.Fatalf("enabled draft missing from route index: %#v", loaded)
	}
	models, err := GatewayRoutes.GetGroupModels("default")
	if err != nil || len(models) != 1 || models[0] != "gpt-test" {
		t.Fatalf("unexpected group models: %v, %v", models, err)
	}
}

func TestBackfillGatewayResourcesIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}

	baseURL := "https://example.invalid"
	proxy := "http://proxy.invalid"
	modelMapping := `{"public-model":"upstream-model"}`
	priority := int64(9)
	weight := uint(3)
	costRatio := 1.25
	channel := Channel{
		Type:              1,
		ProtocolProfileID: "openai-responses",
		Key:               "credential-value",
		Status:            1,
		Name:              "test-endpoint",
		Weight:            &weight,
		BaseURL:           &baseURL,
		Models:            "public-model,second-model",
		Group:             "default",
		Tag:               "test",
		ModelMapping:      &modelMapping,
		Priority:          &priority,
		Proxy:             &proxy,
		PreCost:           1,
		CostRatio:         &costRatio,
		PassThroughBody:   true,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	secretCipher, err := secret.New("test-gateway-secret")
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	for range 2 {
		if err := BackfillGatewayResources(db, secretCipher); err != nil {
			t.Fatalf("backfill gateway resources: %v", err)
		}
	}

	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 1)
	assertGatewayResourceCount(t, db, &GatewayCredential{}, 1)
	assertGatewayResourceCount(t, db, &GatewayModelRoute{}, 2)
	assertGatewayResourceCount(t, db, &GatewayPolicy{}, 1)
	assertGatewayResourceCount(t, db, &GatewayHealthState{}, 1)

	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channel.Id).First(&endpoint).Error; err != nil {
		t.Fatalf("load endpoint: %v", err)
	}
	if endpoint.ProviderID != channel.Type || endpoint.ProtocolProfileID != channel.ProtocolProfileID || endpoint.ActiveCredentialID == 0 || endpoint.BaseURL != baseURL || endpoint.Priority != priority {
		t.Fatalf("unexpected endpoint: %#v", endpoint)
	}

	var credential GatewayCredential
	if err := db.Where("channel_id = ?", channel.Id).First(&credential).Error; err != nil {
		t.Fatalf("load credential: %v", err)
	}
	plaintext, err := secretCipher.Decrypt(credential.SecretCiphertext)
	if err != nil {
		t.Fatalf("decrypt credential: %v", err)
	}
	if plaintext != channel.Key {
		t.Fatalf("unexpected credential value %q", plaintext)
	}
	if credential.Status != "active" || credential.SecretVersion != 1 || credential.ID != endpoint.ActiveCredentialID {
		t.Fatalf("unexpected active credential: %#v", credential)
	}
	var stored Channel
	if err := db.First(&stored, channel.Id).Error; err != nil {
		t.Fatalf("reload legacy channel: %v", err)
	}
	if stored.Key != "" {
		t.Fatalf("legacy channel table still contains plaintext credential: %q", stored.Key)
	}

	var route GatewayModelRoute
	if err := db.Where("channel_id = ? AND public_model = ?", channel.Id, "public-model").First(&route).Error; err != nil {
		t.Fatalf("load model route: %v", err)
	}
	if route.UpstreamModel != "upstream-model" {
		t.Fatalf("unexpected upstream model %q", route.UpstreamModel)
	}
	if route.Protocol != "openai_responses" {
		t.Fatalf("unexpected route protocol %q", route.Protocol)
	}

	viper.Set("gateway_secret_key", "test-gateway-secret")
	defer viper.Set("gateway_secret_key", "")
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	oldLogger := logger.Logger
	logger.Logger = zap.NewNop()
	defer func() { logger.Logger = oldLogger }()
	hydrated, err := GetChannelById(channel.Id)
	if err != nil {
		t.Fatalf("load channel with active gateway credential: %v", err)
	}
	if hydrated.Key != channel.Key {
		t.Fatalf("operational channel did not hydrate the active credential: %q", hydrated.Key)
	}
	snapshots, err := LoadGatewayChannelSnapshots(db)
	if err != nil {
		t.Fatalf("load gateway channel snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected one channel snapshot, got %d", len(snapshots))
	}
	snapshot := snapshots[0]
	if snapshot.Key != channel.Key || snapshot.Models != "public-model,second-model" {
		t.Fatalf("unexpected channel snapshot: %#v", snapshot)
	}
}

func TestBackfillGatewayResourcesKeepsActiveCredentialAuthoritative(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channel := Channel{
		Type:              1,
		ProtocolProfileID: "openai-chat-completions",
		Key:               "first-key",
		Status:            1,
		Name:              "test-endpoint",
		Models:            "first-model,stale-model",
		Group:             "default",
		PreCost:           1,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	secretCipher, _ := secret.New("test-gateway-secret")
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("initial backfill: %v", err)
	}

	channel.Key = "second-key"
	channel.Models = "first-model,new-model"
	if err := db.Save(&channel).Error; err != nil {
		t.Fatalf("update channel: %v", err)
	}
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("updated backfill: %v", err)
	}

	assertGatewayResourceCount(t, db, &GatewayModelRoute{}, 2)
	assertGatewayResourceCount(t, db, &GatewayCredential{}, 1)
	var staleCount int64
	if err := db.Model(&GatewayModelRoute{}).
		Where("channel_id = ? AND public_model = ?", channel.Id, "stale-model").
		Count(&staleCount).Error; err != nil {
		t.Fatalf("count stale route: %v", err)
	}
	if staleCount != 0 {
		t.Fatalf("expected stale route to be removed, got %d", staleCount)
	}

	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channel.Id).First(&endpoint).Error; err != nil {
		t.Fatalf("load endpoint: %v", err)
	}
	var credential GatewayCredential
	if err := db.Where("id = ?", endpoint.ActiveCredentialID).First(&credential).Error; err != nil {
		t.Fatalf("load credential: %v", err)
	}
	plaintext, err := secretCipher.Decrypt(credential.SecretCiphertext)
	if err != nil {
		t.Fatalf("decrypt credential: %v", err)
	}
	if plaintext != "first-key" {
		t.Fatalf("legacy plaintext must not override the active encrypted credential, got %q", plaintext)
	}
	if credential.SecretVersion != 1 || credential.Status != "active" {
		t.Fatalf("unexpected active credential: %#v", credential)
	}
}

func TestBackfillGatewayResourcesPrunesDeletedChannels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channel := Channel{
		Type:    1,
		Key:     "credential",
		Status:  1,
		Name:    "deleted-endpoint",
		Models:  "model",
		Group:   "default",
		PreCost: 1,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	secretCipher, _ := secret.New("test-gateway-secret")
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("initial backfill: %v", err)
	}
	if err := db.Delete(&channel).Error; err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("backfill after delete: %v", err)
	}

	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 0)
	assertGatewayResourceCount(t, db, &GatewayCredential{}, 0)
	assertGatewayResourceCount(t, db, &GatewayModelRoute{}, 0)
	assertGatewayResourceCount(t, db, &GatewayPolicy{}, 0)
	assertGatewayResourceCount(t, db, &GatewayHealthState{}, 0)
}

func TestAutoMigrateGatewayResourcesDropsLegacyCredentialUniqueness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE gateway_credentials (
			id integer PRIMARY KEY AUTOINCREMENT,
			channel_id integer NOT NULL,
			auth_mode varchar(32) NOT NULL,
			secret_ciphertext text,
			secret_version integer NOT NULL DEFAULT 1,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy credentials table: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_gateway_credentials_channel_id ON gateway_credentials(channel_id)`).Error; err != nil {
		t.Fatalf("create legacy unique index: %v", err)
	}

	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	if db.Migrator().HasIndex(&GatewayCredential{}, "idx_gateway_credentials_channel_id") {
		t.Fatal("legacy one-credential-per-channel index still exists")
	}
	first := GatewayCredential{ChannelID: 1, AuthMode: "opaque", SecretCiphertext: "first", SecretVersion: 1, Status: "retired"}
	second := GatewayCredential{ChannelID: 1, AuthMode: "opaque", SecretCiphertext: "second", SecretVersion: 2, Status: "active"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("insert first credential version: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("insert second credential version: %v", err)
	}
}

func TestGatewayCredentialRotationRequiresTestAndSwitchesAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channel := Channel{
		Type:              1,
		ProtocolProfileID: "openai-chat-completions",
		Key:               "active-key",
		Status:            1,
		Name:              "rotation-test",
		Models:            "gpt-test",
		Group:             "default",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	viper.Set("gateway_secret_key", "rotation-test-secret")
	defer viper.Set("gateway_secret_key", "")
	secretCipher, _ := secret.New("rotation-test-secret")
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("backfill active credential: %v", err)
	}

	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channel.Id).First(&endpoint).Error; err != nil {
		t.Fatalf("load endpoint: %v", err)
	}
	oldCredentialID := endpoint.ActiveCredentialID
	pending, err := CreatePendingGatewayCredential(db, channel.Id, "api_key", "replacement-key")
	if err != nil {
		t.Fatalf("create pending credential: %v", err)
	}
	if pending.Status != GatewayCredentialPending || pending.SecretVersion != 2 {
		t.Fatalf("unexpected pending credential: %#v", pending)
	}
	if err := ActivateGatewayCredential(db, channel.Id, pending.ID); err == nil {
		t.Fatal("expected untested credential activation to fail")
	}
	if err := MarkGatewayCredentialTestResult(db, channel.Id, pending.ID, true); err != nil {
		t.Fatalf("mark credential tested: %v", err)
	}

	if err := ActivateGatewayCredential(db, channel.Id, pending.ID); err != nil {
		t.Fatalf("activate tested credential: %v", err)
	}

	if err := db.Where("channel_id = ?", channel.Id).First(&endpoint).Error; err != nil {
		t.Fatalf("reload endpoint: %v", err)
	}
	if endpoint.ActiveCredentialID != pending.ID {
		t.Fatalf("expected active credential %d, got %d", pending.ID, endpoint.ActiveCredentialID)
	}
	var updated Channel
	if err := db.First(&updated, channel.Id).Error; err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if updated.Key != "" {
		t.Fatalf("rotated plaintext leaked back to legacy channel table: %q", updated.Key)
	}
	var oldCredential GatewayCredential
	if err := db.First(&oldCredential, oldCredentialID).Error; err != nil {
		t.Fatalf("load old credential: %v", err)
	}
	if oldCredential.Status != GatewayCredentialRetired || oldCredential.RetiredAt == nil {
		t.Fatalf("old credential was not retired: %#v", oldCredential)
	}
	if err := RevokeGatewayCredential(db, channel.Id, pending.ID); err == nil {
		t.Fatal("expected active credential revoke to fail")
	}
	if err := RevokeGatewayCredential(db, channel.Id, oldCredentialID); err != nil {
		t.Fatalf("revoke retired credential: %v", err)
	}
}

func TestKeylessGatewayChannelCreatesNoneCredentialAndLoadsWithMixedPool(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channels := []Channel{
		{Type: config.ChannelTypeOllama, Key: "", Status: 1, Name: "ollama", Models: "llama3", Group: "default"},
		{Type: 1, Key: "openai-key", Status: 1, Name: "openai", Models: "gpt-test", Group: "default"},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	viper.Set("gateway_secret_key", "keyless-test-secret")
	defer viper.Set("gateway_secret_key", "")
	secretCipher, _ := secret.New("keyless-test-secret")
	if err := BackfillGatewayResources(db, secretCipher); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channels[0].Id).First(&endpoint).Error; err != nil {
		t.Fatalf("load keyless endpoint: %v", err)
	}
	if endpoint.ActiveCredentialID == 0 {
		t.Fatal("keyless endpoint must retain the active-credential invariant")
	}
	var credential GatewayCredential
	if err := db.First(&credential, endpoint.ActiveCredentialID).Error; err != nil {
		t.Fatalf("load keyless credential: %v", err)
	}
	if credential.AuthMode != "none" {
		t.Fatalf("expected auth mode none, got %q", credential.AuthMode)
	}

	snapshots, err := LoadGatewayChannelSnapshots(db)
	if err != nil {
		t.Fatalf("load mixed snapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected mixed pool with two channels, got %d", len(snapshots))
	}
	if snapshots[0].Key != "" {
		t.Fatalf("keyless snapshot unexpectedly has a credential: %q", snapshots[0].Key)
	}
}

func TestSyncGatewayChannelsDoesNotPruneUnrelatedResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	viper.Set("gateway_secret_key", "incremental-sync-secret")
	defer viper.Set("gateway_secret_key", "")

	channels := []Channel{
		{Type: 1, Key: "first", Status: 1, Name: "first", Models: "m1", Group: "default"},
		{Type: 1, Key: "second", Status: 1, Name: "second", Models: "m2", Group: "default"},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatalf("sync first: %v", err)
	}
	if err := SyncGatewayChannels(db, []int{channels[1].Id}); err != nil {
		t.Fatalf("sync second: %v", err)
	}
	if err := db.Model(&Channel{}).Where("id = ?", channels[0].Id).Update("models", "m1-new").Error; err != nil {
		t.Fatalf("update first: %v", err)
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatalf("resync first: %v", err)
	}

	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 2)
	var unrelated GatewayEndpoint
	if err := db.Where("channel_id = ?", channels[1].Id).First(&unrelated).Error; err != nil {
		t.Fatalf("unrelated endpoint was pruned: %v", err)
	}
	if err := db.Delete(&channels[0]).Error; err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatalf("sync deleted first: %v", err)
	}
	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 1)
}

func TestGatewayCutoverBackfillRunsOnlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	viper.Set("gateway_secret_key", "one-time-cutover-secret")
	defer viper.Set("gateway_secret_key", "")

	first := Channel{Type: 1, Key: "first", Status: 1, Name: "first", Models: "m1", Group: "default"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first channel: %v", err)
	}
	if err := MigrateGatewayResources(db); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 1)
	assertGatewayResourceCount(t, db, &GatewayMigrationState{}, 1)

	// Simulate an out-of-contract legacy write after cutover. A second startup
	// must not silently full-scan/backfill it; management writes use the
	// transactional SyncGatewayChannels boundary instead.
	second := Channel{Type: 1, Key: "second", Status: 1, Name: "second", Models: "m2", Group: "default"}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second channel: %v", err)
	}
	if err := MigrateGatewayResources(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 1)
}

func TestChannelDeleteAtomicallyRemovesGatewayProjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}

	viper.Set("gateway_secret_key", "atomic-delete-secret")
	defer viper.Set("gateway_secret_key", "")
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()
	oldLogger := logger.Logger
	logger.Logger = zap.NewNop()
	defer func() { logger.Logger = oldLogger }()

	channel := Channel{
		Type:    1,
		Key:     "delete-me",
		Status:  config.ChannelStatusEnabled,
		Name:    "atomic-delete",
		Models:  "delete-model",
		Group:   "default",
		PreCost: 1,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 1)
	assertGatewayResourceCount(t, db, &GatewayCredential{}, 1)
	assertGatewayResourceCount(t, db, &GatewayModelRoute{}, 1)
	assertGatewayResourceCount(t, db, &GatewayPolicy{}, 1)
	assertGatewayResourceCount(t, db, &GatewayHealthState{}, 1)

	if err := channel.Delete(); err != nil {
		t.Fatalf("delete channel: %v", err)
	}

	assertGatewayResourceCount(t, db, &GatewayEndpoint{}, 0)
	assertGatewayResourceCount(t, db, &GatewayCredential{}, 0)
	assertGatewayResourceCount(t, db, &GatewayModelRoute{}, 0)
	assertGatewayResourceCount(t, db, &GatewayPolicy{}, 0)
	assertGatewayResourceCount(t, db, &GatewayHealthState{}, 0)
}

func assertGatewayResourceCount(t *testing.T, db *gorm.DB, value any, expected int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Count(&count).Error; err != nil {
		t.Fatalf("count %T: %v", value, err)
	}
	if count != expected {
		t.Fatalf("count %T: expected %d, got %d", value, expected, count)
	}
}
