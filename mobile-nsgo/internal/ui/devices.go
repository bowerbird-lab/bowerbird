package ui

import (
	_ "embed"
	"net/url"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
)

//go:embed assets/card_style_1.png
var cardStyle1PNG []byte

//go:embed assets/card_style_2.png
var cardStyle2PNG []byte

//go:embed assets/card_style_3.png
var cardStyle3PNG []byte

//go:embed assets/card_style_4.png
var cardStyle4PNG []byte

//go:embed assets/card_style_5.png
var cardStyle5PNG []byte

//go:embed assets/card_style_6.png
var cardStyle6PNG []byte

var deviceCardStyles = []struct {
	label string
	data  []byte
}{
	{label: "视频拍档", data: cardStyle1PNG},
	{label: "文件夹色卡", data: cardStyle2PNG},
	{label: "音乐耳机", data: cardStyle3PNG},
	{label: "图片画框", data: cardStyle4PNG},
	{label: "桌面电脑", data: cardStyle5PNG},
	{label: "创意画笔", data: cardStyle6PNG},
}

func deviceStyleKey(c storage.Connection) string {
	if strings.TrimSpace(c.DeviceID) != "" {
		return "device:" + strings.TrimSpace(c.DeviceID)
	}
	return c.BaseURL
}

func (a *App) cardStyleFor(baseURL string, fallbackIdx int) int {
	if idx, ok := a.cardStyleIdx[baseURL]; ok && idx >= 0 && idx < len(deviceCardStyles) {
		return idx
	}
	if len(deviceCardStyles) == 0 {
		return 0
	}
	return fallbackIdx % len(deviceCardStyles)
}

func (a *App) cardStyleForConn(c storage.Connection, fallbackIdx int) int {
	if key := deviceStyleKey(c); key != "" {
		if idx, ok := a.cardStyleIdx[key]; ok && idx >= 0 && idx < len(deviceCardStyles) {
			return idx
		}
	}
	return a.cardStyleFor(c.BaseURL, fallbackIdx)
}

func (a *App) entryScreen() string {
	if history, err := a.local.History(); err == nil && len(history) > 0 {
		return screenDevices
	}
	return screenWelcome
}

func (a *App) openAddDeviceWelcome() {
	a.formErr.Set("")
	a.connectStatus.Set("")
	a.screen.Set(screenWelcome)
	a.bump()
}

func (a *App) openManualConnect() {
	a.formErr.Set("")
	a.connectStatus.Set("")
	a.screen.Set(screenConnect)
	a.bump()
}

func (a *App) triggerDropAnimation(baseURL string) {
	a.dropAnimURL = baseURL
	a.dropAnimActive.Set(true)
	if a.scope == nil {
		a.dropAnimActive.Set(false)
		return
	}
	a.after(32*time.Millisecond, func() {
		a.dropAnimActive.Set(false)
	})
}

func (a *App) onScannedConnected(cx *core.Scope, session *storage.Session) {
	a.pairOpen.Set(false)
	a.code.Set("")
	a.legacyNotice.Set("")

	conn := storage.Connection{
		DeviceID:        session.DeviceID,
		Name:            "新建的电脑",
		BaseURL:         session.BaseURL,
		Token:           session.Token,
		CandidateIPs:    session.CandidateIPs,
		Port:            session.Port,
		LibraryID:       session.LibraryID,
		LibraryName:     session.LibraryName,
		LastConnectedAt: time.Now().UnixMilli(),
	}
	// Preserve existing custom name if this device or IP was already saved.
	if history, err := a.local.History(); err == nil {
		for _, old := range history {
			sameDev := conn.DeviceID != "" && old.DeviceID == conn.DeviceID
			if (sameDev || sameOriginURL(old.BaseURL, session.BaseURL)) && strings.TrimSpace(old.Name) != "" {
				conn.Name = old.Name
				break
			}
		}
	}
	_ = a.local.Remember(conn)
	// Keep the user on the computer selection page rather than jumping straight into the gallery.
	_ = a.session.Disconnect()

	a.screen.Set(screenDevices)
	a.bump()
	a.triggerDropAnimation(conn.BaseURL)
	a.openNewDeviceDialog(conn)
}

