package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

type testTitleBarWindow struct {
	maximized      bool
	minimized      int
	maximizedCalls int
	restored       int
	closed         int
}

func (w *testTitleBarWindow) Minimize() { w.minimized++ }
func (w *testTitleBarWindow) Maximize() {
	w.maximized = true
	w.maximizedCalls++
}
func (w *testTitleBarWindow) Unmaximize() {
	w.maximized = false
	w.restored++
}
func (w *testTitleBarWindow) IsMaximized() bool { return w.maximized }
func (w *testTitleBarWindow) Close()            { w.closed++ }

func TestTitleBarWindowActions(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			window := &testTitleBarWindow{}
			modal := false
			tt := ui.NewTester(func(c *ui.Context) {
				p := paletteFor(c.Theme().Dark)
				c.SetTheme(p.theme())
				ui.Row(c).FillWidth().Height(titleBarHeight).DragWindow().Children(func() {
					ui.Spacer(c)
					windowButtons(c, p, window)
				})
				if modal {
					ui.Modal(c, &modal, func() { ui.Text(c, "设置") })
				}
			}, 960, 640)
			tt.SetDark(dark)
			if err := tt.Click("最小化窗口"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("最大化窗口"); err != nil {
				t.Fatal(err)
			}
			if !window.maximized || !tt.HasText("还原窗口") {
				t.Fatal("maximizing did not update the control")
			}
			// The same keyed button keeps keyboard focus as its label changes.
			tt.Key(0, ui.KeyEnter)
			if window.maximized || !tt.HasText("最大化窗口") {
				t.Fatal("the restore control lost focus after maximizing")
			}
			// A title-bar double click or system command changes native state too.
			window.maximized = true
			tt.Frame()
			if err := tt.Click("还原窗口"); err != nil {
				t.Fatal(err)
			}
			modal = true
			tt.Frame()
			if err := tt.Click("关闭窗口"); err != nil {
				t.Fatal(err)
			}
			if window.closed != 0 {
				t.Fatal("a modal let a click reach the window close button")
			}
			if err := tt.Click("关闭窗口"); err != nil {
				t.Fatal(err)
			}
			if window.minimized != 1 || window.maximizedCalls != 1 || window.restored != 2 || window.closed != 1 {
				t.Fatalf("unexpected window actions: %+v", window)
			}
		})
	}
}

func TestTitleBarAtMinimumWindowSize(t *testing.T) {
	a := newTestApp(t, &fakeEngine{})
	tt := testerFor(a)
	tt.SetSize(960, 640)
	for _, label := range []string{"最小化窗口", "最大化窗口", "关闭窗口"} {
		r, ok := tt.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 960 || r.Y+r.H > titleBarHeight {
			t.Fatalf("%s is outside the title bar: %+v", label, r)
		}
	}
	if err := tt.Click("搜索下载"); err != nil {
		t.Fatal(err)
	}
	tt.Type("file")
	if a.query != "file" {
		t.Fatal("the draggable title bar swallowed search input")
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	if !a.dark {
		t.Fatal("the draggable title bar swallowed the theme control")
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen {
		t.Fatal("the draggable title bar swallowed the settings control")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
}
