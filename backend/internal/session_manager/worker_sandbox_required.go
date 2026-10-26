package sessionmanager

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// refuseUnsandboxedWorker stops a worker start or restore when the project
// requires a sandboxed worker and this launch would not actually be sandboxed.
// Orchestrators and the person's own terminals are not workers, so they pass.
func refuseUnsandboxedWorker(kind domain.SessionKind, harness domain.AgentHarness, sandbox domain.WorkerSandboxConfig, restore bool) error {
	return domain.RefuseUnsandboxedWorker(kind, harness, sandbox, claudecode.CheckSandboxAvailability() == nil, restore)
}

// refuseUnsandboxedWorkerForProject applies the project requireSandbox policy.
// Standalone workers have no project, so there is nothing to require.
func refuseUnsandboxedWorkerForProject(project domain.ProjectRecord, kind domain.SessionKind, harness domain.AgentHarness, restore bool) error {
	if project.ID == "" {
		return nil
	}
	return refuseUnsandboxedWorker(kind, harness, project.Config.WorkerSandbox, restore)
}
