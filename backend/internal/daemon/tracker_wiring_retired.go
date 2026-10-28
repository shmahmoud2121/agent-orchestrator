//go:build retired

package daemon

import (
	"errors"
	"log/slog"
	"strings"

	scmgitlab "github.com/aoagents/agent-orchestrator/backend/internal/adapters/scm/gitlab"
	trackergitlab "github.com/aoagents/agent-orchestrator/backend/internal/adapters/tracker/gitlab"
	trackermulti "github.com/aoagents/agent-orchestrator/backend/internal/adapters/tracker/multi"
	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func newGitLabTracker(gitlabCfg config.GitLabConfig) (ports.Tracker, error) {
	hostTokens := make(map[string]scmgitlab.TokenSource, len(gitlabCfg.HostTokens))
	for host, token := range gitlabCfg.HostTokens {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			hostTokens[host] = scmgitlab.StaticTokenSource(token)
		}
	}
	return trackergitlab.New(trackergitlab.Options{
		Token: trackergitlab.DefaultTokenSource(), AllowedHosts: gitlabCfg.AllowedHosts, HostTokens: hostTokens,
	})
}

func appendRetiredTrackers(named []trackermulti.NamedTracker, cfg config.GitLabConfig, logger *slog.Logger) []trackermulti.NamedTracker {
	if t, err := newGitLabTracker(cfg); err != nil {
		logTrackerDisabled(logger, "gitlab", err)
	} else {
		named = append(named, trackermulti.NamedTracker{Key: "gitlab", Tracker: t})
	}
	return named
}

func retiredTrackerNoToken(err error) bool { return errors.Is(err, trackergitlab.ErrNoToken) }
