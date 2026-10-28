package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
	"go-downloader/internal/store"
)

var testNow = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

// fakeEngine is an in-memory downloader for the view tests.
type fakeEngine struct {
	mu              sync.Mutex
	items           []download.Item
	calls           []string
	nextID          int
	dir             string
	max             int
	auto            bool
	onChange        func()
	onComplete      func(download.Item)
	advancedOptions []download.AddOptions
}

func (f *fakeEngine) record(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeEngine) hasCall(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			return true
		}
	}
	return false
}

func (f *fakeEngine) Add(raw string) (int, int, []string) {
	valid, invalid := download.NormalizeURLs(raw)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Add:" + raw)
	added, skipped := 0, 0
	for _, link := range valid {
		duplicate := false
		for _, it := range f.items {
			if it.URL == link && !it.State.Terminal() {
				duplicate = true
				break
			}
		}
		if duplicate {
			skipped++
			continue
		}
		name := "download"
		if parsed, err := url.Parse(link); err == nil {
			name = download.NameFromURL(parsed)
		}
		state := download.StateQueued
		if !f.auto {
			state = download.StatePaused
		}
		f.nextID++
		f.items = append(f.items, download.Item{
			ID:         fmt.Sprintf("id%d", f.nextID),
			URL:        link,
			DisplayURL: link,
			FileName:   name,
			Path:       filepath.Join(f.dir, name),
			State:      state,
			Total:      100,
			CreatedAt:  testNow.Add(time.Duration(f.nextID) * time.Second),
		})
		added++
	}
	return added, skipped, invalid
}

func (f *fakeEngine) setState(id string, state download.State) {
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].State = state
			f.items[i].Speed = 0
			return
		}
	}
}

func (f *fakeEngine) AddTask(opts download.AddOptions) (string, error) {
	added, _, invalid := f.Add(opts.URL)
	if added == 0 || len(invalid) > 0 {
		return "", fmt.Errorf("链接无效或已存在")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advancedOptions = append(f.advancedOptions, opts)
	it := &f.items[len(f.items)-1]
	dir := opts.Dir
	if dir == "" {
		dir = f.dir
	}
	if opts.FileName != "" {
		it.FileName = opts.FileName
	}
	it.Path = filepath.Join(dir, it.FileName)
	return it.ID, nil
}

func (f *fakeEngine) SetRateLimit(rate int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SetRateLimit:%d", rate))
}

func (f *fakeEngine) Pause(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Pause:" + id)
	f.setState(id, download.StatePaused)
}

func (f *fakeEngine) Resume(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Resume:" + id)
	f.setState(id, download.StateDownloading)
}

func (f *fakeEngine) Retry(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Retry:" + id)
	f.setState(id, download.StateQueued)
}

func (f *fakeEngine) Remove(id string, deleteFile bool) {
	f.RemoveMany([]string{id}, deleteFile)
}

func (f *fakeEngine) RemoveMany(ids []string, deleteFiles bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("RemoveMany:%v:%v", ids, deleteFiles))
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	out := f.items[:0]
	for _, it := range f.items {
		if !wanted[it.ID] {
			out = append(out, it)
		}
	}
	f.items = out
}

func (f *fakeEngine) PauseAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("PauseAll")
	for i := range f.items {
		if f.items[i].State.Active() {
			f.items[i].State = download.StatePaused
		}
	}
}

func (f *fakeEngine) ResumeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ResumeAll")
	for i := range f.items {
		if f.items[i].State == download.StatePaused || f.items[i].State == download.StateFailed {
			f.items[i].State = download.StateDownloading
		}
	}
}

func (f *fakeEngine) ClearCompleted() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ClearCompleted")
	out := f.items[:0]
	for _, it := range f.items {
		if it.State != download.StateCompleted {
			out = append(out, it)
		}
	}
	f.items = out
}

func (f *fakeEngine) SetDir(dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dir = dir
	f.record("SetDir:" + dir)
}

