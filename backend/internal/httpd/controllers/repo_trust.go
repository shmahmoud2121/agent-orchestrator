package controllers

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apispec"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"github.com/aoagents/agent-orchestrator/backend/internal/service/repotrust"
)

// RepoTrustRequest accepts the exact fingerprint displayed by the inventory.
type RepoTrustRequest struct {
	Branch          string `json:"branch,omitempty"`
	Fingerprint     string `json:"fingerprint"`
	CallerSessionID string `json:"callerSessionId,omitempty"`
}

type repoTrustManager interface {
	RepoTrust(context.Context, domain.ProjectID, string) (repotrust.Scan, error)
	SetRepoTrust(context.Context, domain.ProjectID, string, string, string, bool) (repotrust.Scan, error)
}

func (c *ProjectsController) repoTrust(w http.ResponseWriter, r *http.Request) {
	mgr, ok := c.Mgr.(repoTrustManager)
	if !ok {
		apispec.NotImplemented(w, r, r.Method, "/api/v1/projects/{id}/repo-trust")
		return
	}
	id := domain.ProjectID(chi.URLParam(r, "id"))
	if r.Method == http.MethodGet {
		scan, err := mgr.RepoTrust(r.Context(), id, r.URL.Query().Get("branch"))
		if err != nil {
			envelope.WriteError(w, r, err)
			return
		}
		envelope.WriteJSON(w, http.StatusOK, scan)
		return
	}
	var in RepoTrustRequest
	if r.Method == http.MethodPost {
		if err := decodeJSONStrict(r, &in); err != nil {
			envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
			return
		}
	}
	if caller := r.Header.Get("X-AO-Session-ID"); caller != "" {
		in.CallerSessionID = caller
	}
	scan, err := mgr.SetRepoTrust(r.Context(), id, in.Fingerprint, in.CallerSessionID, in.Branch, r.Method == http.MethodDelete)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, scan)
}

// RepoTrustQuery selects a worker branch for review without checking it out.
type RepoTrustQuery struct {
	Branch string `query:"branch,omitempty" json:"branch,omitempty"`
}

// RepoTrustCallerHeader forwards AO_SESSION_ID from thin CLI clients.
type RepoTrustCallerHeader struct {
	SessionID string `header:"X-AO-Session-ID,omitempty"`
}
