package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go-downloader/internal/appinfo"
)

var (
	errRestart = errors.New("restart from the beginning")
	errStalled = errors.New("the server stopped sending data")
	errRemoved = errors.New("the download was removed")
)

// httpStatusError is a response the app cannot use.
type httpStatusError struct {
	code   int
	status string
}

func (e *httpStatusError) Error() string {
	return e.userMessage()
}

func (e *httpStatusError) userMessage() string {
	switch e.code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("服务器拒绝访问（%s），链接可能需要登录", e.status)
	case http.StatusNotFound:
		return "文件不存在（404 Not Found）"
	case http.StatusProxyAuthRequired:
		return "代理需要认证（407 Proxy Authentication Required）"
	case http.StatusTooManyRequests:
		return "请求过于频繁（429），请稍后重试"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Sprintf("服务器暂时不可用（%s）", e.status)
	default:
		return fmt.Sprintf("服务器返回 %s", e.status)
	}
}

// fetch makes one request, appends to the partial file, and finishes the
// download when the body is complete.
func (m *Manager) fetch(ctx context.Context, t *task, client *http.Client) error {
	cur := m.current(t)
	if cur.ID == "" {
		return errRemoved
	}
	m.mu.Lock()
	if t.removed {
		m.mu.Unlock()
		return errRemoved
	}
	etag, lastModified := t.etag, t.lastModified
	user, pass := t.user, t.pass
	headers := t.headers.Clone()
	m.mu.Unlock()
	if cur.NeedsHeaders {
		return fmt.Errorf("此任务需要请求头，请通过「高级新建」重新填写同一链接和请求头")
	}

	path := cur.Path
	partPath := path + ".part"
	metaPath := partPath + ".meta"
	partSize := fileSize(partPath)
	meta := partMeta{}
	if partSize > 0 {
		meta = readPartMeta(metaPath)
	}
	if etag == "" {
		etag = meta.ETag
	}
	if lastModified == "" {
		lastModified = meta.LastModified
	}
	hadPart := partSize > 0
	validator := ifRange(etag, lastModified)
	resume := hadPart && validator != ""
	if !resume {
		partSize = 0
	}

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cur.URL, nil)
	if err != nil {
		return err
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", appinfo.UserAgent(m.opts.AppVersion))
	}
	req.Header.Set("Accept-Encoding", "identity")
	if user != "" && req.Header.Get("Authorization") == "" {
		req.SetBasicAuth(user, pass)
	}
	if resume {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", partSize))
		req.Header.Set("If-Range", validator)
	}

	var stalled atomic.Bool
	timer := time.AfterFunc(m.stall, func() {
		stalled.Store(true)
		cancel()
	})
	defer timer.Stop()

	resp, err := requestClient(client, headers).Do(req)
	timer.Stop()
	if err != nil {
		if stalled.Load() {
			return errStalled
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer resp.Body.Close()
	if stalled.Load() {
		return errStalled
	}

	code := resp.StatusCode
	var total int64 = -1
	switch {
	case code == http.StatusPartialContent:
		start, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || !resume || start != partSize || !sameValidators(resp, etag, lastModified) {
			removePart(partPath, metaPath)
			return errRestart
		}
		total = size
		if total < 0 && resp.ContentLength >= 0 {
			total = start + resp.ContentLength
		}
		if total > 0 && meta.Total > 0 && total != meta.Total {
			removePart(partPath, metaPath)
			return errRestart
		}
	case code == http.StatusOK:
		if resume {
			resume = false
			partSize = 0
		}
		total = resp.ContentLength
	case code == http.StatusRequestedRangeNotSatisfiable:
		if size, ok := parseUnsatisfiedRange(resp.Header.Get("Content-Range")); ok && resume && size == partSize && sameValidators(resp, etag, lastModified) {
			return m.finalize(t, partPath, metaPath, partSize, partSize)
		}
		removePart(partPath, metaPath)
		return errRestart
	case code >= 200 && code < 300:
		if resume {
			resume = false
			partSize = 0
		}
		total = resp.ContentLength
	default:
		return &httpStatusError{code: code, status: resp.Status}
	}

	// A file name the response suggests wins over the one from the URL, but
	// only before any bytes are saved.
	if !hadPart {
		m.adoptName(t, resp)
		cur = m.current(t)
		path = cur.Path
		partPath = path + ".part"
		metaPath = partPath + ".meta"
		partSize = 0
	}

	if !resume {
		// A full response replaces the representation, including validators.
		// Keeping an old ETag here could make the next retry append to new
		// bytes using a validator for an entirely different file.
		etag, lastModified = "", ""
	}
	if v := resp.Header.Get("ETag"); v != "" {
		etag = v
	}
	if v := resp.Header.Get("Last-Modified"); v != "" {
		lastModified = v
	}
	resumable := code == http.StatusPartialContent ||
		strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")
	resumable = resumable && ifRange(etag, lastModified) != ""

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("无法创建下载文件夹：%w", err)
	}
	if err := writePartMeta(metaPath, partMeta{
		URL:          cur.URL,
		ETag:         etag,
		LastModified: lastModified,
		Total:        total,
		Resumable:    resumable,
	}); err != nil {
		return fmt.Errorf("无法写入下载记录：%w", err)
	}

	done := partSize
	m.beginTransfer(t, total, done, etag, lastModified, resumable)

	flags := os.O_WRONLY | os.O_CREATE
	if resume && partSize > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		done = 0
	}
	file, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return fmt.Errorf("无法写入文件：%w", err)
	}

	buf := make([]byte, 16*1024)
	for {
		allowed, limitErr := m.bandwidth.take(ctx, len(buf))
		if limitErr != nil {
			file.Close()
			return limitErr
		}
		// Only network time counts toward the stall timeout. Throttling and
		// disk writes can take longer without making a healthy request fail.
		timer.Reset(m.stall)
		n, readErr := resp.Body.Read(buf[:allowed])
		timer.Stop()
		m.bandwidth.refund(allowed - n)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				file.Close()
				return fmt.Errorf("写入文件失败：%w", writeErr)
			}
			done += int64(n)
			m.advance(t, int64(n), total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			file.Close()
			if stalled.Load() {
				return errStalled
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return readErr
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("无法保存文件：%w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("无法保存文件：%w", err)
	}
	// A pause that arrived while the last bytes were in flight keeps the
	// download paused, with a complete partial file to finalize on resume.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if total >= 0 && done != total {
		return fmt.Errorf("%w：已收到 %d 字节，应为 %d 字节", io.ErrUnexpectedEOF, done, total)
	}
	return m.finalize(t, partPath, metaPath, done, total)
}

