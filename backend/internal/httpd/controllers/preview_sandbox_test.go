package controllers_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/requestscope"
	previewutil "github.com/aoagents/agent-orchestrator/backend/internal/preview"
)

// A workspace or artifact HTML page opened from the daemon's own origin would
// be same-origin with the no-auth API, and its reads carry no Origin. On the
// loopback listener such content is sandboxed into an opaque origin. The LAN
// listener and the isolated preview origins are left as they were.
func TestAPIOriginPreviewContentIsSandboxed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("<script>fetch('/api/v1/sessions')</script>"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newFakeSessionService()
	s := svc.sessions["ao-1"]
	s.Metadata.ArtifactDir = dir
	svc.sessions["ao-1"] = s
	router := httpd.NewRouterWithControl(config.Config{AllowedOrigins: config.DefaultAllowedOrigins},
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil, httpd.APIDeps{Sessions: svc}, httpd.ControlDeps{})

	get := func(target, host string, lan bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = host
		if lan {
			req = req.WithContext(requestscope.WithLAN(req.Context()))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s (host %s, lan %v) = %d %s", target, host, lan, w.Code, w.Body.String())
		}
		return w
	}

	const legacy = "/api/v1/sessions/ao-1/preview/files/page.html?source=artifact&raw=true"
	if got := get(legacy, "127.0.0.1:3001", false).Header().Get("Content-Security-Policy"); got != "sandbox allow-scripts allow-forms" {
		t.Fatalf("loopback API-origin preview CSP = %q, want the sandbox", got)
	}
	if got := get(legacy, "192.168.1.9:3011", true).Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("LAN preview CSP = %q, want none (the remote desktop serves it under its own preview origin)", got)
	}

	raw, err := previewutil.ArtifactFileURL("http://localhost:3001", "ao-1", "page.html")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := get(u.RequestURI(), u.Host, false).Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("isolated preview origin CSP = %q, want none", got)
	}
}
