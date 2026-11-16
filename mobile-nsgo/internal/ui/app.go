// Package ui builds the Bowerbird mobile screens on NSGo.
package ui

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/app"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	apptheme "github.com/owocc/bowerbird/mobile-nsgo/internal/theme"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

const maxClipboardFileBytes = 500_000_000

// Screens.
const (
	screenConnect  = "connect"
	screenGallery  = "gallery"
	screenSearch   = "search"
	screenDetail   = "detail"
	screenInbox    = "inbox"
	screenSettings = "settings"
)

// App is the root component's state.
type App struct {
	scope   *core.Scope
	appCtx  context.Context
	client  *transport.Client
	local   *storage.Local
	session *app.SessionManager
	env     platform.Env
	inbox   *app.Inbox

	gallery       *app.Gallery
	searchGallery *app.Gallery
	galleryRef    *core.Ref
	searchRef     *core.Ref
	detailFrom    string

	// Sidebar & library navigation
	sidebarView     SidebarView
	activeFolderID  string
	expandedFolders map[string]bool
	drawerOpen      *core.Signal[bool]
	drawerDragging  *core.Signal[bool]
	drawerPos       *core.Signal[core.Points]
	drawerDragStart core.Points
	sheetDrag       *core.Signal[core.Points]
	sheetDragging   *core.Signal[bool]
	stats           model.Stats
	statsLoaded     bool
	statsSupported  bool
	folders         []model.Folder
	tags            []model.Tag
	colors          []string
	historyIDs      []string
	galleryFilter   query.Spec

	// Grid density (2..5 columns) and cached layout rows (zero-alloc scroll)
	gridCols         *core.Signal[int]
	pinchBaseCols    int
	pinchAppliedCols int
	galleryRev       *core.Signal[int]
	galleryScrollSeq *core.Signal[int]
	searchScrollSeq  *core.Signal[int]
	cachedRows       []GalleryRow
	cachedSearchRows []GalleryRow
	cachedTileSize   core.Points

	// Multi-select & batch actions
	multiSelect     *core.Signal[bool]
	selectedIDs     map[string]bool
	batchBusy       *core.Signal[bool]
	batchTagOpen    *core.Signal[bool]
	batchTagInput   *core.Signal[string]
	batchFolderOpen *core.Signal[bool]
	batchDelOpen    *core.Signal[bool]

	// Quick preview / restore / trash
	quickOpen      *core.Signal[bool]
	quickItem      model.ItemSummary
	quickIndex     int
	quickDelTarget model.ItemSummary
	restoreOpen    *core.Signal[bool]
	restoreTarget  model.ItemSummary
	emptyTrashOpen *core.Signal[bool]

	// Libraries & menu & disconnect
	libsOpen    *core.Signal[bool]
	menuOpen    *core.Signal[bool]
	confirmDisc *core.Signal[bool]
	libs        []model.Library
	libsBusy    *core.Signal[bool]

	// Inbox
	sharesBusy        *core.Signal[bool]
	inboxEntries      []storage.InboxEntry
	inboxBusy         *core.Signal[bool]
	inboxStatus       *core.Signal[string]
	inboxConfirmDel   *core.Signal[bool]
	inboxDeleteEntry  storage.InboxEntry
	inboxTextOpen     *core.Signal[bool]
	inboxPreviewEntry storage.InboxEntry
	inboxBackScreen   string
	inboxUploadMu     sync.Mutex
	inboxUploadCancel context.CancelFunc

	// Active file-picker upload
	uploadStatus *core.Signal[string]
	uploadMu     sync.Mutex
	uploadCancel context.CancelFunc

	// Filter sheet
	filterOpen         *core.Signal[bool]
	filterTarget       string
	filterDraftType    *core.Signal[string]
	filterDraftOrient  *core.Signal[string]
	filterDraftSize    *core.Signal[string]
	filterDraftRating  *core.Signal[int]
	filterDraftHasURL  *core.Signal[string]
	filterDraftHasNote *core.Signal[string]
	filterDraftTag     *core.Signal[string]
	filterDraftColor   *core.Signal[string]
	filterDraftFolder  *core.Signal[string]

	// Share tags lookup
	shareTagIDs   map[string]bool
	shareTagHexes map[string]bool

	// Appearance & settings
	settingsMode         *core.Signal[string]
	settingsLightTheme   *core.Signal[string]
	settingsDarkTheme    *core.Signal[string]
	settingsSystemAccent *core.Signal[bool]
	settingsThemeOpen    *core.Signal[bool]
	settingsThemePick    apptheme.Appearance
	settingsBackScreen   string
	themes               []*apptheme.Theme
	themeIndex           map[string]*apptheme.Theme
	systemAccent         core.Color

	// Connect screen
	screen        *core.Signal[string]
	rev           *core.Signal[int]
	address       *core.Signal[string]
	code          *core.Signal[string]
	busy          *core.Signal[bool]
	formErr       *core.Signal[string]
	connectStatus *core.Signal[string]
	legacyNotice  *core.Signal[string]
	pairOpen      *core.Signal[bool]
	discovering   *core.Signal[bool]
	services      []platform.Service

	// Search screen
	searchText   *core.Signal[string]
	searchType   *core.Signal[string]
	searchOrient *core.Signal[string]
	searchExtra  query.Spec

	// Detail screen
	detailIndex   *core.Signal[int]
	detailChrome  *core.Signal[bool]
	metaOpen      *core.Signal[bool]
	confirmDel    *core.Signal[bool]
	videoPaused   *core.Signal[bool]
	videoError    *core.Signal[string]
	detailCache   map[string]model.ItemDetail
	detailLoading map[string]bool
	toast         *core.Signal[string]

	// Shared-element preview: the thumbnail proxy travels from its tile
	// into the full preview, and back, through a layout transition.
	matchStage       *core.Signal[int]
	matchDetail      *core.Signal[bool]
	matchRev         *core.Signal[int]
	matchFrom        core.Bounds
	matchTo          core.Bounds
	matchThumb       string
	matchOriginal    string
	matchOpenedIndex int
	matchGen         int
	matchAnimated    bool
	matchApply       func()
	matchApplied     bool
	forceMatchAnim   bool

	wsRunning     atomic.Bool
	sharesRunning atomic.Bool
	wsStop        atomic.Bool
	refreshing    atomic.Bool
	lastLifecycle core.Lifecycle
}

