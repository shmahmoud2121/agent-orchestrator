//go:build !retired

package activity

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/fake"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

const retiredActivityAdaptersAvailable = false
func retiredMuseAgent() ports.Agent { return fake.New() }
func retiredCrushAgent() ports.Agent { return fake.New() }
func retiredMemcodeAgent() ports.Agent { return fake.New() }
