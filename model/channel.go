package model

import (
	"crypto/md5"
	"done-hub/common/cache"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/utils"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/secret"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Channel struct {
	Id                   int        `json:"id"`
	Type                 int        `json:"type" form:"type" gorm:"default:0"`
	ProtocolProfileID    string     `json:"protocol_profile_id" form:"protocol_profile_id" gorm:"type:varchar(64);index"`
	Key                  string     `json:"key" form:"key" gorm:"type:text"`
	CredentialConfigured bool       `json:"credential_configured" gorm:"-"`
	CredentialAuthMode   string     `json:"credential_auth_mode,omitempty" gorm:"-"`
	CredentialStatus     string     `json:"credential_status" gorm:"-"`
	CredentialTestedAt   *time.Time `json:"credential_tested_at,omitempty" gorm:"-"`
	ValidationToken      string     `json:"validation_token,omitempty" gorm:"-"`
	Status               int        `json:"status" form:"status" gorm:"default:1"`
	Name                 string     `json:"name" form:"name" gorm:"index"`
	Weight               *uint      `json:"weight" gorm:"default:1"`
	CreatedTime          int64      `json:"created_time" gorm:"bigint"`
	TestTime             int64      `json:"test_time" gorm:"bigint"`
	ResponseTime         int        `json:"response_time"` // in milliseconds
	BaseURL              *string    `json:"base_url" gorm:"column:base_url;default:''"`
	Other                string     `json:"other" form:"other"`
	Remark               string     `json:"remark" form:"remark" gorm:"type:text"`
	Balance              float64    `json:"balance"` // in USD
	BalanceUpdatedTime   int64      `json:"balance_updated_time" gorm:"bigint"`
	Models               string     `json:"models" form:"models"`
	Group                string     `json:"group" form:"group" gorm:"type:varchar(255);default:'default'"`
	Tag                  string     `json:"tag" form:"tag" gorm:"type:varchar(32);default:''"`
	UsedQuota            int64      `json:"used_quota" gorm:"bigint;default:0"`
	ModelMapping         *string    `json:"model_mapping" gorm:"type:text"`
	Need2ResponseModels  *string    `json:"need2response_models" gorm:"type:text"`
	ModelHeaders         *string    `json:"model_headers" gorm:"type:varchar(1024);default:''"`
	CustomParameter      *string    `json:"custom_parameter" gorm:"type:text"`
	Priority             *int64     `json:"priority" gorm:"bigint;default:0"`
	Proxy                *string    `json:"proxy" gorm:"type:varchar(255);default:''"`
	TestModel            string     `json:"test_model" form:"test_model" gorm:"type:varchar(50);default:''"`
	OnlyChat             bool       `json:"only_chat" form:"only_chat" gorm:"default:false"`
	PreCost              int        `json:"pre_cost" form:"pre_cost" gorm:"default:1"`
	CostRatio            *float64   `json:"cost_ratio" form:"cost_ratio" gorm:"type:decimal(10,4);default:0"`
	HeaderOverride       *string    `json:"header_override" gorm:"type:text"`
	PassThroughBody      bool       `json:"pass_through_body" form:"pass_through_body" gorm:"default:false"`
	CompatibleResponse   bool       `json:"compatible_response" gorm:"default:false"`
	AllowExtraBody       bool       `json:"allow_extra_body" form:"allow_extra_body" gorm:"default:false"`

	DisabledStream *datatypes.JSONSlice[string] `json:"disabled_stream,omitempty" gorm:"type:json"`

	Plugin    *datatypes.JSONType[PluginType] `json:"plugin" form:"plugin" gorm:"type:json"`
	DeletedAt gorm.DeletedAt                  `json:"-" gorm:"index"`
}

func (c *Channel) AllowStream(modelName string) bool {
	if c.DisabledStream == nil {
		return true
	}

	return !slices.Contains(*c.DisabledStream, modelName)
}

type PluginType map[string]map[string]interface{}

var allowedChannelOrderFields = map[string]bool{
	"id":            true,
	"name":          true,
	"group":         true,
	"type":          true,
	"status":        true,
	"response_time": true,
	"balance":       true,
	"used_quota":    true,
	"priority":      true,
	"weight":        true,
	"cost_ratio":    true,
}

type SearchChannelsParams struct {
	Channel
	PaginationParams
	FilterTag int    `json:"filter_tag" form:"filter_tag"`
	BaseURL   string `json:"base_url" form:"base_url"`
}

func GetChannelsList(params *SearchChannelsParams) (*DataResult[Channel], error) {
	var channels []*Channel

	db := DB.Omit("key")
	// 代表行取组内最小 id，与 GetChannelsTag 取 channels[0]（Order id ASC）保持一致
	tagDB := DB.Model(&Channel{}).Select("MIN(id) as id").Where("tag != ''").Group("tag")

	if params.Type != 0 {
		db = db.Where("type = ?", params.Type)
		tagDB = tagDB.Where("type = ?", params.Type)
	}

	if params.Status != 0 {
		db = db.Where("status = ?", params.Status)
		tagDB = tagDB.Where("status = ?", params.Status)
	}

	if params.Name != "" {
		like := "%" + params.Name + "%"
		db = db.Where("name LIKE ? OR tag LIKE ?", like, like)
		tagDB = tagDB.Where("name LIKE ? OR tag LIKE ?", like, like)
	}

	if params.Group != "" {
		groupKey := quotePostgresField("group")
		db = db.Where("( "+groupKey+" LIKE ? OR "+groupKey+" LIKE ? OR "+groupKey+" LIKE ? OR "+groupKey+" = ?)",
			"%,"+params.Group+",%", params.Group+",%", "%,"+params.Group, params.Group)
		tagDB = tagDB.Where("( "+groupKey+" LIKE ? OR "+groupKey+" LIKE ? OR "+groupKey+" LIKE ? OR "+groupKey+" = ?)",
			"%,"+params.Group+",%", params.Group+",%", "%,"+params.Group, params.Group)
	}

	if params.Models != "" {
		db = db.Where("models LIKE ?", "%"+params.Models+"%")
		tagDB = tagDB.Where("models LIKE ?", "%"+params.Models+"%")
	}

	if params.Other != "" {
		db = db.Where("other LIKE ?", params.Other+"%")
		tagDB = tagDB.Where("other LIKE ?", params.Other+"%")
	}

	if params.Key != "" {
		db = db.Where(quotePostgresField("key")+" = ?", params.Key)
		tagDB = tagDB.Where(quotePostgresField("key")+" = ?", params.Key)
	}

	if params.TestModel != "" {
		db = db.Where("test_model LIKE ?", params.TestModel+"%")
		tagDB = tagDB.Where("test_model LIKE ?", params.TestModel+"%")
	}

	if params.BaseURL != "" {
		db = db.Where("base_url LIKE ?", "%"+params.BaseURL+"%")
		tagDB = tagDB.Where("base_url LIKE ?", "%"+params.BaseURL+"%")
	}

	if params.Tag != "" {
		db = db.Where("tag = ?", params.Tag)
		tagDB = tagDB.Where("tag = ?", params.Tag)
	}

	switch params.FilterTag {
	case 1:
		db = db.Where("tag = ''")
	case 2:
		db = db.Where("id IN (?)", tagDB)
	default:
		db = db.Where("tag = '' OR id IN (?)", tagDB)
	}

	// 标签代表行只是组内最小 id 的那条渠道，默认排序会按它「自身」的列值排位；
	// 但「已使用/余额/响应时间」展示的是整组合计/平均、「名称」展示的是标签名，
	// 直接按自身值排会与显示值错位。这里对这几列改用标签感知表达式：
	// 代表行按整组聚合排序、普通渠道仍按自身值，使排序与所见一致。
	if order := strings.TrimSpace(params.Order); order != "" {
		field, dir := order, "ASC"
		if strings.HasPrefix(field, "-") {
			field, dir = field[1:], "DESC"
		}
		if expr := tagAwareOrderExpr(field, dir); expr != "" {
			db = db.Order(expr)
			params.Order = "" // 已自定义排序，避免 PaginateAndOrder 对该列重复套用
		}
	}

	result, err := PaginateAndOrder(db, &params.PaginationParams, &channels, allowedChannelOrderFields)
	if err != nil {
		return nil, err
	}
	if err := hydrateGatewayCredentialSummaries(channels, true); err != nil {
		return nil, err
	}
	return result, nil
}

func hydrateGatewayCredentialSummaries(channels []*Channel, aggregateTags bool) error {
	if len(channels) == 0 {
		return nil
	}

	channelIDs := make([]int, 0, len(channels))
	channelIDsByRow := make(map[int][]int, len(channels))
	tags := make([]string, 0)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelIDs = append(channelIDs, channel.Id)
		channelIDsByRow[channel.Id] = []int{channel.Id}
		if aggregateTags && channel.Tag != "" {
			tags = append(tags, channel.Tag)
		}
	}
	if len(tags) > 0 {
		var taggedChannels []Channel
		if err := DB.Select("id", "tag").Where("tag IN ?", tags).Find(&taggedChannels).Error; err != nil {
			return err
		}
		idsByTag := make(map[string][]int, len(tags))
		for _, channel := range taggedChannels {
			idsByTag[channel.Tag] = append(idsByTag[channel.Tag], channel.Id)
			channelIDs = append(channelIDs, channel.Id)
		}
		for _, channel := range channels {
			if channel != nil && channel.Tag != "" {
				channelIDsByRow[channel.Id] = idsByTag[channel.Tag]
			}
		}
	}

	var endpoints []GatewayEndpoint
	if err := DB.Select("channel_id", "active_credential_id").Where("channel_id IN ?", channelIDs).Find(&endpoints).Error; err != nil {
		return err
	}
	activeIDByChannel := make(map[int]uint, len(endpoints))
	activeIDs := make([]uint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		activeIDByChannel[endpoint.ChannelID] = endpoint.ActiveCredentialID
		if endpoint.ActiveCredentialID != 0 {
			activeIDs = append(activeIDs, endpoint.ActiveCredentialID)
		}
	}

	credentialsByID := make(map[uint]GatewayCredential, len(activeIDs))
	if len(activeIDs) > 0 {
		var credentials []GatewayCredential
		if err := DB.Select("id", "auth_mode", "status", "tested_at").Where("id IN ?", activeIDs).Find(&credentials).Error; err != nil {
			return err
		}
		for _, credential := range credentials {
			credentialsByID[credential.ID] = credential
		}
	}

	for _, channel := range channels {
		if channel == nil {
			continue
		}
		statuses := make(map[string]struct{})
		authModes := make(map[string]struct{})
		configured := true
		for _, channelID := range channelIDsByRow[channel.Id] {
			activeID := activeIDByChannel[channelID]
			credential, exists := credentialsByID[activeID]
			if !exists {
				configured = false
				statuses["missing"] = struct{}{}
				continue
			}
			status := credential.Status
			if credential.AuthMode == string(domain.AuthModeNone) {
				status = "keyless"
			}
			statuses[status] = struct{}{}
			authModes[credential.AuthMode] = struct{}{}
			if credential.TestedAt != nil && (channel.CredentialTestedAt == nil || credential.TestedAt.After(*channel.CredentialTestedAt)) {
				testedAt := *credential.TestedAt
				channel.CredentialTestedAt = &testedAt
			}
		}
		channel.CredentialConfigured = configured
		channel.CredentialStatus = onlyMapValue(statuses, "mixed")
		channel.CredentialAuthMode = onlyMapValue(authModes, "mixed")
	}
	return nil
}

