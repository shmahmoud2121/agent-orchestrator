package sessionmanager

import (
	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// refuseUnsandboxedWorker stops a worker start or restore when the project
// requires a sandboxed worker and this launch would not actually be sandboxed.
// Orchestrators and the person's own terminals are not workers, so they pass.
func refuseUnsandboxedWorker(kind domain.SessionKind, harness domain.AgentHarness, sandbox domain.WorkerSandboxConfig, restore bool) error {
	available, message := claudecode.WorkerSandboxOSState()
	return domain.RefuseUnsandboxedWorker(kind, harness, sandbox, available, restore, message)
}

// refuseUnsandboxedWorkerForProject applies the project requireSandbox policy.
// Standalone workers have no project, so there is nothing to require.
func refuseUnsandboxedWorkerForProject(project domain.ProjectRecord, kind domain.SessionKind, harness domain.AgentHarness, restore bool) error {
	if project.ID == "" {
		return nil
	}
	// Work sessions are guarded by the original-file watchdog. Codex is a
	// watched Work harness and has no OS sandbox, and Claude's sandbox does
	// not run on native Windows. The code-project sandbox requirement does
	// not apply.
	if project.Kind.WithDefault() == domain.ProjectKindWork {
		return nil
	}
	return refuseUnsandboxedWorker(kind, harness, project.Config.WorkerSandbox, restore)
}

// disableSandboxWhenUnavailable keeps a worker starting on hosts where Claude
// Code's OS sandbox cannot run (native Windows, or Linux without bubblewrap and
// socat), but only where the person allowed unsandboxed workers: a Work project,
// or a project with "Only allow sandboxed workers" turned off. The watchdog is
// the guard there. Where the sandbox can run, or the project requires it, the
// config is unchanged, so a required sandbox still refuses the worker.
func disableSandboxWhenUnavailable(project domain.ProjectRecord, sandbox domain.WorkerSandboxConfig) domain.WorkerSandboxConfig {
	if !sandbox.IsEnabled() {
		return sandbox
	}
	if project.Kind.WithDefault() != domain.ProjectKindWork && sandbox.IsRequired() {
		return sandbox
	}
	if claudecode.CheckSandboxAvailability() == nil {
		return sandbox
	}
	off := false
	sandbox.Enabled = &off
	return sandbox
}
