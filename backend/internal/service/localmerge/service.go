// Package localmerge merges a worker session's branch into its project's base
// branch entirely on this machine. AO's own code does the merge in a throwaway
// worktree, runs the project's verify command on the result, and only then
// advances the base branch with a compare-and-swap, so a worker's branch lands
// whole or not at all. Nothing here fetches, and the worker's branch is never
// rewritten. The only remote operation is the project's opt-in pushAfterMerge,
// which pushes the advanced base branch (never forced) after the merge landed.
package localmerge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/gitdefault"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/internal/process"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/verify"
)

// Error codes returned in the API envelope. Refusals that depend on the
// current repository state are 409s; requests that can never succeed for this
// session are 400s.
const (
	CodeUnsupported         = "MERGE_UNSUPPORTED"
	CodeBranchNotFound      = "MERGE_BRANCH_NOT_FOUND"
	CodeBaseUnresolved      = "MERGE_BASE_UNRESOLVED"
	CodeWorkerDirty         = "MERGE_WORKER_DIRTY"
	CodeNothingToMerge      = "MERGE_NOTHING_TO_MERGE"
	CodeIdentityMissing     = "MERGE_GIT_IDENTITY_MISSING"
	CodeConflict            = "MERGE_CONFLICT"
	CodeVerifyFailed        = "MERGE_VERIFY_FAILED"
	CodeBaseCheckoutDirty   = "MERGE_BASE_CHECKOUT_DIRTY"
	CodeBaseCheckoutBlocked = "MERGE_BASE_CHECKOUT_BLOCKED"
	CodeBaseMoved           = "MERGE_BASE_MOVED"
	CodeForbidden           = "MERGE_FORBIDDEN"
)

const (
	// maxListedFiles bounds file lists in responses and messages.
	maxListedFiles = 200
	// maxMessageFiles bounds the files named in a "base moved" note.
	maxMessageFiles = 20
	// notifyTimeout bounds delivering one "base moved" note.
	notifyTimeout = 30 * time.Second
	// pushTimeout bounds the optional push after merge.
	pushTimeout = 2 * time.Minute
)

// errBaseMoved means the base branch changed between reading its tip and
// advancing it; the merge is retried once from a fresh base.
var errBaseMoved = errors.New("base branch moved during merge")

// Store is the read side the service needs.
type Store interface {
	GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error)
	ListSessions(ctx context.Context, project domain.ProjectID) ([]domain.SessionRecord, error)
	GetProject(ctx context.Context, id string) (domain.ProjectRecord, bool, error)
}

// Messenger delivers a note to a live session (the session service's Send).
type Messenger interface {
	Send(ctx context.Context, id domain.SessionID, message string, attachment *ports.SpawnAttachment) error
}

// Deps wires a Service.
type Deps struct {
	Store     Store
	Messenger Messenger
	Logger    *slog.Logger
	// TempDir holds the throwaway merge worktrees; empty means os.TempDir().
	TempDir string
}

// Service performs local merges. It is safe for concurrent use: merges into
// one repository are serialized, different repositories run in parallel.
type Service struct {
	store     Store
	messenger Messenger
	logger    *slog.Logger
	tempDir   string

	mu    sync.Mutex
	locks map[string]chan struct{}
}

// New builds a Service.
func New(deps Deps) *Service {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: deps.Store, messenger: deps.Messenger, logger: logger, tempDir: deps.TempDir, locks: map[string]chan struct{}{}}
}

// Request asks for one worker session to be merged.
type Request struct {
	SessionID domain.SessionID
	// DryRun checks everything up to and including the merge itself in a
	// throwaway worktree, then discards it: no commit lands, no ref moves, no
	// session is messaged.
	DryRun bool
	// CallerSessionID is the AO session asking for the merge, empty for a
	// person (desktop app, plain CLI). Only an orchestrator of the same
	// project may merge; worker sessions are refused. The CLI fills it from
	// AO_SESSION_ID, so this guards against agent mistakes, not against a
	// caller that deliberately lies about who it is.
	CallerSessionID domain.SessionID
}

// Overlap is another live worker whose work touches files the merged branch
// also changes. It is a warning, never a refusal.
type Overlap struct {
	SessionID domain.SessionID
	Branch    string
	Files     []string
}

