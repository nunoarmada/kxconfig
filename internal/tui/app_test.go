package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/nunoarmada/kxconfig/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/internal/store"
)

func TestEnterActivatesProject(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	writeProject(t, root, "staging", kubeconfig("stg", stg), 0o600)
	m := open(t, root, 100, 30)
	mustContain(t, plain(m), "active: none")

	m = keys(m, "enter")
	s := store.New(root)
	if name, st, err := s.Active(); err != nil || st != store.LinkOK || name != "prod" {
		t.Fatalf("Active() = %q, %v, %v; want prod, LinkOK", name, st, err)
	}
	v := plain(m)
	mustContain(t, v, "active: ● prod", "✓ Activated prod")

	m = keys(m, "down", "enter")
	if name, _, _ := s.Active(); name != "staging" {
		t.Fatalf("active = %q, want staging", name)
	}
	mustContain(t, plain(m), "active: ● staging", "✓ Activated staging")
	if prev, _ := s.Previous(); prev != "prod" {
		t.Errorf("previous = %q, want prod", prev)
	}
}

func TestEnterOnInvalidProjectReportsError(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "up", "enter") // legacy does not parse
	mustContain(t, plain(m), "✗ Could not use legacy:")
	if name, _, _ := m.store.Active(); name != "prod" {
		t.Errorf("active = %q, want prod unchanged", name)
	}
}

func TestFocusSwitchAndContextEnter(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	if m.focus != paneProjects {
		t.Fatalf("initial focus = %v", m.focus)
	}
	m = keys(m, "tab")
	if m.focus != paneContexts {
		t.Fatalf("focus after tab = %v, want contexts", m.focus)
	}
	mustContain(t, plain(m), "CONTEXTS prod 2", "★ prod-eu")

	m = keys(m, "j", "enter")
	cfg, err := kubecfg.Load(filepath.Join(root, "config-prod"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "prod-us" {
		t.Fatalf("current-context = %q, want prod-us", cfg.CurrentContext)
	}
	v := plain(m)
	mustContain(t, v, "✓ Set current context to prod-us")
	if !strings.Contains(lineWith(v, "prod-us"), "★") {
		t.Errorf("star did not move to prod-us:\n%s", v)
	}

	// h / l / arrows / shift+tab move focus without wrapping or panicking.
	m = keys(m, "l")
	if m.focus != paneDetails {
		t.Errorf("focus after l = %v, want details", m.focus)
	}
	m = keys(m, "l")
	if m.focus != paneDetails {
		t.Errorf("l must not wrap, focus = %v", m.focus)
	}
	m = keys(m, "shift+tab", "left")
	if m.focus != paneProjects {
		t.Errorf("focus = %v, want projects", m.focus)
	}
}

func TestSelectingProjectShowsItsContexts(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "g") // dev
	mustContain(t, plain(m), "CONTEXTS dev 1", "dev-cluster")
	m = keys(m, "tab")
	mustContain(t, plain(m), "context   dev", "server    https://dev.example.com:6443", "auth      basic")
	m = keys(m, "shift+tab", "G") // staging
	mustContain(t, plain(m), "CONTEXTS staging 1", "stg-cluster")
}

func TestFilterNarrowsProjects(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "/")
	if !m.filtering {
		t.Fatal("/ did not start filtering")
	}
	m = typeText(m, "stg")
	v := plain(m)
	mustContain(t, v, "PROJECTS 1/4", "staging")
	for _, gone := range []string{" dev ", "legacy"} {
		if strings.Contains(paneText(v, 0, 30), gone) {
			t.Errorf("projects pane still lists %q:\n%s", gone, v)
		}
	}
	mustContain(t, v, "type to filter  enter keep  esc clear")
	// The contexts pane follows the selected (only) match.
	mustContain(t, v, "CONTEXTS staging 1")

	m = keys(m, "enter")
	if m.filtering || m.plist.filter != "stg" {
		t.Errorf("enter must keep the filter: filtering=%v filter=%q", m.filtering, m.plist.filter)
	}
	m = keys(m, "esc") // clears the kept filter, does not quit
	if m.plist.filter != "" {
		t.Errorf("esc left filter %q", m.plist.filter)
	}
	mustContain(t, plain(m), "PROJECTS 4", "legacy")

	// esc while typing clears and leaves the input.
	m = keys(m, "/", "d", "esc")
	if m.filtering || m.plist.filter != "" {
		t.Errorf("esc in filter: filtering=%v filter=%q", m.filtering, m.plist.filter)
	}
}