// Root creates the data layer, restores the last session and returns the app.
func Root(cx *core.Scope) core.Node {
	return rootWithDir(cx, cx.DataDir())
}

func rootWithDir(cx *core.Scope, dataDir string) core.Node {
	local := storage.NewLocal(dataDir)
	prefs, prefsErr := local.LoadPreferences()
	if prefsErr != nil {
		log.Printf("[Settings] preferences unavailable, using defaults: %v", prefsErr)
	}
	a := &App{
		local:                local,
		themes:               apptheme.Load(filepath.Join(dataDir, "themes")),
		settingsMode:         core.NewSignal(cx, prefs.ThemeMode),
		settingsLightTheme:   core.NewSignal(cx, prefs.LightTheme),
		settingsDarkTheme:    core.NewSignal(cx, prefs.DarkTheme),
		settingsSystemAccent: core.NewSignal(cx, prefs.SystemAccent),
		settingsThemeOpen:    core.NewSignal(cx, false),
		settingsThemePick:    apptheme.Light,
		detailCache:          map[string]model.ItemDetail{},
		detailLoading:        map[string]bool{},
		lastLifecycle:        core.LifecycleActive,
		screen:               core.NewSignal(cx, screenConnect),
		rev:                  core.NewSignal(cx, 0),
		address:              core.NewSignal(cx, ""),
		code:                 core.NewSignal(cx, ""),
		busy:                 core.NewSignal(cx, false),
		formErr:              core.NewSignal(cx, ""),
		connectStatus:        core.NewSignal(cx, ""),
		legacyNotice:         core.NewSignal(cx, ""),
		pairOpen:             core.NewSignal(cx, false),
		discovering:          core.NewSignal(cx, false),
		searchText:           core.NewSignal(cx, ""),
		searchType:           core.NewSignal(cx, ""),
		searchOrient:         core.NewSignal(cx, ""),
		libsOpen:             core.NewSignal(cx, false),
		menuOpen:             core.NewSignal(cx, false),
		confirmDisc:          core.NewSignal(cx, false),
		libsBusy:             core.NewSignal(cx, false),
		drawerOpen:           core.NewSignal(cx, false),
		drawerDragging:       core.NewSignal(cx, false),
		drawerPos:            core.NewSignal(cx, core.Points(0)),
		sheetDrag:            core.NewSignal(cx, core.Points(0)),
		sheetDragging:        core.NewSignal(cx, false),
		gridCols:             core.NewSignal(cx, prefs.GridColumns),
		galleryRev:           core.NewSignal(cx, 0),
		galleryScrollSeq:     core.NewSignal(cx, 0),
		searchScrollSeq:      core.NewSignal(cx, 0),
		cachedTileSize:       118,
		multiSelect:          core.NewSignal(cx, false),
		selectedIDs:          map[string]bool{},
		batchBusy:            core.NewSignal(cx, false),
		batchTagOpen:         core.NewSignal(cx, false),
		batchTagInput:        core.NewSignal(cx, ""),
		batchFolderOpen:      core.NewSignal(cx, false),
		batchDelOpen:         core.NewSignal(cx, false),
		quickOpen:            core.NewSignal(cx, false),
		restoreOpen:          core.NewSignal(cx, false),
		emptyTrashOpen:       core.NewSignal(cx, false),
		sharesBusy:           core.NewSignal(cx, false),
		inboxBusy:            core.NewSignal(cx, false),
		inboxStatus:          core.NewSignal(cx, ""),
		inboxConfirmDel:      core.NewSignal(cx, false),
		inboxTextOpen:        core.NewSignal(cx, false),
		uploadStatus:         core.NewSignal(cx, ""),
		filterOpen:           core.NewSignal(cx, false),
		filterDraftType:      core.NewSignal(cx, ""),
		filterDraftOrient:    core.NewSignal(cx, ""),
		filterDraftSize:      core.NewSignal(cx, ""),
		filterDraftRating:    core.NewSignal(cx, 0),
		filterDraftHasURL:    core.NewSignal(cx, ""),
		filterDraftHasNote:   core.NewSignal(cx, ""),
		filterDraftTag:       core.NewSignal(cx, ""),
		filterDraftColor:     core.NewSignal(cx, ""),
		filterDraftFolder:    core.NewSignal(cx, ""),
		galleryRef:           &core.Ref{},
		searchRef:            &core.Ref{},
		detailIndex:          core.NewSignal(cx, 0),
		matchStage:           core.NewSignal(cx, 0),
		matchDetail:          core.NewSignal(cx, false),
		matchRev:             core.NewSignal(cx, 0),
		detailChrome:         core.NewSignal(cx, false),
		metaOpen:             core.NewSignal(cx, false),
		confirmDel:           core.NewSignal(cx, false),
		videoPaused:          core.NewSignal(cx, false),
		videoError:           core.NewSignal(cx, ""),
		toast:                core.NewSignal(cx, ""),
		sidebarView:          SidebarAll,
		expandedFolders:      map[string]bool{},
		shareTagIDs:          map[string]bool{},
		shareTagHexes:        map[string]bool{},
	}
	a.scope = cx
	a.appCtx = cx.Context()
	a.env = platform.HostEnv(cx)
	a.setupTheme(cx)
	a.client = transport.New(transport.Config{DeviceName: a.env.Device.Name})
	a.session = app.NewSessionManager(a.client, a.local, a.env.Secure)
	a.gallery = app.NewGallery()
	a.searchGallery = app.NewGallery()
	a.inbox = app.NewInbox(a.local)
	a.detailFrom = screenGallery
	a.inboxBackScreen = screenGallery
	a.settingsBackScreen = screenConnect

	a.session.RestoreAsync(func(session *storage.Session, err error) {
		a.post(func() {
			if err == nil && session != nil {
				a.address.Set(session.BaseURL)
				a.reloadShareTags()
				a.reloadUsageIDs()
				a.screen.Set(screenGallery)
				a.refreshGalleryView(cx, true)
				a.loadSidebarData(cx)
				a.startWS(cx)
				a.bump()
			}
		})
	})

	if params := cx.HostParams(); len(params) > 0 {
		if base, ok := params["baseUrl"].(string); ok && base != "" {
			a.address.Set(base)
		}
	}

	cx.Effect(func() {
		cols := a.gridCols.Get()
		if cols < 2 {
			cols = 2
		}
		winW := cx.Env().Window().Width
		if winW <= 0 {
			winW = 390
		}
		safe := cx.Env().SafeArea()
		avail := winW - safe.Left - safe.Right - 24 - core.Points(cols-1)*6
		if avail < 120 {
			avail = 120
		}
		a.cachedTileSize = avail / core.Points(cols)
		a.bumpGallery()
	})

	cx.Effect(func() {
		cur := cx.Env().Lifecycle()
		prev := a.lastLifecycle
		a.lastLifecycle = cur
		if cur == core.LifecycleActive && prev != core.LifecycleActive {
			a.post(func() {
				if a.env.Share != nil {
					a.consumeShares(cx)
				}
				if a.session.Current() != nil {
					a.startWS(cx)
					a.reloadWindow(cx)
					a.loadSidebarData(cx)
				}
			})
		}
	})

	cx.OnHostEvent("share.available", func(any) { a.consumeShares(cx) })
	cx.OnHostEvent("openUrl", func(data any) {
		if raw, ok := data.(string); ok {
			a.handlePairURL(cx, raw)
		}
	})
	if raw := core.Untracked(cx, cx.Env().OpenedURL); raw != "" {
		a.post(func() { a.handlePairURL(cx, raw) })
	}
	a.post(func() {
		a.runLegacyMigration(cx)
		a.reloadInbox()
		if a.env.Share != nil {
			a.consumeShares(cx)
		}
	})
	return a
}

