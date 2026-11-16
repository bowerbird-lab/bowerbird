package ui

import (
	_ "embed"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	nsui "github.com/nsgo-dev/nsgo/ui"
)

//go:embed assets/welcome_hero.png
var welcomeHeroPNG []byte

var welcomeCardTransition = core.Transition{
	Duration:   340 * time.Millisecond,
	Easing:     core.EaseOut,
	Properties: core.TransitionTransform | core.TransitionOpacity,
}

func (a *App) setWelcomeSheetMode(mode string) {
	a.formErr.Set("")
	a.connectStatus.Set("")
	a.welcomeManualPair.Set(false)
	a.code.Set("")
	a.welcomeSheetMode.Set(mode)
}

// formatAutoIP automatically inserts '.' after every 3 digits while typing an IPv4 address,
// allows typing '.' after 1-2 digits to advance to the next octet, and collapses accidental '..'.
func formatAutoIP(prev, next string) string {
	trimmed := strings.TrimSpace(next)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	var clean strings.Builder
	lastDot := false
	for i := 0; i < len(next); i++ {
		ch := next[i]
		if ch >= '0' && ch <= '9' {
			clean.WriteByte(ch)
			lastDot = false
		} else if ch == '.' && clean.Len() > 0 && !lastDot {
			clean.WriteByte('.')
			lastDot = true
		}
	}
	cleaned := clean.String()
	if len(cleaned) < len(prev) {
		return cleaned
	}
	parts := strings.Split(cleaned, ".")
	if len(parts) > 4 {
		parts = parts[:4]
	}
	var out []string
	for i, part := range parts {
		for len(part) > 3 && len(out) < 3 {
			out = append(out, part[:3])
			part = part[3:]
		}
		if len(out) == 3 && len(part) > 3 {
			part = part[:3]
		}
		out = append(out, part)
		if i == len(parts)-1 && len(part) == 3 && len(out) < 4 {
			out = append(out, "")
		}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return strings.Join(out, ".")
}

func buildAddressFromIPPort(ipRaw, portRaw string) string {
	ip := strings.TrimSpace(ipRaw)
	if ip == "" {
		return ""
	}
	if strings.HasPrefix(ip, "http://") || strings.HasPrefix(ip, "https://") {
		return ip
	}
	parts := strings.Split(ip, ".")
	var cleanParts []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			cleanParts = append(cleanParts, strconv.Itoa(n))
		} else {
			cleanParts = append(cleanParts, p)
		}
	}
	host := strings.Join(cleanParts, ".")
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		return host
	}
	port := strings.TrimSpace(portRaw)
	if port == "" {
		port = "17878"
	}
	return host + ":" + port
}

func (a *App) submitWelcomeConnect(cx *core.Scope) {
	ipStr := strings.TrimSpace(a.welcomeIP.Get())
	if ipStr == "" {
		o0 := strings.TrimSpace(a.welcomeOctets[0].Get())
		o1 := strings.TrimSpace(a.welcomeOctets[1].Get())
		o2 := strings.TrimSpace(a.welcomeOctets[2].Get())
		o3 := strings.TrimSpace(a.welcomeOctets[3].Get())
		if o0 != "" || o1 != "" || o2 != "" || o3 != "" {
			ipStr = o0 + "." + o1 + "." + o2 + "." + o3
		}
	}
	addr := buildAddressFromIPPort(ipStr, a.welcomePort.Get())
	if addr == "" {
		a.formErr.Set("请输入电脑 IP 地址")
		return
	}
	a.address.Set(addr)
	a.submitConnect(cx)
}

