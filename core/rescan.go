package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

func isRescanBookkeepingFile(name string) bool {
	switch strings.ToLower(name) {
	case itemMetadataName, "thumbnail.png", "thumbnail.jpg", libraryManifestName, libraryDBName, libraryDBName + "-wal", libraryDBName + "-shm", LibraryIconFile:
		return true
	default:
		return false
	}
}

// inspectExistingThumbnail reads dimensions and color palette from an existing
// image/thumbnail without overwriting any thumbnail file on disk.
func inspectExistingThumbnail(assetPath, thumbPath, ext string) (int, int, string, []string, *ImageMeta) {
	candidates := []string{assetPath, thumbPath}
	if IsVideoExtension(ext) {
		candidates = []string{thumbPath}
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		img, _, decErr := image.Decode(f)
		_ = f.Close()
		if decErr != nil {
			continue
		}
		b := img.Bounds()
		origW, origH := b.Dx(), b.Dy()
		sampleW := min(origW, 128)
		sampleH := min(origH, 128)
		sample := image.NewRGBA(image.Rect(0, 0, max(1, sampleW), max(1, sampleH)))
		draw.BiLinear.Scale(sample, sample.Bounds(), img, b, draw.Over, nil)
		dom, pal, meta := ExtractPaletteAndMeta(img, sample, origW, origH, ext, assetPath)
		return origW, origH, dom, pal, meta
	}
	return 0, 0, "", []string{}, nil
}

