package websocket

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu        sync.Mutex
	clients   map[*websocket.Conn]bool
	broadcast chan []byte
	upgrader  websocket.Upgrader
}

type ProgressUpdate struct {
	Type          string `json:"type"`
	ItemID        string `json:"itemId"`
	Progress      int    `json:"progress"`
	DownloadSpeed string `json:"downloadSpeed"`
}

func NewProgressUpdate(itemID string, progress int, downloadSpeed string) *ProgressUpdate {
	return &ProgressUpdate{
		Type:          "progress",
		ItemID:        itemID,
		Progress:      progress,
		DownloadSpeed: downloadSpeed,
	}
}

func NewHub() *Hub {
	return &Hub{
		mu:        sync.Mutex{},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Hub) Run() {
	for {
		msg := <-h.broadcast
		h.mu.Lock()
		for client := range h.clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				client.Close()
				delete(h.clients, client)
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) StartTicker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		h.BroadcastUpdate()
	}
}

func (h *Hub) BroadcastUpdate() {
	h.broadcast <- []byte(`{"type": "update"}`)
}

func (h *Hub) BroadcastProgress(update *ProgressUpdate) {
	msg, err := json.Marshal(update)
	if err != nil {
		slog.Error("Failed to marshal progress update", "error", err)
		return
	}
	h.broadcast <- msg
}

func (h *Hub) WsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}

	slog.Info("Client connected", "remote_addr", r.RemoteAddr)
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
		slog.Info("Client disconnected")
	}()

	waitTimeout := 60 * time.Second
	for {
		conn.SetReadDeadline(time.Now().Add(waitTimeout))
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("WS read error", "error", err)
			}
			break
		}
	}
}
