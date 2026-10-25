//go:build !retired

package registry

import "github.com/aoagents/agent-orchestrator/backend/internal/adapters"

const retiredBuildEnabled = false

func retiredConstructors() []adapters.Adapter { return nil }
