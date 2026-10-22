package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func boolPtr(v bool) *bool { return &v }

func TestProbeSandboxAvailability(t *testing.T) {
	if err := probeSandboxAvailability("darwin"); err != nil {
		t.Fatalf("darwin: %v", err)
	}
	if err := probeSandboxAvailability("windows"); !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("windows: %v", err)
	}

	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}
	if err := probeSandboxAvailability("linux"); !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("linux without deps: %v", err)
	}
	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	if err := probeSandboxAvailability("linux"); err != nil {
		t.Fatalf("linux with deps: %v", err)
	}
}

func TestGetLaunchCommandWorkerSandboxOn(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxAvailable(t)

	worktree := t.TempDir()
	initGitRepo(t, worktree)

	p := &Plugin{resolvedBinary: "claude"}
	cmd, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: worktree,
		DaemonPort:    4567,
		Permissions:   ports.PermissionModeAcceptEdits,
		Prompt:        "do it",
		WorkerSandbox: domain.WorkerSandboxConfig{
			Enabled:        boolPtr(true),
			AllowedDomains: []string{"registry.npmjs.org"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := mustSettingsJSON(t, cmd)
	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["enabled"] != true || sandbox["failIfUnavailable"] != true ||
		sandbox["allowUnsandboxedCommands"] != false || sandbox["autoAllowBashIfSandboxed"] != true {
		t.Fatalf("sandbox block = %#v", sandbox)
	}
	network := sandbox["network"].(map[string]any)
	domains := asStringSlice(t, network["allowedDomains"])
	if !containsAll(domains, "registry.npmjs.org", "127.0.0.1:4567", "localhost:4567") {
		t.Fatalf("allowedDomains = %#v", domains)
	}
	if network["strictAllowlist"] != true {
		t.Fatal("expected strictAllowlist")
	}
	perms := settings["permissions"].(map[string]any)
	if perms["defaultMode"] != "acceptEdits" {
		t.Fatalf("defaultMode = %#v", perms["defaultMode"])
	}
	deny := asStringSlice(t, perms["deny"])
	if len(deny) != 2 || deny[0] != "Edit(//**)" || !strings.HasPrefix(deny[1], "Edit(!") {
		t.Fatalf("deny = %#v", deny)
	}
	if countFlag(cmd, "--settings") != 1 {
		t.Fatalf("expected one --settings; got %#v", cmd)
	}
	if !containsSubsequence(cmd, []string{"--permission-mode", "acceptEdits"}) {
		t.Fatalf("missing permission mode: %#v", cmd)
	}
	// git-dir should be addable via --add-dir when it resolves under the worktree.
	if !contains(cmd, "--add-dir") {
		t.Fatalf("expected --add-dir for git paths; got %#v", cmd)
	}
}

func TestGetLaunchCommandWorkerSandboxOff(t *testing.T) {
	p := &Plugin{resolvedBinary: "claude"}
	cmd, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeAcceptEdits,
		Prompt:        "do it",
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: boolPtr(false)},
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := mustSettingsJSON(t, cmd)
	if _, ok := settings["sandbox"]; ok {
		t.Fatalf("sandbox must be omitted when disabled: %#v", settings)
	}
	if settings["permissions"].(map[string]any)["defaultMode"] != "acceptEdits" {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestGetLaunchCommandWorkerSandboxDefaultEnabled(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxAvailable(t)
	p := &Plugin{resolvedBinary: "claude"}
	cmd, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: t.TempDir(),
		Prompt:        "do it",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := mustSettingsJSON(t, cmd)
	if settings["sandbox"].(map[string]any)["enabled"] != true {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestGetLaunchCommandWorkerBypassRemappedUnderSandbox(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxAvailable(t)
	p := &Plugin{resolvedBinary: "claude"}
	cmd, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeBypassPermissions,
		Prompt:        "do it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsSubsequence(cmd, []string{"--permission-mode", "bypassPermissions"}) {
		t.Fatalf("bypass must not remain when sandbox is on: %#v", cmd)
	}
	if !containsSubsequence(cmd, []string{"--permission-mode", "acceptEdits"}) {
		t.Fatalf("expected acceptEdits remapping: %#v", cmd)
	}
	settings := mustSettingsJSON(t, cmd)
	if settings["permissions"].(map[string]any)["defaultMode"] != "acceptEdits" {
		t.Fatalf("settings = %#v", settings)
	}
	if settings["sandbox"].(map[string]any)["allowUnsandboxedCommands"] != false {
		t.Fatal("allowUnsandboxedCommands must stay false")
	}
}

func TestGetLaunchCommandOrchestratorAndReviewerUnchanged(t *testing.T) {
	p := &Plugin{resolvedBinary: "claude"}
	for _, kind := range []domain.SessionKind{domain.KindOrchestrator, ""} {
		cmd, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
			Kind:        kind,
			Permissions: ports.PermissionModeBypassPermissions,
			Prompt:      "do it",
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"claude",
			"--permission-mode", "bypassPermissions", "--settings", bypassNudgeGuard,
			"--", "do it",
		}
		if !reflect.DeepEqual(cmd, want) {
			t.Fatalf("kind %q\nwant %#v\n got %#v", kind, want, cmd)
		}
	}
}

func TestGetRestoreCommandCarriesSandboxSettings(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxAvailable(t)
	p := &Plugin{resolvedBinary: "claude"}
	worktree := t.TempDir()
	cmd, ok, err := p.GetRestoreCommand(context.Background(), ports.RestoreConfig{
		Kind: domain.KindWorker,
		Session: ports.SessionRef{
			ID:            "sess-1",
			WorkspacePath: worktree,
			Metadata:      map[string]string{ports.MetadataKeyAgentSessionID: "11111111-1111-4111-8111-111111111111"},
		},
		Permissions:   ports.PermissionModeAuto,
		DaemonPort:    9999,
		WorkerSandbox: domain.WorkerSandboxConfig{Enabled: boolPtr(true)},
	})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	settings := mustSettingsJSON(t, cmd)
	if settings["sandbox"].(map[string]any)["enabled"] != true {
		t.Fatalf("settings = %#v", settings)
	}
	if countFlag(cmd, "--settings") != 1 {
		t.Fatalf("expected one --settings: %#v", cmd)
	}
	if !containsSubsequence(cmd, []string{"--permission-mode", "auto"}) {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestGetLaunchCommandSandboxUnavailableRefuses(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxUnavailable(t)

	p := &Plugin{resolvedBinary: "claude"}
	_, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: t.TempDir(),
		Prompt:        "do it",
	})
	if !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "bubblewrap") && !strings.Contains(err.Error(), "Seatbelt") {
		t.Fatalf("error should be actionable: %v", err)
	}
}

func TestWorkerEditDenyRulesAbsolute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix path fixture")
	}
	got := workerEditDenyRules("/tmp/ao-worktree/sess")
	want := []string{"Edit(//**)", "Edit(!//tmp/ao-worktree/sess/**)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func forceSandboxAvailable(t *testing.T) {
	t.Helper()
	resetSandboxAvailabilityForTest()
	orig := lookPath
	t.Cleanup(func() {
		lookPath = orig
		resetSandboxAvailabilityForTest()
	})
	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	if err := CheckSandboxAvailability(); err != nil {
		// On darwin the probe ignores lookPath; on linux it needs the stub.
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			t.Skipf("sandbox not testable on %s", runtime.GOOS)
		}
		// darwin returns nil without lookPath; linux uses the stub above.
		if err != nil && runtime.GOOS == "linux" {
			t.Fatal(err)
		}
	}
}

