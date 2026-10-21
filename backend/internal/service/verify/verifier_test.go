//go:build !windows

package verify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

type fakeStores struct {
	session domain.SessionRecord
	project domain.ProjectRecord
}

func (f *fakeStores) GetSession(_ context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	return f.session, f.session.ID == id, nil
}

func (f *fakeStores) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	return f.project, f.project.ID == id, nil
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes a project repo whose main has a passing check script and a
// worker branch whose head makes the check fail.
func newRepo(t *testing.T) (repo, goodSHA, badSHA string) {
	t.Helper()
	repo = t.TempDir()
	runGit(t, repo, "init", "-q", "-b", "main")
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(repo, "status"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok\n")
	runGit(t, repo, "add", "status")
	runGit(t, repo, "commit", "-q", "-m", "good")
	goodSHA = runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "checkout", "-q", "-b", "worker")
	write("broken\n")
	runGit(t, repo, "commit", "-q", "-am", "bad")
	badSHA = runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "checkout", "-q", "main")
	return repo, goodSHA, badSHA
}

func newVerifier(repo string, cfg domain.ProjectConfig) (*Verifier, domain.PullRequest) {
	stores := &fakeStores{
		session: domain.SessionRecord{ID: "s1", ProjectID: "p1"},
		project: domain.ProjectRecord{ID: "p1", Path: repo, Config: cfg},
	}
	return New(Deps{Sessions: stores, Projects: stores}), domain.PullRequest{URL: "https://example/pr/1", SessionID: "s1", SourceBranch: "worker"}
}

// checkStatus passes only when the checked-out tree's status file says ok.
var checkStatus = []string{"sh", "-c", `grep -qx ok status || { echo "status: $(cat status)"; exit 1; }`}

func assertNoWorktrees(t *testing.T, repo string) {
	t.Helper()
	if list := runGit(t, repo, "worktree", "list", "--porcelain"); strings.Count(list, "worktree ") != 1 {
		t.Fatalf("throwaway worktree left behind:\n%s", list)
	}
}

func TestVerifyMerge_RunsCommandOnExactHead(t *testing.T) {
	repo, good, bad := newRepo(t)
	v, pr := newVerifier(repo, domain.ProjectConfig{VerifyCommand: checkStatus})

	if err := v.VerifyMerge(context.Background(), pr, good); err != nil {
		t.Fatalf("good head: %v", err)
	}
	err := v.VerifyMerge(context.Background(), pr, bad)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "status: broken") {
		t.Fatalf("bad head err = %v, want ErrFailed carrying the command output", err)
	}
	// The project's own checkout is never touched.
	if head := runGit(t, repo, "rev-parse", "HEAD"); head != good {
		t.Fatalf("project HEAD moved to %s", head)
	}
	assertNoWorktrees(t, repo)
}

func TestVerifyMerge_NoCommandIsANoOp(t *testing.T) {
	v, pr := newVerifier("/nonexistent", domain.ProjectConfig{})
	if err := v.VerifyMerge(context.Background(), pr, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	pr.SessionID = ""
	if err := v.VerifyMerge(context.Background(), pr, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMerge_TimeoutFails(t *testing.T) {
	repo, good, _ := newRepo(t)
	v, pr := newVerifier(repo, domain.ProjectConfig{VerifyCommand: []string{"sleep", "30"}, VerifyTimeoutSeconds: 1})
	err := v.VerifyMerge(context.Background(), pr, good)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	assertNoWorktrees(t, repo)
}

func TestVerifyMerge_UnknownHeadAndUnrunnableCommandFail(t *testing.T) {
	repo, good, _ := newRepo(t)
	v, pr := newVerifier(repo, domain.ProjectConfig{VerifyCommand: checkStatus})
	pr.SourceBranch = ""
	if err := v.VerifyMerge(context.Background(), pr, strings.Repeat("b", 40)); !errors.Is(err, ErrFailed) {
		t.Fatalf("unknown head err = %v", err)
	}

	v, pr = newVerifier(repo, domain.ProjectConfig{VerifyCommand: []string{"definitely-not-a-real-binary-ao"}})
	if err := v.VerifyMerge(context.Background(), pr, good); !errors.Is(err, ErrFailed) {
		t.Fatalf("missing binary err = %v", err)
	}
	assertNoWorktrees(t, repo)
}

func TestVerifyMerge_FetchesHeadFromOrigin(t *testing.T) {
	origin, _, bad := newRepo(t)
	clone := t.TempDir()
	runGit(t, clone, "clone", "-q", "--single-branch", "-b", "main", origin, ".")
	v, pr := newVerifier(clone, domain.ProjectConfig{VerifyCommand: checkStatus})
	err := v.VerifyMerge(context.Background(), pr, bad)
	if !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), "status: broken") {
		t.Fatalf("err = %v, want the command to run on the fetched head", err)
	}
}

func TestGitCommandNeverPromptsForCredentials(t *testing.T) {
	env := strings.Join(gitCommand(context.Background(), t.TempDir(), "fetch").Env, "\n")
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never"} {
		if !strings.Contains(env, want) {
			t.Fatalf("git env is missing %s", want)
		}
	}
}

func TestVerifyMerge_RefusesWorkspaceProjects(t *testing.T) {
	repo, good, _ := newRepo(t)
	stores := &fakeStores{
		session: domain.SessionRecord{ID: "s1", ProjectID: "p1"},
		project: domain.ProjectRecord{ID: "p1", Path: repo, Kind: domain.ProjectKindWorkspace, Config: domain.ProjectConfig{VerifyCommand: checkStatus}},
	}
	v := New(Deps{Sessions: stores, Projects: stores})
	if err := v.VerifyMerge(context.Background(), domain.PullRequest{SessionID: "s1"}, good); !errors.Is(err, ErrFailed) {
		t.Fatalf("err = %v", err)
	}
}
