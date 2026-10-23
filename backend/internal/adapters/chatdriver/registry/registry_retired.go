//go:build retired

package registry

import (
	"log/slog"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/agy"
	commandcodeagent "github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/commandcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cursor"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/deepseekharness"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/droid"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimchi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/kimi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/omp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/opencodev2"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/pi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/qwen"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/unrealagent"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/agyacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/commandcodeacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/copilotacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/cursoracp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/deepseekharnessacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/droidacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/kimchiacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/kimiacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/ompacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/opencodev2acp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/piacp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/qwenacp"
	unrealchat "github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/unrealagent"
)

const retiredBuildEnabled = true

func addRetiredDrivers(r *Registry, log *slog.Logger) {
	opencodeV2Plugin := opencodev2.New()
	r.add(opencodev2acp.New(opencodeV2Plugin, log), opencodeV2Plugin)
	droidPlugin := droid.New(); r.add(droidacp.New(droidPlugin, log), droidPlugin)
	kimiPlugin := kimi.New(); r.add(kimiacp.New(kimiPlugin, log), kimiPlugin)
	kimchiPlugin := kimchi.New(); r.add(kimchiacp.New(kimchiPlugin, log), kimchiPlugin)
	piPlugin := pi.New(); r.add(piacp.New(piPlugin, log), piPlugin)
	cursorPlugin := cursor.New(); r.add(cursoracp.New(cursorPlugin, log), cursorPlugin)
	ompPlugin := omp.New(); r.add(ompacp.New(ompPlugin, log), ompPlugin)
	qwenPlugin := qwen.New(); r.add(qwenacp.New(qwenPlugin, log), qwenPlugin)
	unrealPlugin := unrealagent.New(); r.add(unrealchat.New(unrealPlugin, log), unrealPlugin)
	deepseekPlugin := deepseekharness.New(); r.add(deepseekharnessacp.New(deepseekPlugin, log), deepseekPlugin)
	commandCodePlugin := commandcodeagent.New(); r.add(commandcodeacp.New(commandCodePlugin, log), commandCodePlugin)
	copilotPlugin := copilot.New(); r.add(copilotacp.New(copilotPlugin, log), copilotPlugin)
	agyPlugin := agy.New(); r.add(agyacp.New(agyPlugin, log), agyPlugin)
}
