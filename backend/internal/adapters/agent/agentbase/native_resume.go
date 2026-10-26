package agentbase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/chatdriver/processenv"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	aoprocess "github.com/aoagents/agent-orchestrator/backend/internal/process"
)

// NativeJSONSessionExists runs a read-only local export/list command and requires
// an exact identity in its JSON response. CLIs which create a new conversation
// for an unknown resume ID must pass this check before AO stops the old process.
func NativeJSONSessionExists(ctx context.Context, binary string, args []string, ref ports.SessionRef, nativeID string, env map[string]string) (bool, error) {
	if strings.TrimSpace(nativeID) == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := aoprocess.CommandContext(ctx, binary, args...)
	cmd.Dir = ref.WorkspacePath
	// Merge applies domain.FilterWorkerEnv when the overlay is a worker launch
	// (AO_WORKER_ENV_KEEP). Probes that are not marked inherit the daemon env.
	cmd.Env = processenv.Merge(env)
	data, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("read native conversation %q: %w", nativeID, err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		var sessions []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &sessions); err != nil {
			return false, fmt.Errorf("invalid native session list: %w", err)
		}
		for _, session := range sessions {
			if session.ID == nativeID {
				return true, nil
			}
		}
		return false, nil
	}
	var exported struct {
		Info struct {
			ID string `json:"id"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &exported); err != nil {
		return false, fmt.Errorf("invalid native conversation JSON: %w", err)
	}
	return exported.Info.ID == nativeID, nil
}
