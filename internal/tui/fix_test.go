package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

func shellModel(t *testing.T, w, h int) (Model, string) {
	t.Helper()
	root := fixture(t)
	s := store.New(root)
	env := store.Env{InShell: true, Project: "staging", Kubeconfig: s.ShellCopyPath(os.Getpid(), "staging")}
	if err := os.MkdirAll(s.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Kubeconfig, []byte(kubeconfig("stg", stg)), 0o600); err != nil {
		t.Fatal(err)
	}
	return send(New(s, env), tea.WindowSizeMsg{Width: w, Height: h}), root
}

// R2
func TestEditRefusedInsideShell(t *testing.T) {
	m, root := shellModel(t, 120, 30)
	before := snapshot(t, root)
	m = keys(m, "e")
	mustContain(t, plain(m), "⚠ Exit this kx shell to edit projects")
	sameSnapshot(t, before, snapshot(t, root))
}

// R3
func TestImportRefusesNonRegularFiles(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = typeText(keys(m, "i"), "/dev/null")
	m = keys(m, "enter")
	mustContain(t, plain(m), "✗ Not a regular file")
	if len(m.dialogs) != 1 {
		t.Fatal("dialog must stay open")
	}
}

// R4
func TestEditorWithArguments(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := filepath.Join(dir, "ed.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script+" --wait -x")
	file := filepath.Join(dir, "my config")
	cmd := editCommand(file)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(b)); got != "--wait -x "+file {
		t.Errorf("editor got %q", got)
	}
	t.Setenv("EDITOR", "")
	if c := editCommand(file); !slices.Contains(c.Env, "EDITOR=vi") {
		t.Errorf("vi fallback missing: %v", c.Env)
	}
}

// R5 helper: a live process we control.
func liveChild(t *testing.T) int {
	t.Helper()
	c := exec.Command("sleep", "60")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _, _ = c.Process.Wait() })
	return c.Process.Pid
}

// R6
func TestStaleDerivedConflictIsDropped(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)
	m = keys(typeText(keys(m, "i"), file), "enter", "enter")
	m = keys(m, "r", "j", "o", "enter") // rename cluster, overwrite user -> derived context row
	mustContain(t, plain(m), "3 conflicts")
	m = keys(m, "k", "k", "s") // change the cluster to skip: the context is dropped, no longer conflicts
	m = keys(m, "enter")
	// Overwriting the user still re-points the existing prod-eu: confirm it.
	mustContain(t, plain(m), "Overwrite re-points existing contexts: prod-eu")
	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatalf("stale derived row blocked the import:\n%s", plain(m))
	}
	mustContain(t, plain(m), "Imported eu.yaml into prod:")
}

// U1
func TestSelection256Colours(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = send(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	if m.sty.mode == modeTrue {
		t.Error("ANSI256 must not be treated as truecolor")
	}
	raw := m.View().Content
	mustContain(t, raw, "48;5;237", "38;5;255")
	m = send(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	mustContain(t, m.View().Content, "48;5;189", "38;5;17")
}

// U2
func TestSelectedRowHasNoDimForeground(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "tab", "j") // prod-us: namespace "-" is dim
	var row string
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Strip(l), "▸  prod-us") || strings.Contains(ansi.Strip(l), "▸★ prod-us") {
			row = l
		}
	}
	if row == "" {
		t.Fatal("row not found")
	}
	if strings.Contains(row[strings.Index(row, "prod-us"):], "38;2;138;143;152") {
		t.Errorf("dim foreground on the selected row: %q", row)
	}
}

// U3
func TestShellStartsOnShellProject(t *testing.T) {
	m, _ := shellModel(t, 120, 30)
	if m.cur.name != "staging" {
		t.Errorf("selected %q, want staging", m.cur.name)
	}
	v := plain(m)
	mustContain(t, v, "$ staging")
	h := lastLine(v)
	mustNotContain(t, h, "enter use", "s shell")
	mustContain(t, h, "i import", "r rename")
	t.Setenv("KX_ASCII", "1")
	m2, _ := shellModel(t, 120, 30)
	mustContain(t, plain(m2), "$ staging")
}

// U4
func TestHintBarKeepsCommitKey(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 40, 12)
	m = keys(typeText(keys(m, "i"), file), "enter", "enter")
	h := lastLine(plain(m))
	mustContain(t, h, "enter apply", "esc cancel")
	mustNotContain(t, h, "set all")

	m = keys(open(t, root, 40, 12), ":")
	h = lastLine(plain(m))
	mustContain(t, h, "enter run", "esc close")

	m = keys(open(t, root, 40, 12), "D")
	h = lastLine(plain(m))
	mustContain(t, h, "enter delete", "esc cancel")
	if ansi.StringWidth(h) > 40 {
		t.Errorf("hint bar too wide: %q", h)
	}
}