// Render is the root view.
func (a *App) Render(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return core.View{
		StyleFunc: func() core.Style {
			if s := a.screen.Get(); s == screenGallery || s == screenSettings || s == screenSearch || s == screenDetail {
				return core.Style{
					Width: core.Pct(100), Height: core.Pct(100),
					Background: colors.Background(), Color: colors.Text(),
				}
			}
			return core.Style{
				Width: core.Pct(100), Height: core.Pct(100),
				Background: colors.Background(), Color: colors.Text(),
				Padding: core.All(12), Gap: 8,
				PaddingEnv: core.EnvAll(core.EnvSafeArea),
			}
		},
		Children: []core.Node{
			core.Show{When: func() bool { return a.screen.Get() == screenConnect }, Then: a.connect},
			core.Show{When: func() bool {
				s := a.screen.Get()
				return s == screenGallery || s == screenSearch || s == screenDetail
			}, Then: a.mainStack},
			core.Show{When: func() bool { return a.screen.Get() == screenInbox }, Then: a.inboxPage},
			core.Show{When: func() bool { return a.screen.Get() == screenSettings }, Then: a.settingsPage},
			a.backGuard(cx),
			a.previewMatchOverlay(cx),
			core.Show{When: func() bool { return a.toast.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{
					StyleFunc: func() core.Style {
						bottom := cx.Env().SafeArea().Bottom + 16
						return core.Style{
							Position: core.Absolute, Left: core.Pt(16), Right: core.Pt(16), Bottom: core.Pt(bottom),
							Padding: core.Symmetric(8, 12), CornerRadius: 8, ZIndex: 50,
							Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
						}
					},
					PointerEvents: core.PointerEventsNone,
					Children: []core.Node{
						core.Text{ContentFunc: a.toast.Get, MaxLines: 2, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Text)},
					},
				}
			}},
		},
	}
}

