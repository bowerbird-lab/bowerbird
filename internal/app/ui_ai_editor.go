package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bowerbird/core"
	"bowerbird/internal/plugin"

	"github.com/egoist/mygo/ui"
)

const (
	aiImageEditCapability = "ai-image-edit"
	maxInlineImageBytes   = 2 << 20 // 2 MB inline data URL threshold; larger files use /v1/uploads
)

type aiModelOption struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ModelID      string   `json:"modelId,omitempty"`
	Gems         int      `json:"gems,omitempty"`
	AspectRatios []string `json:"aspectRatios,omitempty"`
	Description  string   `json:"description,omitempty"`
}

type aiListModelsResponse struct {
	Models      []aiModelOption `json:"models"`
	GemsBalance *int            `json:"gemsBalance,omitempty"`
	HasAPIKey   bool            `json:"hasApiKey"`
	DefaultArch string          `json:"defaultArchitecture,omitempty"`
}

type aiUploadTicket struct {
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	URL       string            `json:"url"`
}

type aiRequestBilling struct {
	Mode         string `json:"mode"`
	GemsCharged  int    `json:"gems_charged"`
	GemsRefunded int    `json:"gems_refunded"`
}

type aiRequestResult struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Seed   int64  `json:"seed"`
}

type aiRequestError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

type aiRequestEnvelope struct {
	RequestID    string            `json:"request_id"`
	Status       string            `json:"status"`
	Architecture string            `json:"architecture"`
	ModelID      string            `json:"model_id"`
	Billing      *aiRequestBilling `json:"billing,omitempty"`
	Result       *aiRequestResult  `json:"result,omitempty"`
	Error        *aiRequestError   `json:"error,omitempty"`
	StatusURL    string            `json:"status_url"`
	CancelURL    string            `json:"cancel_url"`
}

type aiEditorState struct {
	pluginID string
	item     *core.Item

	models          []aiModelOption
	modelsLoaded    bool
	modelsLoading   bool
	hasAPIKey       bool
	gemsBalance     string
	selectedArch    string
	aspectRatio     string
	seedText        string
	prompt          string
	inspectorExpand bool

	running     bool
	runSeq      uint64
	cancelFn    context.CancelFunc
	statusText  string
	requestID   string
	statusURL   string
	cancelURL   string
	gemsCharged int
	errorText   string

	resultItemID string
	resultURL    string
	resultWidth  int
	resultHeight int
	resultSeed   int64
	resultBytes  []byte
	resultExt    string
	resultBmp    *ui.Bitmap
	resultPrompt string

	saving               bool
	confirmOverwriteOpen bool
	toastText            string
	toastUntil           time.Time
}

func defaultMageEditModels() []aiModelOption {
	return []aiModelOption{
		{ID: "flux2", Name: "Flux 2", ModelID: "flux2-dev", Gems: 40},
		{ID: "gpt_image_2", Name: "GPT Image 2", Gems: 40},
		{ID: "mango", Name: "Mango", Gems: 40},
		{ID: "nano_banana_v2", Name: "Nano Banana 2", Gems: 40},
		{ID: "seedream", Name: "Seedream", Gems: 40},
		{ID: "guava", Name: "Guava", Gems: 40},
	}
}

func (a *App) aiEditPlugin() *plugin.Plugin {
	if a.pluginHost == nil {
		return nil
	}
	return a.pluginHost.FindByCapability(aiImageEditCapability)
}

func (a *App) ensureAIEditorDefaults() {
	if len(a.aiEditor.models) == 0 {
		a.aiEditor.models = defaultMageEditModels()
	}
	if a.aiEditor.selectedArch == "" {
		a.aiEditor.selectedArch = a.aiEditor.models[0].ID
	}
	if a.aiEditor.aspectRatio == "" {
		a.aiEditor.aspectRatio = "auto"
	}
}

func (a *App) openAIEditorWindow(item *core.Item) {
	if item != nil && categoryOf(*item) == "image" {
		cp := *item
		a.aiEditor.item = &cp
	} else {
		a.syncAIEditorFromActive(false)
	}
	a.ensureAIEditorDefaults()
	a.refreshAIEditorModels(false)
	if a.svc != nil {
		_ = a.svc.OpenAIEditorWindow()
	}
}

func (a *App) syncAIEditorFromActive(force bool) {
	if a.activeItem == nil || categoryOf(*a.activeItem) != "image" {
		return
	}
	if force || a.aiEditor.item == nil || a.aiEditor.item.ID == a.activeItem.ID {
		cp := *a.activeItem
		a.aiEditor.item = &cp
	}
}

