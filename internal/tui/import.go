package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

// startImport opens the file path prompt, the first step of an import.
func (m *Model) startImport() tea.Cmd {
	m.push(newPathDialog(m))
	return nil
}

// expandTilde replaces a leading ~ with the home directory.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home + p[1:]
		}
	}
	return p
}

// completePath completes the path being typed. It returns the new text and
// the candidates when several remain. The typed prefix (including ~) is kept.
func completePath(in string) (string, []string) {
	if in == "~" {
		return "~/", nil
	}
	dirPart, base := "", in
	if i := strings.LastIndex(in, "/"); i >= 0 {
		dirPart, base = in[:i+1], in[i+1:]
	}
	listDir := expandTilde(dirPart)
	if listDir == "" {
		listDir = "."
	}
	des, err := os.ReadDir(listDir)
	if err != nil {
		return in, nil
	}
	var cands []string
	for _, de := range des {
		name := de.Name()
		if !strings.HasPrefix(name, base) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".")) {
			continue
		}
		if fi, err := os.Stat(filepath.Join(listDir, name)); err == nil && fi.IsDir() {
			name += "/"
		}
		cands = append(cands, name)
	}
	sort.Strings(cands)
	switch len(cands) {
	case 0:
		return in, nil
	case 1:
		return dirPart + cands[0], nil
	}
	common := cands[0]
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c, common) {
			common = common[:len(common)-1]
		}
	}
	return dirPart + common, cands
}

// pathDialog asks for the kubeconfig file to import, with tab completion.
// The file is checked on enter, so errors do not flash while typing.
type pathDialog struct {
	in    textinput.Model
	err   string
	cands []string
}

func newPathDialog(m *Model) *pathDialog {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	st.Cursor.Color = m.sty.pal.accent
	in.SetStyles(st)
	in.Focus()
	return &pathDialog{in: in}
}

func (d *pathDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "tab":
			out, cands := completePath(d.in.Value())
			d.in.SetValue(out)
			d.in.CursorEnd()
			d.cands, d.err = cands, ""
			return d, nil
		case "enter":
			return d.next(m), nil
		}
	}
	var cmd tea.Cmd
	d.in, cmd = d.in.Update(msg)
	d.err, d.cands = "", nil
	return d, cmd
}

// next checks the file and moves on to the project name.
func (d *pathDialog) next(m *Model) dialog {
	typed := strings.TrimSpace(d.in.Value())
	if typed == "" {
		return d
	}
	path := expandTilde(typed)
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		d.err = "No such file"
	case fi.IsDir():
		d.err = "That is a folder, not a kubeconfig file"
	case !fi.Mode().IsRegular():
		d.err = "Not a regular file"
	default:
		if _, err := kubecfg.LoadFlattened(path); err != nil {
			d.err = "Not a usable kubeconfig: " + firstLine(err.Error())
		}
	}
	if d.err != "" {
		return d
	}
	return m.importNameDialog(path, typed)
}

func (d *pathDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	d.in.SetWidth(max(1, iw-4))
	row := ""
	switch {
	case d.err != "":
		row = m.sty.err.Render(ansi.Truncate(m.gl.fail+" "+d.err, iw, m.gl.ell))
	case len(d.cands) > 0:
		var shown []string
		for _, c := range d.cands {
			shown = append(shown, safetext.Clean(c))
		}
		row = m.sty.dim.Render(ansi.Truncate(strings.Join(shown, "  "), iw, m.gl.ell))
	}
	return m.dialogBox("Import kubeconfig", []string{"Kubeconfig file to import", d.in.View(), row}, w, false, 1)
}

func (d *pathDialog) hints() []hint {
	return []hint{{"tab", "complete"}, {"enter", "next"}, {"esc", "cancel"}}
}

// importNameDialog asks which project to import into. The default is the
// selected project, or a free name taken from the file name.
func (m *Model) importNameDialog(path, typed string) dialog {
	def := ""
	if p, ok := m.selectedProject(); ok {
		def = p.Name
	} else {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		base = strings.Map(func(r rune) rune {
			if r < 128 && (r == '.' || r == '_' || r == '-' || (r >= '0' && r <= '9') || (r|0x20 >= 'a' && r|0x20 <= 'z')) {
				return r
			}
			return '-'
		}, base)
		if nameSyntax(base) != "" {
			base = "imported"
		}
		def = m.freeName(base)
	}
	d := newInputDialog(m, "Import "+safetext.Clean(typed), "Project name", def,
		func(_ *Model, v string) string { return nameSyntax(v) },
		func(m *Model, name string) tea.Cmd { return m.importInto(path, typed, name) })
	d.note = func(m *Model, v string) (string, bool) {
		if v == "" || nameSyntax(v) != "" {
			return "", false
		}
		for _, p := range m.projects {
			if p.Name == v {
				return "Merges into existing project " + v, false
			}
		}
		return "Creates project " + v, false
	}
	return d
}

