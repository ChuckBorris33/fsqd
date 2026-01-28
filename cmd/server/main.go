package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lmittmann/tint"

	"fsqdgo/internal/config"
	"fsqdgo/internal/download"
	"fsqdgo/internal/handler"
	"fsqdgo/internal/storage"
	"fsqdgo/internal/websocket"
)

func main() {
	cfg := config.LoadConfig()
	SetupLogger(cfg.LogLevel)

	store := storage.New(cfg.DataDir)
	hub := websocket.NewHub()
	downloader := download.New(store, hub, cfg.DownloadDir)
	downloader.Start()

	r := chi.NewRouter()
	r.Handle("/", http.FileServer(http.Dir("static")))
	r.Get("/queue", handler.GetQueueHandler(store))
	r.Post("/queue", handler.AddToQueueHandler(store, hub))
	r.Put("/queue/{id}/move", handler.MoveQueueItemHandler(store, hub))
	r.Put("/queue/{id}/retry", handler.RetryHandler(store, hub))
	r.Put("/queue/{id}/cancelDownload", handler.CancelDownloadHandler(downloader, hub))
	r.Delete("/queue/{id}", handler.DeleteQueueItemHandler(store, hub))
	r.Delete("/queue/failed", handler.ClearFailedHandler(store, hub))
	r.Delete("/queue/completed", handler.ClearCompletedHandler(store, hub))
	r.Get("/ws", hub.WsHandler)

	server := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server forced to shutdown")
		}
		done <- true
	}()

	slog.Info("Server starting", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Failed to start server")
	}
	<-done
	slog.Info("Server exited")
}

func SetupLogger(level slog.Level) {
	handler := tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "2006-01-02 15:04:05",
		AddSource:  true,
	})

	slog.SetDefault(slog.New(handler))
}
