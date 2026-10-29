package activitydispatch

import (
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestCursorDerivesManagedHookActivity(t *testing.T) {
	tests := []struct {
		event string
		want  domain.ActivityState
	}{
		{"session-start", domain.ActivityActive},
		{"user-prompt-submit", domain.ActivityActive},
		{"stop", domain.ActivityIdle},
		{"after-shell-execution", domain.ActivityActive},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			got, ok := Derive("cursor", tt.event, []byte(`{}`))
			if !ok || got != tt.want {
				t.Fatalf("Derive(cursor, %q) = (%q, %v), want (%q, true)", tt.event, got, ok, tt.want)
			}
		})
	}
	if got, ok := Derive("cursor", "before-shell-execution", []byte(`{}`)); ok {
		t.Fatalf("Derive(cursor, before-shell-execution) = (%q, true), want no activity from deriver", got)
	}
}

func TestGrokDerivesClaudeCompatibleActivity(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		payload string
		want    domain.ActivityState
	}{
		{"permission request", "permission-request", `{}`, domain.ActivityBlocked},
		{"idle notification", "notification", `{"notification_type":"idle_prompt"}`, domain.ActivityIdle},
		{"session end", "session-end", `{}`, domain.ActivityExited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Derive("grok", tt.event, []byte(tt.payload))
			if !ok {
				t.Fatalf("Derive(grok, %q) ok=false, want true", tt.event)
			}
			if got != tt.want {
				t.Fatalf("Derive(grok, %q) = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}
