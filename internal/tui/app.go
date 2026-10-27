// Package tui is the interactive kx screen built on Bubble Tea.
//
// The Model is a value type: Update works on a copy and returns it. All
// kubeconfig knowledge lives in internal/store and internal/kubecfg; this
// package only lays out what they return and never renders secret values.
package tui

import (
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/nunoarmada/kxconfig/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/internal/store"
)

// pane identifies one of the three focusable panes.
type pane int

const (
	paneProjects pane = iota
	paneContexts
	paneDetails
	paneCount
)

func (p pane) String() string {
	return [...]string{"Projects", "Contexts", "Details"}[p]
}

type statusKind int

const (
	statusNone statusKind = iota
	statusInfo
	statusSuccess
	statusWarn
	statusError
)

type status struct {
	kind statusKind
	text string
	seq  int
}

// statusExpireMsg clears the status line when its sequence is still current.
type statusExpireMsg struct{ seq int }

// selection is the data shown for the selected project.
type selection struct {
	name string
	path string // project master file
	ctxs []kubecfg.ContextInfo
	err  error // the project does not parse
}

// Model is the root Bubble Tea model of the kx screen.
type Model struct {
	store *store.Store
	env   store.Env

	w, h    int
	isDark  bool
	profile colorprofile.Profile
	sty     styles
	gl      glyphs

	projects   []store.Project
	shells     map[string]int // live kx shells per project
	active     string         // active project when the link is OK
	link       store.LinkState
	linkTarget string // symlink target, for the foreign-link banner
	loadErr    error  // the project list could not be read
	cur        selection

	focus     pane
	plist     listState
	clist     listState
	dscroll   int // details scroll offset
	filtering bool
	finput    textinput.Model

	dialogs []dialog // overlay stack; the last one owns the keyboard
	status  status
	seq     int
}

var _ tea.Model = Model{}

// New returns the main screen for the store. It reads the projects once;
// Update reloads them after every action.
func New(s *store.Store, env store.Env) Model {
	m := Model{
		store:   s,
		env:     env,
		w:       80,
		h:       24,
		isDark:  true,
		profile: colorprofile.TrueColor,
		gl:      newGlyphs(useASCII()),
	}
	m.sty = newStyles(m.isDark, m.profile)
	if err := m.reload(); err != nil {
		m.loadErr = err
		return m
	}
	// Start on the active project, as that is what the user most likely wants.
	for i, p := range m.projects {
		if p.Active {
			m.plist.cursor = i
			m.loadSelected()
			m.settle()
			break
		}
	}
	return m
}

// Run shows the screen full-window until the user quits.
func Run(s *store.Store, env store.Env) error {
	_, err := tea.NewProgram(New(s, env)).Run()
	return err
}

// Init asks the terminal for its background colour so the palette can follow it.
func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.restyle()
	case tea.ColorProfileMsg:
		m.profile = msg.Profile
		m.restyle()
	case statusExpireMsg:
		if msg.seq == m.status.seq {
			m.status = status{seq: m.status.seq}
		}
	case tea.KeyPressMsg:
		cmd = m.handleKey(msg)
	default:
		cmd = m.forward(msg)
	}
	m.settle()
	return m, cmd
}

// restyle rebuilds the styles after the background or colour profile changed.
func (m *Model) restyle() {
	m.sty = newStyles(m.isDark, m.profile)
}

// settle keeps focus, cursors and scroll offsets valid for the current size.
func (m *Model) settle() {
	if m.focus == paneDetails && !m.layout().hasDetails() {
		m.focus = paneContexts
	}
	if len(m.projects) == 0 && m.focus != paneProjects {
		m.focus = paneProjects
	}
	lay := m.layout()
	pv, cv := m.visibleProjects(), m.visibleContexts()
	m.plist.move(m.plist.cursor, len(pv))
	m.clist.move(m.clist.cursor, len(cv))
	m.plist.scroll(len(pv), lay.projectRows(m.showsFilter(paneProjects)))
	m.clist.scroll(len(cv), lay.contextRows(m.showsFilter(paneContexts)))
}

// forward passes non-key messages (paste, cursor blink) to whoever has input.
func (m *Model) forward(msg tea.Msg) tea.Cmd {
	if len(m.dialogs) > 0 {
		return m.updateDialog(msg)
	}
	if m.filtering {
		var cmd tea.Cmd
		m.finput, cmd = m.finput.Update(msg)
		m.applyFilter(m.finput.Value())
		return cmd
	}
	return nil
}

// say sets the status line and returns the command that clears it later.
func (m *Model) say(kind statusKind, text string) tea.Cmd {
	m.seq++
	m.status = status{kind: kind, text: text, seq: m.seq}
	d := 4 * time.Second
	switch kind {
	case statusWarn:
		d = 8 * time.Second
	case statusError:
		d = 15 * time.Second
	}
	seq := m.seq
	return tea.Tick(d, func(time.Time) tea.Msg { return statusExpireMsg{seq: seq} })
}

