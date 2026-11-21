package app

import (
	"os"
	"path/filepath"
	"testing"

	"bowerbird/internal/plugin"

	"github.com/egoist/mygo"
)

func TestIsAllowedExternalNavigation(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://flow.google.com/", true},
		{"https://accounts.google.com/o/oauth2/auth", true},
		{"blob:https://flow.google.com/1234", true},
		{"data:text/plain,hello", true},
		{"about:blank", true},
		{"file:///etc/passwd", false},
		{"bowerplugin://localhost/index.html", false},
		{"javascript:alert(1)", false},
	} {
		if got := isAllowedExternalNavigation(tc.url); got != tc.want {
			t.Errorf("isAllowedExternalNavigation(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestHandlePluginDownloadDoneImportsAndCleansTemp(t *testing.T) {
	a, svc := newFolderOrderApp(t)

	tmpDir, err := os.MkdirTemp("", pluginDownloadTempPrefix+"*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	dlPath := filepath.Join(tmpDir, "flow-output.png")
	// Minimal valid 1x1 PNG
	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(dlPath, pngBytes, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a.handlePluginDownloadDone(&mygo.Download{
		URL:  "https://flow.google.com/download/flow-output.png",
		Path: dlPath,
	})

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("expected temp download dir %q to be removed, stat err = %v", tmpDir, err)
	}
	items, err := svc.GetItems("", "desc")
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 || items[0].Name != "flow-output" {
		t.Fatalf("expected 1 imported item named flow-output, got %+v", items)
	}
}

func TestGoogleFlowPluginManifestLoads(t *testing.T) {
	localDir := t.TempDir()
	pluginDir := filepath.Join(localDir, "google-flow")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifest := `{
  "id": "official.google-flow",
  "name": "Google Flow",
  "version": "0.1.0",
  "kind": "action",
  "ui": {
    "windows": [
      {
        "id": "flow",
        "title": "打开 Google Flow",
        "url": "https://flow.google.com/",
        "interceptDownloads": true
      }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	host := plugin.NewHost(t.TempDir())
	host.SetLocalDirs(localDir)
	host.Scan()
	p, ok := host.Get("official.google-flow")
	if !ok {
		t.Fatal("expected official.google-flow plugin to be discovered")
	}
	if len(p.Manifest.UI.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(p.Manifest.UI.Windows))
	}
	win := p.Manifest.UI.Windows[0]
	if win.URL != "https://flow.google.com/" || !win.InterceptDownloads {
		t.Fatalf("unexpected google-flow window config: %+v", win)
	}
}
