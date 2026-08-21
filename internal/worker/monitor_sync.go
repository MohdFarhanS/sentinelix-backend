package worker

import (
	"context"
	"encoding/json"

	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/rs/zerolog"
	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// monitorSyncPayload — Data di BroadcastMessage buat event monitor.*
// cuma butuh ID, supervisor yang fetch detail terbaru sendiri.
type monitorSyncPayload struct {
	ID string `json:"id"`
}

// RunMonitorSync dipanggil sekali dari cmd/worker/main.go, block sampai
// ctx dibatalkan atau channel Redis ditutup.
func RunMonitorSync(ctx context.Context, supervisor *MonitorSupervisor, broadcaster *redisrepo.Broadcaster, logger zerolog.Logger) error {
	sub := broadcaster.Subscribe(ctx)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()

	logger.Info().Msg("monitor sync subscriber started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}

			var bm redisrepo.BroadcastMessage
			if err := json.Unmarshal([]byte(msg.Payload), &bm); err != nil {
				logger.Error().Err(err).Msg("monitor sync: failed to unmarshal broadcast message")
				continue
			}

			switch bm.Type {
			case domain.MonitorSyncCreated, domain.MonitorSyncUpdated:
				payload, err := parseMonitorSyncPayload(bm.Data)
				if err != nil {
					logger.Error().Err(err).Str("type", bm.Type).Msg("monitor sync: invalid payload")
					continue
				}
				supervisor.Sync(ctx, payload.ID)
			case domain.MonitorSyncDeleted:
				payload, err := parseMonitorSyncPayload(bm.Data)
				if err != nil {
					logger.Error().Err(err).Str("type", bm.Type).Msg("monitor sync: invalid payload")
					continue
				}
				supervisor.Remove(payload.ID)
			default:
				// Event bukan monitor.* (misal issue.created buat WS
				// hub) — bukan urusan supervisor ini.
			}
		}
	}
}

func parseMonitorSyncPayload(data interface{}) (monitorSyncPayload, error) {
	var payload monitorSyncPayload
	raw, err := json.Marshal(data)
	if err != nil {
		return payload, err
	}
	err = json.Unmarshal(raw, &payload)
	return payload, err
}