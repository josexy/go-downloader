// Package resources embeds the assets shared by the UI and application packaging.
package resources

import _ "embed"

// IconPNG is the app icon used by MyGo packaging. Treat it as read-only.
//
//go:embed icon.png
var IconPNG []byte
