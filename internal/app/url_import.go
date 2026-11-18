package app

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"bowerbird/core"
	"bowerbird/internal/plugin"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// attachURLDropTarget makes a native view accept URLs and files dragged from a
// browser, Finder/Explorer, or another application. Files are routed through
// the normal native-drop path; URLs are imported through the resolver chain.
func (a *App) attachURLDropTarget(el ui.Element) {
	opts := transfer.DropOptions{
		// FileList matters: browsers and Explorer advertise CF_HDROP as a file
		// list. Without it the same drag degrades into its URI list, which is how
		// a local image used to be mistaken for a URL.
		Formats:    []transfer.Format{transfer.FileList, transfer.URIList, transfer.Text, transfer.HTML},
		Operations: transfer.Copy,
	}
	if _, over := ui.DataDragOver(el, opts); over {
		// Keep the drop target unobtrusive; the toast reports the result.
	}
	if drop, ok := ui.DropData(el, opts); ok {
		folder := a.takeDropFolder()
		a.handleDropDataToFolder(drop.Data, folder)
	}
}

func (a *App) handleDropDataToFolder(data transfer.Data, folder string) {
	// Finder/Explorer drags also advertise a URI list. Route their file
	// URLs through the normal native-drop path instead of swallowing them.
	if files, err := data.Files(); err == nil && len(files) > 0 {
		a.handleDroppedFiles(files, folder)
		return
	}
	urls, referer := a.dropURLsFromData(data)
	if len(urls) > 0 {
		a.handleURLDropsToFolder(urls, referer, folder)
	}
}

// takeDropFolder captures the destination before an asynchronous import starts.
// A hovered folder takes precedence over the folder currently being viewed.
func (a *App) takeDropFolder() string {
	folder := a.dropFolderID
	a.dropFolderID = ""
	if folder == "" && a.nav == NavFolder {
		folder = a.folderID
	}
	return folder
}

func (a *App) dropURLsFromData(data transfer.Data) ([]string, string) {
	var canResolve func(string) bool
	if a.pluginHost != nil {
		canResolve = a.pluginHost.CanResolve
	}
	return urlsFromDropDataWithResolver(data, canResolve)
}

// urlsFromDropData extracts import URLs from a browser drag, preferring
// URIList/Text and only falling back to HTML <img> extraction when no plugin
// claims the URL and no direct media URL is already present.
func urlsFromDropData(data transfer.Data) ([]string, string) {
	return urlsFromDropDataWithResolver(data, nil)
}

func urlsFromDropDataWithResolver(data transfer.Data, canResolve func(string) bool) ([]string, string) {
	var primaryRaw []string
	if urls, err := data.URLs(); err == nil {
		primaryRaw = append(primaryRaw, urls...)
	}
	if text, err := data.Read(transfer.Text); err == nil {
		primaryRaw = append(primaryRaw, strings.Split(string(text), "\n")...)
	}
	primary := normalizeDroppedURLs(primaryRaw)

	hasPluginMatch := false
	hasMediaURL := false
	if canResolve != nil {
		for _, u := range primary {
			if canResolve(u) {
				hasPluginMatch = true
				break
			}
		}
	}
	for _, u := range primary {
		if looksLikeMediaURL(u) {
			hasMediaURL = true
			break
		}
	}

	referer := ""
	var urls []string

	// Only fall back to scraping <img> tags from the drag's HTML fragment when
	// no plugin claims the dropped URL(s) and the drop didn't already carry a
	// direct media URL. Otherwise a site with a resolver plugin (e.g.
	// Pinterest) would import both the resolved original and the HTML thumbnail.
	if len(primary) == 0 || (!hasPluginMatch && !hasMediaURL) {
		if markup, err := data.Read(transfer.HTML); err == nil && len(markup) > 0 {
			media, page := core.ExtractMediaFromHTML("", markup)
			if page != "" {
				referer = page
			}
			if page != "" && canResolve != nil && canResolve(page) {
				urls = normalizeDroppedURLs([]string{page})
			} else if htmlMedia := normalizeDroppedURLs(media); len(htmlMedia) > 0 {
				urls = htmlMedia
			}
		}
	} else if markup, err := data.Read(transfer.HTML); err == nil && len(markup) > 0 {
		// Still recover the source page URL as Referer for hotlink-protected media.
		_, page := core.ExtractMediaFromHTML("", markup)
		if page != "" {
			referer = page
		}
	}

	if len(urls) == 0 {
		urls = primary
	}

	if referer == "" {
		// A page URL next to a media URL is the image's source page, which is
		// exactly what hosts with hotlink protection expect in Referer.
		for _, candidate := range urls {
			if !looksLikeMediaURL(candidate) {
				referer = candidate
				break
			}
		}
	}
	return urls, referer
}

func looksLikeMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg", ".mp4", ".webm", ".mov", ".m4v", ".pdf"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func normalizeDroppedURLs(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, candidate := range raw {
		for _, line := range strings.Split(candidate, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "\ufeff"))
			if line == "" {
				continue
			}
			// Browser text drags sometimes prefix the link with its title.
			lowerLine := strings.ToLower(line)
			if idx := strings.Index(lowerLine, "https://"); idx > 0 {
				line = line[idx:]
			} else if idx := strings.Index(lowerLine, "http://"); idx > 0 {
				line = line[idx:]
			}
			line = strings.TrimRight(line, ".,;:!?)]}>\"'")
			if !strings.Contains(line, "://") && strings.Contains(line, ".") {
				line = "https://" + line
			}
			u, err := url.Parse(line)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				continue
			}
			key := u.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

func splitDroppedPaths(paths []string) (urls, files []string) {
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Keep URL detection explicit so filesystem paths such as /tmp/a.jpg
		// never become URLs.
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			urls = append(urls, trimmed)
			continue
		}
		files = append(files, raw)
	}
	return urls, files
}

// handleURLDrops resolves every dropped URL through the plugin index. The first
// matching resolver runs and returns media candidates; only if no plugin claims
// the URL does the host try the URL itself as a direct media download.
func (a *App) handleURLDropsToFolder(rawURLs []string, referer string, folderID string) {
	if a.svc == nil {
		return
	}
	urls := normalizeDroppedURLs(rawURLs)
	if len(urls) == 0 {
		return
	}
	core.RecordLog("info", "url", "url drop received", fmt.Sprintf("count=%d referer=%s urls=%v", len(urls), referer, urls), 0)
	a.showToast(a.t("dropzone.urlImporting"))
	go func() {
		imported := 0
		var firstErr error
		for _, raw := range urls {
			items, err := a.importURLItems(raw, referer, folderID)
			if err != nil {
				core.RecordLog("error", "url", "url import failed", fmt.Sprintf("url=%s err=%v", raw, err), 0)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, item := range items {
				if item != nil {
					core.RecordLog("info", "url", "url import succeeded", fmt.Sprintf("url=%s name=%s", raw, item.Name), 0)
					imported++
				}
			}
		}
		a.post(func() {
			if firstErr != nil {
				a.showToast(firstErr.Error())
			} else if imported > 0 {
				a.showToast(a.t("dropzone.urlImported"))
			}
			if imported > 0 {
				a.refreshAll()
			}
		})
	}()
}

// handleDroppedFiles imports files (or folders) dropped on a window, routing
// them through the native drop service so self-drops stay no-ops.
func (a *App) handleDroppedFiles(paths []string, folder string) {
	if a.svc == nil || len(paths) == 0 {
		return
	}
	go func() {
		var (
			result core.NativeDropResult
			err    error
		)
		if folder != "" {
			result, err = a.svc.HandleNativeDropWithResult(paths, "subfolder", folder)
		} else {
			result, err = a.svc.HandleNativeDropWithResult(paths, "", "")
		}
		a.post(func() {
			if err != nil {
				a.showToast(err.Error())
				a.refreshAll()
				return
			}
			if result.Importing {
				a.showToast(a.tf("dropzone.importing", map[string]any{"count": result.External}))
			}
			if result.Importing || result.Internal > 0 {
				a.refreshAll()
			}
		})
	}()
}

func (a *App) importURL(raw string, referer string, folderID string) (*core.Item, error) {
	items, err := a.importURLItems(raw, referer, folderID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("URL import failed")
	}
	return items[0], nil
}

func (a *App) importURLItems(raw string, referer string, folderID string) ([]*core.Item, error) {
	var resolverID string
	var resolution *plugin.ResolveResult
	if a.pluginHost != nil {
		res, id, err := a.pluginHost.Resolve(raw)
		if err == nil && res != nil {
			resolution, resolverID = res, id
		} else if err != nil && !errors.Is(err, plugin.ErrNoURLResolver) {
			if id != "" {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			return nil, err
		}
	}

	baseOpts := core.ImportItemOptions{SkipURL: !a.prefs.RecordImportURL}
	if a.prefs.RecordImportURL {
		baseOpts.URL = raw
	}
	if referer != "" {
		baseOpts.Headers = map[string]string{"Referer": referer}
	}
	if resolverID != "" {
		if p, ok := a.pluginHost.Get(resolverID); ok && p.Settings != nil {
			if cookie := strings.TrimSpace(p.Settings["cookie"]); cookie != "" {
				if baseOpts.Headers == nil {
					baseOpts.Headers = map[string]string{}
				}
				baseOpts.Headers["Cookie"] = cookie
			}
		}
	}

	targetFolderID := strings.TrimSpace(folderID)
	if resolution != nil && len(resolution.Items) > 0 {
		if folderName := strings.TrimSpace(resolution.FolderName); folderName != "" {
			if created, err := a.svc.CreateFolder(folderName, targetFolderID); err == nil && created != nil {
				targetFolderID = created.ID
			}
		}
		var imported []*core.Item
		var firstErr error
		for _, entry := range resolution.Items {
			opts := baseOpts
			if targetFolderID != "" {
				opts.FolderIDs = []string{targetFolderID}
			}
			if a.prefs.RecordImportURL {
				opts.URL = firstNonEmpty(entry.SourceURL, resolution.SourceURL, raw)
			} else {
				opts.URL = ""
				opts.SkipURL = true
			}
			opts.Name = firstNonEmpty(entry.Name, resolution.Name)
			if len(entry.Tags) > 0 {
				opts.Tags = entry.Tags
			} else {
				opts.Tags = resolution.Tags
			}
			candidates := uniqueURLs(append([]string{entry.URL}, entry.Candidates...))
			if len(candidates) == 0 {
				continue
			}
			item, err := a.importFirstCandidate(candidates, opts)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if item != nil {
				imported = append(imported, item)
			}
		}
		if len(imported) == 0 {
			if firstErr != nil {
				return nil, firstErr
			}
			return nil, errors.New("URL import failed")
		}
		return imported, nil
	}

	opts := baseOpts
	if targetFolderID != "" {
		opts.FolderIDs = []string{targetFolderID}
	}
	candidates := []string{raw}
	if resolution != nil {
		if a.prefs.RecordImportURL {
			opts.URL = firstNonEmpty(resolution.SourceURL, raw)
		} else {
			opts.URL = ""
			opts.SkipURL = true
		}
		opts.Name = resolution.Name
		opts.Tags = resolution.Tags
		candidates = uniqueURLs(append([]string{resolution.URL}, resolution.Candidates...))
	}
	if len(candidates) == 0 {
		return nil, errors.New("plugin returned no media URL")
	}
	item, err := a.importFirstCandidate(candidates, opts)
	if err != nil {
		return nil, err
	}
	return []*core.Item{item}, nil
}

func (a *App) importFirstCandidate(candidates []string, opts core.ImportItemOptions) (*core.Item, error) {
	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		item, err := a.svc.ImportFromURLWithOptions(candidate, opts)
		if err == nil && item != nil {
			return item, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("URL import failed")
}

func uniqueURLs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
