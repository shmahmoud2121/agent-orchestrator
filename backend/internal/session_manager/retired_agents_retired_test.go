//go:build retired

package sessionmanager

import (
	"context"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/agy"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/amp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/fx"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/gemini"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/pi"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

const retiredTestAgentsAvailable = true

func testAmpAgent() ports.Agent         { return amp.New() }
func testFXAgent() ports.Agent          { return fx.New() }
func testCommandCodeAgent() ports.Agent { return commandcode.New() }
func testAgyAgent() ports.Agent         { return agy.New() }
func testMemcodeAgent() ports.Agent     { return memcode.New() }
func testMemcodeHooks(ctx context.Context, cfg ports.WorkspaceHookConfig) error {
	return memcode.New().GetAgentHooks(ctx, cfg)
}
func testMemcodeAfterStart(ctx context.Context, cfg ports.LaunchConfig) error {
	_, err := memcode.New().AfterStartCommand(ctx, cfg)
	return err
}
func testMemcodeReadinessAgent() interface {
	ports.Agent
	ports.AgentPromptReadinessProvider
} {
	return memcode.New()
}
func retiredTerminalRefreshAgents() map[domain.AgentHarness]ports.Agent {
	return map[domain.AgentHarness]ports.Agent{domain.HarnessPi: pi.New(), domain.HarnessGemini: gemini.New()}
}
