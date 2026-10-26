package chat

import (
	"context"
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
	if !errors.Is(err, ports.ErrWorkerBypassForbidden) || err.Error() != "Workers can't bypass permissions." {
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
	); !errors.Is(err, ports.ErrWorkerBypassForbidden) {
		t.Fatalf("unsandboxed worker bypass = %v, want refusal", err)
	}
	if err := svc.refuseSandboxedBypassConfig(
		domain.SessionConversationOwner("orch"),
		"mode", ports.ChatConfigOptionValue{Select: "bypassPermissions"},
	); err != nil {
		t.Fatalf("orchestrator should allow bypass: %v", err)
	}
}

func TestSetTurnSettingsRefusesWorkerBypass(t *testing.T) {
	owner := domain.SessionConversationOwner("worker")
	svc := &Service{startConfigs: map[domain.ConversationOwner]StartConfig{
		owner: {Kind: domain.KindWorker, Harness: domain.HarnessCodex},
	}}
	_, err := svc.setTurnSettings(context.Background(), &Controller{}, owner, domain.ConversationSettings{
		ApprovalMode: domain.PermissionModeBypassPermissions,
	})
	if !errors.Is(err, ports.ErrWorkerBypassForbidden) || err.Error() != "Workers can't bypass permissions." {
		t.Fatalf("worker turn settings = %v", err)
	}

	_, err = svc.setTurnSettings(context.Background(), &Controller{}, domain.ReviewConversationOwner("rev"), domain.ConversationSettings{
		ApprovalMode: domain.PermissionModeBypassPermissions,
	})
	if !errors.Is(err, ErrReviewerPermissionsFixed) {
		t.Fatalf("reviewer turn settings = %v, want fixed permissions", err)
	}
}

func TestEffectiveChatPermissionOnlyRemapsClaude(t *testing.T) {
	enabled := true
	sandbox := domain.WorkerSandboxConfig{Enabled: &enabled}
	claude := StartConfig{Harness: domain.HarnessClaudeCode, Kind: domain.KindWorker,
		WorkerSandbox: sandbox, Permissions: ports.PermissionModeBypassPermissions}
	if got := effectiveChatPermission(claude); got != ports.PermissionModeAcceptEdits {
		t.Fatalf("sandboxed Claude worker = %q, want acceptEdits", got)
	}
	for _, harness := range domain.AllHarnesses {
		worker := StartConfig{Harness: harness, Kind: domain.KindWorker, Permissions: ports.PermissionModeBypassPermissions}
		if got := effectiveChatPermission(worker); got != ports.PermissionModeAcceptEdits {
			t.Fatalf("%s worker = %q, want acceptEdits", harness, got)
		}
		orchestrator := worker
		orchestrator.Kind = domain.KindOrchestrator
		if got := effectiveChatPermission(orchestrator); got != ports.PermissionModeBypassPermissions {
			t.Fatalf("%s orchestrator = %q, want bypassPermissions", harness, got)
		}
	}
}
