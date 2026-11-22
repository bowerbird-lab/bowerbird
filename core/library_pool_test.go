package core

import (
	"testing"
)

func TestWindowServiceIndependentLibraries(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	mgr := NewLibraryManager()
	t.Cleanup(func() {
		if mgr.mobileServer != nil {
			_ = mgr.mobileServer.Stop()
		}
		if mgr.assetServer != nil {
			_ = mgr.assetServer.Stop()
		}
	})

	libA, err := mgr.CreateLibrary(tempDir, "LibA")
	if err != nil {
		t.Fatalf("CreateLibrary LibA: %v", err)
	}
	libB, err := mgr.CreateLibrary(tempDir, "LibB")
	if err != nil {
		t.Fatalf("CreateLibrary LibB: %v", err)
	}
	if _, err := mgr.OpenLibrary(libA.Path); err != nil {
		t.Fatalf("OpenLibrary LibA: %v", err)
	}
	mgr.Pool().RegisterInitial(mgr)

	win1 := NewService(mgr)
	win2 := NewWindowService(mgr, nil)
	if _, err := win2.OpenLibrary(libB.Path); err != nil {
		t.Fatalf("win2.OpenLibrary LibB: %v", err)
	}

	got1, _ := win1.GetActiveLibrary()
	got2, _ := win2.GetActiveLibrary()
	if got1 == nil || got1.Path != libA.Path {
		t.Fatalf("win1 expected %s, got %+v", libA.Path, got1)
	}
	if got2 == nil || got2.Path != libB.Path {
		t.Fatalf("win2 expected %s, got %+v", libB.Path, got2)
	}

	// Releasing win2 must leave win1's library open and intact.
	win2.ReleaseWindow()
	got1After, _ := win1.GetActiveLibrary()
	if got1After == nil || got1After.Path != libA.Path {
		t.Fatalf("win1 after win2 release expected %s, got %+v", libA.Path, got1After)
	}
}
