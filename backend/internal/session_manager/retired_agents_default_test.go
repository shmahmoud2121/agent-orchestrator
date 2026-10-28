//go:build !retired

package sessionmanager

import (
	"context"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

const retiredTestAgentsAvailable = false

func testAmpAgent() ports.Agent                                         { return nil }
func testFXAgent() ports.Agent                                          { return nil }
func testCommandCodeAgent() ports.Agent                                 { return nil }
func testAgyAgent() ports.Agent                                         { return nil }
func testMemcodeAgent() ports.Agent                                     { return nil }
func testMemcodeHooks(context.Context, ports.WorkspaceHookConfig) error { return nil }
func testMemcodeAfterStart(context.Context, ports.LaunchConfig) error   { return nil }
func testMemcodeReadinessAgent() interface {
	ports.Agent
	ports.AgentPromptReadinessProvider
} {
	return nil
}
func retiredTerminalRefreshAgents() map[domain.AgentHarness]ports.Agent { return nil }
