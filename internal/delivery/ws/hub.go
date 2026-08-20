package ws

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	projectID string
}

type Hub struct {
	mu     sync.RWMutex
	rooms  map[string]map[*Client]bool
	logger zerolog.Logger
}

func NewHub(logger zerolog.Logger) *Hub {
	return &Hub{
		rooms:  make(map[string]map[*Client]bool),
		logger: logger,
	}
}

func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[client.projectID] == nil {
		h.rooms[client.projectID] = make(map[*Client]bool)
	}
	h.rooms[client.projectID][client] = true
}

func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, ok := h.rooms[client.projectID]
	if !ok {
		return
	}
	if _, exists := room[client]; !exists {
		return
	}
	delete(room, client)
	close(client.send)
	if len(room) == 0 {
		delete(h.rooms, client.projectID)
	}
}

// BroadcastToProject dipanggil dari subscriber goroutine (baca dari Redis
// Pub/Sub) tiap kali ada message masuk buat project tertentu.
func (h *Hub) BroadcastToProject(projectID string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room, ok := h.rooms[projectID]
	if !ok {
		return // tidak ada client yang lagi buka project ini — buang saja
	}

	for client := range room {
		select {
		case client.send <- message:
		default:
			// send buffer client penuh (client macet/lambat) — putus
			// biar tidak nge-block broadcast ke client lain di room yang sama.
			h.logger.Warn().Str("project_id", projectID).Msg("client send buffer full, dropping connection")
			go h.unregister(client)
		}
	}
}

func (h *Hub) Register(client *Client) { h.register(client) }