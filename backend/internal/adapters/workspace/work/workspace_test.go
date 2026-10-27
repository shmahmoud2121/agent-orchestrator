package work

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestCreateTaskFolderAddsSuffix(t *testing.T) {
	results := filepath.Join(t.TempDir(), "results")
	ws := New()
	cfg := ports.WorkspaceConfig{
		ProjectID: "letters",
		SessionID: "s1",
		Kind:      domain.KindWorker,
		RepoPath:  results,
		TaskTitle: "Summarise notes",
	}
	first, err := ws.Create(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now()
	base := when.Format("2006-01-02") + " Summarise notes"
	if filepath.Base(first.Path) != base {
		t.Fatalf("folder = %q, want %q", filepath.Base(first.Path), base)
	}
	second, err := ws.Create(context.Background(), ports.WorkspaceConfig{
		ProjectID: "letters",
		SessionID: "s2",
		Kind:      domain.KindWorker,
		RepoPath:  results,
		TaskTitle: "Summarise notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second.Path) != base+" (2)" {
		t.Fatalf("clash folder = %q", filepath.Base(second.Path))
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatal(err)
	}
	if err := ws.Destroy(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("destroy removed the task folder: %v", err)
	}
}