func (a *App) welcome(cx *core.Scope) core.Node {
	colors := nsui.ColorsOf(cx)
	_ = a.rev.Get()

	isDark := func() bool { return cx.Env().Dark() }
	wordmarkColor := func() core.Color { return colors.Text() }
	subtitleColor := func() core.Color { return colors.Muted() }
	scanBg := func() core.Color {
		if isDark() {
			return colors.Inset()
		}
		return core.RGB(0xFFFFFF)
	}
	scanBgPressed := func() core.Color {
		if isDark() {
			return colors.Surface()
		}
		return core.RGB(0xF3F2F0)
	}
	scanText := func() core.Color { return colors.Text() }
	scanBorder := func() core.Color { return colors.Border() }
	manualBg := func() core.Color { return colors.Text() }
	manualBgPressed := func() core.Color { return colors.Muted() }
	manualText := func() core.Color { return colors.Background() }

	return core.View{
		TestID: "screen.welcome",
		StyleFunc: func() core.Style {
			topGradStart := core.RGB(0xFFFFFF)
			topGradMid := core.RGB(0xEDF7FF)
			topGradEnd := core.RGB(0xD8EEFF)
			if isDark() {
				topGradStart = core.RGB(0x0E141E)
				topGradMid = core.RGB(0x121D2D)
				topGradEnd = core.RGB(0x16273E)
			}
			return core.Style{
				Width:      core.Pct(100),
				Height:     core.Pct(100),
				Background: topGradStart,
				Gradients: []core.Gradient{
					core.LinearGradient(180, topGradStart, topGradMid, topGradEnd),
				},
			}
		},
		Children: []core.Node{
			// Upper illustration & brand area (fades & glides away as the bottom card pushes up)
			core.View{
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					return core.Style{
						Width:      core.Pct(100),
						Height:     core.Pct(100),
						Padding:    core.Edges{Top: safe.Top + 10, Left: safe.Left, Right: safe.Right, Bottom: 260 + safe.Bottom},
					}
				},
				Children: []core.Node{
					// Top bar outside the card:
					// - In Welcome mode: optional back button (when saved devices exist) | empty | settings
					// - In Sheet mode ("manual" / "discover"): back button to Welcome | centered current action title | refresh (discover) or settings
					core.View{
						StyleFunc: func() core.Style {
							return core.Style{
								Width:          core.Pct(100),
								Height:         core.Pt(50),
								Direction:      core.Row,
								JustifyContent: core.JustifySpaceBetween,
								AlignItems:     core.AlignCenter,
								Padding:        core.Symmetric(0, 16),
								ZIndex:         3,
							}
						},
						Children: []core.Node{
							core.View{
								Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
								Children: []core.Node{
									core.Show{
										When: func() bool {
											if a.welcomeSheetMode.Get() != "" {
												return true
											}
											_ = a.rev.Get()
											history, _ := a.local.History()
											return len(history) > 0
										},
										Then: func(cx *core.Scope) core.Node {
											return nsui.IconButton{
												Icon:     lucide.ChevronLeft,
												Label:    "返回",
												Size:     36,
												IconSize: 20,
												OnPress: func() {
													if a.welcomeSheetMode.Get() != "" {
														a.setWelcomeSheetMode("")
														return
													}
													a.formErr.Set("")
													a.connectStatus.Set("")
													a.screen.Set(screenDevices)
												},
												TestID: "welcome.back",
											}
										},
									},
								},
							},
							core.View{
								Style: core.Style{FlexGrow: 1, FlexShrink: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
								Children: []core.Node{
									core.Show{
										When: func() bool { return a.welcomeSheetMode.Get() != "" },
										Then: func(cx *core.Scope) core.Node {
											return core.View{
												StyleFunc: func() core.Style {
													return core.Style{
														Padding:      core.Symmetric(6, 18),
														CornerRadius: 999,
														Background:   colors.Surface(),
														BorderWidth:  1,
														BorderColor:  colors.Border(),
														AlignItems:   core.AlignCenter,
													}
												},
												Children: []core.Node{
													core.Text{
														ContentFunc: func() string {
															switch a.welcomeSheetMode.Get() {
															case "discover":
																return "发现桌面端"
															case "scan":
																return "扫码连接"
															default:
																return "手动连接"
															}
														},
														StyleFunc: func() core.Style {
															return core.Style{
																FontSize:   15,
																LineHeight: 20,
																FontWeight: 700,
																TextAlign:  core.TextAlignCenter,
																Color:      colors.Text(),
															}
														},
													},
												},
											}
										},
									},
								},
							},
							core.View{
								Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
								Children: []core.Node{
									core.Show{
										When: func() bool { return a.welcomeSheetMode.Get() == "discover" },
										Then: func(cx *core.Scope) core.Node {
											return nsui.IconButton{
												Icon:     lucide.RefreshCw,
												Label:    "刷新",
												Size:     36,
												IconSize: 18,
												Disabled: func() bool { return a.discovering.Get() },
												OnPress:  func() { a.discover(cx) },
												TestID:   "welcome.discover.refresh",
											}
										},
									},
									core.Show{
										When: func() bool { return a.welcomeSheetMode.Get() != "discover" },
										Then: func(cx *core.Scope) core.Node {
											return nsui.IconButton{
												Icon:     lucide.Settings,
												Label:    "设置",
												Size:     36,
												IconSize: 18,
												OnPress:  func() { a.openSettings(screenWelcome) },
												TestID:   "welcome.settings",
											}
										},
									},
								},
							},
						},
					},
					core.View{
						StyleFunc: func() core.Style {
							pushed := a.welcomeSheetMode.Get() != ""
							op := float32(1)
							ty := core.Points(0)
							if pushed {
								op = 0
								ty = -44
							}
							return core.Style{
								FlexGrow:   1,
								FlexShrink: 1,
								Opacity:    core.Alpha(op),
								Transform:  core.Transform{TranslateY: ty},
								Transition: welcomeCardTransition,
							}
						},
						PointerEvents: core.PointerEventsNone,
						Children: []core.Node{
							core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(8)}},
							// Bold rounded wordmark
							core.Text{
								Content: "bowerbird",
								StyleFunc: func() core.Style {
									return core.Style{
										FontFamily:    "ui-rounded, sans-serif-rounded, Arial Rounded MT Bold, sans-serif",
										FontSize:      52,
										LineHeight:    58,
										FontWeight:    800,
										LetterSpacing: -1.4,
										TextAlign:     core.TextAlignCenter,
										Color:         wordmarkColor(),
										Margin:        core.Edges{Bottom: 104},
									}
								},
							},
							core.Image{
								TestID:     "welcome.illustration",
								Label:      "Bowerbird 欢迎插画",
								Source:     core.ImageSource{Data: welcomeHeroPNG},
								ResizeMode: core.Contain,
								Style: core.Style{
									FlexShrink: 1,
									Width:      core.Pct(100),
									Height:     core.Pt(330),
									MinHeight:  core.Pt(160),
									Margin:     core.Edges{Left: 8, Right: 8},
									ObjectPosition: core.ObjectPosition{
										X: core.PositionAxis{Offset: core.Pct(50)},
										Y: core.PositionAxis{Offset: core.Pct(100)},
									},
								},
							},
						},
					},
				},
			},

			// Bottom top-rounded card: native Transition animates TranslateY smoothly up to top (safe.Top + 64)
			core.View{
				StyleFunc: func() core.Style {
					safe := cx.Env().SafeArea()
					winH := cx.Env().Window().Height
					if winH <= 0 {
						winH = 800
					}
					topAnchor := safe.Top + 62
					restingTop := winH - (284 + safe.Bottom)
					if restingTop < topAnchor+120 {
						restingTop = topAnchor + 120
					}
					translateY := restingTop - topAnchor
					if a.welcomeSheetMode.Get() != "" {
						translateY = 0
					}

					return core.Style{
						Position:    core.Absolute,
						Top:         core.Pt(topAnchor),
						Bottom:      core.Pt(0),
						Left:        core.Pt(0),
						Right:       core.Pt(0),
						Transform:   core.Transform{TranslateY: translateY},
						Transition:  welcomeCardTransition,
						Background:  colors.Surface(),
						CornerRadii: core.Corners{TopLeft: 54, TopRight: 54},
						BorderCurve: core.BorderCurveContinuous,
						ZIndex:      2,
						Padding: core.Edges{
							Top:    26,
							Bottom: safe.Bottom + 24,
							Left:   safe.Left + 24,
							Right:  safe.Right + 24,
						},
						Gap:        12,
						AlignItems: core.AlignCenter,
					}
				},
				Children: []core.Node{
					// Mode "": Default Welcome buttons (扫码连接 + 手动输入 + 发现电脑)
					core.Show{
						When: func() bool { return a.welcomeSheetMode.Get() == "" },
						Then: func(cx *core.Scope) core.Node {
							return core.View{
								Style: core.Style{Width: core.Pct(100), Gap: 12, AlignItems: core.AlignCenter},
								Children: []core.Node{
									core.Text{
										Content: "连接电脑素材库，开始使用",
										StyleFunc: func() core.Style {
											return core.Style{
												FontSize:   15,
												LineHeight: 21,
												FontWeight: 500,
												Color:      subtitleColor(),
												TextAlign:  core.TextAlignCenter,
												Margin:     core.Edges{Bottom: 4},
											}
										},
									},
									welcomePillButton(welcomePillSpec{
										TestID:    "welcome.scan",
										Text:      "扫码连接",
										Icon:      lucide.ScanQrCode(),
										Bg:        scanBg,
										BgPressed: scanBgPressed,
										Fg:        scanText,
										Border:    scanBorder,
										Shadow:    func() bool { return !isDark() },
										Disabled:  func() bool { return a.busy.Get() },
										OnPress: func() {
											if a.env.HasHost {
												a.setWelcomeSheetMode("scan")
												return
											}
											a.scanQR(cx)
										},
									}),
									welcomePillButton(welcomePillSpec{
										TestID:    "welcome.manual",
										Text:      "手动输入",
										Icon:      lucide.Keyboard(),
										Bg:        manualBg,
										BgPressed: manualBgPressed,
										Fg:        manualText,
										Disabled:  func() bool { return a.busy.Get() },
										OnPress:   func() { a.setWelcomeSheetMode("manual") },
									}),
									welcomePillButton(welcomePillSpec{
										TestID:    "welcome.discover",
										Text:      "发现电脑",
										Icon:      lucide.Search(),
										Bg:        scanBg,
										BgPressed: scanBgPressed,
										Fg:        scanText,
										Border:    scanBorder,
										OnPress: func() {
											a.setWelcomeSheetMode("discover")
											if !a.discovering.Get() && len(a.services) == 0 {
												a.discover(cx)
											}
										},
									}),
								},
							}
						},
					},

					// Mode "scan": Pushed-up card with square inline camera QR scanner
					core.Show{
						When: func() bool { return a.welcomeSheetMode.Get() == "scan" },
						Then: func(cx *core.Scope) core.Node {
							winW := cx.Env().Window().Width
							if winW <= 0 {
								winW = 390
							}
							safe := cx.Env().SafeArea()
							scanSize := winW - safe.Left - safe.Right - 48
							if scanSize > 300 {
								scanSize = 300
							}
							if scanSize < 200 {
								scanSize = 200
							}
							return core.View{
								TestID: "welcome.scanPanel",
								Style: core.Style{
									Width:      core.Pct(100),
									AlignItems: core.AlignCenter,
									Padding:    core.Edges{Top: 24},
									Gap:        16,
								},
								Children: []core.Node{
									core.View{
										Style: core.Style{
											Width:        core.Pt(scanSize),
											Height:       core.Pt(scanSize),
											CornerRadius: 32,
											BorderCurve:  core.BorderCurveContinuous,
											Overflow:     core.Hidden,
											Background:   core.RGB(0x111216),
										},
										Children: []core.Node{
											core.Native{
												Component: "qr_scanner",
												Style:     core.Style{Width: core.Pt(scanSize), Height: core.Pt(scanSize)},
												PropsFunc: func() map[string]any {
													r, g, b, _ := colors.Accent().Components()
													accentInt := (0xFF << 24) | (int(r*255+0.5) << 16) | (int(g*255+0.5) << 8) | int(b*255+0.5)
													return map[string]any{
														"active": a.screen.Get() == screenWelcome && a.welcomeSheetMode.Get() == "scan",
														"accent": accentInt,
													}
												},
												OnEvent: func(name string, data any) {
													if name == "scanned" {
														if m, ok := data.(map[string]any); ok {
															if payload, _ := m["payload"].(string); strings.TrimSpace(payload) != "" {
																a.welcomeSheetMode.Set("")
																a.handlePairURL(cx, payload)
															}
														}
													}
												},
											},
										},
									},
									core.Text{
										Content:   "将桌面端配对二维码放入正方形框内",
										Style:     nsui.TypeCaption.Merge(core.Style{TextAlign: core.TextAlignCenter}),
										StyleFunc: nsui.Colored(colors.Muted),
									},
									mobileActionButton(colors, mobileBtnSpec{
										TestID:  "welcome.scan.toManual",
										Text:    "改用手动输入",
										Icon:    lucide.Keyboard,
										Variant: nsui.ButtonSecondary,
										OnPress: func() { a.setWelcomeSheetMode("manual") },
									}),
								},
							}
						},
					},

					// Mode "manual": Horizontal slide between IP+Port form (left) and 6-digit Pairing Code form (right)
					core.Show{
						When: func() bool { return a.welcomeSheetMode.Get() == "manual" },
						Then: func(cx *core.Scope) core.Node {
							winW := cx.Env().Window().Width
							if winW <= 0 {
								winW = 390
							}
							slideW := winW - 48
							if slideW < 240 {
								slideW = 240
							}
							return core.Scroll{
								Style: core.Style{FlexGrow: 1, FlexShrink: 1, Width: core.Pct(100), MinHeight: core.Pt(0)},
								Children: []core.Node{
									core.View{
										TestID: "welcome.manualPanel",
										Style: core.Style{
											Width:    core.Pct(100),
											Overflow: core.Hidden,
										},
										Children: []core.Node{
											// Step 1: IP + Port form (slides left when pairing code is required)
											core.View{
												StyleFunc: func() core.Style {
													pairing := a.welcomeManualPair.Get()
													tx := core.Points(0)
													op := float32(1)
													if pairing {
														tx = -slideW
														op = 0
													}
													return core.Style{
														Width:      core.Pct(100),
														Gap:        16,
														Transform:  core.Transform{TranslateX: tx},
														Opacity:    core.Alpha(op),
														Transition: welcomeCardTransition,
													}
												},
												Children: []core.Node{
													// 4-octet pure numeric IP inputs (zero '.' key needed, auto-advance on 3 digits)
													core.View{
														Style: core.Style{Gap: 8},
														Children: []core.Node{
															core.Text{
																Content:   "IP 地址",
																Style:     nsui.TypeCaption,
																StyleFunc: nsui.Colored(colors.Muted),
															},
															func() core.Node {
																octetBox := func(idx int, ph string) core.Node {
																	return core.Input{
																		TestID:        "welcome.ip." + itoa(idx),
																		Ref:           a.welcomeOctetRefs[idx],
																		Value:         func() string { return a.welcomeOctets[idx].Get() },
																		Placeholder:   ph,
																		KeyboardType:  core.KeyboardNumber,
																		MaxLength:     4,
																		NoAutoCorrect: true,
																		StyleFunc: func() core.Style {
																			return core.Style{
																				FlexGrow:         1,
																				FlexShrink:       1,
																				MinWidth:         core.Pt(0),
																				Height:           core.Pt(62),
																				Padding:          core.Symmetric(0, 4),
																				CornerRadius:     18,
																				BorderCurve:      core.BorderCurveContinuous,
																				Background:       colors.Inset(),
																				BorderWidth:      1,
																				BorderColor:      colors.Border(),
																				Color:            colors.Text(),
																				PlaceholderColor: colors.Faint(),
																				FontSize:         22,
																				LineHeight:       28,
																				FontWeight:       700,
																				TextAlign:        core.TextAlignCenter,
																			}
																		},
																		OnChange: func(next string) {
																			trimmed := strings.TrimSpace(next)
																			if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
																				a.welcomeIP.Set(trimmed)
																				return
																			}
																			a.welcomeIP.Set("")
																			if strings.Contains(next, ".") {
																				parts := strings.Split(next, ".")
																				if len(parts) >= 2 && idx+1 < 4 && parts[0] != "" {
																					for k := 0; k < len(parts) && idx+k < 4; k++ {
																						var d strings.Builder
																						for j := 0; j < len(parts[k]) && d.Len() < 3; j++ {
																							if parts[k][j] >= '0' && parts[k][j] <= '9' {
																								d.WriteByte(parts[k][j])
																							}
																						}
																						a.welcomeOctets[idx+k].Set(d.String())
																					}
																					a.welcomeOctetRefs[idx+1].Focus()
																					return
																				}
																			}
																			var digits strings.Builder
																			for j := 0; j < len(next) && digits.Len() < 3; j++ {
																				if next[j] >= '0' && next[j] <= '9' {
																					digits.WriteByte(next[j])
																				}
																			}
																			val := digits.String()
																			a.welcomeOctets[idx].Set(val)
																			if len(val) == 3 && idx < 3 {
																				a.welcomeOctetRefs[idx+1].Focus()
																			}
																		},
																		OnKeyPress: func(key string) {
																			if key == "Backspace" && a.welcomeOctets[idx].Get() == "" && idx > 0 {
																				a.welcomeOctetRefs[idx-1].Focus()
																			}
																		},
																		OnSubmit: func(string) {
																			if idx < 3 {
																				a.welcomeOctetRefs[idx+1].Focus()
																			} else if a.welcomePortRef != nil {
																				a.welcomePortRef.Focus()
																			}
																		},
																	}
																}
																dot := func() core.Node {
																	return core.Text{
																		Content:   ".",
																		Style:     core.Style{FontSize: 24, FontWeight: 800, Padding: core.Symmetric(0, 2)},
																		StyleFunc: nsui.Colored(colors.Muted),
																	}
																}
																return core.View{
																	TestID: "welcome.ip",
																	Style: core.Style{
																		Direction:  core.Row,
																		AlignItems: core.AlignCenter,
																		Gap:        4,
																		Width:      core.Pct(100),
																	},
																	Children: []core.Node{
																		octetBox(0, "192"),
																		dot(),
																		octetBox(1, "168"),
																		dot(),
																		octetBox(2, "1"),
																		dot(),
																		octetBox(3, "1"),
																	},
																}
															}(),
														},
													},
													// Port input
													core.View{
														Style: core.Style{Gap: 8},
														Children: []core.Node{
															core.Text{
																Content:   "端口",
																Style:     nsui.TypeCaption,
																StyleFunc: nsui.Colored(colors.Muted),
															},
															core.Input{
																TestID:        "welcome.port",
																Ref:           a.welcomePortRef,
																Value:         func() string { return a.welcomePort.Get() },
																Placeholder:   "17878",
																KeyboardType:  core.KeyboardNumber,
																MaxLength:     5,
																NoAutoCorrect: true,
																StyleFunc: func() core.Style {
																	return core.Style{
																		Width:            core.Pct(100),
																		Height:           core.Pt(56),
																		Padding:          core.Symmetric(0, 16),
																		CornerRadius:     20,
																		BorderCurve:      core.BorderCurveContinuous,
																		Background:       colors.Inset(),
																		BorderWidth:      1,
																		BorderColor:      colors.Border(),
																		Color:            colors.Text(),
																		PlaceholderColor: colors.Faint(),
																		FontSize:         20,
																		LineHeight:       26,
																		FontWeight:       600,
																		TextAlign:        core.TextAlignCenter,
																	}
																},
																OnChange: a.welcomePort.Set,
																OnSubmit: func(string) { a.submitWelcomeConnect(cx) },
															},
														},
													},
													// Connect button
													welcomePillButton(welcomePillSpec{
														TestID:    "welcome.connect",
														Text:      "连接",
														Icon:      lucide.ArrowRight(),
														Bg:        manualBg,
														BgPressed: manualBgPressed,
														Fg:        manualText,
														Disabled:  func() bool { return a.busy.Get() },
														OnPress:   func() { a.submitWelcomeConnect(cx) },
													}),
												},
											},

											// Step 2: 6-digit Pairing Code form (slides in from right when pairing is required)
											core.View{
												StyleFunc: func() core.Style {
													pairing := a.welcomeManualPair.Get()
													tx := slideW
													op := float32(0)
													if pairing {
														tx = 0
														op = 1
													}
													return core.Style{
														Position:   core.Absolute,
														Top:        core.Pt(0),
														Left:       core.Pt(0),
														Right:      core.Pt(0),
														Gap:        16,
														Transform:  core.Transform{TranslateX: tx},
														Opacity:    core.Alpha(op),
														Transition: welcomeCardTransition,
													}
												},
												Children: []core.Node{
													core.View{
														Style: core.Style{Gap: 4},
														Children: []core.Node{
															core.Text{
																Content:   "输入配对码",
																Style:     nsui.TypeTitle,
																StyleFunc: nsui.Colored(colors.Text),
															},
															core.Text{
																Content:   "在电脑的 Bowerbird 设置里查看 6 位配对码",
																Style:     nsui.TypeCaption,
																StyleFunc: nsui.Colored(colors.Muted),
															},
														},
													},
													core.Input{
														TestID:        "welcome.code",
														Value:         func() string { return a.code.Get() },
														Placeholder:   "000000",
														KeyboardType:  core.KeyboardNumber,
														MaxLength:     6,
														NoAutoCorrect: true,
														StyleFunc: func() core.Style {
															return core.Style{
																Width:            core.Pct(100),
																Height:           core.Pt(68),
																Padding:          core.Symmetric(0, 18),
																CornerRadius:     24,
																BorderCurve:      core.BorderCurveContinuous,
																Background:       colors.Inset(),
																BorderWidth:      1,
																BorderColor:      colors.Border(),
																Color:            colors.Text(),
																PlaceholderColor: colors.Faint(),
																FontSize:         28,
																LineHeight:       34,
																FontWeight:       700,
																LetterSpacing:    6,
																TextAlign:        core.TextAlignCenter,
															}
														},
														OnChange: a.code.Set,
														OnSubmit: func(string) { a.submitConnect(cx) },
													},
													welcomePillButton(welcomePillSpec{
														TestID:    "welcome.pair",
														Text:      "配对",
														Icon:      lucide.Check(),
														Bg:        manualBg,
														BgPressed: manualBgPressed,
														Fg:        manualText,
														Disabled:  func() bool { return a.busy.Get() },
														OnPress:   func() { a.submitConnect(cx) },
													}),
													mobileActionButton(colors, mobileBtnSpec{
														TestID:  "welcome.pair.back",
														Text:    "修改 IP 地址",
														Icon:    lucide.ChevronLeft,
														Variant: nsui.ButtonGhost,
														OnPress: func() {
															a.welcomeManualPair.Set(false)
															a.code.Set("")
															a.formErr.Set("")
														},
													}),
												},
											},
										},
									},
								},
							}
						},
					},

					// Mode "discover": Pushed-up tall sheet with Back button + LAN discovered computers
					core.Show{
						When: func() bool { return a.welcomeSheetMode.Get() == "discover" },
						Then: func(cx *core.Scope) core.Node {
							return core.Scroll{
								Style: core.Style{FlexGrow: 1, FlexShrink: 1, Width: core.Pct(100), MinHeight: core.Pt(0)},
								Children: []core.Node{
									core.View{
										TestID: "welcome.discoverPanel",
										Style:  core.Style{Width: core.Pct(100), Gap: 14},
										Children: []core.Node{
											core.Show{
												When: func() bool { return a.discovering.Get() },
												Then: func(cx *core.Scope) core.Node {
													return core.Text{
														Content:   "正在搜索局域网内的 Bowerbird 桌面端…",
														Style:     nsui.TypeCaption,
														StyleFunc: nsui.Colored(colors.Muted),
													}
												},
											},
											core.Dynamic(func(cx *core.Scope) core.Node {
												_ = a.rev.Get()
												if len(a.services) == 0 {
													return core.Text{
														Content:   "暂未发现桌面端，可点击右上角刷新或返回手动输入",
														Style:     nsui.TypeCaption,
														StyleFunc: nsui.Colored(colors.Muted),
													}
												}
												rows := make([]core.Node, 0, len(a.services))
												for _, svc := range a.services {
													rows = append(rows, a.serviceRow(cx, svc))
												}
												return core.View{
													Style:    core.Style{Width: core.Pct(100), Gap: 8},
													Children: rows,
												}
											}),
										},
									},
								},
							}
						},
					},

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
					core.Show{
						When: func() bool { a.rev.Get(); return len(a.inboxEntries) > 0 && a.welcomeSheetMode.Get() == "" },
						Then: func(cx *core.Scope) core.Node {
							return nsui.Button{
								Text:    "收件箱中有 " + itoa(len(a.inboxEntries)) + " 项待上传 · 查看",
								Variant: nsui.ButtonGhost,
								OnPress: func() {
									a.inboxBackScreen = screenWelcome
									a.screen.Set(screenInbox)
								},
							}
						},
					},
				},
			},

			a.pairDialog(cx),
		},
	}
}

