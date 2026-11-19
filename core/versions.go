package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const versionFilePrefix = "v-"

func isVersionBackupFilename(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, versionFilePrefix)
}

func generateVersionID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

func readItemVersions(itemDir string) []ItemVersion {
	if itemDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(itemDir, itemMetadataName))
	if err != nil {
		return nil
	}
	var meta ItemMetadata
	if json.Unmarshal(raw, &meta) != nil || len(meta.Versions) == 0 {
		return nil
	}
	out := make([]ItemVersion, 0, len(meta.Versions))
	for _, v := range meta.Versions {
		if v.ID == "" || v.Filename == "" {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(itemDir, v.Filename)); statErr != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func detectImageExt(data []byte, fallback string) string {
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(fallback), "."))
	if ext != "" {
		return ext
	}
	ct := http.DetectContentType(data)
	switch ct {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "image/bmp":
		return "bmp"
	default:
		return "png"
	}
}

func buildActiveFilename(name, oldFilename, newExt string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = strings.TrimSuffix(oldFilename, filepath.Ext(oldFilename))
	}
	if base == "" {
		base = "image"
	}
	// Strip path separators or illegal characters to keep filename inside itemDir.
	base = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, base)
	if newExt == "" {
		return base
	}
	return base + "." + newExt
}

// GetItemVersions returns the historical backup versions recorded in items/<id>/metadata.json.
func (m *LibraryManager) GetItemVersions(itemID string) ([]ItemVersion, error) {
	item, err := m.GetItem(itemID)
	if err != nil || item == nil {
		return nil, errors.New("item not found")
	}
	return readItemVersions(item.ItemPath), nil
}