// U5
func TestNoForeignBannerWithoutProjects(t *testing.T) {
	root := testRoot(t)
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(kubeconfig("dev", devCtx)), 0o600); err != nil {
		t.Fatal(err)
	}
	v := plain(open(t, root, 100, 30))
	mustNotContain(t, v, "not managed by kx yet")
	mustContain(t, v, "a   adopt the current")
}

// U6
func TestBannerDropsSecondSentenceWhenItDoesNotFit(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	if err := os.Symlink("config-old", filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	v := plain(open(t, root, 90, 30)) // wider than 80, still too narrow for both sentences
	mustContain(t, v, "points to a missing project (config-old).")
	mustNotContain(t, v, "Press enter", "…")
}

// U7
func TestEmptyStateWrapsAndHeader(t *testing.T) {
	m := open(t, testRoot(t), 50, 20)
	v := plain(m)
	mustNotContain(t, v, "…")
	flat := strings.Join(strings.Fields(v), " ")
	mustContain(t, flat, "kx keeps one kubeconfig per project in ~/.kube/config-<name>.")
	mustContain(t, v, "kx  active: none")
}

// U8
func TestDetailsMiddleTruncate(t *testing.T) {
	m := open(t, fixture(t), 120, 30)
	v := plain(m)
	mustContain(t, v, "amazonaws.com")
	mustContain(t, v, "role/admin")
}

// U9
func TestHelpPolish(t *testing.T) {
	m := keys(open(t, fixture(t), 100, 40), "?")
	var colNav, colProj int
	for _, l := range strings.Split(plain(m), "\n") {
		if i := strings.Index(l, "next / prev pane"); i >= 0 {
			colNav = i
		}
		if i := strings.Index(l, "use (activate globally)"); i >= 0 {
			colProj = i
		}
	}
	if colNav == 0 || colNav != colProj {
		t.Errorf("description columns differ: %d vs %d\n%s", colNav, colProj, plain(m))
	}
	small := keys(open(t, fixture(t), 100, 14), "?")
	mustContain(t, lastLine(plain(small)), "j/k scroll", "esc close")

	t.Setenv("KX_ASCII", "1")
	a := keys(open(t, fixture(t), 100, 40), "?")
	mustContain(t, plain(a), "h l ")
	mustNotContain(t, plain(a), "h l left right")
}

// U10
func TestDoctorHangingIndent(t *testing.T) {
	m := open(t, fixture(t), 60, 24)
	m = keys(typeText(keys(m, ":"), "doctor"), "enter")
	var cont string
	for _, l := range strings.Split(plain(m), "\n") {
		if strings.Contains(l, "0600") && !strings.Contains(l, "⚠") {
			cont = l
		}
	}
	if cont == "" {
		t.Fatalf("message did not wrap:\n%s", plain(m))
	}
	if !regexp.MustCompile(`│ {4}\S`).MatchString(cont) {
		t.Errorf("continuation not indented: %q", cont)
	}
}

// U11
func TestPaletteShowsHiddenCount(t *testing.T) {
	m := keys(open(t, fixture(t), 100, 30), ":")
	mustContain(t, plain(m), "+1 more")
}

// U12
func TestBackupsHaveNoOpColumn(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	b, err := s.Backups.Create("prod", "import", filepath.Join(root, "config-prod"))
	if err != nil {
		t.Fatal(err)
	}
	m := keys(open(t, root, 100, 30), "b")
	v := plain(m)
	mustContain(t, v, b.ID, "WHEN")
	mustNotContain(t, v, " OP")
}

// U13
func TestFilterRowUnderTitle(t *testing.T) {
	m := keys(open(t, fixture(t), 100, 30), "/")
	m = typeText(m, "st")
	lines := strings.Split(plain(m), "\n")
	for i, l := range lines {
		if strings.Contains(l, "PROJECTS") {
			if !strings.Contains(lines[i+1], "/ st") {
				t.Errorf("filter row is not under the title: %q", lines[i+1])
			}
			return
		}
	}
	t.Fatal("no title")
}

// U14
func TestNoColorDimIsFaint(t *testing.T) {
	m := send(open(t, fixture(t), 100, 30), tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	if !strings.Contains(m.View().Content, "\x1b[2m") {
		t.Error("dim text must be Faint without colour")
	}
}

// U15
func TestAdoptCopy(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(kubeconfig("x", devCtx)), 0o600); err != nil {
		t.Fatal(err)
	}
	m := keys(open(t, root, 100, 30), "enter")
	flat := strings.Join(strings.Fields(strings.ReplaceAll(plain(m), "│", " ")), " ")
	mustContain(t, flat, "i Keep it as a new project (you name it), then use prod")
}

// U16
func TestNamespaceLeaveEmptyNote(t *testing.T) {
	m := keys(open(t, fixture(t), 100, 30), "tab", "n")
	mustContain(t, plain(m), "Leave empty to clear")
}

// All-overwrite of conflictingIncoming re-points prod-eu (it stays on the
// overwritten cluster eks-eu and user): the screen must say so and ask twice.
func TestConflictOverwriteShowsAndConfirmsReboundContexts(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 140, 30)
	m = keys(typeText(keys(m, "i"), file), "enter", "enter")
	mustNotContain(t, plain(m), "re-points existing contexts")

	m = keys(m, "o") // cluster row -> overwrite
	mustContain(t, plain(m), "Overwriting cluster eks-eu re-points existing contexts: prod-eu")
	m = keys(m, "r")
	mustNotContain(t, plain(m), "re-points existing contexts")
	m = keys(m, "O")

	before := snapshot(t, root)
	m = keys(m, "enter")
	mustContain(t, plain(m), "Overwrite re-points existing contexts: prod-eu", "enter again")
	if len(m.dialogs) != 1 {
		t.Fatal("the conflict screen must stay open until confirmed")
	}
	sameSnapshot(t, before, snapshot(t, root))

	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatalf("dialog still open after the confirming enter: %d", len(m.dialogs))
	}
	mustContain(t, plain(m), "re-pointed contexts: prod-eu")
	if cfg := loadProject(t, root, "prod"); !strings.Contains(cfg.Clusters["eks-eu"].Server, "NEW") {
		t.Errorf("overwrite not applied: %+v", cfg.Clusters["eks-eu"])
	}
}

