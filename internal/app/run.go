// Package app implements the downloader's desktop application.
package app

import (
	"github.com/egoist/mygo"
	// MyGo's own controls, menus and dialogs in Simplified Chinese.
	_ "github.com/egoist/mygo/locales/zh-Hans"
)

// Run initializes and runs the desktop application on the main goroutine.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running instance comes to the front
	}
	// The app's texts are Chinese, so MyGo's own controls follow.
	mygo.App.SetLanguages("zh-Hans", "en")
	mygo.App.SetLocale("zh-Hans")
	a := newApp(realDeps())
	a.load()
	mygo.Theme.SetSource(themeSource(a.s.Theme))
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.win != nil {
			a.win.Show()
			a.win.Focus()
		}
	})
	mygo.App.OnNotificationClick(func(string) {
		if a.win != nil {
			a.win.Show()
			a.win.Focus()
		}
	})
	mygo.App.WhenReady(a.start)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.shutdown() })
	return mygo.App.Run()
}
