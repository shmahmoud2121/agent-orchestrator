package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/cursor"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func retiredCursorTerminalFailureCorrelation(payload []byte) (string, string, bool) {
	return cursor.TerminalFailureCorrelation(payload)
}

func (c *commandContext) runRetiredCursorPermissionHook(ctx context.Context, agent, event, sessionID string, payload []byte) error {
	mode := ports.PermissionMode(strings.TrimSpace(os.Getenv(cursor.EnvPermissionMode)))
	decision := cursor.EvaluatePermission(mode, event, payload)
	launchID := validLaunchID(os.Getenv("AO_RUNTIME_LAUNCH_ID"))
	if launchID == "" {
		launchID = validLaunchID(hookLaunchID(payload))
	}
	path := "sessions/" + url.PathEscape(sessionID) + "/activity"
	req := setActivityAPIRequest{
		State: string(decision.State), Event: event, ToolName: cursor.HookToolName(event, payload),
		AgentSessionID: hookAgentSessionID(payload), LaunchID: launchID,
	}
	if err := c.postActivityHook(ctx, path, req); err != nil {
		c.reportHookFailure(agent, event, sessionID, err)
		if decision.Permission == "ask" {
			return fmt.Errorf("persist blocked Cursor activity: %w", err)
		}
	}
	if err := json.NewEncoder(c.deps.Out).Encode(cursorPermissionHookOutput{Permission: decision.Permission}); err != nil {
		c.reportHookFailure(agent, event, sessionID, fmt.Errorf("write permission response: %w", err))
	}
	return nil
}
