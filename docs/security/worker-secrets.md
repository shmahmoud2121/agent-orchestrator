# Worker secrets

Workers inherit a filtered environment and, when sandboxed, cannot read the
built-in protected-file list. Before each release, run `npm run canary`.

## Watchdog

The watchdog is the guard when a sandbox is not in the way. Claude Code's
sandbox does not run on native Windows, and the manager is never sandboxed on
any operating system. Before an agent uses a tool, AO checks the tool's input
against the protected-file list (the built-in names plus any path the person
added on the project). The list stays in the daemon. It is not written into
the worktree or the agent's environment.

The check covers file tools (Read, Edit, Write, and the equivalents) and, on
a best-effort basis, shell commands. A shell command is split into words,
quotes are removed, and a word that names a protected file is blocked. For
example `cat .env`, `type .env`, and `cp backup.db /tmp` are blocked, while
`go test ./...` and `ls` are not. This does not understand every shell trick.
A command that hides the path inside another program, or builds it from
pieces, can get past the word split. A search over the whole folder can still
read a protected file's contents. Examples are `grep -r KEY .`, or Claude's
Grep with no file filter when `.env` isn't gitignored. The `ao` exemption
covers only a command that is `ao`, `ao.exe`, `altair`, or `altair.exe` on its own.
`altair` is the same binary.

A search over a folder is allowed even when that folder contains a protected
file. A pattern that names a protected file, such as `**/.env*`, is blocked.
The agent's own worktree and the `.git` data it needs are not blocked.

If the call touches a protected file, AO refuses it. The agent is told to
continue the task without reading that file, or to ask the person. The person
gets an alert naming the session, the AI, and the file. Repeats of the same
file in the same session share one alert. Each block is also one line on the
session: "Watchdog blocked reading .env".

A worker is stopped after 3 blocked calls in that session, or immediately if
it tries to change AO's hook settings (`.claude/settings.json`,
`.claude/settings.local.json`, or the Codex hook config). The worktree is
kept. Nothing is deleted. The manager is blocked and the person is told, but
the manager is not stopped automatically.

Every project session is watched: workers, sandboxed or not, and the manager.
A standalone session with no project, and the person's own terminals, are
not checked. If AO is not running, a watched session's tool call is refused.
The check gives up after a few seconds rather than waiting on a stopped daemon.

An AI is watched when its before-tool hook can refuse the call:

- Claude Code: PreToolUse.
- Codex: PreToolUse (Codex 0.163 can refuse shell commands and `apply_patch`).
- Grok Build: the Claude-style PreToolUse hook AO writes to
  `.claude/settings.local.json`. Grok reads it through its Claude
  compatibility. Grok replaces tool input over 128 KiB with a cut-off string;
  AO refuses such a call because it cannot check it.
- Gemini CLI: BeforeTool in `.gemini/settings.json`, answered with
  `decision: deny`.

Cursor's hooks are wired too: `beforeReadFile`, `beforeShellExecution`,
`beforeMCPExecution` and `preToolUse` ask the watchdog and answer
`permission: deny`. Cursor is closed source, and some Cursor builds have been
reported to ignore a read deny, so Cursor is not counted as watched until a
live test on the person's build shows the block holds.

Other AIs are not watched. While "Only allow sandboxed workers" is off, the
worker picker marks those AIs "Not watched".

Each AI's hook settings file is protected: a tool call that writes
`.claude/settings*.json`, `.codex/hooks.json`, `.gemini/settings.json`,
`.cursor/hooks.json` or `.grok/config.toml` counts as tampering and stops the
worker.

Each block is also one line in `watchdog.log` under AO's data directory
(`~/.ao`, or `AO_DATA_DIR`). The line records the time, project, session, AI,
tool, path, and whether the call was blocked or the worker was stopped. The
file is capped the same way as `hooks.log`.

## Read-only originals

A Work project's original folders and files are a second list, next to the
protected-file list. The person added them. Agents may read them. They may
not edit, move, rename, or delete them, and they may not copy or redirect
output into them. The list is applied to every Work session, including the
manager, for every watched AI, through the same before-tool hook.

A blocked call uses the same alert, the same 3-strike stop, and the same
audit log. The reason is "tried to change an original file".

The check reads each shell command word by word. It follows `cd`,
`Set-Location` and `pushd` inside one command, and it opens wrapped commands:
`bash -c`, `sh -c`, `powershell -Command`, `pwsh -c`, `powershell
-EncodedCommand` and `cmd /c`. It also blocks `perl -i` and `ruby -i`, `find`
with `-delete` or `-exec rm`, `xargs rm`, and git commands that change files
(read-only ones such as `git status`, `git log` and `git diff` stay allowed).
Removing or moving a folder that holds an original counts as changing it.

Command inspection cannot catch every write. These still get past it:

- inline scripts: `python -c`, `node -e`, and the like;
- a script file that writes, for example `python fix.py` or `powershell -File fix.ps1`;
- programs the check does not know.

On Mac, Linux, and WSL2 the Claude Code sandbox denies those writes as well:
a Work worker may write only in its task folder and its scratch folder. On
native Windows the sandbox does not run. There the built-in instructions and
this watchdog are the guards. Running workers inside WSL2 (Work step W5) is
the real fix there.
