package report

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// LastExchange is the newest user message time in a worker's conversation and
// the assistant reply that followed it. Reply is empty when there was none.
type LastExchange struct {
	AskedAt time.Time
	Reply   string
}

// BackstopDeps wires TurnBackstop.
type BackstopDeps struct {
	Reports     *Service
	Coordinator *Coordinator
	Store       interface {
		ListReportsBySession(context.Context, domain.SessionID) ([]domain.ReportRecord, error)
	}
	// Exchange reads the worker's last exchange. ok is false for a session
	// without a conversation (a terminal session).
	Exchange func(context.Context, domain.SessionID) (LastExchange, bool, error)
}

// TurnBackstop files a report for a worker that stopped without one, so the
// orchestrator hears about every worker that finishes or waits for an answer,
// not only the ones that remembered `ao report`.
type TurnBackstop struct {
	d BackstopDeps
}

// NewTurnBackstop constructs the backstop.
func NewTurnBackstop(d BackstopDeps) *TurnBackstop {
	return &TurnBackstop{d: d}
}

const backstopReplyLimit = 700

// SessionStopped handles one worker that went idle or is waiting at its prompt.
func (b *TurnBackstop) SessionStopped(ctx context.Context, rec domain.SessionRecord) error {
	if b == nil || b.d.Reports == nil || b.d.Coordinator == nil || b.d.Store == nil {
		return nil
	}
	if rec.Kind != domain.KindWorker || rec.ProjectID == "" || rec.IsTerminated {
		return nil
	}
	waiting := rec.Activity.State == domain.ActivityWaitingInput
	if !waiting && rec.Activity.State != domain.ActivityIdle {
		return nil
	}
	if _, ok, err := b.d.Coordinator.activeOrchestrator(ctx, rec.ProjectID); err != nil || !ok {
		return err
	}
	var exchange LastExchange
	chat := false
	if b.d.Exchange != nil {
		got, ok, err := b.d.Exchange(ctx, rec.ID)
		if err != nil {
			return err
		}
		exchange, chat = got, ok
	}
	if chat && exchange.AskedAt.IsZero() {
		// Never given a task: nothing to report.
		return nil
	}
	if !chat && waiting {
		// A terminal at its prompt may simply be starting up.
		return nil
	}
	reports, err := b.d.Store.ListReportsBySession(ctx, rec.ID)
	if err != nil {
		return err
	}
	since := exchange.AskedAt
	if !chat {
		since = rec.Activity.LastActivityAt.Add(-time.Minute)
	}
	for _, report := range reports {
		if !report.CreatedAt.Before(since) {
			return nil
		}
	}
	state, note := domain.ReportDone, "Stopped without filing a report."
	if waiting {
		state, note = domain.ReportNeedsInput, "Is waiting for an answer and did not file a report."
	}
	if reply := strings.TrimSpace(exchange.Reply); reply != "" {
		note += " Its last reply:\n" + truncateRunes(reply, backstopReplyLimit)
	} else {
		note += " Open its session to see the result."
	}
	_, err = b.d.Reports.Create(ctx, CreateInput{SessionID: rec.ID, State: state, Note: note})
	return err
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// ExchangeFromMessages finds the newest user message and the last assistant
// reply after it.
func ExchangeFromMessages(messages []domain.ConversationMessage) LastExchange {
	var out LastExchange
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == domain.MessageRoleUser {
			out.AskedAt = msg.CreatedAt
			for _, after := range messages[i+1:] {
				if after.Role == domain.MessageRoleAssistant && strings.TrimSpace(after.Text) != "" {
					out.Reply = after.Text
				}
			}
			return out
		}
	}
	return out
}
