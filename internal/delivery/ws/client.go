package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait: waktu maksimal tunggu sampai write ke koneksi selesai
	// sebelum dianggap gagal/macet.
	writeWait = 10 * time.Second

	// pongWait: kalau tidak terima pong dari client dalam waktu ini,
	// anggap koneksi mati (client hilang tanpa close handshake yang benar).
	pongWait = 60 * time.Second

	// pingPeriod harus lebih kecil dari pongWait, supaya ping sempat
	// dikirim & dijawab sebelum pongWait habis.
	pingPeriod = (pongWait * 9) / 10

	sendBufferSize = 16
)

func NewClient(conn *websocket.Conn, projectID string) *Client {
	return &Client{
		conn:      conn,
		send:      make(chan []byte, sendBufferSize),
		projectID: projectID,
	}
}

// readPump baca dari koneksi TERUS-MENERUS. Kita tidak pernah proses pesan
// dari client (dashboard cuma nerima broadcast, tidak pernah kirim data ke
// server lewat WS ini) — tapi readPump WAJIB tetap jalan supaya:
// 1. Pong response dari client kebaca (buat reset read deadline)
// 2. Close frame dari client (browser nutup tab) kedeteksi, biar unregister
func (c *Client) ReadPump(hub *Hub) {
	defer func() {
		hub.unregister(c)
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break // termasuk close frame dari client atau koneksi putus
		}
	}
}

// writePump kirim message dari channel c.send ke koneksi WS, plus ping
// berkala buat keep-alive.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// channel ditutup oleh hub.unregister — kirim close frame
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}