// importInto previews the import and either applies it or opens the conflict
// screen.
func (m *Model) importInto(file, typed, project string) tea.Cmd {
	cs, err := m.store.PreviewImport(file, project)
	if err != nil {
		return m.failed("import "+filepath.Base(file), err)
	}
	if len(cs) == 0 {
		return m.applyImport(file, project, nil)
	}
	m.push(newConflictDialog(m, file, typed, project, cs))
	return nil
}

// applyImport merges file into project with the per-entity policies and
// reports the result on the status line.
func (m *Model) applyImport(file, project string, per map[kubecfg.EntityRef]kubecfg.ConflictPolicy) tea.Cmd {
	res, err := m.store.Import(file, project, store.ImportMerge,
		kubecfg.MergeOptions{Default: kubecfg.PolicyFail, PerItem: per})
	return m.finishImport(file, project, res, err)
}

// finishImport reports the outcome of Import and reloads the screen.
func (m *Model) finishImport(file, project string, res store.ImportResult, err error) tea.Cmd {
	name := filepath.Base(file)
	if err != nil {
		return m.failed("import "+name, err)
	}
	if cmd := m.refresh(); cmd != nil {
		return cmd
	}
	m.selectProject(project)
	if !res.Changed {
		return m.say(statusInfo, "Nothing to import: "+project+" already has everything in "+name)
	}
	r := res.Report
	var parts []string
	for _, c := range []struct {
		n    int
		word string
	}{{len(r.Added), "added"}, {len(r.Renamed), "renamed"}, {len(r.Overwritten), "overwritten"}, {len(r.Skipped), "skipped"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.word))
		}
	}
	if len(parts) == 0 {
		parts = []string{"no new entries"}
	}
	text := "Imported " + name + " into " + project + ": " + strings.Join(parts, ", ")
	kind := statusSuccess
	if n := len(r.Dropped); n > 0 {
		text += ", " + plural(n, "context") + " dropped"
		kind = statusWarn
	}
	if len(res.Warnings) > 0 {
		text += " (" + firstLine(res.Warnings[0]) + ")"
		kind = statusWarn
	}
	return m.say(kind, text)
}

// conflictDialog is the full-body screen that resolves every same-name entity
// that differs. Rows start unresolved and enter is refused until each has a
// policy: nothing is ever overwritten by default.
type conflictDialog struct {
	file, shown, project string
	cs                   []kubecfg.Conflict
	pol                  map[kubecfg.EntityRef]kubecfg.ConflictPolicy
	cursor, offset       int
	added                int // entities that are new in the project; -1 when unknown
	base                 int // the first base rows come from the preview; the rest are derived
}

func newConflictDialog(m *Model, file, typed, project string, cs []kubecfg.Conflict) *conflictDialog {
	d := &conflictDialog{
		file: file, shown: typed, project: project, cs: cs,
		pol: map[kubecfg.EntityRef]kubecfg.ConflictPolicy{}, added: -1, base: len(cs),
	}
	// Count the entities that are new in the project, for the footer. Any
	// policy that resolves conflicts gives the same Added list.
	incoming, err1 := kubecfg.LoadFlattened(file)
	master, err2 := kubecfg.LoadFlattened(m.store.ProjectPath(project))
	if err1 == nil && err2 == nil {
		if _, rep, err := kubecfg.Merge(master, incoming, kubecfg.MergeOptions{Default: kubecfg.PolicyOverwrite}); err == nil {
			d.added = len(rep.Added)
		}
	}
	return d
}

func (d *conflictDialog) wide() bool { return true }

func (d *conflictDialog) unresolved() int {
	n := 0
	for _, c := range d.cs {
		if d.pol[c.Ref] == "" {
			n++
		}
	}
	return n
}

