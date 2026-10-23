package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"done-hub/internal/gateway/secret"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	KeyStrategyRoundRobin = "round_robin"
	KeyStrategyRandom     = "random"
)

// ChannelKey 是统一保存表单和请求运行时使用的凭据，不包含轮换历史。
type ChannelKey struct {
	ID      uint   `json:"id,omitempty"`
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

var credentialCursors sync.Map
var credentialCooldowns sync.Map

// 在允许编辑多 Key 之前，为旧任务固定升级时正在使用的凭据。
func BackfillTaskCredentialReferences(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"tasks", "midjourneys"} {
			if err := tx.Table(table).
				Where("(credential_id IS NULL OR credential_id = 0) AND channel_id IN (?)",
					tx.Model(&GatewayEndpoint{}).Select("channel_id").Where("active_credential_id <> 0")).
				Update("credential_id", gorm.Expr("(SELECT active_credential_id FROM gateway_endpoints WHERE gateway_endpoints.channel_id = "+table+".channel_id AND gateway_endpoints.deleted_at IS NULL)")).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (channel *Channel) HasEnabledCredential() bool {
	if channel.CredentialPool == nil {
		return true
	}
	for _, key := range channel.CredentialPool {
		if key.Enabled && !gatewayCredentialInCooldown(key.ID) {
			return true
		}
	}
	return false
}

func ChannelKeysNeedProbe(keys, existing []ChannelKey) bool {
	known := make(map[string]bool, len(existing))
	for _, key := range existing {
		known[key.Secret] = true
	}
	for _, key := range keys {
		if key.Enabled && !known[key.Secret] {
			return true
		}
	}
	return false
}

func normalizeKeyStrategy(strategy string) string {
	if strategy == "" {
		return KeyStrategyRoundRobin
	}
	return strategy
}

func ValidateChannelKeys(channel *Channel, allowEmpty bool) error {
	channel.KeyStrategy = normalizeKeyStrategy(channel.KeyStrategy)
	if channel.KeyStrategy != KeyStrategyRoundRobin && channel.KeyStrategy != KeyStrategyRandom {
		return errors.New("调用策略只支持轮询或随机")
	}
	if channel.Keys == nil {
		return nil
	}
	if len(channel.Keys) == 0 || len(channel.Keys) > 1000 {
		return errors.New("请配置 1 至 1000 个 Key")
	}
	seen := make(map[string]bool, len(channel.Keys))
	for index := range channel.Keys {
		key := &channel.Keys[index]
		key.Secret = strings.TrimSpace(key.Secret)
		if key.Secret == "" && !allowEmpty {
			return fmt.Errorf("第 %d 个 Key 不能为空", index+1)
		}
		if seen[key.Secret] {
			return fmt.Errorf("第 %d 个 Key 与其他 Key 重复", index+1)
		}
		seen[key.Secret] = true
	}
	channel.Key = channel.Keys[0].Secret
	return nil
}

func LoadGatewayKeyPool(db *gorm.DB, channelID int) ([]ChannelKey, string, error) {
	var endpoint GatewayEndpoint
	if err := db.Where("channel_id = ?", channelID).First(&endpoint).Error; err != nil {
		return nil, "", err
	}
	var credentials []GatewayCredential
	if err := db.Where("channel_id = ? AND status = ?", channelID, GatewayCredentialActive).
		Order("position ASC, id ASC").Find(&credentials).Error; err != nil {
		return nil, "", err
	}
	cipher, err := secret.NewFromConfig()
	if err != nil {
		return nil, "", err
	}
	keys := make([]ChannelKey, 0, len(credentials))
	for _, credential := range credentials {
		plaintext, err := cipher.Decrypt(credential.SecretCiphertext)
		if err != nil {
			return nil, "", err
		}
		keys = append(keys, ChannelKey{ID: credential.ID, Secret: plaintext, Enabled: !credential.Disabled})
	}
	return keys, normalizeKeyStrategy(endpoint.KeyStrategy), nil
}

// saveGatewayKeyPool 与渠道配置共用外层事务；旧轮换历史不会被重新激活。
func saveGatewayKeyPool(db *gorm.DB, channel *Channel) error {
	gatewayResourcesMu.Lock()
	defer gatewayResourcesMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ?", channel.Id).First(&endpoint).Error; err != nil {
			return err
		}
		var existing []GatewayCredential
		if err := tx.Where("channel_id = ? AND status = ?", channel.Id, GatewayCredentialActive).Find(&existing).Error; err != nil {
			return err
		}
		cipher, err := secret.NewFromConfig()
		if err != nil {
			return err
		}
		byID := make(map[uint]GatewayCredential, len(existing))
		bySecret := make(map[string]GatewayCredential, len(existing))
		for _, item := range existing {
			plaintext, err := cipher.Decrypt(item.SecretCiphertext)
			if err != nil {
				return err
			}
			byID[item.ID], bySecret[plaintext] = item, item
		}
		var maxVersion int
		if err := tx.Model(&GatewayCredential{}).Where("channel_id = ?", channel.Id).
			Select("COALESCE(MAX(secret_version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		kept := make([]uint, 0, len(channel.Keys))
		for index, key := range channel.Keys {
			if key.ID != 0 {
				if _, ok := byID[key.ID]; !ok {
					return errors.New("Key 不属于当前渠道或已被删除，请刷新后重试")
				}
			}
			item, sameSecret := bySecret[key.Secret]
			if !sameSecret {
				ciphertext, err := cipher.Encrypt(key.Secret)
				if err != nil {
					return err
				}
				maxVersion++
				item = GatewayCredential{ChannelID: channel.Id, AuthMode: gatewayAuthMode(key.Secret), SecretCiphertext: ciphertext,
					SecretVersion: maxVersion, Status: GatewayCredentialActive, Disabled: !key.Enabled, Position: index}
				if key.Enabled && channel.TestTime > 0 {
					testedAt := time.Unix(channel.TestTime, 0)
					item.TestedAt = &testedAt
				}
				if err := tx.Create(&item).Error; err != nil {
					return err
				}
			} else if err := tx.Model(&item).Updates(map[string]any{"disabled": !key.Enabled, "position": index}).Error; err != nil {
				return err
			}
			kept = append(kept, item.ID)
		}
		if len(kept) == 0 {
			return errors.New("请至少保留一个 Key")
		}
		now := time.Now()
		if err := tx.Model(&GatewayCredential{}).Where("channel_id = ? AND status = ? AND id NOT IN ?", channel.Id, GatewayCredentialActive, kept).
			Updates(map[string]any{"status": GatewayCredentialRetired, "retired_at": &now}).Error; err != nil {
			return err
		}
		primary := kept[0]
		for index, key := range channel.Keys {
			if key.Enabled {
				primary = kept[index]
				break
			}
		}
		return tx.Model(&endpoint).Updates(map[string]any{"active_credential_id": primary, "key_strategy": normalizeKeyStrategy(channel.KeyStrategy)}).Error
	})
}

