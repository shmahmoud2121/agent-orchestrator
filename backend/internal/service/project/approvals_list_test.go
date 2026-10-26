package project_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/project"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

func TestListApprovalsScopesPendingChatRequests(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	seedProject(t, store, "approvals")
	seedProject(t, store, "other")
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "src", "main.go")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	worker := chatSession(t, store, "approvals", domain.KindWorker, domain.SessionModeChat, "Ada", workspace, false, now)
	orchestrator := chatSession(t, store, "approvals", domain.KindOrchestrator, domain.SessionModeChat, "Manager", workspace, false, now)
	tui := chatSession(t, store, "approvals", domain.KindWorker, domain.SessionModeTUI, "Terminal", workspace, false, now)
	other := chatSession(t, store, "other", domain.KindWorker, domain.SessionModeChat, "Elsewhere", workspace, false, now)
	terminated := chatSession(t, store, "approvals", domain.KindWorker, domain.SessionModeChat, "Done", workspace, true, now)

	workerConv := mustConversation(t, store, "conv-worker", domain.ConversationScopeSession, "approvals", worker.ID, now)
	orchConv := mustConversation(t, store, "conv-orch", domain.ConversationScopeProject, "approvals", orchestrator.ID, now)
	tuiConv := mustConversation(t, store, "conv-tui", domain.ConversationScopeSession, "approvals", tui.ID, now)
	otherConv := mustConversation(t, store, "conv-other", domain.ConversationScopeSession, "other", other.ID, now)
	doneConv := mustConversation(t, store, "conv-done", domain.ConversationScopeSession, "approvals", terminated.ID, now)

	mustApproval(t, store, workerConv, "inside", "Apply file changes", fileDetail(inside, "M", decisions()), now)
	mustApproval(t, store, workerConv, "shell", "Run git status", map[string]any{
		"subjectKind": "command",
		"command":     "git status",
		"decisions":   decisions(),
	}, now.Add(time.Second))
	mustApproval(t, store, workerConv, "outside", "Apply file changes", fileDetail(filepath.Join(t.TempDir(), "secret.go"), "M", decisions()), now.Add(2*time.Second))
	mustApproval(t, store, workerConv, "resolved", "Apply file changes", fileDetail(inside, "M", decisions()), now.Add(3*time.Second))
	if err := store.UpsertActivity(ctx, workerConv, "", domain.ConversationActivity{
		ID: "resolved", Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusResolved,
		Summary: "done", RequestID: "resolved", ProviderItemID: "resolved",
		Detail: mustJSON(t, fileDetail(inside, "M", decisions())),
	}, now.Add(4*time.Second)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	mustApproval(t, store, orchConv, "manager", "Apply file changes", fileDetail(inside, "M", decisions()), now.Add(4*time.Second))
	mustApproval(t, store, tuiConv, "tui", "Apply file changes", fileDetail(inside, "M", decisions()), now)
	mustApproval(t, store, otherConv, "other", "Apply file changes", fileDetail(inside, "M", decisions()), now)
	mustApproval(t, store, doneConv, "terminated", "Apply file changes", fileDetail(inside, "M", decisions()), now)

	got, err := project.New(store).ListApprovals(ctx, "approvals")
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	byRequest := map[string]project.Approval{}
	for _, item := range got {
		byRequest[item.RequestID] = item
	}
	for _, absent := range []string{"resolved", "tui", "other", "terminated"} {
		if _, ok := byRequest[absent]; ok {
			t.Fatalf("request %s should be absent, got %#v", absent, byRequest[absent])
		}
	}
	if _, ok := byRequest["inside"]; !ok {
		t.Fatalf("missing inside edit: %#v", got)
	}
	if _, ok := byRequest["manager"]; !ok {
		t.Fatalf("manager request missing: %#v", got)
	}
	insideItem := byRequest["inside"]
	if !insideItem.BulkSafe || insideItem.DisplayName != "Ada" || insideItem.SessionKind != domain.KindWorker {
		t.Fatalf("inside = %#v", insideItem)
	}
	if insideItem.Summary != inside {
		t.Fatalf("summary = %q, want path %q", insideItem.Summary, inside)
	}
	if len(insideItem.Options) != 2 || insideItem.Options[0].Kind != "allow_once" || insideItem.Options[1].Kind != "reject_always" {
		t.Fatalf("options = %#v", insideItem.Options)
	}
	if byRequest["shell"].BulkSafe || byRequest["outside"].BulkSafe {
		t.Fatalf("unsafe items classified safe: shell=%v outside=%v", byRequest["shell"].BulkSafe, byRequest["outside"].BulkSafe)
	}
	if byRequest["shell"].Summary != "git status" {
		t.Fatalf("shell summary = %q", byRequest["shell"].Summary)
	}
}

func seedProject(t *testing.T, store *sqlite.Store, id string) {
	t.Helper()
	if err := store.UpsertProject(context.Background(), domain.ProjectRecord{
		ID: id, Path: filepath.Join(t.TempDir(), id), RegisteredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

func chatSession(t *testing.T, store *sqlite.Store, project string, kind domain.SessionKind, mode domain.SessionMode, name, workspace string, terminated bool, now time.Time) domain.SessionRecord {
	t.Helper()
	rec := domain.SessionRecord{
		ProjectID:    domain.ProjectID(project),
		Kind:         kind,
		Harness:      domain.HarnessClaudeCode,
		Mode:         mode,
		DisplayName:  name,
		IsTerminated: terminated,
		Activity:     domain.Activity{State: domain.ActivityActive, LastActivityAt: now},
		Metadata:     domain.SessionMetadata{WorkspacePath: workspace},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	created, err := store.CreateSession(context.Background(), rec)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return created
}

func mustConversation(t *testing.T, store *sqlite.Store, id string, scope domain.ConversationScope, project string, session domain.SessionID, now time.Time) string {
	t.Helper()
	conv, err := store.CreateConversation(context.Background(), id, scope, domain.ProjectID(project), session, now)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv.ID
}

func mustApproval(t *testing.T, store *sqlite.Store, conversation, requestID, summary string, detail map[string]any, now time.Time) {
	t.Helper()
	err := store.UpsertActivity(context.Background(), conversation, "", domain.ConversationActivity{
		ID: requestID, Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending,
		Summary: summary, RequestID: requestID, ProviderItemID: requestID,
		Detail: mustJSON(t, detail),
	}, now)
	if err != nil {
		t.Fatalf("approval %s: %v", requestID, err)
	}
}

func fileDetail(path, status string, options []any) map[string]any {
	return map[string]any{
		"subjectKind": "file_change",
		"files":       []any{map[string]any{"path": path, "status": status}},
		"decisions":   options,
	}
}

func decisions() []any {
	return []any{
		map[string]any{"id": "allow", "kind": "allow_once", "label": "Always allow"},
		map[string]any{"id": "reject", "kind": "reject_always", "label": "Allow once"},
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return raw
}
