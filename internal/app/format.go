package app

import (
	"fmt"
	"time"

	"go-downloader/internal/download"
)

// formatBytes writes a size the way the UI shows it.
func formatBytes(n int64) string {
	if n < 0 {
		return "未知大小"
	}
	const unit = 1024
	value := float64(n)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f %s", value, units[i])
	case value >= 10:
		return fmt.Sprintf("%.1f %s", value, units[i])
	default:
		return fmt.Sprintf("%.2f %s", value, units[i])
	}
}

// formatSpeed writes bytes per second.
func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return ""
	}
	return formatBytes(int64(bytesPerSecond)) + "/s"
}

// formatETA writes a remaining time.
func formatETA(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	seconds := int(d.Seconds())
	switch {
	case seconds < 60:
		return fmt.Sprintf("剩 %d 秒", seconds)
	case seconds < 3600:
		return fmt.Sprintf("剩 %d 分 %d 秒", seconds/60, seconds%60)
	default:
		return fmt.Sprintf("剩 %d 时 %d 分", seconds/3600, (seconds%3600)/60)
	}
}

// formatPercent writes done/total as a whole percentage.
func formatPercent(done, total int64) string {
	if total <= 0 {
		return ""
	}
	p := float64(done) / float64(total) * 100
	if p > 100 {
		p = 100
	}
	if p < 0 {
		p = 0
	}
	return fmt.Sprintf("%.0f%%", p)
}

// relativeTime writes a moment in the past, as the list's captions do.
func relativeTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.After(now) {
		return "刚刚"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// stateLabel names a download state in the UI.
func stateLabel(s download.State) string {
	switch s {
	case download.StateQueued:
		return "等待中"
	case download.StateConnecting:
		return "连接中"
	case download.StateDownloading:
		return "下载中"
	case download.StatePaused:
		return "已暂停"
	case download.StateCompleted:
		return "已完成"
	case download.StateFailed:
		return "失败"
	default:
		return string(s)
	}
}

// detailLine is the smaller caption under the progress bar.
func detailLine(it download.Item, now time.Time) string {
	switch it.State {
	case download.StateQueued:
		return "等待开始"
	case download.StateConnecting:
		if it.Attempt > 0 {
			return it.Error
		}
		return "正在连接服务器…"
	case download.StateDownloading:
		parts := make([]string, 0, 3)
		parts = append(parts, formatBytes(it.Done))
		if it.Total > 0 {
			parts = append(parts, formatBytes(it.Total))
		}
		if speed := formatSpeed(it.Speed); speed != "" {
			parts = append(parts, speed)
		}
		if eta := formatETA(it.ETA); eta != "" {
			parts = append(parts, eta)
		}
		return joinParts(parts)
	case download.StatePaused:
		if it.Total > 0 {
			return "已暂停 · " + formatBytes(it.Done) + " / " + formatBytes(it.Total)
		}
		return "已暂停"
	case download.StateCompleted:
		if it.Missing {
			return "文件已被移动或删除"
		}
		if !it.FinishedAt.IsZero() {
			return "完成于 " + relativeTime(it.FinishedAt, now)
		}
		return "已完成"
	case download.StateFailed:
		return it.Error
	default:
		return ""
	}
}

func joinParts(parts []string) string {
	out := ""
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i > 0 && out != "" {
			out += " · "
		}
		out += part
	}
	return out
}

// progressValue is what the progress bar shows for an item.
func progressValue(it download.Item) float64 {
	switch it.State {
	case download.StateCompleted:
		return 1
	case download.StateConnecting:
		return -1
	case download.StateDownloading:
		if it.Total <= 0 {
			return -1
		}
		return it.Progress()
	default:
		return it.Progress()
	}
}

// pluralDownloads writes "N 项" with its counter.
func pluralDownloads(n int) string {
	return fmt.Sprintf("%d 项", n)
}
