//go:build !retired

package repotrust_test

import "github.com/aoagents/agent-orchestrator/backend/internal/ports"

func retiredRepoTrustAgents() map[string]ports.Agent { return nil }
