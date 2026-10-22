package sessionmanager

import (
	"context"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestSpawnClaudeChatWorkerPassesWorkerSandbox(t *testing.T) {
	launcher := &recordingLauncher{}
	m, st, _ := newChatManager(launcher)
	wsPath := t.TempDir()
	m.workspace = &fakeWorkspace{path: wsPath}
	m.agents = singleAgent{agent: claudecode.New()}
	project := st.projects[string(chatTestProject)]
	project.Path = t.TempDir()
	project.Config.Worker = domain.RoleOverride{Harness: domain.HarnessClaudeCode}
	project.Config.WorkerSandbox = domain.WorkerSandboxConfig{
		AllowedDomains: []string{"registry.npmjs.org"},
	}
	st.projects[string(chatTestProject)] = project

	_, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
		ProjectID: chatTestProject, Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
		RequestedMode: domain.SessionModeChat, Prompt: "do it",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if len(launcher.started) != 1 {
		t.Fatalf("StartChat calls = %d, want 1", len(launcher.started))
	}
	got := launcher.started[0].WorkerSandbox
	if !got.IsEnabled() {
		t.Fatal("WorkerSandbox should be enabled by default")
	}
	if len(got.AllowedDomains) != 1 || got.AllowedDomains[0] != "registry.npmjs.org" {
		t.Fatalf("AllowedDomains = %v", got.AllowedDomains)
	}
	if launcher.started[0].Kind != domain.KindWorker {
		t.Fatalf("Kind = %q", launcher.started[0].Kind)
	}
}

func TestInterfaceTransitionStatusAllowsChatWhenWorkerSandboxOn(t *testing.T) {
	manager, store, _, _, _ := newTransitionManager(t, domain.SessionModeTUI)
	// Re-enable sandbox (newTransitionManager disables it for unrelated fixtures).
	store.projects["proj"] = domain.ProjectRecord{ID: "proj", Path: "/repo"}
	status, err := manager.InterfaceTransitionStatus(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Supported {
		t.Fatalf("expected Chat switch supported with sandbox on; reason=%q code=%q", status.Reason, status.ReasonCode)
	}
}