func onlyMapValue(values map[string]struct{}, fallback string) string {
	if len(values) != 1 {
		return fallback
	}
	for value := range values {
		return value
	}
	return fallback
}

// tagAwareOrderExpr 为「展示值=整组聚合」的列返回标签感知的排序表达式：标签代表行
// (tag != ”) 按整组聚合排序，普通渠道 (tag = ”) 仍按自身列值；聚合口径与
// GetChannelsTagAllList 的 _all 统计保持一致（SUM/SUM/已测平均）。返回 "" 表示该列
// 代表行展示的就是自身值，无需特殊处理，交由通用排序。dir 取 "ASC"/"DESC"。
func tagAwareOrderExpr(field, dir string) string {
	var expr string
	switch field {
	case "used_quota":
		expr = "CASE WHEN channels.tag = '' THEN channels.used_quota " +
			"ELSE (SELECT SUM(c2.used_quota) FROM channels c2 WHERE c2.tag = channels.tag) END"
	case "balance":
		expr = "CASE WHEN channels.tag = '' THEN channels.balance " +
			"ELSE (SELECT SUM(c2.balance) FROM channels c2 WHERE c2.tag = channels.tag) END"
	case "response_time":
		expr = "CASE WHEN channels.tag = '' THEN channels.response_time " +
			"ELSE (SELECT AVG(CASE WHEN c2.response_time > 0 THEN c2.response_time ELSE NULL END) " +
			"FROM channels c2 WHERE c2.tag = channels.tag) END"
	case "name":
		expr = "CASE WHEN channels.tag = '' THEN channels.name ELSE channels.tag END"
	default:
		return ""
	}
	return expr + " " + dir
}