// mainStack keeps the library, search and preview as screens of one stack:
// the screens below stay mounted, so their scroll offsets survive a preview,
// and Android's predictive back can reveal them under the page being popped.
func (a *App) mainStack(cx *core.Scope) core.Node {
	// The library screen is built once and stays mounted under search and
	// preview, so its list keeps its scroll. Only the screens above it come
	// and go.
	gallery := core.Screen{
		TestID: "screen.gallery", HeaderHidden: true,
		Children: []core.Node{a.galleryPage(cx)},
	}
	search := core.Show{
		When: func() bool {
			s := a.screen.Get()
			return s == screenSearch || (s == screenDetail && a.detailFrom == screenSearch)
		},
		Then: func(cx *core.Scope) core.Node {
			return core.Screen{
				TestID: "screen.search", HeaderHidden: true,
				OnDismiss: func() {
					a.detailFrom = screenGallery
					a.screen.Set(screenGallery)
				},
				Children: []core.Node{a.searchPage(cx)},
			}
		},
	}
	detail := core.Show{
		When: func() bool { return a.screen.Get() == screenDetail },
		Then: func(cx *core.Scope) core.Node {
			return core.Screen{
				TestID: "screen.detail", HeaderHidden: true,
				NoPopOnBack: true,
				OnBackPress: func() { a.backFromDetail() },
				OnDismiss:   func() { a.dismissDetail() },
				Children:    []core.Node{a.detailPage(cx)},
			}
		},
	}
	return core.Stack{
		Style:    core.Style{Width: core.Pct(100), Height: core.Pct(100)},
		Children: []core.Node{gallery, search, detail},
	}
}

// backGuard takes back on the screens that do not pop a stack and whose
// state is not a modal: the library drawer and multi-select, settings and
// the inbox. It is absent on the preview stack so Android's predictive back
// can run there.
func (a *App) backGuard(cx *core.Scope) core.Node {
	return core.Show{
		When: func() bool {
			switch a.screen.Get() {
			case screenGallery:
				return a.drawerOpen.Get() || a.multiSelect.Get()
			case screenSettings, screenInbox:
				return true
			}
			return false
		},
		Then: func(cx *core.Scope) core.Node {
			return core.Overlay{
				Style: core.Style{
					Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0),
					Bottom: core.Pt(0), Left: core.Pt(0),
				},
				PointerEvents: core.PointerEventsNone,
				OnCancel:      func() { a.handleBack(cx) },
			}
		},
	}
}

func (a *App) handleBack(cx *core.Scope) {
	if a.screen.Get() == screenSettings {
		a.closeSettings()
		return
	}
	if a.screen.Get() == screenInbox {
		if a.inboxBackScreen == screenConnect && a.session.Current() == nil {
			a.screen.Set(screenConnect)
		} else {
			a.screen.Set(screenGallery)
		}
		return
	}
	if a.multiSelect.Get() {
		a.exitMultiSelect()
		return
	}
	if a.drawerOpen.Get() {
		a.setDrawerOpen(cx, false)
	}
}

func (a *App) rootCtx() context.Context {
	if a.appCtx != nil {
		return a.appCtx
	}
	return context.Background()
}
func (a *App) post(fn func())                   { a.scope.Post(fn) }
func (a *App) after(d time.Duration, fn func()) { a.scope.AfterFunc(d, fn) }
func (a *App) bump()                            { a.rev.Update(func(v int) int { return v + 1 }) }
func (a *App) bumpGallery()                     { a.galleryRev.Update(func(v int) int { return v + 1 }) }

func (a *App) rebuildGalleryRows() {
	snap := a.gallery.Snapshot()
	cols := a.gridCols.Get()
	if a.sidebarView == SidebarTrash || a.sidebarView == SidebarHistory {
		a.cachedRows = BuildFlatRows(snap.Items, cols)
	} else {
		a.cachedRows = BuildMonthRows(snap.Items, cols)
	}
	a.bumpGallery()
}

func (a *App) rebuildSearchRows() {
	snap := a.searchGallery.Snapshot()
	a.cachedSearchRows = BuildFlatRows(snap.Items, a.gridCols.Get())
	a.bumpGallery()
}

func (a *App) showToast(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	a.toast.Set(msg)
	if a.scope != nil {
		a.scope.AfterFunc(3*time.Second, func() {
			if a.toast.Get() == msg {
				a.toast.Set("")
			}
		})
	}
}

func (a *App) effectiveGallerySpec() query.Spec {
	spec := a.galleryFilter
	spec.Folder = ""
	spec.Untagged = false
	spec.Uncategorized = false
	switch a.sidebarView {
	case SidebarUntagged:
		spec.Untagged = true
	case SidebarUncategorized:
		spec.Uncategorized = true
	case SidebarFolder:
		spec.Folder = a.activeFolderID
	}
	return spec
}

func (a *App) selectCollection(cx *core.Scope, next SidebarView) {
	if a.sidebarView == next && next != SidebarFolder {
		return
	}
	a.exitMultiSelect()
	a.sidebarView = next
	a.activeFolderID = ""
	a.bump()
	a.refreshGalleryView(cx, true)
}

func (a *App) selectFolder(cx *core.Scope, folderID string) {
	a.exitMultiSelect()
	a.sidebarView = SidebarFolder
	a.activeFolderID = folderID
	a.bump()
	a.refreshGalleryView(cx, true)
}

