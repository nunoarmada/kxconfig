package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

// kcYAML renders a kubeconfig with one cluster, one user and one context.
func kcYAML(cluster, server, user, ctx string) string {
	return `apiVersion: v1
kind: Config
current-context: ` + ctx + `
clusters:
- name: ` + cluster + `
  cluster:
    server: ` + server + `
users:
- name: ` + user + `
  user:
    token: secret-` + user + `
contexts:
- name: ` + ctx + `
  context:
    cluster: ` + cluster + `
    user: ` + user + `
`
}

// kcTwoYAML has two contexts (ctx1, ctx2) sharing nothing, so removing one
// orphans its cluster and user.
func kcTwoYAML() string {
	return `apiVersion: v1
kind: Config
current-context: ctx1
clusters:
- name: c1
  cluster:
    server: https://c1.example
- name: c2
  cluster:
    server: https://c2.example
users:
- name: u1
  user:
    token: t1
- name: u2
  user:
    token: t2
contexts:
- name: ctx1
  context:
    cluster: c1
    user: u1
- name: ctx2
  context:
    cluster: c2
    user: u2
`
}

type harness struct {
	t        *testing.T
	d        Deps
	out, err *bytes.Buffer
	files    string
	tuiCalls int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, out: &bytes.Buffer{}, err: &bytes.Buffer{}, files: t.TempDir()}
	h.d = Deps{
		Store:   store.New(t.TempDir()),
		In:      strings.NewReader(""),
		Out:     h.out,
		Err:     h.err,
		Version: "test",
		RunTUI:  func() error { h.tuiCalls++; return nil },
	}
	return h
}

func (h *harness) write(name, content string) string {
	h.t.Helper()
	p := filepath.Join(h.files, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
	return p
}

// run executes kx with args and returns the exit code and captured output.
func (h *harness) run(args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	h.out.Reset()
	h.err.Reset()
	code = Run(h.d, args)
	return code, h.out.String(), h.err.String()
}

func (h *harness) mustRun(args ...string) string {
	h.t.Helper()
	code, out, errOut := h.run(args...)
	if code != 0 {
		h.t.Fatalf("kx %v: exit %d, stderr: %s", args, code, errOut)
	}
	return out
}

func (h *harness) active() string {
	h.t.Helper()
	name, _, err := h.d.Store.Active()
	if err != nil {
		h.t.Fatal(err)
	}
	return name
}

func TestRootNoTTYErrors(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run()
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	want := "kx: the interactive UI needs a terminal; see 'kx --help'\n"
	if errOut != want {
		t.Fatalf("stderr = %q, want %q", errOut, want)
	}
	if h.tuiCalls != 0 {
		t.Fatal("RunTUI must not run without a TTY")
	}
}

func TestRootTTYRunsTUI(t *testing.T) {
	h := newHarness(t)
	h.d.IsTTY = true
	h.mustRun()
	if h.tuiCalls != 1 {
		t.Fatalf("RunTUI calls = %d, want 1", h.tuiCalls)
	}
}

func TestUseAndDash(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("ca", "https://a", "ua", "xa")), "-p", "a")
	h.mustRun("import", h.write("b.yaml", kcYAML("cb", "https://b", "ub", "xb")), "-p", "b")

	h.mustRun("use", "a")
	h.mustRun("use", "b")
	if got := h.active(); got != "b" {
		t.Fatalf("active = %q, want b", got)
	}
	h.mustRun("use", "-")
	if got := h.active(); got != "a" {
		t.Fatalf("after use -, active = %q, want a", got)
	}
	h.mustRun("-")
	if got := h.active(); got != "b" {
		t.Fatalf("after bare -, active = %q, want b", got)
	}
}

func TestUseDashWithoutPrevious(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run("use", "-")
	if code != 1 || !strings.Contains(errOut, "no previous project") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestUseRefusedInShell(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("ca", "https://a", "ua", "xa")), "-p", "a")
	h.d.Env = store.Env{InShell: true, Project: "a"}
	code, _, errOut := h.run("use", "a")
	if code != 1 || !strings.Contains(errOut, "kx shell") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if h.active() != "" {
		t.Fatal("use must not change the link inside a shell")
	}
}