func (d *conflictDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	set := map[string]kubecfg.ConflictPolicy{
		"r": kubecfg.PolicyRename, "o": kubecfg.PolicyOverwrite, "s": kubecfg.PolicySkip,
		"R": kubecfg.PolicyRename, "O": kubecfg.PolicyOverwrite, "S": kubecfg.PolicySkip,
	}
	switch key := k.String(); key {
	case "esc":
		return nil, nil
	case "j", "down":
		d.cursor++
	case "k", "up":
		d.cursor--
	case "g", "home":
		d.cursor = 0
	case "G", "end":
		d.cursor = len(d.cs) - 1
	case "r", "o", "s":
		d.pol[d.cs[d.cursor].Ref] = set[key]
	case "R", "O", "S":
		for _, c := range d.cs {
			d.pol[c.Ref] = set[key]
		}
	case "enter":
		if n := d.unresolvedBase(); n > 0 {
			return d, m.say(statusWarn, plural(n, "conflict")+" unresolved")
		}
		// Choices on earlier rows decide which derived rows still apply.
		if cmd := d.syncDerived(m); cmd != nil {
			return d, cmd
		}
		if n := d.unresolved(); n > 0 {
			return d, m.say(statusWarn, plural(n, "conflict")+" unresolved")
		}
		per := make(map[kubecfg.EntityRef]kubecfg.ConflictPolicy, len(d.pol))
		for ref, p := range d.pol {
			per[ref] = p
		}
		res, err := m.store.Import(d.file, d.project, store.ImportMerge,
			kubecfg.MergeOptions{Default: kubecfg.PolicyFail, PerItem: per})
		if errors.Is(err, kubecfg.ErrConflicts) {
			// Renaming or skipping an entity changes the contexts that use it,
			// which can then differ from the existing ones. Nothing was
			// written: show them as new rows rather than choosing for the user.
			return d, d.addDerived(m, res.Report.Conflicts)
		}
		return nil, m.finishImport(d.file, d.project, res, err)
	}
	d.cursor = max(0, min(d.cursor, len(d.cs)-1))
	return d, nil
}

func (d *conflictDialog) unresolvedBase() int {
	n := 0
	for _, c := range d.cs[:d.base] {
		if d.pol[c.Ref] == "" {
			n++
		}
	}
	return n
}

// syncDerived recomputes the derived rows from the choices on the base rows:
// rows that no longer conflict are dropped, new ones are added unresolved. It
// returns the status command when new rows appeared (the user must look), and
// nil otherwise.
func (d *conflictDialog) syncDerived(m *Model) tea.Cmd {
	incoming, err1 := kubecfg.LoadFlattened(d.file)
	master, err2 := kubecfg.LoadFlattened(m.store.ProjectPath(d.project))
	if err1 != nil || err2 != nil {
		return nil // Import reports the real error on apply
	}
	per := map[kubecfg.EntityRef]kubecfg.ConflictPolicy{}
	for _, c := range d.cs[:d.base] {
		per[c.Ref] = d.pol[c.Ref]
	}
	_, rep, err := kubecfg.Merge(master, incoming, kubecfg.MergeOptions{Default: kubecfg.PolicyFail, PerItem: per})
	if err == nil || !errors.Is(err, kubecfg.ErrConflicts) {
		d.dropDerived(nil)
		return nil
	}
	old := map[kubecfg.EntityRef]bool{}
	for _, c := range d.cs[d.base:] {
		old[c.Ref] = true
	}
	var derived []kubecfg.Conflict
	added := 0
	for _, c := range rep.Conflicts {
		derived = append(derived, c)
		if !old[c.Ref] {
			added++
		}
	}
	d.dropDerived(derived)
	if added == 0 {
		return nil // only rows that went away: nothing new to look at
	}
	d.cursor = d.base
	return m.say(statusWarn, plural(added, "dependent conflict")+" after your choices: resolve them too")
}

// dropDerived replaces the derived rows by cs, forgetting policies of rows
// that went away.
func (d *conflictDialog) dropDerived(cs []kubecfg.Conflict) {
	keep := map[kubecfg.EntityRef]bool{}
	for _, c := range cs {
		keep[c.Ref] = true
	}
	for _, c := range d.cs[d.base:] {
		if !keep[c.Ref] {
			delete(d.pol, c.Ref)
		}
	}
	d.cs = append(d.cs[:d.base:d.base], cs...)
	d.cursor = max(0, min(d.cursor, len(d.cs)-1))
}

// addDerived appends conflicts that only appeared after the user's choices.
func (d *conflictDialog) addDerived(m *Model, cs []kubecfg.Conflict) tea.Cmd {
	known := map[kubecfg.EntityRef]bool{}
	for _, c := range d.cs {
		known[c.Ref] = true
	}
	first := -1
	for _, c := range cs {
		if !known[c.Ref] {
			if first < 0 {
				first = len(d.cs)
			}
			d.cs = append(d.cs, c)
		}
	}
	if first < 0 {
		return m.failed("import "+filepath.Base(d.file), kubecfg.ErrConflicts)
	}
	d.cursor = first
	return m.say(statusWarn, plural(len(d.cs)-first, "more conflict")+" after your choices: resolve them too")
}

func policyCell(p kubecfg.ConflictPolicy) string {
	switch p {
	case kubecfg.PolicyRename:
		return "[ren]"
	case kubecfg.PolicyOverwrite:
		return "[ovr]"
	case kubecfg.PolicySkip:
		return "[skp]"
	}
	return "[ ? ]"
}

