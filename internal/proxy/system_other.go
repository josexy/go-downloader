//go:build !windows && !darwin

package proxy

import "context"

// detectPlatform has no per-desktop proxy settings to read here; the
// environment is used instead.
func detectPlatform(ctx context.Context) (*System, error) {
	return &System{}, nil
}
