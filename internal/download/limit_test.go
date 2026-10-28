package download

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestGlobalRateLimitSharedWithoutStall(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 12*1024)
	srv := rangeServer(t, content, len(content), 0, false)
	defer srv.Close()
	m := New(Options{Dir: t.TempDir(), AutoStart: true, MaxConcurrent: 2,
		RateLimit: 16 * 1024, StallTimeout: 60 * time.Millisecond})
	defer m.Shutdown()
	start := time.Now()
	first := addOne(t, m, srv.URL+"/a")
	second := addOne(t, m, srv.URL+"/b")
	for _, id := range []string{first, second} {
		it := waitState(t, m, id, StateCompleted)
		if it.RetryCount != 0 {
			t.Fatalf("rate waiting caused retries: %+v", it)
		}
		got, err := os.ReadFile(it.Path)
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("throttled file mismatch: %v", err)
		}
	}
	// Two independent per-task limits would finish in about 0.75 seconds.
	if elapsed := time.Since(start); elapsed < 1200*time.Millisecond || elapsed > 8*time.Second {
		t.Fatalf("aggregate rate did not share one budget: %v", elapsed)
	}
}

func TestRateLimitChangesRunningConnection(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 2*1024*1024)
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write(content)
	}))
	defer srv.Close()
	m := New(Options{Dir: t.TempDir(), AutoStart: true, RateLimit: 256 * 1024})
	defer m.Shutdown()
	id := addOne(t, m, srv.URL+"/dynamic")
	waitDownloading(t, m, id, 32*1024)
	m.SetRateLimit(4096)
	before := m.current(m.byID[id]).Done
	time.Sleep(300 * time.Millisecond)
	after := m.current(m.byID[id]).Done
	if after-before > 32*1024 {
		t.Fatalf("lowering the limit did not take effect: %d bytes", after-before)
	}
	start := time.Now()
	m.SetRateLimit(0)
	it := waitState(t, m, id, StateCompleted)
	if time.Since(start) > 2*time.Second || requests.Load() != 1 {
		t.Fatalf("unlimited mode was delayed or reconnected: requests=%d", requests.Load())
	}
	got, err := os.ReadFile(it.Path)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("dynamic throttling lost data: %v", err)
	}
}

func TestLowRatePauseAndShutdownCancelWaiting(t *testing.T) {
	srv := rangeServer(t, bytes.Repeat([]byte("a"), 1024), 1024, 0, false)
	defer srv.Close()
	m := New(Options{Dir: t.TempDir(), AutoStart: true, RateLimit: 1})
	defer m.Shutdown()
	id := addOne(t, m, srv.URL+"/slow")
	waitDownloading(t, m, id, 1)
	m.Pause(id)
	finished := make(chan struct{})
	go func() { m.Shutdown(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("shutdown waited for bandwidth instead of canceling")
	}
	items, _ := m.Snapshot()
	if items[0].State != StatePaused || items[0].Done != 1 {
		t.Fatalf("pause did not preserve progress: %+v", items)
	}
}