func (a *App) ensureLoaded(cx *core.Scope) {
	snap := a.gallery.Snapshot()
	if snap.Loading || snap.LoadedOnce {
		return
	}
	a.refreshGalleryView(cx, true)
}

func (a *App) refreshGalleryView(cx *core.Scope, jumpTop bool) {
	spec := a.effectiveGallerySpec()
	token := a.gallery.Reset(spec, model.SortDesc, model.DefaultPageSize)
	a.bump()
	a.fetchGalleryPage(cx, token, a.sidebarView, spec, 0, model.DefaultPageSize, false, jumpTop)
}

func (a *App) loadMoreGalleryView(cx *core.Scope) {
	token, spec, _, offset, limit, ok := a.gallery.Request(true)
	if !ok {
		return
	}
	a.bump()
	a.fetchGalleryPage(cx, token, a.sidebarView, spec, offset, limit, true, false)
}

func (a *App) reloadWindow(cx *core.Scope) {
	if a.session.Current() == nil {
		return
	}
	winLimit := a.gallery.WindowLimit()
	spec := a.effectiveGallerySpec()
	token := a.gallery.Invalidate()
	a.fetchGalleryPage(cx, token, a.sidebarView, spec, 0, winLimit, false, false)
	if a.screen.Get() == screenSearch {
		sSpec := a.searchSpec()
		sLimit := a.searchGallery.WindowLimit()
		sToken := a.searchGallery.Invalidate()
		go func() {
			ctx, cancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
			defer cancel()
			page, err := a.client.List(ctx, sSpec, model.SortDesc, 0, sLimit)
			a.post(func() {
				a.searchGallery.Apply(sToken, page, false, err)
				a.rebuildSearchRows()
				a.bump()
			})
		}()
	}
}

func (a *App) fetchGalleryPage(cx *core.Scope, token uint64, view SidebarView, spec query.Spec, offset, limit int, appendPage, jumpTop bool) {
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
		defer cancel()
		var page model.ItemsPage
		var err error
		switch view {
		case SidebarTrash:
			page, err = a.client.Collection(ctx, model.CollectionTrash, offset, limit)
		case SidebarHistory:
			page, err = a.fetchHistoryPage(ctx, offset, limit)
		case SidebarUntagged:
			if spec.Type == "" && spec.Orientation == "" && spec.Size == "" && spec.Rating == 0 &&
				spec.HasURL == "" && spec.HasNote == "" && spec.Tag == "" && spec.Color == "" && spec.Text == "" {
				page, err = a.client.Collection(ctx, model.CollectionUntagged, offset, limit)
			} else {
				page, err = a.client.List(ctx, spec, model.SortDesc, offset, limit)
			}
		case SidebarUncategorized:
			if spec.Type == "" && spec.Orientation == "" && spec.Size == "" && spec.Rating == 0 &&
				spec.HasURL == "" && spec.HasNote == "" && spec.Tag == "" && spec.Color == "" && spec.Text == "" {
				page, err = a.client.Collection(ctx, model.CollectionUncategorized, offset, limit)
			} else {
				page, err = a.client.List(ctx, spec, model.SortDesc, offset, limit)
			}
		default:
			page, err = a.client.List(ctx, spec, model.SortDesc, offset, limit)
		}
		a.post(func() {
			applied := a.gallery.Apply(token, page, appendPage, err)
			if applied {
				a.rebuildGalleryRows()
			}
			a.bump()
			if applied && jumpTop && !appendPage {
				a.scrollTop(cx, a.galleryRef)
			}
		})
	}()
}

func (a *App) fetchHistoryPage(ctx context.Context, offset, limit int) (model.ItemsPage, error) {
	ids := a.historyIDs
	total := len(ids)
	if offset >= total {
		return model.ItemsPage{Total: total, Limit: limit, Offset: offset, Items: []model.ItemSummary{}}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	sliceIDs := ids[offset:end]
	items := make([]model.ItemSummary, 0, len(sliceIDs))
	for _, id := range sliceIDs {
		if ctx.Err() != nil {
			return model.ItemsPage{}, ctx.Err()
		}
		detail, err := a.client.Item(ctx, id)
		if err != nil {
			continue
		}
		items = append(items, detail.Summary())
	}
	return model.ItemsPage{Total: total, Limit: limit, Offset: offset, Items: items}, nil
}

func (a *App) loadSidebarData(cx *core.Scope) {
	if a.session.Current() == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 15*time.Second)
		defer cancel()
		stats, statsErr := a.client.Stats(ctx)
		folders, foldersErr := a.client.Folders(ctx)
		tags, tagsErr := a.client.Tags(ctx)
		colors, colorsErr := a.client.Colors(ctx)
		a.post(func() {
			a.statsLoaded = true
			if statsErr == nil {
				a.stats = stats
				a.statsSupported = true
			} else if transport.IsUnsupported(statsErr) {
				a.statsSupported = false
			}
			if foldersErr == nil {
				a.folders = folders
			}
			if tagsErr == nil {
				a.tags = tags
			}
			if colorsErr == nil {
				a.colors = colors
			}
			a.bump()
		})
	}()
}

func (a *App) flattenAllFolders() []model.Folder {
	var out []model.Folder
	var walk func(list []model.Folder)
	walk = func(list []model.Folder) {
		for _, f := range list {
			out = append(out, f)
			if len(f.Children) > 0 {
				walk(f.Children)
			}
		}
	}
	walk(a.folders)
	return out
}