// RescanLibrary scans the active library's items/ directory for asset folders
// that exist on disk but are missing from SQLite, and inserts them into "All"
// without modifying existing database records, folders, tags, or original files.
func (m *LibraryManager) RescanLibrary() (int, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	port := m.GetAssetServerPort()
	m.mu.RUnlock()

	if active == nil || db == nil {
		return 0, errors.New("no active library")
	}

	_ = m.ensureCacheLoaded()

	existingIDs := make(map[string]bool)
	rows, err := db.Query(`SELECT id FROM items`)
	if err != nil {
		return 0, fmt.Errorf("failed to query existing items: %w", err)
	}
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr == nil && id != "" {
			existingIDs[id] = true
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("failed to iterate existing items: %w", rowsErr)
	}
	_ = rows.Close()

	itemsRoot := filepath.Join(active.Path, "items")
	entries, err := os.ReadDir(itemsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read items directory: %w", err)
	}

	type scannedItem struct {
		item       Item
		hasThumb   int
		paletteStr string
		metaJSON   string
	}

	var discovered []scannedItem
	now := time.Now().Unix()
	totalEntries := len(entries)
	if totalEntries > 0 {
		m.emit(LibraryIndexProgressEvent, LibraryIndexProgress{
			Active:  true,
			Current: 0,
			Total:   totalEntries,
		})
		defer m.emit(LibraryIndexProgressEvent, LibraryIndexProgress{
			Active:  false,
			Current: totalEntries,
			Total:   totalEntries,
		})
	}
	var lastEmit time.Time

	for i, entry := range entries {
		if totalEntries > 0 && (i == totalEntries-1 || time.Since(lastEmit) >= 50*time.Millisecond) {
			lastEmit = time.Now()
			m.emit(LibraryIndexProgressEvent, LibraryIndexProgress{
				Active:  true,
				Current: i + 1,
				Total:   totalEntries,
			})
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		itemID := entry.Name()
		if existingIDs[itemID] {
			continue
		}

		itemDir := filepath.Join(itemsRoot, itemID)
		dirEntries, readErr := os.ReadDir(itemDir)
		if readErr != nil {
			continue
		}

		var assetPath string
		var existingThumbPath string
		var hasMetaFile bool
		var preloadedMeta *ItemMetadata
		backupNames := map[string]bool{}

		if raw, rErr := os.ReadFile(filepath.Join(itemDir, itemMetadataName)); rErr == nil {
			var parsed ItemMetadata
			if json.Unmarshal(raw, &parsed) == nil {
				preloadedMeta = &parsed
				hasMetaFile = true
				for _, v := range parsed.Versions {
					if v.Filename != "" {
						backupNames[strings.ToLower(v.Filename)] = true
					}
				}
				if parsed.Filename != "" {
					cand := filepath.Join(itemDir, parsed.Filename)
					if st, stErr := os.Stat(cand); stErr == nil && st.Mode().IsRegular() {
						assetPath = cand
					}
				}
			}
		}

		for _, de := range dirEntries {
			name := de.Name()
			if de.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			lower := strings.ToLower(name)
			if lower == "thumbnail.png" {
				existingThumbPath = filepath.Join(itemDir, name)
				continue
			}
			if lower == "thumbnail.jpg" && existingThumbPath == "" {
				existingThumbPath = filepath.Join(itemDir, name)
				continue
			}
			if lower == itemMetadataName {
				hasMetaFile = true
				continue
			}
			if isRescanBookkeepingFile(name) || backupNames[lower] || isVersionBackupFilename(name) {
				continue
			}
			if assetPath == "" {
				assetPath = filepath.Join(itemDir, name)
			}
		}
		_ = preloadedMeta

		if assetPath == "" {
			continue
		}

		itemLock := m.acquireItemLock(itemID)
		stat, statErr := os.Stat(assetPath)
		if statErr != nil || !stat.Mode().IsRegular() {
			itemLock.Unlock()
			continue
		}

		filename := filepath.Base(assetPath)
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
		nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))
		mimeType := DetectMimeType(assetPath, ext)
		createdAt := stat.ModTime().Unix()
		importedAt := now
		updatedAt := now
		hexVal := ""

		var width, height int
		var hasThumbnail bool
		var dominantColor string
		var palette []string
		var imgMeta *ImageMeta
		var variants []string
		var trashedAt int64
		var inTrash bool

		if hasMetaFile {
			if raw, rErr := os.ReadFile(filepath.Join(itemDir, itemMetadataName)); rErr == nil {
				var meta ItemMetadata
				if json.Unmarshal(raw, &meta) == nil {
					if meta.Name != "" {
						nameWithoutExt = meta.Name
					}
					if meta.Hex != "" && (meta.Size == 0 || meta.Size == stat.Size()) {
						hexVal = meta.Hex
					}
					if meta.MimeType != "" {
						mimeType = meta.MimeType
					}
					if meta.CreatedAt > 0 {
						createdAt = meta.CreatedAt
					}
					if meta.ImportedAt > 0 {
						importedAt = meta.ImportedAt
					}
					if meta.TrashedAt > 0 {
						trashedAt = meta.TrashedAt
						inTrash = true
					}
					width = meta.Width
					height = meta.Height
					hasThumbnail = meta.HasThumbnail
					dominantColor = meta.DominantColor
					palette = meta.Palette
					imgMeta = meta.Meta
					variants = meta.Variants
				}
			}
		}

		if hexVal == "" {
			if computedHex, hErr := ComputeFileSHA256(assetPath); hErr == nil && computedHex != "" {
				hexVal = computedHex
			}
		}

		if existingThumbPath != "" {
			hasThumbnail = true
			if width == 0 || height == 0 || dominantColor == "" {
				w, h, dom, pal, im := inspectExistingThumbnail(assetPath, existingThumbPath, ext)
				if width == 0 {
					width = w
				}
				if height == 0 {
					height = h
				}
				if dominantColor == "" {
					dominantColor = dom
				}
				if len(palette) == 0 {
					palette = pal
				}
				if imgMeta == nil {
					imgMeta = im
				}
			}
		} else {
			w, h, hasThumb, dom, pal, im := m.processImageThumbnail(assetPath, itemDir, ext)
			if hasThumb {
				hasThumbnail = true
			}
			if width == 0 {
				width = w
			}
			if height == 0 {
				height = h
			}
			if dominantColor == "" {
				dominantColor = dom
			}
			if len(palette) == 0 {
				palette = pal
			}
			if imgMeta == nil {
				imgMeta = im
			}
		}

		if palette == nil {
			palette = []string{}
		}

		item := Item{
			ID:            itemID,
			Hex:           hexVal,
			Name:          nameWithoutExt,
			Extension:     ext,
			Filename:      filename,
			Size:          stat.Size(),
			MimeType:      mimeType,
			Width:         width,
			Height:        height,
			HasThumbnail:  hasThumbnail,
			DominantColor: dominantColor,
			Palette:       palette,
			Meta:          imgMeta,
			Variants:      variants,
			Tags:          []string{},
			Folders:       []string{},
			InTrash:       inTrash,
			TrashedAt:     trashedAt,
			CreatedAt:     createdAt,
			ImportedAt:    importedAt,
			UpdatedAt:     updatedAt,
		}
		m.populateItemPaths(&item, active.Path, port)

		hasThumbInt := 0
		if hasThumbnail {
			hasThumbInt = 1
		}
		metaJSONStr := ""
		if imgMeta != nil {
			if b, mErr := json.Marshal(imgMeta); mErr == nil {
				metaJSONStr = string(b)
			}
		}

		discovered = append(discovered, scannedItem{
			item:       item,
			hasThumb:   hasThumbInt,
			paletteStr: strings.Join(palette, ","),
			metaJSON:   metaJSONStr,
		})
		itemLock.Unlock()
	}

	if len(discovered) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin rescan transaction: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO items (id, hex, name, extension, filename, size, mime_type, width, height, has_thumbnail, dominant_color, palette, meta_json, tags, in_trash, trashed_at, created_at, imported_at, updated_at, variants_json)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("failed to prepare rescan insert: %w", err)
	}

	var addedItems []Item
	for _, d := range discovered {
		inTrashInt := 0
		if d.item.InTrash {
			inTrashInt = 1
		}
		variantsJSON := ""
		if len(d.item.Variants) > 0 {
			if vb, vErr := json.Marshal(d.item.Variants); vErr == nil {
				variantsJSON = string(vb)
			}
		}
		res, execErr := stmt.Exec(
			d.item.ID, d.item.Hex, d.item.Name, d.item.Extension, d.item.Filename, d.item.Size, d.item.MimeType,
			d.item.Width, d.item.Height, d.hasThumb, d.item.DominantColor, d.paletteStr, d.metaJSON, "", inTrashInt, d.item.TrashedAt, d.item.CreatedAt, d.item.ImportedAt, d.item.UpdatedAt, variantsJSON,
		)
		if execErr != nil {
			continue
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			addedItems = append(addedItems, d.item)
		}
	}
	_ = stmt.Close()

	for _, d := range discovered {
		for _, vid := range d.item.Variants {
			if vid != "" && vid != d.item.ID {
				_, _ = tx.Exec("UPDATE items SET variant_of = ? WHERE id = ?", d.item.ID, vid)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit rescan transaction: %w", err)
	}

	if len(addedItems) > 0 {
		m.upsertItemsInCache(addedItems, "")
		var activeAdded []Item
		var trashedAdded []Item
		for _, it := range addedItems {
			if it.InTrash {
				trashedAdded = append(trashedAdded, it)
			} else {
				activeAdded = append(activeAdded, it)
			}
		}
		if len(trashedAdded) > 0 {
			m.cacheMu.Lock()
			if m.cacheLoaded {
				m.trashDesc = append(trashedAdded, m.trashDesc...)
			}
			m.cacheMu.Unlock()
		}
		if len(activeAdded) > 0 {
			m.emit("library-items-upserted", activeAdded)
		}
		m.notifyItemsUpdated()
	}

	RecordLog(
		"info",
		"core",
		"Rescanned library items directory",
		fmt.Sprintf("added=%d totalScannedNew=%d", len(addedItems), len(discovered)),
		0,
	)

	return len(addedItems), nil
}
