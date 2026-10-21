package httpd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/controllers"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/requestscope"
	previewutil "github.com/aoagents/agent-orchestrator/backend/internal/preview"
)

// countingMobileBridge records whether a request reached the bridge, so a
// refused request is shown to have had no side effect.
type countingMobileBridge struct {
	fakeMobileBridge
	lanEnables *atomic.Int32
}

func (b countingMobileBridge) EnableLANOnly() (controllers.MobileStatusResponse, error) {
	b.lanEnables.Add(1)
	return controllers.MobileStatusResponse{Enabled: true}, nil
}

func previewHostForTest(t *testing.T, sessionID string) string {
	t.Helper()
	raw, err := previewutil.FileURL("http://localhost:5181", domain.SessionID(sessionID), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func serve(router http.Handler, method, path, host, origin string, lan bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if lan {
		req = req.WithContext(requestscope.WithLAN(req.Context()))
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// A page on another localhost port (a dev server, a tool's web UI, a malicious
// package's page) must not drive agents. The app's own origin and native
// callers still reach the route.
func TestLoopbackOriginCannotCreateSessions(t *testing.T) {
	router := newTestRouter(config.Config{AllowedOrigins: config.DefaultAllowedOrigins}, discardLogger(), nil)
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:8080", "http://tool.localhost:3000"} {
		w := serve(router, http.MethodPost, "/api/v1/sessions", "127.0.0.1:3001", origin, false)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"ORIGIN_FORBIDDEN"`) {
			t.Errorf("POST /api/v1/sessions from %s = %d %s, want 403 ORIGIN_FORBIDDEN", origin, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("refused origin %s got Access-Control-Allow-Origin %q", origin, got)
		}
	}
	for _, origin := range []string{"app://renderer", ""} {
		// No sessions service is wired, so a request that reaches the handler
		// answers 501.
		if w := serve(router, http.MethodPost, "/api/v1/sessions", "127.0.0.1:3001", origin, false); w.Code != http.StatusNotImplemented {
			t.Errorf("POST /api/v1/sessions from %q = %d, want 501 (reached the handler)", origin, w.Code)
		}
	}
}

// The review's second finding: a localhost page turned on the LAN listener and
// read the pairing password from the response.
func TestLoopbackOriginCannotEnableRemoteAccess(t *testing.T) {
	var enables atomic.Int32
	router := NewRouterWithControl(config.Config{AllowedOrigins: config.DefaultAllowedOrigins}, discardLogger(), nil,
		APIDeps{Mobile: &controllers.MobileController{Bridge: countingMobileBridge{lanEnables: &enables}}}, ControlDeps{})

	previewHost := previewHostForTest(t, "ao-1")
	for _, tc := range []struct{ name, host, origin string }{
		{"dev server", "127.0.0.1:3001", "http://localhost:5173"},
		{"loopback IP", "127.0.0.1:3001", "http://127.0.0.1:8080"},
		// Admitted by corsMiddleware for its own files, never for these routes.
		{"preview on its own host", previewHost, "http://" + previewHost},
	} {
		for _, path := range []string{"/api/v1/mobile/enable-lan-only", "/api/v1/mobile/status"} {
			method := http.MethodPost
			if strings.HasSuffix(path, "/status") {
				method = http.MethodGet
			}
			w := serve(router, method, path, tc.host, tc.origin, false)
			if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "password") {
				t.Errorf("%s: %s %s = %d %s, want 403 without a password", tc.name, method, path, w.Code, w.Body.String())
			}
		}
	}
	if n := enables.Load(); n != 0 {
		t.Fatalf("refused requests enabled LAN access %d times", n)
	}

	for _, origin := range []string{"app://renderer", ""} {
		if w := serve(router, http.MethodPost, "/api/v1/mobile/enable-lan-only", "127.0.0.1:3001", origin, false); w.Code != http.StatusOK {
			t.Errorf("enable-lan-only from %q = %d %s, want 200", origin, w.Code, w.Body.String())
		}
	}
	if n := enables.Load(); n != 2 {
		t.Fatalf("app and CLI enables reached the bridge %d times, want 2", n)
	}
}

// A preview page loads its own modules with an Origin of its own host. That
// stays allowed; reaching another session's preview host does not.
func TestPreviewOriginIsAdmittedOnlyOnItsOwnHost(t *testing.T) {
	router := newTestRouter(config.Config{AllowedOrigins: config.DefaultAllowedOrigins}, discardLogger(), nil)
	own := previewHostForTest(t, "ao-1")
	other := previewHostForTest(t, "ao-2")

	w := serve(router, http.MethodGet, "/assets/app.js", own, "http://"+own, false)
	if w.Code == http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "http://"+own {
		t.Fatalf("preview loading its own asset = %d (ACAO %q), want it admitted", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w := serve(router, http.MethodGet, "/assets/app.js", other, "http://"+own, false); w.Code != http.StatusForbidden {
		t.Fatalf("preview reaching another session's preview host = %d, want 403", w.Code)
	}
	if w := serve(router, http.MethodGet, "/healthz", "127.0.0.1:3001", "http://"+own, false); w.Code != http.StatusForbidden {
		t.Fatalf("preview reaching the daemon API host = %d, want 403", w.Code)
	}
}

// The mobile app pins "Origin: http://localhost" on WebSocket upgrades over the
// LAN, where the pairing password authenticates every request.
func TestLANRequestsKeepLoopbackOrigins(t *testing.T) {
	router := newTestRouter(config.Config{AllowedOrigins: config.DefaultAllowedOrigins}, discardLogger(), nil)
	w := serve(router, http.MethodGet, "/healthz", "192.168.1.9:3011", "http://localhost", true)
	if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost" {
		t.Fatalf("LAN request with the mobile app's Origin = %d (ACAO %q), want 200", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w := serve(router, http.MethodGet, "/healthz", "192.168.1.9:3011", "http://evil.example", true); w.Code != http.StatusForbidden {
		t.Fatalf("LAN request with a foreign Origin = %d, want 403", w.Code)
	}
	if w := serve(router, http.MethodGet, "/healthz", "127.0.0.1:3001", "http://localhost", false); w.Code != http.StatusForbidden {
		t.Fatalf("loopback request with the same Origin = %d, want 403", w.Code)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	previewHost := previewHostForTest(t, "ao-1")
	for _, host := range []string{"127.0.0.1:3001", "localhost:3001", "LOCALHOST:3001", "localhost.:3001", "[::1]:3001", "[::1]", "127.0.0.2:3001", "localhost", previewHost} {
		if !isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"", "rebind.attacker.example", "rebind.attacker.example:3001", "localhost.evil.example:3001", "127.0.0.1.nip.io:3001", "192.168.1.9:3001", "0.0.0.0:3001", "[::]:3001"} {
		if isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = true, want false", host)
		}
	}
}

// The review's third finding: a DNS-rebound page read data with a foreign Host
// and no Origin. The loopback server refuses it; the router the LAN listener
// shares stays unguarded, since LAN clients use the machine's network name.
func TestLoopbackServerRefusesForeignHost(t *testing.T) {
	cfg := config.Config{Host: "127.0.0.1", Port: 0, RunFilePath: filepath.Join(t.TempDir(), "r.json"), AllowedOrigins: config.DefaultAllowedOrigins}
	srv, err := NewWithDeps(cfg, discardLogger(), nil, APIDeps{})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.listen.Close()

	w := serve(srv.http.Handler, http.MethodGet, "/healthz", "rebind.attacker.example:3001", "", false)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"HOST_FORBIDDEN"`) {
		t.Fatalf("rebound Host on the loopback server = %d %s, want 403 HOST_FORBIDDEN", w.Code, w.Body.String())
	}
	for _, host := range []string{"127.0.0.1:3001", "localhost:3001", "[::1]:3001", previewHostForTest(t, "ao-1")} {
		if w := serve(srv.http.Handler, http.MethodGet, "/healthz", host, "", false); w.Code == http.StatusForbidden {
			t.Errorf("loopback Host %q = 403, want it served", host)
		}
	}
	if w := serve(srv.Handler(), http.MethodGet, "/healthz", "192.168.1.9:3011", "", true); w.Code != http.StatusOK {
		t.Fatalf("LAN handler with a network Host = %d, want 200", w.Code)
	}
}
