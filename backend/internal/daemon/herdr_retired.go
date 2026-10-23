//go:build retired

package daemon

import (
	"context"
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/fx/herdr"
	"github.com/aoagents/agent-orchestrator/backend/internal/lifecycle"
)

type herdrStopper interface { Stop() }

func startRetiredHerdr(ctx context.Context, dir string, lcm *lifecycle.Manager, logger *slog.Logger) (herdrStopper, error) {
	server, err := herdr.Start(ctx, dir, lcm, logger)
	if server == nil { return nil, err }
	return server, err
}
