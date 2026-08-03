package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/secret"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GatewayEndpoint struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	ChannelID          int            `json:"channel_id" gorm:"uniqueIndex;not null"`
	ProviderID         int            `json:"provider_id" gorm:"index;not null"`
	ProtocolProfileID  string         `json:"protocol_profile_id" gorm:"type:varchar(64);index"`
	ActiveCredentialID uint           `json:"active_credential_id" gorm:"index"`
	Name               string         `json:"name" gorm:"index;not null"`
	BaseURL            string         `json:"base_url" gorm:"type:text"`
	Proxy              string         `json:"proxy" gorm:"type:text"`
	Other              string         `json:"other" gorm:"type:text"`
	Remark             string         `json:"remark" gorm:"type:text"`
	TestModel          string         `json:"test_model" gorm:"type:varchar(255)"`
	Group              string         `json:"group" gorm:"index"`
	Tags               string         `json:"tags" gorm:"type:text"`
	Status             int            `json:"status" gorm:"index"`
	Weight             uint           `json:"weight"`
	Priority           int64          `json:"priority" gorm:"index"`
	CreatedTime        int64          `json:"created_time"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

type GatewayCredential struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	ChannelID        int            `json:"channel_id" gorm:"uniqueIndex:idx_gateway_credential_version,priority:1;index;not null"`
	AuthMode         string         `json:"auth_mode" gorm:"type:varchar(32);not null"`
	SecretCiphertext string         `json:"-" gorm:"type:text"`
	SecretVersion    int            `json:"secret_version" gorm:"uniqueIndex:idx_gateway_credential_version,priority:2;not null;default:1"`
	Status           string         `json:"status" gorm:"type:varchar(16);index;not null;default:'active'"`
	TestedAt         *time.Time     `json:"tested_at"`
	RetiredAt        *time.Time     `json:"retired_at"`
	RevokedAt        *time.Time     `json:"revoked_at"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

const (
	GatewayCredentialPending = "pending"
	GatewayCredentialActive  = "active"
	GatewayCredentialFailed  = "failed"
	GatewayCredentialRetired = "retired"
	GatewayCredentialRevoked = "revoked"
)

var gatewayResourcesMu sync.Mutex

