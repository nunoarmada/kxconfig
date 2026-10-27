package tui

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/internal/store"
	"k8s.io/client-go/tools/clientcmd/api"
)

// snapshot records every file (content hash or link target) under root, so a
// test can prove that nothing on disk changed.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			tg, _ := os.Readlink(p)
			out[rel] = "-> " + tg
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fi, _ := d.Info()
			out[rel] = fmt.Sprintf("%x %v", sha256.Sum256(b), fi.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnapshot(t *testing.T, before, after map[string]string) {
	t.Helper()
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed: %q -> %q", k, v, after[k])
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("%s was created", k)
		}
	}
}

func loadProject(t *testing.T, root, name string) *api.Config {
	t.Helper()
	cfg, err := kubecfg.Load(filepath.Join(root, "config-"+name))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// writeFile writes an incoming kubeconfig outside the kx root.
func writeIncoming(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// eu.yaml conflicts with prod on cluster eks-eu (other server) and user
// arn:...:role/admin (token instead of exec); prod-ap is new.
func conflictingIncoming(t *testing.T) string {
	return writeIncoming(t, "eu.yaml", kubeconfig("prod-eu",
		ctxSpec{"prod-eu", "eks-eu", "https://NEW.eks.eu-west-1.amazonaws.com", "arn:aws:iam::123:role/admin", "api", "token"},
		ctxSpec{"prod-ap", "eks-ap", "https://AP.eks.ap-south-1.amazonaws.com", "arn:aws:iam::123:role/admin", "", "token"}))
}

func TestImportFlowWithConflictsRename(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)

	m = keys(m, "i")
	mustContain(t, plain(m), "Import kubeconfig", "Kubeconfig file to import")
	mustContain(t, lastLine(plain(m)), "tab complete", "enter next", "esc cancel")

	m = typeText(m, file)
	m = keys(m, "enter")
	// The name defaults to the selected project (merge into it).
	v := plain(m)
	mustContain(t, v, "Project name", "> prod", "Merges into existing project prod")

	m = keys(m, "enter")
	v = plain(m)
	mustContain(t, v, "— 2 conflicts", "POLICY", "KIND", "NAME", "EXISTING", "INCOMING",
		"cluster", "eks-eu", "user", "arn:aws:iam::123:role/admin",
		"[ ? ]", "ren keeps both", "ovr incoming wins", "skp keep existing",
		"new entit", "A backup is taken first.")
	mustNotContain(t, v, "[ren]", "[ovr]", "[skp]", secretToken, secretPass, secretKey, secretCert)
	hintBar := lastLine(v)
	mustContain(t, hintBar, "j/k move", "r o s set row", "R O S set all", "enter apply", "esc cancel")

	// Enter is refused while rows are unresolved, and nothing is written.
	before := snapshot(t, root)
	m = keys(m, "enter")
	mustContain(t, plain(m), "⚠ 2 conflicts unresolved")
	sameSnapshot(t, before, snapshot(t, root))
	m = keys(m, "r") // cluster row -> rename
	m = keys(m, "enter")
	mustContain(t, plain(m), "⚠ 1 conflict unresolved", "[ren]", "[ ? ]")
	m = keys(m, "j", "o") // user row -> overwrite
	mustContain(t, plain(m), "[ren]", "[ovr]")
	mustNotContain(t, plain(m), "[ ? ]")

	// Renaming the cluster changes the incoming context that uses it, so
	// that context now differs from the existing one: it becomes a new
	// unresolved row instead of being decided silently. Nothing is written yet.
	before = snapshot(t, root)
	m = keys(m, "enter")
	v = plain(m)
	mustContain(t, v, "3 conflicts", "⚠ 1 more conflict after your choices: resolve them too", "context", "prod-eu", "[ ? ]")
	if len(m.dialogs) != 1 {
		t.Fatal("the conflict screen must stay open")
	}
	sameSnapshot(t, before, snapshot(t, root))
	m = keys(m, "enter")
	mustContain(t, plain(m), "⚠ 1 conflict unresolved")
	m = keys(m, "r")

	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatalf("dialog still open after apply: %d", len(m.dialogs))
	}
	v = plain(m)
	mustContain(t, v, "Imported eu.yaml into prod:", "renamed", "overwritten")

	cfg := loadProject(t, root, "prod")
	if cfg.Clusters["eks-eu"] == nil || !strings.Contains(cfg.Clusters["eks-eu"].Server, "A1B2C3") {
		t.Errorf("existing cluster eks-eu was touched: %+v", cfg.Clusters["eks-eu"])
	}
	if c := cfg.Clusters["eks-eu-2"]; c == nil || !strings.Contains(c.Server, "NEW") {
		t.Errorf("incoming cluster not kept as eks-eu-2: %v", cfg.Clusters)
	}
	if cfg.Contexts["prod-ap"] == nil || cfg.Contexts["prod-eu-2"] == nil || cfg.Contexts["prod-eu-2"].Cluster != "eks-eu-2" {
		t.Errorf("new context prod-ap and renamed prod-eu-2 (on eks-eu-2) expected: %v", cfg.Contexts)
	}
	if cfg.AuthInfos["arn:aws:iam::123:role/admin"].Token == "" {
		t.Errorf("user was not overwritten by the incoming token user")
	}
	bs, err := m.store.Backups.List("prod")
	if err != nil || len(bs) != 1 || bs[0].Op != "import" {
		t.Errorf("backups = %+v, %v; want one import backup", bs, err)
	}
}

