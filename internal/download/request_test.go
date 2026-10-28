package download

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAdvancedDownloadConfigurationAndName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{"Referer": "https://source.example/", "Cookie": "session=private-cookie",
			"X-Token": "private-token", "User-Agent": "Custom Agent", "Accept-Encoding": "identity"} {
			if got := r.Header.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Disposition", `attachment; filename="server.bin"`)
		fmt.Fprint(w, "custom content")
	}))
	defer srv.Close()
	m := newTestManager(t, t.TempDir(), 2)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chosen.bin.part.meta"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"Referer": {"https://source.example/"}, "Cookie": {"session=private-cookie"},
		"X-Token": {"private-token"}, "User-Agent": {"Custom Agent"}}
	id, err := m.AddTask(AddOptions{URL: srv.URL + "/original", Dir: dir, FileName: "chosen.bin", Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("Cookie", "mutated")
	it := waitState(t, m, id, StateCompleted)
	if filepath.Dir(it.Path) != dir || it.FileName == "chosen.bin" || !strings.HasPrefix(it.FileName, "chosen (") {
		t.Fatalf("custom name, folder, or sidecar reservation lost: %+v", it)
	}
	got, err := os.ReadFile(it.Path)
	if err != nil || string(got) != "custom content" {
		t.Fatalf("file content = %q, %v", got, err)
	}
	records, _ := json.Marshal(m.Records())
	items, _ := json.Marshal([]Item{it})
	for _, secret := range []string{"private-cookie", "private-token", "Custom Agent"} {
		if bytes.Contains(records, []byte(secret)) || bytes.Contains(items, []byte(secret)) {
			t.Fatalf("request header leaked into snapshots or records: %s", secret)
		}
	}
}

func TestAdvancedValidation(t *testing.T) {
	for _, raw := range []string{"Range: bytes=0-", "If-Range: x", "Accept-Encoding: gzip", "Connection: close",
		"Host: elsewhere", "Content-Length: 100", "If-None-Match: x", "Proxy-Authorization: secret", "bad line", "X: value\rhidden"} {
		if _, err := ParseHeaders(raw); err == nil {
			t.Errorf("accepted unsafe or invalid headers %q", raw)
		}
	}
	headers, err := ParseHeaders("X-Test: a\nX-Test: b\nAuthorization: Bearer private\n")
	if err != nil || len(headers.Values("X-Test")) != 2 {
		t.Fatalf("valid headers = %v, %v", headers, err)
	}
	m := newTestManager(t, t.TempDir(), 1)
	for _, opts := range []AddOptions{
		{URL: "file:///a"}, {URL: "https://example.com/a", FileName: "../escape"},
		{URL: "https://example.com/a", Headers: http.Header{"Cookie": {"secret\r\nInjected: value"}}},
	} {
		if _, err := m.AddTask(opts); err == nil {
			t.Errorf("accepted invalid task: %+v", opts)
		}
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddTask(AddOptions{URL: "https://example.com/a", Dir: file}); err == nil {
		t.Fatal("accepted a non-writable directory")
	}
}

func TestRedirectsKeepHeadersOnlyWithinOrigin(t *testing.T) {
	var sourceURL string
	headers := http.Header{"Cookie": {"private-cookie"}, "Authorization": {"Bearer private-auth"},
		"Referer": {"https://source.example/private"}, "X-Token": {"private-token"}}
	assertStripped := func(r *http.Request) {
		t.Helper()
		for key := range headers {
			if got := r.Header.Get(key); got != "" {
				t.Errorf("header %s crossed origin: %q", key, got)
			}
		}
	}
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertStripped(r)
		http.Redirect(w, r, sourceURL+"/returned", http.StatusFound)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/returned" {
			assertStripped(r)
			fmt.Fprint(w, "safe")
			return
		}
		for key := range headers {
			if r.Header.Get(key) != headers.Get(key) {
				t.Errorf("same-origin redirect lost %s", key)
			}
		}
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/same-origin", http.StatusFound)
		} else {
			http.Redirect(w, r, destination.URL, http.StatusFound)
		}
	}))
	defer source.Close()
	sourceURL = source.URL
	m := newTestManager(t, t.TempDir(), 1)
	id, err := m.AddTask(AddOptions{URL: source.URL + "/start", Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, m, id, StateCompleted)
}

