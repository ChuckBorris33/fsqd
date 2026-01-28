package config

import (
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Port        string
	LogLevel    slog.Level
	DataDir     string
	DownloadDir string
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	logLevelString := strings.ToUpper(os.Getenv("LOG_LEVEL"))
	if logLevelString == "" {
		logLevelString = "INFO"
	}
	var logLevel slog.Level
	switch logLevelString {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	DataDir := os.Getenv("DATA_DIR")
	if DataDir == "" {
		DataDir = "./data"
	}
	DownloadDir := os.Getenv("DOWNLOAD_DIR")
	if DownloadDir == "" {
		DownloadDir = "./downloads"
	}
	return Config{Port: port, LogLevel: logLevel, DataDir: DataDir, DownloadDir: DownloadDir}
}