func (a *App) scrollTop(cx *core.Scope, ref *core.Ref) {
	if a.env.HasHost {
		if ref == a.searchRef {
			a.searchScrollSeq.Update(func(v int) int { return v + 1 })
		} else {
			a.galleryScrollSeq.Update(func(v int) int { return v + 1 })
		}
		return
	}
	if ref == nil {
		return
	}
	a.after(time.Millisecond, func() { ref.ScrollToIndex(0, false) })
}

func (a *App) openLibraries(cx *core.Scope) {
	if a.libsBusy.Get() {
		return
	}
	a.libsBusy.Set(true)
	a.libsOpen.Set(true)
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 15*time.Second)
		defer cancel()
		libs, err := a.session.Libraries(ctx)
		a.post(func() {
			a.libsBusy.Set(false)
			if err != nil {
				a.showToast(transport.DescribeFailure(err))
				return
			}
			a.libs = libs
			a.bump()
		})
	}()
}

func (a *App) switchLibrary(cx *core.Scope, lib model.Library) {
	if lib.IsActive {
		a.libsOpen.Set(false)
		return
	}
	a.libsBusy.Set(true)
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 20*time.Second)
		defer cancel()
		err := a.session.SwitchLibrary(ctx, lib)
		a.post(func() {
			a.libsBusy.Set(false)
			if err != nil {
				a.showToast(transport.DescribeFailure(err))
				return
			}
			a.libsOpen.Set(false)
			a.invalidateForLibraryChange(cx)
			a.showToast("已切换到 " + lib.Name)
		})
	}()
}

func (a *App) invalidateForLibraryChange(cx *core.Scope) {
	a.exitMultiSelect()
	a.sidebarView = SidebarAll
	a.activeFolderID = ""
	a.galleryFilter = query.Spec{}
	a.folders = nil
	a.tags = nil
	a.colors = nil
	a.stats = model.Stats{}
	a.statsLoaded = false
	clear(a.detailCache)
	clear(a.detailLoading)
	a.reloadShareTags()
	a.reloadUsageIDs()
	a.searchGallery.Reset(query.Spec{}, model.SortDesc, model.DefaultPageSize)
	a.rebuildSearchRows()
	if a.screen.Get() == screenDetail || a.screen.Get() == screenSearch {
		a.cancelPreviewMatch()
		a.screen.Set(screenGallery)
	}
	a.refreshGalleryView(cx, true)
	a.loadSidebarData(cx)
	a.bump()
}

func (a *App) resetSessionState() {
	a.exitMultiSelect()
	a.gallery.Reset(query.Spec{}, model.SortDesc, model.DefaultPageSize)
	a.searchGallery.Reset(query.Spec{}, model.SortDesc, model.DefaultPageSize)
	a.cachedRows = nil
	a.cachedSearchRows = nil
	a.bumpGallery()
	a.folders = nil
	a.tags = nil
	a.colors = nil
	a.libs = nil
	a.stats = model.Stats{}
	a.statsLoaded = false
	clear(a.detailCache)
	clear(a.detailLoading)
	clear(a.shareTagIDs)
	clear(a.shareTagHexes)
	a.historyIDs = nil
}

func (a *App) startWS(cx *core.Scope) {
	if !a.wsRunning.CompareAndSwap(false, true) {
		return
	}
	a.wsStop.Store(false)
	ctx := a.rootCtx()
	go func() {
		attempt := 0
		for !a.wsStop.Load() && ctx.Err() == nil {
			conn, err := a.client.DialWS(ctx)
			if err != nil {
				select {
				case <-time.After(transport.Backoff(attempt)):
				case <-ctx.Done():
					return
				}
				attempt++
				continue
			}
			attempt = 0
			a.post(func() {
				a.reloadWindow(cx)
				a.loadSidebarData(cx)
			})
			for !a.wsStop.Load() && ctx.Err() == nil {
				ev, readErr := conn.Read(ctx)
				if readErr != nil {
					break
				}
				a.post(func() { a.handleEvent(cx, ev) })
				if a.wsStop.Load() {
					break
				}
			}
			conn.Close()
			if a.wsStop.Load() || ctx.Err() != nil {
				break
			}
			select {
			case <-time.After(transport.Backoff(attempt)):
			case <-ctx.Done():
				return
			}
			attempt++
		}
		a.post(func() { a.wsRunning.Store(false) })
	}()
}

func (a *App) stopWS() {
	a.wsStop.Store(true)
}

func (a *App) handleEvent(cx *core.Scope, ev model.WSEvent) {
	action, name := app.ClassifyEvent(ev)
	switch action {
	case app.EventRefresh:
		a.scheduleCalibratedRefresh(cx, 350*time.Millisecond)
	case app.EventLibraryChanged:
		a.session.UseLibraryName(name)
		a.invalidateForLibraryChange(cx)
	case app.EventRevoked:
		a.stopWS()
		_ = a.session.Disconnect()
		a.resetSessionState()
		a.cancelPreviewMatch()
		a.showToast("授权已被桌面端撤销，请重新配对")
		a.screen.Set(screenConnect)
	}
}

func (a *App) scheduleCalibratedRefresh(cx *core.Scope, delay time.Duration) {
	if a.refreshing.Swap(true) {
		return
	}
	a.after(delay, func() {
		a.refreshing.Store(false)
		a.reloadWindow(cx)
		a.loadSidebarData(cx)
	})
}