type welcomePillSpec struct {
	TestID    string
	Text      string
	Icon      *core.SVG
	Bg        func() core.Color
	BgPressed func() core.Color
	Fg        func() core.Color
	Border    func() core.Color
	Shadow    func() bool
	Disabled  func() bool
	OnPress   func()
}

func welcomePillButton(spec welcomePillSpec) core.Node {
	return core.View{
		TestID: spec.TestID,
		Role:   core.RoleButton,
		Label:  spec.Text,
		StyleFunc: func() core.Style {
			st := core.Style{
				Width:          core.Pct(100),
				Height:         core.Pt(50),
				CornerRadius:   mobileButtonRadius,
				Direction:      core.Row,
				AlignItems:     core.AlignCenter,
				JustifyContent: core.JustifyCenter,
				Gap:            10,
				Background:     spec.Bg(),
				Pressed:        &core.Style{Background: spec.BgPressed()},
			}
			if spec.Border != nil {
				if b := spec.Border(); b != 0 {
					st.BorderWidth = 1
					st.BorderColor = b
				}
			}
			if spec.Shadow != nil && spec.Shadow() {
				st.Shadows = []core.BoxShadow{
					{
						Offset: core.Point{X: 0, Y: 4},
						Blur:   14,
						Color:  core.RGBA(0x000000, 0.06),
					},
				}
			}
			if spec.Disabled != nil && spec.Disabled() {
				st.Opacity = core.Alpha(0.55)
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
		Children: []core.Node{
			core.Icon{
				SVG: spec.Icon,
				StyleFunc: func() core.Style {
					return core.Style{
						FontSize: 20,
						Color:    spec.Fg(),
					}
				},
			},
			core.Text{
				Content: spec.Text,
				StyleFunc: func() core.Style {
					return core.Style{
						FontSize:   16,
						LineHeight: 20,
						FontWeight: 600,
						Color:      spec.Fg(),
					}
				},
			},
		},
	}
}