func (a *App) refreshAIEditorModels(force bool) {
	p := a.aiEditPlugin()
	if p == nil {
		return
	}
	a.ensureAIEditorDefaults()
	a.aiEditor.pluginID = p.Manifest.ID
	if a.aiEditor.modelsLoading || (a.aiEditor.modelsLoaded && !force) {
		return
	}
	a.aiEditor.modelsLoading = true
	pluginID := p.Manifest.ID
	go func() {
		var resp aiListModelsResponse
		err := a.pluginHost.CallAction(pluginID, "list-models", nil, &resp)
		a.postAIEditor(func() {
			a.aiEditor.modelsLoading = false
			if err != nil {
				return
			}
			a.aiEditor.modelsLoaded = true
			a.aiEditor.hasAPIKey = resp.HasAPIKey
			if len(resp.Models) > 0 {
				a.aiEditor.models = resp.Models
				found := false
				for _, m := range resp.Models {
					if m.ID == a.aiEditor.selectedArch {
						found = true
						break
					}
				}
				if !found {
					if resp.DefaultArch != "" {
						a.aiEditor.selectedArch = resp.DefaultArch
					} else {
						a.aiEditor.selectedArch = resp.Models[0].ID
					}
				}
			}
			if resp.GemsBalance != nil {
				a.aiEditor.gemsBalance = fmt.Sprintf("%d", *resp.GemsBalance)
			}
		})
	}()
}

func (a *App) aiHTTPClient() *http.Client {
	if a.svc != nil {
		settings := a.svc.GetNetworkSettings()
		return &http.Client{
			Timeout:   90 * time.Second,
			Transport: core.CreateProxyTransport(settings),
		}
	}
	return &http.Client{Timeout: 90 * time.Second}
}

