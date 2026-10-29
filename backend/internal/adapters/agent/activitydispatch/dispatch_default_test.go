//go:build !retired

package activitydispatch

import (
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestDeriverTokensAreKnownHarnesses(t *testing.T) {
	for token := range Derivers {
		if token == string(domain.HarnessFake) {
			continue
		}
		if !domain.AgentHarness(token).IsKnown() {
			t.Errorf("deriver token %q is not a known AgentHarness", token)
		}
	}
}

func TestSupportsHarness(t *testing.T) {
	for _, h := range []domain.AgentHarness{domain.HarnessCodex, domain.HarnessClaudeCode, domain.HarnessOpenCode, domain.HarnessCursor, domain.HarnessGrok, domain.HarnessFake} {
		if !SupportsHarness(h) {
			t.Errorf("SupportsHarness(%q) = false, want true", h)
		}
	}
	for _, h := range []domain.AgentHarness{domain.HarnessCrush, domain.HarnessOpenCodeV2, domain.HarnessAider, domain.HarnessGemini, ""} {
		if SupportsHarness(h) {
			t.Errorf("SupportsHarness(%q) = true, want false", h)
		}
	}
}

func TestSignalCoverageForHarness(t *testing.T) {
	for _, h := range []domain.AgentHarness{domain.HarnessClaudeCode, domain.HarnessCursor, domain.HarnessGrok} {
		if got := CoverageForHarness(h); got != SignalCoverageComplete {
			t.Errorf("%s coverage = %v, want complete", h, got)
		}
	}
	for _, h := range []domain.AgentHarness{domain.HarnessContinue, domain.HarnessAider, domain.HarnessCommandCode, domain.HarnessMemcode, domain.HarnessGemini} {
		if got := CoverageForHarness(h); got != SignalCoverageNone {
			t.Errorf("retired %q coverage = %v, want none", h, got)
		}
	}
}

func TestFullySupportsHarnessRequiresCompleteCoverage(t *testing.T) {
	for _, h := range []domain.AgentHarness{domain.HarnessClaudeCode, domain.HarnessCursor, domain.HarnessGrok} {
		if !FullySupportsHarness(h) {
			t.Errorf("%s needs complete coverage", h)
		}
	}
	if FullySupportsHarness(domain.HarnessContinue) || FullySupportsHarness(domain.HarnessAider) || FullySupportsHarness(domain.HarnessGemini) {
		t.Fatal("retired adapters must not report full coverage")
	}
}
