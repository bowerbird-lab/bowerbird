package core

// LibraryInfo represents metadata about an opened library.
type LibraryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Icon names the built-in icon the library picked, or LibraryIconCustom
	// when LibraryIconFile beside the manifest holds its picture. Empty means
	// the default icon.
	Icon      string `json:"icon,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ItemCount int64  `json:"itemCount"`
}

// Folder represents a virtual organizational directory in the library.
type Folder struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ParentID  string   `json:"parentId"`
	CreatedAt int64    `json:"createdAt"`
	SortOrder int      `json:"sortOrder"`
	Children  []Folder `json:"children,omitempty"`
	ItemCount int      `json:"itemCount"`
}

// Tag represents a tag taxonomy entry with its item count.
type Tag struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	ItemCount int    `json:"itemCount"`
}

// ImageMeta holds extended metadata extracted from an image or media file.
type ImageMeta struct {
	ColorModel  string `json:"colorModel,omitempty"`
	ColorSpace  string `json:"colorSpace,omitempty"`
	HasAlpha    bool   `json:"hasAlpha,omitempty"`
	AspectRatio string `json:"aspectRatio,omitempty"`
	Orientation string `json:"orientation,omitempty"` // "landscape" | "portrait" | "square"
	BitDepth    int    `json:"bitDepth,omitempty"`
	Animated    bool   `json:"animated,omitempty"`
	ExifMake    string `json:"exifMake,omitempty"`
	ExifModel   string `json:"exifModel,omitempty"`
	ExifDate    string `json:"exifDate,omitempty"`
	Megapixels  string `json:"megapixels,omitempty"`
}

// ItemVersion records a backed-up historical file stored inside items/<id>/.
type ItemVersion struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Extension string `json:"extension"`
	MimeType  string `json:"mimeType,omitempty"`
	Size      int64  `json:"size"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Hex       string `json:"hex,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	Note      string `json:"note,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Source    string `json:"source,omitempty"`
}

// ReplaceItemOptions specifies metadata when replacing an item's physical file with a backup.
type ReplaceItemOptions struct {
	Extension string `json:"extension,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Note      string `json:"note,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Item represents a single managed asset in the library.
type Item struct {
	ID            string        `json:"id"`
	Hex           string        `json:"hex"` // uppercase content hash (SHA-256)
	Name          string        `json:"name"`
	Extension     string        `json:"extension"`
	Filename      string        `json:"filename"`
	Size          int64         `json:"size"`
	MimeType      string        `json:"mimeType"`
	Width         int           `json:"width"`
	Height        int           `json:"height"`
	HasThumbnail  bool          `json:"hasThumbnail"`
	DominantColor string        `json:"dominantColor"`
	Palette       []string      `json:"palette"`
	Meta          *ImageMeta    `json:"meta,omitempty"`
	Note          string        `json:"note"`
	URL           string        `json:"url"`
	Rating        int           `json:"rating"`
	Prompt        string        `json:"prompt"`
	Tags          []string      `json:"tags"`
	Folders       []string      `json:"folders"`
	Versions      []ItemVersion `json:"versions,omitempty"`
	InTrash       bool          `json:"inTrash"`
	TrashedAt     int64         `json:"trashedAt"`
	CreatedAt     int64         `json:"createdAt"`
	ImportedAt    int64         `json:"importedAt"`
	UpdatedAt     int64         `json:"updatedAt"`
	ItemPath      string        `json:"itemPath"`     // local disk path of item directory (items/<ID>)
	FilePath      string        `json:"filePath"`     // absolute local path to physical file
	ShellPath     string        `json:"shellPath"`    // shell-escaped path with backslash spaces
	FileURL       string        `json:"fileUrl"`      // file:// URI with spaces %20-encoded
	OriginalURL   string        `json:"originalUrl"`  // HTTP stream URL
	ThumbnailURL  string        `json:"thumbnailUrl"` // HTTP thumbnail URL
	DownloadURL   string        `json:"downloadUrl"`  // HTTP download URL for OS drag-out
}

// ItemMetadata is serialized inside items/<id>/metadata.json.
type ItemMetadata struct {
	ID             string        `json:"id"`
	Hex            string        `json:"hex"`
	Name           string        `json:"name"`
	Extension      string        `json:"extension"`
	Filename       string        `json:"filename"`
	Size           int64         `json:"size"`
	MimeType       string        `json:"mimeType"`
	Width          int           `json:"width"`
	Height         int           `json:"height"`
	HasThumbnail   bool          `json:"hasThumbnail"`
	DominantColor  string        `json:"dominantColor,omitempty"`
	Palette        []string      `json:"palette,omitempty"`
	Meta           *ImageMeta    `json:"meta,omitempty"`
	Note           string        `json:"note,omitempty"`
	URL            string        `json:"url,omitempty"`
	Rating         int           `json:"rating,omitempty"`
	Prompt         string        `json:"prompt,omitempty"`
	Tags           []string      `json:"tags"`
	Folders        []string      `json:"folders"`
	Versions       []ItemVersion `json:"versions,omitempty"`
	InTrash        bool          `json:"inTrash,omitempty"`
	TrashedAt      int64         `json:"trashedAt,omitempty"`
	TrashedFolders []string      `json:"trashedFolders,omitempty"`
	CreatedAt      int64         `json:"createdAt"`
	ImportedAt     int64         `json:"importedAt"`
	UpdatedAt      int64         `json:"updatedAt,omitempty"`
}

// ImportItemOptions specifies metadata when importing items via API or browser extension.
type ImportItemOptions struct {
	Name      string            `json:"name,omitempty"`
	URL       string            `json:"url,omitempty"`
	Note      string            `json:"note,omitempty"`
	Rating    int               `json:"rating,omitempty"`
	Prompt    string            `json:"prompt,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	FolderIDs []string          `json:"folderIds,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	// SkipURL disables the automatic source-URL backfill for callers that
	// intentionally do not want to record where a download came from.
	SkipURL bool `json:"skipUrl,omitempty"`
}

// NetworkInterfaceInfo describes an available network interface for mobile discovery.
type NetworkInterfaceInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // "lan" | "tailscale" | "other"
	IPs      []string `json:"ips"`
	Selected bool     `json:"selected"`
}

// MobileLibraryEntry represents a library for mobile listing and switching.
type MobileLibraryEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ItemCount int64  `json:"itemCount"`
	IsActive  bool   `json:"isActive"`
}

// LibraryIndexProgressEvent is emitted while initializing or rescanning the
// library's SQLite hex index.
const LibraryIndexProgressEvent = "library-index-progress"

// LibraryIndexProgress reports progress for library hex index initialization or rescanning.
type LibraryIndexProgress struct {
	Active  bool `json:"active"`
	Current int  `json:"current"`
	Total   int  `json:"total"`
}
