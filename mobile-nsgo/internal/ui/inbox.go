package ui

import (
	"context"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func (a *App) inboxPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	a.post(func() { a.reloadInbox() })
	_ = a.rev.Get()
	back := func() {
		if (a.inboxBackScreen == screenConnect || a.inboxBackScreen == screenWelcome || a.inboxBackScreen == screenDevices) && a.session.Current() == nil {
			a.screen.Set(a.inboxBackScreen)
		} else {
			a.screen.Set(screenGallery)
		}
	}
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
		nsui.Button{Text: "返回", Variant: nsui.ButtonGhost, OnPress: back},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{Content: "收件箱", Style: nsui.TypeTitle, StyleFunc: nsui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string {
				a.rev.Get()
				return inboxCount(a.inboxEntries)
			}, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
		}},
		core.Show{When: func() bool { return a.inboxBusy.Get() }, Then: func(cx *core.Scope) core.Node {
			return nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.cancelInboxUpload() }}
		}},
		core.Show{When: func() bool { return !a.inboxBusy.Get() }, Then: func(cx *core.Scope) core.Node {
			return nsui.Button{Text: "全部上传", Variant: nsui.ButtonSecondary, OnPress: func() { a.uploadAll(cx) },
				Disabled: func() bool { a.rev.Get(); return len(a.inboxEntries) == 0 }, TestID: "inbox.uploadAll"}
		}},
	}}
	status := core.Show{When: func() bool { return a.inboxStatus.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: a.inboxStatus.Get, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}
	}}
	body := core.List{
		Count: func() int { a.rev.Get(); return len(a.inboxEntries) },
		Key:   func(i int) any { a.rev.Get(); return a.inboxEntries[i].ID },
		Row:   func(cx *core.Scope, row core.ListRow) core.Node { return a.inboxRow(cx, row) },
		Gap:   8, EstimatedSize: 82,
		Empty: core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 8, Padding: core.All(24)}, Children: []core.Node{
			core.Text{Content: "收件箱是空的", Style: nsui.TypeBody.Merge(core.Style{Color: colors.Muted()})},
			core.Text{Content: "从其他应用分享图片、视频或文字到 Bowerbird，会先保存在这里。",
				MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Faint(), TextAlign: core.TextAlignCenter})},
		}},
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
		header, status, body, a.inboxDeleteDialog(cx), a.inboxTextDialog(cx),
	}}
}

func (a *App) inboxRow(cx *core.Scope, row core.ListRow) core.Node {
	colors := nsui.ColorsOf(cx)
	entryAt := func() (storage.InboxEntry, bool) {
		a.rev.Get()
		index := row.Index()
		if index < 0 || index >= len(a.inboxEntries) {
			return storage.InboxEntry{}, false
		}
		return a.inboxEntries[index], true
	}
	name := func() string {
		entry, ok := entryAt()
		if !ok {
			return ""
		}
		return entry.Name
	}
	meta := func() string {
		entry, ok := entryAt()
		if !ok {
			return ""
		}
		kind := "文件"
		if entry.Kind == "text" {
			kind = "文本 · " + shortText(entry.Text, 40)
		}
		size := ""
		if entry.Size > 0 {
			size = " · " + model.FormatSize(entry.Size)
		}
		if entry.Attempts > 0 && entry.LastError != "" {
			return kind + size + " · 上次失败: " + entry.LastError
		}
		return kind + size
	}
	return core.View{
		StyleFunc: func() core.Style {
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
				Padding: core.All(10), CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
		},
		OnPress: func(core.PressEvent) {
			entry, ok := entryAt()
			if ok && entry.Kind == "text" {
				a.inboxPreviewEntry = entry
				a.inboxTextOpen.Set(true)
				a.bump()
			}
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: name, MaxLines: 1, Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
				core.Text{ContentFunc: meta, MaxLines: 2, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
			}},
			nsui.Button{Text: "上传", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.inboxBusy.Get() }, OnPress: func() {
				entry, ok := entryAt()
				if ok {
					a.uploadOne(cx, entry)
				}
			}},
			nsui.Button{Text: "删除", Variant: nsui.ButtonGhost, OnPress: func() {
				entry, ok := entryAt()
				if !ok {
					return
				}
				a.inboxDeleteEntry = entry
				a.inboxConfirmDel.Set(true)
				a.bump()
			}},
		},
	}
}

