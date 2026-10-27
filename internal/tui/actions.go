package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nunoarmada/kxconfig/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/internal/safetext"
	"github.com/nunoarmada/kxconfig/internal/shell"
	"github.com/nunoarmada/kxconfig/internal/store"
)

// activate makes the selected project the active one (enter on a project).
// A foreign ~/.kube/config opens the adoption dialog instead.
func (m *Model) activate() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return nil
	}
	if m.env.InShell {
		return m.say(statusWarn, "Exit this kx shell to switch projects")
	}
	if p.Active && m.link == store.LinkOK {
		return m.say(statusInfo, p.Name+" is already active")
	}
	if p.Err != nil {
		return m.failed("use "+p.Name, p.Err)
	}
	err := m.store.Use(p.Name)
	switch {
	case err == nil:
		return m.afterUse("Activated " + p.Name)
	case errors.Is(err, store.ErrForeignConfig):
		m.push(newAdoptDialog(p.Name))
		return nil
	}
	return m.failed("use "+p.Name, err)
}

// afterUse reloads and reports success, or the reload failure.
func (m *Model) afterUse(msg string) tea.Cmd {
	if cmd := m.refresh(); cmd != nil {
		return cmd
	}
	return m.say(statusSuccess, msg)
}

// setContext makes the context under the cursor current (enter on a context).
// Inside a kx shell the shell's own copy is edited, never the master file.
func (m *Model) setContext() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return nil
	}
	c, ok := m.selectedContext()
	if !ok {
		return nil
	}
	t, fail := m.contextTarget(p, "set current context")
	if fail != nil {
		return fail
	}
	if err := m.store.SetContext(t, c.Name); err != nil {
		return m.failed("set current context", err)
	}
	return m.afterUse("Set current context to " + c.Name)
}

// adoptAndUse backs up a foreign ~/.kube/config (keeping it as project
// importAs when that is not empty), then activates target.
func (m *Model) adoptAndUse(importAs, target string) tea.Cmd {
	if err := m.store.AdoptForeign(importAs); err != nil {
		return m.failed("adopt "+m.tilde(m.store.ConfigPath()), err)
	}
	if err := m.store.Use(target); err != nil {
		_ = m.reload() // the adoption itself succeeded: show its result
		return m.failed("use "+target, err)
	}
	msg := "Activated " + target
	if importAs != "" && importAs != target {
		msg = "Imported " + m.tilde(m.store.ConfigPath()) + " as " + importAs + " and activated " + target
	} else if importAs == target {
		msg = "Adopted " + m.tilde(m.store.ConfigPath()) + " as " + importAs
	}
	return m.afterUse(msg)
}

// contextTarget resolves the file a context edit of p changes. Inside a kx
// shell the shell's own copy is edited, never the master file. On failure it
// returns the status line command that reports why, and the target is unusable.
func (m *Model) contextTarget(p store.Project, what string) (store.Target, tea.Cmd) {
	flag := p.Name
	if m.env.InShell {
		if p.Name != m.env.Project {
			return store.Target{}, m.say(statusWarn, "Inside a kx shell only "+m.env.Project+" can be changed; exit the shell first")
		}
		flag = ""
	}
	t, err := m.store.Target(flag, m.env)
	if err != nil {
		return store.Target{}, m.failed(what, err)
	}
	return t, nil
}

// noProject reports that an action needs a selected project.
func (m *Model) noProject() tea.Cmd {
	return m.say(statusInfo, "Select a project first")
}

// selectProject moves the project cursor to name, dropping a filter that
// hides it, and loads its contexts.
func (m *Model) selectProject(name string) {
	find := func() int {
		for i, vi := range m.visibleProjects() {
			if m.projects[vi].Name == name {
				return i
			}
		}
		return -1
	}
	i := find()
	if i < 0 && m.plist.filter != "" {
		m.plist.filter = ""
		i = find()
	}
	if i < 0 {
		return
	}
	m.plist.cursor = i
	m.loadSelected()
	m.settle()
}