func (a *App) mediaDir(cx *core.Scope) string {
	dir := cx.CacheDir()
	if dir == "" {
		dir = cx.DataDir()
	}
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "media")
}

func (a *App) clipboardDir(cx *core.Scope) string {
	dir := cx.CacheDir()
	if dir == "" {
		dir = cx.DataDir()
	}
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "clipboard")
}

func (a *App) exportItem(cx *core.Scope, item model.ItemSummary, share bool) {
	name := safeFilename(item)
	dir := a.mediaDir(cx)
	transport.PruneDir(dir, 24*time.Hour)
	dest := filepath.Join(dir, item.ID+"-"+name)
	a.showToast("正在准备原文件…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 10*time.Minute)
		defer cancel()
		_, err := a.client.Download(ctx, item.ID, dest, nil)
		a.post(func() {
			if err != nil {
				a.showToast("下载失败: " + transport.DescribeFailure(err))
				return
			}
			if share {
				a.recordUsage(item.ID)
				cx.Share(core.ShareOptions{
					Files: []core.SharedFile{{Path: dest, Name: name, MIME: item.MimeType}},
					Text:  item.DisplayName(),
				}, func(shown bool, shareErr error) {
					if shareErr != nil {
						a.showToast("无法分享: " + transport.DescribeFailure(shareErr))
					} else if shown {
						a.showToast("已打开分享面板")
					}
				})
				return
			}
			if a.env.Media != nil && (item.IsImage() || item.IsVideo()) {
				a.env.Media.SaveToGallery(dest, name, item.MimeType, func(_ string, saveErr error) {
					a.post(func() {
						if saveErr != nil {
							a.showToast("保存到相册失败: " + saveErr.Error())
							return
						}
						a.recordUsage(item.ID)
						a.showToast("已保存到相册 Bowerbird")
					})
				})
				return
			}
			cx.SaveFile(core.SaveOptions{Path: dest, Name: name, MIME: item.MimeType}, func(saved bool, _ string, saveErr error) {
				switch {
				case saveErr != nil:
					a.showToast("保存失败: " + saveErr.Error())
				case saved:
					a.recordUsage(item.ID)
					a.showToast("已保存")
				default:
					a.showToast("已取消保存")
				}
			})
		})
	}()
}

func (a *App) copyItemContent(cx *core.Scope, item model.ItemSummary) {
	if item.Size > maxClipboardFileBytes {
		a.showToast("文件超过 500 MB，请使用分享或下载")
		return
	}
	if a.env.Clipboard == nil {
		cx.SetClipboard(itemInfoText(item, a.detailCache[item.ID]))
		a.showToast("已复制信息文本")
		return
	}
	name := safeFilename(item)
	dir := a.clipboardDir(cx)
	transport.PruneDir(dir, 24*time.Hour)
	dest := filepath.Join(dir, item.ID+"-"+name)
	a.showToast("正在准备复制内容…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 5*time.Minute)
		defer cancel()
		_, err := a.client.Download(ctx, item.ID, dest, nil)
		a.post(func() {
			if err != nil {
				a.showToast("复制失败: " + transport.DescribeFailure(err))
				return
			}
			a.env.Clipboard.CopyFile(dest, item.MimeType, name, func(copyErr error) {
				a.post(func() {
					if copyErr != nil {
						a.showToast("复制失败: " + copyErr.Error())
						return
					}
					a.recordUsage(item.ID)
					a.showToast("已复制素材到剪贴板")
				})
			})
		})
	}()
}

