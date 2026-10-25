//go:build !retired

package modelcatalog

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type fxCatalogRunner func(context.Context, string, []string, string, map[string]string) ([]byte, error)

func fxSettingsPath(env map[string]string) string {
	return filepath.Join(catalogHome(env), ".fx", "settings.json")
}

func discoverFXCatalog(context.Context, ports.AgentModelDiscoveryRequest, fxCatalogRunner) (ports.AgentModelCatalog, error) {
	return ports.AgentModelCatalog{}, fmt.Errorf("this agent is retired in this build")
}
