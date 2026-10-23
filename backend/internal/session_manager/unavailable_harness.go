package sessionmanager

import (
	"fmt"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func unavailableHarnessError(harness domain.AgentHarness) error {
	if harness.IsKnown() {
		return fmt.Errorf("%w: %q: this agent is retired in this build", ErrUnknownHarness, harness)
	}
	return fmt.Errorf("%w: %q", ErrUnknownHarness, harness)
}
