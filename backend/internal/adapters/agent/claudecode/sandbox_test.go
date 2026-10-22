package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	"github.com/aoagents/agent-orchestrator/backend/pkg/agentruntime"
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
	if _, ok := sandbox["filesystem"]; ok {
		t.Fatalf("must not set filesystem.allowWrite; Claude Code handles worktree .git: %#v", sandbox)
	}
	network := sandbox["network"].(map[string]any)
	domains := asStringSlice(t, network["allowedDomains"])
	if !reflect.DeepEqual(domains, []string{"registry.npmjs.org"}) {
		t.Fatalf("allowedDomains = %#v; must not inject localhost/daemon ports", domains)
	}
	if network["strictAllowlist"] != true {
		t.Fatal("expected strictAllowlist")
	}
	perms := settings["permissions"].(map[string]any)
	if perms["defaultMode"] != "acceptEdits" {
		t.Fatalf("defaultMode = %#v", perms["defaultMode"])
	}
	if _, ok := perms["deny"]; ok {
		t.Fatalf("Edit deny rules must be absent; acceptEdits scopes cwd: %#v", perms)
	}
	allow := asStringSlice(t, perms["allow"])
	if !containsAll(allow, "Bash(ao)", "Bash(ao *)") {
		t.Fatalf("allow = %#v; want Bash(ao) and Bash(ao *)", allow)
	}
	if countFlag(cmd, "--settings") != 1 {
		t.Fatalf("expected one --settings; got %#v", cmd)
	}
	if !containsSubsequence(cmd, []string{"--permission-mode", "acceptEdits"}) {
		t.Fatalf("missing permission mode: %#v", cmd)
	}
	if contains(cmd, "--add-dir") {
		t.Fatalf("must not --add-dir main .git: %#v", cmd)
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

func TestGetLaunchCommandSandboxClaudeTooOldRefuses(t *testing.T) {
	t.Cleanup(resetSandboxAvailabilityForTest)
	forceSandboxAvailable(t)
	orig := claudeVersionCommand
	t.Cleanup(func() { claudeVersionCommand = orig })
	claudeVersionCommand = func(context.Context, string) ([]byte, error) {
		return []byte("2.1.148 (Claude Code)\n"), nil
	}
	p := &Plugin{resolvedBinary: "claude"}
	_, err := p.GetLaunchCommand(context.Background(), ports.LaunchConfig{
		Kind:          domain.KindWorker,
		WorkspacePath: t.TempDir(),
		Prompt:        "do it",
	})
	if !errors.Is(err, ErrSandboxClaudeTooOld) {
		t.Fatalf("err = %v", err)
	}
}

func TestClaudeVersionAtLeast(t *testing.T) {
	if !claudeVersionAtLeast("2.1.149", minWorktreeSandboxVersion) {
		t.Fatal("2.1.149 should pass")
	}
	if !claudeVersionAtLeast("Claude Code v2.1.233", minWorktreeSandboxVersion) {
		t.Fatal("banner form should pass")
	}
	if claudeVersionAtLeast("2.1.148", minWorktreeSandboxVersion) {
		t.Fatal("2.1.148 should fail")
	}
	if claudeVersionAtLeast("not a version", minWorktreeSandboxVersion) {
		t.Fatal("garbage should fail")
	}
}

func TestEffectiveWorkerPermissionRemapsBypass(t *testing.T) {
	if got := EffectiveWorkerPermission(domain.KindWorker, domain.WorkerSandboxConfig{}, ports.PermissionModeBypassPermissions); got != ports.PermissionModeAcceptEdits {
		t.Fatalf("sandboxed worker bypass = %q, want acceptEdits", got)
	}
	if got := EffectiveWorkerPermission(domain.KindWorker, domain.WorkerSandboxConfig{Enabled: boolPtr(false)}, ports.PermissionModeBypassPermissions); got != ports.PermissionModeBypassPermissions {
		t.Fatalf("sandbox off = %q", got)
	}
	if got := EffectiveWorkerPermission(domain.KindOrchestrator, domain.WorkerSandboxConfig{}, ports.PermissionModeBypassPermissions); got != ports.PermissionModeBypassPermissions {
		t.Fatalf("orchestrator = %q", got)
	}
}

func TestSandboxSettingsAllowAOBashRules(t *testing.T) {
	settings, err := buildWorkerSandboxSettings(agentruntime.PermissionAcceptEdits, domain.WorkerSandboxConfig{})
	if err != nil {
		t.Fatal(err)
	}
	excluded := asStringSlice(t, settings["sandbox"].(map[string]any)["excludedCommands"])
	if !containsAll(excluded, "ao", "ao *") {
		t.Fatalf("excludedCommands=%v", excluded)
	}
	allow := asStringSlice(t, settings["permissions"].(map[string]any)["allow"])
	if !containsAll(allow, "Bash(ao)", "Bash(ao *)") {
		t.Fatalf("allow=%v", allow)
	}
}

func TestOverrideSandboxAvailabilityForTest(t *testing.T) {
	restore := OverrideSandboxAvailabilityForTest(func() error {
		return errors.New("forced unavailable")
	})
	t.Cleanup(restore)
	if err := CheckSandboxAvailability(); err == nil || err.Error() != "forced unavailable" {
		t.Fatalf("got %v", err)
	}
	restore()
	forceSandboxAvailable(t)
	if err := CheckSandboxAvailability(); err != nil {
		t.Fatal(err)
	}
}

func forceSandboxAvailable(t *testing.T) {
	t.Helper()
	restoreProbe := OverrideSandboxAvailabilityForTest(func() error { return nil })
	origLook := lookPath
	origVersion := claudeVersionCommand
	t.Cleanup(func() {
		lookPath = origLook
		claudeVersionCommand = origVersion
		restoreProbe()
	})
	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	claudeVersionCommand = func(context.Context, string) ([]byte, error) {
		return []byte("2.1.233 (Claude Code)\n"), nil
	}
	if err := CheckSandboxAvailability(); err != nil {
		t.Fatal(err)
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
