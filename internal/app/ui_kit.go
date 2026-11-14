package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"bowerbird/core"
	"bowerbird/internal/theme"

	"github.com/egoist/mygo/ui"
)

const (
	sidebarWidth          = 224
	inspectorWidth        = 304
	headerHeight          = 52
	headerSecondRowHeight = 38
	mainHeaderHeight      = headerHeight + headerSecondRowHeight
	galleryTopPadding     = 10
	gallerySidePadding    = 10
	// inspectorTopHeight is the inspector's window-drag strip. The panel has
	// no toolbar of its own, so a short strip is enough; the content starts
	// right below it instead of leaving a whole header row empty. Platforms
	// that draw window buttons on the right (Windows, Linux) keep the full
	// title bar height so the buttons never sit on top of the content.
	inspectorTopHeight = 20
)

// palette is the app's semantic colour set, resolved from a theme JSON. It is
// rebuilt only when the theme or appearance changes, never per frame.
type palette struct {
	dark           bool
	bg             ui.Color
	sidebar        ui.Color
	surface        ui.Color
	surface2       ui.Color
	text           ui.Color
	muted          ui.Color
	faint          ui.Color
	hairline       ui.Color
	hover          ui.Color
	press          ui.Color
	accent         ui.Color
	onAccent       ui.Color
	cardShade      ui.Color
	danger         ui.Color
	warning        ui.Color
	info           ui.Color
	star           ui.Color
	starStrong     ui.Color
	overlay        ui.Color
	preview        ui.Color
	badge          ui.Color
	onBadge        ui.Color
	headerTint     ui.Color
	headerBlur     float32
	sidebarOpacity float32
}

func paletteFor(t *theme.Theme) palette {
	c := t.Colors
	return palette{
		dark:           t.Appearance == theme.Dark,
		bg:             themeColor(c.Background),
		sidebar:        themeColor(c.Sidebar),
		surface:        themeColor(c.Surface),
		surface2:       themeColor(c.SurfaceAlt),
		text:           themeColor(c.Text),
		muted:          themeColor(c.TextMuted),
		faint:          themeColor(c.TextFaint),
		hairline:       themeColor(c.Border),
		hover:          themeColor(c.Hover),
		press:          themeColor(c.Pressed),
		accent:         themeColor(c.Accent),
		onAccent:       themeColor(c.OnAccent),
		cardShade:      themeColor(c.Shadow),
		danger:         themeColor(c.Danger),
		warning:        themeColor(c.Warning),
		info:           themeColor(c.Info),
		star:           themeColor(c.Star),
		starStrong:     themeColor(c.StarStrong),
		overlay:        themeColor(c.Overlay),
		preview:        themeColor(c.Preview),
		badge:          themeColor(c.Badge),
		onBadge:        themeColor(c.OnBadge),
		headerTint:     themeColor(t.Material.HeaderTint),
		headerBlur:     t.Material.HeaderBlur,
		sidebarOpacity: t.Material.SidebarOpacity,
	}
}

// themeColor parses a token; a broken user theme must not take a window down,
// so a bad value falls back to transparent.
func themeColor(s string) ui.Color {
	col, err := theme.ParseColor(s)
	if err != nil {
		return ui.Transparent
	}
	return col
}

// uiThemeFor maps the app palette onto mygo's control theme, so built-in
// widgets (switches, inputs, pickers, sliders) follow the same colours.
func uiThemeFor(p palette) *ui.Theme {
	var t *ui.Theme
	if p.dark {
		t = ui.DarkTheme()
	} else {
		t = ui.LightTheme()
	}
	t.Background = p.bg
	t.Surface = p.surface
	t.SurfaceHover = p.surface2
	t.SurfacePressed = p.press
	t.Border = p.hairline
	t.Text = p.text
	t.TextMuted = p.muted
	t.Accent = p.accent
	t.AccentHover = p.accent
	t.AccentPressed = p.accent
	t.AccentText = p.onAccent
	t.Danger = p.danger
	t.Warning = p.warning
	t.Focus = p.accent.Alpha(0.55)
	t.Scrollbar = p.faint
	return t
}

