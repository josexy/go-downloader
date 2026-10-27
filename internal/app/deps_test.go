package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"go-downloader/internal/download"
	"go-downloader/internal/proxy"
)

// Check the complete path from MyGo's build metadata to the About page and
// both kinds of network request, including an unversioned go run build.
func TestBuildVersionReachesAboutAndRequests(t *testing.T) {
	previous := mygo.App.Version()
	t.Cleanup(func() { mygo.App.SetVersion(previous) })
	for _, tc := range []struct {
		name, version, want string
	}{
		{name: "packaged", version: "9.8.7", want: "9.8.7"},
		{name: "development", want: "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mygo.App.SetVersion(tc.version)
			wantAgent := "Downloader/" + tc.want + " (MyGo)"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.UserAgent(); got != wantAgent {
					t.Errorf("%s User-Agent = %q, want %q", r.URL.Path, got, wantAgent)
				}
				_, _ = w.Write([]byte("ok"))
			}))
			t.Cleanup(srv.Close)

			d := realDeps()
			d.dataDir, d.defaultDir = t.TempDir(), t.TempDir()
			d.secrets = &memSecrets{m: map[string]string{}}
			a := newApp(d)
			a.load()
			t.Cleanup(a.engine.Shutdown)
			t.Cleanup(func() {
				if h := a.client.Load(); h != nil && h.client != nil {
					h.client.CloseIdleConnections()
				}
			})
			a.openSettings(3)
			if tt := testerFor(a); !tt.HasText("版本 " + tc.want) {
				t.Fatalf("About page has no version %q: %q", tc.want, tt.Texts())
			}

			completed := make(chan download.Item, 1)
			a.engine.SetOnChange(nil)
			a.engine.SetOnComplete(func(it download.Item) { completed <- it })
			if added, _, invalid := a.engine.Add(srv.URL + "/download"); added != 1 {
				t.Fatalf("Add: added %d, invalid %v", added, invalid)
			}
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				t.Fatal("download did not complete")
			}
			if _, err := d.testProxy(context.Background(), proxy.Config{Mode: proxy.ModeNone}, nil, srv.URL+"/proxy-check"); err != nil {
				t.Fatalf("proxy check: %v", err)
			}
		})
	}
}
