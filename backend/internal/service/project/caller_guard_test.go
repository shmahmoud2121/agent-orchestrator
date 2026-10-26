package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/project"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

type catalogSessions map[domain.SessionID]domain.SessionRecord

func (c catalogSessions) GetSession(_ context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	rec, ok := c[id]
	return rec, ok, nil
}

func TestSessionCallerProjectWrites(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "proj", DisplayName: "Proj"}); err != nil {
		t.Fatal(err)
	}
	catalog := catalogSessions{
		"worker-1": {ID: "worker-1", ProjectID: "proj", Kind: domain.KindWorker},
		"orch-1":   {ID: "orch-1", ProjectID: "proj", Kind: domain.KindOrchestrator},
	}
	svc := project.NewWithDeps(project.Deps{Store: store, SessionCatalog: catalog})
	enabled := false
	rules := domain.ProjectConfig{OrchestratorRules: "be careful"}
	perms := domain.ProjectConfig{AgentConfig: domain.AgentConfig{Permissions: domain.PermissionModeBypassPermissions}}
	sandbox := domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled}}

	cases := []struct {
		name    string
		caller  string
		config  domain.ProjectConfig
		wantErr bool
	}{
		{name: "no header rules", config: rules},
		{name: "no header permissions", config: perms},
		{name: "worker rules", caller: "worker-1", config: rules, wantErr: true},
		{name: "worker permissions", caller: "worker-1", config: perms, wantErr: true},
		{name: "worker sandbox", caller: "worker-1", config: sandbox, wantErr: true},
		{name: "orchestrator rules", caller: "orch-1", config: rules},
		{name: "orchestrator permissions", caller: "orch-1", config: perms, wantErr: true},
		{name: "orchestrator sandbox", caller: "orch-1", config: sandbox, wantErr: true},
		{name: "unknown session", caller: "missing", config: rules, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "proj", DisplayName: "Proj"}); err != nil {
				t.Fatal(err)
			}
			_, err := svc.SetConfig(ctx, "proj", project.SetConfigInput{Config: tc.config, CallerSessionID: tc.caller})
			var api *apierr.Error
			forbidden := errors.As(err, &api) && api.Kind == apierr.KindForbidden
			if forbidden != tc.wantErr {
				t.Fatalf("SetConfig err = %v, forbidden=%v, want forbidden=%v", err, forbidden, tc.wantErr)
			}
		})
	}

	_, err = svc.SetPermissions(ctx, "proj", project.SetPermissionsInput{
		Permissions: domain.PermissionModeAuto, CallerSessionID: "orch-1",
	})
	var api *apierr.Error
	if !errors.As(err, &api) || api.Kind != apierr.KindForbidden {
		t.Fatalf("orchestrator SetPermissions = %v", err)
	}
	_, err = svc.SetPermissions(ctx, "proj", project.SetPermissionsInput{
		Permissions: domain.PermissionModeAuto, CallerSessionID: "worker-1",
	})
	if !errors.As(err, &api) || api.Kind != apierr.KindForbidden {
		t.Fatalf("worker SetPermissions = %v", err)
	}
	if _, err := svc.SetPermissions(ctx, "proj", project.SetPermissionsInput{Permissions: domain.PermissionModeAuto}); err != nil {
		t.Fatalf("person SetPermissions = %v", err)
	}

	_, err = svc.SetRepoTrust(ctx, "proj", "", "worker-1", "", true)
	if !errors.As(err, &api) || api.Kind != apierr.KindForbidden {
		t.Fatalf("worker trust = %v", err)
	}
	_, err = svc.SetRepoTrust(ctx, "proj", "", "orch-1", "", true)
	if !errors.As(err, &api) || api.Kind != apierr.KindForbidden {
		t.Fatalf("orchestrator trust = %v", err)
	}
}
