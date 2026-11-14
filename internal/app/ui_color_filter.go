package app

import (
	"fmt"
	"math"
	"strings"

	"bowerbird/internal/theme"

	"github.com/egoist/mygo/ui"
)

// The color filter's picker replaces the menu of hex names with the colors
// themselves: a grid of the library's swatches, and a wheel whose angle picks
// the hue and whose distance from the middle picks the saturation. The wheel
// applies on release, so dragging it filters once rather than every frame.

const (
	colorWheelSize     float32 = 152
	colorSwatchSize    float32 = 22
	colorSwatchesInRow         = 6
)

// hueStops is the wheel's hue ring, red at the top as the knob's angle reads.
var hueStops = func() []ui.GradientStop {
	stops := make([]ui.GradientStop, 0, 13)
	for i := 0; i <= 12; i++ {
		stops = append(stops, ui.GradientStop{Color: hsvColor(float32(i)*30, 1), At: float32(i) / 12})
	}
	return stops
}()

// satStops fades the wheel's hues into white towards its middle, which is how
// its radius picks the saturation.
var satStops = []ui.GradientStop{
	{Color: ui.RGBA(255, 255, 255, 1), At: 0},
	{Color: ui.RGBA(255, 255, 255, 0), At: 1},
}

// filterColorButton is the color criterion's pill: the chosen color as a dot
// (or the palette glyph before one is chosen) and its hex, opening the swatch
// and wheel picker below it.
func (a *App) filterColorButton(c *ui.Context) {
	p := a.palette(c)
	active := a.filterCriterionActive(filterCriterionColor)
	btn := ui.ButtonBase(c.Key("filter-pill-"+filterCriterionColor)).Height(26).PaddingX(9).Radius(999).FocusRing(false).AlignItems(ui.Center).Gap(5).Border(1, p.hairline).Background(p.surface2)
	fg := p.muted
	if active {
		btn.Border(1, p.accent.Alpha(0.55)).Background(p.press)
		fg = p.accent
	}
	btn.Children(func() {
		if hex := a.filterColorValue(); hex != "" {
			if col, err := theme.ParseColor(hex); err == nil {
				ui.Box(c).Size(12, 12).Radius(999).Background(col).Border(1, p.hairline)
			}
		} else {
			ui.Icon(c, iconPalette).Size(13, 13).TextColor(fg)
		}
		ui.Text(c, a.filterCriterionLabel(filterCriterionColor)).FontSize(12).SingleLine().TextColor(p.text)
		ui.Icon(c, iconChevron).Size(11, 11).TextColor(p.faint)
	})
	if btn.Clicked() {
		a.filterColorOpen = !a.filterColorOpen
	}
	ui.PopoverBase(c, btn, &a.filterColorOpen, func(panel ui.Element) {
		a.filterColorPanel(c, panel)
	})
	if !a.filterColorOpen {
		a.filterColorDrag = false
	}
}

// filterColorValue is the color the filter keeps, "" for none.
func (a *App) filterColorValue() string {
	if len(a.filters.Colors) == 0 {
		return ""
	}
	return a.filters.Colors[0]
}

// filterColorKnob is where the wheel's knob sits: on the color the filter
// keeps, or under the pointer while it drags.
func (a *App) filterColorKnob() (hue, sat float32, ok bool) {
	if a.filterColorDrag {
		return a.filterColorHue, a.filterColorSat, true
	}
	if hex := a.filterColorValue(); hex != "" {
		if col, err := theme.ParseColor(hex); err == nil {
			hue, sat = colorHS(col)
			return hue, sat, true
		}
	}
	return 0, 0, false
}

// filterColorPanel is the popover: the swatches of the library's colors, the
// wheel, and the way back to every color.
func (a *App) filterColorPanel(c *ui.Context, panel ui.Element) {
	p := a.palette(c)
	panel.Width(colorSwatchSize*colorSwatchesInRow+5*4+28).Padding(12).Radius(12).Background(p.surface).Border(1, p.hairline).Shadow(0, 18, 44, -14, p.cardShade)
	panel.Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			options := a.filterColorOptions()
			for start := 0; start < len(options); start += colorSwatchesInRow {
				end := start + colorSwatchesInRow
				if end > len(options) {
					end = len(options)
				}
				row := options[start:end]
				ui.Row(c).FillWidth().Gap(4).Children(func() {
					for _, hex := range row {
						hex := hex
						col, err := theme.ParseColor(hex)
						if err != nil {
							continue
						}
						swatch := ui.ButtonBase(c.Key("filter-swatch-"+hex)).Size(colorSwatchSize, colorSwatchSize).Radius(6).Background(col).FocusRing(false)
						if strings.EqualFold(a.filterColorValue(), hex) {
							swatch.Border(2, p.accent)
						} else {
							swatch.Border(1, p.hairline)
						}
						swatch.Tooltip(hex)
						if swatch.Clicked() {
							a.setColorFilter(strings.ToUpper(hex))
							a.filterColorOpen = false
						}
					}
				})
			}
			a.colorWheel(c)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				label := a.t("filters.color")
				if hex := a.filterColorValue(); hex != "" {
					label = hex
				}
				ui.Text(c, label).FontSize(10.5).FontFeatures("tnum").TextColor(p.muted).Grow(1).Shrink(1).SingleLine()
				if ui.ButtonBase(c.Key("filter-color-all")).Height(22).PaddingX(7).Radius(6).FocusRing(false).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.t("filters.allColors")).FontSize(10.5).TextColor(p.muted)
				}).Clicked() {
					a.setColorFilter("")
					a.filterColorOpen = false
				}
			})
		})
	})
}

