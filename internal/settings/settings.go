// Package settings holds the app's saved preferences.
package settings

import (
	"path/filepath"
	"strings"

	"go-downloader/internal/proxy"
)

// Theme is the app's appearance.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// ViewMode is how the download list shows.
type ViewMode string

const (
	ViewList  ViewMode = "list"
	ViewCards ViewMode = "cards"
)

// SortKey orders the download list.
type SortKey string

const (
	SortNewest SortKey = "newest"
	SortName   SortKey = "name"
	SortSize   SortKey = "size"
	SortStatus SortKey = "status"
)

// Settings are saved as settings.json.
type Settings struct {
	Version          int          `json:"version"`
	DownloadDir      string       `json:"downloadDir"`
	Theme            Theme        `json:"theme"`
	View             ViewMode     `json:"view"`
	Sort             SortKey      `json:"sort"`
	Proxy            proxy.Config `json:"proxy"`
	MaxConcurrent    int          `json:"maxConcurrent"`
	RateLimit        int64        `json:"rateLimit,omitempty"`
	NotifyOnComplete bool         `json:"notifyOnComplete"`
	AutoStart        bool         `json:"autoStart"`
}

// Default returns the settings of a first run.
func Default(downloadDir string) Settings {
	s := Settings{
		Version:          1,
		DownloadDir:      downloadDir,
		Theme:            ThemeSystem,
		View:             ViewList,
		Sort:             SortNewest,
		Proxy:            proxy.Config{Mode: proxy.ModeSystem},
		MaxConcurrent:    3,
		NotifyOnComplete: true,
		AutoStart:        true,
	}
	s.Normalize(downloadDir)
	return s
}

// Normalize repairs values a hand-edited or older settings file may hold.
func (s *Settings) Normalize(defaultDir string) {
	s.Version = 1
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
	switch s.View {
	case ViewList, ViewCards:
	default:
		s.View = ViewList
	}
	switch s.Sort {
	case SortNewest, SortName, SortSize, SortStatus:
	default:
		s.Sort = SortNewest
	}
	if s.MaxConcurrent < 1 {
		s.MaxConcurrent = 1
	}
	if s.MaxConcurrent > 8 {
		s.MaxConcurrent = 8
	}
	s.RateLimit = max(0, min(s.RateLimit, 1024*1024*1024))
	s.Proxy.Normalize()
	s.DownloadDir = strings.TrimSpace(s.DownloadDir)
	if s.DownloadDir == "" {
		s.DownloadDir = defaultDir
	}
	if s.DownloadDir != "" {
		if abs, err := filepath.Abs(s.DownloadDir); err == nil {
			s.DownloadDir = abs
		}
	}
}

// Dark reports whether the app should draw dark, given the system's
// appearance.
func (s Settings) Dark(systemDark bool) bool {
	switch s.Theme {
	case ThemeDark:
		return true
	case ThemeLight:
		return false
	default:
		return systemDark
	}
}

// ThemeLabels maps the appearance choices to their Chinese labels, in the
// order the UI shows them.
var ThemeLabels = []string{"浅色", "深色", "跟随系统"}

// ThemeValues is ThemeLabels' values.
var ThemeValues = []Theme{ThemeLight, ThemeDark, ThemeSystem}

// ThemeIndex returns the index of the theme in ThemeValues.
func ThemeIndex(t Theme) int {
	for i, v := range ThemeValues {
		if v == t {
			return i
		}
	}
	return len(ThemeValues) - 1
}