func (a *App) openNewDeviceDialog(c storage.Connection) {
	a.deviceEditConn = c
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = "新建的电脑"
	}
	a.deviceEditName.Set(name)
	a.deviceEditStyle.Set(a.cardStyleForConn(c, 1)) // 默认文件夹/色卡形象
	a.deviceEditIsNew.Set(true)
	a.newDeviceOpen.Set(true)
}

func sameOriginURL(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

func (a *App) openDeviceEdit(c storage.Connection, fallbackIdx int, isNew bool) {
	a.deviceEditConn = c
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = "新建的电脑"
	}
	a.deviceEditName.Set(name)
	a.deviceEditStyle.Set(a.cardStyleForConn(c, fallbackIdx))
	a.deviceEditIsNew.Set(isNew)
	a.bump()
	a.deviceEditOpen.Set(true)
}

func hostLabelFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	return u.Host
}

func (a *App) devicesPage(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)

	return core.View{
		TestID: "screen.devices",
		StyleFunc: func() core.Style {
			return core.Style{
				Width:      core.Pct(100),
				Height:     core.Pct(100),
				Background: colors.Background(),
			}
		},
		Children: []core.Node{
			// Header: centered brand pill only (no left/right scan/connect buttons)
			core.View{
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					return core.Style{
						Width:          core.Pct(100),
						Direction:      core.Row,
						AlignItems:     core.AlignCenter,
						JustifyContent: core.JustifyCenter,
						Padding: core.Edges{
							Top:    safe.Top + 12,
							Bottom: 14,
							Left:   safe.Left + 16,
							Right:  safe.Right + 16,
						},
					}
				},
				Children: []core.Node{
					// Center brand pill
					core.View{
						StyleFunc: func() core.Style {
							return core.Style{
								Padding:      core.Symmetric(6, 18),
								CornerRadius: 999,
								Background:   colors.Surface(),
								BorderWidth:  1,
								BorderColor:  colors.Border(),
							}
						},
						Children: []core.Node{
							core.Text{
								Content: "bowerbird",
								Style: core.Style{
									FontFamily:    "ui-rounded, sans-serif-rounded, Arial Rounded MT Bold, sans-serif",
									FontSize:      16,
									LineHeight:    20,
									FontWeight:    800,
									LetterSpacing: -0.5,
									Color:         colors.Text(),
								},
							},
						},
					},
				},
			},

			// Rounded top-corner sheet containing computer cards
			core.View{
				StyleFunc: func() core.Style {
					return core.Style{
						FlexGrow:    1,
						FlexShrink:  1,
						Width:       core.Pct(100),
						MinHeight:   core.Pt(0),
						Background:  colors.Surface(),
						CornerRadii: core.Corners{TopLeft: 54, TopRight: 54},
						BorderCurve: core.BorderCurveContinuous,
					}
				},
				Children: []core.Node{
					core.Scroll{
						Style: core.Style{
							FlexGrow:   1,
							FlexShrink: 1,
							Width:      core.Pct(100),
							MinHeight:  core.Pt(0),
						},
						Children: []core.Node{
							core.View{
								StyleFunc: func() core.Style {
									safe := cx.Env().SafeArea()
									return core.Style{
										Width:      core.Pct(100),
										AlignItems: core.AlignCenter,
										Gap:        16,
										Padding: core.Edges{
											Top:    36,
											Bottom: safe.Bottom + 96,
											Left:   safe.Left + 20,
											Right:  safe.Right + 20,
										},
									}
								},
								Children: []core.Node{
									core.Show{
										When: func() bool { return a.connectStatus.Get() != "" },
										Then: func(cx *core.Scope) core.Node {
											return core.Text{
												ContentFunc: a.connectStatus.Get,
												MaxLines:    2,
												Style: nsui.TypeCaption.Merge(core.Style{
													Color:     colors.Muted(),
													TextAlign: core.TextAlignCenter,
												}),
											}
										},
									},
									core.Show{
										When: func() bool { return a.formErr.Get() != "" },
										Then: func(cx *core.Scope) core.Node {
											return core.Text{
												ContentFunc: a.formErr.Get,
												MaxLines:    3,
												Style: nsui.TypeCaption.Merge(core.Style{
													Color:     colors.Danger(),
													TextAlign: core.TextAlignCenter,
												}),
											}
										},
									},
									core.Dynamic(func(cx *core.Scope) core.Node {
										_ = a.rev.Get()
										history, _ := a.local.History()
										if len(history) == 0 {
											return core.View{
												Style: core.Style{Padding: core.Symmetric(32, 16), AlignItems: core.AlignCenter, Gap: 8},
												Children: []core.Node{
													core.Icon{SVG: lucide.Monitor(), Style: core.Style{FontSize: 36, Color: colors.Faint()}},
													core.Text{Content: "还没有保存的电脑连接", Style: nsui.TypeBody, StyleFunc: nsui.Colored(colors.Muted)},
												},
											}
										}
										winW := cx.Env().Window().Width
										if winW <= 0 {
											winW = 390
										}
										safe := cx.Env().SafeArea()
										cardSize := (winW - safe.Left - safe.Right - 40 - 14) / 2
										if cardSize < 120 {
											cardSize = 120
										}
										rows := make([]core.Node, 0, (len(history)+1)/2)
										for i := 0; i < len(history); i += 2 {
											left := a.deviceCard(cx, history[i], i, cardSize)
											var right core.Node
											if i+1 < len(history) {
												right = a.deviceCard(cx, history[i+1], i+1, cardSize)
											} else {
												right = core.View{Style: core.Style{Width: core.Pt(cardSize), Height: core.Pt(cardSize)}}
											}
											rows = append(rows, core.View{
												Style: core.Style{
													Direction:      core.Row,
													Gap:            14,
													Width:          core.Pct(100),
													JustifyContent: core.JustifyCenter,
												},
												Children: []core.Node{left, right},
											})
										}
										return core.View{
											Style:    core.Style{Width: core.Pct(100), Gap: 14},
											Children: rows,
										}
									}),
								},
							},
						},
					},

					// Floating bottom pill button ("+ 添加电脑"), matching reference "+ Add Space"
					core.View{
						PointerEvents: core.PointerEventsBoxNone,
						StyleFunc: func() core.Style {
							safe := cx.Env().SafeArea()
							return core.Style{
								Position:       core.Absolute,
								Left:           core.Pt(0),
								Right:          core.Pt(0),
								Bottom:         core.Pt(safe.Bottom + 22),
								AlignItems:     core.AlignCenter,
								JustifyContent: core.JustifyCenter,
							}
						},
						Children: []core.Node{
							core.View{
								TestID: "devices.add",
								Role:   core.RoleButton,
								Label:  "添加电脑",
								StyleFunc: func() core.Style {
									return core.Style{
										Direction:      core.Row,
										AlignItems:     core.AlignCenter,
										JustifyContent: core.JustifyCenter,
										Gap:            8,
										Height:         core.Pt(mobileButtonHeight),
										Padding:        core.Symmetric(0, 24),
										CornerRadius:   mobileButtonRadius,
										Background:     colors.Text(),
										Pressed:        &core.Style{Opacity: core.Alpha(0.85)},
										Shadows: []core.BoxShadow{
											{
												Offset: core.Point{X: 0, Y: 6},
												Blur:   18,
												Color:  core.RGBA(0x000000, 0.18),
											},
										},
									}
								},
								OnPress: func(core.PressEvent) {
									a.openAddDeviceWelcome()
								},
								Children: []core.Node{
									core.Icon{
										SVG:   lucide.Plus(),
										Style: core.Style{FontSize: 18, Color: colors.Background()},
									},
									core.Text{
										Content: "添加电脑",
										Style: core.Style{
											FontSize:   15,
											LineHeight: 19,
											FontWeight: 600,
											Color:      colors.Background(),
										},
									},
								},
							},
						},
					},
				},
			},

			a.newDeviceDialog(cx),
			a.deviceEditSheet(cx),
			a.pairDialog(cx),
		},
	}
}

