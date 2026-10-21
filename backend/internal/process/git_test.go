package process

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitFixture(t *testing.T) (repo, worktree string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	repo = filepath.Join(base, "repo")
	worktree = filepath.Join(base, "wt")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", repo)
	run("-C", repo, "commit", "-q", "--allow-empty", "-m", "init")
	run("-C", repo, "worktree", "add", "-q", "-b", "w", worktree)
	return repo, worktree
}

func TestGitCommandsCarrySafetyFlags(t *testing.T) {
	cmd := Command("git", "-C", t.TempDir(), "status")
	want := append([]string{"git"}, gitSafetyArgs()...)
	for i, arg := range want {
		if cmd.Args[i] != arg {
			t.Fatalf("args[%d] = %q, want %q (args %v)", i, cmd.Args[i], arg, cmd.Args)
		}
	}
	if other := Command("echo", "hi"); len(other.Args) != 2 {
		t.Fatalf("non-git command was modified: %v", other.Args)
	}
}

func TestGenuineWorktreeRuns(t *testing.T) {
	_, worktree := gitFixture(t)
	if out, err := Command("git", "-C", worktree, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Fatalf("status in genuine worktree failed: %v\n%s", err, out)
	}
}

// A worker that can only edit files inside its worktree swaps the .git pointer
// for a planted repository whose config would run a command on `git status`.
func TestPlantedGitDirIsRefusedBeforeGitRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fsmonitor shell payload is POSIX-only")
	}
	_, worktree := gitFixture(t)
	marker := filepath.Join(t.TempDir(), "PWNED")
	evil := filepath.Join(worktree, "evil")
	for _, dir := range []string{filepath.Join(evil, "objects"), filepath.Join(evil, "refs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(evil, "HEAD"), "ref: refs/heads/x\n")
	write(filepath.Join(evil, "config"), "[core]\n\trepositoryformatversion = 0\n\tfsmonitor = \"touch '"+marker+"'; false\"\n")
	write(filepath.Join(worktree, ".git"), "gitdir: ./evil\n")

	err := Command("git", "-C", worktree, "status", "--porcelain").Run()
	if !errors.Is(err, ErrUnsafeGitDir) {
		t.Fatalf("Run() error = %v, want ErrUnsafeGitDir", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("planted fsmonitor command ran")
	}
}

func TestSeparateGitDirAndMovedWorktreeStillRun(t *testing.T) {
	repo, worktree := gitFixture(t)
	moved := worktree + "-moved"
	if err := os.Rename(worktree, moved); err != nil {
		t.Fatal(err)
	}
	if out, err := Command("git", "-C", moved, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Fatalf("status in moved worktree failed: %v\n%s", err, out)
	}
	sep := filepath.Join(filepath.Dir(repo), "sep")
	data := filepath.Join(filepath.Dir(repo), "sep-data")
	if out, err := exec.Command("git", "init", "-q", "--separate-git-dir", data, sep).CombinedOutput(); err != nil {
		t.Fatalf("init separate git dir: %v\n%s", err, out)
	}
	if out, err := Command("git", "-C", sep, "status").CombinedOutput(); err != nil {
		t.Fatalf("status with separate git dir failed: %v\n%s", err, out)
	}
}

func TestCheckGitLinkRefusesAnUninspectableGitFile(t *testing.T) {
	base := t.TempDir()
	if err := checkGitLink(filepath.Join(base, "missing")); err != nil {
		t.Fatalf("missing checkout: %v", err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkGitLink(file); err != nil {
		t.Fatalf("non-directory path: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("name-length errors differ on Windows")
	}
	// A path component over NAME_MAX makes Lstat fail with something other
	// than "not found", which must not be read as "no .git file".
	long := filepath.Join(base, strings.Repeat("x", 300))
	if err := checkGitLink(long); !errors.Is(err, ErrUnsafeGitDir) {
		t.Fatalf("uninspectable .git err = %v, want ErrUnsafeGitDir", err)
	}
}
