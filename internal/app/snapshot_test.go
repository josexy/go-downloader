package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
)

// TestSnapshots renders the interface to PNG files for visual review:
//
//	DOWNLOADER_SNAPSHOT=1 go test -run TestSnapshots ./internal/app
func TestSnapshots(t *testing.T) {
	if os.Getenv("DOWNLOADER_SNAPSHOT") == "" {
		t.Skip("set DOWNLOADER_SNAPSHOT=1 to write the snapshots")
	}
	dir := filepath.Join("testdata", "snapshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, tt *ui.Tester) {
		t.Helper()
		img := tt.Image()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	engine := &fakeEngine{}
	engine.items = sampleItems()
	a := newTestApp(t, engine)
	a.s.DownloadDir = filepath.FromSlash("C:/Users/Ada/Downloads")
	a.deps.dataDir = filepath.FromSlash("C:/Users/Ada/AppData/Roaming/Downloader")
	a.system = &proxy.System{
		Source: "Windows 系统代理",
		HTTP:   &proxy.Endpoint{Kind: proxy.KindHTTP, Host: "127.0.0.1", Port: 7890},
		HTTPS:  &proxy.Endpoint{Kind: proxy.KindHTTP, Host: "127.0.0.1", Port: 7890},
	}
	a.rebuildClient()
	tt := testerFor(a)
	tt.Frame()
	save("light-list.png", tt)

	// The enabled primary action and a chosen row, for reviewing the accent.
	a.input = "https://example.com/file.zip"
	a.selection.Add("d1")
	tt.Frame()
	save("light-list-actions.png", tt)
	a.input = ""
	a.selection.Clear()
	tt.Frame()

	tt.SetDark(true)
	tt.Frame()
	save("dark-list.png", tt)

	// Review compact caption controls at rest and under the pointer in both
	// themes, including their actual size at 125% display scaling.
	caption := ui.NewTester(func(c *ui.Context) {
		a.palette = paletteFor(c.Theme().Dark)
		c.SetTheme(a.palette.theme())
		a.header(c, a.summary)
	}, 1240, titleBarHeight)
	caption.SetLocale("zh-Hans")
	caption.SetScale(1.25)
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		caption.SetDark(dark)
		for _, button := range []struct{ label, name string }{
			{"最大化窗口", "maximize"}, {"关闭窗口", "close"},
		} {
			r, ok := caption.Find(button.label)
			if !ok {
				t.Fatalf("missing caption control %s", button.label)
			}
			caption.Move(r.X+r.W/2, r.Y+r.H/2)
			save(name+"-titlebar-"+button.name+"-hover.png", caption)
		}
	}

	a.input = "https://example.com/file.zip"
	a.selection.Add("d1")
	tt.Frame()
	save("dark-list-actions.png", tt)
	a.input = ""
	a.selection.Clear()
	tt.Frame()

	tt.SetDark(false)
	a.setView(settings.ViewCards)
	tt.Frame()
	save("light-cards.png", tt)

	tt.SetDark(true)
	tt.Frame()
	save("dark-cards.png", tt)

	tt.SetDark(false)
	a.setView(settings.ViewList)
	a.settingsTab = 0
	a.settingsOpen = true
	tt.Frame()
	save("light-settings-general.png", tt)

	a.settingsTab = 1
	a.proxyTest = proxyTest{ok: true, result: "连接成功 · 32 ms"}
	tt.Frame()
	save("light-settings-proxy.png", tt)

	// A custom proxy, as the manual form lays it out.
	a.proxyDraft.Mode = proxy.ModeManual
	a.proxyDraft.Kind = proxy.KindSOCKS5
	a.proxyDraft.Host = "127.0.0.1"
	a.proxyPort = 1080
	a.proxyKindLabel = proxyKindLabel(proxy.KindSOCKS5)
	a.proxyModeIdx = 2
	tt.Frame()
	save("light-settings-proxy-manual.png", tt)

	a.settingsTab = 2
	tt.Frame()
	save("light-settings-appearance.png", tt)

	a.settingsTab = 3
	tt.Frame()
	save("light-settings-about.png", tt)

	a.settingsTab = 1
	tt.SetDark(true)
	tt.Frame()
	save("dark-settings-proxy.png", tt)
	for _, tab := range []struct {
		index int
		name  string
	}{
		{0, "general"}, {2, "appearance"}, {3, "about"},
	} {
		a.settingsTab = tab.index
		tt.Frame()
		save("dark-settings-"+tab.name+".png", tt)
	}

	tt.SetDark(false)
	empty := newTestApp(t, &fakeEngine{})
	empty.s.DownloadDir = a.s.DownloadDir
	tts := testerFor(empty)
	tts.Frame()
	save("light-empty.png", tts)

	tts.SetDark(true)
	tts.Frame()
	save("dark-empty.png", tts)

	// Selection must remain readable after the sidebar takes keyboard focus.
	focused := newTestApp(t, &fakeEngine{items: sampleItems()})
	focused.s.DownloadDir = a.s.DownloadDir
	ft := testerFor(focused)
	if err := ft.Click("下载完成"); err != nil {
		t.Fatal(err)
	}
	ft.Frame()
	save("light-sidebar-selected.png", ft)
	ft.SetDark(true)
	ft.Frame()
	save("dark-sidebar-selected.png", ft)

	// The supported minimum window size must keep actions reachable and
	// make long settings pages scroll rather than overflow the dialog.
	a.settingsOpen = false
	a.selection.Clear()
	tt.SetSize(960, 640)
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		tt.SetDark(dark)
		for _, view := range viewValues {
			a.setView(view)
			tt.Frame()
			save(name+"-compact-"+string(view)+".png", tt)
		}
		a.settingsOpen = true
		a.settingsTab = 1
		tt.Frame()
		save(name+"-compact-settings.png", tt)
		a.settingsOpen = false
		a.requestRemove([]string{"d1", "d3"})
		tt.Frame()
		save(name+"-confirm.png", tt)
		a.confirm.open = false
	}

	// Review the shared app icon in the header and About page at 125% on
	// the minimum supported window, where its bitmap must stay crisp.
	tt.SetScale(1.25)
	a.setView(settings.ViewList)
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		tt.SetDark(dark)
		tt.Frame()
		save(name+"-compact-125-list.png", tt)
		a.settingsOpen = true
		a.settingsTab = 3
		tt.Frame()
		save(name+"-compact-125-about.png", tt)
		a.settingsOpen = false
	}
}

