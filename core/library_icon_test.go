package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func libraryDirWithManifest(t *testing.T, manifest map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, libraryManifestName), data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readManifest(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, libraryManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUpdateLibraryDetailsKeepsTheRestOfTheManifest(t *testing.T) {
	dir := libraryDirWithManifest(t, map[string]any{
		"id":        "lib-1",
		"name":      "Old Name",
		"version":   "1.0.0",
		"createdAt": 1700000000,
		"future":    "kept",
	})
	m := &LibraryManager{activeLib: &LibraryInfo{ID: "lib-1", Name: "Old Name", Path: dir, CreatedAt: 1700000000}}

	info, err := m.UpdateLibraryDetails("New Name", "star")
	if err != nil {
		t.Fatalf("UpdateLibraryDetails: %v", err)
	}
	if info.Name != "New Name" || info.Icon != "star" {
		t.Fatalf("info = %+v", info)
	}
	if m.GetActiveLibrary().Name != "New Name" || m.GetActiveLibrary().Icon != "star" {
		t.Fatalf("active library not refreshed: %+v", m.GetActiveLibrary())
	}

	raw := readManifest(t, dir)
	if raw["name"] != "New Name" || raw["icon"] != "star" {
		t.Fatalf("manifest did not record the change: %+v", raw)
	}
	for key, want := range map[string]any{"id": "lib-1", "version": "1.0.0", "future": "kept"} {
		if raw[key] != want {
			t.Fatalf("manifest lost %q: got %v want %v", key, raw[key], want)
		}
	}
	if raw["createdAt"] != float64(1700000000) {
		t.Fatalf("createdAt = %v", raw["createdAt"])
	}
}

func TestUpdateLibraryDetailsRejectsAnEmptyName(t *testing.T) {
	m := &LibraryManager{activeLib: &LibraryInfo{ID: "lib-1", Name: "Old", Path: t.TempDir()}}
	if _, err := m.UpdateLibraryDetails("   ", "star"); err == nil {
		t.Fatal("an empty name was accepted")
	}
}

func TestSaveLibraryIconWritesThePictureAndMarksCustom(t *testing.T) {
	dir := libraryDirWithManifest(t, map[string]any{"id": "lib-1", "name": "Lib"})
	m := &LibraryManager{activeLib: &LibraryInfo{ID: "lib-1", Name: "Lib", Path: dir}}

	info, err := m.SaveLibraryIcon([]byte("png-bytes"))
	if err != nil {
		t.Fatalf("SaveLibraryIcon: %v", err)
	}
	if info.Icon != LibraryIconCustom {
		t.Fatalf("icon = %q, want %q", info.Icon, LibraryIconCustom)
	}
	data, err := os.ReadFile(filepath.Join(dir, LibraryIconFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("icon file = %q", data)
	}
	if readManifest(t, dir)["icon"] != LibraryIconCustom {
		t.Fatal("manifest does not name the custom icon")
	}
	if _, err := m.UpdateLibraryDetails("Lib", LibraryIconCustom); err != nil {
		t.Fatalf("keeping the custom icon: %v", err)
	}

	// Resetting to the default removes the icon key instead of writing "".
	if _, err := m.UpdateLibraryDetails("Lib", ""); err != nil {
		t.Fatalf("reset icon: %v", err)
	}
	if _, ok := readManifest(t, dir)["icon"]; ok {
		t.Fatal("manifest still carries an icon after the reset")
	}
}

func TestUpdateLibraryDetailsRejectsACustomIconWithoutAPicture(t *testing.T) {
	dir := libraryDirWithManifest(t, map[string]any{"id": "lib-1", "name": "Lib"})
	m := &LibraryManager{activeLib: &LibraryInfo{ID: "lib-1", Name: "Lib", Path: dir}}
	if _, err := m.UpdateLibraryDetails("Lib", LibraryIconCustom); err == nil {
		t.Fatal("a custom icon without a picture was accepted")
	}
}
