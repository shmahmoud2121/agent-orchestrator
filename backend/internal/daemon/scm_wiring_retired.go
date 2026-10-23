//go:build retired

package daemon

import (
	"errors"
	"log/slog"

	scmgitlab "github.com/aoagents/agent-orchestrator/backend/internal/adapters/scm/gitlab"
	scmmulti "github.com/aoagents/agent-orchestrator/backend/internal/adapters/scm/multi"
	"github.com/aoagents/agent-orchestrator/backend/internal/config"
)

func newGitLabSCMProvider(gitlabCfg config.GitLabConfig, logger *slog.Logger) (*scmgitlab.Provider, error) {
	tokens := scmgitlab.FallbackTokenSource{
		scmgitlab.EnvTokenSource{EnvVars: []string{"AO_GITLAB_TOKEN"}},
		&scmgitlab.GLabTokenSource{},
	}
	hostTokens := make(map[string]scmgitlab.TokenSource, len(gitlabCfg.HostTokens))
	for host, token := range gitlabCfg.HostTokens {
		hostTokens[host] = scmgitlab.StaticTokenSource(token)
	}
	return scmgitlab.NewProvider(scmgitlab.ProviderOptions{
		Token: tokens, SkipTokenPreflight: true, Logger: logger,
		AllowedHosts: gitlabCfg.AllowedHosts, HostTokens: hostTokens,
	})
}

func appendRetiredSCMProviders(named []scmmulti.NamedProvider, cfg config.GitLabConfig, logger *slog.Logger) []scmmulti.NamedProvider {
	if gl, err := newGitLabSCMProvider(cfg, logger); err == nil {
		return append(named, scmmulti.NamedProvider{Key: "gitlab", Provider: gl})
	} else {
		logSCMProviderDisabled(logger, "gitlab", err)
	}
	return named
}

func appendRetiredSCMMergers(named []scmmulti.NamedMerger, cfg config.GitLabConfig, logger *slog.Logger) []scmmulti.NamedMerger {
	if gl, err := newGitLabSCMProvider(cfg, logger); err == nil {
		return append(named, scmmulti.NamedMerger{Key: "gitlab", Merger: gl})
	}
	return named
}

func retiredSCMAuthError(err error) bool {
	return errors.Is(err, scmgitlab.ErrNoToken) || errors.Is(err, scmgitlab.ErrAuthFailed)
}
