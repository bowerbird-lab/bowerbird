package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindByCapabilityAndCallAction(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "official.mage-ai-image")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := `{
  "id": "official.mage-ai-image",
  "name": "Mage AI Image",
  "version": "1.0.0",
  "kind": "action",
  "main": "index.js",
  "hooks": ["actions", "runAction"],
  "capabilities": ["ai-image-edit", "standalone-window", "inspector-panel"],
  "permissions": { "network": ["api.mage.space"] },
  "settings": [{ "key": "apiKey", "label": "API Key", "secret": true }]
}`
	script := `
export function actions(ctx) {
  return [{ id: "open-editor", title: "Mage AI Edit", icon: "sparkles" }];
}
export async function runAction(input) {
  if (input.id === "list-models") {
    return {
      hasApiKey: Boolean(host.settings.get("apiKey")),
      models: [{ id: "flux2", name: "Flux 2", gems: 40 }]
    };
  }
  return { echoed: input.prompt || "" };
}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(script), 0o644); err != nil {
		t.Fatalf("write index.js: %v", err)
	}

	host := NewHost(dir)
	found := host.FindByCapability("ai-image-edit")
	if found == nil || found.Manifest.ID != "official.mage-ai-image" {
		t.Fatalf("FindByCapability = %+v, want official.mage-ai-image", found)
	}

	var listResp struct {
		HasAPIKey bool `json:"hasApiKey"`
		Models    []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Gems int    `json:"gems"`
		} `json:"models"`
	}
	if err := host.CallAction("official.mage-ai-image", "list-models", nil, &listResp); err != nil {
		t.Fatalf("CallAction(list-models): %v", err)
	}
	if listResp.HasAPIKey {
		t.Fatalf("expected hasApiKey=false before setting apiKey")
	}
	if len(listResp.Models) != 1 || listResp.Models[0].ID != "flux2" {
		t.Fatalf("models = %+v, want flux2", listResp.Models)
	}

	host.SetSetting("official.mage-ai-image", "apiKey", "mage_sk_test")
	if err := host.CallAction("official.mage-ai-image", "list-models", nil, &listResp); err != nil {
		t.Fatalf("CallAction(list-models with key): %v", err)
	}
	if !listResp.HasAPIKey {
		t.Fatalf("expected hasApiKey=true after SetSetting")
	}

	var echoResp struct {
		Echoed string `json:"echoed"`
	}
	if err := host.CallAction("official.mage-ai-image", "echo", map[string]any{"prompt": "cyberpunk"}, &echoResp); err != nil {
		t.Fatalf("CallAction(echo): %v", err)
	}
	if echoResp.Echoed != "cyberpunk" {
		t.Fatalf("echoed = %q, want cyberpunk", echoResp.Echoed)
	}
}
