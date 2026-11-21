package app

import (
	"testing"

	"bowerbird/core"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestResolveDropZoneDisplays(t *testing.T) {
	displays := []mygo.Display{
		{
			ID:       1,
			Label:    "Built-in Retina Display",
			Bounds:   mygo.Rectangle{X: 0, Y: 0, Width: 1728, Height: 1117},
			WorkArea: mygo.Rectangle{X: 0, Y: 38, Width: 1728, Height: 1005},
			Internal: true,
			Primary:  true,
		},
		{
			ID:       2,
			Label:    "DELL U2720Q",
			Bounds:   mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080},
			WorkArea: mygo.Rectangle{X: -1920, Y: 25, Width: 1920, Height: 1055},
			Internal: false,
			Primary:  false,
		},
	}

	// Disabled returns nil.
	if got := ResolveDropZoneDisplays(displays, core.DropZoneSettings{Enabled: false, ScreenMode: core.DropZoneScreenAll}); len(got) != 0 {
		t.Fatalf("expected 0 displays when disabled, got %d", len(got))
	}

	// Primary mode returns only primary display.
	got := ResolveDropZoneDisplays(displays, core.DropZoneSettings{Enabled: true, ScreenMode: core.DropZoneScreenPrimary})
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("primary mode got %+v, want display 1", got)
	}

	// All mode returns all connected displays.
	got = ResolveDropZoneDisplays(displays, core.DropZoneSettings{Enabled: true, ScreenMode: core.DropZoneScreenAll})
	if len(got) != 2 {
		t.Fatalf("all mode got %d displays, want 2", len(got))
	}

	// Custom mode filters to connected selected IDs (ignoring disconnected IDs).
	got = ResolveDropZoneDisplays(displays, core.DropZoneSettings{
		Enabled:    true,
		ScreenMode: core.DropZoneScreenCustom,
		ScreenIDs:  []int64{2, 999},
	})
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("custom mode got %+v, want display 2", got)
	}
}

func TestDropZoneWindowBoundsMultiScreen(t *testing.T) {
	notchMac := mygo.Display{
		ID:       1,
		Bounds:   mygo.Rectangle{X: 0, Y: 0, Width: 1728, Height: 1117},
		WorkArea: mygo.Rectangle{X: 0, Y: 38, Width: 1728, Height: 1005},
		Internal: true,
		Primary:  true,
	}
	extLeft := mygo.Display{
		ID:       2,
		Bounds:   mygo.Rectangle{X: -1920, Y: 100, Width: 1920, Height: 1080},
		WorkArea: mygo.Rectangle{X: -1920, Y: 125, Width: 1920, Height: 1055},
		Internal: false,
		Primary:  false,
	}

	if !HasHardwareNotch(notchMac) {
		t.Fatalf("expected notchMac to be detected as having a hardware notch")
	}
	if HasHardwareNotch(extLeft) {
		t.Fatalf("expected extLeft not to have a hardware notch")
	}

	// Notch on MacBook with hardware notch sits at Y=0 and extends below the notch.
	b := DropZoneWindowBounds(notchMac, core.DropZoneStyleNotch)
	if b.Y != 0 || b.Height != dropZoneHWNotchHeight || b.Width != dropZoneHWNotchWidth {
		t.Fatalf("notchMac notch bounds = %+v", b)
	}
	if b.X != (1728-dropZoneHWNotchWidth)/2 {
		t.Fatalf("notchMac notch X = %d, want %d", b.X, (1728-dropZoneHWNotchWidth)/2)
	}

	// Capsule on MacBook sits cleanly below WorkArea.Y.
	b = DropZoneWindowBounds(notchMac, core.DropZoneStyleCapsule)
	if b.Y != 38+dropZoneCapsuleTopGap || b.Height != dropZoneCapsuleHeight {
		t.Fatalf("notchMac capsule bounds = %+v", b)
	}

	// Notch on external monitor at negative X and offset Y sits at top-center of that monitor.
	b = DropZoneWindowBounds(extLeft, core.DropZoneStyleNotch)
	wantX := -1920 + (1920-dropZoneNotchWidth)/2
	if b.X != wantX || b.Y != 100 || b.Height != dropZoneNotchHeight {
		t.Fatalf("extLeft notch bounds = %+v, want X=%d Y=100", b, wantX)
	}

	// Capsule on external monitor sits below its WorkArea.Y.
	b = DropZoneWindowBounds(extLeft, core.DropZoneStyleCapsule)
	if b.X != -1920+(1920-dropZoneCapsuleWidth)/2 || b.Y != 125+dropZoneCapsuleTopGap {
		t.Fatalf("extLeft capsule bounds = %+v", b)
	}
}

func TestDropZoneViewAndSettingsRender(t *testing.T) {
	t.Setenv("BOWERBIRD_USER_DATA_DIR", t.TempDir())
	svc := core.NewService(nil)
	a := New(svc)
	a.SetDisplaysProvider(func() []mygo.Display {
		return []mygo.Display{
			{
				ID:       10,
				Label:    "Built-in Retina Display",
				Bounds:   mygo.Rectangle{X: 0, Y: 0, Width: 1512, Height: 982},
				WorkArea: mygo.Rectangle{X: 0, Y: 38, Width: 1512, Height: 870},
				Internal: true,
				Primary:  true,
			},
			{
				ID:       20,
				Label:    "Studio Display",
				Bounds:   mygo.Rectangle{X: 1512, Y: 0, Width: 2560, Height: 1440},
				WorkArea: mygo.Rectangle{X: 1512, Y: 25, Width: 2560, Height: 1415},
				Internal: false,
				Primary:  false,
			},
		}
	})

	// Render Notch mode.
	notchTester := ui.NewTester(func(c *ui.Context) {
		a.dropZoneView(c, 10, true)
	}, dropZoneHWNotchWidth, dropZoneHWNotchHeight)
	notchTester.Frame()

	// Switch to Capsule mode and render.
	a.setDropZoneSettings(core.DropZoneSettings{
		Enabled:    true,
		Style:      core.DropZoneStyleCapsule,
		ScreenMode: core.DropZoneScreenCustom,
		ScreenIDs:  []int64{10, 20},
	})
	capsuleTester := ui.NewTester(func(c *ui.Context) {
		a.dropZoneView(c, 20, false)
	}, dropZoneCapsuleWidth, dropZoneCapsuleHeight)
	capsuleTester.Frame()

	// Render Settings General view with custom screen list.
	a.refreshSettings()
	a.settingsSection = 0
	settingsTester := ui.NewTester(a.viewSettings, 860, 720)
	settingsTester.Frame()
}
