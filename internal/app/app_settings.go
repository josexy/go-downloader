package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
)

var (
	sortLabels = []string{"最新优先", "名称", "大小", "状态"}
	sortKeys   = []settings.SortKey{settings.SortNewest, settings.SortName, settings.SortSize, settings.SortStatus}

	viewLabels = []string{"列表", "卡片"}
	viewValues = []settings.ViewMode{settings.ViewList, settings.ViewCards}

	proxyModes      = []proxy.Mode{proxy.ModeNone, proxy.ModeSystem, proxy.ModeManual}
	proxyModeLabels = []string{"无代理", "系统代理", "自定义"}

	proxyKinds      = []proxy.Kind{proxy.KindHTTP, proxy.KindHTTPS, proxy.KindSOCKS5}
	proxyKindLabels = []string{"HTTP 代理", "HTTPS 代理", "SOCKS5 代理"}

	maxConcurrencyLabels = []string{"1", "2", "3", "4", "5", "6", "7", "8"}
)

// themeSource maps the app's appearance to MyGo's window appearance.
func themeSource(theme settings.Theme) mygo.ThemeSource {
	switch theme {
	case settings.ThemeLight:
		return mygo.ThemeLight
	case settings.ThemeDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

// syncFields copies the settings into the values bound to the widgets.
func (a *app) syncFields() {
	a.themeIdx = settings.ThemeIndex(a.s.Theme)
	a.viewIdx = 0
	if a.s.View == settings.ViewCards {
		a.viewIdx = 1
	}
	a.sortLabel = sortLabelFor(a.s.Sort)
	a.maxLabel = numberLabel(a.s.MaxConcurrent)
	a.proxyModeIdx = proxyModeIndex(a.s.Proxy.Mode)
	a.proxyKindLabel = proxyKindLabel(a.s.Proxy.Kind)
	a.syncProxyDraft()
}

func (a *app) syncProxyDraft() {
	a.proxyDraft = a.s.Proxy
	if a.proxyDraft.Port == 0 {
		a.proxyDraft.Port = 7890
	}
	a.proxyPort = float64(a.proxyDraft.Port)
	a.proxyModeIdx = proxyModeIndex(a.proxyDraft.Mode)
	a.proxyKindLabel = proxyKindLabel(a.proxyDraft.Kind)
}

func sortLabelFor(key settings.SortKey) string {
	for i, k := range sortKeys {
		if k == key {
			return sortLabels[i]
		}
	}
	return sortLabels[0]
}

func sortKeyFor(label string) settings.SortKey {
	for i, l := range sortLabels {
		if l == label {
			return sortKeys[i]
		}
	}
	return settings.SortNewest
}

func proxyModeIndex(mode proxy.Mode) int {
	for i, m := range proxyModes {
		if m == mode {
			return i
		}
	}
	return 1
}

func proxyKindIndex(label string) int {
	for i, l := range proxyKindLabels {
		if l == label {
			return i
		}
	}
	return 0
}

func proxyKindLabel(kind proxy.Kind) string {
	for i, k := range proxyKinds {
		if k == kind {
			return proxyKindLabels[i]
		}
	}
	return proxyKindLabels[0]
}

// setTheme saves and applies an appearance.
func (a *app) setTheme(theme settings.Theme) {
	if a.s.Theme == theme {
		return
	}
	a.s.Theme = theme
	a.themeIdx = settings.ThemeIndex(theme)
	if a.deps.applyTheme != nil {
		a.deps.applyTheme(theme)
	}
	a.saveSettings()
}

// setView saves how the downloads show.
func (a *app) setView(view settings.ViewMode) {
	if a.s.View == view {
		return
	}
	a.s.View = view
	a.viewIdx = 0
	if view == settings.ViewCards {
		a.viewIdx = 1
	}
	a.saveSettings()
}

// setSort saves the list's order.
func (a *app) setSort(key settings.SortKey) {
	if a.s.Sort == key {
		return
	}
	a.s.Sort = key
	a.sortLabel = sortLabelFor(key)
	a.saveSettings()
}

// setMaxConcurrent saves and applies the concurrency limit.
func (a *app) setMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	a.s.MaxConcurrent = n
	a.maxLabel = numberLabel(n)
	a.engine.SetMaxConcurrent(n)
	a.saveSettings()
}

func (a *app) setNotify(on bool) {
	if a.s.NotifyOnComplete == on {
		return
	}
	a.s.NotifyOnComplete = on
	a.saveSettings()
}

func (a *app) setAutoStart(on bool) {
	if a.s.AutoStart == on {
		return
	}
	a.s.AutoStart = on
	a.engine.SetAutoStart(on)
	a.saveSettings()
}

// saveSettings writes the settings, normalizing them first.
func (a *app) saveSettings() {
	if a.store == nil {
		return
	}
	a.s.Proxy.Password = a.proxyDraft.Password
	a.s.Normalize(a.deps.defaultDir)
	if err := a.store.SaveSettings(a.s); err != nil {
		a.toast(err.Error())
	}
}

// openSettings shows the settings dialog, on the given tab.
func (a *app) openSettings(tab int) {
	a.syncProxyDraft()
	a.settingsTab = tab
	a.settingsOpen = true
	if tab == 1 {
		a.refreshSystemAsync()
	}
}

// closeSettings hides the dialog and keeps the proxy password.
func (a *app) closeSettings() {
	a.settingsOpen = false
	a.s.Proxy.Password = a.proxyDraft.Password
	if a.passwordDirty && a.store != nil && a.deps.secrets != nil {
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

// commitProxy applies the proxy the settings page edits.
func (a *app) commitProxy() {
	cfg := a.proxyDraft
	cfg.Port = int(a.proxyPort)
	cfg.Normalize()
	if cfg.Mode == proxy.ModeManual {
		if err := cfg.Validate(); err != nil {
			// Keep editing; do not replace a working proxy with an
			// incomplete one, and do not save it.
			a.proxyDraft = cfg
			return
		}
	}
	a.proxyDraft = cfg
	a.s.Proxy = cfg
	a.passwordDirty = true
	a.rebuildClient()
	a.saveSettings()
}

// setProxyMode chooses direct, system or a custom proxy.
func (a *app) setProxyMode(index int) {
	if index < 0 || index >= len(proxyModes) {
		return
	}
	mode := proxyModes[index]
	a.proxyModeIdx = index
	a.proxyDraft.Mode = mode
	if mode == proxy.ModeManual && strings.TrimSpace(a.proxyDraft.Host) == "" {
		a.proxyDraft.Host = "127.0.0.1"
		if a.proxyPort < 1 {
			a.proxyPort = 7890
		}
	}
	a.commitProxy()
	if mode == proxy.ModeSystem {
		a.refreshSystemAsync()
	}
}

// setProxyKind chooses the custom proxy's protocol.
func (a *app) setProxyKind(label string) {
	index := proxyKindIndex(label)
	a.proxyKindLabel = proxyKindLabels[index]
	a.proxyDraft.Kind = proxyKinds[index]
	a.commitProxy()
}

// setProxyPort applies a typed port.
func (a *app) setProxyPort(port float64) {
	a.proxyPort = port
	a.commitProxy()
}

// setProxyHost applies a typed host.
func (a *app) setProxyHost(host string) {
	a.proxyDraft.Host = strings.TrimSpace(host)
	a.commitProxy()
}

// setProxyAuth applies typed credentials.
func (a *app) setProxyAuth(user, password string) {
	a.proxyDraft.Username = strings.TrimSpace(user)
	a.proxyDraft.Password = password
	a.commitProxy()
}

// setProxyBypass applies the bypass list.
func (a *app) setProxyBypass(bypass string) {
	a.proxyDraft.Bypass = strings.TrimSpace(bypass)
	a.commitProxy()
}

// proxyConfigError explains why the custom proxy cannot be used.
func (a *app) proxyConfigError() string {
	if a.proxyDraft.Mode != proxy.ModeManual {
		return ""
	}
	cfg := a.proxyDraft
	cfg.Port = int(a.proxyPort)
	if err := cfg.Validate(); err != nil {
		return err.Error()
	}
	return ""
}

// chooseFolder asks for the download folder, off the main thread.
func (a *app) chooseFolder() {
	if a.deps.pickFolder == nil {
		return
	}
	win := a.win
	a.deps.async(func() {
		dir, err := a.deps.pickFolder(win, "选择下载文件夹")
		a.dispatch(func() {
			if err != nil {
				a.toast("无法打开文件夹选择器：" + err.Error())
				return
			}
			if dir == "" {
				return
			}
			if err := a.setDownloadDir(dir); err != nil {
				a.toast(err.Error())
			}
		})
	})
}

// setDownloadDir checks the folder and makes it the download folder.
func (a *app) setDownloadDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("下载文件夹不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法使用这个文件夹：%w", err)
	}
	probe, err := os.CreateTemp(dir, ".downloader-test-*")
	if err != nil {
		return fmt.Errorf("这个文件夹不可写：%w", err)
	}
	name := probe.Name()
	probe.Close()
	_ = os.Remove(name)
	a.s.DownloadDir = dir
	a.engine.SetDir(dir)
	a.saveSettings()
	a.toast("下载文件夹已更新")
	return nil
}

// runProxyTest checks the proxy the settings page edits.
func (a *app) runProxyTest() {
	if a.proxyTest.running || a.deps.testProxy == nil {
		return
	}
	cfg := a.proxyDraft
	cfg.Port = int(a.proxyPort)
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		a.proxyTest = proxyTest{result: err.Error()}
		return
	}
	sys := a.system
	a.proxyTest = proxyTest{running: true}
	a.deps.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		elapsed, err := a.deps.testProxy(ctx, cfg, sys, proxyTestURL)
		a.dispatch(func() {
			if err != nil {
				a.proxyTest = proxyTest{result: "连接失败：" + err.Error()}
				return
			}
			a.proxyTest = proxyTest{
				ok:     true,
				result: fmt.Sprintf("连接成功 · %d ms", elapsed.Milliseconds()),
			}
		})
	})
}

