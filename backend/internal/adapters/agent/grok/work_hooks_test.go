package grok

import (
	"context"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestWorkHooksReadyNeedsBeforeToolHook(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	p := New()
	if ready, err := p.WorkHooksReady(ctx, ws); err != nil || ready {
		t.Fatalf("empty workspace ready = %v, %v; want false", ready, err)
	}
	if err := p.GetAgentHooks(ctx, ports.WorkspaceHookConfig{WorkspacePath: ws}); err != nil {
		t.Fatal(err)
	}
	if ready, err := p.WorkHooksReady(ctx, ws); err != nil || !ready {
		t.Fatalf("installed ready = %v, %v; want true", ready, err)
	}
	if err := p.UninstallHooks(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if ready, _ := p.WorkHooksReady(ctx, ws); ready {
		t.Fatal("uninstalled workspace still reports the guard hook")
	}
}