// failed reports a store error as "Could not <what>: <cause>".
func (m *Model) failed(what string, err error) tea.Cmd {
	return m.say(statusError, "Could not "+what+": "+firstLine(err.Error()))
}

// reload re-reads everything from disk, keeping the selected project and
// context when they still exist.
func (m *Model) reload() error {
	prevProject, prevCtx := m.cur.name, m.selectedContextName()

	ps, err := m.store.Projects()
	if err != nil {
		return err
	}
	active, link, err := m.store.Active()
	if err != nil {
		return err
	}
	shells := map[string]int{}
	for _, p := range ps {
		if pids, err := m.store.LiveShells(p.Name); err == nil && len(pids) > 0 {
			shells[p.Name] = len(pids)
		}
	}
	m.projects, m.shells, m.link, m.loadErr = ps, shells, link, nil
	m.active = ""
	if link == store.LinkOK {
		m.active = active
	}
	m.linkTarget = ""
	if link == store.LinkForeignSymlink || link == store.LinkBroken {
		m.linkTarget, _ = os.Readlink(m.store.ConfigPath())
	}

	if prevProject != "" {
		for i, vi := range m.visibleProjects() {
			if m.projects[vi].Name == prevProject {
				m.plist.cursor = i
				break
			}
		}
	}
	m.plist.move(m.plist.cursor, len(m.visibleProjects()))
	m.loadSelected()
	if m.cur.name == prevProject {
		m.selectContext(prevCtx)
	}
	m.settle()
	return nil
}

// refresh reloads after an action and reports a failure on the status line.
func (m *Model) refresh() tea.Cmd {
	if err := m.reload(); err != nil {
		m.loadErr = err
		return m.failed("read projects", err)
	}
	return nil
}

// loadSelected loads the contexts of the selected project. It resets the
// contexts list when the selected project changed.
func (m *Model) loadSelected() {
	prev := m.cur.name
	m.cur = selection{}
	p, ok := m.selectedProject()
	if !ok {
		m.clist = listState{}
		return
	}
	m.cur.name, m.cur.path = p.Name, p.Path
	if p.Err != nil {
		m.cur.err = p.Err
	} else {
		path := p.Path
		if m.env.InShell && p.Name == m.env.Project {
			// Show the shell's own copy, which is what ctx changes edit here.
			if t, err := m.store.Target("", m.env); err == nil {
				path = t.Path
			}
		}
		cfg, err := kubecfg.Load(path)
		if err != nil {
			m.cur.err = err
		} else {
			m.cur.ctxs = kubecfg.Contexts(cfg)
		}
	}
	if m.cur.name != prev {
		m.clist = listState{}
		m.dscroll = 0
		for i, c := range m.cur.ctxs {
			if c.Current {
				m.clist.cursor = i
			}
		}
	}
}

// visibleProjects returns indices into m.projects that pass the filter.
func (m Model) visibleProjects() []int {
	var out []int
	for i, p := range m.projects {
		if fuzzyMatch(m.plist.filter, p.Name) {
			out = append(out, i)
		}
	}
	return out
}

// visibleContexts returns indices into m.cur.ctxs that pass the filter.
func (m Model) visibleContexts() []int {
	var out []int
	for i, c := range m.cur.ctxs {
		f := m.clist.filter
		if fuzzyMatch(f, c.Name) || fuzzyMatch(f, c.Cluster) || (f != "" && fuzzyMatch(f, c.Namespace)) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) selectedProject() (store.Project, bool) {
	v := m.visibleProjects()
	if m.plist.cursor < 0 || m.plist.cursor >= len(v) {
		return store.Project{}, false
	}
	return m.projects[v[m.plist.cursor]], true
}

func (m Model) selectedContext() (kubecfg.ContextInfo, bool) {
	v := m.visibleContexts()
	if m.clist.cursor < 0 || m.clist.cursor >= len(v) {
		return kubecfg.ContextInfo{}, false
	}
	return m.cur.ctxs[v[m.clist.cursor]], true
}

func (m Model) selectedContextName() string {
	c, _ := m.selectedContext()
	return c.Name
}

// selectContext moves the contexts cursor to name when it is visible.
func (m *Model) selectContext(name string) {
	for i, vi := range m.visibleContexts() {
		if m.cur.ctxs[vi].Name == name {
			m.clist.cursor = i
			return
		}
	}
}

// updateDialog hands msg to the top dialog. The dialog answers with what
// should occupy its slot: itself, a replacement, or nil to close.
func (m *Model) updateDialog(msg tea.Msg) tea.Cmd {
	idx := len(m.dialogs) - 1
	next, cmd := m.dialogs[idx].update(m, msg)
	if next == nil {
		m.dialogs = append(m.dialogs[:idx], m.dialogs[idx+1:]...)
	} else {
		m.dialogs[idx] = next
	}
	return cmd
}

// push opens a dialog above everything else.
func (m *Model) push(d dialog) {
	m.dialogs = append(m.dialogs, d)
}
