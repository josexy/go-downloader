package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
	"go-downloader/internal/settings"
)

// view builds the window's interface from the app's state, every frame.
func (a *app) view(c *ui.Context) {
	a.now = a.deps.now()
	a.dark = a.s.Dark(c.Theme().Dark)
	a.palette = paletteFor(a.dark)
	c.SetTheme(a.palette.theme())

	// The settings dialog may have closed in the frame before, by its
	// button or by Escape; keep what it edited.
	if a.settingsWasOpen && !a.settingsOpen {
		a.settingsWasOpen = false
		a.afterSettingsClosed()
	}

	items, summary := a.engine.Snapshot()
	a.summary = summary
	a.visible = a.computeVisible(items)
	a.pruneSelection()
	a.measureRail(items)

	for _, message := range a.toasts {
		c.Toast(message)
	}
	a.toasts = nil

	a.shortcuts(c)

	p := a.palette
	c.Root().Background(p.Canvas)
	ui.Column(c).Fill().Children(func() {
		a.header(c, summary)
		a.progressRail(c)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, summary)
			a.content(c, summary)
		})
	})
	a.settingsModal(c)
	a.confirmModal(c)
}

// shortcuts handles the window's keys.
func (a *app) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.openSettings(0)
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.focusSearch = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyEnter) && strings.TrimSpace(a.input) != "" && !a.settingsOpen {
		a.addDownloads()
	}
}

// header is the app's top bar.
func (a *app) header(c *ui.Context, summary download.Summary) {
	p := a.palette
	ui.Row(c).Height(72).FillWidth().Padding(0, 24).Gap(12).AlignItems(ui.Center).Background(p.Card).Children(func() {
		logo := ui.Box(c).Size(36, 36).Radius(10).Background(p.Accent)
		logo.Center()
		logo.Children(func() {
			ui.Icon(c, icoDownload).FontSize(18).TextColor(p.OnAccent)
		})
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "下载器").FontSize(17).Bold()
			ui.Text(c, headerSubtitle(summary)).FontSize(12).TextColor(p.Muted)
		})
		ui.Spacer(c)
		search := ui.SearchField(c, &a.query).Width(236).FontSize(13).
			Background(p.Canvas).Label("搜索下载").Placeholder("搜索文件名或链接")
		if a.focusSearch {
			search.Focus()
			a.focusSearch = false
		}
		a.themeSwitch(c)
		iconButton(c, p, "设置", icoSettings, false, func() { a.openSettings(0) })
	})
}

func headerSubtitle(summary download.Summary) string {
	if summary.Active > 0 {
		if speed := formatSpeed(summary.Speed); speed != "" {
			return fmt.Sprintf("%d 个进行中 · %s", summary.Active, speed)
		}
		return fmt.Sprintf("%d 个进行中", summary.Active)
	}
	if summary.Total == 0 {
		return "粘贴链接，开始第一次下载"
	}
	if summary.Paused > 0 {
		return fmt.Sprintf("%d 个已暂停", summary.Paused)
	}
	if summary.Failed > 0 {
		return fmt.Sprintf("%d 个下载失败", summary.Failed)
	}
	if summary.Completed == summary.Total {
		return "全部下载已完成"
	}
	return fmt.Sprintf("共 %d 项 · %d 项已完成", summary.Total, summary.Completed)
}

// measureRail computes how much of the working downloads is done.
func (a *app) measureRail(items []download.Item) {
	var done, total int64
	for _, it := range items {
		if it.State.Active() && it.Total > 0 {
			done += it.Done
			total += it.Total
		}
	}
	a.railOn = total > 0
	a.railValue = 0
	if total > 0 {
		a.railValue = float64(done) / float64(total)
	}
}

// progressRail is the thin line under the header: the share of the working
// downloads already transferred.
func (a *app) progressRail(c *ui.Context) {
	p := a.palette
	if !a.railOn {
		ui.Box(c).Height(1).FillWidth().Background(p.Border)
		return
	}
	value := a.railValue
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	track := ui.Box(c).Height(3).FillWidth().Background(p.Track).Clip().
		Label(fmt.Sprintf("当前下载总进度 %.0f%%", value*100))
	track.Children(func() {
		ui.Box(c).Height(3).WidthPercent(float32(value * 100)).Background(p.Accent)
	})
}

// themeSwitch is the three-way light/dark/system control in the header.
func (a *app) themeSwitch(c *ui.Context) {
	a.segmented(c, settings.ThemeLabels,
		[]*ui.SVG{icoSun, icoMoon, icoMonitor},
		settings.ThemeIndex(a.s.Theme), false,
		func(i int) { a.setTheme(settings.ThemeValues[i]) })
}