// Result describes a merge (or, for a dry run, the merge that would happen).
type Result struct {
	SessionID  domain.SessionID
	Branch     string
	BaseBranch string
	DryRun     bool
	// BaseBefore is the base tip the merge was computed against.
	BaseBefore string
	// MergeCommit is the new base tip. Empty for a dry run.
	MergeCommit string
	// CommitCount is how many commits the branch brings into base.
	CommitCount int
	// ChangedFiles lists (at most maxListedFiles of) the files the branch
	// changes relative to base; ChangedFilesTotal is the full count.
	ChangedFiles      []string
	ChangedFilesTotal int
	// Verified reports that the project's verify command passed on the merge.
	Verified bool
	// UpdatedCheckout is the worktree that had the base branch checked out and
	// was fast-forwarded to the merge, if any.
	UpdatedCheckout string
	Overlaps        []Overlap
	// Notified lists live workers told that the base branch moved.
	Notified []domain.SessionID
	// Warnings collects best-effort failures (overlap scan, notifications)
	// that did not stop the merge.
	Warnings []string
	// PushAttempted is true when the project's pushAfterMerge setting asked
	// for the base branch to be pushed to origin after the merge landed.
	PushAttempted bool
	// Pushed reports that origin accepted the push of PushedRef at
	// PushedCommit. A failed push (PushError) never undoes the local merge.
	Pushed       bool
	PushedRef    string
	PushedCommit string
	PushError    string
}

type target struct {
	session  domain.SessionRecord
	project  domain.ProjectRecord
	repo     string
	branch   string
	worktree string
}

// Merge merges the session's branch into its project's base branch, or with
// DryRun reports whether it would merge cleanly. Errors are *apierr.Error.
func (s *Service) Merge(ctx context.Context, req Request) (Result, error) {
	t, err := s.load(ctx, req.SessionID)
	if err != nil {
		return Result{}, err
	}
	if err := s.authorize(ctx, t, req.CallerSessionID); err != nil {
		return Result{}, err
	}
	if t.worktree != "" {
		dirty, err := porcelain(ctx, t.worktree, true)
		if err != nil {
			return Result{}, internal("read worker worktree status", err)
		}
		if len(dirty) > 0 {
			return Result{}, apierr.Conflict(CodeWorkerDirty, fmt.Sprintf(
				"Worker %s has uncommitted or untracked changes in %s. The worker must commit (or remove) them on branch %s, leaving its worktree clean, before it can be merged. Dirty paths: %s",
				t.session.ID, t.worktree, t.branch, listForMessage(dirty, maxMessageFiles)),
				map[string]any{"worktree": t.worktree, "paths": capList(dirty)})
		}
	}
	base, err := s.resolveBase(ctx, t)
	if err != nil {
		return Result{}, err
	}
	if err := checkIdentity(ctx, t.repo); err != nil {
		return Result{}, err
	}

	unlock, err := s.lock(ctx, t.repo)
	if err != nil {
		return Result{}, internal("wait for another merge into this repository", err)
	}
	res, err := s.mergeLocked(ctx, t, base, req.DryRun)
	if err != nil {
		unlock()
		return Result{}, err
	}
	res.Overlaps, res.Warnings = s.overlaps(ctx, t, res)
	if !req.DryRun && t.project.Config.PushAfterMerge {
		// Still under the lock, so base is exactly the merge commit.
		s.push(ctx, t.repo, &res)
	}
	unlock()

	if !req.DryRun {
		notified, warnings := s.notifyBehind(ctx, t, res)
		res.Notified = notified
		res.Warnings = append(res.Warnings, warnings...)
		s.logger.Info("local merge landed", "session", t.session.ID, "branch", t.branch, "base", base, "commit", res.MergeCommit, "commits", res.CommitCount)
	}
	return res, nil
}

func (s *Service) mergeLocked(ctx context.Context, t target, base string, dryRun bool) (Result, error) {
	for attempt := 0; ; attempt++ {
		res, err := s.attempt(ctx, t, base, dryRun)
		if !errors.Is(err, errBaseMoved) {
			return res, err
		}
		if attempt > 0 {
			return Result{}, apierr.Conflict(CodeBaseMoved, fmt.Sprintf(
				"Base branch %s kept moving while %s was being merged; nothing landed. Ask for the merge again.", base, t.session.ID),
				map[string]any{"baseBranch": base, "branch": t.branch})
		}
		s.logger.Info("local merge: base moved, retrying", "session", t.session.ID, "base", base)
	}
}

