package sessionmanager

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestSpawnRequireSandbox(t *testing.T) {
	ctx := context.Background()
	off := false
	on := true
	m, st, _, _ := newManager()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		Orchestrator:  domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &on},
	}}

	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode, Prompt: "do it"}); err != nil {
		t.Fatalf("claude worker: %v", err)
	}
	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex, Prompt: "do it"}); !errors.Is(err, domain.ErrWorkerSandboxRequired) {
		t.Fatalf("codex worker on = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindOrchestrator, Harness: domain.HarnessCodex, Prompt: "do it"}); err != nil {
		t.Fatalf("manager: %v", err)
	}

	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessCodex},
		Orchestrator:  domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &off},
	}}
	if _, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex, Prompt: "do it"}); err != nil {
		t.Fatalf("codex worker off = %v", err)
	}
}

func TestRestoreRequireSandboxRefusesUnsandboxedWorker(t *testing.T) {
	ctx := context.Background()
	on := true
	m, st, _, _ := newManager()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessCodex},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &on},
	}}
	st.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		IsTerminated: true,
		Metadata:     domain.SessionMetadata{WorkspacePath: "/ws/mer-1", Branch: "ao/mer-1"},
	}
	if _, err := m.RestoreWithMode(ctx, "mer-1"); !errors.Is(err, domain.ErrWorkerSandboxRequired) {
		t.Fatalf("restore unsandboxed worker = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	if err := refuseUnsandboxedWorker(domain.KindWorker, domain.HarnessCodex, domain.WorkerSandboxConfig{}, true); err == nil || err.Error() != domain.ErrWorkerSandboxRequired.Error()+": "+domain.WorkerSandboxRequiredRestoreMessage {
		t.Fatalf("restore wording = %v", err)
	}
}

func TestSpawnRequireSandboxWindowsUnavailable(t *testing.T) {
	restore := claudecode.OverrideSandboxAvailabilityForTest(func() error {
		return claudecode.WindowsSandboxUnavailableError()
	})
	t.Cleanup(restore)

	ctx := context.Background()
	on := true
	m, st, _, _ := newManager()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		Orchestrator:  domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &on},
	}}

	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode, Prompt: "do it"})
	if !errors.Is(err, domain.ErrWorkerSandboxRequired) {
		t.Fatalf("claude worker on windows-style probe = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	if !strings.Contains(err.Error(), domain.WorkerSandboxRequiredWindowsMessage) {
		t.Fatalf("windows refusal = %v", err)
	}
}

func TestSpawnRequireSandboxHarnessMessageWhenOSAvailable(t *testing.T) {
	ctx := context.Background()
	on := true
	m, st, _, _ := newManager()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessCodex},
		Orchestrator:  domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &on},
	}}
	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex, Prompt: "do it"})
	if !errors.Is(err, domain.ErrWorkerSandboxRequired) {
		t.Fatalf("codex worker = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	if !strings.Contains(err.Error(), domain.WorkerSandboxRequiredMessage) {
		t.Fatalf("harness refusal = %v, want the original Claude Code wording", err)
	}
	if strings.Contains(err.Error(), domain.WorkerSandboxRequiredWindowsMessage) {
		t.Fatal("a host that can sandbox must not use the Windows wording")
	}
}

func TestRestoreRequireSandboxOSUnavailable(t *testing.T) {
	restore := claudecode.OverrideSandboxAvailabilityForTest(func() error {
		return claudecode.WindowsSandboxUnavailableError()
	})
	t.Cleanup(restore)

	ctx := context.Background()
	on := true
	m, st, _, _ := newManager()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker:        domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &on},
	}}
	st.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
		IsTerminated: true,
		Metadata:     domain.SessionMetadata{WorkspacePath: "/ws/mer-1", Branch: "ao/mer-1"},
	}
	_, err := m.RestoreWithMode(ctx, "mer-1")
	if !errors.Is(err, domain.ErrWorkerSandboxRequired) {
		t.Fatalf("restore = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	want := domain.WorkerSandboxRequiredRestoreLead + " " + domain.WorkerSandboxRequiredWindowsMessage
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("restore wording = %v, want %q", err, want)
	}
}
