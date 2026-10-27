package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// addCard is the batch URL input above the list.
func (a *app) addCard(c *ui.Context) {
	p := a.palette
	card := ui.Column(c).Padding(14).Gap(8).Radius(14).Background(p.Card).Border(1, p.Border)
	if !p.Dark {
		card.Shadow(0, 1, 2, 0, p.Shadow)
	}
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "新建下载").FontSize(12).Bold().Grow(1)
			ui.Text(c, "支持 HTTP / HTTPS，每行一个链接").FontSize(10).TextColor(p.Muted)
		})
		ui.TextArea(c, &a.input).Height(56).FontSize(12).Label("下载链接输入").
			Placeholder("粘贴下载链接，例如 https://example.com/file.zip")
		if a.inputErr != "" {
			ui.Text(c, a.inputErr).FontSize(10).TextColor(p.Danger).SingleLine().Tooltip(a.inputErr)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, linkHint(a.input)).FontSize(10).TextColor(p.Muted).Grow(1)
			ui.Text(c, "Ctrl+Enter 添加").FontSize(10).TextColor(p.Muted)
			if ui.Button(c, "粘贴").FontSize(11).Padding(5, 10).Radius(7).Clicked() {
				a.pasteClipboard(c)
			}
			if ui.PrimaryButton(c, "添加").FontSize(12).Padding(6, 16).Radius(8).
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
