package project

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
)

const approvalSummaryRunes = 160

// ApprovalOption is one provider decision, copied as stored. Kind is the only
// signal a client may act on; the label is display text.
type ApprovalOption struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

// Approval is one pending approval in a project's chat sessions.
type Approval struct {
	SessionID   domain.SessionID   `json:"sessionId"`
	RequestID   string             `json:"requestId"`
	SessionKind domain.SessionKind `json:"sessionKind"`
	DisplayName string             `json:"displayName"`
	Summary     string             `json:"summary"`
	BulkSafe    bool               `json:"bulkSafe"`
	Options     []ApprovalOption   `json:"options"`
}

// ListApprovals returns pending chat approvals for the project. An unknown
// project is an error. An empty list means nothing is waiting.
func (m *Service) ListApprovals(ctx context.Context, id domain.ProjectID) ([]Approval, error) {
	if err := validateProjectID(id); err != nil {
		return nil, err
	}
	row, ok, err := m.store.GetProject(ctx, string(id))
	if err != nil {
		return nil, apierr.Internal("PROJECT_LOAD_FAILED", "Failed to load project")
	}
	if !ok || !row.ArchivedAt.IsZero() {
		return nil, apierr.NotFound("PROJECT_NOT_FOUND", "Unknown project")
	}
	rows, err := m.store.ListPendingProjectApprovals(ctx, string(id))
	if err != nil {
		return nil, apierr.Internal("PROJECT_APPROVALS_FAILED", "Failed to list project approvals")
	}
	out := make([]Approval, 0, len(rows))
	for _, row := range rows {
		detail := decodeApprovalDetail(row.Detail)
		paths, _ := approvalPaths(detail)
		name := strings.TrimSpace(row.DisplayName)
		if name == "" {
			name = "Session"
		}
		options := approvalOptions(detail)
		if options == nil {
			options = []ApprovalOption{}
		}
		out = append(out, Approval{
			SessionID:   row.SessionID,
			RequestID:   row.RequestID,
			SessionKind: row.SessionKind,
			DisplayName: name,
			Summary:     approvalSummary(row.Summary, detail, paths),
			BulkSafe:    approvalBulkSafe(row.WorkspacePath, detail),
			Options:     options,
		})
	}
	return out, nil
}

// approvalBulkSafe is true only for a file edit or write whose every path is
// inside the asking session's workspace. Shell, network, delete, anything
// outside that workspace, and anything that cannot be classified are false.
func approvalBulkSafe(workspace string, detail map[string]any) bool {
	if detail == nil {
		return false
	}
	if stringField(detail, "command") != "" || stringField(detail, "rawCommand") != "" {
		return false
	}
	subject := strings.ToLower(stringField(detail, "subjectKind"))
	if subject == "command" || subject == "mcp_tool" || strings.Contains(subject, "delete") {
		return false
	}
	method := stringField(detail, "method")
	if method == "item/commandExecution/requestApproval" {
		return false
	}
	if len(asSlice(detail["questions"])) > 0 {
		return false
	}
	tool := strings.ToLower(firstNonEmpty(
		stringField(detail, "toolName"),
		stringField(detail, "providerToolName"),
		stringField(detail, "toolKind"),
		stringField(detail, "tool"),
	))
	if toolLooksUnsafe(tool) {
		return false
	}
	paths, deleted := approvalPaths(detail)
	if deleted || !classifiedFileEdit(subject, method, tool) || len(paths) == 0 {
		return false
	}
	if strings.TrimSpace(workspace) == "" {
		return false
	}
	for _, path := range paths {
		if !pathInsideWorkspace(workspace, path) {
			return false
		}
	}
	return true
}

func classifiedFileEdit(subject, method, tool string) bool {
	if subject == "file_change" || method == "item/fileChange/requestApproval" {
		return true
	}
	switch tool {
	case "edit", "write", "apply_patch", "strreplace", "notebookedit", "multiedit", "search_replace":
		return true
	default:
		return false
	}
}

func toolLooksUnsafe(tool string) bool {
	if tool == "" {
		return false
	}
	for _, word := range []string{
		"shell", "bash", "powershell", "command", "exec", "terminal",
		"fetch", "http", "curl", "wget", "network", "web", "search", "browse",
		"delete", "remove", "unlink",
	} {
		if strings.Contains(tool, word) {
			return true
		}
	}
	return false
}

func approvalPaths(detail map[string]any) ([]string, bool) {
	if detail == nil {
		return nil, false
	}
	var paths []string
	deleted := false
	add := func(path, status string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		paths = append(paths, path)
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "d", "deleted", "delete", "removed":
			deleted = true
		}
	}
	for _, item := range asSlice(detail["files"]) {
		switch value := item.(type) {
		case string:
			add(value, "")
		case map[string]any:
			add(stringField(value, "path"), stringField(value, "status"))
		}
	}
	for _, item := range asSlice(detail["locations"]) {
		switch value := item.(type) {
		case string:
			add(value, "")
		case map[string]any:
			add(firstNonEmpty(stringField(value, "path"), stringField(value, "uri")), "")
		}
	}
	if input, ok := detail["input"].(map[string]any); ok {
		add(firstNonEmpty(
			stringField(input, "path"),
			stringField(input, "file_path"),
			stringField(input, "filePath"),
			stringField(input, "target_file"),
			stringField(input, "notebook_path"),
		), "")
	}
	return paths, deleted
}

func approvalOptions(detail map[string]any) []ApprovalOption {
	raw := asSlice(detail["decisions"])
	out := make([]ApprovalOption, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ApprovalOption{
			ID:    stringField(obj, "id"),
			Kind:  stringField(obj, "kind"),
			Label: stringField(obj, "label"),
		})
	}
	return out
}

func approvalSummary(summary string, detail map[string]any, paths []string) string {
	line := firstNonEmpty(stringField(detail, "command"), stringField(detail, "rawCommand"))
	if line == "" && len(paths) == 1 {
		line = paths[0]
	}
	if line == "" {
		line = summary
	}
	return oneLine(line, approvalSummaryRunes)
}

func decodeApprovalDetail(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var detail map[string]any
	if err := json.Unmarshal(raw, &detail); err != nil || detail == nil {
		return map[string]any{}
	}
	return detail
}

func pathInsideWorkspace(workspace, path string) bool {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	path = strings.TrimSpace(path)
	if path == "" || workspace == "" || workspace == "." {
		return false
	}
	if !filepath.IsAbs(path) {
		cleaned := filepath.Clean(path)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return false
		}
		return cleaned != "" && cleaned != "."
	}
	rel, err := filepath.Rel(workspace, filepath.Clean(path))
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func oneLine(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes-1]) + "…"
}

func stringField(obj map[string]any, key string) string {
	value, _ := obj[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func asSlice(value any) []any {
	items, _ := value.([]any)
	return items
}
