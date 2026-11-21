package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *LibraryManager) updateItemMetadataVariants(libPath, itemID string, variants []string, now int64) error {
	metaPath := filepath.Join(libPath, "items", itemID, itemMetadataName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	var meta ItemMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	if len(variants) > 0 {
		meta.Variants = append([]string(nil), variants...)
	} else {
		meta.Variants = nil
	}
	meta.UpdatedAt = now
	out, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, out, 0o644)
}

func marshalVariantsJSON(variants []string) string {
	if len(variants) == 0 {
		return ""
	}
	b, err := json.Marshal(variants)
	if err != nil {
		return ""
	}
	return string(b)
}

// LinkVariants attaches one or more existing library items as variants of parentID.
// Any file type can be linked (images, videos, audio, documents, etc.).
// If parentID is itself a variant, the items are linked to its root primary item.
func (m *LibraryManager) LinkVariants(parentID string, variantIDs []string) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	m.mu.RUnlock()
	if active == nil || db == nil {
		return nil, errors.New("no active library")
	}

	parent, err := m.GetItem(parentID)
	if err != nil || parent == nil {
		return nil, errors.New("parent item not found")
	}
	if parent.InTrash {
		return nil, errors.New("cannot link variants to a trashed item")
	}
	if parent.VariantOf != "" {
		if root, rErr := m.GetItem(parent.VariantOf); rErr == nil && root != nil && !root.InTrash {
			parent = root
		}
	}

	existing := make(map[string]bool, len(parent.Variants)+1)
	existing[parent.ID] = true
	nextVariants := make([]string, 0, len(parent.Variants)+len(variantIDs))
	for _, vid := range parent.Variants {
		if vid == "" || existing[vid] {
			continue
		}
		existing[vid] = true
		nextVariants = append(nextVariants, vid)
	}

	var toAdd []string
	var flattenSources []*Item
	oldParentRemovals := make(map[string]map[string]bool)

	for _, vid := range variantIDs {
		vid = strings.TrimSpace(vid)
		if vid == "" || existing[vid] {
			continue
		}
		vItem, vErr := m.GetItem(vid)
		if vErr != nil || vItem == nil || vItem.InTrash {
			continue
		}
		existing[vid] = true
		toAdd = append(toAdd, vid)
		if vItem.VariantOf != "" && vItem.VariantOf != parent.ID {
			if oldParentRemovals[vItem.VariantOf] == nil {
				oldParentRemovals[vItem.VariantOf] = make(map[string]bool)
			}
			oldParentRemovals[vItem.VariantOf][vid] = true
		}
		if len(vItem.Variants) > 0 {
			flattenSources = append(flattenSources, vItem)
			for _, subID := range vItem.Variants {
				subID = strings.TrimSpace(subID)
				if subID == "" || existing[subID] {
					continue
				}
				subItem, subErr := m.GetItem(subID)
				if subErr != nil || subItem == nil || subItem.InTrash {
					continue
				}
				existing[subID] = true
				toAdd = append(toAdd, subID)
			}
		}
	}

	if len(toAdd) == 0 {
		return parent, nil
	}
	nextVariants = append(nextVariants, toAdd...)
	now := time.Now().Unix()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	// Remove moved variants from any previous parents.
	for oldParentID, removedSet := range oldParentRemovals {
		oldParent, opErr := m.GetItem(oldParentID)
		if opErr != nil || oldParent == nil {
			continue
		}
		filtered := make([]string, 0, len(oldParent.Variants))
		for _, id := range oldParent.Variants {
			if !removedSet[id] {
				filtered = append(filtered, id)
			}
		}
		_, _ = tx.Exec("UPDATE items SET variants_json = ?, updated_at = ? WHERE id = ?", marshalVariantsJSON(filtered), now, oldParentID)
		_ = m.updateItemMetadataVariants(active.Path, oldParentID, filtered, now)
		m.mutateCachedItem(oldParentID, func(it *Item) {
			it.Variants = filtered
			it.UpdatedAt = now
		})
	}

	// Clear Variants on flattened sources.
	for _, src := range flattenSources {
		_, _ = tx.Exec("UPDATE items SET variants_json = '', updated_at = ? WHERE id = ?", now, src.ID)
		_ = m.updateItemMetadataVariants(active.Path, src.ID, nil, now)
		m.mutateCachedItem(src.ID, func(it *Item) {
			it.Variants = nil
			it.UpdatedAt = now
		})
	}

	// Persist parent's updated Variants list.
	if _, err := tx.Exec("UPDATE items SET variants_json = ?, variant_of = '', updated_at = ? WHERE id = ?", marshalVariantsJSON(nextVariants), now, parent.ID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for _, vid := range toAdd {
		if _, err := tx.Exec("UPDATE items SET variant_of = ?, updated_at = ? WHERE id = ?", parent.ID, now, vid); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if err := m.updateItemMetadataVariants(active.Path, parent.ID, nextVariants, now); err != nil {
		return nil, err
	}

	var upserted []Item
	m.mutateCachedItem(parent.ID, func(it *Item) {
		it.Variants = append([]string(nil), nextVariants...)
		it.VariantOf = ""
		it.UpdatedAt = now
	})
	if updatedParent, gErr := m.GetItem(parent.ID); gErr == nil && updatedParent != nil {
		parent = updatedParent
		upserted = append(upserted, *updatedParent)
	}
	for _, vid := range toAdd {
		m.mutateCachedItem(vid, func(it *Item) {
			it.VariantOf = parent.ID
			it.UpdatedAt = now
		})
		if vIt, gErr := m.GetItem(vid); gErr == nil && vIt != nil {
			upserted = append(upserted, *vIt)
		}
	}
	if len(upserted) > 0 {
		m.emit("library-items-upserted", upserted)
	}
	m.notifyItemsUpdated()
	return parent, nil
}

// UnlinkVariant detaches variantID from parentID so it becomes an independent item again.
func (m *LibraryManager) UnlinkVariant(parentID string, variantID string) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	m.mu.RUnlock()
	if active == nil || db == nil {
		return nil, errors.New("no active library")
	}

	parent, err := m.GetItem(parentID)
	if err != nil || parent == nil {
		return nil, errors.New("parent item not found")
	}

	filtered := make([]string, 0, len(parent.Variants))
	removed := false
	for _, vid := range parent.Variants {
		if vid == variantID {
			removed = true
			continue
		}
		filtered = append(filtered, vid)
	}
	if !removed {
		return parent, nil
	}

	now := time.Now().Unix()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec("UPDATE items SET variants_json = ?, updated_at = ? WHERE id = ?", marshalVariantsJSON(filtered), now, parent.ID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.Exec("UPDATE items SET variant_of = '', updated_at = ? WHERE id = ?", now, variantID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if err := m.updateItemMetadataVariants(active.Path, parent.ID, filtered, now); err != nil {
		return nil, err
	}

	var upserted []Item
	m.mutateCachedItem(parent.ID, func(it *Item) {
		it.Variants = filtered
		it.UpdatedAt = now
	})
	if updatedParent, gErr := m.GetItem(parent.ID); gErr == nil && updatedParent != nil {
		parent = updatedParent
		upserted = append(upserted, *updatedParent)
	}
	m.mutateCachedItem(variantID, func(it *Item) {
		it.VariantOf = ""
		it.UpdatedAt = now
	})
	if vIt, gErr := m.GetItem(variantID); gErr == nil && vIt != nil {
		upserted = append(upserted, *vIt)
	}
	if len(upserted) > 0 {
		m.emit("library-items-upserted", upserted)
	}
	m.notifyItemsUpdated()
	return parent, nil
}

// SetPrimaryVariant promotes newPrimaryID (currently a variant of parentID) to become
// the primary item of the variant group.
func (m *LibraryManager) SetPrimaryVariant(parentID string, newPrimaryID string) (*Item, error) {
	m.mu.RLock()
	active := m.activeLib
	db := m.db
	m.mu.RUnlock()
	if active == nil || db == nil {
		return nil, errors.New("no active library")
	}
	if parentID == "" || newPrimaryID == "" || parentID == newPrimaryID {
		return nil, errors.New("invalid variant ids")
	}

	parent, err := m.GetItem(parentID)
	if err != nil || parent == nil {
		return nil, errors.New("parent item not found")
	}
	found := false
	others := make([]string, 0, len(parent.Variants))
	for _, vid := range parent.Variants {
		if vid == newPrimaryID {
			found = true
			continue
		}
		if vid != "" && vid != parentID {
			others = append(others, vid)
		}
	}
	if !found {
		return nil, errors.New("target item is not a variant of parent")
	}

	newPrimary, err := m.GetItem(newPrimaryID)
	if err != nil || newPrimary == nil || newPrimary.InTrash {
		return nil, errors.New("new primary item not found")
	}

	nextVariants := make([]string, 0, 1+len(others))
	nextVariants = append(nextVariants, parent.ID)
	nextVariants = append(nextVariants, others...)
	now := time.Now().Unix()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec("UPDATE items SET variants_json = '', variant_of = ?, updated_at = ? WHERE id = ?", newPrimary.ID, now, parent.ID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.Exec("UPDATE items SET variants_json = ?, variant_of = '', updated_at = ? WHERE id = ?", marshalVariantsJSON(nextVariants), now, newPrimary.ID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for _, vid := range others {
		if _, err := tx.Exec("UPDATE items SET variant_of = ?, updated_at = ? WHERE id = ?", newPrimary.ID, now, vid); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if err := m.updateItemMetadataVariants(active.Path, parent.ID, nil, now); err != nil {
		return nil, err
	}
	if err := m.updateItemMetadataVariants(active.Path, newPrimary.ID, nextVariants, now); err != nil {
		return nil, err
	}

	var upserted []Item
	m.mutateCachedItem(parent.ID, func(it *Item) {
		it.Variants = nil
		it.VariantOf = newPrimary.ID
		it.UpdatedAt = now
	})
	if oldP, gErr := m.GetItem(parent.ID); gErr == nil && oldP != nil {
		upserted = append(upserted, *oldP)
	}
	m.mutateCachedItem(newPrimary.ID, func(it *Item) {
		it.Variants = append([]string(nil), nextVariants...)
		it.VariantOf = ""
		it.UpdatedAt = now
	})
	if np, gErr := m.GetItem(newPrimary.ID); gErr == nil && np != nil {
		newPrimary = np
		upserted = append(upserted, *np)
	}
	for _, vid := range others {
		m.mutateCachedItem(vid, func(it *Item) {
			it.VariantOf = newPrimary.ID
			it.UpdatedAt = now
		})
		if vIt, gErr := m.GetItem(vid); gErr == nil && vIt != nil {
			upserted = append(upserted, *vIt)
		}
	}
	if len(upserted) > 0 {
		m.emit("library-items-upserted", upserted)
	}
	m.notifyItemsUpdated()
	return newPrimary, nil
}

// GetVariantGroup resolves the root primary item and all active variant items for itemID.
func (m *LibraryManager) GetVariantGroup(itemID string) (*Item, []Item, error) {
	item, err := m.GetItem(itemID)
	if err != nil || item == nil {
		return nil, nil, errors.New("item not found")
	}
	primary := item
	if item.VariantOf != "" {
		if root, rErr := m.GetItem(item.VariantOf); rErr == nil && root != nil && !root.InTrash {
			primary = root
		}
	}
	if len(primary.Variants) == 0 {
		return primary, nil, nil
	}
	variants := make([]Item, 0, len(primary.Variants))
	for _, vid := range primary.Variants {
		vIt, vErr := m.GetItem(vid)
		if vErr != nil || vIt == nil || vIt.InTrash {
			continue
		}
		variants = append(variants, *vIt)
	}
	return primary, variants, nil
}

// AddVariantFromFile imports any file on disk and links it as a variant of parentID.
func (m *LibraryManager) AddVariantFromFile(parentID string, sourcePath string) (*Item, error) {
	imported, err := m.importSingleFile(sourcePath)
	if err != nil {
		return nil, err
	}
	if imported == nil {
		return nil, errors.New("failed to import variant file")
	}
	if _, err := m.LinkVariants(parentID, []string{imported.ID}); err != nil {
		return nil, err
	}
	return m.GetItem(imported.ID)
}

// SaveVariantFromBytes imports arbitrary file bytes (any extension/type) and links the
// resulting item as a variant of parentID.
func (m *LibraryManager) SaveVariantFromBytes(parentID string, data []byte, filename string, opts ImportItemOptions) (*Item, error) {
	if len(data) == 0 {
		return nil, errors.New("variant file data is empty")
	}
	cleanName := strings.TrimSpace(filepath.Base(filename))
	if cleanName == "" || cleanName == "." || cleanName == string(filepath.Separator) {
		ext := detectImageExt(data, "")
		cleanName = fmt.Sprintf("variant-%d.%s", time.Now().Unix(), ext)
	} else if filepath.Ext(cleanName) == "" {
		ext := detectImageExt(data, "")
		cleanName = cleanName + "." + ext
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	imported, err := m.ImportFromBase64WithOptions(cleanName, b64, opts)
	if err != nil {
		return nil, err
	}
	if imported == nil {
		return nil, errors.New("failed to import variant")
	}
	if _, err := m.LinkVariants(parentID, []string{imported.ID}); err != nil {
		return nil, err
	}
	return m.GetItem(imported.ID)
}

// cleanupItemVariantLinks removes deletedID from its parent's Variants list or
// detaches its children if deletedID was a primary item.
func (m *LibraryManager) cleanupItemVariantLinks(deletedID string) {
	it, err := m.GetItem(deletedID)
	if err != nil || it == nil {
		return
	}
	if it.VariantOf != "" {
		_, _ = m.UnlinkVariant(it.VariantOf, deletedID)
	}
	if len(it.Variants) > 0 {
		for _, vid := range it.Variants {
			_, _ = m.UnlinkVariant(deletedID, vid)
		}
	}
}
