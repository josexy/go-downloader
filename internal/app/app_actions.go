package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
)

// addDownloads parses the pasted links and queues them.
func (a *app) addDownloads() {
	raw := strings.TrimSpace(a.input)
	if raw == "" {
		return
	}
	added, skipped, invalid := a.engine.Add(raw)
	if added > 0 {
		a.toast(fmt.Sprintf("已添加 %d 个下载", added))
	}
	if skipped > 0 {
		a.toast(fmt.Sprintf("跳过 %d 个重复链接", skipped))
	}
	if len(invalid) > 0 {
		shown := invalid
		if len(shown) > 3 {
			shown = shown[:3]
		}
		a.inputErr = "无法识别：" + strings.Join(shown, "、")
		if len(invalid) > 3 {
			a.inputErr += fmt.Sprintf(" 等 %d 个链接", len(invalid))
		}
		a.input = strings.Join(invalid, "\n")
	} else {
		a.input = ""
		a.inputErr = ""
	}
	a.dirty.Store(true)
}

// pasteClipboard appends the clipboard's links to the input.
func (a *app) pasteClipboard(c *ui.Context) {
	text := strings.TrimSpace(c.ReadClipboard())
	if text == "" {
		a.toast("剪贴板里没有文本")
		return
	}
	if a.input != "" {
		a.input += "\n"
	}
	a.input += text
	a.inputErr = ""
}

// primaryLabel is the row's main action and its icon.
func primaryAction(it download.Item) (label string, icon *ui.SVG) {
	switch it.State {
	case download.StateQueued, download.StateConnecting, download.StateDownloading:
		return "暂停", icoPause
	case download.StatePaused:
		return "继续", icoPlay
	case download.StateFailed:
		return "重试", icoRefresh
	case download.StateCompleted:
		if it.Missing {
			return "重新下载", icoRefresh
		}
		return "打开", icoExternal
	default:
		return "重试", icoRefresh
	}
}

// primaryAction runs the row's main action.
func (a *app) primaryAction(it download.Item) {
	switch it.State {
	case download.StateQueued, download.StateConnecting, download.StateDownloading:
		a.engine.Pause(it.ID)
	case download.StatePaused:
		a.engine.Resume(it.ID)
	case download.StateFailed:
		a.engine.Retry(it.ID)
	case download.StateCompleted:
		if it.Missing {
			a.engine.Retry(it.ID)
		} else {
			a.openItem(it)
		}
	default:
		a.engine.Retry(it.ID)
	}
}

// openItem opens a finished file with its default app.
func (a *app) openItem(it download.Item) {
	if it.State != download.StateCompleted || it.Missing || a.deps.openPath == nil {
		return
	}
	if err := a.deps.openPath(it.Path); err != nil {
		a.toast("无法打开文件：" + err.Error())
		a.deps.async(a.engine.RefreshFiles)
	}
}

// revealItem shows a finished file in the file manager.
func (a *app) revealItem(it download.Item) {
	if it.State != download.StateCompleted || it.Missing || a.deps.reveal == nil {
		return
	}
	a.deps.reveal(it.Path)
}

// revealDownloadDir opens the download folder.
func (a *app) revealDownloadDir() {
	if a.deps.openPath == nil {
		return
	}
	if err := a.deps.openPath(a.s.DownloadDir); err != nil {
		a.toast("无法打开文件夹：" + err.Error())
	}
}

// copyURL puts a download's link on the clipboard.
func (a *app) copyURL(c *ui.Context, it download.Item) {
	c.WriteClipboard(it.URL)
	a.toast("已复制链接")
}

// copyPath puts a download's path on the clipboard.
func (a *app) copyPath(c *ui.Context, it download.Item) {
	c.WriteClipboard(it.Path)
	a.toast("已复制保存路径")
}

// selectedIDs returns the chosen downloads' IDs.
func (a *app) selectedIDs() []string {
	ids := make([]string, 0, a.selection.Len())
	for id := range a.selection.All() {
		ids = append(ids, id)
	}
	return ids
}

// findItems returns the current state of the given downloads.
func (a *app) findItems(ids []string) []download.Item {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	items, _ := a.engine.Snapshot()
	out := make([]download.Item, 0, len(ids))
	for _, it := range items {
		if wanted[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// pauseSelected pauses the chosen downloads.
func (a *app) pauseSelected() {
	for _, id := range a.selectedIDs() {
		a.engine.Pause(id)
	}
}

// resumeSelected continues the chosen downloads.
func (a *app) resumeSelected() {
	for _, id := range a.selectedIDs() {
		a.engine.Resume(id)
	}
}

// requestRemove asks before removing downloads.
func (a *app) requestRemove(ids []string) {
	if len(ids) == 0 {
		return
	}
	items := a.findItems(ids)
	if len(items) == 0 {
		return
	}
	active := 0
	completed := 0
	for _, it := range items {
		switch it.State {
		case download.StateCompleted:
			completed++
		default:
			active++
		}
	}
	title := fmt.Sprintf("移除 %d 个下载？", len(items))
	if len(items) == 1 {
		title = "移除 " + items[0].FileName + "？"
	}
	message := "正在下载的项目会停止，未完成的临时文件会被删除。"
	if completed > 0 && active == 0 {
		message = "记录会从列表中移除，文件默认保留在磁盘上。"
	}
	a.confirm = confirmState{
		open:         true,
		title:        title,
		message:      message,
		confirmLabel: "确认移除",
		deleteOption: completed > 0,
		deleteFiles:  false,
		onConfirm: func(deleteFiles bool) {
			a.removeNow(ids, deleteFiles)
		},
	}
}

// removeNow removes downloads, and their files when asked.
func (a *app) removeNow(ids []string, deleteFiles bool) {
	if len(ids) == 0 {
		return
	}
	a.engine.RemoveMany(ids, deleteFiles)
	for _, id := range ids {
		a.selection.Remove(id)
	}
	if len(ids) == 1 {
		a.toast("已移除下载")
	} else {
		a.toast(fmt.Sprintf("已移除 %d 个下载", len(ids)))
	}
}

// clearCompleted forgets the finished downloads.
func (a *app) clearCompleted() {
	if a.summary.Completed == 0 {
		a.toast("没有已完成的下载")
		return
	}
	a.engine.ClearCompleted()
	a.selection.Clear()
	a.toast("已清除完成记录")
}

// removeFromList is the context menu's quiet removal.
func (a *app) removeFromList(it download.Item) {
	a.engine.Remove(it.ID, false)
	a.selection.Remove(it.ID)
	a.toast("已从列表移除")
}

// deleteFile asks to delete a finished file.
func (a *app) deleteFile(it download.Item) {
	a.confirm = confirmState{
		open:         true,
		title:        "删除 " + it.FileName + "？",
		message:      "文件会被移到回收站之外永久删除，无法恢复。",
		confirmLabel: "删除文件",
		deleteOption: false,
		onConfirm: func(bool) {
			a.engine.Remove(it.ID, true)
			a.selection.Remove(it.ID)
			a.toast("已删除文件")
		},
	}
}
