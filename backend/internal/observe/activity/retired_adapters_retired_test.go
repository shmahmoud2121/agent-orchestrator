//go:build retired

package activity

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/crush"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/memcode"
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/muse"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

const retiredActivityAdaptersAvailable = true
func retiredMuseAgent() ports.Agent { return muse.New() }
func retiredCrushAgent() ports.Agent { return crush.New() }
func retiredMemcodeAgent() ports.Agent { return memcode.New() }