// systemProxySummary describes the detected system proxy for the sidebar.
func (a *app) systemProxySummary() string {
	switch a.s.Proxy.Mode {
	case proxy.ModeNone:
		return "直连，不使用代理"
	case proxy.ModeManual:
		return a.s.Proxy.Describe()
	default:
		if a.system == nil || a.system.Empty() {
			if a.systemErr != "" {
				return "系统代理读取失败"
			}
			return "未检测到系统代理，将直连"
		}
		return a.system.Describe()
	}
}

// proxySummaryShort is the sidebar's one-line proxy state.
func (a *app) proxySummaryShort() string {
	switch a.s.Proxy.Mode {
	case proxy.ModeNone:
		return "直连"
	case proxy.ModeManual:
		return "自定义 · " + a.s.Proxy.Host
	default:
		if a.system == nil || a.system.Empty() {
			return "系统代理 · 未检测到"
		}
		if a.system.SOCKS != nil && a.system.HTTP == nil && a.system.HTTPS == nil {
			return "系统代理 · SOCKS5 " + a.system.SOCKS.Address()
		}
		if a.system.HTTPS != nil {
			return "系统代理 · " + a.system.HTTPS.Address()
		}
		if a.system.HTTP != nil {
			return "系统代理 · " + a.system.HTTP.Address()
		}
		return "系统代理"
	}
}

// folderLabel shortens a folder path for the sidebar card.
func folderLabel(dir string) string {
	if dir == "" {
		return "未设置"
	}
	return filepath.Base(dir)
}
