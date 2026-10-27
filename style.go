package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"go-downloader/internal/download"
)

// palette holds the app's own colors: the theme's widgets take theirs from the
// ui.Theme the palette makes, and the cards, tracks and tints use the rest.
type palette struct {
	Dark bool

	Canvas    ui.Color
	Sidebar   ui.Color
	Card      ui.Color
	CardHover ui.Color
	Border    ui.Color
	Track     ui.Color

	Text  ui.Color
	Muted ui.Color

	Accent        ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentTint    ui.Color
	AccentBorder  ui.Color
	OnAccent      ui.Color

	// SelectionFill and SelectionText paint a chosen row: the accent in the
	// light theme, a deeper petrol in the dark one, where the bright accent
	// would make the whole row glow.
	SelectionFill ui.Color
	SelectionText ui.Color

	Success     ui.Color
	SuccessTint ui.Color
	Danger      ui.Color
	DangerTint  ui.Color
	Warning     ui.Color
	WarningTint ui.Color

	Shadow ui.Color
}

func paletteFor(dark bool) palette {
	if dark {
		return palette{
			Dark:          true,
			Canvas:        ui.Hex("#0B1011"),
			Sidebar:       ui.Hex("#0F1516"),
			Card:          ui.Hex("#141B1C"),
			CardHover:     ui.Hex("#1A2424"),
			Border:        ui.Hex("#253031"),
			Track:         ui.Hex("#1D2728"),
			Text:          ui.Hex("#E8EFEC"),
			Muted:         ui.Hex("#92A09B"),
			Accent:        ui.Hex("#55B3C2"),
			AccentHover:   ui.Hex("#70C7D3"),
			AccentPressed: ui.Hex("#419AA8"),
			AccentTint:    ui.RGBA(85, 179, 194, 0.16),
			AccentBorder:  ui.RGBA(85, 179, 194, 0.38),
			OnAccent:      ui.Hex("#051214"),
			SelectionFill: ui.Hex("#1D5560"),
			SelectionText: ui.Hex("#EAF4F2"),
			Success:       ui.Hex("#48D08D"),
			SuccessTint:   ui.RGBA(72, 208, 141, 0.16),
			Danger:        ui.Hex("#FF7A66"),
			DangerTint:    ui.RGBA(255, 122, 102, 0.16),
			Warning:       ui.Hex("#E6AD57"),
			WarningTint:   ui.RGBA(230, 173, 87, 0.16),
			Shadow:        ui.RGBA(0, 0, 0, 0.45),
		}
	}
	return palette{
		Canvas:        ui.Hex("#F3F1EB"),
		Sidebar:       ui.Hex("#FAF9F5"),
		Card:          ui.Hex("#FFFFFF"),
		CardHover:     ui.Hex("#F0EEE7"),
		Border:        ui.Hex("#E1DED4"),
		Track:         ui.Hex("#E8E5DB"),
		Text:          ui.Hex("#1B1F1D"),
		Muted:         ui.Hex("#6B736D"),
		Accent:        ui.Hex("#12657A"),
		AccentHover:   ui.Hex("#0D5265"),
		AccentPressed: ui.Hex("#0A4250"),
		AccentTint:    ui.RGBA(18, 101, 122, 0.10),
		AccentBorder:  ui.RGBA(18, 101, 122, 0.32),
		OnAccent:      ui.Hex("#FFFFFF"),
		SelectionFill: ui.Hex("#12657A"),
		SelectionText: ui.Hex("#FFFFFF"),
		Success:       ui.Hex("#1E8E5A"),
		SuccessTint:   ui.RGBA(30, 142, 90, 0.12),
		Danger:        ui.Hex("#B9472F"),
		DangerTint:    ui.RGBA(185, 71, 47, 0.12),
		Warning:       ui.Hex("#9C6A14"),
		WarningTint:   ui.RGBA(156, 106, 20, 0.12),
		Shadow:        ui.RGBA(28, 31, 27, 0.07),
	}
}