const (
	mobileButtonHeight = nsui.ButtonHeight * 1.5 // 42pt (1.5x default 28pt)
	mobileButtonRadius = core.Points(999)        // 默认全圆角
)

type mobileBtnSpec struct {
	TestID   string
	Text     string
	Icon     func() *core.SVG
	Variant  nsui.ButtonVariant
	Flex     float32
	Disabled func() bool
	OnPress  func()
}

func mobileActionButton(colors nsui.Colors, spec mobileBtnSpec) core.Node {
	return core.View{
		TestID: spec.TestID,
		Role:   core.RoleButton,
		Label:  spec.Text,
		StyleFunc: func() core.Style {
			bg := colors.Accent()
			border := core.Color(0)
			switch spec.Variant {
			case nsui.ButtonSecondary:
				bg = colors.Inset()
				border = colors.Border()
			case nsui.ButtonOutline:
				bg = 0
				border = colors.Border()
			case nsui.ButtonGhost:
				bg = 0
			}
			st := core.Style{
				Height:         core.Pt(mobileButtonHeight),
				Padding:        core.Symmetric(0, 18),
				CornerRadius:   mobileButtonRadius,
				Direction:      core.Row,
				AlignItems:     core.AlignCenter,
				JustifyContent: core.JustifyCenter,
				Gap:            6,
				Background:     bg,
				Pressed:        &core.Style{Opacity: core.Alpha(0.78)},
			}
			if spec.Flex > 0 {
				st.FlexGrow = spec.Flex
				st.FlexShrink = 1
			}
			if border != 0 {
				st.BorderWidth = 1
				st.BorderColor = border
			}
			if spec.Disabled != nil && spec.Disabled() {
				st.Opacity = core.Alpha(0.5)
			}
			return st
		},
		OnPress: func(core.PressEvent) {
			if spec.Disabled != nil && spec.Disabled() {
				return
			}
			if spec.OnPress != nil {
				spec.OnPress()
			}
		},
		Children: func() []core.Node {
			fg := func() core.Color {
				if spec.Variant == nsui.ButtonPrimary {
					return colors.OnAccent()
				}
				return colors.Text()
			}
			var out []core.Node
			if spec.Icon != nil {
				out = append(out, core.Icon{
					SVG:       spec.Icon(),
					StyleFunc: func() core.Style { return core.Style{FontSize: 16, Color: fg()} },
				})
			}
			out = append(out, core.Text{
				Content:   spec.Text,
				StyleFunc: func() core.Style { return core.Style{FontSize: 14, LineHeight: 18, FontWeight: 600, Color: fg()} },
			})
			return out
		}(),
	}
}