func GetAllChannels() ([]*Channel, error) {
	var channels []*Channel
	err := DB.Order("id desc").Find(&channels).Error
	if err == nil {
		hydrateGatewayCredentials(channels)
	}
	return channels, err
}

func GetChannelById(id int) (*Channel, error) {
	channel := Channel{Id: id}
	err := DB.First(&channel, "id = ?", id).Error
	if err == nil {
		// Compatibility for a database that has not completed the one-time
		// credential projection yet. GatewayCredential wins whenever present.
		channel.CredentialConfigured = channel.Key != ""
		if credential, plaintext, credentialErr := LoadActiveGatewayCredentialSecret(DB, id); credentialErr == nil {
			channel.Key = plaintext
			channel.CredentialConfigured = true
			channel.CredentialAuthMode = credential.AuthMode
			channel.CredentialStatus = credential.Status
			channel.CredentialTestedAt = credential.TestedAt
			if credential.AuthMode == string(domain.AuthModeNone) {
				channel.CredentialStatus = "keyless"
			}
		}
	}
	return &channel, err
}

func GetChannelsByTag(tag string) ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where("tag = ?", tag).Find(&channels).Error
	if err == nil {
		hydrateGatewayCredentials(channels)
		err = hydrateGatewayCredentialSummaries(channels, false)
	}
	return channels, err
}

