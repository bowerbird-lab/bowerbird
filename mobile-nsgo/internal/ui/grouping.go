package ui

import (
	"time"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
)

// RowKind distinguishes a month header, a subfolder header, a row of subfolder
// cards, or a row of gallery tiles.
type RowKind uint8

const (
	RowMonthHeader RowKind = iota
	RowTileGroup
	RowFolderHeader
	RowFolderGroup
)

// TileSlot is one item cell inside a RowTileGroup along with its flat index in
// the underlying Gallery items slice.
type TileSlot struct {
	FlatIndex int
	Item      model.ItemSummary
}

// GalleryRow is one virtualized row in core.List.
type GalleryRow struct {
	Key       string
	Kind      RowKind
	Title     string
	Count     int
	Collapsed bool
	Folders   []model.Folder
	Slots     []TileSlot
}

// BuildMonthRows groups items by month (preserving input order) and packs each
// month's items into rows of cols columns. Each item retains its stable flat
// index so detail navigation maps 1:1.
func BuildMonthRows(items []model.ItemSummary, cols int) []GalleryRow {
	if cols < 2 {
		cols = 2
	}
	if cols > 5 {
		cols = 5
	}
	if len(items) == 0 {
		return nil
	}
	type monthBucket struct {
		key   string
		slots []TileSlot
	}
	var buckets []*monthBucket
	byKey := map[string]*monthBucket{}
	for idx, item := range items {
		label := monthLabel(item.BestDate())
		b, ok := byKey[label]
		if !ok {
			b = &monthBucket{key: label}
			byKey[label] = b
			buckets = append(buckets, b)
		}
		b.slots = append(b.slots, TileSlot{FlatIndex: idx, Item: item})
	}

	rows := make([]GalleryRow, 0, len(buckets)+len(items)/cols+1)
	for _, b := range buckets {
		rows = append(rows, GalleryRow{
			Key:   "h:" + b.key,
			Kind:  RowMonthHeader,
			Title: b.key,
			Count: len(b.slots),
		})
		for start := 0; start < len(b.slots); start += cols {
			end := start + cols
			if end > len(b.slots) {
				end = len(b.slots)
			}
			chunk := b.slots[start:end]
			rows = append(rows, GalleryRow{
				Key:   "r:" + chunk[0].Item.ID,
				Kind:  RowTileGroup,
				Slots: chunk,
			})
		}
	}
	return rows
}

// BuildFlatRows packs items into rows of cols columns without month headers
// (used by search/history views).
func BuildFlatRows(items []model.ItemSummary, cols int) []GalleryRow {
	if cols < 2 {
		cols = 2
	}
	if cols > 5 {
		cols = 5
	}
	if len(items) == 0 {
		return nil
	}
	rows := make([]GalleryRow, 0, len(items)/cols+1)
	for start := 0; start < len(items); start += cols {
		end := start + cols
		if end > len(items) {
			end = len(items)
		}
		slots := make([]TileSlot, 0, end-start)
		for idx := start; idx < end; idx++ {
			slots = append(slots, TileSlot{FlatIndex: idx, Item: items[idx]})
		}
		rows = append(rows, GalleryRow{
			Key:   "r:" + slots[0].Item.ID,
			Kind:  RowTileGroup,
			Slots: slots,
		})
	}
	return rows
}

// BuildGalleryRows prepends the current folder's subfolders (if any) ahead of
// the flat item rows.
func BuildGalleryRows(subfolders []model.Folder, collapsed bool, items []model.ItemSummary, cols int) []GalleryRow {
	if cols < 2 {
		cols = 2
	}
	if cols > 5 {
		cols = 5
	}
	var rows []GalleryRow
	if len(subfolders) > 0 {
		rows = append(rows, GalleryRow{
			Key:       "fh",
			Kind:      RowFolderHeader,
			Title:     "子文件夹 (" + itoa(len(subfolders)) + ")",
			Count:     len(subfolders),
			Collapsed: collapsed,
		})
		if !collapsed {
			for start := 0; start < len(subfolders); start += cols {
				end := start + cols
				if end > len(subfolders) {
					end = len(subfolders)
				}
				chunk := subfolders[start:end]
				rows = append(rows, GalleryRow{
					Key:     "fr:" + chunk[0].ID,
					Kind:    RowFolderGroup,
					Folders: chunk,
				})
			}
		}
	}
	itemRows := BuildFlatRows(items, cols)
	if len(rows) == 0 {
		return itemRows
	}
	return append(rows, itemRows...)
}

// FirstItemIDAtRow returns the first item ID at or after rowIdx, for anchor
// preservation across density changes.
func FirstItemIDAtRow(rows []GalleryRow, rowIdx int) string {
	if rowIdx < 0 {
		rowIdx = 0
	}
	for i := rowIdx; i < len(rows); i++ {
		if rows[i].Kind == RowTileGroup && len(rows[i].Slots) > 0 {
			return rows[i].Slots[0].Item.ID
		}
	}
	return ""
}

// RowIndexForItemID finds the row containing itemID, or 0 if absent.
func RowIndexForItemID(rows []GalleryRow, itemID string) int {
	if itemID == "" {
		return 0
	}
	for i, r := range rows {
		for _, s := range r.Slots {
			if s.Item.ID == itemID {
				return i
			}
		}
	}
	return 0
}

func monthLabel(ms int64) string {
	if ms < 946684800000 {
		return "较早"
	}
	return time.UnixMilli(ms).Format("2006 年 01 月")
}
