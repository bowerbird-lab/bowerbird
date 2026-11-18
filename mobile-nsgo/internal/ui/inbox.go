package ui

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func (a *App) inboxPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()

	back := func() {
		if (a.inboxBackScreen == screenConnect || a.inboxBackScreen == screenWelcome || a.inboxBackScreen == screenDevices) && a.session.Current() == nil {
			a.screen.Set(a.inboxBackScreen)
			return
		}
		a.screen.Set(screenGallery)
	}

	header := core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
		Children: []core.Node{
			nsui.IconButton{Icon: lucide.ArrowLeft, Variant: nsui.IconButtonGhost, Label: "返回", OnPress: back, TestID: "historySave.back"},
			core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.Text{Content: "历史保存", Style: nsui.TypeTitle.Merge(core.Style{Color: colors.Text()})},
				core.Text{ContentFunc: func() string {
					a.rev.Get()
					return saveHistoryCountText(a.saveHistoryIDs)
				}, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			}},
			nsui.Button{
				Text:    "刷新",
				Variant: nsui.ButtonSecondary,
				Disabled: func() bool {
					return a.saveHistoryLoading.Get() || a.session.Current() == nil
				},
				OnPress: func() { a.loadSaveHistoryItems(cx) },
				TestID:  "historySave.refresh",
			},
		},
	}

	status := core.Show{
		When: func() bool { return a.saveHistoryStatus.Get() != "" },
		Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: a.saveHistoryStatus.Get, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}
		},
	}

	list := core.List{
		Count: func() int { a.rev.Get(); return len(a.saveHistoryItems) },
		Key:   func(i int) any { a.rev.Get(); return a.saveHistoryItems[i].ID },
		Row:   func(cx *core.Scope, row core.ListRow) core.Node { return a.saveHistoryRow(cx, row) },
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
	}

	var body core.Node = list
	if a.session.Current() == nil {
		body = core.View{
			Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{
				core.Text{Content: "连接桌面端后可查看历史保存", Style: nsui.TypeBody.Merge(core.Style{Color: colors.Muted()})},
			},
		}
	} else if len(a.saveHistoryIDs) == 0 {
		body = core.View{
			Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(24)},
			Children: []core.Node{
				core.Text{Content: "暂无历史保存，从其他应用分享即可保存到电脑", MaxLines: 2,
					Style: nsui.TypeBody.Merge(core.Style{Color: colors.Muted(), TextAlign: core.TextAlignCenter})},
			},
		}
	}

	return core.View{
		TestID:   "screen.historySave",
		Style:    core.Style{Width: core.Pct(100), Height: core.Pct(100), Gap: 10},
		Children: []core.Node{header, status, body},
	}
}

func (a *App) saveHistoryRow(cx *core.Scope, row core.ListRow) core.Node {
	colors := nsui.ColorsOf(cx)
	itemAt := func() (model.ItemSummary, bool) {
		_ = a.rev.Get()
		index := row.Index()
		if index < 0 || index >= len(a.saveHistoryItems) {
			return model.ItemSummary{}, false
		}
		return a.saveHistoryItems[index], true
	}

	item, ok := itemAt()
	if !ok {
		return core.View{}
	}

	var thumbNode core.Node
	if thumbURL := a.itemThumbURL(item); thumbURL != "" {
		thumbNode = core.Image{
			Source:     core.Src(thumbURL),
			ResizeMode: core.Cover,
			Style:      core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 8, Background: colors.Inset()},
		}
	} else {
		thumbNode = core.View{
			Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 8, Background: colors.Inset(),
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{
				core.Icon{SVG: lucide.File(), Style: core.Style{FontSize: 20, Color: colors.Muted()}},
			},
		}
	}

	subParts := []string{}
	if ext := strings.ToUpper(strings.TrimPrefix(item.Extension, ".")); ext != "" {
		subParts = append(subParts, ext)
	}
	if item.Size > 0 {
		subParts = append(subParts, model.FormatSize(item.Size))
	}
	subText := strings.Join(subParts, " · ")

	return core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.All(10),
			CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border(),
			Margin: core.Edges{Bottom: 6}},
		OnPress: func(core.PressEvent) {
			cur, ok := itemAt()
			if !ok {
				return
			}
			items := a.gallery.Snapshot().Items
			for idx, gItem := range items {
				if gItem.ID == cur.ID {
					a.screen.Set(screenGallery)
					a.openDetail(cx, idx, screenGallery, core.Bounds{})
					return
				}
			}
			a.showToast("可在全部素材中查看「" + cur.Name + "」")
		},
		Children: []core.Node{
			thumbNode,
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{Content: item.Name, MaxLines: 1, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
				core.Text{Content: subText, MaxLines: 1, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
			}},
			core.Icon{SVG: lucide.ChevronRight(), Style: core.Style{FontSize: 16, Color: colors.Faint()}},
		},
	}
}