func TestFilterContexts(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "tab", "/")
	m = typeText(m, "us")
	v := plain(m)
	mustContain(t, v, "CONTEXTS prod 1/2", "prod-us")
	if strings.Contains(lineWith(v, "NAME"), "eks-eu") || strings.Contains(v, "prod-eu  ") {
		t.Errorf("prod-eu should be filtered out:\n%s", v)
	}
}

func TestEscQuitsAtRootAndCtrlCAlways(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	if _, cmd := sendCmd(m, press("esc")); !isQuit(cmd) {
		t.Error("esc at root must quit")
	}
	if _, cmd := sendCmd(m, press("q")); !isQuit(cmd) {
		t.Error("q must quit")
	}
	// Overlay open: esc closes only the overlay, ctrl+c still quits.
	m = keys(m, "?")
	if _, cmd := sendCmd(m, press("esc")); isQuit(cmd) {
		t.Error("esc in help must not quit")
	}
	if _, cmd := sendCmd(m, press("ctrl+c")); !isQuit(cmd) {
		t.Error("ctrl+c must quit from an overlay")
	}
	f := keys(open(t, fixture(t), 100, 30), "/")
	if _, cmd := sendCmd(f, press("ctrl+c")); !isQuit(cmd) {
		t.Error("ctrl+c must quit while filtering")
	}
	if _, cmd := sendCmd(f, press("q")); isQuit(cmd) {
		t.Error("q while filtering is text, not quit")
	}
}

func TestEmptyState(t *testing.T) {
	root := testRoot(t)
	m := open(t, root, 100, 30)
	v := plain(m)
	mustContain(t, v, "No projects yet", "~/.kube/config-<name>", "i   import a kubeconfig file", "q   quit")
	mustNotContain(t, v, "PROJECTS", "adopt the current")

	// With a foreign ~/.kube/config the adopt row appears and `a` opens the name input.
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(kubeconfig("dev", devCtx)), 0o600); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "ctrl+r")
	v = plain(m)
	mustContain(t, v, "a   adopt the current ~/.kube/config as a project", "not managed by kx yet")
	m = keys(m, "a")
	mustContain(t, plain(m), "Import current config", "> default")
	m = keys(m, "enter")
	if name, st, _ := m.store.Active(); st != store.LinkOK || name != "default" {
		t.Fatalf("after adopt: active %q state %v", name, st)
	}
	mustContain(t, plain(m), "active: ● default", "PROJECTS 1")
}

func TestSmallTerminalSinglePane(t *testing.T) {
	m := open(t, fixture(t), 70, 30) // < 80 columns
	v := plain(m)
	mustContain(t, v, "PROJECTS 4", "1/3 tab ▸", "kx › Projects")
	mustNotContain(t, v, "CONTEXTS", "DETAILS")

	m = keys(m, "tab")
	v = plain(m)
	mustContain(t, v, "CONTEXTS prod 2", "2/3 tab ▸", "kx › prod › Contexts")
	mustNotContain(t, v, "PROJECTS")
	// Below 50 columns the CLUSTER column is dropped.
	m = send(m, tea.WindowSizeMsg{Width: 45, Height: 30})
	v = plain(m)
	mustContain(t, v, "NAMESPACE")
	mustNotContain(t, v, "CLUSTER", "eks-eu")

	m = keys(m, "tab")
	mustContain(t, plain(m), "DETAILS", "3/3 tab ▸")
	m = keys(m, "tab")
	mustContain(t, plain(m), "PROJECTS")

	// Short terminals (< 24 rows) are single-pane too, even when wide.
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 20})
	mustNotContain(t, plain(m), "CONTEXTS")

	// Too small: only the notice.
	m = send(m, tea.WindowSizeMsg{Width: 39, Height: 12})
	if got := plain(m); got != "Terminal too small (39x12) — need 40×10" {
		t.Errorf("too-small view = %q", got)
	}
	m = send(m, tea.WindowSizeMsg{Width: 20, Height: 5})
	if got := plain(m); got != "Terminal too small (" {
		t.Errorf("notice must truncate to width: %q", got)
	}
}

func TestLayoutsByWidth(t *testing.T) {
	root := fixture(t)
	wide := plain(open(t, root, 130, 30))
	mustContain(t, wide, "PROJECTS 4", "CONTEXTS prod 2", "DETAILS")
	if lineWith(wide, "PROJECTS") != lineWith(wide, "DETAILS") {
		t.Errorf("wide layout: panes should share a row:\n%s", wide)
	}
	med := plain(open(t, root, 90, 30))
	mustContain(t, med, "PROJECTS 4", "CONTEXTS prod 2", "DETAILS")
	if lineWith(med, "PROJECTS") != lineWith(med, "CONTEXTS") || lineWith(med, "CONTEXTS") == lineWith(med, "DETAILS") {
		t.Errorf("medium layout: details should sit below contexts:\n%s", med)
	}
	// Medium with a short body hides details and tab skips it.
	m := open(t, root, 90, 24)
	m = send(m, tea.WindowSizeMsg{Width: 90, Height: 24})
	if !strings.Contains(plain(m), "DETAILS") {
		t.Fatalf("24 rows leaves 21 body rows: details expected:\n%s", plain(m))
	}
}

