package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a Manager. The zero value is not usable: set Dir.
type Options struct {
	// Dir is where new downloads are saved.
	Dir string
	// MaxConcurrent is how many downloads run at once (default 3).
	MaxConcurrent int
	// AutoStart starts downloads as they are added.
	AutoStart bool
	// ClientFactory returns the HTTP client for a new connection. It is
	// called on worker goroutines and may change with the proxy settings.
	ClientFactory func() (*http.Client, error)
	// OnChange is called from any goroutine when a download changed.
	OnChange func()
	// OnComplete is called from a worker goroutine when a file is done.
	OnComplete func(Item)
	// StallTimeout is how long a silent connection may take before it is
	// retried (default 90s).
	StallTimeout time.Duration
}

// Manager is the download queue. Its methods are safe from any goroutine.
type Manager struct {
	mu       sync.Mutex
	opts     Options
	dir      string
	max      int
	auto     bool
	stall    time.Duration
	tasks    []*task
	byID     map[string]*task
	reserved map[string]bool
	active   int
	closed   bool
	wg       sync.WaitGroup

	lastNotify atomic.Int64
}

type task struct {
	item       Item
	user, pass string

	ctx     context.Context
	cancel  context.CancelFunc
	working bool
	removed bool
	done    chan struct{}

	etag         string
	lastModified string

	lastSample time.Time
	lastDone   int64
}

// New creates a manager.
func New(opts Options) *Manager {
	if opts.MaxConcurrent < 1 {
		opts.MaxConcurrent = 3
	}
	if opts.MaxConcurrent > 32 {
		opts.MaxConcurrent = 32
	}
	if opts.StallTimeout <= 0 {
		opts.StallTimeout = 90 * time.Second
	}
	if opts.ClientFactory == nil {
		opts.ClientFactory = func() (*http.Client, error) { return &http.Client{}, nil }
	}
	return &Manager{
		opts:     opts,
		dir:      opts.Dir,
		max:      opts.MaxConcurrent,
		auto:     opts.AutoStart,
		stall:    opts.StallTimeout,
		byID:     map[string]*task{},
		reserved: map[string]bool{},
	}
}

// SetOnChange replaces the change callback, which lets the app attach the
// window after it is created.
func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.opts.OnChange = fn
	m.mu.Unlock()
}

// SetOnComplete replaces the completion callback.
func (m *Manager) SetOnComplete(fn func(Item)) {
	m.mu.Lock()
	m.opts.OnComplete = fn
	m.mu.Unlock()
}

// Add parses the links in raw, one per line or space separated, and queues
// them. It returns how many were added, how many were skipped as duplicates of
// an unfinished download, and the tokens that were not valid links.
func (m *Manager) Add(raw string) (added int, skipped int, invalid []string) {
	targets, invalid := parseTargets(raw)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return 0, 0, invalid
	}
	for _, t := range targets {
		if m.hasUnfinishedLocked(t.display) {
			skipped++
			continue
		}
		name := "download"
		if u, err := url.Parse(t.display); err == nil {
			name = NameFromURL(u)
		}
		targetPath := uniquePath(m.dir, name, m.reserved)
		m.reserved[pathKey(targetPath)] = true
		state := StateQueued
		if !m.auto {
			// With auto-start off, a new download waits for the user, as
			// paused, rather than joining the active queue.
			state = StatePaused
		}
		item := Item{
			ID:         newID(),
			URL:        t.display,
			DisplayURL: t.display,
			FileName:   filepath.Base(targetPath),
			Path:       targetPath,
			State:      state,
			Total:      -1,
			CreatedAt:  timeNow(),
		}
		m.byID[item.ID] = &task{item: item, user: t.user, pass: t.pass}
		m.tasks = append(m.tasks, m.byID[item.ID])
		added++
	}
	auto := m.auto
	m.mu.Unlock()
	if added > 0 && auto {
		m.kick()
	}
	if added > 0 {
		m.notify()
	}
	return added, skipped, invalid
}

func (m *Manager) hasUnfinishedLocked(url string) bool {
	for _, t := range m.tasks {
		if t.item.URL == url && !t.item.State.Terminal() {
			return true
		}
	}
	return false
}

