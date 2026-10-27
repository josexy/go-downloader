package main

import "github.com/egoist/mygo/ui"

// confirmModal asks before removing downloads or deleting a file.
func (a *app) confirmModal(c *ui.Context) {
	if !a.confirm.open {
		return
	}
	p := a.palette
	ui.Modal(c, &a.confirm.open, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, a.confirm.title).FontSize(16).Bold()
			ui.Text(c, a.confirm.message).FontSize(12).TextColor(p.Muted)
			if a.confirm.deleteOption {
				ui.Checkbox(c, &a.confirm.deleteFiles, "同时删除已下载的文件")
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirm.open = false
				}
				confirm := ui.Button(c, a.confirm.confirmLabel).
					Background(p.Danger).TextColor(ui.Hex("#FFFFFF"))
				if confirm.Clicked() {
					action := a.confirm.onConfirm
					deleteFiles := a.confirm.deleteFiles
					a.confirm.open = false
					if action != nil {
						action(deleteFiles)
					}
				}
			})
		})
	})
}
