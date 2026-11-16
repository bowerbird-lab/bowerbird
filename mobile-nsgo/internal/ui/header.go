package ui

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"
)

// appHeaderSpec describes the shared header container used by the gallery and
// the settings page. Only the placement differs: the gallery's header floats
// over the scrolling grid, while a page header sits in normal flow.
type appHeaderSpec struct {
	Floating bool
	// InsetSafeArea adds the status-bar/safe-area inset for a header that is
	// flush with the window (the gallery). A page inside the root's safe-area
	// padding leaves it off so the inset is not applied twice.
	InsetSafeArea bool
	// OmitLeftSafeArea drops the leading safe-area inset when the header has
	// been moved away from the window's left edge, as the gallery header is
	// while the drawer is open.
	OmitLeftSafeArea func() bool
	TestID           string

	// A page header's leading back button and title. Children replaces them
	// for screens that draw their own row, such as the gallery.
	Title     string
	OnBack    func()
	BackLabel string
	Trailing  []core.Node

	Children []core.Node
}

// appHeader is the one header background container. Both the main gallery and
// the settings page use it, so the safe-area padding, theme background and
// floating/page treatment cannot drift apart again.
// safeSideInset is the shared horizontal margin for the gallery header and
// its content: the larger of the two landscape safe areas plus the normal
// 12 point inset. Using one value keeps the header controls and the grid
// aligned, and keeps both clear of a landscape notch.
func safeSideInset(safe core.Edges) core.Points {
	return max(safe.Left, safe.Right) + 12
}

// appCircleButton is the round surface button shared by the gallery menu and
// page headers, keeping their size, shape and border identical.
func appCircleButton(colors nsui.Colors, testID string, svg func() *core.SVG, onPress func(core.PressEvent)) core.Node {
	return core.View{
		TestID: testID,
		StyleFunc: func() core.Style {
			return core.Style{
				Width: core.Pt(42), Height: core.Pt(42), CornerRadius: 21,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
			}
		},
		OnPress:  onPress,
		Children: []core.Node{nsui.Icon(svg, 20, colors.Text)},
	}
}

func (a *App) appHeader(cx *core.Scope, spec appHeaderSpec) core.Node {
	colors := nsui.ColorsOf(cx)

	content := spec.Children
	if spec.Title != "" || spec.OnBack != nil {
		row := make([]core.Node, 0, len(spec.Trailing)+2)
		if spec.OnBack != nil {
			label := spec.BackLabel
			if label == "" {
				label = "返回"
			}
			row = append(row, nsui.IconButton{Icon: lucide.ChevronLeft, Label: label, Size: 32,
				TestID: spec.TestID + ".back", OnPress: spec.OnBack})
		}
		row = append(row, core.Text{Content: spec.Title, MaxLines: 1, Role: core.RoleHeader,
			Style:     nsui.TypeTitle.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}),
			StyleFunc: nsui.Colored(colors.Text)})
		row = append(row, spec.Trailing...)
		content = []core.Node{core.View{
			Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
			Children: row,
		}}
	}

	return core.View{
		TestID: spec.TestID,
		StyleFunc: func() core.Style {
			bg := colors.Background()
			// The gradient itself is the header's background. A solid fill
			// underneath it hides the fade over content scrolling below.
			st := core.Style{
				Width: core.Pct(100),
				Gradients: []core.Gradient{
					core.LinearGradient(180, bg, nsui.Alpha(bg, 0)),
				},
			}
			if spec.Floating {
				safe := cx.Env().SafeArea()
				st.Position = core.Absolute
				st.Top, st.Left, st.Right = core.Pt(0), core.Pt(0), core.Pt(0)
				left := safe.Left + 12
				right := safe.Right + 12
				if spec.OmitLeftSafeArea != nil && spec.OmitLeftSafeArea() {
					// Landscape safe areas can differ on each side because of
					// the notch. Use the larger inset for both margins so they
					// match and neither control sits under the cutout.
					pad := safeSideInset(safe)
					left, right = pad, pad
				}
				st.Padding = core.Edges{
					Top:    6,
					Bottom: 10,
					Left:   left,
					Right:  right,
				}
				if spec.InsetSafeArea {
					st.Padding.Top = safe.Top + 6
				}
				st.ZIndex = 12
				return st
			}
			st.MinHeight = core.Pt(44)
			st.Padding = core.Symmetric(0, 12)
			return st
		},
		Children: content,
	}
}
