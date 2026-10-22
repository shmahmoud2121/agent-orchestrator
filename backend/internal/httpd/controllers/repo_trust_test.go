package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/controllers"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/project"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

func TestRepoTrustHTTPContract(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	path := filepath.Join(root, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"x":{"command":"echo test"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertProject(context.Background(), domain.ProjectRecord{ID: "p", Path: root}); err != nil {
		t.Fatal(err)
	}
	pc := &controllers.ProjectsController{Mgr: project.New(store)}
	r := chi.NewRouter()
	r.Route("/api/v1", pc.Register)
	request := func(method, body, caller string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/projects/p/repo-trust", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if caller != "" {
			req.Header.Set("X-AO-Session-ID", caller)
		}
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	get := request(http.MethodGet, "", "")
	if get.Code != 200 {
		t.Fatal(get.Body.String())
	}
	var scan repotrust.Scan
	if err := json.Unmarshal(get.Body.Bytes(), &scan); err != nil {
		t.Fatal(err)
	}
	body := `{"fingerprint":"` + scan.Fingerprint + `"}`
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if res := request(method, body, "worker-1"); res.Code != 403 {
			t.Fatalf("agent %s: %d %s", method, res.Code, res.Body.String())
		}
	}
	if res := request(http.MethodPost, `{"fingerprint":"`+scan.Fingerprint+`","callerSessionId":"orchestrator"}`, ""); res.Code != 403 {
		t.Fatalf("body caller: %d %s", res.Code, res.Body.String())
	}
	if res := request(http.MethodPost, body, ""); res.Code != 200 {
		t.Fatalf("approve: %d %s", res.Code, res.Body.String())
	}
	if res := request(http.MethodDelete, "", ""); res.Code != 200 {
		t.Fatalf("revoke: %d %s", res.Code, res.Body.String())
	}
	row, _, err := store.GetProject(context.Background(), "p")
	if err != nil || row.Config.TrustedRepoConfigFingerprint != "" {
		t.Fatalf("not revoked: %+v %v", row, err)
	}
	if err := os.WriteFile(path, []byte(`{"command":"changed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if res := request(http.MethodPost, body, ""); res.Code != 409 {
		t.Fatalf("stale: %d %s", res.Code, res.Body.String())
	}
}
