//go:build !retired

package daemon

import (
	"log/slog"

	scmmulti "github.com/aoagents/agent-orchestrator/backend/internal/adapters/scm/multi"
	"github.com/aoagents/agent-orchestrator/backend/internal/config"
)

func appendRetiredSCMProviders(named []scmmulti.NamedProvider, _ config.GitLabConfig, _ *slog.Logger) []scmmulti.NamedProvider {
	return named
}

func appendRetiredSCMMergers(named []scmmulti.NamedMerger, _ config.GitLabConfig, _ *slog.Logger) []scmmulti.NamedMerger {
	return named
}

func retiredSCMAuthError(error) bool { return false }
