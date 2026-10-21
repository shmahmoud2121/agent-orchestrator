// Package verify runs a project's verify command against a pull request's
// exact head in a throwaway worktree, so the daemon itself proves "tests pass"
// before it merges instead of trusting an agent's report.
package verify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/process"
)

// ErrFailed reports a verify command that exited non-zero, timed out or could
// not start, or a head that could not be checked out.
var ErrFailed = errors.New("verify: command did not pass")

const (
	// outputTailBytes bounds how much command output an error carries.
	outputTailBytes = 4096
	// fetchTimeout bounds fetching a PR head that is not local yet.
	fetchTimeout = 2 * time.Minute
)

type sessionStore interface {
	GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error)
}

type projectStore interface {
	GetProject(ctx context.Context, id string) (domain.ProjectRecord, bool, error)
}

// Deps are the lookups Verifier needs to find a PR's project.
type Deps struct {
	Sessions sessionStore
	Projects projectStore
	Logger   *slog.Logger
	// TempDir holds the throwaway worktrees; empty means os.TempDir().
	TempDir string
}

// Verifier implements pr.MergeVerifier.
type Verifier struct {
	sessions sessionStore
	projects projectStore
	logger   *slog.Logger
	tempDir  string
}

// New builds a Verifier.
func New(deps Deps) *Verifier {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Verifier{sessions: deps.Sessions, projects: deps.Projects, logger: logger, tempDir: deps.TempDir}
}

// VerifyMerge runs the PR's project verify command on headSHA. Projects
// without a verify command, and PRs not linked to a project, pass untouched.
func (v *Verifier) VerifyMerge(ctx context.Context, pr domain.PullRequest, headSHA string) error {
	project, ok, err := v.projectFor(ctx, pr)
	if err != nil || !ok || len(project.Config.VerifyCommand) == 0 {
		return err
	}
	if project.Kind.WithDefault() != domain.ProjectKindSingleRepo {
		return fmt.Errorf("%w: verifyCommand is only supported for single-repo projects", ErrFailed)
	}
	if err := v.ensureCommit(ctx, project.Path, pr.SourceBranch, headSHA); err != nil {
		return err
	}

	dir, cleanup, err := AddTempWorktree(ctx, project.Path, v.tempDir, headSHA, v.logger)
	if err != nil {
		return err
	}
	defer cleanup()
	return RunCommand(ctx, dir, project.Config.VerifyCommand, project.Config.VerifyTimeoutSeconds)
}

// AddTempWorktree checks ref out as a detached worktree of repo in a fresh
// directory under tempDir (empty means os.TempDir()). The returned cleanup
// removes it with a fresh context, so a cancelled request still removes its
// checkout. Errors wrap ErrFailed.
func AddTempWorktree(ctx context.Context, repo, tempDir, ref string, logger *slog.Logger) (string, func(), error) {
	if logger == nil {
		logger = slog.Default()
	}
	dir, err := os.MkdirTemp(tempDir, "ao-verify-")
	if err != nil {
		return "", nil, fmt.Errorf("%w: create worktree dir: %w", ErrFailed, err)
	}
	cleanup := func() { removeWorktree(logger, repo, dir) }
	if out, err := git(ctx, repo, "worktree", "add", "--detach", dir, ref); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("%w: check out %s: %w: %s", ErrFailed, ref, err, out)
	}
	return dir, cleanup, nil
}

func (v *Verifier) projectFor(ctx context.Context, pr domain.PullRequest) (domain.ProjectRecord, bool, error) {
	if pr.SessionID == "" || v.sessions == nil || v.projects == nil {
		return domain.ProjectRecord{}, false, nil
	}
	session, ok, err := v.sessions.GetSession(ctx, pr.SessionID)
	if err != nil {
		return domain.ProjectRecord{}, false, fmt.Errorf("%w: load session: %w", ErrFailed, err)
	}
	if !ok || session.IsStandalone() {
		if !ok {
			v.logger.Warn("pr verify: session not found; merging without project verify", "pr", pr.URL, "session", pr.SessionID)
		}
		return domain.ProjectRecord{}, false, nil
	}
	project, ok, err := v.projects.GetProject(ctx, string(session.ProjectID))
	if err != nil {
		return domain.ProjectRecord{}, false, fmt.Errorf("%w: load project: %w", ErrFailed, err)
	}
	if !ok {
		return domain.ProjectRecord{}, false, fmt.Errorf("%w: project %q not found", ErrFailed, session.ProjectID)
	}
	return project, true, nil
}

// ensureCommit makes headSHA available in the project repository. Worker
// branches usually share the project's object store already; otherwise the
// PR's source branch is fetched from origin.
func (v *Verifier) ensureCommit(ctx context.Context, repo, branch, sha string) error {
	if _, err := git(ctx, repo, "cat-file", "-e", sha+"^{commit}"); err == nil {
		return nil
	}
	if branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, ": \t\n") {
		return fmt.Errorf("%w: head %s is not in the local repository", ErrFailed, sha)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	if out, err := git(fetchCtx, repo, "fetch", "--no-tags", "origin", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("%w: fetch %s: %w: %s", ErrFailed, branch, err, out)
	}
	if _, err := git(ctx, repo, "cat-file", "-e", sha+"^{commit}"); err != nil {
		return fmt.Errorf("%w: head %s not found after fetching %s", ErrFailed, sha, branch)
	}
	return nil
}

// RunCommand runs argv in dir, bounded by timeoutSeconds (0 or less means
// domain.DefaultVerifyTimeoutSeconds). A failure wraps ErrFailed and carries
// the tail of the command's combined output.
func RunCommand(ctx context.Context, dir string, argv []string, timeoutSeconds int) error {
	if len(argv) == 0 {
		return fmt.Errorf("%w: empty command", ErrFailed)
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = domain.DefaultVerifyTimeoutSeconds * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := process.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = dir
	process.ConfigureTreeCancellation(cmd)
	cmd.WaitDelay = 5 * time.Second
	var out tailBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	switch {
	case err == nil:
		return nil
	case runCtx.Err() == context.DeadlineExceeded:
		return fmt.Errorf("%w: %s timed out after %s: %s", ErrFailed, argv[0], timeout, out.String())
	default:
		return fmt.Errorf("%w: %s: %w: %s", ErrFailed, argv[0], err, out.String())
	}
}

// removeWorktree cleans up with a fresh context so a cancelled merge request
// still removes its checkout.
func removeWorktree(logger *slog.Logger, repo, dir string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if out, err := git(ctx, repo, "worktree", "remove", "--force", dir); err != nil {
		logger.Warn("verify: worktree remove failed", "dir", dir, "err", err, "output", out)
	}
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("verify: remove dir failed", "dir", dir, "err", err)
	}
	_, _ = git(ctx, repo, "worktree", "prune")
}

func git(ctx context.Context, repo string, args ...string) (string, error) {
	out, err := gitCommand(ctx, repo, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// gitCommand never lets Git ask for credentials: the daemon has no one to
// answer a terminal or credential-manager prompt, so a fetch must fail fast.
func gitCommand(ctx context.Context, repo string, args ...string) *exec.Cmd {
	cmd := process.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	return cmd
}

// tailBuffer keeps the last outputTailBytes written to it.
type tailBuffer struct{ buf []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - outputTailBytes; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return strings.TrimSpace(string(t.buf)) }
