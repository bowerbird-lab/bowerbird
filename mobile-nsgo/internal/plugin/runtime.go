package plugin

import (
	"fmt"

	"github.com/dop251/goja"
)

// PreviewItemContext is passed to a plugin's onPreview(item, host) JS hook.
type PreviewItemContext struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	MimeType string `json:"mimeType"`
	URL      string `json:"url"`
}

// UINode is a cross-platform declarative UI node returned by a plugin.
type UINode struct {
	Type     string   `json:"type"`
	Text     string   `json:"text,omitempty"`
	Source   string   `json:"source,omitempty"`
	Controls bool     `json:"controls,omitempty"`
	Action   string   `json:"action,omitempty"`
	Children []UINode `json:"children,omitempty"`
}

// PreviewPlan is the routing decision returned by a plugin's JS hook.
type PreviewPlan struct {
	Renderer string  `json:"renderer"`
	Source   string  `json:"source"`
	Fallback string  `json:"fallback"`
	UI       *UINode `json:"ui,omitempty"`
}

// HostCapabilities lists the native rendering primitives available on mobile.
var HostCapabilities = []string{
	"video.native",
	"video.external",
	"audio.native",
	"image.native",
	"markdown.native",
}

// ExecutePreviewHook runs a plugin's JS onPreview(item, host) function in goja.
func ExecutePreviewHook(script string, item PreviewItemContext) (PreviewPlan, error) {
	vm := goja.New()
	if _, err := vm.RunString(script); err != nil {
		return PreviewPlan{}, fmt.Errorf("plugin script error: %w", err)
	}
	fn, ok := goja.AssertFunction(vm.Get("onPreview"))
	if !ok {
		return PreviewPlan{}, fmt.Errorf("plugin missing onPreview(item, host)")
	}
	itemObj := map[string]any{
		"id":       item.ID,
		"filename": item.Filename,
		"ext":      item.Ext,
		"mimeType": item.MimeType,
		"url":      item.URL,
	}
	hostObj := map[string]any{
		"platform":     "mobile",
		"capabilities": HostCapabilities,
	}
	res, err := fn(goja.Undefined(), vm.ToValue(itemObj), vm.ToValue(hostObj))
	if err != nil {
		return PreviewPlan{}, fmt.Errorf("onPreview failed: %w", err)
	}
	var plan PreviewPlan
	if err := vm.ExportTo(res, &plan); err != nil {
		return PreviewPlan{}, fmt.Errorf("invalid PreviewPlan: %w", err)
	}
	return plan, nil
}
