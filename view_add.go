package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// addCard is the batch URL input above the list.
func (a *app) addCard(c *ui.Context) {
	p := a.palette
	card := ui.Column(c).Padding(16, 18).Gap(10).Radius(12).Background(p.Card).Border(1, p.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, icoLink).FontSize(16).TextColor(p.Accent)
			ui.Text(c, "新建下载").FontSize(14).Bold().Grow(1)
			ui.Text(c, "支持 HTTP / HTTPS").FontSize(12).TextColor(p.Muted)
		})
		ui.TextArea(c, &a.input).MinHeight(60).Height(60).FontSize(13).Background(p.Canvas).Label("下载链接输入").
			Placeholder("粘贴下载链接，每行一个…")
		if a.inputErr != "" {
			ui.Text(c, a.inputErr).FontSize(12).TextColor(p.Danger).SingleLine().Tooltip(a.inputErr)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, linkHint(a.input)).FontSize(12).TextColor(p.Muted).Grow(1)
			ui.Text(c, "Ctrl+Enter 添加").FontSize(11).TextColor(p.Muted)
			if smallButton(c, "粘贴").Clicked() {
				a.pasteClipboard(c)
			}
			if ui.PrimaryButton(c, "添加").FontSize(13).Padding(7, 20).Radius(7).
				Disabled(strings.TrimSpace(a.input) == "").Clicked() {
				a.addDownloads()
			}
		})
	})
}

// linkHint counts the links in the input.
func linkHint(raw string) string {
	n := 0
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ',' || r == '，'
	}) {
		if strings.TrimSpace(token) != "" {
			n++
		}
	}
	if n == 0 {
		return "支持批量添加"
	}
	return fmt.Sprintf("已输入 %d 个链接", n)
}
