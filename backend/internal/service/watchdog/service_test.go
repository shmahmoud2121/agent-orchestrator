package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

type fakeStore struct {
	session domain.SessionRecord
	ok      bool
	project domain.ProjectRecord
	hasProj bool
}

func (f fakeStore) GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error) {
	return f.session, f.ok, nil
}

func (f fakeStore) GetProject(context.Context, string) (domain.ProjectRecord, bool, error) {
	return f.project, f.hasProj, nil
}

func TestDecideSkipsStandaloneSession(t *testing.T) {
	svc := &Service{Store: fakeStore{ok: true, session: domain.SessionRecord{ID: "solo"}}}
	got, err := svc.Decide(context.Background(), "solo", "Read", json.RawMessage(`{"file_path":".env"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "skip" {
		t.Fatalf("action = %q, want skip", got.Action)
	}
}

func TestDecideBlocksProtectedRead(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "src", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		DataDir: filepath.Join(root, "data"),
		HomeDir: filepath.Join(root, "home"),
		Store: fakeStore{
			ok: true,
			session: domain.SessionRecord{
				ID: "proj-1", ProjectID: "proj", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
				Metadata: domain.SessionMetadata{WorkspacePath: work},
			},
			hasProj: true,
			project: domain.ProjectRecord{
				ID: "proj", Path: project,
				Config: domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{ProtectedPaths: []string{"notes.txt"}}},
			},
		},
	}
	blocked, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":".env"}`))
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Action != "deny" || blocked.Path != ".env" || blocked.Reason == "" {
		t.Fatalf("env read = %+v", blocked)
	}
	allowed, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":"src/main.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Action != "allow" {
		t.Fatalf("source read = %+v, want allow", allowed)
	}
	added, err := svc.Decide(context.Background(), "proj-1", "Read", json.RawMessage(`{"file_path":`+quote(filepath.Join(project, "notes.txt"))+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if added.Action != "deny" {
		t.Fatalf("added file = %+v, want deny", added)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
