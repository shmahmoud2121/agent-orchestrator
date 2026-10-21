package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/localmerge"
)

type fakeLocalMerge struct {
	got localmerge.Request
	res localmerge.Result
	err error
}

func (f *fakeLocalMerge) Merge(_ context.Context, req localmerge.Request) (localmerge.Result, error) {
	f.got = req
	return f.res, f.err
}

func serveLocalMerge(t *testing.T, svc LocalMergeService, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	(&LocalMergeController{Svc: svc}).Register(r)
	req := httptest.NewRequest(http.MethodPost, "/sessions/w1/merge", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLocalMergeConflictIs409WithFiles(t *testing.T) {
	svc := &fakeLocalMerge{err: apierr.Conflict(localmerge.CodeConflict, "Merging conflicts; nothing landed.", map[string]any{
		"conflictingFiles": []string{"a.go", "b.go"}, "instruction": "In worker w1's own worktree, run `git merge main`",
	})}
	rec := serveLocalMerge(t, svc, `{"dryRun":true,"callerSessionId":"p1-orchestrator"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "conflict" || body.Code != "MERGE_CONFLICT" {
		t.Fatalf("envelope = %+v", body)
	}
	files, _ := body.Details["conflictingFiles"].([]any)
	if len(files) != 2 || files[0] != "a.go" || !strings.Contains(body.Details["instruction"].(string), "git merge main") {
		t.Fatalf("details = %+v", body.Details)
	}
	if svc.got.SessionID != "w1" || !svc.got.DryRun || svc.got.CallerSessionID != "p1-orchestrator" {
		t.Fatalf("request = %+v", svc.got)
	}
}

func TestLocalMergeWorkerCallerIs403(t *testing.T) {
	svc := &fakeLocalMerge{err: apierr.Forbidden(localmerge.CodeForbidden, "Only the orchestrator can merge.")}
	rec := serveLocalMerge(t, svc, `{"callerSessionId":"w2"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MERGE_FORBIDDEN") {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestLocalMergeSuccessShape(t *testing.T) {
	svc := &fakeLocalMerge{res: localmerge.Result{SessionID: "w1", Branch: "ao/w1/root", BaseBranch: "main", MergeCommit: "abc", CommitCount: 2, PushAttempted: true, PushError: "rejected"}}
	rec := serveLocalMerge(t, svc, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var res MergeSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.MergeCommit != "abc" || res.CommitCount != 2 || res.Pushed || res.PushError != "rejected" || res.ChangedFiles == nil || res.Overlaps == nil || res.Notified == nil {
		t.Fatalf("response = %+v", res)
	}
	if svc.got.DryRun || svc.got.CallerSessionID != "" {
		t.Fatalf("empty body should be a real merge by a person: %+v", svc.got)
	}
}

func TestLocalMergeRejectsUnknownFieldsAndNilService(t *testing.T) {
	if rec := serveLocalMerge(t, &fakeLocalMerge{}, `{"dry_run":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("misspelled field status = %d", rec.Code)
	}
	if rec := serveLocalMerge(t, nil, ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("nil service status = %d", rec.Code)
	}
}
