# kx TUI — design brief

Status: brief for Tasks 8–9 · Date: 2026-10-27 · Source: `2026-10-27-kx-go-tui-design.md` (TUI, CLI, Clarifications)
Mockups are schematic: the ratios, clamps and strings are the spec, not the character counts.

## 1. Visual direction and palette

**Instrument panel, not dashboard.** kx is opened for five seconds to switch a project, so it reads like
a calm, precise gauge: the terminal's own foreground and background, rounded hairline panes, one
cool accent (a Kubernetes-adjacent blue) for focus and keys, green reserved for the single "what is live
right now" fact (the active project), and amber/red only when something needs attention. No fills behind
panes, no gradients, no spinners. Brand lives in the details: aligned columns, a dim/bright two-level text
hierarchy, and copy that names files and projects exactly.

Body text = **terminal default fg** (never set). All colours via `lipgloss.LightDark(isDark)`: send
`tea.RequestBackgroundColor` in `Init`, set `isDark` from `tea.BackgroundColorMsg.IsDark()`, default to
dark until it arrives, rebuild styles on change. Bubble Tea v2 downsamples to the detected profile.

| Token | Light terminal | Dark terminal | Use | Non-colour carrier |
|---|---|---|---|---|
| `accent` | `#2A5DB0` | `#79A8FF` | `kx` wordmark, key letters in hint bar, palette match chars | bold |
| `active` | `#1A7F45` | `#5FD38D` | `●` active-project marker + header project name | glyph `●` + bold name |
| `focus` | `#2A5DB0` | `#79A8FF` | border + title of focused pane / dialog | title bold+underline |
| `border` | `#B8BDC6` | `#4A505A` | unfocused pane borders | — |
| `dim` | `#6B7079` | `#8A8F98` | column headers, labels, values `-`, hint labels, counts | faint text, never sole carrier |
| `warn` | `#8A5A00` | `#E5C07B` | `⚠` badges, warning status, banners | glyph `⚠`/`!` + word |
| `error` | `#B42318` | `#FF7A7A` | error status, destructive dialog border | glyph `✗` + word |
| `success` | `#1A7F45` | `#5FD38D` | success status | glyph `✓` |
| `selBg` / `selFg` | `#D6E4FA` / `#0B1B33` | `#2B3A55` / `#E6EDF7` | cursor row in the focused list | `▸` cursor + bold |