func TestResizeNoPanic(t *testing.T) {
	root := fixture(t)
	check := func(t *testing.T, m Model, w, h int) Model {
		t.Helper()
		m = send(m, tea.WindowSizeMsg{Width: w, Height: h})
		v := m.View().Content
		for i, line := range strings.Split(v, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("%dx%d line %d is %d wide: %q", w, h, i, lw, line)
			}
		}
		if n := strings.Count(v, "\n") + 1; n > h {
			t.Fatalf("%dx%d view has %d lines", w, h, n)
		}
		return m
	}

	// Every size from 1x1 to 200x60 on the plain screen.
	t.Run("sweep", func(t *testing.T) {
		m := open(t, root, 100, 30)
		for w := 1; w <= 200; w++ {
			for h := 1; h <= 60; h++ {
				m = check(t, m, w, h)
			}
		}
	})

	// Overlays, filtering, focus and status at a spread of sizes.
	ws := []int{1, 5, 20, 39, 40, 45, 49, 50, 60, 79, 80, 100, 119, 120, 140, 200}
	hs := []int{1, 5, 9, 10, 15, 20, 23, 24, 30, 60}
	states := map[string]func(Model) Model{
		"contexts":  func(m Model) Model { return keys(m, "tab") },
		"details":   func(m Model) Model { return keys(m, "tab", "tab") },
		"filtering": func(m Model) Model { return keys(m, "/", "s") },
		"help":      func(m Model) Model { return keys(m, "?") },
		"status":    func(m Model) Model { return keys(m, "down", "enter") },
		"error":     func(m Model) Model { return keys(m, "up", "enter") },
	}
	for name, setup := range states {
		t.Run(name, func(t *testing.T) {
			m := setup(open(t, root, 100, 30))
			for _, w := range ws {
				for _, h := range hs {
					m = check(t, m, w, h)
				}
			}
		})
	}
	t.Run("adoption", func(t *testing.T) {
		r := testRoot(t)
		writeProject(t, r, "prod", kubeconfig("prod-eu", prodEU), 0o600)
		if err := os.WriteFile(filepath.Join(r, "config"), []byte(kubeconfig("x", devCtx)), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"enter", "i"} {
			m := keys(open(t, r, 100, 30), "enter")
			if k == "i" {
				m = keys(m, "i")
			}
			for _, w := range ws {
				for _, h := range hs {
					m = check(t, m, w, h)
				}
			}
		}
	})
}

func TestBriefSizesHaveFullFrame(t *testing.T) {
	root := fixture(t)
	for _, sz := range [][2]int{{40, 10}, {60, 20}, {79, 23}, {80, 24}, {119, 30}, {120, 40}, {200, 60}} {
		w, h := sz[0], sz[1]
		m := open(t, root, w, h)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines, want %d", w, h, len(lines), h)
		}
		for i, l := range lines {
			if lipgloss.Width(l) != w {
				t.Errorf("%dx%d line %d is %d wide", w, h, i, lipgloss.Width(l))
				break
			}
		}
		if !strings.Contains(plain(m), "? help") {
			t.Errorf("%dx%d: no '? help' in view", w, h)
		}
	}
}

func TestZeroContextsProject(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "empty", kubeconfig(""), 0o600)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	m := open(t, root, 100, 30) // cursor on "empty"
	v := plain(m)
	mustContain(t, v, "CONTEXTS empty 0", "No contexts in empty.")
	if got := lineWith(v, "DETAILS"); got == "" {
		t.Fatalf("no details pane:\n%s", v)
	}
	// Enter in the contexts pane, scrolling and filtering must not panic or change anything.
	m = keys(m, "tab", "enter", "j", "k", "G", "g", "/", "x", "esc", "tab", "j", "k", "enter")
	cfg, err := kubecfg.Load(filepath.Join(root, "config-empty"))
	if err != nil || cfg.CurrentContext != "" {
		t.Fatalf("config-empty changed: %v %+v", err, cfg)
	}
	mustContain(t, plain(m), "No contexts in empty.")

	// Single pane at the smallest allowed size too.
	m = send(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = keys(m, "shift+tab")
	mustContain(t, plain(m), "No contexts in empty.")
}

