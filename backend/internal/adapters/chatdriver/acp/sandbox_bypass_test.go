package acp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func claudeLikeSessionMode(permission ports.PermissionMode) string {
	switch ports.NormalizePermissionMode(permission) {
	case ports.PermissionModeAcceptEdits:
		return "acceptEdits"
	case ports.PermissionModeBypassPermissions:
		return "bypassPermissions"
	case ports.PermissionModeAuto:
		return "auto"
	default:
		return ""
	}
}

func TestSandboxedWorkerStartSendsAcceptEditsMode(t *testing.T) {
	agent := &fakeAgent{}
	driver := New(Config{
		Harness:     domain.HarnessClaudeCode,
		Probe:       func(context.Context) error { return nil },
		Launch:      func(context.Context, LaunchConfig) (Launch, error) { return Launch{Command: "fake"}, nil },
		SessionMode: claudeLikeSessionMode,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	driver.useTestProcess(fakeSpawn(agent))

	enabled := true
	conv, err := driver.Start(context.Background(), ports.ChatStartConfig{
		WorkspacePath: t.TempDir(),
		// Remapped by service/checkpointDriver before Start; assert set_mode sees it.
		Permissions:   ports.PermissionModeAcceptEdits,
		Kind:          domain.KindWorker,
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer conv.Close()
	if agent.mode != "acceptEdits" {
		t.Fatalf("session/set_mode = %q, want acceptEdits", agent.mode)
	}
}

func TestUnsandboxedWorkerStartKeepsBypassMode(t *testing.T) {
	agent := &fakeAgent{}
	driver := New(Config{
		Harness:     domain.HarnessClaudeCode,
		Probe:       func(context.Context) error { return nil },
		Launch:      func(context.Context, LaunchConfig) (Launch, error) { return Launch{Command: "fake"}, nil },
		SessionMode: claudeLikeSessionMode,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	driver.useTestProcess(fakeSpawn(agent))

	disabled := false
	conv, err := driver.Start(context.Background(), ports.ChatStartConfig{
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeBypassPermissions,
		Kind:          domain.KindWorker,
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &disabled},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer conv.Close()
	if agent.mode != "bypassPermissions" {
		t.Fatalf("session/set_mode = %q, want bypassPermissions", agent.mode)
	}
}

func TestSandboxedWorkerRefusesMidSessionBypassSwitch(t *testing.T) {
	catalog := []acpsdk.SessionConfigOption{
		selectConfigOption("mode", "Mode", "mode", "acceptEdits",
			"default", "acceptEdits", "auto", "bypassPermissions"),
	}
	agent := &fakeAgent{newConfig: catalog, setConfig: catalog}
	driver := New(Config{
		Harness:     domain.HarnessClaudeCode,
		Probe:       func(context.Context) error { return nil },
		Launch:      func(context.Context, LaunchConfig) (Launch, error) { return Launch{Command: "fake"}, nil },
		SessionMode: claudeLikeSessionMode,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	driver.useTestProcess(fakeSpawn(agent))

	enabled := true
	opened, err := driver.Start(context.Background(), ports.ChatStartConfig{
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeAcceptEdits,
		Kind:          domain.KindWorker,
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer opened.Close()
	if ready := nextEvent(t, opened.Events()); ready.Kind != ports.ChatEventControllerState {
		t.Fatalf("first event = %#v", ready)
	}

	configurer := opened.(ports.ChatConfigOptionController)
	options, err := configurer.ListConfigOptions(context.Background())
	if err != nil {
		t.Fatalf("ListConfigOptions: %v", err)
	}
	for _, option := range options {
		if option.ID != "mode" {
			continue
		}
		for _, choice := range option.Choices {
			if choice.Value == "bypassPermissions" {
				t.Fatal("bypassPermissions still offered in picker for sandboxed worker")
			}
		}
	}

	_, err = configurer.SetConfigOption(context.Background(), "mode", ports.ChatConfigOptionValue{Select: "bypassPermissions"})
	if !errors.Is(err, ports.ErrChatSandboxBypassDisabled) {
		t.Fatalf("SetConfigOption bypass error = %v, want ErrChatSandboxBypassDisabled", err)
	}

	conv := opened.(*conversation)
	if err := conv.applyTurnSettings(context.Background(), ports.ChatTurnSettings{
		Approval: ports.PermissionModeBypassPermissions,
	}); !errors.Is(err, ports.ErrChatSandboxBypassDisabled) {
		t.Fatalf("applyTurnSettings bypass error = %v, want ErrChatSandboxBypassDisabled", err)
	}
}