// attempt merges once against the current base tip. It returns errBaseMoved
// when the compare-and-swap of the base ref loses a race.
func (s *Service) attempt(ctx context.Context, t target, base string, dryRun bool) (Result, error) {
	baseRef := "refs/heads/" + base
	oldTip, err := revParse(ctx, t.repo, baseRef)
	if err != nil {
		return Result{}, internal("read base branch tip", err)
	}
	branchTip, err := revParse(ctx, t.repo, "refs/heads/"+t.branch)
	if err != nil {
		return Result{}, apierr.Invalid(CodeBranchNotFound, fmt.Sprintf("Branch %s of session %s no longer exists in %s", t.branch, t.session.ID, t.repo), nil)
	}
	countOut, err := gitOut(ctx, t.repo, "rev-list", "--count", oldTip+".."+branchTip)
	if err != nil {
		return Result{}, internal("count commits to merge", err)
	}
	count, _ := strconv.Atoi(strings.TrimSpace(countOut))
	if count == 0 {
		return Result{}, apierr.Conflict(CodeNothingToMerge, fmt.Sprintf(
			"Nothing to merge: branch %s has no commits that are not already on %s. The worker must commit its work on its branch first.", t.branch, base),
			map[string]any{"baseBranch": base, "branch": t.branch})
	}
	files, err := nameList(ctx, t.repo, "diff", "--name-only", "-z", oldTip+"..."+branchTip)
	if err != nil {
		return Result{}, internal("list changed files", err)
	}
	res := Result{
		SessionID: t.session.ID, Branch: t.branch, BaseBranch: base, DryRun: dryRun,
		BaseBefore: oldTip, CommitCount: count, ChangedFiles: capList(files), ChangedFilesTotal: len(files),
	}

	dir, cleanup, err := verify.AddTempWorktree(ctx, t.repo, s.tempDir, oldTip, s.logger)
	if err != nil {
		return Result{}, internal("create merge worktree", err)
	}
	defer cleanup()

	args := []string{"-c", "user.useConfigOnly=true", "merge", "--no-ff", "--no-edit", "-m", mergeMessage(t, base)}
	if dryRun {
		args = append(args, "--no-commit")
	}
	if out, mergeErr := git(ctx, dir, append(args, branchTip)...); mergeErr != nil {
		conflicts, _ := nameList(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z")
		if abortOut, err := git(ctx, dir, "merge", "--abort"); err != nil {
			s.logger.Warn("local merge: merge --abort failed in throwaway worktree", "dir", dir, "err", err, "output", abortOut)
		}
		if len(conflicts) > 0 {
			return Result{}, conflictError(t, base, conflicts)
		}
		return Result{}, internal("merge "+t.branch, fmt.Errorf("%w: %s", mergeErr, out))
	}
	if dryRun {
		if out, err := git(ctx, dir, "merge", "--abort"); err != nil {
			s.logger.Warn("local merge: dry-run merge --abort failed", "dir", dir, "err", err, "output", out)
		}
		return res, nil
	}
	mergeSHA, err := revParse(ctx, dir, "HEAD")
	if err != nil {
		return Result{}, internal("read merge commit", err)
	}

	if cmd := t.project.Config.VerifyCommand; len(cmd) > 0 {
		if err := verify.RunCommand(ctx, dir, cmd, t.project.Config.VerifyTimeoutSeconds); err != nil {
			return Result{}, apierr.Conflict(CodeVerifyFailed, fmt.Sprintf(
				"The project's verify command failed on the merge of %s into %s; nothing landed. %v", t.branch, base, err),
				map[string]any{"baseBranch": base, "branch": t.branch, "reason": err.Error()})
		}
		res.Verified = true
	}

	checkout, err := baseCheckout(ctx, t.repo, baseRef)
	if err != nil {
		return Result{}, internal("list worktrees", err)
	}
	if checkout == "" {
		// Compare-and-swap: the ref only moves if nobody moved it since we read it.
		if out, err := git(ctx, t.repo, "update-ref", "-m", "ao merge: "+string(t.session.ID), baseRef, mergeSHA, oldTip); err != nil {
			if moved(ctx, t.repo, baseRef, oldTip) {
				return Result{}, errBaseMoved
			}
			return Result{}, internal("advance "+base, fmt.Errorf("%w: %s", err, out))
		}
	} else {
		if err := s.fastForwardCheckout(ctx, t, base, checkout, baseRef, oldTip, mergeSHA); err != nil {
			return Result{}, err
		}
		res.UpdatedCheckout = checkout
	}
	res.MergeCommit = mergeSHA
	return res, nil
}

// authorize refuses worker sessions: integration is the orchestrator's job.
// An empty caller is a person (desktop app or a plain terminal).
func (s *Service) authorize(ctx context.Context, t target, caller domain.SessionID) error {
	caller = domain.SessionID(strings.TrimSpace(string(caller)))
	if caller == "" {
		return nil
	}
	sess, ok, err := s.store.GetSession(ctx, caller)
	if err != nil {
		return internal("load calling session", err)
	}
	if !ok {
		return apierr.Forbidden(CodeForbidden, fmt.Sprintf("Only the orchestrator can merge: calling session %s is unknown", caller))
	}
	if sess.Kind != domain.KindOrchestrator {
		return apierr.Forbidden(CodeForbidden, fmt.Sprintf(
			"Only the orchestrator can merge. Session %s is a worker: commit your work on your own branch and report done with `ao report --done`; the orchestrator reviews and merges it.", caller))
	}
	if sess.ProjectID != t.session.ProjectID {
		return apierr.Forbidden(CodeForbidden, fmt.Sprintf("Only the orchestrator of project %s can merge %s", t.session.ProjectID, t.session.ID))
	}
	return nil
}

// push sends the advanced base branch to origin. Never forced; a rejection or
// any other failure is recorded on res and leaves the local merge in place.
func (s *Service) push(ctx context.Context, repo string, res *Result) {
	res.PushAttempted = true
	ref := "refs/heads/" + res.BaseBranch
	res.PushedRef = ref
	if _, err := gitOut(ctx, repo, "remote", "get-url", "origin"); err != nil {
		res.PushError = "pushAfterMerge is on but the repository has no origin remote"
		return
	}
	pushCtx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	out, err := git(pushCtx, repo, "push", "--porcelain", "origin", ref+":"+ref)
	if err != nil {
		res.PushError = tail(fmt.Sprintf("%v: %s", err, out))
		s.logger.Warn("local merge: push after merge failed; local merge kept", "base", res.BaseBranch, "err", err, "output", out)
		return
	}
	res.Pushed = true
	res.PushedCommit = res.MergeCommit
}

// fastForwardCheckout advances a base branch that is checked out somewhere
// (usually the user's main checkout) so its working tree follows the ref. It
// refuses a checkout with tracked changes; untracked files are left alone and
// git itself refuses to fast-forward over one the merge would overwrite.
func (s *Service) fastForwardCheckout(ctx context.Context, t target, base, checkout, baseRef, oldTip, mergeSHA string) error {
	dirty, err := porcelain(ctx, checkout, false)
	if err != nil {
		return internal("read status of "+checkout, err)
	}
	if len(dirty) > 0 {
		return apierr.Conflict(CodeBaseCheckoutDirty, fmt.Sprintf(
			"%s is checked out in %s, which has uncommitted changes; nothing landed. Commit or stash those changes there, then ask for the merge again. Changed paths: %s",
			base, checkout, listForMessage(dirty, maxMessageFiles)),
			map[string]any{"baseBranch": base, "checkout": checkout, "paths": capList(dirty)})
	}
	head, err := gitOut(ctx, checkout, "symbolic-ref", "-q", "HEAD")
	if err != nil || strings.TrimSpace(head) != baseRef {
		// The checkout switched branches since we listed it; re-plan.
		return errBaseMoved
	}
	if out, err := git(ctx, checkout, "merge", "--ff-only", "--no-edit", mergeSHA); err != nil {
		if moved(ctx, t.repo, baseRef, oldTip) {
			return errBaseMoved
		}
		return apierr.Conflict(CodeBaseCheckoutBlocked, fmt.Sprintf(
			"Could not fast-forward %s in %s to the merge; nothing landed. git said: %s", base, checkout, tail(out)),
			map[string]any{"baseBranch": base, "checkout": checkout})
	}
	return nil
}

func (s *Service) load(ctx context.Context, id domain.SessionID) (target, error) {
	if strings.TrimSpace(string(id)) == "" {
		return target{}, apierr.Invalid("SESSION_ID_REQUIRED", "Session id is required", nil)
	}
	sess, ok, err := s.store.GetSession(ctx, id)
	if err != nil {
		return target{}, internal("load session", err)
	}
	if !ok || sess.IsTaskPreparation {
		return target{}, apierr.NotFound("SESSION_NOT_FOUND", "Unknown session")
	}
	if sess.Kind == domain.KindOrchestrator {
		return target{}, unsupported("Session %s is an orchestrator; only worker sessions are merged", id)
	}
	if sess.IsStandalone() {
		return target{}, unsupported("Session %s is a standalone session with no project repository to merge into", id)
	}
	project, ok, err := s.store.GetProject(ctx, string(sess.ProjectID))
	if err != nil {
		return target{}, internal("load project", err)
	}
	if !ok {
		return target{}, apierr.NotFound("PROJECT_NOT_FOUND", "Unknown project")
	}
	switch project.Kind.WithDefault() {
	case domain.ProjectKindScratch:
		return target{}, unsupported("Project %s is a scratch project without a git repository; there is nothing to merge", project.ID)
	case domain.ProjectKindWorkspace:
		return target{}, unsupported("Project %s is a multi-repository workspace; local merge supports single-repository projects only, because it cannot yet land every child repository atomically", project.ID)
	}
	branch := strings.TrimSpace(sess.Metadata.Branch)
	if branch == "" || strings.TrimSpace(project.Path) == "" {
		return target{}, unsupported("Session %s has no git branch recorded", id)
	}
	if err := checkRefName(ctx, project.Path, branch); err != nil {
		return target{}, unsupported("Session %s has an invalid branch name %q", id, branch)
	}
	worktree := strings.TrimSpace(sess.Metadata.WorkspacePath)
	if worktree != "" {
		if info, err := os.Stat(worktree); err != nil || !info.IsDir() {
			// A torn-down worktree has no uncommitted work left to protect;
			// the branch itself is still mergeable.
			worktree = ""
		}
	}
	return target{session: sess, project: project, repo: project.Path, branch: branch, worktree: worktree}, nil
}

// resolveBase uses the project's configured default branch, else the
// repository default from local metadata only (never a network call), and
// requires a local branch of that name.
func (s *Service) resolveBase(ctx context.Context, t target) (string, error) {
	base := t.project.Config.WorktreeBaseBranch()
	if base == "" {
		resolution, err := gitdefault.New("git", nil).Inspect(ctx, t.repo)
		if err != nil || strings.TrimSpace(resolution.Branch) == "" {
			return "", apierr.Invalid(CodeBaseUnresolved, fmt.Sprintf(
				"Cannot tell which branch project %s merges into. Set it with: ao project set-config %s --default-branch <branch>", t.project.ID, t.project.ID), nil)
		}
		base = strings.TrimSpace(resolution.Branch)
	}
	if err := checkRefName(ctx, t.repo, base); err != nil {
		return "", apierr.Invalid(CodeBaseUnresolved, fmt.Sprintf("Base branch %q of project %s is not a valid branch name", base, t.project.ID), nil)
	}
	if base == t.branch {
		return "", unsupported("Session %s works directly on the base branch %s; there is nothing to merge", t.session.ID, base)
	}
	if _, err := revParse(ctx, t.repo, "refs/heads/"+base); err != nil {
		return "", apierr.Invalid(CodeBaseUnresolved, fmt.Sprintf(
			"Base branch %s has no local branch in %s. Create it locally (for example from its remote-tracking branch) or set another default branch for project %s.", base, t.repo, t.project.ID), nil)
	}
	return base, nil
}

// checkIdentity refuses to merge when git has no configured author/committer
// identity, rather than letting git invent one from the host name.
func checkIdentity(ctx context.Context, repo string) error {
	for _, v := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		if _, err := gitOut(ctx, repo, "-c", "user.useConfigOnly=true", "var", v); err != nil {
			return apierr.Conflict(CodeIdentityMissing, fmt.Sprintf(
				"Git has no configured identity for merge commits in %s. Configure one, for example: git config --global user.name \"Your Name\" && git config --global user.email you@example.com", repo), nil)
		}
	}
	return nil
}

