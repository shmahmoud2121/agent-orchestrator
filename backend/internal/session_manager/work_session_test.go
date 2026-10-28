package sessionmanager

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestWorkOriginalsPromptListsSourcesAndWriteDir(t *testing.T) {
	source := t.TempDir()
	results := t.TempDir()
	project := domain.ProjectRecord{
		ID:   "docs",
		Kind: domain.ProjectKindWork,
		Path: results,
		Config: domain.ProjectConfig{Work: &domain.WorkConfig{
			Sources:    []string{source},
			ResultsDir: results,
		}},
	}
	task := filepath.Join(results, "2026-10-27 summary")
	got := applyWorkOriginals("standing instructions", project, task)
	for _, want := range []string{
		"Never edit, move, rename or delete",
		"You may read them.",
		source,
		"Write everything you produce into " + task,
		"Never overwrite an existing file in the results folder",
		"` (2)`",
		"$AO_WORK_SCRATCH",
		"stop and ask the person",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q\n%s", want, got)
		}
	}
}

func TestBuildSystemPromptIncludesWorkOriginals(t *testing.T) {
	m, st, _, _ := newManager()
	source := t.TempDir()
	results := t.TempDir()
	st.projects["docs"] = domain.ProjectRecord{
		ID: "docs", Kind: domain.ProjectKindWork, Path: results,
		Config: domain.ProjectConfig{Work: &domain.WorkConfig{
			Sources: []string{source}, ResultsDir: results,
		}},
	}
	got, err := m.buildSystemPrompt(ctx, domain.KindOrchestrator, "docs", "docs-1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Never edit, move, rename or delete") || !strings.Contains(got, source) || !strings.Contains(got, results) {
		t.Fatalf("system prompt missing the work rule or paths:\n%s", got)
	}
}

func TestSpawn_WorkRejectsUnwatchedHarnessBeforeSessionRow(t *testing.T) {
	m, st, rt, _ := newManager()
	source := t.TempDir()
	results := t.TempDir()
	st.projects["docs"] = workProject("docs", source, results)

	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "docs",
		Kind:      domain.KindWorker,
		Harness:   domain.HarnessCursor,
		Prompt:    "summarise these files",
	})
	if !errors.Is(err, ErrWorkHarnessUnwatched) {
		t.Fatalf("err = %v, want ErrWorkHarnessUnwatched", err)
	}
	if len(st.sessions) != 0 {
		t.Fatalf("unwatched work spawn created %d session rows", len(st.sessions))
	}
	if rt.created != 0 {
		t.Fatalf("runtime creates = %d, want 0", rt.created)
	}
}

func TestSpawn_WorkGuardFailsClosed(t *testing.T) {
	m, st, rt, ws := newManager()
	m.dataDir = t.TempDir()
	source := t.TempDir()
	results := t.TempDir()
	st.projects["docs"] = workProject("docs", source, results)

	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "docs",
		Kind:      domain.KindWorker,
		Prompt:    "summarise these files",
	})
	if !errors.Is(err, ErrWorkGuardUnavailable) {
		t.Fatalf("err = %v, want ErrWorkGuardUnavailable", err)
	}
	if rt.created != 0 {
		t.Fatalf("runtime creates = %d, want 0", rt.created)
	}
	if ws.lastCfg.RepoPath != results {
		t.Fatalf("workspace repo path = %q, want results %q", ws.lastCfg.RepoPath, results)
	}
	if !strings.Contains(ws.lastCfg.TaskTitle, "summarise") {
		t.Fatalf("task title = %q, want the prompt", ws.lastCfg.TaskTitle)
	}
	for _, rec := range st.sessions {
		if !rec.IsTerminated {
			t.Fatalf("failed guard left a live session %#v", rec)
		}
	}
}

func workProject(id, source, results string) domain.ProjectRecord {
	cfg := testRoleAgents()
	cfg.Work = &domain.WorkConfig{Sources: []string{source}, ResultsDir: results}
	return domain.ProjectRecord{ID: id, Kind: domain.ProjectKindWork, Path: results, Config: cfg}
}
