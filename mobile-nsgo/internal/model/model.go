// Package model holds the wire DTOs and the client-side domain values the
// mobile app shares between transport, storage and UI. Every endpoint is
// decoded into its own type; the desktop replies are not wrapped uniformly.
package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Status is the public GET /api/v1/status reply.
type Status struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	ItemCount       int64    `json:"itemCount"`
	PairingRequired bool     `json:"pairingRequired"`
	Capabilities    []string `json:"capabilities"`
	DeviceID        string   `json:"deviceId,omitempty"`
	Addresses       []string `json:"addresses,omitempty"`
	Port            int      `json:"port,omitempty"`
}

// HasCapability reports whether the desktop advertised a capability.
// Status on older builds may be empty, so callers must also probe endpoints.
func (s Status) HasCapability(name string) bool {
	for _, c := range s.Capabilities {
		if c == name {
			return true
		}
	}
	return false
}

// PairRequest is POST /api/v1/pair's body.
type PairRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"deviceName"`
}

// PairResult is the token reply. The envelope also carries a display name.
type PairResult struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// Library is one entry of GET /api/v1/libraries.
type Library struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ItemCount int64  `json:"itemCount"`
	IsActive  bool   `json:"isActive"`
}

// LibrariesResponse is the wrapping object the endpoint returns.
type LibrariesResponse struct {
	Libraries []Library `json:"libraries"`
}

// SwitchLibraryRequest is POST /api/v1/libraries/switch's body.
type SwitchLibraryRequest struct {
	ID   string `json:"id,omitempty"`
	Path string `json:"path,omitempty"`
}

// ItemSummary is a row in a paginated list.
type ItemSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Filename     string   `json:"filename"`
	Extension    string   `json:"extension"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	Size         int64    `json:"size"`
	MimeType     string   `json:"mimeType"`
	ThumbnailURL string   `json:"thumbnailUrl"`
	OriginalURL  string   `json:"originalUrl"`
	CreatedAt    int64    `json:"createdAt"`
	ImportedAt   int64    `json:"importedAt"`
	Date         int64    `json:"date"`
	Orientation  string   `json:"orientation"`
	Tags         []string `json:"tags"`
	Folders      []string `json:"folders"`
	Variants     []string `json:"variants,omitempty"`
	VariantOf    string   `json:"variantOf,omitempty"`
}

// ItemsPage is the reply of /query, /items and collection endpoints.
type ItemsPage struct {
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
	Items  []ItemSummary `json:"items"`
}

// Stats is GET /api/v1/stats.
type Stats struct {
	Total              int      `json:"total"`
	UntaggedCount      int      `json:"untaggedCount"`
	UncategorizedCount int      `json:"uncategorizedCount"`
	TrashCount         int      `json:"trashCount"`
	TotalSize          int64    `json:"totalSize"`
	TopColors          []string `json:"topColors"`
}

// Tag is one entry of GET /api/v1/tags.
type Tag struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	ItemCount int    `json:"itemCount"`
}

// Folder is one node of GET /api/v1/folders.
type Folder struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ParentID  string   `json:"parentId"`
	CreatedAt int64    `json:"createdAt"`
	SortOrder int      `json:"sortOrder"`
	Children  []Folder `json:"children,omitempty"`
	ItemCount int      `json:"itemCount"`
}

// ImageMeta is the extra metadata carried by a detail reply.
type ImageMeta struct {
	ColorModel  string `json:"colorModel,omitempty"`
	ColorSpace  string `json:"colorSpace,omitempty"`
	HasAlpha    bool   `json:"hasAlpha,omitempty"`
	AspectRatio string `json:"aspectRatio,omitempty"`
	Orientation string `json:"orientation,omitempty"`
	BitDepth    int    `json:"bitDepth,omitempty"`
	Animated    bool   `json:"animated,omitempty"`
	ExifMake    string `json:"exifMake,omitempty"`
	ExifModel   string `json:"exifModel,omitempty"`
	ExifDate    string `json:"exifDate,omitempty"`
	Megapixels  string `json:"megapixels,omitempty"`
}

