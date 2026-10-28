package settings

import (
	"testing"

	"go-downloader/internal/proxy"
)

func TestDefault(t *testing.T) {
	dir := t.TempDir()
	s := Default(dir)
	if s.Version != 1 || s.Theme != ThemeSystem || s.View != ViewList || s.Sort != SortNewest {
		t.Errorf("defaults = %+v", s)
	}
	if s.MaxConcurrent != 3 || !s.NotifyOnComplete || !s.AutoStart {
		t.Errorf("defaults = %+v", s)
	}
	if s.Proxy.Mode != proxy.ModeSystem || s.DownloadDir != dir {
		t.Errorf("defaults = %+v", s)
	}
}

func TestNormalize(t *testing.T) {
	fallback := t.TempDir()
	s := Settings{
		Theme:         "purple",
		View:          "mosaic",
		Sort:          "random",
		MaxConcurrent: 99,
		RateLimit:     -10,
		Proxy:         proxy.Config{},
		DownloadDir:   "  ",
	}
	s.Normalize(fallback)
	if s.Theme != ThemeSystem || s.View != ViewList || s.Sort != SortNewest {
		t.Errorf("enums = %+v", s)
	}
	if s.MaxConcurrent != 8 {
		t.Errorf("MaxConcurrent = %d", s.MaxConcurrent)
	}
	if s.RateLimit != 0 {
		t.Errorf("negative rate limit = %d", s.RateLimit)
	}
	if s.DownloadDir != fallback {
		t.Errorf("DownloadDir = %q", s.DownloadDir)
	}
	if s.Proxy.Mode != proxy.ModeSystem {
		t.Errorf("proxy mode = %q", s.Proxy.Mode)
	}
	s.MaxConcurrent = 0
	s.Normalize(fallback)
	if s.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent = %d", s.MaxConcurrent)
	}
}
func TestDark(t *testing.T) {
	cases := []struct {
		theme Theme
		sys   bool
		want  bool
	}{
		{ThemeDark, false, true},
		{ThemeDark, true, true},
		{ThemeLight, true, false},
		{ThemeLight, false, false},
		{ThemeSystem, true, true},
		{ThemeSystem, false, false},
	}
	for _, tc := range cases {
		s := Settings{Theme: tc.theme}
		if got := s.Dark(tc.sys); got != tc.want {
			t.Errorf("Dark(%s, %v) = %v, want %v", tc.theme, tc.sys, got, tc.want)
		}
	}
}

func TestThemeIndex(t *testing.T) {
	for i, theme := range ThemeValues {
		if got := ThemeIndex(theme); got != i {
			t.Errorf("ThemeIndex(%s) = %d, want %d", theme, got, i)
		}
	}
	if got := ThemeIndex("other"); got != len(ThemeValues)-1 {
		t.Errorf("ThemeIndex(other) = %d", got)
	}
}
