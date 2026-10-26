package review

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

func TestReviewerRepositoryTrustBeforePreparationAndLaunch(t *testing.T) {
	for _, restore := range []bool{false, true} {
		name := "spawn"
		if restore {
			name = "restore"
		}
		t.Run(name, func(t *testing.T) {
			rt := &fakeRuntime{}
			called := false
			launcher := NewLauncher(fakeReviewerResolver{}, rt, t.TempDir(), WithRepoTrustCheck(func(_ context.Context, id domain.ProjectID, path string) error {
				called = true
				if id != "p" || path != "/worker" {
					t.Fatalf("gate args: %s %s", id, path)
				}
				return apierr.Conflict("REPO_TRUST_REQUIRED", "review repository configuration", nil)
			}))
			requireOff := false
			spec := LaunchSpec{ProjectID: "p", WorkspacePath: "/worker", WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &requireOff}}
			var err error
			if restore {
				_, err = launcher.RestoreTerminal(context.Background(), spec)
			} else {
				_, err = launcher.Spawn(context.Background(), spec)
			}
			var api *apierr.Error
			if !called || !errors.As(err, &api) || api.Code != "REPO_TRUST_REQUIRED" {
				t.Fatalf("gate called=%v err=%v", called, err)
			}
		})
	}
}
