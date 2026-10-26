package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestSessionSandboxedOnlyForAvailableClaudeWorker(t *testing.T) {
	restore := claudecode.OverrideSandboxAvailabilityForTest(func() error { return nil })
	t.Cleanup(restore)
	disabled := false
	cases := []struct {
		name    string
		rec     domain.SessionRecord
		project *domain.ProjectRecord
		probe   error
		want    bool
	}{
		{
			name:    "sandboxed claude worker",
			rec:     domain.SessionRecord{ID: "s", ProjectID: "p", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode},
			project: &domain.ProjectRecord{ID: "p"},
			want:    true,
		},
		{
			name:    "sandbox off",
			rec:     domain.SessionRecord{ID: "s", ProjectID: "p", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode},
			project: &domain.ProjectRecord{ID: "p", Config: domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &disabled}}},
		},
		{
			name:    "codex worker",
			rec:     domain.SessionRecord{ID: "s", ProjectID: "p", Kind: domain.KindWorker, Harness: domain.HarnessCodex},
			project: &domain.ProjectRecord{ID: "p"},
		},
		{
			name:    "orchestrator",
			rec:     domain.SessionRecord{ID: "s", ProjectID: "p", Kind: domain.KindOrchestrator, Harness: domain.HarnessClaudeCode},
			project: &domain.ProjectRecord{ID: "p"},
		},
		{
			name:    "sandbox unavailable",
			rec:     domain.SessionRecord{ID: "s", ProjectID: "p", Kind: domain.KindWorker, Harness: domain.HarnessClaudeCode},
			project: &domain.ProjectRecord{ID: "p"},
			probe:   errors.New("unavailable"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := claudecode.OverrideSandboxAvailabilityForTest(func() error { return tc.probe })
			t.Cleanup(probe)
			st := newFakeStore()
			if tc.project != nil {
				st.projects[tc.project.ID] = *tc.project
			}
			svc := &Service{store: st, clock: func() time.Time { return time.Unix(1, 0) }}
			session, err := svc.toSessionWithFacts(context.Background(), tc.rec, nil, nil, false, "")
			if err != nil {
				t.Fatal(err)
			}
			if session.Sandboxed != tc.want {
				t.Fatalf("sandboxed = %v, want %v", session.Sandboxed, tc.want)
			}
		})
	}
}
