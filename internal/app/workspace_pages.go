package app

import (
	"context"
	"sort"

	"bowerbird/core"
)

const workspacePageSize = 96
const workspacePageLimit = 4

type workspaceRequest struct {
	context  context.Context
	query    string
	sort     string
	scope    []string
	unrated  bool
	trash    bool
	activeID string
}

func workspaceSummary(item core.Item) core.Item {
	return core.Item{
		ID: item.ID, Name: item.Name, Extension: item.Extension, MimeType: item.MimeType,
		Width: item.Width, Height: item.Height, Size: item.Size, Rating: item.Rating,
		HasThumbnail: item.HasThumbnail, InTrash: item.InTrash,
	}
}

func (a *App) refreshWorkspace() {
	if a.svc == nil {
		return
	}
	if a.loading || a.library == nil {
		a.refreshAll()
		return
	}
	a.loading = true
	if a.loadCancel != nil {
		a.loadCancel()
	}
	loadContext, cancel := context.WithCancel(context.Background())
	a.loadCancel = cancel
	a.loadSeq++
	sequence := a.loadSeq
	request := workspaceRequest{
		context: loadContext, query: a.queryString(), scope: a.folderScope(), sort: a.sortOrder,
		unrated: a.filters.Unrated, trash: a.nav == NavTrash, activeID: a.activeItemID,
	}
	index := a.folderIndex()
	nav, folderID := a.nav, a.folderID
	go func() {
		previews := a.fetchFolderPreviews(index, nav, folderID, request.sort)
		items, total, totalSize, active, err := a.fetchWorkspace(request)
		a.post(func() {
			if sequence != a.loadSeq {
				return
			}
			a.folderPreviews = previews
			a.setWorkspaceItems(items, total, totalSize)
			a.loading = false
			if active != nil {
				a.activeItem = active
			}
			a.rebuildRows()
			a.recordHistory()
			if err != nil {
				a.showToast(err.Error())
			}
		})
	}()
}

func (a *App) fetchFolderPreviews(index *folderIndex, nav Nav, folderID, sortOrder string) map[string]core.FolderPreview {
	if nav != NavFolder || folderID == "" {
		return nil
	}
	children := index.children[folderID]
	if len(children) == 0 {
		return nil
	}
	previews, err := a.svc.GetFolderPreviews(index.previewGroups(folderID), sortOrder)
	if err != nil {
		return nil
	}
	result := make(map[string]core.FolderPreview, len(children))
	for position, child := range children {
		if position < len(previews) {
			result[child.ID] = previews[position]
		}
	}
	return result
}

func (a *App) setWorkspaceItems(items []core.Item, total int, totalSize int64) {
	a.items = items
	a.itemPages = map[int]bool{}
	a.itemPageOrder = nil
	a.itemPagePending = map[int]bool{}
	if len(items) > 0 && items[0].FilePath != "" {
		a.itemPages[0] = true
		a.itemPageOrder = []int{0}
	}
	a.total = total
	a.totalSize = totalSize
}

