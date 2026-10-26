package session

import (
	"context"
	"errors"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestSpawnTeamMode(t *testing.T) {
	off := false
	projectOff := domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{TeamMode: &off}}
	orchestrator := domain.SessionRecord{ID: "mer-orch", ProjectID: "mer", Kind: domain.KindOrchestrator}
	worker := domain.SessionRecord{ID: "mer-worker", ProjectID: "mer", Kind: domain.KindWorker}
	otherOrchestrator := domain.SessionRecord{ID: "other-orch", ProjectID: "other", Kind: domain.KindOrchestrator}

	cases := []struct {
		name    string
		project domain.ProjectRecord
		caller  domain.SessionRecord
		setCall bool
		wantErr string
	}{
		{name: "orchestrator refused when off", project: projectOff, caller: orchestrator, setCall: true, wantErr: teamModeOffMessage},
		{name: "person allowed when off", project: projectOff},
		{name: "worker caller allowed when off", project: projectOff, caller: worker, setCall: true},
		{name: "orchestrator allowed when on", project: domain.ProjectRecord{ID: "mer"}, caller: orchestrator, setCall: true},
		{name: "other project orchestrator allowed when off", project: projectOff, caller: otherOrchestrator, setCall: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore()
			st.projects["mer"] = tc.project
			if tc.setCall {
				st.sessions[tc.caller.ID] = tc.caller
			}
			fc := &fakeCommander{spawnRecord: domain.SessionRecord{ID: "mer-9", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex}}
			svc := &Service{manager: fc, store: st}
			cfg := ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex, Prompt: "do the task"}
			if tc.setCall {
				cfg.CallerSessionID = tc.caller.ID
			}
			_, _, _, err := svc.Spawn(context.Background(), cfg)
			if tc.wantErr != "" {
				var api *apierr.Error
				if !errors.As(err, &api) || api.Kind != apierr.KindForbidden || api.Error() != tc.wantErr {
					t.Fatalf("Spawn err = %v, want forbidden %q", err, tc.wantErr)
				}
				if fc.spawned {
					t.Fatal("manager.Spawn must not run when team mode refuses the worker")
				}
				return
			}
			if err != nil {
				t.Fatalf("Spawn: %v", err)
			}
			if !fc.spawned {
				t.Fatal("manager.Spawn did not run")
			}
		})
	}
}
