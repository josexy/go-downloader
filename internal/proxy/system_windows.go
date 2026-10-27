//go:build windows

package proxy

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// detectPlatform reads WinINet's per-user proxy settings, which Settings and
// the system's own apps follow.
func detectPlatform(ctx context.Context) (*System, error) {
	const keyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("无法读取 Windows 系统代理设置: %w", err)
	}
	defer key.Close()
	var enabled uint64
	if v, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		enabled = v
	}
	server, _, _ := key.GetStringValue("ProxyServer")
	override, _, _ := key.GetStringValue("ProxyOverride")
	pac, _, _ := key.GetStringValue("AutoConfigURL")
	sys := parseWindowsProxy(enabled != 0, server, override)
	sys.Source = "Windows 系统代理"
	sys.PAC = strings.TrimSpace(pac)
	if sys.Empty() && sys.PAC != "" {
		sys.Note = "系统正在使用自动配置脚本（PAC），暂不支持解析，已回退到环境变量或直连"
	}
	return sys, nil
}
