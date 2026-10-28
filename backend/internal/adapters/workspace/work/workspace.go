// Package work materialises Work sessions as plain directories in the person's
// results folder. It never deletes those directories. Scratch files live
// elsewhere and are removed by the session manager.
package work

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// Workspace creates the manager's results folder and each worker's task folder.
type Workspace struct{}

var _ ports.Workspace = (*Workspace)(nil)
var _ ports.WorkspaceObserver = (*Workspace)(nil)
var _ ports.WorkspaceReclaimer = (*Workspace)(nil)

// New returns the Work workspace adapter.
func New() *Workspace { return &Workspace{} }

// Create makes the session directory. An orchestrator works in the results
// folder. A worker gets a new dated task folder that is never reused.
func (w *Workspace) Create(_ context.Context, cfg ports.WorkspaceConfig) (ports.WorkspaceInfo, error) {
	results := strings.TrimSpace(cfg.RepoPath)
	if results == "" || !filepath.IsAbs(results) {
		return ports.WorkspaceInfo{}, errors.New("work workspace: results folder is required")
	}
	if err := os.MkdirAll(results, 0o755); err != nil { //nolint:gosec // the person's own results folder, readable like any folder they make
		return ports.WorkspaceInfo{}, fmt.Errorf("work workspace: create results folder: %w", err)
	}
	path := results
	if cfg.Kind != domain.KindOrchestrator {
		title := cfg.TaskTitle
		when := time.Now()
		created, err := domain.UniqueTaskFolder(results, title, when, pathExists)
		if err != nil {
			return ports.WorkspaceInfo{}, err
		}
		if err := os.Mkdir(created, 0o755); err != nil { //nolint:gosec // task folder inside the person's results folder
			return ports.WorkspaceInfo{}, fmt.Errorf("work workspace: create task folder: %w", err)
		}
		path = created
	}
	return ports.WorkspaceInfo{Path: path, SessionID: cfg.SessionID, ProjectID: cfg.ProjectID}, nil
}

// Restore checks the session's existing folder and leaves it in place.
func (w *Workspace) Restore(_ context.Context, cfg ports.WorkspaceConfig) (ports.WorkspaceInfo, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" || !filepath.IsAbs(path) {
		return ports.WorkspaceInfo{}, errors.New("work workspace: restore path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return ports.WorkspaceInfo{}, fmt.Errorf("work workspace: restore %q: %w", path, err)
	}
	if !info.IsDir() {
		return ports.WorkspaceInfo{}, fmt.Errorf("work workspace: %q is not a directory", path)
	}
	return ports.WorkspaceInfo{Path: path, SessionID: cfg.SessionID, ProjectID: cfg.ProjectID}, nil
}

// Destroy leaves the results and task folders on disk. They are the person's
// documents, not an AO-managed worktree.
func (w *Workspace) Destroy(context.Context, ports.WorkspaceInfo) error { return nil }

// DestroyReclaim reports that nothing was removed.
func (w *Workspace) DestroyReclaim(context.Context, ports.WorkspaceInfo) (ports.WorkspaceReclaim, error) {
	return ports.WorkspaceReclaimAlreadyAbsent, nil
}

// ForceDestroy leaves the person's documents in place.
func (w *Workspace) ForceDestroy(context.Context, ports.WorkspaceInfo) error { return nil }

// StashUncommitted is a no-op because a Work folder is not a git worktree.
func (w *Workspace) StashUncommitted(context.Context, ports.WorkspaceInfo) (string, error) {
	return "", nil
}

// ApplyPreserved refuses a git ref. An empty ref is a no-op.
func (w *Workspace) ApplyPreserved(_ context.Context, _ ports.WorkspaceInfo, ref string) error {
	if strings.TrimSpace(ref) != "" {
		return errors.New("work workspace: preserved refs are not supported")
	}
	return nil
}

// AddExclude is a no-op because Work folders have no git exclude file.
func (w *Workspace) AddExclude(context.Context, ports.WorkspaceInfo, ...string) error { return nil }

// ObserveWorkspace returns the folder and no git facts.
func (w *Workspace) ObserveWorkspace(_ context.Context, info ports.WorkspaceInfo) (ports.WorkspaceObservation, error) {
	path := strings.TrimSpace(info.Path)
	if path == "" {
		return ports.WorkspaceObservation{}, errors.New("work workspace: empty path")
	}
	return ports.WorkspaceObservation{Path: path}, nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