func TestImportConflictsAllSkipKeepsExisting(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)
	m = typeText(keys(m, "i"), file)
	m = keys(m, "enter", "enter", "S")
	mustContain(t, plain(m), "[skp]")
	mustNotContain(t, plain(m), "[ ? ]")
	m = keys(m, "enter") // skipping both leaves the incoming context without its cluster: it is dropped
	mustContain(t, plain(m), "Imported eu.yaml into prod:", "skipped")
	cfg := loadProject(t, root, "prod")
	if !strings.Contains(cfg.Clusters["eks-eu"].Server, "A1B2C3") || cfg.Clusters["eks-eu-2"] != nil {
		t.Errorf("skip must keep the existing entities: %v", cfg.Clusters)
	}
}

func TestImportEscFromConflictsWritesNothing(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)
	before := snapshot(t, root)
	m = typeText(keys(m, "i"), file)
	m = keys(m, "enter", "enter", "R", "esc")
	if len(m.dialogs) != 0 {
		t.Fatal("esc must close the conflict screen")
	}
	sameSnapshot(t, before, snapshot(t, root))
}

func TestImportNewProjectNoConflicts(t *testing.T) {
	root := testRoot(t)
	file := writeIncoming(t, "my cluster.yaml", kubeconfig("a", ctxSpec{"a", "ca", "https://a.example.com", "ua", "", "token"}))
	m := open(t, root, 100, 30)
	m = typeText(keys(m, "i"), file)
	m = keys(m, "enter")
	// Nothing selected: the name comes from the file, made safe and free.
	mustContain(t, plain(m), "> my-cluster", "Creates project my-cluster")
	m = keys(m, "enter")
	mustContain(t, plain(m), "Imported my cluster.yaml into my-cluster: 3 added", "my-cluster")
	if _, err := os.Stat(filepath.Join(root, "config-my-cluster")); err != nil {
		t.Fatal(err)
	}
}

func TestImportPathErrors(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	before := snapshot(t, root)

	m = typeText(keys(m, "i"), filepath.Join(t.TempDir(), "nope.yaml"))
	m = keys(m, "enter")
	mustContain(t, plain(m), "✗ No such file")
	m = keys(m, "esc")

	m = typeText(keys(m, "i"), t.TempDir())
	m = keys(m, "enter")
	mustContain(t, plain(m), "✗ That is a folder")
	m = keys(m, "esc")

	bad := writeIncoming(t, "bad.yaml", "apiVersion: v1\nclusters: [oops\n")
	m = typeText(keys(m, "i"), bad)
	m = keys(m, "enter")
	mustContain(t, plain(m), "✗ Not a usable kubeconfig")
	// Editing clears the message.
	m = keys(m, "x")
	mustNotContain(t, plain(m), "Not a usable kubeconfig")
	sameSnapshot(t, before, snapshot(t, root))
}

func TestImportNameValidation(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)
	m = keys(typeText(keys(m, "i"), file), "enter")
	for range 4 {
		m = keys(m, "backspace")
	}
	m = typeText(m, "a b")
	mustContain(t, plain(m), "✗ Use letters, digits, . _ - (max 64)")
	before := snapshot(t, root)
	m = keys(m, "enter") // disabled while invalid
	mustContain(t, plain(m), "Project name")
	sameSnapshot(t, before, snapshot(t, root))
}

func TestPathCompletion(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"alpha.yaml", "alpine.yaml", "beta.yaml", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		in, want string
		n        int
	}{
		{dir + "/be", dir + "/beta.yaml", 0},
		{dir + "/al", dir + "/alp", 2}, // common prefix, two candidates
		{dir + "/alp", dir + "/alp", 2},
		{dir + "/alpi", dir + "/alpine.yaml", 0},
		{dir + "/su", dir + "/sub/", 0},
		{dir + "/", dir + "/", 4}, // hidden file stays out
		{dir + "/zzz", dir + "/zzz", 0},
		{dir + "/.h", dir + "/.hidden", 0},
	} {
		got, cands := completePath(c.in)
		if got != c.want || len(cands) != c.n {
			t.Errorf("completePath(%q) = %q, %v; want %q with %d candidates", c.in, got, cands, c.want, c.n)
		}
	}
	t.Setenv("HOME", dir)
	if got, _ := completePath("~/be"); got != "~/beta.yaml" {
		t.Errorf("~ completion = %q", got)
	}
	if got, _ := completePath("~"); got != "~/" {
		t.Errorf("completePath(~) = %q", got)
	}

	// Through the dialog: tab completes in the input, and lists candidates.
	root := fixture(t)
	m := open(t, root, 100, 30)
	m = typeText(keys(m, "i"), dir+"/al")
	m = keys(m, "tab")
	mustContain(t, plain(m), dir+"/al", "alpha.yaml  alpine.yaml")
}