// prepareImageForMage reads a local image file, converts non-JPEG/PNG formats
// to PNG, and either returns a data URL (<= 2MB) or uploads it through Mage's
// signed /v1/uploads ticket (> 2MB).
func (a *App) prepareImageForMage(ctx context.Context, pluginID, filePath string) (string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read source image: %w", err)
	}
	contentType := http.DetectContentType(raw)
	if contentType != "image/jpeg" && contentType != "image/png" {
		img, _, decErr := image.Decode(bytes.NewReader(raw))
		if decErr != nil {
			return "", fmt.Errorf("unsupported image format %s: %w", contentType, decErr)
		}
		var buf bytes.Buffer
		if encErr := png.Encode(&buf, img); encErr != nil {
			return "", fmt.Errorf("failed to encode PNG: %w", encErr)
		}
		raw = buf.Bytes()
		contentType = "image/png"
	}

	if len(raw) <= maxInlineImageBytes {
		return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
	}

	var ticket aiUploadTicket
	if err := a.pluginHost.CallAction(pluginID, "create-upload", map[string]any{
		"content_type": contentType,
	}, &ticket); err != nil {
		return "", fmt.Errorf("create upload ticket failed: %w", err)
	}
	if ticket.UploadURL == "" || ticket.URL == "" {
		return "", fmt.Errorf("invalid upload ticket returned by plugin")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, ticket.UploadURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	for k, v := range ticket.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := a.aiHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("upload image failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("upload returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return ticket.URL, nil
}

func (a *App) startAIEdit(target *core.Item) {
	p := a.aiEditPlugin()
	if p == nil {
		a.showAIEditorToast(a.t("aiEditor.pluginMissing"))
		return
	}
	if target == nil || target.FilePath == "" {
		a.showAIEditorToast(a.t("aiEditor.noImageSelected"))
		return
	}
	prompt := strings.TrimSpace(a.aiEditor.prompt)
	if prompt == "" {
		a.showAIEditorToast(a.t("aiEditor.promptRequired"))
		return
	}
	if a.aiEditor.running {
		return
	}

	a.ensureAIEditorDefaults()
	itemCopy := *target
	a.aiEditor.item = &itemCopy
	a.aiEditor.pluginID = p.Manifest.ID
	a.aiEditor.running = true
	a.aiEditor.runSeq++
	seq := a.aiEditor.runSeq
	a.aiEditor.errorText = ""
	a.aiEditor.statusText = a.t("aiEditor.statusPreparing")
	a.aiEditor.requestID = ""
	a.aiEditor.statusURL = ""
	a.aiEditor.cancelURL = ""
	a.aiEditor.gemsCharged = 0

	arch := a.aiEditor.selectedArch
	var modelID string
	for _, m := range a.aiEditor.models {
		if m.ID == arch {
			modelID = m.ModelID
			break
		}
	}
	aspect := a.aiEditor.aspectRatio
	seedText := strings.TrimSpace(a.aiEditor.seedText)

	ctx, cancel := context.WithCancel(context.Background())
	a.aiEditor.cancelFn = cancel

	go func() {
		defer cancel()
		setError := func(msg string) {
			a.postAIEditor(func() {
				if seq != a.aiEditor.runSeq {
					return
				}
				a.aiEditor.running = false
				a.aiEditor.cancelFn = nil
				a.aiEditor.statusText = ""
				a.aiEditor.errorText = msg
			})
		}

		// 1. Prepare image input (data URL or signed Mage upload).
		imageInput, err := a.prepareImageForMage(ctx, p.Manifest.ID, itemCopy.FilePath)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			setError(err.Error())
			return
		}

		a.postAIEditor(func() {
			if seq == a.aiEditor.runSeq {
				a.aiEditor.statusText = a.t("aiEditor.statusSubmitting")
			}
		})

		// 2. Submit generation request via plugin.
		params := map[string]any{
			"architecture": arch,
			"prompt":       prompt,
			"image":        imageInput,
		}
		if modelID != "" {
			params["model_id"] = modelID
		}
		if aspect != "" && aspect != "auto" {
			params["aspect_ratio"] = aspect
		}
		if seedText != "" {
			if seedVal, pErr := strconv.ParseInt(seedText, 10, 64); pErr == nil {
				params["seed"] = seedVal
			}
		}

		var env aiRequestEnvelope
		if err := a.pluginHost.CallAction(p.Manifest.ID, "submit-edit", params, &env); err != nil {
			if ctx.Err() != nil {
				return
			}
			setError(err.Error())
			return
		}

		a.postAIEditor(func() {
			if seq != a.aiEditor.runSeq {
				return
			}
			a.aiEditor.requestID = env.RequestID
			a.aiEditor.statusURL = env.StatusURL
			a.aiEditor.cancelURL = env.CancelURL
			if env.Billing != nil {
				a.aiEditor.gemsCharged = env.Billing.GemsCharged
			}
			a.aiEditor.statusText = a.formatAIStatus(env.Status)
		})

		// 3. Poll until terminal state.
		delay := 2 * time.Second
		deadline := time.Now().Add(5 * time.Minute)
		for env.Status != "completed" && env.Status != "failed" && env.Status != "cancelled" {
			if time.Now().After(deadline) {
				setError(a.t("aiEditor.timeout"))
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < 10*time.Second {
				delay = time.Duration(float64(delay) * 1.4)
				if delay > 10*time.Second {
					delay = 10 * time.Second
				}
			}

			var polled aiRequestEnvelope
			pollErr := a.pluginHost.CallAction(p.Manifest.ID, "poll-status", map[string]any{
				"request_id": env.RequestID,
				"status_url": env.StatusURL,
			}, &polled)
			if pollErr != nil {
				if ctx.Err() != nil {
					return
				}
				// Retry transient polling errors up to deadline; surface hard auth/404 errors immediately.
				low := strings.ToLower(pollErr.Error())
				if strings.Contains(low, "401") || strings.Contains(low, "403") || strings.Contains(low, "404") {
					setError(pollErr.Error())
					return
				}
				continue
			}
			env = polled
			a.postAIEditor(func() {
				if seq == a.aiEditor.runSeq {
					a.aiEditor.statusText = a.formatAIStatus(env.Status)
				}
			})
		}

		if env.Status == "cancelled" {
			a.postAIEditor(func() {
				if seq != a.aiEditor.runSeq {
					return
				}
				a.aiEditor.running = false
				a.aiEditor.cancelFn = nil
				a.aiEditor.statusText = a.t("aiEditor.statusCancelled")
			})
			return
		}
		if env.Status == "failed" {
			msg := a.t("aiEditor.statusFailed")
			if env.Error != nil && env.Error.Message != "" {
				msg = env.Error.Message
				if env.Error.Reason != "" {
					msg += " (" + env.Error.Reason + ")"
				}
			}
			setError(msg)
			return
		}
		if env.Result == nil || env.Result.URL == "" {
			setError(a.t("aiEditor.emptyResult"))
			return
		}

		// 4. Download generated image bytes.
		a.postAIEditor(func() {
			if seq == a.aiEditor.runSeq {
				a.aiEditor.statusText = a.t("aiEditor.statusDownloading")
			}
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.Result.URL, nil)
		if err != nil {
			setError(err.Error())
			return
		}
		resp, err := a.aiHTTPClient().Do(req)
		if err != nil {
			setError(fmt.Errorf("download result failed: %w", err).Error())
			return
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || len(data) == 0 {
			setError(fmt.Sprintf("download result failed (HTTP %d)", resp.StatusCode))
			return
		}

		ext := "png"
		switch http.DetectContentType(data) {
		case "image/jpeg":
			ext = "jpg"
		case "image/webp":
			ext = "webp"
		}
		bmp, _ := ui.DecodeBitmap(data)

		a.postAIEditor(func() {
			if seq != a.aiEditor.runSeq {
				return
			}
			a.aiEditor.running = false
			a.aiEditor.cancelFn = nil
			a.aiEditor.statusText = a.t("aiEditor.statusCompleted")
			a.aiEditor.resultItemID = itemCopy.ID
			a.aiEditor.resultURL = env.Result.URL
			a.aiEditor.resultWidth = env.Result.Width
			a.aiEditor.resultHeight = env.Result.Height
			a.aiEditor.resultSeed = env.Result.Seed
			a.aiEditor.resultBytes = data
			a.aiEditor.resultExt = ext
			a.aiEditor.resultBmp = bmp
			a.aiEditor.resultPrompt = prompt
			// Refresh balance after spending gems.
			a.refreshAIEditorModels(true)
		})
	}()
}

func (a *App) cancelAIEdit() {
	if !a.aiEditor.running {
		return
	}
	if a.aiEditor.cancelFn != nil {
		a.aiEditor.cancelFn()
		a.aiEditor.cancelFn = nil
	}
	pluginID := a.aiEditor.pluginID
	cancelURL := a.aiEditor.cancelURL
	requestID := a.aiEditor.requestID
	a.aiEditor.running = false
	a.aiEditor.statusText = a.t("aiEditor.statusCancelled")
	if pluginID != "" && (cancelURL != "" || requestID != "") && a.pluginHost != nil {
		go func() {
			var out any
			_ = a.pluginHost.CallAction(pluginID, "cancel-request", map[string]any{
				"request_id": requestID,
				"cancel_url": cancelURL,
			}, &out)
		}()
	}
}

func (a *App) formatAIStatus(status string) string {
	switch status {
	case "queued":
		return a.t("aiEditor.statusQueued")
	case "in_progress":
		return a.t("aiEditor.statusRunning")
	case "completed":
		return a.t("aiEditor.statusCompleted")
	case "failed":
		return a.t("aiEditor.statusFailed")
	case "cancelled":
		return a.t("aiEditor.statusCancelled")
	default:
		return status
	}
}

func (a *App) showAIEditorToast(msg string) {
	a.aiEditor.toastText = msg
	a.aiEditor.toastUntil = time.Now().Add(3 * time.Second)
	a.showToast(msg)
}

func (a *App) invalidateItemBitmaps(item *core.Item) {
	if item == nil {
		return
	}
	paths := []string{
		item.FilePath,
		filepath.Join(item.ItemPath, "thumbnail.jpg"),
		filepath.Join(item.ItemPath, "thumbnail.png"),
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		a.thumbs.invalidate(p)
		a.previewImages.invalidate(p)
		a.filmstripImages.invalidate(p)
	}
}

func (a *App) saveAIResultAsNew() {
	if a.svc == nil || len(a.aiEditor.resultBytes) == 0 || a.aiEditor.saving {
		return
	}
	a.aiEditor.saving = true
	data := append([]byte(nil), a.aiEditor.resultBytes...)
	ext := a.aiEditor.resultExt
	if ext == "" {
		ext = "png"
	}
	prompt := a.aiEditor.resultPrompt
	baseName := "ai-edited"
	var folderIDs []string
	var tags []string
	if a.aiEditor.item != nil {
		if a.aiEditor.item.Name != "" {
			baseName = a.aiEditor.item.Name + " (AI)"
		}
		folderIDs = append([]string(nil), a.aiEditor.item.Folders...)
		tags = append([]string(nil), a.aiEditor.item.Tags...)
	}
	filename := baseName + "." + ext

	go func() {
		created, err := a.svc.SaveNewItemFromBytes(data, filename, core.ImportItemOptions{
			Name:      baseName,
			Prompt:    prompt,
			FolderIDs: folderIDs,
			Tags:      tags,
		})
		a.postAIEditor(func() {
			a.aiEditor.saving = false
			if err != nil {
				a.showAIEditorToast(err.Error())
				return
			}
			a.showAIEditorToast(a.t("aiEditor.savedAsNew"))
			a.refreshAll()
			if created != nil {
				a.selected = map[string]bool{created.ID: true}
				a.selectedOrder = []string{created.ID}
				a.activeItemID = created.ID
				a.loadInspector(created.ID)
			}
		})
	}()
}

// requestOverwriteAIResult opens the mandatory confirmation modal before overwriting.
func (a *App) requestOverwriteAIResult(fromStandaloneWindow bool) {
	if len(a.aiEditor.resultBytes) == 0 || a.aiEditor.item == nil || a.aiEditor.saving {
		return
	}
	if fromStandaloneWindow {
		a.aiEditor.confirmOverwriteOpen = true
		return
	}
	a.dialog = dialogOverwriteItem
}

func (a *App) executeOverwriteAIResult() {
	a.aiEditor.confirmOverwriteOpen = false
	if a.dialog == dialogOverwriteItem {
		a.dialog = dialogNone
	}
	if a.svc == nil || a.aiEditor.item == nil || len(a.aiEditor.resultBytes) == 0 || a.aiEditor.saving {
		return
	}
	a.aiEditor.saving = true
	target := *a.aiEditor.item
	data := append([]byte(nil), a.aiEditor.resultBytes...)
	ext := a.aiEditor.resultExt
	prompt := a.aiEditor.resultPrompt
	pluginID := a.aiEditor.pluginID

	go func() {
		updated, err := a.svc.ReplaceItemWithBackup(target.ID, data, core.ReplaceItemOptions{
			Extension: ext,
			Prompt:    prompt,
			Source:    pluginID,
		})
		a.postAIEditor(func() {
			a.aiEditor.saving = false
			if err != nil {
				a.showAIEditorToast(err.Error())
				return
			}
			a.invalidateItemBitmaps(&target)
			if updated != nil {
				a.invalidateItemBitmaps(updated)
				cp := *updated
				a.aiEditor.item = &cp
				if a.activeItem != nil && a.activeItem.ID == updated.ID {
					a.activeItem = updated
					a.promptDraft = updated.Prompt
				}
			}
			a.showAIEditorToast(a.t("aiEditor.overwrittenWithBackup"))
			a.refreshAll()
		})
	}()
}

func (a *App) restoreItemVersion(item core.Item, versionID string) {
	if a.svc == nil || item.ID == "" || versionID == "" {
		return
	}
	go func() {
		updated, err := a.svc.RestoreItemVersion(item.ID, versionID)
		a.postAIEditor(func() {
			if err != nil {
				a.showAIEditorToast(err.Error())
				return
			}
			a.invalidateItemBitmaps(&item)
			if updated != nil {
				a.invalidateItemBitmaps(updated)
				if a.aiEditor.item != nil && a.aiEditor.item.ID == updated.ID {
					cp := *updated
					a.aiEditor.item = &cp
				}
				if a.activeItem != nil && a.activeItem.ID == updated.ID {
					a.activeItem = updated
					a.promptDraft = updated.Prompt
				}
			}
			a.showAIEditorToast(a.t("versions.restored"))
			a.refreshAll()
		})
	}()
}

func (a *App) deleteItemVersion(item core.Item, versionID string) {
	if a.svc == nil || item.ID == "" || versionID == "" {
		return
	}
	go func() {
		updated, err := a.svc.DeleteItemVersion(item.ID, versionID)
		a.postAIEditor(func() {
			if err != nil {
				a.showAIEditorToast(err.Error())
				return
			}
			if updated != nil {
				if a.aiEditor.item != nil && a.aiEditor.item.ID == updated.ID {
					cp := *updated
					a.aiEditor.item = &cp
				}
				if a.activeItem != nil && a.activeItem.ID == updated.ID {
					a.activeItem = updated
				}
			}
			a.showAIEditorToast(a.t("versions.deleted"))
			a.refreshAll()
		})
	}()
}

func (a *App) aiModelSelectOptions() []SelectOption {
	a.ensureAIEditorDefaults()
	opts := make([]SelectOption, 0, len(a.aiEditor.models))
	for _, m := range a.aiEditor.models {
		label := m.Name
		if m.Gems > 0 {
			label = fmt.Sprintf("%s (%d Gems)", m.Name, m.Gems)
		}
		opts = append(opts, SelectOption{Value: m.ID, Label: label})
	}
	return opts
}

func (a *App) aiAspectSelectOptions() []SelectOption {
	return []SelectOption{
		{Value: "auto", Label: a.t("aiEditor.aspectAuto")},
		{Value: "1:1", Label: "1:1"},
		{Value: "4:5", Label: "4:5"},
		{Value: "3:4", Label: "3:4"},
		{Value: "2:3", Label: "2:3"},
		{Value: "4:3", Label: "4:3"},
		{Value: "3:2", Label: "3:2"},
		{Value: "16:9", Label: "16:9"},
		{Value: "9:16", Label: "9:16"},
	}
}

// viewOverwriteConfirmDialog renders the main-window overwrite confirmation modal.
func (a *App) viewOverwriteConfirmDialog(c *ui.Context, open *bool) {
	p := a.palette(c)
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.overlay)
		a.dialogPanelActions(c, panel, a.t("aiEditor.overwriteConfirmTitle"), func() {
			ui.Text(c, a.t("aiEditor.overwriteConfirmMessage")).
				FontSize(12.5).TextColor(p.muted).Wrap()
			if a.aiEditor.item != nil {
				ui.Text(c, a.aiEditor.item.Filename).
					FontSize(11.5).FontWeight(600).TextColor(p.text).SingleLine()
			}
		}, func() {
			if ui.Button(c, a.t("common.cancel")).Clicked() {
				a.dialog = dialogNone
			}
			if ui.PrimaryButton(c, a.t("aiEditor.overwriteConfirmAction")).Clicked() {
				a.executeOverwriteAIResult()
			}
		})
	})
}

