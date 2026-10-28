package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// watchdogDaemon answers the watchdog endpoint with decision and records what
// the hook forwarded.
type watchdogDaemon struct {
	calls     int
	toolName  string
	toolInput json.RawMessage
}

func startWatchdogDaemon(t *testing.T, decision string) *watchdogDaemon {
	t.Helper()
	t.Setenv("AO_SESSION_ID", "proj-1")
	cfg := setConfigEnv(t)
	t.Setenv("AO_PROJECT_ID", "proj")
	d := &watchdogDaemon{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/watchdog") {
			var req struct {
				ToolName  string          `json:"toolName"`
				ToolInput json.RawMessage `json:"toolInput"`
			}
			_ = json.Unmarshal(body, &req)
			d.calls++
			d.toolName, d.toolInput = req.ToolName, req.ToolInput
			if decision == "deny" {
				_, _ = io.WriteString(w, `{"ok":true,"decision":"deny","reason":"AO blocked this: .env is a protected file."}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"decision":"allow"}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)
	return d
}

func runWatchdogHook(t *testing.T, agent, event, payload string) string {
	t.Helper()
	stdout, _, err := executeCLI(t, Deps{
		In:           strings.NewReader(payload),
		ProcessAlive: func(int) bool { return true },
	}, "hooks", agent, event)
	if err != nil {
		t.Fatal(err)
	}
	return stdout
}

// firstJSON decodes the first JSON document on stdout and fails if a second
// one follows: each harness reads a single decision.
func firstJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("stdout is not JSON: %q (%v)", stdout, err)
	}
	if dec.More() {
		t.Fatalf("stdout has more than one decision: %q", stdout)
	}
	return out
}

func TestWatchdogGeminiBeforeToolDeny(t *testing.T) {
	d := startWatchdogDaemon(t, "deny")
	out := firstJSON(t, runWatchdogHook(t, "gemini", "pre-tool-use",
		`{"hook_event_name":"BeforeTool","tool_name":"read_file","tool_input":{"file_path":".env"}}`))
	if out["decision"] != "deny" || !strings.Contains(out["reason"].(string), ".env is a protected file") {
		t.Fatalf("gemini output = %v", out)
	}
	if d.toolName != "read_file" || !strings.Contains(string(d.toolInput), ".env") {
		t.Fatalf("daemon saw %q %s", d.toolName, d.toolInput)
	}
}

func TestWatchdogGeminiAllowPrintsNothing(t *testing.T) {
	startWatchdogDaemon(t, "allow")
	stdout := runWatchdogHook(t, "gemini", "pre-tool-use", `{"tool_name":"read_file","tool_input":{"file_path":"src/a.go"}}`)
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("allowed stdout = %q, want empty", stdout)
	}
}

func TestWatchdogGeminiFailClosedWhenDaemonIsDown(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "proj-1")
	setConfigEnv(t)
	t.Setenv("AO_PROJECT_ID", "proj")
	out := firstJSON(t, runWatchdogHook(t, "gemini", "pre-tool-use", `{"tool_name":"read_file","tool_input":{"file_path":".env"}}`))
	if out["decision"] != "deny" || !strings.Contains(out["reason"].(string), "isn't running") {
		t.Fatalf("output = %v", out)
	}
}

func TestWatchdogGrokUsesClaudeDeny(t *testing.T) {
	startWatchdogDaemon(t, "deny")
	stdout := runWatchdogHook(t, "grok", "pre-tool-use",
		`{"hookEventName":"pre_tool_use","hook_event_name":"PreToolUse","tool_name":"read_file","tool_input":{"target_file":".env"}}`)
	var out preToolUseHookOutput
	if err := json.NewDecoder(strings.NewReader(stdout)).Decode(&out); err != nil {
		t.Fatalf("stdout = %q (%v)", stdout, err)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("grok output = %+v", out)
	}
}

func TestWatchdogGrokTruncatedInputDenied(t *testing.T) {
	d := startWatchdogDaemon(t, "allow")
	stdout := runWatchdogHook(t, "grok", "pre-tool-use",
		`{"tool_name":"run_terminal_cmd","tool_input":"{\"command\":\"echo aaaa [truncated]","tool_input_truncated":true}`)
	var out preToolUseHookOutput
	if err := json.NewDecoder(strings.NewReader(stdout)).Decode(&out); err != nil {
		t.Fatalf("stdout = %q (%v)", stdout, err)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, "too large") {
		t.Fatalf("output = %+v", out)
	}
	if d.calls != 0 {
		t.Fatalf("truncated input reached the daemon %d times", d.calls)
	}
}

func TestWatchdogCodexStringInputStillChecked(t *testing.T) {
	d := startWatchdogDaemon(t, "allow")
	stdout := runWatchdogHook(t, "codex", "pre-tool-use", `{"tool_name":"apply_patch","tool_input":"*** Begin Patch"}`)
	if strings.TrimSpace(stdout) != "" || d.calls != 1 {
		t.Fatalf("codex stdout = %q calls = %d", stdout, d.calls)
	}
}

func TestWatchdogCursorReadFile(t *testing.T) {
	d := startWatchdogDaemon(t, "deny")
	out := firstJSON(t, runWatchdogHook(t, "cursor", "before-read-file",
		`{"hook_event_name":"beforeReadFile","file_path":"/w/.env","content":"SECRET=1"}`))
	if out["permission"] != "deny" || !strings.Contains(out["agentMessage"].(string), "protected") {
		t.Fatalf("cursor output = %v", out)
	}
	if d.toolName != "Read" || strings.Contains(string(d.toolInput), "SECRET") {
		t.Fatalf("daemon saw %q %s; file content must not be forwarded", d.toolName, d.toolInput)
	}
}

func TestWatchdogCursorReadFileAllowed(t *testing.T) {
	startWatchdogDaemon(t, "allow")
	out := firstJSON(t, runWatchdogHook(t, "cursor", "before-read-file", `{"file_path":"src/a.go","content":"x"}`))
	if out["permission"] != "allow" {
		t.Fatalf("cursor output = %v", out)
	}
}

func TestWatchdogCursorShellDenyIsTheOnlyDecision(t *testing.T) {
	d := startWatchdogDaemon(t, "deny")
	t.Setenv("AO_PERMISSION_MODE", "bypassPermissions")
	out := firstJSON(t, runWatchdogHook(t, "cursor", "before-shell-execution", `{"command":"cat .env","cwd":"/w"}`))
	if out["permission"] != "deny" {
		t.Fatalf("cursor output = %v", out)
	}
	if d.toolName != "Shell" || !strings.Contains(string(d.toolInput), "cat .env") {
		t.Fatalf("daemon saw %q %s", d.toolName, d.toolInput)
	}
}

func TestWatchdogCursorPreToolUseDecodesStringInput(t *testing.T) {
	d := startWatchdogDaemon(t, "allow")
	out := firstJSON(t, runWatchdogHook(t, "cursor", "pre-tool-use",
		`{"tool_name":"read_file","tool_input":"{\"target_file\":\"src/a.go\"}"}`))
	if out["permission"] != "allow" {
		t.Fatalf("cursor output = %v", out)
	}
	if !strings.Contains(string(d.toolInput), `"target_file"`) {
		t.Fatalf("daemon saw input %s", d.toolInput)
	}
}

func TestWatchdogHookCoverage(t *testing.T) {
	for _, tc := range []struct {
		agent, event string
		want         bool
	}{
		{"claude-code", "pre-tool-use", true},
		{"codex", "pre-tool-use", true},
		{"grok", "pre-tool-use", true},
		{"gemini", "pre-tool-use", true},
		{"cursor", "before-read-file", true},
		{"cursor", "before-shell-execution", true},
		{"cursor", "before-mcp-execution", true},
		{"cursor", "pre-tool-use", true},
		{"gemini", "after-tool", false},
		{"cursor", "stop", false},
	} {
		if got := isWatchdogHook(tc.agent, tc.event); got != tc.want {
			t.Errorf("isWatchdogHook(%q, %q) = %v, want %v", tc.agent, tc.event, got, tc.want)
		}
	}
}
