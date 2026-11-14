package core

import (
	"context"
	"encoding/json"
	"strings"
)

const storedItemColumns = `i.id, COALESCE(i.hex,''), i.name, i.extension, i.filename, i.size,
	i.mime_type, i.width, i.height, i.has_thumbnail, COALESCE(i.tags,''), i.created_at,
	i.imported_at, COALESCE(i.updated_at,0), COALESCE(i.note,''), COALESCE(i.url,''),
	COALESCE(i.rating,0), COALESCE(i.prompt,''), COALESCE(i.in_trash,0),
	COALESCE(i.trashed_at,0), COALESCE(i.dominant_color,''), COALESCE(i.palette,''),
	COALESCE(i.meta_json,''), CASE WHEN i.in_trash=1 THEN COALESCE(NULLIF(i.trashed_folders,''),'[]')
	ELSE (SELECT json_group_array(folder_id) FROM item_folders WHERE item_id=i.id) END`

func scanStoredItem(row interface{ Scan(...any) error }) (Item, error) {
	item := Item{Tags: []string{}, Palette: []string{}, Folders: []string{}}
	var tags, palette, metadata, folders string
	var thumbnail, trash int
	err := row.Scan(
		&item.ID, &item.Hex, &item.Name, &item.Extension, &item.Filename, &item.Size,
		&item.MimeType, &item.Width, &item.Height, &thumbnail, &tags, &item.CreatedAt,
		&item.ImportedAt, &item.UpdatedAt, &item.Note, &item.URL, &item.Rating,
		&item.Prompt, &trash, &item.TrashedAt, &item.DominantColor, &palette, &metadata, &folders,
	)
	if err != nil {
		return item, err
	}
	item.HasThumbnail = thumbnail != 0
	item.InTrash = trash != 0
	if item.UpdatedAt <= 0 {
		item.UpdatedAt = item.ImportedAt
	}
	if tags != "" {
		item.Tags = strings.Split(tags, ",")
	}
	if palette != "" {
		item.Palette = strings.Split(palette, ",")
	}
	if metadata != "" {
		_ = json.Unmarshal([]byte(metadata), &item.Meta)
	}
	_ = json.Unmarshal([]byte(folders), &item.Folders)
	return item, nil
}

func diskItemScope(trash bool, folders []string) (string, []any) {
	where := "COALESCE(i.in_trash,0)=0"
	if trash {
		where = "i.in_trash=1"
	}
	var args []any
	if len(folders) > 0 {
		placeholders := make([]string, len(folders))
		for index, folder := range folders {
			placeholders[index] = "?"
			args = append(args, folder)
		}
		where += " AND i.id IN (SELECT f.item_id FROM item_folders f WHERE f.folder_id IN (" + strings.Join(placeholders, ",") + "))"
	}
	return where, args
}

func diskItemOrder(sortOrder string, trash bool) string {
	direction := " DESC"
	if strings.EqualFold(sortOrder, "asc") {
		direction = " ASC"
	}
	if trash {
		return "i.trashed_at" + direction + ", i.imported_at" + direction + ", i.id" + direction
	}
	return "i.imported_at" + direction + ", i.id" + direction
}

func simpleWorkspaceQuery(spec QuerySpec) bool {
	return strings.TrimSpace(spec.Text) == "" &&
		len(spec.Types)+len(spec.Tags)+len(spec.Folders)+len(spec.Colors)+len(spec.Orientations)+len(spec.Sizes)+len(spec.Ratings)+len(spec.Has)+len(spec.No) == 0 &&
		!spec.Untagged && !spec.Uncategorized
}

func (m *LibraryManager) diskQuery(spec QuerySpec, sortOrder string, scope []string, unrated, trash, compact bool, visit func(Item)) error {
	return m.diskQueryContext(context.Background(), spec, sortOrder, scope, unrated, trash, compact, visit)
}