// sidebar lists the filters, the download folder and the proxy state.
func (a *app) sidebar(c *ui.Context, summary download.Summary) {
	p := a.palette
	selected := a.filter
	ui.Column(c).Width(218).FillHeight().Background(p.Sidebar).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Border).
		Padding(20, 14, 16, 14).Gap(14).Children(func() {
		sectionLabel(c, p, "下载管理").Padding(0, 10)
		base := c.Theme()
		side := *base
		side.Surface = p.Sidebar
		side.SurfacePressed = p.SelectionFill
		side.Accent = p.Accent
		side.AccentText = p.SelectionText
		c.SetTheme(&side)
		ui.Sidebar(c, &selected, func() {
			for _, item := range []struct {
				id    string
				icon  *ui.SVG
				label string
				count int
			}{
				{filterAll, icoInbox, "全部", summary.Total},
				{filterActive, icoDownload, "正在下载", summary.Active},
				{filterPaused, icoPause, "暂停中", summary.Paused},
				{filterCompleted, icoCheck, "下载完成", summary.Completed},
				{filterFailed, icoAlert, "下载失败", summary.Failed},
			} {
				item := item
				entry := ui.SidebarItem(c, item.id, item.icon, item.label).
					Height(42).Padding(8, 10).Radius(8).FontSize(14)
				if item.id == a.filter {
					entry.Background(p.SelectionFill).TextColor(p.SelectionText)
				}
				entry.Children(func() {
					badge := ui.Text(c, strconv.Itoa(item.count)).FontSize(12).
						FontFeatures("tnum").TextColor(p.Muted)
					if item.id == a.filter {
						badge.TextColor(p.SelectionText)
					}
				})
			}
		}).FillWidth().Padding(0).Gap(5)
		c.SetTheme(base)
		ui.Spacer(c)
		a.folderCard(c)
		a.proxyCard(c)
	})
	if selected != a.filter && isFilter(selected) {
		a.filter = selected
		a.selection.Clear()
	}
}

func isFilter(id string) bool {
	switch id {
	case filterAll, filterActive, filterPaused, filterCompleted, filterFailed:
		return true
	}
	return false
}

// folderCard shows where new downloads go.
func (a *app) folderCard(c *ui.Context) {
	p := a.palette
	ui.Column(c).Padding(12, 10).Gap(10).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Border).Children(func() {
		sectionLabel(c, p, "保存到")
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoFolder).FontSize(13).TextColor(p.Muted)
			ui.Text(c, folderLabel(a.s.DownloadDir)).FontSize(13).SingleLine().Grow(1).Tooltip(a.s.DownloadDir)
		})
		ui.Row(c).Gap(6).Children(func() {
			if smallButton(c, "更改").Clicked() {
				a.chooseFolder()
			}
			if smallButton(c, "打开").Clicked() {
				a.revealDownloadDir()
			}
		})
	})
}

// proxyCard opens the proxy settings.
func (a *app) proxyCard(c *ui.Context) {
	p := a.palette
	card := ui.Row(c).Padding(10).Gap(8).Radius(8).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if card.Hovered() {
		card.Background(p.CardHover)
	}
	card.OnClick(func() { a.openSettings(1) })
	card.Children(func() {
		ui.Icon(c, icoShield).FontSize(14).TextColor(p.Accent)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, a.proxySummaryShort()).FontSize(12).SingleLine()
			ui.Text(c, "代理设置").FontSize(11).TextColor(p.Muted)
		})
		ui.Icon(c, icoChevron).FontSize(12).TextColor(p.Muted)
	})
}

// content is the main column: add card, toolbar and the downloads.
func (a *app) content(c *ui.Context, summary download.Summary) {
	p := a.palette
	ui.Column(c).Grow(1).FillHeight().Background(p.Canvas).Padding(22, 24).Gap(18).Children(func() {
		a.addCard(c)
		a.toolbar(c, summary)
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			if len(a.visible) == 0 {
				a.emptyState(c)
				return
			}
			if a.s.View == settings.ViewCards {
				a.cards(c)
			} else {
				a.rows(c)
			}
		})
	})
}

// emptyState shows what to do when there is nothing to list.
func (a *app) emptyState(c *ui.Context) {
	p := a.palette
	filtered := a.query != "" || a.filter != filterAll
	ui.Column(c).Fill().Center().Gap(14).Radius(12).Background(p.Card).Border(1, p.Border).Children(func() {
		circle := ui.Box(c).Size(64, 64).Radius(20).Background(p.AccentTint).Center()
		circle.Children(func() {
			ui.Icon(c, icoDownload).FontSize(26).TextColor(p.Accent)
		})
		if filtered {
			ui.Text(c, "没有匹配的下载").FontSize(18).Bold()
			ui.Text(c, "换个关键词或筛选条件试试").FontSize(13).TextColor(p.Muted)
			ui.Row(c).Justify(ui.Center).Children(func() {
				if smallButton(c, "清除筛选").Clicked() {
					a.query = ""
					a.filter = filterAll
					a.selection.Clear()
				}
			})
			return
		}
		ui.Text(c, "还没有下载").FontSize(18).Bold()
		ui.Text(c, "在上方粘贴链接，即可开始下载").FontSize(13).TextColor(p.Muted)
		ui.Row(c).Justify(ui.Center).Children(func() {
			if ui.PrimaryButton(c, "从剪贴板粘贴").FontSize(12).Padding(6, 14).Radius(8).Clicked() {
				a.pasteClipboard(c)
			}
		})
	})
}