// colorWheel draws the hue around and the saturation from the middle out, and
// reads the pointer: pressing sets the color under it and dragging moves it,
// applying the color the drag ends on.
func (a *App) colorWheel(c *ui.Context) {
	const size = colorWheelSize
	wheel := ui.Box(c.Key("filter-color-wheel")).Size(size, size).Radius(size / 2).Clip().Cursor(ui.CursorPointer)
	wheel.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.FillConicGradient(r, ui.ConicGradient{Stops: hueStops}, r.W/2)
		pt.FillRadialGradient(r, ui.RadialGradient{Radius: r.W / 2, Stops: satStops}, r.W/2)
		if hue, sat, ok := a.filterColorKnob(); ok {
			var knob ui.Path
			knob.Circle(r.X+r.W/2+sat*(r.W/2)*float32(math.Sin(float64(hue)*math.Pi/180)),
				r.Y+r.H/2-sat*(r.H/2)*float32(math.Cos(float64(hue)*math.Pi/180)), 6)
			pt.FillPath(&knob, hsvColor(hue, sat))
			pt.StrokePath(&knob, 2, ui.RGBA(255, 255, 255, 1))
		}
	})

	x, y, _ := wheel.PointerPosition()
	pick := func() {
		a.filterColorHue, a.filterColorSat = wheelPick(x, y, size)
		a.filterColorDraft = hexOfColor(hsvColor(a.filterColorHue, a.filterColorSat))
	}
	switch {
	case wheel.Pressed():
		pick()
		a.filterColorDrag = true
	case a.filterColorDrag:
		a.filterColorDrag = false
		a.setColorFilter(a.filterColorDraft)
	case wheel.Clicked():
		pick()
		a.setColorFilter(a.filterColorDraft)
	}
}

// wheelPick maps a point of the wheel to the hue and saturation it chooses:
// the angle, clockwise from the top, is the hue, and the distance from the
// middle the saturation, full at the rim.
func wheelPick(x, y, size float32) (hue, sat float32) {
	dx, dy := x-size/2, y-size/2
	dist := float32(math.Hypot(float64(dx), float64(dy)))
	if dist == 0 {
		return 0, 0
	}
	sat = minF(dist/(size/2), 1)
	hue = float32(math.Atan2(float64(dx), float64(-dy))) * 180 / math.Pi
	if hue < 0 {
		hue += 360
	}
	return hue, sat
}

// colorHS returns the hue in degrees and the saturation of c, at full value:
// where its knob belongs on the wheel.
func colorHS(c ui.Color) (hue, sat float32) {
	r, g, b := float32(c.R)/255, float32(c.G)/255, float32(c.B)/255
	hi := maxF(r, maxF(g, b))
	lo := minF(r, minF(g, b))
	d := hi - lo
	if hi <= 0 {
		return 0, 0
	}
	sat = d / hi
	if d == 0 {
		return 0, sat
	}
	switch hi {
	case r:
		hue = 60 * float32(math.Mod(float64((g-b)/d), 6))
	case g:
		hue = 60 * ((b-r)/d + 2)
	default:
		hue = 60 * ((r-g)/d + 4)
	}
	if hue < 0 {
		hue += 360
	}
	return hue, sat
}

// hsvColor returns the color of hue in degrees and saturation at full value.
func hsvColor(hue, sat float32) ui.Color {
	c := sat
	h := math.Mod(float64(hue), 360) / 60
	if h < 0 {
		h += 6
	}
	x := c * (1 - float32(math.Abs(math.Mod(h, 2)-1)))
	var r, g, b float32
	switch {
	case h < 1:
		r, g = c, x
	case h < 2:
		r, g = x, c
	case h < 3:
		g, b = c, x
	case h < 4:
		g, b = x, c
	case h < 5:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := 1 - c
	channel := func(v float32) uint8 { return uint8(math.Round(float64(maxF(0, minF(1, v+m)) * 255))) }
	return ui.RGB(channel(r), channel(g), channel(b))
}

// hexOfColor returns c as an upper case #RRGGBB, as the filter stores colors.
func hexOfColor(c ui.Color) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
