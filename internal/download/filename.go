package download

import (
	"mime"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// NameFromURL suggests a file name from the last segment of a URL.
func NameFromURL(u *url.URL) string {
	name := path.Base(u.Path)
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if name == "." || name == "/" || name == "" {
		name = "download"
	}
	return Sanitize(name)
}

// NameFromDisposition reads a file name out of a Content-Disposition header,
// both filename and RFC 5987 filename* forms.
func NameFromDisposition(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(header); err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return name
		}
	}
	// A malformed header, as some servers send: look for filename= yourself.
	lower := strings.ToLower(header)
	for _, key := range []string{"filename*=utf-8''", "filename*=", "filename="} {
		i := strings.Index(lower, key)
		if i < 0 {
			continue
		}
		rest := header[i+len(key):]
		rest = strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(rest, `"`) {
			rest = rest[1:]
			if end := strings.Index(rest, `"`); end >= 0 {
				rest = rest[:end]
			}
		} else if end := strings.IndexAny(rest, ";,"); end >= 0 {
			rest = rest[:end]
		}
		rest = strings.TrimSpace(rest)
		if decoded, err := url.PathUnescape(rest); err == nil {
			rest = decoded
		}
		if rest != "" {
			return rest
		}
	}
	return ""
}

// ExtensionForType returns a file extension for a Content-Type, or "".
func ExtensionForType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	}
	mediaType = strings.ToLower(mediaType)
	if ext := commonExtensions[mediaType]; ext != "" {
		return ext
	}
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		// Prefer the shortest, friendliest extension.
		best := exts[0]
		for _, e := range exts[1:] {
			if len(e) < len(best) {
				best = e
			}
		}
		return best
	}
	return ""
}

var commonExtensions = map[string]string{
	"application/zip":                               ".zip",
	"application/x-zip-compressed":                  ".zip",
	"application/pdf":                               ".pdf",
	"application/x-7z-compressed":                   ".7z",
	"application/vnd.rar":                           ".rar",
	"application/x-rar-compressed":                  ".rar",
	"application/x-tar":                             ".tar",
	"application/gzip":                              ".gz",
	"application/x-msdownload":                      ".exe",
	"application/vnd.microsoft.portable-executable": ".exe",
	"application/x-msi":                             ".msi",
	"application/vnd.android.package-archive":       ".apk",
	"application/octet-stream":                      "",
	"video/mp4":                                     ".mp4",
	"video/quicktime":                               ".mov",
	"video/x-matroska":                              ".mkv",
	"video/webm":                                    ".webm",
	"audio/mpeg":                                    ".mp3",
	"audio/ogg":                                     ".ogg",
	"audio/wav":                                     ".wav",
	"audio/x-wav":                                   ".wav",
	"image/png":                                     ".png",
	"image/jpeg":                                    ".jpg",
	"image/gif":                                     ".gif",
	"image/webp":                                    ".webp",
	"image/svg+xml":                                 ".svg",
	"text/plain":                                    ".txt",
	"text/html":                                     ".html",
	"application/json":                              ".json",
	"application/msword":                            ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.ms-excel": ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
}

// HasExtension reports whether name already ends in a file extension.
func HasExtension(name string) bool {
	ext := filepath.Ext(name)
	if len(ext) < 2 || len(ext) > 12 {
		return false
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// Sanitize makes name safe to save in dir: it drops what Windows and Linux
// forbid, reserved device names, and shortens it to 180 bytes.
func Sanitize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "download"
	}
	// Some servers send a full path; keep the last segment.
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			continue
		case strings.ContainsRune(`<>:"|?*`, r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	name = strings.TrimRight(strings.TrimSpace(b.String()), ". ")
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	base := name
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	if reservedNames[strings.ToUpper(base)] {
		name = "_" + name
	}
	const maxBytes = 180
	if len(name) > maxBytes {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		stem := strings.TrimSuffix(name, ext)
		budget := maxBytes - len(ext)
		for len(stem) > budget {
			_, size := utf8.DecodeLastRuneInString(stem)
			stem = stem[:len(stem)-size]
		}
		name = stem + ext
	}
	return name
}

// pathKey normalizes a path for the reserved set: Windows paths compare
// without case.
func pathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// uniquePath returns a path for name in dir that neither exists nor is
// reserved, numbering it "name (2).ext" as Finder does.
func uniquePath(dir, name string, reserved map[string]bool) string {
	candidate := filepath.Join(dir, name)
	if !reserved[pathKey(candidate)] && !fileExists(candidate) {
		return candidate
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		candidate = filepath.Join(dir, base+" ("+itoa(i)+")"+ext)
		if !reserved[pathKey(candidate)] && !fileExists(candidate) {
			return candidate
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
