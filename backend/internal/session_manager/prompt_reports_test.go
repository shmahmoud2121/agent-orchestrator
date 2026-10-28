package sessionmanager

import (
	"strings"
	"testing"
)

func TestOrchestratorPromptReportsAndWorkDelegation(t *testing.T) {
	off := false
	for _, tc := range []struct {
		name         string
		project      promptProject
		wantReports  bool
		wantDelegate bool
	}{
		{"code project", promptProject{ID: "p"}, true, false},
		{"work project", promptProject{ID: "p", Work: true}, true, true},
		{"work project, team mode off", promptProject{ID: "p", Work: true, TeamMode: &off}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := orchestratorSystemPrompt(tc.project)
			if strings.Contains(got, "## Worker Reports") != tc.wantReports {
				t.Fatalf("worker reports section present = %v", !tc.wantReports)
			}
			if strings.Contains(got, "## Work Project Delegation") != tc.wantDelegate {
				t.Fatalf("work delegation section present = %v", !tc.wantDelegate)
			}
			if strings.Contains(got, "/delegate command") != (tc.project.TeamMode == nil) {
				t.Fatal("the /delegate rule must follow Team mode")
			}
			if tc.wantReports && strings.Index(got, "## Worker Reports") > strings.Index(got, "## Review and CI Workflow") {
				t.Fatal("worker reports section should come before the review workflow")
			}
		})
	}
}
