package ui

import (
	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"
)

const (
	// mobileButtonHeight is 1.5x the desktop nsui.ButtonHeight (42pt) for
	// comfortable touch targets.
	mobileButtonHeight = nsui.ButtonHeight * 1.5

	// mobileIconButtonSize is 0.8x of the 52pt FAB (42pt), matching
	// mobileButtonHeight for balanced header and toolbar touch targets.
	mobileIconButtonSize = core.Points(42)

	// mobileFabSize is the 52x52 footprint of the gallery bottom-right FAB.
	mobileFabSize = core.Points(52)

	// mobileSidebarRowHeight is 0.8x of 56pt (~45pt) for balanced touch density.
	mobileSidebarRowHeight = core.Points(45)
)

// mobileIconVariant selects the surface treatment of a 1:1 icon button.
type mobileIconVariant uint8

const (
	mobileIconSurface mobileIconVariant = iota
	mobileIconPrimary
	mobileIconGhost
)

type mobileIconBtnSpec struct {
	TestID   string
	Label    string
	Icon     func() *core.SVG
	IconSize core.Points
	Variant  mobileIconVariant
	Tint     func() core.Color
	Disabled func() bool
	OnPress  func()
}

// mobileIconButton is the unified 1:1 (52x52) touch-friendly icon button whose
// corner radius follows the active theme configuration.
func mobileIconButton(cx *core.Scope, spec mobileIconBtnSpec) core.Node {
	colors := nsui.ColorsOf(cx)
	iconSize := spec.IconSize
	if iconSize <= 0 {
		iconSize = 20
	}
	tint := spec.Tint
	if tint == nil {
		if spec.Variant == mobileIconPrimary {
			tint = colors.OnAccent
		} else {
			tint = colors.Text
		}
	}
	return core.View{
		TestID: spec.TestID,
		Role:   core.RoleButton,
		Label:  spec.Label,
		StyleFunc: func() core.Style {
			st := core.Style{
				Width:          core.Pt(mobileIconButtonSize),
				Height:         core.Pt(mobileIconButtonSize),
				CornerRadius:   themeButtonRadius(cx),
				AlignItems:     core.AlignCenter,
				JustifyContent: core.JustifyCenter,
				Pressed:        &core.Style{Opacity: core.Alpha(0.78)},
			}
			switch spec.Variant {
			case mobileIconPrimary:
				st.Background = colors.AccentFill()
			case mobileIconGhost:
				st.Background = 0
				st.Active = &core.Style{Background: colors.Hover()}
			default:
				st.Background = colors.Surface()
				st.BorderWidth = 1
				st.BorderColor = colors.Border()
			}
			if spec.Disabled != nil && spec.Disabled() {
				st.Opacity = core.Alpha(0.5)
			}
			return st
		},
		OnPress: func(core.PressEvent) {
			if spec.Disabled != nil && spec.Disabled() {
				return
			}
			if spec.OnPress != nil {
				spec.OnPress()
			}
		},
		Children: []core.Node{nsui.Icon(spec.Icon, iconSize, tint)},
	}
}

type mobileBtnSpec struct {
	TestID   string
	Text     string
	Icon     func() *core.SVG
	Variant  nsui.ButtonVariant
	Compact  bool
	Flex     float32
	Disabled func() bool
	OnPress  func()
}

// mobileActionButton is the unified mobile text/action button: 1.5x height
// (42pt) with corner radius read from the active theme configuration.
func mobileActionButton(cx *core.Scope, colors nsui.Colors, spec mobileBtnSpec) core.Node {
	hPad := core.Points(18)
	fontSize := core.Points(14)
	lineHeight := core.Points(18)
	if spec.Compact {
		hPad = 12
		fontSize = 13
		lineHeight = 17
	}
	return core.View{
		TestID: spec.TestID,
		Role:   core.RoleButton,
		Label:  spec.Text,
		StyleFunc: func() core.Style {
			bg := colors.Accent()
			border := core.Color(0)
			switch spec.Variant {
			case nsui.ButtonSecondary:
				bg = colors.Inset()
				border = colors.Border()
			case nsui.ButtonOutline:
				bg = 0
				border = colors.Border()
			case nsui.ButtonGhost:
				bg = 0
			}
			st := core.Style{
				Height:         core.Pt(mobileButtonHeight),
				Padding:        core.Symmetric(0, hPad),
				CornerRadius:   themeButtonRadius(cx),
				Direction:      core.Row,
				AlignItems:     core.AlignCenter,
				JustifyContent: core.JustifyCenter,
				Gap:            6,
				Background:     bg,
				Pressed:        &core.Style{Opacity: core.Alpha(0.78)},
			}
			if spec.Flex > 0 {
				st.FlexGrow = spec.Flex
				st.FlexShrink = 1
			}
			if border != 0 {
				st.BorderWidth = 1
				st.BorderColor = border
			}
			if spec.Disabled != nil && spec.Disabled() {
				st.Opacity = core.Alpha(0.5)
			}
			return st
		},
		OnPress: func(core.PressEvent) {
			if spec.Disabled != nil && spec.Disabled() {
				return
			}
			if spec.OnPress != nil {
				spec.OnPress()
			}
		},
		Children: func() []core.Node {
			fg := func() core.Color {
				if spec.Variant == nsui.ButtonPrimary {
					return colors.OnAccent()
				}
				return colors.Text()
			}
			var out []core.Node
			if spec.Icon != nil {
				out = append(out, core.Icon{
					SVG:       spec.Icon(),
					StyleFunc: func() core.Style { return core.Style{FontSize: 16, Color: fg()} },
				})
			}
			out = append(out, core.Text{
				Content:   spec.Text,
				StyleFunc: func() core.Style { return core.Style{FontSize: fontSize, LineHeight: lineHeight, FontWeight: 600, Color: fg()} },
			})
			return out
		}(),
	}
}

func headerActionPill(cx *core.Scope, colors nsui.Colors, testID string, iconFn func() *core.SVG, label string, onPress func()) core.Node {
	return mobileActionButton(cx, colors, mobileBtnSpec{
		TestID:  testID,
		Text:    label,
		Icon:    iconFn,
		Variant: nsui.ButtonSecondary,
		OnPress: onPress,
	})
}
