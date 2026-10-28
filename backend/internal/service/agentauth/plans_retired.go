//go:build retired

package agentauth

import (
	"context"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimi"
)

const retiredAuthPlansEnabled = true

func prepareRetiredKimiWorkspace(ctx context.Context, path string) error {
	return kimi.EnsureWorkspaceTrusted(ctx, path)
}
func prepareRetiredCopilotWorkspace(ctx context.Context, path string) error {
	return copilot.EnsureWorkspaceTrusted(ctx, path)
}
