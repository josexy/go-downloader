package download

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadPreservesExistingSidecars(t *testing.T) {
	for _, suffix := range []string{".part", ".part.meta"} {
		t.Run(suffix, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				fmt.Fprint(w, "new payload")
			}))
			defer srv.Close()
			dir := t.TempDir()
			existing := filepath.Join(dir, "file.bin"+suffix)
			original := []byte("a normal user file")
			if err := os.WriteFile(existing, original, 0o600); err != nil {
				t.Fatal(err)
			}
			m := newTestManager(t, dir, 1)
			id := addOne(t, m, srv.URL+"/file.bin")
			it := waitState(t, m, id, StateCompleted)
			if got, err := os.ReadFile(existing); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("existing file changed: %q, %v", got, err)
			}
			if got, err := os.ReadFile(it.Path); err != nil || string(got) != "new payload" {
				t.Fatalf("download = %q, %v", got, err)
			}
			m.Remove(id, false)
			if got, err := os.ReadFile(existing); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("removing the task changed the existing file: %q, %v", got, err)
			}
		})
	}
}

func TestDownloadReservesTemporaryPaths(t *testing.T) {
	for _, names := range [][2]string{
		{"file.bin", "file.bin.part"},
		{"file.bin.part", "file.bin"},
		{"file.bin", "file.bin.part.meta"},
		{"file.bin.part.meta", "file.bin"},
	} {
		t.Run(strings.Join(names[:], "+"), func(t *testing.T) {
			m := newTestManager(t, t.TempDir(), 1)
			m.SetAutoStart(false)
			for _, name := range names {
				addOne(t, m, "http://example.invalid/"+name)
			}
			items, _ := m.Snapshot()
			paths := map[string]bool{}
			for _, item := range items {
				for _, p := range []string{item.Path, item.Path + ".part", item.Path + ".part.meta"} {
					if paths[p] {
						t.Fatalf("two downloads share %s", p)
					}
					paths[p] = true
				}
			}
		})
	}
}

func restoreTestPartial(t *testing.T, rawURL, prefix string, total int64, etag string) *Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(path+".part", []byte(prefix), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePartMeta(path+".part.meta", partMeta{URL: rawURL, ETag: etag, Total: total, Resumable: true}); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, dir, 1)
	m.Restore([]Record{{ID: "restored", URL: rawURL, Path: path, State: StatePaused, Total: total, ETag: etag}})
	return m
}

func TestResumeWithoutValidatorRestarts(t *testing.T) {
	for _, etag := range []string{"", `W/"old"`} {
		t.Run(etag, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				if r.Header.Get("Range") != "" {
					w.Header().Set("Content-Range", "bytes 4-7/8")
					w.WriteHeader(http.StatusPartialContent)
					fmt.Fprint(w, "BBBB")
					return
				}
				fmt.Fprint(w, "BBBBBBBB")
			}))
			defer srv.Close()
			m := restoreTestPartial(t, srv.URL+"/file.bin", "AAAA", 8, etag)
			m.Resume("restored")
			it := waitState(t, m, "restored", StateCompleted)
			if got, err := os.ReadFile(it.Path); err != nil || string(got) != "BBBBBBBB" {
				t.Fatalf("mixed file versions: %q, %v", got, err)
			}
		})
	}
}

func TestResumeRejectsChangedValidator(t *testing.T) {
	for _, status := range []int{http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			prefix := "AAAA"
			if status == http.StatusRequestedRangeNotSatisfiable {
				prefix = "AAAAAAAA"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("ETag", `"new"`)
				if r.Header.Get("Range") != "" {
					if status == http.StatusPartialContent {
						w.Header().Set("Content-Range", "bytes 4-7/8")
					} else {
						w.Header().Set("Content-Range", "bytes */8")
					}
					w.WriteHeader(status)
					if status == http.StatusPartialContent {
						fmt.Fprint(w, "BBBB")
					}
					return
				}
				fmt.Fprint(w, "BBBBBBBB")
			}))
			defer srv.Close()
			m := restoreTestPartial(t, srv.URL+"/file.bin", prefix, 8, `"old"`)
			m.Resume("restored")
			it := waitState(t, m, "restored", StateCompleted)
			if got, err := os.ReadFile(it.Path); err != nil || string(got) != "BBBBBBBB" {
				t.Fatalf("changed representation accepted: %q, %v", got, err)
			}
		})
	}
}