func (f *fakeEngine) SetMaxConcurrent(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.max = n
	f.record(fmt.Sprintf("SetMaxConcurrent:%d", n))
}

func (f *fakeEngine) SetAutoStart(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auto = on
	f.record(fmt.Sprintf("SetAutoStart:%v", on))
}

func (f *fakeEngine) RefreshFiles() {}

func (f *fakeEngine) Snapshot() ([]download.Item, download.Summary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := append([]download.Item(nil), f.items...)
	var sum download.Summary
	for _, it := range items {
		sum.Total++
		switch it.State {
		case download.StateQueued:
			sum.Queued++
			sum.Active++
		case download.StateConnecting, download.StateDownloading:
			sum.Active++
		case download.StatePaused:
			sum.Paused++
		case download.StateCompleted:
			sum.Completed++
		case download.StateFailed:
			sum.Failed++
		}
		sum.DoneBytes += it.Done
		if it.Total > 0 {
			sum.TotalBytes += it.Total
		}
		if it.State == download.StateDownloading {
			sum.Speed += it.Speed
		}
	}
	return items, sum
}

func (f *fakeEngine) Records() []download.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]download.Record, 0, len(f.items))
	for _, it := range f.items {
		out = append(out, download.Record{
			ID: it.ID, URL: it.URL, FileName: it.FileName, Path: it.Path,
			State: it.State, Total: it.Total, Done: it.Done, CreatedAt: it.CreatedAt,
		})
	}
	return out
}

func (f *fakeEngine) Restore(records []download.Record) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range records {
		f.items = append(f.items, download.Item{
			ID: r.ID, URL: r.URL, DisplayURL: r.URL, FileName: r.FileName,
			Path: r.Path, State: r.State, Total: r.Total, Done: r.Done, CreatedAt: r.CreatedAt,
		})
	}
}

func (f *fakeEngine) SetOnChange(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onChange = fn
}

func (f *fakeEngine) SetOnComplete(fn func(download.Item)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onComplete = fn
}

func (f *fakeEngine) Shutdown() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Shutdown")
}

// memSecrets is an in-memory credential store for the tests.
type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return "", fmt.Errorf("no secret %q", key)
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

// newTestApp builds the app with an empty profile and the fake engine.
func newTestApp(t *testing.T, engine *fakeEngine) *app {
	t.Helper()
	dataDir := t.TempDir()
	downloadDir := t.TempDir()
	engine.dir = downloadDir
	d := deps{
		dataDir:    dataDir,
		defaultDir: downloadDir,
		version:    "test",
		secrets:    &memSecrets{m: map[string]string{}},
		now:        func() time.Time { return testNow },
		async:      func(fn func()) { fn() },
		notify:     func(string, string) {},
		canNotify:  func() bool { return false },
		appActive:  func() bool { return true },
		pickFolder: func(*mygo.Window, string) (string, error) { return "", nil },
		openPath:   func(string) error { return nil },
		reveal:     func(string) {},
		detectSystem: func(context.Context) (*proxy.System, error) {
			return &proxy.System{}, nil
		},
		testProxy: func(context.Context, proxy.Config, *proxy.System, string) (time.Duration, error) {
			return 32 * time.Millisecond, nil
		},
		applyTheme: func(settings.Theme) {},
	}
	a := newApp(d)
	a.load()
	a.engine = engine
	engine.dir = a.s.DownloadDir
	engine.auto = true // settings.Default starts downloads
	a.s = settings.Default(downloadDir)
	a.syncFields()
	a.filter = filterAll
	a.settingsOpen = false
	a.confirm = confirmState{}
	return a
}

