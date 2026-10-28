package download

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

// AddOptions configures one advanced download. Headers stay in memory and are
// never included in Item, Record, or partial-file metadata.
type AddOptions struct {
	URL      string
	Dir      string
	FileName string
	Headers  http.Header
}

// ParseHeaders reads one Name: value header per line. Protocol headers affecting
// transfer framing, range validation, and connections belong to the engine.
func ParseHeaders(raw string) (http.Header, error) {
	headers := make(http.Header)
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("第 %d 行请求头格式不正确，应为 名称: 值", i+1)
		}
		headers.Add(name, value)
	}
	return validateHeaders(headers)
}

func validateHeaders(input http.Header) (http.Header, error) {
	headers := make(http.Header)
	for name, values := range input {
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("请求头名称格式不正确")
		}
		key := http.CanonicalHeaderKey(name)
		switch key {
		case "Host", "Range", "If-Range", "Accept-Encoding", "Content-Length",
			"Transfer-Encoding", "Connection", "Keep-Alive", "Te", "Trailer", "Upgrade",
			"If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since":
			return nil, fmt.Errorf("%s 由下载器管理，不能自定义", key)
		}
		if strings.HasPrefix(key, "Proxy-") {
			return nil, fmt.Errorf("代理请求头请通过代理设置配置")
		}
		for _, value := range values {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, fmt.Errorf("请求头值包含无效字符")
			}
			headers.Add(key, value)
		}
	}
	return headers, nil
}

// AddTask adds a single configured task, or supplies new in-memory headers and
// credentials to the same stopped task without discarding its partial data.
// Directory checks perform I/O, so callers should run this off the UI thread.
func (m *Manager) AddTask(opts AddOptions) (string, error) {
	target, err := parseTarget(strings.TrimSpace(opts.URL))
	if err != nil {
		return "", fmt.Errorf("请输入一条有效的 HTTP/HTTPS 链接")
	}
	headers, err := validateHeaders(opts.Headers)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(opts.FileName)
	customName := name != ""
	if customName && (strings.ContainsAny(name, `/\`) || name == "." || name == "..") {
		return "", fmt.Errorf("文件名不能包含路径或目录分隔符")
	}
	if customName {
		name = Sanitize(name)
	} else {
		u, _ := url.Parse(target.url)
		name = NameFromURL(u)
	}
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		m.mu.Lock()
		dir = m.dir
		m.mu.Unlock()
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("保存目录无效：%w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建保存目录：%w", err)
	}
	probe, err := os.CreateTemp(dir, ".downloader-check-*")
	if err != nil {
		return "", fmt.Errorf("保存目录不可写：%w", err)
	}
	probe.Close()
	_ = os.Remove(probe.Name())

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", fmt.Errorf("下载器正在退出")
	}
	var selected *task
	for _, t := range m.tasks {
		if t.item.URL == target.display && (t.item.State != StateCompleted || t.item.Missing) {
			selected = t
			break
		}
	}
	if selected != nil {
		if selected.working || selected.item.State.Active() {
			m.mu.Unlock()
			return "", fmt.Errorf("相同链接的任务正在运行，请先暂停后再更新")
		}
		if selected.item.NeedsHeaders && len(headers) == 0 {
			m.mu.Unlock()
			return "", fmt.Errorf("请补填此任务需要的请求头")
		}
		// Existing paths are deliberately preserved when refreshing credentials.
		selected.headers = headers
		if target.user != "" {
			selected.user, selected.pass = target.user, target.pass
		}
		selected.item.NeedsHeaders = false
		selected.item.Error = ""
		selected.item.FinishedAt = time.Time{}
	} else {
		path := uniquePath(dir, name, m.reserved)
		reservePath(m.reserved, path)
		selected = &task{
			item: Item{ID: newID(), URL: target.display, DisplayURL: target.display,
				FileName: filepath.Base(path), Path: path, Total: -1, CreatedAt: timeNow()},
			user: target.user, pass: target.pass, headers: headers, customName: customName,
		}
		m.tasks = append(m.tasks, selected)
		m.byID[selected.item.ID] = selected
	}
	selected.item.State = StatePaused
	if m.auto {
		selected.item.State = StateQueued
	}
	id := selected.item.ID
	m.mu.Unlock()
	m.notify()
	m.kick()
	return id, nil
}

func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// requestClient copies the shared client so task-specific redirect protection
// never mutates another transfer's client or its proxy transport.
func requestClient(base *http.Client, headers http.Header) *http.Client {
	client := *base
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if base.CheckRedirect != nil {
			if err := base.CheckRedirect(req, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return fmt.Errorf("重定向次数过多")
		}
		crossed := !sameOrigin(req.URL, via[0].URL)
		for _, hop := range via {
			crossed = crossed || !sameOrigin(hop.URL, via[0].URL)
		}
		if crossed {
			for key := range headers {
				req.Header.Del(key)
			}
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			req.Header.Del("Referer")
			req.URL.User = nil
		}
		return nil
	}
	return &client
}

// DiagnosticURL omits credentials and query values when a link is exported in
// diagnostic text. The original task URL remains available for downloading.
func DiagnosticURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "（无效链接）"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}

var diagnosticURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// DiagnosticText removes URL credentials and query values from error details.
func DiagnosticText(message string) string {
	return diagnosticURLPattern.ReplaceAllStringFunc(message, DiagnosticURL)
}

// taskError is called with the manager locked. Servers and transports can echo
// request values in their errors, so scrub all custom values and credentials.
func taskError(t *task, err error) string {
	message := errorText(err)
	credentials := []string{t.pass, t.user}
	for key, values := range t.headers {
		for _, value := range values {
			credentials = append(credentials, value)
			switch key {
			case "Cookie":
				for _, part := range strings.Split(value, ";") {
					if _, secret, ok := strings.Cut(part, "="); ok {
						credentials = append(credentials, strings.Trim(strings.TrimSpace(secret), `"`))
					}
				}
			case "Authorization":
				if fields := strings.Fields(value); len(fields) == 2 {
					credentials = append(credentials, fields[1])
					if strings.EqualFold(fields[0], "Basic") {
						if decoded, err := base64.StdEncoding.DecodeString(fields[1]); err == nil {
							user, pass, _ := strings.Cut(string(decoded), ":")
							credentials = append(credentials, string(decoded), user, pass)
						}
					}
				}
			}
		}
	}
	if t.user != "" {
		credentials = append(credentials, base64.StdEncoding.EncodeToString([]byte(t.user+":"+t.pass)))
	}
	// Redact complete values before shorter credentials that may be prefixes.
	sort.Slice(credentials, func(i, j int) bool { return len(credentials[i]) > len(credentials[j]) })
	for _, value := range credentials {
		if value != "" {
			for _, encoded := range []string{url.QueryEscape(value), url.PathEscape(value), value} {
				message = strings.ReplaceAll(message, encoded, "[已隐藏]")
			}
		}
	}
	return DiagnosticText(message)
}