// startRename opens the rename dialog for the selected project.
func (m *Model) startRename() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return m.noProject()
	}
	old := p.Name
	d := newInputDialog(m, "Rename project", "New name for "+safetext.Clean(old), old, (*Model).validName,
		func(m *Model, name string) tea.Cmd {
			if err := m.store.Rename(old, name); err != nil {
				return m.failed("rename "+old, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			m.selectProject(name)
			return m.say(statusSuccess, "Renamed "+old+" "+m.gl.arrow+" "+name)
		})
	if w := m.shellWarning(old); w != "" {
		d.note = func(*Model, string) (string, bool) { return w, true }
	}
	m.push(d)
	return nil
}

// startDelete opens the confirmation for deleting the selected project. The
// active project needs its name typed.
func (m *Model) startDelete() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return m.noProject()
	}
	name := p.Name
	clean := safetext.Clean(name)
	if m.link == store.LinkOK && m.active == name {
		m.push(newTypeConfirmDialog(m, "Delete active project", []string{
			clean + " is active. Deleting it unlinks " + m.tilde(m.store.ConfigPath()) + "; nothing will be active until you use another project.",
			"A backup is kept.",
		}, clean, "delete", func(m *Model) tea.Cmd {
			if err := m.store.Delete(name, true); err != nil {
				return m.failed("delete "+name, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			return m.say(statusSuccess, "Deleted "+name+"; no project is active")
		}))
		return nil
	}
	d := &confirmDialog{
		title: "Delete project?",
		body: []string{
			"Delete project " + clean + "?",
			"A backup is kept in " + safetext.Clean(m.tilde(filepath.Join(m.store.Backups.Dir, name))) + ".",
		},
		yes: "Delete", no: "Keep", destructive: true,
		run: func(m *Model) tea.Cmd {
			if err := m.store.Delete(name, false); err != nil {
				return m.failed("delete "+name, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			return m.say(statusSuccess, "Deleted "+name)
		},
	}
	if w := m.shellWarning(name); w != "" {
		d.warn = []string{w}
	}
	m.push(d)
	return nil
}

// shellDoneMsg arrives when a kx shell started from the TUI exits.
type shellDoneMsg struct {
	project string
	err     error
}

// editDoneMsg arrives when $EDITOR started from the TUI exits.
type editDoneMsg struct {
	project string
	err     error
}

// startShell runs a kx shell for the selected project in this terminal.
func (m *Model) startShell() tea.Cmd {
	if m.env.InShell {
		return m.say(statusWarn, "Already inside a kx shell")
	}
	p, ok := m.selectedProject()
	if !ok {
		return m.noProject()
	}
	name := p.Name
	cmd, cleanup, err := shell.Command(m.store, name, m.env)
	if err != nil {
		return m.failed("open a shell for "+name, err)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return shellDoneMsg{project: name, err: errors.Join(runError(err), cleanup())}
	})
}

// runError drops a non-zero exit status: the last command typed in a shell
// or editor failing is not a kx failure.
func runError(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return err
}

func (m *Model) shellDone(msg shellDoneMsg) tea.Cmd {
	if cmd := m.refresh(); cmd != nil {
		return cmd
	}
	if msg.err != nil {
		return m.failed("close the shell for "+msg.project, msg.err)
	}
	return m.say(statusInfo, "Left kx shell for "+msg.project)
}

// startEdit opens the selected project's file in $EDITOR (fallback vi).
func (m *Model) startEdit() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return m.noProject()
	}
	if m.env.InShell {
		return m.say(statusWarn, "Exit this kx shell to edit projects")
	}
	name := p.Name
	return tea.ExecProcess(editCommand(p.Path), func(err error) tea.Msg {
		return editDoneMsg{project: name, err: runError(err)}
	})
}

// editCommand runs $EDITOR (fallback vi) on path through sh, so an editor
// with arguments ("code -w") works and the path is never split.
func editCommand(path string) *exec.Cmd {
	ed := strings.TrimSpace(os.Getenv("EDITOR"))
	if ed == "" {
		ed = "vi"
	}
	cmd := exec.Command("sh", "-c", `exec $EDITOR "$1"`, "sh", path)
	cmd.Env = append(os.Environ(), "EDITOR="+ed)
	return cmd
}

