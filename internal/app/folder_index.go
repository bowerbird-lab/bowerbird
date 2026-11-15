package app

import "bowerbird/core"

type folderIndex struct {
	source    *core.Folder
	sourceLen int
	all       []core.Folder
	roots     []core.Folder
	byID      map[string]core.Folder
	children  map[string][]core.Folder
	options   []filterFolderOption
}

func newFolderIndex(folders []core.Folder) *folderIndex {
	index := &folderIndex{sourceLen: len(folders)}
	if len(folders) > 0 {
		index.source = &folders[0]
	}
	index.all = flattenFolders(folders)
	index.byID = make(map[string]core.Folder, len(index.all))
	index.children = make(map[string][]core.Folder)
	for _, folder := range index.all {
		if _, exists := index.byID[folder.ID]; !exists {
			index.byID[folder.ID] = folder
		}
		index.children[folder.ParentID] = append(index.children[folder.ParentID], folder)
	}
	for _, folder := range index.all {
		if _, exists := index.byID[folder.ParentID]; folder.ParentID == "" || !exists {
			index.roots = append(index.roots, folder)
		}
	}
	var options func([]core.Folder, string)
	options = func(folders []core.Folder, prefix string) {
		for _, folder := range folders {
			name := folder.Name
			if prefix != "" {
				name = prefix + " / " + name
			}
			index.options = append(index.options, filterFolderOption{id: folder.ID, name: name})
			options(folder.Children, name)
		}
	}
	options(folders, "")
	return index
}

func (a *App) folderIndex() *folderIndex {
	var source *core.Folder
	if len(a.folders) > 0 {
		source = &a.folders[0]
	}
	index := a.folderLookup
	if index == nil || index.source != source || index.sourceLen != len(a.folders) {
		index = newFolderIndex(a.folders)
		a.folderLookup = index
	}
	return index
}

func (index *folderIndex) scopeIDs(rootID string, includeSubs bool) []string {
	if rootID == "" {
		return nil
	}
	if !includeSubs {
		return []string{rootID}
	}
	ids := make([]string, 1, len(index.children[rootID])+1)
	ids[0] = rootID
	seen := map[string]bool{rootID: true}
	for _, child := range index.children[rootID] {
		if !seen[child.ID] {
			seen[child.ID] = true
			ids = append(ids, child.ID)
		}
	}
	return ids
}

func (index *folderIndex) previewGroups(parentID string) [][]string {
	children := index.children[parentID]
	groups := make([][]string, len(children))
	for position, child := range children {
		groups[position] = []string{child.ID}
	}
	return groups
}

// pathOf is the folder's place in the tree, "A / B", as the filter menus show
// it; empty when the folder is gone.
func (index *folderIndex) pathOf(id string) string {
	for _, option := range index.options {
		if option.id == id {
			return option.name
		}
	}
	return ""
}

func (index *folderIndex) canNest(folderID, parentID string) bool {
	if folderID == "" || folderID == parentID {
		return false
	}
	for remaining := len(index.byID) + 1; remaining > 0; remaining-- {
		if parentID == "" {
			return true
		}
		if parentID == folderID {
			return false
		}
		parent, exists := index.byID[parentID]
		if !exists {
			return true
		}
		parentID = parent.ParentID
	}
	return false
}
