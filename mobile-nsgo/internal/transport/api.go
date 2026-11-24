package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
)

// List returns one page for a filter. It prefers /query and falls back to the
// legacy /items endpoint only when the selected filters are fully expressible.
// It never silently drops a filter and reports a broad page as if it were full.
func (c *Client) List(ctx context.Context, spec query.Spec, sort string, offset, limit int) (model.ItemsPage, error) {
	if sort != model.SortAsc && sort != model.SortDesc {
		sort = model.SortDesc
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = model.DefaultPageSize
	}
	probeErr := c.ProbeQuery(ctx)
	if probeErr == nil {
		page, err := c.Query(ctx, spec.String(), sort, offset, limit)
		if err == nil {
			return page, nil
		}
		if !IsUnsupported(err) {
			return page, err
		}
	} else if KindOf(probeErr) != KindUnsupported {
		return model.ItemsPage{}, probeErr
	}

	if missing := spec.Unsupported(); len(missing) > 0 {
		return model.ItemsPage{}, &APIError{
			Kind: KindUnsupported,
			Message: "desktop does not support these filters: " + strings.Join(missing, ", ") +
				"; upgrade the desktop app to search with them",
		}
	}
	return c.LegacyItems(ctx, spec, offset, limit)
}

// Query executes POST /api/v1/query.
func (c *Client) Query(ctx context.Context, queryString, sort string, offset, limit int) (model.ItemsPage, error) {
	if limit > model.MaxQueryPage {
		limit = model.MaxQueryPage
	}
	if limit <= 0 {
		limit = model.DefaultPageSize
	}
	body := map[string]any{"query": queryString, "sort": sort, "offset": offset, "limit": limit}
	var page model.ItemsPage
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/query", nil, body, &page)
	if err != nil {
		if api, ok := err.(*APIError); ok && (api.Status == http.StatusNotFound || api.Status == http.StatusMethodNotAllowed) {
			c.setQuerySupport(false)
			api.Kind = KindUnsupported
		}
		return page, err
	}
	normalizePage(&page, offset, limit)
	return page, nil
}

// LegacyItems executes GET /api/v1/items with the filters it understands.
func (c *Client) LegacyItems(ctx context.Context, spec query.Spec, offset, limit int) (model.ItemsPage, error) {
	if limit > model.MaxLegacyPage {
		limit = model.MaxLegacyPage
	}
	if limit <= 0 {
		limit = model.DefaultPageSize
	}
	params := spec.LegacyParams()
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", strconv.Itoa(limit))
	var page model.ItemsPage
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/items", params, nil, &page)
	if err == nil {
		normalizePage(&page, offset, limit)
	}
	return page, err
}

func normalizePage(page *model.ItemsPage, offset, limit int) {
	if page.Items == nil {
		page.Items = []model.ItemSummary{}
	}
	if page.Offset == 0 {
		page.Offset = offset
	}
	if page.Limit == 0 {
		page.Limit = limit
	}
}

// Stats returns the sidebar aggregates. Older desktops return 404/405; callers
// must show counts as unavailable rather than zero.
func (c *Client) Stats(ctx context.Context) (model.Stats, error) {
	var out model.Stats
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/stats", nil, nil, &out)
	if api, ok := err.(*APIError); ok && (api.Status == http.StatusNotFound || api.Status == http.StatusMethodNotAllowed) {
		api.Kind = KindUnsupported
	}
	return out, err
}

// Collection returns one page of a virtual collection (trash, untagged,
// uncategorized). The trash path works without the query endpoint.
func (c *Client) Collection(ctx context.Context, name string, offset, limit int) (model.ItemsPage, error) {
	if limit > model.MaxQueryPage {
		limit = model.MaxQueryPage
	}
	if limit <= 0 {
		limit = model.DefaultPageSize
	}
	params := url.Values{}
	params.Set("name", name)
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", strconv.Itoa(limit))
	var page model.ItemsPage
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/items/collection", params, nil, &page)
	if err == nil {
		normalizePage(&page, offset, limit)
	}
	return page, err
}

// EmptyTrash permanently deletes every item in the trash.
func (c *Client) EmptyTrash(ctx context.Context) error {
	params := url.Values{}
	params.Set("name", model.CollectionTrash)
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/items/collection", params, nil, nil)
}

// Item returns full metadata for one item.
func (c *Client) Item(ctx context.Context, id string) (model.ItemDetail, error) {
	var out model.ItemDetail
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/items/"+url.PathEscape(id), nil, nil, &out)
	return out, err
}

// Delete moves an item to the desktop trash.
func (c *Client) Delete(ctx context.Context, id string) error {
	err := c.doJSON(ctx, http.MethodDelete, "/api/v1/items/"+url.PathEscape(id), nil, nil, nil)
	if api, ok := err.(*APIError); ok {
		switch api.Status {
		case http.StatusNotFound:
			api.Kind = KindNotFound
			api.Message = "素材不存在或已被删除"
		case http.StatusConflict:
			api.Kind = KindConflict
			api.Message = "该素材已在废纸篓"
		case http.StatusServiceUnavailable:
			api.Kind = KindUnavailable
			api.Message = "桌面端未打开资源库"
		}
	}
	return err
}

