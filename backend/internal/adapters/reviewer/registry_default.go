//go:build !retired

package reviewer

import "github.com/aoagents/agent-orchestrator/backend/internal/domain"

const retiredReviewersEnabled = false

func retiredConstructors() []Adapter { return nil }

func registeredReviewerHarnesses() []domain.ReviewerHarness {
	return []domain.ReviewerHarness{
		domain.ReviewerClaudeCode,
		domain.ReviewerCodex,
		domain.ReviewerOpenCode,
		domain.ReviewerCursor,
		domain.ReviewerGrok,
	}
}