// SaveNewItemFromBytes imports generated image bytes into the active library as a new item.
func (m *LibraryManager) SaveNewItemFromBytes(data []byte, filename string, opts ImportItemOptions) (*Item, error) {
	if len(data) == 0 {
		return nil, errors.New("image data is empty")
	}
	ext := detectImageExt(data, filepath.Ext(filename))
	cleanName := strings.TrimSpace(filename)
	if cleanName == "" {
		cleanName = fmt.Sprintf("ai-%d.%s", time.Now().Unix(), ext)
	} else if filepath.Ext(cleanName) == "" {
		cleanName = cleanName + "." + ext
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return m.ImportFromBase64WithOptions(cleanName, b64, opts)
}

// ReplaceItemWithBackup overwrites an item's physical file with newData while
// moving the previous file into the same item directory (items/<id>/) and
// recording it inside metadata.json so it can be restored or deleted later.
func (m *LibraryManager) ReplaceItemWithBackup(itemID string, newData []byte, opts ReplaceItemOptions) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	port := m.GetAssetServerPort()
	m.mu.RUnlock()

	if active == nil || db == nil {
		return nil, errors.New("no active library")
	}
	if len(newData) == 0 {
		return nil, errors.New("replacement file data is empty")
	}

	itemLock := m.acquireItemLock(itemID)
	defer itemLock.Unlock()

	item, err := m.GetItem(itemID)
	if err != nil || item == nil {
		return nil, errors.New("item not found")
	}

	itemDir := filepath.Join(active.Path, "items", item.ID)
	if err := ensureItemDir(itemDir); err != nil {
		return nil, err
	}

	oldPath := filepath.Join(itemDir, item.Filename)
	oldStat, statErr := os.Stat(oldPath)
	if statErr != nil {
		return nil, fmt.Errorf("existing item file not found: %w", statErr)
	}

	// Read existing metadata.json (or synthesize from item if missing).
	metaPath := filepath.Join(itemDir, itemMetadataName)
	var meta ItemMetadata
	if raw, rErr := os.ReadFile(metaPath); rErr == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	if meta.ID == "" {
		meta = ItemMetadata{
			ID:           item.ID,
			Hex:          item.Hex,
			Name:         item.Name,
			Extension:    item.Extension,
			Filename:     item.Filename,
			Size:         item.Size,
			MimeType:     item.MimeType,
			Width:        item.Width,
			Height:       item.Height,
			HasThumbnail: item.HasThumbnail,
			Palette:      item.Palette,
			Meta:         item.Meta,
			Note:         item.Note,
			URL:          item.URL,
			Rating:       item.Rating,
			Prompt:       item.Prompt,
			Tags:         item.Tags,
			Folders:      item.Folders,
			CreatedAt:    item.CreatedAt,
			ImportedAt:   item.ImportedAt,
			UpdatedAt:    item.UpdatedAt,
		}
	}

	// 1. Move the old active file to a collision-safe backup name in the SAME directory.
	versionID := generateVersionID()
	oldExt := strings.ToLower(strings.TrimPrefix(item.Extension, "."))
	if oldExt == "" {
		oldExt = strings.ToLower(strings.TrimPrefix(filepath.Ext(item.Filename), "."))
	}
	backupFilename := versionFilePrefix + versionID
	if oldExt != "" {
		backupFilename += "." + oldExt
	}
	backupPath := filepath.Join(itemDir, backupFilename)
	if err := os.Rename(oldPath, backupPath); err != nil {
		return nil, fmt.Errorf("failed to backup existing file: %w", err)
	}

	// 2. Write the new active file into the same directory.
	newExt := detectImageExt(newData, opts.Extension)
	newFilename := buildActiveFilename(item.Name, item.Filename, newExt)
	// Avoid clobbering the backup file name in the unlikely event they match.
	if strings.EqualFold(newFilename, backupFilename) {
		newFilename = "active-" + newFilename
	}
	newPath := filepath.Join(itemDir, newFilename)
	if err := os.WriteFile(newPath, newData, 0644); err != nil {
		// Rollback backup move on failure.
		_ = os.Rename(backupPath, oldPath)
		return nil, fmt.Errorf("failed to write replacement file: %w", err)
	}

	// 3. Record the backed-up version entry (newest first).
	now := time.Now().Unix()
	backupVersion := ItemVersion{
		ID:        versionID,
		Filename:  backupFilename,
		Extension: oldExt,
		MimeType:  item.MimeType,
		Size:      oldStat.Size(),
		Width:     item.Width,
		Height:    item.Height,
		Hex:       item.Hex,
		CreatedAt: now,
		Note:      item.Note,
		Prompt:    item.Prompt,
		Source:    strings.TrimSpace(opts.Source),
	}
	meta.Versions = append([]ItemVersion{backupVersion}, meta.Versions...)

	// 4. Recompute hash, MIME, dimensions, thumbnail, palette, and extended metadata.
	sum := sha256.Sum256(newData)
	newHex := strings.ToUpper(hex.EncodeToString(sum[:]))
	newSize := int64(len(newData))
	newMime := DetectMimeTypeWithHeader(newPath, newExt, newData)
	w, h, hasThumb, dominantColor, palette, imgMeta := m.processImageThumbnail(newPath, itemDir, newExt)
	if palette == nil {
		palette = []string{}
	}

	newPrompt := item.Prompt
	if strings.TrimSpace(opts.Prompt) != "" {
		newPrompt = strings.TrimSpace(opts.Prompt)
	}
	newNote := item.Note
	if strings.TrimSpace(opts.Note) != "" {
		newNote = strings.TrimSpace(opts.Note)
	}

	paletteStr := strings.Join(palette, ",")
	metaJSONStr := ""
	if imgMeta != nil {
		if b, mErr := json.Marshal(imgMeta); mErr == nil {
			metaJSONStr = string(b)
		}
	}
	hasThumbInt := 0
	if hasThumb {
		hasThumbInt = 1
	}

	_, err = db.Exec(
		`UPDATE items SET hex = ?, extension = ?, filename = ?, size = ?, mime_type = ?, width = ?, height = ?, has_thumbnail = ?, dominant_color = ?, palette = ?, meta_json = ?, prompt = ?, note = ?, updated_at = ? WHERE id = ?`,
		newHex, newExt, newFilename, newSize, newMime, w, h, hasThumbInt, dominantColor, paletteStr, metaJSONStr, newPrompt, newNote, now, item.ID,
	)
	if err != nil {
		// Best-effort rollback on DB failure.
		_ = os.Remove(newPath)
		_ = os.Rename(backupPath, oldPath)
		return nil, fmt.Errorf("failed to update item record: %w", err)
	}

	// 5. Persist extended metadata.json.
	meta.Hex = newHex
	meta.Extension = newExt
	meta.Filename = newFilename
	meta.Size = newSize
	meta.MimeType = newMime
	meta.Width = w
	meta.Height = h
	meta.HasThumbnail = hasThumb
	meta.DominantColor = dominantColor
	meta.Palette = palette
	meta.Meta = imgMeta
	meta.Prompt = newPrompt
	meta.Note = newNote
	meta.UpdatedAt = now
	if out, mErr := json.MarshalIndent(meta, "", "  "); mErr == nil {
		_ = os.WriteFile(metaPath, out, 0644)
	}

	item.Hex = newHex
	item.Extension = newExt
	item.Filename = newFilename
	item.Size = newSize
	item.MimeType = newMime
	item.Width = w
	item.Height = h
	item.HasThumbnail = hasThumb
	item.DominantColor = dominantColor
	item.Palette = palette
	item.Meta = imgMeta
	item.Prompt = newPrompt
	item.Note = newNote
	item.UpdatedAt = now
	item.Versions = append([]ItemVersion(nil), meta.Versions...)
	m.populateItemPaths(item, active.Path, port)
	m.upsertItemsInCache([]Item{*item}, "")
	m.emit("library-items-upserted", []Item{*item})
	m.notifyItemsUpdated()

	return item, nil
}