func forceSandboxUnavailable(t *testing.T) {
	t.Helper()
	resetSandboxAvailabilityForTest()
	sandboxAvailOnce.Do(func() {
		sandboxAvailErr = probeSandboxAvailability("windows")
	})
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "ao@example.com")
	run("config", "user.name", "AO")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")
}

func mustSettingsJSON(t *testing.T, cmd []string) map[string]any {
	t.Helper()
	for i, arg := range cmd {
		if arg == "--settings" && i+1 < len(cmd) {
			var out map[string]any
			if err := json.Unmarshal([]byte(cmd[i+1]), &out); err != nil {
				t.Fatalf("settings JSON: %v\n%s", err, cmd[i+1])
			}
			return out
		}
	}
	t.Fatalf("no --settings in %#v", cmd)
	return nil
}

func asStringSlice(t *testing.T, v any) []string {
	t.Helper()
	switch raw := v.(type) {
	case []string:
		return append([]string(nil), raw...)
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("not a string: %#v", item)
			}
			out = append(out, s)
		}
		return out
	default:
		t.Fatalf("not a list: %#v", v)
		return nil
	}
}

func containsAll(have []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func countFlag(cmd []string, flag string) int {
	n := 0
	for _, arg := range cmd {
		if arg == flag {
			n++
		}
	}
	return n
}
