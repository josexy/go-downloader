package app

import (
	"context"
	"time"

	"github.com/egoist/mygo"

	"go-downloader/internal/proxy"
	"go-downloader/internal/settings"
	"go-downloader/internal/store"
)

// deps are the world outside the app, so tests can replace it.
type deps struct {
	dataDir    string
	defaultDir string
	version    string
	secrets    store.SecretStore

	now       func() time.Time
	async     func(func())
	notify    func(title, body string)
	canNotify func() bool
	appActive func() bool

	pickFolder func(parent *mygo.Window, title string) (string, error)
	openPath   func(path string) error
	reveal     func(path string)

	detectSystem func(ctx context.Context) (*proxy.System, error)
	testProxy    func(ctx context.Context, cfg proxy.Config, sys *proxy.System, target string) (time.Duration, error)
	applyTheme   func(settings.Theme)
}

// realDeps wires the app to MyGo and the operating system.
func realDeps() deps {
	version := mygo.App.Version()
	if version == "" {
		version = "0.1.0"
	}
	return deps{
		version: version,
		secrets: mygoSecrets{},
		now:     time.Now,
		async:   func(fn func()) { go fn() },
		notify: func(title, body string) {
			if !mygo.NotificationsSupported() {
				return
			}
			_ = mygo.NewNotification(mygo.NotificationOptions{
				ID:    "download-complete",
				Title: title,
				Body:  body,
			}).Show()
		},
		canNotify: func() bool { return mygo.NotificationsSupported() },
		// App.IsActive is new in MyGo 0.7: whether the app is the one in
		// front, which is what decides if a completion needs a notification.
		appActive: func() bool { return mygo.App.IsActive() },
		pickFolder: func(parent *mygo.Window, title string) (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:            parent,
				Title:             title,
				Directory:         true,
				CreateDirectories: true,
				ButtonLabel:       "选择",
			})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		},
		openPath: func(path string) error { return mygo.Shell.OpenPath(path) },
		reveal:   func(path string) { mygo.Shell.ShowItemInFolder(path) },
		detectSystem: func(ctx context.Context) (*proxy.System, error) {
			return proxy.Detect(ctx)
		},
		testProxy: func(ctx context.Context, cfg proxy.Config, sys *proxy.System, target string) (time.Duration, error) {
			return proxy.Check(ctx, cfg, sys, target)
		},
		applyTheme: func(theme settings.Theme) {
			mygo.Theme.SetSource(themeSource(theme))
		},
	}
}

// mygoSecrets adapts MyGo's credential store to the store's interface.
type mygoSecrets struct{}

func (mygoSecrets) Get(key string) (string, error) {
	value, err := mygo.Secrets.Get(key)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (mygoSecrets) Set(key, value string) error {
	return mygo.Secrets.Set(key, []byte(value))
}

func (mygoSecrets) Delete(key string) error {
	return mygo.Secrets.Delete(key)
}