// overlaps lists other live workers whose committed or uncommitted changes
// touch files the merged branch changes. Best effort: failures become warnings.
func (s *Service) overlaps(ctx context.Context, t target, res Result) ([]Overlap, []string) {
	others, err := s.otherWorkers(ctx, t)
	if err != nil {
		return nil, []string{"overlap check skipped: " + err.Error()}
	}
	changed := make(map[string]bool, len(res.ChangedFiles))
	for _, f := range res.ChangedFiles {
		changed[f] = true
	}
	var out []Overlap
	var warnings []string
	for _, o := range others {
		touched := map[string]bool{}
		if files, err := nameList(ctx, t.repo, "diff", "--name-only", "-z", res.BaseBefore+"...refs/heads/"+o.branch); err == nil {
			for _, f := range files {
				touched[f] = true
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("overlap check for %s: %v", o.session.ID, err))
		}
		if o.worktree != "" {
			if dirty, err := porcelain(ctx, o.worktree, true); err == nil {
				for _, f := range dirty {
					touched[f] = true
				}
			}
		}
		var shared []string
		for f := range touched {
			if changed[f] {
				shared = append(shared, f)
			}
		}
		if len(shared) > 0 {
			sort.Strings(shared)
			out = append(out, Overlap{SessionID: o.session.ID, Branch: o.branch, Files: capList(shared)})
		}
	}
	return out, warnings
}

