package model

import (
	"done-hub/common/config"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type SearchChannelsTagParams struct {
	Tag string `json:"tag" form:"tag"`
	PaginationParams
}

type ChannelTag struct {
	ID           int     `json:"id"`
	Tag          string  `json:"tag"`
	Count        int     `json:"count"`         // 该标签下的渠道总数
	Enabled      int     `json:"enabled"`       // 其中已启用的渠道数
	TypeCount    int     `json:"type_count"`    // 组内不同渠道类型数，>1 表示类型不一致（混合）
	GroupCount   int     `json:"group_count"`   // 组内不同分组配置数，>1 表示分组不一致（混合）
	UsedQuota    int64   `json:"used_quota"`    // 组内已用额度合计
	Balance      float64 `json:"balance"`       // 组内余额合计
	ResponseTime float64 `json:"response_time"` // 组内已测渠道的平均响应时间(ms)，未测渠道不计入
}

// CheckTagTypeConsistency 护栏：同一标签下的渠道共享同一套配置、各自不同 Key，全组类型必须一致。
// 阻止把不同类型的渠道加入同一标签，避免后续分组统一编辑把整组覆盖成单一类型。
// excludeID 用于编辑自身时排除当前渠道（新建时传 0）。
func CheckTagTypeConsistency(tag string, channelType int, excludeID int) error {
	if tag == "" {
		return nil
	}
	var types []int
	err := DB.Model(&Channel{}).
		Distinct("type").
		Where("tag = ? AND id <> ?", tag, excludeID).
		Pluck("type", &types).Error
	if err != nil {
		return err
	}
	for _, t := range types {
		if t != channelType {
			return fmt.Errorf("标签「%s」下已有其它类型的渠道，无法加入不同类型的渠道；请使用相同类型，或换一个标签名", tag)
		}
	}
	return nil
}

func GetChannelsTagList(params *SearchChannelsTagParams) (*DataResult[Channel], error) {
	var channels []*Channel
	db := DB.Omit("key").Where("tag = ?", params.Tag)
	result, err := PaginateAndOrder(db, &params.PaginationParams, &channels, allowedChannelOrderFields)
	if err != nil {
		return nil, err
	}
	if err := hydrateGatewayCredentialSummaries(channels, false); err != nil {
		return nil, err
	}
	return result, nil
}

func GetChannelsTagAllList() ([]*ChannelTag, error) {
	var channelTags []*ChannelTag
	groupField := quotePostgresField("group")
	err := DB.Model(&Channel{}).
		Select("tag, COUNT(*) as count, "+
			"SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as enabled, "+
			"COUNT(DISTINCT type) as type_count, "+
			"COUNT(DISTINCT "+groupField+") as group_count, "+
			"SUM(used_quota) as used_quota, "+
			"SUM(balance) as balance, "+
			"AVG(CASE WHEN response_time > 0 THEN response_time ELSE NULL END) as response_time", config.ChannelStatusEnabled).
		Where("tag != ''").
		Group("tag").
		Find(&channelTags).Error

	return channelTags, err
}

type ChannelTagCollection struct {
	Channel
	KeyMap map[string]int
	Count  int `json:"count"` // 该标签下的渠道总数（含禁用），用于前端"覆盖全部 N 个"提示
}

func GetChannelsTag(tag string) (*ChannelTagCollection, error) {
	var channelTag ChannelTagCollection

	var channels []Channel
	err := DB.Where("tag = ?", tag).Order("id ASC").Find(&channels).Error
	if err != nil {
		return nil, err
	}

	if len(channels) == 0 {
		return nil, errors.New("tag不存在")
	}

	channelTag.Channel = channels[0]
	channelTag.Count = len(channels)
	channelTag.Key = ""

	channelTag.KeyMap = make(map[string]int)
	return &channelTag, nil
}

func UpdateChannelsTag(tag string, channel *Channel) error {
	_, err := GetChannelsTag(tag)
	if err != nil {
		return err
	}
	// tag 是分组的唯一标识，若清空会因 Select("*") 强制写入而意外解散整组
	if channel.Tag == "" {
		return errors.New("tag不能为空")
	}

	err = mutateChannelsTag(tag, nil, func(tx *gorm.DB, _ []int) error {
		return tx.Model(Channel{}).Where("tag = ?", tag).Updates(
			Channel{
				ProtocolProfileID:   channel.ProtocolProfileID,
				BaseURL:             channel.BaseURL,
				Other:               channel.Other,
				Remark:              channel.Remark,
				Models:              channel.Models,
				Group:               channel.Group,
				Tag:                 channel.Tag,
				ModelMapping:        channel.ModelMapping,
				Need2ResponseModels: channel.Need2ResponseModels,
				ModelHeaders:        channel.ModelHeaders,
				CustomParameter:     channel.CustomParameter,
				Proxy:               channel.Proxy,
				TestModel:           channel.TestModel,
				OnlyChat:            channel.OnlyChat,
				Plugin:              channel.Plugin,
				PreCost:             channel.PreCost,
				DisabledStream:      channel.DisabledStream,
				CompatibleResponse:  channel.CompatibleResponse,
			}).Error
	})
	return err
}

func DeleteChannelsTag(tag string, delDisabled bool) error {
	if tag == "" {
		return nil
	}

	var extraScope func(*gorm.DB) *gorm.DB
	if delDisabled {
		extraScope = func(db *gorm.DB) *gorm.DB {
			return db.Where("status IN ?", []int{config.ChannelStatusAutoDisabled, config.ChannelStatusManuallyDisabled})
		}
	}
	return mutateChannelsTag(tag, extraScope, func(tx *gorm.DB, ids []int) error {
		if len(ids) == 0 {
			return nil
		}
		return tx.Where("id IN ?", ids).Delete(&Channel{}).Error
	})
}

func ChangeChannelsTagStatus(tag string, status int) error {
	if tag == "" {
		return nil
	}

	return mutateChannelsTag(tag, nil, func(tx *gorm.DB, _ []int) error {
		return tx.Model(&Channel{}).Where("tag = ?", tag).Update("status", status).Error
	})
}

func UpdateChannelsTagPriority(tag string, value int) error {
	return updateChannelsTagField(tag, "priority", value)
}

// UpdateChannelsTagWeight 将整组渠道的权重统一设为同一值（权重为逐行字段，需专用入口批量设置）。
func UpdateChannelsTagWeight(tag string, value uint) error {
	return updateChannelsTagField(tag, "weight", value)
}

// UpdateChannelsTagCostRatio 将整组渠道的成本倍率统一设为同一值。
func UpdateChannelsTagCostRatio(tag string, value float64) error {
	return updateChannelsTagField(tag, "cost_ratio", value)
}

func getChannelIDsByTag(tag string) ([]int, error) {
	var channelIDs []int
	err := DB.Model(&Channel{}).Where("tag = ?", tag).Pluck("id", &channelIDs).Error
	return channelIDs, err
}

func updateChannelsTagField(tag, field string, value any) error {
	return mutateChannelsTag(tag, nil, func(tx *gorm.DB, _ []int) error {
		return tx.Model(&Channel{}).Where("tag = ?", tag).Update(field, value).Error
	})
}

// mutateChannelsTag is the single-write boundary for tag management: source
// rows and Gateway projections commit or roll back together, then the runtime
// snapshot is reloaded only after commit.
func mutateChannelsTag(
	tag string,
	extraScope func(*gorm.DB) *gorm.DB,
	mutate func(*gorm.DB, []int) error,
) error {
	var channelIDs []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Channel{}).Where("tag = ?", tag)
		if extraScope != nil {
			query = extraScope(query)
		}
		if err := query.Pluck("id", &channelIDs).Error; err != nil {
			return err
		}
		if err := mutate(tx, channelIDs); err != nil {
			return err
		}
		return SyncGatewayChannels(tx, channelIDs)
	})
	if err != nil {
		return err
	}
	if len(channelIDs) > 0 {
		GatewayRoutes.Load()
	}
	return nil
}
