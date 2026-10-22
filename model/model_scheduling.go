package model

import (
	"context"
	"errors"
	"time"

	"done-hub/common/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const modelRouteSchedulingMigration = "model-route-scheduling-v1"

// Legacy columns are read only here, once. They are not part of Channel/Endpoint
// or any running configuration; leaving them in place permits binary rollback.
func migrateModelRouteScheduling(tx *gorm.DB) error {
	var marker GatewayMigrationState
	err := tx.Where("name = ?", modelRouteSchedulingMigration).First(&marker).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	type legacyScheduling struct {
		ChannelID int
		Priority  int64
		Weight    uint
	}
	values := map[int]legacyScheduling{}
	// Before gateway cutover the channel table was authoritative. Existing gateway
	// deployments use Endpoint values, exactly as LoadGatewayChannelSnapshots did.
	for _, source := range []struct{ table, id string }{{"channels", "id"}, {"gateway_endpoints", "channel_id"}} {
		if !tx.Migrator().HasColumn(source.table, "priority") || !tx.Migrator().HasColumn(source.table, "weight") {
			continue
		}
		var rows []legacyScheduling
		if err := tx.Table(source.table).Select(source.id + " AS channel_id, priority, weight").Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			values[row.ChannelID] = row
		}
	}
	for id, value := range values {
		if value.Weight == 0 {
			value.Weight = config.DefaultChannelWeight
		}
		if err := tx.Model(&GatewayModelRoute{}).Where("channel_id = ?", id).
			Updates(map[string]any{"priority": value.Priority, "weight": value.Weight}).Error; err != nil {
			return err
		}
	}
	return tx.Create(&GatewayMigrationState{Name: modelRouteSchedulingMigration, CompletedAt: time.Now()}).Error
}

func GetModelChannelRoute(ctx context.Context, channelID int, publicModel string) (*GatewayModelRoute, error) {
	var route GatewayModelRoute
	err := DB.WithContext(ctx).Where("channel_id = ? AND public_model = ?", channelID, publicModel).First(&route).Error
	return &route, err
}

func SetModelChannelScheduling(ctx context.Context, channelID int, publicModel string, priority int64, weight uint) error {
	if weight < 1 || weight > 1000000 {
		return errors.New("权重须为 1 到 1000000 的整数")
	}
	if priority < -9007199254740991 || priority > 9007199254740991 {
		return errors.New("优先级超出安全整数范围")
	}
	modelBindingWriteMu.Lock()
	defer modelBindingWriteMu.Unlock()
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var endpoint GatewayEndpoint
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ?", channelID).First(&endpoint).Error; err != nil {
			return err
		}
		var route GatewayModelRoute
		if err := tx.Where("channel_id = ? AND public_model = ?", channelID, publicModel).First(&route).Error; err != nil {
			return err
		}
		return tx.Model(&route).Updates(map[string]any{"priority": priority, "weight": weight}).Error
	})
	if err == nil {
		GatewayRoutes.Load()
	}
	return err
}

func copyModelRouteScheduling(tx *gorm.DB, sourceID, targetID int) error {
	var routes []GatewayModelRoute
	if err := tx.Where("channel_id = ?", sourceID).Find(&routes).Error; err != nil {
		return err
	}
	for _, r := range routes {
		if err := tx.Model(&GatewayModelRoute{}).Where("channel_id = ? AND public_model = ?", targetID, r.PublicModel).
			Updates(map[string]any{"priority": r.Priority, "weight": r.Weight}).Error; err != nil {
			return err
		}
	}
	return nil
}
