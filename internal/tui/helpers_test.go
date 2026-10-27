package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/internal/store"
)

// Secret values that must never reach a view.
const (
	secretToken = "tok-SUPERSECRET-123"
	secretKey   = "S0VZLURBVEEtU0VDUkVU" // base64 of KEY-DATA-SECRET
	secretCert  = "Q0VSVC1EQVRBLVNFQ1JFVA=="
	secretPass  = "hunter2-PASSWORD"
)

// ctxSpec describes one context of a generated kubeconfig.
type ctxSpec struct {
	name, cluster, server, user, ns string
	auth                            string // token | exec | cert | basic
}

func kubeconfig(current string, ctxs ...ctxSpec) string {
	var cl, us, cx strings.Builder
	seenC, seenU := map[string]bool{}, map[string]bool{}
	for _, c := range ctxs {
		if !seenC[c.cluster] {
			seenC[c.cluster] = true
			fmt.Fprintf(&cl, "- name: %s\n  cluster:\n    server: %s\n", c.cluster, c.server)
		}
		if !seenU[c.user] {
			seenU[c.user] = true
			fmt.Fprintf(&us, "- name: %s\n  user:\n", c.user)
			switch c.auth {
			case "exec":
				us.WriteString("    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      command: aws\n      args: [eks, get-token]\n")
			case "cert":
				fmt.Fprintf(&us, "    client-certificate-data: %s\n    client-key-data: %s\n", secretCert, secretKey)
			case "basic":
				fmt.Fprintf(&us, "    username: admin\n    password: %s\n", secretPass)
			default:
				fmt.Fprintf(&us, "    token: %s\n", secretToken)
			}
		}
		fmt.Fprintf(&cx, "- name: %s\n  context:\n    cluster: %s\n    user: %s\n", c.name, c.cluster, c.user)
		if c.ns != "" {
			fmt.Fprintf(&cx, "    namespace: %s\n", c.ns)
		}
	}
	section := func(name string, b *strings.Builder) string {
		if b.Len() == 0 {
			return name + ": []\n"
		}
		return name + ":\n" + b.String()
	}
	return "apiVersion: v1\nkind: Config\ncurrent-context: " + current + "\n" +
		section("clusters", &cl) + section("users", &us) + section("contexts", &cx)
}

// env sets up a temp HOME with root <home>/.kube so paths print as ~/.kube,
// and pins the locale to UTF-8 so tests see the default glyph set.
func testRoot(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("KX_ASCII", "")
	root := filepath.Join(home, ".kube")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeProject(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, "config-"+name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, root, name string) {
	t.Helper()
	if err := os.Symlink("config-"+name, filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
}

var (
	prodEU = ctxSpec{"prod-eu", "eks-eu", "https://A1B2C3.eks.eu-west-1.amazonaws.com", "arn:aws:iam::123:role/admin", "api", "exec"}
	prodUS = ctxSpec{"prod-us", "eks-us", "https://D4E5F6.eks.us-east-1.amazonaws.com", "prod-us-admin", "", "token"}
	stg    = ctxSpec{"stg", "stg-cluster", "https://stg.example.com:6443", "stg-user", "web", "cert"}
	devCtx = ctxSpec{"dev", "dev-cluster", "https://dev.example.com:6443", "dev-user", "", "basic"}
)

// fixture creates prod (2 contexts, active), staging, dev (mode 0644) and
// legacy (does not parse).
func fixture(t *testing.T) string {
	t.Helper()
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU, prodUS), 0o600)
	writeProject(t, root, "staging", kubeconfig("stg", stg), 0o600)
	writeProject(t, root, "dev", kubeconfig("dev", devCtx), 0o644)
	writeProject(t, root, "legacy", "apiVersion: v1\nkind: Config\nclusters: [oops\n", 0o600)
	link(t, root, "prod")
	return root
}

func open(t *testing.T, root string, w, h int) Model {
	t.Helper()
	return send(New(store.New(root), store.Env{}), tea.WindowSizeMsg{Width: w, Height: h})
}

// send runs msgs through Update, discarding commands.
func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// sendCmd is send, but also returns the last command.
func sendCmd(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func press(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	if len(r) != 1 {
		panic("press: unknown key " + s)
	}
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

// keys presses each key in order.
func keys(m Model, ks ...string) Model {
	for _, k := range ks {
		m = send(m, press(k))
	}
	return m
}

// typeText presses each rune of s.
func typeText(m Model, s string) Model {
	for _, r := range s {
		m = send(m, press(string(r)))
	}
	return m
}

// plain is the view with ANSI sequences removed.
func plain(m Model) string { return ansi.Strip(m.View().Content) }

func mustContain(t *testing.T, view string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(view, s) {
			t.Errorf("view does not contain %q:\n%s", s, view)
		}
	}
}

func mustNotContain(t *testing.T, view string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(view, s) {
			t.Errorf("view must not contain %q:\n%s", s, view)
		}
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
