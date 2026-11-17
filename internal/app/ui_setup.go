package app

import (
	_ "embed"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/setup-bg.png
var setupHeroPNG []byte

var (
	setupHeroOnce sync.Once
	setupHeroBmp  *ui.Bitmap
)

func getSetupHeroBitmap() *ui.Bitmap {
	setupHeroOnce.Do(func() {
		bmp, err := ui.DecodeBitmap(setupHeroPNG)
		if err == nil {
			setupHeroBmp = bmp
		}
	})
	return setupHeroBmp
}

// viewSetup is the first-run / library chooser window. It creates a library in
// a chosen parent directory or opens an existing one, then closes itself when
// core opens the main window.
func (a *App) viewSetup(c *ui.Context) {
	p := a.palette(c)
	a.applyVibrancyRoot(c)
	root := ui.Box(c).Fill().Radius(24).Background(p.bg)
	root.Children(func() {
		if bmp := getSetupHeroBitmap(); bmp != nil {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
				ui.Image(c, bmp).Fill().Fit(ui.Cover).Opacity(0.10)
			})
		}
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
			ui.Row(c).Height(32).FillWidth().DragWindow()
			ui.Scroll(c).Grow(1).Shrink(1).Padding(24, 40, 28, 40).Children(func() {
				ui.Column(c).FillWidth().Gap(16).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, a.t("setup.title")).FontSize(26).FontWeight(750).TextColor(p.text)
						ui.Text(c, a.t("setup.subtitle")).FontSize(12.5).TextColor(p.muted)
					})
				ui.Box(c).Height(6)
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					a.sectionLabel(c, a.t("setup.createTitle"))
					ui.Row(c).Gap(8).Children(func() {
						ui.TextInput(c.Key("setup-name"), &a.dialogValue).Placeholder(a.t("setup.libraryName")).FontSize(13).Grow(1)
						if ui.PrimaryButton(c.Key("setup-create"), a.t("common.create")).Clicked() {
							name := a.dialogValue
							if name == "" {
								a.showToast(a.t("setup.libraryNameRequired"))
							} else {
								go func() {
									parent, err := a.svc.SelectDirectory()
									if err != nil || parent == "" {
										return
									}
									_, err = a.svc.CreateLibrary(parent, name)
									a.post(func() {
										if err != nil {
											a.showToast(err.Error())
										}
									})
								}()
							}
						}
					})
				})
				ui.Box(c).Grow(1)
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					if ui.Button(c.Key("setup-open"), a.t("setup.chooseFolder")).Clicked() {
						go func() {
							path, err := a.svc.SelectLibraryDialog()
							if err != nil || path == "" {
								return
							}
							_, err = a.svc.OpenLibrary(path)
							a.post(func() {
								if err != nil {
									a.showToast(err.Error())
								}
							})
						}()
					}
					ui.Box(c).Grow(1)
					if ui.Button(c, a.t("common.quit")).Clicked() {
						a.quit()
					}
				})
			})
			})
		})
		if a.toastActive() {
			ui.Row(c).Absolute().Left(0).Right(0).Bottom(18).Justify(ui.Center).Children(func() {
				ui.Row(c).PaddingX(14).Height(34).Radius(999).Background(p.text).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.toast).FontSize(11.5).TextColor(p.bg)
				})
			})
			c.After(time.Until(a.toastUntil))
		}
	})
}