// RestoreItemVersion restores a previously backed-up version as the item's active
// file, saving the current active file as a new backup version so restoration is
// completely reversible.
func (m *LibraryManager) RestoreItemVersion(itemID string, versionID string) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	port := m.GetAssetServerPort()
	m.mu.RUnlock()

	if active == nil || db == nil {
		return nil, errors.New("no active library")
	}
	if strings.TrimSpace(versionID) == "" {
		return nil, errors.New("version id is required")
	}

	itemLock := m.acquireItemLock(itemID)
	defer itemLock.Unlock()

	item, err := m.GetItem(itemID)
	if err != nil || item == nil {
		return nil, errors.New("item not found")
	}

	itemDir := filepath.Join(active.Path, "items", item.ID)
	metaPath := filepath.Join(itemDir, itemMetadataName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("metadata.json not found: %w", err)
	}
	var meta ItemMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("invalid metadata.json: %w", err)
	}

	targetIdx := -1
	for i, v := range meta.Versions {
		if v.ID == versionID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return nil, errors.New("version not found")
	}
	target := meta.Versions[targetIdx]
	targetPath := filepath.Join(itemDir, target.Filename)
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("backup file missing: %w", err)
	}

	// Backup the current active file first.
	curPath := filepath.Join(itemDir, item.Filename)
	curStat, curStatErr := os.Stat(curPath)
	now := time.Now().Unix()

	var currentBackup *ItemVersion
	if curStatErr == nil {
		curBackupID := generateVersionID()
		curExt := strings.ToLower(strings.TrimPrefix(item.Extension, "."))
		curBackupName := versionFilePrefix + curBackupID
		if curExt != "" {
			curBackupName += "." + curExt
		}
		curBackupPath := filepath.Join(itemDir, curBackupName)
		if err := os.Rename(curPath, curBackupPath); err != nil {
			return nil, fmt.Errorf("failed to backup current file before restore: %w", err)
		}
		currentBackup = &ItemVersion{
			ID:        curBackupID,
			Filename:  curBackupName,
			Extension: curExt,
			MimeType:  item.MimeType,
			Size:      curStat.Size(),
			Width:     item.Width,
			Height:    item.Height,
			Hex:       item.Hex,
			CreatedAt: now,
			Note:      item.Note,
			Prompt:    item.Prompt,
			Source:    "before-restore",
		}
	}

	restoredExt := detectImageExt(targetData, target.Extension)
	newFilename := buildActiveFilename(item.Name, item.Filename, restoredExt)
	newPath := filepath.Join(itemDir, newFilename)
	if err := os.Rename(targetPath, newPath); err != nil {
		// Fallback to copy + remove if rename fails.
		if wErr := os.WriteFile(newPath, targetData, 0644); wErr != nil {
			return nil, fmt.Errorf("failed to restore version file: %w", wErr)
		}
		_ = os.Remove(targetPath)
	}

	// Update version list: remove restored entry, prepend backup of previous active file.
	remaining := make([]ItemVersion, 0, len(meta.Versions))
	if currentBackup != nil {
		remaining = append(remaining, *currentBackup)
	}
	for i, v := range meta.Versions {
		if i == targetIdx {
			continue
		}
		remaining = append(remaining, v)
	}
	meta.Versions = remaining

	sum := sha256.Sum256(targetData)
	newHex := strings.ToUpper(hex.EncodeToString(sum[:]))
	newSize := int64(len(targetData))
	newMime := DetectMimeTypeWithHeader(newPath, restoredExt, targetData)
	w, h, hasThumb, dominantColor, palette, imgMeta := m.processImageThumbnail(newPath, itemDir, restoredExt)
	if palette == nil {
		palette = []string{}
	}

	paletteStr := strings.Join(palette, ",")
	metaJSONStr := ""
	if imgMeta != nil {
		if b, mErr := json.Marshal(imgMeta); mErr == nil {
			metaJSONStr = string(b)
		}
	}
	hasThumbInt := 0
	if hasThumb {
		hasThumbInt = 1
	}

	restoredPrompt := item.Prompt
	if target.Prompt != "" {
		restoredPrompt = target.Prompt
	}

	_, err = db.Exec(
		`UPDATE items SET hex = ?, extension = ?, filename = ?, size = ?, mime_type = ?, width = ?, height = ?, has_thumbnail = ?, dominant_color = ?, palette = ?, meta_json = ?, prompt = ?, updated_at = ? WHERE id = ?`,
		newHex, restoredExt, newFilename, newSize, newMime, w, h, hasThumbInt, dominantColor, paletteStr, metaJSONStr, restoredPrompt, now, item.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update restored item record: %w", err)
	}

	meta.Hex = newHex
	meta.Extension = restoredExt
	meta.Filename = newFilename
	meta.Size = newSize
	meta.MimeType = newMime
	meta.Width = w
	meta.Height = h
	meta.HasThumbnail = hasThumb
	meta.DominantColor = dominantColor
	meta.Palette = palette
	meta.Meta = imgMeta
	meta.Prompt = restoredPrompt
	meta.UpdatedAt = now
	if out, mErr := json.MarshalIndent(meta, "", "  "); mErr == nil {
		_ = os.WriteFile(metaPath, out, 0644)
	}

	item.Hex = newHex
	item.Extension = restoredExt
	item.Filename = newFilename
	item.Size = newSize
	item.MimeType = newMime
	item.Width = w
	item.Height = h
	item.HasThumbnail = hasThumb
	item.DominantColor = dominantColor
	item.Palette = palette
	item.Meta = imgMeta
	item.Prompt = restoredPrompt
	item.UpdatedAt = now
	item.Versions = append([]ItemVersion(nil), meta.Versions...)
	m.populateItemPaths(item, active.Path, port)
	m.upsertItemsInCache([]Item{*item}, "")
	m.emit("library-items-upserted", []Item{*item})
	m.notifyItemsUpdated()

	return item, nil
}

