# kx — Go rewrite with a modern TUI

Status: approved · Date: 2026-10-27

## Intent

`kxconfig` + `kxswap` (bash) manage one "master" kubeconfig per project
(`~/.kube/config-<project>`) and switch the active `~/.kube/config` between
them. The owner wants a considerably better tool: a dynamic, modern terminal UI
in the spirit of k9s, and the flaws found in the audit fixed. Backwards
compatibility of the CLI is explicitly **not** required: everything may be
redesigned.

## Audit findings this design must fix

1. **Merge silently drops credentials.** `KUBECONFIG=master:new kubectl config
   view --merge` keeps the first entry on a name clash (`default`,
   `kubernetes-admin@kubernetes`… are common), so the new cluster/user is lost.
2. **`-d` deletes from the wrong project.** When deleting a context, the same
   name is also deleted from the active `~/.kube/config` even if it belongs to
   another project.
3. **Credential files may be world-readable.** `cp` keeps the source mode;
   the backups dir uses the default umask.
4. `kxswap` forces namespace `default` on every switch.
5. Copy-based switching loses whatever kubectl writes to `~/.kube/config`
   (current context, namespace, refreshed tokens).
6. One backup per project, overwritten each time; no restore command.
7. Deleting a context leaves orphan cluster/user entries (credentials linger).
8. No way to see which project is active.
9. Minor: inconsistent menus (numbers vs arrows; no q/Esc/filter), `mkdir` before
   arg parsing, `kubectl version --short` (removed in 1.28) in `install.sh`,
   CI only tests `--help`, mixed-language comments.

## Goals

- Single static Go binary, `kx`, for macOS and Linux (amd64/arm64).
- Running `kx` with no arguments opens a full-screen TUI.
- Every TUI action also has a non-interactive subcommand (scriptable).
- Fix every audit finding above.
- Kubeconfig parsing/writing via `k8s.io/client-go/tools/clientcmd` — no
  runtime dependency on `kubectl`.
- Two switching modes (decided): **global** (symlink) by default, plus an
  optional **per-terminal** isolated shell.

## Non-goals (v1)

- Talking to clusters (reachability checks, listing live namespaces). The tool
  stays offline; namespace is typed by the user.
- Windows support.
- Managing anything other than kubeconfig files (no Helm, no k9s-like resource
  browsing).
- Editing raw YAML inside the TUI (we can open `$EDITOR` on a project file).

## Anti-goals

- Any operation that can lose credentials without telling the user.
- Writing a credential file with permissions wider than 0600.
- Touching a project other than the one the user acted on.

## Constraints

- Go 1.27 (latest; owner asked for latest versions). Module: `github.com/nunoarmada/kxconfig`.
- Keep the existing on-disk layout `~/.kube/config-<project>` so current users'
  projects work with zero migration.
- Release flow currently uses release-please + a custom Homebrew update
  workflow (tap `nunoarmada/kxconfig`, secret `HOMEBREW_TAP_TOKEN`). Owner
  decided: CI/release changes and removal of the bash tooling ship in this same
  work, as the final phase.

## Approaches

### Language / UI toolkit
- **Go + Bubble Tea (chosen).** Single binary, modern look (Lip Gloss styling,
  Bubbles list with fuzzy filter), Elm-style model is easy to test.
- Go + tview: k9s look, ready-made tables/forms; more rigid styling.
- Bash + fzf: cheap, but not a real k9s-style app and adds an fzf dependency.
- Python + Textual: great UI, but needs a Python runtime; awkward to distribute.

Owner chose a Go rewrite; Bubble Tea proposed for the "modern" feel (my call,
tview remains an acceptable fallback).

### Switching model
- A. Global symlink only — simplest, keeps today's mental model.
- B. Per-terminal only (`KUBECONFIG` via shell hook, like kubie/kubeswitch) —
  safer for prod, needs shell integration, GUI tools don't see it.
- **C. Both (chosen by owner):** symlink by default + optional `kx shell`.

## Design

### On-disk layout

```
~/.kube/config            -> symlink to config-<active project>
~/.kube/config-<project>  project master files (0600)
~/.kube/kx/state.json     { "previous": "<project>" }        (0600)
~/.kube/kx/backups/<project>/<UTC timestamp>-<op>.yaml       (dir 0700, files 0600)
~/.kube/kx/shells/<pid>-<project>.yaml   per-shell isolated copies (deleted on exit)
```

- The active project is derived from the symlink target, not stored.
- Existing `~/.kube/backups/*.bak` from the bash tool are left untouched.
- Backups: one before every mutating operation except `kx ctx`/`kx ns` (a
  one-field change), plus one before `$EDITOR` opens a project from the TUI;
  keep the newest 20 per project (my call). `_initial` backups are never
  pruned and can be restored to a new project with `kx restore --initial
  [ID] --as NAME` (never over an existing project or `~/.kube/config`).
- Read-modify-write operations read the project file once, back up exactly
  those bytes, refuse a zero-length file and hold the client-go lock file
  `<file>.lock` while writing, so they do not race with kubectl.