func saveHistoryCountText(ids []string) string {
	if len(ids) == 0 {
		return "暂无历史保存（最多保留 300 条）"
	}
	return "共 " + itoa(len(ids)) + " 条（最多保留 300 条）"
}

func (a *App) reloadSaveHistory() {
	scope := a.session.ScopeKey()
	if scope == "" {
		a.saveHistoryIDs = nil
		a.saveHistoryItems = nil
		a.bump()
		return
	}
	ids, err := a.local.SaveHistoryIDs(scope)
	if err != nil {
		return
	}
	a.saveHistoryIDs = ids
	a.bump()
}

func (a *App) loadSaveHistoryItems(cx *core.Scope) {
	a.reloadSaveHistory()
	if a.session.Current() == nil || len(a.saveHistoryIDs) == 0 {
		a.saveHistoryItems = nil
		a.saveHistoryStatus.Set("")
		a.bump()
		return
	}
	ids := append([]string(nil), a.saveHistoryIDs...)
	a.saveHistoryLoading.Set(true)
	a.saveHistoryStatus.Set("正在同步历史保存…")
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 20*time.Second)
		defer cancel()
		items := make([]model.ItemSummary, 0, len(ids))
		for _, id := range ids {
			if ctx.Err() != nil {
				break
			}
			detail, err := a.client.Item(ctx, id)
			if err != nil {
				continue
			}
			items = append(items, detail.Summary())
		}
		a.post(func() {
			a.saveHistoryLoading.Set(false)
			a.saveHistoryStatus.Set("")
			a.saveHistoryItems = items
			a.bump()
		})
	}()
}

func (a *App) recordSaveHistory(ids []string) {
	scope := a.session.ScopeKey()
	if scope == "" || len(ids) == 0 {
		return
	}
	_ = a.local.AddSaveHistory(scope, ids)
	a.reloadSaveHistory()
}

func (a *App) consumeShares(cx *core.Scope) {
	if a.env.Share == nil {
		return
	}
	if !a.sharesRunning.CompareAndSwap(false, true) {
		a.sharesRescan = true
		return
	}
	a.sharesBusy.Set(true)
	a.env.Share.TakePending(func(events []platform.ShareEvent, err error) {
		if err != nil {
			a.post(func() {
				a.sharesRunning.Store(false)
				a.sharesBusy.Set(false)
				a.showToast("接收分享失败: " + transport.DescribeFailure(err))
				if a.sharesRescan {
					a.sharesRescan = false
					a.consumeShares(cx)
				}
			})
			return
		}
		var ackIDs []string
		for _, ev := range events {
			if id := strings.TrimSpace(ev.ID); id != "" {
				ackIDs = append(ackIDs, id)
			}
		}
		finishTake := func() {
			a.post(func() {
				a.sharesRunning.Store(false)
				a.sharesBusy.Set(false)
				if len(events) > 0 {
					if a.session.Current() == nil {
						cleanupShareEvents(events)
						a.showToast("请先连接桌面端后再分享保存")
					} else if a.shareMode.Get() == storage.ShareModeChoose {
						a.pendingShareEvents = append(a.pendingShareEvents, events...)
						a.openShareFolderPicker(cx)
					} else {
						a.sendShareEventsToFolder(cx, events, "")
					}
				}
				if a.sharesRescan {
					a.sharesRescan = false
					a.consumeShares(cx)
				}
			})
		}
		if len(ackIDs) > 0 {
			a.env.Share.Ack(ackIDs, func(error) { finishTake() })
			return
		}
		finishTake()
	})
}