func TestDeleteContextConfirmShowsOrphans(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	m = keys(m, "tab", "j", "d") // prod-us: its cluster eks-us and user prod-us-admin are orphans
	v := plain(m)
	mustContain(t, v, "Delete context?", "Delete context prod-us from prod?", "Also removes unused:",
		"cluster  eks-us", "user     prod-us-admin", "A backup is taken first.", "[ y Delete ]", "[ n Keep ]")
	mustContain(t, lastLine(v), "y confirm", "n/esc cancel")

	// Enter is cancel, as the default is the safe choice.
	before := snapshot(t, root)
	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatal("enter must cancel the confirm")
	}
	sameSnapshot(t, before, snapshot(t, root))

	m = keys(m, "d", "y")
	if len(m.dialogs) != 0 {
		t.Fatal("dialog must close after y")
	}
	cfg := loadProject(t, root, "prod")
	if cfg.Contexts["prod-us"] != nil || cfg.Clusters["eks-us"] != nil || cfg.AuthInfos["prod-us-admin"] != nil {
		t.Errorf("context and its orphans must be gone: %v %v %v", cfg.Contexts, cfg.Clusters, cfg.AuthInfos)
	}
	if cfg.Contexts["prod-eu"] == nil || cfg.Clusters["eks-eu"] == nil {
		t.Errorf("other entities must stay")
	}
	bs, _ := m.store.Backups.List("prod")
	if len(bs) != 1 || bs[0].Op != "rmctx" {
		t.Errorf("backups = %+v; want one rmctx", bs)
	}
	mustContain(t, plain(m), "✓ Deleted context prod-us from prod", "CONTEXTS prod 1")
}

func TestDeleteContextWithoutOrphansOmitsList(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "p", kubeconfig("a",
		ctxSpec{"a", "shared", "https://s.example.com", "u", "", "token"},
		ctxSpec{"b", "shared", "https://s.example.com", "u", "", "token"}), 0o600)
	link(t, root, "p")
	m := open(t, root, 100, 30)
	m = keys(m, "tab", "d")
	v := plain(m)
	mustContain(t, v, "Delete context a from p?")
	mustNotContain(t, v, "Also removes unused")
}

func TestRenameDialog(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	if _, err := s.Backups.Create("staging", "import", filepath.Join(root, "config-staging")); err != nil {
		t.Fatal(err)
	}
	m := open(t, root, 100, 30)
	m = keys(m, "j", "j", "j") // dev, legacy, prod, staging
	m = keys(m, "r")
	v := plain(m)
	mustContain(t, v, "Rename project", "New name for staging", "> staging")
	mustContain(t, lastLine(v), "enter confirm", "esc cancel")

	// Invalid and taken names show an inline error and disable enter.
	m = typeText(m, " x")
	mustContain(t, plain(m), "✗ Use letters, digits, . _ - (max 64)")
	for range 9 {
		m = keys(m, "backspace")
	}
	m = typeText(m, "dev")
	mustContain(t, plain(m), "✗ Project dev already exists")
	before := snapshot(t, root)
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Fatal("enter must be disabled while the name is invalid")
	}
	sameSnapshot(t, before, snapshot(t, root))

	m = keys(m, "backspace", "backspace", "backspace")
	m = typeText(m, "stage")
	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatal("dialog must close")
	}
	mustContain(t, plain(m), "✓ Renamed staging → stage")
	if _, err := os.Stat(filepath.Join(root, "config-stage")); err != nil {
		t.Errorf("new file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "config-staging")); err == nil {
		t.Errorf("old file still there")
	}
	if bs, _ := s.Backups.List("stage"); len(bs) != 2 || bs[1].Op != "import" {
		t.Errorf("backups did not follow the rename: %+v", bs)
	}
	// The cursor follows the renamed project.
	if m.cur.name != "stage" {
		t.Errorf("selected = %q, want stage", m.cur.name)
	}
}