func (m *LibraryManager) diskQueryContext(ctx context.Context, spec QuerySpec, sortOrder string, scope []string, unrated, trash, compact bool, visit func(Item)) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil || m.activeLib == nil {
		return nil
	}
	where, args := diskItemScope(trash, scope)
	if unrated {
		where += " AND COALESCE(i.rating,0)=0"
	}
	text := strings.ToLower(strings.TrimSpace(spec.Text))
	simple := simpleWorkspaceQuery(spec)
	columns := storedItemColumns
	if compact && simple {
		columns = "i.id, i.name, i.extension, i.size, i.mime_type, i.width, i.height, i.has_thumbnail, COALESCE(i.rating,0)"
	}
	rows, err := m.db.QueryContext(ctx, "SELECT "+columns+" FROM items i WHERE "+where+" ORDER BY "+diskItemOrder(sortOrder, trash), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		if compact && simple {
			var thumbnail int
			err = rows.Scan(&item.ID, &item.Name, &item.Extension, &item.Size, &item.MimeType, &item.Width, &item.Height, &thumbnail, &item.Rating)
			item.HasThumbnail = thumbnail != 0
			item.InTrash = trash
		} else {
			item, err = scanStoredItem(rows)
		}
		if err != nil {
			return err
		}
		if !simple && (!spec.matches(&item) || text != "" && !strings.Contains(buildItemSearchKey(&item), text)) {
			continue
		}
		if compact {
			item = compactWorkspaceItem(item)
		} else {
			m.populateItemPaths(&item, m.activeLib.Path, m.GetAssetServerPort())
		}
		visit(item)
	}
	return rows.Err()
}

func compactWorkspaceItem(item Item) Item {
	return Item{
		ID: item.ID, Name: item.Name, Extension: item.Extension, MimeType: item.MimeType,
		Width: item.Width, Height: item.Height, Size: item.Size, Rating: item.Rating,
		HasThumbnail: item.HasThumbnail, InTrash: item.InTrash,
	}
}

func (m *LibraryManager) WorkspaceIndex(spec QuerySpec, sortOrder string, scope []string, unrated, trash bool) (QueryPage, error) {
	return m.WorkspaceIndexContext(context.Background(), spec, sortOrder, scope, unrated, trash)
}

func (m *LibraryManager) WorkspaceIndexContext(ctx context.Context, spec QuerySpec, sortOrder string, scope []string, unrated, trash bool) (QueryPage, error) {
	page := QueryPage{Items: []Item{}}
	if simpleWorkspaceQuery(spec) {
		m.mu.RLock()
		if m.db != nil {
			where, args := diskItemScope(trash, scope)
			if unrated {
				where += " AND COALESCE(i.rating,0)=0"
			}
			var count int
			if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items i WHERE "+where, args...).Scan(&count); err == nil {
				page.Items = make([]Item, 0, count)
			}
		}
		m.mu.RUnlock()
	}
	err := m.diskQueryContext(ctx, spec, sortOrder, scope, unrated, trash, true, func(item Item) {
		page.Items = append(page.Items, item)
		page.Total++
		page.TotalSize += item.Size
	})
	return page, err
}

func (m *LibraryManager) WorkspaceItems(ids []string) ([]Item, error) {
	return m.workspaceItems(ids, false)
}

func (m *LibraryManager) workspaceItems(ids []string, cards bool) ([]Item, error) {
	if len(ids) == 0 {
		return []Item{}, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil || m.activeLib == nil {
		return []Item{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index], args[index] = "?", id
	}
	columns := storedItemColumns
	if cards {
		columns = `i.id, COALESCE(i.hex,''), i.name, i.extension, i.filename, i.size,
			i.mime_type, i.width, i.height, i.has_thumbnail, '', i.created_at,
			i.imported_at, COALESCE(i.updated_at,0), '', '', COALESCE(i.rating,0),
			'', COALESCE(i.in_trash,0), COALESCE(i.trashed_at,0),
			COALESCE(i.dominant_color,''), '', '', '[]'`
	}
	rows, err := m.db.Query("SELECT "+columns+" FROM items i WHERE i.id IN ("+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Item, 0, len(ids))
	for rows.Next() {
		item, err := scanStoredItem(rows)
		if err != nil {
			return nil, err
		}
		m.populateItemPaths(&item, m.activeLib.Path, m.GetAssetServerPort())
		items = append(items, item)
	}
	return items, rows.Err()
}

func (m *LibraryManager) diskQueryPage(spec QuerySpec, sortOrder string, offset, limit int, scope []string, unrated bool) (QueryPage, error) {
	page := QueryPage{Items: []Item{}, Offset: offset, Limit: limit}
	var ids []string
	err := m.diskQuery(spec, sortOrder, scope, unrated, false, true, func(item Item) {
		if page.Total >= offset && len(ids) < limit {
			ids = append(ids, item.ID)
		}
		page.Total++
		page.TotalSize += item.Size
	})
	if err != nil {
		return page, err
	}
	items, err := m.WorkspaceItems(ids)
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			page.Items = append(page.Items, item)
		}
	}
	return page, err
}

