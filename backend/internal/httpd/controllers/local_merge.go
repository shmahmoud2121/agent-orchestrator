package controllers

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apispec"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/localmerge"
)

// LocalMergeService merges a worker session's branch into its project's base
// branch on this machine.
type LocalMergeService interface {
	Merge(ctx context.Context, req localmerge.Request) (localmerge.Result, error)
}

// MergeSessionRequest is the optional body of POST /api/v1/sessions/{sessionId}/merge.
type MergeSessionRequest struct {
	DryRun bool `json:"dryRun,omitempty" description:"Merge and run the project verify command in a throwaway worktree, without advancing any branch, pushing, or messaging any session."`
	// CallerSessionID is the AO session asking (the CLI sends AO_SESSION_ID).
	// Worker sessions are refused with 403; empty means a person.
	CallerSessionID string `json:"callerSessionId,omitempty" description:"AO session making the request, if any. Only an orchestrator of the same project may merge; worker sessions get 403. Omit for a person."`
}

// MergeOverlap is another live worker whose work touches files the merged
// branch also changes.
type MergeOverlap struct {
	SessionID string   `json:"sessionId"`
	Branch    string   `json:"branch"`
	Files     []string `json:"files"`
}

// MergeSessionResponse is the 200 body of POST /api/v1/sessions/{sessionId}/merge.
type MergeSessionResponse struct {
	SessionID            string         `json:"sessionId"`
	Branch               string         `json:"branch"`
	BaseBranch           string         `json:"baseBranch"`
	DryRun               bool           `json:"dryRun"`
	BaseBefore           string         `json:"baseBefore" description:"Base branch tip the merge was computed against."`
	MergeCommit          string         `json:"mergeCommit,omitempty" description:"New base branch tip. Empty for a dry run."`
	CommitCount          int            `json:"commitCount" description:"Commits the branch brings into the base branch."`
	ChangedFiles         []string       `json:"changedFiles" description:"Files the branch changes relative to the base branch (capped)."`
	ChangedFilesTotal    int            `json:"changedFilesTotal"`
	Verified             bool           `json:"verified" description:"True when the project's verify command passed on the merge result."`
	UpdatedCheckout      string         `json:"updatedCheckout,omitempty" description:"Checkout that had the base branch checked out and was fast-forwarded."`
	MatchedCheckoutFiles []string       `json:"matchedCheckoutFiles" description:"Uncommitted files in that checkout whose content already equalled the merge, so they did not block it (capped)."`
	Overlaps             []MergeOverlap `json:"overlaps" description:"Other live workers whose committed or uncommitted changes touch the same files."`
	Notified             []string       `json:"notified" description:"Live workers told that the base branch moved."`
	Warnings             []string       `json:"warnings" description:"Best-effort steps that failed without stopping the merge."`
	PushAttempted        bool           `json:"pushAttempted" description:"True when the project's pushAfterMerge setting asked to push the base branch to origin."`
	Pushed               bool           `json:"pushed" description:"True when origin accepted the push. A failed push never undoes the local merge."`
	PushedRef            string         `json:"pushedRef,omitempty"`
	PushedCommit         string         `json:"pushedCommit,omitempty"`
	PushError            string         `json:"pushError,omitempty" description:"Why the push after merge failed (tail of git's output)."`
}

// LocalMergeController owns POST /sessions/{sessionId}/merge. A nil Svc returns 501.
type LocalMergeController struct {
	Svc LocalMergeService
}

// Register mounts the local merge route.
func (c *LocalMergeController) Register(r chi.Router) {
	r.Post("/sessions/{sessionId}/merge", c.merge)
}

func (c *LocalMergeController) merge(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, "POST", "/api/v1/sessions/{sessionId}/merge")
		return
	}
	// Body is optional: omitting it performs the merge.
	var in MergeSessionRequest
	if err := decodeJSONStrict(r, &in); err != nil && !isEmptyBody(err) {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	res, err := c.Svc.Merge(r.Context(), localmerge.Request{
		SessionID: sessionID(r), DryRun: in.DryRun, CallerSessionID: domain.SessionID(strings.TrimSpace(in.CallerSessionID)),
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, mergeSessionResponse(res))
}

func mergeSessionResponse(res localmerge.Result) MergeSessionResponse {
	out := MergeSessionResponse{
		SessionID: string(res.SessionID), Branch: res.Branch, BaseBranch: res.BaseBranch, DryRun: res.DryRun,
		BaseBefore: res.BaseBefore, MergeCommit: res.MergeCommit, CommitCount: res.CommitCount,
		ChangedFiles: nonNilStrings(res.ChangedFiles), ChangedFilesTotal: res.ChangedFilesTotal,
		Verified: res.Verified, UpdatedCheckout: res.UpdatedCheckout, MatchedCheckoutFiles: nonNilStrings(res.MatchedCheckoutFiles),
		Overlaps: []MergeOverlap{}, Notified: []string{}, Warnings: nonNilStrings(res.Warnings),
		PushAttempted: res.PushAttempted, Pushed: res.Pushed, PushedRef: res.PushedRef, PushedCommit: res.PushedCommit, PushError: res.PushError,
	}
	for _, o := range res.Overlaps {
		out.Overlaps = append(out.Overlaps, MergeOverlap{SessionID: string(o.SessionID), Branch: o.Branch, Files: nonNilStrings(o.Files)})
	}
	for _, id := range res.Notified {
		out.Notified = append(out.Notified, string(id))
	}
	return out
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