func seedItem(id, name string, state download.State) download.Item {
	it := download.Item{
		ID:         id,
		URL:        "https://example.com/files/" + name,
		DisplayURL: "https://example.com/files/" + name,
		FileName:   name,
		Path:       filepath.Join("C:", "downloads", name),
		State:      state,
		Total:      48 * 1024 * 1024,
		Done:       12 * 1024 * 1024,
		CreatedAt:  testNow.Add(-time.Hour),
	}
	switch state {
	case download.StateDownloading:
		it.Speed = 1.4 * 1024 * 1024
		it.ETA = 25 * time.Second
	case download.StateCompleted:
		it.Done = it.Total
		it.FinishedAt = testNow.Add(-5 * time.Minute)
	case download.StateFailed:
		it.Error = "网络错误：连接被重置"
	}
	return it
}

func testerFor(a *app) *ui.Tester {
	tt := ui.NewTester(a.view, 1240, 820)
	tt.SetLocale("zh-Hans")
	return tt
}

func TestEmptyState(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	if !tt.HasText("还没有下载") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if !tt.HasText("从剪贴板粘贴") {
		t.Error("the empty state has no paste button")
	}
}

func TestMissingFileRefreshesAndDownloadsAgain(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	path := filepath.Join(a.s.DownloadDir, "refreshed.bin")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "recovered")
	}))
	defer srv.Close()
	engine := download.New(download.Options{Dir: a.s.DownloadDir})
	engine.Restore([]download.Record{{ID: "refresh", URL: srv.URL + "/refreshed.bin", Path: path, State: download.StateCompleted}})
	engine.SetOnChange(a.onEngineChange)
	a.engine = engine
	a.startSaver()
	t.Cleanup(a.shutdown)
	tt := testerFor(a)
	if tt.HasText("文件缺失") {
		t.Fatal("an existing file is marked missing")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitFor := func(wantMissing bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			items, _ := engine.Snapshot()
			if items[0].State == download.StateCompleted && items[0].Missing == wantMissing {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("file refresh did not reach missing=%v: %+v", wantMissing, items)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(true)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("文件缺失") {
		t.Fatalf("the view did not update after background refresh: %q", tt.Texts())
	}
	if err := tt.Click("重新下载 refreshed.bin"); err != nil {
		t.Fatal(err)
	}
	waitFor(false)
	tt.Frame()
	tt.Frame()
	if tt.HasText("文件缺失") {
		t.Fatal("the recovered file is still marked missing")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "recovered" {
		t.Fatalf("re-downloaded file = %q, %v", got, err)
	}
}

func TestAddDownload(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	tt := testerFor(a)
	if err := tt.Click("下载链接输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://example.com/a.zip")
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("Add:https://example.com/a.zip") {
		t.Errorf("calls = %v", engine.calls)
	}
	if !tt.HasText("a.zip") || !tt.HasText("等待中") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if a.input != "" {
		t.Errorf("input = %q", a.input)
	}
}

// TestAddWithAutoStartOff checks that a new download waits, paused, in the
// Paused filter instead of joining the active downloads.
func TestAddWithAutoStartOff(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	a.setAutoStart(false)
	tt := testerFor(a)
	if err := tt.Click("下载链接输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://example.com/queued.zip")
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	items, summary := engine.Snapshot()
	if len(items) != 1 || items[0].State != download.StatePaused {
		t.Fatalf("items after add = %+v", items)
	}
	if summary.Paused != 1 || summary.Active != 0 {
		t.Errorf("summary = %+v", summary)
	}
	if !tt.HasText("1 个已暂停") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if !tt.HasText("全部继续") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("暂停中"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("queued.zip") {
		t.Errorf("the paused filter does not show the download: %q", tt.Texts())
	}
	if err := tt.Click("正在下载"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("queued.zip") || !tt.HasText("没有匹配的下载") {
		t.Errorf("the active filter shows a paused download: %q", tt.Texts())
	}

	a.filter = filterAll
	a.selection.Clear()
	tt.Frame()
	if err := tt.Click("继续 queued.zip"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("Resume:id1") {
		t.Errorf("calls = %v", engine.calls)
	}
}

func TestAddInvalidLink(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	tt := testerFor(a)
	if err := tt.Click("下载链接输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("ftp://example.com/file")
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("无法识别：ftp://example.com/file") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if a.input != "ftp://example.com/file" {
		t.Errorf("input = %q, want the invalid link kept", a.input)
	}
}

func TestPauseAndResume(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	item := seedItem("id1", "a.zip", download.StateDownloading)
	engine.items = []download.Item{item}
	tt := testerFor(a)
	if err := tt.Click("暂停 a.zip"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("Pause:id1") {
		t.Errorf("calls = %v", engine.calls)
	}
	if !tt.HasText("继续 a.zip") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("继续 a.zip"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("Resume:id1") {
		t.Errorf("calls = %v", engine.calls)
	}
}

func TestThemeSwitch(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	var applied []settings.Theme
	a.deps.applyTheme = func(theme settings.Theme) { applied = append(applied, theme) }
	tt := testerFor(a)
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	if a.s.Theme != settings.ThemeDark {
		t.Errorf("theme = %q", a.s.Theme)
	}
	if len(applied) == 0 || applied[len(applied)-1] != settings.ThemeDark {
		t.Errorf("applied = %v", applied)
	}
	if !a.dark {
		t.Error("the frame is not dark")
	}
}

func TestViewSwitch(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{seedItem("id1", "a.zip", download.StateDownloading)}
	tt := testerFor(a)
	if err := tt.Click("卡片"); err != nil {
		t.Fatal(err)
	}
	if a.s.View != settings.ViewCards {
		t.Errorf("view = %q", a.s.View)
	}
	if !tt.HasText("a.zip") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestFilterSidebar(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{
		seedItem("id1", "running.zip", download.StateDownloading),
		seedItem("id2", "done.zip", download.StateCompleted),
		seedItem("id3", "failed.zip", download.StateFailed),
	}
	tt := testerFor(a)
	if err := tt.Click("下载完成"); err != nil {
		t.Fatal(err)
	}
	if a.filter != filterCompleted {
		t.Errorf("filter = %q", a.filter)
	}
	if !tt.HasText("done.zip") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if tt.HasText("running.zip") || tt.HasText("failed.zip") {
		t.Errorf("the completed filter shows other downloads: %q", tt.Texts())
	}
}

func TestSearch(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{
		seedItem("id1", "alpha.zip", download.StateCompleted),
		seedItem("id2", "beta.zip", download.StateCompleted),
	}
	tt := testerFor(a)
	if err := tt.Click("搜索下载"); err != nil {
		t.Fatal(err)
	}
	tt.Type("alpha")
	if !tt.HasText("alpha.zip") || tt.HasText("beta.zip") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestSortBySize(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	small := seedItem("id1", "small.zip", download.StateCompleted)
	small.Total, small.Done = 1024, 1024
	large := seedItem("id2", "large.zip", download.StateCompleted)
	large.Total, large.Done = 1024*1024*100, 1024*1024*100
	engine.items = []download.Item{small, large}
	tt := testerFor(a)
	a.s.Sort = settings.SortSize
	a.sortLabel = sortLabelFor(a.s.Sort)
	tt.Frame()
	if !a.sortedFirst("large.zip") {
		t.Errorf("visible order = %+v", a.visible)
	}
}

func (a *app) sortedFirst(name string) bool {
	return len(a.visible) > 0 && a.visible[0].FileName == name
}

// TestSearchShortcutFocuses checks that Ctrl/Cmd+F puts the keyboard focus in
// the search field, as MyGo 0.7's Focus fix promises.
func TestSearchShortcutFocuses(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Type("alpha")
	if a.query != "alpha" {
		t.Fatalf("query = %q after the search shortcut", a.query)
	}
}

func TestOpenSettingsShortcut(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	tt.Key(ui.Cmd, ui.KeyComma)
	if !a.settingsOpen {
		t.Fatal("the settings dialog did not open")
	}
	if !tt.HasText("常规") || !tt.HasText("代理") || !tt.HasText("外观") {
		t.Errorf("texts = %q", tt.Texts())
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen {
		t.Error("the settings dialog did not close")
	}
}

func TestSettingsProxyTab(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("代理"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("自定义"); err != nil {
		t.Fatal(err)
	}
	if a.s.Proxy.Mode != proxy.ModeManual {
		t.Errorf("proxy mode = %q", a.s.Proxy.Mode)
	}
	if a.s.Proxy.Host != "127.0.0.1" || a.s.Proxy.Port != 7890 {
		t.Errorf("proxy = %+v", a.s.Proxy)
	}
	a.runProxyTest()
	tt.Frame()
	if !tt.HasText("连接成功 · 32 ms") {
		t.Errorf("texts = %q", tt.Texts())
	}
}

func TestSettingsFolderAndConcurrency(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	picked := t.TempDir()
	a.deps.pickFolder = func(*mygo.Window, string) (string, error) { return picked, nil }
	a.chooseFolder()
	if a.s.DownloadDir != picked {
		t.Errorf("download dir = %q, want %q", a.s.DownloadDir, picked)
	}
	if !engine.hasCall("SetDir:" + picked) {
		t.Errorf("calls = %v", engine.calls)
	}
	a.setMaxConcurrent(6)
	if a.s.MaxConcurrent != 6 || !engine.hasCall("SetMaxConcurrent:6") {
		t.Errorf("max = %d, calls = %v", a.s.MaxConcurrent, engine.calls)
	}
	a.setAutoStart(false)
	if a.s.AutoStart || !engine.hasCall("SetAutoStart:false") {
		t.Errorf("auto = %v, calls = %v", a.s.AutoStart, engine.calls)
	}
}

func TestConfirmRemove(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{seedItem("id1", "a.zip", download.StateCompleted)}
	tt := testerFor(a)
	if err := tt.Click("移除 a.zip"); err != nil {
		t.Fatal(err)
	}
	if !a.confirm.open {
		t.Fatal("the confirmation did not open")
	}
	if err := tt.Click("确认移除"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("RemoveMany:[id1]:false") {
		t.Errorf("calls = %v", engine.calls)
	}
	if a.confirm.open {
		t.Error("the confirmation stayed open")
	}
}

func TestDeleteFileCheckbox(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{seedItem("id1", "a.zip", download.StateCompleted)}
	tt := testerFor(a)
	if err := tt.Click("移除 a.zip"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("同时删除已下载的文件"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("确认移除"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("RemoveMany:[id1]:true") {
		t.Errorf("calls = %v", engine.calls)
	}
}

func TestContextMenuCopyLink(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	item := seedItem("id1", "a.zip", download.StateCompleted)
	engine.items = []download.Item{item}
	tt := testerFor(a)
	if err := tt.RightClick("a.zip"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if len(menu) == 0 {
		t.Fatalf("no context menu: %v", menu)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != item.URL {
		t.Errorf("clipboard = %q, want %q", got, item.URL)
	}
}

func TestOpenCompleted(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{seedItem("id1", "a.zip", download.StateCompleted)}
	var opened string
	a.deps.openPath = func(path string) error {
		opened = path
		return nil
	}
	tt := testerFor(a)
	if err := tt.Click("打开 a.zip"); err != nil {
		t.Fatal(err)
	}
	if opened == "" {
		t.Error("the completed file was not opened")
	}
}

func TestToolbarActions(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{
		seedItem("id1", "a.zip", download.StateDownloading),
		seedItem("id2", "b.zip", download.StatePaused),
	}
	tt := testerFor(a)
	if err := tt.Click("全部暂停"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("PauseAll") {
		t.Errorf("calls = %v", engine.calls)
	}
	if err := tt.Click("全部继续"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("ResumeAll") {
		t.Errorf("calls = %v", engine.calls)
	}
}

func TestNotificationOnComplete(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	a.canNotify = true
	a.s.NotifyOnComplete = true
	var notified []string
	a.deps.notify = func(title, body string) { notified = append(notified, title+"|"+body) }
	a.deps.appActive = func() bool { return false }
	a.onEngineComplete(seedItem("id1", "a.zip", download.StateCompleted))
	if len(notified) != 1 || notified[0] != "下载完成|a.zip" {
		t.Errorf("notified = %v", notified)
	}
	if !a.hasToast("已下载完成：a.zip") {
		t.Errorf("toasts = %v", a.toasts)
	}
}

func (a *app) hasToast(message string) bool {
	for _, toast := range a.toasts {
		if toast == message {
			return true
		}
	}
	return false
}

func TestPasteAppendsClipboard(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	tt.SetClipboard("https://example.com/b.zip")
	if err := tt.Click("粘贴"); err != nil {
		t.Fatal(err)
	}
	if a.input != "https://example.com/b.zip" {
		t.Errorf("input = %q", a.input)
	}
}

func TestClearCompleted(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{
		seedItem("id1", "done.zip", download.StateCompleted),
		seedItem("id2", "run.zip", download.StateDownloading),
	}
	tt := testerFor(a)
	if err := tt.Click("清除完成"); err != nil {
		t.Fatal(err)
	}
	if !engine.hasCall("ClearCompleted") {
		t.Errorf("calls = %v", engine.calls)
	}
}

func TestSecretPasswordLoaded(t *testing.T) {
	// The password is read from the credential store at start; settings.json
	// never has it.
	secrets := &memSecrets{m: map[string]string{store.ProxyPasswordKey: "s3cret"}}
	a := newTestApp(t, &fakeEngine{})
	a.deps.secrets = secrets
	a.store.Secrets = secrets
	if err := a.store.SaveSettings(settings.Default(a.deps.defaultDir)); err != nil {
		t.Fatal(err)
	}
	// start() reads the password in the windowed app; emulate that step.
	if password := a.store.LoadProxyPassword(); password != "s3cret" {
		t.Errorf("password = %q", password)
	}
}

func TestProxyPasswordSavedOnClose(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	a.settingsOpen = true
	a.settingsTab = 1
	a.setProxyMode(2) // custom, prefilled with 127.0.0.1
	a.proxyDraft.Password = "s3cret"
	a.commitProxy()
	tt := testerFor(a)
	tt.Frame()
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settingsOpen {
		t.Error("the settings dialog stayed open")
	}
	got := a.store.LoadProxyPassword()
	if got != "s3cret" {
		t.Errorf("saved password = %q", got)
	}
	if a.s.Proxy.Password != "s3cret" {
		t.Errorf("the in-memory password = %q", a.s.Proxy.Password)
	}
}

func TestShutdownSavesEverything(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	engine.items = []download.Item{seedItem("id1", "a.zip", download.StatePaused)}
	a.proxyDraft.Password = "pw"
	a.s.Proxy = proxy.Config{
		Mode: proxy.ModeManual, Kind: proxy.KindHTTP,
		Host: "127.0.0.1", Port: 7890,
	}
	a.shutdown()
	if !engine.hasCall("Shutdown") {
		t.Errorf("calls = %v", engine.calls)
	}
	records, err := a.store.LoadTasks()
	if err != nil || len(records) != 1 || records[0].FileName != "a.zip" {
		t.Errorf("records = %+v, %v", records, err)
	}
	loaded, err := a.store.LoadSettings(a.deps.defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Proxy.Mode != proxy.ModeManual {
		t.Errorf("saved proxy = %+v", loaded.Proxy)
	}
	if password := a.store.LoadProxyPassword(); password != "pw" {
		t.Errorf("saved password = %q", password)
	}
}