// Pause stops a queued or running download, keeping its partial file.
func (m *Manager) Pause(id string) {
	m.mu.Lock()
	t := m.byID[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	switch t.item.State {
	case StateQueued, StateConnecting, StateDownloading:
		t.item.State = StatePaused
		t.item.Speed = 0
		t.item.ETA = 0
		if t.cancel != nil {
			t.cancel()
		}
	}
	m.mu.Unlock()
	m.notify()
}

// Resume starts a paused download.
func (m *Manager) Resume(id string) {
	m.request(id)
}

// Retry starts a failed download over, from the bytes already saved when the
// server supports it.
func (m *Manager) Retry(id string) {
	m.request(id)
}

func (m *Manager) request(id string) {
	m.mu.Lock()
	t := m.byID[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	switch t.item.State {
	case StatePaused, StateFailed:
		t.item.State = StateQueued
		t.item.Error = ""
		t.item.Attempt = 0
		t.item.Missing = false
	case StateCompleted:
		if !t.item.Missing {
			m.mu.Unlock()
			return
		}
		// The file is gone: download it again.
		t.item.State = StateQueued
		t.item.Missing = false
		t.item.Error = ""
		t.item.Done = 0
		t.item.Total = -1
		t.item.FinishedAt = time.Time{}
	default:
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.notify()
	m.kick()
}

// Remove forgets a download and deletes its partial file. With deleteFile the
// finished file goes too.
func (m *Manager) Remove(id string, deleteFile bool) {
	m.RemoveMany([]string{id}, deleteFile)
}

// RemoveMany removes several downloads.
func (m *Manager) RemoveMany(ids []string, deleteFiles bool) {
	m.mu.Lock()
	var removed []*task
	for _, id := range ids {
		t := m.byID[id]
		if t == nil {
			continue
		}
		delete(m.byID, id)
		t.removed = true
		if t.cancel != nil {
			t.cancel()
		}
		for i, other := range m.tasks {
			if other == t {
				m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
				break
			}
		}
		removed = append(removed, t)
	}
	m.mu.Unlock()
	for _, t := range removed {
		m.removeFiles(t, deleteFiles)
	}
	m.notify()
}

// removeFiles deletes a removed download's files once its worker has stopped.
func (m *Manager) removeFiles(t *task, deleteFinal bool) {
	m.mu.Lock()
	path := t.item.Path
	working, done := t.working, t.done
	m.mu.Unlock()
	if !working || done == nil {
		deleteFilesFor(path, deleteFinal)
		m.releaseReservation(path)
		return
	}
	go func() {
		<-done
		deleteFilesFor(path, deleteFinal)
		m.releaseReservation(path)
	}()
}

func deleteFilesFor(path string, deleteFinal bool) {
	_ = os.Remove(path + ".part")
	_ = os.Remove(path + ".part.meta")
	if deleteFinal {
		_ = os.Remove(path)
	}
}

// PauseAll pauses everything that is queued or running.
func (m *Manager) PauseAll() {
	m.mu.Lock()
	for _, t := range m.tasks {
		switch t.item.State {
		case StateQueued, StateConnecting, StateDownloading:
			t.item.State = StatePaused
			t.item.Speed = 0
			t.item.ETA = 0
			if t.cancel != nil {
				t.cancel()
			}
		}
	}
	m.mu.Unlock()
	m.notify()
}

// ResumeAll queues every paused or failed download.
func (m *Manager) ResumeAll() {
	m.mu.Lock()
	for _, t := range m.tasks {
		switch t.item.State {
		case StatePaused, StateFailed:
			t.item.State = StateQueued
			t.item.Error = ""
			t.item.Attempt = 0
		case StateCompleted:
			if t.item.Missing {
				t.item.State = StateQueued
				t.item.Missing = false
				t.item.Error = ""
				t.item.Done = 0
				t.item.Total = -1
				t.item.FinishedAt = time.Time{}
			}
		}
	}
	m.mu.Unlock()
	m.notify()
	m.kick()
}

// ClearCompleted forgets the finished downloads, keeping their files.
func (m *Manager) ClearCompleted() {
	m.mu.Lock()
	var ids []string
	for _, t := range m.tasks {
		if t.item.State == StateCompleted {
			ids = append(ids, t.item.ID)
		}
	}
	m.mu.Unlock()
	m.RemoveMany(ids, false)
}

// SetDir changes where new downloads are saved.
func (m *Manager) SetDir(dir string) {
	if dir == "" {
		return
	}
	m.mu.Lock()
	m.dir = dir
	m.mu.Unlock()
}

// SetMaxConcurrent changes how many downloads run at once.
func (m *Manager) SetMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	m.mu.Lock()
	m.max = n
	m.mu.Unlock()
	m.kick()
}

// SetAutoStart says whether added downloads start at once.
func (m *Manager) SetAutoStart(on bool) {
	m.mu.Lock()
	m.auto = on
	m.mu.Unlock()
}

// Snapshot returns the downloads and their counts for the UI.
func (m *Manager) Snapshot() ([]Item, Summary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Item, 0, len(m.tasks))
	var sum Summary
	for _, t := range m.tasks {
		item := t.item
		items = append(items, item)
		sum.Total++
		switch item.State {
		case StateQueued:
			sum.Queued++
			sum.Active++
		case StateConnecting, StateDownloading:
			sum.Active++
		case StatePaused:
			sum.Paused++
		case StateCompleted:
			sum.Completed++
		case StateFailed:
			sum.Failed++
		}
		if item.State == StateCompleted {
			sum.DoneBytes += item.Done
		} else {
			sum.DoneBytes += item.Done
			if item.Total > 0 {
				sum.TotalBytes += item.Total
			}
		}
		if item.State == StateDownloading {
			sum.Speed += item.Speed
		}
	}
	return items, sum
}

// Records returns what to save for the next run. Working downloads are saved
// as paused.
func (m *Manager) Records() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.tasks))
	for _, t := range m.tasks {
		state := t.item.State
		if state.Active() {
			state = StatePaused
		}
		out = append(out, Record{
			ID:           t.item.ID,
			URL:          t.item.URL,
			FileName:     t.item.FileName,
			Path:         t.item.Path,
			State:        state,
			Total:        t.item.Total,
			Done:         t.item.Done,
			ETag:         t.etag,
			LastModified: t.lastModified,
			Resumable:    t.item.Resumable,
			CreatedAt:    t.item.CreatedAt,
			FinishedAt:   t.item.FinishedAt,
		})
	}
	return out
}