func headerActionPill(colors nsui.Colors, testID string, iconFn func() *core.SVG, label string, onPress func()) core.Node {
	return mobileActionButton(colors, mobileBtnSpec{
		TestID:  testID,
		Text:    label,
		Icon:    iconFn,
		Variant: nsui.ButtonSecondary,
		OnPress: onPress,
	})
}

func (a *App) deviceCard(cx *core.Scope, c storage.Connection, idx int, cardSize core.Points) core.Node {
	colors := nsui.ColorsOf(cx)
	title := strings.TrimSpace(c.Name)
	if title == "" {
		title = strings.TrimSpace(c.LibraryName)
	}
	if title == "" {
		title = "新建的电脑"
	}
	mascotSize := cardSize * 0.58

	return core.View{
		StyleFunc: func() core.Style {
			st := core.Style{
				Width:  core.Pt(cardSize),
				Height: core.Pt(cardSize),
				Transition: core.Transition{
					Duration:   420 * time.Millisecond,
					Easing:     core.EaseOut,
					Properties: core.TransitionTransform,
				},
			}
			if sameOriginURL(c.BaseURL, a.dropAnimURL) && a.dropAnimActive.Get() {
				st.Transform = core.Transform{
					TranslateY: -24,
					ScaleX:     0.92,
					ScaleY:     0.92,
				}
			}
			return st
		},
		Children: []core.Node{
			// Simplified squircle card: status pill + name + floating right mascot PNG (IP hidden until long press)
			core.View{
				TestID: "devices.card." + itoa(idx),
				Role:   core.RoleButton,
				Label:  title,
				StyleFunc: func() core.Style {
					st := core.Style{
						Width:          core.Pt(cardSize),
						Height:         core.Pt(cardSize),
						CornerRadius:   40,
						BorderCurve:    core.BorderCurveContinuous,
						Overflow:       core.Hidden,
						Padding:        core.All(16),
						Background:     colors.Inset(),
						JustifyContent: core.JustifySpaceBetween,
						Pressed:        &core.Style{Opacity: core.Alpha(0.82)},
					}
					if a.busy.Get() {
						st.Opacity = core.Alpha(0.6)
					}
					return st
				},
				OnPress: func(core.PressEvent) {
					a.reconnect(cx, c)
				},
				OnLongPress: func(core.PressEvent) {
					a.openDeviceEdit(c, idx, false)
				},
				Children: []core.Node{
					// Floating transparent mascot PNG on the right side (updates live via cardStyleRev)
					core.Image{
						SourceFunc: func() core.ImageSource {
							_ = a.cardStyleRev.Get()
							sIdx := a.cardStyleForConn(c, idx)
							return core.ImageSource{Data: deviceCardStyles[sIdx].data}
						},
						ResizeMode:     core.Contain,
						ElementsHidden: true,
						Style: core.Style{
							Position: core.Absolute,
							Right:    core.Pt(-4),
							Bottom:   core.Pt(8),
							Width:    core.Pt(mascotSize),
							Height:   core.Pt(mascotSize),
						},
					},

					// Top row: status badge only
					core.View{
						Style: core.Style{
							Direction:  core.Row,
							AlignItems: core.AlignCenter,
							ZIndex:     2,
						},
						Children: []core.Node{
							core.View{
								StyleFunc: func() core.Style {
									bg := colors.Surface()
									if idx == 0 {
										bg = colors.Accent().Faded(0.16)
									}
									return core.Style{
										Direction:    core.Row,
										AlignItems:   core.AlignCenter,
										Gap:          4,
										Padding:      core.Symmetric(3, 8),
										CornerRadius: 999,
										Background:   bg,
									}
								},
								Children: []core.Node{
									core.View{
										StyleFunc: func() core.Style {
											dot := colors.Muted()
											if idx == 0 {
												dot = colors.Accent()
											}
											return core.Style{
												Width:        core.Pt(6),
												Height:       core.Pt(6),
												CornerRadius: 3,
												Background:   dot,
											}
										},
									},
									core.Text{
										Content: func() string {
											if idx == 0 {
												return "最近"
											}
											return "已存"
										}(),
										StyleFunc: func() core.Style {
											fg := colors.Muted()
											if idx == 0 {
												fg = colors.Accent()
											}
											return core.Style{FontSize: 11, LineHeight: 14, FontWeight: 600, Color: fg}
										},
									},
								},
							},
						},
					},

					// Bottom-left Name only (IP hidden until long-press edit sheet)
					core.View{
						Style: core.Style{
							Width:      core.Pct(100),
							AlignItems: core.AlignStart,
							Padding:    core.Edges{Right: cardSize * 0.34},
							ZIndex:     2,
						},
						Children: []core.Node{
							core.Text{
								Content:   title,
								MaxLines:  2,
								Style:     core.Style{FontSize: 16, LineHeight: 20, FontWeight: 700},
								StyleFunc: nsui.Colored(colors.Text),
							},
						},
					},
				},
			},
		},
	}
}