func TestFullResponseClearsOldValidator(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "8")
		if requests.Add(1) == 1 {
			// Range was ignored: this new representation has no validators.
			fmt.Fprint(w, "BBBB")
			return
		}
		if r.Header.Get("Range") != "" {
			t.Error("retried using a validator from the old representation")
		}
		fmt.Fprint(w, "BBBBBBBB")
	}))
	defer srv.Close()
	m := restoreTestPartial(t, srv.URL+"/file.bin", "AAAA", 8, `"old"`)
	m.Resume("restored")
	it := waitState(t, m, "restored", StateCompleted)
	if got, err := os.ReadFile(it.Path); err != nil || string(got) != "BBBBBBBB" {
		t.Fatalf("download = %q, %v", got, err)
	}
}

func TestReaddRestoredAuthentication(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(fmt.Sprint(auto), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "alice" || pass != "secret" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("ETag", `"same"`)
				if r.Header.Get("Range") == "bytes=4-" && r.Header.Get("If-Range") == `"same"` {
					w.Header().Set("Content-Range", "bytes 4-7/8")
					w.WriteHeader(http.StatusPartialContent)
					fmt.Fprint(w, "AAAA")
					return
				}
				t.Error("re-adding credentials lost the resumable partial file")
				fmt.Fprint(w, "AAAAAAAA")
			}))
			defer srv.Close()
			raw := srv.URL + "/file.bin"
			m := restoreTestPartial(t, raw, "AAAA", 8, `"same"`)
			m.SetAutoStart(auto)
			authURL := strings.Replace(raw, "http://", "http://alice:secret@", 1)
			added, skipped, invalid := m.Add(authURL)
			if added != 1 || skipped != 0 || len(invalid) != 0 {
				t.Fatalf("Add = %d, %d, %v", added, skipped, invalid)
			}
			if !auto {
				items, _ := m.Snapshot()
				if items[0].State != StatePaused {
					t.Fatalf("auto-start is off: %+v", items)
				}
				m.Resume("restored")
			}
			it := waitState(t, m, "restored", StateCompleted)
			if got, err := os.ReadFile(it.Path); err != nil || string(got) != "AAAAAAAA" {
				t.Fatalf("authenticated download = %q, %v", got, err)
			}
			if records := m.Records(); len(records) != 1 || strings.Contains(records[0].URL, "secret") {
				t.Fatalf("credentials or duplicate task persisted: %+v", records)
			}
		})
	}
}

func TestRefreshMissingCompletedFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "done")
	}))
	defer srv.Close()
	m := newTestManager(t, t.TempDir(), 1)
	id := addOne(t, m, srv.URL+"/file.bin")
	it := waitState(t, m, id, StateCompleted)
	var changes atomic.Int32
	m.SetOnChange(func() { changes.Add(1) })
	for _, missing := range []bool{true, false, true} {
		if missing {
			if err := os.Remove(it.Path); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(it.Path, []byte("done"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := changes.Load()
		m.RefreshFiles()
		items, _ := m.Snapshot()
		if items[0].Missing != missing || changes.Load() <= before {
			t.Fatalf("file change was not published: %+v", items)
		}
	}
	m.Retry(id)
	it = waitState(t, m, id, StateCompleted)
	if it.Missing || !fileExists(it.Path) {
		t.Fatalf("missing file did not download again: %+v", it)
	}
}
