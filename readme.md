# kx

`kx` keeps one kubeconfig per project and switches between them, from a terminal UI or the command line. It is a single static Go binary and does not need `kubectl`.

`kx` replaces the `kxconfig` and `kxswap` bash scripts of 1.x. See [Migrating from 1.x](#migrating-from-1x).

## Install

**Homebrew** (macOS and Linux)

```bash
brew install nunoarmada/kxconfig/kxconfig
```

**Go** (1.27 or newer)

```bash
go install github.com/nunoarmada/kxconfig/v2/cmd/kx@latest
```

**Release tarball**

Download the archive for your OS and CPU (`darwin` or `linux`, `amd64` or `arm64`) from the [releases page](https://github.com/nunoarmada/kxconfig/releases), check it against `checksums.txt`, and put `kx` in your `PATH`. The archive also holds shell completions in `completions/`.

Check the install with `kx version`. Completions: `kx completion bash|zsh|fish` (Homebrew installs them for you).

## Quick start

```bash
kx import ~/Downloads/dev.yaml -p dev      # create project "dev" from a kubeconfig file
kx import ~/Downloads/prod.yaml -p prod --use
kx ls                                      # list projects, the active one is marked
kx use dev                                 # activate dev globally
kx use -                                   # go back to the previous project
kx                                         # open the terminal UI
```

Each project is a file `~/.kube/config-<project>`. The active project is a symlink: `~/.kube/config` points at it, so `kubectl`, `helm`, IDEs and anything else that reads `~/.kube/config` follow the switch. Project names match `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`.

## Terminal UI

Run `kx` with no arguments in a terminal. Press `?` for the full list.

| Keys | Action |
|---|---|
| `tab` / `shift+tab`, `h` `l`, arrows | move between panes |
| `j` `k`, arrows, `g` `G` | move the cursor, top, bottom |
| `/` | filter the list |
| `:` | command palette (import, rename, delete, backups, shell, edit, doctor, reload, quit) |
| `ctrl+r` | reload from disk |
| `?` | help |
| `esc` | back out of a dialog or filter; quits at the top level |
| `q`, `ctrl+c` | quit |

Projects pane:

| Key | Action |
|---|---|
| `enter` | use (activate globally) |
| `i` | import a kubeconfig |
| `r` | rename |
| `D` | delete (a backup is kept) |
| `b` | backups and restore |
| `s` | open a `kx shell` |
| `e` | edit in `$EDITOR` |

Contexts pane:

| Key | Action |
|---|---|
| `enter` | set as the current context |
| `n` | set the namespace |
| `d` | delete the context (and its orphaned cluster and user) |

## Commands

Every command except `kx` itself works without a terminal. Commands that would ask for confirmation fail instead when stdin is not a terminal; pass `--yes` to confirm up front.

| Command | What it does |
|---|---|
| `kx` | open the terminal UI |
| `kx use PROJECT` / `kx use -` | activate a project globally / go back to the previous one |
| `kx ls [-o json]` | list projects |
| `kx current` | print the active project (and this shell's project inside `kx shell`) |
| `kx import FILE [-p PROJECT] [--replace] [--on-conflict fail\|rename\|overwrite\|skip] [--use] [--yes]` | merge a kubeconfig into a project, or replace the project with it; creates the project if missing |
| `kx ctx [NAME] [-p PROJECT]` | list contexts, or switch the current one |
| `kx ns NAMESPACE [-p PROJECT]` | set the namespace of the current context (a DNS-1123 name; `""` clears it) |
| `kx rm-ctx CONTEXT [-p PROJECT] [--keep-orphans] [--yes]` | delete a context and the cluster and user only it used |
| `kx rename OLD NEW` | rename a project |
| `kx delete PROJECT [--force] [--yes]` | delete a project (a backup is kept; `--force` for the active one) |
| `kx backups PROJECT` / `kx backups --initial` | list a project's backups, newest first (`--initial`: the original `~/.kube/config` files kx backed up) |
| `kx restore PROJECT [ID] [--yes]` | restore a backup (default: the latest) |
| `kx restore --initial [ID] --as NAME` | write an original `~/.kube/config` backup to a new project `NAME` (never overwrites a project or `~/.kube/config`) |
| `kx shell PROJECT` | open a subshell isolated to one project |
| `kx doctor [--fix]` | check permissions, the config link and project files |
| `kx completion bash\|zsh\|fish` | print a shell completion script |
| `kx version` | print the version |

Example:

```bash
$ kx import dev-changed.yaml -p dev
KIND     NAME     EXISTING                     INCOMING
cluster  eks-dev  https://eks-dev.example.com  https://other.example.com
kx: 1 conflict in project dev; nothing changed (choose with --on-conflict rename|overwrite|skip)
$ kx import dev-changed.yaml -p dev --on-conflict=rename
Project dev updated: added 0, updated 0, renamed 2, skipped 0, dropped 0
```

## Switching: global and per terminal

`kx use PROJECT` switches globally by repointing the `~/.kube/config` symlink. Every terminal and tool sees the change at once.

`kx shell PROJECT` is the per-terminal alternative. It copies the project to `~/.kube/kx/shells/<pid>-<project>.yaml`, starts `$SHELL` with `KUBECONFIG` pointing at the copy, and removes the copy when you exit. Context and namespace changes made inside it stay in the copy and are discarded on exit, so a prod shell cannot change what your other terminals talk to. Inside it, `kx use`, `kx import --use` and a nested `kx shell` are refused. While the shell runs, Ctrl-C and Ctrl-\ go to the shell only, and a hangup or termination signal is passed on to it; the copy is removed either way. `kx shell` exports `KX_PROJECT=<project>` and `KX_SHELL=1`.

kx does not edit your shell rc files. To show the project in your prompt, for example in bash:

```bash
PS1='${KX_PROJECT:+[$KX_PROJECT] }'$PS1
```

or in zsh: `PROMPT='${KX_PROJECT:+[$KX_PROJECT] }'$PROMPT`.

### Environment variables

| Variable | Effect |
|---|---|
| `KX_HOME` | directory that holds the kubeconfig files and the `kx/` state; default `~/.kube` |
| `KX_ASCII=1` | use plain ASCII instead of Unicode glyphs in the UI |
| `NO_COLOR` | turn colours off in the UI |
| `KX_PROJECT`, `KX_SHELL` | set by `kx shell`; read-only for you |

## Safety

- **Merge conflicts.** Importing compares clusters, users and contexts by name. Same name and same content is a no-op; same name and different content is a conflict. The CLI default `--on-conflict fail` prints the conflicts and changes nothing. `rename` adds a suffix (`-2`, `-3`, ...) to the incoming entry and rewrites the contexts that refer to it, `overwrite` lets the incoming entry win, `skip` keeps the existing one. Overwriting a cluster or user re-points every existing context that uses it and that the file does not replace: those contexts then talk to the incoming server or use the incoming credentials. kx lists them and asks for confirmation (`--yes` when there is no terminal; the TUI asks on its conflict screen) before writing anything. The TUI shows a conflict screen where you choose per entry or for all. It never shows secret values. The project's `current-context` is kept on merge and taken from the file on `--replace`.
- **Dropped contexts.** An incoming context that refers to a cluster or user that was skipped or is missing is not merged. The import summary counts these as `dropped` and names each one.
- **Backups.** A backup is taken before every change to a project except `kx ctx` and `kx ns` (and the TUI actions that do the same), which change a single field and are not backed up; everything else is, including opening a project in `$EDITOR` from the TUI. Backups live in `~/.kube/kx/backups/<project>/`, and the newest 20 per project are kept. `kx backups` lists them and `kx restore` brings one back. Deleting a project keeps its backups. The `_initial` backups of an original `~/.kube/config` are never pruned; bring one back with `kx restore --initial --as NAME`.
- **Concurrent writers.** Changes read the project file once, back up exactly those bytes, refuse an empty file and take the client-go lock file `<file>.lock` while writing; for the active project they also take `~/.kube/config.lock`, the one kubectl takes. If another process holds it kx waits about a second and then fails; it never removes a lock itself.
- **`KUBECONFIG`.** kubectl reads `$KUBECONFIG` when it is set, not the kx project. Outside a kx shell, `kx ctx`, `kx ns` and the TUI warn when it is set to something other than the kx config or project file, since the change would not reach kubectl in that terminal.
- **Permissions.** Project files, backups and state are written atomically with mode `0600` (directories `0700`). `kx doctor` reports files that are more open than that, a broken or foreign `~/.kube/config`, and invalid project files; `kx doctor --fix` repairs permissions and stale shell copies.
- **Deleting contexts.** `kx rm-ctx` also removes the cluster and user that no other context in that file uses, so credentials do not linger; `--keep-orphans` turns that off. It only touches the named project.
- **Namespaces.** Switching project never changes a namespace; only `kx ns` or the UI does.

## Migrating from 1.x

| 1.x | 2.x |
|---|---|
| `kxswap dev` | `kx use dev` |
| `kxconfig -p dev -m file.yaml` | `kx import file.yaml -p dev` |
| `kxconfig -p dev -x file.yaml` | `kx import file.yaml -p dev --replace` |
| `kxconfig -p dev -d ctx` | `kx rm-ctx ctx -p dev` |

- Your `~/.kube/config-<project>` files keep working as projects. Existing `~/.kube/backups/*.bak` files from 1.x are left untouched; kx backups live in `~/.kube/kx/backups/`.
- 1.x copied the project file over `~/.kube/config`. kx makes `~/.kube/config` a symlink. The first `kx use` on a machine where `~/.kube/config` is a regular file backs it up to `~/.kube/kx/backups/_initial/`. If it holds the same kubeconfig as a project file (compared by content, not by bytes: an imported and re-saved copy still matches) kx just links it. Otherwise it stops and asks you to choose: `kx use PROJECT --adopt NAME` keeps the file as a new project `NAME`, `kx use PROJECT --adopt-replace` replaces it with the link. The TUI asks the same question. If `~/.kube/config` is a symlink to something that is not a kx project, kx refuses to touch it.
- The 1.x scripts are gone, so remove any `kxconfig`/`kxswap` aliases and the old completion line from your shell rc files.
- The Go module is now `github.com/nunoarmada/kxconfig/v2` and release tags are `vX.Y.Z` (they were `kxconfig-vX.Y.Z`).

## License

[MIT](LICENSE)