// theme makes the ui.Theme the widgets use.
func (p palette) theme() *ui.Theme {
	t := ui.LightTheme()
	if p.Dark {
		t = ui.DarkTheme()
	}
	t.Background = p.Canvas
	t.Surface = p.Card
	t.SurfaceHover = p.CardHover
	t.SurfacePressed = p.Track
	t.Border = p.Border
	t.Text = p.Text
	t.TextMuted = p.Muted
	t.Accent = p.Accent
	t.AccentHover = p.AccentHover
	t.AccentPressed = p.AccentPressed
	t.AccentText = p.OnAccent
	t.Danger = p.Danger
	t.Warning = p.Warning
	t.Success = p.Success
	t.Selection = ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.26)
	t.Focus = ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.50)
	t.Scrollbar = ui.RGBA(p.Muted.R, p.Muted.G, p.Muted.B, 0.45)
	t.ScrollbarWidth = 8
	t.Radius = 10
	t.Spacing = 4
	t.FontSize = 14
	if p.Dark {
		t.Inverse = ui.Hex("#F1EEE6")
		t.InverseText = ui.Hex("#0B1011")
	} else {
		t.Inverse = ui.Hex("#1E2623")
		t.InverseText = ui.Hex("#F4F2EC")
	}
	return t
}

// tint makes a translucent fill of a color.
func tint(c ui.Color, alpha float32) ui.Color {
	return ui.RGBA(c.R, c.G, c.B, alpha)
}

// fileKind names a file's type and colors its tile in the list. The dark
// theme uses lighter inks so the letters stay readable on the dark card.
func fileKind(name string, dark bool) (label string, fg, bg ui.Color) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	label = strings.ToUpper(ext)
	if len(label) > 4 {
		label = label[:4]
	}
	ink := func(lightHex, darkHex string) ui.Color {
		if dark {
			return ui.Hex(darkHex)
		}
		return ui.Hex(lightHex)
	}
	var color ui.Color
	switch ext {
	case "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "zst":
		color = ink("#4C6E8A", "#9BBFD8")
	case "mp4", "mkv", "mov", "avi", "webm", "flv", "m4v", "ts":
		color = ink("#8A5C45", "#D3A488")
	case "mp3", "flac", "wav", "ogg", "m4a", "aac", "opus":
		color = ink("#A9645A", "#DFA39A")
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "heic", "avif":
		color = ink("#4F7A5E", "#9CC7A8")
	case "pdf":
		label, color = "PDF", ink("#B04A38", "#E8927E")
	case "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "md", "csv", "rtf":
		color = ink("#9C6A14", "#D9B168")
	case "exe", "msi", "dmg", "pkg", "deb", "rpm", "apk", "appimage":
		color = ink("#4E7378", "#9DC5CA")
	case "json", "js", "tsx", "go", "py", "html", "css", "xml", "yml", "yaml", "toml":
		label, color = "DEV", ink("#6B7280", "#A9B1BC")
	default:
		if label == "" {
			label = "FILE"
		}
		color = ink("#6B7280", "#A9B1BC")
	}
	alpha := float32(0.13)
	if dark {
		alpha = 0.22
	}
	return label, color, ui.RGBA(color.R, color.G, color.B, alpha)
}

// iconButton builds a compact icon-only button with a name for the screen
// reader and the tooltip.
func iconButton(c *ui.Context, p palette, label string, icon *ui.SVG, disabled bool, onClick func()) ui.Element {
	return iconButtonOn(c, p, label, icon, disabled, false, onClick)
}

// iconButtonOn is iconButton on a row painted in the accent color, where the
// icon and its hover take the accent's text color.
func iconButtonOn(c *ui.Context, p palette, label string, icon *ui.SVG, disabled, onAccent bool, onClick func()) ui.Element {
	b := ui.Button(c, "")
	b.Label(label).Tooltip(label).Padding(6).Radius(8).FontSize(15)
	b.Background(ui.Transparent)
	b.Disabled(disabled)
	if onAccent {
		b.TextColor(p.SelectionText)
	}
	if disabled {
		b.Opacity(0.35)
	}
	b.Children(func() {
		iconEl := ui.Icon(c, icon).FontSize(15)
		if onAccent {
			iconEl.TextColor(p.SelectionText)
		}
	})
	if !disabled && onClick != nil {
		b.OnClick(onClick)
	}
	if !disabled && b.Hovered() {
		if onAccent {
			b.Background(tint(p.SelectionText, 0.16))
		} else {
			b.Background(p.CardHover)
		}
	}
	return b
}

