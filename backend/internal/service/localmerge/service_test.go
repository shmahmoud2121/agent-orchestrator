//go:build !windows

package localmerge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type fakeStore struct {
	mu       sync.Mutex
	sessions map[domain.SessionID]domain.SessionRecord
	project  domain.ProjectRecord
}

func (f *fakeStore) GetSession(_ context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	return s, ok, nil
}

func (f *fakeStore) ListSessions(_ context.Context, project domain.ProjectID) ([]domain.SessionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.SessionRecord
	for _, s := range f.sessions {
		if s.ProjectID == project {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	return f.project, f.project.ID == id, nil
}

type sentMessage struct {
	id  domain.SessionID
	msg string
}

type fakeMessenger struct {
	mu   sync.Mutex
	sent []sentMessage
}

func (m *fakeMessenger) Send(_ context.Context, id domain.SessionID, message string, _ *ports.SpawnAttachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMessage{id: id, msg: message})
	return nil
}

func (m *fakeMessenger) ids() []domain.SessionID {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.SessionID
	for _, s := range m.sent {
		out = append(out, s.id)
	}
	return out
}

// isolateGit keeps the developer's global and system git config (identity,
// hooks, default branch) out of the test.
func isolateGit(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()
	writeFile(t, dir, name, content)
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-q", "-m", msg)
	return runGit(t, dir, "rev-parse", "HEAD")
}

type fixture struct {
	t         *testing.T
	root      string
	repo      string
	store     *fakeStore
	messenger *fakeMessenger
	svc       *Service
}

// newFixture makes a project repo whose main checkout has main checked out.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	repo := filepath.Join(root, "main")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "config", "user.name", "AO Test")
	runGit(t, repo, "config", "user.email", "ao@test.invalid")
	commitFile(t, repo, "shared.txt", "base\n", "initial")
	store := &fakeStore{
		sessions: map[domain.SessionID]domain.SessionRecord{},
		project:  domain.ProjectRecord{ID: "p1", Path: repo, Kind: domain.ProjectKindSingleRepo, Config: domain.ProjectConfig{DefaultBranch: "main"}},
	}
	store.sessions["p1-orchestrator"] = domain.SessionRecord{ID: "p1-orchestrator", ProjectID: "p1", Kind: domain.KindOrchestrator,
		Metadata: domain.SessionMetadata{Branch: "ao/p1-orchestrator", WorkspacePath: repo}}
	messenger := &fakeMessenger{}
	svc := New(Deps{Store: store, Messenger: messenger, TempDir: t.TempDir()})
	return &fixture{t: t, root: root, repo: repo, store: store, messenger: messenger, svc: svc}
}

// addWorker creates a worker session with its own worktree and branch off main.
func (f *fixture) addWorker(id string) string {
	f.t.Helper()
	branch := "ao/" + id + "/root"
	dir := filepath.Join(f.root, id)
	runGit(f.t, f.repo, "worktree", "add", "-q", "-b", branch, dir, "main")
	f.store.mu.Lock()
	f.store.sessions[domain.SessionID(id)] = domain.SessionRecord{ID: domain.SessionID(id), ProjectID: "p1", Kind: domain.KindWorker, DisplayName: "task " + id,
		Metadata: domain.SessionMetadata{Branch: branch, WorkspacePath: dir}}
	f.store.mu.Unlock()
	return dir
}

func (f *fixture) mainTip() string {
	return runGit(f.t, f.repo, "rev-parse", "refs/heads/main")
}

func (f *fixture) assertNoTempWorktrees() {
	f.t.Helper()
	list := runGit(f.t, f.repo, "worktree", "list", "--porcelain")
	if strings.Contains(list, "ao-verify-") {
		f.t.Fatalf("throwaway worktree left behind:\n%s", list)
	}
}

func requireCode(t *testing.T, err error, code string) *apierr.Error {
	t.Helper()
	var e *apierr.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want apierr %s", err, code)
	}
	if e.Code != code {
		t.Fatalf("code = %s (%s), want %s", e.Code, e.Message, code)
	}
	return e
}

