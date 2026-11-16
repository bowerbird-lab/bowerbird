package ui

import (
	"context"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/app"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/query"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

func (a *App) connect(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()
	history, _ := a.local.History()
	recent := make([]core.Node, 0, len(history))
	for _, c := range history {
		recent = append(recent, a.recentRow(cx, c))
	}
	return core.Scroll{
		Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
		Children: []core.Node{
			core.View{
				Style: core.Style{Width: core.Pct(100), Gap: 14, Padding: core.Edges{Top: 32, Bottom: 24}},
				Children: []core.Node{
					core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
						core.Text{Content: "Bowerbird", Style: nsui.TypeHeading, StyleFunc: nsui.Colored(colors.Text)},
						core.Text{Content: "连接电脑上的素材库", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Muted)},
					}},
					core.Show{When: func() bool { return a.legacyNotice.Get() != "" }, Then: func(cx *core.Scope) core.Node {
						return core.View{
							StyleFunc: func() core.Style {
								return core.Style{Padding: core.All(10), CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
							},
							Children: []core.Node{
								core.Text{ContentFunc: a.legacyNotice.Get, MaxLines: 3, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
							},
						}
					}},
					core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
						nsui.Input{Value: func() string { return a.address.Get() }, OnChange: a.address.Set,
							Placeholder: "电脑地址，例如 192.168.1.8:17878", TestID: "connect.address"},
						core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
							nsui.Button{Text: "连接", Flex: 1, OnPress: func() { a.submitConnect(cx) },
								Disabled: func() bool { return a.busy.Get() }, TestID: "connect.submit"},
							nsui.Button{Text: "发现", Variant: nsui.ButtonSecondary, OnPress: func() { a.discover(cx) },
								Disabled: func() bool { return a.discovering.Get() }, TestID: "connect.discover"},
							nsui.Button{Text: "扫码", Variant: nsui.ButtonSecondary, OnPress: func() { a.scanQR(cx) },
								Disabled: func() bool { return a.busy.Get() }, TestID: "connect.scan"},
						}},
						core.Show{When: func() bool { return a.formErr.Get() != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{ContentFunc: a.formErr.Get, MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Danger()})}
						}},
						core.Show{When: func() bool { return a.connectStatus.Get() != "" }, Then: func(cx *core.Scope) core.Node {
							return core.Text{ContentFunc: a.connectStatus.Get, MaxLines: 3, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Muted()})}
						}},
					}},
					core.Show{When: func() bool { a.rev.Get(); return len(a.services) > 0 }, Then: func(cx *core.Scope) core.Node {
						rows := make([]core.Node, 0, len(a.services))
						for _, svc := range a.services {
							rows = append(rows, a.serviceRow(cx, svc))
						}
						return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
							core.Text{Content: "发现的桌面端", Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
							core.View{Style: core.Style{Gap: 6}, Children: rows},
						}}
					}},
					core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
						core.Text{Content: "历史连接", Style: nsui.TypeLabel.Merge(core.Style{Color: colors.Muted()})},
						core.Show{When: func() bool { a.rev.Get(); return len(recent) == 0 }, Then: func(cx *core.Scope) core.Node {
							return core.Text{Content: "还没有连接记录", Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Faint()})}
						}},
						core.View{Style: core.Style{Gap: 6}, Children: recent},
					}},
					core.Show{When: func() bool { a.rev.Get(); return len(a.inboxEntries) > 0 }, Then: func(cx *core.Scope) core.Node {
						return core.View{
							Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.All(10),
								CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()},
							Children: []core.Node{
								core.Text{Content: "收件箱中有 " + itoa(len(a.inboxEntries)) + " 项待上传内容",
									Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Text(), FlexGrow: 1})},
								nsui.Button{Text: "查看", Variant: nsui.ButtonGhost, OnPress: func() {
									a.inboxBackScreen = screenConnect
									a.screen.Set(screenInbox)
								}},
							},
						}
					}},
					a.pairDialog(cx),
				},
			},
		},
	}
}

func (a *App) recentRow(cx *core.Scope, c storage.Connection) core.Node {
	colors := nsui.ColorsOf(cx)
	name := c.LibraryName
	if strings.TrimSpace(name) == "" {
		name = c.BaseURL
	}
	return core.View{
		StyleFunc: func() core.Style {
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
				Padding: core.All(10), CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{Content: name, MaxLines: 1, Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
				core.Text{Content: c.BaseURL, MaxLines: 1, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
			}},
			nsui.Button{Text: "重连", Variant: nsui.ButtonSecondary, Disabled: func() bool { return a.busy.Get() },
				OnPress: func() { a.reconnect(cx, c) }, TestID: "connect.reconnect"},
			nsui.Button{Text: "删除", Variant: nsui.ButtonGhost, OnPress: func() {
				if err := a.session.ForgetConnection(c.BaseURL); err != nil {
					a.formErr.Set(err.Error())
				}
				a.bump()
			}},
		},
	}
}

func (a *App) serviceRow(cx *core.Scope, svc platform.Service) core.Node {
	colors := nsui.ColorsOf(cx)
	address := ""
	if len(svc.Hosts) > 0 {
		host := strings.TrimSuffix(svc.Hosts[0], ":")
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			host = "[" + host + "]"
		}
		address = host + ":" + itoa(svc.Port)
	}
	sub := address
	if svc.Version != "" {
		sub += " · v" + svc.Version
	}
	if !svc.PairingRequired {
		sub += " · 免配对"
	}
	return core.View{
		StyleFunc: func() core.Style {
			return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
				Padding: core.All(10), CornerRadius: 10, Background: colors.Surface(), BorderWidth: 1, BorderColor: colors.Border()}
		},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{Content: svc.Name, MaxLines: 1, Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Text)},
				core.Text{Content: sub, MaxLines: 1, Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
			}},
			nsui.Button{Text: "使用", Variant: nsui.ButtonSecondary, OnPress: func() {
				a.address.Set(address)
				a.submitConnect(cx)
			}},
		},
	}
}

