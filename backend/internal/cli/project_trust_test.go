package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectTrustPrintsFilesAndApproves(t *testing.T) {
	cfg := setConfigEnv(t)
	t.Setenv("AO_SESSION_ID", "")
	approved := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telemetry/cli-invoked" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/api/v1/projects/demo/repo-trust" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Method == http.MethodPost {
			var in struct {
				Fingerprint string `json:"fingerprint"`
				Branch      string `json:"branch"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
			if in.Fingerprint != "abc" || in.Branch != "worker" {
				t.Errorf("request %+v", in)
			}
			approved = true
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"fingerprint":"abc","approved":false,"files":[{"path":".mcp.json","summary":["server: touch marker"]}]}`)
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)
	out, stderr, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "project", "trust", "demo", "--branch", "worker", "--yes")
	if err != nil || !approved || !strings.Contains(out, ".mcp.json") || !strings.Contains(out, "server: touch marker") {
		t.Fatalf("approved=%v out=%s stderr=%s err=%v", approved, out, stderr, err)
	}
}
func TestProjectTrustAgentSessionForbidden(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "worker-1")
	_, _, err := executeCLI(t, Deps{}, "project", "trust", "demo", "--yes")
	if err == nil || !strings.Contains(err.Error(), "REPO_TRUST_FORBIDDEN") {
		t.Fatalf("got %v", err)
	}
}

func TestProjectTrustConfirmationAndRevoke(t *testing.T) {
	for _, tc := range []struct {
		name, input        string
		revoke, wantChange bool
	}{{"decline", "n\n", false, false}, {"confirm", "yes\n", false, true}, {"revoke", "y\n", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := setConfigEnv(t)
			t.Setenv("AO_SESSION_ID", "")
			changed := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/internal/telemetry/cli-invoked" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method == http.MethodPost || r.Method == http.MethodDelete {
					changed = true
					if tc.revoke && r.Method != http.MethodDelete {
						t.Errorf("method %s", r.Method)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"fingerprint":"abc","approved":false,"files":[{"path":".mcp.json","summary":["command: test"]}]}`)
			}))
			t.Cleanup(srv.Close)
			writeRunFileFor(t, cfg, srv)
			args := []string{"project", "trust", "demo"}
			if tc.revoke {
				args = append(args, "--revoke")
			}
			_, _, err := executeCLI(t, Deps{In: strings.NewReader(tc.input), ProcessAlive: func(int) bool { return true }}, args...)
			if err != nil || changed != tc.wantChange {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
		})
	}
}