func (a *App) fetchWorkspace(request workspaceRequest) ([]core.Item, int, int64, *core.Item, error) {
	if request.context == nil {
		request.context = context.Background()
	}
	page, err := a.svc.QueryWorkspaceIndexContext(request.context, request.query, request.sort, request.scope, request.unrated, request.trash)
	if err != nil || page == nil {
		return nil, 0, 0, nil, err
	}
	ids := make([]string, min(workspacePageSize, len(page.Items)))
	for index := range ids {
		ids[index] = page.Items[index].ID
	}
	items, err := a.svc.GetWorkspaceCards(ids)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	byID := make(map[string]core.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for index := range ids {
		if item, ok := byID[ids[index]]; ok {
			page.Items[index] = item
		}
	}
	var active *core.Item
	if request.activeID != "" {
		active, _ = a.svc.GetItem(request.activeID)
	}
	return page.Items, page.Total, page.TotalSize, active, nil
}

func (a *App) workspaceItem(index int) core.Item {
	item := a.items[index]
	if a.itemPages == nil || a.svc == nil {
		return item
	}
	page := index / workspacePageSize
	if a.itemPages[page] {
		a.touchItemPage(page)
	} else {
		a.requestItemPage(page)
	}
	if a.activeItem != nil && a.activeItem.ID == item.ID && a.activeItem.FilePath != "" {
		return *a.activeItem
	}
	return item
}

func (a *App) touchItemPage(page int) {
	for index, existing := range a.itemPageOrder {
		if existing == page {
			copy(a.itemPageOrder[index:], a.itemPageOrder[index+1:])
			a.itemPageOrder = a.itemPageOrder[:len(a.itemPageOrder)-1]
			break
		}
	}
	a.itemPageOrder = append(a.itemPageOrder, page)
}

func (a *App) requestItemPage(page int) {
	if a.loading || a.itemPagePending[page] || a.itemPageActive >= 2 {
		return
	}
	start := page * workspacePageSize
	end := min(start+workspacePageSize, len(a.items))
	ids := make([]string, end-start)
	for index := range ids {
		ids[index] = a.items[start+index].ID
	}
	a.itemPagePending[page] = true
	a.itemPageActive++
	sequence := a.loadSeq
	go func() {
		items, err := a.svc.GetWorkspaceCards(ids)
		a.post(func() {
			a.itemPageActive--
			if sequence != a.loadSeq {
				return
			}
			if err != nil {
				a.showToast(err.Error())
				return
			}
			delete(a.itemPagePending, page)
			a.storeItemPage(page, items)
		})
	}()
}

func (a *App) storeItemPage(page int, items []core.Item) {
	start := page * workspacePageSize
	end := min(start+workspacePageSize, len(a.items))
	byID := make(map[string]core.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for index := start; index < end; index++ {
		if item, ok := byID[a.items[index].ID]; ok {
			a.items[index] = item
		}
	}
	a.itemPages[page] = true
	a.touchItemPage(page)
	for len(a.itemPageOrder) > workspacePageLimit {
		drop := a.itemPageOrder[0]
		a.itemPageOrder = a.itemPageOrder[1:]
		delete(a.itemPages, drop)
		for index := drop * workspacePageSize; index < min((drop+1)*workspacePageSize, len(a.items)); index++ {
			a.items[index] = workspaceSummary(a.items[index])
		}
	}
}

func (a *App) fullWorkspaceItems(items []core.Item) []core.Item {
	if a.svc == nil || a.itemPages == nil {
		return items
	}
	result := append([]core.Item(nil), items...)
	for start := 0; start < len(result); start += workspacePageSize {
		end := min(start+workspacePageSize, len(result))
		ids := make([]string, 0, end-start)
		for _, item := range result[start:end] {
			ids = append(ids, item.ID)
		}
		loaded, err := a.svc.GetWorkspaceItems(ids)
		if err != nil {
			a.showToast(err.Error())
			return nil
		}
		byID := make(map[string]core.Item, len(loaded))
		for _, item := range loaded {
			byID[item.ID] = item
		}
		for index := start; index < end; index++ {
			if item, ok := byID[result[index].ID]; ok {
				result[index] = item
			}
		}
	}
	return result
}

func (a *App) selectionSummary() ([]core.Item, int, int64) {
	type rankedItem struct {
		item core.Item
		rank int
	}
	order := make(map[string]int, len(a.selectedOrder))
	for index, id := range a.selectedOrder {
		order[id] = index
	}
	stack := make([]rankedItem, 0, 5)
	count := 0
	var totalSize int64
	for index, item := range a.items {
		if !a.selected[item.ID] {
			continue
		}
		count++
		totalSize += item.Size
		rank, found := order[item.ID]
		if !found {
			rank = len(a.selectedOrder) + index
		}
		if len(stack) < 5 {
			stack = append(stack, rankedItem{item, rank})
			continue
		}
		oldest := 0
		for position := 1; position < len(stack); position++ {
			if stack[position].rank < stack[oldest].rank {
				oldest = position
			}
		}
		if rank > stack[oldest].rank {
			stack[oldest] = rankedItem{item, rank}
		}
	}
	sort.Slice(stack, func(left, right int) bool { return stack[left].rank < stack[right].rank })
	items := make([]core.Item, len(stack))
	for index := range stack {
		items[index] = stack[index].item
	}
	return items, count, totalSize
}

func (a *App) stackWorkspaceCards(items []core.Item) []core.Item {
	if a.svc == nil || a.itemPages == nil {
		return items
	}
	var ids []string
	for index, item := range items {
		if item.FilePath != "" {
			continue
		}
		if cached, found := a.stackCards[item.ID]; found {
			items[index] = cached
		} else {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 || a.stackCardsPending {
		return items
	}
	a.stackCardsPending = true
	sequence := a.loadSeq
	summaries := append([]core.Item(nil), items...)
	go func() {
		cards, err := a.svc.GetWorkspaceCards(ids)
		a.post(func() {
			a.stackCardsPending = false
			if sequence != a.loadSeq {
				return
			}
			a.stackCards = make(map[string]core.Item, len(summaries))
			for _, item := range summaries {
				a.stackCards[item.ID] = item
			}
			for _, item := range cards {
				a.stackCards[item.ID] = item
			}
			if err != nil {
				a.showToast(err.Error())
			}
		})
	}()
	return items
}
