package app

import (
	"testing"

	"bowerbird/core"

	"github.com/egoist/mygo/ui"
)

func TestPlanFolderDeleteSeparatesUnlinkFromTrash(t *testing.T) {
	items := []core.Item{
		{ID: "last-home", Folders: []string{"f1"}},
		{ID: "still-nested", Folders: []string{"f1", "f2"}},
		{ID: "elsewhere", Folders: []string{"f2"}},
		{ID: "uncategorized"},
	}
	plan := planFolderDelete(items, []string{"f1"})

	if len(plan.trashIDs) != 1 || plan.trashIDs[0] != "last-home" {
		t.Fatalf("trash ids = %v, want [last-home]", plan.trashIDs)
	}
	if len(plan.unlinkIDs) != 1 || plan.unlinkIDs[0] != "still-nested" {
		t.Fatalf("unlink ids = %v, want [still-nested]", plan.unlinkIDs)
	}
}

func TestPlanFolderDeleteUsesTheWholeVisibleScope(t *testing.T) {
	items := []core.Item{
		{ID: "in-parent-and-child", Folders: []string{"parent", "child"}},
		{ID: "outside-the-scope", Folders: []string{"parent", "other"}},
	}
	plan := planFolderDelete(items, []string{"parent", "child"})

	if len(plan.trashIDs) != 1 || plan.trashIDs[0] != "in-parent-and-child" {
		t.Fatalf("trash ids = %v, want the item whose only homes are visible", plan.trashIDs)
	}
	if len(plan.unlinkIDs) != 1 || plan.unlinkIDs[0] != "outside-the-scope" {
		t.Fatalf("unlink ids = %v, want the item with a home outside the scope", plan.unlinkIDs)
	}
}

func TestItemFolderIDsAndNamesAreUniqueAndOrdered(t *testing.T) {
	folders := []core.Folder{
		{ID: "parent", Name: "Parent", Children: []core.Folder{{ID: "child", Name: "Child"}}},
		{ID: "sibling", Name: "Sibling"},
	}
	items := []core.Item{
		{ID: "a", Folders: []string{"child", "parent", "child"}},
		{ID: "b", Folders: []string{"sibling", "stale-id"}},
	}
	ids := itemFolderIDs(items)
	wantIDs := []string{"child", "parent", "sibling", "stale-id"}
	if len(ids) != len(wantIDs) {
		t.Fatalf("folder ids = %v, want %v", ids, wantIDs)
	}
	for i := range ids {
		if ids[i] != wantIDs[i] {
			t.Fatalf("folder ids = %v, want %v", ids, wantIDs)
		}
	}

	names := folderNamesForIDs(folders, ids)
	wantNames := []string{"Parent", "Child", "Sibling", "stale-id"}
	if len(names) != len(wantNames) {
		t.Fatalf("folder names = %v, want %v", names, wantNames)
	}
	for i := range names {
		if names[i] != wantNames[i] {
			t.Fatalf("folder names = %v, want %v", names, wantNames)
		}
	}
}

func TestDeleteFromSmartViewOpensTheFolderCheck(t *testing.T) {
	a := seededApp(t)
	a.svc = core.NewService(core.NewLibraryManager())
	a.nav = NavAll
	a.items[0].Folders = []string{"f1"}
	a.selected = map[string]bool{"a": true}

	a.deleteSelection()

	if a.dialog != dialogDeleteItems {
		t.Fatalf("dialog = %v, want the delete check", a.dialog)
	}
	if a.pendingDelete == nil {
		t.Fatal("delete check has no pending selection")
	}
	if len(a.pendingDelete.ids) != 1 || a.pendingDelete.ids[0] != "a" {
		t.Fatalf("pending ids = %v, want [a]", a.pendingDelete.ids)
	}
	if len(a.pendingDelete.folderNames) != 1 || a.pendingDelete.folderNames[0] != "Inspiration" {
		t.Fatalf("pending folder names = %v, want [Inspiration]", a.pendingDelete.folderNames)
	}

	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	for _, want := range []string{
		a.t("workspace.deleteTitle"),
		a.tf("workspace.deleteConfirmMessage", map[string]any{"count": 1}),
		a.t("workspace.deleteConfirmFoldersHint"),
		"Inspiration",
		a.t("workspace.removeFromAllFolders"),
		a.t("contextMenu.moveToTrash"),
	} {
		if !tester.HasText(want) {
			t.Fatalf("delete check is missing %q", want)
		}
	}
}

func TestDeleteFromFolderWarnsBeforeTrashingTheLastHome(t *testing.T) {
	a := seededApp(t)
	a.svc = core.NewService(core.NewLibraryManager())
	a.nav = NavFolder
	a.folderID = "f1"
	a.items[0].Folders = []string{"f1"}
	a.items[1].Folders = []string{"f1", "f2"}
	a.selected = map[string]bool{"a": true, "b": true}

	a.deleteSelection()

	if a.dialog != dialogDeleteLastFolder {
		t.Fatalf("dialog = %v, want the last-folder warning", a.dialog)
	}
	if a.pendingFolderDelete == nil {
		t.Fatal("last-folder warning has no pending plan")
	}
	req := a.pendingFolderDelete
	if len(req.plan.trashIDs) != 1 || req.plan.trashIDs[0] != "a" {
		t.Fatalf("trash ids = %v, want [a]", req.plan.trashIDs)
	}
	if len(req.plan.unlinkIDs) != 1 || req.plan.unlinkIDs[0] != "b" {
		t.Fatalf("unlink ids = %v, want [b]", req.plan.unlinkIDs)
	}

	tester := ui.NewTester(a.viewMain, 1100, 760)
	tester.Frame()
	for _, want := range []string{
		a.t("workspace.deleteLastFolderTitle"),
		a.t("workspace.deleteLastFolderMessage"),
		a.t("workspace.deleteLastFolderOtherItems"),
		a.t("common.delete"),
	} {
		if !tester.HasText(want) {
			t.Fatalf("last-folder warning is missing %q", want)
		}
	}
}

func TestDeleteActionLabelFollowsTheView(t *testing.T) {
	a := seededApp(t)

	a.nav = NavAll
	if got, want := a.deleteActionLabel(), a.t("common.delete"); got != want {
		t.Fatalf("smart view label = %q, want %q", got, want)
	}

	a.nav = NavFolder
	a.folderID = "f1"
	if got, want := a.deleteActionLabel(), a.t("contextMenu.removeFromCurrentFolder"); got != want {
		t.Fatalf("folder label = %q, want %q", got, want)
	}

	a.nav = NavTrash
	if got, want := a.deleteActionLabel(), a.t("contextMenu.deletePermanently"); got != want {
		t.Fatalf("trash label = %q, want %q", got, want)
	}
}
