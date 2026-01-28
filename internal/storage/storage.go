package storage

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	"fsqdgo/internal/models"
)

type Storage struct {
	mu       sync.RWMutex
	filePath string
	queue    models.Queue
}

func New(dataDir string) *Storage {
	os.MkdirAll(dataDir, os.ModePerm)

	store := &Storage{filePath: dataDir + "/queue.json"}
	queue, err := store.LoadQueue()
	if err != nil {
		slog.Warn("Could not load existing queue, starting fresh", "error", err)
		queue = models.Queue{}
	}
	store.queue = queue
	return store
}

func (s *Storage) SaveQueue(data models.Queue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storageData := models.QueueForStorage{
		Pending:   data.Pending,
		Completed: data.Completed,
		Failed:    data.Failed,
	}

	file, err := os.Create(s.filePath)
	if err != nil {
		slog.Error("Failed to save queue", "error", err)
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(storageData)
}

func (s *Storage) LoadQueue() (models.Queue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var data models.QueueForStorage
	file, err := os.Open(s.filePath)
	if err != nil {
		return models.Queue{}, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return models.Queue{}, err
	}

	return models.Queue{
		Downloading: []models.Item{},
		Pending:     data.Pending,
		Completed:   data.Completed,
		Failed:      data.Failed,
	}, nil
}

func (s *Storage) GetQueue() models.Queue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queue
}

func (s *Storage) AddPendingItem(item models.Item) {
	s.queue.Pending = append(s.queue.Pending, item)
	go s.SaveQueue(s.queue)
}

func (s *Storage) RemoveItemById(id string) bool {
	for i, item := range s.queue.Pending {
		if item.Id == id {
			s.queue.Pending = append(s.queue.Pending[:i], s.queue.Pending[i+1:]...)
			go s.SaveQueue(s.queue)
			return true
		}
	}

	for i, item := range s.queue.Completed {
		if item.Id == id {
			s.queue.Completed = append(s.queue.Completed[:i], s.queue.Completed[i+1:]...)
			go s.SaveQueue(s.queue)
			return true
		}
	}

	for i, item := range s.queue.Failed {
		if item.Id == id {
			s.queue.Failed = append(s.queue.Failed[:i], s.queue.Failed[i+1:]...)
			go s.SaveQueue(s.queue)
			return true
		}
	}

	return false
}

func (s *Storage) MovePendingItem(id string, up bool) bool {
	for i, item := range s.queue.Pending {
		if item.Id == id {
			if up && i > 0 {
				s.queue.Pending[i-1], s.queue.Pending[i] = s.queue.Pending[i], s.queue.Pending[i-1]
				go s.SaveQueue(s.queue)
				return true
			} else if !up && i < len(s.queue.Pending)-1 {
				s.queue.Pending[i], s.queue.Pending[i+1] = s.queue.Pending[i+1], s.queue.Pending[i]
				go s.SaveQueue(s.queue)
				return true
			}
			return false
		}
	}
	return false
}

func (s *Storage) ClearFailedItems() {
	s.queue.Failed = []models.FailedItem{}
	go s.SaveQueue(s.queue)
}

func (s *Storage) ClearCompletedItems() {
	s.queue.Completed = []models.Item{}
	go s.SaveQueue(s.queue)
}

func (s *Storage) RetryDownload(id string) bool {
	for i, failedItem := range s.queue.Failed {
		if failedItem.Id == id {
			s.queue.Failed = append(s.queue.Failed[:i], s.queue.Failed[i+1:]...)

			item := models.Item{
				Id:      failedItem.Id,
				Link:    failedItem.Link,
				Name:    failedItem.Name,
				Size:    failedItem.Size,
				AddedAt: failedItem.AddedAt,
			}

			s.queue.Pending = append(s.queue.Pending, item)

			go s.SaveQueue(s.queue)
			return true
		}
	}
	return false
}

func (s *Storage) MoveToDownloading(id string) (*models.Item, bool) {
	for i, item := range s.queue.Pending {
		if item.Id == id {
			s.queue.Pending = append(s.queue.Pending[:i], s.queue.Pending[i+1:]...)
			s.queue.Downloading = append(s.queue.Downloading, item)
			go s.SaveQueue(s.queue)
			return &item, true
		}
	}
	return nil, false
}

func (s *Storage) MoveToCompleted(downloadedItem models.Item) bool {
	for i, item := range s.queue.Downloading {
		if item.Id == downloadedItem.Id {
			s.queue.Downloading = append(s.queue.Downloading[:i], s.queue.Downloading[i+1:]...)
			s.queue.Completed = append(s.queue.Completed, downloadedItem)
			go s.SaveQueue(s.queue)
			return true
		}
	}
	return false
}

func (s *Storage) MoveToFailed(failedItem models.Item, errMsg string) bool {
	for i, item := range s.queue.Downloading {
		if item.Id == failedItem.Id {
			s.queue.Downloading = append(s.queue.Downloading[:i], s.queue.Downloading[i+1:]...)

			newItem := models.FailedItem{
				Item:  item,
				Error: errMsg,
			}
			s.queue.Failed = append(s.queue.Failed, newItem)

			go s.SaveQueue(s.queue)
			return true
		}
	}

	for i, item := range s.queue.Pending {
		if item.Id == failedItem.Id {
			s.queue.Pending = append(s.queue.Pending[:i], s.queue.Pending[i+1:]...)
			newItem := models.FailedItem{
				Item:  item,
				Error: errMsg,
			}
			s.queue.Failed = append(s.queue.Failed, newItem)
			go s.SaveQueue(s.queue)
			return true
		}
	}
	return false
}
