package httpd

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/requestscope"
	previewutil "github.com/aoagents/agent-orchestrator/backend/internal/preview"
)

// corsMiddleware grants cross-origin read access to the allowlisted browser
// origins only. The daemon is a no-auth loopback service, so CORS is the one
// boundary between it and hostile browser content running on the same
// machine: the allowlist must never contain "*" or the opaque "null" origin
// (every file:// page and sandboxed iframe on any website presents "null").
// The packaged Electron renderer is served from app://renderer specifically
// so it has a distinct, unforgeable origin this allowlist can name.
//
// Being served from this machine earns an origin nothing: any dev server or
// tool listening on a localhost port can host a hostile page. The one loopback
// origin admitted without an allowlist entry is a workspace preview reaching
// its own host, where PreviewOrigin answers every path read-only and the API
// is never mounted (previewSelfOrigin). The LAN listener, where every request
// carries the pairing password, still admits loopback origins for the mobile
// app (lanLoopbackOrigin).
//
// Requests without an Origin header (the CLI, curl, health probes) pass
// through untouched. Requests bearing an Origin outside the allowlist are
// rejected with 403 before any handler runs: merely omitting CORS headers
// would hide the response but NOT the side effect — a hostile page can issue
// "simple" cross-origin POSTs (no-cors mode, text/plain body) that handlers
// would otherwise execute. Same philosophy as localControlRequest on
// /shutdown, applied to the whole surface.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := exactAllowedOrigins(allowedOrigins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Cache keys must split on Origin even for rejected values, or a
			// 403 could be replayed to an allowed origin.
			w.Header().Add("Vary", "Origin")
			// An HTML artifact framed in the chat has a real origin so its own
			// files load same-origin, but it is an agent's page: everywhere but
			// that host (the daemon API, other sessions, the preview origins) it
			// is refused like any foreign origin.
			if host, ok := inlineArtifactOriginHost(origin); ok {
				if !strings.EqualFold(host, r.Host) {
					envelope.WriteAPIError(w, r, http.StatusForbidden, "forbidden", "ORIGIN_FORBIDDEN",
						"Origin is not allowed to access this daemon", nil)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok && !previewSelfOrigin(origin, r.Host) && !lanLoopbackOrigin(r, origin) {
				envelope.WriteAPIError(w, r, http.StatusForbidden, "forbidden", "ORIGIN_FORBIDDEN",
					"Origin is not allowed to access this daemon", nil)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
					h.Set("Access-Control-Allow-Headers", reqHeaders)
				}
				h.Set("Access-Control-Max-Age", "600")
				// Chromium's Private Network Access preflight for requests
				// reaching loopback from a less-private address space.
				if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
					h.Set("Access-Control-Allow-Private-Network", "true")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// appOriginMiddleware gives the routes that hand out credentials or open the
// daemon to other machines a stricter browser boundary than the rest of the
// API: only the exact configured renderer origins may call them. This holds
// even for an origin corsMiddleware admits (a preview on its own host), so a
// later loosening there cannot reach these routes. Native clients do not send
// Origin and pass through.
func appOriginMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := exactAllowedOrigins(allowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && isAppOnlyPath(r.URL.Path) {
				w.Header().Add("Vary", "Origin")
				if _, ok := allowed[origin]; !ok {
					envelope.WriteAPIError(w, r, http.StatusForbidden, "forbidden", "ORIGIN_FORBIDDEN",
						"Origin is not allowed to access this route", nil)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func exactAllowedOrigins(origins []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "null" || origin == "*" {
			continue
		}
		allowed[origin] = struct{}{}
	}
	return allowed
}

// isAppOnlyPath reports the routes appOriginMiddleware guards: account
// credentials, and Connect Mobile, which turns on LAN and tunnel access and
// returns the pairing password.
func isAppOnlyPath(path string) bool {
	for _, prefix := range []string{
		"/api/v1/agents/codex/accounts",
		"/api/v1/agents/codex/account-switches",
		"/api/v1/agents/accounts",
		"/api/v1/mobile",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// previewSelfOrigin reports whether origin is a workspace preview origin
// requesting its own host: a preview page loading its modules and assets.
// PreviewOrigin claims every request to such a host, so this grants nothing
// beyond the preview's own read-only files.
func previewSelfOrigin(origin, requestHost string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || !strings.EqualFold(u.Host, requestHost) {
		return false
	}
	if _, ok := previewutil.SessionIDFromHost(u.Host); ok {
		return true
	}
	_, ok := previewutil.SessionIDFromArtifactHost(u.Host)
	return ok
}

// lanLoopbackOrigin admits a loopback origin on a request the LAN listener
// authenticated. The mobile app pins "Origin: http://localhost" on its
// WebSocket upgrades (iOS sets an Origin React Native cannot drop), and every
// LAN request already carries the pairing password, which a hostile page does
// not have.
func lanLoopbackOrigin(r *http.Request, origin string) bool {
	if !requestscope.IsLAN(r.Context()) {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return isLoopbackHostname(u.Hostname())
}

// isLoopbackHostname reports whether host (no port, no brackets) names this
// machine's loopback: localhost, any name below it (RFC 6761), or a loopback IP.
func isLoopbackHostname(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// inlineArtifactOriginHost is the host of an inline-artifact origin, the one
// the chat thread frames an HTML artifact from.
func inlineArtifactOriginHost(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return "", false
	}
	if _, ok := previewutil.SessionIDFromInlineArtifactHost(u.Host); !ok {
		return "", false
	}
	return u.Host, true
}
