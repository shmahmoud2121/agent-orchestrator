//go:build retired

package registry

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/agy"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/aider"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/amp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/auggie"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/autohand"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cline"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/codewhale"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/continueagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/crush"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cursor"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/deepseekharness"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/devin"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/droid"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/fx"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/gemini"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/goose"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/grok"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kilocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimchi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kiro"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/mimocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/muse"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/omp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/opencodev2"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/openhands"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/pi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/primeagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/qwen"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/unrealagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/vibe"
)

const retiredBuildEnabled = true

func retiredConstructors() []adapters.Adapter {
	return []adapters.Adapter{
		agy.New(), aider.New(), amp.New(), auggie.New(), autohand.New(), cline.New(),
		codewhale.New(), commandcode.New(), continueagent.New(), copilot.New(), crush.New(),
		cursor.New(), deepseekharness.New(), devin.New(), droid.New(), fx.New(), gemini.New(),
		goose.New(), grok.New(), kilocode.New(), kimchi.New(), kimi.New(), kiro.New(),
		memcode.New(), mimocode.New(), muse.New(), omp.New(), opencodev2.New(), openhands.New(),
		pi.New(), primeagent.New(), qwen.New(), unrealagent.New(), vibe.New(),
	}
}
