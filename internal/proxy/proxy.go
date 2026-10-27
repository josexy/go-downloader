// Package proxy builds HTTP clients for the downloader: direct connections, a
// custom HTTP/HTTPS/SOCKS5 proxy, or the system's proxy settings.
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	xproxy "golang.org/x/net/proxy"

	"go-downloader/internal/appinfo"
)

// Mode is how downloads reach the network.
type Mode string

const (
	// ModeNone connects directly.
	ModeNone Mode = "none"
	// ModeSystem uses the system's proxy settings, falling back to the
	// environment.
	ModeSystem Mode = "system"
	// ModeManual uses the proxy in the rest of the Config.
	ModeManual Mode = "manual"
)

// Kind is the protocol of a proxy.
type Kind string

const (
	KindHTTP   Kind = "http"
	KindHTTPS  Kind = "https"
	KindSOCKS5 Kind = "socks5"
)

// Config is the user's proxy choice.
type Config struct {
	Mode     Mode   `json:"mode"`
	Kind     Kind   `json:"kind,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	// Password is never written to the settings file: it is kept in the
	// system's credential store, or in memory when there is none.
	Password string `json:"-"`
	// Bypass lists hosts that skip the proxy, separated by commas or
	// semicolons. It accepts host names, *.example.com, .example.com,
	// 10.0.0.0/8, host:port and <local>.
	Bypass string `json:"bypass,omitempty"`
}

// Normalize fills in defaults for an unset mode or kind.
func (c *Config) Normalize() {
	if c.Mode != ModeNone && c.Mode != ModeSystem && c.Mode != ModeManual {
		c.Mode = ModeSystem
	}
	if c.Mode == ModeManual {
		switch c.Kind {
		case KindHTTP, KindHTTPS, KindSOCKS5:
		default:
			c.Kind = KindHTTP
		}
	}
	c.Host = strings.TrimSpace(c.Host)
}

// Validate reports why c cannot be used, in a message fit for the UI.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeNone, ModeSystem:
		return nil
	case ModeManual:
	default:
		return fmt.Errorf("未知的代理模式 %q", c.Mode)
	}
	switch c.Kind {
	case KindHTTP, KindHTTPS, KindSOCKS5:
	default:
		return fmt.Errorf("未知的代理类型 %q", c.Kind)
	}
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return fmt.Errorf("代理主机不能为空")
	}
	if strings.ContainsAny(host, " \t\r\n/\\") || strings.Contains(host, "://") {
		return fmt.Errorf("代理主机格式不正确")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("代理端口必须在 1 到 65535 之间")
	}
	return nil
}

// KindLabel names a proxy kind in the UI's language.
func KindLabel(k Kind) string {
	switch k {
	case KindHTTPS:
		return "HTTPS 代理"
	case KindSOCKS5:
		return "SOCKS5 代理"
	default:
		return "HTTP 代理"
	}
}

// Describe returns a short, password-free description for the UI.
func (c Config) Describe() string {
	c.Normalize()
	switch c.Mode {
	case ModeNone:
		return "直连"
	case ModeSystem:
		return "系统代理"
	default:
		return fmt.Sprintf("%s · %s", KindLabel(c.Kind), net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	}
}

// Endpoint is one proxy address.
type Endpoint struct {
	Kind     Kind
	Host     string
	Port     int
	Username string
	Password string
}

// Address returns host:port.
func (e Endpoint) Address() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// Scheme returns the URL scheme of the endpoint.
func (e Endpoint) Scheme() string {
	if e.Kind == KindSOCKS5 {
		return "socks5"
	}
	return string(e.Kind)
}

// URL returns the endpoint as a proxy URL, with credentials when set.
func (e Endpoint) URL() *url.URL {
	u := &url.URL{Scheme: e.Scheme(), Host: e.Address()}
	if e.Username != "" {
		u.User = url.UserPassword(e.Username, e.Password)
	}
	return u
}

// String shows the endpoint without its password.
func (e Endpoint) String() string {
	s := e.Scheme() + "://"
	if e.Username != "" {
		s += e.Username + "@"
	}
	return s + e.Address()
}

// System is what the operating system (or the environment) says about proxies.
// It is immutable once returned.
type System struct {
	HTTP   *Endpoint
	HTTPS  *Endpoint
	SOCKS  *Endpoint
	Bypass string
	// Source names where the settings came from, such as "Windows 系统代理".
	Source string
	// Note explains an unusual situation, such as an unparsed PAC script.
	Note string
	// PAC is the automatic configuration URL when one is set.
	PAC string
}

// Empty reports whether no proxy was found.
func (s *System) Empty() bool {
	return s == nil || (s.HTTP == nil && s.HTTPS == nil && s.SOCKS == nil)
}

// Describe returns a short, password-free summary for the UI.
func (s *System) Describe() string {
	if s.Empty() {
		return "未检测到系统代理"
	}
	var parts []string
	if s.HTTP != nil && s.HTTPS != nil && s.HTTP.Address() == s.HTTPS.Address() {
		// One proxy for both schemes, as most system settings have.
		parts = append(parts, "代理 "+s.HTTP.Address())
	} else {
		if s.HTTPS != nil {
			parts = append(parts, "HTTPS "+s.HTTPS.Address())
		}
		if s.HTTP != nil {
			parts = append(parts, "HTTP "+s.HTTP.Address())
		}
	}
	if s.SOCKS != nil {
		parts = append(parts, "SOCKS5 "+s.SOCKS.Address())
	}
	return strings.Join(parts, " · ")
}

// Detect reads the system's proxy settings, falling back to the environment
// and then to a direct connection. The returned System is never nil; the
// error only reports that the platform's own settings could not be read.
func Detect(ctx context.Context) (*System, error) {
	sys, err := detectPlatform(ctx)
	if sys == nil {
		sys = &System{}
	}
	if sys.Empty() {
		if env := fromEnvironment(); !env.Empty() {
			// Keep the platform's explanation, but use what we have.
			if sys.Note == "" {
				sys.Note = "系统没有可用的代理设置，已使用环境变量"
			}
			sys.HTTP, sys.HTTPS, sys.SOCKS = env.HTTP, env.HTTPS, env.SOCKS
			sys.Bypass = env.Bypass
			if sys.Source == "" {
				sys.Source = env.Source
			}
		}
	}
	if sys.Source == "" {
		sys.Source = "环境变量"
	}
	if sys.Empty() && sys.Note == "" {
		sys.Note = "未检测到系统代理"
	}
	return sys, err
}

// fromEnvironment reads HTTPS_PROXY, HTTP_PROXY, ALL_PROXY and NO_PROXY, in
// their upper and lower case forms.
func fromEnvironment() *System {
	get := func(names ...string) string {
		for _, name := range names {
			if v := strings.TrimSpace(os.Getenv(name)); v != "" {
				return v
			}
		}
		return ""
	}
	sys := &System{Source: "环境变量", Bypass: get("NO_PROXY", "no_proxy")}
	sys.HTTPS = parseEnvEndpoint(get("HTTPS_PROXY", "https_proxy"))
	sys.HTTP = parseEnvEndpoint(get("HTTP_PROXY", "http_proxy"))
	if all := get("ALL_PROXY", "all_proxy"); all != "" {
		ep := parseEnvEndpoint(all)
		if ep != nil {
			switch ep.Kind {
			case KindSOCKS5:
				sys.SOCKS = ep
			default:
				if sys.HTTP == nil {
					sys.HTTP = ep
				}
				if sys.HTTPS == nil {
					sys.HTTPS = ep
				}
			}
		}
	}
	if sys.HTTP == nil && sys.HTTPS != nil {
		sys.HTTP = sys.HTTPS
	}
	if sys.HTTPS == nil && sys.HTTP != nil {
		sys.HTTPS = sys.HTTP
	}
	return sys
}

func parseEnvEndpoint(raw string) *Endpoint {
	ep, err := parseEndpoint(raw, KindHTTP)
	if err != nil {
		return nil
	}
	return ep
}

// parseEndpoint parses "http://user:pass@host:port", "socks5://host:port" and
// bare "host:port" addresses.
func parseEndpoint(raw string, fallback Kind) (*Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("空的代理地址")
	}
	if !strings.Contains(raw, "://") {
		raw = string(fallback) + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("代理地址无效: %w", err)
	}
	var kind Kind
	switch strings.ToLower(u.Scheme) {
	case "http":
		kind = KindHTTP
	case "https":
		kind = KindHTTPS
	case "socks5", "socks5h", "socks":
		kind = KindSOCKS5
	default:
		return nil, fmt.Errorf("不支持的代理协议 %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("代理地址缺少主机名")
	}
	port := 0
	if p := u.Port(); p != "" {
		port, err = strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("代理端口无效")
		}
	}
	if port == 0 {
		switch kind {
		case KindHTTPS:
			port = 443
		case KindSOCKS5:
			port = 1080
		default:
			port = 80
		}
	}
	ep := &Endpoint{Kind: kind, Host: host, Port: port}
	if u.User != nil {
		ep.Username = u.User.Username()
		ep.Password, _ = u.User.Password()
	}
	return ep, nil
}

// NewTransport returns an HTTP transport for c. sys is used by ModeSystem and
// may be nil, which connects directly.
func NewTransport(c Config, sys *System) (*http.Transport, error) {
	c.Normalize()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		// Keep the bytes the server sends: a decompressing transport would
		// break Content-Length and Range accounting.
		DisableCompression: true,
		Proxy:              nil,
	}
	switch c.Mode {
	case ModeNone:
		return tr, nil
	case ModeManual:
		ep := Endpoint{Kind: c.Kind, Host: c.Host, Port: c.Port, Username: c.Username, Password: c.Password}
		if c.Kind == KindSOCKS5 {
			return transportWithSOCKS(tr, ep, dialer, c.Bypass)
		}
		tr.Proxy = proxyFunc(&ep, &ep, c.Bypass)
		return tr, nil
	case ModeSystem:
		if sys.Empty() {
			return tr, nil
		}
		switch {
		case sys.HTTP != nil || sys.HTTPS != nil:
			tr.Proxy = proxyFunc(sys.HTTP, sys.HTTPS, sys.Bypass)
		case sys.SOCKS != nil:
			return transportWithSOCKS(tr, *sys.SOCKS, dialer, sys.Bypass)
		}
		return tr, nil
	}
	return nil, fmt.Errorf("未知的代理模式 %q", c.Mode)
}

func transportWithSOCKS(tr *http.Transport, ep Endpoint, dialer *net.Dialer, bypass string) (*http.Transport, error) {
	var auth *xproxy.Auth
	if ep.Username != "" {
		auth = &xproxy.Auth{User: ep.Username, Password: ep.Password}
	}
	// SOCKS5 sends names to the proxy, which resolves them (socks5h).
	pd, err := xproxy.SOCKS5("tcp", ep.Address(), auth, dialer)
	if err != nil {
		return nil, fmt.Errorf("无法使用 SOCKS5 代理: %w", err)
	}
	cd, ok := pd.(xproxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 代理不支持可取消的连接")
	}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if MatchesBypass(address, bypass) {
			return dialer.DialContext(ctx, network, address)
		}
		return cd.DialContext(ctx, network, address)
	}
	return tr, nil
}

func proxyFunc(httpEP, httpsEP *Endpoint, bypass string) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if req.URL == nil {
			return nil, nil
		}
		port := req.URL.Port()
		if port == "" {
			port = "80"
			if req.URL.Scheme == "https" {
				port = "443"
			}
		}
		if MatchesBypass(net.JoinHostPort(req.URL.Hostname(), port), bypass) {
			return nil, nil
		}
		ep := httpEP
		if req.URL.Scheme == "https" && httpsEP != nil {
			ep = httpsEP
		}
		if ep == nil {
			return nil, nil
		}
		return ep.URL(), nil
	}
}

// NewClient returns an HTTP client for c. Downloads read its body; the client
// has no overall timeout, so large files are fine.
func NewClient(c Config, sys *System) (*http.Client, error) {
	tr, err := NewTransport(c, sys)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: tr}, nil
}

// Check makes a small request through c and reports how long it took.
func Check(ctx context.Context, c Config, sys *System, target, appVersion string) (time.Duration, error) {
	client, err := NewClient(c, sys)
	if err != nil {
		return 0, err
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", appinfo.UserAgent(appVersion))
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<12))
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return elapsed, fmt.Errorf("服务器返回 %s", resp.Status)
	}
	return elapsed, nil
}

// MatchesBypass reports whether host (optionally host:port) is in the bypass
// list. A rule with a port matches only a target with that same port.
func MatchesBypass(host, bypass string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	port := ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		port = p
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.TrimSpace(bypass) == "" {
		return false
	}
	hostIP := net.ParseIP(host)
	entries := strings.FieldsFunc(bypass, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})
	for _, raw := range entries {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			continue
		}
		if i := strings.Index(entry, "://"); i >= 0 {
			entry = entry[i+3:]
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			if hostIP != nil && network.Contains(hostIP) {
				return true
			}
			continue
		}
		if i := strings.Index(entry, "/"); i >= 0 {
			entry = entry[:i]
		}
		if h, p, err := net.SplitHostPort(entry); err == nil {
			if port != p {
				continue
			}
			entry = h
		}
		entry = strings.Trim(entry, "[]")
		if entry == "*" {
			return true
		}
		if entry == "<local>" {
			if hostIP == nil && !strings.Contains(host, ".") {
				return true
			}
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			if hostIP != nil && hostIP.Equal(ip) {
				return true
			}
			continue
		}
		entry = strings.TrimPrefix(entry, ".")
		entry = strings.TrimPrefix(entry, "*.")
		entry = strings.TrimPrefix(entry, ".")
		if entry == "" {
			continue
		}
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}
