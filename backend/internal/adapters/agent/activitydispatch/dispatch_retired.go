//go:build retired

package activitydispatch

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/activitystate"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/agy"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/aider"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/amp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/auggie"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cline"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/continueagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/droid"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/gemini"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimchi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kiro"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/mimocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/muse"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/omp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/opencodev2"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/pi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/primeagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/vibe"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func registerRetiredDerivers(d map[string]DeriveFunc) {
	d["command-code"] = commandcode.DeriveActivityState
	d["muse"] = muse.DeriveActivityState
	d["omp"] = omp.DeriveActivityState
	d["continue"] = continueagent.DeriveActivityState
	d["droid"] = droid.DeriveActivityState
	d["agy"] = agy.DeriveActivityState
	d["gemini"] = gemini.DeriveActivityState
	d["aider"] = aider.DeriveActivityState
	d["kimchi"] = kimchi.DeriveActivityState
	d["opencode-v2"] = opencodev2.DeriveActivityState
	d["prime-agent"] = primeagent.DeriveActivityState
	d["mimo-code"] = mimocode.DeriveActivityState
	d["memcode"] = memcode.DeriveActivityState
	d["amp"] = amp.DeriveActivityState
	d["pi"] = pi.DeriveActivityState
	d["auggie"] = auggie.DeriveActivityState
	d["goose"] = activitystate.StandardDeriveActivityState
	d["openhands"] = activitystate.StandardDeriveActivityState
	d["devin"] = activitystate.StandardDeriveActivityState
	d["qwen"] = activitystate.StandardDeriveActivityState
	d["copilot"] = activitystate.StandardDeriveActivityState
	d["kimi"] = activitystate.StandardDeriveActivityState
	d["cline"] = cline.DeriveActivityState
	d["kiro"] = kiro.DeriveActivityState
	d["kilocode"] = activitystate.StandardDeriveActivityState
	d["autohand"] = activitystate.StandardDeriveActivityState
	d["vibe"] = vibe.DeriveActivityState
}

func retiredSignalCoverageOverrides() map[domain.AgentHarness]SignalCoverage {
	return map[domain.AgentHarness]SignalCoverage{
		domain.HarnessAider:       SignalCoveragePartial,
		domain.HarnessCommandCode: SignalCoveragePartial,
		domain.HarnessContinue:    SignalCoveragePartial,
		domain.HarnessMemcode:     SignalCoveragePartial,
	}
}