func hydrateGatewayCredentials(channels []*Channel) {
	if len(channels) == 0 {
		return
	}
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelIDs = append(channelIDs, channel.Id)
		}
	}
	if len(channelIDs) == 0 {
		return
	}

	var endpoints []GatewayEndpoint
	if err := DB.Select("channel_id", "active_credential_id").
		Where("channel_id IN ?", channelIDs).
		Find(&endpoints).Error; err != nil {
		return
	}
	activeIDByChannel := make(map[int]uint, len(endpoints))
	activeIDs := make([]uint, 0, len(endpoints))
	seenActiveIDs := make(map[uint]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.ActiveCredentialID == 0 {
			continue
		}
		activeIDByChannel[endpoint.ChannelID] = endpoint.ActiveCredentialID
		if _, exists := seenActiveIDs[endpoint.ActiveCredentialID]; !exists {
			seenActiveIDs[endpoint.ActiveCredentialID] = struct{}{}
			activeIDs = append(activeIDs, endpoint.ActiveCredentialID)
		}
	}
	if len(activeIDs) == 0 {
		return
	}

	var credentials []GatewayCredential
	if err := DB.Where("id IN ?", activeIDs).Find(&credentials).Error; err != nil {
		return
	}
	credentialsByID := make(map[uint]GatewayCredential, len(credentials))
	for _, credential := range credentials {
		credentialsByID[credential.ID] = credential
	}
	secretCipher, err := secret.NewFromConfig()
	if err != nil {
		return
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		activeID := activeIDByChannel[channel.Id]
		credential, exists := credentialsByID[activeID]
		if !exists || credential.ChannelID != channel.Id || credential.Status == GatewayCredentialRevoked {
			continue
		}
		plaintext, decryptErr := secretCipher.Decrypt(credential.SecretCiphertext)
		if decryptErr == nil {
			channel.Key = plaintext
		}
	}
}

