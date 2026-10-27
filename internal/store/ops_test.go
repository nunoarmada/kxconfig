package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

// mkCfg returns a kubeconfig with one context ctx -> cluster cl (server) and
// user us (token).
func mkCfg(ctx, cl, server, us, token string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: %[1]s
clusters:
- name: %[2]s
  cluster:
    server: %[3]s
contexts:
- name: %[1]s
  context:
    cluster: %[2]s
    user: %[4]s
users:
- name: %[4]s
  user:
    token: %[5]s
`, ctx, cl, server, us, token)
}

func writeIncoming(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "incoming.yaml")
	writeFile(t, p, content, 0o644)
	return p
}

func backupCount(t *testing.T, s *Store, project string) int {
	t.Helper()
	bs, err := s.Backups.List(project)
	if err != nil {
		t.Fatal(err)
	}
	return len(bs)
}

// blockBackups makes the backups dir unusable (a regular file in its place),
// which fails even when tests run as root.
func blockBackups(t *testing.T, s *Store) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.Backups.Dir), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.Backups.Dir, "not a dir", 0o600)
}

func TestImportCreatesProject(t *testing.T) {
	s := newStore(t)
	in := writeIncoming(t, mkCfg("a", "ca", "https://a", "ua", "t1"))
	res, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || !res.Changed || res.Project != "dev" || res.Backup != nil {
		t.Fatalf("result = %+v", res)
	}
	fi, err := os.Stat(s.ProjectPath("dev"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("project file: %v, %v", fi, err)
	}
	cfg, err := kubecfg.Load(s.ProjectPath("dev"))
	if err != nil || cfg.Contexts["a"] == nil || cfg.CurrentContext != "a" {
		t.Fatalf("loaded = %+v, %v", cfg, err)
	}
	if backupCount(t, s, "dev") != 0 {
		t.Fatal("creating a project must not take a backup")
	}
}

func TestImportConflictFailWritesNothing(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	before := readFile(t, s.ProjectPath("dev"))
	in := writeIncoming(t, mkCfg("a", "ca", "https://other", "ua", "t1"))
	res, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	if !errors.Is(err, kubecfg.ErrConflicts) {
		t.Fatalf("err = %v, want ErrConflicts", err)
	}
	if len(res.Report.Conflicts) == 0 || res.Changed {
		t.Fatalf("result = %+v", res)
	}
	if readFile(t, s.ProjectPath("dev")) != before {
		t.Fatal("master changed on conflict")
	}
	if backupCount(t, s, "dev") != 0 {
		t.Fatal("backup taken on conflict")
	}
}

func TestImportRenameBacksUp(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	before := readFile(t, s.ProjectPath("dev"))
	in := writeIncoming(t, mkCfg("a", "ca", "https://other", "ua", "t1"))
	res, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{Default: kubecfg.PolicyRename})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Created || res.Backup == nil || res.Backup.Op != "import" {
		t.Fatalf("result = %+v", res)
	}
	if got := readFile(t, res.Backup.Path); got != before {
		t.Fatalf("backup content differs from previous master")
	}
	cfg, _ := kubecfg.Load(s.ProjectPath("dev"))
	if cfg.Contexts["a"] == nil || cfg.Contexts["a-2"] == nil || cfg.Clusters["ca-2"] == nil {
		t.Fatalf("merged config = %+v", cfg)
	}
}

func TestImportNoChangeNoBackup(t *testing.T) {
	s := newStore(t)
	content := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", content)
	before := readFile(t, s.ProjectPath("dev"))
	res, err := s.Import(writeIncoming(t, content), "dev", ImportMerge, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Backup != nil || backupCount(t, s, "dev") != 0 {
		t.Fatalf("result = %+v", res)
	}
	if readFile(t, s.ProjectPath("dev")) != before {
		t.Fatal("file rewritten although nothing changed")
	}
}

func TestImportReplace(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	res, err := s.Import(writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2")), "dev", ImportReplace, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Backup == nil || res.Backup.Op != "replace" {
		t.Fatalf("result = %+v", res)
	}
	cfg, _ := kubecfg.Load(s.ProjectPath("dev"))
	if len(cfg.Contexts) != 1 || cfg.Contexts["b"] == nil || cfg.CurrentContext != "b" {
		t.Fatalf("replaced config = %+v", cfg)
	}
}

func TestImportInvalidFile(t *testing.T) {
	s := newStore(t)
	bad := writeIncoming(t, "clusters: [unterminated\n  :::")
	if _, err := s.Import(bad, "dev", ImportMerge, kubecfg.MergeOptions{}); err == nil {
		t.Fatal("expected error for unparsable file")
	}
	if _, err := s.Import(filepath.Join(t.TempDir(), "nope.yaml"), "dev", ImportMerge, kubecfg.MergeOptions{}); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := os.Lstat(s.ProjectPath("dev")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project must not be created: %v", err)
	}
	if _, err := s.Import(bad, "../x", ImportMerge, kubecfg.MergeOptions{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestImportIntoActiveKeepsSymlink(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	if _, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkOK)
	if cfg, _ := kubecfg.Load(s.ProjectPath("dev")); cfg.Contexts["b"] == nil {
		t.Fatal("master missing imported context")
	}
}

func TestPreviewImport(t *testing.T) {
	s := newStore(t)
	in := writeIncoming(t, mkCfg("a", "ca", "https://other", "ua", "t1"))
	if cs, err := s.PreviewImport(in, "dev"); err != nil || len(cs) != 0 {
		t.Fatalf("missing project: %v, %v", cs, err)
	}
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	cs, err := s.PreviewImport(in, "dev")
	if err != nil || len(cs) != 1 || cs[0].Ref.Name != "ca" {
		t.Fatalf("conflicts = %+v, %v", cs, err)
	}
}

func TestBackupFailureAbortsOperations(t *testing.T) {
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", orig)
	// Take a real backup first so Restore has something to restore.
	bk, err := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(s.Backups.Dir); err != nil {
		t.Fatal(err)
	}
	blockBackups(t, s)

	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	if _, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{}); err == nil {
		t.Error("Import must abort when no backup can be written")
	}
	if _, err := s.RemoveContext("dev", "a", false); err == nil {
		t.Error("RemoveContext must abort when no backup can be written")
	}
	if err := s.Restore("dev", bk.ID); err == nil {
		t.Error("Restore must abort when no backup can be written")
	}
	if err := s.Rename("dev", "dev2"); err == nil {
		t.Error("Rename must abort when no backup can be written")
	}
	if err := s.Delete("dev", false); err == nil {
		t.Error("Delete must abort when no backup can be written")
	}
	if got := readFile(t, s.ProjectPath("dev")); got != orig {
		t.Fatalf("project file changed:\n%s", got)
	}
}

func TestClassifyBackup(t *testing.T) {
	pruneErr := errors.New("prune boom")
	b, warn, err := classifyBackup(backup.Backup{ID: "x"}, pruneErr)
	if err != nil || warn == "" || b.ID != "x" {
		t.Fatalf("created+prune error: %+v, %q, %v", b, warn, err)
	}
	if _, warn, err := classifyBackup(backup.Backup{}, pruneErr); err == nil || warn != "" {
		t.Fatalf("no backup must be fatal: %q, %v", warn, err)
	}
	if b, warn, err := classifyBackup(backup.Backup{ID: "x"}, nil); err != nil || warn != "" || b.ID != "x" {
		t.Fatalf("clean: %+v, %q, %v", b, warn, err)
	}
}

func TestRemoveContextOnlyTouchesProject(t *testing.T) {
	s := newStore(t)
	two := mkCfg("shared", "c1", "https://one", "u1", "t1")
	addProject(t, s, "one", two)
	addProject(t, s, "other", mkCfg("shared", "c2", "https://two", "u2", "t2"))
	if err := s.Use("other"); err != nil {
		t.Fatal(err)
	}
	otherBefore := readFile(t, s.ProjectPath("other"))
	rep, err := s.RemoveContext("one", "shared", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RemovedClusters) != 1 || len(rep.RemovedUsers) != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if got := readFile(t, s.ProjectPath("other")); got != otherBefore {
		t.Fatal("other project modified")
	}
	cfg, _ := kubecfg.Load(s.ProjectPath("one"))
	if len(cfg.Contexts) != 0 || len(cfg.Clusters) != 0 {
		t.Fatalf("one = %+v", cfg)
	}
	if backupCount(t, s, "one") != 1 {
		t.Fatal("expected one rmctx backup")
	}
	if _, err := s.RemoveContext("one", "missing", false); err == nil {
		t.Fatal("expected error for unknown context")
	}
	if backupCount(t, s, "one") != 1 {
		t.Fatal("failed removal must not back up")
	}
	if _, err := s.RemoveContext("nope", "x", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRenameActiveUpdatesLinkAndPrevious(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "old", validCfg)
	addProject(t, s, "cur", validCfg)
	if err := s.Use("old"); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("cur"); err != nil { // previous = old
		t.Fatal(err)
	}
	if err := s.Use("old"); err != nil { // active = old, previous = cur
		t.Fatal(err)
	}
	if err := s.setPrevious("old"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "new", LinkOK)
	if p, _ := s.Previous(); p != "new" {
		t.Fatalf("Previous() = %q, want new", p)
	}
	if _, err := os.Lstat(s.ProjectPath("old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old file still present")
	}
}

func TestRenameMovesBackups(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "old", validCfg)
	if _, err := s.Backups.Create("old", "import", s.ProjectPath("old")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	if n := backupCount(t, s, "new"); n != 2 { // the earlier one + the rename backup
		t.Fatalf("backups under new = %d, want 2", n)
	}
	if n := backupCount(t, s, "old"); n != 0 {
		t.Fatalf("backups under old = %d, want 0", n)
	}
}

func TestRenameErrors(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "a", validCfg)
	addProject(t, s, "b", validCfg)
	if err := s.Rename("a", "b"); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if err := s.Rename("zzz", "c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := s.Rename("a", "../c"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestDeleteActiveNeedsForce(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("dev", false); !errors.Is(err, ErrActive) {
		t.Fatalf("err = %v, want ErrActive", err)
	}
	if _, err := os.Stat(s.ProjectPath("dev")); err != nil {
		t.Fatal("project removed without force")
	}
}

func TestDeleteActiveForceRemovesLink(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("dev", true); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "", LinkNone)
	if _, err := os.Lstat(s.ProjectPath("dev")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("project file still present")
	}
}

func TestDeleteKeepsFinalBackup(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	want := readFile(t, s.ProjectPath("dev"))
	if err := s.Delete("dev", false); err != nil {
		t.Fatal(err)
	}
	bs, err := s.Backups.List("dev")
	if err != nil || len(bs) != 1 || bs[0].Op != "delete" {
		t.Fatalf("backups = %+v, %v", bs, err)
	}
	if readFile(t, bs[0].Path) != want {
		t.Fatal("final backup content differs")
	}
	if err := s.Delete("dev", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRestoreIsUndoable(t *testing.T) {
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", orig)
	first, err := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	if err != nil {
		t.Fatal(err)
	}
	newer := mkCfg("b", "cb", "https://b", "ub", "t2")
	writeFile(t, s.ProjectPath("dev"), newer, 0o600)

	if err := s.Restore("dev", first.ID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("dev")); got != orig {
		t.Fatalf("restored content = %q", got)
	}
	bs, _ := s.Backups.List("dev")
	var pre *backup.Backup
	for i := range bs {
		if bs[i].Op == "prerestore" {
			pre = &bs[i]
		}
	}
	if pre == nil {
		t.Fatalf("no prerestore backup in %+v", bs)
	}
	if err := s.Restore("dev", pre.ID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("dev")); got != newer {
		t.Fatalf("undo content = %q, want pre-restore bytes", got)
	}
}

func TestRestoreKeepsSymlink(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	bk, _ := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.ProjectPath("dev"), validCfg, 0o600)
	if err := s.Restore("dev", bk.ID); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkOK)
	if cfg, err := kubecfg.Load(s.ConfigPath()); err != nil || cfg.Contexts["a"] == nil {
		t.Fatalf("through link: %+v, %v", cfg, err)
	}
}

func TestRestoreErrors(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := s.Restore("dev", "nope"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want not-exist", err)
	}
	if err := s.Restore("../x", "nope"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestRestoreDeletedProject(t *testing.T) {
	s := newStore(t)
	want := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", want)
	if err := s.Delete("dev", false); err != nil {
		t.Fatal(err)
	}
	bs, _ := s.Backups.List("dev")
	if err := s.Restore("dev", bs[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("dev")); got != want {
		t.Fatalf("restored = %q", got)
	}
}

func TestTargetResolution(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	addProject(t, s, "prod", validCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	shellCopy := s.ShellCopyPath(1234, "prod")
	inShell := Env{InShell: true, Project: "prod", Kubeconfig: shellCopy}
	cases := []struct {
		name string
		flag string
		env  Env
		want Target
	}{
		{"not in shell", "", Env{}, Target{Path: s.ProjectPath("dev"), Project: "dev"}},
		{"not in shell with flag", "prod", Env{}, Target{Path: s.ProjectPath("prod"), Project: "prod"}},
		{"in shell no flag", "", inShell, Target{Path: shellCopy, Project: "prod", InShell: true}},
		{"in shell with flag", "dev", inShell, Target{Path: s.ProjectPath("dev"), Project: "dev"}},
	}
	for _, c := range cases {
		got, err := s.Target(c.flag, c.env)
		if err != nil || got != c.want {
			t.Errorf("%s: Target = %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	if _, err := s.Target("nope", Env{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown project flag: %v", err)
	}
	if _, err := s.Target("../x", Env{}); !errors.Is(err, ErrInvalidName) {
		t.Errorf("invalid project flag: %v", err)
	}
	if _, err := s.Target("", Env{InShell: true, Project: "prod"}); err == nil {
		t.Error("in shell without KUBECONFIG must fail")
	}
}

func TestTargetWithoutActiveProject(t *testing.T) {
	s := newStore(t)
	if _, err := s.Target("", Env{}); err == nil {
		t.Fatal("expected error with no active project")
	}
	writeFile(t, s.ConfigPath(), validCfg, 0o600)
	if _, err := s.Target("", Env{}); !errors.Is(err, ErrForeignConfig) {
		t.Fatalf("err = %v, want ErrForeignConfig", err)
	}
}

func TestEnvFromOS(t *testing.T) {
	t.Setenv("KX_SHELL", "1")
	t.Setenv("KX_PROJECT", "prod")
	t.Setenv("KUBECONFIG", "/x/y.yaml")
	if got := EnvFromOS(); got != (Env{InShell: true, Project: "prod", Kubeconfig: "/x/y.yaml"}) {
		t.Fatalf("EnvFromOS() = %+v", got)
	}
	t.Setenv("KX_SHELL", "")
	if EnvFromOS().InShell {
		t.Fatal("InShell without KX_SHELL=1")
	}
}

func TestSetContextAndNamespace(t *testing.T) {
	s := newStore(t)
	two := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", two)
	if _, err := s.Import(writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2")), "dev", ImportMerge, kubecfg.MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	n := backupCount(t, s, "dev")
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	tgt, err := s.Target("", Env{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetContext(tgt, "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNamespace(tgt, "kube-system"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := kubecfg.Load(s.ProjectPath("dev"))
	if cfg.CurrentContext != "b" || cfg.Contexts["b"].Namespace != "kube-system" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if backupCount(t, s, "dev") != n {
		t.Fatal("SetContext/SetNamespace must not back up")
	}
	if err := s.SetContext(tgt, "zzz"); err == nil {
		t.Fatal("expected error for unknown context")
	}
}

func TestSetNamespaceDoesNotBackup(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	if err := s.SetNamespace(Target{Path: s.ProjectPath("dev"), Project: "dev"}, "x"); err != nil {
		t.Fatal(err)
	}
	if backupCount(t, s, "dev") != 0 {
		t.Fatal("namespace change took a backup")
	}
}

// failCreate makes every new backup fail while existing ones stay readable.
func failCreate(s *Store) {
	s.create = func(project, op string, data []byte) (backup.Backup, error) {
		return backup.Backup{}, errors.New("disk full")
	}
}

func TestRestoreAndRenameAbortWhenBackupFails(t *testing.T) {
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", orig)
	bk, err := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.ProjectPath("dev"), validCfg, 0o600)
	failCreate(s)

	if err := s.Restore("dev", bk.ID); err == nil {
		t.Error("Restore must abort when the pre-restore backup fails")
	}
	if got := readFile(t, s.ProjectPath("dev")); got != validCfg {
		t.Errorf("project changed by aborted Restore: %q", got)
	}
	if err := s.Rename("dev", "dev2"); err == nil {
		t.Error("Rename must abort when the backup fails")
	}
	if _, err := os.Lstat(s.ProjectPath("dev2")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dev2 must not exist: %v", err)
	}
	if got := readFile(t, s.ProjectPath("dev")); got != validCfg {
		t.Errorf("project changed by aborted Rename: %q", got)
	}
}

func TestImportPruneFailureIsWarning(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	real := s.Backups.CreateData
	s.create = func(project, op string, data []byte) (backup.Backup, error) {
		b, err := real(project, op, data)
		if err != nil {
			return b, err
		}
		return b, errors.New("prune backups: boom")
	}
	res, err := s.Import(writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2")), "dev", ImportMerge, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Backup == nil || len(res.Warnings) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if cfg, _ := kubecfg.Load(s.ProjectPath("dev")); cfg.Contexts["b"] == nil {
		t.Fatal("import not written")
	}
}

func TestDeleteClearsPrevious(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "a", validCfg)
	addProject(t, s, "b", validCfg)
	if err := s.Use("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("b"); err != nil { // previous = a
		t.Fatal(err)
	}
	if err := s.Delete("a", false); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Previous(); p != "" {
		t.Fatalf("Previous() = %q, want cleared", p)
	}
}

func TestTargetInShellRejectsForeignKubeconfig(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	inside := s.ShellCopyPath(1234, "dev")
	if got, err := s.Target("", Env{InShell: true, Project: "dev", Kubeconfig: inside}); err != nil || got.Path != inside {
		t.Fatalf("valid shell copy: %+v, %v", got, err)
	}
	for _, kc := range []string{
		filepath.Join(s.Root, "config-dev"),
		"/etc/passwd",
		inside + ":" + s.ProjectPath("dev"),
		filepath.Join(s.ShellsDir(), "..", "x.yaml"),
	} {
		_, err := s.Target("", Env{InShell: true, Project: "dev", Kubeconfig: kc})
		if err == nil || !strings.Contains(err.Error(), "KUBECONFIG was changed") {
			t.Errorf("Kubeconfig %q: err = %v", kc, err)
		}
	}
}

// rawCfgWithCA is a master whose cluster references ca.crt by relative path.
func rawCfgWithCA(ca string) string {
	return "apiVersion: v1\nkind: Config\ncurrent-context: a\nclusters:\n- name: ca\n  cluster:\n    server: https://a\n    certificate-authority: " + ca +
		"\ncontexts:\n- name: a\n  context:\n    cluster: ca\n    user: ua\nusers:\n- name: ua\n  user:\n    token: t1\n"
}

func TestImportMergeKeepsMasterFileReferences(t *testing.T) {
	s := newStore(t)
	writeFile(t, filepath.Join(s.Root, "ca.crt"), "CA", 0o600)
	addProject(t, s, "dev", rawCfgWithCA("ca.crt"))
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	if _, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, s.ProjectPath("dev"))
	if !strings.Contains(got, "certificate-authority: ca.crt") || strings.Contains(got, "certificate-authority-data") {
		t.Fatalf("untouched master entry was flattened:\n%s", got)
	}
	if !strings.Contains(got, "name: cb") {
		t.Fatalf("incoming not merged:\n%s", got)
	}
}

func TestImportSameContentDifferentFormNoChange(t *testing.T) {
	s := newStore(t)
	writeFile(t, filepath.Join(s.Root, "ca.crt"), "CA", 0o600)
	addProject(t, s, "dev", rawCfgWithCA("ca.crt"))
	// The same cluster with the CA inlined is not a conflict nor a change.
	in := writeIncoming(t, "apiVersion: v1\nkind: Config\nclusters:\n- name: ca\n  cluster:\n    server: https://a\n    certificate-authority-data: Q0E=\n")
	res, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || backupCount(t, s, "dev") != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestImportMasterWithDanglingCAStillImports(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", rawCfgWithCA("gone.crt"))
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	res, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !res.Changed {
		t.Fatalf("result = %+v", res)
	}
	got := readFile(t, s.ProjectPath("dev"))
	if !strings.Contains(got, "certificate-authority: gone.crt") || !strings.Contains(got, "name: cb") {
		t.Fatalf("unexpected project:\n%s", got)
	}
	if cs, err := s.PreviewImport(in, "dev"); err != nil || len(cs) != 0 {
		t.Fatalf("PreviewImport = %v, %v", cs, err)
	}
}

func TestPreviewMergeReportsReboundAndWritesNothing(t *testing.T) {
	s := newStore(t)
	master := mkCfg("a", "shared", "https://old", "ua", "t1")
	addProject(t, s, "dev", master)
	in := writeIncoming(t, mkCfg("b", "shared", "https://new", "ub", "t2"))
	rep, err := s.PreviewMerge(in, "dev", kubecfg.MergeOptions{Default: kubecfg.PolicyOverwrite})
	if err != nil {
		t.Fatal(err)
	}
	want := []kubecfg.EntityRef{{Kind: kubecfg.KindContext, Name: "a"}}
	if len(rep.Rebound) != 1 || rep.Rebound[0] != want[0] {
		t.Fatalf("Rebound = %v, want %v", rep.Rebound, want)
	}
	if readFile(t, s.ProjectPath("dev")) != master || backupCount(t, s, "dev") != 0 {
		t.Fatal("preview wrote something")
	}
	if _, err := s.PreviewMerge(in, "dev", kubecfg.MergeOptions{}); !errors.Is(err, kubecfg.ErrConflicts) {
		t.Fatalf("err = %v, want ErrConflicts", err)
	}
	rep, err = s.PreviewMerge(in, "nope", kubecfg.MergeOptions{})
	if err != nil || len(rep.Added) == 0 {
		t.Fatalf("new project: %+v, %v", rep, err)
	}
}

func TestEmptyProjectFileIsRefused(t *testing.T) {
	s := newStore(t)
	writeFile(t, s.ProjectPath("dev"), "", 0o600)
	tgt := Target{Path: s.ProjectPath("dev"), Project: "dev"}
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	checks := map[string]error{}
	_, checks["import"] = s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	_, checks["rmctx"] = s.RemoveContext("dev", "a", false)
	checks["ctx"] = s.SetContext(tgt, "a")
	checks["ns"] = s.SetNamespace(tgt, "x")
	for op, err := range checks {
		if !errors.Is(err, ErrEmptyProject) {
			t.Errorf("%s: err = %v, want ErrEmptyProject", op, err)
		}
	}
	if readFile(t, s.ProjectPath("dev")) != "" || backupCount(t, s, "dev") != 0 {
		t.Fatal("empty file was touched or backed up")
	}
}

func TestRestoreRecoversEmptyProjectFile(t *testing.T) {
	s := newStore(t)
	want := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", want)
	bk, err := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, s.ProjectPath("dev"), "", 0o600) // truncated by something else
	if err := s.Restore("dev", bk.ID); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("dev")); got != want {
		t.Fatalf("restored = %q", got)
	}
}

func TestOperationsFailWhenLockIsHeld(t *testing.T) {
	old := fsutil.LockWait
	fsutil.LockWait = 40 * time.Millisecond
	defer func() { fsutil.LockWait = old }()
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", orig)
	bk, _ := s.Backups.Create("dev", "import", s.ProjectPath("dev"))
	writeFile(t, s.ProjectPath("dev")+".lock", "", 0o600)
	tgt := Target{Path: s.ProjectPath("dev"), Project: "dev"}
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))

	checks := map[string]error{}
	_, checks["import"] = s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{})
	_, checks["rmctx"] = s.RemoveContext("dev", "a", false)
	checks["ctx"] = s.SetContext(tgt, "a")
	checks["ns"] = s.SetNamespace(tgt, "x")
	checks["restore"] = s.Restore("dev", bk.ID)
	for op, err := range checks {
		if err == nil || !strings.Contains(err.Error(), ".lock") {
			t.Errorf("%s: err = %v, want a lock error", op, err)
		}
	}
	if readFile(t, s.ProjectPath("dev")) != orig {
		t.Fatal("project changed while locked")
	}
}

func TestLockFileIsRemovedAfterWrite(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", mkCfg("a", "ca", "https://a", "ua", "t1"))
	if err := s.SetNamespace(Target{Path: s.ProjectPath("dev"), Project: "dev"}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.ProjectPath("dev") + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock file left behind: %v", err)
	}
}

func TestImportBacksUpTheBytesItParsed(t *testing.T) {
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	addProject(t, s, "dev", orig)
	var saw []byte
	real := s.Backups.CreateData
	s.create = func(project, op string, data []byte) (backup.Backup, error) {
		saw = data
		return real(project, op, data)
	}
	in := writeIncoming(t, mkCfg("b", "cb", "https://b", "ub", "t2"))
	if _, err := s.Import(in, "dev", ImportMerge, kubecfg.MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	if string(saw) != orig {
		t.Fatalf("backup bytes = %q, want the original file", saw)
	}
}

func TestRestoreInitialWritesNewProject(t *testing.T) {
	s := newStore(t)
	orig := mkCfg("a", "ca", "https://a", "ua", "t1")
	writeFile(t, s.ConfigPath(), orig, 0o600)
	addProject(t, s, "dev", validCfg)
	if err := s.AdoptForeign(""); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	bs, _ := s.Backups.List("_initial")
	if len(bs) != 1 {
		t.Fatalf("setup: %v", bs)
	}
	// Refusals: bad name, existing project, unknown id.
	if err := s.RestoreInitial("", "../x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
	if err := s.RestoreInitial("", "dev"); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if err := s.RestoreInitial("nope", "old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want not-exist", err)
	}
	if err := s.RestoreInitial("", "old"); err != nil { // latest
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("old")); got != orig {
		t.Fatalf("restored = %q", got)
	}
	assertState(t, s, "dev", LinkOK) // never touches the active link
	if err := s.RestoreInitial(bs[0].ID, "old2"); err != nil {
		t.Fatal(err)
	}
}