func TestMergeAdvancesBaseWithMergeCommitViaCompareAndSwap(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	workerTip := commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	// Nothing has main checked out, so the ref is advanced with update-ref.
	runGit(t, f.repo, "checkout", "-q", "--detach")
	oldTip := f.mainTip()

	res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if res.MergeCommit == "" || res.MergeCommit != f.mainTip() {
		t.Fatalf("main = %s, result merge = %s", f.mainTip(), res.MergeCommit)
	}
	parents := strings.Fields(runGit(t, f.repo, "rev-list", "--parents", "-n1", res.MergeCommit))
	if len(parents) != 3 || parents[1] != oldTip || parents[2] != workerTip {
		t.Fatalf("merge parents = %v, want [merge %s %s]", parents, oldTip, workerTip)
	}
	if subject := runGit(t, f.repo, "log", "-1", "--format=%s", res.MergeCommit); subject != "Merge w1 (ao/w1/root): task w1" {
		t.Fatalf("subject = %q", subject)
	}
	if author := runGit(t, f.repo, "log", "-1", "--format=%an <%ae>", res.MergeCommit); author != "AO Test <ao@test.invalid>" {
		t.Fatalf("author = %q", author)
	}
	if res.BaseBranch != "main" || res.BaseBefore != oldTip || res.CommitCount != 1 || res.UpdatedCheckout != "" {
		t.Fatalf("result = %+v", res)
	}
	if len(res.ChangedFiles) != 1 || res.ChangedFiles[0] != "feature.txt" || res.ChangedFilesTotal != 1 {
		t.Fatalf("changed files = %v (%d)", res.ChangedFiles, res.ChangedFilesTotal)
	}
	if got := runGit(t, w1, "rev-parse", "HEAD"); got != workerTip {
		t.Fatalf("worker branch rewritten: %s != %s", got, workerTip)
	}
	f.assertNoTempWorktrees()
}

func TestMergeFastForwardsCleanMainCheckout(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	writeFile(t, f.repo, "scratch.local", "untracked files in the main checkout are left alone\n")

	res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if res.UpdatedCheckout != f.repo {
		t.Fatalf("UpdatedCheckout = %q, want %q", res.UpdatedCheckout, f.repo)
	}
	if head := runGit(t, f.repo, "rev-parse", "HEAD"); head != res.MergeCommit {
		t.Fatalf("main checkout HEAD = %s, want %s", head, res.MergeCommit)
	}
	if data, err := os.ReadFile(filepath.Join(f.repo, "feature.txt")); err != nil || string(data) != "feature\n" {
		t.Fatalf("main checkout working tree not updated: %q %v", data, err)
	}
	if status := runGit(t, f.repo, "status", "--porcelain", "--untracked-files=no"); status != "" {
		t.Fatalf("main checkout dirty after merge:\n%s", status)
	}
}

func TestMergeRefusesDirtyMainCheckout(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	writeFile(t, f.repo, "shared.txt", "user edit in progress\n")
	before := f.mainTip()

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	e := requireCode(t, err, CodeBaseCheckoutDirty)
	if !strings.Contains(e.Message, "shared.txt") {
		t.Fatalf("message does not name the dirty path: %s", e.Message)
	}
	if f.mainTip() != before {
		t.Fatal("main moved despite a dirty main checkout")
	}
	if data, _ := os.ReadFile(filepath.Join(f.repo, "shared.txt")); string(data) != "user edit in progress\n" {
		t.Fatalf("user's edit was touched: %q", data)
	}
	f.assertNoTempWorktrees()
}

func TestMergeRefusesDirtyWorkerWorktree(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	writeFile(t, w1, "notes/untracked.txt", "wip\n")
	before := f.mainTip()

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	e := requireCode(t, err, CodeWorkerDirty)
	if !strings.Contains(e.Message, "commit") || !strings.Contains(e.Message, "notes/untracked.txt") {
		t.Fatalf("message = %s", e.Message)
	}
	if f.mainTip() != before {
		t.Fatal("main moved")
	}
}

func TestMergeRefusesNothingToMerge(t *testing.T) {
	f := newFixture(t)
	f.addWorker("w1")
	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	requireCode(t, err, CodeNothingToMerge)
	f.assertNoTempWorktrees()
}

func TestMergeConflictReturnsPathsAndLeavesBaseUnchanged(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "shared.txt", "worker version\n", "feat: worker edit")
	commitFile(t, f.repo, "shared.txt", "main version\n", "feat: main edit")
	before := f.mainTip()

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	e := requireCode(t, err, CodeConflict)
	files, _ := e.Details["conflictingFiles"].([]string)
	if len(files) != 1 || files[0] != "shared.txt" {
		t.Fatalf("conflictingFiles = %#v", e.Details["conflictingFiles"])
	}
	if instr, _ := e.Details["instruction"].(string); !strings.Contains(instr, "git merge main") {
		t.Fatalf("instruction = %q", instr)
	}
	if f.mainTip() != before {
		t.Fatal("main moved after a conflict")
	}
	if status := runGit(t, f.repo, "status", "--porcelain"); status != "" {
		t.Fatalf("main checkout changed:\n%s", status)
	}
	f.assertNoTempWorktrees()
}