func DeleteChannelTag(channelId int) error {
	var changed bool
	if err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id = ?", channelId).Update("tag", "")
		if result.Error != nil {
			return result.Error
		}
		changed = result.RowsAffected > 0
		if !changed {
			return nil
		}
		return SyncGatewayChannels(tx, []int{channelId})
	}); err != nil {
		return err
	}
	if changed {
		GatewayRoutes.Load()
	}
	return nil
}

func BatchDeleteChannel(ids []int) (int64, error) {
	var rowsAffected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id IN ?", ids).Delete(&Channel{})
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		return SyncGatewayChannels(tx, ids)
	})
	if err != nil {
		return 0, err
	}
	if rowsAffected > 0 {
		GatewayRoutes.Load()
	}
	return rowsAffected, nil
}

func BatchInsertChannels(channels []Channel) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("UsedQuota").Create(&channels).Error; err != nil {
			return err
		}
		ids := make([]int, 0, len(channels))
		for index := range channels {
			ids = append(ids, channels[index].Id)
		}
		return SyncGatewayChannels(tx, ids)
	}); err != nil {
		return err
	}
	GatewayRoutes.Load()
	return nil
}

type BatchChannelsParams struct {
	Value string `json:"value" form:"value" binding:"required"`
	Ids   []int  `json:"ids" form:"ids" binding:"required"`
}

func BatchUpdateChannelsAzureApi(params *BatchChannelsParams) (int64, error) {
	var rowsAffected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id IN ?", params.Ids).Update("other", params.Value)
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		return SyncGatewayChannels(tx, params.Ids)
	})
	if err != nil {
		return 0, err
	}
	if rowsAffected > 0 {
		GatewayRoutes.Load()
	}
	return rowsAffected, nil
}

func BatchDelModelChannels(params *BatchChannelsParams) (int64, error) {
	var count int64
	var changedIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []*Channel
		if err := tx.Select("id, models, "+quotePostgresField("group")).Find(&channels, "id IN ?", params.Ids).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			modelsSlice := strings.Split(channel.Models, ",")
			changed := false
			for i, modelName := range modelsSlice {
				if modelName == params.Value {
					modelsSlice = append(modelsSlice[:i], modelsSlice[i+1:]...)
					changed = true
					break
				}
			}
			if !changed {
				continue
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).
				Update("models", strings.Join(modelsSlice, ",")).Error; err != nil {
				return err
			}
			changedIDs = append(changedIDs, channel.Id)
			count++
		}
		return SyncGatewayChannels(tx, changedIDs)
	})
	if err != nil {
		return 0, err
	}
	if count > 0 {
		GatewayRoutes.Load()
	}
	return count, nil
}

func BatchAddUserGroupToChannels(params *BatchChannelsParams) (int64, error) {
	var count int64
	var changedIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []*Channel
		if err := tx.Select("id, "+quotePostgresField("group")).Find(&channels, "id IN ?", params.Ids).Error; err != nil {
			return err
		}

		for _, channel := range channels {
			currentGroups := strings.Split(channel.Group, ",")
			uniqueGroups := make(map[string]bool)
			for _, group := range currentGroups {
				group = strings.TrimSpace(group)
				if group != "" {
					uniqueGroups[group] = true
				}
			}
			newGroup := strings.TrimSpace(params.Value)
			if newGroup == "" || uniqueGroups[newGroup] {
				continue
			}
			uniqueGroups[newGroup] = true
			groupSlice := make([]string, 0, len(uniqueGroups))
			for group := range uniqueGroups {
				groupSlice = append(groupSlice, group)
			}
			slices.Sort(groupSlice)
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).
				Update("group", strings.Join(groupSlice, ",")).Error; err != nil {
				return err
			}
			changedIDs = append(changedIDs, channel.Id)
			count++
		}
		return SyncGatewayChannels(tx, changedIDs)
	})
	if err != nil {
		return 0, err
	}
	if count > 0 {
		GatewayRoutes.Load()
	}
	return count, nil
}

