package chat

import (
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestRefuseSandboxedBypassConfig(t *testing.T) {
	enabled := true
	disabled := false
	svc := &Service{startConfigs: map[domain.ConversationOwner]StartConfig{
		domain.SessionConversationOwner("sandboxed"): {
			Harness: domain.HarnessClaudeCode, Kind: domain.KindWorker,
			WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled},
		},
		domain.SessionConversationOwner("off"): {
			Harness: domain.HarnessClaudeCode, Kind: domain.KindWorker,
			WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &disabled},
		},
		domain.SessionConversationOwner("orch"): {
			Harness: domain.HarnessClaudeCode, Kind: domain.KindOrchestrator,
			WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled},
		},
	}}

	err := svc.refuseSandboxedBypassConfig(
		domain.SessionConversationOwner("sandboxed"),
		"mode", ports.ChatConfigOptionValue{Select: "bypassPermissions"},
	)
	if !errors.Is(err, ports.ErrChatSandboxBypassDisabled) {
		t.Fatalf("sandboxed worker: %v", err)
	}
	if err := svc.refuseSandboxedBypassConfig(
		domain.SessionConversationOwner("sandboxed"),
		"mode", ports.ChatConfigOptionValue{Select: "auto"},
	); err != nil {
		t.Fatalf("auto should remain allowed: %v", err)
	}
	if err := svc.refuseSandboxedBypassConfig(
		domain.SessionConversationOwner("off"),
		"mode", ports.ChatConfigOptionValue{Select: "bypassPermissions"},
	); err != nil {
		t.Fatalf("sandbox off should allow bypass: %v", err)
	}
	if err := svc.refuseSandboxedBypassConfig(
		domain.SessionConversationOwner("orch"),
		"mode", ports.ChatConfigOptionValue{Select: "bypassPermissions"},
	); err != nil {
		t.Fatalf("orchestrator should allow bypass: %v", err)
	}
}
