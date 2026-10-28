// Package download implements the download engine: a queue of downloads with a
// concurrency limit, pause/resume over HTTP Range requests, retries, and the
// records that survive a restart.
package download

import "time"

// State is where a download is.
type State string

const (
	StateQueued      State = "queued"
	StateConnecting  State = "connecting"
	StateDownloading State = "downloading"
	StatePaused      State = "paused"
	StateCompleted   State = "completed"
	StateFailed      State = "failed"
)

// Active reports whether the state is one of the working states.
func (s State) Active() bool {
	return s == StateQueued || s == StateConnecting || s == StateDownloading
}

// Terminal reports whether no worker will touch the item again.
func (s State) Terminal() bool {
	return s == StateCompleted || s == StateFailed
}

// Item is a snapshot of one download for the UI.
type Item struct {
	ID string
	// URL is the link without its credentials, for showing and copying.
	URL string
	// DisplayURL is the same link shortened for the UI. It equals URL.
	DisplayURL string
	FileName   string
	Path       string
	State      State
	// Total is the size in bytes, or -1 when the server does not say.
	Total int64
	Done  int64
	// Speed is bytes per second over the last samples.
	Speed float64
	ETA   time.Duration
	Error string
	// Attempt is the retry number of the current failure.
	Attempt int
	// RetryCount includes automatic retries across attempts and restarts.
	RetryCount int
	// Resumable reports whether the response supports ranges with a usable validator.
	Resumable bool
	// ResumableKnown distinguishes a response from an unconnected task.
	ResumableKnown bool
	// NeedsHeaders asks the user to re-enter headers lost after a restart.
	NeedsHeaders bool
	// Missing reports a completed download whose file is gone.
	Missing    bool
	CreatedAt  time.Time
	FinishedAt time.Time
}

// Progress returns done/total from 0 to 1, and 0 while the size is unknown.
func (i Item) Progress() float64 {
	if i.Total <= 0 {
		return 0
	}
	p := float64(i.Done) / float64(i.Total)
	if p > 1 {
		return 1
	}
	return p
}

// Summary counts the downloads and their bytes.
type Summary struct {
	Total      int
	Queued     int
	Active     int
	Paused     int
	Completed  int
	Failed     int
	DoneBytes  int64
	TotalBytes int64
	Speed      float64
}

// Record is how a download is saved and restored.
type Record struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	FileName        string    `json:"fileName"`
	Path            string    `json:"path"`
	State           State     `json:"state"`
	Total           int64     `json:"total"`
	Done            int64     `json:"done"`
	ETag            string    `json:"etag,omitempty"`
	LastModified    string    `json:"lastModified,omitempty"`
	Resumable       bool      `json:"resumable"`
	ResumableKnown  bool      `json:"resumableKnown,omitempty"`
	CustomName      bool      `json:"customName,omitempty"`
	HeadersRequired bool      `json:"headersRequired,omitempty"`
	RetryCount      int       `json:"retryCount,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
}
