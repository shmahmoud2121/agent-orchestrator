package sessionmanager

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// workspaceHandleIncomplete reports a session that never finished creating its
// folder. Gitless sessions have no branch, so an empty branch is complete for
// them once the folder exists.
func workspaceHandleIncomplete(meta domain.SessionMetadata, kind domain.ProjectKind) bool {
	return meta.WorkspacePath == "" || (meta.Branch == "" && !kind.IsGitless())
}

// workHooksReady is optional. A Work session fails closed unless the harness
// reports that its pre-tool-use hook will run for this workspace.
type workHooksReady interface {
	WorkHooksReady(ctx context.Context, workspacePath string) (bool, error)
}

// ensureWorkGuards refuses to start a Work session unless the originals, the
// watched harness, the scratch folder, and the pre-tool-use hook are all in
// place. Other project kinds are unchanged.
func (m *Manager) ensureWorkGuards(ctx context.Context, agent ports.Agent, id domain.SessionID, projectID domain.ProjectID, workspacePath string, env map[string]string) error {
	if projectID == "" {
		return nil
	}
	project, err := m.loadProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.Kind.WithDefault() != domain.ProjectKindWork {
		return nil
	}
	if !workConfigReady(project) {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	layout := *project.Config.Work
	layout.Sources = append([]string(nil), project.Config.Work.Sources...)
	if err := domain.ValidateWorkLayout(&layout, domain.AOHome(m.dataDir)); err != nil {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	rec, ok, err := m.store.GetSession(ctx, id)
	if err != nil || !ok || !domain.WorkHarnessWatched(rec.Harness) {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	if err := m.requireWorkScratch(env, project, id); err != nil {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	ready, ok := agent.(workHooksReady)
	if !ok {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	installed, err := ready.WorkHooksReady(ctx, workspacePath)
	if err != nil || !installed {
		return fmt.Errorf("%w", ErrWorkGuardUnavailable)
	}
	return nil
}

func workConfigReady(project domain.ProjectRecord) bool {
	work := project.Config.Work
	return work != nil && len(work.Sources) > 0 && strings.TrimSpace(work.ResultsDir) != ""
}

// requireWorkScratch creates the session's private scratch folder and publishes
// it as AO_WORK_SCRATCH. The folder is removed when the session ends.
func (m *Manager) requireWorkScratch(env map[string]string, project domain.ProjectRecord, id domain.SessionID) error {
	if project.Kind.WithDefault() != domain.ProjectKindWork {
		return nil
	}
	dir, err := domain.WorkScratchDir(m.dataDir, project.ID, string(id))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if env != nil {
		env["AO_WORK_SCRATCH"] = dir
	}
	return nil
}

// removeWorkScratch deletes the private scratch folder after the session has
// ended. Results and task folders are left on disk.
func (m *Manager) removeWorkScratch(ctx context.Context, id domain.SessionID) {
	rec, ok, err := m.store.GetSession(ctx, id)
	if err != nil || !ok || rec.ProjectID == "" {
		return
	}
	project, err := m.loadProject(ctx, rec.ProjectID)
	if err != nil || project.Kind.WithDefault() != domain.ProjectKindWork {
		return
	}
	dir, err := domain.WorkScratchDir(m.dataDir, project.ID, string(id))
	if err != nil || strings.TrimSpace(dir) == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil && m.logger != nil {
		m.logger.Warn("remove work scratch", "sessionID", id, "error", err)
	}
}

func workWriteDir(project domain.ProjectRecord, kind domain.SessionKind, workspacePath string) string {
	if kind == domain.KindWorker && strings.TrimSpace(workspacePath) != "" {
		return workspacePath
	}
	if project.Config.Work != nil && strings.TrimSpace(project.Config.Work.ResultsDir) != "" {
		return project.Config.Work.ResultsDir
	}
	return project.Path
}

// refreshWorkInstructions rewrites the standing prompt once the task folder
// exists, so a worker is told to write there and not only into the results root.
func (m *Manager) refreshWorkInstructions(id domain.SessionID, harness domain.AgentHarness, project domain.ProjectRecord, kind domain.SessionKind, workspacePath string, systemPrompt, systemPromptFile *string) error {
	if project.Kind.WithDefault() != domain.ProjectKindWork || systemPrompt == nil {
		return nil
	}
	next := applyWorkOriginals(*systemPrompt, project, workWriteDir(project, kind, workspacePath))
	*systemPrompt = next
	path, err := m.prepareSystemPromptFile(id, harness, next)
	if err != nil {
		return err
	}
	if systemPromptFile != nil && path != "" {
		*systemPromptFile = path
	}
	return nil
}

// workChatDirectories adds Claude Code's readable source folders. Codex is
// left unchanged: its extra-directory flag would grant writes.
func workChatDirectories(harness domain.AgentHarness, project domain.ProjectRecord, base []string) []string {
	extra := domain.WorkReadableDirs(project, harness)
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	return append(out, extra...)
}

// disableWorkSandboxWhenUnavailable keeps a Work worker starting on hosts
// where Claude Code's OS sandbox cannot run. The watchdog and the built-in
// instructions are the guards there. Where the sandbox can run, it stays on.
func disableWorkSandboxWhenUnavailable(project domain.ProjectRecord, sandbox domain.WorkerSandboxConfig) domain.WorkerSandboxConfig {
	if project.Kind.WithDefault() != domain.ProjectKindWork || !sandbox.IsEnabled() {
		return sandbox
	}
	if claudecode.CheckSandboxAvailability() == nil {
		return sandbox
	}
	off := false
	sandbox.Enabled = &off
	return sandbox
}
