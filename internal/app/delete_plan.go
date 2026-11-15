package app

import "bowerbird/core"

// pendingDelete holds the selection a delete dialog is checking. Capturing it
// keeps the dialog stable while the background refresh after a previous action
// may replace a.items.
type pendingDelete struct {
	ids         []string
	folderIDs   []string
	folderNames []string
}

// pendingFolderDelete is the folder-view warning for items that would leave the
// library because the current folder is their last one. The plan is captured so
// confirming acts on exactly the selection the dialog described.
type pendingFolderDelete struct {
	plan  folderDeletePlan
	scope []string
}

// folderDeletePlan splits a folder-view selection into the items that can
// simply lose the current folder link and the items whose last folder is the
// current one, which therefore have to leave for the trash.
type folderDeletePlan struct {
	unlinkIDs []string
	trashIDs  []string
}

// planFolderDelete decides the folder-view Delete key. An item that also lives
// in a folder outside the current scope keeps existing and only loses its links
// inside the scope; an item whose only folders are inside the scope leaves the
// library for the trash.
func planFolderDelete(items []core.Item, scope []string) folderDeletePlan {
	inScope := make(map[string]bool, len(scope))
	for _, folderID := range scope {
		if folderID != "" {
			inScope[folderID] = true
		}
	}

	var plan folderDeletePlan
	for _, item := range items {
		if len(item.Folders) == 0 {
			continue
		}
		matched := 0
		for _, folderID := range item.Folders {
			if inScope[folderID] {
				matched++
			}
		}
		if matched == 0 {
			// The item is in the view through a filter the folder scope does
			// not cover; leave it alone rather than trashing something the
			// user cannot see a folder link for.
			continue
		}
		if matched == len(item.Folders) {
			plan.trashIDs = append(plan.trashIDs, item.ID)
		} else {
			plan.unlinkIDs = append(plan.unlinkIDs, item.ID)
		}
	}
	return plan
}

// itemFolderIDs returns the unique folder ids referenced by items, in item and
// folder order so dialogs read deterministically.
func itemFolderIDs(items []core.Item) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, item := range items {
		for _, folderID := range item.Folders {
			if folderID != "" && !seen[folderID] {
				seen[folderID] = true
				out = append(out, folderID)
			}
		}
	}
	return out
}

// folderNamesForIDs maps folder ids to display names in tree order. A folder id
// the tree no longer knows (a stale cache) still appears as itself so the
// dialog can tell the user where the item lives.
func folderNamesForIDs(folders []core.Folder, ids []string) []string {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			wanted[id] = true
		}
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, folder := range flattenFolders(folders) {
		if wanted[folder.ID] && !seen[folder.ID] {
			seen[folder.ID] = true
			out = append(out, folder.Name)
		}
	}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