// Restore brings an item back from the trash.
func (c *Client) Restore(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/items/"+url.PathEscape(id)+"/restore", nil, struct{}{}, nil)
}

// AddTag adds one tag to one item.
func (c *Client) AddTag(ctx context.Context, id, tag string) error {
	body := map[string]string{"tag": tag}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/items/"+url.PathEscape(id)+"/tags", nil, body, nil)
}

// AddFolder adds one item to one folder.
func (c *Client) AddFolder(ctx context.Context, id, folderID string) error {
	body := map[string]string{"folderId": folderID}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/items/"+url.PathEscape(id)+"/folders", nil, body, nil)
}

// Tags returns the tag taxonomy.
func (c *Client) Tags(ctx context.Context) ([]model.Tag, error) {
	var out []model.Tag
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/tags", nil, nil, &out)
	if out == nil {
		out = []model.Tag{}
	}
	return out, err
}

// Folders returns the folder tree as a flat list of top-level nodes with
// children where the server sends them.
func (c *Client) Folders(ctx context.Context) ([]model.Folder, error) {
	var out []model.Folder
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/folders", nil, nil, &out)
	if out == nil {
		out = []model.Folder{}
	}
	return out, err
}

// Colors returns the aggregated dominant colors.
func (c *Client) Colors(ctx context.Context) ([]string, error) {
	var out []string
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/colors", nil, nil, &out)
	if out == nil {
		out = []string{}
	}
	return out, err
}

// Libraries lists the desktop's recent libraries and which is active.
func (c *Client) Libraries(ctx context.Context) ([]model.Library, error) {
	var out model.LibrariesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/libraries", nil, nil, &out); err != nil {
		return nil, err
	}
	if out.Libraries == nil {
		out.Libraries = []model.Library{}
	}
	return out.Libraries, nil
}

// SwitchLibrary changes the desktop's active library, which affects every
// connected client.
func (c *Client) SwitchLibrary(ctx context.Context, req model.SwitchLibraryRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/libraries/switch", nil, req, nil)
}

// MediaURL resolves a relative media path against the origin and appends the
// query token the server accepts for images and videos.
func (c *Client) MediaURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	u, err := url.Parse(c.BaseURL() + path)
	if err != nil {
		return ""
	}
	q := u.Query()
	if token := c.Token(); token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// ThumbURL is MediaURL for an item's thumbnail.
func (c *Client) ThumbURL(id string) string {
	return c.MediaURL("/api/v1/thumbs/" + url.PathEscape(id))
}

// OriginalURL is MediaURL for an item's original file.
func (c *Client) OriginalURL(id string) string {
	return c.MediaURL("/api/v1/originals/" + url.PathEscape(id))
}

// OpenOriginal opens a streaming response for an item's original file. The
// caller closes the body. Range and HEAD are the server's responsibility; a
// video player opens this URL directly.
func (c *Client) OpenOriginal(ctx context.Context, id string) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/originals/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, networkError(err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	return resp, nil
}

// Download copies an original to dest through a .part file and an atomic
// rename. progress may be nil. A failed or cancelled download leaves no dest.
func (c *Client) Download(ctx context.Context, id, dest string, progress func(loaded, total int64)) (int64, error) {
	resp, err := c.OpenOriginal(ctx, id)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}()
	reader := io.Reader(resp.Body)
	if progress != nil {
		reader = &progressReader{r: resp.Body, total: resp.ContentLength, report: progress}
	}
	written, err := io.Copy(f, reader)
	if err != nil {
		return written, networkError(err)
	}
	if err := f.Sync(); err != nil {
		return written, err
	}
	if err := f.Close(); err != nil {
		return written, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return written, err
	}
	return written, nil
}

