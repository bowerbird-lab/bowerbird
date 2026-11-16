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
	TestID        string

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
				st.Padding = core.Edges{
					Top:    6,
					Bottom: 10,
					Left:   safe.Left + 12,
					Right:  safe.Right + 12,
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
