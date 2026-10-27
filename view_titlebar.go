package main

import "github.com/egoist/mygo/ui"

const titleBarHeight = 64

// titleBarWindow is implemented by *mygo.Window. Close follows the normal
// window lifecycle, including the app's save-on-quit handler.
type titleBarWindow interface {
	Minimize()
	Maximize()
	Unmaximize()
	IsMaximized() bool
	Close()
}

func windowButtons(c *ui.Context, p palette, window titleBarWindow) {
	services := c.Services()
	ui.Row(c.Key("window-controls")).Height(titleBarHeight).Padding(0, 14, 0, 0).
		Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(1, 20).Background(p.Border).Margin(0, 10, 0, 0)
		windowButton(c.Key("minimize"), p, "最小化窗口", icoMinimize, false).OnClick(func() {
			if window != nil {
				window.Minimize()
			}
		})
		label, icon := "最大化窗口", icoMaximize
		if window != nil && window.IsMaximized() {
			label, icon = "还原窗口", icoRestore
		}
		windowButton(c.Key("maximize"), p, label, icon, false).MaximizeButton().OnClick(func() {
			if window != nil {
				if window.IsMaximized() {
					window.Unmaximize()
				} else {
					window.Maximize()
				}
				services.Invalidate()
			}
		})
		windowButton(c.Key("close"), p, "关闭窗口", icoWindowX, true).OnClick(func() {
			if window != nil {
				window.Close()
			}
		})
	})
}

func windowButton(c *ui.Context, p palette, label string, icon *ui.SVG, isClose bool) ui.Element {
	button := ui.ButtonBase(c).Size(34, 32).Radius(8).Center().
		Label(label).Tooltip(label)
	fg := p.Muted
	if button.Pressed() {
		button.Background(p.Track)
		fg = p.Text
	} else if button.Hovered() {
		button.Background(p.CardHover)
		fg = p.Text
	}
	if isClose && (button.Hovered() || button.Pressed()) {
		button.Background(p.DangerTint)
		if button.Pressed() {
			button.Background(p.DangerTint.Mix(p.Danger, 0.12))
		}
		fg = p.Danger
	}
	button.Children(func() { ui.Icon(c, icon).FontSize(16).TextColor(fg) })
	return button
}
