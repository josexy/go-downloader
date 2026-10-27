// Package appinfo formats the application version and request identity without
// depending on the UI framework.
package appinfo

// NormalizeVersion uses dev when no version was injected by the build tool.
func NormalizeVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

// UserAgent identifies the app and its version to servers.
func UserAgent(version string) string {
	return "Downloader/" + NormalizeVersion(version) + " (MyGo)"
}