- All writes are atomic for project files (write temp in same dir with 0600,
  fsync, rename). Writes through the active symlink follow it (verified:
  client-go `WriteToFile` uses `os.WriteFile`, which follows symlinks).
- On every load, files with mode wider than 0600 are reported (TUI badge,
  `kx doctor`) and fixed by `kx doctor --fix`.

### First run / adopting an existing `~/.kube/config`

When `~/.kube/config` is a regular file (not our symlink) and the user activates
a project:
1. If it holds the same kubeconfig as some `config-<p>` (compared
   semantically, so a file that was imported and re-serialised still matches),
   back it up and replace it with a symlink to `config-<p>` without asking.
2. Otherwise, back it up to `~/.kube/kx/backups/_initial/` and ask: import it
   as a new project (name prompt) or just replace it with the link. Never
   delete it without a backup.

If `~/.kube/config` is a symlink pointing outside `config-*`, refuse to touch it
and explain (the user manages it some other way).

### CLI (cobra)

| Command | Purpose |
|---|---|
| `kx` | open the TUI |
| `kx use <project>` / `kx -` | activate project globally / go back to previous |
| `kx ls` | list projects (active marked); `-o json` for scripts |
| `kx current` | print active project (and per-shell project if inside `kx shell`) |
| `kx import <file> [-p project] [--replace] [--on-conflict=fail\|rename\|overwrite\|skip]` | merge (default) or replace; creates project if missing |
| `kx ctx [name]` | list / switch context within the active project |
| `kx ns <namespace>` | set namespace of the current context |
| `kx rm-ctx <ctx> [-p project] [--keep-orphans]` | delete context; orphan cluster/user removed by default |
| `kx rename <old> <new>` | rename project (updates symlink if active) |
| `kx delete <project>` | delete project (backup kept; refuses if active unless `--force`) |
| `kx backups <project>` / `kx restore <project> [backup-id]` | list / restore backups |
| `kx shell <project>` | per-terminal isolated subshell |
| `kx doctor [--fix]` | check permissions, broken symlink, invalid files |
| `kx completion bash\|zsh\|fish` | shell completion |
| `kx version` | version |

Non-interactive commands never prompt when stdin is not a TTY; they fail with a
clear message instead (`--yes` to confirm destructive ops).

### Merge semantics (fixes finding 1)

Entities: clusters, users (authinfos), contexts. For each incoming entity:
- name absent in master → add;
- same name, identical content → no-op;
- same name, different content → **conflict**.

Conflict policy:
- CLI default `fail`: print a table of conflicts and change nothing.
- `rename`: incoming entity gets a suffix (`-2`, `-3`…), and references from the
  incoming contexts are rewritten accordingly.
- `overwrite`: incoming wins; `skip`: master wins (old behaviour, now explicit).
  Existing contexts that follow an overwritten cluster/user (and are not
  themselves replaced) now use the incoming one; the merge reports them
  (`Rebound`), and the CLI/TUI list them and ask for confirmation (`--yes`
  without a terminal) before anything is written.
- TUI: a conflict screen lists each clash with a short field-level summary
  (server URL, auth type — never secret values) and lets the user choose per
  item or for all.

`current-context` of the master is kept on merge; on replace, taken from the
incoming file.

### Context deletion (fixes 2 and 7)

Operates only on the named project's file. Removes the context, then removes
the cluster and user it referenced if no other context in that file uses them
(`--keep-orphans` to disable). If it was the file's `current-context`, set it
to the first remaining context (alphabetical) or empty. Never touches other
projects. Because the active config is a symlink, there is no separate "active
copy" to fix up.

### Namespace (fixes 4)

Switching projects never changes namespaces. Namespace changes only via
`kx ns` / TUI action.

### `kx shell <project>` (per-terminal mode)

- Copies the project file to `~/.kube/kx/shells/<pid>-<project>.yaml` (0600), spawns
  `$SHELL` with `KUBECONFIG` set to it and `KX_PROJECT=<project>`,
  `KX_SHELL=1`. Removes the copy on exit.
- Context/namespace changes inside the shell are isolated and discarded on exit
  (refreshed tokens too — documented limitation, my call: isolation beats
  write-back for v1).
- Inside a kx shell, `kx use` is refused with a hint (exit the shell first) to
  avoid confusion; `kx ctx`/`kx ns` act on the shell's copy. Nested `kx shell`
  is refused.
- We print a hint for showing `$KX_PROJECT` in the prompt; we do not edit the
  user's rc files.

### TUI (Bubble Tea + Bubbles + Lip Gloss)

Layout (min 80×24, adapts to width; below that, single-pane fallback):

```
┌ kx ─ active: prod ───────────────────────────────── ? help ┐
│ PROJECTS            │ CONTEXTS (prod)                      │
│ ● prod              │ ▸ prod-eu   cluster: eks-eu  ns: api │
│   staging           │   prod-us   cluster: eks-us  ns: -   │
│   dev        ⚠ 0644 │                                      │
│                     │ DETAILS                              │
│                     │ server  https://…eu.eks…             │
│                     │ user    arn:aws:…  (exec: aws)       │
├─────────────────────┴──────────────────────────────────────┤
│ enter use · / filter · i import · d delete · r rename · …  │
└─────────────────────────────────────────────────────────────┘
```

