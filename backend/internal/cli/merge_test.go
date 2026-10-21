package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mergeCapture struct {
	path string
	body mergeSessionRequest
}

func mergeServer(t *testing.T, status int, respBody string) (*httptest.Server, *mergeCapture) {
	t.Helper()
	capture := &mergeCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/merge") {
			http.NotFound(w, r)
			return
		}
		capture.path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &capture.body); err != nil {
			t.Errorf("decode body %q: %v", data, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, capture
}

func TestMergeCommandMergesAndPrintsSummary(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "p1-orchestrator")
	cfg := setConfigEnv(t)
	srv, capture := mergeServer(t, http.StatusOK, `{"sessionId":"p1-3","branch":"ao/p1-3/root","baseBranch":"main","dryRun":false,
		"baseBefore":"1111111111111111","mergeCommit":"abcdef0123456789","commitCount":2,"changedFiles":["a.go"],"changedFilesTotal":1,
		"verified":true,"overlaps":[{"sessionId":"p1-4","branch":"ao/p1-4/root","files":["a.go"]}],"notified":["p1-4"],"warnings":[],
		"pushAttempted":true,"pushed":false,"pushedRef":"refs/heads/main","pushError":"! [rejected] main -> main (fetch first)"}`)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "merge", "p1-3")
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, errOut)
	}
	if capture.path != "/api/v1/sessions/p1-3/merge" || capture.body.DryRun || capture.body.CallerSessionID != "p1-orchestrator" {
		t.Fatalf("request = %s %+v", capture.path, capture.body)
	}
	for _, want := range []string{"merged p1-3 (ao/p1-3/root) into main as abcdef012345", "a.go", "verify command passed", "warning: p1-4", "told p1-4", "PUSH FAILED", "rejected"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMergeCommandDryRun(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "")
	cfg := setConfigEnv(t)
	srv, capture := mergeServer(t, http.StatusOK, `{"sessionId":"p1-3","branch":"b","baseBranch":"main","dryRun":true,"baseBefore":"1111","commitCount":1,"changedFiles":["x"],"changedFilesTotal":1,"overlaps":[],"notified":[],"warnings":[]}`)
	writeRunFileFor(t, cfg, srv)
	out, _, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "merge", "p1-3", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !capture.body.DryRun || capture.body.CallerSessionID != "" || !strings.Contains(out, "dry run: p1-3") {
		t.Fatalf("body = %+v out = %s", capture.body, out)
	}
}

func TestMergeCommandSurfacesConflict(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "")
	cfg := setConfigEnv(t)
	srv, _ := mergeServer(t, http.StatusConflict, `{"error":"conflict","code":"MERGE_CONFLICT","message":"Merging b into main conflicts; nothing landed. Conflicting files: a.go.","requestId":"r1"}`)
	writeRunFileFor(t, cfg, srv)
	_, _, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "merge", "p1-3")
	if err == nil || !strings.Contains(err.Error(), "Conflicting files: a.go") || !strings.Contains(err.Error(), "MERGE_CONFLICT") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeCommandRequiresSession(t *testing.T) {
	_, _, err := executeCLI(t, Deps{}, "merge")
	var usage usageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want usage error", err)
	}
}
