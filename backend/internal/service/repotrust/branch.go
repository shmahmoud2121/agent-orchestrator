package repotrust

import (
	"context"
	"fmt"
	"sort"
	"strings"

	aoprocess "github.com/aoagents/agent-orchestrator/backend/internal/process"
)

// DetectBranch reads an immutable Git tree without checking it out or running
// repository code. The ref is resolved before reading any blobs.
func DetectBranch(ctx context.Context, root, branch string) (Scan, error) {
	out := Scan{Files: []File{}}
	cmd := aoprocess.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--end-of-options", branch+"^{tree}")
	tree, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("resolve repository trust branch: %w", err)
	}
	treeID := strings.TrimSpace(string(tree))
	cmd = aoprocess.CommandContext(ctx, "git", "-C", root, "ls-tree", "-r", "--name-only", "-z", treeID)
	raw, err := cmd.Output()
	if err != nil {
		return out, err
	}
	paths := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	sort.Strings(paths)
	contents := map[string][]byte{}
	for _, p := range paths {
		if !candidate(p) {
			continue
		}
		cmd = aoprocess.CommandContext(ctx, "git", "-C", root, "show", treeID+":"+p)
		data, err := cmd.Output()
		if err != nil {
			return out, err
		}
		data, ignore := repositoryContent(p, data)
		if ignore {
			continue
		}
		contents[p] = data
	}
	out = inventory(contents)
	return out, nil
}

// Combine approves the base AND the actual worker checkout. Identical config
// needs one approval regardless of branch name or absolute worktree path.
func Combine(base, worker Scan) Scan {
	if worker.Fingerprint == "" || worker.Fingerprint == base.Fingerprint {
		return base
	}
	if base.Fingerprint == "" {
		return worker
	}
	contents := map[string][]byte{}
	for p, data := range base.contents {
		contents["base/"+p] = data
	}
	for p, data := range worker.contents {
		contents["worktree/"+p] = data
	}
	out := inventory(contents)
	return out
}

// Check scans both live checkouts immediately before any trust side effect.
func Check(ctx context.Context, projectID, baseRoot, workerRoot, approved string) error {
	base, err := Detect(ctx, baseRoot)
	if err != nil {
		return err
	}
	worker, err := Detect(ctx, workerRoot)
	if err != nil {
		return err
	}
	scan := Combine(base, worker)
	if scan.Fingerprint == "" || scan.Fingerprint == approved {
		return nil
	}
	return Required(projectID, scan)
}