func TestMergeVerifyCommand(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "status", "broken\n", "feat: break it")
	// The check runs on the merge result, where status says broken.
	f.store.project.Config.VerifyCommand = []string{"sh", "-c", `grep -qx ok status || { echo "status: $(cat status)"; exit 1; }`}
	before := f.mainTip()

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	e := requireCode(t, err, CodeVerifyFailed)
	if !strings.Contains(e.Message, "status: broken") {
		t.Fatalf("message lacks command output tail: %s", e.Message)
	}
	if f.mainTip() != before {
		t.Fatal("main moved after a failed verify")
	}
	f.assertNoTempWorktrees()

	commitFile(t, w1, "status", "ok\n", "fix: repair it")
	res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	if err != nil {
		t.Fatalf("Merge after fix: %v", err)
	}
	if !res.Verified || f.mainTip() != res.MergeCommit {
		t.Fatalf("result = %+v, main = %s", res, f.mainTip())
	}
}

func TestConcurrentMergesSerializeAndBothLand(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	w2 := f.addWorker("w2")
	commitFile(t, w1, "one.txt", "1\n", "feat: one")
	commitFile(t, w2, "two.txt", "2\n", "feat: two")
	start := f.mainTip()

	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	for i, id := range []domain.SessionID{"w1", "w2"} {
		wg.Add(1)
		go func(i int, id domain.SessionID) {
			defer wg.Done()
			results[i], errs[i] = f.svc.Merge(context.Background(), Request{SessionID: id})
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("merge %d: %v", i, err)
		}
	}
	first, second := results[0], results[1]
	if second.BaseBefore == start {
		first, second = second, first
	}
	if first.BaseBefore != start || second.BaseBefore != first.MergeCommit || f.mainTip() != second.MergeCommit {
		t.Fatalf("merges did not stack: start=%s first=%+v second=%+v main=%s", start, first, second, f.mainTip())
	}
	for _, name := range []string{"one.txt", "two.txt"} {
		if _, err := os.Stat(filepath.Join(f.repo, name)); err != nil {
			t.Fatalf("%s missing from main checkout: %v", name, err)
		}
	}
}

