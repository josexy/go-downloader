package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSOCKS5Bypass(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "direct")
	}))
	defer target.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connections atomic.Int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			conn.Close()
		}
	}()
	host, port := splitHostPort(t, "http://"+listener.Addr().String())
	for _, mode := range []Mode{ModeManual, ModeSystem} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := Config{Mode: mode, Kind: KindSOCKS5, Host: host, Port: port, Bypass: "127.0.0.1"}
			sys := &System{SOCKS: &Endpoint{Kind: KindSOCKS5, Host: host, Port: port}, Bypass: cfg.Bypass}
			client, err := NewClient(cfg, sys)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("bypassed target did not connect directly: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "direct" || connections.Load() != 0 {
				t.Fatalf("body=%q err=%v proxy connections=%d", body, err, connections.Load())
			}
		})
	}
}

func TestHTTPBypassPorts(t *testing.T) {
	for _, tc := range []struct {
		url, bypass string
		wantDirect  bool
	}{
		{"https://example.com/file", "example.com:8443", false},
		{"https://example.com/file", "example.com:443", true},
		{"http://example.com/file", "example.com:80", true},
		{"http://example.com/file", "example.com:443", false},
		{"https://example.com:8443/file", "example.com:8443", true},
		{"https://example.com:8443/file", "example.com", true},
		{"http://[::1]:8080/file", "[::1]:8080", true},
		{"http://[::1]:8080/file", "[::1]:8081", false},
	} {
		t.Run(tc.url+"/"+tc.bypass, func(t *testing.T) {
			client, err := NewClient(Config{Mode: ModeManual, Kind: KindHTTP, Host: "proxy.invalid", Port: 8080, Bypass: tc.bypass}, nil)
			if err != nil {
				t.Fatal(err)
			}
			req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
			ep, err := client.Transport.(*http.Transport).Proxy(req)
			if err != nil || (ep == nil) != tc.wantDirect {
				t.Fatalf("proxy=%v err=%v, want direct=%v", ep, err, tc.wantDirect)
			}
		})
	}
}