// notifyBehind tells every other live worker whose branch does not contain
// the new base tip that base moved. Best effort.
func (s *Service) notifyBehind(ctx context.Context, t target, res Result) ([]domain.SessionID, []string) {
	if s.messenger == nil {
		return nil, nil
	}
	others, err := s.otherWorkers(ctx, t)
	if err != nil {
		return nil, []string{"base-moved notifications skipped: " + err.Error()}
	}
	short := res.MergeCommit
	if len(short) > 12 {
		short = short[:12]
	}
	msg := fmt.Sprintf("%s moved: %s (%s) was merged as %s. Merge %s into your branch in your own worktree before asking for your own merge: run `git merge %s`, resolve any conflicts, run the tests, commit, then report done again. Files changed: %s",
		res.BaseBranch, t.session.ID, t.branch, short, res.BaseBranch, res.BaseBranch, listForMessage(res.ChangedFiles, maxMessageFiles))
	var notified []domain.SessionID
	var warnings []string
	for _, o := range others {
		// Exit status 0 means the worker already contains the new base.
		if _, err := gitOut(ctx, t.repo, "merge-base", "--is-ancestor", res.MergeCommit, "refs/heads/"+o.branch); err == nil {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, notifyTimeout)
		err := s.messenger.Send(sendCtx, o.session.ID, msg, nil)
		cancel()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("notify %s: %v", o.session.ID, err))
			s.logger.Warn("local merge: base-moved note not delivered", "session", o.session.ID, "err", err)
			continue
		}
		notified = append(notified, o.session.ID)
	}
	return notified, warnings
}