// smallButton is a quiet button for a card's footer.
func smallButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).FontSize(11).Padding(4, 9).Radius(7)
}

// statusBadge is the pill beside a download's name. On a row painted in the
// accent color it turns over to the accent's text color.
func statusBadge(c *ui.Context, p palette, it download.Item, onAccent bool) ui.Element {
	text := stateLabel(it.State)
	fg, bg := p.Accent, p.AccentTint
	switch it.State {
	case download.StatePaused:
		fg, bg = p.Warning, p.WarningTint
	case download.StateCompleted:
		fg, bg = p.Success, p.SuccessTint
	case download.StateFailed:
		fg, bg = p.Danger, p.DangerTint
	}
	if it.Missing {
		text, fg, bg = "文件缺失", p.Warning, p.WarningTint
	}
	if onAccent {
		// The selection is the row's fill here, so the pill takes its
		// text color and shows the selection as the text.
		fg, bg = p.SelectionFill, p.SelectionText
	}
	return ui.Badge(c, text).Background(bg).TextColor(fg).FontSize(10)
}

// progressBar shows how much of a download is done. On the accent-colored row
// it draws in the accent's text color, since the theme's accent bar would
// disappear into the row.
func progressBar(c *ui.Context, p palette, it download.Item, onAccent bool) ui.Element {
	v := progressValue(it)
	if !onAccent {
		bar := ui.Progress(c, v).Grow(1).Label("下载进度 " + it.FileName)
		if it.State == download.StateFailed {
			bar.Label("下载失败：" + it.FileName)
		}
		return bar
	}
	if v < 0 {
		v = 0.3 // connecting, or a size the server did not give
	}
	track := ui.Box(c).Grow(1).Height(6).Radius(999).
		Background(tint(p.SelectionText, 0.22)).Clip().
		Label("下载进度 " + it.FileName)
	track.Children(func() {
		ui.Box(c).Height(6).Radius(999).WidthPercent(float32(v * 100)).
			Background(p.SelectionText)
	})
	return track
}

// sectionLabel is a small all-caps caption.
func sectionLabel(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(10).Bold().TextColor(p.Muted).LetterSpacing(0.4)
}

// segmented is a segmented control with the app's look. With icons set, the
// segments show them instead of the labels, which stay for screen readers.
// Filled controls share their width among the segments, as settings forms do.
func (a *app) segmented(c *ui.Context, labels []string, icons []*ui.SVG, selected int, filled bool, onPick func(int)) {
	p := a.palette
	seg := ui.SegmentedBase(c, &selected, len(labels))
	track := seg.Track.Padding(3).Radius(9).Background(p.Track).Border(1, p.Border)
	if filled {
		track.FillWidth()
	}
	track.Children(func() {
		for i := range labels {
			i := i
			padding := float32(10)
			if icons != nil {
				padding = 8
			}
			segment := seg.Segment(i).Padding(5, padding).Radius(7).
				Label(labels[i]).Tooltip(labels[i])
			if filled {
				segment.Grow(1)
				if icons == nil {
					segment.Justify(ui.Center)
				}
			}
			chosen := i == selected
			if chosen {
				if p.Dark {
					segment.Background(p.CardHover).Border(1, p.Border)
				} else {
					segment.Background(p.Card).Shadow(0, 1, 2, 0, p.Shadow)
				}
			}
			color := p.Muted
			if chosen {
				color = p.Text
			}
			if icons != nil {
				icon := icons[i]
				segment.Children(func() {
					ui.Icon(c, icon).FontSize(14).TextColor(color)
				})
			} else {
				label := labels[i]
				segment.Children(func() {
					ui.Text(c, label).FontSize(11).TextColor(color)
				})
			}
			segment.OnClick(func() { onPick(i) })
		}
	})
}