func (a *App) openShareFolderPicker(cx *core.Scope) {
	a.shareFolderPickerOpen.Set(true)
	if a.session.Current() != nil {
		a.shareFoldersLoading.Set(true)
		go func() {
			ctx, cancel := context.WithTimeout(a.rootCtx(), 10*time.Second)
			defer cancel()
			folders, err := a.client.Folders(ctx)
			a.post(func() {
				a.shareFoldersLoading.Set(false)
				if err == nil {
					a.folders = folders
					a.bump()
				}
			})
		}()
	}
}

func (a *App) shareFolderPickerDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open: func() bool { return a.shareFolderPickerOpen.Get() },
		OnClose: func() {
			a.shareFolderPickerOpen.Set(false)
			if len(a.pendingShareEvents) > 0 {
				cleanupShareEvents(a.pendingShareEvents)
				a.pendingShareEvents = nil
			}
		},
		Title: "选择保存目录",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.shareFolderPickerOpen.Get()
				_ = a.shareFoldersLoading.Get()

				pickFolder := func(folderID string) {
					events := a.pendingShareEvents
					a.pendingShareEvents = nil
					a.shareFolderPickerOpen.Set(false)
					if len(events) > 0 {
						a.sendShareEventsToFolder(cx, events, folderID)
					}
				}

				rows := []core.Node{
					core.View{
						Style:   drawerRowStyle(colors),
						OnPress: func(core.PressEvent) { pickFolder("") },
						Children: []core.Node{
							core.Icon{SVG: lucide.Inbox(), Style: core.Style{FontSize: 18, Color: colors.Accent()}},
							core.Text{Content: "全部 (当前资源库)", Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text(), FlexGrow: 1})},
						},
					},
				}
				if a.shareFoldersLoading.Get() && len(a.folders) == 0 {
					rows = append(rows, core.Text{
						Content: "正在加载目录…",
						Style:   nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()}),
					})
				}
				for _, f := range a.folders {
					folder := f
					rows = append(rows, core.View{
						Style:   drawerRowStyle(colors),
						OnPress: func(core.PressEvent) { pickFolder(folder.ID) },
						Children: []core.Node{
							core.Icon{SVG: lucide.Folder(), Style: core.Style{FontSize: 18, Color: colors.Muted()}},
							core.Text{Content: folder.Name, MaxLines: 1, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text(), FlexGrow: 1})},
							core.Text{Content: itoa(folder.ItemCount), Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})},
						},
					})
				}
				return core.Scroll{
					Style:    core.Style{FlexShrink: 1, MinHeight: core.Pt(0), MaxHeight: core.Pt(380)},
					Children: []core.Node{core.View{Style: core.Style{Gap: 6}, Children: rows}},
				}
			}),
		},
	})
}

