package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

func TestFirstImagePath(t *testing.T) {
	if got := firstImagePath([]string{"/tmp/notes.txt", "/tmp/Photo.PNG"}); got != "/tmp/Photo.PNG" {
		t.Fatalf("firstImagePath = %q", got)
	}
	if got := firstImagePath([]string{"/tmp/only-icon"}); got != "/tmp/only-icon" {
		t.Fatalf("a lone path should still be tried, got %q", got)
	}
	if got := firstImagePath([]string{"/tmp/a.txt", "/tmp/b.md"}); got != "" {
		t.Fatalf("firstImagePath = %q, want empty", got)
	}
}

func TestLibraryInspectorOpensTheIconPicker(t *testing.T) {
	a := seededApp(t)
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 800)
	tester.Frame()

	name, ok := tester.Find("Library")
	if !ok {
		t.Fatal("library inspector has no name")
	}
	// The tile sits left of the name, 42 wide with 10pt gaps.
	tester.ClickAt(name.X-30, name.Y+name.H/2)
	tester.Frame()

	if !tester.HasText(a.t("inspector.iconChooseFile")) {
		t.Fatalf("icon picker did not open, or has no file action: %v", tester.Texts())
	}
	if tester.HasText(a.t("inspector.iconReset")) {
		t.Fatal("reset is offered for a library without a custom icon")
	}
}

func TestLibraryInspectorOffersResetForACustomIcon(t *testing.T) {
	a := seededApp(t)
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	// Write a tiny custom icon where SaveLibraryIcon would.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	f, err := os.Create(filepath.Join(a.library.Path, core.LibraryIconFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	a.library.Icon = core.LibraryIconCustom

	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 800)
	tester.Frame()
	if a.libraryIconBitmap(a.library) == nil {
		t.Fatal("custom icon was not decoded from the library folder")
	}
	name, ok := tester.Find("Library")
	if !ok {
		t.Fatal("library inspector has no name")
	}
	tester.ClickAt(name.X-30, name.Y+name.H/2)
	tester.Frame()
	if !tester.HasText(a.t("inspector.iconReset")) {
		t.Fatalf("reset is missing for a custom icon: %v", tester.Texts())
	}
}

func TestLibraryIconPackIsBroadAndUnique(t *testing.T) {
	if len(libraryIconSet) < 150 {
		t.Fatalf("icon pack has %d icons, want a broad set (150+)", len(libraryIconSet))
	}
	seen := map[string]bool{}
	for _, choice := range libraryIconSet {
		if choice.Name == "" || choice.Icon == nil {
			t.Fatalf("icon %q has no name or picture", choice.Name)
		}
		if seen[choice.Name] {
			t.Fatalf("duplicate icon %q", choice.Name)
		}
		seen[choice.Name] = true
	}
	for _, want := range []string{"library", "folder", "star", "image", "palette", "music"} {
		if !seen[want] {
			t.Fatalf("icon pack is missing %q", want)
		}
	}
}

func TestLegacyLibraryIconsStillResolve(t *testing.T) {
	for legacy, want := range map[string]string{
		"audio":    "music",
		"document": "file-text",
		"prompt":   "terminal",
	} {
		if got := resolvedLibraryIconName(legacy); got != want {
			t.Fatalf("resolvedLibraryIconName(%q) = %q, want %q", legacy, got, want)
		}
		if libraryIconByName(legacy) == nil {
			t.Fatalf("legacy icon %q has no picture", legacy)
		}
	}
}

func TestLibraryIconPickerScrollsSideways(t *testing.T) {
	a := seededApp(t)
	a.activeItem = nil
	a.activeItemID = ""
	a.selected = map[string]bool{}
	tester := ui.NewTester(a.viewInspector, int(defaultInspectorWidth), 800)
	tester.Frame()

	name, ok := tester.Find("Library")
	if !ok {
		t.Fatal("library inspector has no name")
	}
	tester.ClickAt(name.X-30, name.Y+name.H/2)
	tester.Frame()

	action, ok := tester.Find(a.t("inspector.iconChooseFile"))
	if !ok {
		t.Fatalf("icon picker did not open: %v", tester.Texts())
	}
	// The grid sits above the file/reset actions.
	x, y := action.X+40, action.Y-80
	tester.Move(x, y)
	tester.Wheel(x, y, 0, 240)
	tester.Frame()
	if a.libIconScroll.X <= 0 {
		t.Fatalf("a vertical wheel did not scroll the icon strip: x=%v max=%v", a.libIconScroll.X, a.libIconScroll.MaxX)
	}
}