func foreignSetup(t *testing.T) *harness {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("ca", "https://a", "ua", "xa")), "-p", "a")
	if err := os.WriteFile(h.d.Store.ConfigPath(), []byte(kcYAML("cf", "https://f", "uf", "xf")), 0o600); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestUseForeignNonTTYHint(t *testing.T) {
	h := foreignSetup(t)
	code, _, errOut := h.run("use", "a")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	for _, want := range []string{"--adopt=<name>", "--adopt-replace"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stderr %q lacks %q", errOut, want)
		}
	}
}

func TestUseAdoptImports(t *testing.T) {
	h := foreignSetup(t)
	h.mustRun("use", "a", "--adopt", "old")
	if h.active() != "a" {
		t.Fatal("a should be active")
	}
	if _, err := os.Stat(h.d.Store.ProjectPath("old")); err != nil {
		t.Fatalf("adopted project missing: %v", err)
	}
}

func TestUseAdoptReplaceBacksUp(t *testing.T) {
	h := foreignSetup(t)
	h.mustRun("use", "a", "--adopt-replace")
	if h.active() != "a" {
		t.Fatal("a should be active")
	}
	bs, err := h.d.Store.Backups.List("_initial")
	if err != nil || len(bs) != 1 {
		t.Fatalf("want one _initial backup, got %v, %v", bs, err)
	}
}

