package domain

import (
	"errors"
	"strings"
	"testing"
)

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
	err := RefuseUnsandboxedWorker(KindWorker, HarnessCodex, WorkerSandboxConfig{}, true, true, "")
	if err == nil || err.Error() != ErrWorkerSandboxRequired.Error()+": "+WorkerSandboxRequiredRestoreMessage {
		t.Fatalf("restore err = %v", err)
	}
}

func TestRefuseUnsandboxedWorkerWindowsUnavailable(t *testing.T) {
	err := RefuseUnsandboxedWorker(KindWorker, HarnessClaudeCode, WorkerSandboxConfig{}, false, false, WorkerSandboxRequiredWindowsMessage)
	if err == nil || !errors.Is(err, ErrWorkerSandboxRequired) {
		t.Fatalf("err = %v, want WORKER_SANDBOX_REQUIRED", err)
	}
	if err.Error() != ErrWorkerSandboxRequired.Error()+": "+WorkerSandboxRequiredWindowsMessage {
		t.Fatalf("windows spawn err = %v", err)
	}
	if !strings.Contains(err.Error(), `turn off "Only allow sandboxed workers"`) {
		t.Fatalf("windows message missing the setting sentence: %v", err)
	}
}

func TestRefuseUnsandboxedWorkerHarnessWhenOSAvailable(t *testing.T) {
	err := RefuseUnsandboxedWorker(KindWorker, HarnessCodex, WorkerSandboxConfig{}, true, false, WorkerSandboxRequiredWindowsMessage)
	if err == nil || err.Error() != ErrWorkerSandboxRequired.Error()+": "+WorkerSandboxRequiredMessage {
		t.Fatalf("harness err = %v, want the original Claude Code wording", err)
	}
}

func TestRefuseUnsandboxedWorkerRestoreWhenOSUnavailable(t *testing.T) {
	err := RefuseUnsandboxedWorker(KindWorker, HarnessClaudeCode, WorkerSandboxConfig{}, false, true, WorkerSandboxRequiredWindowsMessage)
	want := ErrWorkerSandboxRequired.Error() + ": " + WorkerSandboxRequiredRestoreLead + " " + WorkerSandboxRequiredWindowsMessage
	if err == nil || err.Error() != want {
		t.Fatalf("restore os-unavailable err = %v, want %s", err, want)
	}
}

func TestRefuseUnsandboxedWorkerOtherOSUnavailable(t *testing.T) {
	other := "Claude Code worker sandbox cannot be enforced on this system. " + WorkerSandboxRequiredSettingSentence
	err := RefuseUnsandboxedWorker(KindWorker, HarnessClaudeCode, WorkerSandboxConfig{}, false, false, other)
	if err == nil || err.Error() != ErrWorkerSandboxRequired.Error()+": "+other {
		t.Fatalf("other-os err = %v", err)
	}
}

func TestWorkerSandboxRequiredAPIMessage(t *testing.T) {
	if got := WorkerSandboxRequiredAPIMessage(nil); got != WorkerSandboxRequiredMessage {
		t.Fatalf("nil = %q", got)
	}
	wrapped := RefuseUnsandboxedWorker(KindWorker, HarnessClaudeCode, WorkerSandboxConfig{}, false, false, WorkerSandboxRequiredWindowsMessage)
	spawned := errors.New("spawn: " + wrapped.Error())
	if got := WorkerSandboxRequiredAPIMessage(spawned); got != WorkerSandboxRequiredWindowsMessage {
		t.Fatalf("spawned windows = %q", got)
	}
	restore := RefuseUnsandboxedWorker(KindWorker, HarnessClaudeCode, WorkerSandboxConfig{}, false, true, WorkerSandboxRequiredWindowsMessage)
	if got := WorkerSandboxRequiredAPIMessage(restore); got != WorkerSandboxRequiredRestoreLead+" "+WorkerSandboxRequiredWindowsMessage {
		t.Fatalf("restore = %q", got)
	}
}
