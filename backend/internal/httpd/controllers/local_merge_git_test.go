//go:build !windows

package controllers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/localmerge"
)

type mergeGitStore struct {
	project  domain.ProjectRecord
	sessions map[domain.SessionID]domain.SessionRecord
}

func (s *mergeGitStore) GetSession(_ context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	v, ok := s.sessions[id]
	return v, ok, nil
}
func (s *mergeGitStore) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	return s.project, id == s.project.ID, nil
}
func (s *mergeGitStore) ListSessions(_ context.Context, _ domain.ProjectID) ([]domain.SessionRecord, error) {
	return nil, nil
}

func TestLocalMergeHTTPWithRealGit(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		conflict   bool
		status     int
	}{
		{"worker forbidden", `{"callerSessionId":"w1"}`, false, 403},
		{"orchestrator allowed", `{"callerSessionId":"manager"}`, false, 200},
		{"no session allowed", `{}`, false, 200},
		{"conflict", `{"callerSessionId":"manager"}`, true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, worker := filepath.Join(t.TempDir(), "repo"), filepath.Join(t.TempDir(), "worker")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			git := func(dir string, args ...string) string {
				t.Helper()
				out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			commit := func(dir, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				git(dir, "add", "file.txt")
				git(dir, "commit", "-qm", "change")
			}
			git(repo, "init", "-qb", "main")
			git(repo, "config", "user.name", "AO Test")
			git(repo, "config", "user.email", "ao@test.invalid")
			commit(repo, "base\n")
			git(repo, "worktree", "add", "-qb", "worker", worker, "main")
			commit(worker, "worker\n")
			if tc.conflict {
				commit(repo, "main\n")
			}
			before := git(repo, "rev-parse", "main")
			store := &mergeGitStore{
				project: domain.ProjectRecord{ID: "p1", Path: repo, Kind: domain.ProjectKindSingleRepo, Config: domain.ProjectConfig{DefaultBranch: "main"}},
				sessions: map[domain.SessionID]domain.SessionRecord{
					"w1":      {ID: "w1", ProjectID: "p1", Kind: domain.KindWorker, Metadata: domain.SessionMetadata{Branch: "worker", WorkspacePath: worker}},
					"manager": {ID: "manager", ProjectID: "p1", Kind: domain.KindOrchestrator},
				},
			}
			svc := localmerge.New(localmerge.Deps{Store: store, TempDir: t.TempDir()})
			rec := serveLocalMerge(t, svc, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d: %s", rec.Code, tc.status, rec.Body)
			}
			after := git(repo, "rev-parse", "main")
			if tc.status != 200 && after != before {
				t.Fatal("base moved on refusal")
			}
			if tc.status == 200 && after == before {
				t.Fatal("base did not advance")
			}
			if tc.conflict && !strings.Contains(rec.Body.String(), `"conflictingFiles":["file.txt"]`) {
				t.Fatalf("missing conflicting paths: %s", rec.Body)
			}
		})
	}
}