func TestMergeNotifiesBehindWorkersAndReportsOverlaps(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	w2 := f.addWorker("w2")
	w3 := f.addWorker("w3")
	f.addWorker("w4")
	commitFile(t, w1, "shared.txt", "w1\n", "feat: w1 edits shared")
	commitFile(t, w2, "shared.txt", "w2\n", "feat: w2 also edits shared")
	writeFile(t, w3, "shared.txt", "w3 uncommitted\n")
	f.store.mu.Lock()
	w4 := f.store.sessions["w4"]
	w4.IsTerminated = true
	f.store.sessions["w4"] = w4
	f.store.mu.Unlock()

	res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	overlapping := map[domain.SessionID]bool{}
	for _, o := range res.Overlaps {
		if len(o.Files) != 1 || o.Files[0] != "shared.txt" {
			t.Fatalf("overlap %+v", o)
		}
		overlapping[o.SessionID] = true
	}
	if !overlapping["w2"] || !overlapping["w3"] || len(overlapping) != 2 {
		t.Fatalf("overlaps = %+v, want w2 and w3", res.Overlaps)
	}
	notified := map[domain.SessionID]bool{}
	for _, id := range f.messenger.ids() {
		notified[id] = true
	}
	if !notified["w2"] || !notified["w3"] || notified["w4"] || notified["w1"] || notified["p1-orchestrator"] || len(notified) != 2 {
		t.Fatalf("notified = %v, want w2 and w3 only", f.messenger.ids())
	}
	msg := f.messenger.sent[0].msg
	for _, want := range []string{"main moved", "w1", res.MergeCommit[:12], "git merge main", "shared.txt"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("note missing %q: %s", want, msg)
		}
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	f.addWorker("w2")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	before := f.mainTip()
	refsBefore := runGit(t, f.repo, "for-each-ref")

	res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !res.DryRun || res.MergeCommit != "" || res.CommitCount != 1 || res.BaseBefore != before || len(res.ChangedFiles) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if f.mainTip() != before || runGit(t, f.repo, "for-each-ref") != refsBefore {
		t.Fatal("dry run moved a ref")
	}
	if len(f.messenger.ids()) != 0 {
		t.Fatalf("dry run messaged sessions: %v", f.messenger.ids())
	}
	if _, err := os.Stat(filepath.Join(f.repo, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("dry run touched the main checkout: %v", err)
	}
	f.assertNoTempWorktrees()

	commitFile(t, f.repo, "feature.txt", "main\n", "feat: main adds the same file")
	_, err = f.svc.Merge(context.Background(), Request{SessionID: "w1", DryRun: true})
	requireCode(t, err, CodeConflict)
	f.assertNoTempWorktrees()
}

func TestMergeRefusesUnsupportedSessions(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Merge(context.Background(), Request{SessionID: "p1-orchestrator"})
	requireCode(t, err, CodeUnsupported)

	_, err = f.svc.Merge(context.Background(), Request{SessionID: "missing"})
	requireCode(t, err, "SESSION_NOT_FOUND")

	f.addWorker("w1")
	f.store.project.Kind = domain.ProjectKindWorkspace
	_, err = f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	e := requireCode(t, err, CodeUnsupported)
	if !strings.Contains(e.Message, "single-repository") {
		t.Fatalf("message = %s", e.Message)
	}
}

func TestMergeRequiresConfiguredGitIdentity(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	runGit(t, f.repo, "config", "--unset", "user.name")
	runGit(t, f.repo, "config", "--unset", "user.email")
	before := f.mainTip()

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	requireCode(t, err, CodeIdentityMissing)
	if f.mainTip() != before {
		t.Fatal("main moved without an identity")
	}
}

// moveBaseDuringVerify makes the verify command commit on main in checkout
// once (or every time), standing in for someone advancing the base mid-merge.
func moveBaseDuringVerify(f *fixture, checkout string, everyTime bool) {
	marker := filepath.Join(f.t.TempDir(), "moved")
	script := `[ -f "$1" ] || { touch "$1"; git -C "$2" commit -q --allow-empty -m "concurrent base change"; }`
	if everyTime {
		script = `git -C "$2" commit -q --allow-empty -m "concurrent base change"`
	}
	f.store.project.Config.VerifyCommand = []string{"sh", "-c", script, "sh", marker, checkout}
}

func TestMergeRetriesOnceWhenBaseMoves(t *testing.T) {
	for _, detached := range []bool{false, true} {
		name := "checked-out"
		if detached {
			name = "compare-and-swap"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			w1 := f.addWorker("w1")
			commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
			mover := f.repo
			if detached {
				// Move main through a throwaway checkout, then detach it again
				// so no worktree has main checked out at merge time.
				runGit(t, f.repo, "checkout", "-q", "--detach")
				mover = filepath.Join(f.root, "mover")
				runGit(t, f.repo, "worktree", "add", "-q", "--detach", mover, "main")
				f.store.project.Config.VerifyCommand = []string{"sh", "-c",
					`[ -f "$1" ] || { touch "$1"; git -C "$2" commit -q --allow-empty -m "concurrent base change" && git -C "$2" update-ref refs/heads/main HEAD; }`,
					"sh", filepath.Join(t.TempDir(), "moved"), mover}
			} else {
				moveBaseDuringVerify(f, mover, false)
			}
			start := f.mainTip()

			res, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			moved := runGit(t, f.repo, "rev-parse", res.MergeCommit+"^1")
			if moved == start || runGit(t, f.repo, "log", "-1", "--format=%s", moved) != "concurrent base change" {
				t.Fatalf("merge was not recomputed on the moved base: first parent %s", moved)
			}
			if f.mainTip() != res.MergeCommit {
				t.Fatalf("main = %s, want %s", f.mainTip(), res.MergeCommit)
			}
		})
	}
}

func TestMergeReportsBaseThatKeepsMoving(t *testing.T) {
	f := newFixture(t)
	w1 := f.addWorker("w1")
	commitFile(t, w1, "feature.txt", "feature\n", "feat: feature")
	moveBaseDuringVerify(f, f.repo, true)

	_, err := f.svc.Merge(context.Background(), Request{SessionID: "w1"})
	requireCode(t, err, CodeBaseMoved)
	if _, err := os.Stat(filepath.Join(f.repo, "feature.txt")); !os.IsNotExist(err) {
		t.Fatal("feature landed although the merge reported failure")
	}
	f.assertNoTempWorktrees()
}
