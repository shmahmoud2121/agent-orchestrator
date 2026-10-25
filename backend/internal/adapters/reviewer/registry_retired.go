//go:build retired

package reviewer

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/agy"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/aider"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/amp"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/auggie"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/autohand"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/cline"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/copilot"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/crush"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/devin"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/droid"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/kilocode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/kimchi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/kimi"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/kiro"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/muse"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/opencodev2"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/reviewer/pi"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

const retiredReviewersEnabled = true

func retiredConstructors() []Adapter {
	return []Adapter{
		agy.New(), aider.New(), amp.New(), auggie.New(), autohand.New(), cline.New(),
		copilot.New(), crush.New(), devin.New(), droid.New(),
		kilocode.New(), kimchi.New(), kimi.New(), kiro.New(), muse.New(), opencodev2.New(), pi.New(),
	}
}

func registeredReviewerHarnesses() []domain.ReviewerHarness { return domain.AllReviewerHarnesses }
