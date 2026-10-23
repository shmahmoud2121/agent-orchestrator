//go:build retired

package repotrust_test

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cursor"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/devin"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/gemini"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/mimocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/vibe"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func retiredRepoTrustAgents() map[string]ports.Agent {
	return map[string]ports.Agent{
		"gemini": gemini.New(), "cursor": cursor.New(), "copilot": copilot.New(),
		"vibe": vibe.New(), "mimo": mimocode.New(), "command": commandcode.New(), "devin": devin.New(),
	}
}