func (m *LibraryManager) diskWorkspaceStats(sortOrder string) (WorkspaceStats, error) {
	stats := WorkspaceStats{TopColors: []string{}}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return stats, nil
	}
	err := m.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size),0),
		COALESCE(SUM(CASE WHEN COALESCE(tags,'')='' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN NOT EXISTS (SELECT 1 FROM item_folders f WHERE f.item_id=i.id) THEN 1 ELSE 0 END),0)
		FROM items i WHERE COALESCE(in_trash,0)=0`).Scan(&stats.Total, &stats.TotalSize, &stats.UntaggedCount, &stats.UncategorizedCount)
	if err != nil {
		return stats, err
	}
	rows, err := m.db.Query(`SELECT UPPER(dominant_color) FROM
		(SELECT i.dominant_color, ROW_NUMBER() OVER (ORDER BY ` + diskItemOrder(sortOrder, false) + `) AS sample_order
		FROM items i WHERE COALESCE(i.in_trash,0)=0 ORDER BY ` + diskItemOrder(sortOrder, false) + ` LIMIT 1000)
		WHERE COALESCE(dominant_color,'')<>'' GROUP BY UPPER(dominant_color) ORDER BY COUNT(*) DESC, MIN(sample_order) LIMIT 8`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var color string
		if err := rows.Scan(&color); err != nil {
			return stats, err
		}
		stats.TopColors = append(stats.TopColors, color)
	}
	return stats, rows.Err()
}

func (m *LibraryManager) diskFolderPreviews(groups [][]string, sortOrder string) ([]FolderPreview, error) {
	previews := make([]FolderPreview, len(groups))
	for index, group := range groups {
		previews[index].Items = []Item{}
		if len(group) == 0 {
			continue
		}
		m.mu.RLock()
		db := m.db
		if db == nil {
			m.mu.RUnlock()
			return previews, nil
		}
		where, args := diskItemScope(false, group)
		err := db.QueryRow("SELECT COUNT(*), COALESCE(SUM(i.size),0) FROM items i WHERE "+where, args...).Scan(&previews[index].Total, &previews[index].TotalSize)
		m.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, folder := range group {
			m.mu.RLock()
			db = m.db
			if db == nil {
				m.mu.RUnlock()
				return previews, nil
			}
			where, args = diskItemScope(false, []string{folder})
			rows, err := db.Query("SELECT i.id FROM items i WHERE "+where+" ORDER BY "+diskItemOrder(sortOrder, false)+" LIMIT 4", args...)
			var ids []string
			if err == nil {
				for rows.Next() {
					var id string
					err = rows.Scan(&id)
					if err != nil {
						break
					}
					ids = append(ids, id)
				}
				if err == nil {
					err = rows.Err()
				}
				rows.Close()
			}
			m.mu.RUnlock()
			if err != nil {
				return nil, err
			}
			items, err := m.WorkspaceItems(ids)
			if err != nil {
				return nil, err
			}
			byID := map[string]Item{}
			for _, item := range items {
				byID[item.ID] = item
			}
			for _, id := range ids {
				if !seen[id] && len(previews[index].Items) < 4 {
					if item, ok := byID[id]; ok {
						seen[id] = true
						previews[index].Items = append(previews[index].Items, item)
					}
				}
			}
			if len(previews[index].Items) == 4 {
				break
			}
		}
	}
	return previews, nil
}

func (m *LibraryManager) diskAllItems(query, sortOrder string, trash bool) ([]Item, error) {
	items := []Item{}
	err := m.diskQuery(QuerySpec{Text: query}, sortOrder, nil, false, trash, false, func(item Item) {
		items = append(items, item)
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