// adoptName picks the file name the response suggests.
func (m *Manager) adoptName(t *task, resp *http.Response) {
	name := Sanitize(NameFromDisposition(resp.Header.Get("Content-Disposition")))
	if name == "" || name == "download" {
		cur := m.current(t)
		name = cur.FileName
	}
	if !HasExtension(name) {
		if ext := ExtensionForType(resp.Header.Get("Content-Type")); ext != "" {
			name = Sanitize(name + ext)
		}
	}
	if name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.removed || name == t.item.FileName {
		return
	}
	if t.customName {
		return
	}
	dir := filepath.Dir(t.item.Path)
	unreservePath(m.reserved, t.item.Path)
	p := uniquePath(dir, name, m.reserved)
	reservePath(m.reserved, p)
	t.item.Path = p
	t.item.FileName = filepath.Base(p)
}

func (m *Manager) beginTransfer(t *task, total, done int64, etag, lastModified string, resumable bool) {
	m.mu.Lock()
	t.item.State = StateDownloading
	t.item.Total = total
	t.item.Done = done
	t.item.Error = ""
	t.item.Attempt = 0
	t.item.Resumable = resumable
	t.item.ResumableKnown = true
	t.item.Speed = 0
	t.item.ETA = 0
	t.etag, t.lastModified = etag, lastModified
	t.lastSample = timeNow()
	t.lastDone = done
	m.mu.Unlock()
	m.notify()
}

func (m *Manager) advance(t *task, n, total int64) {
	now := timeNow()
	m.mu.Lock()
	t.item.Done += n
	if total >= 0 {
		t.item.Total = total
	}
	if elapsed := now.Sub(t.lastSample).Seconds(); elapsed >= 0.25 {
		instant := float64(t.item.Done-t.lastDone) / elapsed
		if t.item.Speed <= 0 {
			t.item.Speed = instant
		} else {
			t.item.Speed = 0.7*t.item.Speed + 0.3*instant
		}
		t.lastSample, t.lastDone = now, t.item.Done
		if t.item.Total > 0 && t.item.Speed > 1 {
			remaining := float64(t.item.Total - t.item.Done)
			if remaining < 0 {
				remaining = 0
			}
			t.item.ETA = time.Duration(remaining / t.item.Speed * float64(time.Second))
		} else {
			t.item.ETA = 0
		}
	}
	m.mu.Unlock()
	m.notifyThrottled()
}

