package app

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
	Divider   ui.Color
	Track     ui.Color

	Text  ui.Color
	Muted ui.Color

	Accent        ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentTint    ui.Color
	AccentBorder  ui.Color
	OnAccent      ui.Color

	// Selection keeps file types and semantic status colors visible.
	SelectionFill ui.Color
	SelectionText ui.Color

	Success     ui.Color
	SuccessTint ui.Color
	Danger      ui.Color
	DangerTint  ui.Color
	OnDanger    ui.Color
	Warning     ui.Color
	WarningTint ui.Color

	Shadow ui.Color
}

func paletteFor(dark bool) palette {
	if dark {
		return palette{
			Dark:          true,
			Canvas:        ui.Hex("#151B26"),
			Sidebar:       ui.Hex("#19212E"),
			Card:          ui.Hex("#1F2938"),
			CardHover:     ui.Hex("#273346"),
			Border:        ui.Hex("#364459"),
			Divider:       ui.Hex("#2B3749"),
			Track:         ui.Hex("#334157"),
			Text:          ui.Hex("#E8EDF5"),
			Muted:         ui.Hex("#A2B0C5"),
			Accent:        ui.Hex("#94B5FF"),
			AccentHover:   ui.Hex("#B0C8FF"),
			AccentPressed: ui.Hex("#7EA3F0"),
			AccentTint:    ui.Hex("#293A5A"),
			AccentBorder:  ui.Hex("#536FAD"),
			OnAccent:      ui.Hex("#162746"),
			SelectionFill: ui.Hex("#293A5A"),
			SelectionText: ui.Hex("#C2D4FF"),
			Success:       ui.Hex("#7AD5AD"),
			SuccessTint:   ui.Hex("#253F3B"),
			Danger:        ui.Hex("#FDA4AF"),
			DangerTint:    ui.Hex("#49303D"),
			OnDanger:      ui.Hex("#39131D"),
			Warning:       ui.Hex("#ECC181"),
			WarningTint:   ui.Hex("#433C30"),
			Shadow:        ui.RGBA(0, 0, 0, 0.24),
		}
	}
	return palette{
		Canvas:        ui.Hex("#F4F6FA"),
		Sidebar:       ui.Hex("#EDF1F7"),
		Card:          ui.Hex("#FFFFFF"),
		CardHover:     ui.Hex("#F0F4FB"),
		Border:        ui.Hex("#DCE3ED"),
		Divider:       ui.Hex("#EAF0F6"),
		Track:         ui.Hex("#E5EAF2"),
		Text:          ui.Hex("#1D2A3D"),
		Muted:         ui.Hex("#617089"),
		Accent:        ui.Hex("#315DCE"),
		AccentHover:   ui.Hex("#264DB7"),
		AccentPressed: ui.Hex("#20429D"),
		AccentTint:    ui.Hex("#E9EFFD"),
		AccentBorder:  ui.Hex("#A8BCEB"),
		OnAccent:      ui.Hex("#FFFFFF"),
		SelectionFill: ui.Hex("#E9EFFD"),
		SelectionText: ui.Hex("#244AA8"),
		Success:       ui.Hex("#18724F"),
		SuccessTint:   ui.Hex("#E6F4ED"),
		Danger:        ui.Hex("#B9384B"),
		DangerTint:    ui.Hex("#FCECEF"),
		OnDanger:      ui.Hex("#FFFFFF"),
		Warning:       ui.Hex("#886014"),
		WarningTint:   ui.Hex("#FBF1DE"),
		Shadow:        ui.RGBA(34, 54, 88, 0.08),
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
	t.Radius = 8
	t.Spacing = 4
	t.FontSize = 14
	if p.Dark {
		t.Inverse = ui.Hex("#E8EDF5")
		t.InverseText = ui.Hex("#1D2A3D")
	} else {
		t.Inverse = ui.Hex("#25334A")
		t.InverseText = ui.Hex("#F4F6FA")
	}
	return t
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
	b := ui.ButtonBase(c).Size(32, 32).Center().Radius(7).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	b.Disabled(disabled)
	color := p.Muted
	if disabled {
		b.Opacity(0.35)
	}
	if !disabled && onClick != nil {
		b.OnClick(onClick)
	}
	if !disabled && b.Hovered() {
		b.Background(p.Track)
		color = p.Text
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(16).TextColor(color) })
	return b
}

// smallButton is a compact secondary action for toolbars and the sidebar.
func smallButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).FontSize(12).Padding(6, 10).Radius(7)
}

// statusColors are shared by badges and progress bars in either view.
func statusColors(p palette, it download.Item) (fg, bg ui.Color) {
	fg, bg = p.Accent, p.AccentTint
	switch it.State {
	case download.StateQueued:
		fg, bg = p.Muted, p.Track
	case download.StatePaused:
		fg, bg = p.Warning, p.WarningTint
	case download.StateCompleted:
		fg, bg = p.Success, p.SuccessTint
	case download.StateFailed:
		fg, bg = p.Danger, p.DangerTint
	}
	if it.Missing {
		fg, bg = p.Warning, p.WarningTint
	}
	return
}

func statusBadge(c *ui.Context, p palette, it download.Item) ui.Element {
	text := stateLabel(it.State)
	fg, bg := statusColors(p, it)
	if it.Missing {
		text = "文件缺失"
	}
	return ui.Badge(c, text).Background(bg).TextColor(fg).FontSize(11).Padding(3, 8)
}

// Keep the native progress widget's indeterminate animation and accessibility.
func progressBar(c *ui.Context, p palette, it download.Item) ui.Element {
	base := c.Theme()
	progress := *base
	progress.Accent, _ = statusColors(p, it)
	progress.Border = p.Track
	c.SetTheme(&progress)
	bar := ui.Progress(c, progressValue(it)).Grow(1).Height(4).MinHeight(4).Shrink(0).
		Label("下载进度 " + it.FileName)
	if it.State == download.StateFailed {
		bar.Label("下载失败：" + it.FileName)
	}
	c.SetTheme(base)
	return bar
}

// sectionLabel is a quiet caption for the sidebar's utility controls.
func sectionLabel(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(12).TextColor(p.Muted)
}

// segmented is a segmented control with the app's look. With icons set, the
// segments show them instead of the labels, which stay for screen readers.
// Filled controls share their width among the segments, as settings forms do.
func (a *app) segmented(c *ui.Context, labels []string, icons []*ui.SVG, selected int, filled bool, onPick func(int)) {
	p := a.palette
	seg := ui.SegmentedBase(c, &selected, len(labels))
	track := seg.Track.Padding(3).Radius(9).Background(p.Sidebar)
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
			segment := seg.Segment(i).Padding(6, padding).Radius(6).
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
					segment.Background(p.CardHover)
				} else {
					segment.Background(p.Card).Shadow(0, 1, 2, 0, p.Shadow)
				}
			}
			color := p.Muted
			if chosen {
				color = p.Accent
			}
			if icons != nil {
				icon := icons[i]
				segment.Children(func() {
					ui.Icon(c, icon).FontSize(14).TextColor(color)
				})
			} else {
				label := labels[i]
				segment.Children(func() {
					ui.Text(c, label).FontSize(13).TextColor(color)
				})
			}
			segment.OnClick(func() { onPick(i) })
		}
	})
}