// viewInspectorAIEdit renders the inline AI Image Edit plugin section directly
// inside the right-hand Inspector when an image item is selected.
func (a *App) viewInspectorAIEdit(c *ui.Context, item core.Item) {
	if categoryOf(item) != "image" {
		return
	}
	pl := a.aiEditPlugin()
	if pl == nil {
		return
	}
	a.ensureAIEditorDefaults()
	if !a.aiEditor.modelsLoaded && !a.aiEditor.modelsLoading {
		a.refreshAIEditorModels(false)
	}
	p := a.palette(c)
	a.inspectorDivider(c)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, iconSparkles).Size(13, 13).TextColor(p.accent)
		ui.Text(c, pl.Manifest.Name).FontSize(11).FontWeight(650).TextColor(p.text).Grow(1).Shrink(1).SingleLine()
		popBtn := ui.ButtonBase(c.Key("ai-inspector-popout")).Height(22).PaddingX(7).Radius(5).FocusRing(false).AlignItems(ui.Center).Gap(4)
		if popBtn.Hovered() {
			popBtn.Background(p.hover)
		}
		popBtn.Tooltip(a.t("aiEditor.openWindow"))
		popBtn.Children(func() {
			ui.Icon(c, iconExternal).Size(11, 11).TextColor(p.muted)
			ui.Text(c, a.t("aiEditor.openWindowShort")).FontSize(10.5).TextColor(p.muted)
		})
		if popBtn.Clicked() {
			a.openAIEditorWindow(&item)
		}
	})

	// Model picker
	arch, changed := a.dropdown(c, "inspector-ai-model", a.aiEditor.selectedArch, a.aiModelSelectOptions())
	if changed {
		a.aiEditor.selectedArch = arch
	}

	// Prompt input
	ui.TextArea(c.Key("inspector-ai-prompt"), &a.aiEditor.prompt).
		FontSize(11.5).Height(68).Placeholder(a.t("aiEditor.promptPlaceholder"))

	// Action buttons
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		if a.aiEditor.running {
			if ui.Button(c.Key("inspector-ai-cancel"), a.t("common.cancel")).Clicked() {
				a.cancelAIEdit()
			}
			ui.Text(c, a.aiEditor.statusText).FontSize(10.5).TextColor(p.muted).Grow(1).Shrink(1).SingleLine()
		} else {
			if ui.PrimaryButton(c.Key("inspector-ai-run"), a.t("aiEditor.runEdit")).Clicked() {
				a.startAIEdit(&item)
			}
		}
	})

	if a.aiEditor.errorText != "" {
		ui.Text(c, a.aiEditor.errorText).FontSize(10.5).TextColor(p.danger).Wrap().MaxLines(4)
	}

	// Inline result preview + Save as New / Overwrite buttons
	if a.aiEditor.resultItemID == item.ID && len(a.aiEditor.resultBytes) > 0 {
		ui.Box(c).FillWidth().Height(150).Radius(8).Background(p.surface2).Border(1, p.hairline).Clip().Children(func() {
			if a.aiEditor.resultBmp != nil {
				ui.Image(c, a.aiEditor.resultBmp).Fill().Fit(ui.Contain)
			}
		})
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if ui.Button(c.Key("inspector-ai-save-new"), a.t("aiEditor.saveAsNew")).Clicked() {
				a.saveAIResultAsNew()
			}
			if ui.PrimaryButton(c.Key("inspector-ai-overwrite"), a.t("aiEditor.overwrite")).Clicked() {
				a.aiEditor.item = &item
				a.requestOverwriteAIResult(false)
			}
		})
	}
}