All fg tokens are ≥4.5:1 on white / `#1E1E1E` (dim: 5.0 / 5.1; selection pairs ≥9:1).
**Profile rules** (`tea.ColorProfileMsg`): TrueColor/ANSI256 → table as is. ANSI (16) → selection uses
`Reverse(true)` instead of `selBg`. NO_COLOR / Ascii → no colours at all; selection `Reverse`, focused pane
title bold+underline, everything else is carried by glyphs and words (table's last column). Never Faint
alone for meaning.

## 2. Layout

Frame, top to bottom, always: **header** (1 row) · **banner** (0–1 row) · **body** · **status line**
(1 row, reserved even when empty so nothing jumps) · **hint bar** (1 row). Header/status/hint: no border,
horizontal padding 1. Panes: `lipgloss.RoundedBorder()`, padding `0 1`, 1-col gutter between panes, pane
title = first inner row (`PROJECTS 4`: title bold, count dim). Lists never wrap: truncate with `…` using
`ansi.Truncate` / `lipgloss.Width` (never `len`).

**A. Wide, ≥120 cols and ≥24 rows — three panes.** projects = clamp(24% w, 24, 36); details =
clamp(32% w, 34, 56); contexts = rest. All full body height.
```
 kx  active: ● prod                                                                                          ? help
╭─────────────────────────╮ ╭──────────────────────────────────────────────────────╮ ╭──────────────────────────────────╮
│ PROJECTS 4              │ │ CONTEXTS prod 2                                      │ │ DETAILS                          │
│▸● prod                  │ │   NAME        CLUSTER     NAMESPACE                 │ │ context    prod-eu               │
│   staging           $1  │ │ ▸★ prod-eu    eks-eu      api                       │ │ cluster    eks-eu                │
│   dev           ⚠ 0644  │ │    prod-us    eks-us      -                         │ │ server     https://A1B2…eks.ama… │
│   legacy     ⚠ invalid  │ │                                                      │ │ user       arn:aws:iam::…:role/… │
│                         │ │                                                      │ │ auth       exec: aws             │
│                         │ │                                                      │ │ file       ~/.kube/config-prod   │
╰─────────────────────────╯ ╰──────────────────────────────────────────────────────╯ ╰──────────────────────────────────╯
 ✓ Activated prod
 enter use  / filter  i import  r rename  D delete  b backups  s shell  e edit  : commands  ? help
```
**B. Medium, 80–119 cols and ≥24 rows — two columns.** projects = clamp(32% w, 22, 34); right column
= contexts (60% of body height, min 6 rows) above details (rest). If body < 14 rows, hide details.
```
 kx  active: ● prod                                          ? help
╭──────────────────────╮ ╭───────────────────────────────────────╮
│ PROJECTS 4           │ │ CONTEXTS prod 2                       │
│▸● prod               │ │   NAME       CLUSTER    NAMESPACE     │
│   staging        $1  │ │ ▸★ prod-eu   eks-eu     api           │
│   dev        ⚠ 0644  │ │    prod-us   eks-us     -             │
│   legacy  ⚠ invalid  │ ╰───────────────────────────────────────╯
│                      │ ╭───────────────────────────────────────╮
│                      │ │ DETAILS                               │
│                      │ │ server  https://A1B2…eks.amazonaws.c… │
│                      │ │ auth    exec: aws                     │
╰──────────────────────╯ ╰───────────────────────────────────────╯
 ✓ Activated prod
 enter use  / filter  i import  r rename  D delete  b backups  ? help
```
**C. Narrow, <80 cols or <24 rows — single pane** (full width, focused pane only). Header becomes a
breadcrumb; `tab` cycles Projects → Contexts → Details. Below 50 cols the CLUSTER column is dropped
(NAME + NAMESPACE only). Below **40×10**: render only `Terminal too small (WxH) — need 40×10`, truncated
to width; never panic down to 1×1.
```
 kx › prod › Contexts                ? help
╭────────────────────────────────────────╮
│ CONTEXTS prod 2              1/3 tab ▸ │
│ ▸★ prod-eu    eks-eu    api            │
│    prod-us    eks-us    -              │
╰────────────────────────────────────────╯
 ✓ Activated prod
 enter set current  n namespace  ? help
```
Overlays (dialogs, help, palette): centred over the body with lipgloss v2 layer compositing; width =
min(72, w−4), height ≤ body−2; rounded border in `focus` (destructive: `error`), padding `1 2`,
title on first row bold. At w<50 dialogs take full width (padding `0 1`).

## 3. Glyphs and badges

ASCII mode when `LC_ALL`/`LC_CTYPE`/`LANG` do not contain `UTF-8`/`utf8`, or `KX_ASCII=1`. ASCII mode
also switches borders to `lipgloss.ASCIIBorder()` and `…` to `~`. Never emit emoji variation selectors.

| Meaning | UTF-8 | ASCII | Where |
|---|---|---|---|
| cursor row | `▸` | `>` | every list, col 0 |
| active project | `●` (active, bold name) | `*` | projects col 1; header |
| current context | `★` | `@` | contexts col 1 |
| invalid file | `⚠ invalid` | `! invalid` | right-aligned badge (warn) |
| perms too open | `⚠ 0644` (actual mode) | `! 0644` | right-aligned badge (warn) |
| live kx shells | `$N` (e.g. `$1`) | `$N` | right-aligned badge (dim) |
| inside kx shell | `$ shell: <p>` | same | header |
| success/error/warn/info | `✓` `✗` `⚠` `·` | `+` `x` `!` `-` | status line prefix |
| separator in header | `·` / `›` | `-` / `>` | header |

Badge order when several: `⚠ invalid` > `⚠ 0644` > `$N`; show the first that fits, never wrap.

## 4. Focus model, keys and hint bar

One focus owner at a time: **list pane** (projects | contexts | details) → **filter input** (inside a
pane) → **overlay** (dialog/palette/help). Overlay captures all keys except `ctrl+c`. Esc always goes
one level back: overlay → filter (clears it) → at root with nothing open, quits (as spec).
Focused pane: `focus` border + bold-underlined title + `selBg` cursor row. Unfocused pane: `border`
colour, plain title, cursor shown as dim `▸` with no background. Selection is kept per pane.

Help overlay (`?`) shows exactly these groups:
```
NAVIGATION   tab / shift+tab  next / prev pane     h l ← →  pane left / right
             j k ↑ ↓          move                 g G      top / bottom
             /                filter list          esc      back / clear / quit
             :                command palette      ctrl+r   reload from disk
             ?                this help            q        quit   (ctrl+c always quits)
PROJECTS     enter  use (activate globally)        i  import kubeconfig
             r      rename                         D  delete (backup kept)
             b      backups / restore              s  open kx shell
             e      edit in $EDITOR
CONTEXTS     enter  set as current context         n  set namespace
             d      delete context (and orphans)
PALETTE      import  rename  delete  backups  shell  edit  doctor  reload  quit
```
`d` in projects → status info `· Use D to delete a project`. `enter`/`s` inside a kx shell → status
warn (see §7) — no dialog.

Hint bar copy (key = `accent` bold, label = `dim`, two spaces between items). When too wide, drop items
from the right **before** the final `? help`, which always stays.

| Focus | Hint bar |
|---|---|
| Projects | `enter use  / filter  i import  r rename  D delete  b backups  s shell  e edit  : commands  ? help` |
| Contexts | `enter set current  n namespace  d delete  / filter  : commands  ? help` |
| Details | `tab next pane  : commands  ? help` |
| Filter typing | `type to filter  enter keep  esc clear` |
| Text input | `enter confirm  esc cancel` |
| Path input | `tab complete  enter next  esc cancel` |
| Confirm | `y confirm  n/esc cancel` |
| Type-to-confirm | `type the name  enter delete  esc cancel` |
| Adoption | `i import as project  r replace (backed up)  esc cancel` |
| Conflicts | `j/k move  r o s set row  R O S set all  enter apply  esc cancel` |
| Backups | `enter restore  esc close` |
| Palette | `type to search  ↑↓ move  enter run  esc close` |
| Doctor | `f fix fixable  esc close` |
| Help | `esc close` |

## 5. Dialogs

Text input (rename / namespace / project name): label row, input with `focus` underline, inline error
row (error colour, `✗`) validated on each keystroke against `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`; enter
disabled while invalid. Path input: `bubbles/textinput` + tab completion of dirs/files, `~` expanded.
```
╭ Rename project ─────────────────────────╮    ╭ Delete context? ─────────────────────────╮
│ New name for staging                    │    │ Delete context prod-us from prod?        │
│ > staging-eu█                           │    │ Also removes unused:                     │
│ ✗ Use letters, digits, . _ - (max 64)   │    │   cluster  eks-us                        │
╰─────────────────────────────────────────╯    │   user     prod-us-admin                 │
                                               │ A backup is taken first.                 │
                                               │                   [ y Delete ]  [ n Keep ]│
                                               ╰──────────────────────────────────────────╯
```
Confirm: default is **cancel**; `y` confirms, `n`/esc cancels, enter = cancel. Destructive confirms
use the `error` border. Live shells add a warn row: `⚠ 2 kx shells are using staging; they keep their copy.`
Delete project (not active): `Delete project staging? A backup is kept in ~/.kube/kx/backups/staging.`
Delete **active** project = type-to-confirm:
```
╭ Delete active project ──────────────────────────────╮
│ prod is active. Deleting it unlinks ~/.kube/config;  │
│ nothing will be active until you use another project.│
│ A backup is kept.                                    │
│ Type prod to confirm                                 │
│ > pro█                                               │
╰──────────────────────────────────────────────────────╯
```
Adoption (enter on a project while `~/.kube/config` is a foreign regular file):
`~/.kube/config is not managed by kx. It will be backed up to ~/.kube/kx/backups/_initial first.`
then `i Import it as a new project (name)` / `r Replace it with a link to prod` / `esc Cancel`.

**Conflict resolution** (full-body overlay, not a small dialog). Rows start **unresolved**; enter is
refused with status `⚠ 2 conflicts unresolved` until every row has a policy (never a silent default).
Policy cell: `[ ? ]` unresolved (warn), `[ren]`, `[ovr]`, `[skp]` — text, bold; colour optional.
```
╭ Import ./eu.yaml into prod — 3 conflicts ───────────────────────────────────────╮
│ POLICY  KIND     NAME            EXISTING                 INCOMING              │
│▸[ren]   cluster  kubernetes      https://10.0.0.1:6443    https://10.9.0.1:6443  │
│ [ ? ]   user     admin           client-cert              exec: aws              │
│ [skp]   context  default         kubernetes/admin/-       kubernetes/admin/api   │
│                                                                                 │
│ ren keeps both (incoming gets -2) · ovr incoming wins · skp keep existing       │
│ 4 new entities will be added. A backup is taken first.                          │
╰─────────────────────────────────────────────────────────────────────────────────╯
```
Columns: POLICY 6, KIND 8, NAME 30%, EXISTING/INCOMING split the rest; summaries from
`kubecfg.Conflict` only (never secrets). Under 80 cols drop KIND into NAME (`cluster/kubernetes`) and
show EXISTING/INCOMING for the cursor row only, below the list. After apply, status:
`✓ Imported eu.yaml into prod: 4 added, 1 renamed, 1 overwritten, 1 skipped` (+ `, 1 context dropped`
as warn when `Report.Dropped` non-empty).

Backups (`b`): newest first, `ID` · `WHEN` (relative, `3h ago`; absolute `2026-10-27 14:02` at ≥80
cols) · `OP`. Enter → confirm `Restore prod to backup <id> (3h ago)? The current file is backed up first.`
Empty: `No backups for prod yet.`
```
╭ Backups — prod (12) ─────────────────────────────────────────╮
│   ID                WHEN                        OP           │
│ ▸ 20261027-140212   3h ago  2026-10-27 14:02    import       │
│   20261026-091500   1d ago  2026-10-26 09:15    pre-restore  │
╰──────────────────────────────────────────────────────────────╯
```
Command palette (`:`): one input row `: ` + up to 8 matches, fuzzy (match chars `accent` bold),
each row `name  description` (dim): `import  Import a kubeconfig file`, `rename  Rename selected project`,
`delete  Delete selected project`, `backups  Backups of selected project`, `shell  Open kx shell`,
`edit  Open in $EDITOR`, `doctor  Check permissions and links`, `reload  Reload from disk`, `quit  Quit kx`.
No match: `No command matches "xyz"`.

Doctor: one row per issue `⚠ <kind>  <message>` with suffix `fixable` (dim) or `✓ fixed`; path on a dim
second line, `~`-abbreviated. No issues: `✓ No problems found.` `f` runs `Doctor(true)` and re-lists.

## 6. Status line

One row above the hint bar: `<glyph> <message>`, glyph+message in token colour (success `✓`, error `✗`,
warn `⚠`, info `·` dim). Sentence case, past tense for done (`Activated prod`, `Renamed staging → stage`),
errors as `Could not <verb> <object>: <cause>` (first line of the error only, truncated with `…`).
Lifetime: success/info **4 s**, warn **8 s**, error **until the next action key or 15 s**. Implement with
`tea.Tick` carrying a sequence id; a tick clears only if its id is still current. A new message replaces
the old immediately. Never modal, never steals focus, never blocks input; long work (`$EDITOR`, shell)
uses `tea.ExecProcess` and reports on return.

## 7. Fixed copy

Header: ` kx  active: ● prod` · nothing active: ` kx  active: none` (dim) · right side ` ? help`.
Inside kx shell: ` kx  $ shell: staging  ·  global: prod` (`$ shell: staging` warn bold).
Shell banner (persistent, info): `Inside kx shell for staging: context/namespace changes stay in this shell. Exit it to switch projects.`
Enter/`s` in a kx shell → `⚠ Exit this kx shell to switch projects` / `⚠ Already inside a kx shell`.
Broken-link banner (warn): `⚠ ~/.kube/config points to a missing project (config-old). Press enter on a project to fix it.`
Foreign-symlink banner (warn): `⚠ ~/.kube/config is a link kx doesn't manage (→ /path). kx will not change it.`
Foreign-file banner (info): `· ~/.kube/config is not managed by kx yet. Using a project backs it up first.`
Banner truncates with `…`; at <80 cols drop the sentence after the first period.

Empty state (no projects), centred in body, no panes:
```
No projects yet

kx keeps one kubeconfig per project in ~/.kube/config-<name>.

  i   import a kubeconfig file
  a   adopt the current ~/.kube/config as a project
  q   quit
```
(`a` row only when `~/.kube/config` is a regular file; it opens the project-name input.)
Zero contexts in a project: contexts pane shows `No contexts in prod.` (dim); details `-`.
Invalid project selected: details shows `⚠ Cannot parse config-legacy: <first line of error>` and
`Press e to edit it.`; contexts pane empty with the same dim note.

## 8. Accessibility rules

- Every state has a glyph or word: active `●`, current `★`, problems `⚠ word`, focus = underline title +
  cursor; colour only reinforces. Test views with NO_COLOR and assert the glyphs are present.
- Contrast ≥4.5:1 for every token on default light/dark backgrounds (§1). Body text uses terminal fg.
- No animation, spinner or blinking required anywhere; cursor blink of textinput may stay default.
- All widths measured with `lipgloss.Width`; every row truncated to its pane's inner width; no wrap in
  lists. Golden/regression: render at 1×1, 20×5, 39×9, 40×10, 60×20, 79×23, 80×24, 119×30, 120×40,
  200×60 without panic, and with every line `lipgloss.Width ≤ w`.
- Full keyboard operation; vim and arrow keys both work; `ctrl+c` always exits and restores the terminal.
- Never render secrets: details show server host, user name and auth kind only.
