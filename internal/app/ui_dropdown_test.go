package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDropdownSelectsOption(t *testing.T) {
	options := []SelectOption{
		{Value: "system", Label: "System"},
		{Value: "en", Label: "English"},
		{Value: "zh", Label: "中文"},
	}
	selected := "system"
	changes := 0
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			if value, changed := Dropdown(c, "test-language", selected, options); changed {
				selected = value
				changes++
			}
		})
	}

	tester := ui.NewTester(view, 420, 320)
	tester.Frame()
	if !tester.HasText("System") {
		t.Fatalf("trigger does not show the current option: %v", tester.Texts())
	}
	if tester.HasText("English") {
		t.Fatal("popup is open before the trigger is clicked")
	}

	if err := tester.Click("System"); err != nil {
		t.Fatalf("click trigger: %v", err)
	}
	tester.Frame()
	for _, want := range []string{"English", "中文"} {
		if !tester.HasText(want) {
			t.Fatalf("popup is missing %q: %v", want, tester.Texts())
		}
	}

	if err := tester.Click("English"); err != nil {
		t.Fatalf("click option: %v", err)
	}
	tester.Frame()
	if selected != "en" {
		t.Fatalf("selected = %q, want en", selected)
	}
	if changes != 1 {
		t.Fatalf("change callbacks = %d, want 1", changes)
	}
	if !tester.HasText("English") {
		t.Fatalf("trigger does not show the new option: %v", tester.Texts())
	}
	if tester.HasText("System") {
		t.Fatalf("popup stayed open after choosing: %v", tester.Texts())
	}
}
