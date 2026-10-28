package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
)

func TestAdvancedFormTypingAndCreation(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	a.setAutoStart(false)
	tt := testerFor(a)
	if err := tt.Click("高级新建"); err != nil {
		t.Fatal(err)
	}
	// The first field takes focus and retains edits across multiple builds.
	tt.Type("https://example.com/private.bin")
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Key(ui.Cmd, ui.KeyA)
	dir := t.TempDir()
	tt.Type(dir)
	if err := tt.Click("任务文件名"); err != nil {
		t.Fatal(err)
	}
	tt.Type("chosen.bin")
	if a.advanced.url != "https://example.com/private.bin" || a.advanced.dir != dir || a.advanced.fileName != "chosen.bin" {
		t.Fatalf("form edits lost: %+v", a.advanced)
	}
	tt.Wheel(620, 420, 0, 400)
	tt.Advance(500 * time.Millisecond)
	if err := tt.Click("请求 Cookie"); err != nil {
		t.Fatal(err)
	}
	tt.Type("session=private")
	if err := tt.Click("自定义请求头"); err != nil {
		t.Fatal(err)
	}
	tt.Type("X-Token: private-token")
	if err := tt.Click("创建下载"); err != nil {
		t.Fatal(err)
	}
	if a.advanced.open || a.advanced.cookie != "" || a.advanced.headers != "" {
		t.Fatal("closing the form did not clear its credentials")
	}
	items, _ := engine.Snapshot()
	if len(items) != 1 || items[0].Path != filepath.Join(dir, "chosen.bin") || items[0].State != download.StatePaused {
		t.Fatalf("advanced form created the wrong task: %+v", items)
	}
	if got := engine.advancedOptions[0].Headers; got.Get("Cookie") != "session=private" || got.Get("X-Token") != "private-token" {
		t.Fatalf("headers were not submitted: %v", got)
	}
	tt.Frame()
	if err := tt.Click("高级新建"); err != nil {
		t.Fatal(err)
	}
	if a.advanced.url != "" {
		t.Fatal("reopened form retained the previous URL")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.advanced.open || a.advanced.dir != "" {
		t.Fatal("Escape did not close and clear the form")
	}
}

func TestAdvancedValidationKeepsFormAndFolderIsPerTask(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	defaultDir := a.s.DownloadDir
	picked := t.TempDir()
	a.openAdvanced()
	a.deps.pickFolder = func(_ *mygo.Window, _ string) (string, error) { return picked, nil }
	a.chooseAdvancedFolder()
	if a.s.DownloadDir != defaultDir || a.advanced.dir != picked {
		t.Fatal("per-task folder changed the default folder")
	}
	a.advanced.url = "https://example.com/private"
	a.advanced.headers = "Range: bytes=0-"
	tt := testerFor(a)
	if err := tt.Click("创建下载"); err != nil {
		t.Fatal(err)
	}
	if !a.advanced.open || !tt.HasText("由下载器管理") {
		t.Fatalf("invalid headers did not leave a useful error: %q", tt.Texts())
	}
}

func TestTaskDetailsUpdateAndCopySanitizedDiagnostics(t *testing.T) {
	item := seedItem("details", "sample.bin", download.StateDownloading)
	item.URL = "https://user:private-password@example.com/sample.bin?token=private-query"
	item.Total = -1
	engine := &fakeEngine{items: []download.Item{item}}
	a := newTestApp(t, engine)
	tt := testerFor(a)
	if err := tt.Click("任务详情 sample.bin"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("未知大小") || !tt.HasText("尚未确认") || !tt.HasText("等待估算") {
		t.Fatalf("details do not distinguish unknown values: %q", tt.Texts())
	}
	engine.items[0].State = download.StateFailed
	engine.items[0].Error = "请求失败 https://user:private-password@example.com/a?token=private-query"
	engine.items[0].RetryCount = 2
	engine.items[0].ResumableKnown = true
	tt.Frame()
	if !tt.HasText("错误详情") || !tt.HasText("不支持，继续时重新下载") {
		t.Fatalf("details did not follow engine changes: %q", tt.Texts())
	}
	if err := tt.Click("复制诊断信息"); err != nil {
		t.Fatal(err)
	}
	diagnostic := tt.Clipboard()
	if strings.Contains(diagnostic, "private-") || !strings.Contains(diagnostic, "自动重试次数: 2") {
		t.Fatalf("diagnostic is incomplete or contains credentials: %s", diagnostic)
	}
	engine.items = nil
	tt.Frame()
	if !tt.HasText("此任务已从列表移除") {
		t.Fatal("details kept a removed task snapshot")
	}
}

func TestRateLimitUIAppliesAndPersists(t *testing.T) {
	engine := &fakeEngine{}
	a := newTestApp(t, engine)
	a.openSettings(0)
	tt := testerFor(a)
	if r, ok := tt.Find("全局限速 KB/s"); ok {
		// The number widget also contains two step buttons; type in its text box.
		tt.ClickAt(r.X+20, r.Y+r.H/2)
	} else {
		t.Fatal("rate input is missing")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("512")
	if a.s.RateLimit != 512*1024 || !engine.hasCall("SetRateLimit:524288") {
		t.Fatalf("typed rate did not reach the engine: %d, %v", a.s.RateLimit, engine.calls)
	}
	stored, err := a.store.LoadSettings(a.deps.defaultDir)
	if err != nil || stored.RateLimit != a.s.RateLimit {
		t.Fatalf("rate limit was not persisted: %+v, %v", stored, err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("0")
	if a.s.RateLimit != 0 || !engine.hasCall("SetRateLimit:0") {
		t.Fatal("zero did not remove the rate limit")
	}
}

func TestFeatureDialogsFitCompactWindow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := newTestApp(t, &fakeEngine{items: sampleItems()})
		tt := testerFor(a)
		tt.SetSize(960, 640)
		tt.SetScale(1.25)
		tt.SetDark(dark)
		a.openAdvanced()
		tt.Frame()
		for _, label := range []string{"高级下载链接", "创建下载", "关闭高级新建"} {
			if r, ok := tt.Find(label); !ok || r.Y < 0 || r.Y+r.H > 640 || r.X < 0 || r.X+r.W > 960 {
				t.Fatalf("compact advanced control %s outside window: %+v, found=%v", label, r, ok)
			}
		}
		a.clearAdvanced()
		a.openDetails("d1")
		tt.Frame()
		if r, ok := tt.Find("复制诊断信息"); !ok || r.Y+r.H > 640 {
			t.Fatalf("compact details footer is unreachable: %+v, found=%v", r, ok)
		}
	}
}

func TestDetailsDescribeWaitingPausedCompletedAndMissingFiles(t *testing.T) {
	for _, tc := range []struct {
		state   download.State
		missing bool
		want    string
	}{
		{download.StateQueued, false, "等待中"},
		{download.StatePaused, false, "已暂停"},
		{download.StateCompleted, false, "已完成"},
		{download.StateCompleted, true, "文件缺失"},
	} {
		it := seedItem("task", "state.bin", tc.state)
		it.Missing = tc.missing
		a := newTestApp(t, &fakeEngine{items: []download.Item{it}})
		a.openDetails(it.ID)
		tt := testerFor(a)
		if !tt.HasText(tc.want) || tt.HasText("剩 25 秒") || tt.HasText("等待测速") {
			t.Fatalf("non-running details show stale transfer values: %q", tt.Texts())
		}
	}
}