func (a *App) inboxDeleteDialog(cx *core.Scope) core.Node {
	return nsui.Dialog{
		Open:         func() bool { return a.inboxConfirmDel.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.inboxConfirmDel.Set(open) },
		Title:        "删除本机收件？",
		Description:  "将删除「" + a.inboxDeleteEntry.Name + "」及其本机暂存文件，无法恢复。",
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.inboxConfirmDel.Set(false) }},
			nsui.Button{Text: "删除", OnPress: func() {
				a.inboxConfirmDel.Set(false)
				if err := a.inbox.Remove(a.inboxDeleteEntry); err != nil {
					a.showToast(err.Error())
					return
				}
				a.reloadInbox()
				a.showToast("已删除本机收件")
			}},
		},
	}
}

func (a *App) inboxTextDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return nsui.Dialog{
		Open:         func() bool { return a.inboxTextOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) { a.inboxTextOpen.Set(open) },
		Title:        "文本分享内容",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				_ = a.rev.Get()
				_ = a.inboxTextOpen.Get()
				return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
					core.Text{Content: a.inboxPreviewEntry.Name, Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
					core.Scroll{
						Style: core.Style{MaxHeight: core.Pt(260), Padding: core.All(10), CornerRadius: 8, Background: colors.Inset()},
						Children: []core.Node{
							core.Text{Content: a.inboxPreviewEntry.Text, Style: nsui.TypeBody.Merge(core.Style{Color: colors.Text()})},
						},
					},
				}}
			}),
		},
		Footer: []core.Node{
			nsui.Button{Text: "复制文字", Variant: nsui.ButtonSecondary, OnPress: func() {
				cx.SetClipboard(a.inboxPreviewEntry.Text)
				a.showToast("已复制文字")
			}},
			nsui.Button{Text: "上传为 .txt", Disabled: func() bool { return a.inboxBusy.Get() }, OnPress: func() {
				a.inboxTextOpen.Set(false)
				a.uploadOne(cx, a.inboxPreviewEntry)
			}},
			nsui.Button{Text: "关闭", Variant: nsui.ButtonGhost, OnPress: func() { a.inboxTextOpen.Set(false) }},
		},
	}
}

func inboxCount(entries []storage.InboxEntry) string {
	if len(entries) == 0 {
		return "没有待上传内容"
	}
	return itoa(len(entries)) + " 项待上传"
}

func (a *App) reloadInbox() {
	entries, err := a.inbox.Entries()
	if err != nil {
		a.inboxStatus.Set("读取收件箱失败: " + err.Error())
		return
	}
	a.inboxEntries = entries
	a.bump()
}

func (a *App) consumeShares(cx *core.Scope) {
	if a.env.Share == nil || !a.sharesRunning.CompareAndSwap(false, true) {
		return
	}
	a.sharesBusy.Set(true)
	a.inbox.Consume(a.rootCtx(), a.env.Share, func(added int, err error) {
		a.post(func() {
			a.sharesRunning.Store(false)
			a.sharesBusy.Set(false)
			a.reloadInbox()
			if err != nil {
				a.showToast("接收分享失败: " + transport.DescribeFailure(err))
				return
			}
			if added > 0 {
				a.showToast("已收进 " + itoa(added) + " 项，可在收件箱上传")
			}
		})
	})
}