type GatewayModelRoute struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	ChannelID     int       `json:"channel_id" gorm:"uniqueIndex:idx_gateway_model_route,priority:1;index;not null"`
	PublicModel   string    `json:"public_model" gorm:"uniqueIndex:idx_gateway_model_route,priority:2;type:varchar(255);not null"`
	UpstreamModel string    `json:"upstream_model" gorm:"type:varchar(255);not null"`
	Capability    string    `json:"capability" gorm:"type:varchar(64)"`
	Protocol      string    `json:"protocol" gorm:"type:varchar(64)"`
	Group         string    `json:"group" gorm:"index"`
	PriceVersion  string    `json:"price_version" gorm:"type:varchar(64)"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type GatewayPolicy struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	ChannelID           int       `json:"channel_id" gorm:"uniqueIndex;not null"`
	ModelMapping        string    `json:"model_mapping" gorm:"type:text"`
	Need2ResponseModels string    `json:"need2response_models" gorm:"type:text"`
	ModelHeaders        string    `json:"model_headers" gorm:"type:text"`
	CustomParameter     string    `json:"custom_parameter" gorm:"type:text"`
	HeaderOverride      string    `json:"header_override" gorm:"type:text"`
	Plugin              string    `json:"plugin" gorm:"type:text"`
	DisabledStream      string    `json:"disabled_stream" gorm:"type:text"`
	OnlyChat            bool      `json:"only_chat"`
	PreCost             int       `json:"pre_cost"`
	CostRatio           float64   `json:"cost_ratio"`
	PassThroughBody     bool      `json:"pass_through_body"`
	CompatibleResponse  bool      `json:"compatible_response"`
	AllowExtraBody      bool      `json:"allow_extra_body"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type GatewayHealthState struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ChannelID          int       `json:"channel_id" gorm:"uniqueIndex;not null"`
	TestTime           int64     `json:"test_time"`
	ResponseTime       int       `json:"response_time"`
	Balance            float64   `json:"balance"`
	BalanceUpdatedTime int64     `json:"balance_updated_time"`
	UsedQuota          int64     `json:"used_quota"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type GatewayMigrationState struct {
	Name        string    `gorm:"primaryKey;type:varchar(96)"`
	CompletedAt time.Time `gorm:"not null"`
}

type GatewayProjectionReport struct {
	Channels           int64
	Endpoints          int64
	Credentials        int64
	Routes             int64
	Policies           int64
	HealthStates       int64
	InvalidActiveRefs  int64
	LegacyPlaintextKey int64
}

const gatewayCoreCutoverMigration = "gateway-core-cutover-v1"

func AutoMigrateGatewayResources(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&GatewayEndpoint{},
		&GatewayCredential{},
		&GatewayModelRoute{},
		&GatewayPolicy{},
		&GatewayHealthState{},
		&GatewayMigrationState{},
	); err != nil {
		return err
	}
	const legacyCredentialIndex = "idx_gateway_credentials_channel_id"
	if db.Migrator().HasIndex(&GatewayCredential{}, legacyCredentialIndex) {
		if err := db.Migrator().DropIndex(&GatewayCredential{}, legacyCredentialIndex); err != nil {
			return fmt.Errorf("drop legacy credential uniqueness: %w", err)
		}
	}
	return nil
}

func BackfillGatewayResources(db *gorm.DB, secretCipher *secret.Cipher) error {
	if secretCipher == nil {
		return secret.ErrMissingKey
	}
	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()
	return backfillGatewayResources(db, secretCipher)
}

func backfillGatewayResources(db *gorm.DB, secretCipher *secret.Cipher) error {
	var channels []Channel
	if err := db.Find(&channels).Error; err != nil {
		return fmt.Errorf("load channels for gateway migration: %w", err)
	}
	for index := range channels {
		channel := &channels[index]
		if err := db.Transaction(func(tx *gorm.DB) error {
			return backfillGatewayChannel(tx, secretCipher, channel)
		}); err != nil {
			return fmt.Errorf("migrate channel %d to gateway resources: %w", channel.Id, err)
		}
	}
	return pruneMissingGatewayResources(db)
}

func MigrateGatewayResources(db *gorm.DB) error {
	if err := AutoMigrateGatewayResources(db); err != nil {
		return fmt.Errorf("auto migrate gateway resources: %w", err)
	}
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return fmt.Errorf("initialize gateway credential cipher: %w", err)
	}
	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		var marker GatewayMigrationState
		err := tx.Where("name = ?", gatewayCoreCutoverMigration).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := backfillGatewayResources(tx, secretCipher); err != nil {
			return err
		}
		if _, err := ValidateGatewayResourceProjection(tx); err != nil {
			return err
		}
		return tx.Create(&GatewayMigrationState{
			Name:        gatewayCoreCutoverMigration,
			CompletedAt: time.Now(),
		}).Error
	})
}

func ValidateGatewayResourceProjection(db *gorm.DB) (GatewayProjectionReport, error) {
	report := GatewayProjectionReport{}
	counts := []struct {
		target any
		value  *int64
	}{
		{&Channel{}, &report.Channels},
		{&GatewayEndpoint{}, &report.Endpoints},
		{&GatewayCredential{}, &report.Credentials},
		{&GatewayModelRoute{}, &report.Routes},
		{&GatewayPolicy{}, &report.Policies},
		{&GatewayHealthState{}, &report.HealthStates},
	}
	for _, count := range counts {
		if err := db.Model(count.target).Count(count.value).Error; err != nil {
			return report, err
		}
	}
	if err := db.Model(&Channel{}).Where("key <> ''").Count(&report.LegacyPlaintextKey).Error; err != nil {
		return report, err
	}
	if err := db.Table("gateway_endpoints AS endpoint").
		Joins("LEFT JOIN gateway_credentials AS credential ON credential.id = endpoint.active_credential_id").
		Where("endpoint.active_credential_id = 0 OR credential.id IS NULL OR credential.channel_id <> endpoint.channel_id").
		Count(&report.InvalidActiveRefs).Error; err != nil {
		return report, err
	}
	if report.Endpoints != report.Channels ||
		report.Policies != report.Channels ||
		report.HealthStates != report.Channels ||
		report.Credentials < report.Channels ||
		report.InvalidActiveRefs != 0 ||
		report.LegacyPlaintextKey != 0 {
		return report, fmt.Errorf(
			"gateway projection validation failed: channels=%d endpoints=%d credentials=%d policies=%d health=%d invalid_active_refs=%d plaintext_keys=%d",
			report.Channels,
			report.Endpoints,
			report.Credentials,
			report.Policies,
			report.HealthStates,
			report.InvalidActiveRefs,
			report.LegacyPlaintextKey,
		)
	}
	return report, nil
}

func backfillGatewayChannel(db *gorm.DB, secretCipher *secret.Cipher, channel *Channel) error {
	weight := uint(1)
	if channel.Weight != nil {
		weight = *channel.Weight
	}
	priority := int64(0)
	if channel.Priority != nil {
		priority = *channel.Priority
	}

	var existingEndpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channel.Id).First(&existingEndpoint).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	endpoint := GatewayEndpoint{
		ChannelID:          channel.Id,
		ProviderID:         channel.Type,
		ProtocolProfileID:  channel.ProtocolProfileID,
		ActiveCredentialID: existingEndpoint.ActiveCredentialID,
		Name:               channel.Name,
		BaseURL:            stringValue(channel.BaseURL),
		Proxy:              stringValue(channel.Proxy),
		Other:              channel.Other,
		Remark:             channel.Remark,
		TestModel:          channel.TestModel,
		Group:              channel.Group,
		Tags:               channel.Tag,
		Status:             channel.Status,
		Weight:             weight,
		Priority:           priority,
		CreatedTime:        channel.CreatedTime,
	}
	if err := upsertByChannelID(db, &endpoint); err != nil {
		return err
	}

	activeCredentialID, err := upsertGatewayCredential(db, secretCipher, channel, endpoint.ActiveCredentialID)
	if err != nil {
		return err
	}
	if endpoint.ActiveCredentialID != activeCredentialID {
		if err := db.Model(&GatewayEndpoint{}).
			Where("channel_id = ?", channel.Id).
			Update("active_credential_id", activeCredentialID).Error; err != nil {
			return err
		}
	}
	if channel.Key != "" {
		if err := db.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Update("key", "").Error; err != nil {
			return err
		}
	}
	if err := replaceGatewayModelRoutes(db, channel); err != nil {
		return err
	}

	policy := GatewayPolicy{
		ChannelID:           channel.Id,
		ModelMapping:        stringValue(channel.ModelMapping),
		Need2ResponseModels: stringValue(channel.Need2ResponseModels),
		ModelHeaders:        stringValue(channel.ModelHeaders),
		CustomParameter:     stringValue(channel.CustomParameter),
		HeaderOverride:      stringValue(channel.HeaderOverride),
		Plugin:              jsonValue(channel.Plugin),
		DisabledStream:      jsonValue(channel.DisabledStream),
		OnlyChat:            channel.OnlyChat,
		PreCost:             channel.PreCost,
		CostRatio:           float64Value(channel.CostRatio),
		PassThroughBody:     channel.PassThroughBody,
		CompatibleResponse:  channel.CompatibleResponse,
		AllowExtraBody:      channel.AllowExtraBody,
	}
	if err := upsertByChannelID(db, &policy); err != nil {
		return err
	}

	health := GatewayHealthState{
		ChannelID:          channel.Id,
		TestTime:           channel.TestTime,
		ResponseTime:       channel.ResponseTime,
		Balance:            channel.Balance,
		BalanceUpdatedTime: channel.BalanceUpdatedTime,
		UsedQuota:          channel.UsedQuota,
	}
	return upsertByChannelID(db, &health)
}

func upsertGatewayCredential(db *gorm.DB, secretCipher *secret.Cipher, channel *Channel, activeCredentialID uint) (uint, error) {
	var existing GatewayCredential
	query := db.Where("channel_id = ?", channel.Id)
	if activeCredentialID != 0 {
		query = query.Where("id = ?", activeCredentialID)
	} else {
		query = query.Order("secret_version DESC, id DESC")
	}
	err := query.First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if err == nil && activeCredentialID != 0 {
		if existing.SecretCiphertext == "" {
			return 0, errors.New("gateway active credential ciphertext is empty")
		}
		if _, decryptErr := secretCipher.Decrypt(existing.SecretCiphertext); decryptErr != nil {
			return 0, decryptErr
		}
		if existing.Status != GatewayCredentialActive {
			if updateErr := db.Model(&existing).Updates(map[string]any{
				"status":     GatewayCredentialActive,
				"retired_at": nil,
			}).Error; updateErr != nil {
				return 0, updateErr
			}
		}
		return existing.ID, nil
	}
	if err == nil && existing.SecretCiphertext != "" {
		plaintext, decryptErr := secretCipher.Decrypt(existing.SecretCiphertext)
		if decryptErr != nil {
			return 0, decryptErr
		}
		if plaintext == channel.Key {
			if existing.Status != "active" {
				if updateErr := db.Model(&existing).Updates(map[string]any{"status": "active", "retired_at": nil}).Error; updateErr != nil {
					return 0, updateErr
				}
			}
			return existing.ID, nil
		}
	}

	ciphertext, err := secretCipher.Encrypt(channel.Key)
	if err != nil {
		return 0, err
	}
	var maxVersion int
	if err := db.Model(&GatewayCredential{}).
		Where("channel_id = ?", channel.Id).
		Select("COALESCE(MAX(secret_version), 0)").
		Scan(&maxVersion).Error; err != nil {
		return 0, err
	}
	credential := GatewayCredential{
		ChannelID:        channel.Id,
		AuthMode:         gatewayAuthMode(channel.Key),
		SecretCiphertext: ciphertext,
		SecretVersion:    maxVersion + 1,
		Status:           "active",
	}
	if channel.TestTime > 0 {
		testedAt := time.Unix(channel.TestTime, 0)
		credential.TestedAt = &testedAt
	}
	if err := db.Create(&credential).Error; err != nil {
		return 0, err
	}
	if existing.ID != 0 {
		retiredAt := time.Now()
		if err := db.Model(&GatewayCredential{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{"status": "retired", "retired_at": &retiredAt}).Error; err != nil {
			return 0, err
		}
	}
	return credential.ID, nil
}

func gatewayAuthMode(plaintext string) string {
	if plaintext == "" {
		return string(domain.AuthModeNone)
	}
	return string(domain.AuthModeOpaque)
}

func replaceGatewayModelRoutes(db *gorm.DB, channel *Channel) error {
	mapping := make(map[string]string)
	if raw := stringValue(channel.ModelMapping); raw != "" {
		_ = json.Unmarshal([]byte(raw), &mapping)
	}

	models := splitNonEmpty(channel.Models)
	if len(models) == 0 {
		return db.Where("channel_id = ?", channel.Id).Delete(&GatewayModelRoute{}).Error
	}
	for _, publicModel := range models {
		upstreamModel := publicModel
		if mapped := strings.TrimSpace(mapping[publicModel]); mapped != "" {
			upstreamModel = mapped
		}
		route := GatewayModelRoute{
			ChannelID:     channel.Id,
			PublicModel:   publicModel,
			UpstreamModel: upstreamModel,
			Group:         channel.Group,
		}
		if protocol, ok := domain.ProtocolForProfile(domain.ProtocolProfileID(channel.ProtocolProfileID)); ok {
			route.Protocol = string(protocol)
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "channel_id"}, {Name: "public_model"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"upstream_model",
				"protocol",
				"group",
				"updated_at",
			}),
		}).Create(&route).Error; err != nil {
			return err
		}
	}
	return db.Where("channel_id = ? AND public_model NOT IN ?", channel.Id, models).
		Delete(&GatewayModelRoute{}).Error
}

func upsertByChannelID[T any](db *gorm.DB, value *T) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		UpdateAll: true,
	}).Create(value).Error
}

func pruneMissingGatewayResources(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		activeChannels := tx.Model(&Channel{}).Select("id")
		for _, resource := range []any{
			&GatewayModelRoute{},
			&GatewayPolicy{},
			&GatewayHealthState{},
			&GatewayCredential{},
			&GatewayEndpoint{},
		} {
			if err := tx.Where("channel_id NOT IN (?)", activeChannels).Delete(resource).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func SyncGatewayChannels(db *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 {
		return nil
	}
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return err
	}

	uniqueIDs := make([]int, 0, len(channelIDs))
	seen := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		uniqueIDs = append(uniqueIDs, channelID)
	}
	if len(uniqueIDs) == 0 {
		return nil
	}

	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()

	return db.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Where("id IN ?", uniqueIDs).Find(&channels).Error; err != nil {
			return err
		}
		found := make(map[int]struct{}, len(channels))
		for index := range channels {
			channel := &channels[index]
			found[channel.Id] = struct{}{}
			if err := backfillGatewayChannel(tx, secretCipher, channel); err != nil {
				return fmt.Errorf("sync gateway channel %d: %w", channel.Id, err)
			}
		}
		for _, channelID := range uniqueIDs {
			if _, exists := found[channelID]; exists {
				continue
			}
			if err := DeleteGatewayChannelResources(tx, channelID); err != nil {
				return fmt.Errorf("delete gateway channel %d: %w", channelID, err)
			}
		}
		return nil
	})
}

// DeleteGatewayChannelResources removes the complete runtime projection for a
// channel. Passing an existing transaction keeps the source row and its
// endpoint, credential, route, policy and health records atomic.
func DeleteGatewayChannelResources(db *gorm.DB, channelID int) error {
	if db == nil {
		return errors.New("database is nil")
	}
	if channelID <= 0 {
		return errors.New("channel id must be positive")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, resource := range []any{
			&GatewayModelRoute{},
			&GatewayPolicy{},
			&GatewayHealthState{},
			&GatewayCredential{},
			&GatewayEndpoint{},
		} {
			if err := tx.Where("channel_id = ?", channelID).Delete(resource).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func SyncGatewayEndpointStatus(db *gorm.DB, channelID int, status int) error {
	return db.Model(&GatewayEndpoint{}).
		Where("channel_id = ?", channelID).
		Update("status", status).
		Error
}

func ListGatewayCredentials(db *gorm.DB, channelID int) ([]GatewayCredential, error) {
	var credentials []GatewayCredential
	err := db.
		Where("channel_id = ?", channelID).
		Order("secret_version DESC, id DESC").
		Find(&credentials).Error
	return credentials, err
}

func CreatePendingGatewayCredential(db *gorm.DB, channelID int, authMode, plaintext string) (*GatewayCredential, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil, errors.New("凭据不能为空")
	}
	if authMode == "" {
		authMode = string(domain.AuthModeOpaque)
	}
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return nil, err
	}
	ciphertext, err := secretCipher.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}

	var credential GatewayCredential
	err = db.Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ?", channelID).
			First(&endpoint).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&GatewayCredential{}).
			Where("channel_id = ?", channelID).
			Select("COALESCE(MAX(secret_version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}
		credential = GatewayCredential{
			ChannelID:        channelID,
			AuthMode:         authMode,
			SecretCiphertext: ciphertext,
			SecretVersion:    maxVersion + 1,
			Status:           GatewayCredentialPending,
		}
		return tx.Create(&credential).Error
	})
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func LoadGatewayCredentialSecret(db *gorm.DB, channelID int, credentialID uint) (*GatewayCredential, string, error) {
	var credential GatewayCredential
	if err := db.
		Where("id = ? AND channel_id = ?", credentialID, channelID).
		First(&credential).Error; err != nil {
		return nil, "", err
	}
	if credential.Status == GatewayCredentialRevoked {
		return nil, "", errors.New("凭据已撤销")
	}
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return nil, "", err
	}
	plaintext, err := secretCipher.Decrypt(credential.SecretCiphertext)
	if err != nil {
		return nil, "", err
	}
	return &credential, plaintext, nil
}

func LoadActiveGatewayCredentialSecret(db *gorm.DB, channelID int) (*GatewayCredential, string, error) {
	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channelID).First(&endpoint).Error; err != nil {
		return nil, "", err
	}
	if endpoint.ActiveCredentialID == 0 {
		return nil, "", gorm.ErrRecordNotFound
	}
	return LoadGatewayCredentialSecret(db, channelID, endpoint.ActiveCredentialID)
}

func MarkGatewayCredentialTestResult(db *gorm.DB, channelID int, credentialID uint, passed bool) error {
	updates := map[string]any{
		"tested_at": nil,
		"status":    GatewayCredentialFailed,
	}
	if passed {
		testedAt := time.Now()
		updates["tested_at"] = &testedAt
		updates["status"] = GatewayCredentialPending
	}
	result := db.Model(&GatewayCredential{}).
		Where("id = ? AND channel_id = ? AND status IN ?", credentialID, channelID, []string{
			GatewayCredentialPending,
			GatewayCredentialFailed,
		}).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("凭据不存在或当前状态不可测试")
	}
	return nil
}

func ActivateGatewayCredential(db *gorm.DB, channelID int, credentialID uint) error {
	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ?", channelID).
			First(&endpoint).Error; err != nil {
			return err
		}
		var credential GatewayCredential
		if err := tx.
			Where("id = ? AND channel_id = ?", credentialID, channelID).
			First(&credential).Error; err != nil {
			return err
		}
		if credential.Status != GatewayCredentialPending || credential.TestedAt == nil {
			return errors.New("凭据必须先通过测试才能激活")
		}
		now := time.Now()
		if endpoint.ActiveCredentialID != credential.ID {
			if err := tx.Model(&GatewayCredential{}).
				Where("channel_id = ? AND id <> ? AND status = ?", channelID, credential.ID, GatewayCredentialActive).
				Updates(map[string]any{
					"status":     GatewayCredentialRetired,
					"retired_at": &now,
				}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&credential).Updates(map[string]any{
			"status":     GatewayCredentialActive,
			"retired_at": nil,
			"revoked_at": nil,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&endpoint).Update("active_credential_id", credential.ID).Error; err != nil {
			return err
		}
		return tx.Model(&Channel{}).Where("id = ?", channelID).Update("key", "").Error
	})
}

func ReplaceActiveGatewayCredential(db *gorm.DB, channelID int, plaintext string) error {
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return err
	}
	ciphertext, err := secretCipher.Encrypt(plaintext)
	if err != nil {
		return err
	}

	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ?", channelID).
			First(&endpoint).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&GatewayCredential{}).
			Where("channel_id = ?", channelID).
			Select("COALESCE(MAX(secret_version), 0)").
			Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&GatewayCredential{}).
			Where("channel_id = ? AND status = ?", channelID, GatewayCredentialActive).
			Updates(map[string]any{
				"status":     GatewayCredentialRetired,
				"retired_at": &now,
			}).Error; err != nil {
			return err
		}
		credential := GatewayCredential{
			ChannelID:        channelID,
			AuthMode:         gatewayAuthMode(plaintext),
			SecretCiphertext: ciphertext,
			SecretVersion:    maxVersion + 1,
			Status:           GatewayCredentialActive,
			TestedAt:         &now,
		}
		if err := tx.Create(&credential).Error; err != nil {
			return err
		}
		if err := tx.Model(&endpoint).Update("active_credential_id", credential.ID).Error; err != nil {
			return err
		}
		return tx.Model(&Channel{}).Where("id = ?", channelID).Update("key", "").Error
	})
}

func RevokeGatewayCredential(db *gorm.DB, channelID int, credentialID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_id = ?", channelID).
			First(&endpoint).Error; err != nil {
			return err
		}
		if endpoint.ActiveCredentialID == credentialID {
			return errors.New("不能撤销当前活动凭据，请先激活替代凭据")
		}
		now := time.Now()
		result := tx.Model(&GatewayCredential{}).
			Where("id = ? AND channel_id = ? AND status <> ?", credentialID, channelID, GatewayCredentialRevoked).
			Updates(map[string]any{
				"status":     GatewayCredentialRevoked,
				"revoked_at": &now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("凭据不存在或已经撤销")
		}
		return nil
	})
}

func splitNonEmpty(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func jsonValue(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func LoadGatewayChannelSnapshots(db *gorm.DB) ([]*Channel, error) {
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return nil, err
	}

	var endpoints []GatewayEndpoint
	if err := db.Where("status = ?", 1).Find(&endpoints).Error; err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return []*Channel{}, nil
	}

	channelIDs := make([]int, 0, len(endpoints))
	activeCredentialIDs := make([]uint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		channelIDs = append(channelIDs, endpoint.ChannelID)
		if endpoint.ActiveCredentialID == 0 {
			return nil, fmt.Errorf("gateway active credential missing for channel %d", endpoint.ChannelID)
		}
		activeCredentialIDs = append(activeCredentialIDs, endpoint.ActiveCredentialID)
	}

	var credentials []GatewayCredential
	var routes []GatewayModelRoute
	var policies []GatewayPolicy
	var healthStates []GatewayHealthState
	if err := db.Where("id IN ?", activeCredentialIDs).Find(&credentials).Error; err != nil {
		return nil, err
	}
	if err := db.Where("channel_id IN ?", channelIDs).
		Order("channel_id ASC, id ASC").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	if err := db.Where("channel_id IN ?", channelIDs).Find(&policies).Error; err != nil {
		return nil, err
	}
	if err := db.Where("channel_id IN ?", channelIDs).Find(&healthStates).Error; err != nil {
		return nil, err
	}

	credentialsByID := make(map[uint]GatewayCredential, len(credentials))
	for _, credential := range credentials {
		credentialsByID[credential.ID] = credential
	}
	routesByChannel := make(map[int][]GatewayModelRoute)
	for _, route := range routes {
		routesByChannel[route.ChannelID] = append(routesByChannel[route.ChannelID], route)
	}
	policiesByChannel := make(map[int]GatewayPolicy, len(policies))
	for _, policy := range policies {
		policiesByChannel[policy.ChannelID] = policy
	}
	healthByChannel := make(map[int]GatewayHealthState, len(healthStates))
	for _, health := range healthStates {
		healthByChannel[health.ChannelID] = health
	}

	channels := make([]*Channel, 0, len(endpoints))
	for _, endpoint := range endpoints {
		credential, exists := credentialsByID[endpoint.ActiveCredentialID]
		if !exists {
			return nil, fmt.Errorf("gateway active credential %d missing for channel %d", endpoint.ActiveCredentialID, endpoint.ChannelID)
		}
		key, err := secretCipher.Decrypt(credential.SecretCiphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypt gateway credential for channel %d: %w", endpoint.ChannelID, err)
		}
		policy := policiesByChannel[endpoint.ChannelID]
		health := healthByChannel[endpoint.ChannelID]
		weight := endpoint.Weight
		priority := endpoint.Priority
		costRatio := policy.CostRatio
		baseURL := endpoint.BaseURL
		proxy := endpoint.Proxy
		modelMapping := policy.ModelMapping
		need2ResponseModels := policy.Need2ResponseModels
		modelHeaders := policy.ModelHeaders
		customParameter := policy.CustomParameter
		headerOverride := policy.HeaderOverride

		channel := &Channel{
			Id:                  endpoint.ChannelID,
			Type:                endpoint.ProviderID,
			ProtocolProfileID:   endpoint.ProtocolProfileID,
			Key:                 key,
			Status:              endpoint.Status,
			Name:                endpoint.Name,
			Weight:              &weight,
			CreatedTime:         endpoint.CreatedTime,
			TestTime:            health.TestTime,
			ResponseTime:        health.ResponseTime,
			BaseURL:             &baseURL,
			Other:               endpoint.Other,
			Remark:              endpoint.Remark,
			Balance:             health.Balance,
			BalanceUpdatedTime:  health.BalanceUpdatedTime,
			Models:              joinPublicModels(routesByChannel[endpoint.ChannelID]),
			Group:               endpoint.Group,
			Tag:                 endpoint.Tags,
			UsedQuota:           health.UsedQuota,
			ModelMapping:        &modelMapping,
			Need2ResponseModels: &need2ResponseModels,
			ModelHeaders:        &modelHeaders,
			CustomParameter:     &customParameter,
			Priority:            &priority,
			Proxy:               &proxy,
			TestModel:           endpoint.TestModel,
			OnlyChat:            policy.OnlyChat,
			PreCost:             policy.PreCost,
			CostRatio:           &costRatio,
			HeaderOverride:      &headerOverride,
			PassThroughBody:     policy.PassThroughBody,
			CompatibleResponse:  policy.CompatibleResponse,
			AllowExtraBody:      policy.AllowExtraBody,
		}
		if err := restoreGatewayJSONPolicy(channel, policy); err != nil {
			return nil, fmt.Errorf("restore gateway policy for channel %d: %w", endpoint.ChannelID, err)
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func joinPublicModels(routes []GatewayModelRoute) string {
	models := make([]string, 0, len(routes))
	for _, route := range routes {
		models = append(models, route.PublicModel)
	}
	return strings.Join(models, ",")
}

func restoreGatewayJSONPolicy(channel *Channel, policy GatewayPolicy) error {
	if policy.Plugin != "" {
		var plugin PluginType
		if err := json.Unmarshal([]byte(policy.Plugin), &plugin); err != nil {
			return err
		}
		value := datatypes.NewJSONType(plugin)
		channel.Plugin = &value
	}
	if policy.DisabledStream != "" {
		var disabledStream []string
		if err := json.Unmarshal([]byte(policy.DisabledStream), &disabledStream); err != nil {
			return err
		}
		value := datatypes.JSONSlice[string](disabledStream)
		channel.DisabledStream = &value
	}
	return nil
}
