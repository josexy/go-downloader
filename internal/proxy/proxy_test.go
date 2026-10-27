package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"go-downloader/internal/appinfo"
)

func TestParseEndpoint(t *testing.T) {
	cases := []struct {
		raw      string
		fallback Kind
		want     string
		wantUser string
		wantErr  bool
	}{
		{raw: "http://127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{raw: "127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{raw: "socks5://localhost:1080", want: "socks5://localhost:1080"},
		{raw: "socks5h://user:pass@localhost:1080", want: "socks5://user@localhost:1080", wantUser: "user"},
		{raw: "https://proxy.example.com", want: "https://proxy.example.com:443"},
		{raw: "socks://example.com", want: "socks5://example.com:1080"},
		{raw: "ftp://example.com", wantErr: true},
		{raw: "", wantErr: true},
		{raw: "http://:7890", wantErr: true},
	}
	for _, tc := range cases {
		ep, err := parseEndpoint(tc.raw, KindHTTP)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseEndpoint(%q) = %v, want error", tc.raw, ep)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseEndpoint(%q): %v", tc.raw, err)
			continue
		}
		if got := ep.String(); got != tc.want {
			t.Errorf("parseEndpoint(%q) = %q, want %q", tc.raw, got, tc.want)
		}
		if ep.Username != tc.wantUser {
			t.Errorf("parseEndpoint(%q) username = %q, want %q", tc.raw, ep.Username, tc.wantUser)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	ok := []Config{
		{Mode: ModeNone},
		{Mode: ModeSystem},
		{Mode: ModeManual, Kind: KindHTTP, Host: "127.0.0.1", Port: 7890},
		{Mode: ModeManual, Kind: KindSOCKS5, Host: "example.com", Port: 1080},
	}
	for _, c := range ok {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", c, err)
		}
	}
	bad := []Config{
		{Mode: "weird"},
		{Mode: ModeManual, Kind: KindHTTP, Host: "", Port: 7890},
		{Mode: ModeManual, Kind: KindHTTP, Host: "a b", Port: 7890},
		{Mode: ModeManual, Kind: KindHTTP, Host: "http://x", Port: 7890},
		{Mode: ModeManual, Kind: KindHTTP, Host: "x", Port: 0},
		{Mode: ModeManual, Kind: KindHTTP, Host: "x", Port: 70000},
		{Mode: ModeManual, Kind: "ftp", Host: "x", Port: 1},
	}
	for _, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", c)
		}
	}
}

