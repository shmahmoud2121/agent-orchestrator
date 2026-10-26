package project_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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
	teamOff := false
	rules := domain.ProjectConfig{OrchestratorRules: "be careful"}
	teamMode := domain.ProjectConfig{TeamMode: &teamOff}
	perms := domain.ProjectConfig{AgentConfig: domain.AgentConfig{Permissions: domain.PermissionModeBypassPermissions}}
	sandbox := domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{Enabled: &enabled}}
	requireOff := false
	requireSandbox := domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{RequireSandbox: &requireOff}}

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
		{name: "person require sandbox", config: requireSandbox},
		{name: "worker require sandbox", caller: "worker-1", config: requireSandbox, wantErr: true},
		{name: "orchestrator require sandbox", caller: "orch-1", config: requireSandbox, wantErr: true},
		{name: "person team mode", config: teamMode},
		{name: "worker team mode", caller: "worker-1", config: teamMode, wantErr: true},
		{name: "orchestrator team mode", caller: "orch-1", config: teamMode, wantErr: true},
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

func TestProtectedPathsCallerGuard(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	catalog := catalogSessions{
		"worker-1": {ID: "worker-1", ProjectID: "proj", Kind: domain.KindWorker},
		"orch-1":   {ID: "orch-1", ProjectID: "proj", Kind: domain.KindOrchestrator},
	}
	svc := project.NewWithDeps(project.Deps{Store: store, SessionCatalog: catalog})
	protected := func(paths ...string) domain.ProjectConfig {
		return domain.ProjectConfig{WorkerSandbox: domain.WorkerSandboxConfig{ProtectedPaths: paths}}
	}
	reset := func() {
		t.Helper()
		if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "proj", DisplayName: "Proj", Config: protected("config/prod.yml")}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, caller string
		config       domain.ProjectConfig
		wantMessage  string
	}{
		{name: "manager adds", caller: "orch-1", config: protected("config/prod.yml", "exports/")},
		{name: "manager removes", caller: "orch-1", config: protected("exports/"),
			wantMessage: "Only the person can remove a protected file. You are the manager: you may add protected files, not remove them. Ask the person in your report."},
		{name: "manager clears", caller: "orch-1", config: domain.ProjectConfig{},
			wantMessage: "Only the person can remove a protected file."},
		{name: "worker adds", caller: "worker-1", config: protected("config/prod.yml", "exports/"),
			wantMessage: "You are a worker"},
		{name: "person removes", config: protected()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			_, err := svc.SetConfig(ctx, "proj", project.SetConfigInput{Config: tc.config, CallerSessionID: tc.caller})
			if tc.wantMessage == "" {
				if err != nil {
					t.Fatalf("SetConfig = %v", err)
				}
				return
			}
			var api *apierr.Error
			if !errors.As(err, &api) || api.Kind != apierr.KindForbidden || api.Code != "PROJECT_SETTINGS_FORBIDDEN" {
				t.Fatalf("SetConfig = %v, want 403 PROJECT_SETTINGS_FORBIDDEN", err)
			}
			if !strings.Contains(api.Message, tc.wantMessage) {
				t.Fatalf("message = %q, want %q", api.Message, tc.wantMessage)
			}
		})
	}
}

func TestWorkerEnvKeepCallerGuard(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := project.NewWithDeps(project.Deps{Store: store, SessionCatalog: catalogSessions{
		"worker-1": {ID: "worker-1", ProjectID: "proj", Kind: domain.KindWorker},
		"orch-1":   {ID: "orch-1", ProjectID: "proj", Kind: domain.KindOrchestrator},
	}})
	keep := domain.ProjectConfig{WorkerEnvKeep: []string{"NPM_TOKEN"}}
	for _, tc := range []struct {
		caller, want string
	}{
		{caller: "orch-1", want: "Only the person can change which secret variables workers keep. You are the manager"},
		{caller: "worker-1", want: "Only the person can change which secret variables workers keep. You are a worker"},
		{caller: ""},
	} {
		if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "proj", DisplayName: "Proj"}); err != nil {
			t.Fatal(err)
		}
		_, err := svc.SetConfig(ctx, "proj", project.SetConfigInput{Config: keep, CallerSessionID: tc.caller})
		if tc.want == "" {
			if err != nil {
				t.Fatalf("person SetConfig = %v", err)
			}
			continue
		}
		var api *apierr.Error
		if !errors.As(err, &api) || api.Kind != apierr.KindForbidden || !strings.Contains(api.Message, tc.want) {
			t.Fatalf("%s SetConfig = %v, want %q", tc.caller, err, tc.want)
		}
	}
}

func TestProtectedPathsMustNotCoverGit(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join(t.TempDir(), "proj")
	if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "proj", DisplayName: "Proj", Path: path}); err != nil {
		t.Fatal(err)
	}
	svc := project.NewWithDeps(project.Deps{Store: store})
	_, err = svc.SetConfig(ctx, "proj", project.SetConfigInput{Config: domain.ProjectConfig{
		WorkerSandbox: domain.WorkerSandboxConfig{ProtectedPaths: []string{path}},
	}})
	var api *apierr.Error
	if !errors.As(err, &api) || api.Kind != apierr.KindInvalid {
		t.Fatalf("SetConfig = %v, want invalid", err)
	}
}

// TestWorkerSecretsCanaryCallerGuard is the always-on canary for who may
// change protected files: the manager may add, the manager is refused when
// it removes, workers are refused, and the person may remove.
func TestWorkerSecretsCanaryCallerGuard(t *testing.T) {
	TestProtectedPathsCallerGuard(t)
}
