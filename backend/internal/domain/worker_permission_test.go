package domain

import "testing"

func TestWorkerLaunchPermission(t *testing.T) {
	for _, harness := range AllHarnesses {
		t.Run(string(harness), func(t *testing.T) {
			if got := WorkerLaunchPermission(KindWorker, PermissionModeBypassPermissions); got != PermissionModeAcceptEdits {
				t.Fatalf("worker bypass = %q, want accept-edits", got)
			}
			if got := WorkerLaunchPermission(KindOrchestrator, PermissionModeBypassPermissions); got != PermissionModeBypassPermissions {
				t.Fatalf("orchestrator bypass = %q", got)
			}
			if got := WorkerLaunchPermission("", PermissionModeBypassPermissions); got != PermissionModeBypassPermissions {
				t.Fatalf("unspecified kind bypass = %q", got)
			}
			if got := WorkerLaunchPermission(KindWorker, PermissionModeAuto); got != PermissionModeAuto {
				t.Fatalf("worker auto = %q", got)
			}
		})
	}
}
