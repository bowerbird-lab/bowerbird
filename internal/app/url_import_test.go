package app

import "testing"

func TestDropDestination(t *testing.T) {
	for _, tt := range []struct {
		name    string
		nav     Nav
		current string
		hovered string
		want    string
	}{
		{name: "current folder", nav: NavFolder, current: "current", want: "current"},
		{name: "hovered folder takes priority", nav: NavFolder, current: "current", hovered: "target", want: "target"},
		{name: "hovered folder from all", nav: NavAll, hovered: "target", want: "target"},
		{name: "all ignores previous folder", nav: NavAll, current: "previous"},
		{name: "smart view ignores previous folder", nav: NavUntagged, current: "previous"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{nav: tt.nav, folderID: tt.current, dropFolderID: tt.hovered}
			got := a.takeDropFolder()
			if got != tt.want {
				t.Fatalf("drop destination = %q, want %q", got, tt.want)
			}
			if a.dropFolderID != "" {
				t.Fatal("drop retained its hovered destination")
			}
			// Changing views while an import is running must not change its
			// captured destination or reuse the previous hover target.
			a.nav, a.folderID = NavAll, ""
			if got != tt.want || a.takeDropFolder() != "" {
				t.Fatal("navigation changed the captured or next drop destination")
			}
		})
	}
}
