package httpd

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	previewutil "github.com/aoagents/agent-orchestrator/backend/internal/preview"
)

func apiTokenMiddleware(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiTokenExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		authorization := r.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeTokenRequired(w, r)
			return
		}
		provided := strings.TrimPrefix(authorization, "Bearer ")
		got := sha256.Sum256([]byte(provided))
		if provided == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeTokenRequired(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeTokenRequired(w http.ResponseWriter, r *http.Request) {
	envelope.WriteAPIError(w, r, http.StatusUnauthorized, "unauthorized", "TOKEN_REQUIRED",
		"A valid API token is required", nil)
}

func apiTokenExempt(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if _, ok := previewutil.SessionIDFromInlineArtifactHost(r.Host); ok {
		return true
	}
	if _, ok := previewutil.SessionIDFromArtifactHost(r.Host); ok {
		return true
	}
	_, ok := previewutil.SessionIDFromHost(r.Host)
	return ok
}