func TestRenameActiveKeepsLink(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30) // cursor on prod, the active one
	m = keys(m, "r", "backspace", "backspace", "backspace", "backspace")
	m = typeText(m, "live")
	m = keys(m, "enter")
	if name, st, _ := m.store.Active(); name != "live" || st != store.LinkOK {
		t.Errorf("active = %q, %v; want live OK", name, st)
	}
	mustContain(t, plain(m), "active: ● live")
}

func TestDeleteProjectConfirm(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	m = keys(m, "k", "k") // dev (0644)
	if p, _ := m.selectedProject(); p.Name != "dev" {
		t.Fatalf("selected %q", p.Name)
	}
	m = keys(m, "D")
	v := plain(m)
	mustContain(t, v, "Delete project?", "Delete project dev?", "A backup is kept in ~/.kube/kx/backups/dev.", "[ y Delete ]", "[ n Keep ]")
	m = keys(m, "n")
	if _, err := os.Stat(filepath.Join(root, "config-dev")); err != nil {
		t.Fatal("n must keep the project")
	}
	m = keys(m, "D", "y")
	if _, err := os.Stat(filepath.Join(root, "config-dev")); err == nil {
		t.Error("project file must be gone")
	}
	if bs, _ := m.store.Backups.List("dev"); len(bs) != 1 || bs[0].Op != "delete" {
		t.Errorf("backups = %+v", bs)
	}
	mustContain(t, plain(m), "✓ Deleted dev")
	if name, _, _ := m.store.Active(); name != "prod" {
		t.Errorf("active changed to %q", name)
	}
}

func TestDeleteWarnsAboutLiveShells(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	if err := os.MkdirAll(s.ShellsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	// This process counts as a live shell; a second copy of the same pid adds a project twice.
	for _, pid := range []int{os.Getpid(), os.Getppid()} {
		if err := os.WriteFile(s.ShellCopyPath(pid, "staging"), []byte(kubeconfig("stg", stg)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := open(t, root, 100, 30)
	m = keys(m, "j", "j", "j", "D")
	n := 0
	if pids, _ := s.LiveShells("staging"); len(pids) > 0 {
		n = len(pids)
	}
	if n == 0 {
		t.Skip("no live pid available")
	}
	want := "⚠ 1 kx shell is using staging; it keeps its copy."
	if n > 1 {
		want = fmt.Sprintf("⚠ %d kx shells are using staging; they keep their copy.", n)
	}
	mustContain(t, plain(m), "Delete project staging?", want)
}

func TestDeleteActiveRequiresName(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30) // prod is active and selected
	m = keys(m, "D")
	v := plain(m)
	mustContain(t, v, "Delete active project", "prod is active. Deleting it unlinks ~/.kube/config;",
		"nothing will be active until you use another project.", "A backup is kept.", "Type prod to confirm")
	mustContain(t, lastLine(v), "type the name", "enter delete", "esc cancel")

	before := snapshot(t, root)
	m = keys(m, "enter") // empty
	m = typeText(m, "pro")
	m = keys(m, "enter") // partial
	m = typeText(m, "d")
	m = keys(m, "backspace")
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Fatal("enter must do nothing until the name matches")
	}
	sameSnapshot(t, before, snapshot(t, root))
	// A wrong-case name does not count.
	m = typeText(m, "D")
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Fatal("wrong name accepted")
	}
	m = keys(m, "backspace")
	m = typeText(m, "d")
	m = keys(m, "enter")
	if len(m.dialogs) != 0 {
		t.Fatal("dialog must close once the name matches")
	}
	if _, err := os.Lstat(filepath.Join(root, "config")); err == nil {
		t.Error("the config link must be removed")
	}
	if _, err := os.Stat(filepath.Join(root, "config-prod")); err == nil {
		t.Error("project file must be removed")
	}
	if bs, _ := m.store.Backups.List("prod"); len(bs) != 1 || bs[0].Op != "delete" {
		t.Errorf("backups = %+v", bs)
	}
	v = plain(m)
	mustContain(t, v, "✓ Deleted prod; no project is active", "active: none")
}

func TestRestoreFromBackups(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	master := filepath.Join(root, "config-prod")
	orig, err := os.ReadFile(master)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-3 * time.Hour)
	s.Backups.Now = func() time.Time { return t0 }
	old, err := s.Backups.Create("prod", "import", master)
	if err != nil {
		t.Fatal(err)
	}
	s.Backups.Now = nil
	// Change the live file so a restore is visible.
	if err := os.WriteFile(master, []byte(kubeconfig("prod-us", prodUS)), 0o600); err != nil {
		t.Fatal(err)
	}

	m := open(t, root, 100, 30)
	m = keys(m, "b")
	v := plain(m)
	mustContain(t, v, "Backups — prod (1)", "ID", "WHEN", " OP", old.ID, "3h ago", t0.Local().Format("2006-01-02 15:04"), "import")
	mustContain(t, lastLine(v), "enter restore", "esc close")

	m = keys(m, "enter")
	v = plain(m)
	flat := strings.Join(strings.Fields(strings.ReplaceAll(v, "│", " ")), " ")
	mustContain(t, v, "Restore backup?", "[ y Restore ]")
	mustContain(t, flat, "Restore prod to backup "+old.ID+" (3h ago)? The current file is backed up first.")
	// Cancelling goes back to the list; nothing changed.
	m = keys(m, "n")
	mustContain(t, plain(m), "Backups — prod")
	if b, _ := os.ReadFile(master); string(b) == string(orig) {
		t.Fatal("restore ran without confirmation")
	}

	m = keys(m, "enter", "y")
	if len(m.dialogs) != 0 {
		t.Fatalf("dialogs left: %d", len(m.dialogs))
	}
	mustContain(t, plain(m), "✓ Restored prod from backup "+old.ID)
	cfg := loadProject(t, root, "prod")
	if cfg.CurrentContext != "prod-eu" || cfg.Contexts["prod-eu"] == nil {
		t.Errorf("file not restored: current=%q", cfg.CurrentContext)
	}
	bs, _ := s.Backups.List("prod")
	if len(bs) != 2 || bs[0].Op != "prerestore" {
		t.Errorf("backups = %+v; want a prerestore backup on top", bs)
	}
	// The link still points at the project.
	if name, st, _ := s.Active(); name != "prod" || st != store.LinkOK {
		t.Errorf("active = %q %v", name, st)
	}
}