// SelectGatewayCredential 返回请求私有的 Channel，绝不修改共享路由快照。
func SelectGatewayCredential(channel *Channel) (*Channel, error) {
	selected := *channel
	if channel.CredentialPool == nil {
		return &selected, nil
	}
	available := make([]ChannelKey, 0, len(channel.CredentialPool))
	for _, key := range channel.CredentialPool {
		if key.Enabled && !gatewayCredentialInCooldown(key.ID) {
			available = append(available, key)
		}
	}
	if len(available) == 0 {
		return nil, errors.New("渠道没有已启用的 Key")
	}
	index := 0
	if channel.KeyStrategy == KeyStrategyRandom {
		index = rand.Intn(len(available))
	} else {
		value, _ := credentialCursors.LoadOrStore(channel.Id, &atomic.Uint64{})
		index = int((value.(*atomic.Uint64).Add(1) - 1) % uint64(len(available)))
	}
	selected.Key, selected.CredentialID = available[index].Secret, available[index].ID
	selected.CredentialPool = nil
	return &selected, nil
}

func PinGatewayCredential(channel *Channel, credentialID uint) (*Channel, error) {
	selected := *channel
	selected.CredentialPool = nil
	if credentialID == 0 {
		return &selected, nil
	} // 兼容已有异步任务。
	_, plaintext, err := LoadGatewayCredentialSecret(DB, channel.Id, credentialID)
	if err != nil {
		return nil, err
	}
	selected.Key, selected.CredentialID = plaintext, credentialID
	return &selected, nil
}

func (channel *Channel) CredentialCacheKey(prefix string) string {
	digest := sha256.Sum256([]byte(channel.Key))
	return fmt.Sprintf("%s:%d:%d:%x", prefix, channel.Id, channel.CredentialID, digest[:8])
}

func gatewayCredentialInCooldown(id uint) bool {
	if id == 0 {
		return false
	}
	value, exists := credentialCooldowns.Load(id)
	return exists && time.Now().Before(value.(time.Time))
}

// 多 Key 的失败只冷却本次 Key，其他 Key 仍可被后续重试选择。
func CooldownGatewayCredential(channelID int, credentialID uint, seconds int64) bool {
	channel := GatewayRoutes.GetChannel(channelID)
	if channel == nil || len(channel.CredentialPool) < 2 || credentialID == 0 {
		return false
	}
	if seconds < 1 {
		seconds = 1
	}
	credentialCooldowns.Store(credentialID, time.Now().Add(time.Duration(seconds)*time.Second))
	return true
}

func DisableGatewayCredential(channelID int, credentialID uint) error {
	result := DB.Model(&GatewayCredential{}).Where("channel_id = ? AND id = ? AND status = ?", channelID, credentialID, GatewayCredentialActive).
		Update("disabled", true)
	if result.Error != nil {
		return result.Error
	}
	GatewayRoutes.Load()
	return nil
}

// SaveRefreshedCredential 只刷新本次请求实际使用的 Key，不替换其他 Key。
func (channel *Channel) SaveRefreshedCredential(plaintext string) error {
	if channel.CredentialReadOnly || channel.Id == 0 {
		return nil
	}
	if channel.CredentialID == 0 {
		return UpdateChannelKey(channel.Id, plaintext)
	}
	cipher, err := secret.NewFromConfig()
	if err != nil {
		return err
	}
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		return err
	}
	result := DB.Model(&GatewayCredential{}).Where("id = ? AND channel_id = ? AND status <> ?", channel.CredentialID, channel.Id, GatewayCredentialRevoked).
		Update("secret_ciphertext", ciphertext)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	GatewayRoutes.Load()
	return nil
}