// otherWorkers returns the project's other live worker sessions that have a
// branch in the same repository.
func (s *Service) otherWorkers(ctx context.Context, t target) ([]target, error) {
	sessions, err := s.store.ListSessions(ctx, t.session.ProjectID)
	if err != nil {
		return nil, err
	}
	var out []target
	for _, sess := range sessions {
		branch := strings.TrimSpace(sess.Metadata.Branch)
		if sess.ID == t.session.ID || sess.Kind != domain.KindWorker || sess.IsTerminated || sess.IsTaskPreparation || branch == "" || branch == t.branch {
			continue
		}
		if checkRefName(ctx, t.repo, branch) != nil {
			continue
		}
		if _, err := revParse(ctx, t.repo, "refs/heads/"+branch); err != nil {
			continue
		}
		worktree := strings.TrimSpace(sess.Metadata.WorkspacePath)
		if worktree != "" {
			if info, err := os.Stat(worktree); err != nil || !info.IsDir() {
				worktree = ""
			}
		}
		out = append(out, target{session: sess, project: t.project, repo: t.repo, branch: branch, worktree: worktree})
	}
	return out, nil
}

// lock serializes merges per repository. Waiting honors ctx.
func (s *Service) lock(ctx context.Context, repo string) (func(), error) {
	key := repo
	if abs, err := filepath.Abs(repo); err == nil {
		key = abs
	}
	if real, err := filepath.EvalSymlinks(key); err == nil {
		key = real
	}
	s.mu.Lock()
	ch := s.locks[key]
	if ch == nil {
		ch = make(chan struct{}, 1)
		s.locks[key] = ch
	}
	s.mu.Unlock()
	select {
	case ch <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-ch }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// baseCheckout returns the worktree (main or linked) that has baseRef checked
// out, or "" when none does.
func baseCheckout(ctx context.Context, repo, baseRef string) (string, error) {
	out, err := gitOut(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	var path string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == "branch "+baseRef:
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				return path, nil
			}
		}
	}
	return "", nil
}