func TestUseForeignTTYPromptReplace(t *testing.T) {
	h := foreignSetup(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("r\n")
	h.mustRun("use", "a")
	if h.active() != "a" {
		t.Fatal("a should be active")
	}
}

func TestUseForeignTTYPromptAbort(t *testing.T) {
	h := foreignSetup(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("a\n")
	code, _, _ := h.run("use", "a")
	if code != 1 || h.active() != "" {
		t.Fatalf("abort must fail and change nothing (code=%d)", code)
	}
}

func TestImportConflictTableAndExit(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("prod", "https://one", "u1", "x1")), "-p", "p")
	before, _ := os.ReadFile(h.d.Store.ProjectPath("p"))

	code, out, errOut := h.run("import", h.write("b.yaml", kcYAML("prod", "https://two", "u2", "x2")), "-p", "p")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	all := out + errOut
	for _, want := range []string{"KIND", "NAME", "EXISTING", "INCOMING", "cluster", "prod", "--on-conflict"} {
		if !strings.Contains(all, want) {
			t.Fatalf("output lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "secret") {
		t.Fatal("output leaks a secret")
	}
	after, _ := os.ReadFile(h.d.Store.ProjectPath("p"))
	if !bytes.Equal(before, after) {
		t.Fatal("conflicting import changed the project")
	}
}

func TestImportOnConflictRename(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("prod", "https://one", "u1", "x1")), "-p", "p")
	out := h.mustRun("import", h.write("b.yaml", kcYAML("prod", "https://two", "u2", "x2")), "-p", "p", "--on-conflict", "rename")
	if !strings.Contains(out, "renamed 1") {
		t.Fatalf("summary lacks renamed count:\n%s", out)
	}
	cfg, err := kubecfg.Load(h.d.Store.ProjectPath("p"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Clusters["prod-2"] == nil || cfg.Contexts["x2"].Cluster != "prod-2" {
		t.Fatalf("rename not applied: %+v", cfg.Clusters)
	}
}

func TestImportNeedsNameWithoutTTY(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")))
	if code != 1 || !strings.Contains(errOut, "-p") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestImportTTYPromptsWithDefaultName(t *testing.T) {
	h := newHarness(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("\n")
	h.mustRun("import", h.write("staging.yaml", kcYAML("c", "https://a", "u", "x")))
	if _, err := os.Stat(h.d.Store.ProjectPath("staging")); err != nil {
		t.Fatalf("default name not used: %v", err)
	}
}

func TestImportDroppedListed(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("prod", "https://one", "u1", "x1")), "-p", "p")
	// Incoming context x2 references cluster prod, which is skipped.
	h.mustRun("import", h.write("b.yaml", kcYAML("prod", "https://two", "u2", "x2")), "-p", "p", "--on-conflict", "skip")
	// x2 references the skipped cluster prod, so it is dropped.
	out := h.out.String()
	if !strings.Contains(out, "dropped 1") || !strings.Contains(out, "x2") ||
		!strings.Contains(out, "references a skipped or missing cluster/user") {
		t.Fatalf("dropped not reported:\n%s", out)
	}
}

func TestImportUseActivates(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a", "--use")
	if h.active() != "a" {
		t.Fatal("--use should activate")
	}
}

func TestLsJSON(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("import", h.write("b.yaml", kcYAML("c", "https://b", "u", "x")), "-p", "b")
	h.mustRun("use", "a")

	out := h.mustRun("ls", "-o", "json")
	var got []struct {
		Name     string `json:"name"`
		Active   bool   `json:"active"`
		Contexts int    `json:"contexts"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(got) != 2 || got[0].Name != "a" || !got[0].Active || got[0].Contexts != 2 || got[0].Status != "ok" ||
		got[1].Name != "b" || got[1].Active || got[1].Contexts != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestLsTableStatus(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("import", h.write("b.yaml", kcYAML("c", "https://b", "u", "x")), "-p", "b")
	if err := os.Chmod(h.d.Store.ProjectPath("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.d.Store.ProjectPath("bad"), []byte("::not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustRun("use", "a")
	out := h.mustRun("ls")
	for _, want := range []string{"ACTIVE", "NAME", "CONTEXTS", "STATUS", "perms 0644", "invalid"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls output lacks %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && f[1] == "a" && f[0] != "*" {
			t.Fatalf("active project not marked: %q", line)
		}
	}
}

func TestCurrent(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("use", "a")
	if out := h.mustRun("current"); out != "a\n" {
		t.Fatalf("current = %q", out)
	}
	h.d.Env = store.Env{InShell: true, Project: "b"}
	if out := h.mustRun("current"); out != "a\nshell: b\n" {
		t.Fatalf("current in shell = %q", out)
	}
}

func TestCtxNsInShellTargetsCopy(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("use", "a")
	master, _ := os.ReadFile(h.d.Store.ProjectPath("a"))

	if err := os.MkdirAll(h.d.Store.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	cp := h.d.Store.ShellCopyPath(os.Getpid(), "a")
	if err := os.WriteFile(cp, master, 0o600); err != nil {
		t.Fatal(err)
	}
	h.d.Env = store.Env{InShell: true, Project: "a", Kubeconfig: cp}

	h.mustRun("ctx", "ctx2")
	h.mustRun("ns", "kube-system")

	cfg, err := kubecfg.Load(cp)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "ctx2" || cfg.Contexts["ctx2"].Namespace != "kube-system" {
		t.Fatalf("copy not updated: %q %+v", cfg.CurrentContext, cfg.Contexts["ctx2"])
	}
	after, _ := os.ReadFile(h.d.Store.ProjectPath("a"))
	if !bytes.Equal(master, after) {
		t.Fatal("master changed by ctx/ns inside a shell")
	}
}

func TestCtxListAndProjectFlag(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	out := h.mustRun("ctx", "-p", "a")
	if !strings.Contains(out, "ctx1") || !strings.Contains(out, "ctx2") {
		t.Fatalf("ctx list:\n%s", out)
	}
	h.mustRun("ctx", "ctx2", "-p", "a")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.CurrentContext != "ctx2" {
		t.Fatalf("current = %q", cfg.CurrentContext)
	}
}

func TestRmCtxNeedsYesWithoutTTY(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	code, out, errOut := h.run("rm-ctx", "ctx2", "-p", "a")
	if code != 1 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(out+errOut, "c2") || !strings.Contains(out+errOut, "u2") {
		t.Fatalf("orphans not listed:\n%s%s", out, errOut)
	}
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["ctx2"] == nil {
		t.Fatal("context removed without confirmation")
	}

	h.mustRun("rm-ctx", "ctx2", "-p", "a", "--yes")
	cfg, _ = kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["ctx2"] != nil || cfg.Clusters["c2"] != nil || cfg.AuthInfos["u2"] != nil {
		t.Fatal("context and orphans should be gone")
	}
}

func TestRmCtxKeepOrphans(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("rm-ctx", "ctx2", "-p", "a", "--yes", "--keep-orphans")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["ctx2"] != nil || cfg.Clusters["c2"] == nil {
		t.Fatal("orphans should be kept")
	}
}

func TestRmCtxInShellTargetsMasterWithNote(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.d.Env = store.Env{InShell: true, Project: "a", Kubeconfig: "/ignored"}
	_, _, errOut := h.run("rm-ctx", "ctx2", "--yes")
	if !strings.Contains(errOut, "note: this kx shell uses a snapshot; it was not updated") {
		t.Fatalf("note missing: %q", errOut)
	}
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["ctx2"] != nil {
		t.Fatal("master should be updated")
	}
}

func TestDeleteActive(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("use", "a")

	code, _, errOut := h.run("delete", "a", "--yes")
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	h.mustRun("delete", "a", "--yes", "--force")
	if _, err := os.Stat(h.d.Store.ProjectPath("a")); !os.IsNotExist(err) {
		t.Fatal("project file should be gone")
	}
	if h.active() != "" {
		t.Fatal("link should be removed")
	}
}

func TestDeleteNeedsYesWithoutTTY(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	code, _, errOut := h.run("delete", "a")
	if code != 1 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestRenameUpdatesActive(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("use", "a")
	h.mustRun("rename", "a", "z")
	if h.active() != "z" {
		t.Fatalf("active = %q, want z", h.active())
	}
}

func TestRestoreLatest(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c1", "https://a", "u1", "x1")), "-p", "a")
	h.mustRun("import", h.write("b.yaml", kcYAML("c2", "https://b", "u2", "x2")), "-p", "a")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if len(cfg.Contexts) != 2 {
		t.Fatalf("merge failed: %d contexts", len(cfg.Contexts))
	}

	out := h.mustRun("backups", "a")
	if !strings.Contains(out, "ID") || !strings.Contains(out, "import") {
		t.Fatalf("backups output:\n%s", out)
	}
	h.mustRun("restore", "a", "--yes")
	cfg, _ = kubecfg.Load(h.d.Store.ProjectPath("a"))
	if len(cfg.Contexts) != 1 || cfg.Contexts["x1"] == nil {
		t.Fatalf("restore did not bring back the original: %v", cfg.Contexts)
	}
}

func TestRestoreWithoutBackups(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	code, _, errOut := h.run("restore", "a", "--yes")
	if code != 1 || !strings.Contains(errOut, "no backups") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestLiveShellWarning(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	if err := os.MkdirAll(h.d.Store.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	// This test process is alive, so its pid counts as a live shell.
	if err := os.WriteFile(h.d.Store.ShellCopyPath(os.Getpid(), "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, errOut := h.run("rename", "a", "b")
	if !strings.Contains(errOut, "warning") || !strings.Contains(errOut, "shell") {
		t.Fatalf("no live shell warning: %q", errOut)
	}
}

func TestDoctor(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	if out := h.mustRun("doctor"); !strings.Contains(out, "No problems") {
		t.Fatalf("clean doctor:\n%s", out)
	}
	if err := os.Chmod(h.d.Store.ProjectPath("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := h.run("doctor")
	if code != 1 || !strings.Contains(out, "perms") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	h.mustRun("doctor", "--fix")
	fi, _ := os.Stat(h.d.Store.ProjectPath("a"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", fi.Mode().Perm())
	}
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	if out := h.mustRun("version"); out != "test\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestUnknownFlagKeepsUsage(t *testing.T) {
	h := newHarness(t)
	code, out, errOut := h.run("ls", "--bogus")
	if code != 1 || !strings.Contains(errOut, "kx: unknown flag") || !strings.Contains(out, "Usage:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestRuntimeErrorPrintedOnceWithoutUsage(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run("use", "nope")
	if code != 1 || strings.Count(errOut, "kx: ") != 1 || strings.Contains(errOut, "Usage:") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestCompletionProjects(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "alpha")
	h.mustRun("import", h.write("b.yaml", kcYAML("c", "https://b", "u", "x")), "-p", "beta")
	out := h.mustRun("__complete", "use", "")
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("completion:\n%s", out)
	}
}

func TestCompletionContexts(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("use", "a")
	out := h.mustRun("__complete", "ctx", "")
	if !strings.Contains(out, "ctx1") || !strings.Contains(out, "ctx2") {
		t.Fatalf("completion:\n%s", out)
	}
}

func TestCompletionScript(t *testing.T) {
	h := newHarness(t)
	for _, sh := range []string{"bash", "zsh", "fish"} {
		if out := h.mustRun("completion", sh); !strings.Contains(out, "kx") {
			t.Fatalf("%s completion empty", sh)
		}
	}
}

func twoVersions(t *testing.T) (*harness, []byte) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c1", "https://a", "u1", "x1")), "-p", "a")
	h.mustRun("import", h.write("b.yaml", kcYAML("c2", "https://b", "u2", "x2")), "-p", "a")
	cur, _ := os.ReadFile(h.d.Store.ProjectPath("a"))
	return h, cur
}

func TestRestoreNeedsYesWithoutTTY(t *testing.T) {
	h, cur := twoVersions(t)
	code, _, errOut := h.run("restore", "a")
	if code != 1 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	after, _ := os.ReadFile(h.d.Store.ProjectPath("a"))
	if !bytes.Equal(cur, after) {
		t.Fatal("restore changed the file without confirmation")
	}
}

func TestRestoreExplicitID(t *testing.T) {
	h, _ := twoVersions(t)
	bs, _ := h.d.Store.Backups.List("a")
	h.mustRun("restore", "a", bs[0].ID, "--yes")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if len(cfg.Contexts) != 1 {
		t.Fatalf("contexts = %d", len(cfg.Contexts))
	}
}

func TestRestoreTTYAbort(t *testing.T) {
	h, cur := twoVersions(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("n\n")
	code, _, _ := h.run("restore", "a")
	after, _ := os.ReadFile(h.d.Store.ProjectPath("a"))
	if code != 1 || !bytes.Equal(cur, after) {
		t.Fatal("n must abort and change nothing")
	}
}

func TestAdoptReplaceSaysWhereBackupWent(t *testing.T) {
	h := foreignSetup(t)
	out := h.mustRun("use", "a", "--adopt-replace")
	if !strings.Contains(out, "_initial") {
		t.Fatalf("backup location not shown:\n%s", out)
	}
}

func TestAdoptInteractiveEmptyNameUsesDefault(t *testing.T) {
	h := foreignSetup(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("i\n\n")
	h.mustRun("use", "a")
	if _, err := os.Stat(h.d.Store.ProjectPath("old")); err != nil {
		t.Fatalf("default name not used: %v", err)
	}
	if !strings.Contains(h.err.String(), "Project name [old]:") {
		t.Fatalf("prompt: %q", h.err.String())
	}
}

func TestAdoptInteractiveDefaultSkipsTaken(t *testing.T) {
	h := foreignSetup(t)
	h.mustRun("import", h.write("o.yaml", kcYAML("c", "https://o", "u", "x")), "-p", "old")
	h.d.IsTTY = true
	h.d.In = strings.NewReader("i\n\n")
	h.mustRun("use", "a")
	if _, err := os.Stat(h.d.Store.ProjectPath("old-2")); err != nil {
		t.Fatalf("old-2 not used: %v", err)
	}
}

func TestImportReplaceNeedsYesWithoutTTY(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c1", "https://a", "u1", "x1")), "-p", "a")
	f := h.write("b.yaml", kcYAML("c2", "https://b", "u2", "x2"))
	code, _, errOut := h.run("import", f, "-p", "a", "--replace")
	if code != 1 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["x1"] == nil {
		t.Fatal("replace ran without confirmation")
	}
	h.mustRun("import", f, "-p", "a", "--replace", "--yes")
	cfg, _ = kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["x1"] != nil || cfg.Contexts["x2"] == nil {
		t.Fatal("replace not applied")
	}
}

func TestImportReplaceNewProjectNoConfirm(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "n", "--replace")
}

func TestDeleteTTYAbort(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.d.IsTTY = true
	h.d.In = strings.NewReader("n\n")
	code, _, _ := h.run("delete", "a")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	if _, err := os.Stat(h.d.Store.ProjectPath("a")); err != nil {
		t.Fatal("project must remain")
	}
}

func TestDeleteMissingDoesNotPrompt(t *testing.T) {
	h := newHarness(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("")
	code, _, errOut := h.run("delete", "ghost")
	if code != 1 || !strings.Contains(errOut, "not found") || strings.Contains(errOut, "[y/N]") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestDeleteForceActiveSaysLinkRemoved(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("use", "a")
	out := h.mustRun("delete", "a", "--force", "--yes")
	if !strings.Contains(out, "config link was removed") || !strings.Contains(out, "kx use") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestOutputStripsTerminalEscapes(t *testing.T) {
	h := newHarness(t)
	evil := `apiVersion: v1
kind: Config
current-context: "evil\e]0;pwned\a\e[2J"
clusters:
- name: "cl\e[31m"
  cluster:
    server: "https://x\e[2J"
users:
- name: "us\a"
  user:
    token: t
contexts:
- name: "evil\e]0;pwned\a\e[2J"
  context:
    cluster: "cl\e[31m"
    user: "us\a"
    namespace: "ns\e[H"
`
	// YAML double-quoted strings understand \e and \a.
	file := h.write("evil.yaml", evil)
	h.mustRun("import", file, "-p", "a")
	bad := func(what, s string) {
		t.Helper()
		if strings.ContainsAny(s, "\x1b\x07") {
			t.Errorf("%s output contains terminal control bytes: %q", what, s)
		}
	}
	bad("ctx", h.mustRun("ctx", "-p", "a"))
	bad("ls", h.mustRun("ls"))
	_, out, errOut := h.run("import", file, "-p", "a") // conflicts table + error
	bad("import conflicts", out+errOut)
	_, out, errOut = h.run("rm-ctx", "evil\x1b]0;pwned\x07\x1b[2J", "-p", "a")
	bad("rm-ctx", out+errOut)
	_, out, errOut = h.run("ctx", "nope\x1b[2J", "-p", "a")
	bad("ctx error", out+errOut)
	_, out, errOut = h.run("rm-ctx", "evil\x1b]0;pwned\x07\x1b[2J", "-p", "a", "--yes")
	bad("rm-ctx --yes", out+errOut)
}

func TestConfirmPromptStripsTerminalEscapes(t *testing.T) {
	h, _ := twoVersions(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("n\n")
	_, _, errOut := h.run("restore", "a", "evil\x1b]0;pwned\x07\x1b[2J")
	if strings.ContainsAny(errOut, "\x1b\x07") {
		t.Fatalf("confirm prompt leaked control characters: %q", errOut)
	}
}

// reboundSetup has project p with context x1 on cluster c, and returns the
// path of a file whose cluster c points elsewhere (a different context x2).
func reboundSetup(t *testing.T) (*harness, string, []byte) {
	t.Helper()
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://one", "u1", "x1")), "-p", "p")
	before, err := os.ReadFile(h.d.Store.ProjectPath("p"))
	if err != nil {
		t.Fatal(err)
	}
	return h, h.write("b.yaml", kcYAML("c", "https://evil", "u2", "x2")), before
}

func TestImportOverwriteListsReboundAndNeedsYes(t *testing.T) {
	h, in, before := reboundSetup(t)
	code, out, errOut := h.run("import", in, "-p", "p", "--on-conflict", "overwrite")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stdout %q stderr %q)", code, out, errOut)
	}
	all := out + errOut
	for _, want := range []string{"existing contexts now use the incoming cluster/user", "x1", "--yes"} {
		if !strings.Contains(all, want) {
			t.Fatalf("output lacks %q:\n%s", want, all)
		}
	}
	if after, _ := os.ReadFile(h.d.Store.ProjectPath("p")); !bytes.Equal(before, after) {
		t.Fatal("nothing may be written before confirmation")
	}
	if n, _ := h.d.Store.Backups.List("p"); len(n) != 0 {
		t.Fatalf("backup taken before confirmation: %v", n)
	}

	h.mustRun("import", in, "-p", "p", "--on-conflict", "overwrite", "--yes")
	if !strings.Contains(h.out.String()+h.err.String(), "x1") {
		t.Fatal("rebound contexts must be listed even with --yes")
	}
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("p"))
	if cfg.Clusters["c"].Server != "https://evil" {
		t.Fatalf("overwrite not applied: %+v", cfg.Clusters["c"])
	}
}

func TestImportOverwriteReboundTTYConfirm(t *testing.T) {
	h, in, before := reboundSetup(t)
	h.d.IsTTY = true
	h.d.In = strings.NewReader("n\n")
	if code, _, _ := h.run("import", in, "-p", "p", "--on-conflict", "overwrite"); code != 1 {
		t.Fatalf("declined import must fail, exit = %d", code)
	}
	if after, _ := os.ReadFile(h.d.Store.ProjectPath("p")); !bytes.Equal(before, after) {
		t.Fatal("declined import changed the project")
	}

	h.d.In = strings.NewReader("y\n")
	h.mustRun("import", in, "-p", "p", "--on-conflict", "overwrite")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("p"))
	if cfg.Clusters["c"].Server != "https://evil" {
		t.Fatal("confirmed overwrite not applied")
	}
}

func TestImportOverwriteWithoutReboundDoesNotAsk(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://one", "u1", "x1")), "-p", "p")
	// The incoming x1 differs too (namespace), so it is replaced rather than
	// silently re-pointed.
	in := h.write("b.yaml", strings.Replace(kcYAML("c", "https://two", "u1", "x1"), "user: u1\n", "user: u1\n    namespace: other\n", 1))
	code, _, errOut := h.run("import", in, "-p", "p", "--on-conflict", "overwrite")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut)
	}
}

func TestNsValidatesNamespace(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a")
	h.mustRun("use", "a")
	for _, bad := range []string{"Bad_NS", "x-", "a b", strings.Repeat("a", 64)} {
		if code, _, errOut := h.run("ns", bad); code != 1 || !strings.Contains(errOut, "namespace") {
			t.Errorf("ns %q: code=%d stderr=%q", bad, code, errOut)
		}
	}
	h.mustRun("ns", "kube-system")
	cfg, _ := kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["x"].Namespace != "kube-system" {
		t.Fatalf("namespace = %q", cfg.Contexts["x"].Namespace)
	}
	h.mustRun("ns", "")
	cfg, _ = kubecfg.Load(h.d.Store.ProjectPath("a"))
	if cfg.Contexts["x"].Namespace != "" {
		t.Fatalf("namespace not cleared: %q", cfg.Contexts["x"].Namespace)
	}
}

func TestRestoreInitialAsNewProject(t *testing.T) {
	h := foreignSetup(t)
	foreign, _ := os.ReadFile(h.d.Store.ConfigPath())
	h.mustRun("use", "a", "--adopt-replace")

	code, _, errOut := h.run("restore", "--initial")
	if code != 1 || !strings.Contains(errOut, "--as") {
		t.Fatalf("missing --as: code=%d stderr=%q", code, errOut)
	}
	if code, _, _ := h.run("restore", "--initial", "--as", "a"); code != 1 {
		t.Fatal("restoring over an existing project must fail")
	}
	out := h.mustRun("backups", "--initial")
	if !strings.Contains(out, "adopt") {
		t.Fatalf("backups --initial:\n%s", out)
	}
	h.mustRun("restore", "--initial", "--as", "original")
	got, err := os.ReadFile(h.d.Store.ProjectPath("original"))
	if err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("restored project = %q, %v", got, err)
	}
	if h.active() != "a" {
		t.Fatal("restore --initial must not change the active project")
	}
}

func TestCtxNsWarnOnForeignKubeconfig(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("use", "a")
	for _, args := range [][]string{{"ctx", "ctx2"}, {"ns", "kube-system"}, {"ctx"}} {
		if _, _, errOut := h.run(args...); strings.Contains(errOut, "KUBECONFIG") {
			t.Errorf("%v warned without KUBECONFIG: %q", args, errOut)
		}
	}
	h.d.Env = store.Env{Kubeconfig: "/somewhere/else"}
	for _, args := range [][]string{{"ctx", "ctx2"}, {"ns", "kube-system"}, {"ctx"}} {
		_, _, errOut := h.run(args...)
		if !strings.Contains(errOut, "kubectl reads $KUBECONFIG") || !strings.Contains(errOut, "/somewhere/else") {
			t.Errorf("%v: no KUBECONFIG warning: %q", args, errOut)
		}
	}
}

func TestImportUseRefusedInShellBeforeImporting(t *testing.T) {
	h := newHarness(t)
	h.d.Env = store.Env{InShell: true, Project: "x"}
	code, _, errOut := h.run("import", h.write("a.yaml", kcYAML("c", "https://a", "u", "x")), "-p", "a", "--use")
	if code != 1 || !strings.Contains(errOut, "kx shell") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if _, err := os.Stat(h.d.Store.ProjectPath("a")); err == nil {
		t.Fatal("project was imported although --use was refused")
	}
}

func TestImportThenUseAdoptsForeignConfigSilently(t *testing.T) {
	h := newHarness(t)
	foreign := "# hand written\n" + kcYAML("c", "https://a", "u", "x")
	if err := os.WriteFile(h.d.Store.ConfigPath(), []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustRun("import", h.d.Store.ConfigPath(), "-p", "a")
	h.mustRun("use", "a") // no --adopt needed: same kubeconfig
	if h.active() != "a" {
		t.Fatal("a should be active")
	}
	bs, err := h.d.Store.Backups.List("_initial")
	if err != nil || len(bs) != 1 {
		t.Fatalf("want one _initial backup, got %v, %v", bs, err)
	}
	if got, _ := os.ReadFile(bs[0].Path); string(got) != foreign {
		t.Fatalf("backup = %q", got)
	}
}

func TestKubeconfigWarningSkippedWhenItIsTheKxFile(t *testing.T) {
	h := newHarness(t)
	h.mustRun("import", h.write("a.yaml", kcTwoYAML()), "-p", "a")
	h.mustRun("use", "a")
	for name, kc := range map[string]string{
		"config link":  h.d.Store.ConfigPath(),
		"project file": h.d.Store.ProjectPath("a"),
		"dotted path":  filepath.Join(h.d.Store.Root, "kx", "..", "config"),
	} {
		h.d.Env = store.Env{Kubeconfig: kc}
		if _, _, errOut := h.run("ns", "kube-system"); strings.Contains(errOut, "KUBECONFIG") {
			t.Errorf("%s: warned although kubectl reads the kx file: %q", name, errOut)
		}
	}
	h.d.Env = store.Env{Kubeconfig: h.d.Store.ConfigPath() + string(filepath.ListSeparator) + "/other"}
	if _, _, errOut := h.run("ns", "kube-system"); !strings.Contains(errOut, "KUBECONFIG") {
		t.Error("a multi-entry KUBECONFIG must still warn")
	}
}
