//go:build retired

package activitydispatch

import (
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestGeminiDerivesManagedHookActivity(t *testing.T) {
	tests := []struct {
		event   string
		payload string
		want    domain.ActivityState
		wantOK  bool
	}{
		{"session-start", `{}`, domain.ActivityActive, true},
		{"user-prompt-submit", `{}`, domain.ActivityActive, true},
		{"after-tool", `{}`, domain.ActivityActive, true},
		{"stop", `{}`, domain.ActivityIdle, true},
		{"notification", `{"notification_type":"ToolPermission"}`, domain.ActivityWaitingInput, true},
		{"notification", `{"notification_type":"other"}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.event+" "+tt.payload, func(t *testing.T) {
			got, ok := Derive("gemini", tt.event, []byte(tt.payload))
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("Derive(gemini, %q) = (%q, %v), want (%q, %v)", tt.event, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
