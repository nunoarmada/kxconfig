package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"

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
	flag := p.Name
	if m.env.InShell {
		if p.Name != m.env.Project {
			return m.say(statusWarn, "Inside a kx shell only "+m.env.Project+" can be changed; exit the shell first")
		}
		flag = ""
	}
	t, err := m.store.Target(flag, m.env)
	if err != nil {
		return m.failed("set current context", err)
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
