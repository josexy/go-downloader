package main

import (
	"github.com/egoist/mygo/ui"

	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
)

// settingsModal shows the preferences over the window.
func (a *app) settingsModal(c *ui.Context) {
	if !a.settingsOpen {
		return
	}
	a.settingsWasOpen = true
	p := a.palette
	width, height := c.Size()
	panelWidth := min(640, max(420, width-80))
	ui.Modal(c, &a.settingsOpen, func() {
		ui.Column(c).Width(panelWidth).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, icoSettings).FontSize(17).TextColor(p.Accent)
				ui.Text(c, "设置").FontSize(18).Bold().Grow(1)
				iconButton(c, p, "关闭设置", icoX, false, func() { a.settingsOpen = false })
			})
			ui.Tabs(c, &a.settingsTab, "常规", "代理", "外观", "关于").Label("设置分类")
			ui.Box(c).Height(a.settingsContentHeight(height)).FillWidth().Children(func() {
				ui.Scroll(c).Fill().Gap(10).Children(func() {
					switch a.settingsTab {
					case 1:
						a.settingsProxy(c)
					case 2:
						a.settingsAppearance(c)
					case 3:
						a.settingsAbout(c)
					default:
						a.settingsGeneral(c)
					}
				})
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "完成").AutoFocus().Clicked() {
					a.settingsOpen = false
				}
			})
		})
	})
}

// settingsContentHeight fits the dialog to the tab's content, so a short page
// does not leave an empty panel under it.
func (a *app) settingsContentHeight(windowHeight float32) float32 {
	limit := max(150, windowHeight-240)
	switch a.settingsTab {
	case 1:
		if proxyModes[a.proxyModeIdx] == proxy.ModeManual {
			return min(470, limit)
		}
		return min(260, limit)
	case 2:
		return min(130, limit)
	case 3:
		return min(190, limit)
	default:
		return min(280, limit)
	}
}

// afterSettingsClosed saves what the dialog edited.
func (a *app) afterSettingsClosed() {
	if a.store == nil {
		return
	}
	a.s.Proxy.Password = a.proxyDraft.Password
	if a.passwordDirty && a.deps.secrets != nil {
		a.passwordDirty = false
		password := a.proxyDraft.Password
		a.deps.async(func() {
			if err := a.store.SaveProxyPassword(password); err != nil {
				a.dispatch(func() { a.toast(err.Error()) })
			}
		})
	}
	a.saveSettings()
}

// settingsSection is an icon and caption in the settings dialog.
func (a *app) settingsSection(c *ui.Context, icon *ui.SVG, title string) {
	p := a.palette
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(12).TextColor(p.Muted)
		ui.Text(c, title).FontSize(11).Bold().TextColor(p.Muted).LetterSpacing(0.5)
	})
}

// settingsRow is a labelled setting with its control at the end of the row.
func (a *app) settingsRow(c *ui.Context, icon *ui.SVG, label, tip string, control func()) {
	p := a.palette
	row := ui.Row(c).Height(40).Gap(10).AlignItems(ui.Center)
	if tip != "" {
		row.Tooltip(tip)
	}
	row.Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(p.Muted)
		ui.Text(c, label).FontSize(12).Grow(1)
		control()
	})
}

