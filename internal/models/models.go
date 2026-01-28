package models

type Queue struct {
	Downloading []Item       `json:"downloading"`
	Pending     []Item       `json:"pending"`
	Completed   []Item       `json:"completed"`
	Failed      []FailedItem `json:"failed"`
}

type QueueForStorage struct {
	Pending   []Item       `json:"pending"`
	Completed []Item       `json:"completed"`
	Failed    []FailedItem `json:"failed"`
}

type Item struct {
	Id      string `json:"id"`
	Link    string `json:"link"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	AddedAt string `json:"addedAt"`
}

type FailedItem struct {
	Item
	Error string `json:"error"`
}
