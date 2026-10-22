package claudeacp

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type capturePermissionDriver struct {
	ports.ChatDriver
	startPerms  ports.PermissionMode
	resumePerms ports.PermissionMode
}

func (d *capturePermissionDriver) Start(_ context.Context, cfg ports.ChatStartConfig) (ports.ChatConversation, error) {
	d.startPerms = cfg.Permissions
	return nil, errors.New("capture only")
}

func (d *capturePermissionDriver) Resume(_ context.Context, cfg ports.ChatResumeConfig) (ports.ChatConversation, error) {
	d.resumePerms = cfg.Permissions
	return nil, errors.New("capture only")
}

func TestCheckpointDriverRemapsBypassForSandboxedWorker(t *testing.T) {
	inner := &capturePermissionDriver{}
	driver := &checkpointDriver{ChatDriver: inner, plugin: stubClaudePlugin{}}
	enabled := true
	sandbox := domain.WorkerSandboxConfig{Enabled: &enabled}

	_, _ = driver.Start(context.Background(), ports.ChatStartConfig{
		Permissions: ports.PermissionModeBypassPermissions,
		Kind:        domain.KindWorker, WorkerSandbox: sandbox,
	})
	if inner.startPerms != ports.PermissionModeAcceptEdits {
		t.Fatalf("Start Permissions = %q, want acceptEdits", inner.startPerms)
	}

	_, _ = driver.Resume(context.Background(), ports.ChatResumeConfig{
		Permissions: ports.PermissionModeBypassPermissions,
		Kind:        domain.KindWorker, WorkerSandbox: sandbox,
	})
	if inner.resumePerms != ports.PermissionModeAcceptEdits {
		t.Fatalf("Resume Permissions = %q, want acceptEdits", inner.resumePerms)
	}
}

func TestCheckpointDriverKeepsBypassWhenSandboxOff(t *testing.T) {
	inner := &capturePermissionDriver{}
	driver := &checkpointDriver{ChatDriver: inner, plugin: stubClaudePlugin{}}
	disabled := false

	_, _ = driver.Start(context.Background(), ports.ChatStartConfig{
		Permissions:   ports.PermissionModeBypassPermissions,
		Kind:          domain.KindWorker,
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &disabled},
	})
	if inner.startPerms != ports.PermissionModeBypassPermissions {
		t.Fatalf("Start Permissions = %q, want bypassPermissions", inner.startPerms)
	}
}

type stubClaudePlugin struct{}

func (stubClaudePlugin) ResolveBinary(context.Context) (string, error) { return "/bin/claude", nil }