func TestBackupsEmpty(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "b")
	mustContain(t, plain(m), "Backups — prod (0)", "No backups for prod yet.")
	mustContain(t, lastLine(plain(m)), "esc close")
	m = keys(m, "enter") // nothing to restore
	if len(m.dialogs) != 1 {
		t.Error("enter on an empty list must do nothing")
	}
	m = keys(m, "esc")
	if len(m.dialogs) != 0 {
		t.Error("esc must close")
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"}, {5 * time.Minute, "5m ago"}, {3 * time.Hour, "3h ago"},
		{25 * time.Hour, "1d ago"}, {10 * 24 * time.Hour, "10d ago"}, {70 * 24 * time.Hour, "2mo ago"},
	} {
		if got := relTime(now, now.Add(-c.ago)); got != c.want {
			t.Errorf("relTime(%v) = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestNamespaceInput(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	m = keys(m, "tab", "n")
	v := plain(m)
	mustContain(t, v, "Set namespace", "Namespace for current context prod-eu", "> api")
	mustContain(t, lastLine(v), "enter confirm", "esc cancel")

	m = keys(m, "backspace", "backspace", "backspace")
	m = typeText(m, "Bad_NS")
	mustContain(t, plain(m), "✗ Use lowercase letters, digits and - (max 63)")
	before := snapshot(t, root)
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Fatal("enter must be disabled while invalid")
	}
	sameSnapshot(t, before, snapshot(t, root))

	for range 6 {
		m = keys(m, "backspace")
	}
	m = typeText(m, "payments")
	m = keys(m, "enter")
	mustContain(t, plain(m), "✓ Set namespace of prod-eu to payments")
	cfg := loadProject(t, root, "prod")
	if got := cfg.Contexts["prod-eu"].Namespace; got != "payments" {
		t.Errorf("namespace = %q, want payments", got)
	}
	if got := cfg.Contexts["prod-us"].Namespace; got != "" {
		t.Errorf("another context was changed: %q", got)
	}
	mustContain(t, lineWith(plain(m), "prod-eu"), "payments")

	// Empty clears it.
	m = keys(m, "n")
	for range 8 {
		m = keys(m, "backspace")
	}
	m = keys(m, "enter")
	mustContain(t, plain(m), "✓ Cleared namespace of prod-eu")
	if got := loadProject(t, root, "prod").Contexts["prod-eu"].Namespace; got != "" {
		t.Errorf("namespace = %q, want empty", got)
	}
}

func TestNamespaceNoteWhenCursorIsNotCurrent(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, "tab", "j", "n")
	mustContain(t, plain(m), "Namespace for current context prod-eu", "Applies to the current context, not prod-us.")
}

