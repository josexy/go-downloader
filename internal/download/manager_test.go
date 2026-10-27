package download

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testTimeout = 20 * time.Second

func newTestManager(t *testing.T, dir string, maxConcurrent int) *Manager {
	t.Helper()
	m := New(Options{
		Dir:           dir,
		MaxConcurrent: maxConcurrent,
		AutoStart:     true,
		ClientFactory: func() (*http.Client, error) { return &http.Client{}, nil },
		StallTimeout:  5 * time.Second,
	})
	t.Cleanup(m.Shutdown)
	return m
}

func addOne(t *testing.T, m *Manager, rawURL string) string {
	t.Helper()
	before := map[string]bool{}
	items, _ := m.Snapshot()
	for _, it := range items {
		before[it.ID] = true
	}
	added, _, invalid := m.Add(rawURL)
	if added != 1 {
		t.Fatalf("Add(%q) added %d, invalid %v", rawURL, added, invalid)
	}
	items, _ = m.Snapshot()
	for _, it := range items {
		if !before[it.ID] {
			return it.ID
		}
	}
	t.Fatalf("added download %q is not in the snapshot", rawURL)
	return ""
}

func itemByID(items []Item, id string) (Item, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

func waitState(t *testing.T, m *Manager, id string, want State) Item {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for {
		items, _ := m.Snapshot()
		if it, ok := itemByID(items, id); ok && it.State == want {
			return it
		}
		if time.Now().After(deadline) {
			it, _ := itemByID(items, id)
			t.Fatalf("timeout waiting for %s: %+v", want, it)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitDownloading(t *testing.T, m *Manager, id string, done int64) Item {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for {
		items, _ := m.Snapshot()
		if it, ok := itemByID(items, id); ok && it.State == StateDownloading && it.Done >= done {
			return it
		}
		if time.Now().After(deadline) {
			it, _ := itemByID(items, id)
			t.Fatalf("timeout waiting for %d bytes: %+v", done, it)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// rangeServer serves content, honouring Range when honour ranges is true.
func rangeServer(t *testing.T, content []byte, chunk int, delay time.Duration, ignoreRange bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := int64(0)
		if !ignoreRange {
			if v := r.Header.Get("Range"); strings.HasPrefix(v, "bytes=") {
				spec := strings.TrimPrefix(v, "bytes=")
				spec = strings.TrimSuffix(spec, "-")
				if n, err := strconv.ParseInt(spec, 10, 64); err == nil && n >= 0 && n <= int64(len(content)) {
					start = n
				}
			}
		}
		if start >= int64(len(content)) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(content)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		body := content[start:]
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if start > 0 && !ignoreRange {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(content)-1, len(content)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		flusher, _ := w.(http.Flusher)
		for off := 0; off < len(body); off += chunk {
			end := min(off+chunk, len(body))
			if _, err := w.Write(body[off:end]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}))
}

func TestDownloadCompletes(t *testing.T) {
	content := bytes.Repeat([]byte("abc123"), 20000)
	srv := rangeServer(t, content, 64*1024, 0, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 2)
	completed := make(chan Item, 1)
	m.SetOnComplete(func(it Item) {
		select {
		case completed <- it:
		default:
		}
	})
	id := addOne(t, m, srv.URL+"/files/archive.zip")
	it := waitState(t, m, id, StateCompleted)
	if it.Done != int64(len(content)) || it.Total != int64(len(content)) {
		t.Errorf("done %d total %d, want %d", it.Done, it.Total, len(content))
	}
	got, err := os.ReadFile(it.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Error("the saved file differs from the source")
	}
	select {
	case <-completed:
	case <-time.After(testTimeout):
		t.Error("OnComplete was not called")
	}
	_, sum := m.Snapshot()
	if sum.Completed != 1 || sum.DoneBytes != int64(len(content)) {
		t.Errorf("summary = %+v", sum)
	}
}

// TestAutoStartOffAddsPaused checks that a manager that does not start added
// downloads keeps them paused until asked, outside the active downloads.
func TestAutoStartOffAddsPaused(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("waited for the user"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := New(Options{
		Dir:           dir,
		MaxConcurrent: 1,
		AutoStart:     false,
		ClientFactory: func() (*http.Client, error) { return &http.Client{}, nil },
		StallTimeout:  5 * time.Second,
	})
	t.Cleanup(m.Shutdown)
	added, _, invalid := m.Add(srv.URL + "/file.txt")
	if added != 1 {
		t.Fatalf("added %d, invalid %v", added, invalid)
	}
	items, summary := m.Snapshot()
	if len(items) != 1 || items[0].State != StatePaused {
		t.Fatalf("items after add = %+v", items)
	}
	if summary.Paused != 1 || summary.Active != 0 || summary.Queued != 0 {
		t.Errorf("summary = %+v", summary)
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("the server got %d requests before the user started it", n)
	}
	m.Resume(items[0].ID)
	waitState(t, m, items[0].ID, StateCompleted)
	if requests.Load() == 0 {
		t.Error("resuming the paused download did not request it")
	}
}

func TestContentDispositionName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''%E6%8A%A5%E5%91%8A.pdf`)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("pdf"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/download")
	it := waitState(t, m, id, StateCompleted)
	if it.FileName != "报告.pdf" {
		t.Errorf("file name = %q, want 报告.pdf", it.FileName)
	}
	if _, err := os.Stat(filepath.Join(dir, "报告.pdf")); err != nil {
		t.Errorf("saved file: %v", err)
	}
}

func TestContentTypeExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("zip"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/download")
	it := waitState(t, m, id, StateCompleted)
	if it.FileName != "download.zip" {
		t.Errorf("file name = %q, want download.zip", it.FileName)
	}
}

func TestRedirectAndDisposition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="final.bin"`)
		_, _ = w.Write([]byte("redirected"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/start")
	it := waitState(t, m, id, StateCompleted)
	if it.FileName != "final.bin" {
		t.Errorf("file name = %q, want final.bin", it.FileName)
	}
	body, err := os.ReadFile(it.Path)
	if err != nil || string(body) != "redirected" {
		t.Errorf("body = %q, %v", body, err)
	}
}

func TestPauseResume(t *testing.T) {
	content := bytes.Repeat([]byte("0123456789"), 60000)
	srv := rangeServer(t, content, 8*1024, 2*time.Millisecond, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/big.bin")
	running := waitDownloading(t, m, id, 16*1024)
	m.Pause(id)
	waitState(t, m, id, StatePaused)
	part := running.Path + ".part"
	if fileSize(part) == 0 {
		t.Fatal("pausing kept no partial file")
	}
	m.Resume(id)
	it := waitState(t, m, id, StateCompleted)
	got, err := os.ReadFile(it.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("the resumed file differs: %d bytes, want %d", len(got), len(content))
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Error("the partial file remains after completion")
	}
}

func TestServerIgnoresRange(t *testing.T) {
	content := bytes.Repeat([]byte("xy"), 120000)
	srv := rangeServer(t, content, 4*1024, 2*time.Millisecond, true)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file.dat")
	waitDownloading(t, m, id, 8*1024)
	m.Pause(id)
	waitState(t, m, id, StatePaused)
	m.Resume(id)
	it := waitState(t, m, id, StateCompleted)
	got, err := os.ReadFile(it.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("the restarted file differs: %d bytes, want %d", len(got), len(content))
	}
}

func TestETagMismatchRestarts(t *testing.T) {
	first := bytes.Repeat([]byte("a"), 400000)
	second := bytes.Repeat([]byte("b"), 120000)
	var mu sync.Mutex
	current := first
	modTime := time.Unix(1700000000, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		data := current
		mod := modTime
		mu.Unlock()
		lastModified := mod.UTC().Format(http.TimeFormat)
		w.Header().Set("Last-Modified", lastModified)
		w.Header().Set("Accept-Ranges", "bytes")
		start := 0
		if ifRange := r.Header.Get("If-Range"); ifRange == "" || ifRange == lastModified {
			if v := r.Header.Get("Range"); strings.HasPrefix(v, "bytes=") {
				spec := strings.TrimSuffix(strings.TrimPrefix(v, "bytes="), "-")
				if n, err := strconv.Atoi(spec); err == nil && n >= 0 && n <= len(data) {
					start = n
				}
			}
		}
		body := data[start:]
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		flusher, _ := w.(http.Flusher)
		for off := 0; off < len(body); off += 4096 {
			end := min(off+4096, len(body))
			if _, err := w.Write(body[off:end]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(2 * time.Millisecond)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file.bin")
	waitDownloading(t, m, id, 8*1024)
	m.Pause(id)
	waitState(t, m, id, StatePaused)
	mu.Lock()
	current = second
	modTime = time.Unix(1800000000, 0)
	mu.Unlock()
	m.Resume(id)
	it := waitState(t, m, id, StateCompleted)
	got, err := os.ReadFile(it.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, second) {
		t.Errorf("resuming over a changed file kept stale bytes: %d bytes, want %d", len(got), len(second))
	}
}

func TestResumeAcrossRestart(t *testing.T) {
	content := bytes.Repeat([]byte("restart-"), 30000)
	srv := rangeServer(t, content, 8*1024, 2*time.Millisecond, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file.bin")
	waitDownloading(t, m, id, 8*1024)
	m.Pause(id)
	m.Shutdown()
	records := m.Records()
	if len(records) != 1 || records[0].State != StatePaused {
		t.Fatalf("records = %+v", records)
	}
	partSize := fileSize(records[0].Path + ".part")
	if partSize == 0 {
		t.Fatal("no partial file before the restart")
	}

	m2 := newTestManager(t, dir, 1)
	m2.Restore(records)
	m2.SetAutoStart(true)
	items, _ := m2.Snapshot()
	if len(items) != 1 || items[0].Done != partSize {
		t.Fatalf("restored items = %+v", items)
	}
	m2.Resume(items[0].ID)
	it := waitState(t, m2, items[0].ID, StateCompleted)
	got, err := os.ReadFile(it.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("the file resumed after a restart differs: %d bytes, want %d", len(got), len(content))
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var active, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		time.Sleep(40 * time.Millisecond)
		w.Header().Set("Content-Length", "16")
		_, _ = w.Write([]byte("0123456789abcdef"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 2)
	for i := 0; i < 5; i++ {
		addOne(t, m, fmt.Sprintf("%s/file%d.bin", srv.URL, i))
	}
	deadline := time.Now().Add(testTimeout)
	for {
		items, _ := m.Snapshot()
		done := 0
		for _, it := range items {
			if it.State == StateCompleted {
				done++
			}
		}
		if done == len(items) && len(items) == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %+v", items)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := peak.Load(); got != 2 {
		t.Errorf("peak concurrent downloads = %d, want 2", got)
	}
}

func TestRetriesServerErrors(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "later", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file")
	waitState(t, m, id, StateCompleted)
	if requests.Load() != 2 {
		t.Errorf("requests = %d, want 2", requests.Load())
	}
}

func TestNotFoundFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/missing.bin")
	it := waitState(t, m, id, StateFailed)
	if !strings.Contains(it.Error, "不存在") {
		t.Errorf("error = %q", it.Error)
	}
}

func TestRemoveDeletesPartial(t *testing.T) {
	content := bytes.Repeat([]byte("z"), 200000)
	srv := rangeServer(t, content, 4*1024, 2*time.Millisecond, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file.bin")
	it := waitDownloading(t, m, id, 8*1024)
	part := it.Path + ".part"
	m.Remove(id, false)
	deadline := time.Now().Add(testTimeout)
	for fileExists(part) {
		if time.Now().After(deadline) {
			t.Fatal("the partial file was not deleted")
		}
		time.Sleep(5 * time.Millisecond)
	}
	items, _ := m.Snapshot()
	if len(items) != 0 {
		t.Errorf("items after remove = %+v", items)
	}
}

func TestRemoveKeepsCompletedFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("keep me"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/keep.txt")
	it := waitState(t, m, id, StateCompleted)
	m.Remove(id, false)
	if !fileExists(it.Path) {
		t.Error("removing from the list deleted the file")
	}

	id2 := addOne(t, m, srv.URL+"/keep.txt")
	it2 := waitState(t, m, id2, StateCompleted)
	m.Remove(id2, true)
	deadline := time.Now().Add(testTimeout)
	for fileExists(it2.Path) {
		if time.Now().After(deadline) {
			t.Fatal("the file was not deleted")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestUniqueNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("same"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id1 := addOne(t, m, srv.URL+"/same.txt")
	it1 := waitState(t, m, id1, StateCompleted)
	id2 := addOne(t, m, srv.URL+"/same.txt")
	it2 := waitState(t, m, id2, StateCompleted)
	if it1.Path == it2.Path {
		t.Fatalf("both downloads saved to %q", it1.Path)
	}
	if it2.FileName != "same (2).txt" {
		t.Errorf("second file name = %q", it2.FileName)
	}
}

func TestDuplicateUnfinishedSkipped(t *testing.T) {
	content := bytes.Repeat([]byte("q"), 100000)
	srv := rangeServer(t, content, 8*1024, 2*time.Millisecond, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	url := srv.URL + "/file.bin"
	addOne(t, m, url)
	added, skipped, _ := m.Add(url)
	if added != 0 || skipped != 1 {
		t.Errorf("second Add = %d added, %d skipped", added, skipped)
	}
}

func TestUnknownLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			_, _ = w.Write([]byte("chunk"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/stream")
	it := waitState(t, m, id, StateCompleted)
	if it.Done != 25 {
		t.Errorf("done = %d, want 25", it.Done)
	}
}

func TestZeroLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/empty")
	it := waitState(t, m, id, StateCompleted)
	fi, err := os.Stat(it.Path)
	if err != nil || fi.Size() != 0 {
		t.Fatalf("stat = %v, %v", fi, err)
	}
}

func TestBasicAuthFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ada" || pass != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("private"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	raw := strings.Replace(srv.URL, "http://", "http://ada:s3cret@", 1) + "/private.txt"
	id := addOne(t, m, raw)
	it := waitState(t, m, id, StateCompleted)
	if strings.Contains(it.URL, "s3cret") {
		t.Errorf("the stored URL keeps the password: %q", it.URL)
	}
}

func TestRestoreMissingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("again"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/again.txt")
	it := waitState(t, m, id, StateCompleted)
	records := m.Records()
	if err := os.Remove(it.Path); err != nil {
		t.Fatal(err)
	}
	m2 := newTestManager(t, dir, 1)
	m2.Restore(records)
	items, _ := m2.Snapshot()
	if len(items) != 1 || !items[0].Missing {
		t.Fatalf("restored items = %+v", items)
	}
	m2.Retry(items[0].ID)
	waitState(t, m2, items[0].ID, StateCompleted)
	if !fileExists(it.Path) {
		t.Error("retrying a missing file did not download it again")
	}
}

func TestClearCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("done"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/done.txt")
	it := waitState(t, m, id, StateCompleted)
	m.ClearCompleted()
	items, _ := m.Snapshot()
	if len(items) != 0 {
		t.Errorf("items = %+v", items)
	}
	if !fileExists(it.Path) {
		t.Error("clearing completed downloads deleted the file")
	}
}

func TestShutdownLeavesPaused(t *testing.T) {
	content := bytes.Repeat([]byte("s"), 300000)
	srv := rangeServer(t, content, 4*1024, 3*time.Millisecond, false)
	defer srv.Close()
	dir := t.TempDir()
	m := newTestManager(t, dir, 1)
	id := addOne(t, m, srv.URL+"/file.bin")
	waitDownloading(t, m, id, 8*1024)
	m.Shutdown()
	items, _ := m.Snapshot()
	if len(items) != 1 || items[0].State != StatePaused {
		t.Fatalf("items after shutdown = %+v", items)
	}
}

func TestNormalizeURLs(t *testing.T) {
	valid, invalid := NormalizeURLs("https://example.com/a.zip\nexample.com/b.zip\nftp://example.com/c\n")
	if len(valid) != 2 {
		t.Errorf("valid = %v", valid)
	}
	if valid[0] != "https://example.com/a.zip" || valid[1] != "https://example.com/b.zip" {
		t.Errorf("valid = %v", valid)
	}
	if len(invalid) != 1 {
		t.Errorf("invalid = %v", invalid)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"report.pdf":     "report.pdf",
		`..\..\evil.txt`: "evil.txt",
		`a<b>c:d".txt`:   "abcd.txt",
		"CON.txt":        "_CON.txt",
		"trailing. ":     "trailing",
		"   ":            "download",
		"日本語のファイル.zip":   "日本語のファイル.zip",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("名", 200) + ".pdf"
	if got := Sanitize(long); len(got) > 180 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("Sanitize(long) is %d bytes: %q", len(got), got)
	}
}

func TestParseContentRange(t *testing.T) {
	start, total, ok := parseContentRange("bytes 100-199/1234")
	if !ok || start != 100 || total != 1234 {
		t.Errorf("parseContentRange = %d %d %v", start, total, ok)
	}
	_, total, ok = parseContentRange("bytes 0-9/*")
	if !ok || total != -1 {
		t.Errorf("unknown total = %d %v", total, ok)
	}
	if _, _, ok := parseContentRange("nonsense"); ok {
		t.Error("nonsense parsed")
	}
	size, ok := parseUnsatisfiedRange("bytes */42")
	if !ok || size != 42 {
		t.Errorf("parseUnsatisfiedRange = %d %v", size, ok)
	}
}

func TestStallFails(t *testing.T) {
	content := bytes.Repeat([]byte("p"), 100000)
	var served atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		if served.CompareAndSwap(false, true) {
			_, _ = w.Write(content[:1000])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(600 * time.Millisecond)
			_, _ = w.Write(content[1000:2000])
			return
		}
		_, _ = w.Write(content)
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := New(Options{
		Dir:           dir,
		MaxConcurrent: 1,
		AutoStart:     true,
		ClientFactory: func() (*http.Client, error) { return &http.Client{}, nil },
		StallTimeout:  100 * time.Millisecond,
	})
	t.Cleanup(m.Shutdown)
	id := addOne(t, m, srv.URL+"/stall.bin")
	deadline := time.Now().Add(testTimeout)
	for {
		items, _ := m.Snapshot()
		it, ok := itemByID(items, id)
		if ok && it.State == StateCompleted {
			return // the retry finished the file
		}
		if ok && it.State == StateFailed {
			return // the stall was reported
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %+v", it)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProxyClient(t *testing.T) {
	// The manager uses the factory's client, which the app swaps with the
	// proxy settings; a transport that always fails proves the factory is
	// consulted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("never"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := New(Options{
		Dir:           dir,
		MaxConcurrent: 1,
		AutoStart:     true,
		StallTimeout:  5 * time.Second,
		ClientFactory: func() (*http.Client, error) {
			return &http.Client{Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					return nil, io.ErrClosedPipe
				},
			}}, nil
		},
	})
	t.Cleanup(m.Shutdown)
	id := addOne(t, m, srv.URL+"/file")
	it := waitState(t, m, id, StateFailed)
	if it.Error == "" {
		t.Error("no error text")
	}
}