func TestAdvancedRestartCanRestoreHeadersAndProgress(t *testing.T) {
	content := bytes.Repeat([]byte("a"), 64*1024)
	srv := rangeServer(t, content, 1024, 0, false)
	defer srv.Close()
	dir := t.TempDir()
	m := New(Options{Dir: dir, AutoStart: true, RateLimit: 16 * 1024})
	t.Cleanup(m.Shutdown)
	id, err := m.AddTask(AddOptions{URL: srv.URL + "/file", FileName: "mine.bin", Headers: http.Header{"Cookie": {"private"}}})
	if err != nil {
		t.Fatal(err)
	}
	waitDownloading(t, m, id, 1024)
	m.Shutdown()
	records := m.Records()
	meta, err := os.ReadFile(records[0].Path + ".part.meta")
	if err != nil || bytes.Contains(meta, []byte("private")) {
		t.Fatalf("partial metadata leaked headers: %s, %v", meta, err)
	}
	restored := newTestManager(t, dir, 1)
	restored.Restore(records)
	restored.Resume(id)
	failed := waitState(t, restored, id, StateFailed)
	if !failed.NeedsHeaders {
		t.Fatal("restored task did not ask for missing headers")
	}
	updatedID, err := restored.AddTask(AddOptions{URL: srv.URL + "/file", Headers: http.Header{"Cookie": {"new-private"}}})
	if err != nil || updatedID != id {
		t.Fatalf("updating restored headers: id=%s, err=%v", updatedID, err)
	}
	it := waitState(t, restored, id, StateCompleted)
	got, err := os.ReadFile(it.Path)
	if err != nil || !bytes.Equal(got, content) || it.Path != records[0].Path || it.FileName != "mine.bin" {
		t.Fatalf("restored advanced task lost data or name: %+v, %v", it, err)
	}
}

type failingRequestTransport struct{}

func (failingRequestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	cookie := r.Header.Get("Cookie")
	if parsed, err := r.Cookie("session"); err == nil {
		cookie = parsed.Value
	}
	return nil, fmt.Errorf("transport rejected %s %s %s", cookie, r.Header.Get("X-Token"), r.Header.Get("Authorization"))
}

func TestAdvancedErrorsDoNotExposeCredentials(t *testing.T) {
	m := New(Options{Dir: t.TempDir(), AutoStart: true, ClientFactory: func() (*http.Client, error) {
		return &http.Client{Transport: failingRequestTransport{}}, nil
	}})
	defer m.Shutdown()
	id, err := m.AddTask(AddOptions{URL: "https://private-user:private-password@example.com/file?token=private-query",
		Headers: http.Header{"Cookie": {"session=private-cookie"}, "X-Token": {"private-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	it := waitState(t, m, id, StateFailed)
	for _, secret := range []string{"private-password", "private-user", "private-cookie", "private-token", "private-query",
		base64.StdEncoding.EncodeToString([]byte("private-user:private-password"))} {
		if strings.Contains(it.Error, secret) {
			t.Fatalf("error exposed %s: %s", secret, it.Error)
		}
	}
}

func TestBasicAuthDoesNotCrossOrigin(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("basic authentication crossed a redirect to a different origin")
		}
		fmt.Fprint(w, "ok")
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "secret" {
			t.Error("initial request lost basic authentication")
		}
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()
	m := newTestManager(t, t.TempDir(), 1)
	id := addOne(t, m, strings.Replace(source.URL, "://", "://user:secret@", 1))
	waitState(t, m, id, StateCompleted)
}

func TestRetryCountAndResumeCapabilityRemainAccurate(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, "retry", http.StatusServiceUnavailable)
			return
		}
		// A server without range support must stay non-resumable after completion.
		fmt.Fprint(w, "complete")
	}))
	defer srv.Close()
	m := newTestManager(t, t.TempDir(), 1)
	id := addOne(t, m, srv.URL)
	it := waitState(t, m, id, StateCompleted)
	if it.RetryCount != 1 || !it.ResumableKnown || it.Resumable {
		t.Fatalf("details lost retry history or invented resume support: %+v", it)
	}
	if m.Records()[0].RetryCount != 1 {
		t.Fatal("retry count was not persisted")
	}
}