func TestInvalidProjectShowsCause(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "up")
	v := plain(m)
	mustContain(t, v, "⚠ invalid", "⚠ Cannot parse config-legacy:", "Press e to edit it.", "No contexts in legacy.")
}

func TestBadgesAndMarkers(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	v := plain(m)
	mustContain(t, v, "▸● prod", "⚠ 0644", "⚠ invalid")
	mustNotContain(t, v, "0600")
	if !strings.Contains(lineWith(v, "dev"), "⚠ 0644") {
		t.Errorf("dev should carry the permissions badge:\n%s", v)
	}

	// Live kx shell count is shown as $N.
	if err := os.MkdirAll(m.store.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	shellCopy := m.store.ShellCopyPath(pid, "staging")
	if err := os.WriteFile(shellCopy, []byte(kubeconfig("stg", stg)), 0o600); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "ctrl+r")
	if !strings.Contains(lineWith(plain(m), "staging"), "$1") {
		t.Errorf("staging should show $1:\n%s", plain(m))
	}
}

func TestViewHasNoSecrets(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "tok", kubeconfig("c1", ctxSpec{"c1", "cl1", "https://one.example", "u1", "", "token"}), 0o600)
	writeProject(t, root, "cert", kubeconfig("c2", ctxSpec{"c2", "cl2", "https://two.example", "u2", "", "cert"}), 0o600)
	writeProject(t, root, "basic", kubeconfig("c3", ctxSpec{"c3", "cl3", "https://three.example", "u3", "", "basic"}), 0o600)
	writeProject(t, root, "exec", kubeconfig("c4", prodEU), 0o600)
	link(t, root, "tok")
	secrets := []string{secretToken, secretKey, secretCert, secretPass, "KEY-DATA-SECRET", "CERT-DATA-SECRET"}

	for _, size := range [][2]int{{200, 60}, {130, 30}, {100, 30}, {80, 24}, {60, 20}, {40, 10}} {
		m := open(t, root, size[0], size[1])
		for step := 0; step < 4; step++ { // each project, contexts, details, help
			for _, focus := range [][]string{nil, {"tab"}, {"tab"}} {
				m = keys(m, focus...)
				for _, s := range secrets {
					if strings.Contains(m.View().Content, s) {
						t.Fatalf("%v: secret %q leaked:\n%s", size, s, plain(m))
					}
				}
			}
			m = keys(m, "tab", "down")
		}
		m = keys(m, "?")
		for _, s := range secrets {
			if strings.Contains(m.View().Content, s) {
				t.Fatalf("secret %q in help", s)
			}
		}
	}
	// The auth kind is shown instead.
	m := open(t, root, 130, 30)
	mustContain(t, plain(m), "auth      token")
	m = keys(m, "g")
	mustContain(t, plain(m), "auth      basic")
}

func TestHelpOverlay(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "?")
	v := plain(m)
	mustContain(t, v, "Keys", "NAVIGATION", "PROJECTS", "CONTEXTS", "PALETTE",
		"ctrl+r", "reload from disk", "D", "delete (backup kept)", "set namespace", "esc close")
	if len(m.dialogs) != 1 {
		t.Fatalf("dialogs = %d", len(m.dialogs))
	}
	// The overlay captures keys: j does not move the project cursor.
	before := m.plist.cursor
	m = keys(m, "j", "tab", "enter")
	if m.plist.cursor != before || m.focus != paneProjects || len(m.dialogs) != 1 {
		t.Errorf("help overlay leaked keys: cursor=%d focus=%v dialogs=%d", m.plist.cursor, m.focus, len(m.dialogs))
	}
	m = keys(m, "esc")
	if len(m.dialogs) != 0 {
		t.Fatal("esc must close help")
	}
	mustNotContain(t, plain(m), "NAVIGATION")

	// Short terminal: help scrolls instead of overflowing.
	m = send(open(t, fixture(t), 80, 24), press("?"))
	v1 := plain(m)
	m = keys(m, "G")
	v2 := plain(m)
	if v1 == v2 || !strings.Contains(v2, "PALETTE") {
		t.Errorf("help did not scroll to the end:\n%s", v2)
	}
}

func TestHintBar(t *testing.T) {
	m := open(t, fixture(t), 140, 30)
	mustContain(t, lastLine(plain(m)), "enter use  / filter  i import  r rename  D delete  b backups  s shell  e edit  : commands  ? help")
	m = keys(m, "tab")
	mustContain(t, lastLine(plain(m)), "enter set current  n namespace  d delete  / filter  : commands  ? help")
	m = keys(m, "tab")
	mustContain(t, lastLine(plain(m)), "tab next pane  : commands  ? help")
	// Narrow: items drop from the right, "? help" stays.
	n := send(open(t, fixture(t), 50, 24), press("tab"))
	if got := strings.TrimSpace(lastLine(plain(n))); !strings.HasSuffix(got, "? help") || !strings.HasPrefix(got, "enter set current") {
		t.Errorf("narrow hint bar = %q", got)
	}
}