func (a *App) cancelInboxUpload() {
	a.inboxUploadMu.Lock()
	cancel := a.inboxUploadCancel
	a.inboxUploadMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) setInboxCancel(cancel context.CancelFunc) {
	a.inboxUploadMu.Lock()
	a.inboxUploadCancel = cancel
	a.inboxUploadMu.Unlock()
}

func (a *App) uploadOne(cx *core.Scope, entry storage.InboxEntry) {
	if a.session.Current() == nil {
		a.showToast("请先连接桌面端")
		return
	}
	a.inboxBusy.Set(true)
	a.inboxStatus.Set("正在上传 " + entry.Name + "…")
	ctx, cancel := context.WithTimeout(a.rootCtx(), 5*time.Minute)
	a.setInboxCancel(cancel)
	go func() {
		defer cancel()
		result, shaHex, err := a.inbox.Upload(ctx, a.client, entry)
		a.post(func() {
			a.setInboxCancel(nil)
			a.inboxBusy.Set(false)
			a.reloadInbox()
			if err != nil {
				a.markInboxError(entry, err)
				a.inboxStatus.Set("上传失败: " + transport.DescribeFailure(err))
				a.showToast("上传失败，内容仍在收件箱")
				return
			}
			a.recordShareTag(result, shaHex)
			a.inboxStatus.Set("已上传 " + entry.Name)
			a.showToast("已上传")
			a.scheduleCalibratedRefresh(cx, 100*time.Millisecond)
		})
	}()
}

func (a *App) uploadAll(cx *core.Scope) {
	if a.inboxBusy.Get() {
		return
	}
	entries := append([]storage.InboxEntry(nil), a.inboxEntries...)
	if len(entries) == 0 {
		return
	}
	if a.session.Current() == nil {
		a.showToast("请先连接桌面端")
		return
	}
	a.inboxBusy.Set(true)
	ctx, cancel := context.WithCancel(a.rootCtx())
	a.setInboxCancel(cancel)
	go func() {
		defer cancel()
		okCount := 0
		var lastErr error
		for i, entry := range entries {
			if ctx.Err() != nil {
				lastErr = ctx.Err()
				break
			}
			a.post(func() { a.inboxStatus.Set("正在上传 " + itoa(i+1) + "/" + itoa(len(entries)) + "…") })
			itemCtx, itemCancel := context.WithTimeout(ctx, 5*time.Minute)
			result, shaHex, err := a.inbox.Upload(itemCtx, a.client, entry)
			itemCancel()
			if err != nil {
				lastErr = err
				a.post(func() { a.markInboxError(entry, err) })
				continue
			}
			okCount++
			a.post(func() { a.recordShareTag(result, shaHex) })
		}
		a.post(func() {
			a.setInboxCancel(nil)
			a.inboxBusy.Set(false)
			a.reloadInbox()
			switch {
			case lastErr == nil:
				a.inboxStatus.Set("全部上传完成")
				a.showToast("已上传 " + itoa(okCount) + " 项")
			case okCount == 0:
				a.inboxStatus.Set("上传失败: " + transport.DescribeFailure(lastErr))
				a.showToast("上传失败，内容仍在收件箱")
			default:
				a.inboxStatus.Set("部分上传成功，失败项仍在收件箱")
				a.showToast("成功 " + itoa(okCount) + " 项，失败 " + itoa(len(entries)-okCount) + " 项")
			}
			if okCount > 0 {
				a.scheduleCalibratedRefresh(cx, 100*time.Millisecond)
			}
		})
	}()
}

func (a *App) markInboxError(entry storage.InboxEntry, err error) {
	idx, loadErr := a.local.LoadInbox()
	if loadErr != nil {
		return
	}
	for i := range idx.Entries {
		if idx.Entries[i].ID == entry.ID {
			idx.Entries[i].Attempts++
			idx.Entries[i].LastError = transport.DescribeFailure(err)
			break
		}
	}
	_ = a.local.SaveInbox(idx)
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

func shortText(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}
