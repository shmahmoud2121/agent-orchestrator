package sessionmanager

import (
	"fmt"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func unavailableHarnessError(harness domain.AgentHarness) error {
	if harness.IsKnown() && !activeHarness(harness) {
		return fmt.Errorf("%w: %q: this agent is retired in this build", ErrUnknownHarness, harness)
	}
	return fmt.Errorf("%w: %q", ErrUnknownHarness, harness)
}

func activeHarness(harness domain.AgentHarness) bool {
	switch harness {
	case domain.HarnessClaudeCode, domain.HarnessCodex, domain.HarnessOpenCode,
		domain.HarnessCursor, domain.HarnessGrok, domain.HarnessGemini:
		return true
	default:
		return false
	}
}