func (d *conflictDialog) view(m *Model, maxW, maxH int) string {
	w := maxW
	iw := dialogInner(w)
	narrow := m.w < 80
	tail := " into " + safetext.Clean(d.project) + " " + m.gl.dash + " " + plural(len(d.cs), "conflict")
	file := safetext.Clean(d.shown)
	if lipgloss.Width("Import "+file+tail) > w-6 { // keep the part that matters
		file = safetext.Clean(filepath.Base(d.file))
	}
	title := "Import " + file + tail

	legend := wrap("ren keeps both (incoming gets -2) "+m.gl.dot+" ovr incoming wins "+m.gl.dot+" skp keep existing", iw)
	summary := "A backup is taken first."
	switch {
	case d.added == 1:
		summary = "1 new entity will be added. " + summary
	case d.added > 1:
		summary = fmt.Sprintf("%d new entities will be added. ", d.added) + summary
	case d.added == 0:
		summary = "Nothing new to add. " + summary
	}
	summaryRows := wrap(summary, iw)

	// Rows left for the list: borders, header, blank row, legend, summary.
	rows := maxH - 2 - 1 - 1 - len(legend) - len(summaryRows)
	if narrow {
		rows -= 3 // existing / incoming of the cursor row
	}
	rows = max(1, rows)
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+rows {
		d.offset = d.cursor - rows + 1
	}
	d.offset = max(0, min(d.offset, max(0, len(d.cs)-rows)))

	// Column widths: cursor 1, POLICY 6, KIND 8, NAME 30%, the rest split.
	const gap = 2
	nameW := max(8, iw*30/100)
	rest := iw - 1 - 6 - 8 - nameW - 4*gap
	valW := max(4, rest/2)
	inW := max(4, rest-valW)
	if narrow {
		nameW = max(4, iw-1-6-gap)
	}

	var head string
	if narrow {
		head = " " + padRight("POLICY", 6) + strings.Repeat(" ", gap) + "NAME"
	} else {
		head = " " + padRight("POLICY", 6) + strings.Repeat(" ", gap) + padRight("KIND", 8) + strings.Repeat(" ", gap) +
			padRight("NAME", nameW) + strings.Repeat(" ", gap) + padRight("EXISTING", valW) + strings.Repeat(" ", gap) + "INCOMING"
	}
	lines := []string{m.sty.dim.Render(m.fit(head, iw))}
	for i := d.offset; i < len(d.cs) && i < d.offset+rows; i++ {
		c := d.cs[i]
		pol := d.pol[c.Ref]
		pst := m.sty.bold
		if pol == "" {
			pst = m.sty.warn.Bold(true)
		}
		cur, mode := " ", rowPlain
		if i == d.cursor {
			cur, mode = m.gl.cursor, rowSelected
		}
		sp := strings.Repeat(" ", gap)
		name := safetext.Clean(c.Ref.Name)
		if narrow {
			lines = append(lines, m.row(iw, mode,
				cell{cur, m.sty.plain}, cell{padRight(policyCell(pol), 6), pst}, cell{sp, m.sty.plain},
				cell{padRight(m.truncRow(string(c.Ref.Kind)+"/"+name, nameW), nameW), m.sty.plain}))
			continue
		}
		lines = append(lines, m.row(iw, mode,
			cell{cur, m.sty.plain}, cell{padRight(policyCell(pol), 6), pst}, cell{sp, m.sty.plain},
			cell{padRight(string(c.Ref.Kind), 8) + sp, m.sty.plain},
			cell{padRight(m.truncRow(name, nameW), nameW) + sp, m.sty.plain},
			cell{padRight(m.truncRow(safetext.Clean(c.Existing), valW), valW) + sp, m.sty.plain},
			cell{m.truncRow(safetext.Clean(c.Incoming), inW), m.sty.plain}))
	}
	if narrow {
		c := d.cs[d.cursor]
		lines = append(lines, "",
			m.sty.dim.Render("existing  ")+m.truncRow(safetext.Clean(c.Existing), iw-10),
			m.sty.dim.Render("incoming  ")+m.truncRow(safetext.Clean(c.Incoming), iw-10))
	}
	lines = append(lines, "")
	for _, l := range legend {
		lines = append(lines, m.sty.dim.Render(l))
	}
	lines = append(lines, summaryRows...)
	return m.dialogBox(title, lines, w, false, 0)
}

func (d *conflictDialog) hints() []hint {
	return []hint{{"j/k", "move"}, {"r o s", "set row"}, {"R O S", "set all"}, {"enter", "apply"}, {"esc", "cancel"}}
}