// DeleteItemVersion permanently removes one historical backup file from items/<id>/
// and strips its entry from metadata.json.
func (m *LibraryManager) DeleteItemVersion(itemID string, versionID string) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	port := m.GetAssetServerPort()
	m.mu.RUnlock()

	if active == nil {
		return nil, errors.New("no active library")
	}
	if strings.TrimSpace(versionID) == "" {
		return nil, errors.New("version id is required")
	}

	itemLock := m.acquireItemLock(itemID)
	defer itemLock.Unlock()

	item, err := m.GetItem(itemID)
	if err != nil || item == nil {
		return nil, errors.New("item not found")
	}

	itemDir := filepath.Join(active.Path, "items", item.ID)
	metaPath := filepath.Join(itemDir, itemMetadataName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("metadata.json not found: %w", err)
	}
	var meta ItemMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("invalid metadata.json: %w", err)
	}

	remaining := make([]ItemVersion, 0, len(meta.Versions))
	found := false
	for _, v := range meta.Versions {
		if v.ID == versionID {
			found = true
			// Safety guard: never remove outside itemDir or remove the active file.
			cleanName := filepath.Base(v.Filename)
			if cleanName != "" && cleanName == v.Filename && cleanName != item.Filename {
				_ = os.Remove(filepath.Join(itemDir, cleanName))
			}
			continue
		}
		remaining = append(remaining, v)
	}
	if !found {
		return nil, errors.New("version not found")
	}

	meta.Versions = remaining
	meta.UpdatedAt = time.Now().Unix()
	if out, mErr := json.MarshalIndent(meta, "", "  "); mErr == nil {
		_ = os.WriteFile(metaPath, out, 0644)
	}

	item.Versions = append([]ItemVersion(nil), remaining...)
	m.populateItemPaths(item, active.Path, port)
	m.mutateCachedItem(item.ID, func(it *Item) {
		it.Versions = append([]ItemVersion(nil), remaining...)
	})
	m.emit("library-items-upserted", []Item{*item})
	m.notifyItemsUpdated()

	return item, nil
}