func (a *App) pairDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return nsui.Dialog{
		Open: func() bool { return a.pairOpen.Get() },
		OnOpenChange: func(open bool, _ base.Reason) {
			a.pairOpen.Set(open)
			if !open {
				a.code.Set("")
			}
		},
		Title:       "输入配对码",
		Description: "在电脑的 Bowerbird 设置里查看 6 位配对码",
		Children: []core.Node{
			nsui.Input{Value: func() string { return a.code.Get() }, OnChange: a.code.Set,
				Placeholder: "6 位配对码", MaxLength: 6, TestID: "connect.code"},
			core.Show{When: func() bool { return a.formErr.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: a.formErr.Get, Style: nsui.TypeCaption.Merge(core.Style{Color: colors.Danger()})}
			}},
		},
		Footer: []core.Node{
			nsui.Button{Text: "取消", Variant: nsui.ButtonGhost, OnPress: func() { a.pairOpen.Set(false) }},
			nsui.Button{Text: "配对", OnPress: func() { a.submitConnect(cx) }, Disabled: func() bool { return a.busy.Get() }, TestID: "connect.pair"},
		},
		TestID: "connect.dialog",
	}
}

func (a *App) scanQR(cx *core.Scope) {
	if a.env.Scanner == nil {
		a.formErr.Set("当前平台暂不支持相机扫码，请手动输入地址或使用发现功能")
		return
	}
	a.formErr.Set("")
	a.env.Scanner.Scan(func(payload string, cancelled bool, err error) {
		a.post(func() {
			if err != nil {
				a.formErr.Set("扫码失败: " + err.Error())
				return
			}
			if cancelled || strings.TrimSpace(payload) == "" {
				return
			}
			a.handlePairURL(cx, payload)
		})
	})
}

func (a *App) discover(cx *core.Scope) {
	if a.env.Discovery == nil || a.discovering.Get() {
		return
	}
	a.discovering.Set(true)
	a.formErr.Set("")
	a.env.Discovery.Browse(a.rootCtx(), func(list []platform.Service, err error) {
		a.post(func() {
			a.discovering.Set(false)
			if err != nil {
				a.formErr.Set(transport.DescribeFailure(err))
				return
			}
			a.services = list
			if len(list) == 0 {
				a.formErr.Set("没有发现桌面端，请检查网络或手动输入地址")
			}
			a.bump()
		})
	})
}

func (a *App) submitConnect(cx *core.Scope) {
	if a.busy.Get() {
		return
	}
	address := strings.TrimSpace(a.address.Get())
	if address == "" {
		a.formErr.Set("请输入电脑地址")
		return
	}
	a.busy.Set(true)
	a.formErr.Set("")
	code := strings.TrimSpace(a.code.Get())
	device := a.env.Device.Name
	if device == "" {
		device = "Bowerbird Mobile"
	}
	go func() {
		ctx, cancel := context.WithTimeout(a.rootCtx(), 25*time.Second)
		defer cancel()
		session, err := a.session.Connect(ctx, address, code, device)
		a.post(func() {
			a.busy.Set(false)
			if err == nil {
				a.onSessionConnected(cx, session)
				return
			}
			if err == app.ErrPairingRequired {
				a.pairOpen.Set(true)
				a.formErr.Set("")
				return
			}
			if transport.IsUnauthorized(err) {
				a.pairOpen.Set(true)
				a.formErr.Set("配对码错误或已过期，请核对后重试")
				return
			}
			a.formErr.Set(transport.DescribeFailure(err))
		})
	}()
}

func (a *App) reconnect(cx *core.Scope, c storage.Connection) {
	if a.busy.Get() {
		return
	}
	a.busy.Set(true)
	a.formErr.Set("")
	a.session.LoadConnectionToken(c, func(tok string) {
		c.Token = tok
		go func() {
			ctx, cancel := context.WithTimeout(a.rootCtx(), 20*time.Second)
			defer cancel()
			session, err := a.session.ConnectWithToken(ctx, c)
			a.post(func() {
				a.busy.Set(false)
				if err != nil {
					if err == app.ErrPairingRequired || transport.IsUnauthorized(err) {
						a.address.Set(c.BaseURL)
						a.pairOpen.Set(true)
						a.formErr.Set("连接凭据已失效，请重新输入配对码")
						return
					}
					a.formErr.Set(transport.DescribeFailure(err))
					return
				}
				a.onSessionConnected(cx, session)
			})
		}()
	})
}

func (a *App) onSessionConnected(cx *core.Scope, session *storage.Session) {
	a.address.Set(session.BaseURL)
	a.pairOpen.Set(false)
	a.code.Set("")
	a.legacyNotice.Set("")
	a.sidebarView = SidebarAll
	a.activeFolderID = ""
	a.galleryFilter = query.Spec{}
	a.reloadShareTags()
	a.reloadUsageIDs()
	a.screen.Set(screenGallery)
	a.refreshGalleryView(cx, true)
	a.loadSidebarData(cx)
	a.startWS(cx)
	a.bump()
}

func (a *App) disconnect(cx *core.Scope) {
	a.stopWS()
	_ = a.session.Disconnect()
	a.resetSessionState()
	a.screen.Set(screenConnect)
	a.bump()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