// BatchAddModelToChannels 批量添加模型到渠道
func BatchAddModelToChannels(params *BatchChannelsParams) (int64, error) {
	var count int64

	// 解析要添加的模型列表（支持逗号分隔的多个模型）
	newModels := strings.Split(params.Value, ",")
	var trimmedNewModels []string
	for _, model := range newModels {
		model = strings.TrimSpace(model)
		if model != "" {
			trimmedNewModels = append(trimmedNewModels, model)
		}
	}

	if len(trimmedNewModels) == 0 {
		return 0, nil
	}

	var changedIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []*Channel
		if err := tx.Select("id, models").Find(&channels, "id IN ?", params.Ids).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			// 获取当前渠道的模型列表
			currentModels := strings.Split(channel.Models, ",")

			// 清理空字符串并去重
			uniqueModels := make(map[string]bool)
			for _, model := range currentModels {
				model = strings.TrimSpace(model)
				if model != "" {
					uniqueModels[model] = true
				}
			}

			// 检查要添加的模型，只添加不存在的模型
			hasNewModel := false
			for _, newModel := range trimmedNewModels {
				if !uniqueModels[newModel] {
					uniqueModels[newModel] = true
					hasNewModel = true
				}
			}

			// 如果有新模型添加，则更新渠道
			if hasNewModel {
				// 重新构建模型字符串
				var modelSlice []string
				for model := range uniqueModels {
					modelSlice = append(modelSlice, model)
				}

				newModelString := strings.Join(modelSlice, ",")

				// 更新渠道模型
				if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", newModelString).Error; err != nil {
					return err
				}
				changedIDs = append(changedIDs, channel.Id)
				count++
			}
		}
		return SyncGatewayChannels(tx, changedIDs)
	})
	if err != nil {
		return 0, err
	}
	if count > 0 {
		GatewayRoutes.Load()
	}
	return count, nil
}

func (c *Channel) SetProxy() {
	if c.Proxy == nil {
		return
	}

	if strings.Contains(*c.Proxy, "%s") {
		md5Str := md5.Sum([]byte(c.Key))
		idStr := hex.EncodeToString(md5Str[:])
		*c.Proxy = strings.Replace(*c.Proxy, "%s", idStr, 1)
	}

}

func (c *Channel) GetProxy() string {
	if c.Proxy == nil {
		return ""
	}
	return *c.Proxy
}

func (channel *Channel) GetPriority() int64 {
	if channel.Priority == nil {
		return 0
	}
	return *channel.Priority
}

func (channel *Channel) GetCostRatio() float64 {
	if channel.CostRatio == nil || *channel.CostRatio <= 0 {
		return 0
	}
	return *channel.CostRatio
}

func (channel *Channel) GetBaseURL() string {
	if channel.BaseURL == nil {
		return ""
	}
	return *channel.BaseURL
}

func (channel *Channel) GetModelMapping() string {
	if channel.ModelMapping == nil {
		return ""
	}
	return *channel.ModelMapping
}

func (channel *Channel) GetCustomParameter() string {
	if channel.CustomParameter == nil {
		return ""
	}
	return *channel.CustomParameter
}

func (channel *Channel) Insert() error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("UsedQuota").Create(channel).Error; err != nil {
			return err
		}
		return SyncGatewayChannels(tx, []int{channel.Id})
	})
	if err != nil {
		return err
	}
	GatewayRoutes.Load()
	return nil
}

func (channel *Channel) Update(overwrite bool) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := channel.updateRaw(tx, overwrite); err != nil {
			return err
		}
		return SyncGatewayChannels(tx, []int{channel.Id})
	})
	if err != nil {
		return err
	}
	GatewayRoutes.Load()
	GatewayRoutes.ClearChannelCooldowns(channel.Id)
	return nil
}

