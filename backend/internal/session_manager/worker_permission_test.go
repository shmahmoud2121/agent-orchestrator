package sessionmanager

import (
	"context"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestWorkerLaunchRemapsBypassForEveryHarness(t *testing.T) {
	st := newFakeStore()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: testRoleAgents()}
	agent := &recordingAgent{}
	m := New(Deps{
		Runtime:   &fakeRuntime{},
		Agents:    singleAgent{agent: agent},
		Workspace: &fakeWorkspace{},
		Store:     st,
		Messenger: &fakeMessenger{},
		Lifecycle: &fakeLCM{store: st},
		LookPath:  func(string) (string, error) { return "/bin/true", nil },
	})

	for _, harness := range domain.AllHarnesses {
		t.Run(string(harness), func(t *testing.T) {
			rec, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
				ProjectID:     "mer",
				Kind:          domain.KindWorker,
				Harness:       harness,
				Prompt:        "task",
				RequestedMode: domain.SessionModeTUI,
				AgentConfig:   ports.AgentConfig{Permissions: domain.PermissionModeBypassPermissions},
			})
			if err != nil {
				t.Fatalf("Spawn: %v", err)
			}
			if agent.lastLaunch.Permissions != domain.PermissionModeAcceptEdits || agent.lastLaunch.Config.Permissions != domain.PermissionModeAcceptEdits {
				t.Fatalf("launch permissions = %q / %q, want accept-edits", agent.lastLaunch.Permissions, agent.lastLaunch.Config.Permissions)
			}
			if rec.Metadata.Permissions != domain.PermissionModeBypassPermissions {
				t.Fatalf("stored pin = %q, want the requested bypass left unchanged", rec.Metadata.Permissions)
			}
		})
	}

	rec, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
		ProjectID:     "mer",
		Kind:          domain.KindOrchestrator,
		Harness:       domain.HarnessClaudeCode,
		RequestedMode: domain.SessionModeTUI,
		AgentConfig:   ports.AgentConfig{Permissions: domain.PermissionModeBypassPermissions},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.lastLaunch.Permissions != domain.PermissionModeBypassPermissions {
		t.Fatalf("orchestrator launch = %q, want bypass", agent.lastLaunch.Permissions)
	}
	if rec.Metadata.Permissions != domain.PermissionModeBypassPermissions {
		t.Fatalf("orchestrator pin = %q", rec.Metadata.Permissions)
	}
}