// sampleItems is a mix of states for the snapshots.
func sampleItems() []download.Item {
	mk := func(id, name string, state download.State, total, done int64) download.Item {
		it := download.Item{
			ID:         id,
			URL:        "https://mirrors.example.com/releases/" + name,
			DisplayURL: "https://mirrors.example.com/releases/" + name,
			FileName:   name,
			Path:       filepath.Join("C:", "Users", "Ada", "Downloads", name),
			State:      state,
			Total:      total,
			Done:       done,
			CreatedAt:  testNow.Add(-time.Duration(len(id)+1) * time.Minute),
		}
		switch state {
		case download.StateDownloading:
			it.Speed = 6.4 * 1024 * 1024
			it.ETA = 4*time.Minute + 12*time.Second
			it.Resumable = true
		case download.StateCompleted:
			it.FinishedAt = testNow.Add(-9 * time.Minute)
			it.Done = total
		case download.StateFailed:
			it.Error = "网络错误：连接被重置，点击重试"
		}
		return it
	}
	return []download.Item{
		mk("d1", "ubuntu-24.04.1-desktop-amd64.iso", download.StateDownloading, 5200*1024*1024, 1400*1024*1024),
		mk("d2", "设计稿-final-v3.zip", download.StateDownloading, 268*1024*1024, 189*1024*1024),
		mk("d3", "driver-setup.exe", download.StateFailed, 96*1024*1024, 32*1024*1024),
		mk("d4", "报告-2026-Q4.pdf", download.StateCompleted, 12*1024*1024, 12*1024*1024),
		mk("d5", "音乐合集.flac", download.StatePaused, 640*1024*1024, 210*1024*1024),
		mk("d6", "notes-archive.tar.gz", download.StateQueued, 48*1024*1024, 0),
	}
}