func (a *App) pickAndUpload(cx *core.Scope) {
	if a.session.Current() == nil {
		a.showToast("请先连接桌面端")
		return
	}
	cx.PickFiles(core.PickFilesOptions{
		Multiple: true,
		Types:    []string{"image/*", "video/*", "audio/*"},
	}, func(files []core.File, err error) {
		if err != nil {
			a.showToast("选择文件失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		ctx, cancel := context.WithCancel(a.rootCtx())
		a.uploadMu.Lock()
		a.uploadCancel = cancel
		a.uploadMu.Unlock()
		a.uploadStatus.Set("正在上传 0/" + itoa(len(files)) + "…")
		go func() {
			defer cancel()
			okCount := 0
			var lastErr error
			for i, f := range files {
				if ctx.Err() != nil {
					lastErr = ctx.Err()
					break
				}
				a.post(func() { a.uploadStatus.Set("正在上传 " + itoa(i+1) + "/" + itoa(len(files)) + "：" + f.Name) })
				itemCtx, itemCancel := context.WithTimeout(ctx, 5*time.Minute)
				_, upErr := a.client.UploadFile(itemCtx, f.Path)
				itemCancel()
				if upErr != nil {
					lastErr = upErr
				} else {
					okCount++
				}
			}
			a.post(func() {
				a.uploadMu.Lock()
				a.uploadCancel = nil
				a.uploadMu.Unlock()
				a.uploadStatus.Set("")
				switch {
				case lastErr == nil:
					a.showToast("已上传 " + itoa(okCount) + " 个文件")
				case okCount == 0:
					a.showToast("上传失败: " + transport.DescribeFailure(lastErr))
				default:
					a.showToast("成功 " + itoa(okCount) + " 个，失败 " + itoa(len(files)-okCount) + " 个")
				}
				if okCount > 0 {
					a.scheduleCalibratedRefresh(cx, 150*time.Millisecond)
				}
			})
		}()
	})
}

func (a *App) cancelActiveUpload() {
	a.uploadMu.Lock()
	cancel := a.uploadCancel
	a.uploadMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) recordUsage(itemID string) {
	scope := a.session.ScopeKey()
	if scope == "" || itemID == "" {
		return
	}
	_ = a.local.AddUsage(scope, itemID)
	a.reloadUsageIDs()
}

func (a *App) reloadUsageIDs() {
	scope := a.session.ScopeKey()
	ids, err := a.local.UsageIDs(scope)
	if err != nil {
		return
	}
	a.historyIDs = ids
	a.bump()
}

func (a *App) reloadShareTags() {
	ids, hexes := a.local.ShareTagSets(a.session.ScopeKey(), a.session.LegacyNameScope())
	a.shareTagIDs = ids
	a.shareTagHexes = hexes
	a.bumpGallery()
}

func (a *App) isItemShareMarked(id string) bool {
	if id == "" {
		return false
	}
	upper := strings.ToUpper(id)
	return a.shareTagIDs[id] || a.shareTagIDs[upper] || a.shareTagHexes[upper]
}

func (a *App) runLegacyMigration(cx *core.Scope) {
	if a.env.Migrator == nil {
		return
	}
	a.env.Migrator.ScanLegacy(func(paths platform.LegacyPaths, err error) {
		if err != nil {
			return
		}
		if paths.LegacyInboxPath != "" {
			_, _ = a.local.MarkMigration("flutter-inbox-v1", func() error {
				_, err := a.local.MigrateLegacyInbox(paths.LegacyInboxPath)
				return err
			})
		}
		if paths.LegacyShareTagsPath != "" {
			_, _ = a.local.MarkMigration("flutter-share-tags-v1", func() error {
				_, err := a.local.MigrateLegacyShareTags(paths.LegacyShareTagsPath)
				return err
			})
		}
		a.post(func() {
			a.reloadInbox()
			a.reloadShareTags()
			if paths.HasLegacySecure && a.session.Current() == nil {
				a.legacyNotice.Set("检测到旧版客户端数据：收件箱与分享标记已保留迁移；旧版加密凭据格式不同，请重新扫码或输入地址配对。")
			}
			a.bump()
		})
	})
}

func (a *App) handlePairURL(cx *core.Scope, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	payload := raw
	if u, err := url.Parse(raw); err == nil && u.Scheme == "bowerbird" {
		payload = u.Query().Get("data")
		if payload == "" {
			return
		}
	}
	ips, port, code, err := transport.ValidatePairPayload([]byte(payload))
	if err != nil {
		a.showToast("配对链接无效")
		return
	}
	a.busy.Set(true)
	a.formErr.Set("")
	a.connectStatus.Set(fmt.Sprintf("已识别 %d 个候选地址，正在检查可用性…", len(ips)))
	go func() {
		// Checking candidates and pairing are separate phases: a slow or
		// unreachable address must not consume the pairing timeout.
		probeCtx, probeCancel := context.WithTimeout(a.rootCtx(), 30*time.Second)
		base, status, probeErr := app.ProbeCandidatesWithProgress(probeCtx, a.client, ips, port, func(update app.ProbeProgress) {
			address := strings.TrimPrefix(strings.TrimPrefix(update.Address, "https://"), "http://")
			if update.Responsive {
				a.post(func() { a.connectStatus.Set("已找到可用地址 " + address + "，正在连接…") })
				return
			}
			a.post(func() {
				a.connectStatus.Set(fmt.Sprintf("正在检查 %d/%d：%s", update.Index+1, update.Total, address))
			})
		})
		probeCancel()
		if probeErr != nil {
			a.post(func() {
				a.busy.Set(false)
				a.connectStatus.Set("")
				a.formErr.Set(transport.DescribeFailure(probeErr))
				a.screen.Set(screenConnect)
			})
			return
		}
		a.post(func() { a.connectStatus.Set("已连接 " + status.Name + "，正在配对…") })
		connectCtx, connectCancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
		session, connectErr := a.session.Connect(connectCtx, base, code, a.env.Device.Name)
		connectCancel()
		a.post(func() {
			a.busy.Set(false)
			a.connectStatus.Set("")
			if connectErr != nil {
				a.address.Set(base)
				a.code.Set(code)
				a.screen.Set(screenConnect)
				if connectErr == app.ErrPairingRequired {
					a.pairOpen.Set(true)
				} else {
					a.formErr.Set(transport.DescribeFailure(connectErr))
				}
				return
			}
			a.onSessionConnected(cx, session)
		})
	}()
}

func (a *App) libraryName() string {
	if s := a.session.Current(); s != nil && s.LibraryName != "" {
		return s.LibraryName
	}
	return "Bowerbird"
}

func safeFilename(item model.ItemSummary) string {
	name := strings.TrimSpace(item.Filename)
	if name == "" {
		name = strings.TrimSpace(item.Name)
	}
	if name == "" {
		name = item.ID
	}
	base := filepath.Base(name)
	base = strings.NewReplacer("/", "_", "\\", "_", "\x00", "").Replace(base)
	if base == "." || base == ".." || base == "" {
		base = item.ID
	}
	if item.Extension != "" && !strings.Contains(base, ".") {
		base += "." + strings.TrimPrefix(item.Extension, ".")
	}
	return base
}

func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Has("token") {
		q.Set("token", "redacted")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
