//go:build !retired

package activitydispatch

import "github.com/aoagents/agent-orchestrator/backend/internal/domain"

func registerRetiredDerivers(map[string]DeriveFunc) {}

func retiredSignalCoverageOverrides() map[domain.AgentHarness]SignalCoverage { return nil }
