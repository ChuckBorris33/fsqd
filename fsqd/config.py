import os

# --- Server Configuration ---
HOST = os.environ.get("HOST", "0.0.0.0")
PORT = int(os.environ.get("PORT", 8080))

# --- Logging Configuration ---
LOG_LEVEL = os.environ.get("LOG_LEVEL", "INFO").upper()

# --- Directory Configuration ---
# Use absolute paths for Docker compatibility
DOWNLOAD_DIR = os.environ.get("DOWNLOAD_DIR", "downloads")
CACHE_DIR = os.environ.get("CACHE_DIR", "cache")

# --- Storage Configuration ---
ACTIVE_KEY = "queue_active"
FAILED_KEY = "queue_failed"
COMPLETED_KEY = "queue_completed"
DOWNLOADED_KEY = "queue_downloaded"

# --- Downloader Configuration ---
USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
DOWNLOAD_CHUNK_SIZE = 8192
PROGRESS_UPDATE_INTERVAL = 1.0  # in seconds
