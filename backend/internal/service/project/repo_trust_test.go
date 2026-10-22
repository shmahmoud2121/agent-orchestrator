package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/project"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

func TestRepoTrustApprovalPersistenceAndRevoke(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := gitRepo(t)
	path := filepath.Join(root, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"test":{"command":"touch marker"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	row := domain.ProjectRecord{ID: "trust", Path: root, DisplayName: "Trust"}
	if err := store.UpsertProject(ctx, row); err != nil {
		t.Fatal(err)
	}
	svc := project.New(store)
	scan, err := svc.RepoTrust(ctx, "trust", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range []string{"worker-1", "orchestrator-1", "unknown"} {
		_, err := svc.SetRepoTrust(ctx, "trust", scan.Fingerprint, caller, "", false)
		var api *apierr.Error
		if !errors.As(err, &api) || api.Kind != apierr.KindForbidden {
			t.Fatalf("caller %s: %v", caller, err)
		}
	}
	approved, err := svc.SetRepoTrust(ctx, "trust", scan.Fingerprint, "", "", false)
	if err != nil || !approved.Approved {
		t.Fatalf("approve: %+v %v", approved, err)
	}
	persisted, ok, err := store.GetProject(ctx, "trust")
	if err != nil || !ok || persisted.Config.TrustedRepoConfigFingerprint != scan.Fingerprint {
		t.Fatalf("persistence: %+v %v", persisted, err)
	}
	// Whole-config writes cannot forge approval, drop it, or undo revocation.
	if _, err := svc.SetConfig(ctx, "trust", project.SetConfigInput{Config: domain.ProjectConfig{TrustedRepoConfigFingerprint: strings.Repeat("a", 64)}}); err != nil {
		t.Fatal(err)
	}
	persisted, _, _ = store.GetProject(ctx, "trust")
	if persisted.Config.TrustedRepoConfigFingerprint != scan.Fingerprint {
		t.Fatal("config PUT forged approval")
	}
	if err := os.WriteFile(path, []byte(`{"command":"changed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = svc.SetRepoTrust(ctx, "trust", scan.Fingerprint, "", "", false)
	var api *apierr.Error
	if !errors.As(err, &api) || api.Code != "REPO_TRUST_STALE" || api.Kind != apierr.KindConflict {
		t.Fatalf("stale approval: %v", err)
	}
	current, err := svc.RepoTrust(ctx, "trust", "")
	if err != nil || current.Approved {
		t.Fatalf("changed approved: %+v %v", current, err)
	}
	revoked, err := svc.SetRepoTrust(ctx, "trust", "", "", "", true)
	if err != nil || revoked.Approved {
		t.Fatalf("revoke: %+v %v", revoked, err)
	}
	if _, err := svc.SetConfig(ctx, "trust", project.SetConfigInput{Config: persisted.Config}); err != nil {
		t.Fatal(err)
	}
	persisted, _, _ = store.GetProject(ctx, "trust")
	if persisted.Config.TrustedRepoConfigFingerprint != "" {
		t.Fatal("stale settings restored revoked trust")
	}
}
