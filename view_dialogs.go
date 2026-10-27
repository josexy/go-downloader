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
			ui.Text(c, a.confirm.title).FontSize(18).Bold()
			ui.Text(c, a.confirm.message).FontSize(14).TextColor(p.Muted)
			if a.confirm.deleteOption {
				ui.Checkbox(c, &a.confirm.deleteFiles, "同时删除已下载的文件")
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirm.open = false
				}
				base := c.Theme()
				danger := *base
				danger.Accent, danger.AccentText = p.Danger, p.OnDanger
				danger.AccentHover = p.Danger.Mix(p.Text, 0.12)
				danger.AccentPressed = p.Danger.Mix(p.Text, 0.22)
				c.SetTheme(&danger)
				confirm := ui.PrimaryButton(c, a.confirm.confirmLabel)
				c.SetTheme(base)
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
	}).Background(p.Card).Radius(14).Border(1, p.Border)
}