// Restore loads saved downloads. Anything unfinished waits, paused, for the
// user to continue it.
func (m *Manager) Restore(records []Record) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range records {
		if r.ID == "" || r.URL == "" || r.Path == "" {
			continue
		}
		if _, ok := m.byID[r.ID]; ok {
			continue
		}
		name := filepath.Base(r.Path)
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = r.FileName
		}
		item := Item{
			ID:         r.ID,
			URL:        r.URL,
			DisplayURL: r.URL,
			FileName:   name,
			Path:       r.Path,
			State:      StatePaused,
			Total:      r.Total,
			Done:       r.Done,
			CreatedAt:  r.CreatedAt,
			FinishedAt: r.FinishedAt,
			Resumable:  r.Resumable,
			Missing:    false,
		}
		t := &task{item: item, etag: r.ETag, lastModified: r.LastModified}
		meta := readPartMeta(r.Path + ".part.meta")
		if t.etag == "" {
			t.etag = meta.ETag
		}
		if t.lastModified == "" {
			t.lastModified = meta.LastModified
		}
		if r.State == StateCompleted {
			if fileExists(r.Path) {
				item.State = StateCompleted
			} else {
				item.State = StateCompleted
				item.Missing = true
			}
		} else {
			part := fileSize(r.Path + ".part")
			item.Done = part
			if item.Total <= 0 {
				item.Total = meta.Total
			}
			if item.Total <= 0 {
				item.Total = -1
			}
			item.Resumable = item.Resumable || meta.Resumable
		}
		t.item = item
		m.reserved[pathKey(item.Path)] = true
		m.tasks = append(m.tasks, t)
		m.byID[item.ID] = t
	}
}

