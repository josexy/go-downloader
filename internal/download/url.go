package download

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// target is one parsed link, with the credentials kept out of the display URL.
type target struct {
	display string
	user    string
	pass    string
	url     string // the URL to request, without credentials
}

// NormalizeURLs parses the links in raw, one per line or space separated, and
// returns the valid ones without credentials, plus the tokens that are not.
func NormalizeURLs(raw string) (valid []string, invalid []string) {
	targets, invalid := parseTargets(raw)
	for _, t := range targets {
		valid = append(valid, t.display)
	}
	return valid, invalid
}

func parseTargets(raw string) (targets []target, invalid []string) {
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || unicode.IsSpace(r) || r == ',' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		t, err := parseTarget(token)
		if err != nil {
			invalid = append(invalid, token)
			continue
		}
		targets = append(targets, t)
	}
	return targets, invalid
}

func parseTarget(token string) (target, error) {
	if !strings.Contains(token, "://") {
		if !looksLikeHost(token) {
			return target{}, fmt.Errorf("不是有效的链接: %s", token)
		}
		token = "https://" + token
	}
	u, err := url.Parse(token)
	if err != nil {
		return target{}, fmt.Errorf("链接格式不正确: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return target{}, fmt.Errorf("只支持 HTTP 和 HTTPS 链接")
	}
	if u.Host == "" {
		return target{}, fmt.Errorf("链接缺少主机名")
	}
	t := target{user: "", pass: ""}
	if u.User != nil {
		t.user = u.User.Username()
		t.pass, _ = u.User.Password()
		u.User = nil
	}
	u.Fragment = ""
	u.Scheme = scheme
	t.url = u.String()
	t.display = t.url
	return t, nil
}

func looksLikeHost(token string) bool {
	host := token
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if host == "" || strings.ContainsAny(host, " \t") {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" {
		return true
	}
	return strings.Contains(host, ".") || net.ParseIP(host) != nil
}

// newID returns a random identity for a download.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", timeNow().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// ShortURL trims a URL for a row: its host and last path segment at most.
func ShortURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := u.Host
	if host == "" {
		return raw
	}
	path := u.Path
	if len(path) > 60 {
		path = path[:57] + "…"
	}
	return host + path
}
