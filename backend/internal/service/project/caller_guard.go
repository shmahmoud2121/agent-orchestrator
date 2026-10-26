package project

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

// SessionCatalog resolves the session named by X-AO-Session-ID.
// A request with no session header is the person and is not looked up.
type SessionCatalog interface {
	GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error)
}

// callerChange is the slice of a project write the caller is attempting.
type callerChange struct {
	permissions bool
	sandbox     bool
	trust       bool
	teamMode    bool
	// anyWrite is a project config write. Workers are refused even when the
	// sensitive fields above are unchanged. Orchestrators may change the rest.
	anyWrite bool
}

func (m *Service) authorizeCaller(ctx context.Context, projectID domain.ProjectID, caller string, change callerChange) error {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return nil
	}
	if m.sessionCatalog == nil {
		return apierr.Forbidden("PROJECT_SETTINGS_FORBIDDEN", "Only the person can change project settings. Ask them in your report.")
	}
	sess, ok, err := m.sessionCatalog.GetSession(ctx, domain.SessionID(caller))
	if err != nil || !ok || sess.ProjectID != projectID {
		return apierr.Forbidden("PROJECT_SETTINGS_FORBIDDEN", fmt.Sprintf(
			"Only the person can change project settings. Calling session %s cannot change them. Ask the person in your report.", caller))
	}
	switch sess.Kind {
	case domain.KindWorker:
		return apierr.Forbidden("PROJECT_SETTINGS_FORBIDDEN", workerSettingsRefusal(change))
	case domain.KindOrchestrator:
		if change.permissions || change.sandbox || change.trust || change.teamMode {
			return apierr.Forbidden("PROJECT_SETTINGS_FORBIDDEN", orchestratorSettingsRefusal(change))
		}
		return nil
	default:
		return apierr.Forbidden("PROJECT_SETTINGS_FORBIDDEN", fmt.Sprintf(
			"Only the person can change project settings. Calling session %s cannot change them. Ask the person in your report.", caller))
	}
}

func workerSettingsRefusal(change callerChange) string {
	switch {
	case change.permissions:
		return "Only the person can change permissions. You are a worker: ask the person in your report."
	case change.sandbox:
		return "Only the person can change the worker sandbox. You are a worker: ask the person in your report."
	case change.trust:
		return "Only the person can change repository trust. You are a worker: ask the person in your report."
	case change.teamMode:
		return "Only the person can change team mode. You are a worker: ask the person in your report."
	default:
		return "Only the person can change project settings. You are a worker: ask the person in your report."
	}
}

func orchestratorSettingsRefusal(change callerChange) string {
	switch {
	case change.permissions:
		return "Only the person can change permissions. You are the manager and can be misled by worker output. Ask the person in your report."
	case change.sandbox:
		return "Only the person can change the worker sandbox. You are the manager and can be misled by worker output. Ask the person in your report."
	case change.trust:
		return "Only the person can change repository trust. You are the manager and can be misled by worker output. Ask the person in your report."
	case change.teamMode:
		return "Only the person can change team mode. You are the manager and can be misled by worker output. Ask the person in your report."
	default:
		return "Only the person can change project settings. You are the manager and can be misled by worker output. Ask the person in your report."
	}
}

func sensitiveConfigChange(before, after domain.ProjectConfig) callerChange {
	return callerChange{
		anyWrite: true,
		permissions: before.AgentConfig.Permissions != after.AgentConfig.Permissions ||
			before.Worker.AgentConfig.Permissions != after.Worker.AgentConfig.Permissions ||
			before.Orchestrator.AgentConfig.Permissions != after.Orchestrator.AgentConfig.Permissions,
		sandbox: before.WorkerSandbox.IsEnabled() != after.WorkerSandbox.IsEnabled() ||
			!slices.Equal(before.WorkerSandbox.AllowedDomains, after.WorkerSandbox.AllowedDomains),
		teamMode: before.TeamModeEnabled() != after.TeamModeEnabled(),
	}
}