// iconButton is the standard square toolbar button. The icon inherits hover and
// selection states through AnimateColor, so nothing pops.
func (a *App) iconButton(c *ui.Context, key any, icon *ui.SVG, selected bool, label string, onClick func()) {
	if icon == nil {
		return
	}
	p := a.palette(c)
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Radius(7).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
	bg := ui.Transparent
	switch {
	case selected:
		bg = p.press
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Tooltip(label)
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(p.text)
	})
	if b.Clicked() && onClick != nil {
		onClick()
	}
}
func (a *App) iconButtonMenu(c *ui.Context, key any, icon *ui.SVG, label string, menu func(m *ui.Menu)) {
	if icon == nil {
		return
	}
	p := a.palette(c)
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Radius(7).FocusRing(false).AlignItems(ui.Center).Justify(ui.Center)
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Tooltip(label)
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(p.text)
	})
	b.PopoverMenuButton(menu)
}

func (a *App) pillButton(c *ui.Context, key any, label string, active bool, onClick func()) {
	p := a.palette(c)
	b := ui.ButtonBase(c.Key(key)).Height(28).PaddingX(10).Radius(999).FocusRing(false).AlignItems(ui.Center)
	bg := p.surface2
	fg := p.text
	if active {
		bg = p.accent
		fg = p.onAccent
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(550).TextColor(fg) })
	if b.Clicked() && onClick != nil {
		onClick()
	}
}

func (a *App) sectionLabel(c *ui.Context, text string) {
	p := a.palette(c)
	ui.Text(c, strings.ToUpper(text)).FontSize(10).FontWeight(650).LetterSpacing(0.6).TextColor(p.faint).Padding(0, 4).MarginY(6)
}

func (a *App) hairline(c *ui.Context) {
	p := a.palette(c)
	ui.Box(c).FillWidth().Height(1).Background(p.hairline)
}

func (a *App) emptyState(c *ui.Context, icon *ui.SVG, title, subtitle string) {
	p := a.palette(c)
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		box := ui.Box(c).Size(56, 56).Radius(16).Background(p.surface2).AlignItems(ui.Center).Justify(ui.Center)
		box.Children(func() { ui.Icon(c, icon).Size(26, 26).TextColor(p.faint) })
		ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(p.text)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(12).TextColor(p.muted).TextAlign(ui.Center).MaxWidth(320)
		}
	})
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := int(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))
	if i < 0 {
		i = 0
	}
	if i >= len(units) {
		i = len(units) - 1
	}
	v := float64(bytes) / math.Pow(1024, float64(i))
	if i == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func formatDate(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

func formatDuration(seconds float64) string {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return "00:00"
	}
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func categoryOf(item core.Item) string {
	e := strings.ToLower(item.Extension)
	m := strings.ToLower(item.MimeType)
	switch {
	case e == "ts" && strings.HasPrefix(m, "video/"):
		return "video"
	case in(e, "png", "jpg", "jpeg", "webp", "gif", "bmp", "svg", "tiff", "tif", "avif", "ico"):
		return "image"
	case in(e, "mp4", "mov", "qt", "mkv", "avi", "webm", "m4v", "flv", "mts", "m2ts", "mpg", "mpeg", "wmv", "3gp", "ogv"):
		return "video"
	case in(e, "mp3", "wav", "flac", "aac", "ogg", "m4a", "opus"):
		return "audio"
	case in(e, "pdf", "txt", "md", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "json", "js", "ts", "go", "py"):
		return "document"
	case in(e, "zip", "tar", "gz", "rar", "7z"):
		return "archive"
	case strings.HasPrefix(m, "image/"):
		return "image"
	case strings.HasPrefix(m, "video/"):
		return "video"
	case strings.HasPrefix(m, "audio/"):
		return "audio"
	default:
		return "other"
	}
}

func categoryIcon(item core.Item) *ui.SVG {
	switch categoryOf(item) {
	case "image":
		return iconImage
	case "video":
		return iconVideo
	case "audio":
		return iconAudio
	case "document":
		return iconDocument
	case "archive":
		return iconArchive
	default:
		return iconDocument
	}
}

func in(v string, list ...string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
