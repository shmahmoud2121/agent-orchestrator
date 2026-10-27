package sessionmanager

import (
	"context"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/gitdefault"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
)

func (m *Manager) repoTrustChildren(ctx context.Context, p domain.ProjectRecord) ([]string, error) {
	children := []string{}
	if p.Kind.WithDefault() == domain.ProjectKindWorkspace {
		repos, err := m.store.ListWorkspaceRepos(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			children = append(children, repo.RelativePath)
		}
	}
	return children, nil
}
func (m *Manager) checkSpawnRepoTrust(ctx context.Context, p domain.ProjectRecord, branch string) error {
	if p.Kind.WithDefault().IsGitless() {
		return nil
	}
	children, err := m.repoTrustChildren(ctx, p)
	if err != nil {
		return err
	}
	scan, err := repotrust.Project(ctx, p.Path, children, "")
	if err != nil {
		return err
	}
	if p.Path != "" {
		targets := []string{branch, p.Config.WorktreeBaseBranch()}
		if resolved, err := gitdefault.New("git", nil).Inspect(ctx, p.Path); err == nil {
			targets = append(targets, resolved.Ref)
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			// New branch names are created from the configured or locally known base.
			// Recheck the materialized checkout as well, before provider side effects.
			if other, err := repotrust.Project(ctx, p.Path, children, target); err == nil {
				scan = other
				break
			}
		}
	}
	if scan.Fingerprint == "" || scan.Fingerprint == p.Config.TrustedRepoConfigFingerprint {
		return nil
	}
	return repotrust.Required(p.ID, scan)
}
func (m *Manager) checkWorkspaceRepoTrust(ctx context.Context, p domain.ProjectRecord, workspace string) error {
	if p.Kind.WithDefault().IsGitless() {
		return nil
	}
	children, err := m.repoTrustChildren(ctx, p)
	if err != nil {
		return err
	}
	base, err := repotrust.Project(ctx, p.Path, children, "")
	if err != nil {
		return err
	}
	worker, err := repotrust.Project(ctx, workspace, children, "")
	if err != nil {
		return err
	}
	scan := repotrust.Combine(base, worker)
	if scan.Fingerprint == "" || scan.Fingerprint == p.Config.TrustedRepoConfigFingerprint {
		return nil
	}
	return repotrust.Required(p.ID, scan)
}

// CheckRepoTrust guards unattended launchers that reuse an existing project
// checkout, including reviewer panes. It reloads current approval at launch.
func (m *Manager) CheckRepoTrust(ctx context.Context, id domain.ProjectID, workspace string) error {
	project, err := m.loadProject(ctx, id)
	if err != nil {
		return err
	}
	return m.checkWorkspaceRepoTrust(ctx, project, workspace)
}