// ItemDetail is GET /api/v1/items/{id}. It mirrors the desktop Item JSON, but
// keeps only fields a phone may show or act on.
type ItemDetail struct {
	ID            string     `json:"id"`
	Hex           string     `json:"hex"`
	Name          string     `json:"name"`
	Extension     string     `json:"extension"`
	Filename      string     `json:"filename"`
	Size          int64      `json:"size"`
	MimeType      string     `json:"mimeType"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	HasThumbnail  bool       `json:"hasThumbnail"`
	DominantColor string     `json:"dominantColor"`
	Palette       []string   `json:"palette"`
	Meta          *ImageMeta `json:"meta,omitempty"`
	Note          string     `json:"note"`
	URL           string     `json:"url"`
	Rating        int        `json:"rating"`
	Prompt        string     `json:"prompt"`
	Tags          []string   `json:"tags"`
	Folders       []string   `json:"folders"`
	InTrash       bool       `json:"inTrash"`
	TrashedAt     int64      `json:"trashedAt"`
	CreatedAt     int64      `json:"createdAt"`
	ImportedAt    int64      `json:"importedAt"`
	UpdatedAt     int64      `json:"updatedAt"`
	ItemPath      string     `json:"itemPath"`
	FilePath      string     `json:"filePath"`
	OriginalURL   string     `json:"originalUrl"`
	ThumbnailURL  string     `json:"thumbnailUrl"`
	Variants      []string   `json:"variants,omitempty"`
	VariantOf     string     `json:"variantOf,omitempty"`
}

// UploadResult is POST /api/v1/upload's reply.
type UploadResult struct {
	Status string       `json:"status"`
	Count  int          `json:"count"`
	Items  []ItemDetail `json:"items"`
}

// WSEvent is one WebSocket message. Older servers may fold event fields into
// the top level; Data keeps the nested payload and Raw keeps the whole message.
type WSEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
	Raw   map[string]any  `json:"-"`
}

// Event names understood by the client.
const (
	EventLibraryChanged = "library_changed"
	EventItemsUpdated   = "items_updated"
	EventPairingRevoked = "pairing_revoked"
)

// Collection names the drawer's virtual item lists.
const (
	CollectionTrash         = "trash"
	CollectionUntagged      = "untagged"
	CollectionUncategorized = "uncategorized"
)

// Sort orders accepted by /query.
const (
	SortDesc = "desc"
	SortAsc  = "asc"
)

// Page sizes. Query's server ceiling is 500, the legacy /items ceiling 200.
const (
	DefaultPageSize = 60
	MaxQueryPage    = 500
	MaxLegacyPage   = 200
)

// IsVideo reports whether the summary names a video.
func (i ItemSummary) IsVideo() bool {
	return strings.HasPrefix(strings.ToLower(i.MimeType), "video/") ||
		containsExt(i.Extension, "mp4", "mov", "webm", "mkv", "avi", "m4v")
}

// IsGIF reports whether the summary names a GIF.
func (i ItemSummary) IsGIF() bool {
	return strings.EqualFold(i.MimeType, "image/gif") || strings.EqualFold(i.Extension, "gif")
}

// IsImage reports whether the summary names an image.
func (i ItemSummary) IsImage() bool {
	return strings.HasPrefix(strings.ToLower(i.MimeType), "image/") ||
		containsExt(i.Extension, "png", "jpg", "jpeg", "webp", "gif", "bmp", "tiff", "ico", "avif", "heic")
}

// IsAudio reports whether the summary names audio.
func (i ItemSummary) IsAudio() bool {
	return strings.HasPrefix(strings.ToLower(i.MimeType), "audio/") ||
		containsExt(i.Extension, "mp3", "wav", "flac", "aac", "m4a", "ogg")
}

// BestDate returns the most meaningful timestamp for grouping, in Unix ms.
func (i ItemSummary) BestDate() int64 {
	if i.Date > 0 {
		return i.Date
	}
	if i.CreatedAt > 0 {
		return i.CreatedAt
	}
	return i.ImportedAt
}

// DisplayName falls back from the title to the filename and then the ID.
func (i ItemSummary) DisplayName() string {
	if strings.TrimSpace(i.Name) != "" {
		return i.Name
	}
	if strings.TrimSpace(i.Filename) != "" {
		return i.Filename
	}
	return i.ID
}

// HasPages reports whether the item belongs to a multi-page group.
func (i ItemSummary) HasPages() bool {
	return len(i.Variants) > 0 || i.VariantOf != ""
}

// PageCount returns the number of pages when this item is the group cover, or 0.
func (i ItemSummary) PageCount() int {
	if len(i.Variants) > 0 {
		return len(i.Variants) + 1
	}
	return 0
}

// Summary converts a detail reply into a summary, for usage history and
// places that only hold an ID.
func (d ItemDetail) Summary() ItemSummary {
	orientation := "landscape"
	if d.Width < d.Height {
		orientation = "portrait"
	} else if d.Width == d.Height {
		orientation = "square"
	}
	thumb := d.ThumbnailURL
	if thumb == "" {
		thumb = "/api/v1/thumbs/" + d.ID
	}
	original := d.OriginalURL
	if original == "" {
		original = "/api/v1/originals/" + d.ID
	}
	return ItemSummary{
		ID:           d.ID,
		Name:         d.Name,
		Filename:     d.Filename,
		Extension:    d.Extension,
		Width:        d.Width,
		Height:       d.Height,
		Size:         d.Size,
		MimeType:     d.MimeType,
		ThumbnailURL: thumb,
		OriginalURL:  original,
		CreatedAt:    d.CreatedAt,
		ImportedAt:   d.ImportedAt,
		Date:         d.CreatedAt,
		Orientation:  orientation,
		Tags:         d.Tags,
		Folders:      d.Folders,
		Variants:     d.Variants,
		VariantOf:    d.VariantOf,
	}
}

// FormatSize renders a byte count the way the drawer and detail sheet show it.
func FormatSize(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size) / 1024
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func containsExt(ext string, values ...string) bool {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	for _, v := range values {
		if e == v {
			return true
		}
	}
	return false
}
