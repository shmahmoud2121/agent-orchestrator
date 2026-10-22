package domain

import (
	"fmt"
	"net"
	"strings"
	"unicode"
)

// WorkerSandboxConfig controls Claude Code's OS sandbox for worker sessions.
// Orchestrator and reviewer sessions ignore it.
//
// When the field is absent from stored JSON, IsEnabled reports true so existing
// projects gain the sandbox without a migration. Set Enabled to a false bool to
// opt out (unsafe).
type WorkerSandboxConfig struct {
	// Enabled turns the worker OS sandbox on. nil means true (the default).
	Enabled *bool `json:"enabled,omitempty"`
	// AllowedDomains is the sandbox network allowlist. Empty means no outbound
	// network through the sandbox proxy. The ao CLI is excluded from the
	// sandbox (and allowlisted under permissions) so it can still reach the
	// loopback daemon. Entries are hostnames or "*.example.com" only — no
	// schemes, paths, or ports.
	AllowedDomains []string `json:"allowedDomains,omitempty"`
}

// IsEnabled reports whether worker sandboxing is on. A missing config (nil
// Enabled) defaults to true.
func (c WorkerSandboxConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// WithDefaults returns c with Enabled set to true when it was unset, so callers
// that persist or display the effective policy see the default explicitly.
func (c WorkerSandboxConfig) WithDefaults() WorkerSandboxConfig {
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.AllowedDomains == nil {
		c.AllowedDomains = []string{}
	}
	return c
}

// Validate rejects domain allowlist entries that are not bare hostnames or a
// single leading "*." wildcard. Schemes, paths, ports, and spaces are refused.
func (c WorkerSandboxConfig) Validate() error {
	for i, raw := range c.AllowedDomains {
		if err := ValidateSandboxDomain(raw); err != nil {
			return fmt.Errorf("workerSandbox.allowedDomains[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateSandboxDomain accepts a hostname or "*.example.com" wildcard only.
func ValidateSandboxDomain(raw string) error {
	domain := strings.TrimSpace(raw)
	if domain == "" || domain != raw {
		return fmt.Errorf("must be a hostname or *.example.com wildcard with no leading/trailing space")
	}
	if strings.ContainsAny(domain, `:/\#?@[] `) || strings.Contains(domain, "://") {
		return fmt.Errorf("%q: schemes, paths, ports, and userinfo are not allowed", domain)
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("%q: invalid hostname", domain)
	}

	rest := domain
	if strings.HasPrefix(domain, "*.") {
		rest = strings.TrimPrefix(domain, "*.")
		if rest == "" || strings.HasPrefix(rest, "*.") {
			return fmt.Errorf("%q: only a single leading *. wildcard is allowed", domain)
		}
	}
	if err := validateHostnameLabels(rest); err != nil {
		return fmt.Errorf("%q: %w", domain, err)
	}
	return nil
}

func validateHostnameLabels(host string) error {
	if host == "" {
		return fmt.Errorf("empty hostname")
	}
	// Reject IP literals: the project setting is a domain allowlist. Daemon
	// access uses excludedCommands for ao, not allowlisted IP:port entries.
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("IP addresses are not allowed; use a hostname")
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("invalid hostname label")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("hostname labels must not start or end with '-'")
		}
		for _, r := range label {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				continue
			}
			return fmt.Errorf("hostname labels may contain only letters, digits, and '-'")
		}
	}
	return nil
}