// settingsGeneral is the Downloads tab.
func (a *app) settingsGeneral(c *ui.Context) {
	p := a.palette
	a.settingsSection(c, icoDownload, "下载")

	folder := ui.Row(c).Height(48).Gap(10).AlignItems(ui.Center).
		Tooltip("新添加的下载会保存到这里；正在下载的项目不受影响。")
	folder.Children(func() {
		ui.Icon(c, icoFolder).FontSize(15).TextColor(p.Muted)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, "保存位置").FontSize(12)
			ui.Text(c, a.s.DownloadDir).FontSize(10).TextColor(p.Muted).
				SingleLine().Tooltip(a.s.DownloadDir)
		})
		iconButton(c, p, "选择文件夹", icoFolderPlus, false, func() { a.chooseFolder() })
		iconButton(c, p, "打开文件夹", icoFolderOpen, false, func() { a.revealDownloadDir() })
	})

	a.settingsRow(c, icoZap, "同时下载数", "同时进行的下载任务数量。", func() {
		if ui.Select(c, &a.maxLabel, maxConcurrencyLabels).Width(96).
			Label("同时下载数").Changed() {
			a.setMaxConcurrent(mustAtoi(a.maxLabel, a.s.MaxConcurrent))
		}
	})
	a.settingsRow(c, icoBell, "下载完成后通知", "应用在后台时用系统通知提醒。", func() {
		if ui.Switch(c, &a.s.NotifyOnComplete).Label("下载完成后通知").Changed() {
			a.saveSettings()
		}
	})
	a.settingsRow(c, icoPlay, "添加后自动开始",
		"关闭后，新下载会以暂停状态加入，点击继续或全部继续才会开始。", func() {
			if ui.Switch(c, &a.s.AutoStart).Label("添加后自动开始").Changed() {
				a.engine.SetAutoStart(a.s.AutoStart)
				a.saveSettings()
			}
		})

	if a.proxyErr != "" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoAlert).FontSize(13).TextColor(p.Danger)
			ui.Text(c, "代理配置有误："+a.proxyErr).FontSize(10).TextColor(p.Danger)
		})
	}
}

// settingsProxy is the Proxy tab.
func (a *app) settingsProxy(c *ui.Context) {
	p := a.palette
	a.settingsSection(c, icoShield, "网络代理")
	a.segmented(c, proxyModeLabels, nil, a.proxyModeIdx, true,
		func(i int) { a.setProxyMode(i) })

	switch proxyModes[a.proxyModeIdx] {
	case proxy.ModeSystem:
		a.systemProxyBox(c)
	case proxy.ModeManual:
		a.manualProxyForm(c)
	}

	a.settingsSection(c, icoZap, "连接测试")
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if a.proxyTest.running {
			ui.Spinner(c)
			ui.Text(c, "测试中…").FontSize(11).TextColor(p.Muted)
			return
		}
		if ui.Button(c, "测试连接").FontSize(11).
			Tooltip("对之后开始的连接生效，正在下载的项目不受影响。").Clicked() {
			a.runProxyTest()
		}
		if a.proxyTest.result != "" {
			color := p.Danger
			if a.proxyTest.ok {
				color = p.Success
			}
			ui.Text(c, a.proxyTest.result).FontSize(11).TextColor(color).
				SingleLine().Tooltip(a.proxyTest.result)
		}
	})
}

// systemProxyBox shows the system proxy MyGo found.
func (a *app) systemProxyBox(c *ui.Context) {
	p := a.palette
	box := ui.Column(c).Padding(10, 12).Gap(4).Radius(10).
		Background(p.CardHover).Border(1, p.Border).
		Tooltip("也会读取 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY 环境变量。")
	box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoGlobe).FontSize(14).TextColor(p.Accent)
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				summary := a.systemProxySummary()
				ui.Text(c, summary).FontSize(12).SingleLine().Tooltip(summary)
				ui.Text(c, a.systemSourceLine()).FontSize(10).TextColor(p.Muted).
					SingleLine().Tooltip(a.systemSourceLine())
			})
			if a.systemRefreshing {
				ui.Spinner(c).Label("正在读取系统代理")
			} else {
				iconButton(c, p, "重新检测系统代理", icoRefresh, false, func() { a.refreshSystemAsync() })
			}
		})
		if a.system != nil && a.system.Note != "" {
			ui.Text(c, a.system.Note).FontSize(10).TextColor(p.Warning)
		}
		if a.systemErr != "" {
			ui.Text(c, "读取系统代理失败："+a.systemErr).FontSize(10).TextColor(p.Danger)
		}
	})
}

// systemSourceLine describes where the system proxy came from.
func (a *app) systemSourceLine() string {
	if a.system == nil {
		return "正在读取系统设置"
	}
	source := a.system.Source
	if source == "" {
		source = "环境变量"
	}
	if a.system.PAC != "" {
		source += " · PAC " + a.system.PAC
	}
	return source
}

