package model

import (
	"context"
	"done-hub/internal/audit"
	"errors"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Serialize local saves with reloads so an older DB read cannot overwrite a
// successful local update. This polling is independent of the channel cache.
var auditOptionMu sync.Mutex

func saveAuditConfig(value string) error {
	c, err := audit.ParseConfig(value)
	if err != nil {
		return err
	}
	auditOptionMu.Lock()
	defer auditOptionMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = DB.WithContext(ctx).Save(&Option{Key: audit.OptionKey, Value: value}).Error; err != nil {
		return err
	}
	audit.Default.Apply(c)
	audit.Default.SetConfigError(nil)
	return nil
}

func ReloadAuditConfig(ctx context.Context) error {
	auditOptionMu.Lock()
	defer auditOptionMu.Unlock()
	var option Option
	result := DB.WithContext(ctx).Where(&Option{Key: audit.OptionKey}).Limit(1).Find(&option)
	err := result.Error
	if err == nil && result.RowsAffected == 0 {
		audit.Default.Apply(audit.Config{})
		return nil
	}
	if err != nil {
		return err
	}
	c, err := audit.ParseConfig(option.Value)
	if err == nil {
		audit.Default.Apply(c)
	}
	return err
}

type AuditTarget struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	UserID int    `json:"user_id,omitempty"`
}

// Existing channel lists collapse tags into representative rows; token admin
// lists include the secret. Neither is a safe, complete selector catalogue.
// Project only identifiers/names from the authoritative endpoint/token tables.
func ListAuditTargets(ctx context.Context, kind, query string) ([]AuditTarget, error) {
	var db *gorm.DB
	idColumn := "id"
	switch kind {
	case "channels":
		db = DB.WithContext(ctx).Model(&GatewayEndpoint{}).Select("channel_id AS id", "name")
		idColumn = "channel_id"
	case "tokens":
		db = DB.WithContext(ctx).Model(&Token{}).Select("id", "name", "user_id")
	default:
		return nil, errors.New("无效的审计筛选类型")
	}
	if query != "" {
		if id, err := strconv.Atoi(query); err == nil {
			db = db.Where("name LIKE ? OR "+idColumn+" = ?", "%"+query+"%", id)
		} else {
			db = db.Where("name LIKE ?", "%"+query+"%")
		}
	}
	targets := []AuditTarget{}
	err := db.Order(idColumn + " DESC").Limit(100).Scan(&targets).Error
	return targets, err
}

func SyncAuditConfig(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reloadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := ReloadAuditConfig(reloadCtx)
			cancel()
			audit.Default.SetConfigError(err)
		}
	}
}
