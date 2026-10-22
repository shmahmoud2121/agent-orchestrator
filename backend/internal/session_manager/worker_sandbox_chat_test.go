package sessionmanager

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestSpawnRefusesClaudeChatWorkerWhenSandboxEnabled(t *testing.T) {
	st := newFakeStore()
	st.projects["mer"] = domain.ProjectRecord{
		ID: "mer", Path: t.TempDir(),
		Config: domain.ProjectConfig{
			Worker: domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		},
	}
	chat := &transitionChat{supportsChat: true}
	m := New(Deps{
		Runtime: &fakeRuntime{}, Agents: singleAgent{agent: claudecode.New()},
		Workspace: &fakeWorkspace{}, Store: st, Messenger: &fakeMessenger{}, Chat: chat,
		Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil },
	})
	_, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode,
		RequestedMode: domain.SessionModeChat, Prompt: "do it",
	})
	if !errors.Is(err, claudecode.ErrChatWorkerSandboxUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestInterfaceTransitionStatusHidesChatWhenWorkerSandboxOn(t *testing.T) {
	manager, store, _, _, _ := newTransitionManager(t, domain.SessionModeTUI)
	// Re-enable sandbox (newTransitionManager disables it for unrelated fixtures).
	store.projects["proj"] = domain.ProjectRecord{ID: "proj", Path: "/repo"}
	status, err := manager.InterfaceTransitionStatus(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Supported {
		t.Fatal("expected Chat switch unsupported while worker sandbox is on")
	}
	if status.ReasonCode != "CHAT_WORKER_SANDBOX_UNSUPPORTED" {
		t.Fatalf("reasonCode = %q reason=%q", status.ReasonCode, status.Reason)
	}
	if status.Reason != claudecode.ErrChatWorkerSandboxUnsupported.Error() {
		t.Fatalf("reason = %q", status.Reason)
	}
}
