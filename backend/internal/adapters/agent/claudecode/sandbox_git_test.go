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

// TestWorkerSandboxSettingsOmitGitAllowWrite documents that AO does not widen
// sandbox.filesystem.allowWrite or --add-dir to the main repo's .git. Claude
// Code >= 2.1.149 already allows worktree commits against the shared .git while
// keeping hooks/ and config denied
// (https://code.claude.com/docs/en/sandboxing). Explicit allowWrite/--add-dir
// would reopen those paths.
func TestWorkerSandboxSettingsOmitGitAllowWrite(t *testing.T) {
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

	settings, err := buildWorkerSandboxSettings(
		agentruntime.PermissionAcceptEdits,
		domain.WorkerSandboxConfig{Enabled: boolPtr(true)},
	)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := settings["sandbox"].(map[string]any)
	if _, ok := sandbox["filesystem"]; ok {
		t.Fatalf("filesystem.allowWrite must be omitted: %#v", sandbox["filesystem"])
	}
	_ = worktree
}

// TestBubblewrapWriteOutsideWorktreeFails is a host-level check that bubblewrap
// can deny writes outside a worktree the way Claude Code's sandbox does. It
// does not launch `claude` (often absent in CI); when bwrap is missing the
// test is skipped.
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

// TestBubblewrapGitCommitInWorktreeSucceeds checks that a linked worktree can
// commit when the shared .git is writable — the boundary Claude Code's built-in
// worktree support provides from v2.1.149. Skipped when bubblewrap is absent.
func TestBubblewrapGitCommitInWorktreeSucceeds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap is Linux/WSL2")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
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
	if err := os.WriteFile(filepath.Join(worktree, "note.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	absGit := func(flag string) string {
		t.Helper()
		out, err := exec.Command("git", "-C", worktree, "rev-parse", flag).Output()
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Clean(strings.TrimSpace(string(out)))
		if !filepath.IsAbs(p) {
			p = filepath.Join(worktree, p)
		}
		return p
	}
	gitDir := absGit("--git-dir")
	commonDir := absGit("--git-common-dir")
	// Keep TMPDIR under the worktree bind so we do not remount /tmp and hide
	// the test paths (Go's t.TempDir lives under /tmp).
	tmp := filepath.Join(worktree, ".tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}

	bwrapGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("bwrap", append([]string{
			"--ro-bind", "/", "/",
			"--bind", worktree, worktree,
			"--bind", gitDir, gitDir,
			"--bind", commonDir, commonDir,
			"--dev", "/dev",
			"--proc", "/proc",
			"--die-with-parent",
			"--setenv", "TMPDIR", tmp,
			"git", "-C", worktree,
		}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in sandboxed worktree: %v\n%s", args, err, out)
		}
	}
	bwrapGit("add", "note.txt")
	bwrapGit("-c", "user.email=ao@example.com", "-c", "user.name=AO", "commit", "-m", "worktree commit")
}
