package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApprovalBulkSafe(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "ws")
	inside := filepath.Join(workspace, "src", "main.go")
	outside := filepath.Join(t.TempDir(), "other", "secret.go")

	cases := []struct {
		name   string
		detail map[string]any
		want   bool
	}{
		{
			name: "edit inside workspace",
			detail: map[string]any{
				"subjectKind": "file_change",
				"files":       []any{map[string]any{"path": inside, "status": "M"}},
			},
			want: true,
		},
		{
			name: "relative path is inside",
			detail: map[string]any{
				"subjectKind": "file_change",
				"files":       []any{map[string]any{"path": "src/main.go", "status": "A"}},
			},
			want: true,
		},
		{
			name: "outside workspace",
			detail: map[string]any{
				"subjectKind": "file_change",
				"files":       []any{map[string]any{"path": outside, "status": "M"}},
			},
		},
		{
			name: "relative escape",
			detail: map[string]any{
				"subjectKind": "file_change",
				"files":       []any{map[string]any{"path": "../secret.go", "status": "M"}},
			},
		},
		{
			name: "shell command",
			detail: map[string]any{
				"subjectKind": "command",
				"command":     "git status",
				"files":       []any{map[string]any{"path": inside, "status": "M"}},
			},
		},
		{
			name: "delete",
			detail: map[string]any{
				"subjectKind": "file_change",
				"files":       []any{map[string]any{"path": inside, "status": "D"}},
			},
		},
		{
			name: "file change without paths",
			detail: map[string]any{
				"method": "item/fileChange/requestApproval",
			},
		},
		{
			name: "network tool",
			detail: map[string]any{
				"subjectKind": "file_change",
				"toolName":    "web_fetch",
				"files":       []any{map[string]any{"path": inside, "status": "M"}},
			},
		},
		{
			name: "unclassified",
			detail: map[string]any{
				"reason": "please confirm",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approvalBulkSafe(workspace, tc.detail); got != tc.want {
				t.Fatalf("bulkSafe = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApprovalOptionsCopyKind(t *testing.T) {
	got := approvalOptions(map[string]any{
		"decisions": []any{
			map[string]any{"id": "a", "kind": "allow_once", "label": "Always allow"},
			map[string]any{"id": "b", "label": "Allow once"},
			map[string]any{"id": "c", "kind": "custom", "label": "Ask the person"},
		},
	})
	if len(got) != 3 || got[0].Kind != "allow_once" || got[1].Kind != "" || got[2].Kind != "custom" {
		t.Fatalf("options = %#v", got)
	}
	if strings.Contains(got[0].Kind, "Always") {
		t.Fatal("kind was taken from the label")
	}
}
