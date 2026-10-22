package project

import (
	"context"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
)

// RepoTrust scans current checkout contents, never a caller-provided file list.
func (m *Service) RepoTrust(ctx context.Context, id domain.ProjectID, branch string) (repotrust.Scan, error) {
	row, err := m.repoTrustProject(ctx, id)
	if err != nil {
		return repotrust.Scan{}, err
	}
	scan, err := m.repoTrustScan(ctx, row, branch)
	scan.Approved = scan.Fingerprint != "" && scan.Fingerprint == row.Config.TrustedRepoConfigFingerprint
	return scan, err
}

// SetRepoTrust is human-only, like localmerge's caller-session boundary, but
// rejects orchestrators too. It guards mistakes, not agents stripping identity.
func (m *Service) SetRepoTrust(ctx context.Context, id domain.ProjectID, fingerprint, caller, branch string, revoke bool) (repotrust.Scan, error) {
	if strings.TrimSpace(caller) != "" {
		return repotrust.Scan{}, apierr.Forbidden("REPO_TRUST_FORBIDDEN", "Agent sessions cannot approve or revoke repository trust; ask a person to use Project Settings or ao project trust")
	}
	m.addMu.Lock()
	defer m.addMu.Unlock()
	row, err := m.repoTrustProject(ctx, id)
	if err != nil {
		return repotrust.Scan{}, err
	}
	scan, err := m.repoTrustScan(ctx, row, branch)
	if err != nil {
		return scan, err
	}
	if !revoke && (fingerprint == "" || fingerprint != scan.Fingerprint) {
		return scan, apierr.Conflict("REPO_TRUST_STALE", "Repository configuration changed; scan and review the current files before approving", map[string]any{"fingerprint": scan.Fingerprint, "files": scan.Files})
	}
	row.Config.TrustedRepoConfigFingerprint = ""
	if !revoke {
		row.Config.TrustedRepoConfigFingerprint = fingerprint
	}
	if err := m.store.UpsertProject(ctx, row); err != nil {
		return scan, apierr.Internal("PROJECT_CONFIG_UPDATE_FAILED", "Failed to persist repository trust")
	}
	scan.Approved = !revoke && scan.Fingerprint != ""
	return scan, nil
}
func (m *Service) repoTrustProject(ctx context.Context, id domain.ProjectID) (domain.ProjectRecord, error) {
	if err := validateProjectID(id); err != nil {
		return domain.ProjectRecord{}, err
	}
	row, ok, err := m.store.GetProject(ctx, string(id))
	if err != nil {
		return row, apierr.Internal("PROJECT_LOAD_FAILED", "Failed to load project")
	}
	if !ok || !row.ArchivedAt.IsZero() {
		return row, apierr.NotFound("PROJECT_NOT_FOUND", "Unknown project")
	}
	return row, nil
}

func (m *Service) repoTrustScan(ctx context.Context, row domain.ProjectRecord, branch string) (repotrust.Scan, error) {
	children := []string{}
	if row.Kind.WithDefault() == domain.ProjectKindWorkspace {
		repos, err := m.store.ListWorkspaceRepos(ctx, row.ID)
		if err != nil {
			return repotrust.Scan{}, err
		}
		for _, repo := range repos {
			children = append(children, repo.RelativePath)
		}
	}
	scan, err := repotrust.Project(ctx, row.Path, children, branch)
	if err != nil && branch != "" {
		return scan, apierr.Invalid("REPO_TRUST_BRANCH_INVALID", "Cannot scan the requested branch", nil)
	}
	return scan, err
}
