//go:build darwin

package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// detectPlatform reads the proxies System Settings configures, as scutil
// reports them.
func detectPlatform(ctx context.Context) (*System, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return nil, fmt.Errorf("无法读取 macOS 系统代理设置: %w", err)
	}
	sys := parseScutil(string(out))
	sys.Source = "macOS 系统代理"
	return sys, nil
}