func (a *App) newDeviceDialog(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return core.Show{
		When: func() bool { return a.newDeviceOpen.Get() },
		Then: func(cx *core.Scope) core.Node {
			return core.Overlay{
				TestID: "devices.newDialog",
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					return core.Style{
						Position:       core.Absolute,
						Top:            core.Pt(0),
						Right:          core.Pt(0),
						Bottom:         core.Pt(0),
						Left:           core.Pt(0),
						Width:          core.Pct(100),
						Height:         core.Pct(100),
						ZIndex:         45,
						AlignItems:     core.AlignCenter,
						JustifyContent: core.JustifyCenter,
						Padding: core.Edges{
							Top:    safe.Top + 20,
							Bottom: safe.Bottom + 20,
							Left:   safe.Left + 24,
							Right:  safe.Right + 24,
						},
						Background: themeRole(cx, "overlay"),
					}
				},
				OnCancel: func() { a.newDeviceOpen.Set(false) },
				Children: []core.Node{
					core.View{
						StyleFunc: func() core.Style {
							return core.Style{
								Width:        core.Pct(100),
								MaxWidth:     core.Pt(380),
								CornerRadius: 34,
								BorderCurve:  core.BorderCurveContinuous,
								Background:   colors.Surface(),
								Padding:      core.All(22),
								Gap:          14,
								Shadows: []core.BoxShadow{
									{
										Offset: core.Point{X: 0, Y: 14},
										Blur:   36,
										Color:  core.RGBA(0x000000, 0.24),
									},
								},
							}
						},
						Children: []core.Node{
							core.Dynamic(func(cx *core.Scope) core.Node {
								_ = a.newDeviceOpen.Get()
								c := a.deviceEditConn
								hostPort := hostLabelFromURL(c.BaseURL)
								labName := strings.TrimSpace(c.LibraryName)
								if labName == "" {
									labName = "默认素材库"
								}
								selIdx := a.deviceEditStyle.Get()

								rows := make([]core.Node, 0, 2)
								for r := 0; r < len(deviceCardStyles); r += 3 {
									rowItems := make([]core.Node, 0, 3)
									for col := 0; col < 3 && r+col < len(deviceCardStyles); col++ {
										idx := r + col
										item := deviceCardStyles[idx]
										rowItems = append(rowItems, core.View{
											TestID: "devices.new.styleOption." + itoa(idx),
											Role:   core.RoleButton,
											Label:  item.label,
											StyleFunc: func() core.Style {
												selected := a.deviceEditStyle.Get() == idx
												st := core.Style{
													FlexGrow:     1,
													FlexShrink:   1,
													MinWidth:     core.Pt(0),
													Padding:      core.Symmetric(8, 4),
													CornerRadius: 18,
													BorderCurve:  core.BorderCurveContinuous,
													Background:   colors.Inset(),
													AlignItems:   core.AlignCenter,
													Gap:          4,
													Pressed:      &core.Style{Opacity: core.Alpha(0.78)},
												}
												if selected {
													st.BorderWidth = 2
													st.BorderColor = colors.Accent()
												}
												return st
											},
											OnPress: func(core.PressEvent) {
												a.deviceEditStyle.Set(idx)
											},
											Children: []core.Node{
												core.Image{
													Source:     core.ImageSource{Data: item.data},
													ResizeMode: core.Contain,
													Style:      core.Style{Width: core.Pt(44), Height: core.Pt(44)},
												},
												core.Text{
													Content:   item.label,
													MaxLines:  1,
													Style:     core.Style{FontSize: 11, LineHeight: 14},
													StyleFunc: nsui.Colored(colors.Text),
												},
											},
										})
									}
									rows = append(rows, core.View{
										Style:    core.Style{Direction: core.Row, Gap: 8, Width: core.Pct(100)},
										Children: rowItems,
									})
								}

								return core.View{
									Style: core.Style{Gap: 12, Width: core.Pct(100)},
									Children: []core.Node{
										// Celebratory hero header with selected mascot
										core.View{
											Style: core.Style{AlignItems: core.AlignCenter, Gap: 6},
											Children: []core.Node{
												core.Image{
													SourceFunc: func() core.ImageSource {
														cur := a.deviceEditStyle.Get()
														if cur < 0 || cur >= len(deviceCardStyles) {
															cur = selIdx
														}
														return core.ImageSource{Data: deviceCardStyles[cur].data}
													},
													ResizeMode: core.Contain,
													Style:      core.Style{Width: core.Pt(78), Height: core.Pt(78)},
												},
												core.View{
													StyleFunc: func() core.Style {
														return core.Style{
															Padding:      core.Symmetric(4, 12),
															CornerRadius: 999,
															Background:   colors.Accent().Faded(0.16),
														}
													},
													Children: []core.Node{
														core.Text{
															Content: "✨ 新电脑已就绪",
															StyleFunc: func() core.Style {
																return core.Style{FontSize: 12, LineHeight: 16, FontWeight: 700, Color: colors.Accent()}
															},
														},
													},
												},
											},
										},
										// Computer IP + Active Lab info card
										core.View{
											StyleFunc: func() core.Style {
												return core.Style{
													Gap:          6,
													Padding:      core.Symmetric(10, 14),
													CornerRadius: 16,
													BorderCurve:  core.BorderCurveContinuous,
													Background:   colors.Inset(),
												}
											},
											Children: []core.Node{
												core.View{
													Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
													Children: []core.Node{
														core.Icon{SVG: lucide.Monitor(), Style: core.Style{FontSize: 15, Color: colors.Muted()}},
														core.Text{
															Content: func() string {
																if len(c.CandidateIPs) > 1 {
																	return "当前地址：" + hostPort + "（已绑定 " + itoa(len(c.CandidateIPs)) + " 个 IP）"
																}
																return "电脑地址：" + hostPort
															}(),
															MaxLines:  2,
															Style:     nsui.TypeCaption,
															StyleFunc: nsui.Colored(colors.Text),
														},
													},
												},
												core.View{
													Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
													Children: []core.Node{
														core.Icon{SVG: lucide.FolderOpen(), Style: core.Style{FontSize: 15, Color: colors.Accent()}},
														core.Text{
															Content:   "当前 Lab：" + labName,
															MaxLines:  1,
															Style:     nsui.TypeCaption.Merge(core.Style{FontWeight: 600}),
															StyleFunc: nsui.Colored(colors.Text),
														},
													},
												},
											},
										},
										// Computer name input
										core.View{
											Style: core.Style{Gap: 4},
											Children: []core.Node{
												core.Text{Content: "电脑名称", Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
												nsui.Input{
													Value:       func() string { return a.deviceEditName.Get() },
													OnChange:    a.deviceEditName.Set,
													Placeholder: "输入电脑名称",
													TestID:      "devices.new.name",
												},
											},
										},
										// Folder / Mascot icon selector
										core.View{
											Style: core.Style{Gap: 6},
											Children: append([]core.Node{
												core.Text{Content: "选择卡片图标", Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
											}, rows...),
										},
										// Confirm button
										mobileActionButton(colors, mobileBtnSpec{
											TestID:  "devices.new.confirm",
											Text:    "收入我的电脑",
											Variant: nsui.ButtonPrimary,
											Flex:    1,
											OnPress: func() {
												newName := strings.TrimSpace(a.deviceEditName.Get())
												if newName == "" {
													newName = "新建的电脑"
												}
												updated := c
												updated.Name = newName
												if err := a.local.Remember(updated); err != nil {
													a.showToast(err.Error())
												}
												a.cardStyleIdx[deviceStyleKey(c)] = a.deviceEditStyle.Get()
												a.cardStyleIdx[deviceStyleKey(c)] = a.deviceEditStyle.Get()
										a.cardStyleIdx[c.BaseURL] = a.deviceEditStyle.Get()
												a.cardStyleRev.Update(func(v int) int { return v + 1 })
												a.newDeviceOpen.Set(false)
												a.bump()
												a.triggerDropAnimation(c.BaseURL)
											},
										}),
									},
								}
							}),
						},
					},
				},
			}
		},
	}
}

func (a *App) deviceEditSheet(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	return a.bottomDrawer(cx, bottomDrawerSpec{
		Open:    func() bool { return a.deviceEditOpen.Get() },
		OnClose: func() { a.deviceEditOpen.Set(false) },
		Title:   "编辑电脑",
		TestID:  "devices.editSheet",
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				// Only rebuild sheet contents when the sheet opens/targets a different computer,
				// NOT on every style tap (so there is zero flash or input reset).
				_ = a.deviceEditOpen.Get()
				c := a.deviceEditConn
				hostPort := hostLabelFromURL(c.BaseURL)
				isNew := a.deviceEditIsNew.Get()

				heading := "电脑信息与样式"
				if isNew {
					heading = "已收藏到电脑列表，请为它命名"
				}

				rows := make([]core.Node, 0, 2)
				for r := 0; r < len(deviceCardStyles); r += 3 {
					rowItems := make([]core.Node, 0, 3)
					for col := 0; col < 3 && r+col < len(deviceCardStyles); col++ {
						idx := r + col
						item := deviceCardStyles[idx]
						rowItems = append(rowItems, core.View{
							TestID: "devices.styleOption." + itoa(idx),
							Role:   core.RoleButton,
							Label:  item.label,
							StyleFunc: func() core.Style {
								selected := a.deviceEditStyle.Get() == idx
								st := core.Style{
									FlexGrow:     1,
									FlexShrink:   1,
									MinWidth:     core.Pt(0),
									Padding:      core.Symmetric(10, 6),
									CornerRadius: nsui.RadiusCard,
									BorderCurve:  core.BorderCurveContinuous,
									Background:   colors.Inset(),
									AlignItems:   core.AlignCenter,
									Gap:          6,
									Pressed:      &core.Style{Opacity: core.Alpha(0.78)},
								}
								if selected {
									st.BorderWidth = 2
									st.BorderColor = colors.Accent()
								}
								return st
							},
							OnPress: func(core.PressEvent) {
								a.deviceEditStyle.Set(idx)
								a.cardStyleIdx[deviceStyleKey(c)] = idx
								a.cardStyleIdx[c.BaseURL] = idx
								a.cardStyleRev.Update(func(v int) int { return v + 1 })
							},
							Children: []core.Node{
								core.Image{
									Source:     core.ImageSource{Data: item.data},
									ResizeMode: core.Contain,
									Style:      core.Style{Width: core.Pt(56), Height: core.Pt(56)},
								},
								core.Text{
									Content:   item.label,
									MaxLines:  1,
									Style:     nsui.TypeCaption,
									StyleFunc: nsui.Colored(colors.Text),
								},
							},
						})
					}
					rows = append(rows, core.View{
						Style:    core.Style{Direction: core.Row, Gap: 10, Width: core.Pct(100)},
						Children: rowItems,
					})
				}

				return core.View{
					Style: core.Style{Gap: 12},
					Children: []core.Node{
						core.Text{
							Content:   heading,
							Style:     nsui.TypeLabel,
							StyleFunc: nsui.Colored(colors.Muted),
						},
						// Connection IP address(es) (only shown in this sheet)
						core.View{
							StyleFunc: func() core.Style {
								return core.Style{
									Direction:    core.Row,
									AlignItems:   core.AlignCenter,
									Gap:          8,
									Padding:      core.Symmetric(10, 12),
									CornerRadius: nsui.RadiusControl,
									Background:   colors.Inset(),
								}
							},
							Children: []core.Node{
								core.Icon{SVG: lucide.Monitor(), Style: core.Style{FontSize: 16, Color: colors.Muted()}},
								core.Text{
									Content: func() string {
										if len(c.CandidateIPs) > 1 {
											return "已绑定 " + itoa(len(c.CandidateIPs)) + " 个 IP：" + strings.Join(c.CandidateIPs, " · ")
										}
										return "IP 地址：" + hostPort
									}(),
									MaxLines:  2,
									Style:     nsui.TypeCaption,
									StyleFunc: nsui.Colored(colors.Text),
								},
							},
						},
						// Computer Name input
						core.View{
							Style: core.Style{Gap: 6},
							Children: []core.Node{
								core.Text{Content: "电脑名称", Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
								nsui.Input{
									Value:       func() string { return a.deviceEditName.Get() },
									OnChange:    a.deviceEditName.Set,
									Placeholder: "输入电脑名称，例如 工作室 Mac",
									TestID:      "devices.edit.name",
								},
							},
						},
						// Card Mascot Style selector
						core.View{
							Style: core.Style{Gap: 6},
							Children: append([]core.Node{
								core.Text{Content: "卡片形象", Style: nsui.TypeCaption, StyleFunc: nsui.Colored(colors.Muted)},
							}, rows...),
						},
						// Unified 1.5x height action buttons with theme corner radius
						core.View{
							Style: core.Style{Direction: core.Row, Gap: 10, Margin: core.Edges{Top: 4}},
							Children: []core.Node{
								mobileActionButton(colors, mobileBtnSpec{
									TestID:  "devices.edit.remove",
									Text:    "移除电脑",
									Variant: nsui.ButtonSecondary,
									OnPress: func() {
										if err := a.session.ForgetConnection(c.BaseURL); err != nil {
											a.formErr.Set(err.Error())
										}
										a.deviceEditOpen.Set(false)
										if next, err := a.local.History(); err == nil && len(next) == 0 {
											a.screen.Set(screenWelcome)
										}
										a.bump()
									},
								}),
								mobileActionButton(colors, mobileBtnSpec{
									TestID:  "devices.edit.save",
									Text:    "保存",
									Variant: nsui.ButtonPrimary,
									Flex:    1,
									OnPress: func() {
										newName := strings.TrimSpace(a.deviceEditName.Get())
										if newName == "" {
											newName = "新建的电脑"
										}
										updated := c
										updated.Name = newName
										if err := a.local.Remember(updated); err != nil {
											a.showToast(err.Error())
										}
										a.cardStyleIdx[deviceStyleKey(c)] = a.deviceEditStyle.Get()
										a.cardStyleIdx[c.BaseURL] = a.deviceEditStyle.Get()
										a.cardStyleRev.Update(func(v int) int { return v + 1 })
										a.deviceEditOpen.Set(false)
										a.bump()
									},
								}),
							},
						},
					},
				}
			}),
		},
	})
}
