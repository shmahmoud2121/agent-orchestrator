/**
 * Read-only display copy of domain.BuiltinWorkerProtectedPaths. The daemon
 * enforces its own list; backend/internal/domain/worker_protected_paths_test.go
 * fails when this copy drifts.
 */
export const BUILTIN_WORKER_PROTECTED_PATHS = {
	mainCheckout: [
		".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx",
		"secrets/", "backups/", "backup/",
		"*.sqlite", "*.sqlite3", "*.db", "*.dump", "*.bak",
		"credentials.json", "service-account*.json", "*.tfstate", "*.tfstate.backup",
	],
	worktree: [".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx"],
	home: [
		".ssh", ".aws", ".azure", ".config/gcloud", ".kube",
		".docker/config.json", ".npmrc", ".pypirc", ".netrc", ".git-credentials", ".config/gh",
	],
} as const;
