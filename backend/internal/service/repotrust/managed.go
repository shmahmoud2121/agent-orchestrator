package repotrust

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// repositoryContent removes only exact inert AO callback definitions. A command
// with an appended shell operator, an extra field, or a modified script remains
// in the fingerprint. JSON files AO merges are canonically encoded so adding or
// removing those exact entries does not invalidate the user's own config.
func repositoryContent(path string, data []byte) ([]byte, bool) {
	if path == ".mimocode/hooks/ao-activity.ts" && fmt.Sprintf("%x", sha256.Sum256(data)) == mimoManagedDigest {
		return nil, true
	}
	if path == ".vibe/hooks.toml" {
		data = []byte(strings.TrimSpace(strings.ReplaceAll(string(data), vibeManagedBlock(), "")))
		return data, len(data) == 0
	}
	if path == ".claude/settings.json" {
		var v map[string]any
		if json.Unmarshal(data, &v) == nil && inertClaudeSettings(v) {
			return nil, true
		}
	}
	if path != ".commandcode/settings.json" && path != ".devin/config.local.json" && path != ".claude/settings.local.json" && path != ".gemini/settings.json" && path != ".cursor/hooks.json" && path != ".github/hooks/ao.json" {
		return data, false
	}
	var root map[string]any
	if json.Unmarshal(data, &root) != nil || root == nil {
		return data, false
	}
	hooks, _ := root["hooks"].(map[string]any)
	for event, raw := range hooks {
		groups, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := []any{}
		for _, g := range groups {
			group, ok := g.(map[string]any)
			if !ok {
				kept = append(kept, g)
				continue
			}
			if ownEntry(path, event, group) {
				continue
			}
			entries, ok := group["hooks"].([]any)
			if ok {
				rest := []any{}
				for _, e := range entries {
					entry, ok := e.(map[string]any)
					if !ok || !ownEntry(path, event, entry) {
						rest = append(rest, e)
					}
				}
				if len(rest) == 0 && len(entries) > 0 && (len(group) == 1 || (len(group) == 2 && group["matcher"] != nil)) {
					continue
				}
				group["hooks"] = rest
			}
			kept = append(kept, group)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}
	// These schema-only fields are the defaults in AO's dedicated files.
	if path == ".cursor/hooks.json" || path == ".github/hooks/ao.json" {
		if root["version"] == float64(1) {
			delete(root, "version")
		}
	}
	if len(root) == 0 || (path == ".claude/settings.local.json" && inertClaudeSettings(root)) {
		return nil, true
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return data, false
	}
	return encoded, false
}

func ownEntry(path, event string, entry map[string]any) bool {
	actions := map[string]string{}
	prefix := ""
	timeout := float64(30)
	switch path {
	case ".commandcode/settings.json":
		prefix = "ao hooks command-code "
		actions = map[string]string{"SessionStart": "session-start", "PreToolUse": "pre-tool-use", "PostToolUse": "post-tool-use", "Stop": "stop"}
	case ".devin/config.local.json":
		prefix = "ao hooks devin "
		actions = map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "user-prompt-submit", "Stop": "stop", "SessionEnd": "session-end"}
	case ".claude/settings.local.json":
		prefix = "ao hooks claude-code "
		actions = map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "user-prompt-submit", "PreToolUse": "pre-tool-use", "PostToolUse": "post-tool-use", "PostToolUseFailure": "post-tool-use-failure", "PermissionRequest": "permission-request", "Stop": "stop", "StopFailure": "stop-failure", "Notification": "notification", "SubagentStart": "subagent-start", "SubagentStop": "subagent-stop", "SessionEnd": "session-end"}
	case ".gemini/settings.json":
		prefix = "ao hooks gemini "
		timeout = 30000
		actions = map[string]string{"SessionStart": "session-start", "BeforeAgent": "user-prompt-submit", "AfterAgent": "stop", "Notification": "notification", "AfterTool": "after-tool", "BeforeTool": "pre-tool-use"}
	case ".cursor/hooks.json":
		actions = map[string]string{"sessionStart": "session-start", "beforeSubmitPrompt": "user-prompt-submit", "stop": "stop", "beforeShellExecution": "before-shell-execution", "beforeMCPExecution": "before-mcp-execution", "afterShellExecution": "after-shell-execution", "afterMCPExecution": "after-mcp-execution", "postToolUse": "post-tool-use", "postToolUseFailure": "post-tool-use-failure", "beforeReadFile": "before-read-file", "preToolUse": "pre-tool-use"}
		if action := actions[event]; action != "" {
			expected := map[string]any{"command": "ao hooks cursor " + action}
			if event == "beforeShellExecution" || event == "beforeMCPExecution" || event == "beforeReadFile" || event == "preToolUse" {
				expected["failClosed"] = true
			}
			return reflect.DeepEqual(entry, expected)
		}
		return false
	case ".github/hooks/ao.json":
		actions = map[string]string{"sessionStart": "session-start", "userPromptSubmitted": "user-prompt-submit", "preToolUse": "permission-request", "agentStop": "stop"}
		if action := actions[event]; action != "" {
			cmd := "ao hooks copilot " + action
			return reflect.DeepEqual(entry, map[string]any{"type": "command", "bash": cmd, "powershell": cmd, "timeoutSec": float64(30)})
		}
		return false
	}
	action := actions[event]
	if action == "" {
		return false
	}
	return reflect.DeepEqual(entry, map[string]any{"type": "command", "command": prefix + action, "timeout": timeout})
}

func vibeManagedBlock() string {
	block := "# managed by agent-orchestrator: vibe hooks\n\n"
	for _, s := range []struct{ name, kind, match, action string }{{"ao-session-metadata", "post_agent", "", "post-agent"}, {"ao-pre-tool", "pre_tool", "*", "pre-tool"}, {"ao-post-tool", "post_tool", "*", "post-tool"}} {
		block += fmt.Sprintf("[[hooks]]\nname = %q\ntype = %q\n", s.name, s.kind)
		if s.match != "" {
			block += fmt.Sprintf("match = %q\n", s.match)
		}
		block += fmt.Sprintf("command = %q\ntimeout = 30.0\n\n", "ao hooks vibe "+s.action)
	}
	return block + "# /managed by agent-orchestrator: vibe hooks\n"
}

const mimoManagedDigest = "10193eb74e04f84172ca19a0bd5c62895c91ba611357ddcbb1b464cdb8b53696"

// inertClaudeSettingsKeys are Claude Code settings that cannot run a command.
// Everything else (hooks, statusLine, apiKeyHelper, awsAuthRefresh, env,
// enabledPlugins, MCP approvals, ...) needs approval, so a setting Claude adds
// later is gated until it is known to be inert.
var inertClaudeSettingsKeys = map[string]bool{
	"$schema": true, "permissions": true, "model": true, "cleanupPeriodDays": true,
	"includeCoAuthoredBy": true, "outputStyle": true, "alwaysThinkingEnabled": true,
	"spinnerTipsEnabled": true, "respectGitignore": true, "language": true,
}

// inertClaudeSettings reports whether Claude settings only hold inert keys.
// The "always allow" choices Claude saves into a worktree's settings.local.json
// stay inert; a repository forcing bypassPermissions does not.
func inertClaudeSettings(root map[string]any) bool {
	if root == nil {
		return false
	}
	for k := range root {
		if !inertClaudeSettingsKeys[k] {
			return false
		}
	}
	if perms, ok := root["permissions"].(map[string]any); ok && perms["defaultMode"] == "bypassPermissions" {
		return false
	}
	return true
}
