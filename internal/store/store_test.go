package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-downloader/internal/download"
	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemSecrets() *memSecrets { return &memSecrets{m: map[string]string{}} }

func (s *memSecrets) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (s *memSecrets) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memSecrets) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func TestLoadSettingsDefaults(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	defaultDir := t.TempDir()
	got, err := s.LoadSettings(defaultDir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.DownloadDir != defaultDir || got.MaxConcurrent != 3 {
		t.Errorf("settings = %+v", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	want := settings.Default(dir)
	want.Theme = settings.ThemeDark
	want.View = settings.ViewCards
	want.MaxConcurrent = 5
	want.RateLimit = 2048 * 1024
	want.Proxy = proxy.Config{Mode: proxy.ModeManual, Kind: proxy.KindSOCKS5, Host: "127.0.0.1", Port: 1080, Password: "do-not-save"}
	if err := s.SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "do-not-save") {
		t.Error("the proxy password was written to settings.json")
	}
	if strings.Contains(string(raw), `"password"`) {
		t.Error("settings.json has a password field")
	}
	got, err := s.LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != settings.ThemeDark || got.View != settings.ViewCards || got.MaxConcurrent != 5 {
		t.Errorf("settings = %+v", got)
	}
	if got.RateLimit != want.RateLimit {
		t.Errorf("rate limit = %d, want %d", got.RateLimit, want.RateLimit)
	}
	if got.Proxy.Mode != proxy.ModeManual || got.Proxy.Port != 1080 {
		t.Errorf("proxy = %+v", got.Proxy)
	}
}

func TestCorruptSettingsQuarantined(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	defaultDir := t.TempDir()
	if err := os.WriteFile(s.SettingsPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadSettings(defaultDir)
	if err == nil {
		t.Error("a corrupt settings file returned no error")
	}
	if got.DownloadDir != defaultDir {
		t.Errorf("settings = %+v", got)
	}
	if _, err := os.Stat(s.SettingsPath() + ".bad"); err != nil {
		t.Errorf("the corrupt file was not kept: %v", err)
	}
}

func TestTasksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	now := time.Now().Truncate(time.Second)
	records := []download.Record{
		{ID: "a", URL: "https://example.com/a.zip", FileName: "a.zip", Path: filepath.Join(dir, "a.zip"), State: download.StateCompleted, Total: 10, Done: 10, CreatedAt: now, FinishedAt: now},
		{ID: "b", URL: "https://example.com/b.zip", FileName: "b.zip", Path: filepath.Join(dir, "b.zip"), State: download.StatePaused, Total: 100, Done: 40, ETag: `"x"`, Resumable: true, CreatedAt: now},
	}
	if err := s.SaveTasks(records); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ID != "b" || got[1].Done != 40 || !got[1].Resumable {
		t.Errorf("records = %+v", got)
	}
}

func TestCorruptTasksQuarantined(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	if err := os.WriteFile(s.TasksPath(), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadTasks()
	if err == nil || got != nil {
		t.Errorf("LoadTasks = %v, %v", got, err)
	}
	if _, err := os.Stat(s.TasksPath() + ".bad"); err != nil {
		t.Errorf("the corrupt file was not kept: %v", err)
	}
}

func TestProxyPassword(t *testing.T) {
	dir := t.TempDir()
	secrets := newMemSecrets()
	s := &Store{Dir: dir, Secrets: secrets}
	if got := s.LoadProxyPassword(); got != "" {
		t.Errorf("password = %q", got)
	}
	if err := s.SaveProxyPassword("hunter2"); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadProxyPassword(); got != "hunter2" {
		t.Errorf("password = %q", got)
	}
	if err := s.SaveProxyPassword(""); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadProxyPassword(); got != "" {
		t.Errorf("password after clearing = %q", got)
	}

	noStore := &Store{Dir: dir}
	if err := noStore.SaveProxyPassword("x"); err != nil {
		t.Errorf("saving without a store: %v", err)
	}
	if got := noStore.LoadProxyPassword(); got != "" {
		t.Errorf("password without a store = %q", got)
	}
}

func TestSavesAreValidJSON(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	if err := s.SaveSettings(settings.Default(dir)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var any map[string]any
	if err := json.Unmarshal(data, &any); err != nil {
		t.Errorf("settings.json is not JSON: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}
