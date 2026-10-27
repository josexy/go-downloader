// Package store reads and writes the app's settings and download records as
// JSON files, atomically and without losing the old file to a crash.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go-downloader/internal/download"
	"go-downloader/internal/settings"
)

// SecretStore keeps small secrets in the system's credential store.
type SecretStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// Store persists the app's files in Dir.
type Store struct {
	Dir     string
	Secrets SecretStore

	mu sync.Mutex
}

const (
	settingsFile = "settings.json"
	tasksFile    = "tasks.json"
	// ProxyPasswordKey is where the proxy password is kept.
	ProxyPasswordKey = "proxy-password"
)

// SettingsPath returns where settings.json lives.
func (s *Store) SettingsPath() string { return filepath.Join(s.Dir, settingsFile) }

// TasksPath returns where tasks.json lives.
func (s *Store) TasksPath() string { return filepath.Join(s.Dir, tasksFile) }

// LoadSettings reads the settings. A missing file gives defaults; a corrupt
// one is renamed to settings.json.bad and gives defaults, with the error.
func (s *Store) LoadSettings(defaultDir string) (settings.Settings, error) {
	path := s.SettingsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings.Default(defaultDir), nil
	}
	if err != nil {
		fallback := settings.Default(defaultDir)
		return fallback, fmt.Errorf("读取设置失败：%w", err)
	}
	var v settings.Settings
	if err := json.Unmarshal(data, &v); err != nil {
		quarantine(path)
		return settings.Default(defaultDir), fmt.Errorf("设置文件已损坏，已使用默认设置：%w", err)
	}
	v.Normalize(defaultDir)
	return v, nil
}

// SaveSettings writes the settings atomically.
func (s *Store) SaveSettings(v settings.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeJSON(s.SettingsPath(), v); err != nil {
		return fmt.Errorf("保存设置失败：%w", err)
	}
	return nil
}

// tasksFileJSON is the shape of tasks.json.
type tasksFileJSON struct {
	Version int               `json:"version"`
	Tasks   []download.Record `json:"tasks"`
}

// LoadTasks reads the saved downloads. A corrupt file is quarantined.
func (s *Store) LoadTasks() ([]download.Record, error) {
	path := s.TasksPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取下载记录失败：%w", err)
	}
	var f tasksFileJSON
	if err := json.Unmarshal(data, &f); err != nil {
		quarantine(path)
		return nil, fmt.Errorf("下载记录已损坏，已忽略：%w", err)
	}
	return f.Tasks, nil
}

// SaveTasks writes the downloads atomically.
func (s *Store) SaveTasks(records []download.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeJSON(s.TasksPath(), tasksFileJSON{Version: 1, Tasks: records}); err != nil {
		return fmt.Errorf("保存下载记录失败：%w", err)
	}
	return nil
}

// LoadProxyPassword reads the proxy password from the credential store. It
// returns "" when there is none or no store.
func (s *Store) LoadProxyPassword() string {
	if s.Secrets == nil {
		return ""
	}
	v, err := s.Secrets.Get(ProxyPasswordKey)
	if err != nil {
		return ""
	}
	return v
}

// SaveProxyPassword keeps the proxy password in the credential store, or
// removes it when empty.
func (s *Store) SaveProxyPassword(password string) error {
	if s.Secrets == nil {
		return nil
	}
	if password == "" {
		if err := s.Secrets.Delete(ProxyPasswordKey); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("清除代理密码失败：%w", err)
		}
		return nil
	}
	if err := s.Secrets.Set(ProxyPasswordKey, password); err != nil {
		return fmt.Errorf("保存代理密码失败：%w", err)
	}
	return nil
}

// writeJSON writes v to path through a temporary file in the same directory.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// quarantine renames a corrupt file to <name>.bad, replacing an older copy.
func quarantine(path string) {
	bad := path + ".bad"
	_ = os.Remove(bad)
	_ = os.Rename(path, bad)
}
