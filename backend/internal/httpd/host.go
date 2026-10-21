package httpd

import (
	"net"
	"net/http"
	"strings"

	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
)

// loopbackHostGuard refuses requests on the loopback listener whose Host does
// not name this machine. DNS rebinding points an attacker's hostname at
// 127.0.0.1: the browser then treats the daemon as that site, sends no Origin
// on its same-origin reads, and only the Host header still shows the name the
// page used. The LAN listener serves the same router without this guard; it
// has its own password auth and is reached by real network names.
func loopbackHostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			envelope.WriteAPIError(w, r, http.StatusForbidden, "forbidden", "HOST_FORBIDDEN",
				"Host is not allowed to access this daemon", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether a Host header value (with or without a port,
// IPv6 in brackets) names this machine's loopback.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return host != "" && isLoopbackHostname(host)
}
