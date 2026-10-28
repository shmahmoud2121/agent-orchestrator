//go:build !retired

package daemon

import (
	"context"
	"log/slog"
	"testing"

	scmmulti "github.com/aoagents/agent-orchestrator/backend/internal/adapters/scm/multi"
	"github.com/aoagents/agent-orchestrator/backend/internal/config"
	scmobserve "github.com/aoagents/agent-orchestrator/backend/internal/observe/scm"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestSCMWiring_GitHubProviderSatisfiesScopedIdentityResolver(t *testing.T) {
	gh, err := newGitHubSCMProvider(slog.Default())
	if err != nil {
		t.Skipf("github provider unavailable: %v", err)
	}
	multi := scmmulti.New(scmmulti.NamedProvider{Key: "github", Provider: gh})
	var _ ports.ScopedIdentityResolver = multi
	_ = multi.SCMCredentialsAvailable
	cfg := scmobserve.Config{ScopedIdentityResolver: multi}
	if cfg.ScopedIdentityResolver == nil {
		t.Fatal("ScopedIdentityResolver is nil")
	}
	_, _ = cfg.ScopedIdentityResolver.AuthenticatedIdentityForProvider(context.Background(), "github", "")
}

func TestSCMWiring_NewMultiSCMProviderReturnsScopedResolver(t *testing.T) {
	multi := newMultiSCMProvider(config.GitLabConfig{}, slog.Default())
	if multi == nil {
		t.Skip("no SCM provider available")
	}
	var _ ports.ScopedIdentityResolver = multi
}