func (m *Model) editDone(msg editDoneMsg) tea.Cmd {
	if cmd := m.refresh(); cmd != nil {
		return cmd
	}
	m.selectProject(msg.project)
	if msg.err != nil {
		return m.failed("run the editor", msg.err)
	}
	return m.say(statusSuccess, "Edited "+msg.project)
}

var nsRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// startNamespace asks for the namespace of the current context. Enter with an
// empty value clears it.
func (m *Model) startNamespace() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return nil
	}
	cur, hasCur := m.currentContext()
	if !hasCur {
		return m.say(statusWarn, "No current context: press enter on a context first")
	}
	t, fail := m.contextTarget(p, "set namespace")
	if fail != nil {
		return fail
	}
	clean := safetext.Clean(cur.Name)
	d := newInputDialog(m, "Set namespace", "Namespace for current context "+clean, cur.Namespace,
		func(_ *Model, v string) string {
			if !nsRe.MatchString(v) {
				return "Use lowercase letters, digits and - (max 63)"
			}
			return ""
		},
		func(m *Model, ns string) tea.Cmd {
			if err := m.store.SetNamespace(t, ns); err != nil {
				return m.failed("set namespace", err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			if ns == "" {
				return m.say(statusSuccess, "Cleared namespace of "+cur.Name)
			}
			return m.say(statusSuccess, "Set namespace of "+cur.Name+" to "+ns)
		})
	d.allowEmpty = true
	d.help = "Leave empty to clear"
	if sel, ok := m.selectedContext(); ok && sel.Name != cur.Name {
		d.note = func(*Model, string) (string, bool) {
			return "Applies to the current context, not " + safetext.Clean(sel.Name) + ".", false
		}
	}
	m.push(d)
	return nil
}

// currentContext is the current context of the selected project.
func (m Model) currentContext() (kubecfg.ContextInfo, bool) {
	for _, c := range m.cur.ctxs {
		if c.Current {
			return c, true
		}
	}
	return kubecfg.ContextInfo{}, false
}

// startDeleteContext confirms deleting the context under the cursor, naming
// the cluster and user that would become unused and go with it.
func (m *Model) startDeleteContext() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return nil
	}
	c, ok := m.selectedContext()
	if !ok {
		return nil
	}
	if m.env.InShell && p.Name == m.env.Project {
		return m.say(statusWarn, "Exit this kx shell to delete contexts of "+p.Name)
	}
	cfg, err := kubecfg.Load(p.Path)
	if err != nil {
		return m.failed("read "+p.Name, err)
	}
	clusters, users := kubecfg.Orphans(cfg, c.Name)
	body := []string{"Delete context " + safetext.Clean(c.Name) + " from " + safetext.Clean(p.Name) + "?"}
	if len(clusters)+len(users) > 0 {
		body = append(body, "Also removes unused:")
	}
	d := &confirmDialog{title: "Delete context?", yes: "Delete", no: "Keep", destructive: true}
	for _, n := range clusters {
		body = append(body, "  cluster  "+safetext.Clean(n))
	}
	for _, n := range users {
		body = append(body, "  user     "+safetext.Clean(n))
	}
	d.body = append(body, "A backup is taken first.")
	if w := m.shellWarning(p.Name); w != "" {
		d.warn = []string{w}
	}
	project, ctx := p.Name, c.Name
	d.run = func(m *Model) tea.Cmd {
		if _, err := m.store.RemoveContext(project, ctx, false); err != nil {
			return m.failed("delete context "+ctx, err)
		}
		if cmd := m.refresh(); cmd != nil {
			return cmd
		}
		return m.say(statusSuccess, fmt.Sprintf("Deleted context %s from %s", ctx, project))
	}
	m.push(d)
	return nil
}

// reloadKey re-reads everything from disk (ctrl+r and the reload command).
func (m *Model) reloadKey() tea.Cmd {
	if err := m.reload(); err != nil {
		m.loadErr = err
		return m.failed("read projects", err)
	}
	return m.say(statusInfo, "Reloaded from disk")
}