func TestConflictChangingChoiceAsksAgain(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 140, 30)
	m = keys(typeText(keys(m, "i"), conflictingIncoming(t)), "enter", "enter", "O", "enter")
	mustContain(t, plain(m), "enter again")
	m = keys(m, "S") // everything skipped: nothing is re-pointed
	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatal("skip-all must apply without a confirmation")
	}
	mustNotContain(t, plain(m), "now use the incoming")
}

func TestDetailsShowRedactedServer(t *testing.T) {
	root := fixture(t)
	writeProject(t, root, "staging", kubeconfig("stg",
		ctxSpec{"stg", "stg-cluster", "https://u:pw@h.example:6443/x?token=t#f", "stg-user", "web", "token"}), 0o600)
	m := open(t, root, 140, 30)
	m = keys(m, "j") // staging
	v := plain(m)
	mustContain(t, v, "https://h.example:6443/x")
	mustNotContain(t, v, "u:pw", "token=t", "#f")
}

func TestBannerWarnsWhenKubeconfigIsSet(t *testing.T) {
	root := fixture(t)
	m := send(New(store.New(root), store.Env{Kubeconfig: "/elsewhere/cfg"}), tea.WindowSizeMsg{Width: 140, Height: 30})
	mustContain(t, plain(m), "KUBECONFIG", "kubectl reads $KUBECONFIG, not the kx project")
	m = send(New(store.New(root), store.Env{Kubeconfig: "/x", InShell: true, Project: "prod"}), tea.WindowSizeMsg{Width: 140, Height: 30})
	mustNotContain(t, plain(m), "kubectl reads $KUBECONFIG")
	m = send(New(store.New(root), store.Env{}), tea.WindowSizeMsg{Width: 140, Height: 30})
	mustNotContain(t, plain(m), "kubectl reads $KUBECONFIG")
}

func TestEditBacksUpFirst(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 140, 30)
	t.Setenv("EDITOR", "true")
	orig, _ := os.ReadFile(filepath.Join(root, "config-prod"))
	_, cmd := sendCmd(m, press("e"))
	if cmd == nil {
		t.Fatal("e must return an exec command")
	}
	bs, err := m.store.Backups.List("prod")
	if err != nil || len(bs) != 1 || bs[0].Op != "edit" {
		t.Fatalf("backups = %+v, %v; want one edit backup", bs, err)
	}
	if got, _ := os.ReadFile(bs[0].Path); string(got) != string(orig) {
		t.Fatal("backup differs from the file before editing")
	}
}

func TestEditDoesNotLaunchWhenBackupFails(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 140, 30)
	t.Setenv("EDITOR", "true")
	// A regular file where the backups dir must go makes every backup fail.
	if err := os.MkdirAll(filepath.Join(root, "kx"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "kx", "backups"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The only command is the status expiry (not running it: it sleeps), so
	// the failure is reported instead of starting the editor.
	m2, _ := sendCmd(m, press("e"))
	mustContain(t, plain(m2), "✗ Could not back up prod")
}
