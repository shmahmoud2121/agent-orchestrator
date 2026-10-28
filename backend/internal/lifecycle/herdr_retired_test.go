//go:build retired

package lifecycle

import (
	"context"
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/fx/herdr"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

const testHerdrAvailable = true

func startTestHerdr(ctx context.Context, dir string, m *Manager, logger *slog.Logger) (interface{ Stop() }, error) {
	server, err := herdr.Start(ctx, dir, m, logger)
	if server == nil {
		return nil, err
	}
	return server, err
}
func testHerdrPaneID(session domain.SessionID, launch string) string {
	return herdr.PaneID(session, launch)
}
func testHerdrSocketPath(dir string) string { return herdr.SocketPath(dir) }