func (channel *Channel) updateRaw(db *gorm.DB, overwrite bool) error {
	var err error

	if overwrite {
		err = db.Model(channel).Select("*").Omit("UsedQuota").Updates(channel).Error
	} else {
		err = db.Model(channel).Omit("UsedQuota").Updates(channel).Error
	}
	if err != nil {
		return err
	}
	return db.Model(channel).First(channel, "id = ?", channel.Id).Error
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     utils.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		logger.SysError("failed to update response time: " + err.Error())
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: utils.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		logger.SysError("failed to update balance: " + err.Error())
	}
}

func (channel *Channel) Delete() error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("channel id must be positive")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(channel)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return DeleteGatewayChannelResources(tx, channel.Id)
	})
	if err != nil {
		return err
	}
	GatewayRoutes.Load()
	return nil
}

func (channel *Channel) StatusToStr() string {
	switch channel.Status {
	case config.ChannelStatusEnabled:
		return "启用"
	case config.ChannelStatusAutoDisabled:
		return "自动禁用"
	case config.ChannelStatusManuallyDisabled:
		return "手动禁用"
	}

	return "禁用"
}

func UpdateChannelStatusById(id int, status int) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id = ?", id).Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return SyncGatewayEndpointStatus(tx, id, status)
	})
	if err != nil {
		return err
	}

	isEnabled := status == config.ChannelStatusEnabled
	GatewayRoutes.ChangeStatus(id, isEnabled)

	// 启用渠道时清除冻结缓存
	if isEnabled {
		GatewayRoutes.ClearChannelCooldowns(id)
	}
	return nil
}

func UpdateGatewayConnectionScheduling(id int, updates map[string]any) error {
	if id <= 0 {
		return errors.New("channel id must be positive")
	}
	if len(updates) == 0 {
		return nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return SyncGatewayChannels(tx, []int{id})
	})
	if err != nil {
		return err
	}
	GatewayRoutes.Load()
	return nil
}

func UpdateChannelUsedQuota(id int, quota int) {
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		return
	}
	updateChannelUsedQuota(id, quota)
}

func updateChannelUsedQuota(id int, quota int) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error
	if err != nil {
		logger.SysError("failed to update channel used quota: " + err.Error())
	}
}

func ClearChannelTokenCache(channelId int) {
	cacheKeys := []string{
		fmt.Sprintf("api_token:codex:%d", channelId),
		fmt.Sprintf("api_token:geminicli:%d", channelId),
		fmt.Sprintf("api_token:claudecode:%d", channelId),
		fmt.Sprintf("api_token:vertexai:%d", channelId),
		fmt.Sprintf("api_token:gemini_sa:%d", channelId),
		fmt.Sprintf("api_token:antigravity:%d", channelId),
	}

	for _, key := range cacheKeys {
		if err := cache.DeleteCache(key); err != nil {
			logger.SysError(fmt.Sprintf("failed to clear token cache %s: %v", key, err))
		}
	}
}

func UpdateChannelKey(id int, key string) error {
	err := ReplaceActiveGatewayCredential(DB, id, key)
	if err != nil {
		logger.SysError("failed to update channel key: " + err.Error())
		return err
	}

	ClearChannelTokenCache(id)
	GatewayRoutes.Load()

	return nil
}

func DeleteDisabledChannel() (int64, error) {
	var ids []int
	var rowsAffected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Channel{}).
			Where("status = ? or status = ?", config.ChannelStatusAutoDisabled, config.ChannelStatusManuallyDisabled)
		if err := query.Pluck("id", &ids).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ?", ids).Delete(&Channel{})
		if result.Error != nil {
			return result.Error
		}
		rowsAffected = result.RowsAffected
		return SyncGatewayChannels(tx, ids)
	})
	if err != nil {
		return 0, err
	}
	if rowsAffected > 0 {
		GatewayRoutes.Load()
	}
	return rowsAffected, nil
}

type ChannelStatistics struct {
	TotalChannels int `json:"total_channels"`
	Status        int `json:"status"`
}

func GetStatisticsChannel() (statistics []*ChannelStatistics, err error) {
	err = DB.Model(&Channel{}).Select("count(*) as total_channels, status").Group("status").Scan(&statistics).Error
	return statistics, err
}
