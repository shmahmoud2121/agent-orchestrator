# ao merge

Merge a worker session's committed branch into the project's base branch on
this machine. AO does the merge itself, in a throwaway worktree, so no agent
edits the main checkout and nothing is pushed unless the project opted in.

Only the project orchestrator (or a person) can merge. Worker sessions are
refused: a worker commits on its own branch and reports done with
`ao report --done`.

## Syntax

```
ao merge <worker-session-id> [flags]
```

## Flags

| Flag | Meaning | Default / Required |
|---|---|---|
| `--dry-run` | Merge, verify, and report overlapping work without moving any branch, pushing, or messaging anyone | Optional |
| `--json` | Print the result as JSON | Optional |

## What it does

1. Refuses when the worker's worktree has uncommitted or untracked changes, or
   when its branch has no commits beyond the base branch.
2. Merges the branch with `--no-ff` in a throwaway worktree at the current base
   tip. On a conflict nothing lands and the conflicting files are listed (HTTP 409
   `MERGE_CONFLICT`).
3. Runs the project's verify command on the merge result, if one is set. A
   failure lands nothing (`MERGE_VERIFY_FAILED`).
4. Advances the base branch atomically. If the base branch is checked out (usually
   the user's main checkout), it is fast-forwarded. Uncommitted files there whose
   content already equals the merge do not block it: they come out committed and are
   listed in `matchedCheckoutFiles`. Any other tracked or untracked change refuses
   the merge and names only the differing files (`MERGE_BASE_CHECKOUT_DIRTY`).
5. Tells every other live worker whose branch is now behind that the base moved,
   and warns about workers touching the same files.
6. Only when the project's `pushAfterMerge` setting is on: pushes the base branch
   to `origin` (never forced). A failed push is reported and never undoes the
   local merge. Never run `git push` yourself.

## When the user's checkout has changes they want landed

If the user changed files in their own checkout and wants them committed or
pushed, have a worker copy those exact files into its worktree, commit them, and
report done, then run `ao merge`. Because the files match, the merge lands
without the user discarding anything. Do not ask the user to discard, stash, or
commit their own changes unless the merge names files that differ.

## On a conflict

Do not resolve it yourself. Send the conflicting files to the worker:

```bash
ao send --session mer-3 --message "ao merge conflicts on a.go, b.go. In your worktree run: git merge main, resolve, run the tests, commit, then report done again."
```

## Examples

```bash
ao merge mer-3 --dry-run
ao merge mer-3
```
