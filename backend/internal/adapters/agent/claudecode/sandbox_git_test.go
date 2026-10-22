package claudecode

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
)

// TestResolveGitWritePathsForLinkedWorktree ensures commits in a linked worktree
// get allowWrite for both the worktree git-dir and the main repo's common dir,
// without opening the whole main working tree.
func TestResolveGitWritePathsForLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(main, "init")
	run(main, "config", "user.email", "ao@example.com")
	run(main, "config", "user.name", "AO")
	if err := os.WriteFile(filepath.Join(main, "README"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(main, "add", "README")
	run(main, "commit", "-m", "init")

	worktree := filepath.Join(t.TempDir(), "wt")
	run(main, "worktree", "add", "-b", "ao/worker", worktree)

	gitDir, commonDir := resolveGitWritePaths(worktree)
	if gitDir == "" || commonDir == "" {
		t.Fatalf("gitDir=%q commonDir=%q", gitDir, commonDir)
	}
	if gitDir == commonDir {
		t.Fatalf("linked worktree should separate git-dir (%s) from common-dir (%s)", gitDir, commonDir)
	}
	if !strings.Contains(filepath.ToSlash(gitDir), "/worktrees/") {
		t.Fatalf("git-dir should be under .git/worktrees: %s", gitDir)
	}
	// Main working tree must not appear as a write root — only .git paths.
	if gitDir == main || commonDir == main {
		t.Fatalf("must not allow the whole main repo: gitDir=%s commonDir=%s main=%s", gitDir, commonDir, main)
	}

	settings, addDirs, err := buildWorkerSandboxSettings(
		agentruntime.PermissionAcceptEdits,
		domain.WorkerSandboxConfig{Enabled: boolPtr(true)},
		worktree,
		4242,
	)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := settings["sandbox"].(map[string]any)
	fs := sandbox["filesystem"].(map[string]any)
	allow := asStringSlice(t, fs["allowWrite"])
	if !containsAll(allow, gitDir, commonDir) {
		t.Fatalf("allowWrite=%v want %s and %s", allow, gitDir, commonDir)
	}
	if !containsAll(addDirs, gitDir, commonDir) {
		t.Fatalf("addDirs=%v", addDirs)
	}
}

// TestSandboxExcludedCommandsIncludeAO documents that Linux bubblewrap gives
// sandboxed commands a private localhost, so the ao CLI (loopback daemon
// client) must run outside the sandbox network namespace while still using the
// authenticated token file. See sandbox.go excludedCommands comment.
func TestSandboxExcludedCommandsIncludeAO(t *testing.T) {
	settings, _, err := buildWorkerSandboxSettings(
		agentruntime.PermissionAuto,
		domain.WorkerSandboxConfig{},
		t.TempDir(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	excluded := asStringSlice(t, settings["sandbox"].(map[string]any)["excludedCommands"])
	if !containsAll(excluded, "ao", "ao *") {
		t.Fatalf("excludedCommands=%v", excluded)
	}
}

// TestBubblewrapWriteOutsideWorktreeFails is a host-level check that bubblewrap
// can deny writes outside a worktree the way Claude Code's sandbox does. It
// does not launch `claude` (often absent in CI); when bwrap is missing the
// test is skipped and the PR documents the manual Claude Code check.
func TestBubblewrapWriteOutsideWorktreeFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap is Linux/WSL2")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	worktree := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	cmd := exec.Command("bwrap",
		"--ro-bind", "/", "/",
		"--bind", worktree, worktree,
		"--dev", "/dev",
		"--proc", "/proc",
		"--die-with-parent",
		"bash", "-c", "echo pwned > "+outside,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected write outside worktree to fail; wrote successfully\n%s", out)
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Fatalf("outside file was created despite sandbox")
	}
}
