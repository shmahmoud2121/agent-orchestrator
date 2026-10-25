//go:build !retired

package daemon

import (
	"log/slog"

	trackermulti "github.com/aoagents/agent-orchestrator/backend/internal/adapters/tracker/multi"
	"github.com/aoagents/agent-orchestrator/backend/internal/config"
)

func appendRetiredTrackers(named []trackermulti.NamedTracker, _ config.GitLabConfig, _ *slog.Logger) []trackermulti.NamedTracker {
	return named
}

func retiredTrackerNoToken(error) bool { return false }