func TestPaletteRunsAction(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	m = keys(m, ":")
	v := plain(m)
	mustContain(t, v, "Commands", ": ", "import", "Import a kubeconfig file", "rename", "Rename selected project",
		"delete", "backups", "shell", "edit", "doctor", "reload")
	mustContain(t, lastLine(v), "type to search", "↑↓ move", "enter run", "esc close")

	// Fuzzy: "bkp" matches backups only.
	m = typeText(m, "bkp")
	v = plain(m)
	mustContain(t, v, "backups")
	mustNotContain(t, v, "Import a kubeconfig file", "Quit kx")
	m = keys(m, "enter")
	mustContain(t, plain(m), "Backups — prod")
	m = keys(m, "esc")

	// A palette command opens the same dialog as its key.
	m = keys(m, ":")
	m = typeText(m, "rena")
	m = keys(m, "enter")
	mustContain(t, plain(m), "Rename project", "New name for prod")
	m = keys(m, "esc")

	// Moving with down picks another entry.
	m = keys(m, ":", "down", "enter") // second command: rename
	mustContain(t, plain(m), "Rename project")
	m = keys(m, "esc")

	// No match.
	m = keys(m, ":")
	m = typeText(m, "xyz")
	mustContain(t, plain(m), `No command matches "xyz"`)
	m = keys(m, "enter")
	if len(m.dialogs) != 1 {
		t.Error("enter with no match must keep the palette open")
	}
	m = keys(m, "esc")

	// reload and quit.
	if err := os.WriteFile(filepath.Join(root, "config-new"), []byte(kubeconfig("n", ctxSpec{"n", "c", "https://n", "u", "", "token"})), 0o600); err != nil {
		t.Fatal(err)
	}
	m = keys(m, ":")
	m = typeText(m, "reload")
	m = keys(m, "enter")
	mustContain(t, plain(m), "PROJECTS 5", "Reloaded from disk")

	m = keys(m, ":")
	m = typeText(m, "quit")
	_, cmd := sendCmd(m, press("enter"))
	if !isQuit(cmd) {
		t.Error("the quit command must quit")
	}
}

func TestPaletteMatchHighlight(t *testing.T) {
	m := open(t, fixture(t), 100, 30)
	m = keys(m, ":")
	m = typeText(m, "e")
	// The matched letter of the rows without the cursor is accent bold.
	if !strings.Contains(m.View().Content, "1me\x1b[m") { // accent + bold
		t.Errorf("matched chars are not highlighted")
	}
}

