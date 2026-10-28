//go:build !retired

package lifecycle

import (
	"context"
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

const testHerdrAvailable = false

func startTestHerdr(context.Context, string, *Manager, *slog.Logger) (interface{ Stop() }, error) {
	return nil, nil
}
func testHerdrPaneID(domain.SessionID, string) string { return "" }
func testHerdrSocketPath(string) string               { return "" }