// Shutdown stops every worker and leaves the downloads paused. It waits for
// them to release their files.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.closed = true
	for _, t := range m.tasks {
		if t.item.State.Active() {
			t.item.State = StatePaused
			t.item.Speed = 0
			t.item.ETA = 0
			if t.cancel != nil {
				t.cancel()
			}
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.notify()
}

// kick starts queued downloads while there is room.
func (m *Manager) kick() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	started := false
	for m.active < m.max {
		var next *task
		for _, t := range m.tasks {
			if !t.working && t.item.State == StateQueued {
				next = t
				break
			}
		}
		if next == nil {
			break
		}
		next.working = true
		next.item.State = StateConnecting
		next.item.Error = ""
		next.item.Attempt = 0
		ctx, cancel := context.WithCancel(context.Background())
		next.ctx, next.cancel = ctx, cancel
		next.done = make(chan struct{})
		m.active++
		m.wg.Add(1)
		started = true
		go m.run(next)
	}
	m.mu.Unlock()
	if started {
		m.notify()
	}
}

func (m *Manager) run(t *task) {
	defer m.finishWorker(t)
	err := m.download(t.ctx, t)
	if err == nil || errors.Is(err, errRemoved) {
		return
	}
	if t.ctx.Err() != nil {
		return
	}
	m.fail(t, err)
}

func (m *Manager) finishWorker(t *task) {
	m.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.working = false
	if m.active > 0 {
		m.active--
	}
	if t.done != nil {
		close(t.done)
		t.done = nil
	}
	if !t.removed && (t.item.State == StateConnecting || t.item.State == StateDownloading) {
		t.item.State = StatePaused
		t.item.Speed = 0
		t.item.ETA = 0
	}
	if t.done != nil {
		close(t.done)
		t.done = nil
	}
	m.mu.Unlock()
	m.wg.Done()
	m.notify()
	m.kick()
}

func (m *Manager) fail(t *task, err error) {
	m.mu.Lock()
	if t.removed {
		m.mu.Unlock()
		return
	}
	t.item.State = StateFailed
	t.item.Error = errorText(err)
	t.item.Speed = 0
	t.item.ETA = 0
	t.item.FinishedAt = timeNow()
	m.mu.Unlock()
	m.notify()
}

// download runs an attempt and its retries.
func (m *Manager) download(ctx context.Context, t *task) error {
	client, err := m.opts.ClientFactory()
	if err != nil {
		return fmt.Errorf("无法建立网络连接：%w", err)
	}
	const attempts = 3
	restarts := 0
	for attempt := 1; attempt <= attempts; {
		err := m.fetch(ctx, t, client)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errRemoved) {
			return err
		}
		if errors.Is(err, errRestart) {
			restarts++
			if restarts > 2 {
				return fmt.Errorf("服务器拒绝了续传请求")
			}
			continue
		}
		if !retryable(err) || attempt == attempts {
			return err
		}
		m.setRetrying(t, attempt, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
		}
		attempt++
	}
	return nil
}

func (m *Manager) setRetrying(t *task, attempt int, err error) {
	m.mu.Lock()
	t.item.State = StateConnecting
	t.item.Attempt = attempt
	t.item.Error = fmt.Sprintf("正在重试（%d/%d）：%s", attempt, 2, errorText(err))
	t.item.Speed = 0
	t.item.ETA = 0
	m.mu.Unlock()
	m.notify()
}

func (m *Manager) notify() {
	m.mu.Lock()
	fn := m.opts.OnChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Manager) notifyThrottled() {
	now := time.Now().UnixNano()
	last := m.lastNotify.Load()
	if now-last < int64(200*time.Millisecond) {
		return
	}
	if !m.lastNotify.CompareAndSwap(last, now) {
		return
	}
	m.notify()
}

func (m *Manager) current(t *task) Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	return t.item
}

func (m *Manager) releaseReservation(path string) {
	if path == "" {
		return
	}
	m.mu.Lock()
	delete(m.reserved, pathKey(path))
	m.mu.Unlock()
}
