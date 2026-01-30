package download

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"fsqdgo/internal/models"
	"fsqdgo/internal/storage"
	"fsqdgo/internal/websocket"
)

const (
	chunkSize         = 1024 * 1024
	progressDelay     = 4 * time.Second
	userAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	maxRetries        = 5
	initialRetryDelay = 2 * time.Second
	maxRetryDelay     = 30 * time.Second
)

type Downloader struct {
	store       *storage.Storage
	hub         *websocket.Hub
	downloadDir string
	cancelCh    map[string]chan struct{}
	cancelMu    sync.Mutex
}

func New(store *storage.Storage, hub *websocket.Hub, downloadDir string) *Downloader {
	os.MkdirAll(downloadDir, os.ModePerm)
	return &Downloader{
		store:       store,
		hub:         hub,
		downloadDir: downloadDir,
		cancelCh:    make(map[string]chan struct{}),
	}
}

func (d *Downloader) Start() {
	go d.worker()
}

func (d *Downloader) worker() {
	for {
		queue := d.store.GetQueue()
		if len(queue.Pending) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}

		item := queue.Pending[0]
		d.downloadItem(&item)
	}
}

func (d *Downloader) downloadItem(item *models.Item) {
	slog.Info("Downloading", "id", item.Id, "name", item.Name)

	if _, ok := d.store.MoveToDownloading(item.Id); !ok {
		slog.Warn("Failed to move to downloading", "id", item.Id)
		return
	}
	d.hub.BroadcastUpdate()

	formURL, err := d.getDownloadURL(item.Link)
	if err != nil {
		d.fail(item, err)
		return
	}

	cancel := make(chan struct{})
	d.setCancel(item.Id, cancel)
	defer d.clearCancel(item.Id)

	if err := d.downloadFile(item, formURL, cancel); err != nil {
		d.fail(item, err)
		return
	}

	d.store.MoveToCompleted(*item)
	d.hub.BroadcastUpdate()
	slog.Info("Download complete", "id", item.Id)
}

func (d *Downloader) getDownloadURL(pageURL string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(pageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	var formAction string
	doc.Find("form").Each(func(_ int, s *goquery.Selection) {
		if action, ok := s.Attr("action"); ok && strings.HasPrefix(action, "/free/") {
			formAction = action
		}
	})

	if formAction == "" {
		html, _ := doc.Html()
		slog.Error("Could not find download form", "url", pageURL, "html", html)
		return "", fmt.Errorf("download form not found")
	}

	u, _ := url.Parse(pageURL)
	formURL, _ := u.Parse(formAction)
	return formURL.String(), nil
}

func (d *Downloader) downloadFile(item *models.Item, formURL string, cancel <-chan struct{}) error {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-cancel:
			return fmt.Errorf("cancelled")
		default:
		}

		slog.Info("Download attempt", "id", item.Id, "attempt", attempt, "max", maxRetries)

		err := d.attemptDownload(item, formURL, cancel)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		if attempt < maxRetries {
			// Exponential backoff with jitter
			backoff := float64(initialRetryDelay) * math.Pow(2, float64(attempt-1))
			if backoff > float64(maxRetryDelay) {
				backoff = float64(maxRetryDelay)
			}
			jitter := (rand.Float64() - 0.5) * backoff
			delay := time.Duration(backoff + jitter)

			slog.Warn("Download attempt failed, retrying", "id", item.Id, "error", err, "retryIn", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("download failed after %d attempts: %w", maxRetries, lastErr)
}

func (d *Downloader) attemptDownload(item *models.Item, formURL string, cancel <-chan struct{}) error {
	resp, err := d.doDownloadRequest(formURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.ContentLength > 0 {
		item.Size = resp.ContentLength
	}

	filePath := filepath.Join(d.downloadDir, sanitizeName(item.Name))
	// Overwrite file on each attempt
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return d.copyWithProgress(resp.Body, file, item, cancel)
}

func (d *Downloader) doDownloadRequest(formURL string) (*http.Response, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("POST", formURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server error: %s", resp.Status)
	}

	if resp.ContentLength == 0 {
		resp.Body.Close()
		return nil, fmt.Errorf("empty response, no content")
	}

	contentType := resp.Header.Get("Content-Type")
	validTypes := []string{
		"application/octet-stream",
		"application/force-download",
		"video/",
		"audio/",
		"image/",
		"application/pdf",
		"application/zip",
		"application/x-rar-compressed",
		"application/x-tar",
		"application/x-gzip",
		"application/x-bzip2",
		"application/x-7z-compressed",
	}

	valid := false
	for _, t := range validTypes {
		if strings.HasPrefix(contentType, t) {
			valid = true
			break
		}
	}

	if !valid {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected content type: %s", contentType)
	}

	return resp, nil
}

func (d *Downloader) copyWithProgress(src io.Reader, dst *os.File, item *models.Item, cancel <-chan struct{}) error {
	var total int64
	buf := make([]byte, chunkSize)
	lastReport := time.Now()
	reportedBytes := 0

	progress := &websocket.ProgressUpdate{Type: "progress", ItemID: item.Id}

	for {
		select {
		case <-cancel:
			progress.DownloadSpeed = ""
			d.hub.BroadcastProgress(progress)
			return fmt.Errorf("cancelled")
		default:
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return werr
				}
				total += int64(n)
				reportedBytes += n

				if item.Size > 0 {
					progress.Progress = int(float64(total) / float64(item.Size) * 100)
				}

				if time.Since(lastReport) >= progressDelay {
					speed := float64(reportedBytes) / time.Since(lastReport).Seconds()
					progress.DownloadSpeed = formatSpeed(speed)
					d.hub.BroadcastProgress(progress)

					lastReport = time.Now()
					reportedBytes = 0
				}
			}

			if err == io.EOF {
				progress.Progress = 100
				d.hub.BroadcastProgress(progress)
				return nil
			}
			if err != nil {
				return err
			}
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

func (d *Downloader) fail(item *models.Item, err error) {
	d.store.MoveToFailed(*item, err.Error())
	d.hub.BroadcastUpdate()
	slog.Error("Download failed", "id", item.Id, "error", err)
}

func (d *Downloader) Cancel(id string) bool {
	d.cancelMu.Lock()
	defer d.cancelMu.Unlock()
	if ch, ok := d.cancelCh[id]; ok {
		close(ch)
		delete(d.cancelCh, id)
		return true
	}
	return false
}

func (d *Downloader) setCancel(id string, ch chan struct{}) {
	d.cancelMu.Lock()
	d.cancelCh[id] = ch
	d.cancelMu.Unlock()
}

func (d *Downloader) clearCancel(id string) {
	d.cancelMu.Lock()
	delete(d.cancelCh, id)
	d.cancelMu.Unlock()
}

func sanitizeName(name string) string {
	return regexp.MustCompile(`[^a-zA-Z0-9-_. ]`).ReplaceAllString(name, "")
}

func formatSpeed(bps float64) string {
	switch {
	case bps >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", bps/(1024*1024))
	case bps >= 1024:
		return fmt.Sprintf("%.1f KB/s", bps/1024)
	default:
		return fmt.Sprintf("%.1f B/s", bps)
	}
}