// viewInspectorVersions renders historical backups recorded in items/<id>/metadata.json.
func (a *App) viewInspectorVersions(c *ui.Context, item core.Item) {
	if len(item.Versions) == 0 {
		return
	}
	p := a.palette(c)
	a.inspectorDivider(c)
	a.sectionLabel(c, a.tf("versions.titleCount", map[string]any{"count": len(item.Versions)}))
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for _, v := range item.Versions {
			v := v
			backupPath := filepath.Join(item.ItemPath, v.Filename)
			card := ui.Row(c.Key("ver-"+v.ID)).FillWidth().Padding(6).Radius(8).Background(p.surface2).Border(1, p.hairline).Gap(8).AlignItems(ui.Center)
			card.Children(func() {
				thumb := ui.Box(c).Size(44, 44).Radius(6).Background(p.bg).Clip().Shrink(0)
				if bmp := a.thumbs.get(backupPath, 96); bmp != nil {
					thumb.Children(func() {
						ui.Image(c, bmp).Fill().Fit(ui.Cover)
					})
				}
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, formatDate(v.CreatedAt)).FontSize(10.5).FontWeight(600).TextColor(p.text).SingleLine()
					dim := formatBytes(v.Size)
					if v.Width > 0 && v.Height > 0 {
						dim = fmt.Sprintf("%dx%d · %s", v.Width, v.Height, dim)
					}
					ui.Text(c, dim).FontSize(10).TextColor(p.faint).SingleLine()
					if v.Prompt != "" {
						ui.Text(c, v.Prompt).FontSize(10).TextColor(p.muted).SingleLine()
					}
				})
				ui.Row(c).Gap(4).Shrink(0).Children(func() {
					a.iconButton(c, "ver-restore-"+v.ID, iconRestore, false, a.t("versions.restore"), func() {
						a.restoreItemVersion(item, v.ID)
					})
					a.iconButton(c, "ver-delete-"+v.ID, iconTrash, false, a.t("versions.delete"), func() {
						a.deleteItemVersion(item, v.ID)
					})
				})
			})
		}
	})
}