type progressReader struct {
	r      io.Reader
	total  int64
	loaded int64
	last   int64
	report func(int64, int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.loaded += int64(n)
	if p.loaded-p.last >= 256<<10 || (err == io.EOF && p.loaded != p.last) {
		p.last = p.loaded
		p.report(p.loaded, p.total)
	}
	return n, err
}

// Upload streams one file as multipart field "file". It does not read the file
// into memory. The desktop's total request ceiling is 500 MiB, which includes
// multipart overhead, so one file may not exceed it.
func (c *Client) Upload(ctx context.Context, filename string, r io.Reader, size int64) (model.UploadResult, error) {
	return c.UploadToFolder(ctx, filename, r, size, "")
}

// UploadToFolder streams one file to /api/v1/upload, optionally assigning it to folderID.
func (c *Client) UploadToFolder(ctx context.Context, filename string, r io.Reader, size int64, folderID string) (model.UploadResult, error) {
	var out model.UploadResult
	if size > 500<<20 {
		return out, &APIError{Kind: KindInvalid, Message: "file exceeds the 500 MB upload limit"}
	}
	base := c.BaseURL()
	if base == "" {
		return out, &APIError{Kind: KindInvalid, Message: "no desktop address configured"}
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType()
	go func() {
		var err error
		if folderID != "" {
			err = mw.WriteField("folderId", folderID)
		}
		var part io.Writer
		if err == nil {
			part, err = mw.CreateFormFile("file", filepath.Base(filename))
		}
		if err == nil {
			_, err = io.Copy(part, io.LimitReader(r, 500<<20))
		}
		if closeErr := mw.Close(); err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/upload", pr)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return out, networkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, decodeError(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return out, &APIError{Status: resp.StatusCode, Kind: KindServer, Message: "invalid upload reply", Err: err}
	}
	return out, nil
}

// UploadFile opens path and uploads it.
func (c *Client) UploadFile(ctx context.Context, path string) (model.UploadResult, error) {
	return c.UploadFileToFolder(ctx, path, "")
}

// UploadFileToFolder opens path and uploads it, optionally assigning it to folderID.
func (c *Client) UploadFileToFolder(ctx context.Context, path, folderID string) (model.UploadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.UploadResult{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return model.UploadResult{}, err
	}
	return c.UploadToFolder(ctx, info.Name(), f, info.Size(), folderID)
}

// ShareText sends shared text or URLs directly to desktop for URL resolution or text saving.
func (c *Client) ShareText(ctx context.Context, text, subject, folderID string) (model.UploadResult, error) {
	var out model.UploadResult
	body := map[string]string{
		"text": text,
	}
	if subject != "" {
		body["subject"] = subject
	}
	if folderID != "" {
		body["folderId"] = folderID
	}
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/share/text", nil, body, &out)
	return out, err
}

// BatchItemError describes one item failure in a batch operation.
type BatchItemError struct {
	ID  string
	Err error
}

// BatchResult summarizes per-item batch execution without aborting on first error.
type BatchResult struct {
	Total     int
	Succeeded int
	Failures  []BatchItemError
}

func (r BatchResult) Summary(action string) string {
	if len(r.Failures) == 0 {
		return fmt.Sprintf("已为 %d 项%s", r.Succeeded, action)
	}
	if r.Succeeded == 0 {
		return fmt.Sprintf("%s失败: %s", action, DescribeFailure(r.Failures[0].Err))
	}
	return fmt.Sprintf("成功 %d 项，失败 %d 项（%s）", r.Succeeded, len(r.Failures), DescribeFailure(r.Failures[0].Err))
}

// BatchAddTag adds tag to every ID in ids, continuing past individual failures.
func (c *Client) BatchAddTag(ctx context.Context, ids []string, tag string, progress func(done, total int)) BatchResult {
	res := BatchResult{Total: len(ids)}
	for i, id := range ids {
		if ctx.Err() != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: ctx.Err()})
			continue
		}
		if err := c.AddTag(ctx, id, tag); err != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: err})
		} else {
			res.Succeeded++
		}
		if progress != nil {
			progress(i+1, len(ids))
		}
	}
	return res
}

// BatchAddFolder adds every ID in ids to folderID, continuing past individual failures.
func (c *Client) BatchAddFolder(ctx context.Context, ids []string, folderID string, progress func(done, total int)) BatchResult {
	res := BatchResult{Total: len(ids)}
	for i, id := range ids {
		if ctx.Err() != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: ctx.Err()})
			continue
		}
		if err := c.AddFolder(ctx, id, folderID); err != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: err})
		} else {
			res.Succeeded++
		}
		if progress != nil {
			progress(i+1, len(ids))
		}
	}
	return res
}

// BatchDelete moves every ID in ids to trash, continuing past individual failures.
func (c *Client) BatchDelete(ctx context.Context, ids []string, progress func(done, total int)) BatchResult {
	res := BatchResult{Total: len(ids)}
	for i, id := range ids {
		if ctx.Err() != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: ctx.Err()})
			continue
		}
		if err := c.Delete(ctx, id); err != nil {
			res.Failures = append(res.Failures, BatchItemError{ID: id, Err: err})
		} else {
			res.Succeeded++
		}
		if progress != nil {
			progress(i+1, len(ids))
		}
	}
	return res
}

// PruneDir removes files in dir older than maxAge (plus stale .part files older than 1h).
func PruneDir(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		age := now.Sub(info.ModTime())
		if strings.HasSuffix(entry.Name(), ".part") {
			if age > time.Hour {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
			continue
		}
		if age > maxAge {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// DescribeFailure turns an error into a short, actionable message.
func DescribeFailure(err error) string {
	if err == nil {
		return ""
	}
	kind := KindOf(err)
	switch kind {
	case KindUnauthorized:
		return "授权已失效，请重新配对"
	case KindUnavailable:
		return "桌面端资源库未打开"
	case KindTimeout:
		return "连接超时，请确认手机和电脑在同一网络"
	case KindNetwork:
		return "连接失败，请确认桌面端正在运行且地址可访问"
	case KindUnsupported:
		return "当前桌面端版本不支持该功能，请升级桌面端"
	case KindNotFound:
		return "内容不存在或已被删除"
	case KindConflict:
		return "操作冲突，请刷新后重试"
	case KindCanceled:
		return "操作已取消"
	default:
		return fmt.Sprintf("%v", err)
	}
}