func conflictError(t target, base string, files []string) error {
	instruction := fmt.Sprintf("In worker %s's own worktree, run `git merge %s`, resolve the conflicts, run the tests, commit, and ask for the merge again.", t.session.ID, base)
	return apierr.Conflict(CodeConflict, fmt.Sprintf(
		"Merging %s into %s conflicts; nothing landed. Conflicting files: %s. %s",
		t.branch, base, listForMessage(files, maxMessageFiles), instruction),
		map[string]any{"baseBranch": base, "branch": t.branch, "conflictingFiles": capList(files), "instruction": instruction})
}

func mergeMessage(t target, base string) string {
	title := strings.TrimSpace(t.session.DisplayName)
	if title == "" {
		title = firstLine(t.session.Metadata.Prompt)
	}
	msg := fmt.Sprintf("Merge %s (%s) into %s", t.session.ID, t.branch, base)
	if title != "" {
		msg = fmt.Sprintf("Merge %s (%s): %s", t.session.ID, t.branch, title)
	}
	return msg
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 72 {
		s = string(r[:72]) + "..."
	}
	return strings.TrimSpace(s)
}

func moved(ctx context.Context, repo, ref, oldTip string) bool {
	now, err := revParse(ctx, repo, ref)
	return err == nil && now != oldTip
}

func checkRefName(ctx context.Context, repo, branch string) error {
	if strings.HasPrefix(branch, "-") {
		return errors.New("branch name starts with '-'")
	}
	_, err := gitOut(ctx, repo, "check-ref-format", "--branch", branch)
	return err
}

func revParse(ctx context.Context, dir, ref string) (string, error) {
	out, err := gitOut(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// porcelain lists paths with changes in a worktree. includeUntracked decides
// whether untracked files count.
func porcelain(ctx context.Context, dir string, includeUntracked bool) ([]string, error) {
	untracked := "--untracked-files=all"
	if !includeUntracked {
		untracked = "--untracked-files=no"
	}
	out, err := gitOut(ctx, dir, "status", "--porcelain", "-z", untracked)
	if err != nil {
		return nil, err
	}
	var paths []string
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		paths = append(paths, e[3:])
		if e[0] == 'R' || e[0] == 'C' {
			i++ // rename/copy: the next entry is the source path
		}
	}
	return paths, nil
}

func nameList(ctx context.Context, dir string, args ...string) ([]string, error) {
	out, err := gitOut(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range strings.Split(out, "\x00") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

// git runs git in dir and returns trimmed combined output.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitCommand(ctx, dir, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// gitOut runs git in dir and returns stdout untrimmed; stderr goes into the error.
func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := gitCommand(ctx, dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return string(out), fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return string(out), fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

// gitCommand never lets git prompt: the daemon has no one to answer.
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := process.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	return cmd
}

func unsupported(format string, args ...any) error {
	return apierr.Invalid(CodeUnsupported, fmt.Sprintf(format, args...), nil)
}

func internal(what string, err error) error {
	return &wrapped{api: apierr.Internal("MERGE_FAILED", "Local merge failed: "+what+": "+tail(err.Error())), cause: err}
}

// wrapped keeps the cause for logs while the envelope carries the API error.
type wrapped struct {
	api   *apierr.Error
	cause error
}

func (w *wrapped) Error() string { return w.api.Message }
func (w *wrapped) Unwrap() []error {
	return []error{w.api, w.cause}
}

func capList(files []string) []string {
	if len(files) > maxListedFiles {
		return append([]string(nil), files[:maxListedFiles]...)
	}
	if files == nil {
		return []string{}
	}
	return files
}

func listForMessage(files []string, limit int) string {
	if len(files) == 0 {
		return "(none)"
	}
	if len(files) <= limit {
		return strings.Join(files, ", ")
	}
	return strings.Join(files[:limit], ", ") + fmt.Sprintf(" (+%d more)", len(files)-limit)
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	const max = 2000
	if len(s) > max {
		return "..." + s[len(s)-max:]
	}
	return s
}