func TestPaletteDoctorFixes(t *testing.T) {
	root := fixture(t) // dev is 0644
	m := open(t, root, 100, 30)
	m = keys(m, ":")
	m = typeText(m, "doctor")
	m = keys(m, "enter")
	v := plain(m)
	mustContain(t, v, "Doctor", "⚠ perms", "project dev has mode 0644", "fixable", "~/.kube/config-dev", "invalid-project")
	mustContain(t, lastLine(v), "f fix fixable", "esc close")

	m = keys(m, "f")
	v = plain(m)
	mustContain(t, v, "✓ fixed")
	mustNotContain(t, lineWith(v, "project dev has mode"), "fixable")
	if fi, err := os.Stat(filepath.Join(root, "config-dev")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("dev mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	// Nothing fixable is left: the hint goes away.
	mustNotContain(t, lastLine(v), "fix fixable")
	m = keys(m, "esc")
	mustNotContain(t, plain(m), "⚠ 0644")
}

func TestDoctorNoProblems(t *testing.T) {
	root := testRoot(t)
	writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
	link(t, root, "prod")
	m := open(t, root, 100, 30)
	m = keys(m, ":")
	m = typeText(m, "doctor")
	m = keys(m, "enter")
	mustContain(t, plain(m), "✓ No problems found.")
}

func TestShellRefusedInsideShell(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	env := store.Env{InShell: true, Project: "staging", Kubeconfig: s.ShellCopyPath(os.Getpid(), "staging")}
	m := send(New(s, env), tea.WindowSizeMsg{Width: 120, Height: 30})
	before := snapshot(t, root)

	m, cmd := sendCmd(m, press("s"))
	mustContain(t, plain(m), "⚠ Already inside a kx shell")
	if isQuit(cmd) || len(m.dialogs) != 0 {
		t.Error("s must only warn")
	}
	// Through the palette as well.
	m = keys(m, ":")
	m = typeText(m, "shell")
	m = keys(m, "enter")
	mustContain(t, plain(m), "⚠ Already inside a kx shell")
	sameSnapshot(t, before, snapshot(t, root)) // no shell copy was made
}

func TestShellStartsForSelectedProject(t *testing.T) {
	root := fixture(t)
	t.Setenv("SHELL", "/bin/sh")
	m := open(t, root, 100, 30)
	m = keys(m, "j") // staging? order: dev legacy prod staging; cursor on prod -> staging
	m, cmd := sendCmd(m, press("s"))
	if cmd == nil {
		t.Fatal("s must return an exec command")
	}
	s := store.New(root)
	copyPath := s.ShellCopyPath(os.Getpid(), "staging")
	if _, err := os.Stat(copyPath); err != nil {
		t.Errorf("shell copy for staging was not made: %v", err)
	}
	// An invalid project is reported, not started.
	m = keys(m, "k", "k") // legacy
	m = keys(m, "s")
	mustContain(t, plain(m), "✗ Could not open a shell for legacy:")

	// When the shell exits the screen reloads and reports it.
	m = send(m, shellDoneMsg{project: "staging"})
	mustContain(t, plain(m), "· Left kx shell for staging")
	m = send(m, shellDoneMsg{project: "staging", err: fmt.Errorf("boom")})
	mustContain(t, plain(m), "✗ Could not close the shell for staging: boom")
}

func TestEditUsesEditorEnv(t *testing.T) {
	root := fixture(t)
	m := open(t, root, 100, 30)
	t.Setenv("EDITOR", "")
	_, cmd := sendCmd(m, press("e"))
	if cmd == nil {
		t.Fatal("e must return an exec command (vi fallback)")
	}
	t.Setenv("EDITOR", "code -w")
	_, cmd = sendCmd(m, press("e"))
	if cmd == nil {
		t.Fatal("e must return an exec command")
	}
	// After the editor exits, the file is read again.
	if err := os.WriteFile(filepath.Join(root, "config-prod"), []byte(kubeconfig("prod-us", prodUS)), 0o600); err != nil {
		t.Fatal(err)
	}
	m = send(m, editDoneMsg{project: "prod"})
	mustContain(t, plain(m), "✓ Edited prod", "CONTEXTS prod 1", "prod-us")
	m = send(m, editDoneMsg{project: "prod", err: fmt.Errorf("no such editor")})
	mustContain(t, plain(m), "✗ Could not run the editor: no such editor")
}

// openers lists key sequences that open each dialog from the fixture.
var openers = map[string][]string{
	"import path":     {"i"},
	"rename":          {"r"},
	"delete active":   {"D"},
	"delete":          {"k", "D"},
	"backups":         {"b"},
	"restore confirm": {"b", "enter"},
	"namespace":       {"tab", "n"},
	"delete context":  {"tab", "d"},
	"palette":         {":"},
	"help":            {"?"},
	"adopt":           {}, // set up separately
}

func TestEscClosesDialogs(t *testing.T) {
	for name, ks := range openers {
		if name == "adopt" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			s := store.New(root)
			if _, err := s.Backups.Create("prod", "import", filepath.Join(root, "config-prod")); err != nil {
				t.Fatal(err)
			}
			m := open(t, root, 100, 30)
			before := snapshot(t, root)
			m = keys(m, ks...)
			if len(m.dialogs) == 0 {
				t.Fatal("dialog did not open")
			}
			for i := 0; i < 4 && len(m.dialogs) > 0; i++ {
				m = keys(m, "esc")
			}
			if len(m.dialogs) != 0 {
				t.Fatalf("esc left %d dialogs open", len(m.dialogs))
			}
			sameSnapshot(t, before, snapshot(t, root))
			mustNotContain(t, plain(m), "✓ ")
		})
	}
	t.Run("import name", func(t *testing.T) {
		root := fixture(t)
		file := conflictingIncoming(t)
		m := open(t, root, 100, 30)
		before := snapshot(t, root)
		m = keys(typeText(keys(m, "i"), file), "enter")
		mustContain(t, plain(m), "Project name")
		m = keys(m, "esc")
		if len(m.dialogs) != 0 {
			t.Fatal("esc must close")
		}
		sameSnapshot(t, before, snapshot(t, root))
	})
	t.Run("adopt", func(t *testing.T) {
		root := testRoot(t)
		writeProject(t, root, "prod", kubeconfig("prod-eu", prodEU), 0o600)
		if err := os.WriteFile(filepath.Join(root, "config"), []byte(kubeconfig("x", ctxSpec{"x", "c", "https://x", "u", "", "token"})), 0o600); err != nil {
			t.Fatal(err)
		}
		m := open(t, root, 100, 30)
		before := snapshot(t, root)
		m = keys(m, "enter")
		mustContain(t, plain(m), "not managed by kx")
		m = keys(m, "esc")
		if len(m.dialogs) != 0 {
			t.Fatal("esc must close")
		}
		sameSnapshot(t, before, snapshot(t, root))
	})
}