- Two focusable panes (Tab / ←→ / h l), j/k/↑↓ navigation, `/` fuzzy filter,
  `q`/Esc back or quit, `?` help overlay, `:` command palette (k9s-style) for
  less common actions.
- Projects pane: Enter = activate; `i` import file (path input with tab
  completion); `r` rename; `D` delete (confirm); `b` backups → restore; `s`
  open `kx shell` for it; `e` open in `$EDITOR`.
- Contexts pane: Enter = make current-context; `n` set namespace; `d` delete
  context (confirm, shows which orphans will go).
- Status bar shows result/error of the last action; errors never crash the UI.
- Empty state (no projects): explains and offers `i` import, or adopting the
  current `~/.kube/config`.
- Colours via Lip Gloss adaptive colours (light/dark terminals); respects
  `NO_COLOR`. No secret values ever rendered.

### Code structure (proposal; builder may adjust)

```
cmd/kx/main.go
internal/store/     project discovery, symlink activation, atomic write, perms, state
internal/kubecfg/   load/save via clientcmd, merge+conflicts, delete-context+orphans
internal/backup/    create/list/restore/prune
internal/shell/     kx shell
internal/cli/       cobra commands
internal/tui/       bubble tea models (app, projects, contexts, dialogs, conflicts)
```

`store`, `kubecfg`, `backup` hold all logic and are UI-agnostic; CLI and TUI
are thin layers over them. All paths come from a root dir (`$KX_HOME`, default
`~/.kube`) so tests run against temp dirs.

### Testing

- Unit tests (TDD) for `kubecfg` (merge cases incl. every conflict policy,
  rename rewriting, orphan removal), `store` (symlink adopt rules, atomic write,
  0600), `backup` (prune, restore).
- CLI tests running commands against a temp `KX_HOME`.
- TUI: model-level tests (send key msgs, assert state) and a few
  `teatest` golden snapshots.
- CI: `go vet`, `golangci-lint`, `go test -race` on Linux and macOS.

### Distribution and repo changes

- GoReleaser builds darwin/linux × amd64/arm64 and publishes the Homebrew
  formula to the existing tap, replacing `update-homebrew.yml` and the bash
  asset upload in release-please (release-please stays for versioning,
  release type `go`). Version bump to **2.0.0**.
- Remove `bin/kxconfig`, `bin/kxswap`, bash completions, `scripts/install.sh`
  and `uninstall.sh` (replaced by `brew install` / `go install` / release
  tarballs). README rewritten, with a "migrating from 1.x" section.

## Clarifications (from builder check — my calls unless noted)

**Project names and discovery.** Valid name: `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`
(no `/`, no `..`, no leading `-`/`_`/`.`). A project is a regular file
`config-<valid name>` that does not end in `.lock` (client-go lock files),
`.bak`, `.orig`, `.swp` or `~`. Files that match but don't parse are listed as
*invalid* (warning badge), never hidden and never auto-modified. Every name
coming from the user (`use`, `import -p`, `rename`, `delete`) is validated
before building a path.

**Which file a command edits.** One resolver in `store`:
- `kx ctx` / `kx ns` (and their TUI actions) without `-p`: inside a kx shell
  → the shell copy; otherwise → the active project's master.
- Everything else (`import`, `rm-ctx`, `rename`, `delete`, `restore`, and any
  command with `-p`) → always the master file. Inside a kx shell, `rm-ctx`
  without `-p` defaults to `$KX_PROJECT`'s master and prints that the shell's
  snapshot is not updated.

**"Identical" for merge.** Compare entities after normalising: file references
inlined (as `--flatten` does), `LocationOfOrigin` dropped, extensions kept;
then compare the serialised YAML. **Rename** is per entity: each conflicting
incoming cluster/user is renamed once (`name-2`, `-3`…) and every incoming
context that references it is rewritten; a conflicting incoming context gets
its own suffix.

**Dangling states.**
- `kx -` when `previous` no longer exists → error, state cleared.
- `rename` updates the symlink if active, and `previous` if it matches.
- `delete --force` on the active project → the `~/.kube/config` symlink is
  removed (nothing active); the command says so and suggests `kx use`.
- Broken symlink (target missing) → TUI banner + `kx doctor` reports; `kx use`
  fixes it.

**Live shells.** `delete`, `rename`, `restore` on a project with live
`kx shell` processes (pid alive) warn but proceed. `kx doctor` removes stale
shell copies (pid dead); `kx shell` also prunes stale ones on start.

**Restore.** Backs up the current file first (so a restore is undoable), then
replaces the master atomically in place; the symlink stays valid. Allowed from
inside a kx shell (acts on the master).

## Left to the builder

- Exact package boundaries, key bindings beyond those listed, colour palette.
- Backup id format and retention constant (20 suggested).
- Whether `kx ctx`/`kx ns` without args open a small picker or just list.