func TestBanners(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	cfg := filepath.Join(root, "config")

	if err := os.Symlink("config-old", cfg); err != nil {
		t.Fatal(err)
	}
	v := plain(open(t, root, 120, 30))
	mustContain(t, v, "⚠ ~/.kube/config points to a missing project (config-old). Press enter on a project to fix it.")
	narrow := plain(open(t, root, 70, 30))
	mustContain(t, narrow, "points to a missing project (config-old).")
	mustNotContain(t, narrow, "Press enter on a project")
	_ = os.Remove(cfg)

	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(other, cfg); err != nil {
		t.Fatal(err)
	}
	mustContain(t, plain(open(t, root, 200, 30)), "⚠ ~/.kube/config is a link kx doesn't manage (→ "+other+"). kx will not change it.")
	_ = os.Remove(cfg)

	if err := os.WriteFile(cfg, []byte(kubeconfig("x", devCtx)), 0o600); err != nil {
		t.Fatal(err)
	}
	mustContain(t, plain(open(t, root, 120, 30)), "· ~/.kube/config is not managed by kx yet. Using a project backs it up first.")
}

func TestAdoptionDialog(t *testing.T) {
	setup := func(t *testing.T) (string, Model) {
		root := testRoot(t)
		writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
		foreign := kubeconfig("mine", ctxSpec{"mine", "mine-cluster", "https://mine.example", "me", "", "token"})
		if err := os.WriteFile(filepath.Join(root, "config"), []byte(foreign), 0o600); err != nil {
			t.Fatal(err)
		}
		m := keys(open(t, root, 100, 30), "enter")
		return root, m
	}

	t.Run("dialog", func(t *testing.T) {
		root, m := setup(t)
		v := plain(m)
		mustContain(t, v, "Use prod?", "~/.kube/config is not managed by kx.", "~/.kube/kx/backups/_initial first.",
			"i    Import it as a new project (name)", "r    Replace it with a link to prod", "esc  Cancel",
			"i import as project  r replace (backed up)  esc cancel")
		fi, _ := os.Lstat(filepath.Join(root, "config"))
		if fi.Mode()&os.ModeSymlink != 0 {
			t.Fatal("nothing may change before a choice is made")
		}
		m = keys(m, "esc")
		if len(m.dialogs) != 0 {
			t.Error("esc must cancel")
		}
		if fi, _ := os.Lstat(filepath.Join(root, "config")); fi.Mode()&os.ModeSymlink != 0 {
			t.Error("cancel must leave the file alone")
		}
	})

	t.Run("replace", func(t *testing.T) {
		root, m := setup(t)
		m = keys(m, "r")
		if name, st, _ := m.store.Active(); st != store.LinkOK || name != "prod" {
			t.Fatalf("active = %q, %v", name, st)
		}
		mustContain(t, plain(m), "✓ Activated prod", "active: ● prod")
		backups, err := m.store.Backups.List("_initial")
		if err != nil || len(backups) != 1 {
			t.Fatalf("_initial backups = %v, %v", backups, err)
		}
		if _, err := os.Stat(filepath.Join(root, "config-mine")); err == nil {
			t.Error("replace must not import the file")
		}
	})

	t.Run("import", func(t *testing.T) {
		_, m := setup(t)
		m = keys(m, "i")
		mustContain(t, plain(m), "Import current config", "Name for the imported project", "> default")
		// Invalid names are refused inline and enter is disabled.
		m = typeText(keys(m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace"), "-bad")
		mustContain(t, plain(m), "✗ Use letters, digits, . _ - (max 64)")
		m = keys(m, "enter")
		if len(m.dialogs) != 1 {
			t.Fatal("enter must stay disabled while the name is invalid")
		}
		m = keys(m, "backspace", "backspace", "backspace", "backspace")
		m = typeText(m, "mine")
		m = keys(m, "enter")
		v := plain(m)
		mustContain(t, v, "✓ Imported ~/.kube/config as mine and activated prod", "PROJECTS 2")
		if name, _, _ := m.store.Active(); name != "prod" {
			t.Errorf("active = %q", name)
		}
		ps, _ := m.store.Projects()
		if len(ps) != 2 {
			t.Errorf("projects = %+v", ps)
		}
	})

	t.Run("name taken", func(t *testing.T) {
		_, m := setup(t)
		m = typeText(keys(m, "i", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace"), "prod")
		mustContain(t, plain(m), "✗ Project prod already exists")
		m = keys(m, "enter")
		if len(m.dialogs) != 1 {
			t.Error("enter must stay disabled while the name is taken")
		}
	})
}

func TestShellMode(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	env := store.Env{InShell: true, Project: "staging", Kubeconfig: s.ShellCopyPath(os.Getpid(), "staging")}
	if err := os.MkdirAll(s.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Kubeconfig, []byte(kubeconfig("stg", stg, ctxSpec{"stg2", "stg-cluster", "https://stg.example.com:6443", "stg-user", "x", "cert"})), 0o600); err != nil {
		t.Fatal(err)
	}
	m := send(New(s, env), tea.WindowSizeMsg{Width: 120, Height: 30})
	v := plain(m)
	mustContain(t, v, "kx  $ shell: staging  ·  global: prod",
		"Inside kx shell for staging: context/namespace changes stay in this shell. Exit it to switch projects.")

	// Enter on a project and `s` are refused.
	m = keys(m, "enter")
	mustContain(t, plain(m), "⚠ Exit this kx shell to switch projects")
	if name, _, _ := s.Active(); name != "prod" {
		t.Errorf("active changed to %q inside a shell", name)
	}
	m = keys(m, "s")
	mustContain(t, plain(m), "⚠ Already inside a kx shell")

	// Another project's contexts are refused: nothing else may be edited.
	m = keys(m, "tab", "enter") // cursor is on prod
	mustContain(t, plain(m), "Inside a kx shell only staging can be changed; exit the shell first")
	if c, _ := kubecfg.Load(filepath.Join(root, "config-prod")); c.CurrentContext != "prod-eu" {
		t.Errorf("prod master edited from inside a shell: %q", c.CurrentContext)
	}
	m = keys(m, "shift+tab")

	// Context changes edit the shell's copy, never the master file.
	m = keys(m, "down", "tab") // staging; its contexts come from the shell's copy
	mustContain(t, plain(m), "CONTEXTS staging 2", "stg2")
	m = keys(m, "j", "enter")
	mustContain(t, plain(m), "✓ Set current context to stg2")
	shell, err := kubecfg.Load(env.Kubeconfig)
	if err != nil || shell.CurrentContext != "stg2" {
		t.Errorf("shell copy current-context = %q, %v", shell.CurrentContext, err)
	}
	master, err := kubecfg.Load(filepath.Join(root, "config-staging"))
	if err != nil || master.CurrentContext != "stg" {
		t.Errorf("master current-context = %q, %v; must stay stg", master.CurrentContext, err)
	}
}

func TestStatusLifecycle(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m, cmd := sendCmd(m, press("d"))
	if cmd == nil {
		t.Fatal("status must schedule its expiry")
	}
	mustContain(t, plain(m), "· Use D to delete a project")
	seq := m.status.seq

	// A stale tick does not clear a newer message.
	m, _ = sendCmd(m, press("d"))
	m = send(m, statusExpireMsg{seq: seq})
	mustContain(t, plain(m), "· Use D to delete a project")
	m = send(m, statusExpireMsg{seq: m.status.seq})
	mustNotContain(t, plain(m), "Use D to delete")

	// Errors clear on the next action key but survive plain navigation.
	m = keys(m, "up", "enter")
	mustContain(t, plain(m), "✗ Could not use legacy")
	m = keys(m, "k")
	mustContain(t, plain(m), "✗ Could not use legacy")
	m = keys(m, "r")
	mustNotContain(t, plain(m), "Could not use legacy")
	mustContain(t, plain(m), "Rename is not available yet")
}

func TestReloadKey(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	writeProject(t, root, "newproj", kubeconfig("n", devCtx), 0o600)
	mustNotContain(t, plain(m), "newproj")
	m = keys(m, "ctrl+r")
	mustContain(t, plain(m), "newproj", "· Reloaded from disk", "PROJECTS 5")
}

func TestStoreErrorsGoToStatusLine(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	mustContain(t, plain(m), "▸● prod")
	m = keys(m, "enter")
	mustContain(t, plain(m), "· prod is already active")

	// A project deleted behind the UI's back: enter reports, never panics.
	if err := os.Remove(filepath.Join(root, "config-staging")); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "down", "enter")
	mustContain(t, plain(m), "✗ Could not use staging:")
	// A foreign symlink is refused with a message, not a dialog.
	if err := os.Remove(filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "x"), filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "ctrl+r", "g", "enter") // dev
	mustContain(t, plain(m), "✗ Could not use dev:")
	if len(m.dialogs) != 0 {
		t.Error("no dialog expected for a foreign symlink")
	}

	// The whole root disappearing is shown as an error state, not a crash.
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "ctrl+r")
	mustContain(t, plain(m), "No projects yet")
	if err := os.WriteFile(root, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = keys(m, "ctrl+r")
	mustContain(t, plain(m), "Could not read projects", "ctrl+r try again")
	_ = send(m, tea.WindowSizeMsg{Width: 40, Height: 10}).View()
}

func TestNoColorKeepsGlyphs(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = send(m, tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	raw := m.View().Content
	if strings.Contains(raw, "38;") || strings.Contains(raw, "48;") || strings.Contains(raw, "\x1b[3") {
		t.Errorf("NO_COLOR view contains colour sequences: %q", raw[:min(len(raw), 400)])
	}
	v := plain(m)
	mustContain(t, v, "▸● prod", "★ prod-eu", "⚠ invalid", "⚠ 0644", "PROJECTS 4")
	// The cursor row is reverse video, the focused title bold and underlined.
	if !strings.Contains(raw, "\x1b[7") && !strings.Contains(raw, ";7") {
		t.Errorf("selection must use reverse video without colour")
	}
	if !strings.Contains(raw, "4;") && !strings.Contains(raw, "[4m") && !strings.Contains(raw, ";4;") && !strings.Contains(raw, ";4m") {
		t.Errorf("focused title must be underlined")
	}

	// 16 colours: reverse selection, but colours are kept.
	a := send(open(t, fixture(t), 100, 30), tea.ColorProfileMsg{Profile: colorprofile.ANSI})
	if !strings.Contains(a.View().Content, "\x1b[") {
		t.Error("expected styled output")
	}
	if a.sty.mode != modeANSI {
		t.Error("ANSI profile must select modeANSI")
	}
}

func TestBackgroundDetectionRebuildsStyles(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	dark := m.View().Content
	m = send(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	if m.isDark {
		t.Fatal("white background must switch to light")
	}
	if light := m.View().Content; light == dark {
		t.Error("light and dark palettes should render differently")
	}
	if got := plain(m); got != plain(send(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})) {
		t.Error("text must not depend on the palette")
	}
}

func TestASCIIMode(t *testing.T) {
	root := fixture(t)
	t.Setenv("KX_ASCII", "1")
	m := send(open(t, root, 100, 30), press("tab"))
	v := plain(m)
	for _, r := range v {
		if r > 127 {
			t.Fatalf("non-ASCII rune %q in ASCII mode:\n%s", r, v)
		}
	}
	mustContain(t, v, "+--", ">@ prod-eu", "! invalid", "! 0644", "active: * prod")
	mustContain(t, plain(open(t, root, 100, 30)), ">* prod")
	// Help and the small-terminal notice are ASCII too.
	for _, s := range []Model{keys(m, "?"), send(m, tea.WindowSizeMsg{Width: 30, Height: 8})} {
		for _, r := range plain(s) {
			if r > 127 {
				t.Fatalf("non-ASCII rune %q:\n%s", r, plain(s))
			}
		}
	}

	// A non-UTF-8 locale selects ASCII without KX_ASCII.
	t.Setenv("KX_ASCII", "")
	t.Setenv("LANG", "C")
	if !newGlyphs(useASCII()).ascii {
		t.Error("LANG=C must select ASCII glyphs")
	}
	t.Setenv("LC_ALL", "en_US.utf8")
	if useASCII() {
		t.Error("LC_ALL=en_US.utf8 must select UTF-8 glyphs")
	}
}

func TestCtrlRReloadsAfterExternalChange(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	// Switch the link behind the UI's back.
	if err := os.Remove(filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	link(t, root, "staging")
	mustContain(t, plain(m), "active: ● prod")
	m = keys(m, "ctrl+r")
	mustContain(t, plain(m), "active: ● staging")
}

func TestFuzzyMatch(t *testing.T) {
	for _, c := range []struct {
		pat, s string
		want   bool
	}{
		{"", "x", true}, {"stg", "staging", true}, {"STG", "staging", true},
		{"gts", "staging", false}, {"prod", "prod-eu", true}, {"x", "", false},
	} {
		if got := fuzzyMatch(c.pat, c.s); got != c.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v", c.pat, c.s, got)
		}
	}
}

// TestGolden100x30 pins the main screen at 100x30 through a real Bubble Tea
// program with a fixed colour profile, so the snapshot is deterministic.
func TestGolden100x30(t *testing.T) {
	root := fixture(t)
	tm := teatest.NewTestModel(t, New(store.New(root), store.Env{}),
		teatest.WithInitialTermSize(100, 30),
		teatest.WithProgramOptions(tea.WithColorProfile(colorprofile.TrueColor)),
	)
	tm.Send(press("j"))
	tm.Send(press("tab"))
	tm.Send(press("j"))
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Model)
	if !ok {
		t.Fatal("final model has unexpected type")
	}
	golden.RequireEqual(t, []byte(final.View().Content))
}

// lineWith returns the first line of v containing sub.
func lineWith(v, sub string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func lastLine(v string) string {
	l := strings.Split(v, "\n")
	return l[len(l)-1]
}

// paneText returns the first n columns of every line of v.
func paneText(v string, _, n int) string {
	var b strings.Builder
	for _, l := range strings.Split(v, "\n") {
		r := []rune(l)
		b.WriteString(string(r[:min(len(r), n)]))
		b.WriteByte('\n')
	}
	return b.String()
}

type bareDialog struct{}

func (bareDialog) update(*Model, tea.Msg) (dialog, tea.Cmd) { return nil, nil }
func (bareDialog) view(m *Model, w, _ int) string {
	return m.dialogBox("Bare", []string{"x"}, min(w, 20), false, 0)
}
func (bareDialog) hints() []hint { return nil }

func TestHintBarSurvivesDialogWithoutHints(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m.push(bareDialog{})
	if got := lastLine(plain(m)); !strings.Contains(got, "? help") {
		t.Errorf("hint bar = %q, want the pinned help hint", got)
	}
}

var sgrRe = regexp.MustCompile(`^\[[0-9;:]*m`)

func TestKubeconfigTextCannotInjectEscapes(t *testing.T) {
	root := testRoot(t)
	cfg := "apiVersion: v1\nkind: Config\ncurrent-context: " + `"` + `evil\e]0;pwned\a\e[2J` + `"` + `
clusters:
- name: "cl\e[31m"
  cluster:
    server: "https://x\e[2J"
users:
- name: "us\a"
  user:
    token: t
contexts:
- name: "` + `evil\e]0;pwned\a\e[2J` + `"
  context:
    cluster: "cl\e[31m"
    user: "us\a"
    namespace: "ns\e[H"
`
	writeProject(t, root, "evilproj", cfg, 0o600)
	link(t, root, "evilproj")
	for _, size := range [][2]int{{130, 30}, {90, 30}, {60, 20}} {
		m := open(t, root, size[0], size[1])
		for _, ks := range [][]string{nil, {"tab"}, {"tab"}, {"shift+tab", "shift+tab"}} {
			m = keys(m, ks...)
			raw := m.View().Content
			if strings.ContainsAny(raw, "\a") || strings.Contains(raw, "\x1b]") || strings.Contains(raw, "\x1b[2J") || strings.Contains(raw, "\x1b[H") {
				t.Fatalf("%v: injected sequence reached the view: %q", size, raw)
			}
			// Every ESC left must start an SGR (colour/attribute) sequence.
			for _, part := range strings.Split(raw, "\x1b")[1:] {
				if !sgrRe.MatchString(part) {
					t.Fatalf("%v: non-SGR escape: %q", size, part[:min(len(part), 20)])
				}
			}
		}
	}
	mustContain(t, plain(open(t, root, 130, 30)), "evil")
}

func TestInvalidProjectNeverEchoesSecrets(t *testing.T) {
	root := testRoot(t)
	// Malformed in several ways, each carrying secret values.
	bad := map[string]string{
		"flow":   "apiVersion: v1\nkind: Config\nusers: [{name: u, user: {token: " + secretToken + "\n",
		"tab":    "apiVersion: v1\nkind: Config\nusers:\n- name: u\n\tuser:\n\t  token: " + secretToken + "\n",
		"badkey": "apiVersion: v1\nkind: Config\nclusters: " + secretToken + "\n",
		"binary": "apiVersion: v1\nkind: Config\nusers:\n- name: u\n  user:\n    client-key-data: " + secretKey + "!!!\n    token: [" + secretToken + "\n",
	}
	for name, content := range bad {
		writeProject(t, root, name, content, 0o600)
	}
	for _, size := range [][2]int{{130, 30}, {80, 24}, {50, 12}} {
		m := open(t, root, size[0], size[1])
		for range bad {
			for _, ks := range [][]string{nil, {"tab"}, {"tab"}, {"shift+tab", "shift+tab"}, {"j"}} {
				m = keys(m, ks...)
				m = keys(m, "enter") // tries to use the invalid project: error status
				for _, s := range []string{secretToken, secretKey} {
					if strings.Contains(m.View().Content, s) {
						t.Fatalf("%v: secret %q echoed:\n%s", size, s, plain(m))
					}
				}
			}
		}
	}
}
