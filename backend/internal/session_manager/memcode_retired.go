//go:build retired

package sessionmanager

import (
	"context"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
)

func retiredMemcodeUsesCustomEndpoint(ctx context.Context, path string, env map[string]string) (bool, error) {
	return memcode.UsesCustomEndpoint(ctx, path, env)
}
