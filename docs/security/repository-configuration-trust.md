# Repository configuration trust

AO keeps provider workspace pre-trust for unattended sessions, but first requires
human approval of repository configuration that can start commands or servers.
The session manager checks the project checkout before admitting a spawn and
checks both the project and materialized worker checkout before installing hooks
or invoking provider pre-launch preparation. The latter boundary is shared by
TUI/Chat spawn, restore, and agent handoffs. The independent reviewer launcher
also calls this same gate before preparing or restoring a reviewer; its
production callback is wired by the daemon. Workspace projects include their
registered child repositories. Rejected workspaces never reach Claude's global
trust-file write or Cursor's trust marker installation.

## Review and approve

Use Project Settings → Repository configuration trust, or:

```sh
ao project trust <id>
ao project trust <id> --branch <worker-branch>
ao project trust <id> --yes
ao project trust <id> --revoke
```

The CLI prints paths, command/server summaries, and the full fingerprint before
asking for confirmation. `--yes` suppresses only the confirmation question.
Branch review reads the committed Git tree without checking it out. Commit a
worker's configuration changes before reviewing that branch. A changed live
worktree is always scanned again before launch. One fingerprint is stored per
project, so approving a different base/worker configuration set replaces the
previous approval; it is not a permanent trust-all-project switch.

HTTP clients use:

- `GET /api/v1/projects/{id}/repo-trust?branch=<optional-worker-branch>`
- `POST /api/v1/projects/{id}/repo-trust`, body
  `{"fingerprint":"<reviewed-sha256>","branch":"<optional-worker-branch>"}`
- `DELETE /api/v1/projects/{id}/repo-trust`

GET/POST/DELETE return `{fingerprint, files: [{path, summary}], approved}`. A
missing approval returns 409 `REPO_TRUST_REQUIRED`; stale approval returns 409
`REPO_TRUST_STALE`. Unknown branches return 400 `REPO_TRUST_BRANCH_INVALID`.
Agent callers return 403 `REPO_TRUST_FORBIDDEN`: the CLI refuses `AO_SESSION_ID`
and forwards it as `X-AO-Session-ID`; POST also accepts `callerSessionId` for the
existing caller-declaration convention. Both workers and orchestrators are
refused. Like localmerge's caller identity, this prevents accidental agent
self-approval; it does not authenticate a program that deliberately omits its
identity.

The typed `ProjectConfig.trustedRepoConfigFingerprint` persists in the existing
project JSON. Registration clears caller-supplied approval, and ordinary config
and settings writes preserve the current stored value. Only the explicit trust
endpoint changes it. These project writes use the same service mutex to prevent
a concurrent settings save from restoring a revoked fingerprint. No database
migration or new dependency is needed.

## Inventory and fingerprint

The inventory includes:

- Claude `.claude/settings.json` when it contains hooks, statusLine, MCP, or
  command fields; `.claude/settings.local.json`, `.claude/mcp_servers.json`,
  `.claude/hooks/**`, and root `.mcp.json`.
- Codex `.codex/**` configuration and hooks.
- Gemini `.gemini/settings.json` and `.gemini/hooks/**`.
- Cursor `.cursor/hooks.json` and `.cursor/mcp.json`.
- Copilot `.github/hooks/**` and `.copilot/**` configuration.
- Kimi `.kimi-code/**`, legacy `.kimi/**`, and `.kimi.toml`.
- Muse `.muse/**`.
- MiMo root `mimocode.json`/`mimocode.jsonc`,
  `.mimocode/mimocode.json`/`.mimocode/mimocode.jsonc`, legacy
  `.mimocode/config.json`/`.mimocode/config.toml`, and its `hooks`, `plugin`,
  `plugins`, `tool`, and `tools` directories.
- Vibe `.vibe/**`, Command Code `.commandcode/**`, and Devin `.devin/**`.
- Root `mcp.json`, `opencode.json`/`opencode.jsonc`, and `.zed/settings.json`;
  Devin can import MCP configuration from other tools.

This is deliberately conservative across harnesses: changing the selected agent
does not bypass a project's approval requirement. Ignored config is scanned too,
because provider loaders do not honor Git's ignore rules. Symlinked top-level
config directories are resolved; nested directory symlinks fail closed rather
than silently concealing configuration. Gitignore files themselves are not
agent-executable config.

SHA-256 covers sorted relative paths and retained contents with length framing.
Base/worker differences receive distinct relative `base/` and `worktree/` paths.
Workspace child names are included. The fingerprint is independent of absolute
worktree locations. Files AO merges use canonical JSON after exact callback
removal so AO's own injection does not invalidate existing approval.

Only exact AO callbacks are excluded: known events and complete entry shapes
for `.claude/settings.local.json`, `.gemini/settings.json`,
`.cursor/hooks.json`, `.github/hooks/ao.json`, `.commandcode/settings.json`, and
`.devin/config.local.json`; Vibe's exact generated hooks block; and MiMo's exact
embedded activity script digest. Matching a filename, sentinel, or command prefix
alone is insufficient. Other keys and appended shell commands remain in the
inventory. Dedicated hook-file schema version `1` is non-executable metadata.
Tests generate actual adapter hook files to check these exclusions. Codex and
Muse deliver current AO hooks through flags/AO-owned data, not repo files.
Kimi's generated lifecycle hooks live in its AO-owned profile.

## Verification and limits

`internal/adapters/agent/trustrepro` is a provider-free reproduction: a temporary
Git repo contains a SessionStart hook that would create a marker. Claude's real
PreLaunch writes `hasTrustDialogAccepted: true` into a temporary HOME; Codex's
real command builder emits both workspace trust and hook-trust bypass flags.
The test never starts either provider and asserts the marker is absent. It
proves pre-acceptance and launch flags, not execution of the repository hook.

This gate controls loading repository configuration, not arbitrary code agents
may choose to execute later. It leaves user-level hooks intact and keeps AO's
activity hooks enabled. Revocation affects future launches/restores and does
not stop an already-running provider. Commands can refer to scripts outside the
inventoried configuration paths; those external script contents, system
executables, user-level config, and live-provider config reloads are not a
sandbox or a complete repository-content signature. As with any filesystem
preflight, edits after the final scan can race provider startup.

Vendor path references checked while implementing:

- Kimi: https://www.kimi.com/code/docs/en/kimi-code-cli/customization/mcp.html
- MiMo: https://mimo.xiaomi.com/mimocode/config-overrides
- Devin imports: https://docs.devin.ai/cli/reference/configuration/read-config-from
- Muse hooks: https://meta-models.github.io/muse-code-sdk/next/guides/extend/hooks/
