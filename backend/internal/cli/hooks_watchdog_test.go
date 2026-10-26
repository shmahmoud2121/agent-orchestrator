package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchdogHookDeniesWhenDaemonSaysNo(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "proj-1")
	cfg := setConfigEnv(t)
	t.Setenv("AO_PROJECT_ID", "proj")
	var sawTool string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/watchdog") {
			var req struct {
				ToolName string `json:"toolName"`
			}
			_ = json.Unmarshal(body, &req)
			sawTool = req.ToolName
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true,"decision":"deny","path":".env","reason":"AO blocked this: .env is a protected file. Do the task without reading it, or ask the person."}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)

	stdout, _, err := executeCLI(t, Deps{
		In:           strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":".env"}}`),
		ProcessAlive: func(int) bool { return true },
	}, "hooks", "claude-code", "pre-tool-use")
	if err != nil {
		t.Fatal(err)
	}
	if sawTool != "Read" {
		t.Fatalf("daemon saw tool %q", sawTool)
	}
	var out preToolUseHookOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not deny JSON: %q (%v)", stdout, err)
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" || out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("deny output = %+v", out)
	}
	if !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, ".env is a protected file") {
		t.Fatalf("reason = %q", out.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestWatchdogHookFailClosedWhenDaemonIsDown(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "proj-1")
	setConfigEnv(t)
	t.Setenv("AO_PROJECT_ID", "proj")
	stdout, _, err := executeCLI(t, Deps{
		In: strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":".env"}}`),
	}, "hooks", "claude-code", "pre-tool-use")
	if err != nil {
		t.Fatal(err)
	}
	var out preToolUseHookOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout = %q (%v)", stdout, err)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, "isn't running") {
		t.Fatalf("output = %+v", out)
	}
}

func TestWatchdogHookIgnoresStandaloneSession(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "solo-1")
	t.Setenv("AO_PROJECT_ID", "")
	setConfigEnv(t)
	stdout, _, err := executeCLI(t, Deps{
		In: strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":".env"}}`),
	}, "hooks", "claude-code", "pre-tool-use")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("standalone stdout = %q, want empty", stdout)
	}
}

func TestWatchdogHookAllowsWhenDaemonSaysYes(t *testing.T) {
	t.Setenv("AO_SESSION_ID", "proj-1")
	cfg := setConfigEnv(t)
	t.Setenv("AO_PROJECT_ID", "proj")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/watchdog") {
			_, _ = io.WriteString(w, `{"ok":true,"decision":"allow"}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	writeRunFileFor(t, cfg, srv)
	stdout, _, err := executeCLI(t, Deps{
		In:           strings.NewReader(`{"tool_name":"Read","tool_input":{"file_path":"src/main.go"}}`),
		ProcessAlive: func(int) bool { return true },
	}, "hooks", "claude-code", "pre-tool-use")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("allowed stdout = %q, want empty", stdout)
	}
}