// finalize moves the partial file to its name and marks the download done.
func (m *Manager) finalize(t *task, partPath, metaPath string, done, total int64) error {
	m.mu.Lock()
	if t.removed {
		m.mu.Unlock()
		return errRemoved
	}
	finalPath := t.item.Path
	m.mu.Unlock()

	if fileExists(finalPath) {
		m.mu.Lock()
		unreservePath(m.reserved, finalPath)
		finalPath = uniquePath(filepath.Dir(finalPath), filepath.Base(finalPath), m.reserved)
		reservePath(m.reserved, finalPath)
		t.item.Path = finalPath
		t.item.FileName = filepath.Base(finalPath)
		m.mu.Unlock()
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		return fmt.Errorf("无法保存文件：%w", err)
	}
	_ = os.Remove(metaPath)

	m.mu.Lock()
	t.item.State = StateCompleted
	t.item.Done = done
	if total >= 0 {
		t.item.Total = total
	} else if t.item.Total < 0 {
		t.item.Total = done
	}
	t.item.Speed = 0
	t.item.ETA = 0
	t.item.Error = ""
	t.item.Attempt = 0
	t.item.Missing = false
	t.item.FinishedAt = timeNow()
	item := t.item
	m.mu.Unlock()
	m.notify()
	m.mu.Lock()
	onComplete := m.opts.OnComplete
	m.mu.Unlock()
	if onComplete != nil {
		onComplete(item)
	}
	return nil
}

func removePart(partPath, metaPath string) {
	_ = os.Remove(partPath)
	_ = os.Remove(metaPath)
}

func ifRange(etag, lastModified string) string {
	if len(etag) >= 2 && strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`) {
		return etag
	}
	if _, err := http.ParseTime(lastModified); err == nil {
		return lastModified
	}
	return ""
}

// If-Range normally makes the server return a full response after a change.
// Reject contradictory validators even when a server still returns 206/416.
func sameValidators(resp *http.Response, etag, lastModified string) bool {
	if current := resp.Header.Get("ETag"); etag != "" && current != "" && current != etag {
		return false
	}
	if current := resp.Header.Get("Last-Modified"); lastModified != "" && current != "" && current != lastModified {
		return false
	}
	return true
}

// parseContentRange reads "bytes 100-199/1234" or "bytes 100-199/*".
func parseContentRange(value string) (start, total int64, ok bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "bytes ") {
		return 0, 0, false
	}
	rest := strings.TrimSpace(value[len("bytes "):])
	rangePart, sizePart, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	startPart, _, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startPart), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	total = -1
	if sizePart = strings.TrimSpace(sizePart); sizePart != "*" {
		total, err = strconv.ParseInt(sizePart, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	return start, total, true
}

// parseUnsatisfiedRange reads "bytes */1234".
func parseUnsatisfiedRange(value string) (total int64, ok bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "bytes ") {
		return 0, false
	}
	rest := strings.TrimSpace(value[len("bytes "):])
	if !strings.HasPrefix(rest, "*/") {
		return 0, false
	}
	total, err := strconv.ParseInt(strings.TrimSpace(rest[2:]), 10, 64)
	if err != nil {
		return 0, false
	}
	return total, true
}

func retryable(err error) bool {
	if errors.Is(err, errStalled) {
		return true
	}
	var status *httpStatusError
	if errors.As(err, &status) {
		return status.code >= 500 || status.code == http.StatusRequestTimeout ||
			status.code == http.StatusTooManyRequests || status.code == http.StatusTooEarly
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return false
	}
	return false
}

func errorText(err error) string {
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return "网络错误：" + requestErr.Op + " " + DiagnosticURL(requestErr.URL) + "：" + errorText(requestErr.Err)
	}
	switch {
	case errors.Is(err, errStalled):
		return "下载超时：服务器长时间没有发送数据"
	case errors.Is(err, errRemoved):
		return "下载已取消"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "连接提前中断，文件不完整"
	}
	var status *httpStatusError
	if errors.As(err, &status) {
		return status.userMessage()
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "网络错误：" + err.Error()
	}
	return err.Error()
}