// viewAIEditorWindow renders the standalone Mage AI Image Studio window.
func (a *App) viewAIEditorWindow(c *ui.Context) {
	a.ensureAIEditorDefaults()
	p := a.palette(c)
	pl := a.aiEditPlugin()

	root := ui.Box(c).Fill().Background(p.bg)
	root.Children(func() {
		ui.Column(c).Fill().Children(func() {
			// Titlebar
			bar := c.TitleBar()
			topH := maxF(44, bar.Height)
			ui.Row(c).FillWidth().Height(topH).Padding(0, 16, 0, maxF(16, bar.Left+12)).AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
				ui.Icon(c, iconSparkles).Size(15, 15).TextColor(p.accent)
				title := a.t("aiEditor.windowTitle")
				if pl != nil {
					title = pl.Manifest.Name
				}
				ui.Text(c, title).FontSize(13.5).FontWeight(650).TextColor(p.text)
				ui.Box(c).Grow(1)
				if a.aiEditor.gemsBalance != "" {
					ui.Badge(c, a.aiEditor.gemsBalance+" Gems").FontSize(10.5)
				}
				if a.activeItem != nil && categoryOf(*a.activeItem) == "image" && (a.aiEditor.item == nil || a.aiEditor.item.ID != a.activeItem.ID) {
					if ui.Button(c.Key("ai-win-sync"), a.t("aiEditor.useSelectedImage")).Clicked() {
						a.syncAIEditorFromActive(true)
					}
				}
				if ui.Button(c.Key("ai-win-settings"), a.t("aiEditor.apiSettings")).Clicked() {
					if a.svc != nil {
						_ = a.svc.OpenSettingsWindow()
					}
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(p.hairline)

			if pl == nil {
				ui.Column(c).Fill().Center().Gap(10).Children(func() {
					ui.Icon(c, iconSparkles).Size(32, 32).TextColor(p.faint)
					ui.Text(c, a.t("aiEditor.pluginMissing")).FontSize(14).FontWeight(600).TextColor(p.text)
					ui.Text(c, a.t("aiEditor.pluginMissingHint")).FontSize(12).TextColor(p.muted)
				})
				return
			}

			// Main 2-column workspace
			ui.Row(c).FillWidth().Grow(1).Shrink(1).MinHeight(0).Children(func() {
				// Left column: Visual Canvas (Source + Result + Version History)
				ui.Scroll(c).Grow(1).Shrink(1).MinWidth(0).Padding(16).Children(func() {
					ui.Column(c).FillWidth().Gap(14).Children(func() {
						if a.aiEditor.item == nil {
							ui.Column(c).FillWidth().Height(360).Center().Radius(12).Background(p.surface2).Border(1, p.hairline).Gap(8).Children(func() {
								ui.Icon(c, iconImage).Size(32, 32).TextColor(p.faint)
								ui.Text(c, a.t("aiEditor.noImageSelected")).FontSize(13).FontWeight(600).TextColor(p.text)
								ui.Text(c, a.t("aiEditor.noImageHint")).FontSize(11.5).TextColor(p.muted)
							})
						} else {
							item := *a.aiEditor.item
							// Side-by-side Original vs AI Result
							ui.Row(c).FillWidth().Gap(12).Children(func() {
								// Original card
								ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
									ui.Text(c, a.t("aiEditor.originalImage")).FontSize(11).FontWeight(650).TextColor(p.faint)
									box := ui.Box(c).FillWidth().Height(320).Radius(10).Background(p.surface2).Border(1, p.hairline).Clip()
									if bmp := a.thumbs.get(item.FilePath, 768); bmp != nil {
										box.Children(func() {
											ui.Image(c, bmp).Fill().Fit(ui.Contain)
										})
									}
									ui.Text(c, fmt.Sprintf("%s · %dx%d · %s", item.Name, item.Width, item.Height, formatBytes(item.Size))).
										FontSize(10.5).TextColor(p.faint).SingleLine()
								})

								// Result card
								ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
									ui.Text(c, a.t("aiEditor.resultImage")).FontSize(11).FontWeight(650).TextColor(p.faint)
									box := ui.Box(c).FillWidth().Height(320).Radius(10).Background(p.surface2).Border(1, p.hairline).Clip()
									hasResult := a.aiEditor.resultItemID == item.ID && len(a.aiEditor.resultBytes) > 0
									box.Children(func() {
										if hasResult && a.aiEditor.resultBmp != nil {
											ui.Image(c, a.aiEditor.resultBmp).Fill().Fit(ui.Contain)
										} else if a.aiEditor.running {
											ui.Column(c).Fill().Center().Gap(6).Children(func() {
												ui.Icon(c, iconSparkles).Size(24, 24).TextColor(p.accent)
												ui.Text(c, a.aiEditor.statusText).FontSize(12).TextColor(p.text)
											})
										} else {
											ui.Column(c).Fill().Center().Gap(6).Children(func() {
												ui.Icon(c, iconImage).Size(24, 24).TextColor(p.faint)
												ui.Text(c, a.t("aiEditor.resultPlaceholder")).FontSize(11.5).TextColor(p.faint)
											})
										}
									})
									if hasResult {
										info := fmt.Sprintf("%dx%d · %s", a.aiEditor.resultWidth, a.aiEditor.resultHeight, formatBytes(int64(len(a.aiEditor.resultBytes))))
										if a.aiEditor.resultSeed != 0 {
											info += fmt.Sprintf(" · Seed %d", a.aiEditor.resultSeed)
										}
										ui.Text(c, info).FontSize(10.5).TextColor(p.faint).SingleLine()
									}
								})
							})

							// Save actions when result is available
							if a.aiEditor.resultItemID == item.ID && len(a.aiEditor.resultBytes) > 0 {
								ui.Row(c).FillWidth().Padding(12).Radius(10).Background(p.surface2).Border(1, p.hairline).AlignItems(ui.Center).Gap(8).Children(func() {
									ui.Text(c, a.t("aiEditor.saveResultHint")).FontSize(11.5).TextColor(p.muted).Grow(1).Shrink(1)
									if ui.Button(c.Key("ai-win-save-new"), a.t("aiEditor.saveAsNew")).Clicked() {
										a.saveAIResultAsNew()
									}
									if ui.PrimaryButton(c.Key("ai-win-overwrite"), a.t("aiEditor.overwrite")).Clicked() {
										a.requestOverwriteAIResult(true)
									}
								})
							}

							// Historical versions section inside standalone window
							if len(item.Versions) > 0 {
								a.viewInspectorVersions(c, item)
							}
						}
					})
				})

				ui.Box(c).Width(1).FillHeight().Background(p.hairline)

				// Right column: Parameters & Run Controls
				ui.Scroll(c).Width(310).FillHeight().Background(p.sidebar).Padding(16).Children(func() {
					ui.Column(c).FillWidth().Gap(12).Children(func() {
						a.sectionLabel(c, a.t("aiEditor.model"))
						arch, changed := a.dropdown(c, "win-ai-model", a.aiEditor.selectedArch, a.aiModelSelectOptions())
						if changed {
							a.aiEditor.selectedArch = arch
						}

						a.sectionLabel(c, a.t("aiEditor.aspectRatio"))
						ar, arChanged := a.dropdown(c, "win-ai-aspect", a.aiEditor.aspectRatio, a.aiAspectSelectOptions())
						if arChanged {
							a.aiEditor.aspectRatio = ar
						}

						a.sectionLabel(c, a.t("aiEditor.seed"))
						ui.TextInput(c.Key("win-ai-seed"), &a.aiEditor.seedText).
							FontSize(12).Placeholder(a.t("aiEditor.seedPlaceholder"))

						a.sectionLabel(c, a.t("aiEditor.prompt"))
						ui.TextArea(c.Key("win-ai-prompt"), &a.aiEditor.prompt).
							FontSize(12).Height(130).Placeholder(a.t("aiEditor.promptPlaceholder"))

						if a.aiEditor.running {
							ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
								if ui.Button(c.Key("win-ai-cancel"), a.t("common.cancel")).Clicked() {
									a.cancelAIEdit()
								}
								ui.Text(c, a.aiEditor.statusText).FontSize(11.5).TextColor(p.muted).Grow(1).Shrink(1).SingleLine()
							})
						} else {
							if ui.PrimaryButton(c.Key("win-ai-run"), a.t("aiEditor.runEdit")).FillWidth().Clicked() {
								a.startAIEdit(a.aiEditor.item)
							}
						}

						if a.aiEditor.gemsCharged > 0 {
							ui.Text(c, a.tf("aiEditor.gemsCharged", map[string]any{"gems": a.aiEditor.gemsCharged})).
								FontSize(11).TextColor(p.faint)
						}
						if a.aiEditor.errorText != "" {
							ui.Box(c).FillWidth().Padding(10).Radius(8).Background(p.surface2).Border(1, p.danger).Children(func() {
								ui.Text(c, a.aiEditor.errorText).FontSize(11.5).TextColor(p.danger).Wrap()
							})
						}
					})
				})
			})
		})

		// Standalone window toast
		if a.aiEditor.toastText != "" && time.Now().Before(a.aiEditor.toastUntil) {
			ui.Box(c).Absolute().Bottom(18).Left(0).Right(0).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Padding(8, 14).Radius(999).Background(p.surface).Border(1, p.hairline).Shadow(0, 8, 24, -6, p.cardShade).Children(func() {
					ui.Text(c, a.aiEditor.toastText).FontSize(12).TextColor(p.text)
				})
			})
		}

		// Standalone window overwrite confirmation modal
		if a.aiEditor.confirmOverwriteOpen {
			open := true
			ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
				backdrop.Background(p.overlay)
				a.dialogPanelActions(c, panel, a.t("aiEditor.overwriteConfirmTitle"), func() {
					ui.Text(c, a.t("aiEditor.overwriteConfirmMessage")).
						FontSize(12.5).TextColor(p.muted).Wrap()
					if a.aiEditor.item != nil {
						ui.Text(c, a.aiEditor.item.Filename).
							FontSize(11.5).FontWeight(600).TextColor(p.text).SingleLine()
					}
				}, func() {
					if ui.Button(c, a.t("common.cancel")).Clicked() {
						a.aiEditor.confirmOverwriteOpen = false
					}
					if ui.PrimaryButton(c, a.t("aiEditor.overwriteConfirmAction")).Clicked() {
						a.executeOverwriteAIResult()
					}
				})
			})
			if !open {
				a.aiEditor.confirmOverwriteOpen = false
			}
		}
	})
}