func TestMatchesBypass(t *testing.T) {
	cases := []struct {
		host, bypass string
		want         bool
	}{
		{"example.com", "", false},
		{"example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"www.example.com", ".example.com", true},
		{"www.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"badexample.com", "example.com", false},
		{"10.1.2.3", "10.0.0.0/8", true},
		{"11.1.2.3", "10.0.0.0/8", false},
		{"127.0.0.1", "localhost,127.0.0.1", true},
		{"intranet", "<local>", true},
		{"anything.local", "<local>", false},
		{"example.com", "<local>", false},
		{"example.com", "*", true},
		{"example.com:8443", "example.com:8443", true},
		{"example.com", "https://example.com/base", true},
		{"Example.COM", "example.com", true},
		{"", "example.com", false},
	}
	for _, tc := range cases {
		if got := MatchesBypass(tc.host, tc.bypass); got != tc.want {
			t.Errorf("MatchesBypass(%q, %q) = %v, want %v", tc.host, tc.bypass, got, tc.want)
		}
	}
}

func TestParseWindowsProxy(t *testing.T) {
	sys := parseWindowsProxy(true, "http=127.0.0.1:8080;https=127.0.0.1:8081;socks=127.0.0.1:1080", "*.local;<local>")
	if sys.HTTP == nil || sys.HTTP.Port != 8080 {
		t.Fatalf("HTTP endpoint = %+v", sys.HTTP)
	}
	if sys.HTTPS == nil || sys.HTTPS.Port != 8081 {
		t.Fatalf("HTTPS endpoint = %+v", sys.HTTPS)
	}
	if sys.SOCKS == nil || sys.SOCKS.Kind != KindSOCKS5 || sys.SOCKS.Port != 1080 {
		t.Fatalf("SOCKS endpoint = %+v", sys.SOCKS)
	}
	if sys.Bypass != "*.local;<local>" {
		t.Errorf("bypass = %q", sys.Bypass)
	}

	sys = parseWindowsProxy(true, "127.0.0.1:7890", "")
	if sys.HTTP == nil || sys.HTTPS == nil || sys.HTTP.Port != 7890 {
		t.Fatalf("single proxy = %+v", sys)
	}

	if !parseWindowsProxy(false, "127.0.0.1:7890", "").Empty() {
		t.Error("a disabled proxy should be empty")
	}
}

func TestParseScutil(t *testing.T) {
	out := `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
    1 : 169.254/16
  }
  FTPPassive : 1
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 7891
  HTTPSProxy : 127.0.0.2
  SOCKSEnable : 1
  SOCKSPort : 1080
  SOCKSProxy : 127.0.0.3
}`
	sys := parseScutil(out)
	if sys.HTTP == nil || sys.HTTP.Host != "127.0.0.1" || sys.HTTP.Port != 7890 {
		t.Fatalf("HTTP = %+v", sys.HTTP)
	}
	if sys.HTTPS == nil || sys.HTTPS.Port != 7891 {
		t.Fatalf("HTTPS = %+v", sys.HTTPS)
	}
	if sys.SOCKS == nil || sys.SOCKS.Port != 1080 {
		t.Fatalf("SOCKS = %+v", sys.SOCKS)
	}
	if !strings.Contains(sys.Bypass, "*.local") || !strings.Contains(sys.Bypass, "169.254/16") {
		t.Errorf("bypass = %q", sys.Bypass)
	}

	if !parseScutil("<dictionary> {\n  HTTPEnable : 0\n}").Empty() {
		t.Error("a disabled proxy should be empty")
	}
}

func TestFromEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "localhost,127.0.0.1")
	t.Setenv("https_proxy", "http://127.0.0.1:7890")
	sys := fromEnvironment()
	if sys.HTTPS == nil || sys.HTTPS.Port != 7890 {
		t.Fatalf("HTTPS = %+v", sys.HTTPS)
	}
	if sys.HTTP == nil || sys.HTTP.Port != 7890 {
		t.Fatalf("HTTP falls back to HTTPS: %+v", sys.HTTP)
	}
	if sys.Bypass != "localhost,127.0.0.1" {
		t.Errorf("bypass = %q", sys.Bypass)
	}

	t.Setenv("https_proxy", "")
	t.Setenv("all_proxy", "socks5://127.0.0.1:1080")
	sys = fromEnvironment()
	if sys.SOCKS == nil || sys.SOCKS.Port != 1080 {
		t.Fatalf("ALL_PROXY = %+v", sys.SOCKS)
	}
	if sys.HTTP != nil || sys.HTTPS != nil {
		t.Errorf("a SOCKS ALL_PROXY should not make HTTP endpoints: %+v", sys)
	}
}

func TestHTTPProxyRoutes(t *testing.T) {
	var proxied atomic.Bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() || r.URL.Host != "example.com" {
			t.Errorf("proxy got %q, want the absolute URL", r.URL.String())
		}
		proxied.Store(true)
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	defer proxySrv.Close()
	host, port := splitHostPort(t, proxySrv.URL)

	client, err := NewClient(Config{Mode: ModeManual, Kind: KindHTTP, Host: host, Port: port}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://example.com/file.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" || !proxied.Load() {
		t.Fatalf("body %q, proxied %v", body, proxied.Load())
	}

	// A bypassed host goes direct: this fails to resolve, proving no proxy.
	client2, err := NewClient(Config{Mode: ModeManual, Kind: KindHTTP, Host: host, Port: port, Bypass: "example.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client2.CloseIdleConnections()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	if u, _ := client2.Transport.(*http.Transport).Proxy(req); u != nil {
		t.Errorf("bypassed host uses proxy %v", u)
	}
}

func TestSystemProxyRoutes(t *testing.T) {
	var proxied atomic.Bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Store(true)
		_, _ = w.Write([]byte("ok"))
	}))
	defer proxySrv.Close()
	host, port := splitHostPort(t, proxySrv.URL)
	sys := &System{HTTP: &Endpoint{Kind: KindHTTP, Host: host, Port: port}}
	client, err := NewClient(Config{Mode: ModeSystem}, sys)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !proxied.Load() {
		t.Error("system proxy was not used")
	}
}

