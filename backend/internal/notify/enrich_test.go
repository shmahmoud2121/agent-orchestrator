package notify

import (
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestEnrichReadyToMergePrioritizesPRContext(t *testing.T) {
	t.Parallel()

	rec, err := enrich(Intent{
		Type:               domain.NotificationReadyToMerge,
		SessionID:          "sess-1",
		ProjectID:          "proj-1",
		PRURL:              "https://github.com/acme/app/pull/67",
		SessionDisplayName: "Checkout flow",
		PRNumber:           67,
		PRTitle:            "Fix checkout totals",
		CreatedAt:          time.Date(2026, 8, 5, 4, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("enrich ready notification: %v", err)
	}

	if want := "Fix checkout totals · PR #67"; rec.Title != want {
		t.Fatalf("title = %q, want %q", rec.Title, want)
	}
	if want := "PR from session Checkout flow is ready to merge. CI passed with no blocking review feedback."; rec.Body != want {
		t.Fatalf("body = %q, want %q", rec.Body, want)
	}
}

func TestEnrichWatchdogNeedsNoPullRequest(t *testing.T) {
	t.Parallel()

	rec, err := enrich(Intent{
		Type:               domain.NotificationWatchdogBlocked,
		SessionID:          "worker-1",
		ProjectID:          "proj-1",
		SessionDisplayName: "Docs worker",
		Agent:              "Claude Code",
		Subject:            ".env",
		SourceKey:          "watchdog:worker-1:.env",
		CreatedAt:          time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Title != "Watchdog blocked Docs worker" {
		t.Fatalf("title = %q", rec.Title)
	}
	if rec.Body != "Claude Code tried to read .env." {
		t.Fatalf("body = %q", rec.Body)
	}
	if rec.PRURL != "" || rec.SourceKey != "watchdog:worker-1:.env" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestEnrichWatchdogOriginalChange(t *testing.T) {
	t.Parallel()

	rec, err := enrich(Intent{
		Type:               domain.NotificationWatchdogBlocked,
		SessionID:          "work-1",
		ProjectID:          "docs",
		SessionDisplayName: "W1 safety check",
		Agent:              "Claude Code",
		Subject:            "notes.txt",
		Reason:             domain.WorkOriginalChangedReason,
		SourceKey:          "watchdog:work-1:notes.txt",
		CreatedAt:          time.Date(2026, 10, 27, 18, 26, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Title != "Watchdog blocked W1 safety check" {
		t.Fatalf("title = %q", rec.Title)
	}
	if rec.Body != "Claude Code tried to change an original file: notes.txt." {
		t.Fatalf("body = %q", rec.Body)
	}
}

func TestEnrichReadyToMergeFallsBackWithoutPRTitle(t *testing.T) {
	t.Parallel()

	rec, err := enrich(Intent{
		Type:      domain.NotificationReadyToMerge,
		SessionID: "sess-1",
		ProjectID: "proj-1",
		PRURL:     "https://github.com/acme/app/pull/67",
		PRNumber:  67,
		CreatedAt: time.Date(2026, 8, 5, 4, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("enrich ready notification: %v", err)
	}

	if want := "PR #67 is ready to merge"; rec.Title != want {
		t.Fatalf("title = %q, want %q", rec.Title, want)
	}
}