func (a *App) sendShareEventsToFolder(cx *core.Scope, events []platform.ShareEvent, folderID string) {
	if len(events) == 0 {
		return
	}
	if a.session.Current() == nil {
		cleanupShareEvents(events)
		a.showToast("请先连接桌面端")
		return
	}
	a.activeMobileShares++
	if a.shareNotifyTimer != nil {
		a.shareNotifyTimer.Stop()
		a.shareNotifyTimer = nil
	}
	a.showToast("正在保存到电脑…")
	go func() {
		defer cleanupShareEvents(events)
		var savedIDs []string
		var firstName string
		var lastErr error
		for _, ev := range events {
			for _, item := range ev.Items {
				ctx, cancel := context.WithTimeout(a.rootCtx(), 5*time.Minute)
				var (
					res model.UploadResult
					err error
				)
				if strings.EqualFold(item.Kind, "text") {
					res, err = a.client.ShareText(ctx, item.Text, item.Name, folderID)
				} else if path := strings.TrimSpace(item.Path); path != "" {
					res, err = a.client.UploadFileToFolder(ctx, path, folderID)
				}
				cancel()
				if err != nil {
					lastErr = err
					continue
				}
				for _, saved := range res.Items {
					if saved.ID != "" {
						savedIDs = append(savedIDs, saved.ID)
					}
					if firstName == "" && saved.Name != "" {
						firstName = saved.Name
					}
				}
				a.post(func() { a.recordShareTag(res, "") })
			}
		}
		a.post(func() {
			if a.activeMobileShares > 0 {
				a.activeMobileShares--
			}
			if len(savedIDs) > 0 {
				a.recordSaveHistory(savedIDs)
				a.scheduleCalibratedRefresh(cx, 100*time.Millisecond)
				if a.screen.Get() == screenInbox {
					a.loadSaveHistoryItems(cx)
				}
				a.shareNotifyCount += len(savedIDs)
				if a.shareNotifyFirst == "" {
					a.shareNotifyFirst = firstName
				}
			} else if lastErr != nil {
				a.showToast("保存失败: " + transport.DescribeFailure(lastErr))
			}
			if a.activeMobileShares == 0 && a.shareNotifyCount > 0 {
				if a.shareNotifyTimer != nil {
					a.shareNotifyTimer.Stop()
				}
				a.shareNotifyTimer = time.AfterFunc(1500*time.Millisecond, func() {
					a.post(func() { a.flushMobileSaveNotify(cx) })
				})
			}
		})
	}()
}

func (a *App) flushMobileSaveNotify(cx *core.Scope) {
	if a.activeMobileShares > 0 || a.shareNotifyCount == 0 {
		return
	}
	count := a.shareNotifyCount
	firstName := a.shareNotifyFirst
	a.shareNotifyCount = 0
	a.shareNotifyFirst = ""
	a.shareNotifyTimer = nil
	a.scheduleCalibratedRefresh(cx, 50*time.Millisecond)
	a.notifyMobileSaved(cx, count, firstName)
}

func (a *App) notifyMobileSaved(cx *core.Scope, count int, firstName string) {
	if count <= 0 {
		return
	}
	msg := "已保存到电脑"
	if count == 1 && strings.TrimSpace(firstName) != "" {
		msg = "已保存「" + firstName + "」"
	} else if count > 1 {
		msg = "已保存 " + itoa(count) + " 项到电脑"
	}
	a.showToast(msg)
	if a.shareNotify.Get() && a.env.HasHost && cx.Env().OS() == core.OSAndroid {
		cx.CallHost("notify.show", map[string]any{
			"title": "已保存到电脑",
			"body":  msg,
		}, nil)
	}
}

func cleanupShareEvents(events []platform.ShareEvent) {
	for _, ev := range events {
		for _, item := range ev.Items {
			if p := strings.TrimSpace(item.Path); p != "" {
				_ = os.Remove(p)
			}
		}
	}
}

func (a *App) recordShareTag(result model.UploadResult, shaHex string) {
	scope := a.session.ScopeKey()
	if scope == "" {
		return
	}
	tags, err := a.local.LoadShareTags()
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	if len(result.Items) == 0 && shaHex != "" {
		tags.Tags = append([]storage.ShareTag{{ItemID: shaHex, Hex: shaHex, Scope: scope, CreatedAt: now}}, tags.Tags...)
	} else {
		for _, item := range result.Items {
			h := item.Hex
			if h == "" {
				h = shaHex
			}
			id := item.ID
			if id == "" {
				id = h
			}
			tags.Tags = append([]storage.ShareTag{{ItemID: id, Hex: h, Scope: scope, CreatedAt: now}}, tags.Tags...)
		}
	}
	_ = a.local.SaveShareTags(tags)
	a.reloadShareTags()
}
