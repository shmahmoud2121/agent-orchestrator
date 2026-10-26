package domain

import "testing"

func TestWorkerMeetsSandboxRequirement(t *testing.T) {
	off := false
	on := true
	cases := []struct {
		name        string
		kind        SessionKind
		harness     AgentHarness
		sandbox     WorkerSandboxConfig
		osAvailable bool
		want        bool
	}{
		{name: "claude worker default", kind: KindWorker, harness: HarnessClaudeCode, osAvailable: true, want: true},
		{name: "codex worker default", kind: KindWorker, harness: HarnessCodex, osAvailable: true, want: false},
		{name: "codex worker allowed off", kind: KindWorker, harness: HarnessCodex, sandbox: WorkerSandboxConfig{RequireSandbox: &off}, osAvailable: true, want: true},
		{name: "claude sandbox disabled", kind: KindWorker, harness: HarnessClaudeCode, sandbox: WorkerSandboxConfig{Enabled: &off}, osAvailable: true, want: false},
		{name: "claude os unavailable", kind: KindWorker, harness: HarnessClaudeCode, osAvailable: false, want: false},
		{name: "manager never refused", kind: KindOrchestrator, harness: HarnessCodex, osAvailable: false, want: true},
		{name: "explicit require on", kind: KindWorker, harness: HarnessClaudeCode, sandbox: WorkerSandboxConfig{RequireSandbox: &on, Enabled: &on}, osAvailable: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkerMeetsSandboxRequirement(tc.kind, tc.harness, tc.sandbox, tc.osAvailable); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRefuseUnsandboxedWorkerRestoreWording(t *testing.T) {
	err := RefuseUnsandboxedWorker(KindWorker, HarnessCodex, WorkerSandboxConfig{}, true, true)
	if err == nil || err.Error() != ErrWorkerSandboxRequired.Error()+": "+WorkerSandboxRequiredRestoreMessage {
		t.Fatalf("restore err = %v", err)
	}
}
