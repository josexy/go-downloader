package app

import (
	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
)

// toolbar is the row above the downloads: counts, selection actions, sorting
// and the list/grid switch.
func (a *app) toolbar(c *ui.Context, summary download.Summary) {
	p := a.palette
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if a.selection.Len() > 0 {
			ui.Textf(c, "已选 %d 项", a.selection.Len()).FontSize(16).Bold().Grow(1).TextColor(p.Accent)
			if smallButton(c, "暂停").Clicked() {
				a.pauseSelected()
			}
			if smallButton(c, "继续").Clicked() {
				a.resumeSelected()
			}
			if smallButton(c, "移除").Clicked() {
				a.requestRemove(a.selectedIDs())
			}
		} else {
			ui.Text(c, a.downloadsTitle()).FontSize(18).Bold()
			ui.Textf(c, "%d 项", len(a.visible)).FontSize(12).TextColor(p.Muted).Grow(1)
			if summary.Active > 0 {
				if smallButton(c, "全部暂停").Clicked() {
					a.engine.PauseAll()
				}
			} else if summary.Queued+summary.Paused+summary.Failed > 0 {
				if smallButton(c, "全部继续").Clicked() {
					a.engine.ResumeAll()
				}
			}
			if summary.Completed > 0 {
				if smallButton(c, "清除完成").Clicked() {
					a.clearCompleted()
				}
			}
		}
		if ui.Select(c.Key("sort"), &a.sortLabel, sortLabels).Width(116).FontSize(13).Label("排序").Changed() {
			a.setSort(sortKeyFor(a.sortLabel))
		}
		a.viewSwitch(c)
	})
}

func (a *app) downloadsTitle() string {
	if a.query != "" {
		return "搜索结果"
	}
	switch a.filter {
	case filterActive:
		return "正在下载"
	case filterPaused:
		return "已暂停"
	case filterCompleted:
		return "已完成"
	case filterFailed:
		return "失败的下载"
	default:
		return "全部下载"
	}
}

// viewSwitch chooses the list or the cards.
func (a *app) viewSwitch(c *ui.Context) {
	a.segmented(c, viewLabels, []*ui.SVG{icoList, icoGrid}, a.viewIdx, false,
		func(i int) { a.setView(viewValues[i]) })
}

// rows builds the virtual list of downloads.
func (a *app) rows(c *ui.Context) {
	a.list.Key = func(i int) any { return a.visible[i].ID }
	a.list.Label = func(i int) string { return a.visible[i].FileName }
	a.list.Selection = &a.selection
	ui.List(c, &a.list, len(a.visible)).Grow(1).Gap(0).
		Radius(12).Background(a.palette.Card).Border(1, a.palette.Border).Clip().
		Rows(func(row ui.ListRow) {
			a.row(row.Context, a.visible[row.Index], row.Selected())
		})
}

// row keeps status and file-type colors intact when selected.
func (a *app) row(c *ui.Context, it download.Item, selected bool) {
	p := a.palette
	row := ui.Row(c).Height(88).FillWidth().Padding(14, 18).Gap(14).
		Background(p.Card).BorderWidth(0, 0, 1, 0).BorderColor(p.Divider).
		TextColor(p.Text).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() && !selected {
		row.Background(p.CardHover)
	}
	if selected {
		row.Background(p.SelectionFill).BorderWidth(0, 0, 1, 3).BorderColor(p.AccentBorder)
	}
	row.ContextMenu(func(m *ui.Menu) { a.itemMenu(c, m, it) })
	row.Children(func() {
		label, fg, bg := fileKind(it.FileName, p.Dark)
		tile := ui.Box(c).Size(42, 42).Radius(10).Background(bg).Center()
		tile.Children(func() {
			ui.Text(c, label).FontSize(10).Bold().TextColor(fg)
		})

		ui.Column(c).Grow(1).Gap(7).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, it.FileName).FontSize(14).FontWeight(600).SingleLine().Grow(1)
				name.Tooltip(it.Path)
				if name.RowDoubleClicked() {
					a.primaryAction(it)
				}
				statusBadge(c, p, it)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				progressBar(c, p, it)
				if percent := formatPercent(it.Done, it.Total); percent != "" {
					ui.Text(c, percent).FontSize(11).TextColor(p.Muted).
						FontFeatures("tnum").Width(40).TextAlign(ui.End)
				}
			})
			detail := detailLine(it, a.now)
			line := ui.Text(c, detail).FontSize(12).SingleLine().TextColor(p.Muted)
			if it.State == download.StateFailed || it.Attempt > 0 {
				line.TextColor(p.Danger)
			}
			if it.Error != "" {
				line.Tooltip(it.Error)
			}
		})

		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			a.itemButtons(c, it)
		})
	})
}

