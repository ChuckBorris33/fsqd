package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"fsqdgo/internal/download"
	"fsqdgo/internal/models"
	"fsqdgo/internal/storage"
	"fsqdgo/internal/utils"
	"fsqdgo/internal/websocket"
)

func GetQueueHandler(store *storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		queue := store.GetQueue()
		if err := json.NewEncoder(w).Encode(queue); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "failed to encode queue"}`))
		}
	}
}

func ClearFailedHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store.ClearFailedItems()
		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
	}
}

func ClearCompletedHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store.ClearCompletedItems()
		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
	}
}

func RetryHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
			return
		}

		retried := store.RetryDownload(id)

		if !retried {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed item not found"})
			return
		}

		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "retried"})
	}
}

func MoveQueueItemHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
			return
		}

		var req struct {
			Direction string `json:"direction"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
			return
		}

		if req.Direction != "up" && req.Direction != "down" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "direction must be 'up' or 'down'"})
			return
		}

		up := req.Direction == "up"
		moved := store.MovePendingItem(id, up)

		if !moved {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "item not found or cannot move"})
			return
		}

		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "moved"})
	}
}

func DeleteQueueItemHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
			return
		}

		deleted := store.RemoveItemById(id)

		if !deleted {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "item not found"})
			return
		}

		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

func AddToQueueHandler(store *storage.Storage, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Link string `json:"link"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
			return
		}

		if req.Link == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "link is required"})
			return
		}

		name, sizeStr := utils.ExtractFileInfo(req.Link)
		size := utils.ParseSize(sizeStr)
		slog.Info("Extracted info", "name", name, "sizeStr", sizeStr, "size", size)

		item := models.Item{
			Id:      time.Now().Format("20060102150405.999"),
			Link:    req.Link,
			Name:    name,
			Size:    size,
			AddedAt: time.Now().Format(time.RFC3339),
		}

		store.AddPendingItem(item)
		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	}
}

func CancelDownloadHandler(downloader *download.Downloader, hub *websocket.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "id is required"})
			return
		}

		ok := downloader.Cancel(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "download not found or not active"})
			return
		}

		hub.BroadcastUpdate()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	}
}
