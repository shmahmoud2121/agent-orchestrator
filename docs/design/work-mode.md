# Altair Work mode: design

Status: approved by the owner. Date: 2026-10-27.

## 1. What it is

Altair has two kinds of projects that live side by side.

- **Code:** a repository. A manager AI plans, and worker AIs write code in their own copies (worktrees). This is what exists today.
- **Work:** a set of the person's folders or files (spreadsheets, documents, PDFs, notes) plus a goal. A manager AI plans, and worker AIs do the work: analyse, summarise, draft, convert, build reports.

Both kinds share the same AI accounts, history, approval bar, watchdog and secret protection. Skills are optional add-ons. They never limit what Work mode can do.

## 2. Layout: side by side, not a switch

- **Sidebar sections:**
  - The sidebar has two sections, **Code** and **Work**, each with its own projects.
  - Each project shows its manager row on top, then its workers. This was already agreed for Code.
  - A section can be collapsed. An empty section shows one quiet "New Work project" row.
- **Split view:**
  - The middle area can show two sessions at once, split left and right. For example, a Code manager on one side and a Work manager on the other.
  - The split is opened from a session's menu ("Open beside") or by dragging a sidebar row to the right half.
  - One split only, at most two panes.
- **One look for both:**
  - Code and Work look the same.
  - A small icon (code brackets or a document) on each row, and in each pane header, shows which kind it is.
  - No separate colour for each mode.
- This replaces the earlier idea of a Coding | Work switch at the top. A switch would hide one mode while you use the other.

## 3. Choosing the manager

| | Code project | Work project |
|---|---|---|
| Default manager | **Claude Code** (if it's installed and signed in) | **None.** The person must pick |
| If Claude isn't available | fall back to today's rule (the AI you use most) | n/a |
| Can be changed | yes: when creating the project, and later in project settings (both exist today) | yes: same places |
| Worker AI default | today's rule (most used) | **None.** The person picks, and it can be a different AI from the manager |

- **Work project sheet:** the manager and worker pickers both start empty, and "Create" stays disabled until both are chosen. They can be different AIs. The hints read "Choose which AI manages this project." and "Choose which AI does the work."
- **Code project sheet:** the manager picker pre-selects Claude Code. Today it pre-selects whichever AI was used most recently, so this is a change.
- **The setting:** "Default manager for new Code projects" in Settings → Agents (default: Claude Code), so the person can change the default once rather than on every project.

## 4. Hard rule: originals are never changed

Agents never edit, move, rename or delete the person's original files. Everything an agent produces goes to a **results folder**.

**Where things live:**

- **Originals:** the folders or files the person added to the Work project. Agents can read them and never write to them.
- **Results folder:**
  - The default is `Documents/Altair/<project name>/`, and each task gets its own subfolder `<date> <task title>/`.
  - The person can choose a different location when creating the project. It must not be inside an original folder.
  - It holds documents, not app state, so the `~/.ao` rule doesn't apply.
- **Scratch:** each worker gets a private scratch folder under `~/.ao/work/<project>/<session>/` for temporary files. Copies of originals go there if an agent needs to transform them.

**How Altair enforces it, in layers:**

1. **Built-in instructions.** Every Work session's built-in instructions state the rule, whatever the AI. This isn't a per-project setting and it can't be turned off.
2. **Sandbox (where available).** On Mac, Linux or WSL2, a sandboxed Claude can write only to the results and scratch folders.
3. **Watchdog (everywhere, including Windows).**
   - The watchdog gets a second list: **read-only paths**, meaning the originals.
   - Reads are allowed. Any write, move, rename or delete under them (`Edit`, `Write`, `apply_patch`, shell `mv`, `rm`, `>` and the like) is blocked. This reuses the write detection that already guards the hook settings.
   - It uses the same 3-strike stop, alert and audit log.
4. **Exceptions only from the person.** A one-off exception for a single task goes through the approval bar. It never carries over to the next task.

Secret protection (`.env`, keys, Altair's database) applies to Work projects exactly as it does to Code.

## 5. Results

- A **Results** panel on the Work project lists every file the task produced, newest first, with "Open", "Show in folder" and an in-app preview. The preview covers text, Markdown, images and PDF first. Word, Excel and slides come later.
- When a task finishes, the manager posts a short summary with links to the files.
- **Altair never overwrites a file in the results folder.** A new version gets ` (2)`, ` (3)` and so on.

## 6. Skills and tools (later steps)

- **One shared skills library** that every AI can use. The person's existing skills (tax exports, financial mirror, assembly minutes, VAT breakdown) are the first examples.
- **Document tools** for Excel, Word, PDF and slides, with Arabic and right-to-left support. They come pre-installed and work offline on Windows.
- **Web access** is off by default. The person turns it on per task.

## 7. Build steps

1. **W1, foundation and safety.** Includes:
   - the project kind (`code` or `work`), stored on the project
   - the New Work project flow: pick folders, results location and manager
   - the Code and Work sidebar sections
   - the manager rules from section 3
   - the read-only-paths watchdog list and the built-in instructions
   - the results folder created on the first task

   No Work session can start until its enforcement is in place, so safety ships in the same step.
2. **W2, Results panel** and preview (text, Markdown, images, PDF).
3. **W3, split view:** two sessions side by side.
4. **W4, skills library** and offline document tools, plus Word and Excel preview.
5. **W5 (optional), workers inside WSL2 on Windows** for real sandboxing.

## 8. Decisions

- Work project workers have no default AI. The person picks, and it can differ from the manager (owner decision, 2026-10-27).

- **Results location:** the default is `Documents/Altair/<project>`. The other choice was asking every time.
- **Split view:** at most two panes.
- The Altair rename lands before W1.