// itemButtons are the row's and card's action icons.
func (a *app) itemButtons(c *ui.Context, it download.Item) {
	p := a.palette
	label, icon := primaryAction(it)
	iconButton(c, p, label+" "+it.FileName, icon, false,
		func() { a.primaryAction(it) })
	iconButton(c, p, "在文件夹中显示 "+it.FileName, icoFolderOpen,
		it.State != download.StateCompleted || it.Missing,
		func() { a.revealItem(it) })
	iconButton(c, p, "移除 "+it.FileName, icoTrash, false,
		func() { a.requestRemove([]string{it.ID}) })
}

// itemMenu is the right-click menu of a download.
func (a *app) itemMenu(c *ui.Context, m *ui.Menu, it download.Item) {
	done := it.State == download.StateCompleted && !it.Missing
	if m.Item("打开").Disabled(!done).Chosen() {
		a.openItem(it)
	}
	if m.Item("在文件夹中显示").Disabled(!done).Chosen() {
		a.revealItem(it)
	}
	m.Separator()
	if m.Item("复制链接").Chosen() {
		a.copyURL(c, it)
	}
	if m.Item("复制保存路径").Chosen() {
		a.copyPath(c, it)
	}
	m.Separator()
	if m.Item("暂停").Disabled(!it.State.Active()).Chosen() {
		a.engine.Pause(it.ID)
	}
	if m.Item("继续").Disabled(it.State != download.StatePaused).Chosen() {
		a.engine.Resume(it.ID)
	}
	if m.Item("重试").Disabled(it.State != download.StateFailed && !(it.State == download.StateCompleted && it.Missing)).Chosen() {
		a.engine.Retry(it.ID)
	}
	m.Separator()
	if m.Item("从列表移除").Chosen() {
		a.removeFromList(it)
	}
	if m.Item("删除文件…").Disabled(!done).Chosen() {
		a.deleteFile(it)
	}
}

// cards builds the grid of downloads.
func (a *app) cards(c *ui.Context) {
	a.grid.Key = func(i int) any { return a.visible[i].ID }
	a.grid.Label = func(i int) string { return a.visible[i].FileName }
	a.grid.Selection = &a.selection
	ui.GridView(c, &a.grid, len(a.visible), 280, 220, func(i int) {
		a.card(c, a.visible[i])
	}).Grow(1)
}

// card is one download in the grid.
func (a *app) card(c *ui.Context, it download.Item) {
	p := a.palette
	selected := a.selection.Has(it.ID)
	card := ui.Column(c).Fill().Padding(16).Gap(10).Radius(12).
		Background(p.Card).TextColor(p.Text).Border(1, p.Border).Cursor(ui.CursorPointer)
	if card.Hovered() {
		card.Background(p.CardHover)
	}
	if selected {
		card.Background(p.SelectionFill).BorderColor(p.AccentBorder)
	}
	card.ContextMenu(func(m *ui.Menu) { a.itemMenu(c, m, it) })
	card.Children(func() {
		label, fg, bg := fileKind(it.FileName, p.Dark)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			tile := ui.Box(c).Size(36, 36).Radius(10).Background(bg).Center()
			tile.Children(func() {
				ui.Text(c, label).FontSize(9).Bold().TextColor(fg)
			})
			ui.Spacer(c)
			statusBadge(c, p, it)
		})
		name := ui.Text(c, it.FileName).FontSize(14).FontWeight(600).SingleLine().Tooltip(it.Path)
		if name.RowDoubleClicked() {
			a.primaryAction(it)
		}
		ui.Text(c, shortLink(it.URL)).FontSize(12).TextColor(p.Muted).SingleLine()
		ui.Spacer(c)
		progressBar(c, p, it)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			color := p.Muted
			if it.State == download.StateFailed || it.Attempt > 0 {
				color = p.Danger
			}
			ui.Text(c, detailLine(it, a.now)).FontSize(12).TextColor(color).
				SingleLine().Grow(1).Tooltip(detailLine(it, a.now))
			if percent := formatPercent(it.Done, it.Total); percent != "" {
				ui.Text(c, percent).FontSize(11).TextColor(p.Muted).FontFeatures("tnum")
			}
		})
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			a.itemButtons(c, it)
		})
	})
}

// shortLink trims a URL to its host and path for a card's caption.
func shortLink(raw string) string {
	u := raw
	if i := len("https://"); len(u) > i && u[:i] == "https://" {
		u = u[i:]
	} else if i := len("http://"); len(u) > i && u[:i] == "http://" {
		u = u[i:]
	}
	if len(u) > 64 {
		u = u[:61] + "…"
	}
	return u
}