// manualProxyForm edits a custom proxy.
func (a *app) manualProxyForm(c *ui.Context) {
	ui.Field(c, "代理类型", func() {
		if ui.Select(c, &a.proxyKindLabel, proxyKindLabels).Width(180).Changed() {
			a.setProxyKind(a.proxyKindLabel)
		}
	})

	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Field(c, "主机", func() {
			if ui.TextInput(c, &a.proxyDraft.Host).Placeholder("127.0.0.1").Changed() {
				a.commitProxy()
			}
		}).Grow(1).Error(a.proxyConfigError())
		ui.Column(c).Width(200).Children(func() {
			ui.Field(c, "端口", func() {
				if ui.NumberInput(c, &a.proxyPort, 1, 65535, 1).Changed() {
					a.commitProxy()
				}
			})
		})
	})

	ui.Row(c).Gap(12).Children(func() {
		ui.Field(c, "用户名（可选）", func() {
			if ui.TextInput(c, &a.proxyDraft.Username).Changed() {
				a.commitProxy()
			}
		}).Grow(1)
		ui.Field(c, "密码（可选）", func() {
			if ui.TextInput(c, &a.proxyDraft.Password).Password().Changed() {
				a.commitProxy()
			}
		}).Grow(1).Tooltip("密码保存在系统凭据库中，关闭设置时写入。")
	})

	ui.Field(c, "跳过代理的地址", func() {
		if ui.TextInput(c, &a.proxyDraft.Bypass).
			Placeholder("localhost, *.local, 10.0.0.0/8").Changed() {
			a.commitProxy()
		}
	}).Tooltip("用逗号或分号分隔；支持 <local>、*.example.com 和 CIDR。")
}

// settingsAppearance is the Appearance tab.
func (a *app) settingsAppearance(c *ui.Context) {
	a.settingsRow(c, icoSun, "主题", "浅色、深色，或跟随系统。", func() {
		a.segmented(c, settings.ThemeLabels,
			[]*ui.SVG{icoSun, icoMoon, icoMonitor},
			settings.ThemeIndex(a.s.Theme), false,
			func(i int) { a.setTheme(settings.ThemeValues[i]) })
	})
	a.settingsRow(c, icoList, "下载列表", "列表适合看进度，卡片适合浏览文件。", func() {
		a.segmented(c, viewLabels, []*ui.SVG{icoList, icoGrid}, a.viewIdx, false,
			func(i int) { a.setView(viewValues[i]) })
	})
}

// settingsAbout is the About tab.
func (a *app) settingsAbout(c *ui.Context) {
	p := a.palette
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		logo := ui.Box(c).Size(52, 52).Radius(15).Background(p.Accent)
		logo.Center()
		logo.Children(func() { ui.Icon(c, icoDownload).FontSize(24).TextColor(p.OnAccent) })
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, "下载器").FontSize(17).Bold()
			ui.Text(c, "版本 "+a.deps.version).FontSize(11).TextColor(p.Muted)
		})
	})

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icoInfo).FontSize(13).TextColor(p.Muted)
		ui.Text(c, "基于 MyGo 原生 UI").FontSize(11).TextColor(p.Muted).Grow(1)
		ui.Link(c, "MyGo", "https://github.com/egoist/mygo")
	})

	data := ui.Row(c).Gap(8).AlignItems(ui.Center).
		Tooltip("settings.json 保存设置，tasks.json 保存下载记录；代理密码在系统凭据库中。")
	data.Children(func() {
		ui.Icon(c, icoFolder).FontSize(14).TextColor(p.Muted)
		ui.Text(c, a.deps.dataDir).FontSize(10).TextColor(p.Muted).
			SingleLine().Grow(1).Tooltip(a.deps.dataDir)
		iconButton(c, p, "打开数据目录", icoFolderOpen, false, func() { a.openDataDir() })
		iconButton(c, p, "复制数据目录", icoCopy, false, func() {
			c.WriteClipboard(a.deps.dataDir)
			a.toast("已复制数据目录")
		})
	})
}

// openDataDir opens the app's data directory.
func (a *app) openDataDir() {
	if a.deps.openPath == nil {
		return
	}
	if err := a.deps.openPath(a.deps.dataDir); err != nil {
		a.toast("无法打开数据目录：" + err.Error())
	}
}
