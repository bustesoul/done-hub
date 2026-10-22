package model

import "context"

// ModelProbeResult records a completed manual probe, separately from channel
// health and historical request metrics. At is the completion time in milliseconds.
type ModelProbeResult struct {
	At      int64  `json:"at"`
	Success bool   `json:"success"`
	Latency int64  `json:"latency"`
	Message string `json:"message" gorm:"type:text"`
}

func SaveModelChannelProbe(ctx context.Context, routeID uint, result ModelProbeResult) error {
	// Match the original row: a probe must not recreate a removed binding or
	// overwrite a newer completed result if database writes finish out of order.
	return DB.WithContext(ctx).Model(&GatewayModelRoute{}).
		Where("id = ? AND (last_probe_at IS NULL OR last_probe_at <= ?)", routeID, result.At).
		Updates(map[string]any{
			"last_probe_at": result.At, "last_probe_success": result.Success,
			"last_probe_latency": result.Latency, "last_probe_message": result.Message,
		}).Error
}
