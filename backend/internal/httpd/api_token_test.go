package httpd

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	previewutil "github.com/aoagents/agent-orchestrator/backend/internal/preview"
)

const testAPIToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestAPITokenMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := apiTokenMiddleware(testAPIToken, next)
	previewURL, err := previewutil.FileURL("http://127.0.0.1:3001", domain.SessionID("ao-1"), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	previewHost, err := url.Parse(previewURL)
	if err != nil {
		t.Fatal(err)
	}
	artifactURL, err := previewutil.ArtifactFileURL("http://127.0.0.1:3001", domain.SessionID("ao-1"), "report.html")
	if err != nil {
		t.Fatal(err)
	}
	artifactHost, err := url.Parse(artifactURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name             string
		method           string
		path             string
		host             string
		token            string
		rawAuthorization string
		want             int
	}{
		{name: "missing", method: http.MethodGet, path: "/api/v1/sessions", want: http.StatusUnauthorized},
		{name: "wrong", method: http.MethodGet, path: "/api/v1/sessions", token: "wrong", want: http.StatusUnauthorized},
		{name: "missing bearer scheme", method: http.MethodGet, path: "/api/v1/sessions", rawAuthorization: testAPIToken, want: http.StatusUnauthorized},
		{name: "right", method: http.MethodGet, path: "/api/v1/sessions", token: testAPIToken, want: http.StatusOK},
		{name: "health", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "preview get", method: http.MethodGet, path: "/index.html", host: previewHost.Host, want: http.StatusOK},
		{name: "artifact head", method: http.MethodHead, path: "/report.html", host: artifactHost.Host, want: http.StatusOK},
		{name: "preflight", method: http.MethodOptions, path: "/api/v1/sessions", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.host != "" {
				req.Host = tc.host
			}
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.rawAuthorization != "" {
				req.Header.Set("Authorization", tc.rawAuthorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusUnauthorized {
				var body struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Code != "TOKEN_REQUIRED" {
					t.Fatalf("code = %q, want TOKEN_REQUIRED", body.Code)
				}
			}
		})
	}
}

func TestAPITokenMiddlewareRefusesWebSocketWithoutToken(t *testing.T) {
	handler := apiTokenMiddleware(testAPIToken, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	req := httptest.NewRequest(http.MethodGet, "/mux", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("websocket status = %d, want 401", rec.Code)
	}
}

func TestAPITokenNeverAppearsInRequestLog(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	router := NewRouterWithControl(configForTokenLogTest(), log, nil, APIDeps{}, ControlDeps{})
	handler := apiTokenMiddleware(testAPIToken, router)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(logs.String(), testAPIToken) {
		t.Fatal("request log contains API token")
	}
}

func configForTokenLogTest() config.Config { return config.Config{} }
