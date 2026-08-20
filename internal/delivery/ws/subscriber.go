package ws

import (
	"context"
	"encoding/json"

	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
)

// RunSubscriber jalan sebagai goroutine tunggal sepanjang hidup proses API.
// Baca terus dari Redis Pub/Sub, teruskan ke hub yang route berdasarkan
// project_id di envelope.
func RunSubscriber(ctx context.Context, hub *Hub, broadcaster *redisrepo.Broadcaster) {
	pubsub := broadcaster.Subscribe(ctx)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}

			var envelope redisrepo.BroadcastMessage
			if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
				hub.logger.Error().Err(err).Msg("failed to unmarshal broadcast envelope")
				continue
			}

			// Buang project_id sebelum diteruskan ke client — client
			// cuma perlu tahu type & data (04-API-DESIGN.md §8), tidak
			// perlu tahu project_id-nya lagi karena mereka sudah connect
			// ke /ws/projects/:id yang spesifik.
			outbound, err := json.Marshal(map[string]interface{}{
				"type": envelope.Type,
				"data": envelope.Data,
			})
			if err != nil {
				hub.logger.Error().Err(err).Msg("failed to marshal outbound message")
				continue
			}

			hub.BroadcastToProject(envelope.ProjectID, outbound)
		}
	}
}