func TestDialogsFitEverySize(t *testing.T) {
	root := fixture(t)
	s := store.New(root)
	for range 3 {
		if _, err := s.Backups.Create("prod", "import", filepath.Join(root, "config-prod")); err != nil {
			t.Fatal(err)
		}
	}
	file := conflictingIncoming(t)
	setups := map[string]func(Model) Model{
		"import path": func(m Model) Model { return keys(m, "i") },
		"import conflicts": func(m Model) Model {
			return keys(typeText(keys(m, "i"), file), "enter", "enter", "j")
		},
		"rename":         func(m Model) Model { return keys(m, "r") },
		"delete active":  func(m Model) Model { return keys(m, "D") },
		"delete":         func(m Model) Model { return keys(m, "k", "D") },
		"backups":        func(m Model) Model { return keys(m, "b") },
		"restore":        func(m Model) Model { return keys(m, "b", "enter") },
		"namespace":      func(m Model) Model { return keys(m, "tab", "n") },
		"delete context": func(m Model) Model { return keys(m, "tab", "j", "d") },
		"palette":        func(m Model) Model { return keys(m, ":") },
		"doctor":         func(m Model) Model { return keys(typeText(keys(m, ":"), "doctor"), "enter") },
	}
	for name, setup := range setups {
		for _, sz := range [][2]int{{40, 10}, {49, 12}, {60, 20}, {79, 23}, {80, 24}, {100, 30}, {120, 40}, {200, 60}} {
			t.Run(fmt.Sprintf("%s/%dx%d", name, sz[0], sz[1]), func(t *testing.T) {
				m := setup(open(t, root, sz[0], sz[1]))
				if len(m.dialogs) == 0 {
					t.Fatal("dialog did not open")
				}
				v := plain(m)
				lines := strings.Split(v, "\n")
				if len(lines) != sz[1] {
					t.Errorf("%d lines, want %d", len(lines), sz[1])
				}
				for i, l := range lines {
					if w := ansi.StringWidth(l); w > sz[0] {
						t.Errorf("line %d is %d wide, max %d: %q", i, w, sz[0], l)
					}
				}
				// The hint bar stays on the last line.
				if strings.TrimSpace(lines[len(lines)-1]) == "" {
					t.Error("hint bar is empty")
				}
			})
		}
	}
}

func TestConflictScreenNarrowShowsCursorRowDetail(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 70, 24)
	m = keys(typeText(keys(m, "i"), file), "enter", "enter")
	v := plain(m)
	mustContain(t, v, "cluster/eks-eu", "user/arn:aws", "existing  ", "incoming  ", "[ ? ]")
	mustNotContain(t, v, "KIND")
}

func TestNoColorKeepsConflictGlyphs(t *testing.T) {
	root := fixture(t)
	file := conflictingIncoming(t)
	m := open(t, root, 100, 30)
	m = send(m, tea.ColorProfileMsg{Profile: colorprofile.NoTTY})
	m = keys(typeText(keys(m, "i"), file), "enter", "enter", "r")
	v := m.View().Content
	if strings.Contains(v, "\x1b[3") || strings.Contains(v, "\x1b[9") {
		t.Errorf("colour sequences in NO_COLOR mode: %q", v)
	}
	mustContain(t, ansi.Strip(v), "▸[ren]", "[ ? ]")
}

func TestASCIIModeDialogs(t *testing.T) {
	root := fixture(t)
	t.Setenv("KX_ASCII", "1")
	s := store.New(root)
	if _, err := s.Backups.Create("prod", "import", filepath.Join(root, "config-prod")); err != nil {
		t.Fatal(err)
	}
	file := conflictingIncoming(t)
	for name, ks := range map[string][]string{
		"delete context": {"tab", "j", "d"},
		"delete active":  {"D"},
		"delete":         {"k", "D"},
		"rename":         {"r"},
		"namespace":      {"tab", "n"},
		"backups":        {"b"},
		"restore":        {"b", "enter"},
		"palette":        {":"},
		"doctor":         {":", "d", "o", "c", "enter"},
		"import path":    {"i"},
	} {
		m := keys(open(t, root, 100, 30), ks...)
		if len(m.dialogs) == 0 {
			t.Fatalf("%s: no dialog", name)
		}
		for _, r := range m.View().Content {
			if r > 127 {
				t.Fatalf("%s: non-ASCII %q in ASCII mode:\n%s", name, r, plain(m))
			}
		}
	}
	m := open(t, root, 100, 30)
	m = keys(typeText(keys(m, "i"), file), "enter", "enter", "r")
	for _, r := range m.View().Content {
		if r > 127 {
			t.Fatalf("conflicts: non-ASCII %q:\n%s", r, plain(m))
		}
	}
	mustContain(t, plain(m), ">[ren]", "- 2 conflicts")
}

func TestKubeconfigTextInDialogsIsClean(t *testing.T) {
	root := testRoot(t)
	evil := "ev\u202e\u200eil" // valid YAML, hostile on a terminal
	writeProject(t, root, "p", kubeconfig("c", ctxSpec{"c", "cl", "https://s", evil, "", "token"},
		ctxSpec{"d", "cl2", "https://s2", "u2", "", "token"}), 0o600)
	link(t, root, "p")
	m := open(t, root, 100, 30)
	m = keys(m, "tab", "d")
	v := m.View().Content
	if strings.Contains(v, "\u202e") || strings.Contains(v, "\u200e") {
		t.Errorf("bidi control leaked into the confirm: %q", v)
	}
	mustContain(t, ansi.Strip(v), "user     evil")
}
