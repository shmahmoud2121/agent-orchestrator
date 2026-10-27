package watchdog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

const (
	watchdogLogName     = "watchdog.log"
	maxWatchdogLogBytes = 1 << 20
)

// recordBlock tells the person, writes the timeline and audit lines, and stops
// a worker after three blocks or any tamper attempt. The manager is alerted
// and never stopped. Failures here still leave the tool call denied.
func (s *Service) recordBlock(ctx context.Context, session domain.SessionRecord, toolName string, decision *Decision) {
	if s == nil || decision == nil {
		return
	}
	now := time.Now().UTC()
	s.recordTimeline(ctx, session.ID, decision, now)
	if s.shouldStop(session, decision.Tamper) && s.Stop != nil {
		if err := s.Stop.KillPreservingWorkspace(ctx, session.ID); err == nil {
			decision.Stopped = true
			// A restored worker starts its three blocks over.
			s.clearBlocks(session.ID)
		}
	}
	action := "blocked"
	if decision.Stopped {
		action = "stopped"
	}
	s.appendLog(now, session, toolName, decision.Path, action, decision.Reason)
	// Kill runs under its own timeout and can leave the request context
	// cancelled, or the database briefly busy, by the time we get here.
	// The person still has to hear about the block, so the save retries
	// on a context that outlives this hook call.
	s.notify(ctx, domain.NotificationWatchdogBlocked, session, decision.Path, decision.Reason, now)
	if decision.Stopped {
		s.notify(ctx, domain.NotificationWatchdogStopped, session, decision.Path, decision.Reason, now)
	}
}

func (s *Service) shouldStop(session domain.SessionRecord, tamper bool) bool {
	if session.Kind != domain.KindWorker {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocks == nil {
		s.blocks = map[domain.SessionID]int{}
	}
	s.blocks[session.ID]++
	return tamper || s.blocks[session.ID] >= watchdogStopAfter
}

func (s *Service) clearBlocks(id domain.SessionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blocks, id)
}

func (s *Service) notify(ctx context.Context, typ domain.NotificationType, session domain.SessionRecord, path, reason string, now time.Time) {
	if s.Notify == nil {
		return
	}
	key := "watchdog:" + string(session.ID) + ":" + path
	if typ == domain.NotificationWatchdogStopped {
		key = "watchdog-stop:" + string(session.ID)
	}
	intent := ports.NotificationIntent{
		Type:               typ,
		SessionID:          session.ID,
		ProjectID:          session.ProjectID,
		CreatedAt:          now,
		SourceKey:          key,
		SessionDisplayName: sessionLabel(session),
		Agent:              agentName(session.Harness),
		Subject:            path,
		Reason:             reason,
	}
	ctx = context.WithoutCancel(ctx)
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(50 * time.Millisecond)
		}
		err = s.Notify.Notify(ctx, intent)
		if err == nil {
			return
		}
	}
	s.appendLog(now, session, "", path, "alert-failed", err.Error())
}

func (s *Service) recordTimeline(ctx context.Context, id domain.SessionID, decision *Decision, now time.Time) {
	if s.Activity == nil || decision == nil {
		return
	}
	conversation, err := s.Activity.ConversationForSession(ctx, id)
	if err != nil || conversation.ID == "" {
		return
	}
	detail, err := json.Marshal(map[string]string{
		"event": "watchdog",
		"path":  decision.Path,
	})
	if err != nil {
		return
	}
	summary := "Watchdog blocked reading " + decision.Path
	if decision.Original {
		summary = "Watchdog blocked a change: " + domain.WorkOriginalChangedReason
	}
	_ = s.Activity.UpsertActivity(ctx, conversation.ID, "", domain.ConversationActivity{
		ID:      newActivityID(),
		Kind:    domain.ActivityKindSystem,
		Status:  domain.ActivityStatusCompleted,
		Summary: summary,
		Detail:  detail,
	}, now)
}

func (s *Service) appendLog(now time.Time, session domain.SessionRecord, toolName, path, action, notifyErr string) {
	dataDir := strings.TrimSpace(s.DataDir)
	if dataDir == "" {
		return
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return
	}
	line := fmt.Sprintf("%s project=%s session=%s ai=%s tool=%s path=%q action=%s",
		now.Format(time.RFC3339), session.ProjectID, session.ID, session.Harness, toolName, path, action)
	if notifyErr != "" && action != "alert-failed" {
		line += " reason=" + fmt.Sprintf("%q", notifyErr)
	}
	if action == "alert-failed" && notifyErr != "" {
		line += " error=" + fmt.Sprintf("%q", notifyErr)
	}
	line += "\n"
	logPath := filepath.Join(dataDir, watchdogLogName)
	flags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxWatchdogLogBytes {
		flags = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}
	f, err := os.OpenFile(logPath, flags, 0o600) //nolint:gosec // path is rooted in AO's data dir
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(line)
}

func sessionLabel(session domain.SessionRecord) string {
	if name := strings.TrimSpace(session.DisplayName); name != "" {
		return name
	}
	if session.ID != "" {
		return string(session.ID)
	}
	return "session"
}

func agentName(harness domain.AgentHarness) string {
	switch harness {
	case domain.HarnessClaudeCode:
		return "Claude Code"
	case domain.HarnessCodex:
		return "Codex"
	default:
		if harness == "" {
			return "The AI"
		}
		return string(harness)
	}
}

func newActivityID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("watchdog-%d", time.Now().UnixNano())
	}
	return "watchdog-" + hex.EncodeToString(buf[:])
}
