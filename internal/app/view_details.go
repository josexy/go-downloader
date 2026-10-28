package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
)

func (a *app) openDetails(id string) {
	a.detailsID, a.detailsOpen = id, true
}

func (a *app) detailsModal(c *ui.Context, items []download.Item) {
	if !a.detailsOpen {
		return
	}
	var item download.Item
	found := false
	for _, it := range items {
		if it.ID == a.detailsID {
			item, found = it, true
			break
		}
	}
	p := a.palette
	width, height := c.Size()
	services := c.Services()
	c.Key("task-details-dialog")
	ui.Modal(c, &a.detailsOpen, func() {
		ui.Column(c).Width(min(620, width-100)).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icoInfo).FontSize(18).TextColor(p.Accent)
				ui.Text(c, "任务详情").FontSize(20).Bold().Grow(1)
				iconButton(c, p, "关闭任务详情", icoX, false, func() { a.detailsOpen = false })
			})
			if !found {
				ui.Text(c, "此任务已从列表移除。").FontSize(14).TextColor(p.Muted)
			} else {
				ui.Scroll(c).Height(min(450, max(180, height-230))).Gap(12).Children(func() {
					ui.Text(c, item.FileName).FontSize(16).Bold().Selectable()
					progressBar(c, p, item)
					for _, field := range taskDetailFields(item, a.now) {
						ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
							ui.Text(c, field.label).Width(100).Shrink(0).FontSize(13).TextColor(p.Muted)
							ui.Text(c, field.value).Grow(1).FontSize(13).Selectable()
						})
					}
					if item.Error != "" {
						ui.Column(c).Gap(6).Padding(12).Radius(8).Background(p.DangerTint).Children(func() {
							ui.Text(c, "错误详情").FontSize(13).Bold().TextColor(p.Danger)
							ui.Text(c, download.DiagnosticText(item.Error)).FontSize(13).TextColor(p.Danger).Selectable()
						})
					}
				})
			}
			ui.Divider(c).Background(p.Divider)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "复制诊断信息").Disabled(!found).OnClick(func() {
					services.WriteClipboard(taskDiagnostic(item, a.now))
					a.toast("已复制诊断信息")
				})
				ui.PrimaryButton(c, "关闭").OnClick(func() { a.detailsOpen = false })
			})
		})
	}).Background(p.Card).Radius(14).Border(1, p.Border)
}

type detailField struct{ label, value string }

func taskDetailFields(it download.Item, now time.Time) []detailField {
	state := stateLabel(it.State)
	if it.Missing {
		state += " · 文件缺失"
	}
	speed, eta := "—", "—"
	if it.State == download.StateDownloading {
		speed, eta = formatSpeed(it.Speed), formatETA(it.ETA)
		if speed == "" {
			speed = "等待测速"
		}
		if eta == "" || it.Total <= 0 {
			eta = "等待估算"
		}
	}
	resume := "尚未确认"
	if it.ResumableKnown || it.Resumable {
		resume = "不支持，继续时重新下载"
		if it.Resumable {
			resume = "支持，继续时校验文件版本"
		}
	}
	if it.NeedsHeaders && (it.State != download.StateCompleted || it.Missing) {
		state += " · 需补填请求头"
	}
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2006-01-02 15:04:05")
	}
	duration := "—"
	if !it.CreatedAt.IsZero() {
		end := now
		if !it.FinishedAt.IsZero() {
			end = it.FinishedAt
		}
		duration = max(time.Duration(0), end.Sub(it.CreatedAt)).Round(time.Second).String()
	}
	return []detailField{
		{"状态", state}, {"下载来源", download.DiagnosticURL(it.URL)}, {"保存路径", it.Path},
		{"文件大小", formatBytes(it.Total)}, {"已下载", formatBytes(it.Done)},
		{"当前速度", speed}, {"预计剩余", eta}, {"自动重试次数", fmt.Sprint(it.RetryCount)},
		{"断点续传", resume}, {"创建时间", stamp(it.CreatedAt)}, {"结束时间", stamp(it.FinishedAt)},
		{"时长（含暂停）", duration},
	}
}

func taskDiagnostic(it download.Item, now time.Time) string {
	var text strings.Builder
	fmt.Fprintf(&text, "下载器任务诊断\n任务 ID: %s\n文件名: %s\n", it.ID, it.FileName)
	for _, field := range taskDetailFields(it, now) {
		fmt.Fprintf(&text, "%s: %s\n", field.label, field.value)
	}
	if it.Error != "" {
		fmt.Fprintf(&text, "错误详情: %s\n", download.DiagnosticText(it.Error))
	}
	return text.String()
}
