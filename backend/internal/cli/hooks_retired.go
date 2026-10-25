//go:build retired

package cli

import (
	"os"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
)

func (c *commandContext) handleRetiredMemcodeHook(agent, event, sessionID string) bool {
	if agent != "memcode" {
		return false
	}
	if strings.HasPrefix(event, "context-chunk-") {
		if err := memcode.EmitInstructionChunk(c.deps.Out, event); err != nil {
			c.reportHookFailure(agent, event, sessionID, err)
		}
		return true
	}
	if event == "session-start" {
		reportAfterRestore, err := memcode.EmitStartWitness(c.deps.Out)
		if err != nil {
			c.reportHookFailure(agent, event, sessionID, err)
			return true
		}
		return os.Getenv("AO_MEMCODE_INTERACTIVE_RESTORE") == "1" && !reportAfterRestore
	}
	return event == "session-end"
}