func TestSOCKS5Routes(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through socks"))
	}))
	defer target.Close()

	addr, stop := startSOCKS5(t, "user", "pass")
	defer stop()
	host, port := splitHostPort(t, "http://"+addr)
	cfg := Config{Mode: ModeManual, Kind: KindSOCKS5, Host: host, Port: port, Username: "user", Password: "pass"}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if _, err := client.Get(target.URL + "/file"); err != nil {
		t.Fatalf("download through SOCKS5: %v", err)
	}
}

func TestDirectHeaders(t *testing.T) {
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	client, err := NewClient(Config{Mode: ModeNone}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", appinfo.UserAgent(""))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if seen.Get("Accept-Encoding") != "identity" {
		t.Errorf("Accept-Encoding = %q", seen.Get("Accept-Encoding"))
	}
	if seen.Get("User-Agent") != appinfo.UserAgent("") {
		t.Errorf("User-Agent = %q", seen.Get("User-Agent"))
	}
	if tr := client.Transport.(*http.Transport); !tr.DisableCompression {
		t.Error("the transport decompresses, which breaks Range downloads")
	}
}

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if _, err := Check(context.Background(), Config{Mode: ModeNone}, nil, srv.URL, ""); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := Check(context.Background(), Config{Mode: ModeManual, Kind: KindHTTP, Host: "", Port: 1}, nil, srv.URL, ""); err == nil {
		t.Error("Check with an invalid config should fail")
	}
}

func splitHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u := strings.TrimPrefix(rawURL, "http://")
	host, portStr, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

// startSOCKS5 runs a minimal SOCKS5 server, with username/password when user is
// set, that dials whatever it is asked for.
func startSOCKS5(t *testing.T, user, pass string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSOCKS5(conn, user, pass)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func serveSOCKS5(conn net.Conn, user, pass string) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	header := make([]byte, 2)
	if _, err := io.ReadFull(br, header); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}
	method := byte(0)
	if user != "" {
		method = 2
	}
	if _, err := conn.Write([]byte{5, method}); err != nil {
		return
	}
	if method == 2 {
		auth := make([]byte, 2)
		if _, err := io.ReadFull(br, auth); err != nil || auth[0] != 1 {
			return
		}
		u := make([]byte, auth[1])
		if _, err := io.ReadFull(br, u); err != nil {
			return
		}
		pl := make([]byte, 1)
		if _, err := io.ReadFull(br, pl); err != nil {
			return
		}
		p := make([]byte, pl[0])
		if _, err := io.ReadFull(br, p); err != nil {
			return
		}
		if string(u) != user || string(p) != pass {
			conn.Write([]byte{1, 1})
			return
		}
		conn.Write([]byte{1, 0})
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil || req[0] != 5 || req[1] != 1 {
		return
	}
	var host string
	switch req[3] {
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(br, l); err != nil {
			return
		}
		name := make([]byte, l[0])
		if _, err := io.ReadFull(br, name); err != nil {
			return
		}
		host = string(name)
	case 4:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(br, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	upstream, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, br); done <- struct{}{} }()
	go func() { io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
}

// TestDetectLive reads the machine's proxy settings, which must never fail
// hard: an unreadable platform falls back to the environment or direct.
func TestDetectLive(t *testing.T) {
	sys, err := Detect(context.Background())
	if sys == nil {
		t.Fatal("Detect returned no system")
	}
	t.Logf("source=%q summary=%q note=%q err=%v", sys.Source, sys.Describe(), sys.Note, err)
}
