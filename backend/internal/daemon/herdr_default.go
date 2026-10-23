//go:build !retired

package daemon

import (
	"context"
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/lifecycle"
)

type herdrStopper interface { Stop() }

func startRetiredHerdr(context.Context, string, *lifecycle.Manager, *slog.Logger) (herdrStopper, error) {
	return nil, nil
}
