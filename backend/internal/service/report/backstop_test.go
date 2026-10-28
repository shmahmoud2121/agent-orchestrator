package report

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

type backstopReports struct{ list []domain.ReportRecord }

func (b *backstopReports) ListReportsBySession(context.Context, domain.SessionID) ([]domain.ReportRecord, error) {
	return b.list, nil
}

func newBackstopFixture(t *testing.T, exchange LastExchange, chat bool, orchestrator bool) (*TurnBackstop, *fakeStore, *backstopReports, domain.SessionRecord) {
	t.Helper()
	worker := domain.SessionRecord{ID: "w1", ProjectID: "p", Kind: domain.KindWorker, Activity: domain.Activity{State: domain.ActivityIdle, LastActivityAt: exchange.AskedAt.Add(time.Minute)}}
	svcStore := &fakeStore{session: worker, ok: true}
	coordStore := newCoordinatorStore(exchange.AskedAt)
	if orchestrator {
		coordStore.sessions = []domain.SessionRecord{{ID: "o1", ProjectID: "p", Kind: domain.KindOrchestrator}}
	}
	reports := &backstopReports{}
	b := NewTurnBackstop(BackstopDeps{
		Reports:     New(Deps{Store: svcStore}),
		Coordinator: NewCoordinator(CoordinatorDeps{Store: coordStore, Delivery: &coordinatorDelivery{}}),
		Store:       reports,
		Exchange: func(context.Context, domain.SessionID) (LastExchange, bool, error) {
			return exchange, chat, nil
		},
	})
	return b, svcStore, reports, worker
}

func TestBackstopReportsWorkerThatStoppedSilently(t *testing.T) {
	asked := time.Date(2026, 10, 28, 13, 40, 0, 0, time.UTC)
	b, store, _, worker := newBackstopFixture(t, LastExchange{AskedAt: asked, Reply: "Saved the chart to C:\\Cases\\Output\\ratios.png"}, true, true)
	if err := b.SessionStopped(context.Background(), worker); err != nil {
		t.Fatal(err)
	}
	if store.created.State != domain.ReportDone || !strings.Contains(store.created.Note, "ratios.png") {
		t.Fatalf("report = %+v", store.created)
	}
}

func TestBackstopWaitingWorkerNeedsInput(t *testing.T) {
	asked := time.Date(2026, 10, 28, 13, 40, 0, 0, time.UTC)
	b, store, _, worker := newBackstopFixture(t, LastExchange{AskedAt: asked, Reply: "Which year should I use?"}, true, true)
	worker.Activity.State = domain.ActivityWaitingInput
	if err := b.SessionStopped(context.Background(), worker); err != nil {
		t.Fatal(err)
	}
	if store.created.State != domain.ReportNeedsInput {
		t.Fatalf("report = %+v", store.created)
	}
}

func TestBackstopSkips(t *testing.T) {
	asked := time.Date(2026, 10, 28, 13, 40, 0, 0, time.UTC)
	t.Run("worker already reported this turn", func(t *testing.T) {
		b, store, reports, worker := newBackstopFixture(t, LastExchange{AskedAt: asked}, true, true)
		reports.list = []domain.ReportRecord{{CreatedAt: asked.Add(30 * time.Second)}}
		_ = b.SessionStopped(context.Background(), worker)
		if store.created.ID != "" {
			t.Fatalf("filed a duplicate report: %+v", store.created)
		}
	})
	t.Run("no orchestrator", func(t *testing.T) {
		b, store, _, worker := newBackstopFixture(t, LastExchange{AskedAt: asked}, true, false)
		_ = b.SessionStopped(context.Background(), worker)
		if store.created.ID != "" {
			t.Fatal("reported with no orchestrator to read it")
		}
	})
	t.Run("never given a task", func(t *testing.T) {
		b, store, _, worker := newBackstopFixture(t, LastExchange{}, true, true)
		_ = b.SessionStopped(context.Background(), worker)
		if store.created.ID != "" {
			t.Fatal("reported an idle worker with no task")
		}
	})
	t.Run("orchestrator itself", func(t *testing.T) {
		b, store, _, worker := newBackstopFixture(t, LastExchange{AskedAt: asked}, true, true)
		worker.Kind = domain.KindOrchestrator
		_ = b.SessionStopped(context.Background(), worker)
		if store.created.ID != "" {
			t.Fatal("reported the orchestrator to itself")
		}
	})
}

func TestExchangeFromMessages(t *testing.T) {
	at := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)
	got := ExchangeFromMessages([]domain.ConversationMessage{
		{Role: domain.MessageRoleUser, Text: "first", CreatedAt: at},
		{Role: domain.MessageRoleAssistant, Text: "old"},
		{Role: domain.MessageRoleUser, Text: "graph it", CreatedAt: at.Add(time.Hour)},
		{Role: domain.MessageRoleAssistant, Text: "working"},
		{Role: domain.MessageRoleAssistant, Text: "done: chart.png"},
	})
	if !got.AskedAt.Equal(at.Add(time.Hour)) || got.Reply != "done: chart.png" {
		t.Fatalf("exchange = %+v", got)
	}
}
